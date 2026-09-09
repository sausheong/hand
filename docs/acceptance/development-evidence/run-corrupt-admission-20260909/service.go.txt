// Package app owns Hand application operations independently of terminal rendering.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/harness/llm"
)

type State string

const (
	Idle               State = "idle"
	Running            State = "running"
	AwaitingApproval   State = "awaiting_approval"
	CheckingCompletion State = "checking_completion"
	Compacting         State = "compacting"
	Cancelling         State = "cancelling"
)

var ErrBusy = errors.New("another application operation is active")

type BackendEvent struct {
	Kind    string
	Details Details
	Text    string
	Done    bool
	Err     error
}

// Backend closes its stream only after its turn and deferred cleanup have joined.
type Backend interface {
	Run(context.Context, string, []llm.ImageContent) (<-chan BackendEvent, error)
	StopReason() string
}
type Options struct {
	RunBoundary   RunBoundary // fixed before use; owned capture around the whole run
	ResolveInput  func(context.Context, string) (agentio.PromptInput, error)
	Model         ModelInfo
	InputTypes    []string
	SessionID     string
	MaxIterations int
	Check         func(context.Context, string, int) agentio.GoalLoopOutcome
}

// Event contains values only. Error text is captured at publication so later
// mutation of a backend error cannot change an already delivered event.
type Event struct {
	Model      ModelInfo
	ApprovalID string
	SessionID  string
	RunID      uint64
	Sequence   uint64
	Timestamp  time.Time
	Kind       string // state, text, tool_call, tool_result, usage, compaction_*, turn_end, continuation, terminal
	Details    Details
	State      State
	Text       string
	Iteration  int
	Status     agentio.RunStatus
	Reason     string
	Error      string
	Verified   bool
	Truncated  bool
}
type Snapshot struct {
	SessionID string
	RunID     uint64
	State     State
}

type Service struct {
	steeringSnapshots map[string]agentio.PromptInput
	inputs            []QueuedInput
	control           chan Event
	approvals         map[string]*approvalWait
	mu                sync.Mutex
	backend           Backend
	options           Options
	state             State
	active            bool
	runID             uint64
	operationID       uint64
	sequence          uint64
	cancel            context.CancelFunc
}

func New(backend Backend, options Options) *Service {
	options.InputTypes = slices.Clone(options.InputTypes)
	return &Service{control: make(chan Event), approvals: make(map[string]*approvalWait), backend: backend, options: options, state: Idle}
}
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{SessionID: s.options.SessionID, RunID: s.runID, State: s.state}
}

// Cancel requests cancellation; the operation remains owned until its backend
// and checker return. A new run cannot overlap a still-exiting operation.
func (s *Service) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active || s.state == Idle {
		return false
	}
	s.state = Cancelling
	s.cancel()
	return true
}

// Execute owns the full goal loop. consume is called serially in event order
// and must return promptly; callers may cancel from another goroutine. It is
// an internal synchronous operation, not the future public subscription API.
// Rejected concurrent operations return ErrBusy without emitting a terminal.
func (s *Service) Execute(parent context.Context, prompt string, images []llm.ImageContent, consume func(Event)) (result agentio.RunOutcome, err error) {
	ctx, cancel, runID, _, err := s.beginGoal(parent, images)
	if err != nil {
		return result, err
	}
	return s.executeOwned(ctx, cancel, runID, prompt, images, consume)
}

func (s *Service) beginGoal(parent context.Context, images []llm.ImageContent) (context.Context, context.CancelFunc, uint64, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.beginGoalLocked(parent, images)
}

func (s *Service) beginGoalLocked(parent context.Context, images []llm.ImageContent) (context.Context, context.CancelFunc, uint64, uint64, error) {
	if s.active {
		return nil, nil, 0, 0, ErrBusy
	}
	if len(images) > 0 && len(s.options.InputTypes) > 0 && !slices.Contains(s.options.InputTypes, "image") {
		return nil, nil, 0, 0, errors.New("selected profile does not support image input; choose an image-capable profile")
	}
	if s.backend == nil {
		return nil, nil, 0, 0, errors.New("application backend is not configured")
	}
	s.active = true
	s.operationID++
	s.runID++
	s.state = Running
	ctx, cancel := context.WithCancel(agentio.WithRunIdentity(context.WithValue(parent, serviceContextKey{}, s), agentio.RunIdentity{SessionID: s.options.SessionID, RunID: s.runID, Generation: s.runID}))
	s.cancel = cancel
	return ctx, cancel, s.runID, s.operationID, nil
}

func (s *Service) executeOwned(ctx context.Context, cancel context.CancelFunc, runID uint64, prompt string, images []llm.ImageContent, consume func(Event)) (result agentio.RunOutcome, err error) {
	defer func() {
		cancel()
		s.mu.Lock()
		s.active = false
		s.state = Idle
		s.cancel = nil
		s.mu.Unlock()
	}()
	publish := func(event Event) {
		s.mu.Lock()
		s.sequence++
		event.Sequence = s.sequence
		event.SessionID = s.options.SessionID
		event.RunID = runID
		event.Model = boundModelInfo(s.options.Model)
		event.Truncated = event.Truncated || event.Model.Truncated
		event.State = s.state
		s.mu.Unlock()
		event.Timestamp = time.Now().UTC()
		if consume != nil {
			consume(event)
		}
	}
	emit := func(event Event) {
		if event.Kind == "text" {
			for len(event.Text) > MaxEventTextBytes {
				if ctx.Err() != nil {
					return
				}
				size := eventPrefixBytes(event.Text)
				chunk := event
				chunk.Text = strings.Clone(event.Text[:size])
				publish(chunk)
				event.Text = event.Text[size:]
			}
		}
		if len(event.Text) > MaxEventTextBytes {
			event.Text = event.Text[:eventPrefixBytes(event.Text)]
			event.Truncated = true
		}
		if len(event.Error) > MaxEventTextBytes {
			event.Error = event.Error[:eventPrefixBytes(event.Error)]
			event.Truncated = true
		}
		event.Details = boundDetails(event.Details)
		event.Truncated = event.Truncated || event.Details.Truncated
		event.Text = strings.Clone(event.Text)
		event.Error = strings.Clone(event.Error)
		publish(event)
	}
	transition := func(state State, iteration int) {
		s.mu.Lock()
		if ctx.Err() != nil {
			s.state = Cancelling
		} else {
			s.state = state
		}
		s.mu.Unlock()
		emit(Event{Kind: "state", Iteration: iteration})
	}
	finish := func(status agentio.RunStatus, reason string, iteration int, cause error) agentio.RunOutcome {
		return agentio.RunOutcome{Status: status, Reason: reason, Iterations: iteration, Cause: cause}
	}
	cancelledOutcome := func(iteration int) agentio.RunOutcome {
		cause := context.Cause(ctx)
		if failure := agentio.TerminalFailure("", false, cause, false, iteration); failure != nil && failure.Status == agentio.BudgetExhausted {
			return *failure
		}
		return finish(agentio.Cancelled, "context_cancelled", iteration, ctx.Err())
	}
	execute := func() agentio.RunOutcome {
		if s.options.MaxIterations < 1 {
			return finish(agentio.BudgetExhausted, "max_iterations", 0, nil)
		}
		current := prompt
		for iteration := 1; ; iteration++ {
			if ctx.Err() != nil {
				return cancelledOutcome(iteration - 1)
			}
			transition(Running, iteration)
			events, startErr := s.backend.Run(ctx, current, images)
			images = nil
			if startErr != nil {
				if ctx.Err() != nil {
					return cancelledOutcome(iteration)
				}
				if failure := agentio.TerminalFailure("", false, startErr, false, iteration); failure != nil && failure.Status == agentio.BudgetExhausted {
					return *failure
				}
				return finish(agentio.InfrastructureError, "run_start_failed", iteration, startErr)
			}
			if events == nil {
				return finish(agentio.InfrastructureError, "missing_event_stream", iteration, errors.New("backend returned a nil stream"))
			}
			var runErr error
			done := false
		backendLoop:
			for {
				var event BackendEvent
				select {
				case control := <-s.control:
					control.Iteration = iteration
					if ctx.Err() == nil {
						emit(control)
					}
					continue
				case value, ok := <-events:
					if !ok {
						break backendLoop
					}
					event = value
				}
				if event.Kind != "" && (ctx.Err() == nil || event.Kind == "session_usage") {
					emit(Event{Kind: event.Kind, Details: event.Details, Iteration: iteration})
				}
				if event.Text != "" && ctx.Err() == nil {
					emit(Event{Kind: "text", Text: event.Text, Iteration: iteration})
				}
				if event.Done {
					done = true
				}
				if event.Err != nil && runErr == nil {
					runErr = event.Err
				}
			}
			emit(Event{Kind: "turn_end", Iteration: iteration})
			if ctx.Err() != nil {
				return cancelledOutcome(iteration)
			}
			reason := s.backend.StopReason()
			if failure := agentio.TerminalFailure(reason, done, runErr, false, iteration); failure != nil {
				return *failure
			}
			transition(CheckingCompletion, iteration)
			check := agentio.GoalLoopOutcome{}
			if s.options.Check != nil {
				check = s.options.Check(ctx, reason, iteration)
			}
			if ctx.Err() != nil {
				return cancelledOutcome(iteration)
			}
			if check.Err != nil {
				return finish(agentio.VerificationFailed, "stop_validator_failed", iteration, check.Err)
			}
			if !check.Continue {
				outcome := finish(agentio.Completed, "answer_completed", iteration, nil)
				outcome.Verified = check.Verified
				return outcome
			}
			if iteration >= s.options.MaxIterations {
				return finish(agentio.BudgetExhausted, "max_iterations", iteration, nil)
			}
			current = check.NextPrompt
			emit(Event{Kind: "continuation", Text: current, Iteration: iteration + 1})
		}
	}
	journal, journalEnabled := s.backend.(runJournal)
	record := RunRecord{Version: 1, ID: rand.Text(), RunID: runID, Phase: "start", At: time.Now().UTC(), Model: boundModelInfo(s.options.Model)}
	var budgetPrepareErr error
	if preparer, ok := s.backend.(runBudgetPreparer); ok {
		var release context.CancelFunc
		ctx, release, record.BudgetID, budgetPrepareErr = preparer.PrepareSelectedRunBudget(ctx)
		defer release()
	}
	journalStarted := false
	if journalEnabled {
		if journalErr := journal.RecordRun(record); journalErr != nil {
			result = finish(agentio.InfrastructureError, "run_metadata_start_failed", 0, journalErr)
		} else {
			journalStarted = true
		}
	}
	if budgetPrepareErr != nil {
		if failure := agentio.TerminalFailure("", false, budgetPrepareErr, false, 0); failure != nil && failure.Status == agentio.BudgetExhausted {
			result = *failure
		} else {
			result = finish(agentio.InfrastructureError, "run_budget_prepare_failed", 0, budgetPrepareErr)
		}
	} else if !journalEnabled || journalStarted {
		var finishBoundary func(context.Context) error
		var boundaryErr error
		if s.options.RunBoundary != nil {
			finishBoundary, boundaryErr = s.options.RunBoundary.Begin(ctx, record.ID)
		}
		if boundaryErr != nil {
			result = finish(agentio.InfrastructureError, "checkpoint_start_failed", 0, boundaryErr)
		} else {
			result = execute()
			if finishBoundary != nil {
				// Cleanup capture is bounded and joined even if execution was cancelled.
				finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				boundaryErr = finishBoundary(finishCtx)
				finishCancel()
				if boundaryErr != nil {
					result = finish(agentio.InfrastructureError, "checkpoint_finish_failed", result.Iterations, errors.Join(result.Cause, boundaryErr))
				}
			}
		}
	}
	s.mu.Lock()
	// Commit completion under the same lock as Cancel. A cancellation accepted
	// before this point wins; after this point Cancel returns false.
	if ctx.Err() != nil && result.Status != agentio.Cancelled {
		result = cancelledOutcome(result.Iterations)
	}
	s.state = Idle
	s.mu.Unlock()
	if journalStarted {
		record.Phase, record.At = "finish", time.Now().UTC()
		savedOutcome := result
		savedOutcome.Cause = nil // Error text is not needed in persistent metadata.
		record.Outcome = &savedOutcome
		if journalErr := journal.RecordRun(record); journalErr != nil {
			result = finish(agentio.InfrastructureError, "run_metadata_finish_failed", result.Iterations, errors.Join(result.Cause, journalErr))
		}
	}
	terminal := Event{Kind: "terminal", Status: result.Status, Reason: result.Reason, Iteration: result.Iterations, Verified: result.Verified}
	if result.Cause != nil {
		terminal.Error = result.Cause.Error()
	}
	emit(terminal)
	return result, nil
}

// reserve gives configuration/compaction and transitional turn adapters the
// same ownership gate as Execute. Release only after the operation joins.
func (s *Service) reserve(parent context.Context, state State) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return nil, nil, ErrBusy
	}
	ctx, cancel := context.WithCancel(parent)
	s.active = true
	s.operationID++
	s.state = state
	s.cancel = cancel
	var once sync.Once
	release := func() {
		once.Do(func() {
			cancel()
			s.mu.Lock()
			s.active = false
			s.state = Idle
			s.cancel = nil
			s.mu.Unlock()
		})
	}
	return ctx, release, nil
}

// Keep valid UTF-8 intact when splitting or explicitly truncating event text.
func eventPrefixBytes(value string) int {
	if len(value) <= MaxEventTextBytes {
		return len(value)
	}
	size := MaxEventTextBytes
	for size > 0 && value[size]&0xc0 == 0x80 {
		size--
	}
	if size == 0 {
		return MaxEventTextBytes
	} // input already contains invalid UTF-8
	return size
}

// ConfigureInputs applies declared capabilities only at an idle boundary.
// An empty declaration preserves legacy behaviour until metadata is available.
func (s *Service) ConfigureInputs(inputs []string) error {
	if len(inputs) > 0 && !slices.Contains(inputs, "text") {
		return errors.New("input capabilities must include text")
	}
	for _, kind := range inputs {
		if kind != "text" && kind != "image" {
			return errors.New("unsupported input capability: " + kind)
		}
	}
	_, release, err := s.reserve(context.Background(), Idle)
	if err != nil {
		return err
	}
	defer release()
	s.mu.Lock()
	s.options.InputTypes = slices.Clone(inputs)
	s.mu.Unlock()
	return nil
}

package agentio

import (
	"errors"
	"fmt"
	"github.com/sausheong/harness/budget"
	"github.com/sausheong/harness/runtime"
)

// RunStatus describes execution, independently of evidence that a coding task
// is correct. A completed answer with no required validator is not verified.
type RunStatus string

const (
	Completed           RunStatus = "completed"
	VerificationFailed  RunStatus = "verification_failed"
	BudgetExhausted     RunStatus = "budget_exhausted"
	Cancelled           RunStatus = "cancelled"
	InfrastructureError RunStatus = "infrastructure_error"
)

type RunOutcome struct {
	Status     RunStatus `json:"status"`
	Reason     string    `json:"reason"`
	Iterations int       `json:"iterations"`
	// Verified means all configured mandatory Stop validators passed. It is
	// scoped to those validators, not a claim of general coding correctness.
	Verified bool  `json:"verified"`
	Cause    error `json:"-"`
}

func (o RunOutcome) ExitCode() int {
	switch o.Status {
	case Completed:
		return 0
	case VerificationFailed:
		return 3
	case BudgetExhausted:
		return 4
	case Cancelled:
		return 130
	default:
		return 5
	}
}
func (o RunOutcome) Err() error {
	if o.Status == Completed {
		return nil
	}
	return &RunFailure{Outcome: o}
}

type RunFailure struct{ Outcome RunOutcome }

func (e *RunFailure) Error() string {
	if e.Outcome.Cause != nil {
		return fmt.Sprintf("%s (%s): %v", e.Outcome.Status, e.Outcome.Reason, e.Outcome.Cause)
	}
	return fmt.Sprintf("%s (%s)", e.Outcome.Status, e.Outcome.Reason)
}
func (e *RunFailure) Unwrap() error { return e.Outcome.Cause }

// VerificationError distinguishes policy/validator failure from provider or
// infrastructure failure when Harness relays a lifecycle hook error.
type VerificationError struct{ Cause error }

func (e *VerificationError) Error() string { return e.Cause.Error() }
func (e *VerificationError) Unwrap() error { return e.Cause }
func IsVerificationError(err error) bool {
	var target *VerificationError
	return errors.As(err, &target)
}

// TerminalFailure classifies a joined runtime turn before Stop validation.
// Nil means the answer ended normally and still needs any configured checks.
func TerminalFailure(reason string, done bool, runErr error, cancelled bool, iteration int) *RunOutcome {
	o := RunOutcome{Iterations: iteration, Cause: runErr}
	switch {
	case cancelled:
		o.Status, o.Reason = Cancelled, "context_cancelled"
	case errors.Is(runErr, budget.ErrRunTimeExhausted):
		o.Status, o.Reason = BudgetExhausted, "run_time_budget"
	case errors.Is(runErr, runtime.ErrRunTokenExhausted):
		o.Status, o.Reason = BudgetExhausted, "run_token_budget"
	case errors.Is(runErr, runtime.ErrRunCostExhausted):
		o.Status, o.Reason = BudgetExhausted, "run_cost_budget"
	case errors.Is(runErr, budget.ErrTimeExhausted):
		o.Status, o.Reason = BudgetExhausted, "session_time_budget"
	case errors.Is(runErr, budget.ErrCostExhausted):
		o.Status, o.Reason = BudgetExhausted, "session_cost_budget"
	case errors.Is(runErr, budget.ErrUnknownPrice):
		o.Status, o.Reason = BudgetExhausted, "cost_price_unknown"
	case errors.Is(runErr, budget.ErrExhausted):
		o.Status, o.Reason = BudgetExhausted, "session_token_budget"
	case reason == "max_turns":
		o.Status, o.Reason = BudgetExhausted, "max_turns"
	case IsVerificationError(runErr):
		o.Status, o.Reason = VerificationFailed, "prompt_validator_failed"
	case runErr != nil:
		o.Status, o.Reason = InfrastructureError, "runtime_error"
	case reason == "aborted":
		o.Status, o.Reason = Cancelled, "runtime_aborted"
	case !done || (reason != "" && reason != "completed"):
		o.Status, o.Reason = InfrastructureError, "missing_completion"
	default:
		return nil
	}
	return &o
}

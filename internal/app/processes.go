package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/process"
)

const MaxBackgroundProcesses = 8
const MaxProcessRecords = 32

type ProcessInfo struct {
	ID, Command string
	Running     bool
}
type processHandle interface {
	Wait() error
	Close() error
	Cancel()
	Send(context.Context, []byte) error
	Snapshot() process.HandleSnapshot
}
type processRecord struct {
	command string
	handle  processHandle
	done    chan struct{}
}

// Processes owns deliberately backgrounded work independently of a foreground
// goal. Admission/policy belongs to the caller; this registry never bypasses it.
type Processes struct {
	checkpointCapture bool
	backend           execution.Starter
	boundary          string
	mu                sync.Mutex
	ctx               context.Context
	cancel            context.CancelFunc
	workspace         string
	store             *process.ArtifactStore
	records           map[string]*processRecord
	order             []string
	closed            bool
}

func NewProcesses(parent context.Context, workspace string, store *process.ArtifactStore) (*Processes, error) {
	if store == nil {
		return nil, errors.New("background processes require a capture store")
	}
	ctx, cancel := context.WithCancel(parent)
	return &Processes{ctx: ctx, cancel: cancel, workspace: workspace, store: store, records: make(map[string]*processRecord)}, nil
}
func (p *Processes) Start(ctx context.Context, command string) (ProcessInfo, error) {
	if strings.TrimSpace(command) == "" || len(command) > 64<<10 || !utf8.ValidString(command) || strings.IndexByte(command, 0) >= 0 {
		return ProcessInfo{}, errors.New("command must be nonempty UTF-8 within 64 KiB without NUL")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.checkpointCapture {
		return ProcessInfo{}, errors.New("background start blocked during checkpoint capture")
	}
	if p.closed || p.ctx.Err() != nil {
		return ProcessInfo{}, errors.New("background process owner is closed")
	}
	if err := ctx.Err(); err != nil {
		return ProcessInfo{}, err
	}
	if len(p.records) >= MaxProcessRecords {
		return ProcessInfo{}, errors.New("process record limit reached; forget completed handles")
	}
	active := 0
	for _, r := range p.records {
		select {
		case <-r.done:
		default:
			active++
		}
	}
	if active >= MaxBackgroundProcesses {
		return ProcessInfo{}, errors.New("background process limit reached")
	}
	var handle processHandle
	var err error
	if p.backend != nil {
		handle, err = p.backend.Start(p.ctx, execution.Request{Argv: []string{"/bin/sh", "-c", command}, OutputStore: p.store})
	} else {
		handle, err = process.StartHandle(p.ctx, p.store, p.workspace, "sh", "-c", command)
	}
	if err != nil {
		return ProcessInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		handle.Close()
		return ProcessInfo{}, err
	}
	r := &processRecord{command: command, handle: handle, done: make(chan struct{})}
	id := handle.Snapshot().ID
	p.records[id] = r
	p.order = append(p.order, id)
	go func() { handle.Wait(); close(r.done) }()
	return ProcessInfo{ID: id, Command: command, Running: true}, nil
}
func (p *Processes) record(id string) (*processRecord, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.records[id]
	if !ok {
		return nil, errors.New("unknown process handle")
	}
	return r, nil
}
func (p *Processes) List() []ProcessInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ProcessInfo, 0, len(p.order))
	for _, id := range p.order {
		r := p.records[id]
		running := true
		select {
		case <-r.done:
			running = false
		default:
		}
		out = append(out, ProcessInfo{ID: id, Command: r.command, Running: running})
	}
	return out
}
func (p *Processes) Read(id string) (process.HandleSnapshot, error) {
	r, err := p.record(id)
	if err != nil {
		return process.HandleSnapshot{}, err
	}
	return r.handle.Snapshot(), nil
}
func (p *Processes) Send(ctx context.Context, id string, data []byte) error {
	r, err := p.record(id)
	if err != nil {
		return err
	}
	return r.handle.Send(ctx, data)
}
func (p *Processes) Cancel(id string) error {
	r, err := p.record(id)
	if err != nil {
		return err
	}
	r.handle.Cancel()
	return nil
}
func (p *Processes) Wait(ctx context.Context, id string) (process.HandleSnapshot, error) {
	r, err := p.record(id)
	if err != nil {
		return process.HandleSnapshot{}, err
	}
	select {
	case <-ctx.Done():
		return process.HandleSnapshot{}, ctx.Err()
	case <-r.done:
		return r.handle.Snapshot(), nil
	}
}
func (p *Processes) Forget(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.records[id]
	if !ok {
		return errors.New("unknown process handle")
	}
	select {
	case <-r.done:
	default:
		return errors.New("cannot forget a running process")
	}
	delete(p.records, id)
	for i, key := range p.order {
		if key == id {
			p.order = append(p.order[:i], p.order[i+1:]...)
			break
		}
	}
	return nil
}
func (p *Processes) Close() {
	// Cancel before acquiring the admission lock so a concurrent Start cannot
	// make shutdown wait for a live command it has just created.
	p.cancel()
	p.mu.Lock()
	p.closed = true
	records := make([]*processRecord, 0, len(p.records))
	for _, r := range p.records {
		records = append(records, r)
	}
	p.mu.Unlock()
	for _, r := range records {
		<-r.done
	}
}

// NewProcessesWithBackend refuses unsupported asynchronous execution instead of
// falling back to host processes. The configured backend remains immutable.
func NewProcessesWithBackend(parent context.Context, workspace string, store *process.ArtifactStore, backend execution.Backend) (*Processes, error) {
	starter, ok := backend.(execution.Starter)
	if !ok {
		return nil, errors.New("selected backend does not support owned background processes")
	}
	p, err := NewProcesses(parent, workspace, store)
	if err != nil {
		return nil, err
	}
	p.backend = starter
	p.boundary = backend.Boundary()
	return p, nil
}
func (p *Processes) Boundary() string {
	if p == nil || p.boundary == "" {
		return "unrestricted host"
	}
	return p.boundary
}

// pauseForCheckpoint atomically requires joined background processes and blocks
// new admission. It does not hold the registry mutex while disk capture runs.
func (p *Processes) pauseForCheckpoint(ctx context.Context) (func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.closed || p.ctx.Err() != nil {
		return nil, errors.New("background owner closed")
	}
	if p.checkpointCapture {
		return nil, errors.New("checkpoint capture already active")
	}
	for _, r := range p.records {
		select {
		case <-r.done:
		default:
			return nil, errors.New("checkpoint capture requires background processes to finish")
		}
	}
	p.checkpointCapture = true
	var once sync.Once
	return func() { once.Do(func() { p.mu.Lock(); p.checkpointCapture = false; p.mu.Unlock() }) }, nil
}

package sdk

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/sausheong/harness/process"
)

// ProcessOptions configures an owned Hand subprocess. Arguments must include
// --rpc; no shell is involved. A nil Environment inherits the parent environment.
// Stderr must accept writes promptly; Hand protocol output is reserved for SDK I/O.
type ProcessOptions struct {
	Binary          string
	Arguments       []string
	Directory       string
	Environment     []string
	Stderr          io.Writer
	ShutdownTimeout time.Duration
}

// StartProcess starts Hand and returns a client that owns its process and pipes.
// Closing the client first signals EOF, allowing Hand to cancel and join its
// active work. If it exceeds ShutdownTimeout (default five seconds), it is killed
// and reaped. The context owns the whole process lifetime, not just startup.
// Hello is explicit so clients can choose their negotiation request ID.
func StartProcess(ctx context.Context, options ProcessOptions) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.Binary == "" {
		return nil, errors.New("SDK process binary is required")
	}
	if options.ShutdownTimeout < 0 {
		return nil, errors.New("SDK shutdown timeout cannot be negative")
	}
	timeout := options.ShutdownTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	// Client.Close owns cancellation so Hand first receives EOF and a grace
	// period. The process group also owns children if graceful shutdown fails.
	cmd := process.Command(context.Background(), options.Binary, append([]string(nil), options.Arguments...)...)
	cmd.Dir = options.Directory
	if options.Environment != nil {
		cmd.Env = append([]string{}, options.Environment...)
	}
	cmd.Stderr = options.Stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, err
	}
	conn := &processConnection{input: input, output: output, cmd: cmd, timeout: timeout, done: make(chan struct{})}
	client := NewClient(conn)
	go func() {
		select {
		case <-ctx.Done():
			client.Close()
		case <-conn.done:
		}
	}()
	return client, nil
}

type processConnection struct {
	input   io.WriteCloser
	output  io.ReadCloser
	cmd     *exec.Cmd
	timeout time.Duration
	once    sync.Once
	done    chan struct{}
	err     error
}

func (p *processConnection) Read(b []byte) (int, error)  { return p.output.Read(b) }
func (p *processConnection) Write(b []byte) (int, error) { return p.input.Write(b) }
func (p *processConnection) Close() error {
	p.once.Do(func() {
		// Close both pipes immediately to release blocked callers, but give Hand time
		// to observe EOF and join the application before resorting to a process kill.
		p.input.Close()
		p.output.Close()
		exited := make(chan error, 1)
		go func() { exited <- p.cmd.Wait() }()
		timer := time.NewTimer(p.timeout)
		defer timer.Stop()
		select {
		case p.err = <-exited:
		case <-timer.C:
			process.KillGroup(p.cmd)
			p.err = <-exited
		}
		p.err = errors.Join(p.err, process.KillGroup(p.cmd))
		close(p.done)
	})
	return p.err
}

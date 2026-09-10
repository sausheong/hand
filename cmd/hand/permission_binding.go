package main

import (
	"errors"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/config"
	"sync"
)

// Retain opened journals through shutdown so an in-flight inspection never sees
// a closed or repurposed authority. Cap retained configurations per process.
type cliPermissionBindings struct {
	mu        sync.Mutex
	current   app.PermissionState
	states    map[string]app.PermissionState
	workspace string
	cfg       config.Config
	closed    bool
}

func (b *cliPermissionBindings) snapshot() app.PermissionState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.current
}
func (b *cliPermissionBindings) prepare(p config.ModelProfile) (func(), error) {
	digest, err := cliAuthorityDigest(b.cfg, p)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("permission bindings closed")
	}
	state, ok := b.states[digest]
	if !ok {
		if len(b.states) >= 64 {
			return nil, errors.New("permission configuration limit reached; restart before another configuration")
		}
		a, proposal, d, err := openCLIAuth(b.workspace, b.cfg, p)
		if err != nil {
			return nil, err
		}
		state = app.PermissionState{Authority: a, Legacy: proposal, Digest: d}
		b.states[digest] = state
	}
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if !b.closed {
			b.current = state
		}
	}, nil
}
func (b *cliPermissionBindings) close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, s := range b.states {
		s.Authority.Close()
	}
}

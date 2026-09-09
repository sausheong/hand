package app

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tools/mcp"
)

type MCPConnectionStatus struct{ Name, State string }
type optionalServer struct {
	config mcp.ServerConfig
	state  string
}

// OptionalMCP owns discovery and untransferred clients for this invocation.
// Connected clients transfer to Runtime and are closed by Runtime.Close.
type OptionalMCP struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	slots   chan struct{}
	servers map[string]*optionalServer
	order   []string
	closed  bool
	attach  func(*mcp.Client) error
}

func NewOptionalMCP(c *Controller, configs []mcp.ServerConfig) (*OptionalMCP, error) {
	if len(configs) > 32 {
		return nil, errors.New("at most 32 optional MCP servers are supported")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &OptionalMCP{ctx: ctx, cancel: cancel, slots: make(chan struct{}, 4), servers: make(map[string]*optionalServer)}
	for _, config := range configs {
		if !config.Optional || config.Name == "" || m.servers[config.Name] != nil {
			cancel()
			return nil, errors.New("optional MCP names must be nonempty and unique")
		}
		config.Args = append([]string(nil), config.Args...)
		config.Env = maps.Clone(config.Env)
		config.Headers = maps.Clone(config.Headers)
		m.servers[config.Name] = &optionalServer{config: config, state: "unavailable"}
		m.order = append(m.order, config.Name)
	}
	m.attach = func(client *mcp.Client) error {
		_, release, err := c.owner().reserve(context.Background(), Idle)
		if err != nil {
			return err
		}
		defer release()
		if !c.mu.TryLock() {
			return ErrBusy
		}
		defer c.mu.Unlock()
		return c.Rt.AttachMCP(client, true)
	}
	for _, name := range m.order {
		_ = m.Retry(name)
	}
	return m, nil
}
func (m *OptionalMCP) Retry(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return errors.New("MCP discovery is closed")
	}
	s := m.servers[name]
	if s == nil {
		return errors.New("unknown optional MCP server")
	}
	if s.state != "unavailable" {
		return fmt.Errorf("MCP server is %s", s.state)
	}
	s.state = "queued"
	m.wg.Add(1)
	go m.connect(s)
	return nil
}
func (m *OptionalMCP) set(s *optionalServer, state string) {
	m.mu.Lock()
	s.state = state
	m.mu.Unlock()
}
func (m *OptionalMCP) connect(s *optionalServer) {
	defer m.wg.Done()
	select {
	case m.slots <- struct{}{}:
	case <-m.ctx.Done():
		m.set(s, "unavailable")
		return
	}
	m.set(s, "connecting")
	timeout := s.config.ConnectTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	client, err := mcp.Connect(ctx, s.config)
	cancel()
	<-m.slots
	if err != nil {
		m.set(s, "unavailable")
		return
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = client.Close()
			m.set(s, "unavailable")
		}
	}()
	m.set(s, "ready")
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if m.ctx.Err() != nil {
			return
		}
		err = m.attach(client)
		if err == nil {
			transferred = true
			m.set(s, "connected")
			return
		}
		if !errors.Is(err, ErrBusy) && !errors.Is(err, runtime.ErrMCPAttachBusy) {
			return
		}
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (m *OptionalMCP) Status() []MCPConnectionStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MCPConnectionStatus, 0, len(m.order))
	for _, name := range m.order {
		out = append(out, MCPConnectionStatus{Name: name, State: m.servers[name].state})
	}
	return out
}
func (m *OptionalMCP) Close() { m.mu.Lock(); m.closed = true; m.cancel(); m.mu.Unlock(); m.wg.Wait() }

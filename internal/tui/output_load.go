package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
)

type outputLoad struct {
	cancel context.CancelFunc
	done   chan struct{}
	result outputLoaded
}

func (m *Model) startOutputLoad(v *outputViewer, read func(context.Context) (ToolOutput, error)) tea.Cmd {
	if m.outputLoads == nil {
		m.outputLoads = make(map[*outputLoad]struct{})
	}
	for load := range m.outputLoads {
		select {
		case <-load.done:
			delete(m.outputLoads, load)
		default:
		}
	}
	if len(m.outputLoads) >= 4 {
		v.status = "Previous output loads are still stopping; retry shortly"
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	load := &outputLoad{cancel: cancel, done: make(chan struct{})}
	m.outputLoads[load] = struct{}{}
	v.load = load
	go func() {
		defer close(load.done)
		defer cancel()
		block, err := read(ctx)
		if err == nil {
			err = ctx.Err()
		}
		load.result = outputLoaded{viewer: v, block: block, err: err, load: load}
	}()
	return func() tea.Msg { <-load.done; return load.result }
}
func (m *Model) closeOutputView() {
	if m.outputView != nil && m.outputView.load != nil {
		m.outputView.load.cancel()
	}
	m.outputView = nil
}
func (m *Model) closeOutputLoads() {
	for load := range m.outputLoads {
		load.cancel()
	}
	for load := range m.outputLoads {
		<-load.done
		delete(m.outputLoads, load)
	}
}

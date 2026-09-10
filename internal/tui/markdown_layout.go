package tui

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"sort"
)

type markdownLayout struct {
	cancel            context.CancelFunc
	done              chan struct{}
	width             int
	style             string
	epoch             uint64
	waiting, complete bool
	result            map[int]sourceLayout
}
type markdownLayoutDone struct{ task *markdownLayout }

func (m *Model) startMarkdownLayout(sources map[int]sourceLayout) {
	ctx, cancel := context.WithCancel(context.Background())
	task := &markdownLayout{cancel: cancel, done: make(chan struct{}), width: m.termWidth, style: m.markdownStyle, epoch: m.layoutEpoch}
	m.markdownLayout = task
	go func() {
		defer close(task.done)
		defer cancel()
		task.result = make(map[int]sourceLayout, len(sources))
		indices := make([]int, 0, len(sources))
		for index := range sources {
			indices = append(indices, index)
		}
		sort.Ints(indices)
		for _, index := range indices {
			if ctx.Err() != nil {
				return
			}
			source := sources[index]
			source.Rendered = source.Block.render(task.width, task.style)
			source.Width, source.Style = task.width, task.style
			task.result[index] = source
		}
		task.complete = ctx.Err() == nil
	}()
}
func (m *Model) waitMarkdownLayout() tea.Cmd {
	task := m.markdownLayout
	if task == nil || task.waiting {
		return nil
	}
	task.waiting = true
	return func() tea.Msg { <-task.done; return markdownLayoutDone{task: task} }
}
func (m *Model) closeMarkdownLayout() {
	if m.markdownLayout != nil {
		m.markdownLayout.cancel()
		<-m.markdownLayout.done
		m.markdownLayout = nil
	}
}

//go:build unix

package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sausheong/harness/process"
)

func testProcesses(t *testing.T) *Processes {
	t.Helper()
	store, err := process.NewArtifactStore(filepath.Join(t.TempDir(), "captures"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProcesses(context.Background(), t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}
func waitProcess(t *testing.T, p *Processes, id string) process.HandleSnapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := p.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestProcessesInteractiveLifetime(t *testing.T) {
	p := testProcesses(t)
	foreground, cancel := context.WithCancel(context.Background())
	info, err := p.Start(foreground, `read line; printf 'reply:%s' "$line"; exit 7`)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := p.Forget(info.ID); err == nil {
		t.Fatal("forgot running process")
	}
	if _, err := p.Wait(foreground, info.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait: %v", err)
	}
	s, err := p.Read(info.ID)
	if err != nil || !s.Running {
		t.Fatalf("foreground cancellation stopped background process: %+v %v", s, err)
	}
	if err := p.Send(context.Background(), info.ID, []byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	s = waitProcess(t, p, info.ID)
	if s.Running || s.ExitCode != 7 || s.Stdout != "reply:hello" {
		t.Fatalf("result: %+v", s)
	}
	list := p.List()
	if len(list) != 1 || list[0].Running || list[0].ID != info.ID {
		t.Fatalf("list: %+v", list)
	}
	if err := p.Forget(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Read(info.ID); err == nil {
		t.Fatal("forgotten handle readable")
	}
}
func TestProcessesAdmissionAndRetention(t *testing.T) {
	p := testProcesses(t)
	for i := 0; i < MaxBackgroundProcesses; i++ {
		if _, err := p.Start(context.Background(), "read line"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.Start(context.Background(), "true"); err == nil {
		t.Fatal("active limit bypassed")
	}
	for _, info := range p.List() {
		if err := p.Cancel(info.ID); err != nil {
			t.Fatal(err)
		}
		waitProcess(t, p, info.ID)
	}
	for i := MaxBackgroundProcesses; i < MaxProcessRecords; i++ {
		info, err := p.Start(context.Background(), "true")
		if err != nil {
			t.Fatal(err)
		}
		waitProcess(t, p, info.ID)
	}
	if _, err := p.Start(context.Background(), "true"); err == nil {
		t.Fatal("retention limit bypassed")
	}
	if err := p.Forget(p.List()[0].ID); err != nil {
		t.Fatal(err)
	}
	info, err := p.Start(context.Background(), "true")
	if err != nil {
		t.Fatal(err)
	}
	waitProcess(t, p, info.ID)
}
func TestProcessesConcurrentShutdown(t *testing.T) {
	p := testProcesses(t)
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); p.Start(context.Background(), "sleep 30") }()
	}
	p.Close()
	workers.Wait()
	p.Close()
	for _, info := range p.List() {
		if info.Running {
			t.Fatalf("unjoined process: %+v", info)
		}
	}
	if _, err := p.Start(context.Background(), "true"); err == nil {
		t.Fatal("start after shutdown")
	}
}

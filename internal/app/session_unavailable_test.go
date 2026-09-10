package app

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sausheong/hand/internal/sessionio"
)

func TestSessionOperationsWithoutRuntimePreserveCatalogue(t *testing.T) {
	manager, err := sessionio.NewManager(t.TempDir(), t.TempDir(), "hand")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := manager.Open(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Session.Close()
	before, err := manager.Catalogue().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "export.json")
	operations := map[string]func(*Controller) error{
		"list":   func(c *Controller) error { _, e := c.ListSessions(); return e },
		"rename": func(c *Controller) error { return c.RenameSession("changed") },
		"resume": func(c *Controller) error { return c.ResumeSession(selected.Session.ID) },
		"new":    func(c *Controller) error { return c.NewSession() },
		"fork":   func(c *Controller) error { return c.ForkSession(context.Background()) },
		"export": func(c *Controller) error { return c.ExportSession(context.Background(), destination) },
		"tree":   func(c *Controller) error { _, e := c.SessionTree(context.Background()); return e },
		"select": func(c *Controller) error { return c.SelectSessionNode(context.Background(), "missing") },
		"usage":  func(c *Controller) error { _, e := c.SessionUsage(); return e },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("unavailable runtime panicked: %v", p)
				}
			}()
			c := &Controller{Sessions: manager}
			if err := operation(c); err == nil {
				t.Fatal("operation succeeded without runtime")
			}
			after, err := manager.Catalogue().Snapshot()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("catalogue changed: %v", err)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("unexpected export: %v", err)
			}
			if _, release, err := c.owner().reserve(context.Background(), Idle); err != nil {
				t.Fatal("ownership leaked", err)
			} else {
				release()
			}
		})
	}
	t.Run("history", func(t *testing.T) {
		c := &Controller{Sessions: manager}
		if history := c.SessionHistory(); len(history) != 0 {
			t.Fatal("unavailable history returned entries")
		}
	})
}

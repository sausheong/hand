package extensions

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/harness/session"
)

func TestGoAndPythonTaskNoteExamples(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "task-note")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./examples/extensions/go-task-note")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Go example: %v: %s", err, output)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("Python 3 required for extension example qualification:", err)
	}
	for _, language := range []string{"go", "python"} {
		t.Run(language, func(t *testing.T) {
			cfg := LaunchConfig{Name: "task-note", Executable: binary, Workspace: t.TempDir(), Capabilities: []string{"commands", "questions", "state", "context.transform"}}
			if language == "python" {
				cfg.Executable = python
				cfg.PackageDir = filepath.Join(repo, "examples/extensions/python-task-note")
				cfg.Files = []string{"main.py"}
				cfg.Arguments = []string{"-I", "-B", "${package}/main.py"}
			}
			review, err := ReviewHostLaunch(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			disk := session.NewStore(t.TempDir())
			if err = disk.Create("hand", "notes"); err != nil {
				t.Fatal(err)
			}
			for turn := 0; turn < 2; turn++ {
				sess, err := disk.LoadExclusive("hand", "notes")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { sess.Close() })
				state, err := NewStateStore(sess, "examples/"+language)
				if err != nil {
					t.Fatal(err)
				}
				previous, err := state.Get(ctx)
				if err != nil || previous.Revision != turn {
					t.Fatalf("restart state: %+v %v", previous, err)
				}
				if turn == 1 && string(previous.Data) != `{"note":"remember the regression"}` {
					t.Fatalf("persisted note: %s", previous.Data)
				}
				host, err := NewHostFactory(filepath.Join(t.TempDir(), "private"), []LaunchReview{review}, func(context.Context, LaunchReview) error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				questions := 0
				factory := func(op, life context.Context, spec Specification) (*Connection, error) {
					c, err := host(op, life, spec)
					if err != nil {
						return nil, err
					}
					err = c.SetCallbackHandler(func(callbackCtx context.Context, method string, params json.RawMessage) (json.RawMessage, *protocol.Error) {
						if method != "user.question" {
							return state.Handle(callbackCtx, method, params)
						}
						var q protocol.Question
						if err := protocol.DecodePayload(params, &q); err != nil {
							return nil, &protocol.Error{Code: "invalid_question", Message: err.Error()}
						}
						answer := protocol.Answer{ID: q.ID, Text: "remember the regression"}
						if err := q.ValidateAnswer(answer); err != nil {
							return nil, &protocol.Error{Code: "invalid_answer", Message: err.Error()}
						}
						questions++
						raw, _ := json.Marshal(answer)
						return raw, nil
					})
					if err != nil {
						c.Close()
						return nil, err
					}
					return c, nil
				}
				manager, err := NewManager(ctx, factory)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { manager.Close() })
				if _, err = manager.Reload(ctx, []Specification{review.Specification}); err != nil {
					manager.Close()
					t.Fatal(err)
				}
				result, err := manager.ExecuteCommand(ctx, "task-note", "note", "")
				if err != nil {
					manager.Close()
					t.Fatal(err)
				}
				if questions != 1 || len(result.Blocks) != 1 || result.Blocks[0].Text != "Task note saved." {
					t.Fatalf("command result: %+v, questions %d", result, questions)
				}
				items, diagnostics, err := manager.Transform(ctx, []ContextItem{{ID: "protected", Kind: "user", Text: "  retain  "}, {ID: "retrieved", Kind: "retrieval", Text: "  trim  "}})
				if err != nil || len(diagnostics) != 0 || items[0].Text != "  retain  " || items[1].Text != "trim" {
					t.Fatalf("transform: %+v %+v %v", items, diagnostics, err)
				}
				if err = manager.Close(); err != nil {
					t.Fatal(err)
				}
				if err = sess.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

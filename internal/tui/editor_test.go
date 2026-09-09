package tui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEditorCommandQuotingDoesNotEvaluateShell(t *testing.T) {
	args, err := editorArgs(`"/path with spaces/editor" --wait '$(touch bad)' "two words"`)
	if err != nil || !reflect.DeepEqual(args, []string{"/path with spaces/editor", "--wait", "$(touch bad)", "two words"}) {
		t.Fatal(args, err)
	}
	for _, command := range []string{"", `"unfinished`, "editor\\"} {
		if _, err := editorArgs(command); err == nil {
			t.Fatal("invalid command accepted")
		}
	}
}

func TestExternalEditorProcessRoundTripAndCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "editor with spaces")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf 'first line\\n  second line\\n' > \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd, finish, err := prepareEditor("original", `"`+path+`"`)
	if err != nil {
		t.Fatal(err)
	}
	input := cmd.Args[len(cmd.Args)-1]
	info, err := os.Stat(input)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	result := finish(cmd.Run())
	if result.err != nil || result.text != "first line\n  second line\n" {
		t.Fatal(result)
	}
	if _, err := os.Stat(filepath.Dir(input)); !os.IsNotExist(err) {
		t.Fatal("editor directory not cleaned", err)
	}
	m := NewModel(nil, t.TempDir())
	m.textarea.SetValue("original")
	m.editorActive = true
	m.finishExternalEditor(result)
	if m.textarea.Value() != result.text || m.editorActive {
		t.Fatal("editor result not applied")
	}
}

func TestExternalEditorFailuresPreserveDraft(t *testing.T) {
	for _, mode := range []string{"exit", "oversized", "invalid_utf8", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			cmd, finish, err := prepareEditor("original", "sh")
			if err != nil {
				t.Fatal(err)
			}
			path := cmd.Args[len(cmd.Args)-1]
			var runErr error
			switch mode {
			case "exit":
				runErr = errors.New("exit status 1")
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat("x", maxEditorBytes+1)), 0600)
			case "invalid_utf8":
				err = os.WriteFile(path, []byte{255}, 0600)
			case "symlink":
				os.Remove(path)
				err = os.Symlink("/etc/hosts", path)
			}
			if err != nil {
				t.Fatal(err)
			}
			result := finish(runErr)
			if result.err == nil {
				t.Fatal("bad output accepted")
			}
			m := NewModel(nil, t.TempDir())
			m.textarea.SetValue("original")
			m.editorActive = true
			m.finishExternalEditor(result)
			if m.textarea.Value() != "original" || m.editorActive {
				t.Fatal("draft lost")
			}
			if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
				t.Fatal("temporary input leaked")
			}
		})
	}
}

func TestEditorLargeDraftAndBusyAdmission(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	text := strings.Repeat("x", 2048) + "\nsecond line"
	m.textarea.SetValue(text)
	m.editorActive = true
	m.finishExternalEditor(editorResult{text: text})
	if m.textarea.Value() != text {
		t.Fatal("draft silently truncated")
	}
	m.running = true
	if cmd := m.openExternalEditor(); cmd != nil || m.editorActive {
		t.Fatal("editor started during run")
	}
	if m.textarea.Value() != text {
		t.Fatal("busy admission lost draft")
	}
}

func TestEditorRejectsSilentTextareaTruncation(t *testing.T) {
	m := NewModel(nil, t.TempDir())
	m.textarea.SetValue("original")
	m.editorActive = true
	m.finishExternalEditor(editorResult{text: strings.Repeat("x\n", 10001)})
	if m.textarea.Value() != "original" {
		t.Fatal("truncated editor output replaced draft")
	}
}

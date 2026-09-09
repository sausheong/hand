package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

const maxEditorBytes = 64 << 10

type editorResult struct {
	text string
	err  error
}

// editorArgs supports quoted executables and flags without shell evaluation.
func editorArgs(command string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range command {
		if escaped {
			word.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if escaped || quote != 0 {
		return nil, errors.New("editor command has unfinished quoting")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, errors.New("editor command is empty")
	}
	return args, nil
}

func prepareEditor(text, command string) (*exec.Cmd, func(error) editorResult, error) {
	if len(text) > maxEditorBytes || !utf8.ValidString(text) {
		return nil, nil, errors.New("editor input must be UTF-8 within 64 KiB")
	}
	args, err := editorArgs(command)
	if err != nil {
		return nil, nil, err
	}
	executable, err := exec.LookPath(args[0])
	if err != nil {
		return nil, nil, err
	}
	dir, err := os.MkdirTemp("", "hand-editor-*")
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		os.RemoveAll(dir)
		return nil, nil, err
	}
	finish := func(runErr error) editorResult {
		defer os.RemoveAll(dir)
		if runErr != nil {
			return editorResult{err: fmt.Errorf("editor failed: %w", runErr)}
		}
		info, err := os.Lstat(path)
		if err != nil {
			return editorResult{err: err}
		}
		if !info.Mode().IsRegular() || info.Size() > maxEditorBytes {
			return editorResult{err: errors.New("editor output must be a regular file within 64 KiB")}
		}
		f, err := os.Open(path)
		if err != nil {
			return editorResult{err: err}
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxEditorBytes+1))
		if err != nil {
			return editorResult{err: err}
		}
		if len(data) > maxEditorBytes || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
			return editorResult{err: errors.New("editor output must be UTF-8 text within 64 KiB without NUL bytes")}
		}
		return editorResult{text: string(data)}
	}
	return exec.Command(executable, append(args[1:], path)...), finish, nil
}

func (m *Model) openExternalEditor() tea.Cmd {
	if m.editorActive || m.running || m.compacting || m.sessionChanging || m.profileChanging || m.pending != nil {
		m.queueMessage("External editor is available when active work has settled.")
		return nil
	}
	command := os.Getenv("VISUAL")
	if strings.TrimSpace(command) == "" {
		command = os.Getenv("EDITOR")
	}
	if strings.TrimSpace(command) == "" {
		command = "vi"
	}
	process, finish, err := prepareEditor(m.textarea.Value(), command)
	if err != nil {
		m.queueMessage("Cannot open editor: " + err.Error())
		return nil
	}
	m.editorActive = true
	return tea.ExecProcess(process, func(err error) tea.Msg { return finish(err) })
}

func (m *Model) finishExternalEditor(result editorResult) {
	if !m.editorActive {
		return
	}
	m.editorActive = false
	if result.err != nil {
		m.queueMessage(result.err.Error() + "; original input preserved.")
		return
	}
	original := m.textarea.Value()
	m.textarea.SetValue(result.text)
	if m.textarea.Value() != result.text {
		m.textarea.SetValue(original)
		m.queueMessage("Editor output exceeds input line limits or contains unsupported characters; original input preserved.")
		return
	}
	m.textarea.CursorEnd()
	m.refreshViewport()
}

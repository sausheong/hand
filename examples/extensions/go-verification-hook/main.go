// verification-hook checks tracked Git whitespace, not test correctness.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/harness/process"
)

// check inspects both index and working-tree changes. No shell, external diff,
// text conversion, network access or repository hook is invoked.
func check(parent context.Context, workspace string) error {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	for _, cached := range []bool{false, true} {
		args := []string{"--no-pager", "--no-optional-locks", "diff", "--no-ext-diff", "--no-textconv", "--check"}
		if cached {
			args = append(args, "--cached")
		}
		cmd := process.Command(ctx, "git", args...)
		cmd.Dir = workspace
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0"}
		output := process.NewCapture(4096)
		cmd.Stdout = output
		cmd.Stderr = output
		if err := process.Run(cmd); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("Git whitespace check failed; inspect staged and unstaged changes with git diff --check")
		}
	}
	return nil
}
func handle(frame protocol.Frame, workspace string) (any, error) {
	switch frame.Method {
	case "initialize":
		return protocol.Hello{Version: 1, Name: "verification-hook", Capabilities: []string{"commands", "lifecycle"}, Commands: []protocol.Command{{Name: "verify-whitespace", Description: "Check tracked staged and unstaged Git whitespace"}}, Subscriptions: []string{"run.finish"}}, nil
	case "command.execute":
		var input struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := protocol.DecodePayload(frame.Params, &input); err != nil {
			return nil, err
		}
		if input.Name != "verify-whitespace" || input.Arguments != "" {
			return nil, errors.New("verify-whitespace accepts no arguments")
		}
		text := "Tracked Git whitespace checks passed. Tests and untracked files were not checked."
		if err := check(context.Background(), workspace); err != nil {
			text = "Whitespace verification did not pass: " + err.Error()
		}
		return protocol.Presentation{Blocks: []protocol.Block{{Kind: "text", Text: text}}}, nil
	case "lifecycle.notify":
		var input struct {
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if err := protocol.DecodePayload(frame.Params, &input); err != nil {
			return nil, err
		}
		if input.Event != "run.finish" {
			return nil, errors.New("unsupported lifecycle event")
		}
		if err := check(context.Background(), workspace); err != nil {
			return nil, err
		}
		fmt.Fprintln(os.Stderr, "Tracked Git whitespace checks passed; tests and untracked files were not checked.")
		return struct{}{}, nil
	default:
		return nil, errors.New("unsupported method")
	}
}
func run() error {
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	reader, writer := protocol.NewReader(os.Stdin), protocol.NewWriter(os.Stdout)
	for {
		frame, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if frame.Kind != "request" {
			return errors.New("request required")
		}
		value, err := handle(frame, workspace)
		reply := protocol.Frame{Version: 1, Kind: "response", ID: frame.ID}
		if err != nil {
			reply.Error = &protocol.Error{Code: "verification_failed", Message: err.Error()}
		} else {
			reply.Result, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}
		if err = writer.Write(reply); err != nil {
			return err
		}
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

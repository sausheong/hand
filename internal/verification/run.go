// Package verification records command evidence for a precise captured scope.
// Callers must admit commands through policy and exclude concurrent Hand writes.
package verification

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/process"
)

type Options struct {
	Workspace     string
	Profile       string
	ProfileDigest string
	Argv          []string
	Env           map[string]string
	Limits        checkpoints.Limits
	Backend       execution.Backend
	Checkpoints   *checkpoints.Store
	Output        *process.ArtifactStore
}

type View struct {
	Command         []string               `json:"command"`
	Profile         string                 `json:"profile"`
	ProfileDigest   string                 `json:"profile_digest"`
	Workspace       string                 `json:"workspace"`
	Boundary        string                 `json:"boundary"`
	Before          string                 `json:"before"`
	After           string                 `json:"after,omitempty"`
	Omissions       []checkpoints.Omission `json:"omissions"`
	Started         time.Time              `json:"started"`
	Finished        time.Time              `json:"finished"`
	ExitCode        int                    `json:"exit_code"`
	RunError        string                 `json:"run_error,omitempty"`
	SnapshotError   string                 `json:"snapshot_error,omitempty"`
	Stdout          string                 `json:"stdout"`
	Stderr          string                 `json:"stderr"`
	StdoutBytes     int64                  `json:"stdout_bytes"`
	StderrBytes     int64                  `json:"stderr_bytes"`
	StdoutTruncated bool                   `json:"stdout_truncated"`
	StderrTruncated bool                   `json:"stderr_truncated"`
	StdoutArtifact  process.ArtifactInfo   `json:"stdout_artifact"`
	StderrArtifact  process.ArtifactInfo   `json:"stderr_artifact"`
}
type Record struct{ view View }

func (r *Record) View() View {
	v := r.view
	v.Command = append([]string(nil), v.Command...)
	v.Omissions = append([]checkpoints.Omission(nil), v.Omissions...)
	return v
}

// Status describes evidence for the captured scope, never general correctness.
// Failed commands stay failed; a zero exit code cannot override missing or
// changed snapshot evidence. Callers must recapture before asking for status.
func (r *Record) Status(current *checkpoints.Snapshot) string {
	if r.view.RunError != "" || r.view.ExitCode != 0 {
		return "failed"
	}
	if r.view.SnapshotError != "" || r.view.After == "" || r.view.StdoutArtifact.Error != "" || r.view.StderrArtifact.Error != "" {
		return "unverified"
	}
	if current == nil || r.view.Before != r.view.After || current.Digest() != r.view.Before {
		return "stale"
	}
	return "passed"
}
func canonical(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}
func boundBackend(workspace string, b execution.Backend) (execution.Backend, error) {
	// Construct a value copy so the effective directory cannot change while the
	// record continues to claim the original workspace.
	switch v := b.(type) {
	case execution.Host:
		p, err := canonical(v.Workspace)
		if err != nil {
			return nil, err
		}
		if p != workspace {
			return nil, errors.New("verification backend workspace mismatch")
		}
		v.Workspace = p
		return v, nil
	case execution.Container:
		p, err := canonical(v.Workspace)
		if err != nil {
			return nil, err
		}
		if p != workspace {
			return nil, errors.New("verification backend workspace mismatch")
		}
		v.Workspace = p
		return v, nil
	default:
		return nil, errors.New("verification requires a concrete host or container backend")
	}
}

// Run saves the before-image before executing anything. It returns a record
// after command execution even on command failure, preserving exit evidence.
// An admission/snapshot failure before execution returns no record.
func Run(ctx context.Context, o Options) (*Record, error) {
	if o.Checkpoints == nil || o.Output == nil {
		return nil, errors.New("verification requires checkpoint and output stores")
	}
	digest, err := hex.DecodeString(o.ProfileDigest)
	if err != nil || len(digest) != 32 || o.ProfileDigest != strings.ToLower(o.ProfileDigest) || len(o.Profile) > 256 {
		return nil, errors.New("invalid verification profile identity")
	}
	if len(o.Argv) == 0 || len(o.Argv) > 1024 {
		return nil, errors.New("invalid verification command")
	}
	workspace, err := canonical(o.Workspace)
	if err != nil {
		return nil, err
	}
	backend, err := boundBackend(workspace, o.Backend)
	if err != nil {
		return nil, err
	}
	argv := append([]string(nil), o.Argv...)
	env := make(map[string]string, len(o.Env))
	for k, v := range o.Env {
		env[k] = v
	}
	before, err := checkpoints.Capture(ctx, workspace, o.Limits)
	if err != nil {
		return nil, err
	}
	if err = o.Checkpoints.Save(ctx, before); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	v := View{Command: argv, Profile: o.Profile, ProfileDigest: o.ProfileDigest, Workspace: workspace, Boundary: backend.Boundary(), Before: before.Digest(), Omissions: before.Omissions(), Started: time.Now().UTC(), ExitCode: -1}
	result, runErr := backend.Run(ctx, execution.Request{Argv: argv, Env: env, OutputStore: o.Output})
	v.Finished = time.Now().UTC()
	v.ExitCode = result.ExitCode
	v.Stdout = result.Stdout
	v.Stderr = result.Stderr
	v.StdoutBytes = result.StdoutBytes
	v.StderrBytes = result.StderrBytes
	v.StdoutTruncated = result.StdoutTruncated
	v.StderrTruncated = result.StderrTruncated
	v.StdoutArtifact = result.StdoutArtifact
	v.StderrArtifact = result.StderrArtifact
	if runErr != nil {
		v.RunError = runErr.Error()
	}
	// Cancellation must not start additional workspace work. Its command record
	// is retained as failure with the before snapshot and available artifacts.
	if err = ctx.Err(); err != nil {
		v.SnapshotError = err.Error()
		return &Record{v}, runErr
	}
	after, err := checkpoints.Capture(ctx, workspace, o.Limits)
	if err == nil {
		err = o.Checkpoints.Save(ctx, after)
	}
	if err != nil {
		v.SnapshotError = err.Error()
		return &Record{v}, errors.Join(runErr, fmt.Errorf("verification after snapshot: %w", err))
	}
	v.After = after.Digest()
	return &Record{v}, runErr
}

// StatusFor also prevents reusing evidence after a profile/configuration change.
func (r *Record) StatusFor(current *checkpoints.Snapshot, profileDigest string) string {
	status := r.Status(current)
	if status == "passed" && profileDigest != r.view.ProfileDigest {
		return "stale"
	}
	return status
}

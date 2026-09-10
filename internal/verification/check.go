package verification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/harness/process"
)

type Assessment struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Check is the evidence-backed status path. Status/StatusFor only compare
// in-memory metadata and must not qualify a loaded record without this check.
// The caller supplies a freshly captured current workspace and its profile.
func (r *Record) Check(ctx context.Context, current *checkpoints.Snapshot, profileDigest string, store *checkpoints.Store, output *process.ArtifactStore) Assessment {
	unverified := func(err error) Assessment { return Assessment{"unverified", err.Error()} }
	if err := ctx.Err(); err != nil {
		return unverified(err)
	}
	if err := validateView(r.view); err != nil {
		return unverified(err)
	}
	if store == nil || output == nil {
		return unverified(errors.New("verification evidence stores unavailable"))
	}
	before, err := store.Load(ctx, r.view.Before)
	if err != nil {
		return unverified(fmt.Errorf("before snapshot unavailable: %w", err))
	}
	if !slices.Equal(before.Omissions(), r.view.Omissions) {
		return unverified(errors.New("record omission scope differs from snapshot"))
	}
	if r.view.After != "" {
		if _, err = store.Load(ctx, r.view.After); err != nil {
			return unverified(fmt.Errorf("after snapshot unavailable: %w", err))
		}
	}
	if err = checkOutput(ctx, output, r.view.Stdout, r.view.StdoutBytes, r.view.StdoutTruncated, r.view.StdoutArtifact); err != nil {
		return unverified(fmt.Errorf("stdout evidence: %w", err))
	}
	if err = checkOutput(ctx, output, r.view.Stderr, r.view.StderrBytes, r.view.StderrTruncated, r.view.StderrArtifact); err != nil {
		return unverified(fmt.Errorf("stderr evidence: %w", err))
	}
	return Assessment{Status: r.StatusFor(current, profileDigest)}
}
func checkOutput(ctx context.Context, store *process.ArtifactStore, preview string, total int64, truncated bool, artifact process.ArtifactInfo) error {
	if artifact.Error != "" {
		return errors.New(artifact.Error)
	}
	if !truncated {
		if int64(len(preview)) != total {
			return errors.New("inline output byte count mismatch")
		}
		if artifact.Path != "" {
			return errors.New("unexpected artifact for inline output")
		}
		return nil
	}
	if artifact.Path == "" || artifact.Truncated || artifact.Bytes != total {
		return errors.New("full output capture unavailable")
	}
	b, err := store.Read(ctx, artifact)
	if err != nil {
		return err
	}
	if !bytes.HasPrefix(b, []byte(preview)) {
		return errors.New("output preview differs from artifact")
	}
	return nil
}

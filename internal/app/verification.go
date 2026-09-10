package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/verification"
	"github.com/sausheong/harness/execution"
)

type VerificationProfile = config.VerificationProfile

// verificationOptions binds an explicitly selected command to the application's
// configured execution backend. Callers must already hold controller ownership.
func (c *Controller) verificationOptions(profile VerificationProfile, store *checkpoints.Store) (verification.Options, error) {
	boundary, ok := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
	if !ok || boundary.Store != store || c.Processes == nil || boundary.Processes != c.Processes || c.OutputStore == nil {
		return verification.Options{}, errors.New("verification requires checkpoint, process and output ownership")
	}
	if profile.Name == "" || len(profile.Name) > 256 || len(profile.Command) == 0 || len(profile.Command) > 1024 {
		return verification.Options{}, errors.New("invalid verification profile")
	}
	workspace, err := filepath.EvalSymlinks(boundary.Workspace)
	if err != nil {
		return verification.Options{}, err
	}
	processWorkspace, err := filepath.EvalSymlinks(c.Processes.workspace)
	if err != nil || processWorkspace != workspace {
		return verification.Options{}, errors.New("verification process workspace mismatch")
	}
	var backend execution.Backend
	if c.Processes.backend == nil {
		backend = execution.Host{Workspace: workspace}
	} else {
		backend, ok = c.Processes.backend.(execution.Backend)
		if !ok {
			return verification.Options{}, errors.New("configured execution backend cannot verify commands")
		}
	}
	command := append([]string(nil), profile.Command...)
	identity, err := json.Marshal(struct {
		Profile  VerificationProfile
		Boundary string
		Limits   checkpoints.Limits
	}{VerificationProfile{Name: profile.Name, Command: command}, backend.Boundary(), boundary.Limits})
	if err != nil {
		return verification.Options{}, err
	}
	digest := sha256.Sum256(identity)
	return verification.Options{Workspace: workspace, Profile: profile.Name, ProfileDigest: hex.EncodeToString(digest[:]), Argv: command, Limits: boundary.Limits, Backend: backend, Checkpoints: store, Output: c.OutputStore}, nil
}

// RunVerification executes only a command explicitly authorised by the caller.
// It is not exposed as an agent tool or automatic approval bypass. Application
// ownership and background admission remain held until execution and capture end.
func (c *Controller) RunVerification(ctx context.Context, profile VerificationProfile) (record *verification.Record, err error) {
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		options, e := c.verificationOptions(profile, store)
		if e != nil {
			return e
		}
		record, e = verification.Run(ctx, options)
		return e
	})
	return record, err
}

// CheckVerification recaptures current state and revalidates referenced evidence;
// it never treats an old successful exit as proof about later workspace edits.
func (c *Controller) CheckVerification(ctx context.Context, profile VerificationProfile, record *verification.Record) (assessment verification.Assessment, err error) {
	if record == nil {
		return assessment, errors.New("verification record missing")
	}
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		options, e := c.verificationOptions(profile, store)
		if e != nil {
			return e
		}
		if record.View().Workspace != options.Workspace {
			return errors.New("verification record belongs to another workspace")
		}
		current, e := checkpoints.Capture(ctx, options.Workspace, options.Limits)
		if e != nil {
			return e
		}
		assessment = record.Check(ctx, current, options.ProfileDigest, store, options.Output)
		return nil
	})
	return assessment, err
}

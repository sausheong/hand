package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/verification"
)

type VerificationProfileView struct {
	Name     string   `json:"name"`
	Command  []string `json:"command"`
	Digest   string   `json:"digest"`
	Boundary string   `json:"boundary"`
}
type VerificationResult struct {
	ID         string                  `json:"id,omitempty"`
	Record     verification.View       `json:"record"`
	Assessment verification.Assessment `json:"assessment"`
}

func (c *Controller) ConfigureVerification(ctx context.Context, input config.VerificationConfig) error {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg, err := input.ValidatedCopy()
	if err != nil {
		return err
	}
	boundary, ok := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
	if !ok || boundary.Store == nil {
		return errors.New("verification requires configured checkpoints")
	}
	directory, err := filepath.EvalSymlinks(cfg.Directory)
	if err != nil {
		return err
	}
	work, err := filepath.EvalSymlinks(boundary.Workspace)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(work, directory)
	if err != nil {
		return err
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		return errors.New("verification evidence directory must be outside workspace")
	}
	info, err := os.Stat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("verification evidence directory must be private")
	}
	cfg.Directory = directory
	for _, profile := range cfg.Profiles {
		if _, err := c.verificationOptions(profile, boundary.Store); err != nil {
			return err
		}
	}
	c.verificationConfig = &cfg
	return nil
}
func (c *Controller) VerificationProfiles(ctx context.Context) (profiles []VerificationProfileView, err error) {
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		if c.verificationConfig == nil {
			return errors.New("verification profiles not configured")
		}
		for _, profile := range c.verificationConfig.Profiles {
			o, e := c.verificationOptions(profile, store)
			if e != nil {
				return e
			}
			profiles = append(profiles, VerificationProfileView{Name: profile.Name, Command: o.Argv, Digest: o.ProfileDigest, Boundary: o.Backend.Boundary()})
		}
		return nil
	})
	return profiles, err
}
func (c *Controller) namedVerification(name string, store *checkpoints.Store) (verification.Options, error) {
	if c.verificationConfig != nil {
		for _, p := range c.verificationConfig.Profiles {
			if p.Name == name {
				return c.verificationOptions(p, store)
			}
		}
	}
	return verification.Options{}, errors.New("verification profile not found")
}

// RunNamedVerification requires the digest of the exact profile confirmed by
// the user. All execution and persistence remain within application ownership.
func (c *Controller) RunNamedVerification(ctx context.Context, name, digest string) (result VerificationResult, err error) {
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		o, e := c.namedVerification(name, store)
		if e != nil {
			return e
		}
		if digest == "" || digest != o.ProfileDigest {
			return errors.New("verification profile changed since confirmation")
		}
		record, e := verification.Run(ctx, o)
		if record == nil {
			return e
		}
		result.Record = record.View()
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		id, saveErr := record.SaveWithLimits(persist, c.verificationConfig.Directory, verification.RetentionLimits{MaxRecords: c.verificationConfig.MaxRecords, MaxBytes: c.verificationConfig.MaxBytes})
		result.ID = id
		current, captureErr := checkpoints.Capture(ctx, o.Workspace, o.Limits)
		if captureErr != nil {
			result.Assessment = verification.Assessment{Status: "unverified", Reason: captureErr.Error()}
		} else {
			result.Assessment = record.Check(ctx, current, o.ProfileDigest, store, o.Output)
		}
		if saveErr != nil {
			result.Assessment = verification.Assessment{Status: "unverified", Reason: "verification evidence persistence failed"}
		}
		var contextErr error
		if saveErr == nil {
			contextErr = c.retainVerificationContext(persist, result)
		}
		return errors.Join(e, saveErr, contextErr)
	})
	return result, err
}
func (c *Controller) CheckSavedVerification(ctx context.Context, name, id string) (result VerificationResult, err error) {
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		o, e := c.namedVerification(name, store)
		if e != nil {
			return e
		}
		record, e := verification.Load(ctx, c.verificationConfig.Directory, id)
		if e != nil {
			return e
		}
		if record.View().Workspace != o.Workspace {
			return errors.New("verification record belongs to another workspace")
		}
		current, e := checkpoints.Capture(ctx, o.Workspace, o.Limits)
		if e != nil {
			return e
		}
		result = VerificationResult{ID: id, Record: record.View(), Assessment: record.Check(ctx, current, o.ProfileDigest, store, o.Output)}
		return c.retainVerificationContext(ctx, result)
	})
	return result, err
}

func (c *Controller) ListVerificationEvidence(ctx context.Context, offset int) (page verification.RecordPage, err error) {
	err = c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		if c.verificationConfig == nil {
			return errors.New("verification profiles not configured")
		}
		var e error
		page, e = verification.List(ctx, c.verificationConfig.Directory, offset)
		return e
	})
	return page, err
}

// DeleteVerificationEvidence is an explicitly selected record deletion. Callers
// obtain user confirmation; references and historical ledger responses survive.
func (c *Controller) DeleteVerificationEvidence(ctx context.Context, id string) error {
	return c.withCheckpointRecovery(ctx, func(ctx context.Context, store *checkpoints.Store) error {
		if c.verificationConfig == nil {
			return errors.New("verification profiles not configured")
		}
		boundary := c.owner().options.RunBoundary.(*WorkspaceCheckpoints)
		return verification.Delete(ctx, c.verificationConfig.Directory, id, boundary.Workspace)
	})
}

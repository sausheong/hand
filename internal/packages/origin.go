package packages

import (
	"context"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Origin is importer-observed provenance, separate from the package content pin.
// It is bound into change approval and retained even after staging is removed.
type Origin struct {
	Kind      string `json:"kind"`
	Location  string `json:"location"`
	Reference string `json:"reference,omitempty"`
}

func (o Origin) Validate() error {
	if !filepath.IsAbs(o.Location) || len(o.Location) > 2048 || !utf8.ValidString(o.Location) || strings.ContainsRune(o.Location, 0) {
		return errors.New("invalid package origin location")
	}
	switch o.Kind {
	case "local":
		if o.Reference != "" {
			return errors.New("local origin cannot have a revision")
		}
	case "archive":
		if !validDigest(o.Reference) {
			return errors.New("archive origin requires SHA-256")
		}
	case "git":
		hash, err := hex.DecodeString(o.Reference)
		if err != nil || len(hash) != 20 || strings.ToLower(o.Reference) != o.Reference {
			return errors.New("Git origin requires full SHA-1 commit")
		}
	default:
		return errors.New("unknown package origin")
	}
	return nil
}
func (s *Snapshot) Origin() Origin { return s.origin }

// PrepareSnapshot imports provenance from an owned verified snapshot. Keeping
// snapshot ownership during inspection prevents Close from deleting its source.
func (s *Store) PrepareSnapshot(ctx context.Context, snapshot *Snapshot) (ChangeReview, error) {
	if snapshot == nil {
		return ChangeReview{}, errors.New("package snapshot required")
	}
	snapshot.mu.Lock()
	defer snapshot.mu.Unlock()
	if snapshot.closed {
		return ChangeReview{}, errors.New("package snapshot closed")
	}
	if err := snapshot.origin.Validate(); err != nil {
		return ChangeReview{}, err
	}
	review, err := s.PrepareInstall(ctx, snapshot.directory, snapshot.digest)
	if err != nil {
		return ChangeReview{}, err
	}
	origin := snapshot.origin
	review.Origin = &origin
	if origin.Kind != "local" {
		review.Source = origin.Location
	}
	return review, nil
}

package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/hand/internal/extensions"
)

type ExtensionReviewInput struct {
	Container    *extensions.ContainerLaunch `json:"container,omitempty"`
	Version      int                         `json:"version"`
	SnapshotRoot string                      `json:"snapshot_root"`
	Extensions   []struct {
		Identity string                  `json:"identity"`
		Launch   extensions.LaunchConfig `json:"launch"`
	} `json:"extensions"`
}

// WriteExtensionReview fingerprints explicit files without starting any process.
// Output is exclusively created; existing reviews are never overwritten.
func WriteExtensionReview(ctx context.Context, input, output string) (string, error) {
	f, err := os.OpenFile(input, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > protocol.MaxFrameBytes {
		return "", errors.New("review input must be a regular file at most 256 KiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, protocol.MaxFrameBytes+1))
	if err != nil {
		return "", err
	}
	var requested ExtensionReviewInput
	if err = protocol.DecodePayload(raw, &requested); err != nil {
		return "", err
	}
	if requested.Version != 1 || len(requested.Extensions) > 16 || !filepath.IsAbs(requested.SnapshotRoot) {
		return "", errors.New("invalid extension review input")
	}
	reviewed := ExtensionStartup{Version: 1, SnapshotRoot: requested.SnapshotRoot, Identities: map[string]string{}}
	for _, entry := range requested.Extensions {
		if !utf8.ValidString(entry.Identity) || strings.TrimSpace(entry.Identity) == "" || len(entry.Identity) > 256 || strings.ContainsRune(entry.Identity, 0) {
			return "", errors.New("invalid package identity")
		}
		var review extensions.LaunchReview
		if requested.Container != nil {
			review, err = extensions.ReviewContainerLaunch(ctx, entry.Launch, *requested.Container)
		} else {
			review, err = extensions.ReviewHostLaunch(ctx, entry.Launch)
		}
		if err != nil {
			return "", err
		}
		if _, exists := reviewed.Identities[review.Specification.Name]; exists {
			return "", errors.New("duplicate extension name")
		}
		reviewed.Identities[review.Specification.Name] = entry.Identity
		reviewed.Reviews = append(reviewed.Reviews, review)
	}
	raw, err = json.MarshalIndent(reviewed, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	if len(raw) > protocol.MaxFrameBytes {
		return "", errors.New("review output exceeds 256 KiB")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	target, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := target.Write(raw)
	if writeErr == nil {
		writeErr = target.Sync()
	}
	err = errors.Join(writeErr, target.Close())
	if err != nil {
		os.Remove(output)
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

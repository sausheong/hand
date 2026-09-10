package agentio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// AttachmentPolicy holds invocation-local snapshots of exact user-selected
// paths. It is independent of tool permissions and never grants directory access.
type AttachmentPolicy struct{ files map[string][]byte }
type attachmentPolicyKey struct{}

func WithAttachmentPolicy(ctx context.Context, policy *AttachmentPolicy) context.Context {
	return context.WithValue(ctx, attachmentPolicyKey{}, policy)
}
func AttachmentPolicyFromContext(ctx context.Context) *AttachmentPolicy {
	p, _ := ctx.Value(attachmentPolicyKey{}).(*AttachmentPolicy)
	return p
}

// NewAttachmentPolicy snapshots explicitly granted files before model execution.
// Nonblocking opens prevent a replaced FIFO from hanging admission. The opened
// descriptor is checked before reading and every read is bounded.
func NewAttachmentPolicy(ctx context.Context, paths []string) (*AttachmentPolicy, error) {
	p := &AttachmentPolicy{files: make(map[string][]byte)}
	total := 0
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if path == "" {
			return nil, errors.New("attachment grant requires an exact file path")
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if _, ok := p.files[absolute]; ok {
			continue
		}
		if len(p.files) >= 16 {
			return nil, errors.New("attachment grant limit is 16 files")
		}
		f, err := os.OpenFile(absolute, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			return nil, fmt.Errorf("grant attachment %q: %w", path, err)
		}
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxImageFileSize {
			f.Close()
			return nil, fmt.Errorf("attachment %q must be a regular file within 5 MiB", path)
		}
		data, readErr := io.ReadAll(io.LimitReader(f, maxImageFileSize+1))
		err = errors.Join(readErr, f.Close())
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(data) > maxImageFileSize || total+len(data) > maxPromptImageBytes {
			return nil, errors.New("attachment grants exceed the file or 20 MiB total limit")
		}
		total += len(data)
		p.files[absolute] = data
	}
	return p, nil
}

func grantedAttachment(ctx context.Context, absolute string, explicit bool) ([]byte, bool) {
	p := AttachmentPolicyFromContext(ctx)
	if p == nil || !explicit {
		return nil, false
	}
	data, ok := p.files[filepath.Clean(absolute)]
	return append([]byte(nil), data...), ok
}

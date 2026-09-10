package agentio

import (
	"context"
	"encoding/hex"
	"errors"
	"unicode/utf8"

	"github.com/sausheong/hand/internal/packages"
)

type packagePromptKey struct{}

// WithPackagePrompt carries an explicitly selected, already verified package
// prompt as user-level instructions. Its text is never parsed for attachments,
// slash commands, shell expansion or template evaluation.
func WithPackagePrompt(ctx context.Context, resource packages.TextResource) (context.Context, error) {
	digest, err := hex.DecodeString(resource.PackageDigest)
	if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != resource.PackageDigest || resource.Kind != "prompt" || resource.Package == "" || resource.Path == "" || len(resource.Text) > packages.MaxTextResourceBytes || !utf8.ValidString(resource.Text) {
		return nil, errors.New("valid verified package prompt required")
	}
	return context.WithValue(ctx, packagePromptKey{}, resource), nil
}

package agentio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sausheong/harness/llm"
)

const maxPromptImageBytes = 20 << 20
const maxPromptImages = 16

type imageToken struct {
	start, end int
	path       string
	explicit   bool
}

// imageTokens recognises quoted paths and backslash-escaped spaces while
// preserving the original text positions. Quotes only begin at token start, so
// apostrophes inside prose words are not interpreted as path syntax.
func imageTokens(text string) ([]imageToken, error) { return scanInputTokens(text, false) }

func scanInputTokens(text string, incomplete bool) ([]imageToken, error) {
	var out []imageToken
	for i := 0; i < len(text); {
		if text[i] == ' ' || text[i] == '\n' || text[i] == '\t' || text[i] == '\r' {
			i++
			continue
		}
		start := i
		var value strings.Builder
		var quote byte
		if text[i] == '@' {
			i++
		}
		if i < len(text) && (text[i] == '"' || text[i] == '\'') {
			quote = text[i]
			i++
		}
		closed := quote == 0
		for i < len(text) {
			c := text[i]
			if quote != 0 && c == quote {
				i++
				closed = true
				break
			}
			if quote == 0 && unicode.IsSpace(rune(c)) {
				break
			}
			if c == '\\' && i+1 < len(text) && (text[i+1] == ' ' || text[i+1] == '"' || text[i+1] == '\'' || text[i+1] == '\\') {
				i++
				c = text[i]
			}
			value.WriteByte(c)
			i++
		}
		if !closed && !incomplete {
			// Only report unfinished quoting when it describes an image reference.
			if _, ok := imageExts[strings.ToLower(filepath.Ext(value.String()))]; ok || text[start] == '@' {
				return nil, fmt.Errorf("unfinished quoted image path at byte %d", start)
			}
		}
		out = append(out, imageToken{start: start, end: i, path: value.String(), explicit: text[start] == '@'})
	}
	return out, nil
}

// ParseImageInput loads explicit image-path tokens atomically: on any failure,
// no image set is returned and the caller must retain its draft. External paths
// require an exact invocation grant and an explicit @ reference.
func ParseImageInput(ctx context.Context, workspace, text string) (string, []llm.ImageContent, error) {
	tokens, err := imageTokens(text)
	if err != nil {
		return text, nil, err
	}
	hasImage := false
	for _, token := range tokens {
		if _, ok := imageExts[strings.ToLower(filepath.Ext(token.path))]; ok {
			hasImage = true
			break
		}
	}
	if !hasImage {
		return text, nil, ctx.Err()
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return text, nil, err
	}
	defer root.Close()
	base, err := filepath.Abs(workspace)
	if err != nil {
		return text, nil, err
	}
	var images []llm.ImageContent
	var output strings.Builder
	offset, total := 0, 0
	seen := make(map[string]string)
	for _, token := range tokens {
		mime, ok := imageExts[strings.ToLower(filepath.Ext(token.path))]
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return text, nil, err
		}
		resolved := resolvePath(base, token.path)
		granted, allowed := grantedAttachment(ctx, resolved, token.explicit)
		relative, err := filepath.Rel(base, resolved)
		if !allowed && (err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return text, nil, fmt.Errorf("image %q is outside the workspace; explicit attachment permission is required", token.path)
		}
		placeholder, exists := seen[relative]
		if !exists {
			if len(images) >= maxPromptImages {
				return text, nil, fmt.Errorf("image attachment limit is %d", maxPromptImages)
			}
			data := granted
			if !allowed {
				before, statErr := root.Stat(relative)
				if statErr != nil {
					return text, nil, fmt.Errorf("attach image %q: %w", token.path, statErr)
				}
				if !before.Mode().IsRegular() {
					return text, nil, fmt.Errorf("image %q is not a regular file", token.path)
				}
				f, err := root.Open(relative)
				if err != nil {
					return text, nil, fmt.Errorf("attach image %q: %w", token.path, err)
				}
				info, err := f.Stat()
				if err != nil || !info.Mode().IsRegular() || info.Size() > maxImageFileSize {
					f.Close()
					return text, nil, fmt.Errorf("image %q must be a regular file within 5 MiB", token.path)
				}
				var readErr error
				data, readErr = io.ReadAll(io.LimitReader(f, maxImageFileSize+1))
				closeErr := f.Close()
				if readErr != nil || closeErr != nil {
					return text, nil, fmt.Errorf("read image %q: %v", token.path, errors.Join(readErr, closeErr))
				}
			}
			if err := ctx.Err(); err != nil {
				return text, nil, err
			}
			if len(data) > maxImageFileSize || total+len(data) > maxPromptImageBytes {
				return text, nil, fmt.Errorf("image attachments exceed the file or 20 MiB prompt limit")
			}
			total += len(data)
			images = append(images, llm.ImageContent{MimeType: mime, Data: data})
			placeholder = "[image: " + filepath.Base(resolved) + "]"
			seen[relative] = placeholder
		}
		output.WriteString(text[offset:token.start])
		output.WriteString(placeholder)
		offset = token.end
	}
	output.WriteString(text[offset:])
	return output.String(), images, nil
}

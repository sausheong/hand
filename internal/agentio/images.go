package agentio

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/tool"
)

// imageExts maps a recognized file extension to its MIME type.
var imageExts = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// maxImageFileSize matches harness's own tools/file read-tool limit
// (tools/file/limits.go's unexported maxImageFileSize) so hand doesn't
// accept an image the provider's own file-read tool would reject.
const maxImageFileSize = 5 * 1024 * 1024

// tokenPattern matches whitespace-delimited tokens for ExtractImagePaths.
// Using regexp.ReplaceAllStringFunc (rather than iterating tokens and
// calling strings.Replace on the running output) is deliberate: Replace
// re-scans the *already-substituted* string for the next match, so a
// token that's a substring of an earlier placeholder (or of another
// token) gets replaced at the wrong position. ReplaceAllStringFunc walks
// the original string's real match positions exactly once, so this
// class of bug can't happen here.
var tokenPattern = regexp.MustCompile(`\S+`)

// ExtractImagePaths scans text for whitespace-delimited tokens that
// resolve (via resolvePath, the same expand-home-then-join-if-relative
// helper every tool in this package uses) to an existing file, inside
// workspace, with a recognized image extension, no larger than
// maxImageFileSize. Each match is read into an llm.ImageContent and the
// token is replaced in the returned text with a short "[image:
// name.ext]" placeholder — so the remaining text still reads naturally
// and the transcript shows what was attached. The same resolved path
// referenced more than once in one message is only read and attached
// once; later references still get the placeholder. Tokens that aren't
// in-workspace existing image files pass through untouched; a message
// with no image references is returned unchanged with a nil slice.
//
// A path outside workspace is never attached, full stop, regardless of
// extension — enforced by tool.ValidatePathInWorkDir, the same real
// boundary check SearchTool uses. Without it this function would read
// any absolute path on the filesystem ending in an image extension
// (e.g. /etc/foo.png, a file under ~/.ssh/) and upload its bytes to a
// third-party LLM API.
func ExtractImagePaths(workspace, text string) (string, []llm.ImageContent) {
	var images []llm.ImageContent
	placeholders := make(map[string]string) // resolved path -> placeholder, for dedup

	replaced := tokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		ext := strings.ToLower(filepath.Ext(token))
		mimeType, ok := imageExts[ext]
		if !ok {
			return token
		}

		resolved := resolvePath(workspace, token)
		if err := tool.ValidatePathInWorkDir(resolved, workspace); err != nil {
			return token
		}

		if placeholder, seen := placeholders[resolved]; seen {
			return placeholder
		}

		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxImageFileSize {
			return token
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return token
		}

		images = append(images, llm.ImageContent{MimeType: mimeType, Data: data})
		placeholder := "[image: " + filepath.Base(resolved) + "]"
		placeholders[resolved] = placeholder
		return placeholder
	})

	return replaced, images
}

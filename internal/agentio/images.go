package agentio

import (
	"os"
	"path/filepath"
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

// ExtractImagePaths scans text for whitespace-delimited tokens that
// resolve (via tool.ExpandHome + join against workspace, same as every
// other tool's path handling) to an existing file, inside workspace,
// with a recognized image extension. Each match is read into an
// llm.ImageContent and the token is replaced in the returned text with a
// short "[image: name.ext]" placeholder — so the remaining text still
// reads naturally and the transcript shows what was attached. Tokens
// that aren't in-workspace existing image files pass through untouched;
// a message with no image references is returned unchanged with a nil
// slice.
//
// A path outside workspace is never attached, full stop, regardless of
// extension — enforced by tool.ValidatePathInWorkDir, the same real
// boundary check SearchTool uses. Without it this function would read
// any absolute path on the filesystem ending in an image extension
// (e.g. /etc/foo.png, a file under ~/.ssh/) and upload its bytes to a
// third-party LLM API.
func ExtractImagePaths(workspace, text string) (string, []llm.ImageContent) {
	tokens := strings.Fields(text)
	var images []llm.ImageContent
	replaced := text

	for _, token := range tokens {
		ext := strings.ToLower(filepath.Ext(token))
		mimeType, ok := imageExts[ext]
		if !ok {
			continue
		}

		resolved := tool.ExpandHome(token)
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(workspace, resolved)
		}
		if err := tool.ValidatePathInWorkDir(resolved, workspace); err != nil {
			continue
		}

		data, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}

		images = append(images, llm.ImageContent{MimeType: mimeType, Data: data})
		placeholder := "[image: " + filepath.Base(resolved) + "]"
		replaced = strings.Replace(replaced, token, placeholder, 1)
	}

	return replaced, images
}

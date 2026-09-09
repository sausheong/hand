package agentio

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

type ReferenceCompletion struct {
	Start, End int
	Candidates []string // complete @ tokens, quoted when needed
}

// CompleteReference reads one directory, bounded to 4096 entries and 64 matches.
// Byte cursor positions must be UTF-8 boundaries. No file contents are read.
func CompleteReference(ctx context.Context, workspace, text string, cursor int) (ReferenceCompletion, error) {
	var result ReferenceCompletion
	if cursor < 0 || cursor > len(text) || !utf8.ValidString(text[:cursor]) {
		return result, fmt.Errorf("invalid completion cursor")
	}
	tokens, err := scanInputTokens(text[:cursor], true)
	if err != nil || len(tokens) == 0 {
		return result, err
	}
	token := tokens[len(tokens)-1]
	if !token.explicit || token.end != cursor {
		return result, nil
	}
	result.Start, result.End = token.start, cursor
	all, _ := scanInputTokens(text, true)
	for _, full := range all {
		if full.start == token.start {
			result.End = full.end
			break
		}
	}
	prefix := token.path
	if filepath.IsAbs(prefix) || strings.HasPrefix(prefix, "~") {
		return result, fmt.Errorf("complete a workspace-relative path")
	}
	parent, leaf := filepath.Split(prefix)
	clean := filepath.Clean(parent)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return result, fmt.Errorf("completion cannot leave the workspace")
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return result, err
	}
	defer root.Close()
	dir, err := root.Open(clean)
	if err != nil {
		return result, err
	}
	defer dir.Close()
	scanned := 0
	for {
		if err := ctx.Err(); err != nil {
			return ReferenceCompletion{}, err
		}
		entries, readErr := dir.ReadDir(256)
		scanned += len(entries)
		if scanned > 4096 {
			return ReferenceCompletion{}, fmt.Errorf("directory exceeds completion scan limit (4096 entries)")
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), leaf) {
				continue
			}
			path := filepath.Join(clean, entry.Name())
			info, err := root.Stat(path)
			if err != nil {
				continue
			} // escaping/broken symlinks are not suggestions
			if !info.IsDir() && !info.Mode().IsRegular() {
				continue
			}
			candidate := filepath.ToSlash(filepath.Join(parent, entry.Name()))
			if info.IsDir() {
				candidate += "/"
			}
			if strings.ContainsAny(candidate, " \t\n\r\"'\\") {
				candidate = strings.ReplaceAll(strings.ReplaceAll(candidate, "\\", "\\\\"), "\"", "\\\"")
				candidate = "@\"" + candidate
				if !info.IsDir() {
					candidate += "\""
				}
			} else {
				candidate = "@" + candidate
			}
			result.Candidates = append(result.Candidates, candidate)
			if len(result.Candidates) > 64 {
				return ReferenceCompletion{}, fmt.Errorf("more than 64 matches; type a longer path prefix")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return ReferenceCompletion{}, readErr
		}
	}
	sort.Strings(result.Candidates)
	return result, nil
}

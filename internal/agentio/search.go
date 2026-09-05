package agentio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sausheong/harness/tool"
)

// searchSkipDirs are pruned from the walk entirely — VCS metadata and
// dependency trees that are large, rarely what a search is looking for,
// and expensive to walk for no benefit.
var searchSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	".hand":        true,
}

// maxSearchContentFileSize bounds which files content search will open
// and scan — an unbounded read would happily try to load a multi-GB log
// file sitting in the workspace into memory.
const maxSearchContentFileSize = 64 * 1024

// defaultMaxSearchResults is used when the caller doesn't set
// max_results.
const defaultMaxSearchResults = 100

// binarySniffLen is how many leading bytes are checked for a NUL byte to
// decide a file looks binary — the same heuristic file/grep use.
const binarySniffLen = 512

// SearchTool finds files by name glob and/or content regex under
// WorkDir. Read-only by construction — never registered in gatedTools.
type SearchTool struct {
	WorkDir string
}

type searchInput struct {
	Path       string `json:"path"`
	NameGlob   string `json:"name_glob"`
	Content    string `json:"content"`
	MaxResults int    `json:"max_results"`
}

func (t *SearchTool) Name() string { return "search" }

func (t *SearchTool) Description() string {
	return "Find files by name glob and/or search their contents by regex, without an approval prompt. Provide name_glob (e.g. \"*.go\"), content (a regex to grep for), or both. Results are capped at max_results (default 100)."
}

func (t *SearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Directory to search under, relative to the workspace (default \".\")"
			},
			"name_glob": {
				"type": "string",
				"description": "Filename glob to match against each file's base name, e.g. \"*.go\" (no ** or multi-segment globs)"
			},
			"content": {
				"type": "string",
				"description": "Regex to grep file contents for; matches are reported as path:line:text"
			},
			"max_results": {
				"type": "integer",
				"description": "Maximum number of results to return across all files (default 100)"
			}
		}
	}`)
}

// IsConcurrencySafe returns true — search is a pure read.
func (t *SearchTool) IsConcurrencySafe(_ json.RawMessage) bool { return true }

func (t *SearchTool) Execute(ctx context.Context, input json.RawMessage) (tool.ToolResult, error) {
	var in searchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.ToolResult{Error: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	if in.NameGlob == "" && in.Content == "" {
		return tool.ToolResult{Error: "at least one of name_glob or content is required"}, nil
	}

	maxResults := in.MaxResults
	if maxResults <= 0 {
		maxResults = defaultMaxSearchResults
	}

	path := in.Path
	if path == "" {
		path = "."
	}
	path = resolvePath(t.WorkDir, path)
	if err := tool.ValidatePathInWorkDir(path, t.WorkDir); err != nil {
		return tool.ToolResult{Error: err.Error()}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return tool.ToolResult{Error: fmt.Sprintf("path %q: %v", in.Path, err)}, nil
	}
	if !info.IsDir() {
		return tool.ToolResult{Error: fmt.Sprintf("path %q is not a directory", in.Path)}, nil
	}

	var re *regexp.Regexp
	if in.Content != "" {
		var err error
		re, err = regexp.Compile(in.Content)
		if err != nil {
			return tool.ToolResult{Error: fmt.Sprintf("invalid content regex: %v", err)}, nil
		}
	}

	// name_glob and content combine as AND when both are set: a file must
	// match the glob before its contents are grepped.
	var results []string
	walkErr := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if searchSkipDirs[d.Name()] && p != path {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		if in.NameGlob != "" {
			matched, err := filepath.Match(in.NameGlob, d.Name())
			if err != nil || !matched {
				return nil
			}
		}

		switch {
		case re != nil:
			info, err := d.Info()
			if err != nil || info.Size() > maxSearchContentFileSize {
				return nil
			}
			matches, err := grepFile(p, re, maxResults-len(results))
			if err != nil {
				return nil
			}
			results = append(results, matches...)
		default:
			results = append(results, p)
		}

		if len(results) >= maxResults {
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil && walkErr != fs.SkipAll && walkErr != ctx.Err() {
		return tool.ToolResult{Error: walkErr.Error()}, nil
	}

	if len(results) == 0 {
		return tool.ToolResult{Output: "no matches"}, nil
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return tool.ToolResult{Output: strings.Join(results, "\n")}, nil
}

// grepFile scans path line by line for re, returning up to limit
// "path:lineNumber:line" matches. A file that looks binary (a NUL byte
// in the first binarySniffLen bytes) is skipped entirely.
func grepFile(path string, re *regexp.Regexp, limit int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sniff := make([]byte, binarySniffLen)
	n, _ := f.Read(sniff)
	if bytes.IndexByte(sniff[:n], 0) != -1 {
		return nil, nil
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	var matches []string
	lineNum := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%s:%d:%s", path, lineNum, line))
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches, scanner.Err()
}

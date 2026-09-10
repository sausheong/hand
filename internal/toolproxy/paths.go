package toolproxy

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/sausheong/hand/internal/permissions"
)

// mapInput maps workspace file identities only, not arbitrary strings embedded
// in commands or file content. Additional external mounts require explicit maps.
func mapInput(workspace, name string, input json.RawMessage) (json.RawMessage, error) {
	switch name {
	case "read_file", "write_file", "edit_file", "search":
	default:
		return append(json.RawMessage(nil), input...), nil
	}
	if workspace == "" {
		return nil, errors.New("tool proxy workspace is not configured")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return nil, errors.New("tool input must be an object")
	}
	var path string
	if raw, ok := fields["path"]; ok {
		if err := json.Unmarshal(raw, &path); err != nil {
			return nil, errors.New("tool path must be a string")
		}
	}
	if path == "" {
		if name != "search" {
			return nil, errors.New("tool path is required")
		}
		path = "."
	}
	root, err := permissions.CanonicalResource(workspace, ".")
	if err != nil {
		return nil, err
	}
	target, err := permissions.CanonicalResource(root, path)
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, errors.New("tool path is outside the mounted workspace")
	}
	fields["path"], err = json.Marshal("/workspace/" + filepath.ToSlash(relative))
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

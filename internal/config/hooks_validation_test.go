package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHookPoliciesValidatedOnLoad(t *testing.T) {
	for _, input := range []string{
		`{"hooks":[{"event":"Stop","command":"check","failure_policy":"allow"}]}`,
		`{"hooks":[{"event":"PostToolUse","command":"check","failure_policy":"deny"}]}`,
		`{"hooks":[{"event":"Stop","command":"check","timeout_seconds":-1}]}`,
		`{"hooks":[{"event":"Unknown","command":"check"}]}`,
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("invalid hook accepted: %s", input)
		}
	}
}

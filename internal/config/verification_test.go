package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerificationConfigLiteralArgvAndRelativeEvidence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "verification.json")
	data := `{"directory":"evidence","profiles":[{"name":"tests","command":["go","test","./...","$(touch must-not-exist)",""]}]}`
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := ReadVerification(file)
	if err != nil {
		t.Fatal(err)
	}
	if c.Directory != filepath.Join(dir, "evidence") || c.Profiles[0].Command[3] != "$(touch must-not-exist)" || c.Profiles[0].Command[4] != "" {
		t.Fatal(c)
	}
	if _, err := os.Stat(c.Directory); !os.IsNotExist(err) {
		t.Fatal("loading created evidence directory")
	}
	copy, err := c.ValidatedCopy()
	if err != nil {
		t.Fatal(err)
	}
	c.Profiles[0].Command[0] = "changed"
	if copy.Profiles[0].Command[0] != "go" {
		t.Fatal("configuration slice alias")
	}
}
func TestVerificationConfigRejectsAmbiguousOrUnboundedInput(t *testing.T) {
	for _, raw := range []string{
		`{"directory":"out","profiles":[{"name":"test","command":["go"]}],"unknown":true}`,
		`{"directory":"out","profiles":[{"name":"test","command":["go"]}]} {}`,
		`{"directory":"out","profiles":[{"name":"test","command":["go"]},{"name":"test","command":["sh"]}]}`,
		`{"directory":"out","profiles":[{"name":"two words","command":["go"]}]}`,
		`{"directory":"out","profiles":[{"name":"test","command":[""]}]}`,
		strings.Repeat(" ", maxVerificationConfigBytes+1),
	} {
		file := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadVerification(file); err == nil {
			t.Fatal("invalid config accepted", raw[:min(100, len(raw))])
		}
	}
}

func TestVerificationRetentionConfiguration(t *testing.T) {
	base := VerificationConfig{Directory: t.TempDir(), Profiles: []VerificationProfile{{Name: "unit", Command: []string{"go", "test"}}}}
	defaults, err := base.ValidatedCopy()
	if err != nil || defaults.MaxRecords != 1000 || defaults.MaxBytes != 256<<20 {
		t.Fatal(defaults, err)
	}
	base.MaxRecords = 2
	base.MaxBytes = 4096
	selected, err := base.ValidatedCopy()
	if err != nil || selected.MaxRecords != 2 || selected.MaxBytes != 4096 {
		t.Fatal(selected, err)
	}
	base.MaxRecords = -1
	if _, err := base.ValidatedCopy(); err == nil {
		t.Fatal("negative record limit accepted")
	}
	base.MaxRecords = 2
	base.MaxBytes = 9 << 30
	if _, err := base.ValidatedCopy(); err == nil {
		t.Fatal("excessive byte limit accepted")
	}
}

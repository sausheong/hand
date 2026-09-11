package agentio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillReaderBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SKILL.md")
	body := "---\ndescription: compact metadata\n---\n"
	// Test the exact cap independently of header length.
	body += strings.Repeat("x", skillFileLimit-len(body))
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(path)
	metadata, err := readSkill(path, info, false)
	if err != nil || len(metadata) > skillMetadataLimit {
		t.Fatal(err, len(metadata))
	}
	full, err := readSkill(path, info, true)
	if err != nil || string(full) != body {
		t.Fatal("exact limit rejected", err)
	}
	// Project instructions must retain their independent smaller limit.
	if _, err := readInstruction(path, info); err == nil {
		t.Fatal("project instruction cap weakened")
	}
	link := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readSkill(link, info, true); err == nil {
		t.Fatal("symlink accepted")
	}
	other := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(other, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSkill(other, info, true); err == nil {
		t.Fatal("replacement accepted")
	}
}

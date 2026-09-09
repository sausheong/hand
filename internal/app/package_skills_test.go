package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/packages"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

type authoredSkillFixture struct{}

func (authoredSkillFixture) Get(name string) (string, bool) {
	return "authored body", name == "authored"
}
func (authoredSkillFixture) FormatIndex() string { return "authored skill index" }

type backendSkillLoader struct {
	tool.Tool
	calls []string
}

func (b *backendSkillLoader) IsConcurrencySafe(json.RawMessage) bool { return false }
func (b *backendSkillLoader) Execute(_ context.Context, raw json.RawMessage) (tool.ToolResult, error) {
	var request struct{ Name string }
	if err := json.Unmarshal(raw, &request); err != nil {
		return tool.ToolResult{}, err
	}
	b.calls = append(b.calls, request.Name)
	return tool.ToolResult{Output: "backend skill: " + request.Name}, nil
}

func TestSelectInstalledPackageSkillsOwnedSnapshotAndRemoval(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	body := []byte("Run the project's verification commands. Read refs/checklist.md relative to this skill first.")
	sum := sha256.Sum256(body)
	m := packages.Manifest{Schema: 1, Name: "skill-package", Version: "1.0.0", Compatibility: packages.Compatibility{MinimumHand: "1.0.0", ExtensionProtocol: 1}, Files: []packages.File{{Path: "SKILL.md", Kind: "skill", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])}}}
	asset := []byte("Check the current diff before editing.")
	assetSum := sha256.Sum256(asset)
	m.Files = append(m.Files, packages.File{Path: "refs/checklist.md", Kind: "asset", Size: int64(len(asset)), SHA256: hex.EncodeToString(assetSum[:])})
	if err := os.Mkdir(filepath.Join(source, "refs"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "refs/checklist.md"), asset, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string][]byte{"SKILL.md": body, packages.ManifestName: raw} {
		if err = os.WriteFile(filepath.Join(source, path), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, pin, err := packages.VerifyDirectory(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	store, err := packages.OpenStore(filepath.Join(t.TempDir(), "store"), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	change, err := store.PrepareInstall(ctx, source, pin)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := change.ApprovalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, change, approval); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveSelected(ctx, []string{m.Name})
	if err != nil {
		t.Fatal(err)
	}
	base := resolved.Packages[0].Directory
	if err = os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{Skills: authoredSkillFixture{}}, runtime.RuntimeInputs{Tools: tool.NewRegistry()}, runtime.AgentSpec{SystemPrompt: "identity"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	originalLoader, _ := rt.Tools.Get("load_skill")
	backend := &backendSkillLoader{Tool: originalLoader}
	rt.Tools.(*tool.Registry).Register(backend)
	c := &Controller{Rt: rt}
	selection := []PackageSkillSelection{{Package: m.Name, Digest: pin, Path: "SKILL.md", Name: "verified"}, {Package: m.Name, Digest: pin, Path: "SKILL.md", Name: "authored"}}
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		t.Fatal(err)
	}
	err = c.SelectPackageSkills(ctx, store, selection)
	release()
	if !errors.Is(err, ErrBusy) {
		t.Fatal("busy selection", err)
	}
	if err = c.SelectPackageSkills(ctx, store, selection); err != nil {
		t.Fatal(err)
	}
	loader, _ := rt.Tools.Get("load_skill")
	if loader.IsConcurrencySafe(nil) {
		t.Fatal("package loader widened backend concurrency")
	}
	if result, err := rt.Tools.Execute(ctx, "load_skill", []byte(`{"name":"verified"}`)); err != nil || result.Error != "" || !strings.Contains(result.Output, string(body)) || len(backend.calls) != 0 {
		t.Fatal("selected package body was sent to the workspace backend", result, err, backend.calls)
	}
	if result, err := rt.Tools.Execute(ctx, "load_skill", []byte(`{"name":"authored"}`)); err != nil || result.Output != "backend skill: authored" || len(backend.calls) != 1 {
		t.Fatal("ordinary skill bypassed its configured backend", result, err, backend.calls)
	}
	if !strings.Contains(rt.StaticSystemPrompt, "verified") || !strings.Contains(rt.StaticSystemPrompt, pin) {
		t.Fatal("index missing provenance", rt.StaticSystemPrompt)
	}
	if text, ok := rt.Skills.Get("authored"); !ok || text != "authored body" {
		t.Fatal("authored precedence changed", text, ok)
	}
	if text, ok := rt.Skills.Get("verified"); !ok || !strings.Contains(text, string(body)) || !strings.Contains(text, "Resource base directory: "+strconv.Quote(base)) {
		t.Fatal(text, ok)
	}
	_, sources := rt.Skills.(*packageSkills).IndexSnapshot()
	if len(sources) != 1 || sources[0].Path != filepath.Join(base, "SKILL.md") {
		t.Fatal("context index did not expose the installed source", sources)
	}
	resourceRequest := []byte(`{"name":"verified","resource":"refs/checklist.md"}`)
	loaded, err := rt.Tools.Execute(ctx, "load_skill", resourceRequest)
	if err != nil || loaded.Error != "" || !strings.Contains(loaded.Output, string(asset)) || !strings.Contains(loaded.Output, pin) {
		t.Fatal("selected relative resource unavailable", loaded, err)
	}
	for _, bad := range []string{
		`{"name":"verified","resource":"../outside"}`,
		`{"name":"verified","resource":"/etc/passwd"}`,
		`{"name":"verified","resource":"unlisted"}`,
		`{"name":"authored","resource":"refs/checklist.md"}`,
		`{"name":"missing","resource":"refs/checklist.md"}`,
		`{"name":"verified","resource":"refs/checklist.md","execute":true}`,
	} {
		result, err := rt.Tools.Execute(ctx, "load_skill", []byte(bad))
		if err != nil || result.Error == "" || result.Output != "" {
			t.Fatal("invalid resource request exposed content", bad, result, err)
		}
	}
	assetPath := filepath.Join(base, "refs/checklist.md")
	if err = os.Chmod(assetPath, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(assetPath, []byte("tampered private content"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := rt.Tools.Execute(ctx, "load_skill", resourceRequest); err != nil || result.Error == "" || result.Output != "" {
		t.Fatal("changed resource escaped integrity check", result, err)
	}
	if err = os.WriteFile(assetPath, asset, 0400); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(assetPath, 0400); err != nil {
		t.Fatal(err)
	}
	previous := rt.Skills
	changed := append([]PackageSkillSelection(nil), selection...)
	changed[0].Digest = strings.Repeat("0", 64)
	if err = c.SelectPackageSkills(ctx, store, changed); err == nil || rt.Skills != previous {
		t.Fatal("wrong digest changed skill selection", err)
	}
	if err = c.SelectPackageSkills(ctx, store, append(selection, selection[0])); err == nil || rt.Skills != previous {
		t.Fatal("failed selection changed provider", err)
	}
	// Removing the installation cannot mutate the already selected verified body.
	removal, err := store.PrepareRemove(m.Name)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := removal.ApprovalDigest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(ctx, removal, digest); err != nil {
		t.Fatal(err)
	}
	if text, ok := rt.Skills.Get("verified"); !ok || !strings.Contains(text, string(body)) {
		t.Fatal("snapshot changed", text, ok)
	}
	if retained, err := os.ReadFile(filepath.Join(base, "refs/checklist.md")); err != nil || string(retained) != string(asset) {
		t.Fatal("relative reference changed after package removal", string(retained), err)
	}
	if result, err := rt.Tools.Execute(ctx, "load_skill", resourceRequest); err != nil || result.Error != "" || !strings.Contains(result.Output, string(asset)) {
		t.Fatal("selected resource lost after package removal", result, err)
	}
	if err = c.SelectPackageSkills(ctx, store, selection); err == nil || rt.Skills != previous {
		t.Fatal("removed package reselected", err)
	}
	if err = c.SelectPackageSkills(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if restored, _ := rt.Tools.Get("load_skill"); restored != backend {
		t.Fatal("deselection did not restore the configured backend")
	}
	if _, ok := rt.Skills.Get("verified"); ok || strings.Contains(rt.StaticSystemPrompt, "verified") {
		t.Fatal("package skill survived deselection")
	}
	if text, ok := rt.Skills.Get("authored"); !ok || text != "authored body" {
		t.Fatal("authored skill lost")
	}
}

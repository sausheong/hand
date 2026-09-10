package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/packages"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/tool"
)

func packageCLIExample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"main.py", packages.ManifestName} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "extensions", "python-task-note", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
func invokePackageCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := runPackages(context.Background(), args, &output, "1.0.0"); err != nil {
		t.Fatalf("packages %v: %v (%s)", args, err, output.Bytes())
	}
	return output.Bytes()
}
func applyPackageCLIReview(t *testing.T, raw []byte) {
	t.Helper()
	var review struct {
		File   string `json:"review_file"`
		Digest string `json:"approval_digest"`
	}
	if err := json.Unmarshal(raw, &review); err != nil {
		t.Fatal(err)
	}
	out := invokePackageCLI(t, "apply", "--review", review.File, "--approve", review.Digest)
	var result struct {
		Result packages.ChangeResult `json:"result"`
	}
	if err := json.Unmarshal(out, &result); err != nil || !result.Result.Committed {
		t.Fatal("CLI apply did not commit", string(out), err)
	}
}
func TestPackageCLIInspectReviewInstallUpdateRollbackRemove(t *testing.T) {
	source := packageCLIExample(t)
	store := filepath.Join(t.TempDir(), "store")
	reviews := t.TempDir()
	raw := invokePackageCLI(t, "inspect", "--source", source)
	var inspected struct {
		Digest string `json:"package_digest"`
	}
	if err := json.Unmarshal(raw, &inspected); err != nil || len(inspected.Digest) != 64 {
		t.Fatal(string(raw), err)
	}
	firstPin := inspected.Digest
	first := invokePackageCLI(t, "review-install", "--source", source, "--pin", firstPin, "--store", store, "--out", filepath.Join(reviews, "install.json"))
	applyPackageCLIReview(t, first)
	var initialReview struct {
		Review packages.ChangeReview `json:"review"`
	}
	if err := json.Unmarshal(first, &initialReview); err != nil {
		t.Fatal(err)
	}
	shown := invokePackageCLI(t, "show", "--store", store, "--name", initialReview.Review.Name)
	var selection packages.ResolvedSelection
	if err := json.Unmarshal(shown, &selection); err != nil || len(selection.Packages) != 1 || selection.Generation != 1 || selection.Packages[0].Digest != firstPin {
		t.Fatal("verified CLI selection missing", string(shown), err)
	}

	manifestPath := filepath.Join(source, packages.ManifestName)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	m, err := packages.DecodeManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.Version = "1.1.0"
	m.Extensions[0].Capabilities = append(m.Extensions[0].Capabilities, "file.read")
	raw, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	nextPin, err := m.Digest()
	if err != nil {
		t.Fatal(err)
	}
	next := invokePackageCLI(t, "review-install", "--source", source, "--pin", nextPin, "--store", store, "--out", filepath.Join(reviews, "update.json"))
	applyPackageCLIReview(t, next)
	raw = invokePackageCLI(t, "list", "--store", store)
	var listed struct {
		Lock packages.Lockfile `json:"lock"`
	}
	if err = json.Unmarshal(raw, &listed); err != nil || len(listed.Lock.Packages) != 1 || listed.Lock.Packages[0].Current != nextPin {
		t.Fatal("updated package missing", string(raw), err)
	}
	rollback := invokePackageCLI(t, "review-rollback", "--name", m.Name, "--pin", firstPin, "--store", store, "--out", filepath.Join(reviews, "rollback.json"))
	applyPackageCLIReview(t, rollback)
	removal := invokePackageCLI(t, "review-remove", "--name", m.Name, "--store", store, "--out", filepath.Join(reviews, "remove.json"))
	applyPackageCLIReview(t, removal)
	raw = invokePackageCLI(t, "list", "--store", store)
	if err = json.Unmarshal(raw, &listed); err != nil || len(listed.Lock.Packages) != 0 || listed.Lock.Generation != 4 {
		t.Fatal("removed package still listed", string(raw), err)
	}
}
func TestPackageCLIReviewRefusalsAndDevelopmentInspection(t *testing.T) {
	source := packageCLIExample(t)
	_, pin, err := packages.VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "store")
	file := filepath.Join(t.TempDir(), "review.json")
	raw := invokePackageCLI(t, "review-install", "--source", source, "--pin", pin, "--store", store, "--out", file)
	var review struct {
		Digest string `json:"approval_digest"`
	}
	if err = json.Unmarshal(raw, &review); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"apply", "--review", file, "--approve", strings.Repeat("0", 64)},
		{"review-install", "--source", source, "--pin", pin, "--store", store, "--out", file},
		{"list", "--store", store, "--approve", review.Digest},
		{"inspect", "--source", source, "extra"},
	} {
		if err := runPackages(context.Background(), args, &bytes.Buffer{}, "1.0.0"); err == nil {
			t.Fatal("invalid CLI accepted", args)
		}
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing review replaced", err)
	}
	if err = runPackages(context.Background(), []string{"list", "--store", store}, &bytes.Buffer{}, "dev"); err != nil {
		t.Fatal("development build cannot inspect store", err)
	}
	if err = runPackages(context.Background(), []string{"apply", "--review", file, "--approve", review.Digest}, &bytes.Buffer{}, "dev"); err == nil {
		t.Fatal("unknown development version bypassed compatibility")
	}
	state, err := packages.OpenStore(store, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	lock, err := state.List()
	if err != nil || lock.Generation != 0 {
		t.Fatal("refused commands changed store", err)
	}
}
func TestInvocationDispatchesPackageCommandsBeforeProviderSetup(t *testing.T) {
	invocationFixture(t, "packages", "unknown-package-operation")
	if err := run(); err == nil || !strings.Contains(err.Error(), "unknown package command") {
		t.Fatal("package command reached provider setup", err)
	}
}

func TestPackageCLIRuntimeAndLaunchReviewsRequireSeparateApproval(t *testing.T) {
	// This test checks the approval boundary independently of the machine's
	// Python installation. Real interpreters are exercised by package journeys.
	runtimeDir := t.TempDir()
	marker := filepath.Join(runtimeDir, "probed")
	program := "#!/bin/sh\n[ \"$#\" -eq 1 ] && [ \"$1\" = --version ] || exit 2\nprintf ran > '" + strings.ReplaceAll(marker, "'", "'\"'\"'") + "'\nprintf 'Python 3.11.4\\n'\n"
	if err := os.WriteFile(filepath.Join(runtimeDir, "python3"), []byte(program), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", runtimeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	source := packageCLIExample(t)
	store := filepath.Join(t.TempDir(), "store")
	reviews := t.TempDir()
	m, pin, err := packages.VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	applyPackageCLIReview(t, invokePackageCLI(t, "review-install", "--source", source, "--pin", pin, "--store", store, "--out", filepath.Join(reviews, "install.json")))
	runtimeFile := filepath.Join(reviews, "runtimes.json")
	raw := invokePackageCLI(t, "review-runtimes", "--store", store, "--names", m.Name, "--out", runtimeFile)
	var response struct {
		Reviews []app.PackageRuntimeApproval `json:"reviews"`
		Digests map[string]string            `json:"review_digests"`
	}
	if err = json.Unmarshal(raw, &response); err != nil || len(response.Reviews) != 1 {
		t.Fatal(string(raw), err)
	}
	if response.Reviews[0].ApprovedDigest != "" {
		t.Fatal("review fabricated approval")
	}
	workspace := t.TempDir()
	snapshots := filepath.Join(t.TempDir(), "snapshots")
	startup := filepath.Join(reviews, "startup.json")
	args := []string{"review-extensions", "--store", store, "--names", m.Name, "--workspace", workspace, "--snapshots", snapshots, "--runtime-approvals", runtimeFile, "--out", startup}
	var output bytes.Buffer
	if err = runPackages(context.Background(), args, &output, "1.0.0"); err == nil {
		t.Fatal("unapproved runtime accepted")
	}
	if _, err = os.Stat(startup); !os.IsNotExist(err) {
		t.Fatal("failed review left output", err)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("runtime executed before approval", err)
	}
	r := &response.Reviews[0]
	r.ApprovedDigest = response.Digests[r.Package+"/"+r.Review.Requirement.Name]
	if len(r.ApprovedDigest) != 64 {
		t.Fatal("runtime digest missing")
	}
	raw, err = json.Marshal(packageRuntimeReviews{Reviews: response.Reviews})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(runtimeFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	raw = invokePackageCLI(t, args...)
	if got, readErr := os.ReadFile(marker); readErr != nil || string(got) != "ran" {
		t.Fatal("approved runtime probe did not execute", string(got), readErr)
	}
	var launch struct {
		File   string `json:"review_file"`
		Digest string `json:"approval_digest"`
	}
	if err = json.Unmarshal(raw, &launch); err != nil {
		t.Fatal(err)
	}
	selected, err := app.ReadExtensionStartup(launch.File, launch.Digest)
	if err != nil || len(selected.Reviews) != 1 {
		t.Fatal(selected, err)
	}
	if selected.Identities["task-note"] != "package:"+m.Name+":task-note" {
		t.Fatal(selected.Identities)
	}
	if _, err = os.Stat(snapshots); !os.IsNotExist(err) {
		t.Fatal("review activated extension", err)
	}
	if err = runPackages(context.Background(), args, &output, "1.0.0"); err == nil {
		t.Fatal("existing startup file overwritten")
	}
	if _, err = app.ReadExtensionStartup(launch.File, launch.Digest); err != nil {
		t.Fatal("existing review changed", err)
	}
}

func TestPackageCLIReadInstalledPrompt(t *testing.T) {
	source := packageCLIExample(t)
	body := []byte("Explain the failing test before editing.")
	sum := sha256.Sum256(body)
	raw, err := os.ReadFile(filepath.Join(source, packages.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	m, err := packages.DecodeManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	m.Files = append(m.Files, packages.File{Path: "review.md", Kind: "prompt", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])})
	m.Files = append(m.Files, packages.File{Path: "SKILL.md", Kind: "skill", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:])})
	if err = os.WriteFile(filepath.Join(source, "SKILL.md"), body, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, packages.ManifestName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(source, "review.md"), body, 0600); err != nil {
		t.Fatal(err)
	}
	_, pin, err := packages.VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(t.TempDir(), "store")
	applyPackageCLIReview(t, invokePackageCLI(t, "review-install", "--source", source, "--pin", pin, "--store", store, "--out", filepath.Join(t.TempDir(), "install.json")))
	raw = invokePackageCLI(t, "read", "--store", store, "--name", m.Name, "--path", "review.md")
	var resource packages.TextResource
	if err = json.Unmarshal(raw, &resource); err != nil || resource.Text != string(body) || resource.PackageDigest != pin {
		t.Fatal(string(raw), err)
	}
	var output bytes.Buffer
	if err = runPackages(context.Background(), []string{"read", "--store", store, "--name", m.Name, "--path", "main.py"}, &output, "1.0.0"); err == nil {
		t.Fatal("extension source accepted as prompt")
	}

	promptConfig := filepath.Join(t.TempDir(), "prompt.json")
	promptSelection := map[string]any{"version": 1, "store": store, "package": m.Name, "digest": pin, "path": "review.md"}
	raw, err = json.Marshal(promptSelection)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(promptConfig, raw, 0600); err != nil {
		t.Fatal(err)
	}
	promptResource, err := readInstalledPackagePrompt(context.Background(), promptConfig, "1.0.0")
	if err != nil || promptResource.Text != string(body) {
		t.Fatal(promptResource, err)
	}
	promptSelection["digest"] = strings.Repeat("0", 64)
	raw, err = json.Marshal(promptSelection)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(promptConfig, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readInstalledPackagePrompt(context.Background(), promptConfig, "1.0.0"); err == nil {
		t.Fatal("wrong prompt pin accepted")
	}
	t.Setenv("HOME", t.TempDir())
	provider, _, err := agentio.BuildSkillProvider(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rt, err := runtime.BuildRuntime(runtime.RuntimeDeps{Skills: provider}, runtime.RuntimeInputs{Tools: tool.NewRegistry()}, runtime.AgentSpec{ID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	startup := packageSkillStartup{Version: 1, Store: store, Skills: []app.PackageSkillSelection{{Package: m.Name, Digest: pin, Path: "SKILL.md", Name: "package-review"}}}
	config := filepath.Join(t.TempDir(), "skills.json")
	raw, err = json.Marshal(startup)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	selected, err := readPackageSkillStartup(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = applyPackageSkillStartup(context.Background(), rt, selected, "1.0.0"); err != nil {
		t.Fatal(err)
	}
	result, err := rt.Tools.Execute(context.Background(), "load_skill", []byte(`{"name":"package-review"}`))
	if err != nil || result.Error != "" || !strings.Contains(result.Output, string(body)) {
		t.Fatal(result, err)
	}
	startup.Skills[0].Digest = ""
	raw, err = json.Marshal(startup)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(config, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readPackageSkillStartup(config); err == nil {
		t.Fatal("unpinned startup accepted")
	}

}

func TestNativePackageCLIContainerRuntimeAndLaunchReviews(t *testing.T) {
	image, socket := os.Getenv("HAND_TEST_PYTHON_IMAGE"), os.Getenv("HARNESS_TEST_CONTAINER_SOCKET")
	if image == "" || socket == "" {
		t.Skip("native Python image required")
	}
	configFile := filepath.Join(t.TempDir(), "container.json")
	configRaw, _ := json.Marshal(map[string]any{"boundary": map[string]any{"docker": "/usr/local/bin/docker", "socket": socket, "image": image, "network": false, "writable": false}, "interpreters": map[string]string{"python": "/usr/local/bin/python3"}})
	if err := os.WriteFile(configFile, configRaw, 0600); err != nil {
		t.Fatal(err)
	}
	source := packageCLIExample(t)
	store := filepath.Join(t.TempDir(), "store")
	reviews := t.TempDir()
	m, pin, err := packages.VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	applyPackageCLIReview(t, invokePackageCLI(t, "review-install", "--source", source, "--pin", pin, "--store", store, "--out", filepath.Join(reviews, "install.json")))
	runtimeFile := filepath.Join(reviews, "runtimes.json")
	raw := invokePackageCLI(t, "review-runtimes", "--container", configFile, "--store", store, "--names", m.Name, "--out", runtimeFile)
	var response struct {
		Reviews []app.PackageContainerRuntimeApproval `json:"reviews"`
		Digests map[string]string                     `json:"review_digests"`
	}
	if err = json.Unmarshal(raw, &response); err != nil || len(response.Reviews) != 1 {
		t.Fatal(string(raw), err)
	}
	if response.Reviews[0].ApprovedDigest != "" {
		t.Fatal("review fabricated approval")
	}
	workspace := t.TempDir()
	snapshots := filepath.Join(t.TempDir(), "snapshots")
	startup := filepath.Join(reviews, "startup.json")
	args := []string{"review-extensions", "--container", configFile, "--store", store, "--names", m.Name, "--workspace", workspace, "--snapshots", snapshots, "--runtime-approvals", runtimeFile, "--out", startup}
	var output bytes.Buffer
	if err = runPackages(context.Background(), args, &output, "1.0.0"); err == nil {
		t.Fatal("unapproved runtime accepted")
	}
	if _, err = os.Stat(startup); !os.IsNotExist(err) {
		t.Fatal("failed review left output", err)
	}
	r := &response.Reviews[0]
	r.ApprovedDigest = response.Digests[r.Package+"/"+r.Review.Requirement.Name]
	if len(r.ApprovedDigest) != 64 {
		t.Fatal("runtime digest missing")
	}
	raw, err = json.Marshal(packageContainerReviews{Reviews: response.Reviews})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(runtimeFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	raw = invokePackageCLI(t, args...)
	var launch struct {
		File   string `json:"review_file"`
		Digest string `json:"approval_digest"`
	}
	if err = json.Unmarshal(raw, &launch); err != nil {
		t.Fatal(err)
	}
	selected, err := app.ReadExtensionStartup(launch.File, launch.Digest)
	if err != nil || len(selected.Reviews) != 1 {
		t.Fatal(selected, err)
	}
	if selected.Reviews[0].Container == nil || selected.Reviews[0].Container.Image != image || selected.Reviews[0].ImageInterpreter[0] != "/usr/local/bin/python3" {
		t.Fatal("container mapping lost", selected)
	}
	if selected.Identities["task-note"] != "package:"+m.Name+":task-note" {
		t.Fatal(selected.Identities)
	}
	if _, err = os.Stat(snapshots); !os.IsNotExist(err) {
		t.Fatal("review activated extension", err)
	}
	if err = runPackages(context.Background(), args, &output, "1.0.0"); err == nil {
		t.Fatal("existing startup file overwritten")
	}
	if _, err = app.ReadExtensionStartup(launch.File, launch.Digest); err != nil {
		t.Fatal("existing review changed", err)
	}
}

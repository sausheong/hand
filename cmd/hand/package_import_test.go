package main

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/packages"
)

func TestPackageCLIImportedReviewSurvivesInspectionCleanup(t *testing.T) {
	for _, kind := range []string{"archive", "git"} {
		t.Run(kind, func(t *testing.T) {
			source := packageCLIExample(t)
			_, pin, err := packages.VerifyDirectory(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(t.TempDir(), "store")
			reviewFile := filepath.Join(t.TempDir(), "review.json")
			location, reference := source, ""
			args := []string{"review-" + kind, "--pin", pin, "--store", store, "--out", reviewFile}
			if kind == "archive" {
				location = filepath.Join(t.TempDir(), "package.tar")
				var buffer bytes.Buffer
				writer := tar.NewWriter(&buffer)
				for _, name := range []string{packages.ManifestName, "main.py"} {
					raw, err := os.ReadFile(filepath.Join(source, name))
					if err != nil {
						t.Fatal(err)
					}
					if err = writer.WriteHeader(&tar.Header{Name: name, Size: int64(len(raw)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
						t.Fatal(err)
					}
					if _, err = writer.Write(raw); err != nil {
						t.Fatal(err)
					}
				}
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
				raw := buffer.Bytes()
				hash := sha256.Sum256(raw)
				reference = hex.EncodeToString(hash[:])
				if err = os.WriteFile(location, raw, 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--archive-sha256", reference)
			} else {
				run := func(args ...string) string {
					command := exec.Command("git", append([]string{"-C", source}, args...)...)
					command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Hand test", "GIT_AUTHOR_EMAIL=hand@example.invalid", "GIT_COMMITTER_NAME=Hand test", "GIT_COMMITTER_EMAIL=hand@example.invalid")
					output, err := command.CombinedOutput()
					if err != nil {
						t.Fatalf("Git fixture: %v %s", err, output)
					}
					return strings.TrimSpace(string(output))
				}
				run("init", "-q")
				run("add", "--", packages.ManifestName, "main.py")
				run("commit", "-qm", "package fixture")
				reference = run("rev-parse", "HEAD")
				args = append(args, "--commit", reference)
			}
			args = append(args, "--source", location)
			raw := invokePackageCLI(t, args...)
			var reviewed struct {
				Review packages.ChangeReview `json:"review"`
				Digest string                `json:"approval_digest"`
			}
			if err = json.Unmarshal(raw, &reviewed); err != nil {
				t.Fatal(err)
			}
			if reviewed.Review.Source != location || reviewed.Review.Origin == nil || reviewed.Review.Origin.Reference != reference {
				t.Fatal("review depends on temporary source", reviewed)
			}
			// Inspection has returned and deleted its snapshots. Apply must re-read the
			// original pinned source, refusing when that source is no longer available.
			moved := location + "-temporarily-unavailable"
			if err = os.Rename(location, moved); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			err = runPackages(context.Background(), []string{"apply", "--review", reviewFile, "--approve", reviewed.Digest}, &output, "1.0.0")
			if err == nil {
				t.Fatal("missing import source accepted")
			}
			if err = os.Rename(moved, location); err != nil {
				t.Fatal(err)
			}
			applyPackageCLIReview(t, raw)
			stateRaw := invokePackageCLI(t, "list", "--store", store)
			var state struct {
				Lock packages.Lockfile `json:"lock"`
			}
			if err = json.Unmarshal(stateRaw, &state); err != nil {
				t.Fatal(err)
			}
			if state.Lock.Generation != 1 || len(state.Lock.Packages) != 1 {
				t.Fatal("import application failed", state)
			}
			revision := state.Lock.Packages[0].Revisions[0]
			if revision.Origin == nil || revision.Origin.Kind != kind || revision.Origin.Location != location || revision.Origin.Reference != reference {
				t.Fatal("import provenance lost", revision)
			}
			entries, err := os.ReadDir(filepath.Join(store, "staging"))
			if err != nil || len(entries) != 0 {
				t.Fatal("import staging leaked", entries, err)
			}
		})
	}
}

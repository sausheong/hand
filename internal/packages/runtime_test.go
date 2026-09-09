package packages

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSemanticVersionPrecedenceAndHandCompatibility(t *testing.T) {
	ordered := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0", "1.0.1", "2.0.0", "999999999999999999999999.0.0"}
	for i, a := range ordered {
		for j, b := range ordered {
			got, err := CompareVersions(a, b)
			want := 0
			if i < j {
				want = -1
			}
			if i > j {
				want = 1
			}
			if err != nil || got != want {
				t.Fatalf("%s vs %s: %d %v", a, b, got, err)
			}
		}
	}
	if got, err := CompareVersions("1.0.0+build.1", "1.0.0+build.2"); err != nil || got != 0 {
		t.Fatal("build metadata affected precedence", got, err)
	}
	m := fixtureManifest()
	m.Compatibility.MinimumHand = "1.2.0"
	for _, v := range []string{"1.1.9", "1.2.0-rc.1", "dev", "1.02.0"} {
		if m.CheckHandVersion(v) == nil {
			t.Fatal("incompatible Hand accepted", v)
		}
	}
	for _, v := range []string{"1.2.0", "v1.3.0", "2.0.0"} {
		if err := m.CheckHandVersion(v); err != nil {
			t.Fatal(err)
		}
	}
}
func TestReviewedRuntimeProbeApprovalFingerprintAndCancellation(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	program := fmt.Sprintf(`package main
import("fmt";"os";"strings";"time")
func main(){if len(os.Args)!=2||os.Args[1]!="--version"{os.Exit(2)};if strings.Contains(os.Args[0],"slow"){os.WriteFile(%q,[]byte("ready"),0600);time.Sleep(10*time.Second)};os.WriteFile(%q,[]byte("ran"),0600);fmt.Println("Python 3.11.4")}
`, marker+"-ready", marker)
	source := filepath.Join(dir, "main.go")
	binary := filepath.Join(dir, "python-fixture")
	if err := os.WriteFile(source, []byte(program), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", binary, source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture %v %s", err, out)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	review, err := ReviewRuntime(context.Background(), Runtime{Name: "python", Command: "python-fixture", MinimumVersion: "3.10.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("review executed interpreter")
	}
	digest, err := review.Digest()
	if err != nil {
		t.Fatal(err)
	}
	for _, approval := range []string{"", strings.Repeat("0", 64)} {
		if _, err = ProbeRuntime(context.Background(), review, approval); err == nil {
			t.Fatal("unapproved probe executed")
		}
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("refusal executed interpreter")
	}
	version, err := ProbeRuntime(context.Background(), review, digest)
	if err != nil || version != "3.11.4" {
		t.Fatal(version, err)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatal("approved probe did not execute", err)
	}
	review.Requirement.MinimumVersion = "3.12.0"
	oldApproval := digest
	digest, _ = review.Digest()
	if _, err = ProbeRuntime(context.Background(), review, oldApproval); err == nil {
		t.Fatal("changed review reused approval")
	}
	if version, err = ProbeRuntime(context.Background(), review, digest); err == nil || version != "3.11.4" {
		t.Fatal("old runtime accepted", version, err)
	}
	bytes, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	slow := filepath.Join(dir, "slow-fixture")
	if err = os.WriteFile(slow, bytes, 0700); err != nil {
		t.Fatal(err)
	}
	slowReview, err := ReviewRuntime(context.Background(), Runtime{Name: "python", Command: "slow-fixture", MinimumVersion: "3.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	pin, _ := slowReview.Digest()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, e := ProbeRuntime(ctx, slowReview, pin); done <- e }()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
ready:
	for {
		select {
		case <-ticker.C:
			if _, e := os.Stat(marker + "-ready"); e == nil {
				break ready
			}
		case e := <-done:
			t.Fatalf("probe exited before ready: %v", e)
		case <-deadline.C:
			t.Fatal("probe never started")
		}
	}
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("probe ignored cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("probe cleanup too slow")
	}
	if err = os.WriteFile(binary, []byte("changed executable"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = ProbeRuntime(context.Background(), review, digest); err == nil {
		t.Fatal("changed runtime executed")
	}
	if _, err = ReviewRuntime(context.Background(), Runtime{Name: "python", Command: "missing-hand-runtime-fixture", MinimumVersion: "3.0.0"}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatal("missing runtime diagnostic absent", err)
	}
}
func TestRuntimeVersionParsers(t *testing.T) {
	for _, tc := range []struct{ name, output, want string }{{"python", "Python 3.12.1\n", "3.12.1"}, {"node", "v22.1.0", "22.1.0"}, {"ruby", "ruby 3.2.2p53 (2023-03-30) [arm64-darwin]", "3.2.2"}, {"python", "Python 3.12.1\nextra", ""}, {"node", "not-version", ""}} {
		got, err := parseRuntimeVersion(tc.name, tc.output)
		if tc.want == "" {
			if err == nil {
				t.Fatal("invalid output accepted")
			}
		} else if err != nil || got != tc.want {
			t.Fatal(got, err)
		}
	}
}

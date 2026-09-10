package extensions

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func launchFixture(t *testing.T) LaunchReview {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r, err := ReviewHostLaunch(context.Background(), LaunchConfig{Name: "fixture", Executable: exe, Workspace: t.TempDir(), Arguments: []string{"-test.run=^TestExtensionPeerProcess$"}, Environment: map[string]string{"HAND_EXTENSION_TEST_PEER": "normal"}, Capabilities: []string{"commands"}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestHostLaunchAdmissionAndCleanup(t *testing.T) {
	r := launchFixture(t)
	root := filepath.Join(t.TempDir(), "private")
	denied := errors.New("denied")
	calls := 0
	factory, err := NewHostFactory(root, []LaunchReview{r}, func(_ context.Context, review LaunchReview) error { calls++; return denied })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = factory(context.Background(), context.Background(), r.Specification); !errors.Is(err, denied) {
		t.Fatalf("admission: %v", err)
	}
	entries, _ := os.ReadDir(root)
	if calls != 1 || len(entries) != 0 {
		t.Fatal("denied launch created snapshot")
	}
	factory, err = NewHostFactory(root, []LaunchReview{r}, func(_ context.Context, review LaunchReview) error {
		review.Environment["HAND_EXTENSION_TEST_PEER"] = "hang"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := factory(ctx, ctx, r.Specification)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Initialize(ctx, "fixture", []string{"commands"}); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	entries, _ = os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("snapshot survived joined close")
	}
}
func TestHostLaunchRevalidatesResources(t *testing.T) {
	r := launchFixture(t)
	asset := filepath.Join(t.TempDir(), "source.py")
	if err := os.WriteFile(asset, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := fingerprint(asset, "source.py")
	if err != nil {
		t.Fatal(err)
	}
	r.Assets = []LaunchFile{f}
	r.Specification.Digest, _ = reviewDigest(r)
	root := filepath.Join(t.TempDir(), "private")
	factory, err := NewHostFactory(root, []LaunchReview{r}, func(context.Context, LaunchReview) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(asset, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := factory(context.Background(), context.Background(), r.Specification); err == nil {
		c.Close()
		t.Fatal("changed same-size source accepted")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("rejected snapshot leaked")
	}
}
func TestHostLaunchRejectsManufacturedReviews(t *testing.T) {
	base := launchFixture(t)
	cases := map[string]func(*LaunchReview){
		"traversal":           func(r *LaunchReview) { f := r.Binary; f.Relative = "../outside"; r.Assets = []LaunchFile{f} },
		"absolute":            func(r *LaunchReview) { f := r.Binary; f.Relative = "/outside"; r.Assets = []LaunchFile{f} },
		"negative-size":       func(r *LaunchReview) { r.Binary.Size = -1 },
		"unreviewed-argument": func(r *LaunchReview) { r.Arguments = []string{"${package}/missing"} },
		"boundary":            func(r *LaunchReview) { r.Boundary = "container" },
		"environment":         func(r *LaunchReview) { r.Environment["BAD=KEY"] = "value" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := cloneReview(base)
			change(&r)
			r.Specification.Digest, _ = reviewDigest(r)
			if _, err := NewHostFactory(filepath.Join(t.TempDir(), "private"), []LaunchReview{r}, func(context.Context, LaunchReview) error { return nil }); err == nil {
				t.Fatal("manufactured review accepted")
			}
		})
	}
	if _, err := NewHostFactory(filepath.Join(base.Workspace, "snapshots"), []LaunchReview{base}, func(context.Context, LaunchReview) error { return nil }); err == nil {
		t.Fatal("workspace snapshot accepted")
	}
}

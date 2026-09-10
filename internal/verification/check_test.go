package verification

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCheckRequiresReferencedSnapshots(t *testing.T) {
	o := options(t, "printf checked")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	now := current(t, o)
	if a := r.Check(context.Background(), now, o.ProfileDigest, o.Checkpoints, o.Output); a.Status != "passed" {
		t.Fatal(a)
	}
	if a := r.Check(context.Background(), now, strings.Repeat("b", 64), o.Checkpoints, o.Output); a.Status != "stale" {
		t.Fatal(a)
	}
	if err = o.Checkpoints.Delete(r.View().Before); err != nil {
		t.Fatal(err)
	}
	if a := r.Check(context.Background(), now, o.ProfileDigest, o.Checkpoints, o.Output); a.Status != "unverified" || !strings.Contains(a.Reason, "snapshot") {
		t.Fatal("deleted evidence stayed passed", a)
	}
}
func TestCheckDetectsExpiredOutput(t *testing.T) {
	o := options(t, "head -c 100000 /dev/zero")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	now := current(t, o)
	if a := r.Check(context.Background(), now, o.ProfileDigest, o.Checkpoints, o.Output); a.Status != "passed" {
		t.Fatal(a)
	}
	if err = os.Remove(r.View().StdoutArtifact.Path); err != nil {
		t.Fatal(err)
	}
	if a := r.Check(context.Background(), now, o.ProfileDigest, o.Checkpoints, o.Output); a.Status != "unverified" || !strings.Contains(a.Reason, "stdout") {
		t.Fatal(a)
	}
}
func TestCheckDetectsContradictoryOutputAndScope(t *testing.T) {
	o := options(t, "printf text")
	r, err := Run(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	r.view.StdoutBytes++
	if a := r.Check(context.Background(), current(t, o), o.ProfileDigest, o.Checkpoints, o.Output); a.Status != "unverified" {
		t.Fatal(a)
	}
	r.view.StdoutBytes--
	r.view.Omissions = append(r.view.Omissions, struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	}{Path: "invented", Reason: "excluded"})
	if a := r.Check(context.Background(), current(t, o), o.ProfileDigest, o.Checkpoints, o.Output); a.Status != "unverified" {
		t.Fatal(a)
	}
}

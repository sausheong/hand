package tui

import (
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/verification"
	"strings"
	"testing"
)

func TestVerificationReviewAndStalePresentation(t *testing.T) {
	profile := app.VerificationProfileView{Name: "unit", Command: []string{"echo", "two words"}, Digest: strings.Repeat("a", 64), Boundary: "unrestricted host"}
	text := strings.Join(verificationProfileLines([]app.VerificationProfileView{profile}), "\n")
	if !strings.Contains(text, `["echo","two words"]`) || !strings.Contains(text, "/verify-confirm unit ") {
		t.Fatal(text)
	}
	result := app.VerificationResult{ID: strings.Repeat("b", 64), Record: verification.View{Profile: "unit", ExitCode: 0, Stdout: strings.Repeat("x", 3000)}, Assessment: verification.Assessment{Status: "stale"}}
	text = strings.Join(verificationResultLines(result), "\n")
	if !strings.Contains(text, "stale (exit 0)") || !strings.Contains(text, "/verify-check unit ") || len(text) > 2600 {
		t.Fatal("unbounded or wrong status", len(text))
	}
}
func TestVerificationConfirmationConsumed(t *testing.T) {
	m, _, _ := sessionUIFixture(t)
	m.verificationReviews = []app.VerificationProfileView{{Name: "unit", Digest: "digest"}}
	if cmd := m.runVerificationConfirm([]string{"unit", "wrong"}); cmd != nil {
		t.Fatal("wrong digest dispatched")
	}
	m.running = true
	if cmd := m.handleCommand("/verify-confirm unit digest"); cmd != nil {
		t.Fatal("busy verification dispatched")
	}
	m.running = false
	cmd := m.runVerificationConfirm([]string{"unit", "digest"})
	if cmd == nil || len(m.verificationReviews) != 0 {
		t.Fatal("confirmation not consumed")
	}
	m.Update(cmd())
	if m.sessionChanging || len(m.verificationReviews) != 0 {
		t.Fatal("failed verification left active state")
	}
	if cmd := m.runVerificationConfirm([]string{"unit", "digest"}); cmd != nil {
		t.Fatal("confirmation replay dispatched")
	}
}

package agentio

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestUserOutcomePreservesMeaningAndEvidence(t *testing.T) {
	for _, tc := range []struct {
		outcome RunOutcome
		want    string
	}{
		{RunOutcome{Status: Completed, Reason: "answer_completed"}, "Completed"},
		{RunOutcome{Status: Completed, Verified: true}, "Completed — checks passed"},
		{RunOutcome{Status: Cancelled, Reason: "context_cancelled", Cause: errors.New("context canceled")}, "Cancelled"},
		{RunOutcome{Status: InfrastructureError, Reason: "runtime_error", Cause: errors.New("HTTP 401: No api key passed in")}, "Could not complete the request: HTTP 401: No api key passed in"},
		{RunOutcome{Status: VerificationFailed, Reason: "stop_validator_failed", Cause: errors.New("go test failed")}, "Checks failed: go test failed"},
		{RunOutcome{Status: BudgetExhausted, Reason: "run_token_budget"}, "Token limit reached for this task"},
		{RunOutcome{Status: InfrastructureError, Reason: "missing_completion"}, "The response ended before the task finished"},
	} {
		before, _ := json.Marshal(tc.outcome)
		code := tc.outcome.ExitCode()
		if got := tc.outcome.UserMessage(); got != tc.want {
			t.Fatalf("got %q, want %q", got, tc.want)
		}
		after, _ := json.Marshal(tc.outcome)
		if string(before) != string(after) || code != tc.outcome.ExitCode() {
			t.Fatal("presentation changed machine outcome")
		}
		if tc.outcome.Status != Completed && strings.Contains(tc.outcome.UserMessage(), "checks passed") {
			t.Fatal("failure presented as verified")
		}
	}
}

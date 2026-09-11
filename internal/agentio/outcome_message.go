package agentio

import "strings"

// UserMessage presents an outcome without exposing internal status/reason codes.
// The structured outcome remains unchanged for automation and saved evidence.
func (o RunOutcome) UserMessage() string {
	if o.Status == Completed {
		if o.Verified {
			return "Completed — checks passed"
		}
		return "Completed"
	}
	if o.Status == Cancelled {
		return "Cancelled"
	}
	message := map[RunStatus]string{
		VerificationFailed: "Checks failed", BudgetExhausted: "Limit reached",
		InfrastructureError: "Could not complete the request",
	}[o.Status]
	if message == "" {
		message = "Could not complete the request"
	}
	detail := map[string]string{
		"run_time_budget":            "Time limit reached for this task",
		"session_time_budget":        "Session time limit reached",
		"run_token_budget":           "Token limit reached for this task",
		"session_token_budget":       "Session token limit reached",
		"run_cost_budget":            "Spending limit reached for this task",
		"session_cost_budget":        "Session spending limit reached",
		"cost_price_unknown":         "Model pricing is needed to check the spending limit",
		"max_turns":                  "Step limit reached",
		"max_iterations":             "Attempt limit reached",
		"missing_completion":         "The response ended before the task finished",
		"missing_event_stream":       "The model did not start a response",
		"stop_validator_failed":      "Checks failed",
		"prompt_validator_failed":    "The request did not pass the required checks",
		"operation_cleanup_failed":   "The task finished, but cleanup failed",
		"operation_release_failed":   "Could not finish cleaning up the task",
		"run_metadata_start_failed":  "Could not save the task record",
		"run_metadata_finish_failed": "Could not save the task result",
		"checkpoint_start_failed":    "Could not save a backup before making changes",
		"checkpoint_finish_failed":   "Could not save the change history",
		"run_budget_prepare_failed":  "Could not check the task limits",
	}[o.Reason]
	if detail != "" {
		message = detail
	}
	if o.Cause != nil {
		return message + ": " + o.Cause.Error()
	}
	if detail == "" && o.Reason != "" && o.Reason != "runtime_error" {
		return message + ": " + strings.ReplaceAll(o.Reason, "_", " ")
	}
	return message
}

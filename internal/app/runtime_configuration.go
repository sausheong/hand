package app

import (
	"errors"
	"fmt"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

// ConfigureInitialRuntime applies a resolved profile before the runtime is
// published to a service or started. Subsequent changes must use Controller's
// reserved configuration operations. Validation completes before mutation.
func ConfigureInitialRuntime(rt *runtime.Runtime, profile config.ModelProfile, identityHint string) error {
	if rt == nil {
		return errors.New("cannot configure a missing runtime")
	}
	if profile.ContextLimit < 0 || profile.MaxOutput < 0 {
		return errors.New("context and output limits must not be negative")
	}
	if profile.ContextLimit > 0 && profile.MaxOutput >= profile.ContextLimit {
		return errors.New("output limit must leave room for input within the context limit")
	}
	reasoning, err := llm.ParseReasoningMode(profile.Reasoning)
	if err != nil {
		return fmt.Errorf("configure profile reasoning: %w", err)
	}
	rt.DynamicIdentityHint = identityHint
	rt.ContextWindow = profile.ContextLimit
	rt.MaxOutputTokens = profile.MaxOutput
	rt.Reasoning = reasoning
	return nil
}

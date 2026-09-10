package app

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
)

func TestProfileCancellationBeforePermissionCommit(t *testing.T) {
	for _, stage := range []string{"provider", "identity", "permissions"} {
		t.Run(stage, func(t *testing.T) {
			workspace := t.TempDir()
			authority, err := permissions.OpenAuthority(filepath.Join(t.TempDir(), "authority"), workspace, strings.Repeat("a", 64))
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			original := &controllerProvider{}
			profile := config.ModelProfile{Provider: "openai", Model: "old", CredentialEnv: "OLD_KEY"}
			c := &Controller{Rt: &runtime.Runtime{Provider: "openai", Model: "old", LLM: original, ContextWindow: 12345, FallbackModel: "fallback", DynamicIdentityHint: "old identity"}, Authority: authority, PermissionProfile: profile}
			if err := c.ConfigureProfiles(map[string]config.ModelProfile{
				"old": profile,
				"new": {Provider: "openai", Model: "new", CredentialEnv: "NEW_KEY", ContextLimit: 32000},
			}, "old"); err != nil {
				t.Fatal(err)
			}
			beforeRuntime := c.Rt
			beforeReasoning, beforeRoute := c.Rt.Reasoning, c.Rt.Route
			beforeOptions := c.owner().options
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			shouldCancel := true
			cancelAt := func(at string) {
				if shouldCancel && at == stage {
					cancel()
				}
			}
			c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) {
				cancelAt("provider")
				return &controllerProvider{}, nil
			}
			c.BuildModelIdentityHint = func(string) string { cancelAt("identity"); return "new identity" }
			commits := 0
			c.PreparePermissions = func(config.ModelProfile) (func(), error) {
				cancelAt("permissions")
				return func() { commits++ }, nil
			}
			if err := c.SwitchProfileContext(ctx, "new"); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want cancellation", err)
			}
			if commits != 0 || c.Rt != beforeRuntime || c.Rt.LLM != original || c.Rt.Provider != "openai" || c.Rt.Model != "old" || c.Rt.ContextWindow != 12345 || c.Rt.FallbackModel != "fallback" || c.Rt.DynamicIdentityHint != "old identity" || c.Rt.Reasoning != beforeReasoning || c.Rt.Route != beforeRoute || c.BaseURL != "" || !reflect.DeepEqual(c.PermissionProfile, profile) || c.PermissionState().Authority != authority || c.CurrentProfile() != "old" || !reflect.DeepEqual(c.owner().options, beforeOptions) {
				t.Fatal("cancelled profile changed runtime, permission binding, or service options")
			}
			// A failed operation must release ownership, so the same reviewed switch
			// can subsequently commit once with a fresh caller context.
			shouldCancel = false
			if err := c.SwitchProfileContext(context.Background(), "new"); err != nil {
				t.Fatal(err)
			}
			if commits != 1 || c.CurrentProfile() != "new" || c.CurrentModel() != "openai/new" || c.PermissionProfile.CredentialEnv != "NEW_KEY" {
				t.Fatal("retry did not commit exactly once")
			}
		})
	}
}

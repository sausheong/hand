package app

import (
	"context"
	"errors"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/runtime"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelAndProfilePermissionPreparationIsAtomic(t *testing.T) {
	for _, mode := range []string{"model", "profile"} {
		t.Run(mode, func(t *testing.T) {
			c := &Controller{Rt: &runtime.Runtime{Provider: "openai", Model: "old"}, PermissionProfile: config.ModelProfile{CredentialEnv: "ORIGINAL_KEY"}}
			c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { return &controllerProvider{}, nil }
			if err := c.ConfigureProfiles(map[string]config.ModelProfile{"new": {Provider: "openai", Model: "new", CredentialEnv: "NEW_KEY"}}, ""); err != nil {
				t.Fatal(err)
			}
			fail := true
			commits := 0
			c.PreparePermissions = func(p config.ModelProfile) (func(), error) {
				if p.Model != "new" {
					t.Fatalf("wrong proposed model: %+v", p)
				}
				expected := "ORIGINAL_KEY"
				if mode == "profile" {
					expected = "NEW_KEY"
				}
				if p.CredentialEnv != expected {
					t.Fatalf("credential identity lost: %+v", p)
				}
				if fail {
					return nil, errors.New("authority store unavailable")
				}
				return func() { commits++ }, nil
			}
			change := func() error {
				if mode == "model" {
					return c.SwitchModel("openai/new")
				}
				return c.SwitchProfile("new")
			}
			if err := change(); err == nil {
				t.Fatal("authority failure ignored")
			}
			if c.CurrentModel() != "openai/old" || commits != 0 {
				t.Fatal("failed preparation changed runtime")
			}
			fail = false
			if err := change(); err != nil {
				t.Fatal(err)
			}
			if c.CurrentModel() != "openai/new" || commits != 1 {
				t.Fatal("successful switch did not commit both states")
			}
		})
	}
}

func TestBoundApprovalHookReadsCurrentConfiguration(t *testing.T) {
	workspace := t.TempDir()
	firstDigest, secondDigest := strings.Repeat("a", 64), strings.Repeat("b", 64)
	first, err := permissions.OpenAuthority(filepath.Join(t.TempDir(), "first"), workspace, firstDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := permissions.OpenAuthority(filepath.Join(t.TempDir(), "second"), workspace, secondDigest)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	proposal, err := permissions.PrepareLegacyMigration(permissions.Settings{AlwaysAllow: []string{"bash"}}, workspace, firstDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.AcknowledgeLegacy(proposal, proposal.Fingerprint()); err != nil {
		t.Fatal(err)
	}
	current := PermissionState{Authority: first, Digest: firstDigest}
	hook := NewBoundApprovalHook(func() PermissionState { return current }, workspace, nil)
	ctx := context.WithValue(context.Background(), serviceContextKey{}, &Service{})
	decision, err := hook(ctx, "bash", []byte(`{"command":"echo hello"}`))
	if err != nil || !decision.Allow {
		t.Fatalf("initial grant not used: %+v %v", decision, err)
	}
	current = PermissionState{Authority: second, Digest: secondDigest}
	decision, err = hook(ctx, "bash", []byte(`{"command":"echo hello"}`))
	// The inactive fixture service rejects the new approval request. Crucially the
	// old journal cannot bypass that request after the binding changes.
	if decision.Allow || !errors.Is(err, ErrApprovalExpired) {
		t.Fatalf("old binding reused: %+v %v", decision, err)
	}
}

func TestPermissionPreparationCannotCommitMissingBinding(t *testing.T) {
	for _, mode := range []string{"model", "profile"} {
		for _, failure := range []string{"no-rebinder", "nil-commit"} {
			t.Run(mode+"/"+failure, func(t *testing.T) {
				workspace := t.TempDir()
				authority, err := permissions.OpenAuthority(filepath.Join(t.TempDir(), "authority"), workspace, strings.Repeat("a", 64))
				if err != nil {
					t.Fatal(err)
				}
				defer authority.Close()
				original := &controllerProvider{}
				c := &Controller{Rt: &runtime.Runtime{Provider: "openai", Model: "old", LLM: original}, Authority: authority}
				c.BuildProfileProvider = func(config.ModelProfile) (llm.LLMProvider, error) { return &controllerProvider{}, nil }
				if err = c.ConfigureProfiles(map[string]config.ModelProfile{"new": {Provider: "openai", Model: "new"}}, ""); err != nil {
					t.Fatal(err)
				}
				if failure == "nil-commit" {
					c.PreparePermissions = func(config.ModelProfile) (func(), error) { return nil, nil }
				}
				if mode == "model" {
					err = c.SwitchModel("openai/new")
				} else {
					err = c.SwitchProfile("new")
				}
				if err == nil {
					t.Fatal("missing permission commit accepted")
				}
				if c.CurrentModel() != "openai/old" || c.Rt.LLM != original || c.PermissionState().Authority != authority {
					t.Fatal("failed switch changed live provider or authority")
				}
			})
		}
	}
}

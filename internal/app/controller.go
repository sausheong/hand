package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/hand/internal/sessionio"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sausheong/harness/compaction"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/process"
	"github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
)

// Controller gives the TUI's slash commands (/model, /new, /compact)
// access to state that lives outside the Model: the active *runtime.Runtime
// and its session store. BuildProvider is supplied by the caller (main.go
// knows about provider packages and API keys; tui deliberately doesn't) and
// is only invoked when /model names a different provider than the one
// currently active.
type Controller struct {
	SessionApproval     *SessionApproval
	extensionsActivated bool           // protected by the shared operation reservation
	Extensions          *ExtensionHost // fixed before serving clients; creator owns shutdown
	verificationConfig  *config.VerificationConfig
	ExecutionBoundary   string                                    // immutable effective execution scope
	PermissionProfile   config.ModelProfile                       // initial routing identity; subsequent changes under mu
	ReadPermissions     func() PermissionState                    // fixed before use
	PreparePermissions  func(config.ModelProfile) (func(), error) // prepare before committing runtime fields
	LegacyPermissions   permissions.LegacyMigration               // immutable startup proposal; never automatic authority
	Authority           *permissions.Authority                    // fixed before use; lifetime owned by application creator
	OptionalMCP         *OptionalMCP
	// OutputStore is configured before use; nil selects the built-in per-user store.
	Processes            *Processes // application lifetime; configured before use
	OutputStore          *process.ArtifactStore
	profiles             map[string]config.ModelProfile
	activeProfile        string
	summarizerSelection  *SummarizerView
	BuildProfileProvider func(config.ModelProfile) (llm.LLMProvider, error)
	mu                   sync.RWMutex
	ownerOnce            sync.Once
	Owner                *Service // set before first use; shared by configuration and turn operations
	Rt                   *runtime.Runtime
	Sessions             *sessionio.Manager
	sessionWarning       string
	Store                *session.Store
	SessionKey           string
	BaseURL              string

	BuildProvider func(providerName, baseURL string) (llm.LLMProvider, error)

	// BuildModelIdentityHint returns the per-turn "you are running as
	// model X" text (see agentio.ModelIdentityHint) for a given
	// "provider/model" string. main.go supplies this; tui deliberately
	// doesn't know how that text is worded, same reasoning as
	// BuildProvider above.
	BuildModelIdentityHint func(providerModel string) string
}

// CurrentModel returns the active "provider/model" string.
func (c *Controller) CurrentModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Rt.Provider + "/" + c.Rt.Model
}

// SwitchModel points subsequent turns at a different provider/model.
// Switching to a different provider rebuilds the LLM client via
// BuildProvider; switching within the same provider just updates the
// model name. Either way, it also keeps two other Runtime fields that
// are otherwise silently pinned to hand's startup provider/model in
// sync: the compaction summarizer (agentio.BuildCompactionManager
// deliberately reuses the active run's own provider/model, an invariant
// this must preserve across a switch too) and FallbackModel (a bare
// model id meaningful only against the provider it was resolved for).
func (c *Controller) SwitchModel(providerModel string) error {
	_, release, err := c.owner().reserve(context.Background(), Idle)
	if err != nil {
		return err
	}
	defer release()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return fmt.Errorf("background compaction is still active; retry model switch after it finishes")
	}
	providerName, modelName := llm.ParseProviderModel(providerModel)
	if providerName == "" || modelName == "" {
		return fmt.Errorf("model must be in provider/model form")
	}
	providerChanged := providerName != c.Rt.Provider
	provider := c.Rt.LLM
	endpoint := c.BaseURL
	fallback := c.Rt.FallbackModel
	if providerChanged {
		if c.BuildProvider == nil {
			return fmt.Errorf("switching provider is not supported in this build")
		}
		// An endpoint belongs to its provider configuration. Never send the new
		// provider's credential to the previous provider's custom endpoint.
		endpoint = ""
		provider, err = c.BuildProvider(providerName, endpoint)
		if err != nil {
			return err
		}
		if provider == nil {
			return fmt.Errorf("provider factory returned no provider")
		}
		fallback = ""
	}
	hint := c.Rt.DynamicIdentityHint
	if c.BuildModelIdentityHint != nil {
		hint = c.BuildModelIdentityHint(providerModel)
	}
	permissionProfile := config.ModelProfile{Provider: providerName, Model: modelName, Endpoint: endpoint}
	if !providerChanged {
		permissionProfile.CredentialEnv = c.PermissionProfile.CredentialEnv
	}
	commitPermissions, err := c.preparePermissions(permissionProfile)
	if err != nil {
		return err
	}
	// Everything that can fail was prepared before committing these fields.
	commitPermissions()
	c.PermissionProfile = permissionProfile
	c.Rt.LLM = provider
	c.Rt.Provider = providerName
	c.owner().mu.Lock()
	c.owner().options.InputTypes = nil
	c.owner().mu.Unlock()
	c.activeProfile = ""
	c.Rt.Model = modelName
	c.Rt.ContextWindow = 0
	c.Rt.Reasoning = llm.ReasoningOff
	c.Rt.FallbackModel = fallback
	c.Rt.DynamicIdentityHint = hint
	c.owner().mu.Lock()
	c.owner().options.Model = runtimeModelInfo(c.Rt, config.ModelProfile{})
	c.owner().mu.Unlock()
	c.Rt.Route = providerRoute(providerName, endpoint)
	c.BaseURL = endpoint
	if c.summarizerSelection == nil && c.Rt.Compaction != nil && c.Rt.Compaction.Summarizer != nil {
		if providerChanged {
			c.Rt.Compaction.Summarizer.Provider = provider
		}
		c.Rt.Compaction.Summarizer.Route = c.Rt.Route
		c.Rt.Compaction.Summarizer.Model = modelName
	}
	return nil
}

// NewSession selects a new durable session while preserving all old history.
func (c *Controller) NewSession() error { return c.NewSessionContext(context.Background()) }

func (c *Controller) NewSessionContext(ctx context.Context) error {
	_, release, err := c.owner().reserve(ctx, Idle)
	if err != nil {
		return err
	}
	defer release()
	previousSession := c.SessionID()
	defer c.observeSessionSelection(ctx, previousSession)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.Rt == nil {
		return fmt.Errorf("runtime unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return fmt.Errorf("background compaction is still active; retry session change after it finishes")
	}
	old := c.Rt.Session
	if old != nil && old.PersistenceError() != nil {
		return old.PersistenceError()
	}
	if old != nil && len(old.Entries()) > 0 {
		if err := old.Flush(); err != nil {
			return fmt.Errorf("save current session: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var next *session.Session
	var key string
	if c.Sessions != nil {
		selected, err := c.Sessions.Create(ctx, "")
		if err != nil {
			return err
		}
		next = selected.Session
		key = selected.Record.StoreKey
	} else {
		// Compatibility for internal embedders without a workspace manager. A new
		// backend key still preserves the prior file; production supplies Sessions.
		if c.Store == nil {
			return fmt.Errorf("session store unavailable")
		}
		key = session.NewSession(c.Rt.AgentID, "").ID
		if err := c.Store.Create(c.Rt.AgentID, key); err != nil {
			return err
		}
		next, err = c.Store.LoadExclusive(c.Rt.AgentID, key)
		if err != nil {
			return err
		}
	}
	c.Rt.Session = next
	c.SessionKey = key
	c.sessionWarning = ""
	c.owner().mu.Lock()
	c.owner().options.SessionID = next.ID
	c.owner().mu.Unlock()
	if old != nil {
		if err := old.Close(); err != nil {
			c.sessionWarning = "new session selected; previous writer cleanup failed: " + err.Error()
		}
	}
	return nil
}

func (c *Controller) SessionWarning() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionWarning
}

// Compact forces a compaction pass on the active session
// (compaction.ReasonManual), bypassing the normal token/turn threshold.
// Result.Skipped explains why nothing happened (e.g. "no_summarizer" if
// this build has no compaction manager configured, "too_short" if there
// aren't enough turns yet to compact).
func (c *Controller) Compact(ctx context.Context) (compaction.Result, error) {
	return c.CompactWithFocus(ctx, "")
}

func (c *Controller) CompactWithFocus(ctx context.Context, focus string) (compaction.Result, error) {
	if len(focus) > 4096 || !utf8.ValidString(focus) || strings.ContainsRune(focus, 0) {
		return compaction.Result{}, errors.New("compaction focus must be at most 4096 UTF-8 bytes without NUL")
	}
	operation, release, err := c.owner().reserve(ctx, Compacting)
	if err != nil {
		return compaction.Result{}, err
	}
	defer release()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil {
		return compaction.Result{}, errors.New("runtime unavailable")
	}
	if c.Rt.Compaction.HasInFlight(c.Rt.Session) {
		return compaction.Result{}, fmt.Errorf("background compaction is still active")
	}
	if c.Rt.Compaction == nil || c.Rt.Compaction.Summarizer == nil {
		return c.Rt.Compaction.MaybeCompact(operation, c.Rt.Session, compaction.ReasonManual, focus)
	}
	operation, runCancel, _, err := (&HarnessBackend{Runtime: c.Rt}).PrepareSelectedRunBudget(operation)
	if err != nil {
		return compaction.Result{}, err
	}
	defer runCancel()
	operation, deadlineCancel, err := c.Rt.SessionDeadlineContext(operation)
	if err != nil {
		return compaction.Result{}, err
	}
	defer deadlineCancel()
	operation, err = c.Rt.CompactionContext(operation)
	if err != nil {
		return compaction.Result{}, err
	}
	operation, usageError := observeSessionUsage(operation, c.Rt.Session)
	if err := usageError(); err != nil {
		return compaction.Result{}, err
	}
	result, err := c.Rt.Compaction.MaybeCompact(operation, c.Rt.Session, compaction.ReasonManual, focus)
	return result, errors.Join(err, usageError(), context.Cause(operation))
}

func (c *Controller) owner() *Service {
	c.ownerOnce.Do(func() {
		if c.Owner == nil {
			c.Owner = New(nil, Options{})
		}
	})
	return c.Owner
}
func (c *Controller) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Rt == nil || c.Rt.Session == nil {
		return ""
	}
	return c.Rt.Session.ID
}

package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sausheong/hand/internal/agentio"
	"github.com/sausheong/hand/internal/app"
	"github.com/sausheong/hand/internal/checkpoints"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/isolation"
	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/hand/internal/rpc"
	"github.com/sausheong/hand/internal/sessionio"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/process"
	"github.com/sausheong/harness/runtime"
)

// EmbeddedOptions explicitly selects the workspace, durable store and model.
// No user config is implicitly loaded. Workspace instructions and skills follow
// Hand's normal discovery rules. Tool mutations use the shared approval broker;
// clients answer through approval.respond. CredentialEnv names an environment
// variable, never a credential value.
// ExecutionOptions explicitly selects host or pinned container execution.
type ExecutionOptions = config.ExecutionConfig

type CheckpointOptions = checkpoints.Options
type VerificationOptions = config.VerificationConfig
type VerificationProfile = config.VerificationProfile

type EmbeddedOptions struct {
	Verification *VerificationOptions
	Checkpoints  CheckpointOptions
	// CheckpointDirectory enables bounded run capture in private external storage.
	CheckpointDirectory string
	Execution           ExecutionOptions
	// AuthorityDirectory opts into scoped approvals using private storage outside
	// the workspace. Empty retains the existing approval mode during migration.
	AuthorityDirectory string
	Workspace          string
	StoreDirectory     string
	Model              string
	Endpoint           string
	CredentialEnv      string
	SessionID          string
	NewSession         bool
	MaxTurns           int
	MaxIterations      int
}

// Open embeds Hand's runtime, application service and RPC dispatcher in this
// process. The returned client has the same wire contract as StartProcess.
// Close cancels and joins runs, background processes and owned session resources.
// ctx owns the entire lifetime. The SDK does not expose mutable runtime pointers.
func Open(ctx context.Context, options EmbeddedOptions) (client *Client, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	checkpointLimits, checkpointStoreLimits, checkpointErr := options.Checkpoints.Resolve()
	if checkpointErr != nil {
		return nil, checkpointErr
	}
	if options.Verification != nil {
		if options.CheckpointDirectory == "" {
			return nil, errors.New("verification requires checkpoint directory")
		}
		copied, e := options.Verification.ValidatedCopy()
		if e != nil {
			return nil, e
		}
		options.Verification = &copied
	}
	if options.Workspace == "" || options.StoreDirectory == "" {
		return nil, errors.New("workspace and store directory are required")
	}
	workspace, err := filepath.Abs(options.Workspace)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace must be a directory")
	}
	root, err := filepath.Abs(options.StoreDirectory)
	if err != nil {
		return nil, err
	}
	providerName, model := llm.ParseProviderModel(options.Model)
	profile := config.ModelProfile{Provider: providerName, Model: model, Endpoint: options.Endpoint, CredentialEnv: options.CredentialEnv}
	if profile.CredentialEnv == "" {
		profile.CredentialEnv = config.DefaultCredentialEnv(providerName)
	}
	if err = profile.Validate(); err != nil {
		return nil, err
	}
	if options.MaxTurns < 0 || options.MaxIterations < 0 {
		return nil, errors.New("run limits cannot be negative")
	}
	if options.MaxTurns == 0 {
		options.MaxTurns = 50
	}
	if options.MaxIterations == 0 {
		options.MaxIterations = 1
	}
	if options.Execution.Backend == "container" && options.AuthorityDirectory == "" {
		return nil, errors.New("container SDK execution requires an external authority directory")
	}
	probeCtx, probeCancel := context.WithTimeout(ctx, 15*time.Second)
	executionBackend, executionBoundary, err := isolation.Prepare(probeCtx, config.Config{Execution: options.Execution}, workspace)
	probeCancel()
	if err != nil {
		return nil, err
	}
	provider, err := app.BuildProfileProvider(ctx, profile)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(ctx)
	var cleanup []func() error
	cleanup = append(cleanup, func() error { cancel(); return nil })
	closeResources := func() error {
		var result error
		for i := len(cleanup) - 1; i >= 0; i-- {
			result = errors.Join(result, cleanup[i]())
		}
		return result
	}
	defer func() {
		if err != nil {
			cancel()
			closeResources()
		}
	}()
	ledger, err := rpc.OpenWorkspaceLedger(root, workspace)
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, ledger.Close)
	skills, skillStore, err := agentio.BuildSkillProvider(workspace)
	if err != nil {
		return nil, err
	}
	artifacts, err := process.NewArtifactStore(filepath.Join(root, "output"))
	if err != nil {
		return nil, err
	}
	var background *app.Processes
	if executionBackend != nil {
		background, err = app.NewProcessesWithBackend(lifetime, workspace, artifacts, executionBackend)
	} else {
		background, err = app.NewProcesses(lifetime, workspace, artifacts)
	}
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, func() error { background.Close(); return nil })
	registry := agentio.BuildRegistry(workspace, skillStore)
	registry.Register(&app.ProcessTool{Processes: background})
	perms := permissions.NewEmptyStore(permissions.DefaultPath(workspace))
	var reason string
	var scopedAuthority *permissions.Authority
	approvalHook := app.NewApprovalHook(perms, workspace, nil, nil)
	if options.AuthorityDirectory != "" {
		configuration, _ := json.Marshal(struct {
			Schema, Model, Endpoint, CredentialEnv string
			MaxTurns, MaxIterations                int
			Execution                              ExecutionOptions
		}{"embedded-scoped-v2", options.Model, options.Endpoint, profile.CredentialEnv, options.MaxTurns, options.MaxIterations, options.Execution})
		sum := sha256.Sum256(configuration)
		digest := hex.EncodeToString(sum[:])
		authority, e := permissions.OpenAuthority(options.AuthorityDirectory, workspace, digest)
		if e != nil {
			return nil, e
		}
		scopedAuthority = authority
		cleanup = append(cleanup, authority.Close)
		approvalHook = app.NewScopedApprovalHook(authority, workspace, digest, nil)
	}
	hooks := agentio.BuildLifecycleHooks(nil, workspace, approvalHook, &reason)
	spec := agentio.BuildAgentSpec(options.Model, workspace, options.MaxTurns, "", nil, hooks)
	manager, err := sessionio.NewManager(root, workspace, spec.ID)
	if err != nil {
		return nil, err
	}
	selected, err := manager.Open(lifetime, options.SessionID, options.NewSession)
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, selected.Session.Close)
	rt, err := runtime.BuildRuntimeContext(lifetime, runtime.RuntimeDeps{Skills: skills}, runtime.RuntimeInputs{Provider: provider, Tools: registry, Session: selected.Session, Compaction: agentio.BuildCompactionManager(provider, model, config.DefaultCompactionThreshold)}, spec)
	if err != nil {
		return nil, err
	}
	cleanup = append(cleanup, rt.Close)
	if err = isolation.Install(registry, executionBackend, workspace); err != nil {
		return nil, err
	}
	identityHint := agentio.ModelIdentityHint
	if executionBackend != nil {
		identityHint = func(name string) string {
			return agentio.ModelIdentityHint(name) + "\nExecution boundary: " + executionBoundary + ". Shell commands run in /workspace. Use workspace-relative file/search paths and shell commands."
		}
	}
	rt.DynamicIdentityHint = identityHint(options.Model)
	owner := app.NewHarness(rt, &reason, nil, workspace, options.MaxIterations, profile)
	if options.CheckpointDirectory != "" {
		checkpointStore, openErr := checkpoints.OpenStore(options.CheckpointDirectory, workspace, checkpointStoreLimits)
		if openErr != nil {
			return nil, openErr
		}
		cleanup = append(cleanup, checkpointStore.Close)
		boundary := &app.WorkspaceCheckpoints{Workspace: workspace, Limits: checkpointLimits, Store: checkpointStore, Processes: background}
		if err = owner.ConfigureCheckpoints(boundary); err != nil {
			return nil, err
		}
	}

	controller := &app.Controller{ExecutionBoundary: executionBoundary, Authority: scopedAuthority, Owner: owner, Rt: rt, Sessions: manager, SessionKey: selected.Record.StoreKey, Processes: background, OutputStore: artifacts, BaseURL: options.Endpoint, BuildModelIdentityHint: identityHint}
	if options.Verification != nil {
		if err = controller.ConfigureVerification(ctx, *options.Verification); err != nil {
			return nil, err
		}
	}
	// Session controls can replace the initial session; close the current one too.
	cleanup = append(cleanup, func() error { return rt.Session.Close() })

	dispatcher := rpc.NewControllerDispatcher(controller, ledger)
	server, peer := net.Pipe()
	finished := make(chan struct{})
	conn := &embeddedConnection{Conn: peer, cancel: cancel, done: finished}
	go func() {
		serveErr := rpc.Serve(lifetime, server, dispatcher, rpc.TransportOptions{})
		if lifetime.Err() != nil && (errors.Is(serveErr, context.Canceled) || errors.Is(serveErr, context.DeadlineExceeded) || errors.Is(serveErr, io.ErrClosedPipe)) {
			serveErr = nil
		}
		cancel()
		conn.err = errors.Join(serveErr, closeResources())
		close(finished)
	}()
	return NewClient(conn), nil
}

type embeddedConnection struct {
	net.Conn
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
	err    error
}

func (c *embeddedConnection) Close() error {
	c.once.Do(func() { c.cancel(); c.Conn.Close(); <-c.done })
	return c.err
}

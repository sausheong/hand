package permissions

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Scoped grants are application authority, never project policy input. The
// persistent authority store and approval UI must call Grant only after an
// explicit user decision; project capability requests cannot be imported here.
type Operation string

const (
	FileRead      Operation = "file.read"
	FileWrite     Operation = "file.write"
	SkillWrite    Operation = "skill.write"
	CommandExec   Operation = "command.exec"
	ShellExec     Operation = "shell.exec"
	NetworkAccess Operation = "network.access"
	MCPCall       Operation = "mcp.call"
	LegacyTool    Operation = "legacy.tool"
	ToolCall      Operation = "tool.call"
)

type Lifetime string

const (
	InvocationGrant Lifetime = "invocation"
	SessionGrant    Lifetime = "session"
	PersistentGrant Lifetime = "persistent"
)

type Scope string

const (
	ExactScope Scope = "exact"
	TreeScope  Scope = "tree"
)

type Provenance string

const (
	UserDecision       Provenance = "user_decision"
	LegacyAcknowledged Provenance = "legacy_acknowledged"
)

// AuthorityContext binds decisions to a canonical workspace and capability
// configuration digest. The caller computes the digest from effective config.
type AuthorityContext struct{ Workspace, ConfigDigest, SessionID, InvocationID string }
type ScopedGrant struct {
	ID                      string
	Operation               Operation
	Scope                   Scope
	Resource                string
	Lifetime                Lifetime
	SessionID, InvocationID string
	Provenance              Provenance
	Workspace, ConfigDigest string
}
type AccessRequest struct {
	Operation Operation
	Resource  string
}

// ScopedPolicy owns immutable grant values and immediate revocation. It is not
// an execution sandbox: a permitted executable can run arbitrary child code.
type ScopedPolicy struct {
	mu                sync.RWMutex
	workspace, digest string
	grants            map[string]ScopedGrant
}

func NewScopedPolicy(workspace, digest string) (*ScopedPolicy, error) {
	canonical, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace must be a directory")
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, err
	}
	if len(digest) != 64 {
		return nil, errors.New("configuration identity must be a SHA-256 digest")
	}
	if _, err = hex.DecodeString(digest); err != nil {
		return nil, err
	}
	return &ScopedPolicy{workspace: canonical, digest: digest, grants: make(map[string]ScopedGrant)}, nil
}
func fileOperation(op Operation) bool { return op == FileRead || op == FileWrite || op == SkillWrite }
func validOperation(op Operation) bool {
	return fileOperation(op) || op == CommandExec || op == ShellExec || op == NetworkAccess || op == MCPCall || op == LegacyTool || op == ToolCall
}

// CanonicalResource resolves existing symlinks, including parents of a new file.
// Execution must still use rooted filesystem operations and stale-preview checks
// to prevent a path changing between approval and mutation.
func CanonicalResource(workspace, path string) (string, error) {
	if path == "" {
		return "", errors.New("empty resource")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	path = filepath.Clean(path)
	var tail []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return filepath.Abs(resolved)
		}
		// Only absent descendants may be added back. Permission errors and symlink
		// loops are not grounds for treating an existing path as a new resource.
		if !os.IsNotExist(err) {
			return "", err
		}
		if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("unresolved symlink resource")
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		path = parent
	}
}
func (p *ScopedPolicy) Grant(g ScopedGrant) error { return p.grant(g, true) }
func (p *ScopedPolicy) grant(g ScopedGrant, resolve bool) error {
	if g.ID == "" || !validOperation(g.Operation) || (g.Scope != ExactScope && g.Scope != TreeScope) {
		return errors.New("invalid grant identity or scope")
	}
	if g.Operation == LegacyTool && g.Provenance != LegacyAcknowledged {
		return errors.New("broad tool grants require legacy acknowledgement")
	}
	if g.Provenance != UserDecision && g.Provenance != LegacyAcknowledged {
		return errors.New("grant requires explicit user authority")
	}
	if g.Workspace != p.workspace || g.ConfigDigest != p.digest {
		return errors.New("grant authority context mismatch")
	}
	switch g.Lifetime {
	case InvocationGrant:
		if g.InvocationID == "" || g.SessionID == "" {
			return errors.New("invocation grant needs session and invocation")
		}
	case SessionGrant:
		if g.SessionID == "" || g.InvocationID != "" {
			return errors.New("invalid session grant")
		}
	case PersistentGrant:
		if g.SessionID != "" || g.InvocationID != "" {
			return errors.New("persistent grant cannot imply narrower lifetime")
		}
	default:
		return errors.New("invalid grant lifetime")
	}
	if g.Scope == TreeScope && !fileOperation(g.Operation) {
		return errors.New("tree scope only applies to filesystem resources")
	}
	var err error
	if fileOperation(g.Operation) {
		if resolve {
			g.Resource, err = CanonicalResource(p.workspace, g.Resource)
		} else if !filepath.IsAbs(g.Resource) || filepath.Clean(g.Resource) != g.Resource {
			return errors.New("saved resource is not canonical")
		}
	} else {
		err = validateExactResource(g.Operation, g.Resource)
	}
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.grants[g.ID]; exists {
		return errors.New("grant ID already exists; revoke before replacing")
	}
	p.grants[g.ID] = g
	return nil
}
func validateExactResource(op Operation, resource string) error {
	if resource == "" {
		return errors.New("empty resource")
	}
	switch op {
	case CommandExec:
		var argv []string
		if json.Unmarshal([]byte(resource), &argv) != nil || len(argv) == 0 || argv[0] == "" {
			return errors.New("command resource must be an exact executable/argument vector")
		}
	case NetworkAccess:
		u, err := url.Parse(resource)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
			return errors.New("network resource must be an explicit HTTP(S) origin")
		}
	case MCPCall:
		var identity []string
		if json.Unmarshal([]byte(resource), &identity) != nil || len(identity) != 2 || identity[0] == "" || identity[1] == "" {
			return errors.New("MCP resource must identify server and tool")
		}
	}
	return nil
}
func (p *ScopedPolicy) Allowed(ctx AuthorityContext, request AccessRequest) bool {
	workspace, err := filepath.EvalSymlinks(ctx.Workspace)
	if err != nil {
		return false
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil || workspace != p.workspace || ctx.ConfigDigest != p.digest {
		return false
	}
	resource := request.Resource
	if fileOperation(request.Operation) {
		resource, err = CanonicalResource(p.workspace, resource)
	} else {
		err = validateExactResource(request.Operation, resource)
	}
	if err != nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, g := range p.grants {
		if g.Operation != request.Operation {
			continue
		}
		if g.Lifetime == SessionGrant && ctx.SessionID != g.SessionID {
			continue
		}
		if g.Lifetime == InvocationGrant && (ctx.SessionID != g.SessionID || ctx.InvocationID != g.InvocationID) {
			continue
		}
		if resource == g.Resource {
			return true
		}
		if g.Scope == TreeScope {
			rel, err := filepath.Rel(g.Resource, resource)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return true
			}
		}
	}
	return false
}
func (p *ScopedPolicy) Revoke(id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.grants[id]
	delete(p.grants, id)
	return ok
}
func (p *ScopedPolicy) Grants() []ScopedGrant {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ScopedGrant, 0, len(p.grants))
	for _, g := range p.grants {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AllowedTool preserves a visibly acknowledged legacy grant's broad per-tool
// meaning. It does not reinterpret a shell grant as a command-prefix sandbox.
func (p *ScopedPolicy) AllowedTool(ctx AuthorityContext, tool string, request AccessRequest) bool {
	return p.Allowed(ctx, request) || p.Allowed(ctx, AccessRequest{Operation: LegacyTool, Resource: tool})
}

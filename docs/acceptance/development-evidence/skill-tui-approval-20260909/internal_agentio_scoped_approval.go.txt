package agentio

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sausheong/hand/internal/permissions"
	"github.com/sausheong/harness/runtime"
)

// ToolAccess derives the resource from executed arguments. Unknown operations
// use their exact tool/input pair, never a guessed read-only classification.
func ToolAccess(workspace, name string, input json.RawMessage, servers []string) (permissions.AccessRequest, error) {
	if len(input) > 64<<10 {
		return permissions.AccessRequest{}, errors.New("tool approval input exceeds 64 KiB")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(input, &object); err != nil || object == nil {
		return permissions.AccessRequest{}, errors.New("tool input must be an object")
	}
	field := func(key string) string { var text string; json.Unmarshal(object[key], &text); return text }
	request := permissions.AccessRequest{}
	switch name {
	case "read_file", "write_file", "edit_file":
		request.Operation = permissions.FileRead
		if name != "read_file" {
			request.Operation = permissions.FileWrite
		}
		resource, err := permissions.CanonicalResource(workspace, field("path"))
		if err != nil {
			return request, err
		}
		request.Resource = resource
	case "bash":
		request.Operation = permissions.ShellExec
		request.Resource = field("command")
		if request.Resource == "" {
			return request, errors.New("missing shell command")
		}
	case "web_fetch":
		u, err := url.Parse(field("url"))
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return request, errors.New("invalid network destination")
		}
		request.Operation = permissions.NetworkAccess
		request.Resource = u.Scheme + "://" + strings.ToLower(u.Host)
	default:
		if server, ok := mcpToolServer(name, servers); ok {
			request.Operation = permissions.MCPCall
			raw, _ := json.Marshal([]string{server, strings.TrimPrefix(name, mcpToolPrefix+server+"__")})
			request.Resource = string(raw)
		} else {
			request.Operation = permissions.ToolCall
			var compact bytes.Buffer
			json.Compact(&compact, input)
			raw, _ := json.Marshal([]string{name, compact.String()})
			request.Resource = string(raw)
		}
	}
	return request, nil
}

// NewScopedApprovalHook uses external authority and exact scopes. Read-only file
// access within the workspace is the default; external reads and all other
// capabilities require a grant or an explicit decision. Redirect/network and
// filesystem execution boundaries must still be enforced by the backend.
func NewScopedApprovalHook(sender Sender, authority *permissions.Authority, workspace, digest string, servers []string) func(context.Context, string, json.RawMessage) (runtime.HookDecision, error) {
	serverNames := append([]string(nil), servers...)
	return func(ctx context.Context, name string, input json.RawMessage) (runtime.HookDecision, error) {
		frozen := bytes.Clone(input)
		access, err := ToolAccess(workspace, name, frozen, serverNames)
		if err != nil {
			return runtime.HookDecision{}, err
		}
		identity := IdentityFromContext(ctx)
		scope := permissions.AuthorityContext{Workspace: workspace, ConfigDigest: digest, SessionID: identity.SessionID, InvocationID: strconv.FormatUint(identity.RunID, 10)}
		if ctx.Err() != nil {
			return runtime.HookDecision{}, ctx.Err()
		}
		if authority != nil && authority.AllowedTool(scope, name, access) {
			return runtime.HookDecision{Allow: true}, nil
		}
		if access.Operation == permissions.FileRead {
			root, err := permissions.CanonicalResource(workspace, ".")
			if err != nil {
				return runtime.HookDecision{}, err
			}
			rel, err := filepath.Rel(root, access.Resource)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return runtime.HookDecision{Allow: true}, nil
			}
		}
		if sender == nil {
			return runtime.HookDecision{Allow: false, Reason: "scoped approval requires a decision"}, nil
		}

		var snapshot approvalSnapshot
		if access.Operation == permissions.FileWrite {
			root, e := permissions.CanonicalResource(workspace, ".")
			if e != nil {
				return runtime.HookDecision{}, e
			}
			rel, e := filepath.Rel(root, access.Resource)
			external := e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
			if external && (authority == nil || !authority.AllowedTool(scope, "read_file", permissions.AccessRequest{Operation: permissions.FileRead, Resource: access.Resource})) {
				return runtime.HookDecision{}, errors.New("external write preview requires read authority for the target")
			}
			snapshot, err = snapshotApprovalFile(ctx, access.Resource)
			if err != nil {
				return runtime.HookDecision{}, err
			}
		}
		request := ApprovalRequest{Identity: identity, Tool: name, Input: bytes.Clone(frozen), Preview: fmt.Sprintf("Operation: %s\nScope: %s\nArguments: %s", access.Operation, access.Resource, frozen), Respond: make(chan Decision, 1)}
		if access.Operation == permissions.FileWrite {
			request.Preview += fmt.Sprintf("\nBefore: exists=%t mode=%s bytes=%d sha256=%s", snapshot.Exists, snapshot.Mode, snapshot.Size, snapshot.Digest)
		}
		sender.Send(request)
		select {
		case <-ctx.Done():
			return runtime.HookDecision{}, ctx.Err()
		case decision := <-request.Respond:
			if ctx.Err() != nil {
				return runtime.HookDecision{}, ctx.Err()
			}
			current, err := ToolAccess(workspace, name, input, serverNames)
			if err != nil || current != access || !bytes.Equal(input, frozen) {
				return runtime.HookDecision{}, errors.New("tool arguments or resolved resource changed during approval")
			}
			if decision != DecisionOnce && decision != DecisionAlways {
				return runtime.HookDecision{Allow: false, Reason: "user denied scoped operation"}, nil
			}
			if access.Operation == permissions.FileWrite {
				currentSnapshot, e := snapshotApprovalFile(ctx, access.Resource)
				if e != nil {
					return runtime.HookDecision{}, e
				}
				if currentSnapshot != snapshot {
					return runtime.HookDecision{}, errors.New("file contents or permissions changed during approval; review again")
				}
			}

			if decision == DecisionAlways {
				if authority == nil {
					return runtime.HookDecision{}, errors.New("persistent approval authority unavailable")
				}
				root, err := permissions.CanonicalResource(workspace, ".")
				if err != nil {
					return runtime.HookDecision{}, err
				}
				g := permissions.ScopedGrant{ID: rand.Text(), Operation: access.Operation, Scope: permissions.ExactScope, Resource: access.Resource, Lifetime: permissions.PersistentGrant, Provenance: permissions.UserDecision, Workspace: root, ConfigDigest: digest}
				if err = authority.Grant(g); err != nil {
					return runtime.HookDecision{}, err
				}
			}
			return runtime.HookDecision{Allow: true}, nil
		}
	}
}

package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type permissionTask struct {
	done chan struct{}
	text string
}
type permissionCommandDone struct{ task *permissionTask }

// Explicit terminal-user control only; model requests cannot invoke this path.
// Even inspection runs off the UI thread because authority serialises with fsync.
func (m *Model) runPermissionCommand(args []string) tea.Cmd {
	report := func(s string) tea.Cmd {
		m.appendNotice(sanitizeForTerminal(s), "toolCallStyle")
		m.refreshViewport()
		return nil
	}
	if m.controller == nil || m.controller.PermissionState().Authority == nil {
		return report("Scoped permissions are unavailable")
	}
	if m.permissionTask != nil {
		return report("A permission operation is pending")
	}
	offset, revoke := 0, ""
	legacy, acknowledge := false, ""
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "legacy":
		legacy = true
	case len(args) == 2 && args[0] == "acknowledge":
		acknowledge = args[1]
	case len(args) == 2 && args[0] == "revoke" && args[1] != "":
		revoke = args[1]
	case len(args) == 1:
		var err error
		offset, err = strconv.Atoi(args[0])
		if err != nil || offset < 0 {
			return report("Usage: /permissions [offset] | revoke <ID> | legacy | acknowledge <fingerprint>")
		}
	default:
		return report("Usage: /permissions [offset] | revoke <ID> | legacy | acknowledge <fingerprint>")
	}
	state := m.controller.PermissionState()
	authority := state.Authority
	proposal := state.Legacy
	task := &permissionTask{done: make(chan struct{})}
	m.permissionTask = task
	go func() {
		defer close(task.done)
		if legacy {
			grants := proposal.Grants()
			if len(grants) == 0 {
				task.text = "No legacy permission proposal"
				return
			}
			var b strings.Builder
			b.WriteString("Legacy proposal: broad persistent per-tool authority, including all arguments and destinations for each named tool. Reviewing does not import grants.\n")
			fmt.Fprintf(&b, "Workspace: %s\nConfiguration: %s\n", grants[0].Workspace, grants[0].ConfigDigest)
			for _, g := range grants {
				fmt.Fprintf(&b, "Tool: %q\n", g.Resource)
			}
			fmt.Fprintf(&b, "To explicitly acknowledge this exact proposal: /permissions acknowledge %s", proposal.Fingerprint())
			task.text = b.String()
			return
		}
		if acknowledge != "" {
			if len(proposal.Grants()) == 0 {
				task.text = "No legacy permission proposal"
				return
			}
			if err := authority.AcknowledgeLegacy(proposal, acknowledge); err != nil {
				task.text = "Legacy acknowledgement failed: " + err.Error()
			} else {
				task.text = "Acknowledged legacy tool grants; inspect with /permissions and revoke any grant by ID"
			}
			return
		}
		if revoke != "" {
			if err := authority.Revoke(revoke); err != nil {
				task.text = "Permission revocation failed: " + err.Error()
			} else {
				task.text = "Revoked permission " + revoke + "; already executing work is unaffected"
			}
			return
		}
		grants := authority.Grants()
		if offset > len(grants) {
			task.text = "Permission offset exceeds grant count"
			return
		}
		end := offset + 16
		if end < offset || end > len(grants) {
			end = len(grants)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Scoped permissions: %d grants\n", len(grants))
		if len(proposal.Grants()) > 0 {
			b.WriteString("Legacy proposal available: /permissions legacy (review does not grant authority)\n")
		}
		for _, g := range grants[offset:end] {
			resource := g.Resource
			if len(resource) > 2048 {
				resource = resource[:2048] + "… (truncated; inspect with --permissions)"
			}
			fmt.Fprintf(&b, "%s: %s %s %s [%s; %s]\n", permissionLabel(g.ID), g.Operation, g.Scope, resource, g.Lifetime, g.Provenance)
		}
		if end < len(grants) {
			fmt.Fprintf(&b, "Next page: /permissions %d", end)
		}
		task.text = b.String()
	}()
	return func() tea.Msg { <-task.done; return permissionCommandDone{task} }
}
func (m *Model) finishPermissionCommand(msg permissionCommandDone) tea.Cmd {
	if msg.task != m.permissionTask || msg.task == nil {
		return nil
	}
	<-msg.task.done
	m.permissionTask = nil
	m.appendNotice(sanitizeForTerminal(msg.task.text), "toolCallStyle")
	m.refreshViewport()
	return nil
}

// Durability operations cannot be interrupted safely; join before authority closes.
func (m *Model) closePermissions() {
	if m.permissionTask != nil {
		<-m.permissionTask.done
		m.permissionTask = nil
	}
}

func permissionLabel(s string) string {
	if len(s) > 512 {
		return s[:512] + "… (truncated; inspect with --permissions)"
	}
	return s
}

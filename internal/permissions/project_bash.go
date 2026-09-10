package permissions

import "errors"

// AllowProjectBash must be called only for an explicit terminal-user command.
// It binds broad shell approval to this authority's workspace and configuration.
// A stable ID makes repeated commands idempotent and permits regrant after revoke.
func (a *Authority) AllowProjectBash() (ScopedGrant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.poison != nil {
		return ScopedGrant{}, errors.New("approval authority unavailable")
	}
	g := ScopedGrant{ID: "project-bash", Operation: ShellExec, Scope: AllScope,
		Resource: "*", Lifetime: PersistentGrant, Provenance: UserDecision,
		Workspace: a.policy.workspace, ConfigDigest: a.policy.digest}
	for _, existing := range a.policy.Grants() {
		if existing == g {
			return g, nil
		}
	}
	return g, a.grantLocked(g)
}

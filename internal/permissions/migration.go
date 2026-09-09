package permissions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// LegacyMigration is a reviewable proposal, not authority. Its fingerprint binds
// acknowledgement to the exact tool list, canonical workspace and config.
type LegacyMigration struct {
	fingerprint string
	grants      []ScopedGrant
}

func (m LegacyMigration) Fingerprint() string   { return m.fingerprint }
func (m LegacyMigration) Grants() []ScopedGrant { return append([]ScopedGrant(nil), m.grants...) }
func PrepareLegacyMigration(settings Settings, workspace, digest string) (LegacyMigration, error) {
	p, err := NewScopedPolicy(workspace, digest)
	if err != nil {
		return LegacyMigration{}, err
	}
	tools := append([]string(nil), settings.AlwaysAllow...)
	sort.Strings(tools)
	unique := tools[:0]
	for _, name := range tools {
		if strings.TrimSpace(name) != name || name == "" || len(name) > 256 {
			return LegacyMigration{}, errors.New("invalid legacy tool name")
		}
		if len(unique) == 0 || unique[len(unique)-1] != name {
			unique = append(unique, name)
		}
	}
	if len(unique) > 256 {
		return LegacyMigration{}, errors.New("too many legacy tool grants")
	}
	raw, _ := json.Marshal(struct {
		Workspace, Config string
		Tools             []string
	}{p.workspace, digest, unique})
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	proposal := LegacyMigration{fingerprint: fingerprint}
	for _, name := range unique {
		id := sha256.Sum256([]byte(fingerprint + "\x00" + name))
		proposal.grants = append(proposal.grants, ScopedGrant{ID: "legacy-" + hex.EncodeToString(id[:]), Workspace: p.workspace, ConfigDigest: digest, Operation: LegacyTool, Scope: ExactScope, Resource: name, Lifetime: PersistentGrant, Provenance: LegacyAcknowledged})
	}
	return proposal, nil
}

// AcknowledgeLegacy imports only the reviewed proposal. The caller must obtain
// the user's acknowledgement of its broad meaning before calling this method.
// Each grant is durable; interruption can leave a subset imported. Repeating the
// same acknowledged proposal resumes without duplicate grants.
func (a *Authority) AcknowledgeLegacy(proposal LegacyMigration, acknowledgedFingerprint string) error {
	if proposal.fingerprint == "" || acknowledgedFingerprint != proposal.fingerprint {
		return errors.New("legacy migration requires acknowledgement of the reviewed fingerprint")
	}
	for _, g := range proposal.grants {
		existing := false
		for _, old := range a.Grants() {
			if old.ID == g.ID {
				if old != g {
					return errors.New("legacy grant identity conflict")
				}
				existing = true
			}
		}
		if !existing {
			if err := a.Grant(g); err != nil {
				return err
			}
		}
	}
	return nil
}
func (a *Authority) AllowedTool(ctx AuthorityContext, tool string, request AccessRequest) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.closed && a.poison == nil && a.policy.AllowedTool(ctx, tool, request)
}

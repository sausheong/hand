package permissions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const MaxAuthorityBytes = 8 << 20
const MaxAuthorityRecordBytes = 64 << 10

// Authority owns the private append-only grant journal and its exclusive lock.
// It must live outside the workspace. This is not isolation from arbitrary host
// code running as the same OS user; the execution backend must enforce that.
type Authority struct {
	mu     sync.Mutex
	file   *os.File
	policy *ScopedPolicy
	size   int64
	closed bool
	poison error
}
type authorityRecord struct {
	Version int          `json:"version"`
	Grant   *ScopedGrant `json:"grant,omitempty"`
	Revoke  string       `json:"revoke,omitempty"`
}

func OpenAuthority(directory, workspace, digest string) (*Authority, error) {
	policy, err := NewScopedPolicy(workspace, digest)
	if err != nil {
		return nil, err
	}
	canonical, err := CanonicalResource(policy.workspace, directory)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(policy.workspace, canonical)
	if err != nil {
		return nil, err
	}
	if rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !bytes.HasPrefix([]byte(rel), []byte(".."+string(filepath.Separator)))) {
		return nil, errors.New("approval authority must be outside the workspace")
	}
	if err = os.MkdirAll(canonical, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(canonical)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("authority directory must be private")
	}
	f, err := openAuthorityFile(filepath.Join(canonical, "grants.jsonl"))
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			f.Close()
		}
	}()
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() || stat.Mode().Perm()&0077 != 0 || stat.Size() > MaxAuthorityBytes {
		return nil, errors.New("invalid authority file type, permissions or size")
	}
	if stat.Size() > 0 {
		var last [1]byte
		if _, err = f.ReadAt(last[:], stat.Size()-1); err != nil || last[0] != '\n' {
			return nil, errors.New("truncated authority journal")
		}
	}
	// Sync the directory entry before acknowledging authority held in this file.
	dir, err := os.Open(canonical)
	if err != nil {
		return nil, err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), MaxAuthorityRecordBytes)
	for scanner.Scan() {
		if err = validateAuthorityJSON(scanner.Bytes()); err != nil {
			return nil, err
		}
		var record authorityRecord
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&record); err != nil {
			return nil, err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return nil, errors.New("trailing authority data")
		}
		if err = applyAuthority(policy, record, false); err != nil {
			return nil, err
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	// Reconstruct revocations before consulting live filesystem state. A path
	// belonging only to a revoked grant cannot block unrelated authority.
	for _, g := range policy.Grants() {
		if fileOperation(g.Operation) {
			resolved, err := CanonicalResource(policy.workspace, g.Resource)
			if err != nil || resolved != g.Resource {
				return nil, errors.New("saved resource changed; reapproval required")
			}
		}
	}
	success = true
	return &Authority{file: f, policy: policy, size: stat.Size()}, nil
}
func applyAuthority(p *ScopedPolicy, r authorityRecord, resolve bool) error {
	if r.Version != 1 || (r.Grant == nil) == (r.Revoke == "") {
		return errors.New("invalid authority record")
	}
	if r.Grant != nil {
		g := *r.Grant
		if g.Lifetime != PersistentGrant {
			return errors.New("only persistent grants belong on disk")
		}
		if resolve && fileOperation(g.Operation) {
			resolved, err := CanonicalResource(p.workspace, g.Resource)
			if err != nil || resolved != g.Resource {
				return errors.New("saved resource changed; reapproval required")
			}
		}
		return p.grant(g, resolve)
	}
	if !p.Revoke(r.Revoke) {
		return errors.New("authority revocation references unknown grant")
	}
	return nil
}
func (a *Authority) append(r authorityRecord) error {
	if a.closed || a.poison != nil {
		return errors.New("authority unavailable")
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if len(b) >= MaxAuthorityRecordBytes || a.size+int64(len(b)) > MaxAuthorityBytes {
		return errors.New("authority capacity reached")
	}
	n, err := a.file.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = a.file.Sync()
	}
	if err != nil {
		a.poison = err
		return err
	}
	a.size += int64(n)
	return nil
}
func (a *Authority) Grant(g ScopedGrant) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.grantLocked(g)
}

func (a *Authority) grantLocked(g ScopedGrant) error {
	if g.Lifetime != PersistentGrant {
		return errors.New("durable authority accepts persistent grants only")
	}
	// Validate and canonicalise without changing live authority before fsync.
	candidate, err := NewScopedPolicy(a.policy.workspace, a.policy.digest)
	if err != nil {
		return err
	}
	for _, old := range a.policy.Grants() {
		if err = applyAuthority(candidate, authorityRecord{Version: 1, Grant: &old}, true); err != nil {
			return err
		}
	}
	if err = candidate.Grant(g); err != nil {
		return err
	}
	for _, stored := range candidate.Grants() {
		if stored.ID == g.ID {
			g = stored
			break
		}
	}
	if err = a.append(authorityRecord{Version: 1, Grant: &g}); err != nil {
		return err
	}
	a.policy = candidate
	return nil
}
func (a *Authority) Revoke(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for _, g := range a.policy.Grants() {
		if g.ID == id {
			found = true
		}
	}
	if !found {
		return errors.New("unknown grant")
	}
	if err := a.append(authorityRecord{Version: 1, Revoke: id}); err != nil {
		return err
	}
	a.policy.Revoke(id)
	return nil
}
func (a *Authority) Allowed(ctx AuthorityContext, r AccessRequest) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.closed && a.poison == nil && a.policy.Allowed(ctx, r)
}
func (a *Authority) Grants() []ScopedGrant {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.policy.Grants()
}
func (a *Authority) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	return a.file.Close()
}

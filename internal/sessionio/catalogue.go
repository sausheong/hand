package sessionio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxCatalogueBytes = 4 << 20
const MaxCatalogueSessions = 10000

var ErrCatalogueBusy = errors.New("workspace session catalogue is being updated")

// SessionRecord refers to an already-created durable backend session. The
// application must create/validate that session before registering it here.
type SessionRecord struct {
	ID         string    `json:"id"`
	AgentID    string    `json:"agentId"`
	StoreKey   string    `json:"storeKey"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
	LastActive time.Time `json:"lastActive"`
}

type CatalogueSnapshot struct {
	Version      int             `json:"version"`
	Workspace    string          `json:"workspace"`
	Generation   uint64          `json:"generation"`
	LastActiveID string          `json:"lastActiveId,omitempty"`
	Sessions     []SessionRecord `json:"sessions"`
}

type Catalogue struct {
	dir, workspace string
	io             *catalogueIO // immutable after construction; package tests inject durability faults
}

// OpenCatalogue resolves aliases to the same existing workspace directory. It
// does not change the historical KeyForWorkspace hash used for legacy import.
func OpenCatalogue(root, workspace string) (*Catalogue, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	path, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace is not a directory")
	}
	if !utf8.ValidString(path) {
		return nil, errors.New("workspace path is not valid UTF-8")
	}
	// Keep the canonical path in the document to detect hash collisions.
	return &Catalogue{dir: filepath.Join(root, "catalogues", KeyForWorkspace(path)), workspace: path}, nil
}

func (c *Catalogue) Snapshot() (CatalogueSnapshot, error) {
	empty := CatalogueSnapshot{Version: 1, Workspace: c.workspace, Sessions: []SessionRecord{}}
	path := filepath.Join(c.dir, "catalogue.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return empty, nil
	}
	if err != nil {
		return empty, err
	}
	if !info.Mode().IsRegular() {
		return empty, errors.New("catalogue must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return empty, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, MaxCatalogueBytes+1))
	if err != nil {
		return empty, err
	}
	if len(raw) > MaxCatalogueBytes {
		return empty, errors.New("catalogue exceeds size limit")
	}
	var snapshot CatalogueSnapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&snapshot); err != nil {
		return empty, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return empty, errors.New("catalogue contains trailing data")
	}
	if err := c.validate(snapshot); err != nil {
		return empty, err
	}
	return snapshot, nil
}

func validComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !bytes.ContainsAny([]byte(value), "/\\\x00")
}
func validSessionName(name string) bool {
	if len(name) > 256 || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (c *Catalogue) validate(s CatalogueSnapshot) error {
	if s.Version != 1 {
		return fmt.Errorf("unsupported catalogue version %d", s.Version)
	}
	if s.Workspace != c.workspace {
		return errors.New("catalogue workspace identity mismatch")
	}
	if len(s.Sessions) > MaxCatalogueSessions {
		return errors.New("catalogue exceeds session limit")
	}
	ids := make(map[string]bool)
	keys := make(map[string]bool)
	for _, entry := range s.Sessions {
		if !validComponent(entry.ID) || !validComponent(entry.AgentID) || !validComponent(entry.StoreKey) || !validSessionName(entry.Name) || entry.CreatedAt.IsZero() {
			return errors.New("invalid catalogue session record")
		}
		key := entry.AgentID + "\x00" + entry.StoreKey
		if ids[entry.ID] || keys[key] {
			return errors.New("duplicate catalogue session identity or storage key")
		}
		ids[entry.ID] = true
		keys[key] = true
	}
	if s.LastActiveID != "" && !ids[s.LastActiveID] {
		return errors.New("catalogue active session is missing")
	}
	return nil
}

// Register adds a durable session without deleting or replacing prior sessions.
// Re-registering the same ID and storage binding is idempotent; existing names
// and creation times are preserved. Backend creation is outside this lock.
func (c *Catalogue) Register(record SessionRecord, activate bool) error {
	return c.update(func(s *CatalogueSnapshot) error {
		found := false
		for _, entry := range s.Sessions {
			if entry.ID == record.ID {
				if entry.AgentID != record.AgentID || entry.StoreKey != record.StoreKey {
					return errors.New("session identity already bound to another backend")
				}
				found = true
			}
		}
		if !found {
			s.Sessions = append(s.Sessions, record)
		}
		if activate {
			return selectSession(s, record.ID)
		}
		return nil
	})
}
func (c *Catalogue) Select(id string) error {
	return c.update(func(s *CatalogueSnapshot) error { return selectSession(s, id) })
}
func selectSession(s *CatalogueSnapshot, id string) error {
	for i := range s.Sessions {
		if s.Sessions[i].ID == id {
			s.LastActiveID = id
			s.Sessions[i].LastActive = time.Now().UTC()
			return nil
		}
	}
	return errors.New("session is not registered in this workspace")
}
func (c *Catalogue) Rename(id, name string) error {
	if !validSessionName(name) {
		return errors.New("invalid session name")
	}
	return c.update(func(s *CatalogueSnapshot) error {
		for i := range s.Sessions {
			if s.Sessions[i].ID == id {
				s.Sessions[i].Name = name
				return nil
			}
		}
		return errors.New("session is not registered in this workspace")
	})
}

func (c *Catalogue) update(change func(*CatalogueSnapshot) error) error {
	ops := c.operations()
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return err
	}
	lock, err := lockCatalogue(filepath.Join(c.dir, "catalogue.lock"))
	if err != nil {
		return err
	}
	defer lock.Close()
	snapshot, err := c.Snapshot()
	if err != nil {
		return err
	}
	before, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if err := change(&snapshot); err != nil {
		return err
	}
	if err := c.validate(snapshot); err != nil {
		return err
	}
	after, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		// A previous rename may have succeeded while its directory sync failed.
		// Idempotent retry must re-establish durability before reporting success.
		file, err := os.Open(filepath.Join(c.dir, "catalogue.json"))
		if err != nil {
			return err
		}
		if err := errors.Join(ops.syncFile(file), file.Close()); err != nil {
			return err
		}
		return ops.syncDirectory(c.dir)
	}
	if snapshot.Generation == ^uint64(0) {
		return errors.New("catalogue generation exhausted")
	}
	snapshot.Generation++
	raw, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if len(raw) > MaxCatalogueBytes {
		return errors.New("catalogue exceeds size limit")
	}
	file, err := os.CreateTemp(c.dir, ".catalogue-*")
	if err != nil {
		return err
	}
	defer func() { file.Close(); os.Remove(file.Name()) }()
	if n, err := file.Write(raw); err != nil {
		return err
	} else if n != len(raw) {
		return io.ErrShortWrite
	}
	if err := ops.syncFile(file); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ops.rename(file.Name(), filepath.Join(c.dir, "catalogue.json")); err != nil {
		return err
	}
	return ops.syncDirectory(c.dir)
}

type catalogueIO struct {
	syncFile      func(*os.File) error
	rename        func(string, string) error
	syncDirectory func(string) error
}

func (c *Catalogue) operations() catalogueIO {
	if c.io != nil {
		return *c.io
	}
	return catalogueIO{syncFile: func(f *os.File) error { return f.Sync() }, rename: os.Rename, syncDirectory: func(path string) error {
		dir, err := os.Open(path)
		if err != nil {
			return err
		}
		return errors.Join(dir.Sync(), dir.Close())
	}}
}

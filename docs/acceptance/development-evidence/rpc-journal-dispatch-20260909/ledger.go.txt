package rpc

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/sausheong/hand/protocol"
)

const MaxRequests = 1024
const MaxLedgerBytes = 64 << 20
const MaxRecordBytes = protocol.MaxFrameBytes + 4096

type RequestRecord struct {
	Kind        string          `json:"kind,omitempty"`
	OperationID string          `json:"operation_id,omitempty"`
	ID          string          `json:"id"`
	Fingerprint string          `json:"fingerprint"`
	SessionID   string          `json:"session_id"`
	RunID       string          `json:"run_id,omitempty"`
	State       string          `json:"state"`
	Result      json.RawMessage `json:"result,omitempty"`
}

// Ledger holds an exclusive writer lock. Intent is synced before Begin grants
// execution. Incomplete records become uncertain after reopening and never
// automatically grant execution again.
type Ledger struct {
	mu      sync.Mutex
	file    *os.File
	records map[string]RequestRecord
	bytes   int64
	closed  bool
	poison  error
}

func OpenLedger(path string) (*Ledger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("request ledger requires a private directory")
	}
	f, err := lockLedger(path)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	for _, directory := range []string{dir, filepath.Dir(dir)} {
		d, e := os.Open(directory)
		if e != nil {
			return nil, e
		}
		e = errors.Join(d.Sync(), d.Close())
		if e != nil {
			return nil, e
		}
	}
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > MaxLedgerBytes {
		return nil, errors.New("invalid request ledger file")
	}
	if info.Size() > 0 {
		var last [1]byte
		if _, err = f.ReadAt(last[:], info.Size()-1); err != nil {
			return nil, err
		}
		if last[0] != '\n' {
			return nil, errors.New("truncated request ledger; reconciliation required")
		}
	}
	l := &Ledger{file: f, records: make(map[string]RequestRecord), bytes: info.Size()}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes+1)
	for scanner.Scan() {
		fields, err := ledgerObject(scanner.Bytes())
		if err != nil {
			return nil, fmt.Errorf("invalid request ledger: %w", err)
		}
		for key := range fields {
			switch key {
			case "kind", "operation_id", "id", "fingerprint", "session_id", "run_id", "state", "result":
			default:
				return nil, errors.New("unknown request ledger field")
			}
		}
		var r RequestRecord
		dec := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		dec.DisallowUnknownFields()
		if err = dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("invalid request ledger: %w", err)
		}
		if dec.Decode(new(any)) != io.EOF {
			return nil, errors.New("invalid request ledger record")
		}
		if err = l.validate(r); err != nil {
			return nil, err
		}
		l.records[r.ID] = r
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	for id, r := range l.records {
		if r.State != "completed" {
			r.State = "uncertain"
			l.records[id] = r
		}
	}
	ok = true
	return l, nil
}
func (l *Ledger) validate(r RequestRecord) error {
	if (r.Kind != "" && r.Kind != "control") || (r.Kind == "control" && r.RunID != "") || (r.Kind == "" && r.OperationID != "") || r.ID == "" || len(r.ID) > protocol.MaxRequestIDBytes || r.SessionID == "" || len(r.Fingerprint) != 64 {
		return errors.New("invalid request identity")
	}
	if _, err := hex.DecodeString(r.Fingerprint); err != nil {
		return err
	}
	if r.Kind == "control" && r.State == "completed" {
		if err := validateControlResponse(r); err != nil {
			return err
		}
	}

	old, exists := l.records[r.ID]
	if !exists {
		if r.State != "pending" || r.RunID != "" || r.OperationID != "" || len(r.Result) != 0 || len(l.records) >= MaxRequests {
			return errors.New("invalid initial request record")
		}
		return nil
	}
	if old.Kind != r.Kind || old.Fingerprint != r.Fingerprint || old.SessionID != r.SessionID {
		return errors.New("request identity changed")
	}
	if old.State == "pending" && r.State == "accepted" && (r.RunID != "" || r.OperationID != "") && len(r.Result) == 0 {
		return nil
	}
	if old.State == "accepted" && r.State == "completed" && r.RunID == old.RunID && r.OperationID == old.OperationID && json.Valid(r.Result) {
		if r.Kind == "" {
			return validateTerminal(r)
		}
		return nil
	}
	return errors.New("invalid request state transition")
}
func (l *Ledger) append(r RequestRecord) error {
	if l.closed {
		return errors.New("request ledger closed")
	}
	if l.poison != nil {
		return l.poison
	}
	if err := l.validate(r); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if len(b) > MaxRecordBytes || l.bytes+int64(len(b)) > MaxLedgerBytes {
		return errors.New("request ledger capacity reached")
	}
	n, err := l.file.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = l.file.Sync()
	}
	if err != nil {
		l.poison = err
		return err
	}
	l.bytes += int64(len(b))
	l.records[r.ID] = r
	return nil
}
func cloneRecord(r RequestRecord) RequestRecord { r.Result = bytes.Clone(r.Result); return r }
func (l *Ledger) Begin(request protocol.Request, sessionID string) (RequestRecord, bool, error) {
	return l.begin(request, sessionID, "")
}
func (l *Ledger) BeginControl(request protocol.Request, sessionID string) (RequestRecord, bool, error) {
	return l.begin(request, sessionID, "control")
}
func (l *Ledger) begin(request protocol.Request, sessionID, kind string) (RequestRecord, bool, error) {
	if len(request.Params) == 0 {
		request.Params = json.RawMessage(`{}`)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return RequestRecord{}, false, err
	}
	if request, err = protocol.DecodeRequest(encoded); err != nil {
		return RequestRecord{}, false, err
	}
	var compact bytes.Buffer
	if err = json.Compact(&compact, request.Params); err != nil {
		return RequestRecord{}, false, err
	}
	digest := sha256.Sum256([]byte(request.Method + "\x00" + sessionID + "\x00" + compact.String()))
	fingerprint := hex.EncodeToString(digest[:])
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.poison != nil {
		return RequestRecord{}, false, errors.New("request ledger unavailable")
	}
	if old, ok := l.records[request.ID]; ok {
		if old.Kind != kind || old.Fingerprint != fingerprint {
			return RequestRecord{}, false, errors.New("request ID reused with different operation")
		}
		return cloneRecord(old), false, nil
	}
	r := RequestRecord{Kind: kind, ID: request.ID, Fingerprint: fingerprint, SessionID: sessionID, State: "pending"}
	if err = l.append(r); err != nil {
		return RequestRecord{}, false, err
	}
	return r, true, nil
}
func (l *Ledger) Bind(id, runID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.records[id]
	if !ok {
		return errors.New("unknown request")
	}
	r.State = "accepted"
	if r.Kind == "control" {
		r.OperationID = runID
	} else {
		r.RunID = runID
	}
	return l.append(r)
}
func (l *Ledger) Complete(id string, result json.RawMessage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.records[id]
	if !ok {
		return errors.New("unknown request")
	}
	r.State = "completed"
	r.Result = bytes.Clone(result)
	return l.append(r)
}
func (l *Ledger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	return l.file.Close()
}

func (l *Ledger) Lookup(id string) (RequestRecord, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return RequestRecord{}, errors.New("request ledger closed")
	}
	r, ok := l.records[id]
	if !ok {
		return RequestRecord{}, errors.New("unknown request")
	}
	return cloneRecord(r), nil
}

// unavailable distinguishes storage failure from an ordinary request-ID
// conflict. Only storage failure enables best-effort emergency cancellation.
func (l *Ledger) unavailable() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed || l.poison != nil
}

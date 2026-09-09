package extensions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/sausheong/hand/extension/protocol"
	"github.com/sausheong/harness/session"
)

const MaxStateBytes = 16 << 10
const MaxStateRevisions = 256

type State struct {
	Revision int             `json:"revision"`
	Data     json.RawMessage `json:"data"`
}
type stateRecord struct {
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}
type StateStore struct {
	session *session.Session
	kind    string
}

// NewStateStore binds storage to a host-supplied stable package identity and the
// selected session. Extension input cannot choose another namespace. Package
// activation must bind this identity independently of self-reported peer names.
func NewStateStore(sess *session.Session, identity string) (*StateStore, error) {
	if sess == nil || strings.TrimSpace(identity) == "" || len(identity) > 256 || !utf8.ValidString(identity) || strings.ContainsRune(identity, 0) {
		return nil, errors.New("invalid extension state owner")
	}
	digest := sha256.Sum256([]byte(identity))
	return &StateStore{session: sess, kind: "hand.extension.state." + hex.EncodeToString(digest[:])}, nil
}
func validateState(data json.RawMessage) error {
	if len(data) > MaxStateBytes {
		return errors.New("extension state exceeds 16 KiB")
	}
	var object map[string]json.RawMessage
	return protocol.DecodePayload(data, &object)
}
func (s *StateStore) Get(ctx context.Context) (State, error) {
	out := State{Data: json.RawMessage(`{}`)}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := s.session.PersistenceError(); err != nil {
		return out, err
	}
	records := s.session.Annotations(s.kind)
	out.Revision = len(records)
	if len(records) == 0 {
		return out, nil
	}
	if len(records) > MaxStateRevisions {
		return out, errors.New("extension state revision limit exceeded")
	}
	raw := records[len(records)-1].Payload
	if len(raw) > MaxStateBytes+128 {
		return out, errors.New("extension state record exceeds limit")
	}
	var record stateRecord
	if err := protocol.DecodePayload(raw, &record); err != nil {
		return out, err
	}
	if record.Version != 1 {
		return out, errors.New("unsupported extension state version")
	}
	if err := validateState(record.Data); err != nil {
		return out, err
	}
	out.Data = append(json.RawMessage(nil), record.Data...)
	return out, nil
}
func (s *StateStore) Set(ctx context.Context, revision int, data json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateState(data); err != nil {
		return err
	}
	current, err := s.Get(ctx)
	if err != nil {
		return err
	}
	if revision != current.Revision {
		return session.ErrAnnotationConflict
	}
	if current.Revision >= MaxStateRevisions {
		return errors.New("extension state revision limit reached; retain journal and migrate explicitly")
	}
	raw, err := json.Marshal(stateRecord{Version: 1, Data: data})
	if err != nil {
		return err
	}
	var encoded stateRecord
	if err = protocol.DecodePayload(raw, &encoded); err != nil {
		return err
	}
	if err = validateState(encoded.Data); err != nil {
		return err
	}
	if len(raw) > MaxStateBytes+128 {
		return errors.New("encoded extension state exceeds limit")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return s.session.AnnotateIfCount(s.kind, raw, revision)
}

// Handle services only state operations. It grants no resource capabilities and
// must be reached through the connection's negotiated capability checks.
func (s *StateStore) Handle(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *protocol.Error) {
	fail := func(code string, err error) (json.RawMessage, *protocol.Error) {
		return nil, &protocol.Error{Code: code, Message: err.Error()}
	}
	switch method {
	case "state.get":
		var p struct{}
		if err := protocol.DecodePayload(params, &p); err != nil {
			return fail("invalid_params", err)
		}
		state, err := s.Get(ctx)
		if err != nil {
			return fail("state_unavailable", err)
		}
		raw, err := json.Marshal(state)
		if err != nil {
			return fail("state_unavailable", err)
		}
		return raw, nil
	case "state.set":
		var p struct {
			Revision *int            `json:"revision"`
			Data     json.RawMessage `json:"data"`
		}
		if err := protocol.DecodePayload(params, &p); err != nil {
			return fail("invalid_params", err)
		}
		if p.Revision == nil || *p.Revision < 0 {
			return fail("invalid_params", errors.New("nonnegative state revision is required"))
		}
		if err := s.Set(ctx, *p.Revision, p.Data); err != nil {
			code := "state_rejected"
			if errors.Is(err, session.ErrAnnotationConflict) {
				code = "state_conflict"
			}
			return fail(code, err)
		}
		raw, _ := json.Marshal(struct {
			Revision int `json:"revision"`
		}{*p.Revision + 1})
		return raw, nil
	default:
		return fail("capability_denied", errors.New("state handler cannot access host resources"))
	}
}

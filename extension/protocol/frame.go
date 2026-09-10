// Package protocol defines the extension JSONL protocol, independently of MCP
// and Hand's client RPC. Process ownership and capabilities belong to the host.
package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

const Version = 1
const MaxFrameBytes = 256 << 10

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Frame struct {
	Version int             `json:"version"`
	Kind    string          `json:"kind"`
	ID      string          `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

func identifier(s string) bool {
	return len(s) > 0 && len(s) <= 64 && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-')
	}) < 0
}
func (f Frame) Validate() error {
	if f.Version != Version || !identifier(f.ID) {
		return errors.New("unsupported extension version or invalid ID")
	}
	switch f.Kind {
	case "request":
		if !identifier(f.Method) || len(f.Result) > 0 || f.Error != nil || len(f.Params) == 0 {
			return errors.New("invalid extension request")
		}
		p := bytes.TrimSpace(f.Params)
		if len(p) == 0 || p[0] != '{' || !json.Valid(p) {
			return errors.New("extension params must be an object")
		}
	case "response":
		if f.Method != "" || len(f.Params) > 0 || (len(f.Result) > 0) == (f.Error != nil) {
			return errors.New("extension response requires exactly one result or error")
		}
		if f.Error != nil && (!identifier(f.Error.Code) || len(f.Error.Message) > 4096 || !utf8.ValidString(f.Error.Message)) {
			return errors.New("invalid extension error")
		}
		if len(f.Result) > 0 && !json.Valid(f.Result) {
			return errors.New("invalid extension result")
		}
	default:
		return errors.New("unknown extension frame kind")
	}
	return nil
}

// Reader stops permanently after malformed, truncated or oversized frames. The
// host must tear down that peer; it must not reinterpret trailing bytes as calls.
type Reader struct {
	r      *bufio.Reader
	failed error
}

func NewReader(r io.Reader) *Reader { return &Reader{r: bufio.NewReaderSize(r, MaxFrameBytes+1)} }
func (r *Reader) Read() (Frame, error) {
	var f Frame
	if r.failed != nil {
		return f, r.failed
	}
	raw, err := r.r.ReadSlice('\n')
	if err == io.EOF && len(raw) == 0 {
		return f, io.EOF
	}
	if err != nil || len(raw) > MaxFrameBytes+1 {
		r.failed = errors.New("extension frame is oversized or lacks newline")
		return f, r.failed
	}
	raw = raw[:len(raw)-1]
	if !utf8.Valid(raw) {
		r.failed = errors.New("extension frame is not UTF-8")
		return f, r.failed
	}
	if err = uniqueJSON(raw); err == nil {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		err = dec.Decode(&f)
		if err == nil {
			err = f.Validate()
		}
	}
	if err != nil {
		r.failed = err
	}
	return f, err
}

// uniqueJSON rejects ambiguous duplicate keys at every level and bounds nesting.
func uniqueJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return errors.New("extension JSON nesting exceeds 32")
		}
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return err
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return errors.New("duplicate extension JSON key")
				}
				seen[s] = true
				if err = value(depth + 1); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return err
			}
			if end != json.Delim('}') {
				return errors.New("invalid object")
			}
		case '[':
			for dec.More() {
				if err = value(depth + 1); err != nil {
					return err
				}
			}
			end, err := dec.Token()
			if err != nil {
				return err
			}
			if end != json.Delim(']') {
				return errors.New("invalid array")
			}
		default:
			return errors.New("unexpected delimiter")
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("trailing extension JSON")
	}
	return nil
}

type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }
func (w *Writer) Write(f Frame) error {
	if err := f.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	if len(raw) > MaxFrameBytes {
		return fmt.Errorf("extension frame exceeds %d bytes", MaxFrameBytes)
	}
	if err = uniqueJSON(raw); err != nil {
		return err
	}
	raw = append(raw, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.w.Write(raw)
	if err == nil && n != len(raw) {
		return io.ErrShortWrite
	}
	return err
}

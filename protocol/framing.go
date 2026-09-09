package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

var ErrFrameTooLarge = &Error{Code: "message_too_large", Message: "JSONL frame exceeds 1048576 bytes"}

// Reader reads newline-terminated messages. EOF between messages is normal;
// EOF within a message is an error. Oversized messages are fatal to the stream:
// callers must close it rather than retry from the middle of a rejected frame.
type Reader struct{ reader *bufio.Reader }

func NewReader(r io.Reader) *Reader { return &Reader{reader: bufio.NewReaderSize(r, 4096)} }
func (r *Reader) ReadFrame() ([]byte, error) {
	var frame []byte
	for {
		part, err := r.reader.ReadSlice('\n')
		complete := len(part) > 0 && part[len(part)-1] == '\n'
		if complete {
			part = part[:len(part)-1]
		}
		if len(frame)+len(part) > MaxFrameBytes {
			return nil, ErrFrameTooLarge
		}
		frame = append(frame, part...)
		if complete {
			if !utf8.Valid(frame) {
				return nil, &Error{Code: "invalid_message", Message: "message must be UTF-8"}
			}
			return frame, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) && len(frame) > 0 {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
}

// DecodeRequest rejects duplicate envelope fields so dispatch never depends
// on whether a client's parser uses the first or last occurrence of an ID.
func DecodeRequest(frame []byte) (Request, error) {
	bad := func(message string) (Request, error) {
		return Request{}, &Error{Code: "invalid_request", Message: message}
	}
	if len(frame) > MaxFrameBytes {
		return Request{}, ErrFrameTooLarge
	}
	if !utf8.Valid(frame) {
		return bad("request must be UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(frame))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return bad("request must be an object")
	}
	seen := make(map[string]bool)
	var r Request
	for dec.More() {
		token, err = dec.Token()
		if err != nil {
			return bad("invalid request object")
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return bad("duplicate or invalid request field")
		}
		seen[key] = true
		switch key {
		case "version":
			err = dec.Decode(&r.Version)
		case "id":
			err = dec.Decode(&r.ID)
		case "method":
			err = dec.Decode(&r.Method)
		case "params":
			err = dec.Decode(&r.Params)
		default:
			return bad("unknown request field")
		}
		if err != nil {
			return bad("invalid request field type")
		}
	}
	if _, err = dec.Token(); err != nil {
		return bad("unterminated request object")
	}
	if dec.Decode(new(any)) != io.EOF {
		return bad("expected one request object")
	}
	if !seen["version"] {
		return bad("version is required")
	}
	if r.Version != Version {
		return Request{}, &Error{Code: "unsupported_version", Message: "supported protocol version is 1"}
	}
	if r.ID == "" || len(r.ID) > MaxRequestIDBytes || strings.IndexFunc(r.ID, unicode.IsControl) >= 0 {
		return bad("id must be 1..128 bytes without control characters")
	}
	if r.Method == "" || len(r.Method) > 64 || strings.IndexFunc(r.Method, func(c rune) bool { return !(c >= 'a' && c <= 'z' || c == '.' || c == '_') }) >= 0 {
		return bad("invalid method name")
	}
	if len(r.Params) == 0 {
		r.Params = json.RawMessage(`{}`)
	}
	if p := bytes.TrimSpace(r.Params); len(p) == 0 || p[0] != '{' {
		return bad("params must be an object")
	}
	return r, nil
}

// Writer serializes concurrent frame writes. A write error poisons the stream:
// sending another frame after a partial write would corrupt JSONL boundaries.
// Cancellation of blocked I/O is the transport owner's responsibility (Close).
type Writer struct {
	mu     sync.Mutex
	writer io.Writer
	err    error
}

func NewWriter(w io.Writer) *Writer { return &Writer{writer: w} }
func (w *Writer) Write(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > MaxFrameBytes {
		return ErrFrameTooLarge
	}
	data = append(data, '\n')
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	n, err := w.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return err
}

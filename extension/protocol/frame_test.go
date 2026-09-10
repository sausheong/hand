package protocol

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestFramesAndTerminalFailures(t *testing.T) {
	valid := Frame{Version: 1, Kind: "request", ID: "host-1", Method: "initialize", Params: json.RawMessage(`{}`)}
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(valid); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(Frame{Version: 1, Kind: "response", ID: "host-1", Result: json.RawMessage(`{"ready":true}`)}); err != nil {
		t.Fatal(err)
	}
	r := NewReader(&buf)
	for _, kind := range []string{"request", "response"} {
		f, err := r.Read()
		if err != nil || f.Kind != kind {
			t.Fatal(f, err)
		}
	}
	if _, err := r.Read(); err != io.EOF {
		t.Fatal(err)
	}
	bad := []string{
		`{"version":1,"version":1,"kind":"request","id":"a","method":"x","params":{}}`,
		`{"version":1,"kind":"request","id":"a","method":"x","params":{"nested":{"x":1,"x":2}}}`,
		`{"version":2,"kind":"request","id":"a","method":"x","params":{}}`,
		`{"version":1,"kind":"response","id":"a","result":null,"error":{"code":"bad","message":"x"}}`,
		`{"version":1,"kind":"request","id":"a","method":"x","params":{},"extra":1}`,
		strings.Repeat("x", MaxFrameBytes+1), string([]byte{0xff}),
	}
	for _, input := range bad {
		r := NewReader(strings.NewReader(input + "\n" + `{"version":1,"kind":"request","id":"a","method":"x","params":{}}` + "\n"))
		if _, err := r.Read(); err == nil {
			t.Fatal("bad frame accepted")
		}
		if _, err := r.Read(); err == nil {
			t.Fatal("failed stream resumed")
		}
	}
	r = NewReader(strings.NewReader(`{"version":1}`))
	if _, err := r.Read(); err == nil {
		t.Fatal("missing newline accepted")
	}
}
func TestWriterRejectsAmbiguousPayloadAndShortWrite(t *testing.T) {
	f := Frame{Version: 1, Kind: "request", ID: "a", Method: "x", Params: json.RawMessage(`{"x":1,"x":2}`)}
	var b bytes.Buffer
	if err := NewWriter(&b).Write(f); err == nil || b.Len() != 0 {
		t.Fatal("ambiguous outgoing payload written")
	}
	f.Params = json.RawMessage(`{}`)
	if err := NewWriter(shortWriter{}).Write(f); err != io.ErrShortWrite {
		t.Fatal(err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func FuzzFrameReader(f *testing.F) {
	f.Add([]byte("{\"version\":1,\"kind\":\"request\",\"id\":\"a\",\"method\":\"initialize\",\"params\":{}}\n"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > MaxFrameBytes+2 {
			return
		}
		r := NewReader(bytes.NewReader(b))
		frame, err := r.Read()
		if err == nil {
			var out bytes.Buffer
			encoded, marshalErr := json.Marshal(frame)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			err = NewWriter(&out).Write(frame)
			if len(encoded) > MaxFrameBytes {
				if err == nil || out.Len() != 0 {
					t.Fatal("oversized re-encoding was written")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = NewReader(&out).Read(); err != nil {
				t.Fatalf("writer output rejected: %v", err)
			}
		}
	})
}

// JSON's HTML escaping can expand an accepted input beyond the output bound.
// The reader and writer enforce their own byte limits, not equal wire lengths.
func TestFrameReencodingExpansionIsRejectedBeforeWrite(t *testing.T) {
	input := `{"version":1,"kind":"request","id":"a","method":"x","params":{"text":"` + strings.Repeat("<", MaxFrameBytes/3) + `"}}` + "\n"
	frame, err := NewReader(strings.NewReader(input)).Read()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(frame)
	if err != nil || len(encoded) <= MaxFrameBytes {
		t.Fatal("fixture does not cross encoded limit", err)
	}
	var out bytes.Buffer
	if err := NewWriter(&out).Write(frame); err == nil || out.Len() != 0 {
		t.Fatal("expanded frame was partly written", err, out.Len())
	}
}

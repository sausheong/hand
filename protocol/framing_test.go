package protocol

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestRequestFramingAndValidation(t *testing.T) {
	reader := NewReader(strings.NewReader("{\"version\":1,\"id\":\"r1\",\"method\":\"session.select\",\"params\":{}}\n"))
	frame, err := reader.ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	req, err := DecodeRequest(frame)
	if err != nil || req.ID != "r1" || req.Method != "session.select" {
		t.Fatalf("request %+v %v", req, err)
	}
	if _, err = reader.ReadFrame(); err != io.EOF {
		t.Fatalf("EOF: %v", err)
	}
	for _, input := range []string{
		`{"version":1,"id":"a","id":"b","method":"prompt"}`,
		`{"version":1,"id":"a","method":"prompt","extra":true}`,
		`{"version":1,"id":"a","method":"prompt","params":null}`,
		`{"version":1,"id":"a","method":"prompt"} {}`,
		`{"version":1,"id":"a\n","method":"prompt"}`,
		`{"id":"a","method":"prompt"}`,
		`[]`,
	} {
		if _, err := DecodeRequest([]byte(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	_, err = DecodeRequest([]byte(`{"version":2,"id":"a","method":"prompt"}`))
	var pe *Error
	if !errors.As(err, &pe) || pe.Code != "unsupported_version" {
		t.Fatalf("version error %v", err)
	}
}
func TestFrameLimitsAndTruncatedDisconnect(t *testing.T) {
	for _, size := range []int{MaxFrameBytes, MaxFrameBytes + 1} {
		frame, err := NewReader(strings.NewReader(strings.Repeat("x", size) + "\n")).ReadFrame()
		if size == MaxFrameBytes {
			if err != nil || len(frame) != size {
				t.Fatalf("boundary %d %v", len(frame), err)
			}
		} else if !errors.Is(err, ErrFrameTooLarge) {
			t.Fatalf("oversize %v", err)
		}
	}
	if _, err := NewReader(strings.NewReader(`{"version":1}`)).ReadFrame(); err != io.ErrUnexpectedEOF {
		t.Fatalf("truncated frame %v", err)
	}
	if _, err := NewReader(bytes.NewReader([]byte{0xff, '\n'})).ReadFrame(); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

type shortWriter struct{ calls int }

func (w *shortWriter) Write(p []byte) (int, error) { w.calls++; return len(p) / 2, nil }
func TestWriterPoisonedAfterPartialFrame(t *testing.T) {
	sink := &shortWriter{}
	writer := NewWriter(sink)
	if err := writer.Write(map[string]int{"value": 1}); err != io.ErrShortWrite {
		t.Fatalf("short write %v", err)
	}
	if err := writer.Write(map[string]int{"value": 2}); err != io.ErrShortWrite || sink.calls != 1 {
		t.Fatal("continued corrupt stream")
	}
}
func TestConcurrentFramesDoNotInterleave(t *testing.T) {
	var sink bytes.Buffer
	writer := NewWriter(&sink)
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			if err := writer.Write(Request{Version: 1, ID: strings.Repeat("x", i+1), Method: "state", Params: []byte(`{}`)}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	workers.Wait()
	reader := NewReader(&sink)
	seen := make(map[string]bool)
	for i := 0; i < 32; i++ {
		frame, err := reader.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		r, err := DecodeRequest(frame)
		if err != nil || seen[r.ID] {
			t.Fatalf("corrupted frame %s %v", frame, err)
		}
		seen[r.ID] = true
	}
	if _, err := reader.ReadFrame(); err != io.EOF {
		t.Fatal(err)
	}
}
func FuzzDecodeRequest(f *testing.F) {
	f.Add([]byte(`{"version":1,"id":"a","method":"prompt","params":{}}`))
	f.Add([]byte(`{"id":"a","id":"b"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		r, err := DecodeRequest(b)
		if err == nil && (r.Version != Version || r.ID == "" || r.Method == "") {
			t.Fatal("accepted invalid envelope")
		}
	})
}

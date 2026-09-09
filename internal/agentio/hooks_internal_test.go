package agentio

import (
	"bytes"
	"testing"
)

func TestHookOutputCaptureIsBounded(t *testing.T) {
	var b hookBuffer
	chunk := bytes.Repeat([]byte("x"), 4096)
	for i := 0; i < 1000; i++ {
		n, err := b.Write(chunk)
		if n != len(chunk) || err != nil {
			t.Fatal("writer did not consume output")
		}
	}
	if b.Len() != hookOutputLimit || !b.truncated {
		t.Fatalf("capture length %d, truncation %v", b.Len(), b.truncated)
	}
}

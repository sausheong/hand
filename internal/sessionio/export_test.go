package sessionio

import (
	"bytes"
	"context"
	"errors"
	"github.com/sausheong/harness/session"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type exportShortWriter struct{}

func (exportShortWriter) Write(p []byte) (int, error) { return 0, nil }

func TestSessionExportCopyLimitsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		total      int64
		record     int
		pass       bool
	}{
		{"exact", "abc\ndef\n", 8, 4, true}, {"record", "abcde\n", 20, 4, false}, {"total", "ab\nab\n", 5, 4, false},
		{"large", strings.Repeat("a", 128<<10), 256 << 10, 128 << 10, true},
		{"large_record", strings.Repeat("a", 128<<10), 256 << 10, 100 << 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := copySessionExport(context.Background(), &out, strings.NewReader(tc.data), tc.total, tc.record)
			if (err == nil) != tc.pass {
				t.Fatal(err)
			}
			if tc.pass && out.String() != tc.data {
				t.Fatal("export altered bytes")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := copySessionExport(ctx, io.Discard, strings.NewReader("x"), 10, 10); err != context.Canceled {
		t.Fatal(err)
	}
	if err := copySessionExport(context.Background(), exportShortWriter{}, strings.NewReader("x"), 10, 10); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

func TestExportBoundaryFailureReportsPublicationAndCleansTemporary(t *testing.T) {
	for _, stage := range []string{"before_publish", "after_publish"} {
		t.Run(stage, func(t *testing.T) {
			m, err := NewManager(t.TempDir(), t.TempDir(), "hand")
			if err != nil {
				t.Fatal(err)
			}
			source, err := m.Create(context.Background(), "source")
			if err != nil {
				t.Fatal(err)
			}
			source.Session.Append(session.UserMessageEntry("preserve"))
			if err := source.Session.Flush(); err != nil {
				t.Fatal(err)
			}
			source.Session.Close()
			failure := errors.New("injected boundary failure")
			m.exportCheckpoint = func(at string) error {
				if at == stage {
					return failure
				}
				return nil
			}
			dir := t.TempDir()
			destination := filepath.Join(dir, "result.jsonl")
			err = m.ExportID(context.Background(), source.Session.ID, destination)
			if !errors.Is(err, failure) {
				t.Fatal("underlying error lost", err)
			}
			files, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			expected := 0
			if stage == "after_publish" {
				expected = 1
				if !strings.Contains(err.Error(), "published") {
					t.Fatal("published destination not reported", err)
				}
			}
			if len(files) != expected {
				t.Fatal("temporary leaked or destination state wrong", files)
			}
			if expected == 1 && files[0].Name() != "result.jsonl" {
				t.Fatal("wrong published file")
			}
			// ExportID must close its writer even when export fails.
			current, err := m.Open(context.Background(), source.Session.ID, false)
			if err != nil {
				t.Fatal("failed export retained writer lease", err)
			}
			current.Session.Close()
		})
	}
}

//go:build darwin || linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/harness/process"
)

func TestProcessFloodRetainsBoundedPrefixesAndArtifacts(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "output")
	store, err := process.NewArtifactStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProcesses(context.Background(), t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	info, err := p.Start(context.Background(), "head -c 20971520 /dev/zero; head -c 20971520 /dev/zero >&2")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := p.Wait(ctx, info.ID)
	if err != nil || result.Running || result.ExitCode != 0 {
		t.Fatalf("flood did not drain: %+v %v", result, err)
	}
	for _, stream := range []struct {
		name, text string
		total      int64
		truncated  bool
		artifact   process.ArtifactInfo
	}{
		{"stdout", result.Stdout, result.StdoutBytes, result.StdoutTruncated, result.StdoutArtifact},
		{"stderr", result.Stderr, result.StderrBytes, result.StderrTruncated, result.StderrArtifact},
	} {
		if len(stream.text) != 64<<10 || stream.total != 20<<20 || !stream.truncated {
			t.Fatalf("%s prefix/count bound: len=%d total=%d truncated=%v", stream.name, len(stream.text), stream.total, stream.truncated)
		}
		if stream.artifact.Bytes != process.ArtifactFileLimit || !stream.artifact.Truncated || stream.artifact.Error != "" {
			t.Fatalf("%s artifact: %+v", stream.name, stream.artifact)
		}
		stat, err := os.Stat(stream.artifact.Path)
		if err != nil || stat.Size() != process.ArtifactFileLimit || stat.Mode().Perm() != 0600 {
			t.Fatalf("%s disk cap/mode: %v %v", stream.name, stat, err)
		}
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var diskBytes int64
	for _, entry := range entries {
		stat, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		diskBytes += stat.Size()
	}
	if diskBytes > process.ArtifactStoreLimit {
		t.Fatalf("store exceeded quota: %d", diskBytes)
	}
	if err = p.Forget(info.ID); err != nil {
		t.Fatal(err)
	}
	if len(p.List()) != 0 {
		t.Fatal("forgotten process retained")
	}
	t.Logf("generated_bytes=%d prefix_bytes_per_stream=%d artifact_bytes_per_stream=%d disk_bytes=%d store_limit=%d retention_hours=%d", 40<<20, 64<<10, process.ArtifactFileLimit, diskBytes, process.ArtifactStoreLimit, int(process.ArtifactRetention.Hours()))
}

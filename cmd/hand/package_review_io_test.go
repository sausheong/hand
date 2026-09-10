//go:build darwin || linux

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	extensionprotocol "github.com/sausheong/hand/extension/protocol"
)

func TestPackageReviewReadsRejectUnsafeFilesAndInvalidPayloads(t *testing.T) {
	for name, raw := range map[string][]byte{
		"truncated":       []byte(`{"name":`),
		"unknown field":   []byte(`{"name":"x","execute":true}`),
		"duplicate field": []byte(`{"name":"x","name":"y"}`),
		"trailing object": []byte(`{"name":"x"} {}`),
		"nonobject":       []byte(`null`),
		"invalid utf8":    []byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 255, '"', '}'},
		"oversized":       []byte(strings.Repeat(" ", extensionprotocol.MaxFrameBytes+1)),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "review.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			var result struct {
				Name string `json:"name"`
			}
			if err := readPackageJSON(path, &result); err == nil {
				t.Fatal("invalid review accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(raw, after) {
				t.Fatal("reader modified evidence", err)
			}
		})
	}
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, []byte(`{"name":"x"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, link, fifo, filepath.Join(dir, "missing")} {
		var result struct {
			Name string `json:"name"`
		}
		if err := readPackageJSON(path, &result); err == nil {
			t.Fatalf("unsafe file accepted: %s", path)
		}
	}
}

func TestPackageReviewWritesArePrivateExclusiveAndBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review.json")
	value := struct {
		Name string `json:"name"`
	}{"reviewed"}
	got, err := writePackageReview(path, value)
	if err != nil || got != path {
		t.Fatal(got, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("review not private", err)
	}
	var decoded struct {
		Name string `json:"name"`
	}
	if err = readPackageJSON(path, &decoded); err != nil || decoded.Name != value.Name {
		t.Fatal(decoded, err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err = os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{path, link} {
		if _, err = writePackageReview(target, map[string]string{"name": "overwrite"}); err == nil {
			t.Fatal("existing review overwritten")
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("existing review content changed", err)
	}
	for name, bad := range map[string]any{"oversized": strings.Repeat("x", extensionprotocol.MaxFrameBytes), "unencodable": make(chan int)} {
		output := filepath.Join(dir, name)
		if _, err = writePackageReview(output, bad); err == nil {
			t.Fatal("invalid review written", name)
		}
		if _, err = os.Lstat(output); !os.IsNotExist(err) {
			t.Fatal("failed review left output", name, err)
		}
	}
}

package packages

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

type archiveTestEntry struct {
	name string
	data []byte
	kind byte
	size int64
}

func buildTestArchive(t *testing.T, format string, entries []archiveTestEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if format == "zip" {
		writer := zip.NewWriter(&buffer)
		for _, entry := range entries {
			header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
			header.SetMode(0600)
			if entry.kind == tar.TypeSymlink {
				header.SetMode(os.ModeSymlink | 0777)
			}
			file, err := writer.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = file.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		size := int64(len(entry.data))
		if entry.size != 0 {
			size = entry.size
		}
		header := &tar.Header{Name: entry.name, Size: size, Mode: 0600, Typeflag: kind}
		if kind == tar.TypeSymlink || kind == tar.TypeLink {
			header.Linkname = "/etc/passwd"
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if kind == tar.TypeReg {
			if _, err := writer.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if format == "tar.gz" || format == "tgz" {
		var compressed bytes.Buffer
		gz := gzip.NewWriter(&compressed)
		gz.Write(buffer.Bytes())
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		return compressed.Bytes()
	}
	return buffer.Bytes()
}
func archiveFixtureEntries(t *testing.T) ([]archiveTestEntry, string) {
	t.Helper()
	source := writeFixture(t)
	_, pin, err := VerifyDirectory(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	var entries []archiveTestEntry
	for _, name := range []string{ManifestName, "note.py"} {
		raw, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, archiveTestEntry{name: name, data: raw})
	}
	return entries, pin
}
func TestArchiveImportMatchesLocalPinAndInstalls(t *testing.T) {
	for _, format := range []string{"zip", "tar", "tar.gz", "tgz"} {
		t.Run(format, func(t *testing.T) {
			entries, pin := archiveFixtureEntries(t)
			if format != "zip" {
				for i := range entries {
					entries[i].name = "./" + entries[i].name
				}
				entries = append([]archiveTestEntry{{name: "./", kind: tar.TypeDir}}, entries...)
			}
			raw := buildTestArchive(t, format, entries)
			sum := sha256.Sum256(raw)
			filename := filepath.Join(t.TempDir(), "package."+format)
			if err := os.WriteFile(filename, raw, 0600); err != nil {
				t.Fatal(err)
			}
			parent := privateStageParent(t)
			snapshot, err := ImportArchive(context.Background(), filename, parent, hex.EncodeToString(sum[:]), pin)
			if err != nil {
				t.Fatal(err)
			}
			defer snapshot.Close()
			if snapshot.Digest() != pin {
				t.Fatal("archive changed package identity")
			}
			store, err := OpenStore(privateStageParent(t), "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			review, err := store.PrepareSnapshot(context.Background(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			approveChange(t, store, review)
			if err = snapshot.Close(); err != nil {
				t.Fatal(err)
			}
			assertStoredOrigin(t, store, Origin{Kind: "archive", Location: filename, Reference: hex.EncodeToString(sum[:])})
			remaining, err := os.ReadDir(parent)
			if err != nil || len(remaining) != 0 {
				t.Fatal("archive temporary files survived", remaining, err)
			}
		})
	}
}
func TestArchiveImportRejectsUnsafeEntriesAndCleansUp(t *testing.T) {
	for _, mode := range []string{"traversal", "absolute", "absolute-directory", "duplicate", "symlink", "hardlink", "unlisted", "bad-gzip-checksum", "trailing-data", "wrong-archive-pin", "wrong-package-pin"} {
		t.Run(mode, func(t *testing.T) {
			entries, pin := archiveFixtureEntries(t)
			format := "tar"
			switch mode {
			case "traversal":
				entries = append(entries, archiveTestEntry{name: "../escape", data: []byte("bad")})
			case "absolute":
				entries = append(entries, archiveTestEntry{name: "/escape", data: []byte("bad")})
			case "absolute-directory":
				entries = append(entries, archiveTestEntry{name: "/", kind: tar.TypeDir})
			case "duplicate":
				entries = append(entries, entries[1])
			case "symlink":
				entries = append(entries, archiveTestEntry{name: "link", kind: tar.TypeSymlink})
			case "hardlink":
				entries = append(entries, archiveTestEntry{name: "link", kind: tar.TypeLink})
			case "unlisted":
				entries = append(entries, archiveTestEntry{name: "extra", data: []byte("bad")})
			case "bad-gzip-checksum":
				format = "tar.gz"
			}
			raw := buildTestArchive(t, format, entries)
			if mode == "bad-gzip-checksum" {
				raw[len(raw)-8] ^= 1
			}
			if mode == "trailing-data" {
				raw = append(raw, []byte("hidden data")...)
			}
			sum := sha256.Sum256(raw)
			archivePin := hex.EncodeToString(sum[:])
			if mode == "wrong-archive-pin" {
				archivePin = pin
			}
			if mode == "wrong-package-pin" {
				pin = archivePin
			}
			file := filepath.Join(t.TempDir(), "bad."+format)
			if err := os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			parent := privateStageParent(t)
			snapshot, err := ImportArchive(context.Background(), file, parent, archivePin, pin)
			if err == nil || snapshot != nil {
				t.Fatal("unsafe archive accepted", snapshot, err)
			}
			remaining, err := os.ReadDir(parent)
			if err != nil || len(remaining) != 0 {
				t.Fatal("rejected archive left content", remaining, err)
			}
		})
	}
}
func TestZIPPreflightBoundsIndexBeforeDecode(t *testing.T) {
	raw := buildTestArchive(t, "zip", nil)
	raw[len(raw)-12] = 0xff
	raw[len(raw)-11] = 0xff
	if err := preflightZIP(bytes.NewReader(raw), int64(len(raw))); err == nil {
		t.Fatal("oversized ZIP index accepted")
	}
}

func TestArchiveRefusesOversizedDeclaredFileBeforeExtraction(t *testing.T) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: "oversized", Mode: 0600, Typeflag: tar.TypeReg, Size: MaxFileBytes + 1}); err != nil {
		t.Fatal(err)
	}
	raw := buffer.Bytes()
	sum := sha256.Sum256(raw)
	file := filepath.Join(t.TempDir(), "oversized.tar")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, pin := archiveFixtureEntries(t)
	parent := privateStageParent(t)
	if snapshot, err := ImportArchive(context.Background(), file, parent, hex.EncodeToString(sum[:]), pin); err == nil || snapshot != nil {
		t.Fatal("oversized entry accepted")
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 0 {
		t.Fatal("oversized archive left output", entries, err)
	}
}

func TestArchiveRejectsGlobalPathOverride(t *testing.T) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	if err := writer.WriteHeader(&tar.Header{Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"path": "../escape"}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buffer.Bytes()
	sum := sha256.Sum256(raw)
	file := filepath.Join(t.TempDir(), "global.tar")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	_, pin := archiveFixtureEntries(t)
	if snapshot, err := ImportArchive(context.Background(), file, privateStageParent(t), hex.EncodeToString(sum[:]), pin); err == nil || snapshot != nil {
		t.Fatal("global path override accepted")
	}
}

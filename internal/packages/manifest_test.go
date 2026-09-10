package packages

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureManifest() Manifest {
	sum := sha256.Sum256([]byte("print('hello')\n"))
	return Manifest{Schema: 1, Name: "task-note", Version: "1.0.0", Compatibility: Compatibility{MinimumHand: "0.1.0", ExtensionProtocol: 1}, Files: []File{{Path: "note.py", Kind: "extension", Size: 15, SHA256: hex.EncodeToString(sum[:])}}, Runtimes: []Runtime{{Name: "python", Command: "python3", MinimumVersion: "3.10.0"}}, Extensions: []Extension{{Name: "note", Entrypoint: "note.py", Runtime: "python", Capabilities: []string{"commands", "questions"}}}}
}
func TestManifestStrictValidationAndDigest(t *testing.T) {
	m := fixtureManifest()
	raw, _ := json.Marshal(m)
	decoded, err := DecodeManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	first, err := decoded.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decoded.Extensions[0].Capabilities = []string{"questions", "commands"}
	second, err := decoded.Digest()
	if err != nil || first != second {
		t.Fatal("capability order changed identity", err)
	}
	if decoded.Extensions[0].Capabilities[0] != "questions" {
		t.Fatal("digest mutated caller")
	}
	decoded.Extensions[0].Capabilities = []string{"commands"}
	changed, _ := decoded.Digest()
	if changed == first {
		t.Fatal("capability change omitted from identity")
	}
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.Schema = 2 }, func(m *Manifest) { m.Name = "../escape" },
		func(m *Manifest) { m.Version = "01.0.0" }, func(m *Manifest) { m.Version = "1.0.0-01" },
		func(m *Manifest) { m.Compatibility.ExtensionProtocol = 2 },
		func(m *Manifest) { m.Files[0].Path = "../note.py" }, func(m *Manifest) { m.Files[0].Path = "/note.py" },
		func(m *Manifest) { m.Files[0].Path = "a\\note.py" }, func(m *Manifest) { m.Files[0].Path = ".git/config" },
		func(m *Manifest) { m.Files[0].Path = "HAND-PACKAGE.JSON" },
		func(m *Manifest) { m.Files[0].Size = -1 }, func(m *Manifest) { m.Files[0].Size = MaxFileBytes + 1 },
		func(m *Manifest) { m.Files[0].SHA256 = strings.Repeat("A", 64) },
		func(m *Manifest) { m.Files[0].Kind = "approval" },
		func(m *Manifest) { m.Files = append(m.Files, m.Files[0]) },
		func(m *Manifest) { f := m.Files[0]; f.Path = "NOTE.py"; m.Files = append(m.Files, f) },
		func(m *Manifest) { f := m.Files[0]; f.Path = "note.py/child"; m.Files = append(m.Files, f) },
		func(m *Manifest) { m.Runtimes[0].Command = "python3 -c" },
		func(m *Manifest) { m.Extensions[0].Runtime = "missing" },
		func(m *Manifest) { m.Extensions[0].Runtime = "" },
		func(m *Manifest) { m.Extensions[0].Entrypoint = "missing" },
		func(m *Manifest) { m.Extensions[0].Capabilities = []string{"unrestricted"} },
		func(m *Manifest) { m.Extensions[0].Arguments = []string{"bad\x00arg"} },
	} {
		var candidate Manifest
		json.Unmarshal(raw, &candidate)
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("accepted invalid manifest %+v", candidate)
		}
	}
	for _, bad := range [][]byte{append([]byte(`{"schema":1,`), raw[1:]...), append(raw[:len(raw)-1], []byte(`,"unknown":true}`)...), []byte(`null`)} {
		if _, err := DecodeManifest(bad); err == nil {
			t.Fatalf("accepted invalid JSON %s", bad)
		}
	}
}
func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	m := fixtureManifest()
	content := []byte("print('hello')\n")
	m.Files[0].Size = int64(len(content))
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, ManifestName), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.py"), content, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestVerifyDirectoryRejectsIntegrityChanges(t *testing.T) {
	for _, mode := range []string{"valid", "changed", "missing", "unlisted", "mode", "symlink", "directory-symlink", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			dir := writeFixture(t)
			file := filepath.Join(dir, "note.py")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var err error
			switch mode {
			case "changed":
				err = os.WriteFile(file, []byte("print('other')\n"), 0600)
			case "missing":
				err = os.Remove(file)
			case "unlisted":
				err = os.WriteFile(filepath.Join(dir, "extra"), []byte("unlisted"), 0600)
			case "mode":
				err = os.Chmod(file, 0700)
			case "symlink":
				if err = os.Remove(file); err == nil {
					err = os.Symlink(filepath.Join(t.TempDir(), "outside"), file)
				}
			case "directory-symlink":
				err = os.Symlink(t.TempDir(), filepath.Join(dir, "linked"))
			case "cancelled":
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			m, digest, err := VerifyDirectory(ctx, dir)
			if mode == "valid" {
				if err != nil || m.Name != "task-note" || len(digest) != 64 {
					t.Fatal(m, digest, err)
				}
			} else if err == nil {
				t.Fatal("invalid directory accepted")
			}
		})
	}
}
func FuzzManifest(f *testing.F) {
	raw, _ := json.Marshal(fixtureManifest())
	f.Add(raw)
	f.Add([]byte(`{"schema":1,"schema":2}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		m, err := DecodeManifest(raw)
		if err != nil {
			return
		}
		digest, err := m.Digest()
		if err != nil || len(digest) != 64 {
			t.Fatal("accepted manifest cannot be identified", err)
		}
		canonical, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		next, err := DecodeManifest(canonical)
		if err != nil {
			t.Fatal("accepted manifest cannot round trip", err)
		}
		second, err := next.Digest()
		if err != nil || second != digest {
			t.Fatal("manifest identity changed", err)
		}
	})
}

func TestShippedPythonPackageIntegrity(t *testing.T) {
	m, digest, err := VerifyDirectory(context.Background(), "../../examples/extensions/python-task-note")
	if err != nil {
		t.Fatal(err)
	}
	if len(digest) != 64 || len(m.Extensions) != 1 || m.Extensions[0].Name != "task-note" || m.Runtimes[0].Command != "python3" {
		t.Fatal("example package metadata mismatch", m)
	}
}

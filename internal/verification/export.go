package verification

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const MaxRecordBytes = 2 << 20

type envelope struct {
	Stdout  []byte `json:"stdout_bytes"`
	Stderr  []byte `json:"stderr_bytes"`
	Version int    `json:"version"`
	Record  View   `json:"record"`
}

func hashID(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func validID(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && s == strings.ToLower(s)
}
func validateView(v View) error {
	if len(v.Command) == 0 || len(v.Command) > 1024 || v.Command[0] == "" || !filepath.IsAbs(v.Workspace) || !validID(v.Before) || !validID(v.ProfileDigest) || (v.After != "" && !validID(v.After)) || v.Started.IsZero() || v.Finished.Before(v.Started) || v.ExitCode < -1 {
		return errors.New("invalid verification record")
	}
	if v.StdoutBytes < int64(len(v.Stdout)) || v.StderrBytes < int64(len(v.Stderr)) || len(v.Stdout) > 64<<10 || len(v.Stderr) > 64<<10 {
		return errors.New("invalid verification output counts")
	}
	return nil
}

// Save writes a content-addressed record to an existing private directory
// outside its workspace. The caller owns retention and the directory. The
// digest detects corruption; it is not a signature from an external authority.
func (r *Record) Save(ctx context.Context, directory string) (string, error) {
	return r.SaveWithLimits(ctx, directory, DefaultRetentionLimits())
}

func (r *Record) SaveWithLimits(ctx context.Context, directory string, limits RetentionLimits) (string, error) {
	if err := limits.Validate(); err != nil {
		return "", err
	}
	if err := validateView(r.view); err != nil {
		return "", err
	}
	view := r.View()
	stdout, stderr := []byte(view.Stdout), []byte(view.Stderr)
	view.Stdout, view.Stderr = "", ""
	b, err := json.Marshal(envelope{Version: 1, Record: view, Stdout: stdout, Stderr: stderr})
	if err != nil {
		return "", err
	}
	if len(b) > MaxRecordBytes {
		return "", errors.New("verification record exceeds 2 MiB")
	}
	dir, err := canonical(directory)
	if err != nil {
		return "", err
	}
	work, err := canonical(r.view.Workspace)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(work, dir)
	if err != nil {
		return "", err
	}
	if rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("verification records must be outside workspace")
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("verification directory must be private")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err = ctx.Err(); err != nil {
		return "", err
	}
	lock, err := lockEvidence(root)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	id := hashID(b)
	if _, err := loadRoot(ctx, root, id); err == nil {
		d, e := root.Open(".")
		if e != nil {
			return "", e
		}
		if e = errors.Join(d.Sync(), d.Close()); e != nil {
			return "", e
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := evidenceCapacity(ctx, root, limits, int64(len(b))); err != nil {
		return "", err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return "", err
	}
	temp := ".verification-" + hex.EncodeToString(nonce[:])
	f, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer root.Remove(temp)
	for left := b; len(left) > 0; {
		if err = ctx.Err(); err != nil {
			f.Close()
			return "", err
		}
		n := len(left)
		if n > 32<<10 {
			n = 32 << 10
		}
		written, e := f.Write(left[:n])
		if e != nil {
			f.Close()
			return "", e
		}
		if written != n {
			f.Close()
			return "", io.ErrShortWrite
		}
		left = left[n:]
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	// Hard-link publication refuses replacement of an existing record.
	if err = root.Link(temp, id+".json"); err != nil {
		if !os.IsExist(err) {
			return "", err
		}
		if _, err = loadRoot(ctx, root, id); err != nil {
			return "", err
		}
	}
	if err = root.Remove(temp); err != nil {
		return "", err
	}
	d, err := root.Open(".")
	if err != nil {
		return "", err
	}
	if err = errors.Join(d.Sync(), d.Close()); err != nil {
		return "", err
	}
	return id, nil
}

// Load verifies a record from a trusted private evidence directory. Loading
// never executes commands or follows artifact references. A caller displaying
// status must also load the referenced snapshots and check current scope.
func Load(ctx context.Context, directory, id string) (*Record, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("verification directory must be private")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return loadRoot(ctx, root, id)
}
func loadRoot(ctx context.Context, root *os.Root, id string) (*Record, error) {
	if !validID(id) {
		return nil, errors.New("invalid verification id")
	}
	f, err := root.OpenFile(id+".json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > MaxRecordBytes {
		return nil, errors.New("invalid verification file")
	}
	var b []byte
	buf := make([]byte, 32<<10)
	reader := io.LimitReader(f, MaxRecordBytes+1)
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		n, e := reader.Read(buf)
		b = append(b, buf[:n]...)
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
	}
	if len(b) > MaxRecordBytes || hashID(b) != id {
		return nil, errors.New("verification integrity mismatch")
	}
	var e envelope
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&e); err != nil {
		return nil, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, errors.New("trailing verification data")
	}
	if e.Version != 1 {
		return nil, errors.New("unsupported verification version")
	}
	if e.Record.Stdout != "" || e.Record.Stderr != "" {
		return nil, errors.New("ambiguous verification output")
	}
	e.Record.Stdout, e.Record.Stderr = string(e.Stdout), string(e.Stderr)
	if err = validateView(e.Record); err != nil {
		return nil, err
	}
	return &Record{view: e.Record}, nil
}

package agentio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
)

const MaxApprovalSnapshotBytes = 16 << 20

type approvalSnapshot struct {
	Exists bool
	Mode   os.FileMode
	Size   int64
	Digest string
}

func snapshotApprovalFile(ctx context.Context, path string) (approvalSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return approvalSnapshot{}, err
	}
	f, err := openApprovalSnapshot(path)
	if os.IsNotExist(err) {
		return approvalSnapshot{}, nil
	}
	if err != nil {
		return approvalSnapshot{}, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return approvalSnapshot{}, err
	}
	if !before.Mode().IsRegular() || before.Size() > MaxApprovalSnapshotBytes {
		return approvalSnapshot{}, errors.New("approval requires a regular file of at most 16 MiB")
	}
	hash := sha256.New()
	reader := io.LimitReader(f, MaxApprovalSnapshotBytes+1)
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err = ctx.Err(); err != nil {
			return approvalSnapshot{}, err
		}
		n, e := reader.Read(buffer)
		if n > 0 {
			hash.Write(buffer[:n])
			total += int64(n)
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return approvalSnapshot{}, e
		}
	}
	after, err := f.Stat()
	if err != nil {
		return approvalSnapshot{}, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		return approvalSnapshot{}, err
	}
	if total > MaxApprovalSnapshotBytes || total != before.Size() || !os.SameFile(before, current) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return approvalSnapshot{}, errors.New("file changed while preparing approval")
	}
	return approvalSnapshot{Exists: true, Mode: before.Mode(), Size: total, Digest: hex.EncodeToString(hash.Sum(nil))}, nil
}

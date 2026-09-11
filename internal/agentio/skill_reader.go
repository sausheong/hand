package agentio

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"syscall"
)

// Skill bodies are loaded on demand, unlike always-included project guidance.
const skillFileLimit = 1 << 20
const skillMetadataLimit = 16 << 10

// readSkill reads only frontmatter during discovery, and the complete body on
// demand. Both paths reject changed files and special files, without weakening
// the separate project-instruction bounds.
func readSkill(path string, original os.FileInfo, full bool) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !os.SameFile(original, info) {
		return nil, fmt.Errorf("skill changed during discovery")
	}
	if info.Size() > skillFileLimit {
		return nil, fmt.Errorf("exceeds 1 MiB skill limit; move supporting material into referenced files")
	}
	limit := int64(skillMetadataLimit)
	if full {
		limit = skillFileLimit + 1
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return nil, fmt.Errorf("skill changed during read")
	}
	if full {
		if len(data) > skillFileLimit || int64(len(data)) != info.Size() {
			return nil, fmt.Errorf("skill could not be read completely within 1 MiB limit")
		}
		return data, nil
	}
	// The index needs only frontmatter, never a prefix of the instructions.
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, nil
	}
	end := bytes.Index(data[4:], []byte("\n---"))
	if end < 0 {
		return nil, fmt.Errorf("skill frontmatter must end within 16 KiB")
	}
	return data[:4+end+4], nil
}

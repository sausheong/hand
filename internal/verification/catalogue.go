package verification

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type RecordPage struct {
	IDs   []string `json:"ids"`
	Next  int      `json:"next"`
	Total int      `json:"total"`
}

func evidenceRoot(directory string) (*os.Root, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("verification directory must be private")
	}
	return os.OpenRoot(directory)
}

// List returns bounded sorted candidate IDs, not verified passing results.
// Call Load and Check before making any assertion about their contents/status.
func List(ctx context.Context, directory string, offset int) (RecordPage, error) {
	if offset < 0 {
		return RecordPage{}, errors.New("invalid verification offset")
	}
	root, err := evidenceRoot(directory)
	if err != nil {
		return RecordPage{}, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return RecordPage{}, err
	}
	defer dir.Close()
	ids := []string{}
	count := 0
	for {
		entries, e := dir.ReadDir(128)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return RecordPage{}, err
			}
			count++
			if count > 10129 {
				return RecordPage{}, errors.New("verification directory inventory exceeds bound")
			}
			name := entry.Name()
			id := strings.TrimSuffix(name, ".json")
			if name == id || !validID(id) {
				continue
			}
			info, err := root.Lstat(name)
			if err != nil {
				return RecordPage{}, err
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
				return RecordPage{}, errors.New("unsafe verification catalogue entry")
			}
			ids = append(ids, id)
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return RecordPage{}, e
		}
	}
	sort.Strings(ids)
	if offset > len(ids) {
		return RecordPage{}, errors.New("invalid verification offset")
	}
	end := min(offset+32, len(ids))
	return RecordPage{IDs: ids[offset:end], Next: end, Total: len(ids)}, nil
}

// Delete removes only the explicitly selected integrity-checked evidence record.
// Snapshot/output references are not deleted. Directory-sync errors report an
// uncertain deletion; callers inspect state rather than assuming no mutation.
func Delete(ctx context.Context, directory, id, workspace string) error {
	root, err := evidenceRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockEvidence(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	record, err := loadRoot(ctx, root, id)
	if err != nil {
		return err
	}
	work, err := canonical(workspace)
	if err != nil {
		return err
	}
	if record.view.Workspace != work {
		return errors.New("verification deletion workspace mismatch")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := root.Remove(id + ".json"); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("verification deletion durability uncertain: %w", err)
	}
	if err := errors.Join(dir.Sync(), dir.Close()); err != nil {
		return fmt.Errorf("verification deletion durability uncertain: %w", err)
	}
	return nil
}

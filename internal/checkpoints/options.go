package checkpoints

import (
	"errors"
	"io/fs"
)

// Options uses zero for defaults. Exclusions are exact workspace-relative paths
// or subtrees, not shell patterns. Resolve copies slices before runtime use.
type Options struct {
	Exclude       []string
	MaxEntries    int
	MaxFileBytes  int64
	MaxTotalBytes int64
	MaxSnapshots  int
	MaxStoreBytes int64
}

func (o Options) Resolve() (Limits, StoreLimits, error) {
	capture, store := DefaultLimits(), DefaultStoreLimits()
	if o.MaxEntries != 0 {
		capture.MaxFiles = o.MaxEntries
	}
	if o.MaxFileBytes != 0 {
		capture.MaxFileBytes = o.MaxFileBytes
	}
	if o.MaxTotalBytes != 0 {
		capture.MaxTotalBytes = o.MaxTotalBytes
	}
	if o.MaxSnapshots != 0 {
		store.MaxSnapshots = o.MaxSnapshots
	}
	if o.MaxStoreBytes != 0 {
		store.MaxBytes = o.MaxStoreBytes
	}
	capture.Exclude = append([]string(nil), o.Exclude...)
	if capture.MaxFiles <= 0 || capture.MaxFiles > 100000 || capture.MaxFileBytes <= 0 || capture.MaxFileBytes > 128<<20 || capture.MaxTotalBytes <= 0 || capture.MaxTotalBytes > 512<<20 || store.MaxSnapshots <= 0 || store.MaxSnapshots > 10000 || store.MaxBytes <= 0 || store.MaxBytes > 8<<30 {
		return Limits{}, StoreLimits{}, errors.New("invalid checkpoint limits")
	}
	if len(capture.Exclude) > 1024 {
		return Limits{}, StoreLimits{}, errors.New("too many checkpoint exclusions")
	}
	for _, p := range capture.Exclude {
		if !fs.ValidPath(p) || p == "." || len(p) > 4096 {
			return Limits{}, StoreLimits{}, errors.New("invalid checkpoint exclusion")
		}
	}
	return capture, store, nil
}

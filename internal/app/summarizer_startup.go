package app

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
)

// SummarizerStartup stores reviewed routing options, never credential values.
// Reapplication validates the digest against current profile configuration.
type SummarizerStartup struct {
	Version int               `json:"version"`
	Options SummarizerOptions `json:"options"`
	Digest  string            `json:"digest"`
}

func (s SummarizerStartup) Validate() error {
	if s.Version != 1 {
		return errors.New("unsupported summariser configuration version")
	}
	raw, err := hex.DecodeString(s.Digest)
	if err != nil || len(raw) != 32 || strings.ToLower(s.Digest) != s.Digest {
		return errors.New("summariser configuration requires a reviewed SHA-256 digest")
	}
	if s.Options.FollowMain == (s.Options.Profile != "") {
		return errors.New("summariser configuration requires either follow_main or a profile")
	}
	if len(s.Options.Profile) > 128 || s.Options.MaxOutputTokens < 0 || s.Options.MaxOutputTokens > 32768 || s.Options.TimeoutSeconds < 0 || s.Options.TimeoutSeconds > 300 {
		return errors.New("invalid summariser configuration limits")
	}
	return nil
}
func ReadSummarizerStartup(path string) (SummarizerStartup, error) {
	var selected SummarizerStartup
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return selected, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return selected, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<10 {
		return selected, errors.New("summariser configuration must be a regular file at most 16 KiB")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (16<<10)+1))
	if err != nil {
		return selected, err
	}
	if len(raw) > 16<<10 {
		return selected, errors.New("summariser configuration exceeds 16 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&selected); err != nil {
		return selected, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return selected, errors.New("trailing summariser configuration")
	}
	return selected, selected.Validate()
}

package agentio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const instructionFileLimit = 32 * 1024
const instructionTotalLimit = 128 * 1024
const instructionDepthLimit = 64

// InstructionSource retains provenance independently of rendered prompt text.
type InstructionSource struct{ Path, Body string }
type InstructionReport struct {
	Sources     []InstructionSource
	Diagnostics []string
}

// DiscoverInstructions reads ~/.hand guidance followed by filesystem ancestors
// from broadest to nearest. At each level HAND.md wins over AGENTS.md, preserving
// Hand's existing collision rule. No instruction is interpreted as a capability.
func DiscoverInstructions(workspace string) InstructionReport {
	var r InstructionReport
	home, err := os.UserHomeDir()
	if err != nil {
		r.Diagnostics = append(r.Diagnostics, "Personal instructions: "+err.Error())
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		r.Diagnostics = append(r.Diagnostics, "Workspace instructions: "+err.Error())
		return r
	}
	var ancestors []string
	for dir := abs; ; dir = filepath.Dir(dir) {
		if len(ancestors) == instructionDepthLimit {
			r.Diagnostics = append(r.Diagnostics, "Ancestor instruction depth exceeds 64; ancestor guidance excluded")
			ancestors = nil
			break
		}
		ancestors = append(ancestors, dir)
		if filepath.Dir(dir) == dir {
			break
		}
	}
	dirs := []string{}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".hand"))
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		dirs = append(dirs, ancestors[i])
	}
	return discoverInstructionDirectories(dirs, r)
}

func discoverInstructionDirectories(dirs []string, r InstructionReport) InstructionReport {
	seen := map[string]bool{}
	total := 0
	for _, dir := range dirs {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		selected := ""
		for _, name := range []string{"HAND.md", "AGENTS.md"} {
			path := filepath.Join(dir, name)
			info, err := os.Lstat(path)
			if os.IsNotExist(err) {
				continue
			}
			if selected != "" {
				r.Diagnostics = append(r.Diagnostics, fmt.Sprintf("%s shadowed by %s", path, selected))
				continue
			}
			// Presence establishes precedence even if the preferred file is unreadable.
			selected = path
			if err != nil {
				r.Diagnostics = append(r.Diagnostics, path+": "+err.Error())
				continue
			}
			if !info.Mode().IsRegular() {
				r.Diagnostics = append(r.Diagnostics, path+": non-regular instruction excluded")
				continue
			}
			if info.Size() > instructionFileLimit {
				r.Diagnostics = append(r.Diagnostics, path+": exceeds 32 KiB instruction limit")
				continue
			}
			body, err := readInstruction(path, info)
			if err != nil {
				r.Diagnostics = append(r.Diagnostics, path+": "+err.Error())
				continue
			}
			if total+len(body) > instructionTotalLimit {
				r.Diagnostics = append(r.Diagnostics, path+": exceeds 128 KiB total instruction limit")
				continue
			}
			total += len(body)
			r.Sources = append(r.Sources, InstructionSource{path, string(body)})
		}
	}
	return r
}

func readInstruction(path string, original os.FileInfo) ([]byte, error) {
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
		return nil, fmt.Errorf("instruction changed during discovery")
	}
	data, err := io.ReadAll(io.LimitReader(f, instructionFileLimit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > instructionFileLimit {
		return nil, fmt.Errorf("exceeds 32 KiB instruction limit")
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		return nil, fmt.Errorf("instruction changed during read")
	}
	return data, nil
}

func (r InstructionReport) Format() string {
	if len(r.Sources) == 0 && len(r.Diagnostics) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nProject guidance: user requests take precedence; nearer applicable guidance takes precedence over broader guidance and personal defaults. Guidance does not grant tool permissions.\n")
	for _, source := range r.Sources {
		fmt.Fprintf(&b, "\nProject instructions (%s):\n\n%s\n", source.Path, source.Body)
	}
	for _, diagnostic := range r.Diagnostics {
		fmt.Fprintf(&b, "\nInstruction discovery: %s\n", diagnostic)
	}
	return b.String()
}

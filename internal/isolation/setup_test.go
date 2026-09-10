package isolation

import (
	"context"
	"strings"
	"testing"

	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/hand/internal/toolproxy"
	"github.com/sausheong/harness/execution"
	"github.com/sausheong/harness/tool"
	"github.com/sausheong/harness/tools/file"
)

func TestMissingBackendIsNotHostFallback(t *testing.T) {
	cfg := config.Config{Execution: config.ExecutionConfig{Backend: "container", Docker: "/missing/docker", Socket: "/missing/socket", Worker: "/missing/worker", WorkerSHA256: strings.Repeat("b", 64), Image: "sha256:" + strings.Repeat("a", 64)}}
	backend, _, err := Prepare(context.Background(), cfg, t.TempDir())
	if err == nil || backend != nil {
		t.Fatal("missing container support selected a backend")
	}
}
func TestInstallPreservesDefinitionsAndWrapsExecution(t *testing.T) {
	workspace := t.TempDir()
	reg := tool.NewRegistry()
	original := &file.ReadFileTool{WorkDir: workspace}
	reg.Register(original)
	if err := Install(reg, execution.Container{}, workspace); err != nil {
		t.Fatal(err)
	}
	selected, _ := reg.Get("read_file")
	proxy, ok := selected.(*toolproxy.Tool)
	if !ok || proxy.Tool != original || proxy.Workspace != workspace {
		t.Fatal("original tool not wrapped")
	}
}

// A newly added tool must not silently retain host execution when the rest of
// the registry is converted to container tools.
type namedReadTool struct {
	file.ReadFileTool
	name string
}

func (t *namedReadTool) Name() string { return t.name }

func TestUnsupportedToolRejectsInstallationAtomically(t *testing.T) {
	for _, unknown := range []string{"a_unknown", "z_unknown"} {
		t.Run(unknown, func(t *testing.T) {
			workspace := t.TempDir()
			reg := tool.NewRegistry()
			read := &file.ReadFileTool{WorkDir: workspace}
			other := &namedReadTool{name: unknown}
			reg.Register(read)
			reg.Register(other)
			if err := Install(reg, execution.Container{}, workspace); err == nil {
				t.Fatal("unsupported tool silently retained host access")
			}
			got, _ := reg.Get("read_file")
			if got != read {
				t.Fatal("failed installation partially replaced registry")
			}
			got, _ = reg.Get(unknown)
			if got != other {
				t.Fatal("failed installation changed unsupported tool")
			}
		})
	}
}

func TestAllWorkerToolsReceiveContainerProxy(t *testing.T) {
	workspace := t.TempDir()
	reg := tool.NewRegistry()
	names := []string{"read_file", "write_file", "edit_file", "search", "todo_write", "skill_manage", "load_skill", "web_fetch", "web_search"}
	originals := make(map[string]tool.Tool)
	for _, name := range names {
		originals[name] = &namedReadTool{name: name}
		reg.Register(originals[name])
	}
	backend := &execution.Container{Workspace: workspace}
	if err := Install(reg, backend, workspace); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		got, _ := reg.Get(name)
		proxy, ok := got.(*toolproxy.Tool)
		if !ok || proxy.Tool != originals[name] || proxy.Backend != backend || proxy.Workspace != workspace {
			t.Fatalf("%s did not retain its definition behind the configured boundary", name)
		}
	}
}

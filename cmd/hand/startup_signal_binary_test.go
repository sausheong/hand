//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sausheong/hand/protocol"
)

func TestBinarySignalsJoinMCPStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, mode := range []string{"oneshot", "rpc"} {
		for _, signal := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
			t.Run(mode+"/"+signal.String(), func(t *testing.T) {
				home, workspace := t.TempDir(), t.TempDir()
				pidFile := filepath.Join(home, "mcp.pid")
				configDir := filepath.Join(home, ".hand")
				if err := os.Mkdir(configDir, 0700); err != nil {
					t.Fatal(err)
				}
				config := map[string]any{"mcp_servers": []any{map[string]any{"name": "hung", "command": "/bin/sh", "args": []string{"-c", `echo $$ > "$1"; exec sleep 30`, "hand-mcp", pidFile}}}}
				data, err := json.Marshal(config)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"--model=local/fixture", "--base-url=http://127.0.0.1:1/v1"}
				if mode == "rpc" {
					args = append(args, "--rpc")
				} else {
					args = append(args, "-p", "wait")
				}
				cmd := exec.CommandContext(ctx, binary, args...)
				if mode == "rpc" {
					input, err := cmd.StdinPipe()
					if err != nil {
						t.Fatal(err)
					}
					defer input.Close()
				}
				cmd.Dir = workspace
				cmd.Env = append(os.Environ(), "HOME="+home)
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				defer cmd.Process.Kill()
				waited := make(chan error, 1)
				go func() { waited <- cmd.Wait() }()
				pid := 0
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					if raw, err := os.ReadFile(pidFile); err == nil {
						pid, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
						if pid > 0 {
							break
						}
					}
					select {
					case err := <-waited:
						t.Fatalf("exited before MCP startup: %v %s", err, stderr.String())
					default:
					}
					time.Sleep(5 * time.Millisecond)
				}
				if pid == 0 {
					t.Fatal("MCP child did not start")
				}
				defer syscall.Kill(pid, syscall.SIGKILL)
				start := time.Now()
				if err := cmd.Process.Signal(signal); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-waited:
					var exit *exec.ExitError
					if !errors.As(err, &exit) || exit.ExitCode() != 130 {
						t.Errorf("startup cancellation exit: %v; stderr=%s", err, stderr.String())
					}
				case <-time.After(5 * time.Second):
					t.Fatal("startup shutdown exceeded child cleanup deadline")
				}
				if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
					t.Errorf("MCP child remains after Hand exit: %v", err)
				}
				if stdout.Len() != 0 {
					t.Errorf("startup cancellation fabricated run output: %s", stdout.String())
				}
				t.Logf("mode=%s signal=%v cleanup_ms=%.3f", mode, signal, float64(time.Since(start).Microseconds())/1000)
			})
		}
	}
}

func TestBinaryRPCPreparedInputNegotiation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	cmd := exec.CommandContext(ctx, binary, "--rpc", "--model=local/fixture", "--base-url=http://127.0.0.1:1/v1")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer output.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	// Send before waiting for readiness: construction must retain this request.
	if err := protocol.NewWriter(input).Write(protocol.Request{Version: protocol.Version, ID: "queued", Method: "hello", Params: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.NewReader(output).ReadFrame()
	if err != nil {
		t.Fatal(err)
	}
	var response protocol.Response
	if err := json.Unmarshal(raw, &response); err != nil || response.RequestID != "queued" || response.Error != nil {
		t.Fatalf("queued negotiation: %s %v", raw, err)
	}
	input.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("negotiated EOF shutdown: %v", err)
	}
}

func TestBinaryRPCDisconnectJoinsMCPStartup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "hand")
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatal(err, string(out))
	}
	for _, queued := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(queued), func(t *testing.T) { checkRPCStartupDisconnect(t, ctx, binary, queued) })
	}
}

func checkRPCStartupDisconnect(t *testing.T, ctx context.Context, binary string, queued int) {
	t.Helper()
	home, workspace := t.TempDir(), t.TempDir()
	pidFile := filepath.Join(home, "mcp.pid")
	if err := os.Mkdir(filepath.Join(home, ".hand"), 0700); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"mcp_servers": []any{map[string]any{"name": "hung", "command": "/bin/sh", "args": []string{"-c", `echo $$ > "$1"; exec sleep 30`, "hand-mcp", pidFile}}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".hand", "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary, "--rpc", "--model=local/fixture", "--base-url=http://127.0.0.1:1/v1")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "HOME="+home)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	joined := false
	defer func() {
		if !joined {
			_ = cmd.Process.Kill()
			<-waited
		}
	}()
	pid := 0
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
			if pid > 0 {
				break
			}
		}
		select {
		case err := <-waited:
			joined = true
			t.Fatalf("exit before handshake: %v %s", err, stderr.String())
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("MCP child did not start")
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	start := time.Now()
	writer := protocol.NewWriter(input)
	for i := 0; i < queued; i++ {
		if err := writer.Write(protocol.Request{Version: protocol.Version, ID: strconv.Itoa(i), Method: "hello", Params: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-waited:
		joined = true
		var exit *exec.ExitError
		if queued > 1 {
			if !errors.As(err, &exit) || exit.ExitCode() != 5 || !strings.Contains(stderr.String(), "RPC startup request queue full") {
				t.Fatalf("backlog diagnostic: %v %s", err, stderr.String())
			}
		} else if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 130) {
			t.Fatalf("disconnect exit: %v %s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RPC EOF did not cancel MCP startup within cleanup deadline")
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("child remains after disconnect: %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("startup disconnect fabricated output: %s", stdout.String())
	}
	t.Logf("queued=%d rpc_startup_eof_cleanup_ms=%.3f", queued, float64(time.Since(start).Microseconds())/1000)
}

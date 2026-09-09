package sdk_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/protocol"
	"github.com/sausheong/hand/sdk"
)

func TestSDKProcessHelper(t *testing.T) {
	if os.Getenv("HAND_SDK_HELPER") != "1" {
		return
	}
	reader := protocol.NewReader(os.Stdin)
	writer := protocol.NewWriter(os.Stdout)
	for {
		frame, err := reader.ReadFrame()
		if err != nil {
			break
		}
		req, err := protocol.DecodeRequest(frame)
		if err != nil {
			os.Exit(2)
		}
		writer.Write(protocol.Response{Version: 1, RequestID: req.ID, Result: json.RawMessage(`{"helper":true}`)})
	}
	if os.Getenv("HAND_SDK_HANG") == "1" {
		time.Sleep(time.Minute)
	}
	if err := os.WriteFile(os.Getenv("HAND_SDK_MARKER"), []byte("joined"), 0600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func TestProcessCloseAllowsCleanup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "joined")
	client, err := sdk.StartProcess(context.Background(), sdk.ProcessOptions{Binary: os.Args[0], Arguments: []string{"-test.run=^TestSDKProcessHelper$"}, Environment: append(os.Environ(), "HAND_SDK_HELPER=1", "HAND_SDK_MARKER="+marker)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = client.Hello(ctx, "h"); err != nil {
		t.Fatal(err)
	}
	if err = client.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(marker)
	if err != nil || string(raw) != "joined" {
		t.Fatalf("graceful cleanup missing: %q %v", raw, err)
	}
}
func TestProcessCancellationBoundsUncooperativeShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: os.Args[0], Arguments: []string{"-test.run=^TestSDKProcessHelper$"}, Environment: append(os.Environ(), "HAND_SDK_HELPER=1", "HAND_SDK_HANG=1"), ShutdownTimeout: 30 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	callCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if _, err = client.Hello(callCtx, "h"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	cancel()
	closed := make(chan error, 1)
	go func() { closed <- client.Close() }()
	select {
	case err = <-closed:
		if err == nil {
			t.Fatal("expected forced termination error")
		}
		if time.Since(start) > time.Second {
			t.Fatal("shutdown exceeded bound")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process did not join")
	}
}

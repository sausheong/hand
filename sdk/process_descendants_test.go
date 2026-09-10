//go:build unix

package sdk_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sausheong/hand/sdk"
)

func TestProcessCloseOwnsDescendants(t *testing.T) {
	for _, mode := range []string{"graceful", "forced"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			client, err := sdk.StartProcess(context.Background(), sdk.ProcessOptions{
				Binary: "sh", ShutdownTimeout: 30 * time.Millisecond,
				Arguments: []string{"-c", `(printf ready > "$1/ready"; sleep 1; printf escaped > "$1/late") >/dev/null 2>&1 &
if [ "$2" = graceful ]; then read line; exit 0; else wait; fi`, "sdk", root, mode},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(root, "ready")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("child did not start")
				}
				time.Sleep(time.Millisecond)
			}
			start := time.Now()
			err = client.Close()
			if mode == "graceful" && err != nil {
				t.Fatalf("graceful close failed: %v", err)
			}
			if time.Since(start) > time.Second {
				t.Fatal("SDK shutdown exceeded bound")
			}
			time.Sleep(1100 * time.Millisecond)
			if _, err := os.Stat(filepath.Join(root, "late")); !os.IsNotExist(err) {
				t.Fatalf("SDK-owned descendant survived %s shutdown: %v", mode, err)
			}
		})
	}
}

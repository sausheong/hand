// The embedded example opens Hand and lists sessions without a model call.
// Usage: go run ./examples/embedded WORKSPACE STORE_DIRECTORY
package main

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/sdk"
	"os"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: embedded WORKSPACE STORE_DIRECTORY")
		os.Exit(2)
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := sdk.Open(ctx, sdk.EmbeddedOptions{Workspace: os.Args[1], StoreDirectory: os.Args[2], Model: "local/test"})
	if err != nil {
		return err
	}
	defer client.Close()
	if _, err = client.Hello(ctx, "hello"); err != nil {
		return err
	}
	sessions, err := client.Call(ctx, "sessions", "session.list", nil)
	if err != nil {
		return err
	}
	fmt.Println(string(sessions))
	return client.Close()
}

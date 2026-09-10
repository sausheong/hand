// hand-tool-worker is a private execution helper. Its caller owns admission and
// must run it within the selected isolation boundary; it does not grant access.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/sausheong/hand/internal/toolworker"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "tool worker takes one JSON request on stdin")
		os.Exit(2)
	}
	workspace, err := os.Getwd()
	if err == nil {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		err = toolworker.Serve(ctx, workspace, os.Stdin, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

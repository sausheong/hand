// The RPC example starts an installed Hand binary and negotiates capabilities.
// It makes no model request. Run with: go run ./examples/rpc /path/to/hand
package main

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/sdk"
	"os"
	"time"
)

func main() {
	binary := "hand"
	if len(os.Args) > 1 {
		binary = os.Args[1]
	}
	if err := run(binary); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(binary string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := sdk.StartProcess(ctx, sdk.ProcessOptions{Binary: binary, Arguments: []string{"--rpc"}, Stderr: os.Stderr})
	if err != nil {
		return err
	}
	defer client.Close()
	capabilities, err := client.Hello(ctx, "example-hello")
	if err != nil {
		return err
	}
	fmt.Println(string(capabilities))
	return nil
}

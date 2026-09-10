// tool-policy demonstrates an additional veto for an explicitly named path.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sausheong/hand/extension/protocol"
	"io"
	"os"
)

func run() error {
	denied := flag.String("deny-path", "protected.txt", "exact tool input path to deny")
	flag.Parse()
	reader, writer := protocol.NewReader(os.Stdin), protocol.NewWriter(os.Stdout)
	for {
		frame, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if frame.Kind != "request" {
			return fmt.Errorf("request required")
		}
		reply := protocol.Frame{Version: 1, Kind: "response", ID: frame.ID}
		var value any
		switch frame.Method {
		case "initialize":
			value = map[string]any{"version": 1, "name": "tool-policy", "capabilities": []string{"policy.check"}}
		case "policy.check":
			var request struct {
				Action   string                     `json:"action"`
				Resource string                     `json:"resource"`
				Input    map[string]json.RawMessage `json:"input"`
			}
			err = protocol.DecodePayload(frame.Params, &request)
			if err == nil && request.Action != "tool.execute" {
				err = fmt.Errorf("unsupported policy action")
			}
			var path string
			if err == nil && request.Input["path"] != nil {
				err = json.Unmarshal(request.Input["path"], &path)
			}
			if err == nil {
				value = map[string]any{"allow": path != *denied, "reason": "Example policy protects the configured exact path"}
			}
		default:
			err = fmt.Errorf("unsupported method")
		}
		if err != nil {
			reply.Error = &protocol.Error{Code: "policy_error", Message: "Policy request could not be checked"}
		} else {
			reply.Result, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}
		if err = writer.Write(reply); err != nil {
			return err
		}
	}
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

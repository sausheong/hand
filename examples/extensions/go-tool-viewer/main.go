// tool-viewer renders an explicitly supplied tool result; it executes no tools.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sausheong/hand/extension/protocol"
)

type toolRecord struct {
	Tool   string `json:"tool"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func render(raw string) (protocol.Presentation, error) {
	var empty protocol.Presentation
	if len(raw) > 8<<10 {
		return empty, errors.New("tool record exceeds 8 KiB")
	}
	var record toolRecord
	if err := protocol.DecodePayload([]byte(raw), &record); err != nil {
		return empty, err
	}
	if record.Tool == "" || len(record.Tool) > 64 {
		return empty, errors.New("tool name required within 64 bytes")
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return empty, err
	}
	status := "Supplied record contains no error."
	if record.Error != "" {
		status = "Supplied record contains an error."
	}
	result := protocol.Presentation{Blocks: []protocol.Block{
		{Kind: "text", Text: "Tool result viewer — supplied data, not an independently verified execution record."},
		{Kind: "list", Items: []string{status, "This view does not change Hand's run outcome or permissions."}},
		{Kind: "code", Language: "json", Text: string(encoded)},
	}}
	if err = result.Validate(); err != nil {
		return empty, err
	}
	return result, nil
}
func handle(frame protocol.Frame) (any, error) {
	switch frame.Method {
	case "initialize":
		return protocol.Hello{Version: 1, Name: "tool-viewer", Capabilities: []string{"commands", "presentation"}, Commands: []protocol.Command{{Name: "view-tool", Description: "Render an explicitly supplied JSON tool result"}}}, nil
	case "command.execute":
		var command struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := protocol.DecodePayload(frame.Params, &command); err != nil {
			return nil, err
		}
		if command.Name != "view-tool" {
			return nil, errors.New("unknown command")
		}
		return render(command.Arguments)
	default:
		return nil, errors.New("unsupported method")
	}
}
func run() error {
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
			return errors.New("request required")
		}
		value, err := handle(frame)
		reply := protocol.Frame{Version: 1, Kind: "response", ID: frame.ID}
		if err != nil {
			reply.Error = &protocol.Error{Code: "invalid_tool_record", Message: "Tool viewer requires a bounded JSON tool/output/error record."}
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

// task-note demonstrates questions, durable host state and editable context.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sausheong/hand/extension/protocol"
)

type peer struct {
	reader   *protocol.Reader
	writer   *protocol.Writer
	sequence int
}

func raw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
func (p *peer) callback(method string, params any, result any) error {
	p.sequence++
	id := fmt.Sprintf("note-%d", p.sequence)
	if err := p.writer.Write(protocol.Frame{Version: 1, Kind: "request", ID: id, Method: method, Params: raw(params)}); err != nil {
		return err
	}
	reply, err := p.reader.Read()
	if err != nil {
		return err
	}
	if reply.Kind != "response" || reply.ID != id {
		return fmt.Errorf("unexpected callback reply")
	}
	if reply.Error != nil {
		return fmt.Errorf("host callback: %s", reply.Error.Code)
	}
	return json.Unmarshal(reply.Result, result)
}
func (p *peer) handle(f protocol.Frame) (any, error) {
	switch f.Method {
	case "initialize":
		return map[string]any{"version": 1, "name": "task-note", "capabilities": []string{"commands", "questions", "state", "context.transform"}, "commands": []map[string]string{{"name": "note", "description": "Ask for and remember a task note"}}}, nil
	case "command.execute":
		var cmd struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}
		if err := protocol.DecodePayload(f.Params, &cmd); err != nil {
			return nil, err
		}
		if cmd.Name != "note" {
			return nil, fmt.Errorf("unknown command")
		}
		question := protocol.Question{ID: "note", Title: "What task note should be remembered?", AllowFreeText: true}
		var answer protocol.Answer
		if err := p.callback("user.question", question, &answer); err != nil {
			return nil, err
		}
		if err := question.ValidateAnswer(answer); err != nil {
			return nil, err
		}
		if answer.Cancelled {
			return protocol.Presentation{Blocks: []protocol.Block{{Kind: "text", Text: "Note cancelled."}}}, nil
		}
		var state struct {
			Revision int             `json:"revision"`
			Data     json.RawMessage `json:"data"`
		}
		if err := p.callback("state.get", struct{}{}, &state); err != nil {
			return nil, err
		}
		var ack any
		if err := p.callback("state.set", map[string]any{"revision": state.Revision, "data": map[string]string{"note": answer.Text}}, &ack); err != nil {
			return nil, err
		}
		return protocol.Presentation{Blocks: []protocol.Block{{Kind: "text", Text: "Task note saved."}}}, nil
	case "context.transform":
		var input struct {
			Items []struct {
				ID   string `json:"id"`
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"items"`
		}
		if err := protocol.DecodePayload(f.Params, &input); err != nil {
			return nil, err
		}
		replacements := []map[string]string{}
		for _, item := range input.Items {
			replacements = append(replacements, map[string]string{"id": item.ID, "text": strings.TrimSpace(item.Text)})
		}
		return map[string]any{"replacements": replacements}, nil
	default:
		return nil, fmt.Errorf("unsupported method")
	}
}
func run() error {
	p := peer{reader: protocol.NewReader(os.Stdin), writer: protocol.NewWriter(os.Stdout)}
	for {
		f, err := p.reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if f.Kind != "request" {
			return fmt.Errorf("expected request")
		}
		result, err := p.handle(f)
		reply := protocol.Frame{Version: 1, Kind: "response", ID: f.ID}
		if err != nil {
			reply.Error = &protocol.Error{Code: "request_failed", Message: "Task note request failed; see extension diagnostics."}
			fmt.Fprintln(os.Stderr, err)
		} else {
			reply.Result = raw(result)
		}
		if err = p.writer.Write(reply); err != nil {
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

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sausheong/hand/extension/protocol"
)

func TestToolViewerPreservesLiteralRecordAndReportsScope(t *testing.T) {
	for _, message := range []string{"", "tool failed"} {
		input := toolRecord{Tool: "read_file", Output: "```\n[click](https://example.invalid)\n\u001b[31m", Error: message}
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		result, err := render(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Blocks) != 3 || result.Blocks[2].Kind != "code" || result.Blocks[2].Language != "json" {
			t.Fatal(result)
		}
		var restored toolRecord
		if err = json.Unmarshal([]byte(result.Blocks[2].Text), &restored); err != nil || restored != input {
			t.Fatal(restored, err)
		}
		if strings.ContainsRune(result.Blocks[2].Text, '\x1b') {
			t.Fatal("terminal control not escaped")
		}
		if !strings.Contains(result.Blocks[0].Text, "not an independently verified") {
			t.Fatal("missing scope")
		}
		if err = result.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestToolViewerRejectsMalformedAndOversizedRecords(t *testing.T) {
	for _, input := range []string{`{}`, `{"tool":"x","tool":"y","output":"z"}`, `{"tool":"x","output":"z","outcome":"complete"}`, `[]`, strings.Repeat("x", 8193), `{"tool":"x","output":"` + string([]byte{0xff}) + `"}`} {
		if result, err := render(input); err == nil || len(result.Blocks) != 0 {
			t.Fatal("invalid record accepted", err)
		}
	}
	value, err := handle(protocol.Frame{Method: "initialize"})
	if err != nil {
		t.Fatal(err)
	}
	if err = value.(protocol.Hello).Validate([]string{"commands", "presentation"}); err != nil {
		t.Fatal(err)
	}
}

package agentio

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"unicode/utf8"
)

func FuzzHookPayload(f *testing.F) {
	f.Add("PreToolUse", "read this", `{"path":"file.go"}`)
	f.Add("Stop", "line\nquote\"\x00", "")
	f.Add("PreToolUse", "invalid tool input", `{} {}`)
	f.Fuzz(func(t *testing.T, event, prompt, input string) {
		payload, err := encodeHookInput(context.Background(), map[string]string{
			"HAND_HOOK_EVENT": event, "HAND_PROMPT": prompt, "HAND_TOOL_INPUT": input,
		})
		if input != "" && !json.Valid([]byte(input)) {
			if err == nil || len(payload) != 0 {
				t.Fatal("invalid tool JSON produced hook payload")
			}
			return
		}
		if err != nil {
			t.Fatalf("valid hook input rejected: %v", err)
		}
		var decoded HookInput
		if err := json.Unmarshal(payload, &decoded); err != nil || decoded.Version != 1 {
			t.Fatalf("invalid encoded hook envelope: %v", err)
		}
		if utf8.ValidString(event) && decoded.Event != event || utf8.ValidString(prompt) && decoded.Prompt != prompt {
			t.Fatal("hook string fields changed or escaped the envelope")
		}
		if bytes.ContainsAny(payload, "\n\r") {
			t.Fatal("hook payload contains an unescaped line boundary")
		}
		if input != "" && !json.Valid(decoded.ToolInput) {
			t.Fatal("structured tool input lost")
		}
	})
}

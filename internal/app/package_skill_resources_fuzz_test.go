package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sausheong/harness/runtime"
)

func FuzzPackageSkillToolInput(f *testing.F) {
	for _, seed := range []string{`{"name":"authored"}`, `{"name":"x","resource":"../outside"}`, `{"name":"x","execute":true}`, `null`, `{} {}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		base := &backendSkillLoader{}
		loader := &packageResourceSkillTool{base: base, rt: &runtime.Runtime{}}
		result, err := loader.Execute(context.Background(), raw)
		if err != nil {
			t.Fatalf("input decoder leaked backend error: %v", err)
		}
		if len(base.calls) > 0 {
			var input struct {
				Name     string
				Resource *string
			}
			if json.Unmarshal(raw, &input) != nil || input.Name == "" || input.Resource != nil || len(base.calls) != 1 || base.calls[0] != input.Name {
				t.Fatal("invalid or resource request delegated to ordinary backend")
			}
		} else if result.Error == "" || result.Output != "" {
			t.Fatal("unselected resource or invalid input returned successful content")
		}
		before := len(base.calls)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		result, err = loader.Execute(ctx, raw)
		if !errors.Is(err, context.Canceled) || result.Output != "" || len(base.calls) != before {
			t.Fatal("cancelled request executed or exposed content")
		}
	})
}

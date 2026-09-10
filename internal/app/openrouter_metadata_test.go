package app

import "testing"

func TestRouterMetadataExactModelAndLimits(t *testing.T) {
	raw := []byte(`{"data":[{"id":"vendor/model","context_length":200000,"top_provider":{"context_length":128000,"max_completion_tokens":8192}}]}`)
	m, err := decodeRouterMetadata(raw, "vendor/model")
	if err != nil || m.AdvertisedContext != 128000 || m.MaxOutput != 8192 {
		t.Fatalf("%+v %v", m, err)
	}
	if _, err = decodeRouterMetadata(raw, "model"); err == nil {
		t.Fatal("matched ambiguous alias")
	}
	for _, raw := range []string{`{`, `{"data":[{"id":"x","context_length":0}]}`, `{"data":[{"id":"x","context_length":100,"top_provider":{"max_completion_tokens":-1}}]}`} {
		if _, err := decodeRouterMetadata([]byte(raw), "x"); err == nil {
			t.Fatal("accepted invalid metadata")
		}
	}
}

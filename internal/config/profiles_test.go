package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyAndNamedProfileIsolation(t *testing.T) {
	c := Config{Model: "local/model", BaseURL: "http://localhost:9000/v1", Profiles: map[string]ModelProfile{"hosted": {Provider: "openai", Model: "hosted", CredentialEnv: "HOSTED_KEY", InputTypes: []string{"text"}}}}
	legacy, err := SelectProfile(c, "")
	if err != nil || legacy.Endpoint != c.BaseURL || legacy.CredentialEnv != "" {
		t.Fatal(legacy, err)
	}
	p, err := SelectProfile(c, "hosted")
	if err != nil || p.Endpoint != "" || p.CredentialEnv != "HOSTED_KEY" {
		t.Fatal(p, err)
	}
	p.InputTypes[0] = "image"
	if c.Profiles["hosted"].InputTypes[0] != "text" {
		t.Fatal("profile mutation escaped")
	}
	if _, err := SelectProfile(c, "missing"); err == nil {
		t.Fatal("unknown profile accepted")
	}
}

func TestProfileMetadataPrecedenceAndActiveContext(t *testing.T) {
	p := ModelProfile{Provider: "local", Model: "alias"}
	catalog := ProfileMetadata{Verified: true, Source: "catalog-v1", AdvertisedContext: 131072, MaxOutput: 4096}
	r, err := ResolveProfile(p, ProfileMetadata{}, catalog)
	if err != nil || r.ContextLimit != 8192 || r.ContextSource != "conservative_fallback" || r.AdvertisedContext != 131072 {
		t.Fatal(r, err)
	}
	server := ProfileMetadata{Verified: true, Source: "server-config", ActiveContext: 32768, AdvertisedContext: 262144, MaxOutput: 8192}
	r, err = ResolveProfile(p, server, catalog)
	if err != nil || r.ContextLimit != 32768 || r.ContextSource != "server-config" || r.MaxOutput != 8192 {
		t.Fatal(r, err)
	}
	p.ContextLimit = 16384
	p.MaxOutput = 2048
	r, err = ResolveProfile(p, server, catalog)
	if err != nil || r.ContextLimit != 16384 || r.ContextSource != "explicit_override" || r.MaxOutput != 2048 {
		t.Fatal(r, err)
	}
	p.ContextLimit = 0
	p.MaxOutput = 0
	server.Verified = false
	r, err = ResolveProfile(p, server, ProfileMetadata{})
	if err != nil || r.ContextLimit != 8192 {
		t.Fatal("unverified metadata won", r, err)
	}
}

func TestProfileReasoningAndInvalidConfiguration(t *testing.T) {
	p := ModelProfile{Provider: "openai", Model: "model", Reasoning: "high"}
	if _, err := ResolveProfile(p, ProfileMetadata{}, ProfileMetadata{}); err == nil {
		t.Fatal("unverified reasoning capability accepted")
	}
	p.ReasoningLevels = []string{"high"}
	if _, err := ResolveProfile(p, ProfileMetadata{}, ProfileMetadata{}); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"https://user:secret@host/v1", "https://host/v1?key=secret", "file:///tmp/server"} {
		p.Endpoint = endpoint
		if p.Validate() == nil {
			t.Fatal("unsafe endpoint accepted", endpoint)
		}
	}
	p.Endpoint = ""
	p.CredentialEnv = "literal secret value"
	if p.Validate() == nil {
		t.Fatal("literal credential accepted")
	}
}

func TestProfileConfigRoundTripAndLegacyNotRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"model":"local/model","base_url":"http://localhost:9000/v1"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectProfile(c, ""); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(original) {
		t.Fatal("legacy file rewritten")
	}
	c.Profiles = map[string]ModelProfile{"proxy": {Provider: "litellm", Model: "alias", Endpoint: "https://proxy.example/v1", CredentialEnv: "PROXY_KEY"}}
	c.DefaultProfile = "proxy"
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := SelectProfile(loaded, "")
	if err != nil || p.CredentialEnv != "PROXY_KEY" || p.Endpoint != "https://proxy.example/v1" {
		t.Fatal(p, err)
	}
	loaded.DefaultProfile = "missing"
	if Save(path, loaded) == nil {
		t.Fatal("invalid default saved")
	}
}

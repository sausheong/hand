package config

import "testing"

func TestOutputAllowanceLegacyAndProfile(t *testing.T) {
	c := Config{Model: "litellm/alias", BaseURL: "https://proxy.example", MaxOutput: 8192}
	p, err := SelectProfile(c, "")
	if err != nil || p.MaxOutput != 8192 {
		t.Fatalf("%+v %v", p, err)
	}
	p.ContextLimit = 1000000
	r, err := ResolveProfile(p, ProfileMetadata{}, ProfileMetadata{})
	if err != nil || r.MaxOutput != 8192 || r.OutputSource != "explicit_override" {
		t.Fatalf("%+v %v", r, err)
	}
	c.Profiles = map[string]ModelProfile{"small": {Provider: "openai", Model: "alias", MaxOutput: 512}}
	p, err = SelectProfile(c, "small")
	if err != nil || p.MaxOutput != 512 {
		t.Fatalf("%+v %v", p, err)
	}
	c.MaxOutput = -1
	if ValidateProfiles(c) == nil {
		t.Fatal("negative limit accepted")
	}
}

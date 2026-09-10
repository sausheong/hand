package config

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

// ModelProfile contains references, never credential values. Model is the bare
// ID understood by this endpoint; an aggregator alias is not a verified model.
type ModelProfile struct {
	ProfileName      string   `json:"-"`
	ContextSource    string   `json:"-"`
	MetadataProtocol string   `json:"metadata_protocol,omitempty"`
	MetadataURL      string   `json:"metadata_url,omitempty"`
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	Endpoint         string   `json:"endpoint,omitempty"`
	CredentialEnv    string   `json:"credential_env,omitempty"`
	InputTypes       []string `json:"input_types,omitempty"`
	ContextLimit     int      `json:"context_limit,omitempty"`
	MaxOutput        int      `json:"max_output,omitempty"`
	Reasoning        string   `json:"reasoning,omitempty"`
	ReasoningLevels  []string `json:"reasoning_levels,omitempty"`
}

var environmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (p ModelProfile) Validate() error {
	if p.MetadataProtocol != "" || p.MetadataURL != "" {
		if p.MetadataProtocol != "llamacpp_props" {
			return fmt.Errorf("metadata_protocol must be llamacpp_props")
		}
		endpoint, err := url.Parse(p.Endpoint)
		if err != nil || endpoint.Host == "" {
			return fmt.Errorf("metadata discovery requires an explicit profile endpoint")
		}
		metadata, err := url.Parse(p.MetadataURL)
		if err != nil || metadata.Host != endpoint.Host || metadata.Scheme != endpoint.Scheme || metadata.User != nil || metadata.RawQuery != "" || metadata.Fragment != "" {
			return fmt.Errorf("metadata_url must share the profile endpoint origin without credentials, query or fragment")
		}
	}
	if !slices.Contains([]string{"anthropic", "openai", "gemini", "litellm", "openrouter", "local"}, p.Provider) {
		return fmt.Errorf("unknown provider %q in profile", p.Provider)
	}
	if strings.TrimSpace(p.Model) == "" || strings.TrimSpace(p.Model) != p.Model {
		return fmt.Errorf("profile requires a model ID without surrounding whitespace")
	}
	if p.CredentialEnv != "" && !environmentName.MatchString(p.CredentialEnv) {
		return fmt.Errorf("credential_env must be an environment variable name, not a credential value")
	}
	if p.Endpoint != "" {
		u, err := url.Parse(p.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("profile endpoint must be an HTTP(S) URL without credentials, query or fragment")
		}
		if p.Provider == "gemini" {
			return fmt.Errorf("gemini custom endpoints are not supported by the installed Harness provider")
		}
	}
	if p.Provider == "litellm" && p.Endpoint == "" {
		return fmt.Errorf("litellm requires --base-url or a profile endpoint")
	}
	if p.Provider == "local" && p.CredentialEnv != "" {
		return fmt.Errorf("local provider does not support credentials; use an authenticated OpenAI-compatible profile")
	}
	if p.ContextLimit == 1 {
		return fmt.Errorf("context_limit must leave room for both input and output")
	}
	if p.ContextLimit < 0 || p.MaxOutput < 0 {
		return fmt.Errorf("profile token limits cannot be negative")
	}
	if p.ContextLimit > 0 && p.MaxOutput >= p.ContextLimit {
		return fmt.Errorf("profile max_output must leave room for input within context_limit")
	}
	for _, kind := range p.InputTypes {
		if kind != "text" && kind != "image" {
			return fmt.Errorf("unsupported profile input type %q", kind)
		}
	}
	if len(p.InputTypes) > 0 && !slices.Contains(p.InputTypes, "text") {
		return fmt.Errorf("coding profiles must support text input")
	}
	for _, level := range append(slices.Clone(p.ReasoningLevels), p.Reasoning) {
		if !slices.Contains([]string{"", "off", "low", "medium", "high"}, level) {
			return fmt.Errorf("reasoning %q is unsupported by installed Harness; use off, low, medium or high", level)
		}
	}
	return nil
}

func ValidateProfiles(c Config) error {
	if c.DefaultProfile != "" {
		if _, ok := c.Profiles[c.DefaultProfile]; !ok {
			return fmt.Errorf("default_profile %q is not defined", c.DefaultProfile)
		}
	}
	for name, p := range c.Profiles {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
			return fmt.Errorf("profile names cannot be empty or have surrounding whitespace")
		}
		if err := p.Validate(); err != nil {
			return fmt.Errorf("profile %q: %w", name, err)
		}
	}
	return nil
}

// SelectProfile performs the legacy migration in memory. Load never rewrites an
// existing user's file to perform migration. Persist the returned profile only
// through an explicit configuration edit. Returned slices do not alias Config.
func SelectProfile(c Config, name string) (ModelProfile, error) {
	if name == "" {
		name = c.DefaultProfile
	}
	if name != "" {
		p, ok := c.Profiles[name]
		if !ok {
			return ModelProfile{}, fmt.Errorf("unknown profile %q", name)
		}
		p.ProfileName = name
		p.InputTypes = slices.Clone(p.InputTypes)
		p.ReasoningLevels = slices.Clone(p.ReasoningLevels)
		return p, p.Validate()
	}
	model := c.Model
	if model == "" {
		model = DefaultModel
	}
	provider, bare, ok := strings.Cut(model, "/")
	if !ok {
		return ModelProfile{}, fmt.Errorf("legacy model must be in provider/model form")
	}
	p := ModelProfile{Provider: provider, Model: bare, Endpoint: c.BaseURL, CredentialEnv: DefaultCredentialEnv(provider)}
	return p, p.Validate()
}

func DefaultCredentialEnv(provider string) string {
	return map[string]string{"anthropic": "ANTHROPIC_API_KEY", "openai": "OPENAI_API_KEY", "gemini": "GEMINI_API_KEY", "litellm": "LITELLM_API_KEY", "openrouter": "OPENROUTER_API_KEY"}[provider]
}

// ProfileMetadata is supplied by a verified discovery adapter or a versioned
// catalogue. ActiveContext is a server's configured limit, distinct from the
// model's advertised maximum. Unverified metadata cannot influence resolution.
type ProfileMetadata struct {
	Verified                                    bool
	Source                                      string
	ActiveContext, AdvertisedContext, MaxOutput int
	ReasoningLevels                             []string
	InputTypes                                  []string
}
type ResolvedProfile struct {
	Profile           ModelProfile
	ContextLimit      int
	ContextSource     string
	AdvertisedContext int
	MaxOutput         int
	OutputSource      string
	Reasoning         string
}

// ResolveProfile never guesses that an advertised local model maximum is its
// active server context. Explicit overrides win; verified active metadata then
// versioned catalogue metadata follow. Unknown limits use a labelled fallback.
func ResolveProfile(p ModelProfile, server, catalogue ProfileMetadata) (ResolvedProfile, error) {
	if err := p.Validate(); err != nil {
		return ResolvedProfile{}, err
	}
	p.InputTypes = slices.Clone(p.InputTypes)
	p.ReasoningLevels = slices.Clone(p.ReasoningLevels)
	r := ResolvedProfile{Profile: p, ContextLimit: 8192, ContextSource: "conservative_fallback", MaxOutput: 2048, OutputSource: "conservative_fallback", Reasoning: p.Reasoning}
	for _, m := range []ProfileMetadata{catalogue, server} {
		if !m.Verified {
			continue
		}
		if m.Source == "" {
			return ResolvedProfile{}, fmt.Errorf("verified metadata requires source/version identity")
		}
		if m.ActiveContext < 0 || m.AdvertisedContext < 0 || m.MaxOutput < 0 {
			return ResolvedProfile{}, fmt.Errorf("metadata limits cannot be negative")
		}
		if m.AdvertisedContext > 0 {
			r.AdvertisedContext = m.AdvertisedContext
		}
		limit := m.ActiveContext
		if p.Provider != "local" && limit == 0 {
			limit = m.AdvertisedContext
		}
		if limit > 0 {
			r.ContextLimit = limit
			r.ContextSource = m.Source
		}
		if len(p.InputTypes) == 0 && len(m.InputTypes) > 0 {
			r.Profile.InputTypes = slices.Clone(m.InputTypes)
		}
		if m.MaxOutput > 0 {
			r.MaxOutput = m.MaxOutput
			r.OutputSource = m.Source
		}
	}
	if p.ContextLimit > 0 {
		r.ContextLimit = p.ContextLimit
		r.ContextSource = "explicit_override"
	}
	if p.MaxOutput > 0 {
		r.MaxOutput = p.MaxOutput
		r.OutputSource = "explicit_override"
	}
	if r.MaxOutput >= r.ContextLimit {
		if p.MaxOutput > 0 {
			return ResolvedProfile{}, fmt.Errorf("explicit max_output must be below resolved context limit %d", r.ContextLimit)
		}
		r.MaxOutput = max(1, r.ContextLimit/4)
		r.OutputSource = "conservative_context_fraction"
	}
	if p.Reasoning != "" && p.Reasoning != "off" {
		levels := p.ReasoningLevels
		if len(levels) == 0 && server.Verified {
			levels = server.ReasoningLevels
		}
		if len(levels) == 0 && catalogue.Verified {
			levels = catalogue.ReasoningLevels
		}
		if !slices.Contains(levels, p.Reasoning) {
			return ResolvedProfile{}, fmt.Errorf("reasoning %q is not declared supported for %s/%s; configure verified capability or use off", p.Reasoning, p.Provider, p.Model)
		}
	}
	return r, nil
}

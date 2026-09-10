package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/sausheong/hand/internal/config"
)

const MetadataMaxBytes = 1 << 20
const MetadataTimeout = 2 * time.Second

// DiscoverProfile reads the explicitly configured metadata origin. Credentials
// cannot follow redirects, and response bodies are never included in errors.
// Verified here means decoded server-reported configuration, not independent
// proof of model identity or capability correctness.
func DiscoverProfile(ctx context.Context, p config.ModelProfile) (config.ProfileMetadata, error) {
	if err := p.Validate(); err != nil {
		return config.ProfileMetadata{}, err
	}
	if p.MetadataProtocol == "" {
		if p.Provider == "openrouter" && p.Endpoint == "" && p.ContextLimit == 0 {
			metadata, err := discoverOpenRouter(ctx, p.Model)
			if ctx.Err() != nil {
				return config.ProfileMetadata{}, context.Cause(ctx)
			}
			if err == nil {
				return metadata, nil
			}
		}
		return config.ProfileMetadata{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, MetadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.MetadataURL, nil)
	if err != nil {
		return config.ProfileMetadata{}, err
	}
	if p.CredentialEnv != "" {
		key := os.Getenv(p.CredentialEnv)
		if key == "" {
			return config.ProfileMetadata{}, fmt.Errorf("metadata credential reference %s is not set", p.CredentialEnv)
		}
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: MetadataTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return config.ProfileMetadata{}, fmt.Errorf("read profile metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return config.ProfileMetadata{}, fmt.Errorf("profile metadata returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, MetadataMaxBytes+1))
	if err != nil {
		return config.ProfileMetadata{}, fmt.Errorf("read profile metadata body: %w", err)
	}
	if len(raw) > MetadataMaxBytes {
		return config.ProfileMetadata{}, fmt.Errorf("profile metadata exceeds %d bytes", MetadataMaxBytes)
	}
	var props struct {
		Role                      string `json:"role"`
		DefaultGenerationSettings struct {
			Context int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}
	if err := json.Unmarshal(raw, &props); err != nil {
		return config.ProfileMetadata{}, fmt.Errorf("invalid llama.cpp properties JSON")
	}
	if props.Role == "router" {
		return config.ProfileMetadata{}, fmt.Errorf("router properties do not identify a model's active context; configure a model-specific server")
	}
	if props.DefaultGenerationSettings.Context < 2 {
		return config.ProfileMetadata{}, fmt.Errorf("llama.cpp properties lack a valid active n_ctx")
	}
	hash := sha256.Sum256(raw)
	return config.ProfileMetadata{Verified: true, Source: "llamacpp_props:" + p.MetadataURL + "#sha256=" + hex.EncodeToString(hash[:]), ActiveContext: props.DefaultGenerationSettings.Context}, nil
}

// PrepareProfile applies overrides, server metadata, the versioned catalogue,
// then a labelled conservative fallback. Discovery errors prevent commitment.
func PrepareProfile(ctx context.Context, p config.ModelProfile) (config.ModelProfile, error) {
	metadata, err := DiscoverProfile(ctx, p)
	if err != nil {
		return config.ModelProfile{}, err
	}
	resolved, err := config.ResolveProfile(p, metadata, config.CatalogueMetadata(p))
	if err != nil {
		return config.ModelProfile{}, err
	}
	p = resolved.Profile
	p.ContextLimit, p.ContextSource, p.MaxOutput = resolved.ContextLimit, resolved.ContextSource, resolved.MaxOutput
	return p, nil
}

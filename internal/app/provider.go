package app

import (
	"context"
	"fmt"
	"github.com/sausheong/hand/internal/config"
	"github.com/sausheong/harness/llm"
	"github.com/sausheong/harness/providers/anthropic"
	"github.com/sausheong/harness/providers/gemini"
	"github.com/sausheong/harness/providers/litellm"
	"github.com/sausheong/harness/providers/local"
	"github.com/sausheong/harness/providers/openai"
	"github.com/sausheong/harness/providers/openrouter"
	"os"
)

// BuildProfileProvider constructs the provider shared by CLI and embedded clients.
func BuildProfileProvider(ctx context.Context, profile config.ModelProfile) (provider llm.LLMProvider, err error) {
	defer func() {
		if err == nil && provider != nil {
			provider = withProviderTiming(provider)
		}
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	providerName, baseURL := profile.Provider, profile.Endpoint
	key := ""
	if profile.CredentialEnv != "" {
		key = os.Getenv(profile.CredentialEnv)
	}
	if key == "" && providerName != "local" && providerName != "litellm" {
		return nil, fmt.Errorf("%s is not set (credential reference for provider %s)", profile.CredentialEnv, providerName)
	}
	switch providerName {
	case "anthropic":
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
		}
		return anthropic.NewAnthropicProvider(key, baseURL), nil

	case "openai":
		if key == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is not set")
		}
		return openai.NewOpenAIProvider(key, baseURL), nil

	case "gemini":
		if baseURL != "" {
			return nil, fmt.Errorf("--base-url is not supported for gemini (harness's Gemini provider has no base-URL parameter)")
		}
		if key == "" {
			return nil, fmt.Errorf("GEMINI_API_KEY is not set")
		}
		p, err := gemini.NewGeminiProvider(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("build gemini provider: %w", err)
		}
		return p, nil

	case "litellm":
		if baseURL == "" {
			return nil, fmt.Errorf("litellm requires --base-url (or base_url in ~/.hand/config.json) pointing at your LiteLLM proxy — it has no public default endpoint")
		}
		// LITELLM_API_KEY is deliberately optional, unlike every other
		// provider here: many self-hosted LiteLLM proxies don't enforce
		// auth at all, and forcing a dummy env var just to reach one
		// would be pure friction.
		return litellm.NewLiteLLMProvider(key, baseURL), nil

	case "openrouter":
		if key == "" {
			return nil, fmt.Errorf("OPENROUTER_API_KEY is not set")
		}
		return openrouter.NewOpenRouterProvider(key, baseURL), nil

	case "local":
		// No API key: local model servers (Ollama, LM Studio, llama.cpp's
		// server, vLLM, ...) generally don't authenticate requests.
		return local.NewLocalProvider(baseURL), nil

	default:
		return nil, fmt.Errorf("unknown provider %q (want anthropic, openai, gemini, litellm, openrouter, or local)", providerName)
	}
}

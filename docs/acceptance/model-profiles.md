# Model profiles — development interface

Named profiles are selected at startup with `hand --profile proxy`, or with `default_profile` in the global configuration. In the TUI, `/profile` opens a keyboard picker: use arrows (or j/k), Home/End, Enter to switch, and Escape/Ctrl-C to dismiss. The preview shows the model and configured context/output/reasoning without contacting providers. `/profile <name>` remains available at an idle boundary. Each profile switch builds a fresh client before atomically updating runtime, compaction and UI context. Credentials are referenced by environment variable name; their values are read when constructing the selected client. A named profile does not inherit the legacy endpoint or a provider's conventional credential variable. The existing `model`/`base_url` pair is resolved into an equivalent profile in memory; loading an existing file does not rewrite it for migration.

```json
{
  "default_profile": "proxy",
  "profiles": {
    "proxy": {
      "provider": "litellm",
      "model": "coding-alias",
      "endpoint": "https://proxy.example/v1",
      "credential_env": "CODING_PROXY_KEY",
      "input_types": ["text"],
      "context_limit": 32768,
      "reasoning": "high",
      "reasoning_levels": ["low", "medium", "high"]
    }
  }
}
```

Capability declarations must match the configured server. The alias is a requested model identifier, not verified evidence of an underlying serving model. Explicit context limits reach both runtime and TUI at startup and profile switches. `--context-limit <tokens>` and `--reasoning <level>` override the selected profile for the current invocation; `--reasoning off` disables a configured level. Reasoning is checked against declared support before use. Supported portable levels are limited by installed Harness to off, low, medium and high. Profile max_output now reaches generation requests through the local Harness integration, including fallback requests; omitting it selects the catalogue/server output limit or the 2048 conservative fallback. It must be below an explicitly configured context limit. This requires the pending Harness release.

`--profile` and `--model` cannot be combined. A standalone `--model` selects legacy-style configuration; changing providers clears the legacy endpoint. An explicit `--base-url` overrides the chosen endpoint for that invocation. Profiles reject URL userinfo and queries/fragments; credential_env must contain an environment variable name. Local provider authentication is unsupported; use the appropriate authenticated OpenAI-compatible provider for servers requiring credentials.

The resolver's metadata interface applies explicit overrides before verified server metadata, versioned catalogue metadata and a labelled conservative fallback. Local advertised maxima do not stand in for active server context. Opt-in llama.cpp properties discovery is wired into startup and profile switching, as described below. The built-in catalogue version `2026-09-09.1` supplies documented metadata for exact OpenAI GPT-4o IDs on the default endpoint. Other profiles use the labelled conservative fallback: 8192 context tokens and 2048 output tokens.

Still required: final supported-platform picker qualification, broader metadata discovery and catalogue coverage, actual serving-model provenance, final compaction/profile qualification and release of the configurable-output integration. This development interface does not satisfy all M2.2 acceptance scenarios.

Declared input types are enforced by application Start/Execute before allocating a run ID or invoking the backend. Profile switches update this admission rule under the same operation guard. An omitted declaration preserves legacy input behaviour until capabilities are discovered; it is not evidence of image support. The TUI now reads the runtime context calculation even when no explicit override exists. That fixes a local-provider prefix mismatch, but does not turn heuristic context estimates into verified server metadata.

## Opt-in llama.cpp active-context discovery

A profile may set `metadata_protocol` to `llamacpp_props` and `metadata_url` to its server's complete properties URL, for example `http://localhost:8080/props` alongside endpoint `http://localhost:8080/v1`. Both URLs must share an origin. The reader uses only that profile's credential reference, refuses redirects, limits the response to 1 MiB and imposes a two-second deadline. Configured discovery failures prevent startup/switch completion rather than silently replacing the requested evidence with a guess.

The active limit comes from `default_generation_settings.n_ctx` in the [llama.cpp server properties contract](https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md). Advertised training-context fields are not used as active limits, and router properties are rejected. The decoded metadata carries the source URL and response digest; this is server-reported configuration, not independent model-identity verification. Explicit context overrides retain precedence. The chosen context is applied before runtime construction or atomic profile commit.

Profile switching now runs outside Bubble Tea's update loop. Ctrl+C cancels metadata discovery and prevents a staged configuration from being committed. Terminal closure joins the worker; stale switch messages cannot apply configuration. Discovery for other server protocols, durable metadata provenance and actual serving-model identity remain open.

Application events now carry immutable ModelInfo: selected profile, requested provider/model, context limit and context source. Explicit overrides are labelled; server metadata retains its source URL/response digest; automatic runtime estimates are labelled runtime_heuristic. Requested aggregator aliases never populate serving-model identity. `/usage` shows this provenance for the last recorded run and reports serving identity as unknown while the installed runtime provides no supporting evidence. These in-memory records still need durable persistence and request-level serving identity in later integration.

## Catalogue maintenance and scope

The initial catalogue is compiled into `internal/config/model_catalogue.go`. Version `2026-09-09.1` records the documented 128000 context and 16384 output limits plus text/image input for `openai/gpt-4o`, `openai/gpt-4o-2024-08-06` and `openai/gpt-4o-2024-11-20`, checked against [the provider model page](https://developers.openai.com/api/docs/models/gpt-4o) on 2026-09-09. These entries are documentation-derived defaults, not live measurements or proof of the serving model. Adding or changing an entry requires an exact ID, a primary source, a version increment, endpoint-isolation tests and fresh acceptance evidence.

A custom endpoint, local server, aggregator alias or unknown model ID does not inherit these advertised limits, even when its name contains `gpt-4o`. Configure the active limits explicitly or enable supported server discovery. This intentionally replaces the earlier model-family heuristics in prepared Hand profiles. For models absent from the catalogue, review the fallback and set an appropriate context/output override before demanding workloads.

Resolution applies explicit limits over verified server metadata, then the versioned catalogue, then conservative fallback. Explicit input modalities override catalogue modalities. Output is reduced when necessary to leave input space within a smaller resolved context; an explicit output limit that leaves no input space is rejected. These values reach both profile switches and startup; unknown serving identity remains unknown.

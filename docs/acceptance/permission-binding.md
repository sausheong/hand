# Permission binding across configuration changes

CLI model and profile switches now prepare a separate external authority journal bound to the proposed configuration before committing runtime fields. A journal open or validation failure leaves the existing runtime and binding intact. The active approval hook reads the current binding on each request; TUI and RPC inspection capture that same authority and legacy proposal together.

The identity includes the startup configuration and the selected effective profile. Direct model switching preserves the current credential reference when retaining the provider; a provider change does not inherit it. Returning to an identical configuration reuses its existing grants. Merely preparing a configuration does not select it or import legacy grants.

Opened journals remain owned until application shutdown so permission workers can safely finish against their captured authority. The process retains at most 64 distinct configurations; further new configurations require restart. Switching with scoped authority but without a configured binding factory fails explicitly. The SDK currently uses this guard; dynamic SDK configuration needs its own factory before it can switch scoped authority.

Regression evidence covers configuration separation by model, endpoint and credential reference; preparation without activation; returning to the original grants; closed-owner rejection; atomic failure and success for model/profile switches; and a real approval hook refusing to reuse a previous binding's legacy grant. Affected application, CLI, TUI, RPC and SDK race suites and vet passed. Initial build/fixture failures remain in the evidence manifest; the corrected run is separately identified.

This is development evidence using the unpublished Harness candidate identified in the manifest. The aggregate patch applies to primary Hand but is not integrated there. Fresh full integrated journeys, native isolation, atomic file execution preconditions and final acceptance remain pending.

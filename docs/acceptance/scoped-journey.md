# Scoped SDK approval and revocation journey

The permanent SDK integration test uses a real Hand/Harness runtime and local HTTP provider fixture to request three file writes across separate sessions. The first requires a persistent approval. The second reuses the exact file-write grant without prompting. The client inspects the grant's resource/operation/provenance, revokes it over RPC, then denies the third write when approval is requested again. The file retains its second contents. Exactly six provider requests cover the tool-call and response phases; no live model calls are made.

The final full uncached development race suite passes 771 tests/subtests with no test failures or skips. The three example packages compile and report no test files; they are not counted as passed test cases. Full go vet passes. Raw evidence is indexed in `scoped-journey-integration.json`.

This remains a development checkout using the unpublished Harness candidate recorded in the manifest. It does not satisfy exact clean-candidate, released dependency, coverage, repeated native performance, isolation or live evaluation gates. CLI scoped-policy migration and full M5 acceptance remain pending.

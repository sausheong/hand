# Persistent output artifact references

Tool-result session records now support typed stdout/stderr artifact references, preserving path, captured size, SHA-256 and truncation state. Both serial dispatch and streaming-tool completion record references from completed typed capture metadata. Arbitrary JSON metadata or paths in output prose are not promoted into file references.

The old tool-result constructor remains compatible and writes no artifact field. Existing records remain readable. A regression dispatches an actual bash command producing 70,000 bytes, closes and reopens the session, then verifies the stored capture through the reference and checks every byte.

Targeted runtime/session race tests and vet passed. See `harness-artifact-persistence.json` for hashes and raw evidence. Full regression qualification and Hand viewer wiring remain pending. References retain their original store paths; export/fork does not yet copy process artifacts, and store retention can expire them. Missing artifacts must remain explicit failures.

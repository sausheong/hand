# Model-facing background process tool

The staged `process` tool is registered before runtime construction for interactive and one-shot execution. It shares the invocation-owned registry with terminal commands; all processes are joined on application exit. Commands retain the startup workspace across session switches.

The existing runtime approval hooks gate start, send, cancel and forget. Read, list and wait are non-mutating operations. Unknown actions remain gated and are rejected by execution. Approvals show the process request and disclose host permissions; this is not a sandbox. Existing explicit one-shot auto-approval and saved per-tool grants retain their existing semantics. A grant for bash does not grant process mutations. Configured hook matchers targeting only bash do not match process; policies requiring all shell execution must also match process.

Inputs use a strict single JSON object with unknown fields rejected and a 256 KiB encoded input cap. Command and input writes are independently bounded to 64 KiB by the registry/handle. Wait defaults to one second, supports at most 30 seconds and returns a current snapshot on its own deadline; caller cancellation remains an error. Read returns bounded output with typed artifact references for verified post-completion retrieval through the existing persistence mechanism.

Permanent tests exercise real shell start/input/wait/output/forget, bounded waiting, approval classification and rejection before process creation. Full Hand race-suite and vet evidence is in `process-tool-integration.json`; raw test/subtest events and package outcomes are retained there. This is development evidence, not exact released-candidate qualification.

The aggregate patch excludes the temporary go.mod replacement and requires unpublished Harness commit `540dd08a6975ac0076f697668ca861468715cf44`. Full Harness tests for this candidate, MCP startup changes, native qualification and final acceptance remain pending.

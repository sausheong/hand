# Extension execution follow-ons

Date: 2026-09-09
Status: implementation decision recorded; final acceptance review pending
Plan requirement: M7.D

## Decision

Keep persistent subprocesses and the reviewed container backend as the supported
extension execution models for this plan. Do not add a WASM host, another native
OS sandbox, or a public marketplace as a prerequisite for completing M0–M8.
This decision adds no mandatory implementation follow-on. It does not waive any
existing extension, isolation, packaging, platform or acceptance requirement.

## Demand and capability assessment

The accepted extension use cases are commands, structured questions, context
transformation, lifecycle observation, tool presentation and durable session
state. The shipped Go/Python task-note examples, verification hook, policy hook
and tool viewer cover those categories. Real subprocess, CLI/RPC, PTY and
container journeys now provide development evidence for their execution model.
See the evidence index below. Final qualification of those capabilities remains
required independently of this decision.

No accepted use case has been identified that requires an in-process WASM ABI.
Potential additional demand includes running many small untrusted transforms
without Docker, distributing one portable compiled guest to multiple operating
systems, or deploying where subprocess creation/container services are prohibited.
These are concrete reasons to revisit the decision, but they are not currently
requirements in this plan or demonstrated deployment constraints.

Arbitrary Python/native libraries and external executables are already usable
through reviewed host execution or an image-bound interpreter. A WASM path would
need a separate guest compilation/runtime compatibility story. It must not replace
those accepted workflows or be described as an automatic way to run existing
Python packages unchanged.

## Measurements

The reproducible runner is `scripts/benchmark_extension_processes.py`. It launched
both shipped task-note peers 30 times each, interleaved using seed 709, and executed
ten question/cancel round trips per process. Every protocol response was checked.
The machine reported macOS 26.6.2, arm64; Python executable identity and source
hashes are in the evidence. Filesystem caches were not flushed. All samples and
the initial sandbox-restricted attempt are retained.

| Metric | Go example | Python example |
| --- | ---: | ---: |
| Startup median | 2.459 ms | 16.857 ms |
| Startup p95 | 2.856 ms | 33.629 ms |
| Startup maximum | 3.304 ms | 183.890 ms |
| Question/cancel round-trip p95, 300 samples | 0.080 ms | 0.116 ms |
| Resident memory median | 4,688 KiB | 16,976 KiB |
| Resident memory p95 | 4,736 KiB | 17,104 KiB |
| EOF shutdown p95 | 1.520 ms | 4.539 ms |

Startup covers process creation through the initialization reply. RSS was sampled
with `ps` after ten calls; it is not peak, proportional or private memory. The
shutdown number includes the Python runner's process-wait polling overhead. These
are small example workloads, not large extensions, cold-cache measurements,
whole-Hand latency, container startup, cancellation acceptance or Linux results.
The Python startup outlier remains in the dataset. No confidence interval or
cross-model superiority claim is made.

The built Go peer is 3,023,650 bytes. The Python script is 3,339 bytes; its measured
launcher is 135,696 bytes, which excludes the Python framework, standard library
and shared libraries. Comparing those two executable sizes as complete deployment
footprints would be misleading. Host Python requires an installed runtime;
container Python requires a pinned image containing that runtime. Current package
reviews and approved version probes make those dependencies explicit.

## WASM runtime/library and integration cost

Wazero is a credible Go-native candidate: its official documentation describes a
pure-Go runtime without CGO or platform library prerequisites. Its execution model
requires imported host functions for resources, commonly through WASI. Those
properties reduce native build dependencies but do not implement Hand's extension
contract by themselves. Sources: [wazero](https://wazero.io/) and
[architecture and host access](https://wazero.io/docs/), consulted 2026-09-09.

A production integration would add a runtime dependency and guest toolchain/ABI,
module loading and compilation/cache policy, memory/execution limits, cancellation,
capability-scoped host imports, state/question/presentation bindings, package
compatibility metadata and another recovery/platform/conformance test matrix.
Native libraries or subprocess-dependent guests would need adaptation. These are
engineering and maintenance costs, not measured elapsed-time estimates.

No WASM runtime was added or benchmarked in this decision. Its incremental binary
size, compilation cost and guest memory are therefore unknown, and no relative
speed or memory advantage is asserted. Current subprocess measurements show no
example-workload bottleneck that establishes a new WASM requirement. If one of the
unmet deployment/use-case constraints above becomes real, compare the same
representative transforms using a pinned WASM runtime before changing this decision.
Include compile/instantiate time, warm-call latency, RSS/private memory, module
limits, host-import attacks, cancellation and total distribution size.

## Isolation and distribution scope

Host subprocess capabilities regulate host-mediated callbacks; they are not an OS
sandbox. A trusted host process retains its OS user's authority. Reviewed container
launches pin the image and resources, run as a non-root user with reduced privileges,
limit resources, and enforce the selected workspace/network policy. They require a
container service. The model-provider connection remains on the host, as disclosed
by Hand's runtime boundary. Existing container security and recovery gates still
apply.

WASM memory isolation would provide a different boundary: guest access depends on
which imports the embedding host exposes. It would not make careless host imports
safe. A native OS sandbox could reduce Docker dependency but would introduce
platform-specific enforcement and parity work. Neither is selected without the
additional demonstrated requirement described above.

Explicit local/Git/archive package installation, immutable pins, capability review,
rollback and conformance remain required. A public marketplace is not needed for
those workflows. Discovery/ranking, publisher identity, signing, revocation,
abuse handling and service operation would be a separately scoped product, not
an implicit milestone completion condition.

## Evidence and completion implications

- `docs/acceptance/extension-process-benchmark.json`: raw sample hashes and metrics.
- `docs/acceptance/container-package-rpc.json`: installed Python package, explicit
  runtime approval, removed source, restart replay and cancellation.
- `docs/acceptance/container-extension-terminal.json`: real terminal interaction,
  persisted state, cancellation and reviewed reload.
- `docs/acceptance/container-callback-integration.json`: native container write
  approval and read-only boundary enforcement.
- `docs/acceptance/container-broad.json`: scoped regression reconciliation, with
  its precise source version and qualification limitations.

This closes the implementation decision's scope question. M7.D remains in progress
until the final traceability/review gate accepts the decision and its evidence on
the actual release candidate. It does not make any other milestone complete.

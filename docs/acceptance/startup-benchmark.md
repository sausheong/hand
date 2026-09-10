# Startup baseline procedure

Run `python3 scripts/benchmark_startup.py --output /absolute/path/new-output-directory` with the normal Go build environment. The output directory must be new and outside the checkout. The default collects 30 trials for each case: no MCP, a local MCP server, a server delayed by one second, and an unavailable server executable. Use `--trials 1` for probe calibration only; it is insufficient for the baseline.

The script builds Hand and the existing stdio MCP fixture, records source hashes, commit, Harness module identity, toolchain, hardware/platform, terminal dimensions and binary size/hash. Every trial gets fresh temporary HOME and workspace directories, an explicit local model endpoint at a closed loopback port and no submitted model request. API-key environment variables are removed. The PTY is 120 columns by 40 rows with TERM=xterm-256color. OS caches are not flushed; this measures repeated fresh-process startup, not cold-boot behaviour.

Timing begins immediately before process creation. After the real input placeholder appears, the probe verifies terminal echo is disabled, types a marker without submitting it and waits for the application to render that marker. This measures startup to usable input, including rendering, rather than merely observing the banner. It then sends Ctrl-C and joins the process. The collector kills any remaining process-group descendants as cleanup; this is not a process-leak acceptance test.

Each trial records the raw PTY transcript and SHA-256, input-readiness time (or null), exit code, elapsed time and wait4 maximum resident memory. On macOS that RSS field is bytes; Linux KiB is converted to bytes. It is OS process accounting at exit, not a measurement of simultaneous aggregate process-tree memory. A 4 MiB terminal-capture cap and per-trial deadline prevent a defective probe from growing indefinitely. Probe tests reject terminal echo masquerading as application input and verify a hung process is killed and joined.

The unavailable-MCP case records the existing startup failure and null input-readiness measurement. Its observation can be valid even though this behaviour does not satisfy future optional-MCP startup acceptance. No failure latency is substituted for a successful readiness measurement. When M3 introduces optional MCP startup, version the probe expectation and compare genuinely usable-input observations.

The report uses nearest-rank p95 over real successful readiness observations. Keep raw trials, platform/configuration, workload and measurement procedure identical for the later no-MCP comparison. A source change during collection invalidates the run. These baselines do not establish long-transcript key latency, cancellation latency, native Linux qualification or superiority to Pi.

## Recorded reference baseline

Use `--source /absolute/path/checkout` to measure an isolated Git revision. The original reference was measured from a clean detached checkout of `e6dd26351c1b1436616e1c2b8da0086a3580d882`, with Harness v0.3.9, on Apple M4 Max with 64 GiB memory. Each case has 30 observations. The ordinary baseline collection initially stopped because the sandbox denied CPU identification; that failed attempt collected no trials. The successful runs used approved read-only hardware access.

| Case | Original input-ready p95 | Development input-ready p95 |
|---|---:|---:|
| No MCP | 55.51 ms | 55.60 ms |
| Fast local MCP | 68.01 ms | 65.04 ms |
| One-second delayed MCP | 1,076.63 ms | 1,076.70 ms |
| Unavailable MCP | No input readiness | No input readiness |

The original binary was 43,001,010 bytes; development was 43,179,586 bytes. Observed original peak process RSS ranged from 30,785,536 to 33,259,520 bytes across cases. These are local baseline observations, not a speedup claim or full process-tree memory measurement.

The plan's no-MCP comparison limit is `55.50895805936307 × 1.2 + 50 = 116.61074967123568 ms`, subject to identical reference hardware and configuration. Optional-MCP nonblocking startup remains a separate required behaviour; these current blocking results do not satisfy it.

Durable raw evidence is in `baseline/startup-original-20260908.tar.gz` and `baseline/startup-development-20260908.tar.gz`, with archive digests in `baseline/startup-archives.json`. Each contains report.json, build.log and all 120 raw PTY transcripts. Reports include binary hashes; compiled binaries are omitted from the archive and can be rebuilt from the recorded source. Extract into a fresh directory and verify each transcript against its report hash before using it as evidence.

## Optional MCP development measurement, 9 September

Use `--optional-mcp` with the same runner to set the fixture server optional and require usable input even for an unavailable executable. Default mode preserves required-MCP baseline expectations. Thirty fresh-process trials per case passed with the current source: no MCP p95 88.26 ms, fast 86.70 ms, one-second-delayed 93.91 ms, unavailable 92.55 ms. Hardware, platform, toolchain and terminal fields match the archived original baseline; no-MCP p95 is within its 116.61 ms threshold. Raw transcripts and source report are indexed by `startup-optional-performance.json`. No model requests were submitted; this dirty development result is not final-candidate qualification.

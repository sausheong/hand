# Background process registry development evidence

The staged application registry owns up to eight running processes and 32 retained records. Stable handles support output snapshots, bounded stdin, cancellation, waiting and explicit forgetting of completed records. Cancelling a foreground caller or its wait does not cancel an admitted background process. Closing the registry cancels and joins all owned processes, including concurrent admission.

Three permanent regression tests cover interactive input and nonzero exit capture, foreground independence, active and retained capacity, forgetting and concurrent shutdown. Twenty repetitions passed under the race detector: 60 test events, zero failures or skips. The first fixture run failed because its pre-existing directory was not private; the corrected fixture lets the capture store create its own private subdirectory. Both runs are preserved in `process-registry-integration.json`. The earlier compile check ran two existing output tests and is not registry acceptance evidence.

`process-registry-integration.patch` is the aggregate staged Hand change, excluding the local go.mod replacement. It requires development Harness commit `540dd08a6975ac0076f697668ca861468715cf44`; primary Hand still uses v0.3.9. No dependency has been published.

Production control wiring, policy admission, MCP startup behaviour, full suites and native qualification remain outstanding. This evidence does not close M3.4.

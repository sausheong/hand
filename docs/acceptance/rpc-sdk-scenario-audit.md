# RPC and SDK scenario audit

All 14 required scenarios are mapped below. None is marked finally qualified: clean-candidate, released-dependency and required platform evidence remain open. Source fingerprints are in `rpc-sdk-scenario-audit.json`. A mapped test is not proof that its complete acceptance scenario passed.

| Requirement / scenario | Existing development scope | Remaining proof |
|---|---|---|
| M4.1 / stdout_json_only | Python client parses framed stdout from built Hand; transport handles negotiation. | Refresh built-client run on final candidate and explicitly inventory every stdout line. |
| M4.1 / non_go_client_lifecycle | Python journey asserts approval, write, steering, cancellation and reconnect terminal recovery. | Fresh exact-candidate built journey on required platforms. |
| M4.1 / approval_steer_cancel | Python fixture checks approved file contents, steering continuation and two cancelled provider calls. | Final native runs and required concurrency repetitions. |
| M4.1 / version_size_errors | Framing unit tests and built Python version rejection checks exist. | Built oversized-frame oracle now passes with empty ledger/zero provider calls; final native candidate qualification remains. |
| M4.1 / duplicate_request_id | Concurrent duplicate admission, restart and backend call-count assertions exist. | Final repeated native qualification and built-client bindings. |
| M4.1 / no_sideeffect_replay | Built Python checks approved write once and restore replay preserving later user edits; durable control replay tests exist. | Fresh final-candidate journey and all control side-effect scenario reconciliation. |
| M4.1 / slow_consumer_terminal_retained | Combined regression verifies eviction, write timeout, ledger reopen, exact terminal and no repeated file side effect (20 race repetitions). | Built-process and final native platform/candidate qualification remain. |
| M4.1 / disconnect_cleanup | Transport checks joined backend on disconnect; Python observes cancelled provider requests. | Actual child-tree cleanup within five seconds across required native platforms, final candidate. |
| M4.1 / no_hidden_trust_prompt | Noninteractive RPC approval and SDK scoped-authority journeys exist. | Explicit unattended cold-start trust/permission refusal oracle and final built-client evidence. |
| M4.2 / external_module_build | External module runner builds with public SDK/protocol imports using local checkout dependencies. | Repeat for clean candidate; released dependency mode remains required. |
| M4.2 / released_package_example | Runner has version mode and rejects replacement dependencies/mismatched binary dependency inventory. | Actual released Harness integration and released-package example qualification; development replaces are insufficient. |
| M4.2 / cli_rpc_sdk_equivalence | Three-interface local-provider completion comparison exists. | Fresh candidate completion/cancellation evidence and remaining approved-tool/workspace-effect equivalence audit. |
| M4.2 / context_cancel_cleanup | Cooperative close, uncooperative cancellation, descendants and blocked client reads have permanent tests. | Final native platform/process-tree timing evidence; do not substitute cancellation signal for all-child cleanup. |
| M4.2 / compatibility_fixture | Public compatibility executable plus immutable snapshots, identity and outcome regressions exist. | Freeze fixture/version matrix and rerun against exact released dependencies; aggregate coverage still below threshold. |

The combined in-process slow-consumer recovery gap is covered by `slow-consumer-recovery.json`. Built-process and final platform qualification remain open.

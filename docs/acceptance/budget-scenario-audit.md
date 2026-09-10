# Budget scenario audit

All nine M6.3 scenarios have development test mappings. None is finally qualified. Test locations identify assertions; the linked historical runs do not certify the current dirty candidate.

| Scenario | Existing assertion scope | Remaining qualification |
| --- | --- | --- |
| `retry_summary_in_ledger` | Durable retry reservation, independent summariser route, and manual compaction admission are covered. | Reconcile fallback/cache/provider retry cases across real built-client journeys on final candidate. |
| `reservations_concurrent` | Two ledger owners compete for one remaining balance; exactly one reservation succeeds. | Final race repetitions and released Harness evidence; audit all concurrent runtime admission routes. |
| `strict_unknown_price_blocks` | Unknown strict price blocks provider dispatch. | Final built CLI/RPC refusal evidence for missing, stale and unbounded route tariffs. |
| `advisory_unknown_visible` | Unknown advisory charge remains in ledger and bounded view with disclosure. | Final integrated UI/RPC evidence for advisory unknown charge, not only controller assertions. |
| `exhaustion_stops_new_work` | Overrun stops admission; all budget causes produce unverified exhaustion; validator deadline blocks continuation. | Reconcile owned provider/tool/descendant cancellation trials and final outcome evidence for every limit scope. |
| `explicit_resume_budget` | Explicit new limits resume without erasing charges; ambiguous confirmations cannot change decisions. | Fresh same-candidate complete resume journeys, with prior failed/uncertain state retained. |
| `ledger_restart` | Disk session reopen retains reservation, category amounts, unknown attempts and selected run budget. | Final process-crash and platform suite provenance including persistent token/time/cost scopes. |
| `pricing_provenance` | Route/source/version snapshots persist; confirmation and replay protect updates; ambiguous tariff input is refused. | Released dependency, final tariff validity/route audit, and fresh fuzz plus integrated review evidence. |
| `in_flight_uncertainty` | Partial cancelled usage does not release reservation; outstanding and unknown charges remain visible after restart. | Final non-cancellable provider and disconnect/restart evidence; no exact invoice-cap claim. |

The JSON audit records exact test names, source hashes and supporting reports.

Remaining cross-cutting work:
- Budgets measured 81.3861%, above user-revised 80% threshold; final candidate and platform/source qualification still required.
- Released Harness integration and clean Hand candidate
- Full plan-bullet traceability including cache effectiveness and run/session/time/token/cost combinations
- Fresh final platform, concurrency, fuzz, journey and semantic acceptance evidence

Fallback integration now covers unknown primary token/cost reservations, cached-input subset accounting, durable budget reopen, and rejection before fallback dispatch for missing/expired tariffs or exhausted cost ceiling. See `fallback-budget-accounting.json` and `fallback-cost-refusal.json`; built-client and final-candidate qualification remain required.

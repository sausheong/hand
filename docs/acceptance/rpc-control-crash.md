# Killed-writer recovery for RPC controls

The existing real-process crash fixture now covers control-operation ledger records at pending intent, accepted operation and completed response boundaries. The parent waits for the durable boundary, kills the child without closing its ledger, then reopens the file and confirms that execution is never granted again. Pending/accepted controls become uncertain; completed responses and operation identities are preserved.

Control responses are validated for matching protocol version/request identity and exactly one result or error. Invalid completed responses cannot be appended. Dispatcher persistence failures now construct a fresh error response so a successful mutation result cannot accidentally remain alongside its error.

The final uncached RPC race suite passes 34 tests/subtests, including both run and control killed-writer scenarios. Vet and the CLI build pass. The Python client lifecycle runner also passes against the rebuilt binary: approval-before-write, steering, queue operations, duplicate run replay, cancellation, disconnect and reconnect recovery, using five local provider calls and two cancelled requests.

These tests cover process termination at durable ledger boundaries. They do not establish whole-system power-loss behaviour or final native/released-candidate qualification. Evidence is recorded in `rpc-control-crash-integration.json`.

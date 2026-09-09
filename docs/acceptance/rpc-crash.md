# RPC ledger process-kill recovery

The permanent subprocess fixture opens the real ledger, performs a durable transition and writes a readiness marker after that transition returns. The parent then kills the child while it still owns the open ledger and exclusive writer lock; the fixture never calls Close. Three barriers cover pending intent, accepted run binding and completed result.

After child termination, the parent opens the same ledger and repeats the same prompt request. None of the three cases receives a new execution grant. Pending/accepted records are uncertain; accepted run identity remains available. Completed records retain their exact result. Successful reopen also checks that the terminated writer's OS lock does not remain held.

The full ledger race suite and vet passed. Raw test/subtest events and aggregate source hashes are in `rpc-crash-integration.json`. These local process-kill checks are not power-loss simulation, cross-platform qualification or proof of a connected RPC dispatch journey. M4.1 remains in progress.

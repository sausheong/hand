# Scoped permission RPC inspection and revocation

When an application controller has scoped authority, hello advertises permission.list and permission.revoke. The embedded SDK connects its optional AuthorityDirectory to that controller. Applications without scoped authority reject these operations clearly.

permission.list accepts an offset and returns grants, next and total, bounded to 16 grants and 512 KiB per response. Entries expose the grant's operation/resource, workspace/configuration identity, lifetime and provenance. permission.revoke accepts a grant id and syncs its revocation before replying. Repeated identical request IDs replay the original response through the durable control ledger.

Tests verify that inspection reports the saved grant exactly, revoked scopes immediately fail subsequent checks, lists omit revoked grants, duplicate revocations preserve the response, and scoped SDK clients can inspect permissions. The RPC race suite, scoped SDK lifecycle test and vet pass.

Revocation does not undo operations already executing. CLI migration/inspection UI and full end-to-end acceptance remain pending. Evidence: `permission-rpc-integration.json`.

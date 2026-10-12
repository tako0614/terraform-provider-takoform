# Private intent snapshots in a Runner

The v2 Host Provider has two explicit, mutually exclusive intent-store modes:

- `store_path` opens an already initialized private filesystem store. Its
  `Initialize`/`InitializeBound` and `Open` behavior is unchanged; Configure
  never creates or adopts a missing local path.
- `intent_store_slot` selects an opaque private file supplied by the Runner.
  The Provider also requires `store_id` and `host_api_root`. The slot is not a
  Host resource or a storage-provider selection. It is never inferred from
  the Host endpoint, Workspace, Run ID or product name.

For remote mode, the Runner supplies `TAKOSUMI_PRIVATE_RUN_FILES` as a JSON
array of `{ "slot": "...", "endpoint": "...", "grant": "..." }` objects. The
selected endpoint is the exact internal base
`/internal/private-run-files/v1`; the Provider sends `POST` requests to its
`/read`, `/commit` and `/resolve` children with the opaque grant as a Bearer
header. HTTPS is required except for an explicitly loopback HTTP endpoint for
local/self-hosted integration. Redirects are never followed. The environment
value and grant are process inputs only: neither is included in an intent
snapshot, Terraform state, a Host request, or a diagnostic.

`read` returns an absent marker or `{kind:"present",version,digest,bytesBase64}`.
The Provider verifies the SHA-256 digest and runs the existing strict
snapshot/store-ID/Host-binding validation before it interprets any intent.
For an absent slot only, Configure conditionally writes an empty snapshot at
expected version zero. A concurrent initializer may win; the loser accepts
only a subsequently read, valid snapshot with the same store ID and exact
Host binding. Foreign or malformed custody is never reset or overwritten.

Every subsequent mutation reads the current version, applies the same
`intentv2.Store` business rules as local mode, and commits the new snapshot
with that expected version and a fresh commit ID. The Provider returns an
intent, including its original Host idempotency key, only after an exact
commit receipt is acknowledged. If a commit response is lost or unknown, it
issues one `/resolve` for that exact commit ID, expected version and digest.
Only an exact committed receipt qualifies. Conflict or unresolved outcomes
return a value-free error and do not authorize a Host send or a blind mutation
retry. A competing writer can therefore produce one winner, not two Host
claims. The snapshot limit is 16 MiB decoded.

The Core, not this Provider, enforces grant scope, current Run lease and
execution-authority epoch in its atomic private-file commit. This backend is
an internal Runner custody integration; it does not change a published Host
API, Form, or Provider resource state schema. It does not by itself prove
Runner dispatch, Cloud D1/R2 durability, or production readiness.

# Conformance

The current Phase 0 evidence is executable Go characterization:

- `internal/provider/*_test.go` asserts the exact ten-resource registration, typed schema behavior, validation, CRUD, import, state refresh, and the absence of plan-time remote mutation;
- `internal/client/client_test.go` asserts discovery, capability negotiation, preview/apply evidence, error envelopes, observation, and deletion;
- `examples/resources/` contains one formatted HCL example for every registered resource.

Run:

```console
go test ./...
go vet ./...
tofu fmt -check -recursive examples
go run ./cmd/conformance verify
go run ./cmd/migration-proof
```

Provider publication uses `go run ./cmd/migration-proof --require-complete`.
Unlike the structural local command, release mode exits nonzero for every
`external-required` phase or remaining external blocker.

## Phase 0/1 golden characterization

`compatibility-candidate-v1/` freezes the current ten-kind provider/client
behavior as deterministic offline evidence. It contains JSON Schemas and
fixtures for the provider schema, desired and observed Resource envelopes,
sanitized output projection, provider import IDs, provider/API errors, and host
discovery. The discovery dependency schema is vendored into the same evidence
set. `cmd/conformance` uses the pinned Draft 2020-12 implementation to compile
every schema and validate every fixture without network or parent-directory
resolution. It rejects unknown or escaping references, file or per-kind digest
drift, an incomplete kind set, malformed evidence, publication-ready claims,
and portable-standard claims.

The case digest uses Go `encoding/json` normalization only to detect fixture
drift. It is not a portable canonical serialization algorithm and is not a
definition identity. This evidence is neither a signed package nor a standard
form release. The actual provider and HTTP client parity tests consume these
fixtures so checked-in evidence cannot drift away from the current executable
wire behavior. Provider parity fingerprints validator allow-lists/configuration,
default behavior, and concrete plan-modifier semantics in addition to attribute
types and flags, and invokes every real resource `ImportState` handler.

## Data-only Form Package v1 corpus

[`form-package-v1/`](form-package-v1/) is a separate corpus for the portable
package layer. It includes one valid closed ExampleStore package plus ten
independent `0.0.0-legacy.1` compatibility packages for the current typed
provider inventory. Each legacy package has one exact FormRef, one definition,
one positive desired fixture, closed desired/observed schemas, and no host
authority fields. Tests pin every package/schema digest, reject an unknown host
extension and one kind-specific invalid fixture, and cross-check the exact
machine-readable set in
[`../forms/legacy-package-set.json`](../forms/legacy-package-set.json).

The same corpus contains negative fixtures for duplicate names, invalid Unicode
scalar sequences, negative zero, credentials, operator fields, target/capacity,
price/billing/SKU, executable code fields, plural API/private/SSH/signing key
and manager identifier derivatives, traversal, absolute paths, and backslashes.
The ExampleStore package also fixes boundary-safe words such as `apiKeysight`,
`privateerKeys`, and `managerialIds`. Filesystem-only symlink, executable-bit,
and device/pipe cases are covered by library tests. Unit tests additionally
prove linear admission of a shared-reference DAG, fail-closed schema proof
depth/operation limits, the 16,384-evaluation fixture-validation budget through
the real directory verifier, cardinality amplification through `items`,
`contains`, `additionalProperties`, and `propertyNames`, embedded content
transformation rejection, and the 32-fixture Form Definition limit.

Run it with:

```console
go run ./cmd/form-package conformance
```

The current manifest result is 11 positive packages and 43 negative cases.
Passing this corpus proves the local data contract only. It is not signature,
publisher, remote-install, host-activation, retention/revocation, lifecycle
idempotency, or cross-host/kind-standardization evidence. Those later trust and
host conformance layers remain unimplemented.

## Portable host and provider migration evidence

`portable-host-v1/` pins the versioned discovery/API paths, exact ObjectBucket
FormRef/package identity, concurrency/idempotency rules, stable error taxonomy,
and required cross-repo black-box runner checks. The provider client consumes
the same contract in adversarial HTTP tests.

`provider-migration-v1/` contains a redacted backup for the six types actually
exposed by the old Takosumi provider, a separate all-ten Takoform golden state,
an explicit provider/type mapping, and structural backup/import/rollback
evidence. The four new-only kinds are not represented as fictional
`takosumi_*` migrations. The verifier rejects provider-address aliasing and
secret, price, or backend data in new state. It also compares state lineage,
schema version, and every overlapping desired attribute across the six mapped
resources. Live old/new and rollback refresh no-op proof remains explicitly
external because it requires the pinned old provider artifact, its exact lock
and HCL migration input, and a reachable operator migration host; the release
workflow therefore stays fail-closed until those phases have machine evidence.

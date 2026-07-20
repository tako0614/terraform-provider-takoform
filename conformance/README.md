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

## Actual provider protocol lifecycle candidate

`cmd/provider-lifecycle-conformance` builds the real provider binary and drives
all ten typed resources through a Terraform-compatible CLI against an in-process
versioned Form host. The generic candidate covers create, read plus observe,
mutable update with state-generation fencing, explicit refresh, native import,
CLI import, drift mapping, delete, exact response-identity rejection, and
replacement plans for immutable names, SQL engine, and vector dimensions.
The data-only candidate report binds the CLI product/version, exact canonical
provider FQN, provider schema digest, embedded ten-Form candidate-set digest,
release-descriptor provider version/binary digest, CLI executable basename,
and CLI binary SHA-256 without leaking a host-local path. CI
runs the reviewed OpenTofu and Terraform versions as one fail-closed matrix:

```console
go run ./cmd/provider-lifecycle-conformance verify --cli tofu
go run ./cmd/provider-lifecycle-conformance verify --cli /path/to/terraform
go run ./cmd/provider-lifecycle-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli terraform --output-dir /tmp/takoform-provider-reports
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli tofu --output-dir /tmp/takoform-opentofu-provider-reports
```

`provider-reports` first authenticates the exact ten-package retained
publication closure under `admission/v1/releases/`. It reads each canonical
positive desired fixture and `reject-invalid-semantics` desired fixture from
the retained release archive, projects those exact values into the typed
provider configuration, and executes both through provider protocol v6. The
positive fixture must apply and delete successfully. The negative fixture must
return a provider diagnostic before the in-process Form host receives any
mutation; that rejection is normalized to portable `invalid_argument`.

The command combines those per-package observations with the independently
executed full lifecycle checks and writes one strict RFC 8785
`takoform.standard-runner-report@v1` document with `role: provider-report` per
kind. It refuses to write under `admission/`, signs nothing, publishes nothing,
and does not change `external-required` admission status. Its output directory
must be new or empty. Authentication and admission remain separate protected
release decisions. Each report subject records the executing CLI's exact
distribution identity: Terraform uses
`provider:registry.terraform.io/tako0614/takoform`, while OpenTofu uses
`provider:registry.opentofu.org/tako0614/takoform`. The two addresses are not
interchangeable Registry sources.

The matrix is intentionally classified `generic-lifecycle-candidate` with
`publicationReady: false` and
`bindingStatus: exact-structural-candidate-set`. It does not publish a
checked-in passed report or claim standard Form admission. The matrix requires
Terraform `1.15.8` under the canonical identity
`registry.terraform.io/tako0614/takoform` and OpenTofu `1.12.1` under the
dual-published alternative identity
`registry.opentofu.org/tako0614/takoform`, then requires identical provider
schema, exact FormRef/package identity, and lifecycle evidence. The exact FQN is
retained as state identity; switching distributions requires
`state replace-provider` and never occurs as a silent alias. Immutable
release/readback plus authenticated signed external admission are still
required before these structural candidates can become portable standards.

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

`form-package-v1/positive/standard/` contains the separate ten-package `1.0.1 /
standard` definition candidate set. It does not replace or mutate the legacy compatibility
identities. `go run ./cmd/standard-form-conformance verify` validates package
bytes and fixtures and inspects the actual provider resource structure. It does
not run the Terraform protocol lifecycle or a Takosumi host, and this repository
intentionally contains no locally synthesized passed admission JSON.

The machine-readable inventory classifies the set `structural-candidate`, marks
local coverage `structural-only`, and admission `external-required`. Definition
`status: standard` pins the proposed final bytes; it is not an admission claim.
The local dual-CLI/FQN provider lifecycle matrix and Takosumi host fixture proof
cover the candidate set, including portable negative wire-code coverage
(`invalid_argument`). Signatures/provenance, immutable release tags, Registry
installation/readback, and authenticated signed admission evidence remain
external requirements. Only
that authenticated evidence may classify the exact package as
`portable-standard`.

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

The corpus also contains `positive/interface-declaration`. It proves exact
`(name, version)` identity (including two versions with one name), exact
non-secret document validation, `required` metadata, literal/output/host
mapping sources, explicit JSON `null`, and RFC 6901 root and escaped pointers.
Negative cases cover mapping grammar, invalid pointer escapes, and documents
that do not satisfy their declared schema. Materialization, authorization, and
lifecycle remain host work.

The current manifest result is 22 positive packages (one ExampleStore, ten
legacy compatibility candidates, ten structural standard packages, and one
interface-declaration package) and 51 negative cases.
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

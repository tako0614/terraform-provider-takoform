# Conformance

The current Phase 0 evidence is executable Go characterization:

- `internal/provider/*_test.go` asserts that the resource set is exactly the declared Form set, and covers typed schema behavior, validation, CRUD, import, state refresh, and the absence of plan-time remote mutation;
- `internal/client/client_test.go` asserts discovery, capability negotiation, preview/apply evidence, error envelopes, observation, and deletion;
- `examples/resources/` contains one formatted HCL example for every registered resource.

Run:

```console
go test ./...
go vet ./...
tofu fmt -check -recursive examples
```

## Actual provider protocol lifecycle candidate

`cmd/provider-lifecycle-conformance` builds the real provider binary and drives
every declared typed resource through a Terraform-compatible CLI against an in-process
versioned Form host. The generic candidate covers create, read plus observe,
mutable update with state-generation fencing, explicit refresh, native import,
CLI import, drift mapping, delete, exact response-identity rejection, and
replacement plans for immutable names, SQL engine, and vector dimensions.
The data-only candidate report binds the CLI product/version, exact canonical
provider FQN, provider schema digest, embedded candidate-set digest,
release-descriptor provider version/binary digest, CLI executable basename,
and CLI binary SHA-256 without leaking a host-local path. CI
runs the reviewed OpenTofu and Terraform versions as one fail-closed matrix:

```console
go run ./cmd/provider-lifecycle-conformance verify --cli tofu
go run ./cmd/provider-lifecycle-conformance verify --cli /path/to/terraform
go run ./cmd/provider-lifecycle-conformance matrix --opentofu tofu --terraform terraform
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli terraform --output-dir /tmp/takoform-provider-reports \
  --source-commit "$(git rev-parse HEAD)"
go run ./cmd/provider-lifecycle-conformance provider-reports \
  --cli tofu --output-dir /tmp/takoform-opentofu-provider-reports \
  --source-commit "$(git rev-parse HEAD)"
```

`provider-reports` first authenticates the exact retained
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
release decisions. Each report subject records the one canonical provider
identity, `provider:registry.terraform.io/tako0614/takoform`, regardless of
whether OpenTofu or Terraform executes it.

The matrix is intentionally classified `generic-lifecycle-candidate` with
`publicationReady: false` and
`bindingStatus: exact-structural-candidate-set`. It does not publish a
checked-in passed report or claim standard Form admission. The matrix requires
Terraform `1.15.8` and OpenTofu `1.12.1` under the same canonical identity,
`registry.terraform.io/tako0614/takoform`, then requires identical provider
schema, exact FormRef/package identity, and lifecycle evidence. Immutable
release/readback plus authenticated signed external admission are still
required before these structural candidates can become portable standards.

## Schema derivation gate

The provider schema, the Form Definition, and the conformance fixtures are all
derived from one declaration, so drift between them is a build failure rather
than something a checked-in fixture corpus has to notice later.
`go run ./cmd/standard-form-conformance verify` regenerates nothing: it reads
the committed packages, re-verifies their bytes, and inspects the actual
provider resource schema for every declared Form, including that each field the
definition calls immutable really forces replacement.

## Data-only Form Package v1 corpus

[`form-package-v1/`](form-package-v1/) is a separate corpus for the portable
package layer. It includes one valid closed ExampleStore package. Each package
has one exact FormRef, one definition, one positive desired fixture, closed
desired/observed schemas, and no host authority fields. Tests pin every
package/schema digest and reject an unknown host extension and one
kind-specific invalid fixture.

`form-package-v1/positive/standard/` contains the generated package for every
declared Form. None of them replaces or mutates a published identity: a
retained kind token starts a new major line instead.
`go run ./cmd/standard-form-conformance verify` validates package
bytes and fixtures and inspects the actual provider resource structure. It does
not run the Terraform protocol lifecycle or a conforming host, and this repository
intentionally contains no locally synthesized passed admission JSON.

The machine-readable inventory classifies the set `structural-candidate`, marks
local coverage `structural-only`, and admission `external-required`. Definition
`status: standard` pins the proposed final bytes; it is not an admission claim.
The local dual-CLI/FQN provider lifecycle matrix and conforming-host fixture proof
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
non-secret document validation, `required` metadata,
literal/output/resource-URI/host
mapping sources, explicit JSON `null`, and RFC 6901 root and escaped pointers.
Negative cases cover mapping grammar, invalid pointer escapes, and documents
that do not satisfy their declared schema. Materialization, authorization, and
lifecycle remain host work.

The current manifest result is 36 positive packages (one ExampleStore, one
interface-declaration package, and one generated package per declared Form) and
51 negative cases. The separate `data-indexed-v1` corpus adds six positive
request operations, seven HTTP 200 response shapes, two HTTP 409 conflict
shapes, and bounded negative request/response cases. Its manifest pins both
canonical schemas and the 200/409 association.
Passing this corpus proves the local data contract only. It is not signature,
publisher, remote-install, host-activation, retention/revocation, lifecycle
idempotency, or cross-host/kind-standardization evidence. Those later trust and
host conformance layers remain unimplemented.

## Portable host evidence

`portable-host-v1/` pins the versioned discovery/API paths, exact ObjectBucket
FormRef/package identity, concurrency/idempotency rules, stable error taxonomy,
and required cross-repo black-box runner checks. The provider client consumes
the same contract in adversarial HTTP tests.

# Conformance

The retained v1alpha2 candidate compatibility evidence is executable Go
characterization:

- `internal/provider/*_test.go` asserts that the published provider v2.0.0
  exposes exactly the
  nine independently authored v1alpha2 candidates, and covers typed schema
  behavior, validation, CRUD, import, state refresh, and the absence of
  plan-time remote mutation; the source tree additionally registers the
  v1alpha3-lane Edge Platform Family resources and the generic
  `takoform_resource` carrier for the unpublished v2.1 source candidate
  ([decision 0013](../spec/decisions/0013-v1alpha3-lane-ships-in-provider-v2-1.md));
- `internal/client/client_test.go` asserts discovery, capability negotiation, preview/apply evidence, error envelopes, observation, and deletion;
- `examples/resources/` contains one formatted HCL example for every registered typed resource; the generic `takoform_resource` carrier intentionally has none.

Run:

```console
go test ./...
go vet ./...
tofu fmt -check -recursive examples
```

## Current provider-v2 lifecycle evidence

`cmd/provider-lifecycle-conformance` builds the real provider-v2 candidate
binary and drives every declared v1alpha2 typed resource through a
Terraform-compatible CLI against
an in-process versioned Form host. The regression covers create, read plus observe,
mutable update with state-generation fencing, explicit refresh, native import,
CLI import, drift mapping from observe evidence, delete, exact
response-identity rejection, and
replacement plans for immutable names, SQL engine, and vector dimensions.
The data-only compatibility report binds the CLI product/version, exact canonical
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

`provider-reports` first verifies the exact generated v1alpha2 candidate
package set. It executes each canonical positive fixture and every declared
negative fixture through provider protocol v6. Desired negatives include a
one-at-a-time omission of `name`, artifact `source`, required `connections`,
and every Form-specific required field; each must return a provider diagnostic
before the in-process Form host receives a mutation. The observed negative
lets the host return `OtherKind/<name>` and requires the provider to reject the
response before it enters state. Both classes are reported as portable
`invalid_argument`. The external host report binds and executes the exact
desired-stage subset; the provider report covers that subset plus the
observed-stage response rejection. The retained historical report schema does
not claim output-stage negative coverage until a runner contract exists. These
reports prove conformance behavior only; they grant no lifecycle state or host
authority.

The command combines those per-package observations with the independently
executed full lifecycle checks and writes one strict RFC 8785
`takoform.standard-provider-runner-report@v2` document per kind. It refuses to
write under `admission/`, signs nothing, publishes nothing, and cannot change
Form lifecycle or host activation. The retained `external-required` field is
Legacy data, not a pending central gate. Its output directory must be new or
empty. Each report subject records the one canonical provider identity,
`provider:registry.terraform.io/tako0614/takoform`, regardless of whether
OpenTofu or Terraform executes it.

The matrix retains the machine classification `generic-lifecycle-candidate` with
`publicationReady: false` and
`bindingStatus: exact-structural-candidate-set` because changing an old report
format would falsify historical compatibility. Those labels are not current
Form lifecycle states. The matrix does not publish a checked-in passed report
or claim Form maturity. It requires
Terraform `1.15.8` and OpenTofu `1.12.3` under the same canonical identity,
`registry.terraform.io/tako0614/takoform`, then requires identical provider
schema, exact FormRef/package identity, and lifecycle evidence. Immutable
publication proves distribution only. Hosts separately publish support for an
exact FormRef and activate it under their own policy; neither action changes
the Form's project lifecycle state.

## Schema derivation gate

The provider-v2 schema, v1alpha2 Form Definition, and candidate conformance
fixtures are all derived from one current catalog declaration, so drift
between them is a build failure rather than something a checked-in fixture
corpus has to notice later.
`go run ./cmd/standard-form-conformance verify` regenerates nothing: it reads
the committed packages, re-verifies their bytes, and inspects the actual
provider resource schema for every declared Form, including that each field the
definition calls immutable really forces replacement.

## Retained v1alpha1 Legacy package corpus

[`form-package-v1/`](form-package-v1/) is a separate corpus for the portable
package layer. It includes one valid closed ExampleStore package. Each package
has one exact FormRef, one definition, one positive desired fixture, closed
desired/observed schemas, and no host authority fields. Tests pin every
package/schema digest and reject an unknown host extension and one
kind-specific invalid fixture.

`form-package-v1/positive/standard/` contains the historical generated package
for every one of the 34 v1alpha1 Legacy Forms. It is retained to authenticate
those exact bytes; it is not a source for v1alpha2 candidates and never starts
a new current version line.
`go run ./cmd/standard-form-conformance verify` validates package
bytes and fixtures and inspects the actual provider resource structure. It does
not run the Terraform protocol lifecycle or a conforming host, and this repository
intentionally contains no locally synthesized passed admission JSON.

The machine-readable inventory's `structural-candidate`, `structural-only`,
`external-required`, Definition `status: standard`, and
`portable-standard` values are immutable historical document facts. They are
not a current queue, rank, maturity state, or missing approval. Current
v1alpha2 maturity comes only from `forms/lifecycle.json`.

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
transformation rejection, and the independent limits of 32 positive plus 32
negative fixtures per Form Definition.

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

The Form Definition limits apply independently to each fixture class: at most
32 positive fixtures and at most 32 negative fixtures, not 32 combined. Every
retained Legacy Form and every current candidate stays within the negative
limit without combining or dropping required-field omission cases.

The retained Legacy corpus manifest contains 36 positive packages (one
ExampleStore, one interface-declaration package, and 34 historical generated
packages) and 51 negative cases. Passing this corpus proves the local Legacy
data contract only. It
is not signature,
publisher, remote-install, host-activation, retention/revocation, lifecycle
idempotency, or cross-host/kind-standardization evidence. Those require their
own role-specific execution and, for admission, authenticated external
evidence.

## Portable host evidence

`portable-host-v1/` is the frozen provider-v1 Legacy corpus. It pins the
v1alpha1 discovery/API paths and exact Legacy ObjectBucket identity so retained
publication evidence remains independently verifiable.

`portable-host-v2/` is the retained provider-v2 corpus. It pins the separate
v1alpha2 discovery/API paths, an exact EdgeWorker candidate and all of
its desired-negative fixtures, a Schedule-to-EdgeWorker connection
probe, concurrency/idempotency rules, stable errors, and required cross-repo
black-box checks. The retained-lane provider client consumes this contract in
adversarial HTTP tests.

`portable-host-v3/` is the Host API v1alpha3 corpus consumed by the provider
v2.1 client lane. It pins the v1alpha3 discovery/API paths, the closed
26-code error taxonomy with its exact HTTP status map and 4-code retryable
set, the uid/generation/revision identity rules, the closed portable
condition-reason vocabulary, and exact Edge Family probe identities
(ModuleWorker, EdgeKVNamespace, AtLeastOnceQueue, WorkerBundle,
WorkerVersion, WorkerDeployment, WorkerCronTrigger, QueueConsumer) with their
registry package digests plus byte-pinned
desired-negative fixtures. `self-test --contract conformance/portable-host-v3`
starts a deterministic reference host over the real candidate definitions and
drives the complete 49-check matrix over real HTTP: exact discovery and
availability,
validate/prepare with RFC 8785 prepare binding and substitution rejection
(a prepare against an existing resource requires the update generation
fence, and a fence-less or stale prepare is rejected),
uid minting and delete-then-recreate uid change, generation fences versus
revision fences (including a host-side status touch that advances only the
revision), packageDigest as audit-only evidence that never enters identity or
queries, one kind name in two namespaced groups, revision-role update
rejection, typed binding resolution before mutation with
`dependency_in_use` on bound-target deletion, import validated exactly like
apply, cross-resource semantics the Forms declare in prose but only a host can
enforce (WorkerDeployment weights summing to 10000; a cron trigger or queue
consumer refused with `unsupported_capability` until some WorkerVersion
declares the matching handler), 202 Operation polling with
Retry-After plus terminal replay and cancel — with every fence, binding
resolution, and blob requirement re-verified at commit time rather than at
accept time — the content-addressed artifact
upload/commit flow feeding a WorkerBundle apply including its manifest reject
list and commit-time size binding, the closed SpaceID grammar, support
profiles free of
price/SKU/region/quota, concurrent mutation of unrelated resources, and
idempotency isolation across principals, tenants,
and Spaces. Probing every stable error uses the runner-only
`Takoform-Conformance-Probe` header (`error:<code>`, `async`,
`touch-status`); it is disposable-adapter transport, never a production
surface. As with the other lanes, a passing local report is implementation
evidence for the runner and reference host only: it is explicitly
`publicationReady: false` and is never publication, admission, host support,
or Form maturity evidence.

The executable runner is
`go run ./cmd/portable-host-conformance`. Its `self-test` command starts a
disposable deterministic host and drives the actual HTTP discovery, Form
availability, preview, apply, read, observe, refresh, import, delete, and
read-only Interface endpoints. It rejects a host that accepts a preview plan
for different spec bytes, advances a generation on an exact idempotent replay,
ignores a stale fence, changes an exact response/ETag on replay, or exposes a
required Interface before Ready or after delete.

```console
go run ./cmd/portable-host-conformance self-test
```

The matrix also exercises all stable error envelopes over HTTP. Errors such as
permission or backend failure have no provider-neutral way to induce, so a
disposable external conformance endpoint uses the runner-only
`Takoform-Conformance-Probe-Error` header. The same disposable adapter accepts
the closed authorization probes
`Takoform-Conformance-Probe-Authorization: credential-revoked`,
`permission-revoked`, and `policy-revoked`, and must process each current
denial in the normal authentication/authorization path before any replay
lookup. After every denial, the runner repeats the unprobed request and
requires the original cached success byte-for-byte, proving that a denial did
not overwrite or poison the success record.

Plan binding has two explicitly separated evidence classes. Valid substitutions
of desired spec, Space, and an existing Resource generation are pure black-box
HTTP checks. The remaining one-field substitutions would ordinarily be
rejected by route, envelope, or exact-Form validation before reaching plan
binding, so the disposable adapter accepts the closed-enum
`Takoform-Conformance-Probe-Plan-Binding` header. It passes that request
directly to the same canonical plan-binding function used by normal apply and
returns
`Takoform-Conformance-Probe-Plan-Binding-Result: rejected` with
`invalid_argument` only when that function rejects it. If the function accepts
the substitution, the adapter returns HTTP 204 with
`accepted-no-mutation` and must not execute a fence, mutate state, or record a
replay. Unknown values fail as `invalid_argument`, and authentication and
authorization run before this instrumentation. The report records pure
black-box and instrumented-adapter inputs separately.

The disposable adapter also accepts
`Takoform-Conformance-Probe-Raw-JSON: duplicate-error-code` and returns the
contract's exact malformed error envelope. The runner must reject it during
complete raw-document I-JSON validation, before decoding stable error
semantics. Unknown raw-probe values fail as `invalid_argument`.

All four probe headers are disposable-adapter transport, are not part of the
host API, and must not be implemented or enabled on production.
The first two token variables must name distinct authenticated principals in
the same tenant. The third must represent the first principal in another
tenant. All three credentials must be distinct and authorized for the complete
test lifecycle in both exact runner Spaces. For the cross-tenant isolation
probe only, both tenant contexts must address the same disposable Resource
namespace; an independently executed create then deterministically collides
instead of returning the first tenant's cached success. Run the same matrix
against such an endpoint with:

```console
go run ./cmd/portable-host-conformance run \
  --endpoint https://disposable-host.example \
  --token-env TAKOFORM_CONFORMANCE_TOKEN \
  --alternate-token-env TAKOFORM_CONFORMANCE_ALTERNATE_TOKEN \
  --alternate-tenant-token-env TAKOFORM_CONFORMANCE_ALTERNATE_TENANT_TOKEN
```

Both reports are explicitly `publicationReady: false`. The local reference
report proves the executable runner, not an external host. Admission still
requires a signed report from the host workflow that actually executed its
backend lifecycle. Desired-stage negative fixtures are host request evidence;
observed-stage rejection is provider/host-response evidence because the
portable API intentionally has no operation for injecting observed state.

The required sequence includes create collision, mutable update, stale
update/delete/observe/refresh rejection, exact replay, retry-code semantics,
the fixed stable-code/HTTP mapping, the exact
`1..9223372036854775807` decimal generation range, and Interface readiness.
It also rejects plan substitution field by field for desired spec, Resource
identity/name, Space, generation, every exact FormRef field, and package
digest. Name instrumentation uses the matching alternate Resource URL, while
generation uses a real update after the Resource has advanced, so neither can
pass because of a create fence or URL/body mismatch. Replay
evidence proves that another principal cannot address the first principal's
cached success, that the same principal in another tenant cannot address it,
and that current authentication, permission, and policy denial each precede
replay lookup.
The same Resource name and exact Form are then created and read in
`runnerInput.alternateSpace` with the primary Space's Idempotency-Key; neither
Resource state nor replay records may cross the Space boundary.

Before lifecycle mutation, raw adversarial requests additionally prove that
unknown top-level envelope fields, unknown metadata fields, authority-shaped
desired fields, duplicate `metadata.space`, duplicate
`metadata.resourceVersion`, duplicate `spec`, and invalid UTF-8 fail closed.
The duplicate cases are rejected by shared complete-document I-JSON validation
before typed decoding. The report also records the three replay isolation
dimensions, the three current denial codes, and preservation of the original
successful replay after every denial.

`interfaceDeclarations.checks` is an exact executed subset of
`requiredRunnerChecks`, and the report repeats that exact list as Interface
evidence. Over real HTTP the runner rejects unknown, duplicate, and partially
paired Interface query parameters; requires an explicit Space without
substituting another one; exercises omitted-version success only for a unique
visible declaration; creates a second Ready Resource in the same Space to
require `interface_instance_ambiguous`; and re-reads an unchanged projection
after POST, PUT, PATCH, and DELETE are rejected on both portable Interface
routes. The visible declaration must equal the exact Form descriptor, satisfy
its `documentSchema`, contain no portable authority fields, appear only with a
Ready Resource, and disappear after deletion. When present, `resourceUri` must
pass the shared credential-free HTTPS grammar; the adversarial runner rejects
userinfo, query, fragment, plaintext HTTP, and Unicode-hostname substitutions.

The selected ObjectBucket fixture has one required descriptor at one version,
so this runner does not claim that it induced multi-version identity ambiguity,
optional-descriptor omission, or activation rejection on a feature-disabled
host. Their wire rules remain normative, and the stable ambiguity error
envelopes are exercised, but causal host evidence for those cases needs a
separate exact package fixture.

It treats drift only as validated observed evidence and defines no drift
operation or portable audit endpoint.

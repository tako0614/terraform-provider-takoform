# Conformance

The retained v1alpha2 compatibility evidence is executable Go
characterization for the published Provider 2.0.0 predecessor, while the
current Registry-published Provider 2.1.1 carries the Beta family:

- `internal/provider/*_test.go` asserts that the published compatibility
  provider v2.0.0
  exposes exactly the
  nine independently authored v1alpha2 candidates, and covers typed schema
  behavior, validation, CRUD, import, state refresh, and the absence of
  plan-time remote mutation; the Registry-published v2.1.1 provider source
  additionally registers exactly the v1beta1 Edge Platform Family resources —
  and no generic carrier, which is itself asserted
  ([decision 0035](../spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md),
  [decision 0021](../spec/decisions/0021-third-party-forms-and-contract-distribution.md));
- `internal/client/client_test.go` asserts discovery, capability negotiation, preview/apply evidence, error envelopes, observation, and deletion;
- `examples/resources/` contains one formatted HCL example for every registered resource, with no exceptions: every provider resource is derived from a Form, so every one has an example.

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

`form-package-v1/` is a separate corpus for the portable
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

`portable-host-v3/` is the retained `forms.takoform.com/v1alpha3` corpus: 114
retained v1alpha3 checks, published at the address that has always named them. It is
retained bytes rather than a runnable corpus — the v1alpha3 runner became the
v1beta1 runner instead of being copied — so `bun run check` guarantees that it
does not change rather than that it executes. Its own
`portable-host-v3/RETAINED.md` states why it keeps a generation-numbered name
while the current corpus does not.

Every count this page states about a corpus — the size of a check matrix, of an
error taxonomy, of a retryable set — is bound by `bun run check:public-surfaces`
to the array in that corpus's `contract.json` that defines it. A number written
here beside a number a machine already knows is a defect waiting to recur, so
the gate names both values and fails when they part company.

`portable-host-v1beta1/` is the Host API v1beta1 corpus consumed by the provider
v2.1 client lane. It pins the v1beta1 discovery/API paths, the closed
26-code error taxonomy with its exact HTTP status map and 4-code retryable
set, the uid/generation/revision identity rules, the closed portable
condition-reason vocabulary, and exact Edge Family probe identities
(ModuleWorker, EdgeKVNamespace, AtLeastOnceQueue, WorkerVersion, WorkerBundle,
StaticAssetBundle, SQLiteDatabase, SQLiteMigrationSet,
SQLiteMigrationApplication, WorkerDeployment, WorkerCustomDomain,
WorkerEndpoint, WorkerCronTrigger, QueueConsumer) with their registry package
digests plus byte-pinned desired-negative fixtures. It pins each of those
14 Forms' DESIRED SCHEMA as
byte-digested bytes as well, because the runner materializes every probe spec
against the pin rather than against what the host under test serves: a runner
that took its defaults from the subject agreed with that subject about every
byte it then sent, so a host publishing a default of its own passed a whole run
while a real client — materializing the normative default — computed a different
`specDigest` and had every `prepare` refused. `form-definition-exact` compares
what a host serves against those bytes, and a repository test recomputes them
from the installed Definition at the exact pinned FormRef so the pin cannot
drift
([decision 0022](../spec/decisions/0022-relations-pin-the-target-contract.md)).
The family has one additional current Form: `ObjectBucket` is the one 15th Form
intentionally unprobed by this corpus. Its wire rules remain normative, but
causal host evidence for that Form requires a separate exact package fixture;
this corpus makes no coverage claim for it.
It also pins a byte-digested SECOND Form Definition
of the ModuleWorker line, so the lane can install two contracts of one group and
kind at once; without it, "a host answers for the exact ref recorded in state"
and "a host answers for the kind" are the same behavior and no check can tell
them apart (same decision).
`self-test --contract conformance/portable-host-v1beta1`
starts a deterministic reference host over the real candidate definitions and
drives the complete 116-check matrix over real HTTP: exact discovery and
availability, a credential that is REQUIRED — an absent `Authorization` header
and a bearer credential naming nobody are both refused `unauthenticated` on a
read surface and on a mutating one, and the identical requests under a real
credential succeed, so a host whose credential lookup fails open cannot pick a
tenant the caller never named,
validate/prepare with RFC 8785 prepare binding and substitution rejection
(a prepare against an existing resource requires the update generation
fence, and a fence-less or stale prepare is rejected),
uid minting and delete-then-recreate uid change, a replay record that does not
outlive the incarnation it reports — the byte-identical create that replays while
its resource is present is a NEW create with a new uid once that resource is
deleted, and reads back, so `terraform destroy` followed by `terraform apply` of
an unchanged configuration converges; while the delete's own record, which
reports no live incarnation, still replays its 204 and does not remove the
replacement that holds the name, and an accepted 202 follows the incarnation its
operation committed, generation fences versus
revision fences (including a host-side status touch that advances only the
revision), a delete fenced on the expected generation — refused unfenced,
refused stale, and accepted carrying nothing else, while the representation
fence stays honored when a client sends one and is never required, so tearing
an aggregate down succeeds under the generation a refresh read before the
teardown moved the revision itself, packageDigest as audit-only evidence that never enters identity or
queries, one kind name in two namespaced groups, revision-role update
rejection, cross-resource relation resolution before mutation — every
reference a Form derives from its desired schema, not only typed bindings —
pinned by target uid, with `dependency_in_use` on the deletion of any
referenced target, `ExternalChange` when a target is destroyed and recreated
under the same name — which a spec-identical re-apply repairs, re-pinning
every relation while generation stands still and only the revision moves —
and binding contracts verified against
allowedTargetForms, the target Form's providedInterfaces, and the source
role, the exact identity a host answers for — two definition versions of one
group and kind installed at once and answering independently on availability,
the Form Definition surface, and the support profile, a resource answered only
under the exact ref it was created under and `resource_not_found` under any
other, and a relation refused before mutation when its target does not satisfy
the contract the reference annotates, whether that is an exact Form identity or
a required Interface, with the stored pin recording the target's exact FormRef
beside its uid — import validated exactly like apply and DISTINGUISHABLE from a
create: the `nativeId` an adoption names is a claim the host records on the
resource and holds exclusively within the caller's tenant, so a second live
resource adopting one native identity is refused `import_conflict` before any
mutation — from any space of that tenant, and under any FORM KIND, because a
backend object has one identity whatever Form was pointed at it and a host
indexing claims by `(tenant, kind, nativeId)` lets a queue and a KV namespace
manage one object between them; never across tenants, where a refusal would
report what a stranger manages, while inside the second tenant's own plane the
same claim binds in full. A resource already adopted cannot be re-pointed at
another native identity, and the claim is released when its holder is deleted
and by nothing else — an ordinary update withdraws none of it. Both paths onto a
first claim are driven, because the claim is recorded by the first import
whichever way the resource arrived: the ordinary `terraform import` onto a
resource this host CREATED, and the adoption that brought one into being.
Everything else the lane asks of an import — a minted uid,
generation 1, the whole validation gauntlet — a plain create also satisfies, and
`terraform import` against such a host mints a new backend object and orphans
the one being adopted
([decision 0011](../spec/decisions/0011-resource-identity-generation-and-revision.md);
these are also the corpus's only organic producers of `import_conflict`, which
was otherwise reachable only through the error probe), cross-resource semantics the Forms declare in prose but only a host can
enforce (WorkerDeployment weights summing to exactly 10000 — short and long are
both refused, and a real two-version split at 1000/9000 is accepted, so a host
that admits only one weighted entry has no traffic split and fails; one active
deployment per worker, with a replacement accepted once the live one is released,
so the rule is about what is live rather than a flag nothing clears; a cron trigger or queue
consumer refused with `unsupported_capability` until some WorkerVersion
declares the matching handler; a WorkerEndpoint answered with a complete HTTPS
address the host assigned rather than the author, in canonical form — lowercase,
no trailing root dot, and a url built from exactly that hostname — the same
address still published, under the same uid, after a host-side status refresh,
a promotion of the worker it serves, and a re-read, and a second endpoint
against one worker refused; and `status.outputs` present with exactly the
declared members for the one Form that declares an outputSchema and omitted for
every Form that declares none), 202 Operation polling with
Retry-After plus terminal replay and cancel — with every fence, binding
resolution, and blob requirement re-verified at commit time rather than at
accept time, and the commit bound to the exact incarnation the mutation was
accepted for, so a target removed out of band and re-created under the same
name — under the same contract or under the other definition version, at the
same revision — terminates the operation `uid_mismatch` and survives it, while
one that simply vanished terminates it `resource_not_found` — the
content-addressed artifact
upload/commit flow feeding a WorkerBundle apply including its manifest reject
list and commit-time size binding, the closed SpaceID grammar, support
profiles free of
price/SKU/region/quota, concurrent mutation of unrelated resources, and
idempotency isolation across principals, tenants,
and Spaces. It also drives the operability rules of
[decision 0018](../spec/decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md):
a namespaced group served as two ordinary path segments on the resource,
form-definition, and support routes, with no request target percent-encoding a
slash; an operation id and an upload id answered `operation_not_found` and
`artifact_missing` for every other principal and tenant while the owner's
record survives untouched; and a content address that is a name rather than an
entitlement — another tenant reads neither the manifest nor the blob, a second
principal of the HOLDING tenant reads both, and the other tenant that uploads
the same bytes is told they are missing for IT before committing the identical,
physically deduplicated artifact. The same holding rule governs USING an address
rather than only reading one: a bundle whose desired state references a manifest
the caller's tenant does not hold is refused `artifact_missing` before any
mutation, on apply and on import, storing nothing, while a second principal of
the holding tenant references it successfully and the other tenant references it
once it has supplied the bytes itself. The plane those three surround is held to
the same boundary by
[decision 0028](../spec/decisions/0028-the-resource-plane-is-tenant-isolated.md):
two tenants create one `{space, kind, name}` and get two resources with two uids,
neither reads, observes, updates, or deletes the other's — answered
`resource_not_found`, indistinguishably from a name nobody created, message
included — a reference resolves only inside the referring tenant even when the
name matches exactly, a `prepareDigest` minted by one tenant is not spendable by
another — while a second PRINCIPAL of the minting tenant spends the same review
successfully, because the boundary is the tenant — and one `Idempotency-Key` from
two tenants is two operations rather than a replay, with the second tenant's
resource read back so an answered key cannot stand in for an executed one. Every
one of those refusals is paired with the permissive half it would otherwise be
satisfied by refusing: the second tenant UPDATES and DELETES resources of its own
while the first tenant's identically-named resource does not move; the holder of
a resource a stranger reached for can still update and delete it, so a host that
quarantines the record fails; and the relation check reaches a successful apply
and reads the stored pin by what it protects — the first tenant's identically
named target deletes freely, the second tenant's source stays Ready when it does,
and the second tenant's own target is refused `dependency_in_use`. The one claim in the lane that stops at the tenant WITHOUT the tenant
being in its address is the `WorkerCustomDomain` hostname, and it is measured in
the direction that costs a mistake: a second tenant standing up its own
aggregate and claiming the hostname the first tenant serves is ACCEPTED, both
claims stay live under two uids, releasing one leaves the other where it was,
and a third claim inside the second tenant is still refused — naming that
tenant's own holder and no one else's. `import` is the one surface whose absent answer is a SUCCESS, and it is
measured as one: a second tenant adopting the name the first tenant holds is
answered the 201 it would be answered for a name nobody holds anywhere — same
status, same ETag, same document but for the minted uid and the name — while the
holder's uid, generation, and revision do not move, and the fenced form of the
same adoption is refused `resource_not_found`. Nine of the ten are enumerated by
SURFACE rather than by intent: every route that takes a resource name is listed
against the check measuring it, and that list is bound to the published route
block and to the required-check list, so a name-addressed endpoint cannot be
added without one; the tenth is the permissive half of all of them. All ten are
black box and all ten need the alternate-TENANT
credential, which is why the runner requires it.
The two attachment claims of
[decision 0026](../spec/decisions/0026-attachment-claims-are-canonical-and-acyclic.md)
are driven on all three surfaces that record names, not on `apply` alone: a
colliding hostname and a cycle-closing consumer are both refused through
`import` and accepted through it once the state that made them wrong is gone,
and an accepted `202` that was legal when it was accepted terminates
`invalid_argument` at commit when a synchronous request took the hostname, or
closed the loop, while it was pending — committing nothing and leaving the
synchronous resource alive. The A-label rule is driven as the refusal it is: a
U-label hostname is rejected at `validate` and at `prepare` while the name is
free, so nothing but the Form's grammar can be refusing it, and the A-label of
the same name is accepted and stored byte-for-byte.
Probing every stable error uses the runner-only
`Takoform-Conformance-Probe` header (`error:<code>`, `async`,
`touch-status`, `external-change`); it is disposable-adapter transport, never
a production surface. `external-change` performs one delete as an out-of-band
backend change so a runner can reach the one state relation protection
otherwise makes unreachable. As with the other lanes, a passing local report is implementation
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

The unprobed `ObjectBucket` Form has one required descriptor at one version in
its retained fixture, so this runner does not claim that it induced
multi-version identity ambiguity, optional-descriptor omission, or activation
rejection on a feature-disabled host. Its wire rules remain normative, and the
stable ambiguity error envelopes are exercised, but causal host evidence for
those cases needs a separate exact package fixture.

It treats drift only as validated observed evidence and defines no drift
operation or portable audit endpoint.

## Runtime ABI evidence

`runtime-abi-v1/` measures a different subject from every corpus above. Those
drive a control plane; this one drives a **runtime**. The Host API v1beta1
lane can prove that a host advertises `worker.runtime@1.0.0` at the exact
pinned digest and refuses a `WorkerVersion` that names a handler the ABI does
not define, and
[`../spec/host-api/v1beta1.md`](../spec/host-api/v1beta1.md#what-the-lane-proves-and-what-stays-a-host-obligation)
lists what it cannot prove, because proving it means executing the module
rather than driving the API. This corpus executes it
([decision 0023](../spec/decisions/0023-the-runtime-abi-is-measured-separately-from-the-control-plane.md)).

The corpus is digest-pinned the same way `portable-host-v1beta1/` is: `manifest.json`
carries the sha256 of `contract.json`, and `contract.json` additionally pins the
sha256 of every module byte it ships. It states, as data, the exact
`worker.runtime` InterfaceRef it measures, the closed handler vocabulary, the
closed loadable media-type set, the portable globals floor, the deployment an
operator reproduces — bundle, declared handlers, vars, sensitive variable,
`edge.kv`, `edge.queue` and `worker.service` bindings, cron expression, queue,
and the SECOND worker the service binding addresses — and for each of the 18
required checks its name, what it proves, the bundle it needs, and the
request/expected-observation pairs that decide it. It pins a second
InterfaceRef beside the runtime ABI's, `worker.service@1.0.0`, because two of
those checks are claims about that contract's delivery model and a corpus
naming a contract it does not pin can go on measuring one that no longer
exists.

Five checks drive the module loader: a bundle whose bytes load and report the
handler set THOSE BYTES export, a module media type outside the closed set, a
`mainModule` the bundle does not carry, bytes that are not a compilable module,
and a declared handler the module does not export — the last being exactly the
obligation the Host API lane states it cannot prove for arbitrary bytes.
Thirteen drive a deployed worker over HTTP: the three-argument handler signature, a
returned `Response`, an uncaught throw becoming a completed host-generated 500
rather than a hung request, `env` projecting exactly the declared names, the
globals floor, a byte round trip through the `edge.kv` binding, a request body
answered chunk by chunk before the next chunk is sent, a response body whose
chunks arrive separated in time, `ctx.waitUntil` holding the isolate open while
a rejected task leaves the sent response alone, a `scheduled` invocation from
the cron attachment, and a queue batch delivered to the `queue` handler with the
producer's exact bytes. Two more take the streaming pair one worker further
along: the same request-body and response-body observations, made THROUGH the
`module-worker.service` binding into a SECOND worker, which is what holds the
projection to the streaming model `worker.service@1.0.0` states.

That second worker has to be distinguishable from the first, and stating that
it exists is not enough. While the peer ran the caller's own byte-pinned
bundle, a host that answered `env.PEER.fetch(...)` out of the caller's fetch
handler produced the same accounting, the same chunk timing and the same status
as the dispatch it never performed, so both checks passed a runtime with no
cross-worker projection at all — a required check no incorrect runtime can
fail. The peer therefore runs its own bundle, `conformance-peer`, whose module
declares an identity the caller's bytes do not contain and stamps it on every
observation it emits; the runner credits a service check only for stamped
answers. The loader enforces both halves of that: the identity must be
derivable from the peer module an operator deploys, and it must appear in no
other bundle of the corpus, so an answer the caller could have produced is
refused. Which checks require it is pinned in the runner beside the required
check list, so dropping the marker fails verification rather than quietly
restoring the defect.

The response-stream procedure also holds the response HEAD to the body that
follows it. `worker.service` spells an unknown length as a null `contentLength`
precisely because the call completes at the head, where a body still being
generated has no byte count; the two ways a host can refuse to say so are both
refused here. A host that BUFFERS the body to learn a count delivers chunks the
producer separated in time all at once and fails the separation. A host that
INVENTS one is held to it: the runner reads the body to its end and requires a
declared length to be the length delivered.

Every handler the ABI declares is measured, and the loader ENFORCES that rather
than the corpus asserting it: a handler in `handlerVocabulary` with no check
whose `operation` names it — or with only an `unmeasured` entry, which is what
`tail` used to have — fails corpus verification by name. That is why `tail` left
the ABI rather than staying as an entry nothing could reach
([decision 0019](../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).

The bundles carry module bytes and the handlers those bytes genuinely export,
and loading the corpus recomputes every stated outcome from the bytes. A bundle
that claims an export its module does not have is refused, and so is a check
expecting a failure the bytes cannot cause: a required check no correct runtime
can pass, and one no incorrect runtime can fail, are the same defect.

Three checks read back an observation the runtime stored for them —
the `edge.kv` round trip, the queue delivery and the `ctx.waitUntil` marker —
and the deployment they are measured against outlives the run, `edge.kv`
namespace and all. A pinned constant cannot correlate a run and a per-run value
cannot be pinned, so what the corpus pins is the correlation TEMPLATE:
`runCorrelation` states the placeholder and the token width, each of the three
checks states a template such as `kv-round-trip-{run}`, and the runner mints one
unpredictable token per run, substitutes it into the value it sends, and derives
the observation it expects by the same substitution. The corpus bytes never
move, the token is never in them, and the report states the token a run used so
a failure stays diagnosable afterwards. Pinning those values as constants
instead produced both halves of the same defect at once: on a second run a
runtime whose `put` stored nothing and whose queue delivered nothing passed on
the first run's leftovers, and a conforming runtime failed because the
`waitUntil` marker was already there before its deferred task had run.

`self-test` runs the whole matrix against an in-process stand-in shipped with
this repository, so the corpus is exercised on every `bun run check`. The
stand-in has no JavaScript engine and reimplements each corpus module's
behaviour in Go; what keeps it honest is that it is constructed from the
deployment description and the module bytes and nothing else, so it never sees
a check or an expected observation. The peer's identity is part of that: the
stand-in reads it out of the main module it was handed, the way it reads the
exported handler set, so a stand-in wired to answer service calls from the
caller stamps nothing and fails the two service checks exactly as a real host
doing the same thing would. A self-test report says this in the document itself:
its `classification` is `in-process-fake-runtime-self-test` and its `proves`
member states that it proves the corpus and nothing about any runtime.

```console
go run ./cmd/runtime-conformance verify
go run ./cmd/runtime-conformance self-test
```

Measuring a real runtime means deploying the corpus bundle exactly as the
contract's `deployment` states and pointing the runner at it. `loadModule`
decides its outcome before any traffic arrives, so the load lane is measured
through a disposable adapter over the runtime's own module loader
(`takoform.runtime-abi-loader@v1`); the adapter is not part of the ABI and must
never be exposed by a production deployment. Without one the load lane is
reported `unmeasured` and the run `partial`, rather than counting an
unmeasurable half as evidence.

```console
go run ./cmd/runtime-conformance run \
  --endpoint https://conformance-worker.example \
  --loader-endpoint https://disposable-loader.example/load \
  --token-env TAKOFORM_RUNTIME_CONFORMANCE_TOKEN \
  --loader-token-env TAKOFORM_RUNTIME_LOADER_TOKEN
```

Both reports are explicitly `publicationReady: false`, as in every other lane.
Publication blocker V3-008 closes on a passed
`deployed-runtime-conformance-run` with its load lane measured, against a
runtime this repository does not own; a self-test never closes it, and each
report repeats that sentence in its own `blockerEvidence` member.

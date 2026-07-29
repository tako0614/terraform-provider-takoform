# Portable Form host API v1alpha1

The provider uses a versioned, provider-neutral HTTP boundary. A host owns
placement and execution; this protocol owns exact Form identity, portable
desired state, optimistic concurrency, mutation replay, and stable errors.

Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

[`operations.json`](operations.json) is the machine-readable form of this
contract: every operation with its method, path, exact-identity query, required
precondition, idempotency, and the stable error taxonomy. The reference client
is driven against it by `go test ./internal/client`, so this document describes
behaviour that actually happens rather than behaviour that was intended.

## Discovery and endpoint selection

A conforming host MUST answer `GET /.well-known/takoform` with:

- `api_versions` containing `forms.takoform.com/v1alpha1`;
- `features.service_forms = true`;
- `features.exact_form_ref`, `features.optimistic_concurrency`, and
  `features.idempotent_lifecycle` all set to true;
- an absolute same-origin `endpoints.api` URL;
- an absolute same-origin `endpoints.forms` URL, or `{endpoints.api}/forms`.

A provider MUST send bearer credentials only to same-origin advertised URLs
and MUST use `endpoints.api` exactly as advertised. A provider MUST reject a
discovery document that omits the versioned endpoint: there is no unversioned
lane to downgrade into.

[`../schemas/host-discovery.schema.json`](../schemas/host-discovery.schema.json)
closes the discovery object and enforces the absolute HTTP(S) shape of `api`,
`forms`, and `interfaces`. JSON Schema cannot compare URL origins. After schema
validation, an implementation MUST parse and normalize every advertised URL
and MUST reject `forms` or `interfaces` unless its scheme, host, and effective
port equal those of `api`. Userinfo, query, and fragment components are
forbidden. Plain HTTP is allowed only for a loopback development origin.

## Exact identity

Every typed provider resource MUST be compiled against one release-owned
`InstalledFormReference`: `apiVersion`, `kind`, `definitionVersion`,
`schemaDigest`, and `packageDigest`. `GET /forms` MUST return that exact
identity as installed, executable, activated, available to the principal, and
supporting the requested operation. Resource bodies carry the same `form` and
read/lifecycle URLs carry all five fields as query parameters. A host MUST NOT substitute any Form identity field or
the requested Resource `metadata.name` / `metadata.space`, and a provider MUST
fail closed when it observes such a substitution.

Runtime Resource identity is scoped by `(Space, kind, metadata.name)`. The same
kind, name, and exact Form in another Space is an independent Resource: reads
MUST NOT return the first Space's state, and lifecycle mutations or deletions
MUST NOT change it.

The provider release's exact standard references are pinned by
[`forms/standard-package-set.json`](../../forms/standard-package-set.json).

## Space identity

`SpaceID` is the portable scope identity used by Resource bodies, exact Form
and Resource query parameters, Interface reads, idempotency scope, provider
configuration, and provider state. It is an opaque, case-sensitive string. It
is not a Resource `PatternName` and MUST NOT be validated with the lowercase
URL-safe `metadata.name` grammar.

A valid `SpaceID`:

- is valid UTF-8 containing 1 through 255 Unicode code points;
- does not start or end with a Unicode `White_Space` code point or `U+FEFF`;
- contains no C0 or C1 control code point; and
- contains no `/`.

The normative code-point sets are encoded explicitly in
[`../schemas/host-api-wire.schema.json`](../schemas/host-api-wire.schema.json)
under `$defs.spaceId`. Embedded non-control whitespace is data and is valid.
Every participant MUST preserve the exact decoded value: no trimming, Unicode
normalization, or case folding is allowed. URL percent-encoding MAY represent
that value on the wire, but decoding MUST recover the same code-point
sequence.

The provider applies the same rule to its configured default, each Resource
override, Interface data-source selector, Terraform/OpenTofu state, and import
identifier. Import uses either `NAME` with a configured default or
`SPACE/NAME`; forbidding `/` in `SpaceID` makes that split unambiguous.

## Versioned wire envelopes

[`../schemas/host-api-wire.schema.json`](../schemas/host-api-wire.schema.json)
is the normative machine-readable wire schema. Every operation in
[`operations.json`](operations.json) points to the exact request and response
schema fragment it uses.

Every Host API request and response document MUST be UTF-8 I-JSON. Before
typed decoding, an implementation MUST validate the complete raw document,
including nested objects, and reject invalid UTF-8, duplicate object member
names, invalid Unicode, non-I-JSON numbers, excessive nesting, and trailing
data. A request with a duplicate member or an unknown envelope/metadata field
fails as `invalid_argument` / HTTP 400 before mutation. An authority-shaped
field hidden in portable `spec` is also rejected by the exact Form desired
schema before mutation. A response with duplicate members is protocol-invalid
before any typed or stable-error semantics are applied. The same raw-first
rule applies when decoding this repository's digest-pinned conformance
contract; a duplicate member is verification failure, never last-member-wins.

A Resource request contains exactly `apiVersion`, `kind`, the exact five-field
`form` (`FormRef` plus `packageDigest`), `metadata.name`, `metadata.space`, an
existing Resource's `metadata.resourceVersion`, and `spec`. `kind` MUST equal
`form.formRef.kind`, and `spec` MUST validate against that exact installed
Form's `desiredSchema`.

A Resource response repeats those fields without substitution and requires the
current `metadata.resourceVersion` plus `status.observed`. The observed
document MUST validate against that exact Form's `observedSchema`. When the
Form declares `outputSchema`, the response also requires `status.output`, and
the output document MUST validate against it; otherwise `status.output` MUST
be omitted. These Form-relative checks are semantic validation after the fixed
envelope schema. A host MUST NOT choose a schema by kind or version alone.

Preview returns only the requested Resource and
`review: {planDigest, specDigest}`. Both use canonical lowercase `sha256:`
form. `specDigest` covers the RFC 8785 canonical `spec`; `planDigest` binds
that exact digest, the Resource `apiVersion`, `kind`, `metadata.name`,
`metadata.space`, and `metadata.resourceVersion`, every field of the exact
`form.formRef`, `form.packageDigest`, and the host's exact reviewed plan. Apply
presents only the exact Resource plus `review.planDigest`. Before mutation, a
host MUST resolve that review and compare every bound portable input. Reusing a
plan digest after substituting any Resource identity field, name, Space,
FormRef field, package digest, generation, or desired spec MUST fail as
`invalid_argument` / HTTP 400 without mutation. Backend selection, target
identity, credentials, quotes, and native plan details are not portable
preview or apply fields.

## Resource lifecycle

The API base is `/apis/forms.takoform.com/v1alpha1` on the reference host:

- `GET /forms` discovers exact availability;
- `POST /resources/preview` returns `review.planDigest`;
- `PUT /resources/{kind}/{name}` applies that reviewed plan;
- `GET /resources/{kind}/{name}` reads canonical portable state with exact
  observed and output documents;
- `POST /resources/{kind}/{name}/import` imports a native object;
- `POST /resources/{kind}/{name}/observe` returns updated read-only observed
  evidence;
- `POST /resources/{kind}/{name}/refresh` publishes host-owned backend state
  and sanitized outputs without changing native provider resources;
- `DELETE /resources/{kind}/{name}` deletes it and returns an empty HTTP 204.

Create and new-resource import MUST use `If-None-Match: *`. Update, existing-resource
import, observe, refresh, and delete MUST use one quoted `If-Match` resource
version.
Every apply, import, observe, refresh, and delete request MUST carry a
deterministic `Idempotency-Key`, and a retry MUST reuse the same key.
The key is not a bearer capability and is never a global cache address. A host
MUST namespace a replay record by its authenticated tenant/security domain,
authenticated principal, exact Space, and `Idempotency-Key`. It MUST
authenticate and authorize the current request before consulting or returning
that record. A same-key request from another principal or tenant is an
independent request, and a principal whose credential or permission has since
been revoked receives the current `unauthenticated`, `permission_denied`, or
`policy_denied` result rather than a cached success. The replay fingerprint
also binds the method, exact request target, generation preconditions, and
exact request body bytes; reusing a key for different request bytes fails as
`invalid_argument`.
A client MUST automatically retry only an error with `retryable: true` and code
`resource_busy` or `backend_unavailable`, and MUST NOT retry a resource-version
conflict. A transport failure without a complete stable error envelope is
ambiguous and MUST NOT be automatically retried for a mutation.

`resourceVersion` is not an opaque host token. It is the canonical
positive-decimal desired-state generation in the inclusive range
`1..9223372036854775807`. Leading zeroes, zero, larger values, and
non-decimal tokens are invalid. A successful response MUST return the same value in
`metadata.resourceVersion` and in `ETag` surrounded by exactly one pair of
double quotes. A create with anything other than `If-None-Match: *`, a create
when the exact Resource already exists, a missing or stale `If-Match`, and a
body/ETag generation mismatch all fail as
`resource_version_conflict` / HTTP 412. No mutation occurs on that failure.
Observe and refresh do not advance desired state: their HTTP 200 response MUST
repeat the exact generation sent in `If-Match`. If that generation is no
longer current, the host MUST return `resource_version_conflict` / HTTP 412
instead of returning a Resource at another generation.

An OpenTofu/Terraform provider Read is not the host refresh operation. Every
provider Read first performs the exact GET to obtain the current
`resourceVersion`, then sends a generation-fenced observe. Every successful
Resource response consumed as provider state MUST carry
`status.observed` and, when the exact Form declares `outputSchema`,
`status.output`. Those documents MUST satisfy the exact installed Form's
closed `observedSchema` and `outputSchema`; each present `id` MUST equal
`Kind/name`, each present generation MUST equal `resourceVersion`, output
`kind` / `name` MUST equal the requested Resource, and any shared portability
values MUST agree. The provider synthesizes its `id` locally, derives
`drift_status` only from the validated `observed.driftedFields`, and rejects
the whole projection before state when any arbitrary or mismatched key is
present. A top-level host ID or `metadata.id` is not part of this contract.

The provider does not call the state/output publication endpoint on every
Read; refresh is an explicit host lifecycle action and may do materially more
work than a read-only observation. A genuine Resource `404` removes matching
current-identity state; it is not encoded as an unvalidated status document.

`drift` is a Form lifecycle capability and an outcome derived from observed
evidence. It is not a separate host API operation: there is no `/drift`
endpoint, mutation, or additional lifecycle authority.

Form `connections` are request-only portable metadata. A host MAY deny a
connection it cannot or will not honor. A connection grants no access and
contains no credential, binding, or token. Binding and grant creation, token
issuance, projection materialization, authorization, write fencing, and
lifecycle remain host-owned.

For every Connection, the host resolves `connection.resource` as exactly
`Kind/name` within the source Resource's exact `metadata.space`. The source
Resource carries the only portable Space selector. A host MUST NOT search,
fall back to, read, or mutate a same-named Resource in another Space while
resolving the Connection, even when that other Resource is visible to the same
principal. If the target is absent or denied in the source Space, the
Connection is unresolved. An absent target, including a same-named target that
exists only in another Space, fails apply as `resource_not_found` / HTTP 404
before any source Resource mutation. Apply MUST re-resolve against current
same-Space state; a previously successful preview does not authorize fallback
or waive this check. A required denied Connection cannot be Ready. Cross-Space
composition, if offered, is a host-owned contract outside portable Resource
desired state and does not change this resolution rule.

## Interface declarations

A host MAY expose the OPTIONAL surface defined by
[`spec/interface-declaration`](../interface-declaration/README.md):

- `GET {api}/interfaces?space={space}` lists visible declarations;
- `GET {api}/interfaces/{name}?space={space}&version={version}&resourceKind={kind}&resourceName={name}`
  reads the exact runtime declaration.

For these entries, `requiredQueryParameters` and `optionalQueryParameters` in
`operations.json` are a closed query vocabulary. Parameters named together in
`pairedQueryParameters` must either both be present or both be absent.

A host that exposes it MUST advertise `features.interface_declarations = true`
and MAY advertise an absolute `endpoints.interfaces` URL that is same-origin
with `endpoints.api`; otherwise the surface is rooted at `{api}/interfaces`.
The feature is not part of required host negotiation. This surface is a
read-only projection of declarations materialized from Form descriptors during
the host-owned Resource lifecycle. It defines no standalone Interface
create/update/delete operation.

`space` is REQUIRED on every Interface request and uses the
[Space identity](#space-identity) contract. The provider selects it from the
explicit data-source selector, otherwise from its configured default, and
fails before HTTP if it is missing or invalid. The host scopes visibility and
every uniqueness decision strictly to that requested Space and MUST NOT
substitute another Space.

If `version` is omitted, the read succeeds only when exactly one visible
declaration has that name. No match is `resource_not_found`; multiple versions
fail closed as `interface_identity_ambiguous` (HTTP 409). Resource selectors
must be supplied together. If they are omitted, the read succeeds only when one
visible Resource instance matches; multiple Resources fail closed as
`interface_instance_ambiguous` (HTTP 409).

The response carries the exact identity, the exact non-secret descriptor
`document`, resolved public `values`, a required portable
`resource: {kind,name}` reference, and optionally the exact
`InstalledFormReference` that declared it. When `resourceUriInput` is present
in that Form descriptor, the response MAY include the resolved
credential-free HTTPS `resourceUri`. It uses the same
`credentialFreeHTTPSURL` grammar as immutable artifact URLs: the scheme is the
literal lowercase `https://`, the host is a dotted ASCII hostname, an optional
port is one to five decimal digits, and an optional path contains no
whitespace, query marker, or fragment marker. Userinfo, query, fragment,
single-label and Unicode hostnames are forbidden. A Unicode path may be
represented directly or percent-encoded; an internationalized hostname must
use its dotted ASCII IDNA representation. The whole value must also be valid
URI syntax, so malformed percent escapes and control characters are forbidden.

Every successful Interface list or exact read MUST carry a non-empty,
non-whitespace JSON body matching `interfacesResponse` or
`interfaceDeclaration` in the normative Host API wire schema. An empty or
whitespace-only HTTP 200 body is a protocol error. A list body MUST contain
the `interfaces` member as an array; `{}`, `null`, and
`{"interfaces": null}` are not empty-list encodings, while
`{"interfaces": []}` is.

The host MUST NOT invent or alter the document. Both document and values must
satisfy the same portable data-only forbidden-field policy as a Form
Definition; a provider rejects the response before non-sensitive state if they
contain secret/credential/host-authority, commercial, or executable field
vocabulary. The read says what exists and grants nothing; a host MAY filter
reads to already-visible records, but Interface records, bindings,
authorization, write fencing, and lifecycle remain entirely host-owned and are
not projected.

A host advertising the feature must enforce `required` readiness semantics. A
host without the feature remains conforming, but must reject activation of a
Form whose required declaration it cannot honor rather than reporting the
Resource Ready. Optional skipped descriptors must not be listed.

The host materializes this projection only from descriptors declared by the
Resource's exact installed Form. A portable caller cannot add, replace, or
remove a declaration independently of that Resource lifecycle.

Descriptor identity remains `(name, version)`. Runtime declaration identity is
`(space, resource.kind, resource.name, name, version)`. No host Interface id,
binding, or authorization object appears on this surface.

A host MUST return errors as
`{ "error": { "code", "message", "requestId", "retryable", "hostCode?" } }`.
The versioned portable API normalizes validation failures to
`invalid_argument`; a host-specific cause may be retained in `hostCode` or
`details`. Compatibility-facade codes such as `invalid_spec`, and the package
verifier's internal `schema_validation_failed`, are not portable wire codes.

The complete stable wire taxonomy is:

- request and authority: `invalid_argument`, `unauthenticated`,
  `permission_denied`, and `policy_denied`;
- exact Form identity and availability: `form_unknown`,
  `form_not_installed`, `form_unavailable`, and `form_identity_conflict`;
- Resource lifecycle: `resource_not_found`, `resource_version_conflict`,
  `resource_busy`, and `import_conflict`;
- host execution: `backend_unavailable` and `internal_error`;
- optional Interface projection ambiguity: `interface_identity_ambiguous` and
  `interface_instance_ambiguous`.

The HTTP mapping is fixed:

| Code | HTTP |
| --- | ---: |
| `invalid_argument` | 400 |
| `unauthenticated` | 401 |
| `permission_denied`, `policy_denied` | 403 |
| `form_unknown`, `resource_not_found` | 404 |
| `form_not_installed`, `form_identity_conflict`, `resource_busy`, `import_conflict`, `interface_identity_ambiguous`, `interface_instance_ambiguous` | 409 |
| `resource_version_conflict` | 412 |
| `form_unavailable`, `backend_unavailable` | 503 |
| `internal_error` | 500 |

An unknown code, a stable code paired with a different HTTP status, or
`retryable: true` on any code other than `resource_busy` and
`backend_unavailable` is protocol-invalid. A client MUST NOT give such a
response stable code semantics, retry it, or interpret it as Resource absence.

Generic legacy aliases such as `unauthorized`, `forbidden`, `conflict`, and
`not_implemented` are not portable wire codes. Only `resource_busy` and
`backend_unavailable` are automatically retryable, and only when the envelope
also sets `retryable: true`. All other codes fail without an automatic retry.
Only HTTP 404 with the structured code `resource_not_found` means that a
Resource is absent. A plain router 404 or another 404 code MUST NOT remove
provider state or make delete appear successful.
Provider diagnostics MAY expose the stable code and request ID. Provider state
MUST NOT contain credentials, prices, quotes, backend selection, target
identity, or manager authority.

## Cross-repo conformance

[`conformance/portable-host-v1/contract.json`](../../conformance/portable-host-v1/contract.json)
is the digest-pinned input for any neutral black-box host runner. The contract
names a provider-independent runner subject and pins a digest over that subject,
runner input, mutation preconditions, idempotent operations, and required check
set. It contains no host repository path or closed implementation identity.
`go run ./cmd/portable-host-conformance self-test` executes the checked-in
runner against a deterministic disposable host over real HTTP. The `run`
command drives the same matrix against a disposable external conformance
endpoint. Runner-only error, revoked-authorization, and instrumented
plan-binding probe headers, plus the malformed-response raw-JSON probe header,
are disposable test-adapter transport, not portable API features, and MUST NOT
be implemented on production endpoints. The report
separates pure black-box plan inputs from fields exercised through the
instrumented adapter; the latter is evidence that the adapter invoked the same
canonical binding function, not wire-only causal evidence.
`go run ./cmd/standard-form-conformance verify` checks that contract against
the exact release-owned identity it names. Neither local self-test output nor
an unsigned endpoint run is an admitted host report.

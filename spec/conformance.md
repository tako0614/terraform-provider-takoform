# Conformance language and classes

## Requirement keywords

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**,
**SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** in the
Takoform specification are to be interpreted as described in
[BCP 14](https://www.rfc-editor.org/info/bcp14)
([RFC 2119](https://www.rfc-editor.org/rfc/rfc2119),
[RFC 8174](https://www.rfc-editor.org/rfc/rfc8174)) when, and only when, they
appear in all capitals.

Prose that does not use those keywords is explanatory. A normative JSON Schema
is the structural minimum for its document, not the complete acceptance
contract. Implementations MUST also enforce the BCP 14 semantic verifier rules
in the owning Form Definition, Form Package, Interface, and trust sections; a
schema-valid document can therefore still be non-conforming.

Where a prose statement and a normative schema directly disagree about the
same structural condition, **the schema wins**. Semantic rules that the schema
does not express add fail-closed requirements rather than contradicting it.
Every schema this specification declares normative is listed in
[`schemas/`](schemas/).

## Conformance classes

Takoform defines four independent roles. An implementation claims one or more
of them, and a claim is only ever about the class it names: a conforming host
does not become a conforming publisher by hosting Forms, and a conforming
provider does not promote Form maturity.

### Form Package (data)

A **conforming Form Package** MUST satisfy
[`form-package/`](form-package/) and [`form-definition/`](form-definition/):
one definition, one exact `FormRef`, RFC 8785 canonical bytes, a closed file
inventory, allowlisted data media types, and no executable, credential,
placement, or commercial content. It MUST NOT depend on any host to be valid.

### Form host (protocol)

A **conforming Form host** MUST implement [`host-api/`](host-api/): versioned
discovery, exact Form availability, reviewed preview and apply, optimistic
concurrency, idempotent lifecycle, and the stable error taxonomy. It MUST
reject a request whose exact Form identity it has not installed, and it MUST
NOT return a Resource whose name, space, or Form identity differs from the one
requested.

A conforming **v1beta1 host** ([`host-api/v1beta1.md`](host-api/v1beta1.md))
additionally MUST: issue immutable UIDs and change them across
delete/recreate; increment `generation` only on desired-spec change and
`revision` on any representation change, serving the quoted revision as the
strong ETag; reject stale generation and revision fences with
`generation_conflict` and `revision_conflict`; fence a delete on the expected
generation and never require the revision, which a client's own teardown moves;
keep the package digest out of
resource identity, queries, and fences; enforce role rules (no in-place
revision update, `dependency_in_use` for bound targets); complete accepted
mutations either synchronously or through resumable Operations; verify
content-addressed artifact manifests blob-by-blob; publish Host Support
Profiles free of price, SKU, region, and quota; serve a namespaced Form group
as two ordinary path segments with no percent-encoded slash anywhere in a path;
answer an operation or upload handle presented by any other tenant or principal
with the surface's ordinary not-found outcome rather than a forbidden one; and
answer a manifest or blob read only for a caller whose tenant already holds
that content address
([decision 0018](decisions/0018-the-host-api-is-deployable-behind-ordinary-infrastructure.md)).

A conforming **exact Interface implementation**
([`interface-contract/`](interface-contract/)) MUST satisfy each operation's
input/output schemas, the closed error vocabulary, the declared consistency
and pagination semantics, and every deterministic behavior fixture of the
exact Interface Definition it claims.

When desired state contains a portable Connection, the host MUST resolve its
`Kind/name` only within the source Resource's exact `metadata.space`.
Cross-Space selection is not part of the portable Connection shape, and a host
MUST NOT substitute a target from another Space. A target absent from the
source Space fails apply as `resource_not_found` / HTTP 404 before mutation,
even if the same identity exists in another Space or preview previously
returned a plan.

Host Support evidence MUST execute every `desired`-stage negative fixture from
the exact package bytes. It does not claim `observed`-stage coverage:
portable HTTP has no operation that injects a fabricated observation into an
otherwise conforming host.

Every Experimental or Stable Form MUST declare at least one `desired`-stage
negative fixture. Host Support and provider compatibility evidence therefore
both have a non-empty negative-fixture set to execute.

A host **MAY** additionally implement the optional read-only interface
declaration surface ([`interface-declaration/`](interface-declaration/)). A host
that does not is still fully conforming.

A host decides placement, capacity, credentials, and commercial terms. Those
decisions are outside this specification, and a host MUST NOT require them to
appear in portable desired state.

### Form provider (client)

A **conforming provider** MUST send only declared desired state and MUST NOT
place a credential, price, target, or backend selection in its state. On the
retained v1alpha1 and v1alpha2 lanes it MUST carry the exact five-field
installed Form identity on every mutation; on the v1beta1 lane it carries
the exact four-field FormRef, with the package digest as optional audit
evidence that never enters resource identity, queries, or fences
([decision 0011](decisions/0011-resource-identity-generation-and-revision.md)).

Provider compatibility evidence MUST execute every `desired`-stage negative
and every `observed`-stage negative, rejecting the latter before host-produced
status enters provider state. When the runner defines no semantics for a
fixture stage, its presence fails the compatibility claim closed instead of
being counted as executed evidence.

### Form publisher (distribution)

A **conforming publisher** MUST satisfy [`trust/`](trust/): immutable release
bytes, exact digests, provenance, and an append-only revocation chain. A
publisher MUST NOT overwrite or re-sign a released version in place.

## What conformance is not

Passing the package/schema checks proves the local data contract. Passing the
portable host runner self-test additionally proves that the checked-in
black-box runner can detect the lifecycle failures in its pinned matrix over
HTTP; it proves only the disposable reference host used by that test.

Neither result is, or may be presented as, evidence of publication, external
Host Support, production activation, Form maturity, revocation enforcement, or
interoperability with a particular host. Those require evidence from the party
that actually performed the external operation. The maturity requirements are
owned by [`project-lifecycle.md`](project-lifecycle.md).

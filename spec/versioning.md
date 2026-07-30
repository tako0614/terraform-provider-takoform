# Versioning and stability

Takoform versions four things independently, because they change for different
reasons and at different speeds. Conflating them is how a "stable" label ends
up meaning nothing.

| What | Identifier | Changes when |
| --- | --- | --- |
| API group | `forms.takoform.com/v1alpha1` | the protocol envelope, discovery, or lifecycle semantics change |
| Form definition | SemVer per Form, inside its exact `FormRef` | that one Form's desired shape changes |
| Form Package | SemVer `packageVersion` | the packaged bytes for one definition change |
| Provider | SemVer release | the Terraform/OpenTofu provider binary changes |

## Provider releases

The provider `v1.0.1` candidate begins the stable `v1` provider compatibility
line. Within `v1.x`, a release MUST NOT remove an existing
resource type, silently reinterpret persisted state as a different Form, or
make an existing valid resource configuration invalid. Such a change requires
provider `v2`. Compatible optional fields, new resource types, and bug fixes
remain allowed within `v1`.

The initial `v1.0.1` surface is exactly the 34 typed Form resources plus the
read-only `takoform_interface` data source. The writable
`takoform_interface` resource published in `v0.2.x` is intentionally outside
that stable surface because Interface write authority belongs to the host.
This is not the only v0.2.1 compatibility break: all 33 Form kinds common to
v0.2.1 and v1.0.1 changed their exact FormRef/package identity, and
`HttpService` was replaced by `EdgeWorker`. Provider v1 begins resource schema
version `1`, records the exact Form identity in state, and gives version `0`
only a diagnostic-only rejection handler with no transformed state or Resource
lifecycle request. The explicit migration boundary is documented in
[`../release/migrations/v0.2.1-to-v1.0.1.md`](../release/migrations/v0.2.1-to-v1.0.1.md).

This promise applies only to the provider binary and its Terraform/OpenTofu
surface. It does not graduate the portable API group, admit a Form as
`portable-standard`, or make any host implementation stable.

Provider, Form definition, Form Package, and admission versions MUST NOT be
coordinated merely to share a number:

- a provider release may implement a mixed set of Form definition versions;
- each Form Package has its own immutable SemVer and binds one exact FormRef;
- an admission generation identifies one selected closure and is not a
  provider SemVer;
- changing the provider major does not reset an already-published Form
  identity.

In particular, `EdgeWorker@1.0.0` and `EdgeWorker@1.0.1` are already-published
immutable identities. The rebuilt provider-neutral line therefore did not
reuse `EdgeWorker@1.0.0`. Its retained `2.0.0` release source also remains
unmodified: the later credential-free artifact URL constraint narrows desired
state and therefore starts `EdgeWorker@3.0.0`, including when implemented by
provider `v1.0.1`.

The decision is recorded in
[`decisions/0001-provider-v1-keeps-form-versions-independent.md`](decisions/0001-provider-v1-keeps-form-versions-independent.md).
The machine-readable lock is the `versioning` object in
[`../release/version.json`](../release/version.json).

## Form definitions

A Form's identity is its exact `FormRef`, and the `schemaDigest` in that
reference covers the definition's canonical bytes. Two definitions with
different bytes are therefore different Forms, whatever they are called.

- A **patch** change MUST NOT alter the desired schema at all; it exists for
  documentation and fixture corrections.
- A **minor** change MAY add an optional field or relax a constraint. Existing
  valid desired state MUST stay valid.
- A **major** change MAY remove a field, tighten a constraint, or change
  meaning. It starts a new major line.

The owner gate scans every published identity discoverable from retained
package release manifests in admission snapshots, published-package sets, and
local Form release tags. Before generation mutates the repository, it
authenticates the retained
signed package index, exact package digest, and payload byte closure against
the corresponding `forms/releases` source. Candidates are built in temporary
staging. If a candidate reuses a published `(kind, definitionVersion)`, it must
be byte-for-byte identical to that retained source, and generation leaves the
published directory untouched. A different title, observed/output schema,
fixture, package-index representation, or any other package byte therefore
requires a new Form/Package identity even when `desiredSchema` is unchanged.

The SemVer compatibility check is additional to that no-overwrite gate. It
compares patch `desiredSchema` values after RFC 8785 canonicalization and
rejects any difference. For a minor, it must conservatively prove that the new
schema accepts every desired document accepted by every earlier release in
that major line. Optional fields on closed objects and directly provable
constraint relaxations pass; conditional, recursive, or otherwise unsupported
schema changes fail closed until the proof is strengthened or a major version
is chosen.

A published definition version MUST NOT be reshaped. A kind token whose earlier
major line was published starts its rebuilt definition at the next major
version, so no consumer can resolve one identity and receive different bytes.

## API group

`v1alpha1` says exactly what it says: the envelope may still change in ways
that break implementations, and this project does not yet promise otherwise.

The group graduates to `v1beta1` when all of the following hold:

1. two or more independently operated hosts pass the host conformance class
   against the same Form set, with signed evidence;
2. [`host-api/operations.json`](host-api/operations.json) has covered every
   operation for one release cycle with no breaking change;
3. the interface declaration surface has at least one host materializing a
   declared interface end to end;
4. a published Form Package has been installed, activated, and read back by a
   host that did not publish it.

It graduates to `v1` when, additionally, a deprecation and removal policy has
been exercised at least once on a real Form, and the revocation chain has been
consumed by a host in production.

Until then, this specification MUST NOT be described as stable, and no artifact
in this repository may claim `portable-standard` classification without the
external evidence its inventory lists.

## Deprecation

A Form is deprecated by publishing a new definition version with
`status: deprecated`. Deprecation is not revocation: the bytes stay available
and readable, hosts MAY continue to realize existing Resources, and consumers
SHOULD stop creating new ones.

Security revocation is separate, append-only, and described in
[`trust/`](trust/). It blocks new create, update, and activation while keeping
the referenced bytes available for safe observation, deletion, or an explicit
operator evacuation path.

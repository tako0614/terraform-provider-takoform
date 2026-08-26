# Release trust profile

This directory records the D-08 trust decision — the owning engineering
policy's package/provider trust decision, internal identifier D-08 — for
Takoform provider and Form Package artifacts. Requirement keywords are used as described in
[`../conformance.md`](../conformance.md). It does not make either artifact publishable. The
machine-readable authority is [`profile.json`](profile.json); implementation
and live distribution evidence must satisfy it before a release descriptor can
become publishable.

## Separate trust lanes

The Terraform/OpenTofu provider and Form Packages use separate trust lanes.

The provider follows the Terraform Registry contract:

- a signed immutable `v*` tag selects the source commit;
- deterministic GitHub Release assets are identified by SHA-256;
- the archive-plus-Registry-manifest checksum file is signed with the RSA OpenPGP key
  whose full fingerprint is pinned in `release/version.json` and
  `profile.json`;
- five SPDX 2.3 documents cover the exact provider archives separately and
  are never projected as Registry provider packages;
- one RFC 8785 canonical in-toto Statement v1 with SLSA Provenance v1
  exact-closes the 13 pre-provenance assets by name, byte size, and SHA-256;
  its detached OpenPGP signature is authenticated by the same pinned provider
  key;
- an existing version MUST NOT be overwritten;
- the `tako0614` public namespace and pinned key ID `34FC18AC897FB709` are
  registered; provider `v3.0.0` is published, and the append-only provider
  release identity ledger records a direct OpenTofu 1.12.3 Registry install,
  the exact signing key ID, provider archive digest, dependency-lock digest,
  31-resource schema digest, and immutable GitHub Release readback. Provider
  `v1.0.1` remains published retained history with authenticated direct
  Registry installation through both OpenTofu and Terraform.

Form Packages do not reuse the provider GPG key. The standard Takoform
publisher uses a keyless Sigstore blob signature and bundle bound to the exact
repository, protected workflow, and `forms/*/v*` tag. The bundle MUST carry a
transparency-log inclusion proof so an operator can verify retained artifacts
without trusting the distribution endpoint at verification time. A third-party
publisher has no ambient trust: an operator MUST install a separate publisher
policy for its exact issuer and source identity.

## Canonical package identity

Form Definition and package-index JSON use the JSON Canonicalization Scheme in
RFC 8785. Inputs are UTF-8 I-JSON and reject duplicate object names, invalid
Unicode, non-finite numbers, and negative zero. The exact identity is
`sha256:<lowercase hex>` over the RFC 8785 bytes of the package index. The index
lists the digest, media type, and size of every data payload.

An archive is only a transport container; archive metadata is not the semantic
package identity. A verifier MUST validate the signed canonical index and every
referenced payload before exposing a definition. It rejects unlisted files,
path traversal, links, devices, executable files, credentials, operator
configuration, target/capacity data, prices, billing fields, and executable
validation or adapter code.

## Provenance and publication

The provider's public GitHub Release inventory contains exactly 15 assets. The
13 provenance subjects are five provider archives, their five SPDX documents,
the provider manifest, `SHA256SUMS`, and its detached OpenPGP signature. The
remaining two assets are the canonical provenance statement and its detached
OpenPGP signature. To avoid an impossible self-reference, neither provenance
asset is a subject of the statement. The statement additionally binds the
source commit, release tag, tooling commit, workflow identity, workflow run and
attempt, request ID, and exact annotated tag-object OID and SHA-256. A verifier
MUST reject an omitted, extra, renamed, resized, or redigested subject, a
non-canonical statement, an incomplete build binding, or a provenance
signature that does not verify with the pinned provider key. GitHub artifact
attestation is not provider release authority in this profile.

The package index receives a Sigstore v0.3 bundle before a draft release can
become public. The release also contains an in-toto Statement v1 with SLSA
Provenance v1 and an SPDX 2.3 data-artifact SBOM. The provenance binds the exact
package artifacts to their source commit and protected build workflow. The
protected preparation artifact separately binds the exact release inventory,
source/tooling commits, and workflow run/attempt before local publication.

The implemented distribution lane is an immutable GitHub Release. Current
v1alpha2 packages use `forms/<release-id>/sha256-<hex>`; retained Legacy
v1alpha1 releases keep `forms/<release-id>/v<semver>`. The release ID is
reversible lowercase unpadded base32 of the exact FormRef Kind. A protected-main
`workflow_dispatch` accepts the exact planned tag and approved commit, verifies
their canonical release-plan binding and main ancestry, creates only an
ephemeral local tag object, and prepares the assets without repository-write
authority. It uses the protected `form-package-release` Environment,
commit-pinned Actions, Cosign v3 keyless blob signing, immediate
identity/transparency verification, and a checksum-closed same-run artifact.
The owner repository's local deploy entrypoint verifies that exact candidate,
then creates the tag and performs draft/upload/publish/immutable readback with
operator credentials. A connected or air-gapped mirror copies the exact release
assets only after signature,
transparency proof, provenance, and digest validation. Installation is an
operator action; a customer request path never fetches a package or executable
extension.

The FormRef, Form Definition, package-index and revocation schemas, RFC
8785/I-JSON implementation, closed local verifier, positive/negative corpus,
release builder, keyless Sigstore workflow, and append-only revocation delivery
lane now exist. The retired `1.0.1` packages have real immutable releases; their
exact release closures and signed indexes are retained in repository history
(the admission trees were withdrawn with the Legacy epoch, decision 0042) with
a TUF-authenticated production root and a digest-pinned, version-bound
historical aggregate package publisher policy. All 34 retained Legacy Form Packages
also have signed immutable releases, and provider `v1.0.1` has retained
authenticated direct-Registry readback. No revocation statement has been
released. The historical admission closure is source-retained,
offline-authenticated lifecycle evidence, not another package or activation
release and not a current maturity authority. Remote host
distribution/install, host-side publisher-policy enforcement, activation, and
revocation consumption remain host/operator work.

The generated all-34 source inventory, definition `status: standard`, and the
exact-ten v4 `portable-standard` closure are immutable Legacy facts. Published
tag `forms/admissions/v1.0.7` pins the final historical v4 closure.
The retained Takosumi host closure proves that those
exact ten identities passed that host's historical lifecycle evidence. It does
not define a current approved subset or make the other 24 lower-maturity Forms.
Current Form maturity, Host Support, and availability are separate as defined
by [`../project-lifecycle.md`](../project-lifecycle.md).

Provider distribution and the retained historical admission evidence used an
explicit two-phase authority split. The first phase published signed provider
`v1.0.1`; installability did not promote or activate any Form. After both
supported CLIs installed that exact
version from the canonical Registry FQN, independent protected workflows
produced signed host reports, all-34 provider reports, one canonical Registry
readback, and 10 admission-evidence subjects. Reviewed source retains those
exact candidates, package-release readbacks, and the resulting historical set.
The history verifier pins published admission tag objects, commits, retained
trees, and set digests from the closed identity ledger. It publishes no
aggregate archive or GitHub Release and performs no controller promotion.
Provider GPG
authority, independent package publisher identity, and host, provider,
Registry-readback, and admission-evidence identities remain distinct.

The direct Registry provider is untrusted executable code. It runs only in a
read-only job with no protected Environment, OIDC token, attestation, or
repository-write permission. Only its canonical matrix and readback cross the
job boundary. A distinct protected job verifies the same-run artifact and signs
the canonical readback, producing a non-published checksum-closed candidate.
Those candidates and the later signed admission-evidence subjects are retained
as historical source. The central assembly command and signing workflow have
been removed. The history verifier authenticates published Git identities; it
does not reinterpret old evidence under current lifecycle rules. There is no
current publication job, approval controller, or set-wide stability
transition.

## Withdrawn offline admission evidence

The offline Legacy admission-evidence verifier, its `admission/v1` and
`admission/v4` retained trees, pinned offline Sigstore trust material, and the
history gates that authenticated them were withdrawn with the Legacy epoch
([decision 0042](../decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
The evidence bytes, formats (`takoform.standard-admission-set@v2`/`@v3`,
`takoform.standard-runner-report@v1`,
`takoform.standard-provider-runner-report@v2`), and the verifiers that read
them are all preserved in this repository's git history, where the withdrawal
left them exactly as published; nothing reinterprets that history under
current lifecycle rules, and no current approval derives from it.

What remains standing from that machinery is what published bytes still need:
the pinned Sigstore trust root now lives at
[`release/trust/trusted-root.json`](../../release/trust/trusted-root.json),
and the append-only revocation delivery lane continues to serve every epoch's
published packages.

## Rotation and revocation

Specification 1.1 treats official and external publishers equally at the
Form/Package contract boundary. An operator explicitly chooses the trusted
source, issuer, signature, and revocation policy; publisher branding is not a
trust grant. Provenance is admission evidence and remains outside exact
`FormRef` equality. Authored, verified, published, installed, supported,
activated, provisioned, client-supported, and offered are independent
lifecycle facts.

Provider key rotation is additive: register and pin a new public key before a
new version, retain old public keys for historical verification, and never
replace old release bytes. A compromise disables the release Environment,
removes its secrets, publishes the OpenPGP revocation to operators, coordinates
Registry key removal through the maintainer/support process, and resumes only
with a new key and new semver.

Form Package keyless identity rotation is a reviewed change to the pinned OIDC
issuer/repository/protected-main workflow policy. An append-only revocation
statement references an exact package digest. The signed subject is a
cumulative checkpoint containing every statement from sequence 1 through its
current sequence and the previous checkpoint digest. Security revocation blocks
new create, update, and activation, but referenced package bytes remain
available for safe observe/delete or an explicit operator evacuation path.
Deprecation is not
security revocation, and neither state replaces package bytes in place.

The delivery sources are `forms/revocations/<statementVersion>.json` and
`forms/revocations/checkpoints/<statementVersion>.json`, selected by
`forms/revocations/v<statementVersion>`. CI rejects edits, renames, and deletion
of existing source paths. A host verifies the checkpoint bundle and exact
protected-main publisher identity, requires sequence 1 for its initial pin,
then durably stores `(sequence, canonical checkpoint digest, cumulative entries
digest)` and accepts only the next sequence whose `previousCheckpointDigest`
equals that pin and whose retained entry prefix has the same digest. This
detects rollback, omission, prefix rewrite, and forks. Runtime enforcement remains separate
from delivery and is not yet claimed by this repository.

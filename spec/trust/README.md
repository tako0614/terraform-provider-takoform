# Release trust profile

This directory records the D-08 trust decision for Takoform provider and Form
Package artifacts. Requirement keywords are used as described in
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
  registered; provider `v1.0.1` is published, and its retained authenticated
  readback proves direct canonical Registry installation through both OpenTofu
  and Terraform.

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

The implemented initial distribution lane is an immutable GitHub Release. It
uses `forms/<release-id>/v<semver>`, where the release ID is reversible
lowercase unpadded base32 of the exact FormRef Kind. A protected-main
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
exact release closures and signed indexes are retained under `admission/v1`
with a TUF-authenticated production root and a digest-pinned, version-bound
historical aggregate package publisher policy. All 34 current Form Packages
also have signed immutable releases, and provider `v1.0.1` has retained
authenticated direct-Registry readback. No revocation statement has been
released. Admission is a source-retained, offline-authenticated evidence
decision, not another package or activation release. Remote host
distribution/install, host-side publisher-policy enforcement, activation, and
revocation consumption remain host/operator work.

The generated all-34 source inventory remains `structural-candidate`; that
local classification and definition `status: standard` do not admit a Form.
The exact 10 package identities authenticated by the retained v4 closure and
protected `forms/admissions/v1.0.7` tag are `portable-standard`. The other 24
are published but not admitted. The retained Takosumi host closure proves only
those exact 10 on that host. Source definition status, package publication, or
a different host report cannot inherit that classification. The legacy
packages remain compatibility candidates.

Provider distribution and standard Form admission use an explicit two-phase
authority split. Phase 1 published signed provider `v1.0.1`; installability did
not admit or activate any Form. After both supported CLIs installed that exact
version from the canonical Registry FQN, independent protected workflows
produced signed host reports, all-34 provider reports, one canonical Registry
readback, and 10 admission-evidence subjects. Reviewed source retains those
exact candidates, package-release readbacks, and the resulting admission set.
The closure gate authenticates them offline and resolves the exact provider,
package, and `forms/admissions/v1.0.7` tags. It publishes no aggregate archive
or GitHub Release and performs no controller promotion. Provider GPG
authority, independent package publisher identity, and host, provider,
Registry-readback, and admission-evidence identities remain distinct.

The direct Registry provider is untrusted executable code. It runs only in a
read-only job with no protected Environment, OIDC token, attestation, or
repository-write permission. Only its canonical matrix and readback cross the
job boundary. A distinct protected job verifies the same-run artifact and signs
the canonical readback, producing a non-published checksum-closed candidate.
That candidate is retained in reviewed source with the independently signed
host and provider candidates. Admission assembly accepts only their exact
retained commit, tree, path, run, and digest bindings, then signs only the ten
admission-evidence subjects. The offline closure verifier authenticates those
retained bytes and their exact Git refs. There is no separate publication job,
release-safety controller, controller readback, or set-wide stability
transition.

## Offline standard-admission verification

The admission-closure gate has an offline verifier for the complete retained
standard-admission closure. For every member of the compiled candidate set it
requires the exact RFC 8785 admission-evidence document, canonical host and
provider runner reports, the immutable Form Package release manifest and its
exact five-asset readback, and the keyless-signed canonical package index. One
signed provider Registry readback must additionally bind the entire set to a
two-CLI lifecycle matrix whose nested reports were produced with
`installationSource: direct-registry-install`. A local `dev_overrides` matrix
is explicitly rejected.

The retained admission directory must contain these reviewed source inputs:

```text
admission/v1/trust/offline-sigstore-pins.json
admission/v1/trust/trusted-root.json
admission/v1/trust/publisher-policy.json
admission/v1/trust/host-report-policy.json
admission/v1/trust/provider-report-policy.json
admission/v1/trust/package-index-policy.json
admission/v1/trust/registry-readback-policy.json
admission/v1/registry/provider-readback.json
admission/v1/registry/provider-readback.sigstore.json
admission/v1/registry/provider-lifecycle-matrix.json
admission/v1/packages/<slug>/evidence.json
admission/v1/packages/<slug>/evidence.sigstore.json
admission/v1/packages/<slug>/host-report.json
admission/v1/packages/<slug>/host-report.sigstore.json
admission/v1/packages/<slug>/provider-report.json
admission/v1/packages/<slug>/provider-report.sigstore.json
admission/v1/releases/<release-id>/<version>/release-manifest.json
admission/v1/releases/<release-id>/<version>/<five exact release assets>
```

All retained identities and readback bytes are reviewed source. Historical
`admission/v1` deliberately omits
`registry/provider-readback.sigstore.json`; that incomplete historical lane is
not reusable as current admission evidence. The current lane instead requires
the signed Registry candidate to be retained in source before admission
assembly. The offline gate authenticates that retained bundle like every other
subject; it neither creates an activation archive nor publishes a release.

The `takoform.offline-sigstore-pins@v2` manifest binds the exact trusted-root
and five role-specific publisher-policy byte sets by canonical
`sha256:<lowercase-hex>` digest. Each strict publisher policy pins one exact
Fulcio OIDC issuer, certificate identity, and Sigstore v0.3 media type. The
five `(issuer, certificate identity)` pairs must be mutually distinct, so an
admission-evidence publisher, host runner, provider runner, package publisher,
or Registry-readback/admission authority cannot silently substitute for
another role. The
verifier accepts only keyless blob message signatures over the exact retained
subject SHA-256, requires a verified Rekor inclusion proof and signed
integrated time, validates the Fulcio chain and exact identity, and requires a
verified certificate-transparency SCT. It reads only retained regular files
below `admission/v1`; parent-directory symlinks and network lookups are
rejected by construction.

The historical `admission/v1` set uses
`takoform.standard-admission-set@v2`; the current retained `admission/v4`
closure uses `takoform.standard-admission-set@v3`. The earlier v1 formats were
an intentionally non-opening pre-release foundation: no real set or trust pins
were installed and no provider release could pass them. Therefore v2 was a
clean pre-publication contract replacement, not a migration of admitted or
customer state, while v3 adds the generation-aware current closure. Test
fixtures use an explicit in-process fake subject verifier and are never written
under the repository's `admission/` path; they do not represent signatures or
live evidence.

Canonical host reports remain
`takoform.standard-runner-report@v1`. Historical provider reports may also use
that format; v1 contains only its runner subject and version, exact
`(FormRef, packageDigest)`, `passed` status, all eight lifecycle booleans,
named positive fixture results, and named negative results normalized to
`invalid_argument`. Fixture closure is role-specific and stage-derived: the
host report covers the exact `desired` negatives it can submit through the
portable API, while the provider report covers those plus exact `observed`
response negatives. Neither role may claim an `output` negative until a
normative execution contract exists for that stage.
A standard-admission candidate MUST contain at least one `desired` negative, so
both the host and provider report closures are non-empty.

Current provider reports use the distinct
`takoform.standard-provider-runner-report@v2` format. It adds the required
`providerBinarySha256` field so the report binds the exact provider executable
used by the lifecycle and fixture runs. Current closure verification requires
all provider reports to carry the same digest and requires that digest and
provider version to equal every direct Registry installation readback. The
evidence-tooling source commit remains provenance for the report generator; it
is not incorrectly equated with the older immutable provider-release commit.
`profile.json` schema v2 locks the host/provider format split and digest
algorithm.

Each canonical report SHA-256 must equal both the admission set entry's role
digest and the corresponding
`AdmissionEvidence.conformance.*.evidenceDigest`. Unknown fields,
duplicate/failed fixtures, incomplete lifecycle, identity substitution, and
non-portable negative codes fail closed.

The deterministic package readback does not trust a download URL. The v2 set
pins the exact release-manifest bytes; the validator rereads all five assets,
checks every size and digest, requires the canonical index, archive, Sigstore
bundle, SPDX SBOM, and in-toto provenance names/media types, compares the index
to the provider-compiled candidate, verifies the deterministic tar entry order,
metadata, payload sizes/digests, and absence of unlisted archive entries, and
then authenticates that exact index. The SBOM and provenance are themselves
RFC 8785 canonical, strictly decoded evidence: the verifier recomputes the
SPDX file closure and package verification code and requires SLSA subjects,
source repository, tag, tagged-source commit, distinct protected-main tooling
commit, commit-versioned workflow builder, and canonicalization parameters to
match the exact retained package release. The historical `1.0.1` publisher
policy evidence pins the retired aggregate
`standard-form-package-set-release.yml@refs/tags/standard-forms/v1.0.1`
certificate identity and the same release commit. Unknown, duplicate, omitted,
or substituted metadata fails closed.
The canonical `takoform.provider-registry-readback@v1` similarly binds the
provider version/tag/commit, current release descriptor, candidate-set and
schema digests, both CLI/FQN/binary identities, and the exact direct-install
matrix digest. `cmd/admission-readback` renders this unsigned canonical subject
from a validated direct matrix. Historical v1 evidence retains its original
publisher identity. Current Registry evidence is signed only by
`.github/workflows/provider-registry-readback.yml`, whose identity is distinct
from package, provider-report, host-report, and admission-evidence publishers.

### Generation-aware successor lane

`admission/v3` is immutable `ga-core-v1` history. Its
`published-package-set@v2` authenticates the exact ten per-Form releases,
including `HttpService@1.0.0`, with the independent
`form-package-release.yml` identity. The retained signed report candidates
remain evidence of what ran, but do not grant admission and are not reused for
the successor.

`admission/v4` is the retained admitted `ga-core-v2` closure. It selects the
exact mixed-version ten-Form subset containing the provider-neutral
`EdgeWorker@3.0.0`. Provider reports use generation `portable-v1` and must
close over all 34 current Forms before the builder selects the ten exact
admission identities. Host reports use generation `ga-core-v2` and contain
exactly those ten identities. Neither manifest carries a false uniform
definition or package version.

The active assembly command is `standard-admission-material build-current`; it
requires signed host and full-provider candidates plus one signed Registry
readback candidate binding the two-CLI direct-install matrix, and produces
deterministic material outside the repository. Provider reports use
`takoform.standard-provider-runner-report@v2` and bind the exact provider
binary digest. `standard-admission-evidence.yml` signs only the ten evidence
subjects. It publishes no set-wide archive or GitHub Release. The exact signed
output and assembled closure are retained under `admission/v4`; the final
offline gate authenticates those retained bytes rather than an Actions
artifact.

During authentication, `admission-closure-check` resolves the admission tag,
provider tag, and every Form Package tag from fetched local Git refs and
requires their exact retained commits. The admission tag may identify an
ancestor of the current checkout only while the complete retained-root tree is
byte-identical, and the annotated provider tag must verify against the pinned
`3510E75E05BBCC303B92D77934FC18AC897FB709` GPG fingerprint. Package index
Sigstore authentication remains separate from that Git ref-existence fence.

The production Sigstore trusted-root snapshot, the distinct package-index and
Registry-readback publisher policies, every retired package-index bundle, and the exact immutable release
readbacks are installed and digest-pinned by
`admission/v1/published-package-set.json`. They pass the separate offline
`published-package-check` but grant no admission authority. Exact mutually
distinct admission-evidence, host-report, and provider-report policies and the
five-role offline pin manifest are retained. The v4 signed host/provider and
admission reports, canonical Registry readback, and
`standard-admission-set.json` are retained and authenticated.
The historical canonical Registry matrix/readback for provider `v0.1.3` are also
retained under the retired v1 evidence lane, but grant no current admission
authority. The current v4 lane requires a separately signed readback for the
exact provider `v1.0.1` release and its two-CLI direct-Registry matrix. The
admission checkpoint version is independent from the bound Form
definition/package versions; advancing it does not republish package bytes.
Only the approved role identities can produce that evidence; a distribution
endpoint, unsigned local output, or a different workflow identity cannot
substitute it.

## Rotation and revocation

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
current sequence and the previous checkpoint digest. Security revocation blocks new
create/update and activation, but referenced package bytes remain available for
safe observe/delete or an explicit operator evacuation path. Deprecation is not
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

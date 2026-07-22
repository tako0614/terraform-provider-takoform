# Release trust profile

This directory records the D-08 trust decision for Takoform provider and Form
Package artifacts. It does not make either artifact publishable. The
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
- SPDX 2.3 and SLSA provenance cover the exact release assets separately and
  are never projected as Registry provider packages;
- an existing version is never overwritten;
- the `tako0614` public namespace and pinned key ID `34FC18AC897FB709` are
  registered, while the first clean Terraform Registry/OpenTofu install proof
  remains a post-publication external gate.

Form Packages do not reuse the provider GPG key. The standard Takoform
publisher uses a keyless Sigstore blob signature and bundle bound to the exact
repository, protected workflow, and `forms/*/v*` tag. The bundle must carry a
transparency-log inclusion proof so an operator can verify retained artifacts
without trusting the distribution endpoint at verification time. A third-party
publisher has no ambient trust: an operator must install a separate publisher
policy for its exact issuer and source identity.

## Canonical package identity

Form Definition and package-index JSON use the JSON Canonicalization Scheme in
RFC 8785. Inputs are UTF-8 I-JSON and reject duplicate object names, invalid
Unicode, non-finite numbers, and negative zero. The exact identity is
`sha256:<lowercase hex>` over the RFC 8785 bytes of the package index. The index
lists the digest, media type, and size of every data payload.

An archive is only a transport container; archive metadata is not the semantic
package identity. A verifier must validate the signed canonical index and every
referenced payload before exposing a definition. It rejects unlisted files,
path traversal, links, devices, executable files, credentials, operator
configuration, target/capacity data, prices, billing fields, and executable
validation or adapter code.

## Provenance and publication

The package index receives a Sigstore v0.3 bundle before a draft release can
become public. The release also contains an in-toto Statement v1 with SLSA
Provenance v1 and an SPDX 2.3 data-artifact SBOM. The provenance binds the exact
package artifacts to their source commit and protected build workflow. GitHub
artifact attestations separately bind the exact release inventory and SBOM to
the workflow run.

The implemented initial distribution lane is an immutable GitHub Release. It
uses `forms/<release-id>/v<semver>`, where the release ID is reversible
lowercase unpadded base32 of the exact FormRef Kind. A protected-main
`workflow_dispatch` accepts the exact existing tag and approved commit, then
verifies tag existence, commit equality, and main ancestry before signing. It
uses the protected `form-package-release` Environment, commit-pinned Actions,
Cosign v3 keyless blob signing, immediate
identity/transparency verification, an exact draft inventory check, and
draft-then-publish finalization. A connected or air-gapped mirror copies the
exact release assets only after signature,
transparency proof, provenance, and digest validation. Installation is an
operator action; a customer request path never fetches a package or executable
extension.

The FormRef, Form Definition, package-index and revocation schemas, RFC
8785/I-JSON implementation, closed local verifier, positive/negative corpus,
release builder, keyless Sigstore workflow, and append-only revocation delivery
lane now exist. All ten `1.0.1` packages have real immutable releases; their
exact release closures and signed indexes are retained under `admission/v1`
with a TUF-authenticated production root and a digest-pinned, version-bound
aggregate package publisher policy. No revocation statement or admission activation has
been released. Remote host distribution/install, host-side publisher-policy enforcement,
activation, and revocation consumption still require implementation and live
evidence. The ten current provider resources have local deterministic `1.0.1 /
standard` definition candidate bytes and structural fixtures only. Their
inventory is `structural-candidate`, not `portable-standard`; definition status
does not admit them. Passed host/provider lifecycle reports, portable negative
wire-code coverage, Registry installation/readback, authenticated admission
evidence, and live revocation proof are still missing. Signature and
immutable-tag evidence now prove package publication only. Only authenticated
host/provider evidence can classify the exact
package `portable-standard`. The legacy packages remain compatibility
candidates.

Provider distribution and standard Form admission use an explicit two-phase
authority split. Phase 1 may publish a signed, deterministic provider version
while `release/version.json` and every package entry remain `candidate-only` /
`external-required`; installability does not admit or activate any Form. Phase
2 starts only after that same immutable version is available through both
canonical Registry FQNs. The protected `forms/admissions/v*` workflow then
requires the complete authenticated closure below, reruns and compares the
direct install matrix, and publishes a separately signed admission activation
release. Failure in Phase 2 leaves the already-public provider harmlessly
candidate-only. Provider GPG authority, package publisher identities, runner
report identities, and admission-release identity remain distinct.

The direct Registry provider is untrusted executable code. It runs only in a
read-only job with no protected Environment, OIDC token, attestation, or
repository-write permission. Only its canonical matrix crosses the job
boundary. The protected authentication job starts from a fresh exact-commit
checkout, compares that artifact byte-for-byte with reviewed source, and signs
the readback. It emits a non-published checksum-closed candidate. The separate
publication job consumes those exact bytes only after the ecosystem's fixed
release-safety controller binds the candidate run, authorization, adapter,
artifact set, target fingerprint, and ordered health checks. The final release
contains the controller readback before it becomes stable and is accepted only
after repository-enforced immutability is read back.

## Offline standard-admission verification

Provider `release-check` has an offline verifier for the complete retained
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

All retained identities and readback bytes are reviewed source. The one
`registry/provider-readback.sigstore.json` bundle is deliberately absent from
the tagged tree and is produced in the protected Phase 2 workflow only after
its fresh direct-install matrix exactly matches the retained matrix; the
activation archive retains the generated bundle. The offline gate then
authenticates it like every other subject before creating a release.

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

The admission set format is `takoform.standard-admission-set@v2`. The earlier
v1 formats were an intentionally non-opening pre-release foundation: no real
set or trust pins were installed and no provider release could pass them.
Therefore v2 is a clean pre-publication contract replacement, not a migration
of admitted or customer state. Test fixtures use an explicit in-process fake
subject verifier and are never written under the repository's `admission/`
path; they do not represent signatures or live evidence.

Each canonical `takoform.standard-runner-report@v1` document is role-bound as
`host-report` or `provider-report` and contains only its runner subject and
version, exact `(FormRef, packageDigest)`, `passed` status, all eight lifecycle
booleans, named positive fixture results, and named negative results normalized
to `invalid_argument`. Its canonical SHA-256 must equal both the v2 set entry's
role digest and the corresponding `AdmissionEvidence.conformance.*.evidenceDigest`.
Unknown fields, duplicate/failed fixtures, incomplete lifecycle, identity
substitution, and non-portable negative codes fail closed.

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
match the exact retained package release. The `1.0.1` publisher policy evidence
pins the aggregate
`standard-form-package-set-release.yml@refs/tags/standard-forms/v1.0.1`
certificate identity and the same release commit. Unknown, duplicate, omitted,
or substituted metadata fails closed.
The canonical `takoform.provider-registry-readback@v1` similarly binds the
provider version/tag/commit, current release descriptor, candidate-set and
schema digests, both CLI/FQN/binary identities, and the exact direct-install
matrix digest. `cmd/admission-readback` renders this unsigned canonical subject
from a validated direct matrix; only the protected activation workflow signs
it.

Before authentication opens, `admission-closure-check` resolves the admission
tag, provider tag, and every Form Package tag from fetched local Git refs and
requires the exact retained commit. The admission tag must point at the current
checkout, and the annotated provider tag must verify against the pinned
`3510E75E05BBCC303B92D77934FC18AC897FB709` GPG fingerprint. Package index
Sigstore authentication remains separate from that Git ref-existence fence.

The production Sigstore trusted-root snapshot, the distinct package-index and
Registry-readback publisher policies, all ten package-index bundles, and the exact immutable release
readbacks are installed and digest-pinned by
`admission/v1/published-package-set.json`. They pass the separate offline
`published-package-check` but grant no admission authority. Exact mutually
distinct admission-evidence, host-report, and provider-report policies and the
five-role offline pin manifest are now retained. The signed host/provider and
admission reports and `standard-admission-set.json` are now retained. The
canonical Registry matrix/readback for provider `v0.1.3` are also retained, but
grant nothing until the protected candidate reproduces the exact matrix bytes,
keyless-signs the readback, and the immutable admission activation Release is
published. The admission release version is independent from the bound Form
definition/package versions; advancing activation does not republish package
bytes. Public `release-check` then requires the completed protected controller
promotion, immutable exact-asset Release, and retained controller readback;
missing or mismatched live activation state keeps it fail-closed. The
approved role identities must still produce exact authenticated evidence; a
distribution endpoint or a different workflow identity cannot substitute it.

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

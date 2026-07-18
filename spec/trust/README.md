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
- the binary checksum file is signed with the RSA OpenPGP key whose full
  fingerprint is pinned in `release/version.json` and `profile.json`;
- SPDX 2.3 and SLSA provenance cover the exact release assets;
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
lane now exist. No real package or revocation statement has been released.
Remote host distribution/install, host-side publisher-policy enforcement,
activation, and revocation consumption still require implementation and live
evidence. The ten current provider resources have local deterministic `1.0.0 /
standard` definition candidate bytes and structural fixtures only. Their
inventory is `structural-candidate`, not `portable-standard`; definition status
does not admit them. Passed host/provider
lifecycle reports, portable negative wire-code coverage, signature, immutable
tag, Registry installation/readback, and authenticated admission evidence are
still missing. Only authenticated host/provider evidence can classify the exact
package `portable-standard`. The legacy packages remain compatibility
candidates.

## Offline standard-admission verification

Provider `release-check` has an offline verifier for one deliberately narrow
slice: the exact retained RFC 8785 admission-evidence document for each member
of the compiled standard candidate set. It does not authenticate host/provider
reports, a Form Package release readback, or Registry installation.

The retained admission directory must contain these reviewed source inputs:

```text
admission/v1/trust/offline-sigstore-pins.json
admission/v1/trust/trusted-root.json
admission/v1/trust/publisher-policy.json
admission/v1/packages/<slug>/evidence.sigstore.json
```

The pin manifest binds the exact trusted-root and publisher-policy bytes by
canonical `sha256:<lowercase-hex>` digest. The strict publisher policy pins one
exact Fulcio OIDC issuer, certificate identity, and Sigstore v0.3 media type.
The verifier accepts only a keyless blob message signature over the exact
retained evidence SHA-256, requires a verified Rekor inclusion proof and
signed integrated time, validates the Fulcio chain and exact identity, and
requires a verified certificate-transparency SCT. It reads only retained
regular files below `admission/v1`; parent-directory symlinks and network
lookups are rejected by construction.

No production trust root, publisher policy, admission set, or bundle is
installed in this repository yet. Their absence is intentional and keeps
`release-check` fail-closed. Adding the real retained files and pin digests is
evidence work, not a test-fixture generation step, and must not be synthesized
from the distribution endpoint during release.

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

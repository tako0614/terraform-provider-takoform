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
- Registry public-key registration and clean Terraform/OpenTofu install proof
  remain external release gates.

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

The package index receives a Sigstore bundle before a draft release can become
public. The release also contains an in-toto Statement v1 with SLSA Provenance
v1 and an SPDX 2.3 data-artifact SBOM. The provenance binds the exact package
digest to its source commit and protected build workflow.

The initial distribution is an immutable GitHub Release. A connected or
air-gapped mirror copies the exact release assets only after signature,
transparency proof, provenance, and digest validation. Installation is an
operator action; a customer request path never fetches a package or executable
extension.

The FormRef, Form Definition and package-index schemas, RFC 8785/I-JSON
implementation, closed local verifier, and positive/negative corpus now exist.
The Sigstore workflow and verifier, publisher-policy enforcement, remote
distribution/install, activation, and revocation operations do not. Until those
separate gates land, `formPackage.status` remains non-publishable, no Form
Package is released, and the ten current provider resources remain
compatibility candidates rather than portable standards.

## Rotation and revocation

Provider key rotation is additive: register and pin a new public key before a
new version, retain old public keys for historical verification, and never
replace old release bytes. A compromise disables the release Environment,
removes its secrets, publishes the OpenPGP revocation to operators, coordinates
Registry key removal through the maintainer/support process, and resumes only
with a new key and new semver.

Form Package keyless identity rotation is a reviewed change to the pinned OIDC
issuer/repository/workflow policy. A signed append-only revocation statement
references an exact package digest. Security revocation blocks new
create/update and activation, but referenced package bytes remain available for
safe observe/delete or an explicit operator evacuation path. Deprecation is not
security revocation, and neither state replaces package bytes in place.

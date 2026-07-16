# Provider release boundary

`release/version.json` is the independent Takoform provider version source. It
does not inherit a Takosumi package or release version.

The repository can build deterministic, unsigned candidate evidence:

```console
go -C ./cmd/provider-release run . verify-source
go -C ./cmd/provider-release run . verify-reproducible
go -C ./cmd/provider-release run . build --output ../takoform-provider-candidate
```

`build` refuses a dirty source tree, a missing or mismatched annotated tag, an
unverified tag signature, a different Go toolchain, or an existing output path.
The explicit `--allow-dirty-candidate` and `--allow-untagged-candidate` flags are
for local non-publishable evidence only. Any such exception is recorded in the
manifest and keeps `publicationReady=false`.

Every candidate contains:

- one deterministic archive for each configured platform;
- `SHA256SUMS`;
- `manifest.json` with archive and binary digests, source commit, embedded
  version evidence, and publication blockers;
- `sbom.spdx.json` generated from the exact Go module graph;
- `provenance.json`, an unsigned in-toto statement describing the build.

The tool never signs, uploads, tags, creates a GitHub Release, or publishes to a
Registry/mirror. The environment-gated `v*` tag workflow is the only release
producer. It imports the `provider-release` Environment secret key, verifies the
signed tag, uses the pinned
GoReleaser/Syft toolchain, creates the Registry manifest/checksum/binary detached
signature assets, and records GitHub build provenance.

Repository configuration is part of the trust boundary, not a claim made by
this tree. The workflow references the `provider-release` GitHub Environment,
but publication remains blocked until maintainers verify required reviewers on
that Environment plus protected `main` and restricted `v*` tag creation rules.

The release verifier is an isolated Go module under `cmd/provider-release`.
Its schema/attestation dependencies are not provider runtime dependencies and
do not appear in the provider module graph or provider SBOM.

The approved provider signing fingerprint is
`3510E75E05BBCC303B92D77934FC18AC897FB709`; its public key is pinned under
`release/keys/`. The private key and passphrase remain outside every repository
and are available to Actions only as `GPG_PRIVATE_KEY` and `PASSPHRASE`.
Terraform Registry public-key registration and first real Registry/network-
mirror install still require maintainer OAuth interaction. Existing version
paths must never be overwritten; corrections use a new semver.

Key rotation is additive and review-gated: create a distinct repo-external key,
change the pinned fingerprint/public key in one reviewed commit, register that
public key with the Terraform Registry before tagging, and retain old public
keys for verification of historical releases. Never reuse the Takoform key for
the separately owned Takosumi legacy/admin provider.

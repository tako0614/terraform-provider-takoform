# Provider release boundary

`release/version.json` is the independent Takoform provider version source. It
does not inherit a Takosumi package or release version.

The provider-specific trust lane is pinned by the D-08 profile in
[`../spec/trust/`](../spec/trust/). Form Packages use a separate keyless trust
lane and never reuse this provider GPG key. Its release and revocation delivery
boundary is documented in [`form-packages.md`](form-packages.md).

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

`release/version.json` also pins the supported CLI/FQN matrix. Release CI must
exercise OpenTofu `1.12.1` with
`registry.opentofu.org/tako0614/takoform` and Terraform `1.15.8` with
`registry.terraform.io/tako0614/takoform`. Both must expose the same schema and
complete lifecycle evidence for the exact embedded structural candidate set;
the two provider addresses are recorded independently and are never rewritten
as aliases.

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
The HCP organization `takoform` has claimed the public Terraform Registry
namespace `tako0614`; its GitHub App installation is limited to
`tako0614/terraform-provider-takoform`. Registry key ID `34FC18AC897FB709` is
registered and matches the full pinned fingerprint above. The first real
Registry and OpenTofu/network-mirror install proof remains pending and requires
an explicitly authorized first publication. Existing version paths must never
be overwritten; corrections use a new semver.

Key rotation is additive and review-gated: create a distinct repo-external key,
change the pinned fingerprint/public key in one reviewed commit, register that
public key with the Terraform Registry before tagging, and retain old public
keys for verification of historical releases. Never reuse the Takoform key for
the separately owned Takosumi legacy/admin provider.

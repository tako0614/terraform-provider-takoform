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

The pre-v1 legacy-provider migration report remains useful structural evidence,
but its operator-host refresh/rollback drills are external migration evidence,
not authority to publish this candidate-only provider. The tag and artifact
lanes therefore run the structural migration proof without
`--require-complete`; Form activation remains blocked by the separate complete
Phase 2 admission closure.

Every candidate contains:

- one deterministic archive for each configured platform;
- `SHA256SUMS`, whose signed Registry closure contains the five provider ZIP
  archives and the exact Registry metadata manifest;
- `manifest.json` with archive and binary digests, source commit, embedded
  version evidence, and publication blockers;
- `sbom.spdx.json` generated from the exact Go module graph;
- `provenance.json`, an unsigned in-toto statement describing the build.

The public GitHub Release inventory is deliberately broader than the signed
Registry checksum manifest. Per-archive SPDX documents and workflow
provenance/attestations remain immutable release evidence and are verified
separately; they must not be listed in `SHA256SUMS`. The Registry metadata
manifest is not an installable package, but the public Registry ingress
contract requires its digest in the signed checksum file. Both Terraform and
OpenTofu use the same six-entry checksum target set. Providers `v0.1.1` and
`v0.1.2` remain immutable: `v0.1.1` included SPDX evidence in `SHA256SUMS`,
while `v0.1.2` omitted the required Registry manifest. The corrected `v0.1.3`
candidate supersedes both without replacing any published byte.

`release/version.json` also pins the supported CLI/FQN matrix. Release CI must
exercise Terraform `1.15.8` and OpenTofu `1.12.1` with the same canonical
identity, `registry.terraform.io/tako0614/takoform`. Both must expose the same
schema and complete lifecycle evidence for the exact embedded structural
candidate set.

Provider publication is Phase 1 only. The `v*` workflow runs
`candidate-publication-check`, which requires `publicationStatus:
candidate-only` and the unchanged structural inventory. Publishing that exact
binary, checksums, SBOM, provenance, and signatures does not mutate
`admissionStatus`, create admission evidence, install a Form Package, or grant
host activation authority. This separation is required because a genuine
Public Registry readback cannot exist until the immutable provider version is
already public.

The normal `matrix` command intentionally uses a locally built provider binary
through `dev_overrides`; it is a pre-publication regression gate and is not
Registry evidence. After the first authorized publication, capture the
post-publication readback with:

```console
go run ./cmd/provider-lifecycle-conformance render-registry-matrix \
  --opentofu tofu --terraform terraform \
  > /tmp/provider-lifecycle-matrix.json
go run ./cmd/admission-readback registry \
  --matrix /tmp/provider-lifecycle-matrix.json \
  --provider-release-commit "$(git rev-list -n 1 "$(jq -r .tag release/version.json)")" \
  --output /tmp/provider-readback.json
```

During Terraform Registry propagation, a maintainer can diagnose one exact CLI
execution without weakening the dual-CLI matrix gate:

```console
go run ./cmd/provider-lifecycle-conformance render-registry-report \
  --cli /absolute/path/to/terraform
go run ./cmd/provider-lifecycle-conformance render-registry-report \
  --cli /absolute/path/to/tofu
```

Each command still performs the complete provider lifecycle through
`direct {}` and validates the resulting report, but the single-CLI report is
non-publishable diagnostic evidence. Only `render-registry-matrix` proves that
both reviewed CLIs install the canonical FQN and expose identical bytes,
schema, and lifecycle.

The matrix mode pins the exact descriptor version in generated configuration,
runs `init` with only `direct {}`, locates and hashes the downloaded provider
binary, and repeats the complete lifecycle. Its report carries
`installationSource: direct-registry-install`; the admission validator rejects
otherwise-valid matrices carrying `local-dev-override`. The matrix is still
not self-authenticating: it becomes usable only when an externally signed,
canonical `takoform.provider-registry-readback@v1` document binds its digest,
installed binary/schema digests, CLI/FQN identities, provider tag, and source
commit.

Current Phase 2 has no set-wide release or promotion lane. The protected
`provider-registry-readback.yml` workflow executes both direct Registry
installs and keyless-signs only the canonical readback with its own publisher
identity. `standard-provider-report.yml` independently signs all 34
`portable-v1` provider reports. A conforming host signs the exact ten
`ga-core-v1` host reports. The admission material retains and
offline-authenticates the complete 34-report provider candidate, including the
24 reports outside the selected ten. Finally, `standard-admission-evidence.yml` verifies
those retained candidates, builds the generation-aware v3 set twice, and signs
only the ten admission-evidence subjects. It does not create a tag, release,
registry entry, or production mutation.

The active local gates are
`current-published-package-check` and `current-admission-closure-check`.
Admission remains fail-closed until all retained subjects and exact Git refs
verify offline. The removed `standard-admission-release.yml`, controller
promotion input, release archive, and `release-check` path described a
set-wide artifact promotion that is not part of current Takoform.

The admission checkpoint semver is independent from each exact Form definition
and Form Package semver. It identifies one immutable source-retained closure;
it does not republish package bytes or create another distributable artifact.

`admission-closure-check` also resolves the admission tag, provider tag, and every
package tag from fetched local Git refs and requires their exact retained
commits. The provider tag must be annotated and signed by the pinned provider
GPG fingerprint; import only `release/keys/provider-signing-key.asc` before an
offline local check. A 40-character string without the corresponding immutable
ref is never release evidence.

The provider build tool never signs, uploads, creates a GitHub Release, or
publishes to a Registry/mirror. Maintainers dispatch the protected
`.github/workflows/provider-release-tag.yml` lane with the exact descriptor tag
and current protected-main commit. Its read-only preflight job has no protected
Environment, write token, or signing key and is the only job that executes Go,
the candidate provider, or either Terraform-compatible CLI. Only canonical
descriptor/build/SBOM/provenance/lifecycle digests cross the artifact boundary.
The protected signing job starts from a fresh exact checkout, performs only
static JSON/hash/Git/Registry-absence checks, imports the `provider-release`
Environment key, and exports a checksum-closed public signed-tag object without
repository write credentials. The signed message binds the protected-main
commit, complete preflight checksum inventory, and exact Actions run/attempt.
No local human signing key is required.

After the Environment-approved run succeeds, an admin maintainer downloads both
artifacts and performs the second half of the release boundary locally:

```console
gh run download <run-id> --name provider-tag-preflight-<commit> --dir /tmp/provider-tag-preflight
gh run download <run-id> --name provider-signed-tag-<run-id>-<attempt> --dir /tmp/provider-signed-tag
go -C ./cmd/provider-release run . verify-tag-artifact \
  --artifact /tmp/provider-signed-tag \
  --preflight-artifact /tmp/provider-tag-preflight \
  --expected-run-id <run-id> \
  --expected-run-attempt <attempt> \
  --expected-commit <commit> \
  --materialize-ref
git push origin refs/tags/$(jq -r .tag release/version.json):refs/tags/$(jq -r .tag release/version.json)
```

The verifier closes both inventories, reconstructs the exact public tag object
with `git mktag`, checks its expected object id and peeled target, imports only
the pinned public key in a temporary keyring, verifies the signer fingerprint,
and refuses to replace an existing local tag ref. The final push uses the
maintainer's existing admin authentication to cross the restricted tag-creation
ruleset; the Actions job itself cannot bypass that rule. That push triggers
`release.yml`, the only
provider artifact producer. Its read-only build job verifies the signed tag
with the public key, runs the candidate and pinned GoReleaser/Syft toolchain,
validates every final Syft document against the repository-pinned official
SPDX 2.3 schema, exports an exact unsigned evidence inventory, and closes the
five provider archives plus Registry metadata manifest under the signed
checksum file. A fresh
protected publication job executes no provider or repository Go code: it
statically rechecks the tag, inventory, Registry absence, and checksums, imports
the same Environment key, adds only the detached checksum signature, publishes
the exact draft assets, and records GitHub build provenance.

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
registered and matches the full pinned fingerprint above. Providers `v0.1.1`
and `v0.1.2` remain immutable GitHub Releases. Terraform Registry rejected the
first because its checksum manifest projected SPDX evidence as provider
packages, and rejected the second because the required Registry metadata
manifest was absent from `SHA256SUMS`. Existing version paths must never be
overwritten; the corrected six-entry candidate is `v0.1.3`, and direct
Terraform/OpenTofu Registry install evidence remains post-publication. The
coordinated Form `1.0.1` candidate therefore uses provider `0.1.3`.

Key rotation is additive and review-gated: create a distinct repo-external key,
change the pinned fingerprint/public key in one reviewed commit, register that
public key with the Terraform Registry before tagging, and retain old public
keys for verification of historical releases. Never reuse the Takoform key for
the separately owned Takosumi legacy/admin provider.

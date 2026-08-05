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

Provider `v1.0.3` is the current published `v1` release. Its signed tag,
immutable GitHub Release, and canonical Terraform Registry listing exist.
`release/version.json` retains `publicationStatus: candidate-only` because it
is tag-time build metadata, not a live availability field.

## Provider v1.0.3 RelationalDatabase schema inputs

Provider `v1.0.3` exposes the existing optional `schema_url`,
`schema_sha256`, and `schema_format` fields of the immutable
`RelationalDatabase@3.0.0` Form as typed HCL attributes. It does not mint or
replace a Form identity. Hosts remain responsible for fetching the exact
digest-bound schema bundle and converging it before reporting the Resource
Ready.

## Provider v1.0.2 Interface URL addition

Provider `v1.0.2` adds the optional computed `resource_uri` attribute to the
read-only `takoform_interface` data source. The host remains authoritative for
the URI; the provider does not infer a provider-specific hostname or copy the
URI into portable Resource desired state. Existing provider-v1 Resource state
and every immutable Form identity remain unchanged.

## Provider v1.0.1 resource transition

Provider `v1.0.1` removes the active `takoform_http_service` resource and adds
`takoform_edge_worker` for the distinct `EdgeWorker@3.0.0` Form identity. It
also removes the writable `takoform_interface` resource: portable Form
Definitions may declare non-secret Interface descriptors and the provider
retains the read-only `takoform_interface` data source, but Interface records,
bindings, write fencing, authorization, and lifecycle belong to the host.
Published `HttpService@1.0.0` bytes and provider `v0.2.1` remain immutable.

The compatibility break is broader than those two removed resource types.
Provider `v0.2.1` and published `v1.0.1` each compile 34 Form identities. They
share 33 kind names, but all 33 shared exact FormRefs/packages changed; there
are zero unchanged exact identities. Common names, artifacts, connections, and
semantic field contracts also changed. Every existing v0.2.1 Form resource,
not only `takoform_http_service`, must therefore stay pinned until an operator
performs an explicit create/cutover/destroy migration.

Provider v1 gives every Form resource schema version `1`, stores the exact
FormRef/package identity in state, and handles schema version `0` only with a
diagnostic that returns no transformed state and makes no Resource lifecycle
request. This makes a direct v0.2.1 state load fail before lifecycle code can
query the new exact identity, receive `404`, and erase old state as though the
old Resource had disappeared. Provider configuration may already have
performed host discovery; this fence covers Resource lifecycle requests.
Existing writable `takoform_interface` state also remains manageable only
while pinned to v0.2.1. If the host has adopted the Interface record, destroying
it through v0.2.1 can delete that remote record: verify host ownership and
binding continuity, then remove only the Terraform state entry. Destroy through
v0.2.1 only when the remote Interface itself is intentionally being retired.
An automatic transformation or v1 import target would falsely claim a portable
resource/data migration that does not exist.

Some v0.2.1 OpenTofu state may use the retired
`registry.opentofu.org/tako0614/takoform` identity. That address is not an alias
for `registry.terraform.io/tako0614/takoform`; when it is actually present,
normalize it with an explicit, reviewed `tofu state replace-provider` while
the resource version remains pinned to v0.2.1. Address replacement does not
migrate any Form resource. The full inventory, backup, address-normalization,
and per-resource cutover procedure is
[`migrations/v0.2.1-to-v1.0.1.md`](migrations/v0.2.1-to-v1.0.1.md), backed by
the machine-readable
[`migration audit`](migrations/v0.2.1-to-v1.0.1.json).

This is the first stable provider compatibility line, not a claim that any Form
epoch graduated. The `forms.takoform.com/v1alpha1` Form epoch is now frozen
Legacy; post-reset Forms use `forms.takoform.com/v1alpha2` inside
`packages.forms.takoform.com/v1alpha3` after an individual lifecycle transition.
The current nine are still unpublished candidates. The outer Host API wire independently
uses `forms.takoform.com/v1alpha2` behind `/.well-known/takoform/v1alpha2` for
provider v2. The frozen provider-v1 lane retains
`forms.takoform.com/v1alpha1` behind `/.well-known/takoform`; one discovery
document never advertises both epochs. Provider and Form Definition versions
remain independent; current Form Package artifacts are content-addressed while
retained Legacy package locators preserve their published identities.
Admission generations are a separate historical evidence stream. Published
`EdgeWorker@1.0.0` and
`EdgeWorker@1.0.1` identities cannot be reset, so the provider-neutral
definition uses the independent `EdgeWorker@3.0.0` identity. The intermediate
`2.0.0` release source remains unmodified; tightening its artifact URL grammar
required a new Form major. The exact contract is in
[`../spec/versioning.md`](../spec/versioning.md).

Every candidate contains:

- five deterministic provider archives, one for each configured platform;
- `SHA256SUMS`, whose signed Registry closure contains the five provider ZIP
  archives and the exact Registry metadata manifest;
- the detached OpenPGP signature for `SHA256SUMS`;
- `manifest.json` with archive and binary digests, source commit, embedded
  version evidence, and publication blockers;
- five archive-specific SPDX 2.3 documents generated from the exact Go module
  graph;
- one RFC 8785 canonical in-toto Statement v1 with SLSA Provenance v1; and
- the provenance statement's detached OpenPGP signature.

The public GitHub Release inventory is exactly 15 assets and is deliberately
broader than the signed Registry checksum manifest. Before provenance is
added, the five archives, five SPDX documents, manifest, `SHA256SUMS`, and its
signature form the exact 13-asset payload closure. The canonical provenance
statement binds all 13 by name, byte size, and SHA-256, plus the source commit,
release tag, tooling commit, workflow identity, run and attempt, request ID,
and annotated tag-object OID and SHA-256. It excludes itself and its signature
to avoid self-reference. Its detached signature MUST verify with the same
pinned provider OpenPGP key that authenticates `SHA256SUMS`; a GitHub artifact
attestation is not a substitute.

Per-archive SPDX documents and source-bound provenance remain immutable release
evidence and must not be listed in `SHA256SUMS`. The Registry metadata manifest
is not an installable package, but the public Registry ingress contract
requires its digest in the signed checksum file. Both Terraform and OpenTofu
use the same six-entry checksum target set.

Providers `v0.1.1`, `v0.1.2`, and the then-corrective `v0.1.3` describe
historical pre-v1 layouts only. `v0.1.1` included SPDX evidence in
`SHA256SUMS`; `v0.1.2` omitted the required Registry manifest; and `v0.1.3`
corrected that historical checksum closure without replacing a published byte.
None of those layouts defines the current provider `v1.0.3` release. The v1
release is governed by the 15-asset provenance closure above and MUST NOT
overwrite or inherit the identity of any historical version.

The signed annotated `v1.0.0` tag is immutable but never became a GitHub
Release or Terraform Registry version. Its candidate workflow failed before
publication because the embedded provenance generator did not parse.
`v1.0.0` is permanently abandoned: never move, delete, or reuse that tag.
Provider `v1.0.1` is the forward repair, and workflow-embedded Node programs
are now parsed in the portable gate while this provenance generator is also
executed against its exact 13-subject contract.

`release/version.json` also pins the supported CLI/FQN matrix. Release CI must
exercise Terraform `1.15.8` and OpenTofu `1.12.3` with the same canonical
identity, `registry.terraform.io/tako0614/takoform`. Both must expose the same
schema and complete lifecycle evidence for the exact embedded structural
candidate set.

Provider publication is an independent distribution action. The `v*` workflow runs
`candidate-publication-check`, which requires `publicationStatus:
candidate-only` and the unchanged structural inventory. Publishing that exact
binary, checksums, SBOM, provenance, and signatures does not change any Form's
lifecycle state, install a Form Package, establish Host Support, or grant host
activation authority. Provider SemVer has no admission-generation coupling.

The normal `matrix` command intentionally uses a locally built provider binary
through `dev_overrides`; it is a pre-publication regression gate and is not
Registry evidence. After the first authorized publication, capture the
post-publication readback with:

```console
go run ./cmd/provider-lifecycle-conformance render-registry-matrix \
  --opentofu tofu --terraform terraform \
  > /tmp/provider-lifecycle-matrix.json
go run ./cmd/provider-registry-readback \
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
`installationSource: direct-registry-install`; the historical readback validator rejects
otherwise-valid matrices carrying `local-dev-override`. The matrix is still
not self-authenticating: it becomes usable only when an externally signed,
canonical `takoform.provider-registry-readback@v1` document binds its digest,
installed binary/schema digests, CLI/FQN identities, provider tag, and source
commit.

## Historical admission evidence

Takoform no longer has a central candidate set, admission assembly workflow,
admission publisher, or set-wide promotion lane. The provider Registry
readback and the all-34 provider reports remain useful compatibility evidence,
but they do not vote on Form maturity and are not inputs to a current central
approval process.

Published admission identities are a closed historical namespace. Versions
`1.0.1`, `1.0.2`, `1.0.3`, `1.0.4`, `1.0.6`, and `1.0.7` are pinned by exact annotated
tag object, commit, retained tree, and set digest in
[`../admission/admission-identities.json`](../admission/admission-identities.json).
Version `1.0.5` is permanently `reserved-abandoned`. No new admission checkpoint
is assigned.

The history-only gates are:

```console
go run ./cmd/standard-form-conformance legacy-published-package-check
go run ./cmd/standard-form-conformance legacy-admission-evidence-check
```

They prove the immutable Legacy publication inventory and historical Git
identity ledger. They do not rerun old `portable-standard` assertions as
current conformance, support, availability, or activation claims. Provider
versions remain independent of Form versions and of these retired historical
identities.

The provider build tool never signs, uploads, creates a GitHub Release, or
publishes to a Registry/mirror. The only operator entrypoint is the repository
deploy command. First dispatch the protected signed-tag lane with the exact
descriptor tag and current protected-main commit:

```console
bun run deploy -- takoform-provider-release prepare \
  --tag v1.0.3 \
  --expected-commit <40-character-protected-main-commit>
```

The entrypoint records the workflow run and stops. Its read-only preflight job
has no protected Environment, write token, or signing key and is the only job
that executes Go, the candidate provider, or either Terraform-compatible CLI.
Only canonical descriptor/build/SBOM/provenance/lifecycle digests cross the
artifact boundary. The protected signing job starts from a fresh exact checkout,
performs only static JSON/hash/Git/Registry-absence checks, imports the
`provider-release` Environment key, and exports a checksum-closed public
signed-tag object without repository write credentials. The signed message
binds the protected-main commit, complete preflight checksum inventory, and
exact Actions run/attempt. No local human signing key is required.

After the Environment-approved run succeeds, an admin maintainer consumes that
exact signed-tag run through the same owner entrypoint:

```console
bun run deploy -- takoform-provider-release tag \
  --tag v1.0.3 \
  --expected-commit <same-40-character-commit> \
  --run-id <signed-tag-workflow-run-id> \
  --run-attempt <signed-tag-workflow-attempt>
```

The verifier closes both inventories, reconstructs the exact public tag object
with `git mktag`, checks its expected object id and peeled target, imports only
the pinned public key in a temporary keyring, verifies the signer fingerprint,
and refuses to replace an existing local tag ref. The final push uses the
maintainer's existing admin authentication to cross the restricted tag-creation
ruleset; the Actions job itself cannot bypass that rule. The entrypoint then
dispatches the prepare-only `release.yml` at that exact immutable signed tag
ref, with the same tag and peeled source commit. It never dispatches this
candidate workflow from mutable `main`; its workflow identity is exactly
`.github/workflows/release.yml@refs/tags/<tag>`.
It prints that exact run URL and stops again.
Its read-only build job verifies the signed tag with the public key, runs the
same non-publishing GoReleaser command twice, requires the five final archive
names and bytes to match after a source-mtime perturbation, extracts the exact
final Linux amd64 provider, and runs both supported CLI lifecycles against
those extracted bytes. It then validates every final Syft
document against the repository-pinned official SPDX 2.3 schema, and closes the
five provider archives plus Registry metadata manifest under the checksum
file. A fresh protected signing job executes no provider or repository Go code:
it statically rechecks the tag, inventory, Registry absence, and checksums,
imports the same Environment key, adds the detached checksum signature, builds
the RFC 8785 canonical provenance over that exact 13-subject payload, and adds
the provenance statement's detached GPG signature. It emits a same-run,
checksum-closed local-publication candidate with exactly 15 public assets.
Neither job has tag, GitHub Release, public-attestation, or publication
authority.

After that second run succeeds, publish only its exact run/attempt:

```console
bun run deploy -- takoform-provider-release publish \
  --tag v1.0.3 \
  --expected-commit <same-40-character-commit> \
  --run-id <provider-release-candidate-run-id> \
  --run-attempt <provider-release-candidate-run-attempt>
```

The owner deploy entrypoint verifies the outer checksum closure and detached
GPG signature, then uses only the operator machine's GitHub authority to create
a draft, upload the exact fifteen assets, compare their public API digests,
publish that same draft, and require an immutable exact-ID/tag readback. A
dispatch, tag push, or candidate artifact is not publication success.

The provider candidate metadata is exact recursively key-sorted, two-space
pretty JSON with exactly one trailing LF, matching the `jq -S` workflow output.
Compact JSON, a missing or extra LF, different indentation, unsorted keys, and
duplicate keys are rejected before any publication mutation.

If the exact signed annotated tag already exists locally and remotely but no
GitHub Release or Registry version exists, normal `publish` remains
intentionally unusable. Complete only that exact partial identity with:

```console
bun run deploy -- takoform-provider-release recover-tag-only \
  --tag v1.0.3 \
  --expected-release-commit <signed-tag-peeled-release-commit-E> \
  --expected-tag-object <exact-annotated-signed-tag-object> \
  --expected-recovery-commit <current-reviewed-protected-main-commit-F> \
  --run-id <original-provider-candidate-run-id> \
  --run-attempt <original-provider-candidate-run-attempt>
```

This phase requires `E` to be an ancestor of `F`, and the exact `E..F` diff may
contain only the reviewed recovery implementation, its tests, and this release
documentation. It requires current protected `main` to equal `F`; the exact
local and remote annotated tag object must still peel to `E` and verify with
the pinned provider key; the GitHub Release and Registry version must still be
absent; and the successful candidate run must have exact head `E`, branch
`v1.0.3`, run, attempt, checksums, 15 assets, GPG signatures, and provenance.
The owner gate and the same recovery fence run immediately before draft
creation and again immediately before publication. Recovery never moves,
deletes, or recreates the tag.

If that phase stops after retaining one exact draft, resume only the named
identity:

```console
bun run deploy -- takoform-provider-release recover-draft \
  --tag v1.0.3 \
  --expected-release-commit <signed-tag-peeled-release-commit-E> \
  --expected-tag-object <exact-annotated-signed-tag-object> \
  --expected-recovery-commit <current-reviewed-protected-main-commit-F> \
  --release-id <exact-retained-github-release-id> \
  --run-id <original-provider-candidate-run-id> \
  --run-attempt <original-provider-candidate-run-attempt>
```

`recover-draft` re-verifies the same candidate, tag, Registry absence, owner
gate, and `E..F` recovery fence. It accepts only the exact draft id, tag, name,
body, target commitish, upload/assets endpoints, and already-uploaded subset;
it uploads only missing assets, rereads the complete draft, repeats the fence,
and publishes that same draft. Any competing, public, unknown, duplicate, or
drifted identity fails closed without deletion or blind retry.

Both recovery phases require exclusive single-writer operator authority from
draft creation or resumption through immutable publication. GitHub's REST API
has no atomic precondition spanning release metadata and all assets, so the
strongest available closure is used: an immediate authoritative empty-draft
read after creation, a complete exact-draft reread before publication, a PATCH
that restates the full tag/name/body/target/prerelease identity, an immutable
exact Release readback and download, and one final pinned signed-tag check
before reporting `VERIFIED`.

After the immutable GitHub Release exists and the public Registry has indexed
it, dispatch the signed direct-install readback and then verify that exact run:

For the ordinary lane, protected `main` may still equal the signed tag's peeled
commit. After an exact recovery, `--expected-commit` is instead the current
reviewed protected-main source/tooling commit `F`. The immutable provider
release provenance and provider commit remain the tag's peeled commit `E`.
The readback workflow and local verifier require `E` to be an ancestor of `F`
and preserve those two bindings separately; they never relabel the released
provider bytes as having been built from `F`.

```console
bun run deploy -- takoform-provider-release readback \
  --tag v1.0.3 \
  --expected-commit <current-reviewed-protected-main-source-commit>

bun run deploy -- takoform-provider-release verify \
  --tag v1.0.3 \
  --expected-commit <same-current-reviewed-protected-main-source-commit> \
  --run-id <registry-readback-workflow-run-id> \
  --run-attempt <registry-readback-workflow-attempt>
```

The readback workflow installs the exact public version directly through both
OpenTofu and Terraform, requires one provider binary digest, signs the Registry
readback, and emits the exact six-file candidate used by admission. The verify
phase requires that workflow attempt to have completed successfully and closes
the downloaded artifact inventory and checksums; it never republishes the
provider.

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
overwritten. At that historical point the corrected six-entry candidate was
`v0.1.3`, and direct Terraform/OpenTofu Registry install evidence remained
post-publication; the coordinated Form `1.0.1` candidate therefore used
provider `0.1.3`. Those retained facts do not define the current provider
published `v1.0.1` release or its 15-asset provenance closure.

Key rotation is additive and review-gated: create a distinct repo-external key,
change the pinned fingerprint/public key in one reviewed commit, register that
public key with the Terraform Registry before tagging, and retain old public
keys for verification of historical releases. Never reuse the Takoform key for
the separately owned Takosumi legacy/admin provider.

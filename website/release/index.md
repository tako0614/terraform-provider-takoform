# Release boundaries

## Specification release

[`specification-releases.json`](specification-releases.json) is the append-only
numbered Takoform Specification ledger. Its current 1.0 object is a candidate,
not a completed release. A numbered entry may be recorded only after
[`../spec/publication-evidence.json`](../spec/publication-evidence.json) closes
one exact committed snapshot of the normative `spec/` tree. Candidate Forms,
packages, reference conformance, and Provider behavior are independent evidence
and do not block the numbered Specification.

Specification 1.0 defines Host API `forms.takoform.com/v1` but does not promote
the 31 current Experimental `0.x` FormRefs, publish their packages, or advance
the official Provider. Provider 3 is an independent non-normative sample;
Provider 2.1.1 and its Host v1beta1/15-Form identities remain immutable
Registry history. `bun run check:specification-releases` validates the ledger,
while `bun run check:specification-v1-release` intentionally fails until the
committed evidence is complete.

The exact current candidate is the versionless family set rooted at
`edge.forms.takoform.com` and the other seven groups in the current-family
index, using package envelope `packages.forms.takoform.com/v1alpha5`. Neither
identity is published by recording the Specification candidate.

## Provider release boundary

`release/version.json` is the independent Takoform provider version source. It
does not inherit a Takosumi package or release version.

The provider-specific trust lane is pinned by the release trust profile
(internal decision identifier D-08) in
[`../spec/trust/`](../spec/trust/). Form Packages use a separate keyless trust
lane and never reuse this provider GPG key; its pinned Sigstore trust root
lives at [`trust/trusted-root.json`](trust/trusted-root.json). Since
[decision 0041](../spec/decisions/0041-form-packages-publish-with-the-provider-release.md)
Form Packages have no independent release cadence — they publish with the
provider release that embeds them, when the publication blockers clear — and
only the revocation delivery lane remains a standing workflow
(`.github/workflows/form-package-revocation.yml`).

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

Provider `v2.1.1` is the current Registry-published release. Provider `v2.0.0`
is the published compatibility predecessor and Provider `v1.0.3` is the
published Legacy `v1` release. Their signed tags, immutable GitHub Releases,
and canonical Terraform Registry listings exist.

`release/version.json` describes the next provider release candidate:
`3.0.0`, tag `v3.0.0`, targeting Host API `forms.takoform.com/v1`. Its
`publicationStatus` remains `candidate-only`: a source descriptor, local gate,
candidate build, or compatibility matrix does not establish a signed GitHub
Release or Terraform Registry publication. Provider 3 can be called published
only after its own owner flow and exact Registry readback succeed.

Provider 3 projects exactly 31 Experimental `0.x` Forms from the eight current
versionless families. Their exact FormRefs, provider-owned Terraform resource
types, and definition/package digests are locked in
[`provider-form-identities.json`](provider-form-identities.json). The
`packages.forms.takoform.com/v1alpha5` candidate artifacts remain unpublished,
and publishing the provider neither publishes those packages nor changes Form
maturity. The same ledger retains Provider 2.1.1's exact 15-Form v1beta1 entry
byte-for-byte as immutable history.

Provider 3 removes the nine withdrawn v1alpha2 resource types and historical
ObjectBucket from its current surface. The explicit state and resource-type
boundary is in [`migrations/v2-to-v3.md`](migrations/v2-to-v3.md); in
particular, withdrawn Terraform type names are never reoccupied by unrelated
current Forms.

## Abandoned v2.1.0 candidate and v2.1.1 forward repair

The signed annotated `v2.1.0` tag is a permanently abandoned, unpublished
candidate. Its exact tag object is
`5ec38b5485c774650065e8317f8024ad40270393`, and it peels to source commit
`9add889446740d406a4c9ca93b74137c9b014fca`. Candidate workflow run
`31379384964` failed before artifacts because Bun was missing. No GitHub Release
or Terraform Registry version exists for `v2.1.0`; never move, delete,
recreate, or reuse that tag.

`v2.1.1` is the patch forward repair from the reviewed main commit. It carries
the exact same 15 Beta FormRefs, schema/package digests, Host API
`forms.takoform.com/v1beta1`, and family `edge.forms.takoform.com/v1beta1`;
their retained package envelope is `packages.forms.takoform.com/v1alpha4`;
only provider release identity and tooling changed. The current
`provider-form-identities.json` entry is re-keyed to `v2.1.1` without changing
any nested Form identity or digest. The exact Provider 2.1.1 Registry readback
is now retained in `release/provider-release-identities.json`; the descriptor
remains `candidate-only` metadata by design and no publication claim is made
for a future version without its own readback.

`release/provider-release-identities.json` retains the exact six-file signed
Registry readback closure for current provider releases as canonical base64.
The closure binds the provider release commit, readback tooling commit,
Terraform and OpenTofu direct installs, one provider binary digest, the
workflow certificate identity, transparency-log proof, and original checksum
manifest. The site derivation (`website/.vitepress/site-status.mjs`) reads the
retained readback entries when deriving what public documentation may call
Registry-published. This distribution evidence grants no Form maturity, Host
Support, activation, placement, or commercial authority.

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

This is the first stable provider compatibility line, not a claim that any
Form epoch graduated. The `forms.takoform.com/v1alpha1` and
`forms.takoform.com/v1alpha2` Form epochs those early releases carried were
later withdrawn while Takoform is pre-Stable
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
published provider releases that speak them remain immutable Registry history,
and their identities are recorded as retired in
[`published-document-lanes.json`](published-document-lanes.json). Provider and
Form Definition versions remain independent; current Form Package artifacts
are content-addressed. Published `EdgeWorker@1.0.0` and `EdgeWorker@1.0.1`
identities cannot be reset, which is why the withdrawn provider-neutral
definition used the independent `EdgeWorker@3.0.0` identity. The exact
contract is in [`../spec/versioning.md`](../spec/versioning.md).

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
candidate-only` and the unchanged structural inventory. For v2.1.1 the owner
gate additionally locks all 15 Terraform resource schemas, exact Beta
FormRefs/digests, fake/reference-host v1beta1 conformance, provider state
compatibility, and no-overwrite public identities. Publishing that exact
binary, checksums, SBOM, provenance, and signatures does not change any Form's
lifecycle state, install a Form Package, establish Host Support, or grant host
activation authority. Provider SemVer has no admission-generation coupling.

The normal `matrix` command of `cmd/worker-authoring-conformance`
intentionally uses a locally built provider binary through `dev_overrides`; it
is a pre-publication regression gate and is not Registry evidence. The signed
direct-install Registry readback lane that captured post-publication evidence
for `v2.1.1` was retired with the withdrawn epochs' tooling
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md));
its six-file signed closure for `v2.1.1` is retained in
`release/provider-release-identities.json` and remains the publication
evidence for that release. The next release (`3.0.0`, per
[`migrations/v2-to-v3.md`](migrations/v2-to-v3.md)) must bring its own
readback lane matched to the 31-resource surface before its publication can be
called Registry-verified; a matrix or local report is never
self-authenticating.

## Historical admission evidence

Takoform no longer has a central candidate set, admission assembly workflow,
admission publisher, or set-wide promotion lane, and the retained admission
evidence trees were withdrawn with the Legacy epoch
([decision 0042](../spec/decisions/0042-the-pre-beta-epochs-are-withdrawn.md)).
The admission identity ledger, the 34-package release inventory, and the
history-only verification subcommands that walked them are readable in this
repository's git history; the `formpackage` verifier keeps
every epoch's schemas so bytes retained in history and `forms/*` release tags
stay verifiable. Nothing derives current approval, maturity, or conformance
from that history.

The provider build tool never signs, uploads, creates a GitHub Release, or
publishes to a Registry/mirror. The only operator entrypoint is the repository
deploy command. First dispatch the protected signed-tag lane with the exact
descriptor tag and current protected-main commit:

```console
bun run deploy -- takoform-provider-release prepare \
  --tag <descriptor-tag-from-release/version.json> \
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
  --tag <descriptor-tag-from-release/version.json> \
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
  --tag <descriptor-tag-from-release/version.json> \
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
  --tag <descriptor-tag-from-release/version.json> \
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
the descriptor tag, run, attempt, checksums, 15 assets, GPG signatures, and provenance.
The owner gate and the same recovery fence run immediately before draft
creation and again immediately before publication. Recovery never moves,
deletes, or recreates the tag.

If that phase stops after retaining one exact draft, resume only the named
identity:

```console
bun run deploy -- takoform-provider-release recover-draft \
  --tag <descriptor-tag-from-release/version.json> \
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
it, capturing signed direct-install readback evidence is a release obligation
that the retired lane no longer fulfils; see the readback note above. The
`v2.1.1` closure retained in `release/provider-release-identities.json` is the
only readback evidence that exists.

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
provider `0.1.3`. Those retained facts do not define the then-current
published `v1.0.1` release or its 15-asset provenance closure.

Key rotation is additive and review-gated: create a distinct repo-external key,
change the pinned fingerprint/public key in one reviewed commit, register that
public key with the Terraform Registry before tagging, and retain old public
keys for verification of historical releases. Never reuse the Takoform key for
the separately owned Takosumi legacy/admin provider.

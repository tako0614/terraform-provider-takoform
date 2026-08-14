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

Provider `v1.0.3` is the current published `v1` release. Its signed annotated
tag resolves to commit `87b29ef58066755012a8d80bce0c8f715cf82cb9`; the
append-only provider release ledger records the exact tag object and commit.
Provider `v1.0.4` is the current unissued maintenance candidate;
`release/version.json` says `publicationStatus: candidate-only` because it is
release-descriptor metadata, not a live availability field. The append-only
ledger has no `v1.0.4` assignment.

The `maintenance/v1` branch is rooted exactly at immutable provider `v1.0.3`
and exists only for the maintained v1alpha1 provider line. Current provider-v2
development has independent types and contracts; this branch is not an
automatic backmerge source and must not import v2 resource types. Provider v1
release tooling exists only on this branch and fixes every mutable workflow,
operator, and readback ref to `maintenance/v1`; it is not a release authority
for provider v2. Form Package release and revocation tooling remains fixed to
`main`.

## Provider v1.0.4 recorded Form identity transition

Provider `v1.0.4` retains exact closed codecs and registry references for
`RelationalDatabase@2.0.0` and `EdgeWorker@3.0.0`, while fresh creates use the
current DB3 and Edge4 identities. Read, ordinary Update, and Delete dispatch
only through the exact complete Form identity persisted in state; unknown or
unavailable identities fail before a Resource host request, with no SemVer
inference.

Only `takoform_relational_database` accepts
`form_transition = "relational-database-v2-to-v3"`. The provider keeps state on
DB2 until the host proves an atomic same-resource DB3 spec/identity commit.
Every invocation performs operation readback before mutation; only the exact
operation-not-found response or an exact same-request `prepared` response with
`dispatchAttempted: false` permits one POST. Attempted, indeterminate,
incomplete, or uncertain readback is reconciled without a blind repost. Edge
artifact-only updates remain on its byte-exact historical Edge3 Definition.
The operator guide is
[`migrations/v1.0.2-to-v1.0.4-recorded-form-transition.md`](migrations/v1.0.2-to-v1.0.4-recorded-form-transition.md).

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

This is the first stable provider compatibility line, not a claim that the
portable specification has graduated from
`forms.takoform.com/v1alpha1`. Provider, Form definition, Form Package, and
admission versions remain independent. Published `EdgeWorker@1.0.0` and
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
None of those layouts defines the maintained provider-v1 release. The v1
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
`portable-v1` provider reports using
`takoform.standard-provider-runner-report@v2`, including the exact installed
provider binary digest. A conforming host signs the exact ten
`ga-core-v2` host reports plus the exact portable runner report, then signs the
candidate's exact `SHA256SUMS` envelope. That envelope closes over the manifest,
request/run/source-bound signed candidate, portable report and bundle, and all
ten report/bundle pairs; the checksum signature bundle is the only excluded
self-referential file. Takoform requires this signed-candidate v4 26-file
closure, rejects the legacy runner subject, and semantically verifies every
portable runner result field against its exact retained contract.
The admission material retains and
offline-authenticates the complete 34-report provider candidate, including the
24 reports outside the selected ten. Finally, `standard-admission-evidence.yml` verifies
those retained candidates, builds the generation-aware v4 set twice, and signs
only the ten admission-evidence subjects. It does not create a tag, release,
registry entry, or production mutation.

The portable source gate authenticates historical `ga-core-v1` publication
with `retained-ga-core-v1-published-package-check`.
`current-published-package-check` and `current-admission-closure-check` are
post-publication v4 gates and remain fail-closed until all retained subjects
and exact Git refs verify offline. The removed
`standard-admission-release.yml`,
`standard-form-package-set-release.yml`, `form-package-release` set-wide
subcommands and `--coordinated-standard-set` flag, controller promotion input,
release archive, and `release-check` path described a set-wide artifact
promotion that is not part of current Takoform.

The admission checkpoint semver is independent from each exact Form definition
and Form Package semver. It identifies one immutable source-retained closure;
it does not republish package bytes or create another distributable artifact.

After the signed admission evidence has been retained and independently
reviewed in source, the only checkpoint publication sequence is the owner
entrypoint below. It requires local `gh` and a non-empty operator `GH_TOKEN`;
that token is exposed only to read-only `gh api` ruleset calls and is scrubbed
from every `git`, `bun`, and `go` subprocess.

```console
bun run deploy -- takoform-admission-release prepare \
  --expected-commit <exact-reviewed-closure-commit>
bun run deploy -- takoform-admission-release publish \
  --expected-commit <same-exact-reviewed-closure-commit>
bun run deploy -- takoform-admission-release verify \
  --expected-commit <same-exact-reviewed-closure-commit>
```

All three phases require two exact active GitHub rulesets whose sole include is
`refs/tags/forms/admissions/v*`: a creation-only rule restricted to explicit
always-bypass actors available to the operator, and a distinct no-bypass rule
blocking update, deletion, and non-fast-forward changes. `prepare` is the
non-mutating owner gate. `publish` fingerprints the exact rulesets immediately
before the only push, creates the descriptor-pinned annotated tag only after an
independent review and remote-absence proof, and requires the identical
protection immediately afterward. A failed post-push protection proof is an
indeterminate publication result and is never retried. `verify` performs the
live-protection, authoritative tag-object, peeled-commit, v4-tree, and
offline-closure readback.

The admission tag is intentionally unsigned and has no GitHub Release:
authenticity comes from the separately Sigstore-signed retained evidence, while
the protected ref provides its immutable source identity. Version `1.0.5` is
permanently reserved and abandoned because v3 candidates already used it; the
current assignment is `1.0.7` / `forms/admissions/v1.0.7`. Never reuse, replace,
or move either identity.

The tagged closure commit may later be an ancestor of `main`, but its complete
`admission/v4` tree must stay byte-identical to the current v4 tree. Unrelated
website state may move forward. A changed v4 closure is repaired forward under
a newly assigned checkpoint version.

`admission-closure-check` also resolves the admission tag, provider tag, and
every package tag from fetched local Git refs and requires their exact retained
commits. The provider tag—not the admission tag—must be annotated and signed by
the pinned provider GPG fingerprint; import only
`release/keys/provider-signing-key.asc` before an offline local check. A
40-character string without the corresponding immutable ref is never release
evidence.

The provider build tool never signs, uploads, creates a GitHub Release, or
publishes to a Registry/mirror. The only operator entrypoint is the repository
deploy command. First dispatch the protected signed-tag lane with the exact
descriptor tag and current protected `maintenance/v1` commit:

```console
bun run deploy -- takoform-provider-release prepare \
  --tag v1.0.4 \
  --expected-commit <40-character-protected-maintenance-v1-commit>
```

The entrypoint records the workflow run and stops. Its read-only preflight job
has no protected Environment, write token, or signing key and is the only job
that executes Go, the candidate provider, or either Terraform-compatible CLI.
Only canonical descriptor/build/SBOM/provenance/lifecycle digests cross the
artifact boundary. The protected signing job starts from a fresh exact checkout,
performs only static JSON/hash/Git/Registry-absence checks, imports the
`provider-release` Environment key, and exports a checksum-closed public
signed-tag object without repository write credentials. The signed message
binds the protected `maintenance/v1` commit, complete preflight checksum
inventory, exact `maintenance/v1` workflow/source refs, and exact Actions
run/attempt. A signed artifact that substitutes `main` is rejected. No local
human signing key is required.

After the Environment-approved run succeeds, an admin maintainer consumes that
exact signed-tag run through the same owner entrypoint:

```console
bun run deploy -- takoform-provider-release tag \
  --tag v1.0.4 \
  --expected-commit <same-40-character-commit> \
  --run-id <signed-tag-workflow-run-id> \
  --run-attempt <signed-tag-workflow-attempt>
```

The verifier closes both inventories, reconstructs the exact public tag object
with `git mktag`, checks its expected object id and peeled target, imports only
the pinned public key in a temporary keyring, and verifies the signer
fingerprint. It then reconciles only four safe states: both refs absent, the
exact reconstructed local object only, the exact authoritative remote object
only, or that same exact object both locally and remotely. Local creation is a
zero-object compare-and-swap and the remote write is a create-only lease push.
An exact remote readback also closes a lost push acknowledgement. A different
object, different peeled commit, lightweight/partial remote identity, Release,
or Registry version fails closed. The maintainer's existing admin
authentication crosses the restricted tag-creation ruleset; the Actions job
itself cannot bypass that rule.

`tag` ends with `TAG_READY`. It never dispatches `release.yml`. This boundary
means a crash after the exact tag push but before any candidate request is not
an ambiguous dispatch: rerunning `tag` only re-verifies the exact local/remote
object and still performs no candidate POST.

Before candidate dispatch, the operator must allocate an owner-private
directory outside the source tree, set it to mode `0700`, and durably record a
new lowercase canonical UUIDv4. The request record target itself must not
exist.

`prepare-candidate` is intentionally unusable until two independently
reviewed, active GitHub **branch** rulesets have been installed and read back.
This source change does not create or modify either repository setting:

- `Restrict provider candidate reservation creation` includes exactly
  `refs/heads/provider-candidate-reservation-v*` and
  `refs/heads/provider-candidate-reservation-v*/**/*`, excludes nothing, has
  only the `creation` rule, and gives `always` bypass only to the exact
  `tako0614` User (`96359093`).
- `Keep provider candidate reservations immutable` has the same exact two
  includes and no excludes, has only `update` and `deletion`, and has no bypass
  actor. The current operator must read back `current_user_can_bypass: never`.

The entrypoint reads every branch-ruleset summary and detail and also asks
GitHub to evaluate the rules for both the exact prospective branch and a
descendant probe. The evaluated closure must be exactly `creation` from the
first ruleset plus `update` and `deletion` from the second, all sourced from
this repository. A missing, disabled, extra, bypassable, differently scoped,
or ineffectively matched rule halts before a local record, reservation, or
workflow POST.

After those settings exist, invoke the separate create phase exactly once:

```console
bun run deploy -- takoform-provider-release prepare-candidate \
  --tag v1.0.4 \
  --expected-release-commit <signed-tag-peeled-release-commit-E> \
  --expected-current-commit <current-protected-maintenance-v1-commit-F> \
  --expected-tag-object <exact-annotated-signed-tag-object> \
  --request-id <pre-recorded-lowercase-uuid-v4> \
  --request-record </absolute/operator-private/provider-v1.0.4-request.json>
```

The target parent must already be one physical, operator-owned `0700`
directory; symlinked parents, source-tree paths, absent parents, weak modes,
and moved paths are rejected. The entrypoint repeats the owner gate, exact
protected `maintenance/v1` check, pinned signed local/remote tag verification,
Release/Registry absence, exact UUID run-absence read, and both ruleset
evaluations. It rejects either the exact global reservation or any descendant
conflict before writing locally. It then creates the target with `O_EXCL` and
`0600`, writes recursively key-sorted canonical JSON plus one LF, fsyncs the
file and parent, and reads the exact bytes back. The v2 record binds the UUID,
release/current commit, tag and tag-object OID, repository, dispatch ref, exact
tagged workflow identity, its own path digest, deterministic reservation ref,
reservation commit `F`, and `dispatchAttempted: true`. The path is never
printed, and the record contains no GitHub token or raw API response.

In the ordinary lane `E == F`. A reviewed recovery lane may use `E` strictly
before `F`, but ancestry alone never authorizes dispatch: the entrypoint also
requires the existing narrow provider recovery fence over the exact `E..F`
diff. That diff must be nonempty, include `scripts/release-deploy.mjs`, and
contain only the allowlisted reviewed release orchestration, tests, fixture,
and documentation paths. The checkout must still be clean, attached to
`maintenance/v1`, and exactly equal to freshly fetched canonical `F`; the
signed immutable tag must still peel to `E`. Candidate source/tooling metadata
remains `E`, never `F`.

After that durable write, the entrypoint uses GitHub's Create a reference REST
operation exactly once for the flat deterministic branch
`refs/heads/provider-candidate-reservation-v1.0.4`, pointing directly at `F`.
The branch name is derived only from the release tag—never from the UUID or
record path. The flat form plus the separately protected descendant pattern
prevents a `<exact>/...` Git file/directory collision from escaping the same
creation rule. Only this invocation's exact HTTP `201` response, exact response
body, immediate exact `GET` showing a commit ref at `F`, unchanged evaluated
rules, unchanged release authority, and a final exact ref readback authorize
its one `release.yml` POST.

A pre-existing exact reservation, descendant conflict, `422`, connection loss,
non-`201` response, malformed response, or lost Create Ref acknowledgement is
never adopted—even if authoritative readback later shows the exact ref at
`F`. From the moment the remote reservation may have been created, every
invocation is reconcile-only forever. A crash after exact reservation creation
but before the workflow POST therefore strands this release request without a
candidate; that permanent pre-POST halt is accepted safety behavior and is not
repaired by deleting the branch, choosing another UUID, choosing another
private path, or relying on Actions absence. A crash after only the private
record similarly never permits that record to be retried. These fail-closed
windows trade liveness for a repository-global proof that A and fresh-clone B
cannot both POST.

If the sole workflow POST returns, `prepare-candidate` does not poll for or
adopt a run; success returns `RECONCILIATION_REQUIRED`. A nonzero client result,
process crash, connection loss, or absent visible run is never proof that
GitHub did not accept the POST. Once the record exists, create mode always
refuses it, and once the reservation exists every record path and UUID is
refused across fresh clones. Do not delete, replace, copy, or edit the record or
reservation to retry.

Reconcile only with the same exact inputs and record:

```console
bun run deploy -- takoform-provider-release reconcile-candidate \
  --tag v1.0.4 \
  --expected-release-commit <same-signed-tag-peeled-release-commit-E> \
  --expected-current-commit <same-current-protected-maintenance-v1-commit-F> \
  --expected-tag-object <exact-annotated-signed-tag-object> \
  --request-id <same-pre-recorded-lowercase-uuid-v4> \
  --request-record </same/absolute/operator-private/provider-v1.0.4-request.json>
```

`reconcile-candidate` is read-only: it never dispatches, pushes, creates a
Release, reservation, or result file. It requires the same exact active and
evaluated rulesets, stable-reads the same regular `0600` file without following
links, requires its owner/mode/inode/path/canonical bytes and all bindings to
remain exact, and requires the deterministic immutable reservation to remain
an exact commit ref at `F`. It then enumerates every `release.yml`
`workflow_dispatch` page, and accepts only one run whose UUID, workflow,
tag/head, commit, URL, and identity all match. More than one match or any
matching-run drift fails closed. No match returns `UNRESOLVED_ABSENT`; that is
an unresolved terminal observation, not authorization for another POST.
Exactly one match returns its run id, attempt, and URL as `AWAITING_REVIEW`.
Cancel that exact run before Environment approval if any input or evidence is
wrong; never select a latest run.

The immutable dispatch ref is the signed tag and its workflow identity is
exactly `.github/workflows/release.yml@refs/tags/<tag>`.
The tagged workflow fetches the fixed `maintenance/v1` ref and requires its
peeled tag commit to be an ancestor of the current canonical branch. Its
`sourceCommit` and `toolingCommit` metadata must both equal that peeled commit;
current branch tooling is never relabeled as release-source evidence.
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
  --tag v1.0.4 \
  --expected-commit <same-40-character-commit> \
  --run-id <provider-release-candidate-run-id> \
  --run-attempt <provider-release-candidate-run-attempt>
```

The owner deploy entrypoint verifies the outer checksum closure and detached
GPG signature, then uses only the operator machine's GitHub authority to create
a draft, upload the exact fifteen assets, compare their public API digests,
publish that same draft, and require an immutable exact-ID/tag readback. The
mutation fence reads the active repository immutable-release setting and
requires `enabled: true` immediately before the draft POST and again
immediately before the public PATCH. Both reads also repeat the owner gate,
Registry absence, exact Release identity, and pinned signed local/remote tag
object, peeled commit, and signature verification. After the immutable Release
readback and fresh exact-asset download, one final pinned signed-tag readback
must pass before the entrypoint reports `VERIFIED`. A dispatch, tag push, or
candidate artifact is not publication success.

The provider candidate metadata is exact recursively key-sorted, two-space
pretty JSON with exactly one trailing LF, matching the `jq -S` workflow output.
Compact JSON, a missing or extra LF, different indentation, unsorted keys, and
duplicate keys are rejected before any publication mutation.

An exact signed annotated tag without a candidate run is no longer a
publication-recovery condition: use `prepare-candidate` and
`reconcile-candidate`. In the ordinary `E == F` lane, do not create an
artificial nonempty `E..F` recovery commit merely to dispatch `E`; the empty
diff naturally bypasses the recovery-only fence. `recover-tag-only` is
reserved for completing publication after one exact candidate run already
exists and the reviewed protected-maintenance recovery boundary genuinely
uses a distinct `F`. Complete only that exact partial identity with:

```console
bun run deploy -- takoform-provider-release recover-tag-only \
  --tag v1.0.4 \
  --expected-release-commit <signed-tag-peeled-release-commit-E> \
  --expected-tag-object <exact-annotated-signed-tag-object> \
  --expected-recovery-commit <current-reviewed-protected-maintenance-v1-commit-F> \
  --run-id <original-provider-candidate-run-id> \
  --run-attempt <original-provider-candidate-run-attempt>
```

This phase requires `E` to be an ancestor of `F`, and the exact `E..F` diff may
contain only the reviewed recovery implementation, its tests, and this release
documentation. It requires current protected `maintenance/v1` to equal `F`;
the exact local and remote annotated tag object must still peel to `E` and
verify with the pinned provider key; the GitHub Release and Registry version
must still be absent; and the successful candidate run must have exact head
`E`, branch `v1.0.4`, run, attempt, checksums, 15 assets, GPG signatures, and
provenance.
The owner gate, active `enabled: true` immutable-release repository setting,
and the same recovery fence run immediately before draft creation and again
immediately before publication. Recovery never moves, deletes, or recreates
the tag.

If that phase stops after retaining one exact draft, resume only the named
identity:

```console
bun run deploy -- takoform-provider-release recover-draft \
  --tag v1.0.4 \
  --expected-release-commit <signed-tag-peeled-release-commit-E> \
  --expected-tag-object <exact-annotated-signed-tag-object> \
  --expected-recovery-commit <current-reviewed-protected-maintenance-v1-commit-F> \
  --release-id <exact-retained-github-release-id> \
  --run-id <original-provider-candidate-run-id> \
  --run-attempt <original-provider-candidate-run-attempt>
```

`recover-draft` re-verifies the same candidate, tag, Registry absence, owner
gate, and `E..F` recovery fence. It accepts only the exact draft id, tag, name,
body, target commitish, upload/assets endpoints, and already-uploaded subset;
it uploads only missing assets, rereads the complete draft, repeats the fence,
and publishes that same draft. The retained release target must remain exactly
`maintenance/v1`. Any competing, public, unknown, duplicate, or drifted
identity fails closed without deletion or blind retry.

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

For the ordinary lane, protected `maintenance/v1` may still equal the signed
tag's peeled commit. After an exact recovery, `--expected-commit` is instead the current
reviewed protected-maintenance-v1 source/tooling commit `F`. The immutable provider
release provenance and provider commit remain the tag's peeled commit `E`.
The readback workflow and local verifier require `E` to be an ancestor of `F`
and preserve those two bindings separately; they never relabel the released
provider bytes as having been built from `F`.

```console
bun run deploy -- takoform-provider-release readback \
  --tag v1.0.4 \
  --expected-commit <current-reviewed-protected-maintenance-v1-source-commit>

bun run deploy -- takoform-provider-release verify \
  --tag v1.0.4 \
  --expected-commit <same-current-reviewed-protected-maintenance-v1-source-commit> \
  --run-id <registry-readback-workflow-run-id> \
  --run-attempt <registry-readback-workflow-attempt>
```

The readback workflow installs the exact public version directly through both
OpenTofu and Terraform, requires one provider binary digest, signs the Registry
readback, and emits the exact six-file candidate used by admission. The verify
phase accepts only the exact certificate identity
`.github/workflows/provider-registry-readback.yml@refs/heads/maintenance/v1`,
requires that workflow attempt to have completed successfully, and closes the
downloaded artifact inventory, checksums, and equal Terraform/OpenTofu provider
binary digest. A `main` identity, source drift, or CLI digest mismatch fails
closed; the verifier never republishes the provider.

Repository configuration is part of the trust boundary, not a claim made by
this tree. The workflow references the `provider-release` GitHub Environment,
but publication remains blocked until maintainers verify required reviewers on
that Environment plus protected `maintenance/v1` and restricted `v*` tag
creation rules. Historical retained admission material continues to verify its
already-issued `provider-registry-readback.yml@refs/heads/main` identity; this
maintenance release lane does not rewrite that immutable evidence.

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

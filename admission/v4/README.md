# Standard admission v4 candidate lane

`admission/v4` is the fail-closed successor lane for the exact mixed-version
`ga-core-v2` ten-Form subset selected by
`forms/admission-candidate-set.json`.

The generation replaces `HttpService@1.0.0` with the provider-neutral
`EdgeWorker@3.0.0` and selects successor majors where the clean common
resource contract would otherwise rewrite an existing Form release-source
identity. Artifact-backed Forms use new majors because their persisted URL
grammar now forbids userinfo, query, and fragment.
It does not rewrite the immutable `ga-core-v1` package publication snapshot
under `admission/v3`.

This directory intentionally contains only reviewed trust and conforming-host
policy before publication. After the all-34 portable Form Package publication
plan and provider `v1.0.1` are complete, an operator stages immutable readbacks
for the exact selected-ten `ga-core-v2` admission subset plus the signed
all-34 provider, selected-ten host, and Registry candidates outside the
repository. The publication plan and the admission subset are distinct: v4
authenticates the all-34 provider closure, but admits only the selected ten.
Those exact bytes are reviewed and retained by a source commit; workflow
artifacts do not write this directory.

The accepted Takosumi host candidate is the signed-candidate v4 closure, not a
bag of independently signed reports. Its `SHA256SUMS` lists exactly the
manifest, signed candidate, portable runner report and bundle, and all ten host
reports and bundles; `SHA256SUMS.sigstore.json` authenticates that 24-subject
inventory under the reviewed host workflow identity. Takoform rejects the old
runner subject, semantically verifies the complete portable runner report
against the exact retained contract/input digest, and requires the resulting
26-file candidate inventory. This prevents valid old leaf signatures from
being repackaged under a new request, run, source, or aggregate.

`standard-admission-evidence.yml` accepts only the retained commit, tree, path,
run, and digest bindings. It emits a non-published signed evidence candidate.
A subsequent reviewed source commit retains that output and the
`forms/admissions/v<checkpoint>` tag identifies the exact closure commit. The
tag is an offline identity fence, not a set-wide release or controller
promotion. Until the complete retained closure exists,
`current-published-package-check` and `current-admission-closure-check` fail
closed.

The only publication entrypoint for that final identity is the owning
repository. It requires local `gh` plus a non-empty operator `GH_TOKEN`.
The token is passed only to read-only `gh api` ruleset calls and is removed from
every `git`, `bun`, and `go` subprocess environment.

```console
bun run deploy -- takoform-admission-release prepare \
  --expected-commit <exact-reviewed-closure-commit>

bun run deploy -- takoform-admission-release publish \
  --expected-commit <same-exact-reviewed-closure-commit>

bun run deploy -- takoform-admission-release verify \
  --expected-commit <same-exact-reviewed-closure-commit>
```

Every phase reads the live GitHub ruleset details and requires exactly two
active repository rulesets whose sole include is
`refs/tags/forms/admissions/v*`: one creation-only rule with explicit
always-bypass actors available to the operator, and one no-bypass rule that
blocks update, deletion, and non-fast-forward changes. `prepare` runs the
complete owner and retained-material gates without creating a tag. `publish`
refuses any existing local or remote current identity, rechecks and fingerprints
those rulesets immediately before the only push, creates one annotated tag
through a remote-absent lease, and requires the same protection immediately
afterward before authoritative object/peeled-commit plus offline closure
readback. A changed or unreadable post-push ruleset is an indeterminate
publication failure and must never be retried blindly. `verify` repeats the
live-protection and closure readback without mutation.

The tag is intentionally unsigned: authenticity comes from the role-separated
Sigstore-authenticated retained subjects and exact closure verification, while
the protected immutable tag is only their append-only source identity. No
GitHub Release is created.

Version `1.0.5` is permanently `reserved-abandoned`: retained v3 candidate
paths and its signed Actions candidate already used that checkpoint identity,
even though no remote admission tag was minted. It must never be reused.
The current assigned checkpoint is the descriptor-pinned `1.0.7` /
`forms/admissions/v1.0.7`; `admission/admission-identities.json` records only
assignment state and is not proof of live publication.

The closure commit may later become an ancestor of `main`. This is safe only
while the complete `admission/v4` tree at `main` remains byte-identical to the
tagged tree. Website or other non-v4 state may move forward independently.
Never rewrite or replace the tag; if retained v4 bytes need to change, assign a
new checkpoint version and repair forward.

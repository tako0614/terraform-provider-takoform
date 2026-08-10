# Beta and provider release policy

The current portable contracts are the Host API
`forms.takoform.com/v1beta1` and the Edge Platform Family
`edge.forms.takoform.com/v1beta1`. The API and family are public **Beta**
channels. The 15 Form Definitions remain **Experimental**, each at
`definitionVersion: 0.1.0`; Beta does not make a Form Stable.

Provider versioning is independent. Provider `v2.1.1` is a stable SemVer
release candidate targeting the Beta Host API. Its descriptor remains
`candidate-only` until the owning deploy actually publishes it. This is not a
provider prerelease and it is not a claim that any hosted service, Takosumi
Cloud, or Form Package is GA.

## Provider-first release is allowed

Open records in
[`publication-blockers.json`](publication-blockers.json) do **not** block the
provider v2.1 release. The provider release gate instead locks all of these
facts:

- exactly 15 Terraform resource schemas and resource type names;
- exactly 15 `edge.forms.takoform.com/v1beta1` FormRefs, Definition digests,
  and package digests in
  [`release/provider-form-identities.json`](../release/provider-form-identities.json);
- fake-host and reference-host conformance against
  `forms.takoform.com/v1beta1`;
- exact state compatibility: read, update, import, and delete dispatch on the
  FormRef recorded in state;
- Beta state remains readable after a future `edge.forms.takoform.com/v1`
  create default is added; refresh never upgrades it implicitly;
- append-only public schema identities and no overwrite of retained
  `v1alpha3` bytes.

The provider identity ledger is independent of package artifact publication.
Once provider v2.1 embeds a Beta FormRef, Definition digest, and package digest,
that tuple is immutable even while the corresponding Form Package remains
unpublished. A correction that breaks any Beta contract mints a new
`v1beta2` Host/family identity and new FormRef; it never edits the Beta 1
bytes.

## What remains blocked

While any obligation in `publication-blockers.json` is open, this repository
must not:

- publish the Beta Form Packages;
- independently publish the Interface or Binding Definitions;
- claim a production public Host API service is ready;
- call the family Forms Stable or claim Takosumi/Takosumi Cloud GA;
- claim independent-host, real-backend, or third-party ecosystem
  interoperability without the evidence the obligation names.

`bun run assert:publishable` is intentionally this stricter Form
Package/public-service assertion. It is not part of the provider-first release
path. Closing an obligation still requires real evidence; a passing local gate,
an empty evidence array, or an invented path cannot close one.

Independent host implementations, real backend behavior, hosted operations,
and third-party ecosystem use are later Stable/GA qualification obligations
owned by Takoform's lifecycle authority. They remain open until measured. A
Takosumi deployment may be one independent adopter, but it is not a maturity
or publication authority. The local reference host proves contract coherence
only.

## Existing immutable history

Publication is per identity, not per product lane.

- Every schema identity in
  [`release/public-schema-identities.json`](../release/public-schema-identities.json)
  is append-only. Published `v1alpha3` schema bytes stay served unchanged;
  Beta schemas use new `v1beta1` URLs.
- Provider `v1.0.3` and `v2.0.0`, their tags, and their persisted state
  contracts remain immutable.
- The 34 Legacy Form Packages and retained admission evidence remain
  byte-for-byte history.
- The old `edge.forms.takoform.com/v1alpha1` candidate tree is retained as
  source history. The Beta family was minted in a new directory and identity.

Only contracts that have satisfied the recorded qualification obligations may
be promoted to stable `v1`/`1.0.0` identities by a Takoform lifecycle
decision. Promotion adds identities and create defaults; it does not rewrite
or auto-migrate Beta state.

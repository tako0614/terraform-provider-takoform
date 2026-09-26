# Provider documentation site source

This directory retains the Provider's VitePress source, projections, and
compatibility build output. It is not the current `takoform.com` publication
source. The live site is owned by the `takoform` repository and published
through its `takoform-site` Pages surface; that site serves the Core and
publisher-neutral routes described in [its site contract](https://github.com/tako0614/takoform/blob/main/docs/site.md).

The Provider, Form Packages, and provider-neutral `EdgeWorker` resource do not
require a website runtime. Canonical Provider reference pages live under
[`../docs/`](../docs/) and examples under [`../examples/`](../examples/).
`scripts/sync-website-docs.mjs` projects the generated resource pages and
examples into this directory for local documentation checks and retained
compatibility output.

## Build

The VitePress root is this directory. Source pages are the bilingual Markdown
under `website/` (English at `/`, Japanese at `/ja/`), the theme is
`website/.vitepress/theme/`, and static passthrough assets — the normative
schemas, `tako.png`, and `robots.txt` — live in `website/static/`:

```console
bun install            # pinned lock
bun run website:build  # vitepress build -> website/public (the committed output)
bun run website:dev    # local preview
```

The writer is explicit; portable checks never rewrite the worktree.

`website/public/` is the **committed compatibility build output**. It is kept
for local snapshot and historical continuity checks; it is not an active
production upload. `bun run check:website-snapshot` proves it is not stale by
building the committed source fresh (in a throwaway directory under the
repository) and comparing the whole tree, not only the pages:

| Path class | Comparison |
| --- | --- |
| `*.html` | semantic content, with scripts, styles and tags stripped |
| `hashmap.json` | same page set; every value names a committed asset that exists |
| `assets/**` | same set of hash-stripped names, plus byte equality wherever the fresh build reproduces the exact hashed name |
| everything else | **byte equality** — the site status document, `sitemap.xml`, `robots.txt`, `tako.png`, `vp-icons.css` and every retained mirrored spec, Form Package, release and conformance file |

Outside `assets/` the file set must match exactly, so neither an extra
published file nor a deleted one passes. HTML and `assets/**` cannot be
compared byte-for-byte because VitePress/Vue scoped-style hashes and Rollup
chunk names follow the absolute build path; that is the whole of what this
gate cannot see, and everything production serves outside those two classes is
compared exactly. The same gate runs again inside the deploy pipeline from a
managed install home outside the frozen archive.

## Pages

The landing, docs, and specification pages are bilingual through VitePress
i18n: English at `/`, Japanese at `/ja/`. Canonical provider reference and
normative specification text remain the Markdown files in the repository; the
public pages link to those files instead of silently redefining their
contracts.

The canonical provider reference `docs/index.md` is projected to
`website/docs/reference.md` and published as `/docs/reference.html`. The
`/docs/` index itself (`website/docs/index.md`) is a separate hand-written
quick-start page, not a projection of `docs/index.md`.

[`public/schemas/`](public/schemas/) is generated from the normative
[`../spec/schemas/`](../spec/schemas/) set. Run
`bun run sync:public-schemas` from the repository root after a normative schema
change; the writer fills `website/static/schemas/`, and the VitePress build
copies it verbatim to the committed output. The public-surface gate requires
every schema `$id` route to exist in the committed output and be byte-identical.
Never edit the public copies directly.

## Local preview

```console
bun run website:dev
```

Preview `/`, `/docs/`, `/spec/`, `/ja/`, and at least one `/schemas/...json`
URL. A local preview does not publish anything.

## Website publication status

This repository no longer publishes `takoform.com`. `bun scripts/deploy.mjs --contract` exposes only:

- `takoform-provider-release`
- `takoform-form-package-release`

The historical `takoform-website` surface is disabled by
`release/specification-schema-authority-tombstone.json` and moved to the
`takoform` repository's `takoform-site` Pages surface. The former command
`bun run deploy -- takoform-website` and the old Worker, Cloudflare
account/zone, custom-domain, schema-origin, and recovery instructions are
retained only as historical context; do not execute them from this repository.

For the current site preview and publication procedure, use the
[`takoform` site contract](https://github.com/tako0614/takoform/blob/main/docs/site.md).
This repository's `website/` tree remains useful for local VitePress previews,
projection checks, and compatibility documentation; `website/wrangler.jsonc`
and `website/public/` are not active deployment inputs.

The repository-wide `bun run check` is required handoff evidence. Its command
graph runs `check:public-surfaces` as part of the check, and that stage includes
both `check:website-snapshot` and `check:public-snapshot`. These public-surface
and Provider checks therefore remain required for a site documentation
correction; they are not a separate, optional lane.

## Content policy
## Content policy

The site must claim nothing beyond signed, committed evidence in this
repository. The current public truth is: Core/API `v1.0.1` is published by the
external [Takoform Core release](https://github.com/tako0614/takoform/releases/tag/v1.0.1)
on `/v1`; Provider `v4.0.0` is the current published, Registry
readback-verified typed distribution selecting only the 17 tako0614 Edge Forms;
Provider `v3.0.0` remains the published, Registry readback-verified typed
distribution retaining the former 31-resource aggregate across eight families. Provider
`v2.1.1` remains immutable retained Host API v1beta1 history; Provider `v2.0.0`
is the published compatibility predecessor; Provider `v1.0.3` is the published
Legacy client; and the standalone [`takoform-forms`](https://github.com/tako0614/takoform-forms)
source publishes 17 Edge content-addressed packages from its source tags. Host
implementation, support, deployment, and adoption remain separate facts. The
historical Specification 1.1 receipt is retained by the append-only ledger and
is not a current API/version axis (Specification 1.0 was withdrawn before
publication and may not be reused); and the 34
published Form Package identities are immutable Legacy evidence. There is no
current central Takoform approval or admission. The retained Provider 3
compatibility projection maps eight versionless families and 31 exact
Experimental `0.x` FormRefs. The published Provider 4 maps only the
tako0614 Edge source's 17 Forms: the 16 the retained Provider 3 projection
carries plus `ObjectBucket`, with `edge.objects` and
`module-worker.object-bucket`. This repository does not
assert any host's live catalog. The last published
historical admission identity is `forms/admissions/v1.0.7`; its exact Git and
set identities remain pinned as Legacy evidence.
The frozen Legacy FormRef group is `forms.takoform.com/v1alpha1`. Retained
provider-v2 Form Package indexes use `packages.forms.takoform.com/v1alpha3`;
retained Provider 2.1.1/v1beta1 packages use
`packages.forms.takoform.com/v1alpha4`; current versionless-group packages use
`packages.forms.takoform.com/v1alpha5` and are published independently of the
Provider release. Published v1alpha1/v1alpha2 package indexes remain immutable
Legacy evidence.

`release/version.json` is the Provider `v4.0.0` release descriptor and keeps
`publicationStatus: candidate-only`; the descriptor must not be presented as
live publication state. `release/candidates/provider-v4.0.0.json` retains the
byte-identical candidate record and `release/history/provider-v3.0.0.json`
retains the Provider 3 writer input. The append-only release identity ledger
independently establishes `v4.0.0` as the current Registry-published provider
and retains `v3.0.0` and `v2.1.1` history. Provider releases remain non-normative and cannot close or
block the historical Specification 1.1 receipt. See
[`../release/README.md`](../release/README.md), [`../spec/README.md`](../spec/README.md),
and the repository [`AGENTS.md`](../AGENTS.md).

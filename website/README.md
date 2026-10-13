# Retained Provider website projection

This VitePress tree is retained for generated Provider documentation snapshots,
compatibility checks, and local preview. It is **not** the source of the current
`takoform.com` site, and this repository has no active website deploy surface.
The old `takoform-website` selector is disabled by the active authority
tombstone; do not use the retained Cloudflare Worker configuration or deploy
runbook to publish this tree.

The current public entrypoint for this Provider is the repository
[README](../README.md), followed by the [getting-started guide](../docs/getting-started.md)
and [resource reference](../docs/index.md). The API and common-model-only
[`takoform.com` site](https://takoform.com/) belongs to
[`takoform`](https://github.com/tako0614/takoform); individual Edge Form pages
belong to [`takoform-forms`](https://github.com/tako0614/takoform-forms).
Neither site publishes this Provider's resource reference. A future
Provider-specific public site would need its own explicitly owned deploy
surface and operator-selected routing; this retained tree grants neither.

## Local build and preview

The VitePress root is `website/`. Canonical Provider reference content lives
under [`docs/`](../docs/); generated copies under `website/` are projections,
not another documentation authority. The root package scripts still support
local development and snapshot verification:

```console
bun install --frozen-lockfile
bun run website:dev
bun run check:website-snapshot
```

`bun run website:build` is an explicit writer of the committed
`website/public/` snapshot. Local builds and previews do not publish a site.
The snapshot gate checks that retained generated pages and assets remain in
sync with source; it is not publication or a deploy authorization. Keep
Provider usage guidance in the canonical `README.md` and `docs/` tree.

The retained source and build output contain historical Host API, schema,
Form, and Provider pages from the predecessor combined site. Their presence
does not transfer those authorities back to this repository. See
[`AGENTS.md`](../AGENTS.md) and the
[authority tombstone](../release/specification-schema-authority-tombstone.json)
for the current boundary.

## Edit and regenerate

| Content | Edit here | Projection writer |
| --- | --- | --- |
| Provider reference | `docs/index.md` | `bun run sync:website-spec` → `website/docs/reference.md` |
| Getting started and v5 notes | `docs/*.md`, `docs/ja/getting-started.md` | `bun run sync:website-docs` → matching site paths (the Japanese guide uses `website/ja/docs/`) |
| Resource pages and examples | Provider-owned catalog renderer | Regenerate the catalog, then `bun run sync:website-docs` |
| Retained spec and proposal routes | Frozen source/history | `bun run sync:website-spec` renders non-authoritative stubs |
| Landing pages, navigation, glossary, versions | Curated `website/` Markdown and `.vitepress/config.mts` | `bun run website:build` |

Do not hand-edit generated copies or `website/public/`. After source edits,
run the projection writers, then `bun run website:build` and `bun run check`.
The snapshot check builds in temporary directories and does not rewrite source.
The build writer preserves the frozen public page and its asset dependencies
listed in `scripts/frozen-public-identities.mjs`.

Published v4.1.0 guidance and source-v5 evaluation have separate navigation.
The seventeen generated resource pages remain the v4 / Host API v1 reference;
the v5 consumer reference describes the default source-built plugin's thirteen
exact Form URL mappings. Local projection changes do not publish either a
Provider version or a documentation site.

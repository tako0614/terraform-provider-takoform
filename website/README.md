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
bun install
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

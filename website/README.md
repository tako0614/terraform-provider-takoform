# takoform.com website

Static landing site for the Takoform project, served as a Cloudflare Worker
with static assets. No build step and no server-side code: everything under
[`public/`](public/) is deployed as-is.

The site is bilingual on a single URL: [`public/index.html`](public/index.html)
contains both an English and a Japanese copy of the page (two `.l10n` blocks),
and a small inline head script picks one automatically from
`localStorage["takoform-lang"]` or `navigator.language`. The header/footer
language buttons flip and persist the choice; without JavaScript, English stays
visible. `/ja/` is only a legacy redirect stub to `/`. Keep the two `.l10n`
blocks' section structure and claims in sync when editing either one.

## Local preview

```console
cd website
npx wrangler dev
```

## Deploy

Production deployment is intentionally blocked until the ecosystem release
controller registers a fixed `takoform-website` adapter. Do not run
`wrangler deploy` directly or infer publication from this source directory.
Local preview does not satisfy that release gate.

`wrangler.jsonc` attaches the `takoform.com` and `www.takoform.com` custom
domains; the zone must exist in the deploying Cloudflare account.

## Content policy

The site must claim nothing beyond signed, committed evidence in this
repository. In particular it must not state that a Form Package has been
published, or that the provider is installable from the Terraform Registry or
OpenTofu Registry, until the corresponding live evidence exists (see
[`../release/README.md`](../release/README.md) and the repository `AGENTS.md`).

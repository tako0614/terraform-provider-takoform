# takoform.com website

Static public site for the Takoform project, served as a Cloudflare Worker with
static assets. There is no build step or server-side application: everything
under [`public/`](public/) is deployed as-is.

The landing page is bilingual on one URL.
[`public/index.html`](public/index.html) contains English and Japanese `.l10n`
blocks, and its small inline script selects a language from
`localStorage["takoform-lang"]` or `navigator.language`. Without JavaScript,
English remains visible. [`public/ja/index.html`](public/ja/index.html) is only
a legacy redirect to `/`. Keep the English and Japanese section structure,
claims, examples, and status text in sync.

[`public/docs/index.html`](public/docs/index.html) and
[`public/spec/index.html`](public/spec/index.html) are lightweight public
navigation hubs. Canonical provider reference and normative specification text
remain the Markdown files in the repository; the hubs must link to those files
instead of duplicating or silently redefining their contracts.

[`public/schemas/`](public/schemas/) is generated from the normative
[`../spec/schemas/`](../spec/schemas/) set. Run
`bun run sync:public-schemas` from the repository root after a normative schema
change; the public-surface gate requires every `$id` route to exist and be
byte-identical. Never edit the public copies directly.

## Local preview

```console
cd website
npx wrangler dev
```

Preview `/`, `/docs/`, `/spec/`, and at least one `/schemas/...json` URL. A
local preview does not publish anything.

## Deploy

Production is deployed only through the owning repository's deploy entrypoint:

```console
bun run deploy -- takoform-website
```

Run it from the repository root. The entrypoint runs the narrow
`bun run check:public-surfaces` gate for the bytes and claims this static site
publishes, then performs the credential scan, provenance recording, Cloudflare
publication, production readback, and reversal bookkeeping. Do not run
`wrangler deploy` directly; Wrangler is an implementation detail of the
owner-controlled entrypoint, not independent release authority.

The repository-wide `bun run check` remains required handoff evidence for a
source change. It is deliberately separate from website publication: its Go,
gofmt, and OpenTofu checks do not validate the static bytes under `public/` and
must not block a site-only correction for an unrelated provider failure.

The seven normative schema `$id` URLs are immutable published identities.
Immediately before deployment, the entrypoint fetches every existing URL and
requires its served bytes to equal the candidate exactly. A changed body,
redirect, non-200 response, transport error, or partially existing origin
blocks publication; an existing `$id` is never repaired in place.

The first deployment currently also creates the schema origin. It is allowed
only when every schema URL fails specifically with DNS `ENOTFOUND` and the
operator explicitly acknowledges that one-time mint:

```console
bun run deploy -- takoform-website --acknowledge-initial-schema-origin-mint
```

That acknowledgement is not a force flag. The entrypoint rejects it as soon as
any schema URL resolves, and it never bypasses differing bytes, HTTP 404, a
redirect, timeout, connection failure, or a partially existing origin. After
the first mint, use the ordinary deploy command. A rollback is permitted only
to a version proven to retain all already-minted schema bytes; an initial-mint
failure requires authoritative readback and forward repair.

[`wrangler.jsonc`](wrangler.jsonc) attaches the `takoform.com` and
`www.takoform.com` custom domains. It also attaches
`forms.takoform.com` to publish the normative schema `$id` URLs. This hostname
is a specification identity and static schema origin, not a central Takoform
Host API: each actual Host advertises its own versioned, same-origin lifecycle
endpoints through discovery. The zone and deployment credentials belong to the
operator and are never committed here.

## Content policy

The site must claim nothing beyond signed, committed evidence in this
repository. In particular it must not state that a Form Package is admitted,
that a candidate Form is `portable-standard`, or that provider `v1.0.0` is
installable from a Registry until the corresponding live evidence exists. See
[`../release/README.md`](../release/README.md), [`../spec/README.md`](../spec/README.md),
and the repository [`AGENTS.md`](../AGENTS.md).

# takoform.com website

Static public site for the Takoform project, currently served through a
Cloudflare Worker's static-asset support. Cloudflare is used only to host
`takoform.com` and the immutable public schema URLs; the Takoform provider,
Service Form API contract, Form Packages, and provider-neutral `EdgeWorker`
resource do not require Cloudflare. There is no build step or server-side
application: everything under [`public/`](public/) is deployed as-is.

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
TAKOFORM_CLOUDFLARE_ACCOUNT_ID=<32-lowercase-hex-account-id> \
TAKOFORM_CLOUDFLARE_ZONE_ID=<32-lowercase-hex-zone-id> \
bun run deploy -- takoform-website \
  --acknowledge-exclusive-cloudflare-writer
```

Run it from the repository root. The two IDs are operator-realized identity,
not secrets, but are still not committed because they select production
authority. The acknowledgement asserts that protected `main`, the Worker
deployment, and its custom domains have one writer for the complete attempt.
It is not a force flag and does not replace any check.

The entrypoint rejects ambient Cloudflare/Wrangler credentials and runtime
overrides, then binds the local Wrangler OAuth profile to those exact IDs. It
freezes the exact protected-main commit twice: a Git-metadata-free archive for
the public bytes and an independent non-local, detached clone for offline Git
authority. The clone has no remote or object alternates. The retained
publication/admission checks run only in that clone; the static website,
schema, copy, and deploy-safety checks run only in the archive. Both roots are
clean, exact-commit checked, and re-hashed after validation and before every
writer. Only the archive is used for the credential scan, digest manifest, and
upload, and every archive byte is verified against the commit's Git blobs.
Ignored and untracked files below a publication path are rejected. Wrangler is
installed from the exact committed `bun.lock`; a PATH-provided Wrangler is
never used.

Publication is staged: `versions upload --strict` creates a non-public version,
the source/deployment/domain fences and whole-tree digest are checked again,
and only that version is then deployed at 100%. Custom domains are changed
separately through a no-override API request. This avoids Wrangler's
non-interactive custom-domain override behavior. Do not run `wrangler deploy`,
`wrangler versions deploy`, or a domain API request directly.

The repository-wide `bun run check` remains required handoff evidence for a
source change. It is deliberately separate from website publication: its Go,
gofmt, and OpenTofu checks do not validate the static bytes under `public/` and
must not block a site-only correction for an unrelated provider failure.

The seven normative schema `$id` URLs are immutable published identities.
Immediately before deployment, the entrypoint fetches every existing URL and
requires its served bytes to equal the candidate exactly. A changed body,
redirect, non-200 response, transport error, or partially existing origin
blocks publication; an existing `$id` is never repaired in place.

If the schema origin has never been minted, its first deployment is allowed
only when every schema URL fails specifically with DNS `ENOTFOUND` and the
operator explicitly acknowledges that one-time mint:

```console
TAKOFORM_CLOUDFLARE_ACCOUNT_ID=<account-id> \
TAKOFORM_CLOUDFLARE_ZONE_ID=<zone-id> \
bun run deploy -- takoform-website \
  --acknowledge-exclusive-cloudflare-writer \
  --acknowledge-initial-schema-origin-mint
```

That acknowledgement is not a force flag. Outside the ID-bound recovery lane,
the entrypoint rejects it as soon as any schema URL resolves, and it never
bypasses differing bytes, HTTP 404, a
redirect, timeout, connection failure, or a partially existing origin. After
the first mint, use the ordinary deploy command. Before creating the hostname,
the entrypoint proves the exact no-conflict Cloudflare changeset, Cloudflare
zone/delegation, ENOTFOUND from every authoritative nameserver, and all seven
candidate schema bytes through the already-routed apex Worker. It then writes
the full three-domain closure with both origin and DNS override flags disabled.

If the version is current but the initial domain write or readback becomes
indeterminate, do not repeat the normal deploy. Use the exact deployment and
version IDs printed by the failed attempt:

```console
TAKOFORM_CLOUDFLARE_ACCOUNT_ID=<account-id> \
TAKOFORM_CLOUDFLARE_ZONE_ID=<zone-id> \
bun run deploy -- takoform-website \
  --acknowledge-exclusive-cloudflare-writer \
  --acknowledge-initial-schema-origin-mint \
  --recover-initial-schema-domain \
  --expected-deployment=<deployment-uuid> \
  --expected-version=<version-uuid>
```

Recovery uploads and deploys no Worker version. It requires current production
to equal both IDs, requires that version's committed message and static-only
resource closure, and checks all seven candidate schema bytes through the apex.
It creates the domain only from a still-safe absent changeset; if the exact
domain is already attached it performs readback only. Any competing state
blocks recovery.

A rollback is permitted only to a version proven to retain all already-minted
schema bytes. The postreadback covers apex and `www` roots, docs, spec,
sitemap, static assets, the custom 404 response, all seven schema identities,
and the exact three-domain control-plane closure.

[`wrangler.jsonc`](wrangler.jsonc) attaches the `takoform.com` and
`www.takoform.com` custom domains. It also attaches
`forms.takoform.com` to publish the normative schema `$id` URLs. This hostname
is a specification identity and static schema origin, not a central Takoform
Host API: each actual Host advertises its own versioned, same-origin lifecycle
endpoints through discovery. The zone and deployment credentials belong to the
operator and are never committed here.

## Content policy

The site must claim nothing beyond signed, committed evidence in this
repository. The current public truth is: provider `v1.0.1` is published and
Registry-verified; all 34 current Form Packages are published and immutable;
`forms/admissions/v1.0.6` admits exactly 10 as `portable-standard`; and the
remaining 24 are published but not admitted. The API nevertheless remains
`forms.takoform.com/v1alpha1`.

`release/version.json` keeps `publicationStatus: candidate-only` as descriptor
metadata and must not be presented as live availability state. Any later
publication, admission, or revocation claim still requires the corresponding
retained evidence. See
[`../release/README.md`](../release/README.md), [`../spec/README.md`](../spec/README.md),
and the repository [`AGENTS.md`](../AGENTS.md).

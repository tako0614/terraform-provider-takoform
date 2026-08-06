# takoform.com website

Static public site for the Takoform project, built with **VitePress** and
served through a Cloudflare Worker's static-asset support. Cloudflare is used
only to host `takoform.com` and the immutable public schema URLs; the Takoform
provider, Service Form API contract, Form Packages, and provider-neutral
`EdgeWorker` resource do not require Cloudflare. There is no runtime build or
server-side application: everything under [`public/`](public/) is deployed
as-is.

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

`website/public/` is the **committed build output** and the published byte
set. Because VitePress/Vue scoped-style hashes depend on the absolute build
path, a byte-for-byte rebuild comparison is impossible; instead
`bun run check:website-snapshot` proves the committed output is not stale by
building the committed source fresh (in a throwaway directory under the
repository) and requiring every committed page's semantic content to be
reproduced. The same gate runs again inside the deploy pipeline from a managed
install home outside the frozen archive.

## Pages

The landing, docs, and specification pages are bilingual through VitePress
i18n: English at `/`, Japanese at `/ja/`. Canonical provider reference and
normative specification text remain the Markdown files in the repository; the
public pages link to those files instead of silently redefining their
contracts.

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
publication and historical-ledger checks run only in that clone; the static website,
schema, copy, and deploy-safety checks run only in the archive. Both roots are
clean, exact-commit checked, and re-hashed after validation and before every
writer. Only the archive is used for the credential scan, digest manifest, and
upload, and every archive byte is verified against the commit's Git blobs.
Ignored and untracked files below a publication path are rejected. Wrangler is
installed from the exact committed `bun.lock` and executed by the fixed
absolute Node entrypoint; neither a PATH-provided Wrangler nor its environment
shebang is used.

Before any writer, the deploy re-derives the committed website output with a
fresh pinned VitePress build (same pattern as `check:website-snapshot`, run in
a managed install home outside the archive) and requires every committed page
to be reproduced semantically. This keeps the published bytes equal to the
committed bytes while proving they are current with the committed source.

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

Every normative schema `$id` in
[`../release/public-schema-identities.json`](../release/public-schema-identities.json)
is an immutable published identity. Immediately before every Cloudflare
mutation, the entrypoint binds the current production version message to its
ancestor source commit and retained identity ledger. Every identity from that
deployed ledger must still serve the candidate bytes exactly. An identity that
exists only in the new candidate may be minted from an exact HTTP 404. A
changed body, missing deployed identity, redirect, any other HTTP response, or
transport error blocks publication; an existing `$id` is never repaired in
place. A release that mints a new identity must be repaired forward because
the previous Worker version is no longer a schema-safe rollback target.

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
zone/delegation, ENOTFOUND from every authoritative nameserver, and every
ledger-listed candidate schema byte through the already-routed apex Worker. It then writes
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
resource closure, and checks every ledger-listed candidate schema byte through the apex.
It creates the domain only from a still-safe absent changeset; if the exact
domain is already attached it performs readback only. Any competing state
blocks recovery.

A rollback is permitted only to a version proven to retain all already-minted
schema bytes. The postreadback covers apex and `www` roots, docs, spec,
sitemap, static assets, the custom 404 response, every ledger-listed schema identity,
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
repository. The current public truth is: provider `v2.0.0` is the published,
Registry-verified current client; provider `v1.0.3` is the published Legacy
client; Takoform is an Experimental specification project; and the 34
published Form Package identities are immutable Legacy evidence. There is no
current central Takoform approval or admission. The current
`forms.takoform.com/v1alpha2` epoch contains exactly nine Takosumi Cloud-backed
`0.1.0` candidates. The last published
historical admission identity is `forms/admissions/v1.0.7`; its exact Git and
set identities remain pinned as Legacy evidence.
The frozen Legacy FormRef group is `forms.takoform.com/v1alpha1`. Current Form
Package indexes use `packages.forms.takoform.com/v1alpha3`; published
v1alpha1/v1alpha2 package indexes remain immutable Legacy evidence.

`release/version.json` keeps `publicationStatus: candidate-only` as current
source descriptor metadata for `v2.0.0`; it must not be presented as live
publication state. Signed release and Registry evidence establish `v2.0.0` as
the current published provider. Any later
publication, lifecycle, Host Support, or revocation claim still requires the corresponding
retained evidence. See
[`../release/README.md`](../release/README.md), [`../spec/README.md`](../spec/README.md),
and the repository [`AGENTS.md`](../AGENTS.md).

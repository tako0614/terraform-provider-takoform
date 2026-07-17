# Portable Form host API v1alpha1

The provider uses a versioned, provider-neutral HTTP boundary. A host owns
placement and execution; this protocol owns exact Form identity, portable
desired state, optimistic concurrency, mutation replay, and stable errors.

## Discovery and endpoint selection

`GET /.well-known/takoform` must advertise:

- `api_versions` containing `forms.takoform.com/v1alpha1`;
- `features.service_forms = true`;
- `features.exact_form_ref`, `features.optimistic_concurrency`, and
  `features.idempotent_lifecycle` all set to true;
- an absolute same-origin `endpoints.api` URL;
- an absolute same-origin `endpoints.forms` URL, or `{endpoints.api}/forms`.

The provider sends bearer credentials only to same-origin advertised URLs and
uses `endpoints.api` exactly as advertised. A missing versioned endpoint is an
error. The historical `/v1` facade is available only when
`compatibility_fallback = true` (or `TAKOFORM_COMPATIBILITY_FALLBACK=true`) and
discovery omits `endpoints.api`; it is never an implicit downgrade.

## Exact identity

Every typed provider resource is compiled against one release-owned
`InstalledFormReference`: `apiVersion`, `kind`, `definitionVersion`,
`schemaDigest`, and `packageDigest`. `GET /forms` must return that exact
identity as installed, executable, activated, available to the principal, and
supporting the requested operation. Resource bodies carry the same `form` and
read/lifecycle URLs carry all five fields as query parameters. Responses that
substitute any identity field fail closed.

The current ten exact references are pinned by
[`forms/legacy-package-set.json`](../../forms/legacy-package-set.json). They
remain compatibility candidates, not portable standards.

## Resource lifecycle

The API base is `/apis/forms.takoform.com/v1alpha1` on the reference host:

- `GET /forms` discovers exact availability;
- `POST /resources/preview` returns `review.planDigest`;
- `PUT /resources/{kind}/{name}` applies that reviewed plan;
- `GET /resources/{kind}/{name}` reads canonical portable state;
- `POST /resources/{kind}/{name}/import` imports a native object;
- `POST /resources/{kind}/{name}/observe` observes drift;
- `POST /resources/{kind}/{name}/refresh` refreshes state;
- `DELETE /resources/{kind}/{name}` deletes it.

Create and new-resource import use `If-None-Match: *`. Update, existing-resource
import, observe, refresh, and delete use one quoted `If-Match` resource version.
Every apply/import/observe/refresh/delete request has a deterministic
`Idempotency-Key`; retries reuse the same key.
Only an error with `retryable: true` and code `resource_busy` or
`backend_unavailable` is automatically retried. A resource-version conflict is
never retried.

Stable errors use
`{ "error": { "code", "message", "requestId", "retryable", "hostCode?" } }`.
Provider diagnostics may expose the stable code and request ID, but state never
contains credentials, prices, quotes, backend selection, Target identity, or
manager authority.

## Cross-repo conformance

[`conformance/portable-host-v1/contract.json`](../../conformance/portable-host-v1/contract.json)
is the digest-pinned cross-repo input for a neutral host runner. Its required
check names match Takosumi's black-box
`core/conformance/portable_form_host.ts` runner without making this repository
depend on Takosumi source or closed Cloud code. `go run ./cmd/conformance verify`
checks the fixture and its exact release-owned ObjectBucket identity.

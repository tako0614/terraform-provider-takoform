# Portable form-host API candidate

The provider candidate uses the following HTTP boundary. It is deliberately independent of backend managers and operator administration.

## Discovery

`GET /.well-known/takoform` returns:

- `api_versions`, which must include `forms.takoform.com/v1alpha1`;
- `features.service_forms`, which must be `true`;
- `endpoints.capabilities`, an optional absolute capabilities URL.

The JSON shape is defined by [`../../schemas/host-discovery.schema.json`](../../schemas/host-discovery.schema.json). Unknown feature and endpoint keys are ignored for forward compatibility.

## Capabilities

`GET /v1/capabilities` returns `apiVersion` and a `resources` map. A resource is usable only when its exact kind is `true`. Provider schema presence does not imply that a host offers or can realize a form.

## Resource lifecycle

- `POST /v1/resources/preview` validates and previews a desired Resource envelope.
- `PUT /v1/resources/{kind}/{name}` applies the previewed desired state.
- `GET /v1/resources/{kind}/{name}` observes current state.
- `DELETE /v1/resources/{kind}/{name}` deletes it.
- `space` is passed as a query parameter when non-empty.

The Resource envelope contains `apiVersion`, `kind`, `metadata`, `spec`, and sanitized `status`. The provider never accepts a backend, target pool, credential, price, billing, capacity, or operator-policy selector in desired HCL.

The current preview/apply evidence fields are characterized by Go client tests. They are not yet a final interoperability standard: stable idempotency, optimistic concurrency, retry, timeout, error reason, and versioned FormRef semantics still require normative schemas and cross-host fixtures.

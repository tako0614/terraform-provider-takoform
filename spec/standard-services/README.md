# External standard services (`standards.takoform.com/v1`)

Some service categories already have a portable protocol: S3-compatible object
storage, PostgreSQL, Redis, SMTP, and others. Takoform does not restate those
protocols as Forms. A Form runtime asks its Host for one through a sealed
standard-service slot. This v1 contract is part of the stable
`forms.takoform.com/v1` lane; the occupied beta4 lane retains its published
v1alpha1 standard-service wire unchanged.

Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

## Portable declaration

Portable desired state carries only a runtime binding name, an opaque protocol
identifier, and whether the binding is required:

```json
{
  "name": "MEDIA",
  "service": {
    "apiVersion": "standards.takoform.com/v1",
    "protocol": "com.amazonaws.s3"
  },
  "required": true
}
```

- `name` matches `^[A-Z][A-Z0-9_]*$` and is at most 64 characters. It is the
  key of one sealed runtime-native binding.
- `service` validates against
  [`standard-service-ref-v1.schema.json`](../schemas/standard-service-ref-v1.schema.json).
  `protocol` is a normalized reverse-DNS owner namespace followed by a
  protocol segment, at most 253 characters. It is compared as an opaque,
  case-sensitive string.
- `required` is optional and defaults to `true`. An unsupported required slot
  is refused before mutation and cannot become Ready. An unsupported optional
  slot projects nothing and does not block readiness.
- The array property embedding slots carries
  `x-takoform-standard-services: standards.takoform.com/v1`, so a Host derives
  the declarations from the exact Form Definition.

The protocol grammar is deliberately open. A protocol unknown to Takoform is
schema-valid and needs no Takoform registry entry or release. Schema validity
does not certify that a service conforms to the named protocol.

## Exact Host support

A Host integration plugin owns whether it supports an identifier and how it
materializes that protocol. The Host Support surface answers with a
`StandardServiceSupport` profile whose `serviceRef` exactly equals the slot's
`{apiVersion, protocol}` and whose `satisfiable` value states whether this Host
can wire it for the requesting tenant. A missing profile, a different
identifier, or `satisfiable: false` fails closed for a required slot.

This is capability and tenant wiring, not Form semantics. The same Form and
provider configuration remain valid against another Host that advertises the
exact identifier.

## Sealed runtime binding

Takoform guarantees one sealed runtime-native binding under the declared
`name`. The Host integration and the protocol it implements own that binding's
internal entries, delivery mechanism, endpoint layout, and credential shape.
Takoform defines no protocol-to-environment-variable table and does not require
filesystem delivery.

The slot name shares the consumer runtime's binding namespace with vars,
sensitive-value declarations, and typed bindings. A duplicate binding name is
`invalid_argument` before mutation. Integration-internal entry names do not
become portable Form semantics.

Every materialized value is sealed. Endpoints, regions, instance names,
credentials, and integration-private entries MUST NOT appear in desired state,
observed state, outputs, provider state, diagnostics, or Host-returned logs.
Observed state MAY report whether a slot is satisfied, but never with what.

## Boundary

- A slot grants no portable lifecycle authority over a service instance and
  carries no Resource/Form selector. Provisioning, placement, readiness,
  credentials, and replacement are Host integration responsibilities.
- The standard protocol remains the data-plane authority. Takoform owns only
  the generic reference, sealing, exact support lookup, collision, and
  readiness rules.
- A call-only or externally managed service needs no Form merely to be used by
  a runtime.

## Retained v1alpha1 history

[`standard-service-ref-v1alpha1.schema.json`](../schemas/standard-service-ref-v1alpha1.schema.json)
is immutable published history. Its bare closed values (`s3-compatible`,
`postgresql`, `redis`, `smtp`) and protocol-specific projection table are not
accepted by current `v1` slots and are not widened in place.

## Conformance

Portable conformance is protocol-neutral. It proves:

- an unknown but grammar-valid protocol survives Form, provider, and Host wire
  handling unchanged;
- a required protocol without an exact satisfiable Host profile fails closed;
- optional unsatisfied slots project nothing and do not block readiness;
- slot-name collisions are refused before mutation; and
- no materialized endpoint, credential, or integration-private entry escapes
  the sealed runtime boundary.

Protocol-specific projection and data-plane behavior belong to the Host
integration's own tests (for example, an S3 integration tests its endpoint,
region, bucket, and credential materialization without placing those values in
portable state).

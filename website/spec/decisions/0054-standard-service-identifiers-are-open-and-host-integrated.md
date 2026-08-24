# 0054 — Standard-service identifiers are open and integrations are Host-owned

- Status: Accepted
- Date: 2026-08-23

## Context

[Decision 0043](0043-forms-target-popular-vendor-locked-primitives.md) correctly
separates established standard data planes from lifecycle Forms, and
[decision 0045](0045-external-standard-services-are-sealed-slots.md) correctly
keeps endpoints and credentials behind a sealed slot. Their first concrete
contract, however, made two details Takoform-owned that do not belong here:

1. `standards.takoform.com/v1alpha1` enumerates four protocol names, so a Form
   cannot request a standard Takoform did not know when that schema shipped.
2. Takoform fixes environment members for each protocol, so adding or evolving
   an integration requires a Takoform release even though the protocol and the
   Host implementation own those entries.

This couples an external standard's identity and runtime integration to the
Takoform registry and release cadence. It also makes schema acceptance look
like protocol certification, which a structural schema cannot provide.

## Decision

The current StandardServiceRef is `standards.takoform.com/v1` and contains:

```json
{
  "apiVersion": "standards.takoform.com/v1",
  "protocol": "com.amazonaws.s3"
}
```

`protocol` is an opaque, case-sensitive, normalized reverse-DNS owner namespace
followed by a protocol segment. The schema constrains only that grammar and a
253-character bound. Takoform maintains no enum or registry. A previously
unknown but grammar-valid identifier is valid Form data; its presence is not a
claim or certification that any implementation conforms to that protocol.

A Host integration plugin owns exact protocol support and runtime
materialization. Its `StandardServiceSupport` profile echoes the exact
`{apiVersion, protocol}` and states tenant satisfiability. A required slot is
accepted only with an exact satisfiable answer; an unknown, missing, or
mismatched profile fails closed.

Takoform guarantees one sealed runtime-native binding keyed by the slot's
`name`. The Host integration and protocol own the binding's internal entries,
delivery mechanism, endpoint layout, and credential shape. Takoform defines no
protocol-to-environment-variable or filesystem-projection table. Collision is
generic over the slot binding name and the runtime's other binding names.

The slot remains `{name, service, required?}`. It gains no Resource selector,
endpoint, credential, provider, or arbitrary entry map. `required` continues to
default to true; optional unsatisfied slots project no binding.

This contract is introduced only with the stable
`forms.takoform.com/v1` Host lane and stable Form Definition profile. The
occupied `forms.takoform.com/v1beta4` Host documents, support profile, and
conformance corpus retain their original v1alpha1 standard-service reference.

## Superseded portions

This decision supersedes only:

- decision 0043's closed Takoform protocol vocabulary and reviewed-widening
  requirement; and
- decision 0045's closed vocabulary and protocol-specific projected-member
  table.

Their remaining boundary stays accepted: categories with established
standards are integrated rather than respecified as Forms, and portable state
contains no endpoint or credential.

`standard-service-ref-v1alpha1.schema.json` and its four bare protocol values
remain immutable published history. They are not widened or reinterpreted.

## Enforcement

- The v1 StandardServiceRef schema has a pattern and bound, and no enum.
- Stable-v1 Form schemas embed the exact v1 apiVersion and annotation.
- Model and provider round trips preserve unknown grammar-valid identifiers.
- Provider/Host support lookup compares the complete serviceRef and fails
  closed for an unsupported required slot.
- Portable conformance tests generic sealing, slot-name collision, exact
  unsupported-protocol refusal, and readiness. Protocol-specific projections
  belong to each Host integration's own tests.

## Consequences

A new standard can be integrated by its namespace owner and a Host without a
Takoform schema release. Takoform still gives authors a typed, bounded,
non-secret declaration and still makes unsupported required capability visible
before mutation. The cost is intentional: portable code cannot assume a
Takoform-defined set of environment members for a protocol; it consumes the
runtime-native binding contract supplied by the integration it selected.

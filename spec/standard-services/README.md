# External standard services (standards.takoform.com/v1alpha1)

Some service categories already have the portability layer Takoform exists to
provide: object storage has the S3-compatible API, relational databases have
the PostgreSQL wire protocol, caches have the Redis protocol, mail submission
has SMTP. [Decision 0043](../decisions/0043-forms-target-popular-vendor-locked-primitives.md)
forbids respecifying them as Forms. This contract is the other half of that
decision: how a Form's runtime **reaches** such a service.

Requirement keywords are used as described in
[`../conformance.md`](../conformance.md).

## The model: a sealed slot with a protocol tag

An external standard service is declared exactly the way host-supplied sealed
values already are ([`../form-families.md`](../form-families.md)): **portable
state carries only a name and a protocol, never an endpoint and never a
credential.** The declaration a consuming Form's desired schema embeds is:

```json
{
  "name": "MEDIA",
  "service": {
    "apiVersion": "standards.takoform.com/v1alpha1",
    "protocol": "s3-compatible"
  }
}
```

- `name` follows the consuming family's binding-name grammar and shares its
  one runtime namespace: the union of vars keys, sensitive-value names,
  binding names, and external-service names MUST be unique, and a host MUST
  refuse the collision before mutation.
- `service` validates against
  [`standard-service-ref-v1alpha1.schema.json`](../schemas/standard-service-ref-v1alpha1.schema.json).
  The protocol vocabulary is closed: `s3-compatible`, `postgresql`, `redis`,
  `smtp`. Widening it is a reviewed spec change held to decision 0043's test —
  the protocol must be a de-facto standard, or the category belongs to a Form
  Family instead.

The host or operator **satisfies** a slot by supplying connection material for
a service that actually speaks the declared protocol. Where that service runs —
a cloud provider, the host's own infrastructure, a box under a desk — is
invisible to portable state, exactly as host placement already is. Which slots
a host satisfies, and with what, is host/operator policy; a REQUIRED slot a
host cannot or will not satisfy makes the Resource not Ready, and a host that
knows it cannot satisfy one MUST refuse at plan time with
`unsupported_capability` rather than at runtime.

## What each protocol projects

The slot's runtime projection is defined per protocol, so a consumer's code is
portable across hosts. Projected member names below are **runtime names**, not
portable state; they never appear in a Form Package or a Resource document.
For an environment-style runtime namespace (the Worker family's `env`, a
container revision's environment), a slot named `NAME` projects:

| Protocol | Projected members | Content |
| --- | --- | --- |
| `postgresql` | `NAME_URL` | one libpq-style `postgresql://` connection URI carrying host, port, database, and authentication |
| `redis` | `NAME_URL` | one `redis://` or `rediss://` URI carrying host, port, and authentication |
| `smtp` | `NAME_URL` | one `smtps://` or `smtp://` submission URI carrying host, port, and authentication; `smtp://` implies STARTTLS on the standard submission port |
| `s3-compatible` | `NAME_ENDPOINT`, `NAME_REGION`, `NAME_BUCKET`, `NAME_ACCESS_KEY_ID`, `NAME_SECRET_ACCESS_KEY` | the five values every S3-compatible SDK takes; `NAME_REGION` MAY be the literal `auto` where the service ignores regions |

Every projected value is sealed exactly like a sensitive variable: it MUST NOT
appear in desired state, observed state, outputs, diagnostics, logs the host
returns, or provider state. The observed document MAY state that a slot is
satisfied; it MUST NOT state with what.

## What this contract does not do

- It grants no lifecycle authority over the external service. Takoform never
  creates, migrates, or deletes a PostgreSQL database or an S3 bucket through
  this contract; the slot reaches a service somebody else provisioned. A
  category graduating from "reached" to "provisioned" is a Form Family
  decision under decision 0043's test, not a widening of this contract.
- It does not verify protocol conformance of the satisfied service. The host
  vouches for what it wires; measuring it is host-side quality, not portable
  conformance.
- It adds nothing to the published v1beta1 Edge schemas, which are frozen
  identities. Families minted after this contract carry the declaration from
  birth; the Edge family adopts it when a graduation mints its next
  identities ([decision 0037](../decisions/0037-immutability-begins-at-stable.md)).

## Conformance

The declaration shape is normative now; executable conformance arrives with
the first family whose desired schemas embed it, as part of that family's
corpus (plan-time `unsupported_capability` refusal, readiness gating on an
unsatisfied REQUIRED slot, namespace-collision refusal, and the sealing of
projected values). Until such a corpus exists, no host can claim measured
support for this contract.

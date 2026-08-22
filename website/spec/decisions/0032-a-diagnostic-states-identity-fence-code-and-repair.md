# 0032 — A provider diagnostic states identity, fence, code, and repair

- Status: accepted
- Date: 2026-08-09
- Owners: Takoform maintainers

## Context

A provider diagnostic is the only thing a practitioner sees when an apply stops,
and this lane's whole vocabulary is what makes one actionable. Decision
[0011](0011-resource-identity-generation-and-revision.md) gave every resource a
UID, a generation, and a revision, and made desired mutations fence on the
generation and deletes on the revision — the delete half of which that record
has since amended to the generation, for reasons of its own. Decision
[0017](0017-provider-state-survives-form-evolution-and-interruption.md) made the
exact FormRef the identity every read, update, and delete addresses, and made
`pending_operation_id` load-bearing. The wire error envelope carries a code from
a closed taxonomy, a request id, and a retryable flag.

An audit of all 135 diagnostic call sites in `internal/provider` found that
almost none of that reached the reader. Twenty-one sites rendered `err.Error()`
and nothing else. `Failed to update <Kind>` was raised on the one path where a
`uid_mismatch` or a `generation_conflict` arrives, and neither the expected uid
nor the expected generation — both in scope, both just sent as the fence — was
in the message. `Failed to delete <Kind>` fenced on the revision and never named
it. No diagnostic anywhere carried a JSON pointer. The typed
`clientv3.APIError`, which already holds the code, the request id, the retryable
flag, and the HTTP status, was never destructured by the provider at all: the
only `errors.As` in the package was for `AcceptedError`.

One instance had a further defect of its own shape. `interface_data_source.go`
answered `"Provider not configured"` against a v3-only host, fusing two
structurally different states — a provider bug and an endpoint fact — under one
title, and discarding the recorded `v2Err` that says which. The v2 form
resources and the v3 resources both already reported the lane and the underlying
error; the data source was the one lane-asserting site that did not.

## Decision

Every error the v1alpha3 lane raises goes through one renderer and carries, when
it has them: the host address of the resource (type, space, name), the exact
FormRef, a JSON pointer into the portable document, the expected and current
UID, the expected and current generation and revision, the operation id, the
host request id, a stable error code, whether it is retryable, and one concrete
repair. A fact the diagnostic does not have is omitted, never rendered empty.

Two code vocabularies meet and are kept apart. A HOST error renders its code
from the closed v1alpha3 taxonomy verbatim, labelled as the host's and carrying
its HTTP status; that enum is published and this provider does not extend it. A
PROVIDER refusal — one made before any request — renders a code from a closed
set in its own `takoform.provider/` namespace, because a client-side refusal is
not a host outcome and must not borrow a host code. A response that did not
match the closed taxonomy is labelled protocol-invalid and offers no guessed
remedy.

The repair is per stable code and is complete: a test reads
`../host-api/operations-v1alpha3.json`
and fails if any published code has no repair, or if the provider states a
repair for a code that is not published. A diagnostic that names a fault and no
next action is the shape this decision exists to remove.

The Terraform resource ADDRESS stays Terraform's to print. A provider cannot
know it; core prefixes every diagnostic raised during a resource operation with
`with <type>.<name>, on <file> line <n>`. What the provider owns and now always
states is the HOST address.

The lane-unavailable diagnostic is one shape shared by every lane-asserting
site, resource and data source alike: it names the lane, the resource type, the
recorded per-lane negotiation error, the fact that the provider itself
configured normally, and what to change. `"Provider not configured"` is reserved
for a genuinely nil provider data — a provider bug, and labelled as one.

## Consequences

- The audited sites now render the fences they send. A `generation_conflict` on
  update shows the generation that was sent, and so does one on delete, since
  the delete fence became the generation
  ([0011](0011-resource-identity-generation-and-revision.md), amended
  2026-08-09). `revision_conflict` keeps its repair: the code stays in the
  published taxonomy and stays reachable for a caller that supplies the optional
  representation fence, and the repair test is over the published codes rather
  than over the ones this client happens to provoke.
- The interface data source reports `Takoform v1alpha2 lane unavailable` with
  the endpoint's own reason, symmetric with the v2 resources and the v3
  resources.
- The host's relation-drift `hostReason` is parsed tolerantly for the pointer
  and the two uids, which are promoted into the identity block; the raw
  `hostReason` is rendered either way, so nothing the host said is lost when a
  host words it differently.
- The retained v1alpha2 lane (`internal/provider/form_resource.go`) is out of
  scope by repository rule and keeps its existing diagnostics. Six thin sites
  remain there and are recorded rather than fixed.
- Adding a code to the published taxonomy now fails a provider test until a
  repair is written for it, which is the intended coupling.

## Rejected alternatives

- **Extend the closed host error enum with provider-side codes.** Rejected
  because that enum is published and closed, and because a reader who greps a
  code needs to know whether the host said it or the client did.
- **Attach every diagnostic to an attribute path instead of naming a pointer.**
  An attribute path is the right home for a configuration fault and is used for
  one. It is the wrong home for a fault about the host document — a relation
  whose target moved is at `/kvBindings/0/resource` in the portable spec, and
  the attribute that produced it may not exist in the configuration at all.
  Both are carried.
- **Render `err.Error()` and let the host's message carry everything.** This is
  the status quo. The host's message is one sentence about one request; it does
  not know which resource address Terraform is applying, which FormRef state
  records, or which fence the client chose to send.

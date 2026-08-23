# 0050 — A standard-backed Form provisions an instance; it never respecifies the protocol

- Status: Accepted
- Date: 2026-08-23

## Context

The maintainer asked how the industry-standard endpoints — S3, an
OpenAI-compatible API — are used from Takoform without Takoform specifying
them, given that takoserver wants to implement them too.

[Decision 0043](0043-forms-target-popular-vendor-locked-primitives.md) already
answers half of it: a category with a de-facto standard API is never
respecified as a Form, and a Form's runtime reaches one through a sealed slot
([decision 0045](0045-external-standard-services-are-sealed-slots.md)). What
0043 does not say is how an instance of such a service comes to EXIST, and
that is the half the question is about.

### The repository already answers it twice, and one answer contradicts 0043

`ObjectBucket` is a Form whose desired schema has **no properties at all**. Its
whole portable desired state is its identity: a bucket exists. On takoserver
its data plane is reached by an ordinary S3 client, through a credential
route (`/v1/organizations/{org}/resources/{uid}/s3-credentials`) that vends a
short-lived, bucket-scoped key. Nothing about the endpoint or the credential is
in portable state. That is exactly the shape the question is looking for.

`ObjectBucket` also declares `edge.objects@1.0.0`, and it is worth being exact
about what that is, because the obvious reading is wrong. `edge.objects` is not
an HTTP wire protocol competing with S3: it fixes the storage's SEMANTICS —
strong read-after-write, streaming bodies, last-writer-wins per key, no
cross-key atomicity, no versioning, a 5 GiB object ceiling — and a Binding
projects those operations into the consumer's own runtime, where the code
writes `env.MEDIA.get(...)` rather than signing a request. The two live at
different layers, and the binding is a surface S3 does not provide at all.

Where they DO overlap is the semantics, and 0043's test is about the category
rather than the layer: object storage has a de-facto standard API, so a
contract specifying object-storage semantics is one 0043 would question today.
What follows from that is not that `edge.objects` should be withdrawn — its
bytes are published and [decision 0037](0037-immutability-begins-at-stable.md)
keeps them — but that a bucket needs no SECOND Form to be reachable by an
ordinary S3 client.

takoserver's `/v1/ai` is the second answer, and a different shape: nothing is
provisioned per tenant, an organization key is simply granted `ai:invoke`. That
is a service you call, not a resource you own.

### What is missing

A sealed slot's `service` member is `{apiVersion, protocol}`. It names a
PROTOCOL, so a slot can only ever mean "some service, resolved out of band,
that speaks this". It cannot name a bucket the same desired document just
asked for. Provision-and-wire is therefore not expressible in one portable
document, which is the gap.

## Decision

**A standard-backed Form provisions an INSTANCE and names the standard as its
data plane. It declares no Interface that respecifies that standard, and its
desired state carries no member the standard already defines.**

0043 forbids a Form that describes what a protocol MEANS. It does not forbid a
Form that says an instance of one EXISTS — and existence is portable desired
state in a way the protocol's semantics are not: *"I need a bucket"* is the
same sentence on every host, while how large, where, and on whose hardware are
offering policy and never portable
([decision 0007](0007-current-forms-exclude-substrate-operation.md)).

The rule has a mechanical test: a standard-backed Form's desired schema adds
nothing the standard already carries. `ObjectBucket`'s empty desired schema is
what that looks like when it is done right.

**A sealed slot may name a resource, not only an out-of-band service.** A slot
resolves to either the service an operator wired out of band — unchanged — or
a resource in the same desired state that provides the named protocol. Either
way the host resolves the endpoint and the credential, and neither enters
portable state; the difference is only whether the thing on the other end is
something the same document asked for.

**One resource, two projections — never two Forms for one thing.** A bucket
already exists as a Form, so a standard-backed bucket is not minted beside it.
What is added is the second way to reach the same resource: a typed binding
projects the Interface into code running on the host's own runtime, and a
sealed slot hands credentials to everything else — a laptop, a CI job, a
process on another cloud. Splitting the resource in two would make an author
choose between them at provisioning time for a difference that only shows up
at consumption time.

**A call-only standard gets no Form.** Inference, mail submission, telemetry
export: nothing is provisioned per tenant, so there is no instance to own and
nothing for a lifecycle to hold. The slot is the whole mechanism, and what such
a standard needs from this project is an entry in the protocol vocabulary that
0043 governs — `openai-compatible` and `otlp` are on that roadmap and are not
yet minted.

## Enforcement

Slot resolution refuses a reference to a resource that does not provide the
named protocol, at prepare time, exactly as it refuses an unsatisfiable slot
today (decision 0045). The protocol vocabulary stays closed and every widening
is held to 0043's table. A proposed standard-backed Form is a reviewed change
whose review asks one question: does its desired state state anything the
standard already states? If yes, it is a respecification wearing a lifecycle.

`edge.objects` and `ObjectBucket` are retained as published. What this record
stops is not their existence but the reflex to mint a parallel Form whenever a
standard is involved: the question a review asks is whether the resource
already exists, and only then whether a new one would state anything the
standard already states.

## Consequences

takoserver's two-sided role becomes constructible rather than aspirational: it
serves the standard endpoints itself, and as a Takoform host it provisions
instances of them — and an author writes both halves in one desired document,
provisioning a bucket and wiring it into a worker as an `s3-compatible` slot,
with no endpoint and no credential anywhere in that document.

The cost is that Takoform gains Forms in categories 0043 sent to the integrate
column. That is not a reversal of 0043: those Forms carry no semantics, and the
moment one of them grows a member the standard already defines, this record
says it has become the thing 0043 refused.

This record also corrects itself on one point, because the first draft asserted
more than the evidence carried: it called `edge.objects` a respecification of
the S3 data plane. It is not — it is a semantic contract with a runtime
projection, and S3 is a wire protocol. The overlap is real and worth naming,
and the overstatement was not.

# ContainerCustomDomain — `takoform_container_custom_domain`

> Historical/deferred candidate (English-only). This non-Edge Form is not in
> the current official Edge16 corpus or Current navigation.

## Workload and consumer

A team serves its container service on its own DNS hostname over HTTPS.
External clients reach the service's active traffic split at that hostname;
certificates and TLS termination are the host's obligation, never portable
state.

## Role

`attachment`. Inward activation is an attachment resource, never a binding
(decision 0010). Deleting the attachment detaches the hostname and never
deletes the service.

## Observable semantics

`hostname` is one dotted DNS name; `service` is the Container Service whose
active traffic split answers it. Both are immutable: changing either
replaces the attachment. Requests on the hostname are delivered to instances
of the revisions the traffic resource selects, entirely one revision per
request.

Certificate issuance is a host duty behind this attachment —
[decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md)
records ACME as exactly that — and no certificate material, validation
record, or issuer choice enters portable state.

## Why this is one Form

Hostname attachment is one complete observable fact: which name reaches
which service. Certificate mechanics differ per host and are deliberately
outside the contract.

## What would require a separate Form

Path-pattern routing, multi-service routing under one hostname, and redirect
or rewrite rules carry matching semantics beyond a whole hostname and are
separate attachment Forms.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Two attachments claiming one hostname must conflict deterministically. They
do: the hostname is canonicalized before it is compared and before it is
stored, and one canonical hostname is claimed by at most one attachment per
tenant, with `invalid_argument` (400) before any mutation
([decision 0026](../../spec/decisions/0026-attachment-claims-are-canonical-and-acyclic.md)).
Attaching while the service has no traffic resource must be refused, per the
family aggregate. Deleting the service while the attachment exists must fail
with `dependency_in_use`. Import must recover hostname and service exactly.

## Prior art

The custom-domain mapping of a proven serverless container platform, with
certificate provisioning kept host-side.

# WorkerCustomDomain — `takoform_worker_custom_domain`

## Workload and consumer

A team serves its worker on its own DNS hostname over HTTPS. External
clients reach the worker's active deployment at that hostname; certificates
and TLS termination are the host's obligation, never portable state.

## Role

`attachment`. Inward activation is an attachment resource, never a binding
(decision 0010). Deleting the attachment detaches the hostname and never
deletes the worker.

## Observable semantics

`hostname` is one dotted DNS name; `worker` is the Module Worker whose
active deployment answers it. Both are immutable: changing either replaces
the attachment. Requests on the hostname invoke the worker's `fetch`
handler.

## Why this is one Form

Hostname attachment is one complete observable fact: which name reaches
which worker. Certificate mechanics differ per host and are deliberately
outside the contract.

## What would require a separate Form

Path-pattern or zone-scoped routes carry matching semantics beyond a whole
hostname and are a separate attachment Form (`WorkerRoute` in the family
plan, [spec/form-families.md](../../spec/form-families.md)).

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

Two attachments claiming one hostname must conflict deterministically.
They do: the hostname is canonicalized before it is compared and before it is
stored, and one canonical hostname is claimed by at most one attachment per
tenant, with `invalid_argument` (400) before any mutation
([decision 0023](../../spec/decisions/0023-attachment-claims-are-canonical-and-acyclic.md)).
Deleting the worker while the attachment exists must fail with
`dependency_in_use`. Import must recover hostname and worker exactly.

## Prior art

The custom-domain attachment of a proven edge platform, with its
certificate provisioning kept host-side.

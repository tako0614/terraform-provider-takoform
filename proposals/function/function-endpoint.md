# FunctionEndpoint — `takoform_function_endpoint`

## Workload and consumer

A team wants their function callable over plain HTTPS, now, without owning a
gateway or a domain. External clients send requests to an address the host
assigns and publishes; TLS termination and the naming scheme are the host's
obligation, never portable state.

## Role

`attachment`. Inward activation is an attachment resource, never a binding
(decision 0010). Deleting the attachment removes the address and never
deletes the function.

## Observable semantics

`function` is the only desired member, and it is immutable: changing it
replaces the attachment. A request at the assigned address is delivered as
one `http` invocation event of the `function.runtime@1.0.0` contract to a
version the function's ACTIVE DEPLOYMENT selects, and the handler's return
value is mapped to the HTTP response exactly as that contract fixes.
Promotion and rollback move what answers without the endpoint being
re-applied and without the address changing; each request is served entirely
by one version.

The address is published as outputs: `hostname` is the assigned DNS name,
and `url` is exactly `https://` + that hostname + `/`. A portable author may
rely on a value being returned, on the scheme being HTTPS, and on the
address routing to the active deployment — and on nothing about its shape:
they must not parse it, assert a suffix, or reconstruct it from the resource
name
([decision 0024](../../spec/decisions/0024-a-worker-is-reachable-at-a-host-assigned-address.md),
[decision 0025](../../spec/decisions/0025-declared-outputs-are-a-typed-contract.md)).

A function has at most one endpoint. A host that cannot assign an address
refuses the attachment with `unsupported_capability` rather than answering
with an address it did not assign.

## Why this is one Form

Reachability without a name of one's own is one complete observable fact:
the function answers at an address, over HTTPS, at the path root. Merging it
with an author-owned hostname would need a selector token between two
different requests, exactly as the Edge family records for its endpoint
pair.

## What would require a separate Form

An author-owned hostname is a future custom-domain attachment of this
family. Access control — callers restricted by identity or signature — is
policy, a separate authority-bearing Form. Scheduled or source-driven
invocation is not HTTP activation at all and lives in the source's family.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

The assigned address must be stable for the life of one incarnation: a
consumer holding the URL must not find it moved by a deployment change it
did not make. A second endpoint against one function must be refused
deterministically, by the function's UID rather than its name. Attaching
while the function has no deployment must be refused, per the family
aggregate. Deleting the function while the endpoint exists must fail with
`dependency_in_use`. Import must recover the function reference and the
assigned address exactly, without minting a new one.

## Prior art

The direct HTTPS URL of a proven cloud function service, with the address
published as a typed output contract and its authorization modes left to
separate policy work.

---
title: Concepts
description: The portability boundary, Form identity, and lifecycle behind Takoform.
---

# Concepts

Takoform keeps workload semantics close to the Form and operational concerns
close to the host. These three ideas explain most of the Provider surface.

## Portability has a boundary

A Form owns the application-visible shape of one service primitive: execution
ABI, consistency, delivery guarantees, update units, and lifecycle. The host
owns implementation, placement, capacity, credentials, routing, recovery,
catalog, billing, quota, and SLA.

Portability therefore means moving the same shape between conforming hosts. It
does not mean flattening services into a lowest-common-denominator resource.
Different semantics require different Forms.

## A Form is an exact identity

The four-part current model is deliberately explicit:

- the Host API lane is the literal `forms.takoform.com/v1`;
- a Form carries its own `formId` and `definitionVersion`;
- the Core library follows its own SemVer (`v1.1.0` today); and
- the Provider follows its own Registry SemVer (`3.0.0` today).

A family is a discovery grouping, not a version. A package envelope, schema
`$id`, digest, or Provider descriptor is artifact evidence, not a hidden fifth
stream.

## Resources form a lifecycle graph

Most resources move through a predictable sequence:

1. **Identity** — name the service shape and its stable UID.
2. **Revision** — upload or declare immutable bytes and the fields that make a revision.
3. **Deployment** — choose which revisions receive traffic.
4. **Attachment** — expose a domain, endpoint, trigger, consumer, or other inward edge.

The Provider sends validate, prepare, apply, observe, and delete requests with
UID, generation, and revision fences. A missing deployment or stale generation
is a contract failure to resolve, not a reason to reinterpret a version.

## Capability is host-owned

Before applying, the Provider reads the host's Host Support Profile and checks
that the exact FormRef is supported. Typed bindings then add capability to a
revision. Placement and credentials never move into the Form definition.

## Continue with the source material

The [version model](/docs/versions.html) is the current compatibility entry
point. The old [Specification routes](/spec/) remain available as historical
source with a banner; they document receipts and withdrawn decisions rather
than a live release train.

<StatusNote />

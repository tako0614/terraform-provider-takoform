---
title: Ownership
description: The repositories and runtimes that own each Takoform boundary.
---

# Ownership

Takoform stays portable because each boundary has one authority. A Provider
can project a contract, but it cannot become the authority for the contract it
consumes.

| Boundary                                                | Owner                                                                                    | What that owner decides                                                                                                                             |
| ------------------------------------------------------- | ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Normative Specification, schemas, conformance, receipts | [`takoform`](https://github.com/tako0614/takoform)                                       | Host API semantics, identity grammar, canonicalization, package/trust format, and historical evidence. The same repository publishes Core `v1.1.0`. |
| Form families and exact FormRefs                        | [`takoform-forms`](https://github.com/tako0614/takoform-forms)                           | Family groups, `formId`, `definitionVersion`, Form/package bytes, fixtures, signatures, deprecation, and revocation.                                |
| Terraform/OpenTofu Provider                             | [`terraform-provider-takoform`](https://github.com/tako0614/terraform-provider-takoform) | Typed resource/state/import mappings and immutable Provider release history.                                                                        |
| Host runtime                                            | The host operator and implementation                                                     | Capability support, placement, scaling, credentials, routing, recovery, billing, quota, SLA, and live catalog.                                      |
| Deployment and secrets                                  | The operator                                                                             | Endpoint, space, credentials, capacity policy, and production state. These never become public Form or Provider metadata.                           |

## Practical consequences

- A Provider release does not publish a Form or change the Host API lane.
- A Core release does not select a Provider version or promote a Form.
- A Form publisher changes `definitionVersion` when the Form's semantics change.
- A host advertises the exact FormRefs it supports through its Host Support Profile.
- The site can summarize these facts, but it does not own live capability, credentials, or deployment state.

The current wire identity is the literal `forms.takoform.com/v1`. The site does
not mint `/v1.1` or treat a numbered Specification receipt as a release
authority for the other owners.

## Source and history

The [version model](/docs/versions.html) shows the four current streams. The
retained [Specification routes](/spec/) and [release routes](/release/) are
historical source; their banners make that status explicit.

<StatusNote />

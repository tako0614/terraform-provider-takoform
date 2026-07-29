# 0003 — SpaceID is an opaque portable scope identity

- Status: accepted
- Date: 2026-07-29
- Owners: Takoform maintainers

## Context

`metadata.space` scopes portable Resources and participates in exact Form
queries, Interface reads, idempotency scope, Terraform/OpenTofu configuration,
state, and import identifiers. The host wire schema previously constrained
only its length, while clients and provider paths sometimes used
`strings.TrimSpace`. That made the same identity vulnerable to different
interpretation across runtimes.

Resource `metadata.name` has a separate lowercase URL-safe `PatternName`
grammar. Reusing it for Space would make human or externally assigned scope
identities unnecessarily restrictive and would conflate two domain concepts.

## Decision

`SpaceID` is a distinct, case-sensitive opaque string. It is valid UTF-8 and
contains 1 through 255 Unicode code points. It contains no C0 or C1 control
code point and no `/`. Its first and last code points are neither Unicode
`White_Space` nor `U+FEFF`.

Embedded non-control whitespace is valid data. Clients, hosts, and provider
paths preserve the exact decoded code-point sequence without trimming,
normalization, or case folding. The same validator governs Resource bodies,
query parameters, provider defaults and overrides, Interface selectors,
state, and the `SPACE/NAME` import form.

The normative JSON Schema uses explicit code-point sets. The Go client mirrors
those sets rather than delegating their definition to runtime-specific trim
behavior.

## Consequences

- `Prod`, `prod`, and `Prod North` are three exact values.
- `metadata.name` continues to use `PatternName`; it is not a Space validator.
- Invalid Space values fail before an HTTP host operation.
- Provider environment defaults are validated after selection and before host
  discovery.
- Terraform/OpenTofu state and URL query decoding retain embedded whitespace
  exactly.
- `/` cannot be part of `SpaceID`, so `SPACE/NAME` import parsing is
  unambiguous.
- Tightening the current `v1alpha1` contract is recorded and its public schema
  digest must be re-pinned before publication.

## Rejected alternatives

- **Reuse Resource `PatternName`.** Rejected because Space is an opaque scope
  identity, not a lowercase portable Resource name.
- **Trim values at each input boundary.** Rejected because trimming silently
  aliases distinct caller inputs and makes state or idempotency identity
  runtime-dependent.
- **Apply Unicode normalization or case folding.** Rejected because the host
  API promises exact opaque identity rather than a human-name comparison
  algorithm.
- **Allow `/` and escape the import separator.** Rejected because it adds a
  second import grammar for no portable identity benefit.

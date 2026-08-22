# 0041 — Takoform's Form Packages publish with the provider release that embeds them

- Status: Accepted
- Date: 2026-08-22

## Context

The question was whether the package axis should be merged into the provider's.
Measured, most of the merge has already happened, one release at a time:

- **A package has no version of its own to merge.** Content-addressed packages
  carry no SemVer — the package digest is the whole identity
  ([`../versioning.md`](../versioning.md)) — and the envelope is a manifest
  format tag, not an axis
  ([decision 0040](0040-the-package-envelope-is-a-format-not-an-axis.md)).
- **The provider release already fixes the package set.**
  [`../../release/provider-form-identities.json`](../../release/provider-form-identities.json)
  locks, append-only, the exact `packageDigest` of every Form a provider
  release embeds, and the release gate asserts that embedded set byte-equals
  the candidate set. Provider `v2.1.1` names all fifteen Beta package digests.
- **The provider release is the only vehicle that has ever shipped a current
  Form contract.** No v1alpha2 or Beta Form Package was ever published
  independently; the thirty-four independently published packages are the
  Legacy v1alpha1 epoch, retained as history. In the v1beta1 architecture a
  Form change requires a provider release anyway — the typed resources and
  codecs are compiled in, and a build fails closed on a ref it does not carry —
  so Forms already ride the provider train whether or not packages do.

So the repository maintains an independent package release channel that the
current epoch has never used, for artifacts whose exact digests every provider
release already locks.

## Decision

**Takoform publishes its own Form Packages with the provider release that
embeds them. There is no independent package release cadence.** One release
event: the provider ships, and the package artifacts it locked ship with it,
under the digests `provider-form-identities.json` already records. The
publication blockers continue to gate whether that happens at all; this decides
only that when it happens, it is not a second train.

**What is deliberately not merged is identity.** A package digest contains no
provider version and never will: the Stable criteria in
[`../project-lifecycle.md`](../project-lifecycle.md) require packages published
by a party other than a consuming host, and a third party publishing under its
own cadence needs exactly the provider-independent, content-addressed identity
packages already have. The release *train* is Takoform's own and merges; the
*identity* is the ecosystem's and does not.

The standing rule that axis numbers are never aligned is untouched — nothing
here renumbers anything. A cadence is not a number.

## Enforcement

Already standing, cited rather than rebuilt: `provider-form-identities.json` is
append-only and digest-frozen per release, and the release gate refuses a
provider candidate whose embedded Form and package identities are not
byte-equal to the candidate set. Those two checks are precisely "the packages a
release ships are the packages the provider embeds", which is this decision.

## Consequences

The version axes a user holds stay the three that
[decision 0040](0040-the-package-envelope-is-a-format-not-an-axis.md) named —
Host API lane, Form contract, provider SemVer — and the maintainer's release
axes shrink from two trains to one. `form-package-release` tooling remains the
mechanism that builds and signs package artifacts; it runs as a step of the
provider release when the blockers clear, not as a channel of its own.

A third-party publisher is unaffected and was the point of the carve-out: the
envelope format is open, the identity is content-addressed, and nothing in a
package says which provider release, if any, it rode.

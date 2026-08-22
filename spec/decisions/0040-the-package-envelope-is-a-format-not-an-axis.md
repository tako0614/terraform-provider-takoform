# 0040 — The package envelope names a manifest format, not a Form generation

- Status: Accepted
- Date: 2026-08-22

## Context

`packages.forms.takoform.com/vN` is the `apiVersion` of a package index: the
manifest that binds one exact FormRef to a closed byte inventory — paths, media
types, sizes, digests. Four values of it exist, which made it look like a fifth
version axis a reader had to hold.

Measured, it is not one. Normalising version words, the `v1alpha2`, `v1alpha3`
and `v1alpha4` schemas are **structurally identical** — the only differences
are each schema's own `$id`, its own `apiVersion` const, and the `$ref` to the
FormRef schema it wraps. The one real format change in the envelope's history
is `v1alpha1` to `v1alpha2`, which introduced content addressing
([decision 0005](0005-current-form-packages-use-content-addressed-locators.md)).
The other two mints followed the grammar of the FormRef they wrap: `v1alpha3`
for the retained provider-v2 epoch, `v1alpha4` for namespaced families.

And the envelope has already stopped following. `v1alpha4`'s FormRef grammar
admits any namespaced group, so when the Edge family moved from
`edge.forms.takoform.com/v1alpha1` to `/v1beta1` the envelope stayed put and
carried both. An identifier that does not move when the thing it was named
after moves is not tracking it — it just looks like it is, and the lingering
`v1alpha4`-wraps-`v1beta1` mismatch is one of the things that made "which
number is current" unanswerable by inspection.

The information the envelope version appeared to carry lives three lines below
it in the same document: `formRef.apiVersion`. An axis whose value is derivable
from the document it labels is not an axis.

## Decision

**A new envelope is minted only when the manifest format itself changes
structurally.** Never because a Form generation moved, a family was renamed, or
the FormRef grammar it wraps advanced — `v1alpha4` already proves the grammar
can move underneath a standing envelope.

This is [decision 0039](0039-a-lane-is-minted-for-one-of-two-reasons.md)'s rule
applied to the envelope, with one branch fewer: an envelope has no maturity
channel of its own, so the graduation reason does not exist for it. Format
change is the only minting reason there is.

**The four published values stay exactly as they are.** They are constants
inside published schema bytes, declared by 24 served package indexes, and
[decision 0037](0037-immutability-begins-at-stable.md) forbids a served
identity changing meaning. `v1alpha3` and `v1alpha4` are recorded as what they
were — re-mints that carried a grammar move, not format changes — the same way
`forms.takoform.com/v1beta1` is recorded in 0039's table as a graduation whose
evidence was never stated. History is recorded, not restated.

## Enforcement

The same gate shape as the Host API lanes, in
`scripts/check-public-surfaces.mjs`: every envelope is recorded with the reason
it was minted. A `format` envelope's schema must differ structurally from every
other `format` envelope's, version words normalised, so a rename cannot present
itself as a format; a `carried` envelope must say why it exists, because that
is not provable from bytes. Both refusals were watched failing first — claiming
`v1alpha3` was a format change fails against `v1alpha2`'s identical bytes, and
deleting a `carried` entry's reason fails by name.

## Consequences

The version axes a user of this project actually holds are three: the Host API
lane (the wire a provider speaks), the Form contract (group plus
`definitionVersion` plus digest), and the provider SemVer (the artifact
installed from the Registry). Interface and Binding contracts version with the
Forms that reference them. The envelope is a format tag on the manifest, and
the next Form generation will not mint one.

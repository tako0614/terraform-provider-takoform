# 0005 — Current Form Packages use content-addressed publication locators

- Status: superseded for current v1alpha2 Forms by
  [0006](0006-v1alpha2-restarts-form-lines.md); retained for immutable
  content-addressed packages carrying v1alpha1 Legacy FormRefs
- Date: 2026-08-04
- Owners: Takoform maintainers
- Supersedes: the current-package `packageVersion` decision in
  [0001](0001-provider-v1-keeps-form-versions-independent.md)

## Context

A published Form already has a semantic compatibility identity in its exact
`FormRef`, while a Form Package already has a byte identity in
`packageDigest`. The v1alpha1 package format added a second SemVer
`packageVersion` and reused it in the source directory, Git tag, release asset
names, and manifests. In practice it duplicated the Form version and made
package transport look like another maturity clock. It also allowed fixtures
or explanatory payload changes to compete with the Form contract's SemVer.

The 71 published v1alpha1 release identities, including their paths, tags,
manifests, signatures, and package indexes, are immutable Legacy evidence and
cannot be renamed or rewritten.

## Decision

New current packages use `packages.forms.takoform.com/v1alpha2`. The index has
no `packageVersion`; its exact `packageDigest` is the only Package Artifact
identity. A current publication locator is derived mechanically as:

```text
artifact ID  = sha256-<64 lowercase hex>
source path  = forms/releases/<release-id>/<artifact-id>
Git tag      = forms/<release-id>/<artifact-id>
```

`FormRef.definitionVersion` remains the Form contract's SemVer and retains its
compatibility meaning. The provider version remains independent. Publication,
Form maturity, Host Support, activation, and commercial availability remain
separate facts.

The verifier accepts both profiles through one package-verification interface:
v1alpha1 is a read/recovery compatibility profile with its exact SemVer
locator; v1alpha2 is the only profile accepted for new current lifecycle
records and publications. A caller must not infer a profile from directory
names or labels after parsing the signed canonical index.

## Consequences

- changing any indexed package payload produces a new Package Digest and a new
  immutable locator without pretending the Form contract changed;
- changing Form semantics still requires a new exact FormRef under the Form
  compatibility policy;
- the current lifecycle and release verifier can derive path, tag, and asset
  identity from one verified package result;
- all existing v1alpha1 readers, tags, source paths, release assets, signatures,
  revocations, and recovery evidence remain valid and unchanged;
- hosts add v1alpha2 support before a real current package is published; there
  is no flag day and no reinterpretation of persisted v1alpha1 records.

## Rejected alternatives

- **Keep `packageVersion` but document that it is meaningless.** Rejected
  because a required public SemVer is inevitably treated as a compatibility or
  maturity signal and still creates two clocks.
- **Reuse `FormRef.definitionVersion` as the package locator.** Rejected because
  more than one exact package artifact can carry the same Form Definition while
  improving fixtures, evidence, or non-semantic documentation.
- **Put only a shortened digest in the locator.** Rejected because collision
  handling would introduce another registry rule; the full SHA-256 is already
  the canonical identity.

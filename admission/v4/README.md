# Historical admission v4 material

`admission/v4` is retained evidence from the pre-reset `ga-core-v2` admission
experiment. It is not a current candidate lane, approval authority, or host
activation policy.

The final admission identities actually published were
`forms/admissions/v1.0.6` and `forms/admissions/v1.0.7`. Their annotated tag
objects, commits, exact v4 subtrees, and set digests are pinned by
[`../admission-identities.json`](../admission-identities.json). Verification
reads those historical Git objects; it does not assume that this mutable
working-tree directory is byte-identical to the tagged tree.

Identity `1.0.5` was never published and remains permanently
`reserved-abandoned`; it may not be reassigned. The retained
`standard`, `portable-standard`, candidate, and admission fields describe what
the old experiment asserted. They do not define current Form maturity, a
centrally approved subset, Host Support, availability, or activation.

The central candidate file, material builder, signing workflow, and admission
deploy surface have been removed. New Forms use the project lifecycle in
[`../../forms/lifecycle.json`](../../forms/lifecycle.json): Proposal,
Experimental, Stable, then Legacy. Hosts independently publish support for
exact FormRefs and apply their own activation policy.

Use the history-only checks from the repository root:

```console
go run ./cmd/standard-form-conformance legacy-published-package-check
go run ./cmd/standard-form-conformance legacy-admission-evidence-check
```

These checks prove immutable package publication and the closed historical tag
ledger. They deliberately do not rerun old semantic approval claims under the
current provider, catalog, or lifecycle rules.

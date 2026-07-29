# Current portable admission material

`admission/v3` is the retained, generation-aware admission lane for the exact
`ga-core-v1` ten-Form subset of the current `portable-v1` catalog.

The package releases are independent immutable per-Form releases. Provider
reports close over all 34 current Forms before this lane selects the exact ten
admission identities. The final v3 set retains and offline-authenticates that
full 34-report provider closure rather than discarding the 24 unselected
proofs. Host reports close over those ten identities. Registry
readback separately proves both Terraform and OpenTofu can install the same
canonical provider source.

This directory is fail-closed while signed provider, host, Registry, and
admission-evidence candidates are absent. Local structure, package publication,
or an unsigned report never grants portable-standard admission.

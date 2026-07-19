# Form Package release sources

Each child directory is one exact data-only release source:

```text
<release-id>/<packageVersion>/package-index.json
<release-id>/<packageVersion>/<listed payloads>
```

The directory must pass `go run ./cmd/form-package verify`. Its release ID is
`k-` followed by the lowercase, unpadded base32 encoding of the exact ASCII
FormRef Kind bytes. This encoding is reversible and preserves distinctions such
as `SQLDatabase` versus `SqlDatabase`. Its version must equal `packageVersion`
and the `forms/<release-id>/v<packageVersion>` tag. Compatibility and test fixtures stay
under `conformance/`; copying one here is an explicit reviewed release
decision, not automatic standardization.

The ten `1.0.0` directories currently staged here are the release-owned source
copies of the exact structural candidates in
`conformance/form-package-v1/positive/standard/`. The
`standard-form-conformance verify` gate compares every index and payload byte
to its reviewed fixture source, then verifies the exact FormRef and package
digest. Fixture regeneration never updates these directories automatically.
Their presence makes the protected release workflow reproducible; it does not
change `forms/standard-package-set.json` from `structural-candidate` or satisfy
signature, Registry readback, host/provider lifecycle, or signed admission
requirements.

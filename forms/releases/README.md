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

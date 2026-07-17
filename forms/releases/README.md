# Form Package release sources

Each child directory is one exact data-only release source:

```text
<form-slug>/<packageVersion>/package-index.json
<form-slug>/<packageVersion>/<listed payloads>
```

The directory must pass `go run ./cmd/form-package verify`. Its slug must be
the kebab-case FormRef kind and its version must equal `packageVersion` and the
`forms/<form-slug>/v<packageVersion>` tag. Compatibility and test fixtures stay
under `conformance/`; copying one here is an explicit reviewed release
decision, not automatic standardization.

# Pinned release evidence schemas

`spdx-2.3.schema.json` is the SPDX project's official JSON Schema from tag
`v2.3`:

https://raw.githubusercontent.com/spdx/spdx-spec/refs/tags/v2.3/schemas/spdx-schema.json

The candidate builder validates every generated SBOM against this pinned copy.
SLSA Provenance v1 and its in-toto Statement envelope are validated with the
official `github.com/in-toto/attestation` validators pinned in the isolated
`cmd/provider-release` module.

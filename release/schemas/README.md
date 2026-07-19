# Pinned release evidence schemas

`spdx-2.3.schema.json` is the SPDX project's official JSON Schema from tag
`v2.3`:

https://raw.githubusercontent.com/spdx/spdx-spec/refs/tags/v2.3/schemas/spdx-schema.json

The Form Package and provider candidate tests validate every generated SBOM
against this pinned copy without a network lookup. Form Package admission adds
the stricter release contract on top: the document must `DESCRIBE` the one
package, and that package must `CONTAIN` the index plus every payload file in
the deterministic file order. Omission, substitution, duplication, an extra
relationship, or reordering fails closed even if the generic SPDX schema would
accept it.
SLSA Provenance v1 and its in-toto Statement envelope are validated with the
official `github.com/in-toto/attestation` validators pinned in the isolated
`cmd/provider-release` module.

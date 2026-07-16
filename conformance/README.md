# Conformance

The current Phase 0 evidence is executable Go characterization:

- `internal/provider/*_test.go` asserts the exact ten-resource registration, typed schema behavior, validation, CRUD, import, state refresh, and the absence of plan-time remote mutation;
- `internal/client/client_test.go` asserts discovery, capability negotiation, preview/apply evidence, error envelopes, observation, and deletion;
- `examples/resources/` contains one formatted HCL example for every registered resource.

Run:

```console
go test ./...
go vet ./...
tofu fmt -check -recursive examples
go run ./cmd/conformance verify
```

## Phase 0/1 golden characterization

`compatibility-candidate-v1/` freezes the current ten-kind provider/client
behavior as deterministic offline evidence. It contains JSON Schemas and
fixtures for the provider schema, desired and observed Resource envelopes,
sanitized output projection, provider import IDs, provider/API errors, and host
discovery. `cmd/conformance` rejects file or per-kind digest drift, an incomplete
kind set, malformed evidence, publication-ready claims, and portable-standard
claims.

The case digest uses Go `encoding/json` normalization only to detect fixture
drift. It is not a portable canonical serialization algorithm and is not a
definition identity. This evidence is neither a signed package nor a standard
form release. The actual provider and HTTP client parity tests consume these
fixtures so checked-in evidence cannot drift away from the current executable
wire/default/validation behavior.

## Not implemented yet

A portable conformance runner and cross-language canonicalization do not yet
exist. The candidate evidence above characterizes only the current provider and
client implementation. A later contract phase must separately define and test
signed-package identity, unavailable and unauthorized definitions, stale
versions, digest mismatch, secret rejection, lifecycle idempotency, and package
retention/revocation before any portable contract can be claimed.

No empty directory or README in this scaffold should be interpreted as signed-package, provenance, or cross-host conformance evidence.

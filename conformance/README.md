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
```

## Not implemented yet

A portable conformance runner and golden cross-language fixtures do not yet exist. The next contract phase must add positive and negative fixtures for host discovery, exact FormRef/digest pinning, desired and observed schemas, provider-schema parity, unavailable and unauthorized forms, stale versions, digest mismatch, secret rejection, lifecycle idempotency, import/observe behavior, and package retention/revocation.

No empty directory or README in this scaffold should be interpreted as signed-package, provenance, or cross-host conformance evidence.

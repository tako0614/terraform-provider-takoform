# Form Definition boundary

A future portable Form Definition is a data-only, deterministic description of one Service Form. Its stable identity is expected to be a `FormRef` containing:

- `apiVersion`;
- `kind`;
- `definitionVersion`;
- `schemaDigest` over canonically serialized definition bytes.

A definition may eventually describe desired-spec and observed-output schemas, immutable fields, lifecycle capabilities, non-secret Interface descriptors, code-generation metadata, presentation hints, and conformance fixture references.

It must never contain credentials, secret values, target or pool IDs, account IDs, active region capacity, backend-manager identities, prices, SKUs, quota, billing, SLA, support policy, or arbitrary executable code.

## Phase 0 status

No canonical Form Definition schema or signed Form Package is committed yet. The exact ten schemas currently compiled into the provider remain characterization inputs, not automatically blessed standards. Before publishing definition packages, the project must specify canonical serialization, digest and signature formats, trust roots, publisher policy, key custody/rotation/revocation, package retention, and negative conformance behavior.

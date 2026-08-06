# Decision 0001 — Version provider runner reports separately

- Status: Accepted
- Date: 2026-07-29

## Context

The original `takoform.standard-runner-report@v1` format is shared by host and
provider evidence and is strictly decoded. Adding a provider binary digest to
that object would make newly signed bytes invalid to an existing v1 verifier
because unknown fields fail closed.

Provider report tooling can also advance after a provider release without
changing the provider executable. Comparing the tooling source commit to the
provider release commit therefore rejects valid evidence and does not prove
which bytes actually ran.

## Decision

Host reports keep `takoform.standard-runner-report@v1`. Historical provider v1
reports remain readable for their retained admission generation.

New provider reports use
`takoform.standard-provider-runner-report@v2` and require
`providerBinarySha256`. Current admission requires one digest across the full
provider report closure and exact equality with every direct Registry
installation readback.

The machine-readable contract lock is `spec/trust/profile.json` schema v2.
Unknown fields remain rejected for both formats.

# Mounted Host API v2 native process qualification

This fixture uses the actual OpenTofu CLI, a compiled `New("5.0.0")` Provider plugin, and a local HTTPS Host API v2 fixture. Its source pin is `7ade7034eb05c721b0dbdbd90aab14aa2b176d9f` (the mounted v2 source including the Space-default correction), not the historical callback-only `intent-custody-probe` provider. No production Host or Provider release is contacted.

## Run

From this Provider worktree, with OpenTofu 1.12.3, Go 1.26.0, the pinned local modules, and a writable Go cache:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOCACHE=/root/hdd/go-cache \
  go test -count=1 -v ./native-fixture/mounted-v2
```

Observed: four subtests passed in **9.681 s** (including local plugin and `intent-store-init` builds), OpenTofu `v1.12.3 linux_amd64`, Go `go1.26.0 linux/amd64`. The Provider binary uses the repository's `main.go` with version `5.0.0` set at build time. `tofu init -plugin-dir` installs only the local temporary plugin. The test uses a unique temporary module and a separately preinitialized, exact-root-bound intent store for each case. Its TLS certificate is trusted only by child CLI/Provider processes through `SSL_CERT_FILE`; no global trust or insecure-TLS setting is changed.

The minimal HCL under test has `provider "takoform"` with local HTTPS `endpoint` and `host_api_root`, `token = var.token`, the absolute store path, `store_id = "fixed-scope"`, and `space = "prod"`; `takoform_module_worker.x` has only `name = "app"`. The token variable is ephemeral and supplied through this process's `TF_VAR_token`. The separate update case uses typed `takoform_at_least_once_queue` with `message_retention_seconds` 60 then 120, since ModuleWorker has an empty spec and cannot honestly demonstrate an in-place update.

## Evidence

| Case | Observed CLI/Host boundary |
| --- | --- |
| Ordinary | ModuleWorker create, exact-UID read/refresh, and delete succeeded; Host saw one Create and one Delete; state was empty after destroy. |
| Post-ACK process exit | A **test-only Go build overlay** inserts `os.Exit(73)` immediately after successful `store.RecordAccepted` in `v2Resource.Create` and before its state response. The Provider child wrote its PID marker; it differed from the OpenTofu CLI PID. CLI apply failed with `Plugin did not respond`, `tofu state list` stayed empty, and the retained journal contained `op-1`. A **new CLI invocation** applied the same config, read the exact Operation and Resource, restored `uid-exact-1`, and completed without a second Host mutation: one Create total. It then refreshed and deleted normally. |
| Unknown pre-ACK | The Host accepted the first Create locally but returned an invalid Operation ACK. The Provider kept its claimed intent with no Operation ID and failed. A fresh CLI apply refused with `Unknown pre-ACK v2 outcome`; the Host saw one Create total, not a retry or new key. |
| Typed update | A separate exact Queue Form was created at retention 60, updated in place to 120 under the generation fence, then deleted; the Host saw exactly three mutations. |

The fixture scans the saved plan, Terraform state where present, lockfile, and every intent-store file for the fixture bearer value; none contained it. This is evidence about this credential and these artifacts, not a proof that every private-input variant is absent. No WorkerVersion `required_sensitive_vars` or continuation was exercised.

An earlier native run against source `c3ebd3c12d0cbb76de988169118182360c78e071` caught a real usability failure: with provider `space = "prod"` but no resource `space`, OpenTofu planned resource Space as unknown and Create failed `Invalid v2 plan: space must be known` before HTTP. Adding explicit resource Space made the provisional custody proof pass, but did not satisfy the requested minimal HCL. The source owner corrected the Provider's `ModifyPlan` defaulting in `7ade7034`; this report's final run removed that workaround and passed the original minimal shape.

## Limits

The Host is a deterministic local HTTPS fixture, not Takoserver or another live Host. It does not prove Host v2 conformance, principal continuity under token rotation, live authorization, private-input delivery, a normal Takosumi Runner's persistent store custody, power-loss durability, remote backend state behavior, registry packaging/publication, or release/GA readiness. The exit injection is **not** in the production Provider build; the test constructs the overlay only in a temporary directory. The unknown-ACK case models an invalid response after a Host-side effect; it does not model every transport timing or Host reconciliation path. These conclusions apply to the exact source and toolchain above.

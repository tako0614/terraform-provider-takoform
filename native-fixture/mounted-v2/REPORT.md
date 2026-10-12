# Mounted Host API v2 native process qualification

This fixture uses the actual OpenTofu CLI, a compiled `New("5.0.0")` Provider plugin, and a local HTTPS Host API v2 fixture. Its final source pin is `1bbba0b5aa30d2864c9fffc1d392a9251eae3bd4`, not the historical callback-only `intent-custody-probe` provider. No production Host or Provider release is contacted.

## Run

From this Provider worktree, with OpenTofu 1.12.3, Go 1.26.0, the pinned local modules, and a writable Go cache:

```sh
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOCACHE=/root/hdd/go-cache \
  go test -tags native -count=1 -v ./native-fixture/mounted-v2
```

This test file is guarded by the `native` build tag. The portable `go test ./...`
package inventory excludes this OpenTofu-dependent fixture; native qualification
must be invoked explicitly and must fail if its required `tofu` binary is absent.

Observed with the explicit `native` tag: all five subtests passed together in **8.267 s**, including local plugin and `intent-store-init` builds. Toolchain: OpenTofu `v1.12.3 linux_amd64`, Go `go1.26.0 linux/amd64`. The Provider binary uses the repository's `main.go` with version `5.0.0` set at build time. `tofu init -plugin-dir` installs only the local temporary plugin. The test uses a unique temporary module and a separately preinitialized, exact-root-bound intent store for each case. Its TLS certificate is trusted only by child CLI/Provider processes through `SSL_CERT_FILE`; no global trust or insecure-TLS setting is changed.

The minimal HCL under test has `provider "takoform"` with local HTTPS `endpoint` and `host_api_root`, `token = var.token`, the absolute store path, `store_id = "fixed-scope"`, and `space = "prod"`; `takoform_module_worker.x` has only `name = "app"`. The token variable is ephemeral and supplied through this process's `TF_VAR_token`. The separate update case uses typed `takoform_at_least_once_queue` with `message_retention_seconds` 60 then 120, since ModuleWorker has an empty spec and cannot honestly demonstrate an in-place update.

## Evidence

| Case | Observed CLI/Host boundary |
| --- | --- |
| Ordinary | ModuleWorker create, exact-UID read/refresh, and delete succeeded; Host saw one Create and one Delete; state was empty after destroy. |
| Post-ACK process exit | A **test-only Go build overlay** inserts `os.Exit(73)` immediately after successful `store.RecordAccepted` in `v2Resource.Create` and before its state response. The Provider child wrote its PID marker; it differed from the OpenTofu CLI PID. CLI apply failed with `Plugin did not respond`, `tofu state list` stayed empty, and the retained journal contained `op-1`. A **new CLI invocation** applied the same config, read the exact Operation and Resource, restored `uid-exact-1`, and completed without a second Host mutation: one Create total. It then refreshed and deleted normally. |
| Unknown pre-ACK | The Host accepted the first Create locally but returned an invalid Operation ACK. The Provider kept its claimed intent with no Operation ID and failed. A fresh CLI apply refused with `Unknown pre-ACK v2 outcome`; the Host saw one Create total, not a retry or new key. |
| Typed update | A separate exact Queue Form was created at retention 60, updated in place to 120 under the generation fence, then deleted; the Host saw exactly three mutations. |
| Process-local private input and semantic public JSON | Exact WorkerVersion 0.5.0 typed HCL declares `required_sensitive_vars = ["API_TOKEN"]` and noncanonical public `vars_json = "{\"z\":1.0,\"a\":2}"`; the Host returns the same public value as `{"a":2,"z":1}`. Create and Delete succeed without an OpenTofu inconsistent-result error. The Provider receives the private value in `TAKOFORM_V2_PRIVATE_INPUTS_JSON`, and the HTTPS Host fixture confirms receipt in `privateInputs`. Decoded `tofu show -json saved.tfplan`, decoded `tofu show -json` state after Create, and decoded `state.json` intent journal contain neither the private value nor bearer token. |

The fixture also scans raw saved-plan, Terraform-state, lockfile, and every intent-store file for both fixture values. Raw-byte scans alone would not establish absence from OpenTofu's ZIP-like plan, so the WorkerVersion case additionally decodes the plan and state through OpenTofu and parses journal JSON. This is evidence about these values and artifacts, not every private-input variant or an accepted Operation's `waiting_input` continuation.

An earlier native run against source `c3ebd3c12d0cbb76de988169118182360c78e071` caught a real usability failure: with provider `space = "prod"` but no resource `space`, OpenTofu planned resource Space as unknown and Create failed `Invalid v2 plan: space must be known` before HTTP. Adding explicit resource Space made the provisional custody proof pass, but did not satisfy the requested minimal HCL. The source owner corrected the Provider's `ModifyPlan` defaulting in `7ade7034`; this report's final run removed that workaround and passed the original minimal shape.

The noncanonical WorkerVersion case failed on source `7ade7034` with the real CLI diagnostic `Provider produced inconsistent result after apply`: `.spec.vars_json` changed from authored `{"z":1.0,"a":2}` to Host-canonical `{"a":2,"z":1}`. A first integrated candidate `6af855bb` was also rejected by OpenTofu before Host access with ten `Schema Using Attribute Default For Non-Computed Attribute` diagnostics. The final pin contains the codec and schema corrections; the same five native cases then passed.

## Limits

The Host is a deterministic local HTTPS fixture, not Takoserver or another live Host. It does not prove Host v2 conformance, principal continuity under token rotation, live authorization, a normal Takosumi Runner's private-input delivery or persistent store custody, power-loss durability, remote backend state behavior, registry packaging/publication, or release/GA readiness. The exit injection is **not** in the production Provider build; the test constructs the overlay only in a temporary directory. The unknown-ACK case models an invalid response after a Host-side effect; it does not model every transport timing or Host reconciliation path. These conclusions apply to the exact source and toolchain above.

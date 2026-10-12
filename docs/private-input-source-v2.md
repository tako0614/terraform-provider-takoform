# Provider v2 process-local private-input source

`internal/privateinputsv2` is an internal Provider source helper for a future
Host v2 WorkerVersion caller. It is not mounted in a Terraform resource,
registered as a Provider capability, or a public API. It reads
`TAKOFORM_V2_PRIVATE_INPUTS_JSON` only when the caller explicitly asks for
inputs for one exact Host `space` and resource `name`:

```json
{
  "production": {
    "worker-version": {
      "API_TOKEN": "replace-with-runtime-value"
    }
  }
}
```

The selected keys must equal the caller's expected `requiredSensitiveVars`
names exactly. The helper returns only those values and writes, logs, persists,
or hashes none of them; it keeps no package-global copy. The source is bounded
to 1 MiB. Each value is limited to 1..32768 UTF-8 bytes with no NUL by the
Provider's [run-scoped runtime-input policy](resources/worker_version.md#run-scoped-sensitive-inputs);
WorkerVersion Form independently requires nonempty strings but sets no
value-length or NUL rule. `Load` itself
is a generic selector and does not enforce Form-specific declaration counts.
For WorkerVersion 0.5.0, `requiredSensitiveVars` is limited to 64 names by the
exact published Form. Callers must pass names from an already validated
`edgev2` spec; the helper's 65-name selection test is not evidence of a
Form-valid spec. The 1 MiB source limit bounds the overall input. JSON must be
strict UTF-8 with no duplicate keys or invalid Unicode. Diagnostics are generic
and do not include values or input names.

The caller must read this source only in its Apply-time or explicit
private-input-continuation path, never while planning. This helper does not
create a Host operation, retry or replay one, or decide whether a value matches
an earlier submission. If a Host has already accepted a WorkerVersion
operation and requests private-input continuation, the caller must continue
that exact Operation using the existing Host v2 client and its entire original
map. Host authorization and value equality remain Host decisions.

The environment source is process-local input, not durable storage or a
transport guarantee. Callers and runners remain responsible for controlling
which process receives it and for not placing its values in plans, state,
journals, logs, diagnostics, command arguments, or persistent files. This
package makes no process-memory erasure claim.

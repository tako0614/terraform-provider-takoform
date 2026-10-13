# Provider v2 process-local private-input source

`internal/privateinputsv2` supplies process-local values to the Provider's Host
v2 WorkerVersion Apply and accepted-Operation continuation paths. It is an
internal helper, not an HCL attribute or published API. Those paths call
`Load` or `LoadOptional` only after the WorkerVersion desired spec has passed
`edgev2` validation; planning does not read this source. It reads
`TAKOFORM_V2_PRIVATE_INPUTS_JSON`
only when a caller requests values for one exact Host `space` and resource
`name`:

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
names exactly. A fresh Create and accepted `waiting_input` require the full map
through `Load`. An explicit Update uses `LoadOptional`: if no map is selected
for that exact Space/name, the Provider omits `privateInputs` and the Host
retains prior values; a present but malformed or mismatched map is refused.
The helper returns only selected values and writes, logs, persists,
or hashes none of them; it keeps no package-global copy. The source is bounded
to 1 MiB. Each value is limited to 1..32768 UTF-8 bytes with no NUL by the
shared [runtime-input value validator](https://github.com/tako0614/terraform-provider-takoform/blob/main/internal/provider/v3_runtime_inputs.go);
WorkerVersion Form independently requires nonempty strings but sets no
value-length or NUL rule. `Load` itself
is a generic selector and does not enforce Form-specific declaration counts.
For WorkerVersion 0.5.0, `requiredSensitiveVars` is limited to 64 names by the
exact published Form. Callers must pass names from an already validated
`edgev2` spec; the helper's 65-name selection test is not evidence of a
Form-valid spec. The 1 MiB source limit bounds the overall input. JSON must be
strict UTF-8 with no duplicate keys or invalid Unicode. Diagnostics are generic
and do not include values or input names.

For an accepted Operation in `waiting_input`, the caller reloads the source
and passes the entire original map to the existing Host v2 client for that
same Operation. The helper does not retain or prove the original values; the
Host authorizes the continuation and checks equality. If the original map is
unavailable, the caller must stop and reconcile that Operation rather than
creating a new one or replaying a mutation.

The environment source is process-local input, not durable storage or a
transport guarantee. Callers and runners remain responsible for controlling
which process receives it and for not placing its values in plans, state,
journals, logs, diagnostics, command arguments, or persistent files. The
intent journal may retain private-input names, but not values or value hashes.
This package makes no process-memory erasure claim. Its focused Go tests verify
local source selection and validation only; they do not qualify native
OpenTofu behavior, Runner input delivery or non-persistence, or live Host
interoperability/support.

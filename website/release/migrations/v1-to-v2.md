# Provider v1 to v2 migration

Provider v1 and provider v2 use the same Registry address but implement
different Form epochs.

| Provider | Form epoch | Status | Purpose |
| --- | --- | --- | --- |
| `1.x` | `forms.takoform.com/v1alpha1` | published Legacy | recovery and existing state |
| `2.x` | `forms.takoform.com/v1alpha2` | source candidate | current nine-Form line |

This boundary is intentionally fail-closed. Provider v2 does not upgrade v1
state in place because definition and package identities changed and the new
contract may have different semantics even when the Terraform resource type is
the same.

1. Pin provider v1 and refresh the Legacy resource.
2. Capture non-secret desired configuration and required public outputs.
3. Create the corresponding v1alpha2 resource under provider v2, or import a
   host resource only when the host proves it conforms to the exact v1alpha2
   FormRef.
4. Move consumers to the new resource and verify host observation.
5. Delete the Legacy resource through provider v1 when rollback is no longer
   required.

Do not edit state JSON, substitute FormRef digests, or use an automatic state
upgrader. If recovery is needed, return to the exact provider-v1 constraint and
the retained Legacy host lane.

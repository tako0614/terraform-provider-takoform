# Provider v1 to v2 migration

Provider v1 and provider v2 use the same Registry address but implement
different Form epochs.

| Provider | Form epoch | Status | Purpose |
| --- | --- | --- | --- |
| `1.x` | `forms.takoform.com/v1alpha1` | published Legacy | recovery and existing state |
| `2.x` | `forms.takoform.com/v1alpha2` | published current | retained nine-Form v1alpha2 line |

Provider v2.1 and later additionally carry the current Experimental Edge
Platform Family over Beta Host API `forms.takoform.com/v1beta1`. Provider
v2.1.1 is a stable release target whose descriptor remains `candidate-only`
until the release owner publishes it
([decision 0035](../../spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).
Existing Beta state remains bound to its exact Beta FormRef and codec when a
future Stable `1.0.0` identity becomes the create default; refresh never
performs that migration implicitly.

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

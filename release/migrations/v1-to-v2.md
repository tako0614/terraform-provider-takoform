# Provider v1 to v2 migration

Provider v1 and provider v2 use the same Registry address but implement
different Form epochs.

| Provider | Form epoch | Status | Purpose |
| --- | --- | --- | --- |
| `1.x` | `forms.takoform.com/v1alpha1` | published Legacy | recovery and existing state |
| `2.0.0` | `forms.takoform.com/v1alpha2` | published compatibility predecessor | retained nine-Form v1alpha2 line |
| `2.1.1` | `forms.takoform.com/v1beta1` | Registry-published retained Provider 2 release; Beta Host API and Experimental Forms | retained Edge Form Family line |

Provider v2.1 carries the retained Experimental Edge Platform Family over Beta
Host API `forms.takoform.com/v1beta1`. Provider v2.1.1 was the last Provider 2
release and remains Registry-published immutable history; Provider 3.0.0 is the
current published provider. The v2.1.1 release descriptor remains
`candidate-only` metadata by design after owner publication, while its Registry
publication is independently verified by readback
([decision 0035](../../spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)).
Existing Beta state remains bound to its exact Beta FormRef and codec when a
future Stable `1.0.0` identity becomes the create default; refresh never
performs that migration implicitly.

This guide records the v1-to-v2 boundary. A later upgrade must also follow the
[Provider v2 to v3 migration boundary](v2-to-v3.md).

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

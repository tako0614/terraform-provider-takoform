# Documentation

Get from zero to a running resource, then choose the lane that matches your
job.

## Quick start

The current line is provider `v2.0.0`, published in the Registry. To see the
provider and all nine resources exercised together, run the repository
conformance matrix:

```sh
bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform
```

The matrix proves preview/apply/observe/refresh/delete against the exact
v1alpha2 contracts without touching a real host. Against a real host, first
verify it advertises the exact v1alpha2 FormRefs at its versioned discovery
path (`/.well-known/takoform/v1alpha2`).

### Pinning the current provider

Pin provider `v2.0.0` to use the current line. `init` installs it from the
Registry.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 2.0.0"
    }
  }
}
```

## Lanes

One provider address, `registry.terraform.io/tako0614/takoform`, serves two
lanes:

| Lane | Use for | Install |
| --- | --- | --- |
| **v1.0.3** (published) | existing Legacy state, refresh, delete, recovery | from the Registry |
| **v2.0.0** (current) | the nine current contracts | from the Registry |

### Maintain published Legacy

For existing v1 state, pin the published provider `v1.0.3`. It does not turn
that state into v2 semantics.

```hcl
terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}
```

### Migrate from v1

Migration is an explicit create/import, never an automatic state rewrite.
Provider v2 refuses provider-v1 state.

1. Pin provider v1 and refresh the Legacy resource.
2. Capture non-secret desired configuration and required public outputs.
3. Create under the exact v1alpha2 FormRef, or import only with host
   conformance proof.
4. Move consumers, observe the result, then delete Legacy through v1 after
   rollback is no longer needed.

## Resource reference

Each page documents the arguments, read-only attributes, declared interfaces,
and import behavior of one resource:

- [edge_worker](/docs/resources/edge_worker.html)
- [relational_database](/docs/resources/relational_database.html)
- [object_bucket](/docs/resources/object_bucket.html)
- [key_value_store](/docs/resources/key_value_store.html)
- [queue](/docs/resources/queue.html)
- [schedule](/docs/resources/schedule.html)
- [container_service](/docs/resources/container_service.html)
- [stateful_entity](/docs/resources/stateful_entity.html)
- [vector_index](/docs/resources/vector_index.html)
- [interface data source](/docs/data-sources/interface.html)

## Host boundary

Takoform owns workload semantics, schemas, exact identities, packages, and
conformance. Hosts own capability support, placement, routing, scaling,
credentials, and recovery. Takosumi Cloud owns managed capacity, billing,
quota, and SLA.

<div class="status-note">

Takoform is an **Experimental specification project**. Current FormRefs use
`forms.takoform.com/v1alpha2` and current package envelopes use
`packages.forms.takoform.com/v1alpha3`. Provider `v2.0.0` is the current
published client; provider `v1.0.3` is the published Legacy client. The 34
published Form Package identities from `forms.takoform.com/v1alpha1` are
immutable Legacy evidence. There is no current central approval or admission.

</div>

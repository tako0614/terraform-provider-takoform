# `worker-app`

The official Takoform module for one running ES Module Worker. It is the
surface an ordinary author should use; the raw Forms stay available and stay
the low-level surface.

```hcl
module "worker_app" {
  source      = "takoform/worker-app/takoform"
  name        = "counter"
  main_module = "index.js"
  content_dir = "${path.module}/dist"
  kv_bindings = { COUNTER = takoform_edge_kv_namespace.counter.name }
  endpoint    = true
}
```

## What it assembles

| Resource                     | Role       | Named by                                   |
| ---------------------------- | ---------- | ------------------------------------------ |
| `takoform_module_worker`     | identity   | `var.name` — stable across every deploy    |
| `takoform_worker_bundle`     | revision   | derived `bundle-<manifest digest prefix>`  |
| `takoform_worker_version`    | revision   | derived `version-<spec digest prefix>`     |
| `takoform_worker_deployment` | deployment | `<name>-deployment` — updated, never replaced |
| `takoform_worker_endpoint`   | attachment | `<name>-endpoint`, when `endpoint = true`  |

## Why the revisions are named from their content

A Worker Bundle and a Worker Version are immutable revisions: a host refuses
every update to one, so a code change is a REPLACEMENT. Under a name the author
pins, that replacement completes in neither order —

- destroy-then-create fails the destroy with `dependency_in_use` (409), because
  the version still executes the bundle and the deployment still weights the
  version;
- `create_before_destroy` fails the create with `invalid_argument` (400),
  because the name is still occupied.

So the module writes no `name` on either revision. The provider derives one from
the revision's own content, and changed bytes are therefore a different resource
rather than a replacement of the same one. Both revisions additionally declare
`create_before_destroy`, which is what orders the apply as

```
create bundle-<new> → create version-<new> → re-weight the deployment
→ destroy version-<old> → destroy bundle-<old>
```

The deployment is updated in place throughout, so there is no instant at which
the worker has no active deployment.

## Secrets

`vars` is portable desired state and is persisted in Terraform state. Put no
secret value in it. `required_sensitive_vars` carries only the NAMES of secret
slots the worker requires; the values are host-owned and never enter desired
state.

## Publication

The address in the example above is the module's registry address. This
repository carries the module's source so the provider's own gates
(`tofu validate`, `terraform validate`, and the end-to-end authoring
conformance run) exercise the exact bytes an author would consume.

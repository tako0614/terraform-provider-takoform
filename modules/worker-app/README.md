# `worker-app`

The repository module for one running ES Module Worker. It is the
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

For an explicit Provider-side operation identity, pass the optional
`apply_idempotency_key`. It is forwarded only to the immutable
`takoform_worker_version` resource; the Provider validates and sends it as the
Host API `Idempotency-Key` header, keeps it in Terraform state, and never puts
it in the portable Form spec. This optional argument requires Provider 4.0.0 or
later:

```hcl
module "worker_app" {
  source = "takoform/worker-app/takoform"

  name                  = "counter"
  content_dir           = "${path.module}/dist"
  apply_idempotency_key = "counter-release-v1"
}
```

Changing the key replaces the immutable Worker Version. Omitting it preserves
the Provider's deterministic operation key and existing derived revision name.

## What it assembles

| Resource                     | Role       | Named by                                   |
| ---------------------------- | ---------- | ------------------------------------------ |
| `takoform_module_worker`     | identity   | `var.name` — stable across every deploy    |
| `takoform_worker_bundle`     | revision   | derived `bundle-<manifest digest prefix>-<owner digest prefix>` |
| `takoform_worker_version`    | revision   | derived from spec, owner, and optional apply key |
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
rather than a replacement of the same one.

A derived name also has to say WHOSE revision it is. A content digest names the
bytes, and two instances of this module built from identical output hold
identical bytes — so a name derived from content alone would hand one host
address to two Terraform resources, and the first destroy would break the other
one. The module therefore sets `revision_owner = var.name` on both revisions.
That is what `var.name` is for beyond the worker's own identity, and it is why
the name you choose has to stay distinct inside a space.

Both revisions additionally declare `create_before_destroy`, which is what
orders the apply as

```
create bundle-<new> → create version-<new> → re-weight the deployment
→ destroy version-<old> → destroy bundle-<old>
```

The deployment is updated in place throughout, so there is no instant at which
the worker has no active deployment.

## Secrets

`vars` is portable desired state and is persisted in Terraform state. Put no
secret value in it. Its values keep their own JSON types — `{ enabled = true,
retries = 3, label = "prod" }` reaches the worker as a boolean, a number, and a
string — so the variable is typed `any` rather than as a collection with one
inferred element type. `required_sensitive_vars` carries only the NAMES of secret
slots the worker requires; the values are host-owned and never enter desired
state.

## External services

`external_services` declares opaque standard-service slots without carrying a
URL, endpoint, credential, target identifier, or provider-specific configuration:

```hcl
external_services = [
  { name = "MEDIA", protocol = "com.amazonaws.s3", required = true },
]
```

The Host resolves the protocol out of band and answers support at plan time.
An unsupported required slot is refused; an optional unsupported slot is
omitted from readiness. The module does not translate legacy provider-specific
bindings into this surface.

## Publication

The address in the example above is the module's registry address. This
repository carries the module's source so the provider's own gates
(`tofu validate`, `terraform validate`, and the end-to-end authoring
conformance run) exercise the exact bytes an author would consume.

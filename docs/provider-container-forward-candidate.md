# Unpublished Container forward Provider candidate

This source-only candidate gives OpenTofu typed schemas for the exact local
`ContainerService` and `VectorIndex` Forms, plus the matching WorkerVersion
0.6.0-dev.1 / WorkerDeployment 0.5.0-dev.1 projection. Its WorkerVersion keeps
the seven existing Actor-era bindings and appends the exact `edge.vector` and
`container.http` contracts, for nine binding kinds total.

The candidate input is the unmodified JSON output of
`go run ./cmd/container-http-candidate` in the pinned
`takoform-forms` commit `35a77bdb37483fccbb365998923824040dad67cc`. The
Provider-owned HCL projection is separate from that Form JSON; it does not
rewrite or publish the Form, Interface, or Binding. The source digest closure
and focused tests reject identity or binding-roster drift.

`provider.New()` and the normal Provider resource catalog remain unchanged.
The current released Provider still carries `worker.actor@1.0.0` and
`worker.runtime@1.1.0`. This candidate pins the source WorkerVersion's
`worker.actor@2.0.0` and `worker.runtime@1.2.0` references only inside its
schema projection; it does not replace the released publisher artifacts or
claim that the currently published ActorNamespace can satisfy the forward
Actor binding. That dependent forward Form remains a separate unpublished
integration requirement.
Build the separate schema-only plugin with:

```sh
go build -o /tmp/terraform-provider-takoform-container-candidate ./cmd/provider-container-candidate
```

The local version is `0.0.0-dev+35a77bdb37483fccbb365998923824040dad67cc` and
is not a Provider release identity. A local filesystem plugin mirror can make
that binary available to OpenTofu without registry access; the fixture under
`testdata/provider-container-candidate-verify` contains ContainerService,
VectorIndex, and WorkerVersion references. `tofu validate` checks their typed
attributes. This plugin deliberately rejects Provider configuration, so it
cannot plan or mutate a Host resource; it is not Host support or lifecycle
evidence.

Regenerate the captured Forms CLI output only from the exact clean local Forms
checkout (no fetch or network access):

```sh
TAKOFORM_FORMS_SOURCE_ROOT=/path/to/takoform-forms-at-35a77bdb \
  bun run sync:provider-container-candidate
bun run check:provider-container-candidate
```

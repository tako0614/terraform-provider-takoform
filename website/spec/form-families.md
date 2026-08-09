# Form Families

A Form Family is a named group of Forms that share one platform model and are
designed to compose. A family is an API namespace and a catalog grouping; it
is not a package unit, a maturity state, or a compatibility promise. The
family model is decided in
[decision 0009](decisions/0009-form-families-and-namespaced-api-versions.md);
the semantic rule every member Form must satisfy is
[`portability-boundary.md`](portability-boundary.md).

## Groups

Each family owns a DNS-like API group with its own version:

```text
edge.forms.takoform.com/v1alpha1
containers.forms.takoform.com/v1alpha1   (future)
forms.example.com/v1alpha1               (third party)
```

A FormRef `apiVersion` is validated as `<dns-like group>/<version>`, not as a
fixed constant. Official families use subdomains of `forms.takoform.com`.
The groups `forms.takoform.com/v1alpha1` (Legacy) and
`forms.takoform.com/v1alpha2` (retained provider-v2 preview) are frozen and
never reused by a family. Family groups version independently; a new group
version is a new namespace, and occupied FormRefs are never rebound.

Publisher identity never enters the FormRef. Semantic identity (FormRef),
distribution bytes (package digest), publisher trust (signature policy), and
implementation (Host Support) remain four independent facts.

## Resource roles

Every current Form Definition declares one `role` from a closed enum. Roles
let tooling enforce lifecycle rules mechanically.

| Role | Meaning | Rules |
| --- | --- | --- |
| `identity` | long-lived logical resource | stable name; carries no implementation snapshot |
| `revision` | immutable implementation snapshot | never updated in place; changes create a new resource |
| `deployment` | selects which revisions are active | the only mutable path for traffic movement and rollback |
| `attachment` | connects a parent to external events or endpoints | deleting an attachment never deletes the parent |
| `policy` | operating rules changed independently of the parent | never migrates into the parent identity |

Outward capability use (a worker using KV, buckets, databases, queues,
services) is a typed Binding held by a revision resource
([decision 0010](decisions/0010-exact-interface-and-binding-contracts.md)).
Inward activation (HTTP routes, custom domains, cron triggers, queue
consumption) is an attachment resource. The two are never merged.

A family that splits one running thing across these roles owes a statement of
what holds them together. For the Edge Platform Family that statement is
[decision 0016](decisions/0016-the-worker-aggregate-has-one-active-deployment.md),
normatively stated in
[`host-api/v1alpha3.md`](host-api/v1alpha3.md#the-worker-aggregate): an identity
has at most ONE deployment resource; that deployment selects revisions of its
own identity, each named once, with weights summing to exactly 10000; every
attachment is admitted against the deployment rather than against any stored
revision, and refused when the deployment is absent or does not serve the
handler the attachment invokes; a deployment change that would leave a live
attachment or inbound binding unserved is refused, as is deleting the deployment
while one lives; and the identity reports itself Ready only while its deployment
actually serves. Because that last one is a representation rendered from another
resource, a deployment change also moves the identity's revision and therefore
its ETag, while leaving its generation alone.

## Edge Platform Family

`edge.forms.takoform.com/v1alpha1` is the first official family. Its members
fix the shape of a proven edge developer platform without naming its vendor.
The authored first-milestone members are:

```text
Compute      ModuleWorker, WorkerBundle, WorkerVersion, WorkerDeployment,
             StaticAssetBundle, WorkerCustomDomain, WorkerEndpoint,
             WorkerCronTrigger
Data         EdgeKVNamespace, ObjectBucket, SQLiteDatabase,
             SQLiteMigrationSet, SQLiteMigrationApplication
Messaging    AtLeastOnceQueue, QueueConsumer
```

Later milestones add further members through their own proposals —
`WorkerRoute`, `DenseVectorIndex`, `VectorMetadataIndex`, and
the bucket policy resources (`BucketCorsPolicy`, `BucketLifecyclePolicy`,
`BucketLockPolicy`). Listing a planned member here reserves nothing: a Form
exists only when its proposal, catalog declaration, and candidate package
exist.

Static files and SQLite migrations are artifact-backed rather than inline:
`StaticAssetBundle` and `SQLiteMigrationSet` desired state is exactly one
committed manifest digest, while `SQLiteMigrationApplication` attaches an
ordered set to a database with append-only path+digest history
([decision 0033](decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md)).

`ModuleWorker` fixes the ES Module Worker ABI by identity, and states what that
ABI is: the exact Interface contract `worker.runtime@1.0.0` in its
`providedInterfaces`
([decision 0019](decisions/0019-the-module-worker-abi-is-an-exact-contract.md)).
That contract fixes the module's default-export shape, the `fetch`, `scheduled`,
and `queue` signatures and what each event carries, the `env` object,
`ctx.waitUntil`, exception handling, request and response body streaming, the
minimum Web API surface, and module loading. Its handler vocabulary is those
three and nothing else: a handler no attachment in this family can activate is
a member no run can reach and no divergence between two hosts can be detected
in, so it does not belong in the contract until the attachment that makes it
observable ships beside it, in a new exact version
([decision 0019](decisions/0019-the-module-worker-abi-is-an-exact-contract.md)). A host that supports `ModuleWorker`
implements that contract at its exact digest, and advertises it there; a Worker
Version is the code that fills it.

Consequently **a runtime revision is a new exact Interface version, and — if it
changes what a Form desires — a new Form version. It is never a date.**
`WorkerVersion` therefore declares no `compatibilityDate` and no
`compatibilityFlags`. A compatibility date is meaningful only against a registry
that states which behavior each date changes; this project publishes none, so
two conforming hosts could read the same date differently, which is exactly the
incompleteness [`portability-boundary.md`](portability-boundary.md) forbids. The
`handlers` vocabulary is the handler set the runtime contract defines, and a
host refuses a handler that contract does not define before it mutates anything.

Three authored decisions are recorded here so none is read as
an oversight:

- `WorkerVersion.assets` is one optional closed object referring to the
  separate `StaticAssetBundle` member above. Absence means no asset lookup;
  presence fixes request order and not-found handling without granting a
  hidden runtime binding
  ([decision 0033](decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md)).
- `WorkerVersion` names its sealed-value declaration `requiredSensitiveVars`
  rather than `secretRequirements`. The Form Package data-only policy rejects
  the token `secret` anywhere in a field name
  ([`form-package/`](form-package/index.md)), so the portable field carries
  the same fact — only the names of host-supplied sensitive values are
  portable state — in permitted vocabulary.
- `WorkerVersion` projects `vars` keys, `requiredSensitiveVars` entries, and
  every binding `name` into ONE runtime environment namespace, so their union
  must be unique
  ([decision 0016](decisions/0016-the-worker-aggregate-has-one-active-deployment.md)).
  The desired schema cannot state it — `uniqueItems` compares whole objects, and
  no keyword relates one property's keys to a sibling array's element member —
  so a host refuses the collision before mutation and a client refuses it at
  plan time.
- The family carries two inward activations for `fetch`, and they are two Forms
  rather than one Form with a mode. `WorkerCustomDomain` states which name the
  AUTHOR owns that reaches a worker; `WorkerEndpoint` states that the worker is
  reachable at all, at an address the HOST assigns and publishes as outputs
  ([decision 0024](decisions/0024-a-worker-is-reachable-at-a-host-assigned-address.md)).
  The desired states are disjoint — one carries a hostname, the other carries
  nothing but the worker — so a selector token between them would be a free
  semantic token of exactly the kind this family forbids. A worker may have
  both, and has at most one endpoint.
- `WorkerEndpoint` is the family's first member to declare an `outputSchema`,
  which makes its assigned address a typed contract rather than an untyped
  document a consumer decodes
  ([decision 0025](decisions/0025-declared-outputs-are-a-typed-contract.md)). A
  Form declaring one publishes exactly its members; a Form declaring none
  publishes no `status.outputs` at all.

Semantics that differ from these shapes join other families instead of
widening a member: `PostgresDatabase`, `FifoQueue`, `WasiFunction`,
`ContainerService`, `TimezoneSchedule`, and durable-actor and addressable
container designs are separate family work with their own proposals.

## What a family does not do

- It does not merge packages: one Form Package still contains exactly one
  Form Definition.
- It does not grant maturity: family candidates are tracked in the family
  candidate set, and a member Form gains its own lifecycle record at its
  Experimental transition (no family member has made that transition yet).
- It does not constrain hosts: a host may support any subset of a family and
  states that subset in its Host Support Profile.
- It does not admit vendor identity: adapter profiles map family Forms to
  concrete backends outside the contract.

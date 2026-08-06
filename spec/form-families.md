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

## Edge Platform Family

`edge.forms.takoform.com/v1alpha1` is the first official family. Its members
fix the shape of a proven edge developer platform without naming its vendor.
The authored first-milestone members are:

```text
Compute      ModuleWorker, WorkerBundle, WorkerVersion, WorkerDeployment,
             WorkerCustomDomain, WorkerCronTrigger
Data         EdgeKVNamespace, ObjectBucket, SQLiteDatabase
Messaging    AtLeastOnceQueue, QueueConsumer
```

Later milestones add further members through their own proposals —
`StaticAssetBundle`, `WorkerRoute`, `SQLiteMigrationSet`,
`SQLiteMigrationApplication`, `DenseVectorIndex`, `VectorMetadataIndex`, and
the bucket policy resources (`BucketCorsPolicy`, `BucketLifecyclePolicy`,
`BucketLockPolicy`). Listing a planned member here reserves nothing: a Form
exists only when its proposal, catalog declaration, and candidate package
exist.

Two authored first-milestone decisions are recorded here so neither is read as
an oversight:

- `WorkerVersion` declares no `assets` field. Static assets served next to a
  worker belong to the separate `StaticAssetBundle` member above;
  `WorkerVersion` gains an `assets` reference to it when `StaticAssetBundle`
  lands in the next milestone. Until then a version fixes code, runtime
  behavior, configuration, and bindings only, and an asset-serving worker is
  not yet fully expressible.
- `WorkerVersion` names its sealed-value declaration `requiredSensitiveVars`
  rather than `secretRequirements`. The Form Package data-only policy rejects
  the token `secret` anywhere in a field name
  ([`form-package/`](form-package/README.md)), so the portable field carries
  the same fact — only the names of host-supplied sensitive values are
  portable state — in permitted vocabulary.

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

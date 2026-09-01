# Form Families

A Form Family is a named group of Forms that share one platform model and are
designed to compose. A family is an API namespace and a catalog grouping; it
is not a package unit, a maturity state, or a compatibility promise. The
family model is decided in
[decision 0009](decisions/0009-form-families-and-namespaced-api-versions.md);
the semantic rule every member Form must satisfy is
[`portability-boundary.md`](portability-boundary.md).

## Groups

Each current family owns one versionless reverse-DNS API group:

```text
edge.forms.takoform.com
container.forms.takoform.com
forms.example.com                       (third party)
```

A stable-v1 FormRef `apiVersion` is the whole versionless group, not a central
constant and not `<group>/<version>`. `kind`, `definitionVersion`, and
`schemaDigest` complete the exact Form identity; one Form changes without
renumbering its siblings ([decision
0049](decisions/0049-a-form-versions-alone.md)). Official families use
subdomains of `forms.takoform.com`, and a third-party reverse-DNS group is
equally representable. Publisher trust remains a separate fact.

Versioned groups such as `edge.forms.takoform.com/v1beta1` are retained
pre-v1 identities. The groups `forms.takoform.com/v1alpha1` (Legacy) and
`forms.takoform.com/v1alpha2` (retained provider-v2 preview) are also frozen.
None is accepted as a current v1 group or reused for new semantics.

The generated current-family index closes this repository's candidate corpus;
the prose table is a readable projection, not a second registry:

| Family group | Forms |
| --- | ---: |
| `container.forms.takoform.com` | 5 |
| `edge.forms.takoform.com` | 16 |
| `function.forms.takoform.com` | 4 |
| `queue.forms.takoform.com` | 1 |
| `schedule.forms.takoform.com` | 1 |
| `table.forms.takoform.com` | 1 |
| `topic.forms.takoform.com` | 2 |
| `vector.forms.takoform.com` | 1 |

Those eight groups contain 31 exact Experimental `0.x` FormRefs. The stable
Host API v1 and historical Specification receipts do not promote any of them
to Form `1.0.0`. A future Stable Form starts at `1.0.0` only by an explicit
decision for that Form.

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

Outward capability use (a worker using KV, databases, queues, services,
workflows, or actors) is a typed Binding held by a revision resource
([decision 0010](decisions/0010-exact-interface-and-binding-contracts.md)).
Inward activation (HTTP routes, custom domains, cron triggers, queue
consumption) is an attachment resource. The two are never merged.

A family that splits one running thing across these roles owes a statement of
what holds them together. For the Edge Platform Family that statement is
[decision 0016](decisions/0016-the-worker-aggregate-has-one-active-deployment.md)
and the current Edge Form Definitions: an identity
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

`edge.forms.takoform.com` is the first official family. Its members
fix the shape of a proven edge developer platform without naming its vendor.
Its 16 current members are:

```text
Compute      ModuleWorker, WorkerBundle, WorkerVersion, WorkerDeployment,
             StaticAssetBundle, WorkerCustomDomain, WorkerEndpoint,
             WorkerCronTrigger
Data         EdgeKVNamespace, SQLiteDatabase,
             SQLiteMigrationSet, SQLiteMigrationApplication
Messaging    AtLeastOnceQueue, QueueConsumer
Stateful     DurableWorkflow, ActorNamespace
```

`ObjectBucket` is not a current Form. The related `edge.objects` Interface and
`module-worker.object-bucket` Binding are likewise absent from the current
candidate closure. Runtime access to an externally managed object service uses
a sealed `standards.takoform.com/v1` slot with an opaque reverse-DNS protocol
identifier such as `com.amazonaws.s3`; Takoform carries no central protocol
enum and grants no portable lifecycle authority over that service. The exact
retained v1beta1 ObjectBucket bytes keep their historical meaning.

Static files and SQLite migrations are artifact-backed rather than inline:
`StaticAssetBundle` and `SQLiteMigrationSet` desired state is exactly one
committed manifest digest, while `SQLiteMigrationApplication` attaches an
ordered set to a database with append-only path+digest history
([decision 0033](decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md)).

`ModuleWorker` fixes the ES Module Worker ABI by identity, and states what that
ABI is: the exact Interface contract `worker.runtime@1.1.0` in its
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
widening an Edge member. The current Container, Function, Pull Queue,
Schedule, Table, Topic, and Vector families therefore keep their own groups and
exact FormRefs. Their presence does not turn the family list into a central
kind enum: the generated index closes this candidate corpus, while a Host
advertises the exact subset it actually supports.

## What a family does not do

- It does not merge packages: one Form Package still contains exactly one
  Form Definition.
- It does not grant maturity: all 31 current FormRefs are exact Experimental
  `0.x` identities, including the Edge family's 16. The stable Host lane and
  historical Specification receipts do not make them Stable, and their package
  artifacts remain a separate publication fact.
- It does not constrain hosts: a host may support any subset of a family and
  states that subset in its Host Support Profile.
- It does not admit vendor identity: adapter profiles map family Forms to
  concrete backends outside the contract.

## The Edge Platform Family's artifact semantics

These two rulebooks lived in the Host API document until
`forms.takoform.com/v1beta3`, which states mechanisms rather than rules about
particular Forms. They are family material: they say what an artifact MEANS to
the Form that references it, which is exactly the knowledge a family owns. The
protocol's own obligation — resolve the referenced manifest and hold it to the
contract the referring Form states, before any mutation — is unchanged and
stays there.

### Static assets on a Worker Version

The rules here are decided by
[decision 0033](decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md).
A `WorkerVersion` with no `assets` member performs no asset lookup. When the
member is present it is one closed object with three required members:

```json
{
  "bundle": {
    "apiVersion": "edge.forms.takoform.com",
    "kind": "StaticAssetBundle",
    "name": "static-assets"
  },
  "runWorkerFirst": true,
  "notFoundHandling": "single_page_application"
}
```

The bundle relation requires the target's exact FormRef and is UID-pinned like
every other relation. `notFoundHandling` is exactly `none` or
`single_page_application`.

- With `runWorkerFirst=false`, the host performs asset lookup first and invokes
  `fetch` only when that stage produces no response.
- With `runWorkerFirst=true`, it invokes `fetch` first and performs asset lookup
  only when the worker returns 404. An asset response wins; if asset lookup
  misses, the worker's 404 is preserved.
- `none` leaves a missing exact path as a miss.
- `single_page_application` answers a missing path with `index.html`. The host
  MUST resolve the exact referenced manifest and refuse the Worker Version with
  `invalid_argument` (400), before mutation, when it contains no `index.html`.

Asset lookup maps the runtime URL `pathname` to a manifest path as one closed
operation. Query strings and fragments are ignored; the escaped pathname is
percent-decoded once as strict UTF-8, and exactly one leading `/` is removed.
The host MUST reject encoded `/` or `\\`, repeated or empty interior segments,
dot segments, backslashes, controls, Unicode noncharacters, malformed escapes,
and invalid UTF-8. A valid path must still match the manifest's relative path
grammar. Invalid paths fail closed and MUST NOT enter SPA fallback. A valid
missing path is a miss under `none`, or resolves to `index.html` under
`single_page_application`; the root pathname `/` is the canonical empty-path
miss and follows that same fallback rule.

The attachment never grants a runtime binding and never changes the asset
bundle. A provider may author the manifest from local files, but desired state
and provider state carry no file bytes.

### SQLite migration history

A `MigrationBundle` is an ordered non-empty `files` list and every entry MUST
use `application/sql`; other media types are `artifact_invalid` (400).
`SQLiteMigrationApplication` pins exact `SQLiteDatabase` and
`SQLiteMigrationSet` relations. The host keeps a durable ordered ledger in the
database, whose entry identity is `(path, digest)`.

Before mutation, and again when an accepted operation commits, the host MUST
serialize against other migration applications for the database and prove the
ledger is an exact prefix of the referenced manifest. An applied entry that is
absent, moved, or has another digest is `migration_required` (409) and changes
nothing. Only the unapplied suffix may execute. Each file's SQL execution and
its ledger insertion are one SQLite transaction: failure rolls back both for
that file, keeps earlier committed entries, and makes a retry resume at the
same suffix boundary. Ready means the durable ledger equals the exact ordered
set.

A database has AT MOST ONE live `SQLiteMigrationApplication`, over its UID
exactly like the deployment and consumer rules; a second application against
the same database incarnation fails `invalid_argument` (400) before any
mutation. Advancing a database's schema is deleting the application and
creating one referencing the longer set — the ledger check makes the new
application resume at the unapplied suffix, so the handover replays nothing.

Deleting `SQLiteMigrationApplication` removes the attachment and its relation
holders only. It MUST NOT execute down SQL, remove ledger entries, reinterpret
the schema, or delete the database. The database and set are protected by
ordinary `dependency_in_use` while the attachment lives; database deletion
after it is gone is a separate database policy, not migration rollback.

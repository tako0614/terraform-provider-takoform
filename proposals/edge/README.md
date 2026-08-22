# Edge Platform Family proposals

The Edge Platform Family, `edge.forms.takoform.com/v1beta1`, is the first
official Form Family ([decision 0009](../../spec/decisions/0009-form-families-and-namespaced-api-versions.md)).
Its members fix, completely, the application-visible semantics of a proven
edge developer platform without naming its vendor
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## Authoring policy: shape-preserving contracts

Every member Form preserves one service shape end to end: execution ABI,
client API, data model, consistency, delivery guarantees, update and delete
units, error semantics, and the capabilities exposed through typed Bindings.
No free semantic token is admitted; a difference in semantics is a different
Form, never a selector value. Outward capability use is a digest-bound
Binding held by a revision resource; inward activation is an attachment
resource ([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md)).
Desired schemas carry no `name` or envelope plumbing: the v1beta1 resource
envelope owns identity and status
([decision 0011](../../spec/decisions/0011-resource-identity-generation-and-revision.md)).

The catalog source of truth is `internal/edgeformcatalog`; the generated
candidates live in `forms/candidates/edge/v1beta1`, the exact Interface and
Binding candidates in `interfaces/candidates/v1alpha1` and
`bindings/candidates/v1alpha1`.

## MVP members

| Form | Role | One-line semantics | Separate-Form boundary |
| --- | --- | --- | --- |
| [ModuleWorker](module-worker.md) | identity | Logical identity of one ES Module Worker application; the ABI is fixed by identity. | A WASI function or container service is a different Form. |
| [WorkerBundle](worker-bundle.md) | revision | Immutable content-addressed module bundle: main module plus digest-pinned modules. | A different linking or packaging model is a different Form. |
| [StaticAssetBundle](static-asset-bundle.md) | revision | Immutable content-addressed static-file inventory. | Object storage or request routing is a separate Form. |
| [WorkerVersion](worker-version.md) | revision | Immutable executable snapshot: bundle, handlers, vars, sensitive slots, typed bindings. | Mutable in-place code or config is not a version of this Form. |
| [WorkerDeployment](worker-deployment.md) | deployment | Basis-point traffic split across up to eight Worker Versions of one worker. | Per-request routing rules or geographic steering is separate work. |
| [WorkerCustomDomain](worker-custom-domain.md) | attachment | Serves one DNS hostname from a worker's active deployment over HTTPS. | Path-pattern routes are a separate attachment Form. |
| [WorkerEndpoint](worker-endpoint.md) | attachment | Makes a worker reachable over HTTPS at an address the host assigns and publishes. | A name the author owns is `WorkerCustomDomain`; a chosen label or region is host placement, not a Form. |
| [WorkerCronTrigger](worker-cron-trigger.md) | attachment | Invokes the scheduled handler on a five-field UTC cron schedule. | Timezone-aware scheduling is a different Form. |
| [EdgeKVNamespace](edge-kv-namespace.md) | identity | Globally replicated key/value namespace with eventual consistency. | A linearizable or per-key-consistent store is a different Form. |
| [ObjectBucket](object-bucket.md) | identity | Flat-namespace object store with read-after-write consistency and strong etags. | CORS/lifecycle/lock rules are separate policy Forms. |
| [SQLiteDatabase](sqlite-database.md) | identity | Embedded SQLite database with safe wire values, rollback-only queries, and serializable transactions. | A Postgres or MySQL database is a different Form. |
| [SQLiteMigrationSet](sqlite-migration-set.md) | revision | Immutable ordered application/sql migration manifest. | Another dialect or declarative schema model is a different Form. |
| [SQLiteMigrationApplication](sqlite-migration-application.md) | attachment | Applies only the checksum-safe suffix of one set to one database. | Rollback or destructive reset requires a separate authority-bearing Form. |
| [AtLeastOnceQueue](at-least-once-queue.md) | identity | Queue with at-least-once delivery and no ordering guarantee. | A FIFO or exactly-once queue is a different Form. |
| [QueueConsumer](queue-consumer.md) | attachment | Attaches one worker as the batch consumer of one queue, with retry and dead-lettering. | Pull-based consumption over an API is a different Form. |

Designs that differ in these semantics — `PostgresDatabase`,
`FifoQueue`, `WasiFunction`, `ContainerService`, `TimezoneSchedule` — belong
to future families and get their own proposals when that work starts, per
[spec/form-families.md](../../spec/form-families.md).

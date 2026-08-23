# ActorNamespace — `takoform_actor_namespace`

## Workload and consumer

An application team keeps per-entity coordination — a chat room, a game
session, a booking ledger, a rate-limit bucket — behind one addressable actor
per entity: a class exported by a worker's module, one live execution context
per actor id, private durable storage, and one alarm. Workers reach actors
through `module-worker.actor` bindings — mint an id from a name, get a stub,
invoke; actors are runtime data, never Resources. The Form enters the family
at the v1beta2 generation as a `0.1.0` Experimental member
([decision 0043](../../spec/decisions/0043-forms-target-popular-vendor-locked-primitives.md),
[decision 0046](../../spec/decisions/0046-stable-arrives-through-a-stable-grade-beta-2.md)).

## Role

`identity`, a member of the worker aggregate. Desired state is a `worker`
reference to one [ModuleWorker](module-worker.md) plus `className`, a class
export name in the JavaScript identifier grammar; the reference pins the exact
`ModuleWorker` contract
([decision 0022](../../spec/decisions/0022-relations-pin-the-target-contract.md)).
The Form carries no implementation snapshot: which code answers is whatever
the worker's ACTIVE DEPLOYMENT selects
([decision 0016](../../spec/decisions/0016-the-worker-aggregate-has-one-active-deployment.md)),
so behavior upgrades and rollback ride [WorkerDeployment](worker-deployment.md).
One worker UID carries at most one ActorNamespace per class name — two
namespaces over one class would give one class two disjoint id spaces — and a
second fails `invalid_argument` (400) before any mutation, a count no schema
can state, exactly like one-deployment-per-worker.

## Observable semantics

Exactly the `worker.actor@1.0.0` contract, declared the way `ModuleWorker`
declares `worker.runtime@1.0.0`
([decision 0019](../../spec/decisions/0019-the-module-worker-abi-is-an-exact-contract.md)):
the identity is what a host implements; the class the serving version exports
is the code that fills it. One contract carries the class ABI, addressing,
storage, and the alarm, because each is scoped by the same actor id.

Addressing: an actor is addressed by a NAME the author's code chooses or an id
the host mints. `env.NAME.idFromName(name)` derives the same id for the same
name on every call, with no host round trip; `env.NAME.newUniqueId()` mints an
id no name ever derives; `env.NAME.get(id) -> ActorStub` builds a stub without
host work. An id is an opaque, stable string a configuration must not parse.
Every id addresses an actor: there is no create call and no existence check —
the first delivery to an id is the actor's creation, and an actor with no
storage, no alarm, and no live context costs nothing.

Invocation: `stub.fetch(request) -> Promise<Response>` with exactly the
invocation semantics the family fixed for `worker.service`: bodies stream in
both directions, the promise resolves with the actor's host-generated 500 when
the class's `fetch` throws, and rejects only when the call could not be made.
What is new is WHERE the call lands. The host guarantees AT MOST ONE live
execution context per actor id — across locations, eviction, and process
death — and delivers to it one invocation at a time, each running to
completion before the next begins, so concurrent callers observe
serialization, never interleaving. The class ABI is `fetch(request)` plus an
optional `alarm()` handler, constructed by the host with the actor's id,
storage, and alarm surfaces.

Storage: each actor holds PRIVATE durable storage with the family's SQLite
semantics — the SQL dialect, `EdgeSqlValue` domain, and serializable atomicity
[SQLiteDatabase](sqlite-database.md) fixes through `edge.sql@1.0.0`
([decision 0034](../../spec/decisions/0034-edge-sql-uses-safe-wire-values-and-rollback-only-queries.md))
— scoped to one actor id and reachable only from that actor's own execution
context; the single context is what makes every store single-writer. Unlike
`SQLiteDatabase`, the store admits schema statements from the actor's own
code: the migration ledger exists because runtime SQL from many clients cannot
own one shared schema history, an actor's store has exactly one writer, and
unbounded actor cardinality makes a per-actor migration Resource the same
category error as a per-message one. The contract's `limits` fix the portable
per-actor storage ceiling.

Alarm: each actor has AT MOST ONE pending alarm. `alarm.set(at)` schedules a
wake-up and REPLACES any pending one; `alarm.get()` reads it; `alarm.clear()`
removes it. At or after its time the host invokes the class's `alarm()`
handler in the actor's own execution context, serialized with messages like
any other delivery. Firing is at-least-once — a handler that throws is
re-invoked — and a completed handler run consumes the alarm unless the handler
set a new one, so the handler carries the consumer-idempotency obligation the
family's at-least-once vocabulary already states.

Between deliveries the host may evict the execution context; storage and the
pending alarm survive, and the next invocation or the alarm revives the actor.
The identity outlives every execution context. Actors have unbounded
cardinality and runtime addressing — like a queue's messages, nothing about
them enters desired state, the resource envelope, or a plan: no uid, no
generation, nothing for a provider to manage.

Readiness follows the aggregate: `Ready=True` only while the active deployment
exists and EVERY weighted version exports `className` satisfying the ABI;
otherwise `Ready=False` with `Provisioning` or `UnsupportedCapability` and a
`hostReason` naming the class — a stable readiness failure, never a silent
fallback, rendered from other resources and so moving `metadata.revision`,
never `metadata.generation` (decision 0016). Creating the Form while a live
deployment visibly lacks the class fails `unsupported_capability` (422) before
any mutation; before any deployment it stores `Provisioning`, and invoking a
stub of an unserved namespace rejects — the call could not be made. Host
capability is decided at plan time
([decision 0031](../../spec/decisions/0031-host-capability-is-decided-at-plan-time.md)),
and a client SHOULD refuse a bundle that demonstrably lacks the export.

## Why this is one Form

The at-most-one-execution-context guarantee is the contract: it is what lets
per-actor storage be read and written without cross-actor transactions, and
what makes an actor a correct serialization point for the entity it names.
Addressing, serialization, storage, and the alarm travel as one Form because
each is stated in the others' terms — the storage is safe because delivery is
serialized, the alarm fires into the same serialized context, and the id is
the unit all three are scoped by. A consistency or concurrency selector would
change what application code may assume
([decision 0008](../../spec/decisions/0008-forms-preserve-service-shape.md)).

## What would require a separate Form

A shared multi-writer database is `SQLiteDatabase`; a globally addressed
document/KV table with declared keys is the `table` family of decision 0043; a
placement or geo-pinning rule is a separate `policy`-role Form; a WebSocket
session surface terminated in the actor is the `realtime` family candidate.
An RPC stub projecting the class's own methods needs a value-serialization
contract this generation does not fix, so it is a new exact contract version,
never a quiet widening of this one.

## Provided Interfaces

`worker.actor@1.0.0` — the class ABI a conforming host provides to actor code
and the addressing/invocation surface the binding projects. It is held by the
IDENTITY because the identity is what a host implements; the class a Worker
Version exports is the code that fills it.

## Accepted Bindings

None; it is a binding target (`module-worker.actor`). The class's own code
runs inside its worker's environment, so every capability it uses is a binding
the serving `WorkerVersion` declares
([decision 0010](../../spec/decisions/0010-exact-interface-and-binding-contracts.md));
the v1beta2 `WorkerVersion` carries `actorBindings` as one more annotated
list, whose names join the single environment namespace with no host edit.
Like `module-worker.workflow` and unlike `module-worker.service`, the binding
does not require its target `Ready` at bind time: a namespace's readiness
follows its own worker's deployment, so a Ready gate would make the ordinary
self-bound wiring unconstructible in one apply; the binding requires existence
and the exact Interface, and the deployment that lands next is what makes the
namespace serve.

## Lifecycle risks

Deleting an ActorNamespace destroys EVERY actor's storage and pending alarm —
irreversible, like `SQLiteDatabase` delete; portable state never carries
backups. The namespace does not wait for quiescence: an actor is a passive
addressee, so deleting one is deleting data at rest — where deleting a
[DurableWorkflow](durable-workflow.md) mid-instance would abandon an execution
the host promised to finish, which is why the two Forms answer delete
differently. Deletion while any Worker Version's `actorBindings` entry pins
the namespace fails `dependency_in_use` (409), and deleting the referenced
worker while the namespace lives fails the same way. The Form joins the
deployment fence of decision 0016: an apply that would leave a live
namespace's class unexported, and a deployment delete while one lives, are
refused — pending alarms need code to fire into, and at-least-once firing is a
promise only a served class can keep.

## Prior art

The addressable-actor primitive of a proven edge platform, retained without
its vendor, account, or commercial surface. The withdrawn v1alpha2
`StatefulEntity` candidate is prior art this Form supersedes in part —
decision 0043 splits its remainder into the `table` family and retires the
standalone actor family candidate into this addition.

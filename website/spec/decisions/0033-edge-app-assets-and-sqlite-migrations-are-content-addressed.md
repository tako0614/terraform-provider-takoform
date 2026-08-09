# 0033 — Edge app assets and SQLite migrations are content-addressed

- Status: accepted
- Date: 2026-08-09
- Owners: Takoform maintainers

## Context

The first Edge Platform Family milestone can run a worker and provision a
SQLite database, but it cannot describe either half of a small edge
application that is not executable code: static files served beside the
worker, and ordered SQL changes applied to the database. Putting either byte
stream directly in a Form would create a second artifact protocol beside the
content-addressed transport of [0012](0012-artifacts-use-content-addressed-upload.md),
put raw bytes in provider state, and make an immutable revision's identity
depend on client serialization rather than on the manifest the host commits.

The two byte streams also do not have the same semantics as a `WorkerBundle`.
Static files are looked up by request path and never linked as modules. SQL
migrations are an ordered history: changing an applied file is not a new
rendering of the same state, but a rewrite of the database's past. Reusing the
`WorkerBundle` shape would make one manifest kind mean three different things.

## Decision

The family adds three unpublished candidate Forms at definition version
`0.1.0`, alongside one optional `WorkerVersion.assets` object. The candidate
lane has never been published, so regenerating these exact candidate identities
does not mutate a published FormRef.

### Static assets

`StaticAssetBundle` is an immutable `revision`. Its entire portable desired
state is `{manifestDigest}`. The referenced manifest MUST have kind
`StaticAssetBundle`; it carries `files` and no module members. File order has no
routing meaning. The manifest digest, paths, media types, sizes, and blob
digests are artifact evidence. Raw file bytes never enter desired state or
provider state.

`WorkerVersion.assets`, when present, is one closed object whose three members
are all required:

- `bundle` is an exact-Form relation to `StaticAssetBundle`;
- `runWorkerFirst` is a boolean;
- `notFoundHandling` is exactly `none` or `single_page_application`.

Static asset media types are extensible within the published artifact-manifest
v1alpha1 normalized lowercase type/subtype grammar and carry no parameters;
they are not a closed runtime allowlist. Migration files remain exactly
`application/sql`.

Absence means no asset lookup. With `runWorkerFirst=false`, the host looks up
the request path first and invokes `fetch` only when that lookup produces no
response. With `runWorkerFirst=true`, the host invokes `fetch` first and looks
up assets only when the worker response is 404. An asset response wins; if the
asset stage also misses, the worker's 404 is preserved. `none` makes a missing
exact path a miss. `single_page_application` serves `index.html` for a missing
path, and the host MUST refuse the Worker Version before mutation when the
referenced manifest has no `index.html`. The attachment grants no hidden
runtime binding and does not mutate the asset bundle.

Asset routing takes the runtime URL `pathname`, ignores its query and fragment,
decodes percent escapes once as strict UTF-8, and strips exactly one leading
`/`. Encoded separators, repeated or empty segments, dot segments, backslashes,
controls, Unicode noncharacters, malformed escapes, and invalid UTF-8 are
invalid paths. They fail closed and never enter SPA fallback; only a valid
missing path may resolve to `index.html`.

### SQLite migrations

`SQLiteMigrationSet` is an immutable `revision`. Its entire portable desired
state is `{manifestDigest}`. The referenced manifest MUST have kind
`MigrationBundle`; every file MUST use `application/sql`, and the `files` array
order is the migration order. The ordered manifest is the identity: changing a
path, digest, or position produces a different set. SQL bytes remain blobs in
the artifact store and never enter Form desired state or provider state.

`SQLiteMigrationApplication` is an immutable `attachment` with exactly two
required exact-Form relations: `database` to `SQLiteDatabase` and
`migrationSet` to `SQLiteMigrationSet`. A host keeps a durable ledger in the
database, ordered by migration path and blob digest. Before applying anything,
it MUST prove that the ledger is an exact prefix of the referenced manifest.
An already-applied path with another digest, another order, or absence from the
new set is `migration_required` and mutates nothing. Only the unapplied suffix
may run.

For each suffix entry, execution of that SQL file and insertion of its ledger
record are one SQLite transaction. A failed file rolls back both the file and
its ledger record; earlier committed entries remain, so retry resumes at the
same suffix boundary. Hosts MUST serialize this prefix check and suffix apply
per database and re-check it while holding that serialization boundary. The
attachment is Ready only when the durable ledger equals the referenced ordered
set. Multiple append-only applications may coexist: after a later superset is
applied, an older shorter application remains stored but is not Ready until it
is re-applied against its intended set.

Deleting `SQLiteMigrationApplication` detaches management only. It MUST NOT
execute down migrations, delete ledger records, reinterpret the schema, or
delete the database. Ordinary relation protection separately prevents deleting
the database or migration set while a live application references it. What
happens after the attachment is removed is the database's own deletion policy,
not migration rollback.

Neither migration Form declares a typed output. Readiness plus the exact
relations state the portable result. A resultant schema digest would depend on
SQLite introspection and normalization rules not declared by this family and
would pretend to be portable evidence it is not.

## Consequences

- `WorkerBundle`, `StaticAssetBundle`, and `MigrationBundle` stay three closed
  artifact meanings even though they use one transport.
- Provider local-file authoring may keep local paths, sizes, and digests as
  evidence, but uploads bytes before resource apply and sends only
  `manifestDigest` to the host.
- A new migration is an appended manifest entry and a new
  `SQLiteMigrationSet` revision. An applied file is never edited in place.
- The reference host can prove manifest kind closure, SQL media type, SPA index
  presence, and the prefix/suffix ledger state machine. It deliberately has no
  SQLite execution engine; a real backend transaction and a real asset-serving
  runtime remain publication evidence, not facts inferred from local tests.

## Rejected alternatives

- **Put files or SQL strings directly in desired state.** Rejected because it
  duplicates artifact identity, leaks bytes into state, and makes large payloads
  ordinary resource documents.
- **Reuse `WorkerBundle` for every byte set.** Rejected because module linking,
  path serving, and ordered schema history are different meanings.
- **Track only migration filenames.** Rejected because rewriting an applied
  file under the same name would become invisible.
- **Make an application update point to the next set.** Rejected because the
  attachment's desired history would move in place. A new exact set and a new
  immutable application keep the plan reviewable.
- **Delete means rollback.** Rejected because arbitrary SQL has no portable
  inverse and attachment teardown must not become implicit destructive data
  mutation.

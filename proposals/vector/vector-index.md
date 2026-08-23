# VectorIndex — `takoform_dense_vector_index`

The shipped resource type is `takoform_dense_vector_index`: the withdrawn
v1alpha2 lane published `takoform_vector_index`, and a Terraform resource
type is never reoccupied with a different contract
([decision 0030](../../spec/decisions/0030-a-form-line-moves-a-terraform-resource-type-may-not.md)).
Dense is also the accurate word — this Form fixes the dense-vector shape,
and a sparse or hybrid index is a different Form.

## Workload and consumer

An application stores embeddings — document chunks, products, images — and
asks for the k records nearest a query vector, optionally filtered by
metadata. Workers will consume the index through `module-worker.vector`
bindings; management is the ordinary resource lifecycle.

## Role

`identity`. Desired fields are `dimension` and `metric`, both fixed at
creation; everything else about the index is the `vector.index` Interface.
`metric` is the closed enum `cosine | euclidean | dotproduct` — precisely
the case decision 0008 admits as a closed enum, an algorithm choice within
identical API shape, storage model, and lifecycle ("for example a vector
distance metric"). `dimension` is a positive integer; a conforming host MUST
accept any dimension from 1 through 1536 and states a higher ceiling in its
Host Support Profile. A change to either field is refused before mutation
and plans as replacement. Namespaces are runtime data reached through the
Interface, never Resources.

## Observable semantics

Exactly the `vector.index@1.0.0` contract: `upsert`, `fetch`, `query`, and
`delete`, each addressed to one namespace, with whole-record upsert,
read-after-write fetch, approximate top-k query, closed errors, and portable
minimum limits.

A record is `{id, values, metadata?}`. A record `id` follows the portable
uid grammar (`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`), so UUIDs and content
digests — the common embedding identities — are valid ids; a namespace name
follows the portable resource-name grammar (`^[a-z][a-z0-9-]{0,62}$`). An omitted
namespace means the default namespace, and a namespace exists exactly while
records exist in it. `values` is an array of exactly `dimension` finite
binary64 numbers; a wrong length, a non-finite member, or an all-zero vector
on a `cosine` index fails `invalid_value`, and a malformed id fails
`invalid_key`. Components travel as binary64 but the contract guarantees
only IEEE binary32 precision: a host MAY round components to its storage
precision, never below binary32, and `fetch` returns the stored values.
Metadata is a flat string-keyed map of scalars — UTF-8 string, finite
binary64 number, or boolean; no null, list, map, or bytes — round-tripped
exactly, with `maxMetadataBytes` (40960) bounding the UTF-8 encoding of its
canonical JSON; exceeding it fails `metadata_too_large`.

`upsert` inserts or replaces up to `maxUpsertBatchVectors` (100) whole
records per call — values AND metadata; there is no partial update. A larger
batch fails `batch_too_large`. `fetch` reads up to `maxFetchIds` (100)
records by id and omits absent ids from its result rather than failing: it
is a batch read, and absence is an answer. A fetch after a resolved upsert
or delete observes it — fetch is read-after-write. `query` takes a
`dimension`-length vector (held to the same `invalid_value` rules as stored
vectors), `topK` (1 through `maxTopK`, 256), an optional filter, and flags
selecting whether values and metadata return with each match; it returns at
most `topK` matches ordered most-similar first. The score is the declared
metric's value computed at the host's storage precision: cosine similarity
and dot product order descending, Euclidean distance ascending. Retrieval is
APPROXIMATE: a host MAY return approximate nearest neighbors, recall targets
are host quality stated in its Host Support Profile, and only the scores of
returned matches are exact per the metric. Query freshness is eventual — a
resolved upsert or delete may be missing from, or still present in, query
results until the index converges. `delete` removes up to `maxDeleteIds`
(1000) records by id and succeeds for ids already absent.

The filter grammar is closed equality and inclusion, nothing else: a filter
is a string-keyed map of at most `maxFilterClauses` (16) members, each
either a scalar (the metadata value must equal it) or `{"in": [scalar,
...]}` with at most `maxInValues` (64) members (the value must equal one of
them), and members conjoin. There is no range, negation, disjunction, or
free query language. The grammar is structural, so a malformed filter never
reaches the data plane; a clause naming an absent metadata key, or comparing
across scalar types, matches nothing and is not an error.

## Why this is one Form

Fixed dimension, one metric, approximate top-k, and the closed filter are
the model consumer code and its embeddings are built against; re-embedding
is what changing them means, which is why they are immutable identity rather
than configuration. A metric or dimension "update", an exactness selector,
or an open filter language would hollow the contract out (decision 0008).

## What would require a separate Form

A sparse or hybrid keyword/vector index changes the data model and the query
contract and is a different Form; so is an exact-kNN index whose recall is
the contract rather than host quality. Range and negation filters, id
listing and namespace enumeration, and partial metadata update are versioned
`vector.index` additions, never selectors. A search engine with a de-facto
standard API is an integration, never a Form (decision 0043).

## Provided Interfaces

`vector.index@1.0.0`.

## Accepted Bindings

None; it is a binding target (`module-worker.vector`, a future binding Form
mirroring `module-worker.edge-kv`). Consumers outside the worker family
reach the same Interface only when the cross-family binding projection
exists; that realization is its own work and is not defined here.

## Lifecycle risks

Deleting an index bound by any consumer revision must fail with
`dependency_in_use` (refuse_while_bound). Delete destroys every namespace
and record; there is no portable restore. Because `dimension` and `metric`
plan as replacement, changing them destroys the data with the resource —
re-embedding into a new index is the honest migration, and no portable path
pretends otherwise.

## Prior art

The Pinecone-like serverless vector index category — offered by every major
cloud and several independents, standardized by none (decision 0043). The
withdrawn v1alpha2 `VectorIndex`, whose resource type this Form deliberately
does not reoccupy. The Edge family's planned-member listing of
`DenseVectorIndex` / `VectorMetadataIndex` reserved nothing
([spec/form-families.md](../../spec/form-families.md)); decision 0043 makes
this family the home it assigns to vector search.

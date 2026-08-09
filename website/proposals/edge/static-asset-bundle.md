# StaticAssetBundle — `takoform_static_asset_bundle`

## Workload and consumer

An edge application serves immutable HTML, scripts, styles, images, and other
files beside a Worker Version without turning those files into executable
modules or provider state.

## Role

`revision`. The whole desired state is one committed `manifestDigest`; a byte,
path, media-type, or inventory change is a new revision.

## Observable semantics

The referenced artifact manifest has kind `StaticAssetBundle`, carries `files`,
and carries no module members. The manifest fixes each relative path, media
type, size, and blob digest. A host resolves and validates that exact manifest
before storing the Form. File array order has no routing meaning.

A Worker Version refers to the bundle only through its optional closed `assets`
object. Asset lookup order and SPA fallback are properties of that attachment,
not of this bundle. The bundle grants no runtime binding and is not mutated when
a version uses it
([decision 0033](../../spec/decisions/0033-edge-app-assets-and-sqlite-migrations-are-content-addressed.md)).

## Why this is one Form

The manifest is one immutable inventory with one content address. Splitting
files into resources would replace one atomic revision with a partially updated
set and make rollback unable to name the exact site bytes.

## What would require a separate Form

Mutable object storage, request routing rules, redirects, or generated content
have independent lifecycle or behavior and do not belong in this revision.

## Provided Interfaces

None.

## Accepted Bindings

None.

## Lifecycle risks

An uncommitted, foreign-tenant, wrong-kind, or invalid manifest fails before
mutation. Deleting a bundle referenced by a Worker Version fails
`dependency_in_use`. Raw file bytes never enter desired state or provider
state; local authoring records only path, size, and digest evidence.

## Prior art

The immutable asset manifest served alongside an edge worker, separated from
the executable module bundle so each content address has one meaning.

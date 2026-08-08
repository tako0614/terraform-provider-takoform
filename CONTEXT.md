# Takoform Context

Takoform defines a small portable desired-state boundary between
infrastructure-as-code clients and independently owned resource hosts. This
language separates specification maturity from implementation, availability,
and commercial policy.

## Specification lifecycle

**Specification Epoch**:
A versioned Form identity domain whose API version separates independently
evolving Form lines. Kind names may recur across epochs without sharing Form
SemVer or compatibility.
_Avoid_: Release generation, admission generation

**Legacy Epoch**:
A frozen Specification Epoch retained only for published identity,
compatibility, recovery, revocation, and migration.
_Avoid_: Current API, deprecated-but-mutable API

**Form Proposal**:
A mutable, unversioned design for one portable resource contract, grounded in
a named owner, real consumer, host implementation, and prior-art analysis.
_Avoid_: Draft release, candidate package, pre-standard

**Experimental Form**:
A reproducible public `0.x` Form whose portability and compatibility model are
still being validated through real implementations and consumers.
_Avoid_: Approved Form, standard candidate

**Stable Form**:
A Form whose contract has earned stability through independent implementation,
interoperability, migration, deprecation, and operational evidence.
_Avoid_: Admitted Form, certified Form, portable-standard

**Legacy Form**:
An immutable published Form identity retained for compatibility, recovery, and
explicit migration but not used as the basis for new specification work.
_Avoid_: Deleted Form, invalid Form

## Portable contract

**Form**:
A versioned portable desired-state contract for one resource kind; it is not a
resource instance or a host implementation.
_Avoid_: Resource Shape, service type, provider resource

**FormRef**:
The immutable exact identity of one Form Definition, including its
Specification Epoch, kind, version, and schema digest. Publication makes that
identity publicly resolvable; a local candidate can carry an exact FormRef
without claiming publication.
_Avoid_: Latest Form, floating Form version

**Form Definition**:
The data-only schema and portable semantics bound to an exact FormRef.
_Avoid_: Provider schema, deployment template

**Form Package**:
A content-addressed distribution of one exact Form Definition and its portable
fixtures and metadata.
_Avoid_: App package, provider release

**Package Digest**:
The immutable SHA-256 identity of one canonical Form Package index and thereby
its complete payload closure.
_Avoid_: Package version, release version

**Publication Locator**:
The repository path and Git tag derived from a Package Digest for transport and
retention; it expresses no compatibility or maturity.
_Avoid_: Package version, Form version, latest tag

**Form Family**:
A named group of Forms sharing one platform model and a namespaced API group;
a catalog and namespace fact that carries no maturity and merges no packages.
_Avoid_: Package bundle, product suite, maturity tier

**Resource Role**:
The closed classification of one Form as identity, revision, deployment,
attachment, or policy; tooling enforces role lifecycle rules mechanically.
_Avoid_: Resource category, type tag

**Interface Contract**:
A digest-bound operation-surface contract fixing operations, types, errors,
consistency, and behavior fixtures, distributed with the specification
repository rather than as an independently published package.
_Avoid_: Open descriptor, endpoint credential, independently published package

**Binding Contract**:
A digest-bound typed-capability contract granting a consumer a runtime API and
permission together without exposing credentials.
_Avoid_: Generic connection, permission token

**Interface Declaration**:
A non-secret portable description of a runtime-facing interface that a Form may
produce in the retained lanes; binding, authorization, and credentials remain
host-owned.
_Avoid_: Endpoint credential, connection record

## Implementation and availability

**Provider**:
The Terraform/OpenTofu client adapter that maps typed configuration and state
to the host contract; its version does not express Form maturity.
_Avoid_: Form authority, host implementation

**Host**:
A system that owns Resource instances and implements the lifecycle for exact
FormRefs.
_Avoid_: Provider, standard authority

**Host Support**:
An exact, evidence-backed statement that a named host implementation supports a
specific FormRef and lifecycle contract.
_Avoid_: Admission, approval, certification

**Form Activation**:
A host/operator decision that a supported FormRef is available for use in a
particular scope.
_Avoid_: Form maturity, standardization

**Resource**:
A host-owned instance realized from desired state conforming to one exact
FormRef.
_Avoid_: Form, package, provider resource type

**Service Offering**:
A commercial platform's availability, capacity, pricing, and support policy for
an exact supported FormRef.
_Avoid_: Standard Form, admitted Form, Host Support

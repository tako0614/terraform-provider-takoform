# Form Proposal: RelationalDatabase

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

The candidate Form describes a relational database lifecycle and a query
Interface, including engine compatibility, optional digest-bound schema
material, lifecycle identity, and observable query capability.
Credentials, storage size, size class, topology, replica implementation,
network placement, backup machinery, capacity, and price stay with the host.

## Substrate-neutrality review

Engine/version and schema bytes can be interpreted consistently by a managed
database, an operator-managed database, or a self-hosted engine. Storage size,
size class, high availability, backup tier, region, and endpoint compatibility
are excluded because their guarantees and lifecycle differ by host offering.

## Lifecycle and security risks

Engine-family changes may require export and replacement. Delete and migration
can destroy durable data and therefore require explicit retention/recovery
evidence. Import binds an exact existing database. Credentials are host-owned
Interface bindings and never portable output or provider state.

## Prior art and gap

OCCI Resource/Storage/Link, CIMI System/Volume, TOSCA Database/DBMS nodes,
Kubernetes database operators, Crossplane managed databases, and Terraform
RDS/Cloud SQL/D1/PostgreSQL resources are applicable. The gap is a small
relational service intent with a portable query capability but no standardized
credential, topology, or engine implementation.

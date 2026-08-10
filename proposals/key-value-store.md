# Form Proposal: KeyValueStore

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

The candidate Form describes a namespaced key-value store. It owns namespace
lifecycle, narrow consistency/capability intent, and a
`keyvalue.store@1` Interface. Backend engine, placement, capacity, credentials,
replication, endpoint, and price remain host decisions.

## Substrate-neutrality review

Consistency and default expiry are consumer-observable data semantics across
an edge KV, a database-backed namespace, or an embedded distributed store.
Region, cache tier, replication factor, namespace/account binding, operation
limits, and endpoint compatibility remain host/profile concerns.

## Lifecycle and security risks

Incompatible consistency or namespace changes can require copy-and-replace.
Delete can destroy all keys. Import requires exact namespace ownership and
capability evidence. Values, access tokens, and credential material never
enter portable state.

## Prior art and gap

OCCI generic Resource/Link, TOSCA capability relationships, managed KV
operators/Crossplane resources, and Terraform Cloudflare KV/cache resources
are applicable; CIMI has no focused KV contract. The gap is a minimal KV
namespace and Interface without pretending provider consistency models are
identical.

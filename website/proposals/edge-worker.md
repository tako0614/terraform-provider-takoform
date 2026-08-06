# Form Proposal: EdgeWorker

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need

Takosumi Cloud runs immutable edge applications and is the first host. The portable contract must describe the application a consumer
wants without exposing Cloudflare, placement, routing, credentials, billing,
or a host's runtime manager.

## Portable boundary

The candidate boundary is an immutable artifact, an entrypoint, an open runtime
capability requirement, non-secret configuration, declared Resource
connections, and an `http.request@1` Interface. Runtime implementation,
routing, scaling, placement, assets policy, request limits, secrets, and price
remain host decisions. Every field from the Legacy EdgeWorker must be justified
again; the v1alpha1 document is prior art, not a starting schema.

## Substrate-neutrality review

The artifact and entrypoint can run in a Workers-style isolate, another edge
isolate, a WASI host, or a regional event runtime. `runtime` is an open
capability requirement whose support is host-declared. Concurrency, timeout,
asset fallback, route, compatibility date, account, and placement fields are
excluded because they operate a substrate rather than identify the workload.

## Lifecycle and security risks

Artifact changes create reviewed revisions. Runtime-local state is not durable
portable state. Delete retires serving identity while preserving audit and
recovery evidence. Import requires exact artifact and host identity. Artifact
bytes are digest-bound; credentials and secrets never enter desired/provider
state; network policy remains host-owned.

## Prior art and gap

OCCI Application/Component/Link/Action, CIMI System/Machine lifecycle, TOSCA
nodes/artifacts/relationships, Kubernetes Deployment and Knative Service,
Crossplane reconciliation, and Terraform/OpenTofu edge providers are all
applicable. The remaining gap is a deliberately small immutable edge
application contract with host-owned runtime, routing, credentials, and
placement.

## Experimental exit evidence

Takosumi Cloud supplies the real workload and first host only. Canonical
definition, positive/negative fixtures, compatibility and migration analysis,
security review, provider-v2 behavior, package signature, and public readback
remain required before `EdgeWorker@0.1.0` exists.

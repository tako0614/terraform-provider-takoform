# Form Proposal: ContainerService

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

Takosumi Cloud runs immutable OCI images as long-lived services. The candidate
Form describes a digest-pinned image, non-secret configuration, connections,
and an `http.request@1` Interface. Runtime, ports, public routing, probes,
scaling, placement, networking, credentials, secret injection, capacity, and
price are host decisions.

## Substrate-neutrality review

An OCI digest and application configuration have the same meaning for a
Kubernetes-based host, a managed container platform, or a local OCI runtime.
CPU, memory, replicas, ports, public exposure, and health checks are excluded:
they are operating and exposure policy, not the identity of the service
workload represented by this Form.

## Lifecycle and security risks

Image changes create reviewed revisions; some networking changes may require
replacement. Writable local filesystem is not portable durable state. Delete
retires service instances. Registry/runtime credentials and secret env/files
never enter portable desired or provider state.

## Prior art and gap

OCCI Application/Component, CIMI Machine/System, TOSCA container runtime and
artifacts, Kubernetes Deployment/Service/probes, Crossplane reconciliation,
and Terraform ECS/Cloud Run/Container Apps resources are applicable. The gap
is a digest-bound service intent smaller than Kubernetes and independent of one
managed container product.

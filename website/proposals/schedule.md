# Form Proposal: Schedule

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

Takosumi Cloud invokes one connected Resource on a recurring schedule. The
candidate Form describes expression, timezone, and the exact target
connection. Scheduler implementation, enable/disable policy, delivery transport, retry policy,
credentials, capacity, and price remain host-owned.

## Substrate-neutrality review

A five-field cron expression, timezone, and one declared invocation connection
can be implemented by an OS scheduler, Kubernetes CronJob, managed event
scheduler, or edge trigger. Retry, catch-up, overlap, enablement, jitter, and
delivery transport are excluded until they have independently testable shared
semantics rather than provider defaults.

## Lifecycle and security risks

Expression changes should update in place when the host can preserve identity.
Delete stops future delivery while retaining bounded audit. Import requires
exact scheduler and target evidence. Invocation authority and secret payloads
remain outside portable state.

## Prior art and gap

OCCI Action/Link, TOSCA policies/triggers/workflows, Kubernetes CronJob,
Crossplane managed schedulers, and Terraform scheduler/event-rule resources
are applicable; CIMI has no focused recurring invocation resource. The gap is
a small scheduler-to-Resource connection without provider event-target fields.

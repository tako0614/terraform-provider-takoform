# Form Proposal: Queue

Status: active Proposal; intended first v1alpha2 release `0.1.0`. A local
candidate package exists under `forms/candidates/v1alpha2/`, but no released
FormRef or public release identity exists yet.

## Need and boundary

The candidate Form describes durable message queues for connected producers and
consumers: queue lifecycle, narrow delivery
intent, retention constraints, and messaging capability. Broker choice,
consumer materialization, batching, retries, visibility leases, delivery delay,
credentials, placement, capacity, and price remain host-owned.

## Substrate-neutrality review

At-least-once delivery, retention, and ordering are observable semantics that
can be exercised against multiple brokers. Consumer batch size, retry count,
visibility timeout, delivery delay, maximum message size, broker binding, and
dead-letter policy are excluded because they couple producer lifecycle to host
or consumer operation.

## Lifecycle and security risks

Delivery or retention changes may require a new queue and drain. Delete or
migration can discard messages. Import requires exact broker identity and
delivery evidence. Message bodies, consumer code, and credentials never enter
portable Resource state.

## Prior art and gap

OCCI Resource/Link/Action, TOSCA messaging relationships, Kubernetes broker
operators/KEDA, Crossplane resources, and Terraform SQS/PubSub/Service Bus/
Cloudflare Queue resources are applicable; CIMI has no focused queue contract.
The gap is a deliberately narrow broker-neutral lifecycle and connection model.

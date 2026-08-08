package portableconformancev3

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// runner_edge_semantics_checks.go proves what a black-box Host API runner can
// hold a host to about the family's data and delivery model
// (spec/decisions/0020).
//
// The lane drives DESIRED STATE. It cannot put a byte into a KV namespace, read
// a row back out of a database, or wait for a queue message to be redelivered,
// so it proves none of the value models and none of the delivery guarantees
// directly. What it can prove is exactly three things, and each of them is a
// statement a host makes before any data moves:
//
//  1. which exact Interface contracts it implements, at their exact digests —
//     the difference between "this host has a KV store" and "this host
//     implements THIS consistency model, THIS value model, and THESE limits";
//  2. that the cron grammar is decided by a parser rather than by the pattern
//     the Form Definition carries, so a schedule a host stores is a schedule it
//     can fire;
//  3. that one queue has at most one consumer, which is a cardinality rule over
//     the store that no desired-state schema can express.
//
// Everything else the contracts state — convergence, streaming bodies, lossless
// SQLite values, attempt counts, dead-letter transfer — is a host obligation,
// listed as such in spec/host-api/v1alpha3.md rather than pretended to be
// proven here.

// checkEdgeInterfaceContractsAdvertised proves the host implements each pinned
// data-plane Interface at the EXACT contract the lane names.
//
// A digest that merely looks well formed is not enough, for the same reason it
// was not enough for the runtime ABI: a different digest is a different
// contract, so a host answering with one is answering about a store the lane
// did not ask for. edge.kv at one digest promises eventual consistency and
// byte values; at another it could promise anything.
//
// The other end of the loop is already closed elsewhere: each Form Definition
// carries its providedInterfaces refs, and `form-definition-exact` holds those
// bytes to the exact schemaDigest the corpus pins, so a host serving a
// definition whose interface ref moved fails there.
func (r *v3Runner) checkEdgeInterfaceContractsAdvertised() error {
	for _, pinned := range r.contract.RunnerInput.SupportProbes.DataInterfaces {
		response, err := r.request(
			http.MethodGet,
			fmt.Sprintf("%s/support/interfaces/%s/%s", r.apiBase,
				url.PathEscape(pinned.Name), url.PathEscape(pinned.Version)),
			nil, nil,
		)
		if err != nil {
			return err
		}
		if response.Status != http.StatusOK {
			return fmt.Errorf(
				"the host does not advertise the %s@%s contract: HTTP %d",
				pinned.Name, pinned.Version, response.Status,
			)
		}
		var profile map[string]any
		if err := decodeStrictResponse(response, &profile); err != nil {
			return err
		}
		if profile["apiVersion"] != supportAPIVersion || profile["kind"] != "InterfaceSupport" {
			return fmt.Errorf("%s support profile identity is invalid", pinned.Name)
		}
		reference, _ := profile["interfaceRef"].(map[string]any)
		if reference == nil {
			return fmt.Errorf("%s support profile omitted interfaceRef", pinned.Name)
		}
		if reference["name"] != pinned.Name || reference["version"] != pinned.Version {
			return fmt.Errorf(
				"%s support profile names %v/%v", pinned.Name, reference["name"], reference["version"],
			)
		}
		digest, _ := reference["schemaDigest"].(string)
		if !formpackage.ValidDigest(digest) {
			return fmt.Errorf("%s schemaDigest is not a canonical digest", pinned.Name)
		}
		if digest != pinned.SchemaDigest {
			return fmt.Errorf(
				"the host advertises %s@%s at %s; the lane requires the exact contract %s",
				pinned.Name, pinned.Version, digest, pinned.SchemaDigest,
			)
		}
	}
	r.complete("edge-interface-contracts-advertised")
	return nil
}

// cronOffenders are expressions the STRUCTURAL pattern in the Form Definition
// admits and that name no schedule at all. Each one is a different way for a
// host that stopped at the regex to store a trigger it can never fire.
var cronOffenders = []struct {
	expression string
	why        string
}{
	{"0 24 * * *", "hour 24, which is outside the 0-23 hour field"},
	{"5-1 * * * *", "an inverted minute range"},
	{"*/0 * * * *", "a zero step, which selects nothing"},
	{"0 0 32 * *", "day-of-month 32, which no month has"},
}

// checkCronGrammarEnforced proves the cron grammar is decided by a parser.
//
// The Form's `cron` pattern is a structural minimum and has to be: a Form
// Definition can carry a regex and nothing else. But the widened grammar that
// finally makes hourly and sub-hourly schedules expressible also admits shapes
// that are not schedules — an hour of 24, an inverted range, a zero step — and
// a host that accepted those would store a trigger nothing can fire, while
// another host refused it. So the refusal is asserted at BOTH pre-mutation
// gates a client meets, the advisory `validate` surface and `apply`, and
// nothing is stored either way.
//
// The positive control is the point of the whole change: `*/5 * * * *` is a
// schedule the previous grammar could not express at all, on a Form whose only
// purpose is periodic invocation. A host could otherwise pass by refusing every
// expression.
func (r *v3Runner) checkCronGrammarEnforced() error {
	worker := r.target(r.contract.RunnerInput.ModuleWorker)
	worker.Name = "gate-worker"
	base := r.target(r.contract.RunnerInput.WorkerCronTrigger)
	base.Spec = cloneJSONMap(base.Spec)
	base.Spec["worker"] = exactReference(r.target(r.contract.RunnerInput.ModuleWorker), worker.Name)

	for _, offender := range cronOffenders {
		trigger := base
		trigger.Name = "cron-grammar-trigger"
		trigger.Spec = cloneJSONMap(base.Spec)
		trigger.Spec["cron"] = offender.expression
		subject := "a WorkerCronTrigger whose cron " + offender.expression + " carries " + offender.why

		valid, diagnostics, err := r.validateResource(trigger, trigger.Spec)
		if err != nil {
			return err
		}
		if valid || diagnostics == 0 {
			return fmt.Errorf("validate accepted %s", subject)
		}
		response, err := r.prepareRequest(trigger, map[string]string{})
		if err != nil {
			return err
		}
		if err := r.expectStableError(response, "invalid_argument"); err != nil {
			return fmt.Errorf("prepare of %s: %w", subject, err)
		}
		if err := r.expectResourceAbsent(trigger); err != nil {
			return fmt.Errorf("%s mutated state: %w", subject, err)
		}
	}

	// The schedules the previous grammar could not express, accepted.
	for index, expression := range []string{"*/5 * * * *", "0 * * * *"} {
		accepted := base
		accepted.Name = "cron-grammar-trigger"
		accepted.Spec = cloneJSONMap(base.Spec)
		accepted.Spec["cron"] = expression
		valid, _, err := r.validateResource(accepted, accepted.Spec)
		if err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("validate refused the sub-hourly schedule %q", expression)
		}
		if _, _, err := r.applyResource(accepted, applyOptions{
			Create: true, IdempotencyKey: fmt.Sprintf("key-cron-accepted-%d", index),
		}, http.StatusCreated); err != nil {
			return fmt.Errorf("a WorkerCronTrigger scheduled %q: %w", expression, err)
		}
		if err := r.deleteExisting(accepted, fmt.Sprintf("key-cron-accepted-%d-delete", index)); err != nil {
			return err
		}
	}
	r.complete("cron-grammar-enforced")
	return nil
}

// checkSingleQueueConsumerEnforced proves one queue has at most one consumer.
//
// It is a cardinality rule over the STORE, so no desired-state schema can see
// it: the second consumer's own document is perfectly valid, and what makes it
// wrong is another resource that already exists. `edge.queue` states the rule
// and states why — maxRetries, retryDelaySeconds, maxConcurrency, and the
// dead-letter destination are properties of how that queue is drained, and two
// consumers would give one queue two of each with no rule deciding which
// message got which.
//
// The check is self-contained: it creates its own queue so it depends on no
// state another check left behind, and it proves BOTH directions — the second
// consumer is refused while the first is live, and the same consumer is
// accepted once the first is gone, so a host cannot pass by refusing every
// consumer.
func (r *v3Runner) checkSingleQueueConsumerEnforced() error {
	input := r.contract.RunnerInput
	worker := r.target(input.ModuleWorker)
	worker.Name = "gate-worker"

	queue := r.target(input.AtLeastOnceQueue)
	queue.Name = "single-consumer-queue"
	if _, _, err := r.applyResource(queue, applyOptions{
		Create: true, IdempotencyKey: "key-single-consumer-queue",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the queue under test: %w", err)
	}

	consumerOf := func(name string) probeTarget {
		consumer := r.target(input.QueueConsumer)
		consumer.Name = name
		consumer.Spec = cloneJSONMap(consumer.Spec)
		consumer.Spec["worker"] = exactReference(r.target(input.ModuleWorker), worker.Name)
		consumer.Spec["queue"] = exactReference(queue, queue.Name)
		return consumer
	}

	first := consumerOf("single-consumer-first")
	if _, _, err := r.applyResource(first, applyOptions{
		Create: true, IdempotencyKey: "key-single-consumer-first",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the first consumer of a queue: %w", err)
	}
	second := consumerOf("single-consumer-second")
	if err := r.refuseCreate(
		second, "invalid_argument", "key-single-consumer-second",
		"a second QueueConsumer draining a queue that already has one",
	); err != nil {
		return err
	}
	// Once the first is gone the same second consumer is representable, so the
	// refusal above was about the queue rather than about the consumer.
	if err := r.deleteExisting(first, "key-single-consumer-first-delete"); err != nil {
		return err
	}
	if _, _, err := r.applyResource(second, applyOptions{
		Create: true, IdempotencyKey: "key-single-consumer-second-accepted",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the second consumer once the first is removed: %w", err)
	}
	if err := r.deleteExisting(second, "key-single-consumer-second-delete"); err != nil {
		return err
	}
	current, _, err := r.read(queue)
	if err != nil {
		return err
	}
	response, err := r.deleteResource(queue, current.Metadata.Revision, "key-single-consumer-queue-delete", nil)
	if err != nil {
		return err
	}
	if response.Status != http.StatusNoContent {
		return errors.New("the queue under test could not be released after its consumers were removed")
	}
	r.complete("queue-single-consumer-enforced")
	return nil
}

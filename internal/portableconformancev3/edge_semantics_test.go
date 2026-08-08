package portableconformancev3

// edge_semantics_test.go holds the two attachment-claim rules of
// spec/decisions/0026 to the exact SCOPE the decision states, which is the half
// of each rule the black-box corpus cannot reach on its own.
//
// The corpus proves the cross-SPACE half of the hostname claim over real HTTP,
// because one tenant owns both runner spaces. It cannot prove the other
// boundary: requiring a host to ACCEPT a hostname a different tenant already
// serves would decide who controls a DNS name, which decision 0026 says this
// contract does not pretend to answer. That refusal-to-decide is exactly what
// makes the tenant a boundary the reference host must respect, so it is proved
// here against the host itself.

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	// claimedHostname is the canonical spelling one attachment holds, and
	// restatedHostname is the same DNS name written the two other legal ways at
	// once. A scan that compared written bytes would see two names.
	claimedHostname  = "claim.portable-conformance.invalid"
	restatedHostname = "Claim.Portable-Conformance.INVALID."
)

// domainSpec builds one Worker Custom Domain desired spec of the base worker.
func (f *aggregateFixture) domainSpec(hostname string) map[string]any {
	return map[string]any{"worker": f.ref(moduleWorkerKind, "worker"), "hostname": hostname}
}

// queueSpec builds one At Least Once Queue desired spec.
func queueSpec() map[string]any {
	return map[string]any{"messageRetentionSeconds": json.Number("345600")}
}

// consumerSpec builds one Queue Consumer of the base worker draining one queue,
// with an optional dead-letter destination.
func (f *aggregateFixture) consumerSpec(queue, deadLetter string) map[string]any {
	spec := map[string]any{
		"worker": f.ref(moduleWorkerKind, "worker"), "queue": f.ref("AtLeastOnceQueue", queue),
		"maxBatchSize": json.Number("10"), "maxBatchTimeoutSeconds": json.Number("5"),
		"maxRetries": json.Number("3"), "retryDelaySeconds": json.Number("60"),
		"maxConcurrency": json.Number("4"),
	}
	if deadLetter != "" {
		spec["deadLetterQueue"] = f.ref("AtLeastOnceQueue", deadLetter)
	}
	return spec
}

// TestHostnameClaimStopsAtTheTenant proves the boundary decision 0026 draws
// around a hostname claim: it is one tenant's, and it is the WHOLE of that
// tenant.
//
// A host that scanned its resource store without a tenant would refuse the
// second claim below — a name the contract says another tenant may hold — and
// would do it while reporting a collision with a resource that caller may not
// even read. A host that scanned only the caller's space would admit the third.
func TestHostnameClaimStopsAtTheTenant(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	f.store(workerCustomDomainKind, "holder", f.domainSpec(claimedHostname))

	// One tenant, one hostname: the collision is refused, and it is refused on
	// the CANONICAL spelling rather than on the bytes the second caller wrote.
	hostErr := f.validate(workerCustomDomainKind, "second", f.domainSpec(restatedHostname))
	f.requireCode(hostErr, "invalid_argument", "a second claim on one tenant's hostname")
	if !strings.Contains(hostErr.Message, "holder") {
		t.Fatalf("the refusal does not name the holder: %s", hostErr.Message)
	}

	// Another principal of the SAME tenant is the same claim: the rule is about
	// the tenant, never about who is holding the credential.
	f.requireCode(
		f.validateAs(referenceAlternateAuth, workerCustomDomainKind, "second", f.domainSpec(claimedHostname)),
		"invalid_argument", "a second claim by another principal of the holding tenant",
	)

	// Another TENANT is another claim. Nothing in this contract decides who
	// controls a DNS name, so a name one tenant serves is none of this scan's
	// business.
	f.requireAccepted(
		f.validateAs(referenceOtherTenantAuth, workerCustomDomainKind, "second", f.domainSpec(claimedHostname)),
		"a claim on a hostname another tenant serves",
	)

	// That acceptance is the scope and not an exemption: the other tenant's own
	// claim collides with the other tenant's own holder.
	f.storeAs(referenceOtherTenantAuth, workerCustomDomainKind, "foreign-holder", f.domainSpec(claimedHostname))
	foreignErr := f.validateAs(
		referenceOtherTenantAuth, workerCustomDomainKind, "foreign-second", f.domainSpec(restatedHostname),
	)
	f.requireCode(foreignErr, "invalid_argument", "a second claim inside the other tenant")
	if !strings.Contains(foreignErr.Message, "foreign-holder") {
		t.Fatalf("the other tenant's refusal names a resource outside its tenant: %s", foreignErr.Message)
	}
}

// TestHostnameClaimSpansEverySpaceOfOneTenant proves the other half of the same
// scope with real resources on both sides: spaces partition one tenant's
// resources and DNS does not partition with them.
func TestHostnameClaimSpansEverySpaceOfOneTenant(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	f.store(workerCustomDomainKind, "holder", f.domainSpec(claimedHostname))

	// A second space with its own complete aggregate, so the claim made there is
	// refused for the hostname rather than for anything the attachment gate says.
	elsewhere := f.inSpace(f.host.contract.RunnerInput.AlternateSpace)
	elsewhere.baseAggregate()
	f.requireCode(
		elsewhere.validate(workerCustomDomainKind, "far-domain", elsewhere.domainSpec(restatedHostname)),
		"invalid_argument", "a claim in a second space of the tenant that already serves the name",
	)
	f.requireAccepted(
		elsewhere.validateAs(
			referenceOtherTenantAuth, workerCustomDomainKind, "far-domain", elsewhere.domainSpec(claimedHostname),
		),
		"another tenant's claim in that same second space",
	)
	f.requireAccepted(
		elsewhere.validate(workerCustomDomainKind, "far-domain", elsewhere.domainSpec("other.portable-conformance.invalid")),
		"a second space claiming a name nothing serves",
	)
}

// TestDeadLetterCycleIsWalkedRatherThanPeeked proves the dead-letter rule is a
// graph walk and not a look at the destination's own back edge.
//
// A -> B -> C -> A is the shape that separates the two: the consumer being
// applied drains C and points at A, and the consumer of A points at B, so
// nothing one hop away from the destination refers back to C. A host that
// inspected only the destination's consumer would store it and build the
// infinite circulation the rule exists to prevent.
func TestDeadLetterCycleIsWalkedRatherThanPeeked(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	for _, queue := range []string{"queue-a", "queue-b", "queue-c", "queue-d"} {
		f.store("AtLeastOnceQueue", queue, queueSpec())
	}
	first := f.store(queueConsumerKind, "consumer-a", f.consumerSpec("queue-a", "queue-b"))
	f.store(queueConsumerKind, "consumer-b", f.consumerSpec("queue-b", "queue-c"))

	hostErr := f.validate(queueConsumerKind, "consumer-c", f.consumerSpec("queue-c", "queue-a"))
	f.requireCode(hostErr, "invalid_argument", "a dead-letter destination closing a three-queue cycle")
	// The message names the whole path, so an author can see which consumer to
	// change rather than only that something is circular.
	origin := relationTargetUID(first.Relations, queueRelationPointer)
	if !strings.Contains(hostErr.Message, origin) {
		t.Fatalf("the refusal does not name the queues on the cycle: %s", hostErr.Message)
	}

	// The same chain one hop longer closes nothing, and is representable: the
	// rule forbids the loop, not the depth.
	f.requireAccepted(
		f.validate(queueConsumerKind, "consumer-c", f.consumerSpec("queue-c", "queue-d")),
		"a four-queue dead-letter chain",
	)
}

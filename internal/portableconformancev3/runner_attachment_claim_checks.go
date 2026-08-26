package portableconformancev3

// runner_attachment_claim_checks.go carries the black-box evidence for the two
// attachment-claim rules of spec/decisions/0026, and for the loadable/auxiliary
// split of the reconciled module media-type set (spec/decisions/0012 and 0019).
//
// All three fail a host that decides a claim on the bytes a client wrote
// instead of on the identity those bytes name: a hostname compared as written,
// a dead-letter destination followed no further than one hop, and a bundle
// whose first module is evidence rather than code.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// canonicalClaimHostname is the canonical spelling the two hostname checks
// claim, and nonCanonicalClaimHostname is the same DNS name written the two
// other legal ways at once: uppercase letters and the trailing root dot.
const (
	canonicalClaimHostname    = "claim.portable-conformance.invalid"
	nonCanonicalClaimHostname = "Claim.Portable-Conformance.INVALID."
	// sharedClaimHostname is the one name two TENANTS hold at once. It is a
	// separate name from the one above so the two claim checks cannot interfere,
	// and it is under `.invalid` like every hostname this corpus writes: RFC 2606
	// reserves that TLD, so no host has any control question to answer about it
	// and the only thing a refusal could mean is a claim scan that reached past
	// the tenant.
	sharedClaimHostname = "shared.portable-conformance.invalid"
	// importClaimHostname is the name the ADOPTION path competes for, and
	// commitClaimHostname the name an accepted 202 competes for. Each surface gets
	// its own so the three sequences can run in any order without one's holder
	// deciding another's verdict.
	importClaimHostname = "adopted.portable-conformance.invalid"
	commitClaimHostname = "committed.portable-conformance.invalid"
	// uLabelClaimHostname is an internationalized name written as a U-label, and
	// aLabelClaimHostname is the same name written the way this lane carries it.
	// The Form's `hostname` pattern admits no non-ASCII byte, so the first is
	// refused by the Form's own grammar and the conversion belongs in the author's
	// tooling, where the Unicode table version is the author's choice rather than
	// the host's (spec/decisions/0026).
	uLabelClaimHostname = "café.portable-conformance.invalid"
	aLabelClaimHostname = "xn--caf-dma.portable-conformance.invalid"
)

// customDomainClaiming builds one Worker Custom Domain of the deployed gate
// worker under a given name and hostname.
func (r *v3Runner) customDomainClaiming(name, hostname string) probeTarget {
	return r.customDomainClaimingIn("", name, "gate-worker", hostname)
}

// customDomainClaimingIn is the same target in a named space, of a named
// worker. A reference carries no space, so the worker it names is resolved
// inside the space the attachment itself is applied to — which is what makes a
// claim in a second space a claim on a DIFFERENT aggregate, and what makes the
// hostname the only thing the two have in common.
func (r *v3Runner) customDomainClaimingIn(space, name, worker, hostname string) probeTarget {
	input := r.contract.RunnerInput
	domain := r.target(input.WorkerCustomDomain)
	domain.Name = name
	if space != "" {
		domain.Space = space
	}
	domain.Spec = cloneJSONMap(domain.Spec)
	domain.Spec["worker"] = exactReference(r.target(input.ModuleWorker), worker)
	domain.Spec["hostname"] = hostname
	return domain
}

// claimFixtureElsewhere builds a COMPLETE worker aggregate in the runner's
// second space — its own bundle, worker, version, and the deployment that
// serves fetch — and returns the custom domain target that would claim a
// hostname there, followed by everything it stands on in the order those must
// be removed.
//
// Nothing here is shared with the first space except the committed artifact,
// which is content-addressed and held by the tenant rather than by a space. The
// point of building the whole aggregate rather than reusing the first space's
// is that the attachment gate must have nothing to say about the claim: when
// the host refuses the domain below, the only thing left for it to be refusing
// is the hostname.
func (r *v3Runner) claimFixtureElsewhere(hostname string) (probeTarget, []probeTarget, error) {
	input := r.contract.RunnerInput
	space := input.AlternateSpace
	if r.artifactManifestDigest == "" {
		return probeTarget{}, nil, errors.New(
			"the second space's bundle needs the committed manifest digest; " +
				"this check runs after the artifact sequence",
		)
	}
	elsewhere := func(target probeTarget) probeTarget {
		target.Space = space
		return target
	}

	bundle := elsewhere(r.target(input.WorkerBundle.ResourceProbe))
	bundle.Name = "claim-far-bundle"
	bundle.Spec = map[string]any{"manifestDigest": r.artifactManifestDigest}
	worker := elsewhere(r.target(input.ModuleWorker))
	worker.Name = "claim-far-worker"
	version := elsewhere(r.workerVersionOfBundle("claim-far-version", worker.Name, bundle.Name, fetchHandler))
	deployment := elsewhere(r.deploymentOf("claim-far-deployment", worker.Name, version.Name))
	for _, target := range []probeTarget{bundle, worker, version, deployment} {
		if _, _, err := r.applyResource(target, applyOptions{
			Create: true, IdempotencyKey: "key-" + target.Name,
		}, http.StatusCreated); err != nil {
			return probeTarget{}, nil, fmt.Errorf("the second space's %s: %w", target.Name, err)
		}
	}
	domain := r.customDomainClaimingIn(space, "claim-far-domain", worker.Name, hostname)
	return domain, []probeTarget{deployment, version, worker, bundle}, nil
}

// prepareExpectingRewrite posts one prepare whose spec the host is expected to
// REWRITE, and returns the prepare digest together with the spec the host bound
// it to.
//
// The generic prepare helper holds a host to echoing the exact spec it was
// sent. That is the right rule for every client that already speaks the
// canonical spelling — and the wrong one for the two checks that deliberately
// do not, because canonicalization is precisely a rewrite before the digest.
// The rewrite is asserted here instead: the review must bind the digest of the
// spec the host echoed, so a host cannot answer with a canonical spec and a
// digest computed over something else.
func (r *v3Runner) prepareExpectingRewrite(
	target probeTarget,
	generation string,
) (string, map[string]any, error) {
	var headers map[string]string
	if generation != "" {
		headers = map[string]string{expectedGenerationHeader: generation}
	}
	response, err := r.prepareRequest(target, headers)
	if err != nil {
		return "", nil, err
	}
	if response.Status != http.StatusOK {
		return "", nil, fmt.Errorf("prepare HTTP %d", response.Status)
	}
	var result struct {
		Resource wireResource `json:"resource"`
		Review   struct {
			PrepareDigest string `json:"prepareDigest"`
			SpecDigest    string `json:"specDigest"`
		} `json:"review"`
	}
	if err := decodeStrictResponse(response, &result); err != nil {
		return "", nil, err
	}
	boundDigest, err := specCanonicalDigest(result.Resource.Spec)
	if err != nil {
		return "", nil, err
	}
	if result.Review.SpecDigest != boundDigest {
		return "", nil, errors.New("prepare bound a specDigest that is not the digest of the spec it echoed")
	}
	if !formpackage.ValidDigest(result.Review.PrepareDigest) {
		return "", nil, errors.New("prepare returned an invalid prepareDigest")
	}
	return result.Review.PrepareDigest, result.Resource.Spec, nil
}

// checkCustomDomainHostnameCanonicalized proves a host decides what a hostname
// IS before it stores one.
//
// DNS is case-insensitive and a trailing dot is the fully-qualified spelling of
// the same name, so the three spellings of one hostname are one hostname. A
// host that stored the bytes an author wrote would hold three different values
// for one name, and every later comparison — uniqueness above all — would be
// answered against a spelling rather than against an identity.
//
// The evidence is the stored representation: the host is applied a
// non-canonical spelling and must echo the canonical one. The second half is
// what makes it a canonicalization rather than a display convention: re-applying
// the SAME name in a THIRD spelling must move neither generation nor revision,
// because canonicalization happens before the spec is digested, so two spellings
// are one desired state (spec/decisions/0026).
func (r *v3Runner) checkCustomDomainHostnameCanonicalized() error {
	canonical := currentformmodel.CanonicalHostname(nonCanonicalClaimHostname)
	if canonical != canonicalClaimHostname {
		return fmt.Errorf(
			"the lane's own canonicalization is inconsistent: %q canonicalizes to %q, not %q",
			nonCanonicalClaimHostname, canonical, canonicalClaimHostname,
		)
	}
	domain := r.customDomainClaiming("canonical-domain", nonCanonicalClaimHostname)
	prepareDigest, preparedSpec, err := r.prepareExpectingRewrite(domain, "")
	if err != nil {
		return fmt.Errorf("preparing a custom domain written in a non-canonical spelling: %w", err)
	}
	if hostname, _ := preparedSpec["hostname"].(string); hostname != canonicalClaimHostname {
		return fmt.Errorf(
			"prepare bound hostname %q for the requested spelling %q; canonicalization happens before the digest",
			hostname, nonCanonicalClaimHostname,
		)
	}
	created, _, err := r.applyResource(domain, applyOptions{
		Create: true, IdempotencyKey: "key-canonical-domain", PrepareDigest: prepareDigest,
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("a custom domain written in a non-canonical spelling: %w", err)
	}
	stored, _ := created.Spec["hostname"].(string)
	if stored != canonicalClaimHostname {
		return fmt.Errorf(
			"the host stored hostname %q for the applied spelling %q; one DNS name has one stored spelling, %q",
			stored, nonCanonicalClaimHostname, canonicalClaimHostname,
		)
	}
	read, _, err := r.read(domain)
	if err != nil {
		return err
	}
	if hostname, _ := read.Spec["hostname"].(string); hostname != canonicalClaimHostname {
		return fmt.Errorf("a read echoed hostname %q, not the canonical %q", hostname, canonicalClaimHostname)
	}

	// A third spelling of one name is one desired state, so nothing moves.
	restated := r.customDomainClaiming("canonical-domain", "CLAIM.portable-conformance.invalid")
	restatedDigest, _, err := r.prepareExpectingRewrite(restated, created.Metadata.Generation)
	if err != nil {
		return fmt.Errorf("preparing a re-application in another spelling: %w", err)
	}
	again, _, err := r.applyResource(restated, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-canonical-domain-restate",
		PrepareDigest:      restatedDigest,
	}, http.StatusOK)
	if err != nil {
		return fmt.Errorf("re-applying one hostname in another spelling: %w", err)
	}
	if again.Metadata.Generation != created.Metadata.Generation {
		return fmt.Errorf(
			"re-applying one hostname in another spelling moved generation %s -> %s; the two spellings are one desired state",
			created.Metadata.Generation, again.Metadata.Generation,
		)
	}
	r.complete("custom-domain-hostname-canonicalized")
	return nil
}

// checkCustomDomainHostnameClaimUnique proves one hostname has one answer
// across every space of one tenant.
//
// It is a rule over the STORE, so no desired-state schema can see it: the
// second attachment's document is perfectly valid, and what makes it wrong is
// an attachment that already serves the name. Two live attachments on one
// hostname would leave the host with two answers to one request and no rule
// choosing between them.
//
// The SPACE is the half of the rule a same-space collision cannot prove. Spaces
// partition one tenant's resources and DNS does not partition with them, so a
// host that enforced uniqueness inside one space would pass a same-space corpus
// and still route one DNS name two ways — which is why the collision below is
// driven from a second space, against its own worker, its own version and its
// own deployment, while the first space's holder is live. It is driven in both
// directions, so the rule cannot be a one-way scan: the second space is refused
// while the first space holds the name, and the first space is refused while the
// second space holds it.
//
// Both verdicts are driven too, so a host cannot pass by refusing every second
// custom domain: each colliding claim is refused while a holder is live and
// accepted once that holder is gone. The colliding claim is written in a
// DIFFERENT spelling from the one the store holds, so a host comparing the
// written bytes admits it and fails here.
func (r *v3Runner) checkCustomDomainHostnameClaimUnique() error {
	colliding := r.customDomainClaiming("claim-collision-domain", canonicalClaimHostname)
	if err := r.refuseClaimAtPrepare(
		colliding,
		"a second WorkerCustomDomain claiming a hostname another attachment already serves",
	); err != nil {
		return err
	}
	// The same claim spelled the way the first one was WRITTEN rather than the
	// way it was stored is the same collision, and is refused the same way. A
	// host comparing written bytes admits exactly this one.
	restated := r.customDomainClaiming("claim-collision-domain", nonCanonicalClaimHostname)
	response, err := r.prepareRequest(restated, nil)
	if err != nil {
		return fmt.Errorf("preparing a colliding claim in another spelling: %w", err)
	}
	subject := "a second WorkerCustomDomain claiming the same hostname in another spelling"
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	if err := r.expectResourceAbsent(restated); err != nil {
		return fmt.Errorf("%s mutated state: %w", subject, err)
	}

	// A SECOND SPACE of the same tenant, with an aggregate of its own, claiming
	// the name the first space serves. A host whose uniqueness rule is per space
	// stores this one and answers one hostname two ways.
	far, foundation, err := r.claimFixtureElsewhere(canonicalClaimHostname)
	if err != nil {
		return err
	}
	if err := r.refuseClaimAtPrepare(
		far,
		"a WorkerCustomDomain in a second space claiming a hostname the tenant already serves",
	); err != nil {
		return err
	}

	// Releasing the holder makes the claim representable, so the refusal was
	// about the hostname rather than about the resource or about the space.
	if err := r.deleteExisting(
		r.customDomainClaiming("canonical-domain", canonicalClaimHostname),
		"key-canonical-domain-delete",
	); err != nil {
		return err
	}
	if _, _, err := r.applyResource(far, applyOptions{
		Create: true, IdempotencyKey: "key-claim-far-accepted",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the second space's claim once the first space released it: %w", err)
	}
	// The same refusal in the other direction: the space that now holds the name
	// is the second one, and the first space is the one that may not take it.
	if err := r.refuseClaimAtPrepare(
		colliding,
		"a WorkerCustomDomain claiming a hostname another SPACE of the tenant serves",
	); err != nil {
		return err
	}
	if err := r.deleteExisting(far, "key-claim-far-delete"); err != nil {
		return err
	}
	for _, target := range foundation {
		if err := r.deleteExisting(target, "key-"+target.Name+"-delete"); err != nil {
			return err
		}
	}

	if _, _, err := r.applyResource(colliding, applyOptions{
		Create: true, IdempotencyKey: "key-claim-collision-accepted",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the second claim once the first is released: %w", err)
	}
	if err := r.deleteExisting(colliding, "key-claim-collision-delete"); err != nil {
		return err
	}
	r.complete("custom-domain-hostname-claim-unique")
	return nil
}

// refuseClaimAtPrepare reflects the stable constraint lifecycle: a claim is
// knowable during admission, so a live conflict must be refused before a
// prepare digest can be minted. Apply/import still revalidate the same rule at
// commit time.
func (r *v3Runner) refuseClaimAtPrepare(target probeTarget, subject string) error {
	response, err := r.prepareRequest(target, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	if err := r.expectResourceAbsent(target); err != nil {
		return fmt.Errorf("%s mutated state: %w", subject, err)
	}
	return nil
}

// checkCustomDomainHostnameClaimStopsAtTheTenant proves the other edge of the
// same scope: the claim reaches every space of one tenant, and NOTHING FURTHER.
//
// This is the only rule in the lane whose correct answer is that the operation
// WORKS, and that polarity is why it went unmeasured. Every other boundary check
// drives a refusal, so a host enforcing this claim over an unpartitioned store
// passed the whole corpus while letting whichever tenant asked first deny a DNS
// name to everyone else on the host — the exact outcome decision 0026 rule 2
// says a host must not produce, and the reason decision 0028 put the tenant in
// the resource address rather than in a late filter.
//
// A check that only required the second tenant's claim to succeed would be
// satisfied by a host with no claim rule at all, so both polarities are driven
// against the same live pair: the second tenant's claim is ACCEPTED while the
// first tenant serves the name, and a THIRD claim inside the second tenant is
// REFUSED while the second tenant serves it. One hostname, two tenants, two
// answers, and inside either tenant still exactly one.
//
// The refusal is also read for what it discloses. Decision 0026 requires the
// message to name the holder so an author can act on it without a second
// request; a host scanning host-wide would therefore name a resource in another
// tenant, which is the membership oracle the read boundary spends its own check
// eliminating. So the refusal must name the caller's own holder and must not
// name the other tenant's.
//
// The second tenant stands up a COMPLETE aggregate of its own — bundle, worker,
// version, and the deployment that serves fetch — so the attachment gate has
// nothing to say and the only thing left for a host to be deciding is the
// hostname. Its bundle references the manifest digest that tenant committed for
// itself earlier in the run; a content address is not a capability, so a
// manifest the FIRST tenant committed would not resolve here at all.
func (r *v3Runner) checkCustomDomainHostnameClaimStopsAtTheTenant() error {
	input := r.contract.RunnerInput
	if r.artifactManifestDigest == "" {
		return errors.New(
			"the second tenant's bundle needs the committed manifest digest; " +
				"this check runs after the artifact sequence",
		)
	}

	// The first tenant serves the name, on the aggregate the gate check deployed.
	held := r.customDomainClaiming("claim-tenant-holder", sharedClaimHostname)
	heldCreated, _, err := r.applyResource(held, applyOptions{
		Create: true, IdempotencyKey: "key-claim-tenant-holder",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the first tenant's claim on %s: %w", sharedClaimHostname, err)
	}

	// An aggregate of the second tenant's own, in the same space and under
	// different names: the tenant is the only thing that differs.
	bundle := r.target(input.WorkerBundle.ResourceProbe)
	bundle.Name = "claim-tenant-bundle"
	bundle.Spec = map[string]any{"manifestDigest": r.artifactManifestDigest}
	worker := r.target(input.ModuleWorker)
	worker.Name = "claim-tenant-worker"
	version := r.workerVersionOfBundle("claim-tenant-version", worker.Name, bundle.Name, fetchHandler)
	deployment := r.deploymentOf("claim-tenant-deployment", worker.Name, version.Name)
	for _, target := range []probeTarget{bundle, worker, version, deployment} {
		response, err := r.applyAs(r.alternateTenantToken, target, applyOptions{
			Create: true, IdempotencyKey: "key-" + target.Name,
		})
		if err != nil {
			return err
		}
		if _, err := decodeResource(response, http.StatusCreated); err != nil {
			return fmt.Errorf("the second tenant's %s: %w", target.Name, err)
		}
	}

	// The claim itself. This MUST be accepted: what one tenant may hold in DNS is
	// authority this contract does not answer, and a host answering it out of an
	// unpartitioned store answers it for everybody.
	foreign := r.customDomainClaimingIn("", "claim-tenant-domain", worker.Name, sharedClaimHostname)
	accepted, err := r.applyAs(r.alternateTenantToken, foreign, applyOptions{
		Create: true, IdempotencyKey: "key-claim-tenant-domain",
	})
	if err != nil {
		return err
	}
	foreignCreated, err := decodeResource(accepted, http.StatusCreated)
	if err != nil {
		return fmt.Errorf(
			"a second tenant claiming hostname %s while another tenant serves it: %w; the claim is unique "+
				"per tenant and stops there, so a host scanning its whole store lets whoever asked first "+
				"deny a DNS name to every other tenant",
			sharedClaimHostname, err,
		)
	}
	if foreignCreated.Metadata.UID == heldCreated.Metadata.UID {
		return fmt.Errorf(
			"the two tenants' claims on %s share the host-issued uid %s; they are one record",
			sharedClaimHostname, heldCreated.Metadata.UID,
		)
	}

	// Both live, each read by its holder.
	heldUID, err := r.readUIDAs(r.token, held, "the first tenant reading its own claim")
	if err != nil {
		return err
	}
	foreignUID, err := r.readUIDAs(r.alternateTenantToken, foreign, "the second tenant reading its own claim")
	if err != nil {
		return err
	}
	if heldUID != heldCreated.Metadata.UID || foreignUID != foreignCreated.Metadata.UID {
		return fmt.Errorf(
			"one hostname read back as uid %s for the first tenant and %s for the second, want %s and %s",
			heldUID, foreignUID, heldCreated.Metadata.UID, foreignCreated.Metadata.UID,
		)
	}

	// Inside either tenant it is still exactly one claim, so a host that simply
	// stopped comparing hostnames fails here rather than passing the leg above.
	inside := r.customDomainClaimingIn("", "claim-tenant-domain-again", worker.Name, sharedClaimHostname)
	refused, err := r.prepareRequestAs(r.alternateTenantToken, inside, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(refused, "invalid_argument"); err != nil {
		return fmt.Errorf("a second claim on %s inside the tenant that serves it: %w", sharedClaimHostname, err)
	}
	// And the refusal names the caller's OWN holder. A host-wide scan would name
	// the other tenant's, which tells a stranger that a resource of that name
	// exists on this host and is not theirs.
	message := strings.TrimSpace(string(refused.Body))
	if !strings.Contains(message, foreign.Name) {
		return fmt.Errorf(
			"the refusal does not name the caller's own holder %q, so an author cannot act on it: %s",
			foreign.Name, message,
		)
	}
	if strings.Contains(message, held.Name) {
		return fmt.Errorf(
			"the refusal names %q, a resource in ANOTHER tenant: %s; a claim refusal may not disclose "+
				"what a caller cannot read",
			held.Name, message,
		)
	}
	if err := r.refuseClaimAtPrepare(
		r.customDomainClaiming("claim-tenant-holder-again", sharedClaimHostname),
		"a second claim on one hostname inside the FIRST tenant",
	); err != nil {
		return err
	}

	// Independently deletable: releasing one tenant's claim leaves the other's
	// exactly where it was.
	if err := r.deleteExisting(held, "key-claim-tenant-holder-delete"); err != nil {
		return err
	}
	survivor, err := r.readUIDAs(r.alternateTenantToken, foreign, "the second tenant after the first released")
	if err != nil {
		return err
	}
	if survivor != foreignCreated.Metadata.UID {
		return fmt.Errorf(
			"releasing the first tenant's claim moved the second tenant's from uid %s to %s",
			foreignCreated.Metadata.UID, survivor,
		)
	}
	if err := r.deleteExistingAs(
		r.alternateTenantToken, foreign, "key-claim-tenant-domain-delete",
	); err != nil {
		return err
	}
	for _, target := range []probeTarget{deployment, version, worker, bundle} {
		if err := r.deleteExistingAs(r.alternateTenantToken, target, "key-"+target.Name+"-delete"); err != nil {
			return err
		}
	}
	r.complete("custom-domain-hostname-claim-stops-at-the-tenant")
	return nil
}

// checkDeadLetterCycleRejected proves an exhausted message comes to rest.
//
// `edge.queue` moves a message that exhausts its retries to the dead-letter
// queue as a NEW message whose attempt count starts again at 1
// (spec/decisions/0020). A dead-letter destination that drains back to the
// origin therefore builds a loop nothing terminates: maxRetries bounds one
// message's deliveries and nothing bounds the cycle.
//
// Four shapes are driven, and each one closes a rule a host could otherwise
// hold in a cheaper way than the decision states.
//
// A destination resolving to the consumer's own queue is the one-hop cycle, the
// only shape a FIELD comparison inside one document can see. A -> B -> A adds
// the store: a host that compared the proposed destination against nothing but
// the applied document admits it. A -> B -> C -> A is what separates a graph
// WALK from an immediate back-edge test — the consumer being applied drains C
// and points at A, and the consumer of A points at B, so nothing one hop from
// the destination refers back to C, and a host that inspected only the
// destination's own consumer stores the infinite circulation. A -> B -> C -> D
// is accepted, so a host cannot pass by refusing every dead-letter queue, nor
// by refusing chains once they are longer than the shapes above.
// queueConsumerOf builds one Queue Consumer of the deployed gate worker,
// draining one named queue and dead-lettering into another.
func (r *v3Runner) queueConsumerOf(name string, drains, deadLetter probeTarget) probeTarget {
	input := r.contract.RunnerInput
	consumer := r.target(input.QueueConsumer)
	consumer.Name = name
	consumer.Spec = cloneJSONMap(consumer.Spec)
	consumer.Spec["worker"] = exactReference(r.target(input.ModuleWorker), "gate-worker")
	consumer.Spec["queue"] = exactReference(drains, drains.Name)
	consumer.Spec["deadLetterQueue"] = exactReference(deadLetter, deadLetter.Name)
	return consumer
}

// liveQueue creates one At-Least-Once Queue and returns the target it was
// created under.
func (r *v3Runner) liveQueue(name, key string) (probeTarget, error) {
	queue := r.target(r.contract.RunnerInput.AtLeastOnceQueue)
	queue.Name = name
	if _, _, err := r.applyResource(queue, applyOptions{
		Create: true, IdempotencyKey: key,
	}, http.StatusCreated); err != nil {
		return probeTarget{}, fmt.Errorf("the %s queue: %w", name, err)
	}
	return queue, nil
}

func (r *v3Runner) checkDeadLetterCycleRejected() error {
	queueOf := r.liveQueue
	first, err := queueOf("dead-letter-queue-a", "key-dead-letter-queue-a")
	if err != nil {
		return err
	}
	second, err := queueOf("dead-letter-queue-b", "key-dead-letter-queue-b")
	if err != nil {
		return err
	}

	consumerOf := r.queueConsumerOf

	// One hop: the destination resolves to the queue this consumer drains.
	if err := r.refuseCreate(
		consumerOf("dead-letter-self", first, first),
		"invalid_argument", "key-dead-letter-self",
		"a QueueConsumer whose dead-letter queue is the queue it drains",
	); err != nil {
		return err
	}

	// Two hops: A's consumer dead-letters to B, and B's consumer would
	// dead-letter back to A. Each document is valid on its own.
	forward := consumerOf("dead-letter-forward", first, second)
	if _, _, err := r.applyResource(forward, applyOptions{
		Create: true, IdempotencyKey: "key-dead-letter-forward",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a consumer dead-lettering onward to another queue: %w", err)
	}
	if err := r.refuseCreate(
		consumerOf("dead-letter-back", second, first),
		"invalid_argument", "key-dead-letter-back",
		"a QueueConsumer closing a dead-letter cycle through another queue",
	); err != nil {
		return err
	}

	// The same consumer with an acyclic destination is accepted, so the refusal
	// was about the cycle rather than about dead-lettering. The chain is now
	// A -> B -> C.
	third, err := queueOf("dead-letter-queue-c", "key-dead-letter-queue-c")
	if err != nil {
		return err
	}
	onward := consumerOf("dead-letter-back", second, third)
	if _, _, err := r.applyResource(onward, applyOptions{
		Create: true, IdempotencyKey: "key-dead-letter-acyclic",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a consumer dead-lettering to a queue that closes no cycle: %w", err)
	}

	// Three hops: C's consumer would dead-letter to A, closing A -> B -> C -> A.
	// Nothing one hop from the destination points back at C — the consumer of A
	// dead-letters to B — so only a host that WALKS the graph refuses this, and a
	// host that inspects the destination's consumer for a back edge stores it.
	if err := r.refuseCreate(
		consumerOf("dead-letter-close", third, first),
		"invalid_argument", "key-dead-letter-close",
		"a QueueConsumer closing a three-queue dead-letter cycle",
	); err != nil {
		return err
	}

	// The same consumer one hop further out closes nothing and is accepted: the
	// rule forbids the loop, not the length of the chain, so a host that refuses
	// every deep dead-letter graph fails here.
	fourth, err := queueOf("dead-letter-queue-d", "key-dead-letter-queue-d")
	if err != nil {
		return err
	}
	acyclic := consumerOf("dead-letter-close", third, fourth)
	if _, _, err := r.applyResource(acyclic, applyOptions{
		Create: true, IdempotencyKey: "key-dead-letter-long-chain",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a consumer extending an acyclic dead-letter chain to four queues: %w", err)
	}

	// A DIAMOND: a fifth queue whose consumer dead-letters into C, which B's
	// consumer already dead-letters into. The graph is acyclic — E -> C -> D — so
	// it must be accepted, and until it is driven every destination this check
	// ever accepted had no consumer and no other source. A host that never walks
	// the graph and instead refuses any destination that already has a consumer,
	// or any destination something already points at, passes every refusal above
	// and fails here. Out-degree is what a queue's single consumer bounds
	// (decision 0020); in-degree is unbounded, and nothing about two chains
	// meeting stops a message coming to rest.
	fifth, err := queueOf("dead-letter-queue-e", "key-dead-letter-queue-e")
	if err != nil {
		return err
	}
	diamond := consumerOf("dead-letter-diamond", fifth, third)
	if _, _, err := r.applyResource(diamond, applyOptions{
		Create: true, IdempotencyKey: "key-dead-letter-diamond",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf(
			"a second consumer dead-lettering into a queue another consumer already dead-letters into: %w; "+
				"the rule forbids a cycle, and two acyclic chains meeting is not one", err,
		)
	}

	for _, consumer := range []probeTarget{diamond, acyclic, onward, forward} {
		if err := r.deleteExisting(consumer, "key-"+consumer.Name+"-delete"); err != nil {
			return err
		}
	}
	for _, queue := range []probeTarget{fifth, fourth, third, second, first} {
		if err := r.deleteExisting(queue, "key-"+queue.Name+"-delete"); err != nil {
			return err
		}
	}
	r.complete("dead-letter-cycle-rejected")
	return nil
}

// refuseImport drives one create-intent adoption the host must refuse before
// any mutation, and proves nothing was stored under the name it addressed.
func (r *v3Runner) refuseImport(target probeTarget, code, nativeID, key, subject string) error {
	response, err := r.importResource(target, importOptions{
		NativeID: nativeID, IdempotencyKey: key, Create: true,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(response, code); err != nil {
		return fmt.Errorf("%s: %w", subject, err)
	}
	if err := r.expectResourceAbsent(target); err != nil {
		return fmt.Errorf("%s mutated state: %w", subject, err)
	}
	return nil
}

// checkAttachmentClaimDecidedOnImport proves the two claim rules are decided on
// the ADOPTION surface as well as on apply.
//
// Decision 0026 says both refusals land "on `apply` and on `import` alike", and
// the corpus measured only the first. That is not a formality: import is the
// surface that mints a resource for something that already exists outside the
// host, so a host that skipped the claim scan there would adopt a second
// attachment onto a hostname it already answers, or adopt a consumer that closes
// a dead-letter cycle — and adoption is precisely the path where the author is
// least likely to be looking, because the configuration they are adopting was
// already running. A host validating the whole gauntlet on apply and treating
// import as "write what the backend already has" is the ordinary shape of this
// mistake.
//
// Both polarities are driven for both rules, so a host cannot pass by refusing
// every adoption: the colliding hostname and the closing consumer are refused
// while the state that makes them wrong is live, and the SAME adoptions land
// once it is not.
func (r *v3Runner) checkAttachmentClaimDecidedOnImport() error {
	holder := r.customDomainClaiming("import-claim-holder", importClaimHostname)
	if _, _, err := r.applyResource(holder, applyOptions{
		Create: true, IdempotencyKey: "key-import-claim-holder",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the attachment that serves %s: %w", importClaimHostname, err)
	}
	// Written in a spelling the store does not hold, so a host comparing bytes
	// rather than the canonical name adopts it.
	adopting := r.customDomainClaiming("import-claim-adopted", "Adopted.Portable-Conformance.INVALID.")
	if err := r.refuseImport(
		adopting, "invalid_argument", "native/import-claim", "key-import-claim-refused",
		"adopting a WorkerCustomDomain onto a hostname a live attachment already serves",
	); err != nil {
		return err
	}
	if err := r.deleteExisting(holder, "key-import-claim-holder-delete"); err != nil {
		return err
	}
	adopted, err := r.importResource(adopting, importOptions{
		NativeID: "native/import-claim", IdempotencyKey: "key-import-claim-accepted", Create: true,
	})
	if err != nil {
		return err
	}
	adoptedResource, err := decodeResource(adopted, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("adopting the same hostname once the holder released it: %w", err)
	}
	if stored, _ := adoptedResource.Spec["hostname"].(string); stored != importClaimHostname {
		return fmt.Errorf(
			"an adoption stored hostname %q for the spelling it was handed; canonicalization is at the host's "+
				"single materialization entry point, so import reaches it too, want %q",
			stored, importClaimHostname,
		)
	}
	if err := r.deleteExisting(adopting, "key-import-claim-adopted-delete"); err != nil {
		return err
	}

	first, err := r.liveQueue("import-dead-letter-a", "key-import-dead-letter-a")
	if err != nil {
		return err
	}
	second, err := r.liveQueue("import-dead-letter-b", "key-import-dead-letter-b")
	if err != nil {
		return err
	}
	forward := r.queueConsumerOf("import-dead-letter-forward", first, second)
	if _, _, err := r.applyResource(forward, applyOptions{
		Create: true, IdempotencyKey: "key-import-dead-letter-forward",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("a consumer dead-lettering onward: %w", err)
	}
	if err := r.refuseImport(
		r.queueConsumerOf("import-dead-letter-back", second, first),
		"invalid_argument", "native/import-dead-letter", "key-import-dead-letter-refused",
		"adopting a QueueConsumer that closes a dead-letter cycle",
	); err != nil {
		return err
	}
	third, err := r.liveQueue("import-dead-letter-c", "key-import-dead-letter-c")
	if err != nil {
		return err
	}
	acyclic := r.queueConsumerOf("import-dead-letter-back", second, third)
	onward, err := r.importResource(acyclic, importOptions{
		NativeID: "native/import-dead-letter", IdempotencyKey: "key-import-dead-letter-accepted", Create: true,
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(onward, http.StatusCreated); err != nil {
		return fmt.Errorf("adopting a consumer whose dead-letter destination closes no cycle: %w", err)
	}
	for _, consumer := range []probeTarget{acyclic, forward} {
		if err := r.deleteExisting(consumer, "key-"+consumer.Name+"-delete"); err != nil {
			return err
		}
	}
	for _, queue := range []probeTarget{third, second, first} {
		if err := r.deleteExisting(queue, "key-"+queue.Name+"-delete"); err != nil {
			return err
		}
	}
	r.complete("attachment-claim-decided-on-import")
	return nil
}

// checkAttachmentClaimRevalidatedAtCommit proves both claim rules are re-derived
// when an accepted `202` COMMITS, rather than decided once at accept time.
//
// A claim is a statement about the store, and the store moves between accept and
// commit — which is the whole reason decision 0016 rule 9 and decision 0015
// rule 10 exist, and decision 0026 says the same of these two. The corpus proved
// it for relations and for the delete scan and never for a claim, so a host that
// evaluated the hostname and the dead-letter walk in the accepting request and
// treated the commit as "write what was approved" passed everything. What it
// produces is two live attachments on one DNS name, or an unbounded redelivery
// loop — built, in both cases, by two ordinary requests that were each correct
// when they were made.
//
// Each half is driven the same way: a mutation is accepted while the store makes
// it legal, the store is then moved by a SYNCHRONOUS request that is itself
// legal, and the accepted operation must terminate `invalid_argument` and commit
// nothing. The synchronous request is the positive control — it proves the state
// the commit is refused for was reachable — and the resource it created must
// survive the refusal.
func (r *v3Runner) checkAttachmentClaimRevalidatedAtCommit() error {
	pending := r.customDomainClaiming("commit-claim-pending", commitClaimHostname)
	accepted, err := r.apply(pending, applyOptions{
		Create: true, IdempotencyKey: "key-commit-claim-async",
		ExtraHeaders: map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	claimOperation, err := acceptedOperation(accepted, "a custom domain claiming a free hostname")
	if err != nil {
		return err
	}
	taker := r.customDomainClaiming("commit-claim-taker", commitClaimHostname)
	if _, _, err := r.applyResource(taker, applyOptions{
		Create: true, IdempotencyKey: "key-commit-claim-taker",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("taking the hostname while the accepted claim is pending: %w", err)
	}
	terminal, err := r.pollOperation(claimOperation)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(terminal, "invalid_argument"); err != nil {
		return fmt.Errorf(
			"an accepted claim on %s that another attachment took while it was pending: %w; a claim is a "+
				"statement about the store, and the store moved",
			commitClaimHostname, err,
		)
	}
	if err := r.expectResourceAbsent(pending); err != nil {
		return fmt.Errorf("a claim refused at commit still committed: %w", err)
	}
	if _, _, err := r.read(taker); err != nil {
		return fmt.Errorf("the attachment that legitimately took the hostname did not survive: %w", err)
	}
	if err := r.deleteExisting(taker, "key-commit-claim-taker-delete"); err != nil {
		return err
	}

	// The dead-letter graph, moved the same way. The accepted consumer adds the
	// edge B -> A, which closes no cycle when it is accepted because nothing
	// leaves A; the synchronous consumer adds A -> B, which closes no cycle when
	// IT is applied because the first is not committed. Together they are a loop,
	// and only the commit can see it.
	first, err := r.liveQueue("commit-dead-letter-a", "key-commit-dead-letter-a")
	if err != nil {
		return err
	}
	second, err := r.liveQueue("commit-dead-letter-b", "key-commit-dead-letter-b")
	if err != nil {
		return err
	}
	pendingConsumer := r.queueConsumerOf("commit-dead-letter-pending", second, first)
	acceptedConsumer, err := r.apply(pendingConsumer, applyOptions{
		Create: true, IdempotencyKey: "key-commit-dead-letter-async",
		ExtraHeaders: map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	cycleOperation, err := acceptedOperation(acceptedConsumer, "a consumer whose dead-letter destination is free")
	if err != nil {
		return err
	}
	closing := r.queueConsumerOf("commit-dead-letter-closing", first, second)
	if _, _, err := r.applyResource(closing, applyOptions{
		Create: true, IdempotencyKey: "key-commit-dead-letter-closing",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("the consumer that closes the cycle only in combination: %w", err)
	}
	cycleTerminal, err := r.pollOperation(cycleOperation)
	if err != nil {
		return err
	}
	if err := requireTerminalOperationError(cycleTerminal, "invalid_argument"); err != nil {
		return fmt.Errorf(
			"an accepted consumer whose dead-letter destination acquired a path home while it was pending: %w", err,
		)
	}
	if err := r.expectResourceAbsent(pendingConsumer); err != nil {
		return fmt.Errorf("a dead-letter cycle refused at commit still committed: %w", err)
	}
	if _, _, err := r.read(closing); err != nil {
		return fmt.Errorf("the consumer that was legitimately applied did not survive: %w", err)
	}
	if err := r.deleteExisting(closing, "key-commit-dead-letter-closing-delete"); err != nil {
		return err
	}
	for _, queue := range []probeTarget{second, first} {
		if err := r.deleteExisting(queue, "key-"+queue.Name+"-delete"); err != nil {
			return err
		}
	}
	r.complete("attachment-claim-revalidated-at-commit")
	return nil
}

// acceptedOperation reads the pending Operation id out of one 202.
func acceptedOperation(response wireResponse, subject string) (string, error) {
	if response.Status != http.StatusAccepted {
		return "", fmt.Errorf(
			"%s: async apply HTTP %d, want 202; body=%s",
			subject, response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(response, &envelope); err != nil {
		return "", err
	}
	if envelope.Operation.Done {
		return "", fmt.Errorf("%s: the 202 was already terminal, so nothing could be revalidated", subject)
	}
	return envelope.Operation.ID, nil
}

// checkCustomDomainULabelRefused proves an internationalized hostname travels as
// its A-label, and that the host REFUSES the other form rather than converting
// it.
//
// This is the one half of decision 0026's canonicalization rule the corpus never
// drove, and it is the half where a host doing something helpful is the defect.
// IDNA/UTS-46 mapping inside a host would make that host's Unicode table version
// part of the portable contract: two conforming hosts on two table versions can
// canonicalize one U-label to two different names, so the same desired state
// would claim two different hostnames depending on where it was applied — the
// privately-decided meaning this lane refuses everywhere else. Converting is
// also the obvious thing to reach for, because every mainstream language ships
// an IDNA function and refusing looks unfriendly.
//
// The refusal is driven while the name is FREE, so nothing but the Form's own
// grammar can produce it, and at both pre-mutation gates a client meets. The
// permissive half is the same name written as its A-label: accepted, and stored
// byte-for-byte as written, so a host that refuses anything unusual-looking — or
// that rewrites the A-label back into something else — fails here.
func (r *v3Runner) checkCustomDomainULabelRefused() error {
	offender := r.customDomainClaiming("u-label-domain", uLabelClaimHostname)
	subject := "a WorkerCustomDomain whose hostname is written as a U-label"
	valid, diagnostics, err := r.validateResource(offender, offender.Spec)
	if err != nil {
		return err
	}
	if valid || diagnostics == 0 {
		return fmt.Errorf(
			"validate accepted %s; the Form's hostname pattern admits no non-ASCII byte, and a host that maps "+
				"one to its A-label puts its own Unicode table version into the portable contract",
			subject,
		)
	}
	refused, err := r.prepareRequest(offender, map[string]string{})
	if err != nil {
		return err
	}
	if err := r.expectStableError(refused, "invalid_argument"); err != nil {
		return fmt.Errorf("prepare of %s: %w", subject, err)
	}
	if err := r.expectResourceAbsent(offender); err != nil {
		return fmt.Errorf("%s mutated state: %w", subject, err)
	}

	// The same name, written the way this lane carries it.
	aLabel := r.customDomainClaiming("a-label-domain", aLabelClaimHostname)
	created, _, err := r.applyResource(aLabel, applyOptions{
		Create: true, IdempotencyKey: "key-a-label-domain",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("a WorkerCustomDomain whose hostname is an A-label: %w", err)
	}
	if stored, _ := created.Spec["hostname"].(string); stored != aLabelClaimHostname {
		return fmt.Errorf(
			"the host stored hostname %q for the A-label it was given, want %q; an A-label is already the "+
				"canonical spelling and nothing may rewrite it",
			stored, aLabelClaimHostname,
		)
	}
	if err := r.deleteExisting(aLabel, "key-a-label-domain-delete"); err != nil {
		return err
	}
	r.complete("custom-domain-u-label-refused")
	return nil
}

// checkBundleMainModuleIsLoadable proves the manifest and the runtime agree on
// what a module is.
//
// The published manifest schema admits `application/source-map+json` in
// `modules` beside executable code and cannot tell the two apart, while the
// runtime contract never imports one. A host that let a source map be
// `mainModule` would commit a bundle whose first module the runtime refuses to
// instantiate — a Worker built out of modules no conforming runtime can load,
// discovered at deploy rather than at commit (spec/decisions/0012, 0014, 0019).
//
// Both directions are driven from ONE module set, so the only difference
// between the refusal and the acceptance is which module `mainModule` names:
// the auxiliary module still sits in the accepted bundle, which is what makes
// this a rule about loading rather than about carrying.
func (r *v3Runner) checkBundleMainModuleIsLoadable() error {
	bundle := r.contract.RunnerInput.WorkerBundle
	modules, _ := bundle.Manifest["modules"].([]any)
	code, _ := modules[0].(map[string]any)
	if code == nil {
		return errors.New("workerBundle probe declares no module")
	}
	codeName, _ := code["name"].(string)
	sourceMapBytes := []byte("{\"version\":3,\"sources\":[\"" + codeName + "\"],\"mappings\":\"\"}\n")
	sourceMapName := codeName + ".map"
	auxiliary := map[string]any{
		"name":      sourceMapName,
		"mediaType": "application/source-map+json",
		"size":      len(sourceMapBytes),
		"digest":    formpackage.DigestBytes(sourceMapBytes),
	}
	manifestWithMain := func(mainModule string) map[string]any {
		return map[string]any{
			"apiVersion": artifactAPIVersion,
			"kind":       "WorkerBundle",
			"mainModule": mainModule,
			"modules":    []any{code, auxiliary},
		}
	}

	refused, err := r.startArtifactUploadRaw(manifestWithMain(sourceMapName), "key-auxiliary-main-module")
	if err != nil {
		return err
	}
	if err := r.expectStableError(refused, "artifact_invalid"); err != nil {
		return fmt.Errorf("a WorkerBundle whose mainModule is an auxiliary module: %w", err)
	}

	// The same bundle, still carrying the source map, with a loadable main
	// module: accepted and committed.
	digest, err := r.uploadAndCommitManifest(manifestWithMain(codeName), map[string][]byte{
		formpackage.DigestBytes([]byte(bundle.ModuleSource)): []byte(bundle.ModuleSource),
		formpackage.DigestBytes(sourceMapBytes):              sourceMapBytes,
	}, "key-auxiliary-carried")
	if err != nil {
		return fmt.Errorf("a WorkerBundle carrying a source map beside a loadable main module: %w", err)
	}
	if !formpackage.ValidDigest(digest) {
		return errors.New("the committed manifest digest is not canonical")
	}
	r.complete("bundle-main-module-is-loadable")
	return nil
}

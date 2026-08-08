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

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// canonicalClaimHostname is the canonical spelling the two hostname checks
// claim, and nonCanonicalClaimHostname is the same DNS name written the two
// other legal ways at once: uppercase letters and the trailing root dot.
const (
	canonicalClaimHostname    = "claim.portable-conformance.invalid"
	nonCanonicalClaimHostname = "Claim.Portable-Conformance.INVALID."
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
	if err := r.refuseCreate(
		colliding, "invalid_argument", "key-claim-collision",
		"a second WorkerCustomDomain claiming a hostname another attachment already serves",
	); err != nil {
		return err
	}
	// The same claim spelled the way the first one was WRITTEN rather than the
	// way it was stored is the same collision, and is refused the same way. A
	// host comparing written bytes admits exactly this one.
	restated := r.customDomainClaiming("claim-collision-domain", nonCanonicalClaimHostname)
	restatedDigest, _, err := r.prepareExpectingRewrite(restated, "")
	if err != nil {
		return fmt.Errorf("preparing a colliding claim in another spelling: %w", err)
	}
	response, err := r.apply(restated, applyOptions{
		Create: true, IdempotencyKey: "key-claim-collision-restated", PrepareDigest: restatedDigest,
	})
	if err != nil {
		return err
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
	if err := r.refuseCreate(
		far, "invalid_argument", "key-claim-far-collision",
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
	if err := r.refuseCreate(
		colliding, "invalid_argument", "key-claim-collision-reverse",
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
func (r *v3Runner) checkDeadLetterCycleRejected() error {
	input := r.contract.RunnerInput
	queueOf := func(name, key string) (probeTarget, error) {
		queue := r.target(input.AtLeastOnceQueue)
		queue.Name = name
		if _, _, err := r.applyResource(queue, applyOptions{
			Create: true, IdempotencyKey: key,
		}, http.StatusCreated); err != nil {
			return probeTarget{}, fmt.Errorf("the %s queue: %w", name, err)
		}
		return queue, nil
	}
	first, err := queueOf("dead-letter-queue-a", "key-dead-letter-queue-a")
	if err != nil {
		return err
	}
	second, err := queueOf("dead-letter-queue-b", "key-dead-letter-queue-b")
	if err != nil {
		return err
	}

	consumerOf := func(name string, drains, deadLetter probeTarget) probeTarget {
		consumer := r.target(input.QueueConsumer)
		consumer.Name = name
		consumer.Spec = cloneJSONMap(consumer.Spec)
		consumer.Spec["worker"] = exactReference(r.target(input.ModuleWorker), "gate-worker")
		consumer.Spec["queue"] = exactReference(drains, drains.Name)
		consumer.Spec["deadLetterQueue"] = exactReference(deadLetter, deadLetter.Name)
		return consumer
	}

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

	for _, consumer := range []probeTarget{acyclic, onward, forward} {
		if err := r.deleteExisting(consumer, "key-"+consumer.Name+"-delete"); err != nil {
			return err
		}
	}
	for _, queue := range []probeTarget{fourth, third, second, first} {
		if err := r.deleteExisting(queue, "key-"+queue.Name+"-delete"); err != nil {
			return err
		}
	}
	r.complete("dead-letter-cycle-rejected")
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

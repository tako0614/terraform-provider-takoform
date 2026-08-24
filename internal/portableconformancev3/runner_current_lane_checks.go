package portableconformancev3

// runner_current_lane_checks.go measures the rules the current lane introduced
// over the retained v1beta1 one.
//
// Every check here exists because the v1beta1 corpus COULD NOT have measured
// its rule: the rule did not exist in that lane. They run only on a corpus
// whose required-check list names them, and such a corpus cannot pass without
// them, which is the whole point — decision 0046 makes conformance coverage a
// precondition of minting an identity, and a lane whose new rules are stated
// only in prose is exactly the unmeasured contract that rule exists to prevent.
//
// The file is named for the lane's ROLE rather than its number, because the
// two lanes these rules were first drafted in were withdrawn without ever
// being served (decision 0051) and a filename carrying a withdrawn number
// tells a reader to look for a document that does not exist.

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// checkFenceMatrixObserved drives the fence matrix as behavior rather than as
// a table someone can read: for each operation the lane fences, the absent
// case and the stale case are both requested and both answers are held to the
// matrix the corpus carries.
//
// v1beta1 stated its fences in prose scattered through the lifecycle section,
// and the runner checked a few of them where a check happened to need one. The
// matrix made the rule enumerable; this makes it measured.
func (r *v3Runner) checkFenceMatrixObserved(target probeTarget) error {
	probe := target
	probe.Name = "fence-matrix-probe"
	created, _, err := r.applyResource(probe, applyOptions{
		Create: true, IdempotencyKey: "key-fence-matrix-create",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("creating the fence-matrix probe: %w", err)
	}

	// An UPDATE requires the expected generation. Absent, it is refused before
	// any mutation; stale, it is a generation_conflict. The two are different
	// answers on purpose: one says the request was malformed, the other says
	// the world moved.
	//
	// The absent case is driven against the APPLY route with a prepare digest
	// minted under the correct fence, because prepare enforces the same fence
	// and would otherwise answer first. Both refusals are the matrix working;
	// isolating the apply route is what proves the fence is not merely a
	// prepare-time formality that the mutation itself would accept without.
	prepared, err := r.prepareWithFenceAs(r.token, probe, created.Metadata.Generation)
	if err != nil {
		return fmt.Errorf("preparing the fence-matrix update: %w", err)
	}
	if response, err := r.apply(probe, applyOptions{
		PrepareDigest:  prepared.PrepareDigest,
		IdempotencyKey: "key-fence-matrix-update-absent",
	}); err != nil {
		return err
	} else if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("an update with no generation fence: %w", err)
	}
	if response, err := r.apply(probe, applyOptions{
		PrepareDigest:      prepared.PrepareDigest,
		ExpectedGeneration: "999999",
		IdempotencyKey:     "key-fence-matrix-update-stale",
	}); err != nil {
		return err
	} else if response.Status != http.StatusPreconditionFailed {
		return fmt.Errorf(
			"an update with a stale generation fence answered HTTP %d, want 412 generation_conflict",
			response.Status,
		)
	}

	// A DELETE fences on the generation too, and on nothing else: the revision
	// is deliberately NOT its fence, because a teardown removes dependents
	// first and would otherwise be refused by a revision it moved itself.
	deleteURL := r.resourceURL(probe.Ref, probe.Name, "", r.exactQuery(probe.Space, probe.Ref))
	if response, err := r.request(http.MethodDelete, deleteURL, map[string]string{
		"Idempotency-Key": "key-fence-matrix-delete-absent",
	}, nil); err != nil {
		return err
	} else if response.Status != http.StatusBadRequest {
		return fmt.Errorf(
			"a delete with no generation fence answered HTTP %d, want 400; the matrix requires the fence",
			response.Status,
		)
	}
	if response, err := r.request(http.MethodDelete, deleteURL, map[string]string{
		"Idempotency-Key":              "key-fence-matrix-delete-stale",
		"Takoform-Expected-Generation": "999999",
	}, nil); err != nil {
		return err
	} else if response.Status != http.StatusPreconditionFailed {
		return fmt.Errorf(
			"a delete with a stale generation fence answered HTTP %d, want 412",
			response.Status,
		)
	}

	// The header and the body member are two spellings of one fence, and a
	// request carrying both must not be able to say two things.
	if response, err := r.request(http.MethodDelete, deleteURL, map[string]string{
		"Idempotency-Key":              "key-fence-matrix-delete-disagree",
		"Takoform-Expected-Generation": created.Metadata.Generation,
		"If-Match":                     `"999999"`,
	}, nil); err != nil {
		return err
	} else if response.Status != http.StatusPreconditionFailed {
		return fmt.Errorf(
			"a delete whose revision fence is stale answered HTTP %d, want 412",
			response.Status,
		)
	}

	// The matrix publishes TWO transports for an update fence and calls them
	// equal. A runner that only ever sent the header would leave a host
	// serving only the header passing, and a client reading the document would
	// find the body transport does not work.
	viaBody, err := r.prepareWithFenceAs(r.token, probe, created.Metadata.Generation)
	if err != nil {
		return fmt.Errorf("preparing the body-transport fence: %w", err)
	}
	if response, err := r.apply(probe, applyOptions{
		PrepareDigest:  viaBody.PrepareDigest,
		BodyGeneration: created.Metadata.Generation,
		IdempotencyKey: "key-fence-matrix-body-transport",
	}); err != nil {
		return err
	} else if response.Status != http.StatusOK {
		return fmt.Errorf(
			"an update fencing through the body's expectedGeneration answered HTTP %d, want 200; "+
				"the matrix offers the body and the header as equal transports",
			response.Status,
		)
	}
	afterBody, _, err := r.read(probe)
	if err != nil {
		return fmt.Errorf("reading after the body-transport fence: %w", err)
	}

	// Both transports at once, disagreeing. The lane refuses this BEFORE
	// either is evaluated, so a host that picked one and ignored the other
	// would silently apply against a belief the caller did not hold.
	disagreeing, err := r.prepareWithFenceAs(r.token, probe, afterBody.Metadata.Generation)
	if err != nil {
		return fmt.Errorf("preparing the disagreeing fence: %w", err)
	}
	if response, err := r.apply(probe, applyOptions{
		PrepareDigest:             disagreeing.PrepareDigest,
		ExpectedGeneration:        afterBody.Metadata.Generation,
		DisagreeingBodyGeneration: afterBody.Metadata.Generation + "0",
		IdempotencyKey:            "key-fence-matrix-disagreement",
	}); err != nil {
		return err
	} else if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("an apply whose header and body fences disagree: %w", err)
	}

	// A create carrying If-None-Match together with a generation fence asserts
	// that the resource both does and does not exist.
	contradictory := probe
	contradictory.Name = "fence-matrix-contradictory-create"
	if response, err := r.apply(contradictory, applyOptions{
		Create:                    true,
		CreateWithGenerationFence: "1",
		IdempotencyKey:            "key-fence-matrix-contradictory-create",
	}); err != nil {
		return err
	} else if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("a create carrying both If-None-Match and a generation fence: %w", err)
	}

	// The optional review.specDigest echo is the last fence in the matrix and
	// the only one a client opts into. Echoing what prepare answered must be
	// accepted; echoing anything else must be refused, because the whole
	// purpose of the member is to catch a spec substituted between prepare and
	// apply — a substitution the prepare digest alone cannot show the CLIENT.
	honest, err := r.prepareWithFenceAs(r.token, probe, created.Metadata.Generation)
	if err != nil {
		return fmt.Errorf("preparing the specDigest echo: %w", err)
	}
	if response, err := r.apply(probe, applyOptions{
		PrepareDigest:      honest.PrepareDigest,
		SpecDigestEcho:     honest.SpecDigest,
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-fence-matrix-echo-honest",
	}); err != nil {
		return err
	} else if response.Status != http.StatusOK {
		return fmt.Errorf(
			"an apply echoing the prepare answer's specDigest answered HTTP %d, want 200; "+
				"echoing the review object a prepare handed back is never itself a refusal",
			response.Status,
		)
	}
	current, _, err := r.read(probe)
	if err != nil {
		return fmt.Errorf("reading the fence-matrix probe after the echo: %w", err)
	}
	lying, err := r.prepareWithFenceAs(r.token, probe, current.Metadata.Generation)
	if err != nil {
		return fmt.Errorf("preparing the mismatched specDigest echo: %w", err)
	}
	if response, err := r.apply(probe, applyOptions{
		PrepareDigest:      lying.PrepareDigest,
		SpecDigestEcho:     formpackage.DigestBytes([]byte("a spec that was never prepared")),
		ExpectedGeneration: current.Metadata.Generation,
		IdempotencyKey:     "key-fence-matrix-echo-mismatched",
	}); err != nil {
		return err
	} else if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("an apply echoing a specDigest that is not the applied spec's: %w", err)
	}

	if err := r.deleteExisting(probe, "key-fence-matrix-delete"); err != nil {
		return fmt.Errorf("removing the fence-matrix probe: %w", err)
	}
	r.complete("fence-matrix-observed")
	return nil
}

// checkFormsRouteEnumerates proves the /forms route answers the installed SET.
//
// In v1beta1 the response array was capped at one and every query key was
// required, so a route named "forms" could not tell a client which Forms a
// host had. Enumeration is the repair, and the fully-keyed probe is now the
// narrowest case of one rule rather than the only case of another.
func (r *v3Runner) checkFormsRouteEnumerates(target probeTarget) error {
	// The group keys the lane states, whether that is the whole group or its
	// two halves. Below this point the check reads the keys back from the
	// values it set, so it measures the same rule on either vocabulary.
	narrowed := url.Values{}
	narrowed.Set("space", target.Space)
	if r.contract.lane.FormsFilterGroupIsWhole {
		narrowed.Set("group", target.Ref.APIVersion)
	} else {
		group, version, ok := splitAPIVersion(target.Ref.APIVersion)
		if !ok {
			return fmt.Errorf("probe FormRef apiVersion %q is not a namespaced group", target.Ref.APIVersion)
		}
		narrowed.Set("group", group)
		narrowed.Set("version", version)
	}

	// Narrowing by the group alone must answer MORE than one Form: that is the
	// difference between enumerating and probing, and a host that kept the
	// v1beta1 cap would fail here rather than silently under-report.
	family, err := r.formsQuery(narrowed)
	if err != nil {
		return err
	}
	if len(family) < 2 {
		return fmt.Errorf(
			"forms narrowed to %s answered %d entries; the route enumerates the installed set, "+
				"and this corpus installs more than one Form of that family",
			target.Ref.APIVersion, len(family),
		)
	}
	for _, entry := range family {
		if entry.Identity.FormRef.APIVersion != target.Ref.APIVersion {
			return fmt.Errorf(
				"forms narrowed to %s answered a %s entry; a narrowing key narrows",
				target.Ref.APIVersion, entry.Identity.FormRef.APIVersion,
			)
		}
	}

	// Narrowing by kind answers every definition version installed under it —
	// this corpus installs two of ModuleWorker on purpose — and nothing of
	// another kind. Two definition versions of one line are two installed
	// Forms, so an enumeration that collapsed them would be hiding one.
	narrowed.Set("kind", target.Ref.Kind)
	byKind, err := r.formsQuery(narrowed)
	if err != nil {
		return err
	}
	if len(byKind) == 0 {
		return fmt.Errorf("forms narrowed to kind %s answered nothing", target.Ref.Kind)
	}
	for _, entry := range byKind {
		if entry.Identity.FormRef.Kind != target.Ref.Kind {
			return fmt.Errorf(
				"forms narrowed to kind %s answered a %s entry",
				target.Ref.Kind, entry.Identity.FormRef.Kind,
			)
		}
	}
	// Adding the definition version reaches exactly one line, which is what
	// makes the narrowing keys compose rather than merely filter the first one.
	narrowed.Set("definitionVersion", target.Ref.DefinitionVersion)
	byVersion, err := r.formsQuery(narrowed)
	if err != nil {
		return err
	}
	if len(byVersion) != 1 || byVersion[0].Identity.FormRef != target.Ref {
		return fmt.Errorf(
			"forms narrowed to %s@%s answered %d entries; want exactly the installed line",
			target.Ref.Kind, target.Ref.DefinitionVersion, len(byVersion),
		)
	}

	// All six keys present is the exact-availability probe: zero entries or
	// one, and zero for an identity the host does not hold.
	exact := r.formsExactQuery(target.Space, target.Ref)
	found, err := r.formsQuery(exact)
	if err != nil {
		return err
	}
	if len(found) != 1 {
		return fmt.Errorf("the fully-keyed forms probe answered %d entries, want 1", len(found))
	}
	unknown := r.formsExactQuery(target.Space, FormRef{
		APIVersion:        target.Ref.APIVersion,
		Kind:              target.Ref.Kind,
		DefinitionVersion: target.Ref.DefinitionVersion,
		SchemaDigest:      "sha256:" + strings.Repeat("0", 64),
	})
	absent, err := r.formsQuery(unknown)
	if err != nil {
		return err
	}
	if len(absent) != 0 {
		return fmt.Errorf(
			"the fully-keyed forms probe answered %d entries for an identity this host does not hold, want 0",
			len(absent),
		)
	}

	// A key outside the closed vocabulary is refused rather than ignored.
	bogus := url.Values{}
	bogus.Set("space", target.Space)
	bogus.Set("packageDigest", target.PackageDigest)
	if _, err := r.formsQuery(bogus); err == nil {
		return errors.New(
			"forms accepted a packageDigest query key; audit evidence never enters identity, " +
				"so the query vocabulary is closed against it",
		)
	}

	// Every filter states a grammar, so every filter refuses a value outside
	// it. A host that validated only `space` answers 200 with an empty set to
	// a malformed query, which a client cannot tell from "this host holds no
	// such Form" — the one answer it must not be confused with. An explicitly
	// empty value is malformed too: absent means unfiltered, and a present
	// empty string is neither absent nor a value.
	for _, malformed := range []struct{ key, value string }{
		{"group", "Not A Group"},
		{"version", "beta1"},
		{"kind", "not-a-kind"},
		{"definitionVersion", "0.1"},
		{"schemaDigest", "bad"},
		{"space", ""},
		{"kind", ""},
	} {
		query := url.Values{}
		if malformed.key != "space" {
			query.Set("space", target.Space)
		}
		query.Set(malformed.key, malformed.value)
		if _, err := r.formsQuery(query); err == nil {
			return fmt.Errorf(
				"forms accepted %s=%q; a malformed filter value is invalid_argument, never an empty result",
				malformed.key, malformed.value,
			)
		}
	}
	r.complete("forms-route-enumerates")
	return nil
}

// checkAvailabilityTruthConditions holds the availability answer to the
// implications the lane states, and to the error a lifecycle route must answer
// when one of them is false.
//
// v1beta1 published the same booleans with no stated truth conditions, so two
// hosts could answer differently about one Form and both be conforming.
func (r *v3Runner) checkAvailabilityTruthConditions(target probeTarget) error {
	entries, err := r.formsQuery(r.formsExactQuery(target.Space, target.Ref))
	if err != nil {
		return err
	}
	if len(entries) != 1 {
		return fmt.Errorf("availability probe answered %d entries, want 1", len(entries))
	}
	entry := entries[0]

	// The implications are one-directional and the lane states them: installed
	// implies definitionKnown, executable implies installed. A host answering
	// the consequent without the antecedent is describing an impossible state.
	if entry.Installed && !entry.DefinitionKnown {
		return errors.New("availability says installed without definitionKnown; installed implies it")
	}
	if entry.Executable && !entry.Installed {
		return errors.New("availability says executable without installed; executable implies it")
	}
	// `deprecated` left this lane with the surface that had no source of truth
	// behind it. A host still answering it is answering a member the schema
	// closes against.
	if entry.Deprecated != nil {
		return errors.New(
			"availability carries a deprecated member; this lane removed it rather than invent a " +
				"source of truth for it, and the response object is closed",
		)
	}
	// The capability vocabulary is closed, and `refresh` is not in it: the lane
	// serves no refresh route, so a Form declaring the capability would promise
	// an operation nobody can call.
	for _, capability := range entry.Operations {
		if capability == "refresh" {
			return errors.New(
				"availability advertises the refresh capability; this lane serves no refresh route",
			)
		}
	}

	// The enumerating route answering nothing and a lifecycle route answering
	// form_unknown are the same fact told to two audiences, and both must hold:
	// an enumeration that hid an identity while a lifecycle route still served
	// it would be the more dangerous half of the pair.
	unknownRef := target.Ref
	unknownRef.SchemaDigest = "sha256:" + strings.Repeat("0", 64)
	lifecycle, err := r.request(
		http.MethodGet,
		r.resourceURL(unknownRef, target.Name, "", r.exactQuery(target.Space, unknownRef)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(lifecycle, "form_unknown"); err != nil {
		return fmt.Errorf("a lifecycle route addressed at an unheld identity: %w", err)
	}
	r.complete("availability-truth-conditions")
	return nil
}

// checkCancelOutcomesClosed drives cancel as the three-outcome state machine
// the lane states, rather than as the boolean a reader might assume.
//
// The outcome that matters most is the third: an operation past its safe
// stopping point is REFUSED and keeps running. A host that treated cancel as
// advisory would answer 200 and leave the operation running, and a client
// would believe it had stopped something it had not.
func (r *v3Runner) checkCancelOutcomesClosed(cancelTarget probeTarget) error {
	id := r.cancelledOperationID
	if id == "" {
		return errors.New(
			"no cancelled operation id was recorded; the async lane must run before cancel outcomes",
		)
	}

	// Outcome two: an already-terminal operation replays its terminal record
	// unchanged. Cancelling it again is 200, and nothing about it moves.
	before, err := r.operationRecord(id)
	if err != nil {
		return err
	}
	if !before.Done {
		return fmt.Errorf("operation %s is not terminal; the cancelled probe should have settled", id)
	}
	beforeCode, _ := before.Error["code"].(string)
	if beforeCode != "operation_cancelled" {
		return fmt.Errorf(
			"a cancelled operation's terminal record carries %v, want error.code operation_cancelled; "+
				"cancellation is a terminal failure of the operation, never a fourth state",
			before.Error,
		)
	}
	if before.Result != nil {
		return errors.New(
			"a terminal operation carries both result and error; the envelope admits exactly one",
		)
	}
	response, err := r.request(
		http.MethodPost, r.apiBase+"/operations/"+url.PathEscape(id)+"/cancel",
		map[string]string{"Idempotency-Key": "key-cancel-outcomes-terminal"}, nil,
	)
	if err != nil {
		return err
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf(
			"cancelling an already-terminal operation answered HTTP %d, want 200 replaying its record",
			response.Status,
		)
	}
	after, err := r.operationRecord(id)
	if err != nil {
		return err
	}
	afterCode, _ := after.Error["code"].(string)
	if afterCode != beforeCode || after.Done != before.Done {
		return errors.New("re-cancelling a terminal operation changed its record; it replays unchanged")
	}

	// An id no tenant holds is operation_not_found. This is not one of the
	// three outcomes — it is the answer for an operation that does not exist —
	// and it is checked here because a host that answered it for a live
	// operation would look like a refusal.
	missing, err := r.request(
		http.MethodPost, r.apiBase+"/operations/op_no_such_operation_here/cancel",
		map[string]string{"Idempotency-Key": "key-cancel-outcomes-missing"}, nil,
	)
	if err != nil {
		return err
	}
	if missing.Status != http.StatusNotFound {
		return fmt.Errorf(
			"cancelling an id no tenant holds answered HTTP %d, want 404 operation_not_found",
			missing.Status,
		)
	}

	// Outcome three, the one that needs a LIVE operation: a cancel that
	// arrives past the operation's safe stopping point is REFUSED over HTTP
	// with operation_cancelled, and the operation goes on to its own terminal
	// state. A host that cancels everything it is asked to cancel passes every
	// other assertion here and fails this one, which is the point: a caller
	// must be able to tell a cancellation that took from one that arrived too
	// late.
	if err := r.checkCancelPastSafeStop(cancelTarget); err != nil {
		return err
	}

	r.complete("cancel-outcomes-closed")
	return nil
}

// checkCancelPastSafeStop drives one accepted mutation to the point where its
// work is underway and proves the refusal, then proves the operation still
// reaches its OWN terminal state rather than a cancelled one.
func (r *v3Runner) checkCancelPastSafeStop(target probeTarget) error {
	probe := target
	probe.Name = "cancel-past-safe-stop-probe"
	response, err := r.apply(probe, applyOptions{
		Create:         true,
		IdempotencyKey: "key-cancel-past-safe-stop",
		ExtraHeaders:   map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return fmt.Errorf("accepting the mutation to cancel too late: %w", err)
	}
	if response.Status != http.StatusAccepted {
		return fmt.Errorf("the mutation to cancel too late answered HTTP %d, want 202", response.Status)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(response, &envelope); err != nil {
		return err
	}
	accepted := envelope.Operation.ID

	// Advance the operation until one more poll would settle it. A host whose
	// stopping point is elsewhere is not wrong; what it may not do is accept a
	// cancel it cannot honour, so the loop stops at the first refusal and the
	// check fails only if the refusal never comes.
	refused := false
	for poll := 0; poll < asyncOperationPolls+2 && !refused; poll++ {
		record, err := r.operationRecord(accepted)
		if err != nil {
			return err
		}
		if record.Done {
			break
		}
		response, err := r.request(
			http.MethodPost, r.apiBase+"/operations/"+url.PathEscape(accepted)+"/cancel",
			map[string]string{"Idempotency-Key": fmt.Sprintf("key-cancel-past-safe-stop-%d", poll)}, nil,
		)
		if err != nil {
			return err
		}
		switch response.Status {
		case http.StatusOK:
			continue
		case http.StatusConflict:
			if err := r.expectStableError(response, "operation_cancelled"); err != nil {
				return fmt.Errorf("a cancel refused past the safe stopping point: %w", err)
			}
			refused = true
		default:
			return fmt.Errorf(
				"cancelling a live operation answered HTTP %d; the closed answers are 200 (it took) "+
					"and 409 operation_cancelled (it arrived too late)",
				response.Status,
			)
		}
	}
	if !refused {
		return errors.New(
			"no cancel was ever refused: this host cancelled an operation at every point in its life, " +
				"so it states no safe stopping point and a caller cannot know when a cancel still stops the work",
		)
	}

	// It ran to its own terminal state, not to a cancellation.
	var terminal wireOperation
	for poll := 0; poll < asyncOperationPolls+3; poll++ {
		terminal, err = r.operationRecord(accepted)
		if err != nil {
			return err
		}
		if terminal.Done {
			break
		}
	}
	if !terminal.Done {
		return errors.New("an operation past its safe stopping point never settled")
	}
	if code, _ := terminal.Error["code"].(string); code == "operation_cancelled" {
		return errors.New(
			"an operation whose cancel was refused still terminated as cancelled; " +
				"the refusal said the work could not be stopped",
		)
	}
	return nil
}

// checkExternalServiceSlotsSealed measures the decision-0045 sealed slot: what
// a Form may declare, what a host must refuse, and — the part that makes the
// slot sealed rather than merely typed — that nothing about WHICH service
// answers ever enters portable desired state.
func (r *v3Runner) checkExternalServiceSlotsSealed(version probeTarget) error {
	slots := r.contract.RunnerInput.ExternalServices
	if len(slots.Protocols) == 0 {
		return errors.New("the corpus declares no external standard-service protocol vocabulary")
	}

	// The declared slot travels through a whole apply and comes back intact:
	// the projected NAME and the protocol are portable, and the endpoint and
	// credential the host resolved are not in the answer at all.
	probe := version
	probe.Name = "external-service-probe"
	probe.Spec = r.materialize(probe.Ref, slots.DesiredSpec)
	created, _, err := r.applyResource(probe, applyOptions{
		Create: true, IdempotencyKey: "key-external-service-create",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("creating a version declaring an external-service slot: %w", err)
	}
	declared, ok := created.Spec[slots.Property].([]any)
	if !ok || len(declared) == 0 {
		return fmt.Errorf("the applied spec carries no %s entries", slots.Property)
	}
	for _, raw := range declared {
		entry, _ := raw.(map[string]any)
		service, _ := entry["service"].(map[string]any)
		if service == nil {
			return errors.New("an external-service slot carries no service declaration")
		}
		if service["apiVersion"] != slots.ServiceAPIVersion {
			return fmt.Errorf(
				"an external-service slot names service apiVersion %v, want %s; the vocabulary's identity "+
					"is not an author input",
				service["apiVersion"], slots.ServiceAPIVersion,
			)
		}
		protocol, _ := service["protocol"].(string)
		if !containsString(slots.Protocols, protocol) {
			return fmt.Errorf(
				"an external-service slot names protocol %q outside the closed vocabulary %v",
				protocol, slots.Protocols,
			)
		}
		// A resolved endpoint or credential appearing anywhere in the answer
		// would mean the host had written into portable state the one thing
		// the slot exists to keep out of it.
		for _, forbidden := range []string{"url", "endpoint", "credential", "secret", "password"} {
			if _, present := entry[forbidden]; present {
				return fmt.Errorf(
					"an external-service slot carries %q; the host resolves the address and the credential "+
						"out of band and neither enters portable state",
					forbidden,
				)
			}
		}
	}
	if err := r.deleteExisting(probe, "key-external-service-delete"); err != nil {
		return fmt.Errorf("removing the external-service probe: %w", err)
	}

	// A protocol outside the closed vocabulary is refused BEFORE any mutation,
	// which is what makes the vocabulary closed rather than advisory. Prepare
	// is where "before any mutation" actually is, so that is where the refusal
	// is measured: a host that accepted the spec here and failed at apply would
	// be telling an author about the closed vocabulary one round trip too late.
	refused := version
	refused.Name = "external-service-unknown-protocol"
	refused.Spec = r.materialize(refused.Ref, slots.UnknownProtocolSpec)
	prepared, err := r.prepareRequestAs(r.token, refused, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(prepared, "invalid_argument"); err != nil {
		return fmt.Errorf("a slot naming a protocol outside the vocabulary: %w", err)
	}

	// The support surface answers whether the host can satisfy one protocol at
	// all, so a client learns it at prepare time rather than at apply time.
	for _, protocol := range slots.Protocols {
		response, err := r.request(
			http.MethodGet,
			r.apiBase+"/support/standard-services/"+url.PathEscape(protocol), nil, nil,
		)
		if err != nil {
			return err
		}
		if response.Status != http.StatusOK {
			return fmt.Errorf(
				"the standard-service support route answered HTTP %d for protocol %q, want 200",
				response.Status, protocol,
			)
		}
		var profile struct {
			APIVersion string `json:"apiVersion"`
			Kind       string `json:"kind"`
			ServiceRef struct {
				APIVersion string `json:"apiVersion"`
				Protocol   string `json:"protocol"`
			} `json:"serviceRef"`
			Satisfiable *bool `json:"satisfiable"`
		}
		if err := decodeStrictResponse(response, &profile); err != nil {
			return err
		}
		if profile.Kind != "StandardServiceSupport" {
			return fmt.Errorf("standard-service support answered kind %q", profile.Kind)
		}
		if profile.ServiceRef.Protocol != protocol {
			return fmt.Errorf(
				"standard-service support for %q answered about %q", protocol, profile.ServiceRef.Protocol)
		}
		if profile.Satisfiable == nil {
			return fmt.Errorf(
				"standard-service support for %q omits satisfiable; an absent answer is not a false one",
				protocol,
			)
		}
	}
	r.complete("external-service-slots-sealed")
	return nil
}

// checkStableStandardServiceSupportEnforced measures the stable split between
// an open opaque identifier grammar and this Host's exact support set. It uses
// a supported structured S3 identifier plus the same unknown-valid identifier
// in required and optional slots, so syntax, support, mutation ordering, and
// projection behavior are independently observable.
func (r *v3Runner) checkStableStandardServiceSupportEnforced(version probeTarget) error {
	slots := r.contract.RunnerInput.ExternalServices
	if slots.ServiceAPIVersion != formpackage.StandardServiceAPIVersion ||
		!containsString(slots.Protocols, "com.amazonaws.s3") ||
		len(slots.DesiredSpec) == 0 || len(slots.UnknownProtocolSpec) == 0 ||
		len(slots.OptionalUnsupportedSpec) == 0 {
		return errors.New("the stable corpus does not pin supported, required-unsupported, and optional-unsupported service slots")
	}

	checkSupport := func(protocol string, want bool) error {
		response, err := r.request(
			http.MethodGet,
			r.apiBase+"/support/standard-services/"+url.PathEscape(protocol), nil, nil,
		)
		if err != nil {
			return err
		}
		if response.Status != http.StatusOK {
			return fmt.Errorf("stable standard-service support for %q answered HTTP %d, want 200", protocol, response.Status)
		}
		var profile map[string]any
		if err := decodeStrictResponse(response, &profile); err != nil {
			return err
		}
		got, err := formpackage.ValidateStandardServiceSupport(formpackage.StandardServiceRef{
			APIVersion: formpackage.StandardServiceAPIVersion,
			Protocol:   protocol,
		}, profile)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("stable standard-service support for %q = %t, want %t", protocol, got, want)
		}
		return nil
	}
	if err := checkSupport("com.amazonaws.s3", true); err != nil {
		return err
	}
	unknownProtocol := "com.example.future-store"
	if err := checkSupport(unknownProtocol, false); err != nil {
		return err
	}

	supported := version
	supported.Name = "stable-standard-service-supported"
	supported.Spec = r.materialize(supported.Ref, slots.DesiredSpec)
	if _, _, err := r.applyResource(supported, applyOptions{
		Create: true, IdempotencyKey: "key-stable-standard-service-supported",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("creating a resource with supported S3 service slot: %w", err)
	}
	if err := r.deleteExisting(supported, "key-stable-standard-service-supported-delete"); err != nil {
		return err
	}

	required := version
	required.Name = "stable-standard-service-required-unknown"
	required.Spec = r.materialize(required.Ref, slots.UnknownProtocolSpec)
	prepared, err := r.prepareRequest(required, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(prepared, "unsupported_capability"); err != nil {
		return fmt.Errorf("required unknown-valid standard-service slot: %w", err)
	}
	absent, err := r.request(
		http.MethodGet,
		r.resourceURL(required.Ref, required.Name, "", r.exactQuery(required.Space, required.Ref)),
		nil, nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(absent, "resource_not_found"); err != nil {
		return fmt.Errorf("required unsupported slot mutated before refusal: %w", err)
	}

	optional := version
	optional.Name = "stable-standard-service-optional-unknown"
	optional.Spec = r.materialize(optional.Ref, slots.OptionalUnsupportedSpec)
	created, _, err := r.applyResource(optional, applyOptions{
		Create: true, IdempotencyKey: "key-stable-standard-service-optional-unknown",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("creating a resource with optional unsupported slot: %w", err)
	}
	if created.Status != nil {
		for _, forbidden := range []string{"FUTURE_STORE", "endpoint", "credential", "secret", "url"} {
			if _, projected := created.Status.Outputs[forbidden]; projected {
				return fmt.Errorf("optional unsupported standard-service slot projected %q", forbidden)
			}
		}
	}
	if err := r.deleteExisting(optional, "key-stable-standard-service-optional-unknown-delete"); err != nil {
		return err
	}

	r.complete("stable-standard-service-support-enforced")
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// formsAvailabilityEntry is one entry of the /forms answer. `deprecated` is a
// POINTER so its absence is distinguishable from a false value: this lane
// removed the member, and a host still answering `false` is answering a member
// the response object closes against.
type formsAvailabilityEntry struct {
	Identity struct {
		FormRef       FormRef `json:"formRef"`
		PackageDigest string  `json:"packageDigest,omitempty"`
	} `json:"identity"`
	DefinitionKnown      bool     `json:"definitionKnown"`
	Installed            bool     `json:"installed"`
	Executable           bool     `json:"executable"`
	Activated            bool     `json:"activated"`
	AvailableToPrincipal bool     `json:"availableToPrincipal"`
	Operations           []string `json:"operations"`
	Deprecated           *bool    `json:"deprecated,omitempty"`
}

// formsQuery asks the enumerating route one question and decodes the answer.
func (r *v3Runner) formsQuery(query url.Values) ([]formsAvailabilityEntry, error) {
	response, err := r.request(http.MethodGet, r.apiBase+"/forms?"+query.Encode(), nil, nil)
	if err != nil {
		return nil, err
	}
	if response.Status != http.StatusOK {
		return nil, fmt.Errorf(
			"forms HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var answer struct {
		Forms []formsAvailabilityEntry `json:"forms"`
	}
	if err := decodeStrictResponse(response, &answer); err != nil {
		return nil, err
	}
	return answer.Forms, nil
}

// formsExactQuery is the six-key exact-availability probe: the same question
// v1beta1 asked with five keys, now expressed in the vocabulary that splits an
// apiVersion the way every path in this lane already splits it.
// checkPortableDefaultsMaterialized drives the one request the runner never
// otherwise sends: a spec with a defaulted property OMITTED.
//
// Every other probe is materialized locally before it leaves the runner
// (target), so no request in the corpus has ever exercised the rule. A host
// that stores the wire spec verbatim — never materializing anything — passed
// every check, which made an entire normative section unmeasured while
// reporting green.
//
// The rule's consequences are what a client actually depends on: the echo it
// gets back IS its desired state, the digest is the materialized spec's, and
// omitting a default is the same desired state as writing it — same
// generation, no update.
func (r *v3Runner) checkPortableDefaultsMaterialized(target probeTarget) error {
	defaults := r.declaredDefaults(target.Ref)
	if len(defaults) == 0 {
		return fmt.Errorf(
			"the corpus pins no declared default for %s, so the materialization rule cannot be driven here",
			target.Ref.Kind,
		)
	}

	// The same desired state, written two ways: once with every default
	// omitted, once with each written out at its declared value.
	omitted := probeTarget{
		Ref: target.Ref, PackageDigest: target.PackageDigest, Space: target.Space,
		Name: "portable-defaults-probe", Lifecycle: target.Lifecycle,
		Spec: map[string]any{},
	}
	for key, value := range target.Spec {
		if _, defaulted := defaults[key]; !defaulted {
			omitted.Spec[key] = value
		}
	}
	written := omitted
	written.Spec = map[string]any{}
	for key, value := range omitted.Spec {
		written.Spec[key] = value
	}
	for key, value := range defaults {
		written.Spec[key] = value
	}

	created, _, err := r.applyResource(omitted, applyOptions{
		Create: true, IdempotencyKey: "key-portable-defaults-create",
	}, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("creating a resource with its defaults omitted: %w", err)
	}
	// The whole materialized document, not the defaulted keys. Checking only
	// what the caller sent plus what it expected back would let a host add an
	// undeclared property, bind its digest, echo it consistently, and pass —
	// while the client's desired state silently contains something it never
	// wrote. The corpus's own materialization is the expected value, and it is
	// compared as canonical JSON so a number that decoded to another Go type
	// is not mistaken for a different value.
	expected := r.materialize(omitted.Ref, omitted.Spec)
	wantSpec, err := canonicalJSON(expected)
	if err != nil {
		return err
	}
	gotSpec, err := canonicalJSON(created.Spec)
	if err != nil {
		return err
	}
	if gotSpec != wantSpec {
		return fmt.Errorf(
			"the echo of a spec omitting its defaults is %s, want the materialized %s; the effective "+
				"spec IS the wire spec, so the echo is the client's own desired state and carries "+
				"nothing it did not write and nothing the Definition did not default",
			gotSpec, wantSpec,
		)
	}

	// Writing the default explicitly is the SAME desired state: no update.
	updated, _, err := r.applyResource(written, applyOptions{
		ExpectedGeneration: created.Metadata.Generation,
		IdempotencyKey:     "key-portable-defaults-rewrite",
	}, http.StatusOK)
	if err != nil {
		return fmt.Errorf("re-applying the same state with its defaults written out: %w", err)
	}
	if updated.Metadata.Generation != created.Metadata.Generation {
		return fmt.Errorf(
			"writing a default explicitly moved the generation from %s to %s; omitting a defaulted "+
				"property and writing its default are one desired state",
			created.Metadata.Generation, updated.Metadata.Generation,
		)
	}

	if err := r.deleteExisting(omitted, "key-portable-defaults-delete"); err != nil {
		return fmt.Errorf("removing the defaults probe: %w", err)
	}
	r.complete("portable-defaults-materialized")
	return nil
}

// declaredDefaults reads the top-level defaults the CORPUS pins for one Form.
func (r *v3Runner) declaredDefaults(ref FormRef) map[string]any {
	schema, pinned := r.desiredSchemas[ref]
	if !pinned {
		return nil
	}
	properties, _ := schema["properties"].(map[string]any)
	out := map[string]any{}
	for name, raw := range properties {
		node, _ := raw.(map[string]any)
		if node == nil {
			continue
		}
		if value, declared := node["default"]; declared {
			out[name] = value
		}
	}
	return out
}

// formsExactQuery builds the /forms filter query naming one whole identity.
// It is NOT exactQuery: the filter vocabulary splits `group` from `version`
// and takes six keys, while a resource route's exact-Form query carries the
// apiVersion whole under `group`. Sending one route's spelling to the other
// answers an empty set rather than an error.
func (r *v3Runner) formsExactQuery(space string, ref FormRef) url.Values {
	query := url.Values{}
	query.Set("space", space)
	if r.contract.lane.FormsFilterGroupIsWhole {
		query.Set("group", ref.APIVersion)
	} else {
		group, version, _ := splitAPIVersion(ref.APIVersion)
		query.Set("group", group)
		query.Set("version", version)
	}
	query.Set("kind", ref.Kind)
	query.Set("definitionVersion", ref.DefinitionVersion)
	query.Set("schemaDigest", ref.SchemaDigest)
	return query
}

// operationRecord reads one operation's whole record.
func (r *v3Runner) operationRecord(id string) (wireOperation, error) {
	response, err := r.request(http.MethodGet, r.apiBase+"/operations/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return wireOperation{}, err
	}
	if response.Status != http.StatusOK {
		return wireOperation{}, fmt.Errorf(
			"operation %s HTTP %d; body=%s", id, response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var operation wireOperation
	if err := decodeStrictResponse(response, &operation); err != nil {
		return wireOperation{}, err
	}
	return operation, nil
}

// formsAvailabilityQuery is the exact-availability probe in whichever
// vocabulary the corpus's lane uses: v1beta2's /forms splits group from
// version, v1beta1's took the apiVersion whole.
func (r *v3Runner) formsAvailabilityQuery(space string, ref FormRef) url.Values {
	if r.contract.lane.FormsResponseEnumerates {
		return r.formsExactQuery(space, ref)
	}
	return r.exactQuery(space, ref)
}

// expectNoInstalledForm asserts that an availability response names no
// installed Form, in whichever way the corpus's lane says so.
//
// The two lanes answer the same question differently ON PURPOSE: a route that
// answers about ONE identity reports an unknown one as form_unknown, while a
// route that ENUMERATES reports it by enumerating nothing. Neither is a
// weaker answer, and folding them into one expectation here keeps every caller
// from having to know which lane it is driving.
func (r *v3Runner) expectNoInstalledForm(response wireResponse) error {
	if !r.contract.lane.FormsResponseEnumerates {
		return r.expectStableError(response, "form_unknown")
	}
	if response.Status != http.StatusOK {
		return fmt.Errorf(
			"HTTP %d; an enumerating forms route answers 200 with no entries for an identity it does not hold",
			response.Status,
		)
	}
	var answer struct {
		Forms []formsAvailabilityEntry `json:"forms"`
	}
	if err := decodeStrictResponse(response, &answer); err != nil {
		return err
	}
	if len(answer.Forms) != 0 {
		return fmt.Errorf("forms answered %d entries, want none", len(answer.Forms))
	}
	return nil
}

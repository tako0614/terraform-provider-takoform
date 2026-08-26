package portableconformancev3

// runner_state_continuity_checks.go proves the HOST side of what a long-lived
// client relies on to keep state across Form evolution and interruption
// (spec/decisions/0017):
//
//   - an operation a client recorded stays addressable and replays the SAME
//     terminal state after the system has moved on, so a client that persisted
//     an operation id can still resume from it;
//   - an exact FormRef query naming a definition version the host does not have
//     fails closed instead of matching by group and kind, so a client that
//     dispatches on the identity recorded in state can never be answered about a
//     different contract.
//
// Both are properties a host can pass the rest of the lane without having: the
// existing operation checks replay a terminal operation IMMEDIATELY, and every
// other exact-FormRef check sends the identity the host does have.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
)

// unknownDefinitionVersion is a definition version no candidate Form declares.
// It is deliberately far above every real line rather than adjacent to one, so
// the check cannot pass by accident on a host that happens to install a
// neighbouring version.
const unknownDefinitionVersion = "99.99.99"

// unknownSchemaDigest is a well-formed digest that addresses no installed
// definition bytes.
const unknownSchemaDigest = "sha256:" +
	"9999999999999999999999999999999999999999999999999999999999999999"

// checkOperationResumableAfterSettlement proves an operation id remains a
// usable handle after the mutation it names has settled and the host has moved
// on.
//
// A client that persists `pending_operation_id` resumes by asking the host what
// happened, and it may ask much later: after the resource was updated, after
// other operations ran, after its own restart. A host that keeps a terminal
// operation addressable only until the next thing happens makes that resumption
// a coin flip — the client would sometimes learn the outcome and sometimes be
// told the operation never existed, with no way to tell the two apart from a
// difference in timing.
//
// The check therefore drives the system FORWARD between the first read and the
// replay: the created resource is updated, and a second operation is accepted
// and driven to its own terminal state. Only then is the first operation read
// again, and it must answer byte-identically. The replayed result must still
// name the exact identity and the uid the resource actually carries, because
// that uid is what a resuming client verifies before it adopts anything.
func (r *v3Runner) checkOperationResumableAfterSettlement(queue probeTarget) error {
	first := queue
	first.Name = "resume-probe"
	firstID, err := r.acceptAsyncCreate(first, "key-resume-create")
	if err != nil {
		return err
	}
	terminal, err := r.pollOperation(firstID)
	if err != nil {
		return err
	}
	if terminal.Error != nil || terminal.Result == nil {
		return fmt.Errorf("terminal operation %s must carry exactly the result: %+v", firstID, terminal)
	}
	captured, err := r.operationResponse(firstID)
	if err != nil {
		return err
	}
	createdUID, err := r.contract.lane.operationResultUID(captured.Body, first)
	if err != nil {
		return fmt.Errorf("captured terminal operation %s: %w", firstID, err)
	}

	// The system moves on. The resource the operation created is updated, so its
	// representation and revision change under the operation the client still
	// holds.
	current, _, err := r.read(first)
	if err != nil {
		return err
	}
	mutated := first
	mutated.Spec = r.desiredMutation(queue, 0, cloneJSONMap(first.Spec))
	if reflect.DeepEqual(mutated.Spec, first.Spec) {
		mutated.Spec["deliveryDelaySeconds"] = json.Number("30")
	}
	updated, _, err := r.applyResource(mutated, applyOptions{
		ExpectedGeneration: current.Metadata.Generation,
		IdempotencyKey:     "key-resume-update",
	}, http.StatusOK)
	if err != nil {
		return err
	}
	if updated.Metadata.UID != createdUID {
		return fmt.Errorf(
			"updating the resource changed its uid from %s to %s", createdUID, updated.Metadata.UID,
		)
	}

	// A SECOND operation is accepted and settled. A host that remembers only the
	// most recent operation record fails here and nowhere else.
	second := queue
	second.Name = "resume-second-probe"
	secondID, err := r.acceptAsyncCreate(second, "key-resume-second-create")
	if err != nil {
		return err
	}
	if secondID == firstID {
		return errors.New("two accepted operations shared one id")
	}
	if _, err := r.pollOperation(secondID); err != nil {
		return err
	}

	replay, err := r.operationResponse(firstID)
	if err != nil {
		return fmt.Errorf("the first operation is no longer addressable after the system moved on: %w", err)
	}
	if !bytes.Equal(replay.Body, captured.Body) {
		return fmt.Errorf(
			"terminal operation %s replayed differently after settlement:\nbefore=%s\nafter=%s",
			firstID, strings.TrimSpace(string(captured.Body)), strings.TrimSpace(string(replay.Body)),
		)
	}
	replayedUID, err := r.contract.lane.operationResultUID(replay.Body, first)
	if err != nil {
		return fmt.Errorf("replayed terminal operation %s: %w", firstID, err)
	}
	if replayedUID != createdUID {
		return fmt.Errorf(
			"terminal operation %s replayed uid %s, want the uid it minted, %s",
			firstID, replayedUID, createdUID,
		)
	}
	r.complete("operation-resumable-after-settlement")
	return nil
}

// acceptAsyncCreate drives one create the host answers as a 202 Operation and
// returns the operation id.
func (r *v3Runner) acceptAsyncCreate(target probeTarget, key string) (string, error) {
	accepted, err := r.apply(target, applyOptions{
		Create:         true,
		IdempotencyKey: key,
		ExtraHeaders:   map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return "", err
	}
	if accepted.Status != http.StatusAccepted {
		return "", fmt.Errorf(
			"async apply HTTP %d, want 202; body=%s",
			accepted.Status, strings.TrimSpace(string(accepted.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(accepted, &envelope); err != nil {
		return "", err
	}
	if !strings.HasPrefix(envelope.Operation.ID, "op_") || envelope.Operation.Done {
		return "", fmt.Errorf("202 operation envelope is invalid: %+v", envelope.Operation)
	}
	return envelope.Operation.ID, nil
}

// operationResponse reads one operation, requiring a terminal 200.
func (r *v3Runner) operationResponse(id string) (wireResponse, error) {
	response, err := r.request(http.MethodGet, r.apiBase+"/operations/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return wireResponse{}, err
	}
	if response.Status != http.StatusOK {
		return wireResponse{}, fmt.Errorf(
			"operation %s HTTP %d, want 200; body=%s",
			id, response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	return response, nil
}

// operationResultUID reads the uid out of a terminal operation's result
// resource, holding it to the exact identity the operation was asked about.
func (l lane) operationResultUID(body []byte, target probeTarget) (string, error) {
	var operation wireOperation
	if err := decodeStrictResponse(wireResponse{Body: body}, &operation); err != nil {
		return "", err
	}
	if !operation.Done || operation.Result == nil {
		return "", errors.New("operation is not a terminal success")
	}
	raw, err := encodeRunnerJSON(operation.Result["resource"])
	if err != nil {
		return "", err
	}
	var resource wireResource
	if err := decodeStrictResponse(wireResponse{Body: raw}, &resource); err != nil {
		return "", err
	}
	if err := l.verifyResourceIdentity(resource, target); err != nil {
		return "", err
	}
	return resource.Metadata.UID, nil
}

// checkExactFormRefFailsClosedOnUnknownDefinition proves an exact FormRef query
// is answered about the WHOLE identity or not at all.
//
// A client dispatches reads, updates, and deletes on the exact FormRef recorded
// in its state, precisely so that a resource created under an earlier
// definition version stays addressable as itself. That only works if a host
// treats the identity as a whole. A host that matches by group and kind, and
// ignores the definition version or schema digest it cannot serve, answers
// every such query about a DIFFERENT contract — and it answers successfully, so
// nothing downstream can detect it. The client would then read one Form's
// resource as another's, and would delete under an identity it never wrote.
//
// The check asks three surfaces about an installed group and kind at a
// definition version and schema digest the host does not have, and requires
// `form_unknown` (404) from each: availability, the Form Definition, and a read
// of a resource that DOES exist under the real identity. It then re-asks the
// real identity, so the refusal is proven to be about the identity rather than
// about the surface being broken.
func (r *v3Runner) checkExactFormRefFailsClosedOnUnknownDefinition(kv probeTarget) error {
	if _, _, err := r.read(kv); err != nil {
		return fmt.Errorf("the probe this check falsifies against is not readable: %w", err)
	}
	unknownVersion := kv.Ref
	unknownVersion.DefinitionVersion = unknownDefinitionVersion
	unknownDigest := kv.Ref
	unknownDigest.SchemaDigest = unknownSchemaDigest
	if unknownVersion == kv.Ref || unknownDigest == kv.Ref {
		return errors.New("the unknown-identity probe is not distinguishable from the installed one")
	}

	for _, unknown := range []struct {
		label string
		ref   FormRef
	}{
		{"definitionVersion", unknownVersion},
		{"schemaDigest", unknownDigest},
	} {
		query := r.exactQuery(kv.Space, unknown.ref).Encode()

		availability, err := r.request(
			http.MethodGet,
			r.apiBase+"/forms?"+r.formsAvailabilityQuery(kv.Space, unknown.ref).Encode(), nil, nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectNoInstalledForm(availability); err != nil {
			return fmt.Errorf(
				"availability matched an unknown %s by group and kind: %w", unknown.label, err,
			)
		}

		definitionURL := fmt.Sprintf(
			"%s/form-definitions/%s/%s?%s",
			r.apiBase, groupSegments(unknown.ref.APIVersion), url.PathEscape(unknown.ref.Kind), query,
		)
		definition, err := r.request(http.MethodGet, definitionURL, nil, nil)
		if err != nil {
			return err
		}
		if err := r.expectStableError(definition, "form_unknown"); err != nil {
			return fmt.Errorf(
				"the Form Definition surface served an unknown %s by group and kind: %w", unknown.label, err,
			)
		}

		read, err := r.request(
			http.MethodGet,
			r.resourceURL(unknown.ref, kv.Name, "", r.exactQuery(kv.Space, unknown.ref)),
			nil, nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(read, "form_unknown"); err != nil {
			return fmt.Errorf(
				"a read under an unknown %s was answered about the installed Form: %w", unknown.label, err,
			)
		}
	}

	// The same three surfaces still answer the identity the host DOES have, so
	// the refusals above are about the exact identity and not about a surface
	// that refuses everything.
	installed, err := r.request(http.MethodGet, r.apiBase+"/forms?"+r.formsAvailabilityQuery(kv.Space, kv.Ref).Encode(), nil, nil)
	if err != nil {
		return err
	}
	if installed.Status != http.StatusOK {
		return fmt.Errorf("the installed exact FormRef is no longer available: HTTP %d", installed.Status)
	}
	if _, err := r.formDefinition(kv.Ref); err != nil {
		return fmt.Errorf("the installed exact Form Definition is no longer readable: %w", err)
	}
	if _, _, err := r.read(kv); err != nil {
		return fmt.Errorf("the installed exact identity no longer reads its own resource: %w", err)
	}
	r.complete("exact-form-ref-fails-closed-on-unknown-definition")
	return nil
}

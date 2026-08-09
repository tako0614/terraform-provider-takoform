package portableconformancev3

// runner_replay_lifetime_checks.go measures how long a recorded idempotency
// answer stays true (spec/decisions/0011, "A replay record does not outlive the
// incarnation it reports").
//
// Every other idempotency check in this lane measures that a record REPLAYS.
// That is half the rule, and it is the half that made the other half invisible:
// a create's prepare binds the create markers — no uid, generation 0 — so a
// byte-identical re-create derives a byte-identical Idempotency-Key and a
// byte-identical fingerprint, and against a host that keeps its records
// forever, `terraform destroy` followed by `terraform apply` of an unchanged
// configuration is answered the old 201 and creates nothing. The next refresh
// 404s, the plan proposes the create again, and it replays again. There is no
// client-side fix: the discriminator would have to be the deletion the host
// performed in between, which is not in the request and not something the
// client knows.
//
// So the rule closes at the host, and it has two edges, both driven here. A
// record whose incarnation is gone must stop answering. A record that reports no
// live incarnation — a delete's 204 above all — must keep answering, or a
// lost-response retry of a delete would execute a second time against whatever
// holds the name now.

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// checkReplayRecordRetiresWithItsIncarnation drives the whole lifetime rule
// against one probe kind, in the order an operator meets it.
func (r *v3Runner) checkReplayRecordRetiresWithItsIncarnation(kv probeTarget) error {
	subject := kv
	subject.Name = "replay-retirement-probe"
	prepared, err := r.prepare(subject)
	if err != nil {
		return err
	}
	// ONE create request, kept verbatim: the same URL, the same headers, the same
	// bytes, and therefore the same key and the same fingerprint every time it is
	// sent. That determinism is what makes a lost response recoverable, and it is
	// what makes this check's third leg the defect it is looking for.
	createURL, createHeaders, createBody, err := r.applyRequestParts(subject, applyOptions{
		Create: true, IdempotencyKey: "key-replay-retirement-create", PrepareDigest: prepared.PrepareDigest,
	})
	if err != nil {
		return err
	}
	first, err := r.request(http.MethodPut, createURL, createHeaders, createBody)
	if err != nil {
		return err
	}
	created, err := decodeResource(first, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the create whose record this check is about: %w", err)
	}

	// The record answers while its incarnation is live. This is the lost-response
	// retry `apply-idempotency-replay` exists for, restated here because the
	// retirement rule must not touch it: retiring a record whose resource is still
	// present would turn every retried create into `already_exists` and hand the
	// operator an import.
	retry, err := r.request(http.MethodPut, createURL, createHeaders, createBody)
	if err != nil {
		return err
	}
	if retry.Status != first.Status || !bytes.Equal(retry.Body, first.Body) ||
		retry.Header.Get("ETag") != first.Header.Get("ETag") {
		return errors.New(
			"a create retried while its resource is still present was not a byte-identical replay; a record is " +
				"retired by the removal of its incarnation, never by a retry",
		)
	}

	// The delete, kept verbatim for the same reason: its own record is what the
	// second edge of the rule is about.
	deleteURL := r.resourceURL(subject.Ref, subject.Name, "", r.exactQuery(subject.Space, subject.Ref))
	deleteHeaders := map[string]string{
		expectedGenerationHeader: created.Metadata.Generation,
		"Idempotency-Key":        "key-replay-retirement-delete",
	}
	removed, err := r.request(http.MethodDelete, deleteURL, deleteHeaders, nil)
	if err != nil {
		return err
	}
	if removed.Status != http.StatusNoContent {
		return fmt.Errorf(
			"deleting %s HTTP %d, want 204; body=%s",
			subject.Name, removed.Status, strings.TrimSpace(string(removed.Body)),
		)
	}
	if err := r.expectResourceAbsent(subject); err != nil {
		return fmt.Errorf("the resource whose record must now retire is still readable: %w", err)
	}

	// `terraform apply` of an unchanged configuration after `terraform destroy`.
	// A host holding the old record answers the old 201, reports success, and
	// creates nothing.
	again, err := r.request(http.MethodPut, createURL, createHeaders, createBody)
	if err != nil {
		return err
	}
	recreated, err := decodeResource(again, http.StatusCreated)
	if err != nil {
		return fmt.Errorf(
			"re-creating %s with the byte-identical request that created it before: %w; a record whose incarnation "+
				"is gone must not answer a new request",
			subject.Name, err,
		)
	}
	if recreated.Metadata.UID == created.Metadata.UID {
		return fmt.Errorf(
			"the re-create was answered the removed incarnation's uid %s; the recorded 201 outlived the resource it "+
				"reports, so this configuration never converges — the next read is a 404 and the next apply replays "+
				"the same answer",
			created.Metadata.UID,
		)
	}
	liveAfterRecreate, _, err := r.read(subject)
	if err != nil {
		return fmt.Errorf("the re-create reported success and left nothing readable: %w", err)
	}
	if liveAfterRecreate.Metadata.UID != recreated.Metadata.UID {
		return fmt.Errorf(
			"the re-create answered uid %s and the resource reads back as %s",
			recreated.Metadata.UID, liveAfterRecreate.Metadata.UID,
		)
	}

	// The other edge. A completed delete's record reports the incarnation GONE, so
	// nothing about a later resource retires it — and it must not be executed
	// again either. The bytes below are exactly the bytes that deleted the FIRST
	// incarnation, and the replacement sits at the same generation, so a host that
	// re-executed instead of replaying would destroy a resource this request was
	// never issued against. Retiring records by name rather than by incarnation
	// produces precisely that.
	deleteReplay, err := r.request(http.MethodDelete, deleteURL, deleteHeaders, nil)
	if err != nil {
		return err
	}
	if deleteReplay.Status != http.StatusNoContent {
		return fmt.Errorf(
			"a delete retried after a lost response answered HTTP %d, want its recorded 204; body=%s",
			deleteReplay.Status, strings.TrimSpace(string(deleteReplay.Body)),
		)
	}
	survivor, _, err := r.read(subject)
	if err != nil {
		return fmt.Errorf(
			"replaying the first incarnation's delete removed the resource that holds the name now: %w", err,
		)
	}
	if survivor.Metadata.UID != recreated.Metadata.UID {
		return fmt.Errorf(
			"replaying the first incarnation's delete moved %s from uid %s to %s",
			subject.Name, recreated.Metadata.UID, survivor.Metadata.UID,
		)
	}
	if err := r.deleteExisting(subject, "key-replay-retirement-teardown"); err != nil {
		return err
	}

	if err := r.checkAcceptedReplayRetiresWithItsIncarnation(kv); err != nil {
		return err
	}
	r.complete("replay-record-retires-with-its-incarnation")
	return nil
}

// checkAcceptedReplayRetiresWithItsIncarnation drives the same rule through the
// 202 path, where the record is written before there is any incarnation to bind.
//
// An accepted create's recorded answer is an Operation envelope, so a host that
// bound retirement to "a uid in the recorded body" retires nothing here and the
// defect survives in full: the re-create replays the old 202, the client polls
// the settled operation, reads its terminal success naming a resource that was
// deleted, and records state for something that does not exist. The record
// follows its operation instead.
func (r *v3Runner) checkAcceptedReplayRetiresWithItsIncarnation(kv probeTarget) error {
	subject := kv
	subject.Name = "replay-retirement-async-probe"
	prepared, err := r.prepare(subject)
	if err != nil {
		return err
	}
	acceptURL, acceptHeaders, acceptBody, err := r.applyRequestParts(subject, applyOptions{
		Create: true, IdempotencyKey: "key-replay-retirement-async",
		PrepareDigest: prepared.PrepareDigest,
		ExtraHeaders:  map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	firstUID, firstOperation, err := r.acceptedCreateUID(acceptURL, acceptHeaders, acceptBody, subject)
	if err != nil {
		return err
	}
	if err := r.deleteExisting(subject, "key-replay-retirement-async-delete"); err != nil {
		return err
	}
	secondUID, secondOperation, err := r.acceptedCreateUID(acceptURL, acceptHeaders, acceptBody, subject)
	if err != nil {
		return fmt.Errorf(
			"re-accepting the byte-identical async create after its resource was removed: %w; the 202 record "+
				"follows the incarnation its operation committed", err,
		)
	}
	if secondOperation == firstOperation {
		return fmt.Errorf(
			"the re-create was answered the settled operation %s again; its commit created a resource that no "+
				"longer exists, so replaying the acceptance reports a create that will never happen",
			firstOperation,
		)
	}
	if secondUID == firstUID {
		return fmt.Errorf("the second accepted create committed the removed incarnation's uid %s", firstUID)
	}
	return r.deleteExisting(subject, "key-replay-retirement-async-teardown")
}

// acceptedCreateUID drives one accepted create to its terminal state and returns
// the uid it committed together with the operation that committed it.
func (r *v3Runner) acceptedCreateUID(
	fullURL string,
	headers map[string]string,
	body []byte,
	subject probeTarget,
) (string, string, error) {
	accepted, err := r.request(http.MethodPut, fullURL, headers, body)
	if err != nil {
		return "", "", err
	}
	if accepted.Status != http.StatusAccepted {
		return "", "", fmt.Errorf(
			"async create HTTP %d, want 202; body=%s",
			accepted.Status, strings.TrimSpace(string(accepted.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(accepted, &envelope); err != nil {
		return "", "", err
	}
	terminal, err := r.pollOperation(envelope.Operation.ID)
	if err != nil {
		return "", "", err
	}
	if terminal.Error != nil {
		return "", "", fmt.Errorf("the accepted create terminated with %v", terminal.Error)
	}
	live, _, err := r.read(subject)
	if err != nil {
		return "", "", fmt.Errorf("the accepted create settled and left nothing readable: %w", err)
	}
	return live.Metadata.UID, envelope.Operation.ID, nil
}

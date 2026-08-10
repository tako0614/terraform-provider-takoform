package portableconformancev3

// runner_operability_checks.go carries the checks that make the lane
// DEPLOYABLE behind ordinary infrastructure and safe to expose to more than one
// caller (spec/decisions/0018):
//
//   - a namespaced Form group travels as two ordinary path segments, so no
//     proxy, gateway, or framework has to agree about %2F inside a segment;
//   - an operation id and an upload id are handles bound to the tenant and
//     principal that created them, and a stranger is told the ordinary
//     not-found answer rather than a forbidden one;
//   - a content address names bytes, it does not entitle a caller to them.

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// foreignCaller is one credential that must be answered as if the handle it
// presents does not exist: another principal inside the owning tenant, and a
// principal of an entirely different tenant. Both are required, because the two
// prove different halves of the rule — a host that scoped only by tenant would
// pass the second and fail the first.
type foreignCaller struct {
	label string
	token string
}

func (r *v3Runner) foreignCallers() []foreignCaller {
	return []foreignCaller{
		{"another principal of the same tenant", r.alternateToken},
		{"a principal of another tenant", r.alternateTenantToken},
	}
}

// requireOrdinaryPathSegments holds one request target to the shape this lane
// promises: every path segment is an ordinary segment. A percent-encoded
// character in the path — above all %2F — is exactly the shape intermediaries
// disagree about, so its ABSENCE is the property being checked, not merely the
// success of the request that follows.
func requireOrdinaryPathSegments(target string) error {
	parsed, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("request target %q is not a URL: %w", target, err)
	}
	if strings.Contains(parsed.EscapedPath(), "%") {
		return fmt.Errorf(
			"request target path %q percent-encodes a character; v1beta1 path segments are ordinary segments",
			parsed.EscapedPath(),
		)
	}
	return nil
}

// checkNamespacedGroupPathSegments proves the host serves the split path shape.
// The three URL templates that name a Form group — resources, form-definitions,
// and support/forms — each carry the group NAME and the group VERSION as two
// separate segments, so "edge.forms.takoform.com/v1beta1" travels as
// "edge.forms.takoform.com/v1beta1" and nothing on the wire has to survive a
// percent-encoded slash. The exact FormRef apiVersion string is unchanged
// everywhere else, which the exact-form query on the same requests proves.
func (r *v3Runner) checkNamespacedGroupPathSegments(kv probeTarget) error {
	ref := kv.Ref
	groupName, groupVersion, split := strings.Cut(ref.APIVersion, "/")
	if !split || groupName == "" || groupVersion == "" {
		return fmt.Errorf("probe apiVersion %q is not a namespaced group/version", ref.APIVersion)
	}
	query := r.exactQuery(kv.Space, ref)
	resourceTarget := fmt.Sprintf(
		"%s/resources/%s/%s/%s/%s?%s",
		r.apiBase, url.PathEscape(groupName), url.PathEscape(groupVersion),
		url.PathEscape(ref.Kind), url.PathEscape(kv.Name), query.Encode(),
	)
	// The literal split path above and the runner's own builder must be one
	// string. A client that packed the group into a single segment would address
	// a different resource here, and every other check would still pass.
	if built := r.resourceURL(ref, kv.Name, "", query); built != resourceTarget {
		return fmt.Errorf(
			"the runner builds %q for the resource route; the split-segment shape is %q",
			built, resourceTarget,
		)
	}
	definitionTarget := fmt.Sprintf(
		"%s/form-definitions/%s/%s/%s?%s",
		r.apiBase, url.PathEscape(groupName), url.PathEscape(groupVersion),
		url.PathEscape(ref.Kind), query.Encode(),
	)
	supportTarget := fmt.Sprintf(
		"%s/support/forms/%s/%s/%s/%s",
		r.apiBase, url.PathEscape(groupName), url.PathEscape(groupVersion),
		url.PathEscape(ref.Kind), url.PathEscape(ref.DefinitionVersion),
	)
	for _, entry := range []struct{ label, target string }{
		{"resource", resourceTarget},
		{"form-definition", definitionTarget},
		{"support profile", supportTarget},
	} {
		if err := requireOrdinaryPathSegments(entry.target); err != nil {
			return fmt.Errorf("%s route: %w", entry.label, err)
		}
		response, err := r.request(http.MethodGet, entry.target, nil, nil)
		if err != nil {
			return err
		}
		if response.Status != http.StatusOK {
			return fmt.Errorf(
				"%s route with the group split across two path segments: HTTP %d; body=%s",
				entry.label, response.Status, strings.TrimSpace(string(response.Body)),
			)
		}
	}
	// The split path addresses the SAME resource the rest of the run drives, and
	// the host answers it under the whole apiVersion the two segments spell.
	response, err := r.request(http.MethodGet, resourceTarget, nil, nil)
	if err != nil {
		return err
	}
	resource, err := decodeResource(response, http.StatusOK)
	if err != nil {
		return err
	}
	if err := verifyResourceIdentity(resource, kv); err != nil {
		return fmt.Errorf("split-segment resource read: %w", err)
	}
	if resource.APIVersion != ref.APIVersion {
		return fmt.Errorf(
			"split-segment read answered apiVersion %q; the two path segments name %q",
			resource.APIVersion, ref.APIVersion,
		)
	}
	r.complete("namespaced-group-travels-as-two-path-segments")
	return nil
}

// checkOperationOwnership proves an operation id is a resumption handle bound to
// the caller that created it. A stranger holding the id gets the ordinary
// operation_not_found: a 403 would confirm the id names a real operation, which
// is the one fact the id alone must not disclose. The owner's operation is then
// driven to its terminal state, proving the refused cancel changed nothing.
func (r *v3Runner) checkOperationOwnership(kv probeTarget) error {
	target := kv
	target.Name = "operation-owner-probe"
	accepted, err := r.apply(target, applyOptions{
		Create:         true,
		IdempotencyKey: "key-operation-owner-create",
		ExtraHeaders:   map[string]string{ErrorProbeHeader: ProbeAsync},
	})
	if err != nil {
		return err
	}
	if accepted.Status != http.StatusAccepted {
		return fmt.Errorf(
			"async apply HTTP %d, want 202; body=%s",
			accepted.Status, strings.TrimSpace(string(accepted.Body)),
		)
	}
	var envelope struct {
		Operation wireOperation `json:"operation"`
	}
	if err := decodeStrictResponse(accepted, &envelope); err != nil {
		return err
	}
	if envelope.Operation.ID == "" || envelope.Operation.Done {
		return fmt.Errorf("202 operation envelope is invalid: %+v", envelope.Operation)
	}
	operationURL := r.apiBase + "/operations/" + url.PathEscape(envelope.Operation.ID)
	for index, foreign := range r.foreignCallers() {
		read, err := r.requestWithToken(foreign.token, http.MethodGet, operationURL, nil, nil)
		if err != nil {
			return err
		}
		if err := r.expectStableError(read, "operation_not_found"); err != nil {
			return fmt.Errorf("operation read by %s: %w", foreign.label, err)
		}
		cancel, err := r.requestWithToken(
			foreign.token, http.MethodPost, operationURL+"/cancel",
			map[string]string{"Idempotency-Key": fmt.Sprintf("key-operation-owner-foreign-cancel-%d", index)},
			nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(cancel, "operation_not_found"); err != nil {
			return fmt.Errorf("operation cancel by %s: %w", foreign.label, err)
		}
	}
	// Nothing a stranger did reached the operation: it is still the owner's, and
	// it still settles into the resource it was accepted to create.
	terminal, err := r.pollOperation(envelope.Operation.ID)
	if err != nil {
		return fmt.Errorf("the owner's operation after refused foreign access: %w", err)
	}
	if terminal.Error != nil || terminal.Result == nil {
		return fmt.Errorf("a refused foreign cancel changed the owner's operation: %+v", terminal)
	}
	if _, _, err := r.read(target); err != nil {
		return fmt.Errorf("the owner's accepted mutation did not commit: %w", err)
	}
	r.complete("operation-bound-to-its-creating-principal")
	return nil
}

// ownershipUploadManifest is one WorkerBundle manifest whose bytes belong to no
// other check, so an ownership probe can stage, abandon, and re-stage it
// without touching the artifact any resource references.
func ownershipUploadManifest() (map[string]any, string, string) {
	source := "export default { async fetch() { return new Response(\"ownership\"); } };\n"
	digest := formpackage.DigestBytes([]byte(source))
	return map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "ownership.js",
		"modules": []any{map[string]any{
			"name":      "ownership.js",
			"mediaType": "application/javascript+module",
			"size":      len(source),
			"digest":    digest,
		}},
	}, source, digest
}

// startUploadAs starts one upload session under a named credential and returns
// the raw response, so a caller can hold a refused start to its exact code.
func (r *v3Runner) startUploadAs(token string, manifest map[string]any, key string) (wireResponse, error) {
	body, err := encodeRunnerJSON(map[string]any{"manifest": manifest})
	if err != nil {
		return wireResponse{}, err
	}
	return r.requestWithToken(
		token, http.MethodPost, r.apiBase+"/artifacts/uploads",
		map[string]string{"Idempotency-Key": key}, body,
	)
}

// decodeUploadStatus reads one artifact upload status document.
func decodeUploadStatus(response wireResponse) (string, []string, error) {
	if response.Status != http.StatusOK && response.Status != http.StatusCreated {
		return "", nil, fmt.Errorf(
			"artifact upload start HTTP %d; body=%s", response.Status, strings.TrimSpace(string(response.Body)),
		)
	}
	var status struct {
		UploadID     string   `json:"uploadId"`
		MissingBlobs []string `json:"missingBlobs"`
	}
	if err := decodeStrictResponse(response, &status); err != nil {
		return "", nil, err
	}
	if status.UploadID == "" || status.MissingBlobs == nil {
		return "", nil, errors.New("artifact upload status shape is invalid")
	}
	return status.UploadID, status.MissingBlobs, nil
}

// uploadAndCommitManifestAs drives one complete upload under a named
// credential — start, the blobs the host reports missing FOR THAT CALLER, and
// commit — and returns the committed manifest digest. The missing set is a
// per-tenant answer, so the same manifest legitimately requires bytes from one
// caller and not from another.
func (r *v3Runner) uploadAndCommitManifestAs(
	token string,
	manifest map[string]any,
	blobs map[string][]byte,
	keyPrefix string,
) (string, error) {
	startResponse, err := r.startUploadAs(token, manifest, keyPrefix+"-start")
	if err != nil {
		return "", err
	}
	uploadID, missing, err := decodeUploadStatus(startResponse)
	if err != nil {
		return "", err
	}
	for _, digest := range missing {
		blob, staged := blobs[digest]
		if !staged {
			return "", fmt.Errorf("host requires blob %s which this check did not stage", digest)
		}
		uploaded, err := r.requestWithToken(
			token, http.MethodPut,
			r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID)+"/blobs/"+url.PathEscape(digest),
			map[string]string{"Content-Type": "application/octet-stream"}, blob,
		)
		if err != nil {
			return "", err
		}
		if uploaded.Status != http.StatusCreated && uploaded.Status != http.StatusNoContent {
			return "", fmt.Errorf("blob upload HTTP %d", uploaded.Status)
		}
	}
	commit, err := r.requestWithToken(
		token, http.MethodPost,
		r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID)+"/commit",
		map[string]string{"Idempotency-Key": keyPrefix + "-commit"}, nil,
	)
	if err != nil {
		return "", err
	}
	if commit.Status != http.StatusOK && commit.Status != http.StatusCreated {
		return "", fmt.Errorf(
			"artifact commit HTTP %d; body=%s", commit.Status, strings.TrimSpace(string(commit.Body)),
		)
	}
	var result struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if err := decodeStrictResponse(commit, &result); err != nil {
		return "", err
	}
	if !formpackage.ValidDigest(result.ManifestDigest) {
		return "", errors.New("artifact commit returned an invalid manifestDigest")
	}
	return result.ManifestDigest, nil
}

// checkUploadSessionOwnership proves an upload id is bound the same way an
// operation id is. Continuing (blob PUT), committing, and abandoning a session
// the caller does not own each fail with the surface's ordinary not-found code,
// artifact_missing — including abandon, which must not answer 204 for a foreign
// id while answering 404 for an unknown one, because that difference is itself
// the disclosure. The owner then completes the session it still holds.
func (r *v3Runner) checkUploadSessionOwnership() error {
	manifest, source, blobDigest := ownershipUploadManifest()
	startResponse, err := r.startUploadAs(r.token, manifest, "key-upload-owner-start")
	if err != nil {
		return err
	}
	uploadID, missing, err := decodeUploadStatus(startResponse)
	if err != nil {
		return err
	}
	if len(missing) != 1 || missing[0] != blobDigest {
		return fmt.Errorf("missingBlobs = %v, want exactly the ownership probe module", missing)
	}
	uploadURL := r.apiBase + "/artifacts/uploads/" + url.PathEscape(uploadID)
	blobURL := uploadURL + "/blobs/" + url.PathEscape(blobDigest)
	for index, foreign := range r.foreignCallers() {
		staged, err := r.requestWithToken(
			foreign.token, http.MethodPut, blobURL,
			map[string]string{"Content-Type": "application/octet-stream"}, []byte(source),
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(staged, "artifact_missing"); err != nil {
			return fmt.Errorf("blob upload into a session held by someone else, by %s: %w", foreign.label, err)
		}
		committed, err := r.requestWithToken(
			foreign.token, http.MethodPost, uploadURL+"/commit",
			map[string]string{"Idempotency-Key": fmt.Sprintf("key-upload-owner-foreign-commit-%d", index)}, nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(committed, "artifact_missing"); err != nil {
			return fmt.Errorf("commit of a session held by someone else, by %s: %w", foreign.label, err)
		}
		abandoned, err := r.requestWithToken(
			foreign.token, http.MethodDelete, uploadURL,
			map[string]string{"Idempotency-Key": fmt.Sprintf("key-upload-owner-foreign-abandon-%d", index)}, nil,
		)
		if err != nil {
			return err
		}
		if err := r.expectStableError(abandoned, "artifact_missing"); err != nil {
			return fmt.Errorf("abandon of a session held by someone else, by %s: %w", foreign.label, err)
		}
	}
	// The session survived every refused attempt, so the owner still completes it.
	staged, err := r.request(
		http.MethodPut, blobURL,
		map[string]string{"Content-Type": "application/octet-stream"}, []byte(source),
	)
	if err != nil {
		return err
	}
	if staged.Status != http.StatusCreated && staged.Status != http.StatusNoContent {
		return fmt.Errorf("the owner's blob upload HTTP %d after refused foreign access", staged.Status)
	}
	committed, err := r.commitArtifact(uploadID, "key-upload-owner-commit")
	if err != nil {
		return err
	}
	if committed.Status != http.StatusOK && committed.Status != http.StatusCreated {
		return fmt.Errorf(
			"the owner's commit HTTP %d; body=%s",
			committed.Status, strings.TrimSpace(string(committed.Body)),
		)
	}
	r.complete("upload-session-bound-to-its-creating-principal")
	return nil
}

// checkArtifactDigestIsNotACapability proves a content address is a name, not an
// entitlement. Another tenant holding the exact manifest digest and the exact
// blob digest reads neither; a second principal of the HOLDING tenant reads
// both, which is what makes this an authorization rule rather than a
// per-session one. The alternate tenant then uploads the same bytes and
// commits the same manifest: it must be told the blob is missing for IT — a
// host that reported the physically deduplicated blob as already present would
// be handing over bytes on the strength of a digest — and once it has supplied
// them it holds the identical, physically shared artifact.
func (r *v3Runner) checkArtifactDigestIsNotACapability() error {
	manifestDigest := r.artifactManifestDigest
	if manifestDigest == "" {
		return errors.New("no committed manifest digest was recorded by the artifact checks")
	}
	moduleSource := r.contract.RunnerInput.WorkerBundle.ModuleSource
	blobDigest := formpackage.DigestBytes([]byte(moduleSource))
	manifestURL := r.apiBase + "/artifacts/" + url.PathEscape(manifestDigest)
	blobURL := r.apiBase + "/artifacts/blobs/" + url.PathEscape(blobDigest)

	foreignManifest, err := r.requestWithToken(r.alternateTenantToken, http.MethodGet, manifestURL, nil, nil)
	if err != nil {
		return err
	}
	if err := r.expectStableError(foreignManifest, "artifact_missing"); err != nil {
		return fmt.Errorf("manifest read by a tenant that does not hold it: %w", err)
	}
	foreignBlob, err := r.requestWithToken(r.alternateTenantToken, http.MethodHead, blobURL, nil, nil)
	if err != nil {
		return err
	}
	if foreignBlob.Status != http.StatusNotFound {
		return fmt.Errorf(
			"blob HEAD by a tenant that does not hold it: HTTP %d, want 404",
			foreignBlob.Status,
		)
	}

	// The holding is the TENANT's, not one principal's.
	sameTenantManifest, err := r.requestWithToken(r.alternateToken, http.MethodGet, manifestURL, nil, nil)
	if err != nil {
		return err
	}
	if sameTenantManifest.Status != http.StatusOK {
		return fmt.Errorf(
			"manifest read by another principal of the HOLDING tenant: HTTP %d, want 200",
			sameTenantManifest.Status,
		)
	}
	sameTenantBlob, err := r.requestWithToken(r.alternateToken, http.MethodHead, blobURL, nil, nil)
	if err != nil {
		return err
	}
	if sameTenantBlob.Status != http.StatusOK {
		return fmt.Errorf(
			"blob HEAD by another principal of the HOLDING tenant: HTTP %d, want 200",
			sameTenantBlob.Status,
		)
	}

	// The same bytes, uploaded by the other tenant, are readable by it.
	manifest := r.contract.RunnerInput.WorkerBundle.Manifest
	startResponse, err := r.startUploadAs(r.alternateTenantToken, manifest, "key-artifact-other-tenant-start")
	if err != nil {
		return err
	}
	uploadID, missing, err := decodeUploadStatus(startResponse)
	if err != nil {
		return err
	}
	held := false
	for _, digest := range missing {
		if digest == blobDigest {
			held = true
		}
	}
	if !held {
		return fmt.Errorf(
			"missingBlobs = %v for a tenant that holds nothing; a physically deduplicated blob "+
				"must still be uploaded by the tenant that wants to hold it",
			missing,
		)
	}
	staged, err := r.requestWithToken(
		r.alternateTenantToken, http.MethodPut,
		r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID)+"/blobs/"+url.PathEscape(blobDigest),
		map[string]string{"Content-Type": "application/octet-stream"}, []byte(moduleSource),
	)
	if err != nil {
		return err
	}
	if staged.Status != http.StatusCreated && staged.Status != http.StatusNoContent {
		return fmt.Errorf("the other tenant's blob upload HTTP %d", staged.Status)
	}
	committed, err := r.requestWithToken(
		r.alternateTenantToken, http.MethodPost,
		r.apiBase+"/artifacts/uploads/"+url.PathEscape(uploadID)+"/commit",
		map[string]string{"Idempotency-Key": "key-artifact-other-tenant-commit"}, nil,
	)
	if err != nil {
		return err
	}
	if committed.Status != http.StatusOK && committed.Status != http.StatusCreated {
		return fmt.Errorf(
			"the other tenant's commit HTTP %d; body=%s",
			committed.Status, strings.TrimSpace(string(committed.Body)),
		)
	}
	var commitResult struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if err := decodeStrictResponse(committed, &commitResult); err != nil {
		return err
	}
	if commitResult.ManifestDigest != manifestDigest {
		return fmt.Errorf(
			"the same manifest bytes committed by another tenant answered digest %s, want %s",
			commitResult.ManifestDigest, manifestDigest,
		)
	}
	nowReadable, err := r.requestWithToken(r.alternateTenantToken, http.MethodGet, manifestURL, nil, nil)
	if err != nil {
		return err
	}
	if nowReadable.Status != http.StatusOK {
		return fmt.Errorf(
			"manifest read by the tenant that just committed it: HTTP %d, want 200", nowReadable.Status,
		)
	}
	roundTrip, err := formpackage.DigestCanonicalJSON(nowReadable.Body)
	if err != nil || roundTrip != manifestDigest {
		return errors.New("the manifest the second tenant reads is not the one it committed")
	}
	nowHeld, err := r.requestWithToken(r.alternateTenantToken, http.MethodHead, blobURL, nil, nil)
	if err != nil {
		return err
	}
	if nowHeld.Status != http.StatusOK {
		return fmt.Errorf("blob HEAD by the tenant that just uploaded it: HTTP %d, want 200", nowHeld.Status)
	}
	// Physical dedup did not disturb the first holder.
	original, err := r.request(http.MethodGet, manifestURL, nil, nil)
	if err != nil {
		return err
	}
	if original.Status != http.StatusOK {
		return fmt.Errorf("the first holder lost its manifest: HTTP %d", original.Status)
	}
	originalRoundTrip, err := formpackage.DigestCanonicalJSON(original.Body)
	if err != nil || originalRoundTrip != manifestDigest {
		return errors.New("a second tenant's commit changed the first holder's manifest")
	}
	r.complete("artifact-digest-is-not-a-capability")
	return nil
}

// foreignUseBundleManifest is one WorkerBundle manifest owned by the reference
// rule below and by nothing else, so proving what a foreign tenant may not
// REFERENCE never depends on which other check has already uploaded what.
func foreignUseBundleManifest() (map[string]any, string, string) {
	source := "export default { async fetch() { return new Response(\"reference\"); } };\n"
	digest := formpackage.DigestBytes([]byte(source))
	return map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "reference.js",
		"modules": []any{map[string]any{
			"name":      "reference.js",
			"mediaType": "application/javascript+module",
			"size":      len(source),
			"digest":    digest,
		}},
	}, source, digest
}

// checkManifestReferenceIsNotACapability proves the holding rule governs USING
// a content address, not only reading one. A bundle-shaped resource carries a
// manifest digest as its whole desired state, so a host that resolves that
// digest against the byte store instead of against the caller's tenant hands a
// stranger another tenant's deployable code through desired state — the same
// disclosure the read surfaces refuse, through the resource API.
//
// The refusal is the ordinary artifact_missing a never-committed digest gets,
// before any mutation, on apply and import alike, and nothing is stored. The
// rule is about authorization and not about refusing shared content: a second
// principal of the HOLDING tenant references the same manifest successfully,
// and the foreign tenant that uploads the same bytes itself then references the
// same immutable identity successfully too.
func (r *v3Runner) checkManifestReferenceIsNotACapability(bundle probeTarget) error {
	manifest, source, blobDigest := foreignUseBundleManifest()
	blobs := map[string][]byte{blobDigest: []byte(source)}
	manifestDigest, err := r.uploadAndCommitManifestAs(r.token, manifest, blobs, "key-reference-holder")
	if err != nil {
		return fmt.Errorf("committing the reference probe manifest: %w", err)
	}

	// Holding is the TENANT's: the principal that uploads a bundle and the
	// principal that references it are routinely different
	// (spec/decisions/0018).
	holderUse := bundle
	holderUse.Name = "bundle-holder-reference-probe"
	holderUse.Spec = map[string]any{"manifestDigest": manifestDigest}
	holderApply, err := r.applyAs(r.alternateToken, holderUse, applyOptions{
		Create: true, IdempotencyKey: "key-reference-holder-apply",
	})
	if err != nil {
		return err
	}
	if holderApply.Status != http.StatusCreated {
		return fmt.Errorf(
			"apply by another principal of the HOLDING tenant: HTTP %d, want 201; body=%s",
			holderApply.Status, strings.TrimSpace(string(holderApply.Body)),
		)
	}

	foreignUse := bundle
	foreignUse.Name = "bundle-foreign-reference-probe"
	foreignUse.Spec = map[string]any{"manifestDigest": manifestDigest}
	foreignApply, err := r.applyAs(r.alternateTenantToken, foreignUse, applyOptions{
		Create: true, IdempotencyKey: "key-reference-foreign-apply",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(foreignApply, "artifact_missing"); err != nil {
		return fmt.Errorf("apply referencing a manifest the caller's tenant does not hold: %w", err)
	}
	if err := r.expectResourceAbsent(foreignUse); err != nil {
		return fmt.Errorf("a refused foreign manifest reference stored a resource: %w", err)
	}
	foreignImport, err := r.importResourceAs(r.alternateTenantToken, foreignUse, importOptions{
		NativeID: "native-foreign-reference-probe", IdempotencyKey: "key-reference-foreign-import", Create: true,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(foreignImport, "artifact_missing"); err != nil {
		return fmt.Errorf("import referencing a manifest the caller's tenant does not hold: %w", err)
	}
	if err := r.expectResourceAbsent(foreignUse); err != nil {
		return fmt.Errorf("a refused foreign manifest adoption stored a resource: %w", err)
	}

	// The same bytes, supplied by the other tenant itself, make the same
	// immutable identity referenceable by it.
	foreignDigest, err := r.uploadAndCommitManifestAs(
		r.alternateTenantToken, manifest, blobs, "key-reference-foreign",
	)
	if err != nil {
		return fmt.Errorf("the other tenant committing the same manifest: %w", err)
	}
	if foreignDigest != manifestDigest {
		return fmt.Errorf(
			"the same manifest bytes committed by another tenant answered digest %s, want %s",
			foreignDigest, manifestDigest,
		)
	}
	nowUsable, err := r.applyAs(r.alternateTenantToken, foreignUse, applyOptions{
		Create: true, IdempotencyKey: "key-reference-foreign-apply-held",
	})
	if err != nil {
		return err
	}
	if nowUsable.Status != http.StatusCreated {
		return fmt.Errorf(
			"apply by the tenant that just committed the manifest: HTTP %d, want 201; body=%s",
			nowUsable.Status, strings.TrimSpace(string(nowUsable.Body)),
		)
	}
	r.complete("manifest-reference-is-not-a-capability")
	return nil
}

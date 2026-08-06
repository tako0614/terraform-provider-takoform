package clientv3

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

var uploadIDPattern = regexp.MustCompile(`^up_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$`)

// artifactUploadStatus is the response of an upload start: the resumable
// uploadId and the digests of blobs the host does not already hold.
type artifactUploadStatus struct {
	UploadID     string   `json:"uploadId"`
	MissingBlobs []string `json:"missingBlobs"`
}

// UploadArtifact runs the content-addressed upload flow
// (spec/artifact-transport): it submits the manifest, uploads only the blobs
// the host reports missing (each verified locally against its digest key
// before any bytes leave the client), commits, and returns the immutable
// manifestDigest — the RFC 8785 canonical digest of the manifest, which is
// verified locally against the manifest that was sent. Committing the same
// manifest again is idempotent and returns the same digest. blobs keys are
// full "sha256:<hex>" digests.
func (c *Client) UploadArtifact(ctx context.Context, manifest map[string]any, blobs map[string][]byte) (string, error) {
	if err := c.requireReady(); err != nil {
		return "", err
	}
	if manifest == nil {
		return "", errors.New("takoform: artifact upload requires a manifest")
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		return "", fmt.Errorf("takoform: encoding artifact manifest: %w", err)
	}
	expectedDigest, err := digestCanonical(manifestRaw)
	if err != nil {
		return "", fmt.Errorf("takoform: canonicalizing artifact manifest: %w", err)
	}

	startBody, err := json.Marshal(map[string]json.RawMessage{"manifest": manifestRaw})
	if err != nil {
		return "", fmt.Errorf("takoform: encoding artifact upload request: %w", err)
	}
	// The upload-start key folds in a fresh random nonce per invocation: a
	// replay-conforming host would otherwise return the ORIGINAL missingBlobs
	// forever for the same manifest, defeating resumable re-uploads. Transport
	// retries inside this one call reuse the same headers, so retry safety is
	// preserved; the commit key stays deterministic because it binds the
	// host-issued uploadId.
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", fmt.Errorf("takoform: generating artifact upload-start nonce: %w", err)
	}
	startURL := c.apiBase + "/artifacts/uploads"
	startHeaders := map[string]string{
		"Idempotency-Key": mutationKey("artifact-upload-start", expectedDigest, hex.EncodeToString(nonceBytes)),
	}
	_, _, data, err := c.do(
		ctx, http.MethodPost, startURL, startHeaders, startBody, true,
		http.StatusOK, http.StatusCreated,
	)
	if err != nil {
		return "", err
	}
	var status artifactUploadStatus
	if err := decodeBody(data, startURL, &status); err != nil {
		return "", err
	}
	if !uploadIDPattern.MatchString(status.UploadID) {
		return "", errors.New("takoform: host uploadId must match ^up_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$")
	}
	if status.MissingBlobs == nil {
		return "", errors.New("takoform: host upload status omitted missingBlobs")
	}

	for _, digest := range status.MissingBlobs {
		if !validWireDigest(digest) {
			return "", fmt.Errorf("takoform: host reported an invalid missing blob digest %q", digest)
		}
		blob, held := blobs[digest]
		if !held {
			return "", fmt.Errorf("takoform: host requires blob %s which was not provided", digest)
		}
		localSum := sha256.Sum256(blob)
		if "sha256:"+hex.EncodeToString(localSum[:]) != digest {
			return "", fmt.Errorf("takoform: local blob bytes do not match their digest key %s", digest)
		}
		blobURL := fmt.Sprintf(
			"%s/artifacts/uploads/%s/blobs/%s",
			c.apiBase, url.PathEscape(status.UploadID), url.PathEscape(digest),
		)
		blobHeaders := map[string]string{"Content-Type": "application/octet-stream"}
		if _, _, _, err := c.do(
			ctx, http.MethodPut, blobURL, blobHeaders, blob, false,
			http.StatusCreated, http.StatusNoContent,
		); err != nil {
			return "", err
		}
	}

	commitURL := fmt.Sprintf("%s/artifacts/uploads/%s/commit", c.apiBase, url.PathEscape(status.UploadID))
	commitHeaders := map[string]string{
		"Idempotency-Key": mutationKey("artifact-commit", status.UploadID, expectedDigest),
	}
	commitStatus, responseHeaders, commitData, err := c.do(
		ctx, http.MethodPost, commitURL, commitHeaders, nil, true,
		http.StatusOK, http.StatusCreated, http.StatusAccepted,
	)
	if err != nil {
		return "", err
	}
	var commit struct {
		ManifestDigest string `json:"manifestDigest"`
	}
	if commitStatus == http.StatusAccepted {
		operation, err := c.decodeOperationEnvelope(commitData, commitURL)
		if err != nil {
			return "", err
		}
		terminal, err := c.awaitOperation(ctx, operation, parseRetryAfter(responseHeaders.Get("Retry-After")), 0)
		if err != nil {
			return "", err
		}
		if err := decodeBody(terminal.Result, commitURL, &commit); err != nil {
			return "", fmt.Errorf("takoform: terminal artifact commit result is not a commit response: %w", err)
		}
	} else if err := decodeBody(commitData, commitURL, &commit); err != nil {
		return "", err
	}
	if commit.ManifestDigest != expectedDigest {
		return "", fmt.Errorf(
			"takoform: host committed manifest digest %s, expected the canonical digest %s of the submitted manifest",
			commit.ManifestDigest, expectedDigest,
		)
	}
	return commit.ManifestDigest, nil
}

// AbandonUpload releases a resumable upload session this client will not
// finish (spec/host-api/operations-v1alpha3.json: "artifact-upload-abandon",
// DELETE /artifacts/uploads/{uploadId}, 204). It frees the host's staged
// blobs. An already-committed manifest is unaffected: a committed artifact is
// content-addressed and outlives the upload session that carried its bytes.
func (c *Client) AbandonUpload(ctx context.Context, uploadID string) error {
	if err := c.requireReady(); err != nil {
		return err
	}
	if !uploadIDPattern.MatchString(uploadID) {
		return errors.New("takoform: uploadId must match ^up_[A-Za-z0-9][A-Za-z0-9._-]{0,124}$")
	}
	fullURL := c.apiBase + "/artifacts/uploads/" + url.PathEscape(uploadID)
	headers := map[string]string{
		"Idempotency-Key": mutationKey("artifact-upload-abandon", uploadID),
	}
	_, _, _, err := c.do(ctx, http.MethodDelete, fullURL, headers, nil, true, http.StatusNoContent)
	return err
}

// HeadBlob probes whether the host already holds a blob. 200 means present,
// 404 means absent; every other outcome is an error.
func (c *Client) HeadBlob(ctx context.Context, digest string) (bool, error) {
	if err := c.requireReady(); err != nil {
		return false, err
	}
	if !validWireDigest(digest) {
		return false, errors.New("takoform: blob digest must be a lowercase sha256:<hex> digest")
	}
	fullURL := c.apiBase + "/artifacts/blobs/" + url.PathEscape(digest)
	_, _, _, err := c.do(ctx, http.MethodHead, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// GetArtifactManifest reads a committed manifest by its immutable digest and
// verifies the content address: the RFC 8785 canonical digest of the
// returned document must equal the requested digest.
func (c *Client) GetArtifactManifest(ctx context.Context, digest string) (map[string]any, error) {
	if err := c.requireReady(); err != nil {
		return nil, err
	}
	if !validWireDigest(digest) {
		return nil, errors.New("takoform: manifest digest must be a lowercase sha256:<hex> digest")
	}
	fullURL := c.apiBase + "/artifacts/" + url.PathEscape(digest)
	_, _, data, err := c.do(ctx, http.MethodGet, fullURL, nil, nil, false, http.StatusOK)
	if err != nil {
		return nil, err
	}
	actualDigest, err := digestCanonical(data)
	if err != nil {
		return nil, fmt.Errorf("takoform: canonicalizing artifact manifest response: %w", err)
	}
	if actualDigest != digest {
		return nil, errors.New("takoform: host returned a manifest whose canonical digest does not match its content address")
	}
	var manifest map[string]any
	if err := decodeBody(data, fullURL, &manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

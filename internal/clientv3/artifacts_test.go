package clientv3

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func testManifest(blobDigest string) map[string]any {
	return map[string]any{
		"apiVersion": "artifacts.takoform.com/v1alpha1",
		"kind":       "WorkerBundle",
		"mainModule": "worker.js",
		"modules": []map[string]any{{
			"name":      "worker.js",
			"mediaType": "application/javascript+module",
			"size":      int64(21),
			"digest":    blobDigest,
		}},
	}
}

func TestUploadArtifactFlowAndCommitIdempotency(t *testing.T) {
	blob := []byte(`export default null;` + "\n")
	sum := sha256.Sum256(blob)
	blobDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifest := testManifest(blobDigest)

	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	manifestDigest, err := formpackage.DigestCanonicalJSON(manifestRaw)
	if err != nil {
		t.Fatalf("digest manifest: %v", err)
	}

	starts, blobPuts, commits := 0, 0, 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/artifacts/uploads":
			starts++
			if r.Header.Get("Idempotency-Key") == "" {
				t.Errorf("upload start must send an Idempotency-Key")
			}
			missing := []string{}
			if starts == 1 {
				missing = []string{blobDigest}
			}
			writeJSON(t, w, http.StatusOK, map[string]any{
				"uploadId": "up_1", "missingBlobs": missing,
			})
			return true
		case r.Method == http.MethodPut &&
			r.URL.EscapedPath() == APIRootPath+"/artifacts/uploads/up_1/blobs/"+url.PathEscape(blobDigest):
			blobPuts++
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("blob upload must be application/octet-stream, got %q", r.Header.Get("Content-Type"))
			}
			raw, _ := io.ReadAll(r.Body)
			received := sha256.Sum256(raw)
			if "sha256:"+hex.EncodeToString(received[:]) != blobDigest {
				t.Errorf("blob bytes do not match their digest")
			}
			w.WriteHeader(http.StatusCreated)
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/artifacts/uploads/up_1/commit":
			commits++
			writeJSON(t, w, http.StatusOK, map[string]any{"manifestDigest": manifestDigest})
			return true
		}
		return false
	})

	blobs := map[string][]byte{blobDigest: blob}
	first, err := client.UploadArtifact(context.Background(), manifest, blobs)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	if first != manifestDigest {
		t.Fatalf("committed digest %s is not the canonical manifest digest %s", first, manifestDigest)
	}
	if blobPuts != 1 {
		t.Fatalf("expected exactly one missing-blob upload, saw %d", blobPuts)
	}

	// Re-uploading the same manifest is resumable and commit is idempotent:
	// no blobs are missing and the same digest comes back.
	second, err := client.UploadArtifact(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if second != first {
		t.Fatalf("idempotent commit returned a different digest: %s != %s", second, first)
	}
	if blobPuts != 1 {
		t.Fatalf("second upload must not re-send blobs, saw %d puts", blobPuts)
	}
	if commits != 2 {
		t.Fatalf("expected two commits, saw %d", commits)
	}
}

func TestUploadArtifactRejectsCorruptLocalBlob(t *testing.T) {
	blob := []byte("real bytes")
	sum := sha256.Sum256(blob)
	blobDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifest := testManifest(blobDigest)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/artifacts/uploads" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"uploadId": "up_1", "missingBlobs": []string{blobDigest},
			})
			return true
		}
		return false
	})
	// The caller's bytes do not match the digest key; nothing may be sent.
	_, err := client.UploadArtifact(context.Background(), manifest, map[string][]byte{blobDigest: []byte("corrupt")})
	if err == nil || !contains(err, "do not match their digest key") {
		t.Fatalf("expected local digest mismatch rejection, got %v", err)
	}
}

func TestHeadBlobPresenceProbe(t *testing.T) {
	present := "sha256:" + hexRepeat("1", 64)
	absent := "sha256:" + hexRepeat("2", 64)
	large := "sha256:" + hexRepeat("5", 64)
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodHead {
			switch r.URL.EscapedPath() {
			case APIRootPath + "/artifacts/blobs/" + url.PathEscape(present):
				w.WriteHeader(http.StatusOK)
			case APIRootPath + "/artifacts/blobs/" + url.PathEscape(large):
				// A HEAD response transfers no body; the advertised size of the
				// would-be GET representation may exceed the 8MiB response cap.
				w.Header().Set("Content-Length", "16777217")
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
			return true
		}
		return false
	})
	if held, err := client.HeadBlob(context.Background(), present); err != nil || !held {
		t.Fatalf("expected present blob, got %v %v", held, err)
	}
	if held, err := client.HeadBlob(context.Background(), absent); err != nil || held {
		t.Fatalf("expected absent blob, got %v %v", held, err)
	}
	if held, err := client.HeadBlob(context.Background(), large); err != nil || !held {
		t.Fatalf("expected present blob despite a >8MiB Content-Length, got %v %v", held, err)
	}
}

// TestUploadArtifactStartKeyIsFreshPerInvocation drives a fake host that
// replays upload-start strictly by Idempotency-Key. A deterministic key would
// make the second invocation see the FIRST invocation's missingBlobs forever;
// the per-invocation nonce lets the host answer with the current blob state,
// so the second UploadArtifact converges without re-supplied bytes.
func TestUploadArtifactStartKeyIsFreshPerInvocation(t *testing.T) {
	blob := []byte(`export default null;` + "\n")
	sum := sha256.Sum256(blob)
	blobDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifest := testManifest(blobDigest)

	held := map[string]bool{}
	var startKeys []string
	startReplays := map[string][]byte{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/artifacts/uploads":
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				t.Errorf("upload start must send an Idempotency-Key")
			}
			startKeys = append(startKeys, key)
			if recorded, ok := startReplays[key]; ok {
				// Replay-conforming host: the recorded response, byte for byte.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(recorded)
				return true
			}
			missing := []string{}
			if !held[blobDigest] {
				missing = []string{blobDigest}
			}
			raw, err := json.Marshal(map[string]any{"uploadId": "up_1", "missingBlobs": missing})
			if err != nil {
				t.Fatalf("encoding upload-start response: %v", err)
			}
			startReplays[key] = raw
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(raw)
			return true
		case r.Method == http.MethodPut &&
			r.URL.EscapedPath() == APIRootPath+"/artifacts/uploads/up_1/blobs/"+url.PathEscape(blobDigest):
			held[blobDigest] = true
			w.WriteHeader(http.StatusCreated)
			return true
		case r.Method == http.MethodPost && r.URL.Path == APIRootPath+"/artifacts/uploads/up_1/commit":
			manifestRaw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatalf("marshal manifest: %v", err)
			}
			digest, err := formpackage.DigestCanonicalJSON(manifestRaw)
			if err != nil {
				t.Fatalf("digest manifest: %v", err)
			}
			writeJSON(t, w, http.StatusOK, map[string]any{"manifestDigest": digest})
			return true
		}
		return false
	})

	first, err := client.UploadArtifact(context.Background(), manifest, map[string][]byte{blobDigest: blob})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	// The second invocation supplies no blob bytes: it converges only when the
	// host reports the CURRENT missing set instead of replaying the first one.
	second, err := client.UploadArtifact(context.Background(), manifest, nil)
	if err != nil {
		t.Fatalf("second upload against a key-replaying host: %v", err)
	}
	if second != first {
		t.Fatalf("second upload digest %s != first %s", second, first)
	}
	if len(startKeys) != 2 || startKeys[0] == startKeys[1] {
		t.Fatalf("upload-start keys must be fresh per invocation, got %v", startKeys)
	}
}

func TestGetArtifactManifestVerifiesContentAddress(t *testing.T) {
	manifest := testManifest("sha256:" + hexRepeat("3", 64))
	raw, _ := json.Marshal(manifest)
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(raw)
			return true
		}
		return false
	})
	got, err := client.GetArtifactManifest(context.Background(), digest)
	if err != nil {
		t.Fatalf("manifest read: %v", err)
	}
	if got["kind"] != "WorkerBundle" {
		t.Fatalf("manifest not decoded: %v", got)
	}
	// A different requested digest must fail the content-address check.
	other := "sha256:" + hexRepeat("4", 64)
	if _, err := client.GetArtifactManifest(context.Background(), other); err == nil ||
		!contains(err, "content address") {
		t.Fatalf("expected content-address mismatch, got %v", err)
	}
}

func hexRepeat(ch string, count int) string {
	out := ""
	for i := 0; i < count; i++ {
		out += ch
	}
	return out
}

package portableconformancev3

// artifact_manifest_test.go covers the two rules that make a manifest digest a
// safe desired state: a manifest means exactly one thing per kind, and a
// committed manifest a resource references stays resolvable.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const artifactModuleSource = "export default { async fetch() { return new Response(\"unit\"); } };\n"

func canonicalManifestDigest(t *testing.T, manifest map[string]any) (string, []byte) {
	t.Helper()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return digest, raw
}

func workerBundleUnitManifest() map[string]any {
	return map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "index.js",
		"modules": []any{map[string]any{
			"name":      "index.js",
			"mediaType": "application/javascript+module",
			"size":      len(artifactModuleSource),
			"digest":    formpackage.DigestBytes([]byte(artifactModuleSource)),
		}},
	}
}

func assetFileEntry(body string) map[string]any {
	return map[string]any{
		"path":      "index.html",
		"mediaType": "text/html",
		"size":      len(body),
		"digest":    formpackage.DigestBytes([]byte(body)),
	}
}

// stageUpload plants one already-started upload session on the host so the
// COMMIT path can be driven directly. It models the one thing a black-box
// upload cannot reach: a host laxer than this one, whose upload start admitted
// a manifest it should have refused. Commit is where an immutable identity is
// minted, so every closure rule has to hold there too.
func stageUpload(t *testing.T, host *ReferenceHost, id string, manifest map[string]any, blobs map[string]string) string {
	t.Helper()
	digest, raw := canonicalManifestDigest(t, manifest)
	var decoded artifactManifest
	if err := formpackage.DecodeStrictIJSON(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	host.uploads[id] = &artifactUpload{
		ID: id, ManifestRaw: raw, Manifest: decoded, ManifestDigest: digest,
	}
	for blobDigest, body := range blobs {
		host.blobs[blobDigest] = []byte(body)
	}
	return digest
}

// TestArtifactManifestPerKindClosureIsHostEnforced proves the exclusivity the
// published manifest schema cannot state (spec/decisions/0014): the schema
// declares mainModule, modules, and files for every kind, so only the host
// stops a manifest from carrying two shapes at once. Each mixture is refused
// at commit and never becomes readable.
func TestArtifactManifestPerKindClosureIsHostEnforced(t *testing.T) {
	cases := []struct {
		name     string
		manifest map[string]any
	}{
		{
			name: "worker bundle carrying asset files",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "WorkerBundle",
				"mainModule": "index.js",
				"modules":    workerBundleUnitManifest()["modules"],
				"files":      []any{assetFileEntry("<!doctype html>\n")},
			},
		},
		{
			name: "static asset bundle carrying modules",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "StaticAssetBundle",
				"files":      []any{assetFileEntry("<!doctype html>\n")},
				"modules":    workerBundleUnitManifest()["modules"],
			},
		},
		{
			name: "migration bundle naming a main module",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "MigrationBundle",
				"files":      []any{assetFileEntry("-- migration\n")},
				"mainModule": "index.js",
			},
		},
		{
			name: "worker bundle whose main module is not declared",
			manifest: map[string]any{
				"apiVersion": artifactAPIVersion,
				"kind":       "WorkerBundle",
				"mainModule": "absent.js",
				"modules":    workerBundleUnitManifest()["modules"],
			},
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			host, contract := fallbackHost(t)
			server := httptest.NewServer(host)
			defer server.Close()
			digest := stageUpload(t, host, "up_1", testCase.manifest, map[string]string{
				formpackage.DigestBytes([]byte(artifactModuleSource)): artifactModuleSource,
				formpackage.DigestBytes([]byte("<!doctype html>\n")):  "<!doctype html>\n",
				formpackage.DigestBytes([]byte("-- migration\n")):     "-- migration\n",
			})
			status, body := hostRequest(t, server, http.MethodPost,
				contract.APIPath+"/artifacts/uploads/up_1/commit",
				map[string]string{"Idempotency-Key": "key-commit"}, nil)
			if status != http.StatusBadRequest || !strings.Contains(string(body), "artifact_invalid") {
				t.Fatalf("commit = %d %s, want 400 artifact_invalid", status, strings.TrimSpace(string(body)))
			}
			readStatus, readBody := hostRequest(t, server, http.MethodGet,
				contract.APIPath+"/artifacts/"+url.PathEscape(digest), nil, nil)
			if readStatus != http.StatusNotFound || !strings.Contains(string(readBody), "artifact_missing") {
				t.Fatalf("rejected manifest became readable: %d %s", readStatus, strings.TrimSpace(string(readBody)))
			}
		})
	}
}

// TestBundleDesiredStateResolvesItsManifestBeforeMutation drives the exact
// pre-mutation gate both apply and import run: a WorkerBundle's desired state
// is one digest, so the host has to resolve the manifest it names and hold it
// to the artifact contract before anything is stored.
func TestBundleDesiredStateResolvesItsManifestBeforeMutation(t *testing.T) {
	host, contract := fallbackHost(t)
	bundleRef := contract.RunnerInput.WorkerBundle.Identity.FormRef
	form := host.catalog.form(bundleRef.APIVersion, bundleRef.Kind)
	if form == nil {
		t.Fatal("the WorkerBundle form is not installed")
	}

	committed, raw := canonicalManifestDigest(t, workerBundleUnitManifest())
	host.manifests[committed] = raw
	host.blobs[formpackage.DigestBytes([]byte(artifactModuleSource))] = []byte(artifactModuleSource)

	assetBody := "<!doctype html>\n"
	assetManifest := map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "StaticAssetBundle",
		"files":      []any{assetFileEntry(assetBody)},
	}
	assetDigest, assetRaw := canonicalManifestDigest(t, assetManifest)
	host.manifests[assetDigest] = assetRaw

	tampered, tamperedRaw := canonicalManifestDigest(t, map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "index.js",
		"modules":    workerBundleUnitManifest()["modules"],
		"files":      []any{assetFileEntry(assetBody)},
	})
	host.manifests[tampered] = tamperedRaw

	cases := []struct {
		name string
		spec map[string]any
		code string
	}{
		{"committed WorkerBundle manifest", map[string]any{"manifestDigest": committed}, ""},
		{
			"uncommitted manifest",
			map[string]any{"manifestDigest": formpackage.DigestBytes([]byte("never-committed"))},
			"artifact_missing",
		},
		{"manifest of another kind", map[string]any{"manifestDigest": assetDigest}, "artifact_invalid"},
		{"manifest carrying files", map[string]any{"manifestDigest": tampered}, "artifact_invalid"},
		{"digest grammar violation", map[string]any{"manifestDigest": "sha256:nope"}, "artifact_invalid"},
		{"absent digest", map[string]any{}, "artifact_invalid"},
	}
	for _, testCase := range cases {
		hostErr := host.validateDesiredSemantics(form, contract.RunnerInput.Space, testCase.spec)
		switch {
		case testCase.code == "" && hostErr != nil:
			t.Errorf("%s: hostErr = %+v, want acceptance", testCase.name, hostErr)
		case testCase.code != "" && (hostErr == nil || hostErr.Code != testCase.code):
			t.Errorf("%s: hostErr = %+v, want %s", testCase.name, hostErr, testCase.code)
		}
	}
	if host.manifests[tampered] == nil {
		t.Fatal("the gate deleted a stored manifest; resolution is read-only")
	}
}

// TestCommittedManifestSurvivesUnrelatedAbandonedUpload proves the retention
// rule a bundle resource depends on. A resource's desired state is a manifest
// digest, so a committed manifest and its blobs stay resolvable while any
// resource references them; an unrelated abandoned upload session frees only
// its own staged bytes.
func TestCommittedManifestSurvivesUnrelatedAbandonedUpload(t *testing.T) {
	host, contract := fallbackHost(t)
	server := httptest.NewServer(host)
	defer server.Close()

	referencedManifest := workerBundleUnitManifest()
	referencedDigest, referencedRaw := canonicalManifestDigest(t, referencedManifest)
	referencedBlob := formpackage.DigestBytes([]byte(artifactModuleSource))
	host.manifests[referencedDigest] = referencedRaw
	host.blobs[referencedBlob] = []byte(artifactModuleSource)

	bundleRef := contract.RunnerInput.WorkerBundle.Identity.FormRef
	host.storeResource(&storedResource{
		Group: bundleRef.APIVersion, Kind: bundleRef.Kind,
		Name: "worker-bundle-probe", Space: contract.RunnerInput.Space,
		UID: "uid-1", Generation: 1, Revision: 1,
		Spec: map[string]any{"manifestDigest": referencedDigest},
	})

	// An unrelated upload session stages fresh bytes and is then abandoned.
	const unrelatedSource = "export default { async fetch() { return new Response(\"other\"); } };\n"
	unrelatedBlob := formpackage.DigestBytes([]byte(unrelatedSource))
	stageUpload(t, host, "up_9", map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "other.js",
		"modules": []any{map[string]any{
			"name":      "other.js",
			"mediaType": "application/javascript+module",
			"size":      len(unrelatedSource),
			"digest":    unrelatedBlob,
		}},
	}, map[string]string{unrelatedBlob: unrelatedSource})

	status, body := hostRequest(t, server, http.MethodDelete,
		contract.APIPath+"/artifacts/uploads/up_9",
		map[string]string{"Idempotency-Key": "key-abandon"}, nil)
	if status != http.StatusNoContent {
		t.Fatalf("abandon = %d %s, want 204", status, strings.TrimSpace(string(body)))
	}

	manifestStatus, manifestBody := hostRequest(t, server, http.MethodGet,
		contract.APIPath+"/artifacts/"+url.PathEscape(referencedDigest), nil, nil)
	if manifestStatus != http.StatusOK {
		t.Fatalf("referenced manifest read = %d %s, want 200", manifestStatus, strings.TrimSpace(string(manifestBody)))
	}
	roundTrip, err := formpackage.DigestCanonicalJSON(manifestBody)
	if err != nil || roundTrip != referencedDigest {
		t.Fatalf("referenced manifest is no longer content-addressed by %s", referencedDigest)
	}
	keptStatus, _ := hostRequest(t, server, http.MethodHead,
		contract.APIPath+"/artifacts/blobs/"+url.PathEscape(referencedBlob), nil, nil)
	if keptStatus != http.StatusOK {
		t.Fatalf("referenced blob HEAD = %d, want 200", keptStatus)
	}
	// The abandoned session's own bytes, which nothing committed, are freed.
	freedStatus, _ := hostRequest(t, server, http.MethodHead,
		contract.APIPath+"/artifacts/blobs/"+url.PathEscape(unrelatedBlob), nil, nil)
	if freedStatus != http.StatusNotFound {
		t.Fatalf("abandoned staged blob HEAD = %d, want 404", freedStatus)
	}

	// And the resource that references the manifest still resolves it.
	bundleForm := host.catalog.form(bundleRef.APIVersion, bundleRef.Kind)
	if hostErr := host.validateDesiredSemantics(
		bundleForm, contract.RunnerInput.Space, map[string]any{"manifestDigest": referencedDigest},
	); hostErr != nil {
		t.Fatalf("the referenced manifest stopped resolving: %+v", hostErr)
	}
}

package currentformregistry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The retained v1alpha2 candidate lane is frozen. Work on the v1alpha3 lane —
// portable defaults, derived lifecycle capabilities, the removal of refresh —
// must not move one byte of it, so the exact identities are written out here
// rather than derived. A change to any of them is either a mistake or a
// deliberate act that belongs in its own reviewed change.
var frozenRetainedRefs = map[string][2]string{
	"EdgeWorker": {
		"sha256:226d21f90c739cd749fe41525d4fc3ddd848938aa270df1fdf10f273d8d4e0f8",
		"sha256:71bf6a9afb10559711fb3553f6c8f0f4d2ac470de37149df8d12e049920f1716",
	},
	"RelationalDatabase": {
		"sha256:75e4f1fc7cd894df2ffaebc0655a48d9378150fe8700d3460ec2ecc70e6a26a7",
		"sha256:256b6cb3769fa1a4aaccf4a0e531458ca157869273f681b22f0a1e318a4cfa6f",
	},
	"ObjectBucket": {
		"sha256:86ae3a9dfe78ff0c085dc5640adb36213a58cea1a7c18f43dae319835d554f3a",
		"sha256:9c362a4f38706e1d4e3086b24247bca0555643c71228c304db375702c33de6fe",
	},
	"KeyValueStore": {
		"sha256:f0ccd47c4f6259b6d2f88503e0eacec2aee4a99f3b96164634ee761fe6f93923",
		"sha256:37e54706c0bf6c1298067ed36d8a36ba89e491eff074bacf9851abe59ea9b178",
	},
	"Queue": {
		"sha256:b37964c96b7a0efba07cd30c2cd0f51a5ccfdf57e60de53eb5285d8a6c8d8211",
		"sha256:73fc1cc40cf1e3b81fb3fa2246c96e6170a462c7379261e00ce2ff3852cb9882",
	},
	"Schedule": {
		"sha256:4d99cf7dcc944a91f796c3b028bfd8a48337b0f91de2b1b84b75d647d171ffc3",
		"sha256:4b8a549406cc2b0c009fbc5ab3bfbbb94977c836e7cf3182a1ca8faabd443e53",
	},
	"ContainerService": {
		"sha256:ecef25ff1d48e5a9d5d8f0368834477c8d85984b4b3b9bb874c5f0cddd28a8ca",
		"sha256:eae62434179ba2659a11c1099ddbe727356b9a6d66733b8bedbdc80b660f3bc4",
	},
	"StatefulEntity": {
		"sha256:d620a11017105ab9caa26a5d61d7b87547e8ef0a71310c96441f22ab35c2930f",
		"sha256:576db2dc20a0f791ea027b2b5b07d64846a75d39929510f0f89fd4e0befa3ba2",
	},
	"VectorIndex": {
		"sha256:292df10a0aa0ccb6b8de6727d4772b0aaae282a73b51912d7da6bdccd9179183",
		"sha256:2ef7e04d40cc1b4dc96c336ba9e6fb4d45293dc7273532438cab43dac869cdd7",
	},
}

func TestRetainedV1Alpha2CandidateIdentitiesAreFrozen(t *testing.T) {
	t.Parallel()
	got := All()
	if len(got) != len(frozenRetainedRefs) {
		t.Fatalf("retained lane holds %d candidates, want %d", len(got), len(frozenRetainedRefs))
	}
	for kind, want := range frozenRetainedRefs {
		candidate, present := got[kind]
		if !present {
			t.Errorf("retained candidate %s disappeared", kind)
			continue
		}
		if candidate.SchemaDigest != want[0] {
			t.Errorf("retained %s schemaDigest = %s, want the frozen %s", kind, candidate.SchemaDigest, want[0])
		}
		if candidate.PackageDigest != want[1] {
			t.Errorf("retained %s packageDigest = %s, want the frozen %s", kind, candidate.PackageDigest, want[1])
		}
	}
}

// TestPublishedSchemaBytesAreFrozen re-derives the digest of every published
// schema document from its bytes and holds it to the identity the repository
// publishes. Publication is byte-frozen: a schema whose bytes moved would break
// every consumer that pinned the identity, so this fails on the source file
// rather than at publication time.
func TestPublishedSchemaBytesAreFrozen(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "release", "public-schema-identities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Kind       string `json:"kind"`
		Identities []struct {
			ID     string `json:"id"`
			SHA256 string `json:"sha256"`
			Source string `json:"source"`
			Public string `json:"public"`
		} `json:"identities"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Identities) == 0 {
		t.Fatal("the published schema identity manifest is empty")
	}
	sources := make([]string, 0, len(manifest.Identities))
	for _, identity := range manifest.Identities {
		bytesOnDisk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(identity.Source)))
		if err != nil {
			t.Errorf("%s: %v", identity.Source, err)
			continue
		}
		digest := sha256.Sum256(bytesOnDisk)
		if got := "sha256:" + hex.EncodeToString(digest[:]); got != identity.SHA256 {
			t.Errorf("%s bytes digest %s, published identity %s", identity.Source, got, identity.SHA256)
		}
		sources = append(sources, identity.Source)
	}
	sort.Strings(sources)
	// The v1alpha3 Form Definition schema is the one whose permissiveness this
	// lane relies on (it accepts a `default` inside desiredSchema and an
	// update-less capability list). Its presence here is what makes "we changed
	// no published schema" a checked statement rather than a claim.
	if !containsSource(sources, "spec/schemas/form-definition-v1alpha3.schema.json") {
		t.Fatal("the v1alpha3 Form Definition schema is not a published identity")
	}
}

// TestEmbeddedSchemasMirrorThePublishedBytes closes the other half of the
// freeze: formpackage embeds its own copy of the schema documents, and a
// verifier compiled against different bytes than the ones the world can fetch
// would accept documents the publication rejects (or the reverse).
func TestEmbeddedSchemasMirrorThePublishedBytes(t *testing.T) {
	t.Parallel()
	root := repositoryRoot(t)
	embedded, err := filepath.Glob(filepath.Join(root, "formpackage", "schemas", "*.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(embedded) == 0 {
		t.Fatal("formpackage embeds no schema document")
	}
	for _, path := range embedded {
		name := filepath.Base(path)
		embeddedBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		publishedBytes, err := os.ReadFile(filepath.Join(root, "spec", "schemas", name))
		if err != nil {
			t.Errorf("embedded schema %s has no published counterpart: %v", name, err)
			continue
		}
		embeddedDigest := sha256.Sum256(embeddedBytes)
		publishedDigest := sha256.Sum256(publishedBytes)
		if embeddedDigest != publishedDigest {
			t.Errorf(
				"embedded formpackage/schemas/%s digest %s differs from published spec/schemas/%s digest %s",
				name, hex.EncodeToString(embeddedDigest[:]), name, hex.EncodeToString(publishedDigest[:]),
			)
		}
	}
}

func containsSource(sources []string, want string) bool {
	for _, source := range sources {
		if source == want {
			return true
		}
	}
	return false
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

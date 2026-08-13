package formcatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestRetainedProvider102CodecsAreExactClosedShapes(t *testing.T) {
	tests := []struct {
		kind      string
		version   string
		forbidden []string
		required  []string
	}{
		{
			kind: "RelationalDatabase", version: "2.0.0",
			forbidden: []string{"schemaUrl", "schemaSha256", "schemaFormat"},
			required:  []string{"engine", "engineVersion", "storageGib", "sizeClass", "databaseName", "highAvailability"},
		},
		{
			kind: "EdgeWorker", version: "3.0.0",
			forbidden: []string{"assetsPath", "assetsNotFoundHandling"},
			required:  []string{"entrypoint", "runtime", "runtimeVersion", "requestTimeoutSeconds", "concurrency", "configuration"},
		},
	}

	for _, test := range tests {
		t.Run(test.kind+"@"+test.version, func(t *testing.T) {
			codec, ok := ByKindVersion(test.kind, test.version)
			if !ok {
				t.Fatalf("retained codec %s@%s is unavailable", test.kind, test.version)
			}
			if codec.DefinitionVersion != test.version {
				t.Fatalf("codec version = %q, want %q", codec.DefinitionVersion, test.version)
			}
			properties := codec.DesiredSchema()["properties"].(map[string]any)
			for _, field := range test.required {
				if _, ok := properties[field]; !ok {
					t.Errorf("retained codec omits historical field %q", field)
				}
			}
			for _, field := range test.forbidden {
				if _, ok := properties[field]; ok {
					t.Errorf("retained codec silently widened to successor field %q", field)
				}
			}
		})
	}
}

func TestRetainedCodecLookupDoesNotSelectNearestSemver(t *testing.T) {
	for _, version := range []string{"1.999.999", "2.0.1", "3.1.0"} {
		if _, ok := ByKindVersion("RelationalDatabase", version); ok {
			t.Fatalf("unknown RelationalDatabase@%s selected a codec", version)
		}
	}
}

func TestRetainedDefinitionsAreByteExactPublishedArtifacts(t *testing.T) {
	tests := []struct {
		kind, version string
		releasePath   string
		digest        string
	}{
		{
			kind: "EdgeWorker", version: "3.0.0",
			releasePath: filepath.Join("..", "..", "forms", "releases", "k-ivsgozkxn5zgwzls", "3.0.0", "definition.json"),
			digest:      "sha256:c7fb07db10c937fd6ab119b192552ac239cbcad45dcc12bccd7993decffd2781",
		},
		{
			kind: "RelationalDatabase", version: "2.0.0",
			releasePath: filepath.Join("..", "..", "forms", "releases", "k-kjswyylunfxw4ylmirqxiylcmfzwk", "2.0.0", "definition.json"),
			digest:      "sha256:3898f8ee507bcebd9e03e80fbc1931b67b477299b1ebe2ff395facb7acf018de",
		},
	}

	for _, test := range tests {
		t.Run(test.kind+"@"+test.version, func(t *testing.T) {
			published, err := os.ReadFile(test.releasePath)
			if err != nil {
				t.Fatal(err)
			}
			embedded, ok := retainedDefinitionBytes(test.kind, test.version)
			if !ok {
				t.Fatalf("retained Definition %s@%s is unavailable", test.kind, test.version)
			}
			if !bytes.Equal(embedded, published) {
				t.Fatalf("retained Definition %s@%s is not byte-exact published content", test.kind, test.version)
			}
			digest, err := formpackage.DigestCanonicalJSON(embedded)
			if err != nil {
				t.Fatal(err)
			}
			if digest != test.digest {
				t.Fatalf("retained Definition digest = %q, want %q", digest, test.digest)
			}
		})
	}
}

func TestRetainedEdge3UsesHistoricalArtifactURLPattern(t *testing.T) {
	edge3, ok := ByKindVersion("EdgeWorker", "3.0.0")
	if !ok {
		t.Fatal("retained EdgeWorker@3.0.0 codec is unavailable")
	}
	edge4, ok := ByKindVersion("EdgeWorker", "4.0.0")
	if !ok {
		t.Fatal("current EdgeWorker@4.0.0 codec is unavailable")
	}
	artifactPattern := func(kind Kind) string {
		defs := kind.DesiredSchema()["$defs"].(map[string]any)
		artifact := defs["artifactSource"].(map[string]any)
		properties := artifact["properties"].(map[string]any)
		return properties["artifactUrl"].(map[string]any)["pattern"].(string)
	}
	if got := artifactPattern(edge3); got != PatternRetainedCredentialFreeHTTPSURL {
		t.Fatalf("Edge3 artifact URL pattern = %q, want byte-exact historical pattern %q", got, PatternRetainedCredentialFreeHTTPSURL)
	}
	if artifactPattern(edge3) == artifactPattern(edge4) {
		t.Fatal("Edge3 codec was reconstructed from Edge4 instead of loading its historical Definition")
	}
}

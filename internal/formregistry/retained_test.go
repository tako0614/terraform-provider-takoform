package formregistry

import "testing"

func TestV104RetainsExactProvider102YuruFormReferences(t *testing.T) {
	tests := []struct {
		kind    string
		version string
		want    Ref
	}{
		{
			kind: "RelationalDatabase", version: "2.0.0",
			want: Ref{
				APIVersion: APIVersion, Kind: "RelationalDatabase", DefinitionVersion: "2.0.0",
				SchemaDigest:  "sha256:3898f8ee507bcebd9e03e80fbc1931b67b477299b1ebe2ff395facb7acf018de",
				PackageDigest: "sha256:dc131e4858ddedbb84d553fdf7808c55fc898a37f15d84839e414fe3ca57c910",
			},
		},
		{
			kind: "EdgeWorker", version: "3.0.0",
			want: Ref{
				APIVersion: APIVersion, Kind: "EdgeWorker", DefinitionVersion: "3.0.0",
				SchemaDigest:  "sha256:c7fb07db10c937fd6ab119b192552ac239cbcad45dcc12bccd7993decffd2781",
				PackageDigest: "sha256:f03ede50c6b04459e669ed7aaef3e63397b127882a6b4b19dad45ea2da232381",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.kind+"@"+test.version, func(t *testing.T) {
			got, err := ForExact(test.want)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("exact retained FormRef = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestExactReferenceLookupDoesNotInferFromSemver(t *testing.T) {
	want, err := ForKindVersion("RelationalDatabase", "2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	changed := want
	changed.SchemaDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := ForExact(changed); err == nil {
		t.Fatal("lookup accepted an unknown digest merely because kind and semver matched")
	}

	changed = want
	changed.PackageDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := ForExact(changed); err == nil {
		t.Fatal("lookup accepted an unknown package digest merely because kind and semver matched")
	}
}

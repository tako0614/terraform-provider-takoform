package spec

import "testing"

func TestPublishedV1MaintenanceLineageIsImmutable(t *testing.T) {
	t.Parallel()
	ledger := readFile[struct {
		Format  string `json:"format"`
		Entries []struct {
			Version            string `json:"version"`
			Tag                string `json:"tag"`
			Status             string `json:"status"`
			TagObject          string `json:"tagObject"`
			Commit             string `json:"commit"`
			SigningFingerprint string `json:"signingFingerprint"`
		} `json:"entries"`
	}](t, "..", "release", "provider-release-identities.json")

	if ledger.Format != "takoform.provider-release-identities@v1" {
		t.Fatalf("provider release ledger format = %q", ledger.Format)
	}
	want := []struct {
		version   string
		tagObject string
		commit    string
	}{
		{"1.0.1", "e824793f019a941be11fde0a908fd8d1ea813ba8", "44e1da0bc7e5b2581e2197ccedb107e5d9a7e9db"},
		{"1.0.2", "5bfeb9a01138b9147f0ee1042513098b1b4ad7f6", "28649255756f670f1ead1d044e38fe86ca794df9"},
		{"1.0.3", "0de588120a371db6d22b2fce07b60c967b7e00e7", "87b29ef58066755012a8d80bce0c8f715cf82cb9"},
	}
	if len(ledger.Entries) != len(want) {
		t.Fatalf("published v1 ledger has %d entries, want %d (v1.0.4 must remain unassigned)", len(ledger.Entries), len(want))
	}
	for i, expected := range want {
		got := ledger.Entries[i]
		if got.Version != expected.version || got.Tag != "v"+expected.version ||
			got.Status != "assigned" || got.TagObject != expected.tagObject || got.Commit != expected.commit ||
			got.SigningFingerprint != "3510E75E05BBCC303B92D77934FC18AC897FB709" {
			t.Errorf("published v1 ledger entry %d = %#v, want version=%q tagObject=%q commit=%q", i, got, expected.version, expected.tagObject, expected.commit)
		}
	}
}

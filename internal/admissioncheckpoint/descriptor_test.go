package admissioncheckpoint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validDescriptor = `{
  "format": "takoform.standard-admission-checkpoint@v1",
  "version": "1.0.7",
  "tag": "forms/admissions/v1.0.7",
  "generation": "ga-core-v2",
  "retainedRoot": "admission/v4"
}
`

const validIdentityLedger = `{
  "format": "takoform.standard-admission-identities@v1",
  "entries": [
    {
      "version": "1.0.1",
      "tag": "forms/admissions/v1.0.1",
      "status": "assigned-historical",
      "tagObject": "2b1ca9f68688392869a79de122fbce2a54842301",
      "commit": "57aba7f374bb0d45274044e1dacbea52d16f3f6b"
    },
    {
      "version": "1.0.2",
      "tag": "forms/admissions/v1.0.2",
      "status": "assigned-historical",
      "tagObject": "98af8dd461f24e6dc902f5c56dc6740f74ceb5af",
      "commit": "ff65142ecfab206f58239f095b5e170854ef9dde"
    },
    {
      "version": "1.0.3",
      "tag": "forms/admissions/v1.0.3",
      "status": "assigned-historical",
      "tagObject": "82af8a61666e0194506d0d23d04422ccda4b3d86",
      "commit": "4a40826c7ed467af84e856487998ce365ffe00dd"
    },
    {
      "version": "1.0.4",
      "tag": "forms/admissions/v1.0.4",
      "status": "assigned-historical",
      "tagObject": "b49a55016362d8787966f41b14570e3b67b8ddba",
      "commit": "a426a379e2743b4345e868becf3618357c015447"
    },
    {
      "version": "1.0.5",
      "tag": "forms/admissions/v1.0.5",
      "status": "reserved-abandoned",
      "retainedPaths": [
        "admission/v3/candidates/host-report-1.0.5-63dabf0c64be-bd0b3184aaad",
        "admission/v3/candidates/provider-report-1.0.5-bd0b3184aaad",
        "admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad"
      ]
    },
    {
      "version": "1.0.6",
      "tag": "forms/admissions/v1.0.6",
      "status": "assigned-historical",
      "tagObject": "b34b13a6e2fd3acbcbd73935e3e353f5d05b5c31",
      "commit": "1e438d61ed77f1ccfd3e000250f7dcf0c578c1af"
    },
    {
      "version": "1.0.7",
      "tag": "forms/admissions/v1.0.7",
      "status": "assigned-current",
      "descriptorPath": "admission/v4/version.json"
    }
  ]
}
`

func TestLoadCurrentDescriptorPinsVersionTagAndGeneration(t *testing.T) {
	t.Parallel()
	root := fixtureRoot(t, validDescriptor, validIdentityLedger)

	descriptor, ledger, err := LoadCurrent(root)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Version != "1.0.7" ||
		descriptor.Tag != "forms/admissions/v1.0.7" ||
		descriptor.Generation != "ga-core-v2" ||
		descriptor.RetainedRoot != "admission/v4" {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
	if len(ledger.Entries) != 7 ||
		ledger.Entries[4].Status != "reserved-abandoned" ||
		ledger.Entries[5].Status != "assigned-historical" ||
		ledger.Entries[6].Status != "assigned-current" {
		t.Fatalf("unexpected identity ledger: %#v", ledger)
	}
}

func TestRepositoryCurrentDescriptorReservesAbandonedV105AndAssignsV107(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	descriptor, ledger, err := LoadCurrent(root)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Version != "1.0.7" || descriptor.Tag != "forms/admissions/v1.0.7" {
		t.Fatalf("current identity = %s / %s, want exact v1.0.7", descriptor.Version, descriptor.Tag)
	}
	if ledger.Entries[4].Version != "1.0.5" || ledger.Entries[4].Status != "reserved-abandoned" {
		t.Fatalf("v1.0.5 is not permanently reserved: %#v", ledger.Entries[4])
	}
	if ledger.Entries[5].Version != "1.0.6" || ledger.Entries[5].Status != "assigned-historical" {
		t.Fatalf("v1.0.6 is not historical: %#v", ledger.Entries[5])
	}
}

func TestLoadCurrentDescriptorRejectsDrift(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(string) string{
		"unknown field": func(raw string) string {
			return strings.Replace(raw, `"retainedRoot": "admission/v4"`, `"retainedRoot": "admission/v4", "status": "published"`, 1)
		},
		"prerelease": func(raw string) string {
			return strings.ReplaceAll(raw, "1.0.7", "1.0.7-rc.1")
		},
		"leading zero": func(raw string) string {
			return strings.ReplaceAll(raw, "1.0.7", "1.0.07")
		},
		"tag mismatch": func(raw string) string {
			return strings.Replace(raw, `"tag": "forms/admissions/v1.0.7"`, `"tag": "forms/admissions/v1.0.6"`, 1)
		},
		"generation mismatch": func(raw string) string {
			return strings.Replace(raw, `"generation": "ga-core-v2"`, `"generation": "ga-core-v1"`, 1)
		},
		"root mismatch": func(raw string) string {
			return strings.Replace(raw, `"retainedRoot": "admission/v4"`, `"retainedRoot": "admission/v3"`, 1)
		},
		"duplicate field": func(raw string) string {
			return strings.Replace(raw, `"version": "1.0.7",`, `"version": "1.0.7", "version": "1.0.6",`, 1)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			mutated := mutate(validDescriptor)
			if mutated == validDescriptor {
				t.Fatal("test mutation did not change the descriptor")
			}
			root := fixtureRoot(t, mutated, validIdentityLedger)
			if _, _, err := LoadCurrent(root); err == nil {
				t.Fatal("drifted descriptor unexpectedly accepted")
			}
		})
	}
}

func TestLoadCurrentDescriptorRejectsIdentityLedgerDrift(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(string) string{
		"reuses reserved 1.0.5": func(raw string) string {
			return strings.Replace(raw, `"status": "reserved-abandoned"`, `"status": "assigned-current"`, 1)
		},
		"moves historical assigned tag": func(raw string) string {
			return strings.Replace(raw, `"commit": "57aba7f374bb0d45274044e1dacbea52d16f3f6b"`, `"commit": "ffffffffffffffffffffffffffffffffffffffff"`, 1)
		},
		"current descriptor mismatch": func(raw string) string {
			return strings.Replace(raw, `"version": "1.0.7"`, `"version": "1.0.6"`, 1)
		},
		"missing abandoned evidence path": func(raw string) string {
			return strings.Replace(raw, `"admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad"`, `"admission/v3/candidates/missing"`, 1)
		},
		"unknown field": func(raw string) string {
			return strings.Replace(raw, `"format": "takoform.standard-admission-identities@v1",`, `"format": "takoform.standard-admission-identities@v1", "mutable": true,`, 1)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := fixtureRoot(t, validDescriptor, mutate(validIdentityLedger))
			if _, _, err := LoadCurrent(root); err == nil {
				t.Fatal("drifted identity ledger unexpectedly accepted")
			}
		})
	}
}

func fixtureRoot(t *testing.T, descriptor, ledger string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "admission", "v4", "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, retained := range []string{
		"admission/v3/candidates/host-report-1.0.5-63dabf0c64be-bd0b3184aaad",
		"admission/v3/candidates/provider-report-1.0.5-bd0b3184aaad",
		"admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(retained)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(CurrentDescriptorPath)), []byte(descriptor), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(CurrentIdentityLedgerPath)), []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

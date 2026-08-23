package currentformregistry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEmbeddedV3RefsMatchGeneratedFamilyCandidateSet(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "forms", "candidates", "edge.forms.takoform.com", "candidate-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Family string `json:"family"`
		Forms  []struct {
			Kind          string `json:"kind"`
			Role          string `json:"role"`
			FormRef       V3Ref  `json:"formRef"`
			PackageDigest string `json:"packageDigest"`
		} `json:"forms"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Family != "edge.forms.takoform.com" {
		t.Fatalf("family = %q", manifest.Family)
	}
	registry := V3Current()
	// supported spans generations (released refs stay for state
	// compatibility); the candidate manifest is one generation and must
	// account for exactly its own supported refs and every create default.
	currentSupported := 0
	for key := range registry.supported {
		if key.APIVersion == manifest.Family {
			currentSupported++
		}
	}
	if len(manifest.Forms) != currentSupported {
		t.Fatalf("candidate manifest has %d Forms, provider supports %d of this generation", len(manifest.Forms), currentSupported)
	}
	if len(manifest.Forms) != len(registry.defaultCreates) {
		t.Fatalf("candidate manifest has %d Forms, provider defaults to %d", len(manifest.Forms), len(registry.defaultCreates))
	}
	for _, entry := range manifest.Forms {
		entry.FormRef.PackageDigest = entry.PackageDigest
		got, ok := registry.Lookup(entry.FormRef.ExactKey())
		if !ok {
			t.Fatalf("provider does not support family candidate %s", entry.FormRef.ExactKey())
		}
		if got != entry.FormRef {
			t.Fatalf("provider family candidate %s drifted: got %#v want %#v", entry.Kind, got, entry.FormRef)
		}
		byKind, err := V3ForKind(entry.FormRef.APIVersion, entry.Kind)
		if err != nil {
			t.Fatal(err)
		}
		if byKind != entry.FormRef {
			t.Fatalf("V3ForKind(%s) = %#v", entry.Kind, byKind)
		}
	}
}

func TestV3SupportedFormRefsCoversEveryDefault(t *testing.T) {
	t.Parallel()
	registry := V3Current()
	supported := registry.SupportedRefs()
	// The supported set spans generations: every ref a RELEASED provider
	// embedded stays supported forever (the state-compatibility fence), and
	// the current generation adds its own. Exactly the current generation's
	// refs are create defaults.
	if len(supported) < len(registry.defaultCreates) {
		t.Fatalf("supported family FormRefs = %d, fewer than the %d defaults", len(supported), len(registry.defaultCreates))
	}
	currentFamily := "edge.forms.takoform.com"
	currentCount := 0
	for _, ref := range supported {
		if ref.APIVersion == currentFamily {
			currentCount++
		}
	}
	if currentCount != len(registry.defaultCreates) {
		t.Fatalf("current-generation supported refs = %d, want the %d defaults", currentCount, len(registry.defaultCreates))
	}
	for groupKind := range registry.defaultCreates {
		want, err := registry.DefaultCreate(groupKind)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ref := range supported {
			if ref == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("family candidate %s/%s is missing from the supported set", groupKind.APIVersion, groupKind.Kind)
		}
	}
	if _, err := V3ForKind("edge.forms.takoform.com", "NoSuchKind"); err == nil {
		t.Fatal("unknown family kind unexpectedly resolved")
	}
}

// TestProviderV211IdentityLedgerIsEmbedded separates provider compatibility
// from Form Package publication. Provider v2.1.1 carries these exact 15 Beta
// identities and digests even while the package artifacts remain unpublished.
func TestProviderV211IdentityLedgerIsEmbedded(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "release", "provider-form-identities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Format   string `json:"format"`
		Releases []struct {
			ProviderVersion    string `json:"providerVersion"`
			PortableAPIVersion string `json:"portableApiVersion"`
			Family             string `json:"family"`
			FormMaturity       string `json:"formMaturity"`
			Forms              []struct {
				ResourceType  string `json:"resourceType"`
				FormRef       V3Ref  `json:"formRef"`
				PackageDigest string `json:"packageDigest"`
			} `json:"forms"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.Format != "takoform.provider-form-identities@v1" || len(ledger.Releases) != 1 {
		t.Fatalf("provider identity ledger = %#v", ledger)
	}
	release := ledger.Releases[0]
	if release.ProviderVersion != "2.1.1" || release.PortableAPIVersion != "forms.takoform.com/v1beta1" ||
		release.Family != "edge.forms.takoform.com/v1beta1" || release.FormMaturity != "experimental" || len(release.Forms) != 15 {
		t.Fatalf("provider v2.1 identity set = %#v", release)
	}
	registry := V3Current()
	for _, entry := range release.Forms {
		entry.FormRef.PackageDigest = entry.PackageDigest
		got, ok := registry.Lookup(entry.FormRef.ExactKey())
		if !ok || got != entry.FormRef {
			t.Errorf("%s does not embed %s: got %#v ok=%t", entry.ResourceType, entry.FormRef.ExactKey(), got, ok)
		}
	}
}

// TestFutureStableDefaultDoesNotRebindBetaState pins the GA transition rule:
// adding a 1.0 family identity and making it a create default never removes or
// rewrites the Beta identity that existing state records.
func TestFutureStableDefaultDoesNotRebindBetaState(t *testing.T) {
	t.Parallel()
	beta, err := V3ForKind("edge.forms.takoform.com", "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	stable := V3Ref{
		APIVersion:        "edge.forms.takoform.com/v1",
		Kind:              beta.Kind,
		DefinitionVersion: "1.0.0",
		SchemaDigest:      "sha256:4444444444444444444444444444444444444444444444444444444444444444",
		PackageDigest:     "sha256:5555555555555555555555555555555555555555555555555555555555555555",
	}
	future, err := V3Current().Register(stable, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := future.DefaultCreate(stable.ExactKey().GroupKind()); err != nil || got != stable {
		t.Fatalf("future stable create default = %#v (err %v)", got, err)
	}
	if got, ok := future.Lookup(beta.ExactKey()); !ok || got != beta {
		t.Fatalf("future stable registration rebound Beta state: got %#v ok=%t", got, ok)
	}
}

// TestV3RegistryHoldsTwoDefinitionVersionsOfOneForm is the property the whole
// exact-key shape exists for. The generated data has one definition version per
// group+kind, so the only way to prove the structure can carry a Form line that
// ADVANCED is to register a synthetic second definition version and show that
// both identities resolve, independently, while the create default keeps
// naming exactly one of them.
func TestV3RegistryHoldsTwoDefinitionVersionsOfOneForm(t *testing.T) {
	t.Parallel()
	const group = "edge.forms.takoform.com"
	groupKind := GroupKind{APIVersion: group, Kind: "ModuleWorker"}
	base := V3Current()
	first, err := base.DefaultCreate(groupKind)
	if err != nil {
		t.Fatal(err)
	}
	second := V3Ref{
		APIVersion:        group,
		Kind:              "ModuleWorker",
		DefinitionVersion: "0.2.0",
		SchemaDigest:      "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		PackageDigest:     "sha256:2222222222222222222222222222222222222222222222222222222222222222",
	}

	// Registering without promoting keeps the create default where it was: an
	// added identity is a READ capability, not a migration of new resources.
	added, err := base.Register(second, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := added.DefaultCreate(groupKind); err != nil || got != first {
		t.Fatalf("registering a second definition version moved the create default to %#v (err %v)", got, err)
	}
	for _, want := range []V3Ref{first, second} {
		got, ok := added.Lookup(want.ExactKey())
		if !ok || got != want {
			t.Fatalf("exact lookup of %s = %#v ok=%t", want.ExactKey(), got, ok)
		}
	}
	line := added.SupportedRefsFor(groupKind)
	if len(line) != 2 || line[0] != first || line[1] != second {
		t.Fatalf("group+kind now holds %d refs: %#v", len(line), line)
	}

	// Promoting the newer identity moves ONLY the create default; the older
	// identity stays readable, which is what keeps existing state addressable.
	promoted, err := added.Register(second, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := promoted.DefaultCreate(groupKind); err != nil || got != second {
		t.Fatalf("promoted create default = %#v (err %v), want the second definition version", got, err)
	}
	if got, ok := promoted.Lookup(first.ExactKey()); !ok || got != first {
		t.Fatal("promoting a newer definition version made the older one unreadable")
	}

	// The build's own registry is untouched by either copy.
	if got, err := base.DefaultCreate(groupKind); err != nil || got != first {
		t.Fatalf("the build registry was mutated at a distance: %#v (err %v)", got, err)
	}
	if _, ok := base.Lookup(second.ExactKey()); ok {
		t.Fatal("registering into a copy leaked into the build registry")
	}
	if len(V3SupportedFormRefs()) != len(base.SupportedRefs()) {
		t.Fatal("the package-level supported set drifted from the build registry")
	}
}

// TestV3RegistryRefusesConflictingProvenance proves one exact contract identity
// cannot carry two package digests: `form_package_digest` would then depend on
// which registration happened to win.
func TestV3RegistryRefusesConflictingProvenance(t *testing.T) {
	t.Parallel()
	base := V3Current()
	existing, err := V3ForKind("edge.forms.takoform.com", "ModuleWorker")
	if err != nil {
		t.Fatal(err)
	}
	conflicting := existing
	conflicting.PackageDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
	if _, err := base.Register(conflicting, false); err == nil {
		t.Fatal("a second package digest for one exact identity was accepted")
	}
	// Re-registering the identical ref is a no-op, not a conflict.
	if _, err := base.Register(existing, false); err != nil {
		t.Fatalf("re-registering an identical ref failed: %v", err)
	}
	if _, err := base.Register(V3Ref{Kind: "ModuleWorker"}, false); err == nil {
		t.Fatal("an incomplete FormRef was registered")
	}
}

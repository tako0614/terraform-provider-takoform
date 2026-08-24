package currentformregistry

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type generatedFamilyCandidate struct {
	Kind          string `json:"kind"`
	Role          string `json:"role"`
	FormRef       V3Ref  `json:"formRef"`
	PackageDigest string `json:"packageDigest"`
}

func loadGeneratedCurrentFamilyCandidates(t *testing.T) ([]generatedFamilyCandidate, map[string]struct{}) {
	t.Helper()
	repositoryRoot := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, "forms", "candidates", "current-family-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var exact map[string]json.RawMessage
	if err := json.Unmarshal(raw, &exact); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"format", "families", "interfaceCandidateSet", "bindingCandidateSet"} {
		if _, present := exact[key]; !present {
			t.Fatalf("current family index is missing %q", key)
		}
	}
	if len(exact) != 4 {
		t.Fatalf("current family index top-level keys = %v, want the fixed v1 shape", exact)
	}
	var exactFamilies []map[string]json.RawMessage
	if err := json.Unmarshal(exact["families"], &exactFamilies); err != nil {
		t.Fatal(err)
	}
	for index, entry := range exactFamilies {
		for _, key := range []string{"group", "candidateSet", "sha256", "formCount"} {
			if _, present := entry[key]; !present {
				t.Fatalf("current family index families[%d] is missing %q", index, key)
			}
		}
		if len(entry) != 4 {
			t.Fatalf("current family index families[%d] keys = %v, want the fixed v1 shape", index, entry)
		}
	}
	for _, key := range []string{"interfaceCandidateSet", "bindingCandidateSet"} {
		var reference map[string]json.RawMessage
		if err := json.Unmarshal(exact[key], &reference); err != nil {
			t.Fatal(err)
		}
		if len(reference) != 2 || reference["path"] == nil || reference["sha256"] == nil {
			t.Fatalf("current family index %s keys = %v, want exactly path and sha256", key, reference)
		}
	}
	var index struct {
		Format   string `json:"format"`
		Families []struct {
			Group        string `json:"group"`
			CandidateSet string `json:"candidateSet"`
			SHA256       string `json:"sha256"`
			FormCount    int    `json:"formCount"`
		} `json:"families"`
	}
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if index.Format != "takoform.current-family-index@v1" || len(index.Families) != 8 {
		t.Fatalf("current family index = format %q, families %d", index.Format, len(index.Families))
	}
	var candidates []generatedFamilyCandidate
	groups := make(map[string]struct{}, len(index.Families))
	priorGroup := ""
	for _, family := range index.Families {
		if family.Group <= priorGroup {
			t.Fatalf("current family index is not strictly ordered by group: %q after %q", family.Group, priorGroup)
		}
		priorGroup = family.Group
		wantPath := filepath.ToSlash(filepath.Join("forms", "candidates", family.Group, "candidate-set.json"))
		if family.CandidateSet != wantPath {
			t.Fatalf("%s candidate set path = %q, want %q", family.Group, family.CandidateSet, wantPath)
		}
		candidateRaw, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(family.CandidateSet)))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(candidateRaw)); got != family.SHA256 {
			t.Fatalf("%s candidate set digest = %q, want %q", family.Group, got, family.SHA256)
		}
		var manifest struct {
			Family string                     `json:"family"`
			Forms  []generatedFamilyCandidate `json:"forms"`
		}
		if err := json.Unmarshal(candidateRaw, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Family != family.Group || len(manifest.Forms) != family.FormCount {
			t.Fatalf("%s candidate set = family %q, Forms %d, want %d", family.Group, manifest.Family, len(manifest.Forms), family.FormCount)
		}
		groups[family.Group] = struct{}{}
		candidates = append(candidates, manifest.Forms...)
	}
	return candidates, groups
}

func TestEmbeddedV3RefsMatchGeneratedFamilyCandidateSet(t *testing.T) {
	t.Parallel()
	candidates, groups := loadGeneratedCurrentFamilyCandidates(t)
	registry := V3Current()
	// supported spans generations (released refs stay for state
	// compatibility); the candidate manifest is one generation and must
	// account for exactly its own supported refs and every create default.
	currentSupported := 0
	for key := range registry.supported {
		if _, current := groups[key.APIVersion]; current {
			currentSupported++
		}
	}
	if len(candidates) != currentSupported {
		t.Fatalf("candidate index has %d Forms, registry supports %d current Forms", len(candidates), currentSupported)
	}
	if len(candidates) != len(registry.defaultCreates) {
		t.Fatalf("candidate index has %d Forms, registry defaults to %d", len(candidates), len(registry.defaultCreates))
	}
	for _, entry := range candidates {
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
	_, currentGroups := loadGeneratedCurrentFamilyCandidates(t)
	registry := V3Current()
	supported := registry.SupportedRefs()
	// The supported set spans generations: every ref a RELEASED provider
	// embedded stays supported forever (the state-compatibility fence), and
	// the current generation adds its own. Exactly the current generation's
	// refs are create defaults.
	if len(supported) < len(registry.defaultCreates) {
		t.Fatalf("supported family FormRefs = %d, fewer than the %d defaults", len(supported), len(registry.defaultCreates))
	}
	currentCount := 0
	for _, ref := range supported {
		if _, current := currentGroups[ref.APIVersion]; current {
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
	if ledger.Format != "takoform.provider-form-identities@v1" || len(ledger.Releases) < 1 {
		t.Fatalf("provider identity ledger = %#v", ledger)
	}
	var release *struct {
		ProviderVersion    string `json:"providerVersion"`
		PortableAPIVersion string `json:"portableApiVersion"`
		Family             string `json:"family"`
		FormMaturity       string `json:"formMaturity"`
		Forms              []struct {
			ResourceType  string `json:"resourceType"`
			FormRef       V3Ref  `json:"formRef"`
			PackageDigest string `json:"packageDigest"`
		} `json:"forms"`
	}
	for index := range ledger.Releases {
		if ledger.Releases[index].ProviderVersion == "2.1.1" {
			release = &ledger.Releases[index]
			break
		}
	}
	if release == nil {
		t.Fatal("provider identity ledger no longer retains 2.1.1")
	}
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

// TestV3RegistrySameKindAcrossGroupsNeverFallsBack proves the exact registry
// and the create-default index are multi-family keys, not kind-only aliases.
// No real family is added: these are injected identities exercising the
// generic registry shape.
func TestV3RegistrySameKindAcrossGroupsNeverFallsBack(t *testing.T) {
	t.Parallel()
	ref := func(group, version, schema, pkg string) V3Ref {
		return V3Ref{
			APIVersion: group, Kind: "Queue", DefinitionVersion: version,
			SchemaDigest: "sha256:" + schema, PackageDigest: "sha256:" + pkg,
		}
	}
	a := ref("a.forms.example", "0.1.0", strings.Repeat("a", 64), strings.Repeat("b", 64))
	b := ref("b.forms.example", "1.0.0", strings.Repeat("c", 64), strings.Repeat("d", 64))
	registry, err := newV3Registry(nil, nil).Register(a, true)
	if err != nil {
		t.Fatal(err)
	}
	registry, err = registry.Register(b, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []V3Ref{a, b} {
		if got, err := registry.DefaultCreate(want.ExactKey().GroupKind()); err != nil || got != want {
			t.Fatalf("default %s = %#v (err %v), want %#v", want.APIVersion, got, err, want)
		}
		if got, ok := registry.Lookup(want.ExactKey()); !ok || got != want {
			t.Fatalf("lookup %s = %#v ok=%t", want.ExactKey(), got, ok)
		}
	}
	if _, err := registry.DefaultCreate(GroupKind{APIVersion: "missing.forms.example", Kind: "Queue"}); err == nil {
		t.Fatal("wrong-group create lookup fell back to another family's Queue")
	}
	for name, key := range map[string]ExactFormKey{
		"wrong group":  {APIVersion: "missing.forms.example", Kind: a.Kind, DefinitionVersion: a.DefinitionVersion, SchemaDigest: a.SchemaDigest},
		"latest alias": {APIVersion: a.APIVersion, Kind: a.Kind, DefinitionVersion: "latest", SchemaDigest: a.SchemaDigest},
		"wrong digest": {APIVersion: a.APIVersion, Kind: a.Kind, DefinitionVersion: a.DefinitionVersion, SchemaDigest: "sha256:" + strings.Repeat("f", 64)},
	} {
		if _, ok := registry.Lookup(key); ok {
			t.Errorf("%s exact lookup unexpectedly fell back: %s", name, key)
		}
	}
}

package portableconformancev3

import (
	"bytes"
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestStableGenericInputsContainNoOfficialFamilyOrCatalogSource(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repositoryRoot, "conformance", "takoform-v1", "manifest.json")
	var manifest StableSuiteManifest
	if _, err := stableReadStrict(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(filepath.ToSlash(manifest.Generic.Path), "generic-host/portable-host") {
		t.Fatalf("active generic manifest references retained legacy corpus %q", manifest.Generic.Path)
	}
	forbidden := [][]byte{
		[]byte("forms/candidates/"),
		[]byte("internal/currentformregistry"),
		[]byte("LoadCatalog("),
		[]byte("SelfTest("),
		[]byte("generic-host/portable-host"),
		[]byte("family-host/edge"),
		[]byte("portableHostContract"),
	}
	for _, family := range manifest.Families {
		forbidden = append(forbidden, []byte(family.Group))
	}
	// These are concrete adapter slot symbols, not protocol vocabulary. The
	// active generic orchestrator may name semantic roles only; the private
	// legacy adapter is tested at runtime to prove its branch stays unselected.
	for _, symbol := range []string{
		"ModuleWorker", "EdgeKvNamespace", "AtLeastOnceQueue", "WorkerVersion",
		"WorkerBundle", "StaticAssetBundle", "SQLiteDatabase", "WorkerEndpoint",
		"MigrationBundle", "application/javascript", "application/sql", "mainModule",
	} {
		forbidden = append(forbidden, []byte(symbol))
	}

	paths := []string{
		filepath.Join(repositoryRoot, "conformance", "takoform-v1", "generic.json"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic_plan.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic_plan_cases.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic_memory_adapter.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic_constraints.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic_constraint_values.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "stable_generic_neutral_helpers.go"),
		filepath.Join(repositoryRoot, "internal", "portableconformancev3", "generic_semantic_roles.go"),
	}
	externalRoot := filepath.Join(repositoryRoot, "conformance", "takoform-v1", "generic-host", "external-family")
	if err := filepath.WalkDir(externalRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if bytes.Contains(raw, token) {
				relative, _ := filepath.Rel(repositoryRoot, path)
				t.Fatalf("stable generic input %s contains forbidden official source/group %q", relative, token)
			}
		}
	}

	// Reject official Form Kind identities structurally. Raw token matching is
	// intentionally not used here: artifact-manifest kinds are protocol values
	// and may share words with a Form Kind without becoming a Form identity.
	officialKinds := map[string]bool{}
	for _, family := range manifest.Families {
		familyPath, err := stableResolve(repositoryRoot, manifestPath, family.Path)
		if err != nil {
			t.Fatal(err)
		}
		var familyCorpus stableFamilyCorpus
		if _, err := stableReadStrict(familyPath, &familyCorpus); err != nil {
			t.Fatal(err)
		}
		candidatePath, err := stableResolve(repositoryRoot, familyPath, familyCorpus.CandidateSet.Path)
		if err != nil {
			t.Fatal(err)
		}
		var candidates stableCandidateSet
		if _, err := stableReadStrict(candidatePath, &candidates); err != nil {
			t.Fatal(err)
		}
		for _, candidate := range candidates.Forms {
			officialKinds[candidate.Kind] = true
		}
	}
	for _, path := range paths {
		if filepath.Base(path) != "definition.json" || !strings.HasPrefix(path, externalRoot+string(filepath.Separator)) {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		definition, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			t.Fatal(err)
		}
		if officialKinds[definition.Kind] {
			t.Fatalf("active generic definition %s reuses official Form Kind %q", path, definition.Kind)
		}
	}

	// Go syntax is checked separately from raw corpus bytes. Concrete wire
	// mappings are allowed only in the explicitly excluded HTTP/family adapter
	// files; the plan and executable memory model may carry neutral commands
	// and data-derived constraint kinds only.
	restrictedIdentifiers := map[string]bool{
		"ModuleWorker": true, "WorkerBundle": true, "StaticAssetBundle": true,
		"MigrationBundle": true, "mainModule": true,
	}
	for _, path := range paths {
		if filepath.Ext(path) != ".go" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.Ident:
				if restrictedIdentifiers[typed.Name] || officialKinds[typed.Name] {
					t.Errorf("active generic source %s names concrete identifier %q", filepath.Base(path), typed.Name)
				}
			case *ast.BasicLit:
				if typed.Kind != token.STRING {
					break
				}
				value, err := strconv.Unquote(typed.Value)
				if err != nil {
					t.Errorf("unquote %s literal: %v", filepath.Base(path), err)
					break
				}
				lowered := strings.ToLower(value)
				if officialKinds[value] || restrictedIdentifiers[value] ||
					strings.Contains(lowered, "application/javascript") ||
					strings.Contains(lowered, "application/sql") {
					t.Errorf("active generic source %s names concrete string %q", filepath.Base(path), value)
				}
			}
			return true
		})
	}

	// The independent state-machine subject must not reach the HTTP/reference
	// implementation through either an import or a same-package identifier.
	// Sharing data-model and Snapshot types is intentional; sharing the Host or
	// runner implementation would turn the second adapter into another wrapper.
	forbiddenMemoryDependencies := map[string]string{}
	forbiddenMemoryValues := map[string]string{}
	for _, ownerName := range []string{
		"reference_host.go", "runner_checks.go", "runner_current_lane_checks.go",
		"structural_constraints.go", "resolved_uid_constraints.go", "exclusive.go",
		"relations.go", "edge_semantics.go", "worker_aggregate.go",
	} {
		ownerPath := filepath.Join(repositoryRoot, "internal", "portableconformancev3", ownerName)
		parsedOwner, err := parser.ParseFile(token.NewFileSet(), ownerPath, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsedOwner.Decls {
			switch typed := declaration.(type) {
			case *ast.FuncDecl:
				if typed.Recv == nil {
					forbiddenMemoryDependencies[typed.Name.Name] = ownerName
				}
			case *ast.GenDecl:
				if typed.Tok != token.CONST && typed.Tok != token.VAR {
					continue
				}
				for _, specification := range typed.Specs {
					value, ok := specification.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, identifier := range value.Names {
						forbiddenMemoryValues[identifier.Name] = ownerName
					}
				}
			}
		}
	}
	for _, name := range []string{
		"stable_generic_memory_adapter.go", "stable_generic_constraints.go", "stable_generic_constraint_values.go",
	} {
		path := filepath.Join(repositoryRoot, "internal", "portableconformancev3", name)
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if value == "net/http" || strings.HasPrefix(value, "net/http/") {
				t.Errorf("independent memory subject %s imports %q", name, value)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.Ident:
				if typed.Name == "ReferenceHost" || typed.Name == "v3Runner" || typed.Name == "ServeHTTP" {
					t.Errorf("independent memory subject %s reaches %s", name, typed.Name)
				}
				if owner := forbiddenMemoryValues[typed.Name]; owner != "" {
					t.Errorf("independent memory subject %s reaches value %s owned by %s", name, typed.Name, owner)
				}
			case *ast.CallExpr:
				callee, ok := typed.Fun.(*ast.Ident)
				if !ok {
					break
				}
				if owner := forbiddenMemoryDependencies[callee.Name]; owner != "" {
					t.Errorf("independent memory subject %s calls helper %s owned by %s", name, callee.Name, owner)
				}
			}
			return true
		})
	}
}

func TestStableGenericRuntimeRejectsOfficialGroups(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	var manifest StableSuiteManifest
	if _, err := stableReadStrict(
		filepath.Join(repositoryRoot, "conformance", "takoform-v1", "manifest.json"),
		&manifest,
	); err != nil {
		t.Fatal(err)
	}
	groups := []string{"forms.takoform.com"}
	for _, family := range manifest.Families {
		groups = append(groups, family.Group, family.Group+"/"+strings.TrimPrefix(stableLane.APIVersion, "forms.takoform.com/"))
	}
	for _, group := range groups {
		if err := stableRequireExternalGroup(group); err == nil || !strings.Contains(err.Error(), "forbidden official family group") {
			t.Fatalf("stable generic runtime accepted official group %q: %v", group, err)
		}
	}
	if err := stableRequireExternalGroup("resources.publisher.example"); err != nil {
		t.Fatalf("stable generic runtime rejected independent external group: %v", err)
	}
}

func TestStableGenericFamilyBranchInstrumentationHasTeeth(t *testing.T) {
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object", "additionalProperties": false, "properties": map[string]any{},
	}
	neutral := &InstalledForm{
		Ref: FormRef{APIVersion: "resources.publisher.example", Kind: "NeutralCounter",
			DefinitionVersion: "0.1.0", SchemaDigest: formpackage.DigestBytes([]byte("neutral"))},
		DesiredSchema: schema,
	}
	concrete := &InstalledForm{
		Ref: FormRef{APIVersion: "family.publisher.example", Kind: "ConcreteCounter",
			DefinitionVersion: "0.1.0", SchemaDigest: formpackage.DigestBytes([]byte("concrete"))},
		DesiredSchema: schema, EnforcedFamily: "family.publisher.example",
	}
	catalog := newCatalog(beta4Lane.APIVersion)
	catalog.Forms[exactFormKey(neutral.Ref)] = neutral
	catalog.Forms[exactFormKey(concrete.Ref)] = concrete
	host := NewReferenceHost(Contract{lane: beta4Lane}, catalog)
	host.materialize(neutral, map[string]any{})
	if _, hostErr := host.validateDesiredSemantics(hostAuthContext{}, neutral, resourceScope{}, "neutral", map[string]any{}); hostErr != nil {
		t.Fatal(hostErr.Message)
	}
	if got := host.FamilyBranchSelections(); got != 0 {
		t.Fatalf("neutral Form selected %d concrete-family branches", got)
	}
	host.materialize(concrete, map[string]any{})
	if _, hostErr := host.validateDesiredSemantics(hostAuthContext{}, concrete, resourceScope{}, "concrete", map[string]any{}); hostErr != nil {
		t.Fatal(hostErr.Message)
	}
	if got := host.FamilyBranchSelections(); got != 2 {
		t.Fatalf("concrete Form selected %d instrumented branches, want materialize plus semantics", got)
	}
	artifact := &InstalledForm{Ref: FormRef{Kind: workerBundleKind}}
	if kind, known := host.installedArtifactManifestKind(artifact); !known || kind != workerBundleKind {
		t.Fatalf("concrete artifact kind fallback = %q/%v", kind, known)
	}
	if got := host.ConcreteKindSelections(); got != 1 {
		t.Fatalf("concrete Form-kind fallback selected %d instrumented branches, want 1", got)
	}
}

func TestStableGenericRuntimeRejectsSymlinkedCatalogEscape(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	temporaryRoot := t.TempDir()
	if err := os.CopyFS(
		filepath.Join(temporaryRoot, "conformance", "takoform-v1"),
		os.DirFS(filepath.Join(repositoryRoot, "conformance", "takoform-v1")),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "go.mod"), []byte("module stable-suite-catalog-escape\n\ngo 1.25.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	escapePath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "generic-host", "catalog-escape")
	var manifest StableSuiteManifest
	manifestPath := filepath.Join(repositoryRoot, "conformance", "takoform-v1", "manifest.json")
	if _, err := stableReadStrict(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Families) == 0 {
		t.Fatal("stable suite has no official family corpus for the escape control")
	}
	familyPath, err := stableResolve(repositoryRoot, manifestPath, manifest.Families[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	var family stableFamilyCorpus
	if _, err := stableReadStrict(familyPath, &family); err != nil {
		t.Fatal(err)
	}
	candidateSetPath, err := stableResolve(repositoryRoot, familyPath, family.CandidateSet.Path)
	if err != nil {
		t.Fatal(err)
	}
	var candidates stableCandidateSet
	if _, err := stableReadStrict(candidateSetPath, &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates.Forms) == 0 {
		t.Fatal("official family candidate set has no package for the escape control")
	}
	officialPackage := filepath.Join(repositoryRoot, filepath.FromSlash(candidates.Forms[0].Path))
	if err := os.Symlink(officialPackage, escapePath); err != nil {
		t.Fatal(err)
	}

	genericPath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "generic.json")
	var generic stableGenericCorpus
	if _, err := stableReadStrict(genericPath, &generic); err != nil {
		t.Fatal(err)
	}
	generic.SnapshotInputs[0].Packages[0].Path = "generic-host/catalog-escape/package-index.json"
	genericRaw := writeStableTestJSON(t, genericPath, generic)

	suitePath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "manifest.json")
	var suite StableSuiteManifest
	if _, err := stableReadStrict(suitePath, &suite); err != nil {
		t.Fatal(err)
	}
	suite.Generic.SHA256 = stableDigest(genericRaw)
	writeStableTestJSON(t, suitePath, suite)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := RunStableSuite(ctx, suitePath); err == nil || !strings.Contains(err.Error(), "official or out-of-corpus catalog source") {
		t.Fatalf("suite accepted a symlink escape to an official package: %v", err)
	}
}

func TestStableSuiteExecutesNeutralGenericHostChecks(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	manifestPath := filepath.Join(repositoryRoot, "conformance", "takoform-v1", "manifest.json")
	var manifest StableSuiteManifest
	if _, err := stableReadStrict(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	genericPath, err := stableResolve(repositoryRoot, manifestPath, manifest.Generic.Path)
	if err != nil {
		t.Fatal(err)
	}
	witnesses, err := stableVerifyGeneric(ctx, repositoryRoot, genericPath, manifest.Generic)
	if err != nil {
		t.Fatal(err)
	}
	report, err := RunStableSuite(ctx, manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(report.Generic.Checks); got != len(stableGenericRequiredChecks) {
		t.Fatalf("stable generic runner executed %d checks, want all directly evidenced neutral mechanisms", got)
	}
	for _, check := range report.Generic.Checks {
		if strings.Contains(strings.ToLower(check.Name), "edge") {
			t.Fatalf("stable generic report contains Edge-specific check %q", check.Name)
		}
	}
	if len(witnesses) != 2 {
		t.Fatalf("stable generic Snapshot witnesses = %d, want zero-family plus external-family", len(witnesses))
	}
	if witnesses[0].Name != "external-family" ||
		len(witnesses[0].FormRefs) != 20 ||
		witnesses[1].Name != "zero-family" ||
		len(witnesses[1].FormRefs) != 0 {
		t.Fatalf("stable generic Snapshot witnesses drifted: %+v", witnesses)
	}
	for _, ref := range witnesses[0].FormRefs {
		if err := stableRequireExternalGroup(ref.APIVersion); err != nil {
			t.Fatalf("stable generic witness contains an official family: %v", err)
		}
	}
}

func TestStableGenericNeutralCoreTracer(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repositoryRoot, "conformance", "takoform-v1", "manifest.json")
	var manifest StableSuiteManifest
	if _, err := stableReadStrict(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	genericPath, err := stableResolve(repositoryRoot, manifestPath, manifest.Generic.Path)
	if err != nil {
		t.Fatal(err)
	}
	var corpus stableGenericCorpus
	if _, err := stableReadStrict(genericPath, &corpus); err != nil {
		t.Fatal(err)
	}
	compiled, _, err := stableCompileGenericSnapshots(repositoryRoot, genericPath, corpus)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	completed := map[string]bool{}
	if err := stableRunGenericHostChecks(ctx, corpus, compiled, completed); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"apply-idempotency-replay", "create-conflict-when-exists",
		"declared-constraint-semantics-enforced", "fence-matrix-observed",
		"prepare-binds-exact-spec", "unauthenticated-request-refused",
	}
	if len(completed) < len(want) {
		t.Fatalf("neutral core tracer completed %d checks, want at least %d: %v", len(completed), len(want), completed)
	}
	for _, check := range want {
		if !completed[check] {
			t.Fatalf("neutral core tracer did not execute %q", check)
		}
	}
}

func TestStableLegacyPortableCoverageLedgerMapsAll125Checks(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	var ledger stableLegacyCoverageLedger
	if _, err := stableReadStrict(
		filepath.Join(repositoryRoot, filepath.FromSlash(stableLegacyCoverageRepositoryPath)),
		&ledger,
	); err != nil {
		t.Fatal(err)
	}
	want := stableLegacyExpectedChecks()
	if got := len(ledger.Coverage); got != len(want) || got != 125 {
		t.Fatalf("legacy portable coverage entries = %d, want %d", got, len(want))
	}
	neutralRequired := make(map[string]bool, len(stableGenericRequiredChecks))
	for _, check := range stableGenericRequiredChecks {
		if neutralRequired[check] {
			t.Fatalf("stable neutral generic check %q is duplicated", check)
		}
		neutralRequired[check] = true
	}
	genericOwnerCount := 0
	for index, check := range want {
		entry := ledger.Coverage[index]
		if entry.Check != check || entry.EdgeAdapterCheck != check || entry.Owner != stableLegacyCheckOwner(check) {
			t.Fatalf("legacy portable coverage[%d] = %+v, want Edge adapter check %q", index, entry, check)
		}
		if entry.Owner != stableLegacyOwnerGenericHost {
			if len(entry.NeutralChecks) != 0 {
				t.Fatalf("non-generic legacy check %q unexpectedly names neutral evidence %v", check, entry.NeutralChecks)
			}
			continue
		}
		genericOwnerCount++
		if len(entry.NeutralChecks) != 1 || entry.NeutralChecks[0] != check {
			t.Fatalf("generic-owner legacy check %q must map 1:1 to the same active neutral check, got %v", check, entry.NeutralChecks)
		}
		if !neutralRequired[check] {
			t.Fatalf("generic-owner legacy check %q is absent from the active neutral suite", check)
		}
	}
	if genericOwnerCount != len(neutralRequired) || genericOwnerCount == 0 {
		t.Fatalf("neutral generic ownership covers %d legacy entries and %d active checks, want exact non-empty parity", genericOwnerCount, len(neutralRequired))
	}
}

func TestStableEdgeHostRehomeKeepsAll28PublishedFilesByteIdentical(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := stableVerifyLegacyEdgeRehome(repositoryRoot); err != nil {
		t.Fatal(err)
	}
	var ledger stableLegacyRehomeLedger
	raw, err := stableReadStrict(
		filepath.Join(repositoryRoot, filepath.FromSlash(stableLegacyRehomeRepositoryPath)),
		&ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := stableDigest(raw); got != stableLegacyRehomeSHA256 {
		t.Fatalf("rehome ledger digest = %s, want historical hard pin %s", got, stableLegacyRehomeSHA256)
	}
	if ledger.FileCount != 28 || len(ledger.Files) != 28 {
		t.Fatalf("rehome ledger files = %d/%d, want 28/28", ledger.FileCount, len(ledger.Files))
	}
}

func TestStableSuiteRejectsMissingGenericCheck(t *testing.T) {
	repositoryRoot, err := repositoryRootForContract(".")
	if err != nil {
		t.Fatal(err)
	}
	temporaryRoot := t.TempDir()
	if err := os.CopyFS(
		filepath.Join(temporaryRoot, "conformance", "takoform-v1"),
		os.DirFS(filepath.Join(repositoryRoot, "conformance", "takoform-v1")),
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, "go.mod"), []byte("module stable-suite-tamper\n\ngo 1.25.8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(repositoryRoot, "forms"), filepath.Join(temporaryRoot, "forms")); err != nil {
		t.Fatal(err)
	}

	genericPath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "generic.json")
	var generic stableGenericCorpus
	if _, err := stableReadStrict(genericPath, &generic); err != nil {
		t.Fatal(err)
	}
	omitted := generic.RequiredChecks[len(generic.RequiredChecks)-1]
	generic.RequiredChecks = generic.RequiredChecks[:len(generic.RequiredChecks)-1]
	genericRaw := writeStableTestJSON(t, genericPath, generic)

	suitePath := filepath.Join(temporaryRoot, "conformance", "takoform-v1", "manifest.json")
	var suite StableSuiteManifest
	if _, err := stableReadStrict(suitePath, &suite); err != nil {
		t.Fatal(err)
	}
	suite.Generic.SHA256 = stableDigest(genericRaw)
	suite.Generic.RequiredChecks = generic.RequiredChecks
	writeStableTestJSON(t, suitePath, suite)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := RunStableSuite(ctx, suitePath); err == nil || !strings.Contains(err.Error(), "required runner checks drifted") {
		t.Fatalf("suite accepted generic corpus missing %q: %v", omitted, err)
	}
}

func writeStableTestJSON(t *testing.T, path string, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return raw
}

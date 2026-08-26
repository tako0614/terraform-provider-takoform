package provider

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformsnapshot"
)

const providerV3ArtifactClosureFormat = "takoform.provider-v3-artifact-closure@v1"

//go:embed artifacts/v3/**
var providerV3EmbeddedArtifacts embed.FS

type v3ArtifactClosure struct {
	Format     string                       `json:"format"`
	Projection v3ArtifactClosureFile        `json:"projection"`
	Packages   []v3ArtifactClosurePackage   `json:"packages"`
	Interfaces []v3ArtifactClosureInterface `json:"interfaces"`
	Bindings   []v3ArtifactClosureBinding   `json:"bindings"`
}

type v3ArtifactClosureFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type v3ArtifactClosurePackage struct {
	Root          string              `json:"root"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

type v3ArtifactClosureInterface struct {
	Path string                   `json:"path"`
	Ref  formpackage.InterfaceRef `json:"ref"`
}

type v3ArtifactClosureBinding struct {
	Path string                 `json:"path"`
	Ref  formpackage.BindingRef `json:"ref"`
}

type v3ProviderAssembly struct {
	snapshot      *currentformsnapshot.Snapshot
	projection    *v3ProjectionIndex
	currentForms  []model.Form
	registry      v3FormRegistry
	resourceTypes *v3ResourceTypeRegistry
	codecs        *v3CodecTable
}

var cachedEmbeddedProviderV3Assembly = sync.OnceValues(loadEmbeddedProviderV3Assembly)

func providerV3SnapshotAssembly() (*v3ProviderAssembly, error) {
	return cachedEmbeddedProviderV3Assembly()
}

func mustProviderV3SnapshotAssembly() *v3ProviderAssembly {
	assembly, err := providerV3SnapshotAssembly()
	if err != nil {
		panic(err)
	}
	return assembly
}

// providerV3CurrentForms, v3TerraformResourceTypes, and v3Codecs are the
// production dependency seams used by Provider 3 behavior and its immutable
// goldens. They all resolve through the same once-verified embedded assembly;
// the catalog-backed counterparts are named legacy* and remain comparison-only
// until W08 removes them.
func providerV3CurrentForms() []model.Form {
	forms := mustProviderV3SnapshotAssembly().currentForms
	return append([]model.Form(nil), forms...)
}

func v3TerraformResourceTypes() *v3ResourceTypeRegistry {
	return mustProviderV3SnapshotAssembly().resourceTypes
}

func v3Codecs() *v3CodecTable {
	return mustProviderV3SnapshotAssembly().codecs
}

func loadEmbeddedProviderV3Assembly() (*v3ProviderAssembly, error) {
	source, err := fs.Sub(providerV3EmbeddedArtifacts, "artifacts/v3")
	if err != nil {
		return nil, fmt.Errorf("takoform provider: open embedded Provider 3 artifacts: %w", err)
	}
	return loadProviderV3Assembly(source, ".")
}

// loadProviderV3Assembly is the bounded artifact seam used by production and
// fail-closed tests. It reads no repository-relative path: source is the
// immutable embedded closure in production and a fully in-memory fs.FS in
// negative tests.
func loadProviderV3Assembly(source fs.FS, root string) (*v3ProviderAssembly, error) {
	if source == nil || root == "" || !fs.ValidPath(root) {
		return nil, fmt.Errorf("takoform provider: invalid Provider 3 artifact root %q", root)
	}
	artifactFS, err := fs.Sub(source, root)
	if err != nil {
		return nil, fmt.Errorf("takoform provider: open Provider 3 artifact root %q: %w", root, err)
	}
	closureRaw, err := fs.ReadFile(artifactFS, "closure.json")
	if err != nil {
		return nil, fmt.Errorf("takoform provider: read embedded Provider 3 closure: %w", err)
	}
	var closure v3ArtifactClosure
	if err := formpackage.DecodeStrictIJSON(closureRaw, &closure); err != nil {
		return nil, fmt.Errorf("takoform provider: decode embedded Provider 3 closure: %w", err)
	}
	if err := validateV3ArtifactClosure(closure); err != nil {
		return nil, err
	}
	actualFiles, actualDirectories, err := inventoryV3ArtifactFS(artifactFS)
	if err != nil {
		return nil, err
	}
	declaredFiles := map[string]struct{}{"closure.json": {}, closure.Projection.Path: {}}
	projectionRaw, err := fs.ReadFile(artifactFS, closure.Projection.Path)
	if err != nil {
		return nil, fmt.Errorf("takoform provider: read Provider 3 projection %q: %w", closure.Projection.Path, err)
	}
	projectionDigest, err := formpackage.DigestCanonicalJSON(projectionRaw)
	if err != nil {
		return nil, fmt.Errorf("takoform provider: canonicalize Provider 3 projection: %w", err)
	}
	if projectionDigest != closure.Projection.Digest {
		return nil, fmt.Errorf("takoform provider: Provider 3 projection digest is %s, closure pins %s", projectionDigest, closure.Projection.Digest)
	}
	projection, err := decodeProviderV3Projection(projectionRaw)
	if err != nil {
		return nil, err
	}

	input := currentformsnapshot.Input{HostAPI: providerV3HostAPI}
	for _, entry := range closure.Packages {
		report, err := formpackage.VerifyFS(artifactFS, entry.Root)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: verify embedded Form Package %q: %w", entry.Root, err)
		}
		verified, ok := report.VerifiedPackage()
		if !ok {
			return nil, fmt.Errorf("takoform provider: complete verification issued no package capability for %q", entry.Root)
		}
		if verified.FormRef() != entry.FormRef || verified.PackageDigest() != entry.PackageDigest {
			return nil, fmt.Errorf("takoform provider: embedded Form Package %q identity drift: got %#v/%s, closure pins %#v/%s",
				entry.Root, verified.FormRef(), verified.PackageDigest(), entry.FormRef, entry.PackageDigest)
		}
		declaredFiles[path.Join(entry.Root, formpackage.PackageIndexFilename)] = struct{}{}
		for _, packageFile := range verified.PackageIndex().Files {
			declaredFiles[path.Join(entry.Root, packageFile.Path)] = struct{}{}
		}
		input.Packages = append(input.Packages, currentformsnapshot.PackageArtifact{
			Origin: "embedded://provider-v3/" + entry.Root, ExpectedDigest: entry.PackageDigest, Package: verified,
		})
	}
	for _, entry := range closure.Interfaces {
		declaredFiles[entry.Path] = struct{}{}
		raw, err := fs.ReadFile(artifactFS, entry.Path)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: read embedded Interface %q: %w", entry.Path, err)
		}
		input.Interfaces = append(input.Interfaces, currentformsnapshot.InterfaceArtifact{
			Origin: "embedded://provider-v3/" + entry.Path, ExpectedDigest: entry.Ref.SchemaDigest, Definition: raw,
		})
	}
	for _, entry := range closure.Bindings {
		declaredFiles[entry.Path] = struct{}{}
		raw, err := fs.ReadFile(artifactFS, entry.Path)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: read embedded Binding %q: %w", entry.Path, err)
		}
		input.Bindings = append(input.Bindings, currentformsnapshot.BindingArtifact{
			Origin: "embedded://provider-v3/" + entry.Path, ExpectedDigest: entry.Ref.SchemaDigest, Definition: raw,
		})
	}
	if err := validateV3ArtifactInventory(actualFiles, actualDirectories, declaredFiles); err != nil {
		return nil, err
	}
	for _, ref := range projection.document.DefaultCreates {
		input.DefaultCreates = append(input.DefaultCreates, currentformsnapshot.DefaultPin{
			Group: ref.APIVersion, Kind: ref.Kind, Ref: formpackage.FormRef{
				APIVersion: ref.APIVersion, Kind: ref.Kind, DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
			},
		})
	}
	snapshot, diagnostics := currentformsnapshot.Compile(input)
	if snapshot == nil || len(diagnostics) != 0 {
		return nil, fmt.Errorf("takoform provider: compile embedded Provider 3 Snapshot: %s", renderSnapshotDiagnostics(diagnostics))
	}
	if err := validateProjectionAgainstSnapshot(projection, snapshot, closure); err != nil {
		return nil, err
	}

	registry := newV3ProjectedRegistry(projection)
	resourceTypes := &v3ResourceTypeRegistry{
		byRef:         make(map[currentformregistry.ExactFormKey]string, len(projection.resources)),
		artifactByRef: make(map[currentformregistry.ExactFormKey]*v3ArtifactProjection),
	}
	for key, resource := range projection.resources {
		resourceTypes.byRef[key] = resource.ResourceType
		if resource.Artifact != nil {
			resourceTypes.artifactByRef[key] = cloneV3ArtifactProjection(resource.Artifact)
		}
	}
	codecs := &v3CodecTable{registry: registry, codecs: make(map[currentformregistry.ExactFormKey]v3CodecDeclaration, len(projection.readable))}
	for key := range projection.readable {
		entry := projection.forms[key]
		definitionRaw := entry.Definition
		if entry.Generation == v3ProjectionCurrent {
			var found bool
			definitionRaw, found = snapshot.Definition(formpackage.FormRef{
				APIVersion: entry.Ref.APIVersion, Kind: entry.Ref.Kind, DefinitionVersion: entry.Ref.DefinitionVersion, SchemaDigest: entry.Ref.SchemaDigest,
			})
			if !found {
				return nil, fmt.Errorf("takoform provider: current codec Definition %s disappeared after Snapshot validation", key)
			}
		}
		definition, err := formpackage.ValidateDefinition(definitionRaw)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: decode exact codec Definition %s: %w", key, err)
		}
		codecs.codecs[key] = v3CodecDeclaration{Form: entry.Form, DesiredSchema: definition.DesiredSchema}
	}
	currentForms := make([]model.Form, 0, len(projection.currentOrder))
	for _, key := range projection.currentOrder {
		currentForms = append(currentForms, projection.forms[key].Form)
	}
	return &v3ProviderAssembly{
		snapshot: snapshot, projection: projection, currentForms: currentForms,
		registry: registry, resourceTypes: resourceTypes, codecs: codecs,
	}, nil
}

func validateV3ArtifactClosure(closure v3ArtifactClosure) error {
	if closure.Format != providerV3ArtifactClosureFormat {
		return fmt.Errorf("takoform provider: artifact closure format %q, want %q", closure.Format, providerV3ArtifactClosureFormat)
	}
	if err := validateV3ArtifactPath(closure.Projection.Path, "projection"); err != nil {
		return err
	}
	if !formpackage.ValidDigest(closure.Projection.Digest) {
		return fmt.Errorf("takoform provider: projection digest is not canonical")
	}
	occupiedPaths := map[string]string{"closure.json": "closure"}
	if err := claimV3ArtifactPath(occupiedPaths, closure.Projection.Path, "projection"); err != nil {
		return err
	}
	if len(closure.Packages) != 31 || len(closure.Interfaces) != 13 || len(closure.Bindings) != 6 {
		return fmt.Errorf("takoform provider: artifact closure contains %d packages/%d Interfaces/%d Bindings, want 31/13/6",
			len(closure.Packages), len(closure.Interfaces), len(closure.Bindings))
	}
	packageRoots := make(map[string]struct{}, len(closure.Packages))
	packageRefs := make(map[formpackage.FormRef]struct{}, len(closure.Packages))
	for _, entry := range closure.Packages {
		if err := validateV3ArtifactPath(entry.Root, "package root"); err != nil {
			return err
		}
		if _, duplicate := packageRoots[entry.Root]; duplicate {
			return fmt.Errorf("takoform provider: artifact closure repeats package root %q", entry.Root)
		}
		if !strings.HasPrefix(entry.Root, "packages/") {
			return fmt.Errorf("takoform provider: package root %q is outside packages/", entry.Root)
		}
		for prior := range packageRoots {
			if v3ArtifactPathContains(prior, entry.Root) || v3ArtifactPathContains(entry.Root, prior) {
				return fmt.Errorf("takoform provider: package roots %q and %q overlap", prior, entry.Root)
			}
		}
		packageRoots[entry.Root] = struct{}{}
		if _, duplicate := packageRefs[entry.FormRef]; duplicate {
			return fmt.Errorf("takoform provider: artifact closure repeats exact FormRef %#v", entry.FormRef)
		}
		packageRefs[entry.FormRef] = struct{}{}
		if !formpackage.ValidDigest(entry.FormRef.SchemaDigest) || !formpackage.ValidDigest(entry.PackageDigest) {
			return fmt.Errorf("takoform provider: artifact closure package %q has an invalid digest", entry.Root)
		}
	}
	interfaceRefs := make(map[formpackage.InterfaceRef]struct{}, len(closure.Interfaces))
	for _, entry := range closure.Interfaces {
		if err := validateV3ArtifactPath(entry.Path, "Interface path"); err != nil {
			return err
		}
		if _, duplicate := interfaceRefs[entry.Ref]; duplicate {
			return fmt.Errorf("takoform provider: artifact closure repeats Interface %#v", entry.Ref)
		}
		if !strings.HasPrefix(entry.Path, "interfaces/") {
			return fmt.Errorf("takoform provider: Interface path %q is outside interfaces/", entry.Path)
		}
		if err := claimV3ArtifactPath(occupiedPaths, entry.Path, "Interface"); err != nil {
			return err
		}
		interfaceRefs[entry.Ref] = struct{}{}
		if !formpackage.ValidDigest(entry.Ref.SchemaDigest) {
			return fmt.Errorf("takoform provider: artifact closure Interface %q has an invalid digest", entry.Path)
		}
	}
	bindingRefs := make(map[formpackage.BindingRef]struct{}, len(closure.Bindings))
	for _, entry := range closure.Bindings {
		if err := validateV3ArtifactPath(entry.Path, "Binding path"); err != nil {
			return err
		}
		if _, duplicate := bindingRefs[entry.Ref]; duplicate {
			return fmt.Errorf("takoform provider: artifact closure repeats Binding %#v", entry.Ref)
		}
		if !strings.HasPrefix(entry.Path, "bindings/") {
			return fmt.Errorf("takoform provider: Binding path %q is outside bindings/", entry.Path)
		}
		if err := claimV3ArtifactPath(occupiedPaths, entry.Path, "Binding"); err != nil {
			return err
		}
		bindingRefs[entry.Ref] = struct{}{}
		if !formpackage.ValidDigest(entry.Ref.SchemaDigest) {
			return fmt.Errorf("takoform provider: artifact closure Binding %q has an invalid digest", entry.Path)
		}
	}
	for root := range packageRoots {
		for occupied, label := range occupiedPaths {
			if v3ArtifactPathContains(root, occupied) || v3ArtifactPathContains(occupied, root) {
				return fmt.Errorf("takoform provider: package root %q collides with %s path %q", root, label, occupied)
			}
		}
	}
	return nil
}

func claimV3ArtifactPath(occupied map[string]string, artifactPath, label string) error {
	if prior, duplicate := occupied[artifactPath]; duplicate {
		return fmt.Errorf("takoform provider: %s path %q collides with %s", label, artifactPath, prior)
	}
	occupied[artifactPath] = label
	return nil
}

func v3ArtifactPathContains(parent, candidate string) bool {
	return candidate == parent || strings.HasPrefix(candidate, parent+"/")
}

func inventoryV3ArtifactFS(source fs.FS) (map[string]struct{}, map[string]struct{}, error) {
	files := map[string]struct{}{}
	directories := map[string]struct{}{}
	err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if !fs.ValidPath(name) || entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("artifact entry %q is invalid or a symlink", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("artifact entry %q is a symlink", name)
		}
		if info.IsDir() {
			directories[name] = struct{}{}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact entry %q is not a regular file", name)
		}
		files[name] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("takoform provider: inventory embedded Provider 3 closure: %w", err)
	}
	return files, directories, nil
}

func validateV3ArtifactInventory(actualFiles, actualDirectories, declaredFiles map[string]struct{}) error {
	for declared := range declaredFiles {
		if _, ok := actualFiles[declared]; !ok {
			return fmt.Errorf("takoform provider: declared artifact file %q is missing", declared)
		}
	}
	for actual := range actualFiles {
		if _, ok := declaredFiles[actual]; !ok {
			return fmt.Errorf("takoform provider: unreferenced artifact file %q is outside the exact closure", actual)
		}
	}
	expectedDirectories := map[string]struct{}{}
	for file := range declaredFiles {
		for directory := path.Dir(file); directory != "."; directory = path.Dir(directory) {
			expectedDirectories[directory] = struct{}{}
		}
	}
	for actual := range actualDirectories {
		if _, ok := expectedDirectories[actual]; !ok {
			return fmt.Errorf("takoform provider: unreferenced artifact directory %q is outside the exact closure", actual)
		}
	}
	for expected := range expectedDirectories {
		if _, ok := actualDirectories[expected]; !ok {
			return fmt.Errorf("takoform provider: declared artifact directory %q is missing", expected)
		}
	}
	return nil
}

func validateV3ArtifactPath(value, label string) error {
	if value == "" || value == "." || !fs.ValidPath(value) || strings.HasPrefix(value, "/") {
		return fmt.Errorf("takoform provider: %s %q is not a canonical embedded relative path", label, value)
	}
	return nil
}

func validateProjectionAgainstSnapshot(index *v3ProjectionIndex, snapshot *currentformsnapshot.Snapshot, closure v3ArtifactClosure) error {
	snapshotForms := make(map[currentformregistry.ExactFormKey]currentformsnapshot.Form)
	for _, form := range snapshot.Forms() {
		key := currentformregistry.ExactFormKey{
			APIVersion: form.Ref.APIVersion, Kind: form.Ref.Kind, DefinitionVersion: form.Ref.DefinitionVersion, SchemaDigest: form.Ref.SchemaDigest,
		}
		snapshotForms[key] = form
	}
	if len(snapshotForms) != len(index.currentKeys) {
		return fmt.Errorf("takoform provider: Snapshot has %d exact Forms, projection has %d current refs", len(snapshotForms), len(index.currentKeys))
	}
	for key := range index.currentKeys {
		projected := index.forms[key]
		compiled, ok := snapshotForms[key]
		if !ok {
			return fmt.Errorf("takoform provider: projected current exact FormRef %s is missing from Snapshot", key)
		}
		if compiled.PackageDigest != projected.Ref.PackageDigest {
			return fmt.Errorf("takoform provider: projected current exact FormRef %s pins package %s, Snapshot verified %s", key, projected.Ref.PackageDigest, compiled.PackageDigest)
		}
		selected, ok := snapshot.Default(key.APIVersion, key.Kind)
		if !ok || selected.APIVersion != key.APIVersion || selected.Kind != key.Kind || selected.DefinitionVersion != key.DefinitionVersion || selected.SchemaDigest != key.SchemaDigest {
			return fmt.Errorf("takoform provider: projected default-create exact FormRef %s does not match Snapshot selection %#v", key, selected)
		}
		definitionRaw, ok := snapshot.Definition(compiled.Ref)
		if !ok {
			return fmt.Errorf("takoform provider: Snapshot has no Definition bytes for %s", key)
		}
		definition, err := formpackage.ValidateDefinition(definitionRaw)
		if err != nil {
			return fmt.Errorf("takoform provider: Snapshot Definition %s became invalid: %w", key, err)
		}
		if err := validateProjectedCurrentFormSemantics(projected.Form, definition, snapshot); err != nil {
			return fmt.Errorf("takoform provider: projection Form %s: %w", key, err)
		}
	}
	for key := range snapshotForms {
		if _, ok := index.currentKeys[key]; !ok {
			return fmt.Errorf("takoform provider: Snapshot exact FormRef %s is extra to Provider projection", key)
		}
	}
	if len(snapshot.Interfaces()) != len(closure.Interfaces) || len(snapshot.Bindings()) != len(closure.Bindings) {
		return fmt.Errorf("takoform provider: Snapshot contract counts do not match embedded closure")
	}
	interfaceSet := make(map[formpackage.InterfaceRef]struct{}, len(snapshot.Interfaces()))
	for _, entry := range snapshot.Interfaces() {
		interfaceSet[entry.Ref] = struct{}{}
	}
	for _, entry := range closure.Interfaces {
		if _, ok := interfaceSet[entry.Ref]; !ok {
			return fmt.Errorf("takoform provider: embedded Interface %#v is missing from Snapshot", entry.Ref)
		}
	}
	bindingSet := make(map[formpackage.BindingRef]struct{}, len(snapshot.Bindings()))
	for _, entry := range snapshot.Bindings() {
		bindingSet[entry.Ref] = struct{}{}
	}
	for _, entry := range closure.Bindings {
		if _, ok := bindingSet[entry.Ref]; !ok {
			return fmt.Errorf("takoform provider: embedded Binding %#v is missing from Snapshot", entry.Ref)
		}
	}
	return nil
}

func validateProjectedCurrentFormSemantics(form model.Form, definition formpackage.FormDefinition, snapshot *currentformsnapshot.Snapshot) error {
	if form.Family.APIVersion() != definition.APIVersion || form.Kind != definition.Kind || form.DefinitionVersion != definition.DefinitionVersion ||
		string(form.Role) != definition.Role || form.RequiresHostAPI != definition.RequiresHostAPI || form.Title != definition.Title || form.Description != definition.Description {
		return fmt.Errorf("identity, role, Host API, title, or description differs from verified Definition")
	}
	resolver := v3SnapshotProjectionResolver{snapshot: snapshot}
	desired, err := form.DesiredSchema(resolver)
	if err != nil {
		return fmt.Errorf("derive desired schema from Provider fields: %w", err)
	}
	if !canonicalJSONEqual(desired, definition.DesiredSchema) {
		return fmt.Errorf("HCL/wire fields, kinds, collections, nesting, defaults, validators, or target contracts do not reproduce the verified desired schema")
	}
	outputs, err := form.OutputSchema()
	if err != nil {
		return fmt.Errorf("derive output schema from Provider fields: %w", err)
	}
	if !canonicalJSONEqual(outputs, definition.OutputSchema) {
		return fmt.Errorf("projected outputs do not reproduce the verified output schema: projected=%#v verified=%#v", outputs, definition.OutputSchema)
	}
	if !equalStringSlices(form.ImmutableFields(), definition.ImmutableFields) {
		return fmt.Errorf("projected revision/immutable rules = %#v, Definition = %#v", form.ImmutableFields(), definition.ImmutableFields)
	}
	if !equalStringSlices(form.LifecycleCapabilities(), definition.LifecycleCapabilities) {
		return fmt.Errorf("projected lifecycle/update rules = %#v, Definition = %#v", form.LifecycleCapabilities(), definition.LifecycleCapabilities)
	}
	if err := validateProjectedContractSources(form, definition, snapshot); err != nil {
		return err
	}
	return nil
}

func validateProjectedContractSources(form model.Form, definition formpackage.FormDefinition, snapshot *currentformsnapshot.Snapshot) error {
	provided := make([]formpackage.InterfaceRef, 0, len(form.ProvidedInterfaces))
	for _, source := range form.ProvidedInterfaces {
		matches := make([]formpackage.InterfaceRef, 0, 1)
		for _, contract := range snapshot.Interfaces() {
			if contract.Ref.Name == source.Name && contract.Ref.Version == source.Version {
				matches = append(matches, contract.Ref)
			}
		}
		if len(matches) != 1 {
			return fmt.Errorf("provided Interface %s@%s resolves %d exact Snapshot contracts", source.Name, source.Version, len(matches))
		}
		provided = append(provided, matches[0])
	}
	accepted := make([]formpackage.BindingRef, 0, len(form.AcceptedBindings))
	for _, source := range form.AcceptedBindings {
		matches := make([]formpackage.BindingRef, 0, 1)
		for _, contract := range snapshot.Bindings() {
			if contract.Ref.Name == source.Name && contract.Ref.Version == source.Version {
				matches = append(matches, contract.Ref)
			}
		}
		if len(matches) != 1 {
			return fmt.Errorf("accepted Binding %s@%s resolves %d exact Snapshot contracts", source.Name, source.Version, len(matches))
		}
		accepted = append(accepted, matches[0])
	}
	if !equalInterfaceRefs(provided, definition.ProvidedInterfaces) || !equalBindingRefs(accepted, definition.AcceptedBindings) {
		return fmt.Errorf("projected Interface/Binding sources do not reproduce verified exact refs: projected=%#v/%#v verified=%#v/%#v", provided, accepted, definition.ProvidedInterfaces, definition.AcceptedBindings)
	}
	return nil
}

type v3SnapshotProjectionResolver struct {
	snapshot *currentformsnapshot.Snapshot
}

func (resolver v3SnapshotProjectionResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	resolved := model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName}
	switch {
	case target.Contract.ExactForm && target.Contract.Interface == nil:
		for _, form := range resolver.snapshot.Forms() {
			if form.Ref.APIVersion == target.Group && form.Ref.Kind == target.Kind {
				resolved.TargetFormRefs = append(resolved.TargetFormRefs, model.TargetFormRef{
					APIVersion: form.Ref.APIVersion, Kind: form.Ref.Kind, DefinitionVersion: form.Ref.DefinitionVersion, SchemaDigest: form.Ref.SchemaDigest,
				})
			}
		}
		if len(resolved.TargetFormRefs) == 0 {
			return model.ResolvedResourceTarget{}, fmt.Errorf("exact target %s/%s is absent from Snapshot", target.Group, target.Kind)
		}
		sort.Slice(resolved.TargetFormRefs, func(i, j int) bool {
			if resolved.TargetFormRefs[i].DefinitionVersion != resolved.TargetFormRefs[j].DefinitionVersion {
				return resolved.TargetFormRefs[i].DefinitionVersion < resolved.TargetFormRefs[j].DefinitionVersion
			}
			return resolved.TargetFormRefs[i].SchemaDigest < resolved.TargetFormRefs[j].SchemaDigest
		})
	case !target.Contract.ExactForm && target.Contract.Interface != nil:
		matches := make([]formpackage.InterfaceRef, 0, 1)
		for _, contract := range resolver.snapshot.Interfaces() {
			if contract.Ref.Name == target.Contract.Interface.Name && contract.Ref.Version == target.Contract.Interface.Version {
				matches = append(matches, contract.Ref)
			}
		}
		if len(matches) != 1 {
			return model.ResolvedResourceTarget{}, fmt.Errorf("target Interface %s@%s resolves %d exact Snapshot contracts", target.Contract.Interface.Name, target.Contract.Interface.Version, len(matches))
		}
		resolved.RequiredInterface = &model.RequiredInterface{
			APIVersion: matches[0].APIVersion, Name: matches[0].Name, Version: matches[0].Version, SchemaDigest: matches[0].SchemaDigest,
		}
	default:
		return model.ResolvedResourceTarget{}, fmt.Errorf("ResourceTarget %s/%s has no single exact contract", target.Group, target.Kind)
	}
	return resolved, nil
}

func (resolver v3SnapshotProjectionResolver) ResolveExactFormRelations(ref model.TargetFormRef) ([]model.Relation, error) {
	raw, ok := resolver.snapshot.Definition(formpackage.FormRef{
		APIVersion: ref.APIVersion, Kind: ref.Kind, DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
	})
	if !ok {
		return nil, fmt.Errorf("exact relation target %s is absent from Snapshot", ref.String())
	}
	definition, err := formpackage.ValidateDefinition(raw)
	if err != nil {
		return nil, err
	}
	constraints := make([]model.Constraint, 0, len(definition.Constraints))
	for _, constraint := range definition.Constraints {
		constraints = append(constraints, model.Constraint{
			Kind: model.ConstraintKind(constraint.Kind), Reference: constraint.Reference, KeyedBy: constraint.KeyedBy,
			List: constraint.List, Member: constraint.Member, Total: constraint.Total, Property: constraint.Property,
			Output: constraint.Output, References: append([]string(nil), constraint.References...), Anchor: constraint.Anchor,
			Members: constraint.Members, Through: constraint.Through,
		})
	}
	return model.DeriveRelationsWithConstraints(definition.DesiredSchema, constraints)
}

func canonicalJSONEqual(left, right any) bool {
	leftRaw, leftErr := json.Marshal(left)
	rightRaw, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if bytes.Equal(leftRaw, rightRaw) {
		return true
	}
	leftCanonical, leftErr := formpackage.Canonicalize(leftRaw)
	rightCanonical, rightErr := formpackage.Canonicalize(rightRaw)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftCanonical, rightCanonical)
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalInterfaceRefs(left, right []formpackage.InterfaceRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalBindingRefs(left, right []formpackage.BindingRef) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func renderSnapshotDiagnostics(diagnostics []currentformsnapshot.Diagnostic) string {
	if len(diagnostics) == 0 {
		return "no Snapshot and no diagnostics"
	}
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, fmt.Sprintf("%s %s %s: %s", diagnostic.Code, diagnostic.Subject, diagnostic.Pointer, diagnostic.Message))
	}
	return strings.Join(parts, "; ")
}

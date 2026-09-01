// Package currentformselection acquires the checked-in current Form graph as
// data-only artifacts and compiles it through currentformsnapshot.
//
// The package is deliberately an artifact adapter. It has no knowledge of an
// publisher Form catalog, provider registry, or authoring implementation. The
// current-family-index.json document and the digest-pinned candidate sets are
// the only selection authority; authoringSource is retained as provenance
// metadata and is never opened.
package currentformselection

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformsnapshot"
)

const (
	currentFamilyIndexPath      = "forms/candidates/current-family-index.json"
	currentFamilyIndexFormat    = "takoform.current-family-index@v1"
	familyCandidateSetFormat    = "takoform.form-family-candidates@v1"
	interfaceCandidateSetFormat = "takoform.interface-candidates@v1"
	bindingCandidateSetFormat   = "takoform.binding-candidates@v1"
)

// Family is the stable-copy metadata for one selected family candidate set.
// CandidateSet and SHA256 are the raw repository-relative path and raw-byte
// pin carried by current-family-index.json. AuthoringSource is provenance
// only; it is not a path this package resolves.
type Family struct {
	Group             string `json:"group"`
	CandidateSet      string `json:"candidateSet"`
	SHA256            string `json:"sha256"`
	FormCount         int    `json:"formCount"`
	FormMaturity      string `json:"formMaturity"`
	PublicationStatus string `json:"publicationStatus"`
	AuthoringSource   string `json:"authoringSource,omitempty"`
	AuthoringPolicy   string `json:"authoringPolicy,omitempty"`
	Forms             []Form `json:"forms"`
}

// Form is the stable-copy metadata for one selected Form Package.
type Form struct {
	Group         string              `json:"group"`
	Kind          string              `json:"kind"`
	Role          string              `json:"role"`
	Path          string              `json:"path"`
	Ref           formpackage.FormRef `json:"ref"`
	PackageDigest string              `json:"packageDigest"`
}

// Interface is the stable-copy metadata for one selected Interface
// Definition. Path is the repository-relative definition path derived from
// the candidate name; AuthoringSource is historical provenance only.
type Interface struct {
	Name            string                   `json:"name"`
	Version         string                   `json:"version"`
	SchemaDigest    string                   `json:"schemaDigest"`
	Path            string                   `json:"path"`
	AuthoringSource string                   `json:"authoringSource,omitempty"`
	Ref             formpackage.InterfaceRef `json:"ref"`
}

// Binding is the stable-copy metadata for one selected Binding Definition.
// Path is the repository-relative definition path derived from the candidate
// name; AuthoringSource is historical provenance only.
type Binding struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	SchemaDigest    string                 `json:"schemaDigest"`
	Path            string                 `json:"path"`
	AuthoringSource string                 `json:"authoringSource,omitempty"`
	Ref             formpackage.BindingRef `json:"ref"`
}

// Selection is the immutable result of loading and compiling one repository's
// checked-in current graph. Snapshot's own API returns defensive copies for
// definitions and stable identity views; the metadata accessors below also
// deep-copy their slices on every call.
type Selection struct {
	snapshot   *currentformsnapshot.Snapshot
	families   []Family
	forms      []Form
	interfaces []Interface
	bindings   []Binding
}

// Snapshot returns the compiled, provider-neutral exact-identity graph.
// Snapshot itself is immutable after construction.
func (selection *Selection) Snapshot() *currentformsnapshot.Snapshot {
	if selection == nil {
		return nil
	}
	return selection.snapshot
}

// Families returns stable-copy family and nested Form metadata.
func (selection *Selection) Families() []Family {
	if selection == nil {
		return nil
	}
	output := make([]Family, len(selection.families))
	for index, family := range selection.families {
		output[index] = family
		output[index].Forms = append([]Form(nil), family.Forms...)
	}
	return output
}

// Forms returns stable-copy Form metadata in group, kind, version, digest and
// path order.
func (selection *Selection) Forms() []Form {
	if selection == nil {
		return nil
	}
	return append([]Form(nil), selection.forms...)
}

// Interfaces returns stable-copy Interface metadata in exact identity order.
func (selection *Selection) Interfaces() []Interface {
	if selection == nil {
		return nil
	}
	return append([]Interface(nil), selection.interfaces...)
}

// Bindings returns stable-copy Binding metadata in exact identity order.
func (selection *Selection) Bindings() []Binding {
	if selection == nil {
		return nil
	}
	return append([]Binding(nil), selection.bindings...)
}

type indexDocument struct {
	Format                string        `json:"format"`
	Families              []indexFamily `json:"families"`
	InterfaceCandidateSet indexArtifact `json:"interfaceCandidateSet"`
	BindingCandidateSet   indexArtifact `json:"bindingCandidateSet"`
}

type indexFamily struct {
	Group        string `json:"group"`
	CandidateSet string `json:"candidateSet"`
	SHA256       string `json:"sha256"`
	FormCount    int    `json:"formCount"`
}

type indexArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type familyCandidateSet struct {
	Format            string          `json:"format"`
	Family            string          `json:"family"`
	FormMaturity      string          `json:"formMaturity"`
	PackageAPIVersion string          `json:"packageApiVersion"`
	PublicationStatus string          `json:"publicationStatus"`
	AuthoringSource   string          `json:"authoringSource"`
	AuthoringPolicy   string          `json:"authoringPolicy"`
	Forms             []formCandidate `json:"forms"`
}

type formCandidate struct {
	Kind          string              `json:"kind"`
	Role          string              `json:"role"`
	Path          string              `json:"path"`
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

type interfaceCandidateSet struct {
	Format            string              `json:"format"`
	PublicationStatus string              `json:"publicationStatus"`
	AuthoringSource   string              `json:"authoringSource"`
	Interfaces        []contractCandidate `json:"interfaces"`
}

type bindingCandidateSet struct {
	Format            string              `json:"format"`
	PublicationStatus string              `json:"publicationStatus"`
	AuthoringSource   string              `json:"authoringSource"`
	Bindings          []contractCandidate `json:"bindings"`
}

type contractCandidate struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

type interfaceIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

type bindingIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Version    string `json:"version"`
}

// LoadRepository reads only the checked-in artifact graph below root, verifies
// every pinned byte and package closure, and compiles one stable Host API
// Snapshot. It performs no writes and never follows a repository path through
// a symlink.
func LoadRepository(root string) (*Selection, error) {
	rootPath, err := stableRepositoryRoot(root)
	if err != nil {
		return nil, err
	}

	indexPath, err := repositoryPath(rootPath, currentFamilyIndexPath, false)
	if err != nil {
		return nil, fmt.Errorf("current family index path: %w", err)
	}
	indexRaw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read current family index: %w", err)
	}
	var index indexDocument
	if err := formpackage.DecodeStrictIJSON(indexRaw, &index); err != nil {
		return nil, fmt.Errorf("decode current family index: %w", err)
	}
	if index.Format != currentFamilyIndexFormat {
		return nil, fmt.Errorf("current family index format %q is not %q", index.Format, currentFamilyIndexFormat)
	}
	if len(index.Families) == 0 {
		return nil, fmt.Errorf("current family index contains no families")
	}
	if err := validateIndexFamilies(index.Families); err != nil {
		return nil, err
	}

	input := currentformsnapshot.Input{HostAPI: "forms.takoform.com/v1"}
	families := make([]Family, 0, len(index.Families))
	forms := make([]Form, 0)
	defaults := make(map[string]formpackage.FormRef)
	seenPackagePaths := make(map[string]struct{})
	orderedFamilies := append([]indexFamily(nil), index.Families...)
	sort.Slice(orderedFamilies, func(i, j int) bool { return orderedFamilies[i].Group < orderedFamilies[j].Group })

	for _, familyIndex := range orderedFamilies {
		candidateSetPath, err := repositoryPath(rootPath, familyIndex.CandidateSet, false)
		if err != nil {
			return nil, fmt.Errorf("family %s candidate set path: %w", familyIndex.Group, err)
		}
		candidateRaw, err := os.ReadFile(candidateSetPath)
		if err != nil {
			return nil, fmt.Errorf("read family %s candidate set %q: %w", familyIndex.Group, familyIndex.CandidateSet, err)
		}
		if err := verifyRawSHA256(candidateRaw, familyIndex.SHA256); err != nil {
			return nil, fmt.Errorf("family %s candidate set digest: %w", familyIndex.Group, err)
		}
		var candidateSet familyCandidateSet
		if err := formpackage.DecodeStrictIJSON(candidateRaw, &candidateSet); err != nil {
			return nil, fmt.Errorf("decode family %s candidate set: %w", familyIndex.Group, err)
		}
		if err := validateFamilyCandidateSet(familyIndex, candidateSet); err != nil {
			return nil, err
		}

		family := Family{
			Group: familyIndex.Group, CandidateSet: familyIndex.CandidateSet,
			SHA256: familyIndex.SHA256, FormCount: familyIndex.FormCount,
			FormMaturity: candidateSet.FormMaturity, PublicationStatus: candidateSet.PublicationStatus,
			AuthoringSource: candidateSet.AuthoringSource, AuthoringPolicy: candidateSet.AuthoringPolicy,
			Forms: make([]Form, 0, len(candidateSet.Forms)),
		}
		for _, candidate := range candidateSet.Forms {
			form, artifact, err := loadFormCandidate(rootPath, familyIndex.Group, candidate, seenPackagePaths)
			if err != nil {
				return nil, err
			}
			family.Forms = append(family.Forms, form)
			forms = append(forms, form)
			input.Packages = append(input.Packages, artifact)

			groupKind := candidate.FormRef.APIVersion + "\x00" + candidate.FormRef.Kind
			if prior, exists := defaults[groupKind]; exists {
				return nil, fmt.Errorf("group+kind %s/%s has more than one selected default (%#v and %#v)", candidate.FormRef.APIVersion, candidate.FormRef.Kind, prior, candidate.FormRef)
			}
			defaults[groupKind] = candidate.FormRef
			input.DefaultCreates = append(input.DefaultCreates, currentformsnapshot.DefaultPin{
				Group: candidate.FormRef.APIVersion, Kind: candidate.FormRef.Kind, Ref: candidate.FormRef,
			})
		}
		families = append(families, family)
	}

	interfaces, interfaceArtifacts, err := loadInterfaces(rootPath, index.InterfaceCandidateSet)
	if err != nil {
		return nil, err
	}
	input.Interfaces = interfaceArtifacts

	bindings, bindingArtifacts, err := loadBindings(rootPath, index.BindingCandidateSet)
	if err != nil {
		return nil, err
	}
	input.Bindings = bindingArtifacts

	snapshot, diagnostics := currentformsnapshot.Compile(input)
	if len(diagnostics) != 0 || snapshot == nil {
		return nil, fmt.Errorf("compile current Form Snapshot: %s", formatDiagnostics(diagnostics))
	}

	sort.Slice(families, func(i, j int) bool { return families[i].Group < families[j].Group })
	for i := range families {
		sort.Slice(families[i].Forms, func(left, right int) bool { return lessForm(families[i].Forms[left], families[i].Forms[right]) })
	}
	sort.Slice(forms, func(i, j int) bool { return lessForm(forms[i], forms[j]) })
	sort.Slice(interfaces, func(i, j int) bool { return lessInterface(interfaces[i], interfaces[j]) })
	sort.Slice(bindings, func(i, j int) bool { return lessBinding(bindings[i], bindings[j]) })

	return &Selection{
		snapshot: snapshot,
		families: cloneFamilies(families), forms: append([]Form(nil), forms...),
		interfaces: append([]Interface(nil), interfaces...), bindings: append([]Binding(nil), bindings...),
	}, nil
}

func stableRepositoryRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("repository root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", fmt.Errorf("stat repository root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("repository root is not a regular directory")
	}
	return filepath.Clean(abs), nil
}

func repositoryPath(root, relative string, wantDir bool) (string, error) {
	normalized, err := normalizeRelativePath(relative)
	if err != nil {
		return "", err
	}
	root = filepath.Clean(root)
	joined := filepath.Join(root, filepath.FromSlash(normalized))
	relativeCheck, err := filepath.Rel(root, joined)
	if err != nil || relativeCheck == ".." || strings.HasPrefix(relativeCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository root", relative)
	}

	current := root
	segments := strings.Split(normalized, "/")
	for index, segment := range segments {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if err != nil {
			return "", fmt.Errorf("path %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %q traverses symlink at %q", relative, strings.Join(segments[:index+1], "/"))
		}
		if index < len(segments)-1 && !info.IsDir() {
			return "", fmt.Errorf("path %q has non-directory parent %q", relative, segment)
		}
		if index == len(segments)-1 && wantDir && !info.IsDir() {
			return "", fmt.Errorf("path %q is not a directory", relative)
		}
		if index == len(segments)-1 && !wantDir && !info.Mode().IsRegular() {
			return "", fmt.Errorf("path %q is not a regular file", relative)
		}
	}
	return joined, nil
}

func normalizeRelativePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") {
		return "", fmt.Errorf("path %q is not a normalized repository-relative path", value)
	}
	if strings.HasPrefix(value, "/") || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || strings.Contains(value, ":") {
		return "", fmt.Errorf("path %q is not a repository-relative path", value)
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path %q is not normalized or contains traversal", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("path %q contains an empty, dot, or parent segment", value)
		}
	}
	return value, nil
}

func validateIndexFamilies(families []indexFamily) error {
	seen := make(map[string]struct{}, len(families))
	for _, family := range families {
		if family.Group == "" {
			return fmt.Errorf("current family index contains an empty family group")
		}
		if _, exists := seen[family.Group]; exists {
			return fmt.Errorf("current family index contains duplicate family %q", family.Group)
		}
		seen[family.Group] = struct{}{}
		if family.FormCount < 0 {
			return fmt.Errorf("family %s has a negative formCount", family.Group)
		}
		if err := validRawSHA256(family.SHA256); err != nil {
			return fmt.Errorf("family %s sha256: %w", family.Group, err)
		}
		if _, err := normalizeRelativePath(family.CandidateSet); err != nil {
			return fmt.Errorf("family %s candidateSet: %w", family.Group, err)
		}
	}
	return nil
}

func validateFamilyCandidateSet(index indexFamily, set familyCandidateSet) error {
	if set.Format != familyCandidateSetFormat {
		return fmt.Errorf("family %s candidate set format %q is not %q", index.Group, set.Format, familyCandidateSetFormat)
	}
	if set.Family != index.Group {
		return fmt.Errorf("family candidate set declares %q, index selects %q", set.Family, index.Group)
	}
	if len(set.Forms) != index.FormCount {
		return fmt.Errorf("family %s candidate count = %d, index says %d", index.Group, len(set.Forms), index.FormCount)
	}
	if set.PackageAPIVersion == "" {
		return fmt.Errorf("family %s candidate set has no packageApiVersion", index.Group)
	}
	return nil
}

func loadFormCandidate(root, group string, candidate formCandidate, seenPaths map[string]struct{}) (Form, currentformsnapshot.PackageArtifact, error) {
	if candidate.Kind == "" || candidate.Kind != candidate.FormRef.Kind {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("family %s candidate has inconsistent kind metadata", group)
	}
	if candidate.FormRef.APIVersion != group {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("family %s candidate %s belongs to Form group %q", group, candidate.Kind, candidate.FormRef.APIVersion)
	}
	if candidate.Role == "" || !formpackage.ValidDigest(candidate.PackageDigest) || !formpackage.ValidDigest(candidate.FormRef.SchemaDigest) {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("family %s/%s candidate has a non-canonical digest or empty role", group, candidate.Kind)
	}
	packagePath, err := repositoryPath(root, candidate.Path, true)
	if err != nil {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("Form %s/%s package path: %w", group, candidate.Kind, err)
	}
	if _, exists := seenPaths[candidate.Path]; exists {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("Form package path %q is selected more than once", candidate.Path)
	}
	seenPaths[candidate.Path] = struct{}{}
	report, err := formpackage.VerifyDirectory(packagePath)
	if err != nil {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("verify Form Package %s/%s at %q: %w", group, candidate.Kind, candidate.Path, err)
	}
	verified, ok := report.VerifiedPackage()
	if !ok {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("verify Form Package %s/%s did not issue a package capability", group, candidate.Kind)
	}
	if verified.FormRef() != candidate.FormRef {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("Form %s/%s exact FormRef drift: package=%#v candidate=%#v", group, candidate.Kind, verified.FormRef(), candidate.FormRef)
	}
	if verified.PackageDigest() != candidate.PackageDigest {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("Form %s/%s package digest drift: package=%s candidate=%s", group, candidate.Kind, verified.PackageDigest(), candidate.PackageDigest)
	}
	definition, err := formpackage.ValidateDefinition(verified.Definition())
	if err != nil {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("validate Form %s/%s Definition: %w", group, candidate.Kind, err)
	}
	if definition.Role != candidate.Role {
		return Form{}, currentformsnapshot.PackageArtifact{}, fmt.Errorf("Form %s/%s role drift: package=%q candidate=%q", group, candidate.Kind, definition.Role, candidate.Role)
	}
	form := Form{Group: group, Kind: candidate.Kind, Role: candidate.Role, Path: candidate.Path, Ref: candidate.FormRef, PackageDigest: candidate.PackageDigest}
	return form, currentformsnapshot.PackageArtifact{Origin: "repo://" + candidate.Path, ExpectedDigest: candidate.PackageDigest, Package: verified}, nil
}

func loadInterfaces(root string, pointer indexArtifact) ([]Interface, []currentformsnapshot.InterfaceArtifact, error) {
	pathValue, raw, err := loadContractCandidateSet(root, pointer)
	if err != nil {
		return nil, nil, fmt.Errorf("Interface candidate set: %w", err)
	}
	var candidateSet interfaceCandidateSet
	if err := formpackage.DecodeStrictIJSON(raw, &candidateSet); err != nil {
		return nil, nil, fmt.Errorf("decode Interface candidate set %q: %w", pathValue, err)
	}
	if candidateSet.Format != interfaceCandidateSetFormat {
		return nil, nil, fmt.Errorf("Interface candidate set format %q is not %q", candidateSet.Format, interfaceCandidateSetFormat)
	}
	metadata := make([]Interface, 0, len(candidateSet.Interfaces))
	artifacts := make([]currentformsnapshot.InterfaceArtifact, 0, len(candidateSet.Interfaces))
	seen := make(map[string]struct{}, len(candidateSet.Interfaces))
	for _, candidate := range candidateSet.Interfaces {
		if !formpackage.ValidDigest(candidate.SchemaDigest) {
			return nil, nil, fmt.Errorf("Interface %s has a non-canonical schema digest", candidate.Name)
		}
		definitionPath, rawDefinition, identity, err := loadContractDefinition(root, pathValue, candidate.Name, false)
		if err != nil {
			return nil, nil, fmt.Errorf("Interface %s: %w", candidate.Name, err)
		}
		if identity.Name != candidate.Name || identity.Version != candidate.Version || identity.Kind != "InterfaceDefinition" {
			return nil, nil, fmt.Errorf("Interface %s identity drift: definition is %#v", candidate.Name, identity)
		}
		digest, err := formpackage.DigestCanonicalJSON(rawDefinition)
		if err != nil {
			return nil, nil, fmt.Errorf("Interface %s digest: %w", candidate.Name, err)
		}
		if digest != candidate.SchemaDigest {
			return nil, nil, fmt.Errorf("Interface %s digest drift: definition=%s candidate=%s", candidate.Name, digest, candidate.SchemaDigest)
		}
		ref := formpackage.InterfaceRef{APIVersion: identity.APIVersion, Name: identity.Name, Version: identity.Version, SchemaDigest: digest}
		key := identity.APIVersion + "\x00" + identity.Name + "\x00" + identity.Version + "\x00" + digest
		if _, exists := seen[key]; exists {
			return nil, nil, fmt.Errorf("Interface %s exact identity is selected more than once", candidate.Name)
		}
		seen[key] = struct{}{}
		metadata = append(metadata, Interface{Name: candidate.Name, Version: candidate.Version, SchemaDigest: candidate.SchemaDigest, Path: definitionPath, AuthoringSource: candidateSet.AuthoringSource, Ref: ref})
		artifacts = append(artifacts, currentformsnapshot.InterfaceArtifact{Origin: "repo://" + definitionPath, ExpectedDigest: candidate.SchemaDigest, Definition: append([]byte(nil), rawDefinition...)})
	}
	return metadata, artifacts, nil
}

func loadBindings(root string, pointer indexArtifact) ([]Binding, []currentformsnapshot.BindingArtifact, error) {
	pathValue, raw, err := loadContractCandidateSet(root, pointer)
	if err != nil {
		return nil, nil, fmt.Errorf("Binding candidate set: %w", err)
	}
	var candidateSet bindingCandidateSet
	if err := formpackage.DecodeStrictIJSON(raw, &candidateSet); err != nil {
		return nil, nil, fmt.Errorf("decode Binding candidate set %q: %w", pathValue, err)
	}
	if candidateSet.Format != bindingCandidateSetFormat {
		return nil, nil, fmt.Errorf("Binding candidate set format %q is not %q", candidateSet.Format, bindingCandidateSetFormat)
	}
	metadata := make([]Binding, 0, len(candidateSet.Bindings))
	artifacts := make([]currentformsnapshot.BindingArtifact, 0, len(candidateSet.Bindings))
	seen := make(map[string]struct{}, len(candidateSet.Bindings))
	for _, candidate := range candidateSet.Bindings {
		if !formpackage.ValidDigest(candidate.SchemaDigest) {
			return nil, nil, fmt.Errorf("Binding %s has a non-canonical schema digest", candidate.Name)
		}
		definitionPath, rawDefinition, identity, err := loadContractDefinition(root, pathValue, candidate.Name, true)
		if err != nil {
			return nil, nil, fmt.Errorf("Binding %s: %w", candidate.Name, err)
		}
		if identity.Name != candidate.Name || identity.Version != candidate.Version || identity.Kind != "BindingDefinition" {
			return nil, nil, fmt.Errorf("Binding %s identity drift: definition is %#v", candidate.Name, identity)
		}
		digest, err := formpackage.DigestCanonicalJSON(rawDefinition)
		if err != nil {
			return nil, nil, fmt.Errorf("Binding %s digest: %w", candidate.Name, err)
		}
		if digest != candidate.SchemaDigest {
			return nil, nil, fmt.Errorf("Binding %s digest drift: definition=%s candidate=%s", candidate.Name, digest, candidate.SchemaDigest)
		}
		ref := formpackage.BindingRef{APIVersion: identity.APIVersion, Name: identity.Name, Version: identity.Version, SchemaDigest: digest}
		key := identity.APIVersion + "\x00" + identity.Name + "\x00" + identity.Version + "\x00" + digest
		if _, exists := seen[key]; exists {
			return nil, nil, fmt.Errorf("Binding %s exact identity is selected more than once", candidate.Name)
		}
		seen[key] = struct{}{}
		metadata = append(metadata, Binding{Name: candidate.Name, Version: candidate.Version, SchemaDigest: candidate.SchemaDigest, Path: definitionPath, AuthoringSource: candidateSet.AuthoringSource, Ref: ref})
		artifacts = append(artifacts, currentformsnapshot.BindingArtifact{Origin: "repo://" + definitionPath, ExpectedDigest: candidate.SchemaDigest, Definition: append([]byte(nil), rawDefinition...)})
	}
	return metadata, artifacts, nil
}

func loadContractCandidateSet(root string, pointer indexArtifact) (string, []byte, error) {
	pathValue, err := normalizeRelativePath(pointer.Path)
	if err != nil {
		return "", nil, err
	}
	pathValue, err = repositoryPath(root, pathValue, false)
	if err != nil {
		return "", nil, err
	}
	raw, err := os.ReadFile(pathValue)
	if err != nil {
		return "", nil, err
	}
	if err := verifyRawSHA256(raw, pointer.SHA256); err != nil {
		return "", nil, err
	}
	return filepath.ToSlash(strings.TrimPrefix(pathValue, filepath.Clean(root)+string(filepath.Separator))), raw, nil
}

func loadContractDefinition(root, candidateSetPath, name string, binding bool) (string, []byte, interfaceIdentity, error) {
	if _, err := normalizePathSegment(name); err != nil {
		return "", nil, interfaceIdentity{}, err
	}
	base := path.Dir(candidateSetPath)
	definitionRelative := path.Join(base, name, "definition.json")
	definitionPath, err := repositoryPath(root, definitionRelative, false)
	if err != nil {
		return "", nil, interfaceIdentity{}, err
	}
	raw, err := os.ReadFile(definitionPath)
	if err != nil {
		return "", nil, interfaceIdentity{}, err
	}
	if binding {
		if err := formpackage.ValidateBindingDefinition(raw); err != nil {
			return "", nil, interfaceIdentity{}, err
		}
		var identity bindingIdentity
		if err := decodeStrictObject(raw, &identity); err != nil {
			return "", nil, interfaceIdentity{}, err
		}
		return definitionRelative, raw, interfaceIdentity{APIVersion: identity.APIVersion, Kind: identity.Kind, Name: identity.Name, Version: identity.Version}, nil
	}
	if err := formpackage.ValidateInterfaceDefinition(raw); err != nil {
		return "", nil, interfaceIdentity{}, err
	}
	var identity interfaceIdentity
	if err := decodeStrictObject(raw, &identity); err != nil {
		return "", nil, interfaceIdentity{}, err
	}
	return definitionRelative, raw, identity, nil
}

func normalizePathSegment(value string) (string, error) {
	normalized, err := normalizeRelativePath(value)
	if err != nil {
		return "", err
	}
	if strings.Contains(normalized, "/") {
		return "", fmt.Errorf("path segment %q contains a separator", value)
	}
	return normalized, nil
}

func decodeStrictObject(raw []byte, destination any) error {
	var object map[string]any
	if err := formpackage.DecodeStrictIJSON(raw, &object); err != nil {
		return err
	}
	return json.Unmarshal(raw, destination)
}

func verifyRawSHA256(raw []byte, expected string) error {
	if err := validRawSHA256(expected); err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	actual := hex.EncodeToString(digest[:])
	if actual != expected {
		return fmt.Errorf("raw SHA-256 is %s, index pins %s", actual, expected)
	}
	return nil
}

func validRawSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("raw SHA-256 pin %q is not 64 lowercase hexadecimal characters", value)
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return fmt.Errorf("raw SHA-256 pin %q is not 64 lowercase hexadecimal characters", value)
		}
	}
	return nil
}

func lessForm(left, right Form) bool {
	if left.Group != right.Group {
		return left.Group < right.Group
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Ref.DefinitionVersion != right.Ref.DefinitionVersion {
		return left.Ref.DefinitionVersion < right.Ref.DefinitionVersion
	}
	if left.Ref.SchemaDigest != right.Ref.SchemaDigest {
		return left.Ref.SchemaDigest < right.Ref.SchemaDigest
	}
	return left.Path < right.Path
}

func lessInterface(left, right Interface) bool {
	if left.Ref.APIVersion != right.Ref.APIVersion {
		return left.Ref.APIVersion < right.Ref.APIVersion
	}
	if left.Ref.Name != right.Ref.Name {
		return left.Ref.Name < right.Ref.Name
	}
	if left.Ref.Version != right.Ref.Version {
		return left.Ref.Version < right.Ref.Version
	}
	if left.Ref.SchemaDigest != right.Ref.SchemaDigest {
		return left.Ref.SchemaDigest < right.Ref.SchemaDigest
	}
	return left.Path < right.Path
}

func lessBinding(left, right Binding) bool {
	if left.Ref.APIVersion != right.Ref.APIVersion {
		return left.Ref.APIVersion < right.Ref.APIVersion
	}
	if left.Ref.Name != right.Ref.Name {
		return left.Ref.Name < right.Ref.Name
	}
	if left.Ref.Version != right.Ref.Version {
		return left.Ref.Version < right.Ref.Version
	}
	if left.Ref.SchemaDigest != right.Ref.SchemaDigest {
		return left.Ref.SchemaDigest < right.Ref.SchemaDigest
	}
	return left.Path < right.Path
}

func cloneFamilies(input []Family) []Family {
	output := make([]Family, len(input))
	for index, family := range input {
		output[index] = family
		output[index].Forms = append([]Form(nil), family.Forms...)
	}
	return output
}

func formatDiagnostics(diagnostics []currentformsnapshot.Diagnostic) string {
	if len(diagnostics) == 0 {
		return "no diagnostics and no Snapshot"
	}
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, fmt.Sprintf("%s: %s", diagnostic.Code, diagnostic.Message))
	}
	return strings.Join(parts, "; ")
}

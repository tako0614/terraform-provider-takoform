package portableconformancev3

// This file is the family-owned adapter for the current non-Edge witness. It
// deliberately stays below the neutral Snapshot seam: packages are acquired
// and verified here, while exact definitions, defaults, schema validation and
// default materialisation come only from currentformsnapshot.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformsnapshot"
)

const stableCurrentNonEdgeFamilyGroup = "table.forms.takoform.com"

// stableCandidateSetForm aliases the anonymous JSON member type in
// stableCandidateSet. Keeping this adapter's intermediate records named makes
// the acquisition/compiler boundary explicit without changing the corpus
// model owned by stable_suite.go.
type stableCandidateSetForm = struct {
	Kind          string  `json:"kind"`
	Role          string  `json:"role"`
	Path          string  `json:"path"`
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest"`
}

type stableCurrentVerifiedCandidate struct {
	candidate  stableCandidateSetForm
	verified   formpackage.VerifiedPackage
	definition formpackage.FormDefinition
}

// stableRunCurrentNonEdgeFamilyWitness executes the small, data-only witness
// for the current non-Edge family. The witness is intentionally an adapter
// over a compiled Snapshot rather than a second family catalog or a generic
// Host plan. It proves that the package bytes can pass the complete verifier,
// enter the same exact-identity compiler as any other package, and then carry
// one family-owned resource through its declared lifecycle.
func stableRunCurrentNonEdgeFamilyWitness(
	root, corpusPath string,
	corpus stableFamilyCorpus,
	set stableCandidateSet,
) error {
	if corpus.Group != stableCurrentNonEdgeFamilyGroup || set.Family != stableCurrentNonEdgeFamilyGroup {
		return fmt.Errorf("current non-Edge Snapshot witness only owns %q", stableCurrentNonEdgeFamilyGroup)
	}
	if corpus.HostAPILane != stableLane.APIVersion {
		return fmt.Errorf("current non-Edge Snapshot witness Host API = %q, want %q", corpus.HostAPILane, stableLane.APIVersion)
	}
	if len(set.Forms) == 0 {
		return errors.New("current non-Edge Snapshot witness has no candidate packages")
	}
	if len(corpus.RunnerInput) != len(set.Forms) {
		return fmt.Errorf("current non-Edge Snapshot witness runner roster = %d, candidate roster = %d", len(corpus.RunnerInput), len(set.Forms))
	}

	// Verify every package closure before constructing any compiler input. A
	// package digest is provenance evidence and is therefore checked both at
	// this acquisition boundary and again by Core against the issued package
	// capability.
	verified := make([]stableCurrentVerifiedCandidate, 0, len(set.Forms))
	seenRefs := make(map[string]struct{}, len(set.Forms))
	for _, candidate := range set.Forms {
		if candidate.FormRef.APIVersion != stableCurrentNonEdgeFamilyGroup {
			return fmt.Errorf("current non-Edge candidate %s belongs to %q", candidate.Kind, candidate.FormRef.APIVersion)
		}
		if candidate.Path == "" || filepath.IsAbs(candidate.Path) || filepath.Clean(filepath.FromSlash(candidate.Path)) != filepath.FromSlash(candidate.Path) {
			return fmt.Errorf("current non-Edge candidate %s has invalid package path %q", candidate.Kind, candidate.Path)
		}
		packagePath, err := stableCurrentFamilyDirectory(root, candidate.Path)
		if err != nil {
			return fmt.Errorf("current non-Edge candidate %s: %w", candidate.Kind, err)
		}
		report, err := formpackage.VerifyDirectory(packagePath)
		if err != nil {
			return fmt.Errorf("verify current non-Edge package %s: %w", candidate.Kind, err)
		}
		packageValue, ok := report.VerifiedPackage()
		if !ok {
			return fmt.Errorf("verify current non-Edge package %s issued no verified capability", candidate.Kind)
		}
		if packageValue.PackageDigest() != candidate.PackageDigest {
			return fmt.Errorf("current non-Edge package %s digest = %s, want %s", candidate.Kind, packageValue.PackageDigest(), candidate.PackageDigest)
		}
		packageRef := packageValue.FormRef()
		if packageRef != formpackageFormRef(candidate.FormRef) {
			return fmt.Errorf("current non-Edge package %s FormRef = %#v, want %#v", candidate.Kind, packageRef, candidate.FormRef)
		}
		key := stableFormRefKey(candidate.FormRef)
		if _, repeated := seenRefs[key]; repeated {
			return fmt.Errorf("current non-Edge candidate repeats exact FormRef %s", key)
		}
		seenRefs[key] = struct{}{}
		definition, err := formpackage.ValidateDefinition(packageValue.Definition())
		if err != nil {
			return fmt.Errorf("validate current non-Edge package %s Definition: %w", candidate.Kind, err)
		}
		if definition.RequiresHostAPI != stableLane.APIVersion {
			return fmt.Errorf("current non-Edge package %s requires Host API %q, want %q", candidate.Kind, definition.RequiresHostAPI, stableLane.APIVersion)
		}
		verified = append(verified, stableCurrentVerifiedCandidate{candidate: candidate, verified: packageValue, definition: definition})
	}

	// The current table witness has no separate published binding. Its
	// table.document Interface is still part of the exact Definition graph, so
	// load the digest-pinned data artifact rather than dropping that edge.
	interfaceArtifacts, bindingArtifacts, err := stableCurrentFamilyContractArtifacts(root, verified)
	if err != nil {
		return err
	}
	packageArtifacts := make([]currentformsnapshot.PackageArtifact, 0, len(verified))
	defaults := make([]currentformsnapshot.DefaultPin, 0, len(verified))
	defaultGroups := make(map[string]struct{}, len(verified))
	for _, entry := range verified {
		packageArtifacts = append(packageArtifacts, currentformsnapshot.PackageArtifact{
			Origin:         "family-conformance://" + filepath.ToSlash(entry.candidate.Path),
			ExpectedDigest: entry.candidate.PackageDigest,
			Package:        entry.verified,
		})
		groupKind := entry.candidate.FormRef.APIVersion + "\x00" + entry.candidate.FormRef.Kind
		if _, exists := defaultGroups[groupKind]; exists {
			return fmt.Errorf("current non-Edge Snapshot witness has ambiguous default for %s/%s", entry.candidate.FormRef.APIVersion, entry.candidate.FormRef.Kind)
		}
		defaultGroups[groupKind] = struct{}{}
		defaults = append(defaults, currentformsnapshot.DefaultPin{
			Group: entry.candidate.FormRef.APIVersion,
			Kind:  entry.candidate.FormRef.Kind,
			Ref:   formpackageFormRef(entry.candidate.FormRef),
		})
	}
	sort.Slice(defaults, func(i, j int) bool {
		if defaults[i].Group != defaults[j].Group {
			return defaults[i].Group < defaults[j].Group
		}
		return defaults[i].Kind < defaults[j].Kind
	})

	snapshot, diagnostics := currentformsnapshot.Compile(currentformsnapshot.Input{
		HostAPI:        corpus.HostAPILane,
		Packages:       packageArtifacts,
		Interfaces:     interfaceArtifacts,
		Bindings:       bindingArtifacts,
		DefaultCreates: defaults,
	})
	if snapshot == nil || len(diagnostics) != 0 {
		return fmt.Errorf("compile current non-Edge Snapshot: %#v", diagnostics)
	}
	if snapshot.HostAPI() != stableLane.APIVersion || snapshot.Digest() == "" {
		return errors.New("compile current non-Edge Snapshot returned incomplete identity")
	}
	if len(snapshot.Forms()) != len(verified) {
		return fmt.Errorf("current non-Edge Snapshot Forms = %d, want %d", len(snapshot.Forms()), len(verified))
	}
	for _, entry := range verified {
		ref := formpackageFormRef(entry.candidate.FormRef)
		if _, ok := snapshot.Definition(ref); !ok {
			return fmt.Errorf("current non-Edge Snapshot omitted exact FormRef %s", stableFormRefKey(entry.candidate.FormRef))
		}
		if got, ok := snapshot.Default(ref.APIVersion, ref.Kind); !ok || got != ref {
			return fmt.Errorf("current non-Edge Snapshot default for %s/%s = %#v, want %#v", ref.APIVersion, ref.Kind, got, ref)
		}
	}

	return stableRunCurrentNonEdgeFamilyLifecycle(root, corpusPath, snapshot, corpus, set)
}

func stableCurrentFamilyContractArtifacts(
	root string,
	verified []stableCurrentVerifiedCandidate,
) ([]currentformsnapshot.InterfaceArtifact, []currentformsnapshot.BindingArtifact, error) {
	interfaces := make(map[string]currentformsnapshot.InterfaceArtifact)
	bindings := make(map[string]currentformsnapshot.BindingArtifact)
	for _, entry := range verified {
		for _, ref := range entry.definition.ProvidedInterfaces {
			key := stableCurrentInterfaceRefKey(ref)
			if _, exists := interfaces[key]; exists {
				continue
			}
			path, err := stableCurrentContractDefinitionPath(root, "interfaces", ref.APIVersion, ref.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("load Interface %s: %w", key, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, nil, fmt.Errorf("read Interface %s: %w", key, err)
			}
			digest, err := formpackage.DigestCanonicalJSON(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("digest Interface %s: %w", key, err)
			}
			if digest != ref.SchemaDigest {
				return nil, nil, fmt.Errorf("Interface %s digest = %s, want %s", key, digest, ref.SchemaDigest)
			}
			interfaces[key] = currentformsnapshot.InterfaceArtifact{
				Origin: path, ExpectedDigest: ref.SchemaDigest, Definition: raw,
			}
		}
		for _, ref := range entry.definition.AcceptedBindings {
			key := stableCurrentBindingRefKey(ref)
			if _, exists := bindings[key]; exists {
				continue
			}
			path, err := stableCurrentContractDefinitionPath(root, "bindings", ref.APIVersion, ref.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("load Binding %s: %w", key, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, nil, fmt.Errorf("read Binding %s: %w", key, err)
			}
			digest, err := formpackage.DigestCanonicalJSON(raw)
			if err != nil {
				return nil, nil, fmt.Errorf("digest Binding %s: %w", key, err)
			}
			if digest != ref.SchemaDigest {
				return nil, nil, fmt.Errorf("Binding %s digest = %s, want %s", key, digest, ref.SchemaDigest)
			}
			bindings[key] = currentformsnapshot.BindingArtifact{
				Origin: path, ExpectedDigest: ref.SchemaDigest, Definition: raw,
			}
		}
	}
	interfaceKeys := make([]string, 0, len(interfaces))
	for key := range interfaces {
		interfaceKeys = append(interfaceKeys, key)
	}
	sort.Strings(interfaceKeys)
	interfaceList := make([]currentformsnapshot.InterfaceArtifact, 0, len(interfaceKeys))
	for _, key := range interfaceKeys {
		interfaceList = append(interfaceList, interfaces[key])
	}
	bindingKeys := make([]string, 0, len(bindings))
	for key := range bindings {
		bindingKeys = append(bindingKeys, key)
	}
	sort.Strings(bindingKeys)
	bindingList := make([]currentformsnapshot.BindingArtifact, 0, len(bindingKeys))
	for _, key := range bindingKeys {
		bindingList = append(bindingList, bindings[key])
	}
	return interfaceList, bindingList, nil
}

func stableCurrentInterfaceRefKey(ref formpackage.InterfaceRef) string {
	return ref.APIVersion + "/" + ref.Name + "@" + ref.Version + "#" + ref.SchemaDigest
}

func stableCurrentBindingRefKey(ref formpackage.BindingRef) string {
	return ref.APIVersion + "/" + ref.Name + "@" + ref.Version + "#" + ref.SchemaDigest
}

func stableCurrentContractDefinitionPath(root, kind, apiVersion, name string) (string, error) {
	_, version, ok := splitAPIVersion(apiVersion)
	if !ok || version == "" || name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", fmt.Errorf("invalid %s identity %s/%s", kind, apiVersion, name)
	}
	relative := filepath.Join(kind, "candidates", version, name, "definition.json")
	return stableCurrentFamilyFile(root, filepath.ToSlash(relative))
}

func stableCurrentFamilyDirectory(root, relative string) (string, error) {
	path, err := stableCurrentFamilyPath(root, relative)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", relative)
	}
	return path, nil
}

func stableCurrentFamilyFile(root, relative string) (string, error) {
	path, err := stableCurrentFamilyPath(root, relative)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", relative)
	}
	return path, nil
}

func stableCurrentFamilyPath(root, relative string) (string, error) {
	if root == "" || relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid repository-relative path %q", relative)
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative)))
	if normalized != relative || normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("invalid repository-relative path %q", relative)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(rootAbs, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository root", relative)
	}
	// EvalSymlinks closes the path traversal seam before VerifyDirectory sees a
	// package. A symlinked package or contract file must remain inside the same
	// repository root; VerifyDirectory separately rejects symlinked payloads.
	resolvedRoot, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err = filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes repository root through a symlink", relative)
	}
	return path, nil
}

func stableRunCurrentNonEdgeFamilyLifecycle(
	root, corpusPath string,
	snapshot *currentformsnapshot.Snapshot,
	corpus stableFamilyCorpus,
	set stableCandidateSet,
) error {
	if snapshot == nil {
		return errors.New("current non-Edge lifecycle received a nil Snapshot")
	}
	candidateByRef := make(map[string]stableCandidateSetForm, len(set.Forms))
	for _, candidate := range set.Forms {
		candidateByRef[stableFormRefKey(candidate.FormRef)] = candidate
	}
	probeNames := make([]string, 0, len(corpus.RunnerInput))
	for name := range corpus.RunnerInput {
		probeNames = append(probeNames, name)
	}
	sort.Strings(probeNames)
	live := make(map[string]stableCurrentFamilyState, len(probeNames))
	seen := make(map[string]struct{}, len(probeNames))
	for _, probeName := range probeNames {
		probe := corpus.RunnerInput[probeName]
		key := stableFormRefKey(probe.Identity.FormRef)
		candidate, ok := candidateByRef[key]
		if !ok {
			return fmt.Errorf("current non-Edge probe %s names unknown exact FormRef %s", probeName, key)
		}
		if candidate.PackageDigest != probe.Identity.PackageDigest {
			return fmt.Errorf("current non-Edge probe %s package digest drifted", probeName)
		}
		if len(probe.LifecycleCapabilities) == 0 {
			return fmt.Errorf("current non-Edge probe %s has no lifecycle capabilities", probeName)
		}
		if _, repeated := seen[key]; repeated {
			return fmt.Errorf("current non-Edge exact FormRef lifecycle repeated %s", key)
		}
		seen[key] = struct{}{}
		ref := formpackageFormRef(probe.Identity.FormRef)
		definitionRaw, known := snapshot.Definition(ref)
		if !known {
			return fmt.Errorf("current non-Edge probe %s exact FormRef is absent from Snapshot", probeName)
		}
		definition, err := formpackage.ValidateDefinition(definitionRaw)
		if err != nil {
			return fmt.Errorf("current non-Edge probe %s Snapshot Definition: %w", probeName, err)
		}
		if !reflect.DeepEqual(definition.LifecycleCapabilities, probe.LifecycleCapabilities) {
			return fmt.Errorf("current non-Edge probe %s lifecycle capabilities drifted: got %v, want %v", probeName, definition.LifecycleCapabilities, probe.LifecycleCapabilities)
		}
		capabilities := make(map[string]struct{}, len(definition.LifecycleCapabilities))
		for _, capability := range definition.LifecycleCapabilities {
			capabilities[capability] = struct{}{}
		}
		for _, required := range []string{"create", "read", "delete"} {
			if _, supported := capabilities[required]; !supported {
				return fmt.Errorf("current non-Edge probe %s lacks required %s capability", probeName, required)
			}
		}
		if probe.Desired == nil || len(probe.Desired) == 0 {
			return fmt.Errorf("current non-Edge probe %s has no desired document", probeName)
		}
		if probe.DesiredSchema.Path == "" || probe.DesiredSchema.SHA256 == "" {
			return fmt.Errorf("current non-Edge probe %s has no desired-schema pin", probeName)
		}
		schemaPath, err := stableResolve(root, corpusPath, probe.DesiredSchema.Path)
		if err != nil {
			return fmt.Errorf("current non-Edge probe %s desired-schema pin: %w", probeName, err)
		}
		schemaRaw, err := stableVerifyDigest(schemaPath, probe.DesiredSchema.SHA256)
		if err != nil {
			return fmt.Errorf("current non-Edge probe %s desired-schema pin: %w", probeName, err)
		}
		var pinnedSchema map[string]any
		if err := formpackage.DecodeStrictIJSON(schemaRaw, &pinnedSchema); err != nil {
			return fmt.Errorf("current non-Edge probe %s desired-schema pin: %w", probeName, err)
		}
		pinned, err := canonicalJSON(pinnedSchema)
		if err != nil {
			return fmt.Errorf("current non-Edge probe %s desired-schema pin: %w", probeName, err)
		}
		defined, err := canonicalJSON(definition.DesiredSchema)
		if err != nil || pinned != defined {
			return fmt.Errorf("current non-Edge probe %s desired-schema pin drifted", probeName)
		}
		created, err := stableCurrentFamilyMaterialize(snapshot, ref, probe.Desired)
		if err != nil {
			return fmt.Errorf("current non-Edge probe %s create desired: %w", probeName, err)
		}
		stateKey := key + "\x00" + probeName
		if _, exists := live[stateKey]; exists {
			return fmt.Errorf("current non-Edge probe %s create repeated", probeName)
		}
		live[stateKey] = stableCurrentFamilyState{
			ref: ref, definition: append([]byte(nil), definitionRaw...), desired: append([]byte(nil), created...),
		}

		read, exists := live[stateKey]
		if !exists || read.ref != ref || !bytes.Equal(read.definition, definitionRaw) || !bytes.Equal(read.desired, created) {
			return fmt.Errorf("current non-Edge probe %s read did not return exact created state", probeName)
		}

		if _, supportsUpdate := capabilities["update"]; supportsUpdate {
			updatedInput := stableCurrentFamilyUpdatedDesired(probe.Desired)
			updated, err := stableCurrentFamilyMaterialize(snapshot, ref, updatedInput)
			if err != nil {
				return fmt.Errorf("current non-Edge probe %s update desired: %w", probeName, err)
			}
			if bytes.Equal(updated, created) {
				return fmt.Errorf("current non-Edge probe %s declared update but adapter produced no changed state", probeName)
			}
			state := live[stateKey]
			state.desired = append([]byte(nil), updated...)
			live[stateKey] = state
			read, exists = live[stateKey]
			if !exists || read.ref != ref || !bytes.Equal(read.definition, definitionRaw) || !bytes.Equal(read.desired, updated) {
				return fmt.Errorf("current non-Edge probe %s read did not return exact updated state", probeName)
			}
		} else {
			// A Form without update declares that no update transition exists;
			// the adapter proves the declaration by leaving the created bytes
			// untouched and continuing to the delete transition.
			if !bytes.Equal(live[stateKey].desired, created) {
				return fmt.Errorf("current non-Edge probe %s changed state without update capability", probeName)
			}
		}

		delete(live, stateKey)
		if _, exists := live[stateKey]; exists {
			return fmt.Errorf("current non-Edge probe %s delete left state", probeName)
		}
	}
	if len(seen) != len(candidateByRef) || len(live) != 0 {
		return fmt.Errorf("current non-Edge lifecycle covered %d/%d exact candidates and left %d states", len(seen), len(candidateByRef), len(live))
	}
	return nil
}

type stableCurrentFamilyState struct {
	ref        formpackage.FormRef
	definition []byte
	desired    []byte
}

func stableCurrentFamilyMaterialize(snapshot *currentformsnapshot.Snapshot, ref formpackage.FormRef, desired map[string]any) ([]byte, error) {
	raw, err := json.Marshal(desired)
	if err != nil {
		return nil, fmt.Errorf("encode desired document: %w", err)
	}
	materialized, err := snapshot.Materialize(ref, raw)
	if err != nil {
		return nil, err
	}
	if err := snapshot.Validate(ref, materialized); err != nil {
		return nil, fmt.Errorf("validate materialized desired document: %w", err)
	}
	return materialized, nil
}

func stableCurrentFamilyUpdatedDesired(input map[string]any) map[string]any {
	// The current Table Definition exposes ttlAttribute as mutable. Keep this
	// tiny mutation data-only and deterministic; no family implementation or
	// semantic hook is consulted.
	raw, err := json.Marshal(input)
	if err != nil {
		return cloneJSONMap(input)
	}
	var output map[string]any
	if err := formpackage.DecodeStrictIJSON(raw, &output); err != nil {
		return cloneJSONMap(input)
	}
	if current, ok := output["ttlAttribute"].(string); ok && current != "updatedAt" {
		output["ttlAttribute"] = "updatedAt"
	} else {
		output["ttlAttribute"] = "expiresAt"
	}
	return output
}

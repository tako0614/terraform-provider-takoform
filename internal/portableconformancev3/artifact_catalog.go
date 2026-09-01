package portableconformancev3

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const providerPublisherClosureFormat = "takoform.provider-publisher-set-artifact-closure@v1"

type artifactCatalogClosure struct {
	Format     string `json:"format"`
	Projection struct {
		Path   string `json:"path"`
		Digest string `json:"digest"`
	} `json:"projection"`
	Packages []struct {
		Root          string              `json:"root"`
		FormRef       formpackage.FormRef `json:"formRef"`
		PackageDigest string              `json:"packageDigest"`
	} `json:"packages"`
	Interfaces []struct {
		Path string                   `json:"path"`
		Ref  formpackage.InterfaceRef `json:"ref"`
	} `json:"interfaces"`
	Bindings []struct {
		Path string                 `json:"path"`
		Ref  formpackage.BindingRef `json:"ref"`
	} `json:"bindings"`
}

// LoadArtifactFamilyCatalog constructs a reference-host catalog from one
// verified publisher artifact closure. It is deliberately separate from
// LoadCatalog: retained conformance corpora keep testing their immutable Form
// set, while development authoring tests must exercise the exact Forms the
// current Provider maps.
func LoadArtifactFamilyCatalog(repoRoot string, contract Contract, artifactRoot string) (*Catalog, error) {
	resolvedRoot, err := repositoryRootForContract(repoRoot)
	if err != nil {
		return nil, err
	}
	root, err := containedArtifactPath(resolvedRoot, artifactRoot)
	if err != nil {
		return nil, fmt.Errorf("takoform: publisher artifact root: %w", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("takoform: publisher artifact root: %w", err)
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("takoform: publisher artifact root is not a real directory")
	}

	closureRaw, err := readArtifactFile(root, "closure.json")
	if err != nil {
		return nil, err
	}
	var closure artifactCatalogClosure
	if err := formpackage.DecodeStrictIJSON(closureRaw, &closure); err != nil {
		return nil, fmt.Errorf("takoform: decode publisher artifact closure: %w", err)
	}
	if closure.Format != providerPublisherClosureFormat {
		return nil, fmt.Errorf("takoform: publisher artifact closure format %q is not %q", closure.Format, providerPublisherClosureFormat)
	}
	if len(closure.Packages) == 0 {
		return nil, errors.New("takoform: publisher artifact closure contains no Form Packages")
	}

	catalog := newCatalog(contract.APIVersion)
	seenPackageRoots := make(map[string]struct{}, len(closure.Packages))
	for _, entry := range closure.Packages {
		if _, duplicate := seenPackageRoots[entry.Root]; duplicate {
			return nil, fmt.Errorf("takoform: duplicate publisher package root %q", entry.Root)
		}
		seenPackageRoots[entry.Root] = struct{}{}
		packageRoot, err := containedArtifactPath(root, entry.Root)
		if err != nil {
			return nil, fmt.Errorf("takoform: publisher package root %q: %w", entry.Root, err)
		}
		report, err := formpackage.VerifyDirectory(packageRoot)
		if err != nil {
			return nil, fmt.Errorf("takoform: verify publisher package %q: %w", entry.Root, err)
		}
		verified, ok := report.VerifiedPackage()
		if !ok {
			return nil, fmt.Errorf("takoform: publisher package %q produced no verified capability", entry.Root)
		}
		if verified.FormRef() != entry.FormRef || verified.PackageDigest() != entry.PackageDigest {
			return nil, fmt.Errorf("takoform: publisher package %q disagrees with its closure identity", entry.Root)
		}
		definition, err := formpackage.ValidateDefinition(verified.Definition())
		if err != nil {
			return nil, fmt.Errorf("takoform: publisher package %q definition: %w", entry.Root, err)
		}
		if catalog.family == "" {
			catalog.family = entry.FormRef.APIVersion
		} else if catalog.family != entry.FormRef.APIVersion {
			return nil, fmt.Errorf("takoform: publisher closure mixes Form families %q and %q", catalog.family, entry.FormRef.APIVersion)
		}
		form := &InstalledForm{
			Ref: FormRef{
				APIVersion: entry.FormRef.APIVersion, Kind: entry.FormRef.Kind,
				DefinitionVersion: entry.FormRef.DefinitionVersion, SchemaDigest: entry.FormRef.SchemaDigest,
			},
			PackageDigest:      entry.PackageDigest,
			Role:               definition.Role,
			Title:              definition.Title,
			Description:        definition.Description,
			DesiredSchema:      definition.DesiredSchema,
			OutputSchema:       definition.OutputSchema,
			Lifecycle:          definition.LifecycleCapabilities,
			ProvidedInterfaces: definition.ProvidedInterfaces,
			AcceptedBindings:   definition.AcceptedBindings,
			RequiresHostAPI:    definition.RequiresHostAPI,
			Constraints:        definition.Constraints,
		}
		if err := catalog.install(form); err != nil {
			return nil, err
		}
	}
	if err := catalog.requireEnforceableFamily(); err != nil {
		return nil, err
	}

	for _, entry := range closure.Interfaces {
		raw, err := readArtifactFile(root, entry.Path)
		if err != nil {
			return nil, err
		}
		if err := formpackage.ValidateInterfaceDefinition(raw); err != nil {
			return nil, fmt.Errorf("takoform: interface %q: %w", entry.Path, err)
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("takoform: interface %q digest: %w", entry.Path, err)
		}
		var document interfaceDefinitionDocument
		if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
			return nil, fmt.Errorf("takoform: interface %q: %w", entry.Path, err)
		}
		actualRef := formpackage.InterfaceRef{
			APIVersion: document.APIVersion, Name: document.Name,
			Version: document.Version, SchemaDigest: digest,
		}
		if actualRef != entry.Ref {
			return nil, fmt.Errorf("takoform: interface %q disagrees with its closure identity", entry.Path)
		}
		key := document.Name + "@" + document.Version
		if _, duplicate := catalog.abis[key]; duplicate {
			return nil, fmt.Errorf("takoform: duplicate publisher interface %s", key)
		}
		catalog.interfaces[key] = supportRef{Name: document.Name, Version: document.Version, SchemaDigest: digest}
		catalog.abis[key] = interfaceContract{Ref: actualRef, Handlers: runtimeHandlerVocabulary(document)}
	}

	for _, entry := range closure.Bindings {
		raw, err := readArtifactFile(root, entry.Path)
		if err != nil {
			return nil, err
		}
		if err := formpackage.ValidateBindingDefinition(raw); err != nil {
			return nil, fmt.Errorf("takoform: binding %q: %w", entry.Path, err)
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("takoform: binding %q digest: %w", entry.Path, err)
		}
		var document bindingDefinitionDocument
		if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
			return nil, fmt.Errorf("takoform: binding %q: %w", entry.Path, err)
		}
		actualRef := formpackage.BindingRef{
			APIVersion: document.APIVersion, Name: document.Name,
			Version: document.Version, SchemaDigest: digest,
		}
		if actualRef != entry.Ref {
			return nil, fmt.Errorf("takoform: binding %q disagrees with its closure identity", entry.Path)
		}
		key := document.Name + "@" + document.Version
		if _, duplicate := catalog.contracts[key]; duplicate {
			return nil, fmt.Errorf("takoform: duplicate publisher binding %s", key)
		}
		catalog.bindings[key] = supportRef{Name: document.Name, Version: document.Version, SchemaDigest: digest}
		catalog.contracts[key] = bindingContract{
			Ref: actualRef, SourceRole: document.SourceRole,
			TargetInterface:    document.TargetInterface,
			AllowedTargetForms: document.AllowedTargetForms,
		}
	}

	for _, form := range catalog.Forms {
		for _, ref := range form.ProvidedInterfaces {
			installed, ok := catalog.abis[ref.Name+"@"+ref.Version]
			if !ok || installed.Ref != ref {
				return nil, fmt.Errorf("takoform: %s provides an interface absent from the publisher closure", form.Ref.Kind)
			}
		}
		for _, ref := range form.AcceptedBindings {
			installed, ok := catalog.contracts[ref.Name+"@"+ref.Version]
			if !ok || installed.Ref != ref {
				return nil, fmt.Errorf("takoform: %s accepts a binding absent from the publisher closure", form.Ref.Kind)
			}
		}
	}
	return catalog, nil
}

func containedArtifactPath(root, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") || path.Clean(relative) != relative || relative == "." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("unsafe relative path %q", relative)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(rootAbs, filepath.FromSlash(relative))
	rel, err := filepath.Rel(rootAbs, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes its artifact root", relative)
	}
	return target, nil
}

func readArtifactFile(root, relative string) ([]byte, error) {
	target, err := containedArtifactPath(root, relative)
	if err != nil {
		return nil, fmt.Errorf("takoform: publisher artifact %q: %w", relative, err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		return nil, fmt.Errorf("takoform: publisher artifact %q: %w", relative, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("takoform: publisher artifact %q is not a regular file", relative)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("takoform: publisher artifact %q: %w", relative, err)
	}
	return raw, nil
}

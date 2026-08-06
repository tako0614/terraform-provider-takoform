package portableconformancev3

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// installedForm is one exact FormRef the reference host serves, together
// with the desired-state contract it validates against.
type installedForm struct {
	Ref           FormRef
	PackageDigest string
	Role          string
	Title         string
	Description   string
	DesiredSchema map[string]any
	Lifecycle     []string
	compiled      *jsonschema.Schema
}

func (form *installedForm) operations() []string {
	if len(form.Lifecycle) > 0 {
		return append([]string(nil), form.Lifecycle...)
	}
	return []string{"create", "read", "update", "delete", "import", "observe", "refresh"}
}

// supportRef is one interface or binding contract the host declares support
// for.
type supportRef struct {
	Name         string
	Version      string
	SchemaDigest string
}

// Catalog is the reference host's installed set: Form definitions plus the
// interface and binding contracts it can profile.
type Catalog struct {
	forms      map[string]*installedForm // formKey(group, kind)
	interfaces map[string]supportRef     // name@version
	bindings   map[string]supportRef     // name@version
}

func formKey(group, kind string) string { return group + "\x00" + kind }

func (catalog *Catalog) form(group, kind string) *installedForm {
	return catalog.forms[formKey(group, kind)]
}

type candidateSet struct {
	Format            string `json:"format"`
	Family            string `json:"family"`
	PackageAPIVersion string `json:"packageApiVersion"`
	PublicationStatus string `json:"publicationStatus"`
	AuthoringSource   string `json:"authoringSource"`
	AuthoringPolicy   string `json:"authoringPolicy"`
	Forms             []struct {
		Kind          string  `json:"kind"`
		Role          string  `json:"role"`
		Path          string  `json:"path"`
		FormRef       FormRef `json:"formRef"`
		PackageDigest string  `json:"packageDigest"`
	} `json:"forms"`
}

type interfaceCandidateSet struct {
	Format            string `json:"format"`
	PublicationStatus string `json:"publicationStatus"`
	AuthoringSource   string `json:"authoringSource"`
	Interfaces        []struct {
		Name         string `json:"name"`
		Version      string `json:"version"`
		SchemaDigest string `json:"schemaDigest"`
	} `json:"interfaces"`
}

type bindingCandidateSet struct {
	Format            string `json:"format"`
	PublicationStatus string `json:"publicationStatus"`
	AuthoringSource   string `json:"authoringSource"`
	Bindings          []struct {
		Name         string `json:"name"`
		Version      string `json:"version"`
		SchemaDigest string `json:"schemaDigest"`
	} `json:"bindings"`
}

// LoadCatalog reads the real Edge Family candidate definitions and the
// interface/binding candidate sets from a repository root. The install set
// identity comes from the generated registry (currentformregistry.V3All), so
// registry/candidate drift fails closed.
func LoadCatalog(repoRoot string, contract Contract) (*Catalog, error) {
	catalog := &Catalog{
		forms:      map[string]*installedForm{},
		interfaces: map[string]supportRef{},
		bindings:   map[string]supportRef{},
	}
	var set candidateSet
	setPath := filepath.Join(repoRoot, "forms", "candidates", "edge", "v1alpha1", "candidate-set.json")
	if err := decodeStrictFile(setPath, &set); err != nil {
		return nil, err
	}
	registry := currentformregistry.V3All()
	for _, candidate := range set.Forms {
		registered, ok := registry[candidate.FormRef.APIVersion+"/"+candidate.Kind]
		if !ok {
			return nil, fmt.Errorf("takoform: candidate %s is not in the generated v3 registry", candidate.Kind)
		}
		if registered.SchemaDigest != candidate.FormRef.SchemaDigest ||
			registered.PackageDigest != candidate.PackageDigest ||
			registered.DefinitionVersion != candidate.FormRef.DefinitionVersion {
			return nil, fmt.Errorf("takoform: candidate %s drifted from the generated v3 registry", candidate.Kind)
		}
		definitionPath := filepath.Join(repoRoot, filepath.FromSlash(candidate.Path), "definition.json")
		raw, err := os.ReadFile(definitionPath)
		if err != nil {
			return nil, err
		}
		definition, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			return nil, fmt.Errorf("takoform: candidate %s definition: %w", candidate.Kind, err)
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return nil, err
		}
		if digest != candidate.FormRef.SchemaDigest {
			return nil, fmt.Errorf("takoform: candidate %s definition digest drifted", candidate.Kind)
		}
		form := &installedForm{
			Ref:           candidate.FormRef,
			PackageDigest: candidate.PackageDigest,
			Role:          definition.Role,
			Title:         definition.Title,
			Description:   definition.Description,
			DesiredSchema: definition.DesiredSchema,
			Lifecycle:     definition.LifecycleCapabilities,
		}
		if err := form.compileDesiredSchema(); err != nil {
			return nil, err
		}
		catalog.forms[formKey(form.Ref.APIVersion, form.Ref.Kind)] = form
	}
	if len(catalog.forms) != len(registry) {
		return nil, errors.New("takoform: candidate set does not cover the generated v3 registry")
	}
	if err := catalog.installSyntheticSecondGroup(contract); err != nil {
		return nil, err
	}
	var interfaces interfaceCandidateSet
	interfacesPath := filepath.Join(repoRoot, "interfaces", "candidates", "v1alpha1", "candidate-set.json")
	if err := decodeStrictFile(interfacesPath, &interfaces); err != nil {
		return nil, err
	}
	for _, candidate := range interfaces.Interfaces {
		catalog.interfaces[candidate.Name+"@"+candidate.Version] = supportRef(candidate)
	}
	var bindings bindingCandidateSet
	bindingsPath := filepath.Join(repoRoot, "bindings", "candidates", "v1alpha1", "candidate-set.json")
	if err := decodeStrictFile(bindingsPath, &bindings); err != nil {
		return nil, err
	}
	for _, candidate := range bindings.Bindings {
		catalog.bindings[candidate.Name+"@"+candidate.Version] = supportRef(candidate)
	}
	return catalog, nil
}

// FallbackCatalog constructs minimal in-memory definitions for the five
// probe kinds plus the synthetic second group. It exists so targeted unit
// tests can run without a repository checkout; SelfTest uses the real
// LoadCatalog.
func FallbackCatalog(contract Contract) (*Catalog, error) {
	catalog := &Catalog{
		forms:      map[string]*installedForm{},
		interfaces: map[string]supportRef{},
		bindings:   map[string]supportRef{},
	}
	closedName := map[string]any{
		"type": "string", "minLength": 1, "maxLength": 63,
		"pattern": "^[a-z][a-z0-9-]{0,62}$",
	}
	typedRef := func(kind string) map[string]any {
		return map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"kind", "name"},
			"properties": map[string]any{
				"kind": map[string]any{"const": kind},
				"name": closedName,
			},
		}
	}
	bindingList := func(kind string) map[string]any {
		return map[string]any{
			"type": "array", "uniqueItems": true,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []any{"name", "resource"},
				"properties": map[string]any{
					"name":     map[string]any{"type": "string", "pattern": "^[A-Za-z_$][A-Za-z0-9_$]*$"},
					"resource": typedRef(kind),
				},
			},
		}
	}
	emptyObject := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{}}
	schemas := []struct {
		probe  ResourceProbe
		role   string
		schema map[string]any
	}{
		{contract.RunnerInput.ModuleWorker, "identity", emptyObject},
		{contract.RunnerInput.EdgeKvNamespace, "identity", emptyObject},
		{contract.RunnerInput.AtLeastOnceQueue, "identity", map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"deliveryDelaySeconds":    map[string]any{"type": "integer", "minimum": 0, "maximum": 43200},
				"messageRetentionSeconds": map[string]any{"type": "integer", "minimum": 60, "maximum": 1209600},
			},
		}},
		{contract.RunnerInput.WorkerBundle.ResourceProbe, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"mainModule", "modules"},
			"properties": map[string]any{
				"mainModule": map[string]any{"type": "string", "minLength": 1},
				"modules": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"items": map[string]any{
						"type": "object", "additionalProperties": false,
						"required": []any{"digest", "mediaType", "name", "size"},
						"properties": map[string]any{
							"digest":    map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
							"mediaType": map[string]any{"type": "string"},
							"name":      map[string]any{"type": "string", "minLength": 1},
							"size":      map[string]any{"type": "integer", "minimum": 0},
						},
					},
				},
			},
		}},
		{contract.RunnerInput.WorkerVersion, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"bundle", "compatibilityDate", "handlers", "worker"},
			"properties": map[string]any{
				"bundle":            typedRef("WorkerBundle"),
				"compatibilityDate": map[string]any{"type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$"},
				"handlers": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"items": map[string]any{"enum": []any{"fetch", "scheduled", "queue", "tail"}},
				},
				"kvBindings": bindingList("EdgeKVNamespace"),
				"worker":     typedRef("ModuleWorker"),
			},
		}},
	}
	revisionLifecycle := []string{"create", "read", "delete", "import", "observe"}
	for _, entry := range schemas {
		form := &installedForm{
			Ref:           entry.probe.Identity.FormRef,
			PackageDigest: entry.probe.Identity.PackageDigest,
			Role:          entry.role,
			Title:         entry.probe.Identity.FormRef.Kind,
			DesiredSchema: entry.schema,
		}
		if entry.role == "revision" {
			form.Lifecycle = append([]string(nil), revisionLifecycle...)
		}
		if err := form.compileDesiredSchema(); err != nil {
			return nil, err
		}
		catalog.forms[formKey(form.Ref.APIVersion, form.Ref.Kind)] = form
	}
	if err := catalog.installSyntheticSecondGroup(contract); err != nil {
		return nil, err
	}
	catalog.interfaces[contract.RunnerInput.SupportProbes.Interface.Name+"@"+contract.RunnerInput.SupportProbes.Interface.Version] = supportRef{
		Name:         contract.RunnerInput.SupportProbes.Interface.Name,
		Version:      contract.RunnerInput.SupportProbes.Interface.Version,
		SchemaDigest: formpackage.DigestBytes([]byte("fallback-interface")),
	}
	catalog.bindings[contract.RunnerInput.SupportProbes.Binding.Name+"@"+contract.RunnerInput.SupportProbes.Binding.Version] = supportRef{
		Name:         contract.RunnerInput.SupportProbes.Binding.Name,
		Version:      contract.RunnerInput.SupportProbes.Binding.Version,
		SchemaDigest: formpackage.DigestBytes([]byte("fallback-binding")),
	}
	return catalog, nil
}

// installSyntheticSecondGroup installs the contract's synthetic second-group
// EdgeKVNamespace so one kind name exists in two namespaced groups. It has
// no package digest: it exists only to prove group-scoped identity.
func (catalog *Catalog) installSyntheticSecondGroup(contract Contract) error {
	ref := contract.RunnerInput.SyntheticSecondGroup
	form := &installedForm{
		Ref:         ref,
		Role:        "identity",
		Title:       "Synthetic second-group " + ref.Kind,
		Description: "Conformance-only install proving one kind name in two groups.",
		DesiredSchema: map[string]any{
			"type": "object", "additionalProperties": false, "properties": map[string]any{},
		},
	}
	if err := form.compileDesiredSchema(); err != nil {
		return err
	}
	catalog.forms[formKey(ref.APIVersion, ref.Kind)] = form
	return nil
}

func (form *installedForm) compileDesiredSchema() error {
	if len(form.DesiredSchema) == 0 {
		return fmt.Errorf("takoform: installed form %s has no desired schema", form.Ref.Kind)
	}
	raw, err := json.Marshal(form.DesiredSchema)
	if err != nil {
		return fmt.Errorf("takoform: encode %s desired schema: %w", form.Ref.Kind, err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("takoform: normalize %s desired schema: %w", form.Ref.Kind, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	id := "https://forms.takoform.com/portable-host-v3/" + form.Ref.Kind + ".desired.schema.json"
	if err := compiler.AddResource(id, document); err != nil {
		return fmt.Errorf("takoform: compile %s desired schema: %w", form.Ref.Kind, err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		return fmt.Errorf("takoform: compile %s desired schema: %w", form.Ref.Kind, err)
	}
	form.compiled = compiled
	return nil
}

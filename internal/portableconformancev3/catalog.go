package portableconformancev3

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// ExactFormKey is the WHOLE Form identity a host answers for: the namespaced
// group, the kind, the definition version, and the digest of the definition
// bytes. It is the catalog's key because a host answers for the exact ref
// recorded in a client's state, never for a kind (decision 0022).
//
// The package digest is deliberately absent. It is distribution provenance and
// never identity, so a host that installed the same FormRef from a different
// legitimate package answers about the same Form (decision 0011).
type ExactFormKey struct {
	APIVersion        string
	Kind              string
	DefinitionVersion string
	SchemaDigest      string
}

func exactFormKey(ref FormRef) ExactFormKey {
	return ExactFormKey{
		APIVersion:        ref.APIVersion,
		Kind:              ref.Kind,
		DefinitionVersion: ref.DefinitionVersion,
		SchemaDigest:      ref.SchemaDigest,
	}
}

// String renders the identity for a refusal message.
func (key ExactFormKey) String() string {
	return key.APIVersion + " " + key.Kind + "@" + key.DefinitionVersion + " " + key.SchemaDigest
}

// InstalledForm is one exact FormRef the reference host serves, together
// with the desired-state contract it validates against.
type InstalledForm struct {
	Ref           FormRef
	PackageDigest string
	Role          string
	Title         string
	Description   string
	DesiredSchema map[string]any
	// OutputSchema is the Form's declared `status.outputs` contract, or nil when
	// it publishes none. Its presence is what decides whether a response carries
	// `status.outputs` at all, so a host reads it rather than deciding per kind.
	OutputSchema map[string]any
	Lifecycle    []string
	// ProvidedInterfaces and AcceptedBindings are the exact digest-bound
	// contracts the installed Definition declares. A binding is verified
	// against them, never assumed.
	ProvidedInterfaces []formpackage.InterfaceRef
	AcceptedBindings   []formpackage.BindingRef
	// EnforcedFamily is the family generation whose cross-resource semantics
	// this host enforces, stamped at install time. A Form of any OTHER group —
	// the corpus installs one deliberately — carries a different value here
	// and is skipped by every family guard, which is what makes "this host
	// enforces one family's semantics" a fact about the installed set rather
	// than about a compiled-in constant.
	EnforcedFamily string
	// RequiresHostAPI is the earliest Host API lane this Form's contract needs
	// (decision 0047). A host serving an earlier lane refuses to install it:
	// installing a Form whose rules the served protocol does not have means
	// accepting desired state nothing will ever enforce.
	RequiresHostAPI string
	// Constraints is the Definition's closed constraint list — the rules about
	// resources this Form declares. A host reads them here rather than out of
	// the desired schema's extension slots (decision 0049).
	Constraints []formpackage.FormConstraint
	// Relations are DERIVED from DesiredSchema at install time, never read
	// from a wire member. A host has the desired schema by construction, so
	// deriving costs nothing and removes the second source of truth a declared
	// relation list would create (decision 0014).
	Relations []currentformmodel.Relation
	compiled  *jsonschema.Schema
}

// acceptedBinding resolves one accepted BindingRef by contract name. The
// desired schema names the contract; the Definition carries its exact digest.
func (form *InstalledForm) acceptedBinding(name string) (formpackage.BindingRef, bool) {
	for _, ref := range form.AcceptedBindings {
		if ref.Name == name {
			return ref, true
		}
	}
	return formpackage.BindingRef{}, false
}

// providesInterface reports whether this Form declares the exact Interface
// contract a Binding requires of its target.
func (form *InstalledForm) providesInterface(want formpackage.InterfaceRef) bool {
	for _, provided := range form.ProvidedInterfaces {
		if provided.Name == want.Name && provided.Version == want.Version &&
			provided.SchemaDigest == want.SchemaDigest {
			return true
		}
	}
	return false
}

// deriveRelations computes and caches the Form's cross-resource relations.
func (form *InstalledForm) deriveRelations() error {
	// The constraint list is part of the derivation now: an exclusive hold is
	// declared there, not annotated on the schema node.
	constraints := make([]currentformmodel.Constraint, 0, len(form.Constraints))
	for _, entry := range form.Constraints {
		constraints = append(constraints, currentformmodel.Constraint{
			Kind:      currentformmodel.ConstraintKind(entry.Kind),
			Reference: entry.Reference,
			KeyedBy:   entry.KeyedBy,
			List:      entry.List,
			Member:    entry.Member,
			Total:     entry.Total,
			Property:  entry.Property,
			Output:    entry.Output,
		})
	}
	relations, err := currentformmodel.DeriveRelationsWithConstraints(form.DesiredSchema, constraints)
	if err != nil {
		return fmt.Errorf("takoform: derive %s relations: %w", form.Ref.Kind, err)
	}
	form.Relations = relations
	return nil
}

// bindingContract is one installed Binding Definition, reduced to the facts a
// host must verify before it stores a binding.
type bindingContract struct {
	Ref                formpackage.BindingRef
	SourceRole         string
	TargetInterface    formpackage.InterfaceRef
	AllowedTargetForms []allowedTargetForm
}

type allowedTargetForm struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// allowsTarget reports whether an exact target Form is listed.
func (contract bindingContract) allowsTarget(group, kind string) bool {
	for _, allowed := range contract.AllowedTargetForms {
		if allowed.APIVersion == group && allowed.Kind == kind {
			return true
		}
	}
	return false
}

// bindingDefinitionDocument is the on-disk Binding Definition shape the host
// installs. Only the members a host verifies are decoded.
type bindingDefinitionDocument struct {
	APIVersion         string                   `json:"apiVersion"`
	Kind               string                   `json:"kind"`
	Name               string                   `json:"name"`
	Version            string                   `json:"version"`
	Title              string                   `json:"title"`
	Description        string                   `json:"description"`
	SourceRole         string                   `json:"sourceRole"`
	TargetInterface    formpackage.InterfaceRef `json:"targetInterface"`
	AllowedTargetForms []allowedTargetForm      `json:"allowedTargetForms"`
	BindingNameGrammar string                   `json:"bindingNameGrammar"`
	RuntimeProjection  struct {
		Operations []string `json:"operations"`
	} `json:"runtimeProjection"`
	Lifecycle struct {
		TargetDeletion string `json:"targetDeletion"`
	} `json:"lifecycle"`
}

// operations is the exact lifecycle capability set the installed Form
// Definition declares. There is deliberately no fallback: a host that cannot
// say which operations a Form supports must not guess a generous set, because
// every guessed capability is a promise a client will hold it to.
func (form *InstalledForm) operations() []string {
	return append([]string(nil), form.Lifecycle...)
}

// declaresUpdate reports whether the installed Definition permits a
// spec-changing apply on an existing resource.
func (form *InstalledForm) declaresUpdate() bool {
	for _, operation := range form.Lifecycle {
		if operation == "update" {
			return true
		}
	}
	return false
}

// materialize is the host's single entry-point normalization of one desired
// spec: the Form's declared portable defaults, then the family's canonical
// spellings. It runs before validation, digesting, storage, and echo, so the
// effective spec IS the wire spec: a client that omits a defaulted field and a
// client that writes its default produce the same specDigest and therefore the
// same generation, and so do two clients that spell one hostname two ways
// (spec/decisions/0026).
func (form *InstalledForm) materialize(spec map[string]any) map[string]any {
	return canonicalizeEdgeSpec(form.EnforcedFamily, form, currentformmodel.MaterializeDefaults(form.DesiredSchema, spec))
}

// supportRef is one interface or binding contract the host declares support
// for.
type supportRef struct {
	Name         string
	Version      string
	SchemaDigest string
}

// interfaceDefinitionDocument is the on-disk Interface Definition shape the
// host installs. Only the members a host verifies are decoded; the runtime ABI
// contract is the one that carries an operation set a host must hold a
// declaration to (spec/decisions/0019).
type interfaceDefinitionDocument struct {
	APIVersion  string `json:"apiVersion"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Operations  []struct {
		Name         string         `json:"name"`
		Description  string         `json:"description"`
		InputSchema  map[string]any `json:"inputSchema"`
		OutputSchema map[string]any `json:"outputSchema"`
		Errors       []string       `json:"errors"`
		Idempotent   bool           `json:"idempotent"`
	} `json:"operations"`
	Semantics map[string]string `json:"semantics"`
	Limits    map[string]any    `json:"limits"`
	Fixtures  []struct {
		Name  string `json:"name"`
		Steps []struct {
			Operation     string         `json:"operation"`
			Input         map[string]any `json:"input"`
			Expected      map[string]any `json:"expected"`
			ExpectedError string         `json:"expectedError"`
		} `json:"steps"`
	} `json:"fixtures"`
}

// interfaceContract is one installed Interface Definition reduced to the facts
// a host verifies. Handlers is the closed module-handler vocabulary the
// runtime ABI publishes through its loadModule operation; it is empty for every
// Interface that declares no such operation.
type interfaceContract struct {
	Ref      formpackage.InterfaceRef
	Handlers []string
}

// runtimeHandlerVocabulary reads the handler set out of an installed Interface
// Definition exactly where the contract states it: the item enum of the
// loadModule operation's declaredHandlers property. A host learns which module
// handlers the ABI defines from the contract itself, never from a list of its
// own (spec/decisions/0019).
func runtimeHandlerVocabulary(document interfaceDefinitionDocument) []string {
	for _, operation := range document.Operations {
		if operation.Name != runtimeHandlerOperation {
			continue
		}
		properties, _ := operation.InputSchema["properties"].(map[string]any)
		property, _ := properties[runtimeHandlerProperty].(map[string]any)
		items, _ := property["items"].(map[string]any)
		values, _ := items["enum"].([]any)
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

// Catalog is the reference host's installed set: Form definitions plus the
// interface and binding contracts it can profile.
//
// Forms is keyed by the EXACT identity, not by group and kind. A catalog keyed
// by kind can hold one contract per Form line at a time, so the day a Form's
// definition version moves, every resource already recorded under the previous
// one becomes unaddressable — and a host that answered such a request by kind
// would answer it about a different contract, successfully, with nothing
// downstream able to tell (decision 0022).
type Catalog struct {
	// served is the Host API lane this catalog's host speaks. It decides
	// which Forms may be installed at all: a Form whose contract needs a
	// later lane would be accepted into desired state nothing enforces.
	served string
	// family is the namespaced Form Family generation this catalog installed,
	// READ from the candidate set rather than compiled in. A host whose
	// cross-resource semantics were keyed to a constant would load a
	// different generation, pass every structural check, and then enforce
	// nothing — every one of those guards is a silent early return.
	family string
	Forms  map[ExactFormKey]*InstalledForm
	// lines indexes group+kind+definitionVersion onto the one exact key that
	// names it. It exists for the support surface, whose published path carries
	// no schemaDigest segment; installing two definitions that agree on the
	// version and differ on the digest is refused, so the index is exact.
	lines      map[string]ExactFormKey
	interfaces map[string]supportRef        // name@version
	bindings   map[string]supportRef        // name@version
	contracts  map[string]bindingContract   // name@version
	abis       map[string]interfaceContract // name@version
}

func newCatalog(served string) *Catalog {
	return &Catalog{
		served:     served,
		Forms:      map[ExactFormKey]*InstalledForm{},
		lines:      map[string]ExactFormKey{},
		interfaces: map[string]supportRef{},
		bindings:   map[string]supportRef{},
		contracts:  map[string]bindingContract{},
		abis:       map[string]interfaceContract{},
	}
}

// install compiles one Form, derives its relations, and files it under its
// exact identity.
//
// Two installs of the SAME exact key are a duplicate; two definitions that
// agree on group, kind, and definitionVersion while differing on schemaDigest
// are an identity conflict, because a definition version names one set of
// definition bytes and a host holding two of them cannot say which one a
// version-addressed request means.
// laneOrder ranks the Host API lanes by publication order, which is what makes
// "at least this lane" answerable. A lane absent from it is unknown to this
// host, and an unknown REQUIREMENT is refused rather than assumed satisfied.
// The withdrawn v1beta2 and v1beta3 lanes are absent (decision 0051): they were
// never served, so no published Form can require one, and listing a rank for a
// lane that does not exist would let a corpus require it and be satisfied.
var laneOrder = map[string]int{
	"forms.takoform.com/v1beta1": 1,
	"forms.takoform.com/v1beta4": 2,
}

// satisfiesRequirement reports whether a host serving `served` may install a
// Form requiring `required`. The requirement is a LOWER BOUND (decision 0047):
// a later lane serves an earlier requirement, and the lane's own compatibility
// obligation is what makes that true.
func satisfiesRequirement(served, required string) bool {
	if required == "" {
		return false
	}
	servedRank, known := laneOrder[served]
	if !known {
		return false
	}
	requiredRank, known := laneOrder[required]
	if !known {
		return false
	}
	return servedRank >= requiredRank
}

// requireEnforceableFamily proves the installed generation is one this host
// has cross-resource semantics for. The kinds below are the ones every family
// guard keys on; a generation that renamed or dropped them all is a generation
// this build cannot enforce, and saying so is the difference between a host
// that refuses and a host that silently permits.
func (catalog *Catalog) requireEnforceableFamily() error {
	if catalog.family == "" {
		return errors.New("takoform: the catalog installed Forms before it knew which family it enforces")
	}
	enforced := []string{
		workerVersionKind, workerDeploymentKind, queueConsumerKind,
		workerCustomDomainKind, sqliteMigrationApplicationKind,
	}
	for _, form := range catalog.Forms {
		// The stamp is what every guard reads. An unstamped Form is one the
		// guards will skip, so an empty one is a silent loss of semantics
		// rather than a Form with nothing to enforce.
		if form.EnforcedFamily == "" {
			return fmt.Errorf("takoform: installed Form %s carries no enforced family", exactFormKey(form.Ref))
		}
	}
	for _, form := range catalog.Forms {
		if form.Ref.APIVersion != catalog.family {
			continue
		}
		for _, kind := range enforced {
			if form.Ref.Kind == kind {
				return nil
			}
		}
	}
	return fmt.Errorf(
		"takoform: the installed family %s carries none of the kinds this host has semantics for, "+
			"so every cross-resource guard would silently pass",
		catalog.family,
	)
}

func (catalog *Catalog) install(form *InstalledForm) error {
	key := exactFormKey(form.Ref)
	if _, installed := catalog.Forms[key]; installed {
		return fmt.Errorf("takoform: exact Form %s is already installed", key)
	}
	line := formLineKey(form.Ref.APIVersion, form.Ref.Kind, form.Ref.DefinitionVersion)
	if existing, taken := catalog.lines[line]; taken {
		return fmt.Errorf(
			"takoform: %s %s definition version %s is already installed at digest %s",
			form.Ref.APIVersion, form.Ref.Kind, form.Ref.DefinitionVersion, existing.SchemaDigest,
		)
	}
	form.EnforcedFamily = catalog.family
	// A Form states the substrate it needs, and a host that cannot provide it
	// refuses the Form rather than serving desired state it has no rules for
	// (decision 0047). An absent declaration is not a hole: the Definition
	// profile requires the member, so a published Form always carries one, and
	// the corpus's hand-authored Definitions carry one for the same reason.
	if form.RequiresHostAPI != "" && !satisfiesRequirement(catalog.served, form.RequiresHostAPI) {
		return fmt.Errorf(
			"takoform: %s requires Host API %s and this host serves %s",
			key, form.RequiresHostAPI, catalog.served,
		)
	}
	if err := form.compileDesiredSchema(); err != nil {
		return err
	}
	if err := form.deriveRelations(); err != nil {
		return err
	}
	catalog.Forms[key] = form
	catalog.lines[line] = key
	return nil
}

// exact resolves one installed Form by the WHOLE identity. Nothing in this
// package resolves a Form any other way: a nil answer is `form_unknown`, never
// a reason to look for the same kind under a different contract.
func (catalog *Catalog) exact(ref FormRef) *InstalledForm {
	return catalog.Forms[exactFormKey(ref)]
}

// line resolves the one exact identity a group, kind, and definition version
// name. It serves the support surface's published path shape and nothing else.
func (catalog *Catalog) line(group, kind, definitionVersion string) *InstalledForm {
	key, known := catalog.lines[formLineKey(group, kind, definitionVersion)]
	if !known {
		return nil
	}
	return catalog.Forms[key]
}

// onlyLine resolves the single installed line of one group and kind, or nil
// when none or more than one is installed. A caller that means "this kind's
// contract" must not hard-code a definition version: a member whose contract
// diverged from its generation line does not carry that line's version
// (decision 0046), and a caller that guessed would silently miss it.
func (catalog *Catalog) onlyLine(group, kind string) *InstalledForm {
	var found *InstalledForm
	for _, form := range catalog.sortedForms() {
		if form.Ref.APIVersion != group || form.Ref.Kind != kind {
			continue
		}
		if found != nil {
			return nil
		}
		found = form
	}
	return found
}

// sortedForms lists every installed Form in one stable exact-identity order.
func (catalog *Catalog) sortedForms() []*InstalledForm {
	keys := make([]ExactFormKey, 0, len(catalog.Forms))
	for key := range catalog.Forms {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	out := make([]*InstalledForm, 0, len(keys))
	for _, key := range keys {
		out = append(out, catalog.Forms[key])
	}
	return out
}

func formLineKey(group, kind, definitionVersion string) string {
	return group + "\x00" + kind + "\x00" + definitionVersion
}

// interfaceContractByName resolves one installed Interface Definition.
func (catalog *Catalog) interfaceContractByName(name, version string) (interfaceContract, bool) {
	contract, ok := catalog.abis[name+"@"+version]
	return contract, ok
}

// bindingContractByName resolves the installed contract behind one accepted
// BindingRef. A host that has not installed the contract cannot verify it and
// must not accept a binding that names it.
func (catalog *Catalog) bindingContractByName(name, version string) (bindingContract, bool) {
	contract, ok := catalog.contracts[name+"@"+version]
	return contract, ok
}

// baseLifecycleCapabilities is the closed v1beta1 capability set of a Form
// with nothing mutable to change. It carries no refresh: the lane has no such
// operation.
func baseLifecycleCapabilities() []string {
	return []string{"create", "read", "delete", "import", "observe"}
}

func lifecycleCapabilitiesWithUpdate() []string {
	return []string{"create", "read", "update", "delete", "import", "observe"}
}

type candidateSet struct {
	Format            string `json:"format"`
	Family            string `json:"family"`
	FormMaturity      string `json:"formMaturity"`
	PackageAPIVersion string `json:"packageApiVersion"`
	PublicationStatus string `json:"publicationStatus"`
	AuthoringSource   string `json:"authoringSource"`
	AuthoringPolicy   string `json:"authoringPolicy"`
	Forms             []struct {
		Kind          string  `json:"kind"`
		Role          string  `json:"role"`
		ResourceType  string  `json:"resourceType"`
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
// identity comes from the generated registry (currentformregistry.V3Current),
// so registry/candidate drift fails closed.
func LoadCatalog(repoRoot string, contract Contract) (*Catalog, error) {
	catalog := newCatalog(contract.APIVersion)
	var set candidateSet
	if contract.FamilyCandidateSet == "" {
		return nil, errors.New("takoform: the corpus does not say which Form Family generation it installs")
	}
	setPath := filepath.Join(repoRoot, filepath.FromSlash(contract.FamilyCandidateSet), "candidate-set.json")
	if err := decodeStrictFile(setPath, &set); err != nil {
		return nil, err
	}
	// Before anything installs: every installed Form is stamped with the
	// family this host enforces, so recording it after the install loop would
	// stamp them all empty and silence every guard.
	catalog.family = set.Family
	if set.FormMaturity != "experimental" {
		return nil, fmt.Errorf("takoform: Beta family maturity is %q, want experimental", set.FormMaturity)
	}
	registry := currentformregistry.V3Current()
	for _, candidate := range set.Forms {
		registered, ok := registry.Lookup(currentformregistry.ExactFormKey{
			APIVersion:        candidate.FormRef.APIVersion,
			Kind:              candidate.Kind,
			DefinitionVersion: candidate.FormRef.DefinitionVersion,
			SchemaDigest:      candidate.FormRef.SchemaDigest,
		})
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
		form := &InstalledForm{
			Ref:                candidate.FormRef,
			PackageDigest:      candidate.PackageDigest,
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
	// The registry's supported set spans generations — every RELEASED ref
	// stays supported for state compatibility — while a candidate set is one
	// generation. What the host must cover is every identity of ITS
	// generation: each supported ref whose group matches the candidate
	// family. Released refs of earlier generations live in the retained
	// candidate trees, not here.
	currentFamily := set.Family
	currentSupported := 0
	for _, ref := range registry.SupportedRefs() {
		if ref.APIVersion == currentFamily {
			currentSupported++
		}
	}
	if len(catalog.Forms) != currentSupported {
		return nil, errors.New("takoform: candidate set does not cover its generation of the v3 registry")
	}
	// Fail closed on the failure this whole change exists to prevent. Every
	// family guard is a silent early return, so a host holding a family whose
	// kinds it has no semantics for would install, cover the registry, and
	// enforce nothing — passing conformance while proving nothing. If the
	// installed set contains none of the kinds the semantics are written for,
	// that is not a host with less to do; it is a host that has lost them.
	if err := catalog.requireEnforceableFamily(); err != nil {
		return nil, err
	}
	if err := catalog.installSyntheticSecondGroup(contract); err != nil {
		return nil, err
	}
	if err := catalog.installSyntheticSecondDefinitionVersion(contract); err != nil {
		return nil, err
	}
	var interfaces interfaceCandidateSet
	interfacesPath := filepath.Join(repoRoot, "interfaces", "candidates", "v1alpha1", "candidate-set.json")
	if err := decodeStrictFile(interfacesPath, &interfaces); err != nil {
		return nil, err
	}
	for _, candidate := range interfaces.Interfaces {
		catalog.interfaces[candidate.Name+"@"+candidate.Version] = supportRef(candidate)
		// The Interface Definition is installed, not just its identity. A host
		// that only knew the digest could advertise the runtime ABI it does not
		// hold, and could not read the handler vocabulary the ABI defines, so it
		// would have to keep a handler list of its own — a second source of
		// truth the contract exists to remove (spec/decisions/0019).
		var document interfaceDefinitionDocument
		path := filepath.Join(repoRoot, "interfaces", "candidates", "v1alpha1", candidate.Name, "definition.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return nil, err
		}
		if digest != candidate.SchemaDigest {
			return nil, fmt.Errorf("takoform: interface %s definition digest drifted", candidate.Name)
		}
		if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
			return nil, fmt.Errorf("takoform: interface %s definition: %w", candidate.Name, err)
		}
		if document.Name != candidate.Name || document.Version != candidate.Version {
			return nil, fmt.Errorf("takoform: interface %s definition identity drifted", candidate.Name)
		}
		catalog.abis[candidate.Name+"@"+candidate.Version] = interfaceContract{
			Ref: formpackage.InterfaceRef{
				APIVersion:   document.APIVersion,
				Name:         document.Name,
				Version:      document.Version,
				SchemaDigest: digest,
			},
			Handlers: runtimeHandlerVocabulary(document),
		}
	}
	var bindings bindingCandidateSet
	if contract.BindingCandidateSet == "" {
		return nil, errors.New("takoform: the corpus does not say which Binding envelope generation it installs")
	}
	bindingsRoot := filepath.Join(repoRoot, filepath.FromSlash(contract.BindingCandidateSet))
	bindingsPath := filepath.Join(bindingsRoot, "candidate-set.json")
	if err := decodeStrictFile(bindingsPath, &bindings); err != nil {
		return nil, err
	}
	for _, candidate := range bindings.Bindings {
		catalog.bindings[candidate.Name+"@"+candidate.Version] = supportRef(candidate)
		// The Binding Definition itself is installed, not just its identity: a
		// host that only knew the digest could not check allowedTargetForms,
		// the required target Interface, or the source role, and would have to
		// take every declared binding on trust.
		var document bindingDefinitionDocument
		path := filepath.Join(bindingsRoot, candidate.Name, "definition.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		digest, err := formpackage.DigestCanonicalJSON(raw)
		if err != nil {
			return nil, err
		}
		if digest != candidate.SchemaDigest {
			return nil, fmt.Errorf("takoform: binding %s definition digest drifted", candidate.Name)
		}
		if err := formpackage.DecodeStrictIJSON(raw, &document); err != nil {
			return nil, fmt.Errorf("takoform: binding %s definition: %w", candidate.Name, err)
		}
		if document.Name != candidate.Name || document.Version != candidate.Version {
			return nil, fmt.Errorf("takoform: binding %s definition identity drifted", candidate.Name)
		}
		catalog.contracts[candidate.Name+"@"+candidate.Version] = bindingContract{
			Ref: formpackage.BindingRef{
				APIVersion:   document.APIVersion,
				Name:         document.Name,
				Version:      document.Version,
				SchemaDigest: digest,
			},
			SourceRole:         document.SourceRole,
			TargetInterface:    document.TargetInterface,
			AllowedTargetForms: document.AllowedTargetForms,
		}
	}
	return catalog, nil
}

// FallbackCatalog constructs minimal in-memory definitions for the five
// probe kinds plus the synthetic second group. It exists so targeted unit
// tests can run without a repository checkout; SelfTest uses the real
// LoadCatalog.
func FallbackCatalog(contract Contract) (*Catalog, error) {
	catalog := newCatalog(contract.APIVersion)
	group := contract.RunnerInput.EdgeKvNamespace.Identity.FormRef.APIVersion
	catalog.family = group
	// The fallback Interface and Binding contracts are the ones the support
	// probes name, so the in-memory catalog is internally consistent: the KV
	// Form provides the Interface the binding requires, and the WorkerVersion
	// Form accepts the Binding its kvBindings list annotates.
	fallbackInterfaceRef := formpackage.InterfaceRef{
		APIVersion:   "interfaces.takoform.com/v1alpha1",
		Name:         contract.RunnerInput.SupportProbes.Interface.Name,
		Version:      contract.RunnerInput.SupportProbes.Interface.Version,
		SchemaDigest: formpackage.DigestBytes([]byte("fallback-interface")),
	}
	fallbackBindingRef := formpackage.BindingRef{
		APIVersion:   "bindings.takoform.com/v1alpha1",
		Name:         contract.RunnerInput.SupportProbes.Binding.Name,
		Version:      contract.RunnerInput.SupportProbes.Binding.Version,
		SchemaDigest: formpackage.DigestBytes([]byte("fallback-binding")),
	}
	// The fallback ModuleWorker provides the runtime ABI its Worker Version's
	// /worker reference requires, so the in-memory catalog states the same
	// dependency the real family does.
	fallbackRuntimeRef := formpackage.InterfaceRef{
		APIVersion:   "interfaces.takoform.com/v1alpha1",
		Name:         contract.RunnerInput.SupportProbes.RuntimeContract.Name,
		Version:      contract.RunnerInput.SupportProbes.RuntimeContract.Version,
		SchemaDigest: contract.RunnerInput.SupportProbes.RuntimeContract.SchemaDigest,
	}
	fallbackBindingName := fallbackBindingRef.Name
	closedName := map[string]any{
		"type": "string", "minLength": 1, "maxLength": 63,
		"pattern": "^[a-z][a-z0-9-]{0,62}$",
	}
	// Every fallback reference carries a target contract, exactly as a real
	// Form Definition does: a reference stating only a group and a kind is
	// refused at derivation, so an in-memory Form without one would not install
	// at all (decision 0022).
	typedRef := func(kind string, target map[string]any) map[string]any {
		node := map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"apiVersion", "kind", "name"},
			"properties": map[string]any{
				"apiVersion": map[string]any{"type": "string", "const": group},
				"kind":       map[string]any{"type": "string", "const": kind},
				"name":       closedName,
			},
		}
		for key, value := range target {
			node[key] = value
		}
		return node
	}
	exactFormTarget := func(ref FormRef) map[string]any {
		return map[string]any{currentformmodel.TargetFormRefsAnnotationKey: []any{map[string]any{
			"apiVersion":        ref.APIVersion,
			"kind":              ref.Kind,
			"definitionVersion": ref.DefinitionVersion,
			"schemaDigest":      ref.SchemaDigest,
		}}}
	}
	interfaceTarget := func(ref formpackage.InterfaceRef) map[string]any {
		return map[string]any{currentformmodel.RequiredInterfaceAnnotationKey: map[string]any{
			"apiVersion":   ref.APIVersion,
			"name":         ref.Name,
			"version":      ref.Version,
			"schemaDigest": ref.SchemaDigest,
		}}
	}
	bindingList := func(kind, binding string, target map[string]any) map[string]any {
		return map[string]any{
			"type": "array", "uniqueItems": true,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []any{"name", "resource"},
				"properties": map[string]any{
					"name":     map[string]any{"type": "string", "pattern": "^[A-Za-z_$][A-Za-z0-9_$]*$"},
					"resource": typedRef(kind, target),
				},
			},
			currentformmodel.BindingAnnotationKey: binding,
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
			"required": []any{"messageRetentionSeconds"},
			"properties": map[string]any{
				"deliveryDelaySeconds":    map[string]any{"type": "integer", "minimum": 0, "maximum": 43200, "default": 0},
				"messageRetentionSeconds": map[string]any{"type": "integer", "minimum": 60, "maximum": 1209600},
			},
		}},
		{contract.RunnerInput.WorkerBundle.ResourceProbe, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"manifestDigest"},
			"properties": map[string]any{
				"manifestDigest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			},
		}},
		{contract.RunnerInput.StaticAssetBundle, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"manifestDigest"},
			"properties": map[string]any{
				"manifestDigest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			},
		}},
		{contract.RunnerInput.SQLiteDatabase, "identity", emptyObject},
		{contract.RunnerInput.SQLiteMigrationSet, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"manifestDigest"},
			"properties": map[string]any{
				"manifestDigest": map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
			},
		}},
		{contract.RunnerInput.SQLiteMigrationApplication, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"database", "migrationSet"},
			"properties": map[string]any{
				"database":     typedRef("SQLiteDatabase", exactFormTarget(contract.RunnerInput.SQLiteDatabase.Identity.FormRef)),
				"migrationSet": typedRef("SQLiteMigrationSet", exactFormTarget(contract.RunnerInput.SQLiteMigrationSet.Identity.FormRef)),
			},
		}},
		{contract.RunnerInput.WorkerVersion, "revision", map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []any{"bundle", "handlers", "worker"},
			"properties": map[string]any{
				"bundle": typedRef(
					"WorkerBundle",
					exactFormTarget(contract.RunnerInput.WorkerBundle.Identity.FormRef),
				),
				"handlers": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"items": map[string]any{"enum": []any{"fetch", "scheduled", "queue"}},
				},
				"kvBindings": bindingList(
					"EdgeKVNamespace", fallbackBindingName, interfaceTarget(fallbackInterfaceRef),
				),
				"worker": typedRef("ModuleWorker", interfaceTarget(fallbackRuntimeRef)),
			},
		}},
	}
	// Capabilities are stated, never derived from the role: the fallback forms
	// mirror the real candidates, where update follows the presence of a
	// mutable desired field. Only the queue has one.
	for _, entry := range schemas {
		form := &InstalledForm{
			Ref:           entry.probe.Identity.FormRef,
			PackageDigest: entry.probe.Identity.PackageDigest,
			Role:          entry.role,
			Title:         entry.probe.Identity.FormRef.Kind,
			DesiredSchema: entry.schema,
			Lifecycle:     baseLifecycleCapabilities(),
		}
		if entry.probe.Identity.FormRef.Kind == contract.RunnerInput.AtLeastOnceQueue.Identity.FormRef.Kind {
			form.Lifecycle = lifecycleCapabilitiesWithUpdate()
		}
		if entry.probe.Identity.FormRef.Kind == contract.RunnerInput.EdgeKvNamespace.Identity.FormRef.Kind {
			form.ProvidedInterfaces = []formpackage.InterfaceRef{fallbackInterfaceRef}
		}
		if entry.probe.Identity.FormRef.Kind == contract.RunnerInput.ModuleWorker.Identity.FormRef.Kind {
			form.ProvidedInterfaces = []formpackage.InterfaceRef{fallbackRuntimeRef}
		}
		if entry.probe.Identity.FormRef.Kind == contract.RunnerInput.WorkerVersion.Identity.FormRef.Kind {
			form.AcceptedBindings = []formpackage.BindingRef{fallbackBindingRef}
		}
		if err := catalog.install(form); err != nil {
			return nil, err
		}
	}
	catalog.contracts[fallbackBindingName+"@"+fallbackBindingRef.Version] = bindingContract{
		Ref:                fallbackBindingRef,
		SourceRole:         "revision",
		TargetInterface:    fallbackInterfaceRef,
		AllowedTargetForms: []allowedTargetForm{{APIVersion: group, Kind: "EdgeKVNamespace"}},
	}
	if err := catalog.installSyntheticSecondGroup(contract); err != nil {
		return nil, err
	}
	if err := catalog.installSyntheticSecondDefinitionVersion(contract); err != nil {
		return nil, err
	}
	catalog.interfaces[fallbackInterfaceRef.Name+"@"+fallbackInterfaceRef.Version] = supportRef{
		Name:         fallbackInterfaceRef.Name,
		Version:      fallbackInterfaceRef.Version,
		SchemaDigest: fallbackInterfaceRef.SchemaDigest,
	}
	catalog.bindings[fallbackBindingRef.Name+"@"+fallbackBindingRef.Version] = supportRef{
		Name:         fallbackBindingRef.Name,
		Version:      fallbackBindingRef.Version,
		SchemaDigest: fallbackBindingRef.SchemaDigest,
	}
	return catalog, nil
}

// installSyntheticSecondGroup installs the contract's synthetic second-group
// EdgeKVNamespace so one kind name exists in two namespaced groups. It has
// no package digest: it exists only to prove group-scoped identity.
func (catalog *Catalog) installSyntheticSecondGroup(contract Contract) error {
	ref := contract.RunnerInput.SyntheticSecondGroup
	return catalog.install(&InstalledForm{
		Ref:         ref,
		Role:        "identity",
		Title:       "Synthetic second-group " + ref.Kind,
		Description: "Conformance-only install proving one kind name in two groups.",
		DesiredSchema: map[string]any{
			"type": "object", "additionalProperties": false, "properties": map[string]any{},
		},
		Lifecycle: baseLifecycleCapabilities(),
	})
}

// installSyntheticSecondDefinitionVersion installs the corpus-pinned SECOND
// definition version of one already-installed kind.
//
// It is corpus bytes rather than a Form built here, so the lane pins exactly
// what a second contract of one Form line is: the whole Definition document is
// digest-pinned, its schemaDigest is re-derived from those bytes, and the
// Definition it must NOT equal is the first version's. A host that keyed its
// catalog by group and kind cannot hold both at once, which is the property the
// exact-identity checks fail closed on (decision 0022).
//
// The two versions differ in exactly two places, and both are load-bearing: the
// definitionVersion, which makes it a second contract at all, and the
// providedInterfaces, which the corpus states as `withheldInterface` so a
// relation requiring that Interface has a live target that truthfully does not
// satisfy it.
func (catalog *Catalog) installSyntheticSecondDefinitionVersion(contract Contract) error {
	probe := contract.RunnerInput.SyntheticSecondDefinitionVersion
	definition := probe.Definition
	if definition == nil {
		return errors.New("takoform: the synthetic second definition version was not hydrated from the corpus")
	}
	existing := catalog.line(probe.FormRef.APIVersion, probe.FormRef.Kind, contract.RunnerInput.ModuleWorker.Identity.FormRef.DefinitionVersion)
	if existing == nil {
		return errors.New("takoform: the synthetic second definition version names a Form line this host has not installed")
	}
	return catalog.install(&InstalledForm{
		Ref:                probe.FormRef,
		Role:               definition.Role,
		Title:              definition.Title,
		Description:        definition.Description,
		DesiredSchema:      definition.DesiredSchema,
		Lifecycle:          definition.LifecycleCapabilities,
		ProvidedInterfaces: definition.ProvidedInterfaces,
		AcceptedBindings:   definition.AcceptedBindings,
		RequiresHostAPI:    definition.RequiresHostAPI,
		Constraints:        definition.Constraints,
	})
}

func (form *InstalledForm) compileDesiredSchema() error {
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

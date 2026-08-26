package formpackage

import "encoding/json"

const (
	// LegacyFormAPIVersion identifies the frozen pre-reset Form epoch.
	LegacyFormAPIVersion = "forms.takoform.com/v1alpha1"
	// FormAPIVersion is the retained source-compatible name for the v1alpha1
	// package and provider-v1 verifier surface. New code must select the epoch
	// explicitly instead of treating this alias as current authority.
	FormAPIVersion = LegacyFormAPIVersion
	// CurrentFormAPIVersion identifies the current Form specification epoch.
	// An epoch does not imply that any Form has reached Experimental maturity.
	CurrentFormAPIVersion = "forms.takoform.com/v1alpha2"
	// PackageAPIVersion is the retained v1alpha1 package profile. Its
	// packageVersion remains part of immutable Legacy bytes and locators.
	PackageAPIVersion = "packages.forms.takoform.com/v1alpha1"
	// LegacyContentAddressedPackageAPIVersion is the immutable v1alpha2 package
	// profile published for v1alpha1 FormRefs. It remains readable but cannot
	// carry a Form from the current epoch.
	LegacyContentAddressedPackageAPIVersion = "packages.forms.takoform.com/v1alpha2"
	// CurrentPackageAPIVersion identifies content-addressed packages for the
	// current Form epoch. Their publication locator is derived from
	// packageDigest, never a second SemVer.
	CurrentPackageAPIVersion = "packages.forms.takoform.com/v1alpha3"
	// FamilyPackageAPIVersion identifies content-addressed packages carrying
	// Form Family (namespaced-group, v1beta1 form-definition) Forms. FormRefs
	// in this lane use DNS-like groups, so there is no single Form apiVersion
	// constant; validation accepts any namespaced group outside the two frozen
	// central epochs.
	FamilyPackageAPIVersion = "packages.forms.takoform.com/v1alpha4"
	// VersionlessFamilyPackageAPIVersion identifies content-addressed packages
	// whose family group carries no version segment (decision 0049). It exists
	// only because v1alpha4's index schema refers to a FormRef schema that
	// requires one, and that schema is published.
	VersionlessFamilyPackageAPIVersion = "packages.forms.takoform.com/v1alpha5"
	PackageKind                        = "FormPackage"
	TrustAPIVersion                    = "trust.forms.takoform.com/v1alpha1"
	RevocationKind                     = "FormPackageRevocation"
	RevocationCheckpointKind           = "FormPackageRevocationCheckpoint"
	PackageIndexFilename               = "package-index.json"
	DefinitionMediaType                = "application/vnd.takoform.form-definition.v1+json"
)

// FormRef is the exact portable identity of one immutable Form Definition.
// SchemaDigest is calculated over the definition's RFC 8785 bytes.
type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

// FormConstraint is one entry of a Form Definition's closed constraint list.
// Every pointer is an RFC 6901 JSON Pointer into the desired instance, or into
// the outputs for a host-assigned member. References is shared by the closed
// orderedPair, distinctPair, and uniquePair variants; Kind fixes whether the
// values are numeric desired fields or resolved resource UIDs.
type FormConstraint struct {
	Kind       string   `json:"kind"`
	Reference  string   `json:"reference,omitempty"`
	KeyedBy    string   `json:"keyedBy,omitempty"`
	List       string   `json:"list,omitempty"`
	Member     string   `json:"member,omitempty"`
	Total      int64    `json:"total,omitempty"`
	Property   string   `json:"property,omitempty"`
	Output     string   `json:"output,omitempty"`
	References []string `json:"references,omitempty"`
	Anchor     string   `json:"anchor,omitempty"`
	Members    string   `json:"members,omitempty"`
	Through    string   `json:"through,omitempty"`
}

// MarshalJSON preserves the presence of sum.total when its valid value is
// zero. A plain `omitempty` integer cannot distinguish "sum of exactly zero"
// from "this constraint kind has no total member", while the closed schemas
// require total on sum and forbid it everywhere else.
func (constraint FormConstraint) MarshalJSON() ([]byte, error) {
	type wireConstraint struct {
		Kind       string   `json:"kind"`
		Reference  string   `json:"reference,omitempty"`
		KeyedBy    string   `json:"keyedBy,omitempty"`
		List       string   `json:"list,omitempty"`
		Member     string   `json:"member,omitempty"`
		Total      *int64   `json:"total,omitempty"`
		Property   string   `json:"property,omitempty"`
		Output     string   `json:"output,omitempty"`
		References []string `json:"references,omitempty"`
		Anchor     string   `json:"anchor,omitempty"`
		Members    string   `json:"members,omitempty"`
		Through    string   `json:"through,omitempty"`
	}
	var total *int64
	if constraint.Kind == "sum" || constraint.Total != 0 {
		total = &constraint.Total
	}
	return json.Marshal(wireConstraint{
		Kind: constraint.Kind, Reference: constraint.Reference, KeyedBy: constraint.KeyedBy,
		List: constraint.List, Member: constraint.Member, Total: total,
		Property: constraint.Property, Output: constraint.Output,
		References: constraint.References, Anchor: constraint.Anchor, Members: constraint.Members, Through: constraint.Through,
	})
}

type FormDefinition struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	Title             string `json:"title"`
	Description       string `json:"description,omitempty"`
	// Status is retained only for immutable v1alpha1 Definition bytes. The
	// v1alpha2 schema forbids it because forms/lifecycle.json is the sole
	// authority for Proposal, Experimental, Stable, and Legacy maturity.
	Status string `json:"status,omitempty"`
	// Role is the closed family resource role (decision 0009). The frozen
	// v1alpha1/v1alpha2 schemas forbid it, so it stays empty on those epochs.
	Role string `json:"role,omitempty"`
	// RequiresHostAPI is the earliest Host API lane whose rules this contract
	// needs — a lower bound, not a pin (decision 0047). It is the one
	// dependency every Form has and the only one that used to travel by
	// convention, which is why a family and a lane could never move apart.
	// Empty on the epochs whose frozen schemas forbid it.
	RequiresHostAPI string `json:"requiresHostApi,omitempty"`
	// Constraints is the closed list of rules about RESOURCES this Form
	// declares (decision 0049). They are not shape, so they are not in the
	// desired schema, where they rode in extension slots no standard validator
	// reads. Empty on the epochs whose frozen schemas forbid the member.
	Constraints   []FormConstraint `json:"constraints,omitempty"`
	DesiredSchema map[string]any   `json:"desiredSchema"`
	// ObservedSchema is required by the frozen v1alpha1/v1alpha2 schemas and
	// optional in the family lanes, where the envelope owns status.
	ObservedSchema        map[string]any        `json:"observedSchema,omitempty"`
	OutputSchema          map[string]any        `json:"outputSchema,omitempty"`
	ImmutableFields       []string              `json:"immutableFields,omitempty"`
	LifecycleCapabilities []string              `json:"lifecycleCapabilities"`
	Interfaces            []InterfaceDescriptor `json:"interfaces,omitempty"`
	// ProvidedInterfaces and AcceptedBindings are the exact digest-bound
	// contracts of the family lane (decision 0010).
	ProvidedInterfaces  []InterfaceRef       `json:"providedInterfaces,omitempty"`
	AcceptedBindings    []BindingRef         `json:"acceptedBindings,omitempty"`
	ConformanceFixtures []ConformanceFixture `json:"conformanceFixtures,omitempty"`
	NegativeFixtures    []NegativeFixture    `json:"negativeConformanceFixtures,omitempty"`
}

// InterfaceRef is the exact identity of one published Interface Definition.
// SchemaDigest is calculated over the definition's RFC 8785 bytes.
type InterfaceRef struct {
	APIVersion   string `json:"apiVersion"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

// BindingRef is the exact identity of one published Binding Definition.
// SchemaDigest is calculated over the definition's RFC 8785 bytes.
type BindingRef struct {
	APIVersion   string `json:"apiVersion"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

// InterfaceDescriptor declares one portable runtime interface a Form exposes.
// Name and Version are author-defined: there is no registry, allowlist, or
// central approval for an interface type. A host owns the resulting record,
// authorization, and lifecycle; this descriptor owns only declared data.
type InterfaceDescriptor struct {
	Name             string                      `json:"name"`
	Version          string                      `json:"version"`
	Description      string                      `json:"description,omitempty"`
	Required         bool                        `json:"required,omitempty"`
	ResourceURIInput string                      `json:"resourceUriInput,omitempty"`
	Document         map[string]any              `json:"document,omitempty"`
	DocumentSchema   map[string]any              `json:"documentSchema,omitempty"`
	Inputs           []InterfaceInputDeclaration `json:"inputs,omitempty"`
}

// Portable interface input sources. Any other source must be host-namespaced
// (`<host>.<token>`) and is explicitly non-portable: a host that does not
// understand one fails closed instead of dropping the input.
const (
	InterfaceInputSourceLiteral = "literal"
	InterfaceInputSourceOutput  = "output"
	// InterfaceInputSourceResourceURI asks the host to supply its canonical
	// OAuth resource URI for this runtime declaration. It is non-secret and
	// grants no authorization by itself.
	InterfaceInputSourceResourceURI = "resource_uri"
)

// InterfaceInputDeclaration is a deterministic mapping from the Form's own
// output document (or a literal) into one named interface input. Value is raw
// JSON so an explicit JSON null remains distinguishable from an absent value.
// It never carries credentials or targets. The resource_uri source is the one
// explicit host-resolved identifier and remains a non-secret audience fence.
type InterfaceInputDeclaration struct {
	Name    string          `json:"name"`
	Source  string          `json:"source"`
	Pointer string          `json:"pointer,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
}

// PortableInterfaceInputSource reports whether a source is part of the closed
// portable vocabulary every conforming host must understand.
func PortableInterfaceInputSource(source string) bool {
	return source == InterfaceInputSourceLiteral || source == InterfaceInputSourceOutput || source == InterfaceInputSourceResourceURI
}

type ConformanceFixture struct {
	Name         string `json:"name"`
	DesiredPath  string `json:"desiredPath"`
	ObservedPath string `json:"observedPath,omitempty"`
	OutputPath   string `json:"outputPath,omitempty"`
}

type NegativeFixture struct {
	Name            string `json:"name"`
	Stage           string `json:"stage"`
	InputPath       string `json:"inputPath"`
	ExpectedFailure string `json:"expectedFailure"`
}

type PackageIndex struct {
	APIVersion     string        `json:"apiVersion"`
	Kind           string        `json:"kind"`
	PackageVersion string        `json:"packageVersion,omitempty"`
	FormRef        FormRef       `json:"formRef"`
	DefinitionPath string        `json:"definitionPath"`
	Files          []PackageFile `json:"files"`
}

type PackageFile struct {
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}

type VerificationReport struct {
	PackageDigest string  `json:"packageDigest"`
	FormRef       FormRef `json:"formRef"`
	FileCount     int     `json:"fileCount"`
	PayloadBytes  int64   `json:"payloadBytes"`
	verified      *verifiedPackageData
}

// VerifiedPackage is the immutable capability issued by VerifyDirectory after
// it has verified a complete Form Package closure. Its zero value is invalid;
// callers cannot construct a valid value because the issuance state is
// package-private and is never serialized through VerificationReport.
//
// The value is intentionally small and copyable. The data it points at is
// immutable after issuance, and every method returns a defensive copy at the
// public seam.
type VerifiedPackage struct {
	data *verifiedPackageData
}

type verifiedPackageIssuer struct{}

var verifiedPackageIssuerToken = &verifiedPackageIssuer{}

type verifiedPackageData struct {
	issuer              *verifiedPackageIssuer
	packageDigest       string
	formRef             FormRef
	index               PackageIndex
	canonicalDefinition []byte
	payloads            map[string][]byte
}

// VerifiedPackage returns the package capability attached to a successful
// VerifyDirectory report. Reports built by callers or decoded from JSON never
// carry issuance state and therefore return false.
func (report VerificationReport) VerifiedPackage() (VerifiedPackage, bool) {
	packageValue := VerifiedPackage{data: report.verified}
	if !packageValue.Valid() {
		return VerifiedPackage{}, false
	}
	return packageValue, true
}

// Valid reports whether this value was issued by VerifyDirectory.
func (packageValue VerifiedPackage) Valid() bool {
	return packageValue.data != nil && packageValue.data.issuer == verifiedPackageIssuerToken
}

// PackageDigest returns the verified canonical package-index digest.
func (packageValue VerifiedPackage) PackageDigest() string {
	if !packageValue.Valid() {
		return ""
	}
	return packageValue.data.packageDigest
}

// FormRef returns the verified exact Form identity.
func (packageValue VerifiedPackage) FormRef() FormRef {
	if !packageValue.Valid() {
		return FormRef{}
	}
	return packageValue.data.formRef
}

// PackageIndex returns a defensive copy of the validated package index.
func (packageValue VerifiedPackage) PackageIndex() PackageIndex {
	if !packageValue.Valid() {
		return PackageIndex{}
	}
	return clonePackageIndex(packageValue.data.index)
}

// Definition returns a defensive copy of the canonical Form Definition bytes.
func (packageValue VerifiedPackage) Definition() []byte {
	if !packageValue.Valid() {
		return nil
	}
	return cloneBytes(packageValue.data.canonicalDefinition)
}

// Files returns a defensive copy of the validated payload file inventory.
func (packageValue VerifiedPackage) Files() []PackageFile {
	if !packageValue.Valid() {
		return nil
	}
	return append([]PackageFile(nil), packageValue.data.index.Files...)
}

// Payload returns a defensive copy of one verified payload by its canonical
// package-relative path. The package index remains the source of file metadata;
// this method returns only bytes and whether the path is listed.
func (packageValue VerifiedPackage) Payload(relativePath string) ([]byte, bool) {
	if !packageValue.Valid() {
		return nil, false
	}
	raw, ok := packageValue.data.payloads[relativePath]
	if !ok {
		return nil, false
	}
	return cloneBytes(raw), true
}

// RevocationStatement is one immutable, append-only security decision for an
// exact Form Package digest. Deprecation is represented by Form Definition
// status and must not be encoded as a security revocation.
type RevocationStatement struct {
	APIVersion       string            `json:"apiVersion"`
	Kind             string            `json:"kind"`
	Sequence         uint64            `json:"sequence"`
	StatementVersion string            `json:"statementVersion"`
	PackageDigest    string            `json:"packageDigest"`
	FormRef          FormRef           `json:"formRef"`
	ReasonCode       string            `json:"reasonCode"`
	Summary          string            `json:"summary"`
	AdvisoryURL      string            `json:"advisoryUrl,omitempty"`
	IssuedAt         string            `json:"issuedAt"`
	Effects          RevocationEffects `json:"effects"`
}

type RevocationEffects struct {
	BlockNewCreateOrUpdate         bool `json:"blockNewCreateOrUpdate"`
	BlockActivation                bool `json:"blockActivation"`
	RetainBytesForObserveAndDelete bool `json:"retainBytesForObserveAndDelete"`
}

// RevocationCheckpoint is a signed cumulative index. Sequence and
// PreviousCheckpointDigest form a monotonic hash chain; Entries closes the
// complete statement set from sequence 1 through this checkpoint.
type RevocationCheckpoint struct {
	APIVersion               string                      `json:"apiVersion"`
	Kind                     string                      `json:"kind"`
	CheckpointVersion        string                      `json:"checkpointVersion"`
	Sequence                 uint64                      `json:"sequence"`
	PreviousCheckpointDigest *string                     `json:"previousCheckpointDigest"`
	Entries                  []RevocationCheckpointEntry `json:"entries"`
}

type RevocationCheckpointEntry struct {
	Sequence         uint64  `json:"sequence"`
	StatementVersion string  `json:"statementVersion"`
	StatementDigest  string  `json:"statementDigest"`
	PackageDigest    string  `json:"packageDigest"`
	FormRef          FormRef `json:"formRef"`
}

// RevocationCheckpointPin is the minimum durable state a host retains after
// cryptographically verifying a checkpoint signature and publisher policy.
type RevocationCheckpointPin struct {
	Sequence      uint64 `json:"sequence"`
	Digest        string `json:"digest"`
	EntriesDigest string `json:"entriesDigest"`
}

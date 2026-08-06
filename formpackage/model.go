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
	// Form Family (namespaced-group, v1alpha3 form-definition) Forms. FormRefs
	// in this lane use DNS-like groups, so there is no single Form apiVersion
	// constant; validation accepts any namespaced group outside the two frozen
	// central epochs.
	FamilyPackageAPIVersion  = "packages.forms.takoform.com/v1alpha4"
	PackageKind              = "FormPackage"
	TrustAPIVersion          = "trust.forms.takoform.com/v1alpha1"
	RevocationKind           = "FormPackageRevocation"
	RevocationCheckpointKind = "FormPackageRevocationCheckpoint"
	PackageIndexFilename     = "package-index.json"
	DefinitionMediaType      = "application/vnd.takoform.form-definition.v1+json"
)

// FormRef is the exact portable identity of one immutable Form Definition.
// SchemaDigest is calculated over the definition's RFC 8785 bytes.
type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
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
	// Role is the closed v1alpha3 resource role (decision 0009). The frozen
	// v1alpha1/v1alpha2 schemas forbid it, so it stays empty on those epochs.
	Role          string         `json:"role,omitempty"`
	DesiredSchema map[string]any `json:"desiredSchema"`
	// ObservedSchema is required by the frozen v1alpha1/v1alpha2 schemas and
	// optional in the v1alpha3 family lane, where the envelope owns status.
	ObservedSchema        map[string]any        `json:"observedSchema,omitempty"`
	OutputSchema          map[string]any        `json:"outputSchema,omitempty"`
	ImmutableFields       []string              `json:"immutableFields,omitempty"`
	LifecycleCapabilities []string              `json:"lifecycleCapabilities"`
	Interfaces            []InterfaceDescriptor `json:"interfaces,omitempty"`
	// ProvidedInterfaces and AcceptedBindings are the exact digest-bound
	// contracts of the v1alpha3 family lane (decision 0010).
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

package formpackage

import "encoding/json"

const (
	FormAPIVersion           = "forms.takoform.com/v1alpha1"
	PackageAPIVersion        = "packages.forms.takoform.com/v1alpha1"
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
	APIVersion            string                `json:"apiVersion"`
	Kind                  string                `json:"kind"`
	DefinitionVersion     string                `json:"definitionVersion"`
	Title                 string                `json:"title"`
	Description           string                `json:"description,omitempty"`
	Status                string                `json:"status"`
	DesiredSchema         map[string]any        `json:"desiredSchema"`
	ObservedSchema        map[string]any        `json:"observedSchema"`
	OutputSchema          map[string]any        `json:"outputSchema,omitempty"`
	ImmutableFields       []string              `json:"immutableFields,omitempty"`
	LifecycleCapabilities []string              `json:"lifecycleCapabilities"`
	Interfaces            []InterfaceDescriptor `json:"interfaces,omitempty"`
	ConformanceFixtures   []ConformanceFixture  `json:"conformanceFixtures,omitempty"`
	NegativeFixtures      []NegativeFixture     `json:"negativeConformanceFixtures,omitempty"`
}

// InterfaceDescriptor declares one portable runtime interface a Form exposes.
// Name and Version are author-defined: there is no registry, allowlist, or
// central approval for an interface type. A host owns the resulting record,
// authorization, and lifecycle; this descriptor owns only declared data.
type InterfaceDescriptor struct {
	Name           string                      `json:"name"`
	Version        string                      `json:"version"`
	Description    string                      `json:"description,omitempty"`
	Required       bool                        `json:"required,omitempty"`
	Document       map[string]any              `json:"document,omitempty"`
	DocumentSchema map[string]any              `json:"documentSchema,omitempty"`
	Inputs         []InterfaceInputDeclaration `json:"inputs,omitempty"`
}

// Portable interface input sources. Any other source must be host-namespaced
// (`<host>.<token>`) and is explicitly non-portable: a host that does not
// understand one fails closed instead of dropping the input.
const (
	InterfaceInputSourceLiteral = "literal"
	InterfaceInputSourceOutput  = "output"
)

// InterfaceInputDeclaration is a deterministic mapping from the Form's own
// output document (or a literal) into one named interface input. Value is raw
// JSON so an explicit JSON null remains distinguishable from an absent value.
// It never carries credentials, targets, or host identifiers.
type InterfaceInputDeclaration struct {
	Name    string          `json:"name"`
	Source  string          `json:"source"`
	Pointer string          `json:"pointer,omitempty"`
	Value   json.RawMessage `json:"value,omitempty"`
}

// PortableInterfaceInputSource reports whether a source is part of the closed
// portable vocabulary every conforming host must understand.
func PortableInterfaceInputSource(source string) bool {
	return source == InterfaceInputSourceLiteral || source == InterfaceInputSourceOutput
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
	PackageVersion string        `json:"packageVersion"`
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

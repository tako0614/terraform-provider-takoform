package formpackage

const (
	FormAPIVersion       = "forms.takoform.com/v1alpha1"
	PackageAPIVersion    = "packages.forms.takoform.com/v1alpha1"
	PackageKind          = "FormPackage"
	TrustAPIVersion      = "trust.forms.takoform.com/v1alpha1"
	RevocationKind       = "FormPackageRevocation"
	PackageIndexFilename = "package-index.json"
	DefinitionMediaType  = "application/vnd.takoform.form-definition.v1+json"
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
	ImmutableFields       []string              `json:"immutableFields,omitempty"`
	LifecycleCapabilities []string              `json:"lifecycleCapabilities"`
	Interfaces            []InterfaceDescriptor `json:"interfaces,omitempty"`
	ConformanceFixtures   []ConformanceFixture  `json:"conformanceFixtures,omitempty"`
}

type InterfaceDescriptor struct {
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	Description    string         `json:"description,omitempty"`
	DocumentSchema map[string]any `json:"documentSchema,omitempty"`
}

type ConformanceFixture struct {
	Name         string `json:"name"`
	DesiredPath  string `json:"desiredPath"`
	ObservedPath string `json:"observedPath,omitempty"`
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

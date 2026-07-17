// Package standardform defines and structurally validates externally
// authenticated evidence for an exact portable Form Package. It grants no
// placement, credential, execution, or commercial authority.
package standardform

import "github.com/tako0614/terraform-provider-takoform/formpackage"

const (
	APIVersion               = "forms.takoform.com/standard-admission/v1alpha1"
	InvalidArgumentErrorCode = "invalid_argument"
)

type InstalledFormReference struct {
	FormRef       formpackage.FormRef `json:"formRef"`
	PackageDigest string              `json:"packageDigest"`
}

type PositiveFixture struct {
	Name     string         `json:"name"`
	Desired  map[string]any `json:"desired"`
	Observed map[string]any `json:"observed"`
	Output   map[string]any `json:"output"`
}

type NegativeFixture struct {
	Name              string         `json:"name"`
	Stage             string         `json:"stage"`
	Input             map[string]any `json:"input"`
	ExpectedErrorCode string         `json:"expectedErrorCode"`
}

type ConformanceProof struct {
	Subject          string                 `json:"subject"`
	RunnerVersion    string                 `json:"runnerVersion"`
	Identity         InstalledFormReference `json:"identity"`
	Status           string                 `json:"status"`
	PositiveFixtures []string               `json:"positiveFixtures"`
	NegativeFixtures []string               `json:"negativeFixtures"`
	EvidenceDigest   string                 `json:"evidenceDigest"`
}

type AdmissionEvidence struct {
	APIVersion           string                 `json:"apiVersion"`
	Identity             InstalledFormReference `json:"identity"`
	Classification       string                 `json:"classification"`
	ApprovedSchemaDigest string                 `json:"approvedSchemaDigest"`
	Audit                Audit                  `json:"audit"`
	Fixtures             Fixtures               `json:"fixtures"`
	Conformance          Conformance            `json:"conformance"`
}

type Audit struct {
	Lifecycle    LifecycleAudit    `json:"lifecycle"`
	Immutability ImmutabilityAudit `json:"immutability"`
	Security     SecurityAudit     `json:"security"`
	Interfaces   InterfaceAudit    `json:"interfaces"`
}

type LifecycleAudit struct {
	Create  bool `json:"create"`
	Read    bool `json:"read"`
	Update  bool `json:"update"`
	Delete  bool `json:"delete"`
	Import  bool `json:"import"`
	Observe bool `json:"observe"`
	Refresh bool `json:"refresh"`
	Drift   bool `json:"drift"`
}

type ImmutabilityAudit struct {
	Reviewed bool     `json:"reviewed"`
	Fields   []string `json:"fields"`
}

type SecurityAudit struct {
	SecretFreeDesiredState     bool `json:"secretFreeDesiredState"`
	CredentialBoundaryExternal bool `json:"credentialBoundaryExternal"`
	DataOnlyPackage            bool `json:"dataOnlyPackage"`
}

type InterfaceAudit struct {
	Reviewed                 bool `json:"reviewed"`
	BindingAuthorityExternal bool `json:"bindingAuthorityExternal"`
	SecretFreeDocuments      bool `json:"secretFreeDocuments"`
}

type Fixtures struct {
	Positive []PositiveFixture `json:"positive"`
	Negative []NegativeFixture `json:"negative"`
}

type Conformance struct {
	Host     ConformanceProof `json:"host"`
	Provider ConformanceProof `json:"provider"`
}

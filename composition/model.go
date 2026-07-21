// Package composition defines the portable, data-only Capsule Composition
// manifest. It intentionally describes user-selectable Git Capsule sets, not
// a Takosumi host's execution policy, credentials, targets, or billing.
package composition

const (
	APIVersion = "compositions.takoform.com/v1alpha1"
	Kind       = "CapsuleComposition"
)

// Manifest is a portable selection of one or more ordinary Git-hosted
// OpenTofu/Terraform Capsules. A host still performs source policy checks,
// ProviderConnection selection, InterfaceBinding authorization, Plan review,
// and Run execution through its own control plane.
type Manifest struct {
	APIVersion  string       `json:"apiVersion"`
	Kind        string       `json:"kind"`
	Metadata    Metadata     `json:"metadata"`
	Components  []Component  `json:"components"`
	Connections []Connection `json:"connections,omitempty"`
}

type Metadata struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// Component names one independently installable Capsule source. The source is
// descriptive input to a host's ordinary Git Source flow; it never supplies
// credentials, commands, provider configuration, or a host InstallConfig.
type Component struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Source      Source `json:"source"`
}

type Source struct {
	URL  string `json:"url"`
	Ref  string `json:"ref"`
	Path string `json:"path"`
}

// Connection declares an intended non-secret Interface relationship. It is a
// request for the host to present and validate; it does not itself authorize
// access or create an InterfaceBinding.
type Connection struct {
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`
}

type Endpoint struct {
	Component string `json:"component"`
	Interface string `json:"interface"`
}

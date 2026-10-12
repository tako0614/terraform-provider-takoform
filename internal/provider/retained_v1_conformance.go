//go:build retained_v1_conformance

package provider

import frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"

// NewRetainedV1ForConformance exposes the retained v1 implementation only to
// the explicitly tagged local worker-authoring fixture. Shipping New remains
// v2-only; this entrypoint is neither a fallback nor a release binary.
func NewRetainedV1ForConformance(version string) func() frameworkprovider.Provider {
	return func() frameworkprovider.Provider {
		return &takoformProvider{version: version}
	}
}

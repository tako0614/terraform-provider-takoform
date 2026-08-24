package formpackage

// This file is the provider-neutral consumption seam for stable
// StandardServiceRef and StandardServiceSupport identities. It validates only
// the generic contract: opaque identifier grammar, exact equality, and the
// Host's satisfiable decision. Protocol semantics and materialized entries
// belong to the Host integration.

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

const (
	LegacyStandardServiceAPIVersion  = "standards.takoform.com/v1alpha1"
	StandardServiceAPIVersion        = "standards.takoform.com/v1"
	StandardServiceSupportAPIVersion = "support.takoform.com/v1"
	PatternStandardServiceProtocol   = `^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?){2,}$`
	StandardServiceProtocolMaxLength = 253
)

var standardServiceProtocolPattern = regexp.MustCompile(PatternStandardServiceProtocol)

// StandardServiceRef is one exact opaque protocol identity.
type StandardServiceRef struct {
	APIVersion string `json:"apiVersion"`
	Protocol   string `json:"protocol"`
}

// ValidateStandardServiceRef validates structure, never protocol meaning or
// conformance. A namespaced identifier unknown to Takoform is valid.
func ValidateStandardServiceRef(ref StandardServiceRef) error {
	if ref.APIVersion != StandardServiceAPIVersion {
		return fmt.Errorf("standard service apiVersion must be %s", StandardServiceAPIVersion)
	}
	if utf8.RuneCountInString(ref.Protocol) > StandardServiceProtocolMaxLength {
		return fmt.Errorf("standard service protocol exceeds maxLength %d", StandardServiceProtocolMaxLength)
	}
	if !standardServiceProtocolPattern.MatchString(ref.Protocol) {
		return fmt.Errorf("standard service protocol %q is not a normalized reverse-DNS owner namespace plus protocol segment", ref.Protocol)
	}
	return nil
}

// ValidateStandardServiceSupport validates one stable Host support answer and
// returns its satisfiable decision. Every identity member is exact: a profile
// for a different protocol is not a fallback or an alias.
func ValidateStandardServiceSupport(ref StandardServiceRef, profile map[string]any) (bool, error) {
	if err := ValidateStandardServiceRef(ref); err != nil {
		return false, err
	}
	if got, _ := profile["apiVersion"].(string); got != StandardServiceSupportAPIVersion {
		return false, fmt.Errorf("standard-service support apiVersion must be %s", StandardServiceSupportAPIVersion)
	}
	if got, _ := profile["kind"].(string); got != "StandardServiceSupport" {
		return false, fmt.Errorf("standard-service support kind must be StandardServiceSupport")
	}
	rawRef, ok := profile["serviceRef"].(map[string]any)
	if !ok {
		return false, fmt.Errorf("standard-service support omits serviceRef")
	}
	answered := StandardServiceRef{}
	answered.APIVersion, _ = rawRef["apiVersion"].(string)
	answered.Protocol, _ = rawRef["protocol"].(string)
	if err := ValidateStandardServiceRef(answered); err != nil {
		return false, fmt.Errorf("standard-service support serviceRef: %w", err)
	}
	if answered != ref {
		return false, fmt.Errorf("standard-service support names a different exact serviceRef")
	}
	satisfiable, ok := profile["satisfiable"].(bool)
	if !ok {
		return false, fmt.Errorf("standard-service support omits boolean satisfiable")
	}
	return satisfiable, nil
}

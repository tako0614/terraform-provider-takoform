package provider

import (
	"regexp"

	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

var artifactSHA256Pattern = regexp.MustCompile(`^(sha256:)?[A-Fa-f0-9]{64}$`)
var artifactMediaTypePattern = regexp.MustCompile(formcatalog.PatternMediaType)
var ociDigestReferencePattern = regexp.MustCompile(`^[^@\s]+@sha256:[A-Fa-f0-9]{64}$`)
var portableNamePattern = regexp.MustCompile(formcatalog.PatternName)

const portableCapabilityTokenPattern = `^[A-Za-z][A-Za-z0-9._:-]{0,127}$`
const portableConnectionNamePattern = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
const portableTimezonePattern = `^[A-Za-z][A-Za-z0-9._:/+-]{0,127}$`

func validOCIDigestReference(value string) bool {
	return ociDigestReferencePattern.MatchString(value)
}

func validCredentialFreeArtifactURL(value string) bool {
	return formcatalog.ValidCredentialFreeHTTPSURL(value)
}

func validPortableName(value string) bool {
	return portableNamePattern.MatchString(value)
}

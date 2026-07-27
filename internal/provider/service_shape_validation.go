package provider

import (
	"regexp"
	"strings"
	"unicode"
)

var artifactSHA256Pattern = regexp.MustCompile(`^(sha256:)?[A-Fa-f0-9]{64}$`)
var ociDigestReferencePattern = regexp.MustCompile(`^[^@\s]+@sha256:[A-Fa-f0-9]{64}$`)

const portableCapabilityTokenPattern = `^[A-Za-z][A-Za-z0-9._:-]{0,127}$`
const portableConnectionNamePattern = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
const portableTimezonePattern = `^[A-Za-z][A-Za-z0-9._:/+-]{0,127}$`

func validOCIDigestReference(value string) bool {
	return ociDigestReferencePattern.MatchString(value)
}

func printableBoundedString(value string, max int) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(value) > max {
		return false
	}
	return !strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r)
	})
}

func validPortableName(value string) bool {
	return printableBoundedString(value, 128)
}

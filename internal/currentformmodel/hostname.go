package currentformmodel

import "strings"

// hostname.go is the ONE canonicalization of a DNS hostname in the v1beta1
// lane, shared by the host that compares and stores hostnames and by the
// client that plans them.
//
// DNS names are case-insensitive and a trailing dot is the fully-qualified
// spelling of the same name, so "API.Example.com", "api.example.com." and
// "api.example.com" are one hostname. A host that compared the bytes an author
// wrote would see three, and two resources could then claim one name.
//
// The canonical form is: the trailing root dot removed, and every ASCII letter
// lowercased. Nothing else is rewritten.
//
// An internationalized name is written as its A-LABEL — the "xn--" ASCII form
// — and never as its U-label. That is a refusal rather than a conversion, and
// deliberately so: performing IDNA/UTS-46 mapping inside the host would make
// the host's Unicode table version part of the portable contract, so two
// conforming hosts on different Unicode versions could canonicalize one U-label
// to two different names. Portability is exactly what this lane refuses to
// leave to a host's private tables (decision 0008), so the conversion belongs
// in the author's tooling, and what travels is the ASCII result. PatternHostname
// admits no non-ASCII byte, so a U-label is refused by the Form's own grammar
// with nothing left implicit.
//
// CanonicalHostname is total: a value the grammar refuses is returned with the
// same two rewrites applied and is then rejected by schema validation, so this
// function never has to decide validity and never has to fail.

// CanonicalHostname returns the canonical spelling of one DNS hostname: the
// trailing root dot removed and ASCII letters lowercased.
func CanonicalHostname(hostname string) string {
	trimmed := strings.TrimSuffix(hostname, ".")
	lowered := make([]byte, 0, len(trimmed))
	for index := 0; index < len(trimmed); index++ {
		character := trimmed[index]
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		lowered = append(lowered, character)
	}
	return string(lowered)
}

// HostnameIsCanonical reports whether a hostname is already written in its
// canonical form. A client uses it to refuse a spelling it cannot represent
// without drift: a Terraform attribute holds the literal string an author
// wrote, so a configuration whose hostname the host would rewrite would plan
// one value and read back another forever.
func HostnameIsCanonical(hostname string) bool {
	return CanonicalHostname(hostname) == hostname
}

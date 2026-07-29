package client

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// SpaceIDMaxLength is measured in Unicode code points, matching JSON Schema's
// maxLength semantics for the normative host wire contract.
const SpaceIDMaxLength = 255

// ValidateSpaceID enforces the portable Space identity contract without
// normalizing it. A SpaceID is an opaque, case-sensitive UTF-8 string. Embedded
// whitespace is data; only boundary whitespace, controls, and slash are
// forbidden.
func ValidateSpaceID(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("SpaceID must be valid UTF-8")
	}
	length := utf8.RuneCountInString(value)
	if length < 1 || length > SpaceIDMaxLength {
		return fmt.Errorf(
			"SpaceID must contain between 1 and %d Unicode code points",
			SpaceIDMaxLength,
		)
	}
	first, _ := utf8.DecodeRuneInString(value)
	last, _ := utf8.DecodeLastRuneInString(value)
	if isSpaceIDBoundaryWhitespace(first) || isSpaceIDBoundaryWhitespace(last) {
		return errors.New("SpaceID must not start or end with whitespace")
	}
	for _, candidate := range value {
		switch {
		case candidate == '/':
			return errors.New("SpaceID must not contain '/'")
		case isSpaceIDControl(candidate):
			return errors.New("SpaceID must not contain control characters")
		}
	}
	return nil
}

// These explicit code-point sets are mirrored by the normative JSON Schema.
// U+FEFF is treated as boundary whitespace even though modern Unicode removed
// it from White_Space; accepting a BOM at an identity boundary is never useful
// and would make cross-runtime trimming behavior ambiguous.
func isSpaceIDBoundaryWhitespace(candidate rune) bool {
	switch {
	case candidate >= '\u0009' && candidate <= '\u000d':
		return true
	case candidate == '\u0020',
		candidate == '\u0085',
		candidate == '\u00a0',
		candidate == '\u1680',
		candidate >= '\u2000' && candidate <= '\u200a',
		candidate == '\u2028',
		candidate == '\u2029',
		candidate == '\u202f',
		candidate == '\u205f',
		candidate == '\u3000',
		candidate == '\ufeff':
		return true
	default:
		return false
	}
}

func isSpaceIDControl(candidate rune) bool {
	return candidate >= '\u0000' && candidate <= '\u001f' ||
		candidate >= '\u007f' && candidate <= '\u009f'
}

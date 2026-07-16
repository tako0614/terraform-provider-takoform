// Package formpackage verifies portable, data-only Takoform Form Packages.
//
// It deliberately does not fetch packages, execute extensions, verify
// signatures, or select a host implementation. Those are separate operator
// and release concerns.
package formpackage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	jsoncanonicalizer "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

const (
	digestPrefix = "sha256:"
	maxJSONDepth = 256
)

// Canonicalize validates UTF-8 I-JSON and returns RFC 8785 JSON
// Canonicalization Scheme bytes. Duplicate object names, invalid Unicode,
// non-finite numbers, and every spelling of negative zero are rejected before
// canonicalization.
func Canonicalize(input []byte) ([]byte, error) {
	if !utf8.Valid(input) {
		return nil, fmt.Errorf("I-JSON input is not valid UTF-8")
	}
	parser := strictJSONParser{data: input}
	if err := parser.parseDocument(); err != nil {
		return nil, fmt.Errorf("invalid I-JSON: %w", err)
	}
	canonical, err := jsoncanonicalizer.Transform(input)
	if err != nil {
		return nil, fmt.Errorf("RFC 8785 canonicalization: %w", err)
	}
	return canonical, nil
}

// DigestCanonicalJSON returns the lowercase SHA-256 digest of RFC 8785 bytes.
func DigestCanonicalJSON(input []byte) (string, error) {
	canonical, err := Canonicalize(input)
	if err != nil {
		return "", err
	}
	return DigestBytes(canonical), nil
}

// DigestBytes returns a normalized lowercase sha256:<hex> digest.
func DigestBytes(input []byte) string {
	digest := sha256.Sum256(input)
	return digestPrefix + hex.EncodeToString(digest[:])
}

// ValidDigest reports whether digest is the exact lowercase SHA-256 form used
// by FormRef and package-index documents.
func ValidDigest(digest string) bool {
	if len(digest) != len(digestPrefix)+sha256.Size*2 || !strings.HasPrefix(digest, digestPrefix) {
		return false
	}
	for _, char := range digest[len(digestPrefix):] {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

type strictJSONParser struct {
	data  []byte
	index int
	depth int
}

func (parser *strictJSONParser) parseDocument() error {
	parser.skipWhitespace()
	if err := parser.parseValue(); err != nil {
		return err
	}
	parser.skipWhitespace()
	if parser.index != len(parser.data) {
		return fmt.Errorf("unexpected trailing byte at offset %d", parser.index)
	}
	return nil
}

func (parser *strictJSONParser) parseValue() error {
	parser.skipWhitespace()
	if parser.index >= len(parser.data) {
		return fmt.Errorf("unexpected end of input")
	}
	switch parser.data[parser.index] {
	case '{':
		return parser.parseObject()
	case '[':
		return parser.parseArray()
	case '"':
		_, err := parser.parseString()
		return err
	case 't':
		return parser.parseLiteral("true")
	case 'f':
		return parser.parseLiteral("false")
	case 'n':
		return parser.parseLiteral("null")
	default:
		return parser.parseNumber()
	}
}

func (parser *strictJSONParser) parseObject() error {
	if parser.depth >= maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d containers", maxJSONDepth)
	}
	parser.depth++
	defer func() { parser.depth-- }()
	parser.index++
	parser.skipWhitespace()
	if parser.consume('}') {
		return nil
	}
	names := map[string]struct{}{}
	for {
		parser.skipWhitespace()
		name, err := parser.parseString()
		if err != nil {
			return fmt.Errorf("object name: %w", err)
		}
		if _, duplicate := names[name]; duplicate {
			return fmt.Errorf("duplicate object name %q", name)
		}
		names[name] = struct{}{}
		parser.skipWhitespace()
		if !parser.consume(':') {
			return fmt.Errorf("expected ':' at offset %d", parser.index)
		}
		if err := parser.parseValue(); err != nil {
			return err
		}
		parser.skipWhitespace()
		if parser.consume('}') {
			return nil
		}
		if !parser.consume(',') {
			return fmt.Errorf("expected ',' or '}' at offset %d", parser.index)
		}
	}
}

func (parser *strictJSONParser) parseArray() error {
	if parser.depth >= maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d containers", maxJSONDepth)
	}
	parser.depth++
	defer func() { parser.depth-- }()
	parser.index++
	parser.skipWhitespace()
	if parser.consume(']') {
		return nil
	}
	for {
		if err := parser.parseValue(); err != nil {
			return err
		}
		parser.skipWhitespace()
		if parser.consume(']') {
			return nil
		}
		if !parser.consume(',') {
			return fmt.Errorf("expected ',' or ']' at offset %d", parser.index)
		}
	}
}

func (parser *strictJSONParser) parseString() (string, error) {
	if !parser.consume('"') {
		return "", fmt.Errorf("expected string at offset %d", parser.index)
	}
	var result strings.Builder
	for parser.index < len(parser.data) {
		current := parser.data[parser.index]
		if current == '"' {
			parser.index++
			return result.String(), nil
		}
		if current == '\\' {
			parser.index++
			if parser.index >= len(parser.data) {
				return "", fmt.Errorf("unterminated escape")
			}
			escape := parser.data[parser.index]
			parser.index++
			switch escape {
			case '"', '\\', '/':
				result.WriteByte(escape)
			case 'b':
				result.WriteByte('\b')
			case 'f':
				result.WriteByte('\f')
			case 'n':
				result.WriteByte('\n')
			case 'r':
				result.WriteByte('\r')
			case 't':
				result.WriteByte('\t')
			case 'u':
				decoded, err := parser.parseUnicodeEscape()
				if err != nil {
					return "", err
				}
				result.WriteRune(decoded)
			default:
				return "", fmt.Errorf("invalid escape \\%c", escape)
			}
			continue
		}
		if current < 0x20 {
			return "", fmt.Errorf("unescaped control character at offset %d", parser.index)
		}
		runeValue, width := utf8.DecodeRune(parser.data[parser.index:])
		if runeValue == utf8.RuneError && width == 1 {
			return "", fmt.Errorf("invalid UTF-8 at offset %d", parser.index)
		}
		result.WriteRune(runeValue)
		parser.index += width
	}
	return "", fmt.Errorf("unterminated string")
}

func (parser *strictJSONParser) parseUnicodeEscape() (rune, error) {
	first, err := parser.parseHexCodeUnit()
	if err != nil {
		return 0, err
	}
	firstRune := rune(first)
	if first >= 0xdc00 && first <= 0xdfff {
		return 0, fmt.Errorf("unpaired low surrogate U+%04X", first)
	}
	if first < 0xd800 || first > 0xdbff {
		return firstRune, nil
	}
	if parser.index+2 > len(parser.data) || parser.data[parser.index] != '\\' || parser.data[parser.index+1] != 'u' {
		return 0, fmt.Errorf("unpaired high surrogate U+%04X", first)
	}
	parser.index += 2
	second, err := parser.parseHexCodeUnit()
	if err != nil {
		return 0, err
	}
	if second < 0xdc00 || second > 0xdfff {
		return 0, fmt.Errorf("high surrogate U+%04X followed by non-low-surrogate U+%04X", first, second)
	}
	return utf16.DecodeRune(firstRune, rune(second)), nil
}

func (parser *strictJSONParser) parseHexCodeUnit() (uint16, error) {
	if parser.index+4 > len(parser.data) {
		return 0, fmt.Errorf("truncated Unicode escape")
	}
	value, err := strconv.ParseUint(string(parser.data[parser.index:parser.index+4]), 16, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid Unicode escape at offset %d", parser.index)
	}
	parser.index += 4
	return uint16(value), nil
}

func (parser *strictJSONParser) parseLiteral(literal string) error {
	if parser.index+len(literal) > len(parser.data) || string(parser.data[parser.index:parser.index+len(literal)]) != literal {
		return fmt.Errorf("invalid literal at offset %d", parser.index)
	}
	parser.index += len(literal)
	return nil
}

func (parser *strictJSONParser) parseNumber() error {
	start := parser.index
	if parser.consume('-') && parser.index >= len(parser.data) {
		return fmt.Errorf("truncated number")
	}
	if parser.consume('0') {
		if parser.index < len(parser.data) && parser.data[parser.index] >= '0' && parser.data[parser.index] <= '9' {
			return fmt.Errorf("leading zero at offset %d", parser.index)
		}
	} else {
		if parser.index >= len(parser.data) || parser.data[parser.index] < '1' || parser.data[parser.index] > '9' {
			return fmt.Errorf("invalid number at offset %d", start)
		}
		for parser.index < len(parser.data) && parser.data[parser.index] >= '0' && parser.data[parser.index] <= '9' {
			parser.index++
		}
	}
	if parser.consume('.') {
		fractionStart := parser.index
		for parser.index < len(parser.data) && parser.data[parser.index] >= '0' && parser.data[parser.index] <= '9' {
			parser.index++
		}
		if parser.index == fractionStart {
			return fmt.Errorf("fraction has no digits")
		}
	}
	if parser.index < len(parser.data) && (parser.data[parser.index] == 'e' || parser.data[parser.index] == 'E') {
		parser.index++
		if parser.index < len(parser.data) && (parser.data[parser.index] == '+' || parser.data[parser.index] == '-') {
			parser.index++
		}
		exponentStart := parser.index
		for parser.index < len(parser.data) && parser.data[parser.index] >= '0' && parser.data[parser.index] <= '9' {
			parser.index++
		}
		if parser.index == exponentStart {
			return fmt.Errorf("exponent has no digits")
		}
	}
	number := string(parser.data[start:parser.index])
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return fmt.Errorf("number %q is not finite IEEE-754 binary64", number)
	}
	if strings.HasPrefix(number, "-") && parsed == 0 {
		return fmt.Errorf("negative zero %q is forbidden", number)
	}
	return nil
}

func (parser *strictJSONParser) skipWhitespace() {
	for parser.index < len(parser.data) {
		switch parser.data[parser.index] {
		case ' ', '\t', '\r', '\n':
			parser.index++
		default:
			return
		}
	}
}

func (parser *strictJSONParser) consume(expected byte) bool {
	if parser.index < len(parser.data) && parser.data[parser.index] == expected {
		parser.index++
		return true
	}
	return false
}

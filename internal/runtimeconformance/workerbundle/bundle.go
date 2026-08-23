// Package workerbundle carries the module-level vocabulary of the ES Module
// Worker runtime ABI (`worker.runtime@1.0.0`): the closed loadable media-type
// set, the closed loadModule error vocabulary, the deployment description a
// runtime is measured against, and the derivation of a module's exported
// handler set from its own bytes.
//
// It is deliberately the ONLY thing the in-process fake runtime and the
// conformance runner share. The fake never sees a check or an expectation, so
// it cannot answer from the corpus; it answers from the deployment it was
// given and the bytes it was handed.
package workerbundle

import (
	"errors"
	"fmt"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// HandlerVocabulary is the closed handler vocabulary of the ABI: the
// `declaredHandlers` enum of `worker.runtime@1.0.0`'s loadModule operation,
// which is the single source of truth for it.
var HandlerVocabulary = []string{"fetch", "scheduled", "queue"}

// LoadableMediaTypes is the closed set of module media types
// `worker.runtime@1.0.0` loads. Anything else fails `unsupported_media_type` —
// including a media type a bundle may legitimately CARRY, such as source-map
// evidence, which the graph never imports.
//
// It is read out of the lane's single media-type statement
// (internal/currentformmodel/artifact_media.go) rather than spelled a second
// time here, for the reason that statement exists: a runtime measured against
// a set the artifact manifest does not admit is measured for loading modules
// no conforming bundle can carry.
var LoadableMediaTypes = currentformmodel.LoadableModuleMediaTypes()

// The closed loadModule error vocabulary.
const (
	ErrorModuleNotFound      = "module_not_found"
	ErrorUnsupportedMedia    = "unsupported_media_type"
	ErrorModuleSyntax        = "module_syntax_error"
	ErrorHandlerNotExported  = "handler_not_exported"
	ErrorHandlerNotDeclared  = "handler_not_declared"
	ErrorEnvironmentConflict = "environment_name_collision"
)

// LoadErrorCodes is the closed loadModule error vocabulary in contract order.
var LoadErrorCodes = []string{
	ErrorModuleNotFound,
	ErrorUnsupportedMedia,
	ErrorModuleSyntax,
	ErrorHandlerNotExported,
}

// ErrModuleSyntax is returned when a module's default export cannot be read
// from its bytes at all. A real runtime reaches the same conclusion with a
// JavaScript parser; the reference derivation below reaches it with a scanner,
// and says so.
var ErrModuleSyntax = errors.New("module bytes do not present a derivable default export object")

// IsLoadableMediaType reports whether a module media type is in the closed set.
func IsLoadableMediaType(mediaType string) bool {
	for _, candidate := range LoadableMediaTypes {
		if candidate == mediaType {
			return true
		}
	}
	return false
}

// Module is one module of a Worker Bundle: its bundle-relative name, its
// declared media type, and its exact bytes.
type Module struct {
	Name      string
	MediaType string
	Source    []byte
}

// Bundle is one loadable module set with the name of its main module.
type Bundle struct {
	Name       string
	MainModule string
	Modules    []Module
}

// Module returns the named module of the bundle.
func (b Bundle) Module(name string) (Module, bool) {
	for _, module := range b.Modules {
		if module.Name == name {
			return module, true
		}
	}
	return Module{}, false
}

// Binding is one typed binding projected into `env` under one name.
type Binding struct {
	Name      string
	Interface string
}

// Deployment describes the one worker a runtime conformance run is measured
// against: which bundle runs, which handlers the version declares, and exactly
// what the ABI must project into `env`.
//
// Peer is the SECOND worker a `worker.service` binding addresses. The binding
// projects a call to ANOTHER Module Worker, so a run that measured it against
// the worker itself would measure a short circuit rather than the projection;
// the peer is therefore its own deployment, running its OWN bundle and
// declaring `fetch` and nothing else. Its bundle is not the caller's, because a
// peer running the caller's bytes is observationally the caller: the peer's
// module carries an identity (PeerIdentityBinding) that the caller's does not,
// and the answer a service call returns is credited only when it is stamped
// with it. A deployment with no `worker.service` binding carries no peer.
type Deployment struct {
	Bundle           Bundle
	DeclaredHandlers []string
	Vars             []string
	SensitiveVars    []string
	Bindings         []Binding
	ExternalServices []ExternalService
	Cron             string
	Queue            string
	Peer             *Deployment
}

// ExternalService is one sealed external standard-service slot the version
// declares (decision 0045). The runtime never learns the address or the
// credential from portable state: the host resolves both and projects the
// protocol's fixed member set under Name, which is what makes this a fourth
// source of `env` property names.
type ExternalService struct {
	Name     string
	Protocol string
}

// ExternalServiceProjectedNames is the fixed member set one slot projects, by
// protocol. It is stated once here so the ABI, the corpus, and any host
// derive the same closure.
func ExternalServiceProjectedNames(name, protocol string) []string {
	switch protocol {
	case "s3-compatible":
		return []string{
			name + "_ENDPOINT", name + "_REGION", name + "_BUCKET",
			name + "_ACCESS_KEY_ID", name + "_SECRET_ACCESS_KEY",
		}
	default:
		return []string{name + "_URL"}
	}
}

// EnvironmentPropertyNames is the complete set of own enumerable `env`
// property names the ABI requires, in lexicographic order: every `vars` key,
// every `requiredSensitiveVars` name, every binding name, and every member
// each external-service slot projects. It fails closed on a collision, because
// the four sources share one namespace — and the slot members are exactly why
// the check is stated over the PROJECTED closure rather than the declared
// names: two slots with distinct names can still collide once expanded.
func (d Deployment) EnvironmentPropertyNames() ([]string, error) {
	seen := map[string]bool{}
	names := make([]string, 0, len(d.Vars)+len(d.SensitiveVars)+len(d.Bindings)+len(d.ExternalServices))
	add := func(name string) error {
		if seen[name] {
			return fmt.Errorf("%s: %q is declared twice across vars, sensitive vars, bindings and external-service projections", ErrorEnvironmentConflict, name)
		}
		seen[name] = true
		names = append(names, name)
		return nil
	}
	for _, name := range d.Vars {
		if err := add(name); err != nil {
			return nil, err
		}
	}
	for _, name := range d.SensitiveVars {
		if err := add(name); err != nil {
			return nil, err
		}
	}
	for _, binding := range d.Bindings {
		if err := add(binding.Name); err != nil {
			return nil, err
		}
	}
	for _, service := range d.ExternalServices {
		for _, name := range ExternalServiceProjectedNames(service.Name, service.Protocol) {
			if err := add(name); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

// BindingNamed returns the declared binding of that name.
func (d Deployment) BindingNamed(name string) (Binding, bool) {
	for _, binding := range d.Bindings {
		if binding.Name == name {
			return binding, true
		}
	}
	return Binding{}, false
}

// DeriveExportedHandlers reads a module's own bytes and reports which of the
// ABI's handler vocabulary its default export exposes as own properties.
//
// It is the reference derivation, not a JavaScript engine: it recognises a
// default export that is an object literal, either written inline or bound to
// a single top-level identifier. Anything it cannot read statically — a spread
// element, a computed key, a default export that is a call result — is
// reported as ErrModuleSyntax rather than guessed at, because guessing here
// would either invent an export a runtime will not find or hide one it will.
func DeriveExportedHandlers(source []byte, vocabulary []string) ([]string, error) {
	code, inString, err := sanitize(source)
	if err != nil {
		return nil, err
	}
	start, err := defaultExportObject(code, inString)
	if err != nil {
		return nil, err
	}
	keys, err := objectLiteralKeys(code, inString, start)
	if err != nil {
		return nil, err
	}
	exported := map[string]bool{}
	for _, key := range keys {
		exported[key] = true
	}
	handlers := []string{}
	for _, handler := range vocabulary {
		if exported[handler] {
			handlers = append(handlers, handler)
		}
	}
	return handlers, nil
}

// PeerIdentityBinding is the top-level constant a corpus module declares to
// say which worker it is. It is read out of module BYTES, the way the exported
// handler set is, because that is the only source a worker's answer can come
// from: a host that dispatches a `worker.service` call to the wrong worker —
// most cheaply, back into the caller — runs bytes that do not carry the
// callee's identity and cannot stamp it on what it returns.
const PeerIdentityBinding = "PEER_IDENTITY"

// DerivePeerIdentity reads the module's own bytes and reports the identity its
// top-level PEER_IDENTITY binding declares, if it declares one. A module with
// no such binding has no identity to stamp, which is the ordinary case: only a
// callee needs to be distinguishable from whoever might have answered for it.
//
// It is the same reference derivation DeriveExportedHandlers is, held to the
// same rule: what it cannot read statically it does not guess at. A binding
// that is not a plain string literal is reported as ErrModuleSyntax rather than
// silently treated as absent, because an identity nobody can read is
// indistinguishable from an identity nobody has.
func DerivePeerIdentity(source []byte) (string, error) {
	code, inString, err := sanitize(source)
	if err != nil {
		return "", err
	}
	for _, keyword := range []string{"const", "let", "var"} {
		at := findToken(code, inString, keyword, 0)
		for at >= 0 {
			cursor := skipSpace(code, at+len(keyword))
			name, end := readIdentifier(code, cursor)
			if name != PeerIdentityBinding {
				at = findToken(code, inString, keyword, at+1)
				continue
			}
			cursor = skipSpace(code, end)
			if cursor >= len(code) || code[cursor] != '=' {
				return "", fmt.Errorf("%w: %s is not bound to a value", ErrModuleSyntax, PeerIdentityBinding)
			}
			cursor = skipSpace(code, cursor+1)
			if cursor >= len(code) || (code[cursor] != '"' && code[cursor] != '\'') {
				return "", fmt.Errorf(
					"%w: %s is not bound to a string literal", ErrModuleSyntax, PeerIdentityBinding)
			}
			quote := code[cursor]
			start := cursor + 1
			finish := start
			for finish < len(code) && !(code[finish] == quote && !inString[finish]) {
				finish++
			}
			if finish >= len(code) {
				return "", fmt.Errorf("%w: %s is truncated", ErrModuleSyntax, PeerIdentityBinding)
			}
			return string(code[start:finish]), nil
		}
	}
	return "", nil
}

// sanitize replaces comment bytes with spaces and returns a mask marking every
// byte that lives inside a string or template literal, so the structural scan
// below never reads a brace out of a string or a keyword out of a comment.
func sanitize(source []byte) ([]byte, []bool, error) {
	code := make([]byte, len(source))
	copy(code, source)
	inString := make([]bool, len(source))
	index := 0
	for index < len(code) {
		character := code[index]
		switch {
		case character == '/' && index+1 < len(code) && code[index+1] == '/':
			for index < len(code) && code[index] != '\n' {
				code[index] = ' '
				index++
			}
		case character == '/' && index+1 < len(code) && code[index+1] == '*':
			closed := false
			for index < len(code) {
				if code[index] == '*' && index+1 < len(code) && code[index+1] == '/' {
					code[index] = ' '
					code[index+1] = ' '
					index += 2
					closed = true
					break
				}
				if code[index] != '\n' {
					code[index] = ' '
				}
				index++
			}
			if !closed {
				return nil, nil, fmt.Errorf("%w: unterminated block comment", ErrModuleSyntax)
			}
		case character == '\'' || character == '"' || character == '`':
			quote := character
			index++
			terminated := false
			for index < len(code) {
				if code[index] == '\\' {
					inString[index] = true
					if index+1 < len(code) {
						inString[index+1] = true
					}
					index += 2
					continue
				}
				if code[index] == quote {
					index++
					terminated = true
					break
				}
				inString[index] = true
				index++
			}
			if !terminated {
				return nil, nil, fmt.Errorf("%w: unterminated string literal", ErrModuleSyntax)
			}
		default:
			index++
		}
	}
	return code, inString, nil
}

// defaultExportObject returns the offset of the `{` that opens the object the
// module default-exports.
func defaultExportObject(code []byte, inString []bool) (int, error) {
	exportAt := findToken(code, inString, "export", 0)
	for exportAt >= 0 {
		after := skipSpace(code, exportAt+len("export"))
		if hasToken(code, inString, "default", after) {
			cursor := skipSpace(code, after+len("default"))
			if cursor >= len(code) {
				return 0, fmt.Errorf("%w: export default has no value", ErrModuleSyntax)
			}
			if code[cursor] == '{' && !inString[cursor] {
				return cursor, nil
			}
			identifier, end := readIdentifier(code, cursor)
			if identifier == "" {
				return 0, fmt.Errorf("%w: export default is not an object literal or a bound identifier", ErrModuleSyntax)
			}
			_ = end
			return bindingObject(code, inString, identifier)
		}
		exportAt = findToken(code, inString, "export", exportAt+1)
	}
	return 0, fmt.Errorf("%w: the module has no default export", ErrModuleSyntax)
}

// bindingObject finds the object literal bound to a top-level identifier.
func bindingObject(code []byte, inString []bool, identifier string) (int, error) {
	for _, keyword := range []string{"const", "let", "var"} {
		at := findToken(code, inString, keyword, 0)
		for at >= 0 {
			cursor := skipSpace(code, at+len(keyword))
			name, end := readIdentifier(code, cursor)
			if name == identifier {
				cursor = skipSpace(code, end)
				if cursor < len(code) && code[cursor] == '=' {
					cursor = skipSpace(code, cursor+1)
					if cursor < len(code) && code[cursor] == '{' && !inString[cursor] {
						return cursor, nil
					}
				}
				return 0, fmt.Errorf("%w: %q is not bound to an object literal", ErrModuleSyntax, identifier)
			}
			at = findToken(code, inString, keyword, at+1)
		}
	}
	return 0, fmt.Errorf("%w: the default export identifier %q has no top-level binding", ErrModuleSyntax, identifier)
}

// objectLiteralKeys reads the own property names of the object literal opening
// at start.
func objectLiteralKeys(code []byte, inString []bool, start int) ([]string, error) {
	keys := []string{}
	depth := 1
	index := start + 1
	expectKey := true
	for index < len(code) {
		character := code[index]
		if inString[index] {
			index++
			continue
		}
		if expectKey {
			cursor := skipSpace(code, index)
			if cursor >= len(code) {
				break
			}
			if code[cursor] == '}' {
				index = cursor
				expectKey = false
				continue
			}
			if code[cursor] == '.' {
				return nil, fmt.Errorf("%w: a spread element makes the export set underivable", ErrModuleSyntax)
			}
			if code[cursor] == '[' {
				return nil, fmt.Errorf("%w: a computed key makes the export set underivable", ErrModuleSyntax)
			}
			key, next, err := readKey(code, inString, cursor)
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
			index = next
			expectKey = false
			continue
		}
		switch character {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
			if depth == 0 {
				return keys, nil
			}
		case ',':
			if depth == 1 {
				expectKey = true
			}
		}
		index++
	}
	return nil, fmt.Errorf("%w: the default export object literal is never closed", ErrModuleSyntax)
}

// readKey reads one property key, skipping the `async`, `get`, `set` and `*`
// modifiers a method definition may carry.
func readKey(code []byte, inString []bool, cursor int) (string, int, error) {
	for {
		cursor = skipSpace(code, cursor)
		if cursor >= len(code) {
			return "", 0, fmt.Errorf("%w: a property key is truncated", ErrModuleSyntax)
		}
		if code[cursor] == '*' {
			cursor++
			continue
		}
		if code[cursor] == '\'' || code[cursor] == '"' {
			quote := code[cursor]
			end := cursor + 1
			for end < len(code) && !(code[end] == quote && !inString[end]) {
				end++
			}
			if end >= len(code) {
				return "", 0, fmt.Errorf("%w: a quoted property key is truncated", ErrModuleSyntax)
			}
			return string(code[cursor+1 : end]), end + 1, nil
		}
		name, end := readIdentifier(code, cursor)
		if name == "" {
			return "", 0, fmt.Errorf("%w: a property key is not an identifier", ErrModuleSyntax)
		}
		next := skipSpace(code, end)
		if (name == "async" || name == "get" || name == "set" || name == "static") &&
			next < len(code) && (isIdentifierByte(code[next]) || code[next] == '*' ||
			code[next] == '\'' || code[next] == '"') {
			cursor = next
			continue
		}
		return name, end, nil
	}
}

func skipSpace(code []byte, index int) int {
	for index < len(code) {
		switch code[index] {
		case ' ', '\t', '\r', '\n':
			index++
		default:
			return index
		}
	}
	return index
}

func isIdentifierByte(character byte) bool {
	return character == '_' || character == '$' ||
		(character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}

func readIdentifier(code []byte, index int) (string, int) {
	start := index
	for index < len(code) && isIdentifierByte(code[index]) {
		index++
	}
	return string(code[start:index]), index
}

func hasToken(code []byte, inString []bool, token string, index int) bool {
	if index+len(token) > len(code) || inString[index] {
		return false
	}
	if string(code[index:index+len(token)]) != token {
		return false
	}
	if index > 0 && isIdentifierByte(code[index-1]) {
		return false
	}
	if index+len(token) < len(code) && isIdentifierByte(code[index+len(token)]) {
		return false
	}
	return true
}

func findToken(code []byte, inString []bool, token string, from int) int {
	for index := from; index+len(token) <= len(code); index++ {
		if hasToken(code, inString, token, index) {
			return index
		}
	}
	return -1
}

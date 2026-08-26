package currentformsnapshot

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// InterfaceArtifact is one digest-pinned, data-only Interface Definition.
// Interface publication and trust admission are separate from compilation;
// Core revalidates the exact normative document and its digest here.
type InterfaceArtifact struct {
	Origin         string
	ExpectedDigest string
	Definition     []byte
}

// BindingArtifact is one digest-pinned, data-only Binding Definition.
type BindingArtifact struct {
	Origin         string
	ExpectedDigest string
	Definition     []byte
}

// Interface is the immutable public identity of one compiled Interface.
type Interface struct {
	Ref formpackage.InterfaceRef `json:"ref"`
}

// Binding is the immutable public identity and exact target Interface of one
// compiled Binding contract.
type Binding struct {
	Ref             formpackage.BindingRef   `json:"ref"`
	TargetInterface formpackage.InterfaceRef `json:"targetInterface"`
}

type interfaceKey struct {
	apiVersion, name, version, digest string
}

func interfaceKeyFor(ref formpackage.InterfaceRef) interfaceKey {
	return interfaceKey{ref.APIVersion, ref.Name, ref.Version, ref.SchemaDigest}
}

func (key interfaceKey) ref() formpackage.InterfaceRef {
	return formpackage.InterfaceRef{APIVersion: key.apiVersion, Name: key.name, Version: key.version, SchemaDigest: key.digest}
}

func (key interfaceKey) String() string {
	return fmt.Sprintf("%s/%s@%s schema=%s", key.apiVersion, key.name, key.version, key.digest)
}

type bindingKey struct {
	apiVersion, name, version, digest string
}

func bindingKeyFor(ref formpackage.BindingRef) bindingKey {
	return bindingKey{ref.APIVersion, ref.Name, ref.Version, ref.SchemaDigest}
}

func (key bindingKey) ref() formpackage.BindingRef {
	return formpackage.BindingRef{APIVersion: key.apiVersion, Name: key.name, Version: key.version, SchemaDigest: key.digest}
}

func (key bindingKey) String() string {
	return fmt.Sprintf("%s/%s@%s schema=%s", key.apiVersion, key.name, key.version, key.digest)
}

type interfaceDefinitionDocument struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Operations []struct {
		Name string `json:"name"`
	} `json:"operations"`
}

type bindingDefinitionDocument struct {
	APIVersion        string                   `json:"apiVersion"`
	Kind              string                   `json:"kind"`
	Name              string                   `json:"name"`
	Version           string                   `json:"version"`
	SourceRole        string                   `json:"sourceRole"`
	TargetInterface   formpackage.InterfaceRef `json:"targetInterface"`
	RuntimeProjection struct {
		Operations  []string `json:"operations"`
		AccessModes []struct {
			Operations []string `json:"operations"`
		} `json:"accessModes,omitempty"`
	} `json:"runtimeProjection"`
}

type compiledInterface struct {
	public     Interface
	document   interfaceDefinitionDocument
	canonical  []byte
	origin     string
	operations map[string]struct{}
}

type compiledBinding struct {
	public    Binding
	document  bindingDefinitionDocument
	canonical []byte
	origin    string
}

func compileContracts(interfaceArtifacts []InterfaceArtifact, bindingArtifacts []BindingArtifact) (
	map[interfaceKey]compiledInterface,
	[]interfaceKey,
	map[bindingKey]compiledBinding,
	[]bindingKey,
	[]Diagnostic,
) {
	diagnostics := make([]Diagnostic, 0)
	interfaceCandidates := make(map[interfaceKey][]compiledInterface)
	for _, artifact := range interfaceArtifacts {
		compiled, artifactDiagnostics := compileInterfaceArtifact(artifact)
		diagnostics = append(diagnostics, artifactDiagnostics...)
		if len(artifactDiagnostics) == 0 {
			key := interfaceKeyFor(compiled.public.Ref)
			interfaceCandidates[key] = append(interfaceCandidates[key], compiled)
		}
	}
	bindingCandidates := make(map[bindingKey][]compiledBinding)
	for _, artifact := range bindingArtifacts {
		compiled, artifactDiagnostics := compileBindingArtifact(artifact)
		diagnostics = append(diagnostics, artifactDiagnostics...)
		if len(artifactDiagnostics) == 0 {
			key := bindingKeyFor(compiled.public.Ref)
			bindingCandidates[key] = append(bindingCandidates[key], compiled)
		}
	}
	if len(diagnostics) != 0 {
		return nil, nil, nil, nil, sortedDiagnostics(diagnostics)
	}

	interfaceOrder := sortedInterfaceKeys(interfaceCandidates)
	interfaces := make(map[interfaceKey]compiledInterface, len(interfaceCandidates))
	for _, key := range interfaceOrder {
		matches := interfaceCandidates[key]
		if len(matches) != 1 {
			diagnostics = append(diagnostics, duplicateContractDiagnostic(key.String(), interfaceOrigins(matches)))
			continue
		}
		interfaces[key] = matches[0]
	}
	bindingOrder := sortedBindingKeys(bindingCandidates)
	bindings := make(map[bindingKey]compiledBinding, len(bindingCandidates))
	for _, key := range bindingOrder {
		matches := bindingCandidates[key]
		if len(matches) != 1 {
			diagnostics = append(diagnostics, duplicateContractDiagnostic(key.String(), bindingOrigins(matches)))
			continue
		}
		bindings[key] = matches[0]
	}
	if len(diagnostics) != 0 {
		return nil, nil, nil, nil, sortedDiagnostics(diagnostics)
	}

	for _, key := range bindingOrder {
		binding := bindings[key]
		target, ok := interfaces[interfaceKeyFor(binding.document.TargetInterface)]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:    DiagnosticUnresolvedReference,
				Subject: key.String(),
				Pointer: "/targetInterface",
				Message: fmt.Sprintf("exact target Interface %s is absent from the Snapshot", interfaceKeyFor(binding.document.TargetInterface)),
			})
			continue
		}
		operations := append([]string(nil), binding.document.RuntimeProjection.Operations...)
		for _, mode := range binding.document.RuntimeProjection.AccessModes {
			operations = append(operations, mode.Operations...)
		}
		for _, operation := range operations {
			if _, ok := target.operations[operation]; !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Code:    DiagnosticInvalidArtifact,
					Subject: key.String(),
					Pointer: "/runtimeProjection/operations",
					Message: fmt.Sprintf("operation %q is absent from exact target Interface %s", operation, interfaceKeyFor(binding.document.TargetInterface)),
				})
			}
		}
	}
	if len(diagnostics) != 0 {
		return nil, nil, nil, nil, sortedDiagnostics(diagnostics)
	}
	return interfaces, interfaceOrder, bindings, bindingOrder, nil
}

func compileInterfaceArtifact(artifact InterfaceArtifact) (compiledInterface, []Diagnostic) {
	subject := contractArtifactSubject(artifact.Origin, artifact.Definition)
	if !formpackage.ValidDigest(artifact.ExpectedDigest) {
		return compiledInterface{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/expectedDigest", Message: "expected Interface digest is not a canonical lowercase sha256 digest"}}
	}
	if err := formpackage.ValidateInterfaceDefinition(artifact.Definition); err != nil {
		return compiledInterface{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	digest, err := formpackage.DigestCanonicalJSON(artifact.Definition)
	if err != nil {
		return compiledInterface{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	if digest != artifact.ExpectedDigest {
		return compiledInterface{}, []Diagnostic{{Code: DiagnosticDigestMismatch, Subject: subject, Pointer: "/definition", Message: fmt.Sprintf("Interface Definition digest is %s, expected %s", digest, artifact.ExpectedDigest)}}
	}
	var document interfaceDefinitionDocument
	// The normative schema above has already closed the complete document. This
	// projection intentionally decodes only the identity and operation members
	// Core needs; it is not a second, partial wire schema.
	if err := json.Unmarshal(artifact.Definition, &document); err != nil {
		return compiledInterface{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	canonical, err := formpackage.Canonicalize(artifact.Definition)
	if err != nil {
		return compiledInterface{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	ref := formpackage.InterfaceRef{APIVersion: document.APIVersion, Name: document.Name, Version: document.Version, SchemaDigest: digest}
	operations := make(map[string]struct{}, len(document.Operations))
	for _, operation := range document.Operations {
		operations[operation.Name] = struct{}{}
	}
	return compiledInterface{public: Interface{Ref: ref}, document: document, canonical: append([]byte(nil), canonical...), origin: subject, operations: operations}, nil
}

func compileBindingArtifact(artifact BindingArtifact) (compiledBinding, []Diagnostic) {
	subject := contractArtifactSubject(artifact.Origin, artifact.Definition)
	if !formpackage.ValidDigest(artifact.ExpectedDigest) {
		return compiledBinding{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/expectedDigest", Message: "expected Binding digest is not a canonical lowercase sha256 digest"}}
	}
	if err := formpackage.ValidateBindingDefinition(artifact.Definition); err != nil {
		return compiledBinding{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	digest, err := formpackage.DigestCanonicalJSON(artifact.Definition)
	if err != nil {
		return compiledBinding{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	if digest != artifact.ExpectedDigest {
		return compiledBinding{}, []Diagnostic{{Code: DiagnosticDigestMismatch, Subject: subject, Pointer: "/definition", Message: fmt.Sprintf("Binding Definition digest is %s, expected %s", digest, artifact.ExpectedDigest)}}
	}
	var document bindingDefinitionDocument
	// The normative schema above closes members; this projection reads only the
	// exact reference and role facts needed to close the Snapshot graph.
	if err := json.Unmarshal(artifact.Definition, &document); err != nil {
		return compiledBinding{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	canonical, err := formpackage.Canonicalize(artifact.Definition)
	if err != nil {
		return compiledBinding{}, []Diagnostic{{Code: DiagnosticInvalidArtifact, Subject: subject, Pointer: "/definition", Message: err.Error()}}
	}
	ref := formpackage.BindingRef{APIVersion: document.APIVersion, Name: document.Name, Version: document.Version, SchemaDigest: digest}
	return compiledBinding{
		public:   Binding{Ref: ref, TargetInterface: document.TargetInterface},
		document: document, canonical: append([]byte(nil), canonical...), origin: subject,
	}, nil
}

func closeContractReferences(forms map[exactKey]compiledForm, keys []exactKey, interfaces map[interfaceKey]compiledInterface, bindings map[bindingKey]compiledBinding) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for _, key := range keys {
		form := forms[key]
		for index, ref := range form.definition.ProvidedInterfaces {
			if _, ok := interfaces[interfaceKeyFor(ref)]; !ok {
				diagnostics = append(diagnostics, unresolvedInterfaceDiagnostic(key, fmt.Sprintf("/providedInterfaces/%d", index), ref))
			}
		}
		for index, ref := range form.definition.AcceptedBindings {
			binding, ok := bindings[bindingKeyFor(ref)]
			if !ok {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticUnresolvedReference, Subject: key.String(), Pointer: fmt.Sprintf("/acceptedBindings/%d", index),
					Message: fmt.Sprintf("exact Binding %s is absent from the Snapshot", bindingKeyFor(ref)),
				})
				continue
			}
			if binding.document.SourceRole != form.definition.Role {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticInvalidArtifact, Subject: key.String(), Pointer: fmt.Sprintf("/acceptedBindings/%d", index),
					Message: fmt.Sprintf("Binding sourceRole %q does not match Form role %q", binding.document.SourceRole, form.definition.Role),
				})
			}
		}
		annotations := contractAnnotations(form.definition.DesiredSchema, "/desiredSchema")
		for _, annotation := range annotations.interfaces {
			if _, ok := interfaces[interfaceKeyFor(annotation.ref)]; !ok {
				diagnostics = append(diagnostics, unresolvedInterfaceDiagnostic(key, annotation.pointer, annotation.ref))
				continue
			}
			if annotation.bindingName == "" {
				continue
			}
			accepted := acceptedBindingByName(form.definition.AcceptedBindings, annotation.bindingName)
			if len(accepted) != 1 {
				// The x-takoform-binding annotation is diagnosed below. Without
				// one exact accepted Binding there is no targetInterface against
				// which this nested requirement can be compared.
				continue
			}
			binding, ok := bindings[bindingKeyFor(accepted[0])]
			if !ok {
				// The exact accepted Binding's absence is diagnosed below. Do
				// not turn that missing artifact into a misleading mismatch.
				continue
			}
			if interfaceKeyFor(annotation.ref) != interfaceKeyFor(binding.document.TargetInterface) {
				diagnostics = append(diagnostics, Diagnostic{
					Code:    DiagnosticInvalidArtifact,
					Subject: key.String(),
					Pointer: annotation.pointer,
					Message: fmt.Sprintf("Binding annotation %q requires exact Interface %s, but resolved Binding %s targets %s", annotation.bindingName, interfaceKeyFor(annotation.ref), bindingKeyFor(binding.public.Ref), interfaceKeyFor(binding.document.TargetInterface)),
				})
			}
		}
		for _, annotation := range annotations.bindings {
			matches := acceptedBindingByName(form.definition.AcceptedBindings, annotation.name)
			if len(matches) != 1 {
				diagnostics = append(diagnostics, Diagnostic{
					Code: DiagnosticUnresolvedReference, Subject: key.String(), Pointer: annotation.pointer,
					Message: fmt.Sprintf("Binding annotation %q resolves to %d exact accepted Binding refs; want one", annotation.name, len(matches)),
				})
			}
		}
	}
	return sortedDiagnostics(diagnostics)
}

func acceptedBindingByName(bindings []formpackage.BindingRef, name string) []formpackage.BindingRef {
	matches := make([]formpackage.BindingRef, 0, 1)
	for _, binding := range bindings {
		if binding.Name == name {
			matches = append(matches, binding)
		}
	}
	return matches
}

func unresolvedInterfaceDiagnostic(form exactKey, pointer string, ref formpackage.InterfaceRef) Diagnostic {
	return Diagnostic{
		Code: DiagnosticUnresolvedReference, Subject: form.String(), Pointer: pointer,
		Message: fmt.Sprintf("exact Interface %s is absent from the Snapshot", interfaceKeyFor(ref)),
	}
}

type interfaceAnnotation struct {
	pointer     string
	ref         formpackage.InterfaceRef
	bindingName string
}

type bindingAnnotation struct {
	pointer string
	name    string
}

type annotations struct {
	interfaces []interfaceAnnotation
	bindings   []bindingAnnotation
}

func contractAnnotations(value any, pointer string) annotations {
	return contractAnnotationsWithBinding(value, pointer, "", value, pointer, make(map[string]struct{}))
}

func contractAnnotationsWithBinding(value any, pointer, bindingName string, root any, rootPointer string, refStack map[string]struct{}) annotations {
	var output annotations
	switch typed := value.(type) {
	case map[string]any:
		// An x-takoform-binding annotation applies to the complete schema
		// subtree at this node. Establish that context before walking any
		// children so map key order cannot detach a nested required-interface
		// annotation from its binding relation.
		if name, ok := typed["x-takoform-binding"].(string); ok {
			bindingName = name
			output.bindings = append(output.bindings, bindingAnnotation{pointer: pointer + "/x-takoform-binding", name: name})
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childPointer := pointer + "/" + escapePointer(key)
			switch key {
			case "x-takoform-required-interface":
				raw, err := json.Marshal(typed[key])
				if err == nil {
					var ref formpackage.InterfaceRef
					if err := formpackage.DecodeStrictIJSON(raw, &ref); err == nil {
						output.interfaces = append(output.interfaces, interfaceAnnotation{
							pointer: childPointer, ref: ref, bindingName: bindingName,
						})
					}
				}
			case "x-takoform-binding":
				// The annotation was recorded above so its context is active
				// for every sibling and descendant in this schema node.
			case "$ref":
				if bindingName == "" {
					break
				}
				reference, ok := typed[key].(string)
				if !ok {
					break
				}
				target, targetPointer, ok := resolveLocalContractReference(root, rootPointer, reference)
				if !ok {
					// Form Package validation has already closed local $refs and
					// refused remote or unresolved references. Keep this traversal
					// fail-closed if called outside that validated boundary.
					break
				}
				refKey := targetPointer + "\x00" + bindingName
				if _, visiting := refStack[refKey]; visiting {
					break
				}
				refStack[refKey] = struct{}{}
				child := contractAnnotationsWithBinding(target, targetPointer, bindingName, root, rootPointer, refStack)
				delete(refStack, refKey)
				output.interfaces = append(output.interfaces, child.interfaces...)
				output.bindings = append(output.bindings, child.bindings...)
			default:
				child := contractAnnotationsWithBinding(typed[key], childPointer, bindingName, root, rootPointer, refStack)
				output.interfaces = append(output.interfaces, child.interfaces...)
				output.bindings = append(output.bindings, child.bindings...)
			}
		}
	case []any:
		for index, entry := range typed {
			child := contractAnnotationsWithBinding(entry, fmt.Sprintf("%s/%d", pointer, index), bindingName, root, rootPointer, refStack)
			output.interfaces = append(output.interfaces, child.interfaces...)
			output.bindings = append(output.bindings, child.bindings...)
		}
	}
	return output
}

func resolveLocalContractReference(root any, rootPointer, reference string) (any, string, bool) {
	if reference == "#" {
		return root, rootPointer, true
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, "", false
	}
	pointer, err := url.PathUnescape(reference[1:])
	if err != nil {
		return nil, "", false
	}
	current := root
	canonicalPointer := rootPointer
	for _, rawToken := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		token, ok := decodeLocalJSONPointerToken(rawToken)
		if !ok {
			return nil, "", false
		}
		canonicalPointer += "/" + escapePointer(token)
		switch typed := current.(type) {
		case map[string]any:
			child, exists := typed[token]
			if !exists {
				return nil, "", false
			}
			current = child
		case []any:
			if token == "-" || (len(token) > 1 && token[0] == '0') {
				return nil, "", false
			}
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, "", false
			}
			current = typed[index]
		default:
			return nil, "", false
		}
	}
	return current, canonicalPointer, true
}

func decodeLocalJSONPointerToken(value string) (string, bool) {
	var decoded strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			decoded.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) || (value[index+1] != '0' && value[index+1] != '1') {
			return "", false
		}
		index++
		if value[index] == '0' {
			decoded.WriteByte('~')
		} else {
			decoded.WriteByte('/')
		}
	}
	return decoded.String(), true
}

func duplicateContractDiagnostic(subject string, origins []string) Diagnostic {
	sort.Strings(origins)
	return Diagnostic{Code: DiagnosticDuplicateIdentity, Subject: subject, Message: fmt.Sprintf("exact contract identity is supplied %d times by %s", len(origins), strings.Join(origins, ", "))}
}

func interfaceOrigins(values []compiledInterface) []string {
	origins := make([]string, 0, len(values))
	for _, value := range values {
		origins = append(origins, value.origin)
	}
	return origins
}

func bindingOrigins(values []compiledBinding) []string {
	origins := make([]string, 0, len(values))
	for _, value := range values {
		origins = append(origins, value.origin)
	}
	return origins
}

func sortedInterfaceKeys[T any](values map[interfaceKey]T) []interfaceKey {
	keys := make([]interfaceKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keys[i], keys[j]
		if left.apiVersion != right.apiVersion {
			return left.apiVersion < right.apiVersion
		}
		if left.name != right.name {
			return left.name < right.name
		}
		if left.version != right.version {
			return left.version < right.version
		}
		return left.digest < right.digest
	})
	return keys
}

func sortedBindingKeys[T any](values map[bindingKey]T) []bindingKey {
	keys := make([]bindingKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keys[i], keys[j]
		if left.apiVersion != right.apiVersion {
			return left.apiVersion < right.apiVersion
		}
		if left.name != right.name {
			return left.name < right.name
		}
		if left.version != right.version {
			return left.version < right.version
		}
		return left.digest < right.digest
	})
	return keys
}

func contractArtifactSubject(origin string, raw []byte) string {
	if strings.TrimSpace(origin) != "" {
		return origin
	}
	return "bytes:" + formpackage.DigestBytes(raw)
}

// Interfaces returns exact Interface identities in stable order.
func (snapshot *Snapshot) Interfaces() []Interface {
	if snapshot == nil {
		return nil
	}
	output := make([]Interface, 0, len(snapshot.interfaceOrder))
	for _, key := range snapshot.interfaceOrder {
		output = append(output, snapshot.interfaces[key].public)
	}
	return output
}

// Bindings returns exact Binding identities in stable order.
func (snapshot *Snapshot) Bindings() []Binding {
	if snapshot == nil {
		return nil
	}
	output := make([]Binding, 0, len(snapshot.bindingOrder))
	for _, key := range snapshot.bindingOrder {
		output = append(output, snapshot.bindings[key].public)
	}
	return output
}

// InterfaceDefinition returns canonical bytes for an exact InterfaceRef.
func (snapshot *Snapshot) InterfaceDefinition(ref formpackage.InterfaceRef) ([]byte, bool) {
	if snapshot == nil {
		return nil, false
	}
	value, ok := snapshot.interfaces[interfaceKeyFor(ref)]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), value.canonical...), true
}

// BindingDefinition returns canonical bytes for an exact BindingRef.
func (snapshot *Snapshot) BindingDefinition(ref formpackage.BindingRef) ([]byte, bool) {
	if snapshot == nil {
		return nil, false
	}
	value, ok := snapshot.bindings[bindingKeyFor(ref)]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), value.canonical...), true
}

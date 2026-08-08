package edgeformcatalog

import (
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// The five typed Binding contracts of the Edge Platform Family (decision
// 0010). A Binding grants a capability and a concrete runtime API together,
// without exposing credentials; outward capability use always belongs to a
// revision resource.

// BindingAPIVersion is the fixed group of typed Binding contracts.
const BindingAPIVersion = "bindings.takoform.com/v1alpha1"

// BindingDefinition mirrors binding-definition-v1alpha1.schema.json.
type BindingDefinition struct {
	APIVersion         string                   `json:"apiVersion"`
	Kind               string                   `json:"kind"`
	Name               string                   `json:"name"`
	Version            string                   `json:"version"`
	Title              string                   `json:"title,omitempty"`
	Description        string                   `json:"description,omitempty"`
	SourceRole         string                   `json:"sourceRole"`
	TargetInterface    formpackage.InterfaceRef `json:"targetInterface"`
	AllowedTargetForms []AllowedTargetForm      `json:"allowedTargetForms"`
	BindingNameGrammar string                   `json:"bindingNameGrammar"`
	RuntimeProjection  RuntimeProjection        `json:"runtimeProjection"`
	Lifecycle          BindingLifecycle         `json:"lifecycle"`
}

type AllowedTargetForm struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

type RuntimeProjection struct {
	Operations []string `json:"operations"`
}

type BindingLifecycle struct {
	TargetDeletion string `json:"targetDeletion"`
}

type bindingSpec struct {
	name        string
	title       string
	description string
	iface       string
	targetKind  string
	operations  []string // empty projects every interface operation
}

// Every binding description states the JAVASCRIPT SURFACE the binding projects,
// not only which Interface operations it grants. A binding that names
// operations without saying what the caller actually calls leaves the consumer
// guessing the method names, the argument types, the return types, and what a
// failure looks like — which is the same incompleteness decision 0008 forbids
// in a Form. The Binding Definition meta-schema admits `runtimeProjection`
// (operations and access modes) and a `description`, so the projection prose
// lives in the description, per decision 0014's placement rule.
//
// The `env` object these names land in, and the rule that nothing else portable
// appears there, are fixed by the worker.runtime contract (decision 0019).
var bindingSpecs = []bindingSpec{
	{
		name:  "module-worker.edge-kv",
		title: "Module Worker edge KV binding",
		description: "Projects the complete edge.kv runtime API into a Worker Version under one " +
			"JavaScript-identifier binding name. The binding grants capability and API together; " +
			"no credential or endpoint ever reaches the consumer. " +
			"Runtime surface: env.NAME is an object with the methods get(key), getWithMetadata(key), " +
			"put(key, value, options?), delete(key), and list(options?). A key is a string. A VALUE IS " +
			"BYTES: put accepts an ArrayBuffer, a typed array, or a string (encoded UTF-8), and get resolves " +
			"to an ArrayBuffer — the base64 envelope of edge.kv is the wire form, not the JavaScript form. " +
			"Every method returns a promise. get resolves to null for an absent key rather than rejecting, " +
			"which is how the interface's not_found outcome appears in JavaScript; every other error rejects " +
			"with an Error whose name is the edge.kv error code. Reads are EVENTUALLY CONSISTENT and this " +
			"binding promises no read-your-writes: a get immediately after a resolved put by the same isolate " +
			"may resolve to the previous value, or to null, until replication converges.",
		iface:      "edge.kv",
		targetKind: "EdgeKVNamespace",
	},
	{
		name:  "module-worker.object-bucket",
		title: "Module Worker object bucket binding",
		description: "Projects the complete edge.objects runtime API into a Worker Version under one " +
			"JavaScript-identifier binding name, without exposing credentials or endpoints. " +
			"Runtime surface: env.NAME is an object with the methods head(key), get(key, options?), " +
			"put(key, body, options?), delete(key), list(options?), createMultipartUpload(key, options?), " +
			"uploadPart(key, uploadId, partNumber, body), completeMultipartUpload(key, uploadId, parts), and " +
			"abortMultipartUpload(key, uploadId), each returning a promise. Object bodies STREAM: put and " +
			"uploadPart accept a ReadableStream, an ArrayBuffer, or a string, and get resolves to an object " +
			"whose body is a ReadableStream the handler may consume incrementally — a conforming host never " +
			"requires the whole object in memory, which is what makes the contract's 5 GiB ceiling usable. " +
			"get options carry range for a ranged read and ifMatch or ifNoneMatch for a conditional one; put " +
			"options carry ifMatch, or ifNoneMatch \"*\" to write only when the key is absent. get and head " +
			"resolve to null for an absent key; a failed precondition rejects with an Error named " +
			"precondition_failed, an unsatisfiable range with one named range_not_satisfiable, and every other " +
			"error with an Error named for its edge.objects code. A get, head, or list after a resolved put or " +
			"delete observes it.",
		iface:      "edge.objects",
		targetKind: "ObjectBucket",
	},
	{
		name:  "module-worker.sqlite",
		title: "Module Worker SQLite binding",
		description: "Projects the complete edge.sql runtime API into a Worker Version under one " +
			"JavaScript-identifier binding name, without exposing credentials or connection material. " +
			"Runtime surface: env.NAME is an object with the methods execute(sql, params?), " +
			"query(sql, params?), and transaction(statements), each returning a promise. params is an array of " +
			"TAGGED edge.sql values bound positionally, so a 64-bit INTEGER (decimal text) and a BLOB (base64) " +
			"survive the round trip that an untagged JSON scalar would have changed. Every one of the three " +
			"resolves to the same statement-result shape — rows, rowsWritten, and lastInsertRowId — and " +
			"transaction resolves to one such result per statement, in statement order, so a SELECT inside a " +
			"transaction returns its rows without leaving the transaction. Statements apply atomically under " +
			"serializable isolation, so a rejected transaction has applied nothing at all. Errors reject with " +
			"an Error whose name is the edge.sql error code; busy is the retryable one, and a caller that means " +
			"to retry must re-run the whole call.",
		iface:      "edge.sql",
		targetKind: "SQLiteDatabase",
	},
	{
		name:  "module-worker.queue-producer",
		title: "Module Worker queue producer binding",
		description: "Projects only the submission half of edge.queue (send, sendBatch) into a Worker " +
			"Version. Consumption is never a binding: it is the QueueConsumer attachment resource. " +
			"Runtime surface: env.NAME is an object with exactly the methods send(body, options?) and " +
			"sendBatch(messages), each returning a promise that resolves to the message identity or " +
			"identities the host assigned. A body is BYTES: an ArrayBuffer, a typed array, or a string " +
			"encoded UTF-8. There is no receive, no peek, and none of the settlement methods — acknowledge, " +
			"retry, acknowledgeAll, and retryAll exist on the BATCH the module's queue handler receives, " +
			"never here, because settling a message you were not delivered is not an operation. A resolved " +
			"send means ACCEPTED and durable, not delivered — delivery is at-least-once and unordered, so the " +
			"consumer may see duplicates and must be idempotent. Errors reject with an Error named for the " +
			"edge.queue code.",
		iface:      "edge.queue",
		targetKind: "AtLeastOnceQueue",
		operations: []string{"send", "sendBatch"},
	},
	{
		name:  "module-worker.service",
		title: "Module Worker service binding",
		description: "Projects the worker.service fetch operation into a Worker Version, addressing " +
			"another Module Worker by logical identity without any network endpoint. " +
			"Runtime surface: env.NAME.fetch(request) takes a Request (or a URL string with an optional " +
			"init object, exactly like the global fetch) and returns a Promise<Response>. Request and " +
			"response bodies STREAM in both directions; neither side is buffered before the call is made. " +
			"The call is dispatched to whichever Worker Versions the target's active Worker Deployment " +
			"selects, and never leaves the host, so no DNS name, URL, or credential is involved and the " +
			"request host name carries no routing meaning. An UNCAUGHT exception in the callee's fetch " +
			"handler is the callee's host-generated 500 response: this promise RESOLVES with that " +
			"response rather than rejecting, so a caller distinguishes callee failure by status, not by " +
			"catch. The promise rejects only when the call could not be made at all, with an Error named " +
			"backend_unavailable. A response reflects every effect the callee completed before responding.",
		iface:      "worker.service",
		targetKind: "ModuleWorker",
	},
}

// BindingDefinitions lists the Edge Platform Family binding catalog in a
// stable order. Every targetInterface carries the exact resolved digest of
// the referenced Interface Definition.
func BindingDefinitions() ([]BindingDefinition, error) {
	out := make([]BindingDefinition, 0, len(bindingSpecs))
	for _, spec := range bindingSpecs {
		iface, err := interfaceDefinitionByName(spec.iface)
		if err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.name, err)
		}
		ref, err := InterfaceRefFor(spec.iface, iface.Version)
		if err != nil {
			return nil, fmt.Errorf("binding %s: %w", spec.name, err)
		}
		operations := spec.operations
		if len(operations) == 0 {
			operations = make([]string, 0, len(iface.Operations))
			for _, operation := range iface.Operations {
				operations = append(operations, operation.Name)
			}
		} else {
			for _, operation := range operations {
				if !interfaceHasOperation(iface, operation) {
					return nil, fmt.Errorf("binding %s projects unknown %s operation %s", spec.name, spec.iface, operation)
				}
			}
		}
		out = append(out, BindingDefinition{
			APIVersion: BindingAPIVersion, Kind: "BindingDefinition",
			Name: spec.name, Version: "1.0.0",
			Title: spec.title, Description: spec.description,
			SourceRole:      string(currentformmodel.RoleRevision),
			TargetInterface: ref,
			AllowedTargetForms: []AllowedTargetForm{
				{APIVersion: Family.APIVersion(), Kind: spec.targetKind},
			},
			BindingNameGrammar: currentformmodel.PatternBindingName,
			RuntimeProjection:  RuntimeProjection{Operations: operations},
			Lifecycle:          BindingLifecycle{TargetDeletion: "refuse_while_bound"},
		})
	}
	return out, nil
}

func interfaceDefinitionByName(name string) (InterfaceDefinition, error) {
	for _, candidate := range InterfaceDefinitions() {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return InterfaceDefinition{}, fmt.Errorf("interface %q is not in the catalog", name)
}

func interfaceHasOperation(iface InterfaceDefinition, name string) bool {
	for _, operation := range iface.Operations {
		if operation.Name == name {
			return true
		}
	}
	return false
}

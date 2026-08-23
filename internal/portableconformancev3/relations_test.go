package portableconformancev3

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// relationFixture installs a KV namespace and the Worker Version that binds it,
// through the real wire, and returns both stored records.
func relationFixture(t *testing.T) (*ReferenceHost, Contract, *storedResource, *storedResource) {
	t.Helper()
	host, contract := fallbackHost(t)
	group := contract.RunnerInput.EdgeKvNamespace.Identity.FormRef.APIVersion
	host.storeResource(&storedResource{
		Ref: mustProbeRef(t, contract, "ModuleWorker"), Name: "module-worker-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-worker", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	host.storeResource(&storedResource{
		Ref: mustProbeRef(t, contract, "WorkerBundle"), Name: "worker-bundle-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-bundle", Generation: 1, Revision: 1,
		Spec: map[string]any{"manifestDigest": formpackage.DigestBytes([]byte("bundle"))},
	})
	host.storeResource(&storedResource{
		Ref: mustProbeRef(t, contract, "EdgeKVNamespace"), Name: "edge-kv-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-kv", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	versionForm := host.probeForm("WorkerVersion")
	spec := relationVersionSpec(group, "edge-kv-probe")
	relations, hostErr := host.resolveRelations(versionForm, referencePrimaryAuth.scope("conformance"), spec)
	if hostErr != nil {
		t.Fatalf("resolve relations: %+v", hostErr)
	}
	digest, err := specCanonicalDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	version := &storedResource{
		Ref: mustProbeRef(t, contract, "WorkerVersion"), Name: "worker-version-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-version", Generation: 1, Revision: 1,
		Spec: spec, SpecDigest: digest, Relations: relations,
	}
	host.storeResource(version)
	return host, contract, host.resources[resourceKey(referencePrimaryAuth.scope("conformance"), group, "EdgeKVNamespace", "edge-kv-probe")], version
}

func relationVersionSpec(group, kvName string) map[string]any {
	return map[string]any{
		"worker":   map[string]any{"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker-probe"},
		"bundle":   map[string]any{"apiVersion": group, "kind": "WorkerBundle", "name": "worker-bundle-probe"},
		"handlers": []any{"fetch"},
		"kvBindings": []any{map[string]any{
			"name":     "CACHE",
			"resource": map[string]any{"apiVersion": group, "kind": "EdgeKVNamespace", "name": kvName},
		}},
	}
}

// TestResolvedRelationsStoreTheTargetUID is the central storage rule: what a
// host keeps is the resolved UID, not the name it was resolved from. Every
// dependency and drift decision downstream reads that UID.
func TestResolvedRelationsStoreTheTargetUID(t *testing.T) {
	_, _, kv, version := relationFixture(t)
	byPointer := map[string]storedRelation{}
	for _, relation := range version.Relations {
		byPointer[relation.Pointer] = relation
	}
	if len(byPointer) != 3 {
		t.Fatalf("stored relations = %d, want worker, bundle, and the kv binding", len(byPointer))
	}
	binding, present := byPointer["/kvBindings/0/resource"]
	if !present {
		t.Fatalf("no relation stored for the typed binding: %+v", version.Relations)
	}
	if binding.TargetUID != kv.UID {
		t.Fatalf("binding stored target uid %q, want %q", binding.TargetUID, kv.UID)
	}
	if binding.TargetName != "edge-kv-probe" || binding.Relation != "/kvBindings/*/resource" {
		t.Fatalf("binding record = %+v", binding)
	}
	if binding.BindingRef == nil || binding.BindingRef.Name == "" {
		t.Fatalf("a typed binding stored no exact BindingRef: %+v", binding)
	}
	plain, present := byPointer["/worker"]
	if !present {
		t.Fatal("no relation stored for the plain worker reference")
	}
	if plain.TargetUID != "uid-worker" || plain.BindingRef != nil {
		t.Fatalf("plain relation record = %+v", plain)
	}
}

// TestReverseIndexBlocksEveryRelationKind proves deletion protection is not a
// binding scan: a plain reference protects its target exactly as a typed
// binding does, and releasing the holder releases the target.
func TestReverseIndexBlocksEveryRelationKind(t *testing.T) {
	host, _, _, version := relationFixture(t)
	group := version.group()
	for _, target := range []struct {
		kind, name string
	}{
		{"EdgeKVNamespace", "edge-kv-probe"}, // typed binding
		{"ModuleWorker", "module-worker-probe"},
		{"WorkerBundle", "worker-bundle-probe"},
	} {
		stored := host.resources[resourceKey(referencePrimaryAuth.scope("conformance"), group, target.kind, target.name)]
		if stored == nil {
			t.Fatalf("%s %s is not stored", target.kind, target.name)
		}
		if hostErr := host.dependencyInUse(stored); hostErr == nil || hostErr.Code != "dependency_in_use" {
			t.Fatalf("deleting %s %s = %+v, want dependency_in_use", target.kind, target.name, hostErr)
		}
	}
	// The holder itself is referenced by nothing.
	if hostErr := host.dependencyInUse(version); hostErr != nil {
		t.Fatalf("the holder is not a target but reported %+v", hostErr)
	}
	// Releasing the holder releases every target it pinned.
	host.removeResource(resourceKey(referencePrimaryAuth.scope("conformance"), group, version.kind(), version.Name))
	for _, name := range []string{"edge-kv-probe", "module-worker-probe", "worker-bundle-probe"} {
		for _, kind := range []string{"EdgeKVNamespace", "ModuleWorker", "WorkerBundle"} {
			stored := host.resources[resourceKey(referencePrimaryAuth.scope("conformance"), group, kind, name)]
			if stored == nil {
				continue
			}
			if hostErr := host.dependencyInUse(stored); hostErr != nil {
				t.Fatalf("%s %s stayed pinned after the holder was removed: %+v", kind, name, hostErr)
			}
		}
	}
	if len(host.relationHolders) != 0 {
		t.Fatalf("the reverse index kept %d entries after the only holder was removed", len(host.relationHolders))
	}
}

// TestRecreatedTargetIsNotRebound proves the incarnation rule end to end over
// the wire: a target destroyed and recreated under the same name leaves the
// source reporting ExternalChange, never quietly pointing at the new resource.
func TestRecreatedTargetIsNotRebound(t *testing.T) {
	host, contract, kv, version := relationFixture(t)
	server := httptest.NewServer(host)
	defer server.Close()
	versionRef := contract.RunnerInput.WorkerVersion.Identity.FormRef
	read := func() wireCondition {
		t.Helper()
		status, raw := hostRequest(t, server, http.MethodGet,
			resourcePath(contract, versionRef, version.Name, "", exactQueryValues("conformance", versionRef)),
			nil, nil)
		if status != http.StatusOK {
			t.Fatalf("read = %d %s", status, strings.TrimSpace(string(raw)))
		}
		return readyCondition(decodeHostResource(t, raw))
	}
	if condition := read(); condition.Status != "True" || condition.Reason != "Available" {
		t.Fatalf("a freshly bound source reports %s/%s", condition.Status, condition.Reason)
	}

	// The target vanishes: the source reports the missing dependency.
	key := resourceKey(referencePrimaryAuth.scope("conformance"), kv.group(), kv.kind(), kv.Name)
	host.removeResource(key)
	if condition := read(); condition.Status != "False" || condition.Reason != "DependencyMissing" {
		t.Fatalf("source of a vanished target reports %s/%s", condition.Status, condition.Reason)
	}

	// The same NAME comes back on a different incarnation.
	host.storeResource(&storedResource{
		Ref: kv.Ref, Name: kv.Name, Tenant: kv.Tenant, Space: kv.Space,
		UID: "uid-kv-second", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	condition := read()
	if condition.Status != "False" || condition.Reason != "ExternalChange" {
		t.Fatalf("source of a recreated target reports %s/%s, want False/ExternalChange", condition.Status, condition.Reason)
	}
	for _, want := range []string{"/kvBindings/0/resource", "uid-kv", "uid-kv-second"} {
		if !strings.Contains(condition.HostReason, want) {
			t.Fatalf("hostReason %q does not name %q", condition.HostReason, want)
		}
	}
	// Reading did not re-bind: the stored relation still pins the old uid.
	stored := host.resources[resourceKey(referencePrimaryAuth.scope("conformance"), version.group(), version.kind(), version.Name)]
	for _, relation := range stored.Relations {
		if relation.Pointer == "/kvBindings/0/resource" && relation.TargetUID != "uid-kv" {
			t.Fatalf("a read re-bound the relation to uid %q", relation.TargetUID)
		}
	}
	// And the new incarnation is NOT protected by the stale holder.
	replacement := host.resources[key]
	if hostErr := host.dependencyInUse(replacement); hostErr != nil {
		t.Fatalf("the new incarnation inherited the old holder: %+v", hostErr)
	}
}

// TestBindingContractRulesAreVerified walks every rule of
// spec/binding-contract/README.md by breaking exactly one at a time. Each must
// be refused before anything is stored.
func TestBindingContractRulesAreVerified(t *testing.T) {
	group := "edge.forms.takoform.com/v1beta2"
	cases := []struct {
		name    string
		break_  func(host *ReferenceHost, source, target *InstalledForm)
		code    string
		message string
	}{
		{
			name: "source Definition does not accept the binding",
			break_: func(_ *ReferenceHost, source, _ *InstalledForm) {
				source.AcceptedBindings = nil
			},
			code:    "invalid_argument",
			message: "does not accept",
		},
		{
			name: "host has not installed the binding contract",
			break_: func(host *ReferenceHost, _, _ *InstalledForm) {
				host.catalog.contracts = map[string]bindingContract{}
			},
			code:    "unsupported_capability",
			message: "declares no support",
		},
		{
			name: "the installed contract is a different digest",
			break_: func(host *ReferenceHost, source, _ *InstalledForm) {
				ref := source.AcceptedBindings[0]
				contract := host.catalog.contracts[ref.Name+"@"+ref.Version]
				contract.Ref.SchemaDigest = formpackage.DigestBytes([]byte("other-binding"))
				host.catalog.contracts[ref.Name+"@"+ref.Version] = contract
			},
			code:    "unsupported_capability",
			message: "declares no support",
		},
		{
			name: "source role is not the binding sourceRole",
			break_: func(_ *ReferenceHost, source, _ *InstalledForm) {
				source.Role = "identity"
			},
			code:    "invalid_argument",
			message: "sourceRole",
		},
		{
			name: "target Form is not an allowed target",
			break_: func(host *ReferenceHost, source, _ *InstalledForm) {
				ref := source.AcceptedBindings[0]
				contract := host.catalog.contracts[ref.Name+"@"+ref.Version]
				contract.AllowedTargetForms = []allowedTargetForm{{APIVersion: group, Kind: "ObjectBucket"}}
				host.catalog.contracts[ref.Name+"@"+ref.Version] = contract
			},
			code:    "invalid_argument",
			message: "allowedTargetForms",
		},
		{
			name: "target Form does not provide the required Interface",
			break_: func(_ *ReferenceHost, _, target *InstalledForm) {
				target.ProvidedInterfaces = nil
			},
			code:    "invalid_argument",
			message: "does not provide",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			host, _, _, _ := relationFixture(t)
			source := host.probeForm("WorkerVersion")
			target := host.probeForm("EdgeKVNamespace")
			testCase.break_(host, source, target)
			spec := relationVersionSpec(group, "edge-kv-probe")
			_, hostErr := host.resolveRelations(source, referencePrimaryAuth.scope("conformance"), spec)
			if hostErr == nil {
				t.Fatalf("a broken binding contract was accepted")
			}
			if hostErr.Code != testCase.code || !strings.Contains(hostErr.Message, testCase.message) {
				t.Fatalf("hostErr = %+v, want %s naming %q", hostErr, testCase.code, testCase.message)
			}
		})
	}

	// The unbroken catalog accepts the same spec, so every case above failed
	// for the rule it broke and nothing else.
	host, _, _, _ := relationFixture(t)
	source := host.probeForm("WorkerVersion")
	if _, hostErr := host.resolveRelations(source, referencePrimaryAuth.scope("conformance"), relationVersionSpec(group, "edge-kv-probe")); hostErr != nil {
		t.Fatalf("the intact binding contract was refused: %+v", hostErr)
	}
}

// TestHandlerGateMatchesTheResolvedWorkerIncarnation proves the attachment gate
// is a statement about one worker INCARNATION, not one worker name. A stale
// WorkerDeployment pinned to a ModuleWorker that was replaced out of band
// governs traffic for a resource that is gone, so it cannot open the gate for a
// new attachment; a deployment of the current incarnation can.
func TestHandlerGateMatchesTheResolvedWorkerIncarnation(t *testing.T) {
	// The installed candidate catalog, not the fallback one: this rule is about
	// the attachment Forms, which only the real Edge Family declares.
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	catalog, err := LoadCatalog(filepath.Join("..", ".."), contract)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	host := NewReferenceHost(contract, catalog)
	const group, space = testEdgeFormsGroup, "conformance"
	workerReference := map[string]any{"apiVersion": group, "kind": "ModuleWorker", "name": "module-worker-probe"}
	storeWorker := func(uid string) {
		host.storeResource(&storedResource{
			Ref: mustProbeRef(t, contract, "ModuleWorker"), Name: "module-worker-probe",
			Tenant: referencePrimaryAuth.Tenant, Space: space,
			UID: uid, Generation: 1, Revision: 1, Spec: map[string]any{},
		})
	}
	storeVersion := func(name, workerUID string) {
		host.storeResource(&storedResource{
			Ref: mustProbeRef(t, contract, "WorkerVersion"), Name: name,
			Tenant: referencePrimaryAuth.Tenant, Space: space,
			UID: "uid-" + name, Generation: 1, Revision: 1,
			Spec: map[string]any{"worker": workerReference, "handlers": []any{"fetch", "queue"}},
			Relations: []storedRelation{{
				Pointer: "/worker", Relation: "/worker",
				TargetAPIVersion: group, TargetKind: "ModuleWorker",
				TargetName: "module-worker-probe", TargetUID: workerUID,
			}},
		})
	}
	storeDeployment := func(name, workerUID, version string) {
		host.storeResource(&storedResource{
			Ref: mustProbeRef(t, contract, "WorkerDeployment"), Name: name,
			Tenant: referencePrimaryAuth.Tenant, Space: space,
			UID: "uid-" + name, Generation: 1, Revision: 1,
			Spec: map[string]any{"worker": workerReference, "versions": []any{}},
			Relations: []storedRelation{
				{
					Pointer: "/worker", Relation: "/worker",
					TargetAPIVersion: group, TargetKind: "ModuleWorker",
					TargetName: "module-worker-probe", TargetUID: workerUID,
				},
				{
					Pointer: "/versions/0/workerVersion", Relation: "/versions/*/workerVersion",
					TargetAPIVersion: group, TargetKind: "WorkerVersion",
					TargetName: version, TargetUID: "uid-" + version,
				},
			},
		})
	}
	storeWorker("uid-worker-first")
	host.storeResource(&storedResource{
		Ref: mustProbeRef(t, contract, "AtLeastOnceQueue"), Name: "queue-probe",
		Tenant: referencePrimaryAuth.Tenant, Space: space,
		UID: "uid-queue", Generation: 1, Revision: 1, Spec: map[string]any{},
	})
	storeVersion("stale-version", "uid-worker-first")
	storeDeployment("stale-deployment", "uid-worker-first", "stale-version")
	// The worker is replaced out of band: same name, different resource. The
	// stale deployment stays stored, still pinned to the incarnation that is
	// gone, and so does the version it weights.
	storeWorker("uid-worker-second")

	consumer := host.probeForm("QueueConsumer")
	spec := consumer.materialize(map[string]any{
		"worker": workerReference,
		"queue":  map[string]any{"apiVersion": group, "kind": "AtLeastOnceQueue", "name": "queue-probe"},
	})
	_, hostErr := host.validateDesiredSemantics(
		referencePrimaryAuth, consumer, referencePrimaryAuth.scope(space), "queue-consumer-probe", spec,
	)
	if hostErr == nil || hostErr.Code != "unsupported_capability" {
		t.Fatalf("a stale deployment of the previous worker opened the gate: %+v", hostErr)
	}
	if !strings.Contains(hostErr.Message, "uid-worker-second") {
		t.Fatalf("the refusal does not name the resolved worker incarnation: %s", hostErr.Message)
	}

	// A deployment of the CURRENT incarnation is what the gate is asking for.
	storeVersion("current-version", "uid-worker-second")
	storeDeployment("current-deployment", "uid-worker-second", "current-version")
	if _, hostErr := host.validateDesiredSemantics(
		referencePrimaryAuth, consumer, referencePrimaryAuth.scope(space), "queue-consumer-probe", spec,
	); hostErr != nil {
		t.Fatalf("a deployment of the current worker was refused: %+v", hostErr)
	}
}

// TestRelationTargetMustExistBeforeMutation proves the pre-mutation refusal for
// a NON-binding reference, which the superseded binding-only scan never saw.
func TestRelationTargetMustExistBeforeMutation(t *testing.T) {
	host, _, _, version := relationFixture(t)
	group := version.group()
	source := host.probeForm("WorkerVersion")
	spec := relationVersionSpec(group, "edge-kv-probe")
	spec["worker"] = map[string]any{"apiVersion": group, "kind": "ModuleWorker", "name": "absent-worker"}
	_, hostErr := host.resolveRelations(source, referencePrimaryAuth.scope("conformance"), spec)
	if hostErr == nil || hostErr.Code != "resource_not_found" {
		t.Fatalf("absent non-binding target = %+v, want resource_not_found", hostErr)
	}
	if !strings.Contains(hostErr.Message, "/worker") {
		t.Fatalf("the refusal does not name the relation pointer: %s", hostErr.Message)
	}
}

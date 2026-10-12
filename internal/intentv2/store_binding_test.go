package intentv2

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testStoreBinding() StoreBinding {
	return StoreBinding{HostAPI: "https://host.example/custom/api"}
}

func TestBoundStoreRequiresExactHostAndStoreScope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bound-store")
	binding := testStoreBinding()
	if err := InitializeBound(path, testStoreID, binding); err != nil {
		t.Fatalf("InitializeBound(): %v", err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gotBinding, err := store.Binding(context.Background())
	if err != nil || gotBinding != binding {
		t.Fatalf("Binding() = %#v, error %v; want %#v", gotBinding, err, binding)
	}

	request := testBeginRequest()
	request.Target.PrincipalScope = testStoreID
	wrongHost := request
	wrongHost.Target.HostAPI = "https://other.example/api"
	if _, err := store.Begin(context.Background(), wrongHost); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("Begin() with different Host root = %v, want ErrStoreIdentity", err)
	}
	wrongScope := request
	wrongScope.Target.PrincipalScope = "other-local-scope"
	if _, err := store.Begin(context.Background(), wrongScope); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("Begin() with different local scope = %v, want ErrStoreIdentity", err)
	}
	intent, err := store.Begin(context.Background(), request)
	if err != nil || intent.Target.HostAPI != binding.HostAPI || intent.Target.PrincipalScope != testStoreID {
		t.Fatalf("Begin() with bound identity = %#v, error %v", intent, err)
	}
	for _, target := range []Target{
		{HostAPI: "https://other.example/api", PrincipalScope: testStoreID, Space: request.Target.Space, Name: request.Target.Name},
		{HostAPI: binding.HostAPI, PrincipalScope: "other-local-scope", Space: request.Target.Space, Name: request.Target.Name},
		{HostAPI: "https://other.example/api", PrincipalScope: "other-local-scope", Space: request.Target.Space, Name: request.Target.Name},
	} {
		changed := request
		changed.Target = target
		if _, err := store.Begin(context.Background(), changed); !errors.Is(err, ErrStoreIdentity) {
			t.Fatalf("Begin() after unresolved claim with changed target %#v = %v, want ErrStoreIdentity", target, err)
		}
	}
}

func TestStoreBindingRejectsNonExactV2BaseURLForms(t *testing.T) {
	for _, host := range []string{
		"http://host.example/api",
		"https://host.example/api/",
		"https://host.example/api?",
		"https://host.example/api#",
		"https://user@host.example/api",
	} {
		t.Run(host, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "invalid-binding")
			if err := InitializeBound(path, testStoreID, StoreBinding{HostAPI: host}); err == nil {
				t.Fatalf("InitializeBound() accepted invalid base URL %q", host)
			}
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid initialization created the store path: %v", err)
			}
		})
	}
}

func TestUnboundStoreRequiresExplicitEmptyBindingAndCannotAdoptNonemptyStore(t *testing.T) {
	ctx := context.Background()
	binding := testStoreBinding()

	emptyPath := filepath.Join(t.TempDir(), "empty-unbound")
	if err := Initialize(emptyPath, testStoreID); err != nil {
		t.Fatal(err)
	}
	empty, err := Open(emptyPath, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	request := testBeginRequest()
	request.Target.PrincipalScope = testStoreID
	if _, err := empty.Begin(ctx, request); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("Begin() on unbound store = %v, want ErrStoreIdentity", err)
	}
	if err := empty.BindEmpty(ctx, binding); err != nil {
		t.Fatalf("BindEmpty() on empty unbound store: %v", err)
	}
	if got, err := empty.Binding(ctx); err != nil || got != binding {
		t.Fatalf("Binding() after explicit bind = %#v, error %v", got, err)
	}
	if _, err := empty.Begin(ctx, request); err != nil {
		t.Fatalf("Begin() after explicit empty bind: %v", err)
	}
	if err := empty.Close(); err != nil {
		t.Fatal(err)
	}

	legacyPath := filepath.Join(t.TempDir(), "legacy-nonempty-unbound")
	if err := InitializeBound(legacyPath, testStoreID, binding); err != nil {
		t.Fatal(err)
	}
	legacy, err := Open(legacyPath, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := legacy.Begin(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	removeBindingForLegacyFixture(t, legacyPath)
	if _, err := Open(legacyPath, testStoreID); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("Open() adopted nonempty unbound store = %v, want ErrStoreIdentity", err)
	}
	legacyState, err := os.ReadFile(filepath.Join(legacyPath, "state.json"))
	if err != nil || !json.Valid(legacyState) || !containsBytes(legacyState, []byte(claim.ID)) {
		t.Fatalf("refusing legacy store changed or lost original claim: read error %v", err)
	}
}

func TestImportedResourceReservesSlotAndSupportsExactUIDLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "imported-store")
	binding := testStoreBinding()
	if err := InitializeBound(path, testStoreID, binding); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	target := testBeginRequest().Target
	target.PrincipalScope = testStoreID
	evidence := ImportEvidence{
		HostAPI: binding.HostAPI,
		Path:    "/resources/imported-uid", StatusCode: 200, UID: "imported-uid",
		Form: "https://forms.example/Widget/1.0.0/", Space: target.Space, Name: target.Name, Generation: 7,
	}
	importRequest := ImportRequest{Target: target, UID: evidence.UID, Form: evidence.Form, Evidence: evidence}
	imported, err := store.BindImportedResource(ctx, importRequest)
	if err != nil || imported.Target != target || imported.UID != evidence.UID || imported.Generation != evidence.Generation || imported.Form != evidence.Form {
		t.Fatalf("BindImportedResource() = %#v, error %v", imported, err)
	}
	if repeated, err := store.BindImportedResource(ctx, importRequest); err != nil || repeated != imported {
		t.Fatalf("identical import bind = %#v, error %v; want %#v", repeated, err, imported)
	}
	if reread, found, err := store.ReadImportBinding(ctx, target); err != nil || !found || reread != imported {
		t.Fatalf("ReadImportBinding() = %#v, found %v, error %v", reread, found, err)
	}
	if intents, err := store.Read(ctx, target); err != nil || len(intents) != 0 {
		t.Fatalf("import created mutation history: %#v, error %v", intents, err)
	}

	create := testBeginRequest()
	create.Target = target
	if _, err := store.Begin(ctx, create); !errors.Is(err, ErrTargetOccupied) {
		t.Fatalf("Create on imported slot = %v, want ErrTargetOccupied", err)
	}
	otherFormCreate := create
	otherFormCreate.Form = "https://forms.example/Other/1.0.0/"
	otherFormCreate.PublicJSON = []byte(`{"form":"https://forms.example/Other/1.0.0/","space":"team-a","name":"widget","spec":{"size":"small"}}`)
	if _, err := store.Begin(ctx, otherFormCreate); !errors.Is(err, ErrTargetOccupied) {
		t.Fatalf("Create using another Form on imported slot = %v, want ErrTargetOccupied", err)
	}
	update := testBeginRequest()
	update.Target, update.Action, update.Form = target, ActionUpdate, evidence.Form
	update.Method, update.Path = "PUT", "/resources/"+evidence.UID
	update.UID, update.Generation = evidence.UID, "7"
	update.PublicJSON = []byte(`{"spec":{"size":"updated"}}`)
	wrongUID := update
	wrongUID.UID, wrongUID.Path = "other-uid", "/resources/other-uid"
	if _, err := store.Begin(ctx, wrongUID); !errors.Is(err, ErrTargetOccupied) && !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("Update with wrong imported UID = %v, want identity refusal", err)
	}
	wrongGeneration := update
	wrongGeneration.Generation = "6"
	if _, err := store.Begin(ctx, wrongGeneration); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("Update with wrong imported generation = %v, want ErrIntentConflict", err)
	}
	wrongForm := update
	wrongForm.Form = "https://forms.example/Other/1.0.0/"
	if _, err := store.Begin(ctx, wrongForm); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("Update with wrong imported Form = %v, want ErrIntentConflict", err)
	}
	wrongImportScope := target
	wrongImportScope.PrincipalScope = "other-local-scope"
	if _, err := store.BindImportedResource(ctx, ImportRequest{Target: wrongImportScope, UID: evidence.UID, Form: evidence.Form, Evidence: evidence}); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("import with wrong local scope = %v, want ErrStoreIdentity", err)
	}
	updateIntent, err := store.Begin(ctx, update)
	if err != nil {
		t.Fatalf("Update on exact imported identity: %v", err)
	}
	if _, err := store.BindImportedResource(ctx, importRequest); !errors.Is(err, ErrUnresolvedIntent) && !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("rebind during unresolved Update = %v, want refusal", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if reread, found, err := store.ReadImportBinding(ctx, target); err != nil || !found || reread != imported {
		t.Fatalf("reopened import binding = %#v, found %v, error %v", reread, found, err)
	}
	if replay, err := store.Begin(ctx, update); err != nil || replay.ID != updateIntent.ID || replay.Key != updateIntent.Key {
		t.Fatalf("same imported-UID Update after reopen = %#v, error %v", replay, err)
	}
}

func TestImportEvidenceMustMatchAuthorizedExactResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence-store")
	binding := testStoreBinding()
	if err := InitializeBound(path, testStoreID, binding); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := testBeginRequest().Target
	target.PrincipalScope = testStoreID
	valid := ImportEvidence{HostAPI: binding.HostAPI, Path: "/resources/uid-1", StatusCode: 200, UID: "uid-1", Form: "https://forms.example/Widget/1.0.0/", Space: target.Space, Name: target.Name, Generation: 1}
	validRequest := ImportRequest{Target: target, UID: valid.UID, Form: valid.Form, Evidence: valid}
	cases := []struct {
		name   string
		change func(*ImportEvidence)
	}{
		{"wrong path", func(e *ImportEvidence) { e.Path = "/resources/other" }},
		{"wrong Host", func(e *ImportEvidence) { e.HostAPI = "https://other.example/api" }},
		{"not found", func(e *ImportEvidence) { e.StatusCode = 404 }},
		{"wrong UID", func(e *ImportEvidence) { e.UID = "other" }},
		{"wrong Form", func(e *ImportEvidence) { e.Form = "https://forms.example/Other/1.0.0/" }},
		{"wrong Space", func(e *ImportEvidence) { e.Space = "other-space" }},
		{"wrong name", func(e *ImportEvidence) { e.Name = "other-name" }},
		{"invalid generation", func(e *ImportEvidence) { e.Generation = 0 }},
		{"generation exceeds safe integer", func(e *ImportEvidence) { e.Generation = maxReplayWindowSeconds + 1 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			evidence := valid
			test.change(&evidence)
			request := validRequest
			request.Evidence = evidence
			if _, err := store.BindImportedResource(context.Background(), request); err == nil {
				t.Fatal("BindImportedResource() accepted mismatched Resource evidence")
			}
		})
	}
	wrongUID := validRequest
	wrongUID.UID = "other"
	if _, err := store.BindImportedResource(context.Background(), wrongUID); err == nil {
		t.Fatal("BindImportedResource() accepted a caller UID that differed from the GET result")
	}
	wrongForm := validRequest
	wrongForm.Form = "https://forms.example/Other/1.0.0/"
	if _, err := store.BindImportedResource(context.Background(), wrongForm); err == nil {
		t.Fatal("BindImportedResource() accepted a caller Form that differed from the GET result")
	}
	if _, err := store.BindImportedResource(context.Background(), validRequest); err != nil {
		t.Fatalf("valid Resource evidence refused after negative cases: %v", err)
	}
}

func TestImportedSlotReleasesOnlyAfterExactSuccessfulDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "import-delete-store")
	binding := testStoreBinding()
	if err := InitializeBound(path, testStoreID, binding); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	request := testBeginRequest()
	target := request.Target
	evidence := ImportEvidence{HostAPI: binding.HostAPI, Path: "/resources/imported-uid", StatusCode: 200, UID: "imported-uid", Form: request.Form, Space: target.Space, Name: target.Name, Generation: 5}
	importRequest := ImportRequest{Target: target, UID: evidence.UID, Form: evidence.Form, Evidence: evidence}
	imported, err := store.BindImportedResource(ctx, importRequest)
	if err != nil {
		t.Fatal(err)
	}

	delete := request
	delete.Action, delete.Method, delete.Path = ActionDelete, "DELETE", "/resources/"+evidence.UID
	delete.UID, delete.Generation = evidence.UID, "5"
	delete.PublicJSON, delete.PrivateInputNames = nil, nil
	deleteIntent, err := store.Begin(ctx, delete)
	if err != nil {
		t.Fatalf("Begin() for imported UID delete: %v", err)
	}
	if _, err := store.Begin(ctx, request); !errors.Is(err, ErrUnresolvedIntent) {
		t.Fatalf("Create before confirmed imported UID delete = %v, want ErrUnresolvedIntent", err)
	}
	ack := operationAck(deleteIntent, "operation-import-delete", evidence.UID, "succeeded", "complete")
	if _, err := store.RecordAccepted(ctx, deleteIntent.ID, ack); err != nil {
		t.Fatal(err)
	}
	absent := ResourceObservation{Path: "/resources/" + evidence.UID, StatusCode: 404}
	if _, err := store.Complete(ctx, deleteIntent.ID, CompletionEvidence{Operation: ack, Resource: absent}); err != nil {
		t.Fatalf("Complete() exact successful imported UID Delete: %v", err)
	}
	if reread, found, err := store.ReadImportBinding(ctx, target); err != nil || !found || reread != imported {
		t.Fatalf("import history after deletion = %#v, found %v, error %v", reread, found, err)
	}
	newCreate, err := store.Begin(ctx, request)
	if err != nil || newCreate.Key == deleteIntent.Key {
		t.Fatalf("new incarnation after confirmed Delete = %#v, error %v", newCreate, err)
	}
	if replay, err := store.Begin(ctx, delete); err != nil || replay.ID != deleteIntent.ID || replay.Key != deleteIntent.Key {
		t.Fatalf("old imported UID Delete replay after recreate = %#v, error %v", replay, err)
	}
}

func TestFailedImportedDeleteKeepsSlotReserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failed-import-delete-store")
	binding := testStoreBinding()
	if err := InitializeBound(path, testStoreID, binding); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	create := testBeginRequest()
	target := create.Target
	evidence := ImportEvidence{HostAPI: binding.HostAPI, Path: "/resources/imported-uid", StatusCode: 200, UID: "imported-uid", Form: create.Form, Space: target.Space, Name: target.Name, Generation: 5}
	if _, err := store.BindImportedResource(ctx, ImportRequest{Target: target, UID: evidence.UID, Form: evidence.Form, Evidence: evidence}); err != nil {
		t.Fatal(err)
	}
	delete := create
	delete.Action, delete.Method, delete.Path = ActionDelete, "DELETE", "/resources/"+evidence.UID
	delete.UID, delete.Generation = evidence.UID, "5"
	delete.PublicJSON, delete.PrivateInputNames = nil, nil
	intent, err := store.Begin(ctx, delete)
	if err != nil {
		t.Fatal(err)
	}
	ack := operationAck(intent, "operation-import-delete-failed", evidence.UID, "failed", "partial")
	if _, err := store.RecordAccepted(ctx, intent.ID, ack); err != nil {
		t.Fatal(err)
	}
	resource := resourceObservation(intent, evidence.UID, ack.Generation)
	resource.Phase = "error"
	if _, err := store.Complete(ctx, intent.ID, CompletionEvidence{Operation: ack, Resource: resource}); err != nil {
		t.Fatalf("Complete() failed Delete evidence: %v", err)
	}
	if _, err := store.Begin(ctx, create); !errors.Is(err, ErrTargetOccupied) {
		t.Fatalf("Create after failed imported UID cleanup = %v, want ErrTargetOccupied", err)
	}
	if imported, found, err := store.ReadImportBinding(ctx, target); err != nil || !found || imported.UID != evidence.UID || imported.Generation != evidence.Generation {
		t.Fatalf("failed cleanup lost import identity: %#v found=%v err=%v", imported, found, err)
	}
}

func removeBindingForLegacyFixture(t *testing.T, path string) {
	t.Helper()
	statePath := filepath.Join(path, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	delete(snapshot, "binding")
	data, err = json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
}

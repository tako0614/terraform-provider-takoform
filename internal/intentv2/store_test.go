package intentv2

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

const testStoreID = "workspace-provider-v2"

func testBeginRequest() BeginRequest {
	return BeginRequest{
		Target: Target{
			HostAPI:        "https://host.example/custom/api",
			PrincipalScope: testStoreID,
			Space:          "team-a",
			Name:           "widget",
		},
		Action:                     ActionCreate,
		Form:                       "https://forms.example/Widget/1.0.0/",
		Method:                     "POST",
		Path:                       "/resources",
		PublicJSON:                 []byte(`{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","spec":{"size":"small"}}`),
		PrivateInputNames:          []string{"TOKEN", "PASSWORD"},
		AdvertisedReplayWindowSecs: 3600,
	}
}

func initTestStore(t *testing.T) (string, *Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "intent-store")
	if err := InitializeBound(path, testStoreID, testStoreBinding()); err != nil {
		t.Fatalf("InitializeBound() error = %v", err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return path, store
}

func operationAck(intent Intent, operationID, uid, status, effect string) OperationAck {
	generation := int64(1)
	if intent.Action != ActionCreate {
		generation, _ = strconv.ParseInt(intent.Generation, 10, 64)
		generation++
	}
	return OperationAck{
		ID: operationID, ResourceUID: uid, Action: intent.Action, Generation: generation,
		Status: status, Effect: effect, RetainUntil: "2026-10-13T12:00:00Z",
	}
}

func resourceObservation(intent Intent, uid string, generation int64) ResourceObservation {
	return ResourceObservation{
		Present: true, Path: "/resources/" + uid, StatusCode: 200, UID: uid, Form: intent.Form,
		Space: intent.Target.Space, Name: intent.Target.Name, Generation: generation, Phase: "idle",
	}
}

func acceptAndComplete(store *Store, intent Intent, operationID, uid string) (Intent, error) {
	ack := operationAck(intent, operationID, uid, "succeeded", "complete")
	if _, err := store.RecordAccepted(context.Background(), intent.ID, ack); err != nil {
		return Intent{}, err
	}
	evidence := CompletionEvidence{Operation: ack}
	if intent.Action == ActionDelete {
		evidence.Resource = ResourceObservation{Path: "/resources/" + uid, StatusCode: 404}
	} else {
		evidence.Resource = resourceObservation(intent, uid, ack.Generation)
	}
	return store.Complete(context.Background(), intent.ID, evidence)
}

func TestOpenNeverInitializesAndRequiresExactStoreIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created")
	if _, err := Open(path, testStoreID); err == nil {
		t.Fatal("Open() accepted an uninitialized path")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open() created store path; Lstat error = %v", err)
	}
	if err := Initialize(path, testStoreID); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := Initialize(path, testStoreID); err == nil {
		t.Fatal("Initialize() replaced an existing store")
	}
	if _, err := Open(path, "different-store"); !errors.Is(err, ErrStoreIdentity) {
		t.Fatalf("Open() wrong store identity error = %v, want ErrStoreIdentity", err)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatalf("Open() initialized store error = %v", err)
	}
	_ = store.Close()
}

func TestBeginReopenReturnsOriginalKeyTimeAndWindow(t *testing.T) {
	path, store := initTestStore(t)
	ctx := context.Background()
	request := testBeginRequest()
	first, err := store.Begin(ctx, request)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if first.Status != StatusClaimed || first.FirstSentAt.IsZero() || first.AdvertisedReplayWindowSecs != 3600 {
		t.Fatalf("Begin() intent = %#v", first)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	request.AdvertisedReplayWindowSecs = 120
	reopened, err := Open(path, testStoreID)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	second, err := reopened.Begin(ctx, request)
	if err != nil {
		t.Fatalf("replayed Begin() error = %v", err)
	}
	if second.ID != first.ID || second.Key != first.Key || !second.FirstSentAt.Equal(first.FirstSentAt) || second.AdvertisedReplayWindowSecs != first.AdvertisedReplayWindowSecs {
		t.Fatalf("replayed intent = %#v, want original key/time/window %#v", second, first)
	}
}

func TestBeginRefusesChangedPublicIntentAndNeverRemintsUnknownClaim(t *testing.T) {
	_, store := initTestStore(t)
	ctx := context.Background()
	original := testBeginRequest()
	first, err := store.Begin(ctx, original)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	changed := original
	changed.PublicJSON = []byte(`{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","spec":{"size":"large"}}`)
	if _, err := store.Begin(ctx, changed); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("Begin() changed public request error = %v, want ErrIntentConflict", err)
	}
	replayed, err := store.Begin(ctx, original)
	if err != nil {
		t.Fatalf("same Begin() error = %v", err)
	}
	if replayed.Key != first.Key || !replayed.FirstSentAt.Equal(first.FirstSentAt) {
		t.Fatalf("unresolved claim changed across Begin(): first=%#v replay=%#v", first, replayed)
	}
	records, err := store.Read(ctx, original.Target)
	if err != nil || len(records) != 1 {
		t.Fatalf("Read() records=%d error=%v, want one unresolved record", len(records), err)
	}
}

func TestBeginRejectsPrivateInputsInPublicEnvelope(t *testing.T) {
	path, store := initTestStore(t)
	request := testBeginRequest()
	request.PublicJSON = []byte(`{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","privateInputs":{"TOKEN":"sensitive-runtime-value"},"spec":{}}`)
	if _, err := store.Begin(context.Background(), request); err == nil {
		t.Fatal("Begin() accepted privateInputs in persisted public JSON")
	}
	data, err := os.ReadFile(filepath.Join(path, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || containsBytes(data, []byte("sensitive-runtime-value")) {
		t.Fatal("private input value appeared in durable store")
	}
}

func TestRecordAcceptedRequiresAndPreservesHostAcknowledgement(t *testing.T) {
	_, store := initTestStore(t)
	intent, err := store.Begin(context.Background(), testBeginRequest())
	if err != nil {
		t.Fatal(err)
	}
	ack := operationAck(intent, "operation-1", "uid-1", "succeeded", "complete")
	invalid := ack
	invalid.RetainUntil = ""
	if _, err := store.RecordAccepted(context.Background(), intent.ID, invalid); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("missing retainUntil error = %v, want ErrStoreUnavailable", err)
	}
	accepted, err := store.RecordAccepted(context.Background(), intent.ID, ack)
	if err != nil || accepted.Status != StatusAccepted || accepted.OperationID != "operation-1" || accepted.RetainUntil.IsZero() {
		t.Fatalf("RecordAccepted()=%#v error=%v", accepted, err)
	}
	replayed, err := store.RecordAccepted(context.Background(), intent.ID, ack)
	if err != nil || replayed.ID != accepted.ID {
		t.Fatalf("identical acknowledgement replay=%#v error=%v", replayed, err)
	}
	changed := ack
	changed.ID = "operation-2"
	if _, err := store.RecordAccepted(context.Background(), intent.ID, changed); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("changed acknowledgement error = %v, want ErrIntentConflict", err)
	}
}

func TestCompleteRequiresAcceptedOperationEvidence(t *testing.T) {
	_, store := initTestStore(t)
	intent, err := store.Begin(context.Background(), testBeginRequest())
	if err != nil {
		t.Fatal(err)
	}
	evidence := CompletionEvidence{Operation: operationAck(intent, "operation-1", "uid-1", "succeeded", "complete"), Resource: resourceObservation(intent, "uid-1", 1)}
	if _, err := store.Complete(context.Background(), intent.ID, evidence); err == nil {
		t.Fatal("Complete() accepted a caller-only success label without Host operation evidence")
	}
}

func TestCompleteRequiresResourcePhaseConsistentWithOperation(t *testing.T) {
	ctx := context.Background()
	t.Run("success requires idle", func(t *testing.T) {
		_, store := initTestStore(t)
		intent, err := store.Begin(ctx, testBeginRequest())
		if err != nil {
			t.Fatal(err)
		}
		ack := operationAck(intent, "operation-success", "uid-1", "succeeded", "complete")
		if _, err := store.RecordAccepted(ctx, intent.ID, ack); err != nil {
			t.Fatal(err)
		}
		observation := resourceObservation(intent, "uid-1", ack.Generation)
		observation.Phase = "error"
		if _, err := store.Complete(ctx, intent.ID, CompletionEvidence{Operation: ack, Resource: observation}); !errors.Is(err, ErrIntentConflict) {
			t.Fatalf("success with phase error = %v, want ErrIntentConflict", err)
		}
		observation.Phase = "idle"
		if completed, err := store.Complete(ctx, intent.ID, CompletionEvidence{Operation: ack, Resource: observation}); err != nil || completed.Status != StatusApplied {
			t.Fatalf("success with idle phase = %#v, error %v", completed, err)
		}
	})

	for _, action := range []Action{ActionCreate, ActionUpdate, ActionDelete} {
		t.Run("failed "+string(action)+" requires error", func(t *testing.T) {
			_, store := initTestStore(t)
			intent, err := store.Begin(ctx, testBeginRequest())
			if err != nil {
				t.Fatal(err)
			}
			uid := "uid-1"
			if action != ActionCreate {
				if _, err := acceptAndComplete(store, intent, "operation-create", uid); err != nil {
					t.Fatal(err)
				}
				request := testBeginRequest()
				request.Action, request.UID, request.Generation = action, uid, "1"
				if action == ActionUpdate {
					request.Method, request.Path = "PUT", "/resources/"+uid
					request.PublicJSON = []byte(`{"spec":{"size":"medium"}}`)
				} else {
					request.Method, request.Path = "DELETE", "/resources/"+uid
					request.PublicJSON, request.PrivateInputNames = nil, nil
				}
				started, err := store.Begin(ctx, request)
				if err != nil {
					t.Fatal(err)
				}
				intent = started
			}
			ack := operationAck(intent, "operation-failed", uid, "failed", "partial")
			if _, err := store.RecordAccepted(ctx, intent.ID, ack); err != nil {
				t.Fatal(err)
			}
			observation := resourceObservation(intent, uid, ack.Generation)
			observation.Phase = "idle"
			if _, err := store.Complete(ctx, intent.ID, CompletionEvidence{Operation: ack, Resource: observation}); !errors.Is(err, ErrIntentConflict) {
				t.Fatalf("failed operation with phase idle = %v, want ErrIntentConflict", err)
			}
			observation.Phase = "error"
			if completed, err := store.Complete(ctx, intent.ID, CompletionEvidence{Operation: ack, Resource: observation}); err != nil || completed.Status != StatusFailedReserved {
				t.Fatalf("failed operation with error phase = %#v, error %v", completed, err)
			}
		})
	}
}

func TestRecordAcceptedBindsActionUIDAndAdvancedGeneration(t *testing.T) {
	_, store := initTestStore(t)
	create, err := store.Begin(context.Background(), testBeginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptAndComplete(store, create, "operation-create", "uid-1"); err != nil {
		t.Fatal(err)
	}
	update := testBeginRequest()
	update.Action, update.Method, update.Path = ActionUpdate, "PUT", "/resources/uid-1"
	update.UID, update.Generation = "uid-1", "1"
	update.PublicJSON = []byte(`{"spec":{"size":"medium"}}`)
	updateIntent, err := store.Begin(context.Background(), update)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		ack  OperationAck
	}{
		{name: "wrong uid", ack: operationAck(updateIntent, "operation-1", "other-uid", "succeeded", "complete")},
		{name: "generation did not advance", ack: func() OperationAck {
			ack := operationAck(updateIntent, "operation-1", "uid-1", "succeeded", "complete")
			ack.Generation = 1
			return ack
		}()},
		{name: "wrong action", ack: func() OperationAck {
			ack := operationAck(updateIntent, "operation-1", "uid-1", "succeeded", "complete")
			ack.Action = ActionDelete
			return ack
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.RecordAccepted(context.Background(), updateIntent.ID, test.ack); !errors.Is(err, ErrIntentConflict) {
				t.Fatalf("RecordAccepted() error = %v, want ErrIntentConflict", err)
			}
		})
	}
}

func TestOperationRetentionMayExtendButNeverShorten(t *testing.T) {
	_, store := initTestStore(t)
	intent, err := store.Begin(context.Background(), testBeginRequest())
	if err != nil {
		t.Fatal(err)
	}
	queued := operationAck(intent, "operation-1", "uid-1", "queued", "none")
	if _, err := store.RecordAccepted(context.Background(), intent.ID, queued); err != nil {
		t.Fatalf("RecordAccepted(queued): %v", err)
	}
	succeeded := queued
	succeeded.Status, succeeded.Effect = "succeeded", "complete"
	succeeded.RetainUntil = "2026-10-14T12:00:00Z"
	completed, err := store.Complete(context.Background(), intent.ID, CompletionEvidence{
		Operation: succeeded, Resource: resourceObservation(intent, "uid-1", 1),
	})
	if err != nil || completed.Status != StatusApplied || completed.RetainUntil.UTC().Format(time.RFC3339) != succeeded.RetainUntil || completed.InitialRetainUntil.UTC().Format(time.RFC3339) != queued.RetainUntil {
		t.Fatalf("Complete(extended retainUntil)=%#v error=%v", completed, err)
	}
	shortened := succeeded
	shortened.RetainUntil = queued.RetainUntil
	if _, err := store.Complete(context.Background(), intent.ID, CompletionEvidence{
		Operation: shortened, Resource: resourceObservation(intent, "uid-1", 1),
	}); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("shortened retainUntil error=%v, want ErrIntentConflict", err)
	}
}

func TestAcceptedFailureStaysReservedAndAllowsOnlyExplicitObservedRecovery(t *testing.T) {
	_, store := initTestStore(t)
	ctx := context.Background()
	create, err := store.Begin(ctx, testBeginRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptAndComplete(store, create, "operation-create", "uid-1"); err != nil {
		t.Fatalf("complete create: %v", err)
	}
	update := testBeginRequest()
	update.Action, update.Method, update.Path = ActionUpdate, "PUT", "/resources/uid-1"
	update.UID, update.Generation = "uid-1", "1"
	update.PublicJSON = []byte(`{"spec":{"size":"medium"}}`)
	intent, err := store.Begin(ctx, update)
	if err != nil {
		t.Fatal(err)
	}
	failedAck := operationAck(intent, "operation-failed", "uid-1", "failed", "partial")
	accepted, err := store.RecordAccepted(ctx, intent.ID, failedAck)
	if err != nil || accepted.Status != StatusAcceptedFailed {
		t.Fatalf("RecordAccepted(failed)=%#v error=%v", accepted, err)
	}
	newUpdate := update
	newUpdate.Generation = "2"
	newUpdate.PublicJSON = []byte(`{"spec":{"size":"large"}}`)
	if _, err := store.Begin(ctx, newUpdate); !errors.Is(err, ErrUnresolvedIntent) {
		t.Fatalf("new mutation before failed Operation reconciliation error=%v, want ErrUnresolvedIntent", err)
	}
	failedEvidence := CompletionEvidence{Operation: failedAck, Resource: resourceObservation(intent, "uid-1", failedAck.Generation)}
	failedEvidence.Resource.Phase = "error"
	failed, err := store.Complete(ctx, intent.ID, failedEvidence)
	if err != nil || failed.Status != StatusFailedReserved || failed.OperationUID != "uid-1" || failed.ObservedGeneration != "2" {
		t.Fatalf("Complete(failed evidence)=%#v error=%v", failed, err)
	}
	replay, err := store.Complete(ctx, intent.ID, failedEvidence)
	if err != nil || replay.Status != StatusFailedReserved {
		t.Fatalf("repeat failed evidence=%#v error=%v", replay, err)
	}
	newUpdate.SupersedesIntentID = intent.ID
	retry, err := store.Begin(ctx, newUpdate)
	if err != nil || retry.Key == intent.Key || retry.SupersedesIntentID != intent.ID {
		t.Fatalf("explicit recovery Begin()=%#v error=%v", retry, err)
	}
	newCreate := testBeginRequest()
	newCreate.PublicJSON = []byte(`{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","spec":{"size":"new"}}`)
	if _, err := store.Begin(ctx, newCreate); !errors.Is(err, ErrTargetOccupied) && !errors.Is(err, ErrUnresolvedIntent) {
		t.Fatalf("accepted failed mutation freed resource name: %v", err)
	}
}

func TestBeginRejectsNonNormativeMutationMethodPathAndBody(t *testing.T) {
	_, store := initTestStore(t)
	base := testBeginRequest()
	cases := []struct {
		name   string
		change func(*BeginRequest)
	}{
		{name: "create wrong route", change: func(r *BeginRequest) { r.Path = "/resources/widget" }},
		{name: "create wrong method", change: func(r *BeginRequest) { r.Method = "PUT" }},
		{name: "create mismatched body identity", change: func(r *BeginRequest) {
			r.PublicJSON = []byte(`{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"other","spec":{"size":"small"}}`)
		}},
		{name: "delete has body", change: func(r *BeginRequest) {
			r.Action, r.Method, r.Path, r.UID, r.Generation = ActionDelete, "DELETE", "/resources/uid-1", "uid-1", "1"
			r.PublicJSON = []byte(`{}`)
			r.PrivateInputNames = nil
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.change(&request)
			if _, err := store.Begin(context.Background(), request); err == nil {
				t.Fatal("Begin() accepted a non-normative mutation request")
			}
		})
	}
}

func TestConfirmedDeleteTombstoneAllowsNewCreateKeyAndFencedMutations(t *testing.T) {
	_, store := initTestStore(t)
	ctx := context.Background()
	create := testBeginRequest()
	first, err := store.Begin(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptAndComplete(store, first, "operation-create", "uid-1"); err != nil {
		t.Fatalf("complete create: %v", err)
	}
	update := create
	update.Action, update.Method, update.Path = ActionUpdate, "PUT", "/resources/uid-1"
	update.UID, update.Generation = "uid-1", "1"
	update.PublicJSON = []byte(`{"spec":{"size":"medium"}}`)
	updateIntent, err := store.Begin(ctx, update)
	if err != nil {
		t.Fatalf("begin update: %v", err)
	}
	updateReplay, err := store.Begin(ctx, update)
	if err != nil || updateReplay.Key != updateIntent.Key {
		t.Fatalf("same fenced update replay=%#v error=%v", updateReplay, err)
	}
	wrongForm := update
	wrongForm.Form = "https://forms.example/Other/1.0.0/"
	if _, err := store.Begin(ctx, wrongForm); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("update with substituted Form error = %v, want ErrIntentConflict", err)
	}
	updateNext := update
	updateNext.Generation = "2"
	updateNext.PublicJSON = []byte(`{"spec":{"size":"large"}}`)
	if _, err := store.Begin(ctx, updateNext); !errors.Is(err, ErrUnresolvedIntent) {
		t.Fatalf("new update passed unresolved prior generation: %v", err)
	}
	if _, err := acceptAndComplete(store, updateIntent, "operation-update-1", "uid-1"); err != nil {
		t.Fatalf("complete update: %v", err)
	}
	wrongUID := updateNext
	wrongUID.UID = "uid-other"
	wrongUID.Path = "/resources/uid-other"
	if _, err := store.Begin(ctx, wrongUID); !errors.Is(err, ErrIntentConflict) {
		t.Fatalf("new update with mismatched UID error = %v, want ErrIntentConflict", err)
	}
	secondUpdate := updateNext
	secondUpdate.PublicJSON = []byte(`{"spec":{"size":"xlarge"}}`)
	secondUpdateIntent, err := store.Begin(ctx, secondUpdate)
	if err != nil || secondUpdateIntent.Key == updateIntent.Key {
		t.Fatalf("new generation update intent=%#v error=%v; want a fresh key", secondUpdateIntent, err)
	}
	if _, err := acceptAndComplete(store, secondUpdateIntent, "operation-update-2", "uid-1"); err != nil {
		t.Fatalf("complete second update: %v", err)
	}
	delete := update
	delete.Action, delete.Method, delete.Path = ActionDelete, "DELETE", "/resources/uid-1"
	delete.Generation = "3"
	delete.PublicJSON = nil
	delete.PrivateInputNames = nil
	deleteIntent, err := store.Begin(ctx, delete)
	if err != nil {
		t.Fatalf("begin delete: %v", err)
	}
	if _, err := acceptAndComplete(store, deleteIntent, "operation-delete", "uid-1"); err != nil {
		t.Fatalf("complete delete: %v", err)
	}
	newCreate, err := store.Begin(ctx, create)
	if err != nil {
		t.Fatalf("new Create after confirmed Delete: %v", err)
	}
	if newCreate.Key == first.Key || newCreate.ID == first.ID {
		t.Fatal("new Create reused the previous incarnation key")
	}
	oldDeleteReplay, err := store.Begin(ctx, delete)
	if err != nil || oldDeleteReplay.ID != deleteIntent.ID || oldDeleteReplay.Key != deleteIntent.Key {
		t.Fatalf("old UID Delete replay after recreate=%#v error=%v; want original tombstoned intent", oldDeleteReplay, err)
	}
}

func TestConcurrentBeginHasOneDurableWinner(t *testing.T) {
	path, firstStore := initTestStore(t)
	secondStore, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()

	requests := []BeginRequest{testBeginRequest(), testBeginRequest()}
	requests[1].PublicJSON = []byte(`{"form":"https://forms.example/Widget/1.0.0/","space":"team-a","name":"widget","spec":{"size":"other"}}`)
	type result struct {
		intent Intent
		err    error
	}
	results := make([]result, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index, store := range []*Store{firstStore, secondStore} {
		wait.Add(1)
		go func(index int, store *Store) {
			defer wait.Done()
			<-start
			results[index].intent, results[index].err = store.Begin(context.Background(), requests[index])
		}(index, store)
	}
	close(start)
	wait.Wait()
	success, conflict := 0, 0
	for _, result := range results {
		if result.err == nil {
			success++
		} else if errors.Is(result.err, ErrIntentConflict) {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("concurrent results = %#v, want one winner and one conflict", results)
	}
}

func TestProcessExitReleasesLockAndLeavesDurableClaim(t *testing.T) {
	path, store := initTestStore(t)
	request := testBeginRequest()
	first, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatalf("initial Begin(): %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestIntentV2CrashHelper$")
	command.Env = append(os.Environ(), "INTENTV2_CRASH_PATH="+path)
	if err := command.Run(); err == nil {
		t.Fatal("crash helper unexpectedly exited successfully")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatalf("crash helper error = %v, want exit 23", err)
	}
	reopened, err := Open(path, testStoreID)
	if err != nil {
		t.Fatalf("Open() after process exit: %v", err)
	}
	defer reopened.Close()
	intent, err := reopened.Begin(context.Background(), request)
	if err != nil || intent.Status != StatusClaimed || intent.ID != first.ID || intent.Key != first.Key || !intent.FirstSentAt.Equal(first.FirstSentAt) {
		t.Fatalf("Begin() after process exit intent=%#v error=%v", intent, err)
	}
}

func TestIntentV2CrashHelper(t *testing.T) {
	path := os.Getenv("INTENTV2_CRASH_PATH")
	if path == "" {
		return
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		os.Exit(24)
	}
	if _, err := store.Begin(context.Background(), testBeginRequest()); err != nil {
		os.Exit(25)
	}
	if err := store.dir.withLock(context.Background(), func() error { os.Exit(23); return nil }); err != nil {
		os.Exit(26)
	}
	os.Exit(23)
}

func TestOpenRejectsSymlinkedStorePathAndHardlinkedState(t *testing.T) {
	path, store := initTestStore(t)
	_ = store.Close()
	link := filepath.Join(t.TempDir(), "store-link")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Open(link, testStoreID); err == nil {
		t.Fatal("Open() accepted a symlinked store path")
	}
	if err := os.Link(filepath.Join(path, "state.json"), filepath.Join(t.TempDir(), "state-link")); err != nil {
		t.Skipf("hard link unavailable: %v", err)
	}
	if _, err := Open(path, testStoreID); err == nil {
		t.Fatal("Open() accepted a hard-linked state file")
	}
}

func TestOpenRefusesUnexpectedPartialSnapshotFile(t *testing.T) {
	path, store := initTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, ".state.tmp"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, testStoreID); err == nil {
		t.Fatal("Open() accepted unexpected partial replacement file")
	}
}

func TestOpenRefusesCorruptSnapshotWithoutRebuilding(t *testing.T) {
	path, store := initTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(path, "state.json")
	if err := os.WriteFile(statePath, []byte(`{"interrupted":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, testStoreID); err == nil {
		t.Fatal("Open() accepted corrupt snapshot")
	}
	data, err := os.ReadFile(statePath)
	if err != nil || string(data) != `{"interrupted":` {
		t.Fatalf("corrupt snapshot was modified: data=%q error=%v", data, err)
	}
}

func containsBytes(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

package intentv2

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaimSurvivesAbruptProcessExitBeforeAcknowledgement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent-store")
	if err := Initialize(path, testStoreID); err != nil {
		t.Fatal(err)
	}
	request := testBeginRequest()
	command, reader, input := startIntentProcess(t, path, "claim-pause", "")
	ready := readProcessLine(t, reader)
	if !strings.HasPrefix(ready, "READY ") {
		t.Fatalf("child did not report durable claim readiness: %q", ready)
	}
	parts := strings.Fields(ready)
	if len(parts) != 5 {
		t.Fatalf("claim readiness fields = %d, want 5", len(parts))
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill claim process: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("claim process exited successfully after kill")
	}
	_ = input.Close()

	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatalf("Open() after abrupt process exit: %v", err)
	}
	defer store.Close()
	request.AdvertisedReplayWindowSecs = 60
	intent, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatalf("same Begin() after process exit: %v", err)
	}
	if intent.Status != StatusClaimed || intent.ID != parts[1] || intent.Key != parts[2] ||
		intent.FirstSentAt.UTC().Format(time.RFC3339Nano) != parts[3] || intent.AdvertisedReplayWindowSecs != 3600 || intent.Target != request.Target ||
		intent.Form != request.Form || string(intent.PublicJSON) != string(request.PublicJSON) {
		t.Fatalf("reopened claim = %#v, does not match original process claim", intent)
	}
}

func TestAcceptedOperationSurvivesAbruptProcessExitAndCanContinue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent-store")
	if err := Initialize(path, testStoreID); err != nil {
		t.Fatal(err)
	}
	request := testBeginRequest()
	request.AdvertisedReplayWindowSecs = 3600
	command, reader, input := startIntentProcess(t, path, "accepted-pause", "")
	ready := readProcessLine(t, reader)
	parts := strings.Fields(ready)
	if len(parts) != 5 || parts[0] != "READY" {
		t.Fatalf("accepted readiness = %q, want READY plus claim identity", ready)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("kill accepted process: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("accepted process exited successfully after kill")
	}
	_ = input.Close()

	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatalf("Open() after accepted process exit: %v", err)
	}
	defer store.Close()
	request.AdvertisedReplayWindowSecs = 90
	replayed, err := store.Begin(context.Background(), request)
	if err != nil {
		t.Fatalf("same Begin() after accepted process exit: %v", err)
	}
	if replayed.Status != StatusAccepted || replayed.Key != parts[2] || replayed.ID != parts[1] ||
		replayed.FirstSentAt.UTC().Format(time.RFC3339Nano) != parts[3] || replayed.AdvertisedReplayWindowSecs != 3600 ||
		replayed.OperationID != "operation-process-1" || replayed.OperationUID != "uid-process-1" ||
		replayed.Action != ActionCreate || replayed.OperationGeneration != 1 || replayed.OperationStatus != "queued" {
		t.Fatalf("reopened accepted claim = %#v, original operation evidence was not preserved", replayed)
	}

	operation := OperationAck{
		ID: replayed.OperationID, ResourceUID: replayed.OperationUID, Action: replayed.Action,
		Generation: replayed.OperationGeneration, Status: "succeeded", Effect: "complete",
		RetainUntil: "2026-10-14T12:00:00Z",
	}
	resource := resourceObservation(replayed, operation.ResourceUID, operation.Generation)
	continued, err := store.Complete(context.Background(), replayed.ID, CompletionEvidence{Operation: operation, Resource: resource})
	if err != nil || continued.Status != StatusApplied || continued.OperationID != operation.ID {
		t.Fatalf("Complete() after reopening accepted claim = %#v, error %v", continued, err)
	}
}

func TestConcurrentProcessesCreateOneWinnerAcrossDifferentForms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent-store")
	if err := Initialize(path, testStoreID); err != nil {
		t.Fatal(err)
	}
	first, firstReader, firstInput := startIntentProcess(t, path, "race", "https://forms.example/WidgetA/1.0.0/")
	second, secondReader, secondInput := startIntentProcess(t, path, "race", "https://forms.example/WidgetB/1.0.0/")
	if got := readProcessLine(t, firstReader); got != "READY" {
		t.Fatalf("first process readiness = %q, want READY", got)
	}
	if got := readProcessLine(t, secondReader); got != "READY" {
		t.Fatalf("second process readiness = %q, want READY", got)
	}
	if _, err := io.WriteString(firstInput, "GO\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(secondInput, "GO\n"); err != nil {
		t.Fatal(err)
	}
	firstResult := readProcessLine(t, firstReader)
	secondResult := readProcessLine(t, secondReader)
	if err := first.Wait(); err != nil {
		t.Fatalf("first process: %v", err)
	}
	if err := second.Wait(); err != nil {
		t.Fatalf("second process: %v", err)
	}
	_ = firstInput.Close()
	_ = secondInput.Close()
	results := []string{firstResult, secondResult}
	winners, conflicts := 0, 0
	for _, result := range results {
		switch {
		case strings.HasPrefix(result, "WIN "):
			winners++
		case result == "CONFLICT":
			conflicts++
		default:
			t.Fatalf("unexpected process result: %q", result)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("cross-process results = %q and %q; want one winner and one conflict", firstResult, secondResult)
	}
	store, err := Open(path, testStoreID)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	intents, err := store.Read(context.Background(), testBeginRequest().Target)
	if err != nil || len(intents) != 1 || intents[0].Status != StatusClaimed {
		t.Fatalf("durable winning intent = %#v, error %v; want exactly one unresolved claim", intents, err)
	}
}

func TestIntentV2SubprocessWorker(t *testing.T) {
	path := os.Getenv("INTENTV2_PROCESS_PATH")
	if path == "" {
		return
	}
	mode := os.Getenv("INTENTV2_PROCESS_MODE")
	form := os.Getenv("INTENTV2_PROCESS_FORM")
	store, err := Open(path, testStoreID)
	if err != nil {
		fmt.Fprintln(os.Stdout, "FAIL")
		os.Exit(31)
	}
	defer store.Close()
	request := testBeginRequest()
	if mode == "race" {
		request.Form = form
		request.PublicJSON = []byte(fmt.Sprintf(`{"form":%q,"space":"team-a","name":"widget","spec":{"size":"small"}}`, form))
		fmt.Fprintln(os.Stdout, "READY")
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			os.Exit(32)
		}
		intent, err := store.Begin(context.Background(), request)
		if err == nil {
			fmt.Fprintf(os.Stdout, "WIN %s\n", intent.ID)
			return
		}
		if errors.Is(err, ErrIntentConflict) {
			fmt.Fprintln(os.Stdout, "CONFLICT")
			return
		}
		fmt.Fprintln(os.Stdout, "FAIL")
		os.Exit(33)
	}
	intent, err := store.Begin(context.Background(), request)
	if err != nil {
		fmt.Fprintln(os.Stdout, "FAIL")
		os.Exit(34)
	}
	if mode == "accepted-pause" {
		ack := OperationAck{
			ID: "operation-process-1", ResourceUID: "uid-process-1", Action: intent.Action,
			Generation: 1, Status: "queued", Effect: "none", RetainUntil: "2026-10-14T12:00:00Z",
		}
		if _, err := store.RecordAccepted(context.Background(), intent.ID, ack); err != nil {
			fmt.Fprintln(os.Stdout, "FAIL")
			os.Exit(35)
		}
	} else if mode != "claim-pause" {
		fmt.Fprintln(os.Stdout, "FAIL")
		os.Exit(36)
	}
	fmt.Fprintf(os.Stdout, "READY %s %s %s %d\n", intent.ID, intent.Key,
		intent.FirstSentAt.UTC().Format(time.RFC3339Nano), intent.AdvertisedReplayWindowSecs)
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func startIntentProcess(t *testing.T, path, mode, form string) (*exec.Cmd, *bufio.Reader, io.WriteCloser) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestIntentV2SubprocessWorker$")
	command.Env = append(os.Environ(),
		"INTENTV2_PROCESS_PATH="+path,
		"INTENTV2_PROCESS_MODE="+mode,
		"INTENTV2_PROCESS_FORM="+form,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	return command, bufio.NewReader(stdout), stdin
}

func readProcessLine(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	_ = reader
	result := make(chan struct {
		line string
		err  error
	}, 1)
	go func() {
		line, err := reader.ReadString('\n')
		result <- struct {
			line string
			err  error
		}{line: strings.TrimSpace(line), err: err}
	}()
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("read child handshake: %v", got.err)
		}
		return got.line
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for child handshake")
		return ""
	}
}

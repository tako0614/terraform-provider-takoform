package intentv2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testDescriptorEnvironment = "TAKOSUMI_PRIVATE_RUN_FILES_FILE"

func replaceDescriptorBytes(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".private-run-files-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func replaceDescriptor(path string, sequence uint64, files []remoteFileGrant) error {
	data, err := json.Marshal(struct {
		Version  int               `json:"version"`
		Sequence uint64            `json:"sequence"`
		Files    []remoteFileGrant `json:"files"`
	}{1, sequence, files})
	if err != nil {
		return err
	}
	return replaceDescriptorBytes(path, data)
}

func newDescriptorFixture(t *testing.T) (*remoteFixture, string, string) {
	t.Helper()
	f, server := newRemoteFixture(t)
	// The file path is the only delivery input in this mode. The fixture above
	// installs static delivery for legacy tests, so remove and restore it.
	static, hadStatic := os.LookupEnv(PrivateRunFilesEnvironment)
	if err := os.Unsetenv(PrivateRunFilesEnvironment); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadStatic {
			_ = os.Setenv(PrivateRunFilesEnvironment, static)
		} else {
			_ = os.Unsetenv(PrivateRunFilesEnvironment)
		}
	})
	path := filepath.Join(t.TempDir(), "private-run-files.json")
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := replaceDescriptor(path, 0, []remoteFileGrant{{Slot: "intent-one", Endpoint: server.URL + privateRunFilesPath, Grant: f.grant}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv(testDescriptorEnvironment, path)
	return f, path, server.URL + privateRunFilesPath
}

func TestRemoteDescriptorRotatesAcrossDeniedReadAndLostCommitAck(t *testing.T) {
	f, path, endpoint := newDescriptorFixture(t)
	store := openRemoteFixture(t)
	const secondGrant = "renewed-grant-b"
	const thirdGrant = "renewed-grant-c"
	f.mu.Lock()
	f.grant = secondGrant // Simulate Core rejecting the expired first grant.
	f.mu.Unlock()
	if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("expired grant did not fail closed: %v", err)
	}
	if err := replaceDescriptor(path, 1, []remoteFileGrant{{Slot: "intent-one", Endpoint: endpoint, Grant: secondGrant}}); err != nil {
		t.Fatal(err)
	}
	if records, err := store.Read(context.Background(), testBeginRequest().Target); err != nil || len(records) != 0 {
		t.Fatalf("renewed read failed: error %v, records %d", err, len(records))
	}
	var rotationErr error
	f.mu.Lock()
	f.ackLoss = true
	f.onLostAck = func() {
		rotationErr = replaceDescriptor(path, 2, []remoteFileGrant{{Slot: "intent-one", Endpoint: endpoint, Grant: thirdGrant}})
		if rotationErr == nil {
			f.grant = thirdGrant
		}
	}
	f.mu.Unlock()
	intent, err := store.Begin(context.Background(), testBeginRequest())
	f.mu.Lock()
	gotRotationErr := rotationErr
	f.mu.Unlock()
	if gotRotationErr != nil {
		t.Fatalf("atomic replacement failed: %v", gotRotationErr)
	}
	if err != nil || intent.Key == "" {
		t.Fatalf("lost ACK did not resolve after grant renewal: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.resolveCount != 1 || f.resolveGrant != "Bearer "+thirdGrant {
		t.Fatal("exact receipt resolve did not use the renewed grant")
	}
	if bytes.Contains(f.data, []byte(secondGrant)) || bytes.Contains(f.data, []byte(thirdGrant)) {
		t.Fatal("remote snapshot persisted a rotating bearer grant")
	}
}

func TestRemoteDescriptorRejectsRollbackIdentityTamperAndInsecureFile(t *testing.T) {
	f, path, endpoint := newDescriptorFixture(t)
	store := openRemoteFixture(t)
	original := []remoteFileGrant{{Slot: "intent-one", Endpoint: endpoint, Grant: f.grant}}
	if err := replaceDescriptor(path, 1, original); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), testBeginRequest().Target); err != nil {
		t.Fatalf("higher sequence rejected: %v", err)
	}
	validNext := descriptorJSON(t, 2, original)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"same sequence changed grant", descriptorJSON(t, 1, []remoteFileGrant{{Slot: "intent-one", Endpoint: endpoint, Grant: "forged-grant"}})},
		{"decreasing sequence", descriptorJSON(t, 0, original)},
		{"changed endpoint", descriptorJSON(t, 2, []remoteFileGrant{{Slot: "intent-one", Endpoint: "http://127.0.0.1:49151" + privateRunFilesPath, Grant: f.grant}})},
		{"changed slot set", descriptorJSON(t, 2, append(append([]remoteFileGrant{}, original...), remoteFileGrant{Slot: "other", Endpoint: endpoint, Grant: f.grant}))},
		{"missing slot", descriptorJSON(t, 2, []remoteFileGrant{{Slot: "other", Endpoint: endpoint, Grant: f.grant}})},
		{"malformed JSON", []byte(`{"version":1,`)},
		{"missing sequence", bytes.Replace(validNext, []byte(`"sequence":2,`), nil, 1)},
		{"unknown field", bytes.Replace(validNext, []byte(`"files":`), []byte(`"extra":true,"files":`), 1)},
		{"negative sequence", bytes.Replace(validNext, []byte(`"sequence":2`), []byte(`"sequence":-1`), 1)},
		{"fractional sequence", bytes.Replace(validNext, []byte(`"sequence":2`), []byte(`"sequence":2.5`), 1)},
		{"duplicate sequence", bytes.Replace(validNext, []byte(`"sequence":2,`), []byte(`"sequence":2,"sequence":2,`), 1)},
		{"oversized file", []byte(strings.Repeat("x", (4<<20)+1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := replaceDescriptorBytes(path, tc.data); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) || strings.Contains(err.Error(), path) || strings.Contains(err.Error(), f.grant) {
				t.Fatalf("invalid descriptor accepted: %v", err)
			}
		})
	}
	if err := replaceDescriptor(path, 1, original); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), testBeginRequest().Target); err != nil {
		t.Fatalf("exact sequence replay was not accepted: %v", err)
	}
	other := filepath.Join(filepath.Dir(path), "other.json")
	if err := os.WriteFile(other, descriptorJSON(t, 1, original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("symlinked descriptor accepted: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(other, path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("multiply linked descriptor accepted: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := replaceDescriptor(path, 1, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("public descriptor accepted: %v", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("public descriptor directory accepted: %v", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(path, 65534, 65534); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Read(context.Background(), testBeginRequest().Target); !errors.Is(err, ErrStoreUnavailable) {
			t.Fatalf("foreign-owned descriptor accepted: %v", err)
		}
	}
}

func TestRemoteDescriptorRefusesBothInputsAndBadFileWithoutFallback(t *testing.T) {
	f, path, endpoint := newDescriptorFixture(t)
	t.Setenv(testDescriptorEnvironment, "relative/private-run-files.json")
	if _, err := OpenRemote(context.Background(), "intent-one", testStoreID, testStoreBinding()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("relative descriptor path accepted: %v", err)
	}
	linkParent := filepath.Join(t.TempDir(), "linked-directory")
	if err := os.Symlink(filepath.Dir(path), linkParent); err != nil {
		t.Fatal(err)
	}
	t.Setenv(testDescriptorEnvironment, filepath.Join(linkParent, filepath.Base(path)))
	if _, err := OpenRemote(context.Background(), "intent-one", testStoreID, testStoreBinding()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("symlinked descriptor parent accepted: %v", err)
	}
	t.Setenv(testDescriptorEnvironment, path)
	static, err := json.Marshal([]remoteFileGrant{{Slot: "intent-one", Endpoint: endpoint, Grant: f.grant}})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(PrivateRunFilesEnvironment, string(static))
	if _, err := OpenRemote(context.Background(), "intent-one", testStoreID, testStoreBinding()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("both delivery inputs accepted: %v", err)
	}
	if err := os.Unsetenv(PrivateRunFilesEnvironment); err != nil {
		t.Fatal(err)
	}
	if err := replaceDescriptorBytes(path, []byte(`{"version":1,"sequence":0,"files":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRemote(context.Background(), "intent-one", testStoreID, testStoreBinding()); !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("bad file was bypassed: %v", err)
	}
}

func descriptorJSON(t *testing.T, sequence uint64, files []remoteFileGrant) []byte {
	t.Helper()
	data, err := json.Marshal(struct {
		Version  int               `json:"version"`
		Sequence uint64            `json:"sequence"`
		Files    []remoteFileGrant `json:"files"`
	}{1, sequence, files})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

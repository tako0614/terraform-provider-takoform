package intentv2

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

// Remote snapshots are private Runner custody, not a Host API extension. The
// grant is only kept in this process and is never included in the snapshot.
const (
	PrivateRunFilesEnvironment     = "TAKOSUMI_PRIVATE_RUN_FILES"
	PrivateRunFilesFileEnvironment = "TAKOSUMI_PRIVATE_RUN_FILES_FILE"
)

const (
	privateRunFilesPath = "/internal/private-run-files/v1"
	maxRemoteWireBytes  = (maxSnapshotBytes*4)/3 + 8192
	maxRemoteVersion    = uint64(9007199254740991)
	maxDescriptorBytes  = 4 << 20
)

var (
	ErrStoreConflict    = errors.New("intentv2: remote snapshot version conflict")
	ErrStoreUncertain   = errors.New("intentv2: remote snapshot commit outcome uncertain")
	remoteSlotPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	remoteDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type remoteFileGrant struct {
	Slot     string `json:"slot"`
	Endpoint string `json:"endpoint"`
	Grant    string `json:"grant"`
}

type remoteSnapshotBackend struct {
	endpoint         string
	grant            string // Only static, explicitly nonrenewed delivery.
	descriptorPath   string
	slot             string
	endpoints        map[string]string
	sequence         uint64
	descriptorDigest [sha256.Size]byte
	client           *http.Client
	binding          StoreBinding
}

type remoteDescriptor struct {
	sequence  uint64
	digest    [sha256.Size]byte
	endpoints map[string]string
	grant     string
}

type remoteReceipt struct {
	Version         uint64 `json:"version"`
	Digest          string `json:"digest"`
	CommitID        string `json:"commitId"`
	ExpectedVersion uint64 `json:"expectedVersion"`
}

type remoteResponse struct {
	Kind            string         `json:"kind"`
	Version         uint64         `json:"version,omitempty"`
	Digest          string         `json:"digest,omitempty"`
	BytesBase64     string         `json:"bytesBase64,omitempty"`
	Receipt         *remoteReceipt `json:"receipt,omitempty"`
	CommitID        string         `json:"commitId,omitempty"`
	ExpectedVersion uint64         `json:"expectedVersion,omitempty"`
}

// OpenRemote selects an explicitly declared opaque slot. The environment is
// generic Runner delivery; neither the Provider nor the Host selects storage.
// An absent slot may be initialized only with this exact Host binding.
func OpenRemote(ctx context.Context, slot, storeID string, binding StoreBinding) (*Store, error) {
	if !remoteSlotPattern.MatchString(slot) || validateStoreID(storeID) != nil || validateStoreBinding(binding) != nil {
		return nil, ErrStoreUnavailable
	}
	backend, err := remoteBackendForSlot(slot)
	if err != nil {
		return nil, err
	}
	store := &Store{remote: backend, storeID: storeID}
	backend.binding = binding
	data, version, present, err := backend.read(ctx)
	if err != nil {
		return nil, err
	}
	if !present {
		data, err = jsonv2.Marshal(diskSnapshot{Format: storeFormat, StoreID: storeID, Binding: &binding, Imports: []ImportBinding{}, Records: []Intent{}})
		if err != nil {
			return nil, ErrStoreUnavailable
		}
		if err = backend.commit(ctx, version, data); errors.Is(err, ErrStoreConflict) {
			// Another initializer won. Adopt only a valid, exactly bound snapshot.
			data, _, present, err = backend.read(ctx)
			if err == nil && !present {
				err = ErrStoreUnavailable
			}
		}
		if err != nil {
			return nil, err
		}
	}
	snapshot, err := decodeSnapshot(data, storeID)
	if err != nil || snapshot.Binding == nil || *snapshot.Binding != binding {
		return nil, ErrStoreIdentity
	}
	return store, nil
}

func remoteBackendForSlot(slot string) (*remoteSnapshotBackend, error) {
	raw, staticSet := os.LookupEnv(PrivateRunFilesEnvironment)
	path, fileSet := os.LookupEnv(PrivateRunFilesFileEnvironment)
	if staticSet == fileSet {
		return nil, ErrStoreUnavailable
	}
	backend := &remoteSnapshotBackend{}
	if fileSet {
		if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, ErrStoreUnavailable
		}
		descriptor, err := readRemoteDescriptor(path, slot)
		if err != nil {
			return nil, err
		}
		backend.endpoint = descriptor.endpoints[slot]
		backend.descriptorPath = path
		backend.slot = slot
		backend.endpoints = descriptor.endpoints
		backend.sequence = descriptor.sequence
		backend.descriptorDigest = descriptor.digest
	} else {
		if raw == "" || len(raw) > 1<<20 {
			return nil, ErrStoreUnavailable
		}
		var grants []remoteFileGrant
		if err := decodeStrict([]byte(raw), &grants); err != nil {
			return nil, ErrStoreUnavailable
		}
		selected, _, err := selectRemoteGrant(grants, slot)
		if err != nil {
			return nil, err
		}
		backend.endpoint = selected.Endpoint
		backend.grant = selected.Grant
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	backend.client = &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return backend, nil
}

func selectRemoteGrant(grants []remoteFileGrant, slot string) (remoteFileGrant, map[string]string, error) {
	if len(grants) == 0 || len(grants) > 256 {
		return remoteFileGrant{}, nil, ErrStoreUnavailable
	}
	endpoints := make(map[string]string, len(grants))
	var selected remoteFileGrant
	for _, entry := range grants {
		if !remoteSlotPattern.MatchString(entry.Slot) || endpoints[entry.Slot] != "" ||
			!validPrivateRunFilesEndpoint(entry.Endpoint) || !validRemoteGrant(entry.Grant) {
			return remoteFileGrant{}, nil, ErrStoreUnavailable
		}
		endpoints[entry.Slot] = entry.Endpoint
		if entry.Slot == slot {
			selected = entry
		}
	}
	if selected.Slot == "" {
		return remoteFileGrant{}, nil, ErrStoreUnavailable
	}
	return selected, endpoints, nil
}

func readRemoteDescriptor(path, slot string) (remoteDescriptor, error) {
	raw, err := readPrivateDescriptor(path, maxDescriptorBytes)
	if err != nil {
		return remoteDescriptor{}, ErrStoreUnavailable
	}
	defer clearDescriptorBytes(raw)
	var envelope struct {
		Version  *uint64            `json:"version"`
		Sequence *uint64            `json:"sequence"`
		Files    *[]remoteFileGrant `json:"files"`
	}
	if decodeStrict(raw, &envelope) != nil || envelope.Version == nil || *envelope.Version != 1 ||
		envelope.Sequence == nil || *envelope.Sequence > maxRemoteVersion || envelope.Files == nil {
		return remoteDescriptor{}, ErrStoreUnavailable
	}
	selected, endpoints, err := selectRemoteGrant(*envelope.Files, slot)
	if err != nil {
		return remoteDescriptor{}, err
	}
	return remoteDescriptor{sequence: *envelope.Sequence, digest: sha256.Sum256(raw), endpoints: endpoints, grant: selected.Grant}, nil
}

func (r *remoteSnapshotBackend) currentGrant() (string, error) {
	if r.descriptorPath == "" {
		return r.grant, nil
	}
	descriptor, err := readRemoteDescriptor(r.descriptorPath, r.slot)
	if err != nil || len(descriptor.endpoints) != len(r.endpoints) {
		return "", ErrStoreUnavailable
	}
	for slot, endpoint := range r.endpoints {
		if descriptor.endpoints[slot] != endpoint {
			return "", ErrStoreUnavailable
		}
	}
	if descriptor.sequence < r.sequence || descriptor.sequence == r.sequence && descriptor.digest != r.descriptorDigest {
		return "", ErrStoreUnavailable
	}
	if descriptor.sequence > r.sequence {
		r.sequence = descriptor.sequence
		r.descriptorDigest = descriptor.digest
	}
	return descriptor.grant, nil
}

func validPrivateRunFilesEndpoint(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" || u.Path != privateRunFilesPath ||
		u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") ||
		strings.HasSuffix(u.Host, ".") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func validRemoteGrant(grant string) bool {
	if grant == "" || len(grant) > 8192 {
		return false
	}
	for _, c := range grant {
		if c < '!' || c > '~' {
			return false
		}
	}
	return true
}

func (r *remoteSnapshotBackend) withSnapshot(ctx context.Context, storeID string, write bool, fn func(*diskSnapshot) error) error {
	data, version, present, err := r.read(ctx)
	if err != nil {
		return err
	}
	if !present {
		return ErrStoreUnavailable
	}
	snapshot, err := decodeSnapshot(data, storeID)
	if err != nil || snapshot.Binding == nil || *snapshot.Binding != r.binding {
		return ErrStoreIdentity
	}
	if err := fn(&snapshot); err != nil {
		return err
	}
	if !write {
		return nil
	}
	encoded, err := jsonv2.Marshal(snapshot)
	if err != nil || len(encoded) > maxSnapshotBytes {
		return ErrStoreUnavailable
	}
	return r.commit(ctx, version, encoded)
}

func (r *remoteSnapshotBackend) read(ctx context.Context) ([]byte, uint64, bool, error) {
	response, status, err := r.post(ctx, "read", struct{}{})
	if err != nil || status != http.StatusOK {
		return nil, 0, false, ErrStoreUnavailable
	}
	switch response.Kind {
	case "absent":
		if response.Version != 0 || response.Digest != "" || response.BytesBase64 != "" || response.Receipt != nil {
			return nil, 0, false, ErrStoreUnavailable
		}
		return nil, 0, false, nil
	case "present":
		if response.Version == 0 || response.Version > maxRemoteVersion || !remoteDigestPattern.MatchString(response.Digest) || response.Receipt != nil {
			return nil, 0, false, ErrStoreUnavailable
		}
		data, err := base64.StdEncoding.Strict().DecodeString(response.BytesBase64)
		if err != nil || len(data) == 0 || len(data) > maxSnapshotBytes || snapshotDigest(data) != response.Digest {
			return nil, 0, false, ErrStoreUnavailable
		}
		return data, response.Version, true, nil
	default:
		return nil, 0, false, ErrStoreUnavailable
	}
}

func (r *remoteSnapshotBackend) commit(ctx context.Context, expectedVersion uint64, data []byte) error {
	if expectedVersion >= maxRemoteVersion || len(data) == 0 || len(data) > maxSnapshotBytes {
		return ErrStoreUnavailable
	}
	id, err := newKey()
	if err != nil {
		return ErrStoreUnavailable
	}
	digest := snapshotDigest(data)
	request := struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		CommitID        string `json:"commitId"`
		BytesBase64     string `json:"bytesBase64"`
	}{expectedVersion, id, base64.StdEncoding.EncodeToString(data)}
	response, status, err := r.post(ctx, "commit", request)
	if err != nil || status >= 500 || status >= 300 && status < 400 {
		return r.resolve(ctx, expectedVersion, id, digest)
	}
	if status != http.StatusOK {
		return ErrStoreUnavailable
	}
	switch response.Kind {
	case "committed", "replayed":
		if validRemoteReceipt(response.Receipt, expectedVersion, id, digest) {
			return nil
		}
		return ErrStoreUncertain
	case "conflict":
		if response.CommitID != id || response.ExpectedVersion != expectedVersion || response.Digest != digest {
			return ErrStoreUncertain
		}
		return ErrStoreConflict
	case "authority_rejected":
		return ErrStoreUnavailable
	case "unknown":
		return r.resolve(ctx, expectedVersion, id, digest)
	default:
		return r.resolve(ctx, expectedVersion, id, digest)
	}
}

func (r *remoteSnapshotBackend) resolve(ctx context.Context, expectedVersion uint64, id, digest string) error {
	request := struct {
		ExpectedVersion uint64 `json:"expectedVersion"`
		CommitID        string `json:"commitId"`
		Digest          string `json:"digest"`
	}{expectedVersion, id, digest}
	response, status, err := r.post(ctx, "resolve", request)
	if err != nil || status != http.StatusOK {
		return ErrStoreUncertain
	}
	if response.Kind == "committed" && validRemoteReceipt(response.Receipt, expectedVersion, id, digest) {
		return nil
	}
	return ErrStoreUncertain
}

func validRemoteReceipt(receipt *remoteReceipt, expectedVersion uint64, id, digest string) bool {
	return receipt != nil && receipt.ExpectedVersion == expectedVersion && receipt.Version == expectedVersion+1 &&
		receipt.Version <= maxRemoteVersion && receipt.CommitID == id && receipt.Digest == digest
}

func snapshotDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (r *remoteSnapshotBackend) post(ctx context.Context, operation string, body any) (remoteResponse, int, error) {
	var zero remoteResponse
	grant, err := r.currentGrant()
	if err != nil {
		return zero, 0, ErrStoreUnavailable
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return zero, 0, ErrStoreUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/"+operation, bytes.NewReader(encoded))
	if err != nil {
		return zero, 0, ErrStoreUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+grant)
	request.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return zero, 0, ErrStoreUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteWireBytes+1))
	if err != nil || len(data) > maxRemoteWireBytes {
		return zero, response.StatusCode, ErrStoreUnavailable
	}
	if err := decodeStrict(data, &zero); err != nil {
		return remoteResponse{}, response.StatusCode, ErrStoreUnavailable
	}
	return zero, response.StatusCode, nil
}

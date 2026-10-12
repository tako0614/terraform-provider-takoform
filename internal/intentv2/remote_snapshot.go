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
	"regexp"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

// Remote snapshots are private Runner custody, not a Host API extension. The
// grant is only kept in this process and is never included in the snapshot.
const PrivateRunFilesEnvironment = "TAKOSUMI_PRIVATE_RUN_FILES"

const (
	privateRunFilesPath = "/internal/private-run-files/v1"
	maxRemoteWireBytes  = (maxSnapshotBytes*4)/3 + 8192
	maxRemoteVersion    = uint64(9007199254740991)
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
	endpoint string
	grant    string
	client   *http.Client
	binding  StoreBinding
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
	raw := os.Getenv(PrivateRunFilesEnvironment)
	if raw == "" || len(raw) > 1<<20 {
		return nil, ErrStoreUnavailable
	}
	var grants []remoteFileGrant
	if err := decodeStrict([]byte(raw), &grants); err != nil || len(grants) == 0 || len(grants) > 256 {
		return nil, ErrStoreUnavailable
	}
	seen := make(map[string]bool, len(grants))
	var chosen *remoteFileGrant
	for i := range grants {
		entry := &grants[i]
		if !remoteSlotPattern.MatchString(entry.Slot) || seen[entry.Slot] || !validPrivateRunFilesEndpoint(entry.Endpoint) || !validRemoteGrant(entry.Grant) {
			return nil, ErrStoreUnavailable
		}
		seen[entry.Slot] = true
		if entry.Slot == slot {
			chosen = entry
		}
	}
	if chosen == nil {
		return nil, ErrStoreUnavailable
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableCompression = true
	return &remoteSnapshotBackend{
		endpoint: chosen.Endpoint,
		grant:    chosen.Grant,
		client: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
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
	encoded, err := json.Marshal(body)
	if err != nil {
		return zero, 0, ErrStoreUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint+"/"+operation, bytes.NewReader(encoded))
	if err != nil {
		return zero, 0, ErrStoreUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+r.grant)
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

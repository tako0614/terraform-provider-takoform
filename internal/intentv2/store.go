// Package intentv2 stores durable, caller-owned claims for Host v2 mutations.
// It does not send requests, decide whether replay is safe, or authorize retry.
package intentv2

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

const (
	storeFormat             = "takoform.provider-hostv2-intents@v1"
	maxSnapshotBytes        = 16 << 20
	maxRecords              = 4096
	maxPublicJSONBytes      = 1 << 20
	maxPrivateInputNames    = 256
	maxReplayWindowSeconds  = int64(9007199254740991)
	maxPrincipalScopeBytes  = 256
	maxTargetComponentBytes = 1024
	idempotencyKeyBytes     = 32
)

var (
	ErrStoreUnavailable = errors.New("intentv2: store unavailable or invalid")
	ErrStoreIdentity    = errors.New("intentv2: store identity does not match")
	ErrStoreLocked      = errors.New("intentv2: store lock timed out")
	ErrIntentConflict   = errors.New("intentv2: target already has a different intent")
	ErrUnresolvedIntent = errors.New("intentv2: target has an unresolved intent")
	ErrTargetOccupied   = errors.New("intentv2: target has not been confirmed deleted")
)

type Action string

const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

type Status string

const (
	StatusClaimed         Status = "claimed"
	StatusAccepted        Status = "accepted"
	StatusAcceptedFailed  Status = "accepted-failed"
	StatusFailedReserved  Status = "failed-reserved"
	StatusApplied         Status = "applied"
	StatusDeleteConfirmed Status = "delete-confirmed"
)

var hostIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Target is a stable, non-secret identity supplied by the caller. Name is
// unique within HostAPI + PrincipalScope + Space, independent of Form.
type Target struct {
	HostAPI        string `json:"hostApi"`
	PrincipalScope string `json:"principalScope"`
	Space          string `json:"space"`
	Name           string `json:"name"`
}

// StoreBinding binds one local journal to an exact Host API root. The local
// scope label is the immutable storeID; it is not proof of authenticated
// principal continuity.
type StoreBinding struct {
	HostAPI string `json:"hostApi"`
}

// ImportEvidence is a caller-validated successful exact-UID Resource read.
// HostAPI identifies the origin from which the caller obtained the evidence;
// it does not independently prove authentication or authorization.
type ImportEvidence struct {
	HostAPI    string
	Path       string
	StatusCode int
	UID        string
	Form       string
	Space      string
	Name       string
	Generation int64
}

// ImportRequest couples the caller's exact type/UID selection to the Resource
// fetched by its authorized Host client.
type ImportRequest struct {
	Target   Target
	UID      string
	Form     string
	Evidence ImportEvidence
}

// ImportBinding records the exact current Resource identity explicitly
// imported by the caller. It is not a synthesized mutation or Operation.
type ImportBinding struct {
	Target     Target `json:"target"`
	UID        string `json:"uid"`
	Form       string `json:"form"`
	Generation int64  `json:"generation"`
}

// BeginRequest contains only the original public request. Authentication and
// private-input values are intentionally not accepted by this API.
type BeginRequest struct {
	Target                     Target
	Action                     Action
	Form                       string
	Method                     string
	Path                       string
	UID                        string
	Generation                 string
	SupersedesIntentID         string
	PublicJSON                 []byte
	PrivateInputNames          []string
	AdvertisedReplayWindowSecs int64
}

// Intent is the durable, original mutation claim. PublicJSON is copied on
// return. Its key and first-send timestamp are never regenerated on replay.
type Intent struct {
	ID                         string    `json:"id"`
	Key                        string    `json:"key"`
	Target                     Target    `json:"target"`
	Action                     Action    `json:"action"`
	Form                       string    `json:"form"`
	Method                     string    `json:"method"`
	Path                       string    `json:"path"`
	UID                        string    `json:"uid,omitempty"`
	Generation                 string    `json:"generation,omitempty"`
	SupersedesIntentID         string    `json:"supersedesIntentId,omitempty"`
	PublicJSON                 []byte    `json:"publicJson"`
	PrivateInputNames          []string  `json:"privateInputNames,omitempty"`
	FirstSentAt                time.Time `json:"firstSentAt"`
	AdvertisedReplayWindowSecs int64     `json:"advertisedReplayWindowSeconds"`
	Status                     Status    `json:"status"`
	OperationID                string    `json:"operationId,omitempty"`
	OperationUID               string    `json:"operationUid,omitempty"`
	OperationGeneration        int64     `json:"operationGeneration,omitempty"`
	OperationStatus            string    `json:"operationStatus,omitempty"`
	OperationEffect            string    `json:"operationEffect,omitempty"`
	InitialRetainUntil         time.Time `json:"initialRetainUntil,omitempty"`
	RetainUntil                time.Time `json:"retainUntil"`
	ResultUID                  string    `json:"resultUid,omitempty"`
	ResultGeneration           string    `json:"resultGeneration,omitempty"`
	ObservedGeneration         string    `json:"observedGeneration,omitempty"`
}

// OperationAck is the exact value-free Host Operation evidence returned or
// read for a mutation. Raw error messages and private values are intentionally
// excluded.
type OperationAck struct {
	ID          string
	ResourceUID string
	Action      Action
	Generation  int64
	Status      string
	Effect      string
	RetainUntil string
}

// ResourceObservation is a validated exact-UID Host read. Present observations
// bind the current Form/Space/name/generation. An absent observation binds the
// exact UID route and 404; it is useful only alongside a succeeded Delete.
type ResourceObservation struct {
	Present    bool
	Path       string
	StatusCode int
	UID        string
	Form       string
	Space      string
	Name       string
	Generation int64
	Phase      string
}

// CompletionEvidence combines the stored Operation with the current exact
// Resource read needed to settle successful or known partial effects.
type CompletionEvidence struct {
	Operation OperationAck
	Resource  ResourceObservation
}

type diskHeader struct {
	Format  string `json:"format"`
	StoreID string `json:"storeId"`
	Path    string `json:"path"`
}

type diskSnapshot struct {
	Format  string          `json:"format"`
	StoreID string          `json:"storeId"`
	Binding *StoreBinding   `json:"binding,omitempty"`
	Imports []ImportBinding `json:"imports,omitempty"`
	Records []Intent        `json:"records"`
}

// Store must be opened explicitly after Initialize. It is safe for concurrent
// goroutines and serializes mutations across processes using the OS lock.
type Store struct {
	mu      sync.Mutex
	dir     *secureDirectory
	path    string
	storeID string
	closed  bool
}

// Initialize explicitly creates a new private store directory. It refuses any
// existing path and never replaces a prior store.
func Initialize(path, storeID string) error {
	return initialize(path, storeID, nil)
}

// InitializeBound explicitly creates a store bound to one Host API root.
// Existing paths are never replaced or adopted.
func InitializeBound(path, storeID string, binding StoreBinding) error {
	if err := validateStoreBinding(binding); err != nil {
		return err
	}
	return initialize(path, storeID, &binding)
}

func initialize(path, storeID string, binding *StoreBinding) error {
	if err := validateStoreID(storeID); err != nil {
		return err
	}
	dir, err := createSecureDirectory(path)
	if err != nil {
		return ErrStoreUnavailable
	}
	defer dir.close()
	header, err := jsonv2.Marshal(diskHeader{Format: storeFormat, StoreID: storeID, Path: path})
	if err != nil {
		return ErrStoreUnavailable
	}
	snapshot, err := jsonv2.Marshal(diskSnapshot{Format: storeFormat, StoreID: storeID, Binding: binding, Imports: []ImportBinding{}, Records: []Intent{}})
	if err != nil {
		return ErrStoreUnavailable
	}
	if err := dir.initializeFiles(header, snapshot); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

// Open verifies an existing initialized store. It does not create directories,
// files, locks, or replacement state.
func Open(path, storeID string) (*Store, error) {
	if err := validateStoreID(storeID); err != nil {
		return nil, err
	}
	dir, err := openSecureDirectory(path)
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	s := &Store{dir: dir, path: path, storeID: storeID}
	if err := s.withSnapshot(context.Background(), false, func(snapshot *diskSnapshot) error {
		if snapshot.Binding == nil && (len(snapshot.Records) != 0 || len(snapshot.Imports) != 0) {
			return ErrStoreIdentity
		}
		return nil
	}); err != nil {
		_ = dir.close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.dir.close()
}

// Binding reads the persisted Host binding under the store lock. The returned
// value is descriptive only; Begin and import operations revalidate it under
// their own write lock.
func (s *Store) Binding(ctx context.Context) (StoreBinding, error) {
	var result StoreBinding
	err := s.withSnapshot(ctx, false, func(snapshot *diskSnapshot) error {
		if snapshot.Binding == nil {
			return ErrStoreIdentity
		}
		result = *snapshot.Binding
		return nil
	})
	return result, err
}

// BindEmpty explicitly binds an existing unbound, empty store. Repeating the
// exact binding is idempotent. It cannot adopt nonempty legacy custody.
func (s *Store) BindEmpty(ctx context.Context, binding StoreBinding) error {
	if err := validateStoreBinding(binding); err != nil {
		return err
	}
	return s.withSnapshot(ctx, true, func(snapshot *diskSnapshot) error {
		if snapshot.Binding != nil {
			if *snapshot.Binding == binding {
				return nil
			}
			return ErrStoreIdentity
		}
		if len(snapshot.Records) != 0 || len(snapshot.Imports) != 0 {
			return ErrStoreIdentity
		}
		snapshot.Binding = &binding
		return nil
	})
}

// BindImportedResource durably records an exact current Resource identity
// supplied by the caller after an authorized Host read. Imports remain
// separate from mutation records and cannot clear or bypass unresolved intent.
func (s *Store) BindImportedResource(ctx context.Context, request ImportRequest) (ImportBinding, error) {
	target, evidence := request.Target, request.Evidence
	if err := validateTarget(target); err != nil || !validHostID(request.UID) || !validFormURL(request.Form) || !validHostAPIBaseURL(evidence.HostAPI) ||
		evidence.Path != "/resources/"+evidence.UID || evidence.StatusCode != 200 || !validHostID(evidence.UID) ||
		!validFormURL(evidence.Form) || evidence.Space != target.Space || evidence.Name != target.Name ||
		evidence.Generation < 1 || evidence.Generation > maxReplayWindowSeconds {
		return ImportBinding{}, ErrStoreUnavailable
	}
	if request.UID != evidence.UID || request.Form != evidence.Form {
		return ImportBinding{}, ErrIntentConflict
	}
	imported := ImportBinding{Target: target, UID: request.UID, Form: request.Form, Generation: evidence.Generation}
	var result ImportBinding
	err := s.withSnapshot(ctx, true, func(snapshot *diskSnapshot) error {
		if !bindingMatches(snapshot, s.storeID, target) || evidence.HostAPI != snapshot.Binding.HostAPI {
			return ErrStoreIdentity
		}
		for _, prior := range snapshot.Records {
			if sameTarget(prior.Target, target) {
				return ErrUnresolvedIntent
			}
		}
		for _, prior := range snapshot.Imports {
			if sameTarget(prior.Target, target) {
				if prior == imported {
					result = prior
					return nil
				}
				return ErrIntentConflict
			}
			if prior.Target.HostAPI == target.HostAPI && prior.UID == imported.UID {
				return ErrIntentConflict
			}
		}
		if len(snapshot.Records)+len(snapshot.Imports) >= maxRecords {
			return ErrStoreUnavailable
		}
		snapshot.Imports = append(snapshot.Imports, imported)
		result = imported
		return nil
	})
	return result, err
}

// ReadImportBinding returns the immutable imported identity for a target.
func (s *Store) ReadImportBinding(ctx context.Context, target Target) (ImportBinding, bool, error) {
	if err := validateTarget(target); err != nil {
		return ImportBinding{}, false, err
	}
	var result ImportBinding
	found := false
	err := s.withSnapshot(ctx, false, func(snapshot *diskSnapshot) error {
		if !bindingMatches(snapshot, s.storeID, target) {
			return ErrStoreIdentity
		}
		for _, imported := range snapshot.Imports {
			if sameTarget(imported.Target, target) {
				result, found = imported, true
				break
			}
		}
		return nil
	})
	return result, found, err
}

// Begin durably claims a new mutation before returning its random key. An
// identical unresolved intent returns its original key and timing data; a
// changed request for that identity fails closed.
func (s *Store) Begin(ctx context.Context, request BeginRequest) (Intent, error) {
	request, err := validateBegin(request)
	if err != nil {
		return Intent{}, err
	}
	var result Intent
	err = s.withSnapshot(ctx, true, func(snapshot *diskSnapshot) error {
		if !bindingMatches(snapshot, s.storeID, request.Target) {
			return ErrStoreIdentity
		}
		for i := len(snapshot.Records) - 1; i >= 0; i-- {
			prior := snapshot.Records[i]
			if !sameTarget(prior.Target, request.Target) {
				continue
			}
			if request.SupersedesIntentID != "" && prior.ID != request.SupersedesIntentID {
				if sameFence(prior, request) && sameRequest(prior, request) {
					result = copyIntent(prior)
					return nil
				}
				continue
			}
			// A confirmed Delete starts a new Create incarnation. Never walk
			// backward across that boundary and return an older Create key.
			if request.Action == ActionCreate && prior.Status == StatusDeleteConfirmed {
				break
			}
			if request.Action == ActionCreate && sameCreateRequest(prior, request) {
				result = copyIntent(prior)
				return nil
			}
			if request.Action == ActionCreate && prior.Action == ActionCreate &&
				(prior.Status == StatusClaimed || prior.Status == StatusAccepted) {
				return ErrIntentConflict
			}
			if request.Action != ActionCreate && sameFence(prior, request) {
				if sameRequest(prior, request) {
					result = copyIntent(prior)
					return nil
				}
				if prior.Status == StatusFailedReserved && request.SupersedesIntentID == prior.ID {
					continue
				}
				return ErrIntentConflict
			}
		}
		for _, prior := range snapshot.Records {
			if sameTarget(prior.Target, request.Target) && (prior.Status == StatusClaimed || prior.Status == StatusAccepted || prior.Status == StatusAcceptedFailed) {
				return ErrUnresolvedIntent
			}
		}
		latest := latestIntent(snapshot, request.Target)
		if request.Action == ActionCreate {
			_, _, _, identityFound, identityDeleted := currentIdentity(snapshot, request.Target)
			if identityFound && !identityDeleted || latest != nil && latest.Status != StatusDeleteConfirmed {
				return ErrTargetOccupied
			}
		} else {
			uid, generation, form, found, deleted := currentIdentity(snapshot, request.Target)
			if deleted || !found {
				return ErrTargetOccupied
			}
			if uid != request.UID || generation != request.Generation || form != request.Form {
				return ErrIntentConflict
			}
			if request.SupersedesIntentID != "" {
				prior, priorIndex := findIntent(snapshot, request.SupersedesIntentID)
				if priorIndex < 0 || latest == nil || latest.ID != request.SupersedesIntentID || prior.Status != StatusFailedReserved || !sameTarget(prior.Target, request.Target) || prior.OperationUID != uid || prior.ObservedGeneration != generation {
					return ErrIntentConflict
				}
			} else if latest != nil && latest.Status == StatusFailedReserved {
				return ErrUnresolvedIntent
			}
		}
		if len(snapshot.Records)+len(snapshot.Imports) >= maxRecords {
			return ErrStoreUnavailable
		}
		key, err := newKey()
		if err != nil {
			return ErrStoreUnavailable
		}
		result = Intent{
			ID: key, Key: key, Target: request.Target, Action: request.Action, Form: request.Form,
			Method: request.Method, Path: request.Path, UID: request.UID, Generation: request.Generation,
			SupersedesIntentID: request.SupersedesIntentID,
			PublicJSON:         append([]byte(nil), request.PublicJSON...), PrivateInputNames: append([]string(nil), request.PrivateInputNames...),
			FirstSentAt: time.Now().UTC(), AdvertisedReplayWindowSecs: request.AdvertisedReplayWindowSecs, Status: StatusClaimed,
		}
		snapshot.Records = append(snapshot.Records, result)
		return nil
	})
	if err != nil {
		return Intent{}, err
	}
	return copyIntent(result), nil
}

// Read returns a copy of the durable history for one Host/root/principal/Space/name.
func (s *Store) Read(ctx context.Context, target Target) ([]Intent, error) {
	if err := validateTarget(target); err != nil {
		return nil, err
	}
	var result []Intent
	err := s.withSnapshot(ctx, false, func(snapshot *diskSnapshot) error {
		for _, record := range snapshot.Records {
			if sameTarget(record.Target, target) {
				result = append(result, copyIntent(record))
			}
		}
		return nil
	})
	return result, err
}

// RecordAccepted records the Host Operation returned for a mutation. It never
// authorizes a resend and binds the acknowledgement to this action and fence.
func (s *Store) RecordAccepted(ctx context.Context, id string, ack OperationAck) (Intent, error) {
	if !validKey(id) || !validHostID(ack.ID) || !validHostID(ack.ResourceUID) || !validOperationStatus(ack.Status, ack.Effect) {
		return Intent{}, ErrStoreUnavailable
	}
	retainUntil, err := parseRetainUntil(ack.RetainUntil)
	if err != nil {
		return Intent{}, ErrStoreUnavailable
	}
	var result Intent
	err = s.withSnapshot(ctx, true, func(snapshot *diskSnapshot) error {
		record, index := findIntent(snapshot, id)
		if index < 0 {
			return ErrStoreUnavailable
		}
		if !bindingMatches(snapshot, s.storeID, record.Target) {
			return ErrStoreIdentity
		}
		if record.Status == StatusAccepted || record.Status == StatusAcceptedFailed {
			if !sameOperation(record, ack) || !record.RetainUntil.Equal(retainUntil) {
				return ErrIntentConflict
			}
			result = copyIntent(record)
			return nil
		}
		if record.Status != StatusClaimed {
			return ErrIntentConflict
		}
		if err := validateOperationForIntent(record, ack); err != nil {
			return ErrIntentConflict
		}
		record.Status = StatusAccepted
		if ack.Status == "failed" {
			record.Status = StatusAcceptedFailed
		}
		record.OperationID = ack.ID
		record.OperationUID = ack.ResourceUID
		record.OperationGeneration = ack.Generation
		record.OperationStatus = ack.Status
		record.OperationEffect = ack.Effect
		record.InitialRetainUntil = retainUntil
		record.RetainUntil = retainUntil
		snapshot.Records[index] = record
		result = copyIntent(record)
		return nil
	})
	return result, err
}

// Complete records a Host-observed terminal result. Exact operation and
// Resource evidence are required; a caller-supplied outcome label is not
// enough to settle the intent.
func (s *Store) Complete(ctx context.Context, id string, evidence CompletionEvidence) (Intent, error) {
	if !validKey(id) || !validHostID(evidence.Operation.ID) || !validHostID(evidence.Operation.ResourceUID) ||
		!validOperationStatus(evidence.Operation.Status, evidence.Operation.Effect) {
		return Intent{}, ErrStoreUnavailable
	}
	retainUntil, err := parseRetainUntil(evidence.Operation.RetainUntil)
	if err != nil {
		return Intent{}, ErrStoreUnavailable
	}
	var result Intent
	err = s.withSnapshot(ctx, true, func(snapshot *diskSnapshot) error {
		record, index := findIntent(snapshot, id)
		if index < 0 {
			return ErrStoreUnavailable
		}
		if !bindingMatches(snapshot, s.storeID, record.Target) {
			return ErrStoreIdentity
		}
		if !sameOperation(record, evidence.Operation) || evidence.Operation.Action != record.Action || retainUntil.Before(record.RetainUntil) || retainUntil.Before(record.InitialRetainUntil) {
			return ErrIntentConflict
		}
		if record.Status == StatusApplied && evidence.Operation.Status == "succeeded" && evidence.Operation.Effect == "complete" &&
			evidence.Resource.Present && resourceMatches(record, evidence.Resource, evidence.Operation.Generation) {
			record.RetainUntil = retainUntil
			snapshot.Records[index] = record
			result = copyIntent(record)
			return nil
		}
		if record.Status == StatusDeleteConfirmed && evidence.Operation.Status == "succeeded" && evidence.Operation.Effect == "complete" && resourceAbsentMatches(record, evidence.Resource) {
			record.RetainUntil = retainUntil
			snapshot.Records[index] = record
			result = copyIntent(record)
			return nil
		}
		if record.Status == StatusFailedReserved && evidence.Operation.Status == "failed" && evidence.Resource.Present &&
			evidence.Resource.Generation == record.OperationGeneration && evidence.Resource.Generation == mustGeneration(record.ObservedGeneration) &&
			resourceMatchesTarget(record, evidence.Resource) && evidence.Resource.Phase == "error" {
			record.RetainUntil = retainUntil
			snapshot.Records[index] = record
			result = copyIntent(record)
			return nil
		}
		if record.Status != StatusAccepted && record.Status != StatusAcceptedFailed {
			return ErrIntentConflict
		}
		if evidence.Operation.Status == "failed" {
			if evidence.Resource.Present && resourceMatchesTarget(record, evidence.Resource) &&
				evidence.Resource.Phase == "error" && evidence.Resource.Generation == evidence.Operation.Generation {
				record.Status = StatusFailedReserved
				record.ObservedGeneration = strconv.FormatInt(evidence.Resource.Generation, 10)
				record.OperationStatus = evidence.Operation.Status
				record.OperationEffect = evidence.Operation.Effect
				record.RetainUntil = retainUntil
				snapshot.Records[index] = record
				result = copyIntent(record)
				return nil
			}
			return ErrIntentConflict
		}
		if evidence.Operation.Status != "succeeded" || evidence.Operation.Effect != "complete" {
			return ErrIntentConflict
		}
		if record.Action == ActionDelete {
			if !resourceAbsentMatches(record, evidence.Resource) {
				return ErrIntentConflict
			}
			record.Status = StatusDeleteConfirmed
		} else {
			if !evidence.Resource.Present || !resourceMatches(record, evidence.Resource, evidence.Operation.Generation) {
				return ErrIntentConflict
			}
			record.Status = StatusApplied
			record.ResultUID = evidence.Operation.ResourceUID
			record.ResultGeneration = strconv.FormatInt(evidence.Operation.Generation, 10)
		}
		record.OperationStatus = evidence.Operation.Status
		record.OperationEffect = evidence.Operation.Effect
		record.RetainUntil = retainUntil
		snapshot.Records[index] = record
		result = copyIntent(record)
		return nil
	})
	return result, err
}

func (s *Store) withSnapshot(ctx context.Context, write bool, fn func(*diskSnapshot) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || ctx == nil {
		return ErrStoreUnavailable
	}
	var result error
	err := s.dir.withLock(ctx, func() error {
		if err := s.verifyPathAndHeader(); err != nil {
			return err
		}
		data, err := s.dir.readFile("state.json", maxSnapshotBytes)
		if err != nil {
			return ErrStoreUnavailable
		}
		snapshot, err := decodeSnapshot(data, s.storeID)
		if err != nil {
			return ErrStoreUnavailable
		}
		if result = fn(&snapshot); result != nil {
			return result
		}
		if !write {
			return nil
		}
		encoded, err := jsonv2.Marshal(snapshot)
		if err != nil || len(encoded) > maxSnapshotBytes {
			return ErrStoreUnavailable
		}
		if err := s.verifyPathAndHeader(); err != nil {
			return err
		}
		return s.dir.replaceFile("state.json", encoded)
	})
	if err != nil {
		return err
	}
	return result
}

func (s *Store) verifyPathAndHeader() error {
	if err := s.dir.verifyPathIdentity(); err != nil {
		return ErrStoreUnavailable
	}
	if err := s.dir.validateStoreEntries(); err != nil {
		return ErrStoreUnavailable
	}
	data, err := s.dir.readFile("store.json", 4096)
	if err != nil {
		return ErrStoreUnavailable
	}
	var header diskHeader
	if err := decodeStrict(data, &header); err != nil || header.Format != storeFormat || header.StoreID != s.storeID || header.Path != s.path {
		return ErrStoreIdentity
	}
	return nil
}

func decodeSnapshot(data []byte, storeID string) (diskSnapshot, error) {
	var snapshot diskSnapshot
	if len(data) == 0 || len(data) > maxSnapshotBytes || decodeStrict(data, &snapshot) != nil ||
		snapshot.Format != storeFormat || snapshot.StoreID != storeID || snapshot.Records == nil ||
		len(snapshot.Records)+len(snapshot.Imports) > maxRecords {
		return diskSnapshot{}, ErrStoreUnavailable
	}
	if snapshot.Binding != nil && validateStoreBinding(*snapshot.Binding) != nil || snapshot.Binding == nil && len(snapshot.Imports) != 0 {
		return diskSnapshot{}, ErrStoreUnavailable
	}
	seen := make(map[string]bool, len(snapshot.Records))
	for _, record := range snapshot.Records {
		if !validStoredIntent(record) || seen[record.ID] {
			return diskSnapshot{}, ErrStoreUnavailable
		}
		if snapshot.Binding != nil && !bindingMatches(&snapshot, storeID, record.Target) {
			return diskSnapshot{}, ErrStoreUnavailable
		}
		seen[record.ID] = true
	}
	seenTargets := make(map[Target]bool, len(snapshot.Imports))
	seenUIDs := make(map[string]bool, len(snapshot.Imports))
	for _, imported := range snapshot.Imports {
		if snapshot.Binding == nil || !bindingMatches(&snapshot, storeID, imported.Target) || !validHostID(imported.UID) ||
			!validFormURL(imported.Form) || imported.Generation < 1 || imported.Generation > maxReplayWindowSeconds ||
			seenTargets[imported.Target] || seenUIDs[imported.Target.HostAPI+"\x00"+imported.UID] {
			return diskSnapshot{}, ErrStoreUnavailable
		}
		seenTargets[imported.Target] = true
		seenUIDs[imported.Target.HostAPI+"\x00"+imported.UID] = true
	}
	return snapshot, nil
}

func decodeStrict(data []byte, value any) error {
	if err := jsonv2.Validate(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func validateBegin(r BeginRequest) (BeginRequest, error) {
	if err := validateTarget(r.Target); err != nil || (r.Action != ActionCreate && r.Action != ActionUpdate && r.Action != ActionDelete) ||
		!validFormURL(r.Form) || len(r.Path) > maxTargetComponentBytes || strings.ContainsAny(r.Path, "?#") ||
		!utf8.ValidString(r.Path) || len(r.PublicJSON) > maxPublicJSONBytes ||
		r.AdvertisedReplayWindowSecs <= 0 || r.AdvertisedReplayWindowSecs > maxReplayWindowSeconds {
		return BeginRequest{}, ErrStoreUnavailable
	}
	if !validHostAPIBaseURL(r.Target.HostAPI) {
		return BeginRequest{}, ErrStoreUnavailable
	}
	if r.Action == ActionCreate && (r.UID != "" || r.Generation != "" || r.Method != "POST" || r.Path != "/resources") ||
		r.Action == ActionUpdate && (r.UID == "" || r.Method != "PUT" || r.Path != "/resources/"+r.UID || !validExpectedGeneration(r.Generation)) ||
		r.Action == ActionDelete && (r.UID == "" || r.Method != "DELETE" || r.Path != "/resources/"+r.UID || !validExpectedGeneration(r.Generation) || len(r.PublicJSON) != 0 || len(r.PrivateInputNames) != 0) {
		return BeginRequest{}, ErrStoreUnavailable
	}
	if r.Action != ActionDelete {
		if len(r.PublicJSON) == 0 {
			return BeginRequest{}, ErrStoreUnavailable
		}
		decoded, err := jsonv2.Decode(r.PublicJSON)
		publicObject, ok := decoded.(map[string]any)
		if err != nil || !ok {
			return BeginRequest{}, ErrStoreUnavailable
		}
		if _, hasPrivateInputs := publicObject["privateInputs"]; hasPrivateInputs {
			return BeginRequest{}, ErrStoreUnavailable
		}
		if (r.Action == ActionCreate && !validCreatePublicObject(publicObject, r)) ||
			(r.Action == ActionUpdate && !validUpdatePublicObject(publicObject)) {
			return BeginRequest{}, ErrStoreUnavailable
		}
	}
	if len(r.PrivateInputNames) > maxPrivateInputNames || (r.SupersedesIntentID != "" && (!validKey(r.SupersedesIntentID) || r.Action == ActionCreate)) {
		return BeginRequest{}, ErrStoreUnavailable
	}
	names := append([]string(nil), r.PrivateInputNames...)
	sort.Strings(names)
	for i, name := range names {
		if name == "" || len(name) > maxTargetComponentBytes || !utf8.ValidString(name) || strings.ContainsRune(name, 0) || (i > 0 && name == names[i-1]) {
			return BeginRequest{}, ErrStoreUnavailable
		}
	}
	r.PrivateInputNames = names
	r.PublicJSON = append([]byte(nil), r.PublicJSON...)
	return r, nil
}

func validateTarget(t Target) error {
	if t.HostAPI == "" || t.PrincipalScope == "" || len(t.PrincipalScope) > maxPrincipalScopeBytes ||
		!utf8.ValidString(t.PrincipalScope) || strings.ContainsRune(t.PrincipalScope, 0) ||
		!validHostID(t.Space) || !validHostID(t.Name) {
		return ErrStoreUnavailable
	}
	return nil
}

func validateStoreBinding(binding StoreBinding) error {
	if !validHostAPIBaseURL(binding.HostAPI) {
		return ErrStoreUnavailable
	}
	return nil
}

func bindingMatches(snapshot *diskSnapshot, storeID string, target Target) bool {
	return snapshot != nil && snapshot.Binding != nil && validateStoreBinding(*snapshot.Binding) == nil &&
		target.HostAPI == snapshot.Binding.HostAPI && target.PrincipalScope == storeID
}

func validHostAPIBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.IsAbs() && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.ContainsAny(raw, "?#") &&
		!strings.HasSuffix(u.Path, "/") && isASCII(raw)
}

func validHostID(value string) bool { return hostIDPattern.MatchString(value) }

func validFormURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.IsAbs() && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.ContainsAny(raw, "?#") && isASCII(raw)
}

func isASCII(value string) bool {
	for _, character := range value {
		if character > 0x7f {
			return false
		}
	}
	return true
}

func validExpectedGeneration(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && parsed < maxReplayWindowSeconds && strconv.FormatInt(parsed, 10) == value
}

func validGeneration(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && parsed <= maxReplayWindowSeconds && strconv.FormatInt(parsed, 10) == value
}

func validCreatePublicObject(object map[string]any, request BeginRequest) bool {
	if !hasOnlyKeys(object, "form", "space", "name", "offering", "spec") {
		return false
	}
	form, formOK := object["form"].(string)
	space, spaceOK := object["space"].(string)
	name, nameOK := object["name"].(string)
	_, specOK := object["spec"].(map[string]any)
	if !formOK || !spaceOK || !nameOK || !specOK || form != request.Form || space != request.Target.Space || name != request.Target.Name {
		return false
	}
	if rawOffering, exists := object["offering"]; exists {
		offering, ok := rawOffering.(map[string]any)
		if !ok || !hasOnlyKeys(offering, "id", "revision") {
			return false
		}
		id, idOK := offering["id"].(string)
		revision, revisionOK := offering["revision"].(string)
		if !idOK || !validHostID(id) || !revisionOK || revision == "" {
			return false
		}
	}
	return true
}

func validUpdatePublicObject(object map[string]any) bool {
	_, hasSpec := object["spec"].(map[string]any)
	return hasOnlyKeys(object, "spec") && hasSpec
}

func hasOnlyKeys(object map[string]any, keys ...string) bool {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	for key := range object {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func parseRetainUntil(value string) (time.Time, error) {
	if value == "" || !strings.HasSuffix(value, "Z") {
		return time.Time{}, ErrStoreUnavailable
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || parsed.IsZero() {
		return time.Time{}, ErrStoreUnavailable
	}
	return parsed.UTC(), nil
}

func validOperationStatus(status, effect string) bool {
	switch status {
	case "queued", "running", "waiting_input", "reconciling", "succeeded", "failed":
	default:
		return false
	}
	switch effect {
	case "none", "unknown", "partial", "complete":
	default:
		return false
	}
	if status == "succeeded" && effect != "complete" || status == "failed" && effect != "none" && effect != "partial" || status == "reconciling" && effect != "unknown" {
		return false
	}
	return true
}

func validateOperationForIntent(record Intent, ack OperationAck) error {
	if ack.Action != record.Action || ack.Generation < 1 || ack.Generation > maxReplayWindowSeconds {
		return ErrIntentConflict
	}
	expected := int64(1)
	if record.Action != ActionCreate {
		generation, err := strconv.ParseInt(record.Generation, 10, 64)
		if err != nil || generation < 1 || generation >= maxReplayWindowSeconds {
			return ErrIntentConflict
		}
		expected = generation + 1
		if ack.ResourceUID != record.UID {
			return ErrIntentConflict
		}
	}
	if ack.Generation != expected {
		return ErrIntentConflict
	}
	return nil
}

func sameOperation(record Intent, ack OperationAck) bool {
	return record.OperationID == ack.ID && record.OperationUID == ack.ResourceUID && record.Action == ack.Action &&
		record.OperationGeneration == ack.Generation
}

func resourceMatchesTarget(record Intent, observation ResourceObservation) bool {
	return observation.Path == "/resources/"+record.OperationUID && observation.StatusCode == 200 && observation.Present &&
		observation.UID == record.OperationUID && observation.Form == record.Form && observation.Space == record.Target.Space &&
		observation.Name == record.Target.Name
}

func resourceMatches(record Intent, observation ResourceObservation, generation int64) bool {
	return resourceMatchesTarget(record, observation) && observation.Phase == "idle" && generation == record.OperationGeneration && observation.Generation == generation
}

func resourceAbsentMatches(record Intent, observation ResourceObservation) bool {
	return !observation.Present && observation.Path == "/resources/"+record.UID && observation.StatusCode == 404 &&
		observation.UID == "" && observation.Form == "" && observation.Space == "" && observation.Name == "" && observation.Generation == 0
}

func validateStoreID(id string) error {
	if id == "" || len(id) > 128 || !utf8.ValidString(id) || strings.ContainsRune(id, 0) || strings.TrimSpace(id) != id {
		return ErrStoreUnavailable
	}
	return nil
}

func sameTarget(a, b Target) bool { return a == b }

func sameFence(prior Intent, request BeginRequest) bool {
	return sameTarget(prior.Target, request.Target) && prior.Action == request.Action && prior.UID == request.UID && prior.Generation == request.Generation
}

func sameCreateRequest(prior Intent, request BeginRequest) bool {
	return prior.Action == ActionCreate && sameRequest(prior, request)
}

func sameRequest(prior Intent, request BeginRequest) bool {
	return sameTarget(prior.Target, request.Target) && prior.Action == request.Action && prior.Form == request.Form &&
		prior.Method == request.Method && prior.Path == request.Path && prior.UID == request.UID && prior.Generation == request.Generation &&
		prior.SupersedesIntentID == request.SupersedesIntentID &&
		bytes.Equal(prior.PublicJSON, request.PublicJSON) && sameStrings(prior.PrivateInputNames, request.PrivateInputNames)
}

func latestIntent(snapshot *diskSnapshot, target Target) *Intent {
	for index := len(snapshot.Records) - 1; index >= 0; index-- {
		if sameTarget(snapshot.Records[index].Target, target) {
			return &snapshot.Records[index]
		}
	}
	return nil
}

func currentIdentity(snapshot *diskSnapshot, target Target) (uid, generation, form string, found, deleted bool) {
	for index := len(snapshot.Records) - 1; index >= 0; index-- {
		record := snapshot.Records[index]
		if !sameTarget(record.Target, target) {
			continue
		}
		if record.Status == StatusDeleteConfirmed {
			return "", "", "", false, true
		}
		if record.Status == StatusApplied && (record.Action == ActionCreate || record.Action == ActionUpdate) {
			return record.ResultUID, record.ResultGeneration, record.Form, true, false
		}
		if record.Status == StatusFailedReserved {
			return record.OperationUID, record.ObservedGeneration, record.Form, true, false
		}
	}
	for _, imported := range snapshot.Imports {
		if sameTarget(imported.Target, target) {
			return imported.UID, strconv.FormatInt(imported.Generation, 10), imported.Form, true, false
		}
	}
	return "", "", "", false, false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validStoredIntent(i Intent) bool {
	if !validKey(i.ID) || i.ID != i.Key || validateTarget(i.Target) != nil || i.FirstSentAt.IsZero() ||
		i.FirstSentAt.Location() != time.UTC || (!i.InitialRetainUntil.IsZero() && i.InitialRetainUntil.Location() != time.UTC) ||
		(!i.RetainUntil.IsZero() && i.RetainUntil.Location() != time.UTC) ||
		i.AdvertisedReplayWindowSecs <= 0 || i.AdvertisedReplayWindowSecs > maxReplayWindowSeconds ||
		!validOperationID(i.OperationID) || (!i.RetainUntil.IsZero() && i.OperationID == "") ||
		(!i.InitialRetainUntil.IsZero() && i.OperationID == "") ||
		(i.OperationID != "" && (i.InitialRetainUntil.IsZero() || i.RetainUntil.Before(i.InitialRetainUntil))) ||
		(i.OperationID == "" && (i.OperationUID != "" || i.OperationGeneration != 0 || i.OperationStatus != "" || i.OperationEffect != "")) ||
		(i.OperationID != "" && (!validHostID(i.OperationUID) || i.OperationGeneration < 1 || i.OperationGeneration > maxReplayWindowSeconds || !validOperationStatus(i.OperationStatus, i.OperationEffect))) {
		return false
	}
	request, err := validateBegin(BeginRequest{
		Target: i.Target, Action: i.Action, Form: i.Form, Method: i.Method, Path: i.Path,
		UID: i.UID, Generation: i.Generation, SupersedesIntentID: i.SupersedesIntentID, PublicJSON: i.PublicJSON,
		PrivateInputNames: i.PrivateInputNames, AdvertisedReplayWindowSecs: i.AdvertisedReplayWindowSecs,
	})
	if err != nil || !sameStrings(request.PrivateInputNames, i.PrivateInputNames) {
		return false
	}
	if i.OperationID != "" {
		ack := OperationAck{ID: i.OperationID, ResourceUID: i.OperationUID, Action: i.Action, Generation: i.OperationGeneration, Status: i.OperationStatus, Effect: i.OperationEffect, RetainUntil: i.RetainUntil.Format(time.RFC3339Nano)}
		if validateOperationForIntent(i, ack) != nil {
			return false
		}
	}
	switch i.Status {
	case StatusClaimed:
		if i.OperationID != "" || !i.InitialRetainUntil.IsZero() || !i.RetainUntil.IsZero() || i.ResultUID != "" || i.ResultGeneration != "" || i.ObservedGeneration != "" {
			return false
		}
	case StatusAccepted:
		if i.OperationID == "" || i.OperationStatus == "failed" || i.ResultUID != "" || i.ResultGeneration != "" || i.ObservedGeneration != "" {
			return false
		}
	case StatusAcceptedFailed:
		if i.OperationID == "" || i.OperationStatus != "failed" || i.ResultUID != "" || i.ResultGeneration != "" || i.ObservedGeneration != "" {
			return false
		}
	case StatusFailedReserved:
		if i.OperationID == "" || i.OperationStatus != "failed" || !validGeneration(i.ObservedGeneration) || mustGeneration(i.ObservedGeneration) != i.OperationGeneration || i.ResultUID != "" || i.ResultGeneration != "" {
			return false
		}
	case StatusApplied:
		if (i.Action != ActionCreate && i.Action != ActionUpdate) || i.OperationID == "" || i.OperationStatus != "succeeded" || i.OperationEffect != "complete" ||
			!validResultIdentity(i.ResultUID, i.ResultGeneration) || i.ResultUID != i.OperationUID || mustGeneration(i.ResultGeneration) != i.OperationGeneration {
			return false
		}
	case StatusDeleteConfirmed:
		if i.Action != ActionDelete || i.OperationID == "" || i.OperationStatus != "succeeded" || i.OperationEffect != "complete" ||
			i.OperationUID != i.UID || i.ResultUID != "" || i.ResultGeneration != "" {
			return false
		}
	default:
		return false
	}
	return true
}

func validResultIdentity(uid, generation string) bool {
	return validHostID(uid) && validGeneration(generation)
}

func validOperationID(id string) bool {
	return id == "" || validHostID(id)
}

func mustGeneration(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func validKey(key string) bool {
	decoded, err := hex.DecodeString(key)
	return err == nil && len(decoded) == idempotencyKeyBytes
}

func newKey() (string, error) {
	var value [idempotencyKeyBytes]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("intentv2: random key unavailable")
	}
	return hex.EncodeToString(value[:]), nil
}

func findIntent(snapshot *diskSnapshot, id string) (Intent, int) {
	for index, record := range snapshot.Records {
		if record.ID == id {
			return record, index
		}
	}
	return Intent{}, -1
}

func copyIntent(record Intent) Intent {
	record.PublicJSON = append([]byte(nil), record.PublicJSON...)
	record.PrivateInputNames = append([]string(nil), record.PrivateInputNames...)
	return record
}

package portableconformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const APIVersion = "forms.takoform.com/v1alpha1"

type FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
}

type InstalledFormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest"`
}

type RunnerInput struct {
	Space          string                 `json:"space"`
	Name           string                 `json:"name"`
	Identity       InstalledFormReference `json:"identity"`
	Desired        map[string]any         `json:"desired"`
	ImportNativeID string                 `json:"importNativeId"`
}

type Contract struct {
	Format                 string            `json:"format"`
	APIVersion             string            `json:"apiVersion"`
	DiscoveryPath          string            `json:"discoveryPath"`
	APIPath                string            `json:"apiPath"`
	CompatibilityPath      string            `json:"compatibilityPath"`
	RunnerInput            RunnerInput       `json:"runnerInput"`
	Preconditions          map[string]string `json:"preconditions"`
	IdempotentOperations   []string          `json:"idempotentOperations"`
	StableErrorCodes       []string          `json:"stableErrorCodes"`
	RetryableCodes         []string          `json:"retryableCodes"`
	RequiredRunnerChecks   []string          `json:"requiredRunnerChecks"`
	ForbiddenProviderState []string          `json:"forbiddenProviderState"`
	TakosumiRunnerSource   string            `json:"takosumiRunnerSource"`
}

type manifest struct {
	Format   string `json:"format"`
	Contract string `json:"contract"`
	SHA256   string `json:"sha256"`
}

func Verify(root string) (Contract, error) {
	var index manifest
	if err := decodeStrict(filepath.Join(root, "manifest.json"), &index); err != nil {
		return Contract{}, err
	}
	if index.Format != "takoform.portable-host-conformance-manifest@v1" || index.Contract != "contract.json" {
		return Contract{}, errors.New("portable host conformance manifest identity is invalid")
	}
	contractPath := filepath.Join(root, index.Contract)
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return Contract{}, err
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != index.SHA256 {
		return Contract{}, errors.New("portable host conformance contract digest drifted")
	}
	var contract Contract
	if err := decodeStrict(contractPath, &contract); err != nil {
		return Contract{}, err
	}
	if err := validate(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

func validate(contract Contract) error {
	if contract.Format != "takoform.portable-host-conformance@v1" || contract.APIVersion != APIVersion ||
		contract.DiscoveryPath != "/.well-known/takoform" || contract.APIPath != "/apis/forms.takoform.com/v1alpha1" ||
		contract.CompatibilityPath != "/v1" || contract.TakosumiRunnerSource != "core/conformance/portable_form_host.ts" {
		return errors.New("portable host contract identity is invalid")
	}
	identity := contract.RunnerInput.Identity
	if identity.FormRef.APIVersion != APIVersion || identity.FormRef.Kind == "" || identity.FormRef.DefinitionVersion == "" ||
		!digest(identity.FormRef.SchemaDigest) || !digest(identity.PackageDigest) || contract.RunnerInput.Space == "" ||
		contract.RunnerInput.Name == "" || contract.RunnerInput.ImportNativeID == "" || len(contract.RunnerInput.Desired) == 0 {
		return errors.New("portable host runner input is incomplete")
	}
	wantPreconditions := map[string]string{
		"create": "If-None-Match: *", "update": "If-Match: quoted resourceVersion",
		"import":  "If-None-Match: * or If-Match: quoted resourceVersion",
		"observe": "If-Match: quoted resourceVersion", "refresh": "If-Match: quoted resourceVersion",
		"delete": "If-Match: quoted resourceVersion",
	}
	if !reflect.DeepEqual(contract.Preconditions, wantPreconditions) {
		return errors.New("portable host mutation preconditions drifted")
	}
	wantIdempotent := []string{"apply", "import", "observe", "refresh", "delete"}
	if !reflect.DeepEqual(contract.IdempotentOperations, wantIdempotent) {
		return errors.New("portable host idempotency operations drifted")
	}
	wantRetryable := []string{"resource_busy", "backend_unavailable"}
	if !reflect.DeepEqual(contract.RetryableCodes, wantRetryable) {
		return errors.New("portable host retry taxonomy drifted")
	}
	wantErrors := []string{
		"invalid_argument", "unauthenticated", "permission_denied", "form_unknown", "form_not_installed",
		"form_unavailable", "form_identity_conflict", "resource_not_found", "resource_version_conflict",
		"resource_busy", "import_conflict", "policy_denied", "backend_unavailable", "internal_error",
	}
	if !reflect.DeepEqual(contract.StableErrorCodes, wantErrors) {
		return errors.New("portable host stable error taxonomy drifted")
	}
	wantChecks := []string{
		"discovery", "exact-availability", "preview", "apply", "apply-idempotency", "read",
		"canonical-resource-parity", "exact-digest-substitution-rejected", "observe", "refresh",
		"canonical-audit-parity", "import-idempotency", "delete-idempotency",
	}
	if !reflect.DeepEqual(contract.RequiredRunnerChecks, wantChecks) {
		return errors.New("portable host required runner checks drifted")
	}
	wantForbidden := []string{
		"credential", "secret", "price", "quote", "billing", "backend", "selected_implementation", "target", "locked",
	}
	if !reflect.DeepEqual(contract.ForbiddenProviderState, wantForbidden) {
		return errors.New("portable host forbidden provider state list drifted")
	}
	return nil
}

func decodeStrict(path string, value any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode %s: trailing value", path)
	}
	return nil
}

func digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

// Package admissioncheckpoint owns the immutable historical assignment ledger
// for the retired Standard Form admission namespace. It grants no current
// admission, publication, maturity, activation, or tag-creation authority.
package admissioncheckpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	IdentityLedgerPath    = "admission/admission-identities.json"
	maxIdentityLedgerSize = 32 << 10
)

// IdentityLedger is the closed assignment history for the retired namespace.
type IdentityLedger struct {
	Format  string          `json:"format"`
	Entries []IdentityEntry `json:"entries"`
}

// IdentityEntry records one exact historical or abandoned assignment.
type IdentityEntry struct {
	Version       string   `json:"version"`
	Tag           string   `json:"tag"`
	Status        string   `json:"status"`
	TagObject     string   `json:"tagObject,omitempty"`
	Commit        string   `json:"commit,omitempty"`
	RetainedRoot  string   `json:"retainedRoot,omitempty"`
	RetainedTree  string   `json:"retainedTree,omitempty"`
	SetDigest     string   `json:"setDigest,omitempty"`
	RetainedPaths []string `json:"retainedPaths,omitempty"`
}

// LoadHistory strictly reads and validates the closed historical ledger.
func LoadHistory(root string) (IdentityLedger, error) {
	filename := filepath.Join(root, filepath.FromSlash(IdentityLedgerPath))
	info, err := os.Lstat(filename)
	if err != nil {
		return IdentityLedger{}, fmt.Errorf("read %s: %w", IdentityLedgerPath, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxIdentityLedgerSize {
		return IdentityLedger{}, fmt.Errorf("%s must be one bounded regular non-symlink file", IdentityLedgerPath)
	}
	raw, err := os.ReadFile(filename)
	if err != nil {
		return IdentityLedger{}, err
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return IdentityLedger{}, fmt.Errorf("%s must be strict I-JSON: %w", IdentityLedgerPath, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var ledger IdentityLedger
	if err := decoder.Decode(&ledger); err != nil {
		return IdentityLedger{}, fmt.Errorf("decode %s: %w", IdentityLedgerPath, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return IdentityLedger{}, fmt.Errorf("decode %s: trailing JSON value", IdentityLedgerPath)
	}
	if err := ledger.ValidateHistory(root); err != nil {
		return IdentityLedger{}, fmt.Errorf("%s: %w", IdentityLedgerPath, err)
	}
	return ledger, nil
}

// ValidateHistory pins every published tag object, commit, retained subtree,
// and set digest. Unpublished 1.0.5 remains permanently reserved.
func (ledger IdentityLedger) ValidateHistory(root string) error {
	if ledger.Format != "takoform.standard-admission-identities@v2" {
		return fmt.Errorf("format is %q", ledger.Format)
	}
	expected := expectedHistory()
	if !reflect.DeepEqual(ledger.Entries, expected) {
		return fmt.Errorf("entries do not equal the exact historical/reserved assignment closure")
	}
	for _, retained := range expected[4].RetainedPaths {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(retained)))
		if err != nil {
			return fmt.Errorf("reserved v1.0.5 evidence %s: %w", retained, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("reserved v1.0.5 evidence %s must be one retained directory", retained)
		}
	}
	return nil
}

func expectedHistory() []IdentityEntry {
	return []IdentityEntry{
		{Version: "1.0.1", Tag: "forms/admissions/v1.0.1", Status: "assigned-historical", TagObject: "2b1ca9f68688392869a79de122fbce2a54842301", Commit: "57aba7f374bb0d45274044e1dacbea52d16f3f6b", RetainedRoot: "admission/v1", RetainedTree: "6add327241bfb347a1e8766efa699c6c483c46bb", SetDigest: "sha256:892ae3d51586e50ac0b1c51c57a4889e70d52d1788b379652acb0fd11b680e1b"},
		{Version: "1.0.2", Tag: "forms/admissions/v1.0.2", Status: "assigned-historical", TagObject: "98af8dd461f24e6dc902f5c56dc6740f74ceb5af", Commit: "ff65142ecfab206f58239f095b5e170854ef9dde", RetainedRoot: "admission/v1", RetainedTree: "c9654cfd490bd0f8282984ecad031f5e17b07b5f", SetDigest: "sha256:b5da01634c1da78afe527e05af17487ddaf76dac9a85317ebc5298ea4395f0fb"},
		{Version: "1.0.3", Tag: "forms/admissions/v1.0.3", Status: "assigned-historical", TagObject: "82af8a61666e0194506d0d23d04422ccda4b3d86", Commit: "4a40826c7ed467af84e856487998ce365ffe00dd", RetainedRoot: "admission/v1", RetainedTree: "94c9d4ab118880f8a1f6be3db9a68084e63b6f66", SetDigest: "sha256:4f0d8942e437262465e079d89d74e29d36f7d7f7d53312bb1cb29671f34c82fc"},
		{Version: "1.0.4", Tag: "forms/admissions/v1.0.4", Status: "assigned-historical", TagObject: "b49a55016362d8787966f41b14570e3b67b8ddba", Commit: "a426a379e2743b4345e868becf3618357c015447", RetainedRoot: "admission/v1", RetainedTree: "d2255a52b30b7542554d3468f9dc34e867eae3ec", SetDigest: "sha256:e37b651716a74739d60e91b14aa00e39851ec4fe2fb591957644f7eeafc041c3"},
		{Version: "1.0.5", Tag: "forms/admissions/v1.0.5", Status: "reserved-abandoned", RetainedPaths: []string{"admission/v3/candidates/host-report-1.0.5-63dabf0c64be-bd0b3184aaad", "admission/v3/candidates/provider-report-1.0.5-bd0b3184aaad", "admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad"}},
		{Version: "1.0.6", Tag: "forms/admissions/v1.0.6", Status: "assigned-historical", TagObject: "b34b13a6e2fd3acbcbd73935e3e353f5d05b5c31", Commit: "1e438d61ed77f1ccfd3e000250f7dcf0c578c1af", RetainedRoot: "admission/v4", RetainedTree: "2d7fd930e86738619388f91c0083a983c38628cb", SetDigest: "sha256:ab73d976ed8f8e727a835101e80e3bd519931aa730e87d14de9bbb539bf72951"},
		{Version: "1.0.7", Tag: "forms/admissions/v1.0.7", Status: "assigned-historical", TagObject: "83a231dfb1a39af50f86de6fe8b7a9004e6b7075", Commit: "e56cfc866cc98469bbe4fcfe106cfc73cb08ae8c", RetainedRoot: "admission/v4", RetainedTree: "5b81172ca01f0c47a3d6b7ff653e591dd4b5d5bb", SetDigest: "sha256:6c85f5eebc35d77eef9646f782bd63bfb08ee00321baf0f42f95d36cbd408462"},
	}
}

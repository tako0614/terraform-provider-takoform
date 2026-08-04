// Package admissioncheckpoint owns the source descriptor and assignment ledger
// for the current, source-retained Standard Form admission checkpoint. It does
// not create commits, tags, releases, or any other remote state.
package admissioncheckpoint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	CurrentDescriptorPath     = "admission/v4/version.json"
	CurrentIdentityLedgerPath = "admission/admission-identities.json"

	currentFormat       = "takoform.standard-admission-checkpoint@v1"
	currentGeneration   = "ga-core-v2"
	currentRetainedRoot = "admission/v4"
	maxDescriptorBytes  = 16 << 10
)

var stableVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Descriptor is the one authoring identity for the current admission
// checkpoint. The generated standard-admission-set is a projection of its
// generation and tag, not another version authority.
type Descriptor struct {
	Format       string `json:"format"`
	Version      string `json:"version"`
	Tag          string `json:"tag"`
	Generation   string `json:"generation"`
	RetainedRoot string `json:"retainedRoot"`
}

// IdentityLedger is the append-only assignment ledger for the admission
// checkpoint namespace. A reserved-abandoned version remains unavailable even
// when no remote tag was ever minted.
type IdentityLedger struct {
	Format  string          `json:"format"`
	Entries []IdentityEntry `json:"entries"`
}

// IdentityEntry records one exact historical, abandoned, or current assignment.
type IdentityEntry struct {
	Version        string   `json:"version"`
	Tag            string   `json:"tag"`
	Status         string   `json:"status"`
	TagObject      string   `json:"tagObject,omitempty"`
	Commit         string   `json:"commit,omitempty"`
	RetainedPaths  []string `json:"retainedPaths,omitempty"`
	DescriptorPath string   `json:"descriptorPath,omitempty"`
}

// LoadCurrent strictly reads and validates the current descriptor and
// assignment ledger from a repository root.
func LoadCurrent(root string) (Descriptor, IdentityLedger, error) {
	raw, err := readRegular(filepath.Join(root, filepath.FromSlash(CurrentDescriptorPath)), maxDescriptorBytes)
	if err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("read %s: %w", CurrentDescriptorPath, err)
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("%s is not strict I-JSON: %w", CurrentDescriptorPath, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var descriptor Descriptor
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("decode %s: %w", CurrentDescriptorPath, err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("decode %s: %w", CurrentDescriptorPath, err)
	}
	if err := descriptor.Validate(); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("%s: %w", CurrentDescriptorPath, err)
	}

	ledgerRaw, err := readRegular(filepath.Join(root, filepath.FromSlash(CurrentIdentityLedgerPath)), maxDescriptorBytes)
	if err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("read %s: %w", CurrentIdentityLedgerPath, err)
	}
	if _, err := formpackage.Canonicalize(ledgerRaw); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("%s is not strict I-JSON: %w", CurrentIdentityLedgerPath, err)
	}
	ledgerDecoder := json.NewDecoder(bytes.NewReader(ledgerRaw))
	ledgerDecoder.DisallowUnknownFields()
	var ledger IdentityLedger
	if err := ledgerDecoder.Decode(&ledger); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("decode %s: %w", CurrentIdentityLedgerPath, err)
	}
	if err := requireJSONEOF(ledgerDecoder); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("decode %s: %w", CurrentIdentityLedgerPath, err)
	}
	if err := ledger.Validate(root, descriptor); err != nil {
		return Descriptor{}, IdentityLedger{}, fmt.Errorf("%s: %w", CurrentIdentityLedgerPath, err)
	}

	return descriptor, ledger, nil
}

// Validate rejects identity drift or a mutable/pre-release checkpoint version.
func (descriptor Descriptor) Validate() error {
	if descriptor.Format != currentFormat {
		return fmt.Errorf("format is %q, want %q", descriptor.Format, currentFormat)
	}
	if !stableVersionPattern.MatchString(descriptor.Version) {
		return fmt.Errorf("version %q is not stable canonical SemVer", descriptor.Version)
	}
	if descriptor.Tag != "forms/admissions/v"+descriptor.Version {
		return fmt.Errorf("tag %q does not match version %q", descriptor.Tag, descriptor.Version)
	}
	if descriptor.Generation != currentGeneration {
		return fmt.Errorf("generation is %q, want %q", descriptor.Generation, currentGeneration)
	}
	if descriptor.RetainedRoot != currentRetainedRoot {
		return fmt.Errorf("retainedRoot is %q, want %q", descriptor.RetainedRoot, currentRetainedRoot)
	}
	return nil
}

// ValidateSetProjection requires the retained set to derive its identity from
// the descriptor.
func (descriptor Descriptor) ValidateSetProjection(generation, tag string) error {
	if generation != descriptor.Generation {
		return fmt.Errorf("standard-admission set generation %q does not match descriptor %q", generation, descriptor.Generation)
	}
	if tag != descriptor.Tag {
		return fmt.Errorf("standard-admission set tag %q does not match descriptor %q", tag, descriptor.Tag)
	}
	return nil
}

// Validate requires the complete historical assignment through the current
// identity. Historical tag object/commit identities and the abandoned v1.0.5
// source evidence cannot be silently removed, moved, or reused.
func (ledger IdentityLedger) Validate(root string, descriptor Descriptor) error {
	if ledger.Format != "takoform.standard-admission-identities@v1" {
		return fmt.Errorf("format is %q", ledger.Format)
	}
	expected := []IdentityEntry{
		{
			Version: "1.0.1", Tag: "forms/admissions/v1.0.1", Status: "assigned-historical",
			TagObject: "2b1ca9f68688392869a79de122fbce2a54842301", Commit: "57aba7f374bb0d45274044e1dacbea52d16f3f6b",
		},
		{
			Version: "1.0.2", Tag: "forms/admissions/v1.0.2", Status: "assigned-historical",
			TagObject: "98af8dd461f24e6dc902f5c56dc6740f74ceb5af", Commit: "ff65142ecfab206f58239f095b5e170854ef9dde",
		},
		{
			Version: "1.0.3", Tag: "forms/admissions/v1.0.3", Status: "assigned-historical",
			TagObject: "82af8a61666e0194506d0d23d04422ccda4b3d86", Commit: "4a40826c7ed467af84e856487998ce365ffe00dd",
		},
		{
			Version: "1.0.4", Tag: "forms/admissions/v1.0.4", Status: "assigned-historical",
			TagObject: "b49a55016362d8787966f41b14570e3b67b8ddba", Commit: "a426a379e2743b4345e868becf3618357c015447",
		},
		{
			Version: "1.0.5", Tag: "forms/admissions/v1.0.5", Status: "reserved-abandoned",
			RetainedPaths: []string{
				"admission/v3/candidates/host-report-1.0.5-63dabf0c64be-bd0b3184aaad",
				"admission/v3/candidates/provider-report-1.0.5-bd0b3184aaad",
				"admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad",
			},
		},
		{
			Version: "1.0.6", Tag: "forms/admissions/v1.0.6", Status: "assigned-historical",
			TagObject: "b34b13a6e2fd3acbcbd73935e3e353f5d05b5c31", Commit: "1e438d61ed77f1ccfd3e000250f7dcf0c578c1af",
		},
		{
			Version: descriptor.Version, Tag: descriptor.Tag, Status: "assigned-current",
			DescriptorPath: CurrentDescriptorPath,
		},
	}
	if len(ledger.Entries) != len(expected) {
		return fmt.Errorf("entries has %d identities, want exact historical/reserved/current assignment closure %d", len(ledger.Entries), len(expected))
	}
	for index := range expected {
		if !identityEntryEqual(ledger.Entries[index], expected[index]) {
			return fmt.Errorf("entries[%d] does not equal the pinned identity assignment", index)
		}
	}
	for _, retained := range ledger.Entries[4].RetainedPaths {
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

func identityEntryEqual(actual, expected IdentityEntry) bool {
	if actual.Version != expected.Version ||
		actual.Tag != expected.Tag ||
		actual.Status != expected.Status ||
		actual.TagObject != expected.TagObject ||
		actual.Commit != expected.Commit ||
		actual.DescriptorPath != expected.DescriptorPath ||
		len(actual.RetainedPaths) != len(expected.RetainedPaths) {
		return false
	}
	for index := range expected.RetainedPaths {
		if actual.RetainedPaths[index] != expected.RetainedPaths[index] {
			return false
		}
	}
	return true
}

func readRegular(filename string, limit int64) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("must be one regular file")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("exceeds %d bytes", limit)
	}
	return os.ReadFile(filename)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("contains a trailing JSON value")
		}
		return err
	}
	return nil
}

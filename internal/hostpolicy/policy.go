// Package hostpolicy reads the reviewed set of conforming hosts whose signed
// lifecycle reports this repository accepts as admission input.
//
// Takoform is a portable contract. It therefore never hard-codes one host's
// repository, workflow, or subject as the definition of a valid proof: an
// accepted publisher is data in admission/v1/conforming-hosts.json, and any
// host that can produce the same signed evidence may be added by review.
package hostpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// PolicyPath is the repository-relative location of the reviewed policy.
const PolicyPath = "admission/v1/conforming-hosts.json"

const policyFormat = "takoform.conforming-host-policy@v1"

// pinsPath is the reviewed trust manifest that records this allowlist's exact
// bytes. Widening who may sign admission input is a change tamper-evidence has
// to cover, so the allowlist is read through its pin rather than beside it.
const pinsPath = "admission/v1/trust/offline-sigstore-pins.json"

var (
	hostIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	// EvidenceAPIVersionPattern accepts any publisher-namespaced portable host
	// conformance evidence version. The namespace names who produced the
	// evidence; it never names who owns the contract.
	EvidenceAPIVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*\.portable-form-host-conformance/v1$`)
)

// Host is one accepted host-report publisher.
type Host struct {
	HostID              string `json:"hostId"`
	Title               string `json:"title"`
	ManifestFormat      string `json:"manifestFormat"`
	SignedFormat        string `json:"signedFormat"`
	CertificateIdentity string `json:"certificateIdentity"`
	Workflow            string `json:"workflow"`
	SourceRepository    string `json:"sourceRepository"`
	ProofType           string `json:"proofType"`
	Subject             string `json:"subject"`
	RunnerVersionPrefix string `json:"runnerVersionPrefix"`
	EvidenceAPIVersion  string `json:"evidenceApiVersion"`
}

// Policy is the reviewed allowlist.
type Policy struct {
	Format      string `json:"format"`
	Description string `json:"description"`
	Hosts       []Host `json:"hosts"`
}

// Load reads the allowlist, proves it is the exact pinned bytes, and validates
// every entry.
func Load(root string) (Policy, error) {
	return LoadAt(root, "admission/v1")
}

// LoadAt reads one explicit retained admission generation. This keeps the
// historical v1 policy immutable while allowing a current generation to pin a
// different manifest format without changing who owns the portable contract.
func LoadAt(root, retainedRoot string) (Policy, error) {
	if retainedRoot == "" || path.Clean(retainedRoot) != retainedRoot ||
		strings.HasPrefix(retainedRoot, "../") || strings.Contains(retainedRoot, `\`) {
		return Policy{}, fmt.Errorf("invalid retained admission root %q", retainedRoot)
	}
	policyPath := path.Join(retainedRoot, "conforming-hosts.json")
	pinsPath := path.Join(retainedRoot, "trust/offline-sigstore-pins.json")
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(policyPath)))
	if err != nil {
		return Policy{}, err
	}
	if err := verifyPinnedBytesAt(root, policyPath, pinsPath, raw); err != nil {
		return Policy{}, err
	}
	var policy Policy
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", policyPath, err)
	}
	if policy.Format != policyFormat {
		return Policy{}, fmt.Errorf("%s: format is %q, want %q", policyPath, policy.Format, policyFormat)
	}
	if len(policy.Hosts) == 0 {
		return Policy{}, fmt.Errorf("%s: no conforming host is accepted", policyPath)
	}
	seen := make(map[string]struct{}, len(policy.Hosts))
	for _, host := range policy.Hosts {
		if err := host.validate(); err != nil {
			return Policy{}, fmt.Errorf("%s: %w", policyPath, err)
		}
		if _, duplicate := seen[host.HostID]; duplicate {
			return Policy{}, fmt.Errorf("%s: duplicate hostId %q", policyPath, host.HostID)
		}
		seen[host.HostID] = struct{}{}
	}
	return policy, nil
}

func (h Host) validate() error {
	if !hostIDPattern.MatchString(h.HostID) {
		return fmt.Errorf("invalid hostId %q", h.HostID)
	}
	required := map[string]string{
		"manifestFormat":      h.ManifestFormat,
		"signedFormat":        h.SignedFormat,
		"certificateIdentity": h.CertificateIdentity,
		"workflow":            h.Workflow,
		"sourceRepository":    h.SourceRepository,
		"proofType":           h.ProofType,
		"subject":             h.Subject,
		"runnerVersionPrefix": h.RunnerVersionPrefix,
		"evidenceApiVersion":  h.EvidenceAPIVersion,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("host %q omits %s", h.HostID, field)
		}
	}
	if !strings.HasPrefix(h.CertificateIdentity, "https://") {
		return fmt.Errorf("host %q certificateIdentity must be an https workflow identity", h.HostID)
	}
	if !strings.HasPrefix(h.SourceRepository, "https://") || !strings.HasSuffix(h.SourceRepository, ".git") {
		return fmt.Errorf("host %q sourceRepository must be an https git URL", h.HostID)
	}
	if !strings.HasPrefix(h.Subject, "host:") {
		return fmt.Errorf("host %q subject must be host-scoped", h.HostID)
	}
	if !EvidenceAPIVersionPattern.MatchString(h.EvidenceAPIVersion) {
		return fmt.Errorf("host %q evidenceApiVersion %q is not publisher-namespaced portable host conformance", h.HostID, h.EvidenceAPIVersion)
	}
	return nil
}

// ByHostID selects an accepted host by its reviewed identifier.
func (p Policy) ByHostID(hostID string) (Host, error) {
	for _, host := range p.Hosts {
		if host.HostID == hostID {
			return host, nil
		}
	}
	return Host{}, fmt.Errorf("no conforming host in %s has hostId %q", PolicyPath, hostID)
}

func verifyPinnedBytes(root string, raw []byte) error {
	return verifyPinnedBytesAt(root, PolicyPath, pinsPath, raw)
}

func verifyPinnedBytesAt(root, policyPath, pinsPath string, raw []byte) error {
	pinsRaw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(pinsPath)))
	if err != nil {
		return fmt.Errorf("read %s: %w", pinsPath, err)
	}
	var pins struct {
		ConformingHosts struct {
			Path   string `json:"path"`
			Digest string `json:"digest"`
		} `json:"conformingHosts"`
	}
	if err := json.Unmarshal(pinsRaw, &pins); err != nil {
		return fmt.Errorf("decode %s: %w", pinsPath, err)
	}
	if pins.ConformingHosts.Path != "conforming-hosts.json" {
		return fmt.Errorf("%s does not pin the conforming-host allowlist", pinsPath)
	}
	sum := sha256.Sum256(raw)
	if actual := "sha256:" + hex.EncodeToString(sum[:]); actual != pins.ConformingHosts.Digest {
		return fmt.Errorf("%s digest %s does not match its pin %s", policyPath, actual, pins.ConformingHosts.Digest)
	}
	return nil
}

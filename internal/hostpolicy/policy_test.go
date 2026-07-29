package hostpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReviewedPolicyLoads proves the committed allowlist is valid and that
// Takosumi is one entry in it rather than the definition of a valid proof.
func TestReviewedPolicyLoads(t *testing.T) {
	policy, err := Load(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(policy.Hosts) == 0 {
		t.Fatal("no conforming host is accepted")
	}
	for _, host := range policy.Hosts {
		if _, err := policy.ByHostID(host.HostID); err != nil {
			t.Fatalf("%s is not selectable: %v", host.HostID, err)
		}
	}
	if _, err := policy.ByHostID("no-such-host"); err == nil {
		t.Fatal("an unlisted host was selectable")
	}
}

func TestCurrentReviewedPolicyLoadsWithoutRelabelingV1(t *testing.T) {
	root := filepath.Join("..", "..")
	policy, err := LoadAt(root, "admission/v3")
	if err != nil {
		t.Fatal(err)
	}
	host, err := policy.ByHostID("takosumi-oss-reference")
	if err != nil {
		t.Fatal(err)
	}
	if host.ManifestFormat != "takosumi.standard-form-host-report-candidate@v2" ||
		host.SignedFormat != "takosumi.standard-form-host-report-signed-candidate@v2" ||
		host.RunnerVersionPrefix != "1.2.0+git." {
		t.Fatalf("current host contract drifted: %#v", host)
	}
	legacy, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	legacyHost, err := legacy.ByHostID("takosumi-oss-reference")
	if err != nil {
		t.Fatal(err)
	}
	if legacyHost.ManifestFormat != "takosumi.standard-form-host-report-candidate@v1" {
		t.Fatalf("historical host contract was relabeled: %#v", legacyHost)
	}
}

// TestPolicyRejectsUnusableEntries keeps the allowlist fail-closed: a malformed
// entry must not silently widen who may sign admission input.
func TestPolicyRejectsUnusableEntries(t *testing.T) {
	valid := validTestPolicy
	for name, mutation := range map[string]func(string) string{
		"unknown field": func(s string) string { return strings.Replace(s, `"hostId":"h"`, `"hostId":"h","extra":1`, 1) },
		"wrong format":  func(s string) string { return strings.Replace(s, "conforming-host-policy@v1", "other@v1", 1) },
		"plaintext identity": func(s string) string {
			return strings.Replace(s, "https://example.invalid/w.yml", "http://example.invalid/w.yml", 1)
		},
		"unscoped subject": func(s string) string { return strings.Replace(s, `"subject":"host:`, `"subject":"`, 1) },
		"unnamespaced evidence": func(s string) string {
			return strings.Replace(s, "h.portable-form-host-conformance/v1", "portable/v1", 1)
		},
		"missing runner prefix": func(s string) string {
			return strings.Replace(s, `"runnerVersionPrefix":"1.0.0+git."`, `"runnerVersionPrefix":""`, 1)
		},
		"non-git source": func(s string) string {
			return strings.Replace(s, "https://example.invalid/h.git", "https://example.invalid/h", 1)
		},
	} {
		root := writePinnedPolicy(t, mutation(valid))
		if _, err := Load(root); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// TestLoadRejectsUnpinnedAllowlist proves the allowlist cannot be widened
// without the reviewed trust manifest recording the new bytes.
func TestLoadRejectsUnpinnedAllowlist(t *testing.T) {
	root := writePinnedPolicy(t, validTestPolicy)
	widened := strings.Replace(validTestPolicy, `"hostId":"h"`, `"hostId":"h2"`, 1)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(PolicyPath)), []byte(widened), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "does not match its pin") {
		t.Fatalf("unpinned allowlist error = %v", err)
	}
}

const validTestPolicy = `{"format":"takoform.conforming-host-policy@v1","hosts":[{"hostId":"h","title":"H",` +
	`"manifestFormat":"m","signedFormat":"s","certificateIdentity":"https://example.invalid/w.yml@refs/heads/main",` +
	`"workflow":".github/workflows/w.yml","sourceRepository":"https://example.invalid/h.git","proofType":"p",` +
	`"subject":"host:https://h.invalid","runnerVersionPrefix":"1.0.0+git.",` +
	`"evidenceApiVersion":"h.portable-form-host-conformance/v1"}]}`

func writePinnedPolicy(t *testing.T, policy string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "admission", "v1", "trust"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(PolicyPath)), []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(policy))
	pins := fmt.Sprintf(`{"conformingHosts":{"path":"conforming-hosts.json","digest":"sha256:%s"}}`, hex.EncodeToString(sum[:]))
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(pinsPath)), []byte(pins), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

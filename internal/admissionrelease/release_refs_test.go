package admissionrelease

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestRequireTagCommitRejectsMissingAndMismatchedRefs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	filename := filepath.Join(root, "fixture.txt")
	if err := os.WriteFile(filename, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "fixture.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "first")
	first := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	tag := "forms/k-iv4gc3lqnrsvg5dpojsq/v1.0.0"
	runTestGit(t, root, "tag", tag)
	if err := requireTagCommit(root, "fixture", tag, first); err != nil {
		t.Fatalf("exact tag ref: %v", err)
	}
	if err := requireTagCommit(root, "fixture", "forms/missing/v1.0.0", first); err == nil || !strings.Contains(err.Error(), "resolve") {
		t.Fatalf("missing tag error = %v", err)
	}
	if err := os.WriteFile(filename, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", "fixture.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "second")
	second := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	if err := requireCommitAncestor(root, "fixture tooling", first, second); err != nil {
		t.Fatalf("retained ancestor rejected: %v", err)
	}
	if err := requireCommitAncestor(root, "fixture tooling", second, first); err == nil || !strings.Contains(err.Error(), "not an ancestor") {
		t.Fatalf("non-ancestor tooling commit error = %v", err)
	}
	if err := requireCommitAncestor(root, "fixture tooling", strings.Repeat("f", 40), second); err == nil || !strings.Contains(err.Error(), "not retained") {
		t.Fatalf("missing tooling commit error = %v", err)
	}
	if err := requireTagCommit(root, "fixture", tag, second); err == nil || !strings.Contains(err.Error(), "want retained commit") {
		t.Fatalf("mismatched tag error = %v", err)
	}
}

func TestValidProviderSignatureRequiresExactFingerprint(t *testing.T) {
	t.Parallel()
	const (
		keyCreation      = uint64(1784214429)
		keyExpiry        = uint64(1847286429)
		signatureCreated = uint64(1784214430)
	)
	validSignature := "[GNUPG:] VALIDSIG " + providerSigningFingerprint +
		" 2026-07-16 " + strconv.FormatUint(signatureCreated, 10) +
		" 0 4 0 1 10 00 " + providerSigningFingerprint
	healthy := "[GNUPG:] GOODSIG 34FC18AC897FB709 Takoform Provider Release\n" + validSignature
	fingerprints := validSignatureFingerprints(healthy, keyCreation, keyExpiry, false)
	if len(fingerprints) != 1 || fingerprints[0] != providerSigningFingerprint {
		t.Fatalf("exact pinned provider fingerprint was rejected: %v", fingerprints)
	}
	unreviewed := validSignatureFingerprints(
		strings.Replace(healthy, providerSigningFingerprint, strings.Repeat("0", 40), 1),
		keyCreation,
		keyExpiry,
		false,
	)
	if len(unreviewed) != 0 {
		t.Fatalf("unreviewed provider signing fingerprint was accepted: %v", unreviewed)
	}
	historical := "[GNUPG:] KEYEXPIRED " + strconv.FormatUint(keyExpiry, 10) + "\n" +
		"[GNUPG:] EXPKEYSIG 34FC18AC897FB709 Takoform Provider Release\n" +
		validSignature
	fingerprints = validSignatureFingerprints(historical, keyCreation, keyExpiry, true)
	if len(fingerprints) != 1 || fingerprints[0] != providerSigningFingerprint {
		t.Fatalf("valid pre-expiry historical signature was rejected: %v", fingerprints)
	}
	atExpiry := strings.Replace(
		historical,
		strconv.FormatUint(signatureCreated, 10),
		strconv.FormatUint(keyExpiry, 10),
		1,
	)
	fingerprints = validSignatureFingerprints(atExpiry, keyCreation, keyExpiry, true)
	if len(fingerprints) != 1 || fingerprints[0] != providerSigningFingerprint {
		t.Fatalf("signature at explicitly allowed key-expiry boundary was rejected: %v", fingerprints)
	}

	for name, invalid := range map[string]string{
		"pre-creation signature": strings.Replace(
			historical,
			strconv.FormatUint(signatureCreated, 10),
			strconv.FormatUint(keyCreation-1, 10),
			1,
		),
		"post-expiry signature": strings.Replace(
			historical,
			strconv.FormatUint(signatureCreated, 10),
			strconv.FormatUint(keyExpiry+1, 10),
			1,
		),
		"wrong key expiry": strings.Replace(
			historical,
			strconv.FormatUint(keyExpiry, 10),
			strconv.FormatUint(keyExpiry-1, 10),
			1,
		),
		"missing key expiry": strings.Replace(
			historical,
			"[GNUPG:] KEYEXPIRED "+strconv.FormatUint(keyExpiry, 10)+"\n",
			"",
			1,
		),
		"healthy plus expired": healthy + "\n[GNUPG:] EXPKEYSIG 34FC18AC897FB709 Takoform Provider Release",
		"weak SHA-1 hash": strings.Replace(
			healthy,
			" 1 10 00 ",
			" 1 2 00 ",
			1,
		),
		"non-binary signature class": strings.Replace(
			healthy,
			" 1 10 00 ",
			" 1 10 01 ",
			1,
		),
	} {
		if got := validSignatureFingerprints(invalid, keyCreation, keyExpiry, true); len(got) != 0 {
			t.Fatalf("%s was accepted: %v", name, got)
		}
	}
	if got := validSignatureFingerprints(historical, keyCreation, keyExpiry, false); len(got) != 0 {
		t.Fatalf("expired-key signature without retained tag-object authority was accepted: %v", got)
	}
	for _, rejectedStatus := range []string{
		"[GNUPG:] REVKEYSIG 34FC18AC897FB709 revoked",
		"[GNUPG:] KEYREVOKED",
		"[GNUPG:] BADSIG 34FC18AC897FB709 bad",
		"[GNUPG:] ERRSIG 34FC18AC897FB709 1 10 00 0 0",
		"[GNUPG:] NO_PUBKEY 34FC18AC897FB709",
		"[GNUPG:] EXPSIG 34FC18AC897FB709 expired",
		"[GNUPG:] SIGEXPIRED 1847286429",
	} {
		if got := validSignatureFingerprints(healthy+"\n"+rejectedStatus, keyCreation, keyExpiry, false); len(got) != 0 {
			t.Fatalf("unhealthy GnuPG status %q was accepted: %v", rejectedStatus, got)
		}
	}
}

func TestParsePrimaryKeyAuthorityBindsExactExpiry(t *testing.T) {
	t.Parallel()
	const listing = "pub:-:4096:1:34FC18AC897FB709:1784214429:1847286429::-:::scSC::::::23::0:\n" +
		"fpr:::::::::3510E75E05BBCC303B92D77934FC18AC897FB709:\n" +
		"uid:-::::1784214429::::::::Takoform Provider Release Signing::::::::::0:\n"
	authority, err := parsePrimaryKeyAuthority(listing)
	if err != nil {
		t.Fatalf("parse pinned primary key authority: %v", err)
	}
	if authority.fingerprint != providerSigningFingerprint ||
		authority.createdAt != 1784214429 ||
		authority.expiresAt != 1847286429 {
		t.Fatalf("primary key authority = %#v", authority)
	}
	for name, malformed := range map[string]string{
		"missing expiry":         strings.Replace(listing, ":1847286429:", "::", 1),
		"expiry before creation": strings.Replace(listing, "1847286429", "1784214428", 1),
		"duplicate primary":      listing + listing,
		"missing fingerprint": strings.Replace(
			listing,
			"fpr:::::::::3510E75E05BBCC303B92D77934FC18AC897FB709:\n",
			"",
			1,
		),
	} {
		if _, err := parsePrimaryKeyAuthority(malformed); err == nil {
			t.Fatalf("%s listing unexpectedly accepted", name)
		}
	}
}

func TestVerifyPinnedTagObjectAcceptsExactSignedTag(t *testing.T) {
	const (
		tag    = "provider/v1.0.1"
		commit = "0123456789abcdef0123456789abcdef01234567"
	)
	fixture := newSignedTagFixture(t, tag, commit)

	signer, err := verifyPinnedTagObject(
		fixture.tagObject,
		fixture.publicKey,
		fixture.fingerprint,
		tag,
		commit,
	)
	if err != nil {
		t.Fatalf("verify exact signed tag: %v", err)
	}
	if signer != fixture.fingerprint {
		t.Fatalf("signer = %q, want %q", signer, fixture.fingerprint)
	}
	historicalSigner, err := verifyPinnedTagObjectAtTime(
		fixture.tagObject,
		fixture.publicKey,
		fixture.fingerprint,
		tag,
		commit,
		fixture.keyExpiry+1,
		true,
	)
	if err != nil {
		t.Fatalf("verify pre-expiry signature after primary key expiry: %v", err)
	}
	if historicalSigner != fixture.fingerprint {
		t.Fatalf("historical signer = %q, want %q", historicalSigner, fixture.fingerprint)
	}

	for name, mismatch := range map[string]struct {
		tag    string
		commit string
	}{
		"tag": {
			tag:    "provider/v1.0.2",
			commit: commit,
		},
		"object": {
			tag:    tag,
			commit: "89abcdef0123456789abcdef0123456789abcdef",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyPinnedTagObject(
				fixture.tagObject,
				fixture.publicKey,
				fixture.fingerprint,
				mismatch.tag,
				mismatch.commit,
			); err == nil ||
				!strings.Contains(err.Error(), "does not bind") {
				t.Fatalf("mismatched signed tag header error = %v", err)
			}
		})
	}
	wrongTypeObject := bytes.Replace(fixture.tagObject, []byte("type commit\n"), []byte("type tree\n"), 1)
	if _, err := verifyPinnedTagObject(
		wrongTypeObject,
		fixture.publicKey,
		fixture.fingerprint,
		tag,
		commit,
	); err == nil ||
		!strings.Contains(err.Error(), "type header does not bind") {
		t.Fatalf("mismatched signed tag type error = %v", err)
	}
}

func TestTrustedGPGExecutableCanonicalizesSymlinkCandidate(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "gpg-real")
	writeTestFile(t, target, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(directory, "gpg")
	if err := os.Symlink(filepath.Base(target), candidate); err != nil {
		t.Skipf("create executable symlink: %v", err)
	}

	got, err := trustedGPGExecutableFromCandidates([]string{candidate})
	if err != nil {
		t.Fatalf("resolve symlinked standard candidate: %v", err)
	}
	want, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("trusted gpg executable = %q, want canonical target %q", got, want)
	}
}

func TestDirectGPGUsesBoundedAllowlistedExecution(t *testing.T) {
	t.Setenv("LD_PRELOAD", "/tmp/untrusted-loader.so")
	t.Setenv("LD_LIBRARY_PATH", "/tmp/untrusted-libraries")
	t.Setenv("DYLD_INSERT_LIBRARIES", "/tmp/untrusted-dylib")
	t.Setenv("GPG_AGENT_INFO", "/tmp/untrusted-agent")
	environment := isolatedGPGEnvironment("/tmp/isolated-gnupg")
	if len(environment) != 4 {
		t.Fatalf("isolated GPG environment has %d entries, want exact four-entry allowlist: %v", len(environment), environment)
	}
	for _, value := range environment {
		if strings.HasPrefix(value, "LD_") || strings.HasPrefix(value, "DYLD_") ||
			strings.HasPrefix(value, "GPG_") {
			t.Fatalf("isolated GPG environment retained loader or ambient GPG state: %q", value)
		}
	}
	for _, value := range isolatedGitEnvironment() {
		if strings.HasPrefix(value, "LD_") || strings.HasPrefix(value, "DYLD_") {
			t.Fatalf("isolated Git environment retained loader state: %q", value)
		}
	}

	outputter := filepath.Join(t.TempDir(), "gpg-outputter")
	writeTestFile(t, outputter, "#!/bin/sh\ndd if=/dev/zero bs=1024 count=2 2>/dev/null\n")
	if err := os.Chmod(outputter, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := directGPGWithLimits(outputter, t.TempDir(), time.Second, 64); err == nil ||
		!strings.Contains(err.Error(), "output bound") {
		t.Fatalf("unbounded direct GPG output was accepted: %v", err)
	}

	sleeper := filepath.Join(t.TempDir(), "gpg-sleeper")
	writeTestFile(t, sleeper, "#!/bin/sh\nexec sleep 5\n")
	if err := os.Chmod(sleeper, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := directGPGWithLimits(sleeper, t.TempDir(), 50*time.Millisecond, 64); err == nil ||
		!strings.Contains(err.Error(), "timeout") {
		t.Fatalf("unbounded direct GPG runtime was accepted: %v", err)
	}
}

func TestTrustedProviderPublicKeyUsesReviewedGitBlob(t *testing.T) {
	const (
		tag    = "provider/v1.0.1"
		commit = "0123456789abcdef0123456789abcdef01234567"
	)
	fixture := newSignedTagFixture(t, tag, commit)
	worktreeAuthority := append([]byte(nil), fixture.publicKey...)
	committedAuthority := append(append([]byte(nil), fixture.publicKey...), '\n')

	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	keyPath := filepath.Join(root, filepath.FromSlash(providerSigningPublicKeyPath))
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, committedAuthority, 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", providerSigningPublicKeyPath)
	runTestGit(t, root, "commit", "--quiet", "-m", "trusted authority")
	if err := os.WriteFile(keyPath, worktreeAuthority, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := trustedProviderPublicKey(root, formpackage.DigestBytes(committedAuthority))
	if err != nil {
		t.Fatalf("read reviewed Git authority while worktree is rolled back: %v", err)
	}
	if !bytes.Equal(got, committedAuthority) {
		t.Fatal("trusted provider key came from mutable worktree instead of exact HEAD blob")
	}
	if _, err := trustedProviderPublicKey(root, formpackage.DigestBytes(worktreeAuthority)); err == nil ||
		!strings.Contains(err.Error(), "reviewed digest") {
		t.Fatalf("same-fingerprint historical export rollback was accepted: %v", err)
	}

	if err := os.WriteFile(keyPath, committedAuthority, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(keyPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", providerSigningPublicKeyPath)
	runTestGit(t, root, "commit", "--quiet", "-m", "unsafe authority mode")
	if _, err := trustedProviderPublicKey(root, formpackage.DigestBytes(committedAuthority)); err == nil ||
		!strings.Contains(err.Error(), "100644") {
		t.Fatalf("executable provider key Git blob was accepted: %v", err)
	}
}

func TestReviewedProviderTagObjectUsesSourceRetainedAssignment(t *testing.T) {
	const ledger = `{"format":"takoform.provider-release-identities@v1","entries":[{"version":"1.0.1","tag":"v1.0.1","status":"assigned","tagObject":"e824793f019a941be11fde0a908fd8d1ea813ba8","commit":"44e1da0bc7e5b2581e2197ccedb107e5d9a7e9db","signingFingerprint":"3510E75E05BBCC303B92D77934FC18AC897FB709"}]}`
	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	ledgerPath := filepath.Join(root, filepath.FromSlash(providerReleaseIdentityPath))
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ledgerPath, []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "add", providerReleaseIdentityPath)
	runTestGit(t, root, "commit", "--quiet", "-m", "reviewed provider assignment")

	tagObject, err := reviewedProviderTagObject(
		root,
		"v1.0.1",
		"44e1da0bc7e5b2581e2197ccedb107e5d9a7e9db",
		providerSigningFingerprint,
	)
	if err != nil {
		t.Fatalf("load source-retained provider assignment: %v", err)
	}
	if tagObject != "e824793f019a941be11fde0a908fd8d1ea813ba8" {
		t.Fatalf("reviewed provider tag object = %q", tagObject)
	}
	if err := os.WriteFile(ledgerPath, []byte(`{"format":"substituted"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tagObject, err = reviewedProviderTagObject(
		root,
		"v1.0.1",
		"44e1da0bc7e5b2581e2197ccedb107e5d9a7e9db",
		providerSigningFingerprint,
	)
	if err != nil || tagObject != "e824793f019a941be11fde0a908fd8d1ea813ba8" {
		t.Fatalf("mutable worktree changed reviewed provider assignment: oid=%q err=%v", tagObject, err)
	}
	if _, err := reviewedProviderTagObject(
		root,
		"v1.0.1",
		strings.Repeat("f", 40),
		providerSigningFingerprint,
	); err == nil || !strings.Contains(err.Error(), "retained commit") {
		t.Fatalf("provider assignment commit substitution was accepted: %v", err)
	}
}

func TestReviewedProviderPublicKeyDigestMatchesRepositoryBytes(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(providerSigningPublicKeyPath)))
	if err != nil {
		t.Fatal(err)
	}
	if digest := formpackage.DigestBytes(publicKey); digest != providerSigningPublicKeyHash {
		t.Fatalf("provider signing public key digest = %s, want reviewed %s", digest, providerSigningPublicKeyHash)
	}
}

func TestRequireCommitAncestorIgnoresReplacementRefs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	writeTestFile(t, filepath.Join(root, "main.txt"), "main one\n")
	runTestGit(t, root, "add", "main.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "main one")
	writeTestFile(t, filepath.Join(root, "main.txt"), "main two\n")
	runTestGit(t, root, "commit", "--quiet", "-am", "main two")
	mainCommit := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))

	runTestGit(t, root, "checkout", "--quiet", "--orphan", "unrelated")
	runTestGit(t, root, "rm", "--quiet", "--force", "main.txt")
	writeTestFile(t, filepath.Join(root, "unrelated.txt"), "unrelated one\n")
	runTestGit(t, root, "add", "unrelated.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "unrelated one")
	unrelatedCommit := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	writeTestFile(t, filepath.Join(root, "unrelated.txt"), "unrelated two\n")
	runTestGit(t, root, "commit", "--quiet", "-am", "unrelated two")
	replacementCommit := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	runTestGit(t, root, "checkout", "--quiet", "main")
	runTestGit(t, root, "replace", mainCommit, replacementCommit)

	if err := requireCommitAncestor(root, "unrelated fixture", unrelatedCommit, mainCommit); err == nil ||
		!strings.Contains(err.Error(), "not an ancestor") {
		t.Fatalf("replacement ref changed authority result: %v", err)
	}
}

func TestRequireCommitAncestorRejectsRepositoryGrafts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	writeTestFile(t, filepath.Join(root, "main.txt"), "main\n")
	runTestGit(t, root, "add", "main.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "main")
	mainCommit := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))

	runTestGit(t, root, "checkout", "--quiet", "--orphan", "unrelated")
	runTestGit(t, root, "rm", "--quiet", "--force", "main.txt")
	writeTestFile(t, filepath.Join(root, "unrelated.txt"), "unrelated\n")
	runTestGit(t, root, "add", "unrelated.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "unrelated")
	unrelatedCommit := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))

	grafts := filepath.Join(root, ".git", "info", "grafts")
	writeTestFile(t, grafts, mainCommit+" "+unrelatedCommit+"\n")
	if err := requireCommitAncestor(root, "unrelated fixture", unrelatedCommit, mainCommit); err == nil ||
		!strings.Contains(err.Error(), "Git graft authority is forbidden") {
		t.Fatalf("repository graft was not rejected before ancestry verification: %v", err)
	}
}

func TestPinnedProviderTagVerificationIgnoresAmbientGPGPrograms(t *testing.T) {
	root := t.TempDir()
	runTestGit(t, root, "init", "--quiet", "--initial-branch=main")
	runTestGit(t, root, "config", "user.name", "Takoform Test")
	runTestGit(t, root, "config", "user.email", "test@takoform.invalid")
	writeTestFile(t, filepath.Join(root, "fixture.txt"), "fixture\n")
	runTestGit(t, root, "add", "fixture.txt")
	runTestGit(t, root, "commit", "--quiet", "-m", "fixture")
	commit := strings.TrimSpace(runTestGit(t, root, "rev-parse", "HEAD"))
	const tag = "v1.0.1"
	fixture := newSignedTagFixture(t, tag, commit)
	tagObject := strings.TrimSpace(runTestGitInput(
		t,
		root,
		fixture.tagObject,
		"hash-object", "-t", "tag", "-w", "--stdin",
	))
	runTestGit(t, root, "update-ref", "refs/tags/"+tag, tagObject)

	marker := filepath.Join(t.TempDir(), "fake-gpg-ran")
	fakeGPG := filepath.Join(t.TempDir(), "fake-gpg")
	writeTestFile(t, fakeGPG, "#!/bin/sh\nprintf ran > \"$FAKE_GPG_MARKER\"\nexit 99\n")
	if err := os.Chmod(fakeGPG, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, root, "config", "gpg.program", fakeGPG)
	t.Setenv("FAKE_GPG_MARKER", marker)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "gpg.program")
	t.Setenv("GIT_CONFIG_VALUE_0", fakeGPG)

	signer, err := verifyProviderTagWithAuthority(
		root,
		tag,
		commit,
		tagObject,
		fixture.publicKey,
		fixture.fingerprint,
	)
	if err != nil {
		t.Fatalf("signed provider tag did not reach isolated direct GPG: %v", err)
	}
	if signer != fixture.fingerprint {
		t.Fatalf("signer = %q, want %q", signer, fixture.fingerprint)
	}
	if _, err := verifyProviderTagWithAuthority(
		root,
		tag,
		commit,
		strings.Repeat("f", 40),
		fixture.publicKey,
		fixture.fingerprint,
	); err == nil || !strings.Contains(err.Error(), "reviewed assignment") {
		t.Fatalf("provider tag-object assignment substitution was accepted: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("ambient fake gpg program executed: %v", err)
	}
}

func TestCurrentAdmissionTagMessageBindsExactRetainedSource(t *testing.T) {
	t.Parallel()
	message := currentAdmissionTagMessage(
		"1.0.6",
		"ga-core-v2",
		"0123456789abcdef0123456789abcdef01234567",
		"89abcdef0123456789abcdef0123456789abcdef",
		"sha256:"+strings.Repeat("a", 64),
		"sha256:"+strings.Repeat("c", 64),
		"sha256:"+strings.Repeat("b", 64),
	)
	for _, required := range []string{
		"Activate Standard Form admission v1.0.6\n",
		"generation ga-core-v2\n",
		"commit 0123456789abcdef0123456789abcdef01234567\n",
		"tree 89abcdef0123456789abcdef0123456789abcdef\n",
		"version-descriptor sha256:" + strings.Repeat("a", 64) + "\n",
		"identity-ledger sha256:" + strings.Repeat("c", 64) + "\n",
		"standard-admission-set sha256:" + strings.Repeat("b", 64) + "\n",
	} {
		if !strings.Contains(message, required) {
			t.Errorf("message omits %q", required)
		}
	}
}

func runTestGit(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func runTestGitInput(t *testing.T, root string, input []byte, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

type signedTagFixture struct {
	tagObject   []byte
	publicKey   []byte
	fingerprint string
	keyExpiry   uint64
}

func newSignedTagFixture(t *testing.T, tag, commit string) signedTagFixture {
	t.Helper()
	gpg, err := trustedGPGExecutable()
	if err != nil {
		t.Fatal(err)
	}
	gpgHome := filepath.Join(t.TempDir(), "gnupg")
	if err := os.Mkdir(gpgHome, 0o700); err != nil {
		t.Fatal(err)
	}
	const identity = "Takoform Admission Fixture <admission-fixture@takoform.invalid>"
	if _, diagnostics, err := directGPG(
		gpg,
		gpgHome,
		"--batch",
		"--pinentry-mode", "loopback",
		"--passphrase", "",
		"--quick-generate-key", identity, "ed25519", "sign", "1d",
	); err != nil {
		t.Fatalf("generate ephemeral signing key: %v\n%s", err, diagnostics)
	}
	keyListing, diagnostics, err := directGPG(gpg, gpgHome, "--batch", "--with-colons", "--list-keys", identity)
	if err != nil {
		t.Fatalf("list ephemeral signing key: %v\n%s", err, diagnostics)
	}
	authority, err := parsePrimaryKeyAuthority(keyListing)
	if err != nil {
		t.Fatalf("parse ephemeral primary key authority: %v", err)
	}
	fingerprint := authority.fingerprint
	publicKey, diagnostics, err := directGPG(gpg, gpgHome, "--batch", "--armor", "--export", fingerprint)
	if err != nil {
		t.Fatalf("export ephemeral public key: %v\n%s", err, diagnostics)
	}
	payload := []byte(
		"object " + commit + "\n" +
			"type commit\n" +
			"tag " + tag + "\n" +
			"tagger Takoform Admission Fixture <admission-fixture@takoform.invalid> 0 +0000\n\n" +
			"fixture signed tag\n",
	)
	payloadPath := filepath.Join(t.TempDir(), "tag-payload")
	signaturePath := filepath.Join(t.TempDir(), "tag-signature.asc")
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, diagnostics, err := directGPG(
		gpg,
		gpgHome,
		"--batch",
		"--pinentry-mode", "loopback",
		"--passphrase", "",
		"--armor",
		"--detach-sign",
		"--output", signaturePath,
		payloadPath,
	); err != nil {
		t.Fatalf("sign annotated tag fixture: %v\n%s", err, diagnostics)
	}
	signature, err := os.ReadFile(signaturePath)
	if err != nil {
		t.Fatal(err)
	}
	return signedTagFixture{
		tagObject:   append(append([]byte(nil), payload...), signature...),
		publicKey:   []byte(publicKey),
		fingerprint: fingerprint,
		keyExpiry:   authority.expiresAt,
	}
}

func writeTestFile(t *testing.T, name, value string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(value), 0o644); err != nil {
		t.Fatal(err)
	}
}

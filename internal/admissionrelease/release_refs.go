package admissionrelease

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	providerSigningFingerprint   = "3510E75E05BBCC303B92D77934FC18AC897FB709"
	providerSigningPublicKeyPath = "release/keys/provider-signing-key.asc"
	providerSigningPublicKeyHash = "sha256:f164298b0278f8793bc7ef3b1ed64b2e041c0adcfd0afb06a7521a77ccb9392e"
	providerReleaseIdentityPath  = "release/provider-release-identities.json"
	providerReleaseIdentityFmt   = "takoform.provider-release-identities@v1"
	maxProviderPublicKeyBytes    = 64 << 10
	maxProviderIdentityBytes     = 64 << 10
	maxDirectGPGOutputBytes      = 1 << 20
	directGPGTimeout             = 15 * time.Second
)

type gitReleaseRefVerifier struct{}
type gitMaterialRefVerifier struct{}

type providerReleaseIdentityLedger struct {
	Format  string                              `json:"format"`
	Entries []providerReleaseIdentityAssignment `json:"entries"`
}

type providerReleaseIdentityAssignment struct {
	Version            string                           `json:"version"`
	Tag                string                           `json:"tag"`
	Status             string                           `json:"status"`
	TagObject          string                           `json:"tagObject"`
	Commit             string                           `json:"commit"`
	SigningFingerprint string                           `json:"signingFingerprint"`
	RegistryReadback   *providerRegistryReadbackClosure `json:"registryReadback,omitempty"`
}

type providerRegistryReadbackClosure struct {
	Format             string            `json:"format"`
	WorkflowRunID      string            `json:"workflowRunId"`
	WorkflowRunAttempt int               `json:"workflowRunAttempt"`
	SourceCommit       string            `json:"sourceCommit"`
	Files              map[string]string `json:"files"`
}

func (gitReleaseRefVerifier) VerifyReleaseRefs(root string, set Set, readback ProviderRegistryReadback) error {
	return verifyGitReleaseRefs(root, set, readback, true)
}

func (gitMaterialRefVerifier) VerifyReleaseRefs(root string, set Set, readback ProviderRegistryReadback) error {
	return verifyGitReleaseRefs(root, set, readback, false)
}

func verifyGitReleaseRefs(root string, set Set, readback ProviderRegistryReadback, requireAdmissionTag bool) error {
	head, err := resolveCommit(root, "HEAD")
	if err != nil {
		return err
	}
	if requireAdmissionTag {
		admissionCommit, err := resolveTagCommit(root, set.AdmissionReleaseTag)
		if err != nil {
			return fmt.Errorf("admission checkpoint tag: %w", err)
		}
		if admissionCommit != head {
			if err := requireCommitAncestor(root, "admission checkpoint", admissionCommit, head); err != nil {
				return err
			}
		}
	}

	if err := requireTagCommit(root, "provider release", readback.ProviderReleaseTag, readback.ProviderReleaseCommit); err != nil {
		return err
	}
	tagType, err := gitOutput(root, "cat-file", "-t", "refs/tags/"+readback.ProviderReleaseTag)
	if err != nil || strings.TrimSpace(tagType) != "tag" {
		return fmt.Errorf("provider release tag %q must be an annotated signed tag", readback.ProviderReleaseTag)
	}
	signer, err := verifyPinnedProviderTag(root, readback.ProviderReleaseTag, readback.ProviderReleaseCommit)
	if err != nil {
		return fmt.Errorf("provider release tag %q signature: %w", readback.ProviderReleaseTag, err)
	}
	if signer != providerSigningFingerprint {
		return fmt.Errorf("provider release tag %q is not signed by pinned fingerprint %s", readback.ProviderReleaseTag, providerSigningFingerprint)
	}

	for _, entry := range set.Entries {
		if err := requireTagCommit(root, entry.Kind+" package release", entry.ReleaseTag, entry.ReleaseCommit); err != nil {
			return err
		}
		if err := requireCommitAncestor(root, entry.Kind+" release tooling", entry.ReleaseToolingCommit, head); err != nil {
			return err
		}
	}
	return nil
}

func requireCommitAncestor(root, label, commit, descendant string) error {
	if !releaseCommitPattern.MatchString(commit) || !releaseCommitPattern.MatchString(descendant) {
		return fmt.Errorf("%s commit ancestry requires exact lowercase 40-hex commits", label)
	}
	if _, err := gitOutput(root, "cat-file", "-e", commit+"^{commit}"); err != nil {
		return fmt.Errorf("%s commit %s is not retained in source history: %w", label, commit, err)
	}
	if _, err := gitOutput(root, "merge-base", "--is-ancestor", commit, descendant); err != nil {
		return fmt.Errorf("%s commit %s is not an ancestor of admission commit %s", label, commit, descendant)
	}
	return nil
}

func requireTagCommit(root, label, tag, expectedCommit string) error {
	commit, err := resolveTagCommit(root, tag)
	if err != nil {
		return fmt.Errorf("%s tag: %w", label, err)
	}
	if commit != expectedCommit {
		return fmt.Errorf("%s tag %q resolves to %s, want retained commit %s", label, tag, commit, expectedCommit)
	}
	return nil
}

func verifyPinnedProviderTag(root, tag, expectedCommit string) (string, error) {
	publicKey, err := trustedProviderPublicKey(root, providerSigningPublicKeyHash)
	if err != nil {
		return "", err
	}
	expectedTagObject, err := reviewedProviderTagObject(root, tag, expectedCommit, providerSigningFingerprint)
	if err != nil {
		return "", err
	}
	return verifyProviderTagWithAuthority(
		root,
		tag,
		expectedCommit,
		expectedTagObject,
		publicKey,
		providerSigningFingerprint,
	)
}

func verifyProviderTagWithAuthority(
	root,
	tag,
	expectedCommit,
	expectedTagObject string,
	publicKey []byte,
	expectedFingerprint string,
) (string, error) {
	tagObjectRaw, err := gitOutput(root, "rev-parse", "--verify", "refs/tags/"+tag+"^{tag}")
	if err != nil {
		return "", fmt.Errorf("resolve annotated tag object: %w", err)
	}
	tagObject := strings.TrimSpace(tagObjectRaw)
	if !releaseCommitPattern.MatchString(tagObject) {
		return "", fmt.Errorf("annotated tag object is not one exact SHA-1 object")
	}
	historicalAuthorityBound := false
	if expectedTagObject != "" {
		if !releaseCommitPattern.MatchString(expectedTagObject) {
			return "", fmt.Errorf("reviewed provider tag assignment object is not one exact SHA-1 object")
		}
		if tagObject != expectedTagObject {
			return "", fmt.Errorf("annotated tag object %s does not equal reviewed assignment %s", tagObject, expectedTagObject)
		}
		historicalAuthorityBound = true
	}
	peeledCommit, err := resolveCommit(root, tagObject+"^{}")
	if err != nil {
		return "", fmt.Errorf("peel annotated tag object: %w", err)
	}
	if peeledCommit != expectedCommit {
		return "", fmt.Errorf("annotated tag object peels to %s, want retained commit %s", peeledCommit, expectedCommit)
	}
	raw, err := gitOutputBytes(root, "cat-file", "tag", tagObject)
	if err != nil {
		return "", fmt.Errorf("read annotated tag object: %w", err)
	}
	return verifyPinnedTagObjectWithHistoricalAuthority(
		raw,
		publicKey,
		expectedFingerprint,
		tag,
		peeledCommit,
		historicalAuthorityBound,
	)
}

func verifyPinnedTagObject(tagObject, publicKey []byte, expectedFingerprint, expectedTag, expectedCommit string) (string, error) {
	return verifyPinnedTagObjectWithHistoricalAuthority(tagObject, publicKey, expectedFingerprint, expectedTag, expectedCommit, false)
}

func verifyPinnedTagObjectWithHistoricalAuthority(
	tagObject,
	publicKey []byte,
	expectedFingerprint,
	expectedTag,
	expectedCommit string,
	historicalAuthorityBound bool,
) (string, error) {
	return verifyPinnedTagObjectAtTime(
		tagObject,
		publicKey,
		expectedFingerprint,
		expectedTag,
		expectedCommit,
		0,
		historicalAuthorityBound,
	)
}

func verifyPinnedTagObjectAtTime(
	tagObject []byte,
	publicKey []byte,
	expectedFingerprint,
	expectedTag,
	expectedCommit string,
	verificationTime uint64,
	historicalAuthorityBound bool,
) (string, error) {
	const signatureHeader = "-----BEGIN PGP SIGNATURE-----\n"
	marker := []byte("\n" + signatureHeader)
	position := bytes.Index(tagObject, marker)
	if position < 0 || bytes.Index(tagObject[position+len(marker):], []byte(signatureHeader)) >= 0 {
		return "", fmt.Errorf("annotated tag does not contain one exact OpenPGP signature")
	}
	payload := tagObject[:position+1]
	signature := tagObject[position+1:]
	if !bytes.HasSuffix(signature, []byte("-----END PGP SIGNATURE-----\n")) {
		return "", fmt.Errorf("annotated tag OpenPGP signature is not a complete armored block")
	}
	if err := verifyPinnedTagHeader(payload, expectedTag, expectedCommit); err != nil {
		return "", err
	}

	gpg, err := trustedGPGExecutable()
	if err != nil {
		return "", err
	}
	temporary, err := os.MkdirTemp("", "takoform-admission-tag-verify-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil {
		return "", err
	}
	gpgHome := filepath.Join(temporary, "gnupg")
	if err := os.Mkdir(gpgHome, 0o700); err != nil {
		return "", err
	}
	payloadPath := filepath.Join(temporary, "tag-payload")
	signaturePath := filepath.Join(temporary, "tag-signature.asc")
	publicKeyPath := filepath.Join(temporary, "provider-public-key.asc")
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(signaturePath, signature, 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(publicKeyPath, publicKey, 0o600); err != nil {
		return "", err
	}

	inspect, diagnostics, err := directGPG(gpg, gpgHome,
		"--batch", "--with-colons", "--import-options", "show-only", "--import", publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("inspect pinned provider public key: %w: %s", err, strings.TrimSpace(diagnostics))
	}
	authority, err := parsePrimaryKeyAuthority(inspect)
	if err != nil {
		return "", fmt.Errorf("inspect pinned provider primary key authority: %w", err)
	}
	if authority.fingerprint != expectedFingerprint {
		return "", fmt.Errorf(
			"pinned provider public key fingerprint %s does not equal %s",
			authority.fingerprint,
			expectedFingerprint,
		)
	}
	if _, diagnostics, err := directGPG(gpg, gpgHome, "--batch", "--import", publicKeyPath); err != nil {
		return "", fmt.Errorf("import pinned provider public key: %w: %s", err, strings.TrimSpace(diagnostics))
	}
	verifyArguments := []string{"--batch", "--no-auto-key-retrieve", "--status-fd", "1"}
	if verificationTime != 0 {
		verifyArguments = append(verifyArguments, "--faked-system-time", strconv.FormatUint(verificationTime, 10))
	}
	verifyArguments = append(verifyArguments, "--verify", signaturePath, payloadPath)
	status, diagnostics, err := directGPG(gpg, gpgHome, verifyArguments...)
	if err != nil {
		return "", fmt.Errorf("direct OpenPGP tag verification failed: %w: %s", err, strings.TrimSpace(diagnostics))
	}
	valid := validSignatureFingerprints(
		status,
		authority.createdAt,
		authority.expiresAt,
		historicalAuthorityBound,
	)
	if len(valid) != 1 {
		return "", fmt.Errorf("direct OpenPGP tag verification returned %d exact VALIDSIG fingerprints", len(valid))
	}
	if valid[0] != expectedFingerprint {
		return valid[0], fmt.Errorf("provider tag signer %s does not match pinned signer %s", valid[0], expectedFingerprint)
	}
	return valid[0], nil
}

func verifyPinnedTagHeader(payload []byte, expectedTag, expectedCommit string) error {
	if strings.ContainsAny(expectedTag, "\r\n") {
		return fmt.Errorf("expected provider tag contains a line break")
	}
	if !releaseCommitPattern.MatchString(expectedCommit) {
		return fmt.Errorf("expected provider commit must be one exact lowercase SHA-1 object")
	}
	headerEnd := bytes.Index(payload, []byte("\n\n"))
	if headerEnd < 0 {
		return fmt.Errorf("annotated tag is missing the header separator")
	}
	lines := bytes.Split(payload[:headerEnd], []byte("\n"))
	expected := [][]byte{
		[]byte("object " + expectedCommit),
		[]byte("type commit"),
		[]byte("tag " + expectedTag),
	}
	if len(lines) < len(expected) {
		return fmt.Errorf("annotated tag is missing the object/type/tag header")
	}
	for index, line := range expected {
		if !bytes.Equal(lines[index], line) {
			return fmt.Errorf("annotated tag %s header does not bind the retained provider release", strings.SplitN(string(line), " ", 2)[0])
		}
	}
	for _, line := range lines[len(expected):] {
		field, _, _ := bytes.Cut(line, []byte(" "))
		if bytes.Equal(field, []byte("object")) || bytes.Equal(field, []byte("type")) || bytes.Equal(field, []byte("tag")) {
			return fmt.Errorf("annotated tag contains a duplicate %s header", field)
		}
	}
	return nil
}

func trustedGPGExecutable() (string, error) {
	return trustedGPGExecutableFromCandidates([]string{
		"/usr/bin/gpg",
		"/usr/local/bin/gpg",
		"/opt/homebrew/bin/gpg",
	})
}

func trustedGPGExecutableFromCandidates(candidates []string) (string, error) {
	for _, candidate := range candidates {
		if !filepath.IsAbs(candidate) {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil || !filepath.IsAbs(resolved) {
			continue
		}
		resolved = filepath.Clean(resolved)
		info, err := os.Lstat(resolved)
		if err != nil || info.Mode()&os.ModeSymlink != 0 ||
			!info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			continue
		}
		return resolved, nil
	}
	return "", fmt.Errorf("no fixed absolute gpg executable is available")
}

func directGPG(executable, gpgHome string, arguments ...string) (string, string, error) {
	return directGPGWithLimits(executable, gpgHome, directGPGTimeout, maxDirectGPGOutputBytes, arguments...)
}

func directGPGWithLimits(
	executable,
	gpgHome string,
	timeout time.Duration,
	outputLimit int64,
	arguments ...string,
) (string, string, error) {
	if timeout <= 0 || outputLimit <= 0 {
		return "", "", fmt.Errorf("direct GPG limits must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, executable, append([]string{"--no-options", "--no-tty", "--homedir", gpgHome}, arguments...)...)
	command.Env = isolatedGPGEnvironment(gpgHome)
	stdout := boundedOutput{remaining: outputLimit}
	stderr := boundedOutput{remaining: outputLimit}
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return stdout.buffer.String(), stderr.buffer.String(), fmt.Errorf("direct GPG exceeded %s timeout", timeout)
	}
	if stdout.overflow || stderr.overflow {
		return stdout.buffer.String(), stderr.buffer.String(), fmt.Errorf("direct GPG exceeded %d-byte output bound", outputLimit)
	}
	return stdout.buffer.String(), stderr.buffer.String(), err
}

type primaryKeyAuthority struct {
	fingerprint string
	createdAt   uint64
	expiresAt   uint64
}

func parsePrimaryKeyAuthority(colonListing string) (primaryKeyAuthority, error) {
	var authority primaryKeyAuthority
	primaryRecords := 0
	awaitingFingerprint := false
	for _, line := range strings.Split(colonListing, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "pub":
			primaryRecords++
			if primaryRecords != 1 || awaitingFingerprint || len(fields) <= 6 {
				return primaryKeyAuthority{}, fmt.Errorf("public key does not contain one exact primary pub record")
			}
			createdAt, err := strconv.ParseUint(fields[5], 10, 64)
			if err != nil || createdAt == 0 {
				return primaryKeyAuthority{}, fmt.Errorf("primary pub creation timestamp is not one positive Unix timestamp")
			}
			expiresAt, err := strconv.ParseUint(fields[6], 10, 64)
			if err != nil || expiresAt <= createdAt {
				return primaryKeyAuthority{}, fmt.Errorf("primary pub expiry is not one Unix timestamp after creation")
			}
			authority.createdAt = createdAt
			authority.expiresAt = expiresAt
			awaitingFingerprint = true
		case "fpr":
			if !awaitingFingerprint {
				continue
			}
			if len(fields) <= 9 {
				return primaryKeyAuthority{}, fmt.Errorf("primary pub fingerprint record is malformed")
			}
			fingerprint, ok := canonicalHex(fields[9], 40)
			if !ok {
				return primaryKeyAuthority{}, fmt.Errorf("primary pub fingerprint is not one exact 40-hex value")
			}
			authority.fingerprint = fingerprint
			awaitingFingerprint = false
		case "uid", "sub":
			if awaitingFingerprint {
				return primaryKeyAuthority{}, fmt.Errorf("primary pub fingerprint does not immediately follow its pub record")
			}
		}
	}
	if primaryRecords != 1 || awaitingFingerprint || authority.fingerprint == "" {
		return primaryKeyAuthority{}, fmt.Errorf("public key does not bind one exact primary pub fingerprint")
	}
	return authority, nil
}

func validSignatureFingerprints(
	status string,
	primaryKeyCreation,
	primaryKeyExpiry uint64,
	historicalAuthorityBound bool,
) []string {
	fingerprints := make([]string, 0, 1)
	goodSignatureKeyIDs := make([]string, 0, 1)
	expiredSignatureKeyIDs := make([]string, 0, 1)
	expiredKeyTimestamps := make([]uint64, 0, 1)
	var signatureCreation uint64
	rejected := false
	for _, line := range strings.Split(status, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "[GNUPG:]" {
			continue
		}
		switch fields[1] {
		case "GOODSIG":
			keyID, ok := statusKeyID(fields)
			if !ok {
				rejected = true
				continue
			}
			goodSignatureKeyIDs = append(goodSignatureKeyIDs, keyID)
		case "EXPKEYSIG":
			keyID, ok := statusKeyID(fields)
			if !ok {
				rejected = true
				continue
			}
			expiredSignatureKeyIDs = append(expiredSignatureKeyIDs, keyID)
		case "KEYEXPIRED":
			if len(fields) != 3 {
				rejected = true
				continue
			}
			expiry, err := strconv.ParseUint(fields[2], 10, 64)
			if err != nil || expiry == 0 {
				rejected = true
				continue
			}
			expiredKeyTimestamps = append(expiredKeyTimestamps, expiry)
		case "VALIDSIG":
			if len(fields) < 11 {
				rejected = true
				continue
			}
			fingerprint, ok := canonicalHex(fields[2], 40)
			createdAt, err := strconv.ParseUint(fields[4], 10, 64)
			strongHash := fields[9] == "8" || fields[9] == "9" || fields[9] == "10"
			if !ok || err != nil || createdAt == 0 || !strongHash || fields[10] != "00" {
				rejected = true
				continue
			}
			fingerprints = append(fingerprints, fingerprint)
			signatureCreation = createdAt
		case "REVKEYSIG", "KEYREVOKED", "BADSIG", "ERRSIG", "NO_PUBKEY", "EXPSIG", "SIGEXPIRED":
			rejected = true
		}
	}
	if rejected || len(fingerprints) != 1 || primaryKeyCreation == 0 ||
		primaryKeyExpiry <= primaryKeyCreation ||
		signatureCreation < primaryKeyCreation ||
		signatureCreation > primaryKeyExpiry {
		return nil
	}
	signerKeyID := fingerprints[0][len(fingerprints[0])-16:]
	healthy := len(goodSignatureKeyIDs) == 1 &&
		goodSignatureKeyIDs[0] == signerKeyID &&
		len(expiredSignatureKeyIDs) == 0 &&
		len(expiredKeyTimestamps) == 0
	historical := len(goodSignatureKeyIDs) == 0 &&
		historicalAuthorityBound &&
		len(expiredSignatureKeyIDs) == 1 &&
		expiredSignatureKeyIDs[0] == signerKeyID &&
		len(expiredKeyTimestamps) == 1 &&
		expiredKeyTimestamps[0] == primaryKeyExpiry
	if !healthy && !historical {
		return nil
	}
	return fingerprints
}

func statusKeyID(fields []string) (string, bool) {
	if len(fields) < 3 {
		return "", false
	}
	return canonicalHex(fields[2], 16)
}

func canonicalHex(value string, length int) (string, bool) {
	if len(value) != length {
		return "", false
	}
	canonical := strings.ToUpper(value)
	for _, character := range canonical {
		if (character < '0' || character > '9') && (character < 'A' || character > 'F') {
			return "", false
		}
	}
	return canonical, true
}

func trustedProviderPublicKey(root, expectedDigest string) ([]byte, error) {
	publicKey, err := trustedGitRegularBlob(
		root,
		providerSigningPublicKeyPath,
		"pinned provider public key",
		maxProviderPublicKeyBytes,
	)
	if err != nil {
		return nil, err
	}
	digest := formpackage.DigestBytes(publicKey)
	if digest != expectedDigest {
		return nil, fmt.Errorf("pinned provider public key digest %s does not equal reviewed digest %s", digest, expectedDigest)
	}
	return publicKey, nil
}

func reviewedProviderTagObject(root, tag, expectedCommit, expectedFingerprint string) (string, error) {
	raw, err := trustedGitRegularBlob(
		root,
		providerReleaseIdentityPath,
		"provider release identity ledger",
		maxProviderIdentityBytes,
	)
	if err != nil {
		return "", err
	}
	var ledger providerReleaseIdentityLedger
	if err := decodeStrictJSON(raw, &ledger); err != nil {
		return "", fmt.Errorf("decode provider release identity ledger: %w", err)
	}
	if _, err := formpackage.Canonicalize(raw); err != nil {
		return "", fmt.Errorf("provider release identity ledger is not strict I-JSON: %w", err)
	}
	if ledger.Format != providerReleaseIdentityFmt || len(ledger.Entries) == 0 {
		return "", fmt.Errorf("provider release identity ledger has invalid format or empty assignments")
	}
	seenTags := make(map[string]struct{}, len(ledger.Entries))
	seenObjects := make(map[string]struct{}, len(ledger.Entries))
	for _, assignment := range ledger.Entries {
		fingerprint, fingerprintOK := canonicalHex(assignment.SigningFingerprint, 40)
		if !stableProviderVersion(assignment.Version) ||
			assignment.Tag != "v"+assignment.Version ||
			assignment.Status != "assigned" ||
			!releaseCommitPattern.MatchString(assignment.TagObject) ||
			!releaseCommitPattern.MatchString(assignment.Commit) ||
			!fingerprintOK || fingerprint != assignment.SigningFingerprint {
			return "", fmt.Errorf("provider release identity ledger contains an invalid assignment")
		}
		major, err := strconv.Atoi(strings.Split(assignment.Version, ".")[0])
		if err != nil || (major >= 2 && assignment.RegistryReadback == nil) ||
			(assignment.RegistryReadback != nil && validateProviderRegistryReadbackClosure(*assignment.RegistryReadback) != nil) {
			return "", fmt.Errorf("provider release identity ledger contains an invalid retained Registry readback")
		}
		if _, duplicate := seenTags[assignment.Tag]; duplicate {
			return "", fmt.Errorf("provider release identity ledger duplicates tag %q", assignment.Tag)
		}
		if _, duplicate := seenObjects[assignment.TagObject]; duplicate {
			return "", fmt.Errorf("provider release identity ledger duplicates tag object %q", assignment.TagObject)
		}
		seenTags[assignment.Tag] = struct{}{}
		seenObjects[assignment.TagObject] = struct{}{}
		if assignment.Tag != tag {
			continue
		}
		if assignment.Commit != expectedCommit || assignment.SigningFingerprint != expectedFingerprint {
			return "", fmt.Errorf("provider release identity assignment does not bind retained commit and signer")
		}
		return assignment.TagObject, nil
	}
	return "", nil
}

func validateProviderRegistryReadbackClosure(closure providerRegistryReadbackClosure) error {
	if closure.Format != "takoform.retained-provider-registry-readback@v1" ||
		closure.WorkflowRunAttempt < 1 || !releaseCommitPattern.MatchString(closure.SourceCommit) {
		return fmt.Errorf("invalid Registry readback envelope")
	}
	if runID, err := strconv.ParseUint(closure.WorkflowRunID, 10, 64); err != nil || runID == 0 {
		return fmt.Errorf("invalid Registry readback workflow run")
	}
	expected := map[string]struct{}{
		"SHA256SUMS": {}, "provider-lifecycle-matrix.json": {}, "provider-readback.json": {},
		"provider-readback.sigstore.json": {}, "provider-registry-readback-manifest.json": {},
		"signed-provider-registry-readback-candidate.json": {},
	}
	if len(closure.Files) != len(expected) {
		return fmt.Errorf("invalid Registry readback file closure")
	}
	for name := range expected {
		encoded, ok := closure.Files[name]
		if !ok || encoded == "" {
			return fmt.Errorf("Registry readback omits %s", name)
		}
		raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || len(raw) == 0 || base64.StdEncoding.EncodeToString(raw) != encoded {
			return fmt.Errorf("Registry readback %s is not canonical base64", name)
		}
	}
	return nil
}

func stableProviderVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		value, err := strconv.ParseUint(part, 10, 64)
		if err != nil || strconv.FormatUint(value, 10) != part {
			return false
		}
	}
	return true
}

func trustedGitRegularBlob(root, sourcePath, label string, maximum int64) ([]byte, error) {
	entry, err := gitOutputBytes(root, "ls-tree", "-z", "HEAD", "--", sourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve %s blob: %w", label, err)
	}
	if len(entry) == 0 || entry[len(entry)-1] != 0 || bytes.Count(entry, []byte{0}) != 1 {
		return nil, fmt.Errorf("%s must resolve to one exact Git tree entry", label)
	}
	metadata, path, ok := bytes.Cut(entry[:len(entry)-1], []byte{'\t'})
	if !ok || string(path) != sourcePath {
		return nil, fmt.Errorf("%s Git tree path is not exact", label)
	}
	fields := strings.Fields(string(metadata))
	if len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" ||
		!releaseCommitPattern.MatchString(fields[2]) {
		return nil, fmt.Errorf("%s must be one exact 100644 Git blob", label)
	}
	sizeRaw, err := gitOutput(root, "cat-file", "-s", fields[2])
	if err != nil {
		return nil, fmt.Errorf("resolve %s blob size: %w", label, err)
	}
	size, err := strconv.ParseInt(strings.TrimSpace(sizeRaw), 10, 64)
	if err != nil || size <= 0 || size > maximum {
		return nil, fmt.Errorf("%s blob size is outside the allowed bound", label)
	}
	raw, err := gitOutputBytesLimited(root, size, "cat-file", "blob", fields[2])
	if err != nil {
		return nil, fmt.Errorf("read %s blob: %w", label, err)
	}
	if int64(len(raw)) != size {
		return nil, fmt.Errorf("%s blob length differs from Git object size", label)
	}
	return raw, nil
}

func resolveTagCommit(root, tag string) (string, error) {
	return resolveCommit(root, "refs/tags/"+tag)
}

func resolveCommit(root, ref string) (string, error) {
	output, err := gitOutput(root, "rev-list", "-n", "1", ref)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", ref, err)
	}
	commit := strings.TrimSpace(output)
	if !releaseCommitPattern.MatchString(commit) {
		return "", fmt.Errorf("resolve %q returned invalid commit %q", ref, commit)
	}
	return commit, nil
}

func gitOutput(root string, arguments ...string) (string, error) {
	output, err := gitOutputBytes(root, arguments...)
	return string(output), err
}

func gitOutputBytes(root string, arguments ...string) ([]byte, error) {
	executable, err := trustedGitExecutable()
	if err != nil {
		return nil, err
	}
	if err := rejectGitGrafts(executable, root); err != nil {
		return nil, err
	}
	return runIsolatedGit(executable, root, arguments...)
}

func gitOutputBytesLimited(root string, limit int64, arguments ...string) ([]byte, error) {
	executable, err := trustedGitExecutable()
	if err != nil {
		return nil, err
	}
	if err := rejectGitGrafts(executable, root); err != nil {
		return nil, err
	}
	return runIsolatedGitLimited(executable, root, limit, arguments...)
}

func rejectGitGrafts(executable, root string) error {
	output, err := runIsolatedGit(
		executable,
		root,
		"rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir",
	)
	if err != nil {
		return fmt.Errorf("resolve isolated Git authority directories: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != 2 || !filepath.IsAbs(lines[0]) || !filepath.IsAbs(lines[1]) {
		return fmt.Errorf("resolve isolated Git authority directories returned an ambiguous result")
	}
	seen := make(map[string]struct{}, len(lines))
	for _, directory := range lines {
		grafts := filepath.Clean(filepath.Join(directory, "info", "grafts"))
		if _, duplicate := seen[grafts]; duplicate {
			continue
		}
		seen[grafts] = struct{}{}
		if _, err := os.Lstat(grafts); err == nil {
			return fmt.Errorf("repository-local Git graft authority is forbidden: %s", grafts)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect repository-local Git graft authority %s: %w", grafts, err)
		}
	}
	return nil
}

func runIsolatedGit(executable, root string, arguments ...string) ([]byte, error) {
	command := isolatedGitCommand(executable, root, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func runIsolatedGitLimited(executable, root string, limit int64, arguments ...string) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("git output limit must not be negative")
	}
	command := isolatedGitCommand(executable, root, arguments...)
	stdout := boundedOutput{remaining: limit}
	stderr := boundedOutput{remaining: 32 << 10}
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), err, strings.TrimSpace(stderr.buffer.String()))
	}
	if stdout.overflow {
		return nil, fmt.Errorf("git %s exceeded the exact output bound", strings.Join(arguments, " "))
	}
	return stdout.buffer.Bytes(), nil
}

func isolatedGitCommand(executable, root string, arguments ...string) *exec.Cmd {
	prefix := []string{
		"--no-replace-objects",
		"-c", "advice.graftFileDeprecated=false",
		"-c", "core.fsmonitor=false",
		"-c", "credential.helper=",
		"-c", "core.hooksPath=/dev/null",
		"-C", root,
	}
	command := exec.Command(executable, append(prefix, arguments...)...)
	command.Env = isolatedGitEnvironment()
	return command
}

type boundedOutput struct {
	buffer    bytes.Buffer
	remaining int64
	overflow  bool
}

func (output *boundedOutput) Write(value []byte) (int, error) {
	originalLength := len(value)
	if int64(len(value)) > output.remaining {
		value = value[:output.remaining]
		output.overflow = true
	}
	if len(value) != 0 {
		_, _ = output.buffer.Write(value)
		output.remaining -= int64(len(value))
	}
	return originalLength, nil
}

func trustedGitExecutable() (string, error) {
	for _, candidate := range []string{
		"/usr/bin/git",
		"/usr/local/bin/git",
		"/opt/homebrew/bin/git",
	} {
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 ||
			!info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("no fixed absolute git executable is available")
}

func isolatedGitEnvironment() []string {
	return []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_GRAFT_FILE=/dev/null",
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
		"LANG=C",
		"PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin",
	}
}

func isolatedGPGEnvironment(gpgHome string) []string {
	return []string{
		"GNUPGHOME=" + gpgHome,
		"LC_ALL=C",
		"LANG=C",
		"PATH=/usr/bin:/bin:/usr/local/bin:/opt/homebrew/bin",
	}
}

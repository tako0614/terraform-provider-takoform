package formpackage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	maxIndexBytes       = 4 << 20
	maxDefinitionBytes  = 4 << 20
	maxJSONPayloadBytes = 16 << 20
	maxPayloadBytes     = 64 << 20
	maxPackageBytes     = 256 << 20
	maxPackageFiles     = 1024
)

var executableExtensions = map[string]struct{}{
	".bat": {}, ".bin": {}, ".c": {}, ".cc": {}, ".class": {}, ".cmd": {},
	".com": {}, ".cpp": {}, ".cs": {}, ".cxx": {}, ".dll": {}, ".dylib": {},
	".exe": {}, ".go": {}, ".groovy": {}, ".h": {}, ".hcl": {}, ".hpp": {},
	".htm": {}, ".html": {}, ".jar": {}, ".java": {}, ".js": {}, ".jsx": {},
	".kt": {}, ".kts": {}, ".lua": {}, ".mjs": {}, ".php": {}, ".pl": {},
	".ps1": {}, ".py": {}, ".rb": {}, ".rs": {}, ".scala": {}, ".sh": {},
	".so": {}, ".sql": {}, ".svelte": {}, ".swift": {}, ".tf": {}, ".ts": {},
	".tsx": {}, ".vue": {}, ".wasm": {},
}

// VerifyDirectory verifies a complete local Form Package without network I/O
// or code execution. The directory must contain package-index.json plus exactly
// the regular, non-executable payload files listed by that index. Supported Unix
// systems open payloads relative to a held root descriptor without following
// symlinks. Other systems require an immutable staging directory while this
// function runs; metadata fences are defense in depth, not an atomic snapshot.
func VerifyDirectory(root string) (VerificationReport, error) {
	rootHandle, rootInfo, err := openStablePackageRoot(root)
	if err != nil {
		return VerificationReport{}, err
	}
	defer rootHandle.Close()

	actualFiles, err := inventoryDirectory(root)
	if err != nil {
		return VerificationReport{}, err
	}
	if err := assertPackageRootStable(root, rootHandle, rootInfo); err != nil {
		return VerificationReport{}, err
	}
	indexRaw, err := readBoundedRegularFile(rootHandle, root, PackageIndexFilename, maxIndexBytes, actualFiles[PackageIndexFilename])
	if err != nil {
		return VerificationReport{}, fmt.Errorf("read %s: %w", PackageIndexFilename, err)
	}
	index, _, err := validateIndex(indexRaw)
	if err != nil {
		return VerificationReport{}, err
	}
	if len(index.Files) > maxPackageFiles {
		return VerificationReport{}, fmt.Errorf("package lists %d files; maximum is %d", len(index.Files), maxPackageFiles)
	}
	if index.APIVersion != PackageAPIVersion || index.Kind != PackageKind {
		return VerificationReport{}, fmt.Errorf("unsupported package identity %s/%s", index.APIVersion, index.Kind)
	}

	listed := make(map[string]PackageFile, len(index.Files))
	ordered := make([]string, 0, len(index.Files))
	var payloadBytes int64
	definitionFileCount := 0
	for position, file := range index.Files {
		if err := validatePackagePath(file.Path); err != nil {
			return VerificationReport{}, fmt.Errorf("files[%d].path: %w", position, err)
		}
		if file.Path == PackageIndexFilename {
			return VerificationReport{}, fmt.Errorf("package index is the signed subject and must not list itself as a payload")
		}
		if _, duplicate := listed[file.Path]; duplicate {
			return VerificationReport{}, fmt.Errorf("duplicate payload path %q", file.Path)
		}
		if len(ordered) > 0 && ordered[len(ordered)-1] >= file.Path {
			return VerificationReport{}, fmt.Errorf("payload paths must be unique and sorted lexicographically: %q follows %q", file.Path, ordered[len(ordered)-1])
		}
		if !ValidDigest(file.Digest) {
			return VerificationReport{}, fmt.Errorf("payload %q has non-canonical digest", file.Path)
		}
		if file.Size < 0 || file.Size > maxPayloadBytes {
			return VerificationReport{}, fmt.Errorf("payload %q size is outside 0..%d bytes", file.Path, maxPayloadBytes)
		}
		if file.MediaType == DefinitionMediaType && file.Size > maxDefinitionBytes {
			return VerificationReport{}, fmt.Errorf("Form Definition payload %q exceeds %d bytes", file.Path, maxDefinitionBytes)
		}
		if isJSONMediaType(file.MediaType) && file.Size > maxJSONPayloadBytes {
			return VerificationReport{}, fmt.Errorf("JSON payload %q exceeds %d bytes", file.Path, maxJSONPayloadBytes)
		}
		if err := validateMediaType(file.Path, file.MediaType); err != nil {
			return VerificationReport{}, err
		}
		if file.MediaType == DefinitionMediaType {
			definitionFileCount++
		}
		listed[file.Path] = file
		ordered = append(ordered, file.Path)
		if payloadBytes > maxPackageBytes-file.Size {
			return VerificationReport{}, fmt.Errorf("package payload total exceeds %d bytes", maxPackageBytes)
		}
		payloadBytes += file.Size
	}
	if definitionFileCount != 1 {
		return VerificationReport{}, fmt.Errorf("one Form Package must contain exactly one Form Definition payload; found %d", definitionFileCount)
	}

	wantActual := make([]string, 0, len(actualFiles)-1)
	for file := range actualFiles {
		if file != PackageIndexFilename {
			wantActual = append(wantActual, file)
		}
	}
	sort.Strings(wantActual)
	if !equalStrings(ordered, wantActual) {
		return VerificationReport{}, fmt.Errorf("package file closure mismatch: listed=%v actual=%v", ordered, wantActual)
	}

	payloads := make(map[string][]byte, len(index.Files))
	for _, file := range index.Files {
		limit := int64(maxPayloadBytes)
		if isJSONMediaType(file.MediaType) {
			limit = maxJSONPayloadBytes
		}
		if file.MediaType == DefinitionMediaType {
			limit = maxDefinitionBytes
		}
		raw, err := readBoundedRegularFile(rootHandle, root, file.Path, limit, actualFiles[file.Path])
		if err != nil {
			return VerificationReport{}, fmt.Errorf("read payload %q: %w", file.Path, err)
		}
		if int64(len(raw)) != file.Size {
			return VerificationReport{}, fmt.Errorf("payload %q size is %d, index says %d", file.Path, len(raw), file.Size)
		}
		if digest := DigestBytes(raw); digest != file.Digest {
			return VerificationReport{}, fmt.Errorf("payload %q digest is %s, index says %s", file.Path, digest, file.Digest)
		}
		if isJSONMediaType(file.MediaType) {
			if _, err := Canonicalize(raw); err != nil {
				return VerificationReport{}, fmt.Errorf("JSON payload %q: %w", file.Path, err)
			}
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				return VerificationReport{}, fmt.Errorf("JSON payload %q: %w", file.Path, err)
			}
			if err := rejectForbiddenContent(value, file.Path); err != nil {
				return VerificationReport{}, fmt.Errorf("payload content policy: %w", err)
			}
		} else if !utf8.Valid(raw) || bytes.IndexByte(raw, 0) >= 0 {
			return VerificationReport{}, fmt.Errorf("text payload %q must be valid UTF-8 without NUL bytes", file.Path)
		}
		payloads[file.Path] = raw
	}

	definitionFile, ok := listed[index.DefinitionPath]
	if !ok {
		return VerificationReport{}, fmt.Errorf("definitionPath %q is not listed", index.DefinitionPath)
	}
	if definitionFile.MediaType != DefinitionMediaType {
		return VerificationReport{}, fmt.Errorf("definitionPath %q has media type %q, want %q", index.DefinitionPath, definitionFile.MediaType, DefinitionMediaType)
	}
	definitionRaw := payloads[index.DefinitionPath]
	definition, _, err := validateDefinition(definitionRaw)
	if err != nil {
		return VerificationReport{}, err
	}
	definitionDigest, err := DigestCanonicalJSON(definitionRaw)
	if err != nil {
		return VerificationReport{}, err
	}
	if definitionDigest != index.FormRef.SchemaDigest {
		return VerificationReport{}, fmt.Errorf("FormRef schemaDigest is %s, canonical definition digest is %s", index.FormRef.SchemaDigest, definitionDigest)
	}
	if definition.APIVersion != index.FormRef.APIVersion || definition.Kind != index.FormRef.Kind || definition.DefinitionVersion != index.FormRef.DefinitionVersion {
		return VerificationReport{}, fmt.Errorf("FormRef identity does not match Form Definition identity")
	}
	formRefRaw, err := json.Marshal(index.FormRef)
	if err != nil {
		return VerificationReport{}, fmt.Errorf("encode FormRef for validation: %w", err)
	}
	if _, err := validateFormRef(formRefRaw); err != nil {
		return VerificationReport{}, err
	}
	for _, fixture := range definition.ConformanceFixtures {
		for _, fixturePath := range []string{fixture.DesiredPath, fixture.ObservedPath} {
			if fixturePath == "" {
				continue
			}
			fixtureFile, ok := listed[fixturePath]
			if !ok {
				return VerificationReport{}, fmt.Errorf("conformance fixture %q references unlisted payload %q", fixture.Name, fixturePath)
			}
			if fixtureFile.MediaType != "application/json" {
				return VerificationReport{}, fmt.Errorf("conformance fixture %q payload %q must use application/json", fixture.Name, fixturePath)
			}
		}
		desiredSchema, err := compileInlineSchema(definition.DesiredSchema, "desiredSchema")
		if err != nil {
			return VerificationReport{}, err
		}
		if err := validateFixtureAgainstSchema(desiredSchema, payloads[fixture.DesiredPath], fixture.Name+" desired"); err != nil {
			return VerificationReport{}, err
		}
		if fixture.ObservedPath != "" {
			observedSchema, err := compileInlineSchema(definition.ObservedSchema, "observedSchema")
			if err != nil {
				return VerificationReport{}, err
			}
			if err := validateFixtureAgainstSchema(observedSchema, payloads[fixture.ObservedPath], fixture.Name+" observed"); err != nil {
				return VerificationReport{}, err
			}
		}
	}
	packageDigest, err := DigestCanonicalJSON(indexRaw)
	if err != nil {
		return VerificationReport{}, err
	}
	if err := assertPackageRootStable(root, rootHandle, rootInfo); err != nil {
		return VerificationReport{}, err
	}
	return VerificationReport{PackageDigest: packageDigest, FormRef: index.FormRef, FileCount: len(index.Files), PayloadBytes: payloadBytes}, nil
}

func isJSONMediaType(mediaType string) bool {
	return strings.HasSuffix(mediaType, "+json") || mediaType == "application/json" || mediaType == "application/schema+json"
}

func validateFixtureAgainstSchema(schema *jsonschema.Schema, raw []byte, label string) error {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("conformance fixture %s is invalid JSON: %w", label, err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("conformance fixture %s does not satisfy its Form Definition schema: %w", label, err)
	}
	return nil
}

func inventoryDirectory(root string) (map[string]fs.FileInfo, error) {
	files := map[string]fs.FileInfo{}
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if current == root {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("package entry %q is a symlink", relative)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("package entry %q is not a regular file (devices, sockets, and pipes are forbidden)", relative)
		}
		if info.Mode().Perm()&0o111 != 0 {
			return fmt.Errorf("package entry %q is executable", relative)
		}
		if _, forbidden := executableExtensions[strings.ToLower(filepath.Ext(relative))]; forbidden {
			return fmt.Errorf("package entry %q has executable-code extension", relative)
		}
		if err := validatePackagePath(relative); err != nil {
			return fmt.Errorf("package entry %q: %w", relative, err)
		}
		files[relative] = info
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory package: %w", err)
	}
	if _, ok := files[PackageIndexFilename]; !ok {
		return nil, fmt.Errorf("package has no root %s", PackageIndexFilename)
	}
	return files, nil
}

func validatePackagePath(value string) error {
	if value == "" || len(value) > 240 {
		return fmt.Errorf("path must contain 1..240 bytes")
	}
	if strings.Contains(value, "\\") {
		return fmt.Errorf("backslash paths are forbidden")
	}
	if strings.HasPrefix(value, "/") || filepath.IsAbs(value) || filepath.VolumeName(value) != "" {
		return fmt.Errorf("absolute paths are forbidden")
	}
	if strings.Contains(value, ":") {
		return fmt.Errorf("volume or URI-like paths are forbidden")
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path traversal and non-canonical paths are forbidden")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("empty, dot, and parent segments are forbidden")
		}
	}
	return nil
}

func validateMediaType(filePath, mediaType string) error {
	extension := strings.ToLower(path.Ext(filePath))
	if _, forbidden := executableExtensions[extension]; forbidden {
		return fmt.Errorf("payload %q has executable-code extension", filePath)
	}
	switch mediaType {
	case DefinitionMediaType, "application/schema+json", "application/json":
		if extension != ".json" {
			return fmt.Errorf("JSON payload %q must use .json extension", filePath)
		}
	case "text/markdown":
		if extension != ".md" && extension != ".markdown" {
			return fmt.Errorf("Markdown payload %q must use .md or .markdown extension", filePath)
		}
	case "text/plain":
		if extension != ".txt" {
			return fmt.Errorf("plain-text payload %q must use .txt extension", filePath)
		}
	default:
		return fmt.Errorf("payload %q uses unsupported media type %q", filePath, mediaType)
	}
	return nil
}

func readBoundedRegularFile(rootHandle *os.File, root, relative string, maximum int64, inventoried fs.FileInfo) ([]byte, error) {
	if inventoried == nil {
		return nil, fmt.Errorf("file was not present in the package inventory")
	}
	if err := validatePackagePath(relative); err != nil {
		return nil, err
	}
	filePath := filepath.Join(root, filepath.FromSlash(relative))
	before, err := os.Lstat(filePath)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("not a regular non-link file")
	}
	if !os.SameFile(inventoried, before) {
		return nil, fmt.Errorf("file identity changed after package inventory")
	}
	if before.Mode().Perm()&0o111 != 0 {
		return nil, fmt.Errorf("executable file")
	}
	if before.Size() > maximum {
		return nil, fmt.Errorf("file is %d bytes; maximum is %d", before.Size(), maximum)
	}
	handle, err := secureOpenRelative(rootHandle, root, relative)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	opened, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || opened.Mode().Perm()&0o111 != 0 ||
		!os.SameFile(inventoried, opened) || !sameStableMetadata(before, opened) {
		return nil, fmt.Errorf("file identity or metadata changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(handle, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes while reading", maximum)
	}
	afterHandle, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	afterPath, err := os.Lstat(filePath)
	if err != nil {
		return nil, err
	}
	if afterPath.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, afterPath) ||
		!sameStableMetadata(opened, afterHandle) || !sameStableMetadata(opened, afterPath) {
		return nil, fmt.Errorf("file identity or metadata changed while reading")
	}
	return raw, nil
}

func openStablePackageRoot(root string) (*os.File, fs.FileInfo, error) {
	before, err := os.Lstat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("inspect package root: %w", err)
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("package root must be a real directory")
	}
	handle, err := secureOpenPackageRoot(root)
	if err != nil {
		return nil, nil, fmt.Errorf("open package root: %w", err)
	}
	opened, err := handle.Stat()
	if err != nil {
		handle.Close()
		return nil, nil, fmt.Errorf("inspect opened package root: %w", err)
	}
	if !opened.IsDir() || !os.SameFile(before, opened) || !sameStableMetadata(before, opened) {
		handle.Close()
		return nil, nil, fmt.Errorf("package root identity or metadata changed while opening")
	}
	return handle, opened, nil
}

func assertPackageRootStable(root string, handle *os.File, expected fs.FileInfo) error {
	opened, err := handle.Stat()
	if err != nil {
		return fmt.Errorf("inspect package root handle: %w", err)
	}
	current, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("reinspect package root: %w", err)
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() ||
		!os.SameFile(expected, opened) || !os.SameFile(opened, current) ||
		!sameStableMetadata(expected, opened) || !sameStableMetadata(opened, current) {
		return fmt.Errorf("package root identity or metadata changed during verification")
	}
	return nil
}

func sameStableMetadata(left, right fs.FileInfo) bool {
	return left.Mode() == right.Mode() && left.Size() == right.Size() && left.ModTime().Equal(right.ModTime())
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

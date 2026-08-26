package currentformselection

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestLoadRepositoryCompilesCheckedInCurrentGraph(t *testing.T) {
	selection, err := LoadRepository(repositoryRoot(t))
	if err != nil {
		t.Fatalf("LoadRepository: %v", err)
	}

	if got := len(selection.Families()); got != 8 {
		t.Fatalf("family count = %d, want 8", got)
	}
	if got := len(selection.Forms()); got != 31 {
		t.Fatalf("Form count = %d, want 31", got)
	}
	if got := len(selection.Interfaces()); got != 13 {
		t.Fatalf("Interface count = %d, want 13", got)
	}
	if got := len(selection.Bindings()); got != 6 {
		t.Fatalf("Binding count = %d, want 6", got)
	}

	snapshot := selection.Snapshot()
	if snapshot == nil {
		t.Fatal("selection has no compiled Snapshot")
	}
	if snapshot.HostAPI() != "forms.takoform.com/v1" {
		t.Fatalf("HostAPI = %q", snapshot.HostAPI())
	}
	for _, form := range selection.Forms() {
		if got, ok := snapshot.Default(form.Ref.APIVersion, form.Ref.Kind); !ok || got != form.Ref {
			t.Fatalf("default for %s/%s = %#v, %v; want %#v", form.Ref.APIVersion, form.Ref.Kind, got, ok, form.Ref)
		}
		if definition, ok := snapshot.Definition(form.Ref); !ok || len(definition) == 0 {
			t.Fatalf("exact definition for %#v is unavailable", form.Ref)
		}
	}
	for _, contract := range selection.Interfaces() {
		if definition, ok := snapshot.InterfaceDefinition(contract.Ref); !ok || len(definition) == 0 {
			t.Fatalf("exact Interface definition for %#v is unavailable", contract.Ref)
		}
	}
	for _, contract := range selection.Bindings() {
		if definition, ok := snapshot.BindingDefinition(contract.Ref); !ok || len(definition) == 0 {
			t.Fatalf("exact Binding definition for %#v is unavailable", contract.Ref)
		}
	}
}

func TestSelectionMetadataIsStableAndDefensivelyCopied(t *testing.T) {
	selection, err := LoadRepository(repositoryRoot(t))
	if err != nil {
		t.Fatalf("LoadRepository: %v", err)
	}

	families := selection.Families()
	forms := selection.Forms()
	interfaces := selection.Interfaces()
	bindings := selection.Bindings()
	if len(families) == 0 || len(forms) == 0 || len(interfaces) == 0 || len(bindings) == 0 {
		t.Fatal("current graph metadata is unexpectedly empty")
	}
	families[0].Group = "mutated"
	families[0].Forms[0].Path = "mutated"
	forms[0].Path = "mutated"
	interfaces[0].Path = "mutated"
	bindings[0].Path = "mutated"

	if got := selection.Families()[0].Group; got == "mutated" {
		t.Fatal("Families returned mutable internal metadata")
	}
	if got := selection.Families()[0].Forms[0].Path; got == "mutated" {
		t.Fatal("nested family Form metadata was not copied")
	}
	if got := selection.Forms()[0].Path; got == "mutated" {
		t.Fatal("Forms returned mutable internal metadata")
	}
	if got := selection.Interfaces()[0].Path; got == "mutated" {
		t.Fatal("Interfaces returned mutable internal metadata")
	}
	if got := selection.Bindings()[0].Path; got == "mutated" {
		t.Fatal("Bindings returned mutable internal metadata")
	}
}

func TestLoadRepositoryIgnoresHistoricalAuthoringSourcePaths(t *testing.T) {
	root := copyRepositoryGraph(t, repositoryRoot(t))
	indexPath := filepath.Join(root, "forms", "candidates", "current-family-index.json")
	indexRaw := mustRead(t, indexPath)
	var index currentFamilyIndex
	decodeIndex(t, indexRaw, &index)

	for i, family := range index.Families {
		path := filepath.Join(root, filepath.FromSlash(family.CandidateSet))
		raw := mustRead(t, path)
		raw = bytes.Replace(raw, []byte(`"authoringSource": "internal/`), []byte(`"authoringSource": "external/`), 1)
		if bytes.Equal(raw, mustRead(t, path)) {
			t.Fatalf("family %s did not contain authoringSource", family.Group)
		}
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatalf("rewrite %s: %v", family.CandidateSet, err)
		}
		index.Families[i].SHA256 = rawSHA256(raw)
	}

	interfacePath := filepath.Join(root, filepath.FromSlash(index.InterfaceCandidateSet.Path))
	interfaceRaw := mustRead(t, interfacePath)
	interfaceRaw = bytes.Replace(interfaceRaw, []byte(`"authoringSource": "cmd/`), []byte(`"authoringSource": "xxx/`), 1)
	if err := os.WriteFile(interfacePath, interfaceRaw, 0o600); err != nil {
		t.Fatalf("rewrite Interface candidate set: %v", err)
	}
	index.InterfaceCandidateSet.SHA256 = rawSHA256(interfaceRaw)

	bindingPath := filepath.Join(root, filepath.FromSlash(index.BindingCandidateSet.Path))
	bindingRaw := mustRead(t, bindingPath)
	bindingRaw = bytes.Replace(bindingRaw, []byte(`"authoringSource": "cmd/`), []byte(`"authoringSource": "xxx/`), 1)
	if err := os.WriteFile(bindingPath, bindingRaw, 0o600); err != nil {
		t.Fatalf("rewrite Binding candidate set: %v", err)
	}
	index.BindingCandidateSet.SHA256 = rawSHA256(bindingRaw)
	updatedIndex := mustMarshal(t, index)
	if err := os.WriteFile(indexPath, updatedIndex, 0o600); err != nil {
		t.Fatalf("rewrite current-family index: %v", err)
	}

	if _, err := LoadRepository(root); err != nil {
		t.Fatalf("LoadRepository treated historical authoringSource as authority: %v", err)
	}
}

func TestLoadRepositoryFailsClosedOnPathAndDigestDrift(t *testing.T) {
	t.Run("index path traversal", func(t *testing.T) {
		root := copyRepositoryGraph(t, repositoryRoot(t))
		indexPath := filepath.Join(root, "forms", "candidates", "current-family-index.json")
		raw := mustRead(t, indexPath)
		var index currentFamilyIndex
		decodeIndex(t, raw, &index)
		index.Families[0].CandidateSet = "../escape.json"
		if _, err := LoadRepositoryWithIndex(root, mustMarshal(t, index)); err == nil {
			t.Fatal("LoadRepository accepted a repository-relative traversal path")
		}
	})

	t.Run("raw candidate-set digest", func(t *testing.T) {
		root := copyRepositoryGraph(t, repositoryRoot(t))
		familyPath := filepath.Join(root, filepath.FromSlash("forms/candidates/container.forms.takoform.com/candidate-set.json"))
		raw := mustRead(t, familyPath)
		mutated := bytes.Replace(raw, []byte(`"publicationStatus": "unpublished"`), []byte(`"publicationStatus": "published"`), 1)
		if bytes.Equal(raw, mutated) {
			t.Fatal("candidate set mutation did not apply")
		}
		if err := os.WriteFile(familyPath, mutated, 0o600); err != nil {
			t.Fatalf("rewrite candidate set: %v", err)
		}
		if _, err := LoadRepository(root); err == nil {
			t.Fatal("LoadRepository accepted raw candidate-set bytes with a stale index pin")
		}
	})

	t.Run("symlink escape", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlink fixture requires Unix permissions")
		}
		root := copyRepositoryGraph(t, repositoryRoot(t))
		escape := filepath.Join(root, "outside")
		if err := os.MkdirAll(escape, 0o700); err != nil {
			t.Fatalf("create outside: %v", err)
		}
		outside := filepath.Join(root, "outside", "candidate-set.json")
		if err := os.WriteFile(outside, []byte(`{}`), 0o600); err != nil {
			t.Fatalf("write outside candidate set: %v", err)
		}
		link := filepath.Join(root, "forms", "candidates", "escape.json")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		indexPath := filepath.Join(root, "forms", "candidates", "current-family-index.json")
		var index currentFamilyIndex
		decodeIndex(t, mustRead(t, indexPath), &index)
		index.Families[0].CandidateSet = "forms/candidates/escape.json"
		if _, err := LoadRepositoryWithIndex(root, mustMarshal(t, index)); err == nil {
			t.Fatal("LoadRepository followed a symlink in a repository-relative path")
		}
	})
}

func TestSelectionSourceDoesNotImportOfficialCatalogs(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read currentformselection source directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw := mustRead(t, entry.Name())
		if bytes.Contains(raw, []byte("formcatalog")) || bytes.Contains(raw, []byte("currentformregistry")) {
			t.Fatalf("%s imports or references an official catalog/registry", entry.Name())
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

type currentFamilyIndex struct {
	Format                string             `json:"format"`
	Families              []familyIndexEntry `json:"families"`
	InterfaceCandidateSet artifactPointer    `json:"interfaceCandidateSet"`
	BindingCandidateSet   artifactPointer    `json:"bindingCandidateSet"`
}

type familyIndexEntry struct {
	Group        string `json:"group"`
	CandidateSet string `json:"candidateSet"`
	SHA256       string `json:"sha256"`
	FormCount    int    `json:"formCount"`
}

type artifactPointer struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

func decodeIndex(t *testing.T, raw []byte, index *currentFamilyIndex) {
	t.Helper()
	if err := formpackage.DecodeStrictIJSON(raw, index); err != nil {
		t.Fatalf("decode index: %v", err)
	}
}

func LoadRepositoryWithIndex(root string, index []byte) (*Selection, error) {
	indexPath := filepath.Join(root, "forms", "candidates", "current-family-index.json")
	if err := os.WriteFile(indexPath, index, 0o600); err != nil {
		return nil, err
	}
	return LoadRepository(root)
}

func copyRepositoryGraph(t *testing.T, source string) string {
	t.Helper()
	destination := t.TempDir()
	for _, relative := range []string{"forms/candidates", "interfaces/candidates", "bindings/candidates"} {
		if err := copyTree(filepath.Join(source, relative), filepath.Join(destination, relative)); err != nil {
			t.Fatalf("copy %s: %v", relative, err)
		}
	}
	return destination
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture source contains symlink %s", path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0o600)
	})
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

func rawSHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

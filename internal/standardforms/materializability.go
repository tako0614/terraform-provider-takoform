package standardforms

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const runtimeArtifactSetPath = "forms/standard-runtime-artifact-set.json"

var (
	sha256DigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	unsafeFixtureString = regexp.MustCompile(`(?i)(?:example\.(?:com|test)|\.test(?:/|$)|localhost|127\.0\.0\.1)`)
)

type RuntimeArtifactSet struct {
	Format          string                    `json:"format"`
	Status          string                    `json:"status"`
	Source          RuntimeArtifactSource     `json:"source"`
	ReleaseEvidence []RuntimeArtifactReadback `json:"releaseEvidence"`
	Artifacts       []RuntimeArtifact         `json:"artifacts"`
}

type RuntimeArtifactSource struct {
	Repository   string `json:"repository"`
	ReleaseTag   string `json:"releaseTag"`
	SourceCommit string `json:"sourceCommit"`
	ReleaseURL   string `json:"releaseUrl"`
}

type RuntimeArtifactReadback struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type RuntimeArtifact struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	ArtifactRef string `json:"artifactRef,omitempty"`
	Reference   string `json:"reference,omitempty"`
	Platform    string `json:"platform,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Digest      string `json:"digest"`
}

// VerifyMaterializableCandidate proves that the complete ten-package source
// set uses canonical desired fixtures a real host can attempt without
// substituting placeholder bytes or unsupported optional preferences. It does
// not admit a Form, select a target, or prove a live provider lifecycle.
func VerifyMaterializableCandidate(root string) error {
	set, err := readRuntimeArtifactSet(root)
	if err != nil {
		return err
	}
	if err := verifyRuntimeArtifactSet(set); err != nil {
		return err
	}
	if len(Specs) != 10 {
		return fmt.Errorf("materializable candidate must contain exactly ten Forms")
	}

	expectedKeys := map[string][]string{
		"EdgeWorker":             {"compatibilityDate", "compatibilityFlags", "connections", "name", "profiles", "source"},
		"ObjectBucket":           {"interfaces", "name", "storageClass"},
		"KVStore":                {"consistency", "name"},
		"SQLDatabase":            {"engine", "name"},
		"Queue":                  {"name"},
		"VectorIndex":            {"dimensions", "metric", "name"},
		"DurableWorkflow":        {"entrypoint", "name", "retry", "source"},
		"ContainerService":       {"image", "name", "ports", "publicHttp"},
		"StatefulActorNamespace": {"className", "migrationTag", "name", "storageProfile"},
		"Schedule":               {"connections", "cron", "name", "timezone"},
	}
	expectedInterfaces := map[string]string{
		"EdgeWorker":             "http.request",
		"ObjectBucket":           "object.storage",
		"KVStore":                "keyvalue.store",
		"SQLDatabase":            "sql.query",
		"Queue":                  "queue.messages",
		"VectorIndex":            "vector.query",
		"DurableWorkflow":        "workflow.invoke",
		"ContainerService":       "http.request",
		"StatefulActorNamespace": "actor.invoke",
	}
	seen := make(map[string]struct{}, len(Specs))
	for _, spec := range Specs {
		if _, duplicate := seen[spec.Kind]; duplicate {
			return fmt.Errorf("materializable candidate duplicates %s", spec.Kind)
		}
		seen[spec.Kind] = struct{}{}
		desired, err := canonicalDesired(spec.Kind)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(desired))
		for key := range desired {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if !reflect.DeepEqual(keys, expectedKeys[spec.Kind]) {
			return fmt.Errorf("%s canonical desired keys are not the reviewed materializable set: %v", spec.Kind, keys)
		}
		if err := rejectUnsafeFixtureStrings(spec.Kind, desired); err != nil {
			return err
		}
		var definition formpackage.FormDefinition
		definitionPath := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", spec.Slug, "definition.json")
		if err := readJSON(definitionPath, &definition); err != nil {
			return fmt.Errorf("read %s portable Interface descriptors: %w", spec.Kind, err)
		}
		expectedName, exposesRuntimeSurface := expectedInterfaces[spec.Kind]
		if !exposesRuntimeSurface {
			if spec.Kind != "Schedule" || len(definition.Interfaces) != 0 {
				return fmt.Errorf("%s portable Interface descriptor audit is incomplete", spec.Kind)
			}
			continue
		}
		if len(definition.Interfaces) != 1 {
			return fmt.Errorf("%s must declare exactly one portable Interface", spec.Kind)
		}
		descriptor := definition.Interfaces[0]
		if descriptor.Name != expectedName || descriptor.Version != "1" || !descriptor.Required || descriptor.Document == nil || descriptor.DocumentSchema == nil {
			return fmt.Errorf("%s portable Interface identity or required contract is invalid", spec.Kind)
		}
		actualDescriptors, err := json.Marshal(definition.Interfaces)
		if err != nil {
			return err
		}
		reviewedDescriptors, err := json.Marshal(standardInterfaceDescriptors(spec.Kind))
		if err != nil {
			return err
		}
		if string(actualDescriptors) != string(reviewedDescriptors) {
			return fmt.Errorf("%s portable Interface descriptor is not the reviewed standard descriptor", spec.Kind)
		}
		for _, input := range descriptor.Inputs {
			if !formpackage.PortableInterfaceInputSource(input.Source) {
				return fmt.Errorf("%s Interface input %s is not portable", spec.Kind, input.Name)
			}
		}
		descriptorRaw, err := json.Marshal(descriptor)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(descriptorRaw)), "takosumi.cloud") {
			return fmt.Errorf("%s Interface descriptor contains a Cloud-specific identity", spec.Kind)
		}
	}
	if len(seen) != len(expectedKeys) || len(expectedInterfaces) != len(Specs)-1 {
		return fmt.Errorf("materializable candidate does not close over all ten Forms")
	}

	byKind := make(map[string]RuntimeArtifact, len(set.Artifacts))
	for _, artifact := range set.Artifacts {
		byKind[artifact.Kind] = artifact
	}
	edge, _ := canonicalDesired("EdgeWorker")
	edgeSource, _ := edge["source"].(map[string]any)
	if edgeSource["artifactUrl"] != byKind["EdgeWorker"].URL || "sha256:"+fmt.Sprint(edgeSource["artifactSha256"]) != byKind["EdgeWorker"].Digest {
		return fmt.Errorf("EdgeWorker fixture is not bound to the published runtime readback")
	}
	edgeConnections, _ := edge["connections"].(map[string]any)
	edgeAssets, _ := edgeConnections["ASSETS"].(map[string]any)
	if len(edgeConnections) != 1 || edgeAssets["resource"] != "ObjectBucket/edge-assets" || edgeAssets["projection"] != "object.binding.v1" ||
		!reflect.DeepEqual(edgeAssets["permissions"], []any{"read", "write"}) {
		return fmt.Errorf("EdgeWorker fixture does not bind the exact portable ObjectBucket projection")
	}
	workflow, _ := canonicalDesired("DurableWorkflow")
	workflowSource, _ := workflow["source"].(map[string]any)
	if workflowSource["artifactRef"] != byKind["DurableWorkflow"].ArtifactRef || "sha256:"+fmt.Sprint(workflowSource["artifactSha256"]) != byKind["DurableWorkflow"].Digest {
		return fmt.Errorf("DurableWorkflow fixture is not bound to the published runtime readback")
	}
	container, _ := canonicalDesired("ContainerService")
	if container["image"] != byKind["ContainerService"].Reference {
		return fmt.Errorf("ContainerService fixture is not bound to the published OCI digest")
	}
	var sqlOutput map[string]any
	if err := readJSON(filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", "sql-database", "fixtures", "output.json"), &sqlOutput); err != nil {
		return err
	}
	if sqlOutput["engine"] != "sqlite" {
		return fmt.Errorf("SQLDatabase query Interface cannot resolve its portable engine output")
	}
	schedule, _ := canonicalDesired("Schedule")
	connections, _ := schedule["connections"].(map[string]any)
	workflowConnection, _ := connections["workflow"].(map[string]any)
	if len(connections) != 1 || workflowConnection["resource"] != "DurableWorkflow/ingest" || workflowConnection["projection"] != "schedule_trigger" {
		return fmt.Errorf("Schedule fixture does not bind the exact materializable workflow dependency")
	}
	return nil
}

// VerifyMaterializationReadback downloads the immutable release assets and
// the exact OCI manifest named by the reviewed descriptor. Distribution is not
// a trust root; this is reachability and byte-identity evidence only.
func VerifyMaterializationReadback(ctx context.Context, root string, client *http.Client) error {
	if err := VerifyMaterializableCandidate(root); err != nil {
		return err
	}
	set, err := readRuntimeArtifactSet(root)
	if err != nil {
		return err
	}
	if client == nil {
		client = http.DefaultClient
	}
	for _, evidence := range set.ReleaseEvidence {
		if _, err := fetchExact(ctx, client, evidence.URL, evidence.Size, evidence.Digest, ""); err != nil {
			return fmt.Errorf("runtime release evidence %s: %w", evidence.Name, err)
		}
	}
	for _, artifact := range set.Artifacts {
		if artifact.URL != "" {
			if _, err := fetchExact(ctx, client, artifact.URL, artifact.Size, artifact.Digest, ""); err != nil {
				return fmt.Errorf("%s runtime artifact: %w", artifact.Kind, err)
			}
			continue
		}
		if artifact.Kind == "ContainerService" {
			if err := readDockerManifest(ctx, client, artifact.Reference, artifact.Digest); err != nil {
				return fmt.Errorf("ContainerService OCI artifact: %w", err)
			}
		}
	}
	return nil
}

func readRuntimeArtifactSet(root string) (RuntimeArtifactSet, error) {
	var set RuntimeArtifactSet
	if err := readJSON(filepath.Join(root, filepath.FromSlash(runtimeArtifactSetPath)), &set); err != nil {
		return RuntimeArtifactSet{}, err
	}
	return set, nil
}

func verifyRuntimeArtifactSet(set RuntimeArtifactSet) error {
	if set.Format != "takoform.standard-runtime-artifact-set@v1" || set.Status != "published-immutable-readback" ||
		set.Source.Repository != "tako0614/takosumi" || set.Source.ReleaseTag != "standard-form-runtime-v1.0.3" ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(set.Source.SourceCommit) ||
		set.Source.ReleaseURL != "https://github.com/tako0614/takosumi/releases/tag/"+set.Source.ReleaseTag {
		return fmt.Errorf("standard runtime artifact source identity is invalid")
	}
	expectedEvidence := map[string]struct{}{
		"runtime-manifest.json": {}, "release-manifest.json": {}, "release-safety-readback.json": {},
	}
	for _, evidence := range set.ReleaseEvidence {
		if _, ok := expectedEvidence[evidence.Name]; !ok || evidence.Size <= 0 || !sha256DigestPattern.MatchString(evidence.Digest) || !exactReleaseAssetURL(evidence.URL, set.Source.ReleaseTag, evidence.Name) {
			return fmt.Errorf("invalid runtime release evidence %s", evidence.Name)
		}
		delete(expectedEvidence, evidence.Name)
	}
	if len(expectedEvidence) != 0 {
		return fmt.Errorf("runtime release evidence is incomplete")
	}
	if len(set.Artifacts) != 3 {
		return fmt.Errorf("runtime artifact set must contain EdgeWorker, DurableWorkflow, and ContainerService")
	}
	seen := map[string]struct{}{}
	for _, artifact := range set.Artifacts {
		if _, duplicate := seen[artifact.Kind]; duplicate || !sha256DigestPattern.MatchString(artifact.Digest) {
			return fmt.Errorf("invalid or duplicate runtime artifact %s", artifact.Kind)
		}
		seen[artifact.Kind] = struct{}{}
		switch artifact.Kind {
		case "EdgeWorker":
			if artifact.Name != "edge-worker.mjs" || artifact.Size <= 0 || !exactReleaseAssetURL(artifact.URL, set.Source.ReleaseTag, artifact.Name) || artifact.Reference != "" || artifact.ArtifactRef != "" {
				return fmt.Errorf("invalid EdgeWorker runtime artifact")
			}
		case "DurableWorkflow":
			if artifact.Name != "durable-workflow.mjs" || artifact.Size <= 0 || !exactReleaseAssetURL(artifact.URL, set.Source.ReleaseTag, artifact.Name) || artifact.ArtifactRef != "standard-form-runtime/v1.0.3/durable-workflow.mjs" || artifact.Reference != "" {
				return fmt.Errorf("invalid DurableWorkflow runtime artifact")
			}
		case "ContainerService":
			if artifact.Name != "container-service" || artifact.URL != "" || artifact.ArtifactRef != "" || artifact.Platform != "linux/amd64" || artifact.Reference != "docker.io/library/nginx@"+artifact.Digest {
				return fmt.Errorf("invalid ContainerService runtime artifact")
			}
		default:
			return fmt.Errorf("unexpected runtime artifact kind %s", artifact.Kind)
		}
	}
	return nil
}

func exactReleaseAssetURL(value, tag, name string) bool {
	want := "https://github.com/tako0614/takosumi/releases/download/" + tag + "/" + name
	return value == want
}

func rejectUnsafeFixtureStrings(kind string, value any) error {
	switch typed := value.(type) {
	case string:
		if unsafeFixtureString.MatchString(typed) || illustrativeDigest(typed) {
			return fmt.Errorf("%s canonical fixture contains an illustrative or unsafe value", kind)
		}
	case []any:
		for _, child := range typed {
			if err := rejectUnsafeFixtureStrings(kind, child); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, child := range typed {
			if err := rejectUnsafeFixtureStrings(kind, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func illustrativeDigest(value string) bool {
	value = strings.TrimPrefix(strings.ToLower(value), "sha256:")
	if len(value) != 64 {
		return false
	}
	for index := 1; index < len(value); index++ {
		if value[index] != value[0] {
			return false
		}
	}
	return strings.ContainsRune("0123456789abcdef", rune(value[0]))
}

func fetchExact(ctx context.Context, client *http.Client, rawURL string, size int64, digest, authorization string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	request.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/octet-stream")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", rawURL, response.Status)
	}
	limit := size
	if limit <= 0 {
		limit = 8 << 20
	}
	bytes, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(bytes)) > limit || (size > 0 && int64(len(bytes)) != size) {
		return nil, fmt.Errorf("size mismatch: got %d want %d", len(bytes), size)
	}
	sum := sha256.Sum256(bytes)
	actual := "sha256:" + hex.EncodeToString(sum[:])
	if actual != digest {
		return nil, fmt.Errorf("digest mismatch: got %s want %s", actual, digest)
	}
	return bytes, nil
}

func readDockerManifest(ctx context.Context, client *http.Client, reference, digest string) error {
	const prefix = "docker.io/library/nginx@"
	if !strings.HasPrefix(reference, prefix) || strings.TrimPrefix(reference, prefix) != digest {
		return fmt.Errorf("unsupported OCI reference %s", reference)
	}
	manifestURL := "https://registry-1.docker.io/v2/library/nginx/manifests/" + url.PathEscape(digest)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusUnauthorized {
		response.Body.Close()
		return fmt.Errorf("registry challenge returned %s", response.Status)
	}
	challenge := response.Header.Get("Www-Authenticate")
	response.Body.Close()
	realm, service, scope, err := parseBearerChallenge(challenge)
	if err != nil {
		return err
	}
	tokenURL, err := url.Parse(realm)
	if err != nil {
		return err
	}
	query := tokenURL.Query()
	query.Set("service", service)
	query.Set("scope", scope)
	tokenURL.RawQuery = query.Encode()
	tokenBytes, err := fetchUnverifiedJSON(ctx, client, tokenURL.String())
	if err != nil {
		return err
	}
	var token struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(tokenBytes, &token); err != nil || token.Token == "" {
		return fmt.Errorf("registry token response is invalid")
	}
	bytes, err := fetchExact(ctx, client, manifestURL, 0, digest, "Bearer "+token.Token)
	if err != nil {
		return err
	}
	if len(bytes) == 0 {
		return fmt.Errorf("registry returned an empty manifest")
	}
	return nil
}

func parseBearerChallenge(value string) (string, string, string, error) {
	if !strings.HasPrefix(value, "Bearer ") {
		return "", "", "", fmt.Errorf("registry did not return a Bearer challenge")
	}
	fields := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(value, "Bearer "), ",") {
		pair := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(pair) != 2 {
			continue
		}
		fields[pair[0]] = strings.Trim(pair[1], `"`)
	}
	if fields["realm"] == "" || fields["service"] == "" || fields["scope"] == "" {
		return "", "", "", fmt.Errorf("registry Bearer challenge is incomplete")
	}
	return fields["realm"], fields["service"], fields["scope"], nil
}

func fetchUnverifiedJSON(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", rawURL, response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

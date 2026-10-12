//go:build native

package mountedv2

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const form = "https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/"
const queueForm = "https://edge.forms.takoform.com/forms/AtLeastOnceQueue/0.2.0/"
const privateForm = "https://edge.forms.takoform.com/forms/WorkerVersion/0.5.0/"
const privateSentinel = "fixture-private-input-value"
const apiPath = "/apis/forms.takoform.com/v2"

type fixtureHost struct {
	mu          sync.Mutex
	server      *httptest.Server
	mode        string
	mutations   int
	reads       int
	uid         string
	generation  int
	lastAction  string
	opID        string
	exists      bool
	retention   int
	privateSeen bool
}

func newFixtureHost(t *testing.T, mode string) *fixtureHost {
	t.Helper()
	h := &fixtureHost{mode: mode, uid: "uid-exact-1"}
	if mode == "update" {
		h.uid = "uid-queue-1"
	}
	if mode == "private" {
		h.uid = "uid-version-1"
	}
	h.server = httptest.NewTLSServer(http.HandlerFunc(h.serve))
	t.Cleanup(h.server.Close)
	return h
}

func (h *fixtureHost) serve(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	wantForm, wantName := form, "app"
	if h.mode == "update" {
		wantForm, wantName = queueForm, "queue"
	}
	if h.mode == "private" {
		wantForm, wantName = privateForm, "version"
	}
	if r.URL.Path != "/.well-known/takoform/v2" && r.Header.Get("Authorization") != "Bearer fixture-token" {
		h.problem(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/.well-known/takoform/v2":
		h.write(w, map[string]any{"api": "forms.takoform.com/v2", "baseUrl": h.server.URL + apiPath, "documentation": "https://host.example/docs", "authentication": map[string]any{"schemes": []string{"Bearer"}, "documentation": "https://host.example/auth"}, "capabilities": map[string]any{"offerings": false, "previews": false, "privateInputs": h.mode == "private"}, "limits": map[string]any{"maxRequestBytes": 65536, "maxPageSize": 50, "replayWindowSeconds": 3600}})
	case r.Method == http.MethodGet && r.URL.Path == apiPath+"/support":
		if r.URL.Query().Get("form") != wantForm {
			h.problem(w, http.StatusNotFound, "form_unknown")
			return
		}
		h.write(w, map[string]any{"form": wantForm, "supported": true, "operations": []string{"create", "read", "update", "delete"}, "privateInputs": h.mode == "private"})
	case r.Method == http.MethodPost && r.URL.Path == apiPath+"/resources":
		if r.Header.Get("Idempotency-Key") == "" || h.exists {
			h.problem(w, http.StatusConflict, "conflict")
			return
		}
		var request struct {
			Form          string            `json:"form"`
			Space         string            `json:"space"`
			Name          string            `json:"name"`
			PrivateInputs map[string]string `json:"privateInputs"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Form != wantForm || request.Space != "prod" || request.Name != wantName {
			h.problem(w, http.StatusBadRequest, "invalid")
			return
		}
		if h.mode == "private" {
			if request.PrivateInputs["API_TOKEN"] != privateSentinel || len(request.PrivateInputs) != 1 {
				h.problem(w, http.StatusBadRequest, "private_input_missing")
				return
			}
			h.privateSeen = true
		}
		h.mutations++
		h.exists = true
		h.generation = 1
		h.lastAction = "create"
		h.opID = "op-1"
		if h.mode == "update" {
			h.retention = 60
		}
		if h.mode == "unknown" {
			h.write(w, map[string]any{"invalid": "operation"})
			return
		}
		h.operation(w)
	case r.Method == http.MethodGet && r.URL.Path == apiPath+"/operations/"+h.opID && h.opID != "":
		h.reads++
		h.operation(w)
	case r.Method == http.MethodGet && r.URL.Path == apiPath+"/resources/"+h.uid:
		h.reads++
		if !h.exists {
			h.problem(w, http.StatusNotFound, "resource_not_found")
			return
		}
		spec := map[string]any{}
		if h.mode == "update" {
			spec["messageRetentionSeconds"] = h.retention
		}
		if h.mode == "private" {
			spec = map[string]any{"worker": map[string]any{"resourceUid": "worker-uid"}, "bundle": map[string]any{"resourceUid": "bundle-uid"}, "handlers": []string{"fetch"}, "vars": map[string]any{"a": 2, "z": 1}, "requiredSensitiveVars": []string{"API_TOKEN"}}
		}
		h.write(w, map[string]any{"uid": h.uid, "form": wantForm, "space": "prod", "name": wantName, "generation": h.generation, "observedGeneration": h.generation, "observedAt": "2026-10-12T00:00:00Z", "phase": "idle", "spec": spec, "observed": map[string]any{}, "output": map[string]any{}, "lastOperation": h.opID})
	case r.Method == http.MethodPut && r.URL.Path == apiPath+"/resources/"+h.uid && h.mode == "update":
		if !h.exists || r.Header.Get("Idempotency-Key") == "" || r.Header.Get("Takoform-Expected-Generation") != strconv.Itoa(h.generation) {
			h.problem(w, http.StatusConflict, "conflict")
			return
		}
		var request struct {
			Spec struct {
				Retention int `json:"messageRetentionSeconds"`
			} `json:"spec"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Spec.Retention != 120 {
			h.problem(w, http.StatusBadRequest, "invalid")
			return
		}
		h.mutations++
		h.generation++
		h.retention = request.Spec.Retention
		h.lastAction = "update"
		h.opID = "op-2"
		h.operation(w)
	case r.Method == http.MethodDelete && r.URL.Path == apiPath+"/resources/"+h.uid:
		if !h.exists || r.Header.Get("Idempotency-Key") == "" || r.Header.Get("Takoform-Expected-Generation") != strconv.Itoa(h.generation) {
			h.problem(w, http.StatusConflict, "conflict")
			return
		}
		h.mutations++
		h.generation++
		h.exists = false
		h.lastAction = "delete"
		if h.mode == "update" {
			h.opID = "op-3"
		} else {
			h.opID = "op-2"
		}
		h.operation(w)
	default:
		h.problem(w, http.StatusNotFound, "resource_not_found")
	}
}

func (h *fixtureHost) write(w http.ResponseWriter, value any) { _ = json.NewEncoder(w).Encode(value) }
func (h *fixtureHost) problem(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	h.write(w, map[string]any{"type": "about:blank", "title": code, "status": status, "code": code})
}
func (h *fixtureHost) operation(w http.ResponseWriter) {
	w.Header().Set("Location", h.server.URL+apiPath+"/operations/"+h.opID)
	h.write(w, map[string]any{"id": h.opID, "resourceUid": h.uid, "action": h.lastAction, "generation": h.generation, "status": "succeeded", "effect": "complete", "createdAt": "2026-10-12T00:00:00Z", "updatedAt": "2026-10-12T00:00:00Z", "retainUntil": time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)})
}
func (h *fixtureHost) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mutations, h.reads
}
func (h *fixtureHost) sawPrivateInput() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.privateSeen }

type runner struct {
	t             *testing.T
	root          string
	dir           string
	pluginDir     string
	certFile      string
	storePath     string
	marker        string
	retention     int
	privateInputs bool
}

func TestMountedV2NativeProcessCustody(t *testing.T) {
	tofu, err := exec.LookPath("tofu")
	if err != nil {
		t.Fatalf("OpenTofu 1.12.3 required: %v", err)
	}
	if out, err := exec.Command(tofu, "version").CombinedOutput(); err != nil || !strings.Contains(string(out), "OpenTofu v1.12.3") {
		t.Fatalf("pinned OpenTofu required: %v %s", err, out)
	}
	root := repositoryRoot(t)
	for _, tc := range []struct {
		name, mode string
		crash      bool
	}{
		{"ordinary-create-read-delete", "normal", false},
		{"post-ack-process-exit-recovers-exact-uid", "normal", true},
		{"unknown-pre-ack-refuses-second-create", "unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := newFixtureHost(t, tc.mode)
			r := setupRunner(t, root, host, tc.crash)
			r.runOK("init", "init", "-input=false", "-no-color")
			r.runOK("saved-plan", "plan", "-input=false", "-no-color", "-out=saved.tfplan")
			r.assertSecretAbsent()
			first := r.run("apply", "apply", "-input=false", "-no-color", "-auto-approve")
			if tc.crash {
				if first.code == 0 || !strings.Contains(first.output, "Plugin did not respond") {
					t.Fatalf("provider OS exit not observed: exit=%d %s", first.code, first.output)
				}
				pidBytes, err := os.ReadFile(r.marker)
				if err != nil {
					t.Fatalf("provider exit marker absent: %v", err)
				}
				pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
				if err != nil || pid <= 0 || pid == first.pid {
					t.Fatalf("marker is not a distinct Provider PID: %q; CLI PID=%d", pidBytes, first.pid)
				}
				if state := r.runOK("state-list-after-crash", "state", "list"); strings.TrimSpace(state.output) != "" {
					t.Fatalf("state committed after OS exit: %q", state.output)
				}
				if mutations, _ := host.counts(); mutations != 1 {
					t.Fatalf("create mutations after crash=%d", mutations)
				}
				r.assertJournalContains("op-1")
				r.runOK("fresh-apply", "apply", "-input=false", "-no-color", "-auto-approve")
				if mutations, reads := host.counts(); mutations != 1 || reads < 2 {
					t.Fatalf("recovery mutated or did not read exact operation/resource: mutations=%d reads=%d", mutations, reads)
				}
			} else if tc.mode == "unknown" {
				if first.code == 0 || !strings.Contains(first.output, "Unknown v2 Create outcome") {
					t.Fatalf("pre-ACK failure not observed: exit=%d %s", first.code, first.output)
				}
				if state := r.runOK("state-list-after-unknown", "state", "list"); strings.TrimSpace(state.output) != "" {
					t.Fatalf("unknown outcome entered state: %q", state.output)
				}
				second := r.run("fresh-apply", "apply", "-input=false", "-no-color", "-auto-approve")
				if second.code == 0 || !strings.Contains(second.output, "Unknown pre-ACK v2 outcome") {
					t.Fatalf("second apply did not refuse unknown claim: exit=%d %s", second.code, second.output)
				}
				if mutations, _ := host.counts(); mutations != 1 {
					t.Fatalf("unknown outcome created twice: %d", mutations)
				}
				return
			} else if first.code != 0 {
				t.Fatalf("ordinary apply: exit=%d %s", first.code, first.output)
			}
			state := r.runOK("state-show", "state", "show", "takoform_module_worker.x")
			if !strings.Contains(state.output, "uid-exact-1") {
				t.Fatalf("wrong recovered UID: %s", state.output)
			}
			r.runOK("refresh", "plan", "-input=false", "-no-color", "-refresh-only")
			r.assertSecretAbsent()
			r.runOK("destroy", "destroy", "-input=false", "-no-color", "-auto-approve")
			if mutations, _ := host.counts(); mutations != 2 {
				t.Fatalf("create+delete mutations=%d", mutations)
			}
			if state := r.runOK("state-list-after-delete", "state", "list"); strings.TrimSpace(state.output) != "" {
				t.Fatalf("delete left state: %q", state.output)
			}
			r.assertSecretAbsent()
		})
	}
	t.Run("typed-queue-update", func(t *testing.T) {
		host := newFixtureHost(t, "update")
		r := setupRunner(t, root, host, false)
		r.retention = 60
		r.runOK("init", "init", "-input=false", "-no-color")
		r.runOK("create", "apply", "-input=false", "-no-color", "-auto-approve")
		r.retention = 120
		r.runOK("update", "apply", "-input=false", "-no-color", "-auto-approve")
		state := r.runOK("updated-state", "state", "show", "takoform_at_least_once_queue.x")
		if !strings.Contains(state.output, "message_retention_seconds = 120") {
			t.Fatalf("updated queue state missing value: %s", state.output)
		}
		r.runOK("delete", "destroy", "-input=false", "-no-color", "-auto-approve")
		if mutations, reads := host.counts(); mutations != 3 || reads < 3 {
			t.Fatalf("queue CRUD did not traverse Host: mutations=%d reads=%d", mutations, reads)
		}
		r.assertSecretAbsent()
	})
	t.Run("worker-version-process-private-input-absence", func(t *testing.T) {
		host := newFixtureHost(t, "private")
		r := setupRunner(t, root, host, false)
		r.privateInputs = true
		r.runOK("init", "init", "-input=false", "-no-color")
		r.runOK("saved-plan", "plan", "-input=false", "-no-color", "-out=saved.tfplan")
		r.assertSecretAbsent()
		r.assertDecodedSecretAbsent("saved plan", r.runOK("decoded-plan", "show", "-json", "saved.tfplan").output)
		r.runOK("create", "apply", "-input=false", "-no-color", "-auto-approve")
		if !host.sawPrivateInput() {
			t.Fatal("Host did not receive process-only private input")
		}
		r.assertSecretAbsent()
		r.assertDecodedSecretAbsent("state", r.runOK("decoded-state", "show", "-json").output)
		journal, err := os.ReadFile(filepath.Join(r.storePath, "state.json"))
		if err != nil {
			t.Fatal(err)
		}
		r.assertDecodedSecretAbsent("journal", string(journal))
		r.runOK("delete", "destroy", "-input=false", "-no-color", "-auto-approve")
		if mutations, _ := host.counts(); mutations != 2 {
			t.Fatalf("WorkerVersion create+delete mutations=%d", mutations)
		}
	})
}

func setupRunner(t *testing.T, root string, h *fixtureHost, crash bool) runner {
	t.Helper()
	temp := t.TempDir()
	r := runner{t: t, root: root, dir: filepath.Join(temp, "module"), pluginDir: filepath.Join(temp, "plugins"), certFile: filepath.Join(temp, "fixture-ca.pem"), storePath: filepath.Join(temp, "intent-store"), marker: filepath.Join(temp, "provider-exited.pid")}
	if err := os.Mkdir(r.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cert := h.server.Certificate()
	if _, err := x509.ParseCertificate(cert.Raw); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	pluginInstall := filepath.Join(r.pluginDir, "registry.terraform.io", "tako0614", "takoform", "5.0.0", runtime.GOOS+"_"+runtime.GOARCH)
	if err := os.MkdirAll(pluginInstall, 0o700); err != nil {
		t.Fatal(err)
	}
	plugin := filepath.Join(pluginInstall, "terraform-provider-takoform_v5.0.0")
	buildArgs := []string{"build", "-ldflags=-X main.version=5.0.0", "-o", plugin, "."}
	if crash {
		buildArgs = append([]string{"build", "-overlay", crashOverlay(t, root, temp), "-ldflags=-X main.version=5.0.0", "-o", plugin}, ".")
	}
	build := exec.Command("go", buildArgs...)
	build.Dir = root
	build.Env = append(os.Environ(), "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build mounted Provider: %v\n%s", err, output)
	}
	init := exec.Command(filepath.Join(temp, "intent-store-init"), "--path", r.storePath, "--store-id", "fixed-scope", "--host-api", h.server.URL+apiPath)
	initBuild := exec.Command("go", "build", "-o", init.Path, "./cmd/intent-store-init")
	initBuild.Dir = root
	initBuild.Env = build.Env
	if output, err := initBuild.CombinedOutput(); err != nil {
		t.Fatalf("build store init: %v\n%s", err, output)
	}
	if output, err := init.CombinedOutput(); err != nil {
		t.Fatalf("preinitialize store: %v\n%s", err, output)
	}
	hcl := fmt.Sprintf(`terraform {
  required_providers {
    takoform = { source = "registry.terraform.io/tako0614/takoform", version = "5.0.0" }
  }
}
variable "token" {
  type = string
  sensitive = true
  ephemeral = true
}
variable "absolute_store_path" { type = string }
provider "takoform" {
  endpoint = %q
  host_api_root = %q
  token = var.token
  store_path = var.absolute_store_path
  store_id = "fixed-scope"
  space = "prod"
}
resource "takoform_module_worker" "x" {
  name = "app"
}
`, h.server.URL, h.server.URL+apiPath)
	if err := os.WriteFile(filepath.Join(r.dir, "main.tf"), []byte(hcl), 0o600); err != nil {
		t.Fatal(err)
	}
	if h.mode == "update" {
		hcl = strings.Replace(hcl, `resource "takoform_module_worker" "x" {
  name = "app"
}`, `variable "retention" { type = number }
resource "takoform_at_least_once_queue" "x" {
  name = "queue"
  space = "prod"
  spec = { message_retention_seconds = var.retention }
}`, 1)
		if err := os.WriteFile(filepath.Join(r.dir, "main.tf"), []byte(hcl), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if h.mode == "private" {
		hcl = strings.Replace(hcl, `resource "takoform_module_worker" "x" {
  name = "app"
}`, `resource "takoform_worker_version" "x" {
  name = "version"
  spec = {
    worker = { resource_uid = "worker-uid" }
    bundle = { resource_uid = "bundle-uid" }
    handlers = ["fetch"]
    vars_json = "{\"z\":1.0,\"a\":2}"
    required_sensitive_vars = ["API_TOKEN"]
  }
}`, 1)
		if err := os.WriteFile(filepath.Join(r.dir, "main.tf"), []byte(hcl), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func crashOverlay(t *testing.T, root, temp string) string {
	t.Helper()
	source := filepath.Join(root, "internal", "provider", "v2_resource.go")
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("\tresp.State = tfsdk.State{Schema: req.Plan.Schema, Raw: req.Plan.Raw}\n")
	if bytes.Count(original, marker) != 1 {
		t.Fatal("post-RecordAccepted overlay seam changed")
	}
	replacement := []byte("\tif marker := os.Getenv(\"NATIVE_V2_EXIT_AFTER_ACK\"); marker != \"\" { _ = os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0600); os.Exit(73) }\n")
	patched := bytes.Replace(original, marker, append(replacement, marker...), 1)
	patched = bytes.Replace(patched, []byte("import (\n"), []byte("import (\n\t\"os\"\n"), 1)
	overlaySource := filepath.Join(temp, "v2_resource_overlay.go")
	if err := os.WriteFile(overlaySource, patched, 0o600); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(temp, "overlay.json")
	data, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: overlaySource}})
	if err := os.WriteFile(overlay, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return overlay
}

type result struct {
	code, pid int
	output    string
}

func (r runner) runOK(label string, args ...string) result {
	r.t.Helper()
	got := r.run(label, args...)
	if got.code != 0 {
		r.t.Fatalf("tofu %s: exit=%d PID=%d\n%s", label, got.code, got.pid, got.output)
	}
	return got
}
func (r runner) run(label string, args ...string) result {
	r.t.Helper()
	if args[0] == "init" {
		args = append(args, "-plugin-dir", r.pluginDir)
	}
	cmd := exec.Command("tofu", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+r.certFile, "TF_VAR_token=fixture-token", "TF_VAR_absolute_store_path="+r.storePath, "TF_IN_AUTOMATION=1", "TF_INPUT=0")
	if r.retention != 0 {
		cmd.Env = append(cmd.Env, "TF_VAR_retention="+strconv.Itoa(r.retention))
	}
	if r.privateInputs {
		cmd.Env = append(cmd.Env, `TAKOFORM_V2_PRIVATE_INPUTS_JSON={"prod":{"version":{"API_TOKEN":"`+privateSentinel+`"}}}`)
	}
	if label == "apply" && strings.Contains(r.t.Name(), "post-ack") {
		cmd.Env = append(cmd.Env, "NATIVE_V2_EXIT_AFTER_ACK="+r.marker)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			r.t.Fatalf("tofu %s start: %v", label, err)
		}
	}
	r.t.Logf("tofu %s CLI PID=%d exit=%d seconds not including compile", label, cmd.Process.Pid, code)
	return result{code: code, pid: cmd.Process.Pid, output: out.String()}
}
func (r runner) assertSecretAbsent() {
	r.t.Helper()
	for _, name := range []string{"terraform.tfstate", ".terraform.lock.hcl"} {
		data, err := os.ReadFile(filepath.Join(r.dir, name))
		if err != nil {
			if os.IsNotExist(err) && name == "terraform.tfstate" {
				continue
			}
			r.t.Fatal(err)
		}
		if bytes.Contains(data, []byte("fixture-token")) || bytes.Contains(data, []byte(privateSentinel)) {
			r.t.Fatalf("credential appeared in %s", name)
		}
	}
	if data, err := os.ReadFile(filepath.Join(r.dir, "saved.tfplan")); err == nil && (bytes.Contains(data, []byte("fixture-token")) || bytes.Contains(data, []byte(privateSentinel))) {
		r.t.Fatal("credential appeared in saved plan")
	}
	err := filepath.WalkDir(r.storePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte("fixture-token")) || bytes.Contains(data, []byte(privateSentinel)) {
			return fmt.Errorf("credential appeared in journal %s", path)
		}
		return nil
	})
	if err != nil {
		r.t.Fatal(err)
	}
}
func (r runner) assertDecodedSecretAbsent(label, document string) {
	r.t.Helper()
	var value any
	if err := json.Unmarshal([]byte(document), &value); err != nil {
		r.t.Fatalf("%s not decoded JSON: %v", label, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		r.t.Fatal(err)
	}
	if bytes.Contains(canonical, []byte(privateSentinel)) || bytes.Contains(canonical, []byte("fixture-token")) {
		r.t.Fatalf("private value appeared in decoded %s", label)
	}
}
func (r runner) assertJournalContains(want string) {
	r.t.Helper()
	found := false
	err := filepath.WalkDir(r.storePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(want)) {
			found = true
		}
		return nil
	})
	if err != nil || !found {
		r.t.Fatalf("accepted Operation %q not persisted in journal: %v", want, err)
	}
}
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate fixture")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

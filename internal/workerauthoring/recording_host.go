package workerauthoring

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

// mutation is one desired-state write the CLI drove, in the order the host saw
// it. Only the resource lifecycle routes are recorded: an ordering claim about
// "the deployment moved before the old version was deleted" is a claim about
// these five verbs and nothing else.
type mutation struct {
	Method string `json:"method"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status int    `json:"status"`
}

// recordingHost is a disposable decorator over the stable-v1 reference host.
//
// It does two things the reference host must not do itself. It RECORDS the
// resource mutations in order, which is how a scenario proves that a
// replacement never left the worker unserved rather than merely finishing
// green. And it can WITHHOLD the Host Support Profile of named Form kinds,
// which is how a scenario proves the provider decides capability at plan time:
// a host that implements WorkerVersion and `edge.kv` while implementing none of
// another substrate is a real host shape, and there is no other way to stand
// one up.
type recordingHost struct {
	inner            http.Handler
	unsupportedKinds map[string]bool

	mu        sync.Mutex
	mutations []mutation
}

func newRecordingHost(inner http.Handler, unsupportedKinds []string) *recordingHost {
	withheld := make(map[string]bool, len(unsupportedKinds))
	for _, kind := range unsupportedKinds {
		withheld[kind] = true
	}
	return &recordingHost{inner: inner, unsupportedKinds: withheld}
}

// Mutations is a copy of the recorded resource-write timeline.
func (h *recordingHost) Mutations() []mutation {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]mutation(nil), h.mutations...)
}

// Reset clears the recorded timeline so one scenario step is measured alone.
func (h *recordingHost) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mutations = nil
}

func (h *recordingHost) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	if h.withholdSupport(w, request) {
		return
	}
	kind, name, recorded := resourceMutationTarget(request)
	if !recorded {
		h.inner.ServeHTTP(w, request)
		return
	}
	capture := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	h.inner.ServeHTTP(capture, request)
	h.mu.Lock()
	h.mutations = append(h.mutations, mutation{
		Method: request.Method, Kind: kind, Name: name, Status: capture.status,
	})
	h.mu.Unlock()
}

// withholdSupport answers the support routes of a withheld kind as the host
// that does not implement it would: `form_unknown` on the exact line, and an
// omission from the list. It reports whether it answered.
func (h *recordingHost) withholdSupport(w http.ResponseWriter, request *http.Request) bool {
	if len(h.unsupportedKinds) == 0 || request.Method != http.MethodGet {
		return false
	}
	parts := apiPathSegments(request.URL.Path)
	// {api}/support/forms/{group}[/{version}]/{kind}/{definitionVersion}
	if len(parts) < 5 || parts[0] != "support" || parts[1] != "forms" {
		return false
	}
	_, tail, ok := clientv3.SplitGroupPath(parts[2:])
	if !ok || len(tail) != 2 || !h.unsupportedKinds[tail[0]] {
		return false
	}
	writeHostError(w, http.StatusNotFound, "form_unknown",
		"this host implements no support profile for "+tail[0])
	return true
}

// resourceMutationTarget reports the kind and name of a resource write.
func resourceMutationTarget(request *http.Request) (string, string, bool) {
	if request.Method != http.MethodPut && request.Method != http.MethodDelete {
		return "", "", false
	}
	parts := apiPathSegments(request.URL.Path)
	// {api}/resources/{group}[/{version}]/{kind}/{name}
	//
	// The group is split by the client's own grammar, not by counting: a group
	// carries a version or does not (decision 0049), and a recorder that
	// counted five segments would silently record NOTHING for the versionless
	// shape — a timeline assertion passing on an empty timeline.
	if len(parts) < 4 || parts[0] != "resources" {
		return "", "", false
	}
	_, tail, ok := clientv3.SplitGroupPath(parts[1:])
	if !ok || len(tail) != 2 {
		return "", "", false
	}
	return tail[0], tail[1], true
}

func apiPathSegments(path string) []string {
	if !strings.HasPrefix(path, clientv3.APIRootPath+"/") {
		return nil
	}
	return strings.Split(strings.TrimPrefix(path, clientv3.APIRootPath+"/"), "/")
}

func writeHostError(w http.ResponseWriter, status int, code, message string) {
	body, _ := json.Marshal(map[string]any{"error": map[string]any{
		"code": code, "message": message, "requestId": "worker-authoring-harness", "retryable": false,
	}})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

package fakeruntime

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// routePrefix is where the probe protocol lives under the worker origin.
const routePrefix = "/abi/"

// WorkerHandler is the worker's `fetch` entry point: the host side that
// constructs a request, invokes the exported handler, and turns whatever
// happens into an HTTP response.
func (r *Runtime) WorkerHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		route := ""
		if marker := strings.Index(request.URL.Path, routePrefix); marker >= 0 {
			route = request.URL.Path[marker+len(routePrefix):]
		}
		r.invokeFetch(writer, request, route)
	})
}

// invokeFetch is the host half of the `fetch` operation. An uncaught throw in
// the module becomes a host-generated 500 here — never a hung request and
// never a truncated connection with no status.
func (r *Runtime) invokeFetch(writer http.ResponseWriter, request *http.Request, route string) {
	switch route {
	case "health":
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "ok": true})
	case "handler":
		// The module reports the arguments the HOST handed it.
		writeJSON(writer, http.StatusOK, map[string]any{
			"probe":               ProbeProtocol,
			"argumentCount":       3,
			"requestIsRequest":    true,
			"requestMethod":       request.Method,
			"envIsObject":         true,
			"waitUntilIsFunction": true,
		})
	case "throw":
		// The module threw. The host answers, the request completes.
		writeJSON(writer, http.StatusInternalServerError, map[string]any{
			"error": "the worker threw an uncaught exception",
		})
	case "env":
		names := append([]string(nil), r.environmentNames...)
		sort.Strings(names)
		writeJSON(writer, http.StatusOK, map[string]any{
			"probe": ProbeProtocol, "propertyNames": names,
		})
	case "globals":
		r.reportGlobals(writer, request)
	case "kv":
		r.serveKV(writer, request)
	case "echo-stream":
		r.echoRequestStream(writer, request)
	case "stream":
		r.streamResponse(writer, request)
	case "wait-until":
		r.serveWaitUntil(writer, request)
	case "scheduled":
		r.serveScheduledObservation(writer, request)
	case "queue":
		r.serveQueue(writer, request)
	case "tail":
		r.serveTailObservation(writer)
	default:
		writeJSON(writer, http.StatusNotFound, map[string]any{
			"probe": ProbeProtocol, "error": "unknown probe route", "route": route,
		})
	}
}

func (r *Runtime) reportGlobals(writer http.ResponseWriter, request *http.Request) {
	provided := map[string]bool{}
	for _, name := range providedGlobals {
		provided[name] = true
	}
	present := []string{}
	missing := []string{}
	for _, name := range strings.Split(request.URL.Query().Get("names"), ",") {
		if provided[name] {
			present = append(present, name)
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(present)
	sort.Strings(missing)
	writeJSON(writer, http.StatusOK, map[string]any{
		"probe": ProbeProtocol, "present": present, "missing": missing,
	})
}

// serveKV is the module calling the edge.kv binding the deployment declares.
// The bytes make a real round trip through the binding's own store.
func (r *Runtime) serveKV(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodPost {
		var body struct {
			Key         string `json:"key"`
			ValueBase64 string `json:"valueBase64"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		value, err := base64.StdEncoding.DecodeString(body.ValueBase64)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		r.kvPut(body.Key, value)
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "stored": true})
		return
	}
	value, ok := r.kvGet(request.URL.Query().Get("key"))
	if !ok {
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "found": false})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"probe": ProbeProtocol, "found": true,
		"valueBase64": base64.StdEncoding.EncodeToString(value),
	})
}

// echoRequestStream reads the request body incrementally and answers a line
// per read. A host that buffered the body could not answer the first line
// before the client sent the second chunk.
func (r *Runtime) echoRequestStream(writer http.ResponseWriter, request *http.Request) {
	// net/http drains an unread request body before sending headers, which
	// would deadlock a handler that answers the first chunk before the second
	// is sent. Full duplex is the Go server's name for the behaviour the ABI
	// requires of every conforming runtime.
	_ = http.NewResponseController(writer).EnableFullDuplex()
	writer.Header().Set("content-type", "application/x-ndjson")
	writer.WriteHeader(http.StatusOK)
	flusher, _ := writer.(http.Flusher)
	buffer := make([]byte, 64*1024)
	reads := 0
	for {
		read, err := request.Body.Read(buffer)
		if read > 0 {
			reads++
			writeLine(writer, map[string]any{
				"probe": ProbeProtocol, "read": reads, "bytes": read,
			})
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	writeLine(writer, map[string]any{"probe": ProbeProtocol, "end": true})
	if flusher != nil {
		flusher.Flush()
	}
}

// streamResponse produces its body over time. A host that buffered it would
// deliver every chunk at once.
func (r *Runtime) streamResponse(writer http.ResponseWriter, request *http.Request) {
	chunks, _ := strconv.Atoi(request.URL.Query().Get("chunks"))
	gapMillis, _ := strconv.Atoi(request.URL.Query().Get("gapMillis"))
	writer.Header().Set("content-type", "application/x-ndjson")
	writer.WriteHeader(http.StatusOK)
	flusher, _ := writer.(http.Flusher)
	for chunk := 1; chunk <= chunks; chunk++ {
		if chunk > 1 {
			time.Sleep(time.Duration(gapMillis) * time.Millisecond)
		}
		writeLine(writer, map[string]any{"probe": ProbeProtocol, "chunk": chunk})
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// serveWaitUntil registers two tasks through the invocation context: one that
// rejects, and one that records a marker after the response has been sent. The
// response is written immediately and never revisited.
func (r *Runtime) serveWaitUntil(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		nonce := request.URL.Query().Get("nonce")
		stored, ok := r.kvGet("wait-until:" + nonce)
		if !ok {
			writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "settled": false})
			return
		}
		var marker struct {
			AfterRejection bool   `json:"afterRejection"`
			Nonce          string `json:"nonce"`
		}
		_ = json.Unmarshal(stored, &marker)
		writeJSON(writer, http.StatusOK, map[string]any{
			"probe": ProbeProtocol, "settled": true,
			"afterRejection": marker.AfterRejection, "nonce": marker.Nonce,
		})
		return
	}
	var body struct {
		Nonce       string `json:"nonce"`
		DelayMillis int    `json:"delayMillis"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	// Task one rejects. It reaches host diagnostics and nothing else.
	r.holdIsolate(func() {
		r.mutex.Lock()
		r.rejectedTasks++
		r.mutex.Unlock()
	})
	// Task two settles after the response is already on the wire. The marker
	// carries the correlation value the RUNNER sent, so the observation names
	// the run that caused it.
	r.holdIsolate(func() {
		time.Sleep(time.Duration(body.DelayMillis) * time.Millisecond)
		marker, _ := json.Marshal(map[string]any{
			"afterRejection": true, "nonce": body.Nonce,
		})
		r.kvPut("wait-until:"+body.Nonce, marker)
	})
	writeJSON(writer, http.StatusOK, map[string]any{
		"probe": ProbeProtocol, "registeredTasks": 2, "responseSent": true,
	})
}

func (r *Runtime) serveScheduledObservation(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodDelete {
		r.kvDelete("scheduled:last")
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "cleared": true})
		return
	}
	stored, ok := r.kvGet("scheduled:last")
	if !ok {
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "observed": false})
		return
	}
	var marker struct {
		Cron                string `json:"cron"`
		ScheduledTimeMillis int64  `json:"scheduledTimeMillis"`
	}
	_ = json.Unmarshal(stored, &marker)
	writeJSON(writer, http.StatusOK, map[string]any{
		"probe": ProbeProtocol, "observed": true,
		"cron": marker.Cron, "scheduledTimeMillis": marker.ScheduledTimeMillis,
	})
}

// moduleScheduled is the module's exported `scheduled` handler: it records the
// expression and instant the host matched.
func (r *Runtime) moduleScheduled(cron string, scheduledTimeMillis int64) {
	marker, _ := json.Marshal(map[string]any{
		"cron": cron, "scheduledTimeMillis": scheduledTimeMillis,
	})
	r.kvPut("scheduled:last", marker)
}

// serveQueue submits through the edge.queue producer binding and reports what
// the host later delivered to the exported `queue` handler.
func (r *Runtime) serveQueue(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodPost {
		var body struct {
			Nonce         string `json:"nonce"`
			PayloadBase64 string `json:"payloadBase64"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if !r.exports["queue"] || r.deployment.Queue == "" {
			writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "sent": false})
			return
		}
		message, _ := json.Marshal(map[string]any{
			"nonce": body.Nonce, "payloadBase64": body.PayloadBase64,
		})
		r.acceptQueueMessage(message)
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "sent": true})
		return
	}
	stored, ok := r.kvGet("queue:" + request.URL.Query().Get("nonce"))
	if !ok {
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "delivered": false})
		return
	}
	var marker map[string]any
	_ = json.Unmarshal(stored, &marker)
	marker["probe"] = ProbeProtocol
	marker["delivered"] = true
	writeJSON(writer, http.StatusOK, marker)
}

// acceptQueueMessage is the host accepting a durable message and, later,
// delivering it once to the exported `queue` handler with a stable identity
// and a first-delivery attempt count of 1.
func (r *Runtime) acceptQueueMessage(body []byte) {
	r.mutex.Lock()
	r.queueSequence++
	identity := "message-" + strconv.Itoa(r.queueSequence)
	r.mutex.Unlock()
	delivered := r.holdIsolate(func() {
		time.Sleep(r.queueDelay)
		r.moduleQueue(r.deployment.Queue, identity, 1, body)
	})
	_ = delivered
}

// moduleQueue is the module's exported `queue` handler: it records the batch
// the host delivered, byte for byte.
func (r *Runtime) moduleQueue(queue, identity string, attempts int, body []byte) {
	var decoded struct {
		Nonce         string `json:"nonce"`
		PayloadBase64 string `json:"payloadBase64"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return
	}
	marker, _ := json.Marshal(map[string]any{
		"queue": queue, "attempts": attempts, "messageId": identity,
		"payloadBase64": decoded.PayloadBase64, "nonce": decoded.Nonce,
	})
	r.kvPut("queue:"+decoded.Nonce, marker)
}

// serveTailObservation reports trace deliveries. Nothing in the Edge Platform
// Family attaches a tail consumer, so a conforming deployment of this corpus
// observes none; the route exists so the day an attachment exists, the check
// that measures it has somewhere to read from.
func (r *Runtime) serveTailObservation(writer http.ResponseWriter) {
	r.mutex.Lock()
	deliveries := r.tailDeliveries
	r.mutex.Unlock()
	if deliveries == 0 {
		writeJSON(writer, http.StatusOK, map[string]any{"probe": ProbeProtocol, "observed": false})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{
		"probe": ProbeProtocol, "observed": true, "events": deliveries,
	})
}

func writeJSON(writer http.ResponseWriter, status int, body map[string]any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("content-type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(encoded)
}

func writeLine(writer io.Writer, body map[string]any) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return
	}
	_, _ = writer.Write(append(encoded, '\n'))
}

// Package fakeruntime is an in-process stand-in for an ES Module Worker
// runtime implementing `worker.runtime@1.0.0`.
//
// It exists for ONE purpose: to exercise conformance/runtime-abi-v1 on every
// `bun run check`, so the corpus and its runner are known to work before an
// operator points them at a real deployment. It is not a runtime and a run
// against it is not evidence about any runtime — this repository has no
// JavaScript engine, so the stand-in reimplements the probe module's behaviour
// in Go rather than executing its bytes.
//
// What keeps the stand-in honest is what it is allowed to see. It is
// constructed from a workerbundle.Deployment and nothing else, and its loader
// answers only from the module bytes it is handed on the wire: it never
// receives the conformance contract, a check, or an expected observation, so
// it cannot answer from the answer key. Everything it reports is computed the
// way a runtime would have to compute it — `env` from the declaration, the
// exported handler set from the module's own bytes, the globals floor from
// what this implementation claims to provide, streams from real incremental
// reads and writes, and deferred work from a real goroutine that outlives the
// response.
package fakeruntime

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/workerbundle"
)

// ProbeProtocol is the observation protocol this stand-in serves, matching the
// behaviour of the corpus probe module route for route.
const ProbeProtocol = "takoform.runtime-abi-probe@v1"

// LoaderProtocol is the module-loader protocol this stand-in serves.
const LoaderProtocol = "takoform.runtime-abi-loader@v1"

// providedGlobals is the Web platform surface THIS implementation claims to
// provide. It is written here, by the implementation, and compared by the
// runner against the floor the corpus states; deriving it from the corpus
// would make the globals check assert nothing.
var providedGlobals = []string{
	"AbortController",
	"AbortSignal",
	"Headers",
	"ReadableStream",
	"Request",
	"Response",
	"TextDecoder",
	"TextEncoder",
	"URL",
	"URLSearchParams",
	"WritableStream",
	"clearInterval",
	"clearTimeout",
	"crypto.getRandomValues",
	"crypto.subtle.digest",
	"fetch",
	"setInterval",
	"setTimeout",
	"structuredClone",
}

// Options tunes the parts of the stand-in a real host decides for itself.
type Options struct {
	// CronPeriod is how often the simulated Worker Cron Trigger attachment
	// fires. A real host fires on the schedule's own minute boundary; the
	// stand-in fires far more often so a self-test is not a minute long.
	CronPeriod time.Duration
	// QueueDelay is how long an accepted message waits before the simulated
	// Queue Consumer attachment delivers it.
	QueueDelay time.Duration
}

// Runtime is one loaded, running worker plus the module loader beside it.
type Runtime struct {
	deployment       workerbundle.Deployment
	environmentNames []string
	exports          map[string]bool
	queueDelay       time.Duration

	mutex   sync.Mutex
	kv      map[string][]byte
	closed  bool
	pending sync.WaitGroup
	stop    chan struct{}

	rejectedTasks  int
	queueSequence  int
	cronInvoked    int
	tailDeliveries int
}

// New loads one deployment. It refuses a deployment the ABI itself forbids —
// an `env` namespace collision, or a declared handler the module bytes do not
// export — because a runtime that accepted either could not implement the
// contract afterwards.
func New(deployment workerbundle.Deployment, options Options) (*Runtime, error) {
	names, err := deployment.EnvironmentPropertyNames()
	if err != nil {
		return nil, err
	}
	main, ok := deployment.Bundle.Module(deployment.Bundle.MainModule)
	if !ok {
		return nil, fmt.Errorf(
			"%s: the deployment bundle carries no module named %q",
			workerbundle.ErrorModuleNotFound, deployment.Bundle.MainModule,
		)
	}
	exported, err := workerbundle.DeriveExportedHandlers(main.Source, workerbundle.HandlerVocabulary)
	if err != nil {
		return nil, err
	}
	exports := map[string]bool{}
	for _, handler := range exported {
		exports[handler] = true
	}
	for _, handler := range deployment.DeclaredHandlers {
		if !exports[handler] {
			return nil, fmt.Errorf(
				"%s: the deployment declares %q, which %s does not export",
				workerbundle.ErrorHandlerNotExported, handler, deployment.Bundle.MainModule,
			)
		}
	}
	if !exports["fetch"] {
		return nil, errors.New("the runtime conformance deployment must export fetch")
	}
	if options.CronPeriod <= 0 {
		options.CronPeriod = 25 * time.Millisecond
	}
	if options.QueueDelay <= 0 {
		options.QueueDelay = 10 * time.Millisecond
	}
	runtime := &Runtime{
		deployment:       deployment,
		environmentNames: names,
		exports:          exports,
		queueDelay:       options.QueueDelay,
		kv:               map[string][]byte{},
		stop:             make(chan struct{}),
	}
	if exports["scheduled"] && deployment.Cron != "" {
		runtime.startCron(options.CronPeriod)
	}
	return runtime, nil
}

// Close reclaims the isolate. Work already registered through `waitUntil`, and
// deliveries already accepted, are allowed to settle first — that is the whole
// obligation the `waitUntil` operation states.
func (r *Runtime) Close() {
	r.mutex.Lock()
	if r.closed {
		r.mutex.Unlock()
		return
	}
	r.closed = true
	r.mutex.Unlock()
	close(r.stop)
	r.pending.Wait()
}

// holdIsolate registers host-held work the isolate stays alive for.
func (r *Runtime) holdIsolate(task func()) bool {
	r.mutex.Lock()
	if r.closed {
		r.mutex.Unlock()
		return false
	}
	r.pending.Add(1)
	r.mutex.Unlock()
	go func() {
		defer r.pending.Done()
		task()
	}()
	return true
}

func (r *Runtime) startCron(period time.Duration) {
	r.pending.Add(1)
	go func() {
		defer r.pending.Done()
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case now := <-ticker.C:
				r.invokeScheduled(now)
			}
		}
	}()
}

// invokeScheduled is the host side of the `scheduled` operation: it hands the
// exported handler the matched expression and the UTC instant, and the module
// records what it received.
func (r *Runtime) invokeScheduled(now time.Time) {
	r.mutex.Lock()
	r.cronInvoked++
	r.mutex.Unlock()
	r.moduleScheduled(r.deployment.Cron, now.UnixMilli())
}

func (r *Runtime) kvPut(key string, value []byte) {
	stored := make([]byte, len(value))
	copy(stored, value)
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.kv[key] = stored
}

func (r *Runtime) kvGet(key string) ([]byte, bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	value, ok := r.kv[key]
	if !ok {
		return nil, false
	}
	stored := make([]byte, len(value))
	copy(stored, value)
	return stored, true
}

func (r *Runtime) kvDelete(key string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	delete(r.kv, key)
}

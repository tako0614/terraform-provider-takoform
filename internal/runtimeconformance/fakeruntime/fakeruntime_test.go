package fakeruntime

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/workerbundle"
)

const probeSource = `
const handlers = {
  async fetch(request, env, ctx) { return new Response("ok"); },
  async scheduled(event, env) {},
  async queue(batch, env) {},
};
export default handlers;
`

const fetchOnlySource = `
const handlers = {
  async fetch(request) { return new Response("ok"); },
};
export default handlers;
`

func module(name, mediaType, source string) workerbundle.Module {
	return workerbundle.Module{Name: name, MediaType: mediaType, Source: []byte(source)}
}

func loadRequest(mainModule string, declared []string, modules ...workerbundle.Module) LoadRequest {
	wire := make([]LoadRequestModule, 0, len(modules))
	for _, entry := range modules {
		wire = append(wire, LoadRequestModule{
			Name:          entry.Name,
			MediaType:     entry.MediaType,
			ContentBase64: base64.StdEncoding.EncodeToString(entry.Source),
		})
	}
	return LoadRequest{
		Protocol: LoaderProtocol, MainModule: mainModule,
		DeclaredHandlers: declared, Modules: wire,
	}
}

// TestLoadModuleAnswersFromTheBytesItIsHanded pins the whole closed error
// vocabulary of the operation and the success case beside it.
func TestLoadModuleAnswersFromTheBytesItIsHanded(t *testing.T) {
	probe := module("index.js", "application/javascript+module", probeSource)
	fetchOnly := module("fetch-only.js", "application/javascript+module", fetchOnlySource)
	broken := module("broken.js", "application/javascript+module", "export default {\n  fetch(){\n")
	page := module("page.html", "text/html", "<p>no</p>")

	exported, failure := LoadModule(loadRequest("index.js", workerbundle.HandlerVocabulary, probe))
	if failure != nil {
		t.Fatalf("the probe module must load: %+v", failure)
	}
	if strings.Join(exported, ",") != "fetch,scheduled,queue" {
		t.Fatalf("exports = %v", exported)
	}

	for name, testCase := range map[string]struct {
		request LoadRequest
		code    string
	}{
		"missing main module": {loadRequest("worker.js", []string{"fetch"}, probe), workerbundle.ErrorModuleNotFound},
		"unsupported media":   {loadRequest("index.js", []string{"fetch"}, probe, page), workerbundle.ErrorUnsupportedMedia},
		"unparseable":         {loadRequest("broken.js", []string{"fetch"}, broken), workerbundle.ErrorModuleSyntax},
		"handler not exported": {
			loadRequest("fetch-only.js", []string{"fetch", "scheduled"}, fetchOnly),
			workerbundle.ErrorHandlerNotExported,
		},
	} {
		_, failure := LoadModule(testCase.request)
		if failure == nil {
			t.Fatalf("%s: expected %s", name, testCase.code)
		}
		if failure.Code != testCase.code {
			t.Fatalf("%s: got %s, want %s", name, failure.Code, testCase.code)
		}
	}
}

// TestNewRefusesADeploymentTheABIForbids proves the stand-in fails closed on
// the two states a conforming runtime could not implement afterwards.
func TestNewRefusesADeploymentTheABIForbids(t *testing.T) {
	fetchOnly := workerbundle.Bundle{
		Name: "fetch-only", MainModule: "fetch-only.js",
		Modules: []workerbundle.Module{module("fetch-only.js", "application/javascript+module", fetchOnlySource)},
	}
	_, err := New(workerbundle.Deployment{
		Bundle: fetchOnly, DeclaredHandlers: []string{"fetch", "queue"},
	}, Options{})
	if err == nil || !strings.Contains(err.Error(), workerbundle.ErrorHandlerNotExported) {
		t.Fatalf("expected handler_not_exported, got %v", err)
	}

	probe := workerbundle.Bundle{
		Name: "probe", MainModule: "index.js",
		Modules: []workerbundle.Module{module("index.js", "application/javascript+module", probeSource)},
	}
	_, err = New(workerbundle.Deployment{
		Bundle: probe, DeclaredHandlers: []string{"fetch"},
		Vars:     []string{"CACHE"},
		Bindings: []workerbundle.Binding{{Name: "CACHE", Interface: "edge.kv"}},
	}, Options{})
	if err == nil || !strings.Contains(err.Error(), workerbundle.ErrorEnvironmentConflict) {
		t.Fatalf("expected environment_name_collision, got %v", err)
	}
}

// TestTheStandInProjectsExactlyWhatItWasDeclared proves the stand-in computes
// `env` from the deployment rather than from anything the corpus told it.
func TestTheStandInProjectsExactlyWhatItWasDeclared(t *testing.T) {
	probe := workerbundle.Bundle{
		Name: "probe", MainModule: "index.js",
		Modules: []workerbundle.Module{module("index.js", "application/javascript+module", probeSource)},
	}
	runtime, err := New(workerbundle.Deployment{
		Bundle: probe, DeclaredHandlers: []string{"fetch"},
		Vars: []string{"LOG_LEVEL"}, SensitiveVars: []string{"TOKEN"},
		Bindings: []workerbundle.Binding{{Name: "CACHE", Interface: "edge.kv"}},
	}, Options{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer runtime.Close()
	if strings.Join(runtime.environmentNames, ",") != "CACHE,LOG_LEVEL,TOKEN" {
		t.Fatalf("env names = %v", runtime.environmentNames)
	}
}

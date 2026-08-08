package fakeruntime

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/workerbundle"
)

// LoadRequest is the `takoform.runtime-abi-loader@v1` wire body: one bundle,
// the module whose default export carries the handlers, and the handler set
// the Worker Version declares. The modules arrive as bytes, so a loader
// answers from what it was handed rather than from anything it stored earlier.
type LoadRequest struct {
	Protocol         string              `json:"protocol"`
	MainModule       string              `json:"mainModule"`
	DeclaredHandlers []string            `json:"declaredHandlers"`
	Modules          []LoadRequestModule `json:"modules"`
}

// LoadRequestModule is one module of a load request.
type LoadRequestModule struct {
	Name          string `json:"name"`
	MediaType     string `json:"mediaType"`
	ContentBase64 string `json:"contentBase64"`
}

// LoadResponse is the loader's answer: either the exported handler set, or one
// code from the closed loadModule error vocabulary.
type LoadResponse struct {
	Protocol         string     `json:"protocol"`
	ExportedHandlers []string   `json:"exportedHandlers,omitempty"`
	Error            *LoadError `json:"error,omitempty"`
}

// LoadError is one closed loadModule failure.
type LoadError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LoaderHandler is the module loader beside the running worker: the surface a
// real runtime exposes through a disposable adapter so a conformance run can
// hand it bytes and read back what it made of them. It is not part of the
// worker.runtime ABI and must never be exposed by a production deployment.
func (r *Runtime) LoaderHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writeLoadError(writer, http.StatusMethodNotAllowed,
				workerbundle.ErrorModuleNotFound, "the loader accepts POST only")
			return
		}
		var body LoadRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			writeLoadError(writer, http.StatusBadRequest,
				workerbundle.ErrorModuleSyntax, err.Error())
			return
		}
		exported, failure := LoadModule(body)
		if failure != nil {
			writeLoadError(writer, http.StatusUnprocessableEntity, failure.Code, failure.Message)
			return
		}
		encoded, err := json.Marshal(LoadResponse{
			Protocol: LoaderProtocol, ExportedHandlers: exported,
		})
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("content-type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(encoded)
	})
}

// LoadModule is the whole of the stand-in's `loadModule` operation, in the
// order the contract states it: the main module must be in the bundle, every
// module's media type must be in the closed loadable set, the main module's
// bytes must present a derivable default export, and every DECLARED handler
// must be one the bytes actually export.
func LoadModule(request LoadRequest) ([]string, *LoadError) {
	var main []byte
	found := false
	for _, module := range request.Modules {
		if !workerbundle.IsLoadableMediaType(module.MediaType) {
			return nil, &LoadError{
				Code:    workerbundle.ErrorUnsupportedMedia,
				Message: "module " + module.Name + " declares media type " + module.MediaType,
			}
		}
		if module.Name != request.MainModule {
			continue
		}
		decoded, err := base64.StdEncoding.DecodeString(module.ContentBase64)
		if err != nil {
			return nil, &LoadError{
				Code:    workerbundle.ErrorModuleSyntax,
				Message: "module " + module.Name + " content is not base64",
			}
		}
		main = decoded
		found = true
	}
	if !found {
		return nil, &LoadError{
			Code:    workerbundle.ErrorModuleNotFound,
			Message: "the bundle carries no module named " + request.MainModule,
		}
	}
	exported, err := workerbundle.DeriveExportedHandlers(main, workerbundle.HandlerVocabulary)
	if err != nil {
		if errors.Is(err, workerbundle.ErrModuleSyntax) {
			return nil, &LoadError{
				Code: workerbundle.ErrorModuleSyntax, Message: err.Error(),
			}
		}
		return nil, &LoadError{Code: workerbundle.ErrorModuleSyntax, Message: err.Error()}
	}
	exports := map[string]bool{}
	for _, handler := range exported {
		exports[handler] = true
	}
	for _, handler := range request.DeclaredHandlers {
		if !exports[handler] {
			return nil, &LoadError{
				Code:    workerbundle.ErrorHandlerNotExported,
				Message: "the version declares " + handler + ", which " + request.MainModule + " does not export",
			}
		}
	}
	return exported, nil
}

func writeLoadError(writer http.ResponseWriter, status int, code, message string) {
	encoded, err := json.Marshal(LoadResponse{
		Protocol: LoaderProtocol, Error: &LoadError{Code: code, Message: message},
	})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	writer.Header().Set("content-type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(encoded)
}

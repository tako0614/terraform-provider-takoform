// Command edge-form-source renders the independently authored Edge Platform
// Family Form Definitions, Interface Definitions, Binding Definitions, and
// fixtures consumed by the deterministic family candidate builder
// (scripts/current-form-families.mjs).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

type sourceDocument struct {
	Family string `json:"family"`
	// Forms carry the exact indented definition.json text so large integer
	// schema values survive the JavaScript pipeline without float64 rounding.
	Forms      []edgeformcatalog.RenderedForm     `json:"forms"`
	Interfaces []edgeformcatalog.RenderedContract `json:"interfaces"`
	Bindings   []edgeformcatalog.RenderedContract `json:"bindings"`
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: edge-form-source")
		os.Exit(2)
	}
	forms, err := edgeformcatalog.RenderForms()
	if err != nil {
		fail(err)
	}
	interfaces, err := edgeformcatalog.RenderInterfaces()
	if err != nil {
		fail(err)
	}
	bindings, err := edgeformcatalog.RenderBindings()
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(sourceDocument{
		Family:     edgeformcatalog.Family.APIVersion(),
		Forms:      forms,
		Interfaces: interfaces,
		Bindings:   bindings,
	}); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "edge-form-source:", err)
	os.Exit(1)
}

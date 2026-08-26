// Command current-form-source renders every current versionless Form Family
// and the global Interface and Binding candidate sets. Families are rendered
// in dependency order so cross-family Interface identities are injected from
// their owning source rather than copied or resolved through a latest alias.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/tako0614/terraform-provider-takoform/internal/containerformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/functionformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/scheduleformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/tableformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/topicformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/vectorformcatalog"
)

type sourceDocument struct {
	PackageAPIVersion        string           `json:"packageApiVersion"`
	FamilyIndexFormat        string           `json:"familyIndexFormat"`
	FormMaturity             string           `json:"formMaturity"`
	PublicationStatus        string           `json:"publicationStatus"`
	InterfaceAuthoringSource string           `json:"interfaceAuthoringSource"`
	BindingAuthoringSource   string           `json:"bindingAuthoringSource"`
	Families                 []sourceFamily   `json:"families"`
	Interfaces               []sourceContract `json:"interfaces"`
	Bindings                 []sourceContract `json:"bindings"`
}

type sourceFamily struct {
	Group           string          `json:"group"`
	AuthoringSource string          `json:"authoringSource"`
	AuthoringPolicy string          `json:"authoringPolicy"`
	Forms           json.RawMessage `json:"forms"`
}

type sourceContract struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	DefinitionJSON string `json:"definitionJson"`
	SchemaDigest   string `json:"schemaDigest"`
}

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: current-form-source")
		os.Exit(2)
	}
	document, err := renderSource()
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(document); err != nil {
		fail(err)
	}
}

func renderSource() (sourceDocument, error) {
	document := sourceDocument{
		PackageAPIVersion:        "packages.forms.takoform.com/v1alpha5",
		FamilyIndexFormat:        "takoform.current-family-index@v1",
		FormMaturity:             "experimental",
		PublicationStatus:        "unpublished",
		InterfaceAuthoringSource: "cmd/current-form-source",
		BindingAuthoringSource:   "cmd/current-form-source",
	}
	appendFamily := func(group, authoringSource string, forms any) error {
		raw, err := json.Marshal(forms)
		if err != nil {
			return fmt.Errorf("marshal %s source Forms: %w", group, err)
		}
		document.Families = append(document.Families, sourceFamily{
			Group:           group,
			AuthoringSource: authoringSource,
			AuthoringPolicy: "service-shape-preserving-contract",
			Forms:           raw,
		})
		return nil
	}
	appendContracts := func(destination *[]sourceContract, contracts any) error {
		raw, err := json.Marshal(contracts)
		if err != nil {
			return err
		}
		var decoded []sourceContract
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return err
		}
		*destination = append(*destination, decoded...)
		return nil
	}

	// Independent families render before the three-family Queue -> Topic ->
	// Schedule chain. The order is part of this source command's contract; the
	// published current-family index separately sorts its closed records by
	// group as required by its v1 format.
	edgeForms, err := edgeformcatalog.RenderForms()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Edge Forms: %w", err)
	}
	if err := appendFamily(edgeformcatalog.Family.APIVersion(), "internal/edgeformcatalog", edgeForms); err != nil {
		return sourceDocument{}, err
	}
	edgeInterfaces, err := edgeformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Edge Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, edgeInterfaces); err != nil {
		return sourceDocument{}, err
	}
	edgeBindings, err := edgeformcatalog.RenderBindings()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Edge Bindings: %w", err)
	}
	if err := appendContracts(&document.Bindings, edgeBindings); err != nil {
		return sourceDocument{}, err
	}

	functionForms, err := functionformcatalog.RenderForms()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Function Forms: %w", err)
	}
	if err := appendFamily(functionformcatalog.Family.APIVersion(), "internal/functionformcatalog", functionForms); err != nil {
		return sourceDocument{}, err
	}
	functionInterfaces, err := functionformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Function Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, functionInterfaces); err != nil {
		return sourceDocument{}, err
	}

	containerForms, err := containerformcatalog.RenderForms()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Container Forms: %w", err)
	}
	if err := appendFamily(containerformcatalog.Family.APIVersion(), "internal/containerformcatalog", containerForms); err != nil {
		return sourceDocument{}, err
	}
	containerInterfaces, err := containerformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Container Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, containerInterfaces); err != nil {
		return sourceDocument{}, err
	}

	queueForms, err := queueformcatalog.RenderForms()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Queue Forms: %w", err)
	}
	if err := appendFamily(queueformcatalog.Family.APIVersion(), "internal/queueformcatalog", queueForms); err != nil {
		return sourceDocument{}, err
	}
	queueInterfaces, err := queueformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Queue Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, queueInterfaces); err != nil {
		return sourceDocument{}, err
	}
	queueRef, err := queueformcatalog.InterfaceRefFor(queueformcatalog.QueuePullInterfaceName, "1.0.0")
	if err != nil {
		return sourceDocument{}, fmt.Errorf("resolve Queue Interface: %w", err)
	}

	tableForms, err := tableformcatalog.RenderForms()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Table Forms: %w", err)
	}
	if err := appendFamily(tableformcatalog.Family.APIVersion(), "internal/tableformcatalog", tableForms); err != nil {
		return sourceDocument{}, err
	}
	tableInterfaces, err := tableformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Table Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, tableInterfaces); err != nil {
		return sourceDocument{}, err
	}

	vectorForms, err := vectorformcatalog.RenderForms()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Vector Forms: %w", err)
	}
	if err := appendFamily(vectorformcatalog.Family.APIVersion(), "internal/vectorformcatalog", vectorForms); err != nil {
		return sourceDocument{}, err
	}
	vectorInterfaces, err := vectorformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Vector Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, vectorInterfaces); err != nil {
		return sourceDocument{}, err
	}

	topicForms, err := topicformcatalog.RenderForms(queueRef)
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Topic Forms: %w", err)
	}
	if err := appendFamily(topicformcatalog.Family.APIVersion(), "internal/topicformcatalog", topicForms); err != nil {
		return sourceDocument{}, err
	}
	topicInterfaces, err := topicformcatalog.RenderInterfaces()
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Topic Interfaces: %w", err)
	}
	if err := appendContracts(&document.Interfaces, topicInterfaces); err != nil {
		return sourceDocument{}, err
	}
	topicRef, err := topicformcatalog.InterfaceRefFor(topicformcatalog.TopicPublishInterfaceName, "1.0.0")
	if err != nil {
		return sourceDocument{}, fmt.Errorf("resolve Topic Interface: %w", err)
	}

	scheduleForms, err := scheduleformcatalog.RenderForms(queueRef, topicRef)
	if err != nil {
		return sourceDocument{}, fmt.Errorf("render Schedule Forms: %w", err)
	}
	if err := appendFamily(scheduleformcatalog.Family.APIVersion(), "internal/scheduleformcatalog", scheduleForms); err != nil {
		return sourceDocument{}, err
	}

	// Independent families can render on either side of the Queue -> Topic ->
	// Schedule chain. The source document uses the canonical family dependency
	// order recorded by decision 0043, which is also the generator's closed
	// authoring order. The public index later sorts by group as a separate wire
	// requirement.
	familyOrder := map[string]int{
		edgeformcatalog.Family.APIVersion():      0,
		functionformcatalog.Family.APIVersion():  1,
		containerformcatalog.Family.APIVersion(): 2,
		tableformcatalog.Family.APIVersion():     3,
		queueformcatalog.Family.APIVersion():     4,
		topicformcatalog.Family.APIVersion():     5,
		scheduleformcatalog.Family.APIVersion():  6,
		vectorformcatalog.Family.APIVersion():    7,
	}
	sort.Slice(document.Families, func(left, right int) bool {
		return familyOrder[document.Families[left].Group] < familyOrder[document.Families[right].Group]
	})
	for _, contracts := range []*[]sourceContract{&document.Interfaces, &document.Bindings} {
		if err := sortExactContracts(*contracts); err != nil {
			return sourceDocument{}, err
		}
	}
	return document, nil
}

func sortExactContracts(contracts []sourceContract) error {
	sort.Slice(contracts, func(left, right int) bool {
		if contracts[left].Name != contracts[right].Name {
			return contracts[left].Name < contracts[right].Name
		}
		return contracts[left].Version < contracts[right].Version
	})
	for index := 1; index < len(contracts); index++ {
		previous, current := contracts[index-1], contracts[index]
		if previous.Name == current.Name && previous.Version == current.Version {
			return fmt.Errorf("global contract set duplicates exact identity %s@%s", current.Name, current.Version)
		}
	}
	return nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "current-form-source:", err)
	os.Exit(1)
}

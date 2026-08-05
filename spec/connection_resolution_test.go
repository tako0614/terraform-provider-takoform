package spec

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
)

type connectionResolutionContract struct {
	SourceSpace string `json:"sourceSpace"`
	Reference   string `json:"reference"`
	CrossSpace  string `json:"crossSpace"`
	Missing     string `json:"missing"`
}

type connectionProbeContract struct {
	SourceName     string `json:"sourceName"`
	SourceIdentity struct {
		FormRef       formpackage.FormRef `json:"formRef"`
		PackageDigest string              `json:"packageDigest"`
	} `json:"sourceIdentity"`
	Desired    map[string]any `json:"desired"`
	TargetKind string         `json:"targetKind"`
	TargetName string         `json:"targetName"`
}

func TestConnectionResolutionContractsPinSourceResourceSpace(t *testing.T) {
	t.Parallel()

	type document struct {
		ConnectionResolution connectionResolutionContract `json:"connectionResolution"`
	}
	host := readFile[document](t, "host-api", "operations.json")
	conformance := readFile[document](
		t,
		"..",
		"conformance",
		"portable-host-v2",
		"contract.json",
	)
	want := connectionResolutionContract{
		SourceSpace: "resource.metadata.space",
		Reference:   "Kind/name",
		CrossSpace:  "unrepresentable",
		Missing:     "resource_not_found/404-before-mutation",
	}
	if !reflect.DeepEqual(host.ConnectionResolution, want) {
		t.Errorf("host Connection resolution = %#v, want %#v", host.ConnectionResolution, want)
	}
	if !reflect.DeepEqual(conformance.ConnectionResolution, want) {
		t.Errorf(
			"portable-host Connection resolution = %#v, want %#v",
			conformance.ConnectionResolution,
			want,
		)
	}
}

func TestConnectionProbePinsARealRequiredConnectionForm(t *testing.T) {
	t.Parallel()

	type document struct {
		RunnerInput struct {
			ConnectionProbe connectionProbeContract `json:"connectionProbe"`
		} `json:"runnerInput"`
	}
	contract := readFile[document](
		t,
		"..",
		"conformance",
		"portable-host-v2",
		"contract.json",
	)
	probe := contract.RunnerInput.ConnectionProbe
	if probe.SourceName == "" || probe.TargetKind == "" || probe.TargetName == "" {
		t.Fatalf("Connection probe has incomplete identity: %#v", probe)
	}
	if probe.SourceIdentity.FormRef.Kind != "Schedule" {
		t.Fatalf("Connection probe source Form = %#v", probe.SourceIdentity.FormRef)
	}

	inventory := readFile[currentCandidateInventory](t, "..", "forms", "candidates", "v1alpha2", "candidate-set.json")
	var identityMatches bool
	for _, entry := range inventory.Forms {
		if entry.Kind != probe.SourceIdentity.FormRef.Kind {
			continue
		}
		identityMatches = reflect.DeepEqual(entry.FormRef, probe.SourceIdentity.FormRef) &&
			entry.PackageDigest == probe.SourceIdentity.PackageDigest
	}
	if !identityMatches {
		t.Fatalf("portable-host v2 Connection probe does not pin the exact current Schedule package: %#v", probe.SourceIdentity)
	}

	if probe.Desired["name"] != probe.SourceName {
		t.Fatalf("Connection source name = %#v, want %q", probe.Desired["name"], probe.SourceName)
	}
	connections, ok := probe.Desired["connections"].(map[string]any)
	if !ok || len(connections) != 1 {
		t.Fatalf("Connection probe desired connections = %#v", probe.Desired["connections"])
	}
	store, ok := connections["invocation"].(map[string]any)
	if !ok {
		t.Fatalf("Connection probe invocation = %#v", connections["invocation"])
	}
	wantTarget := fmt.Sprintf("%s/%s", probe.TargetKind, probe.TargetName)
	if store["resource"] != wantTarget {
		t.Fatalf("Connection probe target = %#v, want %q", store["resource"], wantTarget)
	}
	if _, selectable := store["space"]; selectable {
		t.Fatalf("Connection probe unexpectedly carries a Space selector: %#v", store)
	}

	kind, ok := currentformcatalog.ByKind(probe.SourceIdentity.FormRef.Kind)
	if !ok {
		t.Fatalf("Connection probe Form %q is not implemented by the provider", probe.SourceIdentity.FormRef.Kind)
	}
	if err := kind.ValidateDesired(probe.Desired); err != nil {
		t.Fatalf("Connection probe desired state is not valid for its exact Form: %v", err)
	}
}

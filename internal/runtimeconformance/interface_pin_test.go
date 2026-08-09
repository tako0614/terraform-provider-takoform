package runtimeconformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/runtimeconformance/workerbundle"
)

// interfaceDefinition is the fraction of the ABI contract this lane reads
// back: the closed handler vocabulary, and the globals floor its fixture
// states.
type interfaceDefinition struct {
	Operations []struct {
		Name        string `json:"name"`
		InputSchema struct {
			Properties struct {
				DeclaredHandlers struct {
					Items struct {
						Enum []string `json:"enum"`
					} `json:"items"`
				} `json:"declaredHandlers"`
			} `json:"properties"`
		} `json:"inputSchema"`
	} `json:"operations"`
	Fixtures []struct {
		Name  string `json:"name"`
		Steps []struct {
			Operation string `json:"operation"`
			Expected  struct {
				Present []string `json:"present"`
			} `json:"expected"`
		} `json:"steps"`
	} `json:"fixtures"`
}

func readInterfaceDefinition(t *testing.T, contract Contract) (interfaceDefinition, []byte) {
	t.Helper()
	return readCandidateDefinition(t, contract, InterfaceName)
}

func readCandidateDefinition(t *testing.T, contract Contract, name string) (interfaceDefinition, []byte) {
	t.Helper()
	repositoryRoot, err := filepath.Abs(filepath.Join(contract.Root(), "..", ".."))
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	path := filepath.Join(
		repositoryRoot, "interfaces", "candidates", "v1alpha1", name, "definition.json",
	)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var definition interfaceDefinition
	if err := json.Unmarshal(raw, &definition); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return definition, raw
}

// TestTheCorpusVocabularyIsTheContractVocabulary keeps the closed handler set
// with one source of truth. The ABI's loadModule enum IS the vocabulary; a
// corpus that measured a different one would be measuring a different ABI.
func TestTheCorpusVocabularyIsTheContractVocabulary(t *testing.T) {
	contract := verifiedContract(t)
	definition, _ := readInterfaceDefinition(t, contract)
	for _, operation := range definition.Operations {
		if operation.Name != "loadModule" {
			continue
		}
		enum := operation.InputSchema.Properties.DeclaredHandlers.Items.Enum
		if !reflect.DeepEqual(enum, contract.HandlerVocabulary) {
			t.Fatalf("the corpus measures %v; the contract declares %v", contract.HandlerVocabulary, enum)
		}
		return
	}
	t.Fatalf("the ABI contract declares no loadModule operation")
}

// TestTheCorpusGlobalsFloorIsTheContractFloor keeps the floor the runner sends
// equal to the one the ABI's own fixture states.
func TestTheCorpusGlobalsFloorIsTheContractFloor(t *testing.T) {
	contract := verifiedContract(t)
	definition, _ := readInterfaceDefinition(t, contract)
	for _, fixture := range definition.Fixtures {
		for _, step := range fixture.Steps {
			if step.Operation != "globals" {
				continue
			}
			if !reflect.DeepEqual(step.Expected.Present, contract.GlobalsFloor) {
				t.Fatalf(
					"the corpus measures the floor %v; the contract states %v",
					contract.GlobalsFloor, step.Expected.Present,
				)
			}
			return
		}
	}
	t.Fatalf("the ABI contract carries no globals fixture to read the floor from")
}

// TestCorpusMeasuresTheCommittedInterfaceBytes is the coupling that makes the
// corpus's InterfaceRef an identity rather than a label. The contract states
// the exact `schemaDigest` it measures; this test recomputes it from the
// candidate Interface Definition in the repository. If the ABI's bytes move,
// this fails and names the re-pin, rather than leaving a corpus quietly
// measuring a contract that no longer exists.
//
// The digest is data in the corpus, not a lookup at run time, because a
// published corpus must be verifiable by a runtime operator who has the corpus
// and nothing else.
func TestCorpusMeasuresTheCommittedInterfaceBytes(t *testing.T) {
	contract := verifiedContract(t)
	_, raw := readInterfaceDefinition(t, contract)
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatalf("digest the ABI contract: %v", err)
	}
	if digest != contract.Interface.SchemaDigest {
		t.Fatalf(
			"conformance/runtime-abi-v1 measures %s but the committed worker.runtime definition "+
				"canonicalizes to %s; re-pin the corpus interface.schemaDigest and the manifest sha256",
			contract.Interface.SchemaDigest, digest,
		)
	}
}

// TestCorpusMeasuresTheCommittedServiceInterfaceBytes is the same coupling for
// the second contract this corpus reaches.
//
// The two service checks are claims about worker.service@1.0.0's delivery
// model, and that model is exactly what changed when the Interface stopped
// carrying bodies as JSON strings. A corpus that named the contract without
// pinning its bytes could go on asserting a streaming model against a
// definition that had reverted to buffering.
func TestCorpusMeasuresTheCommittedServiceInterfaceBytes(t *testing.T) {
	contract := verifiedContract(t)
	_, raw := readCandidateDefinition(t, contract, ServiceInterfaceName)
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatalf("digest the worker-to-worker contract: %v", err)
	}
	if digest != contract.ServiceInterface.SchemaDigest {
		t.Fatalf(
			"conformance/runtime-abi-v1 measures %s but the committed worker.service definition "+
				"canonicalizes to %s; re-pin the corpus serviceInterface.schemaDigest and the manifest sha256",
			contract.ServiceInterface.SchemaDigest, digest,
		)
	}
}

// TestTheServiceChecksAddressTheBindingTheDeploymentDeclares keeps the two
// worker-to-worker checks pointed at a binding an operator actually deploys. A
// check whose route calls `env.PEER` against a deployment that declares no such
// binding is a check every conforming runtime fails.
func TestTheServiceChecksAddressTheBindingTheDeploymentDeclares(t *testing.T) {
	contract := verifiedContract(t)
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}
	binding, ok := deployment.BindingNamed(contract.ProbeProtocol.ServiceBinding)
	if !ok || binding.Interface != ServiceInterfaceName {
		t.Fatalf("the deployment declares no %s binding named %q",
			ServiceInterfaceName, contract.ProbeProtocol.ServiceBinding)
	}
	if deployment.Peer == nil {
		t.Fatal("the corpus states no peer for the service binding to address")
	}
	for _, name := range serviceBindingChecks {
		if check := checkFor(t, contract, name); check.ThroughBinding != contract.ProbeProtocol.ServiceBinding {
			t.Fatalf("check %q crosses %q, not the declared service binding %q",
				name, check.ThroughBinding, contract.ProbeProtocol.ServiceBinding)
		}
	}
}

// TestThePeerIsDistinguishableFromTheCaller reads the property the two service
// checks now rest on off the committed corpus, the way a reviewer would.
//
// The peer used to run the measured worker's own byte-pinned bundle, and that
// made a self-binding observationally identical to the dispatch: the caller's
// `/abi/echo-stream` answered `/abi/service-echo-stream` with the same
// accounting and the same timing. What separates them now is bytes — the peer's
// module declares an identity, the caller's does not, and no other bundle
// carries the string — so an answer produced by the caller cannot be mistaken
// for one produced by the callee.
func TestThePeerIsDistinguishableFromTheCaller(t *testing.T) {
	contract := verifiedContract(t)
	deployment, err := contract.WorkerDeployment()
	if err != nil {
		t.Fatalf("deployment: %v", err)
	}
	if deployment.Peer.Bundle.Name == deployment.Bundle.Name {
		t.Fatalf("the peer runs the measured worker's bundle %q; a peer nobody can tell apart from the caller "+
			"measures no dispatch", deployment.Bundle.Name)
	}
	identity := contract.Deployment.Peer.Identity
	peerMain, ok := deployment.Peer.Bundle.Module(deployment.Peer.Bundle.MainModule)
	if !ok {
		t.Fatal("the peer bundle carries no main module")
	}
	derived, err := workerbundle.DerivePeerIdentity(peerMain.Source)
	if err != nil {
		t.Fatalf("derive the peer identity: %v", err)
	}
	if derived != identity {
		t.Fatalf("the corpus states peer identity %q and the peer's bytes declare %q", identity, derived)
	}
	callerMain, _ := deployment.Bundle.Module(deployment.Bundle.MainModule)
	if bytes.Contains(callerMain.Source, []byte(identity)) {
		t.Fatalf("the measured worker's bytes carry the peer identity %q; a host could stamp it without "+
			"dispatching anything", identity)
	}
	callerIdentity, err := workerbundle.DerivePeerIdentity(callerMain.Source)
	if err != nil || callerIdentity != "" {
		t.Fatalf("the measured worker declares peer identity %q (%v); only a callee has one", callerIdentity, err)
	}
}

package provider

import _ "embed"

// These two retained definitions are the v1beta1 WorkerVersion and
// WorkerDeployment bytes shipped by Registry provider 2.1.1. The current
// source model has acquired additional current-only fixture/constraint
// vocabulary, so reconstructing these historical Definitions from the live
// model would change their canonical digest. Keep the exact bytes in the
// provider reference binary for state decoding; never use them as current
// Form input.

var (
	//go:embed testdata/v3-retained-worker-version-definition.json
	retainedWorkerVersionDefinition []byte

	//go:embed testdata/v3-retained-worker-deployment-definition.json
	retainedWorkerDeploymentDefinition []byte
)

func retainedFrozenDefinition(kind string) ([]byte, bool) {
	switch kind {
	case "WorkerVersion":
		return retainedWorkerVersionDefinition, true
	case "WorkerDeployment":
		return retainedWorkerDeploymentDefinition, true
	default:
		return nil, false
	}
}

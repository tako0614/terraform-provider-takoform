package portableconformancev3

// This file is the explicitly bounded adapter from the retained published
// portable-Host corpus into neutral semantic roles. The active generic source
// and typed plan never select these concrete slots; legacy byte-identical
// suites keep using them through this adapter until their published lanes are
// retired under separate authority.

func legacySemanticPrimary(input RunnerInput) ResourceProbe {
	return input.ModuleWorker
}

func legacySemanticKeyed(input RunnerInput) ResourceProbe {
	return input.EdgeKvNamespace
}

func legacySemanticSequenced(input RunnerInput) ResourceProbe {
	return input.AtLeastOnceQueue
}

func legacySemanticRevision(input RunnerInput) ResourceProbe {
	return input.WorkerVersion
}

func legacySemanticOutput(input RunnerInput) ResourceProbe {
	return input.WorkerEndpoint
}

func legacyHostArtifactTransport(input RunnerInput) hostArtifactTransport {
	bundle := input.WorkerBundle
	return hostArtifactTransport{
		BlobSource: bundle.ModuleSource, ContentType: "application/octet-stream",
		Manifest: bundle.Manifest, ManifestKind: workerBundleKind,
	}
}

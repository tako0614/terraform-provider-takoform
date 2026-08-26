package portableconformancev3

// genericArtifactTransport is the family-neutral artifact evidence used by
// generic Host checks. It carries only opaque transport bytes; artifact-backed
// resource declarations belong to a concrete family/adapter lane.
type genericArtifactTransport struct {
	BlobSource   string
	DeclaredSize int
	ContentType  string
}

// hostArtifactTransport is adapter-owned wire evidence. The generic plan owns
// only opaque bytes, size, digest, and content type; a concrete HTTP adapter
// translates those facts to the currently published artifact envelope.
type hostArtifactTransport struct {
	BlobSource   string
	ContentType  string
	Manifest     map[string]any
	ManifestKind string
}

// genericSemanticRoles is the private bridge between a data-only external
// Snapshot and the reusable portable Host/Core check engine. The names express
// only behavior needed by a check. In particular, none is a concrete Form
// identity. Concrete family corpora retain RunnerInput and are translated by
// the fallback accessors below.
type genericSemanticRoles struct {
	Primary           ResourceProbe
	Keyed             ResourceProbe
	Sequenced         ResourceProbe
	Revision          ResourceProbe
	Artifact          genericArtifactTransport
	Output            ResourceProbe
	ExclusiveSubjects []ResourceProbe

	SecondGroup      FormRef
	SecondDefinition SyntheticDefinitionProbe
	Constraints      ConstraintSemanticsProbe
	NegativeFixtures []NegativeFixture
	ExternalServices ExternalServiceProbe
	SupportInterface NameVersion
	SupportBinding   NameVersion
}

func (roles *genericSemanticRoles) probeEntries() []probeEntry {
	if roles == nil {
		return nil
	}
	entries := []probeEntry{
		{Label: "primary", Kind: roles.Primary.Identity.FormRef.Kind, Probe: &roles.Primary},
		{Label: "keyed", Kind: roles.Keyed.Identity.FormRef.Kind, Probe: &roles.Keyed},
		{Label: "sequenced", Kind: roles.Sequenced.Identity.FormRef.Kind, Probe: &roles.Sequenced},
		{Label: "revision", Kind: roles.Revision.Identity.FormRef.Kind, Probe: &roles.Revision},
		{Label: "output", Kind: roles.Output.Identity.FormRef.Kind, Probe: &roles.Output},
	}
	for index := range roles.ExclusiveSubjects {
		entries = append(entries, probeEntry{
			Label: "exclusiveSubject", Kind: roles.ExclusiveSubjects[index].Identity.FormRef.Kind,
			Probe: &roles.ExclusiveSubjects[index],
		})
	}
	return entries
}

func (roles *genericSemanticRoles) constraintEntries() []constraintDefinitionEntry {
	if roles == nil {
		return nil
	}
	return []constraintDefinitionEntry{
		{label: "node", probe: &roles.Constraints.Node},
		{label: "distinctPair", probe: &roles.Constraints.DistinctPair},
		{label: "uniquePair", probe: &roles.Constraints.UniquePair},
		{label: "uniquePairSecond", probe: &roles.Constraints.UniquePairSecond},
		{label: "member", probe: &roles.Constraints.Member},
		{label: "sameTarget", probe: &roles.Constraints.SameTarget},
		{label: "structural", probe: &roles.Constraints.Structural},
	}
}

func (c Contract) hasLegacyConcreteFormSlots() bool {
	return len(declaredProbes(&c.RunnerInput)) != 0
}

func (c Contract) semanticProbeEntries() []probeEntry {
	if c.genericRoles != nil {
		return c.genericRoles.probeEntries()
	}
	return declaredProbes(&c.RunnerInput)
}

func (c Contract) semanticConstraintEntries() []constraintDefinitionEntry {
	if c.genericRoles != nil {
		return c.genericRoles.constraintEntries()
	}
	return constraintDefinitionInventory(&c.RunnerInput)
}

func (c Contract) semanticSecondDefinition() SyntheticDefinitionProbe {
	if c.genericRoles != nil {
		return c.genericRoles.SecondDefinition
	}
	return c.RunnerInput.SyntheticSecondDefinitionVersion
}

func (c Contract) semanticSecondGroup() FormRef {
	if c.genericRoles != nil {
		return c.genericRoles.SecondGroup
	}
	return c.RunnerInput.SyntheticSecondGroup
}

func (c Contract) semanticConstraints() ConstraintSemanticsProbe {
	if c.genericRoles != nil {
		return c.genericRoles.Constraints
	}
	return c.RunnerInput.ConstraintSemantics
}

func (c Contract) semanticNegativeFixtures() []NegativeFixture {
	if c.genericRoles != nil {
		return c.genericRoles.NegativeFixtures
	}
	return c.RunnerInput.NegativeFixtures
}

func (c Contract) semanticExternalServices() ExternalServiceProbe {
	if c.genericRoles != nil {
		return c.genericRoles.ExternalServices
	}
	return c.RunnerInput.ExternalServices
}

func (c Contract) semanticPrimary() ResourceProbe {
	if c.genericRoles != nil {
		return c.genericRoles.Primary
	}
	return legacySemanticPrimary(c.RunnerInput)
}

func (c Contract) semanticKeyed() ResourceProbe {
	if c.genericRoles != nil {
		return c.genericRoles.Keyed
	}
	return legacySemanticKeyed(c.RunnerInput)
}

func (c Contract) semanticSequenced() ResourceProbe {
	if c.genericRoles != nil {
		return c.genericRoles.Sequenced
	}
	return legacySemanticSequenced(c.RunnerInput)
}

func (c Contract) semanticRevision() ResourceProbe {
	if c.genericRoles != nil {
		return c.genericRoles.Revision
	}
	return legacySemanticRevision(c.RunnerInput)
}

func (c Contract) semanticOutput() ResourceProbe {
	if c.genericRoles != nil {
		return c.genericRoles.Output
	}
	return legacySemanticOutput(c.RunnerInput)
}

func (r *v3Runner) selectedLegacySemanticInput() {
	if r.contract.genericRoles == nil {
		r.legacySemanticSelections++
	}
}

func (r *v3Runner) semanticProbeEntries() []probeEntry {
	r.selectedLegacySemanticInput()
	return r.contract.semanticProbeEntries()
}

func (r *v3Runner) semanticConstraintEntries() []constraintDefinitionEntry {
	r.selectedLegacySemanticInput()
	return r.contract.semanticConstraintEntries()
}

func (r *v3Runner) semanticSecondDefinition() SyntheticDefinitionProbe {
	r.selectedLegacySemanticInput()
	return r.contract.semanticSecondDefinition()
}

func (r *v3Runner) semanticSecondGroup() FormRef {
	r.selectedLegacySemanticInput()
	return r.contract.semanticSecondGroup()
}

func (r *v3Runner) semanticConstraints() ConstraintSemanticsProbe {
	r.selectedLegacySemanticInput()
	return r.contract.semanticConstraints()
}

func (r *v3Runner) semanticNegativeFixtures() []NegativeFixture {
	r.selectedLegacySemanticInput()
	return r.contract.semanticNegativeFixtures()
}

func (r *v3Runner) semanticExternalServices() ExternalServiceProbe {
	r.selectedLegacySemanticInput()
	return r.contract.semanticExternalServices()
}

func (r *v3Runner) hostArtifactTransport() hostArtifactTransport {
	if r.adapterArtifact != nil {
		return *r.adapterArtifact
	}
	r.selectedLegacySemanticInput()
	return legacyHostArtifactTransport(r.contract.RunnerInput)
}

func (r *v3Runner) semanticPrimary() ResourceProbe {
	r.selectedLegacySemanticInput()
	return r.contract.semanticPrimary()
}

func (r *v3Runner) semanticKeyed() ResourceProbe {
	r.selectedLegacySemanticInput()
	return r.contract.semanticKeyed()
}

func (r *v3Runner) semanticSequenced() ResourceProbe {
	r.selectedLegacySemanticInput()
	return r.contract.semanticSequenced()
}

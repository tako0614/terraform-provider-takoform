package portableconformancev3

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func stableBuildGenericPlan(seed genericPlanSeed) (genericPlan, error) {
	if seed.Snapshot == nil {
		return genericPlan{}, errors.New("cannot build generic plan without a Snapshot")
	}
	primary, err := genericPlanResource(seed, "primary", seed.Probe.FormRef, seed.Probe.Name, seed.Probe.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	keyed, err := genericPlanResource(seed, "keyed", seed.Probe.Resources.Keyed.FormRef, seed.Probe.Resources.Keyed.Name, seed.Probe.Resources.Keyed.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	revision, err := genericPlanResource(seed, "revision", seed.Probe.Resources.Revision.FormRef, seed.Probe.Resources.Revision.Name, seed.Probe.Resources.Revision.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	output, err := genericPlanResource(seed, "output", seed.Probe.Resources.Output.FormRef, seed.Probe.Resources.Output.Name, seed.Probe.Resources.Output.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	secondGroup, err := genericPlanResource(seed, "second-group", seed.Probe.SyntheticSecondGroup, "same-kind-plan", map[string]any{})
	if err != nil {
		return genericPlan{}, err
	}
	primaryMaterialized, err := stableGenericMaterialize(seed.Snapshot, primary.Ref, primary.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	primarySpecHash, err := genericSpecCanonicalDigest(primaryMaterialized)
	if err != nil {
		return genericPlan{}, err
	}
	primaryUpdated := primary
	primaryUpdated.Desired = genericCloneJSONMap(seed.Probe.UpdatedDesired)
	primaryUpdatedMaterialized, err := stableGenericMaterialize(seed.Snapshot, primary.Ref, primaryUpdated.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	primaryUpdatedSpecHash, err := genericSpecCanonicalDigest(primaryUpdatedMaterialized)
	if err != nil {
		return genericPlan{}, err
	}
	keyedMaterialized, err := stableGenericMaterialize(seed.Snapshot, keyed.Ref, keyed.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	keyedUpdated := keyed
	keyedUpdated.Desired = genericCloneJSONMap(seed.Probe.Resources.Keyed.UpdatedDesired)
	keyedUpdatedMaterialized, err := stableGenericMaterialize(seed.Snapshot, keyed.Ref, keyedUpdated.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	keyedSecondUpdate := keyed
	keyedSecondUpdate.Desired = genericCloneJSONMap(seed.Probe.Resources.Keyed.SecondUpdatedDesired)
	keyedSecondMaterialized, err := stableGenericMaterialize(seed.Snapshot, keyed.Ref, keyedSecondUpdate.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	revisionMaterialized, err := stableGenericMaterialize(seed.Snapshot, revision.Ref, revision.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	outputMaterialized, err := stableGenericMaterialize(seed.Snapshot, output.Ref, output.Desired)
	if err != nil {
		return genericPlan{}, err
	}
	primaryForm, primaryDefinition, err := genericPlanCatalogEvidence(seed, primary.Ref)
	if err != nil {
		return genericPlan{}, err
	}
	_, secondDefinition, err := genericPlanCatalogEvidence(seed, seed.Probe.SyntheticSecondDefinitionVersion)
	if err != nil {
		return genericPlan{}, err
	}
	_, secondGroupDefinition, err := genericPlanCatalogEvidence(seed, secondGroup.Ref)
	if err != nil {
		return genericPlan{}, err
	}

	primaryActor := genericActor{Credential: genericCredentialPrimary}
	alternateActor := genericActor{Credential: genericCredentialAlternate}
	otherTenantActor := genericActor{Credential: genericCredentialOtherTenant}
	unauthenticatedActor := genericActor{Credential: genericCredentialNone}
	plan := genericPlan{}
	add := func(planCase genericPlanCase) { plan.Cases = append(plan.Cases, planCase) }

	add(genericPlanCase{
		ID: "catalog-discover", Actor: primaryActor,
		Command: genericCommand{Catalog: &genericCatalogRequest{Action: genericCatalogDiscover}},
		Expected: genericExpected{
			Code: "ok", APIVersions: []string{seed.Contract.APIVersion},
			Features: []string{
				"artifact_upload", "exact_form_ref", "idempotent_lifecycle", "operations",
				"optimistic_concurrency", "service_forms", "support_profiles",
			},
			EndpointPaths: map[string]string{"api": seed.Contract.APIPath},
		},
		Checks: []string{"discovery-exact"},
	})
	add(genericPlanCase{
		ID: "catalog-list-primary", Actor: primaryActor,
		Command:  genericCommand{Catalog: &genericCatalogRequest{Action: genericCatalogList, Space: primary.Space, Ref: primary.Ref}},
		Expected: genericExpected{Code: "ok", Forms: []genericObservedForm{primaryForm}},
		Checks: []string{
			"availability-truth-conditions", "forms-exact-availability", "forms-route-enumerates",
		},
	})
	add(genericPlanCase{
		ID: "catalog-get-primary", Actor: primaryActor,
		Command: genericCommand{Catalog: &genericCatalogRequest{
			Action: genericCatalogGet, Surface: genericCatalogSurfaceDefinition, Space: primary.Space, Ref: primary.Ref,
		}},
		Expected: genericExpected{Code: "ok", FormCount: genericInt(1), DefinitionDigest: primary.Ref.SchemaDigest, Definition: primaryDefinition},
		Checks:   []string{"form-definition-exact"},
	})
	add(genericPlanCase{
		ID: "catalog-get-second-definition", Actor: primaryActor,
		Command: genericCommand{Catalog: &genericCatalogRequest{
			Action: genericCatalogGet, Surface: genericCatalogSurfaceDefinition,
			Space: primary.Space, Ref: seed.Probe.SyntheticSecondDefinitionVersion,
		}},
		Expected: genericExpected{Code: "ok", FormCount: genericInt(1), DefinitionDigest: seed.Probe.SyntheticSecondDefinitionVersion.SchemaDigest, Definition: secondDefinition},
		Checks:   []string{"two-definition-versions-answer-independently"},
	})
	add(genericPlanCase{
		ID: "catalog-get-second-group", Actor: primaryActor,
		Command: genericCommand{Catalog: &genericCatalogRequest{
			Action: genericCatalogGet, Surface: genericCatalogSurfaceDefinition,
			Space: secondGroup.Space, Ref: secondGroup.Ref,
		}},
		Expected: genericExpected{
			Code: "ok", FormCount: genericInt(1), DefinitionDigest: secondGroup.Ref.SchemaDigest,
			Definition: secondGroupDefinition,
			AddressPath: seed.Contract.APIPath + "/form-definitions/" +
				strings.Join(strings.Split(secondGroup.Ref.APIVersion, "/"), "/") + "/" + secondGroup.Ref.Kind,
		},
		Checks: []string{"namespaced-group-travels-as-path-segments", "same-kind-two-groups"},
	})
	add(genericPlanCase{
		ID: "catalog-form-support", Actor: primaryActor,
		Command: genericCommand{Catalog: &genericCatalogRequest{
			Action: genericCatalogGet, Surface: genericCatalogSurfaceSupport,
			Space: primary.Space, Ref: primary.Ref,
		}},
		Expected: genericExpected{Code: "ok", Support: &genericObservedSupport{
			APIVersion: supportAPIVersionPrefix + seed.Contract.lane.SupportProfileSchemaVersion,
			Kind:       "FormSupport", Ref: primary.Ref, Operations: primaryForm.Operations, ExtraKeys: []string{},
		}},
		Checks: []string{"form-support-profile-exact"},
	})
	for _, service := range []struct {
		id          string
		protocol    string
		satisfiable bool
	}{
		{"supported", seed.Probe.ExternalServices.SupportedProtocol, true},
		{"unsupported", seed.Probe.ExternalServices.UnsupportedProtocol, false},
	} {
		add(genericPlanCase{
			ID: "catalog-standard-service-" + service.id, Actor: primaryActor,
			Command: genericCommand{Catalog: &genericCatalogRequest{
				Action: genericCatalogGet, Surface: genericCatalogSurfaceService,
				Protocol: service.protocol,
			}},
			Expected: genericExpected{Code: "ok", Support: &genericObservedSupport{
				APIVersion: supportAPIVersionPrefix + seed.Contract.lane.SupportProfileSchemaVersion,
				Kind:       "StandardServiceSupport", Protocol: service.protocol,
				Satisfiable: genericBool(service.satisfiable), ExtraKeys: []string{},
			}},
		})
	}

	add(genericPlanCase{
		ID: "validate-unauthenticated", Actor: unauthenticatedActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: primary}},
		Expected: genericExpected{
			Code: "unauthenticated", HTTPStatus: genericInt(seed.Contract.lane.ErrorHTTPStatus["unauthenticated"]),
			Retryable: genericBool(false), RequestIDPresent: genericBool(true),
		},
		Checks: []string{"unauthenticated-request-refused"},
	})
	for index, code := range seed.Contract.ErrorEnvelope.Codes {
		checks := []string(nil)
		if index == len(seed.Contract.ErrorEnvelope.Codes)-1 {
			checks = []string{"error-envelope-taxonomy"}
		}
		add(genericPlanCase{
			ID: "error-envelope-" + code, Actor: primaryActor,
			Command: genericCommand{
				Catalog:  &genericCatalogRequest{Action: genericCatalogDiscover},
				Controls: genericControls{Fault: genericErrorFault(code)},
			},
			Expected: genericExpected{
				Code: code, HTTPStatus: genericInt(seed.Contract.ErrorEnvelope.HTTPStatusByCode[code]),
				Retryable:        genericBool(seed.Contract.lane.isAutomaticallyRetryable(code)),
				RequestIDPresent: genericBool(true),
			},
			Checks: checks,
		})
	}
	invalidSpace := primary
	invalidSpace.Space = " invalid"
	add(genericPlanCase{
		ID: "validate-invalid-space", Actor: primaryActor,
		Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: invalidSpace}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"space-id-grammar-enforced"},
	})
	unknown := primary
	unknown.Ref.SchemaDigest = formpackage.DigestBytes([]byte("generic-plan-unknown-definition"))
	add(genericPlanCase{
		ID: "validate-unknown-exact-ref", Actor: primaryActor,
		Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: unknown}},
		Expected: genericExpected{Code: "form_unknown"},
		Checks:   []string{"exact-form-ref-fails-closed-on-unknown-definition"},
	})
	add(genericPlanCase{
		ID: "validate-primary", Actor: primaryActor,
		Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: primary}},
		Expected: genericExpected{Code: "ok", Valid: genericBool(true), EffectiveDesired: primaryMaterialized},
		Checks:   []string{"portable-defaults-materialized", "validate-accepts-canonical"},
	})
	invalidSchema := primary
	invalidSchema.Name = "invalid-schema-plan"
	invalidSchema.Desired = genericCloneJSONMap(seed.Probe.InvalidSchemaDesired)
	add(genericPlanCase{
		ID: "validate-invalid-schema", Actor: primaryActor,
		Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: invalidSchema}},
		Expected: genericExpected{Code: "ok", Valid: genericBool(false)},
		Checks:   []string{"validate-rejects-negative-fixtures"},
	})
	invalidConstraint := primary
	invalidConstraint.Name = "invalid-constraint-plan"
	invalidConstraint.Desired = genericCloneJSONMap(seed.Probe.InvalidConstraintDesired)
	add(genericPlanCase{
		ID: "validate-invalid-declared-constraint", Actor: primaryActor,
		Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: invalidConstraint}},
		Expected: genericExpected{Code: "ok", Valid: genericBool(false)},
	})
	requiredUnsupported := primary
	requiredUnsupported.Name = "required-service-plan"
	requiredUnsupported.Desired = genericCloneJSONMap(seed.Probe.ExternalServices.RequiredUnsupportedDesired)
	add(genericPlanCase{
		ID: "prepare-required-unsupported-service", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "required-service-prepare", Resource: requiredUnsupported,
		}},
		Expected: genericExpected{Code: "unsupported_capability"},
		Checks:   []string{"stable-standard-service-support-enforced"},
	})
	optionalUnsupported := primary
	optionalUnsupported.Name = "optional-service-plan"
	optionalUnsupported.Desired = genericCloneJSONMap(seed.Probe.ExternalServices.OptionalUnsupportedDesired)
	add(genericPlanCase{
		ID: "validate-optional-unsupported-service", Actor: primaryActor,
		Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: optionalUnsupported}},
		Expected: genericExpected{Code: "ok", Valid: genericBool(true)},
	})

	add(genericPlanCase{
		ID: "prepare-primary-create", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "primary-create-prepare", Resource: primary,
		}},
		Expected: genericExpected{
			Code: "ok", PreparationHandle: "primary-create-prepare",
			PreparationSpecHash: primarySpecHash, EffectiveDesired: primaryMaterialized,
		},
		Checks: []string{"prepare-binds-exact-spec"},
	})
	primaryCreate := primary
	primaryCreate.Create = true
	primaryCreate.PreparationHandle = "primary-create-prepare"
	primaryCreate.IdempotencyKey = "generic-plan-primary-create"
	add(genericPlanCase{
		ID: "apply-primary-create", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryCreate}},
		Expected: genericExpected{
			Code: "created", ETag: `"1"`, ResourceHandle: primary.Handle,
			Generation: "1", Revision: "1", Desired: primaryMaterialized,
			Conditions: []string{"Ready=True:Available"},
		},
		Checks: []string{"apply-create-uid-minted", "apply-headers-required", "condition-reason-closed"},
	})
	add(genericPlanCase{
		ID: "apply-primary-replay", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryCreate}},
		Expected: genericExpected{
			Code: "created", ResourceHandle: primary.Handle, Generation: "1", Revision: "1",
			Desired: primaryMaterialized, SameUIDAs: "apply-primary-create",
		},
		Checks: []string{"apply-idempotency-replay"},
	})
	add(genericPlanCase{
		ID: "apply-primary-cross-principal", Actor: alternateActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryCreate}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"cross-principal-idempotency-isolation"},
	})
	primaryFreshConflict := primaryCreate
	primaryFreshConflict.IdempotencyKey = "generic-plan-primary-conflict"
	add(genericPlanCase{
		ID: "apply-primary-create-conflict", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryFreshConflict}},
		Expected: genericExpected{Code: "generation_conflict"},
		Checks:   []string{"create-conflict-when-exists"},
	})
	add(genericPlanCase{
		ID: "read-primary", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceRead, Resource: primary}},
		Expected: genericExpected{
			Code: "ok", ETag: `"1"`, ResourceHandle: primary.Handle, Generation: "1", Revision: "1",
			Desired: primaryMaterialized, SameUIDAs: "apply-primary-create",
		},
		Checks: []string{"resource-address-is-tenant-scoped", "resource-answers-only-under-its-recorded-form-ref", "revision-etag-exact"},
	})
	add(genericPlanCase{
		ID: "read-primary-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceRead, Resource: primary}},
		Expected: genericExpected{Code: "resource_not_found"},
		Checks:   []string{"resource-read-is-tenant-isolated"},
	})
	primaryOtherSpace := primary
	primaryOtherSpace.Space = seed.Contract.RunnerInput.AlternateSpace
	add(genericPlanCase{
		ID: "read-primary-other-space", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceRead, Resource: primaryOtherSpace}},
		Expected: genericExpected{Code: "resource_not_found"},
		Checks:   []string{"cross-space-isolation"},
	})
	add(genericPlanCase{
		ID: "prepare-primary-update-without-fence", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "primary-unfenced-update", Resource: primaryUpdated,
		}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"prepare-requires-update-fence"},
	})
	add(genericPlanCase{
		ID: "prepare-primary-update", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "primary-update-prepare",
			Resource: primaryUpdated, ExpectedGeneration: "1",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "primary-update-prepare", EffectiveDesired: primaryUpdatedMaterialized},
	})
	primaryUpdated.ExpectedGeneration = "1"
	primaryUpdated.ExpectedUID = "primary#1"
	primaryUpdated.PreparationHandle = "primary-update-prepare"
	primaryUpdated.IdempotencyKey = "generic-plan-primary-update"
	add(genericPlanCase{
		ID: "apply-primary-update", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryUpdated}},
		Expected: genericExpected{
			Code: "ok", ResourceHandle: primary.Handle, Generation: "2", Revision: "2",
			Desired: primaryUpdatedMaterialized, SameUIDAs: "apply-primary-create",
		},
		Checks: []string{"concurrent-unrelated-mutation", "spec-change-bumps-generation", "update-generation-fence"},
	})
	staleUpdate := primaryUpdated
	staleUpdate.IdempotencyKey = "generic-plan-primary-stale"
	add(genericPlanCase{
		ID: "apply-primary-stale", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: staleUpdate}},
		Expected: genericExpected{Code: "generation_conflict"},
		Checks:   []string{"stale-generation-rejected", "stale-prepare-rejected"},
	})
	add(genericPlanCase{
		ID: "prepare-primary-current", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "primary-current-prepare",
			Resource: primaryUpdated, ExpectedGeneration: "2",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "primary-current-prepare"},
	})
	wrongUID := primaryUpdated
	wrongUID.ExpectedGeneration = "2"
	wrongUID.ExpectedUID = "not-primary#1"
	wrongUID.PreparationHandle = "primary-current-prepare"
	wrongUID.IdempotencyKey = "generic-plan-primary-wrong-uid"
	add(genericPlanCase{
		ID: "apply-primary-wrong-uid", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: wrongUID}},
		Expected: genericExpected{Code: "uid_mismatch"},
		Checks:   []string{"expected-uid-mismatch-rejected"},
	})
	otherTenantUpdate := primaryUpdated
	otherTenantUpdate.ExpectedGeneration = "2"
	otherTenantUpdate.PreparationHandle = "missing-other-tenant-prepare"
	otherTenantUpdate.IdempotencyKey = "generic-plan-other-tenant-update"
	add(genericPlanCase{
		ID: "apply-primary-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: otherTenantUpdate}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"resource-update-is-tenant-isolated"},
	})
	observePrimary := primary
	observePrimary.ExpectedGeneration = "2"
	observePrimary.IdempotencyKey = "generic-plan-primary-observe"
	add(genericPlanCase{
		ID: "observe-primary", Actor: primaryActor,
		Command: genericCommand{
			Resource: &genericResourceRequest{Action: genericResourceObserve, Resource: observePrimary},
			Controls: genericControls{BackendEffect: genericBackendEffectTouchStatus},
		},
		Expected: genericExpected{
			Code: "ok", ResourceHandle: primary.Handle, Generation: "2", Revision: "3",
			Desired: primaryUpdatedMaterialized, SameUIDAs: "apply-primary-create",
		},
		Checks: []string{"observe-fence-exact", "status-change-bumps-revision-not-generation"},
	})
	add(genericPlanCase{
		ID: "observe-primary-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceObserve, Resource: observePrimary}},
		Expected: genericExpected{Code: "resource_not_found"},
		Checks:   []string{"resource-observe-is-tenant-isolated"},
	})
	add(genericPlanCase{
		ID: "prepare-primary-fence-matrix", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "primary-fence-prepare",
			Resource: primaryUpdated, ExpectedGeneration: "2",
		}},
		Expected: genericExpected{
			Code: "ok", PreparationHandle: "primary-fence-prepare",
			PreparationSpecHash: primaryUpdatedSpecHash,
		},
	})
	fenceEvidenceStart := len(plan.Cases)
	missingFence := primaryUpdated
	missingFence.ExpectedGeneration = ""
	missingFence.OmitGeneration = true
	missingFence.PreparationHandle = "primary-fence-prepare"
	missingFence.IdempotencyKey = "generic-plan-fence-missing"
	add(genericPlanCase{
		ID: "apply-primary-fence-missing", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: missingFence}},
		Expected: genericExpected{Code: "invalid_argument"},
	})
	bodyFence := primaryUpdated
	bodyFence.ExpectedGeneration = ""
	bodyFence.BodyGeneration = "2"
	bodyFence.PreparationHandle = "primary-fence-prepare"
	bodyFence.IdempotencyKey = "generic-plan-fence-body"
	add(genericPlanCase{
		ID: "apply-primary-fence-body", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: bodyFence}},
		Expected: genericExpected{
			Code: "ok", ETag: `"3"`, ResourceHandle: primary.Handle,
			Generation: "2", Revision: "3", Desired: primaryUpdatedMaterialized,
		},
	})
	disagreeFence := primaryUpdated
	disagreeFence.ExpectedGeneration = "2"
	disagreeFence.DisagreeingBodyGeneration = "20"
	disagreeFence.PreparationHandle = "primary-fence-prepare"
	disagreeFence.IdempotencyKey = "generic-plan-fence-disagree"
	add(genericPlanCase{
		ID: "apply-primary-fence-disagree", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: disagreeFence}},
		Expected: genericExpected{Code: "invalid_argument"},
	})
	honestEcho := primaryUpdated
	honestEcho.ExpectedGeneration = "2"
	honestEcho.PreparationHandle = "primary-fence-prepare"
	honestEcho.ReviewSpecHash = primaryUpdatedSpecHash
	honestEcho.IdempotencyKey = "generic-plan-fence-echo-honest"
	add(genericPlanCase{
		ID: "apply-primary-fence-echo-honest", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: honestEcho}},
		Expected: genericExpected{Code: "ok", ResourceHandle: primary.Handle, Generation: "2", Revision: "3"},
	})
	lyingEcho := honestEcho
	lyingEcho.ReviewSpecHash = formpackage.DigestBytes([]byte("unprepared generic spec"))
	lyingEcho.IdempotencyKey = "generic-plan-fence-echo-lying"
	add(genericPlanCase{
		ID: "apply-primary-fence-echo-lying", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: lyingEcho}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"fence-matrix-observed"},
	})
	for index := fenceEvidenceStart; index < len(plan.Cases); index++ {
		plan.Cases[index].Checks = append(plan.Cases[index].Checks, "fence-matrix-observed")
	}
	deleteStaleGeneration := primary
	deleteStaleGeneration.ExpectedGeneration = "1"
	deleteStaleGeneration.IdempotencyKey = "generic-plan-primary-delete-stale-generation"
	add(genericPlanCase{
		ID: "delete-primary-stale-generation", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceDelete, Resource: deleteStaleGeneration}},
		Expected: genericExpected{Code: "generation_conflict"},
		Checks:   []string{"delete-generation-fence"},
	})
	deleteStaleRevision := primary
	deleteStaleRevision.ExpectedGeneration = "2"
	deleteStaleRevision.ExpectedRevision = "2"
	deleteStaleRevision.IdempotencyKey = "generic-plan-primary-delete-stale-revision"
	add(genericPlanCase{
		ID: "delete-primary-stale-revision", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceDelete, Resource: deleteStaleRevision}},
		Expected: genericExpected{Code: "revision_conflict"},
		Checks:   []string{"delete-revision-fence", "stale-revision-rejected"},
	})
	deleteOtherTenant := primary
	deleteOtherTenant.ExpectedGeneration = "2"
	deleteOtherTenant.IdempotencyKey = "generic-plan-primary-delete-other-tenant"
	add(genericPlanCase{
		ID: "delete-primary-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceDelete, Resource: deleteOtherTenant}},
		Expected: genericExpected{Code: "resource_not_found"},
		Checks:   []string{"resource-delete-is-tenant-isolated"},
	})
	deletePrimary := primary
	deletePrimary.ExpectedGeneration = "2"
	deletePrimary.ExpectedRevision = "3"
	deletePrimary.IdempotencyKey = "generic-plan-primary-delete"
	add(genericPlanCase{
		ID: "delete-primary", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceDelete, Resource: deletePrimary}},
		Expected: genericExpected{Code: "deleted"},
	})
	add(genericPlanCase{
		ID: "prepare-primary-recreate", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "primary-recreate-prepare", Resource: primary,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "primary-recreate-prepare"},
	})
	primaryRecreate := primaryCreate
	primaryRecreate.PreparationHandle = "primary-recreate-prepare"
	add(genericPlanCase{
		ID: "apply-primary-recreate", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryRecreate}},
		Expected: genericExpected{
			Code: "created", ResourceHandle: primary.Handle, Generation: "1", Revision: "1",
			Desired: primaryMaterialized, DifferentUIDFrom: "apply-primary-create",
		},
		Checks: []string{"delete-then-recreate-uid-changes", "replay-record-retires-with-its-incarnation"},
	})

	audit := primary
	audit.PackageDigest = formpackage.DigestBytes([]byte("another-valid-audit-package"))
	add(genericPlanCase{
		ID: "prepare-package-audit", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "package-audit-prepare",
			Resource: audit, ExpectedGeneration: "1",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "package-audit-prepare"},
	})
	audit.ExpectedGeneration = "1"
	audit.PreparationHandle = "package-audit-prepare"
	audit.IdempotencyKey = "generic-plan-package-audit"
	add(genericPlanCase{
		ID: "apply-package-audit", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: audit}},
		Expected: genericExpected{
			Code: "ok", ResourceHandle: primary.Handle, Generation: "1", Revision: "1",
			Desired: primaryMaterialized, SameUIDAs: "apply-primary-recreate",
		},
		Checks: []string{"package-digest-not-identity"},
	})

	addRevisionPlanCases(&plan, seed, primaryActor, revision, revisionMaterialized)
	addOutputPlanCases(&plan, seed, primaryActor, output, outputMaterialized)
	addImportPlanCases(&plan, seed, primaryActor, otherTenantActor, keyed, keyedMaterialized)
	addTenantPlanCases(&plan, seed, primaryActor, otherTenantActor, keyed, keyedMaterialized)
	addOperationPlanCases(&plan, seed, primaryActor, alternateActor, keyed, keyedMaterialized, keyedUpdated, keyedUpdatedMaterialized, keyedSecondUpdate, keyedSecondMaterialized)
	addArtifactPlanCases(&plan, seed, primaryActor, otherTenantActor)
	if err := addConstraintPlanCases(&plan, seed, primaryActor, primary); err != nil {
		return genericPlan{}, err
	}

	add(genericPlanCase{
		ID: "prepare-second-group", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "second-group-prepare", Resource: secondGroup,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "second-group-prepare"},
	})
	secondGroup.Create = true
	secondGroup.PreparationHandle = "second-group-prepare"
	secondGroup.IdempotencyKey = "generic-plan-second-group"
	add(genericPlanCase{
		ID: "apply-second-group", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: secondGroup}},
		Expected: genericExpected{Code: "created", ResourceHandle: secondGroup.Handle, Generation: "1", Revision: "1"},
	})

	return genericPlanSelectRequiredEvidence(plan, stableGenericRequiredChecks), nil
}

// genericPlanSelectRequiredEvidence separates execution coverage from claims.
// The trace deliberately exercises more cases than the published generic
// roster, but only evidence explicitly promoted here may complete a check.
// This lets weak candidates remain useful adapter-parity probes without
// presenting their labels as conformance proof.
func genericPlanSelectRequiredEvidence(plan genericPlan, requiredChecks []string) genericPlan {
	required := make(map[string]bool, len(requiredChecks))
	for _, check := range requiredChecks {
		required[check] = true
	}
	for index := range plan.Cases {
		selected := make([]string, 0, len(plan.Cases[index].Checks))
		seen := map[string]bool{}
		for _, check := range plan.Cases[index].Checks {
			if required[check] && !seen[check] {
				selected = append(selected, check)
				seen[check] = true
			}
		}
		plan.Cases[index].Checks = selected
	}
	return plan
}

func genericPlanResource(
	seed genericPlanSeed,
	handle string,
	ref FormRef,
	name string,
	desired map[string]any,
) (genericResourceInput, error) {
	packageDigest := ""
	for _, form := range seed.Snapshot.Forms() {
		if portableFormRef(form.Ref) == ref {
			packageDigest = form.PackageDigest
			break
		}
	}
	if packageDigest == "" {
		return genericResourceInput{}, fmt.Errorf("generic plan resource %s exact FormRef is absent from Snapshot", handle)
	}
	return genericResourceInput{
		Handle: handle, Ref: ref, PackageDigest: packageDigest,
		Name: name, Space: seed.Probe.Space, Desired: genericCloneJSONMap(desired),
	}, nil
}

func addRevisionPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	actor genericActor,
	revision genericResourceInput,
	materialized map[string]any,
) {
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-revision-create", Actor: actor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "revision-create-prepare", Resource: revision,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "revision-create-prepare"},
	})
	revisionCreate := revision
	revisionCreate.Create = true
	revisionCreate.PreparationHandle = "revision-create-prepare"
	revisionCreate.IdempotencyKey = "generic-plan-revision-create"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-revision-create", Actor: actor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: revisionCreate}},
		Expected: genericExpected{Code: "created", ResourceHandle: revision.Handle, Generation: "1", Revision: "1", Desired: materialized},
	})
	revisionUpdated := revision
	revisionUpdated.Desired = genericCloneJSONMap(seed.Probe.Resources.Revision.UpdatedDesired)
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-revision-update", Actor: actor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "revision-update-prepare",
			Resource: revisionUpdated, ExpectedGeneration: "1",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "revision-update-prepare"},
	})
	revisionUpdated.ExpectedGeneration = "1"
	revisionUpdated.PreparationHandle = "revision-update-prepare"
	revisionUpdated.IdempotencyKey = "generic-plan-revision-update"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-revision-update", Actor: actor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: revisionUpdated}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"no-update-spec-change-rejected", "revision-role-update-rejected"},
	})
}

func addOutputPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	actor genericActor,
	output genericResourceInput,
	materialized map[string]any,
) {
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-output-create", Actor: actor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "output-create-prepare", Resource: output,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "output-create-prepare"},
	})
	output.Create = true
	output.PreparationHandle = "output-create-prepare"
	output.IdempotencyKey = "generic-plan-output-create"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-output-create", Actor: actor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: output}},
		Expected: genericExpected{
			Code: "created", ResourceHandle: output.Handle, Generation: "1", Revision: "1",
			Desired: materialized, Outputs: genericCloneJSONMap(seed.Probe.Resources.Output.HostAssignedOutputs),
		},
		Checks: []string{"form-declared-outputs-are-exact"},
	})
}

func addImportPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	primaryActor, otherTenantActor genericActor,
	keyed genericResourceInput,
	materialized map[string]any,
) {
	imported := keyed
	imported.Handle = "imported"
	imported.Name = "generic-import-plan"
	imported.NativeID = "native/generic-plan"
	imported.Create = true
	imported.IdempotencyKey = "generic-plan-import"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "import-resource", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceImport, Resource: imported}},
		Expected: genericExpected{Code: "created", ResourceHandle: imported.Handle, Generation: "1", Revision: "1", Desired: materialized},
		Checks:   []string{"import-adopts-native-resource"},
	})
	rival := imported
	rival.Handle = "import-rival"
	rival.Name = "generic-import-rival"
	rival.IdempotencyKey = "generic-plan-import-rival"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "import-native-claim-conflict", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceImport, Resource: rival}},
		Expected: genericExpected{Code: "import_conflict"},
		Checks:   []string{"import-claims-its-native-identity", "import-records-its-native-identity"},
	})
	invalid := imported
	invalid.Handle = "import-invalid"
	invalid.Name = "generic-import-invalid"
	invalid.NativeID = "native/generic-invalid"
	invalid.Desired = genericCloneJSONMap(seed.Probe.InvalidSchemaDesired)
	invalid.IdempotencyKey = "generic-plan-import-invalid"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "import-invalid", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceImport, Resource: invalid}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"import-validates-like-apply"},
	})
	otherTenant := imported
	otherTenant.Handle = "imported-other-tenant"
	otherTenant.Name = "generic-import-plan"
	otherTenant.IdempotencyKey = "generic-plan-import-other-tenant"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "import-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceImport, Resource: otherTenant}},
		Expected: genericExpected{Code: "created", ResourceHandle: otherTenant.Handle, Generation: "1", Revision: "1", Desired: materialized},
		Checks:   []string{"resource-import-is-tenant-isolated"},
	})
}

func addTenantPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	primaryActor, otherTenantActor genericActor,
	keyed genericResourceInput,
	materialized map[string]any,
) {
	tenantResource := keyed
	tenantResource.Handle = "tenant-resource"
	tenantResource.Name = "tenant-plan"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-other-tenant", Actor: otherTenantActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "other-tenant-prepare", Resource: tenantResource,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "other-tenant-prepare"},
	})
	primaryAttempt := tenantResource
	primaryAttempt.Create = true
	primaryAttempt.PreparationHandle = "other-tenant-prepare"
	primaryAttempt.IdempotencyKey = "generic-plan-tenant-shared-key"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-foreign-preparation", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: primaryAttempt}},
		Expected: genericExpected{Code: "invalid_argument"},
		Checks:   []string{"prepare-is-tenant-scoped", "prepare-substitution-rejected"},
	})
	otherTenantCreate := primaryAttempt
	otherTenantCreate.PreparationHandle = "other-tenant-prepare"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: otherTenantCreate}},
		Expected: genericExpected{Code: "created", ResourceHandle: tenantResource.Handle, Generation: "1", Revision: "1", Desired: materialized},
		Checks:   []string{"each-tenant-mutates-its-own-plane", "idempotency-is-tenant-scoped"},
	})
}

func addOperationPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	primaryActor, alternateActor genericActor,
	keyed genericResourceInput,
	keyedMaterialized map[string]any,
	keyedUpdated genericResourceInput,
	keyedUpdatedMaterialized map[string]any,
	keyedSecondUpdated genericResourceInput,
	keyedSecondMaterialized map[string]any,
) {
	async := keyed
	async.Handle = "async-create"
	async.Name = "async-plan"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-async-create", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "async-create-prepare", Resource: async,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "async-create-prepare"},
	})
	async.Create = true
	async.PreparationHandle = "async-create-prepare"
	async.IdempotencyKey = "generic-plan-async-create"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-async-create", Actor: primaryActor,
		Command: genericCommand{
			Resource: &genericResourceRequest{Action: genericResourceApply, Resource: async},
			Controls: genericControls{Completion: genericCompletionAsync},
		},
		Expected: genericExpected{Code: "accepted", OperationHandle: "async-create-operation", OperationDone: genericBool(false)},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "get-async-as-other-principal", Actor: alternateActor,
		Command:  genericCommand{Operation: &genericOperationRequest{Action: genericOperationGet, Handle: "async-create-operation"}},
		Expected: genericExpected{Code: "operation_not_found"},
		Checks:   []string{"operation-bound-to-its-creating-principal"},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "get-async-terminal", Actor: primaryActor,
		Command: genericCommand{Operation: &genericOperationRequest{Action: genericOperationGet, Handle: "async-create-operation"}},
		Expected: genericExpected{
			Code: "completed", OperationHandle: "async-create-operation", OperationDone: genericBool(true),
			ResourceHandle: async.Handle, Generation: "1", Revision: "1", Desired: keyedMaterialized,
		},
		Checks: []string{"async-operation-flow"},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "get-async-terminal-replay", Actor: primaryActor,
		Command: genericCommand{Operation: &genericOperationRequest{Action: genericOperationGet, Handle: "async-create-operation"}},
		Expected: genericExpected{
			Code: "completed", OperationHandle: "async-create-operation", OperationDone: genericBool(true),
			ResourceHandle: async.Handle, Generation: "1", Revision: "1", Desired: keyedMaterialized,
		},
		Checks: []string{"operation-replay-terminal", "operation-resumable-after-settlement"},
	})

	cancel := keyed
	cancel.Handle = "async-cancel"
	cancel.Name = "async-cancel-plan"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-async-cancel", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "async-cancel-prepare", Resource: cancel,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "async-cancel-prepare"},
	})
	cancel.Create = true
	cancel.PreparationHandle = "async-cancel-prepare"
	cancel.IdempotencyKey = "generic-plan-async-cancel"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-async-cancel", Actor: primaryActor,
		Command: genericCommand{
			Resource: &genericResourceRequest{Action: genericResourceApply, Resource: cancel},
			Controls: genericControls{Completion: genericCompletionAsync},
		},
		Expected: genericExpected{Code: "accepted", OperationHandle: "async-cancel-operation", OperationDone: genericBool(false)},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "cancel-async", Actor: primaryActor,
		Command: genericCommand{Operation: &genericOperationRequest{Action: genericOperationCancel, Handle: "async-cancel-operation"}},
		Expected: genericExpected{
			Code: "cancelled", OperationHandle: "async-cancel-operation",
			OperationDone: genericBool(true), OperationCancelled: genericBool(true),
		},
		Checks: []string{"operation-cancel"},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "cancel-async-replay", Actor: primaryActor,
		Command: genericCommand{Operation: &genericOperationRequest{Action: genericOperationCancel, Handle: "async-cancel-operation"}},
		Expected: genericExpected{
			Code: "cancelled", OperationHandle: "async-cancel-operation",
			OperationDone: genericBool(true), OperationCancelled: genericBool(true),
		},
		Checks: []string{"cancel-outcomes-closed"},
	})

	revalidate := keyed
	revalidate.Handle = "async-revalidate"
	revalidate.Name = "async-revalidate-plan"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-revalidate-create", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "revalidate-create-prepare", Resource: revalidate,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "revalidate-create-prepare"},
	})
	revalidateCreate := revalidate
	revalidateCreate.Create = true
	revalidateCreate.PreparationHandle = "revalidate-create-prepare"
	revalidateCreate.IdempotencyKey = "generic-plan-revalidate-create"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-revalidate-create", Actor: primaryActor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: revalidateCreate}},
		Expected: genericExpected{Code: "created", ResourceHandle: revalidate.Handle, Generation: "1", Revision: "1", Desired: keyedMaterialized},
	})
	revalidateFirst := keyedUpdated
	revalidateFirst.Handle = revalidate.Handle
	revalidateFirst.Name = revalidate.Name
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-revalidate-async", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "revalidate-async-prepare",
			Resource: revalidateFirst, ExpectedGeneration: "1",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "revalidate-async-prepare"},
	})
	revalidateFirst.ExpectedGeneration = "1"
	revalidateFirst.PreparationHandle = "revalidate-async-prepare"
	revalidateFirst.IdempotencyKey = "generic-plan-revalidate-async"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-revalidate-async", Actor: primaryActor,
		Command: genericCommand{
			Resource: &genericResourceRequest{Action: genericResourceApply, Resource: revalidateFirst},
			Controls: genericControls{Completion: genericCompletionAsync},
		},
		Expected: genericExpected{Code: "accepted", OperationHandle: "async-revalidate-operation", OperationDone: genericBool(false)},
	})
	revalidateSecond := keyedSecondUpdated
	revalidateSecond.Handle = revalidate.Handle
	revalidateSecond.Name = revalidate.Name
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "prepare-revalidate-sync", Actor: primaryActor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "revalidate-sync-prepare",
			Resource: revalidateSecond, ExpectedGeneration: "1",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "revalidate-sync-prepare"},
	})
	revalidateSecond.ExpectedGeneration = "1"
	revalidateSecond.PreparationHandle = "revalidate-sync-prepare"
	revalidateSecond.IdempotencyKey = "generic-plan-revalidate-sync"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "apply-revalidate-sync", Actor: primaryActor,
		Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: revalidateSecond}},
		Expected: genericExpected{
			Code: "ok", ResourceHandle: revalidate.Handle, Generation: "2", Revision: "2", Desired: keyedSecondMaterialized,
		},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "get-revalidate-terminal", Actor: primaryActor,
		Command:  genericCommand{Operation: &genericOperationRequest{Action: genericOperationGet, Handle: "async-revalidate-operation"}},
		Expected: genericExpected{Code: "generation_conflict", OperationHandle: "async-revalidate-operation", OperationDone: genericBool(true)},
		Checks:   []string{"async-commit-binds-the-accepted-identity", "async-commit-revalidates"},
	})
}

func genericPlanCatalogEvidence(
	seed genericPlanSeed,
	ref FormRef,
) (genericObservedForm, *genericObservedDefinition, error) {
	for _, compiled := range seed.Snapshot.Forms() {
		if portableFormRef(compiled.Ref) != ref {
			continue
		}
		raw, ok := seed.Snapshot.Definition(compiled.Ref)
		if !ok {
			return genericObservedForm{}, nil, fmt.Errorf("Snapshot omitted generic plan Definition %+v", ref)
		}
		definition, err := formpackage.ValidateDefinition(raw)
		if err != nil {
			return genericObservedForm{}, nil, err
		}
		desiredSchemaHash, err := genericCanonicalHash(definition.DesiredSchema)
		if err != nil {
			return genericObservedForm{}, nil, err
		}
		constraintsHash, err := genericCanonicalHash(definition.Constraints)
		if err != nil {
			return genericObservedForm{}, nil, err
		}
		operations := genericSortedOperations(definition.LifecycleCapabilities)
		return genericObservedForm{
				Ref: ref, PackageDigest: compiled.PackageDigest,
				DefinitionKnown: true, Installed: true, Executable: true, Activated: true,
				AvailableToPrincipal: true, Operations: operations,
			}, &genericObservedDefinition{
				Ref: ref, PackageDigest: compiled.PackageDigest,
				Title: definition.Title, Description: definition.Description,
				DesiredSchemaHash: desiredSchemaHash, ConstraintsHash: constraintsHash,
			}, nil
	}
	return genericObservedForm{}, nil, fmt.Errorf("Snapshot omitted generic plan Form %+v", ref)
}

func addArtifactPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	primaryActor, otherTenantActor genericActor,
) {
	blob := []byte(seed.Artifact.BlobSource)
	digest := formpackage.DigestBytes(blob)
	begin := genericArtifactRequest{
		Action: genericArtifactBegin, UploadHandle: "opaque-upload", ManifestHandle: "opaque-manifest",
		BlobDigest: digest, Blob: blob, DeclaredSize: seed.Artifact.DeclaredSize,
		ContentType: seed.Artifact.ContentType, IdempotencyKey: "generic-plan-artifact-begin",
	}
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-begin", Actor: primaryActor,
		Command:  genericCommand{Artifact: &begin},
		Expected: genericExpected{Code: "ok", UploadHandle: "opaque-upload", MissingBlobCount: genericInt(1)},
	})
	earlyCommit := begin
	earlyCommit.Action = genericArtifactCommit
	earlyCommit.IdempotencyKey = "generic-plan-artifact-early"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-commit-before-blob", Actor: primaryActor,
		Command:  genericCommand{Artifact: &earlyCommit},
		Expected: genericExpected{Code: "artifact_missing"},
		Checks:   []string{"artifact-upload-missing-blob"},
	})
	put := begin
	put.Action = genericArtifactPut
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-put-wrong-digest", Actor: primaryActor,
		Command:  genericCommand{Artifact: &put, Controls: genericControls{Fault: genericFaultWrongBlobBytes}},
		Expected: genericExpected{Code: "artifact_invalid"},
		Checks:   []string{"artifact-digest-mismatch"},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-put", Actor: primaryActor,
		Command:  genericCommand{Artifact: &put},
		Expected: genericExpected{Code: "uploaded", UploadHandle: "opaque-upload"},
	})
	foreignCommit := earlyCommit
	foreignCommit.IdempotencyKey = "generic-plan-artifact-foreign"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-commit-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Artifact: &foreignCommit},
		Expected: genericExpected{Code: "artifact_missing"},
		Checks:   []string{"upload-session-bound-to-its-creating-principal"},
	})
	commit := earlyCommit
	commit.IdempotencyKey = "generic-plan-artifact-commit"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-commit", Actor: primaryActor,
		Command:  genericCommand{Artifact: &commit},
		Expected: genericExpected{Code: "committed", UploadHandle: "opaque-upload", ManifestHandle: "opaque-manifest"},
	})
	commitReplay := commit
	commitReplay.IdempotencyKey = "generic-plan-artifact-commit-replay"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-commit-replay", Actor: primaryActor,
		Command: genericCommand{Artifact: &commitReplay},
		Expected: genericExpected{
			Code: "committed", UploadHandle: "opaque-upload", ManifestHandle: "opaque-manifest",
			SameManifestAs: "artifact-commit",
		},
		Checks: []string{"artifact-commit-idempotent"},
	})
	getManifest := genericArtifactRequest{Action: genericArtifactGetManifest, ManifestHandle: "opaque-manifest"}
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-get-manifest", Actor: primaryActor,
		Command:  genericCommand{Artifact: &getManifest},
		Expected: genericExpected{Code: "ok", ManifestHandle: "opaque-manifest"},
	})
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-get-manifest-other-tenant", Actor: otherTenantActor,
		Command:  genericCommand{Artifact: &getManifest},
		Expected: genericExpected{Code: "artifact_missing"},
		Checks:   []string{"artifact-digest-is-not-a-capability"},
	})
	headBlob := genericArtifactRequest{Action: genericArtifactHeadBlob, BlobDigest: digest}
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-head-blob", Actor: primaryActor,
		Command:  genericCommand{Artifact: &headBlob},
		Expected: genericExpected{Code: "ok", BlobPresent: genericBool(true)},
	})
	sizeLie := begin
	sizeLie.UploadHandle = "size-lie-upload"
	sizeLie.ManifestHandle = "size-lie-manifest"
	sizeLie.DeclaredSize++
	sizeLie.IdempotencyKey = "generic-plan-size-lie-begin"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-size-lie-begin", Actor: primaryActor,
		Command:  genericCommand{Artifact: &sizeLie, Controls: genericControls{Fault: genericFaultWrongDeclaredSize}},
		Expected: genericExpected{Code: "ok", UploadHandle: "size-lie-upload", MissingBlobCount: genericInt(0)},
	})
	sizeCommit := sizeLie
	sizeCommit.Action = genericArtifactCommit
	sizeCommit.IdempotencyKey = "generic-plan-size-lie-commit"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "artifact-size-lie-commit", Actor: primaryActor,
		Command:  genericCommand{Artifact: &sizeCommit},
		Expected: genericExpected{Code: "artifact_invalid"},
		Checks:   []string{"artifact-commit-binds-declared-size"},
	})
}

// addConstraintPlanCases drives one data-only scenario for every neutral
// constraint mechanism present in the external Snapshot. The field names and
// exact Forms come only from the seed; neither adapter branches on a Kind or a
// check ID. Keeping the scenario here also means its expected outcomes are
// shared by the memory state machine and the real HTTP Host.
func addConstraintPlanCases(
	plan *genericPlan,
	seed genericPlanSeed,
	actor genericActor,
	primary genericResourceInput,
) error {
	constraintEvidenceStart := len(plan.Cases)
	resource := func(handle string, probe stableGenericConstraintProbe, desired map[string]any) (genericResourceInput, error) {
		return genericPlanResource(seed, handle, probe.FormRef, probe.Name, desired)
	}
	relation := func(ref FormRef, name string) map[string]any {
		return map[string]any{"apiVersion": ref.APIVersion, "kind": ref.Kind, "name": name}
	}
	appendCreate := func(id string, input genericResourceInput, checks ...string) genericResourceInput {
		prepareHandle := id + "-prepare"
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id + "-prepare", Actor: actor,
			Command: genericCommand{Admission: &genericAdmissionRequest{
				Action: genericAdmissionPrepare, PreparationHandle: prepareHandle, Resource: input,
			}},
			Expected: genericExpected{Code: "ok", PreparationHandle: prepareHandle},
		})
		applied := input
		applied.Create = true
		applied.PreparationHandle = prepareHandle
		applied.IdempotencyKey = "generic-plan-" + id
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id + "-apply", Actor: actor,
			Command: genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: applied}},
			Expected: genericExpected{
				Code: "created", ResourceHandle: input.Handle, Generation: "1", Revision: "1",
			},
			Checks: checks,
		})
		return applied
	}
	appendValidateInvalid := func(id string, input genericResourceInput, checks ...string) {
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id, Actor: actor,
			Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: input}},
			Expected: genericExpected{Code: "ok", Valid: genericBool(false), HasDiagnostics: genericBool(true)},
			Checks:   checks,
		})
	}
	appendValidateValid := func(id string, input genericResourceInput, checks ...string) {
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id, Actor: actor,
			Command:  genericCommand{Admission: &genericAdmissionRequest{Action: genericAdmissionValidate, Resource: input}},
			Expected: genericExpected{Code: "ok", Valid: genericBool(true), HasDiagnostics: genericBool(false)},
			Checks:   checks,
		})
	}
	appendPrepareInvalid := func(id string, input genericResourceInput, checks ...string) {
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id, Actor: actor,
			Command: genericCommand{Admission: &genericAdmissionRequest{
				Action: genericAdmissionPrepare, PreparationHandle: id + "-prepare", Resource: input,
			}},
			Expected: genericExpected{Code: "invalid_argument"},
			Checks:   checks,
		})
	}
	appendApplyInvalid := func(id string, input genericResourceInput, checks ...string) {
		prepareHandle := id + "-prepare"
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id + "-prepare", Actor: actor,
			Command: genericCommand{Admission: &genericAdmissionRequest{
				Action: genericAdmissionPrepare, PreparationHandle: prepareHandle, Resource: input,
			}},
			Expected: genericExpected{Code: "ok", PreparationHandle: prepareHandle},
		})
		applied := input
		applied.Create = true
		applied.PreparationHandle = prepareHandle
		applied.IdempotencyKey = "generic-plan-" + id
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id + "-apply", Actor: actor,
			Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: applied}},
			Expected: genericExpected{Code: "invalid_argument"},
			Checks:   checks,
		})
	}
	appendDelete := func(id string, input genericResourceInput, external bool) {
		deleted := input
		deleted.ExpectedGeneration = "1"
		deleted.IdempotencyKey = "generic-plan-" + id
		controls := genericControls{}
		if external {
			controls.BackendEffect = genericBackendEffectExternalChange
		}
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: id, Actor: actor,
			Command: genericCommand{
				Resource: &genericResourceRequest{Action: genericResourceDelete, Resource: deleted},
				Controls: controls,
			},
			Expected: genericExpected{Code: "deleted"},
		})
	}

	summed, err := resource("constraint-sum", seed.Probe.ConstraintSemantics.Sum, map[string]any{
		"weights": []any{map[string]any{"weight": 40}, map[string]any{"weight": 60}},
	})
	if err != nil {
		return err
	}
	summed.Name = "constraint-sum"
	appendValidateValid("constraint-sum-valid", summed)
	summedInvalid := summed
	summedInvalid.Handle = "constraint-sum-invalid"
	summedInvalid.Name = "constraint-sum-invalid"
	summedInvalid.Desired = map[string]any{
		"weights": []any{map[string]any{"weight": 40}, map[string]any{"weight": 50}},
	}
	appendValidateInvalid("constraint-sum-invalid", summedInvalid)

	claimPrimary, err := resource(
		"constraint-claim-primary", seed.Probe.ConstraintSemantics.ClaimPrimary,
		map[string]any{"claim": "shared-claim"},
	)
	if err != nil {
		return err
	}
	claimPrimary.Name = "constraint-claim-primary"
	claimAlternate, err := resource(
		"constraint-claim-alternate", seed.Probe.ConstraintSemantics.ClaimAlternate,
		map[string]any{"alias": "shared-claim"},
	)
	if err != nil {
		return err
	}
	claimAlternate.Name = "constraint-claim-alternate"
	claimAlternate.Space = seed.Contract.RunnerInput.AlternateSpace
	for _, candidate := range []struct {
		id    string
		input genericResourceInput
	}{
		{"constraint-claim-primary", claimPrimary},
		{"constraint-claim-alternate", claimAlternate},
	} {
		plan.Cases = append(plan.Cases, genericPlanCase{
			ID: candidate.id + "-prepare-before-holder", Actor: actor,
			Command: genericCommand{Admission: &genericAdmissionRequest{
				Action: genericAdmissionPrepare, PreparationHandle: candidate.id + "-prepare-before-holder", Resource: candidate.input,
			}},
			Expected: genericExpected{Code: "ok", PreparationHandle: candidate.id + "-prepare-before-holder"},
		})
	}
	claimPrimaryCreate := claimPrimary
	claimPrimaryCreate.Create = true
	claimPrimaryCreate.PreparationHandle = "constraint-claim-primary-prepare-before-holder"
	claimPrimaryCreate.IdempotencyKey = "generic-plan-constraint-claim-primary"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "constraint-claim-primary-apply", Actor: actor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: claimPrimaryCreate}},
		Expected: genericExpected{Code: "created", ResourceHandle: claimPrimary.Handle, Generation: "1", Revision: "1"},
	})
	claimAlternateConflict := claimAlternate
	claimAlternateConflict.Create = true
	claimAlternateConflict.PreparationHandle = "constraint-claim-alternate-prepare-before-holder"
	claimAlternateConflict.IdempotencyKey = "generic-plan-constraint-claim-alternate-conflict"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "constraint-claim-alternate-conflict", Actor: actor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: claimAlternateConflict}},
		Expected: genericExpected{Code: "invalid_argument"},
	})
	appendDelete("constraint-claim-primary-delete", claimPrimary, false)
	appendCreate("constraint-claim-alternate-after-release", claimAlternate)

	hostAssigned, err := genericPlanResource(
		seed, "constraint-host-assigned-substitute", seed.Probe.Resources.Output.FormRef,
		"constraint-host-assigned-substitute", map[string]any{
			"label": "substitute", "assignedName": "author-chosen.publisher.example",
		},
	)
	if err != nil {
		return err
	}
	appendValidateInvalid("constraint-host-assigned-substitute", hostAssigned)

	structural, err := resource("constraint-structural", seed.Probe.ConstraintSemantics.Structural, map[string]any{
		"lower": 1, "upper": 2,
		"rows": []any{
			map[string]any{"key": "duplicate", "value": 1},
			map[string]any{"key": "duplicate", "value": 2},
		},
	})
	if err != nil {
		return err
	}
	appendValidateInvalid("constraint-unique-by-duplicate", structural)

	nodeA, err := resource("constraint-node-a", seed.Probe.ConstraintSemantics.Node, map[string]any{})
	if err != nil {
		return err
	}
	nodeA.Name = "constraint-node-a"
	nodeB, err := resource("constraint-node-b", seed.Probe.ConstraintSemantics.Node, map[string]any{})
	if err != nil {
		return err
	}
	nodeB.Name = "constraint-node-b"
	appendCreate("constraint-node-a", nodeA)
	appendCreate("constraint-node-b", nodeB)

	distinctMissing, err := resource("constraint-distinct-missing", seed.Probe.ConstraintSemantics.DistinctPair, map[string]any{
		"left": relation(nodeA.Ref, nodeA.Name),
	})
	if err != nil {
		return err
	}
	distinctMissing.Name = "constraint-distinct-missing"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "constraint-distinct-missing-prepare", Actor: actor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "constraint-distinct-missing-prepare", Resource: distinctMissing,
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "constraint-distinct-missing-prepare"},
	})
	distinctSame := distinctMissing
	distinctSame.Handle = "constraint-distinct-same"
	distinctSame.Name = "constraint-distinct-same"
	distinctSame.Desired = map[string]any{
		"left":  relation(nodeA.Ref, nodeA.Name),
		"right": relation(nodeA.Ref, nodeA.Name),
	}
	appendValidateInvalid("constraint-distinct-same-validate", distinctSame)

	pairAB := map[string]any{
		"left":  relation(nodeA.Ref, nodeA.Name),
		"right": relation(nodeB.Ref, nodeB.Name),
	}
	uniqueOne, err := resource("constraint-unique-one", seed.Probe.ConstraintSemantics.UniquePair, pairAB)
	if err != nil {
		return err
	}
	uniqueOne.Name = "constraint-unique-one"
	appendCreate("constraint-unique-one", uniqueOne)
	uniqueDuplicate := uniqueOne
	uniqueDuplicate.Handle = "constraint-unique-duplicate"
	uniqueDuplicate.Name = "constraint-unique-duplicate"
	appendPrepareInvalid("constraint-unique-duplicate-prepare", uniqueDuplicate)
	appendDelete("constraint-unique-one-delete", uniqueOne, false)
	appendCreate("constraint-unique-after-release", uniqueDuplicate)
	uniqueReversed, err := resource("constraint-unique-reversed", seed.Probe.ConstraintSemantics.UniquePair, map[string]any{
		"left":  relation(nodeB.Ref, nodeB.Name),
		"right": relation(nodeA.Ref, nodeA.Name),
	})
	if err != nil {
		return err
	}
	uniqueReversed.Name = "constraint-unique-reversed"
	appendCreate("constraint-unique-reversed", uniqueReversed)
	uniqueSecond, err := resource("constraint-unique-second", seed.Probe.ConstraintSemantics.UniquePairSecond, pairAB)
	if err != nil {
		return err
	}
	uniqueSecond.Name = "constraint-unique-second"
	appendCreate("constraint-unique-second", uniqueSecond)

	nodeBLinked := nodeB
	nodeBLinked.Desired = map[string]any{"next": relation(nodeA.Ref, nodeA.Name)}
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "constraint-node-b-link-prepare", Actor: actor,
		Command: genericCommand{Admission: &genericAdmissionRequest{
			Action: genericAdmissionPrepare, PreparationHandle: "constraint-node-b-link-prepare",
			Resource: nodeBLinked, ExpectedGeneration: "1",
		}},
		Expected: genericExpected{Code: "ok", PreparationHandle: "constraint-node-b-link-prepare"},
	})
	nodeBLinked.ExpectedGeneration = "1"
	nodeBLinked.PreparationHandle = "constraint-node-b-link-prepare"
	nodeBLinked.IdempotencyKey = "generic-plan-constraint-node-b-link"
	plan.Cases = append(plan.Cases, genericPlanCase{
		ID: "constraint-node-b-link-apply", Actor: actor,
		Command:  genericCommand{Resource: &genericResourceRequest{Action: genericResourceApply, Resource: nodeBLinked}},
		Expected: genericExpected{Code: "ok", ResourceHandle: nodeB.Handle, Generation: "2", Revision: "2"},
	})
	nodeCycle := nodeA
	nodeCycle.Desired = map[string]any{"next": relation(nodeB.Ref, nodeB.Name)}
	appendPrepareInvalid("constraint-node-cycle-prepare", nodeCycle)
	nodeSelf := nodeA
	nodeSelf.Desired = map[string]any{"next": relation(nodeA.Ref, nodeA.Name)}
	appendValidateInvalid("constraint-node-self-validate", nodeSelf)

	memberOne, err := resource("constraint-member-one", seed.Probe.ConstraintSemantics.Member, map[string]any{
		"through": relation(nodeA.Ref, nodeA.Name),
	})
	if err != nil {
		return err
	}
	memberOne.Name = "constraint-member-one"
	memberTwo := memberOne
	memberTwo.Handle = "constraint-member-two"
	memberTwo.Name = "constraint-member-two"
	appendCreate("constraint-member-one", memberOne)
	appendCreate("constraint-member-two", memberTwo)
	sameValid, err := resource("constraint-same-valid", seed.Probe.ConstraintSemantics.SameTarget, map[string]any{
		"anchor": relation(nodeA.Ref, nodeA.Name),
		"members": []any{
			relation(memberOne.Ref, memberOne.Name), relation(memberTwo.Ref, memberTwo.Name),
		},
	})
	if err != nil {
		return err
	}
	sameValid.Name = "constraint-same-valid"
	appendCreate("constraint-same-valid", sameValid)
	sameMismatch := sameValid
	sameMismatch.Handle = "constraint-same-mismatch"
	sameMismatch.Name = "constraint-same-mismatch"
	sameMismatch.Desired = map[string]any{
		"anchor": relation(nodeB.Ref, nodeB.Name),
		"members": []any{
			relation(memberOne.Ref, memberOne.Name), relation(memberTwo.Ref, memberTwo.Name),
		},
	}
	appendValidateInvalid("constraint-same-mismatch-validate", sameMismatch)

	appendDelete("constraint-node-a-external-delete", nodeA, true)
	appendCreate("constraint-node-a-recreate", nodeA)
	plan.Cases[len(plan.Cases)-1].Expected.DifferentUIDFrom = "constraint-node-a-apply"
	driftNode := nodeA
	driftNode.Handle = "constraint-node-drift"
	driftNode.Name = "constraint-node-drift"
	driftNode.Desired = map[string]any{"next": relation(nodeB.Ref, nodeB.Name)}
	appendPrepareInvalid("constraint-node-replacement-drift-prepare", driftNode)
	sameDrift := sameValid
	sameDrift.Handle = "constraint-same-drift"
	sameDrift.Name = "constraint-same-drift"
	sameDrift.Desired = map[string]any{
		"anchor":  relation(nodeA.Ref, nodeA.Name),
		"members": []any{relation(memberOne.Ref, memberOne.Name)},
	}
	appendValidateInvalid("constraint-same-replacement-drift-validate", sameDrift, "declared-constraint-semantics-enforced")
	uniqueReplacement := uniqueOne
	uniqueReplacement.Handle = "constraint-unique-replacement"
	uniqueReplacement.Name = "constraint-unique-replacement"
	appendCreate("constraint-unique-replacement", uniqueReplacement)

	exclusiveEvidenceStart := len(plan.Cases)
	lease, err := genericPlanResource(
		seed, "constraint-lease-holder", seed.Probe.Resources.Lease.FormRef,
		seed.Probe.Resources.Lease.Name, seed.Probe.Resources.Lease.Desired,
	)
	if err != nil {
		return err
	}
	appendCreate("constraint-lease-holder", lease)
	leaseRival := lease
	leaseRival.Handle = "constraint-lease-rival"
	leaseRival.Name = "constraint-lease-rival"
	appendApplyInvalid("constraint-lease-rival", leaseRival)
	exclusiveSecond, err := resource(
		"constraint-lease-second", seed.Probe.ConstraintSemantics.ExclusiveSecond,
		map[string]any{"target": relation(primary.Ref, primary.Name)},
	)
	if err != nil {
		return err
	}
	exclusiveSecond.Name = "constraint-lease-second"
	appendCreate("constraint-lease-second-exact-form", exclusiveSecond)

	sequence, err := genericPlanResource(
		seed, "constraint-sequence", seed.Probe.Resources.Sequenced.FormRef,
		seed.Probe.Resources.Sequenced.Name, seed.Probe.Resources.Sequenced.Desired,
	)
	if err != nil {
		return err
	}
	appendCreate("constraint-sequence", sequence)
	reservation, err := genericPlanResource(
		seed, "constraint-reservation-holder", seed.Probe.Resources.Reservation.FormRef,
		seed.Probe.Resources.Reservation.Name, seed.Probe.Resources.Reservation.Desired,
	)
	if err != nil {
		return err
	}
	appendCreate("constraint-reservation-holder", reservation)
	reservationRival := reservation
	reservationRival.Handle = "constraint-reservation-rival"
	reservationRival.Name = "constraint-reservation-rival"
	appendApplyInvalid("constraint-reservation-rival", reservationRival, "declared-exclusive-holds-enforced")
	for index := constraintEvidenceStart; index < len(plan.Cases); index++ {
		plan.Cases[index].Checks = append(plan.Cases[index].Checks, "declared-constraint-semantics-enforced")
	}
	for index := exclusiveEvidenceStart; index < len(plan.Cases); index++ {
		plan.Cases[index].Checks = append(plan.Cases[index].Checks, "declared-exclusive-holds-enforced")
	}
	return nil
}

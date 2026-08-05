import { describe, expect, test } from "bun:test";

import {
  derivePublicationTruth,
  validatePublicationClaimText,
} from "./publication-truth.mjs";

function fixture() {
  return {
    publicationSet: {
      format: "takoform.form-package-publication-set@v1",
      generation: "portable-v1",
      admissionStatus: "external-required",
      publicationStatus: "published-immutable",
      entries: [
        {
          formRef: {
            apiVersion: "forms.takoform.com/v1alpha1",
            definitionVersion: "1.0.0",
            kind: "Example",
            schemaDigest: `sha256:${"a".repeat(64)}`,
          },
          immutable: true,
          kind: "Example",
          packageDigest: `sha256:${"b".repeat(64)}`,
        },
      ],
    },
    releaseVersion: {
      providerAddress: "registry.terraform.io/tako0614/takoform",
      publicationStatus: "candidate-only",
      tag: "v2.0.0",
      version: "2.0.0",
    },
    providerIdentities: {
      format: "takoform.provider-release-identities@v1",
      entries: [
        { status: "assigned", tag: "v1.0.1", version: "1.0.1" },
        { commit: "a".repeat(40), status: "assigned", tag: "v2.0.0", version: "2.0.0" },
      ],
    },
    providerReadback: {
      version: "2.0.0",
      tag: "v2.0.0",
      commit: "a".repeat(40),
      providerAddress: "registry.terraform.io/tako0614/takoform",
      publicationReady: true,
      certificateIdentity: "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/provider-registry-readback.yml@refs/heads/main",
      products: ["OpenTofu", "Terraform"],
      providerBinaryDigest: `sha256:${"c".repeat(64)}`,
    },
  };
}

describe("publication truth", () => {
  test("keeps publication independent from maturity and admission", () => {
    const truth = derivePublicationTruth(fixture());
    expect(truth.publishedCount).toBe(1);
    expect(truth.providerVersion).toBe("2.0.0");
    expect(truth.legacyProviderVersion).toBe("1.0.1");
    expect(truth).not.toHaveProperty("admittedCount");
    expect(truth).not.toHaveProperty("admissionTag");
  });

  test("accepts explicit Experimental and Legacy positioning", () => {
    const truth = derivePublicationTruth(fixture());
    expect(() => validatePublicationClaimText(
      "Takoform is an Experimental specification project. " +
      "The 1 published Form Package identity is immutable Legacy evidence. " +
      "There is no current central approval or admission.",
      truth,
    )).not.toThrow();
  });

  test("accepts an explicit denial of central approval", () => {
    const truth = derivePublicationTruth(fixture());
    expect(() => validatePublicationClaimText(
      "Takoform is an Experimental specification project. " +
      "The 1 published Form Package identity is immutable Legacy evidence. " +
      "There is no current central candidate set or centrally approved subset.",
      truth,
    )).not.toThrow();
  });

  test("rejects a current portable-standard claim", () => {
    const truth = derivePublicationTruth(fixture());
    expect(() => validatePublicationClaimText(
      "Takoform is an Experimental specification project. " +
      "The 1 published Form Package identity is immutable Legacy evidence. " +
      "There is no current central approval. The Form is portable-standard.",
      truth,
    )).toThrow("historical admission field");
  });
});

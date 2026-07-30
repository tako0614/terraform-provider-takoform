import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { afterEach, describe, expect, test } from "bun:test";

import {
  derivePublicationTruth,
  loadPublicationTruth,
  validatePublicationClaimText,
} from "./publication-truth.mjs";

const temporaryDirectories = [];

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

function formRef(kind, definitionVersion, suffix) {
  return {
    apiVersion: "forms.example.test/v1alpha1",
    definitionVersion,
    kind,
    schemaDigest: `sha256:${suffix.repeat(64)}`,
  };
}

function fixture() {
  const commit = "a".repeat(40);
  const toolingCommit = "b".repeat(40);
  const alpha = {
    formRef: formRef("Alpha", "2.0.0", "1"),
    immutable: true,
    kind: "Alpha",
    packageDigest: `sha256:${"2".repeat(64)}`,
    peeledCommit: commit,
    releaseId: "alpha",
    tag: "forms/alpha/v5.0.0",
    toolingCommit,
    version: "5.0.0",
  };
  const beta = {
    formRef: formRef("Beta", "1.0.0", "3"),
    immutable: true,
    kind: "Beta",
    packageDigest: `sha256:${"4".repeat(64)}`,
    peeledCommit: commit,
    releaseId: "beta",
    tag: "forms/beta/v1.0.0",
    toolingCommit,
    version: "1.0.0",
  };
  const releaseVersion = {
    cliMatrix: [
      {
        product: "Client A",
        providerAddress: "registry.example.test/example/takoform",
        version: "3.2.1",
      },
      {
        product: "Client B",
        providerAddress: "registry.example.test/example/takoform",
        version: "4.5.6",
      },
    ],
    providerAddress: "registry.example.test/example/takoform",
    publicationStatus: "candidate-only",
    tag: "v9.2.0",
    version: "9.2.0",
    versioning: {
      portableApiVersion: "forms.example.test/v1alpha1",
    },
  };
  const providerReadback = {
    format: "takoform.provider-registry-readback@v1",
    installs: releaseVersion.cliMatrix.map((entry) => ({
      cliVersion: entry.version,
      product: entry.product,
      providerAddress: entry.providerAddress,
      providerBinarySha256: `sha256:${"5".repeat(64)}`,
      providerSchemaSha256: `sha256:${"6".repeat(64)}`,
      providerVersion: releaseVersion.version,
    })),
    providerAddress: releaseVersion.providerAddress,
    providerReleaseTag: releaseVersion.tag,
    providerVersion: releaseVersion.version,
    publicationReady: true,
  };
  const admissionEntry = {
    admissionStatus: "portable-standard",
    formRef: alpha.formRef,
    kind: alpha.kind,
    packageDigest: alpha.packageDigest,
    releaseCommit: alpha.peeledCommit,
    releaseTag: alpha.tag,
    releaseToolingCommit: alpha.toolingCommit,
  };
  const providerReports = [alpha, beta].map((entry) => ({
    identity: {
      formRef: entry.formRef,
      packageDigest: entry.packageDigest,
    },
    kind: entry.kind,
  }));
  return {
    admissionSet: {
      admissionReleaseTag: "forms/admissions/v7.4.1",
      entries: [admissionEntry],
      format: "takoform.standard-admission-set@v3",
      generation: "example-generation",
      providerRegistryReadback: {
        digest: "",
        path: "registry/provider-readback.json",
        sigstoreBundle: "registry/provider-readback.sigstore.json",
      },
      providerReportClosure: {
        generation: "example-publication",
        reports: providerReports,
      },
    },
    checkpoint: {
      format: "takoform.standard-admission-checkpoint@v1",
      generation: "example-generation",
      retainedRoot: "admission/v4",
      tag: "forms/admissions/v7.4.1",
      version: "7.4.1",
    },
    providerReadback,
    publicationSet: {
      admissionStatus: "external-required",
      entries: [alpha, beta],
      format: "takoform.form-package-publication-set@v1",
      generation: "example-publication",
      publicationStatus: "published-immutable",
    },
    releaseVersion,
  };
}

describe("publication truth", () => {
  test("derives counts and identities from retained evidence", () => {
    const material = fixture();
    expect(derivePublicationTruth(material)).toEqual({
      admissionTag: "forms/admissions/v7.4.1",
      admittedCount: 1,
      admittedKinds: ["Alpha"],
      apiVersion: "forms.example.test/v1alpha1",
      providerAddress: "registry.example.test/example/takoform",
      providerVersion: "9.2.0",
      publishedCount: 2,
      publishedKinds: ["Alpha", "Beta"],
      remainingCount: 1,
    });
  });

  test("rejects an admission identity outside the published set", () => {
    const material = fixture();
    material.admissionSet.entries[0].kind = "Gamma";
    material.admissionSet.entries[0].formRef = formRef("Gamma", "1.0.0", "7");
    expect(() => derivePublicationTruth(material)).toThrow(
      "standard admission kind Gamma is not in the published package set",
    );
  });

  test("rejects a provider closure that omits a published Form", () => {
    const material = fixture();
    material.admissionSet.providerReportClosure.reports.pop();
    expect(() => derivePublicationTruth(material)).toThrow(
      "provider report closure must cover every published Form Package kind",
    );
  });

  test("loads the referenced Registry readback and verifies its digest", () => {
    const material = fixture();
    const repository = mkdtempSync(
      path.join(tmpdir(), "takoform-publication-truth-"),
    );
    temporaryDirectories.push(repository);
    const admissionRoot = path.join(repository, "admission", "v4");
    const registryRoot = path.join(admissionRoot, "registry");
    mkdirSync(registryRoot, { recursive: true });
    mkdirSync(path.join(repository, "release"), { recursive: true });

    const readbackRaw = Buffer.from(
      `${JSON.stringify(material.providerReadback)}\n`,
    );
    material.admissionSet.providerRegistryReadback.digest =
      `sha256:${createHash("sha256").update(readbackRaw).digest("hex")}`;
    writeFileSync(
      path.join(admissionRoot, "form-package-publication-set.json"),
      `${JSON.stringify(material.publicationSet)}\n`,
    );
    writeFileSync(
      path.join(admissionRoot, "version.json"),
      `${JSON.stringify(material.checkpoint)}\n`,
    );
    writeFileSync(
      path.join(admissionRoot, "standard-admission-set.json"),
      `${JSON.stringify(material.admissionSet)}\n`,
    );
    writeFileSync(
      path.join(registryRoot, "provider-readback.json"),
      readbackRaw,
    );
    writeFileSync(
      path.join(repository, "release", "version.json"),
      `${JSON.stringify(material.releaseVersion)}\n`,
    );

    expect(() => loadPublicationTruth(repository)).toThrow();
    writeFileSync(
      path.join(registryRoot, "provider-readback.sigstore.json"),
      "{}\n",
    );
    expect(loadPublicationTruth(repository).admittedKinds).toEqual(["Alpha"]);
  });

  test("binds publication and admission counts to their meanings", () => {
    const truth = derivePublicationTruth(fixture());
    const copy = [
      "All 2 Form Packages are published and immutable.",
      "Exactly 1 is admitted portable-standard.",
      "The remaining 1 is published but not admitted.",
    ].join(" ");
    expect(validatePublicationClaimText(copy, truth)).toBe(true);
    expect(() =>
      validatePublicationClaimText(
        "2 Forms exist. Exactly 1 is published. The other 1 is admitted.",
        truth,
      ),
    ).toThrow("published count is not bound");
    expect(() =>
      validatePublicationClaimText(
        `${copy} All 2 Form Packages are unpublished.`,
        truth,
      ),
    ).toThrow("contradicts the all-Form publication claim");
  });
});

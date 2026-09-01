#!/usr/bin/env bun

import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

export const PROVIDER4_DESCRIPTOR = "release/candidates/provider-v4.0.0.json";
export const PROVIDER4_IDENTITIES =
  "release/candidates/provider-v4.0.0-form-identities.json";
const PROVIDER3_DESCRIPTOR = "release/version.json";
const PROVIDER3_HISTORY_DESCRIPTOR = "release/history/provider-v3.0.0.json";
const PROVIDER_HISTORY_IDENTITIES = "release/provider-form-identities.json";
const PUBLISHER_CLOSURE = "internal/provider/artifacts/publisher/closure.json";
const PUBLISHER_ROOT = "internal/provider/artifacts/publisher";
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const RESOURCE_TYPE = /^takoform_[a-z0-9_]+$/u;
const PUBLISHER_FAMILY = "edge.forms.takoform.com";

function fail(message) {
  throw new Error(`Provider 4 candidate: ${message}`);
}

function readJson(root, relativePath) {
  try {
    return JSON.parse(readFileSync(path.join(root, relativePath), "utf8"));
  } catch (error) {
    fail(`${relativePath} is not readable JSON (${error.message})`);
  }
}

function recursivelySorted(value) {
  if (Array.isArray(value)) return value.map(recursivelySorted);
  if (value === null || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, recursivelySorted(value[key])]),
  );
}

function canonicalDigest(value) {
  return `sha256:${createHash("sha256")
    .update(JSON.stringify(recursivelySorted(value)))
    .digest("hex")}`;
}

function formRefKey(ref) {
  return JSON.stringify([
    ref?.apiVersion,
    ref?.kind,
    ref?.definitionVersion,
    ref?.schemaDigest,
  ]);
}

function validateDescriptor(descriptor) {
  if (
    descriptor?.schemaVersion !== 1 ||
    descriptor.version !== "4.0.0" ||
    descriptor.tag !== "v4.0.0" ||
    descriptor.sourceRepository !==
      "github.com/tako0614/terraform-provider-takoform" ||
    descriptor.formPublisherRepository !==
      "github.com/tako0614/takoform-forms" ||
    descriptor.formPublisherCommit !==
      "3231633605b737ce5279d7fc020b4780568e7091" ||
    descriptor.formSetTag !==
      "forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c" ||
    descriptor.providerAddress !==
      "registry.terraform.io/tako0614/takoform" ||
    descriptor.publicationStatus !== "candidate-only" ||
    descriptor.versioning?.providerCompatibility !== "semver-major" ||
    descriptor.versioning?.portableApiVersion !== "forms.takoform.com/v1" ||
    descriptor.versioning?.formDefinitionVersions !==
      "independent-immutable-semver" ||
    !Array.isArray(descriptor.cliMatrix) ||
    descriptor.cliMatrix.length !== 2
  ) {
    fail(`${PROVIDER4_DESCRIPTOR} is not the exact unpublished Provider 4 descriptor`);
  }
}

export function generateProvider4CandidateIdentities(root) {
  const descriptor = readJson(root, PROVIDER4_DESCRIPTOR);
  validateDescriptor(descriptor);

  const provider3 = readFileSync(path.join(root, PROVIDER3_DESCRIPTOR));
  const provider3History = readFileSync(
    path.join(root, PROVIDER3_HISTORY_DESCRIPTOR),
  );
  if (!provider3.equals(provider3History)) {
    fail("the retired Provider 3 descriptor and its history copy differ");
  }
  const provider3Document = JSON.parse(provider3.toString("utf8"));
  if (
    provider3Document.version !== "3.0.0" ||
    provider3Document.tag !== "v3.0.0"
  ) {
    fail("release/version.json no longer preserves the tombstoned Provider 3 writer input");
  }

  const history = readJson(root, PROVIDER_HISTORY_IDENTITIES);
  if (
    history.format !== "takoform.provider-form-identities@v1" ||
    history.releases?.some((release) => release?.providerVersion === "4.0.0") ||
    history.releases?.find((release) => release?.providerVersion === "3.0.0")
      ?.forms?.length !== 31
  ) {
    fail("the retired Provider identity ledger is not an exact Provider 3 history surface");
  }

  const closure = readJson(root, PUBLISHER_CLOSURE);
  if (
    closure.format !== "takoform.provider-publisher-set-artifact-closure@v1" ||
    closure.projection?.path !== "projection.json" ||
    !SHA256.test(closure.projection?.digest ?? "") ||
    closure.packages?.length !== 17
  ) {
    fail(`${PUBLISHER_CLOSURE} is not the exact 17-package publisher closure`);
  }
  const projection = readJson(
    root,
    path.posix.join(PUBLISHER_ROOT, closure.projection.path),
  );
  if (
    canonicalDigest(projection) !== closure.projection.digest ||
    projection.format !== "takoform.provider-publisher-set-projection@v1" ||
    projection.hostApi !== descriptor.versioning.portableApiVersion
  ) {
    fail("the publisher-selected Provider projection identity or digest drifted");
  }

  const packages = new Map();
  for (const entry of closure.packages) {
    if (
      entry?.formRef?.apiVersion !== PUBLISHER_FAMILY ||
      !SHA256.test(entry?.formRef?.schemaDigest ?? "") ||
      !SHA256.test(entry?.packageDigest ?? "")
    ) {
      fail("the publisher package closure contains a non-Edge or malformed FormRef");
    }
    const key = formRefKey(entry.formRef);
    if (packages.has(key)) fail("the publisher package closure repeats a FormRef");
    packages.set(key, entry);
  }

  const resourceTypes = new Set();
  const forms = projection.resources
    .filter((entry) => entry?.register === true)
    .map((entry) => {
      const packageEntry = packages.get(formRefKey(entry.ref));
      if (
        packageEntry === undefined ||
        entry.ref?.packageDigest !== packageEntry.packageDigest ||
        !RESOURCE_TYPE.test(entry.resourceType ?? "") ||
        resourceTypes.has(entry.resourceType)
      ) {
        fail("the registered Provider projection is not an exact publisher package mapping");
      }
      resourceTypes.add(entry.resourceType);
      return {
        resourceType: entry.resourceType,
        formRef: {
          apiVersion: entry.ref.apiVersion,
          kind: entry.ref.kind,
          definitionVersion: entry.ref.definitionVersion,
          schemaDigest: entry.ref.schemaDigest,
        },
        packageDigest: entry.ref.packageDigest,
      };
    })
    .sort((left, right) => left.resourceType.localeCompare(right.resourceType));
  if (forms.length !== 17 || packages.size !== forms.length) {
    fail(`the Provider 4 candidate maps ${forms.length}/${packages.size} Forms, want 17/17`);
  }

  return {
    format: "takoform.provider-candidate-form-identities@v1",
    providerVersion: descriptor.version,
    formPublisherRepository: descriptor.formPublisherRepository,
    formPublisherCommit: descriptor.formPublisherCommit,
    formSetTag: descriptor.formSetTag,
    portableApiVersion: descriptor.versioning.portableApiVersion,
    families: [PUBLISHER_FAMILY],
    forms,
  };
}

export function validateProvider4Candidate(root) {
  const expected = generateProvider4CandidateIdentities(root);
  const actual = readJson(root, PROVIDER4_IDENTITIES);
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    fail(`${PROVIDER4_IDENTITIES} is stale; run bun run sync:provider4-candidate`);
  }
  return expected;
}

function main() {
  const mode = process.argv[2];
  if (!["--check", "--write"].includes(mode)) {
    fail("usage: bun scripts/provider4-candidate.mjs --check|--write");
  }
  const root = path.resolve(import.meta.dirname, "..");
  const candidate = generateProvider4CandidateIdentities(root);
  if (mode === "--write") {
    writeFileSync(
      path.join(root, PROVIDER4_IDENTITIES),
      `${JSON.stringify(candidate, null, 2)}\n`,
    );
  } else {
    validateProvider4Candidate(root);
  }
  process.stdout.write(
    `Provider 4 candidate: ${candidate.forms.length} publisher-selected Forms; Provider 3 writer remains tombstoned\n`,
  );
}

if (import.meta.main) main();

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const DESCRIPTOR_PATH = "release/version.json";
const SOURCE_REPOSITORY = "github.com/tako0614/terraform-provider-takoform";
const FORM_PUBLISHER_REPOSITORY = "github.com/tako0614/takoform-forms";
const FORM_PUBLISHER_COMMIT = "3231633605b737ce5279d7fc020b4780568e7091";
const FORM_SET_TAG =
  "forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c";
const PROVIDER_ADDRESS = "registry.terraform.io/tako0614/takoform";
const GO_MODULE = "github.com/tako0614/terraform-provider-takoform";
const SIGNING_FINGERPRINT =
  "3510E75E05BBCC303B92D77934FC18AC897FB709";
const PORTABLE_API_VERSION = "forms.takoform.com/v1";
const PROVIDER_PLATFORMS = [
  "darwin_amd64",
  "darwin_arm64",
  "linux_amd64",
  "linux_arm64",
  "windows_amd64",
];
const VERSIONING_KEYS = [
  "formDefinitionVersions",
  "formPackageVersions",
  "portableApiVersion",
  "providerCompatibility",
];
const STABLE_PROVIDER4_VERSION =
  /^4\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?![\s\S])/u;
const STABLE_PROVIDER5_VERSION =
  /^5\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?![\s\S])/u;
const V5_PUBLISHER_COMMIT = "cfd376b724803dd2e5d112669b84589c0de517e0";

function fail(message) {
  throw new Error(`Provider 4 release descriptor: ${message}`);
}

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function hasExactKeys(value, expected) {
  return (
    isRecord(value) &&
    JSON.stringify(Object.keys(value).sort()) ===
      JSON.stringify([...expected].sort())
  );
}

function assertStableProvider4Version(version) {
  if (typeof version !== "string" || !STABLE_PROVIDER4_VERSION.test(version)) {
    fail(
      "version must be an exact stable Provider 4.x SemVer (major 4, with no prerelease/build metadata)",
    );
  }
}

/**
 * Validate the Provider release descriptor without consulting the filesystem.
 *
 * The descriptor is the current release writer input.  It deliberately keeps
 * Provider, Host API, Form definition, and Form Package version axes separate;
 * only the Provider major lane is selected here.
 */
export function validateProvider4ReleaseDescriptor(value) {
  if (!isRecord(value)) {
    fail("descriptor must be a JSON object");
  }
  if (value.schemaVersion !== 1) {
    fail("schemaVersion must be 1");
  }
  assertStableProvider4Version(value.version);
  if (value.tag !== `v${value.version}`) {
    fail("tag must exactly equal v${value.version}");
  }
  if (
    value.sourceRepository !== SOURCE_REPOSITORY ||
    value.formPublisherRepository !== FORM_PUBLISHER_REPOSITORY ||
    value.formPublisherCommit !== FORM_PUBLISHER_COMMIT ||
    value.formSetTag !== FORM_SET_TAG ||
    value.providerAddress !== PROVIDER_ADDRESS ||
    value.goModule !== GO_MODULE ||
    value.signingFingerprint !== SIGNING_FINGERPRINT
  ) {
    fail("repository, publisher, provider address, module, or signer identity is invalid");
  }
  if (value.publicationStatus !== "candidate-only") {
    fail("publicationStatus must remain candidate-only");
  }
  if (
    !Array.isArray(value.platforms) ||
    JSON.stringify([...value.platforms].sort()) !==
      JSON.stringify([...PROVIDER_PLATFORMS].sort())
  ) {
    fail("platforms must be the exact Provider release platform set");
  }
  if (
    !hasExactKeys(value.versioning, VERSIONING_KEYS) ||
    value.versioning.providerCompatibility !== "semver-major" ||
    value.versioning.portableApiVersion !== PORTABLE_API_VERSION ||
    value.versioning.formDefinitionVersions !==
      "independent-immutable-semver" ||
    value.versioning.formPackageVersions !==
      "content-addressed-current-retained-legacy-semver"
  ) {
    fail("versioning must preserve the independent Provider/Form/API axes");
  }
  if (!Array.isArray(value.cliMatrix) || value.cliMatrix.length !== 2) {
    fail("cliMatrix must contain exactly OpenTofu and Terraform entries");
  }
  for (const entry of value.cliMatrix) {
    if (
      !isRecord(entry) ||
      entry.providerAddress !== PROVIDER_ADDRESS ||
      !["OpenTofu", "Terraform"].includes(entry.product) ||
      typeof entry.version !== "string" ||
      entry.version === ""
    ) {
      fail("cliMatrix contains an invalid provider/FQN entry");
    }
  }
  if (new Set(value.cliMatrix.map((entry) => entry.product)).size !== 2) {
    fail("cliMatrix must contain one unique OpenTofu and Terraform entry");
  }
  return value;
}

export function validateProvider5ReleaseDescriptor(value) {
  if (!hasExactKeys(value, [
    "schemaVersion", "version", "tag", "sourceRepository",
    "formPublisherRepository", "formPublisherCommit", "formIdentityModel",
    "providerAddress", "cliMatrix", "goModule", "goVersion",
    "signingFingerprint", "publicKeyPath", "platforms",
    "publicationStatus", "versioning",
  ]) || value.schemaVersion !== 2 ||
      typeof value.version !== "string" || !STABLE_PROVIDER5_VERSION.test(value.version) ||
      value.tag !== `v${value.version}`) {
    fail("v5 descriptor requires schemaVersion 2 and exact stable Provider 5.x tag");
  }
  if (value.sourceRepository !== SOURCE_REPOSITORY ||
      value.formPublisherRepository !== FORM_PUBLISHER_REPOSITORY ||
      value.formPublisherCommit !== V5_PUBLISHER_COMMIT ||
      value.formIdentityModel !== "exact-published-urls" ||
      Object.hasOwn(value, "formSetTag") ||
      value.providerAddress !== PROVIDER_ADDRESS || value.goModule !== GO_MODULE ||
      value.signingFingerprint !== SIGNING_FINGERPRINT ||
      value.publicKeyPath !== "release/keys/provider-signing-key.asc" ||
      value.publicationStatus !== "candidate-only") {
    fail("v5 repository, URL identity model, provider, signer, or candidate status is invalid");
  }
  if (!Array.isArray(value.platforms) ||
      JSON.stringify([...value.platforms].sort()) !== JSON.stringify([...PROVIDER_PLATFORMS].sort())) {
    fail("v5 platforms must be the exact Provider release platform set");
  }
  if (!hasExactKeys(value.versioning, VERSIONING_KEYS) ||
      value.versioning.providerCompatibility !== "semver-major" ||
      value.versioning.portableApiVersion !== "forms.takoform.com/v2" ||
      value.versioning.formDefinitionVersions !== "independent-immutable-semver" ||
      value.versioning.formPackageVersions !== "independent-from-form-url") {
    fail("v5 versioning must preserve independent Provider, Host API, and Form URL axes");
  }
  if (!Array.isArray(value.cliMatrix) || value.cliMatrix.length !== 2 ||
      new Set(value.cliMatrix.map((entry) => entry?.product)).size !== 2 ||
      value.cliMatrix.some((entry) => !isRecord(entry) ||
        !["OpenTofu", "Terraform"].includes(entry.product) ||
        entry.providerAddress !== PROVIDER_ADDRESS ||
        typeof entry.version !== "string" || entry.version === "")) {
    fail("v5 cliMatrix must contain one OpenTofu and one Terraform FQN entry");
  }
  return value;
}

export function providerCandidatePaths(version) {
  if (STABLE_PROVIDER4_VERSION.test(version)) return provider4CandidatePaths(version);
  if (!STABLE_PROVIDER5_VERSION.test(version)) fail("current version must be stable Provider 4.x or 5.x");
  const base = `release/candidates/provider-v${version}`;
  return { descriptorPath: `${base}.json`, formIdentitiesPath: `${base}-form-identities.json` };
}

/**
 * Derive all versioned candidate paths from one validated Provider 4 version.
 */
export function provider4CandidatePaths(version) {
  assertStableProvider4Version(version);
  const base = `release/candidates/provider-v${version}`;
  return {
    descriptorPath: `${base}.json`,
    formIdentitiesPath: `${base}-form-identities.json`,
  };
}

function parseDescriptor(bytes) {
  try {
    return JSON.parse(bytes.toString("utf8"));
  } catch (error) {
    fail(`release/version.json is not readable JSON (${error.message})`);
  }
}

/**
 * Read the current descriptor as the authority, then derive and verify the
 * corresponding versioned candidate descriptor.  No candidate path is chosen
 * before the current descriptor has passed pure validation.
 */
export function readCurrentProviderRelease(repoRoot) {
  if (typeof repoRoot !== "string" || repoRoot === "") {
    fail("repoRoot must be a non-empty filesystem path");
  }
  let descriptorBytes;
  try {
    descriptorBytes = readFileSync(join(repoRoot, DESCRIPTOR_PATH));
  } catch (error) {
    fail(`release/version.json is not readable (${error.message})`);
  }
  const parsed = parseDescriptor(descriptorBytes);
  const descriptor = parsed?.schemaVersion === 2
    ? validateProvider5ReleaseDescriptor(parsed)
    : validateProvider4ReleaseDescriptor(parsed);
  const { descriptorPath: candidateDescriptorPath, formIdentitiesPath: candidateFormIdentitiesPath } =
    providerCandidatePaths(descriptor.version);
  let candidateBytes;
  try {
    candidateBytes = readFileSync(join(repoRoot, candidateDescriptorPath));
  } catch (error) {
    fail(`${candidateDescriptorPath} is not readable (${error.message})`);
  }
  if (!descriptorBytes.equals(candidateBytes)) {
    fail(
      `release/version.json must be byte-identical to ${candidateDescriptorPath}`,
    );
  }
  return {
    descriptor,
    descriptorPath: DESCRIPTOR_PATH,
    candidateDescriptorPath,
    candidateFormIdentitiesPath,
  };
}

// Retained Provider 4.1.0 is immutable published history. Its descriptor is
// read from the versioned historical candidate, never selected for v5 writes.
export function readCurrentProvider4Release(repoRoot) {
  const candidateDescriptorPath = "release/candidates/provider-v4.1.0.json";
  const bytes = readFileSync(join(repoRoot, candidateDescriptorPath));
  if (createHash("sha256").update(bytes).digest("hex") !==
      "40cc9a96cfae2196dc45c1b0b597e2447344d5caf02b18149941b9be18608d60") {
    fail("immutable Provider 4.1.0 descriptor bytes changed");
  }
  const descriptor = validateProvider4ReleaseDescriptor(parseDescriptor(bytes));
  if (descriptor.version !== "4.1.0") fail("retained Provider 4.1.0 descriptor drifted");
  const candidateFormIdentitiesPath = provider4CandidatePaths(descriptor.version).formIdentitiesPath;
  if (createHash("sha256").update(readFileSync(join(repoRoot, candidateFormIdentitiesPath))).digest("hex") !==
      "22c56394444bcd58ad88291abc3ab44244522eb33133defcef210f61c43e186f") {
    fail("immutable Provider 4.1.0 Form identity bytes changed");
  }
  return {
    descriptor,
    descriptorPath: candidateDescriptorPath,
    candidateDescriptorPath,
    candidateFormIdentitiesPath,
  };
}

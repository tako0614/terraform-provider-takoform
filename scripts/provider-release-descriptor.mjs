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
export function readCurrentProvider4Release(repoRoot) {
  if (typeof repoRoot !== "string" || repoRoot === "") {
    fail("repoRoot must be a non-empty filesystem path");
  }
  let descriptorBytes;
  try {
    descriptorBytes = readFileSync(join(repoRoot, DESCRIPTOR_PATH));
  } catch (error) {
    fail(`release/version.json is not readable (${error.message})`);
  }
  const descriptor = validateProvider4ReleaseDescriptor(
    parseDescriptor(descriptorBytes),
  );
  const { descriptorPath: candidateDescriptorPath, formIdentitiesPath: candidateFormIdentitiesPath } =
    provider4CandidatePaths(descriptor.version);
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

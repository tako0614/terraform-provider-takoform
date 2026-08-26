#!/usr/bin/env bun

// Takoform has two independent release tracks. Specification 1.1 is normative
// and closes on one committed snapshot of the normative specification source.
// Candidate Forms, the reference suite, and the official Terraform Provider
// are useful implementation evidence, but none is normative release authority.
// External Hosts, operators, deployments, backends, and products are likewise
// not authorities for either result.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

import { createHardenedGitEnvironment } from "./deploy-safety.mjs";

export const PUBLICATION_EVIDENCE = "spec/publication-evidence.json";
export const EVIDENCE_FORMAT = "takoform.release-evidence@v3";
export const REPORT_FORMAT = "takoform.release-evidence-report@v3";
export const REPOSITORY_POLICY_FORMAT = "takoform.repository-authority@v2";
export const SPECIFICATION_SOURCE_FORMAT =
  "takoform.specification-source-snapshot@v2";
export const SPECIFICATION_VERSION = "1.1";
export const CANDIDATE_CORPUS_FORMAT =
  "takoform.multi-family-candidate-corpus@v1";
export const REFERENCE_CONFORMANCE_FORMAT =
  "takoform.reference-conformance-suite-evidence@v1";
export const PROVIDER_CONFORMANCE_FORMAT =
  "takoform.provider-v3-exact-conformance@v2";
export const PROVIDER_IDENTITY_FORMAT =
  "takoform.provider-v3-identity-lock@v2";
export const PROVIDER_COMPATIBILITY_FORMAT =
  "takoform.provider-v3-compatibility-migration-lock@v2";

export const FAMILY_INDEX_FORMAT = "takoform.current-family-index@v1";
export const CONFORMANCE_SUITE_FORMAT = "takoform.conformance-suite@v1";
export const FAMILY_INDEX_PATH = "forms/candidates/current-family-index.json";
export const CONFORMANCE_SUITE_PATH =
  "conformance/takoform-v1/manifest.json";
export const EDGE_FAMILY_GROUP = "edge.forms.takoform.com";
export const EDGE_CANDIDATE_SIZE = 16;
export const HOST_API_LANE = "forms.takoform.com/v1";
export const PACKAGE_ENVELOPE = "packages.forms.takoform.com/v1alpha5";
export const STANDARD_SERVICE_API = "standards.takoform.com/v1";
export const STANDARD_SERVICE_PROTOCOL_PATTERN =
  "^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?){2,}$";
export const S3_STANDARD_SERVICE_PROTOCOL = "com.amazonaws.s3";

export const SPECIFICATION_TRACK = "specification-v1";
export const PROVIDER_TRACK = "provider-3.0";
export const SPECIFICATION_PREREQUISITES = Object.freeze([
  "specification-source-snapshot",
]);
export const PROVIDER_PREREQUISITES = Object.freeze([
  "provider-v3-exact-conformance",
  "provider-v3-identity-lock",
  "provider-v3-compatibility-migration-lock",
]);

export const CANONICAL_ORIGIN =
  "https://github.com/tako0614/terraform-provider-takoform.git";
export const ALLOWED_REACHABLE_REFS = Object.freeze([
  "refs/remotes/origin/main",
  "refs/tags/v*",
  "refs/tags/specification/*",
]);
export const SPECIFICATION_RELEASE_LEDGER =
  "release/specification-releases.json";
export const RETAINED_HISTORY_PATH = "spec/publication-blockers.json";
export const RETAINED_HISTORY_SHA256 =
  "8bc708163e789b95833331a537abf1c455062179c0eef5b57c583c76b8d740e0";

const SPECIFICATION_ROOTS = Object.freeze(["spec"]);
const SPECIFICATION_EXCLUDED_PATHS = Object.freeze([
  PUBLICATION_EVIDENCE,
  RETAINED_HISTORY_PATH,
]);
export const PUBLICATION_EVIDENCE_PROJECTION_PATHS = Object.freeze([
  "website/static/spec/publication-evidence.json",
  "website/public/spec/publication-evidence.json",
]);
const EVIDENCE_PROJECTION_PATHS = Object.freeze([
  ...PUBLICATION_EVIDENCE_PROJECTION_PATHS,
  "website/static/release/specification-releases.json",
  "website/public/release/specification-releases.json",
]);
const REFERENCE_IMPLEMENTATION_ROOTS = Object.freeze(["."]);
const REFERENCE_IMPLEMENTATION_EXCLUDED_PATHS = Object.freeze([
  PUBLICATION_EVIDENCE,
  SPECIFICATION_RELEASE_LEDGER,
  // The compatibility report is a separately generated report, not release
  // evidence or reference implementation input.
  "release/specification-compatibility.json",
  ...EVIDENCE_PROJECTION_PATHS,
]);
const REFERENCE_EXECUTION_PATHS = Object.freeze([
  ".",
  `:(exclude)${PUBLICATION_EVIDENCE}`,
  `:(exclude)${SPECIFICATION_RELEASE_LEDGER}`,
  ":(exclude)release/specification-compatibility.json",
  ...EVIDENCE_PROJECTION_PATHS.map((relativePath) => `:(exclude)${relativePath}`),
]);
const PROVIDER_IMPLEMENTATION_ROOTS = Object.freeze([
  "cmd/worker-authoring-conformance",
  "formpackage",
  "internal/clientv3",
  "internal/currentformmodel",
  "internal/currentformselection",
  "internal/currentformsnapshot",
  "internal/provider",
  "internal/retainededgeformcatalog",
  "internal/workerauthoring",
  "go.mod",
  "go.sum",
]);
const PROVIDER_EXECUTION_ROOTS = Object.freeze([
  ...PROVIDER_IMPLEMENTATION_ROOTS,
  "forms/candidates",
  "interfaces/candidates",
  "bindings/candidates",
  "conformance/takoform-v1",
  "release/version.json",
  "release/provider-form-identities.json",
  "release/provider-release-identities.json",
  "release/migrations/v2-to-v3.md",
]);
const PROVIDER_COMMANDS = Object.freeze([
  Object.freeze([
    "go",
    "test",
    "-count=1",
    "./internal/provider",
    "./internal/currentformselection",
    "./internal/currentformsnapshot",
    "./internal/clientv3",
  ]),
  Object.freeze([
    "go",
    "run",
    "./cmd/worker-authoring-conformance",
    "matrix",
    "--opentofu",
    "tofu",
    "--terraform",
    "terraform",
  ]),
]);
export const PROVIDER_COMPATIBILITY_TESTS = Object.freeze([
  "TestProviderV3ReleaseLedgerMatchesExactProviderProjection",
  "TestFutureStableCodecDoesNotImplicitlyUpgradeBetaState",
  "TestV3Provider211RetainedGoldenLocksImmutableHistory",
  "TestRetainedProvider211DefinitionsRemainByteIdentical",
  "TestV3CodecTableCoversEverySupportedRef",
  "TestV3PerRefCodecEncodesTheStateRefFieldSet",
  "TestV3ReadAndDeleteDispatchOnTheStateFormRef",
  "TestV3StateRefWithNoCompiledCodecFailsClosed",
]);
const PROVIDER_COMPATIBILITY_COMMAND = Object.freeze([
  "go",
  "test",
  "-json",
  "-count=1",
  "./internal/provider",
  "./internal/currentformselection",
  "./internal/currentformsnapshot",
  "-run",
  `^(?:${PROVIDER_COMPATIBILITY_TESTS.join("|")})$`,
]);

const COMMIT = /^[0-9a-f]{40}$/u;
const SHA256 = /^[a-f0-9]{64}$/u;
const SHA256_ID = /^sha256:[a-f0-9]{64}$/u;
const KIND = /^[A-Z][A-Za-z0-9]{0,63}$/u;
const GROUP = /^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.forms\.takoform\.com$/u;
const SEMVER = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/u;
const FORBIDDEN_CURRENT_IDENTITIES = new Set([
  "ObjectBucket",
  "edge.objects",
  "module-worker.object-bucket",
]);

function fail(message) {
  throw new Error(`publication evidence: ${message}`);
}

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function exactKeys(value, expected, context) {
  if (!isRecord(value)) fail(`${context} must be an object`);
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  if (JSON.stringify(actual) !== JSON.stringify(wanted)) {
    fail(`${context} keys must be exactly ${wanted.join(", ")}`);
  }
}

function requiredKeys(value, expected, context) {
  if (!isRecord(value)) fail(`${context} must be an object`);
  for (const key of expected) {
    if (!Object.hasOwn(value, key)) fail(`${context} is missing ${key}`);
  }
}

function nonEmptyString(value, context) {
  if (typeof value !== "string" || value.trim() === "") {
    fail(`${context} must be a non-empty string`);
  }
  return value;
}

function exactArray(actual, expected, context) {
  if (!Array.isArray(actual) || JSON.stringify(actual) !== JSON.stringify(expected)) {
    fail(`${context} must be exactly ${JSON.stringify(expected)}`);
  }
}

function sortedUniqueStrings(value, context, { allowEmpty = false } = {}) {
  if (
    !Array.isArray(value) ||
    (!allowEmpty && value.length === 0) ||
    value.some((entry) => typeof entry !== "string" || entry === "") ||
    new Set(value).size !== value.length ||
    JSON.stringify(value) !== JSON.stringify([...value].sort())
  ) {
    fail(`${context} must be a ${allowEmpty ? "possibly empty " : "non-empty "}sorted unique string array`);
  }
  return value;
}

function sha256Bytes(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function canonicalize(value, context = "canonical JSON") {
  if (value === null || typeof value === "string" || typeof value === "boolean") {
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) fail(`${context} contains a non-finite number`);
    if (Number.isInteger(value) && !Number.isSafeInteger(value)) {
      fail(`${context} contains a non-safe integer`);
    }
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) {
    return `[${value.map((entry) => canonicalize(entry, context)).join(",")}]`;
  }
  if (!isRecord(value)) fail(`${context} contains an unsupported value`);
  return `{${Object.keys(value)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonicalize(value[key], context)}`)
    .join(",")}}`;
}

export function canonicalJson(value) {
  return canonicalize(value);
}

function canonicalDigest(value) {
  return sha256Bytes(Buffer.from(canonicalJson(value), "utf8"));
}

function validateSha256(value, context) {
  if (typeof value !== "string" || !SHA256.test(value)) {
    fail(`${context} must be 64 lowercase hexadecimal characters`);
  }
}

function validateSha256Id(value, context) {
  if (typeof value !== "string" || !SHA256_ID.test(value)) {
    fail(`${context} must be sha256:<64 lowercase hexadecimal characters>`);
  }
}

function digestHex(value, context) {
  if (typeof value === "string" && SHA256.test(value)) return value;
  if (typeof value === "string" && SHA256_ID.test(value)) return value.slice(7);
  fail(`${context} must be a bare or sha256:-prefixed SHA-256 digest`);
}

function assertRelativePath(value, context) {
  nonEmptyString(value, context);
  if (
    value.includes("\\") ||
    path.posix.isAbsolute(value) ||
    path.posix.normalize(value) !== value ||
    value.split("/").includes("..")
  ) {
    fail(`${context} must be a normalized repository-relative POSIX path`);
  }
}

function environmentWithoutGitHubAuthority(environment = process.env) {
  const sanitized = { ...environment };
  delete sanitized.GH_TOKEN;
  delete sanitized.GH_ENTERPRISE_TOKEN;
  delete sanitized.GITHUB_TOKEN;
  delete sanitized.GITHUB_ENTERPRISE_TOKEN;
  return sanitized;
}

function gitEnvironment() {
  const environment = createHardenedGitEnvironment(
    environmentWithoutGitHubAuthority(),
  );
  delete environment.SSH_ASKPASS;
  delete environment.SSH_ASKPASS_REQUIRE;
  delete environment.GCM_INTERACTIVE;
  return environment;
}

function git(repositoryRoot, args, encoding = "utf8") {
  try {
    return execFileSync("git", ["-C", repositoryRoot, ...args], {
      encoding,
      env: gitEnvironment(),
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch (error) {
    const detail = String(error.stderr ?? "").trim();
    fail(`git ${args.join(" ")} failed${detail ? `: ${detail}` : ""}`);
  }
}

function gitSucceeds(repositoryRoot, args) {
  try {
    execFileSync("git", ["-C", repositoryRoot, ...args], {
      env: gitEnvironment(),
      stdio: "ignore",
    });
    return true;
  } catch {
    return false;
  }
}

function assertWorktreePathsClean(repositoryRoot, paths, context) {
  const status = git(repositoryRoot, [
    "status",
    "--porcelain=v1",
    "--untracked-files=all",
    "--",
    ...paths,
  ]).trim();
  if (status !== "") fail(`${context} has uncommitted changes`);
}

function gitShowBytes(repositoryRoot, commit, relativePath, context = relativePath) {
  assertRelativePath(relativePath, context);
  try {
    return execFileSync("git", ["-C", repositoryRoot, "show", `${commit}:${relativePath}`], {
      encoding: "buffer",
      env: gitEnvironment(),
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch {
    fail(`${context} does not exist at committed source ${commit}`);
  }
}

function gitPathExists(repositoryRoot, commit, relativePath) {
  return gitSucceeds(repositoryRoot, ["cat-file", "-e", `${commit}:${relativePath}`]);
}

function parseJson(bytes, context) {
  try {
    return JSON.parse(Buffer.isBuffer(bytes) ? bytes.toString("utf8") : bytes);
  } catch (error) {
    fail(`${context} is not JSON: ${error.message}`);
  }
}

function committedJson(repositoryRoot, commit, relativePath, context = relativePath) {
  return parseJson(gitShowBytes(repositoryRoot, commit, relativePath, context), context);
}

function normalizeOriginUrl(value) {
  const trimmed = String(value ?? "").trim().replace(/\/+$/u, "");
  const scp = /^git@github\.com:(.+)$/u.exec(trimmed);
  let normalized = scp ? `https://github.com/${scp[1]}` : trimmed;
  normalized = normalized
    .replace(/^ssh:\/\/git@github\.com\//u, "https://github.com/")
    .replace(/^http:\/\/github\.com\//u, "https://github.com/");
  if (!normalized.endsWith(".git")) normalized += ".git";
  return normalized;
}

function localGitConfiguration(repositoryRoot) {
  const raw = String(
    git(repositoryRoot, ["config", "--local", "-z", "--list"]),
  );
  const configuration = new Map();
  const allowedCore = new Set([
    "core.bare",
    "core.filemode",
    "core.logallrefupdates",
    "core.repositoryformatversion",
  ]);
  for (const record of raw.split("\0")) {
    if (record === "") continue;
    const separator = record.indexOf("\n");
    if (separator <= 0) fail("repository Git configuration has an invalid entry");
    const name = record.slice(0, separator);
    const value = record.slice(separator + 1);
    const allowedBranch = /^branch\..+\.(?:merge|remote)$/u.test(name);
    const allowedRemote =
      name === "remote.origin.fetch" || name === "remote.origin.url";
    const allowedFixtureIdentity = name === "user.name" || name === "user.email";
    const allowedDisabledAutomaticGc = name === "gc.auto" && value === "0";
    if (
      !allowedCore.has(name) &&
      !allowedBranch &&
      !allowedRemote &&
      !allowedFixtureIdentity &&
      !allowedDisabledAutomaticGc
    ) {
      if (name === "gc.auto") {
        fail("repository Git configuration gc.auto must be exactly 0");
      }
      fail(`repository Git configuration can influence evidence: ${name}`);
    }
    if (configuration.has(name)) {
      fail(`repository Git configuration is duplicated: ${name}`);
    }
    configuration.set(name, value);
  }
  if (
    configuration.get("core.repositoryformatversion") !== "0" ||
    configuration.get("core.bare") !== "false" ||
    !new Set(["true", "false"]).has(configuration.get("core.filemode"))
  ) {
    fail("repository Git configuration is not the canonical evidence shape");
  }
  if (
    configuration.has("remote.origin.url") &&
    configuration.get("remote.origin.fetch") !==
      "+refs/heads/*:refs/remotes/origin/*"
  ) {
    fail("repository Git configuration is not the canonical evidence shape");
  }
  return configuration;
}

function assertSelfContainedGitObjectStore(repositoryRoot) {
  if (
    String(
      git(repositoryRoot, ["rev-parse", "--is-shallow-repository"]),
    ).trim() !== "false"
  ) {
    fail("repository must be complete and non-shallow");
  }
  const commonDirectory = String(
    git(repositoryRoot, [
      "rev-parse",
      "--path-format=absolute",
      "--git-common-dir",
    ]),
  ).trim();
  for (const relativePath of ["objects/info/alternates", "info/grafts"]) {
    if (existsSync(path.join(commonDirectory, relativePath))) {
      fail(`repository Git object authority uses forbidden ${relativePath}`);
    }
  }
}

function matchingRefs(repositoryRoot, pattern) {
  const prefix = pattern.endsWith("*") ? pattern.slice(0, -1) : pattern;
  const refs = String(
    git(repositoryRoot, ["for-each-ref", "--format=%(refname)", prefix]),
  )
    .split("\n")
    .filter(Boolean);
  return pattern.endsWith("*")
    ? refs.filter((ref) => ref.startsWith(prefix))
    : refs.filter((ref) => ref === pattern);
}

function validateRepositoryAuthority(document, repositoryRoot) {
  const policy = document.repositoryAuthority;
  exactKeys(
    policy,
    ["format", "repository", "canonicalOrigin", "allowedReachableRefs", "refVerification"],
    "repositoryAuthority",
  );
  if (
    policy.format !== REPOSITORY_POLICY_FORMAT ||
    policy.repository !== "takoform" ||
    policy.canonicalOrigin !== CANONICAL_ORIGIN ||
    policy.refVerification !== "trusted-fetched-checkout-precondition"
  ) {
    fail("repositoryAuthority must pin the canonical Takoform repository");
  }
  exactArray(policy.allowedReachableRefs, ALLOWED_REACHABLE_REFS, "allowedReachableRefs");
  const configuration = localGitConfiguration(repositoryRoot);
  assertSelfContainedGitObjectStore(repositoryRoot);
  const configuredOrigin = configuration.get("remote.origin.url");
  if (typeof configuredOrigin !== "string") {
    fail("the Takoform checkout has no origin remote");
  }
  const origin = configuredOrigin;
  if (normalizeOriginUrl(origin) !== CANONICAL_ORIGIN) {
    fail(`origin ${JSON.stringify(origin)} is not the canonical Takoform repository`);
  }
}

function assertReachableCommit(repositoryRoot, commit, context) {
  assertSelfContainedGitObjectStore(repositoryRoot);
  if (typeof commit !== "string" || !COMMIT.test(commit)) {
    fail(`${context} must be a full lowercase Git commit SHA`);
  }
  if (!gitSucceeds(repositoryRoot, ["cat-file", "-e", `${commit}^{commit}`])) {
    fail(`${context} ${commit} is not present`);
  }
  const refs = ALLOWED_REACHABLE_REFS.flatMap((pattern) =>
    matchingRefs(repositoryRoot, pattern),
  );
  if (!refs.some((ref) => gitSucceeds(repositoryRoot, ["merge-base", "--is-ancestor", commit, ref]))) {
    fail(`${context} ${commit} is not reachable from an allowed canonical ref`);
  }
  const head = git(repositoryRoot, ["rev-parse", "HEAD"]).trim();
  if (!gitSucceeds(repositoryRoot, ["merge-base", "--is-ancestor", commit, head])) {
    fail(`${context} ${commit} is not an ancestor of the checkout`);
  }
}

function listCommittedInventory(repositoryRoot, commit, roots, excludedPaths = []) {
  const raw = git(repositoryRoot, [
    "ls-tree",
    "-r",
    "-z",
    "--full-tree",
    commit,
    "--",
    ...roots,
  ]);
  const excluded = new Set(excludedPaths);
  const entries = [];
  for (const record of raw.split("\0")) {
    if (!record) continue;
    const match = /^(\d+) (\S+) ([0-9a-f]+)\t(.+)$/u.exec(record);
    if (!match) fail("git ls-tree returned an unreadable source record");
    const [, mode, type, , relativePath] = match;
    if (excluded.has(relativePath)) continue;
    if (type !== "blob" || mode === "120000") {
      fail(`source snapshot path ${relativePath} must be a regular committed file`);
    }
    entries.push({
      path: relativePath,
      sha256: sha256Bytes(gitShowBytes(repositoryRoot, commit, relativePath)),
    });
  }
  entries.sort((a, b) => a.path.localeCompare(b.path));
  if (entries.length === 0) fail("source snapshot contains no files");
  return entries;
}

function deriveSpecificationSourceSnapshotAtCommit(repositoryRoot, sourceCommit) {
  const inventory = listCommittedInventory(
    repositoryRoot,
    sourceCommit,
    SPECIFICATION_ROOTS,
    SPECIFICATION_EXCLUDED_PATHS,
  );
  return {
    format: SPECIFICATION_SOURCE_FORMAT,
    releaseVersion: SPECIFICATION_VERSION,
    repository: "takoform",
    sourceCommit,
    roots: [...SPECIFICATION_ROOTS],
    excludedPaths: [...SPECIFICATION_EXCLUDED_PATHS],
    fileCount: inventory.length,
    pathSetSha256: canonicalDigest(inventory.map((entry) => entry.path)),
    documentSetSha256: canonicalDigest(inventory),
  };
}

export function deriveSpecificationSourceSnapshot(repositoryRoot, sourceCommit) {
  assertReachableCommit(repositoryRoot, sourceCommit, "Specification source commit");
  return deriveSpecificationSourceSnapshotAtCommit(repositoryRoot, sourceCommit);
}

function validateFormRef(ref, context, expectedGroup = null) {
  exactKeys(ref, ["apiVersion", "kind", "definitionVersion", "schemaDigest"], context);
  if (typeof ref.apiVersion !== "string" || !GROUP.test(ref.apiVersion)) {
    fail(`${context}.apiVersion must be a versionless Takoform family group`);
  }
  if (expectedGroup !== null && ref.apiVersion !== expectedGroup) {
    fail(`${context}.apiVersion must be ${expectedGroup}`);
  }
  if (typeof ref.kind !== "string" || !KIND.test(ref.kind)) fail(`${context}.kind is invalid`);
  if (typeof ref.definitionVersion !== "string" || !SEMVER.test(ref.definitionVersion)) {
    fail(`${context}.definitionVersion must be an independent SemVer`);
  }
  validateSha256Id(ref.schemaDigest, `${context}.schemaDigest`);
  return ref;
}

function formRefKey(ref) {
  return `${ref.apiVersion}/${ref.kind}@${ref.definitionVersion}#${ref.schemaDigest}`;
}

function validateDigestRecord(value, expectedPath, actualDigest, context) {
  exactKeys(value, ["path", "sha256"], context);
  if (value.path !== expectedPath) fail(`${context}.path must be ${expectedPath}`);
  validateSha256(value.sha256, `${context}.sha256`);
  if (value.sha256 !== actualDigest) fail(`${context}.sha256 does not match committed bytes`);
}

function containsForbiddenCurrentIdentity(value) {
  if (typeof value === "string") return FORBIDDEN_CURRENT_IDENTITIES.has(value);
  if (Array.isArray(value)) return value.some(containsForbiddenCurrentIdentity);
  if (!isRecord(value)) return false;
  return Object.entries(value).some(
    ([key, entry]) =>
      key === "resourceType" ||
      FORBIDDEN_CURRENT_IDENTITIES.has(key) ||
      containsForbiddenCurrentIdentity(entry),
  );
}

function assertProviderNeutralCurrentArtifact(value, context) {
  if (containsForbiddenCurrentIdentity(value)) {
    fail(`${context} contains Provider-owned resourceType or removed ObjectBucket/edge.objects identity`);
  }
}

function validateManifestFiles(repositoryRoot, commit, packagePath, manifest, context, inventory) {
  if (!Array.isArray(manifest.files) || manifest.files.length === 0) {
    fail(`${context}.files must be a non-empty committed inventory`);
  }
  const seen = new Set();
  for (const [index, file] of manifest.files.entries()) {
    exactKeys(file, ["path", "mediaType", "size", "digest"], `${context}.files[${index}]`);
    assertRelativePath(file.path, `${context}.files[${index}].path`);
    if (seen.has(file.path)) fail(`${context}.files repeats ${file.path}`);
    seen.add(file.path);
    nonEmptyString(file.mediaType, `${context}.files[${index}].mediaType`);
    if (!Number.isSafeInteger(file.size) || file.size < 0) {
      fail(`${context}.files[${index}].size must be a non-negative safe integer`);
    }
    validateSha256Id(file.digest, `${context}.files[${index}].digest`);
    const relativePath = path.posix.join(packagePath, file.path);
    const bytes = gitShowBytes(repositoryRoot, commit, relativePath);
    if (bytes.length !== file.size || `sha256:${sha256Bytes(bytes)}` !== file.digest) {
      fail(`${context}.files[${index}] does not match committed bytes`);
    }
    inventory.set(relativePath, sha256Bytes(bytes));
  }
  if (!seen.has(manifest.definitionPath)) {
    fail(`${context}.definitionPath is absent from the package file inventory`);
  }
}

function validateFamilyCandidateSet(repositoryRoot, commit, indexEntry, inventory) {
  exactKeys(indexEntry, ["group", "candidateSet", "sha256", "formCount"], `family index ${indexEntry?.group ?? "entry"}`);
  if (typeof indexEntry.group !== "string" || !GROUP.test(indexEntry.group)) {
    fail("family index group must be a versionless Takoform family group");
  }
  assertRelativePath(indexEntry.candidateSet, `family ${indexEntry.group}.candidateSet`);
  const expectedPath = `forms/candidates/${indexEntry.group}/candidate-set.json`;
  if (indexEntry.candidateSet !== expectedPath) {
    fail(`family ${indexEntry.group}.candidateSet must be ${expectedPath}`);
  }
  validateSha256(indexEntry.sha256, `family ${indexEntry.group}.sha256`);
  if (!Number.isSafeInteger(indexEntry.formCount) || indexEntry.formCount <= 0) {
    fail(`family ${indexEntry.group}.formCount must be a positive derived count`);
  }
  const bytes = gitShowBytes(repositoryRoot, commit, indexEntry.candidateSet);
  if (sha256Bytes(bytes) !== indexEntry.sha256) {
    fail(`family ${indexEntry.group} candidate-set digest does not match committed bytes`);
  }
  inventory.set(indexEntry.candidateSet, indexEntry.sha256);
  const candidate = parseJson(bytes, indexEntry.candidateSet);
  exactKeys(
    candidate,
    [
      "format",
      "family",
      "formMaturity",
      "packageApiVersion",
      "publicationStatus",
      "authoringSource",
      "authoringPolicy",
      "forms",
    ],
    `candidate set ${indexEntry.group}`,
  );
  if (
    candidate.format !== "takoform.form-family-candidates@v1" ||
    candidate.family !== indexEntry.group ||
    candidate.packageApiVersion !== PACKAGE_ENVELOPE ||
    candidate.publicationStatus !== "unpublished" ||
    !Array.isArray(candidate.forms)
  ) {
    fail(`candidate set ${indexEntry.group} is not the exact unpublished versionless family`);
  }
  if (candidate.forms.length !== indexEntry.formCount) {
    fail(`family ${indexEntry.group}.formCount does not equal reopened candidate Forms`);
  }
  assertProviderNeutralCurrentArtifact(candidate, `candidate set ${indexEntry.group}`);

  const byRef = new Map();
  const byKind = new Set();
  const identities = [];
  const definitions = new Map();
  for (const [index, entry] of candidate.forms.entries()) {
    const context = `${indexEntry.candidateSet}.forms[${index}]`;
    exactKeys(entry, ["kind", "role", "path", "formRef", "packageDigest"], context);
    validateFormRef(entry.formRef, `${context}.formRef`, indexEntry.group);
    if (entry.kind !== entry.formRef.kind) fail(`${context}.kind drifted from its FormRef`);
    nonEmptyString(entry.role, `${context}.role`);
    assertRelativePath(entry.path, `${context}.path`);
    if (!entry.path.startsWith(`forms/candidates/${indexEntry.group}/`)) {
      fail(`${context}.path must stay inside its family candidate tree`);
    }
    validateSha256Id(entry.packageDigest, `${context}.packageDigest`);
    const refKey = formRefKey(entry.formRef);
    if (byRef.has(refKey) || byKind.has(entry.kind)) {
      fail(`candidate family ${indexEntry.group} repeats ${entry.kind} or ${refKey}`);
    }

    const packageIndexPath = `${entry.path}/package-index.json`;
    const manifestBytes = gitShowBytes(repositoryRoot, commit, packageIndexPath);
    inventory.set(packageIndexPath, sha256Bytes(manifestBytes));
    const manifest = parseJson(manifestBytes, packageIndexPath);
    exactKeys(
      manifest,
      ["apiVersion", "kind", "formRef", "definitionPath", "files"],
      packageIndexPath,
    );
    if (manifest.apiVersion !== PACKAGE_ENVELOPE || manifest.kind !== "FormPackage") {
      fail(`${packageIndexPath} is not a ${PACKAGE_ENVELOPE} FormPackage`);
    }
    validateFormRef(manifest.formRef, `${packageIndexPath}.formRef`, indexEntry.group);
    if (canonicalJson(manifest.formRef) !== canonicalJson(entry.formRef)) {
      fail(`${packageIndexPath}.formRef does not match the candidate identity`);
    }
    if (`sha256:${canonicalDigest(manifest)}` !== entry.packageDigest) {
      fail(`${packageIndexPath} canonical digest does not match candidate packageDigest`);
    }
    assertRelativePath(manifest.definitionPath, `${packageIndexPath}.definitionPath`);
    validateManifestFiles(repositoryRoot, commit, entry.path, manifest, packageIndexPath, inventory);

    const definitionPath = path.posix.join(entry.path, manifest.definitionPath);
    const definition = committedJson(repositoryRoot, commit, definitionPath);
    assertProviderNeutralCurrentArtifact(definition, definitionPath);
    if (
      definition.apiVersion !== entry.formRef.apiVersion ||
      definition.kind !== entry.formRef.kind ||
      definition.definitionVersion !== entry.formRef.definitionVersion ||
      `sha256:${canonicalDigest(definition)}` !== entry.formRef.schemaDigest ||
      definition.role !== entry.role ||
      definition.requiresHostApi !== HOST_API_LANE ||
      !Array.isArray(definition.lifecycleCapabilities)
    ) {
      fail(`${definitionPath} does not define the exact current candidate Form identity`);
    }
    const identity = {
      formRef: entry.formRef,
      packageDigest: entry.packageDigest,
      definitionPath,
      packageIndexPath,
      requiresHostApi: definition.requiresHostApi,
      lifecycleCapabilities: definition.lifecycleCapabilities,
    };
    byRef.set(refKey, { entry, manifest, definition, identity });
    byKind.add(entry.kind);
    definitions.set(entry.kind, definition);
    identities.push(identity);
  }
  identities.sort((a, b) => formRefKey(a.formRef).localeCompare(formRefKey(b.formRef)));
  return {
    group: indexEntry.group,
    candidateSet: { path: indexEntry.candidateSet, sha256: indexEntry.sha256 },
    formCount: identities.length,
    candidateIdentities: identities,
    byRef,
    definitions,
  };
}

function validateAggregateCandidateSet(repositoryRoot, commit, record, type, inventory) {
  const plural = type === "interface" ? "interfaces" : "bindings";
  const format = type === "interface"
    ? "takoform.interface-candidates@v1"
    : "takoform.binding-candidates@v1";
  const key = type === "interface" ? "interfaceCandidateSet" : "bindingCandidateSet";
  exactKeys(record, ["path", "sha256"], key);
  assertRelativePath(record.path, `${key}.path`);
  if (!record.path.startsWith(`${plural}/candidates/`) || !record.path.endsWith("/candidate-set.json")) {
    fail(`${key}.path must name the aggregate ${type} candidate set`);
  }
  validateSha256(record.sha256, `${key}.sha256`);
  const bytes = gitShowBytes(repositoryRoot, commit, record.path);
  if (sha256Bytes(bytes) !== record.sha256) fail(`${key}.sha256 does not match committed bytes`);
  inventory.set(record.path, record.sha256);
  const candidate = parseJson(bytes, record.path);
  exactKeys(candidate, ["format", "publicationStatus", "authoringSource", plural], record.path);
  if (
    candidate.format !== format ||
    candidate.publicationStatus !== "unpublished" ||
    !Array.isArray(candidate[plural])
  ) {
    fail(`${record.path} is not the exact unpublished aggregate ${type} set`);
  }
  assertProviderNeutralCurrentArtifact(candidate, record.path);
  const identities = [];
  const seen = new Set();
  for (const [index, entry] of candidate[plural].entries()) {
    const context = `${record.path}.${plural}[${index}]`;
    exactKeys(entry, ["name", "version", "schemaDigest"], context);
    nonEmptyString(entry.name, `${context}.name`);
    if (seen.has(entry.name)) fail(`${record.path} repeats ${entry.name}`);
    seen.add(entry.name);
    if (!SEMVER.test(entry.version)) fail(`${context}.version must be SemVer`);
    validateSha256Id(entry.schemaDigest, `${context}.schemaDigest`);
    const definitionPath = path.posix.join(path.posix.dirname(record.path), entry.name, "definition.json");
    const definitionBytes = gitShowBytes(repositoryRoot, commit, definitionPath);
    inventory.set(definitionPath, sha256Bytes(definitionBytes));
    const definition = parseJson(definitionBytes, definitionPath);
    assertProviderNeutralCurrentArtifact(definition, definitionPath);
    if (
      definition.name !== entry.name ||
      definition.version !== entry.version ||
      `sha256:${canonicalDigest(definition)}` !== entry.schemaDigest ||
      definition.kind !== (type === "interface" ? "InterfaceDefinition" : "BindingDefinition")
    ) {
      fail(`${definitionPath} does not match its aggregate ${type} identity`);
    }
    identities.push({
      name: entry.name,
      version: entry.version,
      schemaDigest: entry.schemaDigest,
      definitionPath,
    });
  }
  identities.sort((a, b) => `${a.name}@${a.version}`.localeCompare(`${b.name}@${b.version}`));
  return { path: record.path, sha256: record.sha256, identities };
}

function validateStandardServiceBoundary(edgeFamily) {
  const definition = edgeFamily.definitions.get("WorkerVersion");
  if (!definition) fail("Edge current family must contain WorkerVersion");
  const external = definition.desiredSchema?.properties?.externalServices;
  if (!isRecord(external)) fail("WorkerVersion must declare externalServices slots");
  if (external["x-takoform-standard-services"] !== STANDARD_SERVICE_API) {
    fail(`WorkerVersion externalServices must bind ${STANDARD_SERVICE_API}`);
  }
  const item = external.items;
  if (!isRecord(item) || !isRecord(item.properties)) {
    fail("WorkerVersion externalServices items must be closed slot objects");
  }
  exactArray(item.required, ["name", "service"], "WorkerVersion externalServices required fields");
  exactArray(Object.keys(item.properties).sort(), ["name", "required", "service"], "WorkerVersion externalServices wire fields");
  if (item.additionalProperties !== false) {
    fail("WorkerVersion externalServices slots must reject extra portable fields");
  }
  if (
    item.properties.required?.type !== "boolean" ||
    item.properties.required?.default !== true
  ) {
    fail("WorkerVersion externalServices.required must be boolean and default to true");
  }
  const service = item.properties.service;
  if (!isRecord(service) || !isRecord(service.properties)) {
    fail("WorkerVersion externalServices.service must be an object schema");
  }
  exactArray(Object.keys(service.properties).sort(), ["apiVersion", "protocol"], "standard service wire fields");
  exactArray(service.required, ["apiVersion", "protocol"], "standard service required fields");
  if (
    service.additionalProperties !== false ||
    service.properties.apiVersion?.const !== STANDARD_SERVICE_API
  ) {
    fail(`standard service slots must be closed and bind ${STANDARD_SERVICE_API}`);
  }
  const protocol = service.properties.protocol;
  if (
    !isRecord(protocol) ||
    protocol.type !== "string" ||
    protocol.maxLength !== 253 ||
    protocol.pattern !== STANDARD_SERVICE_PROTOCOL_PATTERN
  ) {
    fail("standard service protocol must use the exact opaque reverse-DNS v1 grammar");
  }
  for (const closedKey of ["enum", "const", "oneOf", "anyOf"]) {
    if (Object.hasOwn(protocol, closedKey)) {
      fail("standard service protocol must not create a central Takoform protocol enum");
    }
  }
  const nonNarrowingKeys = new Set([
    "type",
    "maxLength",
    "pattern",
    "description",
    "title",
    "$comment",
    "examples",
  ]);
  for (const key of Object.keys(protocol)) {
    if (!nonNarrowingKeys.has(key)) {
      fail(`standard service protocol schema must not add narrowing keyword ${key}`);
    }
  }
}

function resolveReferencedPath(repositoryRoot, commit, parentPath, referencedPath, context) {
  assertRelativePath(referencedPath, context);
  const relative = path.posix.join(path.posix.dirname(parentPath), referencedPath);
  if (gitPathExists(repositoryRoot, commit, relative)) return relative;
  if (gitPathExists(repositoryRoot, commit, referencedPath)) return referencedPath;
  fail(`${context} does not name a committed transitive artifact`);
}

function collectTransitiveArtifacts(repositoryRoot, commit, rootPath, rootBytes, inventory) {
  const visited = new Set();
  const documents = [];
  function visit(relativePath, bytes) {
    if (visited.has(relativePath)) return;
    visited.add(relativePath);
    inventory.set(relativePath, sha256Bytes(bytes));
    if (!relativePath.endsWith(".json")) return;
    const document = parseJson(bytes, relativePath);
    documents.push({ path: relativePath, document, bytes });
    function walk(value, context) {
      if (Array.isArray(value)) {
        value.forEach((entry, index) => walk(entry, `${context}[${index}]`));
        return;
      }
      if (!isRecord(value)) return;
      if (typeof value.path === "string" && typeof value.sha256 === "string") {
        const childPath = resolveReferencedPath(
          repositoryRoot,
          commit,
          relativePath,
          value.path,
          `${context}.path`,
        );
        const childBytes = gitShowBytes(repositoryRoot, commit, childPath);
        if (sha256Bytes(childBytes) !== digestHex(value.sha256, `${context}.sha256`)) {
          fail(`${context} does not match committed transitive bytes`);
        }
        visit(childPath, childBytes);
      }
      for (const [key, entry] of Object.entries(value)) walk(entry, `${context}.${key}`);
    }
    walk(document, relativePath);
  }
  visit(rootPath, rootBytes);
  return documents;
}

function corpusRequiredChecks(corpus, context) {
  const checks = corpus.requiredChecks ?? corpus.requiredRunnerChecks;
  sortedUniqueStrings(checks, `${context}.requiredChecks`);
  return checks;
}

function validateCorpusScenarios(
  corpus,
  requiredChecks,
  context,
  expectedFormat,
  familyGroups = null,
) {
  if (corpus.format !== expectedFormat) {
    fail(`${context}.format must be ${expectedFormat}`);
  }
  if (!Array.isArray(corpus.scenarios) || corpus.scenarios.length === 0) {
    fail(`${context}.scenarios must contain executable class-specific cases`);
  }
  const checks = [];
  for (const [index, scenario] of corpus.scenarios.entries()) {
    const scenarioContext = `${context}.scenarios[${index}]`;
    exactKeys(scenario, ["check", "input", "expected"], scenarioContext);
    nonEmptyString(scenario.check, `${scenarioContext}.check`);
    if (!isRecord(scenario.input) || Object.keys(scenario.input).length === 0) {
      fail(`${scenarioContext}.input must be a concrete non-empty observation input`);
    }
    if (!isRecord(scenario.expected) || Object.keys(scenario.expected).length === 0) {
      fail(`${scenarioContext}.expected must be a concrete non-empty semantic readback`);
    }
    if (familyGroups !== null) {
      exactArray(scenario.input.familyGroups, familyGroups, `${scenarioContext}.input.familyGroups`);
    }
    checks.push(scenario.check);
  }
  exactArray(checks, requiredChecks, `${context} scenario check coverage`);
  if (familyGroups !== null) {
    exactArray(corpus.familyGroups, familyGroups, `${context}.familyGroups`);
  }
}

function runnerProbeEntries(corpus, context) {
  const input = corpus.runnerInput;
  if (!isRecord(input) && !Array.isArray(input)) {
    fail(`${context}.runnerInput must carry the actual family probe inputs`);
  }
  const entries = Array.isArray(input)
    ? input.map((value, index) => [String(index), value])
    : Object.entries(input);
  const out = [];
  for (const [probeName, value] of entries) {
    const probe = isRecord(value?.resourceProbe) ? value.resourceProbe : value;
    if (isRecord(probe?.identity?.formRef)) out.push({ probeName, probe });
  }
  if (out.length === 0) fail(`${context}.runnerInput has no concrete Form lifecycle probes`);
  return out;
}

function validateStandardServiceFixtures(corpus, context, requireEdgeFixtures) {
  if (!Array.isArray(corpus.standardServiceFixtures)) {
    fail(`${context}.standardServiceFixtures must be an array`);
  }
  let supportedS3 = false;
  let refusedUnknown = false;
  const protocolGrammar = new RegExp(STANDARD_SERVICE_PROTOCOL_PATTERN, "u");
  for (const [index, fixture] of corpus.standardServiceFixtures.entries()) {
    const fixtureContext = `${context}.standardServiceFixtures[${index}]`;
    exactKeys(fixture, ["serviceRef", "satisfiable"], fixtureContext);
    exactKeys(fixture.serviceRef, ["apiVersion", "protocol"], `${fixtureContext}.serviceRef`);
    if (fixture.serviceRef.apiVersion !== STANDARD_SERVICE_API) {
      fail(`${fixtureContext}.serviceRef.apiVersion must be ${STANDARD_SERVICE_API}`);
    }
    if (
      typeof fixture.serviceRef.protocol !== "string" ||
      fixture.serviceRef.protocol.length > 253 ||
      !protocolGrammar.test(fixture.serviceRef.protocol)
    ) {
      fail(`${fixtureContext}.serviceRef.protocol must use the opaque reverse-DNS grammar`);
    }
    if (typeof fixture.satisfiable !== "boolean") {
      fail(`${fixtureContext}.satisfiable must be boolean`);
    }
    if (
      fixture.serviceRef.protocol === S3_STANDARD_SERVICE_PROTOCOL &&
      fixture.satisfiable === true
    ) {
      supportedS3 = true;
    }
    if (
      fixture.serviceRef.protocol !== S3_STANDARD_SERVICE_PROTOCOL &&
      fixture.satisfiable === false
    ) {
      refusedUnknown = true;
    }
  }
  if (requireEdgeFixtures && (!supportedS3 || !refusedUnknown)) {
    fail(
      `Edge semantic corpus must contain exact ${STANDARD_SERVICE_API}/${S3_STANDARD_SERVICE_PROTOCOL} satisfiable readback and an unknown-valid refusal`,
    );
  }
  return !requireEdgeFixtures || (supportedS3 && refusedUnknown);
}

function validateFamilyCorpus(repositoryRoot, commit, suiteEntry, family, inventory) {
  const context = `conformance suite family ${suiteEntry.group}`;
  exactKeys(suiteEntry, ["group", "path", "sha256", "requiredChecks", "dependencyGroups"], context);
  if (suiteEntry.group !== family.group) fail(`${context}.group drifted from family index`);
  assertRelativePath(suiteEntry.path, `${context}.path`);
  validateSha256(suiteEntry.sha256, `${context}.sha256`);
  sortedUniqueStrings(suiteEntry.requiredChecks, `${context}.requiredChecks`);
  sortedUniqueStrings(suiteEntry.dependencyGroups, `${context}.dependencyGroups`, { allowEmpty: true });
  if (suiteEntry.dependencyGroups.includes(suiteEntry.group)) {
    fail(`${context}.dependencyGroups must not include itself`);
  }
  const bytes = gitShowBytes(repositoryRoot, commit, suiteEntry.path);
  if (sha256Bytes(bytes) !== suiteEntry.sha256) fail(`${context}.sha256 does not match committed bytes`);
  const documents = collectTransitiveArtifacts(
    repositoryRoot,
    commit,
    suiteEntry.path,
    bytes,
    inventory,
  );
  const corpus = documents[0].document;
  if (corpus.hostApiLane !== HOST_API_LANE && corpus.apiVersion !== HOST_API_LANE) {
    fail(`${context} does not bind ${HOST_API_LANE}`);
  }
  if (corpus.group !== undefined && corpus.group !== suiteEntry.group) {
    fail(`${context} corpus group drifted`);
  }
  exactArray(corpusRequiredChecks(corpus, context), suiteEntry.requiredChecks, `${context} required checks`);
  assertProviderNeutralCurrentArtifact(corpus, context);
  validateCorpusScenarios(
    corpus,
    suiteEntry.requiredChecks,
    context,
    "takoform.family-semantic-corpus@v1",
  );
  const standardServiceSupportExact = validateStandardServiceFixtures(
    corpus,
    context,
    family.group === EDGE_FAMILY_GROUP,
  );

  const runnerIdentities = [];
  const seen = new Set();
  for (const { probeName, probe } of runnerProbeEntries(corpus, context)) {
    requiredKeys(
      probe,
      ["name", "identity", "lifecycleCapabilities", "desired", "desiredSchema"],
      `${context}.runnerInput.${probeName}`,
    );
    exactKeys(probe.identity, ["formRef", "packageDigest"], `${context}.runnerInput.${probeName}.identity`);
    validateFormRef(probe.identity.formRef, `${context}.runnerInput.${probeName}.identity.formRef`, family.group);
    validateSha256Id(probe.identity.packageDigest, `${context}.runnerInput.${probeName}.identity.packageDigest`);
    const key = formRefKey(probe.identity.formRef);
    if (seen.has(key)) fail(`${context} repeats lifecycle probe ${key}`);
    seen.add(key);
    const candidate = family.byRef.get(key);
    if (!candidate) fail(`${context} carries non-candidate FormRef ${key}`);
    if (probe.identity.packageDigest !== candidate.entry.packageDigest) {
      fail(`${context}.runnerInput.${probeName} packageDigest drifted from the candidate`);
    }
    if (canonicalJson(probe.lifecycleCapabilities) !== canonicalJson(candidate.definition.lifecycleCapabilities)) {
      fail(`${context}.runnerInput.${probeName} lifecycleCapabilities drifted from its Definition`);
    }
    exactKeys(probe.desiredSchema, ["path", "sha256"], `${context}.runnerInput.${probeName}.desiredSchema`);
    const desiredPath = resolveReferencedPath(
      repositoryRoot,
      commit,
      suiteEntry.path,
      probe.desiredSchema.path,
      `${context}.runnerInput.${probeName}.desiredSchema.path`,
    );
    const desiredBytes = gitShowBytes(repositoryRoot, commit, desiredPath);
    if (sha256Bytes(desiredBytes) !== digestHex(probe.desiredSchema.sha256, `${context}.runnerInput.${probeName}.desiredSchema.sha256`)) {
      fail(`${context}.runnerInput.${probeName}.desiredSchema does not match committed bytes`);
    }
    const desiredSchema = parseJson(desiredBytes, desiredPath);
    if (canonicalJson(desiredSchema) !== canonicalJson(candidate.definition.desiredSchema)) {
      fail(`${context}.runnerInput.${probeName}.desiredSchema does not pin its Definition schema`);
    }
    runnerIdentities.push({
      probeName,
      probeInstanceName: nonEmptyString(probe.name, `${context}.runnerInput.${probeName}.name`),
      formRef: probe.identity.formRef,
      packageDigest: probe.identity.packageDigest,
      desiredSchema: { path: desiredPath, sha256: `sha256:${sha256Bytes(desiredBytes)}` },
      lifecycleCapabilities: probe.lifecycleCapabilities,
    });
  }
  runnerIdentities.sort((a, b) => formRefKey(a.formRef).localeCompare(formRefKey(b.formRef)));
  const missingFormRefs = family.candidateIdentities
    .filter((identity) => !seen.has(formRefKey(identity.formRef)))
    .map((identity) => identity.formRef);
  if (
    runnerIdentities.length !== family.candidateIdentities.length ||
    missingFormRefs.length !== 0
  ) {
    fail(`${context} must cover every family candidate exactly once`);
  }
  return {
    group: family.group,
    corpus: { path: suiteEntry.path, sha256: suiteEntry.sha256 },
    requiredChecks: suiteEntry.requiredChecks,
    dependencyGroups: suiteEntry.dependencyGroups,
    runnerIdentities,
    missingFormRefs,
    standardServiceSupportExact,
  };
}

function validateNonFamilyCorpus(repositoryRoot, commit, record, kind, inventory, familyGroups = null) {
  const context = `conformance suite ${kind}`;
  exactKeys(record, ["path", "sha256", "requiredChecks"], context);
  assertRelativePath(record.path, `${context}.path`);
  validateSha256(record.sha256, `${context}.sha256`);
  sortedUniqueStrings(record.requiredChecks, `${context}.requiredChecks`);
  const bytes = gitShowBytes(repositoryRoot, commit, record.path);
  if (sha256Bytes(bytes) !== record.sha256) fail(`${context}.sha256 does not match committed bytes`);
  const documents = collectTransitiveArtifacts(repositoryRoot, commit, record.path, bytes, inventory);
  const corpus = documents[0].document;
  if (corpus.hostApiLane !== HOST_API_LANE && corpus.apiVersion !== HOST_API_LANE) {
    fail(`${context} does not bind ${HOST_API_LANE}`);
  }
  exactArray(corpusRequiredChecks(corpus, context), record.requiredChecks, `${context} required checks`);
  assertProviderNeutralCurrentArtifact(corpus, context);
  validateCorpusScenarios(
    corpus,
    record.requiredChecks,
    context,
    kind === "generic"
      ? "takoform.generic-host-corpus@v1"
      : "takoform.all-family-composition-corpus@v1",
    kind === "composition" ? familyGroups : null,
  );
  return { path: record.path, sha256: record.sha256, requiredChecks: record.requiredChecks };
}

function validateRunnerCommand(repositoryRoot, commit, runner) {
  exactKeys(runner, ["command", "reportFormat"], "conformance suite runner");
  nonEmptyString(runner.reportFormat, "conformance suite runner.reportFormat");
  if (
    !Array.isArray(runner.command) ||
    runner.command.length < 2 ||
    runner.command.some(
      (entry) =>
        typeof entry !== "string" ||
        entry === "" ||
        /[\0\r\n;&|><`$()]/u.test(entry),
    )
  ) {
    fail("conformance suite runner.command must be shell-free argv");
  }
  if (!runner.command.includes(CONFORMANCE_SUITE_PATH)) {
    fail(`conformance suite runner.command must consume ${CONFORMANCE_SUITE_PATH}`);
  }
  let sourcePath;
  if (runner.command[0] === "go") {
    if (runner.command[1] !== "run" || !runner.command[2]?.startsWith("./cmd/")) {
      fail("Go suite runner must use go run with a repository-relative ./cmd source");
    }
    sourcePath = runner.command[2].slice(2);
  } else if (runner.command[0] === "bun") {
    const candidate = runner.command[1]?.replace(/^\.\//u, "");
    if (!candidate?.startsWith("scripts/")) {
      fail("Bun suite runner must name a repository-relative scripts source");
    }
    sourcePath = candidate;
  } else {
    fail("conformance suite runner executable must be go or bun");
  }
  assertRelativePath(sourcePath, "conformance suite runner source");
  if (!gitPathExists(repositoryRoot, commit, sourcePath)) {
    fail("conformance suite runner source is not committed at the Specification source commit");
  }
  return { command: [...runner.command], reportFormat: runner.reportFormat, sourcePath };
}

function validateCandidateBaselineShape(repositoryRoot, baseline) {
  exactKeys(
    baseline,
    ["repository", "commit", "familyIndex", "conformanceSuite"],
    "candidateBaseline",
  );
  if (baseline.repository !== "takoform") fail("candidateBaseline.repository must be takoform");
  assertReachableCommit(repositoryRoot, baseline.commit, "candidateBaseline.commit");
  const bothNull = baseline.familyIndex === null && baseline.conformanceSuite === null;
  const bothPresent = baseline.familyIndex !== null && baseline.conformanceSuite !== null;
  if (!bothNull && !bothPresent) {
    fail("candidateBaseline familyIndex and conformanceSuite must be null together or present together");
  }
  return bothPresent;
}

export function deriveCandidateCorpusEvidence(repositoryRoot, baseline) {
  if (!validateCandidateBaselineShape(repositoryRoot, baseline)) return null;
  const familyIndexBytes = gitShowBytes(repositoryRoot, baseline.commit, FAMILY_INDEX_PATH);
  const suiteBytes = gitShowBytes(repositoryRoot, baseline.commit, CONFORMANCE_SUITE_PATH);
  validateDigestRecord(
    baseline.familyIndex,
    FAMILY_INDEX_PATH,
    sha256Bytes(familyIndexBytes),
    "candidateBaseline.familyIndex",
  );
  validateDigestRecord(
    baseline.conformanceSuite,
    CONFORMANCE_SUITE_PATH,
    sha256Bytes(suiteBytes),
    "candidateBaseline.conformanceSuite",
  );
  const familyIndex = parseJson(familyIndexBytes, FAMILY_INDEX_PATH);
  exactKeys(
    familyIndex,
    ["format", "families", "interfaceCandidateSet", "bindingCandidateSet"],
    FAMILY_INDEX_PATH,
  );
  if (familyIndex.format !== FAMILY_INDEX_FORMAT || !Array.isArray(familyIndex.families)) {
    fail(`${FAMILY_INDEX_PATH} must use ${FAMILY_INDEX_FORMAT}`);
  }
  if (familyIndex.families.length === 0) fail(`${FAMILY_INDEX_PATH} must name at least one family`);
  const groups = familyIndex.families.map((entry) => entry?.group);
  if (
    groups.some((group) => typeof group !== "string") ||
    new Set(groups).size !== groups.length ||
    JSON.stringify(groups) !== JSON.stringify([...groups].sort())
  ) {
    fail(`${FAMILY_INDEX_PATH}.families must be closed, unique, and ordered by group`);
  }
  const candidatePaths = familyIndex.families.map((entry) => entry?.candidateSet);
  if (JSON.stringify(candidatePaths) !== JSON.stringify([...candidatePaths].sort())) {
    fail(`${FAMILY_INDEX_PATH}.families must also be ordered by candidate-set path`);
  }

  const inventory = new Map([
    [FAMILY_INDEX_PATH, sha256Bytes(familyIndexBytes)],
    [CONFORMANCE_SUITE_PATH, sha256Bytes(suiteBytes)],
  ]);
  const families = familyIndex.families.map((entry) =>
    validateFamilyCandidateSet(repositoryRoot, baseline.commit, entry, inventory),
  );
  const edgeFamily = families.find((entry) => entry.group === EDGE_FAMILY_GROUP);
  if (!edgeFamily) fail(`current family index must contain ${EDGE_FAMILY_GROUP}`);
  if (edgeFamily.formCount !== EDGE_CANDIDATE_SIZE) {
    fail(`Edge current family must contain exactly ${EDGE_CANDIDATE_SIZE} Forms`);
  }
  if (edgeFamily.candidateIdentities.some((entry) => entry.formRef.kind === "ObjectBucket")) {
    fail("Edge current family must not contain an ObjectBucket Form");
  }
  if (
    gitPathExists(
      repositoryRoot,
      baseline.commit,
      "forms/candidates/edge.forms.takoform.com/object-bucket",
    )
  ) {
    fail("the versionless Edge current tree must not retain an unindexed ObjectBucket package");
  }
  validateStandardServiceBoundary(edgeFamily);
  const interfaceCandidateSet = validateAggregateCandidateSet(
    repositoryRoot,
    baseline.commit,
    familyIndex.interfaceCandidateSet,
    "interface",
    inventory,
  );
  const bindingCandidateSet = validateAggregateCandidateSet(
    repositoryRoot,
    baseline.commit,
    familyIndex.bindingCandidateSet,
    "binding",
    inventory,
  );

  const suite = parseJson(suiteBytes, CONFORMANCE_SUITE_PATH);
  exactKeys(
    suite,
    ["format", "hostApiLane", "generic", "families", "composition", "runner"],
    CONFORMANCE_SUITE_PATH,
  );
  if (
    suite.format !== CONFORMANCE_SUITE_FORMAT ||
    suite.hostApiLane !== HOST_API_LANE ||
    !Array.isArray(suite.families)
  ) {
    fail(`${CONFORMANCE_SUITE_PATH} must be the exact ${HOST_API_LANE} suite`);
  }
  const suiteGroups = suite.families.map((entry) => entry?.group);
  exactArray(suiteGroups, groups, "conformance suite family groups");
  const suitePaths = suite.families.map((entry) => entry?.path);
  if (JSON.stringify(suitePaths) !== JSON.stringify([...suitePaths].sort())) {
    fail(`${CONFORMANCE_SUITE_PATH}.families must be ordered by group and path`);
  }
  for (const entry of suite.families) {
    const allowed = new Set(groups);
    for (const dependency of entry?.dependencyGroups ?? []) {
      if (!allowed.has(dependency) || dependency === entry?.group) {
        fail(`conformance suite family ${entry?.group} dependency ${dependency} must be another indexed family`);
      }
    }
  }
  const generic = validateNonFamilyCorpus(
    repositoryRoot,
    baseline.commit,
    suite.generic,
    "generic",
    inventory,
  );
  const familyCorpora = suite.families.map((entry, index) =>
    validateFamilyCorpus(repositoryRoot, baseline.commit, entry, families[index], inventory),
  );
  const composition = validateNonFamilyCorpus(
    repositoryRoot,
    baseline.commit,
    suite.composition,
    "composition",
    inventory,
    groups,
  );
  const edgeCorpus = familyCorpora.find((entry) => entry.group === EDGE_FAMILY_GROUP);
  if (
    edgeCorpus.runnerIdentities.length !== EDGE_CANDIDATE_SIZE ||
    edgeCorpus.missingFormRefs.length !== 0
  ) {
    fail(`Edge semantic corpus must cover the exact ${EDGE_CANDIDATE_SIZE}/${EDGE_CANDIDATE_SIZE} current roster`);
  }
  if (!edgeCorpus.standardServiceSupportExact) {
    fail("Edge semantic corpus standard-service support/refusal observations are incomplete");
  }
  const runner = validateRunnerCommand(repositoryRoot, baseline.commit, suite.runner);
  const artifactInventory = [...inventory.entries()]
    .map(([relativePath, sha256]) => ({ path: relativePath, sha256 }))
    .sort((a, b) => a.path.localeCompare(b.path));
  return {
    format: CANDIDATE_CORPUS_FORMAT,
    sourceCommit: baseline.commit,
    familyIndex: baseline.familyIndex,
    conformanceSuite: baseline.conformanceSuite,
    interfaceCandidateSet,
    bindingCandidateSet,
    families: families.map((family, index) => ({
      group: family.group,
      candidateSet: family.candidateSet,
      formCount: family.formCount,
      candidateIdentities: family.candidateIdentities,
      corpus: familyCorpora[index].corpus,
      requiredChecks: familyCorpora[index].requiredChecks,
      dependencyGroups: familyCorpora[index].dependencyGroups,
      runnerIdentities: familyCorpora[index].runnerIdentities,
      missingFormRefs: familyCorpora[index].missingFormRefs,
    })),
    generic,
    composition,
    runner: {
      command: runner.command,
      reportFormat: runner.reportFormat,
    },
    artifactInventory,
    artifactSetSha256: canonicalDigest(artifactInventory),
  };
}

function assertEvidenceRecordCommitted(document, repositoryRoot) {
  const head = git(repositoryRoot, ["rev-parse", "HEAD"]).trim();
  if (!gitPathExists(repositoryRoot, head, PUBLICATION_EVIDENCE)) {
    fail(`${PUBLICATION_EVIDENCE} must be committed before evidence can close`);
  }
  assertWorktreePathsClean(repositoryRoot, [PUBLICATION_EVIDENCE], PUBLICATION_EVIDENCE);
  const committed = committedJson(repositoryRoot, head, PUBLICATION_EVIDENCE);
  if (canonicalJson(committed) !== canonicalJson(document)) {
    fail(`${PUBLICATION_EVIDENCE} does not equal the evidence record committed at HEAD`);
  }
}

function validateSourceEvidence(document, repositoryRoot) {
  const evidence = document.evidence.specification.sourceSnapshot;
  if (evidence === null) return false;
  if (!isRecord(evidence) || typeof evidence.sourceCommit !== "string") {
    fail("sourceSnapshot must identify one exact Specification source commit");
  }
  const expected = deriveSpecificationSourceSnapshot(
    repositoryRoot,
    evidence.sourceCommit,
  );
  exactKeys(evidence, Object.keys(expected), "evidence.specification.sourceSnapshot");
  if (canonicalJson(evidence) !== canonicalJson(expected)) {
    fail("source snapshot evidence does not equal the exact committed normative document set");
  }
  assertWorktreePathsClean(
    repositoryRoot,
    SPECIFICATION_ROOTS,
    "normative Specification source",
  );
  const head = git(repositoryRoot, ["rev-parse", "HEAD"]).trim();
  const current = deriveSpecificationSourceSnapshotAtCommit(repositoryRoot, head);
  if (
    current.fileCount !== evidence.fileCount ||
    current.pathSetSha256 !== evidence.pathSetSha256 ||
    current.documentSetSha256 !== evidence.documentSetSha256
  ) {
    fail("normative Specification source at HEAD does not equal the recorded source snapshot");
  }
  return true;
}

function validateCandidateEvidence(document, candidate) {
  const evidence = document.evidence.specification.candidateCorpus;
  if (evidence === null) return false;
  if (candidate === null) fail("candidateCorpus cannot close without the canonical index/suite tuple");
  exactKeys(evidence, Object.keys(candidate), "evidence.specification.candidateCorpus");
  if (canonicalJson(evidence) !== canonicalJson(candidate)) {
    fail("candidate corpus evidence does not equal the reopened multi-family artifact closure");
  }
  return true;
}

function runTextCommand(repositoryRoot, command, context) {
  try {
    return execFileSync(command[0], command.slice(1), {
      cwd: repositoryRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch (error) {
    const detail = String(error.stderr ?? "").trim();
    fail(`${context} command failed${detail ? `: ${detail}` : ""}`);
  }
}

function runCommand(repositoryRoot, command, context) {
  runTextCommand(repositoryRoot, command, context);
}

export function parseProviderMatrixObservations(output) {
  const match = /^verified non-publishable worker authoring evidence: (\d+) CLIs, (\d+) validated configurations, same-name replacement refused at plan, roll-forward serves throughout \((\d+) Ready samples, (\d+) not ready\), two owners of identical output hold (\d+) distinct revisions, heterogeneous vars keep their JSON types, destroy removes the (\d+)-resource aggregate in dependency order and leaves (\d+) behind$/u.exec(
    output.trim(),
  );
  if (!match) fail("Provider lifecycle matrix did not return its exact observation report");
  const observations = {
    cliCount: Number(match[1]),
    validatedConfigurations: Number(match[2]),
    sameNameReplacementRefusedAtPlan: true,
    readySamples: Number(match[3]),
    notReadySamples: Number(match[4]),
    distinctOwnedRevisions: Number(match[5]),
    heterogeneousVariableTypesPreserved: true,
    destroyedResources: Number(match[6]),
    resourcesRemainingAfterDestroy: Number(match[7]),
  };
  const required = {
    cliCount: 2,
    validatedConfigurations: 19,
    sameNameReplacementRefusedAtPlan: true,
    readySamples: 124,
    notReadySamples: 0,
    distinctOwnedRevisions: 4,
    heterogeneousVariableTypesPreserved: true,
    destroyedResources: 5,
    resourcesRemainingAfterDestroy: 0,
  };
  if (canonicalJson(observations) !== canonicalJson(required)) {
    fail("Provider lifecycle matrix observations do not satisfy its exact Provider contract");
  }
  return observations;
}

export function parseProviderCompatibilityOutput(output) {
  const passed = [];
  for (const [index, line] of output.trim().split("\n").entries()) {
    if (line === "") continue;
    const event = parseJson(line, `Provider compatibility event ${index}`);
    if (event.Action === "pass" && typeof event.Test === "string") passed.push(event.Test);
  }
  passed.sort();
  exactArray(passed, [...PROVIDER_COMPATIBILITY_TESTS].sort(), "Provider compatibility passed tests");
  return passed;
}

function runProviderCompatibility(repositoryRoot) {
  return parseProviderCompatibilityOutput(
    runTextCommand(repositoryRoot, PROVIDER_COMPATIBILITY_COMMAND, "Provider compatibility"),
  );
}

function implementationDigest(repositoryRoot, commit, roots, excludedPaths = []) {
  return canonicalDigest(listCommittedInventory(repositoryRoot, commit, roots, excludedPaths));
}

function assertCheckoutMatchesCommit(repositoryRoot, commit, roots, context) {
  if (!gitSucceeds(repositoryRoot, [
    "-c",
    "diff.renames=false",
    "diff",
    "--quiet",
    "--no-renames",
    "--no-ext-diff",
    "--no-textconv",
    commit,
    "HEAD",
    "--",
    ...roots,
  ])) {
    fail(`${context} implementation at HEAD differs from source commit ${commit}`);
  }
  assertWorktreePathsClean(repositoryRoot, roots, `${context} implementation`);
}

function expectedReferenceReport(candidate) {
  const passedChecks = (checks) => checks.map((name) => ({ name, status: "passed" }));
  return {
    format: candidate.runner.reportFormat,
    status: "passed",
    hostApiLane: HOST_API_LANE,
    suite: candidate.conformanceSuite,
    generic: {
      path: candidate.generic.path,
      sha256: candidate.generic.sha256,
      checks: passedChecks(candidate.generic.requiredChecks),
    },
    families: candidate.families.map((family) => ({
      group: family.group,
      path: family.corpus.path,
      sha256: family.corpus.sha256,
      checks: passedChecks(family.requiredChecks),
      runnerFormRefs: family.runnerIdentities.map((entry) => entry.formRef),
    })),
    composition: {
      path: candidate.composition.path,
      sha256: candidate.composition.sha256,
      checks: passedChecks(candidate.composition.requiredChecks),
    },
  };
}

function validateReferenceReport(report, candidate) {
  const expected = expectedReferenceReport(candidate);
  exactKeys(report, Object.keys(expected), "reference conformance suite report");
  if (canonicalJson(report) !== canonicalJson(expected)) {
    fail("reference conformance suite report does not contain the exact generic, family, composition, and FormRef observations");
  }
}

export function deriveReferenceConformanceEvidence(repositoryRoot, candidate) {
  if (candidate === null || candidate.families.length === 0) {
    fail("reference conformance cannot run without the complete multi-family tuple");
  }
  assertCheckoutMatchesCommit(
    repositoryRoot,
    candidate.sourceCommit,
    REFERENCE_EXECUTION_PATHS,
    "reference conformance suite",
  );
  const output = runTextCommand(repositoryRoot, candidate.runner.command, "reference conformance suite");
  const report = parseJson(output, "reference conformance suite report");
  validateReferenceReport(report, candidate);
  return {
    format: REFERENCE_CONFORMANCE_FORMAT,
    sourceCommit: candidate.sourceCommit,
    familyIndex: candidate.familyIndex,
    conformanceSuite: candidate.conformanceSuite,
    implementationSetSha256: implementationDigest(
      repositoryRoot,
      candidate.sourceCommit,
      REFERENCE_IMPLEMENTATION_ROOTS,
      REFERENCE_IMPLEMENTATION_EXCLUDED_PATHS,
    ),
    command: candidate.runner.command,
    reportFormat: candidate.runner.reportFormat,
    reportSha256: sha256Bytes(Buffer.from(output, "utf8")),
    report,
  };
}

function validateReferenceEvidence(document, repositoryRoot, candidate) {
  const evidence = document.evidence.specification.referenceConformance;
  if (evidence === null) return false;
  if (candidate === null) fail("referenceConformance cannot close without the canonical index/suite tuple");
  const expected = deriveReferenceConformanceEvidence(repositoryRoot, candidate);
  exactKeys(evidence, Object.keys(expected), "evidence.specification.referenceConformance");
  if (canonicalJson(evidence) !== canonicalJson(expected)) {
    fail("reference evidence does not equal the manifest-owned executable suite report");
  }
  return true;
}

function allCandidateIdentities(candidate) {
  return candidate.families.flatMap((family) => family.candidateIdentities);
}

export function validateProviderIdentityProjection(releaseForms, candidateIdentities) {
  if (!Array.isArray(releaseForms) || releaseForms.length !== candidateIdentities.length) {
    fail("Provider 3.0 identity ledger must project every indexed Form exactly once");
  }
  const candidates = new Map(candidateIdentities.map((entry) => [formRefKey(entry.formRef), entry]));
  const identities = [];
  const seenFormRefs = new Set();
  for (const [index, entry] of releaseForms.entries()) {
    exactKeys(entry, ["resourceType", "formRef", "packageDigest"], `Provider identities[${index}]`);
    nonEmptyString(entry.resourceType, `Provider identities[${index}].resourceType`);
    validateFormRef(entry.formRef, `Provider identities[${index}].formRef`);
    const refKey = formRefKey(entry.formRef);
    const candidateIdentity = candidates.get(refKey);
    if (!candidateIdentity || candidateIdentity.packageDigest !== entry.packageDigest) {
      fail(`Provider identity ${entry.resourceType} is not an exact indexed Form projection`);
    }
    if (seenFormRefs.has(refKey)) fail(`Provider identity ledger repeats FormRef ${refKey}`);
    seenFormRefs.add(refKey);
    identities.push(entry);
  }
  identities.sort((a, b) => a.resourceType.localeCompare(b.resourceType));
  if (new Set(identities.map((entry) => entry.resourceType)).size !== identities.length) {
    fail("Provider 3.0 identity ledger repeats a Terraform resource type");
  }
  if (seenFormRefs.size !== candidates.size) {
    fail("Provider 3.0 identity ledger omits an indexed FormRef");
  }
  return identities;
}

function providerReleaseContext(repositoryRoot, candidate, providerCommit) {
  const descriptorPath = "release/version.json";
  const identityPath = "release/provider-form-identities.json";
  const descriptorBytes = gitShowBytes(repositoryRoot, providerCommit, descriptorPath);
  const identityBytes = gitShowBytes(repositoryRoot, providerCommit, identityPath);
  const descriptor = parseJson(descriptorBytes, descriptorPath);
  if (
    descriptor.version !== "3.0.0" ||
    descriptor.providerAddress !== "registry.terraform.io/tako0614/takoform" ||
    descriptor.versioning?.portableApiVersion !== HOST_API_LANE
  ) {
    fail(`Provider evidence requires the exact 3.0.0 descriptor targeting ${HOST_API_LANE}`);
  }
  const ledger = parseJson(identityBytes, identityPath);
  const release = ledger.releases?.find((entry) => entry.providerVersion === "3.0.0");
  const candidateIdentities = allCandidateIdentities(candidate);
  if (!release) fail("Provider 3.0 identity ledger has no 3.0.0 release");
  const identities = validateProviderIdentityProjection(release.forms, candidateIdentities);
  return { descriptorBytes, identityBytes, identities };
}

function validateProviderEvidence(document, repositoryRoot, candidate) {
  const evidence = document.evidence.provider;
  const result = Object.fromEntries(PROVIDER_PREREQUISITES.map((id) => [id, false]));
  if (Object.values(evidence).every((value) => value === null)) return result;
  if (candidate === null) fail("Provider evidence requires an indexed specification input tuple");
  const providerCommits = Object.values(evidence)
    .filter((value) => value !== null)
    .map((value) => value?.sourceCommit);
  if (
    providerCommits.some((commit) => typeof commit !== "string" || !COMMIT.test(commit)) ||
    new Set(providerCommits).size !== 1
  ) {
    fail("Provider evidence artifacts must name one common full Provider source commit");
  }
  const providerCommit = providerCommits[0];
  assertReachableCommit(repositoryRoot, providerCommit, "Provider source commit");
  const context = providerReleaseContext(repositoryRoot, candidate, providerCommit);
  if (evidence.exactConformance !== null || evidence.compatibilityMigrationLock !== null) {
    assertCheckoutMatchesCommit(repositoryRoot, providerCommit, PROVIDER_EXECUTION_ROOTS, "Provider 3.0");
  }

  if (evidence.exactConformance !== null) {
    runCommand(repositoryRoot, PROVIDER_COMMANDS[0], "Provider conformance");
    const observations = parseProviderMatrixObservations(
      runTextCommand(repositoryRoot, PROVIDER_COMMANDS[1], "Provider conformance"),
    );
    const expected = {
      format: PROVIDER_CONFORMANCE_FORMAT,
      providerVersion: "3.0.0",
      sourceCommit: providerCommit,
      familyIndexSha256: candidate.familyIndex.sha256,
      conformanceSuiteSha256: candidate.conformanceSuite.sha256,
      implementationSetSha256: implementationDigest(
        repositoryRoot,
        providerCommit,
        PROVIDER_IMPLEMENTATION_ROOTS,
      ),
      resourceTypes: context.identities.map((entry) => entry.resourceType),
      exactFormRefs: context.identities.map((entry) => entry.formRef),
      commands: PROVIDER_COMMANDS.map((command) => [...command]),
      observations,
    };
    exactKeys(evidence.exactConformance, Object.keys(expected), "evidence.provider.exactConformance");
    if (canonicalJson(evidence.exactConformance) !== canonicalJson(expected)) {
      fail("Provider exact conformance evidence drifted from executable Provider source");
    }
    result["provider-v3-exact-conformance"] = true;
  }

  if (evidence.identityLock !== null) {
    const expected = {
      format: PROVIDER_IDENTITY_FORMAT,
      providerVersion: "3.0.0",
      sourceCommit: providerCommit,
      descriptorSha256: sha256Bytes(context.descriptorBytes),
      identityLedgerSha256: sha256Bytes(context.identityBytes),
      exactIdentitiesSha256: canonicalDigest(context.identities),
      identities: context.identities,
    };
    exactKeys(evidence.identityLock, Object.keys(expected), "evidence.provider.identityLock");
    if (canonicalJson(evidence.identityLock) !== canonicalJson(expected)) {
      fail("Provider identity lock drifted from its append-only ledger");
    }
    result["provider-v3-identity-lock"] = true;
  }

  if (evidence.compatibilityMigrationLock !== null) {
    const releaseIdentitiesPath = "release/provider-release-identities.json";
    const migrationPath = "release/migrations/v2-to-v3.md";
    const releaseIdentityBytes = gitShowBytes(repositoryRoot, providerCommit, releaseIdentitiesPath);
    const migrationBytes = gitShowBytes(repositoryRoot, providerCommit, migrationPath);
    const releaseLedger = parseJson(releaseIdentityBytes, releaseIdentitiesPath);
    const versions = (releaseLedger.entries ?? []).map((entry) => entry.version);
    for (const version of ["1.0.3", "2.0.0", "2.1.1", "3.0.0"]) {
      if (!versions.includes(version)) fail(`Provider release ledger is missing ${version}`);
    }
    const passedTests = runProviderCompatibility(repositoryRoot);
    const expected = {
      format: PROVIDER_COMPATIBILITY_FORMAT,
      providerVersion: "3.0.0",
      sourceCommit: providerCommit,
      retainedVersions: ["1.0.3", "2.0.0", "2.1.1"],
      releaseIdentityLedger: {
        path: releaseIdentitiesPath,
        sha256: sha256Bytes(releaseIdentityBytes),
      },
      migration: { path: migrationPath, sha256: sha256Bytes(migrationBytes) },
      providerIdentityLedgerSha256: sha256Bytes(context.identityBytes),
      command: [...PROVIDER_COMPATIBILITY_COMMAND],
      passedTests,
    };
    exactKeys(
      evidence.compatibilityMigrationLock,
      Object.keys(expected),
      "evidence.provider.compatibilityMigrationLock",
    );
    if (canonicalJson(evidence.compatibilityMigrationLock) !== canonicalJson(expected)) {
      fail("Provider compatibility/migration lock drifted from retained history");
    }
    result["provider-v3-compatibility-migration-lock"] = true;
  }
  return result;
}

function validateRetainedHistory(document, repositoryRoot) {
  exactKeys(document.retainedHistory, ["path", "format", "lane", "sha256"], "retainedHistory");
  if (
    document.retainedHistory.path !== RETAINED_HISTORY_PATH ||
    document.retainedHistory.format !== "takoform.publication-blockers@v1" ||
    document.retainedHistory.lane !== "forms.takoform.com/v1beta1" ||
    document.retainedHistory.sha256 !== RETAINED_HISTORY_SHA256
  ) {
    fail("retained v1beta1 publication history identity drifted");
  }
  const head = git(repositoryRoot, ["rev-parse", "HEAD"]).trim();
  assertWorktreePathsClean(repositoryRoot, [RETAINED_HISTORY_PATH], "retained v1beta1 publication ledger");
  if (sha256Bytes(gitShowBytes(repositoryRoot, head, RETAINED_HISTORY_PATH)) !== RETAINED_HISTORY_SHA256) {
    fail("retained v1beta1 publication ledger bytes changed");
  }
}

function validateAxes(document) {
  exactKeys(
    document.identityAxes,
    [
      "hostApiLane",
      "familyGroupSet",
      "familyGroupsAreVersionless",
      "formRefIdentity",
      "packageEnvelope",
      "standardServiceContract",
      "standardServiceProtocolIdentity",
      "specificationAxis",
      "formMaturityAxis",
      "specificationReleaseFormEffect",
      "providerAxis",
      "hostAxis",
      "packageAxis",
      "interfaceAxis",
      "bindingAxis",
    ],
    "identityAxes",
  );
  if (
    document.identityAxes.hostApiLane !== HOST_API_LANE ||
    document.identityAxes.familyGroupSet !== "current-family-index-derived" ||
    document.identityAxes.familyGroupsAreVersionless !== true ||
    document.identityAxes.packageEnvelope !== PACKAGE_ENVELOPE ||
    document.identityAxes.standardServiceContract !== STANDARD_SERVICE_API ||
    document.identityAxes.standardServiceProtocolIdentity !== "opaque-reverse-dns-minimum-three-labels" ||
    document.identityAxes.formMaturityAxis !== "independent" ||
    document.identityAxes.specificationReleaseFormEffect !== "preserve-exact-current-formrefs"
  ) {
    fail("current Host/family/package/standard-service identities drifted");
  }
  exactArray(
    document.identityAxes.formRefIdentity,
    ["group", "kind", "definitionVersion", "schemaDigest"],
    "identityAxes.formRefIdentity",
  );
  for (const axis of [
    "specificationAxis",
    "formMaturityAxis",
    "providerAxis",
    "hostAxis",
    "packageAxis",
    "interfaceAxis",
    "bindingAxis",
  ]) {
    if (document.identityAxes[axis] !== "independent") {
      fail(`identityAxes.${axis} must remain independent`);
    }
  }
}

function validateTracks(document) {
  if (!Array.isArray(document.releaseTracks) || document.releaseTracks.length !== 2) {
    fail("releaseTracks must contain exactly Specification 1.1 and Provider 3.0");
  }
  const [specification, provider] = document.releaseTracks;
  exactKeys(specification, ["id", "version", "authority", "normative", "prerequisites"], "releaseTracks[0]");
  if (
    specification.id !== SPECIFICATION_TRACK ||
    specification.version !== SPECIFICATION_VERSION ||
    specification.authority !== "specification" ||
    specification.normative !== true
  ) {
    fail("releaseTracks[0] must be the normative Specification 1.1 track");
  }
  exactArray(specification.prerequisites, SPECIFICATION_PREREQUISITES, "Specification prerequisites");
  exactKeys(provider, ["id", "version", "authority", "normative", "prerequisites"], "releaseTracks[1]");
  if (
    provider.id !== PROVIDER_TRACK ||
    provider.version !== "3.0.0" ||
    provider.authority !== "official-terraform-provider" ||
    provider.normative !== false
  ) {
    fail("releaseTracks[1] must be the non-normative official Provider 3.0 track");
  }
  exactArray(provider.prerequisites, PROVIDER_PREREQUISITES, "Provider prerequisites");
}

function rows(ids, values) {
  return ids.map((id) => ({ id, status: values[id] ? "closed" : "open" }));
}

function track(id, authority, normative, prerequisites) {
  return {
    id,
    authority,
    normative,
    status: prerequisites.every((entry) => entry.status === "closed") ? "ready" : "open",
    prerequisites,
  };
}

export function validatePublicationEvidence(document, options = {}) {
  if (Object.keys(options).some((key) => !["repositoryRoot", "releaseTrack"].includes(key))) {
    fail("caller repository mappings, result injectors, and authority overrides are forbidden");
  }
  const releaseTrack = options.releaseTrack ?? null;
  if (releaseTrack !== null && ![SPECIFICATION_TRACK, PROVIDER_TRACK].includes(releaseTrack)) {
    fail(`unknown release track ${releaseTrack}`);
  }
  const repositoryRoot = path.resolve(options.repositoryRoot ?? path.resolve(import.meta.dirname, ".."));
  exactKeys(
    document,
    [
      "format",
      "policy",
      "repositoryAuthority",
      "identityAxes",
      "releaseTracks",
      "candidateBaseline",
      "retainedHistory",
      "evidence",
    ],
    "publication evidence document",
  );
  if (document.format !== EVIDENCE_FORMAT) fail(`format must be ${EVIDENCE_FORMAT}`);
  nonEmptyString(document.policy, "policy");
  validateRepositoryAuthority(document, repositoryRoot);
  validateAxes(document);
  validateTracks(document);
  validateRetainedHistory(document, repositoryRoot);
  exactKeys(document.evidence, ["specification", "provider"], "evidence");
  exactKeys(
    document.evidence.specification,
    ["sourceSnapshot", "candidateCorpus", "referenceConformance"],
    "evidence.specification",
  );
  exactKeys(
    document.evidence.provider,
    ["exactConformance", "identityLock", "compatibilityMigrationLock"],
    "evidence.provider",
  );

  const candidate = releaseTrack === SPECIFICATION_TRACK
    ? null
    : deriveCandidateCorpusEvidence(repositoryRoot, document.candidateBaseline);
  const specificationValues = releaseTrack === PROVIDER_TRACK
    ? Object.fromEntries(SPECIFICATION_PREREQUISITES.map((id) => [id, false]))
    : {
        "specification-source-snapshot": validateSourceEvidence(document, repositoryRoot),
      };
  if (releaseTrack !== SPECIFICATION_TRACK) {
    validateCandidateEvidence(document, candidate);
    validateReferenceEvidence(document, repositoryRoot, candidate);
  }
  const providerValues = releaseTrack === SPECIFICATION_TRACK
    ? Object.fromEntries(PROVIDER_PREREQUISITES.map((id) => [id, false]))
    : validateProviderEvidence(document, repositoryRoot, candidate);
  if (
    Object.values(document.evidence.specification).some((value) => value !== null) ||
    Object.values(document.evidence.provider).some((value) => value !== null)
  ) {
    assertEvidenceRecordCommitted(document, repositoryRoot);
  }
  const tracks = [
    track(SPECIFICATION_TRACK, "specification", true, rows(SPECIFICATION_PREREQUISITES, specificationValues)),
    track(PROVIDER_TRACK, "official-terraform-provider", false, rows(PROVIDER_PREREQUISITES, providerValues)),
  ];
  const edge = candidate?.families.find((entry) => entry.group === EDGE_FAMILY_GROUP);
  const totalCandidateForms = candidate?.families.reduce((sum, entry) => sum + entry.formCount, 0) ?? 0;
  const totalRunnerForms = candidate?.families.reduce((sum, entry) => sum + entry.runnerIdentities.length, 0) ?? 0;
  const missingFormRefs = candidate?.families.flatMap((entry) => entry.missingFormRefs) ?? [];
  return {
    format: REPORT_FORMAT,
    candidate: {
      available: candidate !== null,
      sourceCommit: document.candidateBaseline.commit,
      familyCount: candidate?.families.length ?? 0,
      totalCandidateForms,
      totalRunnerForms,
      missingFormRefs,
      edgeCandidateForms: edge?.formCount ?? 0,
      edgeRunnerForms: edge?.runnerIdentities.length ?? 0,
      edgeRosterExact:
        edge?.formCount === EDGE_CANDIDATE_SIZE &&
        edge.runnerIdentities.length === EDGE_CANDIDATE_SIZE &&
        edge.missingFormRefs.length === 0 &&
        edge.candidateIdentities.every((entry) => entry.formRef.kind !== "ObjectBucket"),
    },
    tracks,
    status: tracks.every((entry) => entry.status === "ready") ? "ready" : "open",
  };
}

export function loadPublicationEvidence(repositoryRoot = path.resolve(import.meta.dirname, "..")) {
  return parseJson(readFileSync(path.join(repositoryRoot, PUBLICATION_EVIDENCE)), PUBLICATION_EVIDENCE);
}

export function prepareSpecificationEvidence(
  repositoryRoot = path.resolve(import.meta.dirname, ".."),
) {
  const document = loadPublicationEvidence(repositoryRoot);
  if (
    Object.values(document.evidence.specification).some((value) => value !== null)
  ) {
    fail("Specification evidence is already closed; preparation is create-only");
  }
  assertWorktreePathsClean(
    repositoryRoot,
    [PUBLICATION_EVIDENCE, ...PUBLICATION_EVIDENCE_PROJECTION_PATHS],
    "Specification evidence preparation input",
  );
  const sourceCommit = git(repositoryRoot, ["rev-parse", "HEAD"]).trim();
  assertReachableCommit(repositoryRoot, sourceCommit, "Specification source commit");
  document.evidence.specification.sourceSnapshot =
    deriveSpecificationSourceSnapshot(repositoryRoot, sourceCommit);
  const raw = `${JSON.stringify(document, null, 2)}\n`;
  for (const relativePath of [
    PUBLICATION_EVIDENCE,
    ...PUBLICATION_EVIDENCE_PROJECTION_PATHS,
  ]) {
    writeFileSync(path.join(repositoryRoot, relativePath), raw);
  }
  return document;
}

export function assertPublicationEvidenceReady(report, trackId = null) {
  const selected = trackId === null ? report.tracks : report.tracks.filter((entry) => entry.id === trackId);
  if (selected.length === 0) fail(`unknown release track ${trackId}`);
  const open = selected.flatMap((entry) =>
    entry.prerequisites
      .filter((prerequisite) => prerequisite.status !== "closed")
      .map((prerequisite) => `${entry.id}:${prerequisite.id}`),
  );
  if (open.length !== 0) fail(`open release evidence: ${open.join(", ")}`);
}

function main() {
  const mode = process.argv[2];
  const allowed = [
    "--prepare-specification-1-1",
    "--prepare-specification-v1",
    "--check",
    "--assert-specification-1-1",
    "--assert-specification-v1",
    "--assert-provider-3",
    "--assert-all",
  ];
  if (process.argv.length !== 3 || !allowed.includes(mode)) {
    fail(`usage: bun scripts/publication-evidence.mjs ${allowed.join("|")}`);
  }
  const repositoryRoot = path.resolve(import.meta.dirname, "..");
  if (mode === "--prepare-specification-1-1" || mode === "--prepare-specification-v1") {
    const document = prepareSpecificationEvidence(repositoryRoot);
    process.stdout.write(
      `prepared exact Specification 1.1 source evidence and two byte-identical website projections for ${document.evidence.specification.sourceSnapshot.sourceCommit}; commit the exact three-path record before assertion\n`,
    );
    return;
  }
  const releaseTrack = mode === "--assert-specification-1-1" || mode === "--assert-specification-v1"
    ? SPECIFICATION_TRACK
    : mode === "--assert-provider-3"
      ? PROVIDER_TRACK
      : null;
  const report = validatePublicationEvidence(loadPublicationEvidence(repositoryRoot), {
    repositoryRoot,
    releaseTrack,
  });
  if (mode === "--assert-specification-1-1" || mode === "--assert-specification-v1") {
    assertPublicationEvidenceReady(report, SPECIFICATION_TRACK);
  } else if (mode === "--assert-provider-3") {
    assertPublicationEvidenceReady(report, PROVIDER_TRACK);
  } else if (mode === "--assert-all") {
    assertPublicationEvidenceReady(report);
  }
  const [specification, provider] = report.tracks;
  const tuple = report.candidate.available
    ? `${report.candidate.familyCount} families, ${report.candidate.totalCandidateForms}/${report.candidate.totalRunnerForms} Forms`
    : "canonical family index/conformance suite absent";
  process.stdout.write(
    `release evidence OK: ${tuple}; Specification 1.1 ${specification.status}; Provider 3.0 ${provider.status}\n`,
  );
}

if (import.meta.main) main();

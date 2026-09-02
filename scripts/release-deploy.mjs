import { createHash, randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  accessSync,
  chmodSync,
  constants as fsConstants,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { delimiter, isAbsolute, join, relative, resolve, sep } from "node:path";
import { tmpdir } from "node:os";
import process from "node:process";

import {
  assertSafeRepositoryGitConfiguration,
  assertManagedGateState,
  assertManagedToolSnapshot,
  createHardenedGateEnvironment,
  createHardenedGitEnvironment,
  createManagedGateState,
  createManagedToolSnapshot,
  prepareManagedHomeForRemoval,
} from "./deploy-safety.mjs";
import {
  LEDGER_PATH as SPECIFICATION_LEDGER_PATH,
  SOURCE_EVIDENCE_ASSET,
  SOURCE_EVIDENCE_PATH,
  SPECIFICATION_TAG as SPECIFICATION_RELEASE_TAG,
  appendReleaseReceipt,
  releaseFromEvidence,
  sourceSnapshotDigest,
  validateC2DiffPaths,
  validateSpecificationRecoveryPath,
} from "./specification-release.mjs";
import { generateProvider4Identities } from "./provider4-candidate.mjs";

const GITHUB_REPOSITORY = "tako0614/terraform-provider-takoform";
const SOURCE_REPOSITORY = `https://github.com/${GITHUB_REPOSITORY}.git`;
const PROVIDER_ADDRESS = "registry.terraform.io/tako0614/takoform";
const PROVIDER_SIGNER = "3510E75E05BBCC303B92D77934FC18AC897FB709";
const PROVIDER_VERSION = "4.0.0";
// Retained majors keep their published identity in the append-only ledgers;
// the writer only ever addresses PROVIDER_VERSION.
const PROVIDER_RETAINED_AGGREGATE_VERSION = "3.0.0";
const PROVIDER_HOST_API = "forms.takoform.com/v1";
const PROVIDER_IDENTITY_LEDGER = "release/provider-form-identities.json";
const PROVIDER_CURRENT_FAMILY_INDEX =
  "forms/candidates/current-family-index.json";
const PROVIDER_V211_LEDGER_DIGEST =
  "sha256:981181257fac1ec43f85eb250fc12dd271236b1bbde94dc93323ee2180c4255d";
// Provider 3.0.0 is Registry-published immutable history. Its 31-Form embedded
// identity lost its implicit byte protection the moment the descriptor moved
// off it, so it is pinned explicitly here and in the Go mirror's
// frozenLedgerEntryDigests.
const PROVIDER_V300_LEDGER_DIGEST =
  "sha256:165f9377f0a37d1994d96e28c7494dc71dc4e6457d0679229a7f1819c17f77fb";
const PROVIDER_WITHDRAWN_V1ALPHA2_RESOURCE_TYPES = new Set([
  "takoform_edge_worker",
  "takoform_relational_database",
  "takoform_object_bucket",
  "takoform_key_value_store",
  "takoform_queue",
  "takoform_schedule",
  "takoform_container_service",
  "takoform_stateful_entity",
  "takoform_vector_index",
]);
const OIDC_ISSUER = "https://token.actions.githubusercontent.com";
const TRUSTED_ROOT = "release/trust/trusted-root.json";
const PINNED_GH_VERSION = "2.96.0";
const PINNED_COSIGN_VERSION = "v3.0.6";
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const COMMIT = /^[0-9a-f]{40}$/u;
const GIT_OBJECT = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u;
const POSITIVE_INTEGER = /^[1-9][0-9]*$/u;
const REQUEST_ID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;
const PROVIDER_TAG =
  /^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$/u;
const REVOCATION_TAG =
  /^forms\/revocations\/v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$/u;
const FORM_RELEASE_AUTHORITY_PATHS = Object.freeze([
  ".github/workflows/form-package-revocation.yml",
  TRUSTED_ROOT,
  "cmd/form-package-release",
  "formpackage",
  "go.mod",
  "go.sum",
  "internal/admissioncheckpoint",
  "internal/admissionrelease",
  "internal/client",
  "internal/formcatalog",
  "internal/formregistry",
  "internal/hostpolicy",
  "internal/portableconformance",
  "internal/provider",
  "internal/providerlifecycle",
  "internal/standardforms",
  "package.json",
  "scripts/release-deploy.mjs",
  "standardform",
]);
const PROVIDER_RECOVERY_ALLOWED_PATHS = Object.freeze([
  "release/README.md",
  "scripts/check-public-surfaces.mjs",
  "scripts/release-deploy.mjs",
  "scripts/release-deploy.test.mjs",
  "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
]);
const PROVIDER_SURFACE = "takoform-provider-release";
const FORM_SURFACE = "takoform-form-package-release";
export const SPECIFICATION_SURFACE = "takoform-specification-release";
export const SPECIFICATION_TAG = SPECIFICATION_RELEASE_TAG;
const WORKFLOW_NAMES = {
  "provider-release-tag.yml": "Author provider release tag",
  "release.yml": "Prepare provider release candidate",
  "form-package-revocation.yml":
    "Prepare signed Form Package revocation checkpoint",
};

export const RELEASE_SURFACES = Object.freeze([
  {
    surface: PROVIDER_SURFACE,
    target:
      "github-release:tako0614/terraform-provider-takoform/provider + registry.terraform.io/tako0614/takoform",
    covers: [
      "release/version.json",
      "release/keys/provider-signing-key.asc",
      ".github/workflows/provider-release-tag.yml",
      ".github/workflows/release.yml",
      "cmd/provider-release",
    ],
    requiresScripts: ["check"],
    requiresTools: ["git", "bun", "gh", "go", "gpg", "cosign", "curl"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "requires local operator GH_TOKEN authority, a clean non-shallow main checkout equal to a freshly fetched canonical origin/main, the complete owner check before every dispatch, tag push, or release mutation, an explicitly named successful workflow run/attempt, checksum closure over its same-run candidate, the pinned provider GPG signer, and a record of the source commit plus every published asset digest; GH_TOKEN is never printed or retained",
      "post-conditions":
        "publishes the exact verified same-run bytes locally under exclusive single-writer authority because GitHub REST has no atomic asset-plus-metadata precondition, immediately rereads the empty and complete exact draft, restates the full exact identity on PATCH, requires GitHub's immutable release readback and a fresh download with identical digests, and rechecks the pinned signed tag before VERIFIED",
      reversal:
        "provider versions, signed tags, and release assets are immutable and cannot be rolled back or overwritten; an exact signed tag-only partial state may only be completed by recover-tag-only, and an exact retained draft may only be resumed by recover-draft without changing the tag, candidate, or draft identity; any other bad publication is halted and repaired forward under a new version",
      "failure-handling":
        "prints raw command diagnostics and distinguishes failure before tag creation, after local tag materialization, after remote tag push, after draft creation, and after immutable publication; it retains and reports every created draft for authoritative inspection, never automatically removes a release or tag, refuses blind retry after an indeterminate mutation, and exposes only explicit exact-tag and exact-draft recovery bound to the original release commit, tag object, run attempt, and current reviewed recovery commit",
      "independent-review":
        "the provider-release protected Environment reviews the signed tag and release candidates; the local publisher accepts only that exact successful run/attempt and re-verifies it independently before using local GitHub authority",
      "no-overwrite":
        "requires the descriptor tag, refuses any pre-existing local/remote tag, Registry version, or GitHub Release before creation, uses zero-object-id compare-and-swap for local refs, pushes an exact ref, and accepts only an immutable release with the exact candidate inventory; recovery never mutates or deletes a tag and accepts only the exact pinned signed local/remote annotated object",
      halt: "prepare and tag stop after one exact workflow dispatch and return its URL as AWAITING_REVIEW; cancel that exact run before approval on any input or evidence mismatch, and never continue by selecting a latest run",
    },
  },
  {
    surface: FORM_SURFACE,
    target:
      "github-release:tako0614/terraform-provider-takoform/forms/revocations/*",
    covers: [
      "forms/revocations",
      ".github/workflows/form-package-revocation.yml",
      "cmd/form-package-release",
    ],
    requiresScripts: ["check"],
    requiresTools: ["git", "bun", "gh", "go", "cosign"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "uses local operator GH_TOKEN authority without printing or retaining GH_TOKEN; every phase accepts one exact forms/revocations/v<semver> tag plus its exact source commit, requires the committed revocation source and checkpoint files, a clean non-shallow current protected main that descends from that source commit, and the pinned gh/cosign toolchain, runs the complete owner check before dispatch and again before every mutation, consumes only one explicitly named successful revocation-workflow run/attempt, checksum-closes that same-run candidate, verifies its deterministic tag object, source/tooling commit ancestry, and Sigstore identity against the trusted root retained at the tooling commit, and records the exact tag object and every published asset digest",
      "post-conditions":
        "after the create-only tag push and Release publication, publish-revocation requires exact remote tag resolution, one immutable release with the exact id/tag identity, exact API asset digests, and a fresh six-file revocation download identical to the verified candidate; verify-revocation closes the published identity by re-downloading the live release, re-verifying its manifest, checksum closure, deep Go semantic report, and Sigstore bundle, and binding the published source commit to the expected commit and its tooling commit to an ancestor of current protected main",
      reversal:
        "revocation checkpoints are append-only cumulative identities: a published checkpoint tag or release is never overwritten, deleted, or rolled back, and no automated recovery phase exists; a wrong or incomplete checkpoint is superseded only by a later cumulative checkpoint under a new version, and an interrupted publication leaves the reported exact tag/release state for operator inspection instead of being resumed blindly",
      "failure-handling":
        "prints raw command diagnostics, reports the exact local and remote ref state after a tag-side failure and the retained draft or ambiguous release identities after a release-side failure, never automatically removes a release or tag, and refuses blind retry whenever mutation state is indeterminate",
      "independent-review":
        "the form-package-release protected Environment reviews the revocation workflow run that builds and keyless-signs the exact cumulative checkpoint candidate; local publication consumes only that named successful run/attempt and independently re-verifies its checksum, deterministic tag object, and Sigstore closure before any mutation",
      "no-overwrite":
        "the exact tag must be absent locally and remotely and no GitHub Release identity may exist for it before creation; local tag creation is zero-object-id compare-and-swap over the exact reconstructed candidate object, the remote push is a create-only lease of that exact ref, and final readback accepts only one immutable release with the exact candidate inventory, so every checkpoint identity only appends",
      halt: "prepare-revocation stops after one exact workflow dispatch and returns its URL as AWAITING_REVIEW; cancel that exact run before approval if any input changes, and publish-revocation never infers or selects a latest run — it consumes only the explicitly named run/attempt",
    },
  },
  {
    surface: SPECIFICATION_SURFACE,
    target:
      "github-release:tako0614/terraform-provider-takoform/specification/1.1",
    covers: [
      "spec",
      "release/specification-releases.json",
      "spec/publication-evidence.json",
      "scripts/specification-release.mjs",
    ],
    requiresScripts: [
      "check",
      "check:specification-releases",
      "check:specification-1-1-release",
    ],
    requiresTools: ["git", "bun", "gh"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "uses the canonical old repository as the sole Specification 1.1 authority, requires a clean non-shallow attached main equal to freshly fetched origin/main, runs the complete owner gate, requires C2 to be the direct evidence-only child of the normative C1 commit, and binds the exact committed source-snapshot evidence bytes; the normal history is C2 -> C3 -> C4, while the sole exceptional history is C2 -> R -> C3 -> C4 where R is one direct single-parent child of C2 whose diff includes scripts/release-deploy.mjs and contains only scripts/release-deploy.mjs, scripts/release-deploy.test.mjs, scripts/specification-release.mjs, and scripts/specification-release.test.mjs; the five-class compatibility report remains a separately checked W09 report and is not release authority",
      "post-conditions":
        "publishes at most one create-only Specification 1.1 identity bound to annotated specification/1.1 and the exact C1 source/C2 release commits, rereads the immutable release and exact downloaded source evidence bytes, and appends one C3 receipt only after authoritative tag/release readback matches; record-receipt requires expected-recovery-commit equal to C2 normally or the exact R during recovery, binds current protected main to it, repeats the recovery fence before live readback and immediately before writing, compares the C3 diff with its immediate C2-or-R base, and keeps receipt.releaseCommit, tag, release body, and asset authority bound to immutable C2; a separate non-authoritative C4 derived-public-truth commit is required before the branch is green, and no Form, package, Provider, Host API lane, v2 schema, v2 tag, or v2 receipt is created",
      reversal:
        "an unpublished C1/C2 candidate may be withdrawn; an exact tag-only state or exact retained draft may be completed only by the explicit bound recovery phases, while any mismatched publication is halted for forward repair and a published numbered Specification identity is never deleted, overwritten, or reissued",
      "failure-handling":
        "fails closed before any mutation on missing source evidence, non-evidence C1/C2 drift, a dirty, shallow, detached, stale, or non-canonical checkout, an existing identity, an extra or merge-parent recovery edge, a forbidden recovery path, or an ambiguous GitHub response; failures report the exact surface and observed local tag, remote tag, draft, and immutable release state, never delete an identity, and never retry blindly",
      "independent-review":
        "the operator reviews the exact C1 source/C2 release commits, separately checked five-class compatibility report, reserved 1.0/no-reuse rule, and zero Host/Form/Provider effects before invoking publish; no compatibility report, Provider, Host, product, signer, or adoption result can substitute for the normative source prerequisite",
      "no-overwrite":
        "requires specification/1.1 and its GitHub Release identity to be absent, creates one deterministic annotated object with a zero-object-id local compare-and-swap, uses a create-only remote lease, and rejects any existing or mismatched tag/release without transferring or reusing withdrawn 1.0",
      halt: "prepare returns AWAITING_REVIEW after mutation-free C1 preflight; publish consumes only the exact pushed C2 commit, explicit recovery consumes only an exact tag-only or retained-draft state at the single allowed R, verify is read-only, record-receipt accepts only the exact C2-or-R protected-main pin and writes only the C3 ledger projections after live exact readback, and deterministic public-truth regeneration remains a separate C4 commit",
    },
  },
]);

const PHASES = {
  [PROVIDER_SURFACE]: {
    prepare: ["tag", "expected-commit"],
    tag: ["tag", "expected-commit", "run-id", "run-attempt"],
    publish: ["tag", "expected-commit", "run-id", "run-attempt"],
    "recover-tag-only": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "run-id",
      "run-attempt",
    ],
    "recover-draft": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "release-id",
      "run-id",
      "run-attempt",
    ],
  },
  [FORM_SURFACE]: {
    "prepare-revocation": ["tag", "expected-commit"],
    "publish-revocation": ["tag", "expected-commit", "run-id", "run-attempt"],
    "verify-revocation": ["tag", "expected-commit"],
  },
  [SPECIFICATION_SURFACE]: {
    prepare: ["tag", "expected-commit"],
    publish: ["tag", "expected-commit"],
    "recover-tag-only": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
    ],
    "recover-draft": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "release-id",
    ],
    verify: [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "release-id",
    ],
    "record-receipt": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "release-id",
    ],
  },
};

export function isReleaseSurface(name) {
  return (
    name === PROVIDER_SURFACE ||
    name === FORM_SURFACE ||
    name === SPECIFICATION_SURFACE
  );
}

export function parseReleaseSurfaceArgs(surface, args) {
  if (!isReleaseSurface(surface)) {
    throw new Error(`unknown release surface ${JSON.stringify(surface)}`);
  }
  if (!Array.isArray(args) || args.length === 0 || args[0].startsWith("-")) {
    throw usageError(surface);
  }
  const [phase, ...rest] = args;
  const required = PHASES[surface][phase];
  if (!required) throw usageError(surface);
  const values = {};
  for (let index = 0; index < rest.length; index += 2) {
    const option = rest[index];
    const value = rest[index + 1];
    if (
      typeof option !== "string" ||
      !option.startsWith("--") ||
      option === "--" ||
      typeof value !== "string" ||
      value === "" ||
      value.startsWith("--")
    ) {
      throw usageError(surface);
    }
    const name = option.slice(2);
    if (!required.includes(name) || Object.hasOwn(values, name)) {
      throw usageError(surface);
    }
    values[name] = value;
  }
  if (
    rest.length !== required.length * 2 ||
    required.some((name) => !Object.hasOwn(values, name))
  ) {
    throw usageError(surface);
  }
  if (surface === SPECIFICATION_SURFACE && values.tag !== SPECIFICATION_TAG) {
    throw new Error(
      `--tag must be exactly ${SPECIFICATION_TAG} for the create-only Specification 1.1 surface`,
    );
  }
  for (const name of [
    "expected-commit",
    "expected-release-commit",
    "expected-recovery-commit",
  ]) {
    if (values[name] && !COMMIT.test(values[name])) {
      throw new Error(
        `--${name} must be an exact lowercase 40-character commit`,
      );
    }
  }
  if (
    values["expected-tag-object"] &&
    !GIT_OBJECT.test(values["expected-tag-object"])
  ) {
    throw new Error(
      "--expected-tag-object must be an exact lowercase Git object id",
    );
  }
  for (const name of ["release-id", "run-id", "run-attempt"]) {
    if (values[name] && !POSITIVE_INTEGER.test(values[name])) {
      throw new Error(`--${name} must be a positive decimal integer`);
    }
    if (values[name] && !Number.isSafeInteger(Number(values[name]))) {
      throw new Error(`--${name} must be a safe positive decimal integer`);
    }
  }
  return { phase, ...values };
}

function usageError(surface) {
  const phases = Object.entries(PHASES[surface] ?? {})
    .map(
      ([phase, options]) =>
        `  ${phase}${options.map((name) => ` --${name} <value>`).join("")}`,
    )
    .join("\n");
  return new Error(
    `usage: bun run deploy -- ${surface} <phase> [exact options]\n${phases}`,
  );
}

export function runReleaseSurface({
  surface,
  args,
  repo,
  stdout = process.stdout,
  stderr = process.stderr,
  execFile = execFileSync,
  uuidFactory = randomUUID,
  now = Date.now,
  wait = blockingWait,
}) {
  if (!repo || typeof repo !== "string") throw new Error("repo is required");
  if (
    typeof process.env.GH_TOKEN !== "string" ||
    process.env.GH_TOKEN.trim() === ""
  ) {
    throw new Error(
      "release blocked: non-empty GH_TOKEN is required for local owner authority",
    );
  }
  return withTemporaryDirectory(
    "takoform-release-gh-config",
    (githubConfigDirectory) => {
      chmodSync(githubConfigDirectory, 0o700);
      const context = {
        repo: resolve(repo),
        stdout,
        stderr,
        execFile,
        uuidFactory,
        now,
        wait,
        githubConfigDirectory: realpathSync(githubConfigDirectory),
      };
      const options = parseReleaseSurfaceArgs(surface, args);
      if (surface === PROVIDER_SURFACE) {
        return runProvider(context, options);
      }
      if (surface === FORM_SURFACE) {
        return runForm(context, options);
      }
      return runSpecification(context, options);
    },
  );
}

function blockingWait(milliseconds) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds);
}

function runProvider(context, options) {
  const descriptor = readProviderDescriptor(context.repo);
  requireProviderTag(options.tag, descriptor);
  switch (options.phase) {
    case "prepare":
      return providerPrepare(context, options, descriptor);
    case "tag":
      return providerTag(context, options, descriptor);
    case "publish":
      return providerPublish(context, options, descriptor);
    case "recover-tag-only":
      return providerRecoverTagOnly(context, options, descriptor);
    case "recover-draft":
      return providerRecoverDraft(context, options, descriptor);
    default:
      throw usageError(PROVIDER_SURFACE);
  }
}

function runForm(context, options) {
  switch (options.phase) {
    case "prepare-revocation":
      return revocationPrepare(context, options);
    case "publish-revocation":
      return revocationPublish(context, options);
    case "verify-revocation":
      return revocationVerify(context, options);
    default:
      throw usageError(FORM_SURFACE);
  }
}

function requireSpecificationTag(tag) {
  if (tag !== SPECIFICATION_TAG) {
    throw new Error(
      `Specification release tag must be exactly ${SPECIFICATION_TAG}; no /v1.1 or v2 lane/tag is permitted`,
    );
  }
}

function runSpecification(context, options) {
  requireSpecificationTag(options.tag);
  switch (options.phase) {
    case "prepare":
      return specificationPrepare(context, options);
    case "publish":
      return specificationPublish(context, options);
    case "recover-tag-only":
      return specificationRecoverTagOnly(context, options);
    case "recover-draft":
      return specificationRecoverDraft(context, options);
    case "verify":
      return specificationVerify(context, options);
    case "record-receipt":
      return specificationRecordReceipt(context, options);
    default:
      throw usageError(SPECIFICATION_SURFACE);
  }
}

function parseCommittedJSON(raw, label) {
  try {
    return JSON.parse(asText(raw));
  } catch (error) {
    throw new Error(
      `${label} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
}

function committedBytes(context, commit, relativePath) {
  return command(context, "git", ["show", `${commit}:${relativePath}`], {
    encoding: null,
    label: `read committed ${relativePath} at ${commit}`,
  });
}

function exactDiffPaths(context, base, head, { recovery = false } = {}) {
  const runner = recovery
    ? recoveryGit
    : (ctx, args, options) => command(ctx, "git", args, options);
  const raw = runner(
    context,
    [
      "-c",
      "diff.renames=false",
      "diff",
      "--no-renames",
      "--no-ext-diff",
      "--no-textconv",
      "--name-only",
      "-z",
      "--diff-filter=ACDMRTUXB",
      base,
      head,
      "--",
    ],
    { label: `read exact changed paths for ${base}..${head}` },
  );
  if (typeof raw !== "string" || (raw !== "" && !raw.endsWith("\0"))) {
    throw new Error("Specification release diff path output is ambiguous");
  }
  const paths = raw === "" ? [] : raw.slice(0, -1).split("\0");
  if (
    paths.some((entry) => entry === "" || /[\r\n]/u.test(entry)) ||
    new Set(paths).size !== paths.length
  ) {
    throw new Error("Specification release diff contains an ambiguous path");
  }
  return paths;
}

function assertSpecificationC2Fence(context, sourceCommit, releaseCommit) {
  const ancestry = git(
    context,
    "rev-list",
    "--parents",
    "-n",
    "1",
    releaseCommit,
  ).split(" ");
  const parents = ancestry[0] === releaseCommit ? ancestry.slice(1) : [];
  if (
    ancestry.some((entry) => !COMMIT.test(entry)) ||
    parents.length !== 1 ||
    parents[0] !== sourceCommit
  ) {
    throw new Error(
      `Specification C2 ${releaseCommit} must be the direct single-parent evidence-only child of C1 ${sourceCommit}; observed parents ${parents.join(", ") || "unreadable"}`,
    );
  }
  const paths = exactDiffPaths(context, sourceCommit, releaseCommit);
  const problems = validateC2DiffPaths(paths);
  if (problems.length !== 0) {
    throw new Error(`Specification C1/C2 fence failed: ${problems.join("; ")}`);
  }
  const c1 = parseCommittedJSON(
    committedBytes(context, sourceCommit, SOURCE_EVIDENCE_PATH),
    `${SOURCE_EVIDENCE_PATH} at C1`,
  );
  if (c1?.evidence?.specification?.sourceSnapshot !== null) {
    throw new Error("Specification C1 sourceSnapshot must be null");
  }
  const c2Ledger = parseCommittedJSON(
    committedBytes(context, releaseCommit, SPECIFICATION_LEDGER_PATH),
    `${SPECIFICATION_LEDGER_PATH} at C2`,
  );
  if (!Array.isArray(c2Ledger.releases) || c2Ledger.releases.length !== 0) {
    throw new Error("Specification C2 must not contain a release receipt");
  }
  return paths;
}

function specificationPublicationInput(context, releaseCommit) {
  const evidenceBytes = committedBytes(
    context,
    releaseCommit,
    SOURCE_EVIDENCE_PATH,
  );
  const document = parseCommittedJSON(
    evidenceBytes,
    `${SOURCE_EVIDENCE_PATH} at C2`,
  );
  const sourceSnapshot = document?.evidence?.specification?.sourceSnapshot;
  if (!sourceSnapshot || !COMMIT.test(sourceSnapshot.sourceCommit ?? "")) {
    throw new Error(
      "Specification C2 evidence must bind one exact C1 source commit",
    );
  }
  const sourceCommit = sourceSnapshot.sourceCommit;
  assertSpecificationC2Fence(context, sourceCommit, releaseCommit);
  const sourceSnapshotSha256 = sourceSnapshotDigest(sourceSnapshot);
  const sourceEvidenceSha256 = sha256(evidenceBytes);
  const timestamp = git(context, "show", "-s", "--format=%ct", releaseCommit);
  if (!/^(?:0|[1-9][0-9]*)$/u.test(timestamp)) {
    throw new Error("Specification C2 commit timestamp is not canonical");
  }
  const tagObjectBytes = Buffer.from(
    `object ${releaseCommit}\n` +
      "type commit\n" +
      `tag ${SPECIFICATION_TAG}\n` +
      `tagger Takoform Specification Release <release@takoform.invalid> ${timestamp} +0000\n\n` +
      "Takoform Specification 1.1\n\n" +
      `source-commit: ${sourceCommit}\n` +
      `release-commit: ${releaseCommit}\n` +
      `source-snapshot-sha256: ${sourceSnapshotSha256}\n` +
      `source-evidence-sha256: ${sourceEvidenceSha256}\n`,
    "utf8",
  );
  const tagObject = asText(
    command(context, "git", ["hash-object", "-t", "tag", "--stdin"], {
      input: tagObjectBytes,
      label: "hash deterministic Specification tag",
    }),
  ).trim();
  if (!GIT_OBJECT.test(tagObject)) {
    throw new Error("deterministic Specification tag object id is invalid");
  }
  return {
    document,
    evidenceBytes,
    sourceCommit,
    releaseCommit,
    sourceSnapshotSha256,
    sourceEvidenceSha256,
    tagObjectBytes,
    tagObject,
  };
}

function specificationReleaseBody(input) {
  return (
    "Takoform Specification 1.1.\n\n" +
    "This create-only numbered identity is derived only from the exact committed normative source snapshot. Forms, packages, Providers, Hosts, compatibility reports, and adoption evidence are independent. Specification 1.0 was never published, is withdrawn, and is never reused. No Host API /v1.1 or API v2 identity is created.\n\n" +
    `sourceCommit: ${input.sourceCommit}\n` +
    `releaseCommit: ${input.releaseCommit}\n` +
    `sourceSnapshotSha256: ${input.sourceSnapshotSha256}\n` +
    `sourceEvidenceSha256: ${input.sourceEvidenceSha256}`
  );
}

function specificationAssets(input, temporaryRoot) {
  const assetPath = join(temporaryRoot, SOURCE_EVIDENCE_ASSET);
  writeFileSync(assetPath, input.evidenceBytes);
  return new Map([
    [
      SOURCE_EVIDENCE_ASSET,
      {
        name: SOURCE_EVIDENCE_ASSET,
        path: assetPath,
        sha256: input.sourceEvidenceSha256,
      },
    ],
  ]);
}

function materializeSpecificationTag(context, input) {
  if (localTagOID(context, SPECIFICATION_TAG)) {
    throw new Error(
      `local tag ${SPECIFICATION_TAG} already exists; use exact recovery`,
    );
  }
  const object = asText(
    command(context, "git", ["mktag"], {
      input: input.tagObjectBytes,
      label: "materialize deterministic Specification tag object",
    }),
  ).trim();
  if (object !== input.tagObject) {
    throw new Error(
      `Specification tag reconstructed as ${object}, expected ${input.tagObject}`,
    );
  }
  command(context, "git", [
    "update-ref",
    `refs/tags/${SPECIFICATION_TAG}`,
    object,
    "0".repeat(object.length),
  ]);
  return assertExactLocalSpecificationTag(context, input, object).object;
}

function assertExactLocalSpecificationTag(context, input, expectedObject) {
  if (expectedObject !== input.tagObject) {
    throw new Error(
      `expected tag object ${expectedObject} differs from deterministic ${input.tagObject}`,
    );
  }
  const local = localTagOID(context, SPECIFICATION_TAG);
  const localType = local
    ? git(context, "cat-file", "-t", `refs/tags/${SPECIFICATION_TAG}`)
    : "";
  const localCommit = local
    ? git(context, "rev-parse", `refs/tags/${SPECIFICATION_TAG}^{commit}`)
    : "";
  if (
    local !== expectedObject ||
    localType !== "tag" ||
    localCommit !== input.releaseCommit
  ) {
    throw new Error(
      `Specification release requires exact local annotated tag ${expectedObject} -> ${input.releaseCommit}`,
    );
  }
  const raw = command(context, "git", ["cat-file", "tag", expectedObject], {
    encoding: null,
    label: "read exact Specification tag object",
  });
  if (!Buffer.from(raw).equals(input.tagObjectBytes)) {
    throw new Error(
      "Specification annotated tag bytes differ from C2 evidence",
    );
  }
  return { object: local, type: localType, commit: localCommit };
}

function assertExactSpecificationTag(context, input, expectedObject) {
  const local = assertExactLocalSpecificationTag(
    context,
    input,
    expectedObject,
  );
  const remote = assertExactRemoteTag(
    context,
    SPECIFICATION_TAG,
    input.releaseCommit,
    expectedObject,
  );
  return {
    local,
    remote,
  };
}

function assertSpecificationRecoveryFence(
  context,
  { releaseCommit, recoveryCommit, label },
) {
  if (!COMMIT.test(releaseCommit ?? "") || !COMMIT.test(recoveryCommit ?? "")) {
    throw new Error(`${label} requires exact lowercase commits`);
  }
  recoveryGit(context, ["cat-file", "-e", `${releaseCommit}^{commit}`]);
  recoveryGit(context, ["cat-file", "-e", `${recoveryCommit}^{commit}`]);
  const recoveryParents =
    releaseCommit === recoveryCommit
      ? null
      : (() => {
          const ancestry = asText(
            recoveryGit(context, [
              "rev-list",
              "--parents",
              "-n",
              "1",
              recoveryCommit,
            ]),
          )
            .trim()
            .split(" ");
          return ancestry[0] === recoveryCommit &&
            ancestry.every((entry) => COMMIT.test(entry))
            ? ancestry.slice(1)
            : null;
        })();
  const changed =
    releaseCommit === recoveryCommit
      ? []
      : exactDiffPaths(context, releaseCommit, recoveryCommit, {
          recovery: true,
        });
  const problems = validateSpecificationRecoveryPath({
    releaseCommit,
    recoveryCommit,
    recoveryParents,
    changedPaths: changed,
  });
  if (problems.length !== 0) {
    throw new Error(`${label}: ${problems.join("; ")}`);
  }
  return changed;
}

function specificationReadiness(context) {
  command(
    context,
    "bun",
    ["scripts/publication-evidence.mjs", "--assert-specification-1-1"],
    { echo: true, label: "Specification publication evidence" },
  );
  command(
    context,
    "bun",
    ["scripts/specification-release.mjs", "--assert-ready"],
    { echo: true, label: "Specification release readiness" },
  );
}

function specificationMutationFence(
  context,
  { input, expectedObject, currentCommit, releaseId, label },
) {
  const current = specificationOwnerGateAndFence(context, currentCommit);
  assertSpecificationRecoveryFence(context, {
    releaseCommit: input.releaseCommit,
    recoveryCommit: current,
    label,
  });
  assertExactSpecificationTag(context, input, expectedObject);
  if (releaseId === null || releaseId === undefined) {
    assertReleaseAbsent(context, SPECIFICATION_TAG);
  } else {
    assertUniqueReleaseIdentity(context, SPECIFICATION_TAG, releaseId, true);
  }
  return current;
}

function readExactSpecificationRelease(
  context,
  { input, expectedObject, releaseId, assets, temporaryRoot },
) {
  assertExactSpecificationTag(context, input, expectedObject);
  const release = getRelease(context, SPECIFICATION_TAG);
  if (release.id !== releaseId) {
    throw new Error(
      `Specification release id ${release.id} differs from expected ${releaseId}`,
    );
  }
  validateReleaseReadback(release, SPECIFICATION_TAG, assets, {
    expectedReleaseId: releaseId,
    expectedName: SPECIFICATION_TAG,
    expectedBody: specificationReleaseBody(input),
    expectedTargetCommitish: "main",
    expectedAssetsURL: `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets`,
    expectedUploadURL: `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets{?name,label}`,
  });
  downloadAndCompareRelease(context, SPECIFICATION_TAG, assets, temporaryRoot);
  assertUniqueReleaseIdentity(context, SPECIFICATION_TAG, releaseId, false);
  const receipt = releaseFromEvidence(input.document, {
    releaseCommit: input.releaseCommit,
    tag: SPECIFICATION_TAG,
    tagObject: expectedObject,
    release: {
      id: release.id,
      url: release.html_url,
      immutable: release.immutable,
    },
    assetDigests: Object.fromEntries(
      [...assets].map(([name, asset]) => [name, asset.sha256]),
    ),
  });
  return { release, receipt };
}

function specificationPrepare(context, options) {
  const commit = specificationOwnerGateAndFence(
    context,
    options["expected-commit"],
  );
  assertTagAbsent(context, SPECIFICATION_TAG);
  assertReleaseAbsent(context, SPECIFICATION_TAG);
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: SPECIFICATION_SURFACE,
    phase: "prepare",
    tag: SPECIFICATION_TAG,
    commit,
    status: "AWAITING_REVIEW",
    mutation: "none",
    prerequisite: "specification-source-snapshot",
    note: "C1 create-only preflight; prepare performs no identity mutation",
  });
}

function specificationPublish(context, options) {
  const releaseCommit = options["expected-commit"];
  specificationOwnerGateAndFence(context, releaseCommit);
  specificationReadiness(context);
  const input = specificationPublicationInput(context, releaseCommit);
  assertTagAbsent(context, SPECIFICATION_TAG);
  assertReleaseAbsent(context, SPECIFICATION_TAG);
  let materializedObject = "";
  try {
    specificationOwnerGateAndFence(context, releaseCommit);
    assertTagAbsent(context, SPECIFICATION_TAG);
    materializedObject = materializeSpecificationTag(context, input);
    specificationOwnerGateAndFence(context, releaseCommit);
    assertReleaseAbsent(context, SPECIFICATION_TAG);
    pushExactTag(context, SPECIFICATION_TAG, releaseCommit, materializedObject);
    return withTemporaryDirectory(
      "takoform-specification-publish",
      (temporaryRoot) => {
        const assets = specificationAssets(input, temporaryRoot);
        const release = publishReleaseLocally(context, {
          surface: SPECIFICATION_SURFACE,
          tag: SPECIFICATION_TAG,
          assets,
          body: specificationReleaseBody(input),
          temporaryRoot,
          strictIdentity: true,
          preMutationFence: (_stage, releaseId) =>
            specificationMutationFence(context, {
              input,
              expectedObject: materializedObject,
              currentCommit: releaseCommit,
              releaseId,
              label: "Specification publish mutation fence",
            }),
        });
        assertExactSpecificationTag(context, input, materializedObject);
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: SPECIFICATION_SURFACE,
          phase: "publish",
          tag: SPECIFICATION_TAG,
          sourceCommit: input.sourceCommit,
          releaseCommit,
          tagObject: materializedObject,
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "PUBLISHED_AWAITING_C3_RECEIPT",
        });
      },
    );
  } catch (error) {
    reportTagFailure(context, SPECIFICATION_TAG, materializedObject, {
      surface: SPECIFICATION_SURFACE,
      phase: "publish",
    });
    throw error;
  }
}

function specificationRecoveryInput(context, options, label) {
  const releaseCommit = options["expected-release-commit"];
  const recoveryCommit = options["expected-recovery-commit"];
  const current = assertCurrentProtectedMain(context, recoveryCommit);
  assertSpecificationRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: current,
    label,
  });
  specificationReadiness(context);
  const input = specificationPublicationInput(context, releaseCommit);
  if (options["expected-tag-object"] !== input.tagObject) {
    throw new Error(
      "expected Specification tag object differs from deterministic C2 object",
    );
  }
  assertExactSpecificationTag(context, input, options["expected-tag-object"]);
  return { input, releaseCommit, recoveryCommit };
}

function specificationRecoverTagOnly(context, options) {
  const { input, recoveryCommit } = specificationRecoveryInput(
    context,
    options,
    "Specification tag-only recovery fence",
  );
  assertReleaseAbsent(context, SPECIFICATION_TAG);
  return withTemporaryDirectory(
    "takoform-specification-tag-only-recovery",
    (temporaryRoot) => {
      const assets = specificationAssets(input, temporaryRoot);
      const release = publishReleaseLocally(context, {
        surface: SPECIFICATION_SURFACE,
        tag: SPECIFICATION_TAG,
        assets,
        body: specificationReleaseBody(input),
        temporaryRoot,
        strictIdentity: true,
        preMutationFence: (_stage, releaseId) =>
          specificationMutationFence(context, {
            input,
            expectedObject: options["expected-tag-object"],
            currentCommit: recoveryCommit,
            releaseId,
            label: "Specification tag-only recovery mutation fence",
          }),
      });
      assertExactSpecificationTag(
        context,
        input,
        options["expected-tag-object"],
      );
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: SPECIFICATION_SURFACE,
        phase: "recover-tag-only",
        tag: SPECIFICATION_TAG,
        sourceCommit: input.sourceCommit,
        releaseCommit: input.releaseCommit,
        recoveryCommit,
        tagObject: options["expected-tag-object"],
        recoveredFrom: "EXACT_ANNOTATED_TAG_PRESENT_RELEASE_ABSENT",
        releaseId: release.id,
        releaseUrl: release.html_url,
        productionReadback: "EXACT_IMMUTABLE_RELEASE",
        status: "PUBLISHED_AWAITING_C3_RECEIPT",
      });
    },
  );
}

function specificationRecoverDraft(context, options) {
  const { input, recoveryCommit } = specificationRecoveryInput(
    context,
    options,
    "Specification retained-draft recovery fence",
  );
  const releaseId = Number(options["release-id"]);
  return withTemporaryDirectory(
    "takoform-specification-draft-recovery",
    (temporaryRoot) => {
      const assets = specificationAssets(input, temporaryRoot);
      const release = resumeDraftReleaseLocally(context, {
        releaseId,
        surface: SPECIFICATION_SURFACE,
        tag: SPECIFICATION_TAG,
        assets,
        body: specificationReleaseBody(input),
        temporaryRoot,
        preMutationFence: (_stage, retainedReleaseId) =>
          specificationMutationFence(context, {
            input,
            expectedObject: options["expected-tag-object"],
            currentCommit: recoveryCommit,
            releaseId: retainedReleaseId,
            label: "Specification retained-draft recovery mutation fence",
          }),
      });
      assertExactSpecificationTag(
        context,
        input,
        options["expected-tag-object"],
      );
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: SPECIFICATION_SURFACE,
        phase: "recover-draft",
        tag: SPECIFICATION_TAG,
        sourceCommit: input.sourceCommit,
        releaseCommit: input.releaseCommit,
        recoveryCommit,
        tagObject: options["expected-tag-object"],
        recoveredFrom: "EXACT_RETAINED_DRAFT",
        releaseId: release.id,
        releaseUrl: release.html_url,
        productionReadback: "EXACT_IMMUTABLE_RELEASE",
        status: "PUBLISHED_AWAITING_C3_RECEIPT",
      });
    },
  );
}

function specificationLiveReceipt(
  context,
  options,
  { exactMain, expectedCurrentCommit = options["expected-release-commit"] },
) {
  const releaseCommit = options["expected-release-commit"];
  const currentMain = assertCurrentProtectedMain(context, expectedCurrentCommit, {
    exact: exactMain,
  });
  if (!exactMain) {
    assertCommitAncestor(
      context,
      releaseCommit,
      currentMain,
      "Specification read-only verify release/current-main binding",
    );
  }
  const input = specificationPublicationInput(context, releaseCommit);
  if (input.tagObject !== options["expected-tag-object"]) {
    throw new Error(
      "live verify tag object differs from deterministic C2 object",
    );
  }
  return withTemporaryDirectory(
    "takoform-specification-live-verify",
    (temporaryRoot) => {
      const assets = specificationAssets(input, temporaryRoot);
      const readback = readExactSpecificationRelease(context, {
        input,
        expectedObject: options["expected-tag-object"],
        releaseId: Number(options["release-id"]),
        assets,
        temporaryRoot,
      });
      return { ...readback, input, assets, currentMain };
    },
  );
}

function specificationVerify(context, options) {
  const verified = specificationLiveReceipt(context, options, {
    exactMain: false,
  });
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: SPECIFICATION_SURFACE,
    phase: "verify",
    tag: SPECIFICATION_TAG,
    sourceCommit: verified.input.sourceCommit,
    releaseCommit: verified.input.releaseCommit,
    tagObject: options["expected-tag-object"],
    releaseId: verified.release.id,
    releaseUrl: verified.release.html_url,
    assetDigests: Object.fromEntries(
      [...verified.assets].map(([name, asset]) => [name, asset.sha256]),
    ),
    status: "VERIFIED",
  });
}

function specificationRecordReceipt(context, options) {
  const releaseCommit = options["expected-release-commit"];
  const recoveryCommit = options["expected-recovery-commit"];
  const assertReceiptFence = (label) => {
    const current = specificationOwnerGateAndFence(context, recoveryCommit);
    assertSpecificationRecoveryFence(context, {
      releaseCommit,
      recoveryCommit: current,
      label,
    });
    return current;
  };
  assertReceiptFence("Specification record-receipt pre-readback fence");
  const verified = specificationLiveReceipt(context, options, {
    exactMain: true,
    expectedCurrentCommit: recoveryCommit,
  });
  assertReceiptFence("Specification record-receipt pre-write fence");
  const ledger = appendReleaseReceipt(verified.receipt, context.repo);
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: SPECIFICATION_SURFACE,
    phase: "record-receipt",
    tag: SPECIFICATION_TAG,
    sourceCommit: verified.input.sourceCommit,
    releaseCommit: verified.input.releaseCommit,
    recoveryCommit,
    tagObject: options["expected-tag-object"],
    releaseId: verified.release.id,
    releaseUrl: verified.release.html_url,
    receiptCount: ledger.releases.length,
    mutation: "C3_LEDGER_PROJECTIONS_ONLY",
    status: "RECEIPT_WRITTEN_AWAITING_C3_AND_C4_COMMITS",
  });
}

function command(
  context,
  executable,
  args,
  { cwd = context.repo, input, echo = false, label, env, encoding } = {},
) {
  try {
    const output = context.execFile(executable, args, {
      cwd,
      input,
      encoding:
        encoding === undefined
          ? input instanceof Uint8Array
            ? null
            : "utf8"
          : encoding,
      stdio: ["pipe", "pipe", "pipe"],
      maxBuffer: 128 * 1024 * 1024,
      env: env ?? subprocessEnvironment(context, executable),
    });
    if (echo && output) context.stdout.write(output);
    return output;
  } catch (error) {
    if (error.stdout) context.stdout.write(asText(error.stdout));
    if (error.stderr) context.stderr.write(asText(error.stderr));
    const detail = label ?? `${executable} ${args.join(" ")}`;
    const failure = new Error(`${detail} failed`);
    failure.cause = error;
    throw failure;
  }
}

function attemptCommand(context, executable, args, options = {}) {
  try {
    return {
      ok: true,
      output: context.execFile(executable, args, {
        cwd: options.cwd ?? context.repo,
        input: options.input,
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
        maxBuffer: 128 * 1024 * 1024,
        env: options.env ?? subprocessEnvironment(context, executable),
      }),
      stderr: "",
      status: 0,
    };
  } catch (error) {
    return {
      ok: false,
      output: asText(error.stdout),
      stderr: asText(error.stderr),
      status: error.status,
      error,
    };
  }
}

function environmentWithoutGitHubAuthority() {
  const environment = { ...process.env };
  for (const name of Object.keys(environment)) {
    if (name.startsWith("GH_") || name.startsWith("GITHUB_")) {
      delete environment[name];
    }
  }
  return environment;
}

function isolatedGitHubConfigDirectory(context) {
  const directory = context?.githubConfigDirectory;
  if (typeof directory !== "string" || !isAbsolute(directory)) {
    throw new Error(
      "release blocked: GitHub CLI requires an isolated absolute config directory",
    );
  }
  const metadata = lstatSync(directory);
  if (metadata.isSymbolicLink() || !metadata.isDirectory()) {
    throw new Error(
      "release blocked: GitHub CLI config path must be a real directory",
    );
  }
  return realpathSync(directory);
}

function githubCommandEnvironment(context) {
  const environment = createHardenedGitEnvironment(
    environmentWithoutGitHubAuthority(),
  );
  environment.GH_CONFIG_DIR = isolatedGitHubConfigDirectory(context);
  environment.GH_HOST = "github.com";
  environment.GH_NO_UPDATE_NOTIFIER = "1";
  environment.GH_PROMPT_DISABLED = "1";
  environment.GH_TOKEN = process.env.GH_TOKEN;
  return environment;
}

function subprocessEnvironment(context, executable) {
  if (executable === "git") return normalGitEnvironment();
  if (executable === "gh") return githubCommandEnvironment(context);
  const environment = environmentWithoutGitHubAuthority();
  return environment;
}

function normalGitEnvironment() {
  const environment = createHardenedGitEnvironment(
    environmentWithoutGitHubAuthority(),
  );
  delete environment.SSH_ASKPASS;
  delete environment.SSH_ASKPASS_REQUIRE;
  delete environment.GCM_INTERACTIVE;
  return environment;
}

function githubUploadEnvironment(context) {
  const environment = githubCommandEnvironment(context);
  environment.GH_TOKEN = process.env.GH_TOKEN;
  delete environment.GITHUB_TOKEN;
  delete environment.GH_ENTERPRISE_TOKEN;
  delete environment.GITHUB_ENTERPRISE_TOKEN;
  return environment;
}

function gitPushEnvironment() {
  const environment = normalGitEnvironment();
  environment.GIT_TERMINAL_PROMPT = "0";
  environment.GIT_ASKPASS = "/bin/false";
  environment.SSH_ASKPASS = "/bin/false";
  environment.GIT_CONFIG_COUNT = "5";
  environment.GIT_CONFIG_KEY_0 = "credential.helper";
  environment.GIT_CONFIG_VALUE_0 = "";
  environment.GIT_CONFIG_KEY_1 = "credential.interactive";
  environment.GIT_CONFIG_VALUE_1 = "never";
  environment.GIT_CONFIG_KEY_2 = "http.https://github.com/.extraHeader";
  environment.GIT_CONFIG_VALUE_2 = "";
  environment.GIT_CONFIG_KEY_3 = "http.https://github.com/.extraHeader";
  environment.GIT_CONFIG_VALUE_3 = `AUTHORIZATION: basic ${Buffer.from(`x-access-token:${process.env.GH_TOKEN}`, "utf8").toString("base64")}`;
  environment.GIT_CONFIG_KEY_4 = "core.hooksPath";
  environment.GIT_CONFIG_VALUE_4 = "/dev/null";
  return environment;
}

function recoveryReadOnlyGitEnvironment() {
  return normalGitEnvironment();
}

function recoveryGit(context, args, options = {}) {
  return command(context, "git", args, {
    ...options,
    env: recoveryReadOnlyGitEnvironment(),
  });
}

function trustedGpgExecutable() {
  const candidates = [
    "/usr/bin/gpg",
    "/usr/local/bin/gpg",
    "/opt/homebrew/bin/gpg",
    ...(process.env.PATH ?? "")
      .split(delimiter)
      .filter((directory) => isAbsolute(directory))
      .map((directory) => resolve(directory, "gpg")),
  ];
  for (const candidate of new Set(candidates)) {
    try {
      accessSync(candidate, fsConstants.X_OK);
      const executable = realpathSync(candidate);
      if (isAbsolute(executable) && lstatSync(executable).isFile()) {
        return executable;
      }
    } catch {
      // Continue to the next operator-installed absolute candidate.
    }
  }
  throw new Error("release blocked: no absolute trusted gpg executable");
}

function isolatedGpgEnvironment(gpgHome) {
  const environment = recoveryReadOnlyGitEnvironment();
  delete environment.GNUPGHOME;
  delete environment.GPG_AGENT_INFO;
  delete environment.GPG_TTY;
  environment.GNUPGHOME = gpgHome;
  environment.LC_ALL = "C";
  return environment;
}

function asText(value) {
  if (!value) return "";
  return Buffer.isBuffer(value) ? value.toString("utf8") : String(value);
}

function git(context, ...args) {
  return command(context, "git", args).trim();
}

function progress(context, text) {
  context.stdout.write(`\n==> ${text}\n`);
}

function emit(context, result) {
  context.stdout.write(`\n${JSON.stringify(result, null, 2)}\n`);
  return result;
}

function ownerGateToolchain(repo) {
  const descriptorPath = join(repo, "release/version.json");
  let descriptor;
  try {
    descriptor = JSON.parse(readFileSync(descriptorPath, "utf8"));
  } catch (error) {
    throw new Error(
      `owner gate toolchain descriptor is not valid JSON: ${descriptorPath} (${error instanceof Error ? error.message : String(error)})`,
    );
  }
  if (
    !Array.isArray(descriptor?.cliMatrix) ||
    descriptor.cliMatrix.length !== 2
  ) {
    throw new Error(
      "owner gate toolchain descriptor must contain exactly OpenTofu and Terraform entries",
    );
  }
  const byProduct = new Map();
  for (const entry of descriptor.cliMatrix) {
    if (
      entry === null ||
      typeof entry !== "object" ||
      (entry.product !== "OpenTofu" && entry.product !== "Terraform") ||
      typeof entry.version !== "string" ||
      entry.version.length === 0 ||
      byProduct.has(entry.product)
    ) {
      throw new Error(
        "owner gate toolchain descriptor must contain one exact OpenTofu and Terraform version",
      );
    }
    byProduct.set(entry.product, entry.version);
  }
  if (
    !byProduct.has("OpenTofu") ||
    !byProduct.has("Terraform") ||
    typeof descriptor.goVersion !== "string" ||
    !/^go[0-9]+\.[0-9]+\.[0-9]+(?:[._-][0-9A-Za-z.-]+)?$/u.test(
      descriptor.goVersion,
    )
  ) {
    throw new Error(
      "owner gate toolchain descriptor is missing an exact Go version",
    );
  }
  return {
    go: descriptor.goVersion,
    tofu: { product: "OpenTofu", version: byProduct.get("OpenTofu") },
    terraform: {
      product: "Terraform",
      version: byProduct.get("Terraform"),
    },
  };
}

function ownerGateToolVersion(context, executable, expected, environment) {
  let json;
  try {
    json = JSON.parse(
      asText(
        command(context, executable, ["version", "-json"], {
          env: environment,
          label: `${expected.product} version -json`,
        }),
      ),
    );
  } catch (error) {
    throw new Error(
      `${expected.product} version -json output is invalid: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  if (json?.terraform_version !== expected.version) {
    throw new Error(
      `${expected.product} version drift: require ${expected.version}, observed ${json?.terraform_version ?? "missing"}`,
    );
  }
  const plain = asText(
    command(context, executable, ["version"], {
      env: environment,
      label: `${expected.product} version`,
    }),
  )
    .trim()
    .split(/\r?\n/u, 1)[0];
  if (plain !== `${expected.product} v${expected.version}`) {
    throw new Error(
      `${expected.product} product/version drift: require ${expected.product} v${expected.version}, observed ${plain || "missing"}`,
    );
  }
}

const GO_ENV_COMPONENT = /^[A-Za-z0-9][A-Za-z0-9._-]*$/u;

function assertManagedGateStateClosure(state) {
  assertManagedGateState(state);
  const expected = ["go-build", "go-mod", "go-path", "t"].sort();
  const observed = readdirSync(state.root).sort();
  if (JSON.stringify(observed) !== JSON.stringify(expected)) {
    throw new Error(
      `owner gate mutable state topology changed (expected ${expected.join(", ")}, observed ${observed.join(", ")})`,
    );
  }
  return state;
}

function assertManagedGateEnvironment(environment, managedState) {
  if (Object.hasOwn(environment, "GOCACHEPROG")) {
    throw new Error("owner gate GOCACHEPROG must be scrubbed");
  }
  if (managedState === undefined) return environment;
  for (const [environmentName, stateName] of [
    ["GOCACHE", "gocache"],
    ["GOMODCACHE", "gomodcache"],
    ["GOPATH", "gopath"],
    ["TMPDIR", "tmpdir"],
  ]) {
    if (environment[environmentName] !== managedState[stateName]) {
      throw new Error(
        `owner gate ${environmentName} must use managed mutable state`,
      );
    }
  }
  return environment;
}

function readManagedGoEnvironment(context, snapshot, environment) {
  const raw = asText(
    command(
      context,
      snapshot.go.go.path,
      ["env", "GOROOT", "GOTOOLDIR", "GOOS", "GOARCH"],
      { env: environment, label: "Go toolchain environment" },
    ),
  ).replace(/\r\n/gu, "\n");
  if (!raw.endsWith("\n")) {
    throw new Error(
      "Go toolchain environment output must contain exactly GOROOT, GOTOOLDIR, GOOS, and GOARCH",
    );
  }
  const values = raw.slice(0, -1).split("\n");
  if (values.length !== 4 || values.some((value) => value.length === 0)) {
    throw new Error(
      "Go toolchain environment output must contain exactly GOROOT, GOTOOLDIR, GOOS, and GOARCH",
    );
  }
  const [goroot, gotooldir, goos, goarch] = values;
  if (goroot !== snapshot.go.root) {
    throw new Error(
      `Go toolchain root drift: require ${snapshot.go.root}, observed ${goroot || "missing"}`,
    );
  }
  if (!GO_ENV_COMPONENT.test(goos) || !GO_ENV_COMPONENT.test(goarch)) {
    throw new Error(
      `Go toolchain GOTOOLDIR platform drift: require safe GOOS/GOARCH, observed ${goos}/${goarch}`,
    );
  }
  const managedToolRoot = resolve(snapshot.go.root, "pkg", "tool");
  const expectedToolDirectory = resolve(managedToolRoot, `${goos}_${goarch}`);
  const expectedRelative = relative(managedToolRoot, expectedToolDirectory);
  if (
    expectedRelative.length === 0 ||
    expectedRelative === ".." ||
    expectedRelative.startsWith(`..${sep}`) ||
    isAbsolute(expectedRelative) ||
    expectedRelative.includes(sep)
  ) {
    throw new Error(
      `Go toolchain GOTOOLDIR escaped managed pkg/tool: ${expectedToolDirectory}`,
    );
  }
  if (!isAbsolute(gotooldir)) {
    throw new Error(
      `Go toolchain GOTOOLDIR escaped managed pkg/tool: ${gotooldir || "missing"}`,
    );
  }
  const observedRelative = relative(managedToolRoot, gotooldir);
  if (
    observedRelative === ".." ||
    observedRelative.startsWith(`..${sep}`) ||
    isAbsolute(observedRelative)
  ) {
    throw new Error(
      `Go toolchain GOTOOLDIR escaped managed pkg/tool: ${gotooldir}`,
    );
  }
  if (gotooldir !== expectedToolDirectory) {
    throw new Error(
      `Go toolchain GOTOOLDIR drift: require ${expectedToolDirectory}, observed ${gotooldir || "missing"}`,
    );
  }
  let toolDirectory;
  try {
    toolDirectory = lstatSync(expectedToolDirectory);
  } catch (error) {
    throw new Error(
      `Go toolchain GOTOOLDIR is not a real safe directory: ${expectedToolDirectory} (${error instanceof Error ? error.message : String(error)})`,
    );
  }
  if (
    toolDirectory.isSymbolicLink() ||
    !toolDirectory.isDirectory() ||
    (toolDirectory.mode & 0o7777) !== 0o500 ||
    realpathSync(expectedToolDirectory) !== expectedToolDirectory
  ) {
    throw new Error(
      `Go toolchain GOTOOLDIR is not a real safe directory: ${expectedToolDirectory}`,
    );
  }
  const logicalToolDirectory = `pkg/tool/${goos}_${goarch}`;
  const manifestEntry = snapshot.go.manifest.entries.find(
    (entry) => entry.path === logicalToolDirectory,
  );
  if (manifestEntry?.type !== "directory" || manifestEntry.mode !== 0o500) {
    throw new Error(
      `Go toolchain GOTOOLDIR is not a real safe directory: ${expectedToolDirectory}`,
    );
  }
  return { goroot, gotooldir, goos, goarch };
}

function verifyOwnerGateToolchain(
  context,
  snapshot,
  expected,
  environment,
  managedState,
) {
  assertManagedToolSnapshot(snapshot);
  if (managedState !== undefined) assertManagedGateStateClosure(managedState);
  assertManagedGateEnvironment(environment, managedState);
  readManagedGoEnvironment(context, snapshot, environment);
  const goVersion = command(context, snapshot.go.go.path, ["version"], {
    env: environment,
    label: "Go version",
  })
    .trim()
    .split(/\r?\n/u, 1)[0];
  const exactGoVersion = new RegExp(
    `^go version ${expected.go.replace(/[.*+?^${}()|[\]\\]/gu, "\\$&")} [^/\\s]+/[^/\\s]+$`,
    "u",
  );
  if (!exactGoVersion.test(goVersion)) {
    throw new Error(
      `Go version drift: require ${expected.go}, observed ${goVersion || "missing"}`,
    );
  }
  const gofmtBuild = asText(
    command(
      context,
      snapshot.go.go.path,
      ["version", "-m", snapshot.go.gofmt.path],
      { env: environment, label: "gofmt build identity" },
    ),
  )
    .replace(/\r\n/gu, "\n")
    .replace(/\n$/u, "")
    .split("\n");
  if (
    gofmtBuild[0] !== `${snapshot.go.gofmt.path}: ${expected.go}` ||
    gofmtBuild[1] !== "\tpath\tcmd/gofmt"
  ) {
    throw new Error(
      `gofmt build identity drift: require ${expected.go} cmd/gofmt`,
    );
  }
  ownerGateToolVersion(
    context,
    snapshot.tools.tofu.path,
    expected.tofu,
    environment,
  );
  ownerGateToolVersion(
    context,
    snapshot.tools.terraform.path,
    expected.terraform,
    environment,
  );
  assertManagedToolSnapshot(snapshot);
  if (managedState !== undefined) assertManagedGateStateClosure(managedState);
  assertManagedGateEnvironment(environment, managedState);
}

function warmOwnerGateGoCaches(context, snapshot, environment, managedState) {
  assertManagedToolSnapshot(snapshot);
  if (managedState !== undefined) assertManagedGateStateClosure(managedState);
  assertManagedGateEnvironment(environment, managedState);
  const commands = [
    [["mod", "download"], context.repo, "owner gate root module download"],
    [
      ["-C", "cmd/provider-release", "mod", "download"],
      context.repo,
      "owner gate release module download",
    ],
    [
      ["test", "-run", "^$", "./..."],
      context.repo,
      "owner gate root compile cache",
    ],
    [
      ["-C", "cmd/provider-release", "test", "-run", "^$", "./..."],
      context.repo,
      "owner gate release compile cache",
    ],
  ];
  for (const [args, cwd, label] of commands) {
    command(context, snapshot.go.go.path, args, {
      cwd,
      env: environment,
      label,
    });
    assertManagedToolSnapshot(snapshot);
    if (managedState !== undefined) assertManagedGateStateClosure(managedState);
    assertManagedGateEnvironment(environment, managedState);
  }
  assertManagedToolSnapshot(snapshot);
  if (managedState !== undefined) assertManagedGateStateClosure(managedState);
  assertManagedGateEnvironment(environment, managedState);
}

function runOwnerCheck(context) {
  // Publication proves bytes; admission proves a conforming host signed for
  // them. They are separate phases, and the admission closure can only be
  // green after publication has already happened. Gating publication on it
  // would make the first release of any new Form version unreachable, so the
  // owner gate stops at publication authority and admission keeps its own gate.
  return withTemporaryDirectory("t", (managedHome) => {
    try {
      const nominationEnvironment = environmentWithoutGitHubAuthority();
      const expected = ownerGateToolchain(context.repo);
      const snapshot = createManagedToolSnapshot({
        environment: nominationEnvironment,
        managedHome,
      });
      const managedState = createManagedGateState(managedHome);
      assertManagedGateStateClosure(managedState);
      const environment = createHardenedGateEnvironment(
        nominationEnvironment,
        process.execPath,
        managedHome,
        {
          managedToolBin: snapshot.toolBin,
          goBin: snapshot.go.bin,
          goRoot: snapshot.go.root,
        },
      );
      verifyOwnerGateToolchain(
        context,
        snapshot,
        expected,
        environment,
        managedState,
      );
      warmOwnerGateGoCaches(context, snapshot, environment, managedState);
      progress(context, "bun run check:release-owner-gate");
      const result = command(
        context,
        "bun",
        ["run", "check:release-owner-gate"],
        {
          echo: true,
          env: environment,
          label: "owner gate",
        },
      );
      assertManagedToolSnapshot(snapshot);
      assertManagedGateStateClosure(managedState);
      assertManagedGateEnvironment(environment, managedState);
      return result;
    } finally {
      prepareManagedHomeForRemoval(managedHome);
    }
  });
}

function ownerGateAndFence(context, expectedCommit, options) {
  verifyLocalReleaseToolchain(context);
  assertCurrentProtectedMain(context, expectedCommit, options);
  runOwnerCheck(context);
  return assertCurrentProtectedMain(context, expectedCommit, options);
}

function specificationOwnerGateAndFence(context, expectedCommit, options) {
  verifySpecificationReleaseToolchain(context);
  assertCurrentProtectedMain(context, expectedCommit, options);
  runOwnerCheck(context);
  const current = assertCurrentProtectedMain(context, expectedCommit, options);
  // GitHub only makes releases immutable when the repository setting was
  // enabled before publication. Recheck it at every Specification mutation
  // fence so a tag or draft cannot be created under mutable-release policy.
  assertReleaseImmutabilityEnabled(context);
  return current;
}

function verifySpecificationReleaseToolchain(context) {
  if (context.specificationReleaseToolchainVerified) return;
  const ghVersion = command(context, "gh", ["--version"]);
  if (!ghVersion.startsWith(`gh version ${PINNED_GH_VERSION} `)) {
    throw new Error(
      `Specification release toolchain drift: require gh ${PINNED_GH_VERSION}`,
    );
  }
  context.specificationReleaseToolchainVerified = true;
}

function verifyLocalReleaseToolchain(context) {
  if (context.releaseToolchainVerified) return;
  const ghVersion = command(context, "gh", ["--version"]);
  const cosignVersion = command(context, "cosign", ["version"]);
  if (
    !ghVersion.startsWith(`gh version ${PINNED_GH_VERSION} `) ||
    !new RegExp(
      `^GitVersion:\\s+${PINNED_COSIGN_VERSION.replaceAll(".", "\\.")}$`,
      "mu",
    ).test(cosignVersion)
  ) {
    throw new Error(
      `release toolchain drift: require gh ${PINNED_GH_VERSION} and cosign ${PINNED_COSIGN_VERSION}`,
    );
  }
  context.releaseToolchainVerified = true;
}

function assertCurrentProtectedMain(
  context,
  expectedCommit,
  { exact = true } = {},
) {
  const localConfiguration = command(
    context,
    "git",
    ["config", "--local", "-z", "--list"],
    { label: "read release repository Git configuration" },
  );
  try {
    assertSafeRepositoryGitConfiguration(localConfiguration, SOURCE_REPOSITORY);
  } catch (error) {
    throw new Error(
      `release blocked: repository Git configuration is not publication-safe: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const commonDirectory = git(
    context,
    "rev-parse",
    "--path-format=absolute",
    "--git-common-dir",
  );
  for (const relativePath of ["objects/info/alternates", "info/grafts"]) {
    if (existsSync(join(commonDirectory, relativePath))) {
      throw new Error(
        `release blocked: repository Git object authority uses forbidden ${relativePath}`,
      );
    }
  }
  const dirty = git(
    context,
    "status",
    "--porcelain=v1",
    "--untracked-files=all",
  );
  if (dirty !== "") {
    throw new Error(
      `release blocked: worktree is dirty\n${dirty.split("\n").slice(0, 30).join("\n")}`,
    );
  }
  if (git(context, "rev-parse", "--is-shallow-repository") !== "false") {
    throw new Error(
      "release blocked: shallow repositories cannot prove ancestry",
    );
  }
  const origin = git(context, "remote", "get-url", "origin");
  if (!isCanonicalOrigin(origin)) {
    throw new Error(
      `release blocked: origin is not the canonical ${SOURCE_REPOSITORY}: ${origin}`,
    );
  }
  if (git(context, "symbolic-ref", "--quiet", "--short", "HEAD") !== "main") {
    throw new Error("release blocked: checkout must be attached to main");
  }
  progress(context, "fetch canonical protected origin/main");
  command(context, "git", [
    "fetch",
    "--no-tags",
    "--prune",
    "origin",
    "+refs/heads/main:refs/remotes/origin/main",
  ]);
  const head = git(context, "rev-parse", "HEAD");
  const remoteMain = git(context, "rev-parse", "refs/remotes/origin/main");
  if (head !== remoteMain) {
    throw new Error(
      `release blocked: HEAD ${head} is not fresh origin/main ${remoteMain}`,
    );
  }
  if (expectedCommit) {
    if (exact && expectedCommit !== head) {
      throw new Error(
        `release blocked: expected commit ${expectedCommit} is not current protected main ${head}`,
      );
    }
    command(context, "git", ["cat-file", "-e", `${expectedCommit}^{commit}`]);
    if (!exact) {
      command(context, "git", [
        "merge-base",
        "--is-ancestor",
        expectedCommit,
        head,
      ]);
    }
  }
  return head;
}

function assertCommitAncestor(context, ancestor, descendant, label) {
  if (!COMMIT.test(ancestor ?? "") || !COMMIT.test(descendant ?? "")) {
    throw new Error(`${label} requires exact lowercase commits`);
  }
  command(context, "git", ["cat-file", "-e", `${ancestor}^{commit}`]);
  command(
    context,
    "git",
    ["merge-base", "--is-ancestor", ancestor, descendant],
    { label: `${label}: ${ancestor} must be an ancestor of ${descendant}` },
  );
}

function assertRecoveryCommitAncestor(context, ancestor, descendant, label) {
  if (!COMMIT.test(ancestor ?? "") || !COMMIT.test(descendant ?? "")) {
    throw new Error(`${label} requires exact lowercase commits`);
  }
  recoveryGit(context, ["cat-file", "-e", `${ancestor}^{commit}`]);
  recoveryGit(context, ["merge-base", "--is-ancestor", ancestor, descendant], {
    label: `${label}: ${ancestor} must be an ancestor of ${descendant}`,
  });
}

function assertFormReleaseAuthorityFence(
  context,
  { sourceCommit, toolingCommit, currentMain, label },
) {
  assertCommitAncestor(
    context,
    sourceCommit,
    toolingCommit,
    `${label} source/tooling ancestry`,
  );
  assertCommitAncestor(
    context,
    toolingCommit,
    currentMain,
    `${label} tooling/current-main ancestry`,
  );
  const authorityPaths = [
    ...FORM_RELEASE_AUTHORITY_PATHS,
    "forms/revocations",
  ].sort();
  command(
    context,
    "git",
    ["diff", "--quiet", toolingCommit, currentMain, "--", ...authorityPaths],
    {
      label: `${label}: release authority paths changed after reviewed tooling commit`,
    },
  );
}

function assertProviderRecoveryFence(
  context,
  { releaseCommit, recoveryCommit, label },
) {
  assertRecoveryCommitAncestor(
    context,
    releaseCommit,
    recoveryCommit,
    `${label} release/recovery ancestry`,
  );
  const rawChanged = recoveryGit(context, [
    "-c",
    "diff.renames=false",
    "diff",
    "--no-renames",
    "--no-ext-diff",
    "--no-textconv",
    "--name-only",
    "-z",
    "--diff-filter=ACDMRTUXB",
    releaseCommit,
    recoveryCommit,
    "--",
  ]);
  if (
    typeof rawChanged !== "string" ||
    (rawChanged !== "" && !rawChanged.endsWith("\0"))
  ) {
    throw new Error(`${label}: recovery diff path output is ambiguous`);
  }
  const changed = rawChanged === "" ? [] : rawChanged.slice(0, -1).split("\0");
  if (
    changed.some((path) => path === "" || /[\r\n]/u.test(path)) ||
    new Set(changed).size !== changed.length
  ) {
    throw new Error(`${label}: recovery diff contains an ambiguous path`);
  }
  if (
    changed.length === 0 ||
    !changed.includes("scripts/release-deploy.mjs") ||
    changed.some((path) => !PROVIDER_RECOVERY_ALLOWED_PATHS.includes(path))
  ) {
    throw new Error(
      `${label}: release/recovery diff must contain only the exact reviewed recovery implementation, tests, and documentation; observed ${changed.join(", ") || "no changes"}`,
    );
  }
  return changed;
}

function isCanonicalOrigin(origin) {
  return [SOURCE_REPOSITORY, SOURCE_REPOSITORY.slice(0, -4)].includes(origin);
}

function readJSON(path, label) {
  let value;
  try {
    value = JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    throw new Error(`${label} is not valid JSON: ${error.message}`);
  }
  return value;
}

function recursivelySorted(value) {
  if (Array.isArray(value)) return value.map(recursivelySorted);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, recursivelySorted(value[key])]),
    );
  }
  return value;
}

function requireCanonicalSortedJSON(raw, value, label) {
  if (!raw.equals(Buffer.from(JSON.stringify(recursivelySorted(value))))) {
    throw new Error(`${label} must be canonical recursively key-sorted JSON`);
  }
}

function parseCandidateMetadata(
  raw,
  label,
  { profile = "compact-optional-lf" } = {},
) {
  if (profile !== "compact-optional-lf" && profile !== "pretty-required-lf") {
    throw new Error(`${label} uses unsupported metadata profile: ${profile}`);
  }
  let value;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`${label} is not valid JSON: ${error.message}`);
  }
  const canonical = Buffer.from(JSON.stringify(recursivelySorted(value)));
  const canonicalWithNewline = Buffer.concat([canonical, Buffer.from("\n")]);
  const prettyWithNewline = Buffer.from(
    `${JSON.stringify(recursivelySorted(value), null, 2)}\n`,
  );
  const accepted =
    profile === "compact-optional-lf"
      ? raw.equals(canonical) || raw.equals(canonicalWithNewline)
      : raw.equals(prettyWithNewline);
  if (!accepted) {
    throw new Error(
      `${label} must be exact recursively key-sorted ${profile === "pretty-required-lf" ? "two-space pretty JSON with one trailing LF" : "compact canonical JSON"}`,
    );
  }
  return value;
}

function parseCanonicalCandidateMetadata(raw, label) {
  return parseCandidateMetadata(raw, label);
}

function parsePrettyCandidateMetadata(raw, label) {
  return parseCandidateMetadata(raw, label, {
    profile: "pretty-required-lf",
  });
}

function providerFormRefKey(formRef) {
  return JSON.stringify([
    formRef.apiVersion,
    formRef.kind,
    formRef.definitionVersion,
    formRef.schemaDigest,
  ]);
}

function loadCurrentProviderCandidateProjection(repo) {
  const index = readJSON(
    join(repo, PROVIDER_CURRENT_FAMILY_INDEX),
    "current provider family index",
  );
  requireExactKeys(
    index,
    ["bindingCandidateSet", "families", "format", "interfaceCandidateSet"],
    "current provider family index",
  );
  if (
    index.format !== "takoform.current-family-index@v1" ||
    !Array.isArray(index.families) ||
    index.families.length === 0
  ) {
    throw new Error("current provider family index has an invalid envelope");
  }
  const families = [];
  const candidates = new Map();
  for (const [entryIndex, entry] of index.families.entries()) {
    requireExactKeys(
      entry,
      ["candidateSet", "formCount", "group", "sha256"],
      `current provider family index entry ${entryIndex}`,
    );
    const expectedPath = `forms/candidates/${entry.group}/candidate-set.json`;
    if (
      typeof entry.group !== "string" ||
      entry.group === "" ||
      families.includes(entry.group) ||
      entry.candidateSet !== expectedPath ||
      !/^[0-9a-f]{64}$/u.test(entry.sha256 ?? "") ||
      !Number.isSafeInteger(entry.formCount) ||
      entry.formCount <= 0
    ) {
      throw new Error(
        `current provider family index entry ${entryIndex} is invalid`,
      );
    }
    families.push(entry.group);
    const candidatePath = join(repo, entry.candidateSet);
    if (fileDigest(candidatePath) !== `sha256:${entry.sha256}`) {
      throw new Error(
        `current provider candidate set ${entry.group} digest drifted`,
      );
    }
    const candidate = readJSON(
      candidatePath,
      `current provider candidate set ${entry.group}`,
    );
    requireExactKeys(
      candidate,
      [
        "authoringPolicy",
        "authoringSource",
        "family",
        "formMaturity",
        "format",
        "forms",
        "packageApiVersion",
        "publicationStatus",
      ],
      `current provider candidate set ${entry.group}`,
    );
    if (
      candidate.format !== "takoform.form-family-candidates@v1" ||
      candidate.family !== entry.group ||
      candidate.formMaturity !== "experimental" ||
      candidate.packageApiVersion !== "packages.forms.takoform.com/v1alpha5" ||
      candidate.publicationStatus !== "unpublished" ||
      !Array.isArray(candidate.forms) ||
      candidate.forms.length !== entry.formCount
    ) {
      throw new Error(
        `current provider candidate set ${entry.group} is not the exact unpublished Experimental family set`,
      );
    }
    for (const [formIndex, form] of candidate.forms.entries()) {
      requireExactKeys(
        form.formRef,
        ["apiVersion", "definitionVersion", "kind", "schemaDigest"],
        `${entry.group} candidate FormRef ${formIndex}`,
      );
      if (
        form.formRef.apiVersion !== entry.group ||
        form.formRef.kind === "ObjectBucket" ||
        !/^[A-Z][A-Za-z0-9]{0,63}$/u.test(form.formRef.kind ?? "") ||
        !PROVIDER_TAG.test(`v${form.formRef.definitionVersion ?? ""}`) ||
        !SHA256.test(form.formRef.schemaDigest ?? "") ||
        !SHA256.test(form.packageDigest ?? "")
      ) {
        throw new Error(
          `${entry.group} candidate Form ${formIndex} is invalid`,
        );
      }
      const key = providerFormRefKey(form.formRef);
      if (candidates.has(key)) {
        throw new Error(`current provider candidate index duplicates ${key}`);
      }
      candidates.set(key, {
        formRef: form.formRef,
        packageDigest: form.packageDigest,
      });
    }
  }
  if (JSON.stringify(families) !== JSON.stringify([...families].sort())) {
    throw new Error(
      "current provider family index is not in canonical lexical order",
    );
  }
  return { families, candidates };
}

/**
 * loadProviderRosterProjection dispatches on the descriptor version so a
 * retained major keeps exactly the roster rule it was published under.
 *
 * 3.0.0 is the retained 31-Form aggregate: its roster comes from the local
 * eight-family candidate index and ObjectBucket stays banned, because that
 * kind was deliberately absent from the Provider 3 surface.
 *
 * 4.0.0 is the publisher-selected release: its roster is derived from
 * internal/provider/artifacts/publisher/closure.json plus projection.json
 * through the single derivation in scripts/provider4-candidate.mjs, so a
 * publisher-set change cannot silently pass and there is no second literal
 * copy of the family list or the count. ObjectBucket is part of that selected
 * set and is therefore allowed on this path only.
 */
function loadProviderRosterProjection(repo, descriptor) {
  if (descriptor.version === PROVIDER_RETAINED_AGGREGATE_VERSION) {
    const { families, candidates } = loadCurrentProviderCandidateProjection(repo);
    return {
      families,
      candidates,
      banObjectBucket: true,
      rosterLabel: "current-family",
    };
  }
  const { ledgerEntry } = generateProvider4Identities(repo);
  if (ledgerEntry.providerVersion !== descriptor.version) {
    throw new Error(
      `provider ${descriptor.version} has no publisher-selected roster derivation`,
    );
  }
  const candidates = new Map(
    ledgerEntry.forms.map((form) => [
      providerFormRefKey(form.formRef),
      { formRef: form.formRef, packageDigest: form.packageDigest },
    ]),
  );
  if (candidates.size !== ledgerEntry.forms.length) {
    throw new Error(
      `provider ${descriptor.version} publisher-selected roster repeats a FormRef`,
    );
  }
  return {
    families: ledgerEntry.families,
    candidates,
    banObjectBucket: false,
    rosterLabel: "publisher-selected",
  };
}

export function validateProviderIdentityLedger(repo, descriptor) {
  const ledger = readJSON(
    join(repo, PROVIDER_IDENTITY_LEDGER),
    "provider Form identity ledger",
  );
  requireExactKeys(
    ledger,
    ["format", "releases"],
    "provider Form identity ledger",
  );
  if (
    ledger.format !== "takoform.provider-form-identities@v1" ||
    !Array.isArray(ledger.releases) ||
    ledger.releases.length === 0
  ) {
    throw new Error("provider Form identity ledger has an invalid envelope");
  }
  const providerVersions = new Set();
  let current;
  for (const [releaseIndex, release] of ledger.releases.entries()) {
    const releaseKeys = Object.keys(release ?? {}).sort();
    const hasSingleFamily =
      JSON.stringify(releaseKeys) ===
        JSON.stringify(
          [
            "family",
            "formMaturity",
            "forms",
            "portableApiVersion",
            "providerVersion",
          ].sort(),
        ) && typeof release?.family === "string";
    const hasFamilySet =
      JSON.stringify(releaseKeys) ===
        JSON.stringify(
          [
            "families",
            "formMaturity",
            "forms",
            "portableApiVersion",
            "providerVersion",
          ].sort(),
        ) &&
      Array.isArray(release?.families) &&
      release.families.length > 0 &&
      JSON.stringify(release.families) ===
        JSON.stringify([...release.families].sort()) &&
      new Set(release.families).size === release.families.length;
    if (
      typeof release.providerVersion !== "string" ||
      !PROVIDER_TAG.test(`v${release.providerVersion}`) ||
      typeof release.portableApiVersion !== "string" ||
      (!hasSingleFamily && !hasFamilySet) ||
      release.formMaturity !== "experimental" ||
      !Array.isArray(release.forms) ||
      release.forms.length === 0
    ) {
      throw new Error(
        `provider Form identity ledger release ${releaseIndex} is invalid`,
      );
    }
    if (providerVersions.has(release.providerVersion)) {
      throw new Error(
        `provider Form identity ledger duplicates ${release.providerVersion}`,
      );
    }
    providerVersions.add(release.providerVersion);
    if (release.providerVersion === descriptor.version) current = release;
    const allowedFamilies = new Set(
      hasSingleFamily ? [release.family] : release.families,
    );
    if (
      release.providerVersion === "2.1.1" &&
      sha256(Buffer.from(JSON.stringify(recursivelySorted(release)))) !==
        PROVIDER_V211_LEDGER_DIGEST
    ) {
      throw new Error("immutable provider 2.1.1 identity ledger entry changed");
    }
    if (
      release.providerVersion === PROVIDER_RETAINED_AGGREGATE_VERSION &&
      sha256(Buffer.from(JSON.stringify(recursivelySorted(release)))) !==
        PROVIDER_V300_LEDGER_DIGEST
    ) {
      throw new Error("immutable provider 3.0.0 identity ledger entry changed");
    }
    // FormRef uniqueness is per release: a later major legitimately carries
    // forward the exact FormRefs an earlier major published, and the identity
    // that must stay unique inside one release is the FormRef-to-resource-type
    // mapping, not the FormRef across the whole append-only lineage.
    const formRefs = new Set();
    for (const [formIndex, form] of release.forms.entries()) {
      requireExactKeys(
        form,
        ["formRef", "packageDigest", "resourceType"],
        `${release.providerVersion} provider-embedded Form identity ${formIndex}`,
      );
      requireExactKeys(
        form.formRef,
        ["apiVersion", "definitionVersion", "kind", "schemaDigest"],
        `${release.providerVersion} provider-embedded FormRef ${formIndex}`,
      );
      if (
        typeof form.resourceType !== "string" ||
        !/^takoform_[a-z0-9_]+$/u.test(form.resourceType) ||
        !SHA256.test(form.packageDigest ?? "") ||
        typeof form.formRef.apiVersion !== "string" ||
        !allowedFamilies.has(form.formRef.apiVersion) ||
        !/^[A-Z][A-Za-z0-9]{0,63}$/u.test(form.formRef.kind ?? "") ||
        !PROVIDER_TAG.test(`v${form.formRef.definitionVersion ?? ""}`) ||
        !SHA256.test(form.formRef.schemaDigest ?? "")
      ) {
        throw new Error(
          `${release.providerVersion}: invalid provider-embedded Form identity`,
        );
      }
      const refKey = providerFormRefKey(form.formRef);
      if (formRefs.has(refKey)) {
        throw new Error(
          `${release.providerVersion}: duplicate provider-embedded FormRef`,
        );
      }
      formRefs.add(refKey);
    }
  }
  const projection = loadProviderRosterProjection(repo, descriptor);
  if (
    current === undefined ||
    current.portableApiVersion !== descriptor.versioning?.portableApiVersion ||
    current.portableApiVersion !== PROVIDER_HOST_API ||
    current.family !== undefined ||
    JSON.stringify(current.families) !== JSON.stringify(projection.families) ||
    current.formMaturity !== "experimental" ||
    current.forms.length !== projection.candidates.size
  ) {
    throw new Error(
      `provider Form identity ledger has no exact ${projection.candidates.size}-entry release for the descriptor`,
    );
  }
  const resourceTypes = new Set();
  for (const [formIndex, form] of current.forms.entries()) {
    const candidate = projection.candidates.get(
      providerFormRefKey(form.formRef),
    );
    if (
      resourceTypes.has(form.resourceType) ||
      PROVIDER_WITHDRAWN_V1ALPHA2_RESOURCE_TYPES.has(form.resourceType) ||
      (projection.banObjectBucket &&
        (form.resourceType === "takoform_edge_object_bucket" ||
          form.formRef.kind === "ObjectBucket")) ||
      candidate === undefined ||
      candidate.packageDigest !== form.packageDigest
    ) {
      throw new Error(
        `provider ${descriptor.version} embedded Form identity ${formIndex} is not an exact ${projection.rosterLabel} entry`,
      );
    }
    resourceTypes.add(form.resourceType);
  }
  if (resourceTypes.size !== projection.candidates.size) {
    throw new Error(
      `provider ${descriptor.version} identity ledger does not contain one unique resource type per ${projection.rosterLabel} Form`,
    );
  }
  return ledger;
}

export function readProviderDescriptor(repo) {
  const descriptor = readJSON(
    join(repo, "release/version.json"),
    "provider release descriptor",
  );
  if (
    typeof descriptor !== "object" ||
    descriptor === null ||
    descriptor.version !== PROVIDER_VERSION ||
    !PROVIDER_TAG.test(descriptor.tag) ||
    descriptor.tag !== `v${descriptor.version}` ||
    descriptor.sourceRepository !== `github.com/${GITHUB_REPOSITORY}` ||
    descriptor.providerAddress !== PROVIDER_ADDRESS ||
    descriptor.goModule !== `github.com/${GITHUB_REPOSITORY}` ||
    descriptor.signingFingerprint !== PROVIDER_SIGNER ||
    descriptor.publicationStatus !== "candidate-only" ||
    !Array.isArray(descriptor.platforms) ||
    JSON.stringify([...descriptor.platforms].sort()) !==
      JSON.stringify(
        [
          "darwin_amd64",
          "darwin_arm64",
          "linux_amd64",
          "linux_arm64",
          "windows_amd64",
        ].sort(),
      )
  ) {
    throw new Error("provider release descriptor identity is invalid");
  }
  requireExactKeys(
    descriptor.versioning,
    [
      "formDefinitionVersions",
      "formPackageVersions",
      "portableApiVersion",
      "providerCompatibility",
    ],
    "provider release descriptor versioning",
  );
  if (
    descriptor.versioning.providerCompatibility !== "semver-major" ||
    descriptor.versioning.portableApiVersion !== PROVIDER_HOST_API ||
    descriptor.versioning.formDefinitionVersions !==
      "independent-immutable-semver" ||
    descriptor.versioning.formPackageVersions !==
      "content-addressed-current-retained-legacy-semver"
  ) {
    throw new Error(
      "provider release descriptor conflates independent version streams",
    );
  }
  if (
    !Array.isArray(descriptor.cliMatrix) ||
    descriptor.cliMatrix.length !== 2 ||
    descriptor.cliMatrix.some(
      (entry) =>
        !entry ||
        entry.providerAddress !== PROVIDER_ADDRESS ||
        !["OpenTofu", "Terraform"].includes(entry.product) ||
        typeof entry.version !== "string" ||
        entry.version === "",
    ) ||
    new Set(descriptor.cliMatrix.map((entry) => entry.product)).size !== 2
  ) {
    throw new Error("provider release descriptor CLI/FQN matrix is invalid");
  }
  validateProviderIdentityLedger(repo, descriptor);
  return descriptor;
}

function requireProviderTag(tag, descriptor) {
  if (!tag || tag !== descriptor.tag) {
    throw new Error(
      `--tag must exactly match release/version.json (${descriptor.tag})`,
    );
  }
}

function dispatchWorkflow(
  context,
  workflow,
  inputs,
  { headSha = inputs.expected_commit, ref = "main", headBranch = ref } = {},
) {
  if (!COMMIT.test(headSha ?? "")) {
    throw new Error(`dispatch ${workflow} requires an exact head SHA`);
  }
  const requestId = context.uuidFactory();
  if (!REQUEST_ID.test(requestId)) {
    throw new Error("generated workflow request ID is not a canonical UUIDv4");
  }
  const createdAfter = new Date(context.now() - 2000).toISOString();
  const dispatchInputs = { ...inputs, request_id: requestId };
  const workflowName = WORKFLOW_NAMES[workflow];
  if (!workflowName) throw new Error(`unknown dispatch workflow ${workflow}`);
  const listCorrelated = () => {
    const listArguments = [
      "run",
      "list",
      "--repo",
      GITHUB_REPOSITORY,
      "--workflow",
      workflow,
      ...(ref === "main" ? ["--branch", ref] : []),
      "--event",
      "workflow_dispatch",
      "--commit",
      headSha,
      "--created",
      `>=${createdAfter}`,
      "--limit",
      "100",
      "--json",
      "attempt,createdAt,databaseId,displayTitle,event,headBranch,headSha,status,url,workflowName",
    ];
    const listed = JSON.parse(command(context, "gh", listArguments));
    return listed.filter(
      (run) =>
        run.workflowName === workflowName &&
        run.event === "workflow_dispatch" &&
        run.headBranch === headBranch &&
        run.headSha === headSha &&
        new Date(run.createdAt).getTime() >= new Date(createdAfter).getTime() &&
        run.displayTitle === requestId,
    );
  };
  const before = listCorrelated();
  if (before.length !== 0) {
    throw new Error(
      `request UUID unexpectedly matched ${before.length} pre-existing workflow runs`,
    );
  }
  const args = [
    "workflow",
    "run",
    workflow,
    "--repo",
    GITHUB_REPOSITORY,
    "--ref",
    ref,
  ];
  for (const [name, value] of Object.entries(dispatchInputs)) {
    args.push("-f", `${name}=${value}`);
  }
  progress(context, `dispatch ${workflow}`);
  const raw = command(context, "gh", args, {
    echo: true,
    label: `dispatch ${workflow}`,
  });
  let correlated = [];
  for (let attempt = 1; attempt <= 12; attempt += 1) {
    correlated = listCorrelated();
    if (correlated.length === 1) break;
    if (correlated.length > 1) {
      throw new Error(
        `dispatch correlation is ambiguous for request ${requestId}; halt without selecting a run`,
      );
    }
    if (attempt < 12) {
      context.wait(1000);
    }
  }
  if (correlated.length !== 1) {
    throw new Error(
      `workflow dispatch may have occurred but request ${requestId} did not correlate to exactly one run; halt and inspect Actions without selecting latest`,
    );
  }
  const run = correlated[0];
  const runId = String(run.databaseId);
  if (
    !POSITIVE_INTEGER.test(runId) ||
    run.attempt !== 1 ||
    !/^https:\/\/github\.com\/tako0614\/terraform-provider-takoform\/actions\/runs\/[1-9][0-9]*$/u.test(
      run.url,
    )
  ) {
    throw new Error(`correlated workflow run has invalid identity`);
  }
  const returnedURLs = raw.match(
    /https:\/\/github\.com\/tako0614\/terraform-provider-takoform\/actions\/runs\/[1-9][0-9]*/gu,
  );
  if (
    returnedURLs &&
    (returnedURLs.length !== 1 || returnedURLs[0] !== run.url)
  ) {
    throw new Error(
      `workflow dispatch URL disagrees with request-correlated run ${runId}`,
    );
  }
  return { runId, requestId, url: run.url };
}

function requireSuccessfulRun(
  context,
  runId,
  runAttempt,
  { workflowName, headSha, headBranch = "main", displayTitle },
) {
  const raw = command(context, "gh", [
    "run",
    "view",
    runId,
    "--repo",
    GITHUB_REPOSITORY,
    "--attempt",
    runAttempt,
    "--json",
    "attempt,conclusion,databaseId,displayTitle,event,headBranch,headSha,status,url,workflowName",
  ]);
  const run = JSON.parse(raw);
  const attempt = Number(runAttempt);
  const expectedUrl =
    `https://github.com/tako0614/terraform-provider-takoform/actions/runs/${runId}` +
    `/attempts/${runAttempt}`;
  if (
    run.databaseId?.toString() !== runId ||
    run.attempt !== attempt ||
    run.workflowName !== workflowName ||
    run.event !== "workflow_dispatch" ||
    run.headBranch !== headBranch ||
    (headSha ? run.headSha !== headSha : !COMMIT.test(run.headSha ?? "")) ||
    run.status !== "completed" ||
    run.conclusion !== "success" ||
    !REQUEST_ID.test(run.displayTitle ?? "") ||
    run.url !== expectedUrl ||
    (displayTitle && run.displayTitle !== displayTitle)
  ) {
    throw new Error(
      `workflow run ${runId} attempt ${runAttempt} is not the exact successful reviewed candidate: ${raw}`,
    );
  }
  return run;
}

function downloadArtifact(context, runId, artifactName, destination) {
  mkdirSync(destination, { recursive: false });
  progress(context, `download ${artifactName} from run ${runId}`);
  command(context, "gh", [
    "run",
    "download",
    runId,
    "--repo",
    GITHUB_REPOSITORY,
    "--name",
    artifactName,
    "--dir",
    destination,
  ]);
  return destination;
}

function sha256(raw) {
  return `sha256:${createHash("sha256").update(raw).digest("hex")}`;
}

function fileDigest(path) {
  return sha256(readFileSync(path));
}

function compareNames(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function listRegularFiles(root) {
  const files = [];
  const visit = (directory, prefix = "") => {
    const entries = readdirSync(directory, { withFileTypes: true }).sort(
      (left, right) => compareNames(left.name, right.name),
    );
    if (prefix && entries.length === 0) {
      throw new Error(`candidate contains an empty directory: ${prefix}`);
    }
    for (const entry of entries) {
      const path = join(directory, entry.name);
      const name = prefix ? `${prefix}/${entry.name}` : entry.name;
      const stat = lstatSync(path);
      if (stat.isSymbolicLink()) {
        throw new Error(`candidate contains a symbolic link: ${name}`);
      }
      if (stat.isDirectory()) {
        visit(path, name);
      } else if (stat.isFile()) {
        files.push(name);
      } else {
        throw new Error(`candidate contains a non-regular entry: ${name}`);
      }
    }
  };
  visit(root);
  return files;
}

export function parseStrictChecksums(raw) {
  if (!raw.endsWith("\n") || raw === "\n") {
    throw new Error("SHA256SUMS must be non-empty and newline terminated");
  }
  const entries = new Map();
  for (const line of raw.slice(0, -1).split("\n")) {
    const match = /^([0-9a-f]{64})  ([0-9A-Za-z._+/@-]+)$/u.exec(line);
    if (!match) throw new Error(`invalid SHA256SUMS line: ${line}`);
    const name = match[2];
    if (
      name.startsWith("/") ||
      name
        .split("/")
        .some((part) => part === "" || part === "." || part === "..") ||
      entries.has(name)
    ) {
      throw new Error(`unsafe or duplicate SHA256SUMS target: ${name}`);
    }
    entries.set(name, `sha256:${match[1]}`);
  }
  return entries;
}

function verifyChecksumClosure(root, checksumName = "SHA256SUMS") {
  const all = listRegularFiles(root);
  if (!all.includes(checksumName)) {
    throw new Error(`${checksumName} is missing`);
  }
  const entries = parseStrictChecksums(
    readFileSync(join(root, checksumName), "utf8"),
  );
  const expected = all.filter((name) => name !== checksumName).sort();
  const actual = [...entries.keys()].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${checksumName} does not close the exact inventory; expected ${expected.join(", ")}, got ${actual.join(", ")}`,
    );
  }
  for (const [name, digest] of entries) {
    const observed = fileDigest(join(root, ...name.split("/")));
    if (observed !== digest) {
      throw new Error(`${checksumName} digest mismatch for ${name}`);
    }
  }
  return entries;
}

function requireExactKeys(value, keys, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} must be an object`);
  }
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${label} fields differ; expected ${expected.join(", ")}, got ${actual.join(", ")}`,
    );
  }
}

function normalizedPositiveInteger(value, label) {
  const text = String(value);
  if (!POSITIVE_INTEGER.test(text)) {
    throw new Error(`${label} must be a positive integer`);
  }
  return text;
}

function validateCandidateAssets(
  root,
  metadata,
  expectedNames,
  { digestFormat = "prefixed" } = {},
) {
  if (!Array.isArray(metadata.assets)) {
    throw new Error("candidate metadata assets must be an array");
  }
  const seen = new Map();
  for (const asset of metadata.assets) {
    requireExactKeys(asset, ["name", "sha256"], "candidate asset");
    if (
      typeof asset.name !== "string" ||
      asset.name.includes("/") ||
      !(digestFormat === "prefixed"
        ? SHA256.test(asset.sha256)
        : /^[0-9a-f]{64}$/u.test(asset.sha256)) ||
      seen.has(asset.name)
    ) {
      throw new Error(`invalid candidate asset: ${JSON.stringify(asset)}`);
    }
    const path = join(root, "assets", asset.name);
    const expectedDigest =
      digestFormat === "prefixed" ? asset.sha256 : `sha256:${asset.sha256}`;
    if (!existsSync(path) || fileDigest(path) !== expectedDigest) {
      throw new Error(`candidate asset digest mismatch: ${asset.name}`);
    }
    seen.set(asset.name, expectedDigest);
  }
  const actualNames = [...seen.keys()].sort();
  if (
    JSON.stringify(actualNames) !== JSON.stringify([...expectedNames].sort())
  ) {
    throw new Error(
      `candidate public inventory differs; expected ${expectedNames.join(", ")}, got ${actualNames.join(", ")}`,
    );
  }
  return new Map(
    actualNames.map((name) => [
      name,
      { name, sha256: seen.get(name), path: join(root, "assets", name) },
    ]),
  );
}

function verifyCandidateRoot(
  root,
  { tagObject = false, metadataProfile = "compact-optional-lf" } = {},
) {
  const expectedRoot = [
    "SHA256SUMS",
    "assets",
    "metadata.json",
    ...(tagObject ? ["tag-object"] : []),
  ].sort();
  const rootEntries = readdirSync(root, { withFileTypes: true }).sort(
    (left, right) => compareNames(left.name, right.name),
  );
  if (
    JSON.stringify(rootEntries.map((entry) => entry.name)) !==
      JSON.stringify(expectedRoot) ||
    rootEntries.some((entry) =>
      entry.name === "assets" ? !entry.isDirectory() : !entry.isFile(),
    )
  ) {
    throw new Error(
      `candidate root layout differs; expected ${expectedRoot.join(", ")}`,
    );
  }
  verifyChecksumClosure(root);
  return parseCandidateMetadata(
    readFileSync(join(root, "metadata.json")),
    "candidate metadata",
    { profile: metadataProfile },
  );
}

function withTemporaryDirectory(prefix, operation) {
  const configuredRoot = process.platform === "win32" ? tmpdir() : "/tmp";
  const trustedRoot = realpathSync(configuredRoot);
  if (!lstatSync(trustedRoot).isDirectory()) {
    throw new Error(
      "release blocked: trusted temporary root is not a directory",
    );
  }
  const root = mkdtempSync(join(trustedRoot, `${prefix}-`));
  try {
    return operation(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function localTagOID(context, tag) {
  return git(
    context,
    "for-each-ref",
    "--format=%(objectname)",
    `refs/tags/${tag}`,
  );
}

function recoveryLocalTagOID(context, tag) {
  return recoveryGit(context, [
    "for-each-ref",
    "--format=%(objectname)",
    `refs/tags/${tag}`,
  ]).trim();
}

function remoteTagState(context, tag) {
  const ref = `refs/tags/${tag}`;
  const output = git(
    context,
    "ls-remote",
    "--tags",
    "origin",
    ref,
    `${ref}^{}`,
  );
  return parseRemoteTagState(output, ref);
}

function recoveryRemoteTagState(context, tag) {
  const ref = `refs/tags/${tag}`;
  const output = recoveryGit(context, [
    "ls-remote",
    "--tags",
    SOURCE_REPOSITORY,
    ref,
    `${ref}^{}`,
  ]).trim();
  return parseRemoteTagState(output, ref);
}

function parseRemoteTagState(output, ref) {
  const state = {};
  for (const line of output ? output.split("\n") : []) {
    const [oid, name] = line.split("\t");
    if (name === ref) state.object = oid;
    if (name === `${ref}^{}`) state.commit = oid;
  }
  return state;
}

function observeTagFailureState(context, tag) {
  const localResult = attemptCommand(context, "git", [
    "for-each-ref",
    "--format=%(objectname)",
    `refs/tags/${tag}`,
  ]);
  const local = localResult.ok ? localResult.output.trim() : "";
  const ref = `refs/tags/${tag}`;
  const remoteResult = attemptCommand(context, "git", [
    "ls-remote",
    "--tags",
    "origin",
    ref,
    `${ref}^{}`,
  ]);
  const remote = remoteResult.ok
    ? parseRemoteTagState(remoteResult.output.trim(), ref)
    : null;
  let mutationState = "UNCHANGED";
  if (!localResult.ok && !remoteResult.ok) {
    mutationState = "LOCAL_AND_REMOTE_UNREADABLE";
  } else if (!remoteResult.ok) mutationState = "REMOTE_UNREADABLE";
  else if (remote.object || remote.commit) mutationState = "REMOTE_PRESENT";
  else if (!localResult.ok) mutationState = "LOCAL_UNREADABLE";
  else if (local) mutationState = "LOCAL_ONLY";
  return {
    local: local || null,
    localReadable: localResult.ok,
    remote,
    remoteReadable: remoteResult.ok,
    mutationState,
  };
}

function assertTagAbsent(context, tag) {
  const local = localTagOID(context, tag);
  const remote = remoteTagState(context, tag);
  if (local || remote.object || remote.commit) {
    throw new Error(
      `no-overwrite blocked ${tag}: local=${local || "absent"} remote=${JSON.stringify(remote)}`,
    );
  }
}

function assertExactRemoteTag(context, tag, expectedCommit, expectedObject) {
  const state = remoteTagState(context, tag);
  if (
    !state.object ||
    state.commit !== expectedCommit ||
    (expectedObject && state.object !== expectedObject)
  ) {
    throw new Error(
      `remote tag ${tag} is not the exact annotated object/commit: ${JSON.stringify(state)}`,
    );
  }
  return state;
}

function assertRecoveryExactRemoteTag(
  context,
  tag,
  expectedCommit,
  expectedObject,
) {
  const state = recoveryRemoteTagState(context, tag);
  if (
    !state.object ||
    state.commit !== expectedCommit ||
    state.object !== expectedObject
  ) {
    throw new Error(
      `remote tag ${tag} is not the exact annotated object/commit: ${JSON.stringify(state)}`,
    );
  }
  return state;
}

function reconstructCandidateTagObject(
  context,
  tag,
  expectedCommit,
  root,
  metadata,
) {
  const tagObject = readFileSync(join(root, "tag-object"));
  if (
    metadata.objectFormat !==
      git(context, "rev-parse", "--show-object-format") ||
    !/^[0-9a-f]{40,64}$/u.test(metadata.tagObjectOid) ||
    !SHA256.test(metadata.tagObjectSha256) ||
    fileDigest(join(root, "tag-object")) !== metadata.tagObjectSha256
  ) {
    throw new Error("candidate tag-object metadata is invalid");
  }
  const text = tagObject.toString("utf8");
  const header = text.slice(0, text.indexOf("\n\n"));
  const fields = Object.fromEntries(
    header.split("\n").map((line) => {
      const space = line.indexOf(" ");
      return [line.slice(0, space), line.slice(space + 1)];
    }),
  );
  if (
    fields.object !== expectedCommit ||
    fields.type !== "commit" ||
    fields.tag !== tag
  ) {
    throw new Error(
      "candidate tag object does not bind the expected tag/commit",
    );
  }
  const reconstructed = command(context, "git", ["mktag"], {
    input: tagObject,
    label: "reconstruct candidate tag object",
  })
    .toString("utf8")
    .trim();
  if (reconstructed !== metadata.tagObjectOid) {
    throw new Error(
      `candidate tag object OID ${metadata.tagObjectOid} reconstructed as ${reconstructed}`,
    );
  }
  return reconstructed;
}

function materializeTagObject(context, tag, expectedCommit, root, metadata) {
  const reconstructed = reconstructCandidateTagObject(
    context,
    tag,
    expectedCommit,
    root,
    metadata,
  );
  if (localTagOID(context, tag)) {
    throw new Error(`local tag ${tag} already exists; refusing resume`);
  }
  const zero = "0".repeat(reconstructed.length);
  command(context, "git", [
    "update-ref",
    `refs/tags/${tag}`,
    reconstructed,
    zero,
  ]);
  return reconstructed;
}

function splitSignedProviderTagObject(raw, tag, expectedCommit) {
  if (!Buffer.isBuffer(raw)) {
    throw new Error(
      `provider tag ${tag} exact object bytes were not read as a Buffer`,
    );
  }
  const object = raw;
  const signatureBegin = Buffer.from("-----BEGIN PGP SIGNATURE-----\n");
  const signatureEnd = Buffer.from("-----END PGP SIGNATURE-----\n");
  const signatureOffset = object.indexOf(signatureBegin);
  if (
    signatureOffset <= 0 ||
    object[signatureOffset - 1] !== 0x0a ||
    object.indexOf(signatureBegin, signatureOffset + 1) !== -1 ||
    object.length < signatureEnd.length ||
    !object.subarray(object.length - signatureEnd.length).equals(signatureEnd)
  ) {
    throw new Error(
      `provider tag ${tag} does not contain one exact armored OpenPGP signature`,
    );
  }
  const payload = object.subarray(0, signatureOffset);
  const signature = object.subarray(signatureOffset);
  const expectedHeader = Buffer.from(
    `object ${expectedCommit}\ntype commit\ntag ${tag}\n`,
  );
  if (
    payload.length <= expectedHeader.length ||
    !payload.subarray(0, expectedHeader.length).equals(expectedHeader)
  ) {
    throw new Error(`provider tag ${tag} signed payload identity differs`);
  }
  return { payload, signature };
}

function assertPinnedProviderGpgVerification(verification, tag) {
  const valid = asText(verification.output)
    .split("\n")
    .map((line) => /^\[GNUPG:\] VALIDSIG ([0-9A-F]{40})\b/u.exec(line)?.[1])
    .filter(Boolean);
  if (!verification.ok || valid.length !== 1 || valid[0] !== PROVIDER_SIGNER) {
    throw new Error(
      `provider tag ${tag} is not signed by the pinned exact signer`,
    );
  }
  return PROVIDER_SIGNER;
}

function assertExactSignedProviderTag(
  context,
  { tag, expectedCommit, expectedObject },
) {
  const local = recoveryLocalTagOID(context, tag);
  const localType = local
    ? recoveryGit(context, ["cat-file", "-t", `refs/tags/${tag}`]).trim()
    : "";
  const localCommit = local
    ? recoveryGit(context, ["rev-parse", `refs/tags/${tag}^{commit}`]).trim()
    : "";
  if (
    local !== expectedObject ||
    localType !== "tag" ||
    localCommit !== expectedCommit
  ) {
    throw new Error(
      `provider recovery requires exact local annotated tag object ${expectedObject} -> ${expectedCommit}; observed ${JSON.stringify(
        {
          object: local || null,
          type: localType || null,
          commit: localCommit || null,
        },
      )}`,
    );
  }
  const remote = assertRecoveryExactRemoteTag(
    context,
    tag,
    expectedCommit,
    expectedObject,
  );
  const rawTagObject = recoveryGit(
    context,
    ["cat-file", "tag", expectedObject],
    { encoding: null },
  );
  const signed = splitSignedProviderTagObject(
    rawTagObject,
    tag,
    expectedCommit,
  );
  withTemporaryDirectory("takoform-provider-recovery-gpg", (gpgHome) => {
    chmodSync(gpgHome, 0o700);
    const gpgExecutable = trustedGpgExecutable();
    const gpgEnvironment = isolatedGpgEnvironment(gpgHome);
    const key = join(context.repo, "release/keys/provider-signing-key.asc");
    const inspect = command(
      context,
      gpgExecutable,
      [
        "--no-options",
        "--homedir",
        gpgHome,
        "--batch",
        "--no-tty",
        "--with-colons",
        "--import-options",
        "show-only",
        "--import",
        key,
      ],
      { env: gpgEnvironment },
    );
    const fingerprints = inspect
      .split("\n")
      .filter((line) => line.startsWith("fpr:"))
      .map((line) => line.split(":")[9])
      .filter(Boolean);
    if (fingerprints.length !== 1 || fingerprints[0] !== PROVIDER_SIGNER) {
      throw new Error("provider public signing key fingerprint drifted");
    }
    command(
      context,
      gpgExecutable,
      [
        "--no-options",
        "--homedir",
        gpgHome,
        "--batch",
        "--no-tty",
        "--import",
        key,
      ],
      { env: gpgEnvironment },
    );
    const payloadPath = join(gpgHome, "tag-payload");
    const signaturePath = join(gpgHome, "tag-signature.asc");
    writeFileSync(payloadPath, signed.payload, { mode: 0o600 });
    writeFileSync(signaturePath, signed.signature, { mode: 0o600 });
    const verification = attemptCommand(
      context,
      gpgExecutable,
      [
        "--no-options",
        "--homedir",
        gpgHome,
        "--batch",
        "--no-tty",
        "--no-auto-key-retrieve",
        "--status-fd",
        "1",
        "--verify",
        signaturePath,
        payloadPath,
      ],
      { env: gpgEnvironment },
    );
    try {
      assertPinnedProviderGpgVerification(verification, tag);
    } catch (error) {
      if (verification.output) context.stdout.write(verification.output);
      if (verification.stderr) context.stderr.write(verification.stderr);
      throw error;
    }
  });
  return {
    local: { object: local, type: localType, commit: localCommit },
    remote,
    signerFingerprint: PROVIDER_SIGNER,
  };
}

function ensureCandidateTagPublished(
  context,
  tag,
  expectedCommit,
  root,
  metadata,
) {
  assertTagAbsent(context, tag);
  const object = materializeTagObject(
    context,
    tag,
    expectedCommit,
    root,
    metadata,
  );
  pushExactTag(context, tag, expectedCommit, object);
  return object;
}

function pushExactTag(context, tag, expectedCommit, expectedObject) {
  progress(context, `push create-only tag ${tag}`);
  command(
    context,
    "git",
    [
      "push",
      "--no-verify",
      `--force-with-lease=refs/tags/${tag}:`,
      SOURCE_REPOSITORY,
      `refs/tags/${tag}:refs/tags/${tag}`,
    ],
    { env: gitPushEnvironment() },
  );
  return assertExactRemoteTag(context, tag, expectedCommit, expectedObject);
}

function apiTagPath(tag) {
  return `repos/${GITHUB_REPOSITORY}/releases/tags/${encodeURIComponent(tag)}`;
}

function getRelease(context, tag, { absentOK = false } = {}) {
  const result = attemptCommand(context, "gh", ["api", apiTagPath(tag)]);
  if (result.ok) {
    return JSON.parse(result.output);
  }
  if (absentOK && /\bHTTP 404\b/u.test(result.stderr)) return null;
  if (result.output) context.stdout.write(result.output);
  if (result.stderr) context.stderr.write(result.stderr);
  throw new Error(`cannot read GitHub Release ${tag}`);
}

function releaseListArguments() {
  return [
    "api",
    "--paginate",
    "--slurp",
    `repos/${GITHUB_REPOSITORY}/releases?per_page=100`,
  ];
}

function parseReleasePages(raw, tag) {
  const pages = JSON.parse(raw);
  if (!Array.isArray(pages) || pages.some((page) => !Array.isArray(page))) {
    throw new Error("GitHub Release list pagination response is invalid");
  }
  const releases = pages.flat();
  const ids = new Set();
  for (const release of releases) {
    if (
      !release ||
      typeof release !== "object" ||
      Array.isArray(release) ||
      !Number.isSafeInteger(release.id) ||
      release.id <= 0 ||
      ids.has(release.id) ||
      typeof release.tag_name !== "string" ||
      typeof release.draft !== "boolean"
    ) {
      throw new Error("GitHub Release list contains an invalid identity");
    }
    ids.add(release.id);
  }
  return releases.filter((release) => release.tag_name === tag);
}

function releasesByTag(context, tag) {
  return parseReleasePages(command(context, "gh", releaseListArguments()), tag);
}

function assertReleaseAbsent(context, tag) {
  const releases = releasesByTag(context, tag);
  if (releases.length !== 0) {
    throw new Error(
      `no-overwrite blocked ${tag}: GitHub Release identities already exist: ` +
        releases
          .map(
            (release) => `${release.id}:${release.draft ? "draft" : "public"}`,
          )
          .join(","),
    );
  }
}

function assertUniqueReleaseIdentity(context, tag, releaseId, draft) {
  const releases = releasesByTag(context, tag);
  if (
    releases.length !== 1 ||
    releases[0].id !== releaseId ||
    releases[0].draft !== draft
  ) {
    throw new Error(
      `no-overwrite blocked ${tag}: expected only release ${releaseId}:${draft ? "draft" : "public"}, observed ` +
        (releases.length === 0
          ? "none"
          : releases
              .map(
                (release) =>
                  `${release.id}:${release.draft ? "draft" : "public"}`,
              )
              .join(",")),
    );
  }
  return releases[0];
}

function expectedAssetMap(assets) {
  return new Map(
    [...assets.values()].map((asset) => [
      asset.name,
      { digest: asset.sha256, path: asset.path },
    ]),
  );
}

function validateReleaseReadback(
  release,
  tag,
  assets,
  {
    prerelease = false,
    expectedReleaseId,
    expectedName,
    expectedBody,
    expectedTargetCommitish,
    expectedAssetsURL,
    expectedUploadURL,
  } = {},
) {
  const expected = expectedAssetMap(assets);
  if (
    !Number.isSafeInteger(release.id) ||
    release.id <= 0 ||
    (expectedReleaseId !== undefined && release.id !== expectedReleaseId) ||
    release.tag_name !== tag ||
    (expectedName !== undefined && release.name !== expectedName) ||
    (expectedBody !== undefined && release.body !== expectedBody) ||
    (expectedTargetCommitish !== undefined &&
      release.target_commitish !== expectedTargetCommitish) ||
    (expectedAssetsURL !== undefined &&
      release.assets_url !== expectedAssetsURL) ||
    (expectedUploadURL !== undefined &&
      release.upload_url !== expectedUploadURL) ||
    release.draft !== false ||
    release.prerelease !== prerelease ||
    release.immutable !== true ||
    typeof release.html_url !== "string" ||
    !Array.isArray(release.assets) ||
    release.assets.length !== expected.size
  ) {
    throw new Error(
      `published release identity/state mismatch: ${JSON.stringify({
        id: release.id,
        tag_name: release.tag_name,
        draft: release.draft,
        prerelease: release.prerelease,
        immutable: release.immutable,
        assets: release.assets?.length,
      })}`,
    );
  }
  const seen = new Set();
  for (const remote of release.assets) {
    const local = expected.get(remote.name);
    if (
      !local ||
      seen.has(remote.name) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.digest ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0
    ) {
      throw new Error(`published asset mismatch: ${JSON.stringify(remote)}`);
    }
    seen.add(remote.name);
  }
  return release;
}

function downloadAndCompareRelease(context, tag, assets, parent) {
  const output = join(parent, "public-readback");
  mkdirSync(output);
  command(context, "gh", [
    "release",
    "download",
    tag,
    "--repo",
    GITHUB_REPOSITORY,
    "--dir",
    output,
  ]);
  const names = listRegularFiles(output);
  const expected = [...assets.keys()].sort();
  if (JSON.stringify(names) !== JSON.stringify(expected)) {
    throw new Error(
      `downloaded release inventory differs: ${names.join(", ")}`,
    );
  }
  for (const [name, asset] of assets) {
    if (fileDigest(join(output, name)) !== asset.sha256) {
      throw new Error(`downloaded release digest mismatch: ${name}`);
    }
  }
}

function validateDraftBeforePublication(
  draft,
  { releaseId, tag, prerelease, assets },
) {
  if (
    draft.id !== releaseId ||
    draft.draft !== true ||
    draft.prerelease !== prerelease ||
    draft.tag_name !== tag ||
    !Array.isArray(draft.assets) ||
    draft.assets.length !== assets.size
  ) {
    throw new Error("draft changed identity before publication");
  }
  const draftAssets = new Map();
  const assetIDs = new Set();
  for (const remote of draft.assets) {
    const local = assets.get(remote.name);
    if (
      !local ||
      draftAssets.has(remote.name) ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0 ||
      assetIDs.has(remote.id) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.sha256 ||
      remote.size !== lstatSync(local.path).size
    ) {
      throw new Error(
        "draft API asset identity differs from the same-run candidate",
      );
    }
    draftAssets.set(remote.name, remote);
    assetIDs.add(remote.id);
  }
}

function readExactRetainedDraft(
  context,
  { releaseId, tag, prerelease = false, body, assets, requireComplete = false },
) {
  assertUniqueReleaseIdentity(context, tag, releaseId, true);
  const draft = JSON.parse(
    command(context, "gh", [
      "api",
      `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
    ]),
  );
  const expectedUploadURL =
    `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets{?name,label}`;
  const expectedAssetsURL =
    `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets`;
  if (
    draft.id !== releaseId ||
    draft.tag_name !== tag ||
    draft.target_commitish !== "main" ||
    draft.name !== tag ||
    draft.body !== body ||
    draft.draft !== true ||
    draft.prerelease !== prerelease ||
    draft.immutable !== false ||
    draft.published_at !== null ||
    draft.assets_url !== expectedAssetsURL ||
    draft.upload_url !== expectedUploadURL ||
    !Array.isArray(draft.assets)
  ) {
    throw new Error(
      `retained draft ${releaseId} identity/state differs from the exact recovery input`,
    );
  }
  const seenNames = new Set();
  const seenIDs = new Set();
  for (const remote of draft.assets) {
    const local = assets.get(remote.name);
    if (
      !local ||
      seenNames.has(remote.name) ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0 ||
      seenIDs.has(remote.id) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.sha256 ||
      remote.size !== lstatSync(local.path).size
    ) {
      throw new Error(
        `retained draft ${releaseId} contains an unknown, duplicate, or drifted asset`,
      );
    }
    seenNames.add(remote.name);
    seenIDs.add(remote.id);
  }
  if (requireComplete && seenNames.size !== assets.size) {
    throw new Error(
      `retained draft ${releaseId} does not contain the complete exact candidate`,
    );
  }
  return {
    draft,
    missing: [...assets.values()]
      .filter((asset) => !seenNames.has(asset.name))
      .sort((left, right) => compareNames(left.name, right.name)),
  };
}

function validateExactReleasePatchResponse(
  response,
  { releaseId, tag, prerelease, body, assets },
) {
  const expectedAssetsURL =
    `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets`;
  const expectedUploadURL =
    `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets{?name,label}`;
  if (
    response.id !== releaseId ||
    response.tag_name !== tag ||
    response.target_commitish !== "main" ||
    response.name !== tag ||
    response.body !== body ||
    response.draft !== false ||
    response.prerelease !== prerelease ||
    response.assets_url !== expectedAssetsURL ||
    response.upload_url !== expectedUploadURL ||
    !Array.isArray(response.assets) ||
    response.assets.length !== assets.size
  ) {
    throw new Error(
      `release ${releaseId} PATCH response differs from the exact retained draft identity`,
    );
  }
  const seenNames = new Set();
  const seenIDs = new Set();
  for (const remote of response.assets) {
    const local = assets.get(remote.name);
    if (
      !local ||
      seenNames.has(remote.name) ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0 ||
      seenIDs.has(remote.id) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.sha256 ||
      remote.size !== lstatSync(local.path).size
    ) {
      throw new Error(
        `release ${releaseId} PATCH response asset closure differs from the exact candidate`,
      );
    }
    seenNames.add(remote.name);
    seenIDs.add(remote.id);
  }
  return response;
}

function reportRetainedDraftFailure(
  context,
  tag,
  releaseId,
  { surface = FORM_SURFACE } = {},
) {
  let mutationState = "REMOTE_STATE_UNREADABLE";
  let observedReleaseIDs = [];
  const listed = attemptCommand(context, "gh", releaseListArguments());
  if (listed.ok) {
    try {
      const matches = parseReleasePages(listed.output, tag);
      observedReleaseIDs = matches.map((release) => release.id);
      if (matches.length !== 1 || matches[0].id !== releaseId) {
        mutationState = "REMOTE_STATE_AMBIGUOUS";
      } else if (matches[0].draft === true) {
        mutationState = "MATCHING_DRAFT_RETAINED";
      } else {
        mutationState = "PUBLICATION_INDETERMINATE";
      }
    } catch {
      mutationState = "REMOTE_STATE_UNREADABLE";
    }
  }
  context.stderr.write(
    `${JSON.stringify({
      kind: "takos.deploy-failure@v1",
      surface,
      phase: "recover-draft",
      tag,
      releaseId,
      observedReleaseIDs,
      mutationState,
      instruction:
        "read the authoritative tag/release state; rerun recover-draft only if the same exact draft is retained",
    })}\n`,
  );
}

function resumeDraftReleaseLocally(
  context,
  {
    releaseId,
    tag,
    assets,
    body,
    prerelease = false,
    temporaryRoot,
    surface = FORM_SURFACE,
    preMutationFence = () => {},
    prePublishFence = () => {},
  },
) {
  const expectedAssetsURL =
    `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets`;
  const expectedUploadURL =
    `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets{?name,label}`;
  try {
    const initial = readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
    });
    progress(
      context,
      `resume exact draft ${releaseId}: upload ${initial.missing.length} missing assets`,
    );
    for (const asset of initial.missing) {
      preMutationFence("upload-asset", releaseId);
      const uploaded = JSON.parse(
        command(
          context,
          "gh",
          [
            "api",
            "--method",
            "POST",
            `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets?name=${encodeURIComponent(asset.name)}`,
            "--header",
            "Content-Type: application/octet-stream",
            "--input",
            asset.path,
          ],
          { env: githubUploadEnvironment(context) },
        ),
      );
      if (
        !Number.isSafeInteger(uploaded.id) ||
        uploaded.id <= 0 ||
        uploaded.name !== asset.name ||
        uploaded.state !== "uploaded" ||
        uploaded.digest !== asset.sha256 ||
        uploaded.size !== lstatSync(asset.path).size
      ) {
        throw new Error(
          `exact retained draft ${releaseId} asset upload mismatch: ${asset.name}`,
        );
      }
    }
    readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
      requireComplete: true,
    });
    prePublishFence(releaseId);
    preMutationFence("publish-draft", releaseId);
    readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
      requireComplete: true,
    });
    progress(context, `publish exact retained draft ${releaseId}`);
    let patchResponse;
    try {
      patchResponse = JSON.parse(
        command(context, "gh", [
          "api",
          "--method",
          "PATCH",
          `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
          "-f",
          `tag_name=${tag}`,
          "-f",
          "target_commitish=main",
          "-f",
          `name=${tag}`,
          "-f",
          `body=${body}`,
          "-F",
          "draft=false",
          "-F",
          `prerelease=${prerelease}`,
          "-f",
          "make_latest=false",
        ]),
      );
    } catch (error) {
      const observed = attemptCommand(context, "gh", ["api", apiTagPath(tag)]);
      if (!observed.ok) throw error;
      let release;
      try {
        release = JSON.parse(observed.output);
      } catch {
        throw error;
      }
      if (release.id !== releaseId || release.draft !== false) {
        throw error;
      }
      patchResponse = release;
    }
    validateExactReleasePatchResponse(patchResponse, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
    });
    let release;
    let published = false;
    for (let attempt = 1; attempt <= 12; attempt += 1) {
      release = getRelease(context, tag);
      if (
        release.id === releaseId &&
        release.draft === false &&
        release.immutable === true
      ) {
        published = true;
        break;
      }
      if (attempt < 12) context.wait(1000);
    }
    if (!published) {
      throw new Error(
        `release ${releaseId} did not converge to immutable exact publication`,
      );
    }
    validateReleaseReadback(release, tag, assets, {
      prerelease,
      expectedReleaseId: releaseId,
      expectedName: tag,
      expectedBody: body,
      expectedTargetCommitish: "main",
      expectedAssetsURL,
      expectedUploadURL,
    });
    downloadAndCompareRelease(context, tag, assets, temporaryRoot);
    assertUniqueReleaseIdentity(context, tag, releaseId, false);
    return release;
  } catch (error) {
    reportRetainedDraftFailure(context, tag, releaseId, { surface });
    throw error;
  }
}

function publishReleaseLocally(
  context,
  {
    tag,
    surface = tag.startsWith("v") ? PROVIDER_SURFACE : FORM_SURFACE,
    assets,
    body,
    prerelease = false,
    temporaryRoot,
    strictIdentity = false,
    preMutationFence = () => {},
    prePublishFence = () => {},
  },
) {
  assertReleaseAbsent(context, tag);
  let releaseId = null;
  let published = false;
  let draftCreationAttempted = false;
  try {
    progress(context, `create local-authority draft for ${tag}`);
    preMutationFence("create-draft", null);
    draftCreationAttempted = true;
    const created = JSON.parse(
      command(context, "gh", [
        "api",
        "--method",
        "POST",
        `repos/${GITHUB_REPOSITORY}/releases`,
        "-f",
        `tag_name=${tag}`,
        ...(strictIdentity ? ["-f", "target_commitish=main"] : []),
        "-f",
        `name=${tag}`,
        "-F",
        "draft=true",
        "-F",
        `prerelease=${prerelease}`,
        "-f",
        `body=${body}`,
        "-f",
        "make_latest=false",
      ]),
    );
    if (
      !Number.isSafeInteger(created.id) ||
      created.id <= 0 ||
      created.tag_name !== tag ||
      created.draft !== true ||
      created.upload_url !==
        `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${created.id}/assets{?name,label}`
    ) {
      throw new Error(`draft creation returned an unexpected identity`);
    }
    releaseId = created.id;
    const orderedAssets = [...assets.values()].sort((left, right) =>
      compareNames(left.name, right.name),
    );
    if (strictIdentity) {
      const initial = readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
      });
      if (
        initial.draft.assets.length !== 0 ||
        initial.missing.length !== assets.size
      ) {
        throw new Error(
          `new exact draft ${releaseId} is not empty before upload`,
        );
      }
    }
    progress(context, `upload ${orderedAssets.length} exact assets`);
    for (const asset of orderedAssets) {
      preMutationFence("upload-asset", releaseId);
      const uploaded = JSON.parse(
        command(
          context,
          "gh",
          [
            "api",
            "--method",
            "POST",
            `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets?name=${encodeURIComponent(asset.name)}`,
            "--header",
            "Content-Type: application/octet-stream",
            "--input",
            asset.path,
          ],
          { env: githubUploadEnvironment(context) },
        ),
      );
      if (
        !Number.isSafeInteger(uploaded.id) ||
        uploaded.id <= 0 ||
        uploaded.name !== asset.name ||
        uploaded.state !== "uploaded" ||
        uploaded.digest !== asset.sha256 ||
        uploaded.size !== lstatSync(asset.path).size
      ) {
        throw new Error(
          `exact release ${releaseId} asset upload mismatch: ${asset.name}`,
        );
      }
    }
    if (strictIdentity) {
      readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        requireComplete: true,
      });
    } else {
      const draft = JSON.parse(
        command(context, "gh", [
          "api",
          `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
        ]),
      );
      validateDraftBeforePublication(draft, {
        releaseId,
        tag,
        prerelease,
        assets,
      });
    }
    prePublishFence(releaseId);
    preMutationFence("publish-draft", releaseId);
    if (strictIdentity) {
      readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        requireComplete: true,
      });
    } else {
      assertUniqueReleaseIdentity(context, tag, releaseId, true);
    }
    progress(context, `publish exact draft ${releaseId}`);
    const patchResponse = JSON.parse(
      command(context, "gh", [
        "api",
        "--method",
        "PATCH",
        `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
        ...(strictIdentity
          ? [
              "-f",
              `tag_name=${tag}`,
              "-f",
              "target_commitish=main",
              "-f",
              `name=${tag}`,
              "-f",
              `body=${body}`,
              "-F",
              `prerelease=${prerelease}`,
            ]
          : []),
        "-F",
        "draft=false",
        "-f",
        "make_latest=false",
      ]),
    );
    if (strictIdentity) {
      validateExactReleasePatchResponse(patchResponse, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
      });
    }
    let release;
    for (let attempt = 1; attempt <= 12; attempt += 1) {
      release = getRelease(context, tag);
      if (
        release.id === releaseId &&
        release.draft === false &&
        release.immutable === true
      ) {
        published = true;
        break;
      }
      if (attempt < 12) {
        context.wait(1000);
      }
    }
    if (!published) {
      throw new Error(
        `release ${releaseId} did not converge to immutable exact publication`,
      );
    }
    validateReleaseReadback(release, tag, assets, {
      prerelease,
      ...(strictIdentity
        ? {
            expectedReleaseId: releaseId,
            expectedName: tag,
            expectedBody: body,
            expectedTargetCommitish: "main",
            expectedAssetsURL: `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets`,
            expectedUploadURL: `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets{?name,label}`,
          }
        : {}),
    });
    downloadAndCompareRelease(context, tag, assets, temporaryRoot);
    assertUniqueReleaseIdentity(context, tag, releaseId, false);
    return release;
  } catch (error) {
    let state = draftCreationAttempted
      ? "DRAFT_CREATION_INDETERMINATE"
      : "BEFORE_DRAFT";
    let observedReleaseIDs = [];
    if (releaseId !== null) {
      const observed = attemptCommand(context, "gh", [
        "api",
        `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
      ]);
      if (observed.ok) {
        let release;
        try {
          release = JSON.parse(observed.output);
        } catch {
          release = null;
        }
        if (!release || typeof release !== "object" || Array.isArray(release)) {
          state = "REMOTE_STATE_UNREADABLE";
        } else if (release.tag_name === tag && release.draft === true) {
          state = "MATCHING_DRAFT_RETAINED";
        } else {
          state =
            release.draft === false
              ? "PUBLICATION_INDETERMINATE"
              : "DRAFT_INDETERMINATE";
        }
      } else {
        state = "REMOTE_STATE_UNREADABLE";
      }
    }
    if (draftCreationAttempted) {
      const observed = attemptCommand(context, "gh", releaseListArguments());
      if (observed.ok) {
        try {
          const matches = parseReleasePages(observed.output, tag);
          observedReleaseIDs = matches.map((release) => release.id);
          if (matches.length > 1) {
            state = "REMOTE_STATE_AMBIGUOUS";
          } else if (
            matches.length === 1 &&
            releaseId !== null &&
            matches[0].id !== releaseId
          ) {
            state = "REMOTE_STATE_AMBIGUOUS";
          } else if (
            releaseId === null &&
            matches.length === 1 &&
            matches[0].draft === true
          ) {
            state = "MATCHING_DRAFT_RETAINED";
          } else if (releaseId === null && matches.length === 1) {
            state = "PUBLICATION_INDETERMINATE";
          }
        } catch {
          state = "REMOTE_STATE_UNREADABLE";
        }
      } else {
        state = "REMOTE_STATE_UNREADABLE";
      }
    }
    context.stderr.write(
      `${JSON.stringify({
        kind: "takos.deploy-failure@v1",
        surface,
        tag,
        releaseId,
        observedReleaseIDs,
        mutationState: state,
        instruction:
          state.includes("INDETERMINATE") ||
          state.includes("UNREADABLE") ||
          state.includes("AMBIGUOUS") ||
          state === "MATCHING_DRAFT_RETAINED"
            ? "read the authoritative tag/release state; do not retry blindly"
            : "production release was not published",
      })}\n`,
    );
    throw error;
  }
}

function verifyNamedChecksums(root, checksumName, expectedTargets) {
  const entries = parseStrictChecksums(
    readFileSync(join(root, checksumName), "utf8"),
  );
  const expected = [...expectedTargets].sort();
  const actual = [...entries.keys()].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${checksumName} target set differs; expected ${expected.join(", ")}, got ${actual.join(", ")}`,
    );
  }
  for (const [name, digest] of entries) {
    if (fileDigest(join(root, name)) !== digest) {
      throw new Error(`${checksumName} digest mismatch for ${name}`);
    }
  }
}

function registryVersions(context) {
  const raw = command(context, "curl", [
    "--fail-with-body",
    "--silent",
    "--show-error",
    "--location",
    "https://registry.terraform.io/v1/providers/tako0614/takoform/versions",
  ]);
  const value = JSON.parse(raw);
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    !Array.isArray(value.versions)
  ) {
    throw new Error("Terraform Registry versions response is invalid");
  }
  const versions = value.versions.map((entry) => entry?.version);
  if (
    versions.some(
      (version) =>
        typeof version !== "string" || !PROVIDER_TAG.test(`v${version}`),
    ) ||
    new Set(versions).size !== versions.length
  ) {
    throw new Error(
      "Terraform Registry versions response contains invalid or duplicate identities",
    );
  }
  return new Set(versions);
}

function assertRegistryVersionAbsent(context, version) {
  if (registryVersions(context).has(version)) {
    throw new Error(
      `no-overwrite blocked provider ${version}: Terraform Registry version identity already exists`,
    );
  }
}

function assertReleaseImmutabilityEnabled(context) {
  const value = JSON.parse(
    command(context, "gh", [
      "api",
      `repos/${GITHUB_REPOSITORY}/immutable-releases`,
    ]),
  );
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    value.enabled !== true
  ) {
    throw new Error(
      "release publication requires repository immutable releases to be enabled",
    );
  }
}

function providerAssetNames(descriptor) {
  const base = `terraform-provider-takoform_${descriptor.version}`;
  const archives = descriptor.platforms
    .map((platform) => `${base}_${platform}.zip`)
    .sort();
  const payload = [
    ...archives,
    ...archives.map((name) => `${name}.spdx.json`),
    `${base}_manifest.json`,
    `${base}_SHA256SUMS`,
    `${base}_SHA256SUMS.sig`,
  ].sort();
  const provenance = `${base}_provenance.intoto.json`;
  const provenanceSignature = `${provenance}.sig`;
  return {
    archives,
    checksum: `${base}_SHA256SUMS`,
    signature: `${base}_SHA256SUMS.sig`,
    manifest: `${base}_manifest.json`,
    payload,
    provenance,
    provenanceSignature,
    all: [...payload, provenance, provenanceSignature].sort(),
  };
}

function verifyProviderReleaseProvenance(context, root, descriptor, expected) {
  const names = providerAssetNames(descriptor);
  const raw = command(context, "go", [
    "-C",
    "cmd/provider-release",
    "run",
    ".",
    "verify-release-provenance",
    "--assets",
    root,
    "--expected-tag",
    descriptor.tag,
    "--expected-source-commit",
    expected.sourceCommit,
    "--expected-tooling-commit",
    expected.toolingCommit,
    "--expected-request-id",
    expected.requestId,
    "--expected-run-id",
    expected.runId,
    "--expected-run-attempt",
    expected.runAttempt,
    "--expected-tag-object-oid",
    expected.tagObjectOid,
    "--expected-tag-object-sha256",
    expected.tagObjectSha256,
  ]);
  const report = JSON.parse(raw);
  if (
    report.kind !== "takoform.provider-release-provenance-verification@v1" ||
    report.tag !== descriptor.tag ||
    report.provenance !== names.provenance ||
    report.subjectCount !== 13 ||
    report.verified !== true ||
    report.signerFingerprint !== PROVIDER_SIGNER
  ) {
    throw new Error("provider release provenance verification report drifted");
  }
  return report;
}

function verifyProviderCandidate(
  context,
  root,
  { descriptor, runId, runAttempt, requestId, expectedCommit, tagObjectOid },
) {
  validateProviderIdentityLedger(context.repo, descriptor);
  const metadata = verifyCandidateRoot(root, {
    metadataProfile: "pretty-required-lf",
  });
  requireExactKeys(
    metadata,
    [
      "format",
      "repository",
      "workflowPath",
      "workflowRef",
      "runId",
      "attempt",
      "requestId",
      "tag",
      "sourceCommit",
      "toolingCommit",
      "tagObjectOid",
      "tagObjectSha256",
      "assetCount",
      "assets",
    ],
    "provider release candidate metadata",
  );
  if (
    metadata.format !== "takoform.provider-release-candidate.v1" ||
    metadata.repository !== GITHUB_REPOSITORY ||
    metadata.workflowPath !== ".github/workflows/release.yml" ||
    metadata.workflowRef !==
      `${GITHUB_REPOSITORY}/.github/workflows/release.yml@refs/tags/${descriptor.tag}` ||
    normalizedPositiveInteger(metadata.runId, "metadata runId") !== runId ||
    normalizedPositiveInteger(metadata.attempt, "metadata attempt") !==
      runAttempt ||
    metadata.requestId !== requestId ||
    metadata.tag !== descriptor.tag ||
    metadata.sourceCommit !== expectedCommit ||
    metadata.toolingCommit !== expectedCommit ||
    metadata.tagObjectOid !== tagObjectOid ||
    !SHA256.test(metadata.tagObjectSha256 ?? "") ||
    metadata.assetCount !== 15
  ) {
    throw new Error("provider release candidate metadata binding mismatch");
  }
  const tagObject = command(context, "git", ["cat-file", "tag", tagObjectOid]);
  if (sha256(Buffer.from(tagObject, "utf8")) !== metadata.tagObjectSha256) {
    throw new Error("provider candidate tag-object digest binding mismatch");
  }
  const names = providerAssetNames(descriptor);
  const assets = validateCandidateAssets(root, metadata, names.all);
  const assetRoot = join(root, "assets");
  verifyNamedChecksums(assetRoot, names.checksum, [
    ...names.archives,
    names.manifest,
  ]);
  verifyProviderSignature(context, assetRoot, names);
  verifyProviderReleaseProvenance(context, assetRoot, descriptor, {
    sourceCommit: expectedCommit,
    toolingCommit: expectedCommit,
    requestId,
    runId,
    runAttempt,
    tagObjectOid,
    tagObjectSha256: metadata.tagObjectSha256,
  });
  command(context, "go", [
    "-C",
    "cmd/provider-release",
    "run",
    ".",
    "verify-sbom",
    ...names.archives.map((name) => join(assetRoot, `${name}.spdx.json`)),
  ]);
  return assets;
}

function verifyProviderSignature(context, root, names) {
  withTemporaryDirectory("takoform-provider-gpg", (gpgHome) => {
    chmodSync(gpgHome, 0o700);
    const key = join(context.repo, "release/keys/provider-signing-key.asc");
    const inspect = command(context, "gpg", [
      "--homedir",
      gpgHome,
      "--batch",
      "--with-colons",
      "--import-options",
      "show-only",
      "--import",
      key,
    ]);
    const fingerprints = inspect
      .split("\n")
      .filter((line) => line.startsWith("fpr:"))
      .map((line) => line.split(":")[9])
      .filter(Boolean);
    if (fingerprints.length !== 1 || fingerprints[0] !== PROVIDER_SIGNER) {
      throw new Error("provider public signing key fingerprint drifted");
    }
    command(context, "gpg", ["--homedir", gpgHome, "--batch", "--import", key]);
    for (const [label, signature, subject] of [
      ["provider checksum", names.signature, names.checksum],
      [
        "provider release provenance",
        names.provenanceSignature,
        names.provenance,
      ],
    ]) {
      const status = command(
        context,
        "gpg",
        [
          "--homedir",
          gpgHome,
          "--batch",
          "--status-fd",
          "1",
          "--verify",
          join(root, signature),
          join(root, subject),
        ],
        { label: `verify ${label} signature` },
      );
      if (!status.includes(`[GNUPG:] VALIDSIG ${PROVIDER_SIGNER} `)) {
        throw new Error(
          `${label} signature did not return the pinned VALIDSIG`,
        );
      }
    }
  });
}

function publicAssets(root, names) {
  return new Map(
    names.map((name) => [
      name,
      { name, sha256: fileDigest(join(root, name)), path: join(root, name) },
    ]),
  );
}

export function providerReleaseBody(descriptor) {
  if (
    descriptor?.version !== PROVIDER_VERSION ||
    descriptor?.tag !== `v${PROVIDER_VERSION}` ||
    descriptor?.versioning?.portableApiVersion !== PROVIDER_HOST_API
  ) {
    throw new Error(
      `provider release body requires the exact v${PROVIDER_VERSION} stable Host API descriptor`,
    );
  }
  return (
    "Signed deterministic Takoform Provider v4.0.0 release. Provider publication does not publish, mature, activate, or make any Form commercially available.\n\nBreaking upgrade from Provider v3.0.0: Provider 4 registers only the publisher-selected roster and drops the 15 withdrawn aggregate Terraform resource types takoform_container_custom_domain, takoform_container_endpoint, takoform_container_revision, takoform_container_traffic, takoform_serverless_container_service, takoform_function, takoform_function_deployment, takoform_function_endpoint, takoform_function_version, takoform_pull_queue, takoform_message_schedule, takoform_table, takoform_topic, takoform_topic_subscription, and takoform_dense_vector_index. Existing state must stay pinned to Provider 3.0.0 or be explicitly forgotten or destroyed before upgrading; follow the fail-closed migration guide: " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/release/migrations/v3-to-v4.md` +
    "\n\nProvider v2.1.1 and Provider v1 remain separate migration boundaries and Provider 4 does not rewrite their state. Operators coming from those majors follow the retained boundary records: " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/release/migrations/v2-to-v3.md` +
    " and " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/release/migrations/v1-to-v2.md` +
    "\n\nThis provider release targets stable Host API `forms.takoform.com/v1` and the single versionless current Form family `edge.forms.takoform.com`. Provider SemVer, Host API, Form Family, Form definition, and Form Package versions are independent axes. The exact 17 Experimental 0.x FormRefs, provider-owned Terraform resource types, and package digests are selected from the `github.com/tako0614/takoform-forms` publisher set tag `forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c` at commit `3231633605b737ce5279d7fc020b4780568e7091` and locked in " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/${PROVIDER_IDENTITY_LEDGER}` +
    ". Published Provider 2.1.1 and 3.0.0 identities remain immutable history."
  );
}

function providerPrepare(context, options, descriptor) {
  const commit = assertCurrentProtectedMain(
    context,
    options["expected-commit"],
  );
  assertTagAbsent(context, descriptor.tag);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  ownerGateAndFence(context, commit);
  const dispatched = dispatchWorkflow(context, "provider-release-tag.yml", {
    tag: descriptor.tag,
    expected_commit: commit,
  });
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: PROVIDER_SURFACE,
    phase: "prepare",
    target: PROVIDER_ADDRESS,
    commit,
    tag: descriptor.tag,
    dispatchStatus: "DISPATCHED",
    status: "AWAITING_REVIEW",
    workflowRun: dispatched,
  });
}

function providerTag(context, options, descriptor) {
  const expectedCommit = options["expected-commit"];
  assertCurrentProtectedMain(context, expectedCommit);
  assertTagAbsent(context, descriptor.tag);
  ownerGateAndFence(context, expectedCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Author provider release tag",
      headSha: expectedCommit,
    },
  );
  return withTemporaryDirectory("takoform-provider-tag", (temporaryRoot) => {
    const preflight = downloadArtifact(
      context,
      options["run-id"],
      `provider-tag-preflight-${run.displayTitle}-${options["run-id"]}-${options["run-attempt"]}-${expectedCommit}`,
      join(temporaryRoot, "preflight"),
    );
    const signedTag = downloadArtifact(
      context,
      options["run-id"],
      `provider-signed-tag-${run.displayTitle}-${options["run-id"]}-${options["run-attempt"]}`,
      join(temporaryRoot, "signed-tag"),
    );
    let localObject = "";
    try {
      assertCurrentProtectedMain(context, expectedCommit);
      progress(context, "verify and materialize exact signed provider tag");
      command(
        context,
        "go",
        [
          "-C",
          "./cmd/provider-release",
          "run",
          ".",
          "verify-tag-artifact",
          "--artifact",
          signedTag,
          "--preflight-artifact",
          preflight,
          "--expected-run-id",
          options["run-id"],
          "--expected-run-attempt",
          options["run-attempt"],
          "--expected-request-id",
          run.displayTitle,
          "--expected-commit",
          expectedCommit,
          "--materialize-ref",
        ],
        { echo: true },
      );
      localObject = localTagOID(context, descriptor.tag);
      if (!localObject)
        throw new Error("provider tag verifier created no local ref");
      ownerGateAndFence(context, expectedCommit);
      assertReleaseAbsent(context, descriptor.tag);
      assertRegistryVersionAbsent(context, descriptor.version);
      pushExactTag(context, descriptor.tag, expectedCommit, localObject);
      ownerGateAndFence(context, expectedCommit);
      const dispatched = dispatchWorkflow(
        context,
        "release.yml",
        {
          tag: descriptor.tag,
          expected_commit: expectedCommit,
        },
        {
          headSha: expectedCommit,
          ref: descriptor.tag,
          headBranch: descriptor.tag,
        },
      );
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: PROVIDER_SURFACE,
        phase: "tag",
        commit: expectedCommit,
        tag: descriptor.tag,
        tagObject: localObject,
        reviewedRun: {
          id: options["run-id"],
          attempt: options["run-attempt"],
          url: run.url,
        },
        dispatchStatus: "DISPATCHED",
        status: "AWAITING_REVIEW",
        releaseCandidateRun: dispatched,
      });
    } catch (error) {
      reportTagFailure(context, descriptor.tag, localObject, {
        surface: PROVIDER_SURFACE,
        phase: "tag",
      });
      throw error;
    }
  });
}

function providerPublish(context, options, descriptor) {
  const expectedCommit = options["expected-commit"];
  const releaseBody = providerReleaseBody(descriptor);
  // Independent second copy of the body facts on purpose: the release body is
  // immutable once published, so the builder and the publisher assert the same
  // contract separately rather than sharing one statement of it.
  if (
    !releaseBody.includes("Provider v4.0.0") ||
    !releaseBody.includes(PROVIDER_HOST_API) ||
    !releaseBody.includes(
      "single versionless current Form family `edge.forms.takoform.com`",
    ) ||
    !releaseBody.includes(PROVIDER_IDENTITY_LEDGER) ||
    !releaseBody.includes("17 Experimental") ||
    !releaseBody.includes("release/migrations/v3-to-v4.md") ||
    !releaseBody.includes("release/migrations/v2-to-v3.md") ||
    !releaseBody.includes("release/migrations/v1-to-v2.md") ||
    !releaseBody.includes("Breaking upgrade from Provider v3.0.0") ||
    !releaseBody.includes("15 withdrawn aggregate Terraform resource types") ||
    !releaseBody.includes(
      "forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c",
    ) ||
    !releaseBody.includes(
      "Provider 2.1.1 and 3.0.0 identities remain immutable history",
    )
  ) {
    throw new Error(
      "provider release body omits the exact v4 publisher-set identity and migration contract",
    );
  }
  assertCurrentProtectedMain(context, expectedCommit);
  const localObject = localTagOID(context, descriptor.tag);
  if (!localObject)
    throw new Error(`local signed provider tag ${descriptor.tag} is missing`);
  assertExactRemoteTag(context, descriptor.tag, expectedCommit, localObject);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  ownerGateAndFence(context, expectedCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare provider release candidate",
      headSha: expectedCommit,
      headBranch: descriptor.tag,
    },
  );
  return withTemporaryDirectory(
    "takoform-provider-publish",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `provider-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const assets = verifyProviderCandidate(context, candidate, {
        descriptor,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit,
        tagObjectOid: localObject,
      });
      ownerGateAndFence(context, expectedCommit);
      assertRegistryVersionAbsent(context, descriptor.version);
      const release = publishReleaseLocally(context, {
        tag: descriptor.tag,
        assets,
        prerelease: descriptor.version.includes("-"),
        body: releaseBody,
        temporaryRoot,
        prePublishFence: () => {
          ownerGateAndFence(context, expectedCommit);
          assertRegistryVersionAbsent(context, descriptor.version);
        },
      });
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: PROVIDER_SURFACE,
        phase: "publish",
        target: `github-release:${GITHUB_REPOSITORY}/${descriptor.tag}`,
        commit: expectedCommit,
        tag: descriptor.tag,
        candidateRun: {
          id: options["run-id"],
          attempt: options["run-attempt"],
          url: run.url,
        },
        releaseId: release.id,
        releaseUrl: release.html_url,
        assetDigests: Object.fromEntries(
          [...assets].map(([name, asset]) => [name, asset.sha256]),
        ),
        productionReadback: "EXACT_IMMUTABLE_RELEASE",
        status: "VERIFIED",
      });
    },
  );
}

function providerRecoveryMutationFence(
  context,
  {
    descriptor,
    releaseCommit,
    recoveryCommit,
    expectedObject,
    releaseId,
    label,
  },
) {
  const currentMain = ownerGateAndFence(context, recoveryCommit);
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: currentMain,
    label,
  });
  const tag = assertExactSignedProviderTag(context, {
    tag: descriptor.tag,
    expectedCommit: releaseCommit,
    expectedObject,
  });
  assertRegistryVersionAbsent(context, descriptor.version);
  assertReleaseImmutabilityEnabled(context);
  if (releaseId === undefined) {
    assertReleaseAbsent(context, descriptor.tag);
  } else {
    assertUniqueReleaseIdentity(context, descriptor.tag, releaseId, true);
  }
  return { currentMain, tag };
}

function providerRecoverTagOnly(context, options, descriptor) {
  const releaseCommit = options["expected-release-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const initialMain = assertCurrentProtectedMain(context, recoveryCommit);
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: initialMain,
    label: "provider tag-only reviewed recovery fence",
  });
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare provider release candidate",
      headSha: releaseCommit,
      headBranch: descriptor.tag,
    },
  );
  return withTemporaryDirectory(
    "takoform-provider-tag-only-recovery",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `provider-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const assets = verifyProviderCandidate(context, candidate, {
        descriptor,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit: releaseCommit,
        tagObjectOid: expectedObject,
      });
      try {
        providerRecoveryMutationFence(context, {
          descriptor,
          releaseCommit,
          recoveryCommit,
          expectedObject,
          label: "provider tag-only pre-draft recovery fence",
        });
        const release = publishReleaseLocally(context, {
          tag: descriptor.tag,
          assets,
          prerelease: descriptor.version.includes("-"),
          body: providerReleaseBody(descriptor),
          temporaryRoot,
          strictIdentity: true,
          prePublishFence: (releaseId) =>
            providerRecoveryMutationFence(context, {
              descriptor,
              releaseCommit,
              recoveryCommit,
              expectedObject,
              releaseId,
              label: "provider tag-only pre-publication recovery fence",
            }),
        });
        assertExactSignedProviderTag(context, {
          tag: descriptor.tag,
          expectedCommit: releaseCommit,
          expectedObject,
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: PROVIDER_SURFACE,
          phase: "recover-tag-only",
          target: `github-release:${GITHUB_REPOSITORY}/${descriptor.tag}`,
          commit: recoveryCommit,
          releaseCommit,
          candidateToolingCommit: releaseCommit,
          recoveryCommit,
          tag: descriptor.tag,
          tagObject: expectedObject,
          signerFingerprint: PROVIDER_SIGNER,
          recoveredFrom: "EXACT_SIGNED_TAG_PRESENT_RELEASE_ABSENT",
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, descriptor.tag, expectedObject, {
          surface: PROVIDER_SURFACE,
          phase: "recover-tag-only",
        });
        throw error;
      }
    },
  );
}

function providerRecoverDraft(context, options, descriptor) {
  const releaseCommit = options["expected-release-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const releaseId = Number(options["release-id"]);
  const initialMain = assertCurrentProtectedMain(context, recoveryCommit);
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: initialMain,
    label: "provider retained-draft reviewed recovery fence",
  });
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare provider release candidate",
      headSha: releaseCommit,
      headBranch: descriptor.tag,
    },
  );
  return withTemporaryDirectory(
    "takoform-provider-draft-recovery",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `provider-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const assets = verifyProviderCandidate(context, candidate, {
        descriptor,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit: releaseCommit,
        tagObjectOid: expectedObject,
      });
      try {
        providerRecoveryMutationFence(context, {
          descriptor,
          releaseCommit,
          recoveryCommit,
          expectedObject,
          releaseId,
          label: "provider retained-draft mutation recovery fence",
        });
        const release = resumeDraftReleaseLocally(context, {
          releaseId,
          tag: descriptor.tag,
          assets,
          prerelease: descriptor.version.includes("-"),
          body: providerReleaseBody(descriptor),
          temporaryRoot,
          surface: PROVIDER_SURFACE,
          prePublishFence: (retainedReleaseId) =>
            providerRecoveryMutationFence(context, {
              descriptor,
              releaseCommit,
              recoveryCommit,
              expectedObject,
              releaseId: retainedReleaseId,
              label: "provider retained-draft pre-publication recovery fence",
            }),
        });
        assertExactSignedProviderTag(context, {
          tag: descriptor.tag,
          expectedCommit: releaseCommit,
          expectedObject,
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: PROVIDER_SURFACE,
          phase: "recover-draft",
          target: `github-release:${GITHUB_REPOSITORY}/${descriptor.tag}`,
          commit: recoveryCommit,
          releaseCommit,
          candidateToolingCommit: releaseCommit,
          recoveryCommit,
          tag: descriptor.tag,
          tagObject: expectedObject,
          signerFingerprint: PROVIDER_SIGNER,
          recoveredFrom: "EXACT_RETAINED_DRAFT",
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, descriptor.tag, expectedObject, {
          surface: PROVIDER_SURFACE,
          phase: "recover-draft",
        });
        throw error;
      }
    },
  );
}

function revocationAssetNames(tag) {
  const match = REVOCATION_TAG.exec(tag);
  if (!match) {
    throw new Error("--tag must match forms/revocations/v<canonical-semver>");
  }
  const version = match[1];
  const base = `takoform-form-revocation_${version}`;
  return {
    base,
    version,
    subject: `${base}_checkpoint.json`,
    bundle: `${base}_checkpoint.sigstore.json`,
    all: [
      `${base}_checkpoint.json`,
      `${base}_checkpoint.sigstore.json`,
      `${base}_statement.json`,
      `${base}_provenance.intoto.json`,
      "release-manifest.json",
      "SHA256SUMS",
    ].sort(),
  };
}

function verifySigstoreSubject(
  context,
  root,
  subject,
  bundle,
  workflow,
  trustedRoot = join(context.repo, TRUSTED_ROOT),
) {
  verifyLocalReleaseToolchain(context);
  command(context, "cosign", [
    "verify-blob",
    join(root, subject),
    "--bundle",
    join(root, bundle),
    "--trusted-root",
    trustedRoot,
    "--certificate-identity",
    `https://github.com/${GITHUB_REPOSITORY}/${workflow}@refs/heads/main`,
    "--certificate-oidc-issuer",
    OIDC_ISSUER,
  ]);
}

function verifyDeepAssetReport(context, root, report, expected) {
  if (
    report.format !== expected.format ||
    report.semanticStatus !== "verified" ||
    report.cryptographicStatus !== "external-required" ||
    report.tag !== expected.tag ||
    report.sourceCommit !== expected.sourceCommit ||
    report.toolingCommit !== expected.toolingCommit ||
    report.trustedRoot?.path !== resolve(context.repo, TRUSTED_ROOT) ||
    report.trustedRoot?.sha256 !== expected.trustedRootDigest ||
    !Array.isArray(report.assets)
  ) {
    throw new Error(`${expected.label} deep semantic report identity mismatch`);
  }
  const actual = report.assets
    .map((asset) => {
      requireExactKeys(
        asset,
        ["name", "sha256", "size"],
        `${expected.label} deep asset`,
      );
      const path = join(root, asset.name);
      if (
        !expected.names.includes(asset.name) ||
        !SHA256.test(asset.sha256 ?? "") ||
        asset.sha256 !== fileDigest(path) ||
        !Number.isSafeInteger(asset.size) ||
        asset.size < 0 ||
        asset.size !== lstatSync(path).size
      ) {
        throw new Error(
          `${expected.label} deep asset report mismatch: ${asset.name}`,
        );
      }
      return asset.name;
    })
    .sort();
  if (JSON.stringify(actual) !== JSON.stringify([...expected.names].sort())) {
    throw new Error(`${expected.label} deep asset inventory mismatch`);
  }
}

function withTrustedRootAtCommit(context, toolingCommit, operation) {
  return withTemporaryDirectory("takoform-trusted-root", (temporaryRoot) => {
    const raw = command(context, "git", [
      "show",
      `${toolingCommit}:${TRUSTED_ROOT}`,
    ]);
    const path = join(temporaryRoot, "trusted-root.json");
    writeFileSync(path, raw, { flag: "wx", mode: 0o600 });
    return operation(path, fileDigest(path));
  });
}

function expectedFormTagObject(
  context,
  { tag, sourceCommit, requestId, runId, runAttempt },
) {
  const epoch = git(context, "show", "-s", "--format=%ct", sourceCommit);
  if (!/^(?:0|[1-9][0-9]*)$/u.test(epoch)) {
    throw new Error("tag source commit timestamp is not a Unix epoch");
  }
  return (
    `object ${sourceCommit}\n` +
    "type commit\n" +
    `tag ${tag}\n` +
    "tagger Takoform Form Package Revocation " +
    `<release@takoform.invalid> ${epoch} +0000\n\n` +
    `Takoform Form Package revocation checkpoint ${tag}\n\n` +
    `source-commit: ${sourceCommit}\n` +
    `request-id: ${requestId}\n` +
    `workflow-run: https://github.com/${GITHUB_REPOSITORY}/actions/runs/` +
    `${runId}/attempts/${runAttempt}\n`
  );
}

function verifyTagObjectWorkflowBinding(
  context,
  root,
  metadata,
  runId,
  runAttempt,
) {
  const text = readFileSync(join(root, "tag-object"), "utf8");
  const expected = expectedFormTagObject(context, {
    tag: metadata.tag,
    sourceCommit: metadata.sourceCommit,
    requestId: metadata.requestId,
    runId,
    runAttempt,
  });
  if (text !== expected) {
    throw new Error(
      "tag object bytes differ from the exact deterministic workflow object",
    );
  }
}

function verifyRevocationSemanticClosure(
  context,
  root,
  tag,
  sourceCommit,
  toolingCommit,
  trustedRoot,
) {
  const names = revocationAssetNames(tag);
  const raw = command(context, "go", [
    "run",
    "./cmd/form-package-release",
    "verify-revocation-directory",
    "--asset-root",
    root,
    "--source-root",
    context.repo,
    "--tag",
    tag,
    "--source-commit",
    sourceCommit,
    "--tooling-commit",
    toolingCommit,
    "--trusted-root",
    trustedRoot,
  ]);
  const report = JSON.parse(raw);
  if (
    report.version !== names.version ||
    !Number.isSafeInteger(report.checkpointSequence) ||
    report.checkpointSequence <= 0 ||
    !SHA256.test(report.checkpointDigest ?? "") ||
    !SHA256.test(report.packageDigest ?? "") ||
    !report.formRef ||
    typeof report.formRef !== "object"
  ) {
    throw new Error("revocation deep semantic report binding mismatch");
  }
  return report;
}

function verifyRevocationPublicAssets(context, root, tag, { metadata } = {}) {
  const names = revocationAssetNames(tag);
  const actual = listRegularFiles(root);
  if (JSON.stringify(actual) !== JSON.stringify(names.all)) {
    throw new Error(`revocation asset inventory differs: ${actual.join(", ")}`);
  }
  const manifest = readJSON(
    join(root, "release-manifest.json"),
    "revocation release manifest",
  );
  if (
    manifest.schemaVersion !== 1 ||
    manifest.releaseType !== "form-package-revocation" ||
    manifest.tag !== tag ||
    manifest.sourceRepository !== `github.com/${GITHUB_REPOSITORY}` ||
    !COMMIT.test(manifest.sourceCommit ?? "") ||
    !COMMIT.test(manifest.toolingCommit ?? "") ||
    manifest.workflow !== ".github/workflows/form-package-revocation.yml" ||
    manifest.packageVersion !== names.version ||
    manifest.signedSubject !== names.subject ||
    manifest.signatureBundle !== names.bundle ||
    manifest.publicationReady !== true ||
    !Array.isArray(manifest.publicationBlockers) ||
    manifest.publicationBlockers.length !== 0 ||
    !Array.isArray(manifest.assets) ||
    manifest.assets.length !== 4
  ) {
    throw new Error("revocation manifest identity/closure mismatch");
  }
  if (
    metadata &&
    (manifest.sourceCommit !== metadata.sourceCommit ||
      manifest.toolingCommit !== metadata.toolingCommit)
  ) {
    throw new Error(
      "revocation manifest commit binding differs from candidate",
    );
  }
  const manifestNames = manifest.assets.map((asset) => asset.name).sort();
  for (const asset of manifest.assets) {
    if (
      !SHA256.test(asset.digest ?? "") ||
      fileDigest(join(root, asset.name)) !== asset.digest
    ) {
      throw new Error(`revocation manifest asset mismatch: ${asset.name}`);
    }
  }
  verifyNamedChecksums(root, "SHA256SUMS", [
    ...manifestNames,
    "release-manifest.json",
  ]);
  withTrustedRootAtCommit(
    context,
    manifest.toolingCommit,
    (trustedRoot, trustedRootDigest) => {
      const deepReport = verifyRevocationSemanticClosure(
        context,
        root,
        tag,
        manifest.sourceCommit,
        manifest.toolingCommit,
        trustedRoot,
      );
      verifyDeepAssetReport(context, root, deepReport, {
        format: "takoform.form-package-revocation-directory-verification@v1",
        label: "revocation",
        names: names.all,
        tag,
        sourceCommit: manifest.sourceCommit,
        toolingCommit: manifest.toolingCommit,
        trustedRootDigest,
      });
      verifySigstoreSubject(
        context,
        root,
        names.subject,
        names.bundle,
        ".github/workflows/form-package-revocation.yml",
        trustedRoot,
      );
    },
  );
  return manifest;
}

function verifyRevocationCandidate(
  context,
  root,
  { tag, runId, runAttempt, requestId, expectedCommit, toolingCommit },
) {
  const metadata = verifyCandidateRoot(root, {
    tagObject: true,
    metadataProfile: "pretty-required-lf",
  });
  requireExactKeys(
    metadata,
    [
      "format",
      "repository",
      "workflowPath",
      "workflowRef",
      "runId",
      "runAttempt",
      "sourceRef",
      "tag",
      "sourceCommit",
      "toolingCommit",
      "objectFormat",
      "tagObjectOid",
      "tagObjectSha256",
      "requestId",
      "assetCount",
      "assets",
    ],
    "revocation candidate metadata",
  );
  if (
    metadata.format !== "takoform.form-package-revocation-candidate@v1" ||
    metadata.repository !== GITHUB_REPOSITORY ||
    metadata.workflowPath !== ".github/workflows/form-package-revocation.yml" ||
    metadata.workflowRef !==
      `${GITHUB_REPOSITORY}/.github/workflows/form-package-revocation.yml@refs/heads/main` ||
    metadata.sourceRef !== "refs/heads/main" ||
    metadata.requestId !== requestId ||
    normalizedPositiveInteger(metadata.runId, "metadata runId") !== runId ||
    normalizedPositiveInteger(metadata.runAttempt, "metadata runAttempt") !==
      runAttempt ||
    metadata.tag !== tag ||
    metadata.sourceCommit !== expectedCommit ||
    metadata.toolingCommit !== toolingCommit ||
    metadata.assetCount !== 6
  ) {
    throw new Error("revocation candidate metadata binding mismatch");
  }
  const names = revocationAssetNames(tag);
  const assets = validateCandidateAssets(root, metadata, names.all);
  verifyRevocationPublicAssets(context, join(root, "assets"), tag, {
    metadata,
  });
  verifyTagObjectWorkflowBinding(context, root, metadata, runId, runAttempt);
  return { assets, metadata };
}

function readExactPublicRelease(
  context,
  tag,
  expectedNames,
  temporaryRoot,
  { prerelease = false } = {},
) {
  const release = getRelease(context, tag);
  const output = join(temporaryRoot, "release-readback");
  mkdirSync(output);
  command(context, "gh", [
    "release",
    "download",
    tag,
    "--repo",
    GITHUB_REPOSITORY,
    "--dir",
    output,
  ]);
  const actualNames = listRegularFiles(output);
  if (
    JSON.stringify(actualNames) !== JSON.stringify([...expectedNames].sort())
  ) {
    throw new Error(
      `public release inventory differs: ${actualNames.join(", ")}`,
    );
  }
  const assets = publicAssets(output, actualNames);
  validateReleaseReadback(release, tag, assets, { prerelease });
  return { release, output, assets };
}

function reportTagFailure(
  context,
  tag,
  materializedObject,
  { surface = FORM_SURFACE, phase } = {},
) {
  let observed = {
    local: null,
    localReadable: false,
    remote: null,
    remoteReadable: false,
    mutationState: "LOCAL_AND_REMOTE_UNREADABLE",
  };
  try {
    observed = observeTagFailureState(context, tag);
  } catch {
    // Failure reporting is diagnostic only and must never replace the original
    // release failure.
  }
  const uncertain =
    !observed.localReadable ||
    !observed.remoteReadable ||
    Boolean(
      materializedObject ||
      observed.local ||
      observed.remote?.object ||
      observed.remote?.commit,
    );
  const mutationState =
    materializedObject && observed.mutationState === "UNCHANGED"
      ? "INDETERMINATE_AFTER_TAG_MATERIALIZATION"
      : observed.mutationState;
  try {
    context.stderr.write(
      `${JSON.stringify({
        kind: "takos.deploy-failure@v1",
        surface,
        ...(phase ? { phase } : {}),
        tag,
        materializedTagObject: materializedObject || null,
        localTagObject: observed.local,
        localReadable: observed.localReadable,
        remoteTag: observed.remote,
        remoteReadable: observed.remoteReadable,
        mutationState,
        instruction: uncertain
          ? "inspect the exact local/remote ref and release state; do not delete, overwrite, or retry blindly"
          : "public tag/release is unchanged",
      })}\n`,
    );
  } catch {
    // Preserve the primary exception even if stderr itself is unavailable.
  }
}

function requireRevocationSource(context, tag, expectedCommit) {
  const names = revocationAssetNames(tag);
  const toolingCommit = assertCurrentProtectedMain(context, expectedCommit, {
    exact: false,
  });
  for (const path of [
    join(context.repo, "forms/revocations", `${names.version}.json`),
    join(
      context.repo,
      "forms/revocations/checkpoints",
      `${names.version}.json`,
    ),
  ]) {
    if (!existsSync(path) || !lstatSync(path).isFile()) {
      throw new Error(`revocation source is missing or non-regular: ${path}`);
    }
  }
  return { commit: expectedCommit, toolingCommit, names };
}

function revocationPrepare(context, options) {
  const { commit, toolingCommit, names } = requireRevocationSource(
    context,
    options.tag,
    options["expected-commit"],
  );
  assertTagAbsent(context, options.tag);
  assertReleaseAbsent(context, options.tag);
  ownerGateAndFence(context, toolingCommit);
  const dispatched = dispatchWorkflow(
    context,
    "form-package-revocation.yml",
    {
      tag: options.tag,
      expected_commit: commit,
    },
    { headSha: toolingCommit },
  );
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: FORM_SURFACE,
    phase: "prepare-revocation",
    commit,
    toolingCommit,
    tag: options.tag,
    checkpointVersion: names.version,
    dispatchStatus: "DISPATCHED",
    status: "AWAITING_REVIEW",
    workflowRun: dispatched,
  });
}

function revocationPublish(context, options) {
  const {
    commit,
    toolingCommit: initialMain,
    names,
  } = requireRevocationSource(context, options.tag, options["expected-commit"]);
  assertTagAbsent(context, options.tag);
  assertReleaseAbsent(context, options.tag);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare signed Form Package revocation checkpoint",
    },
  );
  const toolingCommit = run.headSha;
  assertFormReleaseAuthorityFence(context, {
    sourceCommit: commit,
    toolingCommit,
    currentMain: initialMain,
    label: "revocation reviewed candidate fence",
  });
  return withTemporaryDirectory(
    "takoform-revocation-publish",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `form-package-revocation-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const verified = verifyRevocationCandidate(context, candidate, {
        tag: options.tag,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit: commit,
        toolingCommit,
      });
      let tagObject = "";
      try {
        const mutationMain = ownerGateAndFence(context);
        assertFormReleaseAuthorityFence(context, {
          sourceCommit: commit,
          toolingCommit,
          currentMain: mutationMain,
          label: "revocation tag mutation fence",
        });
        tagObject = ensureCandidateTagPublished(
          context,
          options.tag,
          commit,
          candidate,
          verified.metadata,
        );
        const releaseMain = ownerGateAndFence(context);
        assertFormReleaseAuthorityFence(context, {
          sourceCommit: commit,
          toolingCommit,
          currentMain: releaseMain,
          label: "revocation release mutation fence",
        });
        const release = publishReleaseLocally(context, {
          tag: options.tag,
          assets: verified.assets,
          body: "Append-only Takoform Form Package security revocation checkpoint. Verify the signed cumulative checkpoint before enforcement.",
          temporaryRoot,
          prePublishFence: () => {
            const publishMain = ownerGateAndFence(context);
            assertFormReleaseAuthorityFence(context, {
              sourceCommit: commit,
              toolingCommit,
              currentMain: publishMain,
              label: "revocation pre-publish fence",
            });
          },
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: FORM_SURFACE,
          phase: "publish-revocation",
          commit,
          tag: options.tag,
          checkpointVersion: names.version,
          tagObject,
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...verified.assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, options.tag, tagObject);
        throw error;
      }
    },
  );
}

function revocationVerify(context, options) {
  const names = revocationAssetNames(options.tag);
  const currentMain = assertCurrentProtectedMain(
    context,
    options["expected-commit"],
    {
      exact: false,
    },
  );
  assertExactRemoteTag(context, options.tag, options["expected-commit"]);
  return withTemporaryDirectory(
    "takoform-revocation-verify",
    (temporaryRoot) => {
      const live = readExactPublicRelease(
        context,
        options.tag,
        names.all,
        temporaryRoot,
      );
      const manifest = verifyRevocationPublicAssets(
        context,
        live.output,
        options.tag,
      );
      if (manifest.sourceCommit !== options["expected-commit"]) {
        throw new Error("public revocation source commit differs");
      }
      assertCommitAncestor(
        context,
        manifest.sourceCommit,
        manifest.toolingCommit,
        "public revocation source/tooling binding",
      );
      assertCommitAncestor(
        context,
        manifest.toolingCommit,
        currentMain,
        "public revocation tooling/current-main binding",
      );
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: FORM_SURFACE,
        phase: "verify-revocation",
        tag: options.tag,
        commit: manifest.sourceCommit,
        toolingCommit: manifest.toolingCommit,
        releaseId: live.release.id,
        releaseUrl: live.release.html_url,
        assetDigests: Object.fromEntries(
          [...live.assets].map(([name, asset]) => [name, asset.sha256]),
        ),
        status: "VERIFIED",
      });
    },
  );
}

// Narrow executable seams for the owner-local release safety tests. Production
// callers use runReleaseSurface; these hooks keep tests off live git/GitHub
// authority while exercising the same mutation and verification code paths.
export const releaseDeployTestHooks = Object.freeze({
  assertExactSignedProviderTag,
  assertExactSpecificationTag,
  assertPinnedProviderGpgVerification,
  assertReleaseImmutabilityEnabled,
  assertFormReleaseAuthorityFence,
  assertProviderRecoveryFence,
  assertSpecificationC2Fence,
  assertSpecificationRecoveryFence,
  assertReleaseAbsent,
  assertRegistryVersionAbsent,
  command,
  dispatchWorkflow,
  expectedFormTagObject,
  githubCommandEnvironment,
  githubUploadEnvironment,
  gitPushEnvironment,
  isCanonicalOrigin,
  normalGitEnvironment,
  ownerGateAndFence,
  runOwnerCheck,
  ownerGateToolchain,
  verifyOwnerGateToolchain,
  observeTagFailureState,
  parseCandidateMetadata,
  parseCanonicalCandidateMetadata,
  parsePrettyCandidateMetadata,
  publishReleaseLocally,
  providerAssetNames,
  readProviderDescriptor,
  validateProviderIdentityLedger,
  pushExactTag,
  reportTagFailure,
  resumeDraftReleaseLocally,
  materializeSpecificationTag,
  reconstructCandidateTagObject,
  recoveryReadOnlyGitEnvironment,
  requireSuccessfulRun,
  specificationPublicationInput,
  specificationReleaseBody,
  validateReleaseReadback,
  validateDraftBeforePublication,
  verifyChecksumClosure,
  verifyProviderCandidate,
  verifyProviderReleaseProvenance,
  verifyProviderSignature,
  verifyRevocationSemanticClosure,
  verifyTagObjectWorkflowBinding,
});

#!/usr/bin/env bun

// Validate the append-only numbered Specification release ledger.
//
// Readiness and release are deliberately separate. The publication-evidence
// record proves that one committed source snapshot, its complete multi-family
// corpus, and the manifest-owned reference suite agree. This ledger records a
// numbered Specification only after those three prerequisites are closed.
// Neither operation promotes a current 0.x Form, publishes a Form Package, or
// changes the independent official Provider release stream.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  CONFORMANCE_SUITE_PATH,
  FAMILY_INDEX_PATH,
  HOST_API_LANE,
  SPECIFICATION_PREREQUISITES,
  SPECIFICATION_TRACK,
  assertPublicationEvidenceReady,
  canonicalJson,
  loadPublicationEvidence,
  validatePublicationEvidence,
} from "./publication-evidence.mjs";

export const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
export const LEDGER_PATH = "release/specification-releases.json";
export const LEDGER_KIND = "takoform.specification-releases@v1";
export const RELEASE_VERSION = "1.0";
export const FORM_MATURITY_EFFECT = "none-current-formrefs-remain-exact";
export const PROVIDER_EFFECT = "independent-non-normative-sample";

const FULL_SHA = /^[0-9a-f]{40}$/u;
const SHA256 = /^[0-9a-f]{64}$/u;
const NUMBERED_VERSION = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/u;

export const EXPECTED_CANDIDATE = Object.freeze({
  version: RELEASE_VERSION,
  title: "Takoform Specification 1.0",
  track: SPECIFICATION_TRACK,
  hostApiLane: HOST_API_LANE,
  familyIndexPath: FAMILY_INDEX_PATH,
  conformanceSuitePath: CONFORMANCE_SUITE_PATH,
  prerequisites: [...SPECIFICATION_PREREQUISITES],
  formMaturityEffect: FORM_MATURITY_EFFECT,
  providerEffect: PROVIDER_EFFECT,
});

function fail(message) {
  throw new Error(`specification release: ${message}`);
}

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function same(left, right) {
  return canonicalJson(left) === canonicalJson(right);
}

function git(root, args) {
  try {
    return execFileSync("git", args, {
      cwd: root,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 64 * 1024 * 1024,
    }).trim();
  } catch {
    return null;
  }
}

function loadLedger(root = repositoryRoot) {
  const absolute = path.join(root, LEDGER_PATH);
  let ledger;
  try {
    ledger = JSON.parse(readFileSync(absolute, "utf8"));
  } catch (error) {
    fail(`${LEDGER_PATH} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
  return ledger;
}

function evidenceDigest(value) {
  return createHash("sha256").update(canonicalJson(value)).digest("hex");
}

export function releaseFromEvidence(document) {
  const baseline = document?.candidateBaseline;
  const specification = document?.evidence?.specification;
  if (
    !isRecord(baseline) ||
    !isRecord(baseline.familyIndex) ||
    !isRecord(baseline.conformanceSuite) ||
    !isRecord(specification?.sourceSnapshot) ||
    !isRecord(specification?.candidateCorpus) ||
    !isRecord(specification?.referenceConformance)
  ) {
    fail("Specification 1.0 evidence is not complete");
  }
  return {
    version: RELEASE_VERSION,
    title: EXPECTED_CANDIDATE.title,
    track: SPECIFICATION_TRACK,
    hostApiLane: HOST_API_LANE,
    sourceCommit: baseline.commit,
    familyIndex: baseline.familyIndex,
    conformanceSuite: baseline.conformanceSuite,
    sourceSnapshotSha256: evidenceDigest(specification.sourceSnapshot),
    candidateCorpusSha256: evidenceDigest(specification.candidateCorpus),
    referenceConformanceSha256: evidenceDigest(specification.referenceConformance),
    prerequisites: [...SPECIFICATION_PREREQUISITES],
    formMaturityEffect: FORM_MATURITY_EFFECT,
    providerEffect: PROVIDER_EFFECT,
  };
}

function validateDigestRecord(value, expectedPath, context, problems) {
  if (
    !isRecord(value) ||
    value.path !== expectedPath ||
    typeof value.sha256 !== "string" ||
    !SHA256.test(value.sha256)
  ) {
    problems.push(`${context} must pin ${expectedPath} with a lowercase SHA-256 digest`);
  }
}

export function validateReleaseShape(release) {
  const problems = [];
  if (!isRecord(release)) return ["release entry must be an object"];
  const expectedKeys = [
    "version",
    "title",
    "track",
    "hostApiLane",
    "sourceCommit",
    "familyIndex",
    "conformanceSuite",
    "sourceSnapshotSha256",
    "candidateCorpusSha256",
    "referenceConformanceSha256",
    "prerequisites",
    "formMaturityEffect",
    "providerEffect",
  ].sort();
  const actualKeys = Object.keys(release).sort();
  if (!same(actualKeys, expectedKeys)) {
    problems.push(`${release.version ?? "release"}: keys must be exactly ${expectedKeys.join(", ")}`);
  }
  if (!NUMBERED_VERSION.test(release.version ?? "")) {
    problems.push(`${release.version ?? "release"}: version must be a numbered major.minor identity`);
  }
  if (release.title !== EXPECTED_CANDIDATE.title) {
    problems.push(`${release.version ?? "release"}: title must be ${EXPECTED_CANDIDATE.title}`);
  }
  if (release.track !== SPECIFICATION_TRACK || release.hostApiLane !== HOST_API_LANE) {
    problems.push(`${release.version ?? "release"}: must identify ${SPECIFICATION_TRACK} on ${HOST_API_LANE}`);
  }
  if (typeof release.sourceCommit !== "string" || !FULL_SHA.test(release.sourceCommit)) {
    problems.push(`${release.version ?? "release"}: sourceCommit must be a full lowercase commit SHA`);
  }
  validateDigestRecord(release.familyIndex, FAMILY_INDEX_PATH, `${release.version}.familyIndex`, problems);
  validateDigestRecord(
    release.conformanceSuite,
    CONFORMANCE_SUITE_PATH,
    `${release.version}.conformanceSuite`,
    problems,
  );
  for (const field of [
    "sourceSnapshotSha256",
    "candidateCorpusSha256",
    "referenceConformanceSha256",
  ]) {
    if (typeof release[field] !== "string" || !SHA256.test(release[field])) {
      problems.push(`${release.version ?? "release"}.${field} must be a lowercase SHA-256 digest`);
    }
  }
  if (!same(release.prerequisites, SPECIFICATION_PREREQUISITES)) {
    problems.push(
      `${release.version ?? "release"}: prerequisites must be exactly ${SPECIFICATION_PREREQUISITES.join(", ")}`,
    );
  }
  if (release.formMaturityEffect !== FORM_MATURITY_EFFECT) {
    problems.push(`${release.version ?? "release"}: must not promote or relabel current FormRefs`);
  }
  if (release.providerEffect !== PROVIDER_EFFECT) {
    problems.push(`${release.version ?? "release"}: must keep the Provider release stream independent`);
  }
  return problems;
}

function versionTuple(version) {
  const match = NUMBERED_VERSION.exec(version ?? "");
  return match === null ? null : [Number(match[1]), Number(match[2])];
}

function compareVersions(left, right) {
  const a = versionTuple(left);
  const b = versionTuple(right);
  if (a === null || b === null) return 0;
  return a[0] - b[0] || a[1] - b[1];
}

export function validateCommittedHistory(currentLedger, history) {
  const problems = [];
  const current = Array.isArray(currentLedger?.releases) ? currentLedger.releases : [];
  for (const revision of history) {
    const historical = revision.ledger?.releases;
    if (!Array.isArray(historical)) {
      problems.push(`committed ledger ${revision.commit.slice(0, 8)} has no releases array`);
      continue;
    }
    for (const [index, release] of historical.entries()) {
      if (current[index]?.version !== release?.version) {
        problems.push(`committed release ${release?.version ?? "unknown"} was deleted or reordered`);
      } else if (!same(current[index], release)) {
        problems.push(`committed release ${release.version} was mutated; releases are append-only`);
      }
    }
  }
  return problems;
}

function committedHistory(root) {
  const commits = git(root, ["log", "--full-history", "--format=%H", "HEAD", "--", LEDGER_PATH]);
  if (commits === null || commits === "") return [];
  return commits.split("\n").map((commit) => {
    const source = git(root, ["show", `${commit}:${LEDGER_PATH}`]);
    if (source === null) fail(`cannot read ${LEDGER_PATH} at ${commit}`);
    try {
      return { commit, ledger: JSON.parse(source) };
    } catch (error) {
      fail(`committed ${LEDGER_PATH} at ${commit} is invalid JSON: ${error instanceof Error ? error.message : String(error)}`);
    }
  });
}

export function validateLedger(ledger, history = []) {
  const problems = [];
  if (!isRecord(ledger)) return ["ledger must be an object"];
  const expectedKeys = ["kind", "policy", "candidate", "releases"].sort();
  if (!same(Object.keys(ledger).sort(), expectedKeys)) {
    problems.push(`ledger keys must be exactly ${expectedKeys.join(", ")}`);
  }
  if (ledger.kind !== LEDGER_KIND) problems.push(`kind must be ${LEDGER_KIND}`);
  if (typeof ledger.policy !== "string" || ledger.policy.trim() === "") {
    problems.push("policy must be a non-empty string");
  }
  if (!same(ledger.candidate, EXPECTED_CANDIDATE)) {
    problems.push("candidate must state the exact Specification 1.0 track without Form or Provider promotion");
  }
  if (!Array.isArray(ledger.releases)) {
    problems.push("releases must be an array");
  } else {
    const versions = new Set();
    for (const [index, release] of ledger.releases.entries()) {
      problems.push(...validateReleaseShape(release).map((problem) => `releases[${index}]: ${problem}`));
      if (versions.has(release?.version)) problems.push(`release ${release.version} is duplicated`);
      versions.add(release?.version);
      if (index > 0 && compareVersions(ledger.releases[index - 1].version, release?.version) >= 0) {
        problems.push(`release ${release?.version} is not appended in numeric order`);
      }
    }
  }
  problems.push(...validateCommittedHistory(ledger, history));
  return problems;
}

function expectedReadyRelease(root) {
  const document = loadPublicationEvidence(root);
  const report = validatePublicationEvidence(document, {
    repositoryRoot: root,
    releaseTrack: SPECIFICATION_TRACK,
  });
  assertPublicationEvidenceReady(report, SPECIFICATION_TRACK);
  return releaseFromEvidence(document);
}

export function run(mode, root = repositoryRoot) {
  if (!["--check", "--assert-ready"].includes(mode)) {
    fail("usage: bun scripts/specification-release.mjs --check|--assert-ready");
  }
  const ledger = loadLedger(root);
  const problems = validateLedger(ledger, committedHistory(root));
  if (problems.length !== 0) {
    for (const problem of problems) process.stderr.write(`- ${problem}\n`);
    fail(`${problems.length} ledger problem(s)`);
  }

  if (ledger.releases.length !== 0) {
    const ready = expectedReadyRelease(root);
    const recorded = ledger.releases.find((release) => release.version === RELEASE_VERSION);
    if (recorded === undefined || !same(recorded, ready)) {
      fail("a recorded Specification 1.0 release must equal the exact committed publication evidence");
    }
  }

  if (mode === "--assert-ready") {
    expectedReadyRelease(root);
  }
  process.stdout.write(
    `specification release ledger OK: Specification 1.0 candidate; ${ledger.releases.length} numbered release(s) recorded\n`,
  );
  return ledger;
}

if (import.meta.main) {
  run(process.argv[2]);
}

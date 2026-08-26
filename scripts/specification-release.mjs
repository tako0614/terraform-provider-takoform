#!/usr/bin/env bun

// Validate and append the create-only Specification 1.1 release receipt.
//
// The numbered identity has one authority: the exact committed normative
// source snapshot in spec/publication-evidence.json. Compatibility reports,
// Forms, packages, Providers, Hosts, products, and adoption evidence remain
// independent axes and never enter the release identity or its asset set.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  existsSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  HOST_API_LANE,
  PUBLICATION_EVIDENCE_PROJECTION_PATHS,
  SPECIFICATION_PREREQUISITES,
  SPECIFICATION_TRACK,
  SPECIFICATION_VERSION,
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
export const RELEASE_VERSION = SPECIFICATION_VERSION;
export const WITHDRAWN_VERSION = "1.0";
export const SPECIFICATION_TAG = "specification/1.1";
export const RELEASE_RECEIPT_FORMAT =
  "takoform.specification-release-receipt@v1";
export const SOURCE_EVIDENCE_PATH = "spec/publication-evidence.json";
export const SOURCE_EVIDENCE_ASSET =
  "takoform-specification-1.1-source-snapshot.json";
export const HOST_API_EFFECT = "none";
export const FORM_PUBLICATION_EFFECT = "none";
export const PROVIDER_EFFECT = "none";

export const LEDGER_PROJECTION_PATHS = Object.freeze([
  "website/static/release/specification-releases.json",
  "website/public/release/specification-releases.json",
]);
export const C2_ALLOWED_PATHS = Object.freeze([
  SOURCE_EVIDENCE_PATH,
  ...PUBLICATION_EVIDENCE_PROJECTION_PATHS,
]);
export const C3_ALLOWED_PATHS = Object.freeze([
  LEDGER_PATH,
  ...LEDGER_PROJECTION_PATHS,
]);

export const EXPECTED_CANDIDATE = Object.freeze({
  version: RELEASE_VERSION,
  title: "Takoform Specification 1.1",
  track: SPECIFICATION_TRACK,
  hostApiLane: HOST_API_LANE,
  prerequisites: [...SPECIFICATION_PREREQUISITES],
  hostApiEffect: HOST_API_EFFECT,
  formPublicationEffect: FORM_PUBLICATION_EFFECT,
  providerEffect: PROVIDER_EFFECT,
});

export const EXPECTED_RESERVED = Object.freeze([
  Object.freeze({
    version: WITHDRAWN_VERSION,
    status: "withdrawn-retained",
    noReuse: true,
    reason:
      "reserved as a never-published Specification identity and withdrawn before publication",
  }),
]);

const FULL_SHA = /^[0-9a-f]{40}$/u;
const GIT_OBJECT = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u;
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const NUMBERED_VERSION = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/u;

function fail(message) {
  throw new Error(`specification release: ${message}`);
}

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function same(left, right) {
  return canonicalJson(left) === canonicalJson(right);
}

function sha256(raw) {
  return `sha256:${createHash("sha256").update(raw).digest("hex")}`;
}

export function sourceSnapshotDigest(sourceSnapshot) {
  return sha256(Buffer.from(canonicalJson(sourceSnapshot), "utf8"));
}

function gitOutput(root, args, { encoding = "utf8", trim = true } = {}) {
  try {
    const output = execFileSync("git", args, {
      cwd: root,
      encoding,
      stdio: ["ignore", "pipe", "pipe"],
      maxBuffer: 64 * 1024 * 1024,
    });
    if (encoding === null) return output;
    return trim ? output.trim() : output;
  } catch {
    return null;
  }
}

function parseJson(raw, label) {
  try {
    return JSON.parse(raw);
  } catch (error) {
    fail(
      `${label} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
}

function loadLedger(root = repositoryRoot) {
  return parseJson(
    readFileSync(path.join(root, LEDGER_PATH), "utf8"),
    LEDGER_PATH,
  );
}

function releaseUrlNamesTag(url, tag) {
  try {
    const parsed = new URL(url);
    const prefix =
      "/tako0614/terraform-provider-takoform/releases/tag/";
    return (
      parsed.protocol === "https:" &&
      parsed.hostname === "github.com" &&
      parsed.username === "" &&
      parsed.password === "" &&
      parsed.search === "" &&
      parsed.hash === "" &&
      parsed.pathname.startsWith(prefix) &&
      decodeURIComponent(parsed.pathname.slice(prefix.length)) === tag
    );
  } catch {
    return false;
  }
}

export function releaseFromEvidence(document, readback) {
  const sourceSnapshot = document?.evidence?.specification?.sourceSnapshot;
  if (
    !isRecord(sourceSnapshot) ||
    typeof sourceSnapshot.sourceCommit !== "string"
  ) {
    fail("Specification 1.1 evidence is not complete");
  }
  const assetDigest = readback?.assetDigests?.[SOURCE_EVIDENCE_ASSET];
  const release = {
    format: RELEASE_RECEIPT_FORMAT,
    version: RELEASE_VERSION,
    title: EXPECTED_CANDIDATE.title,
    track: SPECIFICATION_TRACK,
    hostApiLane: HOST_API_LANE,
    sourceCommit: sourceSnapshot.sourceCommit,
    releaseCommit: readback?.releaseCommit,
    sourceSnapshotSha256: sourceSnapshotDigest(sourceSnapshot),
    sourceEvidenceSha256: assetDigest,
    prerequisites: [...SPECIFICATION_PREREQUISITES],
    hostApiEffect: HOST_API_EFFECT,
    formPublicationEffect: FORM_PUBLICATION_EFFECT,
    providerEffect: PROVIDER_EFFECT,
    tag: readback?.tag,
    tagObject: readback?.tagObject,
    annotatedTag: true,
    release: {
      id: readback?.release?.id,
      url: readback?.release?.url,
      immutable: readback?.release?.immutable,
    },
    assets: [
      {
        name: SOURCE_EVIDENCE_ASSET,
        sourcePath: SOURCE_EVIDENCE_PATH,
        sha256: assetDigest,
      },
    ],
  };
  const problems = validateReleaseShape(release);
  if (problems.length !== 0) {
    fail(`authoritative receipt is invalid: ${problems.join("; ")}`);
  }
  return release;
}

export function validateReleaseShape(release) {
  const problems = [];
  if (!isRecord(release)) return ["release entry must be an object"];
  const expectedKeys = [
    "format",
    "version",
    "title",
    "track",
    "hostApiLane",
    "sourceCommit",
    "releaseCommit",
    "sourceSnapshotSha256",
    "sourceEvidenceSha256",
    "prerequisites",
    "hostApiEffect",
    "formPublicationEffect",
    "providerEffect",
    "tag",
    "tagObject",
    "annotatedTag",
    "release",
    "assets",
  ].sort();
  if (!same(Object.keys(release).sort(), expectedKeys)) {
    problems.push(
      `${release.version ?? "release"}: keys must be exactly ${expectedKeys.join(", ")}`,
    );
  }
  if (release.format !== RELEASE_RECEIPT_FORMAT) {
    problems.push(`format must be exactly ${RELEASE_RECEIPT_FORMAT}`);
  }
  if (!NUMBERED_VERSION.test(release.version ?? "")) {
    problems.push("version must be a numbered major.minor identity");
  }
  if (release.version !== RELEASE_VERSION) {
    problems.push(`version must be exactly ${RELEASE_VERSION}`);
  }
  if (release.title !== EXPECTED_CANDIDATE.title) {
    problems.push(`title must be ${EXPECTED_CANDIDATE.title}`);
  }
  if (
    release.track !== SPECIFICATION_TRACK ||
    release.hostApiLane !== HOST_API_LANE
  ) {
    problems.push(`must identify ${SPECIFICATION_TRACK} on ${HOST_API_LANE}`);
  }
  if (!FULL_SHA.test(release.sourceCommit ?? "")) {
    problems.push("sourceCommit must be a full lowercase commit SHA");
  }
  if (!FULL_SHA.test(release.releaseCommit ?? "")) {
    problems.push("releaseCommit must be a full lowercase commit SHA");
  }
  if (release.sourceCommit === release.releaseCommit) {
    problems.push("releaseCommit must be the distinct C2 evidence commit");
  }
  for (const field of [
    "sourceSnapshotSha256",
    "sourceEvidenceSha256",
  ]) {
    if (!SHA256.test(release[field] ?? "")) {
      problems.push(`${field} must be a lowercase sha256 identity`);
    }
  }
  if (!same(release.prerequisites, SPECIFICATION_PREREQUISITES)) {
    problems.push(
      `prerequisites must be exactly ${SPECIFICATION_PREREQUISITES.join(", ")}`,
    );
  }
  if (release.hostApiEffect !== HOST_API_EFFECT) {
    problems.push("must not mint or graduate a Host API lane");
  }
  if (release.formPublicationEffect !== FORM_PUBLICATION_EFFECT) {
    problems.push("must not publish or promote a Form Package");
  }
  if (release.providerEffect !== PROVIDER_EFFECT) {
    problems.push("must not publish or advance the Provider release stream");
  }
  if (release.tag !== SPECIFICATION_TAG) {
    problems.push(`tag must be exactly ${SPECIFICATION_TAG}`);
  }
  if (release.annotatedTag !== true || !GIT_OBJECT.test(release.tagObject ?? "")) {
    problems.push("tagObject must identify one exact annotated Git tag object");
  }
  if (
    !isRecord(release.release) ||
    !same(Object.keys(release.release).sort(), ["id", "immutable", "url"])
  ) {
    problems.push("release must contain exactly id, immutable, and url");
  } else {
    if (
      !Number.isSafeInteger(release.release.id) ||
      release.release.id <= 0
    ) {
      problems.push("release.id must be a positive safe integer");
    }
    if (release.release.immutable !== true) {
      problems.push("release.immutable must be true");
    }
    if (!releaseUrlNamesTag(release.release.url, SPECIFICATION_TAG)) {
      problems.push("release.url must be the canonical exact GitHub tag URL");
    }
  }
  if (!Array.isArray(release.assets) || release.assets.length !== 1) {
    problems.push("assets must contain the one exact committed source evidence asset");
  } else {
    const asset = release.assets[0];
    if (
      !isRecord(asset) ||
      !same(Object.keys(asset).sort(), ["name", "sha256", "sourcePath"]) ||
      asset.name !== SOURCE_EVIDENCE_ASSET ||
      asset.sourcePath !== SOURCE_EVIDENCE_PATH ||
      !SHA256.test(asset.sha256 ?? "") ||
      asset.sha256 !== release.sourceEvidenceSha256
    ) {
      problems.push("asset must bind the exact committed source evidence bytes");
    }
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
  const current = Array.isArray(currentLedger?.releases)
    ? currentLedger.releases
    : [];
  for (const revision of history) {
    const historical = revision.ledger?.releases;
    if (!Array.isArray(historical)) {
      problems.push(
        `committed ledger ${revision.commit.slice(0, 8)} has no releases array`,
      );
      continue;
    }
    for (const [index, release] of historical.entries()) {
      if (current[index]?.version !== release?.version) {
        problems.push(
          `committed release ${release?.version ?? "unknown"} was deleted or reordered`,
        );
      } else if (!same(current[index], release)) {
        problems.push(
          `committed release ${release.version} was mutated; releases are append-only`,
        );
      }
    }
  }
  return problems;
}

function validatePathList(paths, label) {
  const problems = [];
  if (
    !Array.isArray(paths) ||
    paths.some(
      (entry) =>
        typeof entry !== "string" ||
        entry === "" ||
        /[\0\r\n]/u.test(entry),
    ) ||
    new Set(paths).size !== paths.length
  ) {
    problems.push(`${label} contains an ambiguous path list`);
  }
  return problems;
}

export function validateC2DiffPaths(paths) {
  const problems = validatePathList(paths, "C1/C2 diff");
  if (!Array.isArray(paths)) return problems;
  for (const required of C2_ALLOWED_PATHS) {
    if (!paths.includes(required)) {
      problems.push(`C2 evidence-only diff must include ${required}`);
    }
  }
  const unexpected = paths.filter((entry) => !C2_ALLOWED_PATHS.includes(entry));
  if (unexpected.length !== 0) {
    problems.push(
      `C2 evidence-only diff contains forbidden paths: ${unexpected.join(", ")}`,
    );
  }
  return problems;
}

export function validateC3DiffPaths(paths) {
  const problems = validatePathList(paths, "C2/C3 diff");
  if (!Array.isArray(paths)) return problems;
  for (const required of C3_ALLOWED_PATHS) {
    if (!paths.includes(required)) {
      problems.push(
        `C3 receipt/ledger projection-only diff must include ${required}`,
      );
    }
  }
  const unexpected = paths.filter((entry) => !C3_ALLOWED_PATHS.includes(entry));
  if (unexpected.length !== 0) {
    problems.push(
      `C3 receipt/ledger projection-only diff contains forbidden paths: ${unexpected.join(", ")}`,
    );
  }
  return problems;
}

export function validateLedger(ledger, history = []) {
  const problems = [];
  if (!isRecord(ledger)) return ["ledger must be an object"];
  const expectedKeys = [
    "kind",
    "policy",
    "reserved",
    "candidate",
    "releases",
  ].sort();
  if (!same(Object.keys(ledger).sort(), expectedKeys)) {
    problems.push(`ledger keys must be exactly ${expectedKeys.join(", ")}`);
  }
  if (ledger.kind !== LEDGER_KIND) problems.push(`kind must be ${LEDGER_KIND}`);
  if (typeof ledger.policy !== "string" || ledger.policy.trim() === "") {
    problems.push("policy must be a non-empty string");
  }
  if (!same(ledger.reserved, EXPECTED_RESERVED)) {
    problems.push(
      "reserved must withdraw Specification 1.0 before publication and prohibit reuse",
    );
  }
  if (!same(ledger.candidate, EXPECTED_CANDIDATE)) {
    problems.push(
      "candidate must state the exact source-only Specification 1.1 track without Host, Form, Provider, or compatibility-report authority",
    );
  }
  if (!Array.isArray(ledger.releases)) {
    problems.push("releases must be an array");
  } else {
    const versions = new Set();
    for (const [index, release] of ledger.releases.entries()) {
      problems.push(
        ...validateReleaseShape(release).map(
          (problem) => `releases[${index}]: ${problem}`,
        ),
      );
      if (versions.has(release?.version)) {
        problems.push(`release ${release.version} is duplicated`);
      }
      versions.add(release?.version);
      if (release?.version === WITHDRAWN_VERSION) {
        problems.push("releases must not reuse withdrawn Specification 1.0");
      }
      if (
        index > 0 &&
        compareVersions(ledger.releases[index - 1].version, release?.version) >=
          0
      ) {
        problems.push(
          `release ${release?.version} is not appended in numeric order`,
        );
      }
    }
  }
  problems.push(...validateCommittedHistory(ledger, history));
  return problems;
}

function committedHistory(root) {
  const commits = gitOutput(root, [
    "log",
    "--full-history",
    "--format=%H",
    "HEAD",
    "--",
    LEDGER_PATH,
  ]);
  if (commits === null || commits === "") return [];
  return commits.split("\n").map((commit) => {
    const source = gitOutput(root, ["show", `${commit}:${LEDGER_PATH}`]);
    if (source === null) fail(`cannot read ${LEDGER_PATH} at ${commit}`);
    return {
      commit,
      ledger: parseJson(source, `committed ${LEDGER_PATH} at ${commit}`),
    };
  });
}

function changedPaths(root, base, head) {
  const raw = gitOutput(
    root,
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
    { trim: false },
  );
  if (raw === null || (raw !== "" && !raw.endsWith("\0"))) {
    fail(`cannot read an exact diff path set for ${base}..${head}`);
  }
  return raw === "" ? [] : raw.slice(0, -1).split("\0");
}

function showBytes(root, commit, relativePath) {
  const raw = gitOutput(
    root,
    ["show", `${commit}:${relativePath}`],
    { encoding: null, trim: false },
  );
  if (raw === null) fail(`cannot read ${relativePath} at ${commit}`);
  return raw;
}

export function validateReleaseRepositoryBinding(release, root = repositoryRoot) {
  const problems = [];
  const parent = gitOutput(root, ["rev-parse", `${release.releaseCommit}^`]);
  if (parent !== release.sourceCommit) {
    problems.push(
      `releaseCommit must be the direct C2 child of sourceCommit; observed parent ${parent ?? "unreadable"}`,
    );
    return problems;
  }
  problems.push(
    ...validateC2DiffPaths(
      changedPaths(root, release.sourceCommit, release.releaseCommit),
    ),
  );
  const sourceEvidence = showBytes(
    root,
    release.releaseCommit,
    SOURCE_EVIDENCE_PATH,
  );
  const document = parseJson(
    sourceEvidence.toString("utf8"),
    `${SOURCE_EVIDENCE_PATH} at ${release.releaseCommit}`,
  );
  const expected = releaseFromEvidence(document, {
    releaseCommit: release.releaseCommit,
    tag: release.tag,
    tagObject: release.tagObject,
    release: release.release,
    assetDigests: {
      [SOURCE_EVIDENCE_ASSET]: sha256(sourceEvidence),
    },
  });
  if (!same(release, expected)) {
    problems.push(
      "receipt does not equal the exact committed C2 source evidence and authoritative readback",
    );
  }
  const c1Evidence = parseJson(
    showBytes(root, release.sourceCommit, SOURCE_EVIDENCE_PATH).toString("utf8"),
    `${SOURCE_EVIDENCE_PATH} at ${release.sourceCommit}`,
  );
  if (c1Evidence?.evidence?.specification?.sourceSnapshot !== null) {
    problems.push("C1 sourceSnapshot must be null before the evidence-only C2 commit");
  }
  const c2Ledger = parseJson(
    showBytes(root, release.releaseCommit, LEDGER_PATH).toString("utf8"),
    `${LEDGER_PATH} at ${release.releaseCommit}`,
  );
  if (!Array.isArray(c2Ledger.releases) || c2Ledger.releases.length !== 0) {
    problems.push("C2 ledger must contain no receipt before authoritative readback");
  }
  return problems;
}

function validateReceiptIntroduction(history, root) {
  const problems = [];
  const chronological = [...history].reverse();
  let previous = [];
  for (const revision of chronological) {
    const releases = Array.isArray(revision.ledger?.releases)
      ? revision.ledger.releases
      : [];
    if (
      !previous.some((release) => release?.version === RELEASE_VERSION) &&
      releases.some((release) => release?.version === RELEASE_VERSION)
    ) {
      const parent = gitOutput(root, ["rev-parse", `${revision.commit}^`]);
      if (parent === null) {
        problems.push("C3 receipt introduction commit has no readable parent");
      } else {
        problems.push(
          ...validateC3DiffPaths(changedPaths(root, parent, revision.commit)),
        );
      }
    }
    previous = releases;
  }
  return problems;
}

function validateLedgerProjections(ledger, root) {
  const problems = [];
  for (const projection of LEDGER_PROJECTION_PATHS) {
    const absolute = path.join(root, projection);
    if (!existsSync(absolute)) {
      problems.push(`required ledger projection ${projection} is missing`);
      continue;
    }
    const projected = parseJson(readFileSync(absolute, "utf8"), projection);
    if (!same(projected, ledger)) {
      problems.push(`${projection} differs from ${LEDGER_PATH}`);
    }
  }
  return problems;
}

function expectedReadyDocument(root) {
  const document = loadPublicationEvidence(root);
  const report = validatePublicationEvidence(document, {
    repositoryRoot: root,
    releaseTrack: SPECIFICATION_TRACK,
  });
  assertPublicationEvidenceReady(report, SPECIFICATION_TRACK);
  return document;
}

// This writer is intentionally not a CLI mode. The production entrypoint calls
// it only after exact remote tag, immutable Release, API asset, and downloaded
// byte readback have all succeeded. It appends one receipt and mirrors only the
// public ledger projections; it never edits normative source or release tools.
export function appendReleaseReceipt(receipt, root = repositoryRoot) {
  const shapeProblems = validateReleaseShape(receipt);
  if (shapeProblems.length !== 0) {
    fail(`refusing invalid receipt: ${shapeProblems.join("; ")}`);
  }
  const ledger = loadLedger(root);
  const ledgerProblems = validateLedger(ledger);
  if (ledgerProblems.length !== 0) {
    fail(`refusing writer on invalid ledger: ${ledgerProblems.join("; ")}`);
  }
  if (ledger.releases.some((entry) => entry.version === receipt.version)) {
    fail(`receipt ${receipt.version} already exists; writer is create-only`);
  }
  const projectionProblems = validateLedgerProjections(ledger, root);
  if (projectionProblems.length !== 0) {
    fail(`refusing writer with drifted projection: ${projectionProblems.join("; ")}`);
  }
  const document = loadPublicationEvidence(root);
  const sourceBytes = readFileSync(path.join(root, SOURCE_EVIDENCE_PATH));
  const expected = releaseFromEvidence(document, {
    releaseCommit: receipt.releaseCommit,
    tag: receipt.tag,
    tagObject: receipt.tagObject,
    release: receipt.release,
    assetDigests: {
      [SOURCE_EVIDENCE_ASSET]: sha256(sourceBytes),
    },
  });
  if (!same(receipt, expected)) {
    fail("receipt differs from the exact local committed source evidence");
  }
  const next = structuredClone(ledger);
  next.releases.push(receipt);
  const raw = `${JSON.stringify(next, null, 2)}\n`;
  writeFileSync(path.join(root, LEDGER_PATH), raw);
  for (const projection of LEDGER_PROJECTION_PATHS) {
    writeFileSync(path.join(root, projection), raw);
  }
  return next;
}

export function run(mode, root = repositoryRoot) {
  if (!["--check", "--assert-ready"].includes(mode)) {
    fail("usage: bun scripts/specification-release.mjs --check|--assert-ready");
  }
  const ledger = loadLedger(root);
  const history = committedHistory(root);
  const problems = [
    ...validateLedger(ledger, history),
    ...validateLedgerProjections(ledger, root),
  ];
  if (ledger.releases.length !== 0) {
    expectedReadyDocument(root);
    const recorded = ledger.releases.find(
      (release) => release.version === RELEASE_VERSION,
    );
    if (recorded === undefined) {
      problems.push("Specification 1.1 authoritative receipt is missing");
    } else {
      problems.push(...validateReleaseRepositoryBinding(recorded, root));
      const committed = history.some((revision) =>
        revision.ledger?.releases?.some(
          (release) =>
            release.version === RELEASE_VERSION && same(release, recorded),
        ),
      );
      if (!committed) {
        problems.push("Specification 1.1 receipt must be committed as C3");
      } else {
        problems.push(...validateReceiptIntroduction(history, root));
      }
    }
  }
  if (problems.length !== 0) {
    for (const problem of problems) process.stderr.write(`- ${problem}\n`);
    fail(`${problems.length} ledger problem(s)`);
  }
  if (mode === "--assert-ready") expectedReadyDocument(root);
  const releaseStatus = ledger.releases.some(
    (release) => release?.version === RELEASE_VERSION,
  )
    ? "released"
    : "candidate-open";
  process.stdout.write(
    `specification release ledger OK: Specification 1.1 ${releaseStatus}; ${ledger.releases.length} authoritative receipt(s) recorded\n`,
  );
  return ledger;
}

if (import.meta.main) run(process.argv[2]);

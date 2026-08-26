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
import process from "node:process";
import { fileURLToPath } from "node:url";

import {
  assertSafeRepositoryGitConfiguration,
  createHardenedGitEnvironment,
} from "./deploy-safety.mjs";
import {
  CANONICAL_ORIGIN,
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
export const C4_REQUIRED_PATHS = Object.freeze([
  "README.md",
  "release/specification-compatibility.json",
  "website/static/release/specification-compatibility.json",
  "website/static/.well-known/takoform-site.json",
  "website/public/.well-known/takoform-site.json",
  "website/public/release/specification-compatibility.json",
  "website/public/404.html",
  "website/public/index.html",
  "website/public/release/index.html",
  "website/public/spec/index.html",
  "website/public/ja/spec/index.html",
]);
export const C4_EXACT_ALLOWED_PATHS = Object.freeze([
  ...C4_REQUIRED_PATHS,
]);
export const C4_PRESERVED_PUBLIC_PATHS = Object.freeze([
  "website/public/conformance/runtime-abi-v1/bundles/unsupported-media-type/page.html",
]);
const C4_CONTENT_HASHED_ASSET =
  /^website\/public\/assets\/((?:chunks\/)?[A-Za-z0-9@][A-Za-z0-9@._-]*)\.([A-Za-z0-9_-]{8})((?:\.lean)?\.(?:js|css|woff2))$/u;
export const RELEASE_STATE_NEUTRAL_SOURCE_PATHS = Object.freeze([
  "release/README.md",
  "scripts/current-generation.mjs",
  "spec/README.md",
  "spec/decisions/0057-specification-1-1-compatibility-and-independent-identities.md",
  "spec/host-api/README.md",
  "spec/project-lifecycle.md",
  "spec/publication-freeze.md",
  "website/README.md",
  "website/docs/versions.md",
  "website/ja/spec/index.md",
  "website/ja/docs/versions.md",
  "website/spec/index.md",
]);
export const RELEASE_STATE_CURRENT_PUBLIC_PATHS = Object.freeze([
  "website/public/index.html",
  "website/public/release/index.html",
  "website/public/spec/index.html",
  "website/public/spec/overview.html",
  "website/public/spec/project-lifecycle.html",
  "website/public/spec/publication-freeze.html",
  "website/public/spec/host-api/index.html",
  "website/public/ja/spec/index.html",
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

function gitOutput(root, args, { encoding = "utf8", trim = true } = {}) {
  try {
    const output = execFileSync("git", args, {
      cwd: root,
      encoding,
      env: gitEnvironment(),
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

export function isAllowedC4Path(relativePath) {
  if (C4_PRESERVED_PUBLIC_PATHS.includes(relativePath)) return false;
  return (
    C4_EXACT_ALLOWED_PATHS.includes(relativePath) ||
    websiteAssetRole(relativePath) !== null ||
    (relativePath.startsWith("website/public/") &&
      relativePath.endsWith(".html"))
  );
}

export function websiteAssetRole(relativePath) {
  const match = C4_CONTENT_HASHED_ASSET.exec(relativePath);
  return match === null ? null : `${match[1]}${match[3]}`;
}

export function validateC4DiffPaths(paths) {
  const problems = validatePathList(paths, "C3/C4 diff");
  if (!Array.isArray(paths)) return problems;
  for (const required of C4_REQUIRED_PATHS) {
    if (!paths.includes(required)) {
      problems.push(`C4 derived-public diff must include ${required}`);
    }
  }
  if (!paths.some((entry) => entry.startsWith("website/public/assets/"))) {
    problems.push(
      "C4 derived-public diff must include the rebuilt website/public/assets closure",
    );
  }
  const unexpected = paths.filter((entry) => !isAllowedC4Path(entry));
  if (unexpected.length !== 0) {
    problems.push(
      `C4 derived-public diff contains forbidden paths: ${unexpected.join(", ")}`,
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
    "--no-show-signature",
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

function assertSelfContainedGitObjectStore(root) {
  const configuration = gitOutput(
    root,
    ["config", "--local", "-z", "--list"],
    { trim: false },
  );
  if (configuration === null) {
    fail("cannot read repository Git configuration");
  }
  try {
    assertSafeRepositoryGitConfiguration(configuration, CANONICAL_ORIGIN);
  } catch (error) {
    fail(error instanceof Error ? error.message : String(error));
  }
  if (gitOutput(root, ["rev-parse", "--is-shallow-repository"]) !== "false") {
    fail("repository must be complete and non-shallow");
  }
  const commonDirectory = gitOutput(root, [
    "rev-parse",
    "--path-format=absolute",
    "--git-common-dir",
  ]);
  if (commonDirectory === null || commonDirectory === "") {
    fail("cannot resolve the repository Git object authority");
  }
  for (const relativePath of ["objects/info/alternates", "info/grafts"]) {
    if (existsSync(path.join(commonDirectory, relativePath))) {
      fail(`repository Git object authority uses forbidden ${relativePath}`);
    }
  }
}

export function validateReleaseRepositoryBinding(release, root = repositoryRoot) {
  assertSelfContainedGitObjectStore(root);
  const problems = [];
  const parents = commitParents(root, release.releaseCommit);
  if (
    parents === null ||
    parents.length !== 1 ||
    parents[0] !== release.sourceCommit
  ) {
    problems.push(
      `releaseCommit must be the direct single-parent C2 child of sourceCommit; observed parents ${parents?.join(", ") || "unreadable"}`,
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

function commitParents(root, commit) {
  const raw = gitOutput(root, ["rev-list", "--parents", "-n", "1", commit]);
  if (raw === null) return null;
  const values = raw.split(" ");
  if (
    values[0] !== commit ||
    values.some((value) => !FULL_SHA.test(value))
  ) {
    return null;
  }
  return values.slice(1);
}

function receiptIntroductions(history) {
  const introductions = [];
  const removals = [];
  let present = false;
  for (const revision of [...history].reverse()) {
    const releases = Array.isArray(revision.ledger?.releases)
      ? revision.ledger.releases
      : [];
    const next = releases.some(
      (release) => release?.version === RELEASE_VERSION,
    );
    if (!present && next) introductions.push(revision);
    if (present && !next) removals.push(revision.commit);
    present = next;
  }
  return { introductions, removals };
}

function firstParentSuccessor(root, commit) {
  const raw = gitOutput(root, ["rev-list", "--first-parent", "HEAD"]);
  if (raw === null || raw === "") return null;
  const chain = raw.split("\n");
  const index = chain.indexOf(commit);
  if (index <= 0) return null;
  return chain[index - 1];
}

function servedHtmlPaths(root, commit) {
  const raw = gitOutput(
    root,
    [
      "ls-tree",
      "-r",
      "-z",
      "--name-only",
      commit,
      "--",
      "website/public",
    ],
    { trim: false },
  );
  if (raw === null) return [];
  return raw
    .split("\0")
    .filter((relativePath) => relativePath.endsWith(".html"))
    .sort();
}

function websiteAssetPaths(root, commit) {
  const raw = gitOutput(
    root,
    [
      "ls-tree",
      "-r",
      "-z",
      "--name-only",
      commit,
      "--",
      "website/public/assets",
    ],
    { trim: false },
  );
  if (raw === null || (raw !== "" && !raw.endsWith("\0"))) {
    fail(`cannot read the website asset inventory at ${commit}`);
  }
  return raw === "" ? [] : raw.slice(0, -1).split("\0").sort();
}

function websiteAssetInventory(root, commit) {
  const roles = new Map();
  const problems = [];
  for (const relativePath of websiteAssetPaths(root, commit)) {
    const role = websiteAssetRole(relativePath);
    if (role === null) {
      problems.push(
        `${relativePath} is not a bounded content-hashed VitePress asset`,
      );
      continue;
    }
    if (roles.has(role)) {
      problems.push(
        `website asset role ${role} is duplicated by ${roles.get(role)} and ${relativePath}`,
      );
      continue;
    }
    roles.set(role, relativePath);
  }
  return { problems, roles };
}

function validateC4AssetTransition(root, c3, c4, changedPathsAtC4) {
  const before = websiteAssetInventory(root, c3);
  const after = websiteAssetInventory(root, c4);
  const problems = [...before.problems, ...after.problems];
  const beforeRoles = [...before.roles.keys()].sort();
  const afterRoles = [...after.roles.keys()].sort();
  if (!same(beforeRoles, afterRoles)) {
    const added = afterRoles.filter((role) => !before.roles.has(role));
    const removed = beforeRoles.filter((role) => !after.roles.has(role));
    problems.push(
      `C4 must preserve the exact website asset role inventory; added ${added.join(", ") || "none"}; removed ${removed.join(", ") || "none"}`,
    );
  }

  const expectedChangedPaths = new Set();
  for (const role of beforeRoles.filter((entry) => after.roles.has(entry))) {
    const previousPath = before.roles.get(role);
    const nextPath = after.roles.get(role);
    if (previousPath === nextPath) {
      if (changedPathsAtC4.includes(previousPath)) {
        problems.push(
          `C4 must not mutate content-hashed website asset in place: ${previousPath}`,
        );
      }
      continue;
    }
    expectedChangedPaths.add(previousPath);
    expectedChangedPaths.add(nextPath);
    if (showBytes(root, c3, previousPath).equals(showBytes(root, c4, nextPath))) {
      problems.push(
        `C4 website asset role ${role} changed hash path without changing bytes`,
      );
    }
  }

  const actualChangedPaths = changedPathsAtC4
    .filter((relativePath) => relativePath.startsWith("website/public/assets/"))
    .sort();
  const expected = [...expectedChangedPaths].sort();
  if (!same(actualChangedPaths, expected)) {
    const unexpected = actualChangedPaths.filter(
      (relativePath) => !expectedChangedPaths.has(relativePath),
    );
    const missing = expected.filter(
      (relativePath) => !actualChangedPaths.includes(relativePath),
    );
    problems.push(
      `C4 website asset diff must be exact content-hash replacements; unexpected ${unexpected.join(", ") || "none"}; missing ${missing.join(", ") || "none"}`,
    );
  }
  if (expected.length === 0) {
    problems.push("C4 must replace at least one content-hashed website asset");
  }
  return problems;
}

export function staleSpecificationReleaseWording(raw) {
  return [
    /(?:Takoform\s+)?Specification\s+1\.1\s+(?:release\s+)?(?:is\s+|remains\s+)?(?:an?\s+)?(?:open\s+)?(?:numbered\s+)?candidate(?:-open)?\b/iu,
    /\bTakoform\s+1\.1\s+candidate\b/iu,
    /\bTakoform\s+1\.1\s+(?:is\s+)?(?:an?\s+)?open\s+(?:numbered\s+)?candidate\b/iu,
    /\b(?:Takoform\s+)?(?:Specification\s+)?1\.1\b[^.\n]{0,160}\bopen\s+until\b/iu,
    /\bSpecification\s*:?\s*1\.1\s*\(\s*candidate-open\b/iu,
    /\bSpecification\s+1\.1\s+release\s+candidate\s+remains\s+open\b/iu,
    /\bnumbered\s+release\s+remains\s+open\s+until\b/iu,
    /\bSpecification\s+release\s+assertion\s+remains[^.\n]{0,100}\bopen\b/iu,
  ].some((pattern) => pattern.test(raw));
}

function validateC4CommittedBytes(root, c3, c4) {
  const problems = [];
  const compatibilityPaths = [
    "release/specification-compatibility.json",
    "website/static/release/specification-compatibility.json",
    "website/public/release/specification-compatibility.json",
  ];
  const compatibilityBytes = compatibilityPaths.map((relativePath) =>
    showBytes(root, c4, relativePath),
  );
  if (
    compatibilityBytes.some(
      (bytes) => !bytes.equals(compatibilityBytes[0]),
    )
  ) {
    problems.push(
      "C4 canonical/static/public Specification compatibility bytes must be identical",
    );
  } else {
    const compatibility = parseJson(
      compatibilityBytes[0].toString("utf8"),
      `Specification compatibility report at ${c4}`,
    );
    const entries = Array.isArray(compatibility?.classes)
      ? compatibility.classes.flatMap((entry) => entry?.entries ?? [])
      : [];
    const specification = entries.find(
      (entry) => entry?.identity === `takoform.specification@${RELEASE_VERSION}`,
    );
    if (
      specification?.status !== "retained" ||
      specification?.publication !== "retained"
    ) {
      problems.push(
        "C4 compatibility report must derive Specification 1.1 as retained publication history",
      );
    }
  }

  const statusPaths = [
    "website/static/.well-known/takoform-site.json",
    "website/public/.well-known/takoform-site.json",
  ];
  const statusBytes = statusPaths.map((relativePath) =>
    showBytes(root, c4, relativePath),
  );
  if (!statusBytes[0].equals(statusBytes[1])) {
    problems.push("C4 source/public site-status bytes must be identical");
  } else {
    const status = parseJson(
      statusBytes[0].toString("utf8"),
      `site status at ${c4}`,
    );
    if (
      status?.specificationVersion !== RELEASE_VERSION ||
      status?.specificationReleaseStatus !== "released"
    ) {
      problems.push("C4 site status must derive Specification 1.1 as released");
    }
  }

  const readme = showBytes(root, c4, "README.md").toString("utf8");
  if (
    !readme.includes(
      "| Specification | `1.1` | released; one exact committed normative source snapshot is release authority |",
    ) ||
    staleSpecificationReleaseWording(readme)
  ) {
    problems.push("C4 README generation must derive Specification 1.1 as released");
  }

  const htmlPaths = servedHtmlPaths(root, c4);
  if (htmlPaths.length === 0) {
    problems.push("C4 contains no served HTML output");
  }
  for (const relativePath of htmlPaths) {
    const raw = showBytes(root, c4, relativePath).toString("utf8");
    const visible = raw
      .replace(/<script\b[\s\S]*?<\/script>/giu, " ")
      .replace(/<style\b[\s\S]*?<\/style>/giu, " ")
      .replace(/<[^>]+>/gu, " ")
      .replace(/\s+/gu, " ");
    if (
      staleSpecificationReleaseWording(raw) ||
      staleSpecificationReleaseWording(visible)
    ) {
      problems.push(`${relativePath} still calls Specification 1.1 candidate/open at C4`);
    }
  }

  for (const relativePath of C3_ALLOWED_PATHS) {
    if (!showBytes(root, c3, relativePath).equals(showBytes(root, c4, relativePath))) {
      problems.push(`C4 must preserve C3 authority bytes at ${relativePath}`);
    }
  }
  return problems;
}

export function validateReceiptTransitionHistory(
  release,
  history,
  root = repositoryRoot,
) {
  assertSelfContainedGitObjectStore(root);
  const problems = [];
  const { introductions, removals } = receiptIntroductions(history);
  if (removals.length !== 0) {
    problems.push(
      `Specification 1.1 receipt disappeared in committed history at ${removals.join(", ")}`,
    );
  }
  if (introductions.length !== 1) {
    problems.push(
      `Specification 1.1 must have exactly one C3 receipt introduction; observed ${introductions.length}`,
    );
    return problems;
  }
  const c3 = introductions[0].commit;
  const c3Parents = commitParents(root, c3);
  if (
    c3Parents === null ||
    c3Parents.length !== 1 ||
    c3Parents[0] !== release.releaseCommit
  ) {
    problems.push(
      `C3 ${c3} must be the direct single-parent child of C2 ${release.releaseCommit}`,
    );
    return problems;
  }
  problems.push(...validateC3DiffPaths(changedPaths(root, release.releaseCommit, c3)));
  const c3Bytes = C3_ALLOWED_PATHS.map((relativePath) =>
    showBytes(root, c3, relativePath),
  );
  if (c3Bytes.some((bytes) => !bytes.equals(c3Bytes[0]))) {
    problems.push(
      "C3 canonical/static/public Specification ledger bytes must be identical",
    );
  } else {
    const c3Ledger = parseJson(
      c3Bytes[0].toString("utf8"),
      `${LEDGER_PATH} at C3 ${c3}`,
    );
    const introduced = c3Ledger?.releases?.find(
      (entry) => entry?.version === RELEASE_VERSION,
    );
    if (!same(introduced, release)) {
      problems.push("C3 must contain the exact current Specification 1.1 receipt");
    }
  }

  const c4 = firstParentSuccessor(root, c3);
  if (c4 === null) {
    problems.push(
      "C4 derived-public commit is missing from the current first-parent ancestry",
    );
    return problems;
  }
  const c4Parents = commitParents(root, c4);
  if (
    c4Parents === null ||
    c4Parents.length !== 1 ||
    c4Parents[0] !== c3
  ) {
    problems.push(`C4 ${c4} must be the direct single-parent child of C3 ${c3}`);
    return problems;
  }
  const c4ChangedPaths = changedPaths(root, c3, c4);
  problems.push(...validateC4DiffPaths(c4ChangedPaths));
  problems.push(...validateC4AssetTransition(root, c3, c4, c4ChangedPaths));
  const c3AllHtmlPaths = servedHtmlPaths(root, c3);
  const c4AllHtmlPaths = servedHtmlPaths(root, c4);
  const c3HtmlPathSet = new Set(c3AllHtmlPaths);
  const c4HtmlPathSet = new Set(c4AllHtmlPaths);
  const addedHtmlPaths = c4AllHtmlPaths.filter(
    (relativePath) => !c3HtmlPathSet.has(relativePath),
  );
  const removedHtmlPaths = c3AllHtmlPaths.filter(
    (relativePath) => !c4HtmlPathSet.has(relativePath),
  );
  if (addedHtmlPaths.length !== 0 || removedHtmlPaths.length !== 0) {
    problems.push(
      `C4 must preserve the exact served HTML path inventory; added ${addedHtmlPaths.join(", ") || "none"}; removed ${removedHtmlPaths.join(", ") || "none"}`,
    );
  }
  const c3HtmlPaths = c3AllHtmlPaths.filter(
    (relativePath) => !C4_PRESERVED_PUBLIC_PATHS.includes(relativePath),
  );
  const unchangedHtmlPaths = c3HtmlPaths.filter(
    (relativePath) => !c4ChangedPaths.includes(relativePath),
  );
  if (unchangedHtmlPaths.length !== 0) {
    problems.push(
      `C4 must refresh every served HTML page: ${unchangedHtmlPaths.join(", ")}`,
    );
  }
  problems.push(...validateC4CommittedBytes(root, c3, c4));
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
  assertSelfContainedGitObjectStore(root);
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
        problems.push(
          ...validateReceiptTransitionHistory(recorded, history, root),
        );
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

import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  renameSync,
  writeFileSync,
} from "node:fs";
import process from "node:process";

/**
 * The W10 predecessor authority record is deliberately a small, closed
 * document. It is read before deploy capability is constructed; no
 * credential, network, sibling source, or release gate is consulted here.
 * Local Git history and the committed standalone readback are the only
 * authority evidence allowed at this boundary.
 */
export const AUTHORITY_TOMBSTONE_PATH =
  "release/specification-schema-authority-tombstone.json";
export const PREDECESSOR_TOMBSTONE_RECORD_PATH = AUTHORITY_TOMBSTONE_PATH;
export const CORE_AUTHORITY_RECORD_PATH = "release/specification-authority.json";
export const CANONICAL_AUTHORITY_RECORD_PATH = CORE_AUTHORITY_RECORD_PATH;
export const CORE_AUTHORITY_FORMAT = "takoform.specification-authority-transfer@v1";
export const CORE_AUTHORITY_STATE = "prepared-writer-disabled";
export const AUTHORITY_EVIDENCE_PATH =
  "release/authority/transition-evidence.json";
export const AUTHORITY_TOMBSTONE_FORMAT =
  "takoform.specification-schema-authority-tombstone@v1";
export const AUTHORITY_EVIDENCE_FORMAT =
  "takoform.specification-schema-authority-transition-evidence@v1";

export const PREDECESSOR_REPOSITORY =
  "https://github.com/tako0614/terraform-provider-takoform.git";
export const SUCCESSOR_REPOSITORY =
  "https://github.com/tako0614/takoform.git";
export const PREDECESSOR_CUTOFF_COMMIT =
  "1fa34160a4ed152443b4ea424a324f7677716e36";
export const PREDECESSOR_CUTOFF_TREE =
  "7e4a2578af2f50b826fba1004fdd4e430c761314";

export const PREDECESSOR_WRITER_BLOB_OIDS = Object.freeze({
  "scripts/deploy.mjs": "acca72d18f1c0342f5c413a8e039bec3d710e54d",
  "scripts/release-deploy.mjs": "92434be6df8fe099821a63d5d18a67d52cc93be1",
  "scripts/specification-release.mjs":
    "1b693d16172b5bf7f78f3cca02e40fe0ae62775b",
  "scripts/public-schema-manifest.mjs":
    "416ba477abf39717af02103ce94c30f51ecd78bc",
  "scripts/schema-publication-guard.mjs":
    "9ec47547843c9f70a8cbadb0dac6e3e87724b09c",
  "website/wrangler.jsonc": "2ec5ab30974539ee4daa120dab4e7efe200aa2bb",
});

export const PREDECESSOR_WORKFLOW_BLOB_OIDS = Object.freeze({
  ".github/workflows/provider-release-tag.yml":
    "59eda81a1b87ca33fc5ee8e815d7b7253047d02c",
  ".github/workflows/release.yml":
    "fba301ac83788849d4c2615d284a21c760628a71",
  ".github/workflows/form-package-revocation.yml":
    "a1dbde9ab288517b49b5b08a440969dc45b4adeb",
});

export const DISABLED_SURFACES = Object.freeze([
  "takoform-website",
  "takoform-specification-release",
  "legacy-schema-publication-through-website",
]);
export const RETAINED_SURFACES = Object.freeze([
  "takoform-provider-release",
  "takoform-form-package-release",
]);

const TOP_LEVEL_KEYS = Object.freeze([
  "format",
  "status",
  "predecessorRepository",
  "predecessorSourceCommit",
  "predecessorSourceTree",
  "predecessorWriterBlobOids",
  "predecessorWorkflowBlobOids",
  "successorRepository",
  "successorPreparedCommit",
  "successorAuthorityCommit",
  "disabledAt",
  "authorityEvidence",
  "publishedSpecification",
  "signers",
  "disabledSurfaces",
  "retainedSurfaces",
]);
const SPECIFICATION_KEYS = Object.freeze([
  "version",
  "tag",
  "tagObject",
  "sourceCommit",
  "releaseCommit",
  "releaseId",
  "releaseUrl",
  "immutable",
  "assetId",
  "assetName",
  "assetSourcePath",
  "assetSha256",
  "sourceSnapshotSha256",
  "sourceEvidenceSha256",
]);
const SIGNER_KEYS = Object.freeze([
  "predecessorProviderSigningFingerprint",
  "successorTagSignerFingerprint",
  "successorRecordHeadSpkiSha256",
]);
const AUTHORITY_EVIDENCE_KEYS = Object.freeze([
  "format",
  "path",
  "sha256",
  "tombstoneSha256",
  "canonicalPath",
  "preparedCommit",
  "authorityCommit",
]);
const EVIDENCE_DOCUMENT_KEYS = Object.freeze([
  "format",
  "tombstoneSha256",
  "canonicalPath",
  "preparedCommit",
  "authorityCommit",
  "preparedTree",
  "authorityTree",
  "changedPaths",
  "objects",
]);
const EVIDENCE_OBJECT_KEYS = Object.freeze(["oid", "type", "data"]);
const GIT_OBJECT_TYPES = Object.freeze(["blob", "commit", "tree"]);
const CORE_AUTHORITY_KEYS = Object.freeze([
  "format",
  "lastPredecessorSpecificationRelease",
  "predecessorCutoffCommit",
  "predecessorCutoffTree",
  "predecessorRepository",
  "predecessorTombstoneCommit",
  "predecessorWriterDisabledAt",
  "rollback",
  "schemaRouteCutover",
  "state",
  "successorPreparedCommit",
  "successorRepository",
  "successorWriterEnabledAt",
  "writerOverlapAllowed",
]);
const CORE_LAST_SPECIFICATION_KEYS = Object.freeze([
  "releaseId",
  "tag",
  "tagObject",
  "version",
]);
const CORE_ROLLBACK =
  "Before successor activation, abandon the prepared repository and leave the predecessor writer unchanged. After predecessor disablement, repair forward in the successor; never reopen the predecessor writer or recreate Specification 1.1.";
const CORE_LAST_SPECIFICATION = Object.freeze({
  version: "1.1",
  tag: "specification/1.1",
  tagObject: "e2c1ba71766a6b25cae0826df99c8906a7f3f20b",
  releaseId: 377480828,
});
const COMMIT = /^[0-9a-f]{40}$/u;
const OBJECT_OID = /^[0-9a-f]{40}$/u;
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const INSTANT =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$/u;
const PREDECESSOR_PROVIDER_SIGNING_FINGERPRINT =
  "3510E75E05BBCC303B92D77934FC18AC897FB709";
const SUCCESSOR_TAG_SIGNER_FINGERPRINT =
  "SHA256:C9nOGYF3q5s7QoftDP/eB7oAGtmC7fjC6UX+60/VyzE";
const SUCCESSOR_RECORD_HEAD_SPKI_SHA256 =
  "sha256:a4f2a0811b8d9432a8d5ecea246f768470d78d0ed53214a587ba2f7c238e8cbb";
const W09_SPECIFICATION = Object.freeze({
  version: "1.1",
  tag: "specification/1.1",
  tagObject: "e2c1ba71766a6b25cae0826df99c8906a7f3f20b",
  sourceCommit: "00ae5ee4e2ea2eb62ea796499a93081374dc36b9",
  releaseCommit: "35c03a76326c808e859aa77172e086f15a2aeb5d",
  releaseId: 377480828,
  releaseUrl:
    "https://github.com/tako0614/terraform-provider-takoform/releases/tag/specification/1.1",
  immutable: true,
  assetId: 531545608,
  assetName: "takoform-specification-1.1-source-snapshot.json",
  assetSourcePath: "spec/publication-evidence.json",
  assetSha256:
    "sha256:6f2ba3d51261f2559d0738ed2b22f51d7066d4d5b9f9bf0213694f352b677a84",
  sourceSnapshotSha256:
    "sha256:23a9b14dc79f46fae632624fc5c442f947f63565e9d7f9d0a614598b5027ae03",
  sourceEvidenceSha256:
    "sha256:6f2ba3d51261f2559d0738ed2b22f51d7066d4d5b9f9bf0213694f352b677a84",
});

const moduleRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const SHA1 = /^[0-9a-f]{40}$/u;
const BASE64 = /^[A-Za-z0-9+/]*={0,2}$/u;

function canonicalize(value) {
  if (Array.isArray(value)) return value.map((entry) => canonicalize(entry));
  if (!isRecord(value)) return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, canonicalize(value[key])]),
  );
}

/**
 * The evidence digest intentionally excludes the evidence pointer itself.
 * Otherwise the tombstone would need to contain a hash of bytes that contain
 * a hash of the tombstone (an impossible cycle).  Every other tombstone field
 * is included and canonicalized, so formatting or key-order changes do not
 * create a second authority record.
 */
export function authorityTombstoneDigest(value) {
  const withoutEvidence = structuredClone(value);
  if (isRecord(withoutEvidence)) withoutEvidence.authorityEvidence = null;
  return `sha256:${createHash("sha256")
    .update(JSON.stringify(canonicalize(withoutEvidence)))
    .digest("hex")}`;
}

export const canonicalAuthorityTombstoneDigest = authorityTombstoneDigest;

function gitError(repositoryRoot, args, error) {
  const stderr = Buffer.isBuffer(error?.stderr)
    ? error.stderr.toString("utf8").trim()
    : String(error?.stderr ?? "").trim();
  const rendered = args.join(" ");
  return new Error(
    `authority tombstone history: git ${rendered} failed${
      stderr ? ` (${stderr})` : ""
    }`,
  );
}

function runGit(repositoryRoot, args, options = {}) {
  try {
    const environment = { ...process.env };
    for (const key of Object.keys(environment)) {
      if (key.startsWith("GIT_CONFIG_")) delete environment[key];
    }
    for (const key of [
      "GIT_DIR",
      "GIT_WORK_TREE",
      "GIT_INDEX_FILE",
      "GIT_OBJECT_DIRECTORY",
      "GIT_ALTERNATE_OBJECT_DIRECTORIES",
      "GIT_COMMON_DIR",
      "GIT_NAMESPACE",
      "GIT_REPLACE_REF_BASE",
    ]) {
      delete environment[key];
    }
    environment.GIT_OPTIONAL_LOCKS = "0";
    environment.GIT_CONFIG_NOSYSTEM = "1";
    environment.GIT_CONFIG_GLOBAL = "/dev/null";
    return execFileSync("/usr/bin/git", ["--no-replace-objects", ...args], {
      cwd: repositoryRoot,
      env: environment,
      encoding: options.encoding ?? "buffer",
      maxBuffer: options.maxBuffer ?? 64 * 1024 * 1024,
      stdio: ["ignore", "pipe", "pipe"],
    });
  } catch (error) {
    if (options.allowFailure) return null;
    throw gitError(repositoryRoot, args, error);
  }
}

function runGitText(repositoryRoot, args, options = {}) {
  const output = runGit(repositoryRoot, args, {
    ...options,
    encoding: "utf8",
  });
  return output === null ? "" : output.trim();
}

function gitObject(repositoryRoot, oid, expectedType) {
  if (!SHA1.test(oid) || /^0+$/u.test(oid)) {
    throw new Error(`authority evidence object ${oid}: invalid object id`);
  }
  const type = runGitText(repositoryRoot, ["cat-file", "-t", oid]);
  if (!GIT_OBJECT_TYPES.includes(type)) {
    throw new Error(`authority evidence object ${oid}: unsupported type ${type}`);
  }
  if (expectedType !== undefined && type !== expectedType) {
    throw new Error(
      `authority evidence object ${oid}: expected ${expectedType}, got ${type}`,
    );
  }
  // `cat-file -p` pretty-prints trees as text, which is not the bytes hashed
  // by Git. The type-specific form returns the raw tree object.
  const body = runGit(
    repositoryRoot,
    ["cat-file", type === "tree" ? "tree" : "-p", oid],
  );
  const header = Buffer.from(`${type} ${body.length}\0`, "utf8");
  const computed = createHash("sha1")
    .update(header)
    .update(body)
    .digest("hex");
  if (computed !== oid) {
    throw new Error(`authority evidence object ${oid}: content hash mismatch`);
  }
  return { oid, type, body };
}

function objectRecordFromGit(repositoryRoot, oid, expectedType) {
  const object = gitObject(repositoryRoot, oid, expectedType);
  return {
    oid: object.oid,
    type: object.type,
    data: object.body.toString("base64"),
  };
}

function isRecord(value) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    return false;
  }
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function fail(path, message) {
  throw new Error(`authority tombstone ${path}: ${message}`);
}

function exactKeys(value, keys, path) {
  if (!isRecord(value)) fail(path, "must be an object");
  const actual = Object.keys(value);
  if (
    actual.length !== keys.length ||
    actual.some((key, index) => key !== keys[index])
  ) {
    fail(
      path,
      `keys must be exactly [${keys.join(", ")}], got [${actual.join(", ")}]`,
    );
  }
}

function exactString(value, path, expected) {
  if (typeof value !== "string") fail(path, "must be a string");
  if (expected !== undefined && value !== expected) {
    fail(path, `must equal ${JSON.stringify(expected)}`);
  }
}

function exactBoolean(value, path, expected) {
  if (typeof value !== "boolean" || value !== expected) {
    fail(path, `must equal ${JSON.stringify(expected)}`);
  }
}

function exactPositiveInteger(value, path, expected) {
  if (!Number.isSafeInteger(value) || value <= 0) {
    fail(path, "must be a positive safe integer");
  }
  if (expected !== undefined && value !== expected) {
    fail(path, `must equal ${expected}`);
  }
}

function exactCommit(value, path, expected) {
  exactString(value, path);
  if (!COMMIT.test(value) || /^0+$/u.test(value)) {
    fail(path, "must be a non-zero lowercase 40-character commit");
  }
  if (expected !== undefined && value !== expected) {
    fail(path, `must equal ${expected}`);
  }
}

function exactObjectOid(value, path, expected) {
  exactString(value, path);
  if (!OBJECT_OID.test(value) || /^0+$/u.test(value)) {
    fail(path, "must be a non-zero lowercase 40-character Git object id");
  }
  if (expected !== undefined && value !== expected) {
    fail(path, `must equal ${expected}`);
  }
}

function exactSha256(value, path, expected) {
  exactString(value, path);
  if (!SHA256.test(value)) fail(path, "must be sha256:<64 lowercase hex>");
  if (expected !== undefined && value !== expected) {
    fail(path, `must equal ${expected}`);
  }
}

function exactArray(value, path, expected) {
  if (!Array.isArray(value)) fail(path, "must be an array");
  if (
    value.length !== expected.length ||
    value.some((entry, index) => entry !== expected[index])
  ) {
    fail(path, `must equal ${JSON.stringify(expected)}`);
  }
}

function exactBlobMap(value, path, expected) {
  exactKeys(value, Object.keys(expected), path);
  for (const [relativePath, oid] of Object.entries(expected)) {
    exactObjectOid(value[relativePath], `${path}.${relativePath}`, oid);
  }
}

function validateInstant(value, path) {
  exactString(value, path);
  if (!INSTANT.test(value)) {
    fail(path, "must be a canonical UTC RFC3339 instant");
  }
  const parsed = new Date(value);
  if (!Number.isFinite(parsed.getTime())) fail(path, "must be a valid instant");
  const canonical = parsed.toISOString();
  const expectedCanonical = value.endsWith(".000Z")
    ? value
    : value.replace(/Z$/u, ".000Z");
  if (canonical !== expectedCanonical) {
    fail(path, "must be a real calendar instant");
  }
}

function validateSpecification(value) {
  exactKeys(value, SPECIFICATION_KEYS, "publishedSpecification");
  exactString(value.version, "publishedSpecification.version", "1.1");
  exactString(
    value.tag,
    "publishedSpecification.tag",
    W09_SPECIFICATION.tag,
  );
  exactObjectOid(
    value.tagObject,
    "publishedSpecification.tagObject",
    W09_SPECIFICATION.tagObject,
  );
  exactCommit(
    value.sourceCommit,
    "publishedSpecification.sourceCommit",
    W09_SPECIFICATION.sourceCommit,
  );
  exactCommit(
    value.releaseCommit,
    "publishedSpecification.releaseCommit",
    W09_SPECIFICATION.releaseCommit,
  );
  exactPositiveInteger(
    value.releaseId,
    "publishedSpecification.releaseId",
    W09_SPECIFICATION.releaseId,
  );
  exactString(
    value.releaseUrl,
    "publishedSpecification.releaseUrl",
    W09_SPECIFICATION.releaseUrl,
  );
  exactBoolean(value.immutable, "publishedSpecification.immutable", true);
  exactPositiveInteger(
    value.assetId,
    "publishedSpecification.assetId",
    W09_SPECIFICATION.assetId,
  );
  exactString(
    value.assetName,
    "publishedSpecification.assetName",
    W09_SPECIFICATION.assetName,
  );
  exactString(
    value.assetSourcePath,
    "publishedSpecification.assetSourcePath",
    W09_SPECIFICATION.assetSourcePath,
  );
  exactSha256(
    value.assetSha256,
    "publishedSpecification.assetSha256",
    W09_SPECIFICATION.assetSha256,
  );
  exactSha256(
    value.sourceSnapshotSha256,
    "publishedSpecification.sourceSnapshotSha256",
    W09_SPECIFICATION.sourceSnapshotSha256,
  );
  exactSha256(
    value.sourceEvidenceSha256,
    "publishedSpecification.sourceEvidenceSha256",
    W09_SPECIFICATION.sourceEvidenceSha256,
  );
}

function validateSigners(value) {
  exactKeys(value, SIGNER_KEYS, "signers");
  exactString(
    value.predecessorProviderSigningFingerprint,
    "signers.predecessorProviderSigningFingerprint",
    PREDECESSOR_PROVIDER_SIGNING_FINGERPRINT,
  );
  exactString(
    value.successorTagSignerFingerprint,
    "signers.successorTagSignerFingerprint",
    SUCCESSOR_TAG_SIGNER_FINGERPRINT,
  );
  exactSha256(
    value.successorRecordHeadSpkiSha256,
    "signers.successorRecordHeadSpkiSha256",
    SUCCESSOR_RECORD_HEAD_SPKI_SHA256,
  );
}

function validateEvidenceObjectRecord(value, path) {
  exactKeys(value, EVIDENCE_OBJECT_KEYS, path);
  exactObjectOid(value.oid, `${path}.oid`);
  exactString(value.type, `${path}.type`);
  if (!GIT_OBJECT_TYPES.includes(value.type)) {
    fail(`${path}.type`, `must be one of ${GIT_OBJECT_TYPES.join(", ")}`);
  }
  exactString(value.data, `${path}.data`);
  if (!BASE64.test(value.data) || value.data.length % 4 !== 0) {
    fail(`${path}.data`, "must be canonical base64");
  }
  let body;
  try {
    body = Buffer.from(value.data, "base64");
  } catch (error) {
    fail(`${path}.data`, `invalid base64 (${error.message})`);
  }
  const computed = createHash("sha1")
    .update(Buffer.from(`${value.type} ${body.length}\0`, "utf8"))
    .update(body)
    .digest("hex");
  if (computed !== value.oid) {
    fail(`${path}.data`, `Git object content hash does not equal ${value.oid}`);
  }
  return { oid: value.oid, type: value.type, body };
}

function parseCommitObject(object, path) {
  if (object.type !== "commit") fail(path, "must be a commit object");
  const text = object.body.toString("utf8");
  const separator = text.indexOf("\n\n");
  const headers = (separator === -1 ? text : text.slice(0, separator)).split(
    "\n",
  );
  const tree = headers.find((line) => line.startsWith("tree "))?.slice(5);
  const parents = headers
    .filter((line) => line.startsWith("parent "))
    .map((line) => line.slice(7));
  if (!SHA1.test(tree ?? "")) fail(`${path}.tree`, "must name a Git tree");
  if (
    parents.some((parent) => !SHA1.test(parent) || /^0+$/u.test(parent))
  ) {
    fail(`${path}.parents`, "must contain valid Git commit ids");
  }
  return { tree, parents };
}

function parseTreeObject(object, path) {
  if (object.type !== "tree") fail(path, "must be a tree object");
  const entries = [];
  let offset = 0;
  while (offset < object.body.length) {
    const separator = object.body.indexOf(0, offset);
    if (separator === -1) fail(path, "contains an unterminated tree entry");
    const header = object.body.slice(offset, separator).toString("utf8");
    const split = header.indexOf(" ");
    if (split <= 0) fail(path, "contains a malformed tree mode/name");
    const mode = header.slice(0, split);
    const name = header.slice(split + 1);
    if (!/^[0-7]{5,6}$/u.test(mode) || name.length === 0) {
      fail(path, "contains an invalid tree mode or name");
    }
    const oidStart = separator + 1;
    const oidEnd = oidStart + 20;
    if (oidEnd > object.body.length) fail(path, "contains a truncated tree id");
    const oid = object.body.slice(oidStart, oidEnd).toString("hex");
    if (!SHA1.test(oid) || /^0+$/u.test(oid)) {
      fail(path, "contains an invalid tree object id");
    }
    if (name === "." || name === ".." || name.includes("/")) {
      fail(path, "contains an invalid tree entry name");
    }
    entries.push({ mode, name, oid });
    offset = oidEnd;
  }
  return entries;
}

function evidenceObjectMap(objects, path) {
  if (!Array.isArray(objects) || objects.length === 0) {
    fail(path, "must be a non-empty object array");
  }
  const map = new Map();
  objects.forEach((value, index) => {
    const object = validateEvidenceObjectRecord(value, `${path}[${index}]`);
    if (map.has(object.oid)) fail(`${path}[${index}].oid`, "is duplicated");
    map.set(object.oid, object);
  });
  return map;
}

function flattenTreeObjects(objects, rootTree, path) {
  const paths = new Map();
  const visited = new Set();
  function visit(treeOid, prefix, treePath) {
    if (visited.has(treeOid)) return;
    visited.add(treeOid);
    const tree = objects.get(treeOid);
    if (!tree) fail(`${treePath}.objects`, `missing tree object ${treeOid}`);
    const entries = parseTreeObject(tree, `${treePath}.${treeOid}`);
    for (const entry of entries) {
      const relativePath = prefix ? `${prefix}/${entry.name}` : entry.name;
      const isTree = entry.mode === "040000" || entry.mode === "40000";
      if (isTree) {
        const child = objects.get(entry.oid);
        if (!child) {
          fail(
            `${treePath}.objects`,
            `missing child tree object ${entry.oid} for ${relativePath}`,
          );
        }
        if (child.type !== "tree") {
          fail(
            `${treePath}.objects`,
            `tree entry ${relativePath} points to ${child.type}, not tree`,
          );
        }
        visit(entry.oid, relativePath, treePath);
      } else {
        if (paths.has(relativePath)) {
          fail(`${treePath}.paths`, `duplicate path ${relativePath}`);
        }
        paths.set(relativePath, { mode: entry.mode, oid: entry.oid });
      }
    }
  }
  visit(rootTree, "", path);
  return paths;
}

function coreAuthorityProblem(path, message) {
  fail(`${path}`, `Core authority record ${message}`);
}

function parseCanonicalJsonBlob(object, path) {
  if (object.type !== "blob") coreAuthorityProblem(path, "must be a blob");
  const raw = object.body.toString("utf8");
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    coreAuthorityProblem(path, `is not valid JSON (${error.message})`);
  }
  if (!isRecord(parsed)) coreAuthorityProblem(path, "must be a JSON object");
  // Core's canonical authority record is checked as exact pretty JSON. This
  // rejects duplicate keys and whitespace/encoding variants that JSON.parse
  // alone would silently normalize.
  if (`${JSON.stringify(parsed, null, 2)}\n` !== raw) {
    coreAuthorityProblem(path, "is not canonical JSON bytes");
  }
  return parsed;
}

function validateCoreAuthorityRecord(value, path, expectedPreparedCommit) {
  if (!isRecord(value)) coreAuthorityProblem(path, "must be an object");
  const keys = Object.keys(value).sort();
  if (JSON.stringify(keys) !== JSON.stringify([...CORE_AUTHORITY_KEYS].sort())) {
    coreAuthorityProblem(path, "has an unexpected field set");
  }
  if (value.format !== CORE_AUTHORITY_FORMAT) {
    coreAuthorityProblem(path, "has an unknown format");
  }
  if (value.state !== CORE_AUTHORITY_STATE) {
    coreAuthorityProblem(path, `must remain ${CORE_AUTHORITY_STATE}`);
  }
  if (
    value.predecessorRepository !== PREDECESSOR_REPOSITORY ||
    value.predecessorCutoffCommit !== PREDECESSOR_CUTOFF_COMMIT ||
    value.predecessorCutoffTree !== PREDECESSOR_CUTOFF_TREE ||
    value.successorRepository !== SUCCESSOR_REPOSITORY
  ) {
    coreAuthorityProblem(path, "changed a fixed repository or cutoff identity");
  }
  if (value.rollback !== CORE_ROLLBACK) {
    coreAuthorityProblem(path, "changed the fixed rollback policy");
  }
  if (
    !isRecord(value.lastPredecessorSpecificationRelease) ||
    JSON.stringify(Object.keys(value.lastPredecessorSpecificationRelease).sort()) !==
      JSON.stringify([...CORE_LAST_SPECIFICATION_KEYS].sort()) ||
    JSON.stringify(canonicalize(value.lastPredecessorSpecificationRelease)) !==
      JSON.stringify(canonicalize(CORE_LAST_SPECIFICATION))
  ) {
    coreAuthorityProblem(path, "changed the fixed last Specification release");
  }
  if (value.predecessorTombstoneCommit !== null) {
    coreAuthorityProblem(path, "must leave predecessorTombstoneCommit null");
  }
  if (value.schemaRouteCutover !== null) {
    coreAuthorityProblem(path, "must leave schemaRouteCutover null");
  }
  if (value.predecessorWriterDisabledAt !== null) {
    coreAuthorityProblem(path, "must leave predecessorWriterDisabledAt null");
  }
  if (value.successorWriterEnabledAt !== null) {
    coreAuthorityProblem(path, "must leave successorWriterEnabledAt null");
  }
  if (value.writerOverlapAllowed !== false) {
    coreAuthorityProblem(path, "must forbid writer overlap");
  }
  if (expectedPreparedCommit === null) {
    if (value.successorPreparedCommit !== null) {
      coreAuthorityProblem(path, "P0 must leave successorPreparedCommit null");
    }
  } else {
    exactCommit(
      value.successorPreparedCommit,
      `${path}.successorPreparedCommit`,
      expectedPreparedCommit,
    );
  }
  return value;
}

function validateCoreAuthorityTransition(objects, preparedRecord, authorityRecord, preparedCommit) {
  const preparedObject = objects.get(preparedRecord.oid);
  const authorityObject = objects.get(authorityRecord.oid);
  const prepared = parseCanonicalJsonBlob(
    preparedObject,
    `prepared canonical path ${CORE_AUTHORITY_RECORD_PATH}`,
  );
  const authority = parseCanonicalJsonBlob(
    authorityObject,
    `authority canonical path ${CORE_AUTHORITY_RECORD_PATH}`,
  );
  validateCoreAuthorityRecord(prepared, "prepared canonical path", null);
  validateCoreAuthorityRecord(authority, "authority canonical path", preparedCommit);
  const expectedAuthority = {
    ...structuredClone(prepared),
    successorPreparedCommit: preparedCommit,
  };
  if (
    JSON.stringify(canonicalize(authority)) !==
    JSON.stringify(canonicalize(expectedAuthority))
  ) {
    coreAuthorityProblem(
      "authority canonical path",
      "P may change only successorPreparedCommit from P0",
    );
  }
}

function validateEvidenceDocument(value, tombstone, path = "evidence") {
  exactKeys(value, EVIDENCE_DOCUMENT_KEYS, path);
  exactString(value.format, `${path}.format`, AUTHORITY_EVIDENCE_FORMAT);
  exactSha256(
    value.tombstoneSha256,
    `${path}.tombstoneSha256`,
    tombstone.authorityEvidence?.tombstoneSha256,
  );
  exactString(value.canonicalPath, `${path}.canonicalPath`, CORE_AUTHORITY_RECORD_PATH);
  exactCommit(value.preparedCommit, `${path}.preparedCommit`, tombstone.successorPreparedCommit);
  exactCommit(value.authorityCommit, `${path}.authorityCommit`, tombstone.successorAuthorityCommit);
  exactObjectOid(value.preparedTree, `${path}.preparedTree`);
  exactObjectOid(value.authorityTree, `${path}.authorityTree`);
  exactArray(value.changedPaths, `${path}.changedPaths`, [CORE_AUTHORITY_RECORD_PATH]);

  const objects = evidenceObjectMap(value.objects, `${path}.objects`);
  const preparedCommitObject = objects.get(value.preparedCommit);
  const authorityCommitObject = objects.get(value.authorityCommit);
  if (!preparedCommitObject) {
    fail(`${path}.objects`, `missing prepared commit object ${value.preparedCommit}`);
  }
  if (!authorityCommitObject) {
    fail(`${path}.objects`, `missing authority commit object ${value.authorityCommit}`);
  }
  const preparedCommit = parseCommitObject(
    preparedCommitObject,
    `${path}.objects.${value.preparedCommit}`,
  );
  const authorityCommit = parseCommitObject(
    authorityCommitObject,
    `${path}.objects.${value.authorityCommit}`,
  );
  if (preparedCommit.tree !== value.preparedTree) {
    fail(`${path}.preparedTree`, "does not match the prepared commit tree");
  }
  if (authorityCommit.tree !== value.authorityTree) {
    fail(`${path}.authorityTree`, "does not match the authority commit tree");
  }
  if (
    authorityCommit.parents.length !== 1 ||
    authorityCommit.parents[0] !== value.preparedCommit
  ) {
    fail(
      `${path}.authorityCommit`,
      "must be a direct child of successorPreparedCommit (exactly one parent)",
    );
  }
  const preparedPaths = flattenTreeObjects(
    objects,
    value.preparedTree,
    `${path}.preparedTree`,
  );
  const authorityPaths = flattenTreeObjects(
    objects,
    value.authorityTree,
    `${path}.authorityTree`,
  );
  const changedPaths = [];
  const allPaths = new Set([...preparedPaths.keys(), ...authorityPaths.keys()]);
  for (const changedPath of allPaths) {
    const before = preparedPaths.get(changedPath);
    const after = authorityPaths.get(changedPath);
    if (
      before?.mode !== after?.mode ||
      before?.oid !== after?.oid
    ) {
      changedPaths.push(changedPath);
    }
  }
  changedPaths.sort();
  if (JSON.stringify(changedPaths) !== JSON.stringify([CORE_AUTHORITY_RECORD_PATH])) {
    fail(
      `${path}.changedPaths`,
      `Git trees prove changed paths ${JSON.stringify(changedPaths)}, expected only ${CORE_AUTHORITY_RECORD_PATH}`,
    );
  }
  const preparedRecord = preparedPaths.get(CORE_AUTHORITY_RECORD_PATH);
  const authorityRecord = authorityPaths.get(CORE_AUTHORITY_RECORD_PATH);
  if (!preparedRecord || !authorityRecord) {
    fail(
      `${path}.canonicalPath`,
      "must exist in both prepared and authority trees",
    );
  }
  if (preparedRecord.oid === authorityRecord.oid) {
    fail(`${path}.canonicalPath`, "must change between prepared and authority trees");
  }
  for (const [label, record] of [
    ["prepared", preparedRecord],
    ["authority", authorityRecord],
  ]) {
    const object = objects.get(record.oid);
    if (!object) {
      fail(`${path}.objects`, `missing ${label} canonical record blob ${record.oid}`);
    }
    if (object.type !== "blob") {
      fail(`${path}.objects`, `${label} canonical record ${record.oid} is not a blob`);
    }
  }
  validateCoreAuthorityTransition(
    objects,
    preparedRecord,
    authorityRecord,
    value.preparedCommit,
  );
  return {
    objects,
    preparedPaths,
    authorityPaths,
    preparedRecord,
    authorityRecord,
  };
}

function validateAuthorityEvidenceFile(repositoryRoot, tombstone) {
  const evidence = tombstone.authorityEvidence;
  if (!evidence) fail("authorityEvidence", "must be present while status is active");
  const path = resolve(repositoryRoot, evidence.path);
  if (!existsSync(path)) fail("authorityEvidence.path", `missing ${evidence.path}`);
  const bytes = readFileSync(path);
  const digest = `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
  if (digest !== evidence.sha256) {
    fail("authorityEvidence.sha256", `does not match ${evidence.path}`);
  }
  let parsed;
  try {
    parsed = JSON.parse(bytes.toString("utf8"));
  } catch (error) {
    fail("authorityEvidence.file", `invalid JSON (${error.message})`);
  }
  validateEvidenceDocument(parsed, tombstone, "authorityEvidence.file");
  return parsed;
}

function validateAuthorityEvidenceMetadata(value, tombstone) {
  exactKeys(value, AUTHORITY_EVIDENCE_KEYS, "authorityEvidence");
  exactString(value.format, "authorityEvidence.format", AUTHORITY_EVIDENCE_FORMAT);
  exactString(value.path, "authorityEvidence.path", AUTHORITY_EVIDENCE_PATH);
  exactSha256(value.sha256, "authorityEvidence.sha256");
  exactSha256(
    value.tombstoneSha256,
    "authorityEvidence.tombstoneSha256",
    authorityTombstoneDigest(tombstone),
  );
  exactString(
    value.canonicalPath,
    "authorityEvidence.canonicalPath",
    CORE_AUTHORITY_RECORD_PATH,
  );
  exactCommit(
    value.preparedCommit,
    "authorityEvidence.preparedCommit",
    tombstone.successorPreparedCommit,
  );
  exactCommit(
    value.authorityCommit,
    "authorityEvidence.authorityCommit",
    tombstone.successorAuthorityCommit,
  );
}

/**
 * Validate local Git history without contacting the predecessor or successor
 * repository.  The predecessor cutoff is a hard floor: a shallow checkout,
 * missing object, or an ancestry that cannot be proven is unsafe.
 */
export function assertAuthorityHistoryContinuity(
  repositoryRoot = moduleRoot,
  currentTombstone,
) {
  const gitDirectory = runGitText(
    repositoryRoot,
    ["rev-parse", "--git-dir"],
    { allowFailure: true },
  );
  if (!gitDirectory) {
    throw new Error(
      "authority tombstone history continuity cannot be proven: repository is not a Git checkout",
    );
  }
  const shallow = runGitText(repositoryRoot, ["rev-parse", "--is-shallow-repository"], {
    allowFailure: true,
  });
  if (shallow !== "false") {
    throw new Error(
      "authority tombstone history continuity cannot be proven: checkout is shallow or Git did not report its depth",
    );
  }
  const anchorExists = runGit(
    repositoryRoot,
    ["cat-file", "-e", `${PREDECESSOR_CUTOFF_COMMIT}^{commit}`],
    { allowFailure: true },
  );
  if (anchorExists === null) {
    throw new Error(
      `authority tombstone history continuity cannot be proven: missing predecessor cutoff commit ${PREDECESSOR_CUTOFF_COMMIT}`,
    );
  }
  const anchorTree = runGitText(repositoryRoot, ["rev-parse", `${PREDECESSOR_CUTOFF_COMMIT}^{tree}`], {
    allowFailure: true,
  });
  if (anchorTree !== PREDECESSOR_CUTOFF_TREE) {
    throw new Error(
      `authority tombstone history continuity cannot be proven: predecessor cutoff tree mismatch (${anchorTree || "missing"})`,
    );
  }
  const head = runGitText(repositoryRoot, ["rev-parse", "HEAD"], {
    allowFailure: true,
  });
  if (!head || !SHA1.test(head)) {
    throw new Error(
      "authority tombstone history continuity cannot be proven: HEAD is unavailable",
    );
  }
  const ancestor = runGit(
    repositoryRoot,
    ["merge-base", "--is-ancestor", PREDECESSOR_CUTOFF_COMMIT, head],
    { allowFailure: true },
  );
  if (ancestor === null) {
    throw new Error(
      `authority tombstone history continuity cannot be proven: HEAD ${head} is not a descendant of ${PREDECESSOR_CUTOFF_COMMIT}`,
    );
  }

  const commitsText = runGitText(repositoryRoot, ["rev-list", "--first-parent", head]);
  const commits = commitsText ? commitsText.split("\n") : [];
  if (!commits.includes(PREDECESSOR_CUTOFF_COMMIT)) {
    throw new Error(
      "authority tombstone history continuity cannot be proven: first-parent history is incomplete",
    );
  }
  const chronological = commits
    .slice(0, commits.indexOf(PREDECESSOR_CUTOFF_COMMIT) + 1)
    .reverse();
  let activeSeen = false;
  let activeCommit;
  for (const commit of chronological) {
    if (commit === PREDECESSOR_CUTOFF_COMMIT) continue;
    const record = runGit(
      repositoryRoot,
      ["show", `${commit}:${PREDECESSOR_TOMBSTONE_RECORD_PATH}`],
      { allowFailure: true },
    );
    // The record was introduced after the predecessor cutoff.  A missing
    // record is therefore harmless until an active state has been observed.
    if (record === null) {
      if (activeSeen) {
        throw new Error(
          `authority tombstone history continuity rejected ${commit}: active record disappeared after ${activeCommit}`,
        );
      }
      continue;
    }
    let parsed;
    try {
      parsed = JSON.parse(record.toString("utf8"));
    } catch (error) {
      throw new Error(
        `authority tombstone history continuity rejected ${commit}: canonical record is not JSON (${error.message})`,
      );
    }
    const status = parsed?.status;
    if (status !== "pending" && status !== "active") {
      throw new Error(
        `authority tombstone history continuity rejected ${commit}: canonical record has invalid status`,
      );
    }
    if (status === "pending" && activeSeen) {
      throw new Error(
        `authority tombstone history rejected active -> pending transition at ${commit} (active ${activeCommit}); history rewrite belongs to external branch protection`,
      );
    }
    if (status === "active") {
      activeSeen = true;
      activeCommit = commit;
    }
  }

  // First-parent history catches the ordinary linear revert. Also inspect the
  // complete ancestry so a pending side branch cannot be merged after an
  // active commit and reopen the predecessor writer.
  const ancestryText = runGitText(repositoryRoot, [
    "rev-list",
    "--ancestry-path",
    head,
    `^${PREDECESSOR_CUTOFF_COMMIT}`,
  ]);
  const ancestryCommits = ancestryText ? ancestryText.split("\n") : [];
  const states = new Map();
  for (const commit of ancestryCommits) {
    const record = runGit(
      repositoryRoot,
      ["show", `${commit}:${PREDECESSOR_TOMBSTONE_RECORD_PATH}`],
      { allowFailure: true },
    );
    if (record === null) {
      states.set(commit, "missing");
      continue;
    }
    let parsed;
    try {
      parsed = JSON.parse(record.toString("utf8"));
    } catch (error) {
      throw new Error(
        `authority tombstone history continuity rejected ${commit}: canonical record is not JSON (${error.message})`,
      );
    }
    if (parsed?.status !== "pending" && parsed?.status !== "active") {
      throw new Error(
        `authority tombstone history continuity rejected ${commit}: canonical record has invalid status`,
      );
    }
    states.set(commit, parsed.status);
  }
  const activeCommits = [...states.entries()]
    .filter(([, status]) => status === "active")
    .map(([commit]) => commit);
  const pendingOrMissingCommits = [...states.entries()]
    .filter(([, status]) => status === "pending" || status === "missing")
    .map(([commit, status]) => ({ commit, status }));
  for (const activeAncestor of activeCommits) {
    for (const { commit, status } of pendingOrMissingCommits) {
      const descendant = runGit(
        repositoryRoot,
        ["merge-base", "--is-ancestor", activeAncestor, commit],
        { allowFailure: true },
      );
      if (descendant === null) continue;
      if (status === "pending") {
        throw new Error(
          `authority tombstone history rejected active -> pending transition at ${commit} (active ${activeAncestor}); history rewrite belongs to external branch protection`,
        );
      }
      throw new Error(
        `authority tombstone history continuity rejected ${commit}: active record disappeared after ${activeAncestor}`,
      );
    }
  }
  if (currentTombstone?.status === "pending" && activeSeen) {
    throw new Error(
      `authority tombstone history rejected active -> pending working-tree state after ${activeCommit}; history rewrite belongs to external branch protection`,
    );
  }
  return {
    head,
    anchor: PREDECESSOR_CUTOFF_COMMIT,
    activeCommit: activeCommit ?? null,
  };
}

export const validateAuthorityHistory = assertAuthorityHistoryContinuity;

export function validateAuthorityTransitionEvidence(
  repositoryRoot,
  tombstone,
) {
  validateAuthorityTombstone(tombstone, { skipEvidenceMetadata: false });
  if (tombstone.status !== "active") {
    fail("authorityEvidence", "transition evidence is required only for active records");
  }
  return validateAuthorityEvidenceFile(repositoryRoot, tombstone);
}

export const validateAuthorityEvidence = validateAuthorityTransitionEvidence;

/**
 * Validate one parsed tombstone.  This is intentionally independent of the
 * repository or any external state so contract and invocation gates can use
 * it as a pure preflight.
 */
export function validateAuthorityTombstone(value, options = {}) {
  exactKeys(value, TOP_LEVEL_KEYS, "document");
  exactString(value.format, "format", AUTHORITY_TOMBSTONE_FORMAT);
  exactString(value.status, "status");
  if (value.status !== "pending" && value.status !== "active") {
    fail("status", "must be either pending or active");
  }
  exactString(
    value.predecessorRepository,
    "predecessorRepository",
    PREDECESSOR_REPOSITORY,
  );
  exactCommit(
    value.predecessorSourceCommit,
    "predecessorSourceCommit",
    PREDECESSOR_CUTOFF_COMMIT,
  );
  exactObjectOid(
    value.predecessorSourceTree,
    "predecessorSourceTree",
    PREDECESSOR_CUTOFF_TREE,
  );
  exactBlobMap(
    value.predecessorWriterBlobOids,
    "predecessorWriterBlobOids",
    PREDECESSOR_WRITER_BLOB_OIDS,
  );
  exactBlobMap(
    value.predecessorWorkflowBlobOids,
    "predecessorWorkflowBlobOids",
    PREDECESSOR_WORKFLOW_BLOB_OIDS,
  );
  exactString(
    value.successorRepository,
    "successorRepository",
    SUCCESSOR_REPOSITORY,
  );
  if (value.successorPreparedCommit !== null) {
    exactCommit(value.successorPreparedCommit, "successorPreparedCommit");
  }
  if (value.successorAuthorityCommit !== null) {
    exactCommit(value.successorAuthorityCommit, "successorAuthorityCommit");
  }
  if (value.disabledAt !== null) validateInstant(value.disabledAt, "disabledAt");
  if (value.authorityEvidence !== null) {
    validateAuthorityEvidenceMetadata(value.authorityEvidence, value);
  }
  validateSpecification(value.publishedSpecification);
  validateSigners(value.signers);
  exactArray(value.disabledSurfaces, "disabledSurfaces", DISABLED_SURFACES);
  exactArray(value.retainedSurfaces, "retainedSurfaces", RETAINED_SURFACES);

  if (value.status === "pending") {
    if (value.successorPreparedCommit !== null) {
      fail("successorPreparedCommit", "must remain null while status is pending");
    }
    if (value.successorAuthorityCommit !== null) {
      fail("successorAuthorityCommit", "must remain null while status is pending");
    }
    if (value.disabledAt !== null) {
      fail("disabledAt", "must remain null while status is pending");
    }
    if (value.authorityEvidence !== null) {
      fail("authorityEvidence", "must remain null while status is pending");
    }
  } else {
    if (value.successorPreparedCommit === null) {
      fail("successorPreparedCommit", "must be a P0 commit when status is active");
    }
    if (value.successorAuthorityCommit === null) {
      fail("successorAuthorityCommit", "must be a P commit when status is active");
    }
    if (value.successorPreparedCommit === value.successorAuthorityCommit) {
      fail("successorAuthorityCommit", "P must differ from P0");
    }
    if (value.disabledAt === null) {
      fail("disabledAt", "must be set when status is active");
    }
    if (value.authorityEvidence === null) {
      fail("authorityEvidence", "must be set when status is active");
    }
  }
  if (
    (options.activation === true ||
      options.requireActive === true ||
      options.active === true) &&
    value.status !== "active"
  ) {
    fail("status", "pending tombstones cannot activate the predecessor cutover");
  }
  return value;
}

export function assertActiveAuthorityTombstone(value) {
  return validateAuthorityTombstone(value, { activation: true });
}

export const validateAuthorityTombstoneActivation =
  assertActiveAuthorityTombstone;
export const assertAuthorityTombstoneActive = assertActiveAuthorityTombstone;

/** Read and validate the tracked record for a repository root. */
export function readAuthorityTombstone(repositoryRoot = moduleRoot, options = {}) {
  const path = resolve(repositoryRoot, AUTHORITY_TOMBSTONE_PATH);
  if (!existsSync(path)) fail("file", `missing ${AUTHORITY_TOMBSTONE_PATH}`);
  let parsed;
  try {
    parsed = JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    fail("file", `invalid JSON (${error.message})`);
  }
  const tombstone = validateAuthorityTombstone(parsed);
  if (options.history !== false) {
    assertAuthorityHistoryContinuity(repositoryRoot, tombstone);
  }
  if (tombstone.status === "active" && options.evidence !== false) {
    validateAuthorityTransitionEvidence(repositoryRoot, tombstone);
  }
  return tombstone;
}

export const loadAuthorityTombstone = readAuthorityTombstone;

export function isAuthorityTombstoneActive(tombstone) {
  return validateAuthorityTombstone(tombstone).status === "active";
}

export function isAuthorityDisabledSurface(surface, tombstone) {
  return (
    isAuthorityTombstoneActive(tombstone) &&
    tombstone.disabledSurfaces.includes(surface)
  );
}

export function assertAuthorityInvocationAllowed(surface, tombstone) {
  if (isAuthorityDisabledSurface(surface, tombstone)) {
    throw new Error(
      `authority tombstone is active; invocation of ${surface} is disabled`,
    );
  }
  return surface;
}

/**
 * Apply the tombstone to a deploy contract without changing retained entries.
 * Pending records return the exact object, preserving pre-W10 behavior.
 */
export function applyAuthorityTombstoneToContract(contract, tombstone) {
  validateAuthorityTombstone(tombstone);
  if (tombstone.status === "pending") return contract;
  if (!isRecord(contract) || !Array.isArray(contract.surfaces)) {
    throw new Error("deploy contract must contain a surfaces array");
  }
  const disabled = new Set(tombstone.disabledSurfaces);
  return {
    ...contract,
    surfaces: contract.surfaces.filter(
      (entry) => !disabled.has(entry?.surface),
    ),
  };
}

export const filterDeployContract = applyAuthorityTombstoneToContract;

export const authorityTombstoneKeys = Object.freeze({
  topLevel: TOP_LEVEL_KEYS,
  specification: SPECIFICATION_KEYS,
  signers: SIGNER_KEYS,
  evidence: AUTHORITY_EVIDENCE_KEYS,
  evidenceDocument: EVIDENCE_DOCUMENT_KEYS,
  evidenceObject: EVIDENCE_OBJECT_KEYS,
  coreAuthority: CORE_AUTHORITY_KEYS,
  coreLastSpecification: CORE_LAST_SPECIFICATION_KEYS,
});

function collectGitTreeObjects(repositoryRoot, treeOid, objects) {
  if (objects.has(treeOid)) return;
  const tree = gitObject(repositoryRoot, treeOid, "tree");
  objects.set(treeOid, objectRecordFromGit(repositoryRoot, treeOid, "tree"));
  for (const entry of parseTreeObject(tree, `git tree ${treeOid}`)) {
    if (entry.mode === "040000" || entry.mode === "40000") {
      collectGitTreeObjects(repositoryRoot, entry.oid, objects);
    }
  }
}

function collectGitCommitObjects(repositoryRoot, oid, objects) {
  if (objects.has(oid)) return;
  const commit = gitObject(repositoryRoot, oid, "commit");
  const record = objectRecordFromGit(repositoryRoot, oid, "commit");
  objects.set(oid, record);
  const parsed = parseCommitObject(commit, `git commit ${oid}`);
  collectGitTreeObjects(repositoryRoot, parsed.tree, objects);
}

function collectGitBlobForPath(repositoryRoot, commit, path, objects) {
  const oid = runGitText(repositoryRoot, ["rev-parse", `${commit}:${path}`]);
  if (!SHA1.test(oid)) {
    throw new Error(`authority evidence: ${commit}:${path} is not a blob`);
  }
  objects.set(oid, objectRecordFromGit(repositoryRoot, oid, "blob"));
}

/**
 * Build a standalone, content-addressed evidence document from local Git
 * objects.  It never fetches or reads a sibling checkout.  The caller may
 * write the returned bytes to AUTHORITY_EVIDENCE_PATH and put the returned
 * metadata into the active tombstone.
 */
export function createAuthorityTransitionEvidence({
  repositoryRoot = moduleRoot,
  objectRepositoryRoot = repositoryRoot,
  preparedCommit,
  authorityCommit,
  tombstone,
}) {
  exactCommit(preparedCommit, "successorPreparedCommit");
  exactCommit(authorityCommit, "successorAuthorityCommit");
  if (!isRecord(tombstone)) fail("document", "must be an object");
  if (tombstone.status !== "active") {
    fail("document.status", "must be active when generating transition evidence");
  }
  if (tombstone.successorPreparedCommit !== preparedCommit) {
    fail("document.successorPreparedCommit", "does not match preparedCommit");
  }
  if (tombstone.successorAuthorityCommit !== authorityCommit) {
    fail("document.successorAuthorityCommit", "does not match authorityCommit");
  }
  const objects = new Map();
  collectGitCommitObjects(objectRepositoryRoot, preparedCommit, objects);
  collectGitCommitObjects(objectRepositoryRoot, authorityCommit, objects);
  collectGitBlobForPath(
    objectRepositoryRoot,
    preparedCommit,
    CORE_AUTHORITY_RECORD_PATH,
    objects,
  );
  collectGitBlobForPath(
    objectRepositoryRoot,
    authorityCommit,
    CORE_AUTHORITY_RECORD_PATH,
    objects,
  );
  const preparedObject = gitObject(objectRepositoryRoot, preparedCommit, "commit");
  const authorityObject = gitObject(objectRepositoryRoot, authorityCommit, "commit");
  const preparedParsed = parseCommitObject(preparedObject, "preparedCommit");
  const authorityParsed = parseCommitObject(authorityObject, "authorityCommit");
  const objectList = [...objects.values()].sort((left, right) =>
    left.oid.localeCompare(right.oid),
  );
  const evidence = {
    format: AUTHORITY_EVIDENCE_FORMAT,
    tombstoneSha256: authorityTombstoneDigest(tombstone),
    canonicalPath: CORE_AUTHORITY_RECORD_PATH,
    preparedCommit,
    authorityCommit,
    preparedTree: preparedParsed.tree,
    authorityTree: authorityParsed.tree,
    changedPaths: [CORE_AUTHORITY_RECORD_PATH],
    objects: objectList,
  };
  // Validate the object graph before handing bytes to the caller.  This is
  // intentionally done against the standalone representation, not Git's
  // mutable object database.
  validateEvidenceDocument(evidence, {
    ...tombstone,
    authorityEvidence: {
      format: AUTHORITY_EVIDENCE_FORMAT,
      path: AUTHORITY_EVIDENCE_PATH,
      sha256: "sha256:" + "0".repeat(64),
      tombstoneSha256: evidence.tombstoneSha256,
      canonicalPath: CORE_AUTHORITY_RECORD_PATH,
      preparedCommit,
      authorityCommit,
    },
  });
  const bytes = Buffer.from(`${JSON.stringify(evidence, null, 2)}\n`, "utf8");
  const metadata = {
    format: AUTHORITY_EVIDENCE_FORMAT,
    path: AUTHORITY_EVIDENCE_PATH,
    sha256: `sha256:${createHash("sha256").update(bytes).digest("hex")}`,
    tombstoneSha256: evidence.tombstoneSha256,
    canonicalPath: CORE_AUTHORITY_RECORD_PATH,
    preparedCommit,
    authorityCommit,
  };
  const document = {
    ...tombstone,
    authorityEvidence: metadata,
  };
  validateAuthorityTombstone(document, { activation: true });
  return { evidence, bytes, metadata, document };
}

export const buildAuthorityTransitionEvidence = createAuthorityTransitionEvidence;

export function writeAuthorityTransitionEvidence(
  repositoryRoot,
  transition,
) {
  if (!transition?.bytes || !transition?.metadata) {
    throw new Error("authority evidence transition is incomplete");
  }
  const target = resolve(repositoryRoot, transition.metadata.path);
  const temporary = `${target}.tmp-${process.pid}`;
  const directory = dirname(target);
  if (!existsSync(directory)) {
    mkdirSync(directory, { recursive: true });
  }
  writeFileSync(temporary, transition.bytes, { mode: 0o644 });
  renameSync(temporary, target);
  return target;
}

export function activateAuthorityTombstone({
  repositoryRoot = moduleRoot,
  objectRepositoryRoot = repositoryRoot,
  preparedCommit,
  authorityCommit,
  disabledAt,
}) {
  const pending = readAuthorityTombstone(repositoryRoot, {
    evidence: false,
  });
  if (pending.status !== "pending") {
    fail("status", "activation helper requires the tracked record to be pending");
  }
  validateInstant(disabledAt, "disabledAt");
  const active = {
    ...pending,
    status: "active",
    successorPreparedCommit: preparedCommit,
    successorAuthorityCommit: authorityCommit,
    disabledAt,
    authorityEvidence: null,
  };
  const transition = createAuthorityTransitionEvidence({
    repositoryRoot,
    objectRepositoryRoot,
    preparedCommit,
    authorityCommit,
    tombstone: active,
  });
  writeAuthorityTransitionEvidence(repositoryRoot, transition);
  const recordPath = resolve(repositoryRoot, AUTHORITY_TOMBSTONE_PATH);
  const temporary = `${recordPath}.tmp-${process.pid}`;
  writeFileSync(temporary, `${JSON.stringify(transition.document, null, 2)}\n`, {
    mode: 0o644,
  });
  renameSync(temporary, recordPath);
  return transition.document;
}

export const activateAuthorityTransition = activateAuthorityTombstone;

if (import.meta.main) {
  const mode = process.argv[2] ?? "--check";
  try {
    if (mode === "--check") {
      const tombstone = readAuthorityTombstone(moduleRoot);
      process.stdout.write(
        `authority tombstone OK: ${tombstone.status} (${AUTHORITY_TOMBSTONE_PATH})\n`,
      );
    } else if (mode === "--activate") {
      const prepared = process.argv
        .find((argument) => argument.startsWith("--prepared="))
        ?.slice("--prepared=".length);
      const authority = process.argv
        .find((argument) => argument.startsWith("--authority="))
        ?.slice("--authority=".length);
      const disabledAt = process.argv
        .find((argument) => argument.startsWith("--disabled-at="))
        ?.slice("--disabled-at=".length);
      const objectRepositoryRoot = process.argv
        .find((argument) => argument.startsWith("--objects-from="))
        ?.slice("--objects-from=".length);
      if (!prepared || !authority || !disabledAt) {
        throw new Error(
          "usage: bun scripts/authority-tombstone.mjs --activate --prepared=<P0> --authority=<P> --disabled-at=<RFC3339> [--objects-from=<local-checkout>]",
        );
      }
      const tombstone = activateAuthorityTombstone({
        repositoryRoot: moduleRoot,
        objectRepositoryRoot: objectRepositoryRoot || moduleRoot,
        preparedCommit: prepared,
        authorityCommit: authority,
        disabledAt,
      });
      process.stdout.write(
        `authority tombstone activated: ${tombstone.successorPreparedCommit} -> ${tombstone.successorAuthorityCommit}\n`,
      );
    } else {
      throw new Error(
        "usage: bun scripts/authority-tombstone.mjs [--check|--activate --prepared=<P0> --authority=<P> --disabled-at=<RFC3339> [--objects-from=<local-checkout>]]",
      );
    }
  } catch (error) {
    process.stderr.write(`authority tombstone blocked: ${error.message}\n`);
    process.exitCode = 1;
  }
}

import {
  afterAll,
  beforeAll,
  describe,
  expect,
  setDefaultTimeout,
  test,
} from "bun:test";
import { execFileSync, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  CANDIDATE_CORPUS_FORMAT,
  CANONICAL_ORIGIN,
  CONFORMANCE_SUITE_FORMAT,
  CONFORMANCE_SUITE_PATH,
  EDGE_CANDIDATE_SIZE,
  EDGE_FAMILY_GROUP,
  FAMILY_INDEX_FORMAT,
  FAMILY_INDEX_PATH,
  HOST_API_LANE,
  PACKAGE_ENVELOPE,
  PROVIDER_COMPATIBILITY_TESTS,
  PROVIDER_TRACK,
  PUBLICATION_EVIDENCE_PROJECTION_PATHS,
  REFERENCE_CONFORMANCE_FORMAT,
  S3_STANDARD_SERVICE_PROTOCOL,
  SPECIFICATION_SOURCE_FORMAT,
  SPECIFICATION_TRACK,
  STANDARD_SERVICE_API,
  STANDARD_SERVICE_PROTOCOL_PATTERN,
  assertPublicationEvidenceReady,
  canonicalJson,
  deriveCandidateCorpusEvidence,
  deriveReferenceConformanceEvidence,
  deriveSpecificationSourceSnapshot,
  loadPublicationEvidence,
  parseProviderCompatibilityOutput,
  parseProviderMatrixObservations,
  prepareSpecificationEvidence,
  validateProviderIdentityProjection,
  validatePublicationEvidence,
} from "./publication-evidence.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const DOCUMENT = loadPublicationEvidence(ROOT);
const rootsToRemove = new Set();

// Reference-evidence adversarial cases repeatedly clone and hash the complete
// committed repository. The generated website is intentionally part of that
// closure, so a five-second unit-test default is not a meaningful correctness
// limit as the immutable public corpus grows.
setDefaultTimeout(30_000);

function clone(value) {
  return structuredClone(value);
}

function git(root, ...args) {
  return execFileSync("git", ["-C", root, ...args], {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function sha256Bytes(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function sha256File(root, relativePath) {
  return sha256Bytes(readFileSync(path.join(root, relativePath)));
}

function canonicalDigest(value) {
  return sha256Bytes(Buffer.from(canonicalJson(value), "utf8"));
}

describe("canonical JSON numeric domain", () => {
  test("accepts finite fractional values without admitting unsafe integers", () => {
    expect(canonicalJson({ negative: -0.125, positive: 1.5 })).toBe(
      '{"negative":-0.125,"positive":1.5}',
    );
    for (const value of [Number.MAX_SAFE_INTEGER + 1, Number.MIN_SAFE_INTEGER - 1]) {
      expect(() => canonicalJson({ value })).toThrow("non-safe integer");
    }
    for (const value of [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
      expect(() => canonicalJson({ value })).toThrow("non-finite number");
    }
  });
});

function writeText(root, relativePath, value) {
  const absolutePath = path.join(root, relativePath);
  mkdirSync(path.dirname(absolutePath), { recursive: true });
  writeFileSync(absolutePath, value);
}

function writeJson(root, relativePath, value) {
  writeText(root, relativePath, `${JSON.stringify(value, null, 2)}\n`);
}

function makeCanonicalClone(source, commit, prefix = "takoform-publication-evidence-") {
  const root = mkdtempSync(path.join(tmpdir(), prefix));
  rootsToRemove.add(root);
  execFileSync("git", ["clone", "--no-local", source, root], {
    stdio: "ignore",
  });
  git(root, "checkout", "--detach", commit);
  git(root, "config", "user.name", "Publication Evidence Test");
  git(root, "config", "user.email", "publication-evidence@example.invalid");
  git(root, "remote", "set-url", "origin", CANONICAL_ORIGIN);
  git(root, "update-ref", "refs/remotes/origin/main", commit);
  return root;
}

function makeClone() {
  return makeCanonicalClone(ROOT, DOCUMENT.candidateBaseline.commit);
}

function slug(kind) {
  return kind
    .replace(/([a-z0-9])([A-Z])/gu, "$1-$2")
    .replace(/([A-Z])([A-Z][a-z])/gu, "$1-$2")
    .toLowerCase();
}

function ordinaryDesiredSchema(kind) {
  return {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: {
      name: { type: "string", minLength: 1, maxLength: 63 },
    },
    required: kind === "ModuleWorker" ? ["name"] : [],
  };
}

function workerVersionDesiredSchema({ closedProtocolEnum = false, protocolAllOf = false } = {}) {
  const protocol = {
    type: "string",
    maxLength: 253,
    pattern: STANDARD_SERVICE_PROTOCOL_PATTERN,
  };
  if (closedProtocolEnum) protocol.enum = [S3_STANDARD_SERVICE_PROTOCOL];
  if (protocolAllOf) {
    protocol.allOf = [{ const: S3_STANDARD_SERVICE_PROTOCOL }];
  }
  return {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: {
      externalServices: {
        type: "array",
        maxItems: 16,
        uniqueItems: true,
        default: [],
        "x-takoform-standard-services": STANDARD_SERVICE_API,
        items: {
          type: "object",
          additionalProperties: false,
          properties: {
            name: {
              type: "string",
              pattern: "^[A-Z][A-Z0-9_]*$",
              maxLength: 64,
            },
            required: { type: "boolean", default: true },
            service: {
              type: "object",
              additionalProperties: false,
              properties: {
                apiVersion: { const: STANDARD_SERVICE_API },
                protocol,
              },
              required: ["apiVersion", "protocol"],
            },
          },
          required: ["name", "service"],
        },
      },
    },
  };
}

function buildFamily(root, group, kinds, options = {}) {
  const familyRoot = `forms/candidates/${group}`;
  rmSync(path.join(root, familyRoot), { recursive: true, force: true });
  const forms = [];
  const definitions = new Map();
  for (const kind of kinds) {
    const role = kind.includes("Version") || kind.includes("Bundle") ? "revision" : "identity";
    const desiredSchema = kind === "WorkerVersion"
      ? workerVersionDesiredSchema(options)
      : ordinaryDesiredSchema(kind);
    const definition = {
      apiVersion: group,
      kind,
      definitionVersion: "0.1.0",
      title: `${kind} synthetic candidate`,
      description: `Committed synthetic ${kind} candidate used to prove the publication verifier.`,
      role,
      requiresHostApi: HOST_API_LANE,
      desiredSchema,
      lifecycleCapabilities: ["create", "read", "delete"],
    };
    const relativePackagePath = `${familyRoot}/${slug(kind)}`;
    const definitionPath = `${relativePackagePath}/definition.json`;
    writeJson(root, definitionPath, definition);
    const definitionBytes = readFileSync(path.join(root, definitionPath));
    const formRef = {
      apiVersion: group,
      kind,
      definitionVersion: "0.1.0",
      schemaDigest: `sha256:${canonicalDigest(definition)}`,
    };
    const manifest = {
      apiVersion: PACKAGE_ENVELOPE,
      kind: "FormPackage",
      formRef,
      definitionPath: "definition.json",
      files: [
        {
          path: "definition.json",
          mediaType: "application/vnd.takoform.form-definition.v1+json",
          size: definitionBytes.length,
          digest: `sha256:${sha256Bytes(definitionBytes)}`,
        },
      ],
    };
    writeJson(root, `${relativePackagePath}/package-index.json`, manifest);
    forms.push({
      kind,
      role,
      path: relativePackagePath,
      formRef,
      packageDigest: `sha256:${canonicalDigest(manifest)}`,
    });
    definitions.set(kind, definition);
  }
  const candidateSet = {
    format: "takoform.form-family-candidates@v1",
    family: group,
    formMaturity: "experimental",
    packageApiVersion: PACKAGE_ENVELOPE,
    publicationStatus: "unpublished",
    authoringSource: `synthetic/${group}`,
    authoringPolicy: "provider-neutral-family-renderer",
    forms,
  };
  const candidateSetPath = `${familyRoot}/candidate-set.json`;
  writeJson(root, candidateSetPath, candidateSet);
  return { group, candidateSetPath, candidateSet, definitions };
}

function buildAggregateCandidateSet(root, type, entries) {
  const plural = type === "interface" ? "interfaces" : "bindings";
  const version = type === "interface" ? "v1" : "v1";
  const base = `${plural}/candidates/${version}`;
  const identities = [];
  for (const entry of entries) {
    const definition = {
      apiVersion: `${plural}.takoform.com/${version}`,
      kind: type === "interface" ? "InterfaceDefinition" : "BindingDefinition",
      name: entry.name,
      version: "1.0.0",
      title: `${entry.name} synthetic ${type}`,
      description: `Provider-neutral ${type} identity for publication closure.`,
      operations: [],
    };
    writeJson(root, `${base}/${entry.name}/definition.json`, definition);
    identities.push({
      name: entry.name,
      version: "1.0.0",
      schemaDigest: `sha256:${canonicalDigest(definition)}`,
    });
  }
  const candidateSet = {
    format: type === "interface"
      ? "takoform.interface-candidates@v1"
      : "takoform.binding-candidates@v1",
    publicationStatus: "unpublished",
    authoringSource: "synthetic/current-family-aggregate",
    [plural]: identities,
  };
  const candidateSetPath = `${base}/candidate-set.json`;
  writeJson(root, candidateSetPath, candidateSet);
  return { path: candidateSetPath, sha256: sha256File(root, candidateSetPath) };
}

function buildFamilyCorpus(root, family, options = {}) {
  const corpusPath = `conformance/takoform-v1/families/${family.group}/corpus.json`;
  const runnerInput = {};
  for (const entry of family.candidateSet.forms) {
    const desiredSchemaPath = `fixtures/${slug(entry.kind)}-desired-schema.json`;
    const absoluteDesiredPath = path.posix.join(path.posix.dirname(corpusPath), desiredSchemaPath);
    const desiredSchema = family.definitions.get(entry.kind).desiredSchema;
    writeJson(root, absoluteDesiredPath, desiredSchema);
    runnerInput[slug(entry.kind)] = {
      name: `${slug(entry.kind)}-probe`,
      identity: {
        formRef: entry.formRef,
        packageDigest: entry.packageDigest,
      },
      lifecycleCapabilities: ["create", "read", "delete"],
      desired: {},
      desiredSchema: {
        path: desiredSchemaPath,
        sha256: `sha256:${sha256File(root, absoluteDesiredPath)}`,
      },
    };
  }
  const requiredChecks = family.candidateSet.forms
    .map((entry) => `${family.group}.${entry.kind}.lifecycle`)
    .sort();
  const probeByKind = new Map(
    family.candidateSet.forms.map((entry) => [entry.kind, slug(entry.kind)]),
  );
  const corpus = {
    format: "takoform.family-semantic-corpus@v1",
    hostApiLane: HOST_API_LANE,
    group: family.group,
    requiredChecks,
    runnerInput,
    scenarios: requiredChecks.map((check) => {
      const kind = check.slice(`${family.group}.`.length, -".lifecycle".length);
      return {
        check,
        input: {
          probeName: probeByKind.get(kind),
          operations: ["create", "read", "delete"],
        },
        expected: {
          created: true,
          readbackIdentityExact: true,
          deleted: true,
        },
      };
    }),
    standardServiceFixtures: family.group === EDGE_FAMILY_GROUP
      ? [
          {
            serviceRef: {
              apiVersion: STANDARD_SERVICE_API,
              protocol: options.omitS3Fixture
                ? "dev.example.unknown"
                : S3_STANDARD_SERVICE_PROTOCOL,
            },
            satisfiable: true,
          },
          {
            serviceRef: {
              apiVersion: STANDARD_SERVICE_API,
              protocol: "dev.example.unknown",
            },
            satisfiable: false,
          },
        ]
      : [],
  };
  if (options.s3SubstringOnly && family.group === EDGE_FAMILY_GROUP) {
    corpus.description = `Unrelated label: ${S3_STANDARD_SERVICE_PROTOCOL}`;
  }
  writeJson(root, corpusPath, corpus);
  return {
    group: family.group,
    path: corpusPath,
    sha256: sha256File(root, corpusPath),
    requiredChecks,
    dependencyGroups: family.group === EDGE_FAMILY_GROUP ? [] : [EDGE_FAMILY_GROUP],
  };
}

function writeManifestOwnedRunner(root, { genericPass = false } = {}) {
  const runnerPath = "scripts/synthetic-takoform-v1-suite-runner.mjs";
  const body = genericPass
    ? `process.stdout.write(JSON.stringify({ result: "pass" }) + "\\n");\n`
    : `import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";

const manifestPath = process.argv[process.argv.indexOf("--manifest") + 1];
if (!manifestPath) throw new Error("missing manifest");
const manifestBytes = readFileSync(manifestPath);
const manifest = JSON.parse(manifestBytes);
const index = JSON.parse(readFileSync("${FAMILY_INDEX_PATH}"));
const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");
const refKey = (ref) => ref.apiVersion + "/" + ref.kind + "@" + ref.definitionVersion + "#" + ref.schemaDigest;
const passedChecks = (checks) => checks.map((name) => ({ name, status: "passed" }));
if (manifest.hostApiLane !== "${HOST_API_LANE}") throw new Error("wrong Host lane");
if (JSON.stringify(manifest.families.map((entry) => entry.group)) !== JSON.stringify(index.families.map((entry) => entry.group))) throw new Error("family omission");
const genericCorpus = JSON.parse(readFileSync(manifest.generic.path));
if (JSON.stringify(genericCorpus.requiredChecks) !== JSON.stringify(manifest.generic.requiredChecks)) throw new Error("generic checks drifted");
for (const scenario of genericCorpus.scenarios) {
  const observed = scenario.input.operation === "discover"
    ? { hostApiLane: manifest.hostApiLane }
    : scenario.input.operation === "read-missing"
      ? { code: "not_found" }
      : null;
  if (JSON.stringify(observed) !== JSON.stringify(scenario.expected)) throw new Error("generic semantic observation failed");
}
const families = manifest.families.map((suiteFamily) => {
  const indexFamily = index.families.find((entry) => entry.group === suiteFamily.group);
  const candidates = JSON.parse(readFileSync(indexFamily.candidateSet));
  const corpus = JSON.parse(readFileSync(suiteFamily.path));
  if (JSON.stringify(corpus.requiredChecks) !== JSON.stringify(suiteFamily.requiredChecks)) throw new Error("family checks drifted");
  const candidateRefs = candidates.forms.map((entry) => entry.formRef).sort((a, b) => refKey(a).localeCompare(refKey(b)));
  const runnerRefs = Object.values(corpus.runnerInput).map((entry) => entry.identity.formRef).sort((a, b) => refKey(a).localeCompare(refKey(b)));
  if (JSON.stringify(candidateRefs) !== JSON.stringify(runnerRefs)) throw new Error("family lifecycle coverage drifted");
  for (const scenario of corpus.scenarios) {
    const probe = corpus.runnerInput[scenario.input.probeName];
    if (!probe || JSON.stringify(scenario.input.operations) !== JSON.stringify(["create", "read", "delete"])) throw new Error("family lifecycle scenario malformed");
    const state = new Map();
    const key = refKey(probe.identity.formRef);
    state.set(key, probe.identity);
    const observed = {
      created: state.has(key),
      readbackIdentityExact: JSON.stringify(state.get(key)) === JSON.stringify(probe.identity),
      deleted: state.delete(key) && !state.has(key),
    };
    if (JSON.stringify(observed) !== JSON.stringify(scenario.expected)) throw new Error("family lifecycle semantic observation failed");
  }
  return { group: suiteFamily.group, path: suiteFamily.path, sha256: suiteFamily.sha256, checks: passedChecks(suiteFamily.requiredChecks), runnerFormRefs: runnerRefs };
});
const compositionCorpus = JSON.parse(readFileSync(manifest.composition.path));
if (JSON.stringify(compositionCorpus.requiredChecks) !== JSON.stringify(manifest.composition.requiredChecks)) throw new Error("composition checks drifted");
for (const scenario of compositionCorpus.scenarios) {
  let observed;
  if (scenario.input.operation === "resolve-all") {
    observed = { resolvedFamilyCount: index.families.length };
  } else if (scenario.input.operation === "reject-wrong-digest") {
    const firstSet = JSON.parse(readFileSync(index.families[0].candidateSet));
    const wrong = { ...firstSet.forms[0].formRef, schemaDigest: "sha256:" + "0".repeat(64) };
    const exactRefs = new Set(index.families.flatMap((entry) => JSON.parse(readFileSync(entry.candidateSet)).forms.map((form) => refKey(form.formRef))));
    observed = { rejected: !exactRefs.has(refKey(wrong)) };
  } else {
    observed = null;
  }
  if (JSON.stringify(observed) !== JSON.stringify(scenario.expected)) throw new Error("composition semantic observation failed");
}
const report = {
  format: manifest.runner.reportFormat,
  status: "passed",
  hostApiLane: manifest.hostApiLane,
  suite: { path: manifestPath, sha256: digest(manifestBytes) },
  generic: { path: manifest.generic.path, sha256: manifest.generic.sha256, checks: passedChecks(manifest.generic.requiredChecks) },
  families,
  composition: { path: manifest.composition.path, sha256: manifest.composition.sha256, checks: passedChecks(manifest.composition.requiredChecks) },
};
process.stdout.write(JSON.stringify(report) + "\\n");
`;
  writeText(root, runnerPath, body);
  return runnerPath;
}

function buildSourceFixture(options = {}) {
  const root = makeClone();
  // The future fixture must be self-verifying: do not let an out-of-tree copy
  // of the verifier be the only reason its evidence can close.
  writeText(
    root,
    "scripts/publication-evidence.mjs",
    readFileSync(path.join(ROOT, "scripts/publication-evidence.mjs"), "utf8"),
  );
  const edgeKinds = [
    "ModuleWorker",
    "WorkerBundle",
    "StaticAssetBundle",
    "WorkerVersion",
    "WorkerDeployment",
    "WorkerCustomDomain",
    "WorkerEndpoint",
    "WorkerCronTrigger",
    "EdgeKVNamespace",
    "SQLiteDatabase",
    "SQLiteMigrationSet",
    "SQLiteMigrationApplication",
    "AtLeastOnceQueue",
    "QueueConsumer",
    "DurableWorkflow",
    options.includeObjectBucket ? "ObjectBucket" : "ActorNamespace",
  ];
  expect(edgeKinds).toHaveLength(EDGE_CANDIDATE_SIZE);
  const families = [
    buildFamily(root, EDGE_FAMILY_GROUP, edgeKinds, options),
    buildFamily(root, "queue.forms.takoform.com", ["PullQueue"], options),
  ].sort((a, b) => a.group.localeCompare(b.group));

  const interfaces = buildAggregateCandidateSet(
    root,
    "interface",
    [{ name: options.includeEdgeObjects ? "edge.objects" : "worker.runtime" }],
  );
  const bindings = buildAggregateCandidateSet(
    root,
    "binding",
    [{ name: "module-worker.service" }],
  );
  const familyIndex = {
    format: FAMILY_INDEX_FORMAT,
    families: families.map((family) => ({
      group: family.group,
      candidateSet: family.candidateSetPath,
      sha256: sha256File(root, family.candidateSetPath),
      formCount:
        family.group === EDGE_FAMILY_GROUP && options.wrongEdgeCount
          ? EDGE_CANDIDATE_SIZE - 1
          : family.candidateSet.forms.length,
    })),
    interfaceCandidateSet: interfaces,
    bindingCandidateSet: bindings,
  };
  writeJson(root, FAMILY_INDEX_PATH, familyIndex);

  const genericPath = "conformance/takoform-v1/generic/corpus.json";
  const generic = {
    format: "takoform.generic-host-corpus@v1",
    hostApiLane: HOST_API_LANE,
    requiredChecks: ["generic.discovery", "generic.error-envelope"],
    scenarios: [
      {
        check: "generic.discovery",
        input: { operation: "discover" },
        expected: { hostApiLane: HOST_API_LANE },
      },
      {
        check: "generic.error-envelope",
        input: { operation: "read-missing" },
        expected: { code: "not_found" },
      },
    ],
  };
  writeJson(root, genericPath, generic);
  const compositionPath = "conformance/takoform-v1/composition/corpus.json";
  const composition = {
    format: options.relabelComposition
      ? "takoform.generic-host-corpus@v1"
      : "takoform.all-family-composition-corpus@v1",
    hostApiLane: HOST_API_LANE,
    requiredChecks: ["composition.all-families", "composition.exact-target-resolution"],
    familyGroups: families.map((entry) => entry.group),
    scenarios: [
      {
        check: "composition.all-families",
        input: {
          familyGroups: families.map((entry) => entry.group),
          operation: "resolve-all",
        },
        expected: { resolvedFamilyCount: families.length },
      },
      {
        check: "composition.exact-target-resolution",
        input: {
          familyGroups: families.map((entry) => entry.group),
          operation: "reject-wrong-digest",
        },
        expected: { rejected: true },
      },
    ],
  };
  if (options.labelOnlyComposition) composition.scenarios = [];
  writeJson(root, compositionPath, composition);
  let suiteFamilies = families.map((family) => buildFamilyCorpus(root, family, options));
  if (options.omitQueueCorpus) {
    suiteFamilies = suiteFamilies.filter((entry) => entry.group !== "queue.forms.takoform.com");
  }
  const runnerPath = writeManifestOwnedRunner(root, options);
  const reportFormat = "takoform.synthetic-reference-suite-report@v1";
  const suite = {
    format: CONFORMANCE_SUITE_FORMAT,
    hostApiLane: HOST_API_LANE,
    generic: {
      path: genericPath,
      sha256: sha256File(root, genericPath),
      requiredChecks: generic.requiredChecks,
    },
    families: suiteFamilies,
    composition: {
      path: compositionPath,
      sha256: sha256File(root, compositionPath),
      requiredChecks: composition.requiredChecks,
    },
    runner: {
      command: options.missingRunner
        ? ["bun", "scripts/missing-suite-runner.mjs", "--manifest", CONFORMANCE_SUITE_PATH]
        : ["bun", runnerPath, "--manifest", CONFORMANCE_SUITE_PATH],
      reportFormat,
    },
  };
  writeJson(root, CONFORMANCE_SUITE_PATH, suite);
  const openDocument = clone(DOCUMENT);
  openDocument.evidence.specification.sourceSnapshot = null;
  writeJson(root, "spec/publication-evidence.json", openDocument);
  git(
    root,
    "add",
    "forms/candidates",
    "interfaces/candidates/v1",
    "bindings/candidates/v1",
    "conformance/takoform-v1",
    "spec/publication-evidence.json",
    "scripts/publication-evidence.mjs",
    runnerPath,
  );
  git(root, "commit", "-m", "test: synthetic multi-family Specification v1 tuple");
  const sourceCommit = git(root, "rev-parse", "HEAD");
  git(root, "update-ref", "refs/remotes/origin/main", sourceCommit);
  const baseline = {
    repository: "takoform",
    commit: sourceCommit,
    familyIndex: { path: FAMILY_INDEX_PATH, sha256: sha256File(root, FAMILY_INDEX_PATH) },
    conformanceSuite: {
      path: CONFORMANCE_SUITE_PATH,
      sha256: sha256File(root, CONFORMANCE_SUITE_PATH),
    },
  };
  return { root, baseline, sourceCommit };
}

function buildSpecificationFuture() {
  const fixture = buildSourceFixture();
  const { root, sourceCommit } = fixture;
  const document = clone(DOCUMENT);
  document.candidateBaseline = fixture.baseline;
  const candidateEvidence = deriveCandidateCorpusEvidence(root, document.candidateBaseline);
  document.evidence.specification.sourceSnapshot =
    deriveSpecificationSourceSnapshot(root, sourceCommit);
  document.evidence.specification.candidateCorpus = candidateEvidence;
  document.evidence.specification.referenceConformance =
    deriveReferenceConformanceEvidence(root, candidateEvidence);
  writeJson(root, "spec/publication-evidence.json", document);
  git(root, "add", "spec/publication-evidence.json");
  git(root, "commit", "-m", "test: record exact Specification v1 evidence");
  git(root, "update-ref", "refs/remotes/origin/main", "HEAD");
  return { ...fixture, document, candidateEvidence };
}

let future;

beforeAll(() => {
  future = buildSpecificationFuture();
}, 30_000);

afterAll(() => {
  for (const root of rootsToRemove) rmSync(root, { recursive: true, force: true });
}, 30_000);

describe("current fail-closed state", () => {
  test("keeps the absent canonical multi-family tuple and reports the exact release stage", () => {
    const report = validatePublicationEvidence(DOCUMENT, { repositoryRoot: ROOT });
    expect(report.candidate.available).toBe(false);
    expect(report.candidate.familyCount).toBe(0);
    expect(report.candidate.edgeRosterExact).toBe(false);
    const specificationStatus =
      DOCUMENT.evidence.specification.sourceSnapshot === null
        ? "open"
        : "ready";
    expect(report.tracks.map((entry) => [entry.id, entry.status])).toEqual([
      [SPECIFICATION_TRACK, specificationStatus],
      [PROVIDER_TRACK, "open"],
    ]);
  });

  test("keeps compatibility reporting outside the publication-evidence shape", () => {
    expect(Object.keys(DOCUMENT.evidence.specification)).toEqual([
      "sourceSnapshot",
      "candidateCorpus",
      "referenceConformance",
    ]);
    expect(DOCUMENT.evidence.specification.candidateCorpus).toBeNull();
    expect(DOCUMENT.evidence.specification.referenceConformance).toBeNull();
    const sourceSnapshot = DOCUMENT.evidence.specification.sourceSnapshot;
    if (sourceSnapshot !== null) {
      expect(sourceSnapshot).toMatchObject({
        format: SPECIFICATION_SOURCE_FORMAT,
        releaseVersion: "1.1",
        repository: "takoform",
        roots: ["spec"],
        excludedPaths: [
          "spec/publication-evidence.json",
          "spec/publication-blockers.json",
        ],
      });
    }
    expect(JSON.stringify(DOCUMENT)).not.toContain("compatibilityManifest");
  });

  test("has no trusted status, signer policy, or writer mode", () => {
    expect(JSON.stringify(DOCUMENT)).not.toContain('"status"');
    expect(JSON.stringify(DOCUMENT)).not.toContain("trustedSigner");
    const result = spawnSync("bun", ["scripts/publication-evidence.mjs", "--write"], {
      cwd: ROOT,
      encoding: "utf8",
    });
    expect(result.status).not.toBe(0);
    expect(`${result.stdout}${result.stderr}`).toContain("usage:");
  });

  test("the Specification assertion depends only on its normative source prerequisite", () => {
    const result = spawnSync("bun", ["scripts/publication-evidence.mjs", "--assert-specification-v1"], {
      cwd: ROOT,
      encoding: "utf8",
    });
    const output = `${result.stdout}${result.stderr}`;
    if (DOCUMENT.evidence.specification.sourceSnapshot === null) {
      expect(result.status).not.toBe(0);
      expect(output).toContain(
        "specification-v1:specification-source-snapshot",
      );
    } else {
      expect(result.status).toBe(0);
      expect(output).toContain("Specification 1.1 ready");
    }
    expect(output).not.toContain("candidate-form-corpus");
    expect(output).not.toContain("reference-conformance");
    expect(output).not.toContain("provider-3.0:");
    expect(output).not.toContain("takoserver");
  });

  test("rejects a partial or caller-renamed canonical tuple", () => {
    const partial = clone(DOCUMENT);
    partial.candidateBaseline.familyIndex = {
      path: FAMILY_INDEX_PATH,
      sha256: "0".repeat(64),
    };
    expect(() => validatePublicationEvidence(partial, { repositoryRoot: ROOT })).toThrow(
      /null together or present together/,
    );
    const renamed = clone(future.document);
    renamed.candidateBaseline.familyIndex.path = "forms/candidates/caller-index.json";
    expect(() => validatePublicationEvidence(renamed, { repositoryRoot: future.root })).toThrow(
      /path must be forms\/candidates\/current-family-index\.json/,
    );
  });
});

describe("canonical repository authority", () => {
  test("forbids caller mappings and result injection", () => {
    expect(() =>
      validatePublicationEvidence(DOCUMENT, {
        repositoryRoot: ROOT,
        repositoryRoots: { takoform: ROOT },
      }),
    ).toThrow(/forbidden/);
    expect(() =>
      validatePublicationEvidence(DOCUMENT, {
        repositoryRoot: ROOT,
        referenceReport: { result: "pass" },
      }),
    ).toThrow(/forbidden/);
  });

  test("rejects a complete temp repository with no canonical origin", () => {
    const root = makeClone();
    git(root, "remote", "remove", "origin");
    expect(() => validatePublicationEvidence(DOCUMENT, { repositoryRoot: root })).toThrow(
      /no origin remote/,
    );
  });

  test("rejects a spoofed origin", () => {
    const root = makeClone();
    git(root, "remote", "set-url", "origin", "https://example.invalid/takoform.git");
    expect(() => validatePublicationEvidence(DOCUMENT, { repositoryRoot: root })).toThrow(
      /not the canonical Takoform repository/,
    );
  });

  test("rejects a present commit unreachable from allowed canonical refs", () => {
    const root = makeClone();
    const tree = git(root, "rev-parse", "HEAD^{tree}");
    const commit = execFileSync("git", ["-C", root, "commit-tree", tree], {
      input: "unreachable fixture\n",
      encoding: "utf8",
      env: {
        ...process.env,
        GIT_AUTHOR_NAME: "Fixture",
        GIT_AUTHOR_EMAIL: "fixture@example.invalid",
        GIT_COMMITTER_NAME: "Fixture",
        GIT_COMMITTER_EMAIL: "fixture@example.invalid",
      },
    }).trim();
    const changed = clone(DOCUMENT);
    changed.candidateBaseline.commit = commit;
    expect(() => validatePublicationEvidence(changed, { repositoryRoot: root })).toThrow(
      /not reachable from an allowed canonical ref/,
    );
  });

  test("derives committed source bytes from the unreplaced object view despite ambient Git overrides", () => {
    const root = makeClone();
    const sourceCommit = DOCUMENT.candidateBaseline.commit;
    const expected = deriveSpecificationSourceSnapshot(root, sourceCommit);
    writeText(root, "spec/ambient-replacement.md", "attacker replacement bytes\n");
    git(root, "add", "spec/ambient-replacement.md");
    git(root, "commit", "-m", "attacker replacement commit");
    const replacementCommit = git(root, "rev-parse", "HEAD");
    git(root, "replace", sourceCommit, replacementCommit);

    const names = [
      "GIT_ALTERNATE_OBJECT_DIRECTORIES",
      "GIT_CONFIG_COUNT",
      "GIT_CONFIG_GLOBAL",
      "GIT_CONFIG_KEY_0",
      "GIT_CONFIG_VALUE_0",
      "GIT_INDEX_FILE",
      "GIT_OBJECT_DIRECTORY",
      "GIT_REPLACE_REF_BASE",
      "GIT_WORK_TREE",
    ];
    const previous = Object.fromEntries(
      names.map((name) => [name, process.env[name]]),
    );
    const globalConfig = path.join(root, "attacker.gitconfig");
    writeFileSync(globalConfig, "[core]\n\tuseReplaceRefs = true\n");
    process.env.GIT_ALTERNATE_OBJECT_DIRECTORIES = "/tmp/attacker-alternates";
    process.env.GIT_CONFIG_COUNT = "1";
    process.env.GIT_CONFIG_GLOBAL = globalConfig;
    process.env.GIT_CONFIG_KEY_0 = "core.useReplaceRefs";
    process.env.GIT_CONFIG_VALUE_0 = "true";
    process.env.GIT_INDEX_FILE = "/tmp/attacker-index";
    process.env.GIT_OBJECT_DIRECTORY = "/tmp/attacker-objects";
    process.env.GIT_REPLACE_REF_BASE = "refs/attacker/";
    process.env.GIT_WORK_TREE = "/tmp/attacker-worktree";
    try {
      expect(deriveSpecificationSourceSnapshot(root, sourceCommit)).toEqual(
        expected,
      );
    } finally {
      for (const name of names) {
        if (previous[name] === undefined) delete process.env[name];
        else process.env[name] = previous[name];
      }
      git(root, "replace", "-d", sourceCommit);
    }
  });

  test("rejects repository URL rewrites and alternate object authority", () => {
    const rewritten = makeClone();
    git(
      rewritten,
      "config",
      "url.file:///tmp/attacker/.insteadOf",
      "https://github.com/",
    );
    expect(() =>
      validatePublicationEvidence(DOCUMENT, { repositoryRoot: rewritten }),
    ).toThrow(/configuration can influence evidence/);

    const alternate = makeClone();
    const commonDirectory = git(
      alternate,
      "rev-parse",
      "--path-format=absolute",
      "--git-common-dir",
    );
    writeText(
      commonDirectory,
      "objects/info/alternates",
      `${path.join(ROOT, ".git", "objects")}\n`,
    );
    expect(() => deriveSpecificationSourceSnapshot(
      alternate,
      DOCUMENT.candidateBaseline.commit,
    )).toThrow(/forbidden objects\/info\/alternates/);
  });

  test("rejects a shallow repository before making history or reachability claims", () => {
    const root = mkdtempSync(
      path.join(tmpdir(), "takoform-publication-evidence-shallow-"),
    );
    rootsToRemove.add(root);
    execFileSync(
      "git",
      ["clone", "--depth", "1", `file://${ROOT}`, root],
      { stdio: "ignore" },
    );
    git(root, "remote", "set-url", "origin", CANONICAL_ORIGIN);
    git(
      root,
      "config",
      "remote.origin.fetch",
      "+refs/heads/*:refs/remotes/origin/*",
    );
    expect(() =>
      validatePublicationEvidence(DOCUMENT, { repositoryRoot: root }),
    ).toThrow(/complete and non-shallow/);
  });
});

describe("class-specific anti-false-claim validation", () => {
  test("generic pass JSON cannot masquerade as any Specification class", () => {
    const source = clone(future.document);
    source.evidence.specification.sourceSnapshot = {
      format: SPECIFICATION_SOURCE_FORMAT,
      result: "pass",
    };
    expect(() => validatePublicationEvidence(source, { repositoryRoot: future.root })).toThrow(
      /sourceSnapshot must identify one exact Specification source commit/,
    );

    const candidate = clone(future.document);
    candidate.evidence.specification.candidateCorpus = {
      format: CANDIDATE_CORPUS_FORMAT,
      familyCount: 2,
      candidateCount: 17,
      runnerCount: 17,
      missing: [],
      result: "pass",
    };
    expect(() => validatePublicationEvidence(candidate, { repositoryRoot: future.root })).toThrow(
      /candidateCorpus keys must be exactly/,
    );

    const reference = clone(future.document);
    reference.evidence.specification.referenceConformance = {
      format: REFERENCE_CONFORMANCE_FORMAT,
      result: "pass",
    };
    expect(() => validatePublicationEvidence(reference, { repositoryRoot: future.root })).toThrow(
      /referenceConformance keys must be exactly/,
    );
  });

  test("hand counts and omitted family/corpus entries cannot close candidate coverage", () => {
    const changedCount = clone(future.document);
    changedCount.evidence.specification.candidateCorpus.families[0].formCount = 999;
    expect(() => validatePublicationEvidence(changedCount, { repositoryRoot: future.root })).toThrow(
      /does not equal the reopened multi-family artifact closure/,
    );
    const omitted = clone(future.document);
    omitted.evidence.specification.candidateCorpus.families.pop();
    expect(() => validatePublicationEvidence(omitted, { repositoryRoot: future.root })).toThrow(
      /does not equal the reopened multi-family artifact closure/,
    );
  });

  test("the reopened index rejects a false family formCount", () => {
    const fixture = buildSourceFixture({ wrongEdgeCount: true });
    expect(() => deriveCandidateCorpusEvidence(fixture.root, fixture.baseline)).toThrow(
      /formCount does not equal reopened candidate Forms/,
    );
  });

  test("suite omission, missing runner source, and generic runner output fail closed", () => {
    const omitted = buildSourceFixture({ omitQueueCorpus: true });
    expect(() => deriveCandidateCorpusEvidence(omitted.root, omitted.baseline)).toThrow(
      /conformance suite family groups must be exactly/,
    );
    const missing = buildSourceFixture({ missingRunner: true });
    expect(() => deriveCandidateCorpusEvidence(missing.root, missing.baseline)).toThrow(
      /runner source is not committed/,
    );
    const generic = buildSourceFixture({ genericPass: true });
    const candidate = deriveCandidateCorpusEvidence(generic.root, generic.baseline);
    expect(() => deriveReferenceConformanceEvidence(generic.root, candidate)).toThrow(
      /report keys must be exactly/,
    );
    const relabeled = buildSourceFixture({ relabelComposition: true });
    expect(() => deriveCandidateCorpusEvidence(relabeled.root, relabeled.baseline)).toThrow(
      /composition\.format must be takoform\.all-family-composition-corpus@v1/,
    );
    const labelsOnly = buildSourceFixture({ labelOnlyComposition: true });
    expect(() => deriveCandidateCorpusEvidence(labelsOnly.root, labelsOnly.baseline)).toThrow(
      /composition\.scenarios must contain executable class-specific cases/,
    );
  });

  test("ObjectBucket, edge.objects, and a new central protocol enum are rejected", () => {
    const objectBucket = buildSourceFixture({ includeObjectBucket: true });
    expect(() => deriveCandidateCorpusEvidence(objectBucket.root, objectBucket.baseline)).toThrow(
      /removed ObjectBucket\/edge\.objects identity|must not contain an ObjectBucket/,
    );
    const edgeObjects = buildSourceFixture({ includeEdgeObjects: true });
    expect(() => deriveCandidateCorpusEvidence(edgeObjects.root, edgeObjects.baseline)).toThrow(
      /removed ObjectBucket\/edge\.objects identity/,
    );
    const enumFixture = buildSourceFixture({ closedProtocolEnum: true });
    expect(() => deriveCandidateCorpusEvidence(enumFixture.root, enumFixture.baseline)).toThrow(
      /must not create a central Takoform protocol enum/,
    );
    const allOfFixture = buildSourceFixture({ protocolAllOf: true });
    expect(() => deriveCandidateCorpusEvidence(allOfFixture.root, allOfFixture.baseline)).toThrow(
      /must not add narrowing keyword allOf/,
    );
  });

  test("the exact opaque S3 slot corpus is required without defining S3 semantics", () => {
    const noS3 = buildSourceFixture({ omitS3Fixture: true, s3SubstringOnly: true });
    expect(() => deriveCandidateCorpusEvidence(noS3.root, noS3.baseline)).toThrow(
      /must contain exact standards\.takoform\.com\/v1\/com\.amazonaws\.s3 satisfiable readback/,
    );
    const protocol = future.candidateEvidence.families
      .find((entry) => entry.group === EDGE_FAMILY_GROUP);
    expect(protocol).toBeDefined();
    expect(JSON.stringify(future.candidateEvidence)).not.toContain("edge.objects");
    expect(JSON.stringify(future.candidateEvidence)).not.toContain("ObjectBucket");
  });

  test("external products and signer policy cannot be added as Takoform authority", () => {
    for (const key of ["takoserver", "productionEvidence", "trustedSigners", "operatorSignatures"]) {
      const changed = clone(DOCUMENT);
      changed[key] = [];
      expect(() => validatePublicationEvidence(changed, { repositoryRoot: ROOT })).toThrow(
        /publication evidence document keys must be exactly/,
      );
    }
  });

  test("Provider lifecycle and compatibility parsers require exact Provider observations", () => {
    expect(() => parseProviderMatrixObservations('{"result":"pass"}')).toThrow(
      /exact observation report/,
    );
    const matrix =
      "verified non-publishable worker authoring evidence: 2 CLIs, 19 validated configurations, same-name replacement refused at plan, roll-forward serves throughout (124 Ready samples, 0 not ready), two owners of identical output hold 4 distinct revisions, heterogeneous vars keep their JSON types, destroy removes the 5-resource aggregate in dependency order and leaves 0 behind";
    expect(parseProviderMatrixObservations(matrix).resourcesRemainingAfterDestroy).toBe(0);
    const partialCompatibility = JSON.stringify({
      Action: "pass",
      Test: PROVIDER_COMPATIBILITY_TESTS[0],
    });
    expect(() => parseProviderCompatibilityOutput(partialCompatibility)).toThrow(
      /passed tests must be exactly/,
    );
    const candidateIdentities = future.candidateEvidence.families.flatMap(
      (family) => family.candidateIdentities,
    );
    const projection = candidateIdentities.map((identity, index) => ({
      resourceType: `takoform_fixture_${index}`,
      formRef: identity.formRef,
      packageDigest: identity.packageDigest,
    }));
    projection[projection.length - 1] = {
      ...projection[0],
      resourceType: "takoform_distinct_type_same_formref",
    };
    expect(() => validateProviderIdentityProjection(projection, candidateIdentities)).toThrow(
      /repeats FormRef/,
    );
  });

  test("staged and untracked runner source cannot execute as committed evidence", () => {
    const head = git(future.root, "rev-parse", "HEAD");
    const staged = makeCanonicalClone(future.root, head, "takoform-publication-staged-runner-");
    writeText(
      staged,
      "scripts/synthetic-takoform-v1-suite-runner.mjs",
      `${readFileSync(path.join(staged, "scripts/synthetic-takoform-v1-suite-runner.mjs"), "utf8")}\n// staged attack\n`,
    );
    git(staged, "add", "scripts/synthetic-takoform-v1-suite-runner.mjs");
    expect(() => validatePublicationEvidence(future.document, { repositoryRoot: staged })).toThrow(
      /implementation has uncommitted changes/,
    );

    const untracked = makeCanonicalClone(future.root, head, "takoform-publication-untracked-runner-");
    writeJson(untracked, "reference-data/result.json", { result: "forged-pass" });
    expect(() => validatePublicationEvidence(future.document, { repositoryRoot: untracked })).toThrow(
      /implementation has uncommitted changes/,
    );
  });
});

describe("independent release tracks", () => {
  test("the create-only writer derives only the normative Specification source record", () => {
    const fixture = buildSourceFixture();
    const prepared = prepareSpecificationEvidence(fixture.root);
    expect(prepared.evidence.specification.sourceSnapshot.sourceCommit).toBe(
      fixture.sourceCommit,
    );
    expect(prepared.evidence.specification.sourceSnapshot).not.toBeNull();
    expect(prepared.evidence.specification.candidateCorpus).toBeNull();
    expect(prepared.evidence.specification.referenceConformance).toBeNull();
    expect(loadPublicationEvidence(fixture.root)).toEqual(prepared);
    const raw = readFileSync(
      path.join(fixture.root, "spec/publication-evidence.json"),
    );
    for (const relativePath of PUBLICATION_EVIDENCE_PROJECTION_PATHS) {
      expect(readFileSync(path.join(fixture.root, relativePath))).toEqual(raw);
    }
    git(
      fixture.root,
      "add",
      "spec/publication-evidence.json",
      ...PUBLICATION_EVIDENCE_PROJECTION_PATHS,
    );
    git(fixture.root, "commit", "-m", "test: record source-only C2 evidence");
    git(fixture.root, "update-ref", "refs/remotes/origin/main", "HEAD");
    const report = validatePublicationEvidence(prepared, {
      repositoryRoot: fixture.root,
      releaseTrack: SPECIFICATION_TRACK,
    });
    expect(
      report.tracks.find((entry) => entry.id === SPECIFICATION_TRACK)?.status,
    ).toBe("ready");
    expect(
      report.tracks.find((entry) => entry.id === PROVIDER_TRACK)?.status,
    ).toBe("open");
    expect(() =>
      assertPublicationEvidenceReady(report, SPECIFICATION_TRACK),
    ).not.toThrow();
    expect(() => prepareSpecificationEvidence(fixture.root)).toThrow(
      "Specification evidence is already closed; preparation is create-only",
    );
  }, 60_000);

  test("a fully valid multi-family Specification future passes without Provider or external evidence", () => {
    const report = validatePublicationEvidence(future.document, { repositoryRoot: future.root });
    expect(report.candidate.available).toBe(true);
    expect(report.candidate.familyCount).toBe(2);
    expect(report.candidate.edgeCandidateForms).toBe(EDGE_CANDIDATE_SIZE);
    expect(report.candidate.edgeRunnerForms).toBe(EDGE_CANDIDATE_SIZE);
    expect(report.candidate.edgeRosterExact).toBe(true);
    expect(report.tracks.find((entry) => entry.id === SPECIFICATION_TRACK)?.status).toBe("ready");
    expect(report.tracks.find((entry) => entry.id === PROVIDER_TRACK)?.status).toBe("open");
    expect(() => assertPublicationEvidenceReady(report, SPECIFICATION_TRACK)).not.toThrow();
    expect(() => assertPublicationEvidenceReady(report, PROVIDER_TRACK)).toThrow(/provider-3\.0:/);
    const ownVerifier = spawnSync("bun", ["scripts/publication-evidence.mjs", "--check"], {
      cwd: future.root,
      encoding: "utf8",
    });
    expect(ownVerifier.status).toBe(0);
    expect(ownVerifier.stdout).toContain("Specification 1.1");
  });

  test("only reopening the normative source snapshot blocks Specification readiness", () => {
    const root = makeCanonicalClone(
      future.root,
      git(future.root, "rev-parse", "HEAD"),
      "takoform-publication-reopened-",
    );
    const changed = clone(future.document);
    changed.evidence.specification.sourceSnapshot = null;
    changed.evidence.specification.candidateCorpus = null;
    changed.evidence.specification.referenceConformance = null;
    writeJson(root, "spec/publication-evidence.json", changed);
    git(root, "add", "spec/publication-evidence.json");
    git(root, "commit", "-m", "test: reopen normative source snapshot");
    git(root, "update-ref", "refs/remotes/origin/main", "HEAD");
    const report = validatePublicationEvidence(changed, { repositoryRoot: root });
    expect(() => assertPublicationEvidenceReady(report, SPECIFICATION_TRACK)).toThrow(
      /specification-v1:/,
    );
  }, 60_000);

  test("candidate corpus and Reference Host evidence do not block the Specification", () => {
    const root = makeCanonicalClone(
      future.root,
      git(future.root, "rev-parse", "HEAD"),
      "takoform-publication-source-only-",
    );
    const changed = clone(future.document);
    changed.evidence.specification.candidateCorpus = null;
    changed.evidence.specification.referenceConformance = null;
    writeJson(root, "spec/publication-evidence.json", changed);
    git(root, "add", "spec/publication-evidence.json");
    git(root, "commit", "-m", "test: keep reference evidence optional");
    const report = validatePublicationEvidence(changed, {
      repositoryRoot: root,
      releaseTrack: SPECIFICATION_TRACK,
    });
    expect(() => assertPublicationEvidenceReady(report, SPECIFICATION_TRACK)).not.toThrow();
  }, 60_000);

  test("rejects committed or uncommitted normative changes after source evidence preparation", () => {
    const committed = makeCanonicalClone(
      future.root,
      git(future.root, "rev-parse", "HEAD"),
      "takoform-publication-source-drift-",
    );
    writeText(
      committed,
      "spec/versioning.md",
      `${readFileSync(path.join(committed, "spec/versioning.md"), "utf8")}\nCommitted drift.\n`,
    );
    git(committed, "add", "spec/versioning.md");
    git(committed, "commit", "-m", "test: mutate normative source after evidence");
    git(committed, "update-ref", "refs/remotes/origin/main", "HEAD");
    expect(() => validatePublicationEvidence(future.document, {
      repositoryRoot: committed,
      releaseTrack: SPECIFICATION_TRACK,
    })).toThrow(/source at HEAD does not equal the recorded source snapshot/);

    const staged = makeCanonicalClone(
      future.root,
      git(future.root, "rev-parse", "HEAD"),
      "takoform-publication-source-staged-",
    );
    writeText(
      staged,
      "spec/versioning.md",
      `${readFileSync(path.join(staged, "spec/versioning.md"), "utf8")}\nStaged drift.\n`,
    );
    git(staged, "add", "spec/versioning.md");
    expect(() => validatePublicationEvidence(future.document, {
      repositoryRoot: staged,
      releaseTrack: SPECIFICATION_TRACK,
    })).toThrow(/normative Specification source has uncommitted changes/);
  }, 60_000);

  test("a malformed Provider milestone cannot block the Specification assertion", () => {
    const root = makeCanonicalClone(
      future.root,
      git(future.root, "rev-parse", "HEAD"),
      "takoform-publication-provider-independent-",
    );
    const changed = clone(future.document);
    changed.evidence.provider.exactConformance = {
      format: "generic.pass@v1",
      result: "pass",
    };
    writeJson(root, "spec/publication-evidence.json", changed);
    git(root, "add", "spec/publication-evidence.json");
    git(root, "commit", "-m", "test: malformed independent Provider milestone");
    git(root, "update-ref", "refs/remotes/origin/main", "HEAD");
    const specification = spawnSync(
      "bun",
      ["scripts/publication-evidence.mjs", "--assert-specification-v1"],
      { cwd: root, encoding: "utf8" },
    );
    expect(specification.status).toBe(0);
      expect(specification.stdout).toContain("Specification 1.1");
    const fullCheck = spawnSync("bun", ["scripts/publication-evidence.mjs", "--check"], {
      cwd: root,
      encoding: "utf8",
    });
    expect(fullCheck.status).not.toBe(0);
    expect(`${fullCheck.stdout}${fullCheck.stderr}`).toContain(
      "Provider evidence artifacts must name one common full Provider source commit",
    );
  });

  test("the immutable historical v1beta1 blocker ledger is unchanged", () => {
    expect(sha256File(future.root, "spec/publication-blockers.json")).toBe(
      "8bc708163e789b95833331a537abf1c455062179c0eef5b57c583c76b8d740e0",
    );
    const changed = clone(future.document);
    changed.retainedHistory.sha256 = "f".repeat(64);
    expect(() => validatePublicationEvidence(changed, { repositoryRoot: future.root })).toThrow(
      /history identity drifted/,
    );
  });
});

test("package scripts expose independent assertions without a false Stable mint alias", () => {
  const pkg = JSON.parse(readFileSync(path.join(ROOT, "package.json"), "utf8"));
  expect(pkg.scripts["check:specification-1-1-release"]).toContain("--assert-specification-1-1");
  expect(pkg.scripts["check:provider-v3-release"]).toContain("--assert-provider-3");
  expect(pkg.scripts["check:takoform-milestones"]).toContain("--assert-all");
  expect(pkg.scripts["check:stable-mint"]).toBeUndefined();
});

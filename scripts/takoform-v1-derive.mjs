#!/usr/bin/env bun

import { createHash } from "node:crypto";
import {
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/takoform-v1-derive.mjs --write|--check");
}

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const targetRoot = path.join(root, "conformance", "takoform-v1");
const betaRoot = path.join(root, "conformance", "portable-host-v1beta4");
const indexPath = "forms/candidates/current-family-index.json";
const HOST_LANE = "forms.takoform.com/v1";
const SERVICE_API = "standards.takoform.com/v1";
const S3 = "com.amazonaws.s3";
const UNKNOWN_SERVICE = "com.example.future-store";
const outputs = new Map();

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function canonical(value) {
  if (value === null || typeof value === "string" || typeof value === "boolean") {
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value) || (Number.isInteger(value) && !Number.isSafeInteger(value))) {
      throw new Error("canonical JSON contains a non-finite number or non-safe integer");
    }
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (typeof value !== "object") throw new Error("canonical JSON contains an unsupported value");
  return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${canonical(value[key])}`).join(",")}}`;
}

function canonicalDigest(document) {
  return `sha256:${sha256(Buffer.from(canonical(document)))}`;
}

function jsonBytes(document) {
  return Buffer.from(`${JSON.stringify(document, null, 2)}\n`);
}

function readJSON(relativePath) {
  return JSON.parse(readFileSync(path.join(root, relativePath), "utf8"));
}

function emit(relativePath, bytesOrDocument) {
  const bytes = Buffer.isBuffer(bytesOrDocument) ? bytesOrDocument : jsonBytes(bytesOrDocument);
  outputs.set(relativePath, bytes);
  return bytes;
}

function slug(value) {
  return value
    .replace(/([a-z0-9])([A-Z])/g, "$1-$2")
    .replace(/[^A-Za-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .toLowerCase();
}

function lowerCamel(value) {
  return value[0].toLowerCase() + value.slice(1);
}

function exactRefKey(ref) {
  return `${ref.apiVersion}/${ref.kind}@${ref.definitionVersion}#${ref.schemaDigest}`;
}

const currentIndex = readJSON(indexPath);
if (currentIndex.format !== "takoform.current-family-index@v1" || currentIndex.families.length !== 8) {
  throw new Error(`${indexPath} does not carry the canonical 8-family stable index`);
}

const familyInputs = currentIndex.families.map((entry) => {
  const bytes = readFileSync(path.join(root, entry.candidateSet));
  if (sha256(bytes) !== entry.sha256) throw new Error(`${entry.candidateSet} drifted from the current-family index`);
  const candidateSet = JSON.parse(bytes);
  if (candidateSet.family !== entry.group || candidateSet.forms.length !== entry.formCount) {
    throw new Error(`${entry.candidateSet} roster drifted from the current-family index`);
  }
  const forms = candidateSet.forms.map((candidate) => {
    const definitionPath = `${candidate.path}/definition.json`;
    const definitionBytes = readFileSync(path.join(root, definitionPath));
    const definition = JSON.parse(definitionBytes);
    if (
      definition.requiresHostApi !== HOST_LANE ||
      canonicalDigest(definition) !== candidate.formRef.schemaDigest
    ) {
      throw new Error(`${definitionPath} is not the exact stable Form named by its candidate`);
    }
    const desiredPath = `${candidate.path}/fixtures/desired.json`;
    return { candidate, definition, desired: readJSON(desiredPath) };
  });
  return { entry, candidateSet, forms };
});

const edge = familyInputs.find(({ entry }) => entry.group === "edge.forms.takoform.com");
if (edge === undefined || edge.forms.length !== 16 || edge.forms.some(({ candidate }) => candidate.kind === "ObjectBucket")) {
  throw new Error("stable Edge roster must be the exact 16-Form set without ObjectBucket");
}

// ---- full generic Host lifecycle corpus ----

const contract = structuredClone(readJSON("conformance/portable-host-v1beta4/contract.json"));
contract.format = "takoform.portable-host-conformance@v1";
contract.apiVersion = HOST_LANE;
contract.discoveryPath = "/.well-known/takoform/v1";
contract.apiPath = "/apis/forms.takoform.com/v1";
contract.familyCandidateSet = path.posix.dirname(edge.entry.candidateSet);
contract.bindingCandidateSet = path.posix.dirname(currentIndex.bindingCandidateSet.path);
contract.runnerInput.syntheticSecondGroup.apiVersion = "other.forms.takoform.com";

const familyDerivedChecks = new Set(["class-holder-rules-enforced"]);
const laneChecks = contract.requiredRunnerChecks.filter((check) => !familyDerivedChecks.has(check));
const derivedChecks = contract.requiredRunnerChecks.filter((check) => familyDerivedChecks.has(check));
contract.requiredRunnerChecks = [
  ...laneChecks.map((check) => check === "external-service-slots-sealed"
    ? "stable-standard-service-support-enforced"
    : check),
  "declared-constraint-semantics-enforced",
  ...derivedChecks,
];

const probeByKind = new Map();
for (const [key, value] of Object.entries(contract.runnerInput)) {
  const probe = value?.resourceProbe?.identity?.formRef !== undefined ? value.resourceProbe : value;
  if (probe?.identity?.formRef?.kind !== undefined) probeByKind.set(probe.identity.formRef.kind, probe);
}
for (const { candidate, definition } of edge.forms) {
  const probe = probeByKind.get(candidate.kind);
  if (probe === undefined) throw new Error(`stable generic corpus has no ${candidate.kind} lifecycle probe`);
  probe.identity.formRef = structuredClone(candidate.formRef);
  probe.identity.packageDigest = candidate.packageDigest;
  probe.lifecycleCapabilities = structuredClone(definition.lifecycleCapabilities);
  const fixturePath = `fixtures/desired-schema-${slug(candidate.kind)}.json`;
  const bytes = emit(`conformance/takoform-v1/generic-host/portable-host/${fixturePath}`, definition.desiredSchema);
  probe.desiredSchema = { path: fixturePath, sha256: `sha256:${sha256(bytes)}` };
}

const interfaceSet = readJSON(currentIndex.interfaceCandidateSet.path);
const interfaces = new Map(interfaceSet.interfaces.map((entry) => [`${entry.name}@${entry.version}`, entry]));
const runtime = interfaces.get(`${contract.runnerInput.supportProbes.runtimeContract.name}@${contract.runnerInput.supportProbes.runtimeContract.version}`);
if (runtime === undefined) throw new Error("current interface set has no worker.runtime@1.1.0");
contract.runnerInput.supportProbes.runtimeContract.schemaDigest = runtime.schemaDigest;
contract.runnerInput.supportProbes.dataInterfaces = contract.runnerInput.supportProbes.dataInterfaces
  .filter((entry) => entry.name !== "edge.objects")
  .map((entry) => {
    const current = interfaces.get(`${entry.name}@${entry.version}`);
    if (current === undefined) throw new Error(`current interface set has no ${entry.name}@${entry.version}`);
    return { ...entry, schemaDigest: current.schemaDigest };
  });

const baseVersionDesired = structuredClone(contract.runnerInput.workerVersion.desired);
function serviceSpec(protocol, required = true) {
  return {
    ...structuredClone(baseVersionDesired),
    externalServices: [{
      name: protocol === S3 ? "ASSETS" : "FUTURE_STORE",
      ...(required ? {} : { required: false }),
      service: { apiVersion: SERVICE_API, protocol },
    }],
  };
}
contract.runnerInput.externalServices = {
  property: "externalServices",
  serviceApiVersion: SERVICE_API,
  protocols: [S3],
  desiredSpec: serviceSpec(S3),
  unknownProtocolSpec: serviceSpec(UNKNOWN_SERVICE),
  optionalUnsupportedSpec: serviceSpec(UNKNOWN_SERVICE, false),
};

const synthetic = structuredClone(readJSON(
  "conformance/portable-host-v1beta4/fixtures/synthetic-module-worker-second-definition-edge-family.json",
));
synthetic.requiresHostApi = HOST_LANE;
synthetic.description = synthetic.description.replace("this lane", "the stable v1 lane");
const syntheticBytes = emit(
  "conformance/takoform-v1/generic-host/portable-host/fixtures/synthetic-module-worker-second-definition.json",
  synthetic,
);
contract.runnerInput.syntheticSecondDefinitionVersion.formRef = {
  apiVersion: synthetic.apiVersion,
  kind: synthetic.kind,
  definitionVersion: synthetic.definitionVersion,
  schemaDigest: canonicalDigest(synthetic),
};
contract.runnerInput.syntheticSecondDefinitionVersion.path =
  "fixtures/synthetic-module-worker-second-definition.json";
contract.runnerInput.syntheticSecondDefinitionVersion.sha256 = `sha256:${sha256(syntheticBytes)}`;

const workerService = interfaces.get("worker.service@1.0.0");
if (workerService === undefined) throw new Error("current interface set has no worker.service@1.0.0");
const workerServiceRef = {
  apiVersion: "interfaces.takoform.com/v1alpha1",
  name: workerService.name,
  version: workerService.version,
  schemaDigest: workerService.schemaDigest,
};
const constraintGroup = "constraints.forms.takoform.com";
const lifecycle = ["create", "read", "update", "delete", "import", "observe"];
function referenceSchema(kind, requiredInterface = workerServiceRef) {
  return {
    type: "object",
    additionalProperties: false,
    properties: {
      apiVersion: { type: "string", const: constraintGroup },
      kind: { type: "string", const: kind },
      name: { type: "string", minLength: 1, maxLength: 63, pattern: "^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$" },
    },
    required: ["apiVersion", "kind", "name"],
    ...(requiredInterface === null ? {} : { "x-takoform-required-interface": requiredInterface }),
  };
}
function closedSchema(properties, required = []) {
  return {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties,
    ...(required.length === 0 ? {} : { required }),
  };
}
function constraintDefinition(
  kind,
  version,
  role,
  desiredSchema,
  constraints = undefined,
  providedInterfaces = undefined,
  capabilities = lifecycle,
) {
  return {
    apiVersion: constraintGroup,
    kind,
    definitionVersion: version,
    title: `${kind} stable conformance probe`,
    description: "Synthetic stable-v1 conformance Definition; never a published family member.",
    role,
    requiresHostApi: HOST_LANE,
    desiredSchema,
    lifecycleCapabilities: capabilities,
    ...(constraints === undefined ? {} : { constraints }),
    ...(providedInterfaces === undefined ? {} : { providedInterfaces }),
  };
}
const constraintDefinitions = {};
constraintDefinitions.node = constraintDefinition(
  "ConstraintNode", "0.1.0", "identity",
  closedSchema({ next: referenceSchema("ConstraintNode") }),
  [{ kind: "acyclic", reference: "/next" }], [workerServiceRef],
);
constraintDefinitions.distinctPair = constraintDefinition(
  "DistinctPairHolder", "0.1.0", "attachment",
  closedSchema({ left: referenceSchema("ConstraintNode"), right: referenceSchema("ConstraintNode") }, ["left"]),
  [{ kind: "distinctPair", references: ["/left", "/right"] }],
);
constraintDefinitions.uniquePair = constraintDefinition(
  "UniquePairHolder", "0.1.0", "attachment",
  closedSchema({ left: referenceSchema("ConstraintNode"), right: referenceSchema("ConstraintNode") }, ["left", "right"]),
  [{ kind: "uniquePair", references: ["/left", "/right"] }],
);
constraintDefinitions.uniquePairSecond = constraintDefinition(
  "UniquePairHolder", "0.2.0", "attachment",
  closedSchema({ left: referenceSchema("ConstraintNode"), right: referenceSchema("ConstraintNode") }, ["left", "right"]),
  [{ kind: "uniquePair", references: ["/left", "/right"] }],
);
constraintDefinitions.member = constraintDefinition(
  "ConstraintMember", "0.1.0", "revision",
  closedSchema({ through: referenceSchema("ConstraintNode") }, ["through"]),
  undefined,
  undefined,
  ["create", "read", "delete", "import", "observe"],
);
const memberRef = {
  apiVersion: constraintGroup,
  kind: constraintDefinitions.member.kind,
  definitionVersion: constraintDefinitions.member.definitionVersion,
  schemaDigest: canonicalDigest(constraintDefinitions.member),
};
const exactMemberReference = {
  ...referenceSchema("ConstraintMember", null),
  "x-takoform-target-formrefs": [memberRef],
};
constraintDefinitions.sameTarget = constraintDefinition(
  "SameTargetHolder", "0.1.0", "deployment",
  closedSchema({
    anchor: referenceSchema("ConstraintNode"),
    members: { type: "array", minItems: 1, maxItems: 8, items: exactMemberReference },
  }, ["anchor", "members"]),
  [{ kind: "sameResolvedTarget", anchor: "/anchor", members: "/members/*", through: "/through" }],
);
constraintDefinitions.structural = constraintDefinition(
  "StructuralConstraintHolder", "0.1.0", "policy",
  closedSchema({
    lower: { type: "integer" },
    upper: { type: "integer" },
    rows: {
      type: "array", minItems: 1, maxItems: 8,
      items: {
        type: "object", additionalProperties: false,
        properties: { key: { type: "string" }, value: { type: "integer" } },
        required: ["key", "value"],
      },
    },
  }, ["lower", "upper", "rows"]),
  [
    { kind: "orderedPair", references: ["/lower", "/upper"] },
    { kind: "uniqueBy", list: "/rows", member: "key" },
  ],
);
contract.runnerInput.constraintSemantics = {};
for (const [label, definition] of Object.entries(constraintDefinitions)) {
  const fixture = `fixtures/constraint-${slug(label)}.json`;
  const bytes = emit(`conformance/takoform-v1/generic-host/portable-host/${fixture}`, definition);
  contract.runnerInput.constraintSemantics[label] = {
    name: `constraint-${slug(label)}-probe`,
    formRef: {
      apiVersion: definition.apiVersion,
      kind: definition.kind,
      definitionVersion: definition.definitionVersion,
      schemaDigest: canonicalDigest(definition),
    },
    path: fixture,
    sha256: `sha256:${sha256(bytes)}`,
  };
}

for (const fixture of contract.runnerInput.negativeFixtures) {
  const bytes = readFileSync(path.join(betaRoot, fixture.path));
  emit(`conformance/takoform-v1/generic-host/portable-host/${fixture.path}`, bytes);
  fixture.sha256 = `sha256:${sha256(bytes)}`;
}

const contractBytes = emit(
  "conformance/takoform-v1/generic-host/portable-host/contract.json",
  contract,
);
emit("conformance/takoform-v1/generic-host/portable-host/manifest.json", {
  format: "takoform.portable-host-conformance-manifest@v1",
  contract: "contract.json",
  sha256: sha256(contractBytes),
});

const genericChecks = [
  "declared-constraint-semantics-enforced",
  "exact-form-ref-resolution",
  "generic-lifecycle-semantics",
  "optional-unsupported-standard-service-omitted",
  "required-unsupported-standard-service-refused-before-mutation",
  "supported-standard-service-resolved",
];
const genericCorpus = {
  format: "takoform.generic-host-corpus@v1",
  hostApiLane: HOST_LANE,
  requiredChecks: genericChecks,
  scenarios: genericChecks.map((check) => ({
    check,
    input: { operation: "portable-host-self-test", portableHostCheck: check },
    expected: { status: "passed", mutationBoundary: check.includes("required-unsupported") ? "before" : "preserved" },
  })),
  portableHostContract: {
    path: "generic-host/portable-host/contract.json",
    sha256: `sha256:${sha256(contractBytes)}`,
  },
};
const genericPath = "conformance/takoform-v1/generic.json";
const genericBytes = emit(genericPath, genericCorpus);

// ---- one executable semantic corpus per current family ----

function collectGroups(value, knownGroups, into) {
  if (Array.isArray(value)) {
    for (const entry of value) collectGroups(entry, knownGroups, into);
    return;
  }
  if (value === null || typeof value !== "object") return;
  if (typeof value.const === "string" && knownGroups.has(value.const)) into.add(value.const);
  for (const entry of Object.values(value)) collectGroups(entry, knownGroups, into);
}

const knownGroups = new Set(familyInputs.map(({ entry }) => entry.group));
const familyRecords = [];
const allRefs = [];
for (const { entry, forms } of familyInputs) {
  const runnerInput = {};
  const dependencies = new Set();
  for (const { candidate, definition, desired } of forms) {
    const fixture = `fixtures/${slug(candidate.kind)}-desired-schema.json`;
    const corpusPath = `conformance/takoform-v1/families/${entry.group}.json`;
    const fixtureBytes = emit(`conformance/takoform-v1/families/${fixture}`, definition.desiredSchema);
    runnerInput[lowerCamel(candidate.kind)] = {
      name: `${slug(candidate.kind)}-probe`,
      identity: {
        formRef: structuredClone(candidate.formRef),
        packageDigest: candidate.packageDigest,
      },
      lifecycleCapabilities: structuredClone(definition.lifecycleCapabilities),
      desired: structuredClone(desired),
      desiredSchema: { path: fixture, sha256: `sha256:${sha256(fixtureBytes)}` },
    };
    collectGroups(definition.desiredSchema, knownGroups, dependencies);
    allRefs.push(structuredClone(candidate.formRef));
    void corpusPath;
  }
  dependencies.delete(entry.group);
  const requiredChecks = [
    "candidate-desired-schema-validated",
    "exact-form-ref-lifecycle",
    "lifecycle-capabilities-preserved",
    ...(entry.group === "edge.forms.takoform.com" ? ["stable-standard-service-support-enforced"] : []),
  ].sort();
  const serviceFixtures = entry.group === "edge.forms.takoform.com"
    ? [
        { serviceRef: { apiVersion: SERVICE_API, protocol: S3 }, satisfiable: true },
        { serviceRef: { apiVersion: SERVICE_API, protocol: UNKNOWN_SERVICE }, satisfiable: false },
      ]
    : [];
  const corpus = {
    format: "takoform.family-semantic-corpus@v1",
    hostApiLane: HOST_LANE,
    group: entry.group,
    candidateSet: { path: entry.candidateSet, sha256: entry.sha256 },
    requiredChecks,
    scenarios: requiredChecks.map((check) => ({
      check,
      input: {
        operation: check,
        exactFormRefs: forms.map(({ candidate }) => candidate.formRef),
        candidateCount: forms.length,
      },
      expected: { status: "passed", exactCandidateCount: forms.length },
    })),
    runnerInput,
    standardServiceFixtures: serviceFixtures,
  };
  const corpusPath = `conformance/takoform-v1/families/${entry.group}.json`;
  const bytes = emit(corpusPath, corpus);
  familyRecords.push({
    group: entry.group,
    path: corpusPath,
    sha256: sha256(bytes),
    requiredChecks,
    dependencyGroups: [...dependencies].sort(),
  });
}

allRefs.sort((a, b) => exactRefKey(a).localeCompare(exactRefKey(b)));
const wrongRef = structuredClone(allRefs[0]);
wrongRef.schemaDigest = `sha256:${"0".repeat(64)}`;
const familyGroups = familyInputs.map(({ entry }) => entry.group);
const compositionChecks = ["all-family-composition-resolves", "wrong-digest-refused"];
const composition = {
  format: "takoform.all-family-composition-corpus@v1",
  hostApiLane: HOST_LANE,
  familyGroups,
  requiredChecks: compositionChecks,
  scenarios: [
    {
      check: compositionChecks[0],
      input: { familyGroups, formRefs: allRefs, resolution: "exact-only" },
      expected: { status: "passed", resolvedFormCount: allRefs.length, unresolvedFormCount: 0 },
    },
    {
      check: compositionChecks[1],
      input: { familyGroups, formRefs: [wrongRef], resolution: "exact-only" },
      expected: { status: "passed", refusal: "form_unknown", mutated: false },
    },
  ],
};
const compositionPath = "conformance/takoform-v1/composition.json";
const compositionBytes = emit(compositionPath, composition);

emit("conformance/takoform-v1/manifest.json", {
  format: "takoform.conformance-suite@v1",
  hostApiLane: HOST_LANE,
  generic: { path: genericPath, sha256: sha256(genericBytes), requiredChecks: genericChecks },
  families: familyRecords,
  composition: {
    path: compositionPath,
    sha256: sha256(compositionBytes),
    requiredChecks: compositionChecks,
  },
  runner: {
    command: [
      "go", "run", "./cmd/portable-host-conformance", "suite",
      "--manifest", "conformance/takoform-v1/manifest.json",
    ],
    reportFormat: "takoform.reference-host-suite-report@v1",
  },
});

// ---- stable Host wire documents ----

const dnsLabel = "[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?";
const stableFormRef = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  $id: "https://forms.takoform.com/schemas/v1/form-ref.schema.json",
  title: "Takoform exact FormRef v1",
  description: "The immutable identity of one Form Definition. Every lookup uses all four members; no member is a latest selector and no lookup falls back to another definition.",
  type: "object",
  additionalProperties: false,
  required: ["apiVersion", "kind", "definitionVersion", "schemaDigest"],
  properties: {
    apiVersion: {
      type: "string",
      maxLength: 253,
      pattern: `^${dnsLabel}(?:\\.${dnsLabel})+$`,
      description: "Versionless reverse-DNS Form Family group. A slash and a family-version suffix are invalid in stable v1.",
    },
    kind: { type: "string", pattern: "^[A-Z][A-Za-z0-9]{0,63}$" },
    definitionVersion: {
      type: "string",
      pattern: "^(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)\\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)(\\.(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*))*))?(\\+([0-9A-Za-z-]+(\\.[0-9A-Za-z-]+)*))?$",
    },
    schemaDigest: { type: "string", pattern: "^sha256:[0-9a-f]{64}$" },
  },
};
emit("spec/schemas/form-ref-v1.schema.json", stableFormRef);

const stableWire = JSON.parse(
  JSON.stringify(readJSON("spec/schemas/host-api-wire-v1beta4.schema.json"))
    .replaceAll(
      "https://forms.takoform.com/schemas/operations/v1alpha2/operation.schema.json",
      "https://forms.takoform.com/schemas/operations/v1/operation.schema.json",
    )
    .replaceAll(
      "https://forms.takoform.com/schemas/support/v1alpha2/host-support-profile.schema.json",
      "https://forms.takoform.com/schemas/support/v1/host-support-profile.schema.json",
    ),
);
stableWire.$id = "https://forms.takoform.com/schemas/v1/host-api-wire.schema.json";
stableWire.title = "Takoform portable Host API wire envelopes v1";
stableWire.$defs.formReference.properties.formRef.$ref =
  "https://forms.takoform.com/schemas/v1/form-ref.schema.json";
stableWire.$defs.resourceCore.properties.apiVersion = {
  ...structuredClone(stableFormRef.properties.apiVersion),
  "x-takoform-equals": "/form/formRef/apiVersion",
};
emit("spec/schemas/host-api-wire-v1.schema.json", stableWire);

const stableDiscovery = structuredClone(readJSON("spec/schemas/host-discovery-v1beta4.schema.json"));
stableDiscovery.$id = "https://forms.takoform.com/schemas/v1/host-discovery.schema.json";
stableDiscovery.title = "Takoform Host discovery v1";
stableDiscovery.properties.api_versions.const = [HOST_LANE];
stableDiscovery.properties.endpoints.properties.api.allOf[1].pattern =
  "^https?://[^/?#]+/apis/forms\\.takoform\\.com/v1$";
emit("spec/schemas/host-discovery-v1.schema.json", stableDiscovery);

const stableOperation = structuredClone(readJSON("spec/schemas/operation-v1alpha2.schema.json"));
stableOperation.$id = "https://forms.takoform.com/schemas/operations/v1/operation.schema.json";
stableOperation.title = "Takoform long-running Operation wire contract v1";
emit("spec/schemas/operation-v1.schema.json", stableOperation);

let stableOperations = JSON.parse(
  JSON.stringify(readJSON("spec/host-api/operations-v1beta4.json"))
    .replaceAll("v1beta4", "v1")
    .replaceAll("{formGroup}/{formVersion}", "{formGroup}"),
);
stableOperations = JSON.parse(
  JSON.stringify(stableOperations)
    .replaceAll(
      "https://forms.takoform.com/schemas/operations/v1alpha2/operation.schema.json",
      "https://forms.takoform.com/schemas/operations/v1/operation.schema.json",
    )
    .replaceAll(
      "https://forms.takoform.com/schemas/support/v1alpha2/host-support-profile.schema.json",
      "https://forms.takoform.com/schemas/support/v1/host-support-profile.schema.json",
    ),
);
stableOperations.format = "takoform.host-api@v1";
stableOperations.apiGroup = HOST_LANE;
stableOperations.pathShape.namespacedGroup = "one-ordinary-path-segment-containing-the-whole-versionless-family-group";
for (const operation of stableOperations.operations) {
  if (Array.isArray(operation.queryParameters)) {
    operation.queryParameters = operation.queryParameters.filter((parameter) => parameter.name !== "version");
  }
  if (typeof operation.queryRule === "string") {
    operation.queryRule = operation.queryRule
      .replace("all six keys", "all five keys")
      .replace("all six", "all five")
      .replace("six query", "five query");
  }
}
emit("spec/host-api/operations-v1.json", stableOperations);

function generatedFiles(directory) {
  const result = [];
  function walk(current) {
    for (const name of readdirSync(current)) {
      const absolute = path.join(current, name);
      if (statSync(absolute).isDirectory()) walk(absolute);
      else result.push(path.relative(root, absolute).split(path.sep).join("/"));
    }
  }
  try {
    walk(directory);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  return result;
}

if (mode === "--write") {
  rmSync(targetRoot, { recursive: true, force: true });
  for (const [relativePath, bytes] of outputs) {
    const absolute = path.join(root, relativePath);
    mkdirSync(path.dirname(absolute), { recursive: true });
    writeFileSync(absolute, bytes);
  }
  console.log(`derived stable Takoform v1 suite: ${familyInputs.length} families, ${allRefs.length} exact Forms, ${contract.requiredRunnerChecks.length} generic Host checks`);
} else {
  const drift = [];
  for (const [relativePath, bytes] of outputs) {
    let actual;
    try {
      actual = readFileSync(path.join(root, relativePath));
    } catch {
      drift.push(`${relativePath}: missing`);
      continue;
    }
    if (!actual.equals(bytes)) drift.push(`${relativePath}: drifted`);
  }
  for (const relativePath of generatedFiles(targetRoot)) {
    if (!outputs.has(relativePath)) drift.push(`${relativePath}: unexpected`);
  }
  if (drift.length > 0) {
    for (const line of drift.sort()) process.stderr.write(`- ${line}\n`);
    throw new Error("stable Takoform v1 conformance suite is stale; run bun scripts/takoform-v1-derive.mjs --write");
  }
  console.log(`stable Takoform v1 suite is current: ${familyInputs.length} families, ${allRefs.length} exact Forms`);
}

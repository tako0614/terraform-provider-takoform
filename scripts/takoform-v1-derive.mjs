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

// ---- Edge family/concrete-Host adapter (the retained 125-check matrix) ----

const edgeHostRoot = "conformance/takoform-v1/family-host/edge/portable-host";
const edgeHostContract = structuredClone(readJSON("conformance/portable-host-v1beta4/contract.json"));
edgeHostContract.format = "takoform.portable-host-conformance@v1";
edgeHostContract.apiVersion = HOST_LANE;
edgeHostContract.discoveryPath = "/.well-known/takoform/v1";
edgeHostContract.apiPath = "/apis/forms.takoform.com/v1";
edgeHostContract.familyCandidateSet = path.posix.dirname(edge.entry.candidateSet);
edgeHostContract.bindingCandidateSet = path.posix.dirname(currentIndex.bindingCandidateSet.path);
edgeHostContract.runnerInput.syntheticSecondGroup.apiVersion = "other.forms.takoform.com";

const edgeHostFamilyDerivedChecks = new Set(["class-holder-rules-enforced"]);
const edgeHostLaneChecks = edgeHostContract.requiredRunnerChecks
  .filter((check) => !edgeHostFamilyDerivedChecks.has(check));
const edgeHostDerivedChecks = edgeHostContract.requiredRunnerChecks
  .filter((check) => edgeHostFamilyDerivedChecks.has(check));
edgeHostContract.requiredRunnerChecks = [
  ...edgeHostLaneChecks.map((check) => check === "external-service-slots-sealed"
    ? "stable-standard-service-support-enforced"
    : check),
  "declared-constraint-semantics-enforced",
  ...edgeHostDerivedChecks,
];

const edgeHostProbeByKind = new Map();
for (const value of Object.values(edgeHostContract.runnerInput)) {
  const probe = value?.resourceProbe?.identity?.formRef !== undefined ? value.resourceProbe : value;
  if (probe?.identity?.formRef?.kind !== undefined) edgeHostProbeByKind.set(probe.identity.formRef.kind, probe);
}
for (const { candidate, definition } of edge.forms) {
  const probe = edgeHostProbeByKind.get(candidate.kind);
  if (probe === undefined) throw new Error(`stable Edge Host adapter has no ${candidate.kind} lifecycle probe`);
  probe.identity.formRef = structuredClone(candidate.formRef);
  probe.identity.packageDigest = candidate.packageDigest;
  probe.lifecycleCapabilities = structuredClone(definition.lifecycleCapabilities);
  const fixturePath = `fixtures/desired-schema-${slug(candidate.kind)}.json`;
  const bytes = emit(`${edgeHostRoot}/${fixturePath}`, definition.desiredSchema);
  probe.desiredSchema = { path: fixturePath, sha256: `sha256:${sha256(bytes)}` };
}

const interfaceSet = readJSON(currentIndex.interfaceCandidateSet.path);
const interfaces = new Map(interfaceSet.interfaces.map((entry) => [`${entry.name}@${entry.version}`, entry]));
const runtime = interfaces.get(
  `${edgeHostContract.runnerInput.supportProbes.runtimeContract.name}@${edgeHostContract.runnerInput.supportProbes.runtimeContract.version}`,
);
if (runtime === undefined) throw new Error("current interface set has no worker.runtime@1.1.0");
edgeHostContract.runnerInput.supportProbes.runtimeContract.schemaDigest = runtime.schemaDigest;
edgeHostContract.runnerInput.supportProbes.dataInterfaces = edgeHostContract.runnerInput.supportProbes.dataInterfaces
  .filter((entry) => entry.name !== "edge.objects")
  .map((entry) => {
    const current = interfaces.get(`${entry.name}@${entry.version}`);
    if (current === undefined) throw new Error(`current interface set has no ${entry.name}@${entry.version}`);
    return { ...entry, schemaDigest: current.schemaDigest };
  });

const edgeHostBaseVersionDesired = structuredClone(edgeHostContract.runnerInput.workerVersion.desired);
function edgeHostServiceSpec(protocol, required = true) {
  return {
    ...structuredClone(edgeHostBaseVersionDesired),
    externalServices: [{
      name: protocol === S3 ? "ASSETS" : "FUTURE_STORE",
      ...(required ? {} : { required: false }),
      service: { apiVersion: SERVICE_API, protocol },
    }],
  };
}
edgeHostContract.runnerInput.externalServices = {
  property: "externalServices",
  serviceApiVersion: SERVICE_API,
  protocols: [S3],
  desiredSpec: edgeHostServiceSpec(S3),
  unknownProtocolSpec: edgeHostServiceSpec(UNKNOWN_SERVICE),
  optionalUnsupportedSpec: edgeHostServiceSpec(UNKNOWN_SERVICE, false),
};

const edgeHostSynthetic = structuredClone(readJSON(
  "conformance/portable-host-v1beta4/fixtures/synthetic-module-worker-second-definition-edge-family.json",
));
edgeHostSynthetic.requiresHostApi = HOST_LANE;
edgeHostSynthetic.description = edgeHostSynthetic.description.replace("this lane", "the stable v1 lane");
const edgeHostSyntheticBytes = emit(
  `${edgeHostRoot}/fixtures/synthetic-module-worker-second-definition.json`,
  edgeHostSynthetic,
);
edgeHostContract.runnerInput.syntheticSecondDefinitionVersion.formRef = {
  apiVersion: edgeHostSynthetic.apiVersion,
  kind: edgeHostSynthetic.kind,
  definitionVersion: edgeHostSynthetic.definitionVersion,
  schemaDigest: canonicalDigest(edgeHostSynthetic),
};
edgeHostContract.runnerInput.syntheticSecondDefinitionVersion.path =
  "fixtures/synthetic-module-worker-second-definition.json";
edgeHostContract.runnerInput.syntheticSecondDefinitionVersion.sha256 =
  `sha256:${sha256(edgeHostSyntheticBytes)}`;

const workerService = interfaces.get("worker.service@1.0.0");
if (workerService === undefined) throw new Error("current interface set has no worker.service@1.0.0");
const workerServiceRef = {
  apiVersion: "interfaces.takoform.com/v1alpha1",
  name: workerService.name,
  version: workerService.version,
  schemaDigest: workerService.schemaDigest,
};
const constraintGroup = "constraints.forms.takoform.com";
const edgeHostLifecycle = ["create", "read", "update", "delete", "import", "observe"];
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
  capabilities = edgeHostLifecycle,
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

edgeHostContract.runnerInput.constraintSemantics = {};
for (const [label, definition] of Object.entries(constraintDefinitions)) {
  const fixture = `fixtures/constraint-${slug(label)}.json`;
  const bytes = emit(`${edgeHostRoot}/${fixture}`, definition);
  edgeHostContract.runnerInput.constraintSemantics[label] = {
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

for (const fixture of edgeHostContract.runnerInput.negativeFixtures) {
  const bytes = readFileSync(path.join(betaRoot, fixture.path));
  emit(`${edgeHostRoot}/${fixture.path}`, bytes);
  fixture.sha256 = `sha256:${sha256(bytes)}`;
}

const edgeHostContractBytes = emit(`${edgeHostRoot}/contract.json`, edgeHostContract);
emit(`${edgeHostRoot}/manifest.json`, {
  format: "takoform.portable-host-conformance-manifest@v1",
  contract: "contract.json",
  sha256: sha256(edgeHostContractBytes),
});
const previousEdgeHostRoot = "conformance/takoform-v1/generic-host/portable-host";
const edgeHostRehome = [...outputs.entries()]
  .filter(([relativePath]) => relativePath.startsWith(`${edgeHostRoot}/`))
  .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
  .map(([newPath, bytes]) => ({
    oldPath: `${previousEdgeHostRoot}/${newPath.slice(edgeHostRoot.length + 1)}`,
    newPath,
    sha256: sha256(bytes),
  }));
if (edgeHostRehome.length !== 28) {
  throw new Error(`stable Edge Host rehome ledger has ${edgeHostRehome.length} files, want 28`);
}
emit("conformance/takoform-v1/family-host/edge/rehome-ledger.json", {
  format: "takoform.edge-host-rehome@v1",
  fileCount: edgeHostRehome.length,
  files: edgeHostRehome,
});
// These are stable published bytes, not an active generic input. Keep their
// historical addresses byte-identical while every executable consumer points
// at the Edge family/concrete-Host owner above.
for (const entry of edgeHostRehome) {
  emit(entry.oldPath, outputs.get(entry.newPath));
}

// ---- family-neutral generic Snapshot and Host lifecycle corpus ----

const externalGroup = "resources.publisher.example";
const externalKind = "RangeCounter";
const externalPackageRoot = "conformance/takoform-v1/generic-host/external-family/range-counter";
const supportedService = S3;
const unsupportedService = UNKNOWN_SERVICE;
const lifecycle = ["create", "read", "update", "delete", "import", "observe"];
const standardServiceProperty = {
  default: [],
  type: "array",
  maxItems: 4,
  uniqueItems: true,
  items: {
    type: "object",
    additionalProperties: false,
    properties: {
      name: { type: "string", pattern: "^[A-Z][A-Z0-9_]*$", maxLength: 64 },
      required: { type: "boolean", default: true },
      service: {
        type: "object",
        additionalProperties: false,
        properties: {
          apiVersion: { const: SERVICE_API },
          protocol: {
            type: "string",
            maxLength: 253,
            pattern: "^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?){2,}$",
          },
        },
        required: ["apiVersion", "protocol"],
      },
    },
    required: ["name", "service"],
  },
  "x-takoform-standard-services": SERVICE_API,
};
const externalDefinition = {
  apiVersion: externalGroup,
  kind: externalKind,
  definitionVersion: "0.1.0",
  title: "Independent range counter",
  description: "Synthetic external publisher fixture for family-neutral generic conformance.",
  role: "identity",
  requiresHostApi: HOST_LANE,
  desiredSchema: {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: {
      lower: { type: "integer" },
      upper: { type: "integer" },
      mode: { type: "string", enum: ["safe", "fast"], default: "safe" },
      externalServices: standardServiceProperty,
    },
    required: ["lower", "upper"],
  },
  immutableFields: ["/externalServices"],
  lifecycleCapabilities: lifecycle,
  constraints: [{ kind: "orderedPair", references: ["/lower", "/upper"] }],
  conformanceFixtures: [{ name: "canonical", desiredPath: "fixtures/desired.json" }],
  negativeConformanceFixtures: [{
    name: "reject-unexpected-property",
    stage: "desired",
    inputPath: "fixtures/negative-unexpected-property.json",
    expectedFailure: "schema_validation_failed",
  }],
};
const externalRef = {
  apiVersion: externalGroup,
  kind: externalKind,
  definitionVersion: externalDefinition.definitionVersion,
  schemaDigest: canonicalDigest(externalDefinition),
};
const externalDesired = { lower: 1, upper: 2 };
const externalNegative = { lower: 1, upper: 2, unexpected: true };
const externalDefinitionBytes = emit(`${externalPackageRoot}/definition.json`, externalDefinition);
const externalDesiredBytes = emit(`${externalPackageRoot}/fixtures/desired.json`, externalDesired);
const externalNegativeBytes = emit(
  `${externalPackageRoot}/fixtures/negative-unexpected-property.json`,
  externalNegative,
);
const externalPackageIndex = {
  apiVersion: "packages.forms.takoform.com/v1alpha5",
  kind: "FormPackage",
  formRef: externalRef,
  definitionPath: "definition.json",
  files: [
    {
      path: "definition.json",
      mediaType: "application/vnd.takoform.form-definition.v1+json",
      size: externalDefinitionBytes.length,
      digest: `sha256:${sha256(externalDefinitionBytes)}`,
    },
    {
      path: "fixtures/desired.json",
      mediaType: "application/json",
      size: externalDesiredBytes.length,
      digest: `sha256:${sha256(externalDesiredBytes)}`,
    },
    {
      path: "fixtures/negative-unexpected-property.json",
      mediaType: "application/json",
      size: externalNegativeBytes.length,
      digest: `sha256:${sha256(externalNegativeBytes)}`,
    },
  ],
};
emit(`${externalPackageRoot}/package-index.json`, externalPackageIndex);
const externalPackageDigest = canonicalDigest(externalPackageIndex);

function serviceSpec(protocol, required = true) {
  return {
    lower: 1,
    upper: 2,
    externalServices: [{
      name: protocol === supportedService ? "OBJECTS" : "FUTURE",
      ...(required ? {} : { required: false }),
      service: { apiVersion: SERVICE_API, protocol },
    }],
  };
}

function emitNeutralPackage(directory, definition) {
  const definitionBytes = emit(`${directory}/definition.json`, definition);
  const formRef = {
    apiVersion: definition.apiVersion,
    kind: definition.kind,
    definitionVersion: definition.definitionVersion,
    schemaDigest: canonicalDigest(definition),
  };
  const packageIndex = {
    apiVersion: "packages.forms.takoform.com/v1alpha5",
    kind: "FormPackage",
    formRef,
    definitionPath: "definition.json",
    files: [{
      path: "definition.json",
      mediaType: "application/vnd.takoform.form-definition.v1+json",
      size: definitionBytes.length,
      digest: `sha256:${sha256(definitionBytes)}`,
    }],
  };
  emit(`${directory}/package-index.json`, packageIndex);
  return {
    formRef,
    packageDigest: canonicalDigest(packageIndex),
    path: `${directory.slice("conformance/takoform-v1/".length)}/package-index.json`,
    definition,
  };
}

function neutralDefinition(kind, version, role, desiredSchema, capabilities = lifecycle, extra = {}) {
  return {
    apiVersion: externalGroup,
    kind,
    definitionVersion: version,
    title: `${kind} independent conformance fixture`,
    description: `External publisher ${kind} Definition for family-neutral Host conformance.`,
    role,
    requiresHostApi: HOST_LANE,
    desiredSchema,
    lifecycleCapabilities: capabilities,
    ...extra,
  };
}

const neutralScalarSchema = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  type: "object",
  additionalProperties: false,
  properties: {
    lower: { type: "integer" },
    upper: { type: "integer" },
    mode: { type: "string", enum: ["safe", "fast"], default: "safe" },
  },
  required: ["lower", "upper"],
};
const neutralGauge = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/range-gauge",
  neutralDefinition("RangeGauge", "0.1.0", "identity", neutralScalarSchema),
);
const neutralSequence = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/range-sequence",
  neutralDefinition("RangeSequence", "0.1.0", "identity", neutralScalarSchema),
);
const neutralSecondDefinition = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/range-counter-v0-2",
  neutralDefinition("RangeCounter", "0.2.0", "identity", {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: {},
  }),
);
const neutralOtherGroup = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/other-range-gauge",
  {
    ...neutralDefinition("RangeGauge", "0.1.0", "identity", {
      $schema: "https://json-schema.org/draft/2020-12/schema",
      type: "object",
      additionalProperties: false,
      properties: {},
    }),
    apiVersion: "records.publisher.example",
    title: "Independent second-group range gauge",
    description: "A second external publisher owns the same Kind without sharing identity or state.",
  },
);
const neutralRevision = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/revision-note",
  neutralDefinition("RevisionNote", "0.1.0", "revision", {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: {
      note: { type: "string", minLength: 1 },
    },
    required: ["note"],
  }, ["create", "read", "delete", "import", "observe"]),
);
const neutralOutputs = {
  assignedName: "counter-001.publisher.example",
  ordinal: 7,
};
const neutralOutput = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/assigned-counter",
  neutralDefinition("AssignedCounter", "0.1.0", "identity", {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: {
      label: { type: "string", minLength: 1 },
      assignedName: { type: "string", pattern: "^[a-z0-9.-]+$" },
      ordinal: { type: "integer", minimum: 1 },
    },
    required: ["label"],
  }, lifecycle, {
    outputSchema: {
      $schema: "https://json-schema.org/draft/2020-12/schema",
      type: "object",
      additionalProperties: false,
      properties: {
        assignedName: { type: "string", pattern: "^[a-z0-9.-]+$" },
        ordinal: { type: "integer", minimum: 1 },
      },
      required: ["assignedName", "ordinal"],
    },
    constraints: [
      { kind: "hostAssigned", output: "/assignedName" },
      { kind: "hostAssigned", output: "/ordinal" },
    ],
  }),
);

function neutralExactReferenceSchema(ref) {
  return {
    type: "object",
    additionalProperties: false,
    properties: {
      apiVersion: { type: "string", const: ref.apiVersion },
      kind: { type: "string", const: ref.kind },
      name: { type: "string", minLength: 1, maxLength: 63, pattern: "^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$" },
    },
    required: ["apiVersion", "kind", "name"],
    "x-takoform-target-formrefs": [ref],
  };
}

function neutralHolder(kind, targetRef) {
  return emitNeutralPackage(
    `conformance/takoform-v1/generic-host/external-family/${slug(kind)}`,
    neutralDefinition(kind, "0.1.0", "attachment", {
      $schema: "https://json-schema.org/draft/2020-12/schema",
      type: "object",
      additionalProperties: false,
      properties: { target: neutralExactReferenceSchema(targetRef) },
      required: ["target"],
    }, ["create", "read", "delete", "import", "observe"], {
      constraints: [{ kind: "exclusive", reference: "/target" }],
    }),
  );
}
const neutralLease = neutralHolder("CounterLease", externalRef);
const neutralReservation = neutralHolder("CounterReservation", neutralSequence.formRef);
const neutralLeaseSecond = emitNeutralPackage(
  "conformance/takoform-v1/generic-host/external-family/counter-lease-v0-2",
  neutralDefinition("CounterLease", "0.2.0", "attachment", {
    $schema: "https://json-schema.org/draft/2020-12/schema",
    type: "object",
    additionalProperties: false,
    properties: { target: neutralExactReferenceSchema(externalRef) },
    required: ["target"],
  }, ["create", "read", "delete", "import", "observe"], {
    constraints: [{ kind: "exclusive", reference: "/target" }],
  }),
);

const neutralConstraintGroup = "constraints.publisher.example";
const neutralRelationInterfaceDefinition = {
  apiVersion: "interfaces.takoform.com/v1alpha1",
  kind: "InterfaceDefinition",
  name: "publisher.constraintnode",
  version: "0.1.0",
  operations: [{
    name: "resolve",
    inputSchema: {
      $schema: "https://json-schema.org/draft/2020-12/schema",
      type: "object",
    },
    outputSchema: {
      $schema: "https://json-schema.org/draft/2020-12/schema",
      type: "object",
    },
    errors: [],
  }],
  semantics: { consistency: "serializable" },
};
const neutralRelationInterfacePath = "generic-host/external-family/constraint-node-interface.json";
emit(`conformance/takoform-v1/${neutralRelationInterfacePath}`, neutralRelationInterfaceDefinition);
const neutralRelationInterface = {
  apiVersion: neutralRelationInterfaceDefinition.apiVersion,
  name: neutralRelationInterfaceDefinition.name,
  version: neutralRelationInterfaceDefinition.version,
  schemaDigest: canonicalDigest(neutralRelationInterfaceDefinition),
};
const neutralRelationBindingDefinition = {
  apiVersion: "bindings.takoform.com/v1alpha2",
  kind: "BindingDefinition",
  name: "publisher.constraintlink",
  version: "0.1.0",
  sourceRole: "attachment",
  targetInterface: neutralRelationInterface,
  allowedTargetForms: [{
    apiVersion: neutralConstraintGroup,
    kind: "ConstraintNode",
  }],
  bindingNameGrammar: "^[A-Za-z_$][A-Za-z0-9_$]*$",
  runtimeProjection: { operations: ["resolve"] },
  lifecycle: { targetDeletion: "refuse_while_bound" },
};
const neutralRelationBindingPath = "generic-host/external-family/constraint-link-binding.json";
const neutralRelationBinding = {
  apiVersion: neutralRelationBindingDefinition.apiVersion,
  name: neutralRelationBindingDefinition.name,
  version: neutralRelationBindingDefinition.version,
  schemaDigest: canonicalDigest(neutralRelationBindingDefinition),
};
emit(`conformance/takoform-v1/${neutralRelationBindingPath}`, neutralRelationBindingDefinition);
function neutralConstraintReference(kind, requiredInterface = neutralRelationInterface) {
  return {
    type: "object",
    additionalProperties: false,
    properties: {
      apiVersion: { type: "string", const: neutralConstraintGroup },
      kind: { type: "string", const: kind },
      name: { type: "string", minLength: 1, maxLength: 63, pattern: "^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$" },
    },
    required: ["apiVersion", "kind", "name"],
    ...(requiredInterface === null ? {} : { "x-takoform-required-interface": requiredInterface }),
  };
}
function neutralConstraintDefinition(kind, version, role, desiredSchema, constraints, providedInterfaces, capabilities = lifecycle) {
  return {
    apiVersion: neutralConstraintGroup,
    kind,
    definitionVersion: version,
    title: `${kind} independent constraint fixture`,
    description: "External publisher Definition for the family-neutral declared-constraint matrix.",
    role,
    requiresHostApi: HOST_LANE,
    desiredSchema,
    lifecycleCapabilities: capabilities,
    ...(constraints === undefined ? {} : { constraints }),
    ...(providedInterfaces === undefined ? {} : { providedInterfaces }),
  };
}
const neutralConstraintDefinitions = {};
neutralConstraintDefinitions.node = neutralConstraintDefinition(
  "ConstraintNode", "0.1.0", "identity",
  closedSchema({ next: neutralConstraintReference("ConstraintNode") }),
  [{ kind: "acyclic", reference: "/next" }], [neutralRelationInterface],
);
neutralConstraintDefinitions.distinctPair = neutralConstraintDefinition(
  "DistinctPairHolder", "0.1.0", "attachment",
  closedSchema({ left: neutralConstraintReference("ConstraintNode"), right: neutralConstraintReference("ConstraintNode") }, ["left"]),
  [{ kind: "distinctPair", references: ["/left", "/right"] }],
);
neutralConstraintDefinitions.uniquePair = neutralConstraintDefinition(
  "UniquePairHolder", "0.1.0", "attachment",
  closedSchema({ left: neutralConstraintReference("ConstraintNode"), right: neutralConstraintReference("ConstraintNode") }, ["left", "right"]),
  [{ kind: "uniquePair", references: ["/left", "/right"] }],
);
neutralConstraintDefinitions.uniquePairSecond = neutralConstraintDefinition(
  "UniquePairHolder", "0.2.0", "attachment",
  closedSchema({ left: neutralConstraintReference("ConstraintNode"), right: neutralConstraintReference("ConstraintNode") }, ["left", "right"]),
  [{ kind: "uniquePair", references: ["/left", "/right"] }],
);
neutralConstraintDefinitions.member = neutralConstraintDefinition(
  "ConstraintMember", "0.1.0", "revision",
  closedSchema({ through: neutralConstraintReference("ConstraintNode") }, ["through"]),
  undefined, undefined, ["create", "read", "delete", "import", "observe"],
);
const neutralMemberRef = {
  apiVersion: neutralConstraintGroup,
  kind: neutralConstraintDefinitions.member.kind,
  definitionVersion: neutralConstraintDefinitions.member.definitionVersion,
  schemaDigest: canonicalDigest(neutralConstraintDefinitions.member),
};
const neutralExactMemberReference = {
  ...neutralConstraintReference("ConstraintMember", null),
  "x-takoform-target-formrefs": [neutralMemberRef],
};
neutralConstraintDefinitions.sameTarget = neutralConstraintDefinition(
  "SameTargetHolder", "0.1.0", "deployment",
  closedSchema({
    anchor: neutralConstraintReference("ConstraintNode"),
    members: { type: "array", minItems: 1, maxItems: 8, items: neutralExactMemberReference },
  }, ["anchor", "members"]),
  [{ kind: "sameResolvedTarget", anchor: "/anchor", members: "/members/*", through: "/through" }],
);
neutralConstraintDefinitions.structural = neutralConstraintDefinition(
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
neutralConstraintDefinitions.sum = neutralConstraintDefinition(
  "WeightedSet", "0.1.0", "policy",
  closedSchema({
    weights: {
      type: "array", minItems: 1, maxItems: 8,
      items: {
        type: "object", additionalProperties: false,
        properties: { weight: { type: "integer" } },
        required: ["weight"],
      },
    },
  }, ["weights"]),
  [{ kind: "sum", list: "/weights", member: "weight", total: 100 }],
);
neutralConstraintDefinitions.claimPrimary = neutralConstraintDefinition(
  "ClaimTicket", "0.1.0", "attachment",
  closedSchema({ claim: { type: "string", minLength: 1, maxLength: 64 } }, ["claim"]),
  [{ kind: "claim", property: "/claim" }],
);
neutralConstraintDefinitions.claimAlternate = neutralConstraintDefinition(
  "ClaimLease", "0.1.0", "attachment",
  closedSchema({ alias: { type: "string", minLength: 1, maxLength: 64 } }, ["alias"]),
  [{ kind: "claim", property: "/alias" }],
);
const neutralConstraintPackages = {};
for (const [label, definition] of Object.entries(neutralConstraintDefinitions)) {
  neutralConstraintPackages[label] = emitNeutralPackage(
    `conformance/takoform-v1/generic-host/external-family/constraint-${slug(label)}`,
    definition,
  );
}

const retainedPortableContract = JSON.parse(readFileSync(path.join(betaRoot, "contract.json"), "utf8"));
const legacyPortableChecks = [
  ...retainedPortableContract.requiredRunnerChecks
    .filter((check) => check !== "class-holder-rules-enforced")
    .map((check) => check === "external-service-slots-sealed"
      ? "stable-standard-service-support-enforced"
      : check),
  "declared-constraint-semantics-enforced",
  "class-holder-rules-enforced",
];
if (legacyPortableChecks.length !== 125 || new Set(legacyPortableChecks).size !== 125) {
  throw new Error("stable legacy portable coverage must account for the exact old 125-check matrix");
}
const interfaceRuntimeChecks = new Set([
  "module-worker-runtime-contract-advertised",
  "undeclared-runtime-handler-rejected",
  "declared-handler-not-exported-rejected",
  "edge-interface-contracts-advertised",
  "support-profiles-present",
]);
const familySemanticChecks = new Set([
  "static-asset-spa-paths",
  "sqlite-migration-ledger-readiness",
  "artifact-manifest-reject-list",
  "artifact-manifest-kind-exclusive",
  "manifest-reference-is-not-a-capability",
  "cron-grammar-enforced",
  "queue-single-consumer-enforced",
  "custom-domain-hostname-canonicalized",
  "custom-domain-hostname-claim-unique",
  "custom-domain-hostname-claim-stops-at-the-tenant",
  "dead-letter-cycle-rejected",
  "attachment-claim-decided-on-import",
  "attachment-claim-revalidated-at-commit",
  "custom-domain-u-label-refused",
  "bundle-main-module-is-loadable",
  "class-holder-rules-enforced",
]);
const compositionOwnedChecks = new Set([
  "deployment-weight-sum-enforced",
  "binding-target-missing-404-before-mutation",
  "dependency-in-use-on-bound-target-delete",
  "relation-target-missing-rejected",
  "relation-target-deletion-blocked",
  "relation-incarnation-change-detected",
  "relation-reapply-repins",
  "binding-contract-verified",
  "artifact-then-bundle-apply",
  "artifact-retention-while-referenced",
  "deployment-single-active-per-worker",
  "deployment-version-ownership",
  "deployment-version-duplicate-rejected",
  "attachment-requires-active-deployment",
  "handler-gated-attachments",
  "binding-name-collision-rejected",
  "deployment-change-preserves-dependents",
  "deployment-delete-blocked-by-dependent",
  "deployment-delete-blocked-by-inbound-binding",
  "dependent-revision-advances-with-rendering",
  "delete-fence-survives-derived-rendering",
  "worker-endpoint-address-is-host-assigned",
  "worker-endpoint-single-per-worker",
  "worker-endpoint-follows-the-active-deployment",
  "worker-endpoint-address-is-stable-for-its-uid",
  "relation-target-form-ref-verified",
  "relation-target-interface-verified",
  "relation-pin-records-target-form-ref",
  "relation-resolution-is-tenant-scoped",
]);
const genericCheckSet = new Set([
  "apply-idempotency-replay",
  "artifact-commit-binds-declared-size",
  "artifact-digest-mismatch",
  "artifact-upload-missing-blob",
  "async-operation-flow",
  "create-conflict-when-exists",
  "declared-constraint-semantics-enforced",
  "declared-exclusive-holds-enforced",
  "delete-generation-fence",
  "delete-revision-fence",
  "delete-then-recreate-uid-changes",
  "error-envelope-taxonomy",
  "exact-form-ref-fails-closed-on-unknown-definition",
  "fence-matrix-observed",
  "operation-bound-to-its-creating-principal",
  "operation-cancel",
  "operation-replay-terminal",
  "operation-resumable-after-settlement",
  "portable-defaults-materialized",
  "prepare-binds-exact-spec",
  "prepare-is-tenant-scoped",
  "prepare-requires-update-fence",
  "replay-record-retires-with-its-incarnation",
  "spec-change-bumps-generation",
  "stale-revision-rejected",
  "status-change-bumps-revision-not-generation",
  "unauthenticated-request-refused",
  "upload-session-bound-to-its-creating-principal",
]);
function legacyCoverageOwner(check) {
  if (interfaceRuntimeChecks.has(check)) return "interface-runtime";
  if (familySemanticChecks.has(check)) return "family-semantic";
  if (compositionOwnedChecks.has(check)) return "composition";
  if (genericCheckSet.has(check)) return "generic-host";
  return "concrete-host";
}
const genericChecks = legacyPortableChecks
	.filter((check) => genericCheckSet.has(check))
	.sort();
if (genericChecks.length !== genericCheckSet.size || genericChecks.length === 0) {
	throw new Error("stable neutral generic corpus does not match its explicit evidence roster");
}
const legacyPortableCoverage = legacyPortableChecks.map((check) => {
  const owner = legacyCoverageOwner(check);
  return {
    check,
    owner,
    edgeAdapterCheck: check,
	...(genericCheckSet.has(check) ? { neutralChecks: [check] } : {}),
  };
});
emit("conformance/takoform-v1/family-host/edge/coverage-ledger.json", {
  format: "takoform.legacy-portable-coverage@v1",
  owner: "edge-family-concrete-host",
  contract: {
    path: "portable-host/contract.json",
    sha256: sha256(edgeHostContractBytes),
  },
  checkCount: legacyPortableChecks.length,
  coverage: legacyPortableCoverage,
});
const neutralArtifactBytes = "-- family-neutral conformance payload\n";
const neutralSnapshotPackages = [
  {
    path: "generic-host/external-family/range-counter/package-index.json",
    packageDigest: externalPackageDigest,
    formRef: externalRef,
  },
  neutralGauge,
  neutralSequence,
  neutralSecondDefinition,
  neutralOtherGroup,
  neutralRevision,
  neutralOutput,
  neutralLease,
  neutralLeaseSecond,
  neutralReservation,
  ...Object.values(neutralConstraintPackages),
].sort((left, right) => left.path.localeCompare(right.path));
const neutralExpectedRefs = neutralSnapshotPackages
  .map((entry) => structuredClone(entry.formRef))
  .sort((left, right) => exactRefKey(left).localeCompare(exactRefKey(right)));
const neutralDefaultCreates = [];
const neutralDefaultLines = new Set();
for (const formRef of neutralExpectedRefs) {
  const line = `${formRef.apiVersion}\u0000${formRef.kind}`;
  if (neutralDefaultLines.has(line)) continue;
  neutralDefaultLines.add(line);
  neutralDefaultCreates.push({ group: formRef.apiVersion, kind: formRef.kind, ref: structuredClone(formRef) });
}
neutralDefaultCreates.sort((left, right) =>
  `${left.group}/${left.kind}`.localeCompare(`${right.group}/${right.kind}`));
const genericCorpus = {
  format: "takoform.generic-host-corpus@v1",
  hostApiLane: HOST_LANE,
  requiredChecks: genericChecks,
  snapshotInputs: [
    {
      name: "external-family",
      packages: neutralSnapshotPackages.map(({ path, packageDigest }) => ({ path, packageDigest })),
      interfaces: [{
        path: neutralRelationInterfacePath,
        schemaDigest: neutralRelationInterface.schemaDigest,
      }],
      bindings: [{
        path: neutralRelationBindingPath,
        schemaDigest: neutralRelationBinding.schemaDigest,
      }],
      defaultCreates: neutralDefaultCreates,
      expectedFormRefs: neutralExpectedRefs,
    },
    {
      name: "zero-family",
      packages: [],
      interfaces: [],
      bindings: [],
      defaultCreates: [],
      expectedFormRefs: [],
    },
  ],
  externalHostProbe: {
    snapshot: "external-family",
    formRef: externalRef,
    packageDigest: externalPackageDigest,
    name: "range-counter-probe",
    space: "conformance",
    desired: externalDesired,
    updatedDesired: { lower: 1, upper: 3, mode: "fast" },
    invalidSchemaDesired: externalNegative,
    invalidConstraintDesired: { lower: 3, upper: 2 },
    externalServices: {
      property: "externalServices",
      serviceApiVersion: SERVICE_API,
      supportedProtocol: supportedService,
      unsupportedProtocol: unsupportedService,
      supportedDesired: serviceSpec(supportedService),
      requiredUnsupportedDesired: serviceSpec(unsupportedService),
      optionalUnsupportedDesired: serviceSpec(unsupportedService, false),
    },
    resources: {
      keyed: {
        formRef: neutralGauge.formRef,
        name: "range-gauge-probe",
        desired: { lower: 2, upper: 4 },
        updatedDesired: { lower: 2, upper: 5, mode: "fast" },
        secondUpdatedDesired: { lower: 2, upper: 6, mode: "fast" },
      },
      sequenced: {
        formRef: neutralSequence.formRef,
        name: "range-sequence-probe",
        desired: { lower: 3, upper: 6 },
        updatedDesired: { lower: 3, upper: 7, mode: "fast" },
        secondUpdatedDesired: { lower: 3, upper: 8, mode: "fast" },
      },
      revision: {
        formRef: neutralRevision.formRef,
        name: "revision-note-probe",
        desired: { note: "sealed" },
        updatedDesired: { note: "changed" },
      },
      output: {
        formRef: neutralOutput.formRef,
        name: "assigned-counter-probe",
        desired: { label: "primary" },
        hostAssignedOutputs: neutralOutputs,
      },
      lease: {
        formRef: neutralLease.formRef,
        name: "counter-lease-probe",
        desired: { target: { apiVersion: externalRef.apiVersion, kind: externalRef.kind, name: "range-counter-probe" } },
      },
      reservation: {
        formRef: neutralReservation.formRef,
        name: "counter-reservation-probe",
        desired: { target: { apiVersion: neutralSequence.formRef.apiVersion, kind: neutralSequence.formRef.kind, name: "range-sequence-probe" } },
      },
    },
    syntheticSecondGroup: neutralOtherGroup.formRef,
    syntheticSecondDefinitionVersion: neutralSecondDefinition.formRef,
    constraintSemantics: Object.fromEntries(Object.entries(neutralConstraintPackages).map(
      ([label, entry]) => [label, { name: `constraint-${slug(label)}-probe`, formRef: entry.formRef }],
    ).concat([["exclusiveSecond", {
      name: "constraint-exclusive-second-probe",
      formRef: neutralLeaseSecond.formRef,
    }]])),
    artifactTransport: {
      blobSource: neutralArtifactBytes,
      declaredSize: Buffer.byteLength(neutralArtifactBytes),
      contentType: "application/octet-stream",
    },
    support: {
      interface: { name: neutralRelationInterface.name, version: neutralRelationInterface.version },
      binding: { name: neutralRelationBinding.name, version: neutralRelationBinding.version },
    },
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
  console.log(`derived stable Takoform v1 suite: ${familyInputs.length} families, ${allRefs.length} exact Forms, ${genericChecks.length} generic checks`);
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

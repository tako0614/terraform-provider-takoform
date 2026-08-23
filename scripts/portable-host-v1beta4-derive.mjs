#!/usr/bin/env bun

// Derives conformance/portable-host-v1beta4 from the v1beta1 corpus.
//
// The two corpora measure ONE set of lifecycle semantics through the two lanes
// this project serves. Hand-authoring the second would let them drift apart
// silently, and a pair of conformance corpora that disagree cannot tell a host
// which of them it failed — so the v1beta4 corpus is derived, and the only
// hand-authored part is the three deltas below: what each lane actually
// changed.
//
// There are three deltas and one target because two of the lanes between them
// were withdrawn before either was ever served (decision 0051). Their content
// is here; only their names are gone, and folding their deltas into one script
// keeps the derivation a record of what changed rather than of what was named.
//
// The v1beta1 corpus is never written by this script. It is the retained lane,
// Registry-published provider 2.1.1 speaks it, and hosts serve it today.

import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/portable-host-v1beta4-derive.mjs --write|--check");
}

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const sourceRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta1");
const targetRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta4");

const source = JSON.parse(readFileSync(path.join(sourceRoot, "contract.json"), "utf8"));
// ---- delta 1: the Beta 2 hardening review (decisions 0039 and 0046) ----
//
// Every entry here is a rule this lane states and the v1beta1 document did
// not. Nothing else about the corpus moves, which is what makes
// the two comparable: a check that fails on one lane and passes on the other
// is reporting a lane difference rather than a corpus difference.

const contract = structuredClone(source);

// Two codes leave and one status moves. form_identity_conflict was never given
// a trigger; deletion_protected claimed a policy surface no Form provides; and
// form_unavailable is transient backing capacity, which is 503.
const withdrawnCodes = new Set(["form_identity_conflict", "deletion_protected"]);
contract.errorEnvelope.codes = contract.errorEnvelope.codes.filter(
  (code) => !withdrawnCodes.has(code),
);
contract.errorEnvelope.httpStatusByCode = Object.fromEntries(
  Object.entries(contract.errorEnvelope.httpStatusByCode)
    .filter(([code]) => !withdrawnCodes.has(code))
    .map(([code, status]) => [code, code === "form_unavailable" ? 503 : status]),
);

// The class-selecting probes exist only in the family generation this corpus
// installs, so they are added here rather than carried by the source corpus.
// Their identities and pinned desired schemas are READ from the candidate
// tree: hand-written digests here would go stale the moment a Definition
// changed, and a stale pin fails as "the host does not have this Form" — a
// diagnosis pointing at the host for a defect in this script.
const familyTree = "forms/candidates/edge.forms.takoform.com";
const bindingTree = "bindings/candidates/v1alpha2";
const familySet = JSON.parse(
  readFileSync(path.join(repositoryRoot, familyTree, "candidate-set.json"), "utf8"),
);

// The class holders reference the worker they hold a class on, and that
// reference names the family the corpus installs. Reading it from the family
// set rather than a literal is what keeps it from naming a generation the
// corpus stopped installing (decision 0049).
const CLASS_HOLDER_DESIRED = {
  "durableWorkflow": {
    "className": "OrderFulfilment",
    "worker": {
      "apiVersion": familySet.family,
      "kind": "ModuleWorker",
      "name": "module-worker-probe"
    }
  },
  "actorNamespace": {
    "className": "ChatRoom",
    "worker": {
      "apiVersion": familySet.family,
      "kind": "ModuleWorker",
      "name": "module-worker-probe"
    }
  }
};

const CLASS_HOLDER_LIFECYCLE = {
  "durableWorkflow": [
    "create",
    "read",
    "delete",
    "import",
    "observe"
  ],
  "actorNamespace": [
    "create",
    "read",
    "delete",
    "import",
    "observe"
  ]
};

const CLASS_HOLDER_NAMES = {
  "durableWorkflow": "durable-workflow-probe",
  "actorNamespace": "actor-namespace-probe"
};

const CLASS_HOLDER_SLUGS = {
  "durableWorkflow": "durable-workflow",
  "actorNamespace": "actor-namespace"
};

const classHolderFixtures = [];

function classHolderProbe(probeKey, kind) {
  const candidate = familySet.forms.find((form) => form.formRef.kind === kind);
  if (candidate === undefined) {
    throw new Error(`${familyTree} carries no ${kind}`);
  }
  const definition = JSON.parse(
    readFileSync(path.join(repositoryRoot, candidate.path, "definition.json"), "utf8"),
  );
  const fixture = `fixtures/desired-schema-${CLASS_HOLDER_SLUGS[probeKey]}.json`;
  const bytes = `${JSON.stringify(definition.desiredSchema, null, 2)}\n`;
  // Queued, never written here: --check must leave the tree alone, and a
  // derivation that wrote during verification would report itself clean.
  classHolderFixtures.push({ file: path.join(targetRoot, fixture), bytes: Buffer.from(bytes) });
  return {
    name: CLASS_HOLDER_NAMES[probeKey],
    identity: {
      formRef: structuredClone(candidate.formRef),
      packageDigest: candidate.packageDigest,
    },
    lifecycleCapabilities: CLASS_HOLDER_LIFECYCLE[probeKey],
    desired: structuredClone(CLASS_HOLDER_DESIRED[probeKey]),
    desiredSchema: {
      path: fixture,
      sha256: `sha256:${createHash("sha256").update(bytes).digest("hex")}`,
    },
    // These two probes carry the family's class-keyed holds, and they are the
    // ones a corpus pinning only the source generation's probes had no pin
    // for — the exact Forms whose rule the declared-holds check exists to
    // measure. A probe this script mints pins its constraints like any other.
    ...(definition.constraints?.length > 0
      ? { constraints: structuredClone(definition.constraints) }
      : {}),
  };
}

// The sealed external standard-service slot (decision 0045). Its two specs are
// the whole point of the probe: one the host must accept and one it must refuse
// before any mutation, so the vocabulary is closed rather than advisory.
const workerVersionDesired = contract.runnerInput.workerVersion.desired;
contract.runnerInput.externalServices = {
  property: "externalServices",
  serviceApiVersion: "standards.takoform.com/v1alpha1",
  protocols: ["postgresql", "redis", "s3-compatible", "smtp"],
  desiredSpec: {
    ...structuredClone(workerVersionDesired),
    externalServices: [
      {
        name: "PRIMARY_DB",
        service: { apiVersion: "standards.takoform.com/v1alpha1", protocol: "postgresql" },
      },
      {
        name: "MEDIA_STORE",
        service: { apiVersion: "standards.takoform.com/v1alpha1", protocol: "s3-compatible" },
      },
    ],
  },
  unknownProtocolSpec: {
    ...structuredClone(workerVersionDesired),
    externalServices: [
      {
        name: "PRIMARY_DB",
        service: {
          apiVersion: "standards.takoform.com/v1alpha1",
          protocol: "not-a-standard-protocol",
        },
      },
    ],
  },
};

// One check per rule the lane introduced. The list is the corpus's copy of what
// internal/portableconformancev3 requires of a v1beta2 run; the two are compared
// at verify time, so neither can grow a check the other does not know about.
// The lane delta carries the family axis too: this corpus drives the current
// protocol lane against the current family, and inheriting the source corpus's
// tree would silently re-conflate the two axes.
contract.familyCandidateSet = familyTree;
// The Binding envelope is a third axis on the same footing: it moved to
// v1alpha2 when allowedTargetForms had to admit a Form group carrying no
// version, and a corpus states which generation it installs rather than
// letting a loader pick one.
contract.bindingCandidateSet = bindingTree;

// The synthetic second definition belongs to the family this corpus installs,
// so the source corpus's v1beta1-group document is not it. The file is named
// for the family rather than a generation, because the family has none to name
// (decision 0049) and a filename carrying one would drift on the next move.
contract.runnerInput.syntheticSecondDefinitionVersion.path =
  "fixtures/synthetic-module-worker-second-definition-edge-family.json";

// The class-selecting identities exist only in the generation this corpus
// installs. They are added HERE rather than carried by the source corpus,
// because a corpus measures the generation it declares — the v1beta1 family
// has no DurableWorkflow to probe.
contract.runnerInput.durableWorkflow = classHolderProbe("durableWorkflow", "DurableWorkflow");
contract.runnerInput.actorNamespace = classHolderProbe("actorNamespace", "ActorNamespace");

contract.requiredRunnerChecks = [
  ...source.requiredRunnerChecks,
  "fence-matrix-observed",
  "forms-route-enumerates",
  "availability-truth-conditions",
  "cancel-outcomes-closed",
  "external-service-slots-sealed",
  "portable-defaults-materialized",
  // The class-export gate, measurable only where the class-selecting
  // identities exist.
  "class-holder-rules-enforced",
];

// ---- delta 2: cross-resource rules become declared mechanisms (decision 0048) ----
//
// The wire envelope does not move here. What changed is not what a host puts
// on the wire but what it must DO: derive the cross-resource rules from a
// Definition's declarations instead of carrying a table of Form kinds.
//
// One added check. It drives every exclusive hold the installed family
// declares, discovered from the served Definitions — a host that hardcoded one
// rule and forgot another fails on the one it forgot, which is the failure the
// previous lane's per-Form checks could not see.
// Family-derived checks are appended AFTER the lane's own, so the new lane
// check goes before them rather than at the end of the list — the verifier
// compares the list in order, and a corpus that appended blindly would drift
// from the lane the moment a family added a check.
const FAMILY_DERIVED = new Set(["class-holder-rules-enforced"]);
const inheritedLaneChecks = contract.requiredRunnerChecks.filter(
  (check) => !FAMILY_DERIVED.has(check),
);
const inheritedFamilyChecks = contract.requiredRunnerChecks.filter((check) =>
  FAMILY_DERIVED.has(check),
);
contract.requiredRunnerChecks = [
  ...inheritedLaneChecks,
  "declared-exclusive-holds-enforced",
  ...inheritedFamilyChecks,
];


// ---- delta 3: a Form versions alone (decisions 0049 and 0051) ----
//
// The family group stopped carrying a version segment, which changes the
// FormRef grammar every request and response carries and, with it, the shape of
// every path that names a group. The renamed check is the whole of the wire
// difference a corpus can see: the rule it measures is no longer "two path
// segments" but "the group's own segments", and the corpus installs one group
// of each length so the check has both shapes to prove it on.

contract.format = "takoform.portable-host-conformance@v1beta4";
contract.apiVersion = "forms.takoform.com/v1beta4";
contract.discoveryPath = "/.well-known/takoform/v1beta4";
contract.apiPath = "/apis/forms.takoform.com/v1beta4";

contract.requiredRunnerChecks = contract.requiredRunnerChecks.map((check) =>
  check === "namespaced-group-travels-as-two-path-segments"
    ? "namespaced-group-travels-as-path-segments"
    : check,
);

// ---- write or check ----

const contractText = `${JSON.stringify(contract, null, 2)}\n`;
const manifest = {
  format: "takoform.portable-host-conformance-manifest@v1beta4",
  contract: "contract.json",
  sha256: createHash("sha256").update(contractText).digest("hex"),
};
const manifestText = `${JSON.stringify(manifest, null, 2)}\n`;

const fixtureDir = path.join(sourceRoot, "fixtures");
// The source corpus still carries fixture files for probes it no longer
// declares. Copying those would produce a second writer for the same path,
// and two writers of one file is a --check that fails the moment they
// disagree — which is how this was found.
const classHolderFixtureNames = new Set(
  classHolderFixtures.map(({ file }) => path.basename(file)),
);
const fixtures = readdirSync(fixtureDir)
  .filter((name) => !classHolderFixtureNames.has(name))
  .map((name) => ({
    name,
    bytes: readFileSync(path.join(fixtureDir, name)),
  }));

const outputs = [
  { file: path.join(targetRoot, "contract.json"), bytes: Buffer.from(contractText) },
  { file: path.join(targetRoot, "manifest.json"), bytes: Buffer.from(manifestText) },
  ...fixtures.map(({ name, bytes }) => ({
    file: path.join(targetRoot, "fixtures", name),
    bytes,
  })),
  ...classHolderFixtures,
];

if (mode === "--write") {
  mkdirSync(path.join(targetRoot, "fixtures"), { recursive: true });
  for (const { file, bytes } of outputs) writeFileSync(file, bytes);
  console.log(
    `derived portable-host-v1beta4 from v1beta1: ${contract.requiredRunnerChecks.length} required checks, ` +
      `${contract.errorEnvelope.codes.length} error codes, ${fixtures.length} fixtures`,
  );
} else {
  const drift = [];
  for (const { file, bytes } of outputs) {
    let actual;
    try {
      actual = readFileSync(file);
    } catch {
      drift.push(`${path.relative(repositoryRoot, file)}: missing`);
      continue;
    }
    if (!actual.equals(bytes)) drift.push(`${path.relative(repositoryRoot, file)}: drifted`);
  }
  if (drift.length > 0) {
    for (const line of drift) process.stderr.write(`- ${line}\n`);
    throw new Error(
      "portable-host-v1beta4 is stale; run bun scripts/portable-host-v1beta4-derive.mjs --write",
    );
  }
  console.log(
    `portable-host-v1beta4 matches its derivation from v1beta1 (${contract.requiredRunnerChecks.length} checks)`,
  );
}

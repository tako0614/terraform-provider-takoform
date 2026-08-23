#!/usr/bin/env bun

// Re-pins only the source-derived closure of conformance/portable-host-v1beta1:
// exact Form identities/packages, desired-schema fixture bytes, Interface
// digests, and the contract manifest. The behavioral runner input and required
// checks remain hand-authored corpus source and are never regenerated here.

import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/portable-host-v1beta1-pins.mjs --write|--check");
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const corpusRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta1");
const contractPath = path.join(corpusRoot, "contract.json");
const manifestPath = path.join(corpusRoot, "manifest.json");

// The family generation this corpus installs is the CORPUS's statement, not
// this script's. It is a separate axis from the protocol lane (decision 0047),
// and reading it from a literal here is exactly how a Host API v1beta1 corpus
// came to be pinned to the v1beta2 family.
const declaredFamilyTree = readJSON(contractPath).familyCandidateSet;
if (typeof declaredFamilyTree !== "string" || declaredFamilyTree === "") {
  throw new Error("portable-host-v1beta1 contract does not declare familyCandidateSet");
}
const forms = readJSON(
  path.join(repositoryRoot, declaredFamilyTree, "candidate-set.json"),
);
const interfaces = readJSON(
  path.join(repositoryRoot, "interfaces", "candidates", "v1alpha1", "candidate-set.json"),
);
if (
  forms.format !== "takoform.form-family-candidates@v1" ||
  interfaces.format !== "takoform.interface-candidates@v1" ||
  interfaces.publicationStatus !== "unpublished"
) {
  throw new Error("portable-host-v1beta1 pins require a family candidate lane and the interface candidate lane");
}

const formsByFamily = new Map(
  forms.forms.map((candidate) => [
    formFamily(candidate.formRef),
    candidate,
  ]),
);
const formsByKind = new Map(
  forms.forms.map((candidate) => [candidate.formRef.kind, candidate]),
);
if (formsByFamily.size !== forms.forms.length) {
  throw new Error("unpublished Form candidate set contains a duplicate family");
}
if (formsByKind.size !== forms.forms.length) {
  throw new Error("unpublished Form candidate set contains a duplicate kind");
}
const interfacesByIdentity = new Map(
  interfaces.interfaces.map((candidate) => [
    `${candidate.name}@${candidate.version}`,
    candidate,
  ]),
);
if (interfacesByIdentity.size !== interfaces.interfaces.length) {
  throw new Error("unpublished Interface candidate set contains a duplicate identity");
}

const contract = readJSON(contractPath);
const fixtureOutputs = [];
let pinnedForms = 0;
// repointFamilyReferences rewrites every nested apiVersion that names ANOTHER
// generation of the same Form Family group. It leaves other groups alone: the
// corpus deliberately installs a second group, and rewriting that one would
// erase the very thing it exists to prove.
function repointFamilyReferences(node, family) {
  // A family that carries no version segment IS its own group name
  // (decision 0049). Slicing at a separator that is not there would drop the
  // last character and match nothing, so every nested reference would keep
  // naming the generation the corpus just stopped installing.
  const separator = family.lastIndexOf("/");
  const groupName = separator === -1 ? family : family.slice(0, separator);
  const walk = (value) => {
    if (Array.isArray(value)) {
      for (const item of value) walk(item);
      return;
    }
    if (value === null || typeof value !== "object") return;
    for (const [key, member] of Object.entries(value)) {
      if (
        key === "apiVersion" &&
        typeof member === "string" &&
        member.startsWith(`${groupName}/`) &&
        member !== family
      ) {
        value[key] = family;
        continue;
      }
      walk(member);
    }
  };
  walk(node);
}

for (const probe of Object.values(contract.runnerInput)) {
  const ref = probe?.identity?.formRef;
  if (ref === undefined) continue;
  // Matched by KIND, not by the whole family identity: re-pinning is exactly
  // the operation that moves a probe from one family generation to another,
  // and matching on the old apiVersion would make the corpus unable to follow
  // its own declared tree (decision 0047).
  const candidate = formsByKind.get(ref.kind);
  if (candidate === undefined) {
    throw new Error(
      `portable-host-v1beta1 references ${ref.kind}, which the declared family tree ` +
        `${declaredFamilyTree} does not carry`,
    );
  }

  probe.identity.formRef = structuredClone(candidate.formRef);
  probe.identity.packageDigest = candidate.packageDigest;
  // A probe's DESIRED document names other Forms by reference, and those
  // references carry the family group too. Re-pinning the identity without
  // them would leave a spec whose every nested reference names the generation
  // the corpus just stopped installing — which a conforming host refuses,
  // correctly, and which looks like a host bug rather than a corpus one.
  repointFamilyReferences(probe.desired, forms.family);
  pinnedForms++;

  if (probe.desiredSchema === undefined) continue;
  const relativeFixture = probe.desiredSchema.path;
  if (!/^fixtures\/desired-schema-[a-z0-9-]+\.json$/u.test(relativeFixture)) {
    throw new Error(`refusing unexpected desired-schema path ${relativeFixture}`);
  }
  const definitionPath = path.join(repositoryRoot, candidate.path, "definition.json");
  const desiredSchema = readJSON(definitionPath).desiredSchema;
  if (desiredSchema === undefined) {
    throw new Error(`${candidate.kind}: candidate definition has no desiredSchema`);
  }
  const fixturePath = path.join(corpusRoot, relativeFixture);
  const current = readFileSync(fixturePath, "utf8");
  // Preserve the corpus's established formatting convention: large schemas
  // are two-space documents, tiny schemas are compact, and both end in a
  // newline. Only semantic bytes change.
  const expected = current.startsWith("{\n")
    ? `${stringifyASCII(desiredSchema, 2)}\n`
    : `${stringifyASCII(desiredSchema)}\n`;
  fixtureOutputs.push({ path: fixturePath, expected });
  probe.desiredSchema.sha256 = digest(expected);
}
if (pinnedForms === 0) {
  throw new Error(
    "portable-host-v1beta1 does not reference any current Form candidates",
  );
}

const support = contract.runnerInput.supportProbes;
for (const probe of support.dataInterfaces) {
  const candidate = requiredInterface(probe.name, probe.version);
  probe.schemaDigest = candidate.schemaDigest;
}
const runtime = requiredInterface(
  support.runtimeContract.name,
  support.runtimeContract.version,
);
support.runtimeContract.schemaDigest = runtime.schemaDigest;

const contractOutput = `${JSON.stringify(contract, null, 2)}\n`;
const manifest = {
  format: "takoform.portable-host-conformance-manifest@v1beta1",
  contract: "contract.json",
  sha256: digest(contractOutput).slice("sha256:".length),
};
const manifestOutput = `${JSON.stringify(manifest, null, 2)}\n`;
const outputs = [
  ...fixtureOutputs,
  { path: contractPath, expected: contractOutput },
  { path: manifestPath, expected: manifestOutput },
];

if (mode === "--write") {
  for (const output of outputs) writeFileSync(output.path, output.expected);
  process.stdout.write(
    `re-pinned portable-host-v1beta1 from ${pinnedForms} Forms and ${support.dataInterfaces.length + 1} Interfaces\n`,
  );
} else {
  const stale = outputs
    .filter((output) => readFileSync(output.path, "utf8") !== output.expected)
    .map((output) => path.relative(repositoryRoot, output.path));
  if (stale.length > 0) {
    throw new Error(
      `portable-host-v1beta1 source-derived pins are stale: ${stale.join(", ")}; ` +
        "run bun run sync:portable-host-v1beta1",
    );
  }
  process.stdout.write(
    `portable-host-v1beta1 source-derived pins match ${pinnedForms} Forms and ${support.dataInterfaces.length + 1} Interfaces\n`,
  );
}

function readJSON(file) {
  return JSON.parse(readFileSync(file, "utf8"));
}

function formFamily(ref) {
  return `${ref.apiVersion}\u0000${ref.kind}`;
}

function requiredInterface(name, version) {
  const candidate = interfacesByIdentity.get(`${name}@${version}`);
  if (candidate === undefined) {
    throw new Error(`portable-host-v1beta1 references unknown Interface ${name}@${version}`);
  }
  return candidate;
}

function digest(value) {
  return `sha256:${createHash("sha256").update(value).digest("hex")}`;
}

function stringifyASCII(value, indent) {
  return JSON.stringify(value, null, indent).replace(
    /[^\x00-\x7f]/g,
    (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, "0")}`,
  );
}

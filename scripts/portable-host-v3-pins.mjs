#!/usr/bin/env bun

// Re-pins only the source-derived closure of conformance/portable-host-v3:
// exact Form identities/packages, desired-schema fixture bytes, Interface
// digests, and the contract manifest. The behavioral runner input and required
// checks remain hand-authored corpus source and are never regenerated here.

import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/portable-host-v3-pins.mjs --write|--check");
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const corpusRoot = path.join(repositoryRoot, "conformance", "portable-host-v3");
const contractPath = path.join(corpusRoot, "contract.json");
const manifestPath = path.join(corpusRoot, "manifest.json");

const forms = readJSON(
  path.join(repositoryRoot, "forms", "candidates", "edge", "v1alpha1", "candidate-set.json"),
);
const interfaces = readJSON(
  path.join(repositoryRoot, "interfaces", "candidates", "v1alpha1", "candidate-set.json"),
);
if (
  forms.format !== "takoform.form-family-candidates@v1" ||
  forms.publicationStatus !== "unpublished" ||
  interfaces.format !== "takoform.interface-candidates@v1" ||
  interfaces.publicationStatus !== "unpublished"
) {
  throw new Error("portable-host-v3 pins require the unpublished family candidate lane");
}

const formsByFamily = new Map(
  forms.forms.map((candidate) => [
    formFamily(candidate.formRef),
    candidate,
  ]),
);
if (formsByFamily.size !== forms.forms.length) {
  throw new Error("unpublished Form candidate set contains a duplicate family");
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
for (const probe of Object.values(contract.runnerInput)) {
  const ref = probe?.identity?.formRef;
  if (ref === undefined) continue;
  const candidate = formsByFamily.get(formFamily(ref));
  if (candidate === undefined) {
    throw new Error(
      `portable-host-v3 references unknown current Form family ${ref.apiVersion}/${ref.kind}`,
    );
  }

  probe.identity.formRef = structuredClone(candidate.formRef);
  probe.identity.packageDigest = candidate.packageDigest;
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
    "portable-host-v3 does not reference any current Form candidates",
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
  format: "takoform.portable-host-conformance-manifest@v3",
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
    `re-pinned portable-host-v3 from ${pinnedForms} Forms and ${support.dataInterfaces.length + 1} Interfaces\n`,
  );
} else {
  const stale = outputs
    .filter((output) => readFileSync(output.path, "utf8") !== output.expected)
    .map((output) => path.relative(repositoryRoot, output.path));
  if (stale.length > 0) {
    throw new Error(
      `portable-host-v3 source-derived pins are stale: ${stale.join(", ")}; ` +
        "run bun run sync:portable-host-v3",
    );
  }
  process.stdout.write(
    `portable-host-v3 source-derived pins match ${pinnedForms} Forms and ${support.dataInterfaces.length + 1} Interfaces\n`,
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
    throw new Error(`portable-host-v3 references unknown Interface ${name}@${version}`);
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

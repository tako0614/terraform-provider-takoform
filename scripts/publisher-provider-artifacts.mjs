import { createHash } from "node:crypto";
import {
  mkdirSync,
  lstatSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import process from "node:process";

const repositoryRoot = resolve(import.meta.dirname, "..");
const outputRoot = join(repositoryRoot, "internal/provider/artifacts/publisher");
const publisherGroup = "edge.forms.takoform.com";
const mode = process.argv[2] ?? "--check";
const requestedSourceRoot = process.env.TAKOFORM_PROVIDER_ARTIFACT_SOURCE_ROOT;
const sourceRoot = requestedSourceRoot ? resolve(requestedSourceRoot) : outputRoot;

function fail(message) {
  throw new Error(message);
}

function readJSON(path) {
  return JSON.parse(readFileSync(path, "utf8"));
}

function sourcePath(name) {
  if (typeof name !== "string" || name.length === 0 || isAbsolute(name) || name.includes("\\")) {
    fail(`publisher-selected Provider source path is not a canonical relative path: ${JSON.stringify(name)}`);
  }
  const target = resolve(sourceRoot, name);
  if (target === sourceRoot || !target.startsWith(sourceRoot + sep)) {
    fail(`publisher-selected Provider source path escapes the retained closure: ${JSON.stringify(name)}`);
  }
  return target;
}

function refKey(ref) {
  return [ref.apiVersion, ref.kind, ref.definitionVersion, ref.schemaDigest].join("\u0000");
}

function canonicalJSON(value) {
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(",")}]`;
  return `{${Object.keys(value)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonicalJSON(value[key])}`)
    .join(",")}}`;
}

function sha256(value) {
  return `sha256:${createHash("sha256").update(value).digest("hex")}`;
}

function collectFieldInterfaces(fields, names) {
  for (const field of fields ?? []) {
    if (field?.Target?.Interface?.Name) names.add(field.Target.Interface.Name);
    collectFieldInterfaces(field?.Fields, names);
    for (const variant of field?.Variants ?? []) collectFieldInterfaces(variant?.Fields, names);
  }
}

function filesUnder(root) {
  const output = [];
  for (const name of readdirSync(root)) {
    const path = join(root, name);
    const entry = lstatSync(path);
    if (entry.isSymbolicLink()) fail(`publisher-selected Provider artifact source contains a symlink: ${path}`);
    if (entry.isDirectory()) output.push(...filesUnder(path));
    else if (entry.isFile()) output.push(path);
    else fail(`publisher-selected Provider artifact source is not a regular file or directory: ${path}`);
  }
  return output;
}

function expectedArtifacts() {
  const sourceClosure = readJSON(join(sourceRoot, "closure.json"));
  const sourceProjection = readJSON(sourcePath(sourceClosure.projection.path));
  const currentForms = sourceProjection.forms.filter(
    (entry) => entry.generation === "current" && entry.ref.apiVersion === publisherGroup,
  );
  if (currentForms.length !== 17) {
    fail(`publisher-selected current Form count is ${currentForms.length}, want 17`);
  }

  const currentKeys = new Set(currentForms.map((entry) => refKey(entry.ref)));
  const currentResources = sourceProjection.resources
    .filter((entry) => currentKeys.has(refKey(entry.ref)))
    .map((entry, registrationOrder) => ({ ...entry, registrationOrder }));
  const resourceTypes = new Set(currentResources.map((entry) => entry.resourceType));
  const readableKeys = new Set(sourceProjection.readableRefs.map(refKey));
  const retainedResources = sourceProjection.resources.filter(
    (entry) => !entry.register && resourceTypes.has(entry.resourceType) && readableKeys.has(refKey(entry.ref)),
  );
  const retainedKeys = new Set(retainedResources.map((entry) => refKey(entry.ref)));
  const forms = sourceProjection.forms.filter(
    (entry) => currentKeys.has(refKey(entry.ref)) || retainedKeys.has(refKey(entry.ref)),
  );
  const resources = [...currentResources, ...retainedResources];
  const projection = {
    format: "takoform.provider-publisher-set-projection@v1",
    hostApi: sourceProjection.hostApi,
    forms,
    resources,
    defaultCreates: sourceProjection.defaultCreates.filter((entry) => currentKeys.has(refKey(entry))),
    readableRefs: sourceProjection.readableRefs.filter(
      (entry) => currentKeys.has(refKey(entry)) || retainedKeys.has(refKey(entry)),
    ),
  };

  const interfaceNames = new Set();
  const bindingNames = new Set();
  for (const entry of currentForms) {
    for (const ref of entry.form?.ProvidedInterfaces ?? []) interfaceNames.add(ref.Name);
    for (const ref of entry.form?.AcceptedBindings ?? []) bindingNames.add(ref.Name);
    collectFieldInterfaces(entry.form?.Fields, interfaceNames);
  }
  const packages = sourceClosure.packages.filter(
    (entry) => entry.formRef.apiVersion === publisherGroup,
  );
  const interfaces = sourceClosure.interfaces.filter((entry) => interfaceNames.has(entry.ref.name));
  const bindings = sourceClosure.bindings.filter((entry) => bindingNames.has(entry.ref.name));
  if (packages.length !== 17 || interfaces.length !== 8 || bindings.length !== 7) {
    fail(
      `publisher-selected closure is ${packages.length} packages/${interfaces.length} Interfaces/${bindings.length} Bindings, want 17/8/7`,
    );
  }

  const projectionBytes = `${JSON.stringify(projection, null, 2)}\n`;
  const closure = {
    format: "takoform.provider-publisher-set-artifact-closure@v1",
    projection: {
      path: "projection.json",
      digest: sha256(canonicalJSON(projection)),
    },
    packages,
    interfaces,
    bindings,
  };
  const expected = new Map([
    ["closure.json", Buffer.from(`${JSON.stringify(closure, null, 2)}\n`)],
    ["projection.json", Buffer.from(projectionBytes)],
  ]);
  for (const entry of packages) {
    for (const source of filesUnder(sourcePath(entry.root))) {
      expected.set(relative(sourceRoot, source), readFileSync(source));
    }
  }
  for (const entry of [...interfaces, ...bindings]) {
    expected.set(entry.path, readFileSync(sourcePath(entry.path)));
  }
  return expected;
}

function writeArtifacts(expected) {
  rmSync(outputRoot, { recursive: true, force: true });
  for (const [name, bytes] of expected) {
    const target = join(outputRoot, name);
    mkdirSync(dirname(target), { recursive: true });
    writeFileSync(target, bytes, { mode: 0o644 });
  }
}

function checkArtifacts(expected) {
  const actual = new Set(
    statSync(outputRoot, { throwIfNoEntry: false })?.isDirectory()
      ? filesUnder(outputRoot).map((path) => relative(outputRoot, path))
      : [],
  );
  for (const [name, bytes] of expected) {
    if (!actual.delete(name)) fail(`publisher-selected Provider artifact ${name} is missing; run bun run sync:publisher-provider-artifacts`);
    const current = readFileSync(join(outputRoot, name));
    if (!current.equals(bytes)) fail(`publisher-selected Provider artifact ${name} is stale; run bun run sync:publisher-provider-artifacts`);
  }
  if (actual.size !== 0) {
    fail(`publisher-selected Provider artifact closure has extra files: ${[...actual].sort().join(", ")}`);
  }
}

const expected = expectedArtifacts();
if (mode === "--write" && !requestedSourceRoot) {
  fail("--write requires TAKOFORM_PROVIDER_ARTIFACT_SOURCE_ROOT pointing at an exact Provider mapping source closure");
}
if (mode === "--write") writeArtifacts(expected);
else if (mode === "--check") checkArtifacts(expected);
else fail(`usage: bun scripts/publisher-provider-artifacts.mjs [--check|--write]`);

console.log(`publisher-selected Provider artifact closure: ${expected.size} files (${mode.slice(2)})`);

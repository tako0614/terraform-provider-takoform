import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");
const inputPath = "internal/provider/container-forward-input.json";
const sourcePath = "cmd/provider-container-candidate/container-http-candidate.json";
const closurePath = "cmd/provider-container-candidate/closure.json";
const sourceCommit = "35a77bdb37483fccbb365998923824040dad67cc";
const fail = (message) => { throw new Error(`Provider Container forward candidate: ${message}`); };
const digest = (bytes) => `sha256:${createHash("sha256").update(bytes).digest("hex")}`;

function readExact(name) {
  try { return readFileSync(join(root, name)); }
  catch (error) { fail(`read ${name}: ${error.message}`); }
}

function verifyCandidate(value, input) {
  const candidate = input.candidate;
  const normalize = (value) => Array.isArray(value)
    ? value.map(normalize)
    : value && typeof value === "object"
      ? Object.fromEntries(Object.keys(value).sort().map((key) => [key, normalize(value[key])]))
      : value;
  const same = (actual, expected) => JSON.stringify(normalize(actual)) === JSON.stringify(normalize(expected));
  const workerDeploymentDefinition = JSON.parse(value.workerDeployment.definitionJson);
  const vectorDefinition = JSON.parse(value.vector.form.definitionJson);
  const workerVersionTargets = workerDeploymentDefinition.desiredSchema.properties.versions.items.properties.workerVersion["x-takoform-target-formrefs"];
  if (input.format !== "takoform.provider-container-forward-input@v1" || input.publicationStatus !== "unpublished" || input.source.commit !== sourceCommit) {
    fail("candidate input source identity or publication status drifted");
  }
  if (!same(value.formRef, candidate.form) || value.form.kind !== candidate.form.kind ||
      value.interface.name !== candidate.interface.name || value.interface.version !== candidate.interface.version || value.interface.schemaDigest !== candidate.interface.schemaDigest ||
      value.binding.name !== candidate.binding.name || value.binding.version !== candidate.binding.version || value.binding.schemaDigest !== candidate.binding.schemaDigest ||
      value.vector.form.kind !== candidate.vector.form.kind || value.vector.form.definition.definitionVersion !== candidate.vector.form.definitionVersion ||
      !same(vectorDefinition.immutableFields, candidate.vector.form.immutableFields) ||
      value.vector.interface.name !== candidate.vector.interface.name || value.vector.interface.version !== candidate.vector.interface.version || value.vector.interface.schemaDigest !== candidate.vector.interface.schemaDigest ||
      value.vector.binding.name !== candidate.vector.binding.name || value.vector.binding.version !== candidate.vector.binding.version || value.vector.binding.schemaDigest !== candidate.vector.binding.schemaDigest ||
      value.workerVersion.kind !== candidate.workerVersion.kind || value.workerVersion.definition.definitionVersion !== candidate.workerVersion.definitionVersion ||
      value.workerVersion.definition.acceptedBindings.length !== candidate.workerVersion.bindingCount ||
      !same(value.workerVersion.definition.acceptedBindings.map((binding) => binding.name), candidate.workerVersion.bindingNames) ||
      !Object.hasOwn(value.workerVersion.definition.desiredSchema.properties, candidate.workerVersion.typedField) ||
      value.workerDeployment.kind !== candidate.workerDeployment.kind || value.workerDeployment.definition.definitionVersion !== candidate.workerDeployment.definitionVersion ||
      !same(workerVersionTargets, [candidate.workerDeploymentWorkerVersionTarget])) {
    fail("Go Forms CLI output differs from the exact Container/Vector/Worker candidate input");
  }
}

function renderFromSource(sourceBytes, inputBytes) {
  const input = JSON.parse(inputBytes.toString("utf8"));
  const value = JSON.parse(sourceBytes.toString("utf8"));
  verifyCandidate(value, input);
  const closure = {
    format: "takoform.provider-container-forward-artifact-closure@v1",
    publicationStatus: "unpublished",
    source: { path: "container-http-candidate.json", commit: sourceCommit, digest: digest(sourceBytes) },
    input: { path: "../../internal/provider/container-forward-input.json", digest: digest(inputBytes) },
  };
  return { sourceBytes, closureBytes: Buffer.from(`${JSON.stringify(closure, null, 2)}\n`) };
}

function sourceBytesFromCheckout() {
  const publisherRoot = process.env.TAKOFORM_FORMS_SOURCE_ROOT;
  if (!publisherRoot) fail("set TAKOFORM_FORMS_SOURCE_ROOT to the exact local takoform-forms checkout");
  const commit = execFileSync("git", ["-C", publisherRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
  if (commit !== sourceCommit) fail(`Forms checkout is ${commit}, want ${sourceCommit}`);
  execFileSync("git", ["-C", publisherRoot, "diff", "--quiet", "HEAD", "--", "cmd/container-http-candidate", "internal/edgeformcatalog"], { stdio: "inherit" });
  return execFileSync("go", ["run", "./cmd/container-http-candidate"], { cwd: publisherRoot, maxBuffer: 32 * 1024 * 1024 });
}

function main() {
  const mode = process.argv[2];
  if (mode !== "--check" && mode !== "--write") fail("usage: bun scripts/provider-container-forward-mapping.mjs --check | --write");
  const inputBytes = readExact(inputPath);
  if (mode === "--write") {
    const { sourceBytes, closureBytes } = renderFromSource(sourceBytesFromCheckout(), inputBytes);
    for (const [name, bytes] of [[sourcePath, sourceBytes], [closurePath, closureBytes]]) {
      try {
        if (!readFileSync(join(root, name)).equals(bytes)) fail(`${name} already exists with different bytes; refusing overwrite`);
      } catch (error) {
        if (error.code !== "ENOENT") throw error;
        mkdirSync(dirname(join(root, name)), { recursive: true });
        writeFileSync(join(root, name), bytes, { flag: "wx" });
      }
    }
  } else {
    const sourceBytes = readExact(sourcePath);
    const { closureBytes } = renderFromSource(sourceBytes, inputBytes);
    if (!readExact(closurePath).equals(closureBytes)) fail(`${closurePath} digest closure is stale`);
  }
  console.log(`unpublished Container/Vector typed-Provider input: ${mode}`);
}

if (import.meta.main) main();

export { renderFromSource, sourceCommit };

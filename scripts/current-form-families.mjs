#!/usr/bin/env bun

// Deterministic builder for the Form Family candidate lane (spec decisions
// 0008..0013): Edge Platform Family Form Packages (package-index v1alpha4,
// form-definition v1alpha3), the exact Interface and Binding candidate sets,
// and the provider v3 registry. Mirrors scripts/current-form-candidates.mjs:
// stage, verify, then install atomically; --check regenerates into a
// temporary tree and compares.

import { createHash } from "node:crypto";
import {
  existsSync,
  lstatSync,
  mkdtempSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const FAMILY = "edge.forms.takoform.com/v1alpha1";
const PACKAGE_API_VERSION = "packages.forms.takoform.com/v1alpha4";
const trackedTargets = {
  forms: path.join(repositoryRoot, "forms", "candidates", "edge", "v1alpha1"),
  interfaces: path.join(repositoryRoot, "interfaces", "candidates", "v1alpha1"),
  bindings: path.join(repositoryRoot, "bindings", "candidates", "v1alpha1"),
};
const registryPath = path.join(
  repositoryRoot,
  "internal",
  "currentformregistry",
  "registry_v3_generated.go",
);
const forms = [
  ["ModuleWorker", "module-worker", "identity"],
  ["WorkerBundle", "worker-bundle", "revision"],
  ["StaticAssetBundle", "static-asset-bundle", "revision"],
  ["WorkerVersion", "worker-version", "revision"],
  ["WorkerDeployment", "worker-deployment", "deployment"],
  ["WorkerCustomDomain", "worker-custom-domain", "attachment"],
  ["WorkerEndpoint", "worker-endpoint", "attachment"],
  ["WorkerCronTrigger", "worker-cron-trigger", "attachment"],
  ["EdgeKVNamespace", "edge-kv-namespace", "identity"],
  ["ObjectBucket", "object-bucket", "identity"],
  ["SQLiteDatabase", "sqlite-database", "identity"],
  ["SQLiteMigrationSet", "sqlite-migration-set", "revision"],
  ["SQLiteMigrationApplication", "sqlite-migration-application", "attachment"],
  ["AtLeastOnceQueue", "at-least-once-queue", "identity"],
  ["QueueConsumer", "queue-consumer", "attachment"],
];
// The interface lane order MUST match edgeformcatalog.InterfaceDefinitions().
// worker.runtime is the exact ES Module Worker runtime ABI the ModuleWorker
// identity provides (spec/decisions/0019).
const interfaceNames = [
  "edge.kv",
  "edge.objects",
  "edge.sql",
  "edge.queue",
  "worker.service",
  "worker.runtime",
];
const bindingNames = [
  "module-worker.edge-kv",
  "module-worker.object-bucket",
  "module-worker.sqlite",
  "module-worker.queue-producer",
  "module-worker.service",
];

if (import.meta.main) {
  main();
}

function main() {
  const mode = process.argv[2];
  if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
    throw new Error("usage: bun scripts/current-form-families.mjs --write|--check");
  }

  if (mode === "--write") {
    for (const target of Object.values(trackedTargets)) {
      assertSafeGeneratedTarget(target);
    }
    const stagingParent = mkdtempSync(
      path.join(repositoryRoot, ".form-families-build-"),
    );
    try {
      const stagedRoots = {
        forms: path.join(stagingParent, "forms-v1alpha1"),
        interfaces: path.join(stagingParent, "interfaces-v1alpha1"),
        bindings: path.join(stagingParent, "bindings-v1alpha1"),
      };
      const stagedRegistryPath = path.join(stagingParent, "registry_v3_generated.go");
      const manifest = generate(stagedRoots);
      verifyPackages(stagedRoots.forms);
      writeFileSync(stagedRegistryPath, renderRegistry(manifest));
      installGeneratedOutputs(stagingParent, [
        [stagedRoots.forms, trackedTargets.forms],
        [stagedRoots.interfaces, trackedTargets.interfaces],
        [stagedRoots.bindings, trackedTargets.bindings],
        [stagedRegistryPath, registryPath],
      ]);
      process.stdout.write(
        `wrote ${forms.length} Edge Platform Family Form candidates, six interface candidates, five binding candidates, and the provider v3 registry\n`,
      );
    } finally {
      rmSync(stagingParent, { recursive: true, force: true });
    }
    return;
  }

  const temporary = mkdtempSync(path.join(tmpdir(), "takoform-form-families-"));
  try {
    const generatedRoots = {
      forms: path.join(temporary, "forms-v1alpha1"),
      interfaces: path.join(temporary, "interfaces-v1alpha1"),
      bindings: path.join(temporary, "bindings-v1alpha1"),
    };
    const manifest = generate(generatedRoots);
    for (const [key, generatedRoot] of Object.entries(generatedRoots)) {
      compareTrees(generatedRoot, trackedTargets[key], key);
    }
    verifyPackages(trackedTargets.forms);
    compareFile(renderRegistry(manifest), registryPath, "provider v3 registry");
    process.stdout.write("Form Family candidates are reproducible and valid\n");
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}

function generate(outputRoots) {
  for (const outputRoot of Object.values(outputRoots)) {
    mkdirSync(outputRoot, { recursive: true });
  }
  const source = renderFamilySources();
  const manifest = {
    format: "takoform.form-family-candidates@v1",
    family: FAMILY,
    packageApiVersion: PACKAGE_API_VERSION,
    publicationStatus: "unpublished",
    authoringSource: "internal/edgeformcatalog",
    authoringPolicy: "service-shape-preserving-contract",
    forms: [],
  };

  for (const [formIndex, [kind, slug, role]] of forms.entries()) {
    const rendered = source.forms[formIndex];
    if (rendered?.kind !== kind || rendered?.slug !== slug || rendered?.role !== role) {
      throw new Error(`${slug}: family catalog order or identity drifted`);
    }
    const definition = rendered.definition;
    if (
      definition.kind !== kind ||
      definition.apiVersion !== FAMILY ||
      definition.definitionVersion !== "0.1.0" ||
      definition.role !== role
    ) {
      throw new Error(`${slug}: family catalog emitted an invalid candidate identity`);
    }
    if ((definition.negativeConformanceFixtures ?? []).length === 0) {
      throw new Error(`${slug}: every family candidate must carry a negative fixture`);
    }
    if (Object.hasOwn(definition.desiredSchema?.properties ?? {}, "name")) {
      throw new Error(`${slug}: v1alpha3 desired schemas must not declare a name property`);
    }

    const destinationRoot = path.join(outputRoots.forms, slug);
    mkdirSync(path.join(destinationRoot, "fixtures"), { recursive: true });
    const fixtureNames = Object.keys(rendered.fixtures).sort();
    for (const fixtureName of fixtureNames) {
      writeJson(
        path.join(destinationRoot, "fixtures", fixtureName),
        rendered.fixtures[fixtureName],
      );
    }

    // Use the verbatim JSON text emitted by the Go source renderer. Re-
    // serializing through JavaScript would round large integer schema values
    // to the nearest float64 and silently change the normative bytes.
    const definitionRaw = rendered.definitionJson;
    if (typeof definitionRaw !== "string" || definitionRaw.length === 0) {
      throw new Error(`${slug}: source renderer emitted no definitionJson text`);
    }
    writeFileSync(path.join(destinationRoot, "definition.json"), definitionRaw);
    const payloadPaths = [
      "definition.json",
      ...fixtureNames.map((name) => `fixtures/${name}`),
    ].sort();
    const files = payloadPaths.map((relative) => {
      const raw = readFileSync(path.join(destinationRoot, relative));
      return {
        path: relative,
        mediaType:
          relative === "definition.json"
            ? "application/vnd.takoform.form-definition.v1+json"
            : "application/json",
        size: raw.length,
        digest: digest(raw),
      };
    });
    const formRef = {
      apiVersion: FAMILY,
      kind,
      definitionVersion: "0.1.0",
      schemaDigest: digestCanonicalJSON(
        path.join(destinationRoot, "definition.json"),
      ),
    };
    const index = {
      apiVersion: PACKAGE_API_VERSION,
      kind: "FormPackage",
      formRef,
      definitionPath: "definition.json",
      files,
    };
    const indexPath = path.join(destinationRoot, "package-index.json");
    writeJson(indexPath, index);
    manifest.forms.push({
      kind,
      role,
      path: `forms/candidates/edge/v1alpha1/${slug}`,
      formRef,
      packageDigest: digestCanonicalJSON(indexPath),
    });
  }
  writeJson(path.join(outputRoots.forms, "candidate-set.json"), manifest);

  writeContractCandidates({
    outputRoot: outputRoots.interfaces,
    contracts: source.interfaces,
    expectedNames: interfaceNames,
    candidateSet: {
      format: "takoform.interface-candidates@v1",
      publicationStatus: "unpublished",
      authoringSource: "internal/edgeformcatalog",
      interfaces: [],
    },
    listKey: "interfaces",
  });
  writeContractCandidates({
    outputRoot: outputRoots.bindings,
    contracts: source.bindings,
    expectedNames: bindingNames,
    candidateSet: {
      format: "takoform.binding-candidates@v1",
      publicationStatus: "unpublished",
      authoringSource: "internal/edgeformcatalog",
      bindings: [],
    },
    listKey: "bindings",
  });
  return manifest;
}

function writeContractCandidates({ outputRoot, contracts, expectedNames, candidateSet, listKey }) {
  for (const [index, name] of expectedNames.entries()) {
    const contract = contracts[index];
    if (contract?.name !== name || contract?.version !== "1.0.0") {
      throw new Error(`${listKey} catalog order or identity drifted at ${name}`);
    }
    const definitionRaw = contract.definitionJson;
    if (typeof definitionRaw !== "string" || definitionRaw.length === 0) {
      throw new Error(`${name}: source renderer emitted no definitionJson text`);
    }
    const destination = path.join(outputRoot, name, "definition.json");
    mkdirSync(path.dirname(destination), { recursive: true });
    writeFileSync(destination, definitionRaw);
    const schemaDigest = digestCanonicalJSON(destination);
    if (schemaDigest !== contract.schemaDigest) {
      throw new Error(`${name}: rendered schemaDigest drifted from the catalog`);
    }
    candidateSet[listKey].push({
      name,
      version: contract.version,
      schemaDigest,
    });
  }
  writeJson(path.join(outputRoot, "candidate-set.json"), candidateSet);
}

function renderFamilySources() {
  const result = spawnSync("go", ["run", "./cmd/edge-form-source"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.status !== 0) {
    throw new Error(`family Form source rendering failed\n${result.stderr}${result.stdout}`);
  }
  const rendered = JSON.parse(result.stdout);
  if (
    rendered?.family !== FAMILY ||
    rendered?.forms?.length !== forms.length ||
    rendered?.interfaces?.length !== interfaceNames.length ||
    rendered?.bindings?.length !== bindingNames.length
  ) {
    throw new Error("family Form source emitted an unexpected document shape");
  }
  return rendered;
}

function verifyPackages(root) {
  for (const [, slug] of forms) {
    const result = spawnSync(
      "go",
      ["run", "./cmd/form-package", "verify", path.join(root, slug)],
      { cwd: repositoryRoot, encoding: "utf8" },
    );
    if (result.status !== 0) {
      throw new Error(`${slug}: package verification failed\n${result.stderr}${result.stdout}`);
    }
  }
}

function compareTrees(expectedRoot, actualRoot, label) {
  if (!existsSync(actualRoot)) {
    throw new Error(`tracked ${label} candidate set is missing; run bun run build:current-form-families`);
  }
  const expected = inventory(expectedRoot);
  const actual = inventory(actualRoot);
  if (canonicalJson(expected) !== canonicalJson(actual)) {
    throw new Error(
      `tracked ${label} candidate set is stale; run bun run build:current-form-families`,
    );
  }
}

// Installs every staged output atomically: each existing target is parked
// inside the staging parent first, so any failure restores the previous
// tree instead of leaving a partial install.
function installGeneratedOutputs(stagingParent, pairs) {
  const installed = [];
  const parked = [];
  try {
    for (const [index, [, target]] of pairs.entries()) {
      if (existsSync(target)) {
        const parking = path.join(stagingParent, `previous-${index}`);
        renameSync(target, parking);
        parked.push([parking, target]);
      }
    }
    for (const [staged, target] of pairs) {
      mkdirSync(path.dirname(target), { recursive: true });
      renameSync(staged, target);
      installed.push(target);
    }
  } catch (error) {
    for (const target of installed) {
      rmSync(target, { recursive: true, force: true });
    }
    for (const [parking, target] of parked) {
      if (existsSync(parking)) {
        renameSync(parking, target);
      }
    }
    throw error;
  }
  for (const [parking] of parked) {
    rmSync(parking, { recursive: true, force: true });
  }
}

function compareFile(expected, actualPath, label) {
  if (!existsSync(actualPath) || readFileSync(actualPath, "utf8") !== expected) {
    throw new Error(`tracked ${label} is stale; run bun run build:current-form-families`);
  }
}

// renderRegistry emits the two independent facts the provider dispatches on:
// the exact identities this build supports (keyed by the WHOLE ExactFormKey, so
// two definition versions of one kind coexist) and the one create default per
// group+kind. Today each group+kind has exactly one supported identity, so both
// maps carry the same Form entries; the shape is what lets that stop being
// true without a state migration
// (spec/decisions/0017-provider-state-survives-form-evolution-and-interruption.md).
function renderRegistry(manifest) {
  const exactKey = (entry) =>
    `{APIVersion: ${JSON.stringify(entry.formRef.apiVersion)}, ` +
    `Kind: ${JSON.stringify(entry.kind)}, ` +
    `DefinitionVersion: ${JSON.stringify(entry.formRef.definitionVersion)}, ` +
    `SchemaDigest: ${JSON.stringify(entry.formRef.schemaDigest)}}`;
  const defaults = manifest.forms
    .map(
      (entry) =>
        `\t{APIVersion: ${JSON.stringify(entry.formRef.apiVersion)}, ` +
        `Kind: ${JSON.stringify(entry.kind)}}: ${exactKey(entry)},`,
    )
    .join("\n");
  const supported = manifest.forms
    .map(
      (entry) =>
        `\t${exactKey(entry)}: {` +
        `APIVersion: ${JSON.stringify(entry.formRef.apiVersion)}, ` +
        `Kind: ${JSON.stringify(entry.kind)}, ` +
        `DefinitionVersion: ${JSON.stringify(entry.formRef.definitionVersion)}, ` +
        `SchemaDigest: ${JSON.stringify(entry.formRef.schemaDigest)}, ` +
        `PackageDigest: ${JSON.stringify(entry.packageDigest)}},`,
    )
    .join("\n");
  const source =
    `// Code generated by scripts/current-form-families.mjs; DO NOT EDIT.\n\n` +
    `package currentformregistry\n\n` +
    `// v3DefaultCreates names the one exact identity a NEW resource of each\n` +
    `// group+kind is created under.\n` +
    `var v3DefaultCreates = map[GroupKind]ExactFormKey{\n${defaults}\n}\n\n` +
    `// v3Supported is every exact identity this build can read, observe, update,\n` +
    `// and delete, keyed by the whole contract identity.\n` +
    `var v3Supported = map[ExactFormKey]V3Ref{\n${supported}\n}\n`;
  const formatted = spawnSync("gofmt", [], {
    cwd: repositoryRoot,
    encoding: "utf8",
    input: source,
  });
  if (formatted.status !== 0) {
    throw new Error(`generated provider v3 registry formatting failed\n${formatted.stderr}`);
  }
  return formatted.stdout;
}

function inventory(root) {
  const entries = [];
  const visit = (directory, prefix = "") => {
    for (const name of readdirSync(directory).sort()) {
      const full = path.join(directory, name);
      const relative = prefix ? `${prefix}/${name}` : name;
      const info = lstatSync(full);
      if (info.isSymbolicLink()) throw new Error(`${relative}: symlink is forbidden`);
      if (info.isDirectory()) visit(full, relative);
      else if (info.isFile()) entries.push([relative, digest(readFileSync(full))]);
      else throw new Error(`${relative}: unsupported filesystem entry`);
    }
  };
  visit(root);
  return entries;
}

function assertSafeGeneratedTarget(target) {
  const expected = Object.values(trackedTargets);
  if (!expected.includes(target) || path.dirname(target) === repositoryRoot) {
    throw new Error(`refusing unsafe generated target ${target}`);
  }
}

function writeJson(file, value) {
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function digest(bytes) {
  return `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
}

function digestCanonicalJSON(file) {
  const result = spawnSync("go", ["run", "./cmd/form-package", "digest", file], {
    cwd: repositoryRoot,
    encoding: "utf8",
  });
  const value = result.stdout.trim();
  if (result.status !== 0 || !/^sha256:[0-9a-f]{64}$/u.test(value)) {
    throw new Error(
      `RFC 8785 digest failed for ${path.relative(repositoryRoot, file)}\n${result.stderr}${result.stdout}`,
    );
  }
  return value;
}

function canonicalJson(value) {
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(",")}]`;
  return `{${Object.keys(value)
    .sort()
    .map((key) => `${JSON.stringify(key)}:${canonicalJson(value[key])}`)
    .join(",")}}`;
}

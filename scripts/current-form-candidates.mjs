#!/usr/bin/env bun

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
const trackedRoot = path.join(repositoryRoot, "forms", "candidates", "v1alpha2");
const registryPath = path.join(
  repositoryRoot,
  "internal",
  "currentformregistry",
  "registry_generated.go",
);
const forms = [
  ["EdgeWorker", "edge-worker", "cloud-edge-worker"],
  ["RelationalDatabase", "relational-database", "cloud-relational-database"],
  ["ObjectBucket", "object-bucket", "cloud-object-bucket"],
  ["KeyValueStore", "key-value-store", "cloud-key-value-store"],
  ["Queue", "queue", "cloud-queue"],
  ["Schedule", "schedule", "cloud-schedule"],
  ["ContainerService", "container-service", "cloud-container-service"],
  ["StatefulEntity", "stateful-entity", "cloud-stateful-entity"],
  ["VectorIndex", "vector-index", "cloud-vector-index"],
];

if (import.meta.main) {
  main();
}

function main() {
  const mode = process.argv[2];
  if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
    throw new Error("usage: bun scripts/current-form-candidates.mjs --write|--check");
  }

  if (mode === "--write") {
    assertSafeGeneratedTarget(trackedRoot);
    const stagingParent = mkdtempSync(
      path.join(path.dirname(trackedRoot), ".v1alpha2-build-"),
    );
    try {
      const stagedRoot = path.join(stagingParent, "v1alpha2");
      const stagedRegistryPath = path.join(stagingParent, "registry_generated.go");
      const manifest = generate(stagedRoot);
      verifyPackages(stagedRoot);
      writeFileSync(stagedRegistryPath, renderRegistry(manifest));
      installGeneratedOutputs({
        stagedRoot,
        targetRoot: trackedRoot,
        stagedRegistryPath,
        targetRegistryPath: registryPath,
        stagingParent,
      });
      process.stdout.write(
        "wrote nine v1alpha2 Form publication candidates and provider registry\n",
      );
    } finally {
      rmSync(stagingParent, { recursive: true, force: true });
    }
    return;
  }

  const temporary = mkdtempSync(path.join(tmpdir(), "takoform-current-candidates-"));
  try {
    const generated = path.join(temporary, "v1alpha2");
    const manifest = generate(generated);
    compareTrees(generated, trackedRoot);
    verifyPackages(trackedRoot);
    compareFile(renderRegistry(manifest), registryPath, "provider registry");
    process.stdout.write("current Form candidates are reproducible and valid\n");
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}

function generate(outputRoot) {
  mkdirSync(outputRoot, { recursive: true });
  const sourceForms = renderCurrentSources();
  const lifecycle = JSON.parse(
    readFileSync(path.join(repositoryRoot, "forms", "lifecycle.json"), "utf8"),
  );
  const proposals = new Map(lifecycle.proposals.map((proposal) => [proposal.id, proposal]));
  if (proposals.size !== forms.length || lifecycle.currentForms.length !== 0) {
    throw new Error(
      "current candidates must correspond exactly to the nine Proposal records before an Experimental lifecycle transition",
    );
  }
  const manifest = {
    format: "takoform.current-form-candidates@v2",
    formApiVersion: "forms.takoform.com/v1alpha2",
    packageApiVersion: "packages.forms.takoform.com/v1alpha3",
    publicationStatus: "unpublished",
    lifecycleAuthority: "forms/lifecycle.json",
    authoringSource: "internal/currentformcatalog",
    authoringPolicy: "independent-semantic-contract",
    forms: [],
  };

  for (const [formIndex, [kind, slug, proposalId]] of forms.entries()) {
    const source = sourceForms[formIndex];
    if (
      source?.kind !== kind ||
      source?.slug !== slug ||
      source?.proposalId !== proposalId
    ) {
      throw new Error(`${slug}: current catalog order or identity drifted`);
    }
    const proposal = proposals.get(proposalId);
    if (proposal?.document !== `proposals/${slug}.md`) {
      throw new Error(`${slug}: candidate is not bound to its canonical Proposal document`);
    }
    const destinationRoot = path.join(outputRoot, slug);
    const definition = source.definition;
    if (
      definition.kind !== kind ||
      definition.apiVersion !== "forms.takoform.com/v1alpha2" ||
      definition.definitionVersion !== "0.1.0" ||
      Object.hasOwn(definition, "status")
    ) {
      throw new Error(`${slug}: current catalog emitted an invalid candidate identity`);
    }
    const desiredFields = Object.keys(definition.desiredSchema?.properties ?? {}).sort();
    const reviewedFields = [...proposal.portableFields].sort();
    if (canonicalJson(desiredFields) !== canonicalJson(reviewedFields)) {
      throw new Error(
        `${slug}: candidate desired fields ${desiredFields.join(", ")} differ from Proposal review ${reviewedFields.join(", ")}`,
      );
    }

    mkdirSync(path.join(destinationRoot, "fixtures"), { recursive: true });
    const fixtureNames = Object.keys(source.fixtures).sort();
    for (const fixtureName of fixtureNames) {
      const destination = path.join(destinationRoot, "fixtures", fixtureName);
      writeJson(destination, source.fixtures[fixtureName]);
    }

    const definitionRaw = prettyJson(definition);
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
      apiVersion: "forms.takoform.com/v1alpha2",
      kind,
      definitionVersion: "0.1.0",
      schemaDigest: digestCanonicalJSON(
        path.join(destinationRoot, "definition.json"),
      ),
    };
    const index = {
      apiVersion: "packages.forms.takoform.com/v1alpha3",
      kind: "FormPackage",
      formRef,
      definitionPath: "definition.json",
      files,
    };
    const indexPath = path.join(destinationRoot, "package-index.json");
    writeJson(indexPath, index);
    manifest.forms.push({
      kind,
      proposalId,
      path: `forms/candidates/v1alpha2/${slug}`,
      formRef,
      packageDigest: digestCanonicalJSON(indexPath),
    });
  }
  writeJson(path.join(outputRoot, "candidate-set.json"), manifest);
  return manifest;
}

function renderCurrentSources() {
  const result = spawnSync("go", ["run", "./cmd/current-form-source"], {
    cwd: repositoryRoot,
    encoding: "utf8",
  });
  if (result.status !== 0) {
    throw new Error(`current Form source rendering failed\n${result.stderr}${result.stdout}`);
  }
  const rendered = JSON.parse(result.stdout);
  if (!Array.isArray(rendered) || rendered.length !== forms.length) {
    throw new Error(`current Form source emitted ${rendered?.length ?? "invalid"} entries`);
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

function compareTrees(expectedRoot, actualRoot) {
  if (!existsSync(actualRoot)) throw new Error("tracked current candidate set is missing");
  const expected = inventory(expectedRoot);
  const actual = inventory(actualRoot);
  if (canonicalJson(expected) !== canonicalJson(actual)) {
    throw new Error("tracked current candidate set is stale; run bun run build:current-form-candidates");
  }
}

export function installGeneratedOutputs({
  stagedRoot,
  targetRoot,
  stagedRegistryPath,
  targetRegistryPath,
  stagingParent,
}) {
  const previousRoot = path.join(stagingParent, "previous-v1alpha2");
  const previousRegistryPath = path.join(stagingParent, "previous-registry_generated.go");
  let retainedPreviousRoot = false;
  let retainedPreviousRegistry = false;
  let installedRoot = false;
  let installedRegistry = false;
  try {
    if (existsSync(targetRoot)) {
      renameSync(targetRoot, previousRoot);
      retainedPreviousRoot = true;
    }
    if (existsSync(targetRegistryPath)) {
      renameSync(targetRegistryPath, previousRegistryPath);
      retainedPreviousRegistry = true;
    }
    renameSync(stagedRoot, targetRoot);
    installedRoot = true;
    renameSync(stagedRegistryPath, targetRegistryPath);
    installedRegistry = true;
  } catch (error) {
    if (installedRoot && existsSync(targetRoot)) {
      rmSync(targetRoot, { recursive: true, force: true });
    }
    if (installedRegistry && existsSync(targetRegistryPath)) {
      rmSync(targetRegistryPath, { force: true });
    }
    if (retainedPreviousRoot && existsSync(previousRoot)) {
      renameSync(previousRoot, targetRoot);
    }
    if (retainedPreviousRegistry && existsSync(previousRegistryPath)) {
      renameSync(previousRegistryPath, targetRegistryPath);
    }
    throw error;
  }
  if (retainedPreviousRoot) {
    rmSync(previousRoot, { recursive: true, force: true });
  }
  if (retainedPreviousRegistry) {
    rmSync(previousRegistryPath, { force: true });
  }
}

function compareFile(expected, actualPath, label) {
  if (!existsSync(actualPath) || readFileSync(actualPath, "utf8") !== expected) {
    throw new Error(`tracked ${label} is stale; run bun run build:current-form-candidates`);
  }
}

function renderRegistry(manifest) {
  const entries = manifest.forms
    .map(
      (entry) =>
        `\t${JSON.stringify(entry.kind)}: ref(${JSON.stringify(entry.kind)}, ${JSON.stringify(entry.formRef.schemaDigest)}, ${JSON.stringify(entry.packageDigest)}),`,
    )
    .join("\n");
  const source = `// Code generated by scripts/current-form-candidates.mjs; DO NOT EDIT.\n\npackage currentformregistry\n\nvar refs = map[string]Ref{\n${entries}\n}\n`;
  const formatted = spawnSync("gofmt", [], {
    cwd: repositoryRoot,
    encoding: "utf8",
    input: source,
  });
  if (formatted.status !== 0) {
    throw new Error(`generated provider registry formatting failed\n${formatted.stderr}`);
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
  const expected = path.join(repositoryRoot, "forms", "candidates", "v1alpha2");
  if (target !== expected || path.dirname(target) === repositoryRoot) {
    throw new Error(`refusing unsafe generated target ${target}`);
  }
}

function writeJson(file, value) {
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, prettyJson(value));
}

function prettyJson(value) {
  return `${JSON.stringify(value, null, 2)}\n`;
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

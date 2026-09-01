#!/usr/bin/env bun

// Deterministic builder for every current versionless Form Family, the global
// Interface and Binding candidate sets, and the closed current-family index.
// The Go source renders the publisher composition; this writer stages,
// verifies, then installs all outputs atomically. --check regenerates into a
// temporary tree and compares exact bytes.

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
const FAMILY_INDEX_PATH = "forms/candidates/current-family-index.json";
const INTERFACE_CANDIDATE_SET_PATH =
  "interfaces/candidates/v1alpha1/candidate-set.json";
const BINDING_CANDIDATE_SET_PATH =
  "bindings/candidates/v1alpha2/candidate-set.json";
const DNS_LABEL = "[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?";
const PUBLISHER_FAMILY_GROUP = new RegExp(
  `^(?:${DNS_LABEL}\\.)+forms\\.takoform\\.com$`,
  "u",
);
const FORM_KIND = /^[A-Z][A-Za-z0-9]{0,63}$/u;
const FORM_SLUG = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/u;
const CONTRACT_NAME = /^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$/u;
const CONTRACT_VERSION = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u;
const FIXTURE_NAME = /^(?:desired|negative-[a-z0-9]+(?:-[a-z0-9]+)*)\.json$/u;

if (import.meta.main) {
  main();
}

function main() {
  const mode = process.argv[2];
  if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
    throw new Error("usage: bun scripts/current-form-families.mjs --write|--check");
  }

  const source = renderFamilySources();
  const trackedTargets = createTrackedTargets(source);

  if (mode === "--write") {
    for (const target of [
      ...trackedTargets.forms.values(),
      trackedTargets.familyIndex,
      trackedTargets.interfaces,
      trackedTargets.bindings,
    ]) {
      assertSafeGeneratedTarget(target, trackedTargets);
    }
    const stagingParent = mkdtempSync(
      path.join(repositoryRoot, ".form-families-build-"),
    );
    try {
      const stagedRoots = createOutputRoots(stagingParent, source);
      const generation = generate(stagedRoots, source);
      verifyPackages(stagedRoots.forms, source);
      installGeneratedOutputs(stagingParent, [
        ...source.families.map(({ group }) => [
          stagedRoots.forms.get(group),
          trackedTargets.forms.get(group),
        ]),
        [stagedRoots.familyIndex, trackedTargets.familyIndex],
        [stagedRoots.interfaces, trackedTargets.interfaces],
        [stagedRoots.bindings, trackedTargets.bindings],
      ]);
      const formCount = generation.forms.length;
      process.stdout.write(
        `wrote ${formCount} Forms in ${source.families.length} versionless families, ${source.interfaces.length} interface candidates, ${source.bindings.length} binding candidates, and the current-family index\n`,
      );
    } finally {
      rmSync(stagingParent, { recursive: true, force: true });
    }
    return;
  }

  const temporary = mkdtempSync(path.join(tmpdir(), "takoform-form-families-"));
  try {
    const generatedRoots = createOutputRoots(temporary, source);
    generate(generatedRoots, source);
    for (const { group } of source.families) {
      compareTrees(
        generatedRoots.forms.get(group),
        trackedTargets.forms.get(group),
        `${group} Forms`,
      );
    }
    compareFile(
      readFileSync(generatedRoots.familyIndex, "utf8"),
      trackedTargets.familyIndex,
      "current-family index",
    );
    compareTrees(generatedRoots.interfaces, trackedTargets.interfaces, "interfaces");
    compareTrees(generatedRoots.bindings, trackedTargets.bindings, "bindings");
    verifyPackages(trackedTargets.forms, source);
    process.stdout.write("Current Form Family candidates are reproducible and valid\n");
  } finally {
    rmSync(temporary, { recursive: true, force: true });
  }
}

function createTrackedTargets(source) {
  const familyRoot = path.join(repositoryRoot, "forms", "candidates");
  return {
    forms: new Map(
      source.families.map(({ group }) => [
        group,
        directChildPath(familyRoot, group, "family group"),
      ]),
    ),
    familyIndex: path.join(repositoryRoot, FAMILY_INDEX_PATH),
    interfaces: path.join(repositoryRoot, "interfaces", "candidates", "v1alpha1"),
    bindings: path.join(repositoryRoot, "bindings", "candidates", "v1alpha2"),
  };
}

function createOutputRoots(root, source) {
  const familyRoot = path.join(root, "forms");
  return {
    forms: new Map(
      source.families.map(({ group }) => [
        group,
        directChildPath(familyRoot, group, "family group"),
      ]),
    ),
    familyIndex: path.join(root, "current-family-index.json"),
    interfaces: path.join(root, "interfaces-v1alpha1"),
    bindings: path.join(root, "bindings-v1alpha2"),
  };
}

function generate(outputRoots, source) {
  for (const outputRoot of [
    ...outputRoots.forms.values(),
    path.dirname(outputRoots.familyIndex),
    outputRoots.interfaces,
    outputRoots.bindings,
  ]) {
    mkdirSync(outputRoot, { recursive: true });
  }
  const manifests = [];
  for (const sourceFamily of source.families) {
    const familyGroup = sourceFamily.group;
    const manifest = {
      format: "takoform.form-family-candidates@v1",
      family: familyGroup,
      formMaturity: source.formMaturity,
      packageApiVersion: source.packageApiVersion,
      publicationStatus: source.publicationStatus,
      authoringSource: sourceFamily.authoringSource,
      authoringPolicy: sourceFamily.authoringPolicy,
      forms: [],
    };
    for (const rendered of sourceFamily.forms) {
      const { kind, slug, role } = rendered;
      const definition = rendered.definition;
      if (
        definition?.kind !== kind ||
        definition?.apiVersion !== familyGroup ||
        !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(
          definition.definitionVersion ?? "",
        ) ||
        definition?.role !== role
      ) {
        throw new Error(`${familyGroup}/${slug}: family catalog emitted an invalid candidate identity`);
      }
      if ((definition.negativeConformanceFixtures ?? []).length === 0) {
        throw new Error(`${slug}: every family candidate must carry a negative fixture`);
      }
      if (Object.hasOwn(definition.desiredSchema?.properties ?? {}, "name")) {
        throw new Error(`${slug}: current desired schemas must not declare a name property`);
      }

	  const destinationRoot = directChildPath(
	    outputRoots.forms.get(familyGroup),
	    slug,
	    "Form slug",
	  );
	  const fixtureRoot = path.join(destinationRoot, "fixtures");
	  mkdirSync(fixtureRoot, { recursive: true });
      const fixtureNames = Object.keys(rendered.fixtures).sort();
      for (const fixtureName of fixtureNames) {
        writeJson(
		  directChildPath(fixtureRoot, fixtureName, "fixture name"),
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
        apiVersion: familyGroup,
        kind,
        // The version is the Form's own, not a catalog generation.
        definitionVersion: definition.definitionVersion,
        schemaDigest: digestCanonicalJSON(
          path.join(destinationRoot, "definition.json"),
        ),
      };
      const index = {
        apiVersion: source.packageApiVersion,
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
        path: `forms/candidates/${familyGroup}/${slug}`,
        formRef,
        packageDigest: digestCanonicalJSON(indexPath),
      });
    }
    if (manifest.forms.length === 0) {
      throw new Error(`${familyGroup}: family source contains no Forms`);
    }
    writeJson(
      path.join(outputRoots.forms.get(familyGroup), "candidate-set.json"),
      manifest,
    );
    manifests.push(manifest);
  }

  writeContractCandidates({
    outputRoot: outputRoots.interfaces,
    contracts: source.interfaces,
    candidateSet: {
      format: "takoform.interface-candidates@v1",
      publicationStatus: source.publicationStatus,
      authoringSource: source.interfaceAuthoringSource,
      interfaces: [],
    },
    listKey: "interfaces",
  });
  writeContractCandidates({
    outputRoot: outputRoots.bindings,
    contracts: source.bindings,
    candidateSet: {
      format: "takoform.binding-candidates@v1",
      publicationStatus: source.publicationStatus,
      authoringSource: source.bindingAuthoringSource,
      bindings: [],
    },
    listKey: "bindings",
  });
  const familyIndex = {
    format: source.familyIndexFormat,
    families: manifests
      .map((manifest) => {
        const candidateSet = `forms/candidates/${manifest.family}/candidate-set.json`;
        const stagedCandidateSet = path.join(
          outputRoots.forms.get(manifest.family),
          "candidate-set.json",
        );
        return {
          group: manifest.family,
          candidateSet,
          sha256: sha256Hex(readFileSync(stagedCandidateSet)),
          formCount: manifest.forms.length,
        };
      })
      .sort((left, right) =>
        left.group.localeCompare(right.group) ||
        left.candidateSet.localeCompare(right.candidateSet),
      ),
    interfaceCandidateSet: {
      path: INTERFACE_CANDIDATE_SET_PATH,
      sha256: sha256Hex(
        readFileSync(path.join(outputRoots.interfaces, "candidate-set.json")),
      ),
    },
    bindingCandidateSet: {
      path: BINDING_CANDIDATE_SET_PATH,
      sha256: sha256Hex(
        readFileSync(path.join(outputRoots.bindings, "candidate-set.json")),
      ),
    },
  };
  writeJson(outputRoots.familyIndex, familyIndex);
  return {
    families: manifests,
    forms: manifests.flatMap((manifest) => manifest.forms),
    familyIndex,
  };
}

function writeContractCandidates({ outputRoot, contracts, candidateSet, listKey }) {
  const seen = new Set();
  for (const contract of contracts) {
    const { name, version } = contract ?? {};
    if (
      typeof name !== "string" ||
      typeof version !== "string" ||
      seen.has(`${name}@${version}`)
    ) {
      throw new Error(`${listKey} source contains a duplicate or invalid contract identity`);
    }
    seen.add(`${name}@${version}`);
    // A contract's version is its own: worker.runtime went to 1.1.0 when the
    // env closure gained the external-service projections (decision 0045).
    // The source document's order and exact identities are the catalog.
    const definitionRaw = contract.definitionJson;
    if (typeof definitionRaw !== "string" || definitionRaw.length === 0) {
      throw new Error(`${name}: source renderer emitted no definitionJson text`);
    }
	const destination = path.join(
	  directChildPath(outputRoot, name, `${listKey} contract name`),
	  "definition.json",
	);
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
  const result = spawnSync("go", ["run", "./cmd/current-form-source"], {
    cwd: repositoryRoot,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.status !== 0) {
    throw new Error(`family Form source rendering failed\n${result.stderr}${result.stdout}`);
  }
  const rendered = JSON.parse(result.stdout);
  if (
    !Array.isArray(rendered?.families) ||
    rendered.families.length === 0 ||
    !Array.isArray(rendered?.interfaces) ||
    !Array.isArray(rendered?.bindings) ||
    typeof rendered.packageApiVersion !== "string" ||
    typeof rendered.familyIndexFormat !== "string" ||
    typeof rendered.formMaturity !== "string" ||
    typeof rendered.publicationStatus !== "string" ||
    typeof rendered.interfaceAuthoringSource !== "string" ||
    typeof rendered.bindingAuthoringSource !== "string"
  ) {
    throw new Error("family Form source emitted an unexpected document shape");
  }
  validatePublisherPathMetadata(rendered);
  const groups = new Set();
  for (const family of rendered.families) {
    if (
      family === null ||
      typeof family !== "object" ||
      typeof family.group !== "string" ||
      groups.has(family.group) ||
      typeof family.authoringSource !== "string" ||
      typeof family.authoringPolicy !== "string" ||
      !Array.isArray(family.forms) ||
      family.forms.length === 0
    ) {
      throw new Error("family Form source emitted an invalid family metadata record");
    }
    groups.add(family.group);
    const formIdentities = new Set();
    for (const form of family.forms) {
      if (
        form === null ||
        typeof form !== "object" ||
        typeof form.kind !== "string" ||
        typeof form.slug !== "string" ||
        typeof form.role !== "string" ||
        formIdentities.has(`${form.kind}/${form.slug}`)
      ) {
        throw new Error(`${family.group}: family Form source contains an invalid or duplicate Form`);
      }
      formIdentities.add(`${form.kind}/${form.slug}`);
    }
  }
  return rendered;
}

// validatePublisherPathMetadata closes every publisher-provided value that is
// later used as a filesystem component. The tako0614 publisher source is an
// authority for artifact identity, not authority to address arbitrary paths
// in the repository or staging tree.
export function validatePublisherPathMetadata(source) {
  if (!Array.isArray(source?.families) || !Array.isArray(source?.interfaces) || !Array.isArray(source?.bindings)) {
    throw new Error("publisher path metadata has an invalid source shape");
  }
  for (const family of source.families) {
    if (
      typeof family?.group !== "string" ||
      family.group.length > 253 ||
      !PUBLISHER_FAMILY_GROUP.test(family.group) ||
      !Array.isArray(family.forms)
    ) {
      throw new Error(`unsafe publisher family group ${JSON.stringify(family?.group)}`);
    }
    for (const form of family.forms) {
      if (
        typeof form?.kind !== "string" ||
        !FORM_KIND.test(form.kind) ||
        typeof form.slug !== "string" ||
        form.slug.length > 127 ||
        !FORM_SLUG.test(form.slug) ||
        form.fixtures === null ||
        typeof form.fixtures !== "object" ||
        Array.isArray(form.fixtures)
      ) {
        throw new Error(`${family.group}: unsafe Form path metadata`);
      }
      for (const name of Object.keys(form.fixtures)) {
        if (name.length > 127 || !FIXTURE_NAME.test(name)) {
          throw new Error(`${family.group}/${form.slug}: unsafe fixture name ${JSON.stringify(name)}`);
        }
      }
    }
  }
  for (const [kind, contracts] of [
    ["Interface", source.interfaces],
    ["Binding", source.bindings],
  ]) {
    for (const contract of contracts) {
      if (
        typeof contract?.name !== "string" ||
        contract.name.length > 127 ||
        !CONTRACT_NAME.test(contract.name) ||
        typeof contract.version !== "string" ||
        !CONTRACT_VERSION.test(contract.version)
      ) {
        throw new Error(`unsafe ${kind} path metadata`);
      }
    }
  }
}

function verifyPackages(roots, source) {
  for (const family of source.families) {
    for (const { slug } of family.forms) {
      const result = spawnSync(
        "go",
        [
          "run",
          "./cmd/form-package",
          "verify",
          path.join(roots.get(family.group), slug),
        ],
        { cwd: repositoryRoot, encoding: "utf8" },
      );
      if (result.status !== 0) {
        throw new Error(
          `${family.group}/${slug}: package verification failed\n${result.stderr}${result.stdout}`,
        );
      }
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

function assertSafeGeneratedTarget(target, trackedTargets) {
  const expected = [
    ...trackedTargets.forms.values(),
    trackedTargets.familyIndex,
    trackedTargets.interfaces,
    trackedTargets.bindings,
  ];
  const normalizedTarget = path.resolve(target);
  const familyRoot = path.resolve(repositoryRoot, "forms", "candidates");
  const isManagedFamily = path.dirname(normalizedTarget) === familyRoot;
  const isFixedTarget = [
    path.resolve(repositoryRoot, FAMILY_INDEX_PATH),
    path.resolve(repositoryRoot, "interfaces", "candidates", "v1alpha1"),
    path.resolve(repositoryRoot, "bindings", "candidates", "v1alpha2"),
  ].includes(normalizedTarget);
  if (
    !expected.map((entry) => path.resolve(entry)).includes(normalizedTarget) ||
    (!isManagedFamily && !isFixedTarget) ||
    path.dirname(normalizedTarget) === repositoryRoot
  ) {
    throw new Error(`refusing unsafe generated target ${target}`);
  }
}

function directChildPath(root, component, label) {
  const normalizedRoot = path.resolve(root);
  const candidate = path.resolve(normalizedRoot, component);
  if (path.dirname(candidate) !== normalizedRoot) {
    throw new Error(`refusing unsafe ${label} ${JSON.stringify(component)}`);
  }
  return candidate;
}

function writeJson(file, value) {
  mkdirSync(path.dirname(file), { recursive: true });
  writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function digest(bytes) {
  return `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
}

function sha256Hex(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
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

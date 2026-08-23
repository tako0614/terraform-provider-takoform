#!/usr/bin/env bun

// Deterministic builder for the Form Family candidate lane (spec decisions
// 0008..0035): Edge Platform Family Form Packages (package-index v1alpha4,
// form-definition v1beta1), the exact Interface and Binding candidate sets,
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
const FAMILY = "edge.forms.takoform.com/v1beta2";
const PACKAGE_API_VERSION = "packages.forms.takoform.com/v1alpha4";
const trackedTargets = {
  forms: path.join(repositoryRoot, "forms", "candidates", "edge", "v1beta2"),
  interfaces: path.join(repositoryRoot, "interfaces", "candidates", "v1alpha1"),
  bindings: path.join(repositoryRoot, "bindings", "candidates", "v1alpha1"),
};
const registryPath = path.join(
  repositoryRoot,
  "internal",
  "currentformregistry",
  "registry_v3_generated.go",
);
const providerIdentityLedgerPath = path.join(
  repositoryRoot,
  "release",
  "provider-form-identities.json",
);
const releaseDescriptorPath = path.join(repositoryRoot, "release", "version.json");
// v2.1.1 is the forward-repaired provider candidate's immutable compatibility
// commitment. The abandoned v2.1.0 candidate was never published and has no
// provider identity-ledger entry. Changing this entry together with the
// generated catalog must fail instead of being accepted as a self-consistent
// rebuild.
export const FROZEN_PROVIDER_RELEASES = new Map([
  [
    "2.1.1",
    Object.freeze({
      tag: "v2.1.1",
      ledgerDigest:
        "sha256:981181257fac1ec43f85eb250fc12dd271236b1bbde94dc93323ee2180c4255d",
    }),
  ],
]);
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
  ["DurableWorkflow", "durable-workflow", "identity"],
  ["ActorNamespace", "actor-namespace", "identity"],
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
  "worker.workflow",
  "worker.actor",
];
const bindingNames = [
  "module-worker.edge-kv",
  "module-worker.object-bucket",
  "module-worker.sqlite",
  "module-worker.queue-producer",
  "module-worker.service",
  "module-worker.workflow",
  "module-worker.actor",
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
        forms: path.join(stagingParent, "forms-v1beta1"),
        interfaces: path.join(stagingParent, "interfaces-v1alpha1"),
        bindings: path.join(stagingParent, "bindings-v1alpha1"),
      };
      const stagedRegistryPath = path.join(stagingParent, "registry_v3_generated.go");
      const manifest = generate(stagedRoots);
      verifyPackages(stagedRoots.forms);
      const embeddedIdentities = loadProviderIdentityLedger(manifest);
      writeFileSync(
        stagedRegistryPath,
        renderRegistry(manifest, unionSupportedIdentities(manifest, embeddedIdentities)),
      );
      installGeneratedOutputs(stagingParent, [
        [stagedRoots.forms, trackedTargets.forms],
        [stagedRoots.interfaces, trackedTargets.interfaces],
        [stagedRoots.bindings, trackedTargets.bindings],
        [stagedRegistryPath, registryPath],
      ]);
      process.stdout.write(
        `wrote ${forms.length} Edge Platform Family Form candidates, ${interfaceNames.length} interface candidates, ${bindingNames.length} binding candidates, and the provider v3 registry\n`,
      );
    } finally {
      rmSync(stagingParent, { recursive: true, force: true });
    }
    return;
  }

  const temporary = mkdtempSync(path.join(tmpdir(), "takoform-form-families-"));
  try {
    const generatedRoots = {
      forms: path.join(temporary, "forms-v1beta1"),
      interfaces: path.join(temporary, "interfaces-v1alpha1"),
      bindings: path.join(temporary, "bindings-v1alpha1"),
    };
    const manifest = generate(generatedRoots);
    const embeddedIdentities = loadProviderIdentityLedger(manifest);
    for (const [key, generatedRoot] of Object.entries(generatedRoots)) {
      compareTrees(generatedRoot, trackedTargets[key], key);
    }
    verifyPackages(trackedTargets.forms);
    compareFile(
      renderRegistry(manifest, unionSupportedIdentities(manifest, embeddedIdentities)),
      registryPath,
      "provider v3 registry",
    );
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
    formMaturity: "experimental",
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
      !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(
        definition.definitionVersion ?? "",
      ) ||
      definition.role !== role
    ) {
      throw new Error(`${slug}: family catalog emitted an invalid candidate identity`);
    }
    if ((definition.negativeConformanceFixtures ?? []).length === 0) {
      throw new Error(`${slug}: every family candidate must carry a negative fixture`);
    }
    if (Object.hasOwn(definition.desiredSchema?.properties ?? {}, "name")) {
      throw new Error(`${slug}: v1beta1 desired schemas must not declare a name property`);
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
      // The version is the Form's own, not the generation's: a member whose
      // contract diverged from the generation line carries a different one,
      // and the source catalog is the only place that decides which
      // (decision 0046).
      definitionVersion: definition.definitionVersion,
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
      path: `forms/candidates/edge/v1beta2/${slug}`,
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
    // A contract's version is its own: worker.runtime went to 1.1.0 when the
    // env closure gained the external-service projections (decision 0045).
    // What must not drift is the ORDER and the NAMES, which is what pins the
    // candidate set to the catalog.
    if (
      contract?.name !== name ||
      !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(contract?.version ?? "")
    ) {
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
function renderRegistry(manifest, supportedIdentities) {
  const exactKey = (entry) =>
    `{APIVersion: ${JSON.stringify(entry.formRef.apiVersion)}, ` +
    `Kind: ${JSON.stringify(entry.formRef.kind)}, ` +
    `DefinitionVersion: ${JSON.stringify(entry.formRef.definitionVersion)}, ` +
    `SchemaDigest: ${JSON.stringify(entry.formRef.schemaDigest)}}`;
  const defaults = manifest.forms
    .map(
      (entry) =>
        `\t{APIVersion: ${JSON.stringify(entry.formRef.apiVersion)}, ` +
        `Kind: ${JSON.stringify(entry.formRef.kind)}}: ${exactKey(entry)},`,
    )
    .join("\n");
  const supported = supportedIdentities
    .map(
      (entry) =>
        `\t${exactKey(entry)}: {` +
        `APIVersion: ${JSON.stringify(entry.formRef.apiVersion)}, ` +
        `Kind: ${JSON.stringify(entry.formRef.kind)}, ` +
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


// unionSupportedIdentities is the state-compatibility fence in two-generation
// form: every exact identity a RELEASED provider embedded stays supported
// forever, and the current candidate generation's identities are what this
// build creates and must equally serve. Before the catalog moved past the
// published release the two sets were equal and the union is a no-op.
function unionSupportedIdentities(manifest, embeddedIdentities) {
  const byKey = new Map();
  for (const form of embeddedIdentities) {
    byKey.set(canonicalJson(form.formRef), form);
  }
  for (const { formRef, packageDigest } of manifest.forms) {
    const key = canonicalJson(formRef);
    // A candidate contributes its IDENTITY. The provider's authoring name is
    // the provider's, and a released ledger entry that carries one keeps it
    // (decision 0047).
    if (!byKey.has(key)) byKey.set(key, { formRef, packageDigest });
  }
  return [...byKey.values()];
}

// The provider identity ledger is independent of package publication. Once a
// provider release embeds an exact Beta FormRef and package digest, the entry
// remains in v3Supported forever, even after a later family becomes the create
// default. This is the source-level state-compatibility fence for existing
// Beta resources; release/provider-form-identities.json is never regenerated.
function loadProviderIdentityLedger(manifest) {
  const ledger = JSON.parse(readFileSync(providerIdentityLedgerPath, "utf8"));
  const release = JSON.parse(readFileSync(releaseDescriptorPath, "utf8"));
  if (
    ledger?.format !== "takoform.provider-form-identities@v1" ||
    !Array.isArray(ledger.releases) ||
    ledger.releases.length === 0
  ) {
    throw new Error("provider Form identity ledger has an invalid envelope");
  }
  if (
    release?.version !== "2.1.1" ||
    release?.tag !== `v${release.version}` ||
    release?.publicationStatus !== "candidate-only" ||
    release?.versioning?.portableApiVersion !== "forms.takoform.com/v1beta1"
  ) {
    throw new Error("provider release descriptor must name candidate-only v2.1.1 on Host API v1beta1");
  }
  assertFrozenProviderReleaseDescriptor(release);

  const supported = [];
  const exactKeys = new Set();
  const providerVersions = new Set();
  const families = new Set();
  let currentRelease;
  for (const entry of ledger.releases) {
    if (
      entry === null ||
      typeof entry !== "object" ||
      Object.keys(entry).sort().join(",") !==
        "family,formMaturity,forms,portableApiVersion,providerVersion" ||
      typeof entry.providerVersion !== "string" ||
      typeof entry.portableApiVersion !== "string" ||
      typeof entry.family !== "string" ||
      entry.formMaturity !== "experimental" ||
      !Array.isArray(entry.forms) ||
      entry.forms.length === 0
    ) {
      throw new Error("provider Form identity ledger contains an invalid release");
    }
    if (providerVersions.has(entry.providerVersion)) {
      throw new Error(`provider Form identity ledger duplicates ${entry.providerVersion}`);
    }
    providerVersions.add(entry.providerVersion);
    families.add(entry.family);
    assertFrozenProviderRelease(entry);
    if (entry.providerVersion === release.version) currentRelease = entry;

    for (const form of entry.forms) {
      if (
        form === null ||
        typeof form !== "object" ||
        Object.keys(form).sort().join(",") !== "formRef,packageDigest,resourceType" ||
        !/^takoform_[a-z0-9_]+$/u.test(form.resourceType) ||
        !/^sha256:[0-9a-f]{64}$/u.test(form.packageDigest) ||
        form.formRef === null ||
        typeof form.formRef !== "object" ||
        Object.keys(form.formRef).sort().join(",") !==
          "apiVersion,definitionVersion,kind,schemaDigest" ||
        form.formRef.apiVersion !== entry.family ||
        !/^[A-Z][A-Za-z0-9]{0,63}$/u.test(form.formRef.kind) ||
        !/^sha256:[0-9a-f]{64}$/u.test(form.formRef.schemaDigest)
      ) {
        throw new Error(`${entry.providerVersion}: invalid provider-embedded Form identity`);
      }
      const exactKey = canonicalJson(form.formRef);
      if (exactKeys.has(exactKey)) {
        throw new Error(`${entry.providerVersion}: duplicate provider-embedded FormRef ${exactKey}`);
      }
      exactKeys.add(exactKey);
      supported.push(form);
    }
  }
  if (
    currentRelease === undefined ||
    currentRelease.portableApiVersion !== release.versioning.portableApiVersion ||
    currentRelease.formMaturity !== manifest.formMaturity
  ) {
    throw new Error("provider Form identity ledger has no exact entry for the release descriptor");
  }
  if (currentRelease.family === manifest.family) {
    // The catalog builds the generation the descriptor-named release
    // embeds, so the two must be byte-equal: a drifted digest is a changed
    // contract hiding inside a published SemVer.
    const currentManifest = manifest.forms.map(
      ({ resourceType, formRef, packageDigest }) => ({
        resourceType,
        formRef,
        packageDigest,
      }),
    );
    if (canonicalJson(currentRelease.forms) !== canonicalJson(currentManifest)) {
      throw new Error(
        "published embedded FormRefs/digests drifted; mint a new family identity and provider release instead of changing the ledger",
      );
    }
  } else {
    // The catalog has moved PAST the published release: the Beta 2 state
    // decision 0046 names, with the published generation frozen in its
    // ledger entry and the current generation awaiting its own release.
    // What holds then: no released entry may already claim the new family
    // version (a release's family is immutable), and the frozen-entry
    // checks above already refused any edit to the published set. The
    // byte-equality obligation transfers to the next release's gate when
    // its ledger entry is minted.
    if (families.has(manifest.family)) {
      throw new Error(
        `family ${manifest.family} is already claimed by a released ledger entry; a moved catalog mints a NEW family version`,
      );
    }
  }
  return supported;
}

// Keep the check as a pure exported helper so the direct script test can prove
// that mutating a retained ledger entry is rejected even when a candidate
// catalog is regenerated alongside it.
export function assertFrozenProviderRelease(entry) {
  const frozen = FROZEN_PROVIDER_RELEASES.get(entry?.providerVersion);
  if (frozen === undefined) return;
  const actual = digestCanonicalValue(entry);
  if (actual !== frozen.ledgerDigest) {
    throw new Error(
      `immutable provider ${entry.providerVersion} identity ledger entry changed: ${actual} != ${frozen.ledgerDigest}`,
    );
  }
}

export function assertFrozenProviderReleaseDescriptor(release) {
  const frozen = FROZEN_PROVIDER_RELEASES.get(release?.version);
  if (frozen === undefined || release.tag === frozen.tag) return;
  throw new Error(
    `immutable provider ${release.version} release tag changed: ${release.tag} != ${frozen.tag}`,
  );
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

function digestCanonicalValue(value) {
  return `sha256:${createHash("sha256").update(canonicalJson(value)).digest("hex")}`;
}

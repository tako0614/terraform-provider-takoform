#!/usr/bin/env bun

import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  provider4CandidatePaths,
  readCurrentProvider4Release,
} from "./provider-release-descriptor.mjs";

// These exports are retained only for callers that inspect the immutable
// historical Provider 4.0.0 candidate.  Generation below always derives paths
// from readCurrentProvider4Release rather than addressing these fixed paths.
const HISTORICAL_PROVIDER4_VERSION = "4.0.0";
const HISTORICAL_PROVIDER4_PATHS = provider4CandidatePaths(
  HISTORICAL_PROVIDER4_VERSION,
);
export const PROVIDER4_DESCRIPTOR = HISTORICAL_PROVIDER4_PATHS.descriptorPath;
export const PROVIDER4_IDENTITIES =
  HISTORICAL_PROVIDER4_PATHS.formIdentitiesPath;
export const PROVIDER_RELEASE_DESCRIPTOR = "release/version.json";
const PROVIDER3_HISTORY_DESCRIPTOR = "release/history/provider-v3.0.0.json";
const PROVIDER_IDENTITY_LEDGER = "release/provider-form-identities.json";
const PROVIDER_RELEASE_IDENTITIES = "release/provider-release-identities.json";
const PUBLISHER_CLOSURE = "internal/provider/artifacts/publisher/closure.json";
const PUBLISHER_ROOT = "internal/provider/artifacts/publisher";
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const RESOURCE_TYPE = /^takoform_[a-z0-9_]+$/u;
const PUBLISHER_FAMILY = "edge.forms.takoform.com";
const HISTORICAL_PROVIDER4_DESCRIPTOR_SHA256 =
  "f658e968009d68b6642753ef61aeb437a7e3c54427d1f01327aedadbe442d099";
const HISTORICAL_PROVIDER4_IDENTITIES_SHA256 =
  "c6b32649209474dcb2ae29c6670c41124126f887ada76d62493e2b33721ee21b";
const HISTORICAL_PROVIDER4_LEDGER_ENTRY_DIGEST =
  "sha256:76086ec15b12c9d7c8e9cb4cc281d08f006ae2028fbee5b3bbac85bc59bbc2c6";
const HISTORICAL_PROVIDER4_PUBLISHED_READBACK_DIGEST =
  "sha256:f9659e1205a899949b98da8f220dc98b66e97078fbc2d13beee53662b2acaa22";

function fail(message) {
  throw new Error(`Provider 4 candidate: ${message}`);
}

function readJson(root, relativePath) {
  try {
    return JSON.parse(readFileSync(path.join(root, relativePath), "utf8"));
  } catch (error) {
    fail(`${relativePath} is not readable JSON (${error.message})`);
  }
}

function recursivelySorted(value) {
  if (Array.isArray(value)) return value.map(recursivelySorted);
  if (value === null || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, recursivelySorted(value[key])]),
  );
}

function canonicalDigest(value) {
  return `sha256:${createHash("sha256")
    .update(JSON.stringify(recursivelySorted(value)))
    .digest("hex")}`;
}

function rawDigest(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function readBytes(root, relativePath) {
  try {
    return readFileSync(path.join(root, relativePath));
  } catch (error) {
    fail(`${relativePath} is not readable (${error.message})`);
  }
}

function assertImmutableFile(root, relativePath, expectedDigest) {
  const actualDigest = rawDigest(readBytes(root, relativePath));
  if (actualDigest !== expectedDigest) {
    fail(`${relativePath} immutable Provider 4.0.0 bytes changed`);
  }
}

function uniqueRelease(releases, version, label) {
  const matches = releases.filter(
    (release) => release?.providerVersion === version,
  );
  if (matches.length !== 1) {
    fail(`${label} must carry exactly one ${version} release, found ${matches.length}`);
  }
  return matches[0];
}

function uniquePublishedReadback(entries, version) {
  const matches = entries.filter((entry) => entry?.version === version);
  if (matches.length !== 1) {
    fail(
      `${PROVIDER_RELEASE_IDENTITIES} must carry exactly one published ${version} entry, found ${matches.length}`,
    );
  }
  return matches[0];
}

function assertImmutableProvider4History(root) {
  // Published Provider 4.0.0 is retained history.  Keep its candidate
  // descriptor and identity projection byte-exact even while a later 4.x
  // candidate is current.
  assertImmutableFile(
    root,
    HISTORICAL_PROVIDER4_PATHS.descriptorPath,
    HISTORICAL_PROVIDER4_DESCRIPTOR_SHA256,
  );
  assertImmutableFile(
    root,
    HISTORICAL_PROVIDER4_PATHS.formIdentitiesPath,
    HISTORICAL_PROVIDER4_IDENTITIES_SHA256,
  );

  const history = readJson(root, PROVIDER_IDENTITY_LEDGER);
  if (!Array.isArray(history?.releases)) {
    fail(`${PROVIDER_IDENTITY_LEDGER} has no release entries`);
  }
  const retained = uniqueRelease(
    history.releases,
    HISTORICAL_PROVIDER4_VERSION,
    PROVIDER_IDENTITY_LEDGER,
  );
  if (canonicalDigest(retained) !== HISTORICAL_PROVIDER4_LEDGER_ENTRY_DIGEST) {
    fail("immutable provider 4.0.0 identity ledger entry changed");
  }

  const published = readJson(root, PROVIDER_RELEASE_IDENTITIES);
  if (!Array.isArray(published.entries)) {
    fail(`${PROVIDER_RELEASE_IDENTITIES} has no published release entries`);
  }
  const readback = uniquePublishedReadback(
    published.entries,
    HISTORICAL_PROVIDER4_VERSION,
  );
  if (canonicalDigest(readback) !== HISTORICAL_PROVIDER4_PUBLISHED_READBACK_DIGEST) {
    fail("immutable published Provider 4.0.0 readback entry changed");
  }
  return { history, published, retained, readback };
}

function currentPublishedEntry(version, published) {
  const matches = (published.entries ?? []).filter(
    (entry) => entry?.version === version,
  );
  if (matches.length > 1) {
    fail(
      `${PROVIDER_RELEASE_IDENTITIES} must carry exactly one ${version} release, found ${matches.length}`,
    );
  }
  return matches[0];
}

function formRefKey(ref) {
  return JSON.stringify([
    ref?.apiVersion,
    ref?.kind,
    ref?.definitionVersion,
    ref?.schemaDigest,
  ]);
}

function deriveProvider4Identities(root) {
  const current = readCurrentProvider4Release(root);
  const { descriptor, candidateFormIdentitiesPath } = current;
  const { published } = assertImmutableProvider4History(root);
  const publishedCurrent = currentPublishedEntry(descriptor.version, published);

  const provider3Document = readJson(root, PROVIDER3_HISTORY_DESCRIPTOR);
  if (
    provider3Document.version !== "3.0.0" ||
    provider3Document.tag !== "v3.0.0" ||
    provider3Document.publicationStatus !== "candidate-only"
  ) {
    fail(
      `${PROVIDER3_HISTORY_DESCRIPTOR} no longer preserves the retained Provider 3 writer input`,
    );
  }

  const history = readJson(root, PROVIDER_IDENTITY_LEDGER);
  if (
    history.format !== "takoform.provider-form-identities@v1" ||
    history.releases?.find((release) => release?.providerVersion === "3.0.0")
      ?.forms?.length !== 31
  ) {
    fail("the Provider identity ledger is not an exact Provider 3 history surface");
  }

  const closure = readJson(root, PUBLISHER_CLOSURE);
  if (
    closure.format !== "takoform.provider-publisher-set-artifact-closure@v1" ||
    closure.projection?.path !== "projection.json" ||
    !SHA256.test(closure.projection?.digest ?? "") ||
    closure.packages?.length !== 17
  ) {
    fail(`${PUBLISHER_CLOSURE} is not the exact 17-package publisher closure`);
  }
  const projection = readJson(
    root,
    path.posix.join(PUBLISHER_ROOT, closure.projection.path),
  );
  if (
    canonicalDigest(projection) !== closure.projection.digest ||
    projection.format !== "takoform.provider-publisher-set-projection@v1" ||
    projection.hostApi !== descriptor.versioning.portableApiVersion
  ) {
    fail("the publisher-selected Provider projection identity or digest drifted");
  }

  const packages = new Map();
  for (const entry of closure.packages) {
    if (
      entry?.formRef?.apiVersion !== PUBLISHER_FAMILY ||
      !SHA256.test(entry?.formRef?.schemaDigest ?? "") ||
      !SHA256.test(entry?.packageDigest ?? "")
    ) {
      fail("the publisher package closure contains a non-Edge or malformed FormRef");
    }
    const key = formRefKey(entry.formRef);
    if (packages.has(key)) fail("the publisher package closure repeats a FormRef");
    packages.set(key, entry);
  }

  const resourceTypes = new Set();
  const forms = projection.resources
    .filter((entry) => entry?.register === true)
    .map((entry) => {
      const packageEntry = packages.get(formRefKey(entry.ref));
      if (
        packageEntry === undefined ||
        entry.ref?.packageDigest !== packageEntry.packageDigest ||
        !RESOURCE_TYPE.test(entry.resourceType ?? "") ||
        resourceTypes.has(entry.resourceType)
      ) {
        fail("the registered Provider projection is not an exact publisher package mapping");
      }
      resourceTypes.add(entry.resourceType);
      return {
        resourceType: entry.resourceType,
        formRef: {
          apiVersion: entry.ref.apiVersion,
          kind: entry.ref.kind,
          definitionVersion: entry.ref.definitionVersion,
          schemaDigest: entry.ref.schemaDigest,
        },
        packageDigest: entry.ref.packageDigest,
      };
    })
    .sort((left, right) => left.resourceType.localeCompare(right.resourceType));
  if (forms.length !== 17 || packages.size !== forms.length) {
    fail(`the Provider 4 release maps ${forms.length}/${packages.size} Forms, want 17/17`);
  }

  // One derivation, two projections of it. The candidate record keeps the
  // publisher provenance triple; the identity ledger entry is the append-only
  // cross-version shape both release entrypoints read for the descriptor.
  return {
    candidate: {
      format: "takoform.provider-candidate-form-identities@v1",
      providerVersion: descriptor.version,
      formPublisherRepository: descriptor.formPublisherRepository,
      formPublisherCommit: descriptor.formPublisherCommit,
      formSetTag: descriptor.formSetTag,
      portableApiVersion: descriptor.versioning.portableApiVersion,
      families: [PUBLISHER_FAMILY],
      forms,
    },
    ledgerEntry: {
      providerVersion: descriptor.version,
      portableApiVersion: descriptor.versioning.portableApiVersion,
      families: [PUBLISHER_FAMILY],
      formMaturity: "experimental",
      forms,
    },
    paths: { descriptorPath: current.descriptorPath, candidateFormIdentitiesPath },
    publishedCurrent,
  };
}

export function generateProvider4Identities(root) {
  const { candidate, ledgerEntry } = deriveProvider4Identities(root);
  return { candidate, ledgerEntry };
}

export function generateProvider4CandidateIdentities(root) {
  return deriveProvider4Identities(root).candidate;
}

export function validateProvider4Candidate(root) {
  const { candidate, ledgerEntry, paths } = deriveProvider4Identities(root);
  const actual = readJson(root, paths.candidateFormIdentitiesPath);
  if (JSON.stringify(actual) !== JSON.stringify(candidate)) {
    fail(
      `${paths.candidateFormIdentitiesPath} is stale; run bun run sync:provider4-candidate`,
    );
  }
  const ledger = readJson(root, PROVIDER_IDENTITY_LEDGER);
  const recorded = (ledger.releases ?? []).filter(
    (release) => release?.providerVersion === ledgerEntry.providerVersion,
  );
  if (recorded.length !== 1) {
    fail(
      `${PROVIDER_IDENTITY_LEDGER} must carry exactly one ${ledgerEntry.providerVersion} release, found ${recorded.length}`,
    );
  }
  if (JSON.stringify(recorded[0]) !== JSON.stringify(ledgerEntry)) {
    fail(`${PROVIDER_IDENTITY_LEDGER} is stale; run bun run sync:provider4-candidate`);
  }
  return candidate;
}

function writeLedgerEntry(root, ledgerEntry, publishedCurrent) {
  const ledgerPath = path.join(root, PROVIDER_IDENTITY_LEDGER);
  const ledger = readJson(root, PROVIDER_IDENTITY_LEDGER);
  const releases = [...(ledger.releases ?? [])];
  const positions = releases.flatMap((release, index) =>
    release?.providerVersion === ledgerEntry.providerVersion ? [index] : [],
  );
  if (positions.length > 1) {
    fail(
      `${PROVIDER_IDENTITY_LEDGER} must carry exactly one ${ledgerEntry.providerVersion} release, found ${positions.length}`,
    );
  }
  const position = positions[0] ?? -1;
  // A published identity is append-only.  An exact no-op is safe, while any
  // attempted rewrite (or omission) of its candidate ledger entry is rejected
  // before writing.  This applies to later Provider 4.x publications too.
  if (publishedCurrent !== undefined) {
    if (
      position === -1 ||
      JSON.stringify(releases[position]) !== JSON.stringify(ledgerEntry)
    ) {
      fail(
        `published Provider ${ledgerEntry.providerVersion} identity ledger entry changed`,
      );
    }
    return;
  }
  // Unpublished candidate entries may be regenerated in place.  New releases
  // append, preserving the ledger's append-only history.
  if (position === -1) releases.push(ledgerEntry);
  else releases[position] = ledgerEntry;
  writeFileSync(
    ledgerPath,
    `${JSON.stringify({ ...ledger, releases }, null, 2)}\n`,
  );
}

function writeCandidateIdentities(root, relativePath, candidate, publishedCurrent) {
  const bytes = Buffer.from(`${JSON.stringify(candidate, null, 2)}\n`);
  if (publishedCurrent !== undefined) {
    let existing;
    try {
      existing = readBytes(root, relativePath);
    } catch (error) {
      fail(
        `published Provider ${candidate.providerVersion} candidate identity is missing`,
      );
    }
    if (!existing.equals(bytes)) {
      fail(
        `published Provider ${candidate.providerVersion} candidate identity bytes changed`,
      );
    }
    return;
  }
  writeFileSync(path.join(root, relativePath), bytes);
}

export function writeProvider4Candidate(root) {
  const { candidate, ledgerEntry, paths, publishedCurrent } =
    deriveProvider4Identities(root);
  writeCandidateIdentities(
    root,
    paths.candidateFormIdentitiesPath,
    candidate,
    publishedCurrent,
  );
  writeLedgerEntry(root, ledgerEntry, publishedCurrent);
  return candidate;
}

function main() {
  const mode = process.argv[2];
  if (!["--check", "--write"].includes(mode)) {
    fail("usage: bun scripts/provider4-candidate.mjs --check|--write");
  }
  const root = path.resolve(import.meta.dirname, "..");
  const candidate =
    mode === "--write"
      ? writeProvider4Candidate(root)
      : validateProvider4Candidate(root);
  process.stdout.write(
    `Provider 4 release: ${candidate.forms.length} publisher-selected Forms recorded in ${PROVIDER_IDENTITY_LEDGER}; the retained Provider 3 writer input stays in ${PROVIDER3_HISTORY_DESCRIPTOR}\n`,
  );
}

if (import.meta.main) main();

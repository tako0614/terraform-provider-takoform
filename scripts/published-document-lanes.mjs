#!/usr/bin/env bun

// A published URL names one lane forever.
//
// The conformance corpora are published, and the corpus directories were
// numbered by generation rather than by the lane they measure. For three
// generations the counter and the lane agreed; when the current lane became
// forms.takoform.com/v1beta1, conformance/portable-host-v3 was rewritten in
// place and one published address began answering about a different contract
// than it had answered about the day before. Nothing noticed, because every
// gate compared the corpus against itself.
//
// This ledger records which lane each published document declares. A new
// document may be added. An existing one may never change what it means, and
// may never disappear: both are the same promise decision 0035 makes about
// v1alpha3 schemas, specifications, operation tables, public URLs, and bytes.
//
// It deliberately reads the BUILT site rather than the sources, because the
// thing that must not move is the address a reader fetches.

import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Glob } from "bun";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/published-document-lanes.mjs --write|--check");
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const publishedRoot = path.join(repositoryRoot, "website", "public");
const ledgerPath = path.join(
  repositoryRoot,
  "release",
  "published-document-lanes.json",
);
const LEDGER_KIND = "takoform.published-document-lanes@v1";

// Only a top-level string apiVersion is a lane declaration. A nested one
// describes something the document refers to, not what the document is.
function declaredLane(absolutePath) {
  let parsed;
  try {
    parsed = JSON.parse(readFileSync(absolutePath, "utf8"));
  } catch {
    return undefined;
  }
  if (parsed === null || typeof parsed !== "object" || Array.isArray(parsed)) {
    return undefined;
  }
  return typeof parsed.apiVersion === "string" ? parsed.apiVersion : undefined;
}

function scanPublishedDocuments() {
  const found = new Map();
  for (const relative of new Glob("**/*.json").scanSync(publishedRoot)) {
    const lane = declaredLane(path.join(publishedRoot, relative));
    if (lane !== undefined) {
      found.set(relative.split(path.sep).join("/"), lane);
    }
  }
  return found;
}

function readLedger() {
  const ledger = JSON.parse(readFileSync(ledgerPath, "utf8"));
  if (ledger?.kind !== LEDGER_KIND) {
    throw new Error(`${ledgerPath}: ledger kind is not ${LEDGER_KIND}`);
  }
  const recorded = new Map();
  for (const entry of ledger.documents ?? []) {
    recorded.set(entry.path, entry.apiVersion);
  }
  return recorded;
}

function writeLedger(found) {
  const documents = [...found.entries()]
    .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
    .map(([documentPath, apiVersion]) => ({ path: documentPath, apiVersion }));
  writeFileSync(
    ledgerPath,
    `${JSON.stringify({ kind: LEDGER_KIND, documents }, null, 2)}\n`,
  );
  process.stdout.write(
    `recorded the lane of ${documents.length} published documents\n`,
  );
}

const found = scanPublishedDocuments();

if (mode === "--write") {
  writeLedger(found);
  process.exit(0);
}

const recorded = readLedger();
const problems = [];

for (const [documentPath, recordedLane] of recorded) {
  const currentLane = found.get(documentPath);
  if (currentLane === undefined) {
    problems.push(
      `${documentPath} declared ${recordedLane} and is no longer published; ` +
        `a published address is retained history and does not disappear`,
    );
    continue;
  }
  if (currentLane !== recordedLane) {
    problems.push(
      `${documentPath} declared ${recordedLane} and now declares ${currentLane}; ` +
        `a published address names one lane forever, so serve the new contract ` +
        `at a new address and leave this one alone`,
    );
  }
}

const added = [...found.keys()].filter((each) => !recorded.has(each));

if (problems.length > 0) {
  process.stderr.write(
    `Published document lanes changed in ${problems.length} place(s):\n`,
  );
  for (const problem of problems) {
    process.stderr.write(`- ${problem}\n`);
  }
  process.stderr.write(
    "\nRun bun run sync:published-document-lanes only after the address that " +
      "moved has been restored; the ledger records history and never launders " +
      "a change to it.\n",
  );
  process.exit(1);
}

process.stdout.write(
  `published document lanes OK: ${recorded.size} addresses hold their lane` +
    (added.length > 0
      ? `, ${added.length} newly published address(es) not yet recorded\n`
      : "\n"),
);

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
// document may be added. An existing one may never change what it means.
//
// While Takoform is pre-Stable an address may be WITHDRAWN, and decision 0037
// makes that a recorded act rather than a deletion: the entry moves to
// `retired`, keeping the lane it declared, so the ledger still answers what
// that URL meant. What stops is the promise that it keeps answering. Silence
// is still refused — `--write` will not drop a recorded document, because a
// regenerating writer that quietly forgets an address is the same laundering
// this file exists to prevent.
//
// It deliberately reads the BUILT site rather than the sources, because the
// thing that must not move is the address a reader fetches.

import { execFileSync } from "node:child_process";
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
  const retired = new Map();
  for (const [index, entry] of (ledger.retired ?? []).entries()) {
    if (
      typeof entry?.retiredBecause !== "string" ||
      entry.retiredBecause.trim() === ""
    ) {
      throw new Error(
        `${ledgerPath}: retired ${index} must say why the address was withdrawn`,
      );
    }
    if (recorded.has(entry.path)) {
      throw new Error(
        `${ledgerPath}: ${entry.path} is both published and retired; ` +
          `an address is one or the other`,
      );
    }
    retired.set(entry.path, entry.apiVersion);
  }
  return { recorded, retired };
}

function writeLedger(found, previous) {
  const dropped = [...previous.recorded.keys()].filter(
    (each) => !found.has(each),
  );
  if (dropped.length > 0) {
    process.stderr.write(
      `Refusing to forget ${dropped.length} recorded address(es):\n`,
    );
    for (const each of dropped) {
      process.stderr.write(`- ${each}\n`);
    }
    process.stderr.write(
      "\nA writer that regenerates from what happens to be on disk would drop a " +
        "published address without anyone deciding to. Move each entry to the " +
        "ledger's `retired` list, with the lane it declared and why it was " +
        "withdrawn, and run this again.\n",
    );
    process.exit(1);
  }
  const documents = [...found.entries()]
    .filter(([documentPath]) => !previous.retired.has(documentPath))
    .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
    .map(([documentPath, apiVersion]) => ({ path: documentPath, apiVersion }));
  const ledger = { kind: LEDGER_KIND, documents };
  if (previous.retired.size > 0) {
    ledger.retired = JSON.parse(readFileSync(ledgerPath, "utf8")).retired;
  }
  writeFileSync(ledgerPath, `${JSON.stringify(ledger, null, 2)}\n`);
  process.stdout.write(
    `recorded the lane of ${documents.length} published documents` +
      (previous.retired.size > 0
        ? `, ${previous.retired.size} retired\n`
        : "\n"),
  );
}

// The refusal to forget lives in the writer, and a writer is something a
// person chooses to run. An entry deleted by hand — from the ledger and from
// the tree in one edit — would leave nothing behind to notice it, so the same
// promise is enforced here against the ledger's own committed history, the way
// scripts/deploy.mjs enforces the schema identity ledger.
//
// A recorded address must still be one of two things: served under the lane it
// declared, or withdrawn under that same lane. Vanishing is neither.
function ledgerHistory() {
  const git = (...args) => {
    try {
      return execFileSync("git", args, {
        cwd: repositoryRoot,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
      });
    } catch {
      return undefined;
    }
  };
  const relative = path.relative(repositoryRoot, ledgerPath);
  const log = git("log", "--full-history", "--format=%H", "HEAD", "--", relative);
  if (log === undefined) return undefined;
  const revisions = [];
  for (const commit of log.split("\n").filter((value) => value !== "")) {
    const blob = git("show", `${commit}:${relative}`);
    if (blob === undefined) continue;
    let parsed;
    try {
      parsed = JSON.parse(blob);
    } catch {
      continue;
    }
    revisions.push({ commit, documents: parsed?.documents ?? [] });
  }
  return revisions;
}

const found = scanPublishedDocuments();
const previous = readLedger();

if (mode === "--write") {
  writeLedger(found, previous);
  process.exit(0);
}

const { recorded, retired } = previous;
const problems = [];

for (const [documentPath, recordedLane] of recorded) {
  const currentLane = found.get(documentPath);
  if (currentLane === undefined) {
    problems.push(
      `${documentPath} declared ${recordedLane} and is no longer published; ` +
        `withdraw it by moving it to the ledger's retired list, so the ledger ` +
        `still answers what that address meant`,
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

// A retired address that answers again is the dangerous case the ledger exists
// for: the URL a reader knows now means whatever was published on top of it.
for (const [documentPath, retiredLane] of retired) {
  const currentLane = found.get(documentPath);
  if (currentLane !== undefined) {
    problems.push(
      `${documentPath} was retired declaring ${retiredLane} and is published ` +
        `again declaring ${currentLane}; a withdrawn address is not free real ` +
        `estate, so serve this at a new address`,
    );
  }
}

const history = ledgerHistory();
let historyNote = "no committed ledger history here, so no erasure was refuted";
if (history !== undefined && history.length > 0) {
  // One address answered for across many revisions is one problem, not one per
  // revision it appears in.
  const reported = new Set();
  for (const { commit, documents } of history) {
    for (const entry of documents) {
      if (reported.has(entry.path)) continue;
      reported.add(entry.path);
      const servedLane = recorded.get(entry.path);
      const withdrawnLane = retired.get(entry.path);
      if (servedLane === undefined && withdrawnLane === undefined) {
        problems.push(
          `${entry.path} declared ${entry.apiVersion} at ${commit.slice(0, 8)} ` +
            `and is in neither list now; an address is served or withdrawn, ` +
            `never erased`,
        );
        continue;
      }
      const lane = servedLane ?? withdrawnLane;
      if (lane !== entry.apiVersion) {
        problems.push(
          `${entry.path} declared ${entry.apiVersion} at ${commit.slice(0, 8)} ` +
            `and is now recorded as ${lane}; the ledger records what an address ` +
            `meant and does not restate it`,
        );
      }
    }
  }
  historyNote = `${history.length} committed ledger revision(s) checked`;
}

const added = [...found.keys()].filter(
  (each) => !recorded.has(each) && !retired.has(each),
);

if (problems.length > 0) {
  process.stderr.write(
    `Published document lanes changed in ${problems.length} place(s):\n`,
  );
  for (const problem of problems) {
    process.stderr.write(`- ${problem}\n`);
  }
  process.stderr.write(
    "\nThe ledger records history. It never launders a change to it, and " +
      "sync:published-document-lanes will not forget an address for you.\n",
  );
  process.exit(1);
}

process.stdout.write(
  `published document lanes OK: ${recorded.size} addresses hold their lane` +
    (retired.size > 0 ? `, ${retired.size} withdrawn` : "") +
    (added.length > 0
      ? `, ${added.length} newly published address(es) not yet recorded`
      : "") +
    `; ${historyNote}\n`,
);

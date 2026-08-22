#!/usr/bin/env bun

// What a generation move actually changed, per Form.
//
// When the Edge family moved from edge.forms.takoform.com/v1alpha1 to
// /v1beta1, all fifteen Forms got new schemaDigests — the group string is
// inside the digested bytes, so renaming the group re-identifies every member
// whether or not its contract moved. Three contracts actually changed. Twelve
// did not, and their exact FormRefs became unresolvable anyway.
//
// Nothing recorded which was which. A client author holding twelve recorded
// FormRefs could not tell whether they were looking at the same contract under
// a new name or at twelve contracts to re-read, and the repository knew the
// answer and did not say it.
//
// spec/versioning.md already gives that answer a job: "an additive minor MAY
// share one codec with the definitions before it", "a breaking major MUST have
// its own codec". This is the same question asked across a group rename.
//
// The classification is DERIVED, never hand-written: each Form's definition is
// compared before and after with version words normalised, so a Form counts as
// changed only when something other than its own identity moved. The old side
// lives in Git history, so --check says so plainly where Git is absent rather
// than reporting a comparison it did not make.

import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/form-contract-continuity.mjs --write|--check");
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const ledgerPath = path.join(
  repositoryRoot,
  "release",
  "form-contract-continuity.json",
);
const LEDGER_KIND = "takoform.form-contract-continuity@v1";

function git(...args) {
  try {
    return execFileSync("git", args, {
      cwd: repositoryRoot,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
      stdio: ["ignore", "pipe", "ignore"],
    });
  } catch {
    return undefined;
  }
}

function canonical(raw) {
  const sortDeep = (value) =>
    Array.isArray(value)
      ? value.map(sortDeep)
      : value !== null && typeof value === "object"
        ? Object.fromEntries(
            Object.keys(value)
              .sort()
              .map((key) => [key, sortDeep(value[key])]),
          )
        : value;
  return JSON.stringify(sortDeep(JSON.parse(raw))).replace(
    /v1(?:alpha|beta)\d+/gu,
    "vN",
  );
}

function classify(move) {
  const fromSet = git("show", `${move.fromCommit}:${move.fromTree}/candidate-set.json`);
  if (fromSet === undefined) return undefined;
  const previous = JSON.parse(fromSet);
  const current = JSON.parse(
    readFileSync(path.join(repositoryRoot, move.toTree, "candidate-set.json"), "utf8"),
  );
  const previousSlugs = new Map(
    previous.forms.map((form) => [form.formRef.kind, form.path.split("/").at(-1)]),
  );

  const contractChanged = [];
  const reIdentifiedOnly = [];
  const absentBefore = [];
  for (const form of current.forms) {
    const kind = form.formRef.kind;
    const slug = previousSlugs.get(kind);
    if (slug === undefined) {
      absentBefore.push(kind);
      continue;
    }
    const before = git("show", `${move.fromCommit}:${move.fromTree}/${slug}/definition.json`);
    const after = readFileSync(
      path.join(repositoryRoot, form.path, "definition.json"),
      "utf8",
    );
    if (before === undefined) {
      absentBefore.push(kind);
      continue;
    }
    (canonical(before) === canonical(after) ? reIdentifiedOnly : contractChanged).push(kind);
  }
  const sorted = (list) => [...list].sort();
  return {
    contractChanged: sorted(contractChanged),
    reIdentifiedOnly: sorted(reIdentifiedOnly),
    ...(absentBefore.length > 0 ? { absentBefore: sorted(absentBefore) } : {}),
  };
}

const ledger = JSON.parse(readFileSync(ledgerPath, "utf8"));
if (ledger?.kind !== LEDGER_KIND) {
  throw new Error(`${ledgerPath}: ledger kind is not ${LEDGER_KIND}`);
}

const problems = [];
let derivedAny = false;
for (const move of ledger.moves) {
  const derived = classify(move);
  if (derived === undefined) {
    continue;
  }
  derivedAny = true;
  if (mode === "--write") {
    Object.assign(move, derived);
    continue;
  }
  for (const field of ["contractChanged", "reIdentifiedOnly", "absentBefore"]) {
    const recorded = JSON.stringify(move[field] ?? undefined);
    const actual = JSON.stringify(derived[field] ?? undefined);
    if (recorded !== actual) {
      problems.push(
        `${move.from} -> ${move.to}: ${field} records ${recorded ?? "nothing"} ` +
          `but the definitions say ${actual ?? "nothing"}`,
      );
    }
  }
}

if (mode === "--write") {
  writeFileSync(ledgerPath, `${JSON.stringify(ledger, null, 2)}\n`);
  for (const move of ledger.moves) {
    process.stdout.write(
      `${move.from} -> ${move.to}: ` +
        `${move.contractChanged?.length ?? 0} contract(s) changed, ` +
        `${move.reIdentifiedOnly?.length ?? 0} re-identified only\n`,
    );
  }
  process.exit(0);
}

if (problems.length > 0) {
  process.stderr.write(
    `Form contract continuity disagrees with the definitions in ${problems.length} place(s):\n`,
  );
  for (const problem of problems) process.stderr.write(`- ${problem}\n`);
  process.stderr.write(
    "\nThe classification is derived, not authored; run " +
      "bun run sync:form-contract-continuity.\n",
  );
  process.exit(1);
}

process.stdout.write(
  derivedAny
    ? `form contract continuity OK: ${ledger.moves.length} generation move(s) match the definitions\n`
    : "form contract continuity: no committed history here, so no move was re-derived\n",
);

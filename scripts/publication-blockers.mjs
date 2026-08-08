// publication-blockers.mjs — the machine authority for the v1alpha3 publication
// freeze.
//
//   bun scripts/publication-blockers.mjs --check
//
// spec/publication-freeze.md names the blockers that must close before this
// lane publishes anything. Prose cannot stop a release, so the same facts live
// here as data. A blocker closes by naming evidence that exists, never by
// editing a status field alone.
//
// The freeze is scoped to this lane's own artifacts. It is deliberately NOT
// wired into the shared release-owner gate: that gate also serves the retained
// v1alpha1/v1alpha2 packages and the append-only security revocation path, and
// an urgent revocation for an already-published package must never wait on this
// lane's product readiness. What the check enforces instead is the state that
// would exist if the lane had published: an unpublished candidate set and an
// empty family lifecycle.

import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

export const BLOCKER_LEDGER = "spec/publication-blockers.json";
const LEDGER_FORMAT = "takoform.publication-blockers@v1";
const PRIORITIES = new Set(["P0", "P1", "P2"]);
const STATUSES = new Set(["open", "closed"]);
const BLOCKER_ID = /^V3-[0-9]{3}$/;

function fail(message) {
  throw new Error(message);
}

/**
 * parseBlockerLedger decodes the ledger and rejects a shape that could make a
 * blocker unenforceable: an unknown status, a closed blocker with no evidence,
 * a duplicate identity, or an entry that names no affected Form.
 */
export function parseBlockerLedger(document, repositoryRoot = null) {
  if (document === null || typeof document !== "object" || Array.isArray(document)) {
    fail(`${BLOCKER_LEDGER} must be a JSON object`);
  }
  if (document.format !== LEDGER_FORMAT) {
    fail(`${BLOCKER_LEDGER} format = ${JSON.stringify(document.format)}, want ${LEDGER_FORMAT}`);
  }
  for (const field of ["lane", "family", "policy"]) {
    if (typeof document[field] !== "string" || document[field].length === 0) {
      fail(`${BLOCKER_LEDGER} must state a non-empty ${field}`);
    }
  }
  if (!Array.isArray(document.blockers) || document.blockers.length === 0) {
    fail(`${BLOCKER_LEDGER} must list at least one blocker`);
  }

  const seen = new Set();
  const blockers = document.blockers.map((blocker, index) => {
    const at = `${BLOCKER_LEDGER} blocker ${index}`;
    if (blocker === null || typeof blocker !== "object" || Array.isArray(blocker)) {
      fail(`${at} must be an object`);
    }
    const keys = Object.keys(blocker).sort().join(",");
    const withIssue = "affectedForms,evidence,id,issue,priority,rationale,status,title";
    const withoutIssue = "affectedForms,evidence,id,priority,rationale,status,title";
    if (keys !== withIssue && keys !== withoutIssue) {
      fail(`${at} has fields ${keys}; a blocker carries id, title, priority, affectedForms, status, rationale, evidence and an optional issue`);
    }
    if (typeof blocker.id !== "string" || !BLOCKER_ID.test(blocker.id)) {
      fail(`${at} id must match V3-NNN`);
    }
    if (seen.has(blocker.id)) {
      fail(`${at} repeats blocker ${blocker.id}`);
    }
    seen.add(blocker.id);
    if (typeof blocker.title !== "string" || blocker.title.length === 0) {
      fail(`${blocker.id} must state a title`);
    }
    if (typeof blocker.rationale !== "string" || blocker.rationale.length === 0) {
      fail(`${blocker.id} must state why it blocks publication`);
    }
    if (!PRIORITIES.has(blocker.priority)) {
      fail(`${blocker.id} priority = ${JSON.stringify(blocker.priority)}, want one of P0, P1, P2`);
    }
    if (!STATUSES.has(blocker.status)) {
      fail(`${blocker.id} status = ${JSON.stringify(blocker.status)}, want open or closed`);
    }
    if (!Array.isArray(blocker.affectedForms) || blocker.affectedForms.length === 0) {
      fail(`${blocker.id} must name the Forms it affects, or "*" for every Form`);
    }
    if (!Array.isArray(blocker.evidence)) {
      fail(`${blocker.id} evidence must be an array`);
    }
    // A status field alone is a promise. Closing a blocker means naming what
    // was actually demonstrated, so the ledger cannot be closed by editing one
    // word — nor by naming evidence that does not exist. An unchecked list
    // would be the same unverified status edit wearing an array.
    for (const [position, entry] of blocker.evidence.entries()) {
      if (typeof entry !== "string" || entry.trim() !== entry || entry.length === 0) {
        fail(`${blocker.id} evidence[${position}] must be a non-empty path or https URL with no surrounding whitespace`);
      }
      if (entry.startsWith("https://")) {
        continue;
      }
      if (entry.startsWith("/") || entry.includes("..") || entry.includes("\\")) {
        fail(`${blocker.id} evidence[${position}] must be a repository-relative path or an https URL, not ${JSON.stringify(entry)}`);
      }
      if (repositoryRoot !== null && !existsSync(path.join(repositoryRoot, entry))) {
        fail(`${blocker.id} evidence[${position}] names ${JSON.stringify(entry)}, which does not exist; a blocker closes on evidence that can be read`);
      }
    }
    if (blocker.status === "closed" && blocker.evidence.length === 0) {
      fail(`${blocker.id} is closed but names no evidence; a blocker closes by evidence, not by editing its status`);
    }
    if (blocker.issue !== undefined && (!Number.isInteger(blocker.issue) || blocker.issue <= 0)) {
      fail(`${blocker.id} issue must be a positive integer when present`);
    }
    return blocker;
  });

  return { lane: document.lane, family: document.family, policy: document.policy, blockers };
}

export function loadBlockerLedger(repositoryRoot) {
  const file = path.join(repositoryRoot, BLOCKER_LEDGER);
  return parseBlockerLedger(JSON.parse(readFileSync(file, "utf8")), repositoryRoot);
}

/** openBlockers returns every blocker that still forbids publication. */
export function openBlockers(ledger) {
  return ledger.blockers.filter((blocker) => blocker.status === "open");
}

/**
 * assertPublicationAllowed is what a release path calls. It refuses while any
 * blocker is open, and names them, so the refusal is actionable rather than a
 * bare exit code.
 */
export function assertPublicationAllowed(ledger) {
  const open = openBlockers(ledger);
  if (open.length === 0) {
    return;
  }
  const lines = open.map((blocker) => `  ${blocker.id} [${blocker.priority}] ${blocker.title}`);
  fail(
    `the ${ledger.lane} lane is publication-frozen: ${open.length} blocker${open.length === 1 ? "" : "s"} open\n` +
      `${lines.join("\n")}\n` +
      `see ${BLOCKER_LEDGER} and spec/publication-freeze.md`,
  );
}

/**
 * assertLaneStillUnpublished is the half of the freeze with teeth today. A
 * status field says what someone intended; these two files say what the
 * repository actually did. While a blocker is open, the family candidate set
 * must still declare itself unpublished and no family Form may hold a lifecycle
 * record, because either would mean the lane published while frozen.
 */
export function assertLaneStillUnpublished(repositoryRoot, ledger, open) {
  if (open.length === 0) {
    return;
  }
  const candidateSet = JSON.parse(
    readFileSync(path.join(repositoryRoot, "forms/candidates/edge/v1alpha1/candidate-set.json"), "utf8"),
  );
  if (candidateSet.publicationStatus !== "unpublished") {
    fail(
      `the family candidate set declares publicationStatus ${JSON.stringify(candidateSet.publicationStatus)} ` +
        `while ${open.length} publication blocker${open.length === 1 ? " is" : "s are"} open`,
    );
  }
  const lifecycle = JSON.parse(readFileSync(path.join(repositoryRoot, "forms/lifecycle.json"), "utf8"));
  const familyPrefix = `${ledger.family.split("/")[0]}/`;
  const published = (lifecycle.currentForms ?? []).filter((record) =>
    String(record?.formRef?.apiVersion ?? "").startsWith(familyPrefix),
  );
  if (published.length > 0) {
    const names = published.map((record) => `${record.formRef.kind}@${record.formRef.definitionVersion}`);
    fail(
      `${names.join(", ")} hold a lifecycle record while ${open.length} publication blocker` +
        `${open.length === 1 ? " is" : "s are"} open; the ${ledger.lane} lane is frozen`,
    );
  }
}

function main() {
  const repositoryRoot = path.resolve(import.meta.dirname, "..");
  const mode = process.argv[2] ?? "--check";
  const ledger = loadBlockerLedger(repositoryRoot);
  if (mode === "--assert-publishable") {
    assertPublicationAllowed(ledger);
    console.log(`publication blockers: none open; the ${ledger.lane} lane may publish`);
    return;
  }
  if (mode !== "--check") {
    fail(`usage: bun scripts/publication-blockers.mjs [--check|--assert-publishable]`);
  }
  const open = openBlockers(ledger);
  assertLaneStillUnpublished(repositoryRoot, ledger, open);
  const byPriority = new Map();
  for (const blocker of open) {
    byPriority.set(blocker.priority, (byPriority.get(blocker.priority) ?? 0) + 1);
  }
  const summary = [...byPriority.entries()].sort().map(([priority, count]) => `${priority}=${count}`).join(" ");
  console.log(
    `publication blockers OK: ${ledger.blockers.length} recorded, ${open.length} open (${summary || "none"}); ` +
      `${ledger.lane} stays frozen`,
  );
}

if (import.meta.main) {
  try {
    main();
  } catch (error) {
    console.error(String(error instanceof Error ? error.message : error));
    process.exitCode = 1;
  }
}

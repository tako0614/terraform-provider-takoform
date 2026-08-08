// publication-blockers.mjs — the machine authority for the v1alpha3 publication
// freeze.
//
//   bun scripts/publication-blockers.mjs --check
//
// spec/publication-freeze.md names the blockers that must close before this
// lane publishes anything. Prose cannot stop a release, so the same facts live
// here as data, and the release-owner gate reads them. A blocker closes by
// naming its evidence, never by editing a status field alone.

import { readFileSync } from "node:fs";
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
export function parseBlockerLedger(document) {
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
    // word.
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
  return parseBlockerLedger(JSON.parse(readFileSync(file, "utf8")));
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

// publication-blockers.mjs — the machine authority for scoped Beta release
// policy.
//
//   bun scripts/publication-blockers.mjs --check
//
// spec/publication-freeze.md names later Stable/GA qualification obligations.
// They still block Form Package and public-service publication, but they do not
// block provider v2.1.1 from carrying the locally proven Beta contracts.
//
// The check therefore proves two separate facts: provider v2.1's exact Beta
// identity set is coherent, while the Form Package candidate set remains
// unpublished until the stricter obligations close. An urgent revocation for
// an already-published package also never waits on these obligations.

import { execFileSync } from "node:child_process";
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
 * mergedPullRequestNumbers reads the numbers this repository's own history
 * records as PULL REQUESTS, from the `(#N)` suffix the squash-merge convention
 * puts on every landed commit subject.
 *
 * It exists because `issue` cannot be validated as an issue offline, and a
 * network call from `bun run check` is not something this repository does.
 * GitHub draws issues and pull requests from ONE number sequence, so a blocker
 * naming a pull request number is a mistake nothing notices: `gh issue view
 * 116` resolves the pull request and reads as a confirmation. That is exactly
 * how V3-012 came to record 116 — the number of the pull request that landed
 * the preceding ledger edit — instead of its real issue, 121.
 *
 * What this cannot do is confirm that a number IS an issue; nothing offline
 * can. What it can do is REFUTE, and soundly: a number the history records as a
 * pull request is, by the shared sequence, not an issue. Incomplete and sound
 * beats absent, and it costs one local `git log`.
 *
 * A history that cannot be read yields null rather than an empty set, so the
 * caller reports that the refutation did not run instead of reporting a clean
 * result it did not earn. CI checks this repository out at full depth, so the
 * gate that guards a merge always runs it.
 */
function mergedPullRequestNumbers(repositoryRoot) {
  let log;
  try {
    log = execFileSync("git", ["log", "--format=%s", "HEAD"], {
      cwd: repositoryRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
      maxBuffer: 64 * 1024 * 1024,
    });
  } catch {
    return null;
  }
  const numbers = new Map();
  for (const subject of log.split("\n")) {
    const match = /\(#([0-9]+)\)\s*$/u.exec(subject);
    if (match !== null && !numbers.has(Number(match[1]))) {
      numbers.set(Number(match[1]), subject.trim());
    }
  }
  return numbers.size === 0 ? null : numbers;
}

/**
 * parseBlockerLedger decodes the ledger and rejects a shape that could make a
 * blocker unenforceable: an unknown status, a closed blocker with no evidence,
 * a duplicate identity, or an entry that names no affected Form.
 */
export function parseBlockerLedger(document, repositoryRoot = null, knownPullRequests = undefined) {
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
  const issues = new Map();
  // knownPullRequests lets a caller supply the refutation set instead of
  // reading this repository's history. A test needs that: the history is
  // ambient, and the one environment where it is absent — the deploy's frozen
  // git-archive snapshot — is exactly where an ambient read cannot be asserted
  // against.
  const pullRequests =
    knownPullRequests !== undefined
      ? knownPullRequests
      : repositoryRoot === null
        ? null
        : mergedPullRequestNumbers(repositoryRoot);
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
    if (blocker.issue !== undefined) {
      if (!Number.isInteger(blocker.issue) || blocker.issue <= 0) {
        fail(`${blocker.id} issue must be a positive integer when present`);
      }
      // One issue tracks one blocker. Two blockers on one number is the same
      // copy-paste that puts the wrong number there in the first place, and it
      // makes the ledger's traceability a coincidence.
      if (issues.has(blocker.issue)) {
        fail(
          `${blocker.id} names issue #${blocker.issue}, which ${issues.get(blocker.issue)} already tracks; ` +
            `one issue tracks one blocker`,
        );
      }
      issues.set(blocker.issue, blocker.id);
      const landed = pullRequests?.get(blocker.issue);
      if (landed !== undefined) {
        fail(
          `${blocker.id} names issue #${blocker.issue}, but this repository's history records #${blocker.issue} ` +
            `as a pull request: ${JSON.stringify(landed)}. GitHub numbers issues and pull requests from one ` +
            `sequence, so \`gh issue view ${blocker.issue}\` resolves that pull request and the mistake reads as ` +
            `a confirmation. Name the issue's own number.`,
        );
      }
    }
    return blocker;
  });

  return {
    lane: document.lane,
    family: document.family,
    policy: document.policy,
    blockers,
    // How many pull-request numbers the offline refutation had to work with, or
    // null when the history could not be read. The caller reports it rather
    // than claiming a clean result the check did not earn.
    pullRequestNumbersKnown: pullRequests === null ? null : pullRequests.size,
  };
}

export function loadBlockerLedger(repositoryRoot) {
  const file = path.join(repositoryRoot, BLOCKER_LEDGER);
  return parseBlockerLedger(JSON.parse(readFileSync(file, "utf8")), repositoryRoot);
}

/** openBlockers returns every obligation still blocking package/service publication. */
export function openBlockers(ledger) {
  return ledger.blockers.filter((blocker) => blocker.status === "open");
}

/**
 * assertPublicationAllowed is the strict Form Package/public-service gate.
 * Provider publication deliberately does not call it.
 */
export function assertPublicationAllowed(ledger) {
  const open = openBlockers(ledger);
  if (open.length === 0) {
    return;
  }
  const lines = open.map((blocker) => `  ${blocker.id} [${blocker.priority}] ${blocker.title}`);
  fail(
    `Form Package/public-service publication for ${ledger.lane} is blocked: ${open.length} obligation${open.length === 1 ? "" : "s"} open\n` +
      `${lines.join("\n")}\n` +
      `see ${BLOCKER_LEDGER} and spec/publication-freeze.md`,
  );
}

/**
 * assertLaneStillUnpublished keeps the package publication claim honest while
 * stricter obligations remain. Experimental Form maturity and provider
 * embedding are independent and are therefore allowed.
 */
export function assertLaneStillUnpublished(repositoryRoot, ledger, open) {
  if (open.length === 0) {
    return;
  }
  const candidateSet = JSON.parse(
    readFileSync(path.join(repositoryRoot, "forms/candidates/edge/v1beta2/candidate-set.json"), "utf8"),
  );
  if (candidateSet.publicationStatus !== "unpublished") {
    fail(
      `the family candidate set declares publicationStatus ${JSON.stringify(candidateSet.publicationStatus)} ` +
        `while ${open.length} publication blocker${open.length === 1 ? " is" : "s are"} open`,
    );
  }
}

// assertProviderReleaseCandidate proves the provider-first lane without using
// package/GA obligations as a proxy. It locks v2.1.1 to the 15 Experimental
// Beta FormRefs and package digests recorded in the append-only provider
// identity ledger. Candidate-only is a publication fact, not a prerelease
// version: the owner deploy changes it only after the real release exists.
//
// The two counts below are different facts and only one of them moves: the
// published release embeds the fifteen identities it shipped with, forever,
// while the current candidate lane carries whatever the generation now
// declares (decision 0046).
export function assertProviderReleaseCandidate(repositoryRoot) {
  const descriptor = JSON.parse(
    readFileSync(path.join(repositoryRoot, "release/version.json"), "utf8"),
  );
  const candidate = JSON.parse(
    readFileSync(
      path.join(repositoryRoot, "forms/candidates/edge/v1beta2/candidate-set.json"),
      "utf8",
    ),
  );
  const identities = JSON.parse(
    readFileSync(path.join(repositoryRoot, "release/provider-form-identities.json"), "utf8"),
  );
  if (
    descriptor.version !== "2.1.1" ||
    descriptor.tag !== "v2.1.1" ||
    descriptor.publicationStatus !== "candidate-only" ||
    descriptor.versioning?.portableApiVersion !== "forms.takoform.com/v1beta1"
  ) {
    fail("provider release descriptor is not candidate-only v2.1.1 on Host API v1beta1");
  }
  if (
    candidate.family !== "edge.forms.takoform.com/v1beta2" ||
    candidate.formMaturity !== "experimental" ||
    candidate.packageApiVersion !== "packages.forms.takoform.com/v1alpha4" ||
    candidate.publicationStatus !== "unpublished" ||
    candidate.forms?.length !== 17
  ) {
    fail("the current candidate lane must carry exactly 17 Experimental Beta Forms in v1alpha4 package envelopes");
  }
  const embedded = identities.releases?.find(
    (entry) => entry.providerVersion === descriptor.version,
  );
  // The catalog has moved past the published release (decision 0046): the
  // 2.1.1 ledger entry stays frozen at its own family and byte-exact set,
  // while the current candidate lane awaits the next release's entry. What
  // publication still requires of the PUBLISHED release is that its entry
  // exists, names its own lane, and was never edited.
  if (
    identities.format !== "takoform.provider-form-identities@v1" ||
    embedded?.portableApiVersion !== descriptor.versioning.portableApiVersion ||
    embedded?.family !== "edge.forms.takoform.com/v1beta1" ||
    embedded?.formMaturity !== candidate.formMaturity ||
    embedded?.forms?.length !== 15
  ) {
    fail("provider v2.1 embedded Beta FormRefs/digests drifted from the generated family set");
  }
  return {
    formCount: embedded.forms.length,
    candidateFormCount: candidate.forms.length,
    version: descriptor.version,
  };
}

/**
 * summarizeTraceability states what the offline refutation actually covered.
 * A run with no readable history says so rather than reporting a clean result
 * it did not earn — the deploy publishes from a frozen git-archive snapshot, so
 * that is a real environment and not a hypothetical one.
 */
export function summarizeTraceability(ledger) {
  return ledger.pullRequestNumbersKnown === null
    ? "no git history here, so no issue number was refuted"
    : `${ledger.pullRequestNumbersKnown} pull-request numbers refuted against`;
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
  const provider = assertProviderReleaseCandidate(repositoryRoot);
  const byPriority = new Map();
  for (const blocker of open) {
    byPriority.set(blocker.priority, (byPriority.get(blocker.priority) ?? 0) + 1);
  }
  const summary = [...byPriority.entries()].sort().map(([priority, count]) => `${priority}=${count}`).join(" ");
  const traceability = summarizeTraceability(ledger);
  console.log(
    `Beta release policy OK: provider v${provider.version} locks ${provider.formCount} exact Experimental Forms ` +
      `and the current candidate lane carries ${provider.candidateFormCount}; ` +
      `${open.length} later package/Stable/GA obligation${open.length === 1 ? "" : "s"} remain (${summary || "none"}); ${traceability}`,
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

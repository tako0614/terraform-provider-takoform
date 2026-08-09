import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  assertPublicationAllowed,
  loadBlockerLedger,
  openBlockers,
  parseBlockerLedger,
  assertLaneStillUnpublished,
} from "./publication-blockers.mjs";

const repositoryRoot = path.resolve(import.meta.dirname, "..");

function blocker(overrides = {}) {
  return {
    id: "V3-001",
    title: "A real host preserves the exact FormRef",
    priority: "P0",
    affectedForms: ["*"],
    status: "open",
    rationale: "Only the disposable reference host implements the lane.",
    evidence: [],
    ...overrides,
  };
}

function ledger(blockers) {
  return {
    format: "takoform.publication-blockers@v1",
    lane: "forms.takoform.com/v1alpha3",
    family: "edge.forms.takoform.com/v1alpha1",
    policy: "frozen while any blocker is open",
    blockers,
  };
}

describe("the committed ledger", () => {
  test("parses and still forbids publication", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    expect(parsed.blockers.length).toBeGreaterThan(0);
    expect(openBlockers(parsed).length).toBeGreaterThan(0);
    expect(() => assertPublicationAllowed(parsed)).toThrow(/publication-frozen/);
  });

  test("names every open blocker in the refusal, so the message is actionable", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    let message = "";
    try {
      assertPublicationAllowed(parsed);
    } catch (error) {
      message = String(error.message);
    }
    for (const open of openBlockers(parsed)) {
      expect(message).toContain(open.id);
      expect(message).toContain(open.title);
    }
  });
});

describe("closing a blocker", () => {
  // The point of the ledger is that a release cannot be unblocked by editing
  // one word. Closing means naming what was demonstrated.
  test("requires evidence", () => {
    expect(() => parseBlockerLedger(ledger([blocker({ status: "closed" })]))).toThrow(
      /closes by evidence/,
    );
  });

  test("is accepted once evidence is named", () => {
    const parsed = parseBlockerLedger(
      ledger([blocker({ status: "closed", evidence: ["conformance/real-host/report.json"] })]),
    );
    expect(openBlockers(parsed)).toHaveLength(0);
    expect(() => assertPublicationAllowed(parsed)).not.toThrow();
  });
});

describe("shapes that would make a blocker unenforceable", () => {
  test("an unknown status is refused", () => {
    expect(() => parseBlockerLedger(ledger([blocker({ status: "in-progress" })]))).toThrow(/status/);
  });

  test("a duplicate identity is refused", () => {
    expect(() => parseBlockerLedger(ledger([blocker(), blocker()]))).toThrow(/repeats blocker/);
  });

  test("a blocker affecting nothing is refused", () => {
    expect(() => parseBlockerLedger(ledger([blocker({ affectedForms: [] })]))).toThrow(
      /must name the Forms/,
    );
  });

  test("an unexpected field is refused", () => {
    expect(() => parseBlockerLedger(ledger([{ ...blocker(), waived: true }]))).toThrow(/has fields/);
  });

  test("an empty ledger is refused", () => {
    expect(() => parseBlockerLedger(ledger([]))).toThrow(/at least one blocker/);
  });
});

describe("evidence must be real, not decorative", () => {
  const bad = [null, "", "   ", " a ", "/etc/passwd", "../outside", "does/not/exist.json"];
  for (const entry of bad) {
    test(`${JSON.stringify(entry)} does not close a blocker`, () => {
      expect(() =>
        parseBlockerLedger(
          ledger([blocker({ status: "closed", evidence: [entry] })]),
          repositoryRoot,
        ),
      ).toThrow();
    });
  }

  test("an existing repository path is accepted", () => {
    const parsed = parseBlockerLedger(
      ledger([blocker({ status: "closed", evidence: ["spec/publication-freeze.md"] })]),
      repositoryRoot,
    );
    expect(() => assertPublicationAllowed(parsed)).not.toThrow();
  });

  test("an https URL is accepted without a filesystem read", () => {
    const parsed = parseBlockerLedger(
      ledger([blocker({ status: "closed", evidence: ["https://example.invalid/run/1"] })]),
      repositoryRoot,
    );
    expect(openBlockers(parsed)).toHaveLength(0);
  });
});

describe("an issue number traces to an issue", () => {
  // GitHub draws issues and pull requests from ONE sequence, so a blocker that
  // records a pull request number reads as correct: `gh issue view 116`
  // resolves the pull request and answers. Nothing offline can confirm a
  // number IS an issue, but the repository's own history can REFUTE one: a
  // number it landed as a pull request is not an issue.
  test("a number this repository landed as a pull request is refused", () => {
    expect(() =>
      parseBlockerLedger(ledger([blocker({ issue: 116 })]), repositoryRoot),
    ).toThrow(/records #116 as a pull request/);
  });

  test("the committed ledger names no pull request number", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    expect(parsed.pullRequestNumbersKnown).toBeGreaterThan(0);
    for (const entry of parsed.blockers) {
      expect(typeof entry.issue).toBe("number");
    }
  });

  test("two blockers may not name one issue", () => {
    expect(() =>
      parseBlockerLedger(
        ledger([blocker({ id: "V3-001", issue: 900001 }), blocker({ id: "V3-002", issue: 900001 })]),
      ),
    ).toThrow(/one issue tracks one blocker/);
  });

  test("a number no commit landed is accepted", () => {
    const parsed = parseBlockerLedger(ledger([blocker({ issue: 900002 })]), repositoryRoot);
    expect(parsed.blockers[0].issue).toBe(900002);
  });

  test("a non-positive issue is still refused", () => {
    expect(() => parseBlockerLedger(ledger([blocker({ issue: 0 })]))).toThrow(/positive integer/);
  });
});

describe("the freeze stays scoped to this lane", () => {
  // The shared owner gate also serves the retained packages and the append-only
  // revocation path. An urgent revocation must never wait on this lane.
  test("the release-owner gate does not assert publishability", () => {
    const manifest = JSON.parse(
      readFileSync(path.join(repositoryRoot, "package.json"), "utf8"),
    );
    expect(manifest.scripts["check:release-owner-gate"]).not.toContain("assert:publishable");
  });

  test("the lane's own artifacts are what the check enforces", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    expect(() =>
      assertLaneStillUnpublished(repositoryRoot, parsed, openBlockers(parsed)),
    ).not.toThrow();
  });
});

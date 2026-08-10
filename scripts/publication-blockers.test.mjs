import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  assertPublicationAllowed,
  assertProviderReleaseCandidate,
  loadBlockerLedger,
  openBlockers,
  parseBlockerLedger,
  assertLaneStillUnpublished,
  summarizeTraceability,
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
    lane: "forms.takoform.com/v1beta1",
    family: "edge.forms.takoform.com/v1beta1",
    policy: "frozen while any blocker is open",
    blockers,
  };
}

describe("the committed ledger", () => {
  test("parses and still forbids Form Package/public-service publication", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    expect(parsed.blockers.length).toBeGreaterThan(0);
    expect(openBlockers(parsed).length).toBeGreaterThan(0);
    expect(() => assertPublicationAllowed(parsed)).toThrow(/publication.*blocked/);
  });

  test("allows the exact provider v2.1 Beta candidate independently", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    expect(openBlockers(parsed).length).toBeGreaterThan(0);
    expect(assertProviderReleaseCandidate(repositoryRoot)).toEqual({
      formCount: 15,
      version: "2.1.0",
    });
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
  // The refutation set is supplied rather than read from this repository's own
  // history, so the assertion means the same thing everywhere. The one place
  // the history is absent is the deploy's frozen git-archive snapshot, which is
  // precisely where a test written against the ambient history cannot hold.
  const landedAsPullRequests = new Map([
    [116, "Give V3-011 the issue every other blocker has (#116)"],
  ]);

  test("a number this repository landed as a pull request is refused", () => {
    expect(() =>
      parseBlockerLedger(ledger([blocker({ issue: 116 })]), repositoryRoot, landedAsPullRequests),
    ).toThrow(/records #116 as a pull request/);
  });

  test("a number no commit subject claims is left alone", () => {
    const parsed = parseBlockerLedger(
      ledger([blocker({ issue: 121 })]),
      repositoryRoot,
      landedAsPullRequests,
    );
    expect(parsed.pullRequestNumbersKnown).toBe(1);
  });

  test("the committed ledger names no number this history landed as a pull request", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    for (const entry of parsed.blockers) {
      expect(typeof entry.issue).toBe("number");
    }
    // Where the history is readable the refutation ran and the ledger survived
    // it. Where it is not — the deploy's frozen snapshot — the contract is that
    // the validator reports it did not run rather than reporting a clean
    // result it did not earn. Both are assertions; neither is a skip.
    if (parsed.pullRequestNumbersKnown === null) {
      expect(summarizeTraceability(parsed)).toContain("no git history");
      return;
    }
    expect(parsed.pullRequestNumbersKnown).toBeGreaterThan(0);
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

describe("the stricter package policy stays scoped", () => {
  // The shared owner gate also serves the retained packages and the append-only
  // revocation path. An urgent revocation must never wait on this lane.
  test("the release-owner gate does not assert publishability", () => {
    const manifest = JSON.parse(
      readFileSync(path.join(repositoryRoot, "package.json"), "utf8"),
    );
    expect(manifest.scripts["check:release-owner-gate"]).not.toContain("assert:publishable");
  });

  test("the unpublished package candidate is what the open-obligation check enforces", () => {
    const parsed = loadBlockerLedger(repositoryRoot);
    expect(() =>
      assertLaneStillUnpublished(repositoryRoot, parsed, openBlockers(parsed)),
    ).not.toThrow();
  });
});

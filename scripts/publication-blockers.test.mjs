import { describe, expect, test } from "bun:test";
import path from "node:path";

import {
  assertPublicationAllowed,
  loadBlockerLedger,
  openBlockers,
  parseBlockerLedger,
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

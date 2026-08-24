import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  EXPECTED_CANDIDATE,
  FORM_MATURITY_EFFECT,
  LEDGER_PATH,
  PROVIDER_EFFECT,
  releaseFromEvidence,
  validateCommittedHistory,
  validateLedger,
  validateReleaseShape,
} from "./specification-release.mjs";
import {
  CONFORMANCE_SUITE_PATH,
  FAMILY_INDEX_PATH,
  SPECIFICATION_PREREQUISITES,
} from "./publication-evidence.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const LEDGER = JSON.parse(readFileSync(path.join(ROOT, LEDGER_PATH), "utf8"));

function clone(value) {
  return structuredClone(value);
}

function completeDocument() {
  return {
    candidateBaseline: {
      repository: "takoform",
      commit: "a".repeat(40),
      familyIndex: { path: FAMILY_INDEX_PATH, sha256: "b".repeat(64) },
      conformanceSuite: { path: CONFORMANCE_SUITE_PATH, sha256: "c".repeat(64) },
    },
    evidence: {
      specification: {
        sourceSnapshot: { format: "source", fileCount: 397 },
        candidateCorpus: { format: "corpus", familyCount: 8, formCount: 31 },
        referenceConformance: { format: "reference", status: "passed" },
      },
    },
  };
}

describe("Specification release ledger", () => {
  test("records an exact open candidate and no false release", () => {
    expect(validateLedger(LEDGER)).toEqual([]);
    expect(LEDGER.candidate).toEqual(EXPECTED_CANDIDATE);
    expect(LEDGER.candidate.prerequisites).toEqual(SPECIFICATION_PREREQUISITES);
    expect(LEDGER.releases).toEqual([]);
    expect(JSON.stringify(LEDGER)).not.toContain("stable-mint");
    expect(JSON.stringify(LEDGER)).not.toContain("Form 1.0.0");
  });

  test("does not let Provider, external Host, production, or signer evidence replace a prerequisite", () => {
    for (const replacement of [
      "provider-v3-exact-conformance",
      "two-independent-hosts",
      "production-runtime",
      "takosumi-adoption",
      "operator-signature",
    ]) {
      const changed = clone(LEDGER);
      changed.candidate.prerequisites[0] = replacement;
      expect(validateLedger(changed).join("\n")).toContain(
        "candidate must state the exact Specification 1.0 track",
      );
    }
  });

  test("rejects changing the literal v1 suite or implying Form and Provider promotion", () => {
    const lane = clone(LEDGER);
    lane.candidate.hostApiLane = "forms.takoform.com/v1beta4";
    expect(validateLedger(lane).join("\n")).toContain("exact Specification 1.0 track");

    const forms = clone(LEDGER);
    forms.candidate.formMaturityEffect = "promote-all-to-1.0.0";
    expect(validateLedger(forms).join("\n")).toContain("exact Specification 1.0 track");

    const provider = clone(LEDGER);
    provider.candidate.providerEffect = "required-provider-3.0.0";
    expect(validateLedger(provider).join("\n")).toContain("exact Specification 1.0 track");
  });

  test("derives a numbered release only from all three exact evidence objects", () => {
    expect(() => releaseFromEvidence({ candidateBaseline: {}, evidence: {} })).toThrow(
      "evidence is not complete",
    );
    const release = releaseFromEvidence(completeDocument());
    expect(validateReleaseShape(release)).toEqual([]);
    expect(release.sourceCommit).toBe("a".repeat(40));
    expect(release.familyIndex.path).toBe(FAMILY_INDEX_PATH);
    expect(release.conformanceSuite.path).toBe(CONFORMANCE_SUITE_PATH);
    expect(release.prerequisites).toEqual(SPECIFICATION_PREREQUISITES);
    expect(release.formMaturityEffect).toBe(FORM_MATURITY_EFFECT);
    expect(release.providerEffect).toBe(PROVIDER_EFFECT);
  });

  test("preserves every committed numbered release byte-for-byte and in order", () => {
    const release = releaseFromEvidence(completeDocument());
    const historical = { ...clone(LEDGER), releases: [release] };
    expect(
      validateCommittedHistory(historical, [
        { commit: "d".repeat(40), ledger: historical },
      ]),
    ).toEqual([]);

    const mutated = clone(historical);
    mutated.releases[0].title = "rewritten";
    expect(
      validateCommittedHistory(mutated, [
        { commit: "d".repeat(40), ledger: historical },
      ]).join("\n"),
    ).toContain("was mutated");

    expect(
      validateCommittedHistory(LEDGER, [
        { commit: "d".repeat(40), ledger: historical },
      ]).join("\n"),
    ).toContain("was deleted or reordered");
  });
});

test("package commands keep validation separate from the fail-closed readiness assertion", () => {
  const pkg = JSON.parse(readFileSync(path.join(ROOT, "package.json"), "utf8"));
  expect(pkg.scripts["check:specification-releases"]).toContain("--check");
  expect(pkg.scripts["check:specification-v1-release"]).toContain(
    "--assert-specification-v1",
  );
  expect(pkg.scripts["check:stable-mint"]).toBeUndefined();
});

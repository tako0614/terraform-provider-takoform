import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  C2_ALLOWED_PATHS,
  C3_ALLOWED_PATHS,
  EXPECTED_CANDIDATE,
  EXPECTED_RESERVED,
  FORM_PUBLICATION_EFFECT,
  HOST_API_EFFECT,
  LEDGER_KIND,
  LEDGER_PATH,
  PROVIDER_EFFECT,
  RELEASE_RECEIPT_FORMAT,
  SOURCE_EVIDENCE_ASSET,
  SOURCE_EVIDENCE_PATH,
  appendReleaseReceipt,
  releaseFromEvidence,
  validateC2DiffPaths,
  validateC3DiffPaths,
  validateCommittedHistory,
  validateLedger,
  validateReleaseShape,
} from "./specification-release.mjs";
import { SPECIFICATION_PREREQUISITES } from "./publication-evidence.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const sourceCommit = "a".repeat(40);
const releaseCommit = "b".repeat(40);
const tagObject = "c".repeat(40);
const sourceEvidenceSha256 = `sha256:${"d".repeat(64)}`;

function clone(value) {
  return structuredClone(value);
}

function c1Ledger() {
  return {
    kind: LEDGER_KIND,
    policy: "Specification 1.0 is withdrawn; 1.1 is create-only.",
    reserved: clone(EXPECTED_RESERVED),
    candidate: clone(EXPECTED_CANDIDATE),
    releases: [],
  };
}

function completeDocument() {
  return {
    evidence: {
      specification: {
        sourceSnapshot: {
          format: "takoform.specification-source-snapshot@v2",
          releaseVersion: "1.1",
          repository: "takoform",
          sourceCommit,
          roots: ["spec"],
          excludedPaths: [
            "spec/publication-evidence.json",
            "spec/publication-blockers.json",
          ],
          fileCount: 397,
          pathSetSha256: `sha256:${"e".repeat(64)}`,
          documentSetSha256: `sha256:${"f".repeat(64)}`,
        },
        candidateCorpus: null,
        referenceConformance: null,
      },
    },
  };
}

function exactReadback() {
  return {
    releaseCommit,
    tag: "specification/1.1",
    tagObject,
    release: {
      id: 41,
      url: "https://github.com/tako0614/terraform-provider-takoform/releases/tag/specification/1.1",
      immutable: true,
    },
    assetDigests: {
      [SOURCE_EVIDENCE_ASSET]: sourceEvidenceSha256,
    },
  };
}

function receipt() {
  return releaseFromEvidence(completeDocument(), exactReadback());
}

describe("Specification 1.1 owner ledger", () => {
  test("keeps C1 open with withdrawn 1.0 and sourceSnapshot as the sole authority", () => {
    const ledger = c1Ledger();
    expect(validateLedger(ledger)).toEqual([]);
    expect(ledger.candidate).toEqual(EXPECTED_CANDIDATE);
    expect(ledger.candidate.prerequisites).toEqual(
      SPECIFICATION_PREREQUISITES,
    );
    expect(ledger.releases).toEqual([]);
    expect(JSON.stringify(ledger)).not.toContain("compatibilityManifest");
    expect(JSON.stringify(ledger)).not.toContain("v1.1");
    expect(JSON.stringify(ledger)).not.toContain("@v2");
  });

  test("builds the C3 receipt only from source evidence plus exact live publication readback", () => {
    expect(() => releaseFromEvidence({ evidence: {} }, exactReadback())).toThrow(
      "evidence is not complete",
    );
    const release = receipt();
    expect(validateReleaseShape(release)).toEqual([]);
    expect(release).toMatchObject({
      format: RELEASE_RECEIPT_FORMAT,
      version: "1.1",
      sourceCommit,
      releaseCommit,
      sourceSnapshotSha256: expect.stringMatching(/^sha256:[0-9a-f]{64}$/),
      tag: "specification/1.1",
      tagObject,
      annotatedTag: true,
      release: {
        id: 41,
        immutable: true,
      },
      assets: [
        {
          name: SOURCE_EVIDENCE_ASSET,
          sourcePath: SOURCE_EVIDENCE_PATH,
          sha256: sourceEvidenceSha256,
        },
      ],
      hostApiEffect: HOST_API_EFFECT,
      formPublicationEffect: FORM_PUBLICATION_EFFECT,
      providerEffect: PROVIDER_EFFECT,
    });
    expect(JSON.stringify(release)).not.toContain("compatibility");
  });

  test("rejects mutable, mismatched, v2, /v1.1, and incomplete live receipt shapes", () => {
    for (const mutate of [
      (value) => (value.release.immutable = false),
      (value) => (value.tag = "specification/v1.1"),
      (value) => (value.format = "takoform.specification-release-receipt@v2"),
      (value) => (value.releaseCommit = value.sourceCommit),
      (value) => (value.assets[0].sha256 = `sha256:${"0".repeat(64)}`),
      (value) => value.assets.push(clone(value.assets[0])),
      (value) => (value.compatibilityManifestSha256 = "0".repeat(64)),
    ]) {
      const changed = receipt();
      mutate(changed);
      expect(validateReleaseShape(changed).length).toBeGreaterThan(0);
    }
  });

  test("preserves every committed receipt byte-for-byte and in order", () => {
    const historical = { ...c1Ledger(), releases: [receipt()] };
    expect(
      validateCommittedHistory(historical, [
        { commit: "d".repeat(40), ledger: historical },
      ]),
    ).toEqual([]);

    const mutated = clone(historical);
    mutated.releases[0].release.url += "?rewritten=1";
    expect(
      validateCommittedHistory(mutated, [
        { commit: "d".repeat(40), ledger: historical },
      ]).join("\n"),
    ).toContain("was mutated");

    expect(
      validateCommittedHistory(c1Ledger(), [
        { commit: "d".repeat(40), ledger: historical },
      ]).join("\n"),
    ).toContain("was deleted or reordered");
  });
});

describe("C1/C2/C3 commit fences", () => {
  test("C2 permits only the evidence record and exact static/public projections", () => {
    expect(validateC2DiffPaths(C2_ALLOWED_PATHS)).toEqual([]);
    expect(validateC2DiffPaths(["spec/publication-evidence.json"])).toEqual([]);
    for (const pathName of [
      "spec/core/README.md",
      "scripts/release-deploy.mjs",
      "release/specification-compatibility.json",
      "website/static/forms/new-v2.json",
    ]) {
      expect(
        validateC2DiffPaths([
          "spec/publication-evidence.json",
          pathName,
        ]).join("\n"),
      ).toContain("evidence-only");
    }
  });

  test("C3 permits only the append-only ledger and its exact projections", () => {
    expect(validateC3DiffPaths(C3_ALLOWED_PATHS)).toEqual([]);
    expect(
      validateC3DiffPaths([LEDGER_PATH, "scripts/release-deploy.mjs"]).join(
        "\n",
      ),
    ).toContain("receipt/ledger projection-only");
    expect(validateC3DiffPaths([LEDGER_PATH]).join("\n")).toContain(
      "static projection",
    );
  });
});

test("receipt writer is create-only and writes only the ledger projections", () => {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-spec-receipt-"));
  for (const relativePath of [
    "release",
    "spec",
    "website/static/release",
    "website/public/release",
  ]) {
    mkdirSync(path.join(root, relativePath), { recursive: true });
  }
  const ledger = c1Ledger();
  const initial = `${JSON.stringify(ledger, null, 2)}\n`;
  for (const relativePath of [
    LEDGER_PATH,
    "website/static/release/specification-releases.json",
    "website/public/release/specification-releases.json",
  ]) {
    writeFileSync(path.join(root, relativePath), initial);
  }

  const evidenceRaw = `${JSON.stringify(completeDocument(), null, 2)}\n`;
  writeFileSync(path.join(root, SOURCE_EVIDENCE_PATH), evidenceRaw);
  const writerReadback = exactReadback();
  writerReadback.assetDigests[SOURCE_EVIDENCE_ASSET] =
    `sha256:${createHash("sha256").update(evidenceRaw).digest("hex")}`;
  const writerReceipt = releaseFromEvidence(completeDocument(), writerReadback);

  const written = appendReleaseReceipt(writerReceipt, root);
  expect(written.releases).toEqual([writerReceipt]);
  const canonical = readFileSync(path.join(root, LEDGER_PATH), "utf8");
  expect(
    readFileSync(
      path.join(root, "website/static/release/specification-releases.json"),
      "utf8",
    ),
  ).toBe(canonical);
  expect(
    readFileSync(
      path.join(root, "website/public/release/specification-releases.json"),
      "utf8",
    ),
  ).toBe(canonical);
  expect(() => appendReleaseReceipt(writerReceipt, root)).toThrow("create-only");
});

test("package commands keep network-free validation separate from publication", () => {
  const pkg = JSON.parse(readFileSync(path.join(ROOT, "package.json"), "utf8"));
  expect(pkg.scripts["check:specification-releases"]).toContain("--check");
  expect(pkg.scripts["check:specification-1-1-release"]).toContain(
    "--assert-specification-1-1",
  );
  expect(pkg.scripts["check:specification-v2-release"]).toBeUndefined();
  expect(pkg.scripts["check:stable-mint"]).toBeUndefined();
});

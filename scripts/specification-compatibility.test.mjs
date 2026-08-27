import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  CLASS_IDS,
  MANIFEST_PATH,
  MANIFEST_FORMAT,
  SPECIFICATION_VERSION,
  STATUS_VALUES,
  canonicalJson,
  deriveExpectedIdentitySets,
  generateManifest,
  validateManifest,
} from "./specification-compatibility.mjs";

const ROOT = path.resolve(import.meta.dirname, "..");
const MANIFEST = JSON.parse(readFileSync(path.join(ROOT, MANIFEST_PATH), "utf8"));
const SPECIFICATION_LEDGER = JSON.parse(
  readFileSync(path.join(ROOT, "release/specification-releases.json"), "utf8"),
);

function clone(value) {
  return structuredClone(value);
}

describe("Specification 1.1 compatibility manifest", () => {
  test("is generated from exactly five classes and the four explicit statuses", () => {
    expect(MANIFEST.format).toBe(MANIFEST_FORMAT);
    expect(MANIFEST.specificationVersion).toBe(SPECIFICATION_VERSION);
    expect(MANIFEST.classes).toHaveLength(5);
    expect(MANIFEST.classes.map((entry) => entry.id)).toEqual(CLASS_IDS);
    expect(MANIFEST.statusVocabulary).toEqual(STATUS_VALUES);
    expect(validateManifest(MANIFEST, ROOT)).toBe(MANIFEST);
  });

  test("binds every entry to source bytes, a ledger, and a migration disposition", () => {
    for (const classValue of MANIFEST.classes) {
      expect(classValue.entries.length).toBeGreaterThan(0);
      for (const value of classValue.entries) {
        expect(value.sources.length).toBeGreaterThan(0);
        expect(value.owningLedger).toMatch(/\.json$|\.md$/u);
        expect(STATUS_VALUES).toContain(value.migration.kind);
      }
    }
    const statuses = new Set(MANIFEST.classes.flatMap((entry) => entry.entries.map((value) => value.status)));
    const released = SPECIFICATION_LEDGER.releases.some(
      (release) => release?.version === "1.1",
    );
    expect(statuses).toEqual(
      released
        ? new Set(["retained", "unpublished-candidate", "withdrawn-retained"])
        : new Set(STATUS_VALUES),
    );
  });

  test("contains the exact occupied identity sets derived from owning ledgers", () => {
    const expected = deriveExpectedIdentitySets(ROOT);
    for (const classValue of MANIFEST.classes) {
      expect(classValue.entries.map((value) => value.identity).sort()).toEqual(expected[classValue.id]);
    }
    const all = new Set(MANIFEST.classes.flatMap((value) => value.entries.map((entry) => entry.identity)));
    for (const identity of [
      "forms.takoform.com/v1alpha1",
      "forms.takoform.com/v1alpha2",
      "forms.takoform.com/v1alpha3",
      "packages.forms.takoform.com/v1alpha1#FormPackage",
      "packages.forms.takoform.com/v1alpha2#FormPackage",
      "packages.forms.takoform.com/v1alpha3#FormPackage",
      "packages.forms.takoform.com/v1alpha4#FormPackage",
      "packages.forms.takoform.com/v1alpha5#FormPackage",
      "edge.forms.takoform.com/v1alpha1",
      "edge.forms.takoform.com/v1beta1",
      "edge.forms.takoform.com/v1beta2",
      "artifacts.takoform.com/v1alpha1",
      "standards.takoform.com/v1alpha1",
      "trust.forms.takoform.com/v1alpha1",
      "registry.terraform.io/tako0614/takoform@1.0.1",
      "registry.terraform.io/tako0614/takoform@1.0.2",
      "registry.terraform.io/tako0614/takoform@1.0.3",
      "registry.terraform.io/tako0614/takoform@2.0.0",
      "registry.terraform.io/tako0614/takoform@2.1.1",
      "registry.terraform.io/tako0614/takoform@3.0.0",
    ]) expect(all.has(identity)).toBe(true);
    expect([...all].some((identity) => identity.includes("github.com/tako0614/takoform"))).toBe(false);
    expect([...all].some((identity) => identity.includes("artifact-transport/v1"))).toBe(false);
  });

  test("reports Specification 1.1 from its append-only receipt state", () => {
    const entry = MANIFEST.classes
      .flatMap((classValue) => classValue.entries)
      .find((value) => value.identity === "takoform.specification@1.1");
    const released = SPECIFICATION_LEDGER.releases.some(
      (release) => release?.version === "1.1",
    );
    expect(entry).toMatchObject(
      released
        ? { status: "retained", publication: "retained" }
        : { status: "new-independent", publication: "unpublished-candidate" },
    );
  });

  test("explicitly byte-pins literal Host API v1 without minting a new lane", () => {
    expect(MANIFEST.hostApiV1Pin.lane).toBe("forms.takoform.com/v1");
    expect(MANIFEST.hostApiV1Pin.status).toBe("unpublished-candidate");
    expect(MANIFEST.hostApiV1Pin.sources).toHaveLength(5);
    expect(canonicalJson(MANIFEST)).not.toContain("/v1.1");
    expect(canonicalJson(MANIFEST)).not.toContain("/v2");
  });

  test("fails closed for source digest, class, status, and host-lane drift", () => {
    const digest = clone(MANIFEST);
    digest.classes[0].entries[0].sources[0].sha256 = `sha256:${"0".repeat(64)}`;
    expect(() => validateManifest(digest, ROOT)).toThrow("digest does not match");

    const classes = clone(MANIFEST);
    classes.classes.pop();
    expect(() => validateManifest(classes, ROOT)).toThrow("exactly 5 classes");

    const status = clone(MANIFEST);
    status.classes[0].entries[0].status = "published";
    expect(() => validateManifest(status, ROOT)).toThrow("status is not recognized");

    const lane = clone(MANIFEST);
    lane.hostApiV1Pin.lane = "forms.takoform.com/v1.1";
    expect(() => validateManifest(lane, ROOT)).toThrow("byte-pin");
  });

  test("fails closed for missing and extra ledger identities", () => {
    const missing = clone(MANIFEST);
    missing.classes[0].entries.pop();
    expect(() => validateManifest(missing, ROOT)).toThrow("missing ledger identities");

    const extra = clone(MANIFEST);
    const extraEntry = clone(extra.classes[0].entries[0]);
    extraEntry.identity = "not-present-in-an-owning-ledger";
    extra.classes[0].entries.push(extraEntry);
    expect(() => validateManifest(extra, ROOT)).toThrow("not present in owning ledgers");
  });

  test("checked output is deterministic and has no hidden source mutations", () => {
    expect(canonicalJson(generateManifest(ROOT))).toBe(canonicalJson(MANIFEST));
  });
});

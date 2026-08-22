import { describe, expect, test } from "bun:test";
import { cpSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  SITE_STATUS_PUBLISHED_PATH,
  SITE_STATUS_REPOSITORY_PATH,
  verifySiteStatusDocument,
} from "./site-status.mjs";
import {
  FAMILY_CANDIDATE_SET,
  SITE_STATUS_DEPRECATED_FIELDS,
  SITE_STATUS_FIELDS,
  deriveSiteStatusFacts,
} from "../website/.vitepress/site-status.mjs";

const repositoryRoot = path.resolve(import.meta.dirname, "..");

/**
 * fixture copies only the files the derivation and the gate read, so a test can
 * corrupt one of them without touching the working tree.
 */
function fixture(mutate) {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-site-status-"));
  try {
    for (const relativePath of [
      "release/version.json",
      "release/provider-release-identities.json",
      "spec/publication-blockers.json",
      FAMILY_CANDIDATE_SET,
      SITE_STATUS_REPOSITORY_PATH,
      SITE_STATUS_PUBLISHED_PATH,
    ]) {
      cpSync(path.join(repositoryRoot, relativePath), path.join(root, relativePath));
    }
    return mutate(root);
  } finally {
    rmSync(root, { force: true, recursive: true });
  }
}

function write(root, relativePath, document) {
  writeFileSync(
    path.join(root, relativePath),
    `${JSON.stringify(document, null, 2)}\n`,
  );
}

function read(root, relativePath) {
  return JSON.parse(readFileSync(path.join(root, relativePath), "utf8"));
}

describe("the committed status document", () => {
  test("states the repository's own facts in both copies", () => {
    expect(verifySiteStatusDocument(repositoryRoot)).toEqual([]);
  });

  test("records no commit, so a fresh build can reproduce it", () => {
    const document = read(repositoryRoot, SITE_STATUS_REPOSITORY_PATH);
    expect(Object.keys(document)).toEqual(SITE_STATUS_FIELDS);
    expect(document.sourceCommit).toBeUndefined();
  });

  test("publishes independent family, definition, and package axes", () => {
    const document = read(repositoryRoot, SITE_STATUS_REPOSITORY_PATH);
    expect(document.formFamilyMaturity).toBe("beta");
    expect(document.formMaturity).toBe("experimental");
    expect(document.formPackagePublicationStatus).toBe("unpublished");
    expect(document.formFamilyMaturity).not.toBe(document.formMaturity);
    expect(document.formPackageStatus).toBe(
      document.formPackagePublicationStatus,
    );
    expect(document.edgeFamilyStatus).toBe(
      document.formPackagePublicationStatus,
    );
    expect(Object.keys(SITE_STATUS_DEPRECATED_FIELDS)).toEqual([
      "providerCurrent",
      "edgePreviewProvider",
      "edgeFamilyStatus",
      "formPackageStatus",
    ]);
  });

  test("is byte-identical in the source and the published copy", () => {
    expect(
      readFileSync(path.join(repositoryRoot, SITE_STATUS_REPOSITORY_PATH)).equals(
        readFileSync(path.join(repositoryRoot, SITE_STATUS_PUBLISHED_PATH)),
      ),
    ).toBe(true);
  });
});

describe("the gate refuses", () => {
  test("a published copy that disagrees with the repository", () => {
    const failures = fixture((root) => {
      const document = read(root, SITE_STATUS_PUBLISHED_PATH);
      write(root, SITE_STATUS_PUBLISHED_PATH, {
        ...document,
        openPublicationBlockers: document.openPublicationBlockers + 1,
      });
      return verifySiteStatusDocument(root);
    });
    expect(
      failures.some(
        (failure) =>
          failure.startsWith(`${SITE_STATUS_PUBLISHED_PATH}: openPublicationBlockers`) &&
          failure.includes("but the repository derives"),
      ),
    ).toBe(true);
  });

  test("a published copy that differs from the source copy", () => {
    const failures = fixture((root) => {
      const document = read(root, SITE_STATUS_PUBLISHED_PATH);
      // Same facts, different bytes: the build never emits this.
      writeFileSync(
        path.join(root, SITE_STATUS_PUBLISHED_PATH),
        `${JSON.stringify(document)}\n`,
      );
      return verifySiteStatusDocument(root);
    });
    expect(failures).toContain(
      `${SITE_STATUS_PUBLISHED_PATH}: bytes differ from ${SITE_STATUS_REPOSITORY_PATH}; ` +
        "the served copy is not the one the build produced; run bun run website:build",
    );
  });

  test("a document that carries a commit id again", () => {
    const failures = fixture((root) => {
      for (const relativePath of [
        SITE_STATUS_REPOSITORY_PATH,
        SITE_STATUS_PUBLISHED_PATH,
      ]) {
        write(root, relativePath, {
          sourceCommit: "0".repeat(40),
          ...read(root, relativePath),
        });
      }
      return verifySiteStatusDocument(root);
    });
    expect(
      failures.some(
        (failure) =>
          failure.startsWith(`${SITE_STATUS_PUBLISHED_PATH}: fields are sourceCommit,`) &&
          failure.includes(`want exactly ${SITE_STATUS_FIELDS.join(", ")} in that order`),
      ),
    ).toBe(true);
  });

  test("a missing published copy", () => {
    const failures = fixture((root) => {
      rmSync(path.join(root, SITE_STATUS_PUBLISHED_PATH));
      return verifySiteStatusDocument(root);
    });
    expect(failures).toContain(
      `${SITE_STATUS_PUBLISHED_PATH}: missing; run bun run website:build`,
    );
  });

  test("a providerPublished that contradicts the readback-backed derivation", () => {
    // providerPublished derives from the append-only registryReadback entries
    // in release/provider-form-identities.json; a document claiming another
    // version disagrees with that derivation and fails.
    const failures = fixture((root) => {
      for (const relativePath of [
        SITE_STATUS_REPOSITORY_PATH,
        SITE_STATUS_PUBLISHED_PATH,
      ]) {
        const document = read(root, relativePath);
        write(root, relativePath, { ...document, providerPublished: "9.9.9" });
      }
      return verifySiteStatusDocument(root);
    });
    for (const relativePath of [
      SITE_STATUS_REPOSITORY_PATH,
      SITE_STATUS_PUBLISHED_PATH,
    ]) {
      expect(
        failures.some(
          (failure) =>
            failure.startsWith(`${relativePath}: providerPublished`) &&
            failure.includes('"9.9.9"'),
        ),
      ).toBe(true);
    }
  });

  test("rejects a document that conflates family and definition maturity", () => {
    const failures = fixture((root) => {
      for (const relativePath of [
        SITE_STATUS_REPOSITORY_PATH,
        SITE_STATUS_PUBLISHED_PATH,
      ]) {
        const document = read(root, relativePath);
        write(root, relativePath, {
          ...document,
          formFamilyMaturity: "experimental",
          formMaturity: "experimental",
        });
      }
      return verifySiteStatusDocument(root);
    });
    // The document no longer has to say one frozen channel word; it has to say
    // what the repository derives. A document claiming the family is
    // Experimental therefore fails as a disagreement with the derivation,
    // which is the stronger statement and survives the family advancing.
    expect(
      failures.filter((failure) =>
        failure.includes("formFamilyMaturity = \"experimental\""),
      ),
    ).toHaveLength(2);
    expect(
      failures.filter((failure) =>
        failure.includes("formFamilyMaturity and formMaturity are conflated"),
      ),
    ).toHaveLength(2);
  });
});

describe("the derivation", () => {
  test("depends on nothing but committed bytes", () => {
    const first = deriveSiteStatusFacts(repositoryRoot);
    const second = deriveSiteStatusFacts(repositoryRoot);
    expect(first).toEqual(second);
    expect(Object.keys(first)).toEqual(SITE_STATUS_FIELDS);
  });

  test("follows the candidate set's bytes", () => {
    const changed = fixture((root) => {
      const candidateSet = read(root, FAMILY_CANDIDATE_SET);
      write(root, FAMILY_CANDIDATE_SET, {
        ...candidateSet,
        publicationStatus: "still-unpublished",
      });
      return deriveSiteStatusFacts(root);
    });
    expect(changed.formFamilyMaturity).toBe("beta");
    expect(changed.formMaturity).toBe("experimental");
    expect(changed.formPackagePublicationStatus).toBe("still-unpublished");
    expect(changed.formPackageStatus).toBe("still-unpublished");
    expect(changed.edgeFamilyStatus).toBe("still-unpublished");
    expect(changed.candidateSetDigest).not.toBe(
      deriveSiteStatusFacts(repositoryRoot).candidateSetDigest,
    );
  });

  test("rejects a candidate set that relabels definitions outside Experimental", () => {
    expect(() =>
      fixture((root) => {
        const candidateSet = read(root, FAMILY_CANDIDATE_SET);
        write(root, FAMILY_CANDIDATE_SET, {
          ...candidateSet,
          formMaturity: "beta",
        });
        return deriveSiteStatusFacts(root);
      }),
    ).toThrow("current Form definitions must be experimental");
  });
});

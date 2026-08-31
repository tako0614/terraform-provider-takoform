import { describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  cpSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  SITE_STATUS_PUBLISHED_PATH,
  SITE_STATUS_REPOSITORY_PATH,
  verifySiteStatusDocument,
} from "./site-status.mjs";
import {
  CURRENT_FAMILY_INDEX,
  FAMILY_CANDIDATE_SET,
  SITE_STATUS_DEPRECATED_FIELDS,
  SITE_STATUS_FIELDS,
  deriveSiteStatusFacts,
  prepareSiteStatus,
  renderSiteStatusDocument,
} from "../website/.vitepress/site-status.mjs";

const repositoryRoot = path.resolve(import.meta.dirname, "..");

/**
 * fixture copies only the files the derivation and the gate read, so a test can
 * corrupt one of them without touching the working tree.
 */
function fixture(mutate) {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-site-status-"));
  try {
    const currentFamilyIndex = JSON.parse(
      readFileSync(path.join(repositoryRoot, CURRENT_FAMILY_INDEX), "utf8"),
    );
    for (const relativePath of [
      "release/version.json",
      "release/provider-release-identities.json",
      "release/specification-releases.json",
      "spec/publication-blockers.json",
      FAMILY_CANDIDATE_SET,
      CURRENT_FAMILY_INDEX,
      ...currentFamilyIndex.families.map(({ candidateSet }) => candidateSet),
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

function rewriteIndexedFamilies(root, mutate) {
  const index = read(root, CURRENT_FAMILY_INDEX);
  for (const family of index.families) {
    const candidateSet = mutate(read(root, family.candidateSet));
    write(root, family.candidateSet, candidateSet);
    family.sha256 = createHash("sha256")
      .update(readFileSync(path.join(root, family.candidateSet)))
      .digest("hex");
  }
  write(root, CURRENT_FAMILY_INDEX, index);
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

  test("publishes independent definition and package axes, and no family maturity", () => {
    const document = read(repositoryRoot, SITE_STATUS_REPOSITORY_PATH);
    // The family group carries no version (decision 0049), so there is no
    // group maturity to derive: the axes that remain are the Form's own
    // maturity, the package envelope's publication status, and the Host API
    // lane — each with its own source. A group maturity would have to be read
    // off a version string that no longer exists.
    expect(document.formFamilyCurrent).toBe("edge.forms.takoform.com");
    expect(document.formFamilyCurrent).not.toMatch(/\/v\d/u);
    expect(document.formFamilyMaturity).toBeUndefined();
    expect(document.formMaturity).toBe("experimental");
    expect(document.formPackagePublicationStatus).toBe("unpublished");
    expect(document.hostApiMaturity).toBe("stable");
    expect(document.hostApiMaturity).not.toBe(document.formMaturity);
    expect(document.hostApiPublicationStatus).toBe("unpublished-candidate");
    expect(document.specificationVersion).toBe("1.1");
    const specificationLedger = read(
      repositoryRoot,
      "release/specification-releases.json",
    );
    const expectedSpecificationStatus = specificationLedger.releases.some(
      (release) => release?.version === document.specificationVersion,
    )
      ? "released"
      : "candidate-open";
    expect(document.specificationReleaseStatus).toBe(
      expectedSpecificationStatus,
    );
    expect(document.currentFamilyIndex).toBe(CURRENT_FAMILY_INDEX);
    expect(document.currentFamilyCount).toBe(8);
    expect(document.currentFormCount).toBe(31);
    expect(document.providerPublished).toBe("3.0.0");
    expect(document.providerTarget).toBe("3.0.0");
    expect(document.providerTargetStatus).toBe("registry-published");
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
      "formFamilyCurrent",
      "candidateSetDigest",
      "openPublicationBlockers",
    ]);
  });

  test("keeps current website navigation on the stable API and official Provider labels", () => {
    const config = readFileSync(
      path.join(repositoryRoot, "website/.vitepress/config.mts"),
      "utf8",
    );
    expect(config).toContain("Stable Host API v1 and current Forms");
    expect(config).toContain("Provider typed reference (official Forms)");
    expect(config).toContain("Current normative contracts");
    expect(config).toContain("Current Edge16 candidate notes");
    expect(config).toContain("Deferred / historical candidate resources");
    expect(config).toContain("Deferred / historical candidate families");
    expect(config).toContain("Form Package (wire envelope)");
    expect(config).toContain("Core contracts");
    expect(config).toContain("Historical source");
    expect(config).not.toContain("Specification 1.0 candidate / Host API v1");
    expect(config).not.toContain("Specification 1.1 / separate Host API v1 candidate");
    expect(config).not.toContain("One provider. Dependent on none.");
    expect(config).not.toContain("Provider 3 candidate reference");
  });

  test("keeps status components on one stable-wire wording and localized Japanese labels", () => {
    for (const relativePath of [
      "website/.vitepress/theme/components/StatusNote.vue",
      "website/.vitepress/theme/components/SiteFooter.vue",
    ]) {
      const source = readFileSync(path.join(repositoryRoot, relativePath), "utf8");
      expect(source).not.toContain("stable stable");
      expect(source).not.toContain("currentFamilyCount");
      expect(source).not.toContain("currentFormCount");
      expect(source).toContain("<strong>現在の契約</strong>");
      expect(source).toContain("<strong>配布境界</strong>");
    }
  });

  test("keeps every live status owner on the independent current identities", () => {
    const security = readFileSync(
      path.join(repositoryRoot, "SECURITY.md"),
      "utf8",
    );
    expect(security).toContain(
      "Provider `v3.0.0` is the current\nRegistry-published typed client",
    );
    expect(security).not.toContain(
      "Provider `v2.1.1` is the current\npublished client",
    );

    const migration = readFileSync(
      path.join(repositoryRoot, "release/migrations/v1-to-v2.md"),
      "utf8",
    );
    expect(migration).toContain("Provider 3.0.0 is the\ncurrent published provider");
    expect(migration).not.toContain(
      "v2.1.1 is the Registry-published current provider",
    );

    for (const relativePath of [
      "website/index.md",
      "website/docs/index.md",
      "website/ja/index.md",
      "website/ja/docs/index.md",
    ]) {
      const page = readFileSync(path.join(repositoryRoot, relativePath), "utf8");
      expect(page).toContain("Host API v1");
      expect(page).toContain("official");
      expect(page).not.toContain("unpublished Host API v1");
      expect(page).not.toContain("Specification 1.1 / separate Host API v1 candidate");
    }

    const decision = readFileSync(
      path.join(
        repositoryRoot,
        "spec/decisions/0057-specification-1-1-compatibility-and-independent-identities.md",
      ),
      "utf8",
    );
    expect(decision).toContain(
      "every published Provider identity from 1.0.1 through\n3.0.0 remains `retained`",
    );
    expect(decision).not.toContain(
      "withdrawn lanes and old Provider identities remain",
    );
  });

  test("is byte-identical in the source and the published copy", () => {
    expect(
      readFileSync(path.join(repositoryRoot, SITE_STATUS_REPOSITORY_PATH)).equals(
        readFileSync(path.join(repositoryRoot, SITE_STATUS_PUBLISHED_PATH)),
      ),
    ).toBe(true);
  });
});

describe("read-only site status preparation", () => {
  test("fails closed for invalid snapshot flag values", () => {
    for (const value of ["", "0", "true"]) {
      expect(() =>
        execFileSync(
          process.execPath,
          ["-e", 'import("./website/.vitepress/config.mts")'],
          {
            cwd: repositoryRoot,
            env: {
              ...process.env,
              TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY: value,
            },
            stdio: "pipe",
          },
        ),
      ).toThrow(
        'TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY must be exactly "1" when set',
      );
    }
  });

  test("leaves committed bytes and stat metadata unchanged", () => {
    fixture((root) => {
      const statusPath = path.join(root, SITE_STATUS_REPOSITORY_PATH);
      const beforeBytes = readFileSync(statusPath);
      const stableStat = () => {
        const stats = statSync(statusPath, { bigint: true });
        return {
          mode: stats.mode,
          size: stats.size,
          mtimeNs: stats.mtimeNs,
          ctimeNs: stats.ctimeNs,
        };
      };
      const beforeStat = stableStat();

      const facts = prepareSiteStatus(path.join(root, "website"), {
        write: false,
      });

      const afterStat = stableStat();
      const afterBytes = readFileSync(statusPath);
      expect(afterBytes.equals(beforeBytes)).toBe(true);
      expect(afterBytes.toString("utf8")).toBe(renderSiteStatusDocument(facts));
      expect(afterStat).toEqual(beforeStat);
    });
  });

  test("rejects stale committed bytes without replacing them", () => {
    fixture((root) => {
      const statusPath = path.join(root, SITE_STATUS_REPOSITORY_PATH);
      const staleBytes = Buffer.concat([
        readFileSync(statusPath),
        Buffer.from("stale\n"),
      ]);
      writeFileSync(statusPath, staleBytes);

      expect(() =>
        prepareSiteStatus(path.join(root, "website"), { write: false }),
      ).toThrow(/is stale: committed bytes do not match/);
      expect(readFileSync(statusPath).equals(staleBytes)).toBe(true);
    });
  });

  test("rejects a missing committed status document", () => {
    fixture((root) => {
      const statusPath = path.join(root, SITE_STATUS_REPOSITORY_PATH);
      rmSync(statusPath);

      expect(() =>
        prepareSiteStatus(path.join(root, "website"), { write: false }),
      ).toThrow(/is missing or unreadable in read-only mode/);
    });
  });
});

describe("Specification release status derivation", () => {
  test("an empty receipt ledger is candidate-open", () => {
    const status = fixture((root) => {
      const ledger = read(root, "release/specification-releases.json");
      ledger.releases = [];
      write(root, "release/specification-releases.json", ledger);
      return deriveSiteStatusFacts(root);
    });
    expect(status.specificationReleaseStatus).toBe("candidate-open");
    expect(status.hostApiPublicationStatus).toBe("unpublished-candidate");
  });

  test("an exact 1.1 receipt changes only the Specification status to released", () => {
    const status = fixture((root) => {
      const ledger = read(root, "release/specification-releases.json");
      ledger.releases = [{ version: "1.1" }];
      write(root, "release/specification-releases.json", ledger);
      return deriveSiteStatusFacts(root);
    });
    expect(status.specificationReleaseStatus).toBe("released");
    expect(status.hostApiPublicationStatus).toBe("unpublished-candidate");
    expect(status.formMaturity).toBe("experimental");
    expect(status.formPackagePublicationStatus).toBe("unpublished");
    expect(status.providerTargetStatus).toBe("registry-published");
  });
});

describe("the gate refuses", () => {
  test("an incomplete current Provider Registry readback", () => {
    const failures = fixture((root) => {
      const ledger = read(root, "release/provider-release-identities.json");
      const current = ledger.entries.find((entry) => entry.version === "3.0.0");
      current.registryReadback.installation.resourceSchemaCount = 30;
      write(root, "release/provider-release-identities.json", ledger);
      return verifySiteStatusDocument(root);
    });
    expect(
      failures.some((failure) =>
        failure.includes("Registry schema count differs from the current Form count"),
      ),
    ).toBe(true);
  });

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

  test("refuses a document that reintroduces a family maturity axis", () => {
    const failures = fixture((root) => {
      for (const relativePath of [
        SITE_STATUS_REPOSITORY_PATH,
        SITE_STATUS_PUBLISHED_PATH,
      ]) {
        const document = read(root, relativePath);
        write(root, relativePath, {
          ...document,
          formFamilyMaturity: "beta",
        });
      }
      return verifySiteStatusDocument(root);
    });
    // Before decision 0049 this was guarded by a bespoke rule naming the two
    // fields it knew could be confused. The exact-field-set rule is the
    // stronger one and needs no such list: an axis with no derivation cannot
    // appear in the document at all, whatever it is called.
    expect(
      failures.filter((failure) => failure.includes("want exactly")),
    ).toHaveLength(2);
    for (const failure of failures) {
      expect(failure).toContain("formFamilyMaturity");
    }
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
      rewriteIndexedFamilies(root, (candidateSet) => ({
        ...candidateSet,
        publicationStatus: "still-unpublished",
      }));
      return deriveSiteStatusFacts(root);
    });
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
        rewriteIndexedFamilies(root, (candidateSet) => ({
          ...candidateSet,
          formMaturity: "beta",
        }));
        return deriveSiteStatusFacts(root);
      }),
    ).toThrow("current Form definitions must be experimental");
  });
});

import { describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  C2_ALLOWED_PATHS,
  C3_ALLOWED_PATHS,
  C4_REQUIRED_PATHS,
  C4_PRESERVED_PUBLIC_PATHS,
  EXPECTED_CANDIDATE,
  EXPECTED_RESERVED,
  FORM_PUBLICATION_EFFECT,
  HOST_API_EFFECT,
  LEDGER_KIND,
  LEDGER_PATH,
  PROVIDER_EFFECT,
  RELEASE_STATE_CURRENT_PUBLIC_PATHS,
  RELEASE_STATE_NEUTRAL_SOURCE_PATHS,
  RELEASE_RECEIPT_FORMAT,
  SPECIFICATION_RECOVERY_ALLOWED_PATHS,
  SOURCE_EVIDENCE_ASSET,
  SOURCE_EVIDENCE_PATH,
  appendReleaseReceipt,
  releaseFromEvidence,
  staleSpecificationReleaseWording,
  validateC2DiffPaths,
  validateC3DiffPaths,
  validateC4DiffPaths,
  validateCommittedHistory,
  validateLedger,
  validateReceiptTransitionHistory,
  validateReleaseShape,
  validateSpecificationRecoveryPath,
} from "./specification-release.mjs";
import {
  CANONICAL_ORIGIN,
  SPECIFICATION_PREREQUISITES,
} from "./publication-evidence.mjs";

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

function fixtureGit(root, ...args) {
  return execFileSync("git", ["-C", root, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      GIT_AUTHOR_EMAIL: "specification-release@example.invalid",
      GIT_AUTHOR_NAME: "Specification Release Test",
      GIT_COMMITTER_EMAIL: "specification-release@example.invalid",
      GIT_COMMITTER_NAME: "Specification Release Test",
      GIT_CONFIG_GLOBAL: "/dev/null",
      GIT_CONFIG_NOSYSTEM: "1",
      GIT_CONFIG_SYSTEM: "/dev/null",
      GIT_NO_REPLACE_OBJECTS: "1",
    },
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function writeFixture(root, relativePath, value) {
  const absolutePath = path.join(root, relativePath);
  mkdirSync(path.dirname(absolutePath), { recursive: true });
  writeFileSync(absolutePath, value);
}

function writeFixtureJson(root, relativePath, value) {
  writeFixture(root, relativePath, `${JSON.stringify(value, null, 2)}\n`);
}

function commitFixture(root, message, { allowEmpty = false } = {}) {
  fixtureGit(root, "add", "-A");
  fixtureGit(
    root,
    "commit",
    ...(allowEmpty ? ["--allow-empty"] : []),
    "-m",
    message,
  );
  return fixtureGit(root, "rev-parse", "HEAD");
}

function candidateCompatibility() {
  return {
    classes: [
      {
        entries: [
          {
            identity: "takoform.specification@1.1",
            publication: "candidate",
            status: "candidate",
          },
        ],
      },
    ],
  };
}

function releasedCompatibility() {
  return {
    classes: [
      {
        entries: [
          {
            identity: "takoform.specification@1.1",
            publication: "retained",
            status: "retained",
          },
        ],
      },
    ],
  };
}

function writeC4Inputs(root, state, { preserveAssetClosure = false } = {}) {
  const compatibility =
    state === "released" ? releasedCompatibility() : candidateCompatibility();
  const status = {
    specificationVersion: "1.1",
    specificationReleaseStatus:
      state === "released" ? "released" : "candidate-open",
  };
  const readme =
    state === "released"
      ? "| Specification | `1.1` | released; one exact committed normative source snapshot is release authority |\n"
      : "Specification 1.1 is an open candidate.\n";
  const html =
    state === "released"
      ? "<!doctype html><title>Specification 1.1</title><p>Release state is ledger-derived.</p>\n"
      : "<!doctype html><title>Specification 1.1 candidate-open</title>\n";

  writeFixture(root, "README.md", readme);
  for (const relativePath of [
    "release/specification-compatibility.json",
    "website/static/release/specification-compatibility.json",
    "website/public/release/specification-compatibility.json",
  ]) {
    writeFixtureJson(root, relativePath, compatibility);
  }
  for (const relativePath of [
    "website/static/.well-known/takoform-site.json",
    "website/public/.well-known/takoform-site.json",
  ]) {
    writeFixtureJson(root, relativePath, status);
  }
  writeFixtureJson(root, "website/public/hashmap.json", {
    generated: true,
  });
  for (const relativePath of [
    ...RELEASE_STATE_CURRENT_PUBLIC_PATHS,
    "website/public/404.html",
  ]) {
    writeFixture(root, relativePath, html);
  }
  if (!preserveAssetClosure) {
    const candidateAsset = "website/public/assets/app.CAND0001.js";
    const releasedAsset = "website/public/assets/app.RELS0001.js";
    rmSync(path.join(root, state === "released" ? candidateAsset : releasedAsset), {
      force: true,
    });
    writeFixture(
      root,
      state === "released" ? releasedAsset : candidateAsset,
      `export const specificationReleaseStatus = ${JSON.stringify(state)};\n`,
    );
  }
  for (const relativePath of C4_PRESERVED_PUBLIC_PATHS) {
    writeFixture(
      root,
      relativePath,
      "<!doctype html><title>Immutable conformance fixture</title>\n",
    );
  }
}

function transitionFixture(options = {}) {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-spec-transition-"));
  fixtureGit(root, "init", "-b", "main");
  fixtureGit(root, "remote", "add", "origin", CANONICAL_ORIGIN);

  const initialLedger = c1Ledger();
  const initialBytes = `${JSON.stringify(initialLedger, null, 2)}\n`;
  for (const relativePath of C3_ALLOWED_PATHS) {
    writeFixture(root, relativePath, initialBytes);
  }
  writeC4Inputs(root, "candidate");
  const c1 = commitFixture(root, "test: C1 frozen source");
  const c2 = commitFixture(root, "test: C2 publication", { allowEmpty: true });

  let recovery = null;
  if (options.recovery) {
    if (options.extraBeforeRecovery) {
      commitFixture(root, "test: unexpected commit before recovery", {
        allowEmpty: true,
      });
    }
    if (options.mergeRecovery) {
      fixtureGit(root, "switch", "-c", "test-recovery-side", c2);
      writeFixture(root, "scripts/release-deploy.mjs", "reviewed recovery\n");
      writeFixture(
        root,
        "scripts/specification-release.test.mjs",
        "reviewed recovery test\n",
      );
      commitFixture(root, "test: recovery side parent");
      fixtureGit(root, "switch", "main");
      fixtureGit(
        root,
        "merge",
        "--no-ff",
        "-m",
        "test: merge-parent recovery",
        "test-recovery-side",
      );
      recovery = fixtureGit(root, "rev-parse", "HEAD");
    } else {
      writeFixture(root, "scripts/release-deploy.mjs", "reviewed recovery\n");
      if (options.forbiddenRecoveryPath) {
        writeFixture(root, options.forbiddenRecoveryPath, "forbidden recovery\n");
      }
      recovery = commitFixture(root, "test: reviewed recovery R");
    }
  }

  const release = receipt();
  release.sourceCommit = c1;
  release.releaseCommit = c2;
  const releasedLedger = { ...c1Ledger(), releases: [release] };
  const releasedBytes = `${JSON.stringify(releasedLedger, null, 2)}\n`;
  for (const [index, relativePath] of C3_ALLOWED_PATHS.entries()) {
    writeFixture(
      root,
      relativePath,
      options.c3ProjectionDrift && index === 1
        ? `${JSON.stringify({ ...releasedLedger, drift: true }, null, 2)}\n`
        : releasedBytes,
    );
  }
  if (options.squashC4IntoC3) writeC4Inputs(root, "released");
  const c3 = commitFixture(root, "test: C3 receipt");

  let c4 = null;
  if (!options.omitC4 && !options.squashC4IntoC3) {
    if (options.extraBeforeC4) {
      commitFixture(root, "test: unexpected commit between C3 and C4", {
        allowEmpty: true,
      });
    }
    if (options.mergeC4) {
      fixtureGit(root, "switch", "-c", "test-side", c3);
      writeFixture(root, "side.txt", "merge-parent\n");
      commitFixture(root, "test: side parent");
      fixtureGit(root, "switch", "main");
      fixtureGit(root, "merge", "--no-ff", "--no-commit", "test-side");
    }
    writeC4Inputs(root, "released", {
      preserveAssetClosure: options.unchangedAssetClosure,
    });
    if (options.keepGeneratedHtmlStale) {
      writeFixture(
        root,
        options.keepGeneratedHtmlStale,
        "<!doctype html><title>Specification 1.1 candidate-open</title>\n",
      );
    }
    if (options.c3ProjectionDrift) {
      writeFixture(root, C3_ALLOWED_PATHS[1], releasedBytes);
    }
    if (options.forbiddenC4Path) {
      writeFixture(root, options.forbiddenC4Path, "forbidden at C4\n");
    }
    c4 = commitFixture(root, "test: C4 derived public refresh");
  }
  if (options.laterDescendant) {
    writeFixture(root, "later-unrelated.txt", "allowed after C4\n");
    commitFixture(root, "test: later descendant");
  }

  return {
    c1,
    c2,
    recovery,
    c3,
    c4,
    history: [
      { commit: c3, ledger: releasedLedger },
      { commit: c1, ledger: initialLedger },
    ],
    release,
    root,
  };
}

function withTransitionFixture(options, assertion) {
  const fixture = transitionFixture(options);
  try {
    assertion(fixture);
  } finally {
    rmSync(fixture.root, { force: true, recursive: true });
  }
}

describe("Specification 1.1 owner ledger", () => {
  test("documents all four W09 commit boundaries at the root", () => {
    const readme = readFileSync(path.join(ROOT, "README.md"), "utf8");
    expect(readme).toContain("four explicit boundaries");
    expect(readme).toContain("C1 freezes the normative tree");
    expect(readme).toContain("C2 is an");
    expect(readme).toContain("C3 is the authoritative append-only");
    expect(readme).toContain("C4 is its direct generated-output-only child");
    expect(readme).not.toContain("three explicit boundaries");
  });

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
    expect(
      validateC2DiffPaths(["spec/publication-evidence.json"]).join("\n"),
    ).toContain("website/static/spec/publication-evidence.json");
    expect(
      validateC2DiffPaths([
        "spec/publication-evidence.json",
        "website/static/spec/publication-evidence.json",
      ]).join("\n"),
    ).toContain("website/public/spec/publication-evidence.json");
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
      "website/static/release/specification-releases.json",
    );
    expect(
      validateC3DiffPaths([
        LEDGER_PATH,
        "website/static/release/specification-releases.json",
      ]).join("\n"),
    ).toContain(
      "website/public/release/specification-releases.json",
    );
  });

  test("Specification recovery is either C2 itself or one exact direct reviewed child", () => {
    expect(SPECIFICATION_RECOVERY_ALLOWED_PATHS).toEqual([
      "scripts/release-deploy.mjs",
      "scripts/release-deploy.test.mjs",
      "scripts/specification-release.mjs",
      "scripts/specification-release.test.mjs",
    ]);
    expect(
      validateSpecificationRecoveryPath({
        releaseCommit,
        recoveryCommit: releaseCommit,
        recoveryParents: null,
        changedPaths: [],
      }),
    ).toEqual([]);
    expect(
      validateSpecificationRecoveryPath({
        releaseCommit,
        recoveryCommit: releaseCommit,
        recoveryParents: null,
        changedPaths: ["scripts/release-deploy.mjs"],
      }).join("\n"),
    ).toContain("must have no recovery diff");
    expect(
      validateSpecificationRecoveryPath({
        releaseCommit,
        recoveryCommit: "c".repeat(40),
        recoveryParents: [releaseCommit],
        changedPaths: [
          "scripts/release-deploy.mjs",
          "scripts/specification-release.mjs",
        ],
      }),
    ).toEqual([]);
    expect(
      validateSpecificationRecoveryPath({
        releaseCommit,
        recoveryCommit: "c".repeat(40),
        recoveryParents: ["d".repeat(40)],
        changedPaths: ["scripts/release-deploy.mjs"],
      }).join("\n"),
    ).toContain("direct single-parent child of C2");
    expect(
      validateSpecificationRecoveryPath({
        releaseCommit,
        recoveryCommit: "c".repeat(40),
        recoveryParents: [releaseCommit],
        changedPaths: ["release/README.md"],
      }).join("\n"),
    ).toContain("must include scripts/release-deploy.mjs");
    expect(
      validateSpecificationRecoveryPath({
        releaseCommit,
        recoveryCommit: "c".repeat(40),
        recoveryParents: [releaseCommit],
        changedPaths: ["scripts/release-deploy.mjs", "spec/README.md"],
      }).join("\n"),
    ).toContain("spec/README.md");
  });
});

describe("C3/C4 fixed-point history", () => {
  test("C4 permits only the explicit generated/public set", () => {
    expect(validateC4DiffPaths([...C4_REQUIRED_PATHS])).toEqual([]);
    expect(
      validateC4DiffPaths([
        ...C4_REQUIRED_PATHS,
        "website/public/assets/app.CAND0001.js",
        "website/public/assets/app.RELS0001.js",
      ]),
    ).toEqual([]);
    expect(
      validateC4DiffPaths([
        ...C4_REQUIRED_PATHS,
        "website/public/assets/index.01234567.js",
        "website/public/spec/core/index.html",
      ]),
    ).toEqual([]);
    for (const relativePath of [
      "spec/README.md",
      "spec/publication-evidence.json",
      "release/specification-releases.json",
      "website/static/release/specification-releases.json",
      "website/public/release/specification-releases.json",
      "website/docs/reference.md",
      "website/public/hashmap.json",
      "scripts/release-deploy.mjs",
      "provider/resource_worker.go",
      "forms/candidates/index.json",
      "host/reference/main.go",
      "website/public/assets/evil.txt",
      "website/public/assets/app.not-a-hash.js",
      ...C4_PRESERVED_PUBLIC_PATHS,
      "unrelated.txt",
    ]) {
      expect(
        validateC4DiffPaths([...C4_REQUIRED_PATHS, relativePath]).join("\n"),
      ).toContain("forbidden paths");
    }
  });

  test("accepts exact C3/C4 with an unchanged content-hashed asset closure", () => {
    withTransitionFixture(
      { unchangedAssetClosure: true },
      ({ release, history, root }) => {
        expect(validateReceiptTransitionHistory(release, history, root)).toEqual(
          [],
        );
      },
    );
  });

  test("accepts exact C3/C4 followed by a later descendant", () => {
    withTransitionFixture({ laterDescendant: true }, ({ release, history, root }) => {
      expect(validateReceiptTransitionHistory(release, history, root)).toEqual(
        [],
      );
    });
  });

  test("accepts one exact C2/R/C3/C4 recovery history", () => {
    withTransitionFixture(
      { recovery: true, laterDescendant: true },
      ({ release, history, root }) => {
        expect(validateReceiptTransitionHistory(release, history, root)).toEqual(
          [],
        );
      },
    );
  });

  test("rejects extra, merge-parent, and forbidden Specification recovery edges", () => {
    withTransitionFixture(
      { recovery: true, extraBeforeRecovery: true },
      ({ release, history, root }) => {
        expect(
          validateReceiptTransitionHistory(release, history, root).join("\n"),
        ).toContain("direct single-parent child of C2");
      },
    );
    withTransitionFixture(
      { recovery: true, mergeRecovery: true },
      ({ release, history, root }) => {
        expect(
          validateReceiptTransitionHistory(release, history, root).join("\n"),
        ).toContain("direct single-parent child of C2");
      },
    );
    withTransitionFixture(
      { recovery: true, forbiddenRecoveryPath: "spec/README.md" },
      ({ release, history, root }) => {
        expect(
          validateReceiptTransitionHistory(release, history, root).join("\n"),
        ).toContain("spec/README.md");
      },
    );
  });

  test("rejects a missing C4", () => {
    withTransitionFixture({ omitC4: true }, ({ release, history, root }) => {
      expect(
        validateReceiptTransitionHistory(release, history, root).join("\n"),
      ).toContain("C4 derived-public commit is missing");
    });
  });

  test("rejects a squashed C3/C4 and an extra commit between them", () => {
    withTransitionFixture(
      { omitC4: true, squashC4IntoC3: true },
      ({ release, history, root }) => {
        const result = validateReceiptTransitionHistory(
          release,
          history,
          root,
        ).join("\n");
        expect(result).toContain("receipt/ledger projection-only");
        expect(result).toContain("C4 derived-public commit is missing");
      },
    );
    withTransitionFixture(
      { extraBeforeC4: true },
      ({ release, history, root }) => {
        expect(
          validateReceiptTransitionHistory(release, history, root).join("\n"),
        ).toContain("C4 derived-public diff must include README.md");
      },
    );
  });

  test("rejects C3 projection byte drift even when C4 repairs it", () => {
    withTransitionFixture(
      { c3ProjectionDrift: true },
      ({ release, history, root }) => {
        const result = validateReceiptTransitionHistory(
          release,
          history,
          root,
        ).join("\n");
        expect(result).toContain(
          "C3 canonical/static/public Specification ledger bytes must be identical",
        );
        expect(result).toContain("forbidden paths");
      },
    );
  });

  test("rejects a C4 that leaves one generated served page at candidate-open", () => {
    withTransitionFixture(
      { keepGeneratedHtmlStale: "website/public/spec/overview.html" },
      ({ release, history, root }) => {
        const result = validateReceiptTransitionHistory(
          release,
          history,
          root,
        ).join("\n");
        expect(result).toContain("must refresh every served HTML page");
        expect(result).toContain("website/public/spec/overview.html");
      },
    );
  });

  test("rejects normative, authority, and unrelated C4 changes", () => {
    for (const forbiddenC4Path of [
      "spec/README.md",
      "spec/publication-evidence.json",
      "release/specification-releases.json",
      "scripts/specification-release.mjs",
      "provider/resource_worker.go",
      "forms/candidates/index.json",
      "host/reference/main.go",
      "unrelated.txt",
      "website/public/attacker.html",
      "website/public/assets/evil.01234567.js",
      "website/public/assets/app.EVIL0001.js",
    ]) {
      withTransitionFixture(
        { forbiddenC4Path },
        ({ release, history, root }) => {
          expect(
            validateReceiptTransitionHistory(release, history, root).join(
              "\n",
            ),
          ).toContain(forbiddenC4Path);
        },
      );
    }
  });

  test("rejects a merge-parent C4", () => {
    withTransitionFixture({ mergeC4: true }, ({ release, history, root }) => {
      expect(
        validateReceiptTransitionHistory(release, history, root).join("\n"),
      ).toContain("must be the direct single-parent child of C3");
    });
  });

  test("reads C3/C4 from the unreplaced object view despite ambient Git overrides", () => {
    withTransitionFixture({}, ({ c3, c4, release, history, root }) => {
      fixtureGit(root, "replace", c4, c3);
      const overrides = {
        GIT_ALTERNATE_OBJECT_DIRECTORIES: "/tmp/attacker-objects",
        GIT_CONFIG_COUNT: "1",
        GIT_CONFIG_KEY_0: "core.useReplaceRefs",
        GIT_CONFIG_VALUE_0: "true",
        GIT_INDEX_FILE: "/tmp/attacker-index",
        GIT_OBJECT_DIRECTORY: "/tmp/attacker-object-directory",
        GIT_REPLACE_REF_BASE: "refs/replace",
        GIT_WORK_TREE: "/tmp/attacker-worktree",
      };
      const previous = new Map(
        Object.keys(overrides).map((name) => [name, process.env[name]]),
      );
      try {
        Object.assign(process.env, overrides);
        expect(validateReceiptTransitionHistory(release, history, root)).toEqual(
          [],
        );
      } finally {
        for (const [name, value] of previous) {
          if (value === undefined) delete process.env[name];
          else process.env[name] = value;
        }
      }
    });
  });

  test("rejects repository-local URL rewrite configuration", () => {
    withTransitionFixture({}, ({ release, history, root }) => {
      fixtureGit(
        root,
        "config",
        "url.file:///tmp/attacker/.insteadOf",
        "https://github.com/",
      );
      expect(() =>
        validateReceiptTransitionHistory(release, history, root),
      ).toThrow("repository Git configuration can influence publication");
    });
  });

  test("accepts only the CI checkout's disabled automatic Git GC setting", () => {
    withTransitionFixture({}, ({ release, history, root }) => {
      fixtureGit(
        root,
        "remote",
        "set-url",
        "origin",
        CANONICAL_ORIGIN.slice(0, -4),
      );
      fixtureGit(root, "config", "gc.auto", "0");
      expect(validateReceiptTransitionHistory(release, history, root)).toEqual(
        [],
      );

      fixtureGit(root, "config", "gc.auto", "1");
      expect(() =>
        validateReceiptTransitionHistory(release, history, root),
      ).toThrow("gc.auto must be exactly 0");
    });
  });
});

describe("release-state-neutral current wording", () => {
  test("rejects stale candidate/open claims without rejecting the separate Host candidate", () => {
    for (const stale of [
      "Specification 1.1 candidate",
      "Specification 1.1 release candidate remains open",
      "Takoform Specification 1.1 is an open numbered candidate",
      "Takoform 1.1 candidate",
      "Takoform 1.1 open candidate",
      "Takoform 1.1 is open until one exact committed snapshot exists",
      "Specification: 1.1 (candidate-open, release status is ledger-derived)",
      "The numbered release remains open until evidence exists",
      "The Specification release assertion remains fail-closed while its evidence is open",
    ]) {
      expect(staleSpecificationReleaseWording(stale)).toBe(true);
    }
    for (const neutral of [
      "Specification 1.1 publication state is ledger-derived.",
      "Specification 1.1 does not publish the separate Host API v1 candidate.",
      "The Host API v1 candidate remains separately unpublished.",
    ]) {
      expect(staleSpecificationReleaseWording(neutral)).toBe(false);
    }
  });

  test("keeps every normative or curated current source release-state-neutral", () => {
    for (const relativePath of RELEASE_STATE_NEUTRAL_SOURCE_PATHS) {
      expect(
        staleSpecificationReleaseWording(
          readFileSync(path.join(ROOT, relativePath), "utf8"),
        ),
        relativePath,
      ).toBe(false);
    }
  });
});

describe("Specification release CI checkout", () => {
  test("validates the exact pull-request head instead of a synthetic merge ref", () => {
    const workflow = readFileSync(
      path.join(ROOT, ".github/workflows/quality.yml"),
      "utf8",
    );
    expect(workflow).toContain(
      "ref: ${{ github.event.pull_request.head.sha || github.sha }}",
    );
    expect(workflow).toContain("fetch-depth: 0");
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

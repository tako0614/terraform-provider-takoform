import { afterEach, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import {
  cpSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";

import {
  AUTHORITY_EVIDENCE_PATH,
  AUTHORITY_TOMBSTONE_PATH,
  CORE_AUTHORITY_FORMAT,
  CORE_AUTHORITY_RECORD_PATH,
  CORE_AUTHORITY_STATE,
  DISABLED_SURFACES,
  PREDECESSOR_CUTOFF_COMMIT,
  PREDECESSOR_WORKFLOW_BLOB_OIDS,
  RETAINED_SURFACES,
  applyAuthorityTombstoneToContract,
  assertActiveAuthorityTombstone,
  assertAuthorityHistoryContinuity,
  assertAuthorityInvocationAllowed,
  authorityTombstoneKeys,
  activateAuthorityTombstone,
  createAuthorityTransitionEvidence,
  readAuthorityTombstone,
  validateAuthorityTombstone,
  validateAuthorityTransitionEvidence,
  writeAuthorityTransitionEvidence,
} from "./authority-tombstone.mjs";

const repositoryRoot = resolve(import.meta.dir, "..");
const temporaryDirectories = [];
const CORE_AUTHORITY_FIXTURE = Object.freeze({
  format: CORE_AUTHORITY_FORMAT,
  state: CORE_AUTHORITY_STATE,
  predecessorRepository:
    "https://github.com/tako0614/terraform-provider-takoform.git",
  predecessorCutoffCommit:
    "1fa34160a4ed152443b4ea424a324f7677716e36",
  predecessorCutoffTree: "7e4a2578af2f50b826fba1004fdd4e430c761314",
  successorRepository: "https://github.com/tako0614/takoform.git",
  lastPredecessorSpecificationRelease: {
    version: "1.1",
    tag: "specification/1.1",
    tagObject: "e2c1ba71766a6b25cae0826df99c8906a7f3f20b",
    releaseId: 377480828,
  },
  predecessorTombstoneCommit: null,
  successorPreparedCommit: null,
  schemaRouteCutover: null,
  predecessorWriterDisabledAt: null,
  successorWriterEnabledAt: null,
  writerOverlapAllowed: false,
  rollback:
    "Before successor activation, abandon the prepared repository and leave the predecessor writer unchanged. After predecessor disablement, repair forward in the successor; never reopen the predecessor writer or recreate Specification 1.1.",
});

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

function git(...args) {
  return execFileSync("/usr/bin/git", args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function gitAt(cwd, ...args) {
  return execFileSync("/usr/bin/git", args, {
    cwd,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

function runDeploy(cwd, args, environment = {}) {
  return spawnSync(process.execPath, ["scripts/deploy.mjs", ...args], {
    cwd,
    encoding: "utf8",
    env: { ...process.env, ...environment },
  });
}

function mkdirSyncForFile(path) {
  const directory = path.slice(0, path.lastIndexOf("/"));
  if (!existsSync(directory)) mkdirSync(directory, { recursive: true });
}

function writeJson(path, value) {
  mkdirSyncForFile(path);
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`);
}

function trackedDocument() {
  return structuredClone(readAuthorityTombstone(repositoryRoot));
}

/**
 * Clone the predecessor cutoff so local history checks exercise a real
 * complete ancestry. The source checkout can contain the untracked W10
 * files, so the fixture copies those files explicitly after cloning.
 */
function bootstrapFixture(document) {
  const root = mkdtempSync(join(tmpdir(), "takoform-authority-tombstone-"));
  temporaryDirectories.push(root);
  execFileSync(
    "/usr/bin/git",
    ["clone", "--local", "--no-hardlinks", repositoryRoot, root],
    { cwd: repositoryRoot, stdio: ["ignore", "pipe", "pipe"] },
  );
  gitAt(root, "checkout", "--detach", PREDECESSOR_CUTOFF_COMMIT);
  cpSync(join(repositoryRoot, "scripts"), join(root, "scripts"), {
    recursive: true,
  });
  if (document) writeJson(join(root, AUTHORITY_TOMBSTONE_PATH), document);
  return root;
}

function makePendingCommit(root, pending) {
  writeJson(join(root, AUTHORITY_TOMBSTONE_PATH), pending);
  gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH);
  gitAt(
    root,
    "-c",
    "user.name=W10 test",
    "-c",
    "user.email=w10@example.invalid",
    "commit",
    "-m",
    "test: prepare authority record",
  );
  return gitAt(root, "rev-parse", "HEAD");
}

function commitAt(root, message) {
  gitAt(
    root,
    "-c",
    "user.name=W10 test",
    "-c",
    "user.email=w10@example.invalid",
    "commit",
    "-m",
    message,
  );
  return gitAt(root, "rev-parse", "HEAD");
}

function makeCoreFixture({ mutatePrepared, mutateAuthority, wrongPath } = {}) {
  const root = mkdtempSync(join(tmpdir(), "takoform-core-authority-"));
  temporaryDirectories.push(root);
  gitAt(root, "init", "--quiet");
  gitAt(root, "config", "user.name", "W10 test");
  gitAt(root, "config", "user.email", "w10@example.invalid");
  const authorityPath = join(root, CORE_AUTHORITY_RECORD_PATH);
  mkdirSyncForFile(authorityPath);
  const prepared = structuredClone(CORE_AUTHORITY_FIXTURE);
  mutatePrepared?.(prepared);
  writeJson(authorityPath, prepared);
  gitAt(root, "add", CORE_AUTHORITY_RECORD_PATH);
  const preparedCommit = commitAt(root, "test: prepare Core authority");
  const authority = { ...prepared, successorPreparedCommit: preparedCommit };
  mutateAuthority?.(authority);
  const authorityTarget = wrongPath
    ? join(root, wrongPath)
    : authorityPath;
  if (wrongPath) {
    writeJson(authorityPath, prepared);
  }
  writeJson(authorityTarget, authority);
  gitAt(root, "add", ".");
  const authorityCommit = commitAt(root, "test: pin Core prepared authority");
  return { root, preparedCommit, authorityCommit, prepared, authority };
}

function pendingDocument() {
  const tracked = trackedDocument();
  return {
    ...tracked,
    status: "pending",
    successorPreparedCommit: null,
    successorAuthorityCommit: null,
    disabledAt: null,
    authorityEvidence: null,
  };
}

function activeDocumentForCore(pending, core) {
  return {
    ...pending,
    status: "active",
    successorPreparedCommit: core.preparedCommit,
    successorAuthorityCommit: core.authorityCommit,
    disabledAt: "2026-08-27T00:00:00Z",
    authorityEvidence: null,
  };
}

function makeActiveFixture() {
  const pending = pendingDocument();
  const root = bootstrapFixture();
  // P0/P are made in a separate fixture seeded from the real future Core
  // checkout. The predecessor tombstone itself never masquerades as Core's
  // canonical authority record.
  const core = makeCoreFixture();
  const active = activeDocumentForCore(pending, core);
  const transition = createAuthorityTransitionEvidence({
    repositoryRoot: root,
    objectRepositoryRoot: core.root,
    preparedCommit: core.preparedCommit,
    authorityCommit: core.authorityCommit,
    tombstone: active,
  });
  writeAuthorityTransitionEvidence(root, transition);
  writeJson(join(root, AUTHORITY_TOMBSTONE_PATH), transition.document);
  return {
    root,
    core,
    pending,
    active: transition.document,
    preparedCommit: core.preparedCommit,
    authorityCommit: core.authorityCommit,
  };
}

describe("W10 predecessor authority tombstone", () => {
  test("the tracked record validates in whichever canonical state is present", () => {
    const document = trackedDocument();
    expect(["pending", "active"]).toContain(document.status);
    expect(() => validateAuthorityTombstone(document)).not.toThrow();
    if (document.status === "pending") {
      expect(document.successorPreparedCommit).toBeNull();
      expect(document.successorAuthorityCommit).toBeNull();
      expect(document.disabledAt).toBeNull();
      expect(document.authorityEvidence).toBeNull();
      expect(() => assertActiveAuthorityTombstone(document)).toThrow(
        "pending tombstones cannot activate",
      );
    } else {
      expect(document.successorPreparedCommit).not.toBeNull();
      expect(document.successorAuthorityCommit).not.toBeNull();
      expect(document.disabledAt).not.toBeNull();
      expect(document.authorityEvidence).not.toBeNull();
      expect(() => assertActiveAuthorityTombstone(document)).not.toThrow();
    }
    expect(authorityTombstoneKeys.topLevel).toContain("authorityEvidence");
  });

  test("pins the exact baseline workflow blobs before and after the change", () => {
    const document = trackedDocument();
    expect(document.predecessorWorkflowBlobOids).toEqual(
      PREDECESSOR_WORKFLOW_BLOB_OIDS,
    );
    for (const [path, oid] of Object.entries(PREDECESSOR_WORKFLOW_BLOB_OIDS)) {
      expect(git("rev-parse", `${PREDECESSOR_CUTOFF_COMMIT}:${path}`)).toBe(oid);
      expect(git("hash-object", path)).toBe(oid);
    }
  });

  // The tombstone's claim is that the RETIRED Specification/schema writer
  // surface is gone, not that the retained Provider/Form release lane can
  // never evolve: takoform-provider-release and takoform-form-package-release
  // are listed under retainedSurfaces precisely so they can keep shipping
  // majors. So this pins the invariant that actually holds across a retained
  // release change — every disabled surface stays absent from the effective
  // contract and every retained surface stays invocable — instead of freezing
  // scripts/release-deploy.mjs byte-for-byte. The predecessor writer blob oids
  // in release/specification-schema-authority-tombstone.json remain frozen
  // history of the cutoff commit and are unaffected.
  test("disabled writers stay absent while retained Provider/Form entries stay usable", () => {
    const contract = JSON.parse(
      execFileSync(process.execPath, ["scripts/deploy.mjs", "--contract"], {
        cwd: repositoryRoot,
        encoding: "utf8",
      }),
    );
    const current = trackedDocument();
    const effective = applyAuthorityTombstoneToContract(contract, current);
    const effectiveNames = effective.surfaces.map(({ surface }) => surface);
    if (current.status === "pending") {
      expect(effective).toEqual(contract);
    } else {
      expect(effectiveNames).toEqual([...RETAINED_SURFACES]);
      for (const surface of DISABLED_SURFACES) {
        expect(effectiveNames).not.toContain(surface);
        expect(() =>
          assertAuthorityInvocationAllowed(surface, current),
        ).toThrow();
      }
    }
    for (const surface of RETAINED_SURFACES) {
      expect(() => assertAuthorityInvocationAllowed(surface, current)).not.toThrow();
      expect(effectiveNames).toContain(surface);
    }
  });

  test("activation builds offline evidence from a self-contained future Core P0/P fixture", () => {
    const pending = pendingDocument();
    const root = bootstrapFixture(pending);
    const core = makeCoreFixture();
    expect(core.prepared.format).toBe(CORE_AUTHORITY_FORMAT);
    expect(core.prepared.state).toBe(CORE_AUTHORITY_STATE);
    expect(core.prepared.successorPreparedCommit).toBeNull();
    expect(core.authority.successorPreparedCommit).toBe(core.preparedCommit);
    const active = activateAuthorityTombstone({
      repositoryRoot: root,
      objectRepositoryRoot: core.root,
      preparedCommit: core.preparedCommit,
      authorityCommit: core.authorityCommit,
      disabledAt: "2026-08-27T00:00:00Z",
    });
    expect(active.status).toBe("active");
    expect(active.authorityEvidence?.canonicalPath).toBe(
      CORE_AUTHORITY_RECORD_PATH,
    );
    expect(() => readAuthorityTombstone(root)).not.toThrow();
  }, 30_000);

  test("pending behavior leaves the old contract and invocation path unchanged", () => {
    const document = trackedDocument();
    if (document.status !== "pending") return;
    const contract = JSON.parse(
      execFileSync(process.execPath, ["scripts/deploy.mjs", "--contract"], {
        cwd: repositoryRoot,
        encoding: "utf8",
      }),
    );
    expect(applyAuthorityTombstoneToContract(contract, document)).toEqual(contract);
    for (const surface of DISABLED_SURFACES.slice(0, 2)) {
      expect(() => assertAuthorityInvocationAllowed(surface, document)).not.toThrow();
    }
  });

  test("active guard blocks old writers before credential or runner canaries", () => {
    const { root, active } = makeActiveFixture();
    // The fixture's full readback is validated before deploy capability is
    // constructed. This catches a future helper that forgets the evidence file.
    expect(() => validateAuthorityTransitionEvidence(root, active)).not.toThrow();
    const canary = join(root, "canary-not-touched");
    const environment = {
      GH_TOKEN: canary,
      GITHUB_TOKEN: canary,
      TAKOFORM_CLOUDFLARE_ACCOUNT_ID: canary,
      TAKOFORM_CLOUDFLARE_ZONE_ID: canary,
      PATH: "/definitely-not-a-real-runner",
    };
    for (const surface of ["takoform-website", "takoform-specification-release"]) {
      for (const invocation of [
        [surface],
        ["--acknowledge-exclusive-cloudflare-writer", surface],
      ]) {
        const result = runDeploy(root, invocation, environment);
        expect(result.status).toBe(1);
        expect(result.stderr).toContain(
          `authority tombstone is active; invocation of ${surface} is disabled`,
        );
        expect(result.stdout).toBe("");
        expect(existsSync(canary)).toBe(false);
      }
    }
  });

  test("active contract omits only disabled old surfaces and unknown names remain fail-closed", () => {
    const { root } = makeActiveFixture();
    const contractResult = runDeploy(root, ["--contract"]);
    expect(contractResult.status).toBe(0);
    const contract = JSON.parse(contractResult.stdout);
    expect(contract.surfaces.map(({ surface }) => surface)).toEqual([
      ...RETAINED_SURFACES,
    ]);
    const unknown = runDeploy(root, ["an-unknown-surface"]);
    expect(unknown.status).toBe(1);
    expect(unknown.stderr).toContain("known surfaces:");
    expect(unknown.stderr).not.toContain("takoform-website");
    expect(unknown.stderr).not.toContain("takoform-specification-release");
  });

  test("rejects malformed, tampered, or prematurely activated records", () => {
    const pending = trackedDocument();
    const cases = [
      ["unknown key", { ...pending, unexpected: true }],
      ["format drift", { ...pending, format: "wrong@v1" }],
      [
        "baseline workflow drift",
        {
          ...pending,
          predecessorWorkflowBlobOids: {
            ...pending.predecessorWorkflowBlobOids,
            ".github/workflows/release.yml": "0".repeat(40),
          },
        },
      ],
      ["pending P0", { ...pending, successorPreparedCommit: "a".repeat(40) }],
      ["pending evidence", { ...pending, authorityEvidence: { format: "tampered" } }],
    ];
    if (pending.status === "active") {
      cases.push(["active missing disabledAt", { ...pending, disabledAt: null }]);
      cases.push([
        "active uppercase P",
        { ...pending, successorAuthorityCommit: "A".repeat(40) },
      ]);
      cases.push([
        "active digest mismatch",
        {
          ...pending,
          authorityEvidence: {
            ...pending.authorityEvidence,
            tombstoneSha256: "sha256:" + "0".repeat(64),
          },
        },
      ]);
    } else {
      cases.push([
        "active missing evidence",
        {
          ...pending,
          status: "active",
          successorPreparedCommit: "a".repeat(40),
          successorAuthorityCommit: "b".repeat(40),
          disabledAt: "2026-08-27T00:00:00Z",
        },
      ]);
    }
    for (const [label, value] of cases) {
      expect(() => validateAuthorityTombstone(value), label).toThrow(
        /authority tombstone/,
      );
    }
  });

  test("history continuity rejects an active commit followed by a pending descendant", () => {
    const { root, active } = makeActiveFixture();
    // The adversarial state transition must be committed: a working-tree
    // active record is not evidence that the published writer was disabled.
    gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH, AUTHORITY_EVIDENCE_PATH);
    commitAt(root, "test: commit active authority record");
    writeJson(join(root, AUTHORITY_TOMBSTONE_PATH), {
      ...active,
      status: "pending",
      successorPreparedCommit: null,
      successorAuthorityCommit: null,
      disabledAt: null,
      authorityEvidence: null,
    });
    gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH);
    gitAt(
      root,
      "-c",
      "user.name=W10 test",
      "-c",
      "user.email=w10@example.invalid",
      "commit",
      "-m",
      "test: adversarial pending revert",
    );
    const pending = JSON.parse(
      readFileSync(join(root, AUTHORITY_TOMBSTONE_PATH), "utf8"),
    );
    expect(() => assertAuthorityHistoryContinuity(root, pending)).toThrow(
      /active -> pending/,
    );
    const result = runDeploy(root, ["takoform-website"], {
      GH_TOKEN: join(root, "credential-canary"),
      PATH: "/definitely-not-a-real-runner",
    });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("active -> pending");
  });

  test("history continuity rejects a coherently regenerated active descendant", () => {
    const { root, core, active } = makeActiveFixture();
    gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH, AUTHORITY_EVIDENCE_PATH);
    commitAt(root, "test: commit immutable active authority record");

    const rewritten = createAuthorityTransitionEvidence({
      repositoryRoot: root,
      objectRepositoryRoot: core.root,
      preparedCommit: core.preparedCommit,
      authorityCommit: core.authorityCommit,
      tombstone: {
        ...active,
        disabledAt: "2026-08-27T00:00:01Z",
        authorityEvidence: null,
      },
    });
    writeAuthorityTransitionEvidence(root, rewritten);
    writeJson(join(root, AUTHORITY_TOMBSTONE_PATH), rewritten.document);
    gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH, AUTHORITY_EVIDENCE_PATH);
    commitAt(root, "test: rewrite active authority record and evidence");

    expect(() =>
      validateAuthorityTransitionEvidence(root, rewritten.document),
    ).not.toThrow();
    expect(() =>
      assertAuthorityHistoryContinuity(root, rewritten.document),
    ).toThrow(/active record changed after immutable activation/);
    const result = runDeploy(root, ["takoform-website"], {
      GH_TOKEN: join(root, "credential-canary"),
      PATH: "/definitely-not-a-real-runner",
    });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain(
      "active record changed after immutable activation",
    );
  });

  test("history continuity rejects a different active record from an unrelated merged parent", () => {
    const { root, core, active } = makeActiveFixture();
    gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH, AUTHORITY_EVIDENCE_PATH);
    commitAt(root, "test: commit canonical active authority record");

    const unrelatedRoot = mkdtempSync(
      join(tmpdir(), "takoform-unrelated-authority-"),
    );
    temporaryDirectories.push(unrelatedRoot);
    gitAt(unrelatedRoot, "init", "--quiet");
    const unrelated = createAuthorityTransitionEvidence({
      repositoryRoot: unrelatedRoot,
      objectRepositoryRoot: core.root,
      preparedCommit: core.preparedCommit,
      authorityCommit: core.authorityCommit,
      tombstone: {
        ...active,
        disabledAt: "2026-08-27T00:00:02Z",
        authorityEvidence: null,
      },
    });
    writeAuthorityTransitionEvidence(unrelatedRoot, unrelated);
    writeJson(
      join(unrelatedRoot, AUTHORITY_TOMBSTONE_PATH),
      unrelated.document,
    );
    gitAt(unrelatedRoot, "add", AUTHORITY_TOMBSTONE_PATH, AUTHORITY_EVIDENCE_PATH);
    const unrelatedCommit = commitAt(
      unrelatedRoot,
      "test: create unrelated active authority history",
    );
    gitAt(root, "fetch", unrelatedRoot, unrelatedCommit);
    gitAt(
      root,
      "-c",
      "user.name=W10 test",
      "-c",
      "user.email=w10@example.invalid",
      "merge",
      "--allow-unrelated-histories",
      "--strategy=ours",
      "--no-edit",
      "FETCH_HEAD",
    );

    expect(() => assertAuthorityHistoryContinuity(root, active)).toThrow(
      /active record changed after immutable activation/,
    );
    const result = runDeploy(root, ["takoform-website"], {
      GH_TOKEN: join(root, "credential-canary"),
      PATH: "/definitely-not-a-real-runner",
    });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain(
      "active record changed after immutable activation",
    );
  });

  test("history continuity rejects a coherently rewritten active working tree", () => {
    const { root, core, active } = makeActiveFixture();
    gitAt(root, "add", AUTHORITY_TOMBSTONE_PATH, AUTHORITY_EVIDENCE_PATH);
    commitAt(root, "test: commit active record before working-tree rewrite");

    const rewritten = createAuthorityTransitionEvidence({
      repositoryRoot: root,
      objectRepositoryRoot: core.root,
      preparedCommit: core.preparedCommit,
      authorityCommit: core.authorityCommit,
      tombstone: {
        ...active,
        disabledAt: "2026-08-27T00:00:03Z",
        authorityEvidence: null,
      },
    });
    writeAuthorityTransitionEvidence(root, rewritten);
    writeJson(join(root, AUTHORITY_TOMBSTONE_PATH), rewritten.document);

    expect(() =>
      validateAuthorityTransitionEvidence(root, rewritten.document),
    ).not.toThrow();
    expect(() =>
      assertAuthorityHistoryContinuity(root, rewritten.document),
    ).toThrow(/working tree: active record changed/);
    const check = spawnSync(
      process.execPath,
      ["scripts/authority-tombstone.mjs", "--check"],
      { cwd: root, encoding: "utf8", env: process.env },
    );
    expect(check.status).toBe(1);
    expect(check.stderr).toContain("working tree: active record changed");
    const result = runDeploy(root, ["takoform-website"], {
      GH_TOKEN: join(root, "credential-canary"),
      PATH: "/definitely-not-a-real-runner",
    });
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("working tree: active record changed");
  });

  test("evidence rejects nonexistent objects, wrong parent, extra paths, and digest tampering", () => {
    const fixture = makeActiveFixture();
    const evidencePath = join(fixture.root, AUTHORITY_EVIDENCE_PATH);
    const original = JSON.parse(readFileSync(evidencePath, "utf8"));
    const active = fixture.active;
    const cases = [
      ["nonexistent object", (value) => {
        value.objects[0].oid = "f".repeat(40);
      }],
      ["wrong parent", (value) => {
        const authority = value.objects.find((object) => object.oid === value.authorityCommit);
        const body = Buffer.from(authority.data, "base64").toString("utf8");
        authority.data = Buffer.from(body.replace(
          `parent ${value.preparedCommit}`,
          `parent ${"e".repeat(40)}`,
        )).toString("base64");
        authority.oid = "e".repeat(40);
      }],
      ["extra changed path", (value) => {
        value.changedPaths = [CORE_AUTHORITY_RECORD_PATH, "README.md"];
      }],
      ["digest mismatch", (value) => {
        value.tombstoneSha256 = "sha256:" + "0".repeat(64);
      }],
    ];
    for (const [label, mutate] of cases) {
      const mutated = structuredClone(original);
      mutate(mutated);
      writeJson(evidencePath, mutated);
      const bytes = readFileSync(evidencePath);
      const metadata = {
        ...active.authorityEvidence,
        sha256:
          label === "digest mismatch"
            ? active.authorityEvidence.sha256
            : `sha256:${createHash("sha256").update(bytes).digest("hex")}`,
      };
      expect(
        () =>
          validateAuthorityTransitionEvidence(fixture.root, {
            ...active,
            authorityEvidence: metadata,
          }),
        label,
      ).toThrow(/authority/);
      writeFileSync(evidencePath, `${JSON.stringify(original, null, 2)}\n`);
    }
  });

  test("evidence rejects wrong-path and malformed Core P0/P authority records", () => {
    const cases = [
      [
        "wrong canonical path",
        { wrongPath: "release/specification-authority-extra.json" },
      ],
      [
        "malformed P0",
        {
          mutatePrepared: (record) => {
            record.successorPreparedCommit = "f".repeat(40);
          },
        },
      ],
      [
        "malformed P",
        {
          mutateAuthority: (record) => {
            record.predecessorWriterDisabledAt = "2026-08-27T00:00:00Z";
          },
        },
      ],
      [
        "extra authority drift",
        {
          mutateAuthority: (record) => {
            record.rollback = "drift";
          },
        },
      ],
    ];
    for (const [label, options] of cases) {
      const core = makeCoreFixture(options);
      const active = activeDocumentForCore(pendingDocument(), core);
      expect(
        () =>
          createAuthorityTransitionEvidence({
            repositoryRoot,
            objectRepositoryRoot: core.root,
            preparedCommit: core.preparedCommit,
            authorityCommit: core.authorityCommit,
            tombstone: active,
          }),
        label,
      ).toThrow(/authority|Core/);
    }
  });

  test("does not add an official or third-party authority distinction", () => {
    const document = trackedDocument();
    const serialized = JSON.stringify(document).toLowerCase();
    expect(serialized).not.toContain("official");
    expect(serialized).not.toContain("third-party");
    expect(serialized).not.toContain("thirdparty");
    expect(Object.keys(document)).not.toContain("official");
    expect(Object.keys(document)).not.toContain("thirdParty");
  });
});

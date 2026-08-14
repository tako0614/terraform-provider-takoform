import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  constants as fsConstants,
  copyFileSync,
  existsSync,
  fstatSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, resolve } from "node:path";

import {
  RELEASE_SURFACES,
  parseReleaseSurfaceArgs,
  parseStrictChecksums,
  releaseDeployTestHooks,
  runReleaseSurface,
} from "./release-deploy.mjs";

const repositoryRoot = resolve(import.meta.dir, "..");
const commit = "0123456789abcdef0123456789abcdef01234567";
const laterCommit = "89abcdef0123456789abcdef0123456789abcdef";
const requestId = "01234567-89ab-4cde-8fab-0123456789ab";
const providerReleaseBranch = "maintenance/v1";
const providerReleaseRef = `refs/heads/${providerReleaseBranch}`;
const providerReleaseRemoteRef = `refs/remotes/origin/${providerReleaseBranch}`;
const temporaryDirectories = [];
let previousGH;
let previousGitHub;
let previousGHEnterprise;
let previousGitHubEnterprise;

beforeEach(() => {
  previousGH = process.env.GH_TOKEN;
  previousGitHub = process.env.GITHUB_TOKEN;
  previousGHEnterprise = process.env.GH_ENTERPRISE_TOKEN;
  previousGitHubEnterprise = process.env.GITHUB_ENTERPRISE_TOKEN;
  process.env.GH_TOKEN = "operator-only-test-token";
  process.env.GITHUB_TOKEN = "ambient-token-must-be-scrubbed";
});

afterEach(() => {
  if (previousGH === undefined) delete process.env.GH_TOKEN;
  else process.env.GH_TOKEN = previousGH;
  if (previousGitHub === undefined) delete process.env.GITHUB_TOKEN;
  else process.env.GITHUB_TOKEN = previousGitHub;
  if (previousGHEnterprise === undefined) {
    delete process.env.GH_ENTERPRISE_TOKEN;
  } else {
    process.env.GH_ENTERPRISE_TOKEN = previousGHEnterprise;
  }
  if (previousGitHubEnterprise === undefined) {
    delete process.env.GITHUB_ENTERPRISE_TOKEN;
  } else {
    process.env.GITHUB_ENTERPRISE_TOKEN = previousGitHubEnterprise;
  }
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { recursive: true, force: true });
  }
});

function temporaryDirectory(prefix) {
  const directory = mkdtempSync(join(tmpdir(), `${prefix}-`));
  temporaryDirectories.push(directory);
  return directory;
}

function memoryIO() {
  let output = "";
  let errors = "";
  return {
    stdout: { write: (value) => (output += String(value)) },
    stderr: { write: (value) => (errors += String(value)) },
    get output() {
      return output;
    },
    get errors() {
      return errors;
    },
  };
}

function context(execFile, overrides = {}) {
  const io = memoryIO();
  return {
    repo: repositoryRoot,
    stdout: io.stdout,
    stderr: io.stderr,
    execFile,
    uuidFactory: () => requestId,
    now: () => Date.parse("2026-07-29T00:00:01.000Z"),
    wait: () => {},
    io,
    ...overrides,
  };
}

function commandFailure(stderr, status = 1) {
  const error = new Error(stderr);
  error.stdout = "";
  error.stderr = stderr;
  error.status = status;
  return error;
}

function sha256(raw) {
  return `sha256:${createHash("sha256").update(raw).digest("hex")}`;
}

function isReleaseList(args) {
  return (
    args[0] === "api" &&
    args.includes("--paginate") &&
    args.some((argument) =>
      argument.includes(
        "repos/tako0614/terraform-provider-takoform/releases?per_page=100",
      ),
    )
  );
}

function recursivelySorted(value) {
  if (Array.isArray(value)) return value.map(recursivelySorted);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, recursivelySorted(value[key])]),
    );
  }
  return value;
}

function writeChecksumFixture(root, names, destination = "SHA256SUMS") {
  const lines = [...names]
    .sort()
    .map(
      (name) =>
        `${sha256(readFileSync(join(root, ...name.split("/")))).slice(7)}  ${name}`,
    );
  writeFileSync(join(root, destination), `${lines.join("\n")}\n`);
}

function tagObjectFixture({
  tag,
  sourceCommit,
  request = requestId,
  runId,
  runAttempt,
  revocation = false,
}) {
  const actor = revocation
    ? "Takoform Form Package Revocation"
    : "Takoform Form Package Release";
  const title = revocation
    ? `Takoform Form Package revocation checkpoint ${tag}`
    : `Takoform Form Package ${tag}`;
  const raw = Buffer.from(
    `object ${sourceCommit}\ntype commit\ntag ${tag}\n` +
      `tagger ${actor} <release@takoform.invalid> 1 +0000\n\n` +
      `${title}\n\nsource-commit: ${sourceCommit}\n` +
      `request-id: ${request}\n` +
      `workflow-run: https://github.com/tako0614/terraform-provider-takoform/actions/runs/${runId}/attempts/${runAttempt}\n`,
  );
  return {
    raw,
    oid: createHash("sha1")
      .update(Buffer.from(`tag ${raw.length}\0`))
      .update(raw)
      .digest("hex"),
  };
}

function writeDeepFailureCandidate(
  destination,
  { type, entry, tag, runId, runAttempt, sourceCommit, toolingCommit },
) {
  const revocation = type === "revocation";
  const version = revocation
    ? /^forms\/revocations\/v(.+)$/u.exec(tag)[1]
    : entry.version;
  const releaseId = revocation ? null : entry.releaseId;
  const base = revocation
    ? `takoform-form-revocation_${version}`
    : `takoform-form-${releaseId}_${version}`;
  const names = revocation
    ? [
        `${base}_checkpoint.json`,
        `${base}_checkpoint.sigstore.json`,
        `${base}_statement.json`,
        `${base}_provenance.intoto.json`,
        "release-manifest.json",
        "SHA256SUMS",
      ].sort()
    : [
        `${base}.tar.gz`,
        `${base}_package-index.json`,
        `${base}_package-index.sigstore.json`,
        `${base}_provenance.intoto.json`,
        `${base}_sbom.spdx.json`,
        "release-manifest.json",
        "SHA256SUMS",
      ].sort();
  const subject = revocation
    ? `${base}_checkpoint.json`
    : `${base}_package-index.json`;
  const bundle = revocation
    ? `${base}_checkpoint.sigstore.json`
    : `${base}_package-index.sigstore.json`;
  const payloadNames = names.filter(
    (name) => name !== "release-manifest.json" && name !== "SHA256SUMS",
  );
  const assetsRoot = join(destination, "assets");
  mkdirSync(assetsRoot, { recursive: true });
  for (const name of payloadNames) {
    const raw =
      !revocation && name === subject
        ? Buffer.from('{"fixture":"package-index"}\n')
        : Buffer.from(`${name}\n`);
    writeFileSync(join(assetsRoot, name), raw);
  }
  const manifest = {
    schemaVersion: 1,
    releaseType: revocation ? "form-package-revocation" : "form-package",
    tag,
    sourceRepository: "github.com/tako0614/terraform-provider-takoform",
    sourceCommit,
    toolingCommit,
    workflow: revocation
      ? ".github/workflows/form-package-revocation.yml"
      : ".github/workflows/form-package-release.yml",
    packageVersion: version,
    ...(revocation
      ? {}
      : {
          releaseId,
          packageDigest: entry.packageDigest,
          formRef: entry.formRef,
        }),
    signedSubject: subject,
    signatureBundle: bundle,
    publicationReady: true,
    publicationBlockers: [],
    assets: payloadNames.map((name) => ({
      name,
      digest: sha256(readFileSync(join(assetsRoot, name))),
    })),
  };
  writeFileSync(
    join(assetsRoot, "release-manifest.json"),
    JSON.stringify(manifest),
  );
  writeChecksumFixture(assetsRoot, [
    ...payloadNames,
    "release-manifest.json",
  ]);

  const metadataAssets = names.map((name) => ({
    name,
    sha256: sha256(readFileSync(join(assetsRoot, name))),
  }));
  const workflow = revocation
    ? "form-package-revocation.yml"
    : "form-package-release.yml";
  const metadata = {
    format: revocation
      ? "takoform.form-package-revocation-candidate@v1"
      : "takoform.form-package-release-candidate@v1",
    repository: "tako0614/terraform-provider-takoform",
    workflowPath: `.github/workflows/${workflow}`,
    workflowRef:
      `tako0614/terraform-provider-takoform/.github/workflows/${workflow}` +
      "@refs/heads/main",
    sourceRef: "refs/heads/main",
    runId,
    runAttempt,
    tag,
    sourceCommit,
    toolingCommit,
    objectFormat: "sha1",
    tagObjectOid: "",
    tagObjectSha256: "",
    requestId,
    ...(revocation ? { assetCount: names.length } : {}),
    assets: metadataAssets,
  };
  const tagObject = tagObjectFixture({
    tag,
    sourceCommit,
    runId,
    runAttempt,
    revocation,
  });
  metadata.tagObjectOid = tagObject.oid;
  metadata.tagObjectSha256 = sha256(tagObject.raw);
  writeFileSync(
    join(destination, "metadata.json"),
    revocation
      ? `${JSON.stringify(recursivelySorted(metadata), null, 2)}\n`
      : JSON.stringify(recursivelySorted(metadata)),
  );
  writeFileSync(join(destination, "tag-object"), tagObject.raw);
  writeChecksumFixture(destination, [
    ...names.map((name) => `assets/${name}`),
    "metadata.json",
    "tag-object",
  ]);
}

function writePublicationSetFixture(staging, plan) {
  const authorityRoot = join(staging, "authority", commit);
  mkdirSync(authorityRoot, { recursive: true });
  const planRaw = Buffer.from(JSON.stringify(plan));
  const trustedRootRaw = Buffer.from("{}\n");
  writeFileSync(join(authorityRoot, "release-plan.json"), planRaw);
  writeFileSync(join(authorityRoot, "trusted-root.json"), trustedRootRaw);
  writeFileSync(
    join(staging, "form-package-publication-set.json"),
    JSON.stringify({
      format: "takoform.form-package-publication-set@v1",
      generation: plan.generation,
      repository: "tako0614/terraform-provider-takoform",
      protectedMainCommit: commit,
      entries: plan.releases.map((entry) => ({
        tag: entry.tag,
        releaseId: entry.releaseId,
        version: entry.version,
        toolingCommit: commit,
        releasePlan: {
          path: `authority/${commit}/release-plan.json`,
          sourcePath: "forms/release-plan.json",
          sha256: sha256(planRaw),
        },
        trustedRoot: {
          path: `authority/${commit}/trusted-root.json`,
          sourcePath: "admission/v4/trust/trusted-root.json",
          sha256: sha256(trustedRootRaw),
        },
      })),
    }),
  );
}

function writeRegistryReadbackCandidate(
  root,
  {
    sourceCommit = commit,
    providerCommit = "b".repeat(40),
    certificateIdentity =
      "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/provider-registry-readback.yml@refs/heads/maintenance/v1",
    installedDigests = ["sha256:" + "c".repeat(64), "sha256:" + "c".repeat(64)],
  } = {},
) {
  const descriptor = JSON.parse(
    readFileSync(join(repositoryRoot, "release", "version.json"), "utf8"),
  );
  const matrix = Buffer.from('{"fixture":"registry-matrix"}');
  const readback = Buffer.from(
    JSON.stringify({
      installs: [
        { product: "OpenTofu", providerBinarySha256: installedDigests[0] },
        { product: "Terraform", providerBinarySha256: installedDigests[1] },
      ],
    }),
  );
  const bundle = Buffer.from("sigstore fixture\n");
  writeFileSync(join(root, "provider-lifecycle-matrix.json"), matrix);
  writeFileSync(join(root, "provider-readback.json"), readback);
  writeFileSync(join(root, "provider-readback.sigstore.json"), bundle);
  const manifest = recursivelySorted({
    format: "takoform.provider-registry-readback-candidate@v1",
    status: "candidate-only",
    proofType: "registry-readback",
    generation: "portable-v1",
    source: {
      repository:
        "https://github.com/tako0614/terraform-provider-takoform.git",
      commit: sourceCommit,
    },
    provider: {
      address: "registry.terraform.io/tako0614/takoform",
      version: descriptor.version,
      tag: descriptor.tag,
      commit: providerCommit,
    },
    matrix: {
      path: "provider-lifecycle-matrix.json",
      digest: sha256(matrix),
    },
    readback: {
      path: "provider-readback.json",
      digest: sha256(readback),
      bundlePath: "provider-readback.sigstore.json",
    },
  });
  const manifestRaw = Buffer.from(JSON.stringify(manifest));
  writeFileSync(
    join(root, "provider-registry-readback-manifest.json"),
    manifestRaw,
  );
  const signed = recursivelySorted({
    format: "takoform.provider-registry-readback-signed-candidate@v1",
    status: "candidate-only",
    proofType: "registry-readback",
    generation: "portable-v1",
    certificateIdentity,
    workflow: ".github/workflows/provider-registry-readback.yml",
    requestId,
    workflowRunId: "123",
    workflowRunAttempt: 1,
    source: manifest.source,
    manifest: {
      path: "provider-registry-readback-manifest.json",
      digest: sha256(manifestRaw),
    },
    readback: {
      path: "provider-readback.json",
      digest: sha256(readback),
      bundlePath: "provider-readback.sigstore.json",
      bundleDigest: sha256(bundle),
    },
  });
  writeFileSync(
    join(root, "signed-provider-registry-readback-candidate.json"),
    JSON.stringify(signed),
  );
  writeChecksumFixture(root, [
    "provider-lifecycle-matrix.json",
    "provider-readback.json",
    "provider-readback.sigstore.json",
    "provider-registry-readback-manifest.json",
    "signed-provider-registry-readback-candidate.json",
  ]);
  return { descriptor, providerCommit, readback: readback.toString("utf8") };
}

describe("release surface contract and strict parsing", () => {
  test("declares owner authority, no-overwrite, and asynchronous halt", () => {
    expect(RELEASE_SURFACES.map((surface) => surface.surface)).toEqual([
      "takoform-provider-release",
      "takoform-form-package-release",
    ]);
    for (const surface of RELEASE_SURFACES) {
      expect(surface.requiresScripts).toContain("check");
      expect(surface.requiresEnv).toEqual(["GH_TOKEN"]);
      expect(surface.triggers).toEqual([
        "authority",
        "published-identity",
        "asynchronous",
      ]);
      expect(Object.keys(surface.obligations).sort()).toEqual(
        [
          "failure-handling",
          "halt",
          "independent-review",
          "no-overwrite",
          "post-conditions",
          "provenance",
          "reversal",
        ].sort(),
      );
    }
    const provider = RELEASE_SURFACES.find(
      (surface) => surface.surface === "takoform-provider-release",
    );
    expect(provider.obligations.halt).toContain("prepare-candidate");
    expect(provider.obligations.halt).toContain("reconcile-candidate");
    expect(provider.obligations["no-overwrite"]).toContain(
      "owner-private request record",
    );
    expect(provider.obligations["no-overwrite"]).toContain(
      "refs/heads/provider-candidate-reservation-<tag>",
    );
    expect(provider.obligations.provenance).toContain(
      "evaluated creation/update/deletion rules",
    );
    expect(provider.obligations["failure-handling"]).toContain(
      "lost create-ref acknowledgement",
    );
  });

  test("accepts only exact phase options and canonical values", () => {
    expect(
      parseReleaseSurfaceArgs("takoform-provider-release", [
        "publish",
        "--tag",
        "v1.0.0",
        "--expected-commit",
        commit,
        "--run-id",
        "42",
        "--run-attempt",
        "1",
      ]),
    ).toEqual({
      phase: "publish",
      tag: "v1.0.0",
      "expected-commit": commit,
      "run-id": "42",
      "run-attempt": "1",
    });
    for (const args of [
      ["publish", "--tag", "v1.0.0"],
      [
        "publish",
        "--tag",
        "v1.0.0",
        "--expected-commit",
        "HEAD",
        "--run-id",
        "42",
        "--run-attempt",
        "1",
      ],
      [
        "publish",
        "--tag",
        "v1.0.0",
        "--expected-commit",
        commit,
        "--run-id",
        "42",
        "--run-attempt",
        "1",
        "--run-id",
        "43",
      ],
    ]) {
      expect(() =>
        parseReleaseSurfaceArgs("takoform-provider-release", args),
      ).toThrow();
    }

    const recovery = parseReleaseSurfaceArgs(
      "takoform-form-package-release",
      [
        "recover-tag-only",
        "--tag",
        "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0",
        "--expected-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        "89abcdef0123456789abcdef0123456789abcdef",
        "--run-id",
        "123",
        "--run-attempt",
        "1",
      ],
    );
    expect(recovery.phase).toBe("recover-tag-only");
    expect(recovery["expected-tag-object"]).toBe("a".repeat(40));
    const draftRecovery = parseReleaseSurfaceArgs(
      "takoform-form-package-release",
      [
        "recover-draft",
        "--tag",
        "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0",
        "--expected-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        "89abcdef0123456789abcdef0123456789abcdef",
        "--release-id",
        "362120999",
        "--run-id",
        "123",
        "--run-attempt",
        "1",
      ],
    );
    expect(draftRecovery.phase).toBe("recover-draft");
    expect(draftRecovery["release-id"]).toBe("362120999");
    const providerRecovery = parseReleaseSurfaceArgs(
      "takoform-provider-release",
      [
        "recover-tag-only",
        "--tag",
        "v1.0.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        "89abcdef0123456789abcdef0123456789abcdef",
        "--run-id",
        "30507374579",
        "--run-attempt",
        "1",
      ],
    );
    expect(providerRecovery).toEqual({
      phase: "recover-tag-only",
      tag: "v1.0.1",
      "expected-release-commit": commit,
      "expected-tag-object": "a".repeat(40),
      "expected-recovery-commit":
        "89abcdef0123456789abcdef0123456789abcdef",
      "run-id": "30507374579",
      "run-attempt": "1",
    });
    expect(
      parseReleaseSurfaceArgs("takoform-provider-release", [
        "recover-draft",
        "--tag",
        "v1.0.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        "89abcdef0123456789abcdef0123456789abcdef",
        "--release-id",
        "362120999",
        "--run-id",
        "30507374579",
        "--run-attempt",
        "1",
      ])["release-id"],
    ).toBe("362120999");
    const requestRecord = join(
      temporaryDirectory("provider-candidate-request"),
      "request.json",
    );
    for (const phase of ["prepare-candidate", "reconcile-candidate"]) {
      expect(
        parseReleaseSurfaceArgs("takoform-provider-release", [
          phase,
          "--tag",
          "v1.0.4",
          "--expected-release-commit",
          commit,
          "--expected-current-commit",
          "89abcdef0123456789abcdef0123456789abcdef",
          "--expected-tag-object",
          "a".repeat(40),
          "--request-id",
          requestId,
          "--request-record",
          requestRecord,
        ]),
      ).toEqual({
        phase,
        tag: "v1.0.4",
        "expected-release-commit": commit,
        "expected-current-commit":
          "89abcdef0123456789abcdef0123456789abcdef",
        "expected-tag-object": "a".repeat(40),
        "request-id": requestId,
        "request-record": requestRecord,
      });
    }
    for (const invalidRequestId of [
      "",
      "01234567-89AB-4CDE-8FAB-0123456789AB",
      "01234567-89ab-1cde-8fab-0123456789ab",
      "01234567-89ab-4cde-7fab-0123456789ab",
      "not-a-uuid",
    ]) {
      expect(() =>
        parseReleaseSurfaceArgs("takoform-provider-release", [
          "prepare-candidate",
          "--tag",
          "v1.0.4",
          "--expected-release-commit",
          commit,
          "--expected-current-commit",
          commit,
          "--expected-tag-object",
          "a".repeat(40),
          "--request-id",
          invalidRequestId,
          "--request-record",
          requestRecord,
        ]),
      ).toThrow();
    }
    expect(() =>
      parseReleaseSurfaceArgs("takoform-provider-release", [
        "prepare-candidate",
        "--tag",
        "v1.0.4",
        "--expected-release-commit",
        commit,
        "--expected-current-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--request-id",
        requestId,
        "--request-record",
        "request.json",
      ]),
    ).toThrow("--request-record must be an absolute path");
    for (const invalid of ["HEAD", "A".repeat(40), "a".repeat(39), "a".repeat(41)]) {
      expect(() =>
        parseReleaseSurfaceArgs("takoform-form-package-release", [
          "recover-tag-only",
          "--tag",
          "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0",
          "--expected-commit",
          commit,
          "--expected-tag-object",
          invalid,
          "--expected-recovery-commit",
          "89abcdef0123456789abcdef0123456789abcdef",
          "--run-id",
          "123",
          "--run-attempt",
          "1",
        ]),
      ).toThrow();
    }
    for (const releaseId of ["0", "-1", "1.0", "9007199254740992"]) {
      expect(() =>
        parseReleaseSurfaceArgs("takoform-form-package-release", [
          "recover-draft",
          "--tag",
          "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0",
          "--expected-commit",
          commit,
          "--expected-tag-object",
          "a".repeat(40),
          "--expected-recovery-commit",
          "89abcdef0123456789abcdef0123456789abcdef",
          "--release-id",
          releaseId,
          "--run-id",
          "123",
          "--run-attempt",
          "1",
        ]),
      ).toThrow();
    }
  });

  test("checksum closure rejects traversal, duplicates, extras, and drift", () => {
    expect(
      parseStrictChecksums(`${"a".repeat(64)}  assets/file.zip\n`),
    ).toEqual(new Map([["assets/file.zip", `sha256:${"a".repeat(64)}`]]));
    for (const raw of [
      `${"a".repeat(64)}  ../file\n`,
      `${"a".repeat(64)}  file\n${"b".repeat(64)}  file\n`,
      `${"a".repeat(64)}  file`,
    ]) {
      expect(() => parseStrictChecksums(raw)).toThrow();
    }

    const root = temporaryDirectory("release-checksum");
    writeFileSync(join(root, "asset"), "exact bytes\n");
    writeFileSync(
      join(root, "SHA256SUMS"),
      `${sha256(readFileSync(join(root, "asset"))).slice(7)}  asset\n`,
    );
    expect(releaseDeployTestHooks.verifyChecksumClosure(root).size).toBe(1);
    writeFileSync(join(root, "extra"), "not checksummed\n");
    expect(() =>
      releaseDeployTestHooks.verifyChecksumClosure(root),
    ).toThrow("exact inventory");
  });

  test("uses the Registry versions authority for exact no-overwrite", () => {
    const fake = () =>
      JSON.stringify({
        versions: [{ version: "0.2.1" }, { version: "1.0.0" }],
      });
    expect(() =>
      releaseDeployTestHooks.assertRegistryVersionAbsent(
        context(fake),
        "1.0.0",
      ),
    ).toThrow("version identity already exists");
    expect(() =>
      releaseDeployTestHooks.assertRegistryVersionAbsent(
        context(fake),
        "1.0.1",
      ),
    ).not.toThrow();
    expect(() =>
      releaseDeployTestHooks.assertRegistryVersionAbsent(
        context(() =>
          JSON.stringify({
            versions: [{ version: "1.0.0" }, { version: "1.0.0" }],
          }),
        ),
        "1.0.1",
      ),
    ).toThrow("duplicate identities");
  });

  test("rejects duplicate-key candidate metadata for every release kind", () => {
    expect(() =>
      releaseDeployTestHooks.parseCanonicalCandidateMetadata(
        Buffer.from('{"requestId":"first","requestId":"second"}'),
        "Form Package candidate metadata",
      ),
    ).toThrow("canonical JSON");
    expect(() =>
      releaseDeployTestHooks.parsePrettyCandidateMetadata(
        Buffer.from(
          '{\n  "requestId": "first",\n  "requestId": "second"\n}\n',
        ),
        "provider candidate metadata",
      ),
    ).toThrow("two-space pretty JSON");
  });

  test("accepts the exact provider run 30507374579 metadata bytes and no encoding variant", () => {
    const raw = readFileSync(
      join(
        repositoryRoot,
        "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
      ),
    );
    expect(sha256(raw)).toBe(
      "sha256:3a981c9762da688a76f0af4a9756c9257920cd3e6992abd3db6af37870dc84f0",
    );
    const metadata = releaseDeployTestHooks.parsePrettyCandidateMetadata(
      raw,
      "provider candidate metadata",
    );
    expect(metadata).toMatchObject({
      runId: "30507374579",
      attempt: "1",
      sourceCommit: "44e1da0bc7e5b2581e2197ccedb107e5d9a7e9db",
      toolingCommit: "44e1da0bc7e5b2581e2197ccedb107e5d9a7e9db",
      tagObjectOid: "e824793f019a941be11fde0a908fd8d1ea813ba8",
      assetCount: 15,
    });
    const value = JSON.parse(raw);
    for (const variant of [
      raw.subarray(0, raw.length - 1),
      Buffer.concat([raw, Buffer.from("\n")]),
      Buffer.from(raw.toString("utf8").replaceAll("\n", "\r\n")),
      Buffer.from(`${JSON.stringify(recursivelySorted(value), null, 4)}\n`),
      Buffer.from(
        `${JSON.stringify({ workflowRef: value.workflowRef, ...value }, null, 2)}\n`,
      ),
      Buffer.from(JSON.stringify(recursivelySorted(value))),
    ]) {
      expect(() =>
        releaseDeployTestHooks.parsePrettyCandidateMetadata(
          variant,
          "provider candidate metadata",
        ),
      ).toThrow("two-space pretty JSON");
    }
  });

  test("preserves compact Form metadata and exact pretty revocation metadata as separate profiles", () => {
    const compact = Buffer.from('{"assets":[],"tag":"forms/k-test/v1.0.0"}');
    expect(
      releaseDeployTestHooks.parseCanonicalCandidateMetadata(
        compact,
        "Form candidate metadata",
      ),
    ).toEqual({ assets: [], tag: "forms/k-test/v1.0.0" });
    expect(
      releaseDeployTestHooks.parseCanonicalCandidateMetadata(
        Buffer.concat([compact, Buffer.from("\n")]),
        "Form candidate metadata",
      ),
    ).toEqual({ assets: [], tag: "forms/k-test/v1.0.0" });
    const revocation = Buffer.from(
      '{\n  "assets": [],\n  "tag": "forms/revocations/v1.0.0"\n}\n',
    );
    expect(
      releaseDeployTestHooks.parsePrettyCandidateMetadata(
        revocation,
        "revocation candidate metadata",
      ),
    ).toEqual({ assets: [], tag: "forms/revocations/v1.0.0" });
    expect(() =>
      releaseDeployTestHooks.parsePrettyCandidateMetadata(
        compact,
        "revocation candidate metadata",
      ),
    ).toThrow("two-space pretty JSON");
    const prettyForm = Buffer.from(
      '{\n  "assets": [],\n  "tag": "forms/k-test/v1.0.0"\n}\n',
    );
    expect(() =>
      releaseDeployTestHooks.parseCanonicalCandidateMetadata(
        prettyForm,
        "Form candidate metadata",
      ),
    ).toThrow("compact canonical JSON");
    expect(() =>
      releaseDeployTestHooks.parseCandidateMetadata(
        compact,
        "candidate metadata",
        { profile: "unknown" },
      ),
    ).toThrow("unsupported metadata profile");
  });
});

describe("workflow dispatch authority and correlation", () => {
  test("provider prepare dispatches and correlates only maintenance/v1", () => {
    const calls = [];
    let dispatched = false;
    const run = {
      attempt: 1,
      createdAt: "2026-07-29T00:00:01.000Z",
      databaseId: 123,
      displayTitle: requestId,
      event: "workflow_dispatch",
      headBranch: providerReleaseBranch,
      headSha: commit,
      status: "queued",
      url:
        "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123",
      workflowName: "Author provider release tag",
    };
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (2026-07-02)\n";
      }
      if (executable === "cosign" && args[0] === "version") {
        return "GitVersion:    v3.0.6\n";
      }
      if (
        executable === "bun" &&
        args.join(" ") === "run check:release-owner-gate"
      ) {
        return "";
      }
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return `${providerReleaseBranch}\n`;
        if (args[0] === "fetch") return "";
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === `rev-parse ${providerReleaseRemoteRef}`
        ) {
          return `${commit}\n`;
        }
        if (args[0] === "cat-file") return "";
        if (args[0] === "for-each-ref" || args[0] === "ls-remote") {
          return "";
        }
      }
      if (executable === "curl" && args.at(-1).endsWith("/versions")) {
        return '{"versions":[]}';
      }
      if (executable === "gh" && isReleaseList(args)) return "[[]]";
      if (executable === "gh" && args[0] === "run" && args[1] === "list") {
        return JSON.stringify(dispatched ? [run] : []);
      }
      if (
        executable === "gh" &&
        args[0] === "workflow" &&
        args[1] === "run"
      ) {
        dispatched = true;
        return `${run.url}\n`;
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };

    const result = runReleaseSurface({
      surface: "takoform-provider-release",
      args: [
        "prepare",
        "--tag",
        "v1.0.4",
        "--expected-commit",
        commit,
      ],
      repo: repositoryRoot,
      execFile: fake,
      stdout: memoryIO().stdout,
      stderr: memoryIO().stderr,
      uuidFactory: () => requestId,
      now: () => Date.parse("2026-07-29T00:00:01.000Z"),
      wait: () => {},
    });

    expect(result.workflowRun.runId).toBe("123");
    const dispatch = calls.find(
      (call) =>
        call.executable === "gh" &&
        call.args[0] === "workflow" &&
        call.args[1] === "run",
    );
    expect(dispatch.args).toContain(providerReleaseBranch);
    const correlatedLists = calls.filter(
      (call) =>
        call.executable === "gh" &&
        call.args[0] === "run" &&
        call.args[1] === "list",
    );
    expect(correlatedLists).toHaveLength(2);
    for (const listed of correlatedLists) {
      expect(listed.args).toContain("--branch");
      expect(listed.args).toContain(providerReleaseBranch);
    }
  });

  test("scopes upload authority to the absolute uploads host without argv exposure", () => {
    process.env.GH_ENTERPRISE_TOKEN = "ambient-enterprise-token";
    process.env.GITHUB_ENTERPRISE_TOKEN = "ambient-enterprise-token";
    const environment = releaseDeployTestHooks.githubUploadEnvironment();
    expect(environment.GH_TOKEN).toBeUndefined();
    expect(environment.GITHUB_TOKEN).toBeUndefined();
    expect(environment.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
    expect(environment.GH_ENTERPRISE_TOKEN).toBe(
      "operator-only-test-token",
    );
  });

  test("scrubs authority from non-gh children and binds one UUID run", () => {
    const calls = [];
    let dispatched = false;
    const run = {
      attempt: 1,
      createdAt: "2026-07-29T00:00:01.000Z",
      databaseId: 123,
      displayTitle: requestId,
      event: "workflow_dispatch",
      headBranch: "main",
      headSha: commit,
      status: "queued",
      url:
        "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123",
      workflowName: "Prepare provider release candidate",
    };
    const fake = (executable, args, options) => {
      calls.push({ executable, args: [...args], env: { ...options.env } });
      if (executable === "bun") return "";
      if (executable !== "gh") throw new Error(`unexpected ${executable}`);
      if (args[0] === "run" && args[1] === "list") {
        return JSON.stringify(dispatched ? [run] : []);
      }
      if (args[0] === "workflow" && args[1] === "run") {
        dispatched = true;
        return `${run.url}\n`;
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    const execution = context(fake);

    releaseDeployTestHooks.command(execution, "bun", ["run", "check"]);
    const result = releaseDeployTestHooks.dispatchWorkflow(
      execution,
      "release.yml",
      { tag: "v1.0.0", expected_commit: commit },
      { headSha: commit },
    );

    expect(result).toEqual({
      runId: "123",
      requestId,
      url: run.url,
    });
    const dispatch = calls.find(
      (call) =>
        call.executable === "gh" &&
        call.args[0] === "workflow" &&
        call.args[1] === "run",
    );
    expect(dispatch.args).toContain(`request_id=${requestId}`);
    expect(
      calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "run" &&
          call.args[1] === "list",
      ).length,
    ).toBe(2);
    for (const call of calls) {
      expect(call.env.GITHUB_TOKEN).toBeUndefined();
      expect(call.env.GH_TOKEN).toBe(
        call.executable === "gh" ? "operator-only-test-token" : undefined,
      );
    }
  });

  test("halts instead of selecting latest when UUID correlation is ambiguous", () => {
    let dispatched = false;
    const run = (id) => ({
      attempt: 1,
      createdAt: "2026-07-29T00:00:01.000Z",
      databaseId: id,
      displayTitle: requestId,
      event: "workflow_dispatch",
      headBranch: "main",
      headSha: commit,
      status: "queued",
      url:
        `https://github.com/tako0614/terraform-provider-takoform/actions/runs/${id}`,
      workflowName: "Prepare provider release candidate",
    });
    const fake = (_executable, args) => {
      if (args[0] === "run" && args[1] === "list") {
        return JSON.stringify(dispatched ? [run(123), run(124)] : []);
      }
      dispatched = true;
      return "";
    };
    expect(() =>
      releaseDeployTestHooks.dispatchWorkflow(
        context(fake),
        "release.yml",
        { tag: "v1.0.0", expected_commit: commit },
        { headSha: commit },
      ),
    ).toThrow("ambiguous");
  });

  test("binds a reviewed workflow run to its exact attempt URL", () => {
    const reviewedRun = (url) =>
      context((_executable, args) => {
        expect(args).toContain("--attempt");
        expect(args).toContain("2");
        return JSON.stringify({
          attempt: 2,
          conclusion: "success",
          databaseId: 123,
          displayTitle: requestId,
          event: "workflow_dispatch",
          headBranch: "main",
          headSha: commit,
          status: "completed",
          url,
          workflowName: "Author provider release tag",
        });
      });
    const exactUrl =
      "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/2";
    expect(
      releaseDeployTestHooks.requireSuccessfulRun(
        reviewedRun(exactUrl),
        "123",
        "2",
        {
          workflowName: "Author provider release tag",
          headSha: commit,
        },
      ).url,
    ).toBe(exactUrl);
    for (const wrongUrl of [
      "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123",
      "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/1",
      "https://github.com/tako0614/terraform-provider-takoform/actions/runs/124/attempts/2",
    ]) {
      expect(() =>
        releaseDeployTestHooks.requireSuccessfulRun(
          reviewedRun(wrongUrl),
          "123",
          "2",
          {
            workflowName: "Author provider release tag",
            headSha: commit,
          },
        ),
      ).toThrow("not the exact successful reviewed candidate");
    }
  });
});

describe("provider candidate durable dispatch request", () => {
  function expectedRequestRecord(path, overrides = {}) {
    return releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: "v1.0.4",
      requestId,
      requestRecord: path,
      tagObjectOid: "a".repeat(40),
      ...overrides,
    });
  }

  test("persists canonical owner-private create-only bytes before dispatch", () => {
    const ownerRoot = temporaryDirectory("provider-owner-record");
    chmodSync(ownerRoot, 0o700);
    const path = join(ownerRoot, "candidate-request.json");
    const expected = expectedRequestRecord(path);

    const created = releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      path,
      expected,
    );

    expect(created).toEqual(expected);
    expect(created).toMatchObject({
      currentCommit: commit,
      format: "takoform.provider-candidate-dispatch-request@v2",
      releaseCommit: commit,
      requestId,
      reservationCommit: commit,
      reservationRef: "refs/heads/provider-candidate-reservation-v1.0.4",
    });
    expect(readFileSync(path, "utf8")).toBe(
      `${JSON.stringify(recursivelySorted(expected))}\n`,
    );
    expect((lstatSync(path).mode & 0o777).toString(8)).toBe("600");
    expect(
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
      ),
    ).toEqual(expected);
    expect(() =>
      releaseDeployTestHooks.createProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
      ),
    ).toThrow("already exists");
  });

  test("rejects missing, divergent, linked, weak-mode, wrong-owner, and moved records", () => {
    const ownerRoot = temporaryDirectory("provider-owner-record-invalid");
    chmodSync(ownerRoot, 0o700);
    const missing = join(ownerRoot, "missing.json");
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        missing,
        expectedRequestRecord(missing),
      ),
    ).toThrow();

    const path = join(ownerRoot, "candidate-request.json");
    const expected = expectedRequestRecord(path);
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      path,
      expected,
    );
    writeFileSync(path, `${JSON.stringify({ ...expected, requestId: "fedcba98-7654-4321-8fed-cba987654321" })}\n`);
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
      ),
    ).toThrow();

    writeFileSync(
      path,
      `${JSON.stringify(recursivelySorted({
        ...expected,
        reservationRef:
          "refs/heads/provider-candidate-reservation-v1.0.4-other",
      }))}\n`,
    );
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
      ),
    ).toThrow("binding mismatch");

    writeFileSync(path, `${JSON.stringify(recursivelySorted(expected))}\n`);
    chmodSync(path, 0o644);
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
      ),
    ).toThrow("0600");
    chmodSync(path, 0o600);
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
        { getuid: () => process.getuid() + 1 },
      ),
    ).toThrow("owner");
    const wrongFileOwner = (descriptor) => {
      const stat = fstatSync(descriptor);
      return new Proxy(stat, {
        get(target, property) {
          if (property === "uid") return target.uid + 1;
          const value = Reflect.get(target, property, target);
          return typeof value === "function" ? value.bind(target) : value;
        },
      });
    };
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        path,
        expected,
        { fstat: wrongFileOwner },
      ),
    ).toThrow("record owner");

    const movedRoot = temporaryDirectory("provider-owner-record-moved");
    chmodSync(movedRoot, 0o700);
    const moved = join(movedRoot, "candidate-request.json");
    copyFileSync(path, moved);
    chmodSync(moved, 0o600);
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        moved,
        expectedRequestRecord(moved),
      ),
    ).toThrow("binding mismatch");

    const linked = join(ownerRoot, "linked.json");
    symlinkSync(path, linked);
    expect(() =>
      releaseDeployTestHooks.readProviderCandidateRequestRecord(
        repositoryRoot,
        linked,
        expectedRequestRecord(linked),
      ),
    ).toThrow();
  });

  test("rejects non-private, linked, missing, and source-tree parents", () => {
    const weakRoot = temporaryDirectory("provider-owner-parent-weak");
    chmodSync(weakRoot, 0o755);
    const weakPath = join(weakRoot, "candidate-request.json");
    expect(() =>
      releaseDeployTestHooks.createProviderCandidateRequestRecord(
        repositoryRoot,
        weakPath,
        expectedRequestRecord(weakPath),
      ),
    ).toThrow("0700");

    const physicalRoot = temporaryDirectory("provider-owner-parent-physical");
    chmodSync(physicalRoot, 0o700);
    const linkRoot = `${physicalRoot}-link`;
    symlinkSync(physicalRoot, linkRoot);
    temporaryDirectories.push(linkRoot);
    const linkedPath = join(linkRoot, "candidate-request.json");
    expect(() =>
      releaseDeployTestHooks.createProviderCandidateRequestRecord(
        repositoryRoot,
        linkedPath,
        expectedRequestRecord(linkedPath),
      ),
    ).toThrow("physical");

    const missingPath = join(
      temporaryDirectory("provider-owner-parent-missing"),
      "absent",
      "candidate-request.json",
    );
    expect(() =>
      releaseDeployTestHooks.createProviderCandidateRequestRecord(
        repositoryRoot,
        missingPath,
        expectedRequestRecord(missingPath),
      ),
    ).toThrow();

    const sourcePath = join(repositoryRoot, ".candidate-request.json");
    expect(() =>
      releaseDeployTestHooks.createProviderCandidateRequestRecord(
        repositoryRoot,
        sourcePath,
        expectedRequestRecord(sourcePath),
      ),
    ).toThrow("outside the source tree");
  });
});

describe("provider signed-tag materialization state machine", () => {
  const tag = "v1.0.4";
  const tagObject = "a".repeat(40);
  const runId = "30507374579";
  const runAttempt = "1";

  function tagExecution({
    local = "",
    remote = {},
    losePushAck = false,
  } = {}) {
    const calls = [];
    const state = {
      local,
      remote: { ...remote },
    };
    const run = {
      attempt: 1,
      conclusion: "success",
      databaseId: Number(runId),
      displayTitle: requestId,
      event: "workflow_dispatch",
      headBranch: providerReleaseBranch,
      headSha: commit,
      status: "completed",
      url:
        `https://github.com/tako0614/terraform-provider-takoform/actions/runs/${runId}/attempts/${runAttempt}`,
      workflowName: "Author provider release tag",
    };
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (2026-07-02)\n";
      }
      if (executable === "cosign" && args[0] === "version") {
        return "GitVersion:    v3.0.6\n";
      }
      if (
        executable === "bun" &&
        args.join(" ") === "run check:release-owner-gate"
      ) {
        return "";
      }
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return `${providerReleaseBranch}\n`;
        if (args[0] === "fetch") return "";
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === `rev-parse ${providerReleaseRemoteRef}`
        ) {
          return `${commit}\n`;
        }
        if (args[0] === "cat-file") return "";
        if (args[0] === "for-each-ref") return `${state.local}\n`;
        if (args[0] === "ls-remote") {
          return [
            state.remote.object
              ? `${state.remote.object}\trefs/tags/${tag}`
              : "",
            state.remote.commit
              ? `${state.remote.commit}\trefs/tags/${tag}^{}`
              : "",
          ].filter(Boolean).join("\n");
        }
        if (args[0] === "update-ref") {
          state.local = tagObject;
          return "";
        }
        if (args[0] === "push") {
          state.remote = { object: tagObject, commit };
          if (losePushAck) {
            throw commandFailure("connection closed after server accepted push");
          }
          return "";
        }
      }
      if (executable === "gh" && isReleaseList(args)) return "[[]]";
      if (executable === "curl" && args.at(-1).endsWith("/versions")) {
        return '{"versions":[]}';
      }
      if (executable === "gh" && args[0] === "run" && args[1] === "view") {
        return JSON.stringify(run);
      }
      if (
        executable === "gh" &&
        args[0] === "run" &&
        args[1] === "download"
      ) {
        return "";
      }
      if (
        executable === "go" &&
        args.includes("verify-tag-artifact")
      ) {
        return JSON.stringify({
          kind: "takoform.provider-signed-tag-verification@v1",
          requestId,
          releaseTag: tag,
          sourceCommit: commit,
          workflowRun: run.url,
          preflightSha256: `sha256:${"b".repeat(64)}`,
          tagObjectOid: tagObject,
          tagObjectSha256: `sha256:${"c".repeat(64)}`,
          signerFingerprint:
            "3510E75E05BBCC303B92D77934FC18AC897FB709",
          localRefMaterialized: false,
          verified: true,
        });
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    return { calls, fake, state };
  }

  function runTag(execution) {
    return runReleaseSurface({
      surface: "takoform-provider-release",
      args: [
        "tag",
        "--tag",
        tag,
        "--expected-commit",
        commit,
        "--run-id",
        runId,
        "--run-attempt",
        runAttempt,
      ],
      repo: repositoryRoot,
      execFile: execution.fake,
      stdout: memoryIO().stdout,
      stderr: memoryIO().stderr,
    });
  }

  test("adopts an authoritative exact remote tag after a prior crash without dispatch", () => {
    const execution = tagExecution({
      remote: { object: tagObject, commit },
    });
    const result = runTag(execution);
    expect(result.status).toBe("TAG_READY");
    expect(result.tagObject).toBe(tagObject);
    expect(execution.state.local).toBe(tagObject);
    expect(
      execution.calls.filter(
        (call) => call.executable === "git" && call.args[0] === "push",
      ),
    ).toHaveLength(0);
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(0);
  });

  test("reconciles a lost push acknowledgement from authoritative exact remote state", () => {
    const execution = tagExecution({ losePushAck: true });
    const result = runTag(execution);
    expect(result.status).toBe("TAG_READY");
    expect(result.tagReconciliation).toContain("REMOTE_PUSH_ACK_LOST");
    expect(execution.state.remote).toEqual({ object: tagObject, commit });
    expect(
      execution.calls.filter(
        (call) => call.executable === "git" && call.args[0] === "push",
      ),
    ).toHaveLength(1);
  });

  test("resumes an exact local-only signed tag without rematerializing it", () => {
    const execution = tagExecution({ local: tagObject });
    const result = runTag(execution);
    expect(result.status).toBe("TAG_READY");
    expect(execution.state.remote).toEqual({ object: tagObject, commit });
    expect(
      execution.calls.filter(
        (call) => call.executable === "git" && call.args[0] === "update-ref",
      ),
    ).toHaveLength(0);
  });

  test("fails closed on a drifted remote tag object before any writer", () => {
    const execution = tagExecution({
      remote: { object: "d".repeat(40), commit },
    });
    expect(() => runTag(execution)).toThrow("signed tag state drift");
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "git" &&
          ["update-ref", "push"].includes(call.args[0]),
      ),
    ).toHaveLength(0);
  });
});

describe("provider candidate dispatch and read-only reconciliation", () => {
  const tag = "v1.0.4";
  const tagObject = "a".repeat(40);
  const signer = "3510E75E05BBCC303B92D77934FC18AC897FB709";
  const reservationBranch = `provider-candidate-reservation-${tag}`;
  const reservationRef = `refs/heads/${reservationBranch}`;
  const reservationOwner = { id: 96359093, login: "tako0614", type: "User" };
  const creationRulesetID = 7001;
  const immutableRulesetID = 7002;
  const reservationPatterns = [
    "refs/heads/provider-candidate-reservation-v*",
    "refs/heads/provider-candidate-reservation-v*/**/*",
  ];

  function exactReservationRulesets() {
    return [
      {
        bypass_actors: [
          {
            actor_id: reservationOwner.id,
            actor_type: "User",
            bypass_mode: "always",
          },
        ],
        conditions: {
          ref_name: { exclude: [], include: reservationPatterns },
        },
        current_user_can_bypass: "always",
        enforcement: "active",
        id: creationRulesetID,
        name: "Restrict provider candidate reservation creation",
        rules: [{ type: "creation" }],
        source: "tako0614/terraform-provider-takoform",
        source_type: "Repository",
        target: "branch",
      },
      {
        bypass_actors: [],
        conditions: {
          ref_name: { exclude: [], include: reservationPatterns },
        },
        current_user_can_bypass: "never",
        enforcement: "active",
        id: immutableRulesetID,
        name: "Keep provider candidate reservations immutable",
        rules: [{ type: "deletion" }, { type: "update" }],
        source: "tako0614/terraform-provider-takoform",
        source_type: "Repository",
        target: "branch",
      },
    ];
  }

  function exactEvaluatedReservationRules() {
    return [
      {
        ruleset_id: creationRulesetID,
        ruleset_source: "tako0614/terraform-provider-takoform",
        ruleset_source_type: "Repository",
        type: "creation",
      },
      {
        ruleset_id: immutableRulesetID,
        ruleset_source: "tako0614/terraform-provider-takoform",
        ruleset_source_type: "Repository",
        type: "deletion",
      },
      {
        ruleset_id: immutableRulesetID,
        ruleset_source: "tako0614/terraform-provider-takoform",
        ruleset_source_type: "Repository",
        type: "update",
      },
    ];
  }

  function reservationAPIRef(sha, ref = reservationRef) {
    return {
      object: {
        sha,
        type: "commit",
        url: `https://api.github.com/repos/tako0614/terraform-provider-takoform/git/commits/${sha}`,
      },
      ref,
      url:
        "https://api.github.com/repos/tako0614/terraform-provider-takoform/git/refs/heads/" +
        reservationBranch,
    };
  }

  function candidateExecution(
    requestRecord,
    {
      branch = providerReleaseBranch,
      currentCommit = commit,
      dispatch = "success",
      localObject = tagObject,
      recoveryPaths = [],
      releaseCommit = commit,
      remoteObject = tagObject,
      remoteCommit = releaseCommit,
      releases = "[[]]",
      registryVersions = [],
      reservationCreate = "success",
      reservationState,
      reservationRulesets = exactReservationRulesets(),
      reservationRulesetsAfterCreate,
      evaluatedExact = exactEvaluatedReservationRules(),
      evaluatedDescendant = exactEvaluatedReservationRules(),
      evaluatedAfterCreate,
      reservationOperator = reservationOwner,
      runs = [],
    } = {},
  ) {
    const calls = [];
    const sharedReservation = reservationState ?? {
      descendants: [],
      exactCommit: existsSync(requestRecord) ? currentCommit : null,
    };
    const state = {
      dispatch,
      reservation: sharedReservation,
      reservationCreate,
      rulesetReads: 0,
      runs: [...runs],
    };
    const exactRun = (overrides = {}) => ({
      conclusion: null,
      display_title: requestId,
      event: "workflow_dispatch",
      head_branch: tag,
      head_sha: releaseCommit,
      html_url:
        "https://github.com/tako0614/terraform-provider-takoform/actions/runs/4242",
      id: 4242,
      name: requestId,
      path: ".github/workflows/release.yml",
      run_attempt: 1,
      status: "queued",
      ...overrides,
    });
    const signedTag = Buffer.from(
      `object ${releaseCommit}\ntype commit\ntag ${tag}\n` +
        "tagger Takoform Provider Release <release@takoform.invalid> 1 +0000\n\n" +
        "exact provider release\n" +
        "-----BEGIN PGP SIGNATURE-----\nopaque\n-----END PGP SIGNATURE-----\n",
    );
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      const apiEndpoint =
        executable === "gh" && args[0] === "api"
          ? args.find(
              (argument) =>
                argument === "user" || argument.startsWith("repos/"),
            )
          : null;
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (2026-07-02)\n";
      }
      if (executable === "cosign" && args[0] === "version") {
        return "GitVersion:    v3.0.6\n";
      }
      if (
        executable === "bun" &&
        args.join(" ") === "run check:release-owner-gate"
      ) {
        return "";
      }
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return `${branch}\n`;
        if (args[0] === "fetch") return "";
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === `rev-parse ${providerReleaseRemoteRef}`
        ) {
          return `${currentCommit}\n`;
        }
        if (args.join(" ") === `rev-parse refs/tags/${tag}^{commit}`) {
          return `${releaseCommit}\n`;
        }
        if (args[0] === "merge-base") return "";
        if (args[0] === "-c" && args.includes("diff")) {
          return recoveryPaths.length === 0
            ? ""
            : `${recoveryPaths.join("\0")}\0`;
        }
        if (args[0] === "for-each-ref") return `${localObject}\n`;
        if (args[0] === "ls-remote") {
          return [
            remoteObject ? `${remoteObject}\trefs/tags/${tag}` : "",
            remoteCommit ? `${remoteCommit}\trefs/tags/${tag}^{}` : "",
          ].filter(Boolean).join("\n");
        }
        if (args.join(" ") === `cat-file -t refs/tags/${tag}`) {
          return "tag\n";
        }
        if (args.join(" ") === `cat-file tag ${tagObject}`) {
          return signedTag;
        }
        if (args[0] === "cat-file") return "";
      }
      if (executable.endsWith("/gpg")) {
        if (args.includes("show-only")) {
          return `fpr:::::::::${signer}:\n`;
        }
        if (args.includes("--verify")) {
          return `[GNUPG:] VALIDSIG ${signer} 2026-07-30 0 4 0 1 10 00 ${signer}\n`;
        }
        return "";
      }
      if (executable === "gh" && isReleaseList(args)) return releases;
      if (executable === "curl" && args.at(-1).endsWith("/versions")) {
        return JSON.stringify({
          versions: registryVersions.map((version) => ({ version })),
        });
      }
      if (apiEndpoint === "user") {
        return JSON.stringify(reservationOperator);
      }
      if (apiEndpoint?.includes("/rulesets?")) {
        state.rulesetReads += 1;
        const active =
          state.reservation.exactCommit && reservationRulesetsAfterCreate
            ? reservationRulesetsAfterCreate
            : reservationRulesets;
        state.lastRulesets = active;
        return JSON.stringify([
          active.map((ruleset) => ({
            enforcement: ruleset.enforcement,
            id: ruleset.id,
            name: ruleset.name,
            source: ruleset.source,
            source_type: ruleset.source_type,
            target: ruleset.target,
          })),
        ]);
      }
      const rulesetDetail = apiEndpoint?.match(/\/rulesets\/([0-9]+)\?/u);
      if (rulesetDetail) {
        const ruleset = (state.lastRulesets ?? reservationRulesets).find(
          (entry) => entry.id === Number(rulesetDetail[1]),
        );
        if (!ruleset) throw commandFailure("ruleset detail missing", 404);
        return JSON.stringify(ruleset);
      }
      if (apiEndpoint?.includes("/rules/branches/")) {
        const active =
          state.reservation.exactCommit && evaluatedAfterCreate
            ? evaluatedAfterCreate
            : apiEndpoint.includes("%2F")
              ? evaluatedDescendant
              : evaluatedExact;
        return JSON.stringify([active]);
      }
      if (apiEndpoint?.includes("/git/matching-refs/heads/")) {
        const refs = [];
        if (state.reservation.exactCommit) {
          refs.push(reservationAPIRef(state.reservation.exactCommit));
        }
        for (const descendant of state.reservation.descendants) {
          refs.push(
            reservationAPIRef(
              descendant.commit ?? currentCommit,
              `${reservationRef}/${descendant.name}`,
            ),
          );
        }
        return JSON.stringify([refs]);
      }
      if (
        apiEndpoint?.endsWith(`/git/ref/heads/${reservationBranch}`) &&
        !args.includes("POST")
      ) {
        if (!state.reservation.exactCommit) {
          throw commandFailure("reservation ref missing", 404);
        }
        return JSON.stringify(
          reservationAPIRef(state.reservation.exactCommit),
        );
      }
      if (
        apiEndpoint?.endsWith("/git/refs") &&
        args.includes("POST")
      ) {
        expect(existsSync(requestRecord)).toBe(true);
        expect(args).toContain(`ref=${reservationRef}`);
        expect(args).toContain(`sha=${currentCommit}`);
        if (state.reservationCreate === "lost-ack") {
          state.reservation.exactCommit = currentCommit;
          throw commandFailure("connection closed after ref creation");
        }
        if (state.reservationCreate === "competing-exact") {
          state.reservation.exactCommit = currentCommit;
          throw commandFailure("reference already exists", 422);
        }
        if (state.reservationCreate === "failure") {
          throw commandFailure("ref creation rejected", 422);
        }
        if (
          state.reservation.exactCommit ||
          state.reservation.descendants.length !== 0
        ) {
          throw commandFailure("reference already exists", 422);
        }
        state.reservation.exactCommit = currentCommit;
        const status =
          state.reservationCreate === "wrong-status" ? 200 : 201;
        const responseCommit =
          state.reservationCreate === "drifted-response"
            ? laterCommit
            : currentCommit;
        return (
          `HTTP/2.0 ${status} ${status === 201 ? "Created" : "OK"}\n` +
          "Content-Type: application/json\n\n" +
          JSON.stringify(reservationAPIRef(responseCommit))
        );
      }
      if (
        executable === "gh" &&
        args[0] === "api" &&
        args.some((argument) =>
          argument.includes("actions/workflows/release.yml/runs?"),
        )
      ) {
        return JSON.stringify([
          { total_count: state.runs.length, workflow_runs: state.runs },
        ]);
      }
      if (
        executable === "gh" &&
        args[0] === "workflow" &&
        args[1] === "run"
      ) {
        expect(existsSync(requestRecord)).toBe(true);
        expect(state.reservation.exactCommit).toBe(currentCommit);
        expect(
          JSON.parse(readFileSync(requestRecord, "utf8")).dispatchAttempted,
        ).toBe(true);
        if (state.dispatch === "success" || state.dispatch === "lost-ack") {
          state.runs.push(exactRun());
        }
        if (state.dispatch === "lost-ack") {
          throw commandFailure("connection closed before response");
        }
        if (state.dispatch === "failure") {
          throw commandFailure("dispatch request rejected");
        }
        return "raw candidate dispatch response must not leak";
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    return { calls, exactRun, fake, state };
  }

  function candidateArgs(phase, requestRecord, overrides = {}) {
    return [
      phase,
      "--tag",
      tag,
      "--expected-release-commit",
      overrides.releaseCommit ?? commit,
      "--expected-current-commit",
      overrides.currentCommit ?? commit,
      "--expected-tag-object",
      overrides.tagObject ?? tagObject,
      "--request-id",
      overrides.requestId ?? requestId,
      "--request-record",
      requestRecord,
    ];
  }

  function runCandidate(execution, phase, requestRecord, overrides = {}) {
    const io = memoryIO();
    const result = runReleaseSurface({
      surface: "takoform-provider-release",
      args: candidateArgs(phase, requestRecord, overrides),
      repo: repositoryRoot,
      execFile: execution.fake,
      stdout: io.stdout,
      stderr: io.stderr,
    });
    return { io, result };
  }

  function ownerRecordPath(prefix) {
    const root = temporaryDirectory(prefix);
    chmodSync(root, 0o700);
    return join(root, "candidate-request.json");
  }

  test("durably records the sole candidate dispatch attempt and then halts", () => {
    const requestRecord = ownerRecordPath("provider-candidate-dispatch");
    const execution = candidateExecution(requestRecord);
    const { io, result } = runCandidate(
      execution,
      "prepare-candidate",
      requestRecord,
    );
    expect(result.status).toBe("RECONCILIATION_REQUIRED");
    expect(result.dispatchStatus).toBe("ATTEMPTED_ONCE");
    expect(io.output).not.toContain(requestRecord);
    expect(io.output).not.toContain("raw candidate dispatch response");
    const durable = JSON.parse(readFileSync(requestRecord, "utf8"));
    expect(durable.releaseCommit).toBe(commit);
    expect(durable.currentCommit).toBe(commit);
    expect(durable.reservationCommit).toBe(commit);
    expect(durable.reservationRef).toBe(reservationRef);
    const reservationCreateIndex = execution.calls.findIndex(
      (call) =>
        call.executable === "gh" &&
        call.args.includes("POST") &&
        call.args.includes(`ref=${reservationRef}`),
    );
    const dispatchIndex = execution.calls.findIndex(
      (call) =>
        call.executable === "gh" &&
        call.args[0] === "workflow" &&
        call.args[1] === "run",
    );
    expect(reservationCreateIndex).toBeGreaterThan(-1);
    expect(dispatchIndex).toBeGreaterThan(reservationCreateIndex);
    expect(
      execution.calls.some(
        (call) =>
          call.executable === "gh" &&
          call.args.some((argument) =>
            argument.includes(
              `/rules/branches/${reservationBranch}%2Fruleset-probe?`,
            ),
          ),
      ),
    ).toBe(true);
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(1);
  });

  test("missing, bypassable, or unevaluated reservation rules fail before every writer", () => {
    const missingDescendant = exactReservationRulesets();
    missingDescendant[0].conditions.ref_name.include = [
      reservationPatterns[0],
    ];
    const wrongActor = exactReservationRulesets();
    wrongActor[0].bypass_actors[0].actor_id += 1;
    const bypassableImmutable = exactReservationRulesets();
    bypassableImmutable[1].bypass_actors = [
      {
        actor_id: reservationOwner.id,
        actor_type: "User",
        bypass_mode: "always",
      },
    ];
    bypassableImmutable[1].current_user_can_bypass = "always";
    const missingUpdate = exactReservationRulesets();
    missingUpdate[1].rules = [{ type: "deletion" }];
    const cases = [
      { options: { reservationRulesets: [] }, message: "rulesets are absent" },
      {
        options: { reservationRulesets: missingDescendant },
        message: "ambiguous or incomplete",
      },
      {
        options: { reservationRulesets: wrongActor },
        message: "exact operator User",
      },
      {
        options: { reservationRulesets: bypassableImmutable },
        message: "protection has a bypass",
      },
      {
        options: { reservationRulesets: missingUpdate },
        message: "distinct exact creation and immutable rulesets",
      },
      {
        options: {
          evaluatedExact: exactEvaluatedReservationRules().slice(0, 2),
        },
        message: "evaluated rules are incomplete",
      },
      {
        options: {
          evaluatedDescendant:
            exactEvaluatedReservationRules().slice(1),
        },
        message: "evaluated rules are incomplete",
      },
      {
        options: {
          reservationOperator: {
            id: reservationOwner.id + 1,
            login: reservationOwner.login,
            type: "User",
          },
        },
        message: "exact repository owner",
      },
    ];
    for (const [index, scenario] of cases.entries()) {
      const requestRecord = ownerRecordPath(
        `provider-candidate-reservation-rules-${index}`,
      );
      const execution = candidateExecution(requestRecord, scenario.options);
      expect(() =>
        runCandidate(execution, "prepare-candidate", requestRecord),
      ).toThrow(scenario.message);
      expect(existsSync(requestRecord)).toBe(false);
      expect(
        execution.calls.some(
          (call) =>
            call.executable === "gh" &&
            ((call.args[0] === "workflow" && call.args[1] === "run") ||
              (call.args.includes("POST") &&
                call.args.includes(`ref=${reservationRef}`))),
        ),
      ).toBe(false);
    }
  });

  test("reconcile adopts one exact run after a lost dispatch acknowledgement", () => {
    const requestRecord = ownerRecordPath("provider-candidate-lost-ack");
    const execution = candidateExecution(requestRecord, {
      dispatch: "lost-ack",
    });
    expect(() =>
      runCandidate(execution, "prepare-candidate", requestRecord),
    ).toThrow("dispatch acknowledgement is unresolved");
    const { result } = runCandidate(
      execution,
      "reconcile-candidate",
      requestRecord,
    );
    expect(result.status).toBe("AWAITING_REVIEW");
    expect(result.workflowRun.runId).toBe("4242");
    expect(result.workflowRun.requestId).toBe(requestId);
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(1);
  });

  test("a global tag reservation blocks an alternate request after lost visibility", () => {
    const sharedReservation = { descendants: [], exactCommit: null };
    const firstRecord = ownerRecordPath("provider-candidate-global-first");
    const first = candidateExecution(firstRecord, {
      dispatch: "lost-ack",
      reservationState: sharedReservation,
    });
    expect(() =>
      runCandidate(first, "prepare-candidate", firstRecord),
    ).toThrow("dispatch acknowledgement is unresolved");

    const secondRecord = ownerRecordPath("provider-candidate-global-second");
    const secondRequestId = "11111111-2222-4333-8444-555555555555";
    const freshClone = candidateExecution(secondRecord, {
      reservationState: sharedReservation,
    });
    expect(() =>
      runCandidate(freshClone, "prepare-candidate", secondRecord, {
        requestId: secondRequestId,
      }),
    ).toThrow("already reserved");
    expect(existsSync(secondRecord)).toBe(false);
    expect(
      freshClone.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(0);
  });

  test("a lost reservation-create acknowledgement is permanently reconcile-only", () => {
    const sharedReservation = { descendants: [], exactCommit: null };
    const requestRecord = ownerRecordPath(
      "provider-candidate-reservation-lost-ack",
    );
    const execution = candidateExecution(requestRecord, {
      reservationCreate: "lost-ack",
      reservationState: sharedReservation,
    });
    expect(() =>
      runCandidate(execution, "prepare-candidate", requestRecord),
    ).toThrow("reservation creation acknowledgement is unresolved");
    expect(existsSync(requestRecord)).toBe(true);
    expect(sharedReservation.exactCommit).toBe(commit);
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(0);
    expect(
      runCandidate(
        execution,
        "reconcile-candidate",
        requestRecord,
      ).result.status,
    ).toBe("UNRESOLVED_ABSENT");
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args.includes("POST") &&
          call.args.includes(`ref=${reservationRef}`),
      ),
    ).toHaveLength(1);
  });

  test("a competing exact reservation creator is never adopted for dispatch", () => {
    const sharedReservation = { descendants: [], exactCommit: null };
    const firstRecord = ownerRecordPath(
      "provider-candidate-reservation-competing",
    );
    const first = candidateExecution(firstRecord, {
      reservationCreate: "competing-exact",
      reservationState: sharedReservation,
    });
    expect(() =>
      runCandidate(first, "prepare-candidate", firstRecord),
    ).toThrow("reservation creation acknowledgement is unresolved");
    expect(sharedReservation.exactCommit).toBe(commit);
    expect(
      first.calls.some(
        (call) =>
          call.executable === "gh" && call.args[0] === "workflow",
      ),
    ).toBe(false);

    const secondRecord = ownerRecordPath(
      "provider-candidate-reservation-competing-second",
    );
    const second = candidateExecution(secondRecord, {
      reservationState: sharedReservation,
    });
    expect(() =>
      runCandidate(second, "prepare-candidate", secondRecord, {
        requestId: "11111111-2222-4333-8444-555555555555",
      }),
    ).toThrow("already reserved");
    expect(existsSync(secondRecord)).toBe(false);
    expect(
      second.calls.some(
        (call) =>
          call.executable === "gh" &&
          (call.args[0] === "workflow" || call.args.includes("POST")),
      ),
    ).toBe(false);
  });

  test("pre-existing exact or descendant reservation refs block before local record creation", () => {
    const cases = [
      {
        message: "already reserved",
        state: { descendants: [], exactCommit: commit },
      },
      {
        message: "descendant ref conflict",
        state: {
          descendants: [{ commit, name: "blocker" }],
          exactCommit: null,
        },
      },
    ];
    for (const [index, scenario] of cases.entries()) {
      const requestRecord = ownerRecordPath(
        `provider-candidate-reservation-preexisting-${index}`,
      );
      const execution = candidateExecution(requestRecord, {
        reservationState: scenario.state,
      });
      expect(() =>
        runCandidate(execution, "prepare-candidate", requestRecord),
      ).toThrow(scenario.message);
      expect(existsSync(requestRecord)).toBe(false);
      expect(
        execution.calls.some(
          (call) =>
            call.executable === "gh" &&
            (call.args[0] === "workflow" || call.args.includes("POST")),
        ),
      ).toBe(false);
    }
  });

  test("non-201 or drifted reservation creation responses can never dispatch", () => {
    for (const mode of ["wrong-status", "drifted-response"]) {
      const requestRecord = ownerRecordPath(
        `provider-candidate-reservation-response-${mode}`,
      );
      const execution = candidateExecution(requestRecord, {
        reservationCreate: mode,
      });
      expect(() =>
        runCandidate(execution, "prepare-candidate", requestRecord),
      ).toThrow("reservation creation acknowledgement is unresolved");
      expect(existsSync(requestRecord)).toBe(true);
      expect(
        execution.calls.some(
          (call) =>
            call.executable === "gh" && call.args[0] === "workflow",
        ),
      ).toBe(false);
    }
  });

  test("reservation ruleset drift after create strands the ref before dispatch", () => {
    const driftedRulesets = exactReservationRulesets();
    driftedRulesets[0].id = creationRulesetID + 100;
    driftedRulesets[1].id = immutableRulesetID + 100;
    const driftedEvaluated = exactEvaluatedReservationRules().map((rule) => ({
      ...rule,
      ruleset_id:
        rule.ruleset_id === creationRulesetID
          ? creationRulesetID + 100
          : immutableRulesetID + 100,
    }));
    const requestRecord = ownerRecordPath(
      "provider-candidate-reservation-rules-drift",
    );
    const execution = candidateExecution(requestRecord, {
      evaluatedAfterCreate: driftedEvaluated,
      reservationRulesetsAfterCreate: driftedRulesets,
    });
    expect(() =>
      runCandidate(execution, "prepare-candidate", requestRecord),
    ).toThrow("protection changed after creation");
    expect(existsSync(requestRecord)).toBe(true);
    expect(execution.state.reservation.exactCommit).toBe(commit);
    expect(
      execution.calls.some(
        (call) =>
          call.executable === "gh" && call.args[0] === "workflow",
      ),
    ).toBe(false);
  });

  test("reconciliation rejects a reservation ref that moved away from F", () => {
    const requestRecord = ownerRecordPath(
      "provider-candidate-reservation-ref-drift",
    );
    const record = releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: tag,
      requestId,
      requestRecord,
      tagObjectOid: tagObject,
    });
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      requestRecord,
      record,
    );
    const execution = candidateExecution(requestRecord, {
      reservationState: { descendants: [], exactCommit: laterCommit },
    });
    expect(() =>
      runCandidate(execution, "reconcile-candidate", requestRecord),
    ).toThrow("ref readback drifted");
    expect(
      execution.calls.some(
        (call) =>
          call.executable === "gh" &&
          (call.args[0] === "workflow" || call.args.includes("POST")),
      ),
    ).toBe(false);
  });

  test("a failed dispatch remains unresolved and can never be retried in create mode", () => {
    const requestRecord = ownerRecordPath("provider-candidate-failed");
    const execution = candidateExecution(requestRecord, {
      dispatch: "failure",
    });
    expect(() =>
      runCandidate(execution, "prepare-candidate", requestRecord),
    ).toThrow("dispatch acknowledgement is unresolved");
    expect(
      runCandidate(execution, "reconcile-candidate", requestRecord).result
        .status,
    ).toBe("UNRESOLVED_ABSENT");
    expect(() =>
      runCandidate(execution, "prepare-candidate", requestRecord),
    ).toThrow("already exists");
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(1);
    const freshClone = candidateExecution(requestRecord);
    expect(() =>
      runCandidate(freshClone, "prepare-candidate", requestRecord),
    ).toThrow("already exists");
    expect(
      freshClone.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(0);
  });

  test("create mode refuses to adopt a pre-existing request run", () => {
    const requestRecord = ownerRecordPath("provider-candidate-preexisting");
    const seed = candidateExecution(requestRecord);
    const execution = candidateExecution(requestRecord, {
      runs: [seed.exactRun()],
    });
    expect(() =>
      runCandidate(execution, "prepare-candidate", requestRecord),
    ).toThrow("already has a workflow run");
    expect(existsSync(requestRecord)).toBe(false);
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(0);
  });

  test("reconciliation is strictly read-only and fails on ambiguous exact runs", () => {
    const requestRecord = ownerRecordPath("provider-candidate-ambiguous");
    const seed = candidateExecution(requestRecord);
    const record = releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: tag,
      requestId,
      requestRecord,
      tagObjectOid: tagObject,
    });
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      requestRecord,
      record,
    );
    const execution = candidateExecution(requestRecord, {
      runs: [seed.exactRun(), seed.exactRun({ id: 4243, html_url:
        "https://github.com/tako0614/terraform-provider-takoform/actions/runs/4243" })],
    });
    expect(() =>
      runCandidate(execution, "reconcile-candidate", requestRecord),
    ).toThrow("ambiguous");
    expect(
      execution.calls.some(
        (call) =>
          (call.executable === "gh" && call.args[0] === "workflow") ||
          (call.executable === "git" &&
            ["push", "update-ref"].includes(call.args[0])),
      ),
    ).toBe(false);
  });

  test("a crash after the durable attempt record is reconcile-only and stays absent", () => {
    const requestRecord = ownerRecordPath("provider-candidate-pre-post-crash");
    const record = releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: tag,
      requestId,
      requestRecord,
      tagObjectOid: tagObject,
    });
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      requestRecord,
      record,
    );
    const execution = candidateExecution(requestRecord);
    const { result } = runCandidate(
      execution,
      "reconcile-candidate",
      requestRecord,
    );
    expect(result.status).toBe("UNRESOLVED_ABSENT");
    expect(
      execution.calls.filter(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "workflow" &&
          call.args[1] === "run",
      ),
    ).toHaveLength(0);
  });

  test("a crash before global reservation creation strands without creating or dispatching", () => {
    const requestRecord = ownerRecordPath(
      "provider-candidate-pre-reservation-crash",
    );
    const record = releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: tag,
      requestId,
      requestRecord,
      tagObjectOid: tagObject,
    });
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      requestRecord,
      record,
    );
    const execution = candidateExecution(requestRecord, {
      reservationState: { descendants: [], exactCommit: null },
    });
    expect(() =>
      runCandidate(execution, "reconcile-candidate", requestRecord),
    ).toThrow("reservation ref readback is unreadable");
    expect(
      execution.calls.some(
        (call) =>
          call.executable === "gh" &&
          (call.args[0] === "workflow" || call.args.includes("POST")),
      ),
    ).toBe(false);
  });

  test("reconcile rejects a drifted exact-request run and a different durable request", () => {
    const requestRecord = ownerRecordPath("provider-candidate-wrong-run");
    const record = releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: tag,
      requestId,
      requestRecord,
      tagObjectOid: tagObject,
    });
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      requestRecord,
      record,
    );
    const seed = candidateExecution(requestRecord);
    const wrongRun = candidateExecution(requestRecord, {
      runs: [seed.exactRun({ head_branch: "main" })],
    });
    expect(() =>
      runCandidate(wrongRun, "reconcile-candidate", requestRecord),
    ).toThrow("drifted workflow run");

    const differentRecord = ownerRecordPath("provider-candidate-wrong-request");
    const differentRequestId = "11111111-2222-4333-8444-555555555555";
    const different = releaseDeployTestHooks.providerCandidateRequestRecord({
      currentCommit: commit,
      releaseCommit: commit,
      releaseTag: tag,
      requestId: differentRequestId,
      requestRecord: differentRecord,
      tagObjectOid: tagObject,
    });
    releaseDeployTestHooks.createProviderCandidateRequestRecord(
      repositoryRoot,
      differentRecord,
      different,
    );
    const wrongRequest = candidateExecution(differentRecord);
    expect(() =>
      runCandidate(wrongRequest, "reconcile-candidate", differentRecord),
    ).toThrow("binding mismatch");
  });

  test("wrong branch, tag object, Release, and Registry state fail before record or dispatch", () => {
    const cases = [
      { branch: "main" },
      { localObject: "d".repeat(40) },
      { remoteObject: "d".repeat(40) },
      {
        releases: JSON.stringify([
          [{ id: 99, tag_name: tag, draft: false }],
        ]),
      },
      { registryVersions: ["1.0.4"] },
    ];
    for (const [index, scenario] of cases.entries()) {
      const requestRecord = ownerRecordPath(
        `provider-candidate-fail-closed-${index}`,
      );
      const execution = candidateExecution(requestRecord, scenario);
      expect(() =>
        runCandidate(execution, "prepare-candidate", requestRecord),
      ).toThrow();
      expect(existsSync(requestRecord)).toBe(false);
      expect(
        execution.calls.some(
          (call) =>
            (call.executable === "gh" && call.args[0] === "workflow") ||
            (call.executable === "git" &&
              ["push", "update-ref"].includes(call.args[0])),
        ),
      ).toBe(false);
    }
  });

  test("permits E before F only through the exact reviewed recovery diff fence", () => {
    const allowedPaths = [
      "release/README.md",
      "scripts/release-deploy.mjs",
      "scripts/release-deploy.test.mjs",
    ];
    const requestRecord = ownerRecordPath("provider-candidate-reviewed-ef");
    const execution = candidateExecution(requestRecord, {
      currentCommit: laterCommit,
      recoveryPaths: allowedPaths,
      releaseCommit: commit,
    });
    const { result } = runCandidate(
      execution,
      "prepare-candidate",
      requestRecord,
      { releaseCommit: commit, currentCommit: laterCommit },
    );
    expect(result.status).toBe("RECONCILIATION_REQUIRED");
    expect(JSON.parse(readFileSync(requestRecord, "utf8"))).toMatchObject({
      currentCommit: laterCommit,
      releaseCommit: commit,
    });

    const rejectedRecord = ownerRecordPath(
      "provider-candidate-unreviewed-ef",
    );
    const rejected = candidateExecution(rejectedRecord, {
      currentCommit: laterCommit,
      recoveryPaths: [
        "scripts/release-deploy.mjs",
        ".github/workflows/release.yml",
      ],
      releaseCommit: commit,
    });
    expect(() =>
      runCandidate(rejected, "prepare-candidate", rejectedRecord, {
        releaseCommit: commit,
        currentCommit: laterCommit,
      }),
    ).toThrow("exact reviewed recovery implementation");
    expect(existsSync(rejectedRecord)).toBe(false);
    expect(
      rejected.calls.some(
        (call) =>
          call.executable === "gh" && call.args[0] === "workflow",
      ),
    ).toBe(false);
  });
});

describe("owner gate final fence and pinned release tools", () => {
  function toolOutput(executable, args, version = "v3.0.6") {
    if (executable === "gh" && args[0] === "--version") {
      return "gh version 2.96.0 (2026-07-02)\n";
    }
    if (executable === "cosign" && args[0] === "version") {
      return `GitVersion:    ${version}\n`;
    }
    return null;
  }

  for (const scenario of [
    {
      name: "main substitution",
      branch: "main",
      remote: commit,
      dirty: "",
      error: "attached to maintenance/v1",
    },
    {
      name: "detached HEAD",
      branch: null,
      remote: commit,
      dirty: "",
      error: "attached to maintenance/v1",
    },
    {
      name: "stale maintenance checkout",
      branch: providerReleaseBranch,
      remote: "89abcdef0123456789abcdef0123456789abcdef",
      dirty: "",
      error: "fresh origin/maintenance/v1",
    },
    {
      name: "dirty maintenance checkout",
      branch: providerReleaseBranch,
      remote: commit,
      dirty: " M scripts/release-deploy.mjs\n",
      error: "worktree is dirty",
    },
  ]) {
    test(`provider refuses ${scenario.name} before any writer`, () => {
      const calls = [];
      const fake = (executable, args) => {
        calls.push({ executable, args: [...args] });
        const version = toolOutput(executable, args);
        if (version !== null) return version;
        if (executable === "bun") return "";
        if (executable === "git") {
          if (args[0] === "status") return scenario.dirty;
          if (args.join(" ") === "rev-parse --is-shallow-repository") {
            return "false\n";
          }
          if (args.join(" ") === "remote get-url origin") {
            return "https://github.com/tako0614/terraform-provider-takoform.git\n";
          }
          if (args[0] === "symbolic-ref") {
            if (scenario.branch === null) {
              throw commandFailure("fatal: ref HEAD is not a symbolic ref\n");
            }
            return `${scenario.branch}\n`;
          }
          if (args[0] === "fetch") return "";
          if (args.join(" ") === "rev-parse HEAD") return `${commit}\n`;
          if (args.join(" ") === `rev-parse ${providerReleaseRemoteRef}`) {
            return `${scenario.remote}\n`;
          }
          if (args.join(" ") === "rev-parse refs/remotes/origin/main") {
            return `${commit}\n`;
          }
        }
        throw new Error(`unexpected ${executable} ${args.join(" ")}`);
      };
      expect(() =>
        releaseDeployTestHooks.providerOwnerGateAndFence(
          context(fake),
          commit,
        ),
      ).toThrow(scenario.error);
      expect(
        calls.some(
          ({ executable, args }) =>
            (executable === "git" &&
              (args[0] === "push" || args[0] === "update-ref")) ||
            (executable === "gh" &&
              ((args[0] === "workflow" && args[1] === "run") ||
                (args[0] === "api" &&
                  ["POST", "PATCH", "DELETE"].some((method) =>
                    args.includes(method),
                  )))),
        ),
      ).toBe(false);
    });
  }

  test("Form Package owner fence remains fixed to main", () => {
    const calls = [];
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      const version = toolOutput(executable, args);
      if (version !== null) return version;
      if (executable === "bun") return "";
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return "main\n";
        if (args[0] === "fetch") return "";
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === "rev-parse refs/remotes/origin/main"
        ) {
          return `${commit}\n`;
        }
        if (args[0] === "cat-file") return "";
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    expect(releaseDeployTestHooks.ownerGateAndFence(context(fake), commit)).toBe(
      commit,
    );
    const fetch = calls.find(
      ({ executable, args }) => executable === "git" && args[0] === "fetch",
    );
    expect(fetch.args).toContain("+refs/heads/main:refs/remotes/origin/main");
    expect(fetch.args.join(" ")).not.toContain(providerReleaseBranch);
  });

  test("blocks mutation when origin/main advances after the owner check", () => {
    const calls = [];
    const advanced = "89abcdef0123456789abcdef0123456789abcdef";
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      const version = toolOutput(executable, args);
      if (version !== null) return version;
      if (executable === "bun") return "";
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return "main\n";
        if (args[0] === "fetch") return "";
        if (args.join(" ") === "rev-parse HEAD") return `${commit}\n`;
        if (args.join(" ") === "rev-parse refs/remotes/origin/main") {
          return `${advanced}\n`;
        }
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.ownerGateAndFence(context(fake), commit),
    ).toThrow("is not fresh origin/main");
    expect(
      calls.some(
        (call) =>
          call.executable === "gh" &&
          ["api", "workflow", "release"].includes(call.args[0]),
      ),
    ).toBe(false);
    expect(
      calls.some(
        (call) =>
          call.executable === "git" && call.args[0] === "push",
      ),
    ).toBe(false);
  });

  test("rejects an unpinned verifier before the owner check", () => {
    let ranOwnerCheck = false;
    const fake = (executable, args) => {
      const version = toolOutput(executable, args, "v3.0.5");
      if (version !== null) return version;
      if (executable === "bun") ranOwnerCheck = true;
      return "";
    };
    expect(() =>
      releaseDeployTestHooks.ownerGateAndFence(context(fake), commit),
    ).toThrow("release toolchain drift");
    expect(ranOwnerCheck).toBe(false);
  });
});

describe("serial Form Package batch publication", () => {
  const tree = "a".repeat(40);

  function writeBatchInput(entries) {
    const directory = temporaryDirectory("form-publish-batch");
    const path = join(directory, "input.json");
    writeFileSync(
      path,
      `${JSON.stringify(recursivelySorted(entries))}\n`,
    );
    return path;
  }

  function batchExec(calls, timeline = []) {
    return (executable, args) => {
      calls.push({ executable, args: [...args] });
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (2026-07-02)\n";
      }
      if (executable === "cosign" && args[0] === "version") {
        return "GitVersion:    v3.0.6\n";
      }
      if (executable === "bun" && args.join(" ") === "run check:release-owner-gate") {
        return "";
      }
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return "main\n";
        if (args[0] === "fetch") {
          timeline.push("fresh-protected-main");
          return "";
        }
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === "rev-parse refs/remotes/origin/main"
        ) {
          return `${commit}\n`;
        }
        if (args.join(" ") === "rev-parse HEAD^{tree}") {
          return `${tree}\n`;
        }
        if (args[0] === "diff") {
          timeline.push("authority-path-fence");
          return "";
        }
        if (args[0] === "cat-file" || args[0] === "merge-base") {
          return "";
        }
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
  }

  test("two entries share exactly one owner check and retain serial mutation fences", () => {
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const selected = plan.releases.slice(0, 2);
    const input = writeBatchInput(
      selected.map((entry, index) => ({
        tag: entry.tag,
        expectedCommit: commit,
        runId: String(100 + index),
        runAttempt: "1",
      })),
    );
    const calls = [];
    const timeline = [];
    const execution = context(batchExec(calls, timeline));
    const result = releaseDeployTestHooks.formPublishBatch(
      execution,
      { input },
      plan,
      (activeContext, options) => {
        timeline.push(`start:${options.tag}`);
        const entry = selected.find(
          (candidate) => candidate.tag === options.tag,
        );
        for (const phase of ["tag", "release", "publish"]) {
          releaseDeployTestHooks.formPublicationMutationFence(
            activeContext,
            {
              expectedCommit: options["expected-commit"],
              toolingCommit: commit,
              entry,
              label: `${phase} test fence`,
            },
          );
          timeline.push(`mutation:${options.tag}:${phase}`);
        }
        timeline.push(`done:${options.tag}`);
        return {
          tag: options.tag,
          commit: options["expected-commit"],
          tagObject: "b".repeat(40),
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
          },
          releaseId: Number(options["run-id"]),
          releaseUrl: `https://example.invalid/${options["run-id"]}`,
          assetDigests: {},
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
        };
      },
    );

    expect(
      calls.filter(
        (call) =>
          call.executable === "bun" &&
          call.args.join(" ") === "run check:release-owner-gate",
      ),
    ).toHaveLength(1);
    expect(result.releases.map((release) => release.tag)).toEqual(
      selected.map((entry) => entry.tag),
    );
    for (const entry of selected) {
      const start = timeline.indexOf(`start:${entry.tag}`);
      const done = timeline.indexOf(`done:${entry.tag}`);
      expect(start).toBeGreaterThanOrEqual(0);
      expect(done).toBeGreaterThan(start);
      const segment = timeline.slice(start + 1, done);
      expect(segment).toEqual([
        "fresh-protected-main",
        "authority-path-fence",
        `mutation:${entry.tag}:tag`,
        "fresh-protected-main",
        "authority-path-fence",
        `mutation:${entry.tag}:release`,
        "fresh-protected-main",
        "authority-path-fence",
        `mutation:${entry.tag}:publish`,
      ]);
    }
    expect(timeline.indexOf(`done:${selected[0].tag}`)).toBeLessThan(
      timeline.indexOf(`start:${selected[1].tag}`),
    );
  });

  test("an invalid later candidate stops before its own mutation", () => {
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const selected = plan.releases.slice(0, 2);
    const input = writeBatchInput(
      selected.map((entry, index) => ({
        tag: entry.tag,
        expectedCommit: commit,
        runId: String(200 + index),
        runAttempt: "1",
      })),
    );
    const calls = [];
    const mutations = [];
    const execution = context(batchExec(calls));
    expect(() =>
      releaseDeployTestHooks.formPublishBatch(
        execution,
        { input },
        plan,
        (activeContext, options) => {
          if (options.tag === selected[1].tag) {
            throw new Error("reviewed candidate is invalid");
          }
          releaseDeployTestHooks.formPublicationMutationFence(
            activeContext,
            {
              expectedCommit: options["expected-commit"],
              toolingCommit: commit,
              entry: selected[0],
              label: "tag test fence",
            },
          );
          mutations.push(options.tag);
          return {
            tag: options.tag,
            commit: options["expected-commit"],
            tagObject: "b".repeat(40),
            candidateRun: {
              id: options["run-id"],
              attempt: options["run-attempt"],
            },
            releaseId: Number(options["run-id"]),
            releaseUrl: `https://example.invalid/${options["run-id"]}`,
            assetDigests: {},
            productionReadback: "EXACT_IMMUTABLE_RELEASE",
          };
        },
      ),
    ).toThrow("reviewed candidate is invalid");
    expect(mutations).toEqual([selected[0].tag]);
    expect(execution.io.errors).toContain(
      `"failedTag":"${selected[1].tag}"`,
    );
  });

  test("blocks dirty, HEAD, origin/main, and tree drift after the proof before a writer", () => {
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const entry = plan.releases[0];
    const advanced = "89abcdef0123456789abcdef0123456789abcdef";
    for (const drift of ["dirty", "head", "origin-main", "tree"]) {
      let proofEstablished = false;
      let writerCalled = false;
      const fake = (executable, args) => {
        if (executable === "gh" && args[0] === "--version") {
          return "gh version 2.96.0 (2026-07-02)\n";
        }
        if (executable === "cosign" && args[0] === "version") {
          return "GitVersion:    v3.0.6\n";
        }
        if (executable === "bun" && args.join(" ") === "run check:release-owner-gate") {
          return "";
        }
        if (executable === "git") {
          if (args[0] === "status") {
            return proofEstablished && drift === "dirty"
              ? " M scripts/release-deploy.mjs\n"
              : "";
          }
          if (args.join(" ") === "rev-parse --is-shallow-repository") {
            return "false\n";
          }
          if (args.join(" ") === "remote get-url origin") {
            return "https://github.com/tako0614/terraform-provider-takoform.git\n";
          }
          if (args[0] === "symbolic-ref") return "main\n";
          if (args[0] === "fetch") return "";
          if (args.join(" ") === "rev-parse HEAD") {
            return `${proofEstablished && drift === "head" ? advanced : commit}\n`;
          }
          if (args.join(" ") === "rev-parse refs/remotes/origin/main") {
            return `${proofEstablished && drift === "origin-main" ? advanced : commit}\n`;
          }
          if (args.join(" ") === "rev-parse HEAD^{tree}") {
            return `${proofEstablished && drift === "tree" ? advanced : tree}\n`;
          }
          if (
            args[0] === "cat-file" ||
            args[0] === "merge-base" ||
            args[0] === "diff"
          ) {
            return "";
          }
        }
        throw new Error(`unexpected ${executable} ${args.join(" ")}`);
      };
      const execution = context(fake);
      releaseDeployTestHooks.establishFormPublishBatchOwnerGateProof(
        execution,
      );
      proofEstablished = true;
      expect(() => {
        releaseDeployTestHooks.formPublicationMutationFence(execution, {
          expectedCommit: commit,
          toolingCommit: commit,
          entry,
          label: `${drift} test fence`,
        });
        writerCalled = true;
      }).toThrow();
      expect(writerCalled).toBe(false);
    }
  });

  test("clears the reusable proof after both success and failure", () => {
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const entry = plan.releases[0];
    const input = writeBatchInput([
      {
        tag: entry.tag,
        expectedCommit: commit,
        runId: "250",
        runAttempt: "1",
      },
    ]);
    for (const outcome of ["success", "failure"]) {
      const calls = [];
      const execution = context(batchExec(calls));
      const run = () =>
        releaseDeployTestHooks.formPublishBatch(
          execution,
          { input },
          plan,
          (_activeContext, options) => {
            if (outcome === "failure") {
              throw new Error("candidate failed");
            }
            return {
              tag: options.tag,
              commit: options["expected-commit"],
              tagObject: "b".repeat(40),
              candidateRun: {
                id: options["run-id"],
                attempt: options["run-attempt"],
              },
              releaseId: Number(options["run-id"]),
              releaseUrl: `https://example.invalid/${options["run-id"]}`,
              assetDigests: {},
              productionReadback: "EXACT_IMMUTABLE_RELEASE",
            };
          },
        );
      if (outcome === "failure") {
        expect(run).toThrow("candidate failed");
      } else {
        expect(run().status).toBe("VERIFIED");
      }
      releaseDeployTestHooks.ownerGateAndFence(execution, commit);
      expect(
        calls.filter(
          (call) =>
            call.executable === "bun" &&
            call.args.join(" ") === "run check:release-owner-gate",
        ),
      ).toHaveLength(2);
    }
  });

  test("opens without following links and inspects and reads the same file descriptor", () => {
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const entry = plan.releases[0];
    const raw = Buffer.from(
      `${JSON.stringify(
        recursivelySorted([
          {
            tag: entry.tag,
            expectedCommit: commit,
            runId: "275",
            runAttempt: "1",
          },
        ]),
      )}\n`,
    );
    const descriptor = 91;
    const events = [];
    const parsed = releaseDeployTestHooks.readFormPublishBatch(
      "/absolute/operator/input.json",
      plan,
      {
        open: (path, flags) => {
          events.push(["open", path, flags]);
          return descriptor;
        },
        fstat: (fd) => {
          events.push(["fstat", fd]);
          return { isFile: () => true, size: raw.length };
        },
        read: (fd, buffer, offset, length, position) => {
          events.push([
            "read",
            fd,
            buffer.length,
            offset,
            length,
            position,
          ]);
          if (offset === 0) {
            raw.copy(buffer, offset);
            return raw.length;
          }
          return 0;
        },
        close: (fd) => {
          events.push(["close", fd]);
        },
      },
    );
    expect(events).toEqual([
      [
        "open",
        "/absolute/operator/input.json",
        fsConstants.O_RDONLY |
          fsConstants.O_NOFOLLOW |
          fsConstants.O_NONBLOCK,
      ],
      ["fstat", descriptor],
      [
        "read",
        descriptor,
        1024 * 1024 + 1,
        0,
        1024 * 1024 + 1,
        null,
      ],
      [
        "read",
        descriptor,
        1024 * 1024 + 1,
        raw.length,
        1024 * 1024 + 1 - raw.length,
        null,
      ],
      ["close", descriptor],
    ]);
    expect(parsed).toEqual([
      {
        phase: "publish",
        tag: entry.tag,
        "expected-commit": commit,
        "run-id": "275",
        "run-attempt": "1",
      },
    ]);
  });

  test("rejects non-canonical, duplicate, symlink, oversized, and relative input", () => {
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const [entry, second] = plan.releases;
    const exact = {
      tag: entry.tag,
      expectedCommit: commit,
      runId: "300",
      runAttempt: "1",
    };
    const duplicateInput = writeBatchInput([exact, exact]);
    expect(() =>
      releaseDeployTestHooks.readFormPublishBatch(duplicateInput, plan),
    ).toThrow("duplicates tag");

    const duplicateRunInput = writeBatchInput([
      exact,
      { ...exact, tag: second.tag },
    ]);
    expect(() =>
      releaseDeployTestHooks.readFormPublishBatch(duplicateRunInput, plan),
    ).toThrow("duplicates workflow run/attempt");

    const nonCanonicalDirectory = temporaryDirectory(
      "form-publish-batch-noncanonical",
    );
    const nonCanonicalInput = join(nonCanonicalDirectory, "input.json");
    writeFileSync(nonCanonicalInput, JSON.stringify([exact], null, 2));
    expect(() =>
      releaseDeployTestHooks.readFormPublishBatch(nonCanonicalInput, plan),
    ).toThrow("canonical JSON");

    const symlinkDirectory = temporaryDirectory(
      "form-publish-batch-symlink",
    );
    const symlinkInput = join(symlinkDirectory, "input.json");
    symlinkSync(duplicateInput, symlinkInput);
    expect(() =>
      releaseDeployTestHooks.readFormPublishBatch(symlinkInput, plan),
    ).toThrow("without following symbolic links");

    const oversizedDirectory = temporaryDirectory(
      "form-publish-batch-oversized",
    );
    const oversizedInput = join(oversizedDirectory, "input.json");
    writeFileSync(oversizedInput, Buffer.alloc(1024 * 1024 + 1));
    expect(() =>
      releaseDeployTestHooks.readFormPublishBatch(oversizedInput, plan),
    ).toThrow("exceeds 1 MiB");

    expect(() =>
      parseReleaseSurfaceArgs("takoform-form-package-release", [
        "publish-batch",
        "--input",
        "relative.json",
      ]),
    ).toThrow("--input must be an absolute path");
  });
});

test("deep Form and revocation semantic verifier failures block publication", () => {
  const calls = [];
  const fake = (executable, args) => {
    calls.push({ executable, args: [...args] });
    return JSON.stringify({
      format: "unexpected",
      semanticStatus: "rejected",
      cryptographicStatus: "external-required",
    });
  };
  const execution = context(fake);
  expect(() =>
    releaseDeployTestHooks.verifyFormSemanticClosure(
      execution,
      "/tmp/untrusted-form-assets",
      {
        kind: "StaticSite",
        releaseId: "k-kn2gc5djmnjws5df",
        version: "1.0.0",
        tag: "forms/k-kn2gc5djmnjws5df/v1.0.0",
      },
      commit,
      commit,
      "/tmp/historical-trusted-root.json",
    ),
  ).toThrow("deep semantic report plan binding");
  expect(() =>
    releaseDeployTestHooks.verifyRevocationSemanticClosure(
      execution,
      "/tmp/untrusted-revocation-assets",
      "forms/revocations/v1.0.0",
      commit,
      commit,
      "/tmp/historical-trusted-root.json",
    ),
  ).toThrow("deep semantic report binding");
  expect(
    calls.some(
      (call) =>
        call.executable === "go" &&
        call.args.includes("verify-plan-directory"),
    ),
  ).toBe(true);
  expect(
    calls.some(
      (call) =>
        call.executable === "go" &&
        call.args.includes("verify-revocation-directory"),
    ),
  ).toBe(true);
  for (const call of calls) {
    expect(call.args[call.args.indexOf("--trusted-root") + 1]).toBe(
      "/tmp/historical-trusted-root.json",
    );
  }
});

test("top-level deep semantic rejection cannot push a tag, mutate a Release, or emit VERIFIED", () => {
  const cases = [
    {
      type: "form",
      phase: "publish",
      workflowName: "Prepare signed Form Package release candidate",
      deepCommand: "verify-plan-directory",
      expectedError: "Form Package deep semantic report identity mismatch",
    },
    {
      type: "revocation",
      phase: "publish-revocation",
      workflowName:
        "Prepare signed Form Package revocation checkpoint",
      deepCommand: "verify-revocation-directory",
      expectedError: "revocation deep semantic report identity mismatch",
    },
  ];
  for (const scenario of cases) {
    const repo = temporaryDirectory(`release-top-level-${scenario.type}`);
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const entry = plan.releases[0];
    entry.packageDigest = sha256(
      Buffer.from('{"fixture":"package-index"}\n'),
    );
    mkdirSync(join(repo, "forms"), { recursive: true });
    writeFileSync(
      join(repo, "forms", "release-plan.json"),
      JSON.stringify(plan),
    );
    const tag =
      scenario.type === "form"
        ? entry.tag
        : "forms/revocations/v1.0.0";
    if (scenario.type === "revocation") {
      mkdirSync(join(repo, "forms", "revocations", "checkpoints"), {
        recursive: true,
      });
      writeFileSync(
        join(repo, "forms", "revocations", "1.0.0.json"),
        "{}\n",
      );
      writeFileSync(
        join(repo, "forms", "revocations", "checkpoints", "1.0.0.json"),
        "{}\n",
      );
    }
    const calls = [];
    const io = memoryIO();
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return "main\n";
        if (args[0] === "fetch") return "";
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === "rev-parse refs/remotes/origin/main"
        ) {
          return `${commit}\n`;
        }
        if (
          args[0] === "show" &&
          args[1] === `${commit}:admission/v4/trust/trusted-root.json`
        ) {
          return "{}\n";
        }
        if (
          ["cat-file", "merge-base", "diff", "for-each-ref", "ls-remote"].includes(
            args[0],
          )
        ) {
          return "";
        }
      }
      if (executable === "gh" && isReleaseList(args)) return "[[]]";
      if (
        executable === "gh" &&
        args[0] === "run" &&
        args[1] === "view"
      ) {
        return JSON.stringify({
          databaseId: 123,
          attempt: 1,
          workflowName: scenario.workflowName,
          event: "workflow_dispatch",
          headBranch: "main",
          headSha: commit,
          status: "completed",
          conclusion: "success",
          displayTitle: requestId,
          url:
            "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/1",
        });
      }
      if (
        executable === "gh" &&
        args[0] === "run" &&
        args[1] === "download"
      ) {
        writeDeepFailureCandidate(args[args.indexOf("--dir") + 1], {
          type: scenario.type,
          entry,
          tag,
          runId: "123",
          runAttempt: "1",
          sourceCommit: commit,
          toolingCommit: commit,
        });
        return "";
      }
      if (
        executable === "go" &&
        args.includes(scenario.deepCommand)
      ) {
        return JSON.stringify(
          scenario.type === "form"
            ? {
                format:
                  "takoform.form-package-directory-verification@v1",
                semanticStatus: "rejected",
                cryptographicStatus: "external-required",
                kind: entry.kind,
                releaseId: entry.releaseId,
                version: entry.version,
                tag,
                sourceCommit: commit,
                toolingCommit: commit,
                trustedRoot: {},
                assets: [],
              }
            : {
                format:
                  "takoform.form-package-revocation-directory-verification@v1",
                semanticStatus: "rejected",
                cryptographicStatus: "external-required",
                version: "1.0.0",
                checkpointSequence: 1,
                checkpointDigest: `sha256:${"c".repeat(64)}`,
                packageDigest: `sha256:${"d".repeat(64)}`,
                formRef: {},
                tag,
                sourceCommit: commit,
                toolingCommit: commit,
                trustedRoot: {},
                assets: [],
              },
        );
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    expect(() =>
      runReleaseSurface({
        surface: "takoform-form-package-release",
        args: [
          scenario.phase,
          "--tag",
          tag,
          "--expected-commit",
          commit,
          "--run-id",
          "123",
          "--run-attempt",
          "1",
        ],
        repo,
        stdout: io.stdout,
        stderr: io.stderr,
        execFile: fake,
        uuidFactory: () => requestId,
        wait: () => {},
      }),
    ).toThrow(scenario.expectedError);
    expect(
      calls.some(
        (call) => call.executable === "go" && call.args.includes(scenario.deepCommand),
      ),
    ).toBe(true);
    expect(
      calls.some(
        (call) => call.executable === "git" && call.args[0] === "push",
      ),
    ).toBe(false);
    expect(
      calls.some(
        (call) =>
          call.executable === "gh" &&
          call.args[0] === "api" &&
          (call.args.includes("POST") || call.args.includes("PATCH")),
      ),
    ).toBe(false);
    expect(io.output).not.toContain("VERIFIED");
  }
});

test("tag-only recovery completes the exact candidate Release without any tag mutation", () => {
  const repo = temporaryDirectory("release-tag-only-top-level");
  const plan = JSON.parse(
    readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
  );
  const entry = plan.releases.find((candidate) => candidate.kind === "ModelEndpoint");
  entry.packageDigest = sha256(Buffer.from('{"fixture":"package-index"}\n'));
  mkdirSync(join(repo, "forms"), { recursive: true });
  writeFileSync(join(repo, "forms", "release-plan.json"), JSON.stringify(plan));
  const recoveryCommit = "89abcdef0123456789abcdef0123456789abcdef";
  const expectedTagObject = tagObjectFixture({
    tag: entry.tag,
    sourceCommit: commit,
    runId: "123",
    runAttempt: "1",
  }).oid;
  const calls = [];
  const io = memoryIO();
  let candidateRoot;
  let metadata;
  let draftCreated = false;
  let published = false;
  const releaseAssets = [];
  const releaseIdentity = () => ({
    id: 7,
    tag_name: entry.tag,
    draft: !published,
    prerelease: false,
    immutable: published,
    html_url: "https://github.com/example/recovered-release",
    assets: releaseAssets,
  });
  const fake = (executable, args, options) => {
    calls.push({ executable, args: [...args] });
    if (executable === "bun") return "";
    if (executable === "cosign") {
      if (args[0] === "version") return "GitVersion:    v3.0.6\n";
      if (args[0] === "verify-blob") return "";
    }
    if (executable === "git") {
      if (args[0] === "status") return "";
      if (args.join(" ") === "rev-parse --is-shallow-repository") {
        return "false\n";
      }
      if (args.join(" ") === "remote get-url origin") {
        return "https://github.com/tako0614/terraform-provider-takoform.git\n";
      }
      if (args[0] === "symbolic-ref") return "main\n";
      if (args[0] === "fetch") return "";
      if (
        args.join(" ") === "rev-parse HEAD" ||
        args.join(" ") === "rev-parse refs/remotes/origin/main"
      ) {
        return `${recoveryCommit}\n`;
      }
      if (args.join(" ") === "rev-parse --show-object-format") {
        return "sha1\n";
      }
      if (args.join(" ") === `rev-parse refs/tags/${entry.tag}^{commit}`) {
        return `${commit}\n`;
      }
      if (args[0] === "for-each-ref") {
        return metadata ? `${metadata.tagObjectOid}\n` : "";
      }
      if (
        args[0] === "cat-file" &&
        args[1] === "-t" &&
        args[2] === `refs/tags/${entry.tag}`
      ) {
        return "tag\n";
      }
      if (args[0] === "cat-file" || args[0] === "merge-base" || args[0] === "diff") {
        return "";
      }
      if (
        args[0] === "show" &&
        args[1] === `${commit}:admission/v4/trust/trusted-root.json`
      ) {
        return "{}\n";
      }
      if (args[0] === "show" && args.includes("--format=%ct")) return "1\n";
      if (args[0] === "mktag") return `${metadata.tagObjectOid}\n`;
      if (args[0] === "ls-remote") {
        const ref = `refs/tags/${entry.tag}`;
        return (
          `${metadata.tagObjectOid}\t${ref}\n` +
          `${commit}\t${ref}^{}\n`
        );
      }
    }
    if (executable === "go" && args.includes("verify-plan-directory")) {
      const assetRoot = args[args.indexOf("--asset-root") + 1];
      const trustedRoot = args[args.indexOf("--trusted-root") + 1];
      return JSON.stringify({
        format: "takoform.form-package-directory-verification@v1",
        semanticStatus: "verified",
        cryptographicStatus: "external-required",
        kind: entry.kind,
        releaseId: entry.releaseId,
        version: entry.version,
        tag: entry.tag,
        sourceCommit: commit,
        toolingCommit: commit,
        trustedRoot: {
          path: resolve(repo, "admission/v4/trust/trusted-root.json"),
          sha256: sha256(readFileSync(trustedRoot)),
        },
        assets: readdirSync(assetRoot)
          .sort()
          .map((name) => ({
            name,
            sha256: sha256(readFileSync(join(assetRoot, name))),
            size: readFileSync(join(assetRoot, name)).length,
          })),
      });
    }
    if (executable === "gh" && args[0] === "--version") {
      return "gh version 2.96.0 (2026-07-02)\n";
    }
    if (executable === "gh" && args[0] === "run" && args[1] === "view") {
      return JSON.stringify({
        databaseId: 123,
        attempt: 1,
        workflowName: "Prepare signed Form Package release candidate",
        event: "workflow_dispatch",
        headBranch: "main",
        headSha: commit,
        status: "completed",
        conclusion: "success",
        displayTitle: requestId,
        url:
          "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/1",
      });
    }
    if (executable === "gh" && args[0] === "run" && args[1] === "download") {
      candidateRoot = args[args.indexOf("--dir") + 1];
      writeDeepFailureCandidate(candidateRoot, {
        type: "form",
        entry,
        tag: entry.tag,
        runId: "123",
        runAttempt: "1",
        sourceCommit: commit,
        toolingCommit: commit,
      });
      metadata = JSON.parse(readFileSync(join(candidateRoot, "metadata.json"), "utf8"));
      return "";
    }
    if (executable === "gh" && isReleaseList(args)) {
      if (!draftCreated) return "[[]]";
      return JSON.stringify([
        [{ id: 7, tag_name: entry.tag, draft: !published }],
      ]);
    }
    if (
      executable === "gh" &&
      args[0] === "api" &&
      args.includes("POST") &&
      args.some((argument) => argument.includes("/assets?name="))
    ) {
      const path = args[args.indexOf("--input") + 1];
      const name = decodeURIComponent(
        args
          .find((argument) => argument.includes("/assets?name="))
          .split("/assets?name=")[1],
      );
      const asset = {
        id: releaseAssets.length + 100,
        name,
        state: "uploaded",
        digest: sha256(readFileSync(path)),
        size: readFileSync(path).length,
      };
      releaseAssets.push(asset);
      return JSON.stringify(asset);
    }
    if (executable === "gh" && args[0] === "api" && args.includes("POST")) {
      draftCreated = true;
      return JSON.stringify({
        id: 7,
        tag_name: entry.tag,
        draft: true,
        upload_url:
          "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
      });
    }
    if (executable === "gh" && args[0] === "api" && args.includes("PATCH")) {
      published = true;
      return JSON.stringify(releaseIdentity());
    }
    if (
      executable === "gh" &&
      args[0] === "api" &&
      (args[1] ===
        "repos/tako0614/terraform-provider-takoform/releases/7" ||
        args[1]?.includes("/releases/tags/"))
    ) {
      return JSON.stringify(releaseIdentity());
    }
    if (executable === "gh" && args[0] === "release" && args[1] === "download") {
      const destination = args[args.indexOf("--dir") + 1];
      for (const asset of releaseAssets) {
        copyFileSync(join(candidateRoot, "assets", asset.name), join(destination, asset.name));
      }
      return "";
    }
    throw new Error(`unexpected ${executable} ${args.join(" ")}`);
  };

  const result = runReleaseSurface({
    surface: "takoform-form-package-release",
    args: [
      "recover-tag-only",
      "--tag",
      entry.tag,
      "--expected-commit",
      commit,
      "--expected-tag-object",
      expectedTagObject,
      "--expected-recovery-commit",
      recoveryCommit,
      "--run-id",
      "123",
      "--run-attempt",
      "1",
    ],
    repo,
    stdout: io.stdout,
    stderr: io.stderr,
    execFile: fake,
    uuidFactory: () => requestId,
    wait: () => {},
  });
  expect(result.status).toBe("VERIFIED");
  expect(result.phase).toBe("recover-tag-only");
  expect(published).toBe(true);
  expect(releaseAssets).toHaveLength(7);
  expect(
    calls.some(
      (call) =>
        call.executable === "git" &&
        ["push", "update-ref"].includes(call.args[0]),
    ),
  ).toBe(false);
  expect(
    calls.some(
      (call) =>
        call.args.includes("DELETE") ||
        (call.executable === "git" && call.args.includes("-d")),
    ),
  ).toBe(false);
  expect(
    calls.filter(
      (call) => call.executable === "gh" && call.args.includes("POST"),
    ).length,
  ).toBe(8);
  expect(
    calls.filter(
      (call) => call.executable === "gh" && call.args.includes("PATCH"),
    ),
  ).toHaveLength(1);
  expect(
    calls.filter(
      (call) =>
        call.executable === "bun" &&
        call.args[0] === "run" &&
        call.args[1] === "check:release-owner-gate",
    ),
  ).toHaveLength(2);
  expect(
    calls.filter(
      (call) =>
        call.executable === "go" &&
        call.args.includes("verify-plan-directory"),
    ),
  ).toHaveLength(1);
});

test("top-level public verify cannot emit VERIFIED after deep semantic rejection", () => {
  for (const scenario of [
    {
      type: "form",
      phase: "verify",
      deepCommand: "verify-plan-directory",
      expectedError: "Form Package deep semantic report identity mismatch",
    },
    {
      type: "revocation",
      phase: "verify-revocation",
      deepCommand: "verify-revocation-directory",
      expectedError: "revocation deep semantic report identity mismatch",
    },
  ]) {
    const repo = temporaryDirectory(`release-public-verify-${scenario.type}`);
    const plan = JSON.parse(
      readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
    );
    const entry = plan.releases[0];
    entry.packageDigest = sha256(
      Buffer.from('{"fixture":"package-index"}\n'),
    );
    const tag =
      scenario.type === "form"
        ? entry.tag
        : "forms/revocations/v1.0.0";
    const fixture = join(repo, "fixture");
    mkdirSync(fixture);
    writeDeepFailureCandidate(fixture, {
      type: scenario.type,
      entry,
      tag,
      runId: "123",
      runAttempt: "1",
      sourceCommit: commit,
      toolingCommit: commit,
    });
    const publicRoot = join(fixture, "assets");
    const publicNames = readdirSync(publicRoot).sort();
    const remoteAssets = publicNames.map((name, index) => ({
      id: index + 100,
      name,
      state: "uploaded",
      digest: sha256(readFileSync(join(publicRoot, name))),
      size: readFileSync(join(publicRoot, name)).length,
    }));
    const calls = [];
    const io = memoryIO();
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      if (executable === "git") {
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return "main\n";
        if (args[0] === "fetch") return "";
        if (
          args.join(" ") === "rev-parse HEAD" ||
          args.join(" ") === "rev-parse refs/remotes/origin/main"
        ) {
          return `${commit}\n`;
        }
        if (args[0] === "cat-file" || args[0] === "merge-base") return "";
        if (args[0] === "ls-remote") {
          const ref = `refs/tags/${tag}`;
          return `${"a".repeat(40)}\t${ref}\n${commit}\t${ref}^{}\n`;
        }
        if (
          args[0] === "show" &&
          args[1] === `${commit}:forms/release-plan.json`
        ) {
          return JSON.stringify(plan);
        }
        if (
          args[0] === "show" &&
          args[1] === `${commit}:admission/v4/trust/trusted-root.json`
        ) {
          return "{}\n";
        }
      }
      if (
        executable === "gh" &&
        args[0] === "api" &&
        args[1]?.includes("/releases/tags/")
      ) {
        return JSON.stringify({
          id: 7,
          tag_name: tag,
          draft: false,
          prerelease: false,
          immutable: true,
          html_url: "https://github.com/example/release",
          assets: remoteAssets,
        });
      }
      if (
        executable === "gh" &&
        args[0] === "release" &&
        args[1] === "download"
      ) {
        const destination = args[args.indexOf("--dir") + 1];
        for (const name of publicNames) {
          copyFileSync(join(publicRoot, name), join(destination, name));
        }
        return "";
      }
      if (
        executable === "go" &&
        args.includes(scenario.deepCommand)
      ) {
        return JSON.stringify(
          scenario.type === "form"
            ? {
                format:
                  "takoform.form-package-directory-verification@v1",
                semanticStatus: "rejected",
                cryptographicStatus: "external-required",
                kind: entry.kind,
                releaseId: entry.releaseId,
                version: entry.version,
                tag,
                sourceCommit: commit,
                toolingCommit: commit,
                trustedRoot: {},
                assets: [],
              }
            : {
                format:
                  "takoform.form-package-revocation-directory-verification@v1",
                semanticStatus: "rejected",
                cryptographicStatus: "external-required",
                version: "1.0.0",
                checkpointSequence: 1,
                checkpointDigest: `sha256:${"c".repeat(64)}`,
                packageDigest: `sha256:${"d".repeat(64)}`,
                formRef: {},
                tag,
                sourceCommit: commit,
                toolingCommit: commit,
                trustedRoot: {},
                assets: [],
              },
        );
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    expect(() =>
      runReleaseSurface({
        surface: "takoform-form-package-release",
        args: [
          scenario.phase,
          "--tag",
          tag,
          "--expected-commit",
          commit,
        ],
        repo,
        stdout: io.stdout,
        stderr: io.stderr,
        execFile: fake,
        wait: () => {},
      }),
    ).toThrow(scenario.expectedError);
    expect(
      calls.some(
        (call) => call.executable === "go" && call.args.includes(scenario.deepCommand),
      ),
    ).toBe(true);
    expect(io.output).not.toContain("VERIFIED");
    expect(
      calls.some(
        (call) =>
          (call.executable === "git" && call.args[0] === "push") ||
          (call.executable === "gh" &&
            call.args[0] === "api" &&
            (call.args.includes("POST") || call.args.includes("PATCH"))),
      ),
    ).toBe(false);
  }
});

describe("deterministic Form tag objects", () => {
  const metadata = {
    tag: "forms/k-kn2gc5djmnjws5df/v1.0.0",
    sourceCommit: commit,
    requestId,
  };
  const fake = (executable, args) => {
    if (
      executable === "git" &&
      args.join(" ") === `show -s --format=%ct ${commit}`
    ) {
      return "1785283200\n";
    }
    throw new Error(`unexpected ${executable} ${args.join(" ")}`);
  };

  test("accepts only the exact workflow-generated byte sequence", () => {
    const root = temporaryDirectory("form-tag-object");
    const execution = context(fake);
    const exact = releaseDeployTestHooks.expectedFormTagObject(execution, {
      ...metadata,
      runId: "123",
      runAttempt: "2",
    });
    writeFileSync(join(root, "tag-object"), exact);
    expect(() =>
      releaseDeployTestHooks.verifyTagObjectWorkflowBinding(
        execution,
        root,
        metadata,
        "123",
        "2",
      ),
    ).not.toThrow();

    for (const changed of [
      exact.replace("type commit\n", "type commit\nobject deadbeef\n"),
      exact.replace("Takoform Form Package Release", "Wrong Tagger"),
      exact.replace(
        `Takoform Form Package ${metadata.tag}`,
        "Wrong title",
      ),
      exact.replace(`source-commit: ${commit}`, `source-commit: ${"f".repeat(40)}`),
      `${exact}extra-message: forbidden\n`,
    ]) {
      writeFileSync(join(root, "tag-object"), changed);
      expect(() =>
        releaseDeployTestHooks.verifyTagObjectWorkflowBinding(
          execution,
          root,
          metadata,
          "123",
          "2",
        ),
      ).toThrow("exact deterministic workflow object");
    }
  });

  test("uses the distinct exact revocation identity and title", () => {
    const root = temporaryDirectory("revocation-tag-object");
    const execution = context(fake);
    const revocation = {
      tag: "forms/revocations/v1.0.0",
      sourceCommit: commit,
      requestId,
    };
    const exact = releaseDeployTestHooks.expectedFormTagObject(execution, {
      ...revocation,
      runId: "456",
      runAttempt: "3",
      revocation: true,
    });
    expect(exact).toContain(
      "tagger Takoform Form Package Revocation <release@takoform.invalid>",
    );
    expect(exact).toContain(
      "Takoform Form Package revocation checkpoint forms/revocations/v1.0.0",
    );
    writeFileSync(join(root, "tag-object"), exact);
    expect(() =>
      releaseDeployTestHooks.verifyTagObjectWorkflowBinding(
        execution,
        root,
        revocation,
        "456",
        "3",
        { revocation: true },
      ),
    ).not.toThrow();
  });
});

describe("provider 15-asset provenance closure", () => {
  const descriptor = JSON.parse(
    readFileSync(join(repositoryRoot, "release/version.json"), "utf8"),
  );
  const names = releaseDeployTestHooks.providerAssetNames(descriptor);
  const tagObjectOid = "89abcdef0123456789abcdef0123456789abcdef";
  const tagObjectSha256 = `sha256:${"a".repeat(64)}`;

  function providerAssets(mutate = () => {}) {
    const root = temporaryDirectory("provider-provenance");
    for (const name of names.payload) {
      writeFileSync(join(root, name), `exact provider payload ${name}\n`);
    }
    const workflowRef =
      `tako0614/terraform-provider-takoform/.github/workflows/release.yml@refs/tags/${descriptor.tag}`;
    const statement = {
      _type: "https://in-toto.io/Statement/v1",
      subject: names.payload.map((name) => {
        const raw = readFileSync(join(root, name));
        return {
          name,
          digest: { sha256: sha256(raw).slice(7) },
          annotations: { size: raw.length },
        };
      }),
      predicateType: "https://slsa.dev/provenance/v1",
      predicate: {
        buildDefinition: {
          buildType:
            "https://takoform.com/buildtypes/provider-release/v1",
          externalParameters: {
            tag: descriptor.tag,
            requestId,
          },
          internalParameters: {
            canonicalization: "RFC8785",
            sourceCommit: commit,
            toolingCommit: commit,
            workflow: {
              path: ".github/workflows/release.yml",
              ref: workflowRef,
            },
            run: { id: "123", attempt: "2" },
            tagObject: {
              oid: tagObjectOid,
              sha256: tagObjectSha256,
            },
          },
          resolvedDependencies: [
            {
              name: "signed-provider-tag",
              uri:
                `git+https://${descriptor.sourceRepository}` +
                `@refs/tags/${descriptor.tag}`,
              digest: {
                gitCommit: commit,
                gitTagObject: tagObjectOid,
                sha256: tagObjectSha256.slice(7),
              },
            },
            {
              name: "signed-tag-release-tooling",
              uri:
                `git+https://${descriptor.sourceRepository}@${commit}`,
              digest: { gitCommit: commit },
            },
          ],
        },
        runDetails: {
          builder: { id: `https://github.com/${workflowRef}` },
          metadata: {
            invocationId:
              "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/2",
          },
        },
      },
    };
    mutate(statement, root);
    writeFileSync(
      join(root, names.provenance),
      JSON.stringify(recursivelySorted(statement)),
    );
    writeFileSync(join(root, names.provenanceSignature), "detached gpg signature\n");
    return { root, statement };
  }

  function runVerifier(root, overrides = {}) {
    return execFileSync(
      "go",
      [
        "-C",
        "cmd/provider-release",
        "run",
        ".",
        "verify-release-provenance",
        "--assets",
        root,
        "--expected-tag",
        descriptor.tag,
        "--expected-source-commit",
        commit,
        "--expected-tooling-commit",
        commit,
        "--expected-request-id",
        requestId,
        "--expected-run-id",
        "123",
        "--expected-run-attempt",
        "2",
        "--expected-tag-object-oid",
        tagObjectOid,
        "--expected-tag-object-sha256",
        tagObjectSha256,
        ...(overrides.extraArgs ?? []),
      ],
      {
        cwd: repositoryRoot,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
  }

  test("accepts one exact 13-subject statement plus detached signature", () => {
    expect(names.payload).toHaveLength(13);
    expect(names.all).toHaveLength(15);
    const { root } = providerAssets();
    const calls = [];
    const execution = context((executable, args) => {
      calls.push({ executable, args: [...args] });
      return JSON.stringify({
        kind: "takoform.provider-release-provenance-verification@v1",
        tag: descriptor.tag,
        provenance: names.provenance,
        subjectCount: 13,
        signerFingerprint:
          "3510E75E05BBCC303B92D77934FC18AC897FB709",
        verified: true,
      });
    });
    const report = releaseDeployTestHooks.verifyProviderReleaseProvenance(
      execution,
      root,
      descriptor,
      {
        sourceCommit: commit,
        toolingCommit: commit,
        requestId,
        runId: "123",
        runAttempt: "2",
        tagObjectOid,
        tagObjectSha256,
      },
    );
    expect(report).toEqual({
      kind: "takoform.provider-release-provenance-verification@v1",
      tag: descriptor.tag,
      provenance: names.provenance,
      subjectCount: 13,
      signerFingerprint:
        "3510E75E05BBCC303B92D77934FC18AC897FB709",
      verified: true,
    });
    expect(calls[0].args).toContain("verify-release-provenance");
    expect(calls[0].args).toContain("--expected-request-id");
    expect(calls[0].args).toContain(requestId);
  });

  test.each([
    [
      "tag",
      (statement) =>
        (statement.predicate.buildDefinition.externalParameters.tag =
          "v1.0.1"),
    ],
    [
      "source",
      (statement) =>
        (statement.predicate.buildDefinition.internalParameters.sourceCommit =
          "f".repeat(40)),
    ],
    [
      "tooling",
      (statement) =>
        (statement.predicate.buildDefinition.internalParameters.toolingCommit =
          "f".repeat(40)),
    ],
    [
      "request",
      (statement) =>
        (statement.predicate.buildDefinition.externalParameters.requestId =
          "11234567-89ab-4cde-8fab-0123456789ab"),
    ],
    [
      "run id",
      (statement) =>
        (statement.predicate.buildDefinition.internalParameters.run.id =
          "124"),
    ],
    [
      "run attempt",
      (statement) =>
        (statement.predicate.buildDefinition.internalParameters.run.attempt =
          "3"),
    ],
    [
      "subject digest",
      (statement) => (statement.subject[0].digest.sha256 = "f".repeat(64)),
    ],
  ])("rejects substituted %s binding", (_label, mutate) => {
    const { root } = providerAssets(mutate);
    expect(() => runVerifier(root)).toThrow();
  });

  test("rejects omitted and extra release assets", () => {
    {
      const { root } = providerAssets();
      rmSync(join(root, names.payload[0]));
      expect(() => runVerifier(root)).toThrow();
    }
    {
      const { root } = providerAssets();
      mkdirSync(join(root, "unexpected-empty-directory"));
      expect(() => runVerifier(root)).toThrow();
    }
  });

  test("module rejects a provenance signature without the pinned VALIDSIG", () => {
    const fake = (_executable, args) => {
      if (args.includes("show-only")) {
        return "fpr:::::::::3510E75E05BBCC303B92D77934FC18AC897FB709:\n";
      }
      if (args.includes("--import")) return "";
      if (
        args.some((argument) =>
          argument.endsWith(`/${names.provenanceSignature}`),
        )
      ) {
        throw commandFailure("BAD signature");
      }
      if (args.includes("--verify")) {
        return "[GNUPG:] VALIDSIG 3510E75E05BBCC303B92D77934FC18AC897FB709 2026-01-01 0 4 0 1 10 00 3510E75E05BBCC303B92D77934FC18AC897FB709\n";
      }
      throw new Error(`unexpected gpg ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.verifyProviderSignature(
        context(fake),
        "/tmp/provider-assets",
        names,
      ),
    ).toThrow("verify provider release provenance signature");
  });
});

describe("provider Registry readback maintenance identity", () => {
  function verifier(root, readback, providerCommit) {
    const fake = (executable, args) => {
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (2026-07-02)\n";
      }
      if (executable === "cosign" && args[0] === "version") {
        return "GitVersion:    v3.0.6\n";
      }
      if (executable === "git") {
        if (args[0] === "rev-list") return `${providerCommit}\n`;
        if (args[0] === "cat-file" || args[0] === "merge-base") return "";
      }
      if (executable === "go") return readback;
      if (executable === "cosign" && args[0] === "verify-blob") return "";
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    const descriptor = JSON.parse(
      readFileSync(join(repositoryRoot, "release", "version.json"), "utf8"),
    );
    return releaseDeployTestHooks.verifyRegistryCandidate(
      context(fake),
      root,
      {
        descriptor,
        expectedCommit: commit,
        runId: "123",
        runAttempt: "1",
        requestId,
      },
    );
  }

  test("accepts only the maintenance workflow identity", () => {
    const root = temporaryDirectory("registry-maintenance-identity");
    const fixture = writeRegistryReadbackCandidate(root);
    expect(
      verifier(root, fixture.readback, fixture.providerCommit)
        .installedProviderDigest,
    ).toBe("sha256:" + "c".repeat(64));
  });

  test("rejects main identity substitution and source drift", () => {
    {
      const root = temporaryDirectory("registry-main-substitution");
      const fixture = writeRegistryReadbackCandidate(root, {
        certificateIdentity:
          "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/provider-registry-readback.yml@refs/heads/main",
      });
      expect(() =>
        verifier(root, fixture.readback, fixture.providerCommit),
      ).toThrow("metadata binding mismatch");
    }
    {
      const root = temporaryDirectory("registry-source-drift");
      const fixture = writeRegistryReadbackCandidate(root, {
        sourceCommit: "d".repeat(40),
      });
      expect(() =>
        verifier(root, fixture.readback, fixture.providerCommit),
      ).toThrow("metadata binding mismatch");
    }
  });

  test("rejects Terraform and OpenTofu binary digest mismatch", () => {
    const root = temporaryDirectory("registry-binary-digest-drift");
    const fixture = writeRegistryReadbackCandidate(root, {
      installedDigests: [
        "sha256:" + "c".repeat(64),
        "sha256:" + "d".repeat(64),
      ],
    });
    expect(() => verifier(root, fixture.readback, fixture.providerCommit)).toThrow(
      "one provider binary digest",
    );
  });
});

describe("local immutable GitHub Release publication", () => {
  function assetFixture() {
    const root = temporaryDirectory("release-publish");
    const source = join(root, "candidate");
    mkdirSync(source);
    const path = join(source, "asset.txt");
    writeFileSync(path, "candidate bytes\n");
    const digest = sha256(readFileSync(path));
    return {
      root,
      path,
      digest,
      assets: new Map([
        ["asset.txt", { name: "asset.txt", path, sha256: digest }],
      ]),
    };
  }

  function retainedDraftFixture() {
    const fixture = assetFixture();
    const secondPath = join(fixture.root, "candidate", "second.txt");
    writeFileSync(secondPath, "second candidate bytes\n");
    fixture.assets.set("second.txt", {
      name: "second.txt",
      path: secondPath,
      sha256: sha256(readFileSync(secondPath)),
    });
    return fixture;
  }

  test("preflight rejects an existing draft and duplicate exact-tag identities", () => {
    for (const releases of [
      [
        {
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
        },
      ],
      [
        {
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
        },
        {
          id: 8,
          tag_name: "v1.0.0",
          draft: false,
        },
      ],
    ]) {
      let mutations = 0;
      const fake = (_executable, args) => {
        if (isReleaseList(args)) return JSON.stringify([releases]);
        mutations += 1;
        throw new Error("mutation must not run");
      };
      expect(() =>
        releaseDeployTestHooks.assertReleaseAbsent(
          context(fake),
          "v1.0.0",
        ),
      ).toThrow("already exist");
      expect(mutations).toBe(0);
    }
  });

  test("strict publication rejects drifted or nonempty authoritative drafts before asset upload", () => {
    const fixture = assetFixture();
    const tag = "v1.0.1";
    const body = "exact provider release";
    const remoteAsset = {
      id: 9,
      name: "asset.txt",
      state: "uploaded",
      digest: fixture.digest,
      size: readFileSync(fixture.path).length,
    };
    const exactDraft = () => ({
      id: 7,
      tag_name: tag,
      target_commitish: "main",
      name: tag,
      body,
      draft: true,
      prerelease: false,
      immutable: false,
      published_at: null,
      assets_url:
        "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
      upload_url:
        "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
      assets: [],
    });
    for (const mutate of [
      (draft) => (draft.target_commitish = commit),
      (draft) => (draft.name = "drifted name"),
      (draft) => (draft.body = "drifted body"),
      (draft) => draft.assets.push(remoteAsset),
    ]) {
      const draft = exactDraft();
      mutate(draft);
      let listCalls = 0;
      const calls = [];
      const fake = (_executable, args) => {
        calls.push([...args]);
        if (isReleaseList(args)) {
          listCalls += 1;
          return listCalls === 1
            ? "[[]]"
            : JSON.stringify([
                [{ id: 7, tag_name: tag, draft: true }],
              ]);
        }
        if (
          args.includes("POST") &&
          !args.some((argument) => argument.includes("/assets?name="))
        ) {
          return JSON.stringify({
            id: 7,
            tag_name: tag,
            draft: true,
            upload_url:
              "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
          });
        }
        if (
          args[0] === "api" &&
          args[1] ===
            "repos/tako0614/terraform-provider-takoform/releases/7"
        ) {
          return JSON.stringify(draft);
        }
        throw new Error(`unexpected gh ${args.join(" ")}`);
      };
      expect(() =>
        releaseDeployTestHooks.publishReleaseLocally(context(fake), {
          tag,
          assets: fixture.assets,
          body,
          temporaryRoot: fixture.root,
          strictIdentity: true,
        }),
      ).toThrow();
      expect(
        calls.some(
          (args) =>
            args.includes("POST") &&
            args.some((argument) => argument.includes("/assets?name=")),
        ),
      ).toBe(false);
      expect(calls.some((args) => args.includes("PATCH"))).toBe(false);
      expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
    }
  });

  test("strict publication sends the selected full exact PATCH identity and halts on a drifted response", () => {
    const fixture = assetFixture();
    const tag = "v1.0.1";
    const body = "exact provider release";
    const calls = [];
    let listCalls = 0;
    let remoteAssets = [];
    const exactDraft = () => ({
      id: 7,
      tag_name: tag,
      target_commitish: providerReleaseBranch,
      name: tag,
      body,
      draft: true,
      prerelease: false,
      immutable: false,
      published_at: null,
      assets_url:
        "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
      upload_url:
        "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
      assets: remoteAssets,
    });
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([
              [{ id: 7, tag_name: tag, draft: true }],
            ]);
      }
      if (
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name="))
      ) {
        remoteAssets = [
          {
            id: 9,
            name: "asset.txt",
            state: "uploaded",
            digest: fixture.digest,
            size: readFileSync(fixture.path).length,
          },
        ];
        return JSON.stringify(remoteAssets[0]);
      }
      if (args.includes("POST")) {
        return JSON.stringify({
          id: 7,
          tag_name: tag,
          draft: true,
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify(exactDraft());
      }
      if (args.includes("PATCH")) {
        return JSON.stringify({
          ...exactDraft(),
          body: "raced body drift",
          draft: false,
        });
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(context(fake), {
        tag,
        assets: fixture.assets,
        body,
        temporaryRoot: fixture.root,
        strictIdentity: true,
        targetCommitish: providerReleaseBranch,
      }),
    ).toThrow("PATCH response differs");
    const patch = calls.find((args) => args.includes("PATCH"));
    expect(patch).toContain(`tag_name=${tag}`);
    expect(patch).toContain(`target_commitish=${providerReleaseBranch}`);
    expect(patch).toContain(`name=${tag}`);
    expect(patch).toContain(`body=${body}`);
    expect(patch).toContain("prerelease=false");
    expect(patch).toContain("draft=false");
    expect(patch).toContain("make_latest=false");
    expect(
      calls.some(
        (args) =>
          args[0] === "api" && args[1]?.includes("/releases/tags/"),
      ),
    ).toBe(false);
    expect(calls.some((args) => args[0] === "release")).toBe(false);
  });

  test("lost POST response finds and retains the exact visible draft", () => {
    const fixture = assetFixture();
    let listCalls = 0;
    const calls = [];
    const execution = context((_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([
              [
                {
                  id: 7,
                  tag_name: "v1.0.0",
                  draft: true,
                },
              ],
            ]);
      }
      if (args.includes("POST")) {
        throw commandFailure("connection lost after draft POST");
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    });
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(execution, {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("gh api");
    expect(listCalls).toBe(2);
    expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
    expect(execution.io.errors).toContain("MATCHING_DRAFT_RETAINED");
    expect(execution.io.errors).toContain('"observedReleaseIDs":[7]');
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(execution, {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("already exist");
    expect(
      calls.filter((args) => args.includes("POST")).length,
    ).toBe(1);
  });

  test("retained draft recovery resumes after a lost upload response and uploads only missing assets", () => {
    const fixture = retainedDraftFixture();
    const tag = "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0";
    const body = "exact retained draft body";
    const calls = [];
    const remoteAssets = [];
    let loseFirstUploadResponse = true;
    let published = false;
    let fenceCalls = 0;
    const draft = () => ({
      id: 7,
      tag_name: tag,
      target_commitish: "main",
      name: tag,
      body,
      draft: true,
      prerelease: false,
      immutable: false,
      published_at: null,
      assets_url:
        "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
      upload_url:
        "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
      assets: remoteAssets,
    });
    const publicRelease = () => ({
      id: 7,
      tag_name: tag,
      target_commitish: "main",
      name: tag,
      body,
      draft: false,
      prerelease: false,
      immutable: true,
      html_url: "https://github.com/example/recovered-release",
      assets_url:
        "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
      upload_url:
        "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
      assets: remoteAssets,
    });
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        return JSON.stringify([
          [{ id: 7, tag_name: tag, draft: !published }],
        ]);
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify(draft());
      }
      if (
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name="))
      ) {
        const path = args[args.indexOf("--input") + 1];
        const name = decodeURIComponent(
          args
            .find((argument) => argument.includes("/assets?name="))
            .split("/assets?name=")[1],
        );
        const asset = {
          id: remoteAssets.length + 100,
          name,
          state: "uploaded",
          digest: sha256(readFileSync(path)),
          size: readFileSync(path).length,
        };
        remoteAssets.push(asset);
        if (loseFirstUploadResponse) {
          loseFirstUploadResponse = false;
          throw commandFailure("connection lost after asset upload");
        }
        return JSON.stringify(asset);
      }
      if (args.includes("PATCH")) {
        published = true;
        return JSON.stringify(publicRelease());
      }
      if (args[0] === "api" && args[1]?.includes("/releases/tags/")) {
        return JSON.stringify(publicRelease());
      }
      if (args[0] === "release" && args[1] === "download") {
        const output = args[args.indexOf("--dir") + 1];
        for (const asset of fixture.assets.values()) {
          copyFileSync(asset.path, join(output, asset.name));
        }
        return "";
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    const execution = context(fake);
    expect(() =>
      releaseDeployTestHooks.resumeDraftReleaseLocally(execution, {
        releaseId: 7,
        tag,
        assets: fixture.assets,
        body,
        temporaryRoot: fixture.root,
      }),
    ).toThrow("gh api");
    expect(remoteAssets).toHaveLength(1);
    expect(execution.io.errors).toContain("MATCHING_DRAFT_RETAINED");

    const release = releaseDeployTestHooks.resumeDraftReleaseLocally(
      execution,
      {
        releaseId: 7,
        tag,
        assets: fixture.assets,
        body,
        temporaryRoot: fixture.root,
        prePublishFence: () => {
          fenceCalls += 1;
        },
      },
    );
    expect(release.id).toBe(7);
    expect(published).toBe(true);
    expect(remoteAssets).toHaveLength(2);
    expect(fenceCalls).toBe(1);
    expect(
      calls.filter(
        (args) =>
          args.includes("POST") &&
          args.some((argument) => argument.includes("/assets?name=")),
      ),
    ).toHaveLength(2);
    for (const upload of calls.filter(
      (args) =>
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name=")),
    )) {
      expect(upload).not.toContain("--hostname");
      expect(
        upload.some((argument) =>
          argument.startsWith(
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets?name=",
          ),
        ),
      ).toBe(true);
    }
    expect(
      calls.some(
        (args) =>
          args.includes("POST") &&
          !args.some((argument) => argument.includes("/assets?name=")),
      ),
    ).toBe(false);
    expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
    const patch = calls.find((args) => args.includes("PATCH"));
    expect(patch).toContain(`tag_name=${tag}`);
    expect(patch).toContain("target_commitish=main");
    expect(patch).toContain(`name=${tag}`);
    expect(patch).toContain(`body=${body}`);
    expect(patch).toContain("draft=false");
    expect(patch).toContain("prerelease=false");
  });

  test("retained draft recovery rejects asset drift and a competing identity before PATCH", () => {
    const fixture = retainedDraftFixture();
    const tag = "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0";
    const body = "exact retained draft body";
    const first = [...fixture.assets.values()][0];
    for (const remoteAssets of [
      [
        {
          id: 100,
          name: first.name,
          state: "uploaded",
          digest: `sha256:${"f".repeat(64)}`,
          size: readFileSync(first.path).length,
        },
      ],
      [
        {
          id: 100,
          name: "unexpected.txt",
          state: "uploaded",
          digest: first.sha256,
          size: readFileSync(first.path).length,
        },
      ],
    ]) {
      const calls = [];
      const fake = (_executable, args) => {
        calls.push([...args]);
        if (isReleaseList(args)) {
          return JSON.stringify([
            [{ id: 7, tag_name: tag, draft: true }],
          ]);
        }
        if (
          args[0] === "api" &&
          args[1] ===
            "repos/tako0614/terraform-provider-takoform/releases/7"
        ) {
          return JSON.stringify({
            id: 7,
            tag_name: tag,
            target_commitish: "main",
            name: tag,
            body,
            draft: true,
            prerelease: false,
            immutable: false,
            published_at: null,
            assets_url:
              "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
            upload_url:
              "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
            assets: remoteAssets,
          });
        }
        throw new Error(`unexpected gh ${args.join(" ")}`);
      };
      expect(() =>
        releaseDeployTestHooks.resumeDraftReleaseLocally(context(fake), {
          releaseId: 7,
          tag,
          assets: fixture.assets,
          body,
          temporaryRoot: fixture.root,
        }),
      ).toThrow("unknown, duplicate, or drifted asset");
      expect(calls.some((args) => args.includes("PATCH"))).toBe(false);
      expect(calls.some((args) => args.includes("POST"))).toBe(false);
    }

    const calls = [];
    let competing = false;
    const remoteAssets = [...fixture.assets.values()].map((asset, index) => ({
      id: index + 100,
      name: asset.name,
      state: "uploaded",
      digest: asset.sha256,
      size: readFileSync(asset.path).length,
    }));
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        return JSON.stringify([
          competing
            ? [
                { id: 7, tag_name: tag, draft: true },
                { id: 8, tag_name: tag, draft: true },
              ]
            : [{ id: 7, tag_name: tag, draft: true }],
        ]);
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify({
          id: 7,
          tag_name: tag,
          target_commitish: "main",
          name: tag,
          body,
          draft: true,
          prerelease: false,
          immutable: false,
          published_at: null,
          assets_url:
            "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
          assets: remoteAssets,
        });
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.resumeDraftReleaseLocally(context(fake), {
        releaseId: 7,
        tag,
        assets: fixture.assets,
        body,
        temporaryRoot: fixture.root,
        prePublishFence: () => {
          competing = true;
        },
      }),
    ).toThrow("expected only release 7:draft");
    expect(calls.some((args) => args.includes("PATCH"))).toBe(false);
    expect(calls.some((args) => args.includes("POST"))).toBe(false);
  });

  test("publishes exact bytes without moving the floating Latest identity", () => {
    const fixture = assetFixture();
    const calls = [];
    let published = false;
    let listCalls = 0;
    const remoteAsset = {
      id: 9,
      name: "asset.txt",
      state: "uploaded",
      digest: fixture.digest,
      size: readFileSync(fixture.path).length,
    };
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([
              [
                {
                  id: 7,
                  tag_name: "v1.0.0",
                  draft: !published,
                },
              ],
            ]);
      }
      if (
        args[0] === "api" &&
        args[1]?.startsWith("repos/") &&
        args[1]?.includes("/releases/tags/")
      ) {
        if (!published) throw commandFailure("HTTP 404: Not Found");
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: false,
          prerelease: false,
          immutable: true,
          html_url: "https://github.com/example/release",
          assets: [remoteAsset],
        });
      }
      if (
        args.includes("POST") &&
        args.some((argument) =>
          argument.startsWith(
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets?name=",
          ),
        )
      ) {
        return JSON.stringify(remoteAsset);
      }
      if (args.includes("POST")) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          prerelease: false,
          assets: [remoteAsset],
        });
      }
      if (args.includes("PATCH")) {
        published = true;
        return "{}";
      }
      if (args[0] === "release" && args[1] === "download") {
        const output = args[args.indexOf("--dir") + 1];
        copyFileSync(fixture.path, join(output, "asset.txt"));
        return "";
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    const release = releaseDeployTestHooks.publishReleaseLocally(
      context(fake),
      {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      },
    );
    expect(release.id).toBe(7);
    for (const mutation of calls.filter(
      (args) =>
        (args.includes("POST") &&
          !args.some((argument) => argument.includes("/assets?name="))) ||
        args.includes("PATCH"),
    )) {
      expect(mutation).toContain("make_latest=false");
    }
    const uploads = calls.filter((args) =>
      args.some((argument) =>
        argument.startsWith(
          "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets?name=",
        ),
      ),
    );
    expect(uploads).toHaveLength(1);
    expect(uploads[0]).not.toContain("--hostname");
    expect(uploads[0]).toContain(fixture.path);
    expect(
      calls.some(
        (args) => args[0] === "release" && args[1] === "upload",
      ),
    ).toBe(false);
  });

  test("competing exact-tag draft during upload blocks PATCH and is retained", () => {
    const fixture = assetFixture();
    const calls = [];
    let listCalls = 0;
    const remoteAsset = {
      id: 9,
      name: "asset.txt",
      state: "uploaded",
      digest: fixture.digest,
      size: readFileSync(fixture.path).length,
    };
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([
              [
                { id: 7, tag_name: "v1.0.0", draft: true },
                { id: 8, tag_name: "v1.0.0", draft: true },
              ],
            ]);
      }
      if (
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name="))
      ) {
        return JSON.stringify(remoteAsset);
      }
      if (args.includes("POST")) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          prerelease: false,
          assets: [remoteAsset],
        });
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    const execution = context(fake);
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(execution, {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("expected only release 7:draft");
    expect(calls.some((args) => args.includes("PATCH"))).toBe(false);
    expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
    expect(execution.io.errors).toContain("REMOTE_STATE_AMBIGUOUS");
    expect(execution.io.errors).toContain('"observedReleaseIDs":[7,8]');
  });

  test("never deletes a release observed as public during failure cleanup", () => {
    const fixture = assetFixture();
    const calls = [];
    let releaseReads = 0;
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) return "[[]]";
      if (
        args[0] === "api" &&
        args[1]?.includes("/releases/tags/")
      ) {
        throw commandFailure("HTTP 404: Not Found");
      }
      if (
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name="))
      ) {
        return JSON.stringify({
          id: 9,
          name: "asset.txt",
          state: "uploaded",
          digest: fixture.digest,
          size: readFileSync(fixture.path).length,
        });
      }
      if (args.includes("POST")) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        releaseReads += 1;
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: releaseReads === 1,
          prerelease: false,
          assets:
            releaseReads === 1
              ? [
                  {
                    id: 9,
                    name: "asset.txt",
                    state: "uploaded",
                    digest: `sha256:${"f".repeat(64)}`,
                    size: readFileSync(fixture.path).length,
                  },
                ]
              : [],
        });
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(context(fake), {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("draft API asset identity");
    expect(
      calls.some((args) => args.includes("DELETE")),
    ).toBe(false);
  });

  test("retains the exact POST-returned release still reread as a draft", () => {
    const fixture = assetFixture();
    const calls = [];
    let releaseReads = 0;
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) return "[[]]";
      if (args[0] === "api" && args[1]?.includes("/releases/tags/")) {
        throw commandFailure("HTTP 404: Not Found");
      }
      if (
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name="))
      ) {
        return JSON.stringify({
          id: 9,
          name: "asset.txt",
          state: "uploaded",
          digest: fixture.digest,
          size: readFileSync(fixture.path).length,
        });
      }
      if (args.includes("POST")) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          prerelease: false,
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        releaseReads += 1;
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          prerelease: false,
          assets: [
            {
              id: 9,
              name: "asset.txt",
              state: "uploaded",
              digest: `sha256:${"f".repeat(64)}`,
              size: readFileSync(fixture.path).length,
            },
          ],
        });
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(context(fake), {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("draft API asset identity");
    expect(releaseReads).toBe(2);
    expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
  });

  test("rejects an upload endpoint not bound to the created release id", () => {
    const fixture = assetFixture();
    const calls = [];
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) return "[[]]";
      if (args[0] === "api" && args[1]?.includes("/releases/tags/")) {
        throw commandFailure("HTTP 404: Not Found");
      }
      if (args.includes("POST")) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: true,
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/8/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify({
          id: 7,
          tag_name: "v1.0.0",
          draft: false,
        });
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(context(fake), {
        tag: "v1.0.0",
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("draft creation returned an unexpected identity");
    expect(
      calls.some((args) =>
        args.some((argument) => argument.includes("/assets?name=")),
      ),
    ).toBe(false);
  });

  test("remote tag race fails the create-only lease push", () => {
    let push;
    let pushEnvironment;
    const fake = (_executable, args, options) => {
      if (args[0] === "push") {
        push = [...args];
        pushEnvironment = { ...options.env };
        throw commandFailure("remote ref appeared during push");
      }
      throw new Error(`unexpected git ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.pushExactTag(
        context(fake),
        "v1.0.0",
        commit,
        "89abcdef0123456789abcdef0123456789abcdef",
      ),
    ).toThrow("git push");
    expect(push).toContain("--force-with-lease=refs/tags/v1.0.0:");
    expect(push).toContain("--no-verify");
    expect(push).toContain(
      "https://github.com/tako0614/terraform-provider-takoform.git",
    );
    expect(push.join(" ")).not.toContain("operator-only-test-token");
    expect(pushEnvironment.GH_TOKEN).toBeUndefined();
    expect(pushEnvironment.GITHUB_TOKEN).toBeUndefined();
    expect(pushEnvironment.GIT_TERMINAL_PROMPT).toBe("0");
    expect(pushEnvironment.GIT_CONFIG_VALUE_0).toBe("");
    expect(pushEnvironment.GIT_CONFIG_VALUE_2).toBe("");
    expect(pushEnvironment.GIT_CONFIG_VALUE_3).toStartWith(
      "AUTHORIZATION: basic ",
    );
    expect(pushEnvironment.GIT_CONFIG_VALUE_3).not.toContain(
      "operator-only-test-token",
    );
    expect(
      Buffer.from(
        pushEnvironment.GIT_CONFIG_VALUE_3.split(" ").at(-1),
        "base64",
      ).toString("utf8"),
    ).toBe("x-access-token:operator-only-test-token");
    expect(pushEnvironment.GIT_CONFIG_KEY_4).toBe("core.hooksPath");
    expect(pushEnvironment.GIT_CONFIG_VALUE_4).toBe("/dev/null");
  });

  test("tag-only recovery accepts only the exact local/remote annotated object and absent Release", () => {
    const tag = "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0";
    const object = "a".repeat(40);
    const ref = `refs/tags/${tag}`;
    const exactRemote = `${object}\t${ref}\n${commit}\t${ref}^{}\n`;
    const execute = ({
      local = object,
      localType = "tag",
      localCommit = commit,
      remote = exactRemote,
      releases = "[[]]",
    } = {}) => {
      const calls = [];
      const fake = (_executable, args) => {
        calls.push([...args]);
        if (args[0] === "for-each-ref") {
          if (local instanceof Error) throw local;
          return `${local}\n`;
        }
        if (args[0] === "cat-file" && args[1] === "-t") {
          if (localType instanceof Error) throw localType;
          return `${localType}\n`;
        }
        if (args[0] === "rev-parse") {
          if (localCommit instanceof Error) throw localCommit;
          return `${localCommit}\n`;
        }
        if (args[0] === "ls-remote") {
          if (remote instanceof Error) throw remote;
          return remote;
        }
        if (isReleaseList(args)) return releases;
        throw new Error(`unexpected command ${args.join(" ")}`);
      };
      return {
        calls,
        run: () =>
          releaseDeployTestHooks.assertTagOnlyRecoveryState(context(fake), {
            tag,
            expectedCommit: commit,
            expectedObject: object,
          }),
      };
    };

    expect(() => execute().run()).not.toThrow();
    for (const scenario of [
      { local: "" },
      { local: "b".repeat(40) },
      { local: commandFailure("local unreadable") },
      { localType: "commit" },
      { localType: commandFailure("local type unreadable") },
      { localCommit: "b".repeat(40) },
      { localCommit: commandFailure("local peel unreadable") },
      { remote: "" },
      { remote: `${object}\t${ref}\n` },
      { remote: `${"b".repeat(40)}\t${ref}\n${commit}\t${ref}^{}\n` },
      { remote: `${object}\t${ref}\n${"b".repeat(40)}\t${ref}^{}\n` },
      { remote: commandFailure("remote unreadable") },
      {
        releases: JSON.stringify([
          [{ id: 7, tag_name: tag, draft: true }],
        ]),
      },
      {
        releases: JSON.stringify([
          [{ id: 8, tag_name: tag, draft: false }],
        ]),
      },
    ]) {
      const execution = execute(scenario);
      expect(() => execution.run()).toThrow();
      expect(
        execution.calls.some(
          (args) =>
            args.includes("push") ||
            args.includes("update-ref") ||
            args.includes("DELETE") ||
            args.includes("POST") ||
            args.includes("PATCH"),
        ),
      ).toBe(false);
    }
  });

  test("tag-only recovery reconstructs the reviewed object without changing a ref", () => {
    const root = temporaryDirectory("release-recovery-tag-object");
    const tag = "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0";
    const raw = Buffer.from(
      `object ${commit}\ntype commit\ntag ${tag}\n` +
        "tagger Takoform Form Package Release <release@takoform.invalid> 1 +0000\n\nexact\n",
    );
    writeFileSync(join(root, "tag-object"), raw);
    const object = "a".repeat(40);
    const calls = [];
    const fake = (_executable, args, options) => {
      calls.push([...args]);
      if (args.join(" ") === "rev-parse --show-object-format") return "sha1\n";
      if (args[0] === "mktag") {
        expect(Buffer.from(options.input)).toEqual(raw);
        return `${object}\n`;
      }
      throw new Error(`unexpected git ${args.join(" ")}`);
    };
    expect(
      releaseDeployTestHooks.reconstructCandidateTagObject(
        context(fake),
        tag,
        commit,
        root,
        {
          objectFormat: "sha1",
          tagObjectOid: object,
          tagObjectSha256: sha256(raw),
        },
      ),
    ).toBe(object);
    expect(
      calls.some(
        (args) =>
          args.includes("push") ||
          args.includes("update-ref") ||
          args.includes("DELETE"),
      ),
    ).toBe(false);
  });

  test("tag-only recovery fence permits current recovery code but rejects candidate input drift", () => {
    const root = temporaryDirectory("release-tag-only-fence");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    mkdirSync(join(root, "forms", "releases", "k-test", "1.0.0"), {
      recursive: true,
    });
    mkdirSync(join(root, "scripts"), { recursive: true });
    mkdirSync(join(root, ".github", "workflows"), { recursive: true });
    writeFileSync(join(root, "forms", "release-plan.json"), "{}\n");
    writeFileSync(
      join(root, "forms", "releases", "k-test", "1.0.0", "package-index.json"),
      "{}\n",
    );
    writeFileSync(
      join(root, ".github", "workflows", "form-package-release.yml"),
      "name: exact\n",
    );
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "old\n");
    runGit("add", ".");
    runGit("commit", "-m", "reviewed candidate tooling");
    const toolingCommit = runGit("rev-parse", "HEAD");

    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "recovery\n");
    runGit("add", "scripts/release-deploy.mjs");
    runGit("commit", "-m", "reviewed recovery");
    const recoveryCommit = runGit("rev-parse", "HEAD");
    const execution = context(execFileSync, { repo: root });
    const fence = (current) =>
      releaseDeployTestHooks.assertFormTagOnlyRecoveryFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        recoveryCommit: current,
        sourcePath: "forms/releases/k-test/1.0.0",
        label: "tag-only recovery test",
      });
    expect(() => fence(recoveryCommit)).not.toThrow();

    writeFileSync(
      join(root, "forms", "releases", "k-test", "1.0.0", "package-index.json"),
      '{"drift":true}\n',
    );
    runGit("add", ".");
    runGit("commit", "-m", "candidate source drift");
    expect(() => fence(runGit("rev-parse", "HEAD"))).toThrow(
      "candidate generation, identity, signing, or trust inputs changed",
    );
  });

  test("provider recovery permits only the reviewed implementation, tests, and documentation between E and F", () => {
    const root = temporaryDirectory("provider-recovery-fence");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    mkdirSync(join(root, "scripts", "testdata"), { recursive: true });
    mkdirSync(join(root, "release"), { recursive: true });
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "E\n");
    writeFileSync(join(root, "scripts", "release-deploy.test.mjs"), "E\n");
    writeFileSync(join(root, "release", "README.md"), "E\n");
    writeFileSync(
      join(
        root,
        "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
      ),
      "{}\n",
    );
    runGit("add", ".");
    runGit("commit", "-m", "provider release E");
    const releaseCommit = runGit("rev-parse", "HEAD");
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "F\n");
    writeFileSync(join(root, "scripts", "release-deploy.test.mjs"), "F\n");
    writeFileSync(join(root, "release", "README.md"), "F\n");
    writeFileSync(
      join(
        root,
        "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
      ),
      '{"fixture":"F"}\n',
    );
    runGit("add", ".");
    runGit("commit", "-m", "provider recovery F");
    const recoveryCommit = runGit("rev-parse", "HEAD");
    const execution = context(execFileSync, { repo: root });
    expect(
      releaseDeployTestHooks.assertProviderRecoveryFence(execution, {
        releaseCommit,
        recoveryCommit,
        label: "provider recovery test",
      }),
    ).toEqual([
      "release/README.md",
      "scripts/release-deploy.mjs",
      "scripts/release-deploy.test.mjs",
      "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
    ]);

    mkdirSync(join(root, ".github", "workflows"), { recursive: true });
    writeFileSync(join(root, ".github", "workflows", "release.yml"), "drift\n");
    runGit("add", ".");
    runGit("commit", "-m", "candidate workflow drift");
    const driftCommit = runGit("rev-parse", "HEAD");
    runGit("replace", driftCommit, recoveryCommit);
    expect(() =>
      releaseDeployTestHooks.assertProviderRecoveryFence(execution, {
        releaseCommit,
        recoveryCommit: driftCommit,
        label: "provider recovery test",
      }),
    ).toThrow("exact reviewed recovery implementation");
    runGit("replace", "-d", driftCommit);

    mkdirSync(join(root, "private"), { recursive: true });
    writeFileSync(join(root, "private", "provider-helper.mjs"), "hidden\n");
    runGit("add", ".");
    runGit("commit", "-m", "rename attack baseline");
    const renameBase = runGit("rev-parse", "HEAD");
    runGit("config", "diff.renames", "true");
    runGit(
      "mv",
      "private/provider-helper.mjs",
      "scripts/check-public-surfaces.mjs",
    );
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "rename F\n");
    runGit("add", ".");
    runGit("commit", "-m", "rename disallowed source to allowed destination");
    let renameError = "";
    try {
      releaseDeployTestHooks.assertProviderRecoveryFence(execution, {
        releaseCommit: renameBase,
        recoveryCommit: runGit("rev-parse", "HEAD"),
        label: "provider recovery rename test",
      });
    } catch (error) {
      renameError = error.message;
    }
    expect(renameError).toContain("private/provider-helper.mjs");
    expect(renameError).toContain("scripts/check-public-surfaces.mjs");
  });

  test("provider public readback binds current source F separately from immutable release E", () => {
    const root = temporaryDirectory("provider-readback-binding");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    writeFileSync(join(root, "release.txt"), "E\n");
    runGit("add", ".");
    runGit("commit", "-m", "provider release E");
    const providerReleaseCommit = runGit("rev-parse", "HEAD");
    writeFileSync(join(root, "recovery.txt"), "F\n");
    runGit("add", ".");
    runGit("commit", "-m", "readback source F");
    const sourceCommit = runGit("rev-parse", "HEAD");
    const execution = context(execFileSync, { repo: root });
    expect(
      releaseDeployTestHooks.assertProviderReadbackCommitBinding(execution, {
        sourceCommit,
        providerReleaseCommit,
      }),
    ).toEqual({ sourceCommit, providerReleaseCommit });
    expect(() =>
      releaseDeployTestHooks.assertProviderReadbackCommitBinding(execution, {
        sourceCommit: providerReleaseCommit,
        providerReleaseCommit: sourceCommit,
      }),
    ).toThrow();
  });

  test("provider recovery rechecks the exact signed tag after immutable publication and before VERIFIED", () => {
    const source = readFileSync(
      join(repositoryRoot, "scripts/release-deploy.mjs"),
      "utf8",
    );
    for (const [functionName, publicationCall] of [
      ["providerRecoverTagOnly", "publishReleaseLocally"],
      ["providerRecoverDraft", "resumeDraftReleaseLocally"],
    ]) {
      const start = source.indexOf(`function ${functionName}(`);
      const end = source.indexOf("\nfunction ", start + 1);
      const body = source.slice(start, end);
      const publication = body.indexOf(
        `const release = ${publicationCall}(`,
      );
      const preDraftFence = body.indexOf(
        "providerRecoveryMutationFence(context,",
      );
      const prePatchFence = body.indexOf(
        "providerRecoveryMutationFence(context,",
        publication,
      );
      const postTagFence = body.indexOf(
        "assertExactSignedProviderTag(context,",
        publication,
      );
      const verifiedEmit = body.indexOf("return emit(context,", publication);
      expect(start).toBeGreaterThanOrEqual(0);
      expect(preDraftFence).toBeGreaterThanOrEqual(0);
      expect(publication).toBeGreaterThan(preDraftFence);
      expect(publication).toBeGreaterThanOrEqual(0);
      expect(prePatchFence).toBeGreaterThan(publication);
      expect(postTagFence).toBeGreaterThan(publication);
      expect(postTagFence).toBeGreaterThan(prePatchFence);
      expect(verifiedEmit).toBeGreaterThan(postTagFence);
    }
  });

  test("provider recovery immutable-release preflight fails closed without a writer", () => {
    for (const response of [
      JSON.stringify({ enabled: false }),
      JSON.stringify({}),
      "[]",
      "not JSON",
      commandFailure("immutable setting unreadable"),
    ]) {
      const calls = [];
      const fake = (_executable, args) => {
        calls.push([...args]);
        if (response instanceof Error) throw response;
        return response;
      };
      expect(() =>
        releaseDeployTestHooks.assertReleaseImmutabilityEnabled(
          context(fake),
        ),
      ).toThrow();
      expect(
        calls.some(
          (args) =>
            args.includes("POST") ||
            args.includes("PATCH") ||
            args.includes("DELETE"),
        ),
      ).toBe(false);
    }
    expect(() =>
      releaseDeployTestHooks.assertReleaseImmutabilityEnabled(
        context((_executable, args) => {
          expect(args).toEqual([
            "api",
            "repos/tako0614/terraform-provider-takoform/immutable-releases",
          ]);
          return JSON.stringify({ enabled: true });
        }),
      ),
    ).not.toThrow();
  });

  test("provider recovery requires the exact pinned signed local and remote annotated tag object", () => {
    const tag = "v1.0.1";
    const object = "a".repeat(40);
    const ref = `refs/tags/${tag}`;
    const calls = [];
    const signedTag = Buffer.from(
      `object ${commit}\ntype commit\ntag ${tag}\n` +
        "tagger Takoform Provider Release <release@takoform.invalid> 1 +0000\n\n" +
        "exact provider release\n" +
        "-----BEGIN PGP SIGNATURE-----\n\nZmFrZQ==\n=abcd\n" +
        "-----END PGP SIGNATURE-----\n",
    );
    const execute = (
      signer = "3510E75E05BBCC303B92D77934FC18AC897FB709",
    ) => {
      const fake = (executable, args) => {
        calls.push({ executable, args: [...args] });
        if (basename(executable) === "gpg" && args.includes("show-only")) {
          return "fpr:::::::::3510E75E05BBCC303B92D77934FC18AC897FB709:\n";
        }
        if (basename(executable) === "gpg" && args.includes("--import")) {
          return "";
        }
        if (basename(executable) === "gpg" && args.includes("--verify")) {
          return `[GNUPG:] VALIDSIG ${signer} 2026-07-30 0 4 0 1 10 00 ${signer}\n`;
        }
        if (executable === "git" && args[0] === "for-each-ref") {
          return `${object}\n`;
        }
        if (
          executable === "git" &&
          args[0] === "cat-file" &&
          args[1] === "-t"
        ) {
          return "tag\n";
        }
        if (
          executable === "git" &&
          args[0] === "cat-file" &&
          args[1] === "tag"
        ) {
          return signedTag;
        }
        if (executable === "git" && args[0] === "rev-parse") {
          return `${commit}\n`;
        }
        if (executable === "git" && args[0] === "ls-remote") {
          return `${object}\t${ref}\n${commit}\t${ref}^{}\n`;
        }
        throw new Error(`unexpected ${executable} ${args.join(" ")}`);
      };
      return releaseDeployTestHooks.assertExactSignedProviderTag(
        context(fake),
        {
          tag,
          expectedCommit: commit,
          expectedObject: object,
        },
      );
    };
    expect(execute().signerFingerprint).toBe(
      "3510E75E05BBCC303B92D77934FC18AC897FB709",
    );
    expect(() => execute("F".repeat(40))).toThrow("pinned exact signer");
    expect(() =>
      releaseDeployTestHooks.assertPinnedProviderGpgVerification(
        {
          ok: true,
          output: "",
          stderr:
            "[GNUPG:] VALIDSIG 3510E75E05BBCC303B92D77934FC18AC897FB709 spoofed\n",
        },
        tag,
      ),
    ).toThrow("pinned exact signer");
    expect(
      calls.some(
        (call) =>
          call.args.includes("verify-tag") ||
          call.args.includes("push") ||
          call.args.includes("update-ref") ||
          call.args.includes("DELETE"),
      ),
    ).toBe(false);
  });

  test("provider recovery never executes repository, global, or environment gpg.program overrides", () => {
    const root = temporaryDirectory("provider-gpg-config-isolation");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    mkdirSync(join(root, "release", "keys"), { recursive: true });
    writeFileSync(
      join(root, "release", "keys", "provider-signing-key.asc"),
      "fake pinned key fixture\n",
    );
    writeFileSync(join(root, "source.txt"), "provider release E\n");
    runGit("add", ".");
    runGit("commit", "-m", "provider release E");
    const sourceCommit = runGit("rev-parse", "HEAD");
    const tag = "v1.0.1";
    const rawTag = Buffer.from(
      `object ${sourceCommit}\ntype commit\ntag ${tag}\n` +
        "tagger Takoform Provider Release <release@takoform.invalid> 1 +0000\n\n" +
        "exact provider release\n" +
        "-----BEGIN PGP SIGNATURE-----\n\nZmFrZQ==\n=abcd\n" +
        "-----END PGP SIGNATURE-----\n",
    );
    const object = execFileSync(
      "git",
      ["hash-object", "-t", "tag", "-w", "--stdin"],
      { cwd: root, input: rawTag, encoding: "utf8" },
    ).trim();
    runGit("update-ref", `refs/tags/${tag}`, object);

    const marker = join(root, "malicious-gpg-ran");
    const maliciousGpg = join(root, "malicious-gpg");
    writeFileSync(
      maliciousGpg,
      `#!/bin/sh\nprintf invoked > ${JSON.stringify(marker)}\nprintf '[GNUPG:] VALIDSIG 3510E75E05BBCC303B92D77934FC18AC897FB709 spoofed\\n' >&2\nexit 0\n`,
    );
    chmodSync(maliciousGpg, 0o755);
    runGit("config", "gpg.program", maliciousGpg);
    const globalConfig = join(root, "global.gitconfig");
    writeFileSync(
      globalConfig,
      `[gpg]\n\tprogram = ${maliciousGpg}\n`,
    );
    const environmentNames = [
      "GIT_CONFIG_GLOBAL",
      "GIT_CONFIG_COUNT",
      "GIT_CONFIG_KEY_0",
      "GIT_CONFIG_VALUE_0",
    ];
    const previous = Object.fromEntries(
      environmentNames.map((name) => [name, process.env[name]]),
    );
    process.env.GIT_CONFIG_GLOBAL = globalConfig;
    process.env.GIT_CONFIG_COUNT = "1";
    process.env.GIT_CONFIG_KEY_0 = "gpg.program";
    process.env.GIT_CONFIG_VALUE_0 = maliciousGpg;
    const calls = [];
    const signer = "3510E75E05BBCC303B92D77934FC18AC897FB709";
    try {
      const fake = (executable, args, options) => {
        calls.push({ executable, args: [...args], env: options?.env });
        if (basename(executable) === "gpg" && args.includes("show-only")) {
          return `fpr:::::::::${signer}:\n`;
        }
        if (basename(executable) === "gpg" && args.includes("--import")) {
          return "";
        }
        if (basename(executable) === "gpg" && args.includes("--verify")) {
          return `[GNUPG:] VALIDSIG ${signer} 2026-07-30 0 4 0 1 10 00 ${signer}\n`;
        }
        if (executable === "git" && args[0] === "ls-remote") {
          const ref = `refs/tags/${tag}`;
          return `${object}\t${ref}\n${sourceCommit}\t${ref}^{}\n`;
        }
        return execFileSync(executable, args, options);
      };
      expect(
        releaseDeployTestHooks.assertExactSignedProviderTag(
          context(fake, { repo: root }),
          {
            tag,
            expectedCommit: sourceCommit,
            expectedObject: object,
          },
        ).signerFingerprint,
      ).toBe(signer);
    } finally {
      for (const name of environmentNames) {
        if (previous[name] === undefined) delete process.env[name];
        else process.env[name] = previous[name];
      }
    }
    expect(existsSync(marker)).toBe(false);
    expect(
      calls.some(
        (call) =>
          call.executable === "git" && call.args.includes("verify-tag"),
      ),
    ).toBe(false);
    for (const call of calls.filter((entry) => entry.executable === "git")) {
      expect(call.env.GIT_NO_REPLACE_OBJECTS).toBe("1");
      expect(call.env.GIT_CONFIG_COUNT).toBeUndefined();
      expect(call.env.GIT_CONFIG_GLOBAL).toBe("/dev/null");
    }
  });

  test("pre-PATCH authority fence allows unrelated main advance and blocks Form or revocation drift", () => {
    const root = temporaryDirectory("release-authority-fence");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    mkdirSync(join(root, "scripts"), { recursive: true });
    mkdirSync(join(root, "forms", "releases", "k-test", "1.0.0"), {
      recursive: true,
    });
    mkdirSync(join(root, "forms", "revocations"), { recursive: true });
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "authority\n");
    writeFileSync(join(root, "forms", "release-plan.json"), "{}\n");
    writeFileSync(
      join(root, "forms", "releases", "k-test", "1.0.0", "package-index.json"),
      "{}\n",
    );
    writeFileSync(
      join(root, "forms", "revocations", "checkpoint.json"),
      "{}\n",
    );
    writeFileSync(join(root, "README.md"), "unrelated\n");
    runGit("add", ".");
    runGit("commit", "-m", "reviewed tooling");
    const toolingCommit = runGit("rev-parse", "HEAD");

    writeFileSync(join(root, "README.md"), "unrelated main advance\n");
    runGit("add", "README.md");
    runGit("commit", "-m", "unrelated advance");
    const unrelatedMain = runGit("rev-parse", "HEAD");
    const execution = context(execFileSync, { repo: root });
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: unrelatedMain,
        sourcePath: "forms/releases/k-test/1.0.0",
        label: "Form Package pre-publish fence",
      }),
    ).not.toThrow();
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: unrelatedMain,
        revocation: true,
        label: "revocation pre-publish fence",
      }),
    ).not.toThrow();

    writeFileSync(join(root, "forms", "release-plan.json"), '{"drift":true}\n');
    runGit("add", "forms/release-plan.json");
    runGit("commit", "-m", "Form authority drift");
    const formDriftMain = runGit("rev-parse", "HEAD");
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: formDriftMain,
        sourcePath: "forms/releases/k-test/1.0.0",
        label: "Form Package pre-publish fence",
      }),
    ).toThrow("release authority paths changed");
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: formDriftMain,
        revocation: true,
        label: "revocation pre-publish fence",
      }),
    ).not.toThrow();

    writeFileSync(
      join(root, "forms", "revocations", "checkpoint.json"),
      '{"drift":true}\n',
    );
    runGit("add", "forms/revocations/checkpoint.json");
    runGit("commit", "-m", "revocation authority drift");
    const revocationDriftMain = runGit("rev-parse", "HEAD");
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: revocationDriftMain,
        revocation: true,
        label: "revocation pre-publish fence",
      }),
    ).toThrow("release authority paths changed");
  });

  test("pre-PATCH draft validator rejects changed identity, state, and duplicate asset ids", () => {
    const root = temporaryDirectory("release-draft-validation");
    const firstPath = join(root, "first.txt");
    const secondPath = join(root, "second.txt");
    writeFileSync(firstPath, "first\n");
    writeFileSync(secondPath, "second\n");
    const assets = new Map([
      [
        "first.txt",
        { name: "first.txt", path: firstPath, sha256: sha256(readFileSync(firstPath)) },
      ],
      [
        "second.txt",
        { name: "second.txt", path: secondPath, sha256: sha256(readFileSync(secondPath)) },
      ],
    ]);
    const draft = {
      id: 7,
      tag_name: "v1.0.0",
      draft: true,
      prerelease: false,
      assets: [...assets.values()].map((asset, index) => ({
        id: index + 9,
        name: asset.name,
        state: "uploaded",
        digest: asset.sha256,
        size: readFileSync(asset.path).length,
      })),
    };
    const validate = (candidate) =>
      releaseDeployTestHooks.validateDraftBeforePublication(candidate, {
        releaseId: 7,
        tag: "v1.0.0",
        prerelease: false,
        assets,
      });
    expect(() => validate(structuredClone(draft))).not.toThrow();
    for (const mutate of [
      (candidate) => (candidate.id = 8),
      (candidate) => (candidate.draft = false),
      (candidate) => (candidate.prerelease = true),
      (candidate) => (candidate.assets[0].state = "new"),
      (candidate) => (candidate.assets[1].id = candidate.assets[0].id),
    ]) {
      const candidate = structuredClone(draft);
      mutate(candidate);
      expect(() => validate(candidate)).toThrow();
    }
  });

  test("retained-draft public readback rejects release metadata drift", () => {
    const fixture = assetFixture();
    const tag = "forms/k-jvxwizlmivxgi4dpnfxhi/v3.0.0";
    const body = "exact retained draft body";
    const assetsURL =
      "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets";
    const uploadURL =
      "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}";
    const release = {
      id: 7,
      tag_name: tag,
      target_commitish: "main",
      name: tag,
      body,
      draft: false,
      prerelease: false,
      immutable: true,
      html_url: "https://github.com/example/recovered-release",
      assets_url: assetsURL,
      upload_url: uploadURL,
      assets: [
        {
          id: 100,
          name: "asset.txt",
          state: "uploaded",
          digest: fixture.digest,
          size: readFileSync(fixture.path).length,
        },
      ],
    };
    const validate = (candidate) =>
      releaseDeployTestHooks.validateReleaseReadback(
        candidate,
        tag,
        fixture.assets,
        {
          expectedReleaseId: 7,
          expectedName: tag,
          expectedBody: body,
          expectedTargetCommitish: "main",
          expectedAssetsURL: assetsURL,
          expectedUploadURL: uploadURL,
        },
      );
    expect(() => validate(structuredClone(release))).not.toThrow();
    for (const mutate of [
      (candidate) => (candidate.id = 8),
      (candidate) => (candidate.name = "changed"),
      (candidate) => (candidate.body = "changed"),
      (candidate) => (candidate.target_commitish = "changed"),
      (candidate) => (candidate.assets_url = "https://example.invalid/assets"),
      (candidate) => (candidate.upload_url = "https://example.invalid/upload"),
    ]) {
      const candidate = structuredClone(release);
      mutate(candidate);
      expect(() => validate(candidate)).toThrow();
    }
  });

  test("tag failure observation never reports unreadable state as unchanged or masks stderr failure", () => {
    const fake = (_executable, args) => {
      if (args[0] === "for-each-ref") {
        throw commandFailure("local refs unreadable");
      }
      if (args[0] === "ls-remote") return "";
      throw new Error(`unexpected git ${args.join(" ")}`);
    };
    const execution = context(fake);
    expect(
      releaseDeployTestHooks.observeTagFailureState(execution, "v1.0.0")
        .mutationState,
    ).toBe("LOCAL_UNREADABLE");
    expect(() =>
      releaseDeployTestHooks.reportTagFailure(
        context(fake, {
          stderr: {
            write: () => {
              throw new Error("stderr unavailable");
            },
          },
        }),
        "v1.0.0",
        "89abcdef0123456789abcdef0123456789abcdef",
      ),
    ).not.toThrow();
  });
});

test("verify-all leaves no final or plausible set when trusted-root Cosign fails", () => {
  const parent = temporaryDirectory("release-verify-all");
  const output = join(parent, "publication-set");
  const plan = JSON.parse(
    readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
  );
  const calls = [];
  const fake = (executable, args, options) => {
    calls.push({ executable, args: [...args], env: { ...options.env } });
    if (executable === "gh" && args[0] === "--version") {
      return "gh version 2.96.0 (2026-07-02)\n";
    }
    if (executable === "cosign" && args[0] === "version") {
      return "GitVersion:    v3.0.6\n";
    }
    if (executable === "git") {
      if (args[0] === "status") return "";
      if (args.join(" ") === "rev-parse --is-shallow-repository") {
        return "false\n";
      }
      if (args.join(" ") === "remote get-url origin") {
        return "https://github.com/tako0614/terraform-provider-takoform.git\n";
      }
      if (args[0] === "symbolic-ref") return "main\n";
      if (args[0] === "fetch") return "";
      if (args.join(" ") === "rev-parse HEAD") return `${commit}\n`;
      if (args.join(" ") === "rev-parse refs/remotes/origin/main") {
        return `${commit}\n`;
      }
      if (
        args[0] === "show" &&
        args[1] === `${commit}:admission/v4/trust/trusted-root.json`
      ) {
        return "{}\n";
      }
    }
    if (executable === "go") {
      const staging = args[args.indexOf("--output-root") + 1];
      mkdirSync(staging, { recursive: true });
      writePublicationSetFixture(staging, plan);
      return "";
    }
    if (executable === "cosign") {
      throw commandFailure("trusted-root verification failed");
    }
    throw new Error(`unexpected ${executable} ${args.join(" ")}`);
  };
  expect(() =>
    releaseDeployTestHooks.formVerifyAll(
      context(fake),
      { "output-root": output },
      plan,
    ),
  ).toThrow("cosign verify-blob");
  expect(existsSync(output)).toBe(false);
  expect(
    readdirSync(parent).filter((name) =>
      name.includes(".publication-set.takoform-staging-"),
    ),
  ).toEqual([]);
  const goCall = calls.find((call) => call.executable === "go");
  expect(goCall.env.GH_TOKEN).toBeUndefined();
  expect(goCall.env.GITHUB_TOKEN).toBe("operator-only-test-token");
  const cosignCall = calls.find((call) => call.executable === "cosign");
  expect(cosignCall.env.GH_TOKEN).toBeUndefined();
  expect(cosignCall.env.GITHUB_TOKEN).toBeUndefined();
});

test("verify-all atomic commit preserves a destination that appears at commit instant", () => {
  const parent = temporaryDirectory("release-verify-all-race");
  const output = join(parent, "publication-set");
  const plan = JSON.parse(
    readFileSync(join(repositoryRoot, "forms/release-plan.json"), "utf8"),
  );
  const calls = [];
  const fake = (executable, args, options) => {
    calls.push({ executable, args: [...args], env: { ...options.env } });
    if (executable === "gh" && args[0] === "--version") {
      return "gh version 2.96.0 (2026-07-02)\n";
    }
    if (executable === "cosign" && args[0] === "version") {
      return "GitVersion:    v3.0.6\n";
    }
    if (executable === "git") {
      if (args[0] === "status") return "";
      if (args.join(" ") === "rev-parse --is-shallow-repository") {
        return "false\n";
      }
      if (args.join(" ") === "remote get-url origin") {
        return "https://github.com/tako0614/terraform-provider-takoform.git\n";
      }
      if (args[0] === "symbolic-ref") return "main\n";
      if (args[0] === "fetch") return "";
      if (args.join(" ") === "rev-parse HEAD") return `${commit}\n`;
      if (args.join(" ") === "rev-parse refs/remotes/origin/main") {
        return `${commit}\n`;
      }
      if (
        args[0] === "show" &&
        args[1] === `${commit}:admission/v4/trust/trusted-root.json`
      ) {
        return "{}\n";
      }
    }
    if (
      executable === "go" &&
      args.includes("./cmd/published-package-set") &&
      args.includes("download-plan")
    ) {
      const staging = args[args.indexOf("--output-root") + 1];
      mkdirSync(staging, { recursive: true });
      writePublicationSetFixture(staging, plan);
      return "";
    }
    if (
      executable === "go" &&
      args.includes("./cmd/release-output-commit") &&
      args.includes("probe")
    ) {
      return '{"format":"takoform.release-output-commit@v1","status":"verified"}\n';
    }
    if (
      executable === "go" &&
      args.includes("./cmd/release-output-commit") &&
      args.includes("commit")
    ) {
      const target = args[args.indexOf("--target") + 1];
      mkdirSync(target);
      writeFileSync(join(target, "racer.txt"), "racer-owned\n");
      return execFileSync(executable, args, options);
    }
    if (executable === "cosign" && args[0] === "verify-blob") return "";
    throw new Error(`unexpected ${executable} ${args.join(" ")}`);
  };
  expect(() =>
    releaseDeployTestHooks.formVerifyAll(
      context(fake),
      { "output-root": output },
      plan,
    ),
  ).toThrow("go run ./cmd/release-output-commit commit");
  expect(readFileSync(join(output, "racer.txt"), "utf8")).toBe("racer-owned\n");
  expect(readdirSync(output)).toEqual(["racer.txt"]);
  expect(
    readdirSync(parent).filter((name) =>
      name.includes(".publication-set.takoform-staging-"),
    ),
  ).toEqual([]);
  expect(calls.some((call) => call.executable === "mv")).toBe(false);
});

import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";

import {
  RELEASE_SURFACES,
  parseReleaseSurfaceArgs,
  parseStrictChecksums,
  providerReleaseBody,
  releaseDeployTestHooks,
  runReleaseSurface,
} from "./release-deploy.mjs";
import {
  EXPECTED_CANDIDATE,
  EXPECTED_RESERVED,
  LEDGER_KIND as SPECIFICATION_LEDGER_KIND,
} from "./specification-release.mjs";

const repositoryRoot = resolve(import.meta.dir, "..");
const commit = "0123456789abcdef0123456789abcdef01234567";
const requestId = "01234567-89ab-4cde-8fab-0123456789ab";
const temporaryDirectories = [];
let previousGH;
let previousGitHub;
let previousGHEnterprise;
let previousGitHubEnterprise;
let previousPath;

beforeEach(() => {
  previousGH = process.env.GH_TOKEN;
  previousGitHub = process.env.GITHUB_TOKEN;
  previousGHEnterprise = process.env.GH_ENTERPRISE_TOKEN;
  previousGitHubEnterprise = process.env.GITHUB_ENTERPRISE_TOKEN;
  previousPath = process.env.PATH;
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
  if (previousPath === undefined) delete process.env.PATH;
  else process.env.PATH = previousPath;
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { recursive: true, force: true });
  }
});

function temporaryDirectory(prefix) {
  const directory = mkdtempSync(join(tmpdir(), `${prefix}-`));
  temporaryDirectories.push(directory);
  return directory;
}

function trustedTemporaryDirectory(prefix) {
  const directory = mkdtempSync(
    join(dirname(repositoryRoot), `.takoform-${prefix}-`),
  );
  temporaryDirectories.push(directory);
  return directory;
}

function writeExecutable(path) {
  writeFileSync(path, "#!/bin/sh\nexit 0\n");
  chmodSync(path, 0o500);
}

function replaceManagedReadOnlyFile(path, bytes = "mutated\n", mode = 0o400) {
  chmodSync(path, 0o700);
  writeFileSync(path, bytes);
  chmodSync(path, mode);
}

function nominateOwnerGateTools() {
  const root = trustedTemporaryDirectory("owner-gate-tools");
  const tofuBin = join(root, "tofu-bin");
  const terraformBin = join(root, "terraform-user-bin");
  const goRoot = join(root, "go-root");
  const goBin = join(goRoot, "bin");
  const goToolDir = join(goRoot, "pkg", "tool", "linux_amd64");
  const goIncludeDir = join(goRoot, "pkg", "include");
  const goSourceDir = join(goRoot, "src", "runtime");
  for (const directory of [
    tofuBin,
    terraformBin,
    goBin,
    goToolDir,
    goIncludeDir,
    goSourceDir,
  ]) {
    mkdirSync(directory, { recursive: true, mode: 0o700 });
  }
  for (const path of [
    join(tofuBin, "tofu"),
    join(terraformBin, "terraform"),
    join(goBin, "go"),
    join(goBin, "gofmt"),
    join(goToolDir, "compile"),
  ]) {
    writeExecutable(path);
  }
  writeFileSync(join(goIncludeDir, "textflag.h"), "#define TEXTFLAG 1\n");
  writeFileSync(join(goSourceDir, "runtime.go"), "package runtime\n");
  chmodSync(join(goIncludeDir, "textflag.h"), 0o400);
  chmodSync(join(goSourceDir, "runtime.go"), 0o400);
  writeExecutable(join(terraformBin, "bun"));
  process.env.PATH = [
    tofuBin,
    terraformBin,
    goBin,
    "/usr/local/bin",
    "/usr/bin",
    "/bin",
  ].join(":");
  return {
    goBin,
    goIncludeDir,
    goRoot,
    goSourceDir,
    goToolDir,
    terraformBin,
    tofuBin,
  };
}

function ownerGateToolOutput(tools, executable, args, overrides = {}) {
  const name = basename(executable);
  if (name === "go" && args.join(" ") === "env GOROOT GOTOOLDIR GOOS GOARCH") {
    const managedRoot =
      overrides.goroot ?? dirname(dirname(resolve(executable)));
    const goos = overrides.goos ?? "linux";
    const goarch = overrides.goarch ?? "amd64";
    const gotooldir =
      typeof overrides.gotooldir === "function"
        ? overrides.gotooldir(managedRoot)
        : (overrides.gotooldir ??
          join(managedRoot, "pkg", "tool", `${goos}_${goarch}`));
    return `${managedRoot}\n${gotooldir}\n${goos}\n${goarch}\n`;
  }
  if (name === "go" && args.join(" ") === "version") {
    return `${overrides.goVersion ?? "go version go1.26.5 linux/amd64"}\n`;
  }
  if (name === "go" && args[0] === "version" && args[1] === "-m") {
    return (
      (typeof overrides.gofmtBuild === "function"
        ? overrides.gofmtBuild(args[2])
        : overrides.gofmtBuild) ??
      `${args[2]}: go1.26.5\n\tpath\tcmd/gofmt\n\tbuild\tGOOS=linux\n`
    );
  }
  if (
    name === "go" &&
    [
      "mod download",
      "-C cmd/provider-release mod download",
      "test -run ^$ ./...",
      "-C cmd/provider-release test -run ^$ ./...",
    ].includes(args.join(" "))
  ) {
    return "";
  }
  if (name === "tofu" && args.join(" ") === "version -json") {
    return overrides.tofuJSON ?? '{"terraform_version":"1.12.3"}\n';
  }
  if (name === "tofu" && args.join(" ") === "version") {
    return `${overrides.tofuPlain ?? "OpenTofu v1.12.3"}\n`;
  }
  if (name === "terraform" && args.join(" ") === "version -json") {
    return overrides.terraformJSON ?? '{"terraform_version":"1.15.8"}\n';
  }
  if (name === "terraform" && args.join(" ") === "version") {
    return `${overrides.terraformPlain ?? "Terraform v1.15.8"}\n`;
  }
  return null;
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

// An independent copy of the canonicalization the writer pins with, so a
// change to either side has to be made deliberately on both.
function canonicalLedgerDigest(value) {
  const sorted = (input) => {
    if (Array.isArray(input)) return input.map(sorted);
    if (input === null || typeof input !== "object") return input;
    return Object.fromEntries(
      Object.keys(input)
        .sort()
        .map((key) => [key, sorted(input[key])]),
    );
  };
  return `sha256:${createHash("sha256")
    .update(JSON.stringify(sorted(value)))
    .digest("hex")}`;
}

test("provider v4 release body names the publisher-set identity and v3 migration boundary", () => {
  const descriptor = JSON.parse(
    readFileSync(join(repositoryRoot, "release/version.json"), "utf8"),
  );
  const body = providerReleaseBody(descriptor);
  expect(body).toContain("Provider v4.0.0");
  expect(body).toContain("forms.takoform.com/v1");
  expect(body).toContain(
    "single versionless current Form family `edge.forms.takoform.com`",
  );
  expect(body).toContain("17 Experimental 0.x FormRefs");
  expect(body).toContain("release/provider-form-identities.json");
  expect(body).toContain("Provider SemVer, Host API, Form Family");
  expect(body).toContain("Breaking upgrade from Provider v3.0.0");
  expect(body).toContain("15 withdrawn aggregate Terraform resource types");
  expect(body).toContain("release/migrations/v3-to-v4.md");
  expect(body).toContain("release/migrations/v2-to-v3.md");
  expect(body).toContain("release/migrations/v1-to-v2.md");
  expect(body).toContain("github.com/tako0614/takoform-forms");
  expect(body).toContain("forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c");
  expect(body).toContain("3231633605b737ce5279d7fc020b4780568e7091");
  // The retained majors must stay named in the immutable public record.
  expect(body).toContain(
    "Provider v2.1.1 and Provider v1 remain separate migration boundaries",
  );
  expect(body).toContain(
    "Provider 2.1.1 and 3.0.0 identities remain immutable history",
  );
});

test("provider descriptor and identity ledger are exact current and retained release inputs", () => {
  const descriptor =
    releaseDeployTestHooks.readProviderDescriptor(repositoryRoot);
  expect(descriptor.version).toBe("4.0.0");
  expect(descriptor.versioning.portableApiVersion).toBe(
    "forms.takoform.com/v1",
  );
  const releases = releaseDeployTestHooks.validateProviderIdentityLedger(
    repositoryRoot,
    descriptor,
  ).releases;
  expect(
    releases.find((release) => release.providerVersion === "2.1.1")?.forms,
  ).toHaveLength(15);
  const current = releases.find(
    (release) => release.providerVersion === "4.0.0",
  );
  expect(current?.families).toEqual(["edge.forms.takoform.com"]);
  expect(current?.forms).toHaveLength(17);
  // Provider 3.0.0 is Registry-published immutable history: it keeps its exact
  // eight-family, 31-Form projection and its canonical digest even though the
  // writer no longer addresses it.
  const retained = releases.find(
    (release) => release.providerVersion === "3.0.0",
  );
  expect(retained?.families).toHaveLength(8);
  expect(retained?.forms).toHaveLength(31);
  expect(canonicalLedgerDigest(retained)).toBe(
    "sha256:165f9377f0a37d1994d96e28c7494dc71dc4e6457d0679229a7f1819c17f77fb",
  );
});

function context(execFile, overrides = {}) {
  const io = memoryIO();
  return {
    repo: repositoryRoot,
    githubConfigDirectory: temporaryDirectory("release-gh-config"),
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

function safeReleaseGitConfiguration() {
  return [
    "core.repositoryformatversion\n0",
    "core.filemode\ntrue",
    "core.bare\nfalse",
    "core.logallrefupdates\ntrue",
    "remote.origin.url\nhttps://github.com/tako0614/terraform-provider-takoform.git",
    "remote.origin.fetch\n+refs/heads/*:refs/remotes/origin/*",
    "branch.main.remote\norigin",
    "branch.main.merge\nrefs/heads/main",
    "",
  ].join("\0");
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
}) {
  const raw = Buffer.from(
    `object ${sourceCommit}\ntype commit\ntag ${tag}\n` +
      "tagger Takoform Form Package Revocation " +
      "<release@takoform.invalid> 1 +0000\n\n" +
      `Takoform Form Package revocation checkpoint ${tag}\n\n` +
      `source-commit: ${sourceCommit}\n` +
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
  { tag, runId, runAttempt, sourceCommit, toolingCommit },
) {
  const version = /^forms\/revocations\/v(.+)$/u.exec(tag)[1];
  const base = `takoform-form-revocation_${version}`;
  const names = [
    `${base}_checkpoint.json`,
    `${base}_checkpoint.sigstore.json`,
    `${base}_statement.json`,
    `${base}_provenance.intoto.json`,
    "release-manifest.json",
    "SHA256SUMS",
  ].sort();
  const subject = `${base}_checkpoint.json`;
  const bundle = `${base}_checkpoint.sigstore.json`;
  const payloadNames = names.filter(
    (name) => name !== "release-manifest.json" && name !== "SHA256SUMS",
  );
  const assetsRoot = join(destination, "assets");
  mkdirSync(assetsRoot, { recursive: true });
  for (const name of payloadNames) {
    writeFileSync(join(assetsRoot, name), Buffer.from(`${name}\n`));
  }
  const manifest = {
    schemaVersion: 1,
    releaseType: "form-package-revocation",
    tag,
    sourceRepository: "github.com/tako0614/terraform-provider-takoform",
    sourceCommit,
    toolingCommit,
    workflow: ".github/workflows/form-package-revocation.yml",
    packageVersion: version,
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
  writeChecksumFixture(assetsRoot, [...payloadNames, "release-manifest.json"]);

  const metadataAssets = names.map((name) => ({
    name,
    sha256: sha256(readFileSync(join(assetsRoot, name))),
  }));
  const metadata = {
    format: "takoform.form-package-revocation-candidate@v1",
    repository: "tako0614/terraform-provider-takoform",
    workflowPath: ".github/workflows/form-package-revocation.yml",
    workflowRef:
      "tako0614/terraform-provider-takoform/.github/workflows/" +
      "form-package-revocation.yml@refs/heads/main",
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
    assetCount: names.length,
    assets: metadataAssets,
  };
  const tagObject = tagObjectFixture({
    tag,
    sourceCommit,
    runId,
    runAttempt,
  });
  metadata.tagObjectOid = tagObject.oid;
  metadata.tagObjectSha256 = sha256(tagObject.raw);
  writeFileSync(
    join(destination, "metadata.json"),
    `${JSON.stringify(recursivelySorted(metadata), null, 2)}\n`,
  );
  writeFileSync(join(destination, "tag-object"), tagObject.raw);
  writeChecksumFixture(destination, [
    ...names.map((name) => `assets/${name}`),
    "metadata.json",
    "tag-object",
  ]);
}

describe("release surface contract and strict parsing", () => {
  test("declares owner authority, no-overwrite, and asynchronous halt", () => {
    expect(RELEASE_SURFACES.map((surface) => surface.surface)).toEqual([
      "takoform-provider-release",
      "takoform-form-package-release",
      "takoform-specification-release",
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
    const specification = RELEASE_SURFACES.find(
      (surface) => surface.surface === "takoform-specification-release",
    );
    expect(specification.covers).toEqual([
      "spec",
      "release/specification-releases.json",
      "spec/publication-evidence.json",
      "scripts/specification-release.mjs",
    ]);
    expect(specification.requiresScripts).toEqual([
      "check",
      "check:specification-releases",
      "check:specification-1-1-release",
    ]);
    expect(specification.obligations.provenance).toContain(
      "separately checked W09 report and is not release authority",
    );
    expect(specification.obligations.provenance).toContain(
      "normal history is C2 -> C3 -> C4",
    );
    expect(specification.obligations.provenance).toContain(
      "sole exceptional history is C2 -> R -> C3 -> C4",
    );
    expect(specification.obligations.provenance).toContain(
      "R is one direct single-parent child of C2",
    );
    for (const relativePath of [
      "scripts/release-deploy.mjs",
      "scripts/release-deploy.test.mjs",
      "scripts/specification-release.mjs",
      "scripts/specification-release.test.mjs",
    ]) {
      expect(specification.obligations.provenance).toContain(relativePath);
    }
    expect(specification.obligations.provenance).not.toContain(
      "release/README.md",
    );
    expect(specification.obligations.provenance).not.toContain(
      "website/release/index.md",
    );
    expect(specification.obligations["post-conditions"]).toContain(
      "expected-recovery-commit equal to C2 normally or the exact R",
    );
    expect(specification.obligations["post-conditions"]).toContain(
      "repeats the recovery fence before live readback and immediately before writing",
    );
    expect(specification.obligations["post-conditions"]).toContain(
      "keeps receipt.releaseCommit, tag, release body, and asset authority bound to immutable C2",
    );
    expect(specification.obligations["failure-handling"]).toContain(
      "extra or merge-parent recovery edge",
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

    expect(
      parseReleaseSurfaceArgs("takoform-form-package-release", [
        "prepare-revocation",
        "--tag",
        "forms/revocations/v1.0.0",
        "--expected-commit",
        commit,
      ]).phase,
    ).toBe("prepare-revocation");
    expect(
      parseReleaseSurfaceArgs("takoform-form-package-release", [
        "publish-revocation",
        "--tag",
        "forms/revocations/v1.0.0",
        "--expected-commit",
        commit,
        "--run-id",
        "123",
        "--run-attempt",
        "1",
      ]),
    ).toEqual({
      phase: "publish-revocation",
      tag: "forms/revocations/v1.0.0",
      "expected-commit": commit,
      "run-id": "123",
      "run-attempt": "1",
    });
    expect(
      parseReleaseSurfaceArgs("takoform-form-package-release", [
        "verify-revocation",
        "--tag",
        "forms/revocations/v1.0.0",
        "--expected-commit",
        commit,
      ]).phase,
    ).toBe("verify-revocation");
    expect(
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "prepare",
        "--tag",
        "specification/1.1",
        "--expected-commit",
        commit,
      ]),
    ).toEqual({
      phase: "prepare",
      tag: "specification/1.1",
      "expected-commit": commit,
    });
    expect(() =>
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "publish",
        "--tag",
        "specification/v1.1",
        "--expected-commit",
        commit,
      ]),
    ).toThrow();
    expect(
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "recover-tag-only",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        commit,
      ]),
    ).toEqual({
      phase: "recover-tag-only",
      tag: "specification/1.1",
      "expected-release-commit": commit,
      "expected-tag-object": "a".repeat(40),
      "expected-recovery-commit": commit,
    });
    expect(
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "recover-draft",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        "b".repeat(40),
        "--release-id",
        "41",
      ])["release-id"],
    ).toBe("41");
    expect(
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "verify",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--release-id",
        "41",
      ]).phase,
    ).toBe("verify");
    expect(
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "record-receipt",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--expected-recovery-commit",
        commit,
        "--release-id",
        "41",
      ]),
    ).toMatchObject({
      phase: "record-receipt",
      "expected-release-commit": commit,
      "expected-recovery-commit": commit,
    });
    expect(() =>
      parseReleaseSurfaceArgs("takoform-specification-release", [
        "record-receipt",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        commit,
        "--expected-tag-object",
        "a".repeat(40),
        "--release-id",
        "41",
      ]),
    ).toThrow("usage:");

    for (const withdrawn of [
      "plan",
      "prepare",
      "publish",
      "publish-batch",
      "recover-tag-only",
      "recover-draft",
      "verify",
      "verify-all",
    ]) {
      expect(() =>
        parseReleaseSurfaceArgs("takoform-form-package-release", [
          withdrawn,
          "--tag",
          "forms/revocations/v1.0.0",
          "--expected-commit",
          commit,
        ]),
      ).toThrow("usage:");
    }

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
      "expected-recovery-commit": "89abcdef0123456789abcdef0123456789abcdef",
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
    for (const invalid of [
      "HEAD",
      "A".repeat(40),
      "a".repeat(39),
      "a".repeat(41),
    ]) {
      expect(() =>
        parseReleaseSurfaceArgs("takoform-provider-release", [
          "recover-tag-only",
          "--tag",
          "v1.0.1",
          "--expected-release-commit",
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
          releaseId,
          "--run-id",
          "123",
          "--run-attempt",
          "1",
        ]),
      ).toThrow();
    }
  });

  test("Specification prepare is create-only, fail-closed, and does not mutate", () => {
    const ownerGateTools = nominateOwnerGateTools();
    const specificationCalls = [];
    const specificationIO = memoryIO();
    const specificationFake = (executable, args) => {
      specificationCalls.push({ executable, args });
      const ownerGateOutput = ownerGateToolOutput(
        ownerGateTools,
        executable,
        args,
      );
      if (ownerGateOutput !== null) return ownerGateOutput;
      if (
        executable === "git" &&
        args.join(" ") === "config --local -z --list"
      ) {
        return safeReleaseGitConfiguration();
      }
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (test)\n";
      }
      if (
        executable === "gh" &&
        args.join(" ") ===
          "api repos/tako0614/terraform-provider-takoform/immutable-releases"
      ) {
        return JSON.stringify({ enabled: true });
      }
      if (executable === "git" && args[0] === "status") return "";
      if (
        executable === "git" &&
        args.join(" ") === "rev-parse --is-shallow-repository"
      ) {
        return "false\n";
      }
      if (executable === "git" && args.join(" ") === "remote get-url origin") {
        return "https://github.com/tako0614/terraform-provider-takoform.git\n";
      }
      if (executable === "git" && args[0] === "symbolic-ref") return "main\n";
      if (executable === "git" && args[0] === "fetch") return "";
      if (executable === "git" && args[0] === "rev-parse") return `${commit}\n`;
      if (executable === "git" && args[0] === "cat-file") return "";
      if (executable === "git" && args[0] === "for-each-ref") return "";
      if (executable === "git" && args[0] === "ls-remote") return "";
      if (executable === "gh" && isReleaseList(args)) return "[[]]";
      if (executable === "bun") return "";
      throw new Error(
        `unexpected Specification preflight command: ${executable} ${args.join(" ")}`,
      );
    };
    const prepared = runReleaseSurface({
      surface: "takoform-specification-release",
      args: [
        "prepare",
        "--tag",
        "specification/1.1",
        "--expected-commit",
        commit,
      ],
      repo: repositoryRoot,
      stdout: specificationIO.stdout,
      stderr: specificationIO.stderr,
      execFile: specificationFake,
      uuidFactory: () => requestId,
      wait: () => {},
    });
    expect(prepared).toMatchObject({
      surface: "takoform-specification-release",
      phase: "prepare",
      tag: "specification/1.1",
      commit,
      status: "AWAITING_REVIEW",
      mutation: "none",
      prerequisite: "specification-source-snapshot",
    });
    expect(specificationIO.output).toContain('"mutation": "none"');
    expect(
      specificationCalls.some(
        ({ executable, args }) =>
          executable === "git" && (args[0] === "push" || args[0] === "tag"),
      ),
    ).toBe(false);
    expect(
      specificationCalls.some(
        ({ executable, args }) =>
          executable === "gh" &&
          args.join(" ") ===
            "api repos/tako0614/terraform-provider-takoform/immutable-releases",
      ),
    ).toBe(true);
    expect(
      specificationCalls.some(
        ({ executable, args }) =>
          executable === "gh" &&
          args[0] === "api" &&
          (args.includes("POST") || args.includes("PATCH")),
      ),
    ).toBe(false);
    expect(
      specificationCalls.some(({ executable }) => executable === "cosign"),
    ).toBe(false);
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
    expect(() => releaseDeployTestHooks.verifyChecksumClosure(root)).toThrow(
      "exact inventory",
    );
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
        Buffer.from('{\n  "requestId": "first",\n  "requestId": "second"\n}\n'),
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

describe("Specification 1.1 deterministic C2 publication inputs", () => {
  function createC2() {
    const root = temporaryDirectory("specification-c2");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    mkdirSync(join(root, "spec"), { recursive: true });
    mkdirSync(join(root, "release"), { recursive: true });
    copyFileSync(
      join(repositoryRoot, "release", "version.json"),
      join(root, "release", "version.json"),
    );
    mkdirSync(join(root, "website", "static", "spec"), { recursive: true });
    mkdirSync(join(root, "website", "public", "spec"), { recursive: true });
    mkdirSync(join(root, "website", "static", "release"), { recursive: true });
    mkdirSync(join(root, "website", "public", "release"), { recursive: true });
    mkdirSync(join(root, "scripts"), { recursive: true });
    const c1Evidence = {
      evidence: {
        specification: {
          sourceSnapshot: null,
          candidateCorpus: null,
          referenceConformance: null,
        },
      },
    };
    writeFileSync(
      join(root, "spec", "publication-evidence.json"),
      `${JSON.stringify(c1Evidence, null, 2)}\n`,
    );
    const ledgerRaw = `${JSON.stringify(
      {
        kind: SPECIFICATION_LEDGER_KIND,
        policy: "Specification 1.0 is withdrawn; 1.1 is create-only.",
        reserved: structuredClone(EXPECTED_RESERVED),
        candidate: structuredClone(EXPECTED_CANDIDATE),
        releases: [],
      },
      null,
      2,
    )}\n`;
    for (const relativePath of [
      ["release", "specification-releases.json"],
      ["website", "static", "release", "specification-releases.json"],
      ["website", "public", "release", "specification-releases.json"],
    ]) {
      writeFileSync(join(root, ...relativePath), ledgerRaw);
    }
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "C1\n");
    runGit("add", ".");
    runGit("commit", "-m", "Specification C1");
    const sourceCommit = runGit("rev-parse", "HEAD");
    const c2Evidence = structuredClone(c1Evidence);
    c2Evidence.evidence.specification.sourceSnapshot = {
      format: "takoform.specification-source-snapshot@v2",
      releaseVersion: "1.1",
      repository: "takoform",
      sourceCommit,
      roots: ["spec"],
      excludedPaths: [
        "spec/publication-evidence.json",
        "spec/publication-blockers.json",
      ],
      fileCount: 1,
      pathSetSha256: `sha256:${"a".repeat(64)}`,
      documentSetSha256: `sha256:${"b".repeat(64)}`,
    };
    const c2Raw = `${JSON.stringify(c2Evidence, null, 2)}\n`;
    writeFileSync(join(root, "spec", "publication-evidence.json"), c2Raw);
    writeFileSync(
      join(root, "website", "static", "spec", "publication-evidence.json"),
      c2Raw,
    );
    writeFileSync(
      join(root, "website", "public", "spec", "publication-evidence.json"),
      c2Raw,
    );
    runGit("add", ".");
    runGit("commit", "-m", "Specification C2 evidence only");
    return {
      root,
      runGit,
      sourceCommit,
      releaseCommit: runGit("rev-parse", "HEAD"),
    };
  }

  test("generates and zero-OID materializes one exact source-only annotated tag", () => {
    const fixture = createC2();
    const execution = context(execFileSync, { repo: fixture.root });
    const first = releaseDeployTestHooks.specificationPublicationInput(
      execution,
      fixture.releaseCommit,
    );
    const second = releaseDeployTestHooks.specificationPublicationInput(
      execution,
      fixture.releaseCommit,
    );
    expect(second.tagObject).toBe(first.tagObject);
    expect(second.tagObjectBytes).toEqual(first.tagObjectBytes);
    expect(first.sourceCommit).toBe(fixture.sourceCommit);
    expect(first.tagObjectBytes.toString("utf8")).toContain(
      `release-commit: ${fixture.releaseCommit}`,
    );
    expect(first.tagObjectBytes.toString("utf8")).not.toContain(
      "compatibility",
    );

    fixture.runGit("replace", fixture.releaseCommit, fixture.sourceCommit);
    const overriddenNames = [
      "GIT_ALTERNATE_OBJECT_DIRECTORIES",
      "GIT_CONFIG_COUNT",
      "GIT_CONFIG_KEY_0",
      "GIT_CONFIG_VALUE_0",
      "GIT_INDEX_FILE",
      "GIT_OBJECT_DIRECTORY",
      "GIT_REPLACE_REF_BASE",
      "GIT_WORK_TREE",
    ];
    const previous = Object.fromEntries(
      overriddenNames.map((name) => [name, process.env[name]]),
    );
    process.env.GIT_ALTERNATE_OBJECT_DIRECTORIES = "/tmp/attacker-objects";
    process.env.GIT_CONFIG_COUNT = "1";
    process.env.GIT_CONFIG_KEY_0 = "core.useReplaceRefs";
    process.env.GIT_CONFIG_VALUE_0 = "true";
    process.env.GIT_INDEX_FILE = "/tmp/attacker-index";
    process.env.GIT_OBJECT_DIRECTORY = "/tmp/attacker-object-directory";
    process.env.GIT_REPLACE_REF_BASE = "refs/attacker/";
    process.env.GIT_WORK_TREE = "/tmp/attacker-worktree";
    try {
      const isolated = releaseDeployTestHooks.specificationPublicationInput(
        execution,
        fixture.releaseCommit,
      );
      expect(isolated.tagObject).toBe(first.tagObject);
      expect(isolated.tagObjectBytes).toEqual(first.tagObjectBytes);
      expect(isolated.releaseCommit).toBe(fixture.releaseCommit);
    } finally {
      for (const name of overriddenNames) {
        if (previous[name] === undefined) delete process.env[name];
        else process.env[name] = previous[name];
      }
      fixture.runGit("replace", "-d", fixture.releaseCommit);
    }
    expect(
      releaseDeployTestHooks.materializeSpecificationTag(execution, first),
    ).toBe(first.tagObject);
    expect(
      fixture.runGit("rev-parse", "refs/tags/specification/1.1^{commit}"),
    ).toBe(fixture.releaseCommit);
    expect(() =>
      releaseDeployTestHooks.materializeSpecificationTag(execution, first),
    ).toThrow("use exact recovery");
  });

  test("rejects normative/tooling drift in C2 and fences reviewed recovery commits", () => {
    const fixture = createC2();
    const execution = context(execFileSync, { repo: fixture.root });
    expect(
      releaseDeployTestHooks.assertSpecificationC2Fence(
        execution,
        fixture.sourceCommit,
        fixture.releaseCommit,
      ),
    ).toEqual([
      "spec/publication-evidence.json",
      "website/public/spec/publication-evidence.json",
      "website/static/spec/publication-evidence.json",
    ]);

    writeFileSync(join(fixture.root, "scripts", "release-deploy.mjs"), "F\n");
    fixture.runGit("add", ".");
    fixture.runGit("commit", "-m", "reviewed recovery F");
    const recoveryCommit = fixture.runGit("rev-parse", "HEAD");
    expect(
      releaseDeployTestHooks.assertSpecificationRecoveryFence(execution, {
        releaseCommit: fixture.releaseCommit,
        recoveryCommit,
        label: "Specification recovery test",
      }),
    ).toEqual(["scripts/release-deploy.mjs"]);

    writeFileSync(join(fixture.root, "spec", "normative.md"), "drift\n");
    fixture.runGit("add", ".");
    fixture.runGit("commit", "-m", "forbidden normative drift");
    expect(() =>
      releaseDeployTestHooks.assertSpecificationRecoveryFence(execution, {
        releaseCommit: fixture.releaseCommit,
        recoveryCommit: fixture.runGit("rev-parse", "HEAD"),
        label: "Specification recovery test",
      }),
    ).toThrow("exact reviewed Specification recovery");

    const extra = createC2();
    extra.runGit("commit", "--allow-empty", "-m", "unexpected extra commit");
    writeFileSync(join(extra.root, "scripts", "release-deploy.mjs"), "R\n");
    extra.runGit("add", ".");
    extra.runGit("commit", "-m", "reviewed recovery after extra commit");
    expect(() =>
      releaseDeployTestHooks.assertSpecificationRecoveryFence(
        context(execFileSync, { repo: extra.root }),
        {
          releaseCommit: extra.releaseCommit,
          recoveryCommit: extra.runGit("rev-parse", "HEAD"),
          label: "Specification extra recovery test",
        },
      ),
    ).toThrow("direct single-parent child of C2");

    const merge = createC2();
    merge.runGit("switch", "-c", "recovery-side");
    writeFileSync(join(merge.root, "scripts", "release-deploy.mjs"), "R\n");
    merge.runGit("add", ".");
    merge.runGit("commit", "-m", "reviewed recovery side");
    merge.runGit("switch", "main");
    merge.runGit("merge", "--no-ff", "-m", "merge recovery", "recovery-side");
    expect(() =>
      releaseDeployTestHooks.assertSpecificationRecoveryFence(
        context(execFileSync, { repo: merge.root }),
        {
          releaseCommit: merge.releaseCommit,
          recoveryCommit: merge.runGit("rev-parse", "HEAD"),
          label: "Specification merge recovery test",
        },
      ),
    ).toThrow("direct single-parent child of C2");
  });

  test("rejects a merge-parent C2 even when its resulting tree is evidence-only", () => {
    const fixture = createC2();
    fixture.runGit("switch", "-c", "merge-c2", fixture.sourceCommit);
    fixture.runGit("merge", "--no-ff", "--no-commit", "main");
    fixture.runGit("commit", "-m", "forbidden merge-parent C2");
    const mergeCommit = fixture.runGit("rev-parse", "HEAD");
    expect(() =>
      releaseDeployTestHooks.assertSpecificationC2Fence(
        context(execFileSync, { repo: fixture.root }),
        fixture.sourceCommit,
        mergeCommit,
      ),
    ).toThrow("direct single-parent evidence-only child");
  });

  test("publishes the exact C2 source asset through create-only tag and immutable readback", () => {
    const ownerGateTools = nominateOwnerGateTools();
    const fixture = createC2();
    const bare = temporaryDirectory("specification-origin");
    execFileSync("git", ["init", "--bare", "--initial-branch=main"], {
      cwd: bare,
    });
    fixture.runGit("remote", "add", "origin", bare);
    fixture.runGit("push", "origin", "main");

    const calls = [];
    let remoteTag = null;
    let draft = null;
    let uploadedPath = null;
    let uploadedBytes = null;
    let ownerChecks = 0;
    const ownerGateEnvironments = [];
    const releaseId = 41;
    const assetsURL = `https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/${releaseId}/assets`;
    const uploadURL = `https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/${releaseId}/assets{?name,label}`;
    const list = () =>
      JSON.stringify([
        draft === null
          ? []
          : [
              {
                id: releaseId,
                tag_name: "specification/1.1",
                draft: draft.draft,
              },
            ],
      ]);
    const fake = (executable, args, options = {}) => {
      calls.push({ executable, args: [...args], env: { ...options.env } });
      const ownerGateOutput = ownerGateToolOutput(
        ownerGateTools,
        executable,
        args,
      );
      if (ownerGateOutput !== null) return ownerGateOutput;
      if (executable === "bun") {
        if (args.join(" ") === "run check:release-owner-gate") {
          ownerChecks += 1;
          ownerGateEnvironments.push({ ...options.env });
        }
        return "";
      }
      if (executable === "gh" && args[0] === "--version") {
        return "gh version 2.96.0 (test)\n";
      }
      if (
        executable === "gh" &&
        args.join(" ") ===
          "api repos/tako0614/terraform-provider-takoform/immutable-releases"
      ) {
        return JSON.stringify({ enabled: true });
      }
      if (executable === "git") {
        if (args.join(" ") === "config --local -z --list") {
          return safeReleaseGitConfiguration();
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "ls-remote" && args.includes("--tags")) {
          return remoteTag === null
            ? ""
            : `${remoteTag.object}\trefs/tags/specification/1.1\n${remoteTag.commit}\trefs/tags/specification/1.1^{}\n`;
        }
        if (args[0] === "push") {
          const object = fixture.runGit(
            "rev-parse",
            "refs/tags/specification/1.1",
          );
          remoteTag = { object, commit: fixture.releaseCommit };
          return "";
        }
        return execFileSync("git", args, {
          ...options,
          cwd: fixture.root,
        });
      }
      if (executable !== "gh") {
        throw new Error(`unexpected executable ${executable}`);
      }
      if (isReleaseList(args)) return list();
      if (
        args[0] === "api" &&
        args.includes("--method") &&
        args[args.indexOf("--method") + 1] === "POST" &&
        args[3] === "repos/tako0614/terraform-provider-takoform/releases"
      ) {
        const value = (prefix) =>
          args.find((entry) => entry.startsWith(prefix)).slice(prefix.length);
        draft = {
          id: releaseId,
          tag_name: value("tag_name="),
          target_commitish: "main",
          name: value("name="),
          body: value("body="),
          draft: true,
          prerelease: false,
          immutable: false,
          published_at: null,
          assets_url: assetsURL,
          upload_url: uploadURL,
          html_url:
            "https://github.com/tako0614/terraform-provider-takoform/releases/tag/specification/1.1",
          assets: [],
        };
        return JSON.stringify(draft);
      }
      if (
        args[0] === "api" &&
        args.includes("--method") &&
        args[args.indexOf("--method") + 1] === "POST" &&
        args[3].startsWith("https://uploads.github.com/")
      ) {
        uploadedPath = args[args.indexOf("--input") + 1];
        uploadedBytes = readFileSync(uploadedPath);
        const name = decodeURIComponent(/\?name=(.+)$/u.exec(args[3])[1]);
        const asset = {
          id: 71,
          name,
          state: "uploaded",
          digest: sha256(uploadedBytes),
          size: uploadedBytes.length,
        };
        draft.assets.push(asset);
        return JSON.stringify(asset);
      }
      if (
        args[0] === "api" &&
        args.includes("--method") &&
        args[args.indexOf("--method") + 1] === "PATCH"
      ) {
        draft.draft = false;
        return JSON.stringify(draft);
      }
      if (
        args[0] === "api" &&
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/41"
      ) {
        return JSON.stringify(draft);
      }
      if (
        args[0] === "api" &&
        args[1] ===
          "repos/tako0614/terraform-provider-takoform/releases/tags/specification%2F1.1"
      ) {
        return JSON.stringify({ ...draft, immutable: !draft.draft });
      }
      if (args[0] === "release" && args[1] === "download") {
        const destination = args[args.indexOf("--dir") + 1];
        mkdirSync(destination, { recursive: true });
        writeFileSync(
          join(destination, "takoform-specification-1.1-source-snapshot.json"),
          uploadedBytes,
        );
        return "";
      }
      throw new Error(`unexpected gh ${args.join(" ")}`);
    };

    const io = memoryIO();
    const result = runReleaseSurface({
      surface: "takoform-specification-release",
      args: [
        "publish",
        "--tag",
        "specification/1.1",
        "--expected-commit",
        fixture.releaseCommit,
      ],
      repo: fixture.root,
      stdout: io.stdout,
      stderr: io.stderr,
      execFile: fake,
      wait: () => {},
    });
    expect(result).toMatchObject({
      surface: "takoform-specification-release",
      phase: "publish",
      sourceCommit: fixture.sourceCommit,
      releaseCommit: fixture.releaseCommit,
      tag: "specification/1.1",
      releaseId,
      productionReadback: "EXACT_IMMUTABLE_RELEASE",
      status: "PUBLISHED_AWAITING_C3_RECEIPT",
    });
    expect(result.assetDigests).toEqual({
      "takoform-specification-1.1-source-snapshot.json": expect.stringMatching(
        /^sha256:[0-9a-f]{64}$/,
      ),
    });
    expect(
      calls.some(
        ({ executable, args }) =>
          executable === "gh" &&
          args.join(" ") ===
            "api repos/tako0614/terraform-provider-takoform/immutable-releases",
      ),
    ).toBe(true);
    expect(JSON.stringify(result)).not.toContain("compatibility");
    expect(ownerChecks).toBeGreaterThanOrEqual(5);
    for (const environment of ownerGateEnvironments) {
      expect(environment.GH_TOKEN).toBeUndefined();
      expect(environment.GITHUB_TOKEN).toBeUndefined();
      expect(environment.GIT_NO_REPLACE_OBJECTS).toBe("1");
      expect(environment.GIT_CONFIG_GLOBAL).toBe("/dev/null");
      expect(environment.GIT_CONFIG_SYSTEM).toBe("/dev/null");
      expect(environment.GIT_OBJECT_DIRECTORY).toBeUndefined();
      expect(environment.GIT_ALTERNATE_OBJECT_DIRECTORIES).toBeUndefined();
      expect(environment.GIT_WORK_TREE).toBeUndefined();
    }
    expect(
      calls.filter(
        ({ executable, args }) =>
          executable === "gh" &&
          args.includes("--method") &&
          ["POST", "PATCH"].includes(args[args.indexOf("--method") + 1]),
      ).length,
    ).toBe(3);
    expect(io.errors).toBe("");

    // Exercise both explicit recovery entrypoints against their exact
    // authoritative partial states. The fake remote is reset directly; the
    // production code itself never issues DELETE or rewrites the tag.
    draft = null;
    const recoveredTagOnly = runReleaseSurface({
      surface: "takoform-specification-release",
      args: [
        "recover-tag-only",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        fixture.releaseCommit,
        "--expected-tag-object",
        result.tagObject,
        "--expected-recovery-commit",
        fixture.releaseCommit,
      ],
      repo: fixture.root,
      stdout: io.stdout,
      stderr: io.stderr,
      execFile: fake,
      wait: () => {},
    });
    expect(recoveredTagOnly).toMatchObject({
      phase: "recover-tag-only",
      recoveredFrom: "EXACT_ANNOTATED_TAG_PRESENT_RELEASE_ABSENT",
      status: "PUBLISHED_AWAITING_C3_RECEIPT",
    });

    draft.draft = true;
    draft.immutable = false;
    draft.published_at = null;
    const recoveredDraft = runReleaseSurface({
      surface: "takoform-specification-release",
      args: [
        "recover-draft",
        "--tag",
        "specification/1.1",
        "--expected-release-commit",
        fixture.releaseCommit,
        "--expected-tag-object",
        result.tagObject,
        "--expected-recovery-commit",
        fixture.releaseCommit,
        "--release-id",
        String(releaseId),
      ],
      repo: fixture.root,
      stdout: io.stdout,
      stderr: io.stderr,
      execFile: fake,
      wait: () => {},
    });
    expect(recoveredDraft).toMatchObject({
      phase: "recover-draft",
      recoveredFrom: "EXACT_RETAINED_DRAFT",
      status: "PUBLISHED_AWAITING_C3_RECEIPT",
    });
    expect(
      calls.some(
        ({ executable, args }) =>
          executable === "gh" &&
          (args.includes("DELETE") || args.includes("--method=DELETE")),
      ),
    ).toBe(false);

    const liveArgs = [
      "--tag",
      "specification/1.1",
      "--expected-release-commit",
      fixture.releaseCommit,
      "--expected-tag-object",
      result.tagObject,
      "--release-id",
      String(releaseId),
    ];
    const correctDigest = draft.assets[0].digest;
    draft.assets[0].digest = `sha256:${"0".repeat(64)}`;
    expect(() =>
      runReleaseSurface({
        surface: "takoform-specification-release",
        args: ["verify", ...liveArgs],
        repo: fixture.root,
        stdout: io.stdout,
        stderr: io.stderr,
        execFile: fake,
        wait: () => {},
      }),
    ).toThrow("published asset mismatch");
    draft.assets[0].digest = correctDigest;

    const verified = runReleaseSurface({
      surface: "takoform-specification-release",
      args: ["verify", ...liveArgs],
      repo: fixture.root,
      stdout: io.stdout,
      stderr: io.stderr,
      execFile: fake,
      wait: () => {},
    });
    expect(verified.status).toBe("VERIFIED");

    writeFileSync(
      join(fixture.root, "scripts", "release-deploy.mjs"),
      "reviewed retained-draft recovery repair\n",
    );
    fixture.runGit("add", "scripts/release-deploy.mjs");
    fixture.runGit("commit", "-m", "repair Specification release recovery");
    const recoveryCommit = fixture.runGit("rev-parse", "HEAD");
    fixture.runGit("push", "origin", "main");
    const recorded = runReleaseSurface({
      surface: "takoform-specification-release",
      args: [
        "record-receipt",
        ...liveArgs,
        "--expected-recovery-commit",
        recoveryCommit,
      ],
      repo: fixture.root,
      stdout: io.stdout,
      stderr: io.stderr,
      execFile: fake,
      wait: () => {},
    });
    expect(recorded).toMatchObject({
      phase: "record-receipt",
      recoveryCommit,
      receiptCount: 1,
      mutation: "C3_LEDGER_PROJECTIONS_ONLY",
      status: "RECEIPT_WRITTEN_AWAITING_C3_AND_C4_COMMITS",
    });
    const receiptLedger = JSON.parse(
      readFileSync(
        join(fixture.root, "release", "specification-releases.json"),
        "utf8",
      ),
    );
    expect(receiptLedger.releases[0]).toMatchObject({
      sourceCommit: fixture.sourceCommit,
      releaseCommit: fixture.releaseCommit,
      tag: "specification/1.1",
      tagObject: result.tagObject,
      release: { id: releaseId, immutable: true },
    });
    expect(JSON.stringify(receiptLedger.releases[0])).not.toContain(
      "compatibility",
    );
    expect(receiptLedger.releases[0]).not.toHaveProperty("recoveryCommit");
    const uploadCalls = calls.filter(
      ({ executable, args }) =>
        executable === "gh" &&
        args[0] === "api" &&
        args.includes("POST") &&
        args.some((argument) => argument.startsWith("https://uploads.github.com/")),
    );
    expect(uploadCalls.length).toBeGreaterThanOrEqual(2);
    for (const upload of uploadCalls) {
      expect(upload.env.GH_TOKEN).toBe("operator-only-test-token");
      expect(upload.env.GITHUB_TOKEN).toBeUndefined();
      expect(upload.env.GH_ENTERPRISE_TOKEN).toBeUndefined();
      expect(upload.env.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
      expect(upload.env.GH_HOST).toBe("github.com");
      expect(upload.env.GH_CONFIG_DIR).toBeTruthy();
      expect(upload.args).not.toContain("operator-only-test-token");
    }
    expect(
      fixture
        .runGit("status", "--porcelain=v1")
        .split("\n")
        .filter(Boolean)
        .map((line) => line.trim().split(/\s+/u).at(-1))
        .sort(),
    ).toEqual([
      "release/specification-releases.json",
      "website/public/release/specification-releases.json",
      "website/static/release/specification-releases.json",
    ]);
  });
});

describe("workflow dispatch authority and correlation", () => {
  test("pins ordinary GitHub CLI authority to github.com and an isolated config", () => {
    const names = [
      "GH_CONFIG_DIR",
      "GH_HOST",
      "GH_REPO",
      "GITHUB_API_URL",
      "GITHUB_SERVER_URL",
      "HTTPS_PROXY",
      "NODE_OPTIONS",
    ];
    const previous = Object.fromEntries(
      names.map((name) => [name, process.env[name]]),
    );
    process.env.GH_CONFIG_DIR = "/tmp/attacker-gh-config";
    process.env.GH_HOST = "example.invalid";
    process.env.GH_REPO = "attacker/repository";
    process.env.GITHUB_API_URL = "https://example.invalid/api/v3";
    process.env.GITHUB_SERVER_URL = "https://example.invalid";
    process.env.HTTPS_PROXY = "https://example.invalid";
    process.env.NODE_OPTIONS = "--require=/tmp/attacker.cjs";
    try {
      const execution = context(() => "");
      const environment =
        releaseDeployTestHooks.githubCommandEnvironment(execution);
      expect(environment.GH_HOST).toBe("github.com");
      expect(environment.GH_CONFIG_DIR).toBe(execution.githubConfigDirectory);
      expect(environment.GH_TOKEN).toBe("operator-only-test-token");
      expect(environment.GH_PROMPT_DISABLED).toBe("1");
      expect(environment.GH_NO_UPDATE_NOTIFIER).toBe("1");
      expect(environment.GH_REPO).toBeUndefined();
      expect(environment.GITHUB_API_URL).toBeUndefined();
      expect(environment.GITHUB_SERVER_URL).toBeUndefined();
      expect(environment.HTTPS_PROXY).toBeUndefined();
      expect(environment.NODE_OPTIONS).toBeUndefined();
      expect(environment.GIT_CONFIG_GLOBAL).toBe("/dev/null");
      expect(environment.GIT_CONFIG_SYSTEM).toBe("/dev/null");
    } finally {
      for (const name of names) {
        if (previous[name] === undefined) delete process.env[name];
        else process.env[name] = previous[name];
      }
    }
  });

  test("scopes upload authority to the absolute uploads host without argv exposure", () => {
    process.env.GH_ENTERPRISE_TOKEN = "ambient-enterprise-token";
    process.env.GITHUB_ENTERPRISE_TOKEN = "ambient-enterprise-token";
    const execution = context(() => "");
    const environment =
      releaseDeployTestHooks.githubUploadEnvironment(execution);
    expect(environment.GH_TOKEN).toBe("operator-only-test-token");
    expect(environment.GITHUB_TOKEN).toBeUndefined();
    expect(environment.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
    expect(environment.GH_ENTERPRISE_TOKEN).toBeUndefined();
    expect(environment.GH_HOST).toBe("github.com");
    expect(environment.GH_CONFIG_DIR).toBe(execution.githubConfigDirectory);
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
      url: "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123",
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
      url: `https://github.com/tako0614/terraform-provider-takoform/actions/runs/${id}`,
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

  test("accepts only the canonical HTTPS origin spellings", () => {
    for (const origin of [
      "https://github.com/tako0614/terraform-provider-takoform.git",
      "https://github.com/tako0614/terraform-provider-takoform",
    ]) {
      expect(releaseDeployTestHooks.isCanonicalOrigin(origin)).toBe(true);
    }
    for (const origin of [
      "http://github.com/tako0614/terraform-provider-takoform.git",
      "git@github.com:tako0614/terraform-provider-takoform.git",
      "ssh://git@github.com/tako0614/terraform-provider-takoform.git",
      "https://github.com/tako0614/terraform-provider-takoform/",
      "https://github.com/attacker/terraform-provider-takoform.git",
    ]) {
      expect(releaseDeployTestHooks.isCanonicalOrigin(origin)).toBe(false);
    }
  });

  test("runs the complete owner gate with pinned Go and managed-home authority", () => {
    const names = [
      "BUN_CONFIG_FILE",
      "CGO_CFLAGS",
      "GH_HOST",
      "GNUPGHOME",
      "GPG_AGENT_INFO",
      "GPG_TTY",
      "GOENV",
      "GOFLAGS",
      "GOCACHEPROG",
      "GOPROXY",
      "GOTOOLCHAIN",
      "GOWORK",
      "HOME",
      "NODE_OPTIONS",
      "PATH",
      "SSH_ASKPASS",
      "SSH_ASKPASS_REQUIRE",
      "SSH_AUTH_SOCK",
      "SSH_AGENT_PID",
      "TF_CLI_ARGS",
      "TMPDIR",
      "TOFU_CLI_ARGS",
    ];
    const previous = Object.fromEntries(
      names.map((name) => [name, process.env[name]]),
    );
    let environment;
    let ownerGateTools;
    const execution = context((executable, args, options) => {
      const ownerGateOutput = ownerGateToolOutput(
        ownerGateTools,
        executable,
        args,
      );
      if (ownerGateOutput !== null) return ownerGateOutput;
      expect(executable).toBe("bun");
      expect(args).toEqual(["run", "check:release-owner-gate"]);
      environment = options.env;
      return "";
    });
    process.env.BUN_CONFIG_FILE = "/tmp/attacker-bun.toml";
    process.env.CGO_CFLAGS = "-include /tmp/attacker.h";
    process.env.GH_HOST = "example.invalid";
    process.env.GNUPGHOME = "/tmp/attacker-gnupg";
    process.env.GPG_AGENT_INFO = "/tmp/attacker-agent";
    process.env.GPG_TTY = "/dev/attacker";
    process.env.GOENV = "/tmp/attacker-goenv";
    process.env.GOFLAGS = "-run=never";
    process.env.GOCACHEPROG = "/tmp/attacker-cache-program";
    process.env.GOPROXY = "https://example.invalid";
    process.env.GOTOOLCHAIN = "path";
    process.env.GOWORK = "/tmp/attacker.work";
    process.env.HOME = "/tmp/attacker-home";
    process.env.NODE_OPTIONS = "--require=/tmp/attacker.cjs";
    process.env.SSH_ASKPASS = "/tmp/attacker-askpass";
    process.env.SSH_ASKPASS_REQUIRE = "force";
    process.env.SSH_AUTH_SOCK = "/tmp/attacker-agent.sock";
    process.env.SSH_AGENT_PID = "12345";
    ownerGateTools = nominateOwnerGateTools();
    process.env.TF_CLI_ARGS = "-plugin-dir=/tmp/attacker";
    process.env.TMPDIR = "/tmp/attacker-tmp";
    process.env.TOFU_CLI_ARGS = "-plugin-dir=/tmp/attacker";
    try {
      releaseDeployTestHooks.runOwnerCheck(execution);
      expect(environment.CGO_ENABLED).toBe("0");
      expect(environment.GOENV).toBe("off");
      expect(environment.GOFLAGS).toBe("-mod=readonly -buildvcs=false");
      expect(environment.GOCACHEPROG).toBeUndefined();
      expect(environment.GOCACHE).toBe(join(environment.HOME, "m", "go-build"));
      expect(environment.GOMODCACHE).toBe(
        join(environment.HOME, "m", "go-mod"),
      );
      expect(environment.GOPATH).toBe(join(environment.HOME, "m", "go-path"));
      expect(
        new Set([
          environment.GOCACHE,
          environment.GOMODCACHE,
          environment.GOPATH,
          environment.TMPDIR,
        ]).size,
      ).toBe(4);
      for (const path of [
        environment.GOCACHE,
        environment.GOMODCACHE,
        environment.GOPATH,
        environment.TMPDIR,
      ]) {
        expect(path.startsWith(join(environment.HOME, "m") + "/")).toBe(true);
      }
      expect(environment.GOPROXY).toBe("https://proxy.golang.org");
      expect(environment.GOTOOLCHAIN).toBe("local");
      expect(environment.GOWORK).toBe("off");
      expect(environment.HOME).not.toBe("/tmp/attacker-home");
      expect(environment.HOME.startsWith("/")).toBe(true);
      expect(environment.TMPDIR).toBe(join(environment.HOME, "m", "t"));
      expect(environment.TMP).toBe(environment.TMPDIR);
      expect(environment.TEMP).toBe(environment.TMPDIR);
      expect(environment.PATH).not.toContain("attacker-bin");
      expect(environment.PATH).not.toContain(ownerGateTools.terraformBin);
      expect(environment.BUN_CONFIG_FILE).toBeUndefined();
      expect(environment.CGO_CFLAGS).toBeUndefined();
      expect(environment.GH_HOST).toBeUndefined();
      expect(environment.GH_TOKEN).toBeUndefined();
      expect(environment.GITHUB_TOKEN).toBeUndefined();
      expect(environment.GNUPGHOME).toBeUndefined();
      expect(environment.GPG_AGENT_INFO).toBeUndefined();
      expect(environment.GPG_TTY).toBeUndefined();
      expect(environment.NODE_OPTIONS).toBeUndefined();
      expect(environment.SSH_ASKPASS).toBeUndefined();
      expect(environment.SSH_ASKPASS_REQUIRE).toBeUndefined();
      expect(environment.SSH_AUTH_SOCK).toBeUndefined();
      expect(environment.SSH_AGENT_PID).toBeUndefined();
      expect(environment.TF_CLI_ARGS).toBeUndefined();
      expect(environment.TOFU_CLI_ARGS).toBeUndefined();
      expect(environment.GIT_NO_REPLACE_OBJECTS).toBe("1");
      expect(environment.GIT_CONFIG_GLOBAL).toBe("/dev/null");
    } finally {
      for (const name of names) {
        if (previous[name] === undefined) delete process.env[name];
        else process.env[name] = previous[name];
      }
    }
  });

  test("rejects descriptor and executable identity drift before Bun", () => {
    const descriptorRepo = trustedTemporaryDirectory("owner-gate-descriptor");
    mkdirSync(join(descriptorRepo, "release"), { recursive: true });
    writeFileSync(
      join(descriptorRepo, "release", "version.json"),
      JSON.stringify({
        cliMatrix: [{ product: "OpenTofu", version: "1.12.3" }],
        goVersion: "go1.26.5",
      }),
    );
    expect(() =>
      releaseDeployTestHooks.ownerGateToolchain(descriptorRepo),
    ).toThrow("exactly OpenTofu and Terraform");

    const cases = [
      [
        "missing OpenTofu version",
        { tofuJSON: "{}\n" },
        "OpenTofu version drift",
      ],
      [
        "wrong OpenTofu product",
        { tofuPlain: "Terraform v1.12.3" },
        "OpenTofu product/version drift",
      ],
      [
        "wrong Terraform version",
        { terraformJSON: '{"terraform_version":"1.15.7"}\n' },
        "Terraform version drift",
      ],
      [
        "wrong Go version",
        { goVersion: "go version go1.26.4 linux/amd64" },
        "Go version drift",
      ],
      [
        "wrong GOROOT",
        { goroot: "/safe-but-wrong/go-root" },
        "Go toolchain root drift",
      ],
      [
        "wrong GOTOOLDIR",
        { gotooldir: (root) => join(root, "pkg", "tool", "wrong") },
        "Go toolchain GOTOOLDIR drift",
      ],
      [
        "escaped GOTOOLDIR",
        { gotooldir: "/tmp/owner-gate-escaped-tool-dir" },
        "Go toolchain GOTOOLDIR escaped managed pkg/tool",
      ],
      [
        "mismatched gofmt version",
        {
          gofmtBuild: (path) => `${path}: go1.26.4\n\tpath\tcmd/gofmt\n`,
        },
        "gofmt build identity drift",
      ],
      [
        "mismatched gofmt package",
        {
          gofmtBuild: (path) => `${path}: go1.26.5\n\tpath\tcmd/go\n`,
        },
        "gofmt build identity drift",
      ],
    ];
    for (const [, overrides, message] of cases) {
      const ownerGateTools = nominateOwnerGateTools();
      let bunCalls = 0;
      const execution = context((executable, args) => {
        const output = ownerGateToolOutput(
          ownerGateTools,
          executable,
          args,
          overrides,
        );
        if (output !== null) return output;
        if (executable === "bun") {
          bunCalls += 1;
          return "";
        }
        throw new Error(`unexpected owner gate command: ${executable}`);
      });
      expect(() => releaseDeployTestHooks.runOwnerCheck(execution)).toThrow(
        message,
      );
      expect(bunCalls).toBe(0);
    }
  });

  test("rejects managed tool closure mutation performed by the owner gate", () => {
    const ownerGateTools = nominateOwnerGateTools();
    const execution = context((executable, args, options) => {
      const output = ownerGateToolOutput(ownerGateTools, executable, args);
      if (output !== null) return output;
      if (executable === "bun") {
        writeFileSync(join(options.env.PATH.split(":")[1], "extra"), "x");
        return "";
      }
      throw new Error(`unexpected owner gate command: ${executable}`);
    });
    expect(() => releaseDeployTestHooks.runOwnerCheck(execution)).toThrow(
      "managed tool-bin closure changed",
    );
  });

  test("rejects managed Go compiler, source, and include mutations during warmup", () => {
    const targets = [
      [
        "compiler",
        (root) => join(root, "pkg", "tool", "linux_amd64", "compile"),
        0o500,
      ],
      ["source", (root) => join(root, "src", "runtime", "runtime.go"), 0o400],
      ["include", (root) => join(root, "pkg", "include", "textflag.h"), 0o400],
    ];
    for (const [, target, mode] of targets) {
      const ownerGateTools = nominateOwnerGateTools();
      let bunCalls = 0;
      let mutated = false;
      const execution = context((executable, args, options) => {
        const output = ownerGateToolOutput(ownerGateTools, executable, args);
        if (output !== null) {
          if (!mutated && args.join(" ") === "test -run ^$ ./...") {
            replaceManagedReadOnlyFile(
              target(options.env.GOROOT),
              "mutated\n",
              mode,
            );
            mutated = true;
          }
          return output;
        }
        if (executable === "bun") {
          bunCalls += 1;
          return "";
        }
        throw new Error(`unexpected owner gate command: ${executable}`);
      });
      expect(() => releaseDeployTestHooks.runOwnerCheck(execution)).toThrow(
        "managed GOROOT",
      );
      expect(mutated).toBe(true);
      expect(bunCalls).toBe(0);
    }
  });

  test("rejects mutable state topology mutation during warmup", () => {
    const ownerGateTools = nominateOwnerGateTools();
    let bunCalls = 0;
    let mutated = false;
    const execution = context((executable, args, options) => {
      const output = ownerGateToolOutput(ownerGateTools, executable, args);
      if (output !== null) {
        if (!mutated && args.join(" ") === "test -run ^$ ./...") {
          rmSync(options.env.GOCACHE, { recursive: true, force: true });
          mutated = true;
        }
        return output;
      }
      if (executable === "bun") {
        bunCalls += 1;
        return "";
      }
      throw new Error(`unexpected owner gate command: ${executable}`);
    });
    expect(() => releaseDeployTestHooks.runOwnerCheck(execution)).toThrow(
      "owner gate mutable gocache",
    );
    expect(mutated).toBe(true);
    expect(bunCalls).toBe(0);
  });

  test("rejects managed GOROOT mutation performed during Bun before returning", () => {
    const ownerGateTools = nominateOwnerGateTools();
    let bunCalls = 0;
    let mutated = false;
    const execution = context((executable, args, options) => {
      const output = ownerGateToolOutput(ownerGateTools, executable, args);
      if (output !== null) return output;
      if (executable === "bun") {
        bunCalls += 1;
        replaceManagedReadOnlyFile(
          join(options.env.GOROOT, "pkg", "tool", "linux_amd64", "compile"),
          "mutated\n",
          0o500,
        );
        mutated = true;
        return "";
      }
      throw new Error(`unexpected owner gate command: ${executable}`);
    });
    expect(() => releaseDeployTestHooks.runOwnerCheck(execution)).toThrow(
      "managed GOROOT",
    );
    expect(mutated).toBe(true);
    expect(bunCalls).toBe(1);
  });

  test("blocks mutation when origin/main advances after the owner check", () => {
    const ownerGateTools = nominateOwnerGateTools();
    const calls = [];
    const advanced = "89abcdef0123456789abcdef0123456789abcdef";
    let ownerChecks = 0;
    let remoteMainReads = 0;
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      const ownerGateOutput = ownerGateToolOutput(
        ownerGateTools,
        executable,
        args,
      );
      if (ownerGateOutput !== null) return ownerGateOutput;
      const version = toolOutput(executable, args);
      if (version !== null) return version;
      if (executable === "bun") {
        ownerChecks += 1;
        return "";
      }
      if (executable === "git") {
        if (args.join(" ") === "config --local -z --list") {
          return safeReleaseGitConfiguration();
        }
        if (
          args.join(" ") === "rev-parse --path-format=absolute --git-common-dir"
        ) {
          return `${join(repositoryRoot, ".git")}\n`;
        }
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
          remoteMainReads += 1;
          return `${remoteMainReads === 1 ? commit : advanced}\n`;
        }
        if (args[0] === "cat-file") return "";
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    expect(() =>
      releaseDeployTestHooks.ownerGateAndFence(context(fake), commit),
    ).toThrow("is not fresh origin/main");
    expect(ownerChecks).toBe(1);
    expect(remoteMainReads).toBe(2);
    expect(
      calls.some(
        (call) =>
          call.executable === "gh" &&
          ["api", "workflow", "release"].includes(call.args[0]),
      ),
    ).toBe(false);
    expect(
      calls.some(
        (call) => call.executable === "git" && call.args[0] === "push",
      ),
    ).toBe(false);
  });

  test("refuses under mutable-release policy before any mutation", () => {
    const ownerGateTools = nominateOwnerGateTools();
    const calls = [];
    let ownerChecks = 0;
    let immutabilityReads = 0;
    const fake = (executable, args) => {
      calls.push({ executable, args: [...args] });
      const ownerGateOutput = ownerGateToolOutput(
        ownerGateTools,
        executable,
        args,
      );
      if (ownerGateOutput !== null) return ownerGateOutput;
      const version = toolOutput(executable, args);
      if (version !== null) return version;
      if (executable === "bun") {
        ownerChecks += 1;
        return "";
      }
      if (
        executable === "gh" &&
        args.join(" ") ===
          "api repos/tako0614/terraform-provider-takoform/immutable-releases"
      ) {
        immutabilityReads += 1;
        return JSON.stringify({ enabled: false });
      }
      if (executable === "git") {
        if (args.join(" ") === "config --local -z --list") {
          return safeReleaseGitConfiguration();
        }
        if (
          args.join(" ") === "rev-parse --path-format=absolute --git-common-dir"
        ) {
          return `${join(repositoryRoot, ".git")}\n`;
        }
        if (args[0] === "status") return "";
        if (args.join(" ") === "rev-parse --is-shallow-repository") {
          return "false\n";
        }
        if (args.join(" ") === "remote get-url origin") {
          return "https://github.com/tako0614/terraform-provider-takoform.git\n";
        }
        if (args[0] === "symbolic-ref") return "main\n";
        if (args[0] === "fetch") return "";
        if (args[0] === "rev-parse") return `${commit}\n`;
        if (args[0] === "cat-file") return "";
      }
      throw new Error(`unexpected ${executable} ${args.join(" ")}`);
    };
    // The provider lane shares one gate with the Specification lane, so the
    // repository immutable-release setting is proved before prepare dispatches
    // a workflow, before a tag is pushed, and before a draft is created.
    expect(() =>
      releaseDeployTestHooks.ownerGateAndFence(context(fake), commit),
    ).toThrow("immutable releases to be enabled");
    expect(ownerChecks).toBe(1);
    expect(immutabilityReads).toBe(1);
    expect(
      calls.some(
        (call) =>
          (call.executable === "git" &&
            ["push", "tag"].includes(call.args[0])) ||
          (call.executable === "gh" &&
            (call.args.includes("POST") ||
              call.args.includes("PATCH") ||
              call.args.includes("DELETE") ||
              call.args[0] === "workflow")),
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

test("deep revocation semantic verifier failures block publication", () => {
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
  const repo = temporaryDirectory("release-top-level-revocation");
  const tag = "forms/revocations/v1.0.0";
  mkdirSync(join(repo, "forms", "revocations", "checkpoints"), {
    recursive: true,
  });
  writeFileSync(join(repo, "forms", "revocations", "1.0.0.json"), "{}\n");
  writeFileSync(
    join(repo, "forms", "revocations", "checkpoints", "1.0.0.json"),
    "{}\n",
  );
  const calls = [];
  const io = memoryIO();
  const fake = (executable, args) => {
    calls.push({ executable, args: [...args] });
    if (executable === "git") {
      if (args.join(" ") === "config --local -z --list") {
        return safeReleaseGitConfiguration();
      }
      if (
        args.join(" ") === "rev-parse --path-format=absolute --git-common-dir"
      ) {
        return `${join(repo, ".git")}\n`;
      }
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
        args[1] === `${commit}:release/trust/trusted-root.json`
      ) {
        return "{}\n";
      }
      if (
        [
          "cat-file",
          "merge-base",
          "diff",
          "for-each-ref",
          "ls-remote",
        ].includes(args[0])
      ) {
        return "";
      }
    }
    if (executable === "gh" && isReleaseList(args)) return "[[]]";
    if (executable === "gh" && args[0] === "run" && args[1] === "view") {
      return JSON.stringify({
        databaseId: 123,
        attempt: 1,
        workflowName: "Prepare signed Form Package revocation checkpoint",
        event: "workflow_dispatch",
        headBranch: "main",
        headSha: commit,
        status: "completed",
        conclusion: "success",
        displayTitle: requestId,
        url: "https://github.com/tako0614/terraform-provider-takoform/actions/runs/123/attempts/1",
      });
    }
    if (executable === "gh" && args[0] === "run" && args[1] === "download") {
      writeDeepFailureCandidate(args[args.indexOf("--dir") + 1], {
        tag,
        runId: "123",
        runAttempt: "1",
        sourceCommit: commit,
        toolingCommit: commit,
      });
      return "";
    }
    if (executable === "go" && args.includes("verify-revocation-directory")) {
      return JSON.stringify({
        format: "takoform.form-package-revocation-directory-verification@v1",
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
      });
    }
    throw new Error(`unexpected ${executable} ${args.join(" ")}`);
  };
  expect(() =>
    runReleaseSurface({
      surface: "takoform-form-package-release",
      args: [
        "publish-revocation",
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
  ).toThrow("revocation deep semantic report identity mismatch");
  expect(
    calls.some(
      (call) =>
        call.executable === "go" &&
        call.args.includes("verify-revocation-directory"),
    ),
  ).toBe(true);
  expect(
    calls.some((call) => call.executable === "git" && call.args[0] === "push"),
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
});

test("top-level public verify cannot emit VERIFIED after deep semantic rejection", () => {
  const repo = temporaryDirectory("release-public-verify-revocation");
  const tag = "forms/revocations/v1.0.0";
  const fixture = join(repo, "fixture");
  mkdirSync(fixture);
  writeDeepFailureCandidate(fixture, {
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
      if (args.join(" ") === "config --local -z --list") {
        return safeReleaseGitConfiguration();
      }
      if (
        args.join(" ") === "rev-parse --path-format=absolute --git-common-dir"
      ) {
        return `${join(repo, ".git")}\n`;
      }
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
        args[1] === `${commit}:release/trust/trusted-root.json`
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
    if (executable === "go" && args.includes("verify-revocation-directory")) {
      return JSON.stringify({
        format: "takoform.form-package-revocation-directory-verification@v1",
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
      });
    }
    throw new Error(`unexpected ${executable} ${args.join(" ")}`);
  };
  expect(() =>
    runReleaseSurface({
      surface: "takoform-form-package-release",
      args: ["verify-revocation", "--tag", tag, "--expected-commit", commit],
      repo,
      stdout: io.stdout,
      stderr: io.stderr,
      execFile: fake,
      wait: () => {},
    }),
  ).toThrow("revocation deep semantic report identity mismatch");
  expect(
    calls.some(
      (call) =>
        call.executable === "go" &&
        call.args.includes("verify-revocation-directory"),
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
});

describe("deterministic revocation tag objects", () => {
  const metadata = {
    tag: "forms/revocations/v1.0.0",
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
    const root = temporaryDirectory("revocation-tag-object");
    const execution = context(fake);
    const exact = releaseDeployTestHooks.expectedFormTagObject(execution, {
      ...metadata,
      runId: "456",
      runAttempt: "3",
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
        metadata,
        "456",
        "3",
      ),
    ).not.toThrow();

    for (const changed of [
      exact.replace("type commit\n", "type commit\nobject deadbeef\n"),
      exact.replace("Takoform Form Package Revocation", "Wrong Tagger"),
      exact.replace(
        `Takoform Form Package revocation checkpoint ${metadata.tag}`,
        "Wrong title",
      ),
      exact.replace(
        `source-commit: ${commit}`,
        `source-commit: ${"f".repeat(40)}`,
      ),
      `${exact}extra-message: forbidden\n`,
    ]) {
      writeFileSync(join(root, "tag-object"), changed);
      expect(() =>
        releaseDeployTestHooks.verifyTagObjectWorkflowBinding(
          execution,
          root,
          metadata,
          "456",
          "3",
        ),
      ).toThrow("exact deterministic workflow object");
    }
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
    const workflowRef = `tako0614/terraform-provider-takoform/.github/workflows/release.yml@refs/tags/${descriptor.tag}`;
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
          buildType: "https://takoform.com/buildtypes/provider-release/v1",
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
              uri: `git+https://${descriptor.sourceRepository}@${commit}`,
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
    writeFileSync(
      join(root, names.provenanceSignature),
      "detached gpg signature\n",
    );
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
        signerFingerprint: "3510E75E05BBCC303B92D77934FC18AC897FB709",
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
      signerFingerprint: "3510E75E05BBCC303B92D77934FC18AC897FB709",
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
        (statement.predicate.buildDefinition.externalParameters.tag = "v1.0.1"),
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
        (statement.predicate.buildDefinition.internalParameters.run.id = "124"),
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
        releaseDeployTestHooks.assertReleaseAbsent(context(fake), "v1.0.0"),
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
            : JSON.stringify([[{ id: 7, tag_name: tag, draft: true }]]);
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
          args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
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

  test("strict publication sends full exact PATCH identity and halts on a drifted response", () => {
    const fixture = assetFixture();
    const tag = "v1.0.1";
    const body = "exact provider release";
    const calls = [];
    let listCalls = 0;
    let remoteAssets = [];
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
      assets: remoteAssets,
    });
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([[{ id: 7, tag_name: tag, draft: true }]]);
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
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
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
      }),
    ).toThrow("PATCH response differs");
    const patch = calls.find((args) => args.includes("PATCH"));
    expect(patch).toContain(`tag_name=${tag}`);
    expect(patch).toContain("target_commitish=main");
    expect(patch).toContain(`name=${tag}`);
    expect(patch).toContain(`body=${body}`);
    expect(patch).toContain("prerelease=false");
    expect(patch).toContain("draft=false");
    expect(patch).toContain("make_latest=false");
    expect(
      calls.some(
        (args) => args[0] === "api" && args[1]?.includes("/releases/tags/"),
      ),
    ).toBe(false);
    expect(calls.some((args) => args[0] === "release")).toBe(false);
  });

  test("lost POST response reports the explicit surface and retains the exact visible draft", () => {
    const fixture = assetFixture();
    const tag = "specification/1.1";
    const surface = "takoform-specification-release";
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
                  tag_name: tag,
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
        tag,
        surface,
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("gh api");
    expect(listCalls).toBe(2);
    expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
    expect(execution.io.errors).toContain("MATCHING_DRAFT_RETAINED");
    expect(execution.io.errors).toContain(`"surface":"${surface}"`);
    expect(execution.io.errors).toContain('"observedReleaseIDs":[7]');
    expect(() =>
      releaseDeployTestHooks.publishReleaseLocally(execution, {
        tag,
        surface,
        assets: fixture.assets,
        body: "exact release",
        temporaryRoot: fixture.root,
      }),
    ).toThrow("already exist");
    expect(calls.filter((args) => args.includes("POST")).length).toBe(1);
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
    const uploadEnvironments = [];
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
    const fake = (_executable, args, options = {}) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        return JSON.stringify([[{ id: 7, tag_name: tag, draft: !published }]]);
      }
      if (
        args[0] === "api" &&
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify(draft());
      }
      if (
        args.includes("POST") &&
        args.some((argument) => argument.includes("/assets?name="))
      ) {
        uploadEnvironments.push({ ...options.env });
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
    expect(uploadEnvironments).toHaveLength(2);
    for (const environment of uploadEnvironments) {
      expect(environment.GH_TOKEN).toBe("operator-only-test-token");
      expect(environment.GITHUB_TOKEN).toBeUndefined();
      expect(environment.GH_ENTERPRISE_TOKEN).toBeUndefined();
      expect(environment.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
      expect(environment.GH_CONFIG_DIR).toBe(execution.githubConfigDirectory);
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
          return JSON.stringify([[{ id: 7, tag_name: tag, draft: true }]]);
        }
        if (
          args[0] === "api" &&
          args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
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
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
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
    let uploaded = [];
    const exactDraft = () => ({
      id: 7,
      tag_name: "v1.0.0",
      target_commitish: "main",
      name: "v1.0.0",
      body: "exact release",
      draft: true,
      prerelease: false,
      immutable: false,
      published_at: null,
      assets_url:
        "https://api.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets",
      upload_url:
        "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
      assets: uploaded,
    });
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
          ...exactDraft(),
          draft: false,
          immutable: true,
          published_at: "2026-01-01T00:00:00Z",
          html_url: "https://github.com/example/release",
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
        uploaded = [remoteAsset];
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
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        return JSON.stringify(exactDraft());
      }
      if (args.includes("PATCH")) {
        published = true;
        return JSON.stringify({ ...exactDraft(), draft: false });
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
      calls.some((args) => args[0] === "release" && args[1] === "upload"),
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
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
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
    let listCalls = 0;
    const exactDraft = () => ({
      id: 7,
      tag_name: "v1.0.0",
      target_commitish: "main",
      name: "v1.0.0",
      body: "exact release",
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
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([[{ id: 7, tag_name: "v1.0.0", draft: true }]]);
      }
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
          upload_url:
            "https://uploads.github.com/repos/tako0614/terraform-provider-takoform/releases/7/assets{?name,label}",
        });
      }
      if (
        args[0] === "api" &&
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        releaseReads += 1;
        return JSON.stringify({
          ...exactDraft(),
          draft: releaseReads <= 2,
          published_at: releaseReads <= 2 ? null : "2026-01-01T00:00:00Z",
          assets:
            releaseReads === 2
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
    ).toThrow("drifted asset");
    expect(calls.some((args) => args.includes("DELETE"))).toBe(false);
  });

  test("retains the exact POST-returned release still reread as a draft", () => {
    const fixture = assetFixture();
    const calls = [];
    let releaseReads = 0;
    let listCalls = 0;
    const exactDraft = () => ({
      id: 7,
      tag_name: "v1.0.0",
      target_commitish: "main",
      name: "v1.0.0",
      body: "exact release",
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
    const fake = (_executable, args) => {
      calls.push([...args]);
      if (isReleaseList(args)) {
        listCalls += 1;
        return listCalls === 1
          ? "[[]]"
          : JSON.stringify([[{ id: 7, tag_name: "v1.0.0", draft: true }]]);
      }
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
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
      ) {
        releaseReads += 1;
        return JSON.stringify({
          ...exactDraft(),
          assets:
            releaseReads === 1
              ? []
              : [
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
    ).toThrow("drifted asset");
    expect(releaseReads).toBe(3);
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
        args[1] === "repos/tako0614/terraform-provider-takoform/releases/7"
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
    expect(pushEnvironment.GIT_NO_REPLACE_OBJECTS).toBe("1");
    expect(pushEnvironment.GIT_CONFIG_GLOBAL).toBe("/dev/null");
    expect(pushEnvironment.GIT_CONFIG_SYSTEM).toBe("/dev/null");
    expect(pushEnvironment.GIT_OBJECT_DIRECTORY).toBeUndefined();
    expect(pushEnvironment.GIT_ALTERNATE_OBJECT_DIRECTORIES).toBeUndefined();
    expect(pushEnvironment.GIT_INDEX_FILE).toBeUndefined();
    expect(pushEnvironment.GIT_WORK_TREE).toBeUndefined();
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

  test("normal Git reads scrub ambient authority, object, worktree, command, and protocol overrides", () => {
    const names = [
      "GIT_ALTERNATE_OBJECT_DIRECTORIES",
      "GIT_CONFIG_COUNT",
      "GIT_CONFIG_KEY_0",
      "GIT_CONFIG_VALUE_0",
      "GIT_EXTERNAL_DIFF",
      "GIT_INDEX_FILE",
      "GIT_OBJECT_DIRECTORY",
      "GIT_PROXY_COMMAND",
      "GIT_REPLACE_REF_BASE",
      "GIT_SSH_COMMAND",
      "GIT_WORK_TREE",
    ];
    const previous = Object.fromEntries(
      names.map((name) => [name, process.env[name]]),
    );
    for (const name of names) process.env[name] = "/tmp/ambient-attacker";
    process.env.GIT_CONFIG_COUNT = "1";
    process.env.GIT_CONFIG_KEY_0 = "url.file:///tmp/attacker/.insteadOf";
    process.env.GIT_CONFIG_VALUE_0 = "https://github.com/";
    try {
      const environment = releaseDeployTestHooks.normalGitEnvironment();
      expect(environment.GH_TOKEN).toBeUndefined();
      expect(environment.GH_ENTERPRISE_TOKEN).toBeUndefined();
      expect(environment.GITHUB_TOKEN).toBeUndefined();
      expect(environment.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
      expect(environment.GIT_NO_REPLACE_OBJECTS).toBe("1");
      expect(environment.GIT_CONFIG_GLOBAL).toBe("/dev/null");
      expect(environment.GIT_CONFIG_SYSTEM).toBe("/dev/null");
      for (const name of names) {
        expect(environment[name]).toBeUndefined();
      }
    } finally {
      for (const name of names) {
        if (previous[name] === undefined) delete process.env[name];
        else process.env[name] = previous[name];
      }
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
      const publication = body.indexOf(`const release = ${publicationCall}(`);
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
        releaseDeployTestHooks.assertReleaseImmutabilityEnabled(context(fake)),
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
    const execute = (signer = "3510E75E05BBCC303B92D77934FC18AC897FB709") => {
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
    writeFileSync(globalConfig, `[gpg]\n\tprogram = ${maliciousGpg}\n`);
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
        (call) => call.executable === "git" && call.args.includes("verify-tag"),
      ),
    ).toBe(false);
    for (const call of calls.filter((entry) => entry.executable === "git")) {
      expect(call.env.GIT_NO_REPLACE_OBJECTS).toBe("1");
      expect(call.env.GIT_CONFIG_COUNT).toBeUndefined();
      expect(call.env.GIT_CONFIG_GLOBAL).toBe("/dev/null");
    }
  });

  test("pre-PATCH authority fence allows unrelated main advance and blocks revocation authority drift", () => {
    const root = temporaryDirectory("release-authority-fence");
    const runGit = (...args) =>
      execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
    runGit("init", "-b", "main");
    runGit("config", "user.name", "Takoform release test");
    runGit("config", "user.email", "release-test@example.invalid");
    mkdirSync(join(root, "scripts"), { recursive: true });
    mkdirSync(join(root, "forms", "revocations"), { recursive: true });
    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "authority\n");
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
        label: "revocation pre-publish fence",
      }),
    ).not.toThrow();

    writeFileSync(
      join(root, "forms", "revocations", "checkpoint.json"),
      '{"drift":true}\n',
    );
    runGit("add", "forms/revocations/checkpoint.json");
    runGit("commit", "-m", "revocation authority drift");
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: runGit("rev-parse", "HEAD"),
        label: "revocation pre-publish fence",
      }),
    ).toThrow("release authority paths changed");

    writeFileSync(join(root, "scripts", "release-deploy.mjs"), "drift\n");
    runGit("add", "scripts/release-deploy.mjs");
    runGit("commit", "-m", "tooling authority drift");
    expect(() =>
      releaseDeployTestHooks.assertFormReleaseAuthorityFence(execution, {
        sourceCommit: toolingCommit,
        toolingCommit,
        currentMain: runGit("rev-parse", "HEAD"),
        label: "revocation pre-publish fence",
      }),
    ).toThrow("release authority paths changed");
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

import {
  cpSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const sourceRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const tag = "v3.0.0";
const tagObject = "2c0f879b6e38d9995a4f5a4853a44a22c6aaa50a";
const sourceCommit = "a225cfa7c84aa551981cc8ad56c9a281fa6e051a";

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: sourceRoot,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
    ...options,
  });
  if (result.status !== 0) {
    const detail = `${result.stdout ?? ""}${result.stderr ?? ""}`.trim();
    throw new Error(`${command} ${args.join(" ")} failed${detail ? `:\n${detail}` : ""}`);
  }
  return result.stdout.trim();
}

const resolvedTag = run("git", ["rev-parse", `refs/tags/${tag}`]);
if (resolvedTag !== tagObject) {
  throw new Error(`${tag} resolves to ${resolvedTag}, want annotated tag object ${tagObject}`);
}
if (run("git", ["cat-file", "-t", tagObject]) !== "tag") {
  throw new Error(`${tagObject} is not an annotated Git tag object`);
}
const peeled = run("git", ["rev-parse", `${tag}^{}`]);
if (peeled !== sourceCommit) {
  throw new Error(`${tag} peels to ${peeled}, want immutable source ${sourceCommit}`);
}

function runGoTest(label, cwd, args) {
  const output = run("go", ["test", ...args], { cwd });
  process.stdout.write(`${label} OK${output ? `: ${output}\n` : "\n"}`);
}

// Exercise the current worktree first. This includes the release-evidence
// test (which intentionally reads the forward publication ledger) and keeps
// the provider diagnostics package in the focused release gate. Do not call
// the package script here: this script is itself the package entrypoint.
runGoTest("Provider 3 current characterization (including release evidence)", sourceRoot, [
  "./internal/provider",
  "-count=1",
]);
runGoTest("Provider diagnostics current", sourceRoot, [
  "./internal/providerdiagnostics",
  "-count=1",
]);

const temporaryRoot = mkdtempSync(join(tmpdir(), "takoform-provider3-baseline-"));
const extractedRoot = join(temporaryRoot, "source");
mkdirSync(extractedRoot);

try {
  const archive = spawnSync("git", ["archive", "--format=tar", sourceCommit], {
    cwd: sourceRoot,
    encoding: null,
    maxBuffer: 128 * 1024 * 1024,
  });
  if (archive.status !== 0) {
    throw new Error(`git archive failed: ${archive.stderr?.toString() ?? ""}`);
  }
  const extracted = spawnSync("tar", ["-xf", "-", "-C", extractedRoot], {
    input: archive.stdout,
    encoding: "utf8",
    maxBuffer: 128 * 1024 * 1024,
  });
  if (extracted.status !== 0) {
    throw new Error(`tar extraction failed: ${extracted.stderr ?? ""}`);
  }

  const providerRoot = join(sourceRoot, "internal", "provider");
  const baselineProviderRoot = join(extractedRoot, "internal", "provider");
  for (const name of readdirSync(providerRoot)) {
    // Release-evidence validation intentionally reads the later, forward-fixed
    // publication ledger. Every behavioral characterization test, however,
    // must compile and pass against the immutable release source itself.
    if (
      (/^v3_provider3_.*_test\.go$/u.test(name) && name !== "v3_provider3_release_evidence_test.go") ||
      /^v3_provider211_.*_test\.go$/u.test(name)
    ) {
      cpSync(join(providerRoot, name), join(baselineProviderRoot, name));
    }
  }
  const fixtureRoot = join(providerRoot, "testdata");
  const baselineFixtureRoot = join(baselineProviderRoot, "testdata");
  for (const name of readdirSync(fixtureRoot)) {
    if (/^v3-provider3-/u.test(name) || /^v3-provider211-/u.test(name)) {
      cpSync(join(fixtureRoot, name), join(baselineFixtureRoot, basename(name)));
    }
  }

  const checked = spawnSync(
    "go",
    ["test", "./internal/provider", "-count=1"],
    {
      cwd: extractedRoot,
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    },
  );
  if (checked.status !== 0) {
    throw new Error(
      `immutable ${tag}/${sourceCommit} characterization failed:\n${checked.stdout}${checked.stderr}`,
    );
  }
  process.stdout.write(
    `Provider 3 baseline OK: ${tagObject} -> ${sourceCommit}; copied characterization tests pass against the immutable source archive.\n`,
  );
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}

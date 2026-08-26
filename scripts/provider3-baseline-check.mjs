import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const sourceRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const tag = "v3.0.0";
const tagObject = "2c0f879b6e38d9995a4f5a4853a44a22c6aaa50a";
const sourceCommit = "a225cfa7c84aa551981cc8ad56c9a281fa6e051a";
// W02 added the complete Provider 3/retained-2.1.1 characterization after the
// immutable release was cut. This checkpoint is the last version of that
// harness written against the release's private registry seam. W08 may adapt
// the live tests to a new private seam, but it cannot rewrite this historical
// harness or the shared golden bytes it executes against.
const compatibilityHarnessCommit =
  "2fbc557265bc52ac2e046b9df0be2bfa3565c3d6";
const compatibilityHarnessTests = [
  "internal/provider/v3_provider211_codec_golden_test.go",
  "internal/provider/v3_provider211_import_golden_test.go",
  "internal/provider/v3_provider211_retained_golden_test.go",
  "internal/provider/v3_provider3_branch_golden_test.go",
  "internal/provider/v3_provider3_codec_golden_test.go",
  "internal/provider/v3_provider3_framework_golden_test.go",
  "internal/provider/v3_provider3_golden_test.go",
  "internal/provider/v3_provider3_import_golden_test.go",
  "internal/provider/v3_provider3_protocol_schema_golden_test.go",
];
const currentCompatibilityTestMarkers = new Map([
  [
    "internal/provider/v3_provider211_codec_golden_test.go",
    ["TestV3Provider211RetainedCodecsReadExactHostResponses"],
  ],
  [
    "internal/provider/v3_provider211_import_golden_test.go",
    ["TestV3Provider211RetainedCanonicalImportAdoptsHostExactRefs"],
  ],
  [
    "internal/provider/v3_provider211_retained_golden_test.go",
    ["TestV3Provider211RetainedGoldenLocksImmutableHistory"],
  ],
  [
    "internal/provider/v3_provider3_branch_golden_test.go",
    ["TestV3Provider3BranchGoldenLocksBehavior"],
  ],
  [
    "internal/provider/v3_provider3_codec_golden_test.go",
    ["TestV3Provider3CodecGoldenLocksAllResources"],
  ],
  [
    "internal/provider/v3_provider3_framework_golden_test.go",
    ["TestV3Provider3FrameworkBehaviorGoldenLocksAllResources"],
  ],
  [
    "internal/provider/v3_provider3_golden_test.go",
    [
      "TestV3Provider3GoldenLocksCurrentSurface",
      "TestV3Provider3GoldenRetainsProvider211History",
    ],
  ],
  [
    "internal/provider/v3_provider3_import_golden_test.go",
    ["TestV3Provider3GoldenLocksEveryCurrentImportPath"],
  ],
  [
    "internal/provider/v3_provider3_protocol_schema_golden_test.go",
    ["TestV3Provider3ProtocolSchemaMatchesPublishedBinary"],
  ],
]);
const compatibilityGoldenPaths = [
  "internal/provider/testdata/v3-provider211-retained-golden.json",
  "internal/provider/testdata/v3-provider3-branch-golden.json",
  "internal/provider/testdata/v3-provider3-codec-golden.json",
  "internal/provider/testdata/v3-provider3-framework-golden.json",
  "internal/provider/testdata/v3-provider3-golden.json",
  "internal/provider/testdata/v3-provider3-release-evidence.json",
  "internal/provider/testdata/v3-provider3-tofu-schema.json",
];

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

function readGitBlob(commit, path) {
  const result = spawnSync("git", ["show", `${commit}:${path}`], {
    cwd: sourceRoot,
    encoding: null,
    maxBuffer: 64 * 1024 * 1024,
  });
  if (result.status !== 0) {
    throw new Error(
      `cannot read immutable compatibility input ${commit}:${path}: ${result.stderr?.toString() ?? ""}`,
    );
  }
  return result.stdout;
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
for (const [path, tests] of currentCompatibilityTestMarkers) {
  const source = readFileSync(join(sourceRoot, path), "utf8");
  for (const test of tests) {
    if (!source.includes(`func ${test}(`)) {
      throw new Error(`current compatibility oracle is missing ${test} in ${path}`);
    }
  }
}

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

  // Execute the same immutable golden bytes from both sides of the private
  // seam change. The historical W02 harness compiles inside the v3.0.0 source
  // archive; the adapted live harness ran above. Checking every live golden
  // byte against the immutable checkpoint makes a coordinated current-code +
  // fixture rewrite fail before either suite starts.
  for (const path of compatibilityGoldenPaths) {
    const historical = readGitBlob(compatibilityHarnessCommit, path);
    const current = readFileSync(join(sourceRoot, path));
    if (!current.equals(historical)) {
      throw new Error(
        `immutable Provider compatibility golden changed: ${path}`,
      );
    }
    writeFileSync(join(extractedRoot, path), historical);
  }
  for (const path of compatibilityHarnessTests) {
    writeFileSync(
      join(extractedRoot, path),
      readGitBlob(compatibilityHarnessCommit, path),
    );
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
    `Provider 3 baseline OK: ${tagObject} -> ${sourceCommit}; immutable W02 state/codec/import/lifecycle/diagnostic/schema harness and adapted current harness pass against identical frozen goldens.\n`,
  );
} finally {
  rmSync(temporaryRoot, { recursive: true, force: true });
}

import { createHash, randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  accessSync,
  chmodSync,
  constants as fsConstants,
  existsSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { delimiter, isAbsolute, join, resolve } from "node:path";
import { tmpdir } from "node:os";
import process from "node:process";

const GITHUB_REPOSITORY = "tako0614/terraform-provider-takoform";
const SOURCE_REPOSITORY = `https://github.com/${GITHUB_REPOSITORY}.git`;
const PROVIDER_ADDRESS = "registry.terraform.io/tako0614/takoform";
const PROVIDER_SIGNER = "3510E75E05BBCC303B92D77934FC18AC897FB709";
const PROVIDER_VERSION = "2.1.1";
const PROVIDER_HOST_API = "forms.takoform.com/v1beta1";
const PROVIDER_FORM_FAMILY = "edge.forms.takoform.com/v1beta1";
const PROVIDER_IDENTITY_LEDGER = "release/provider-form-identities.json";
const PROVIDER_CANDIDATE_SET =
  "forms/candidates/edge/v1beta1/candidate-set.json";
const PROVIDER_FORM_KINDS = Object.freeze({
  takoform_module_worker: "ModuleWorker",
  takoform_worker_bundle: "WorkerBundle",
  takoform_static_asset_bundle: "StaticAssetBundle",
  takoform_worker_version: "WorkerVersion",
  takoform_worker_deployment: "WorkerDeployment",
  takoform_worker_custom_domain: "WorkerCustomDomain",
  takoform_worker_endpoint: "WorkerEndpoint",
  takoform_worker_cron_trigger: "WorkerCronTrigger",
  takoform_edge_kv_namespace: "EdgeKVNamespace",
  takoform_edge_object_bucket: "ObjectBucket",
  takoform_sqlite_database: "SQLiteDatabase",
  takoform_sqlite_migration_set: "SQLiteMigrationSet",
  takoform_sqlite_migration_application: "SQLiteMigrationApplication",
  takoform_at_least_once_queue: "AtLeastOnceQueue",
  takoform_queue_consumer: "QueueConsumer",
});
const OIDC_ISSUER = "https://token.actions.githubusercontent.com";
const TRUSTED_ROOT = "release/trust/trusted-root.json";
const PINNED_GH_VERSION = "2.96.0";
const PINNED_COSIGN_VERSION = "v3.0.6";
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const COMMIT = /^[0-9a-f]{40}$/u;
const GIT_OBJECT = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u;
const POSITIVE_INTEGER = /^[1-9][0-9]*$/u;
const REQUEST_ID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;
const PROVIDER_TAG = /^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$/u;
const REVOCATION_TAG = /^forms\/revocations\/v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$/u;
const FORM_RELEASE_AUTHORITY_PATHS = Object.freeze([
  ".github/workflows/form-package-revocation.yml",
  TRUSTED_ROOT,
  "cmd/form-package-release",
  "formpackage",
  "go.mod",
  "go.sum",
  "internal/admissioncheckpoint",
  "internal/admissionrelease",
  "internal/client",
  "internal/formcatalog",
  "internal/formregistry",
  "internal/hostpolicy",
  "internal/portableconformance",
  "internal/provider",
  "internal/providerlifecycle",
  "internal/standardforms",
  "package.json",
  "scripts/release-deploy.mjs",
  "standardform",
]);
const PROVIDER_RECOVERY_ALLOWED_PATHS = Object.freeze([
  "release/README.md",
  "scripts/check-public-surfaces.mjs",
  "scripts/release-deploy.mjs",
  "scripts/release-deploy.test.mjs",
  "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
]);
const PROVIDER_SURFACE = "takoform-provider-release";
const FORM_SURFACE = "takoform-form-package-release";
const WORKFLOW_NAMES = {
  "provider-release-tag.yml": "Author provider release tag",
  "release.yml": "Prepare provider release candidate",
  "form-package-revocation.yml":
    "Prepare signed Form Package revocation checkpoint",
};

export const RELEASE_SURFACES = Object.freeze([
  {
    surface: PROVIDER_SURFACE,
    target:
      "github-release:tako0614/terraform-provider-takoform/provider + registry.terraform.io/tako0614/takoform",
    covers: [
      "release/version.json",
      "release/keys/provider-signing-key.asc",
      ".github/workflows/provider-release-tag.yml",
      ".github/workflows/release.yml",
      "cmd/provider-release",
    ],
    requiresScripts: ["check"],
    requiresTools: ["git", "bun", "gh", "go", "gpg", "cosign", "curl"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "requires local operator GH_TOKEN authority, a clean non-shallow main checkout equal to a freshly fetched canonical origin/main, the complete owner check before every dispatch, tag push, or release mutation, an explicitly named successful workflow run/attempt, checksum closure over its same-run candidate, the pinned provider GPG signer, and a record of the source commit plus every published asset digest; GH_TOKEN is never printed or retained",
      "post-conditions":
        "publishes the exact verified same-run bytes locally under exclusive single-writer authority because GitHub REST has no atomic asset-plus-metadata precondition, immediately rereads the empty and complete exact draft, restates the full exact identity on PATCH, requires GitHub's immutable release readback and a fresh download with identical digests, and rechecks the pinned signed tag before VERIFIED",
      reversal:
        "provider versions, signed tags, and release assets are immutable and cannot be rolled back or overwritten; an exact signed tag-only partial state may only be completed by recover-tag-only, and an exact retained draft may only be resumed by recover-draft without changing the tag, candidate, or draft identity; any other bad publication is halted and repaired forward under a new version",
      "failure-handling":
        "prints raw command diagnostics and distinguishes failure before tag creation, after local tag materialization, after remote tag push, after draft creation, and after immutable publication; it retains and reports every created draft for authoritative inspection, never automatically removes a release or tag, refuses blind retry after an indeterminate mutation, and exposes only explicit exact-tag and exact-draft recovery bound to the original release commit, tag object, run attempt, and current reviewed recovery commit",
      "independent-review":
        "the provider-release protected Environment reviews the signed tag and release candidates; the local publisher accepts only that exact successful run/attempt and re-verifies it independently before using local GitHub authority",
      "no-overwrite":
        "requires the descriptor tag, refuses any pre-existing local/remote tag, Registry version, or GitHub Release before creation, uses zero-object-id compare-and-swap for local refs, pushes an exact ref, and accepts only an immutable release with the exact candidate inventory; recovery never mutates or deletes a tag and accepts only the exact pinned signed local/remote annotated object",
      halt:
        "prepare and tag stop after one exact workflow dispatch and return its URL as AWAITING_REVIEW; cancel that exact run before approval on any input or evidence mismatch, and never continue by selecting a latest run",
    },
  },
  {
    surface: FORM_SURFACE,
    target:
      "github-release:tako0614/terraform-provider-takoform/forms/revocations/*",
    covers: [
      "forms/revocations",
      ".github/workflows/form-package-revocation.yml",
      "cmd/form-package-release",
    ],
    requiresScripts: ["check"],
    requiresTools: ["git", "bun", "gh", "go", "cosign"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "uses local operator GH_TOKEN authority without printing or retaining GH_TOKEN; every phase accepts one exact forms/revocations/v<semver> tag plus its exact source commit, requires the committed revocation source and checkpoint files, a clean non-shallow current protected main that descends from that source commit, and the pinned gh/cosign toolchain, runs the complete owner check before dispatch and again before every mutation, consumes only one explicitly named successful revocation-workflow run/attempt, checksum-closes that same-run candidate, verifies its deterministic tag object, source/tooling commit ancestry, and Sigstore identity against the trusted root retained at the tooling commit, and records the exact tag object and every published asset digest",
      "post-conditions":
        "after the create-only tag push and Release publication, publish-revocation requires exact remote tag resolution, one immutable release with the exact id/tag identity, exact API asset digests, and a fresh six-file revocation download identical to the verified candidate; verify-revocation closes the published identity by re-downloading the live release, re-verifying its manifest, checksum closure, deep Go semantic report, and Sigstore bundle, and binding the published source commit to the expected commit and its tooling commit to an ancestor of current protected main",
      reversal:
        "revocation checkpoints are append-only cumulative identities: a published checkpoint tag or release is never overwritten, deleted, or rolled back, and no automated recovery phase exists; a wrong or incomplete checkpoint is superseded only by a later cumulative checkpoint under a new version, and an interrupted publication leaves the reported exact tag/release state for operator inspection instead of being resumed blindly",
      "failure-handling":
        "prints raw command diagnostics, reports the exact local and remote ref state after a tag-side failure and the retained draft or ambiguous release identities after a release-side failure, never automatically removes a release or tag, and refuses blind retry whenever mutation state is indeterminate",
      "independent-review":
        "the form-package-release protected Environment reviews the revocation workflow run that builds and keyless-signs the exact cumulative checkpoint candidate; local publication consumes only that named successful run/attempt and independently re-verifies its checksum, deterministic tag object, and Sigstore closure before any mutation",
      "no-overwrite":
        "the exact tag must be absent locally and remotely and no GitHub Release identity may exist for it before creation; local tag creation is zero-object-id compare-and-swap over the exact reconstructed candidate object, the remote push is a create-only lease of that exact ref, and final readback accepts only one immutable release with the exact candidate inventory, so every checkpoint identity only appends",
      halt:
        "prepare-revocation stops after one exact workflow dispatch and returns its URL as AWAITING_REVIEW; cancel that exact run before approval if any input changes, and publish-revocation never infers or selects a latest run — it consumes only the explicitly named run/attempt",
    },
  },
]);

const PHASES = {
  [PROVIDER_SURFACE]: {
    prepare: ["tag", "expected-commit"],
    tag: ["tag", "expected-commit", "run-id", "run-attempt"],
    publish: ["tag", "expected-commit", "run-id", "run-attempt"],
    "recover-tag-only": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "run-id",
      "run-attempt",
    ],
    "recover-draft": [
      "tag",
      "expected-release-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "release-id",
      "run-id",
      "run-attempt",
    ],
  },
  [FORM_SURFACE]: {
    "prepare-revocation": ["tag", "expected-commit"],
    "publish-revocation": [
      "tag",
      "expected-commit",
      "run-id",
      "run-attempt",
    ],
    "verify-revocation": ["tag", "expected-commit"],
  },
};

export function isReleaseSurface(name) {
  return name === PROVIDER_SURFACE || name === FORM_SURFACE;
}

export function parseReleaseSurfaceArgs(surface, args) {
  if (!isReleaseSurface(surface)) {
    throw new Error(`unknown release surface ${JSON.stringify(surface)}`);
  }
  if (!Array.isArray(args) || args.length === 0 || args[0].startsWith("-")) {
    throw usageError(surface);
  }
  const [phase, ...rest] = args;
  const required = PHASES[surface][phase];
  if (!required) throw usageError(surface);
  const values = {};
  for (let index = 0; index < rest.length; index += 2) {
    const option = rest[index];
    const value = rest[index + 1];
    if (
      typeof option !== "string" ||
      !option.startsWith("--") ||
      option === "--" ||
      typeof value !== "string" ||
      value === "" ||
      value.startsWith("--")
    ) {
      throw usageError(surface);
    }
    const name = option.slice(2);
    if (!required.includes(name) || Object.hasOwn(values, name)) {
      throw usageError(surface);
    }
    values[name] = value;
  }
  if (
    rest.length !== required.length * 2 ||
    required.some((name) => !Object.hasOwn(values, name))
  ) {
    throw usageError(surface);
  }
  for (const name of [
    "expected-commit",
    "expected-release-commit",
    "expected-recovery-commit",
  ]) {
    if (values[name] && !COMMIT.test(values[name])) {
      throw new Error(
        `--${name} must be an exact lowercase 40-character commit`,
      );
    }
  }
  if (
    values["expected-tag-object"] &&
    !GIT_OBJECT.test(values["expected-tag-object"])
  ) {
    throw new Error(
      "--expected-tag-object must be an exact lowercase Git object id",
    );
  }
  for (const name of ["release-id", "run-id", "run-attempt"]) {
    if (values[name] && !POSITIVE_INTEGER.test(values[name])) {
      throw new Error(`--${name} must be a positive decimal integer`);
    }
    if (
      values[name] &&
      !Number.isSafeInteger(Number(values[name]))
    ) {
      throw new Error(`--${name} must be a safe positive decimal integer`);
    }
  }
  return { phase, ...values };
}

function usageError(surface) {
  const phases = Object.entries(PHASES[surface] ?? {})
    .map(
      ([phase, options]) =>
        `  ${phase}${options.map((name) => ` --${name} <value>`).join("")}`,
    )
    .join("\n");
  return new Error(
    `usage: bun run deploy -- ${surface} <phase> [exact options]\n${phases}`,
  );
}

export function runReleaseSurface({
  surface,
  args,
  repo,
  stdout = process.stdout,
  stderr = process.stderr,
  execFile = execFileSync,
  uuidFactory = randomUUID,
  now = Date.now,
  wait = blockingWait,
}) {
  if (!repo || typeof repo !== "string") throw new Error("repo is required");
  if (typeof process.env.GH_TOKEN !== "string" || process.env.GH_TOKEN.trim() === "") {
    throw new Error(
      "release blocked: non-empty GH_TOKEN is required for local owner authority",
    );
  }
  const context = {
    repo: resolve(repo),
    stdout,
    stderr,
    execFile,
    uuidFactory,
    now,
    wait,
  };
  const options = parseReleaseSurfaceArgs(surface, args);
  if (surface === PROVIDER_SURFACE) {
    return runProvider(context, options);
  }
  return runForm(context, options);
}

function blockingWait(milliseconds) {
  Atomics.wait(
    new Int32Array(new SharedArrayBuffer(4)),
    0,
    0,
    milliseconds,
  );
}

function runProvider(context, options) {
  const descriptor = readProviderDescriptor(context.repo);
  requireProviderTag(options.tag, descriptor);
  switch (options.phase) {
    case "prepare":
      return providerPrepare(context, options, descriptor);
    case "tag":
      return providerTag(context, options, descriptor);
    case "publish":
      return providerPublish(context, options, descriptor);
    case "recover-tag-only":
      return providerRecoverTagOnly(context, options, descriptor);
    case "recover-draft":
      return providerRecoverDraft(context, options, descriptor);
    default:
      throw usageError(PROVIDER_SURFACE);
  }
}

function runForm(context, options) {
  switch (options.phase) {
    case "prepare-revocation":
      return revocationPrepare(context, options);
    case "publish-revocation":
      return revocationPublish(context, options);
    case "verify-revocation":
      return revocationVerify(context, options);
    default:
      throw usageError(FORM_SURFACE);
  }
}

function command(
  context,
  executable,
  args,
  {
    cwd = context.repo,
    input,
    echo = false,
    label,
    env,
    encoding,
  } = {},
) {
  try {
    const output = context.execFile(executable, args, {
      cwd,
      input,
      encoding:
        encoding === undefined
          ? input instanceof Uint8Array
            ? null
            : "utf8"
          : encoding,
      stdio: ["pipe", "pipe", "pipe"],
      maxBuffer: 128 * 1024 * 1024,
      env: env ?? subprocessEnvironment(executable),
    });
    if (echo && output) context.stdout.write(output);
    return output;
  } catch (error) {
    if (error.stdout) context.stdout.write(asText(error.stdout));
    if (error.stderr) context.stderr.write(asText(error.stderr));
    const detail = label ?? `${executable} ${args.join(" ")}`;
    const failure = new Error(`${detail} failed`);
    failure.cause = error;
    throw failure;
  }
}

function attemptCommand(context, executable, args, options = {}) {
  try {
    return {
      ok: true,
      output: context.execFile(executable, args, {
        cwd: options.cwd ?? context.repo,
        input: options.input,
        encoding: "utf8",
        stdio: ["pipe", "pipe", "pipe"],
        maxBuffer: 128 * 1024 * 1024,
        env: options.env ?? subprocessEnvironment(executable),
      }),
      stderr: "",
      status: 0,
    };
  } catch (error) {
    return {
      ok: false,
      output: asText(error.stdout),
      stderr: asText(error.stderr),
      status: error.status,
      error,
    };
  }
}

function environmentWithoutGitHubAuthority() {
  const environment = { ...process.env };
  delete environment.GH_TOKEN;
  delete environment.GITHUB_TOKEN;
  return environment;
}

function subprocessEnvironment(executable) {
  const environment = environmentWithoutGitHubAuthority();
  if (executable === "gh") environment.GH_TOKEN = process.env.GH_TOKEN;
  return environment;
}

function githubUploadEnvironment() {
  const environment = environmentWithoutGitHubAuthority();
  delete environment.GH_ENTERPRISE_TOKEN;
  delete environment.GITHUB_ENTERPRISE_TOKEN;
  environment.GH_ENTERPRISE_TOKEN = process.env.GH_TOKEN;
  return environment;
}

function gitPushEnvironment() {
  const environment = environmentWithoutGitHubAuthority();
  delete environment.GIT_CONFIG_COUNT;
  for (const name of Object.keys(environment)) {
    if (
      /^GIT_CONFIG_(?:KEY|VALUE)_[0-9]+$/u.test(name) ||
      /^GIT_TRACE/u.test(name) ||
      name === "GIT_CURL_VERBOSE"
    ) {
      delete environment[name];
    }
  }
  environment.GIT_TERMINAL_PROMPT = "0";
  environment.GIT_ASKPASS = "/bin/false";
  environment.SSH_ASKPASS = "/bin/false";
  environment.GIT_CONFIG_COUNT = "5";
  environment.GIT_CONFIG_KEY_0 = "credential.helper";
  environment.GIT_CONFIG_VALUE_0 = "";
  environment.GIT_CONFIG_KEY_1 = "credential.interactive";
  environment.GIT_CONFIG_VALUE_1 = "never";
  environment.GIT_CONFIG_KEY_2 =
    "http.https://github.com/.extraHeader";
  environment.GIT_CONFIG_VALUE_2 = "";
  environment.GIT_CONFIG_KEY_3 =
    "http.https://github.com/.extraHeader";
  environment.GIT_CONFIG_VALUE_3 =
    `AUTHORIZATION: basic ${Buffer.from(`x-access-token:${process.env.GH_TOKEN}`, "utf8").toString("base64")}`;
  environment.GIT_CONFIG_KEY_4 = "core.hooksPath";
  environment.GIT_CONFIG_VALUE_4 = "/dev/null";
  return environment;
}

function recoveryReadOnlyGitEnvironment() {
  const environment = environmentWithoutGitHubAuthority();
  const unsafeNames = new Set([
    "GIT_ALTERNATE_OBJECT_DIRECTORIES",
    "GIT_ASKPASS",
    "GIT_ATTR_NOSYSTEM",
    "GIT_CEILING_DIRECTORIES",
    "GIT_COMMON_DIR",
    "GIT_CONFIG_PARAMETERS",
    "GIT_DIFF_OPTS",
    "GIT_DIR",
    "GIT_DISCOVERY_ACROSS_FILESYSTEM",
    "GIT_EXEC_PATH",
    "GIT_EXTERNAL_DIFF",
    "GIT_GRAFT_FILE",
    "GIT_INDEX_FILE",
    "GIT_INTERNAL_SUPER_PREFIX",
    "GIT_NAMESPACE",
    "GIT_OBJECT_DIRECTORY",
    "GIT_PREFIX",
    "GIT_PROXY_COMMAND",
    "GIT_QUARANTINE_PATH",
    "GIT_REPLACE_REF_BASE",
    "GIT_SHALLOW_FILE",
    "GIT_SSH",
    "GIT_SSH_COMMAND",
    "GIT_TEMPLATE_DIR",
    "GIT_WORK_TREE",
    "SSH_ASKPASS",
  ]);
  for (const name of Object.keys(environment)) {
    if (
      unsafeNames.has(name) ||
      /^GIT_CONFIG_/u.test(name) ||
      /^GIT_TRACE/u.test(name)
    ) {
      delete environment[name];
    }
  }
  environment.GIT_CONFIG_NOSYSTEM = "1";
  environment.GIT_CONFIG_GLOBAL = "/dev/null";
  environment.GIT_CONFIG_SYSTEM = "/dev/null";
  environment.GIT_NO_REPLACE_OBJECTS = "1";
  environment.GIT_OPTIONAL_LOCKS = "0";
  return environment;
}

function recoveryGit(context, args, options = {}) {
  return command(context, "git", args, {
    ...options,
    env: recoveryReadOnlyGitEnvironment(),
  });
}

function trustedGpgExecutable() {
  const candidates = [
    "/usr/bin/gpg",
    "/usr/local/bin/gpg",
    "/opt/homebrew/bin/gpg",
    ...(process.env.PATH ?? "")
      .split(delimiter)
      .filter((directory) => isAbsolute(directory))
      .map((directory) => resolve(directory, "gpg")),
  ];
  for (const candidate of new Set(candidates)) {
    try {
      accessSync(candidate, fsConstants.X_OK);
      const executable = realpathSync(candidate);
      if (isAbsolute(executable) && lstatSync(executable).isFile()) {
        return executable;
      }
    } catch {
      // Continue to the next operator-installed absolute candidate.
    }
  }
  throw new Error("release blocked: no absolute trusted gpg executable");
}

function isolatedGpgEnvironment(gpgHome) {
  const environment = recoveryReadOnlyGitEnvironment();
  delete environment.GNUPGHOME;
  delete environment.GPG_AGENT_INFO;
  delete environment.GPG_TTY;
  environment.GNUPGHOME = gpgHome;
  environment.LC_ALL = "C";
  return environment;
}

function asText(value) {
  if (!value) return "";
  return Buffer.isBuffer(value) ? value.toString("utf8") : String(value);
}

function git(context, ...args) {
  return command(context, "git", args).trim();
}

function progress(context, text) {
  context.stdout.write(`\n==> ${text}\n`);
}

function emit(context, result) {
  context.stdout.write(`\n${JSON.stringify(result, null, 2)}\n`);
  return result;
}

function runOwnerCheck(context) {
  // Publication proves bytes; admission proves a conforming host signed for
  // them. They are separate phases, and the admission closure can only be
  // green after publication has already happened. Gating publication on it
  // would make the first release of any new Form version unreachable, so the
  // owner gate stops at publication authority and admission keeps its own gate.
  progress(context, "bun run check:release-owner-gate");
  command(context, "bun", ["run", "check:release-owner-gate"], {
    echo: true,
    label: "owner gate",
  });
}

function ownerGateAndFence(context, expectedCommit, options) {
  verifyLocalReleaseToolchain(context);
  runOwnerCheck(context);
  return assertCurrentProtectedMain(context, expectedCommit, options);
}

function verifyLocalReleaseToolchain(context) {
  if (context.releaseToolchainVerified) return;
  const ghVersion = command(context, "gh", ["--version"]);
  const cosignVersion = command(context, "cosign", ["version"]);
  if (
    !ghVersion.startsWith(`gh version ${PINNED_GH_VERSION} `) ||
    !new RegExp(
      `^GitVersion:\\s+${PINNED_COSIGN_VERSION.replaceAll(".", "\\.")}$`,
      "mu",
    ).test(cosignVersion)
  ) {
    throw new Error(
      `release toolchain drift: require gh ${PINNED_GH_VERSION} and cosign ${PINNED_COSIGN_VERSION}`,
    );
  }
  context.releaseToolchainVerified = true;
}

function assertCurrentProtectedMain(context, expectedCommit, { exact = true } = {}) {
  const dirty = git(context, "status", "--porcelain=v1", "--untracked-files=all");
  if (dirty !== "") {
    throw new Error(
      `release blocked: worktree is dirty\n${dirty.split("\n").slice(0, 30).join("\n")}`,
    );
  }
  if (git(context, "rev-parse", "--is-shallow-repository") !== "false") {
    throw new Error("release blocked: shallow repositories cannot prove ancestry");
  }
  const origin = git(context, "remote", "get-url", "origin");
  if (!isCanonicalOrigin(origin)) {
    throw new Error(
      `release blocked: origin is not the canonical ${SOURCE_REPOSITORY}: ${origin}`,
    );
  }
  if (git(context, "symbolic-ref", "--quiet", "--short", "HEAD") !== "main") {
    throw new Error("release blocked: checkout must be attached to main");
  }
  progress(context, "fetch canonical protected origin/main");
  command(context, "git", [
    "fetch",
    "--no-tags",
    "--prune",
    "origin",
    "+refs/heads/main:refs/remotes/origin/main",
  ]);
  const head = git(context, "rev-parse", "HEAD");
  const remoteMain = git(context, "rev-parse", "refs/remotes/origin/main");
  if (head !== remoteMain) {
    throw new Error(
      `release blocked: HEAD ${head} is not fresh origin/main ${remoteMain}`,
    );
  }
  if (expectedCommit) {
    if (exact && expectedCommit !== head) {
      throw new Error(
        `release blocked: expected commit ${expectedCommit} is not current protected main ${head}`,
      );
    }
    command(context, "git", ["cat-file", "-e", `${expectedCommit}^{commit}`]);
    if (!exact) {
      command(context, "git", [
        "merge-base",
        "--is-ancestor",
        expectedCommit,
        head,
      ]);
    }
  }
  return head;
}

function assertCommitAncestor(context, ancestor, descendant, label) {
  if (!COMMIT.test(ancestor ?? "") || !COMMIT.test(descendant ?? "")) {
    throw new Error(`${label} requires exact lowercase commits`);
  }
  command(context, "git", ["cat-file", "-e", `${ancestor}^{commit}`]);
  command(context, "git", [
    "merge-base",
    "--is-ancestor",
    ancestor,
    descendant,
  ], { label: `${label}: ${ancestor} must be an ancestor of ${descendant}` });
}

function assertRecoveryCommitAncestor(
  context,
  ancestor,
  descendant,
  label,
) {
  if (!COMMIT.test(ancestor ?? "") || !COMMIT.test(descendant ?? "")) {
    throw new Error(`${label} requires exact lowercase commits`);
  }
  recoveryGit(context, ["cat-file", "-e", `${ancestor}^{commit}`]);
  recoveryGit(
    context,
    ["merge-base", "--is-ancestor", ancestor, descendant],
    { label: `${label}: ${ancestor} must be an ancestor of ${descendant}` },
  );
}

function assertFormReleaseAuthorityFence(
  context,
  { sourceCommit, toolingCommit, currentMain, label },
) {
  assertCommitAncestor(
    context,
    sourceCommit,
    toolingCommit,
    `${label} source/tooling ancestry`,
  );
  assertCommitAncestor(
    context,
    toolingCommit,
    currentMain,
    `${label} tooling/current-main ancestry`,
  );
  const authorityPaths = [
    ...FORM_RELEASE_AUTHORITY_PATHS,
    "forms/revocations",
  ].sort();
  command(
    context,
    "git",
    [
      "diff",
      "--quiet",
      toolingCommit,
      currentMain,
      "--",
      ...authorityPaths,
    ],
    {
      label: `${label}: release authority paths changed after reviewed tooling commit`,
    },
  );
}

function assertProviderRecoveryFence(
  context,
  { releaseCommit, recoveryCommit, label },
) {
  assertRecoveryCommitAncestor(
    context,
    releaseCommit,
    recoveryCommit,
    `${label} release/recovery ancestry`,
  );
  const rawChanged = recoveryGit(context, [
    "-c",
    "diff.renames=false",
    "diff",
    "--no-renames",
    "--no-ext-diff",
    "--no-textconv",
    "--name-only",
    "-z",
    "--diff-filter=ACDMRTUXB",
    releaseCommit,
    recoveryCommit,
    "--",
  ]);
  if (
    typeof rawChanged !== "string" ||
    (rawChanged !== "" && !rawChanged.endsWith("\0"))
  ) {
    throw new Error(`${label}: recovery diff path output is ambiguous`);
  }
  const changed =
    rawChanged === "" ? [] : rawChanged.slice(0, -1).split("\0");
  if (
    changed.some((path) => path === "" || /[\r\n]/u.test(path)) ||
    new Set(changed).size !== changed.length
  ) {
    throw new Error(`${label}: recovery diff contains an ambiguous path`);
  }
  if (
    changed.length === 0 ||
    !changed.includes("scripts/release-deploy.mjs") ||
    changed.some((path) => !PROVIDER_RECOVERY_ALLOWED_PATHS.includes(path))
  ) {
    throw new Error(
      `${label}: release/recovery diff must contain only the exact reviewed recovery implementation, tests, and documentation; observed ${changed.join(", ") || "no changes"}`,
    );
  }
  return changed;
}

function isCanonicalOrigin(origin) {
  return [
    `https://github.com/${GITHUB_REPOSITORY}.git`,
    `https://github.com/${GITHUB_REPOSITORY}`,
    `git@github.com:${GITHUB_REPOSITORY}.git`,
    `ssh://git@github.com/${GITHUB_REPOSITORY}.git`,
  ].includes(origin);
}

function readJSON(path, label) {
  let value;
  try {
    value = JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    throw new Error(`${label} is not valid JSON: ${error.message}`);
  }
  return value;
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

function requireCanonicalSortedJSON(raw, value, label) {
  if (!raw.equals(Buffer.from(JSON.stringify(recursivelySorted(value))))) {
    throw new Error(`${label} must be canonical recursively key-sorted JSON`);
  }
}

function parseCandidateMetadata(
  raw,
  label,
  { profile = "compact-optional-lf" } = {},
) {
  if (
    profile !== "compact-optional-lf" &&
    profile !== "pretty-required-lf"
  ) {
    throw new Error(`${label} uses unsupported metadata profile: ${profile}`);
  }
  let value;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`${label} is not valid JSON: ${error.message}`);
  }
  const canonical = Buffer.from(
    JSON.stringify(recursivelySorted(value)),
  );
  const canonicalWithNewline = Buffer.concat([
    canonical,
    Buffer.from("\n"),
  ]);
  const prettyWithNewline = Buffer.from(
    `${JSON.stringify(recursivelySorted(value), null, 2)}\n`,
  );
  const accepted =
    profile === "compact-optional-lf"
      ? raw.equals(canonical) || raw.equals(canonicalWithNewline)
      : raw.equals(prettyWithNewline);
  if (!accepted) {
    throw new Error(
      `${label} must be exact recursively key-sorted ${profile === "pretty-required-lf" ? "two-space pretty JSON with one trailing LF" : "compact canonical JSON"}`,
    );
  }
  return value;
}

function parseCanonicalCandidateMetadata(raw, label) {
  return parseCandidateMetadata(raw, label);
}

function parsePrettyCandidateMetadata(raw, label) {
  return parseCandidateMetadata(raw, label, {
    profile: "pretty-required-lf",
  });
}

export function validateProviderIdentityLedger(repo, descriptor) {
  const ledger = readJSON(
    join(repo, PROVIDER_IDENTITY_LEDGER),
    "provider Form identity ledger",
  );
  requireExactKeys(ledger, ["format", "releases"], "provider Form identity ledger");
  if (
    ledger.format !== "takoform.provider-form-identities@v1" ||
    !Array.isArray(ledger.releases) ||
    ledger.releases.length === 0
  ) {
    throw new Error("provider Form identity ledger has an invalid envelope");
  }
  const providerVersions = new Set();
  const formRefs = new Set();
  let current;
  for (const [releaseIndex, release] of ledger.releases.entries()) {
    requireExactKeys(
      release,
      ["family", "formMaturity", "forms", "portableApiVersion", "providerVersion"],
      `provider Form identity ledger release ${releaseIndex}`,
    );
    if (
      typeof release.providerVersion !== "string" ||
      !PROVIDER_TAG.test(`v${release.providerVersion}`) ||
      typeof release.portableApiVersion !== "string" ||
      typeof release.family !== "string" ||
      release.formMaturity !== "experimental" ||
      !Array.isArray(release.forms) ||
      release.forms.length === 0
    ) {
      throw new Error(
        `provider Form identity ledger release ${releaseIndex} is invalid`,
      );
    }
    if (providerVersions.has(release.providerVersion)) {
      throw new Error(
        `provider Form identity ledger duplicates ${release.providerVersion}`,
      );
    }
    providerVersions.add(release.providerVersion);
    if (release.providerVersion === descriptor.version) current = release;
    for (const [formIndex, form] of release.forms.entries()) {
      requireExactKeys(
        form,
        ["formRef", "packageDigest", "resourceType"],
        `${release.providerVersion} provider-embedded Form identity ${formIndex}`,
      );
      requireExactKeys(
        form.formRef,
        ["apiVersion", "definitionVersion", "kind", "schemaDigest"],
        `${release.providerVersion} provider-embedded FormRef ${formIndex}`,
      );
      if (
        typeof form.resourceType !== "string" ||
        !/^takoform_[a-z0-9_]+$/u.test(form.resourceType) ||
        !SHA256.test(form.packageDigest ?? "") ||
        typeof form.formRef.apiVersion !== "string" ||
        form.formRef.apiVersion !== release.family ||
        !/^[A-Z][A-Za-z0-9]{0,63}$/u.test(form.formRef.kind ?? "") ||
        !PROVIDER_TAG.test(`v${form.formRef.definitionVersion ?? ""}`) ||
        !SHA256.test(form.formRef.schemaDigest ?? "")
      ) {
        throw new Error(
          `${release.providerVersion}: invalid provider-embedded Form identity`,
        );
      }
      const refKey = JSON.stringify([
        form.formRef.apiVersion,
        form.formRef.kind,
        form.formRef.definitionVersion,
        form.formRef.schemaDigest,
      ]);
      if (formRefs.has(refKey)) {
        throw new Error(
          `${release.providerVersion}: duplicate provider-embedded FormRef`,
        );
      }
      formRefs.add(refKey);
    }
  }
  if (
    current === undefined ||
    current.portableApiVersion !== descriptor.versioning?.portableApiVersion ||
    current.portableApiVersion !== PROVIDER_HOST_API ||
    current.family !== PROVIDER_FORM_FAMILY ||
    current.formMaturity !== "experimental" ||
    current.forms.length !== Object.keys(PROVIDER_FORM_KINDS).length
  ) {
    throw new Error(
      "provider Form identity ledger has no exact 15-entry release for the descriptor",
    );
  }
  const resourceTypes = new Set();
  for (const [formIndex, form] of current.forms.entries()) {
    const expectedKind = PROVIDER_FORM_KINDS[form.resourceType];
    if (
      expectedKind === undefined ||
      resourceTypes.has(form.resourceType) ||
      form.formRef.kind !== expectedKind ||
      form.formRef.apiVersion !== PROVIDER_FORM_FAMILY ||
      form.formRef.definitionVersion !== "0.1.0"
    ) {
      throw new Error(
        `provider v2.1 embedded Form identity ${formIndex} is not an exact Beta family entry`,
      );
    }
    resourceTypes.add(form.resourceType);
  }
  if (resourceTypes.size !== Object.keys(PROVIDER_FORM_KINDS).length) {
    throw new Error(
      "provider v2.1 identity ledger does not contain the exact 15 resource types",
    );
  }

  const candidate = readJSON(
    join(repo, PROVIDER_CANDIDATE_SET),
    "provider Beta candidate set",
  );
  requireExactKeys(
    candidate,
    [
      "authoringPolicy",
      "authoringSource",
      "family",
      "formMaturity",
      "format",
      "forms",
      "packageApiVersion",
      "publicationStatus",
    ],
    "provider Beta candidate set",
  );
  if (
    candidate.format !== "takoform.form-family-candidates@v1" ||
    candidate.family !== PROVIDER_FORM_FAMILY ||
    candidate.formMaturity !== "experimental" ||
    candidate.packageApiVersion !== "packages.forms.takoform.com/v1alpha4" ||
    candidate.publicationStatus !== "unpublished" ||
    !Array.isArray(candidate.forms) ||
    candidate.forms.length !== current.forms.length
  ) {
    throw new Error("provider Beta candidate set is not the exact 15-entry family set");
  }
  const candidateProjection = candidate.forms.map(
    ({ resourceType, formRef, packageDigest }) => ({
      resourceType,
      formRef,
      packageDigest,
    }),
  );
  if (JSON.stringify(candidateProjection) !== JSON.stringify(current.forms)) {
    throw new Error(
      "provider v2.1 identity ledger differs from the exact Beta candidate set",
    );
  }
  return ledger;
}

export function readProviderDescriptor(repo) {
  const descriptor = readJSON(
    join(repo, "release/version.json"),
    "provider release descriptor",
  );
  if (
    typeof descriptor !== "object" ||
    descriptor === null ||
    descriptor.version !== PROVIDER_VERSION ||
    !PROVIDER_TAG.test(descriptor.tag) ||
    descriptor.tag !== `v${descriptor.version}` ||
    descriptor.sourceRepository !== `github.com/${GITHUB_REPOSITORY}` ||
    descriptor.providerAddress !== PROVIDER_ADDRESS ||
    descriptor.goModule !== `github.com/${GITHUB_REPOSITORY}` ||
    descriptor.signingFingerprint !== PROVIDER_SIGNER ||
    descriptor.publicationStatus !== "candidate-only" ||
    !Array.isArray(descriptor.platforms) ||
    JSON.stringify([...descriptor.platforms].sort()) !==
      JSON.stringify(
        [
          "darwin_amd64",
          "darwin_arm64",
          "linux_amd64",
          "linux_arm64",
          "windows_amd64",
        ].sort(),
      )
  ) {
    throw new Error("provider release descriptor identity is invalid");
  }
  requireExactKeys(
    descriptor.versioning,
    [
      "formDefinitionVersions",
      "formPackageVersions",
      "portableApiVersion",
      "providerCompatibility",
    ],
    "provider release descriptor versioning",
  );
  if (
    descriptor.versioning.providerCompatibility !== "semver-major" ||
    descriptor.versioning.portableApiVersion !== PROVIDER_HOST_API ||
    descriptor.versioning.formDefinitionVersions !==
      "independent-immutable-semver" ||
    descriptor.versioning.formPackageVersions !==
      "content-addressed-current-retained-legacy-semver"
  ) {
    throw new Error("provider release descriptor conflates independent version streams");
  }
  if (
    !Array.isArray(descriptor.cliMatrix) ||
    descriptor.cliMatrix.length !== 2 ||
    descriptor.cliMatrix.some(
      (entry) =>
        !entry ||
        entry.providerAddress !== PROVIDER_ADDRESS ||
        !["OpenTofu", "Terraform"].includes(entry.product) ||
        typeof entry.version !== "string" ||
        entry.version === "",
    ) ||
    new Set(descriptor.cliMatrix.map((entry) => entry.product)).size !== 2
  ) {
    throw new Error("provider release descriptor CLI/FQN matrix is invalid");
  }
  validateProviderIdentityLedger(repo, descriptor);
  return descriptor;
}

function requireProviderTag(tag, descriptor) {
  if (!tag || tag !== descriptor.tag) {
    throw new Error(
      `--tag must exactly match release/version.json (${descriptor.tag})`,
    );
  }
}

function dispatchWorkflow(
  context,
  workflow,
  inputs,
  {
    headSha = inputs.expected_commit,
    ref = "main",
    headBranch = ref,
  } = {},
) {
  if (!COMMIT.test(headSha ?? "")) {
    throw new Error(`dispatch ${workflow} requires an exact head SHA`);
  }
  const requestId = context.uuidFactory();
  if (!REQUEST_ID.test(requestId)) {
    throw new Error("generated workflow request ID is not a canonical UUIDv4");
  }
  const createdAfter = new Date(context.now() - 2000).toISOString();
  const dispatchInputs = { ...inputs, request_id: requestId };
  const workflowName = WORKFLOW_NAMES[workflow];
  if (!workflowName) throw new Error(`unknown dispatch workflow ${workflow}`);
  const listCorrelated = () => {
    const listArguments = [
      "run",
      "list",
      "--repo",
      GITHUB_REPOSITORY,
      "--workflow",
      workflow,
      ...(ref === "main" ? ["--branch", ref] : []),
      "--event",
      "workflow_dispatch",
      "--commit",
      headSha,
      "--created",
      `>=${createdAfter}`,
      "--limit",
      "100",
      "--json",
      "attempt,createdAt,databaseId,displayTitle,event,headBranch,headSha,status,url,workflowName",
    ];
    const listed = JSON.parse(
      command(context, "gh", listArguments),
    );
    return listed.filter(
      (run) =>
        run.workflowName === workflowName &&
        run.event === "workflow_dispatch" &&
        run.headBranch === headBranch &&
        run.headSha === headSha &&
        new Date(run.createdAt).getTime() >= new Date(createdAfter).getTime() &&
        run.displayTitle === requestId,
    );
  };
  const before = listCorrelated();
  if (before.length !== 0) {
    throw new Error(
      `request UUID unexpectedly matched ${before.length} pre-existing workflow runs`,
    );
  }
  const args = [
    "workflow",
    "run",
    workflow,
    "--repo",
    GITHUB_REPOSITORY,
    "--ref",
    ref,
  ];
  for (const [name, value] of Object.entries(dispatchInputs)) {
    args.push("-f", `${name}=${value}`);
  }
  progress(context, `dispatch ${workflow}`);
  const raw = command(context, "gh", args, {
    echo: true,
    label: `dispatch ${workflow}`,
  });
  let correlated = [];
  for (let attempt = 1; attempt <= 12; attempt += 1) {
    correlated = listCorrelated();
    if (correlated.length === 1) break;
    if (correlated.length > 1) {
      throw new Error(
        `dispatch correlation is ambiguous for request ${requestId}; halt without selecting a run`,
      );
    }
    if (attempt < 12) {
      context.wait(1000);
    }
  }
  if (correlated.length !== 1) {
    throw new Error(
      `workflow dispatch may have occurred but request ${requestId} did not correlate to exactly one run; halt and inspect Actions without selecting latest`,
    );
  }
  const run = correlated[0];
  const runId = String(run.databaseId);
  if (
    !POSITIVE_INTEGER.test(runId) ||
    run.attempt !== 1 ||
    !/^https:\/\/github\.com\/tako0614\/terraform-provider-takoform\/actions\/runs\/[1-9][0-9]*$/u.test(
      run.url,
    )
  ) {
    throw new Error(`correlated workflow run has invalid identity`);
  }
  const returnedURLs = raw.match(
    /https:\/\/github\.com\/tako0614\/terraform-provider-takoform\/actions\/runs\/[1-9][0-9]*/gu,
  );
  if (
    returnedURLs &&
    (returnedURLs.length !== 1 || returnedURLs[0] !== run.url)
  ) {
    throw new Error(
      `workflow dispatch URL disagrees with request-correlated run ${runId}`,
    );
  }
  return { runId, requestId, url: run.url };
}

function requireSuccessfulRun(
  context,
  runId,
  runAttempt,
  { workflowName, headSha, headBranch = "main", displayTitle },
) {
  const raw = command(context, "gh", [
    "run",
    "view",
    runId,
    "--repo",
    GITHUB_REPOSITORY,
    "--attempt",
    runAttempt,
    "--json",
    "attempt,conclusion,databaseId,displayTitle,event,headBranch,headSha,status,url,workflowName",
  ]);
  const run = JSON.parse(raw);
  const attempt = Number(runAttempt);
  const expectedUrl =
    `https://github.com/tako0614/terraform-provider-takoform/actions/runs/${runId}` +
    `/attempts/${runAttempt}`;
  if (
    run.databaseId?.toString() !== runId ||
    run.attempt !== attempt ||
    run.workflowName !== workflowName ||
    run.event !== "workflow_dispatch" ||
    run.headBranch !== headBranch ||
    (headSha ? run.headSha !== headSha : !COMMIT.test(run.headSha ?? "")) ||
    run.status !== "completed" ||
    run.conclusion !== "success" ||
    !REQUEST_ID.test(run.displayTitle ?? "") ||
    run.url !== expectedUrl ||
    (displayTitle && run.displayTitle !== displayTitle)
  ) {
    throw new Error(
      `workflow run ${runId} attempt ${runAttempt} is not the exact successful reviewed candidate: ${raw}`,
    );
  }
  return run;
}

function downloadArtifact(context, runId, artifactName, destination) {
  mkdirSync(destination, { recursive: false });
  progress(context, `download ${artifactName} from run ${runId}`);
  command(context, "gh", [
    "run",
    "download",
    runId,
    "--repo",
    GITHUB_REPOSITORY,
    "--name",
    artifactName,
    "--dir",
    destination,
  ]);
  return destination;
}

function sha256(raw) {
  return `sha256:${createHash("sha256").update(raw).digest("hex")}`;
}

function fileDigest(path) {
  return sha256(readFileSync(path));
}

function compareNames(left, right) {
  return left < right ? -1 : left > right ? 1 : 0;
}

function listRegularFiles(root) {
  const files = [];
  const visit = (directory, prefix = "") => {
    const entries = readdirSync(directory, { withFileTypes: true }).sort(
      (left, right) => compareNames(left.name, right.name),
    );
    if (prefix && entries.length === 0) {
      throw new Error(`candidate contains an empty directory: ${prefix}`);
    }
    for (const entry of entries) {
      const path = join(directory, entry.name);
      const name = prefix ? `${prefix}/${entry.name}` : entry.name;
      const stat = lstatSync(path);
      if (stat.isSymbolicLink()) {
        throw new Error(`candidate contains a symbolic link: ${name}`);
      }
      if (stat.isDirectory()) {
        visit(path, name);
      } else if (stat.isFile()) {
        files.push(name);
      } else {
        throw new Error(`candidate contains a non-regular entry: ${name}`);
      }
    }
  };
  visit(root);
  return files;
}

export function parseStrictChecksums(raw) {
  if (!raw.endsWith("\n") || raw === "\n") {
    throw new Error("SHA256SUMS must be non-empty and newline terminated");
  }
  const entries = new Map();
  for (const line of raw.slice(0, -1).split("\n")) {
    const match = /^([0-9a-f]{64})  ([0-9A-Za-z._+/@-]+)$/u.exec(line);
    if (!match) throw new Error(`invalid SHA256SUMS line: ${line}`);
    const name = match[2];
    if (
      name.startsWith("/") ||
      name.split("/").some((part) => part === "" || part === "." || part === "..") ||
      entries.has(name)
    ) {
      throw new Error(`unsafe or duplicate SHA256SUMS target: ${name}`);
    }
    entries.set(name, `sha256:${match[1]}`);
  }
  return entries;
}

function verifyChecksumClosure(root, checksumName = "SHA256SUMS") {
  const all = listRegularFiles(root);
  if (!all.includes(checksumName)) {
    throw new Error(`${checksumName} is missing`);
  }
  const entries = parseStrictChecksums(
    readFileSync(join(root, checksumName), "utf8"),
  );
  const expected = all.filter((name) => name !== checksumName).sort();
  const actual = [...entries.keys()].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${checksumName} does not close the exact inventory; expected ${expected.join(", ")}, got ${actual.join(", ")}`,
    );
  }
  for (const [name, digest] of entries) {
    const observed = fileDigest(join(root, ...name.split("/")));
    if (observed !== digest) {
      throw new Error(`${checksumName} digest mismatch for ${name}`);
    }
  }
  return entries;
}

function requireExactKeys(value, keys, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} must be an object`);
  }
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${label} fields differ; expected ${expected.join(", ")}, got ${actual.join(", ")}`,
    );
  }
}

function normalizedPositiveInteger(value, label) {
  const text = String(value);
  if (!POSITIVE_INTEGER.test(text)) {
    throw new Error(`${label} must be a positive integer`);
  }
  return text;
}

function validateCandidateAssets(
  root,
  metadata,
  expectedNames,
  { digestFormat = "prefixed" } = {},
) {
  if (!Array.isArray(metadata.assets)) {
    throw new Error("candidate metadata assets must be an array");
  }
  const seen = new Map();
  for (const asset of metadata.assets) {
    requireExactKeys(asset, ["name", "sha256"], "candidate asset");
    if (
      typeof asset.name !== "string" ||
      asset.name.includes("/") ||
      !(digestFormat === "prefixed"
        ? SHA256.test(asset.sha256)
        : /^[0-9a-f]{64}$/u.test(asset.sha256)) ||
      seen.has(asset.name)
    ) {
      throw new Error(`invalid candidate asset: ${JSON.stringify(asset)}`);
    }
    const path = join(root, "assets", asset.name);
    const expectedDigest =
      digestFormat === "prefixed" ? asset.sha256 : `sha256:${asset.sha256}`;
    if (!existsSync(path) || fileDigest(path) !== expectedDigest) {
      throw new Error(`candidate asset digest mismatch: ${asset.name}`);
    }
    seen.set(asset.name, expectedDigest);
  }
  const actualNames = [...seen.keys()].sort();
  if (JSON.stringify(actualNames) !== JSON.stringify([...expectedNames].sort())) {
    throw new Error(
      `candidate public inventory differs; expected ${expectedNames.join(", ")}, got ${actualNames.join(", ")}`,
    );
  }
  return new Map(
    actualNames.map((name) => [
      name,
      { name, sha256: seen.get(name), path: join(root, "assets", name) },
    ]),
  );
}

function verifyCandidateRoot(
  root,
  { tagObject = false, metadataProfile = "compact-optional-lf" } = {},
) {
  const expectedRoot = [
    "SHA256SUMS",
    "assets",
    "metadata.json",
    ...(tagObject ? ["tag-object"] : []),
  ].sort();
  const rootEntries = readdirSync(root, { withFileTypes: true }).sort(
    (left, right) => compareNames(left.name, right.name),
  );
  if (
    JSON.stringify(rootEntries.map((entry) => entry.name)) !==
      JSON.stringify(expectedRoot) ||
    rootEntries.some((entry) =>
      entry.name === "assets"
        ? !entry.isDirectory()
        : !entry.isFile(),
    )
  ) {
    throw new Error(
      `candidate root layout differs; expected ${expectedRoot.join(", ")}`,
    );
  }
  verifyChecksumClosure(root);
  return parseCandidateMetadata(
    readFileSync(join(root, "metadata.json")),
    "candidate metadata",
    { profile: metadataProfile },
  );
}

function withTemporaryDirectory(prefix, operation) {
  const root = mkdtempSync(join(tmpdir(), `${prefix}-`));
  try {
    return operation(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

function localTagOID(context, tag) {
  return git(
    context,
    "for-each-ref",
    "--format=%(objectname)",
    `refs/tags/${tag}`,
  );
}

function recoveryLocalTagOID(context, tag) {
  return recoveryGit(context, [
    "for-each-ref",
    "--format=%(objectname)",
    `refs/tags/${tag}`,
  ]).trim();
}

function remoteTagState(context, tag) {
  const ref = `refs/tags/${tag}`;
  const output = git(context, "ls-remote", "--tags", "origin", ref, `${ref}^{}`);
  return parseRemoteTagState(output, ref);
}

function recoveryRemoteTagState(context, tag) {
  const ref = `refs/tags/${tag}`;
  const output = recoveryGit(context, [
    "ls-remote",
    "--tags",
    SOURCE_REPOSITORY,
    ref,
    `${ref}^{}`,
  ]).trim();
  return parseRemoteTagState(output, ref);
}

function parseRemoteTagState(output, ref) {
  const state = {};
  for (const line of output ? output.split("\n") : []) {
    const [oid, name] = line.split("\t");
    if (name === ref) state.object = oid;
    if (name === `${ref}^{}`) state.commit = oid;
  }
  return state;
}

function observeTagFailureState(context, tag) {
  const localResult = attemptCommand(context, "git", [
    "for-each-ref",
    "--format=%(objectname)",
    `refs/tags/${tag}`,
  ]);
  const local = localResult.ok ? localResult.output.trim() : "";
  const ref = `refs/tags/${tag}`;
  const remoteResult = attemptCommand(context, "git", [
    "ls-remote",
    "--tags",
    "origin",
    ref,
    `${ref}^{}`,
  ]);
  const remote = remoteResult.ok
    ? parseRemoteTagState(remoteResult.output.trim(), ref)
    : null;
  let mutationState = "UNCHANGED";
  if (!localResult.ok && !remoteResult.ok) {
    mutationState = "LOCAL_AND_REMOTE_UNREADABLE";
  } else if (!remoteResult.ok) mutationState = "REMOTE_UNREADABLE";
  else if (remote.object || remote.commit) mutationState = "REMOTE_PRESENT";
  else if (!localResult.ok) mutationState = "LOCAL_UNREADABLE";
  else if (local) mutationState = "LOCAL_ONLY";
  return {
    local: local || null,
    localReadable: localResult.ok,
    remote,
    remoteReadable: remoteResult.ok,
    mutationState,
  };
}

function assertTagAbsent(context, tag) {
  const local = localTagOID(context, tag);
  const remote = remoteTagState(context, tag);
  if (local || remote.object || remote.commit) {
    throw new Error(
      `no-overwrite blocked ${tag}: local=${local || "absent"} remote=${JSON.stringify(remote)}`,
    );
  }
}

function assertExactRemoteTag(context, tag, expectedCommit, expectedObject) {
  const state = remoteTagState(context, tag);
  if (
    !state.object ||
    state.commit !== expectedCommit ||
    (expectedObject && state.object !== expectedObject)
  ) {
    throw new Error(
      `remote tag ${tag} is not the exact annotated object/commit: ${JSON.stringify(state)}`,
    );
  }
  return state;
}

function assertRecoveryExactRemoteTag(
  context,
  tag,
  expectedCommit,
  expectedObject,
) {
  const state = recoveryRemoteTagState(context, tag);
  if (
    !state.object ||
    state.commit !== expectedCommit ||
    state.object !== expectedObject
  ) {
    throw new Error(
      `remote tag ${tag} is not the exact annotated object/commit: ${JSON.stringify(state)}`,
    );
  }
  return state;
}

function reconstructCandidateTagObject(
  context,
  tag,
  expectedCommit,
  root,
  metadata,
) {
  const tagObject = readFileSync(join(root, "tag-object"));
  if (
    metadata.objectFormat !== git(context, "rev-parse", "--show-object-format") ||
    !/^[0-9a-f]{40,64}$/u.test(metadata.tagObjectOid) ||
    !SHA256.test(metadata.tagObjectSha256) ||
    fileDigest(join(root, "tag-object")) !== metadata.tagObjectSha256
  ) {
    throw new Error("candidate tag-object metadata is invalid");
  }
  const text = tagObject.toString("utf8");
  const header = text.slice(0, text.indexOf("\n\n"));
  const fields = Object.fromEntries(
    header.split("\n").map((line) => {
      const space = line.indexOf(" ");
      return [line.slice(0, space), line.slice(space + 1)];
    }),
  );
  if (
    fields.object !== expectedCommit ||
    fields.type !== "commit" ||
    fields.tag !== tag
  ) {
    throw new Error("candidate tag object does not bind the expected tag/commit");
  }
  const reconstructed = command(context, "git", ["mktag"], {
    input: tagObject,
    label: "reconstruct candidate tag object",
  })
    .toString("utf8")
    .trim();
  if (reconstructed !== metadata.tagObjectOid) {
    throw new Error(
      `candidate tag object OID ${metadata.tagObjectOid} reconstructed as ${reconstructed}`,
    );
  }
  return reconstructed;
}

function materializeTagObject(context, tag, expectedCommit, root, metadata) {
  const reconstructed = reconstructCandidateTagObject(
    context,
    tag,
    expectedCommit,
    root,
    metadata,
  );
  if (localTagOID(context, tag)) {
    throw new Error(`local tag ${tag} already exists; refusing resume`);
  }
  const zero = "0".repeat(reconstructed.length);
  command(context, "git", [
    "update-ref",
    `refs/tags/${tag}`,
    reconstructed,
    zero,
  ]);
  return reconstructed;
}

function splitSignedProviderTagObject(raw, tag, expectedCommit) {
  if (!Buffer.isBuffer(raw)) {
    throw new Error(
      `provider tag ${tag} exact object bytes were not read as a Buffer`,
    );
  }
  const object = raw;
  const signatureBegin = Buffer.from("-----BEGIN PGP SIGNATURE-----\n");
  const signatureEnd = Buffer.from("-----END PGP SIGNATURE-----\n");
  const signatureOffset = object.indexOf(signatureBegin);
  if (
    signatureOffset <= 0 ||
    object[signatureOffset - 1] !== 0x0a ||
    object.indexOf(signatureBegin, signatureOffset + 1) !== -1 ||
    object.length < signatureEnd.length ||
    !object.subarray(object.length - signatureEnd.length).equals(signatureEnd)
  ) {
    throw new Error(
      `provider tag ${tag} does not contain one exact armored OpenPGP signature`,
    );
  }
  const payload = object.subarray(0, signatureOffset);
  const signature = object.subarray(signatureOffset);
  const expectedHeader = Buffer.from(
    `object ${expectedCommit}\ntype commit\ntag ${tag}\n`,
  );
  if (
    payload.length <= expectedHeader.length ||
    !payload.subarray(0, expectedHeader.length).equals(expectedHeader)
  ) {
    throw new Error(
      `provider tag ${tag} signed payload identity differs`,
    );
  }
  return { payload, signature };
}

function assertPinnedProviderGpgVerification(verification, tag) {
  const valid = asText(verification.output)
    .split("\n")
    .map((line) =>
      /^\[GNUPG:\] VALIDSIG ([0-9A-F]{40})\b/u.exec(line)?.[1],
    )
    .filter(Boolean);
  if (
    !verification.ok ||
    valid.length !== 1 ||
    valid[0] !== PROVIDER_SIGNER
  ) {
    throw new Error(
      `provider tag ${tag} is not signed by the pinned exact signer`,
    );
  }
  return PROVIDER_SIGNER;
}

function assertExactSignedProviderTag(
  context,
  { tag, expectedCommit, expectedObject },
) {
  const local = recoveryLocalTagOID(context, tag);
  const localType = local
    ? recoveryGit(
        context,
        ["cat-file", "-t", `refs/tags/${tag}`],
      ).trim()
    : "";
  const localCommit = local
    ? recoveryGit(
        context,
        ["rev-parse", `refs/tags/${tag}^{commit}`],
      ).trim()
    : "";
  if (
    local !== expectedObject ||
    localType !== "tag" ||
    localCommit !== expectedCommit
  ) {
    throw new Error(
      `provider recovery requires exact local annotated tag object ${expectedObject} -> ${expectedCommit}; observed ${JSON.stringify({
        object: local || null,
        type: localType || null,
        commit: localCommit || null,
      })}`,
    );
  }
  const remote = assertRecoveryExactRemoteTag(
    context,
    tag,
    expectedCommit,
    expectedObject,
  );
  const rawTagObject = recoveryGit(
    context,
    ["cat-file", "tag", expectedObject],
    { encoding: null },
  );
  const signed = splitSignedProviderTagObject(
    rawTagObject,
    tag,
    expectedCommit,
  );
  withTemporaryDirectory("takoform-provider-recovery-gpg", (gpgHome) => {
    chmodSync(gpgHome, 0o700);
    const gpgExecutable = trustedGpgExecutable();
    const gpgEnvironment = isolatedGpgEnvironment(gpgHome);
    const key = join(context.repo, "release/keys/provider-signing-key.asc");
    const inspect = command(
      context,
      gpgExecutable,
      [
        "--no-options",
        "--homedir",
        gpgHome,
        "--batch",
        "--no-tty",
        "--with-colons",
        "--import-options",
        "show-only",
        "--import",
        key,
      ],
      { env: gpgEnvironment },
    );
    const fingerprints = inspect
      .split("\n")
      .filter((line) => line.startsWith("fpr:"))
      .map((line) => line.split(":")[9])
      .filter(Boolean);
    if (
      fingerprints.length !== 1 ||
      fingerprints[0] !== PROVIDER_SIGNER
    ) {
      throw new Error("provider public signing key fingerprint drifted");
    }
    command(
      context,
      gpgExecutable,
      [
        "--no-options",
        "--homedir",
        gpgHome,
        "--batch",
        "--no-tty",
        "--import",
        key,
      ],
      { env: gpgEnvironment },
    );
    const payloadPath = join(gpgHome, "tag-payload");
    const signaturePath = join(gpgHome, "tag-signature.asc");
    writeFileSync(payloadPath, signed.payload, { mode: 0o600 });
    writeFileSync(signaturePath, signed.signature, { mode: 0o600 });
    const verification = attemptCommand(
      context,
      gpgExecutable,
      [
        "--no-options",
        "--homedir",
        gpgHome,
        "--batch",
        "--no-tty",
        "--no-auto-key-retrieve",
        "--status-fd",
        "1",
        "--verify",
        signaturePath,
        payloadPath,
      ],
      { env: gpgEnvironment },
    );
    try {
      assertPinnedProviderGpgVerification(verification, tag);
    } catch (error) {
      if (verification.output) context.stdout.write(verification.output);
      if (verification.stderr) context.stderr.write(verification.stderr);
      throw error;
    }
  });
  return {
    local: { object: local, type: localType, commit: localCommit },
    remote,
    signerFingerprint: PROVIDER_SIGNER,
  };
}

function ensureCandidateTagPublished(
  context,
  tag,
  expectedCommit,
  root,
  metadata,
) {
  assertTagAbsent(context, tag);
  const object = materializeTagObject(
    context,
    tag,
    expectedCommit,
    root,
    metadata,
  );
  pushExactTag(context, tag, expectedCommit, object);
  return object;
}

function pushExactTag(context, tag, expectedCommit, expectedObject) {
  progress(context, `push create-only tag ${tag}`);
  command(context, "git", [
    "push",
    "--no-verify",
    `--force-with-lease=refs/tags/${tag}:`,
    SOURCE_REPOSITORY,
    `refs/tags/${tag}:refs/tags/${tag}`,
  ], { env: gitPushEnvironment() });
  return assertExactRemoteTag(context, tag, expectedCommit, expectedObject);
}

function apiTagPath(tag) {
  return `repos/${GITHUB_REPOSITORY}/releases/tags/${encodeURIComponent(tag)}`;
}

function getRelease(context, tag, { absentOK = false } = {}) {
  const result = attemptCommand(context, "gh", ["api", apiTagPath(tag)]);
  if (result.ok) {
    return JSON.parse(result.output);
  }
  if (absentOK && /\bHTTP 404\b/u.test(result.stderr)) return null;
  if (result.output) context.stdout.write(result.output);
  if (result.stderr) context.stderr.write(result.stderr);
  throw new Error(`cannot read GitHub Release ${tag}`);
}

function releaseListArguments() {
  return [
    "api",
    "--paginate",
    "--slurp",
    `repos/${GITHUB_REPOSITORY}/releases?per_page=100`,
  ];
}

function parseReleasePages(raw, tag) {
  const pages = JSON.parse(raw);
  if (
    !Array.isArray(pages) ||
    pages.some((page) => !Array.isArray(page))
  ) {
    throw new Error("GitHub Release list pagination response is invalid");
  }
  const releases = pages.flat();
  const ids = new Set();
  for (const release of releases) {
    if (
      !release ||
      typeof release !== "object" ||
      Array.isArray(release) ||
      !Number.isSafeInteger(release.id) ||
      release.id <= 0 ||
      ids.has(release.id) ||
      typeof release.tag_name !== "string" ||
      typeof release.draft !== "boolean"
    ) {
      throw new Error("GitHub Release list contains an invalid identity");
    }
    ids.add(release.id);
  }
  return releases.filter((release) => release.tag_name === tag);
}

function releasesByTag(context, tag) {
  return parseReleasePages(
    command(context, "gh", releaseListArguments()),
    tag,
  );
}

function assertReleaseAbsent(context, tag) {
  const releases = releasesByTag(context, tag);
  if (releases.length !== 0) {
    throw new Error(
      `no-overwrite blocked ${tag}: GitHub Release identities already exist: ` +
        releases
          .map((release) => `${release.id}:${release.draft ? "draft" : "public"}`)
          .join(","),
    );
  }
}

function assertUniqueReleaseIdentity(context, tag, releaseId, draft) {
  const releases = releasesByTag(context, tag);
  if (
    releases.length !== 1 ||
    releases[0].id !== releaseId ||
    releases[0].draft !== draft
  ) {
    throw new Error(
      `no-overwrite blocked ${tag}: expected only release ${releaseId}:${draft ? "draft" : "public"}, observed ` +
        (releases.length === 0
          ? "none"
          : releases
              .map(
                (release) =>
                  `${release.id}:${release.draft ? "draft" : "public"}`,
              )
              .join(",")),
    );
  }
  return releases[0];
}

function expectedAssetMap(assets) {
  return new Map(
    [...assets.values()].map((asset) => [
      asset.name,
      { digest: asset.sha256, path: asset.path },
    ]),
  );
}

function validateReleaseReadback(
  release,
  tag,
  assets,
  {
    prerelease = false,
    expectedReleaseId,
    expectedName,
    expectedBody,
    expectedTargetCommitish,
    expectedAssetsURL,
    expectedUploadURL,
  } = {},
) {
  const expected = expectedAssetMap(assets);
  if (
    !Number.isSafeInteger(release.id) ||
    release.id <= 0 ||
    (expectedReleaseId !== undefined && release.id !== expectedReleaseId) ||
    release.tag_name !== tag ||
    (expectedName !== undefined && release.name !== expectedName) ||
    (expectedBody !== undefined && release.body !== expectedBody) ||
    (expectedTargetCommitish !== undefined &&
      release.target_commitish !== expectedTargetCommitish) ||
    (expectedAssetsURL !== undefined &&
      release.assets_url !== expectedAssetsURL) ||
    (expectedUploadURL !== undefined &&
      release.upload_url !== expectedUploadURL) ||
    release.draft !== false ||
    release.prerelease !== prerelease ||
    release.immutable !== true ||
    typeof release.html_url !== "string" ||
    !Array.isArray(release.assets) ||
    release.assets.length !== expected.size
  ) {
    throw new Error(
      `published release identity/state mismatch: ${JSON.stringify({
        id: release.id,
        tag_name: release.tag_name,
        draft: release.draft,
        prerelease: release.prerelease,
        immutable: release.immutable,
        assets: release.assets?.length,
      })}`,
    );
  }
  const seen = new Set();
  for (const remote of release.assets) {
    const local = expected.get(remote.name);
    if (
      !local ||
      seen.has(remote.name) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.digest ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0
    ) {
      throw new Error(`published asset mismatch: ${JSON.stringify(remote)}`);
    }
    seen.add(remote.name);
  }
  return release;
}

function downloadAndCompareRelease(context, tag, assets, parent) {
  const output = join(parent, "public-readback");
  mkdirSync(output);
  command(context, "gh", [
    "release",
    "download",
    tag,
    "--repo",
    GITHUB_REPOSITORY,
    "--dir",
    output,
  ]);
  const names = listRegularFiles(output);
  const expected = [...assets.keys()].sort();
  if (JSON.stringify(names) !== JSON.stringify(expected)) {
    throw new Error(
      `downloaded release inventory differs: ${names.join(", ")}`,
    );
  }
  for (const [name, asset] of assets) {
    if (fileDigest(join(output, name)) !== asset.sha256) {
      throw new Error(`downloaded release digest mismatch: ${name}`);
    }
  }
}

function validateDraftBeforePublication(
  draft,
  { releaseId, tag, prerelease, assets },
) {
  if (
    draft.id !== releaseId ||
    draft.draft !== true ||
    draft.prerelease !== prerelease ||
    draft.tag_name !== tag ||
    !Array.isArray(draft.assets) ||
    draft.assets.length !== assets.size
  ) {
    throw new Error("draft changed identity before publication");
  }
  const draftAssets = new Map();
  const assetIDs = new Set();
  for (const remote of draft.assets) {
    const local = assets.get(remote.name);
    if (
      !local ||
      draftAssets.has(remote.name) ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0 ||
      assetIDs.has(remote.id) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.sha256 ||
      remote.size !== lstatSync(local.path).size
    ) {
      throw new Error(
        "draft API asset identity differs from the same-run candidate",
      );
    }
    draftAssets.set(remote.name, remote);
    assetIDs.add(remote.id);
  }
}

function readExactRetainedDraft(
  context,
  {
    releaseId,
    tag,
    prerelease = false,
    body,
    assets,
    requireComplete = false,
  },
) {
  assertUniqueReleaseIdentity(context, tag, releaseId, true);
  const draft = JSON.parse(
    command(context, "gh", [
      "api",
      `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
    ]),
  );
  const expectedUploadURL =
    `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets{?name,label}`;
  const expectedAssetsURL =
    `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets`;
  if (
    draft.id !== releaseId ||
    draft.tag_name !== tag ||
    draft.target_commitish !== "main" ||
    draft.name !== tag ||
    draft.body !== body ||
    draft.draft !== true ||
    draft.prerelease !== prerelease ||
    draft.immutable !== false ||
    draft.published_at !== null ||
    draft.assets_url !== expectedAssetsURL ||
    draft.upload_url !== expectedUploadURL ||
    !Array.isArray(draft.assets)
  ) {
    throw new Error(
      `retained draft ${releaseId} identity/state differs from the exact recovery input`,
    );
  }
  const seenNames = new Set();
  const seenIDs = new Set();
  for (const remote of draft.assets) {
    const local = assets.get(remote.name);
    if (
      !local ||
      seenNames.has(remote.name) ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0 ||
      seenIDs.has(remote.id) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.sha256 ||
      remote.size !== lstatSync(local.path).size
    ) {
      throw new Error(
        `retained draft ${releaseId} contains an unknown, duplicate, or drifted asset`,
      );
    }
    seenNames.add(remote.name);
    seenIDs.add(remote.id);
  }
  if (requireComplete && seenNames.size !== assets.size) {
    throw new Error(
      `retained draft ${releaseId} does not contain the complete exact candidate`,
    );
  }
  return {
    draft,
    missing: [...assets.values()]
      .filter((asset) => !seenNames.has(asset.name))
      .sort((left, right) => compareNames(left.name, right.name)),
  };
}

function validateExactReleasePatchResponse(
  response,
  { releaseId, tag, prerelease, body, assets },
) {
  const expectedAssetsURL =
    `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets`;
  const expectedUploadURL =
    `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets{?name,label}`;
  if (
    response.id !== releaseId ||
    response.tag_name !== tag ||
    response.target_commitish !== "main" ||
    response.name !== tag ||
    response.body !== body ||
    response.draft !== false ||
    response.prerelease !== prerelease ||
    response.assets_url !== expectedAssetsURL ||
    response.upload_url !== expectedUploadURL ||
    !Array.isArray(response.assets) ||
    response.assets.length !== assets.size
  ) {
    throw new Error(
      `release ${releaseId} PATCH response differs from the exact retained draft identity`,
    );
  }
  const seenNames = new Set();
  const seenIDs = new Set();
  for (const remote of response.assets) {
    const local = assets.get(remote.name);
    if (
      !local ||
      seenNames.has(remote.name) ||
      !Number.isSafeInteger(remote.id) ||
      remote.id <= 0 ||
      seenIDs.has(remote.id) ||
      remote.state !== "uploaded" ||
      remote.digest !== local.sha256 ||
      remote.size !== lstatSync(local.path).size
    ) {
      throw new Error(
        `release ${releaseId} PATCH response asset closure differs from the exact candidate`,
      );
    }
    seenNames.add(remote.name);
    seenIDs.add(remote.id);
  }
  return response;
}

function reportRetainedDraftFailure(
  context,
  tag,
  releaseId,
  { surface = FORM_SURFACE } = {},
) {
  let mutationState = "REMOTE_STATE_UNREADABLE";
  let observedReleaseIDs = [];
  const listed = attemptCommand(context, "gh", releaseListArguments());
  if (listed.ok) {
    try {
      const matches = parseReleasePages(listed.output, tag);
      observedReleaseIDs = matches.map((release) => release.id);
      if (matches.length !== 1 || matches[0].id !== releaseId) {
        mutationState = "REMOTE_STATE_AMBIGUOUS";
      } else if (matches[0].draft === true) {
        mutationState = "MATCHING_DRAFT_RETAINED";
      } else {
        mutationState = "PUBLICATION_INDETERMINATE";
      }
    } catch {
      mutationState = "REMOTE_STATE_UNREADABLE";
    }
  }
  context.stderr.write(
    `${JSON.stringify({
      kind: "takos.deploy-failure@v1",
      surface,
      phase: "recover-draft",
      tag,
      releaseId,
      observedReleaseIDs,
      mutationState,
      instruction:
        "read the authoritative tag/release state; rerun recover-draft only if the same exact draft is retained",
    })}\n`,
  );
}

function resumeDraftReleaseLocally(
  context,
  {
    releaseId,
    tag,
    assets,
    body,
    prerelease = false,
    temporaryRoot,
    surface = FORM_SURFACE,
    prePublishFence = () => {},
  },
) {
  const expectedAssetsURL =
    `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets`;
  const expectedUploadURL =
    `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/` +
    `${releaseId}/assets{?name,label}`;
  try {
    const initial = readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
    });
    progress(
      context,
      `resume exact draft ${releaseId}: upload ${initial.missing.length} missing assets`,
    );
    for (const asset of initial.missing) {
      const uploaded = JSON.parse(
        command(context, "gh", [
          "api",
          "--method",
          "POST",
          `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets?name=${encodeURIComponent(asset.name)}`,
          "--header",
          "Content-Type: application/octet-stream",
          "--input",
          asset.path,
        ], { env: githubUploadEnvironment() }),
      );
      if (
        !Number.isSafeInteger(uploaded.id) ||
        uploaded.id <= 0 ||
        uploaded.name !== asset.name ||
        uploaded.state !== "uploaded" ||
        uploaded.digest !== asset.sha256 ||
        uploaded.size !== lstatSync(asset.path).size
      ) {
        throw new Error(
          `exact retained draft ${releaseId} asset upload mismatch: ${asset.name}`,
        );
      }
    }
    readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
      requireComplete: true,
    });
    prePublishFence(releaseId);
    readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
      requireComplete: true,
    });
    progress(context, `publish exact retained draft ${releaseId}`);
    let patchResponse;
    try {
      patchResponse = JSON.parse(
        command(context, "gh", [
          "api",
          "--method",
          "PATCH",
          `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
          "-f",
          `tag_name=${tag}`,
          "-f",
          "target_commitish=main",
          "-f",
          `name=${tag}`,
          "-f",
          `body=${body}`,
          "-F",
          "draft=false",
          "-F",
          `prerelease=${prerelease}`,
          "-f",
          "make_latest=false",
        ]),
      );
    } catch (error) {
      const observed = attemptCommand(context, "gh", [
        "api",
        apiTagPath(tag),
      ]);
      if (!observed.ok) throw error;
      let release;
      try {
        release = JSON.parse(observed.output);
      } catch {
        throw error;
      }
      if (release.id !== releaseId || release.draft !== false) {
        throw error;
      }
      patchResponse = release;
    }
    validateExactReleasePatchResponse(patchResponse, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
    });
    let release;
    let published = false;
    for (let attempt = 1; attempt <= 12; attempt += 1) {
      release = getRelease(context, tag);
      if (
        release.id === releaseId &&
        release.draft === false &&
        release.immutable === true
      ) {
        published = true;
        break;
      }
      if (attempt < 12) context.wait(1000);
    }
    if (!published) {
      throw new Error(
        `release ${releaseId} did not converge to immutable exact publication`,
      );
    }
    validateReleaseReadback(release, tag, assets, {
      prerelease,
      expectedReleaseId: releaseId,
      expectedName: tag,
      expectedBody: body,
      expectedTargetCommitish: "main",
      expectedAssetsURL,
      expectedUploadURL,
    });
    downloadAndCompareRelease(context, tag, assets, temporaryRoot);
    assertUniqueReleaseIdentity(context, tag, releaseId, false);
    return release;
  } catch (error) {
    reportRetainedDraftFailure(context, tag, releaseId, { surface });
    throw error;
  }
}

function publishReleaseLocally(
  context,
  {
    tag,
    assets,
    body,
    prerelease = false,
    temporaryRoot,
    strictIdentity = false,
    prePublishFence = () => {},
  },
) {
  assertReleaseAbsent(context, tag);
  let releaseId = null;
  let published = false;
  let draftCreationAttempted = false;
  try {
    progress(context, `create local-authority draft for ${tag}`);
    draftCreationAttempted = true;
    const created = JSON.parse(
      command(context, "gh", [
        "api",
        "--method",
        "POST",
        `repos/${GITHUB_REPOSITORY}/releases`,
        "-f",
        `tag_name=${tag}`,
        ...(strictIdentity ? ["-f", "target_commitish=main"] : []),
        "-f",
        `name=${tag}`,
        "-F",
        "draft=true",
        "-F",
        `prerelease=${prerelease}`,
        "-f",
        `body=${body}`,
        "-f",
        "make_latest=false",
      ]),
    );
    if (
      !Number.isSafeInteger(created.id) ||
      created.id <= 0 ||
      created.tag_name !== tag ||
      created.draft !== true ||
      created.upload_url !==
        `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${created.id}/assets{?name,label}`
    ) {
      throw new Error(`draft creation returned an unexpected identity`);
    }
    releaseId = created.id;
    const orderedAssets = [...assets.values()].sort((left, right) =>
      compareNames(left.name, right.name),
    );
    if (strictIdentity) {
      const initial = readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
      });
      if (
        initial.draft.assets.length !== 0 ||
        initial.missing.length !== assets.size
      ) {
        throw new Error(
          `new exact draft ${releaseId} is not empty before upload`,
        );
      }
    }
    progress(context, `upload ${orderedAssets.length} exact assets`);
    for (const asset of orderedAssets) {
      const uploaded = JSON.parse(
        command(context, "gh", [
          "api",
          "--method",
          "POST",
          `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets?name=${encodeURIComponent(asset.name)}`,
          "--header",
          "Content-Type: application/octet-stream",
          "--input",
          asset.path,
        ], { env: githubUploadEnvironment() }),
      );
      if (
        !Number.isSafeInteger(uploaded.id) ||
        uploaded.id <= 0 ||
        uploaded.name !== asset.name ||
        uploaded.state !== "uploaded" ||
        uploaded.digest !== asset.sha256 ||
        uploaded.size !== lstatSync(asset.path).size
      ) {
        throw new Error(
          `exact release ${releaseId} asset upload mismatch: ${asset.name}`,
        );
      }
    }
    if (strictIdentity) {
      readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        requireComplete: true,
      });
    } else {
      const draft = JSON.parse(
        command(context, "gh", [
          "api",
          `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
        ]),
      );
      validateDraftBeforePublication(draft, {
        releaseId,
        tag,
        prerelease,
        assets,
      });
    }
    prePublishFence(releaseId);
    if (strictIdentity) {
      readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        requireComplete: true,
      });
    } else {
      assertUniqueReleaseIdentity(context, tag, releaseId, true);
    }
    progress(context, `publish exact draft ${releaseId}`);
    const patchResponse = JSON.parse(
      command(context, "gh", [
        "api",
        "--method",
        "PATCH",
        `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
        ...(strictIdentity
          ? [
              "-f",
              `tag_name=${tag}`,
              "-f",
              "target_commitish=main",
              "-f",
              `name=${tag}`,
              "-f",
              `body=${body}`,
              "-F",
              `prerelease=${prerelease}`,
            ]
          : []),
        "-F",
        "draft=false",
        "-f",
        "make_latest=false",
      ]),
    );
    if (strictIdentity) {
      validateExactReleasePatchResponse(patchResponse, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
      });
    }
    let release;
    for (let attempt = 1; attempt <= 12; attempt += 1) {
      release = getRelease(context, tag);
      if (
        release.id === releaseId &&
        release.draft === false &&
        release.immutable === true
      ) {
        published = true;
        break;
      }
      if (attempt < 12) {
        context.wait(1000);
      }
    }
    if (!published) {
      throw new Error(
        `release ${releaseId} did not converge to immutable exact publication`,
      );
    }
    validateReleaseReadback(release, tag, assets, {
      prerelease,
      ...(strictIdentity
        ? {
            expectedReleaseId: releaseId,
            expectedName: tag,
            expectedBody: body,
            expectedTargetCommitish: "main",
            expectedAssetsURL:
              `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets`,
            expectedUploadURL:
              `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${releaseId}/assets{?name,label}`,
          }
        : {}),
    });
    downloadAndCompareRelease(context, tag, assets, temporaryRoot);
    assertUniqueReleaseIdentity(context, tag, releaseId, false);
    return release;
  } catch (error) {
    let state = draftCreationAttempted
      ? "DRAFT_CREATION_INDETERMINATE"
      : "BEFORE_DRAFT";
    let observedReleaseIDs = [];
    if (releaseId !== null) {
      const observed = attemptCommand(context, "gh", [
        "api",
        `repos/${GITHUB_REPOSITORY}/releases/${releaseId}`,
      ]);
      if (observed.ok) {
        let release;
        try {
          release = JSON.parse(observed.output);
        } catch {
          release = null;
        }
        if (!release || typeof release !== "object" || Array.isArray(release)) {
          state = "REMOTE_STATE_UNREADABLE";
        } else if (release.tag_name === tag && release.draft === true) {
          state = "MATCHING_DRAFT_RETAINED";
        } else {
          state = release.draft === false ? "PUBLICATION_INDETERMINATE" : "DRAFT_INDETERMINATE";
        }
      } else {
        state = "REMOTE_STATE_UNREADABLE";
      }
    }
    if (draftCreationAttempted) {
      const observed = attemptCommand(
        context,
        "gh",
        releaseListArguments(),
      );
      if (observed.ok) {
        try {
          const matches = parseReleasePages(observed.output, tag);
          observedReleaseIDs = matches.map((release) => release.id);
          if (matches.length > 1) {
            state = "REMOTE_STATE_AMBIGUOUS";
          } else if (
            matches.length === 1 &&
            releaseId !== null &&
            matches[0].id !== releaseId
          ) {
            state = "REMOTE_STATE_AMBIGUOUS";
          } else if (
            releaseId === null &&
            matches.length === 1 &&
            matches[0].draft === true
          ) {
            state = "MATCHING_DRAFT_RETAINED";
          } else if (releaseId === null && matches.length === 1) {
            state = "PUBLICATION_INDETERMINATE";
          }
        } catch {
          state = "REMOTE_STATE_UNREADABLE";
        }
      } else {
        state = "REMOTE_STATE_UNREADABLE";
      }
    }
    context.stderr.write(
      `${JSON.stringify({
        kind: "takos.deploy-failure@v1",
        surface: tag.startsWith("v") ? PROVIDER_SURFACE : FORM_SURFACE,
        tag,
        releaseId,
        observedReleaseIDs,
        mutationState: state,
        instruction:
          state.includes("INDETERMINATE") ||
          state.includes("UNREADABLE") ||
          state.includes("AMBIGUOUS") ||
          state === "MATCHING_DRAFT_RETAINED"
            ? "read the authoritative tag/release state; do not retry blindly"
            : "production release was not published",
      })}\n`,
    );
    throw error;
  }
}

function verifyNamedChecksums(root, checksumName, expectedTargets) {
  const entries = parseStrictChecksums(
    readFileSync(join(root, checksumName), "utf8"),
  );
  const expected = [...expectedTargets].sort();
  const actual = [...entries.keys()].sort();
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    throw new Error(
      `${checksumName} target set differs; expected ${expected.join(", ")}, got ${actual.join(", ")}`,
    );
  }
  for (const [name, digest] of entries) {
    if (fileDigest(join(root, name)) !== digest) {
      throw new Error(`${checksumName} digest mismatch for ${name}`);
    }
  }
}

function registryVersions(context) {
  const raw = command(context, "curl", [
    "--fail-with-body",
    "--silent",
    "--show-error",
    "--location",
    "https://registry.terraform.io/v1/providers/tako0614/takoform/versions",
  ]);
  const value = JSON.parse(raw);
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    !Array.isArray(value.versions)
  ) {
    throw new Error("Terraform Registry versions response is invalid");
  }
  const versions = value.versions.map((entry) => entry?.version);
  if (
    versions.some(
      (version) =>
        typeof version !== "string" ||
        !PROVIDER_TAG.test(`v${version}`),
    ) ||
    new Set(versions).size !== versions.length
  ) {
    throw new Error(
      "Terraform Registry versions response contains invalid or duplicate identities",
    );
  }
  return new Set(versions);
}

function assertRegistryVersionAbsent(context, version) {
  if (registryVersions(context).has(version)) {
    throw new Error(
      `no-overwrite blocked provider ${version}: Terraform Registry version identity already exists`,
    );
  }
}

function assertReleaseImmutabilityEnabled(context) {
  const value = JSON.parse(
    command(context, "gh", [
      "api",
      `repos/${GITHUB_REPOSITORY}/immutable-releases`,
    ]),
  );
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    value.enabled !== true
  ) {
    throw new Error(
      "provider recovery requires repository immutable releases to be enabled",
    );
  }
}

function providerAssetNames(descriptor) {
  const base = `terraform-provider-takoform_${descriptor.version}`;
  const archives = descriptor.platforms
    .map((platform) => `${base}_${platform}.zip`)
    .sort();
  const payload = [
    ...archives,
    ...archives.map((name) => `${name}.spdx.json`),
    `${base}_manifest.json`,
    `${base}_SHA256SUMS`,
    `${base}_SHA256SUMS.sig`,
  ].sort();
  const provenance = `${base}_provenance.intoto.json`;
  const provenanceSignature = `${provenance}.sig`;
  return {
    archives,
    checksum: `${base}_SHA256SUMS`,
    signature: `${base}_SHA256SUMS.sig`,
    manifest: `${base}_manifest.json`,
    payload,
    provenance,
    provenanceSignature,
    all: [...payload, provenance, provenanceSignature].sort(),
  };
}

function verifyProviderReleaseProvenance(context, root, descriptor, expected) {
  const names = providerAssetNames(descriptor);
  const raw = command(context, "go", [
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
    expected.sourceCommit,
    "--expected-tooling-commit",
    expected.toolingCommit,
    "--expected-request-id",
    expected.requestId,
    "--expected-run-id",
    expected.runId,
    "--expected-run-attempt",
    expected.runAttempt,
    "--expected-tag-object-oid",
    expected.tagObjectOid,
    "--expected-tag-object-sha256",
    expected.tagObjectSha256,
  ]);
  const report = JSON.parse(raw);
  if (
    report.kind !==
      "takoform.provider-release-provenance-verification@v1" ||
    report.tag !== descriptor.tag ||
    report.provenance !== names.provenance ||
    report.subjectCount !== 13 ||
    report.verified !== true ||
    report.signerFingerprint !== PROVIDER_SIGNER
  ) {
    throw new Error("provider release provenance verification report drifted");
  }
  return report;
}

function verifyProviderCandidate(
  context,
  root,
  {
    descriptor,
    runId,
    runAttempt,
    requestId,
    expectedCommit,
    tagObjectOid,
  },
) {
  validateProviderIdentityLedger(context.repo, descriptor);
  const metadata = verifyCandidateRoot(root, {
    metadataProfile: "pretty-required-lf",
  });
  requireExactKeys(
    metadata,
    [
      "format",
      "repository",
      "workflowPath",
      "workflowRef",
      "runId",
      "attempt",
      "requestId",
      "tag",
      "sourceCommit",
      "toolingCommit",
      "tagObjectOid",
      "tagObjectSha256",
      "assetCount",
      "assets",
    ],
    "provider release candidate metadata",
  );
  if (
    metadata.format !== "takoform.provider-release-candidate.v1" ||
    metadata.repository !== GITHUB_REPOSITORY ||
    metadata.workflowPath !== ".github/workflows/release.yml" ||
    metadata.workflowRef !==
      `${GITHUB_REPOSITORY}/.github/workflows/release.yml@refs/tags/${descriptor.tag}` ||
    normalizedPositiveInteger(metadata.runId, "metadata runId") !== runId ||
    normalizedPositiveInteger(metadata.attempt, "metadata attempt") !==
      runAttempt ||
    metadata.requestId !== requestId ||
    metadata.tag !== descriptor.tag ||
    metadata.sourceCommit !== expectedCommit ||
    metadata.toolingCommit !== expectedCommit ||
    metadata.tagObjectOid !== tagObjectOid ||
    !SHA256.test(metadata.tagObjectSha256 ?? "") ||
    metadata.assetCount !== 15
  ) {
    throw new Error("provider release candidate metadata binding mismatch");
  }
  const tagObject = command(context, "git", ["cat-file", "tag", tagObjectOid]);
  if (sha256(Buffer.from(tagObject, "utf8")) !== metadata.tagObjectSha256) {
    throw new Error("provider candidate tag-object digest binding mismatch");
  }
  const names = providerAssetNames(descriptor);
  const assets = validateCandidateAssets(root, metadata, names.all);
  const assetRoot = join(root, "assets");
  verifyNamedChecksums(assetRoot, names.checksum, [
    ...names.archives,
    names.manifest,
  ]);
  verifyProviderSignature(context, assetRoot, names);
  verifyProviderReleaseProvenance(context, assetRoot, descriptor, {
    sourceCommit: expectedCommit,
    toolingCommit: expectedCommit,
    requestId,
    runId,
    runAttempt,
    tagObjectOid,
    tagObjectSha256: metadata.tagObjectSha256,
  });
  command(context, "go", [
    "-C",
    "cmd/provider-release",
    "run",
    ".",
    "verify-sbom",
    ...names.archives.map((name) => join(assetRoot, `${name}.spdx.json`)),
  ]);
  return assets;
}

function verifyProviderSignature(context, root, names) {
  withTemporaryDirectory("takoform-provider-gpg", (gpgHome) => {
    chmodSync(gpgHome, 0o700);
    const key = join(context.repo, "release/keys/provider-signing-key.asc");
    const inspect = command(context, "gpg", [
      "--homedir",
      gpgHome,
      "--batch",
      "--with-colons",
      "--import-options",
      "show-only",
      "--import",
      key,
    ]);
    const fingerprints = inspect
      .split("\n")
      .filter((line) => line.startsWith("fpr:"))
      .map((line) => line.split(":")[9])
      .filter(Boolean);
    if (
      fingerprints.length !== 1 ||
      fingerprints[0] !== PROVIDER_SIGNER
    ) {
      throw new Error("provider public signing key fingerprint drifted");
    }
    command(context, "gpg", [
      "--homedir",
      gpgHome,
      "--batch",
      "--import",
      key,
    ]);
    for (const [label, signature, subject] of [
      ["provider checksum", names.signature, names.checksum],
      [
        "provider release provenance",
        names.provenanceSignature,
        names.provenance,
      ],
    ]) {
      const status = command(
        context,
        "gpg",
        [
          "--homedir",
          gpgHome,
          "--batch",
          "--status-fd",
          "1",
          "--verify",
          join(root, signature),
          join(root, subject),
        ],
        { label: `verify ${label} signature` },
      );
      if (!status.includes(`[GNUPG:] VALIDSIG ${PROVIDER_SIGNER} `)) {
        throw new Error(`${label} signature did not return the pinned VALIDSIG`);
      }
    }
  });
}

function publicAssets(root, names) {
  return new Map(
    names.map((name) => [
      name,
      { name, sha256: fileDigest(join(root, name)), path: join(root, name) },
    ]),
  );
}

export function providerReleaseBody(descriptor) {
  if (
    descriptor?.version !== PROVIDER_VERSION ||
    descriptor?.tag !== `v${PROVIDER_VERSION}` ||
    descriptor?.versioning?.portableApiVersion !== PROVIDER_HOST_API
  ) {
    throw new Error(
      "provider release body requires the exact v2.1.1 Beta Host API descriptor",
    );
  }
  return (
    "Signed deterministic Takoform provider v2.1.1 release. Provider publication does not publish, mature, activate, or make any Form commercially available.\n\nBreaking upgrade from provider v1: Provider v2.1.1 retains the published provider-v2 state compatibility lane and its historical Host API epoch forms.takoform.com/v1alpha2. Provider-v1 state remains a breaking boundary and is rejected instead of being rewritten; follow the explicit create/import/cutover migration guide: " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/release/migrations/v1-to-v2.md` +
    "\n\nThis provider release targets the Beta Host API `forms.takoform.com/v1beta1` and the Edge Platform Family `edge.forms.takoform.com/v1beta1`. Provider SemVer, Host API, Form Family, Form definition, and Form Package versions are independent axes. The exact 15 Experimental 0.1.0 FormRefs and package digests are locked in " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/${PROVIDER_IDENTITY_LEDGER}` +
    "."
  );
}

function providerPrepare(context, options, descriptor) {
  const commit = assertCurrentProtectedMain(context, options["expected-commit"]);
  assertTagAbsent(context, descriptor.tag);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  ownerGateAndFence(context, commit);
  const dispatched = dispatchWorkflow(context, "provider-release-tag.yml", {
    tag: descriptor.tag,
    expected_commit: commit,
  });
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: PROVIDER_SURFACE,
    phase: "prepare",
    target: PROVIDER_ADDRESS,
    commit,
    tag: descriptor.tag,
    dispatchStatus: "DISPATCHED",
    status: "AWAITING_REVIEW",
    workflowRun: dispatched,
  });
}

function providerTag(context, options, descriptor) {
  const expectedCommit = options["expected-commit"];
  assertCurrentProtectedMain(context, expectedCommit);
  assertTagAbsent(context, descriptor.tag);
  ownerGateAndFence(context, expectedCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Author provider release tag",
      headSha: expectedCommit,
    },
  );
  return withTemporaryDirectory("takoform-provider-tag", (temporaryRoot) => {
    const preflight = downloadArtifact(
      context,
      options["run-id"],
      `provider-tag-preflight-${run.displayTitle}-${options["run-id"]}-${options["run-attempt"]}-${expectedCommit}`,
      join(temporaryRoot, "preflight"),
    );
    const signedTag = downloadArtifact(
      context,
      options["run-id"],
      `provider-signed-tag-${run.displayTitle}-${options["run-id"]}-${options["run-attempt"]}`,
      join(temporaryRoot, "signed-tag"),
    );
    let localObject = "";
    try {
      assertCurrentProtectedMain(context, expectedCommit);
      progress(context, "verify and materialize exact signed provider tag");
      command(
        context,
        "go",
        [
          "-C",
          "./cmd/provider-release",
          "run",
          ".",
          "verify-tag-artifact",
          "--artifact",
          signedTag,
          "--preflight-artifact",
          preflight,
          "--expected-run-id",
          options["run-id"],
          "--expected-run-attempt",
          options["run-attempt"],
          "--expected-request-id",
          run.displayTitle,
          "--expected-commit",
          expectedCommit,
          "--materialize-ref",
        ],
        { echo: true },
      );
      localObject = localTagOID(context, descriptor.tag);
      if (!localObject) throw new Error("provider tag verifier created no local ref");
      ownerGateAndFence(context, expectedCommit);
      assertReleaseAbsent(context, descriptor.tag);
      assertRegistryVersionAbsent(context, descriptor.version);
      pushExactTag(context, descriptor.tag, expectedCommit, localObject);
      ownerGateAndFence(context, expectedCommit);
      const dispatched = dispatchWorkflow(
        context,
        "release.yml",
        {
          tag: descriptor.tag,
          expected_commit: expectedCommit,
        },
        {
          headSha: expectedCommit,
          ref: descriptor.tag,
          headBranch: descriptor.tag,
        },
      );
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: PROVIDER_SURFACE,
        phase: "tag",
        commit: expectedCommit,
        tag: descriptor.tag,
        tagObject: localObject,
        reviewedRun: { id: options["run-id"], attempt: options["run-attempt"], url: run.url },
        dispatchStatus: "DISPATCHED",
        status: "AWAITING_REVIEW",
        releaseCandidateRun: dispatched,
      });
    } catch (error) {
      reportTagFailure(context, descriptor.tag, localObject, {
        surface: PROVIDER_SURFACE,
        phase: "tag",
      });
      throw error;
    }
  });
}

function providerPublish(context, options, descriptor) {
  const expectedCommit = options["expected-commit"];
  const releaseBody = providerReleaseBody(descriptor);
  if (
    !releaseBody.includes("Provider v2.1.1") ||
    !releaseBody.includes(PROVIDER_HOST_API) ||
    !releaseBody.includes(PROVIDER_FORM_FAMILY) ||
    !releaseBody.includes(PROVIDER_IDENTITY_LEDGER) ||
    !releaseBody.includes("15 Experimental") ||
    !releaseBody.includes("release/migrations/v1-to-v2.md") ||
    !releaseBody.includes("Breaking upgrade from provider v1") ||
    !releaseBody.includes("forms.takoform.com/v1alpha2")
  ) {
    throw new Error("provider release body omits the exact v2.1 Beta identity contract");
  }
  assertCurrentProtectedMain(context, expectedCommit);
  const localObject = localTagOID(context, descriptor.tag);
  if (!localObject) throw new Error(`local signed provider tag ${descriptor.tag} is missing`);
  assertExactRemoteTag(context, descriptor.tag, expectedCommit, localObject);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  ownerGateAndFence(context, expectedCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare provider release candidate",
      headSha: expectedCommit,
      headBranch: descriptor.tag,
    },
  );
  return withTemporaryDirectory("takoform-provider-publish", (temporaryRoot) => {
    const candidate = downloadArtifact(
      context,
      options["run-id"],
      `provider-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
      join(temporaryRoot, "candidate"),
    );
    const assets = verifyProviderCandidate(context, candidate, {
      descriptor,
      runId: options["run-id"],
      runAttempt: options["run-attempt"],
      requestId: run.displayTitle,
      expectedCommit,
      tagObjectOid: localObject,
    });
    ownerGateAndFence(context, expectedCommit);
    assertRegistryVersionAbsent(context, descriptor.version);
    const release = publishReleaseLocally(context, {
      tag: descriptor.tag,
      assets,
      prerelease: descriptor.version.includes("-"),
      body: releaseBody,
      temporaryRoot,
      prePublishFence: () => {
        ownerGateAndFence(context, expectedCommit);
        assertRegistryVersionAbsent(context, descriptor.version);
      },
    });
    return emit(context, {
      kind: "takos.deploy-result@v1",
      surface: PROVIDER_SURFACE,
      phase: "publish",
      target: `github-release:${GITHUB_REPOSITORY}/${descriptor.tag}`,
      commit: expectedCommit,
      tag: descriptor.tag,
      candidateRun: { id: options["run-id"], attempt: options["run-attempt"], url: run.url },
      releaseId: release.id,
      releaseUrl: release.html_url,
      assetDigests: Object.fromEntries(
        [...assets].map(([name, asset]) => [name, asset.sha256]),
      ),
      productionReadback: "EXACT_IMMUTABLE_RELEASE",
      status: "VERIFIED",
    });
  });
}

function providerRecoveryMutationFence(
  context,
  {
    descriptor,
    releaseCommit,
    recoveryCommit,
    expectedObject,
    releaseId,
    label,
  },
) {
  const currentMain = ownerGateAndFence(context, recoveryCommit);
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: currentMain,
    label,
  });
  const tag = assertExactSignedProviderTag(context, {
    tag: descriptor.tag,
    expectedCommit: releaseCommit,
    expectedObject,
  });
  assertRegistryVersionAbsent(context, descriptor.version);
  assertReleaseImmutabilityEnabled(context);
  if (releaseId === undefined) {
    assertReleaseAbsent(context, descriptor.tag);
  } else {
    assertUniqueReleaseIdentity(
      context,
      descriptor.tag,
      releaseId,
      true,
    );
  }
  return { currentMain, tag };
}

function providerRecoverTagOnly(context, options, descriptor) {
  const releaseCommit = options["expected-release-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const initialMain = assertCurrentProtectedMain(context, recoveryCommit);
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: initialMain,
    label: "provider tag-only reviewed recovery fence",
  });
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare provider release candidate",
      headSha: releaseCommit,
      headBranch: descriptor.tag,
    },
  );
  return withTemporaryDirectory(
    "takoform-provider-tag-only-recovery",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `provider-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const assets = verifyProviderCandidate(context, candidate, {
        descriptor,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit: releaseCommit,
        tagObjectOid: expectedObject,
      });
      try {
        providerRecoveryMutationFence(context, {
          descriptor,
          releaseCommit,
          recoveryCommit,
          expectedObject,
          label: "provider tag-only pre-draft recovery fence",
        });
        const release = publishReleaseLocally(context, {
          tag: descriptor.tag,
          assets,
          prerelease: descriptor.version.includes("-"),
          body: providerReleaseBody(descriptor),
          temporaryRoot,
          strictIdentity: true,
          prePublishFence: (releaseId) =>
            providerRecoveryMutationFence(context, {
              descriptor,
              releaseCommit,
              recoveryCommit,
              expectedObject,
              releaseId,
              label: "provider tag-only pre-publication recovery fence",
            }),
        });
        assertExactSignedProviderTag(context, {
          tag: descriptor.tag,
          expectedCommit: releaseCommit,
          expectedObject,
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: PROVIDER_SURFACE,
          phase: "recover-tag-only",
          target: `github-release:${GITHUB_REPOSITORY}/${descriptor.tag}`,
          commit: recoveryCommit,
          releaseCommit,
          candidateToolingCommit: releaseCommit,
          recoveryCommit,
          tag: descriptor.tag,
          tagObject: expectedObject,
          signerFingerprint: PROVIDER_SIGNER,
          recoveredFrom: "EXACT_SIGNED_TAG_PRESENT_RELEASE_ABSENT",
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, descriptor.tag, expectedObject, {
          surface: PROVIDER_SURFACE,
          phase: "recover-tag-only",
        });
        throw error;
      }
    },
  );
}

function providerRecoverDraft(context, options, descriptor) {
  const releaseCommit = options["expected-release-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const releaseId = Number(options["release-id"]);
  const initialMain = assertCurrentProtectedMain(context, recoveryCommit);
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: initialMain,
    label: "provider retained-draft reviewed recovery fence",
  });
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare provider release candidate",
      headSha: releaseCommit,
      headBranch: descriptor.tag,
    },
  );
  return withTemporaryDirectory(
    "takoform-provider-draft-recovery",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `provider-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const assets = verifyProviderCandidate(context, candidate, {
        descriptor,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit: releaseCommit,
        tagObjectOid: expectedObject,
      });
      try {
        providerRecoveryMutationFence(context, {
          descriptor,
          releaseCommit,
          recoveryCommit,
          expectedObject,
          releaseId,
          label: "provider retained-draft mutation recovery fence",
        });
        const release = resumeDraftReleaseLocally(context, {
          releaseId,
          tag: descriptor.tag,
          assets,
          prerelease: descriptor.version.includes("-"),
          body: providerReleaseBody(descriptor),
          temporaryRoot,
          surface: PROVIDER_SURFACE,
          prePublishFence: (retainedReleaseId) =>
            providerRecoveryMutationFence(context, {
              descriptor,
              releaseCommit,
              recoveryCommit,
              expectedObject,
              releaseId: retainedReleaseId,
              label:
                "provider retained-draft pre-publication recovery fence",
            }),
        });
        assertExactSignedProviderTag(context, {
          tag: descriptor.tag,
          expectedCommit: releaseCommit,
          expectedObject,
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: PROVIDER_SURFACE,
          phase: "recover-draft",
          target: `github-release:${GITHUB_REPOSITORY}/${descriptor.tag}`,
          commit: recoveryCommit,
          releaseCommit,
          candidateToolingCommit: releaseCommit,
          recoveryCommit,
          tag: descriptor.tag,
          tagObject: expectedObject,
          signerFingerprint: PROVIDER_SIGNER,
          recoveredFrom: "EXACT_RETAINED_DRAFT",
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, descriptor.tag, expectedObject, {
          surface: PROVIDER_SURFACE,
          phase: "recover-draft",
        });
        throw error;
      }
    },
  );
}

function revocationAssetNames(tag) {
  const match = REVOCATION_TAG.exec(tag);
  if (!match) {
    throw new Error(
      "--tag must match forms/revocations/v<canonical-semver>",
    );
  }
  const version = match[1];
  const base = `takoform-form-revocation_${version}`;
  return {
    base,
    version,
    subject: `${base}_checkpoint.json`,
    bundle: `${base}_checkpoint.sigstore.json`,
    all: [
      `${base}_checkpoint.json`,
      `${base}_checkpoint.sigstore.json`,
      `${base}_statement.json`,
      `${base}_provenance.intoto.json`,
      "release-manifest.json",
      "SHA256SUMS",
    ].sort(),
  };
}

function verifySigstoreSubject(
  context,
  root,
  subject,
  bundle,
  workflow,
  trustedRoot = join(context.repo, TRUSTED_ROOT),
) {
  verifyLocalReleaseToolchain(context);
  command(context, "cosign", [
    "verify-blob",
    join(root, subject),
    "--bundle",
    join(root, bundle),
    "--trusted-root",
    trustedRoot,
    "--certificate-identity",
    `https://github.com/${GITHUB_REPOSITORY}/${workflow}@refs/heads/main`,
    "--certificate-oidc-issuer",
    OIDC_ISSUER,
  ]);
}

function verifyDeepAssetReport(context, root, report, expected) {
  if (
    report.format !== expected.format ||
    report.semanticStatus !== "verified" ||
    report.cryptographicStatus !== "external-required" ||
    report.tag !== expected.tag ||
    report.sourceCommit !== expected.sourceCommit ||
    report.toolingCommit !== expected.toolingCommit ||
    report.trustedRoot?.path !== resolve(context.repo, TRUSTED_ROOT) ||
    report.trustedRoot?.sha256 !== expected.trustedRootDigest ||
    !Array.isArray(report.assets)
  ) {
    throw new Error(`${expected.label} deep semantic report identity mismatch`);
  }
  const actual = report.assets
    .map((asset) => {
      requireExactKeys(
        asset,
        ["name", "sha256", "size"],
        `${expected.label} deep asset`,
      );
      const path = join(root, asset.name);
      if (
        !expected.names.includes(asset.name) ||
        !SHA256.test(asset.sha256 ?? "") ||
        asset.sha256 !== fileDigest(path) ||
        !Number.isSafeInteger(asset.size) ||
        asset.size < 0 ||
        asset.size !== lstatSync(path).size
      ) {
        throw new Error(
          `${expected.label} deep asset report mismatch: ${asset.name}`,
        );
      }
      return asset.name;
    })
    .sort();
  if (
    JSON.stringify(actual) !== JSON.stringify([...expected.names].sort())
  ) {
    throw new Error(`${expected.label} deep asset inventory mismatch`);
  }
}

function withTrustedRootAtCommit(context, toolingCommit, operation) {
  return withTemporaryDirectory("takoform-trusted-root", (temporaryRoot) => {
    const raw = command(context, "git", [
      "show",
      `${toolingCommit}:${TRUSTED_ROOT}`,
    ]);
    const path = join(temporaryRoot, "trusted-root.json");
    writeFileSync(path, raw, { flag: "wx", mode: 0o600 });
    return operation(path, fileDigest(path));
  });
}

function expectedFormTagObject(
  context,
  { tag, sourceCommit, requestId, runId, runAttempt },
) {
  const epoch = git(
    context,
    "show",
    "-s",
    "--format=%ct",
    sourceCommit,
  );
  if (!/^(?:0|[1-9][0-9]*)$/u.test(epoch)) {
    throw new Error("tag source commit timestamp is not a Unix epoch");
  }
  return (
    `object ${sourceCommit}\n` +
    "type commit\n" +
    `tag ${tag}\n` +
    "tagger Takoform Form Package Revocation " +
    `<release@takoform.invalid> ${epoch} +0000\n\n` +
    `Takoform Form Package revocation checkpoint ${tag}\n\n` +
    `source-commit: ${sourceCommit}\n` +
    `request-id: ${requestId}\n` +
    `workflow-run: https://github.com/${GITHUB_REPOSITORY}/actions/runs/` +
    `${runId}/attempts/${runAttempt}\n`
  );
}

function verifyTagObjectWorkflowBinding(
  context,
  root,
  metadata,
  runId,
  runAttempt,
) {
  const text = readFileSync(join(root, "tag-object"), "utf8");
  const expected = expectedFormTagObject(context, {
    tag: metadata.tag,
    sourceCommit: metadata.sourceCommit,
    requestId: metadata.requestId,
    runId,
    runAttempt,
  });
  if (text !== expected) {
    throw new Error(
      "tag object bytes differ from the exact deterministic workflow object",
    );
  }
}

function verifyRevocationSemanticClosure(
  context,
  root,
  tag,
  sourceCommit,
  toolingCommit,
  trustedRoot,
) {
  const names = revocationAssetNames(tag);
  const raw = command(context, "go", [
    "run",
    "./cmd/form-package-release",
    "verify-revocation-directory",
    "--asset-root",
    root,
    "--source-root",
    context.repo,
    "--tag",
    tag,
    "--source-commit",
    sourceCommit,
    "--tooling-commit",
    toolingCommit,
    "--trusted-root",
    trustedRoot,
  ]);
  const report = JSON.parse(raw);
  if (
    report.version !== names.version ||
    !Number.isSafeInteger(report.checkpointSequence) ||
    report.checkpointSequence <= 0 ||
    !SHA256.test(report.checkpointDigest ?? "") ||
    !SHA256.test(report.packageDigest ?? "") ||
    !report.formRef ||
    typeof report.formRef !== "object"
  ) {
    throw new Error("revocation deep semantic report binding mismatch");
  }
  return report;
}

function verifyRevocationPublicAssets(context, root, tag, { metadata } = {}) {
  const names = revocationAssetNames(tag);
  const actual = listRegularFiles(root);
  if (JSON.stringify(actual) !== JSON.stringify(names.all)) {
    throw new Error(`revocation asset inventory differs: ${actual.join(", ")}`);
  }
  const manifest = readJSON(
    join(root, "release-manifest.json"),
    "revocation release manifest",
  );
  if (
    manifest.schemaVersion !== 1 ||
    manifest.releaseType !== "form-package-revocation" ||
    manifest.tag !== tag ||
    manifest.sourceRepository !== `github.com/${GITHUB_REPOSITORY}` ||
    !COMMIT.test(manifest.sourceCommit ?? "") ||
    !COMMIT.test(manifest.toolingCommit ?? "") ||
    manifest.workflow !== ".github/workflows/form-package-revocation.yml" ||
    manifest.packageVersion !== names.version ||
    manifest.signedSubject !== names.subject ||
    manifest.signatureBundle !== names.bundle ||
    manifest.publicationReady !== true ||
    !Array.isArray(manifest.publicationBlockers) ||
    manifest.publicationBlockers.length !== 0 ||
    !Array.isArray(manifest.assets) ||
    manifest.assets.length !== 4
  ) {
    throw new Error("revocation manifest identity/closure mismatch");
  }
  if (
    metadata &&
    (manifest.sourceCommit !== metadata.sourceCommit ||
      manifest.toolingCommit !== metadata.toolingCommit)
  ) {
    throw new Error("revocation manifest commit binding differs from candidate");
  }
  const manifestNames = manifest.assets.map((asset) => asset.name).sort();
  for (const asset of manifest.assets) {
    if (
      !SHA256.test(asset.digest ?? "") ||
      fileDigest(join(root, asset.name)) !== asset.digest
    ) {
      throw new Error(`revocation manifest asset mismatch: ${asset.name}`);
    }
  }
  verifyNamedChecksums(root, "SHA256SUMS", [
    ...manifestNames,
    "release-manifest.json",
  ]);
  withTrustedRootAtCommit(
    context,
    manifest.toolingCommit,
    (trustedRoot, trustedRootDigest) => {
      const deepReport = verifyRevocationSemanticClosure(
        context,
        root,
        tag,
        manifest.sourceCommit,
        manifest.toolingCommit,
        trustedRoot,
      );
      verifyDeepAssetReport(context, root, deepReport, {
        format:
          "takoform.form-package-revocation-directory-verification@v1",
        label: "revocation",
        names: names.all,
        tag,
        sourceCommit: manifest.sourceCommit,
        toolingCommit: manifest.toolingCommit,
        trustedRootDigest,
      });
      verifySigstoreSubject(
        context,
        root,
        names.subject,
        names.bundle,
        ".github/workflows/form-package-revocation.yml",
        trustedRoot,
      );
    },
  );
  return manifest;
}

function verifyRevocationCandidate(
  context,
  root,
  { tag, runId, runAttempt, requestId, expectedCommit, toolingCommit },
) {
  const metadata = verifyCandidateRoot(root, {
    tagObject: true,
    metadataProfile: "pretty-required-lf",
  });
  requireExactKeys(
    metadata,
    [
      "format",
      "repository",
      "workflowPath",
      "workflowRef",
      "runId",
      "runAttempt",
      "sourceRef",
      "tag",
      "sourceCommit",
      "toolingCommit",
      "objectFormat",
      "tagObjectOid",
      "tagObjectSha256",
      "requestId",
      "assetCount",
      "assets",
    ],
    "revocation candidate metadata",
  );
  if (
    metadata.format !== "takoform.form-package-revocation-candidate@v1" ||
    metadata.repository !== GITHUB_REPOSITORY ||
    metadata.workflowPath !== ".github/workflows/form-package-revocation.yml" ||
    metadata.workflowRef !==
      `${GITHUB_REPOSITORY}/.github/workflows/form-package-revocation.yml@refs/heads/main` ||
    metadata.sourceRef !== "refs/heads/main" ||
    metadata.requestId !== requestId ||
    normalizedPositiveInteger(metadata.runId, "metadata runId") !== runId ||
    normalizedPositiveInteger(metadata.runAttempt, "metadata runAttempt") !==
      runAttempt ||
    metadata.tag !== tag ||
    metadata.sourceCommit !== expectedCommit ||
    metadata.toolingCommit !== toolingCommit ||
    metadata.assetCount !== 6
  ) {
    throw new Error("revocation candidate metadata binding mismatch");
  }
  const names = revocationAssetNames(tag);
  const assets = validateCandidateAssets(root, metadata, names.all);
  verifyRevocationPublicAssets(context, join(root, "assets"), tag, { metadata });
  verifyTagObjectWorkflowBinding(
    context,
    root,
    metadata,
    runId,
    runAttempt,
  );
  return { assets, metadata };
}

function readExactPublicRelease(
  context,
  tag,
  expectedNames,
  temporaryRoot,
  { prerelease = false } = {},
) {
  const release = getRelease(context, tag);
  const output = join(temporaryRoot, "release-readback");
  mkdirSync(output);
  command(context, "gh", [
    "release",
    "download",
    tag,
    "--repo",
    GITHUB_REPOSITORY,
    "--dir",
    output,
  ]);
  const actualNames = listRegularFiles(output);
  if (JSON.stringify(actualNames) !== JSON.stringify([...expectedNames].sort())) {
    throw new Error(`public release inventory differs: ${actualNames.join(", ")}`);
  }
  const assets = publicAssets(output, actualNames);
  validateReleaseReadback(release, tag, assets, { prerelease });
  return { release, output, assets };
}

function reportTagFailure(
  context,
  tag,
  materializedObject,
  { surface = FORM_SURFACE, phase } = {},
) {
  let observed = {
    local: null,
    localReadable: false,
    remote: null,
    remoteReadable: false,
    mutationState: "LOCAL_AND_REMOTE_UNREADABLE",
  };
  try {
    observed = observeTagFailureState(context, tag);
  } catch {
    // Failure reporting is diagnostic only and must never replace the original
    // release failure.
  }
  const uncertain =
    !observed.localReadable ||
    !observed.remoteReadable ||
    Boolean(
      materializedObject ||
        observed.local ||
        observed.remote?.object ||
        observed.remote?.commit,
    );
  const mutationState =
    materializedObject && observed.mutationState === "UNCHANGED"
      ? "INDETERMINATE_AFTER_TAG_MATERIALIZATION"
      : observed.mutationState;
  try {
    context.stderr.write(
      `${JSON.stringify({
        kind: "takos.deploy-failure@v1",
        surface,
        ...(phase ? { phase } : {}),
        tag,
        materializedTagObject: materializedObject || null,
        localTagObject: observed.local,
        localReadable: observed.localReadable,
        remoteTag: observed.remote,
        remoteReadable: observed.remoteReadable,
        mutationState,
        instruction: uncertain
          ? "inspect the exact local/remote ref and release state; do not delete, overwrite, or retry blindly"
          : "public tag/release is unchanged",
      })}\n`,
    );
  } catch {
    // Preserve the primary exception even if stderr itself is unavailable.
  }
}

function requireRevocationSource(context, tag, expectedCommit) {
  const names = revocationAssetNames(tag);
  const toolingCommit = assertCurrentProtectedMain(context, expectedCommit, {
    exact: false,
  });
  for (const path of [
    join(context.repo, "forms/revocations", `${names.version}.json`),
    join(
      context.repo,
      "forms/revocations/checkpoints",
      `${names.version}.json`,
    ),
  ]) {
    if (!existsSync(path) || !lstatSync(path).isFile()) {
      throw new Error(`revocation source is missing or non-regular: ${path}`);
    }
  }
  return { commit: expectedCommit, toolingCommit, names };
}

function revocationPrepare(context, options) {
  const { commit, toolingCommit, names } = requireRevocationSource(
    context,
    options.tag,
    options["expected-commit"],
  );
  assertTagAbsent(context, options.tag);
  assertReleaseAbsent(context, options.tag);
  ownerGateAndFence(context, toolingCommit);
  const dispatched = dispatchWorkflow(
    context,
    "form-package-revocation.yml",
    {
      tag: options.tag,
      expected_commit: commit,
    },
    { headSha: toolingCommit },
  );
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: FORM_SURFACE,
    phase: "prepare-revocation",
    commit,
    toolingCommit,
    tag: options.tag,
    checkpointVersion: names.version,
    dispatchStatus: "DISPATCHED",
    status: "AWAITING_REVIEW",
    workflowRun: dispatched,
  });
}

function revocationPublish(context, options) {
  const { commit, toolingCommit: initialMain, names } = requireRevocationSource(
    context,
    options.tag,
    options["expected-commit"],
  );
  assertTagAbsent(context, options.tag);
  assertReleaseAbsent(context, options.tag);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare signed Form Package revocation checkpoint",
    },
  );
  const toolingCommit = run.headSha;
  assertFormReleaseAuthorityFence(context, {
    sourceCommit: commit,
    toolingCommit,
    currentMain: initialMain,
    label: "revocation reviewed candidate fence",
  });
  return withTemporaryDirectory("takoform-revocation-publish", (temporaryRoot) => {
    const candidate = downloadArtifact(
      context,
      options["run-id"],
      `form-package-revocation-candidate-${options["run-id"]}-${options["run-attempt"]}`,
      join(temporaryRoot, "candidate"),
    );
    const verified = verifyRevocationCandidate(context, candidate, {
      tag: options.tag,
      runId: options["run-id"],
      runAttempt: options["run-attempt"],
      requestId: run.displayTitle,
      expectedCommit: commit,
      toolingCommit,
    });
    let tagObject = "";
    try {
      const mutationMain = ownerGateAndFence(context);
      assertFormReleaseAuthorityFence(context, {
        sourceCommit: commit,
        toolingCommit,
        currentMain: mutationMain,
        label: "revocation tag mutation fence",
      });
      tagObject = ensureCandidateTagPublished(
        context,
        options.tag,
        commit,
        candidate,
        verified.metadata,
      );
      const releaseMain = ownerGateAndFence(context);
      assertFormReleaseAuthorityFence(context, {
        sourceCommit: commit,
        toolingCommit,
        currentMain: releaseMain,
        label: "revocation release mutation fence",
      });
      const release = publishReleaseLocally(context, {
        tag: options.tag,
        assets: verified.assets,
        body:
          "Append-only Takoform Form Package security revocation checkpoint. Verify the signed cumulative checkpoint before enforcement.",
        temporaryRoot,
        prePublishFence: () => {
          const publishMain = ownerGateAndFence(context);
          assertFormReleaseAuthorityFence(context, {
            sourceCommit: commit,
            toolingCommit,
            currentMain: publishMain,
            label: "revocation pre-publish fence",
          });
        },
      });
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: FORM_SURFACE,
        phase: "publish-revocation",
        commit,
        tag: options.tag,
        checkpointVersion: names.version,
        tagObject,
        candidateRun: { id: options["run-id"], attempt: options["run-attempt"], url: run.url },
        releaseId: release.id,
        releaseUrl: release.html_url,
        assetDigests: Object.fromEntries(
          [...verified.assets].map(([name, asset]) => [name, asset.sha256]),
        ),
        productionReadback: "EXACT_IMMUTABLE_RELEASE",
        status: "VERIFIED",
      });
    } catch (error) {
      reportTagFailure(context, options.tag, tagObject);
      throw error;
    }
  });
}

function revocationVerify(context, options) {
  const names = revocationAssetNames(options.tag);
  const currentMain = assertCurrentProtectedMain(
    context,
    options["expected-commit"],
    {
    exact: false,
    },
  );
  assertExactRemoteTag(context, options.tag, options["expected-commit"]);
  return withTemporaryDirectory("takoform-revocation-verify", (temporaryRoot) => {
    const live = readExactPublicRelease(
      context,
      options.tag,
      names.all,
      temporaryRoot,
    );
    const manifest = verifyRevocationPublicAssets(
      context,
      live.output,
      options.tag,
    );
    if (manifest.sourceCommit !== options["expected-commit"]) {
      throw new Error("public revocation source commit differs");
    }
    assertCommitAncestor(
      context,
      manifest.sourceCommit,
      manifest.toolingCommit,
      "public revocation source/tooling binding",
    );
    assertCommitAncestor(
      context,
      manifest.toolingCommit,
      currentMain,
      "public revocation tooling/current-main binding",
    );
    return emit(context, {
      kind: "takos.deploy-result@v1",
      surface: FORM_SURFACE,
      phase: "verify-revocation",
      tag: options.tag,
      commit: manifest.sourceCommit,
      toolingCommit: manifest.toolingCommit,
      releaseId: live.release.id,
      releaseUrl: live.release.html_url,
      assetDigests: Object.fromEntries(
        [...live.assets].map(([name, asset]) => [name, asset.sha256]),
      ),
      status: "VERIFIED",
    });
  });
}

// Narrow executable seams for the owner-local release safety tests. Production
// callers use runReleaseSurface; these hooks keep tests off live git/GitHub
// authority while exercising the same mutation and verification code paths.
export const releaseDeployTestHooks = Object.freeze({
  assertExactSignedProviderTag,
  assertPinnedProviderGpgVerification,
  assertReleaseImmutabilityEnabled,
  assertFormReleaseAuthorityFence,
  assertProviderRecoveryFence,
  assertReleaseAbsent,
  assertRegistryVersionAbsent,
  command,
  dispatchWorkflow,
  expectedFormTagObject,
  githubUploadEnvironment,
  ownerGateAndFence,
  observeTagFailureState,
  parseCandidateMetadata,
  parseCanonicalCandidateMetadata,
  parsePrettyCandidateMetadata,
  publishReleaseLocally,
  providerAssetNames,
  readProviderDescriptor,
  validateProviderIdentityLedger,
  pushExactTag,
  reportTagFailure,
  resumeDraftReleaseLocally,
  reconstructCandidateTagObject,
  requireSuccessfulRun,
  validateReleaseReadback,
  validateDraftBeforePublication,
  verifyChecksumClosure,
  verifyProviderCandidate,
  verifyProviderReleaseProvenance,
  verifyProviderSignature,
  verifyRevocationSemanticClosure,
  verifyTagObjectWorkflowBinding,
});

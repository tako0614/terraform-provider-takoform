import { createHash, randomUUID } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  accessSync,
  chmodSync,
  closeSync,
  constants as fsConstants,
  existsSync,
  fstatSync,
  fsyncSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  realpathSync,
  readSync,
  readdirSync,
  rmSync,
  writeSync,
  writeFileSync,
} from "node:fs";
import {
  basename,
  delimiter,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
  sep,
} from "node:path";
import { tmpdir } from "node:os";
import process from "node:process";

const GITHUB_REPOSITORY = "tako0614/terraform-provider-takoform";
const SOURCE_REPOSITORY = `https://github.com/${GITHUB_REPOSITORY}.git`;
const PROVIDER_ADDRESS = "registry.terraform.io/tako0614/takoform";
const PROVIDER_RELEASE_BRANCH = "maintenance/v1";
const PROVIDER_RELEASE_REF = `refs/heads/${PROVIDER_RELEASE_BRANCH}`;
const PROVIDER_RELEASE_REMOTE_REF =
  `refs/remotes/origin/${PROVIDER_RELEASE_BRANCH}`;
const PROVIDER_SIGNER = "3510E75E05BBCC303B92D77934FC18AC897FB709";
const OIDC_ISSUER = "https://token.actions.githubusercontent.com";
const TRUSTED_ROOT = "admission/v4/trust/trusted-root.json";
const PINNED_GH_VERSION = "2.96.0";
const PINNED_COSIGN_VERSION = "v3.0.6";
const SHA256 = /^sha256:[0-9a-f]{64}$/u;
const COMMIT = /^[0-9a-f]{40}$/u;
const GIT_OBJECT = /^(?:[0-9a-f]{40}|[0-9a-f]{64})$/u;
const POSITIVE_INTEGER = /^[1-9][0-9]*$/u;
const FORM_BATCH_MAX_BYTES = 1024 * 1024;
const PROVIDER_REQUEST_RECORD_MAX_BYTES = 4096;
const PROVIDER_REQUEST_RECORD_FORMAT =
  "takoform.provider-candidate-dispatch-request@v1";
const PROVIDER_CANDIDATE_WORKFLOW = ".github/workflows/release.yml";
const REQUEST_ID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;
const PROVIDER_TAG = /^v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$/u;
const FORM_TAG = /^forms\/(k-[a-z2-7]{2,103})\/v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$/u;
const REVOCATION_TAG = /^forms\/revocations\/v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?)$/u;
const FORM_RELEASE_AUTHORITY_PATHS = Object.freeze([
  ".github/workflows/form-package-release.yml",
  ".github/workflows/form-package-revocation.yml",
  TRUSTED_ROOT,
  "cmd/form-package-release",
  "cmd/published-package-set",
  "cmd/release-output-commit",
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
const FORM_TAG_ONLY_RECOVERY_STABLE_PATHS = Object.freeze([
  ".github/workflows/form-package-release.yml",
  TRUSTED_ROOT,
  "cmd/form-package-release",
  "formpackage",
  "go.mod",
  "go.sum",
  "internal/client",
  "internal/formcatalog",
  "internal/formregistry",
  "internal/hostpolicy",
  "standardform",
]);
const PROVIDER_RECOVERY_ALLOWED_PATHS = Object.freeze([
  "release/README.md",
  "scripts/check-public-surfaces.mjs",
  "scripts/release-deploy.mjs",
  "scripts/release-deploy.test.mjs",
  "scripts/testdata/provider-release-candidate-30507374579-1-metadata.json",
]);
const FORM_TAG_ONLY_RELEASE_BODY =
  "Data-only Takoform Form Package release completed forward from an exact tag-only partial state. Verify the canonical package index, Sigstore bundle, and publisher identity before installation.";

const PROVIDER_SURFACE = "takoform-provider-release";
const FORM_SURFACE = "takoform-form-package-release";
const WORKFLOW_NAMES = {
  "provider-release-tag.yml": "Author provider release tag",
  "release.yml": "Prepare provider release candidate",
  "provider-registry-readback.yml":
    "Build signed provider Registry readback",
  "form-package-release.yml":
    "Prepare signed Form Package release candidate",
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
      ".github/workflows/provider-registry-readback.yml",
      "cmd/provider-release",
    ],
    requiresScripts: ["check"],
    requiresTools: ["git", "bun", "gh", "go", "gpg", "cosign", "curl"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "requires local operator GH_TOKEN authority, a clean non-shallow maintenance/v1 checkout equal to a freshly fetched canonical origin/maintenance/v1, the complete owner check before every dispatch, tag push, or release mutation, an explicitly named successful workflow run/attempt, checksum closure over its same-run candidate, the pinned provider GPG signer, and a record of the source commit plus every published asset digest; the release-candidate dispatch additionally requires separately pinned release E and current maintenance F commits, E==F for the ordinary lane or the existing exact allowlisted nonempty E..F provider recovery fence, an operator-supplied lowercase UUID, and a canonical create-only owner-private request record durably binding E, F, the exact signed tag object, workflow, and dispatchAttempted=true before its sole POST; GH_TOKEN is never printed or retained",
      "post-conditions":
        "publishes the exact verified same-run bytes locally under exclusive single-writer authority because GitHub REST has no atomic asset-plus-metadata precondition, immediately rereads the empty and complete exact draft, restates the full exact identity on PATCH, requires GitHub's immutable release readback and a fresh download with identical digests, rechecks the pinned signed tag before VERIFIED, then separately installs the indexed provider with both supported Registry clients and checksum-closes the signed direct-install readback",
      reversal:
        "provider versions, signed tags, and release assets are immutable and cannot be rolled back or overwritten; an exact signed tag-only partial state may only be completed by recover-tag-only, and an exact retained draft may only be resumed by recover-draft without changing the tag, candidate, or draft identity; any other bad publication is halted and repaired forward under a new version",
      "failure-handling":
        "distinguishes failure before tag creation, after local tag materialization, after remote tag push, after durable candidate-dispatch intent, after draft creation, and after immutable publication; an exact authoritative remote tag reconciles a lost acknowledgement without another push, while a durable dispatch attempt with a lost acknowledgement can only use read-only reconcile-candidate and can never authorize another POST; it retains and reports every created draft for authoritative inspection, never automatically removes a release or tag, refuses blind retry after an indeterminate mutation, and exposes only explicit exact-tag and exact-draft publication recovery bound to the original release commit, tag object, run attempt, and current reviewed recovery commit",
      "independent-review":
        "the provider-release protected Environment reviews the signed tag and release candidates; the local publisher accepts only that exact successful run/attempt and re-verifies it independently before using local GitHub authority",
      "no-overwrite":
        "requires the descriptor tag; a new tag uses zero-object-id compare-and-swap and a create-only lease push, while resumption accepts only the exact verified signed local and/or authoritative remote annotated object and refuses every drift or partial remote identity; GitHub Release and Registry identities must remain absent before candidate dispatch or publication; prepare-candidate requires an absent non-symlink target under one physical operator-owned 0700 directory, creates the 0600 owner-private request record with O_EXCL and fsync before one POST, and every later create-mode call refuses that record; publication accepts only an immutable release with the exact candidate inventory, and recovery never mutates or deletes a tag",
      halt:
        "prepare stops after dispatching the protected signed-tag workflow; tag verifies and reconciles the exact signed local/remote tag and stops as TAG_READY without dispatching release.yml; prepare-candidate durably records one exact request and attempts at most one release.yml dispatch before returning RECONCILIATION_REQUIRED; reconcile-candidate is strictly read-only, never dispatches, and returns only one exact UUID-bound run or UNRESOLVED_ABSENT; readback stops after its exact workflow dispatch, and no phase selects a latest or ambiguous run",
    },
  },
  {
    surface: FORM_SURFACE,
    target:
      "github-release:tako0614/terraform-provider-takoform/forms/*",
    covers: [
      "forms/release-plan.json",
      "forms/revocations",
      ".github/workflows/form-package-release.yml",
      ".github/workflows/form-package-revocation.yml",
      "cmd/form-package-release",
      "cmd/published-package-set",
    ],
    requiresScripts: ["check"],
    requiresTools: ["git", "bun", "gh", "go", "cosign"],
    requiresEnv: ["GH_TOKEN"],
    triggers: ["authority", "published-identity", "asynchronous"],
    obligations: {
      provenance:
        "uses local operator GH_TOKEN authority without printing or retaining GH_TOKEN, accepts only exact canonical release-plan tags or one exact source-backed revocation tag; ordinary phases accept one Form tag and explicit publish-batch accepts a non-empty unique ordered set, requires a clean non-shallow current protected main, runs the complete owner check before dispatch or mutation, or exactly once for an explicit serial publish-batch whose process-scoped proof is bound to the same clean protected-main commit and tree and revalidated immediately before each tag push, Release draft creation, and final draft publication, checksum-closes every explicit successful same-run candidate, verifies its source/tooling commits and Sigstore identity, and records every exact tag object and asset digest",
      "post-conditions":
        "after local tag/release publication requires exact remote tag resolution, immutable release id/tag, exact API asset digests, and a fresh seven-file package or six-file revocation download; verify-all delegates all 34 semantic readbacks to the repository's Go verifier and writes one create-only external publication set",
      reversal:
        "Form Package and revocation identities are append-only and cannot be replaced; an exact tag-only partial state may only be completed forward by recover-tag-only without changing the tag, and its exact retained draft may only be resumed by recover-draft without replacing any identity, while a bad identity requires a corrected package version or later cumulative revocation checkpoint",
      "failure-handling":
        "prints raw diagnostics, retains and reports every created draft for authoritative inspection, never automatically removes a release or tag, reports local and remote ref state after a tag-side partial failure, serial publish-batch preserves input order, stops on the first failure, and reports completedTags plus failedTag, refuses blind retry whenever mutation state is indeterminate, exposes an explicit tag-only recovery that requires the exact candidate object/commit and a completely absent Release, and exposes a separate exact-draft recovery that accepts only a named matching draft and uploads only its missing same-candidate assets",
      "independent-review":
        "the form-package-release protected Environment reviews and signs the exact candidate; local publication consumes only that named successful run/attempt and independently verifies its checksum and Sigstore closure",
      "no-overwrite":
        "the plan/tag/source must match exactly, local tag creation is compare-and-swap, remote refs must be absent before creation and exact afterwards, existing releases are refused, and final readback requires immutable exact assets",
      halt:
        "prepare and prepare-revocation stop at the exact returned workflow URL as AWAITING_REVIEW; cancel that exact run before approval if anything changes, and publish never infers or selects a latest candidate",
    },
  },
]);

const PHASES = {
  [PROVIDER_SURFACE]: {
    prepare: ["tag", "expected-commit"],
    tag: ["tag", "expected-commit", "run-id", "run-attempt"],
    "prepare-candidate": [
      "tag",
      "expected-release-commit",
      "expected-current-commit",
      "expected-tag-object",
      "request-id",
      "request-record",
    ],
    "reconcile-candidate": [
      "tag",
      "expected-release-commit",
      "expected-current-commit",
      "expected-tag-object",
      "request-id",
      "request-record",
    ],
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
    readback: ["tag", "expected-commit"],
    verify: ["tag", "expected-commit", "run-id", "run-attempt"],
  },
  [FORM_SURFACE]: {
    plan: [],
    prepare: ["tag", "expected-commit"],
    publish: ["tag", "expected-commit", "run-id", "run-attempt"],
    "publish-batch": ["input"],
    "recover-tag-only": [
      "tag",
      "expected-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "run-id",
      "run-attempt",
    ],
    "recover-draft": [
      "tag",
      "expected-commit",
      "expected-tag-object",
      "expected-recovery-commit",
      "release-id",
      "run-id",
      "run-attempt",
    ],
    verify: ["tag", "expected-commit"],
    "verify-all": ["output-root"],
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
    "expected-current-commit",
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
  if (values["output-root"] && !isAbsolute(values["output-root"])) {
    throw new Error("--output-root must be an absolute path");
  }
  if (values.input && !isAbsolute(values.input)) {
    throw new Error("--input must be an absolute path");
  }
  if (values["request-id"] && !REQUEST_ID.test(values["request-id"])) {
    throw new Error("--request-id must be a canonical lowercase UUIDv4");
  }
  if (
    values["request-record"] &&
    !isAbsolute(values["request-record"])
  ) {
    throw new Error("--request-record must be an absolute path");
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
    case "prepare-candidate":
      return providerPrepareCandidate(context, options, descriptor);
    case "reconcile-candidate":
      return providerReconcileCandidate(context, options, descriptor);
    case "publish":
      return providerPublish(context, options, descriptor);
    case "recover-tag-only":
      return providerRecoverTagOnly(context, options, descriptor);
    case "recover-draft":
      return providerRecoverDraft(context, options, descriptor);
    case "readback":
      return providerReadback(context, options, descriptor);
    case "verify":
      return providerVerify(context, options, descriptor);
    default:
      throw usageError(PROVIDER_SURFACE);
  }
}

function runForm(context, options) {
  switch (options.phase) {
    case "plan":
      return formPlan(context, readReleasePlan(context.repo));
    case "prepare":
      return formPrepare(context, options, readReleasePlan(context.repo));
    case "publish":
      return formPublish(context, options, readReleasePlan(context.repo));
    case "publish-batch":
      return formPublishBatch(
        context,
        options,
        readReleasePlan(context.repo),
      );
    case "recover-tag-only":
      return formRecoverTagOnly(
        context,
        options,
        readReleasePlan(context.repo),
      );
    case "recover-draft":
      return formRecoverDraft(
        context,
        options,
        readReleasePlan(context.repo),
      );
    case "verify":
      return formVerify(context, options);
    case "verify-all":
      return formVerifyAll(context, options, readReleasePlan(context.repo));
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
  if (context.formPublishBatchOwnerGateProof) {
    return assertReusableOwnerGateProof(
      context,
      context.formPublishBatchOwnerGateProof,
      expectedCommit,
      options,
    );
  }
  runOwnerCheck(context);
  return assertCurrentProtectedMain(context, expectedCommit, options);
}

function providerOwnerGateAndFence(context, expectedCommit, options) {
  verifyLocalReleaseToolchain(context);
  runOwnerCheck(context);
  return assertCurrentProviderReleaseBranch(
    context,
    expectedCommit,
    options,
  );
}

function establishFormPublishBatchOwnerGateProof(context) {
  if (context.formPublishBatchOwnerGateProof) {
    throw new Error("Form Package batch owner gate proof is already active");
  }
  verifyLocalReleaseToolchain(context);
  const initialMain = assertCurrentProtectedMain(context);
  runOwnerCheck(context);
  const checkedMain = assertCurrentProtectedMain(context, initialMain);
  const tree = git(context, "rev-parse", "HEAD^{tree}");
  const proof = Object.freeze({
    repo: context.repo,
    commit: checkedMain,
    tree,
  });
  context.formPublishBatchOwnerGateProof = proof;
  return proof;
}

function assertReusableOwnerGateProof(
  context,
  proof,
  expectedCommit,
  options,
) {
  if (
    !proof ||
    proof.repo !== context.repo ||
    !COMMIT.test(proof.commit ?? "") ||
    !GIT_OBJECT.test(proof.tree ?? "")
  ) {
    throw new Error("Form Package batch owner gate proof is invalid");
  }
  const currentMain = assertCurrentProtectedMain(context, proof.commit);
  const currentTree = git(context, "rev-parse", "HEAD^{tree}");
  if (currentTree !== proof.tree) {
    throw new Error(
      `release blocked: protected-main tree changed after the batch owner gate; expected ${proof.tree}, observed ${currentTree}`,
    );
  }
  if (expectedCommit) {
    const exact = options?.exact ?? true;
    if (exact && expectedCommit !== currentMain) {
      throw new Error(
        `release blocked: expected commit ${expectedCommit} is not current protected main ${currentMain}`,
      );
    }
    command(context, "git", ["cat-file", "-e", `${expectedCommit}^{commit}`]);
    if (!exact) {
      command(context, "git", [
        "merge-base",
        "--is-ancestor",
        expectedCommit,
        currentMain,
      ]);
    }
  }
  return currentMain;
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

function assertCurrentCanonicalBranch(
  context,
  expectedCommit,
  {
    branch,
    branchRef,
    remoteRef,
    label,
    exact = true,
  },
) {
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
  let attachedBranch;
  try {
    attachedBranch = git(context, "symbolic-ref", "--quiet", "--short", "HEAD");
  } catch {
    throw new Error(`release blocked: checkout must be attached to ${branch}`);
  }
  if (attachedBranch !== branch) {
    throw new Error(`release blocked: checkout must be attached to ${branch}`);
  }
  progress(context, `fetch canonical protected origin/${branch}`);
  command(context, "git", [
    "fetch",
    "--no-tags",
    "--prune",
    "origin",
    `+${branchRef}:${remoteRef}`,
  ]);
  const head = git(context, "rev-parse", "HEAD");
  const remoteHead = git(context, "rev-parse", remoteRef);
  if (head !== remoteHead) {
    throw new Error(
      `release blocked: HEAD ${head} is not fresh origin/${branch} ${remoteHead}`,
    );
  }
  if (expectedCommit) {
    if (exact && expectedCommit !== head) {
      throw new Error(
        `release blocked: expected commit ${expectedCommit} is not current ${label} ${head}`,
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

function assertCurrentProtectedMain(
  context,
  expectedCommit,
  { exact = true } = {},
) {
  return assertCurrentCanonicalBranch(context, expectedCommit, {
    branch: "main",
    branchRef: "refs/heads/main",
    remoteRef: "refs/remotes/origin/main",
    label: "protected main",
    exact,
  });
}

function assertCurrentProviderReleaseBranch(
  context,
  expectedCommit,
  { exact = true } = {},
) {
  return assertCurrentCanonicalBranch(context, expectedCommit, {
    branch: PROVIDER_RELEASE_BRANCH,
    branchRef: PROVIDER_RELEASE_REF,
    remoteRef: PROVIDER_RELEASE_REMOTE_REF,
    label: "protected maintenance/v1",
    exact,
  });
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
  {
    sourceCommit,
    toolingCommit,
    currentMain,
    sourcePath,
    revocation = false,
    label,
  },
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
    revocation ? "forms/revocations" : "forms/release-plan.json",
    ...(!revocation && sourcePath ? [sourcePath] : []),
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

function assertFormTagOnlyRecoveryFence(
  context,
  {
    sourceCommit,
    toolingCommit,
    recoveryCommit,
    sourcePath,
    label,
  },
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
    recoveryCommit,
    `${label} tooling/recovery ancestry`,
  );
  command(
    context,
    "git",
    [
      "diff",
      "--quiet",
      toolingCommit,
      recoveryCommit,
      "--",
      "forms/release-plan.json",
      sourcePath,
      ...FORM_TAG_ONLY_RECOVERY_STABLE_PATHS,
    ],
    {
      label:
        `${label}: candidate generation, identity, signing, or trust inputs ` +
        "changed after the reviewed candidate",
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

function readProviderDescriptor(repo) {
  const descriptor = readJSON(
    join(repo, "release/version.json"),
    "provider release descriptor",
  );
  if (
    typeof descriptor !== "object" ||
    descriptor === null ||
    !PROVIDER_TAG.test(descriptor.tag) ||
    descriptor.tag !== `v${descriptor.version}` ||
    descriptor.providerAddress !== PROVIDER_ADDRESS ||
    descriptor.signingFingerprint !== PROVIDER_SIGNER ||
    descriptor.publicationStatus !== "candidate-only" ||
    !Array.isArray(descriptor.platforms) ||
    descriptor.platforms.length !== 5
  ) {
    throw new Error("provider release descriptor identity is invalid");
  }
  return descriptor;
}

function requireProviderTag(tag, descriptor) {
  if (!tag || tag !== descriptor.tag) {
    throw new Error(
      `--tag must exactly match release/version.json (${descriptor.tag})`,
    );
  }
}

function readReleasePlan(repo) {
  const plan = readJSON(join(repo, "forms/release-plan.json"), "release plan");
  return validateReleasePlan(plan);
}

function readReleasePlanAtCommit(context, commit) {
  const raw = command(context, "git", [
    "show",
    `${commit}:forms/release-plan.json`,
  ]);
  let plan;
  try {
    plan = JSON.parse(raw);
  } catch (error) {
    throw new Error(`release plan at tooling commit is invalid JSON`, {
      cause: error,
    });
  }
  return validateReleasePlan(plan);
}

function validateReleasePlan(plan) {
  if (
    plan?.format !== "takoform.release-plan@v1" ||
    plan.generation !== "portable-v1" ||
    plan.repository !== GITHUB_REPOSITORY ||
    !Array.isArray(plan.releases) ||
    plan.releases.length !== 34
  ) {
    throw new Error("forms/release-plan.json is not the exact 34-entry plan");
  }
  const tags = new Set();
  for (const entry of plan.releases) {
    const match = FORM_TAG.exec(entry?.tag ?? "");
    if (
      !match ||
      entry.releaseId !== match[1] ||
      entry.version !== match[2] ||
      entry.sourcePath !==
        `forms/releases/${entry.releaseId}/${entry.version}` ||
      !SHA256.test(entry.packageDigest) ||
      typeof entry.kind !== "string" ||
      typeof entry.formRef !== "object" ||
      tags.has(entry.tag)
    ) {
      throw new Error(`release plan entry is invalid: ${entry?.tag ?? "unknown"}`);
    }
    tags.add(entry.tag);
  }
  return plan;
}

function formReleaseIdentity(tag) {
  const match = FORM_TAG.exec(tag ?? "");
  if (!match) {
    throw new Error(
      "--tag must match forms/<canonical-release-id>/v<canonical-semver>",
    );
  }
  return { tag, releaseId: match[1], version: match[2] };
}

function plannedRelease(plan, tag) {
  const entry = plan.releases.find((candidate) => candidate.tag === tag);
  if (!entry) {
    throw new Error(
      `Form Package tag is not one of the exact 34 release-plan entries: ${tag}`,
    );
  }
  return entry;
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
      ...(ref === "main" || ref === PROVIDER_RELEASE_BRANCH
        ? ["--branch", ref]
        : []),
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

function providerCandidateRequestRecord({
  currentCommit,
  releaseCommit,
  releaseTag,
  requestId,
  requestRecord,
  tagObjectOid,
}) {
  if (
    !COMMIT.test(currentCommit ?? "") ||
    !COMMIT.test(releaseCommit ?? "") ||
    !PROVIDER_TAG.test(releaseTag ?? "") ||
    !REQUEST_ID.test(requestId ?? "") ||
    !GIT_OBJECT.test(tagObjectOid ?? "") ||
    !isAbsolute(requestRecord ?? "") ||
    resolve(requestRecord) !== requestRecord
  ) {
    throw new Error("provider candidate request record bindings are invalid");
  }
  return {
    currentCommit,
    dispatchAttempted: true,
    dispatchRef: `refs/tags/${releaseTag}`,
    format: PROVIDER_REQUEST_RECORD_FORMAT,
    releaseCommit,
    releaseTag,
    repository: GITHUB_REPOSITORY,
    requestId,
    requestRecordPathSha256: sha256(Buffer.from(requestRecord, "utf8")),
    tagObjectOid,
    workflowPath: PROVIDER_CANDIDATE_WORKFLOW,
    workflowRef:
      `${GITHUB_REPOSITORY}/${PROVIDER_CANDIDATE_WORKFLOW}@refs/tags/${releaseTag}`,
  };
}

function canonicalProviderCandidateRequestRecord(record, requestRecord) {
  requireExactKeys(
    record,
    [
      "currentCommit",
      "dispatchAttempted",
      "dispatchRef",
      "format",
      "releaseCommit",
      "releaseTag",
      "repository",
      "requestId",
      "requestRecordPathSha256",
      "tagObjectOid",
      "workflowPath",
      "workflowRef",
    ],
    "provider candidate request record",
  );
  const expected = providerCandidateRequestRecord({
    currentCommit: record.currentCommit,
    releaseCommit: record.releaseCommit,
    releaseTag: record.releaseTag,
    requestId: record.requestId,
    requestRecord,
    tagObjectOid: record.tagObjectOid,
  });
  const canonical = Buffer.from(
    `${JSON.stringify(recursivelySorted(expected))}\n`,
    "utf8",
  );
  if (
    canonical.length > PROVIDER_REQUEST_RECORD_MAX_BYTES ||
    !canonical.equals(
      Buffer.from(`${JSON.stringify(recursivelySorted(record))}\n`, "utf8"),
    )
  ) {
    throw new Error("provider candidate request record binding mismatch");
  }
  return { canonical, expected };
}

function requestRecordOwnerUid(getuid = process.getuid) {
  if (typeof getuid !== "function") {
    throw new Error("provider candidate request records require POSIX ownership");
  }
  const uid = getuid();
  if (!Number.isSafeInteger(uid) || uid < 0) {
    throw new Error("provider candidate request record owner is invalid");
  }
  return uid;
}

function isPathInside(root, candidate) {
  const pathFromRoot = relative(root, candidate);
  return (
    pathFromRoot === "" ||
    (pathFromRoot !== ".." &&
      !pathFromRoot.startsWith(`..${sep}`) &&
      !isAbsolute(pathFromRoot))
  );
}

function inspectProviderRequestRecordParent(
  repo,
  requestRecord,
  { getuid = process.getuid } = {},
) {
  if (
    !isAbsolute(requestRecord ?? "") ||
    resolve(requestRecord) !== requestRecord
  ) {
    throw new Error("provider request record path must be canonical and absolute");
  }
  const physicalRepo = realpathSync(repo);
  if (isPathInside(physicalRepo, requestRecord)) {
    throw new Error("provider request record must be outside the source tree");
  }
  const parent = dirname(requestRecord);
  let parentStat;
  let physicalParent;
  try {
    parentStat = lstatSync(parent);
    physicalParent = realpathSync(parent);
  } catch {
    throw new Error("provider request record parent must already exist");
  }
  if (
    !parentStat.isDirectory() ||
    parentStat.isSymbolicLink() ||
    physicalParent !== parent
  ) {
    throw new Error(
      "provider request record parent must be one physical directory",
    );
  }
  const uid = requestRecordOwnerUid(getuid);
  if (parentStat.uid !== uid) {
    throw new Error("provider request record parent owner must be the operator");
  }
  if ((parentStat.mode & 0o7777) !== 0o700) {
    throw new Error("provider request record parent mode must be exactly 0700");
  }
  return { parent, parentStat, uid };
}

function assertOwnerPrivateRequestRecordFile(
  stat,
  uid,
  { emptyOK = false } = {},
) {
  if (!stat.isFile()) {
    throw new Error("provider candidate request record must be a regular file");
  }
  if (stat.uid !== uid) {
    throw new Error(
      "provider candidate request record owner must be the operator",
    );
  }
  if ((stat.mode & 0o7777) !== 0o600) {
    throw new Error(
      "provider candidate request record mode must be exactly 0600",
    );
  }
  if (
    !Number.isSafeInteger(stat.size) ||
    (!emptyOK && stat.size <= 0) ||
    stat.size > PROVIDER_REQUEST_RECORD_MAX_BYTES
  ) {
    throw new Error("provider candidate request record has an invalid size");
  }
}

function writeAll(descriptor, raw, write = writeSync) {
  let offset = 0;
  while (offset < raw.length) {
    const written = write(
      descriptor,
      raw,
      offset,
      raw.length - offset,
      null,
    );
    if (
      !Number.isSafeInteger(written) ||
      written <= 0 ||
      written > raw.length - offset
    ) {
      throw new Error("provider candidate request record write was incomplete");
    }
    offset += written;
  }
}

function createProviderCandidateRequestRecord(
  repo,
  requestRecord,
  record,
  {
    getuid = process.getuid,
    open = openSync,
    fstat = fstatSync,
    fsync = fsyncSync,
    close = closeSync,
    write = writeSync,
  } = {},
) {
  const { canonical, expected } =
    canonicalProviderCandidateRequestRecord(record, requestRecord);
  const { parent, parentStat, uid } = inspectProviderRequestRecordParent(
    repo,
    requestRecord,
    { getuid },
  );
  try {
    lstatSync(requestRecord);
    throw new Error(
      "provider candidate request record already exists; use reconcile-candidate",
    );
  } catch (error) {
    if (error?.code !== "ENOENT") {
      if (error?.message?.startsWith("provider candidate request record")) {
        throw error;
      }
      throw new Error("provider candidate request record state is unreadable");
    }
  }
  let descriptor;
  try {
    descriptor = open(
      requestRecord,
      fsConstants.O_WRONLY |
        fsConstants.O_CREAT |
        fsConstants.O_EXCL |
        fsConstants.O_NOFOLLOW,
      0o600,
    );
  } catch {
    throw new Error(
      "provider candidate request record cannot be created exclusively",
    );
  }
  try {
    const initialStat = fstat(descriptor);
    assertOwnerPrivateRequestRecordFile(initialStat, uid, { emptyOK: true });
    if (initialStat.size !== 0) {
      throw new Error("provider candidate request record was not created empty");
    }
    writeAll(descriptor, canonical, write);
    fsync(descriptor);
    const writtenStat = fstat(descriptor);
    assertOwnerPrivateRequestRecordFile(writtenStat, uid);
    if (writtenStat.size !== canonical.length) {
      throw new Error("provider candidate request record write was incomplete");
    }
  } finally {
    close(descriptor);
  }
  let parentDescriptor;
  try {
    parentDescriptor = open(
      parent,
      fsConstants.O_RDONLY |
        fsConstants.O_DIRECTORY |
        fsConstants.O_NOFOLLOW,
    );
    const openedParent = fstat(parentDescriptor);
    if (
      !openedParent.isDirectory() ||
      openedParent.dev !== parentStat.dev ||
      openedParent.ino !== parentStat.ino
    ) {
      throw new Error("provider request record parent changed during creation");
    }
    fsync(parentDescriptor);
  } catch (error) {
    if (error?.message?.startsWith("provider request record")) throw error;
    throw new Error(
      "provider request record parent could not be durably synchronized",
    );
  } finally {
    if (parentDescriptor !== undefined) close(parentDescriptor);
  }
  return readProviderCandidateRequestRecord(repo, requestRecord, expected, {
    getuid,
  });
}

function readProviderCandidateRequestRecord(
  repo,
  requestRecord,
  expected,
  {
    getuid = process.getuid,
    open = openSync,
    fstat = fstatSync,
    read = readSync,
    close = closeSync,
  } = {},
) {
  const { canonical } = canonicalProviderCandidateRequestRecord(
    expected,
    requestRecord,
  );
  const { parentStat, uid } = inspectProviderRequestRecordParent(
    repo,
    requestRecord,
    { getuid },
  );
  let descriptor;
  try {
    descriptor = open(
      requestRecord,
      fsConstants.O_RDONLY |
        fsConstants.O_NOFOLLOW |
        fsConstants.O_NONBLOCK,
    );
  } catch {
    throw new Error(
      "provider candidate request record cannot be opened without following links",
    );
  }
  let raw;
  let initialStat;
  try {
    initialStat = fstat(descriptor);
    assertOwnerPrivateRequestRecordFile(initialStat, uid);
    const buffer = Buffer.alloc(PROVIDER_REQUEST_RECORD_MAX_BYTES + 1);
    let offset = 0;
    while (offset < buffer.length) {
      const bytesRead = read(
        descriptor,
        buffer,
        offset,
        buffer.length - offset,
        null,
      );
      if (
        !Number.isSafeInteger(bytesRead) ||
        bytesRead < 0 ||
        bytesRead > buffer.length - offset
      ) {
        throw new Error("provider candidate request record read was invalid");
      }
      if (bytesRead === 0) break;
      offset += bytesRead;
    }
    if (offset > PROVIDER_REQUEST_RECORD_MAX_BYTES) {
      throw new Error("provider candidate request record is too large");
    }
    const finalStat = fstat(descriptor);
    assertOwnerPrivateRequestRecordFile(finalStat, uid);
    if (
      offset !== initialStat.size ||
      finalStat.size !== initialStat.size ||
      finalStat.dev !== initialStat.dev ||
      finalStat.ino !== initialStat.ino ||
      finalStat.mtimeMs !== initialStat.mtimeMs ||
      finalStat.ctimeMs !== initialStat.ctimeMs
    ) {
      throw new Error("provider candidate request record changed while read");
    }
    let pathStat;
    let physicalPath;
    try {
      pathStat = lstatSync(requestRecord);
      physicalPath = realpathSync(requestRecord);
    } catch {
      throw new Error(
        "provider candidate request record path changed while read",
      );
    }
    if (
      pathStat.isSymbolicLink() ||
      pathStat.dev !== initialStat.dev ||
      pathStat.ino !== initialStat.ino ||
      physicalPath !== requestRecord
    ) {
      throw new Error("provider candidate request record path changed while read");
    }
    raw = buffer.subarray(0, offset);
  } finally {
    close(descriptor);
  }
  let finalParent;
  try {
    finalParent = lstatSync(dirname(requestRecord));
  } catch {
    throw new Error("provider request record parent changed while read");
  }
  if (
    finalParent.dev !== parentStat.dev ||
    finalParent.ino !== parentStat.ino
  ) {
    throw new Error("provider request record parent changed while read");
  }
  if (!raw.equals(canonical)) {
    throw new Error("provider candidate request record binding mismatch");
  }
  return expected;
}

function assertProviderCandidateRequestRecordAbsent(repo, requestRecord) {
  inspectProviderRequestRecordParent(repo, requestRecord);
  try {
    lstatSync(requestRecord);
  } catch (error) {
    if (error?.code === "ENOENT") return;
    throw new Error("provider candidate request record state is unreadable");
  }
  throw new Error(
    "provider candidate request record already exists; use reconcile-candidate",
  );
}

function providerCandidateRunListArguments() {
  return [
    "api",
    "--paginate",
    "--slurp",
    `repos/${GITHUB_REPOSITORY}/actions/workflows/release.yml/runs?event=workflow_dispatch&per_page=100`,
  ];
}

function validateProviderCandidateRun(run, expected) {
  const runId = String(run?.id ?? "");
  const runAttempt = String(run?.run_attempt ?? "");
  const expectedURL =
    `https://github.com/${GITHUB_REPOSITORY}/actions/runs/${runId}`;
  const validStatuses = new Set([
    "completed",
    "in_progress",
    "pending",
    "queued",
    "requested",
    "waiting",
  ]);
  if (
    !POSITIVE_INTEGER.test(runId) ||
    !POSITIVE_INTEGER.test(runAttempt) ||
    !Number.isSafeInteger(Number(runId)) ||
    !Number.isSafeInteger(Number(runAttempt)) ||
    run.display_title !== expected.requestId ||
    run.event !== "workflow_dispatch" ||
    run.head_branch !== expected.tag ||
    run.head_sha !== expected.expectedCommit ||
    run.name !== expected.requestId ||
    run.path !== PROVIDER_CANDIDATE_WORKFLOW ||
    run.html_url !== expectedURL ||
    !validStatuses.has(run.status) ||
    (run.conclusion !== null && typeof run.conclusion !== "string")
  ) {
    throw new Error(
      `provider candidate request ${expected.requestId} matched a drifted workflow run`,
    );
  }
  return {
    runId,
    runAttempt,
    requestId: expected.requestId,
    url: run.html_url,
    status: run.status,
    conclusion: run.conclusion,
  };
}

function findProviderCandidateRun(context, expected) {
  let pages;
  try {
    const response = attemptCommand(
      context,
      "gh",
      providerCandidateRunListArguments(),
    );
    if (!response.ok) throw new Error("request failed");
    pages = JSON.parse(response.output);
  } catch {
    throw new Error("provider candidate workflow-run inventory is unreadable");
  }
  if (
    !Array.isArray(pages) ||
    pages.some(
      (page) =>
        !page ||
        typeof page !== "object" ||
        Array.isArray(page) ||
        !Number.isSafeInteger(page.total_count) ||
        page.total_count < 0 ||
        !Array.isArray(page.workflow_runs),
    )
  ) {
    throw new Error("provider candidate workflow-run pagination is invalid");
  }
  const runs = pages.flatMap((page) => page.workflow_runs);
  const totals = new Set(pages.map((page) => page.total_count));
  if (totals.size !== 1 || runs.length !== pages[0].total_count) {
    throw new Error(
      "provider candidate workflow-run pagination did not close exactly",
    );
  }
  const ids = new Set();
  for (const run of runs) {
    if (
      !run ||
      typeof run !== "object" ||
      Array.isArray(run) ||
      !Number.isSafeInteger(run.id) ||
      run.id <= 0 ||
      ids.has(run.id) ||
      typeof run.display_title !== "string"
    ) {
      throw new Error("provider candidate workflow-run inventory is invalid");
    }
    ids.add(run.id);
  }
  const matched = runs.filter(
    (run) => run.display_title === expected.requestId,
  );
  if (matched.length > 1) {
    throw new Error(
      `provider candidate request ${expected.requestId} is ambiguous across ${matched.length} workflow runs`,
    );
  }
  if (matched.length === 0) return null;
  return validateProviderCandidateRun(matched[0], expected);
}

function providerCandidateRequestFromOptions(options, currentCommit) {
  return providerCandidateRequestRecord({
    currentCommit,
    releaseCommit: options["expected-release-commit"],
    releaseTag: options.tag,
    requestId: options["request-id"],
    requestRecord: options["request-record"],
    tagObjectOid: options["expected-tag-object"],
  });
}

function assertProviderCandidateAuthority(
  context,
  options,
  descriptor,
  { ownerGate = false } = {},
) {
  const releaseCommit = options["expected-release-commit"];
  const expectedCurrentCommit = options["expected-current-commit"];
  const currentCommit = ownerGate
    ? providerOwnerGateAndFence(context, expectedCurrentCommit)
    : assertCurrentProviderReleaseBranch(context, expectedCurrentCommit);
  if (releaseCommit !== currentCommit) {
    assertProviderRecoveryFence(context, {
      releaseCommit,
      recoveryCommit: currentCommit,
      label: "provider candidate-dispatch reviewed recovery fence",
    });
  }
  assertExactSignedProviderTag(context, {
    tag: descriptor.tag,
    expectedCommit: releaseCommit,
    expectedObject: options["expected-tag-object"],
  });
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  return currentCommit;
}

function attemptProviderCandidateDispatch(context, options) {
  progress(context, "dispatch release.yml once from the exact signed tag");
  return attemptCommand(context, "gh", [
    "workflow",
    "run",
    "release.yml",
    "--repo",
    GITHUB_REPOSITORY,
    "--ref",
    options.tag,
    "-f",
    `tag=${options.tag}`,
    "-f",
    `expected_commit=${options["expected-release-commit"]}`,
    "-f",
    `request_id=${options["request-id"]}`,
  ]);
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

function assertTagOnlyRecoveryState(
  context,
  { tag, expectedCommit, expectedObject },
) {
  const local = localTagOID(context, tag);
  if (local !== expectedObject) {
    throw new Error(
      `tag-only recovery requires exact local annotated tag object ${expectedObject}; observed ${local || "absent"}`,
    );
  }
  const localType = git(context, "cat-file", "-t", `refs/tags/${tag}`);
  const localCommit = git(
    context,
    "rev-parse",
    `refs/tags/${tag}^{commit}`,
  );
  if (localType !== "tag" || localCommit !== expectedCommit) {
    throw new Error(
      `tag-only recovery local tag is not the exact annotated object/commit: ${JSON.stringify({
        object: local,
        type: localType,
        commit: localCommit,
      })}`,
    );
  }
  const remote = assertExactRemoteTag(
    context,
    tag,
    expectedCommit,
    expectedObject,
  );
  assertReleaseAbsent(context, tag);
  return { local: { object: local, type: localType, commit: localCommit }, remote };
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

function validateProviderSignedTagArtifactEvidence(
  raw,
  { tag, expectedCommit, runId, runAttempt, requestId },
) {
  let evidence;
  try {
    evidence = JSON.parse(raw);
  } catch {
    throw new Error("provider signed-tag verifier returned invalid JSON");
  }
  requireExactKeys(
    evidence,
    [
      "kind",
      "requestId",
      "releaseTag",
      "sourceCommit",
      "workflowRun",
      "preflightSha256",
      "tagObjectOid",
      "tagObjectSha256",
      "signerFingerprint",
      "localRefMaterialized",
      "verified",
    ],
    "provider signed-tag verification evidence",
  );
  const workflowRun =
    `https://github.com/${GITHUB_REPOSITORY}/actions/runs/${runId}` +
    `/attempts/${runAttempt}`;
  if (
    evidence.kind !== "takoform.provider-signed-tag-verification@v1" ||
    evidence.requestId !== requestId ||
    evidence.releaseTag !== tag ||
    evidence.sourceCommit !== expectedCommit ||
    evidence.workflowRun !== workflowRun ||
    !SHA256.test(evidence.preflightSha256 ?? "") ||
    !GIT_OBJECT.test(evidence.tagObjectOid ?? "") ||
    !SHA256.test(evidence.tagObjectSha256 ?? "") ||
    evidence.signerFingerprint !== PROVIDER_SIGNER ||
    evidence.localRefMaterialized !== false ||
    evidence.verified !== true
  ) {
    throw new Error("provider signed-tag verification evidence binding mismatch");
  }
  return evidence;
}

function exactProviderSignedTagState(
  context,
  { tag, expectedCommit, expectedObject },
) {
  const local = localTagOID(context, tag);
  const remote = remoteTagState(context, tag);
  const remoteAbsent = !remote.object && !remote.commit;
  const remoteExact =
    remote.object === expectedObject && remote.commit === expectedCommit;
  if (
    (local && local !== expectedObject) ||
    (!remoteAbsent && !remoteExact)
  ) {
    throw new Error(
      `provider signed tag state drift: local=${local || "absent"} remote=${JSON.stringify(remote)}`,
    );
  }
  return {
    local: local || null,
    remote,
    remoteAbsent,
    remoteExact,
  };
}

function providerTagMutationFence(
  context,
  descriptor,
  expectedCommit,
  expectedObject,
) {
  providerOwnerGateAndFence(context, expectedCommit);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  return exactProviderSignedTagState(context, {
    tag: descriptor.tag,
    expectedCommit,
    expectedObject,
  });
}

function materializeExactProviderTagRef(
  context,
  tag,
  expectedObject,
) {
  progress(context, `materialize create-only local tag ${tag}`);
  command(context, "git", [
    "update-ref",
    `refs/tags/${tag}`,
    expectedObject,
    "0".repeat(expectedObject.length),
  ]);
  const local = localTagOID(context, tag);
  if (local !== expectedObject) {
    throw new Error("provider local signed tag readback differs after creation");
  }
}

function pushProviderTagWithAcknowledgementRecovery(
  context,
  tag,
  expectedCommit,
  expectedObject,
) {
  progress(context, `push create-only tag ${tag}`);
  const result = attemptCommand(
    context,
    "git",
    [
      "push",
      "--no-verify",
      `--force-with-lease=refs/tags/${tag}:`,
      SOURCE_REPOSITORY,
      `refs/tags/${tag}:refs/tags/${tag}`,
    ],
    { env: gitPushEnvironment() },
  );
  let remote;
  try {
    remote = assertExactRemoteTag(
      context,
      tag,
      expectedCommit,
      expectedObject,
    );
  } catch (error) {
    if (!result.ok) {
      if (result.output) context.stdout.write(result.output);
      if (result.stderr) context.stderr.write(result.stderr);
    }
    throw error;
  }
  return {
    remote,
    acknowledgement: result.ok ? "ACKNOWLEDGED" : "LOST_ACK_RECONCILED",
  };
}

function reconcileExactProviderSignedTag(
  context,
  descriptor,
  expectedCommit,
  expectedObject,
) {
  const actions = [];
  let state = exactProviderSignedTagState(context, {
    tag: descriptor.tag,
    expectedCommit,
    expectedObject,
  });
  if (!state.local) {
    state = providerTagMutationFence(
      context,
      descriptor,
      expectedCommit,
      expectedObject,
    );
    if (state.local) {
      throw new Error(
        "provider local signed tag changed before create-only materialization",
      );
    }
    materializeExactProviderTagRef(context, descriptor.tag, expectedObject);
    actions.push("LOCAL_REF_MATERIALIZED");
    state = exactProviderSignedTagState(context, {
      tag: descriptor.tag,
      expectedCommit,
      expectedObject,
    });
  }
  if (state.remoteAbsent) {
    state = providerTagMutationFence(
      context,
      descriptor,
      expectedCommit,
      expectedObject,
    );
    if (state.local !== expectedObject || !state.remoteAbsent) {
      throw new Error(
        "provider signed tag state changed before create-only push",
      );
    }
    const pushed = pushProviderTagWithAcknowledgementRecovery(
      context,
      descriptor.tag,
      expectedCommit,
      expectedObject,
    );
    actions.push(
      pushed.acknowledgement === "ACKNOWLEDGED"
        ? "REMOTE_PUSHED"
        : "REMOTE_PUSH_ACK_LOST_RECONCILED",
    );
  }
  const finalState = exactProviderSignedTagState(context, {
    tag: descriptor.tag,
    expectedCommit,
    expectedObject,
  });
  if (finalState.local !== expectedObject || !finalState.remoteExact) {
    throw new Error("provider signed tag reconciliation did not close exactly");
  }
  if (actions.length === 0) actions.push("EXACT_TAG_ALREADY_RECONCILED");
  return { actions, state: finalState };
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
    targetCommitish = "main",
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
    draft.target_commitish !== targetCommitish ||
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
  {
    releaseId,
    tag,
    prerelease,
    body,
    assets,
    targetCommitish = "main",
  },
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
    response.target_commitish !== targetCommitish ||
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
    targetCommitish = "main",
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
      targetCommitish,
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
      targetCommitish,
      requireComplete: true,
    });
    prePublishFence(releaseId);
    readExactRetainedDraft(context, {
      releaseId,
      tag,
      prerelease,
      body,
      assets,
      targetCommitish,
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
          `target_commitish=${targetCommitish}`,
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
      targetCommitish,
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
      expectedTargetCommitish: targetCommitish,
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
    targetCommitish = "main",
    prePublishFence = () => {},
  },
) {
  assertReleaseAbsent(context, tag);
  const exactIdentity = strictIdentity || targetCommitish !== "main";
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
        ...(exactIdentity
          ? ["-f", `target_commitish=${targetCommitish}`]
          : []),
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
    if (exactIdentity) {
      const initial = readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        targetCommitish,
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
    if (exactIdentity) {
      readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        targetCommitish,
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
    if (exactIdentity) {
      readExactRetainedDraft(context, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        targetCommitish,
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
        ...(exactIdentity
          ? [
              "-f",
              `tag_name=${tag}`,
              "-f",
              `target_commitish=${targetCommitish}`,
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
    if (exactIdentity) {
      validateExactReleasePatchResponse(patchResponse, {
        releaseId,
        tag,
        prerelease,
        body,
        assets,
        targetCommitish,
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
      ...(exactIdentity
        ? {
            expectedReleaseId: releaseId,
            expectedName: tag,
            expectedBody: body,
            expectedTargetCommitish: targetCommitish,
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

function registryStatus(context, version) {
  const status = command(context, "curl", [
    "--silent",
    "--show-error",
    "--location",
    "--output",
    "/dev/null",
    "--write-out",
    "%{http_code}",
    `https://registry.terraform.io/v1/providers/tako0614/takoform/${version}/download/linux/amd64`,
  ]).trim();
  if (!/^[0-9]{3}$/u.test(status)) {
    throw new Error(`Terraform Registry returned invalid HTTP status ${status}`);
  }
  return status;
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

function providerProvenanceExpectations(root, descriptor) {
  const names = providerAssetNames(descriptor);
  const statement = readJSON(
    join(root, names.provenance),
    "provider release provenance",
  );
  const build = statement.predicate?.buildDefinition;
  const external = build?.externalParameters;
  const internal = build?.internalParameters;
  const expected = {
    sourceCommit: internal?.sourceCommit,
    toolingCommit: internal?.toolingCommit,
    requestId: external?.requestId,
    runId: String(internal?.run?.id ?? ""),
    runAttempt: String(internal?.run?.attempt ?? ""),
    tagObjectOid: internal?.tagObject?.oid,
    tagObjectSha256: internal?.tagObject?.sha256,
  };
  if (
    external?.tag !== descriptor.tag ||
    !COMMIT.test(expected.sourceCommit ?? "") ||
    !COMMIT.test(expected.toolingCommit ?? "") ||
    !REQUEST_ID.test(expected.requestId ?? "") ||
    !POSITIVE_INTEGER.test(expected.runId) ||
    !POSITIVE_INTEGER.test(expected.runAttempt) ||
    !/^[0-9a-f]{40,64}$/u.test(expected.tagObjectOid ?? "") ||
    !SHA256.test(expected.tagObjectSha256 ?? "")
  ) {
    throw new Error("provider release provenance expected bindings are invalid");
  }
  return expected;
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

function providerReleaseBody(descriptor) {
  return (
    "Signed deterministic Takoform provider release. Provider publication does not activate Standard Forms.\n\nBreaking upgrade from v0.2.1: follow the migration guide before using provider v1 with existing state: " +
    `https://github.com/${GITHUB_REPOSITORY}/blob/${descriptor.tag}/release/migrations/v0.2.1-to-v1.0.1.md`
  );
}

function providerPrepare(context, options, descriptor) {
  const commit = assertCurrentProviderReleaseBranch(
    context,
    options["expected-commit"],
  );
  assertTagAbsent(context, descriptor.tag);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  providerOwnerGateAndFence(context, commit);
  const dispatched = dispatchWorkflow(
    context,
    "provider-release-tag.yml",
    {
      tag: descriptor.tag,
      expected_commit: commit,
    },
    {
      headSha: commit,
      ref: PROVIDER_RELEASE_BRANCH,
      headBranch: PROVIDER_RELEASE_BRANCH,
    },
  );
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
  assertCurrentProviderReleaseBranch(context, expectedCommit);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  providerOwnerGateAndFence(context, expectedCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Author provider release tag",
      headSha: expectedCommit,
      headBranch: PROVIDER_RELEASE_BRANCH,
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
    let tagObject = "";
    try {
      assertCurrentProviderReleaseBranch(context, expectedCommit);
      progress(context, "verify exact signed provider tag artifact");
      const verification = validateProviderSignedTagArtifactEvidence(
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
          ],
          { label: "verify provider signed-tag artifacts" },
        ),
        {
          tag: descriptor.tag,
          expectedCommit,
          runId: options["run-id"],
          runAttempt: options["run-attempt"],
          requestId: run.displayTitle,
        },
      );
      tagObject = verification.tagObjectOid;
      const reconciled = reconcileExactProviderSignedTag(
        context,
        descriptor,
        expectedCommit,
        tagObject,
      );
      assertCurrentProviderReleaseBranch(context, expectedCommit);
      assertReleaseAbsent(context, descriptor.tag);
      assertRegistryVersionAbsent(context, descriptor.version);
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: PROVIDER_SURFACE,
        phase: "tag",
        commit: expectedCommit,
        tag: descriptor.tag,
        tagObject,
        reviewedRun: { id: options["run-id"], attempt: options["run-attempt"], url: run.url },
        tagReconciliation: reconciled.actions.join("+"),
        dispatchStatus: "NOT_DISPATCHED",
        nextPhase: "prepare-candidate",
        status: "TAG_READY",
      });
    } catch (error) {
      reportTagFailure(context, descriptor.tag, tagObject, {
        surface: PROVIDER_SURFACE,
        phase: "tag",
      });
      throw error;
    }
  });
}

function providerPrepareCandidate(context, options, descriptor) {
  assertProviderCandidateRequestRecordAbsent(
    context.repo,
    options["request-record"],
  );
  const currentCommit = assertProviderCandidateAuthority(
    context,
    options,
    descriptor,
  );
  let request = providerCandidateRequestFromOptions(options, currentCommit);
  const expectedRun = {
    requestId: request.requestId,
    tag: descriptor.tag,
    expectedCommit: request.releaseCommit,
  };
  if (findProviderCandidateRun(context, expectedRun)) {
    throw new Error(
      `provider candidate request ${request.requestId} already has a workflow run; create mode cannot adopt it`,
    );
  }

  const fencedCommit = assertProviderCandidateAuthority(
    context,
    options,
    descriptor,
    { ownerGate: true },
  );
  request = providerCandidateRequestFromOptions(options, fencedCommit);
  if (findProviderCandidateRun(context, expectedRun)) {
    throw new Error(
      `provider candidate request ${request.requestId} already has a workflow run; create mode cannot adopt it`,
    );
  }
  createProviderCandidateRequestRecord(
    context.repo,
    options["request-record"],
    request,
  );
  const attempted = attemptProviderCandidateDispatch(context, options);
  if (!attempted.ok) {
    throw new Error(
      "provider candidate dispatch acknowledgement is unresolved; preserve the request record and use reconcile-candidate only",
    );
  }
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: PROVIDER_SURFACE,
    phase: "prepare-candidate",
    target: PROVIDER_ADDRESS,
    commit: request.currentCommit,
    releaseCommit: request.releaseCommit,
    tag: request.releaseTag,
    tagObject: request.tagObjectOid,
    requestId: request.requestId,
    requestRecordSha256: sha256(
      Buffer.from(
        `${JSON.stringify(recursivelySorted(request))}\n`,
        "utf8",
      ),
    ),
    dispatchStatus: "ATTEMPTED_ONCE",
    nextPhase: "reconcile-candidate",
    status: "RECONCILIATION_REQUIRED",
  });
}

function providerReconcileCandidate(context, options, descriptor) {
  verifyLocalReleaseToolchain(context);
  const currentCommit = assertProviderCandidateAuthority(
    context,
    options,
    descriptor,
  );
  const expected = providerCandidateRequestFromOptions(
    options,
    currentCommit,
  );
  const request = readProviderCandidateRequestRecord(
    context.repo,
    options["request-record"],
    expected,
  );
  const run = findProviderCandidateRun(context, {
    requestId: request.requestId,
    tag: request.releaseTag,
    expectedCommit: request.releaseCommit,
  });
  if (!run) {
    return emit(context, {
      kind: "takos.deploy-result@v1",
      surface: PROVIDER_SURFACE,
      phase: "reconcile-candidate",
      target: PROVIDER_ADDRESS,
      commit: request.currentCommit,
      releaseCommit: request.releaseCommit,
      tag: request.releaseTag,
      tagObject: request.tagObjectOid,
      requestId: request.requestId,
      dispatchStatus: "NOT_DISPATCHED",
      status: "UNRESOLVED_ABSENT",
    });
  }
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: PROVIDER_SURFACE,
    phase: "reconcile-candidate",
    target: PROVIDER_ADDRESS,
    commit: request.currentCommit,
    releaseCommit: request.releaseCommit,
    tag: request.releaseTag,
    tagObject: request.tagObjectOid,
    requestId: request.requestId,
    dispatchStatus: "NOT_DISPATCHED",
    status: "AWAITING_REVIEW",
    workflowRun: run,
  });
}

function providerPublish(context, options, descriptor) {
  const expectedCommit = options["expected-commit"];
  const releaseBody = providerReleaseBody(descriptor);
  if (
    !releaseBody.includes("Breaking upgrade from v0.2.1") ||
    !releaseBody.includes("release/migrations/v0.2.1-to-v1.0.1.md")
  ) {
    throw new Error("provider release body omits the exact v1 migration guide");
  }
  assertCurrentProviderReleaseBranch(context, expectedCommit);
  const localObject = localTagOID(context, descriptor.tag);
  if (!localObject) throw new Error(`local signed provider tag ${descriptor.tag} is missing`);
  assertExactRemoteTag(context, descriptor.tag, expectedCommit, localObject);
  assertReleaseAbsent(context, descriptor.tag);
  assertRegistryVersionAbsent(context, descriptor.version);
  providerOwnerGateAndFence(context, expectedCommit);
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
    providerOwnerGateAndFence(context, expectedCommit);
    assertRegistryVersionAbsent(context, descriptor.version);
    const release = publishReleaseLocally(context, {
      tag: descriptor.tag,
      assets,
      prerelease: descriptor.version.includes("-"),
      body: releaseBody,
      temporaryRoot,
      targetCommitish: PROVIDER_RELEASE_BRANCH,
      prePublishFence: () => {
        providerOwnerGateAndFence(context, expectedCommit);
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
  const currentMaintenance = providerOwnerGateAndFence(
    context,
    recoveryCommit,
  );
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: currentMaintenance,
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
  return { currentMaintenance, tag };
}

function providerRecoverTagOnly(context, options, descriptor) {
  const releaseCommit = options["expected-release-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const initialMaintenance = assertCurrentProviderReleaseBranch(
    context,
    recoveryCommit,
  );
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: initialMaintenance,
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
          targetCommitish: PROVIDER_RELEASE_BRANCH,
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
  const initialMaintenance = assertCurrentProviderReleaseBranch(
    context,
    recoveryCommit,
  );
  assertProviderRecoveryFence(context, {
    releaseCommit,
    recoveryCommit: initialMaintenance,
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
          targetCommitish: PROVIDER_RELEASE_BRANCH,
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

function readProviderPublicRelease(
  context,
  descriptor,
  temporaryRoot,
  releaseCommit,
) {
  const names = providerAssetNames(descriptor);
  const release = getRelease(context, descriptor.tag);
  if (!release || !Array.isArray(release.assets)) {
    throw new Error("provider GitHub Release is unavailable");
  }
  const assets = new Map(
    release.assets.map((asset) => [
      asset.name,
      { name: asset.name, sha256: asset.digest, path: null },
    ]),
  );
  if (
    assets.size !== names.all.length ||
    names.all.some((name) => !SHA256.test(assets.get(name)?.sha256 ?? ""))
  ) {
    throw new Error("provider GitHub Release inventory is not the exact 15 assets");
  }
  const output = join(temporaryRoot, "provider-public");
  mkdirSync(output);
  command(context, "gh", [
    "release",
    "download",
    descriptor.tag,
    "--repo",
    GITHUB_REPOSITORY,
    "--dir",
    output,
  ]);
  for (const [name, asset] of assets) {
    asset.path = join(output, name);
  }
  validateReleaseReadback(release, descriptor.tag, assets, {
    prerelease: descriptor.version.includes("-"),
    expectedReleaseId: release.id,
    expectedName: descriptor.tag,
    expectedBody: providerReleaseBody(descriptor),
    expectedTargetCommitish: PROVIDER_RELEASE_BRANCH,
    expectedAssetsURL:
      `https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/${release.id}/assets`,
    expectedUploadURL:
      `https://uploads.github.com/repos/${GITHUB_REPOSITORY}/releases/${release.id}/assets{?name,label}`,
  });
  const actual = listRegularFiles(output);
  if (JSON.stringify(actual) !== JSON.stringify(names.all)) {
    throw new Error("downloaded provider release inventory differs");
  }
  for (const [name, asset] of assets) {
    if (fileDigest(asset.path) !== asset.sha256) {
      throw new Error(`provider release readback digest mismatch: ${name}`);
    }
  }
  verifyNamedChecksums(output, names.checksum, [
    ...names.archives,
    names.manifest,
  ]);
  verifyProviderSignature(context, output, names);
  const provenance = providerProvenanceExpectations(output, descriptor);
  const localObject = localTagOID(context, descriptor.tag);
  const tagObject = command(context, "git", [
    "cat-file",
    "tag",
    provenance.tagObjectOid,
  ]);
  if (
    provenance.sourceCommit !== releaseCommit ||
    provenance.toolingCommit !== releaseCommit ||
    provenance.tagObjectOid !== localObject ||
    sha256(Buffer.from(tagObject, "utf8")) !== provenance.tagObjectSha256
  ) {
    throw new Error(
      "public provider provenance does not bind the current exact signed tag",
    );
  }
  verifyProviderReleaseProvenance(
    context,
    output,
    descriptor,
    provenance,
  );
  command(context, "go", [
    "-C",
    "cmd/provider-release",
    "run",
    ".",
    "verify-sbom",
    ...names.archives.map((name) => join(output, `${name}.spdx.json`)),
  ]);
  return { release, assets };
}

function assertProviderReadbackCommitBinding(
  context,
  { sourceCommit, providerReleaseCommit },
) {
  assertCommitAncestor(
    context,
    providerReleaseCommit,
    sourceCommit,
    "provider readback release/current-source ancestry",
  );
  return { sourceCommit, providerReleaseCommit };
}

function providerReadback(context, options, descriptor) {
  const currentCommit = options["expected-commit"];
  assertCurrentProviderReleaseBranch(context, currentCommit);
  const localObject = localTagOID(context, descriptor.tag);
  if (!localObject) throw new Error(`local provider tag ${descriptor.tag} is missing`);
  const releaseCommit = git(
    context,
    "rev-list",
    "-n",
    "1",
    descriptor.tag,
  );
  assertProviderReadbackCommitBinding(context, {
    sourceCommit: currentCommit,
    providerReleaseCommit: releaseCommit,
  });
  assertExactRemoteTag(
    context,
    descriptor.tag,
    releaseCommit,
    localObject,
  );
  return withTemporaryDirectory("takoform-provider-readback", (temporaryRoot) => {
    readProviderPublicRelease(
      context,
      descriptor,
      temporaryRoot,
      releaseCommit,
    );
    const status = registryStatus(context, descriptor.version);
    if (status !== "200") {
      throw new Error(
        `Terraform Registry has not indexed ${descriptor.version}: HTTP ${status}`,
      );
    }
    providerOwnerGateAndFence(context, currentCommit);
    const dispatched = dispatchWorkflow(
      context,
      "provider-registry-readback.yml",
      {},
      {
        headSha: currentCommit,
        ref: PROVIDER_RELEASE_BRANCH,
        headBranch: PROVIDER_RELEASE_BRANCH,
      },
    );
    return emit(context, {
      kind: "takos.deploy-result@v1",
      surface: PROVIDER_SURFACE,
      phase: "readback",
      target: PROVIDER_ADDRESS,
      commit: currentCommit,
      sourceCommit: currentCommit,
      providerReleaseCommit: releaseCommit,
      tag: descriptor.tag,
      dispatchStatus: "DISPATCHED",
      status: "AWAITING_REVIEW",
      workflowRun: dispatched,
    });
  });
}

function providerVerify(context, options, descriptor) {
  const expectedCommit = options["expected-commit"];
  assertCurrentProviderReleaseBranch(context, expectedCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Build signed provider Registry readback",
      headSha: expectedCommit,
      headBranch: PROVIDER_RELEASE_BRANCH,
    },
  );
  return withTemporaryDirectory("takoform-registry-proof", (temporaryRoot) => {
    const candidate = downloadArtifact(
      context,
      options["run-id"],
      `takoform-provider-registry-readback-candidate-${run.displayTitle}-${options["run-id"]}-${options["run-attempt"]}-${expectedCommit.slice(0, 12)}`,
      join(temporaryRoot, "candidate"),
    );
    const proof = verifyRegistryCandidate(context, candidate, {
      descriptor,
      expectedCommit,
      runId: options["run-id"],
      runAttempt: options["run-attempt"],
      requestId: run.displayTitle,
    });
    return emit(context, {
      kind: "takos.deploy-result@v1",
      surface: PROVIDER_SURFACE,
      phase: "verify",
      target: PROVIDER_ADDRESS,
      commit: expectedCommit,
      sourceCommit: expectedCommit,
      providerReleaseCommit: proof.providerReleaseCommit,
      tag: descriptor.tag,
      candidateRun: { id: options["run-id"], attempt: options["run-attempt"], url: run.url },
      candidateDigest: fileDigest(join(candidate, "SHA256SUMS")),
      installedProviderDigest: proof.installedProviderDigest,
      status: "VERIFIED",
    });
  });
}

function verifyRegistryCandidate(
  context,
  root,
  { descriptor, expectedCommit, runId, runAttempt, requestId },
) {
  verifyLocalReleaseToolchain(context);
  const expectedFiles = [
    "SHA256SUMS",
    "provider-lifecycle-matrix.json",
    "provider-readback.json",
    "provider-readback.sigstore.json",
    "provider-registry-readback-manifest.json",
    "signed-provider-registry-readback-candidate.json",
  ].sort();
  verifyChecksumClosure(root);
  if (JSON.stringify(listRegularFiles(root)) !== JSON.stringify(expectedFiles)) {
    throw new Error("Registry readback candidate is not the exact six-file closure");
  }
  const manifestRaw = readFileSync(
    join(root, "provider-registry-readback-manifest.json"),
  );
  const manifest = JSON.parse(manifestRaw);
  const signedRaw = readFileSync(
    join(root, "signed-provider-registry-readback-candidate.json"),
  );
  const signed = JSON.parse(signedRaw);
  requireCanonicalSortedJSON(
    manifestRaw,
    manifest,
    "Registry readback manifest",
  );
  requireCanonicalSortedJSON(
    signedRaw,
    signed,
    "signed Registry readback candidate",
  );
  requireExactKeys(
    manifest,
    [
      "format",
      "status",
      "proofType",
      "generation",
      "source",
      "provider",
      "matrix",
      "readback",
    ],
    "Registry readback manifest",
  );
  requireExactKeys(manifest.source, ["repository", "commit"], "Registry source");
  requireExactKeys(
    manifest.provider,
    ["address", "version", "tag", "commit"],
    "Registry provider",
  );
  requireExactKeys(manifest.matrix, ["path", "digest"], "Registry matrix");
  requireExactKeys(
    manifest.readback,
    ["path", "digest", "bundlePath"],
    "Registry readback",
  );
  requireExactKeys(
    signed,
    [
      "format",
      "status",
      "proofType",
      "generation",
      "certificateIdentity",
      "workflow",
      "requestId",
      "workflowRunId",
      "workflowRunAttempt",
      "source",
      "manifest",
      "readback",
    ],
    "signed Registry readback candidate",
  );
  requireExactKeys(signed.source, ["repository", "commit"], "signed Registry source");
  requireExactKeys(signed.manifest, ["path", "digest"], "signed Registry manifest");
  requireExactKeys(
    signed.readback,
    ["path", "digest", "bundlePath", "bundleDigest"],
    "signed Registry readback",
  );
  const providerCommit = git(context, "rev-list", "-n", "1", descriptor.tag);
  assertProviderReadbackCommitBinding(context, {
    sourceCommit: expectedCommit,
    providerReleaseCommit: providerCommit,
  });
  const certificateIdentity =
    `https://github.com/${GITHUB_REPOSITORY}/.github/workflows/` +
    `provider-registry-readback.yml@${PROVIDER_RELEASE_REF}`;
  if (
    manifest.format !== "takoform.provider-registry-readback-candidate@v1" ||
    manifest.status !== "candidate-only" ||
    manifest.proofType !== "registry-readback" ||
    manifest.generation !== "portable-v1" ||
    manifest.source.repository !== SOURCE_REPOSITORY ||
    manifest.source?.commit !== expectedCommit ||
    manifest.provider?.address !== PROVIDER_ADDRESS ||
    manifest.provider?.tag !== descriptor.tag ||
    manifest.provider?.version !== descriptor.version ||
    manifest.provider?.commit !== providerCommit ||
    manifest.matrix?.path !== "provider-lifecycle-matrix.json" ||
    manifest.matrix?.digest !==
      fileDigest(join(root, "provider-lifecycle-matrix.json")) ||
    manifest.readback?.path !== "provider-readback.json" ||
    manifest.readback?.bundlePath !== "provider-readback.sigstore.json" ||
    manifest.readback?.digest !==
      fileDigest(join(root, "provider-readback.json")) ||
    signed.format !==
      "takoform.provider-registry-readback-signed-candidate@v1" ||
    signed.status !== "candidate-only" ||
    signed.proofType !== "registry-readback" ||
    signed.generation !== "portable-v1" ||
    signed.certificateIdentity !== certificateIdentity ||
    signed.workflow !==
      ".github/workflows/provider-registry-readback.yml" ||
    signed.requestId !== requestId ||
    !REQUEST_ID.test(signed.requestId ?? "") ||
    signed.workflowRunId?.toString() !== runId ||
    normalizedPositiveInteger(
      signed.workflowRunAttempt,
      "signed Registry workflowRunAttempt",
    ) !== runAttempt ||
    signed.source?.repository !== SOURCE_REPOSITORY ||
    signed.source?.commit !== expectedCommit ||
    signed.manifest?.path !==
      "provider-registry-readback-manifest.json" ||
    signed.manifest?.digest !== sha256(manifestRaw) ||
    signed.readback?.path !== "provider-readback.json" ||
    signed.readback?.digest !== manifest.readback.digest ||
    signed.readback?.bundlePath !== "provider-readback.sigstore.json" ||
    signed.readback?.bundleDigest !==
      fileDigest(join(root, "provider-readback.sigstore.json"))
  ) {
    throw new Error("Registry readback signed metadata binding mismatch");
  }
  const rebuilt = command(context, "go", [
    "run",
    "./cmd/admission-readback",
    "registry",
    "--matrix",
    join(root, "provider-lifecycle-matrix.json"),
    "--provider-release-commit",
    providerCommit,
  ]);
  const retained = readFileSync(join(root, "provider-readback.json"), "utf8");
  if (rebuilt !== retained) {
    throw new Error("Registry readback is not the canonical semantic rebuild");
  }
  const readback = JSON.parse(retained);
  if (
    !Array.isArray(readback.installs) ||
    readback.installs.length !== 2 ||
    JSON.stringify(
      readback.installs.map((install) => install.product).sort(),
    ) !== JSON.stringify(["OpenTofu", "Terraform"]) ||
    readback.installs.some(
      (install) => !SHA256.test(install.providerBinarySha256 ?? ""),
    ) ||
    new Set(
      readback.installs.map((install) => install.providerBinarySha256),
    ).size !== 1
  ) {
    throw new Error(
      "Registry readback must bind Terraform and OpenTofu to one provider binary digest",
    );
  }
  command(context, "cosign", [
    "verify-blob",
    join(root, "provider-readback.json"),
    "--bundle",
    join(root, "provider-readback.sigstore.json"),
    "--trusted-root",
    join(context.repo, TRUSTED_ROOT),
    "--certificate-identity",
    certificateIdentity,
    "--certificate-oidc-issuer",
    OIDC_ISSUER,
  ]);
  return {
    installedProviderDigest: readback.installs[0].providerBinarySha256,
    providerReleaseCommit: providerCommit,
  };
}

function formAssetNames(entry) {
  const base = `takoform-form-${entry.releaseId}_${entry.version}`;
  return {
    base,
    index: `${base}_package-index.json`,
    bundle: `${base}_package-index.sigstore.json`,
    all: [
      `${base}.tar.gz`,
      `${base}_package-index.json`,
      `${base}_package-index.sigstore.json`,
      `${base}_provenance.intoto.json`,
      `${base}_sbom.spdx.json`,
      "release-manifest.json",
      "SHA256SUMS",
    ].sort(),
  };
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

function verifyFormSemanticClosure(
  context,
  root,
  entry,
  sourceCommit,
  toolingCommit,
  trustedRoot,
) {
  const raw = command(context, "go", [
    "run",
    "./cmd/published-package-set",
    "verify-plan-directory",
    "--asset-root",
    root,
    "--source-root",
    context.repo,
    "--kind",
    entry.kind,
    "--tag",
    entry.tag,
    "--source-commit",
    sourceCommit,
    "--tooling-commit",
    toolingCommit,
    "--trusted-root",
    trustedRoot,
  ]);
  const report = JSON.parse(raw);
  if (
    report.kind !== entry.kind ||
    report.releaseId !== entry.releaseId ||
    report.version !== entry.version
  ) {
    throw new Error("Form Package deep semantic report plan binding mismatch");
  }
  return report;
}

function verifyFormPublicAssets(context, root, entry, { metadata } = {}) {
  const names = formAssetNames(entry);
  const actual = listRegularFiles(root);
  if (JSON.stringify(actual) !== JSON.stringify(names.all)) {
    throw new Error(`Form Package asset inventory differs: ${actual.join(", ")}`);
  }
  const manifest = readJSON(
    join(root, "release-manifest.json"),
    "Form Package release manifest",
  );
  if (
    manifest.schemaVersion !== 1 ||
    manifest.releaseType !== "form-package" ||
    manifest.tag !== entry.tag ||
    manifest.sourceRepository !== `github.com/${GITHUB_REPOSITORY}` ||
    !COMMIT.test(manifest.sourceCommit ?? "") ||
    !COMMIT.test(manifest.toolingCommit ?? "") ||
    manifest.workflow !== ".github/workflows/form-package-release.yml" ||
    manifest.packageVersion !== entry.version ||
    manifest.releaseId !== entry.releaseId ||
    manifest.packageDigest !== entry.packageDigest ||
    JSON.stringify(manifest.formRef) !== JSON.stringify(entry.formRef) ||
    manifest.signedSubject !== names.index ||
    manifest.signatureBundle !== names.bundle ||
    manifest.publicationReady !== true ||
    !Array.isArray(manifest.publicationBlockers) ||
    manifest.publicationBlockers.length !== 0 ||
    !Array.isArray(manifest.assets) ||
    manifest.assets.length !== 5
  ) {
    throw new Error("Form Package manifest does not bind the exact plan entry");
  }
  if (
    metadata &&
    (manifest.sourceCommit !== metadata.sourceCommit ||
      manifest.toolingCommit !== metadata.toolingCommit)
  ) {
    throw new Error("Form Package manifest commit binding differs from candidate");
  }
  const manifestAssets = new Map();
  for (const asset of manifest.assets) {
    if (
      typeof asset.name !== "string" ||
      !SHA256.test(asset.digest ?? "") ||
      manifestAssets.has(asset.name) ||
      fileDigest(join(root, asset.name)) !== asset.digest
    ) {
      throw new Error(`Form Package manifest asset is invalid: ${asset?.name}`);
    }
    manifestAssets.set(asset.name, asset.digest);
  }
  const checksumTargets = [
    ...manifestAssets.keys(),
    "release-manifest.json",
  ].sort();
  verifyNamedChecksums(root, "SHA256SUMS", checksumTargets);
  if (fileDigest(join(root, names.index)) !== entry.packageDigest) {
    throw new Error("signed package index digest differs from release plan");
  }
  withTrustedRootAtCommit(
    context,
    manifest.toolingCommit,
    (trustedRoot, trustedRootDigest) => {
      const deepReport = verifyFormSemanticClosure(
        context,
        root,
        entry,
        manifest.sourceCommit,
        manifest.toolingCommit,
        trustedRoot,
      );
      verifyDeepAssetReport(context, root, deepReport, {
        format: "takoform.form-package-directory-verification@v1",
        label: "Form Package",
        names: names.all,
        tag: entry.tag,
        sourceCommit: manifest.sourceCommit,
        toolingCommit: manifest.toolingCommit,
        trustedRootDigest,
      });
      verifySigstoreSubject(
        context,
        root,
        names.index,
        names.bundle,
        ".github/workflows/form-package-release.yml",
        trustedRoot,
      );
    },
  );
  return manifest;
}

function verifyFormCandidate(
  context,
  root,
  { entry, runId, runAttempt, requestId, expectedCommit, toolingCommit },
) {
  const metadata = verifyCandidateRoot(root, { tagObject: true });
  requireExactKeys(
    metadata,
    [
      "format",
      "repository",
      "workflowPath",
      "workflowRef",
      "sourceRef",
      "runId",
      "runAttempt",
      "tag",
      "sourceCommit",
      "toolingCommit",
      "objectFormat",
      "tagObjectOid",
      "tagObjectSha256",
      "requestId",
      "assets",
    ],
    "Form Package candidate metadata",
  );
  if (
    metadata.format !== "takoform.form-package-release-candidate@v1" ||
    metadata.repository !== GITHUB_REPOSITORY ||
    metadata.workflowPath !== ".github/workflows/form-package-release.yml" ||
    metadata.workflowRef !==
      `${GITHUB_REPOSITORY}/.github/workflows/form-package-release.yml@refs/heads/main` ||
    metadata.sourceRef !== "refs/heads/main" ||
    metadata.requestId !== requestId ||
    normalizedPositiveInteger(metadata.runId, "metadata runId") !== runId ||
    normalizedPositiveInteger(metadata.runAttempt, "metadata runAttempt") !==
      runAttempt ||
    metadata.tag !== entry.tag ||
    metadata.sourceCommit !== expectedCommit ||
    metadata.toolingCommit !== toolingCommit
  ) {
    throw new Error("Form Package candidate metadata binding mismatch");
  }
  const names = formAssetNames(entry);
  const assets = validateCandidateAssets(root, metadata, names.all);
  verifyFormPublicAssets(context, join(root, "assets"), entry, { metadata });
  verifyTagObjectWorkflowBinding(
    context,
    root,
    metadata,
    runId,
    runAttempt,
  );
  return { assets, metadata };
}

function expectedFormTagObject(
  context,
  { tag, sourceCommit, requestId, runId, runAttempt, revocation = false },
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
  const actor = revocation
    ? "Takoform Form Package Revocation"
    : "Takoform Form Package Release";
  const title = revocation
    ? `Takoform Form Package revocation checkpoint ${tag}`
    : `Takoform Form Package ${tag}`;
  return (
    `object ${sourceCommit}\n` +
    "type commit\n" +
    `tag ${tag}\n` +
    `tagger ${actor} <release@takoform.invalid> ${epoch} +0000\n\n` +
    `${title}\n\n` +
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
  { revocation = false } = {},
) {
  const text = readFileSync(join(root, "tag-object"), "utf8");
  const expected = expectedFormTagObject(context, {
    tag: metadata.tag,
    sourceCommit: metadata.sourceCommit,
    requestId: metadata.requestId,
    runId,
    runAttempt,
    revocation,
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
    { revocation: true },
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

function formPlan(context, plan) {
  const commit = assertCurrentProtectedMain(context);
  const commands = plan.releases.map(
    (entry) =>
      `bun run deploy -- ${FORM_SURFACE} prepare --tag ${entry.tag} --expected-commit ${commit}`,
  );
  context.stdout.write(`${commands.join("\n")}\n`);
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: FORM_SURFACE,
    phase: "plan",
    generation: plan.generation,
    commit,
    releases: commands.length,
    status: "VERIFIED",
  });
}

function formPrepare(context, options, plan) {
  const entry = plannedRelease(plan, options.tag);
  const sourceCommit = options["expected-commit"];
  const commit = assertCurrentProtectedMain(context, sourceCommit, {
    exact: false,
  });
  assertTagAbsent(context, entry.tag);
  assertReleaseAbsent(context, entry.tag);
  ownerGateAndFence(context, commit);
  const dispatched = dispatchWorkflow(context, "form-package-release.yml", {
    tag: entry.tag,
    expected_commit: sourceCommit,
  }, { headSha: commit });
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: FORM_SURFACE,
    phase: "prepare",
    commit: sourceCommit,
    toolingCommit: commit,
    tag: entry.tag,
    packageDigest: entry.packageDigest,
    dispatchStatus: "DISPATCHED",
    status: "AWAITING_REVIEW",
    workflowRun: dispatched,
  });
}

function readFormPublishBatch(
  input,
  plan,
  {
    open = openSync,
    fstat = fstatSync,
    read = readSync,
    close = closeSync,
  } = {},
) {
  let descriptor;
  try {
    descriptor = open(
      input,
      fsConstants.O_RDONLY |
        fsConstants.O_NOFOLLOW |
        fsConstants.O_NONBLOCK,
    );
  } catch (error) {
    throw new Error(
      `Form Package batch input cannot be opened without following symbolic links: ${error.message}`,
    );
  }
  let raw;
  let stat;
  try {
    stat = fstat(descriptor);
    if (!stat.isFile()) {
      throw new Error("Form Package batch input must be a regular file");
    }
    if (stat.size > FORM_BATCH_MAX_BYTES) {
      throw new Error("Form Package batch input exceeds 1 MiB");
    }
    const buffer = Buffer.alloc(FORM_BATCH_MAX_BYTES + 1);
    let offset = 0;
    while (offset < buffer.length) {
      const bytesRead = read(
        descriptor,
        buffer,
        offset,
        buffer.length - offset,
        null,
      );
      if (
        !Number.isSafeInteger(bytesRead) ||
        bytesRead < 0 ||
        bytesRead > buffer.length - offset
      ) {
        throw new Error(
          "Form Package batch input read returned an invalid byte count",
        );
      }
      if (bytesRead === 0) break;
      offset += bytesRead;
    }
    if (offset > FORM_BATCH_MAX_BYTES) {
      throw new Error("Form Package batch input exceeds 1 MiB");
    }
    raw = buffer.subarray(0, offset);
    if (raw.length !== stat.size) {
      throw new Error("Form Package batch input changed while it was read");
    }
  } finally {
    close(descriptor);
  }
  const value = parseCanonicalCandidateMetadata(
    raw,
    "Form Package batch input",
  );
  if (
    !Array.isArray(value) ||
    value.length === 0 ||
    value.length > plan.releases.length
  ) {
    throw new Error(
      `Form Package batch input must contain 1-${plan.releases.length} entries`,
    );
  }
  const tags = new Set();
  const runs = new Set();
  return value.map((entry, index) => {
    const label = `Form Package batch input entry ${index + 1}`;
    if (
      !entry ||
      typeof entry !== "object" ||
      Array.isArray(entry) ||
      JSON.stringify(Object.keys(entry).sort()) !==
        JSON.stringify(
          ["expectedCommit", "runAttempt", "runId", "tag"].sort(),
        )
    ) {
      throw new Error(
        `${label} must contain exactly tag, expectedCommit, runId, and runAttempt`,
      );
    }
    for (const [name, field] of [
      ["tag", entry.tag],
      ["expectedCommit", entry.expectedCommit],
      ["runId", entry.runId],
      ["runAttempt", entry.runAttempt],
    ]) {
      if (typeof field !== "string" || field === "") {
        throw new Error(`${label} ${name} must be a non-empty string`);
      }
    }
    const options = parseReleaseSurfaceArgs(FORM_SURFACE, [
      "publish",
      "--tag",
      entry.tag,
      "--expected-commit",
      entry.expectedCommit,
      "--run-id",
      entry.runId,
      "--run-attempt",
      entry.runAttempt,
    ]);
    plannedRelease(plan, options.tag);
    const run = `${options["run-id"]}/${options["run-attempt"]}`;
    if (tags.has(options.tag)) {
      throw new Error(`${label} duplicates tag ${options.tag}`);
    }
    if (runs.has(run)) {
      throw new Error(`${label} duplicates workflow run/attempt ${run}`);
    }
    tags.add(options.tag);
    runs.add(run);
    return options;
  });
}

function formPublishBatch(
  context,
  options,
  plan,
  publishOne = formPublish,
) {
  const entries = readFormPublishBatch(options.input, plan);
  const proof = establishFormPublishBatchOwnerGateProof(context);
  const results = [];
  let active = null;
  try {
    for (const entry of entries) {
      active = entry;
      results.push(publishOne(context, entry, plan));
    }
  } catch (error) {
    context.stderr.write(
      `${JSON.stringify({
        kind: "takos.deploy-failure@v1",
        surface: FORM_SURFACE,
        phase: "publish-batch",
        ownerGateCommit: proof.commit,
        completedTags: results.map((result) => result.tag),
        failedTag: active?.tag ?? null,
        instruction:
          "stop; inspect the exact failed tag and Release state, then create a new input containing only identities authoritatively proven absent",
      })}\n`,
    );
    throw error;
  } finally {
    delete context.formPublishBatchOwnerGateProof;
  }
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: FORM_SURFACE,
    phase: "publish-batch",
    commit: proof.commit,
    ownerGateTree: proof.tree,
    releases: results.map((result) => ({
      tag: result.tag,
      commit: result.commit,
      tagObject: result.tagObject,
      candidateRun: result.candidateRun,
      releaseId: result.releaseId,
      releaseUrl: result.releaseUrl,
      assetDigests: result.assetDigests,
      productionReadback: result.productionReadback,
    })),
    status: "VERIFIED",
  });
}

function formPublicationMutationFence(
  context,
  { expectedCommit, toolingCommit, entry, label },
) {
  const currentMain = ownerGateAndFence(context);
  assertFormReleaseAuthorityFence(context, {
    sourceCommit: expectedCommit,
    toolingCommit,
    currentMain,
    sourcePath: entry.sourcePath,
    label,
  });
  return currentMain;
}

function formPublish(context, options, plan) {
  const entry = plannedRelease(plan, options.tag);
  const expectedCommit = options["expected-commit"];
  const initialMain = assertCurrentProtectedMain(context, expectedCommit, {
    exact: false,
  });
  assertTagAbsent(context, entry.tag);
  assertReleaseAbsent(context, entry.tag);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare signed Form Package release candidate",
    },
  );
  const toolingCommit = run.headSha;
  assertFormReleaseAuthorityFence(context, {
    sourceCommit: expectedCommit,
    toolingCommit,
    currentMain: initialMain,
    sourcePath: entry.sourcePath,
    label: "Form Package reviewed candidate fence",
  });
  return withTemporaryDirectory("takoform-form-publish", (temporaryRoot) => {
    const candidate = downloadArtifact(
      context,
      options["run-id"],
      `form-package-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
      join(temporaryRoot, "candidate"),
    );
    const verified = verifyFormCandidate(context, candidate, {
      entry,
      runId: options["run-id"],
      runAttempt: options["run-attempt"],
      requestId: run.displayTitle,
      expectedCommit,
      toolingCommit,
    });
    let tagObject = "";
    try {
      formPublicationMutationFence(context, {
        expectedCommit,
        toolingCommit,
        entry,
        label: "Form Package tag mutation fence",
      });
      tagObject = ensureCandidateTagPublished(
        context,
        entry.tag,
        expectedCommit,
        candidate,
        verified.metadata,
      );
      formPublicationMutationFence(context, {
        expectedCommit,
        toolingCommit,
        entry,
        label: "Form Package release mutation fence",
      });
      const release = publishReleaseLocally(context, {
        tag: entry.tag,
        assets: verified.assets,
        body:
          "Data-only Takoform Form Package release. Verify the canonical package index, Sigstore bundle, and publisher identity before installation.",
        temporaryRoot,
        prePublishFence: () => {
          formPublicationMutationFence(context, {
            expectedCommit,
            toolingCommit,
            entry,
            label: "Form Package pre-publish fence",
          });
        },
      });
      return emit(context, {
        kind: "takos.deploy-result@v1",
        surface: FORM_SURFACE,
        phase: "publish",
        commit: expectedCommit,
        tag: entry.tag,
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
      reportTagFailure(context, entry.tag, tagObject);
      throw error;
    }
  });
}

function formRecoverTagOnly(context, options, plan) {
  const entry = plannedRelease(plan, options.tag);
  const expectedCommit = options["expected-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const initialMain = assertCurrentProtectedMain(context, recoveryCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare signed Form Package release candidate",
    },
  );
  const toolingCommit = run.headSha;
  assertFormTagOnlyRecoveryFence(context, {
    sourceCommit: expectedCommit,
    toolingCommit,
    recoveryCommit: initialMain,
    sourcePath: entry.sourcePath,
    label: "Form Package tag-only reviewed candidate fence",
  });
  return withTemporaryDirectory(
    "takoform-form-tag-only-recovery",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `form-package-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const verified = verifyFormCandidate(context, candidate, {
        entry,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit,
        toolingCommit,
      });
      try {
        const candidateObject = reconstructCandidateTagObject(
          context,
          entry.tag,
          expectedCommit,
          candidate,
          verified.metadata,
        );
        if (
          candidateObject !== expectedObject ||
          verified.metadata.tagObjectOid !== expectedObject
        ) {
          throw new Error(
            `tag-only recovery candidate object ${candidateObject} does not equal explicitly expected ${expectedObject}`,
          );
        }
        assertTagOnlyRecoveryState(context, {
          tag: entry.tag,
          expectedCommit,
          expectedObject,
        });
        const mutationMain = ownerGateAndFence(context, recoveryCommit);
        assertFormTagOnlyRecoveryFence(context, {
          sourceCommit: expectedCommit,
          toolingCommit,
          recoveryCommit: mutationMain,
          sourcePath: entry.sourcePath,
          label: "Form Package tag-only release mutation fence",
        });
        assertTagOnlyRecoveryState(context, {
          tag: entry.tag,
          expectedCommit,
          expectedObject,
        });
        const release = publishReleaseLocally(context, {
          tag: entry.tag,
          assets: verified.assets,
          body: FORM_TAG_ONLY_RELEASE_BODY,
          temporaryRoot,
          prePublishFence: () => {
            const publishMain = ownerGateAndFence(context, recoveryCommit);
            assertFormTagOnlyRecoveryFence(context, {
              sourceCommit: expectedCommit,
              toolingCommit,
              recoveryCommit: publishMain,
              sourcePath: entry.sourcePath,
              label: "Form Package tag-only pre-publish fence",
            });
            const state = assertExactRemoteTag(
              context,
              entry.tag,
              expectedCommit,
              expectedObject,
            );
            const local = localTagOID(context, entry.tag);
            if (local !== expectedObject) {
              throw new Error(
                `tag-only recovery local tag changed before publication: ${local || "absent"}`,
              );
            }
            return state;
          },
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: FORM_SURFACE,
          phase: "recover-tag-only",
          commit: expectedCommit,
          toolingCommit,
          recoveryCommit,
          tag: entry.tag,
          tagObject: expectedObject,
          recoveredFrom: "EXACT_TAG_PRESENT_RELEASE_ABSENT",
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...verified.assets].map(([name, asset]) => [name, asset.sha256]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, entry.tag, "", {
          phase: "recover-tag-only",
        });
        throw error;
      }
    },
  );
}

function formRecoverDraft(context, options, plan) {
  const entry = plannedRelease(plan, options.tag);
  const expectedCommit = options["expected-commit"];
  const expectedObject = options["expected-tag-object"];
  const recoveryCommit = options["expected-recovery-commit"];
  const releaseId = Number(options["release-id"]);
  const initialMain = assertCurrentProtectedMain(context, recoveryCommit);
  const run = requireSuccessfulRun(
    context,
    options["run-id"],
    options["run-attempt"],
    {
      workflowName: "Prepare signed Form Package release candidate",
    },
  );
  const toolingCommit = run.headSha;
  assertFormTagOnlyRecoveryFence(context, {
    sourceCommit: expectedCommit,
    toolingCommit,
    recoveryCommit: initialMain,
    sourcePath: entry.sourcePath,
    label: "Form Package retained-draft reviewed candidate fence",
  });
  return withTemporaryDirectory(
    "takoform-form-draft-recovery",
    (temporaryRoot) => {
      const candidate = downloadArtifact(
        context,
        options["run-id"],
        `form-package-release-candidate-${options["run-id"]}-${options["run-attempt"]}`,
        join(temporaryRoot, "candidate"),
      );
      const verified = verifyFormCandidate(context, candidate, {
        entry,
        runId: options["run-id"],
        runAttempt: options["run-attempt"],
        requestId: run.displayTitle,
        expectedCommit,
        toolingCommit,
      });
      try {
        const candidateObject = reconstructCandidateTagObject(
          context,
          entry.tag,
          expectedCommit,
          candidate,
          verified.metadata,
        );
        if (
          candidateObject !== expectedObject ||
          verified.metadata.tagObjectOid !== expectedObject
        ) {
          throw new Error(
            `draft recovery candidate object ${candidateObject} does not equal explicitly expected ${expectedObject}`,
          );
        }
        assertExactRemoteTag(
          context,
          entry.tag,
          expectedCommit,
          expectedObject,
        );
        const local = localTagOID(context, entry.tag);
        if (local !== expectedObject) {
          throw new Error(
            `draft recovery local tag differs: ${local || "absent"}`,
          );
        }
        const mutationMain = ownerGateAndFence(context, recoveryCommit);
        assertFormTagOnlyRecoveryFence(context, {
          sourceCommit: expectedCommit,
          toolingCommit,
          recoveryCommit: mutationMain,
          sourcePath: entry.sourcePath,
          label: "Form Package retained-draft mutation fence",
        });
        assertExactRemoteTag(
          context,
          entry.tag,
          expectedCommit,
          expectedObject,
        );
        const release = resumeDraftReleaseLocally(context, {
          releaseId,
          tag: entry.tag,
          assets: verified.assets,
          body: FORM_TAG_ONLY_RELEASE_BODY,
          temporaryRoot,
          prePublishFence: () => {
            const publishMain = ownerGateAndFence(context, recoveryCommit);
            assertFormTagOnlyRecoveryFence(context, {
              sourceCommit: expectedCommit,
              toolingCommit,
              recoveryCommit: publishMain,
              sourcePath: entry.sourcePath,
              label: "Form Package retained-draft pre-publish fence",
            });
            const publishState = assertExactRemoteTag(
              context,
              entry.tag,
              expectedCommit,
              expectedObject,
            );
            const publishLocal = localTagOID(context, entry.tag);
            if (publishLocal !== expectedObject) {
              throw new Error(
                `draft recovery local tag changed before publication: ${publishLocal || "absent"}`,
              );
            }
            return publishState;
          },
        });
        return emit(context, {
          kind: "takos.deploy-result@v1",
          surface: FORM_SURFACE,
          phase: "recover-draft",
          commit: expectedCommit,
          toolingCommit,
          recoveryCommit,
          tag: entry.tag,
          tagObject: expectedObject,
          recoveredFrom: "EXACT_RETAINED_DRAFT",
          candidateRun: {
            id: options["run-id"],
            attempt: options["run-attempt"],
            url: run.url,
          },
          releaseId: release.id,
          releaseUrl: release.html_url,
          assetDigests: Object.fromEntries(
            [...verified.assets].map(([name, asset]) => [
              name,
              asset.sha256,
            ]),
          ),
          productionReadback: "EXACT_IMMUTABLE_RELEASE",
          status: "VERIFIED",
        });
      } catch (error) {
        reportTagFailure(context, entry.tag, "", {
          phase: "recover-draft",
        });
        throw error;
      }
    },
  );
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

function formVerify(context, options) {
  const identity = formReleaseIdentity(options.tag);
  const currentMain = assertCurrentProtectedMain(
    context,
    options["expected-commit"],
    {
    exact: false,
    },
  );
  assertExactRemoteTag(context, identity.tag, options["expected-commit"]);
  return withTemporaryDirectory("takoform-form-verify", (temporaryRoot) => {
    const names = formAssetNames(identity);
    const live = readExactPublicRelease(
      context,
      identity.tag,
      names.all,
      temporaryRoot,
    );
    const releaseManifest = readJSON(
      join(live.output, "release-manifest.json"),
      "Form Package release manifest",
    );
    if (!COMMIT.test(releaseManifest.toolingCommit ?? "")) {
      throw new Error("public Form Package tooling commit is invalid");
    }
    const entry = plannedRelease(
      readReleasePlanAtCommit(context, releaseManifest.toolingCommit),
      identity.tag,
    );
    const manifest = verifyFormPublicAssets(context, live.output, entry);
    if (manifest.sourceCommit !== options["expected-commit"]) {
      throw new Error("public Form Package source commit differs");
    }
    assertCommitAncestor(
      context,
      manifest.sourceCommit,
      manifest.toolingCommit,
      "public Form Package source/tooling binding",
    );
    assertCommitAncestor(
      context,
      manifest.toolingCommit,
      currentMain,
      "public Form Package tooling/current-main binding",
    );
    return emit(context, {
      kind: "takos.deploy-result@v1",
      surface: FORM_SURFACE,
      phase: "verify",
      tag: entry.tag,
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

function requireAtomicOutputCommitResult(raw, phase) {
  let result;
  try {
    result = JSON.parse(raw);
  } catch (error) {
    throw new Error(`atomic output ${phase} returned invalid JSON`, {
      cause: error,
    });
  }
  requireExactKeys(result, ["format", "status"], `atomic output ${phase}`);
  if (
    result.format !== "takoform.release-output-commit@v1" ||
    result.status !== "verified"
  ) {
    throw new Error(`atomic output ${phase} protocol drifted`);
  }
}

function commitVerifiedOutput(context, stagingParent, staging, output) {
  requireAtomicOutputCommitResult(
    command(context, "go", [
      "run",
      "./cmd/release-output-commit",
      "probe",
      "--parent",
      stagingParent,
    ]),
    "capability probe",
  );
  if (existsSync(output)) {
    throw new Error("--output-root appeared after the atomic capability probe");
  }
  requireAtomicOutputCommitResult(
    command(context, "go", [
      "run",
      "./cmd/release-output-commit",
      "commit",
      "--source",
      staging,
      "--target",
      output,
    ]),
    "commit",
  );
}

function formVerifyAll(context, options, plan) {
  verifyLocalReleaseToolchain(context);
  const commit = assertCurrentProtectedMain(context);
  const output = resolve(options["output-root"]);
  const inside =
    output === context.repo ||
    (!relative(context.repo, output).startsWith(`..${sep}`) &&
      relative(context.repo, output) !== "..");
  if (inside || existsSync(output)) {
    throw new Error(
      "--output-root must be a new absent directory outside the repository",
    );
  }
  const stagingParent = mkdtempSync(
    join(dirname(output), `.${basename(output)}.takoform-staging-`),
  );
  const staging = join(stagingParent, "verified-output");
  let publicationSet;
  try {
    progress(context, "deep-read all 34 immutable Form Package releases");
    command(
      context,
      "go",
      [
        "run",
        "./cmd/published-package-set",
        "download-plan",
        "--output-root",
        staging,
      ],
      {
        echo: true,
        env: {
          ...environmentWithoutGitHubAuthority(),
          GITHUB_TOKEN: process.env.GH_TOKEN,
        },
      },
    );
    publicationSet = readJSON(
      join(staging, "form-package-publication-set.json"),
      "Form Package publication set",
    );
    if (
      publicationSet.format !== "takoform.form-package-publication-set@v1" ||
      publicationSet.generation !== plan.generation ||
      publicationSet.repository !== GITHUB_REPOSITORY ||
      publicationSet.protectedMainCommit !== commit ||
      !Array.isArray(publicationSet.entries) ||
      publicationSet.entries.length !== 34
    ) {
      throw new Error("all-Form publication set identity drifted");
    }
    for (const entry of publicationSet.entries) {
      const identity = formReleaseIdentity(entry.tag);
      if (
        entry.releaseId !== identity.releaseId ||
        entry.version !== identity.version ||
        !COMMIT.test(entry.toolingCommit ?? "")
      ) {
        throw new Error(
          `publication set entry has an invalid historical tooling identity: ${entry.tag}`,
        );
      }
      const authorityPrefix = `authority/${entry.toolingCommit}`;
      for (const [label, authority, expectedPath, expectedSourcePath] of [
        [
          "release plan",
          entry.releasePlan,
          `${authorityPrefix}/release-plan.json`,
          "forms/release-plan.json",
        ],
        [
          "trusted root",
          entry.trustedRoot,
          `${authorityPrefix}/trusted-root.json`,
          TRUSTED_ROOT,
        ],
      ]) {
        requireExactKeys(
          authority,
          ["path", "sourcePath", "sha256"],
          `publication entry ${label}`,
        );
        const retained = join(staging, ...expectedPath.split("/"));
        if (
          authority.path !== expectedPath ||
          authority.sourcePath !== expectedSourcePath ||
          !SHA256.test(authority.sha256 ?? "") ||
          !existsSync(retained) ||
          fileDigest(retained) !== authority.sha256
        ) {
          throw new Error(
            `publication entry ${label} does not bind retained tooling authority: ${entry.tag}`,
          );
        }
      }
      const names = formAssetNames(identity);
      verifySigstoreSubject(
        context,
        join(staging, "releases", identity.releaseId, identity.version),
        names.index,
        names.bundle,
        ".github/workflows/form-package-release.yml",
        join(staging, ...entry.trustedRoot.path.split("/")),
      );
    }
    if (existsSync(output)) {
      throw new Error("--output-root appeared during verification; refusing overwrite");
    }
    commitVerifiedOutput(context, stagingParent, staging, output);
    if (existsSync(staging) || !existsSync(output)) {
      throw new Error(
        "--output-root appeared during create-only publication; verified staging was not published",
      );
    }
  } finally {
    rmSync(stagingParent, { recursive: true, force: true });
  }
  return emit(context, {
    kind: "takos.deploy-result@v1",
    surface: FORM_SURFACE,
    phase: "verify-all",
    commit,
    generation: plan.generation,
    releases: 34,
    outputRoot: output,
    publicationSetDigest: fileDigest(
      join(output, "form-package-publication-set.json"),
    ),
    status: "VERIFIED",
  });
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
    revocation: true,
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
        revocation: true,
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
        revocation: true,
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
            revocation: true,
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
  assertFormTagOnlyRecoveryFence,
  assertProviderRecoveryFence,
  assertProviderReadbackCommitBinding,
  assertReusableOwnerGateProof,
  assertReleaseAbsent,
  assertRegistryVersionAbsent,
  assertTagOnlyRecoveryState,
  command,
  createProviderCandidateRequestRecord,
  dispatchWorkflow,
  establishFormPublishBatchOwnerGateProof,
  expectedFormTagObject,
  formPublishBatch,
  formPublicationMutationFence,
  formVerifyAll,
  githubUploadEnvironment,
  ownerGateAndFence,
  providerOwnerGateAndFence,
  observeTagFailureState,
  parseCandidateMetadata,
  parseCanonicalCandidateMetadata,
  parsePrettyCandidateMetadata,
  publishReleaseLocally,
  providerAssetNames,
  providerCandidateRequestRecord,
  pushExactTag,
  reportTagFailure,
  resumeDraftReleaseLocally,
  reconstructCandidateTagObject,
  readFormPublishBatch,
  readProviderCandidateRequestRecord,
  requireSuccessfulRun,
  validateReleaseReadback,
  validateDraftBeforePublication,
  verifyChecksumClosure,
  verifyFormSemanticClosure,
  verifyProviderCandidate,
  verifyRegistryCandidate,
  verifyProviderReleaseProvenance,
  verifyProviderSignature,
  verifyRevocationSemanticClosure,
  verifyTagObjectWorkflowBinding,
});

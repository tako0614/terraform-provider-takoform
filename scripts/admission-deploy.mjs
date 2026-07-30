import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import {
  lstatSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, isAbsolute, join, resolve } from "node:path";
import process from "node:process";

const SURFACE = "takoform-admission-release";
const CANONICAL_ORIGINS = new Set([
  "https://github.com/tako0614/terraform-provider-takoform.git",
]);
const DESCRIPTOR_PATH = "admission/v4/version.json";
const IDENTITY_LEDGER_PATH = "admission/admission-identities.json";
const SET_PATH = "admission/v4/standard-admission-set.json";
const ADMISSION_TAG_RULESET_PATTERN = "refs/tags/forms/admissions/v*";
const GITHUB_REPOSITORY = "tako0614/terraform-provider-takoform";
const GITHUB_API_VERSION = "2022-11-28";
const ADMISSION_TAGGER_NAME = "Takoform Standard Admission";
const ADMISSION_TAGGER_EMAIL = "admission@takoform.invalid";
const MAX_RULESET_SUMMARIES = 100;
const COMMIT = /^[0-9a-f]{40}$/u;
const DIGEST = /^sha256:[0-9a-f]{64}$/u;
const STABLE_VERSION =
  /^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$/u;

const PINNED_IDENTITIES = Object.freeze([
  {
    version: "1.0.1",
    tag: "forms/admissions/v1.0.1",
    status: "assigned-historical",
    tagObject: "2b1ca9f68688392869a79de122fbce2a54842301",
    commit: "57aba7f374bb0d45274044e1dacbea52d16f3f6b",
  },
  {
    version: "1.0.2",
    tag: "forms/admissions/v1.0.2",
    status: "assigned-historical",
    tagObject: "98af8dd461f24e6dc902f5c56dc6740f74ceb5af",
    commit: "ff65142ecfab206f58239f095b5e170854ef9dde",
  },
  {
    version: "1.0.3",
    tag: "forms/admissions/v1.0.3",
    status: "assigned-historical",
    tagObject: "82af8a61666e0194506d0d23d04422ccda4b3d86",
    commit: "4a40826c7ed467af84e856487998ce365ffe00dd",
  },
  {
    version: "1.0.4",
    tag: "forms/admissions/v1.0.4",
    status: "assigned-historical",
    tagObject: "b49a55016362d8787966f41b14570e3b67b8ddba",
    commit: "a426a379e2743b4345e868becf3618357c015447",
  },
  {
    version: "1.0.5",
    tag: "forms/admissions/v1.0.5",
    status: "reserved-abandoned",
    retainedPaths: [
      "admission/v3/candidates/host-report-1.0.5-63dabf0c64be-bd0b3184aaad",
      "admission/v3/candidates/provider-report-1.0.5-bd0b3184aaad",
      "admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad",
    ],
  },
  {
    version: "1.0.6",
    tag: "forms/admissions/v1.0.6",
    status: "assigned-current",
    descriptorPath: DESCRIPTOR_PATH,
  },
]);

export const ADMISSION_SURFACE = Object.freeze({
  surface: SURFACE,
  target:
    "annotated-git-tag:tako0614/terraform-provider-takoform/forms/admissions/v*",
  covers: [
    DESCRIPTOR_PATH,
    IDENTITY_LEDGER_PATH,
    "admission/v4",
    "cmd/standard-form-conformance",
    "internal/admissioncheckpoint",
    "internal/admissionrelease",
  ],
  requiresScripts: ["check"],
  requiresTools: ["git", "bun", "go", "gh"],
  requiresEnv: ["GH_TOKEN"],
  triggers: ["authority", "published-identity"],
  obligations: {
    provenance:
      "requires a clean non-shallow protected main checkout equal to a fresh canonical origin/main read, runs the complete owner check plus the current source-retained admission-material check, uses GH_TOKEN only for read-only gh API calls that prove exact active admission-tag rulesets, derives v1.0.6 and its tag only from the reviewed descriptor, and binds the exact commit, tree, descriptor digest, assignment-ledger digest, and retained-set digest into the annotated tag message",
    "post-conditions":
      "immediately re-reads the same exact active admission-tag ruleset protection, reads the authoritative remote annotated tag object and peeled commit, requires its exact retained-source message, and runs the exact current-admission-closure-check over the Sigstore-authenticated source-retained evidence and immutable provider/package refs",
    reversal:
      "admission checkpoint tags and assigned versions are append-only; an incorrect checkpoint is never deleted, moved, or reused and is repaired forward under the next identity",
    "failure-handling":
      "prints raw diagnostics, reports whether failure occurred before local tag creation, with an exact local-only tag, or after the remote ref became visible, never retries an ambiguous push, and never deletes or force-updates an admission tag",
    "independent-review":
      "the final retained closure commit, exact descriptor and identity ledger, complete owner gate, offline evidence authentication, and protected tag ruleset are reviewed independently before the operator invokes publish",
    "no-overwrite":
      "requires all historical assigned tags to retain their exact object and peeled commit, requires reserved-abandoned v1.0.5 to remain absent remotely, refuses any existing local or remote current tag, requires exact active rulesets that restrict creation to bypass actors and give update/deletion/non-fast-forward protection no bypass, creates only the descriptor tag through a server-enforced remote-absent lease that cannot overwrite a ref, and accepts only the exact annotated remote object at the exact retained commit",
  },
});

export function isAdmissionSurface(name) {
  return name === SURFACE;
}

export function parseAdmissionArguments(args) {
  if (!Array.isArray(args) || args.length === 0) throw usageError();
  const [phase, ...rest] = args;
  if (!["prepare", "publish", "verify"].includes(phase)) throw usageError();
  if (
    rest.length !== 2 ||
    rest[0] !== "--expected-commit" ||
    !COMMIT.test(rest[1] ?? "")
  ) {
    throw usageError();
  }
  return { phase, expectedCommit: rest[1] };
}

function usageError() {
  return new Error(
    `usage: bun run deploy -- ${SURFACE} <prepare|publish|verify> --expected-commit <lowercase-40-hex>`,
  );
}

export function parseAdmissionDescriptor(raw) {
  const value = parseObject(raw, DESCRIPTOR_PATH);
  requireExactKeys(value, [
    "format",
    "version",
    "tag",
    "generation",
    "retainedRoot",
  ]);
  if (
    value.format !== "takoform.standard-admission-checkpoint@v1" ||
    !STABLE_VERSION.test(value.version ?? "") ||
    value.tag !== `forms/admissions/v${value.version}` ||
    value.generation !== "ga-core-v2" ||
    value.retainedRoot !== "admission/v4"
  ) {
    throw new Error(`${DESCRIPTOR_PATH} does not equal the pinned v4 identity`);
  }
  return value;
}

export function parseAdmissionIdentityLedger(raw, descriptor) {
  const value = parseObject(raw, IDENTITY_LEDGER_PATH);
  requireExactKeys(value, ["format", "entries"]);
  if (
    value.format !== "takoform.standard-admission-identities@v1" ||
    !Array.isArray(value.entries) ||
    value.entries.length !== PINNED_IDENTITIES.length
  ) {
    throw new Error(
      `${IDENTITY_LEDGER_PATH} is not the exact identity closure`,
    );
  }
  for (let index = 0; index < PINNED_IDENTITIES.length; index += 1) {
    const actual = requireObject(value.entries[index], `entries[${index}]`);
    const expected = PINNED_IDENTITIES[index];
    requireExactKeys(actual, Object.keys(expected));
    if (JSON.stringify(actual) !== JSON.stringify(expected)) {
      throw new Error(
        `${IDENTITY_LEDGER_PATH} entries[${index}] changes a pinned identity`,
      );
    }
  }
  const current = value.entries.at(-1);
  if (
    current.version !== descriptor.version ||
    current.tag !== descriptor.tag ||
    current.descriptorPath !== DESCRIPTOR_PATH
  ) {
    throw new Error("current identity ledger entry does not match descriptor");
  }
  return value;
}

export function parseRemoteAdmissionTag(raw, tag, expectedCommit) {
  const lines = raw
    .trim()
    .split("\n")
    .filter((line) => line !== "");
  const direct = `refs/tags/${tag}`;
  const peeled = `${direct}^{}`;
  const refs = new Map();
  for (const line of lines) {
    const fields = line.split(/\s+/u);
    if (
      fields.length !== 2 ||
      !COMMIT.test(fields[0]) ||
      ![direct, peeled].includes(fields[1]) ||
      refs.has(fields[1])
    ) {
      throw new Error(`remote tag ${tag} returned an invalid ref inventory`);
    }
    refs.set(fields[1], fields[0]);
  }
  if (!refs.has(direct) || !refs.has(peeled)) {
    throw new Error(`remote tag ${tag} must be one annotated tag`);
  }
  if (refs.get(peeled) !== expectedCommit) {
    throw new Error(
      `remote tag ${tag} peels to ${refs.get(peeled)}, want ${expectedCommit}`,
    );
  }
  if (refs.get(direct) === expectedCommit) {
    throw new Error(`remote tag ${tag} is lightweight, want annotated`);
  }
  return { tagObject: refs.get(direct), commit: refs.get(peeled) };
}

export function parseAdmissionTagRulesetProtection(raw) {
  let rulesets;
  try {
    rulesets = typeof raw === "string" ? JSON.parse(raw) : raw;
  } catch (error) {
    throw new Error(
      `GitHub admission-tag rulesets are not JSON: ${error.message}`,
    );
  }
  if (!Array.isArray(rulesets) || rulesets.length === 0) {
    throw new Error("GitHub returned no detailed admission-tag rulesets");
  }
  const seenIDs = new Set();
  const exact = [];
  for (const [index, candidate] of rulesets.entries()) {
    const ruleset = requireObject(candidate, `rulesets[${index}]`);
    if (
      !Number.isSafeInteger(ruleset.id) ||
      ruleset.id <= 0 ||
      seenIDs.has(ruleset.id)
    ) {
      throw new Error(
        "GitHub admission-tag rulesets contain an invalid or duplicate id",
      );
    }
    seenIDs.add(ruleset.id);
    if (
      typeof ruleset.name !== "string" ||
      ruleset.name.trim() === "" ||
      ruleset.target !== "tag" ||
      typeof ruleset.source_type !== "string" ||
      typeof ruleset.source !== "string" ||
      !["active", "disabled", "evaluate"].includes(ruleset.enforcement)
    ) {
      throw new Error(
        `GitHub ruleset ${ruleset.id} has an insufficient identity`,
      );
    }
    const conditions = requireObject(
      ruleset.conditions,
      `rulesets[${index}].conditions`,
    );
    const refName = requireObject(
      conditions.ref_name,
      `rulesets[${index}].conditions.ref_name`,
    );
    if (!Array.isArray(refName.include) || !Array.isArray(refName.exclude)) {
      throw new Error(
        `GitHub ruleset ${ruleset.id} has incomplete ref conditions`,
      );
    }
    if (
      ruleset.source_type !== "Repository" ||
      ruleset.source !== GITHUB_REPOSITORY ||
      ruleset.enforcement !== "active" ||
      refName.include.length !== 1 ||
      refName.include[0] !== ADMISSION_TAG_RULESET_PATTERN ||
      refName.exclude.length !== 0
    ) {
      continue;
    }
    requireExactKeys(conditions, ["ref_name"]);
    requireExactKeys(refName, ["include", "exclude"]);
    if (
      !Array.isArray(ruleset.rules) ||
      !Array.isArray(ruleset.bypass_actors)
    ) {
      throw new Error(
        `GitHub ruleset ${ruleset.id} has incomplete rules or bypass data`,
      );
    }
    const ruleTypes = [];
    for (const rule of ruleset.rules) {
      const value = requireObject(rule, `ruleset ${ruleset.id} rule`);
      if (typeof value.type !== "string" || ruleTypes.includes(value.type)) {
        throw new Error(
          `GitHub ruleset ${ruleset.id} has an invalid or duplicate rule`,
        );
      }
      if (value.type === "update" && value.parameters !== undefined) {
        const parameters = requireObject(
          value.parameters,
          `ruleset ${ruleset.id} update parameters`,
        );
        requireExactKeys(parameters, ["update_allows_fetch_and_merge"]);
        if (parameters.update_allows_fetch_and_merge !== false) {
          throw new Error(
            `GitHub ruleset ${ruleset.id} permits fetch-and-merge updates`,
          );
        }
        requireExactKeys(value, ["type", "parameters"]);
      } else {
        requireExactKeys(value, ["type"]);
      }
      ruleTypes.push(value.type);
    }
    ruleTypes.sort();
    const bypassActors = ruleset.bypass_actors.map((actor, actorIndex) => {
      const value = requireObject(
        actor,
        `ruleset ${ruleset.id} bypass actor ${actorIndex}`,
      );
      if (
        !["Integration", "RepositoryRole", "Team", "User"].includes(
          value.actor_type,
        ) ||
        !Number.isSafeInteger(value.actor_id) ||
        value.actor_id <= 0 ||
        value.bypass_mode !== "always"
      ) {
        throw new Error(
          `GitHub ruleset ${ruleset.id} has an invalid tag bypass actor`,
        );
      }
      return {
        actorId: value.actor_id,
        actorType: value.actor_type,
        bypassMode: value.bypass_mode,
      };
    });
    bypassActors.sort((left, right) =>
      JSON.stringify(left).localeCompare(JSON.stringify(right)),
    );
    if (
      new Set(bypassActors.map((actor) => JSON.stringify(actor))).size !==
      bypassActors.length
    ) {
      throw new Error(`GitHub ruleset ${ruleset.id} repeats a bypass actor`);
    }
    exact.push({
      id: ruleset.id,
      name: ruleset.name,
      ruleTypes,
      bypassActors,
      currentUserCanBypass: ruleset.current_user_can_bypass,
    });
  }

  const creation = exact.filter(
    (ruleset) =>
      JSON.stringify(ruleset.ruleTypes) === JSON.stringify(["creation"]),
  );
  const immutable = exact.filter(
    (ruleset) =>
      JSON.stringify(ruleset.ruleTypes) ===
      JSON.stringify(["deletion", "non_fast_forward", "update"]),
  );
  if (creation.length !== 1 || immutable.length !== 1 || exact.length !== 2) {
    throw new Error(
      `GitHub admission-tag rulesets are ambiguous or incomplete: exact=${exact.length} creation=${creation.length} immutable=${immutable.length}`,
    );
  }
  if (
    creation[0].currentUserCanBypass !== "always" ||
    creation[0].bypassActors.length === 0
  ) {
    throw new Error(
      "GitHub admission-tag creation is not restricted to explicit bypass actors available to this operator",
    );
  }
  if (
    immutable[0].currentUserCanBypass !== "never" ||
    immutable[0].bypassActors.length !== 0
  ) {
    throw new Error(
      "GitHub admission-tag update/deletion/non-fast-forward protection has a bypass",
    );
  }

  const projection = {
    pattern: ADMISSION_TAG_RULESET_PATTERN,
    creation: creation[0],
    immutable: immutable[0],
  };
  return {
    creationRulesetID: creation[0].id,
    immutableRulesetID: immutable[0].id,
    fingerprint: digest(Buffer.from(JSON.stringify(projection))),
  };
}

export function buildAdmissionTagMessage({
  descriptor,
  commit,
  tree,
  descriptorDigest,
  identityLedgerDigest,
  setDigest,
}) {
  for (const [label, value] of Object.entries({ commit, tree })) {
    if (!COMMIT.test(value ?? "")) throw new Error(`${label} is not 40-hex`);
  }
  for (const [label, value] of Object.entries({
    descriptorDigest,
    identityLedgerDigest,
    setDigest,
  })) {
    if (!DIGEST.test(value ?? "")) throw new Error(`${label} is not sha256`);
  }
  const lines = [
    `Activate Standard Form admission v${descriptor.version}`,
    "",
    `generation ${descriptor.generation}`,
    `commit ${commit}`,
    `tree ${tree}`,
    `version-descriptor ${descriptorDigest}`,
    `identity-ledger ${identityLedgerDigest}`,
  ];
  lines.push(`standard-admission-set ${setDigest}`, "");
  return lines.join("\n");
}

export async function runAdmissionSurface({
  surface,
  args,
  repo,
  stdout = process.stdout,
  stderr = process.stderr,
  commandRunner,
  githubToken = process.env.GH_TOKEN,
}) {
  if (!isAdmissionSurface(surface)) {
    throw new Error(`unknown admission surface ${JSON.stringify(surface)}`);
  }
  if (typeof repo !== "string" || repo === "") {
    throw new Error("repo is required");
  }
  const options = parseAdmissionArguments(args);
  if (
    typeof githubToken !== "string" ||
    githubToken === "" ||
    /\s/u.test(githubToken)
  ) {
    throw new Error(
      "admission publication requires one non-empty GH_TOKEN without whitespace",
    );
  }
  const context = {
    repo: resolve(repo),
    stdout,
    stderr,
    commandRunner,
    githubToken,
  };
  const source = requireSourceCheckpoint(
    context,
    options.expectedCommit,
    options.phase,
  );

  if (options.phase === "verify") {
    runOwnerGate(context);
    const protection = readAdmissionTagRulesetProtection(context);
    const result = verifyPublishedCheckpoint(
      context,
      source,
      options,
      null,
      protection,
    );
    writeResult(context, result);
    return result;
  }

  requireHistoricalRemoteIdentities(context, source.ledger);
  const remoteCurrent = readRemoteTag(context, source.descriptor.tag);
  if (remoteCurrent !== "") {
    throw new Error(
      `${source.descriptor.tag} already exists remotely; publish never overwrites`,
    );
  }
  const local = readLocalTag(context, source.descriptor.tag);
  if (local !== null) {
    throw new Error(
      `${source.descriptor.tag} already exists locally; publication refuses resume`,
    );
  }

  runOwnerGate(context);
  runCommand(context, "go", [
    "run",
    "./cmd/standard-form-conformance",
    "current-admission-material-check",
  ]);
  if (options.phase === "prepare") {
    const protection = readAdmissionTagRulesetProtection(context);
    const result = {
      kind: "takoform.admission-deploy-result@v1",
      surface: SURFACE,
      phase: "prepare",
      status: "READY",
      version: source.descriptor.version,
      tag: source.descriptor.tag,
      commit: options.expectedCommit,
      tree: source.tree,
      tagProtection: protection,
    };
    writeResult(context, result);
    return result;
  }

  const pushURL = requirePreMutationSourceFence(
    context,
    options.expectedCommit,
  );
  const beforeLocalTagProtection = readAdmissionTagRulesetProtection(context);
  const message = buildAdmissionTagMessage({
    descriptor: source.descriptor,
    commit: options.expectedCommit,
    tree: source.tree,
    descriptorDigest: source.descriptorDigest,
    identityLedgerDigest: source.identityLedgerDigest,
    setDigest: source.setDigest,
  });
  createLocalTag(context, source.descriptor, options.expectedCommit, message);
  const localTag = requireExactLocalTag(
    context,
    source.descriptor,
    options.expectedCommit,
    message,
  );

  const prePushProtection = readAdmissionTagRulesetProtection(context);
  if (!sameTagProtection(beforeLocalTagProtection, prePushProtection)) {
    throw new Error(
      `admission-tag ruleset protection changed before push from ${beforeLocalTagProtection.fingerprint} to ${prePushProtection.fingerprint}; exact local tag ${localTag.tagObject} remains and the remote tag is unchanged; do not push it without repeating the owner flow from a clean reviewed state`,
    );
  }
  let pushError = null;
  try {
    runAuthenticatedGitPush(
      context,
      [
        "push",
        "--no-verify",
        "--porcelain",
        `--force-with-lease=refs/tags/${source.descriptor.tag}:`,
        pushURL,
        `refs/tags/${source.descriptor.tag}:refs/tags/${source.descriptor.tag}`,
      ],
      pushURL,
    );
  } catch (error) {
    pushError = error;
  }
  let postPushProtection;
  try {
    postPushProtection = readAdmissionTagRulesetProtection(context);
  } catch (error) {
    throw postPushProtectionFailure(
      context,
      source,
      options,
      localTag,
      pushError,
      error,
    );
  }
  if (!sameTagProtection(prePushProtection, postPushProtection)) {
    throw postPushProtectionFailure(
      context,
      source,
      options,
      localTag,
      pushError,
      new Error(
        `admission-tag ruleset protection changed from ${prePushProtection.fingerprint} to ${postPushProtection.fingerprint}`,
      ),
    );
  }
  if (pushError) {
    let observed;
    try {
      observed = readRemoteTag(context, source.descriptor.tag);
    } catch (error) {
      throw new Error(
        `remote push failed and authoritative ${source.descriptor.tag} visibility could not be read; exact local object ${localTag.tagObject} remains; pre/post protection stayed exact; do not retry, delete, or move the tag: ${error.message}; push error: ${pushError.message}`,
      );
    }
    if (observed === "") {
      throw new Error(
        `remote push failed before ${source.descriptor.tag} became visible; exact annotated local tag ${localTag.tagObject} remains for operator reconciliation; pre/post protection stayed exact; do not retry blindly: ${pushError.message}`,
      );
    }
    let parsed;
    try {
      parsed = parseRemoteAdmissionTag(
        observed,
        source.descriptor.tag,
        options.expectedCommit,
      );
    } catch (error) {
      throw new Error(
        `remote push failed and ${source.descriptor.tag} became visible with an unsafe or unreadable identity; exact local object ${localTag.tagObject} remains; pre/post protection stayed exact; do not retry, delete, or move the tag: ${error.message}; push error: ${pushError.message}`,
      );
    }
    if (parsed.tagObject === localTag.tagObject) {
      throw new Error(
        `remote push command failed after exact ${source.descriptor.tag} object ${parsed.tagObject} became visible; pre/post protection stayed exact but publication outcome is ambiguous; do not retry or move it: ${pushError.message}`,
      );
    }
    throw new Error(
      `remote push failed after ${source.descriptor.tag} became visible as unexpected object ${parsed.tagObject}; expected local object ${localTag.tagObject}; pre/post protection stayed exact; do not retry or move it: ${pushError.message}`,
    );
  }

  let result;
  try {
    result = verifyPublishedCheckpoint(
      context,
      source,
      options,
      localTag,
      postPushProtection,
    );
  } catch (error) {
    throw new Error(
      `admission tag push returned success and exact protection remained active, but authoritative post-conditions failed; ${source.descriptor.tag} may already be published as local object ${localTag.tagObject}; do not retry, delete, or move the tag: ${error.message}`,
    );
  }
  writeResult(context, result);
  return result;
}

function requireSourceCheckpoint(context, expectedCommit, phase) {
  const dirty = readAdmissionGitStatus(context);
  if (dirty !== "") {
    throw new Error("admission publication requires a clean worktree");
  }
  if (git(context, ["rev-parse", "--is-shallow-repository"]) !== "false") {
    throw new Error("admission publication requires complete Git history");
  }
  if (git(context, ["rev-parse", "--abbrev-ref", "HEAD"]) !== "main") {
    throw new Error("admission publication requires protected main");
  }
  const head = git(context, ["rev-parse", "HEAD"]);
  if (phase !== "verify" && head !== expectedCommit) {
    throw new Error(`HEAD ${head} does not equal --expected-commit`);
  }
  runCommand(context, "git", [
    "merge-base",
    "--is-ancestor",
    expectedCommit,
    head,
  ]);
  requireCanonicalAdmissionOrigin(context);
  const remoteMain = git(context, [
    "ls-remote",
    "--exit-code",
    "origin",
    "refs/heads/main",
  ]);
  const mainFields = remoteMain.split(/\s+/u);
  if (
    mainFields.length !== 2 ||
    mainFields[0] !== head ||
    mainFields[1] !== "refs/heads/main"
  ) {
    throw new Error(
      `protected origin/main does not equal checked-out main ${head}`,
    );
  }

  const descriptorRaw = readRegular(context.repo, DESCRIPTOR_PATH);
  const descriptor = parseAdmissionDescriptor(descriptorRaw.toString("utf8"));
  const ledgerRaw = readRegular(context.repo, IDENTITY_LEDGER_PATH);
  const ledger = parseAdmissionIdentityLedger(
    ledgerRaw.toString("utf8"),
    descriptor,
  );
  for (const retained of ledger.entries[4].retainedPaths) {
    const info = lstatSync(join(context.repo, retained));
    if (!info.isDirectory() || info.isSymbolicLink()) {
      throw new Error(`reserved v1.0.5 evidence ${retained} is not retained`);
    }
  }

  const setRaw = readRegular(context.repo, SET_PATH);
  const set = parseObject(setRaw.toString("utf8"), SET_PATH);
  if (
    set.format !== "takoform.standard-admission-set@v3" ||
    set.generation !== descriptor.generation ||
    set.admissionReleaseTag !== descriptor.tag
  ) {
    throw new Error(`${SET_PATH} does not project the current descriptor`);
  }
  const tree = git(context, ["rev-parse", `${expectedCommit}^{tree}`]);
  const taggedAdmissionTree = git(context, [
    "rev-parse",
    `${expectedCommit}:${descriptor.retainedRoot}`,
  ]);
  const headAdmissionTree = git(context, [
    "rev-parse",
    `${head}:${descriptor.retainedRoot}`,
  ]);
  if (taggedAdmissionTree !== headAdmissionTree) {
    throw new Error(
      `${descriptor.retainedRoot} differs between expected checkpoint ${expectedCommit} and current main ${head}`,
    );
  }
  return {
    descriptor,
    ledger,
    tree,
    descriptorDigest: digest(descriptorRaw),
    identityLedgerDigest: digest(ledgerRaw),
    setDigest: digest(setRaw),
  };
}

function requireHistoricalRemoteIdentities(context, ledger) {
  for (const entry of ledger.entries) {
    const raw = readRemoteTag(context, entry.tag);
    if (entry.status === "assigned-historical") {
      const observed = parseRemoteAdmissionTag(raw, entry.tag, entry.commit);
      if (observed.tagObject !== entry.tagObject) {
        throw new Error(
          `historical assigned tag ${entry.tag} moved to ${observed.tagObject}`,
        );
      }
    } else if (entry.status === "reserved-abandoned" && raw !== "") {
      throw new Error(
        `reserved-abandoned identity ${entry.tag} must never be minted`,
      );
    }
  }
}

function readAdmissionTagRulesetProtection(context) {
  const summariesRaw = githubAPI(
    context,
    `repos/${GITHUB_REPOSITORY}/rulesets?includes_parents=true&targets=tag&per_page=${MAX_RULESET_SUMMARIES}`,
  );
  let summaries;
  try {
    summaries = JSON.parse(summariesRaw);
  } catch (error) {
    throw new Error(
      `GitHub admission-tag ruleset inventory is not JSON: ${error.message}`,
    );
  }
  if (
    !Array.isArray(summaries) ||
    summaries.length === 0 ||
    summaries.length >= MAX_RULESET_SUMMARIES
  ) {
    throw new Error(
      `GitHub admission-tag ruleset inventory is empty, invalid, or pagination-ambiguous: count=${Array.isArray(summaries) ? summaries.length : "not-array"}`,
    );
  }
  const seenIDs = new Set();
  const details = summaries.map((summary, index) => {
    const value = requireObject(summary, `ruleset summaries[${index}]`);
    if (
      !Number.isSafeInteger(value.id) ||
      value.id <= 0 ||
      seenIDs.has(value.id) ||
      value.target !== "tag"
    ) {
      throw new Error(
        "GitHub admission-tag ruleset inventory contains an invalid, duplicate, or non-tag summary",
      );
    }
    seenIDs.add(value.id);
    const raw = githubAPI(
      context,
      `repos/${GITHUB_REPOSITORY}/rulesets/${value.id}?includes_parents=true`,
    );
    let detail;
    try {
      detail = JSON.parse(raw);
    } catch (error) {
      throw new Error(
        `GitHub ruleset ${value.id} is not JSON: ${error.message}`,
      );
    }
    const object = requireObject(detail, `ruleset ${value.id}`);
    if (object.id !== value.id) {
      throw new Error(
        `GitHub ruleset detail ${object.id} does not match summary ${value.id}`,
      );
    }
    return object;
  });
  return parseAdmissionTagRulesetProtection(details);
}

function githubAPI(context, endpoint) {
  return runCommand(
    context,
    resolveAdmissionGitHubCLI(),
    [
      "api",
      endpoint,
      "--header",
      "Accept: application/vnd.github+json",
      "--header",
      `X-GitHub-Api-Version: ${GITHUB_API_VERSION}`,
    ],
    { captureOnly: true, githubAPIAuthority: true },
  );
}

function sameTagProtection(left, right) {
  return (
    left.fingerprint === right.fingerprint &&
    left.creationRulesetID === right.creationRulesetID &&
    left.immutableRulesetID === right.immutableRulesetID
  );
}

function runOwnerGate(context) {
  runCommand(context, "bun", ["run", "check"]);
}

function requirePreMutationSourceFence(context, expectedCommit) {
  if (readAdmissionGitStatus(context) !== "") {
    throw new Error(
      "admission publication worktree changed after the owner gates",
    );
  }
  if (git(context, ["rev-parse", "--is-shallow-repository"]) !== "false") {
    throw new Error(
      "admission publication checkout became shallow after the owner gates",
    );
  }
  if (git(context, ["rev-parse", "--abbrev-ref", "HEAD"]) !== "main") {
    throw new Error(
      "admission publication left protected main after the owner gates",
    );
  }
  if (git(context, ["rev-parse", "HEAD"]) !== expectedCommit) {
    throw new Error("admission publication HEAD changed after the owner gates");
  }
  requireCanonicalAdmissionOrigin(context);
  const pushURL = git(context, [
    "remote",
    "get-url",
    "--push",
    "--all",
    "origin",
  ]);
  if (!CANONICAL_ORIGINS.has(pushURL)) {
    throw new Error(
      "admission publication push URL is not one exact canonical Takoform repository",
    );
  }
  const remoteMain = git(context, [
    "ls-remote",
    "--exit-code",
    "origin",
    "refs/heads/main",
  ]);
  if (remoteMain !== `${expectedCommit}\trefs/heads/main`) {
    throw new Error(
      `protected origin/main advanced or changed after the owner gates; expected ${expectedCommit}`,
    );
  }
  return pushURL;
}

export function createLocalTag(context, descriptor, expectedCommit, message) {
  const temporary = mkdtempSync(join(tmpdir(), "takoform-admission-tag-"));
  try {
    const messagePath = join(temporary, "message.txt");
    writeFileSync(messagePath, message, { encoding: "utf8", mode: 0o600 });
    runCommand(
      context,
      "git",
      [
        "-c",
        `user.name=${ADMISSION_TAGGER_NAME}`,
        "-c",
        `user.email=${ADMISSION_TAGGER_EMAIL}`,
        "-c",
        "core.hooksPath=/dev/null",
        "tag",
        "--no-sign",
        "-a",
        descriptor.tag,
        expectedCommit,
        "-F",
        messagePath,
      ],
      { gitOperation: "tag" },
    );
  } finally {
    rmSync(temporary, { force: true, recursive: true });
  }
}

function readLocalTag(context, tag) {
  const result = runCommand(
    context,
    "git",
    ["show-ref", "--verify", "--hash", `refs/tags/${tag}`],
    { allowFailure: true, captureOnly: true },
  );
  if (result.status === 1 && result.stdout === "") return null;
  if (result.status !== 0 || !COMMIT.test(result.stdout)) {
    throw new Error(`cannot determine local tag state for ${tag}`);
  }
  return { tagObject: result.stdout };
}

function requireExactLocalTag(
  context,
  descriptor,
  expectedCommit,
  expectedMessage,
) {
  const tagObject = git(context, ["rev-parse", `refs/tags/${descriptor.tag}`]);
  if (git(context, ["cat-file", "-t", tagObject]) !== "tag") {
    throw new Error(`${descriptor.tag} is not an annotated local tag`);
  }
  const commit = git(context, [
    "rev-list",
    "-n",
    "1",
    `refs/tags/${descriptor.tag}`,
  ]);
  if (commit !== expectedCommit) {
    throw new Error(
      `${descriptor.tag} resolves to ${commit}, want ${expectedCommit}`,
    );
  }
  const object = runCommand(context, "git", ["cat-file", "tag", tagObject], {
    captureOnly: true,
    preserveOutput: true,
  });
  const messageStart = object.indexOf("\n\n");
  if (messageStart < 0) {
    throw new Error(`${descriptor.tag} does not contain an annotated message`);
  }
  const actualMessage = object.slice(messageStart + 2);
  if (actualMessage !== expectedMessage) {
    throw new Error(
      `${descriptor.tag} message does not bind the retained source`,
    );
  }
  return { tagObject, commit };
}

function postPushProtectionFailure(
  context,
  source,
  options,
  localTag,
  pushError,
  protectionError,
) {
  let visibility = `remote visibility is indeterminate; exact local object ${localTag.tagObject} remains`;
  try {
    const raw = readRemoteTag(context, source.descriptor.tag);
    if (raw === "") {
      visibility = `${source.descriptor.tag} is not visible remotely`;
    } else {
      const remote = parseRemoteAdmissionTag(
        raw,
        source.descriptor.tag,
        options.expectedCommit,
      );
      visibility =
        remote.tagObject === localTag.tagObject
          ? `exact ${source.descriptor.tag} object ${remote.tagObject} is visible remotely`
          : `${source.descriptor.tag} is visible as unexpected object ${remote.tagObject}, expected ${localTag.tagObject}`;
    }
  } catch (error) {
    visibility = `remote visibility could not be reconciled: ${error.message}`;
  }
  return new Error(
    `admission tag push ${pushError ? "failed and" : "returned success but"} the immediate post-push GitHub protection proof failed; ${visibility}; do not retry, delete, or move the tag: ${protectionError.message}${pushError ? `; push error: ${pushError.message}` : ""}`,
  );
}

function verifyPublishedCheckpoint(
  context,
  source,
  options,
  knownLocal = null,
  protection = null,
) {
  const verifiedProtection =
    protection ?? readAdmissionTagRulesetProtection(context);
  requireHistoricalRemoteIdentities(context, source.ledger);
  const remote = parseRemoteAdmissionTag(
    readRemoteTag(context, source.descriptor.tag),
    source.descriptor.tag,
    options.expectedCommit,
  );
  const local =
    knownLocal ??
    requireExactLocalTag(
      context,
      source.descriptor,
      options.expectedCommit,
      buildAdmissionTagMessage({
        descriptor: source.descriptor,
        commit: options.expectedCommit,
        tree: source.tree,
        descriptorDigest: source.descriptorDigest,
        identityLedgerDigest: source.identityLedgerDigest,
        setDigest: source.setDigest,
      }),
    );
  if (remote.tagObject !== local.tagObject) {
    throw new Error(
      `remote tag object ${remote.tagObject} does not equal local annotated object ${local.tagObject}`,
    );
  }
  runCommand(context, "go", [
    "run",
    "./cmd/standard-form-conformance",
    "current-admission-closure-check",
  ]);
  return {
    kind: "takoform.admission-deploy-result@v1",
    surface: SURFACE,
    phase: options.phase,
    status: "VERIFIED",
    version: source.descriptor.version,
    tag: source.descriptor.tag,
    tagObject: remote.tagObject,
    commit: remote.commit,
    tree: source.tree,
    descriptorDigest: source.descriptorDigest,
    identityLedgerDigest: source.identityLedgerDigest,
    setDigest: source.setDigest,
    tagProtection: verifiedProtection,
  };
}

function readRemoteTag(context, tag) {
  return git(context, [
    "ls-remote",
    "origin",
    `refs/tags/${tag}`,
    `refs/tags/${tag}^{}`,
  ]);
}

function readRegular(root, relative) {
  const filename = join(root, relative);
  const info = lstatSync(filename);
  if (!info.isFile() || info.isSymbolicLink()) {
    throw new Error(`${relative} must be one regular file`);
  }
  return readFileSync(filename);
}

function parseObject(raw, label) {
  let value;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`${label} is not JSON: ${error.message}`);
  }
  return requireObject(value, label);
}

function requireObject(value, label) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} must be one object`);
  }
  return value;
}

function requireExactKeys(value, expected) {
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  if (JSON.stringify(actual) !== JSON.stringify(wanted)) {
    throw new Error(
      `object keys ${JSON.stringify(actual)} do not equal ${JSON.stringify(wanted)}`,
    );
  }
}

function digest(bytes) {
  return `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
}

function git(context, args) {
  const result = runCommand(context, "git", args, { captureOnly: true });
  return result.trim();
}

export function readAdmissionGitStatus(context) {
  return git(context, [
    "status",
    "--porcelain",
    "--untracked-files=all",
    "--ignore-submodules=none",
  ]);
}

export function requireCanonicalAdmissionOrigin(context) {
  const origin = git(context, ["remote", "get-url", "origin"]);
  if (!CANONICAL_ORIGINS.has(origin)) {
    throw new Error(
      `origin is not the canonical HTTPS Takoform repository: ${origin}`,
    );
  }
  return origin;
}

function runCommand(
  context,
  executable,
  args,
  {
    allowFailure = false,
    captureOnly = false,
    preserveOutput = false,
    gitOperation = "read-only",
    gitAuthorityPath = null,
    githubAPIAuthority = false,
    input = null,
  } = {},
) {
  if (githubAPIAuthority) {
    validateGitHubAPICommand(executable, args);
  }
  if (executable === "git") {
    validateGitOperation(gitOperation, args, gitAuthorityPath);
  }
  const environment =
    executable === "git" ? admissionGitEnvironment() : { ...process.env };
  delete environment.GH_TOKEN;
  delete environment.GITHUB_TOKEN;
  if (githubAPIAuthority) {
    for (const name of Object.keys(environment)) {
      if (
        name.startsWith("GH_") ||
        name.startsWith("GITHUB_") ||
        name.startsWith("DYLD_") ||
        name === "LD_AUDIT" ||
        name === "LD_LIBRARY_PATH" ||
        name === "LD_PRELOAD"
      ) {
        delete environment[name];
      }
    }
    environment.GH_TOKEN = context.githubToken;
  }
  if (executable === "git") {
    rejectRepositoryGitOverrides(context, environment);
  }
  const result = invokeCommand(context, executable, args, environment, input);
  const stdout = result.stdout ?? "";
  const stderr = result.stderr ?? "";
  if (!captureOnly) {
    if (stdout) context.stdout.write(stdout);
    if (stderr) context.stderr.write(stderr);
  }
  if (result.error) throw result.error;
  if (result.status !== 0 && !allowFailure) {
    throw new Error(
      `${executable} ${args.join(" ")} failed (${result.status})\n${stdout}${stderr}`,
    );
  }
  if (allowFailure) {
    return {
      status: result.status,
      stdout: stdout.trim(),
      stderr: stderr.trim(),
    };
  }
  if (preserveOutput) return stdout + stderr;
  return stdout.trim() + (stderr.trim() ? `\n${stderr.trim()}` : "");
}

function invokeCommand(context, executable, args, environment, input = null) {
  return context.commandRunner
    ? context.commandRunner(executable, args, {
        cwd: context.repo,
        env: environment,
        input,
      })
    : spawnSync(executable, args, {
        cwd: context.repo,
        encoding: "utf8",
        env: environment,
        maxBuffer: 64 * 1024 * 1024,
        input,
        stdio: [input === null ? "ignore" : "pipe", "pipe", "pipe"],
      });
}

function rejectRepositoryGitOverrides(context, environment) {
  const args = [
    "rev-parse",
    "--path-format=absolute",
    "--git-dir",
    "--git-common-dir",
    "--show-toplevel",
  ];
  const result = invokeCommand(context, "git", args, environment);
  const stdout = result.stdout ?? "";
  const stderr = result.stderr ?? "";
  if (result.error) throw result.error;
  if (result.status !== 0) {
    throw new Error(
      `git ${args.join(" ")} failed (${result.status})\n${stdout}${stderr}`,
    );
  }
  const directories = stdout.endsWith("\n")
    ? stdout.slice(0, -1).split("\n")
    : [];
  if (
    directories.length !== 3 ||
    directories.some((directory) => !isAbsolute(directory))
  ) {
    throw new Error(
      "isolated Git authority directories returned an ambiguous result",
    );
  }
  if (realpathSync(directories[2]) !== realpathSync(context.repo)) {
    throw new Error(
      "isolated Git source root differs from the requested repository",
    );
  }
  const authorityDirectories = directories.slice(0, 2);
  for (const relative of ["info/grafts", "info/attributes"]) {
    for (const authorityPath of new Set(
      authorityDirectories.map((directory) => join(directory, relative)),
    )) {
      try {
        lstatSync(authorityPath);
      } catch (error) {
        if (error?.code === "ENOENT") continue;
        throw new Error(
          `cannot inspect repository-local Git authority ${authorityPath}: ${error.message}`,
        );
      }
      throw new Error(
        `repository-local Git authority is forbidden: ${authorityPath}`,
      );
    }
  }

  const configArguments = [
    "config",
    "--show-scope",
    "--show-origin",
    "--get-regexp",
    "^(filter\\.|core\\.(attributesfile|sshcommand|worktree)$|http\\.|url\\.)",
  ];
  const config = invokeCommand(context, "git", configArguments, environment);
  if (config.error) throw config.error;
  const configLines = (config.stdout ?? "")
    .split("\n")
    .filter((line) => line !== "");
  const exactSafeConfig =
    configLines.length === 1 &&
    configLines[0] ===
      "command\tcommand line:\tcore.attributesfile /dev/null";
  if (
    (config.status === 0 && !exactSafeConfig) ||
    (config.status === 1 && configLines.length !== 0)
  ) {
    throw new Error(
      "Git executable, attribute, worktree, or transport authority is forbidden",
    );
  }
  if (config.status !== 1 && !(config.status === 0 && exactSafeConfig)) {
    throw new Error(
      `git ${configArguments.join(" ")} failed (${config.status})\n${config.stdout ?? ""}${config.stderr ?? ""}`,
    );
  }

  const indexArguments = ["ls-files", "-v", "-z"];
  const index = invokeCommand(context, "git", indexArguments, environment);
  if (index.error) throw index.error;
  if (index.status !== 0) {
    throw new Error(
      `git ${indexArguments.join(" ")} failed (${index.status})\n${index.stdout ?? ""}${index.stderr ?? ""}`,
    );
  }
  for (const record of (index.stdout ?? "").split("\0")) {
    if (record === "") continue;
    if (/^[a-zS] /u.test(record)) {
      throw new Error(
        "repository index contains assume-unchanged or skip-worktree state",
      );
    }
    const trackedPath = record.slice(2);
    let directory = dirname(trackedPath);
    for (;;) {
      const attributes = join(
        context.repo,
        directory === "." ? "" : directory,
        ".gitattributes",
      );
      try {
        lstatSync(attributes);
      } catch (error) {
        if (error?.code === "ENOENT") {
          if (directory === "." || directory === "") break;
          const parent = dirname(directory);
          if (parent === directory) break;
          directory = parent;
          continue;
        }
        throw new Error(
          `cannot inspect working-tree attribute authority ${attributes}: ${error.message}`,
        );
      }
      throw new Error(
        `working-tree Git attribute authority is forbidden: ${attributes}`,
      );
    }
  }
}

function admissionGitEnvironment() {
  const environment = { ...process.env };
  for (const name of Object.keys(environment)) {
    if (
      name.startsWith("GIT_") ||
      name.startsWith("GPG_") ||
      name.startsWith("DYLD_") ||
      name === "LD_AUDIT" ||
      name === "LD_LIBRARY_PATH" ||
      name === "LD_PRELOAD"
    ) {
      delete environment[name];
    }
  }
  for (const name of [
    "GH_TOKEN",
    "GITHUB_TOKEN",
    "GH_ENTERPRISE_TOKEN",
    "GITHUB_ENTERPRISE_TOKEN",
    "GH_HOST",
  ]) {
    delete environment[name];
  }
  delete environment.GNUPGHOME;
  environment.GIT_CONFIG_NOSYSTEM = "1";
  environment.GIT_CONFIG_GLOBAL = "/dev/null";
  environment.GIT_CONFIG_SYSTEM = "/dev/null";
  environment.GIT_CONFIG_COUNT = "5";
  environment.GIT_CONFIG_KEY_0 = "advice.graftFileDeprecated";
  environment.GIT_CONFIG_VALUE_0 = "false";
  environment.GIT_CONFIG_KEY_1 = "core.fsmonitor";
  environment.GIT_CONFIG_VALUE_1 = "false";
  environment.GIT_CONFIG_KEY_2 = "core.hooksPath";
  environment.GIT_CONFIG_VALUE_2 = "/dev/null";
  environment.GIT_CONFIG_KEY_3 = "credential.helper";
  environment.GIT_CONFIG_VALUE_3 = "";
  environment.GIT_CONFIG_KEY_4 = "core.attributesFile";
  environment.GIT_CONFIG_VALUE_4 = "/dev/null";
  environment.GIT_NO_REPLACE_OBJECTS = "1";
  environment.GIT_GRAFT_FILE = "/dev/null";
  environment.GIT_ATTR_NOSYSTEM = "1";
  environment.GIT_OPTIONAL_LOCKS = "0";
  environment.GIT_TERMINAL_PROMPT = "0";
  environment.GIT_ASKPASS = "/bin/false";
  environment.SSH_ASKPASS = "/bin/false";
  environment.LC_ALL = "C";
  return environment;
}

function validateGitOperation(operation, args, authorityPath) {
  const command = gitSubcommand(args);
  if (operation === "read-only") {
    if (command === "tag" || command === "push") {
      throw new Error(`Git mutation ${command} requires an explicit operation`);
    }
    if (authorityPath !== null) {
      throw new Error("read-only Git must not receive mutation credentials");
    }
    return;
  }
  if (operation === "tag") {
    if (
      command !== "tag" ||
      !args.includes("--no-sign") ||
      authorityPath !== null
    ) {
      throw new Error("local tag mutation is not explicitly unsigned");
    }
    return;
  }
  if (operation === "credential") {
    if (command !== "credential" || !isAbsolute(authorityPath ?? "")) {
      throw new Error(
        "Git credential validation requires one explicit operator authority",
      );
    }
    return;
  }
  if (operation === "push") {
    if (command !== "push" || !isAbsolute(authorityPath ?? "")) {
      throw new Error("Git push requires one explicit operator authority");
    }
    return;
  }
  throw new Error(`unknown Git operation ${JSON.stringify(operation)}`);
}

function gitSubcommand(args) {
  let position = 0;
  while (args[position] === "-c") {
    position += 2;
  }
  return args[position] ?? "";
}

export function runAuthenticatedGitPush(context, args, pushURL) {
  if (!CANONICAL_ORIGINS.has(pushURL)) {
    throw new Error("authenticated Git push requires one canonical push URL");
  }
  if (
    args.filter((argument) => CANONICAL_ORIGINS.has(argument)).length !== 1 ||
    !args.includes(pushURL) ||
    args.includes("origin")
  ) {
    throw new Error(
      "authenticated Git push arguments do not bind the validated canonical URL",
    );
  }
  const gh = resolveAdmissionGitHubCLI();
  const credentialArguments = operatorGitHubCredentialArguments(gh);
  verifyOperatorGitCredential(context, credentialArguments, gh);
  return runCommand(context, "git", [...credentialArguments, ...args], {
    gitOperation: "push",
    gitAuthorityPath: gh,
  });
}

export function verifyAuthenticatedGitCredential(context) {
  const gh = resolveAdmissionGitHubCLI();
  verifyOperatorGitCredential(
    context,
    operatorGitHubCredentialArguments(gh),
    gh,
  );
}

export function resolveAdmissionGitHubCLI() {
  for (const candidate of [
    "/usr/bin/gh",
    "/usr/local/bin/gh",
    "/opt/homebrew/bin/gh",
  ]) {
    try {
      const canonical = realpathSync(candidate);
      const info = lstatSync(canonical);
      if (isAbsolute(canonical) && info.isFile() && (info.mode & 0o111) !== 0) {
        return canonical;
      }
    } catch (error) {
      if (error?.code !== "ENOENT") {
        throw new Error(
          `cannot resolve fixed GitHub CLI ${candidate}: ${error.message}`,
        );
      }
    }
  }
  throw new Error("no fixed absolute GitHub CLI is available");
}

function validateGitHubAPICommand(executable, args) {
  if (
    executable !== resolveAdmissionGitHubCLI() ||
    args.length !== 6 ||
    args[0] !== "api" ||
    !/^repos\/tako0614\/terraform-provider-takoform\/rulesets(?:\?|\/[0-9]+\?)/u.test(
      args[1],
    ) ||
    args[2] !== "--header" ||
    args[3] !== "Accept: application/vnd.github+json" ||
    args[4] !== "--header" ||
    args[5] !== `X-GitHub-Api-Version: ${GITHUB_API_VERSION}`
  ) {
    throw new Error("GitHub API authority is not one exact ruleset read");
  }
}

function operatorGitHubCredentialArguments(gh) {
  return [
    "-c",
    "core.hooksPath=/dev/null",
    "-c",
    "credential.helper=",
    "-c",
    `credential.helper=!${gh} auth git-credential`,
  ];
}

function verifyOperatorGitCredential(context, credentialArguments, gh) {
  const raw = runCommand(
    context,
    "git",
    [...credentialArguments, "credential", "fill"],
    {
      captureOnly: true,
      gitOperation: "credential",
      gitAuthorityPath: gh,
      input: "protocol=https\nhost=github.com\n\n",
    },
  );
  const fields = new Map();
  for (const line of raw.split("\n")) {
    const separator = line.indexOf("=");
    if (separator <= 0 || fields.has(line.slice(0, separator))) {
      throw new Error(
        "operator Git credential helper returned an ambiguous result",
      );
    }
    fields.set(line.slice(0, separator), line.slice(separator + 1));
  }
  if (
    fields.size !== 4 ||
    fields.get("protocol") !== "https" ||
    fields.get("host") !== "github.com" ||
    !/^[^\s=]+$/u.test(fields.get("username") ?? "") ||
    !/^[^\s=]+$/u.test(fields.get("password") ?? "")
  ) {
    throw new Error(
      "operator Git credential helper did not return exact GitHub authority",
    );
  }
}

function writeResult(context, result) {
  context.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
}

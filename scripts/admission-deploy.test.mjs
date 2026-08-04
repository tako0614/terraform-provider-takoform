import { afterEach, describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import {
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import {
  ADMISSION_SURFACE,
  buildAdmissionTagMessage,
  createLocalTag,
  isAdmissionSurface,
  parseAdmissionArguments,
  parseAdmissionDescriptor,
  parseAdmissionTagRulesetProtection,
  parseRemoteAdmissionTag,
  readAdmissionGitStatus,
  requireCanonicalAdmissionOrigin,
  resolveAdmissionGitHubCLI,
  runAdmissionSurface,
  verifyAuthenticatedGitCredential,
} from "./admission-deploy.mjs";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const temporaryDirectories = [];
const expectedCommit = "0123456789abcdef0123456789abcdef01234567";
const expectedTree = "89abcdef0123456789abcdef0123456789abcdef";
const githubToken = "operator-only-test-token";

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

const descriptor = {
  format: "takoform.standard-admission-checkpoint@v1",
  version: "1.0.7",
  tag: "forms/admissions/v1.0.7",
  generation: "ga-core-v2",
  retainedRoot: "admission/v4",
};

describe("admission deploy surface", () => {
  test("declares the owner-only authority and published identity boundary", () => {
    expect(ADMISSION_SURFACE.surface).toBe("takoform-admission-release");
    expect(ADMISSION_SURFACE.triggers).toEqual([
      "authority",
      "published-identity",
    ]);
    expect(ADMISSION_SURFACE.requiresScripts).toContain("check");
    expect(ADMISSION_SURFACE.requiresTools).toEqual(["git", "bun", "go", "gh"]);
    expect(ADMISSION_SURFACE.requiresEnv).toEqual(["GH_TOKEN"]);
    expect(Object.keys(ADMISSION_SURFACE.obligations).sort()).toEqual(
      [
        "failure-handling",
        "independent-review",
        "no-overwrite",
        "post-conditions",
        "provenance",
        "reversal",
      ].sort(),
    );
    expect(isAdmissionSurface("takoform-admission-release")).toBe(true);
    expect(isAdmissionSurface("takoform-standard-admission-checkpoint")).toBe(
      false,
    );
  });

  test("accepts only the prepare, publish, and verify phases with one exact commit", () => {
    const commit = "0123456789abcdef0123456789abcdef01234567";
    for (const phase of ["prepare", "publish", "verify"]) {
      expect(
        parseAdmissionArguments([phase, "--expected-commit", commit]),
      ).toEqual({ expectedCommit: commit, phase });
    }
    for (const args of [
      [],
      ["publish"],
      ["delete", "--expected-commit", commit],
      ["publish", "--expected-commit", "HEAD"],
      [
        "publish",
        "--expected-commit",
        commit,
        "--tag",
        "forms/admissions/v1.0.7",
      ],
    ]) {
      expect(() => parseAdmissionArguments(args)).toThrow();
    }
  });
});

describe("admission checkpoint identity", () => {
  test("strictly binds canonical v4 identity", () => {
    expect(parseAdmissionDescriptor(JSON.stringify(descriptor))).toEqual(
      descriptor,
    );
    for (const mutate of [
      (value) => ({ ...value, unexpected: true }),
      (value) => ({ ...value, version: "1.0.7-rc.1" }),
      (value) => ({ ...value, tag: "forms/admissions/v1.0.6" }),
      (value) => ({ ...value, generation: "ga-core-v1" }),
      (value) => ({ ...value, retainedRoot: "admission/v3" }),
    ]) {
      expect(() =>
        parseAdmissionDescriptor(JSON.stringify(mutate(descriptor))),
      ).toThrow();
    }
  });

  test("requires one remote annotated tag object and its exact peeled commit", () => {
    const tagObject = "1234567890abcdef1234567890abcdef12345678";
    const commit = "0123456789abcdef0123456789abcdef01234567";
    const tag = descriptor.tag;
    expect(
      parseRemoteAdmissionTag(
        `${tagObject}\trefs/tags/${tag}\n${commit}\trefs/tags/${tag}^{}\n`,
        tag,
        commit,
      ),
    ).toEqual({ commit, tagObject });
    expect(() =>
      parseRemoteAdmissionTag(`${commit}\trefs/tags/${tag}\n`, tag, commit),
    ).toThrow("annotated");
  });

  test("tag message binds the exact source tree and retained descriptor/ledger/set bytes", () => {
    const message = buildAdmissionTagMessage({
      descriptor,
      commit: "0123456789abcdef0123456789abcdef01234567",
      tree: "89abcdef0123456789abcdef0123456789abcdef",
      descriptorDigest:
        "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      identityLedgerDigest:
        "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      setDigest:
        "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    });
    expect(message).toContain("Activate Standard Form admission v1.0.7");
    expect(message).toContain("generation ga-core-v2");
    expect(message).toContain(
      "commit 0123456789abcdef0123456789abcdef01234567",
    );
    expect(message).toContain("tree 89abcdef0123456789abcdef0123456789abcdef");
    expect(message).toContain(
      "version-descriptor sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    );
    expect(message).toContain(
      "identity-ledger sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    );
    expect(message).toContain(
      "standard-admission-set sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    );
  });

  test("accepts only separate exact active creation and non-bypassable immutable tag rulesets", () => {
    const protection = parseAdmissionTagRulesetProtection(
      exactAdmissionRulesets(),
    );
    expect(protection.creationRulesetID).toBe(101);
    expect(protection.immutableRulesetID).toBe(102);
    expect(protection.fingerprint).toMatch(/^sha256:[0-9a-f]{64}$/);

    const cases = [
      exactAdmissionRulesets().filter((ruleset) => ruleset.id !== 101),
      mutateRuleset(102, (ruleset) => ({
        ...ruleset,
        rules: ruleset.rules.filter((rule) => rule.type !== "non_fast_forward"),
      })),
      mutateRuleset(102, (ruleset) => ({
        ...ruleset,
        rules: ruleset.rules.filter((rule) => rule.type !== "update"),
      })),
      mutateRuleset(102, (ruleset) => ({
        ...ruleset,
        rules: ruleset.rules.map((rule) =>
          rule.type === "update"
            ? {
                ...rule,
                parameters: { update_allows_fetch_and_merge: true },
              }
            : rule,
        ),
      })),
      mutateRuleset(101, (ruleset) => ({
        ...ruleset,
        conditions: {
          ref_name: {
            include: ["refs/tags/forms/*/v*"],
            exclude: [],
          },
        },
      })),
      mutateRuleset(101, (ruleset) => ({
        ...ruleset,
        enforcement: "evaluate",
      })),
      mutateRuleset(101, (ruleset) => ({
        ...ruleset,
        conditions: {
          ...ruleset.conditions,
          repository_name: {
            include: ["terraform-provider-takoform"],
            exclude: [],
          },
        },
      })),
      mutateRuleset(102, (ruleset) => ({
        ...ruleset,
        bypass_actors: [
          {
            actor_id: 5,
            actor_type: "RepositoryRole",
            bypass_mode: "always",
          },
        ],
        current_user_can_bypass: "always",
      })),
      [
        ...exactAdmissionRulesets(),
        {
          ...exactAdmissionRulesets()[0],
          id: 104,
          name: "ambiguous duplicate creation",
        },
      ],
    ];
    for (const value of cases) {
      expect(() => parseAdmissionTagRulesetProtection(value)).toThrow();
    }
  });
});

describe("admission deploy execution", () => {
  test("prepare, publish, and verify execute one create-only tag/readback flow", async () => {
    const repo = sourceFixture();
    const fake = fakeCommands();
    const io = memoryIO();

    const prepared = await runAdmissionSurface({
      surface: ADMISSION_SURFACE.surface,
      args: ["prepare", "--expected-commit", expectedCommit],
      repo,
      stdout: io.stdout,
      stderr: io.stderr,
      commandRunner: fake.run,
      githubToken,
    });
    expect(prepared.status).toBe("READY");
    expect(fake.state.local).toBeNull();

    const published = await runAdmissionSurface({
      surface: ADMISSION_SURFACE.surface,
      args: ["publish", "--expected-commit", expectedCommit],
      repo,
      stdout: io.stdout,
      stderr: io.stderr,
      commandRunner: fake.run,
      githubToken,
    });
    expect(published.status).toBe("VERIFIED");
    expect(fake.state.remoteCurrent?.tagObject).toBe(
      fake.state.local?.tagObject,
    );
    expect(fake.state.local?.message).toContain(
      `commit ${expectedCommit}\ntree ${expectedTree}\n`,
    );

    const verified = await runAdmissionSurface({
      surface: ADMISSION_SURFACE.surface,
      args: ["verify", "--expected-commit", expectedCommit],
      repo,
      stdout: io.stdout,
      stderr: io.stderr,
      commandRunner: fake.run,
      githubToken,
    });
    expect(verified.tagObject).toBe(fake.state.local?.tagObject);
    expect(
      fake.state.calls.filter(
        ([command, args]) =>
          command === "go" && args.at(-1) === "current-admission-closure-check",
      ).length,
    ).toBe(2);
    const protectionInventoryCalls = fake.state.calls.filter(
      ([command, args]) =>
        isGitHubCLI(command) &&
        args[0] === "api" &&
        args[1].includes("rulesets?"),
    );
    expect(protectionInventoryCalls).toHaveLength(5);
    expect(
      fake.state.calls.some(
        ([command, args]) =>
          command === "git" &&
          args
            .join(" ")
            .startsWith(
              "-c user.name=Takoform Standard Admission -c user.email=admission@takoform.invalid -c core.hooksPath=/dev/null tag --no-sign -a ",
            ),
      ),
    ).toBe(true);
    const pushCall = fake.state.calls.find(
      ([command, args]) => command === "git" && args.includes("push"),
    );
    expect(pushCall?.[1]).toContain(
      "https://github.com/tako0614/terraform-provider-takoform.git",
    );
    expect(pushCall?.[1]).not.toContain("origin");
    for (const [command, , options] of fake.state.calls) {
      if (isGitHubCLI(command)) {
        expect(options.env.GH_TOKEN).toBe(githubToken);
        expect(options.env.GITHUB_TOKEN).toBeUndefined();
      } else {
        expect(options.env.GH_TOKEN).toBeUndefined();
        expect(options.env.GITHUB_TOKEN).toBeUndefined();
      }
      if (command === "git") {
        expect(options.env.GIT_NO_REPLACE_OBJECTS).toBe("1");
        expect(options.env.GIT_GRAFT_FILE).toBe("/dev/null");
        expect(options.env.GIT_CONFIG_COUNT).toBe("5");
        expect(options.env.GIT_CONFIG_KEY_0).toBe("advice.graftFileDeprecated");
        expect(options.env.GIT_CONFIG_VALUE_0).toBe("false");
        expect(options.env.GIT_CONFIG_KEY_1).toBe("core.fsmonitor");
        expect(options.env.GIT_CONFIG_VALUE_1).toBe("false");
        expect(options.env.GIT_CONFIG_KEY_2).toBe("core.hooksPath");
        expect(options.env.GIT_CONFIG_VALUE_2).toBe("/dev/null");
        expect(options.env.GIT_CONFIG_KEY_3).toBe("credential.helper");
        expect(options.env.GIT_CONFIG_VALUE_3).toBe("");
        expect(options.env.GIT_CONFIG_KEY_4).toBe("core.attributesFile");
        expect(options.env.GIT_CONFIG_VALUE_4).toBe("/dev/null");
        expect(options.env.GIT_ATTR_NOSYSTEM).toBe("1");
        expect(options.env.GIT_CONFIG_NOSYSTEM).toBe("1");
        expect(options.env.GIT_CONFIG_GLOBAL).toBe("/dev/null");
        expect(options.env.GIT_CONFIG_SYSTEM).toBe("/dev/null");
      }
    }
  });

  test("scrubs ambient Git config and replacement authority from every Git subprocess", async () => {
    process.env.GIT_DIR = "/tmp/attacker-git-dir";
    process.env.GIT_WORK_TREE = "/tmp/attacker-work-tree";
    process.env.GIT_REPLACE_REF_BASE = "refs/attacker/";
    process.env.GIT_CONFIG_PARAMETERS = "'gpg.program=/tmp/fake-gpg'";
    process.env.GIT_CONFIG_COUNT = "1";
    process.env.GIT_CONFIG_KEY_0 = "gpg.program";
    process.env.GIT_CONFIG_VALUE_0 = "/tmp/fake-gpg";
    process.env.GH_ENTERPRISE_TOKEN = "attacker-enterprise-token";
    process.env.GITHUB_ENTERPRISE_TOKEN = "attacker-github-enterprise-token";
    process.env.GH_HOST = "attacker.invalid";
    process.env.LD_AUDIT = "/tmp/attacker-audit.so";
    process.env.LD_LIBRARY_PATH = "/tmp/attacker-library";
    process.env.LD_PRELOAD = "/tmp/attacker-preload.so";
    process.env.DYLD_INSERT_LIBRARIES = "/tmp/attacker-dyld.dylib";
    try {
      const fake = fakeCommands();
      await runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["prepare", "--expected-commit", expectedCommit],
        repo: sourceFixture(),
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      });
      const gitCalls = fake.state.calls.filter(
        ([command]) => command === "git",
      );
      expect(gitCalls.length).toBeGreaterThan(0);
      for (const [, , options] of gitCalls) {
        expect(options.env.GIT_DIR).toBeUndefined();
        expect(options.env.GIT_WORK_TREE).toBeUndefined();
        expect(options.env.GIT_REPLACE_REF_BASE).toBeUndefined();
        expect(options.env.GIT_CONFIG_PARAMETERS).toBeUndefined();
        expect(options.env.GIT_CONFIG_COUNT).toBe("5");
        expect(options.env.GIT_CONFIG_KEY_0).toBe("advice.graftFileDeprecated");
        expect(options.env.GIT_CONFIG_VALUE_0).toBe("false");
        expect(options.env.GIT_CONFIG_KEY_1).toBe("core.fsmonitor");
        expect(options.env.GIT_CONFIG_VALUE_1).toBe("false");
        expect(options.env.GIT_CONFIG_KEY_2).toBe("core.hooksPath");
        expect(options.env.GIT_CONFIG_VALUE_2).toBe("/dev/null");
        expect(options.env.GIT_CONFIG_KEY_3).toBe("credential.helper");
        expect(options.env.GIT_CONFIG_VALUE_3).toBe("");
        expect(options.env.GIT_CONFIG_KEY_4).toBe("core.attributesFile");
        expect(options.env.GIT_CONFIG_VALUE_4).toBe("/dev/null");
        expect(options.env.GIT_ATTR_NOSYSTEM).toBe("1");
        expect(options.env.GIT_NO_REPLACE_OBJECTS).toBe("1");
        expect(options.env.GIT_GRAFT_FILE).toBe("/dev/null");
        expect(options.env.GIT_CONFIG_NOSYSTEM).toBe("1");
        expect(options.env.GIT_CONFIG_GLOBAL).toBe("/dev/null");
        expect(options.env.GIT_CONFIG_SYSTEM).toBe("/dev/null");
        expect(options.env.GH_ENTERPRISE_TOKEN).toBeUndefined();
        expect(options.env.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
        expect(options.env.GH_HOST).toBeUndefined();
        expect(options.env.LD_AUDIT).toBeUndefined();
        expect(options.env.LD_LIBRARY_PATH).toBeUndefined();
        expect(options.env.LD_PRELOAD).toBeUndefined();
        expect(options.env.DYLD_INSERT_LIBRARIES).toBeUndefined();
      }
      for (const [, , options] of fake.state.calls.filter(([command]) =>
        isGitHubCLI(command),
      )) {
        expect(options.env.GH_TOKEN).toBe(githubToken);
        expect(options.env.GH_ENTERPRISE_TOKEN).toBeUndefined();
        expect(options.env.GITHUB_ENTERPRISE_TOKEN).toBeUndefined();
        expect(options.env.GH_HOST).toBeUndefined();
        expect(options.env.LD_AUDIT).toBeUndefined();
        expect(options.env.LD_LIBRARY_PATH).toBeUndefined();
        expect(options.env.LD_PRELOAD).toBeUndefined();
        expect(options.env.DYLD_INSERT_LIBRARIES).toBeUndefined();
      }
    } finally {
      delete process.env.GIT_DIR;
      delete process.env.GIT_WORK_TREE;
      delete process.env.GIT_REPLACE_REF_BASE;
      delete process.env.GIT_CONFIG_PARAMETERS;
      delete process.env.GIT_CONFIG_COUNT;
      delete process.env.GIT_CONFIG_KEY_0;
      delete process.env.GIT_CONFIG_VALUE_0;
      delete process.env.GH_ENTERPRISE_TOKEN;
      delete process.env.GITHUB_ENTERPRISE_TOKEN;
      delete process.env.GH_HOST;
      delete process.env.LD_AUDIT;
      delete process.env.LD_LIBRARY_PATH;
      delete process.env.LD_PRELOAD;
      delete process.env.DYLD_INSERT_LIBRARIES;
    }
  });

  test("rejects repository-local Git graft authority before owner Git checks", async () => {
    const repo = sourceFixture();
    writeFileSync(
      join(repo, ".git/info/grafts"),
      `${expectedCommit} ffffffffffffffffffffffffffffffffffffffff\n`,
    );
    const fake = fakeCommands();
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["prepare", "--expected-commit", expectedCommit],
        repo,
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow("Git authority is forbidden");
    expect(
      fake.state.calls.some(
        ([command, args]) =>
          command === "git" &&
          args.join(" ") ===
            "rev-parse --path-format=absolute --git-dir --git-common-dir --show-toplevel",
      ),
    ).toBe(true);
  });

  test("real Git mutation uses the fixed unsigned tagger and isolated explicit credential", () => {
    const repo = mkdtempSync(join(tmpdir(), "takoform-admission-real-git-"));
    temporaryDirectories.push(repo);
    runTestGit(repo, "init", "--quiet", "--initial-branch=main");
    writeFileSync(join(repo, "fixture.txt"), "fixture\n");
    runTestGit(repo, "add", "fixture.txt");
    runTestGit(
      repo,
      "-c",
      "user.name=Fixture",
      "-c",
      "user.email=fixture@takoform.invalid",
      "commit",
      "--quiet",
      "-m",
      "fixture",
    );
    const commit = runTestGit(repo, "rev-parse", "HEAD").trim();
    const programRoot = mkdtempSync(
      join(tmpdir(), "takoform-admission-ambient-"),
    );
    temporaryDirectories.push(programRoot);
    const marker = join(programRoot, "ambient-program-ran");
    const fakeProgram = join(programRoot, "ambient-program");
    writeFileSync(
      fakeProgram,
      `#!/bin/sh\nprintf ran > ${JSON.stringify(marker)}\nexit 1\n`,
      { mode: 0o700 },
    );
    const fakeBin = join(programRoot, "bin");
    mkdirSync(fakeBin, { recursive: true });
    const fakeGHMarker = join(programRoot, "fake-gh-ran");
    writeFileSync(
      join(fakeBin, "gh"),
      `#!/bin/sh\nprintf ran > ${JSON.stringify(fakeGHMarker)}\nexit 1\n`,
      { mode: 0o700 },
    );
    const previousPath = process.env.PATH;
    process.env.PATH = `${fakeBin}:${previousPath ?? ""}`;
    try {
      const resolvedGH = resolveAdmissionGitHubCLI();
      expect(resolvedGH).not.toBe(join(fakeBin, "gh"));
      execFileSync(resolvedGH, ["--version"], { stdio: "ignore" });
    } finally {
      if (previousPath === undefined) {
        delete process.env.PATH;
      } else {
        process.env.PATH = previousPath;
      }
    }
    expect(() => lstatSync(fakeGHMarker)).toThrow();
    runTestGit(repo, "config", "tag.gpgSign", "true");
    runTestGit(repo, "config", "gpg.program", fakeProgram);
    runTestGit(repo, "config", "core.fsmonitor", fakeProgram);
    runTestGit(repo, "config", "user.name", "Ambient Identity");
    runTestGit(repo, "config", "user.email", "ambient@example.invalid");

    const io = memoryIO();
    const context = {
      repo,
      githubToken,
      stdout: io.stdout,
      stderr: io.stderr,
    };
    const tag = "forms/admissions/v9.9.9";
    expect(readAdmissionGitStatus(context)).toBe("");
    expect(() => lstatSync(marker)).toThrow();
    createLocalTag(context, { tag }, commit, "exact mutation fixture\n");
    const tagObject = runTestGit(repo, "cat-file", "tag", `refs/tags/${tag}`);
    expect(tagObject).toContain(
      "tagger Takoform Standard Admission <admission@takoform.invalid>",
    );
    expect(tagObject).not.toContain("BEGIN PGP SIGNATURE");
    expect(() => lstatSync(marker)).toThrow();

    runTestGit(
      repo,
      "remote",
      "add",
      "origin",
      "git@github.com:tako0614/terraform-provider-takoform.git",
    );
    expect(() => requireCanonicalAdmissionOrigin(context)).toThrow(
      "canonical HTTPS",
    );
    expect(() => lstatSync(marker)).toThrow();

    const globalConfig = join(programRoot, "ambient-global-config");
    writeFileSync(globalConfig, `[credential]\n\thelper = !${fakeProgram}\n`, {
      mode: 0o600,
    });
    const ghConfig = join(programRoot, "gh-config");
    mkdirSync(ghConfig, { recursive: true });
    writeFileSync(
      join(ghConfig, "hosts.yml"),
      "github.com:\n    user: stored-operator\n    oauth_token: stored-operator-token\n    git_protocol: https\n",
      { mode: 0o600 },
    );
    const previousGitConfig = process.env.GIT_CONFIG_GLOBAL;
    const previousGHConfig = process.env.GH_CONFIG_DIR;
    process.env.GIT_CONFIG_GLOBAL = globalConfig;
    process.env.GH_CONFIG_DIR = ghConfig;
    try {
      verifyAuthenticatedGitCredential(context);
    } finally {
      if (previousGitConfig === undefined) {
        delete process.env.GIT_CONFIG_GLOBAL;
      } else {
        process.env.GIT_CONFIG_GLOBAL = previousGitConfig;
      }
      if (previousGHConfig === undefined) {
        delete process.env.GH_CONFIG_DIR;
      } else {
        process.env.GH_CONFIG_DIR = previousGHConfig;
      }
    }
    expect(() => lstatSync(marker)).toThrow();

    const filterMarker = join(programRoot, "filter-ran");
    const filterProgram = join(programRoot, "filter-program");
    writeFileSync(
      filterProgram,
      `#!/bin/sh\nprintf ran > ${JSON.stringify(filterMarker)}\nprintf "fixture\\n"\n`,
      { mode: 0o700 },
    );
    writeFileSync(join(repo, ".gitattributes"), "fixture.txt filter=review\n");
    runTestGit(repo, "add", ".gitattributes");
    runTestGit(
      repo,
      "-c",
      "user.name=Fixture",
      "-c",
      "user.email=fixture@takoform.invalid",
      "commit",
      "--quiet",
      "-m",
      "attributes fixture",
    );
    runTestGit(repo, "config", "filter.review.clean", filterProgram);
    writeFileSync(join(repo, "fixture.txt"), "different worktree bytes\n");
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "Git executable, attribute, worktree, or transport authority is forbidden",
    );
    expect(() => lstatSync(filterMarker)).toThrow();

    runTestGit(repo, "config", "--unset-all", "filter.review.clean");
    writeFileSync(join(repo, "fixture.txt"), "fixture\n");
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "working-tree Git attribute authority is forbidden",
    );
    runTestGit(repo, "rm", "--quiet", ".gitattributes");
    runTestGit(
      repo,
      "-c",
      "user.name=Fixture",
      "-c",
      "user.email=fixture@takoform.invalid",
      "commit",
      "--quiet",
      "-m",
      "remove attributes fixture",
    );
    runTestGit(repo, "update-index", "--assume-unchanged", "fixture.txt");
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "assume-unchanged or skip-worktree",
    );
    runTestGit(repo, "update-index", "--no-assume-unchanged", "fixture.txt");
    runTestGit(repo, "update-index", "--skip-worktree", "fixture.txt");
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "assume-unchanged or skip-worktree",
    );
    runTestGit(repo, "update-index", "--no-skip-worktree", "fixture.txt");

    writeFileSync(join(repo, ".git/info/attributes"), "* -diff\n");
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "Git authority is forbidden",
    );
    rmSync(join(repo, ".git/info/attributes"));

    runTestGit(repo, "config", "http.sslVerify", "false");
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "transport authority is forbidden",
    );
    runTestGit(repo, "config", "--unset-all", "http.sslVerify");

    runTestGit(
      repo,
      "config",
      "url.https://attacker.invalid/.insteadOf",
      "https://github.com/",
    );
    expect(() => readAdmissionGitStatus(context)).toThrow(
      "transport authority is forbidden",
    );
    runTestGit(
      repo,
      "config",
      "--unset-all",
      "url.https://attacker.invalid/.insteadOf",
    );

    const alternateWorktree = mkdtempSync(
      join(tmpdir(), "takoform-admission-alternate-worktree-"),
    );
    temporaryDirectories.push(alternateWorktree);
    runTestGit(repo, "config", "core.worktree", alternateWorktree);
    expect(() => readAdmissionGitStatus(context)).toThrow(
      /source root differs|worktree.*authority is forbidden/u,
    );
  });

  test("rejects a moved historical tag and a minted abandoned v1.0.5", async () => {
    for (const changes of [
      { movedHistorical: true },
      { mintedAbandoned: true },
    ]) {
      const repo = sourceFixture();
      const fake = fakeCommands(changes);
      await expect(
        runAdmissionSurface({
          surface: ADMISSION_SURFACE.surface,
          args: ["prepare", "--expected-commit", expectedCommit],
          repo,
          stdout: memoryIO().stdout,
          stderr: memoryIO().stderr,
          commandRunner: fake.run,
          githubToken,
        }),
      ).rejects.toThrow(
        changes.movedHistorical ? "moved" : "must never be minted",
      );
    }
  });

  test("rejects an existing current tag and distinguishes exact from differing ambiguous pushes", async () => {
    {
      const repo = sourceFixture();
      const fake = fakeCommands({
        remoteCurrent: {
          tagObject: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
          commit: expectedCommit,
        },
      });
      await expect(
        runAdmissionSurface({
          surface: ADMISSION_SURFACE.surface,
          args: ["publish", "--expected-commit", expectedCommit],
          repo,
          stdout: memoryIO().stdout,
          stderr: memoryIO().stderr,
          commandRunner: fake.run,
          githubToken,
        }),
      ).rejects.toThrow("already exists remotely");
    }
    {
      const repo = sourceFixture();
      const fake = fakeCommands({ pushDiffering: true });
      await expect(
        runAdmissionSurface({
          surface: ADMISSION_SURFACE.surface,
          args: ["publish", "--expected-commit", expectedCommit],
          repo,
          stdout: memoryIO().stdout,
          stderr: memoryIO().stderr,
          commandRunner: fake.run,
          githubToken,
        }),
      ).rejects.toThrow("unexpected object");
    }
    {
      const repo = sourceFixture();
      const fake = fakeCommands({ pushExactButError: true });
      await expect(
        runAdmissionSurface({
          surface: ADMISSION_SURFACE.surface,
          args: ["publish", "--expected-commit", expectedCommit],
          repo,
          stdout: memoryIO().stdout,
          stderr: memoryIO().stderr,
          commandRunner: fake.run,
          githubToken,
        }),
      ).rejects.toThrow("publication outcome is ambiguous");
      expect(
        fake.state.calls.filter(
          ([command, args]) => command === "git" && args.includes("push"),
        ),
      ).toHaveLength(1);
    }
  });

  test("refuses tag creation when protected main advances after the owner gates", async () => {
    const repo = sourceFixture();
    const fake = fakeCommands({ advanceMainAfterGate: true });
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["publish", "--expected-commit", expectedCommit],
        repo,
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow("origin/main advanced or changed after the owner gates");
    expect(
      fake.state.calls.some(
        ([command, args]) =>
          command === "git" && (args[0] === "tag" || args[0] === "push"),
      ),
    ).toBe(false);
  });

  test("refuses a non-canonical push URL before local tag mutation", async () => {
    const fake = fakeCommands({
      pushURL: "https://attacker.invalid/takoform.git",
    });
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["publish", "--expected-commit", expectedCommit],
        repo: sourceFixture(),
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow("push URL is not one exact canonical");
    expect(fake.state.local).toBeNull();
    expect(fake.state.pushAttempts).toBe(0);
  });

  test("fails closed after a successful push if exact protection changes and never retries", async () => {
    const repo = sourceFixture();
    const fake = fakeCommands({ changeProtectionAfterPush: true });
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["publish", "--expected-commit", expectedCommit],
        repo,
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow("immediate post-push GitHub protection proof failed");
    expect(fake.state.remoteCurrent?.tagObject).toBe(
      fake.state.local?.tagObject,
    );
    expect(
      fake.state.calls.filter(
        ([command, args]) => command === "git" && args.includes("push"),
      ),
    ).toHaveLength(1);
  });

  test("reports an exact remote mutation without retry when post-closure verification fails", async () => {
    const fake = fakeCommands({ closureFailure: true });
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["publish", "--expected-commit", expectedCommit],
        repo: sourceFixture(),
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow(
      "push returned success and exact protection remained active",
    );
    expect(fake.state.remoteCurrent?.tagObject).toBe(
      fake.state.local?.tagObject,
    );
    expect(
      fake.state.calls.filter(
        ([command, args]) => command === "git" && args.includes("push"),
      ),
    ).toHaveLength(1);
  });

  test("blocks before push when the live immutable ruleset is incomplete", async () => {
    const repo = sourceFixture();
    const fake = fakeCommands({ omitNonFastForward: true });
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["publish", "--expected-commit", expectedCommit],
        repo,
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow("ambiguous or incomplete");
    expect(fake.state.local).toBeNull();
    expect(
      fake.state.calls.some(
        ([command, args]) => command === "git" && args[0] === "push",
      ),
    ).toBe(false);
  });

  test("blocks a pagination-ambiguous live ruleset inventory", async () => {
    const fake = fakeCommands({ paginationAmbiguous: true });
    await expect(
      runAdmissionSurface({
        surface: ADMISSION_SURFACE.surface,
        args: ["prepare", "--expected-commit", expectedCommit],
        repo: sourceFixture(),
        stdout: memoryIO().stdout,
        stderr: memoryIO().stderr,
        commandRunner: fake.run,
        githubToken,
      }),
    ).rejects.toThrow("pagination-ambiguous");
    expect(
      fake.state.calls.filter(
        ([command, args]) =>
          isGitHubCLI(command) &&
          args[0] === "api" &&
          /\/rulesets\/[0-9]+\?/u.test(args[1]),
      ),
    ).toHaveLength(0);
  });

  test("rejects missing or whitespace-bearing GitHub authority before running commands", async () => {
    for (const token of [null, "", "bad token", "bad\nvalue"]) {
      const fake = fakeCommands();
      await expect(
        runAdmissionSurface({
          surface: ADMISSION_SURFACE.surface,
          args: ["prepare", "--expected-commit", expectedCommit],
          repo: sourceFixture(),
          stdout: memoryIO().stdout,
          stderr: memoryIO().stderr,
          commandRunner: fake.run,
          githubToken: token,
        }),
      ).rejects.toThrow("GH_TOKEN");
      expect(fake.state.calls).toHaveLength(0);
    }
  });
});

function sourceFixture() {
  const repo = mkdtempSync(join(tmpdir(), "takoform-admission-deploy-"));
  temporaryDirectories.push(repo);
  for (const relative of [
    ".git/info",
    "admission/v4",
    "admission/v3/candidates/host-report-1.0.5-63dabf0c64be-bd0b3184aaad",
    "admission/v3/candidates/provider-report-1.0.5-bd0b3184aaad",
    "admission/v3/candidates/registry-readback-1.0.5-bd0b3184aaad",
  ]) {
    mkdirSync(join(repo, relative), { recursive: true });
  }
  writeFileSync(
    join(repo, "admission/v4/version.json"),
    readFileSync(join(repositoryRoot, "admission/v4/version.json")),
  );
  mkdirSync(join(repo, "admission"), { recursive: true });
  writeFileSync(
    join(repo, "admission/admission-identities.json"),
    readFileSync(join(repositoryRoot, "admission/admission-identities.json")),
  );
  writeFileSync(
    join(repo, "admission/v4/standard-admission-set.json"),
    `${JSON.stringify({
      format: "takoform.standard-admission-set@v3",
      generation: "ga-core-v2",
      admissionReleaseTag: "forms/admissions/v1.0.7",
    })}\n`,
  );
  return repo;
}

function exactAdmissionRulesets() {
  return [
    {
      id: 101,
      name: "Restrict Standard Form admission tag creation",
      target: "tag",
      source_type: "Repository",
      source: "tako0614/terraform-provider-takoform",
      enforcement: "active",
      current_user_can_bypass: "always",
      bypass_actors: [
        {
          actor_id: 5,
          actor_type: "RepositoryRole",
          bypass_mode: "always",
        },
      ],
      conditions: {
        ref_name: {
          include: ["refs/tags/forms/admissions/v*"],
          exclude: [],
        },
      },
      rules: [{ type: "creation" }],
    },
    {
      id: 102,
      name: "Keep Standard Form admission tags immutable",
      target: "tag",
      source_type: "Repository",
      source: "tako0614/terraform-provider-takoform",
      enforcement: "active",
      current_user_can_bypass: "never",
      bypass_actors: [],
      conditions: {
        ref_name: {
          include: ["refs/tags/forms/admissions/v*"],
          exclude: [],
        },
      },
      rules: [
        { type: "deletion" },
        { type: "non_fast_forward" },
        {
          type: "update",
          parameters: { update_allows_fetch_and_merge: false },
        },
      ],
    },
    {
      id: 103,
      name: "Broader Form Package tag protection",
      target: "tag",
      source_type: "Repository",
      source: "tako0614/terraform-provider-takoform",
      enforcement: "active",
      current_user_can_bypass: "never",
      bypass_actors: [],
      conditions: {
        ref_name: {
          include: ["refs/tags/forms/*/v*"],
          exclude: [],
        },
      },
      rules: [{ type: "deletion" }, { type: "non_fast_forward" }],
    },
  ];
}

function mutateRuleset(id, mutate) {
  return exactAdmissionRulesets().map((ruleset) =>
    ruleset.id === id ? mutate(structuredClone(ruleset)) : ruleset,
  );
}

function fakeCommands({
  movedHistorical = false,
  mintedAbandoned = false,
  pushDiffering = false,
  pushExactButError = false,
  advanceMainAfterGate = false,
  changeProtectionAfterPush = false,
  closureFailure = false,
  omitNonFastForward = false,
  paginationAmbiguous = false,
  remoteCurrent = null,
  pushURL = "https://github.com/tako0614/terraform-provider-takoform.git",
} = {}) {
  const historical = new Map([
    [
      "forms/admissions/v1.0.1",
      {
        tagObject: movedHistorical
          ? "ffffffffffffffffffffffffffffffffffffffff"
          : "2b1ca9f68688392869a79de122fbce2a54842301",
        commit: "57aba7f374bb0d45274044e1dacbea52d16f3f6b",
      },
    ],
    [
      "forms/admissions/v1.0.2",
      {
        tagObject: "98af8dd461f24e6dc902f5c56dc6740f74ceb5af",
        commit: "ff65142ecfab206f58239f095b5e170854ef9dde",
      },
    ],
    [
      "forms/admissions/v1.0.3",
      {
        tagObject: "82af8a61666e0194506d0d23d04422ccda4b3d86",
        commit: "4a40826c7ed467af84e856487998ce365ffe00dd",
      },
    ],
    [
      "forms/admissions/v1.0.4",
      {
        tagObject: "b49a55016362d8787966f41b14570e3b67b8ddba",
        commit: "a426a379e2743b4345e868becf3618357c015447",
      },
    ],
    [
      "forms/admissions/v1.0.6",
      {
        tagObject: "b34b13a6e2fd3acbcbd73935e3e353f5d05b5c31",
        commit: "1e438d61ed77f1ccfd3e000250f7dcf0c578c1af",
      },
    ],
  ]);
  const state = {
    calls: [],
    local: null,
    mainReads: 0,
    pushAttempts: 0,
    remoteCurrent,
  };
  const success = (stdout = "") => ({ status: 0, stdout, stderr: "" });
  const currentRulesets = () => {
    if (paginationAmbiguous) {
      return Array.from({ length: 100 }, (_, index) => ({
        ...exactAdmissionRulesets()[2],
        id: 1000 + index,
        name: `broader tag protection ${index}`,
      }));
    }
    let rulesets = exactAdmissionRulesets();
    if (omitNonFastForward) {
      rulesets = rulesets.map((ruleset) =>
        ruleset.id === 102
          ? {
              ...ruleset,
              rules: ruleset.rules.filter(
                (rule) => rule.type !== "non_fast_forward",
              ),
            }
          : ruleset,
      );
    }
    if (!changeProtectionAfterPush || state.pushAttempts === 0) return rulesets;
    return rulesets.map((ruleset) => {
      if (ruleset.id === 101) return { ...ruleset, id: 201 };
      if (ruleset.id === 102) return { ...ruleset, id: 202 };
      return ruleset;
    });
  };
  const run = (command, args, options = {}) => {
    state.calls.push([
      command,
      [...args],
      { ...options, env: { ...(options.env ?? {}) } },
    ]);
    if (command === "bun") return success("gate passed\n");
    if (command === "go") {
      if (
        closureFailure &&
        state.pushAttempts > 0 &&
        args.at(-1) === "current-admission-closure-check"
      ) {
        return {
          status: 1,
          stdout: "",
          stderr: "retained closure mismatch\n",
        };
      }
      return success("gate passed\n");
    }
    if (isGitHubCLI(command)) {
      if (args[0] !== "api") {
        throw new Error(`unexpected gh command: ${args.join(" ")}`);
      }
      const endpoint = args[1];
      const rulesets = currentRulesets();
      if (endpoint.includes("/rulesets?")) {
        return success(
          `${JSON.stringify(
            rulesets.map((ruleset) => ({
              id: ruleset.id,
              name: ruleset.name,
              target: ruleset.target,
              source_type: ruleset.source_type,
              source: ruleset.source,
              enforcement: ruleset.enforcement,
            })),
          )}\n`,
        );
      }
      const detail = endpoint.match(/\/rulesets\/([0-9]+)\?/u);
      if (detail) {
        const ruleset = rulesets.find(
          (candidate) => candidate.id === Number(detail[1]),
        );
        if (!ruleset) throw new Error(`unknown fake ruleset ${detail[1]}`);
        return success(`${JSON.stringify(ruleset)}\n`);
      }
      throw new Error(`unexpected gh endpoint: ${endpoint}`);
    }
    if (command !== "git") throw new Error(`unexpected command ${command}`);
    const joined = args.join(" ");
    if (
      joined ===
      "rev-parse --path-format=absolute --git-dir --git-common-dir --show-toplevel"
    ) {
      return success(
        `${join(options.cwd, ".git")}\n${join(options.cwd, ".git")}\n${options.cwd}\n`,
      );
    }
    if (
      joined ===
      "config --show-scope --show-origin --get-regexp ^(filter\\.|core\\.(attributesfile|sshcommand|worktree)$|http\\.|url\\.)"
    ) {
      return { status: 1, stdout: "", stderr: "" };
    }
    if (joined === "ls-files -v -z") return success();
    if (
      joined ===
      "status --porcelain --untracked-files=all --ignore-submodules=none"
    ) {
      return success();
    }
    if (joined === "rev-parse --is-shallow-repository") {
      return success("false\n");
    }
    if (joined === "rev-parse --abbrev-ref HEAD") return success("main\n");
    if (joined === "rev-parse HEAD") return success(`${expectedCommit}\n`);
    if (joined === "remote get-url origin") {
      return success(
        "https://github.com/tako0614/terraform-provider-takoform.git\n",
      );
    }
    if (joined === "remote get-url --push --all origin") {
      return success(`${pushURL}\n`);
    }
    if (joined === "ls-remote --exit-code origin refs/heads/main") {
      state.mainReads += 1;
      if (advanceMainAfterGate && state.mainReads > 1) {
        return success(
          "ffffffffffffffffffffffffffffffffffffffff\trefs/heads/main\n",
        );
      }
      return success(`${expectedCommit}\trefs/heads/main\n`);
    }
    if (args[0] === "merge-base") return success();
    if (joined === `rev-parse ${expectedCommit}^{tree}`) {
      return success(`${expectedTree}\n`);
    }
    if (joined === `rev-parse ${expectedCommit}:admission/v4`) {
      return success("1111111111111111111111111111111111111111\n");
    }
    if (args[0] === "ls-remote" && args[1] === "origin") {
      const tag = args[2].slice("refs/tags/".length);
      let identity = historical.get(tag) ?? null;
      if (tag === "forms/admissions/v1.0.5" && mintedAbandoned) {
        identity = {
          tagObject: "5555555555555555555555555555555555555555",
          commit: expectedCommit,
        };
      }
      if (tag === descriptor.tag) identity = state.remoteCurrent;
      return success(remoteTagOutput(tag, identity));
    }
    if (
      joined ===
      `rev-parse --verify --quiet refs/tags/${descriptor.tag}`
    ) {
      return state.local
        ? success(`${state.local.tagObject}\n`)
        : { status: 1, stdout: "", stderr: "" };
    }
    const tagCommand = args.indexOf("tag");
    if (
      tagCommand >= 0 &&
      args[tagCommand + 1] === "--no-sign" &&
      args[tagCommand + 2] === "-a"
    ) {
      state.local = {
        tagObject: "6666666666666666666666666666666666666666",
        commit: expectedCommit,
        message: readFileSync(args[args.indexOf("-F") + 1], "utf8"),
      };
      return success();
    }
    if (joined === `rev-parse refs/tags/${descriptor.tag}`) {
      return success(`${state.local.tagObject}\n`);
    }
    if (joined === `cat-file -t ${state.local?.tagObject}`) {
      return success("tag\n");
    }
    if (args[0] === "rev-list") return success(`${expectedCommit}\n`);
    if (joined === `cat-file tag ${state.local?.tagObject}`) {
      return success(
        `object ${expectedCommit}\ntype commit\ntag ${descriptor.tag}\ntagger Test <test@example.invalid> 0 +0000\n\n${state.local.message}`,
      );
    }
    if (joined.endsWith("credential fill")) {
      expect(options.input).toBe("protocol=https\nhost=github.com\n\n");
      expect(options.env.GH_TOKEN).toBeUndefined();
      expect(options.env.GITHUB_TOKEN).toBeUndefined();
      return success(
        "protocol=https\nhost=github.com\nusername=stored-operator\npassword=stored-operator-token\n",
      );
    }
    if (args.includes("push")) {
      state.pushAttempts += 1;
      if (pushExactButError) {
        state.remoteCurrent = {
          tagObject: state.local.tagObject,
          commit: expectedCommit,
        };
        return { status: 1, stdout: "", stderr: "connection closed\n" };
      }
      if (pushDiffering) {
        state.remoteCurrent = {
          tagObject: "7777777777777777777777777777777777777777",
          commit: expectedCommit,
        };
        return { status: 1, stdout: "", stderr: "rejected\n" };
      }
      state.remoteCurrent = {
        tagObject: state.local.tagObject,
        commit: expectedCommit,
      };
      return success("ok\n");
    }
    throw new Error(`unexpected git command: ${joined}`);
  };
  return { run, state };
}

function remoteTagOutput(tag, identity) {
  if (!identity) return "";
  return (
    `${identity.tagObject}\trefs/tags/${tag}\n` +
    `${identity.commit}\trefs/tags/${tag}^{}\n`
  );
}

function memoryIO() {
  let output = "";
  return {
    stdout: { write: (value) => (output += value) },
    stderr: { write: (value) => (output += value) },
    read: () => output,
  };
}

function runTestGit(repo, ...args) {
  return execFileSync("git", ["-C", repo, ...args], {
    encoding: "utf8",
    env: {
      ...process.env,
      GIT_CONFIG_NOSYSTEM: "1",
      GIT_CONFIG_GLOBAL: "/dev/null",
      GIT_CONFIG_SYSTEM: "/dev/null",
    },
  });
}

function isGitHubCLI(command) {
  return command === "gh" || command.endsWith("/gh");
}

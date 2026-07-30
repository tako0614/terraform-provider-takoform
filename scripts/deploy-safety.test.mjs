import { afterEach, describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  assertPublicationManifest,
  assertSafeRepositoryGitConfiguration,
  collectRegularFiles,
  createCommittedPublicationManifest,
  createCommittedSnapshot,
  createHardenedGateEnvironment,
  createHardenedGitEnvironment,
  createPublicationManifest,
  inspectUncommittedPublicationPaths,
  parseCurrentProductionDeployment,
  pinnedWranglerInvocation,
} from "./deploy-safety.mjs";

const temporaryDirectories = [];

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

describe("production deployment readback", () => {
  const current = {
    id: "4ed02082-0ac8-469f-aca7-cd3862e6b348",
    strategy: "percentage",
    versions: [
      {
        percentage: 100,
        version_id: "37dd55a9-d75b-452f-ab62-77486fb7204e",
      },
    ],
  };

  test("selects the sole version receiving 100% production traffic", () => {
    expect(parseCurrentProductionDeployment(JSON.stringify(current))).toEqual({
      deploymentId: current.id,
      versionId: current.versions[0].version_id,
    });
  });

  test.each([
    {
      ...current,
      versions: [
        {
          percentage: 50,
          version_id: "37dd55a9-d75b-452f-ab62-77486fb7204e",
        },
        {
          percentage: 50,
          version_id: "067ae8ca-1300-4de6-94ee-c56f3ca65000",
        },
      ],
    },
    {
      ...current,
      versions: [{ ...current.versions[0], percentage: 99 }],
    },
  ])("rejects an ambiguous or split deployment", (deployment) => {
    expect(() =>
      parseCurrentProductionDeployment(JSON.stringify(deployment)),
    ).toThrow("single 100% version is required");
  });
});

test("published asset discovery rejects symbolic links", () => {
  const directory = mkdtempSync(join(tmpdir(), "takoform-deploy-safety-"));
  temporaryDirectories.push(directory);
  const outside = join(directory, "outside.txt");
  writeFileSync(outside, "outside\n");
  symlinkSync(outside, join(directory, "linked.txt"));

  expect(() => collectRegularFiles(directory)).toThrow(
    "published asset must not be a symbolic link",
  );
});

function temporaryGitRepository() {
  const directory = mkdtempSync(join(tmpdir(), "takoform-deploy-git-"));
  temporaryDirectories.push(directory);
  const git = (...args) =>
    execFileSync("git", args, {
      cwd: directory,
      encoding: "utf8",
      env: createHardenedGitEnvironment(process.env),
      stdio: ["ignore", "pipe", "pipe"],
    }).trim();
  git("init", "--initial-branch=main");
  git("config", "user.email", "test@example.invalid");
  git("config", "user.name", "Takoform test");
  mkdirSync(join(directory, "website", "public"), { recursive: true });
  writeFileSync(
    join(directory, ".gitignore"),
    "website/public/*.tfstate\n",
  );
  writeFileSync(
    join(directory, "website", "wrangler.jsonc"),
    '{"name":"test"}\n',
  );
  writeFileSync(join(directory, "website", "public", "index.html"), "v1\n");
  mkdirSync(join(directory, "scripts"));
  writeFileSync(
    join(directory, "scripts", "check-public-surfaces.mjs"),
    'process.stdout.write("ok\\n");\n',
  );
  git("add", ".");
  git("commit", "-m", "fixture");
  return { directory, git };
}

describe("committed publication snapshot", () => {
  const publicationPaths = ["website/wrangler.jsonc", "website/public"];

  test("is isolated from a later live-worktree edit and matches Git blobs", () => {
    const { directory, git } = temporaryGitRepository();
    const commit = git("rev-parse", "HEAD");
    const snapshot = createCommittedSnapshot({ commit, repo: directory });
    try {
      const expected = createCommittedPublicationManifest({
        commit,
        paths: ["."],
        repo: directory,
      });
      const before = createPublicationManifest(snapshot.root, ["."]);
      expect(before).toEqual(expected);

      writeFileSync(join(directory, "website", "public", "index.html"), "v2\n");

      const after = createPublicationManifest(snapshot.root, ["."]);
      expect(after).toEqual(before);
      expect(
        readFileSync(
          join(snapshot.root, "website", "public", "index.html"),
          "utf8",
        ),
      ).toBe("v1\n");
    } finally {
      snapshot.dispose();
    }
  });

  test("rejects ignored and untracked files below publication paths", () => {
    const { directory } = temporaryGitRepository();
    writeFileSync(
      join(directory, "website", "public", "local.tfstate"),
      "secret state\n",
    );
    writeFileSync(
      join(directory, "website", "public", "untracked.txt"),
      "untracked\n",
    );

    expect(
      inspectUncommittedPublicationPaths({
        paths: publicationPaths,
        repo: directory,
      }),
    ).toEqual({
      ignored: ["website/public/local.tfstate"],
      untracked: ["website/public/untracked.txt"],
    });
  });

  test("mutation fence detects a changed snapshot before a writer runs", () => {
    const { directory, git } = temporaryGitRepository();
    const snapshot = createCommittedSnapshot({
      commit: git("rev-parse", "HEAD"),
      repo: directory,
    });
    try {
      const before = createPublicationManifest(snapshot.root, publicationPaths);
      writeFileSync(
        join(snapshot.root, "website", "public", "index.html"),
        "mutated\n",
      );
      const after = createPublicationManifest(
        snapshot.root,
        publicationPaths,
      );

      expect(() => assertPublicationManifest(before, after)).toThrow(
        "publication snapshot changed after verification",
      );
    } finally {
      snapshot.dispose();
    }
  });

  test("whole-tree fence also detects mutation of a gate outside asset paths", () => {
    const { directory, git } = temporaryGitRepository();
    const snapshot = createCommittedSnapshot({
      commit: git("rev-parse", "HEAD"),
      repo: directory,
    });
    try {
      const before = createPublicationManifest(snapshot.root, ["."]);
      writeFileSync(
        join(snapshot.root, "scripts", "check-public-surfaces.mjs"),
        'process.stdout.write("bypassed\\n");\n',
      );
      const after = createPublicationManifest(snapshot.root, ["."]);
      expect(() => assertPublicationManifest(before, after)).toThrow(
        "scripts/check-public-surfaces.mjs",
      );
    } finally {
      snapshot.dispose();
    }
  });
});

test("hardened Git environment removes inherited Git configuration", () => {
  const environment = createHardenedGitEnvironment({
    GIT_CONFIG_COUNT: "1",
    GIT_CONFIG_KEY_0: "url.file:///attacker/.insteadOf",
    GIT_CONFIG_VALUE_0: "https://github.com/",
    GIT_REPLACE_REF_BASE: "refs/evil/",
    PATH: "/usr/bin",
  });

  expect(environment.PATH).toBe("/usr/bin");
  expect(environment.GIT_CONFIG_COUNT).toBeUndefined();
  expect(environment.GIT_REPLACE_REF_BASE).toBeUndefined();
  expect(environment).toMatchObject({
    GIT_CONFIG_GLOBAL: "/dev/null",
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_SYSTEM: "/dev/null",
    GIT_NO_REPLACE_OBJECTS: "1",
  });
});

test("snapshot gate cannot resolve Bun or authority from ambient PATH", () => {
  const environment = createHardenedGateEnvironment(
    {
      BUN_CONFIG_FILE: "/tmp/attacker.toml",
      CLOUDFLARE_API_TOKEN: "must-not-reach-gate",
      HOME: "/tmp/attacker-home",
      NODE_OPTIONS: "--require=/tmp/attacker.cjs",
      npm_config_userconfig: "/tmp/attacker-npmrc",
      PATH: "/tmp/fake-bin:/usr/bin",
      TAKOFORM_CLOUDFLARE_ACCOUNT_ID: "a".repeat(32),
      WRANGLER_CI_OVERRIDE_NAME: "other-worker",
    },
    "/trusted/bun/bin/bun",
    "/private/gate-home",
  );
  expect(environment.PATH).toBe(
    "/trusted/bun/bin:/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin",
  );
  expect(environment.CLOUDFLARE_API_TOKEN).toBeUndefined();
  expect(environment.BUN_CONFIG_FILE).toBeUndefined();
  expect(environment.HOME).toBe("/private/gate-home");
  expect(environment.NODE_OPTIONS).toBeUndefined();
  expect(environment.npm_config_userconfig).toBeUndefined();
  expect(environment.TAKOFORM_CLOUDFLARE_ACCOUNT_ID).toBeUndefined();
  expect(environment.WRANGLER_CI_OVERRIDE_NAME).toBeUndefined();
  expect(environment.XDG_CONFIG_HOME).toBe(
    "/private/gate-home/.config",
  );
});

test("repository Git configuration rejects local transport rewrites", () => {
  const canonical =
    "https://github.com/tako0614/terraform-provider-takoform.git";
  const ordinary = [
    "core.repositoryformatversion\n0",
    "core.filemode\ntrue",
    "core.bare\nfalse",
    "core.logallrefupdates\ntrue",
    `remote.origin.url\n${canonical}`,
    "remote.origin.fetch\n+refs/heads/*:refs/remotes/origin/*",
    "branch.main.remote\norigin",
    "branch.main.merge\nrefs/heads/main",
    "",
  ].join("\0");
  expect(() =>
    assertSafeRepositoryGitConfiguration(ordinary, canonical),
  ).not.toThrow();
  expect(() =>
    assertSafeRepositoryGitConfiguration(
      ordinary.replace(
        "\0branch.main.remote",
        "\0url.file:///attacker/.insteadof\nhttps://github.com/\0branch.main.remote",
      ),
      canonical,
    ),
  ).toThrow("can influence publication: url.file:///attacker/.insteadof");
  expect(() =>
    assertSafeRepositoryGitConfiguration(
      ordinary.replace(
        "\0branch.main.remote",
        "\0core.sshcommand\n/tmp/attacker\0branch.main.remote",
      ),
      canonical,
    ),
  ).toThrow("can influence publication: core.sshcommand");
});

test("pinned Wrangler bypasses PATH and its env-based node shebang", () => {
  expect(
    pinnedWranglerInvocation(
      "/sealed/node_modules/wrangler/bin/wrangler.js",
      ["--version"],
      "/trusted/bun",
    ),
  ).toEqual({
    args: [
      "--config=/dev/null",
      "--no-env-file",
      "/sealed/node_modules/wrangler/bin/wrangler.js",
      "--version",
    ],
    command: "/trusted/bun",
  });
  expect(() =>
    pinnedWranglerInvocation(
      "/sealed/node_modules/wrangler/bin/wrangler.js",
      [],
      "node",
    ),
  ).toThrow("absolute paths");
});

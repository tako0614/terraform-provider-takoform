import { afterEach, describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import {
  existsSync,
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
  createPublicationManifestFromEntries,
  diagnostics,
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

test("website snapshot copy includes its complete module closure", () => {
  const deploySource = readFileSync(new URL("./deploy.mjs", import.meta.url), "utf8");
  expect(deploySource).toContain(
    '"scripts/check-website-dist.mjs",\n    "scripts/frozen-public-identities.mjs",\n    "scripts/website-html-normalization.mjs",\n    "scripts/website-snapshot-temp.mjs",\n    "scripts/website-snapshot-temp.test.mjs",',
  );
  const contract = JSON.parse(
    execFileSync(process.execPath, ["scripts/deploy.mjs", "--contract"], {
      cwd: new URL("..", import.meta.url),
      encoding: "utf8",
      stdio: ["ignore", "pipe", "pipe"],
    }),
  );
  const website = contract.surfaces.find(
    (surface) => surface.surface === "takoform-website",
  );
  expect(website).toBeDefined();
  const provenance = website.obligations.provenance;
  expect(provenance).toContain("two concurrent read-only VitePress builds");
  expect(provenance).toContain(
    "page byte-for-byte after the writer's trailing-whitespace-only normalization",
  );
  expect(provenance).toContain(
    "content-addressed asset set by exact path and bytes",
  );
  expect(provenance).toContain("`hashmap.json` is the sole byte exception");
  expect(provenance).toContain("exact page-key set");
  expect(provenance).not.toContain("page semantically");
  expect(provenance).not.toContain("asset set by role");
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
    const snapshot = createCommittedSnapshot({
      commit,
      environment: createHardenedGitEnvironment({
        ...process.env,
        GIT_DIR: "/tmp/attacker-git-dir",
        GIT_OBJECT_DIRECTORY: "/tmp/attacker-objects",
        GIT_WORK_TREE: "/tmp/attacker-worktree",
      }),
      repo: directory,
    });
    try {
      expect(existsSync(join(snapshot.root, ".git"))).toBe(false);
      expect(
        execFileSync(
          "git",
          ["-C", snapshot.authorityRoot, "rev-parse", "HEAD"],
          { encoding: "utf8" },
        ).trim(),
      ).toBe(commit);
      expect(
        execFileSync(
          "git",
          ["-C", snapshot.authorityRoot, "rev-parse", "--abbrev-ref", "HEAD"],
          { encoding: "utf8" },
        ).trim(),
      ).toBe("HEAD");
      expect(
        existsSync(
          join(snapshot.authorityRoot, ".git", "objects", "info", "alternates"),
        ),
      ).toBe(false);

      const expected = createCommittedPublicationManifest({
        commit,
        paths: ["."],
        repo: directory,
      });
      const before = createPublicationManifest(snapshot.root, ["."]);
      expect(before).toEqual(expected);
      expect(
        createPublicationManifestFromEntries(
          snapshot.authorityRoot,
          expected.entries,
        ),
      ).toEqual(expected);

      writeFileSync(join(directory, "website", "public", "index.html"), "v2\n");
      git("add", "website/public/index.html");
      git("commit", "-m", "advance source after snapshot");
      git("tag", "post-snapshot-live-ref");
      expect(git("rev-parse", "HEAD")).not.toBe(commit);

      const after = createPublicationManifest(snapshot.root, ["."]);
      expect(after).toEqual(before);
      expect(
        readFileSync(
          join(snapshot.root, "website", "public", "index.html"),
          "utf8",
        ),
      ).toBe("v1\n");
      expect(() =>
        execFileSync(
          "git",
          [
            "-C",
            snapshot.authorityRoot,
            "show-ref",
            "--verify",
            "refs/tags/post-snapshot-live-ref",
          ],
          { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
        ),
      ).toThrow();
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
      GIT_DIR: "/tmp/attacker.git",
      GIT_OBJECT_DIRECTORY: "/tmp/attacker-objects",
      GIT_REPLACE_REF_BASE: "refs/evil/",
      GIT_WORK_TREE: "/tmp/attacker-worktree",
    PATH: "/usr/bin",
  });

  expect(environment.PATH).toBe("/usr/bin");
  expect(environment.GIT_CONFIG_COUNT).toBeUndefined();
  expect(environment.GIT_DIR).toBeUndefined();
  expect(environment.GIT_OBJECT_DIRECTORY).toBeUndefined();
  expect(environment.GIT_REPLACE_REF_BASE).toBeUndefined();
  expect(environment.GIT_WORK_TREE).toBeUndefined();
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
      CGO_CFLAGS: "-include /tmp/attacker.h",
      CLOUDFLARE_API_TOKEN: "must-not-reach-gate",
      GOENV: "/tmp/attacker-goenv",
      GOFLAGS: "-toolexec=/tmp/attacker",
      GOPROXY: "https://attacker.invalid",
      GOWORK: "/tmp/attacker.work",
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
  expect(environment.CGO_CFLAGS).toBeUndefined();
  expect(environment.CGO_ENABLED).toBe("0");
  expect(environment.GOAUTH).toBe("off");
  expect(environment.GOENV).toBe("off");
  expect(environment.GOFLAGS).toBe("-mod=readonly -buildvcs=false");
  expect(environment.GOMODCACHE).toBe("/private/gate-home/go/pkg/mod");
  expect(environment.GOPATH).toBe("/private/gate-home/go");
  expect(environment.GOPROXY).toBe("https://proxy.golang.org");
  expect(environment.GOSUMDB).toBe("sum.golang.org");
  expect(environment.GOTOOLCHAIN).toBe("local");
  expect(environment.GOVCS).toBe("*:off");
  expect(environment.GOWORK).toBe("off");
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

test("pinned Wrangler uses an absolute Node executable and bypasses PATH and its shebang", () => {
  expect(
    pinnedWranglerInvocation(
      "/sealed/node_modules/wrangler/bin/wrangler.js",
      ["--version"],
      "/trusted/node",
    ),
  ).toEqual({
    args: ["/sealed/node_modules/wrangler/bin/wrangler.js", "--version"],
    command: "/trusted/node",
  });
  expect(() =>
    pinnedWranglerInvocation(
      "/sealed/node_modules/wrangler/bin/wrangler.js",
      [],
      "node",
    ),
  ).toThrow("absolute Node");
});

describe("a blocked deploy says what went wrong", () => {
  // The failure-handling obligation is raw diagnostics AND no blind retry.
  // They are one requirement: an operator told only that a step is
  // indeterminate has been left with retrying as the only move.
  test("an ordinary Error's message reaches the operator", () => {
    expect(diagnostics(new Error("production status is aaa/bbb"))).toContain(
      "production status is aaa/bbb",
    );
  });

  test("a subprocess failure keeps its own output", () => {
    const failure = Object.assign(new Error("command failed"), {
      stdout: "uploading\n",
      stderr: "Error 10007: version not found\n",
    });
    const rendered = diagnostics(failure);
    expect(rendered).toContain("Error 10007: version not found");
    expect(rendered).toContain("uploading");
  });

  // Asserted against a real failure rather than a hand-built one: execFileSync
  // quotes the captured stderr INSIDE its message, so a containment test
  // written the other way round prints the same stderr twice.
  test("a real subprocess failure prints its stderr once, with the command", () => {
    let failure;
    try {
      // The stderr text is assembled at runtime so it is NOT a substring of
      // the command line the message also quotes; otherwise the count below
      // would measure the test's own echo argument.
      execFileSync("/bin/sh", ["-c", "echo \"zone $((400 + 3))\" >&2; exit 7"], {
        encoding: "utf8",
        stdio: "pipe",
      });
    } catch (error) {
      failure = error;
    }
    const rendered = diagnostics(failure);
    expect(rendered.match(/zone 403/g)).toHaveLength(1);
    expect(rendered).toContain("/bin/sh");
  });

  test("a subprocess that already printed the message does not print it twice", () => {
    const failure = Object.assign(new Error("boom"), { stdout: "", stderr: "boom\n" });
    expect(diagnostics(failure).match(/boom/g)).toHaveLength(1);
  });

  test("a step that produced nothing says so rather than printing a blank line", () => {
    expect(diagnostics(Object.assign(new Error(""), { stdout: "", stderr: "" }))).toBe(
      "no diagnostic was produced by the failing step\n",
    );
  });

  test("a cause is not swallowed", () => {
    expect(
      diagnostics(new Error("fence failed", { cause: new Error("zone 403") })),
    ).toContain("zone 403");
  });
});

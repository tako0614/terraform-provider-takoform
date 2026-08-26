import { afterEach, describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  chownSync,
  existsSync,
  linkSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  renameSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";

import {
  assertPublicationManifest,
  assertManagedGateState,
  assertManagedToolSnapshot,
  assertSafeRepositoryGitConfiguration,
  collectRegularFiles,
  createCommittedPublicationManifest,
  createCommittedSnapshot,
  createHardenedGateEnvironment,
  createHardenedGitEnvironment,
  createManagedGateState,
  createManagedToolSnapshot,
  createPublicationManifest,
  createPublicationManifestFromEntries,
  diagnostics,
  inspectUncommittedPublicationPaths,
  parseCurrentProductionDeployment,
  pinnedWranglerInvocation,
  prepareManagedHomeForRemoval,
} from "./deploy-safety.mjs";

const repositoryRoot = resolve(import.meta.dir, "..");
const temporaryDirectories = [];
const managedHomes = [];

afterEach(() => {
  for (const managedHome of managedHomes.splice(0)) {
    if (existsSync(managedHome)) prepareManagedHomeForRemoval(managedHome);
  }
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

function safeToolFixture({ debianLayout = false } = {}) {
  const root = mkdtempSync(
    join(dirname(repositoryRoot), ".takoform-deploy-tool-test-"),
  );
  temporaryDirectories.push(root);
  const tofuBin = join(root, "tofu-bin");
  const terraformBin = join(root, "terraform-user-bin");
  const goRoot = debianLayout
    ? join(root, "usr", "lib", "go-1.26")
    : join(root, "go-root");
  const goBin = join(goRoot, "bin");
  const goToolDir = join(goRoot, "pkg", "tool", "linux_amd64");
  const goIncludeDir = join(goRoot, "pkg", "include");
  const goSourceDir = join(goRoot, "src", "runtime");
  const goEmptyDir = join(goRoot, "misc", "empty");
  const managedHome = join(root, "managed-home");
  for (const directory of [
    tofuBin,
    terraformBin,
    goBin,
    goToolDir,
    goIncludeDir,
    goSourceDir,
    goEmptyDir,
    managedHome,
  ]) {
    mkdirSync(directory, { recursive: true, mode: 0o700 });
  }
  managedHomes.push(managedHome);
  const paths = {
    compile: join(goToolDir, "compile"),
    go: join(goBin, "go"),
    gofmt: join(goBin, "gofmt"),
    include: join(goIncludeDir, "textflag.h"),
    source: join(goSourceDir, "runtime.go"),
    terraform: join(terraformBin, "terraform"),
    tofu: join(tofuBin, "tofu"),
  };
  for (const [name, path] of Object.entries(paths).filter(
    ([name]) => name !== "include" && name !== "source",
  )) {
    writeFileSync(path, `#!/bin/sh\n# ${name}\nexit 0\n`);
    chmodSync(path, 0o500);
  }
  writeFileSync(paths.include, "#define TEXTFLAG 1\n");
  writeFileSync(paths.source, "package runtime\n");
  chmodSync(paths.include, 0o400);
  chmodSync(paths.source, 0o400);
  const siblingBun = join(terraformBin, "bun");
  writeFileSync(siblingBun, "#!/bin/sh\nexit 97\n");
  chmodSync(siblingBun, 0o500);
  return {
    environment: {
      PATH: [tofuBin, terraformBin, goBin, "/usr/bin"].join(":"),
    },
    goBin,
    goEmptyDir,
    goIncludeDir,
    goRoot,
    goSourceDir,
    goToolDir,
    managedHome,
    paths,
    root,
    siblingBun,
    terraformBin,
    tofuBin,
  };
}

function toolSnapshot(fixture = safeToolFixture()) {
  return {
    fixture,
    snapshot: createManagedToolSnapshot({
      environment: fixture.environment,
      managedHome: fixture.managedHome,
    }),
  };
}

function managedFixturePath(fixture, snapshot, sourcePath) {
  return join(snapshot.go.root, relative(fixture.goRoot, sourcePath));
}

function collectTreeKinds(root, logicalPath = ".") {
  const path = logicalPath === "." ? root : join(root, logicalPath);
  const metadata = lstatSync(path);
  const found = [{ logicalPath, metadata }];
  if (metadata.isDirectory() && !metadata.isSymbolicLink()) {
    for (const child of readdirSync(path).sort()) {
      found.push(
        ...collectTreeKinds(
          root,
          logicalPath === "." ? child : join(logicalPath, child),
        ),
      );
    }
  }
  return found;
}

function replaceReadOnlyFile(path, bytes) {
  chmodSync(path, 0o700);
  writeFileSync(path, bytes);
  chmodSync(path, 0o500);
}

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
  const deploySource = readFileSync(
    new URL("./deploy.mjs", import.meta.url),
    "utf8",
  );
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
  writeFileSync(join(directory, ".gitignore"), "website/public/*.tfstate\n");
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
      const after = createPublicationManifest(snapshot.root, publicationPaths);

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
      SSH_ASKPASS: "/tmp/attacker-askpass",
      SSH_AUTH_SOCK: "/tmp/attacker-agent.sock",
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
  expect(environment.GOCACHE).toBe("/private/gate-home/m/go-build");
  expect(environment.GOMODCACHE).toBe("/private/gate-home/m/go-mod");
  expect(environment.GOPATH).toBe("/private/gate-home/m/go-path");
  expect(environment.GOPROXY).toBe("https://proxy.golang.org");
  expect(environment.GOSUMDB).toBe("sum.golang.org");
  expect(environment.GOTOOLCHAIN).toBe("local");
  expect(environment.GOCACHEPROG).toBeUndefined();
  expect(environment.GOVCS).toBe("*:off");
  expect(environment.GOWORK).toBe("off");
  expect(environment.HOME).toBe("/private/gate-home");
  expect(environment.TMPDIR).toBe("/private/gate-home/m/t");
  expect(environment.NODE_OPTIONS).toBeUndefined();
  expect(environment.npm_config_userconfig).toBeUndefined();
  expect(environment.SSH_ASKPASS).toBeUndefined();
  expect(environment.SSH_AUTH_SOCK).toBeUndefined();
  expect(environment.TAKOFORM_CLOUDFLARE_ACCOUNT_ID).toBeUndefined();
  expect(environment.WRANGLER_CI_OVERRIDE_NAME).toBeUndefined();
  expect(environment.XDG_CONFIG_HOME).toBe("/private/gate-home/.config");
});

describe("owner gate tool nomination", () => {
  test("copies user-local CLIs and never inherits their sibling PATH authority", () => {
    const { fixture, snapshot } = toolSnapshot();
    const terraformBytes = readFileSync(snapshot.tools.terraform.path);
    expect(lstatSync(snapshot.toolBin).mode & 0o7777).toBe(0o700);
    expect(lstatSync(snapshot.tools.terraform.path).mode & 0o7777).toBe(0o500);

    const replacement = join(fixture.terraformBin, "terraform.next");
    writeFileSync(replacement, "#!/bin/sh\nexit 88\n");
    chmodSync(replacement, 0o500);
    renameSync(replacement, fixture.paths.terraform);
    expect(readFileSync(snapshot.tools.terraform.path)).toEqual(terraformBytes);
    expect(() => assertManagedToolSnapshot(snapshot)).not.toThrow();

    const environment = createHardenedGateEnvironment(
      {
        GH_TOKEN: "scrub-me",
        GITHUB_TOKEN: "scrub-me-too",
        PATH: fixture.environment.PATH,
        TF_CLI_ARGS: "-plugin-dir=/tmp/attacker",
        TOFU_CLI_ARGS: "-plugin-dir=/tmp/attacker",
      },
      "/trusted/bun/bin/bun",
      fixture.managedHome,
      {
        managedToolBin: snapshot.toolBin,
        goBin: snapshot.go.bin,
        goRoot: snapshot.go.root,
      },
    );
    expect(
      environment.PATH.startsWith(`${snapshot.go.bin}:${snapshot.toolBin}:`),
    ).toBe(true);
    expect(environment.PATH).not.toContain(fixture.terraformBin);
    expect(environment.PATH).not.toContain(fixture.tofuBin);
    expect(environment.GH_TOKEN).toBeUndefined();
    expect(environment.GITHUB_TOKEN).toBeUndefined();
    expect(environment.TF_CLI_ARGS).toBeUndefined();
    expect(environment.TOFU_CLI_ARGS).toBeUndefined();
  });

  test("materializes the complete logical GOROOT as a sealed private tree", () => {
    const { fixture, snapshot } = toolSnapshot();
    expect(snapshot.go.sourceRoot).toBe(fixture.goRoot);
    expect(snapshot.go.root).toBe(join(fixture.managedHome, "goroot"));
    expect(snapshot.go.bin).toBe(join(snapshot.go.root, "bin"));
    expect(snapshot.go.go.path).toBe(join(snapshot.go.bin, "go"));
    expect(snapshot.go.gofmt.path).toBe(join(snapshot.go.bin, "gofmt"));

    for (const sourcePath of [
      fixture.paths.compile,
      fixture.paths.include,
      fixture.paths.source,
    ]) {
      const managedPath = managedFixturePath(fixture, snapshot, sourcePath);
      expect(readFileSync(managedPath)).toEqual(readFileSync(sourcePath));
      expect(lstatSync(managedPath).nlink).toBe(1);
    }
    expect(
      lstatSync(managedFixturePath(fixture, snapshot, fixture.goEmptyDir))
        .mode & 0o7777,
    ).toBe(0o500);
    for (const { metadata } of collectTreeKinds(snapshot.go.root)) {
      expect(metadata.isSymbolicLink()).toBe(false);
      if (metadata.isDirectory()) expect(metadata.mode & 0o7777).toBe(0o500);
      if (metadata.isFile()) {
        expect([0o400, 0o500]).toContain(metadata.mode & 0o7777);
      }
    }
    expect(snapshot.go.sourceManifest.entries).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ path: "misc/empty", type: "directory" }),
        expect.objectContaining({
          path: "pkg/tool/linux_amd64/compile",
          executable: true,
        }),
        expect.objectContaining({ path: "pkg/include/textflag.h" }),
        expect.objectContaining({ path: "src/runtime/runtime.go" }),
      ]),
    );
    expect(snapshot.go.manifest.entries).toHaveLength(
      snapshot.go.expectedManifest.entries.length,
    );
    expect(() => assertManagedToolSnapshot(snapshot)).not.toThrow();
  });

  test("copies source hardlinks independently and leaves every managed file at nlink one", () => {
    const fixture = safeToolFixture();
    const alias = join(fixture.goSourceDir, "runtime_alias.go");
    linkSync(fixture.paths.source, alias);
    expect(lstatSync(fixture.paths.source).nlink).toBe(2);

    const snapshot = createManagedToolSnapshot({
      environment: fixture.environment,
      managedHome: fixture.managedHome,
    });
    const sourceEntry = snapshot.go.sourceManifest.entries.find(
      (entry) => entry.path === "src/runtime/runtime.go",
    );
    expect(sourceEntry.nlink).toBe("2");
    expect(sourceEntry.hardlinks).toEqual([
      "src/runtime/runtime.go",
      "src/runtime/runtime_alias.go",
    ]);
    const managedOriginal = managedFixturePath(
      fixture,
      snapshot,
      fixture.paths.source,
    );
    const managedAlias = managedFixturePath(fixture, snapshot, alias);
    expect(lstatSync(managedOriginal).nlink).toBe(1);
    expect(lstatSync(managedAlias).nlink).toBe(1);
    expect(lstatSync(managedOriginal).ino).not.toBe(
      lstatSync(managedAlias).ino,
    );
  });

  test("dereferences safe Debian-style external and absolute GOROOT links", () => {
    const fixture = safeToolFixture({ debianLayout: true });
    const sharedRoot = join(fixture.root, "usr", "share", "go-1.26");
    const sharedSource = join(sharedRoot, "src");
    const sharedInclude = join(sharedRoot, "pkg", "include");
    mkdirSync(dirname(sharedSource), { recursive: true, mode: 0o700 });
    mkdirSync(dirname(sharedInclude), { recursive: true, mode: 0o700 });
    renameSync(join(fixture.goRoot, "src"), sharedSource);
    renameSync(fixture.goIncludeDir, sharedInclude);
    symlinkSync("../../share/go-1.26/src", join(fixture.goRoot, "src"));
    symlinkSync("../../../share/go-1.26/pkg/include", fixture.goIncludeDir);
    const absoluteTarget = join(fixture.root, "absolute-misc");
    renameSync(join(fixture.goRoot, "misc"), absoluteTarget);
    symlinkSync(absoluteTarget, join(fixture.goRoot, "misc"));

    const snapshot = createManagedToolSnapshot({
      environment: fixture.environment,
      managedHome: fixture.managedHome,
    });
    expect(
      readFileSync(join(snapshot.go.root, "src/runtime/runtime.go")),
    ).toEqual(readFileSync(join(sharedSource, "runtime", "runtime.go")));
    expect(
      readFileSync(join(snapshot.go.root, "pkg/include/textflag.h")),
    ).toEqual(readFileSync(join(sharedInclude, "textflag.h")));
    expect(lstatSync(join(snapshot.go.root, "misc")).isDirectory()).toBe(true);
    expect(
      collectTreeKinds(snapshot.go.root).some(({ metadata }) =>
        metadata.isSymbolicLink(),
      ),
    ).toBe(false);
    expect(
      snapshot.go.sourceManifest.entries.find((entry) => entry.path === "src")
        .links[0].target,
    ).toBe("../../share/go-1.26/src");
    expect(
      snapshot.go.sourceManifest.entries.find((entry) => entry.path === "misc")
        .links[0].target,
    ).toBe(absoluteTarget);
  });

  test("fails when the source GOROOT changes during its create-only copy", () => {
    const cases = [
      [
        "added path",
        (fixture) =>
          writeFileSync(
            join(fixture.goSourceDir, "added.go"),
            "package runtime\n",
          ),
      ],
      ["deleted path", (fixture) => rmSync(fixture.paths.source)],
      ["file mode", (fixture) => chmodSync(fixture.paths.include, 0o500)],
      ["directory mode", (fixture) => chmodSync(fixture.goEmptyDir, 0o500)],
      [
        "compiler bytes",
        (fixture) => replaceReadOnlyFile(fixture.paths.compile, "changed\n"),
      ],
    ];
    if (typeof process.getuid === "function" && process.getuid() === 0) {
      cases.push([
        "owner",
        (fixture) => chownSync(fixture.paths.include, 65534, 65534),
      ]);
    }
    for (const [, mutate] of cases) {
      const fixture = safeToolFixture();
      let changed = false;
      expect(() =>
        createManagedToolSnapshot({
          environment: fixture.environment,
          managedHome: fixture.managedHome,
          testHooks: {
            afterCopyEntry: ({ logicalPath }) => {
              if (!changed && logicalPath === "src/runtime/runtime.go") {
                changed = true;
                mutate(fixture);
              }
            },
          },
        }),
      ).toThrow("owner gate source GOROOT");
    }
  });

  test("rejects unsafe, dangling, cyclic, set-id, and special source closure entries", () => {
    const cases = [
      [
        "writable external target",
        (fixture) => {
          const target = join(fixture.root, "unsafe-source");
          renameSync(join(fixture.goRoot, "src"), target);
          chmodSync(target, 0o770);
          symlinkSync(target, join(fixture.goRoot, "src"));
        },
        "must not be group/other-writable",
      ],
      [
        "dangling target",
        (fixture) => {
          rmSync(fixture.paths.source);
          symlinkSync("missing.go", fixture.paths.source);
        },
        "cannot be inspected",
      ],
      [
        "cyclic target",
        (fixture) => {
          rmSync(join(fixture.goRoot, "src"), { recursive: true });
          symlinkSync("src", join(fixture.goRoot, "src"));
        },
        "cyclic symbolic link",
      ],
      [
        "set-id file",
        (fixture) =>
          execFileSync("/usr/bin/chmod", ["4500", fixture.paths.compile]),
        "must not be setuid or setgid",
      ],
      [
        "group-writable file",
        (fixture) => chmodSync(fixture.paths.include, 0o420),
        "must not be group/other-writable",
      ],
      [
        "special file",
        (fixture) => {
          const fifo = join(fixture.root, "compiler.pipe");
          execFileSync("/usr/bin/mkfifo", [fifo]);
          rmSync(fixture.paths.source);
          symlinkSync(fifo, fixture.paths.source);
        },
        "must not contain a special file",
      ],
    ];
    if (typeof process.getuid === "function" && process.getuid() === 0) {
      cases.push([
        "foreign target owner",
        (fixture) => {
          const target = join(fixture.root, "foreign-source");
          renameSync(join(fixture.goRoot, "src"), target);
          chownSync(target, 65534, 65534);
          symlinkSync(target, join(fixture.goRoot, "src"));
        },
        "owned by root or the current user",
      ]);
    }
    for (const [, mutate, message] of cases) {
      const fixture = safeToolFixture();
      mutate(fixture);
      expect(() =>
        createManagedToolSnapshot({
          environment: fixture.environment,
          managedHome: fixture.managedHome,
        }),
      ).toThrow(message);
    }
  });

  test("recursively detects compiler, source, include, empty-directory, and ownership drift", () => {
    const mutations = [
      ({ fixture, snapshot }) =>
        replaceReadOnlyFile(
          managedFixturePath(fixture, snapshot, fixture.paths.compile),
          "changed compiler\n",
        ),
      ({ fixture, snapshot }) => {
        const source = managedFixturePath(
          fixture,
          snapshot,
          fixture.paths.source,
        );
        chmodSync(dirname(source), 0o700);
        rmSync(source);
        chmodSync(dirname(source), 0o500);
      },
      ({ fixture, snapshot }) => {
        const include = managedFixturePath(
          fixture,
          snapshot,
          fixture.paths.include,
        );
        chmodSync(dirname(include), 0o700);
        writeFileSync(join(dirname(include), "added.h"), "added\n");
        chmodSync(dirname(include), 0o500);
      },
      ({ fixture, snapshot }) =>
        chmodSync(
          managedFixturePath(fixture, snapshot, fixture.goEmptyDir),
          0o700,
        ),
    ];
    if (typeof process.getuid === "function" && process.getuid() === 0) {
      mutations.push(({ fixture, snapshot }) =>
        chownSync(
          managedFixturePath(fixture, snapshot, fixture.paths.include),
          65534,
          65534,
        ),
      );
    }
    for (const mutate of mutations) {
      const state = toolSnapshot();
      mutate(state);
      expect(() => assertManagedToolSnapshot(state.snapshot)).toThrow();
    }
  });

  test("follows a safe nominated symlink but snapshots only its resolved bytes", () => {
    const fixture = safeToolFixture();
    const target = join(fixture.terraformBin, "terraform.real");
    renameSync(fixture.paths.terraform, target);
    symlinkSync(target, fixture.paths.terraform);
    const snapshot = createManagedToolSnapshot({
      environment: fixture.environment,
      managedHome: fixture.managedHome,
    });
    expect(snapshot.tools.terraform.path).toBe(
      join(snapshot.toolBin, "terraform"),
    );
    expect(readFileSync(snapshot.tools.terraform.path)).toEqual(
      readFileSync(target),
    );
  });

  test("rejects empty and relative PATH entries before nomination", () => {
    for (const path of ["", "relative:/usr/bin", "/usr/bin:"]) {
      const fixture = safeToolFixture();
      expect(() =>
        createManagedToolSnapshot({
          environment: { PATH: path },
          managedHome: fixture.managedHome,
        }),
      ).toThrow(path === "" ? "non-empty PATH" : "PATH entries");
    }
  });

  test("rejects missing, symlink, nonregular, writable, and unsafe-ancestor candidates", () => {
    const cases = [
      [
        "missing",
        (fixture) => rmSync(fixture.paths.terraform),
        "terraform was not found",
      ],
      [
        "unsafe symlink target",
        (fixture) => {
          const unsafe = join(fixture.terraformBin, "unsafe");
          mkdirSync(unsafe, { mode: 0o770 });
          chmodSync(unsafe, 0o770);
          const target = join(unsafe, "terraform");
          writeFileSync(target, "#!/bin/sh\nexit 0\n");
          chmodSync(target, 0o500);
          rmSync(fixture.paths.terraform);
          symlinkSync(target, fixture.paths.terraform);
        },
        "unsafe writable ancestor",
      ],
      [
        "nonregular",
        (fixture) => {
          rmSync(fixture.paths.terraform);
          mkdirSync(fixture.paths.terraform, { mode: 0o700 });
        },
        "must be a regular file",
      ],
      [
        "writable candidate",
        (fixture) => chmodSync(fixture.paths.terraform, 0o720),
        "must not be group/other-writable",
      ],
      [
        "non-executable candidate",
        (fixture) => chmodSync(fixture.paths.terraform, 0o400),
        "is not executable",
      ],
      [
        "writable ancestor",
        (fixture) => chmodSync(fixture.terraformBin, 0o770),
        "unsafe writable ancestor",
      ],
    ];
    if (typeof process.getuid === "function" && process.getuid() === 0) {
      cases.push([
        "foreign owner",
        (fixture) => chownSync(fixture.paths.terraform, 65534, 65534),
        "owned by root or the current user",
      ]);
    }
    for (const [, mutate, message] of cases) {
      const fixture = safeToolFixture();
      mutate(fixture);
      expect(() =>
        createManagedToolSnapshot({
          environment: fixture.environment,
          managedHome: fixture.managedHome,
        }),
      ).toThrow(message);
    }
  });

  test("requires the exact executable Go bin closure", () => {
    const fixture = safeToolFixture();
    writeFileSync(join(fixture.goBin, "bun"), "#!/bin/sh\nexit 91\n");
    chmodSync(join(fixture.goBin, "bun"), 0o500);
    expect(() =>
      createManagedToolSnapshot({
        environment: fixture.environment,
        managedHome: fixture.managedHome,
      }),
    ).toThrow("Go bin executable closure changed");
  });

  test("detects copied bytes, Go bytes, and tool-bin closure changes after the gate", () => {
    for (const mutate of [
      ({ snapshot }) => {
        chmodSync(snapshot.tools.tofu.path, 0o700);
        writeFileSync(snapshot.tools.tofu.path, "changed\n");
        chmodSync(snapshot.tools.tofu.path, 0o500);
      },
      ({ snapshot }) => {
        chmodSync(snapshot.go.gofmt.path, 0o700);
        writeFileSync(snapshot.go.gofmt.path, "changed\n");
        chmodSync(snapshot.go.gofmt.path, 0o500);
      },
      ({ snapshot }) => writeFileSync(join(snapshot.toolBin, "extra"), "x"),
    ]) {
      const state = toolSnapshot();
      mutate(state);
      expect(() => assertManagedToolSnapshot(state.snapshot)).toThrow();
    }
  });
});

describe("owner gate private mutable state and cleanup", () => {
  test("creates fresh real cache, module, GOPATH, and temporary roots", () => {
    const fixture = safeToolFixture();
    const state = createManagedGateState(fixture.managedHome);
    expect(state).toEqual({
      root: join(fixture.managedHome, "m"),
      gocache: join(fixture.managedHome, "m", "go-build"),
      gomodcache: join(fixture.managedHome, "m", "go-mod"),
      gopath: join(fixture.managedHome, "m", "go-path"),
      tmpdir: join(fixture.managedHome, "m", "t"),
    });
    expect(() => assertManagedGateState(state)).not.toThrow();
    for (const path of Object.values(state)) {
      expect(lstatSync(path).isDirectory()).toBe(true);
      expect(lstatSync(path).isSymbolicLink()).toBe(false);
      expect(lstatSync(path).mode & 0o7777).toBe(0o700);
    }

    rmSync(state.gocache, { recursive: true });
    symlinkSync(state.gopath, state.gocache);
    expect(() => assertManagedGateState(state)).toThrow("real directory");
  });

  test("rejects preexisting managed GOROOT and mutable state create-only", () => {
    const gorootFixture = safeToolFixture();
    mkdirSync(join(gorootFixture.managedHome, "goroot"), { mode: 0o700 });
    writeFileSync(join(gorootFixture.managedHome, "goroot", "sentinel"), "x");
    expect(() =>
      createManagedToolSnapshot({
        environment: gorootFixture.environment,
        managedHome: gorootFixture.managedHome,
      }),
    ).toThrow("could not be materialized create-only");
    expect(
      readFileSync(
        join(gorootFixture.managedHome, "goroot", "sentinel"),
        "utf8",
      ),
    ).toBe("x");

    const stateFixture = safeToolFixture();
    mkdirSync(join(stateFixture.managedHome, "m"), { mode: 0o700 });
    expect(() => createManagedGateState(stateFixture.managedHome)).toThrow(
      "must be fresh and create-only",
    );
  });

  test("unseals owned directories only when cleanup begins", () => {
    const { fixture, snapshot } = toolSnapshot();
    const state = createManagedGateState(fixture.managedHome);
    const moduleDirectory = join(state.gomodcache, "example.invalid", "module");
    mkdirSync(moduleDirectory, { recursive: true, mode: 0o500 });
    chmodSync(dirname(moduleDirectory), 0o500);
    expect(lstatSync(snapshot.go.root).mode & 0o7777).toBe(0o500);
    expect(lstatSync(moduleDirectory).mode & 0o7777).toBe(0o500);

    prepareManagedHomeForRemoval(fixture.managedHome);
    expect(lstatSync(snapshot.go.root).mode & 0o7777).toBe(0o700);
    expect(lstatSync(moduleDirectory).mode & 0o7777).toBe(0o700);
    expect(() =>
      rmSync(fixture.managedHome, { recursive: true, force: true }),
    ).not.toThrow();
  });
});

test("repository Git configuration accepts only inert settings", () => {
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
      ordinary.replace(canonical, canonical.slice(0, -4)),
      canonical,
    ),
  ).not.toThrow();
  expect(() =>
    assertSafeRepositoryGitConfiguration(
      ordinary.replace(canonical, canonical.replace("https://", "http://")),
      canonical,
    ),
  ).toThrow("repository origin URL is not the canonical URL");
  expect(() =>
    assertSafeRepositoryGitConfiguration(
      ordinary.replace(
        "\0branch.main.remote",
        "\0gc.auto\n0\0branch.main.remote",
      ),
      canonical,
    ),
  ).not.toThrow();
  expect(() =>
    assertSafeRepositoryGitConfiguration(
      ordinary.replace(
        "\0branch.main.remote",
        "\0gc.auto\n1\0branch.main.remote",
      ),
      canonical,
    ),
  ).toThrow("gc.auto must be exactly 0");
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
      execFileSync("/bin/sh", ["-c", 'echo "zone $((400 + 3))" >&2; exit 7'], {
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
    const failure = Object.assign(new Error("boom"), {
      stdout: "",
      stderr: "boom\n",
    });
    expect(diagnostics(failure).match(/boom/g)).toHaveLength(1);
  });

  test("a step that produced nothing says so rather than printing a blank line", () => {
    expect(
      diagnostics(Object.assign(new Error(""), { stdout: "", stderr: "" })),
    ).toBe("no diagnostic was produced by the failing step\n");
  });

  test("a cause is not swallowed", () => {
    expect(
      diagnostics(new Error("fence failed", { cause: new Error("zone 403") })),
    ).toContain("zone 403");
  });
});

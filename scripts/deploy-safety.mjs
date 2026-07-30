import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

function sortPaths(left, right) {
  return left.path < right.path ? -1 : left.path > right.path ? 1 : 0;
}

function splitNullTerminated(raw) {
  if (raw.length === 0) return [];
  const values = raw.split("\0");
  if (values.at(-1) === "") values.pop();
  return values;
}

function publicationEntry(root, path) {
  const name = relative(root, path).split(sep).join("/");
  if (name === "" || name === ".." || name.startsWith("../")) {
    throw new Error(`published asset escaped the snapshot root: ${path}`);
  }
  const bytes = readFileSync(path);
  return {
    path: name,
    bytes: bytes.length,
    sha256: digest(bytes),
  };
}

function manifest(entries) {
  const sorted = [...entries].sort(sortPaths);
  return {
    entries: sorted,
    sha256: digest(Buffer.from(JSON.stringify(sorted))),
  };
}

export function collectRegularFiles(directory) {
  const found = [];
  for (const entry of readdirSync(directory).sort()) {
    const path = join(directory, entry);
    const metadata = lstatSync(path);
    if (metadata.isSymbolicLink()) {
      throw new Error(`published asset must not be a symbolic link: ${path}`);
    }
    if (metadata.isDirectory()) {
      found.push(...collectRegularFiles(path));
      continue;
    }
    if (!metadata.isFile()) {
      throw new Error(`published asset must be a regular file: ${path}`);
    }
    found.push(path);
  }
  return found;
}

export function createHardenedGitEnvironment(environment = process.env) {
  const unsafe = new Set([
    "ALL_PROXY",
    "BASH_ENV",
    "BUN_OPTIONS",
    "DYLD_INSERT_LIBRARIES",
    "DYLD_LIBRARY_PATH",
    "ENV",
    "HTTPS_PROXY",
    "HTTP_PROXY",
    "LD_LIBRARY_PATH",
    "LD_PRELOAD",
    "NODE_OPTIONS",
    "TAR_OPTIONS",
    "all_proxy",
    "https_proxy",
    "http_proxy",
  ]);
  const hardened = {};
  for (const [name, value] of Object.entries(environment)) {
    if (!name.startsWith("GIT_") && !unsafe.has(name)) hardened[name] = value;
  }
  return {
    ...hardened,
    GIT_CONFIG_GLOBAL: "/dev/null",
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_SYSTEM: "/dev/null",
    GIT_NO_REPLACE_OBJECTS: "1",
    GIT_OPTIONAL_LOCKS: "0",
    GIT_TERMINAL_PROMPT: "0",
  };
}

export function createHardenedGateEnvironment(
  environment,
  bunExecutable = process.execPath,
  managedHome = "/nonexistent/takoform-gate-home",
) {
  if (!bunExecutable.startsWith("/") || !managedHome.startsWith("/")) {
    throw new Error("gate Bun executable and HOME must be absolute paths");
  }
  const hardened = createHardenedGitEnvironment(environment);
  for (const name of Object.keys(hardened)) {
    if (
      name.startsWith("BUN_") ||
      name.startsWith("CF_") ||
      name.startsWith("CLOUDFLARE_") ||
      name.startsWith("NODE_") ||
      name.startsWith("NPM_CONFIG_") ||
      name.startsWith("TAKOFORM_CLOUDFLARE_") ||
      name.startsWith("WRANGLER_") ||
      name.startsWith("npm_config_")
    ) {
      delete hardened[name];
    }
  }
  return {
    ...hardened,
    HOME: managedHome,
    PATH: [
      dirname(bunExecutable),
      "/usr/local/go/bin",
      "/usr/local/bin",
      "/usr/bin",
      "/bin",
    ].join(":"),
    XDG_CACHE_HOME: join(managedHome, ".cache"),
    XDG_CONFIG_HOME: join(managedHome, ".config"),
    XDG_DATA_HOME: join(managedHome, ".local", "share"),
  };
}

export function assertSafeRepositoryGitConfiguration(raw, canonicalOrigin) {
  const values = new Map();
  for (const entry of splitNullTerminated(raw)) {
    const separator = entry.indexOf("\n");
    if (separator <= 0) {
      throw new Error("repository Git configuration has an invalid entry");
    }
    const name = entry.slice(0, separator);
    const value = entry.slice(separator + 1);
    const allowedCore = new Set([
      "core.bare",
      "core.filemode",
      "core.logallrefupdates",
      "core.repositoryformatversion",
    ]);
    const allowedBranch =
      /^branch\..+\.(?:merge|remote|vscode-merge-base)$/u.test(name);
    const allowedRemote =
      name === "remote.origin.fetch" || name === "remote.origin.url";
    if (!allowedCore.has(name) && !allowedBranch && !allowedRemote) {
      throw new Error(
        `repository Git configuration can influence publication: ${name}`,
      );
    }
    if (values.has(name)) {
      throw new Error(`repository Git configuration is duplicated: ${name}`);
    }
    values.set(name, value);
  }
  if (values.get("core.repositoryformatversion") !== "0") {
    throw new Error("repository Git format is not the expected format 0");
  }
  if (values.get("core.bare") !== "false") {
    throw new Error("publication requires a non-bare repository");
  }
  if (values.get("remote.origin.url") !== canonicalOrigin) {
    throw new Error("repository origin URL is not the canonical URL");
  }
  if (
    values.get("remote.origin.fetch") !==
    "+refs/heads/*:refs/remotes/origin/*"
  ) {
    throw new Error("repository origin fetch mapping is not canonical");
  }
}

function runGit({
  args,
  encoding,
  environment,
  gitExecutable,
  repo,
}) {
  return execFileSync(gitExecutable, args, {
    cwd: repo,
    encoding,
    env: environment,
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  });
}

export function inspectUncommittedPublicationPaths({
  environment = createHardenedGitEnvironment(),
  gitExecutable = "git",
  paths,
  repo,
}) {
  if (!Array.isArray(paths) || paths.length === 0) {
    throw new Error("at least one publication path is required");
  }
  const list = (args) =>
    splitNullTerminated(
      runGit({
        args: [...args, "--", ...paths],
        encoding: "utf8",
        environment,
        gitExecutable,
        repo,
      }),
    );
  return {
    ignored: list([
      "ls-files",
      "--others",
      "--ignored",
      "--exclude-standard",
      "-z",
    ]),
    untracked: list(["ls-files", "--others", "--exclude-standard", "-z"]),
  };
}

export function createCommittedSnapshot({
  commit,
  environment = createHardenedGitEnvironment(),
  gitExecutable = "git",
  repo,
  tarExecutable = "tar",
}) {
  if (!/^[0-9a-f]{40}$/u.test(commit)) {
    throw new Error(`snapshot commit is not a full Git object id: ${commit}`);
  }
  const temporaryRoot = mkdtempSync(
    join(tmpdir(), "takoform-publication-snapshot-"),
  );
  chmodSync(temporaryRoot, 0o700);
  const root = join(temporaryRoot, "source");
  mkdirSync(root, { mode: 0o700 });
  try {
    const archive = runGit({
      args: ["archive", "--format=tar", commit],
      environment,
      gitExecutable,
      repo,
    });
    execFileSync(
      tarExecutable,
      ["--extract", "--file=-", "--directory", root, "--no-same-owner"],
      {
        encoding: "buffer",
        env: environment,
        input: archive,
        maxBuffer: 64 * 1024 * 1024,
        stdio: ["pipe", "pipe", "pipe"],
      },
    );
  } catch (error) {
    rmSync(temporaryRoot, { force: true, recursive: true });
    throw error;
  }
  let disposed = false;
  return {
    root,
    dispose() {
      if (disposed) return;
      disposed = true;
      rmSync(temporaryRoot, { force: true, recursive: true });
    },
  };
}

export function createPinnedWranglerInstallation({
  bunExecutable,
  environment,
  snapshotRoot,
}) {
  const temporaryRoot = mkdtempSync(
    join(tmpdir(), "takoform-pinned-wrangler-"),
  );
  chmodSync(temporaryRoot, 0o700);
  const cleanedEnvironment = {};
  for (const [name, value] of Object.entries(environment)) {
    if (
      !name.startsWith("BUN_") &&
      !name.startsWith("NPM_CONFIG_") &&
      !name.startsWith("npm_config_")
    ) {
      cleanedEnvironment[name] = value;
    }
  }
  const installEnvironment = {
    ...cleanedEnvironment,
    BUN_INSTALL_CACHE_DIR: join(temporaryRoot, "cache"),
    HOME: temporaryRoot,
  };
  try {
    copyFileSync(join(snapshotRoot, "package.json"), join(temporaryRoot, "package.json"));
    copyFileSync(join(snapshotRoot, "bun.lock"), join(temporaryRoot, "bun.lock"));
    execFileSync(
      bunExecutable,
      ["install", "--frozen-lockfile", "--ignore-scripts"],
      {
        cwd: temporaryRoot,
        encoding: "utf8",
        env: installEnvironment,
        maxBuffer: 64 * 1024 * 1024,
        stdio: ["ignore", "pipe", "pipe"],
      },
    );
    const script = join(
      temporaryRoot,
      "node_modules",
      "wrangler",
      "bin",
      "wrangler.js",
    );
    lstatSync(script);
    let disposed = false;
    return {
      environment: installEnvironment,
      script,
      dispose() {
        if (disposed) return;
        disposed = true;
        rmSync(temporaryRoot, { force: true, recursive: true });
      },
    };
  } catch (error) {
    rmSync(temporaryRoot, { force: true, recursive: true });
    throw error;
  }
}

export function pinnedWranglerInvocation(
  script,
  args,
  bunExecutable = process.execPath,
) {
  if (!bunExecutable.startsWith("/") || !script.startsWith("/")) {
    throw new Error("pinned Wrangler invocation requires absolute paths");
  }
  return {
    args: [
      "--config=/dev/null",
      "--no-env-file",
      script,
      ...args,
    ],
    command: bunExecutable,
  };
}

export function createPublicationManifest(root, paths) {
  const absoluteRoot = resolve(root);
  const files = [];
  for (const name of paths) {
    const path = resolve(absoluteRoot, name);
    const metadata = lstatSync(path);
    if (metadata.isSymbolicLink()) {
      throw new Error(`published asset must not be a symbolic link: ${path}`);
    }
    if (metadata.isDirectory()) {
      files.push(...collectRegularFiles(path));
      continue;
    }
    if (!metadata.isFile()) {
      throw new Error(`published asset must be a regular file: ${path}`);
    }
    files.push(path);
  }
  return manifest(files.map((path) => publicationEntry(absoluteRoot, path)));
}

export function createCommittedPublicationManifest({
  commit,
  environment = createHardenedGitEnvironment(),
  gitExecutable = "git",
  paths,
  repo,
}) {
  const raw = runGit({
    args: [
      "ls-tree",
      "--full-tree",
      "-r",
      "-z",
      commit,
      "--",
      ...paths,
    ],
    encoding: "utf8",
    environment,
    gitExecutable,
    repo,
  });
  const entries = splitNullTerminated(raw).map((entry) => {
    const match =
      /^(100644|100755) blob ([0-9a-f]{40})\t([^\0]+)$/u.exec(entry);
    if (!match) {
      throw new Error(
        `publication path is not a committed regular blob: ${JSON.stringify(entry)}`,
      );
    }
    const [, , object, path] = match;
    const bytes = runGit({
      args: ["cat-file", "blob", object],
      environment,
      gitExecutable,
      repo,
    });
    return {
      path,
      bytes: bytes.length,
      sha256: digest(bytes),
    };
  });
  if (entries.length === 0) {
    throw new Error("the committed publication set is empty");
  }
  return manifest(entries);
}

export function assertPublicationManifest(expected, observed) {
  if (expected.sha256 === observed.sha256) return;
  const expectedByPath = new Map(
    expected.entries.map((entry) => [entry.path, entry]),
  );
  const observedByPath = new Map(
    observed.entries.map((entry) => [entry.path, entry]),
  );
  const changed = [...new Set([...expectedByPath.keys(), ...observedByPath.keys()])]
    .sort()
    .filter(
      (path) =>
        JSON.stringify(expectedByPath.get(path)) !==
        JSON.stringify(observedByPath.get(path)),
    )
    .slice(0, 20);
  throw new Error(
    `publication snapshot changed after verification (${changed.join(", ") || "manifest digest mismatch"})`,
  );
}

export function parseCurrentProductionDeployment(raw) {
  let deployment;
  try {
    deployment = JSON.parse(raw);
  } catch (error) {
    throw new Error(`current deployment status was not JSON: ${error.message}`);
  }

  if (
    deployment === null ||
    typeof deployment !== "object" ||
    !UUID.test(deployment.id ?? "")
  ) {
    throw new Error("current deployment status has no valid deployment id");
  }
  if (
    deployment.strategy !== "percentage" ||
    !Array.isArray(deployment.versions) ||
    deployment.versions.length !== 1
  ) {
    throw new Error(
      "current deployment is ambiguous or has split traffic; a single 100% version is required",
    );
  }

  const [{ percentage, version_id: versionId }] = deployment.versions;
  if (percentage !== 100 || !UUID.test(versionId ?? "")) {
    throw new Error(
      "current deployment is ambiguous or has split traffic; a single 100% version is required",
    );
  }
  return {
    deploymentId: deployment.id,
    versionId,
  };
}

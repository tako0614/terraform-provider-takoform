import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  copyFileSync,
  existsSync,
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

export // diagnostics renders what actually went wrong, whatever threw.
//
// The failure-handling obligation this entrypoint declares is raw diagnostics
// and no blind retry, and the two are one requirement: an operator told only
// that a step is indeterminate, with nothing said about why, has been left with
// retrying as the only move available. A subprocess failure carries its output
// on stdout/stderr, but a fence, a precondition read, or any ordinary assertion
// throws an Error whose message is the whole diagnosis — printing only the two
// subprocess members discards it and prints a blank line instead.
function diagnostics(error) {
  const parts = [];
  const stdout = String(error?.stdout ?? "");
  const stderr = String(error?.stderr ?? "");
  if (stdout.trim().length > 0) parts.push(stdout.replace(/\n*$/, "\n"));
  if (stderr.trim().length > 0) parts.push(stderr.replace(/\n*$/, "\n"));
  const message = error instanceof Error ? error.message : String(error);
  // A subprocess whose own output already carries the diagnosis does not need
  // the wrapper's restatement of it.
  if (message.trim().length > 0 && !stdout.includes(message) && !stderr.includes(message)) {
    parts.push(`${message}\n`);
  }
  if (error instanceof Error && error.cause !== undefined) {
    parts.push(`caused by: ${error.cause instanceof Error ? error.cause.message : String(error.cause)}\n`);
  }
  return parts.length > 0 ? parts.join("") : "no diagnostic was produced by the failing step\n";
}

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
      name.startsWith("CGO_") ||
      name.startsWith("CLOUDFLARE_") ||
      name.startsWith("GO") ||
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
    CGO_ENABLED: "0",
    GOAUTH: "off",
    GOCACHE: join(managedHome, "go-build"),
    GOENV: "off",
    GOFLAGS: "-mod=readonly -buildvcs=false",
    GOMODCACHE: join(managedHome, "go", "pkg", "mod"),
    GONOPROXY: "",
    GONOSUMDB: "",
    GOPATH: join(managedHome, "go"),
    GOPRIVATE: "",
    GOPROXY: "https://proxy.golang.org",
    GOSUMDB: "sum.golang.org",
    GOTOOLCHAIN: "local",
    GOVCS: "*:off",
    GOWORK: "off",
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
  const authorityRoot = join(temporaryRoot, "git-authority");
  const emptyTemplateRoot = join(temporaryRoot, "empty-git-template");
  mkdirSync(root, { mode: 0o700 });
  mkdirSync(emptyTemplateRoot, { mode: 0o700 });
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
    runGit({
      args: [
        "-c",
        "core.hooksPath=/dev/null",
        "clone",
        "--no-local",
        "--no-checkout",
        "--quiet",
        "--template",
        emptyTemplateRoot,
        "--",
        repo,
        authorityRoot,
      ],
      environment,
      gitExecutable,
      repo: "/",
    });
    runGit({
      args: [
        "-c",
        "core.hooksPath=/dev/null",
        "checkout",
        "--detach",
        "--force",
        commit,
      ],
      environment,
      gitExecutable,
      repo: authorityRoot,
    });
    runGit({
      args: ["remote", "remove", "origin"],
      environment,
      gitExecutable,
      repo: authorityRoot,
    });
    assertCommittedGitAuthority({
      authorityRoot,
      commit,
      environment,
      gitExecutable,
    });
  } catch (error) {
    rmSync(temporaryRoot, { force: true, recursive: true });
    throw error;
  }
  let disposed = false;
  return {
    authorityRoot,
    root,
    dispose() {
      if (disposed) return;
      disposed = true;
      rmSync(temporaryRoot, { force: true, recursive: true });
    },
  };
}

export function assertCommittedGitAuthority({
  authorityRoot,
  commit,
  environment = createHardenedGitEnvironment(),
  gitExecutable = "git",
}) {
  if (!/^[0-9a-f]{40}$/u.test(commit)) {
    throw new Error(`Git authority commit is not a full object id: ${commit}`);
  }
  const text = (args) =>
    runGit({
      args,
      encoding: "utf8",
      environment,
      gitExecutable,
      repo: authorityRoot,
    }).trim();
  const raw = (args) =>
    runGit({
      args,
      encoding: "utf8",
      environment,
      gitExecutable,
      repo: authorityRoot,
    });
  const directories = text([
    "rev-parse",
    "--path-format=absolute",
    "--git-dir",
    "--git-common-dir",
    "--show-toplevel",
  ]).split("\n");
  const expectedGitDirectory = join(resolve(authorityRoot), ".git");
  if (
    directories.length !== 3 ||
    directories[0] !== expectedGitDirectory ||
    directories[1] !== expectedGitDirectory ||
    directories[2] !== resolve(authorityRoot)
  ) {
    throw new Error("Git authority clone directories are not isolated");
  }
  const absent = (path) => {
    try {
      lstatSync(path);
      return false;
    } catch (error) {
      if (error?.code === "ENOENT") return true;
      throw error;
    }
  };
  if (
    !absent(join(expectedGitDirectory, "objects", "info", "alternates")) ||
    !absent(join(expectedGitDirectory, "info", "grafts")) ||
    !absent(join(expectedGitDirectory, "shallow")) ||
    readdirSync(join(expectedGitDirectory, "objects", "pack")).some((name) =>
      name.endsWith(".promisor"),
    )
  ) {
    throw new Error(
      "Git authority clone has external, partial, shallow, or graft authority",
    );
  }
  const configEntries = splitNullTerminated(
    raw(["config", "--local", "-z", "--list"]),
  );
  const allowedConfig = new Set([
    "core.bare",
    "core.filemode",
    "core.logallrefupdates",
    "core.repositoryformatversion",
  ]);
  const config = new Map();
  for (const entry of configEntries) {
    const separator = entry.indexOf("\n");
    const name = separator < 0 ? entry : entry.slice(0, separator);
    const value = separator < 0 ? "" : entry.slice(separator + 1);
    if (!allowedConfig.has(name) || config.has(name)) {
      throw new Error(`Git authority clone has unexpected config ${name}`);
    }
    config.set(name, value);
  }
  if (
    config.get("core.repositoryformatversion") !== "0" ||
    config.get("core.bare") !== "false" ||
    config.get("core.logallrefupdates") !== "true" ||
    !new Set(["true", "false"]).has(config.get("core.filemode"))
  ) {
    throw new Error("Git authority clone core config is not exact");
  }
  if (
    text(["rev-parse", "HEAD"]) !== commit ||
    text(["rev-parse", "--abbrev-ref", "HEAD"]) !== "HEAD" ||
    text(["rev-parse", "--is-shallow-repository"]) !== "false" ||
    text(["rev-parse", "--show-object-format"]) !== "sha1" ||
    text([
      "status",
      "--porcelain=v1",
      "--untracked-files=all",
      "--ignored=matching",
    ]) !== ""
  ) {
    throw new Error("Git authority clone is not the exact clean detached commit");
  }
  raw(["fsck", "--strict", "--connectivity-only"]);
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
  nodeExecutable = "/usr/bin/node",
) {
  if (!nodeExecutable.startsWith("/") || !script.startsWith("/")) {
    throw new Error(
      "pinned Wrangler invocation requires absolute Node and script paths",
    );
  }
  return {
    args: [script, ...args],
    command: nodeExecutable,
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

export function createPublicationManifestFromEntries(root, entries) {
  if (!Array.isArray(entries) || entries.length === 0) {
    throw new Error("publication manifest entries must be non-empty");
  }
  const absoluteRoot = resolve(root);
  const seen = new Set();
  const files = entries.map((entry) => {
    if (
      entry === null ||
      typeof entry !== "object" ||
      typeof entry.path !== "string" ||
      entry.path === "" ||
      seen.has(entry.path)
    ) {
      throw new Error("publication manifest has an invalid or duplicate path");
    }
    seen.add(entry.path);
    const path = resolve(absoluteRoot, entry.path);
    const name = relative(absoluteRoot, path).split(sep).join("/");
    if (name !== entry.path) {
      throw new Error(`publication manifest path escaped its root: ${entry.path}`);
    }
    const metadata = lstatSync(path);
    if (metadata.isSymbolicLink() || !metadata.isFile()) {
      throw new Error(
        `publication manifest path is not a regular file: ${entry.path}`,
      );
    }
    return path;
  });
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

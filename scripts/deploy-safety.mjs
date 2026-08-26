import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import {
  chmodSync,
  closeSync,
  copyFileSync,
  constants as fsConstants,
  existsSync,
  fstatSync,
  lstatSync,
  mkdirSync,
  mkdtempSync,
  openSync,
  readFileSync,
  readSync,
  readlinkSync,
  readdirSync,
  realpathSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import {
  delimiter,
  dirname,
  isAbsolute,
  join,
  relative,
  resolve,
  sep,
} from "node:path";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

function digestFile(path) {
  let descriptor;
  try {
    descriptor = openSync(
      path,
      fsConstants.O_RDONLY | (fsConstants.O_NOFOLLOW ?? 0),
    );
    const hash = createHash("sha256");
    const buffer = Buffer.allocUnsafe(1024 * 1024);
    while (true) {
      const bytesRead = readSync(descriptor, buffer, 0, buffer.length, null);
      if (bytesRead === 0) break;
      hash.update(buffer.subarray(0, bytesRead));
    }
    return hash.digest("hex");
  } finally {
    if (descriptor !== undefined) closeSync(descriptor);
  }
}

// diagnostics renders what actually went wrong, whatever threw.
//
// The failure-handling obligation this entrypoint declares is raw diagnostics
// and no blind retry, and the two are one requirement: an operator told only
// that a step is indeterminate, with nothing said about why, has been left with
// retrying as the only move available. A subprocess failure carries its output
// on stdout/stderr, but a fence, a precondition read, or any ordinary assertion
// throws an Error whose message is the whole diagnosis — printing only the two
// subprocess members discards it and prints a blank line instead.
export function diagnostics(error) {
  const message = error instanceof Error ? error.message : String(error);
  const captured = [
    String(error?.stdout ?? ""),
    String(error?.stderr ?? ""),
  ].filter((stream) => stream.trim().length > 0);
  const parts = [];
  // execFileSync builds its message from the command AND the captured stderr,
  // so the message is usually the superset: printing both would show the same
  // stderr twice, once bare and once quoted inside "Command failed: ...".
  // Whichever text contains the other is the one worth printing.
  if (
    message.trim().length > 0 &&
    captured.every((stream) => message.includes(stream.trim()))
  ) {
    parts.push(`${message.replace(/\n*$/, "")}\n`);
  } else {
    parts.push(...captured.map((stream) => stream.replace(/\n*$/, "\n")));
    if (
      message.trim().length > 0 &&
      !captured.some((stream) => stream.includes(message))
    ) {
      parts.push(`${message}\n`);
    }
  }
  if (error instanceof Error && error.cause !== undefined) {
    parts.push(
      `caused by: ${error.cause instanceof Error ? error.cause.message : String(error.cause)}\n`,
    );
  }
  return parts.length > 0
    ? parts.join("")
    : "no diagnostic was produced by the failing step\n";
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

const COPIED_GATE_TOOLS = Object.freeze(["tofu", "terraform"]);
const GO_BINARIES = Object.freeze(["go", "gofmt"]);
const MANAGED_GOROOT_DIRECTORY_MODE = 0o500;
const MANAGED_GOROOT_EXECUTABLE_MODE = 0o500;
const MANAGED_GOROOT_FILE_MODE = 0o400;

function currentUserId() {
  return typeof process.getuid === "function" ? process.getuid() : undefined;
}

function currentGroupId() {
  return typeof process.getgid === "function" ? process.getgid() : undefined;
}

function pathMetadata(path, label) {
  try {
    return lstatSync(path);
  } catch (error) {
    throw new Error(
      `${label} cannot be inspected: ${path} (${error instanceof Error ? error.message : String(error)})`,
    );
  }
}

function assertTrustedOwner(metadata, path, label) {
  const uid = currentUserId();
  const owner = Number(metadata.uid);
  if (owner !== 0 && (uid === undefined || owner !== uid)) {
    throw new Error(
      `${label} must be owned by root or the current user: ${path}`,
    );
  }
}

function assertCurrentOwner(metadata, path, label) {
  const uid = currentUserId();
  const gid = currentGroupId();
  if (uid !== undefined && Number(metadata.uid) !== uid) {
    throw new Error(`${label} must be owned by the current user: ${path}`);
  }
  if (gid !== undefined && Number(metadata.gid) !== gid) {
    throw new Error(`${label} must use the current user's group: ${path}`);
  }
}

function assertSafeDirectory(path, label, { exactMode } = {}) {
  if (!isAbsolute(path)) {
    throw new Error(`${label} must be absolute: ${path}`);
  }
  const metadata = pathMetadata(path, label);
  if (metadata.isSymbolicLink() || !metadata.isDirectory()) {
    throw new Error(`${label} must be a real directory: ${path}`);
  }
  assertTrustedOwner(metadata, path, label);
  const mode = metadata.mode & 0o7777;
  if (exactMode === undefined ? (mode & 0o022) !== 0 : mode !== exactMode) {
    if (exactMode === undefined) {
      throw new Error(`${label} must not be group/other-writable: ${path}`);
    }
    throw new Error(
      `${label} must have mode ${exactMode.toString(8).padStart(4, "0")}: ${path}`,
    );
  }
  return metadata;
}

function assertSafeAncestors(path, label) {
  const starts = new Set([dirname(path), dirname(realpathSync(path))]);
  for (const start of starts) {
    let current = start;
    while (true) {
      const metadata = pathMetadata(current, `${label} ancestor`);
      assertTrustedOwner(metadata, current, `${label} ancestor`);
      if (metadata.isSymbolicLink()) {
        assertSafeDirectory(
          realpathSync(current),
          `${label} resolved ancestor`,
        );
      } else if (!metadata.isDirectory()) {
        throw new Error(`${label} ancestor is not a directory: ${current}`);
      } else if ((metadata.mode & 0o022) !== 0) {
        throw new Error(`${label} has an unsafe writable ancestor: ${current}`);
      }
      if (current === sep) break;
      current = dirname(current);
    }
  }
}

function inspectSafeExecutable(path, label, { checkAncestors = true } = {}) {
  if (!isAbsolute(path)) {
    throw new Error(`${label} must be absolute: ${path}`);
  }
  const candidate = pathMetadata(path, label);
  const nominatedBySymlink = candidate.isSymbolicLink();
  if (!nominatedBySymlink && !candidate.isFile()) {
    throw new Error(`${label} must be a regular file: ${path}`);
  }
  assertTrustedOwner(candidate, path, label);
  if (!nominatedBySymlink) {
    if ((candidate.mode & 0o111) === 0) {
      throw new Error(`${label} is not executable: ${path}`);
    }
    if ((candidate.mode & 0o022) !== 0) {
      throw new Error(`${label} must not be group/other-writable: ${path}`);
    }
  }
  if (checkAncestors) assertSafeAncestors(path, label);

  const resolvedPath = realpathSync(path);
  if (!isAbsolute(resolvedPath)) {
    throw new Error(`${label} resolved path must be absolute: ${resolvedPath}`);
  }
  const resolved = pathMetadata(resolvedPath, `${label} resolved path`);
  if (resolved.isSymbolicLink() || !resolved.isFile()) {
    throw new Error(
      `${label} resolved path must be a regular file: ${resolvedPath}`,
    );
  }
  assertTrustedOwner(resolved, resolvedPath, `${label} resolved path`);
  if ((resolved.mode & 0o111) === 0) {
    throw new Error(
      `${label} resolved path is not executable: ${resolvedPath}`,
    );
  }
  if ((resolved.mode & 0o022) !== 0) {
    throw new Error(
      `${label} resolved path must not be group/other-writable: ${resolvedPath}`,
    );
  }
  if (checkAncestors) {
    assertSafeAncestors(resolvedPath, `${label} resolved path`);
  }
  return {
    path: resolvedPath,
    mode: resolved.mode & 0o7777,
    sha256: digestFile(resolvedPath),
  };
}

function bigintMetadata(path, label) {
  try {
    return lstatSync(path, { bigint: true });
  } catch (error) {
    throw new Error(
      `${label} cannot be inspected: ${path} (${error instanceof Error ? error.message : String(error)})`,
    );
  }
}

function sourceMetadata(metadata) {
  return {
    mode: Number(metadata.mode & 0o7777n),
    uid: Number(metadata.uid),
    gid: Number(metadata.gid),
    dev: metadata.dev.toString(),
    ino: metadata.ino.toString(),
    nlink: metadata.nlink.toString(),
  };
}

function assertSafeSourceMetadata(metadata, path, label) {
  assertTrustedOwner(metadata, path, label);
  const mode = Number(metadata.mode & 0o7777n);
  if (metadata.isDirectory()) {
    if ((mode & 0o022) !== 0) {
      throw new Error(`${label} must not be group/other-writable: ${path}`);
    }
    return "directory";
  }
  if (metadata.isFile()) {
    if ((mode & 0o022) !== 0) {
      throw new Error(`${label} must not be group/other-writable: ${path}`);
    }
    if ((mode & 0o6000) !== 0) {
      throw new Error(`${label} must not be setuid or setgid: ${path}`);
    }
    return "file";
  }
  throw new Error(`${label} must not contain a special file: ${path}`);
}

// Resolve one absolute path using the kernel's component-by-component symlink
// semantics without executing anything from the nominated toolchain.  Doing
// this ourselves, instead of accepting realpathSync as the whole proof, keeps
// the raw link topology in the source manifest and validates every physical
// directory crossed by relative and absolute links.
function resolveSafeSourceNode(path, label) {
  if (!isAbsolute(path)) throw new Error(`${label} must be absolute: ${path}`);
  const physicalRoot = bigintMetadata(sep, `${label} physical root`);
  if (!physicalRoot.isDirectory()) {
    throw new Error(`${label} physical root is not a directory: ${sep}`);
  }
  assertSafeSourceMetadata(physicalRoot, sep, `${label} physical root`);
  let current = sep;
  let pending = path.slice(sep.length).split(sep);
  const links = [];
  const visitedLinks = new Set();
  let linkCount = 0;

  while (pending.length > 0) {
    const component = pending.shift();
    if (component === "" || component === ".") continue;
    if (component === "..") {
      current = dirname(current);
      continue;
    }
    const candidate = join(current, component);
    const metadata = bigintMetadata(candidate, label);
    if (metadata.isSymbolicLink()) {
      assertTrustedOwner(metadata, candidate, `${label} symbolic link`);
      const identity = `${metadata.dev}:${metadata.ino}`;
      if (visitedLinks.has(identity) || linkCount >= 128) {
        throw new Error(
          `${label} contains a cyclic symbolic link: ${candidate}`,
        );
      }
      visitedLinks.add(identity);
      linkCount += 1;
      const target = readlinkSync(candidate);
      links.push({
        path: candidate,
        target,
        ...sourceMetadata(metadata),
      });
      pending = [...target.split(sep), ...pending];
      current = isAbsolute(target) ? sep : dirname(candidate);
      continue;
    }
    if (pending.length > 0) {
      if (!metadata.isDirectory()) {
        throw new Error(
          `${label} physical ancestor is not a directory: ${candidate}`,
        );
      }
      assertSafeSourceMetadata(
        metadata,
        candidate,
        `${label} physical ancestor`,
      );
    }
    current = candidate;
  }

  const metadata = bigintMetadata(current, label);
  return { links, metadata, path: current };
}

function readStableSourceFile(path, initial, label) {
  let descriptor;
  try {
    descriptor = openSync(
      path,
      fsConstants.O_RDONLY | (fsConstants.O_NOFOLLOW ?? 0),
    );
    const opened = fstatSync(descriptor, { bigint: true });
    const beforeIdentity = JSON.stringify(sourceMetadata(initial));
    if (
      !opened.isFile() ||
      beforeIdentity !== JSON.stringify(sourceMetadata(opened))
    ) {
      throw new Error(`${label} changed before it was read: ${path}`);
    }
    const hash = createHash("sha256");
    const buffer = Buffer.allocUnsafe(1024 * 1024);
    let bytes = 0;
    while (true) {
      const bytesRead = readSync(descriptor, buffer, 0, buffer.length, null);
      if (bytesRead === 0) break;
      bytes += bytesRead;
      hash.update(buffer.subarray(0, bytesRead));
    }
    const after = fstatSync(descriptor, { bigint: true });
    if (
      !after.isFile() ||
      beforeIdentity !== JSON.stringify(sourceMetadata(after)) ||
      after.size !== BigInt(bytes) ||
      initial.size !== after.size
    ) {
      throw new Error(`${label} changed while it was inspected: ${path}`);
    }
    return { bytes, sha256: hash.digest("hex") };
  } finally {
    if (descriptor !== undefined) closeSync(descriptor);
  }
}

function canonicalManifest(entries) {
  const sorted = [...entries].sort(sortPaths);
  return {
    entries: sorted,
    sha256: digest(Buffer.from(JSON.stringify(sorted))),
  };
}

function addHardlinkTopology(entries) {
  const groups = new Map();
  for (const entry of entries) {
    if (entry.type !== "file" || entry.nlink === "1") continue;
    const identity = `${entry.dev}:${entry.ino}`;
    const paths = groups.get(identity) ?? [];
    paths.push(entry.path);
    groups.set(identity, paths);
  }
  return entries.map((entry) => ({
    ...entry,
    ...(entry.type === "file"
      ? {
          hardlinks:
            entry.nlink === "1"
              ? []
              : [...(groups.get(`${entry.dev}:${entry.ino}`) ?? [])].sort(),
        }
      : {}),
  }));
}

function createSourceGoRootManifest(sourceRoot) {
  const entries = [];
  const copySources = new Map();

  function visit(logicalPath, physicalPath, physicalAncestors) {
    const label = `owner gate source GOROOT entry ${logicalPath}`;
    const resolved = resolveSafeSourceNode(physicalPath, label);
    const type = assertSafeSourceMetadata(
      resolved.metadata,
      resolved.path,
      label,
    );
    const metadata = sourceMetadata(resolved.metadata);
    const base = {
      path: logicalPath,
      type,
      resolvedPath: resolved.path,
      ...metadata,
      links: resolved.links,
    };
    if (type === "file") {
      const content = readStableSourceFile(
        resolved.path,
        resolved.metadata,
        label,
      );
      entries.push({
        ...base,
        ...content,
        executable: (metadata.mode & 0o111) !== 0,
      });
      copySources.set(logicalPath, resolved.path);
      return;
    }

    const identity = `${metadata.dev}:${metadata.ino}`;
    if (physicalAncestors.has(identity)) {
      throw new Error(
        `owner gate source GOROOT contains a cyclic directory link at ${logicalPath}`,
      );
    }
    entries.push(base);
    const nextAncestors = new Set(physicalAncestors);
    nextAncestors.add(identity);
    let children;
    try {
      children = readdirSync(resolved.path).sort();
    } catch (error) {
      throw new Error(
        `${label} cannot be enumerated: ${resolved.path} (${error instanceof Error ? error.message : String(error)})`,
      );
    }
    for (const child of children) {
      visit(
        logicalPath === "." ? child : `${logicalPath}/${child}`,
        join(resolved.path, child),
        nextAncestors,
      );
    }
  }

  const root = resolveSafeSourceNode(sourceRoot, "owner gate source GOROOT");
  if (root.path !== sourceRoot || root.links.length !== 0) {
    throw new Error(
      `owner gate source GOROOT must be a physical directory: ${sourceRoot}`,
    );
  }
  visit(".", sourceRoot, new Set());
  return {
    manifest: canonicalManifest(addHardlinkTopology(entries)),
    copySources,
  };
}

function normalizedGoRootEntries(sourceManifest) {
  return sourceManifest.entries.map((entry) => ({
    path: entry.path,
    type: entry.type,
    mode:
      entry.type === "directory"
        ? MANAGED_GOROOT_DIRECTORY_MODE
        : entry.executable
          ? MANAGED_GOROOT_EXECUTABLE_MODE
          : MANAGED_GOROOT_FILE_MODE,
    ...(entry.type === "file"
      ? {
          bytes: entry.bytes,
          sha256: entry.sha256,
          executable: entry.executable,
        }
      : {}),
  }));
}

function exactManifestDifference(before, after) {
  const beforeByPath = new Map(
    before.entries.map((entry) => [entry.path, entry]),
  );
  const afterByPath = new Map(
    after.entries.map((entry) => [entry.path, entry]),
  );
  for (const path of [
    ...new Set([...beforeByPath.keys(), ...afterByPath.keys()]),
  ].sort()) {
    if (
      JSON.stringify(beforeByPath.get(path)) !==
      JSON.stringify(afterByPath.get(path))
    ) {
      return path;
    }
  }
  return undefined;
}

function assertExactManifest(before, after, label) {
  if (
    before.sha256 !== after.sha256 ||
    JSON.stringify(before.entries) !== JSON.stringify(after.entries)
  ) {
    const path = exactManifestDifference(before, after);
    throw new Error(`${label} changed${path ? ` at ${path}` : ""}`);
  }
}

function managedPath(root, logicalPath) {
  if (logicalPath === ".") return root;
  const path = join(root, ...logicalPath.split("/"));
  const name = relative(root, path).split(sep).join("/");
  if (name !== logicalPath || name.startsWith("../")) {
    throw new Error(
      `owner gate managed GOROOT path escaped its root: ${logicalPath}`,
    );
  }
  return path;
}

function materializeManagedGoRoot({
  copySources,
  expectedEntries,
  managedRoot,
  sourceRoot,
  testHooks,
}) {
  const directories = expectedEntries
    .filter((entry) => entry.type === "directory")
    .sort((left, right) => {
      const depth = (entry) => entry.path.split("/").length;
      return depth(left) - depth(right) || sortPaths(left, right);
    });
  const files = expectedEntries.filter((entry) => entry.type === "file");
  try {
    for (const entry of directories) {
      const destination = managedPath(managedRoot, entry.path);
      mkdirSync(destination, { mode: 0o700 });
      testHooks?.afterCopyEntry?.({
        logicalPath: entry.path,
        managedRoot,
        sourceRoot,
      });
    }
    for (const entry of files) {
      const source = copySources.get(entry.path);
      const destination = managedPath(managedRoot, entry.path);
      if (typeof source !== "string") {
        throw new Error(
          `owner gate source GOROOT copy source is missing: ${entry.path}`,
        );
      }
      copyFileSync(source, destination, fsConstants.COPYFILE_EXCL);
      chmodSync(destination, entry.mode);
      testHooks?.afterCopyEntry?.({
        logicalPath: entry.path,
        managedRoot,
        sourceRoot,
      });
    }
    for (const entry of [...directories].reverse()) {
      chmodSync(managedPath(managedRoot, entry.path), entry.mode);
    }
  } catch (error) {
    throw new Error(
      `owner gate managed GOROOT could not be materialized create-only: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
}

function collectManagedGoRootManifest(managedRoot, expectedEntries) {
  const expected = new Map(expectedEntries.map((entry) => [entry.path, entry]));
  const entries = [];

  function visit(logicalPath, path) {
    const label = `owner gate managed GOROOT entry ${logicalPath}`;
    const metadata = bigintMetadata(path, label);
    if (metadata.isSymbolicLink()) {
      throw new Error(`${label} must not be a symbolic link: ${path}`);
    }
    assertCurrentOwner(metadata, path, label);
    const expectedEntry = expected.get(logicalPath);
    if (!expectedEntry) {
      throw new Error(
        `${label} is outside the nominated logical GOROOT closure`,
      );
    }
    const observedMode = Number(metadata.mode & 0o7777n);
    if (metadata.isDirectory()) {
      if (expectedEntry.type !== "directory") {
        throw new Error(`${label} changed from a file to a directory`);
      }
      if (observedMode !== MANAGED_GOROOT_DIRECTORY_MODE) {
        throw new Error(`${label} must have mode 0500: ${path}`);
      }
      entries.push({
        path: logicalPath,
        type: "directory",
        mode: observedMode,
        uid: Number(metadata.uid),
        gid: Number(metadata.gid),
        dev: metadata.dev.toString(),
        ino: metadata.ino.toString(),
        nlink: metadata.nlink.toString(),
      });
      for (const child of readdirSync(path).sort()) {
        visit(
          logicalPath === "." ? child : `${logicalPath}/${child}`,
          join(path, child),
        );
      }
      return;
    }
    if (!metadata.isFile()) {
      throw new Error(`${label} must not be a special file: ${path}`);
    }
    if (expectedEntry.type !== "file") {
      throw new Error(`${label} changed from a directory to a file`);
    }
    if (metadata.nlink !== 1n) {
      throw new Error(`${label} must have exactly one link: ${path}`);
    }
    if (observedMode !== expectedEntry.mode) {
      throw new Error(
        `${label} must have mode ${expectedEntry.mode.toString(8).padStart(4, "0")}: ${path}`,
      );
    }
    const content = readStableSourceFile(path, metadata, label);
    entries.push({
      path: logicalPath,
      type: "file",
      mode: observedMode,
      uid: Number(metadata.uid),
      gid: Number(metadata.gid),
      dev: metadata.dev.toString(),
      ino: metadata.ino.toString(),
      nlink: metadata.nlink.toString(),
      ...content,
      executable: (observedMode & 0o111) !== 0,
    });
  }

  visit(".", managedRoot);
  const observedPaths = new Set(entries.map((entry) => entry.path));
  for (const path of expected.keys()) {
    if (!observedPaths.has(path)) {
      throw new Error(`owner gate managed GOROOT entry is missing: ${path}`);
    }
  }
  const normalized = entries.map((entry) => ({
    path: entry.path,
    type: entry.type,
    mode: entry.mode,
    ...(entry.type === "file"
      ? {
          bytes: entry.bytes,
          sha256: entry.sha256,
          executable: entry.executable,
        }
      : {}),
  }));
  assertExactManifest(
    canonicalManifest(expectedEntries),
    canonicalManifest(normalized),
    "owner gate managed GOROOT logical closure",
  );
  return canonicalManifest(entries);
}

function assertSourceGoBinClosure(sourceManifest) {
  const executableEntries = sourceManifest.entries
    .filter(
      (entry) =>
        entry.type === "file" &&
        entry.executable &&
        entry.path.startsWith("bin/") &&
        !entry.path.slice(4).includes("/"),
    )
    .map((entry) => entry.path.slice(4))
    .sort();
  const expected = [...GO_BINARIES].sort();
  if (JSON.stringify(executableEntries) !== JSON.stringify(expected)) {
    throw new Error(
      `owner gate Go bin executable closure changed (expected ${expected.join(", ")}, observed ${executableEntries.join(", ")})`,
    );
  }
}

function nominatedExecutable(name, environment) {
  const rawPath = environment?.PATH;
  if (typeof rawPath !== "string" || rawPath.length === 0) {
    throw new Error("owner gate tool nomination requires a non-empty PATH");
  }
  const entries = rawPath.split(delimiter);
  if (entries.some((entry) => entry.length === 0)) {
    throw new Error("owner gate tool nomination rejects empty PATH entries");
  }
  if (entries.some((entry) => !isAbsolute(entry))) {
    throw new Error("owner gate tool nomination rejects relative PATH entries");
  }
  for (const directory of entries) {
    const candidate = join(directory, name);
    try {
      lstatSync(candidate);
    } catch (error) {
      if (error?.code === "ENOENT" || error?.code === "ENOTDIR") continue;
      throw new Error(
        `owner gate ${name} candidate cannot be inspected: ${candidate} (${error instanceof Error ? error.message : String(error)})`,
      );
    }
    return inspectSafeExecutable(candidate, `owner gate ${name} candidate`);
  }
  throw new Error(`owner gate ${name} was not found in the nominated PATH`);
}

function copyGateTool(source, destination, label) {
  const before = digestFile(source.path);
  if (before !== source.sha256) {
    throw new Error(`${label} changed before it could be copied`);
  }
  try {
    copyFileSync(source.path, destination, fsConstants.COPYFILE_EXCL);
    chmodSync(destination, 0o500);
  } catch (error) {
    throw new Error(
      `${label} could not be copied create-only: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  const copy = inspectSafeExecutable(destination, `${label} managed copy`, {
    checkAncestors: false,
  });
  if (copy.mode !== 0o500) {
    throw new Error(`${label} managed copy must have mode 0500`);
  }
  if (copy.sha256 !== before) {
    throw new Error(`${label} changed while it was copied`);
  }
  return copy;
}

function assertExactExecutableClosure(directory, expected, label) {
  const executableEntries = readdirSync(directory)
    .filter((name) => {
      const metadata = pathMetadata(join(directory, name), `${label} entry`);
      return (
        metadata.isSymbolicLink() ||
        (!metadata.isDirectory() && (metadata.mode & 0o111) !== 0)
      );
    })
    .sort();
  const exact = [...expected].sort();
  if (JSON.stringify(executableEntries) !== JSON.stringify(exact)) {
    throw new Error(
      `${label} executable closure changed (expected ${exact.join(", ")}, observed ${executableEntries.join(", ")})`,
    );
  }
}

function assertSnapshotExecutable(
  path,
  expected,
  label,
  { checkAncestors = true } = {},
) {
  if (!expected || typeof expected !== "object") {
    throw new Error(`${label} snapshot is missing`);
  }
  const observed = inspectSafeExecutable(path, label, { checkAncestors });
  if (observed.mode !== expected.mode) {
    throw new Error(`${label} mode changed after nomination: ${path}`);
  }
  if (observed.sha256 !== expected.sha256) {
    throw new Error(`${label} bytes changed after nomination: ${path}`);
  }
}

export function createManagedToolSnapshot({
  environment = process.env,
  managedHome,
  testHooks,
} = {}) {
  assertSafeDirectory(managedHome, "owner gate managed HOME", {
    exactMode: 0o700,
  });
  assertCurrentOwner(
    pathMetadata(managedHome, "owner gate managed HOME"),
    managedHome,
    "owner gate managed HOME",
  );
  const toolBin = join(managedHome, "tool-bin");
  try {
    mkdirSync(toolBin, { mode: 0o700 });
  } catch (error) {
    throw new Error(
      `owner gate managed tool-bin must be fresh: ${toolBin} (${error instanceof Error ? error.message : String(error)})`,
    );
  }
  assertSafeDirectory(toolBin, "owner gate managed tool-bin", {
    exactMode: 0o700,
  });

  const nominated = Object.fromEntries(
    [...COPIED_GATE_TOOLS, "go"].map((name) => [
      name,
      nominatedExecutable(name, environment),
    ]),
  );
  const sourceGoBin = dirname(nominated.go.path);
  const sourceGoRoot = dirname(sourceGoBin);
  const sourceBefore = createSourceGoRootManifest(sourceGoRoot);
  assertSourceGoBinClosure(sourceBefore.manifest);
  const sourceEntries = new Map(
    sourceBefore.manifest.entries.map((entry) => [entry.path, entry]),
  );
  const nominatedGo = sourceEntries.get("bin/go");
  const nominatedGofmt = sourceEntries.get("bin/gofmt");
  if (
    nominatedGo?.type !== "file" ||
    nominatedGo.resolvedPath !== nominated.go.path ||
    nominatedGo.sha256 !== nominated.go.sha256 ||
    nominatedGofmt?.type !== "file" ||
    !nominatedGofmt.executable
  ) {
    throw new Error(
      "owner gate nominated Go is not the exact bin/go and bin/gofmt closure of its physical GOROOT",
    );
  }

  const tools = {};
  for (const name of COPIED_GATE_TOOLS) {
    tools[name] = copyGateTool(
      nominated[name],
      join(toolBin, name),
      `owner gate ${name}`,
    );
  }
  const managedGoRoot = join(managedHome, "goroot");
  const expectedEntries = normalizedGoRootEntries(sourceBefore.manifest);
  materializeManagedGoRoot({
    copySources: sourceBefore.copySources,
    expectedEntries,
    managedRoot: managedGoRoot,
    sourceRoot: sourceGoRoot,
    testHooks,
  });
  const sourceAfter = createSourceGoRootManifest(sourceGoRoot);
  assertExactManifest(
    sourceBefore.manifest,
    sourceAfter.manifest,
    "owner gate source GOROOT",
  );
  const managedManifest = collectManagedGoRootManifest(
    managedGoRoot,
    expectedEntries,
  );
  const managedEntries = new Map(
    managedManifest.entries.map((entry) => [entry.path, entry]),
  );
  const managedGoBin = join(managedGoRoot, "bin");
  const executableSnapshot = (name) => {
    const entry = managedEntries.get(`bin/${name}`);
    if (entry?.type !== "file") {
      throw new Error(`owner gate managed ${name} executable is missing`);
    }
    return {
      path: join(managedGoBin, name),
      mode: entry.mode,
      sha256: entry.sha256,
    };
  };
  const snapshot = {
    managedHome,
    toolBin,
    tools,
    go: {
      sourceRoot: sourceGoRoot,
      sourceManifest: sourceBefore.manifest,
      root: managedGoRoot,
      bin: managedGoBin,
      expectedManifest: canonicalManifest(expectedEntries),
      manifest: managedManifest,
      go: executableSnapshot("go"),
      gofmt: executableSnapshot("gofmt"),
    },
  };
  assertManagedToolSnapshot(snapshot);
  return snapshot;
}

export function assertManagedToolSnapshot(snapshot) {
  if (!snapshot || typeof snapshot !== "object") {
    throw new Error("owner gate managed tool snapshot is missing");
  }
  assertSafeDirectory(snapshot.managedHome, "owner gate managed HOME", {
    exactMode: 0o700,
  });
  assertCurrentOwner(
    pathMetadata(snapshot.managedHome, "owner gate managed HOME"),
    snapshot.managedHome,
    "owner gate managed HOME",
  );
  assertSafeDirectory(snapshot.toolBin, "owner gate managed tool-bin", {
    exactMode: 0o700,
  });
  const copiedEntries = readdirSync(snapshot.toolBin).sort();
  const expectedCopiedEntries = [...COPIED_GATE_TOOLS].sort();
  if (JSON.stringify(copiedEntries) !== JSON.stringify(expectedCopiedEntries)) {
    throw new Error(
      `owner gate managed tool-bin closure changed (expected ${expectedCopiedEntries.join(", ")}, observed ${copiedEntries.join(", ")})`,
    );
  }
  for (const name of COPIED_GATE_TOOLS) {
    assertSnapshotExecutable(
      join(snapshot.toolBin, name),
      snapshot.tools?.[name],
      `owner gate ${name} managed copy`,
      { checkAncestors: false },
    );
  }

  if (
    typeof snapshot.go?.sourceRoot !== "string" ||
    typeof snapshot.go?.root !== "string" ||
    typeof snapshot.go?.bin !== "string" ||
    snapshot.go.root !== join(snapshot.managedHome, "goroot") ||
    snapshot.go.bin !== join(snapshot.go.root, "bin") ||
    !Array.isArray(snapshot.go?.expectedManifest?.entries) ||
    !Array.isArray(snapshot.go?.manifest?.entries)
  ) {
    throw new Error("owner gate Go toolchain snapshot is invalid");
  }
  const observedManifest = collectManagedGoRootManifest(
    snapshot.go.root,
    snapshot.go.expectedManifest.entries,
  );
  assertExactManifest(
    snapshot.go.manifest,
    observedManifest,
    "owner gate managed GOROOT snapshot",
  );
  assertExactExecutableClosure(
    snapshot.go.bin,
    GO_BINARIES,
    "owner gate Go bin",
  );
  for (const name of GO_BINARIES) {
    const expected = snapshot.go[name];
    if (dirname(expected?.path ?? "") !== snapshot.go.bin) {
      throw new Error(
        `owner gate ${name} is no longer beside the nominated Go`,
      );
    }
    assertSnapshotExecutable(
      expected.path,
      expected,
      `owner gate ${name} executable`,
      { checkAncestors: false },
    );
  }
  return snapshot;
}

const MANAGED_GATE_STATE_PATHS = Object.freeze({
  gocache: "go-build",
  gomodcache: "go-mod",
  gopath: "go-path",
  tmpdir: "t",
});
const MANAGED_GATE_STATE_ROOT = "m";

function managedGateStateForHome(managedHome) {
  const root = join(managedHome, MANAGED_GATE_STATE_ROOT);
  return {
    root,
    ...Object.fromEntries(
      Object.entries(MANAGED_GATE_STATE_PATHS).map(([name, child]) => [
        name,
        join(root, child),
      ]),
    ),
  };
}

export function assertManagedGateState(state) {
  if (!state || typeof state !== "object" || !isAbsolute(state.root ?? "")) {
    throw new Error("owner gate mutable state is missing");
  }
  const expectedPaths = new Map([
    ["root", state.root],
    ...Object.entries(MANAGED_GATE_STATE_PATHS).map(([name, child]) => [
      name,
      join(state.root, child),
    ]),
  ]);
  for (const [name, expected] of expectedPaths) {
    if (state[name] !== expected) {
      throw new Error(`owner gate mutable ${name} path changed`);
    }
    const metadata = assertSafeDirectory(
      expected,
      `owner gate mutable ${name}`,
      { exactMode: 0o700 },
    );
    assertCurrentOwner(metadata, expected, `owner gate mutable ${name}`);
    if (realpathSync(expected) !== expected) {
      throw new Error(
        `owner gate mutable ${name} must not contain a symbolic path`,
      );
    }
  }
  return state;
}

export function createManagedGateState(managedHome) {
  assertSafeDirectory(managedHome, "owner gate managed HOME", {
    exactMode: 0o700,
  });
  const state = managedGateStateForHome(managedHome);
  try {
    mkdirSync(state.root, { mode: 0o700 });
    for (const child of Object.values(MANAGED_GATE_STATE_PATHS)) {
      mkdirSync(join(state.root, child), { mode: 0o700 });
    }
  } catch (error) {
    throw new Error(
      `owner gate mutable state must be fresh and create-only: ${error instanceof Error ? error.message : String(error)}`,
    );
  }
  return assertManagedGateState(state);
}

function makeOwnedDirectoriesRemovable(path, label) {
  const metadata = bigintMetadata(path, label);
  if (metadata.isSymbolicLink() || !metadata.isDirectory()) return;
  assertCurrentOwner(metadata, path, label);
  chmodSync(path, 0o700);
  for (const child of readdirSync(path)) {
    const childPath = join(path, child);
    const childMetadata = bigintMetadata(childPath, `${label} child`);
    if (childMetadata.isDirectory() && !childMetadata.isSymbolicLink()) {
      makeOwnedDirectoriesRemovable(childPath, label);
    }
  }
}

// Call only from a cleanup/finally path.  The managed GOROOT stays sealed for
// every command and every assertion; this makes its owned directories
// removable immediately before the enclosing temporary HOME is deleted.
export function prepareManagedHomeForRemoval(managedHome) {
  if (!isAbsolute(managedHome)) {
    throw new Error("owner gate managed HOME cleanup path must be absolute");
  }
  makeOwnedDirectoriesRemovable(managedHome, "owner gate managed HOME cleanup");
}

export function createHardenedGateEnvironment(
  environment,
  bunExecutable = process.execPath,
  managedHome = "/nonexistent/takoform-gate-home",
  managedTools = undefined,
) {
  if (!bunExecutable.startsWith("/") || !managedHome.startsWith("/")) {
    throw new Error("gate Bun executable and HOME must be absolute paths");
  }
  let managedToolBin;
  let goBin;
  let goRoot;
  if (typeof managedTools === "string") {
    managedToolBin = managedTools;
  } else if (managedTools && typeof managedTools === "object") {
    ({ managedToolBin, goBin, goRoot } = managedTools);
  }
  for (const [name, path] of [
    ["managed tool-bin", managedToolBin],
    ["managed Go bin", goBin],
    ["managed Go root", goRoot],
  ]) {
    if (path !== undefined && (!isAbsolute(path) || path.length === 0)) {
      throw new Error(`${name} must be an absolute path`);
    }
  }
  if (goRoot === undefined && goBin !== undefined) goRoot = dirname(goBin);
  if (goBin === undefined && goRoot !== undefined) goBin = join(goRoot, "bin");
  const managedState = assertManagedGateState(
    managedGateStateForHome(managedHome),
  );
  const hardened = createHardenedGitEnvironment(environment);
  for (const name of Object.keys(hardened)) {
    if (
      name.startsWith("BUN_") ||
      name.startsWith("CF_") ||
      name.startsWith("CGO_") ||
      name.startsWith("CLOUDFLARE_") ||
      name.startsWith("GH_") ||
      name.startsWith("GITHUB_") ||
      name === "GNUPGHOME" ||
      name === "GPG_AGENT_INFO" ||
      name === "GPG_TTY" ||
      name.startsWith("GO") ||
      name.startsWith("NODE_") ||
      name.startsWith("NPM_CONFIG_") ||
      name.startsWith("SSH_") ||
      name.startsWith("TAKOFORM_CLOUDFLARE_") ||
      /^TOFU_/u.test(name) ||
      /^TF_/u.test(name) ||
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
    GOCACHE: managedState.gocache,
    GOENV: "off",
    GOFLAGS: "-mod=readonly -buildvcs=false",
    GOMODCACHE: managedState.gomodcache,
    GONOPROXY: "",
    GONOSUMDB: "",
    GOPATH: managedState.gopath,
    GOPRIVATE: "",
    GOPROXY: "https://proxy.golang.org",
    GOSUMDB: "sum.golang.org",
    GOTOOLCHAIN: "local",
    GOVCS: "*:off",
    GOWORK: "off",
    HOME: managedHome,
    PATH: [
      ...(goBin ? [goBin] : []),
      ...(managedToolBin ? [managedToolBin] : []),
      dirname(bunExecutable),
      "/usr/local/go/bin",
      "/usr/local/bin",
      "/usr/bin",
      "/bin",
    ].join(":"),
    TEMP: managedState.tmpdir,
    TMP: managedState.tmpdir,
    TMPDIR: managedState.tmpdir,
    XDG_CACHE_HOME: join(managedHome, ".cache"),
    XDG_CONFIG_HOME: join(managedHome, ".config"),
    XDG_DATA_HOME: join(managedHome, ".local", "share"),
    ...(goRoot ? { GOROOT: goRoot } : {}),
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
    const allowedDisabledAutomaticGc = name === "gc.auto" && value === "0";
    if (
      !allowedCore.has(name) &&
      !allowedBranch &&
      !allowedRemote &&
      !allowedDisabledAutomaticGc
    ) {
      if (name === "gc.auto") {
        throw new Error(
          "repository Git configuration gc.auto must be exactly 0",
        );
      }
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
  const canonicalHttpsOrigins = new Set([
    canonicalOrigin,
    canonicalOrigin.endsWith(".git")
      ? canonicalOrigin.slice(0, -4)
      : canonicalOrigin,
  ]);
  if (!canonicalHttpsOrigins.has(values.get("remote.origin.url"))) {
    throw new Error("repository origin URL is not the canonical URL");
  }
  if (
    values.get("remote.origin.fetch") !== "+refs/heads/*:refs/remotes/origin/*"
  ) {
    throw new Error("repository origin fetch mapping is not canonical");
  }
}

function runGit({ args, encoding, environment, gitExecutable, repo }) {
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
    throw new Error(
      "Git authority clone is not the exact clean detached commit",
    );
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
    copyFileSync(
      join(snapshotRoot, "package.json"),
      join(temporaryRoot, "package.json"),
    );
    copyFileSync(
      join(snapshotRoot, "bun.lock"),
      join(temporaryRoot, "bun.lock"),
    );
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
      throw new Error(
        `publication manifest path escaped its root: ${entry.path}`,
      );
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
    args: ["ls-tree", "--full-tree", "-r", "-z", commit, "--", ...paths],
    encoding: "utf8",
    environment,
    gitExecutable,
    repo,
  });
  const entries = splitNullTerminated(raw).map((entry) => {
    const match = /^(100644|100755) blob ([0-9a-f]{40})\t([^\0]+)$/u.exec(
      entry,
    );
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
  const changed = [
    ...new Set([...expectedByPath.keys(), ...observedByPath.keys()]),
  ]
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

#!/usr/bin/env bun

// check-website-dist.mjs — fresh-build drift gate for the committed website.
//
// `website/public` is the tree the deploy uploads: every byte of it is served
// from takoform.com. This gate compares ALL of it against a fresh build, not
// just the HTML pages, because a hand-edited or stale non-HTML file — the
// /.well-known/takoform-site.json status document, a mirrored schema, robots,
// the sitemap — is served exactly as wrongly as a hand-edited page would be.
//
// A byte-for-byte comparison of the whole tree is impossible: VitePress/Vue
// scoped-style hashes and Rollup chunk names depend on the absolute build path,
// and the local search index and shared chunks are not stable between builds.
// Those paths are named below and given the strongest comparison each admits.
//
//   path class      comparison
//   ------------    ---------------------------------------------------------
//   *.html          byte equality after the writer's trailing-whitespace-only
//                   normalization
//   hashmap.json    same key set; every value names a committed asset that
//                   exists (the hash values themselves are build-path derived)
//   assets/**       exact generated path set and byte equality (frozen extras
//                   remain immutable and are exempt from the fresh set)
//   everything else BYTE EQUALITY — ~1050 files including the status document,
//                   sitemap.xml, robots.txt, tako.png, vp-icons.css and every
//                   mirrored spec/forms/formpackage/release/conformance file
//
// Outside assets/, the file SET must match exactly too, so neither an extra
// published file nor a deleted one can pass.
//
// Two fresh builds run concurrently from the actual website root. Their
// output, Vite cache and VitePress SSR temp files all live in separate
// process-unique operating-system temp directories. The config's internal
// read-only mode derives repository facts but refuses to rewrite the committed
// status document. The parent snapshots every website entry before the builds
// and requires type, mode, size, digest, mtime and ctime to remain unchanged.

import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import {
  existsSync,
  lstatSync,
  readFileSync,
  readdirSync,
  readlinkSync,
  statSync,
  symlinkSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  FROZEN_EXTRA_ASSETS,
  FROZEN_HASHMAP_ENTRIES,
  FROZEN_PUBLIC_IDENTITIES,
  FROZEN_PUBLIC_PAGES,
} from "./frozen-public-identities.mjs";
import { normalizeGeneratedHtml } from "./website-html-normalization.mjs";
import { createWebsiteSnapshotWorkspace } from "./website-snapshot-temp.mjs";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const scriptPath = fileURLToPath(import.meta.url);
const websiteRoot = path.join(repositoryRoot, "website");
const committedRoot = path.join(websiteRoot, "public");
const vitepressPackage = path.join(repositoryRoot, "node_modules", "vitepress");
const SINGLE_BUILD_ARGUMENT = "--single-build";
const SNAPSHOT_READ_ONLY_ENV = "TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY";
const CONCURRENT_BUILD_COUNT = 2;
const pages = [
  "index.html",
  "docs/index.html",
  "spec/index.html",
  "ja/index.html",
  "ja/docs/index.html",
  "ja/spec/index.html",
  "404.html",
];

// Rollup names every emitted asset `<role>.<hash>.<ext>`.
const ASSET_HASH = /\.[A-Za-z0-9_-]{8}(\.lean)?\.([A-Za-z0-9]+)$/u;

function fail(message) {
  throw new Error(`website snapshot: ${message}`);
}

export function generatedHtmlMatches(committedHtml, freshHtml) {
  return String(committedHtml) === normalizeGeneratedHtml(freshHtml);
}

function collectFiles(directory, relative = "") {
  const entries = readdirSync(directory, { withFileTypes: true }).sort(
    (left, right) => left.name.localeCompare(right.name),
  );
  const files = [];
  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    const relativePath =
      relative === "" ? entry.name : `${relative}/${entry.name}`;
    if (entry.isDirectory()) {
      files.push(...collectFiles(entryPath, relativePath));
    } else if (entry.isFile()) {
      files.push(relativePath);
    }
  }
  return files;
}

function metadataRecord(filePath) {
  const stats = lstatSync(filePath, { bigint: true });
  let kind = "other";
  let digest = "";
  if (stats.isFile()) {
    kind = "file";
    digest = createHash("sha256").update(readFileSync(filePath)).digest("hex");
  } else if (stats.isDirectory()) {
    kind = "directory";
  } else if (stats.isSymbolicLink()) {
    kind = "symlink";
    digest = createHash("sha256").update(readlinkSync(filePath)).digest("hex");
  }
  return [
    kind,
    stats.mode.toString(),
    stats.size.toString(),
    stats.mtimeNs.toString(),
    stats.ctimeNs.toString(),
    digest,
  ].join(":");
}

/** Capture every source entry without treating atime as part of the contract. */
export function snapshotWebsiteMetadata(directory) {
  const snapshot = new Map();
  function visit(currentPath, relativePath) {
    snapshot.set(
      relativePath === "" ? "." : relativePath,
      metadataRecord(currentPath),
    );
    const stats = lstatSync(currentPath);
    if (!stats.isDirectory()) return;
    for (const entry of readdirSync(currentPath, { withFileTypes: true }).sort(
      (left, right) => left.name.localeCompare(right.name),
    )) {
      visit(
        path.join(currentPath, entry.name),
        relativePath === "" ? entry.name : `${relativePath}/${entry.name}`,
      );
    }
  }
  visit(directory, "");
  return snapshot;
}

export function assertWebsiteMetadataUnchanged(before, after) {
  const changes = [];
  for (const key of new Set([...before.keys(), ...after.keys()])) {
    if (!before.has(key)) {
      changes.push(`${key} (created)`);
    } else if (!after.has(key)) {
      changes.push(`${key} (removed)`);
    } else if (before.get(key) !== after.get(key)) {
      changes.push(`${key} (metadata or bytes changed)`);
    }
  }
  if (changes.length > 0) {
    const listed =
      changes.length > 20
        ? `${changes.slice(0, 20).join(", ")} and ${changes.length - 20} more`
        : changes.join(", ");
    fail(
      `concurrent fresh builds modified website source entries: ${listed}`,
    );
  }
}

async function waitForSitemapWrite(outDir) {
  const sitemapPath = path.join(outDir, "sitemap.xml");
  const deadline = Date.now() + 5_000;
  while (Date.now() < deadline) {
    try {
      const sitemap = readFileSync(sitemapPath, "utf8");
      if (/<\/urlset>\s*$/u.test(sitemap)) return;
    } catch (error) {
      if (error?.code !== "ENOENT") throw error;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  fail("fresh sitemap.xml did not finish writing within 5 seconds");
}

function isAsset(relativePath) {
  return relativePath.startsWith("assets/");
}

/** Assert that a generated asset carries VitePress's content hash suffix. */
function assertAssetPath(relativePath) {
  if (!ASSET_HASH.test(relativePath)) {
    fail(`assets/ entry is not content-addressed: ${relativePath}`);
  }
}

/**
 * Compare the generated (non-frozen) asset tree exactly.
 *
 * Hash-stripped role matching is intentionally not enough here: a renamed
 * JavaScript or CSS file can retain the same role while carrying different
 * bytes. The deterministic single-worker build lets this gate require both
 * the exact generated path and the exact generated bytes. FROZEN_EXTRA_ASSETS
 * are retained published identities rather than current build output, so they
 * remain outside this comparison; other frozen identities keep their bytes.
 */
export function compareGeneratedAssets({
  freshFiles,
  committedFiles,
  freshRoot,
  committedRoot,
}) {
  const freshAssets = freshFiles.filter(
    (file) => isAsset(file) && !FROZEN_EXTRA_ASSETS.has(file),
  );
  const committedAssets = committedFiles.filter(
    (file) => isAsset(file) && !FROZEN_EXTRA_ASSETS.has(file),
  );
  for (const file of [...freshAssets, ...committedAssets]) {
    assertAssetPath(file);
  }

  const freshSet = new Set(freshAssets);
  const committedSet = new Set(committedAssets);
  const onlyCommitted = committedAssets.filter((file) => !freshSet.has(file));
  const onlyFresh = freshAssets.filter((file) => !committedSet.has(file));
  if (onlyCommitted.length > 0 || onlyFresh.length > 0) {
    fail(
      "the committed website/public/assets path set is not what a fresh build produces " +
        `(only in website/public: ${onlyCommitted.join(", ") || "none"}; ` +
        `only in the fresh build: ${onlyFresh.join(", ") || "none"}); run bun run website:build`,
    );
  }

  const byteDrift = [];
  for (const file of committedAssets) {
    // Frozen identities are checked against their recorded digest above, not
    // against a fresh build that may intentionally produce a new hash.
    if (FROZEN_PUBLIC_IDENTITIES.has(file)) {
      continue;
    }
    const committedBytes = readFileSync(path.join(committedRoot, file));
    const freshBytes = readFileSync(path.join(freshRoot, file));
    if (!committedBytes.equals(freshBytes)) {
      byteDrift.push(file);
    }
  }
  if (byteDrift.length > 0) {
    fail(
      `committed website/public is stale: a fresh build changes the bytes of ${byteDrift.join(", ")}; run bun run website:build`,
    );
  }

  return committedAssets.length;
}

function verifyCommittedCompleteness() {
  if (!existsSync(path.join(committedRoot, "index.html"))) {
    fail("committed website/public/index.html is missing");
  }
  for (const page of pages) {
    if (!existsSync(path.join(committedRoot, page))) {
      fail(`committed website/public/${page} is missing`);
    }
  }
  const hashmapPath = path.join(committedRoot, "hashmap.json");
  if (!existsSync(hashmapPath)) {
    fail("committed website/public/hashmap.json is missing");
  }
  let hashmap;
  try {
    hashmap = JSON.parse(readFileSync(hashmapPath, "utf8"));
  } catch (error) {
    fail(`committed hashmap.json is invalid JSON (${error.message})`);
  }
  if (hashmap === null || typeof hashmap !== "object") {
    fail("committed hashmap.json must be an object");
  }
  // VitePress resolves a route to `assets/<page-key>.<hash>.js` and its
  // `.lean.js` sibling. Both must exist or the route loads nothing.
  for (const [page, hash] of Object.entries(hashmap)) {
    for (const suffix of [".js", ".lean.js"]) {
      const name = `${page}.${hash}${suffix}`;
      const assetPath = path.join(committedRoot, "assets", name);
      if (!existsSync(assetPath) || !statSync(assetPath).isFile()) {
        fail(`committed hashmap asset is missing: assets/${name}`);
      }
    }
  }
  for (const [page, expectedHash] of FROZEN_HASHMAP_ENTRIES) {
    if (hashmap[page] !== expectedHash) {
      fail(
        `frozen hashmap route ${page} must stay pinned to ${expectedHash} (got ${hashmap[page] ?? "missing"})`,
      );
    }
    for (const suffix of [".js", ".lean.js"]) {
      const name = `${page}.${expectedHash}${suffix}`;
      const assetPath = path.join(committedRoot, "assets", name);
      if (!existsSync(assetPath) || !statSync(assetPath).isFile()) {
        fail(`frozen hashmap asset is missing: assets/${name}`);
      }
    }
  }
  return hashmap;
}

export async function runWebsiteSnapshotCheck() {
  const committedHashmap = verifyCommittedCompleteness();
  const committedFiles = collectFiles(committedRoot);
  for (const file of committedFiles) {
    if (!file.endsWith(".html") || FROZEN_PUBLIC_IDENTITIES.has(file)) {
      continue;
    }
    const html = readFileSync(path.join(committedRoot, file), "utf8");
    if (/[ \t]+$/mu.test(html)) {
      fail(`committed generated HTML has trailing whitespace: ${file}`);
    }
  }
  for (const [file, expectedDigest] of FROZEN_PUBLIC_IDENTITIES) {
    const bytes = readFileSync(path.join(committedRoot, file));
    const actualDigest = createHash("sha256").update(bytes).digest("hex");
    if (actualDigest !== expectedDigest) {
      fail(
        `immutable public page ${file} digest ${actualDigest} != ${expectedDigest}`,
      );
    }
  }

  if (!existsSync(vitepressPackage)) {
    fail("vitepress is not installed; run bun install before this gate");
  }

  const workspace = createWebsiteSnapshotWorkspace();
  const freshRoot = path.join(workspace.root, "public");
  try {
    // VitePress imports its generated SSR entry from tempDir. Give that entry
    // a local dependency parent without moving tempDir back under website/.
    symlinkSync(
      path.join(repositoryRoot, "node_modules"),
      path.join(workspace.root, "node_modules"),
      "junction",
    );
    // Keep the source root authoritative and read-only while moving every
    // VitePress write surface into this process-owned workspace. A separate
    // process owns each build, so Vite/Rollup module caches are not shared.
    const { build } = await import("vitepress");
    await build(websiteRoot, {
      outDir: freshRoot,
      onAfterConfigResolve(siteConfig) {
        siteConfig.tempDir = path.join(workspace.root, "vitepress-temp");
        siteConfig.cacheDir = path.join(workspace.root, "vitepress-cache");
        const previousBuildEnd = siteConfig.buildEnd;
        siteConfig.buildEnd = async (resolvedConfig) => {
          // VitePress 1.6.4 ends the sitemap stream without awaiting its
          // finish event. Its CLI stays alive long enough, but the API's
          // buildEnd hook can run first, so wait for complete XML explicitly.
          await waitForSitemapWrite(resolvedConfig.outDir);
          await previousBuildEnd?.(resolvedConfig);
        };
      },
    });

    const freshFiles = collectFiles(freshRoot);

    // 1. The published file set, outside the content-addressed assets/ tree.
    const freshNamed = freshFiles.filter((file) => !isAsset(file));
    const committedNamed = committedFiles.filter((file) => !isAsset(file));
    const uncommitted = freshNamed.filter(
      (file) => !committedNamed.includes(file),
    );
    const unbuilt = committedNamed.filter((file) => !freshNamed.includes(file));
    if (uncommitted.length > 0 || unbuilt.length > 0) {
      fail(
        "the committed website/public file set is not what a fresh build produces " +
          `(only in the fresh build: ${uncommitted.join(", ") || "none"}; ` +
          `only in website/public: ${unbuilt.join(", ") || "none"}); run bun run website:build`,
      );
    }

    // 2. Content-addressed assets are compared by exact path and bytes.
    const generatedAssetCount = compareGeneratedAssets({
      freshFiles,
      committedFiles,
      freshRoot,
      committedRoot,
    });

    // 3. Every committed file, by the strongest comparison its class admits.
    const htmlDrift = [];
    const byteDrift = [];
    for (const file of committedFiles) {
      if (FROZEN_PUBLIC_IDENTITIES.has(file)) {
        continue;
      }
      if (file === "hashmap.json") {
        // Values are build-path derived; the key set and asset closure are not.
        const freshHashmap = JSON.parse(
          readFileSync(path.join(freshRoot, file), "utf8"),
        );
        const committedKeys = Object.keys(committedHashmap).sort();
        const freshKeys = Object.keys(freshHashmap).sort();
        if (committedKeys.join("\n") !== freshKeys.join("\n")) {
          fail(
            "committed hashmap.json page set differs from a fresh build; run bun run website:build",
          );
        }
        continue;
      }
      if (file.endsWith(".html")) {
        const committedHtml = readFileSync(
          path.join(committedRoot, file),
          "utf8",
        );
        const freshHtml = readFileSync(path.join(freshRoot, file), "utf8");
        if (!generatedHtmlMatches(committedHtml, freshHtml)) {
          htmlDrift.push(file);
        }
        continue;
      }
      if (isAsset(file)) {
        // Generated assets were compared above. Frozen extras are retained
        // published identities and are intentionally absent from fresh output.
        continue;
      }
      const committedBytes = readFileSync(path.join(committedRoot, file));
      const freshBytes = readFileSync(path.join(freshRoot, file));
      if (!committedBytes.equals(freshBytes)) {
        byteDrift.push(file);
      }
    }
    const listed = (files) =>
      files.length > 20
        ? `${files.slice(0, 20).join(", ")} and ${files.length - 20} more`
        : files.join(", ");
    if (htmlDrift.length > 0) {
      fail(
        `committed website/public is stale: a fresh build changes normalized HTML bytes of ${listed(htmlDrift)}; run bun run website:build`,
      );
    }
    if (byteDrift.length > 0) {
      fail(
        `committed website/public is stale: a fresh build changes the bytes of ${listed(byteDrift)}; run bun run website:build`,
      );
    }

    const htmlCount = committedFiles.filter(
      (file) => file.endsWith(".html") && !FROZEN_PUBLIC_PAGES.has(file),
    ).length;
    const byteCount = committedFiles.filter(
      (file) =>
        !file.endsWith(".html") && !isAsset(file) && file !== "hashmap.json",
    ).length;
    process.stdout.write(
      `website dist OK: fresh build reproduces ${htmlCount} committed pages by normalized bytes, ` +
        `${FROZEN_PUBLIC_PAGES.size} immutable published page and ` +
        `${FROZEN_PUBLIC_IDENTITIES.size - FROZEN_PUBLIC_PAGES.size} asset dependencies byte-for-byte, ` +
        `${byteCount} non-HTML published files byte-for-byte, and ${generatedAssetCount} content-addressed assets by exact path and bytes\n`,
    );
  } finally {
    workspace.cleanup();
  }
}

function runSnapshotChild(index) {
  return new Promise((resolve, reject) => {
    const child = spawn(process.execPath, [scriptPath, SINGLE_BUILD_ARGUMENT], {
      cwd: repositoryRoot,
      env: {
        ...process.env,
        [SNAPSHOT_READ_ONLY_ENV]: "1",
        VITE_EXTRA_EXTENSIONS: "tf",
      },
      stdio: ["ignore", "pipe", "pipe"],
    });
    const stdout = [];
    const stderr = [];
    child.stdout.on("data", (chunk) => stdout.push(Buffer.from(chunk)));
    child.stderr.on("data", (chunk) => stderr.push(Buffer.from(chunk)));
    child.once("error", reject);
    child.once("close", (code, signal) => {
      const output = Buffer.concat(stdout).toString("utf8");
      const errors = Buffer.concat(stderr).toString("utf8");
      if (code === 0) {
        resolve({ index, output, errors });
        return;
      }
      reject(
        new Error(
          `fresh build ${index} exited ${code ?? `by signal ${signal}`}` +
            `${output === "" ? "" : `\nstdout:\n${output}`}` +
            `${errors === "" ? "" : `\nstderr:\n${errors}`}`,
        ),
      );
    });
  });
}

export async function runConcurrentWebsiteSnapshotCheck() {
  if (process.env[SNAPSHOT_READ_ONLY_ENV] !== undefined) {
    fail(`${SNAPSHOT_READ_ONLY_ENV} is internal and must not be set by callers`);
  }
  const before = snapshotWebsiteMetadata(websiteRoot);
  const results = await Promise.allSettled(
    Array.from({ length: CONCURRENT_BUILD_COUNT }, (_, index) =>
      runSnapshotChild(index + 1),
    ),
  );
  const after = snapshotWebsiteMetadata(websiteRoot);
  assertWebsiteMetadataUnchanged(before, after);

  const failures = results.filter((result) => result.status === "rejected");
  if (failures.length > 0) {
    throw failures[0].reason;
  }
  const firstStatus = results[0].value.output
    .split("\n")
    .find((line) => line.startsWith("website dist OK:"));
  process.stdout.write(
    `${firstStatus ?? "website dist OK"}; ${CONCURRENT_BUILD_COUNT} concurrent read-only builds agreed\n`,
  );
}

if (import.meta.main) {
  const arguments_ = process.argv.slice(2);
  if (arguments_.length === 1 && arguments_[0] === SINGLE_BUILD_ARGUMENT) {
    if (process.env[SNAPSHOT_READ_ONLY_ENV] !== "1") {
      fail(
        `${SINGLE_BUILD_ARGUMENT} requires the internal read-only environment`,
      );
    }
    await runWebsiteSnapshotCheck();
  } else if (arguments_.length === 0) {
    await runConcurrentWebsiteSnapshotCheck();
  } else {
    fail(`unexpected arguments: ${arguments_.join(" ")}`);
  }
}

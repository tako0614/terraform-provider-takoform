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
//   *.html          semantic content (scripts, styles and tags stripped)
//   hashmap.json    same key set; every value names a committed asset that
//                   exists (the hash values themselves are build-path derived)
//   assets/**       same set of hash-stripped names, and byte equality
//                   wherever the fresh build reproduces the exact hashed name
//   everything else BYTE EQUALITY — ~1050 files including the status document,
//                   sitemap.xml, robots.txt, tako.png, vp-icons.css and every
//                   mirrored spec/forms/formpackage/release/conformance file
//
// Outside assets/, the file SET must match exactly too, so neither an extra
// published file nor a deleted one can pass.
//
// The fresh build runs in a throwaway directory under the repository root so
// that module resolution reaches the pinned `node_modules`; it is removed on
// every exit path and never touches the committed `website/public`.

import { execFileSync } from "node:child_process";
import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const websiteRoot = path.join(repositoryRoot, "website");
const committedRoot = path.join(websiteRoot, "public");
const vitepressBinary = path.join(
  repositoryRoot,
  "node_modules",
  ".bin",
  "vitepress",
);
const buildRoot = path.join(repositoryRoot, ".website-snapshot-tmp");

const pages = [
  "index.html",
  "docs/index.html",
  "spec/index.html",
  "ja/index.html",
  "ja/docs/index.html",
  "ja/spec/index.html",
  "404.html",
];

// Rollup names every emitted asset `<role>.<hash>.<ext>`; the hash follows the
// absolute build path, so only the role survives across build locations.
const ASSET_HASH = /\.[A-Za-z0-9_-]{8}(\.lean)?\.([A-Za-z0-9]+)$/u;

function fail(message) {
  throw new Error(`website snapshot: ${message}`);
}

function semanticText(html) {
  return String(html)
    .replace(/<script\b[\s\S]*?<\/script>/gi, " ")
    .replace(/<style\b[\s\S]*?<\/style>/gi, " ")
    .replace(/<!--[\s\S]*?-->/g, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&apos;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&")
    .replace(/\s+/g, " ")
    .trim();
}

function collectFiles(directory, relative = "") {
  const entries = readdirSync(directory, { withFileTypes: true })
    .sort((left, right) => left.name.localeCompare(right.name));
  const files = [];
  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    const relativePath = relative === "" ? entry.name : `${relative}/${entry.name}`;
    if (entry.isDirectory()) {
      files.push(...collectFiles(entryPath, relativePath));
    } else if (entry.isFile()) {
      files.push(relativePath);
    }
  }
  return files;
}

function isGeneratedArtifact(relativePath) {
  return (
    relativePath === "public" ||
    relativePath.startsWith("public/") ||
    relativePath === ".vitepress/cache" ||
    relativePath.startsWith(".vitepress/cache/") ||
    relativePath === ".vitepress/.temp" ||
    relativePath.startsWith(".vitepress/.temp/") ||
    relativePath === ".vitepress/dist" ||
    relativePath.startsWith(".vitepress/dist/")
  );
}

function isAsset(relativePath) {
  return relativePath.startsWith("assets/");
}

/** assetRole strips the build-path-derived hash from an emitted asset name. */
function assetRole(relativePath) {
  const role = relativePath.replace(
    ASSET_HASH,
    (match, lean, extension) => `${lean ?? ""}.${extension}`,
  );
  if (role === relativePath) {
    fail(`assets/ entry is not content-addressed: ${relativePath}`);
  }
  return role;
}

function multisetDifference(left, right) {
  const remaining = new Map();
  for (const value of right) {
    remaining.set(value, (remaining.get(value) ?? 0) + 1);
  }
  const extra = [];
  for (const value of left) {
    const count = remaining.get(value) ?? 0;
    if (count === 0) {
      extra.push(value);
      continue;
    }
    remaining.set(value, count - 1);
  }
  return extra;
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
  return hashmap;
}

const committedHashmap = verifyCommittedCompleteness();

if (!existsSync(vitepressBinary)) {
  fail("vitepress is not installed; run bun install before this gate");
}

try {
  rmSync(buildRoot, { force: true, recursive: true });
  mkdirSync(buildRoot, { recursive: true });
  const sourceCopy = path.join(buildRoot, "website");
  cpSync(websiteRoot, sourceCopy, {
    recursive: true,
    filter: (source) => {
      const relative = path.relative(websiteRoot, source);
      if (relative === "" || relative.startsWith("..")) {
        return true;
      }
      return !isGeneratedArtifact(relative);
    },
  });

  const freshRoot = path.join(sourceCopy, "public");
  execFileSync(process.execPath, [vitepressBinary, "build", sourceCopy], {
    cwd: buildRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    env: { ...process.env, VITE_EXTRA_EXTENSIONS: "tf" },
  });

  const freshFiles = collectFiles(freshRoot);
  const committedFiles = collectFiles(committedRoot);

  // 1. The published file set, outside the content-addressed assets/ tree.
  const freshNamed = freshFiles.filter((file) => !isAsset(file));
  const committedNamed = committedFiles.filter((file) => !isAsset(file));
  const uncommitted = freshNamed.filter((file) => !committedNamed.includes(file));
  const unbuilt = committedNamed.filter((file) => !freshNamed.includes(file));
  if (uncommitted.length > 0 || unbuilt.length > 0) {
    fail(
      "the committed website/public file set is not what a fresh build produces " +
        `(only in the fresh build: ${uncommitted.join(", ") || "none"}; ` +
        `only in website/public: ${unbuilt.join(", ") || "none"}); run bun run website:build`,
    );
  }

  // 2. The content-addressed assets/ tree, compared by role.
  const freshRoles = freshFiles.filter(isAsset).map(assetRole).sort();
  const committedRoles = committedFiles.filter(isAsset).map(assetRole).sort();
  const uncommittedAssets = multisetDifference(committedRoles, freshRoles);
  const unbuiltAssets = multisetDifference(freshRoles, committedRoles);
  if (uncommittedAssets.length > 0 || unbuiltAssets.length > 0) {
    fail(
      "the committed website/public/assets set is not what a fresh build produces " +
        `(only in website/public: ${uncommittedAssets.join(", ") || "none"}; ` +
        `only in the fresh build: ${unbuiltAssets.join(", ") || "none"}); run bun run website:build`,
    );
  }

  // 3. Every committed file, by the strongest comparison its class admits.
  const freshSet = new Set(freshFiles);
  const semanticDrift = [];
  const byteDrift = [];
  for (const file of committedFiles) {
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
      const committedText = semanticText(
        readFileSync(path.join(committedRoot, file), "utf8"),
      );
      const freshText = semanticText(
        readFileSync(path.join(freshRoot, file), "utf8"),
      );
      if (committedText !== freshText) {
        semanticDrift.push(file);
      }
      continue;
    }
    if (isAsset(file) && !freshSet.has(file)) {
      // A different build path renamed it; role equality above already covered
      // its presence, and its bytes are not comparable across build locations.
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
  if (semanticDrift.length > 0) {
    fail(
      `committed website/public is stale: a fresh build changes semantic content of ${listed(semanticDrift)}; run bun run website:build`,
    );
  }
  if (byteDrift.length > 0) {
    fail(
      `committed website/public is stale: a fresh build changes the bytes of ${listed(byteDrift)}; run bun run website:build`,
    );
  }

  const htmlCount = committedFiles.filter((file) => file.endsWith(".html")).length;
  const byteCount = committedFiles.filter(
    (file) => !file.endsWith(".html") && !isAsset(file) && file !== "hashmap.json",
  ).length;
  process.stdout.write(
    `website dist OK: fresh build reproduces ${htmlCount} committed pages semantically, ` +
      `${byteCount} non-HTML published files byte-for-byte, and ${committedRoles.length} content-addressed assets by role\n`,
  );
} finally {
  rmSync(buildRoot, { force: true, recursive: true });
}

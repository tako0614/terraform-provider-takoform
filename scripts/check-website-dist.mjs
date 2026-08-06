#!/usr/bin/env bun

// check-website-dist.mjs — fresh-build drift gate for the committed website.
//
// VitePress/Vue scoped-style hashes depend on the absolute build path, so a
// byte-for-byte rebuild comparison is impossible. This gate instead proves:
//
//   1. the committed source builds successfully with the pinned toolchain;
//   2. every committed page's *semantic* content (visible text, scripts and
//      styles stripped) is reproduced by the fresh build;
//   3. the committed output is complete (every page and the generated
//      hashmap.json asset set exist).
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
    relativePath === ".vitepress/cache" ||
    relativePath.startsWith(".vitepress/cache/") ||
    relativePath === ".vitepress/.temp" ||
    relativePath.startsWith(".vitepress/.temp/") ||
    relativePath === ".vitepress/dist" ||
    relativePath.startsWith(".vitepress/dist/")
  );
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
  for (const asset of Object.values(hashmap)) {
    const assetPath = path.join(committedRoot, "assets", `${asset}.js`);
    if (!existsSync(assetPath) || !statSync(assetPath).isFile()) {
      fail(`committed hashmap asset is missing: assets/${asset}.js`);
    }
  }
}

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

  const freshPages = collectFiles(freshRoot).filter(
    (file) => file.endsWith(".html"),
  );
  const committedPages = collectFiles(committedRoot).filter(
    (file) => file.endsWith(".html"),
  );
  const extraFresh = freshPages.filter(
    (file) => !committedPages.includes(file),
  );
  const missingFresh = committedPages.filter(
    (file) => !freshPages.includes(file),
  );
  if (extraFresh.length > 0 || missingFresh.length > 0) {
    fail(
      `fresh build page set differs from committed website/public (extra: ${extraFresh.join(", ")}; missing: ${missingFresh.join(", ")})`,
    );
  }

  const drift = [];
  for (const page of committedPages) {
    const committedText = semanticText(
      readFileSync(path.join(committedRoot, page), "utf8"),
    );
    const freshText = semanticText(
      readFileSync(path.join(freshRoot, page), "utf8"),
    );
    if (committedText !== freshText) {
      drift.push(page);
    }
  }
  if (drift.length > 0) {
    fail(
      `committed website/public is stale: a fresh build changes semantic content of ${drift.join(", ")}; run bun run website:build`,
    );
  }

  process.stdout.write(
    `website dist OK: fresh build reproduces ${freshPages.length} committed pages semantically\n`,
  );
} finally {
  rmSync(buildRoot, { force: true, recursive: true });
}

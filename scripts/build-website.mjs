#!/usr/bin/env bun

// VitePress owns the generated website, except for public pages whose bytes
// are already an immutable published identity. Keep those exact bytes across
// a rebuild while still regenerating every current page and content-addressed
// asset normally.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  FROZEN_HASHMAP_ENTRIES,
  FROZEN_PUBLIC_IDENTITIES,
} from "./frozen-public-identities.mjs";
import { normalizeGeneratedHtml } from "./website-html-normalization.mjs";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const vitepressBinary = path.join(
  repositoryRoot,
  "node_modules",
  ".bin",
  "vitepress",
);
const preserved = new Map();
for (const [relativePath, expectedDigest] of FROZEN_PUBLIC_IDENTITIES) {
  const filePath = path.join(repositoryRoot, "website", "public", relativePath);
  const bytes = readFileSync(filePath);
  const actualDigest = createHash("sha256").update(bytes).digest("hex");
  if (actualDigest !== expectedDigest) {
    throw new Error(
      `${relativePath}: immutable public page digest ${actualDigest} != ${expectedDigest}`,
    );
  }
  preserved.set(filePath, bytes);
}

function pinFrozenHashmap() {
  const hashmapPath = path.join(
    repositoryRoot,
    "website",
    "public",
    "hashmap.json",
  );
  const hashmap = JSON.parse(readFileSync(hashmapPath, "utf8"));
  for (const [page, hash] of FROZEN_HASHMAP_ENTRIES) {
    hashmap[page] = hash;
  }
  writeFileSync(hashmapPath, JSON.stringify(hashmap));
}

function normalizeGeneratedHtmlTree(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      normalizeGeneratedHtmlTree(entryPath);
      continue;
    }
    if (
      !entry.isFile() ||
      !entry.name.endsWith(".html") ||
      preserved.has(entryPath)
    ) {
      continue;
    }
    const generated = readFileSync(entryPath, "utf8");
    const normalized = normalizeGeneratedHtml(generated);
    if (normalized !== generated) {
      writeFileSync(entryPath, normalized);
    }
  }
}

try {
  const buildEnvironment = { ...process.env, VITE_EXTRA_EXTENSIONS: "tf" };
  // Snapshot checks alone may ask the config to derive and verify status
  // without writing it. A real website build must always regenerate it.
  delete buildEnvironment.TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY;
  execFileSync(process.execPath, [vitepressBinary, "build", "website"], {
    cwd: repositoryRoot,
    stdio: "inherit",
    env: buildEnvironment,
  });
  pinFrozenHashmap();
  normalizeGeneratedHtmlTree(path.join(repositoryRoot, "website", "public"));
} finally {
  for (const [filePath, bytes] of preserved) {
    writeFileSync(filePath, bytes);
  }
}

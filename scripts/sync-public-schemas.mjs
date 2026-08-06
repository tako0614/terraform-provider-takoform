#!/usr/bin/env bun

import { copyFileSync, mkdirSync, rmSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { discoverPublicSchemas } from "./public-schema-manifest.mjs";

if (process.argv.slice(2).join(" ") !== "--write") {
  process.stderr.write(
    "usage: bun scripts/sync-public-schemas.mjs --write\n",
  );
  process.exit(1);
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
// The append-only identity ledger pins each served URL to
// `website/public/schemas/...`, the committed build output. The VitePress
// build copies `website/static/` verbatim to `website/public`, so the sync
// target is the static source projection; the ledger and the served paths do
// not change.
const websiteRoot = path.join(repositoryRoot, "website");
const publicSchemasRoot = path.join(websiteRoot, "public", "schemas");
const staticSchemasRoot = path.join(websiteRoot, "static", "schemas");
const schemas = discoverPublicSchemas(repositoryRoot);

// This directory is a generated URL projection of spec/schemas. It contains
// no hand-authored assets, so replace it as one exact set and leave stale
// public schema URLs impossible.
rmSync(staticSchemasRoot, { force: true, recursive: true });
for (const schema of schemas) {
  const relativePath = path.relative(publicSchemasRoot, schema.publicPath);
  if (relativePath.startsWith("..") || path.isAbsolute(relativePath)) {
    throw new Error(
      `schema public path escapes website/public: ${schema.publicPath}`,
    );
  }
  const target = path.join(staticSchemasRoot, relativePath);
  mkdirSync(path.dirname(target), { recursive: true });
  copyFileSync(schema.sourcePath, target);
}

for (const schema of schemas) {
  const relativePath = path.relative(publicSchemasRoot, schema.publicPath);
  process.stdout.write(
    `${path.relative(repositoryRoot, schema.sourcePath)} -> website/static/schemas${path.sep}${relativePath}\n`,
  );
}

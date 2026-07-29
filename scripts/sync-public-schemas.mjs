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
const publicSchemasRoot = path.join(
  repositoryRoot,
  "website",
  "public",
  "schemas",
);
const schemas = discoverPublicSchemas(repositoryRoot);

// This directory is a generated URL projection of spec/schemas. It contains
// no hand-authored assets, so replace it as one exact set and leave stale
// public schema URLs impossible.
rmSync(publicSchemasRoot, { force: true, recursive: true });
for (const schema of schemas) {
  mkdirSync(path.dirname(schema.publicPath), { recursive: true });
  copyFileSync(schema.sourcePath, schema.publicPath);
}

for (const schema of schemas) {
  process.stdout.write(
    `${path.relative(repositoryRoot, schema.sourcePath)} -> ${path.relative(repositoryRoot, schema.publicPath)}\n`,
  );
}

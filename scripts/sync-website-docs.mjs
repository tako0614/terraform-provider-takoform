#!/usr/bin/env bun

// sync-website-docs.mjs — projects the canonical generated provider
// documentation and examples into the VitePress site so the docs live in the
// same project instead of linking out to GitHub.
//
//   bun run sync:website-docs --write   # copy canonical docs into the site
//   bun run sync:website-docs --check   # verify the committed copies match
//
// The copies are byte-identical projections of the canonical files; the check
// mode is also exercised by check-public-surfaces.mjs.

import { copyFileSync, mkdirSync, readFileSync, readdirSync, rmSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv.slice(2).join(" ");
if (mode !== "--write" && mode !== "--check") {
  process.stderr.write("usage: bun scripts/sync-website-docs.mjs --write|--check\n");
  process.exit(1);
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const resourceDocs = readdirSync(
  path.join(repositoryRoot, "docs", "resources"),
  { withFileTypes: true },
)
  .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
  .map((entry) => entry.name)
  .sort();

const projections = [
  ...resourceDocs.map((name) => ({
    canonical: path.join(repositoryRoot, "docs", "resources", name),
    site: path.join(repositoryRoot, "website", "docs", "resources", name),
  })),
  {
    canonical: path.join(repositoryRoot, "docs", "data-sources", "interface.md"),
    site: path.join(
      repositoryRoot,
      "website",
      "docs",
      "data-sources",
      "interface.md",
    ),
  },
  ...resourceDocs.map((name) => ({
    canonical: path.join(
      repositoryRoot,
      "examples",
      "resources",
      `takoform_${name.replace(/\.md$/, "")}`,
      "resource.tf",
    ),
    // The canonical resource docs link to `../../examples/resources/
    // takoform_<name>/resource.tf`, so the static projection must keep the
    // canonical directory name to serve those URLs.
    site: path.join(
      repositoryRoot,
      "website",
      "static",
      "examples",
      "resources",
      `takoform_${name.replace(/\.md$/, "")}`,
      "resource.tf",
    ),
  })),
  {
    canonical: path.join(
      repositoryRoot,
      "examples",
      "data-sources",
      "takoform_interface",
      "data-source.tf",
    ),
    site: path.join(
      repositoryRoot,
      "website",
      "static",
      "examples",
      "data-sources",
      "takoform_interface",
      "data-source.tf",
    ),
  },
];

if (mode === "--write") {
  for (const { canonical, site } of projections) {
    mkdirSync(path.dirname(site), { recursive: true });
    copyFileSync(canonical, site);
  }
  for (const { canonical, site } of projections) {
    process.stdout.write(
      `${path.relative(repositoryRoot, canonical)} -> ${path.relative(repositoryRoot, site)}\n`,
    );
  }
  process.exit(0);
}

const drift = [];
for (const { canonical, site } of projections) {
  let expected;
  let actual;
  try {
    expected = readFileSync(canonical);
  } catch (error) {
    drift.push(`${path.relative(repositoryRoot, canonical)}: canonical missing (${error.message})`);
    continue;
  }
  try {
    actual = readFileSync(site);
  } catch (error) {
    drift.push(`${path.relative(repositoryRoot, site)}: site copy missing (${error.message})`);
    continue;
  }
  if (!expected.equals(actual)) {
    drift.push(`${path.relative(repositoryRoot, site)}: drifted from canonical`);
  }
}
if (drift.length > 0) {
  for (const line of drift) process.stderr.write(`- ${line}\n`);
  throw new Error(
    "website docs projection is stale; run bun run sync:website-docs --write",
  );
}
process.stdout.write(
  `website docs projection OK: ${projections.length} canonical files reproduced\n`,
);

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
//
// Naming note: the canonical provider reference `docs/index.md` projects to
// `website/docs/reference.md`, because the site's `/docs/` index
// (`website/docs/index.md`) is a separate hand-written quick-start page that
// is NOT a projection of `docs/index.md`. Keep those two pages distinct:
// edit the canonical `docs/` tree for reference content and the
// `website/docs/index.md` page for the quick-start surface.

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
];

// The projection is a MIRROR, not an append: these site trees hold
// projected files and nothing else. Copying alone cannot express that, so a
// canonical file that goes away — a resource document whose resource was
// withdrawn, for example — would otherwise leave an orphan the site keeps
// serving and no gate can see. --write prunes those; --check reports them.
const mirroredTrees = [
  path.join(repositoryRoot, "website", "docs", "resources"),
  path.join(repositoryRoot, "website", "docs", "data-sources"),
  path.join(repositoryRoot, "website", "static", "examples", "resources"),
  path.join(repositoryRoot, "website", "static", "examples", "data-sources"),
];

function siteFiles(directory) {
  let entries;
  try {
    entries = readdirSync(directory, { withFileTypes: true });
  } catch {
    return [];
  }
  const files = [];
  for (const entry of entries.sort((left, right) => left.name.localeCompare(right.name))) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...siteFiles(entryPath));
    } else {
      files.push(entryPath);
    }
  }
  return files;
}

function orphanedSiteFiles() {
  const projected = new Set(projections.map(({ site }) => site));
  const orphans = [];
  for (const tree of mirroredTrees) {
    for (const file of siteFiles(tree)) {
      if (!projected.has(file)) {
        orphans.push(file);
      }
    }
  }
  return orphans;
}

if (mode === "--write") {
  for (const { canonical, site } of projections) {
    mkdirSync(path.dirname(site), { recursive: true });
    copyFileSync(canonical, site);
  }
  for (const orphan of orphanedSiteFiles()) {
    rmSync(orphan);
    // An example projection owns its whole takoform_<name> directory, so the
    // now-empty directory is removed with the file it held.
    const parent = path.dirname(orphan);
    if (!mirroredTrees.includes(parent) && readdirSync(parent).length === 0) {
      rmSync(parent, { recursive: true });
    }
    process.stdout.write(`pruned ${path.relative(repositoryRoot, orphan)}\n`);
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
for (const orphan of orphanedSiteFiles()) {
  drift.push(
    `${path.relative(repositoryRoot, orphan)}: projects no canonical file (orphaned copy)`,
  );
}
if (drift.length > 0) {
  for (const line of drift) process.stderr.write(`- ${line}\n`);
  throw new Error(
    "website docs projection is stale; run bun run sync:website-docs --write",
  );
}
process.stdout.write(
  `website docs projection OK: ${projections.length} canonical files reproduced, no orphaned copies\n`,
);

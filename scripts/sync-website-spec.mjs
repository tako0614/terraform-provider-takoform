#!/usr/bin/env bun

// sync-website-spec.mjs — mirrors the canonical spec, proposals, forms,
// conformance, release, and provider-reference trees into the VitePress site
// so the documentation lives on the site at the same URLs the repository uses.
//
//   bun run sync:website-spec --write   # project the canonical trees
//   bun run sync:website-spec --check   # verify the committed projections
//
// The site mirrors repository paths as URLs, so relative links keep working.
// Documented transforms (applied identically by --write and --check):
//   - README.md page files are written as index.md (URL-neutral rename);
//   - markdown link targets ending in README.md are rewritten to index.md;
//   - links to spec/schemas/*.json are rewritten to the published /schemas/
//     URLs recorded in release/public-schema-identities.json;
//   - links to non-public trees (cmd/, internal/, website/README.md) are
//     stripped to plain text;
//   - referenced machine files (forms/*.json, conformance corpus, formpackage
//     schemas, release json) are copied into website/static/ at mirrored paths
//     so their URLs are served verbatim.

import {
  copyFileSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { discoverPublicSchemas } from "./public-schema-manifest.mjs";

const mode = process.argv.slice(2).join(" ");
if (mode !== "--write" && mode !== "--check") {
  process.stderr.write("usage: bun scripts/sync-website-spec.mjs --write|--check\n");
  process.exit(1);
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const websiteRoot = path.join(repositoryRoot, "website");
const staticRoot = path.join(websiteRoot, "static");

const schemaUrls = new Map();
for (const schema of discoverPublicSchemas(repositoryRoot)) {
  const name = path.basename(schema.sourcePath);
  const urlPath = path
    .relative(path.join(repositoryRoot, "website", "public"), schema.publicPath)
    .split(path.sep)
    .join("/");
  schemaUrls.set(name, `/schemas/${urlPath.replace(/^schemas\//, "")}`);
}

function markdownFiles(directory) {
  return readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".md"))
    .map((entry) => entry.name)
    .sort();
}

function transformLink(target) {
  const stripped = target.trim().replace(/^<|>$/g, "");
  if (
    stripped.startsWith("http:") ||
    stripped.startsWith("https:") ||
    stripped.startsWith("#") ||
    stripped.startsWith("mailto:")
  ) {
    return target;
  }
  let rewritten = stripped;
  const schemaMatch = rewritten.match(
    /^(\.\.\/|\.\.\/\.\.\/)schemas\/([^/?#]+\.json)([?#].*)?$/,
  );
  if (schemaMatch) {
    const url = schemaUrls.get(schemaMatch[2]);
    if (url !== undefined) {
      return `${url}${schemaMatch[3] ?? ""}`;
    }
  }
  if (
    rewritten.includes("cmd/form-package") ||
    rewritten.includes("internal/currentformcatalog") ||
    rewritten.includes("website/README.md") ||
    rewritten.includes("TEMPLATE.md")
  ) {
    return null; // internal or authoring-template link, stripped
  }
  // directory links into static-only corpus trees have no index page
  if (
    /^(?:\.\.\/|\.\.\/\.\.\/)?(?:form-package-v1|portable-host-v1|revocation-checkpoint-v1)\/$/.test(
      rewritten,
    ) ||
    /^(?:\.\.\/|\.\.\/\.\.\/)formpackage\/$/.test(rewritten)
  ) {
    return null;
  }
  rewritten = rewritten.replace(/README\.md$/i, "index.md");
  return rewritten;
}

function transformMarkdown(source) {
  // [text](target) links
  return source.replace(
    /(!?\[[^\]]*\])\s*\(\s*(<[^>\n]*>|[^)\s]*)(?:\s+["'][^"']*["'])?\s*\)/g,
    (match, text, rawTarget) => {
      const rewritten = transformLink(rawTarget);
      if (rewritten === null) {
        // stripped internal link, keep the plain text without the link
        // brackets so the projected page does not show `[`text`]` residue
        return text.replace(/^!?\[/, "").replace(/\]$/, "");
      }
      if (rewritten === rawTarget) {
        return match;
      }
      const quoted = rawTarget.startsWith("<") ? `<${rewritten}>` : rewritten;
      return `${text}(${quoted})`;
    },
  );
}

// pages: canonical source -> site projection (with transforms)
const pages = [];
for (const file of markdownFiles(path.join(repositoryRoot, "spec"))) {
  // The spec/README.md overview is projected as overview.md so the curated
  // contract-map index at /spec/ stays in place.
  const name =
    file === "README.md"
      ? "overview.md"
      : file.replace(/^README\.md$/i, "index.md");
  pages.push({
    canonical: path.join(repositoryRoot, "spec", file),
    site: path.join(websiteRoot, "spec", name),
  });
}
for (const directory of [
  "artifact-transport",
  "binding-contract",
  "decisions",
  "form-definition",
  "form-package",
  "host-api",
  "interface-contract",
  "schemas",
  "standard-services",
  "trust",
]) {
  const dir = path.join(repositoryRoot, "spec", directory);
  for (const file of markdownFiles(dir)) {
    pages.push({
      canonical: path.join(dir, file),
      site: path.join(
        websiteRoot,
        "spec",
        directory,
        file.replace(/^README\.md$/i, "index.md"),
      ),
    });
  }
}
for (const file of markdownFiles(path.join(repositoryRoot, "proposals"))) {
  if (file === "TEMPLATE.md") {
    continue; // authoring template with HTML-ish placeholders, not a public page
  }
  pages.push({
    canonical: path.join(repositoryRoot, "proposals", file),
    site: path.join(
      websiteRoot,
      "proposals",
      file.replace(/^README\.md$/i, "index.md"),
    ),
  });
}
for (const familyDirectory of ["edge"]) {
  const dir = path.join(repositoryRoot, "proposals", familyDirectory);
  for (const file of markdownFiles(dir)) {
    pages.push({
      canonical: path.join(dir, file),
      site: path.join(
        websiteRoot,
        "proposals",
        familyDirectory,
        file.replace(/^README\.md$/i, "index.md"),
      ),
    });
  }
}
for (const [canonical, site] of [
  [path.join(repositoryRoot, "forms", "README.md"), path.join(websiteRoot, "forms", "index.md")],
  [path.join(repositoryRoot, "conformance", "README.md"), path.join(websiteRoot, "conformance", "index.md")],
  [path.join(repositoryRoot, "release", "README.md"), path.join(websiteRoot, "release", "index.md")],
  [path.join(repositoryRoot, "docs", "index.md"), path.join(websiteRoot, "docs", "reference.md")],
]) {
  pages.push({ canonical, site });
}
for (const file of markdownFiles(path.join(repositoryRoot, "release", "migrations"))) {
  pages.push({
    canonical: path.join(repositoryRoot, "release", "migrations", file),
    site: path.join(websiteRoot, "release", "migrations", file),
  });
}

// static: machine files served verbatim at mirrored repo paths
const staticFiles = [];
const staticDirectories = [
  [
    path.join(repositoryRoot, "forms"),
    path.join(staticRoot, "forms"),
    (file) => file.startsWith("candidates/"),
  ],
  [
    path.join(repositoryRoot, "conformance"),
    path.join(staticRoot, "conformance"),
    (file) => !file.endsWith(".md"),
  ],
  [
    path.join(repositoryRoot, "formpackage", "schemas"),
    path.join(staticRoot, "formpackage", "schemas"),
    () => true,
  ],
  [
    path.join(repositoryRoot, "release"),
    path.join(staticRoot, "release"),
    (file) =>
      (file === "form-contract-continuity.json" ||
        file === "trust/trusted-root.json" ||
        file === "public-schema-identities.json" ||
        file === "provider-form-identities.json" ||
        file === "published-document-lanes.json" ||
        file.startsWith("migrations/")) &&
      !file.endsWith(".md"),
  ],
  [
    path.join(repositoryRoot, "spec"),
    path.join(staticRoot, "spec"),
    (file) => !file.endsWith(".md") && !file.endsWith(".go"),
  ],
];
function collect(directory, relative = "") {
  const entries = readdirSync(directory, { withFileTypes: true }).sort((a, b) =>
    a.name.localeCompare(b.name),
  );
  const files = [];
  for (const entry of entries) {
    const entryPath = path.join(directory, entry.name);
    const relativePath = relative === "" ? entry.name : `${relative}/${entry.name}`;
    if (entry.isDirectory()) {
      files.push(...collect(entryPath, relativePath));
    } else if (entry.isFile()) {
      files.push(relativePath);
    }
  }
  return files;
}
for (const [sourceRoot, targetRoot, include] of staticDirectories) {
  for (const file of collect(sourceRoot)) {
    if (include(file)) {
      staticFiles.push({
        canonical: path.join(sourceRoot, file),
        site: path.join(targetRoot, file),
      });
    }
  }
}

const writeProjection = () => {
  for (const { canonical, site } of pages) {
    mkdirSync(path.dirname(site), { recursive: true });
    const transformed = transformMarkdown(readFileSync(canonical, "utf8"));
    const output = transformed.endsWith("\n") ? transformed : `${transformed}\n`;
    if (Buffer.byteLength(output, "utf8") === 0) {
      throw new Error(`empty projection for ${path.relative(repositoryRoot, canonical)}`);
    }
    writeFileSync(site, Buffer.from(output, "utf8"));
  }
  for (const { canonical, site } of staticFiles) {
    mkdirSync(path.dirname(site), { recursive: true });
    copyFileSync(canonical, site);
  }
  for (const { canonical, site } of pages) {
    process.stdout.write(
      `${path.relative(repositoryRoot, canonical)} -> ${path.relative(repositoryRoot, site)}\n`,
    );
  }
  process.stdout.write(
    `website spec projection wrote ${pages.length} pages and ${staticFiles.length} static files\n`,
  );
};

const checkProjection = () => {
  const drift = [];
  for (const { canonical, site } of pages) {
    const expected = transformMarkdown(readFileSync(canonical, "utf8"));
    const output = expected.endsWith("\n") ? expected : `${expected}\n`;
    const expectedBytes = Buffer.from(output, "utf8");
    let actual;
    try {
      actual = readFileSync(site);
    } catch (error) {
      drift.push(`${path.relative(repositoryRoot, site)}: site page missing (${error.message})`);
      continue;
    }
    if (!expectedBytes.equals(actual)) {
      drift.push(`${path.relative(repositoryRoot, site)}: drifted from canonical`);
    }
  }
  for (const { canonical, site } of staticFiles) {
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
      drift.push(`${path.relative(repositoryRoot, site)}: static copy missing (${error.message})`);
      continue;
    }
    if (!expected.equals(actual)) {
      drift.push(`${path.relative(repositoryRoot, site)}: static copy drifted from canonical`);
    }
  }
  if (drift.length > 0) {
    for (const line of drift.slice(0, 40)) process.stderr.write(`- ${line}\n`);
    throw new Error(
      `website spec projection is stale (${drift.length} problems); run bun run sync:website-spec --write`,
    );
  }
  process.stdout.write(
    `website spec projection OK: ${pages.length} pages, ${staticFiles.length} static files\n`,
  );
};

if (mode === "--write") {
  writeProjection();
} else {
  checkProjection();
}

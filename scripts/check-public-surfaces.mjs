import {
  existsSync,
  readFileSync,
  readdirSync,
  statSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  discoverPublicSchemas,
  PUBLIC_SCHEMA_ROUTE,
} from "./public-schema-manifest.mjs";
import {
  loadPublicationTruth,
  validatePublicationClaimText,
} from "./publication-truth.mjs";
import { verifySiteStatusDocument } from "./site-status.mjs";
import {
  FAMILY_CANDIDATE_SET,
  PROVIDER_RELEASE_TARGET_VERSION,
} from "../website/.vitepress/site-status.mjs";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const publicRoot = path.join(repositoryRoot, "website", "public");
const failures = [];

function relative(filePath) {
  return path.relative(repositoryRoot, filePath).split(path.sep).join("/");
}

function fail(message) {
  failures.push(message);
}

function read(filePath) {
  try {
    return readFileSync(filePath, "utf8");
  } catch (error) {
    fail(`${relative(filePath)}: cannot read file (${error.message})`);
    return "";
  }
}

function readJson(filePath) {
  const source = read(filePath);
  if (source === "") {
    return {};
  }
  try {
    return JSON.parse(source);
  } catch (error) {
    fail(`${relative(filePath)}: invalid JSON (${error.message})`);
    return {};
  }
}

function sorted(values) {
  return [...values].sort((left, right) => left.localeCompare(right));
}

function compareExact(label, actualValues, expectedValues) {
  const actual = sorted(actualValues);
  const expected = sorted(expectedValues);
  const actualSet = new Set(actual);
  const expectedSet = new Set(expected);
  const missing = expected.filter((value) => !actualSet.has(value));
  const extra = actual.filter((value) => !expectedSet.has(value));

  if (missing.length > 0) {
    fail(`${label}: missing ${missing.join(", ")}`);
  }
  if (extra.length > 0) {
    fail(`${label}: unexpected ${extra.join(", ")}`);
  }
  if (actual.length !== actualSet.size) {
    const duplicates = actual.filter(
      (value, index) => actual.indexOf(value) !== index,
    );
    fail(`${label}: duplicate ${sorted(new Set(duplicates)).join(", ")}`);
  }
}

function directoryEntries(directory) {
  try {
    return readdirSync(directory, { withFileTypes: true });
  } catch (error) {
    fail(`${relative(directory)}: cannot list directory (${error.message})`);
    return [];
  }
}

function walkFiles(directory, predicate = () => true) {
  const files = [];
  for (const entry of directoryEntries(directory)) {
    const entryPath = path.join(directory, entry.name);
    if (entry.isDirectory()) {
      files.push(...walkFiles(entryPath, predicate));
    } else if (entry.isFile() && predicate(entryPath)) {
      files.push(entryPath);
    }
  }
  return files;
}

function withoutMarkdownCode(source) {
  return source
    .replace(/^[ \t]*(```|~~~)[^\n]*\n[\s\S]*?^[ \t]*\1[ \t]*$/gm, "")
    .replace(/`[^`\n]*`/g, "");
}

function markdownTargets(source) {
  const targets = [];
  const text = withoutMarkdownCode(source);
  const inlinePattern = /!?\[[^\]]*]\(\s*(<[^>\n]+>|[^)\s]+)(?:\s+["'][^"']*["'])?\s*\)/g;
  const referencePattern = /^[ \t]{0,3}\[[^\]]+]:[ \t]*(<[^>\n]+>|\S+)/gm;

  for (const pattern of [inlinePattern, referencePattern]) {
    for (const match of text.matchAll(pattern)) {
      targets.push(match[1].replace(/^<|>$/g, ""));
    }
  }
  return targets;
}

function decodePath(value, context) {
  try {
    return decodeURIComponent(value);
  } catch {
    fail(`${context}: invalid URL encoding in ${JSON.stringify(value)}`);
    return value;
  }
}

function localLinkParts(rawTarget) {
  const target = rawTarget.trim();
  if (
    target === "" ||
    target.startsWith("//") ||
    /^[a-z][a-z0-9+.-]*:/i.test(target)
  ) {
    return null;
  }

  const hashIndex = target.indexOf("#");
  const beforeHash = hashIndex === -1 ? target : target.slice(0, hashIndex);
  const fragment = hashIndex === -1 ? "" : target.slice(hashIndex + 1);
  const queryIndex = beforeHash.indexOf("?");
  const pathname =
    queryIndex === -1 ? beforeHash : beforeHash.slice(0, queryIndex);
  return { fragment, pathname };
}

function resolveMarkdownTarget(sourceFile, rawTarget) {
  const parts = localLinkParts(rawTarget);
  if (parts === null) {
    return null;
  }
  const decodedPath = decodePath(
    parts.pathname,
    `${relative(sourceFile)} Markdown link`,
  );
  if (decodedPath === "") {
    return sourceFile;
  }
  return decodedPath.startsWith("/")
    ? path.resolve(repositoryRoot, `.${decodedPath}`)
    : path.resolve(path.dirname(sourceFile), decodedPath);
}

function checkMarkdownLinks() {
  const markdownRoots = [
    path.join(repositoryRoot, "docs"),
    path.join(repositoryRoot, "spec"),
    path.join(repositoryRoot, "forms"),
    path.join(repositoryRoot, "release"),
    path.join(repositoryRoot, "admission"),
    path.join(repositoryRoot, "conformance"),
  ];
  const markdownFiles = [
    path.join(repositoryRoot, "README.md"),
    path.join(repositoryRoot, "website", "README.md"),
    ...markdownRoots.flatMap((directory) =>
      walkFiles(directory, (filePath) => filePath.endsWith(".md")),
    ),
  ];

  for (const filePath of markdownFiles) {
    for (const rawTarget of markdownTargets(read(filePath))) {
      const resolved = resolveMarkdownTarget(filePath, rawTarget);
      if (resolved !== null && !existsSync(resolved)) {
        fail(
          `${relative(filePath)}: local Markdown link target does not exist: ${rawTarget}`,
        );
      }
    }
  }
}

// The site mirrors repository paths as URLs: a link target may resolve in the
// website tree or in website/static/, which is served verbatim at the same
// relative URL. The canonical-tree check above covers repository files; this
// check covers the committed site projections, which can resolve differently.
function checkWebsiteMarkdownLinks() {
  const websiteRoot = path.join(repositoryRoot, "website");
  const staticRoot = path.join(websiteRoot, "static");
  const siteFiles = walkFiles(websiteRoot, (filePath) => {
    if (!filePath.endsWith(".md")) {
      return false;
    }
    return !filePath.startsWith(path.join(websiteRoot, "public"));
  });

  const existsOnSite = (targetPath) => {
    const candidates = [
      targetPath,
      `${targetPath}.md`,
      `${targetPath}.html`,
      targetPath.replace(/\.html$/, ".md"),
      path.join(targetPath, "index.md"),
    ];
    if (candidates.some((candidate) => existsSync(candidate))) {
      return true;
    }
    const underWebsite = path.relative(websiteRoot, targetPath);
    if (underWebsite === "" || underWebsite.startsWith("..")) {
      return false;
    }
    const mirrored = path.join(staticRoot, underWebsite);
    const mirroredCandidates = [
      mirrored,
      `${mirrored}.md`,
      `${mirrored}.html`,
      path.join(mirrored, "index.md"),
    ];
    return mirroredCandidates.some((candidate) => existsSync(candidate));
  };

  for (const filePath of siteFiles) {
    for (const rawTarget of markdownTargets(read(filePath))) {
      const parts = localLinkParts(rawTarget);
      if (parts === null) {
        continue;
      }
      const decodedPath = decodePath(
        parts.pathname,
        `${relative(filePath)} website link`,
      );
      if (decodedPath === "") {
        continue;
      }
      const candidate = decodedPath.startsWith("/")
        ? path.join(websiteRoot, `.${decodedPath}`)
        : path.resolve(path.dirname(filePath), decodedPath);
      if (!existsOnSite(candidate)) {
        fail(
          `${relative(filePath)}: website link target does not exist on the site: ${rawTarget}`,
        );
      }
    }
  }
}

function htmlAttribute(attributes, name) {
  const pattern = new RegExp(
    `(?:^|\\s)${name}\\s*=\\s*(?:"([^"]*)"|'([^']*)'|([^\\s"'=<>]+))`,
    "i",
  );
  const match = attributes.match(pattern);
  return match ? (match[1] ?? match[2] ?? match[3]) : null;
}

function htmlAttributes(source, names) {
  const wanted = new Set(names);
  const matches = [];
  const tagPattern = /<([a-z][a-z0-9:-]*)\b([^>]*)>/gi;
  for (const tag of source.matchAll(tagPattern)) {
    for (const name of wanted) {
      const value = htmlAttribute(tag[2], name);
      if (value !== null) {
        matches.push({ name, value });
      }
    }
  }
  return matches;
}

function htmlIds(source) {
  return htmlAttributes(source, ["id"]).map(({ value }) => value);
}

function decodeHtml(value) {
  return value
    .replace(/&quot;/g, '"')
    .replace(/&#39;|&apos;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&amp;/g, "&")
    .replace(/&#x([0-9a-f]+);/gi, (_, digits) =>
      String.fromCodePoint(Number.parseInt(digits, 16)),
    )
    .replace(/&#([0-9]+);/g, (_, digits) =>
      String.fromCodePoint(Number.parseInt(digits, 10)),
    );
}

function resolvePublicPath(sourceFile, rawPathname) {
  const pathname = decodePath(
    decodeHtml(rawPathname),
    `${relative(sourceFile)} HTML link`,
  );
  let candidate =
    pathname === ""
      ? sourceFile
      : pathname.startsWith("/")
        ? path.resolve(publicRoot, `.${pathname}`)
        : path.resolve(path.dirname(sourceFile), pathname);

  if (existsSync(candidate) && statSync(candidate).isDirectory()) {
    candidate = path.join(candidate, "index.html");
  } else if (!existsSync(candidate) && path.extname(candidate) === "") {
    candidate = path.join(candidate, "index.html");
  }
  return candidate;
}

function checkHtmlFiles() {
  const htmlFiles = walkFiles(publicRoot, (filePath) =>
    filePath.endsWith(".html"),
  );
  const sourceByFile = new Map(
    htmlFiles.map((filePath) => [filePath, read(filePath)]),
  );

  for (const [filePath, source] of sourceByFile) {
    const ids = htmlIds(source);
    const seenIds = new Set();
    for (const id of ids) {
      if (seenIds.has(id)) {
        fail(`${relative(filePath)}: duplicate HTML id ${JSON.stringify(id)}`);
      }
      seenIds.add(id);
    }

    for (const { name, value } of htmlAttributes(source, ["href", "src"])) {
      const parts = localLinkParts(decodeHtml(value));
      if (parts === null) {
        continue;
      }
      // VitePress generates locale-alternate links to untranslated Japanese
      // pages; only /ja/, /ja/docs/, and /ja/spec/ have Japanese content.
      const realJapanesePages = new Set([
        "/ja/",
        "/ja/docs/",
        "/ja/spec/",
      ]);
      if (
        parts.pathname.startsWith("/ja/") &&
        !realJapanesePages.has(parts.pathname)
      ) {
        continue;
      }
      const targetFile = resolvePublicPath(filePath, parts.pathname);
      if (!existsSync(targetFile) || !statSync(targetFile).isFile()) {
        fail(
          `${relative(filePath)}: local ${name} target does not exist: ${value}`,
        );
        continue;
      }
      if (parts.fragment !== "" && targetFile.endsWith(".html")) {
        const targetSource = sourceByFile.get(targetFile) ?? read(targetFile);
        const targetIds = new Set(htmlIds(targetSource));
        const fragment = decodePath(
          decodeHtml(parts.fragment),
          `${relative(filePath)} HTML anchor`,
        );
        if (!targetIds.has(fragment)) {
          fail(
            `${relative(filePath)}: local anchor does not exist: ${value}`,
          );
        }
      }
    }
  }
}

function checkResourceInventory(expectedFormNames) {
  for (const [page, label] of [
    ["index.html", "website/public/index.html English resource inventory"],
    [
      "ja/index.html",
      "website/public/ja/index.html Japanese resource inventory",
    ],
  ]) {
    const text = visibleHtmlText(read(path.join(publicRoot, page)));
    for (const name of expectedFormNames) {
      if (!text.includes(`takoform_${name}`)) {
        fail(`${label}: missing ${name}`);
      }
    }
  }
}

function hrefTargetsDocumentation(href, directory, docName) {
  const normalized = href.split(/[?#]/, 1)[0].replace(/\\/g, "/");
  const suffix = `/docs/${directory}/${docName}`;
  return (
    normalized.endsWith(`${suffix}.md`) ||
    normalized.endsWith(`${suffix}.html`) ||
    normalized.endsWith(suffix) ||
    normalized.endsWith(`${suffix}/`)
  );
}

function checkDocsPageLinks(expectedResourceDocNames) {
  const docsPage = path.join(publicRoot, "docs", "index.html");
  const specPage = path.join(publicRoot, "spec", "index.html");
  for (const filePath of [docsPage, specPage]) {
    if (!existsSync(filePath)) {
      fail(`${relative(filePath)}: required local page is missing`);
    }
  }
  if (!existsSync(docsPage)) {
    return;
  }

  const hrefs = htmlAttributes(read(docsPage), ["href"])
    .map(({ value }) => decodeHtml(value))
    .map((value) => {
      try {
        return decodeURIComponent(value);
      } catch {
        return value;
      }
    });
  for (const docName of expectedResourceDocNames) {
    const linked = hrefs.some((href) =>
      hrefTargetsDocumentation(href, "resources", docName),
    );
    if (!linked) {
      fail(
        `${relative(docsPage)}: missing resource documentation link for ${docName}`,
      );
    }
  }
  if (
    !hrefs.some((href) =>
      hrefTargetsDocumentation(href, "data-sources", "interface"),
    )
  ) {
    fail(
      `${relative(docsPage)}: missing data source documentation link for interface`,
    );
  }
}

function visibleHtmlText(source) {
  return decodeHtml(
    source
      .replace(/<script\b[\s\S]*?<\/script>/gi, " ")
      .replace(/<style\b[\s\S]*?<\/style>/gi, " ")
      .replace(/<[^>]+>/g, " "),
  ).replace(/\s+/g, " ");
}

function relativeToPublic(filePath) {
  return path.relative(publicRoot, filePath).split(path.sep).join("/");
}

function checkStaleWebsiteContent() {
  // Canonical mirrors (spec/, proposals/, forms/, conformance/, release/,
  // docs/reference) legitimately discuss retired versions and kinds; the
  // stale-content guard covers only the hand-authored pages.
  const htmlFiles = walkFiles(publicRoot, (filePath) => {
    if (!filePath.endsWith(".html")) {
      return false;
    }
    const relative = relativeToPublic(filePath);
    return (
      relative === "index.html" ||
      relative === "docs/index.html" ||
      relative === "spec/index.html" ||
      relative.startsWith("ja/")
    );
  });
  const combinedSource = htmlFiles.map((filePath) => read(filePath)).join("\n");
  const visibleText = visibleHtmlText(combinedSource);
  const forbidden = [
    {
      label: "removed takoform_http_service presented on the active website",
      pattern: /\btakoform_http_service\b/i,
      source: combinedSource,
    },
    {
      label: "stale provider 0.2.0 website version",
      pattern: /\b0\.2\.0\b/,
      source: visibleText,
    },
    {
      label: "removed cmd/conformance command",
      pattern: /\bcmd\/conformance\b/,
      source: visibleText,
    },
    {
      label: "removed ObjectBucket interfaces argument",
      pattern:
        /resource\s+"takoform_object_bucket"\s+"[^"]+"\s*\{[^}]*\binterfaces\s*=/i,
      source: visibleText,
    },
    {
      label: "stale ComputeInstance immutable-image description",
      pattern: /\bbuilt from an immutable image\b/i,
      source: visibleText,
    },
    {
      label: "stale ModelEndpoint declared-model description",
      pattern: /\bserving one declared model\b/i,
      source: visibleText,
    },
    {
      label: "stale FeatureFlag optional-rollout description",
      pattern: /\boptional percentage rollout\b/i,
      source: visibleText,
    },
    {
      label: "stale ObjectLifecycleRule two-action description",
      pattern: /\bRetention and transition rule\b/i,
      source: visibleText,
    },
    {
      label: "stale Japanese ComputeInstance description",
      pattern:
        /<code>\s*takoform_compute_instance\s*<\/code>\s*<span>\s*マシンインスタンス\s*<\/span>/i,
      source: combinedSource,
    },
    {
      label: "stale Japanese StatefulEntity description",
      pattern:
        /<code>\s*takoform_stateful_entity\s*<\/code>\s*<span>\s*状態を持つエンティティ\s*<\/span>/i,
      source: combinedSource,
    },
    {
      label: "stale Japanese ObjectLifecycleRule description",
      pattern:
        /<code>\s*takoform_object_lifecycle_rule\s*<\/code>\s*<span>\s*オブジェクト保持ルール\s*<\/span>/i,
      source: combinedSource,
    },
    {
      label: "stale Japanese ModelEndpoint description",
      pattern:
        /<code>\s*takoform_model_endpoint\s*<\/code>\s*<span>\s*推論エンドポイント\s*<\/span>/i,
      source: combinedSource,
    },
    {
      label: "stale Japanese FeatureFlag description",
      pattern:
        /<code>\s*takoform_feature_flag\s*<\/code>\s*<span>\s*フィーチャーフラグ\s*<\/span>/i,
      source: combinedSource,
    },
  ];

  for (const rule of forbidden) {
    if (rule.pattern.test(rule.source)) {
      fail(`website/public: ${rule.label}`);
    }
  }
}

function providerCodeBlocksFromMarkdown(source) {
  return [...source.matchAll(/^[ \t]*```[^\n]*\n([\s\S]*?)^[ \t]*```[ \t]*$/gm)]
    .map((match) => match[1])
    .filter((block) =>
      block.includes("registry.terraform.io/tako0614/takoform"),
    );
}

function providerCodeBlocksFromHtml(source) {
  return [...source.matchAll(/<pre\b[^>]*>\s*<code\b[^>]*>([\s\S]*?)<\/code>\s*<\/pre>/gi)]
    .map((match) => visibleHtmlText(match[1]))
    .filter((block) =>
      block.includes("registry.terraform.io/tako0614/takoform"),
    );
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function hasExactProviderPin(block, providerVersion) {
  return new RegExp(
    `\\bversion\\s*=\\s*"= ${escapeRegExp(providerVersion)}"`,
  ).test(block);
}

function checkTerraformProviderExample(filePath, providerVersion) {
  const source = read(filePath);
  if (!source.includes("registry.terraform.io/tako0614/takoform")) {
    fail(`${relative(filePath)}: missing canonical provider source`);
    return;
  }
  if (!hasExactProviderPin(source, providerVersion)) {
    fail(
      `${relative(filePath)}: provider example must contain version = "= ${providerVersion}"`,
    );
  }
}

// The Edge Platform Family examples pin the Registry-published v2.1.1
// distribution. The release descriptor intentionally remains candidate-only
// metadata after owner publication, so an example must state both independent
// facts without treating descriptor metadata as Provider availability.
//
// Keep the example pin bound to the canonical Provider target. The retained
// edgePreviewProvider field is descriptor metadata only and is not authority.
const edgeFamilySourceCandidateVersion = PROVIDER_RELEASE_TARGET_VERSION;

function checkEdgeFamilyProviderExample(filePath) {
  const source = read(filePath);
  if (!source.includes("registry.terraform.io/tako0614/takoform")) {
    fail(`${relative(filePath)}: missing canonical provider source`);
    return;
  }
  if (!hasExactProviderPin(source, edgeFamilySourceCandidateVersion)) {
    fail(
      `${relative(filePath)}: Edge Family example must contain version = "= ${edgeFamilySourceCandidateVersion}"`,
    );
  }
  if (
    !source.includes(`Provider v${PROVIDER_RELEASE_TARGET_VERSION} is Registry-published`) ||
    !source.includes("candidate-only descriptor metadata after owner publication")
  ) {
    fail(
      `${relative(filePath)}: Edge Family example must state Registry-published Provider ` +
        "availability and candidate-only descriptor metadata after owner publication",
    );
  }
}

function hasNotInstallableWording(text) {
  return (
    /\bnot\s+(?:yet\s+)?installable\b/i.test(text) ||
    /Registry[^.。]{0,120}(?:インストールできません|インストール不可)/i.test(
      text,
    )
  );
}

function checkImmutableProviderTagDocs(source, truth) {
  const escapedVersion = escapeRegExp(truth.providerVersion);
  const forbidden = [
    {
      label: "stale unpublished provider status",
      pattern: new RegExp(
        `\\bv?${escapedVersion}\\b[^.\\n]{0,140}\\b(?:unpublished|unavailable)\\b`,
        "i",
      ),
    },
    {
      label: "stale not-installable provider status",
      pattern: new RegExp(
        `\\bv?${escapedVersion}\\b[^.\\n]{0,140}\\bnot (?:yet )?installable\\b`,
        "i",
      ),
    },
    {
      label: "shallow immutable-tag verification checkout",
      pattern: /^git clone[^\n]*(?:--depth|--shallow)/m,
    },
  ];
  for (const rule of forbidden) {
    if (rule.pattern.test(source)) {
      fail(`docs/index.md: immutable provider tag docs contain ${rule.label}`);
    }
  }
  const required = [
    {
      label: "timeless immutable-document availability boundary",
      pattern:
        /Availability is verified, not declared by this immutable documentation\./,
    },
    {
      label: "canonical Registry version endpoint",
      pattern:
        /https:\/\/registry\.terraform\.io\/v1\/providers\/tako0614\/takoform\/versions/,
    },
    {
      label: "exact provider tag checkout",
      pattern: new RegExp(
        `git checkout --detach v${escapeRegExp(truth.providerVersion)}`,
      ),
    },
    {
      label: "full provider source clone",
      pattern:
        /^git clone https:\/\/github\.com\/tako0614\/terraform-provider-takoform\.git$/m,
    },
    {
      label: "Registry publication evidence boundary",
      pattern:
        /source tag, documentation page, or local build alone is not\s+Registry publication or installation evidence\./,
    },
  ];
  for (const rule of required) {
    if (!rule.pattern.test(source)) {
      fail(`docs/index.md: missing ${rule.label}`);
    }
  }
}

function deriveHistoricalLegacyProviderVersion() {
  const ledger = readJson(
    path.join(repositoryRoot, "release", "provider-release-identities.json"),
  );
  const entries = Array.isArray(ledger.entries) ? ledger.entries : [];
  const candidates = entries.filter((entry) => {
    const version = typeof entry?.version === "string" ? entry.version : "";
    const [major, minor, patch] = version.split(".").map(Number);
    return (
      entry?.status === "assigned" &&
      Number.isInteger(major) &&
      major === 1 &&
      Number.isInteger(minor) &&
      Number.isInteger(patch) &&
      entry?.tag === `v${version}` &&
      typeof entry?.tagObject === "string" &&
      entry.tagObject !== "" &&
      typeof entry?.commit === "string" &&
      entry.commit !== "" &&
      typeof entry?.signingFingerprint === "string" &&
      entry.signingFingerprint !== ""
    );
  });
  candidates.sort((left, right) => {
    const parse = (value) => value.split(".").map(Number);
    const a = parse(left.version);
    const b = parse(right.version);
    return b[0] - a[0] || b[1] - a[1] || b[2] - a[2];
  });
  return candidates[0]?.version ?? null;
}

function handAuthoredPages() {
  return walkFiles(publicRoot, (filePath) => {
    if (!filePath.endsWith(".html")) {
      return false;
    }
    const relative = relativeToPublic(filePath);
    const keep =
      relative === "index.html" ||
      relative === "docs/index.html" ||
      relative === "spec/index.html" ||
      relative.startsWith("ja/");
    return keep;
  });
}

function checkPublishedProviderExamples(truth) {
  if (truth === null) {
    return;
  }
  const historicalLegacyProviderVersion =
    deriveHistoricalLegacyProviderVersion();
  if (historicalLegacyProviderVersion === null) {
    fail(
      "release/provider-release-identities.json: missing an assigned, signed major-1 Legacy provider identity",
    );
    return;
  }
  const docsIndex = path.join(repositoryRoot, "docs", "index.md");
  const docsSource = read(docsIndex);
  const docsBlocks = providerCodeBlocksFromMarkdown(docsSource);
  if (docsBlocks.length === 0) {
    fail("docs/index.md: missing provider source example");
  }
  if (!docsBlocks.some((block) => hasExactProviderPin(block, truth.providerVersion))) {
    fail(`docs/index.md: missing current published provider v${truth.providerVersion} pin`);
  }
  if (!docsBlocks.some((block) => hasExactProviderPin(block, truth.legacyProviderVersion))) {
    fail(`docs/index.md: missing Legacy provider v${truth.legacyProviderVersion} pin`);
  }
  checkImmutableProviderTagDocs(docsSource, truth);

  // Historical migration guides legitimately pin older provider versions, so
  // the exact-pin requirement covers only the hand-authored pages.
  const htmlFiles = handAuthoredPages();
  let sawPublished = false;
  let sawLegacy = false;
  let sawHistoricalLegacy = false;
  for (const filePath of htmlFiles) {
    const source = read(filePath);
    const blocks = providerCodeBlocksFromHtml(source);
    if (blocks.length === 0) {
      continue;
    }
    for (const [index, block] of blocks.entries()) {
      const published = hasExactProviderPin(block, truth.providerVersion);
      const legacy = hasExactProviderPin(block, truth.legacyProviderVersion);
      const historicalLegacy = hasExactProviderPin(
        block,
        historicalLegacyProviderVersion,
      );
      sawPublished ||= published;
      sawLegacy ||= legacy;
      sawHistoricalLegacy ||= historicalLegacy;
      if (
        historicalLegacy &&
        !/(?:published\s+Legacy|Legacy\s+Provider|公開済み[^\n]*Legacy)/i.test(
          block,
        )
      ) {
        fail(
          `${relative(filePath)}: provider example ${index + 1} must label ` +
            `v${historicalLegacyProviderVersion} as published Legacy`,
        );
      }
      if (!published && !legacy && !historicalLegacy) {
        fail(
          `${relative(filePath)}: provider example ${index + 1} must contain ` +
            `a current v${truth.providerVersion}, compatibility v${truth.legacyProviderVersion}, ` +
            `or published Legacy v${historicalLegacyProviderVersion} exact pin`,
        );
      }
    }
  }
  if (!sawPublished || !sawLegacy) {
    fail("website/public: provider examples must distinguish published current v2 from published Legacy v1");
  }
  if (!sawHistoricalLegacy) {
    fail(
      `website/public: provider examples must include published Legacy v${historicalLegacyProviderVersion}`,
    );
  }
}

function checkRetainedPublicationTruthCopy(truth) {
  if (truth === null) {
    return;
  }
  const truthFiles = [
    path.join(repositoryRoot, "README.md"),
    path.join(repositoryRoot, "SECURITY.md"),
    path.join(repositoryRoot, "docs", "index.md"),
    path.join(repositoryRoot, "website", "README.md"),
    path.join(publicRoot, "index.html"),
    path.join(publicRoot, "docs", "index.html"),
    path.join(publicRoot, "spec", "index.html"),
  ];
  const textByFile = new Map(
    truthFiles.map((filePath) => {
      const source = read(filePath);
      const text = filePath.endsWith(".html")
        ? visibleHtmlText(source)
        : source.replace(/\s+/g, " ");
      return [filePath, text];
    }),
  );

  for (const filePath of truthFiles) {
    const text = textByFile.get(filePath) ?? "";
    for (const [label, value] of [
      ["provider version", truth.providerVersion],
      ["Legacy provider version", truth.legacyProviderVersion],
      ["project status", "Experimental"],
      ["published identity classification", "Legacy"],
    ]) {
      if (!text.includes(value)) {
        fail(`${relative(filePath)}: missing evidence-derived ${label} ${value}`);
      }
    }
    try {
      validatePublicationClaimText(text, truth, relative(filePath));
    } catch (error) {
      fail(error.message);
    }
  }

  const apiTruthFiles = truthFiles.filter(
    (filePath) => filePath !== path.join(repositoryRoot, "SECURITY.md"),
  );
  for (const filePath of apiTruthFiles) {
    const text = textByFile.get(filePath) ?? "";
    if (!text.includes("forms.takoform.com/v1alpha1")) {
      fail(`${relative(filePath)}: missing v1alpha1 API boundary`);
    }
    if (!text.includes("forms.takoform.com/v1alpha2")) {
      fail(`${relative(filePath)}: missing retained provider-v2 v1alpha2 boundary`);
    }
    if (!text.includes("packages.forms.takoform.com/v1alpha3")) {
      fail(
        `${relative(filePath)}: missing retained provider-v2 v1alpha3 package boundary`,
      );
    }
  }

  const descriptorFiles = new Set([
    path.join(repositoryRoot, "README.md"),
    path.join(repositoryRoot, "SECURITY.md"),
    path.join(repositoryRoot, "docs", "index.md"),
    path.join(repositoryRoot, "website", "README.md"),
    path.join(publicRoot, "index.html"),
    path.join(publicRoot, "docs", "index.html"),
  ]);
  for (const [filePath, text] of textByFile) {
    const escapedVersion = escapeRegExp(truth.providerVersion);
    const staleProvider = new RegExp(
      `\\bv?${escapedVersion}\\b[^.。]{0,180}` +
        `(?:\\bunpublished\\b|\\bunavailable\\b|` +
        `\\bnot (?:yet )?installable\\b|未公開|インストールできません)`,
      "i",
    );
    if (staleProvider.test(text)) {
      fail(`${relative(filePath)}: contains stale provider publication status`);
    }
    if (descriptorFiles.has(filePath) && /\bcandidate-only\b/i.test(text)) {
      if (
        !new RegExp(`\\bv?${escapeRegExp(PROVIDER_RELEASE_TARGET_VERSION)}\\b`, "i").test(text) ||
        !/(?:release target|stable provider)/i.test(text) ||
        !/(?:descriptor|release\/version\.json)/i.test(text) ||
        !/(?:owner[^.。]{0,80}(?:publish(?:es|ed)?|publication)|owner[^.。]{0,80}公開)/i.test(text) ||
        !/(?:Registry[- ](?:published|readback)|公開済み)/i.test(text)
      ) {
        fail(
          `${relative(filePath)}: candidate-only is not bound to the stable ` +
            `${PROVIDER_RELEASE_TARGET_VERSION} release target and owner-publication boundary`,
        );
      }
    }
  }

  const websiteReadme =
    textByFile.get(path.join(repositoryRoot, "website", "README.md")) ?? "";
  for (const required of [
    /Cloudflare is used only to host/,
    /provider-neutral `EdgeWorker`/,
    /do not require Cloudflare/,
  ]) {
    if (!required.test(websiteReadme)) {
      fail("website/README.md: missing static-hosting-only Cloudflare boundary");
    }
  }
}

function checkContractLaneDocumentation(retainedSet) {
  const legacyHostWire = "forms.takoform.com/v1alpha1";
  const retainedHostWire = "forms.takoform.com/v1alpha2";
  const retainedForm = retainedSet.formApiVersion;
  const retainedPackage = retainedSet.packageApiVersion;
  const currentHostWire = "forms.takoform.com/v1beta1";
  const currentFamily = "edge.forms.takoform.com/v1beta1";
  const currentPackage = "packages.forms.takoform.com/v1alpha4";
  const documents = [
    {
      file: path.join(repositoryRoot, "spec", "README.md"),
      required: [
        legacyHostWire,
        retainedHostWire,
        currentHostWire,
        currentFamily,
        retainedPackage,
        currentPackage,
        "/.well-known/takoform/v1beta1",
        "protocol compatibility identity",
      ],
    },
    {
      file: path.join(repositoryRoot, "spec", "form-definition", "README.md"),
      required: [
        currentFamily,
        retainedForm,
        "form-definition-v1beta1.schema.json",
        "form-ref-v1beta1.schema.json",
        "retained v1alpha1 Legacy profiles",
      ],
    },
    {
      file: path.join(repositoryRoot, "spec", "form-package", "README.md"),
      required: [
        retainedForm,
        retainedPackage,
        currentPackage,
        "package-index-v1alpha4.schema.json",
      ],
    },
    {
      file: path.join(repositoryRoot, "spec", "versioning.md"),
      required: [
        legacyHostWire,
        retainedHostWire,
        currentHostWire,
        currentFamily,
        retainedPackage,
        currentPackage,
        "/.well-known/takoform/v1beta1",
        "Form epoch",
      ],
    },
    {
      file: path.join(repositoryRoot, "release", "README.md"),
      required: [
        legacyHostWire,
        retainedHostWire,
        currentHostWire,
        currentFamily,
        retainedPackage,
        currentPackage,
        "outer Host API wire",
      ],
    },
    {
      file: path.join(repositoryRoot, "proposals", "README.md"),
      required: [
        "retained v1alpha2 Proposal set",
        "v1alpha3 package",
        "forms/candidates/edge/v1beta1/candidate-set.json",
      ],
    },
  ];

  for (const { file, required } of documents) {
    const source = read(file);
    const normalized = source.replace(/\s+/gu, " ");
    for (const text of required) {
      if (!normalized.includes(text.replace(/\s+/gu, " "))) {
        fail(`${relative(file)}: missing retained/current lane boundary ${JSON.stringify(text)}`);
      }
    }
  }

  const staleClaims = [
    [
      path.join(repositoryRoot, "spec", "form-definition", "README.md"),
      "The current family profile is\n[`form-definition-v1alpha3.schema.json`",
    ],
    [
      path.join(repositoryRoot, "release", "README.md"),
      "release/version.json continues to describe provider v2.0.0",
    ],
    [
      path.join(repositoryRoot, "release", "README.md"),
      "until release assigns 2.1.1",
    ],
  ];
  for (const [file, stale] of staleClaims) {
    if (read(file).includes(stale)) {
      fail(`${relative(file)}: retains stale current-lane claim ${JSON.stringify(stale)}`);
    }
  }
}

// Published alpha schemas, frozen candidate trees, and historical ADRs are
// intentionally retained. Current Host API/family implementation code is a
// different class: an alpha identity there is semantic drift, not history.
// Keep this list narrow so a future compatibility reader must be added as an
// explicit retained exception instead of silently becoming the current lane.
function checkCurrentLaneSemanticResidue() {
  const currentCodeRoots = [
    "internal/clientv3",
    "internal/currentformmodel",
    "internal/currentformregistry",
    "internal/edgeformcatalog",
    "internal/portableconformancev3",
    "internal/provider",
    "internal/runtimeconformance",
    "internal/workerauthoring",
    "cmd/portable-host-conformance",
    "cmd/reference-host",
    "cmd/worker-authoring-conformance",
  ].map((entry) => path.join(repositoryRoot, entry));

  // The identities current code may not carry are DERIVED, not listed. They are
  // the ones this project has withdrawn, minus the ones it still serves — a
  // hand-written list of "the previous generation" is the same relative name
  // that went stale when the lane moved (decision 0036), and it would have to
  // be edited by whoever performs the next withdrawal, which is exactly the
  // edit that gets missed.
  const withdrawnIdentities = new Set(
    (readJson(path.join(repositoryRoot, "release", "published-document-lanes.json"))
      ?.retired ?? []).map(({ apiVersion }) => apiVersion),
  );
  const siteStatus = readJson(
    path.join(repositoryRoot, "website", "public", ".well-known", "takoform-site.json"),
  );
  for (const served of [
    siteStatus?.hostApiCurrent,
    siteStatus?.formFamilyCurrent,
    siteStatus?.formPackageApiCurrent,
  ]) {
    withdrawnIdentities.delete(served);
  }
  if (withdrawnIdentities.size === 0) {
    fail(
      "release/published-document-lanes.json records no withdrawn identity that " +
        "current code could still be carrying; this check would pass vacuously",
    );
  }
  const retainedCodeFiles = new Set([
    path.join(
      repositoryRoot,
      "internal",
      "currentformregistry",
      "frozen_retained_lane_test.go",
    ),
  ]);
  const currentCodeFiles = currentCodeRoots.flatMap((root) =>
    walkFiles(root, (filePath) =>
      (filePath.endsWith(".go") || filePath.endsWith(".mjs")) &&
      !retainedCodeFiles.has(filePath),
    ),
  );
  for (const filePath of currentCodeFiles) {
    const source = read(filePath);
    // The prose spellings stay hand-listed: they are phrases rather than
    // identities, so nothing derives them.
    for (const stale of [
      ...withdrawnIdentities,
      "v1alpha3 client",
      "v1alpha3 reference host",
      "Host API v1alpha3 lane",
      "raw v1alpha3 resources",
    ]) {
      if (source.includes(stale)) {
        fail(
          `${relative(filePath)}: current Beta implementation contains stale alpha semantic ${JSON.stringify(stale)}`,
        );
      }
    }
  }

  const currentDocumentation = [
    "README.md",
    "docs/index.md",
    "release/README.md",
    "spec/README.md",
    "spec/conformance.md",
    "spec/form-definition/README.md",
    "spec/host-api/README.md",
    "spec/host-api/v1beta1.md",
    "spec/schemas/README.md",
    "spec/versioning.md",
  ].map((entry) => path.join(repositoryRoot, entry));
  const staleDocumentationPatterns = [
    /The current family profile is[\s\S]{0,180}v1alpha3/i,
    /\bcurrent Host API[^\n]{0,100}\bv1alpha3\b/i,
    /\bcurrent family[^\n]{0,100}\bv1alpha3\b/i,
    /\bv1alpha3 (?:client|reference host|lane)\b/i,
    /release\/version\.json continues[\s\S]{0,160}\b2\.0\.0\b/i,
    /\b2\.0\.0 until release assigns 2\.1\.0\b/i,
  ];
  for (const filePath of currentDocumentation) {
    const source = read(filePath);
    for (const pattern of staleDocumentationPatterns) {
      if (pattern.test(source)) {
        fail(
          `${relative(filePath)}: current Beta documentation contains stale alpha/provider semantics matching ${pattern}`,
        );
      }
    }
  }

  const currentInventoryFiles = [
    "README.md",
    "docs/index.md",
    "forms/README.md",
    "website/index.md",
    "website/docs/index.md",
    "website/forms/index.md",
    "website/ja/index.md",
    "website/ja/docs/index.md",
    path.join("website", "public", "forms", "index.html"),
    ...walkFiles(path.join(publicRoot, "assets"), (filePath) =>
      /(?:^|[\\/])forms_index\.md\.[^./]+\.js$/.test(filePath),
    ).map((filePath) => path.relative(repositoryRoot, filePath)),
  ].map((entry) => path.join(repositoryRoot, entry));
  for (const filePath of currentInventoryFiles) {
    const source = read(filePath);
    const comparable = filePath.endsWith(".html")
      ? visibleHtmlText(source)
      : source;
    for (const pattern of [
      /Takosumi Cloud (?:provides|offers|runs all nine)/i,
      /Cloud provides all nine/i,
      /first production-shaped host/i,
      new RegExp(
        ["Resources", "currently", "operated", "by", "Takosumi", "Cloud"].join(
          "\\s+",
        ),
        "i",
      ),
      /currently (?:implemented|offered|operated|provided|run) by Takosumi Cloud/i,
      /Takosumi Cloud (?:implementation|service|host)[^.\n]{0,140}\b(?:currently|now|first[- ]host|first host|workload evidence|starting point)\b/i,
      /\b(?:first[- ]host|first host|workload evidence|first[- ]workload)\b[^.\n]{0,140}\b(?:Takosumi Cloud|Takosumi deployment)\b/i,
    ]) {
      if (pattern.test(comparable)) {
        fail(
          `${relative(filePath)}: current inventory claims Cloud availability matching ${pattern}`,
        );
      }
    }
  }
}

function checkSingleRegistryVocabulary() {
  const historicalAddressGuides = new Set([
    path.join(repositoryRoot, "release", "README.md"),
    path.join(
      repositoryRoot,
      "release",
      "migrations",
      "v0.2.1-to-v1.0.1.md",
    ),
    path.join(repositoryRoot, "website", "release", "index.md"),
    path.join(
      repositoryRoot,
      "website",
      "release",
      "migrations",
      "v0.2.1-to-v1.0.1.md",
    ),
  ]);
  const documentationFiles = [
    path.join(repositoryRoot, "README.md"),
    path.join(repositoryRoot, "admission", "v4", "README.md"),
    ...["docs", "forms", "release", "spec", "website"].flatMap((directory) =>
      walkFiles(path.join(repositoryRoot, directory), (filePath) =>
        filePath.endsWith(".md"),
      ),
    ),
  ];
  const stalePatterns = [
    {
      label: "dual-Registry wording",
      pattern: /\bdual[- ]Registry\b/i,
    },
    {
      label: "multiple Registry readbacks",
      pattern: /\bboth\s+Registry\s+readbacks\b/i,
    },
    {
      label: "non-canonical OpenTofu Registry FQN",
      pattern: /\bregistry\.opentofu\.org\/tako0614\/takoform\b/i,
    },
  ];

  for (const filePath of documentationFiles) {
    const source = read(filePath);
    for (const { label, pattern } of stalePatterns) {
      if (pattern.test(source)) {
        if (
          label === "non-canonical OpenTofu Registry FQN" &&
          historicalAddressGuides.has(filePath)
        ) {
          if (
            !source.includes("state replace-provider") ||
            !source.includes("registry.terraform.io/tako0614/takoform")
          ) {
            fail(
              `${relative(filePath)}: retired FQN must appear only with explicit canonical replace-provider guidance`,
            );
          }
          continue;
        }
        fail(`${relative(filePath)}: ${label}`);
      }
    }
  }
}

function checkProviderReleaseCommitBindings() {
  const releaseGuide = read(path.join(repositoryRoot, "release", "README.md"));
  for (const required of [
    "--expected-release-commit <signed-tag-peeled-release-commit-E>",
    "--expected-recovery-commit <current-reviewed-protected-main-commit-F>",
    "After an exact recovery, `--expected-commit` is instead the current",
    "release provenance and provider commit remain the tag's peeled commit `E`.",
    "require `E` to be an ancestor of `F`",
    "--expected-commit <current-reviewed-protected-main-source-commit>",
  ]) {
    if (!releaseGuide.includes(required)) {
      fail(
        `release/README.md: provider release/readback is missing split E/F binding wording ${JSON.stringify(required)}`,
      );
    }
  }
}

function checkWebsiteDocsProjection(formDocNames) {
  const byteEqual = (label, canonical, projected) => {
    let expected;
    let actual;
    try {
      expected = readFileSync(canonical);
    } catch (error) {
      fail(`${label}: canonical file missing (${error.message})`);
      return;
    }
    try {
      actual = readFileSync(projected);
    } catch (error) {
      fail(`${label}: site projection missing (${error.message})`);
      return;
    }
    if (!expected.equals(actual)) {
      fail(`${label}: site projection drifted from canonical`);
    }
  };

  for (const name of formDocNames) {
    byteEqual(
      `website/docs/resources/${name}.md`,
      path.join(repositoryRoot, "docs", "resources", `${name}.md`),
      path.join(repositoryRoot, "website", "docs", "resources", `${name}.md`),
    );
    byteEqual(
      `website/static/examples/resources/takoform_${name}/resource.tf`,
      path.join(repositoryRoot, "examples", "resources", `takoform_${name}`, "resource.tf"),
      path.join(repositoryRoot, "website", "static", "examples", "resources", `takoform_${name}`, "resource.tf"),
    );
  }
  byteEqual(
    "website/docs/data-sources/interface.md",
    path.join(repositoryRoot, "docs", "data-sources", "interface.md"),
    path.join(repositoryRoot, "website", "docs", "data-sources", "interface.md"),
  );
  byteEqual(
    "website/static/examples/data-sources/takoform_interface/data-source.tf",
    path.join(repositoryRoot, "examples", "data-sources", "takoform_interface", "data-source.tf"),
    path.join(repositoryRoot, "website", "static", "examples", "data-sources", "takoform_interface", "data-source.tf"),
  );
}

// Every hand-written inventory of the resource set, and what each one must say
// about a Form for a reader to be able to find it.
//
// The generated trees (docs/resources, examples/resources, the site
// projections) were already compared against the roster above; nothing checked
// the prose and the navigation, which is exactly why WorkerEndpoint reached
// production missing from the README, the sidebar and the Japanese docs index
// while every generated surface carried it. A Form now cannot be added or
// removed without every one of these learning about it.
function checkHandWrittenInventories(retainedForms, familyRoster) {
  const retainedKinds = retainedForms.map(({ kind }) => kind);
  const familyKinds = familyRoster.map(({ kind }) => kind);
  const retainedDocNames = retainedForms.map(({ docName }) => docName);
  const familyDocNames = familyRoster.map(({ docName }) => docName);
  const retainedSlugs = retainedForms.map(({ slug }) => slug);
  const familySlugs = familyRoster.map(({ slug }) => slug);

  const resourceDocLinks = (names) =>
    names.map((name) => ({
      needle: `/docs/resources/${name}.html`,
      subject: name,
    }));

  const inventories = [
    {
      file: "README.md",
      label: "the Edge Platform Family list",
      required: familyKinds.map((kind) => ({
        needle: `\`${kind}\``,
        subject: kind,
      })),
    },
    {
      file: "README.md",
      label: "the retained v1alpha2 list",
      required: retainedKinds.map((kind) => ({
        needle: `\`${kind}\``,
        subject: kind,
      })),
    },
    {
      file: "website/.vitepress/config.mts",
      label: "the Edge preview resource sidebar",
      required: resourceDocLinks(familyDocNames),
    },
    {
      file: "website/.vitepress/config.mts",
      label: "the current published resource sidebar",
      required: [
        ...resourceDocLinks(retainedDocNames),
        { needle: "/docs/data-sources/interface.html", subject: "interface" },
      ],
    },
    {
      file: "website/.vitepress/config.mts",
      label: "the Edge preview proposal sidebar",
      required: familySlugs.map((slug) => ({
        needle: `/proposals/edge/${slug}.html`,
        subject: slug,
      })),
    },
    {
      file: "website/.vitepress/config.mts",
      label: "the current published proposal sidebar",
      required: retainedSlugs.map((slug) => ({
        needle: `/proposals/${slug}.html`,
        subject: slug,
      })),
    },
    {
      file: "website/docs/index.md",
      label: "the English resource reference",
      required: resourceDocLinks([...retainedDocNames, ...familyDocNames]),
    },
    {
      file: "website/ja/docs/index.md",
      label: "the Japanese resource reference",
      required: resourceDocLinks([...retainedDocNames, ...familyDocNames]),
    },
    {
      file: "website/index.md",
      label: "the English landing inventory",
      required: [...retainedDocNames, ...familyDocNames].map((name) => ({
        needle: `takoform_${name}`,
        subject: name,
      })),
    },
    {
      file: "website/ja/index.md",
      label: "the Japanese landing inventory",
      required: [...retainedDocNames, ...familyDocNames].map((name) => ({
        needle: `takoform_${name}`,
        subject: name,
      })),
    },
    {
      file: "forms/README.md",
      label: "the generated Form inventory",
      required: [...retainedKinds, ...familyKinds].map((kind) => ({
        needle: `\`${kind}\``,
        subject: kind,
      })),
    },
  ];

  for (const { file, label, required } of inventories) {
    const source = read(path.join(repositoryRoot, file));
    for (const { needle, subject } of required) {
      if (!source.includes(needle)) {
        fail(
          `${file}: ${label} is missing ${subject}; expected ${JSON.stringify(needle)}`,
        );
      }
    }
  }

  for (const { slug } of familyRoster) {
    const proposal = path.join(repositoryRoot, "proposals", "edge", `${slug}.md`);
    if (!existsSync(proposal)) {
      fail(`proposals/edge/${slug}.md: Edge Platform Family Form has no Proposal`);
    }
  }
}

// A corpus directory named for its place in a sequence has to be renamed by
// hand every time a lane moves, and when v1alpha3 became v1beta1 it was not:
// portable-host-v3 kept its name and changed its contract, so one published
// address began answering about a different lane. A name that states the lane
// cannot rot that way, so a NEW corpus must carry one.
//
// The three generation-named corpora below are published addresses. Their
// names are retained history and are supposed to stay, which is exactly why
// they are listed one by one rather than matched by a pattern: adding a fourth
// is an edit somebody has to justify.
const RETAINED_GENERATION_NAMED_CORPORA = new Map([
  ["portable-host-v1", "forms.takoform.com/v1alpha1"],
  ["portable-host-v2", "forms.takoform.com/v1alpha2"],
]);

// A Host API lane is minted for one of exactly two reasons, and this table says
// which for every lane that exists (decision 0039).
//
//   "protocol"   — the wire contract changed. Provable: the lane's wire schema
//                  must differ structurally from every other lane's, with
//                  version words normalised so a rename cannot masquerade as a
//                  contract.
//   "graduation" — the lane itself advanced a maturity channel on the evidence
//                  spec/versioning.md names. Not provable from bytes, so the
//                  entry has to say what it satisfied.
//
// What a lane may NOT be minted for is the family or the Forms moving. That is
// the rule spec/versioning.md already states — the group MUST NOT graduate on a
// Form count, package publication, provider major, historical admission, or one
// host's conformance report — and it is the rule v1beta1 was minted against.
const HOST_API_LANES = new Map([
  ["forms.takoform.com/v1alpha1", {
    wireSchema: "spec/schemas/host-api-wire.schema.json",
    mintedFor: "protocol",
  }],
  ["forms.takoform.com/v1alpha2", {
    wireSchema: "spec/schemas/host-api-wire-v1alpha2.schema.json",
    mintedFor: "protocol",
  }],
  ["forms.takoform.com/v1alpha3", {
    wireSchema: "spec/schemas/host-api-wire-v1alpha3.schema.json",
    mintedFor: "protocol",
  }],
  ["forms.takoform.com/v1beta1", {
    wireSchema: "spec/schemas/host-api-wire-v1beta1.schema.json",
    mintedFor: "graduation",
    // Recorded as it happened rather than as it should have. This lane was
    // minted alongside the Edge family channel move in #132, and its wire
    // contract is structurally identical to v1alpha3's — measured in decision
    // 0038. No lane-specific evidence was stated, which is exactly what the
    // rule above now requires. It is frozen into Registry-published provider
    // v2.1.1 and cannot be withdrawn; the entry stands as the record.
    evidence: "none stated; minted with the family channel move (decision 0038)",
  }],
]);

// Version words are the one thing a lane rename is guaranteed to move, so they
// are exactly what must not count as a contract difference.
function wireContractShape(relativePath) {
  const sortDeep = (value) =>
    Array.isArray(value)
      ? value.map(sortDeep)
      : value !== null && typeof value === "object"
        ? Object.fromEntries(
            Object.keys(value).sort().map((key) => [key, sortDeep(value[key])]),
          )
        : value;
  return JSON.stringify(
    sortDeep(readJson(path.join(repositoryRoot, relativePath))),
  ).replace(/v1(?:alpha|beta)\d+/gu, "vN");
}

// The package envelope gets the same rule as the lane, because it has the same
// history: one real format change and two renames that followed the grammar of
// the FormRef they wrap (decision 0040).
//
//   "format"  — the manifest format itself changed. Provable: the envelope's
//               schema must differ structurally from every other "format"
//               envelope's, version words normalised. That has happened once:
//               v1alpha1 -> v1alpha2 introduced content addressing.
//   "carried" — the envelope was re-minted because the FormRef grammar it
//               wraps moved, not because the format did. v1alpha3 and v1alpha4
//               are structurally identical to v1alpha2. Recorded as history;
//               the rule prevents the next one, and v1alpha4 has already
//               carried two family generations without moving, which is the
//               proof it never needed to track them.
const PACKAGE_ENVELOPES = new Map([
  ["packages.forms.takoform.com/v1alpha1", {
    schema: "spec/schemas/package-index.schema.json",
    mintedFor: "format",
  }],
  ["packages.forms.takoform.com/v1alpha2", {
    schema: "spec/schemas/package-index-v1alpha2.schema.json",
    mintedFor: "format",
  }],
  ["packages.forms.takoform.com/v1alpha3", {
    schema: "spec/schemas/package-index-v1alpha3.schema.json",
    mintedFor: "carried",
    evidence: "re-minted for the retained provider-v2 FormRef grammar; format unchanged (decision 0040)",
  }],
  ["packages.forms.takoform.com/v1alpha4", {
    schema: "spec/schemas/package-index-v1alpha4.schema.json",
    mintedFor: "carried",
    evidence: "re-minted for the namespaced family FormRef grammar; format unchanged, and it has since carried two family generations (decision 0040)",
  }],
]);

function checkVersionedIdentitiesAreMintedForAReason(label, table, kinds) {
  const shapes = new Map();
  for (const [identity, entry] of table) {
    if (!existsSync(path.join(repositoryRoot, entry.schema))) {
      fail(`${identity}: schema ${entry.schema} does not exist`);
      continue;
    }
    shapes.set(identity, wireContractShape(entry.schema));
  }
  for (const [identity, entry] of table) {
    if (entry.mintedFor === kinds.exempt) {
      if (typeof entry.evidence !== "string" || entry.evidence.trim() === "") {
        fail(
          `${identity}: a ${label} minted as ${JSON.stringify(kinds.exempt)} must ` +
            `say why; that is not provable from bytes`,
        );
      }
      continue;
    }
    if (entry.mintedFor !== kinds.proved) {
      fail(
        `${identity}: mintedFor must be ${JSON.stringify(kinds.proved)} or ${JSON.stringify(kinds.exempt)}`,
      );
      continue;
    }
    const shape = shapes.get(identity);
    for (const [other, otherShape] of table) {
      if (other === identity || otherShape.mintedFor !== kinds.proved) continue;
      if (shape === shapes.get(other)) {
        fail(
          `${identity}: minted as ${JSON.stringify(kinds.proved)}, but its schema is ` +
            `structurally identical to ${other}; an identity that changes no bytes ` +
            `is a rename`,
        );
        break;
      }
    }
  }
}

function checkPackageEnvelopesAreMintedForAReason() {
  checkVersionedIdentitiesAreMintedForAReason("package envelope", PACKAGE_ENVELOPES, {
    proved: "format",
    exempt: "carried",
  });
}

function checkHostApiLanesAreMintedForAReason() {
  const shapes = new Map();
  for (const [apiVersion, lane] of HOST_API_LANES) {
    if (!existsSync(path.join(repositoryRoot, lane.wireSchema))) {
      fail(`${apiVersion}: wire schema ${lane.wireSchema} does not exist`);
      continue;
    }
    shapes.set(apiVersion, wireContractShape(lane.wireSchema));
  }
  for (const [apiVersion, lane] of HOST_API_LANES) {
    if (lane.mintedFor === "graduation") {
      if (typeof lane.evidence !== "string" || lane.evidence.trim() === "") {
        fail(
          `${apiVersion}: a lane minted for a graduation must say what evidence ` +
            `it satisfied; spec/versioning.md names the prerequisites`,
        );
      }
      continue;
    }
    if (lane.mintedFor !== "protocol") {
      fail(`${apiVersion}: mintedFor must be "protocol" or "graduation"`);
      continue;
    }
    const shape = shapes.get(apiVersion);
    for (const [other, otherShape] of shapes) {
      // A graduation lane deliberately carries an existing contract under a new
      // maturity name, so it is expected to match one. Only two lanes both
      // claiming to be distinct CONTRACTS may not be the same bytes.
      if (HOST_API_LANES.get(other)?.mintedFor !== "protocol") continue;
      if (other !== apiVersion && shape === otherShape) {
        fail(
          `${apiVersion}: minted for a protocol change, but its wire contract is ` +
            `structurally identical to ${other}; a lane that changes no bytes is a ` +
            `rename, and every client renegotiates for nothing`,
        );
        break;
      }
    }
  }
}

// The README stopped telling a reader to point a provider at a placeholder and
// started telling them to run something. A command named in a walk somebody is
// expected to follow has to exist, and the walk has to name the command that
// exists — the failure mode here is a getting-started that quietly stops
// working, which is what the placeholder already was.
function checkDocumentedWalkIsRunnable() {
  const readme = read(path.join(repositoryRoot, "README.md"));
  const named = [...readme.matchAll(/go run \.\/(cmd\/[a-z0-9-]+)/gu)].map(
    ([, command]) => command,
  );
  if (named.length === 0) {
    fail("README.md: the getting-started walk names no command to run");
  }
  for (const command of new Set(named)) {
    if (!existsSync(path.join(repositoryRoot, command, "main.go"))) {
      fail(`README.md: names \`go run ./${command}\`, which does not exist`);
    }
  }
  // Prose wraps, so compare against normalized whitespace the way the corpus
  // count claims do.
  if (!readme.replace(/\s+/gu, " ").includes("serves no application traffic")) {
    fail(
      "README.md: the walk must say the reference host serves no application " +
        "traffic; a host a reader can start is a host they will believe in",
    );
  }
}

function checkCorpusNamesStateTheirLane() {
  const conformanceRoot = path.join(repositoryRoot, "conformance");
  for (const entry of readdirSync(conformanceRoot, { withFileTypes: true })) {
    if (!entry.isDirectory() || !entry.name.startsWith("portable-host-")) {
      continue;
    }
    const contractPath = path.join(
      conformanceRoot,
      entry.name,
      "contract.json",
    );
    if (!existsSync(contractPath)) {
      continue;
    }
    const apiVersion = readJson(contractPath)?.apiVersion;
    if (typeof apiVersion !== "string") {
      fail(`${contractPath}: corpus states no apiVersion`);
      continue;
    }
    const retainedLane = RETAINED_GENERATION_NAMED_CORPORA.get(entry.name);
    if (retainedLane !== undefined) {
      if (retainedLane !== apiVersion) {
        fail(
          `conformance/${entry.name}: retained corpus measures ${apiVersion}, ` +
            `but this address is retained history for ${retainedLane}; serve a ` +
            `new lane from a new directory named for it`,
        );
      }
      continue;
    }
    const expected = `portable-host-${apiVersion.split("/").pop()}`;
    if (entry.name !== expected) {
      fail(
        `conformance/${entry.name}: corpus measures ${apiVersion} and should be ` +
          `named conformance/${expected}; a corpus named for its place in a ` +
          `sequence has to be renamed by hand when a lane moves, and that is the ` +
          `step that gets missed`,
      );
    }
  }
}

// A count written in prose beside a count a machine already knows rots the
// moment the corpus moves, and nothing notices. Every such number in the
// conformance guide is bound here to the array in the corpus that defines it.
function checkConformanceCorpusCounts() {
  const file = path.join(repositoryRoot, "conformance", "README.md");
  const text = read(file).replace(/\s+/gu, " ");
  const claims = [
    {
      label: "the Host API v1beta1 check matrix size",
      pattern: /(\d+)-check matrix/g,
      corpus: "conformance/portable-host-v1beta1/contract.json",
      field: ["requiredRunnerChecks"],
    },
    {
      label: "the v1beta1 error taxonomy size",
      pattern: /(\d+)-code error taxonomy/g,
      corpus: "conformance/portable-host-v1beta1/contract.json",
      field: ["errorEnvelope", "codes"],
    },
    {
      label: "the v1beta1 automatically-retryable set size",
      pattern: /(\d+)-code retryable set/g,
      corpus: "conformance/portable-host-v1beta1/contract.json",
      field: ["errorEnvelope", "automaticallyRetryable"],
    },
    {
      label: "the runtime ABI corpus check list size",
      pattern: /(\d+) required checks/g,
      corpus: "conformance/runtime-abi-v1/contract.json",
      field: ["requiredChecks"],
    },
  ];

  for (const { label, pattern, corpus, field } of claims) {
    let declared = readJson(path.join(repositoryRoot, corpus));
    for (const key of field) {
      declared = declared?.[key];
    }
    if (!Array.isArray(declared)) {
      fail(`${corpus}: ${field.join(".")} must be an array to count`);
      continue;
    }
    const matches = [...text.matchAll(pattern)];
    if (matches.length === 0) {
      fail(
        `conformance/README.md: ${label} is not stated; the corpus declares ` +
          `${declared.length} in ${corpus} ${field.join(".")}`,
      );
      continue;
    }
    for (const match of matches) {
      if (Number(match[1]) !== declared.length) {
        fail(
          `conformance/README.md: ${label} is written as ${match[1]}, but ` +
            `${corpus} ${field.join(".")} has ${declared.length} entries`,
        );
      }
    }
  }

  const hostContractPath = path.join(
    repositoryRoot,
    "conformance",
    "portable-host-v1beta1",
    "contract.json",
  );
  const hostContract = readJson(hostContractPath);
  const runnerInput = hostContract?.runnerInput ?? {};
  const drivenKinds = Object.entries(runnerInput)
    .filter(
      ([, probe]) =>
        probe !== null &&
        typeof probe === "object" &&
        probe.identity?.formRef?.kind &&
        probe.desiredSchema !== undefined,
    )
    .map(([, probe]) => probe.identity.formRef.kind);
  const uniqueDrivenKinds = new Set(drivenKinds);
  if (drivenKinds.length !== 14 || uniqueDrivenKinds.size !== 14) {
    fail(
      `${hostContractPath}: portable-host-v1beta1 must drive exactly 14 distinct ` +
        `Form probes, found ${drivenKinds.length} (${[...uniqueDrivenKinds].join(", ")})`,
    );
  }

  const familyCandidateSet = readJson(
    path.join(repositoryRoot, FAMILY_CANDIDATE_SET),
  );
  const familyKinds = Array.isArray(familyCandidateSet.forms)
    ? familyCandidateSet.forms.map((entry) => entry?.kind).filter(Boolean)
    : [];
  const unprobedKinds = familyKinds.filter(
    (kind) => !uniqueDrivenKinds.has(kind),
  );
  if (
    familyKinds.length !== 15 ||
    unprobedKinds.length !== 1 ||
    unprobedKinds[0] !== "ObjectBucket"
  ) {
    fail(
      `${FAMILY_CANDIDATE_SET}: portable-host-v1beta1 coverage must leave exactly ` +
        `ObjectBucket unprobed (family=${familyKinds.length}, ` +
        `unprobed=${unprobedKinds.join(", ") || "none"})`,
    );
  }

  const schemaCoverageClaims = [
    ...text.matchAll(/pins each of those (\d+) Forms' DESIRED SCHEMA/gi),
  ];
  if (schemaCoverageClaims.length === 0) {
    fail(
      "conformance/README.md: portable-host-v1beta1 Form schema coverage count is not stated",
    );
  } else {
    for (const match of schemaCoverageClaims) {
      if (Number(match[1]) !== drivenKinds.length) {
        fail(
          `conformance/README.md: portable-host-v1beta1 schema coverage is written as ${match[1]}, ` +
            `but the runner drives ${drivenKinds.length} Forms`,
        );
      }
    }
  }
  if (
    !/\bObjectBucket\b[^.]{0,220}\b(?:intentionally unprobed|unprobed by this corpus)\b/i.test(
      text,
    )
  ) {
    fail(
      "conformance/README.md: portable-host-v1beta1 must state the intentional ObjectBucket coverage exception",
    );
  }
}

function checkPublicSchemas() {
  let schemas;
  try {
    schemas = discoverPublicSchemas(repositoryRoot);
  } catch (error) {
    fail(error.message);
    return;
  }

  const publicSchemasRoot = path.join(publicRoot, "schemas");
  const actualFiles = walkFiles(publicSchemasRoot);
  compareExact(
    "website/public/schemas",
    actualFiles.map((filePath) => relative(filePath)),
    schemas.map(({ publicPath }) => relative(publicPath)),
  );

  for (const schema of schemas) {
    let normative;
    let published;
    try {
      normative = readFileSync(schema.sourcePath);
      published = readFileSync(schema.publicPath);
    } catch (error) {
      fail(
        `${relative(schema.publicPath)}: cannot compare with ${relative(schema.sourcePath)} (${error.message})`,
      );
      continue;
    }
    if (!normative.equals(published)) {
      fail(
        `${relative(schema.publicPath)}: bytes drifted from ${relative(schema.sourcePath)}`,
      );
    }
  }

  const wrangler = readJson(
    path.join(repositoryRoot, "website", "wrangler.jsonc"),
  );
  const routes = Array.isArray(wrangler.routes) ? wrangler.routes : [];
  const schemaRoutes = routes.filter(
    (route) =>
      route !== null &&
      typeof route === "object" &&
      route.pattern === PUBLIC_SCHEMA_ROUTE,
  );
  if (
    schemaRoutes.length !== 1 ||
    schemaRoutes[0].custom_domain !== true
  ) {
    fail(
      `website/wrangler.jsonc: expected one ${PUBLIC_SCHEMA_ROUTE} custom domain for public schema $id URLs`,
    );
  }
}

const standardSetPath = path.join(
  repositoryRoot,
  "forms",
  "standard-package-set.json",
);
const standardSet = readJson(standardSetPath);
if (standardSet.publicationReady !== false) {
  fail("forms/standard-package-set.json: publicationReady must be false");
}
if (standardSet.admissionStatus !== "external-required") {
  fail(
    "forms/standard-package-set.json: admissionStatus must be external-required",
  );
}

const legacyPackages = Array.isArray(standardSet.packages)
  ? standardSet.packages
  : [];
if (legacyPackages.length === 0) {
  fail(
    "forms/standard-package-set.json: packages must not be empty",
  );
}

const legacyKinds = legacyPackages.map((entry, index) => {
  const kind = entry.formRef?.kind;
  if (entry.admissionStatus !== "external-required") {
    fail(
      `forms/standard-package-set.json: packages[${index}].admissionStatus must be external-required`,
    );
  }
  if (typeof kind !== "string" || kind === "" || entry.kind !== kind) {
    fail(
      `forms/standard-package-set.json: packages[${index}] kind/formRef.kind must match`,
    );
  }
  return typeof kind === "string" ? kind : "";
});

const retainedProviderV2SetPath = path.join(
  repositoryRoot,
  "forms",
  "candidates",
  "v1alpha2",
  "candidate-set.json",
);
const retainedProviderV2Set = readJson(retainedProviderV2SetPath);
if (
  retainedProviderV2Set.format !== "takoform.current-form-candidates@v2" ||
  retainedProviderV2Set.formApiVersion !== "forms.takoform.com/v1alpha2" ||
  retainedProviderV2Set.packageApiVersion !== "packages.forms.takoform.com/v1alpha3" ||
  retainedProviderV2Set.authoringSource !== "internal/currentformcatalog" ||
  retainedProviderV2Set.authoringPolicy !== "independent-semantic-contract" ||
  retainedProviderV2Set.publicationStatus !== "unpublished" ||
  retainedProviderV2Set.lifecycleAuthority !== "forms/lifecycle.json" ||
  Object.hasOwn(retainedProviderV2Set, "classification") ||
  Object.hasOwn(retainedProviderV2Set, "targetLifecycleState") ||
  Object.hasOwn(retainedProviderV2Set, "publicationReady")
) {
  fail("forms/candidates/v1alpha2/candidate-set.json: invalid retained provider-v2 candidate boundary");
}
const retainedProviderV2Entries = Array.isArray(retainedProviderV2Set.forms)
  ? retainedProviderV2Set.forms
  : [];
if (retainedProviderV2Entries.length !== 9) {
  fail(`forms/candidates/v1alpha2/candidate-set.json: forms must contain exactly 9 independently authored candidates`);
}

const forms = retainedProviderV2Entries.map((entry, index) => {
  const pathValue = typeof entry.path === "string" ? entry.path : "";
  const slug = path.posix.basename(pathValue);
  const kind = entry.formRef?.kind;
  const version = entry.formRef?.definitionVersion;
  if (
    entry.formRef?.apiVersion !== "forms.takoform.com/v1alpha2" ||
    version !== "0.1.0"
  ) {
    fail(
      `forms/candidates/v1alpha2/candidate-set.json: forms[${index}] must be retained provider-v2 definition 0.1.0`,
    );
  }
  if (typeof kind !== "string" || kind === "" || entry.kind !== kind) {
    fail(
      `forms/candidates/v1alpha2/candidate-set.json: forms[${index}] kind/formRef.kind must match`,
    );
  }
	if (typeof entry.proposalId !== "string" || entry.proposalId === "") {
	  fail(
		`forms/candidates/v1alpha2/candidate-set.json: forms[${index}] has no Proposal identity`,
	  );
	}
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)) {
    fail(
      `forms/candidates/v1alpha2/candidate-set.json: forms[${index}] has invalid Form slug ${JSON.stringify(slug)}`,
    );
  }
  if (
    typeof version !== "string" ||
    !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)
  ) {
    fail(
      `forms/candidates/v1alpha2/candidate-set.json: forms[${index}] has invalid definitionVersion ${JSON.stringify(version)}`,
    );
  }
  return {
    docName: slug.replaceAll("-", "_"),
    kind: typeof kind === "string" ? kind : "",
    slug,
    version: typeof version === "string" ? version : "",
  };
});
compareExact(
  "retained provider-v2 Form slugs",
  forms.map(({ slug }) => slug),
  new Set(forms.map(({ slug }) => slug)),
);
compareExact(
  "retained provider-v2 Form kinds",
  forms.map(({ kind }) => kind),
  new Set(forms.map(({ kind }) => kind)),
);

const releaseVersion = readJson(
  path.join(repositoryRoot, "release", "version.json"),
);
if (releaseVersion.publicationStatus !== "candidate-only") {
  fail("release/version.json: publicationStatus must be candidate-only");
}
if (
  typeof releaseVersion.version !== "string" ||
  !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(
    releaseVersion.version,
  )
) {
  fail("release/version.json: version must be exact SemVer");
}
if (releaseVersion.tag !== `v${releaseVersion.version}`) {
  fail("release/version.json: tag must match version");
}

let publicationTruth = null;
try {
  publicationTruth = loadPublicationTruth(repositoryRoot);
} catch (error) {
  fail(`retained publication truth: ${error.message}`);
}
if (publicationTruth !== null) {
  compareExact(
    "published Form Package kinds",
    publicationTruth.publishedKinds,
    legacyKinds,
  );
}

// The Host API v1beta1 channel: exactly the fifteen Edge Platform Family
// resources, all rendered by internal/standardforms (doc name = resource type
// minus the takoform_ prefix). They share the stable v2.1.1 release-target example
// pin. There is deliberately no generic takoform_resource carrier: the lane
// exposes no resource that is not a Form (spec/decisions/0021), so this exact
// set is also the assertion that the carrier has not come back.
//
// Each column is bound to repository data below: the kinds and slugs to the
// family candidate set, the doc names to docs/resources, the slugs to
// proposals/edge. A Form cannot enter or leave the family without this table
// moving, and the table is what every hand-written inventory is measured
// against.
const edgeFamilyRoster = [
  { kind: "ModuleWorker", slug: "module-worker", docName: "module_worker" },
  { kind: "WorkerBundle", slug: "worker-bundle", docName: "worker_bundle" },
  {
    kind: "StaticAssetBundle",
    slug: "static-asset-bundle",
    docName: "static_asset_bundle",
  },
  { kind: "WorkerVersion", slug: "worker-version", docName: "worker_version" },
  {
    kind: "WorkerDeployment",
    slug: "worker-deployment",
    docName: "worker_deployment",
  },
  {
    kind: "WorkerCustomDomain",
    slug: "worker-custom-domain",
    docName: "worker_custom_domain",
  },
  { kind: "WorkerEndpoint", slug: "worker-endpoint", docName: "worker_endpoint" },
  {
    kind: "WorkerCronTrigger",
    slug: "worker-cron-trigger",
    docName: "worker_cron_trigger",
  },
  {
    kind: "EdgeKVNamespace",
    slug: "edge-kv-namespace",
    docName: "edge_kv_namespace",
  },
  { kind: "ObjectBucket", slug: "object-bucket", docName: "edge_object_bucket" },
  { kind: "SQLiteDatabase", slug: "sqlite-database", docName: "sqlite_database" },
  {
    kind: "SQLiteMigrationSet",
    slug: "sqlite-migration-set",
    docName: "sqlite_migration_set",
  },
  {
    kind: "SQLiteMigrationApplication",
    slug: "sqlite-migration-application",
    docName: "sqlite_migration_application",
  },
  {
    kind: "AtLeastOnceQueue",
    slug: "at-least-once-queue",
    docName: "at_least_once_queue",
  },
  { kind: "QueueConsumer", slug: "queue-consumer", docName: "queue_consumer" },
];
const edgeFamilyDocNames = edgeFamilyRoster.map(({ docName }) => docName);

const familySet = readJson(path.join(repositoryRoot, FAMILY_CANDIDATE_SET));
const familyEntries = Array.isArray(familySet.forms) ? familySet.forms : [];
compareExact(
  "Edge Platform Family kinds",
  familyEntries.map((entry) => entry?.kind ?? ""),
  edgeFamilyRoster.map(({ kind }) => kind),
);
compareExact(
  "Edge Platform Family Form slugs",
  familyEntries.map((entry) =>
    path.posix.basename(typeof entry?.path === "string" ? entry.path : ""),
  ),
  edgeFamilyRoster.map(({ slug }) => slug),
);

const formDocNames = [...forms.map(({ docName }) => docName), ...edgeFamilyDocNames];
const expectedResourceDocs = formDocNames.map((name) => `${name}.md`);
const docsResourceDirectory = path.join(repositoryRoot, "docs", "resources");
const docsResourceEntries = directoryEntries(docsResourceDirectory);
compareExact(
  "docs/resources",
  docsResourceEntries.map(({ name }) => name),
  expectedResourceDocs,
);
for (const entry of docsResourceEntries) {
  if (!entry.isFile()) {
    fail(`docs/resources/${entry.name}: expected a regular file`);
  }
}

const expectedExampleDirectories = formDocNames.map(
  (name) => `takoform_${name}`,
);
const exampleResourceDirectory = path.join(
  repositoryRoot,
  "examples",
  "resources",
);
const expectedResourceExampleFiles = expectedExampleDirectories.map(
  (directoryName) => `${directoryName}/resource.tf`,
);
const resourceExampleFiles = walkFiles(exampleResourceDirectory);
compareExact(
  "examples/resources files",
  resourceExampleFiles.map((filePath) =>
    path.relative(exampleResourceDirectory, filePath).split(path.sep).join("/"),
  ),
  expectedResourceExampleFiles,
);
const edgeFamilyExampleDirectories = new Set(
  edgeFamilyDocNames.map((name) => `takoform_${name}`),
);
for (const example of resourceExampleFiles) {
  const directoryName = path
    .relative(exampleResourceDirectory, example)
    .split(path.sep)[0];
  if (edgeFamilyExampleDirectories.has(directoryName)) {
    checkEdgeFamilyProviderExample(example);
  } else {
    checkTerraformProviderExample(example, publicationTruth?.providerVersion ?? "");
  }
}

const dataSourceDocs = path.join(repositoryRoot, "docs", "data-sources");
const dataSourceDocEntries = directoryEntries(dataSourceDocs);
compareExact(
  "docs/data-sources",
  dataSourceDocEntries.map(({ name }) => name),
  ["interface.md"],
);
for (const entry of dataSourceDocEntries) {
  if (!entry.isFile()) {
    fail(`docs/data-sources/${entry.name}: expected a regular file`);
  }
}
const dataSourceExamples = path.join(
  repositoryRoot,
  "examples",
  "data-sources",
);
const dataSourceExampleFiles = walkFiles(dataSourceExamples);
compareExact(
  "examples/data-sources files",
  dataSourceExampleFiles.map((filePath) =>
    path.relative(dataSourceExamples, filePath).split(path.sep).join("/"),
  ),
  ["takoform_interface/data-source.tf"],
);
for (const example of dataSourceExampleFiles) {
  checkTerraformProviderExample(example, publicationTruth?.providerVersion ?? "");
}

const docsIndexPath = path.join(repositoryRoot, "docs", "index.md");
const docsIndexTargets = markdownTargets(read(docsIndexPath))
  .map((target) => resolveMarkdownTarget(docsIndexPath, target))
  .filter((target) => target !== null)
  .map((target) => path.resolve(target));
for (const docName of formDocNames) {
  const expected = path.resolve(
    repositoryRoot,
    "docs",
    "resources",
    `${docName}.md`,
  );
  if (!docsIndexTargets.includes(expected)) {
    fail(`docs/index.md: missing link to resources/${docName}.md`);
  }
}
const interfaceDataSourceDoc = path.resolve(
  repositoryRoot,
  "docs",
  "data-sources",
  "interface.md",
);
if (!docsIndexTargets.includes(interfaceDataSourceDoc)) {
  fail("docs/index.md: missing link to data-sources/interface.md");
}

checkMarkdownLinks();
checkWebsiteMarkdownLinks();
checkHtmlFiles();
checkResourceInventory(formDocNames);
checkDocsPageLinks(formDocNames);
checkStaleWebsiteContent();
checkPublishedProviderExamples(publicationTruth);
checkRetainedPublicationTruthCopy(publicationTruth);
checkContractLaneDocumentation(retainedProviderV2Set);
checkCurrentLaneSemanticResidue();
checkSingleRegistryVocabulary();
checkProviderReleaseCommitBindings();
checkPublicSchemas();
checkWebsiteDocsProjection(formDocNames);
checkHandWrittenInventories(forms, edgeFamilyRoster);
checkHostApiLanesAreMintedForAReason();
checkPackageEnvelopesAreMintedForAReason();
checkDocumentedWalkIsRunnable();
checkCorpusNamesStateTheirLane();
checkConformanceCorpusCounts();
for (const failure of verifySiteStatusDocument(repositoryRoot, publicationTruth)) {
  fail(failure);
}

if (failures.length > 0) {
  console.error(
    `Public surface check failed with ${failures.length} problem${failures.length === 1 ? "" : "s"}:`,
  );
  for (const failure of failures) {
    console.error(`- ${failure}`);
  }
  process.exitCode = 1;
} else {
  console.log(
    `Public surfaces OK: provider v${publicationTruth.providerVersion}, ` +
      `${publicationTruth.publishedCount} published Legacy Form Packages, ` +
      `no current central admission, interface data ` +
      "source, docs, examples, website links, and normative schema URLs " +
      "are consistent.",
  );
}

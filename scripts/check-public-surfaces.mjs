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

function extractLocalizedSections(source) {
  const sections = new Map();
  const openings = [];
  const tagPattern = /<([a-z][a-z0-9:-]*)\b([^>]*)>/gi;
  for (const match of source.matchAll(tagPattern)) {
    const className = htmlAttribute(match[2], "class") ?? "";
    const lang = htmlAttribute(match[2], "lang");
    if (className.split(/\s+/).includes("l10n") && ["en", "ja"].includes(lang)) {
      openings.push({ index: match.index, lang });
    }
  }

  openings.sort((left, right) => left.index - right.index);
  for (let index = 0; index < openings.length; index += 1) {
    const opening = openings[index];
    const end = openings[index + 1]?.index ?? source.length;
    if (sections.has(opening.lang)) {
      fail(`website/public/index.html: duplicate ${opening.lang} .l10n section`);
    }
    sections.set(opening.lang, source.slice(opening.index, end));
  }
  return sections;
}

function resourceCards(source) {
  const resources = [];
  const cardPattern =
    /<(?:div|article|li)\b([^>]*)>\s*<code\b[^>]*>\s*takoform_([a-z0-9_]+)\s*<\/code>/gi;
  for (const match of source.matchAll(cardPattern)) {
    const className = htmlAttribute(match[1], "class") ?? "";
    const classes = className.split(/\s+/);
    if (!classes.includes("resource") && !classes.includes("resource-card")) {
      continue;
    }
    resources.push(match[2]);
  }
  return resources;
}

function checkLocalizedResourceCards(expectedFormNames) {
  const indexPath = path.join(publicRoot, "index.html");
  const sections = extractLocalizedSections(read(indexPath));
  for (const lang of ["en", "ja"]) {
    if (!sections.has(lang)) {
      fail(`website/public/index.html: missing ${lang} .l10n section`);
      continue;
    }
    compareExact(
      `website/public/index.html ${lang} resource cards`,
      resourceCards(sections.get(lang)),
      expectedFormNames,
    );
  }
}

function hrefTargetsDocumentation(href, directory, docName) {
  const normalized = href.split(/[?#]/, 1)[0].replace(/\\/g, "/");
  const suffix = `/docs/${directory}/${docName}`;
  return (
    normalized.endsWith(`${suffix}.md`) ||
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

function checkStaleWebsiteContent() {
  const htmlFiles = walkFiles(publicRoot, (filePath) =>
    filePath.endsWith(".html"),
  );
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
      label: "publication-pending wording",
      pattern:
        /\buntil\s+(?:the\s+)?(?:provider\s+)?publication\b|\bpublication[^.]{0,120}\b(?:pending|does not exist|do not exist|not yet)\b/i,
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
  if (hasNotInstallableWording(source)) {
    fail(
      "docs/index.md: immutable provider tag docs contain temporary not-installable wording",
    );
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

function checkPublishedProviderExamples(truth) {
  if (truth === null) {
    return;
  }
  const docsIndex = path.join(repositoryRoot, "docs", "index.md");
  const docsSource = read(docsIndex);
  const docsBlocks = providerCodeBlocksFromMarkdown(docsSource);
  if (docsBlocks.length === 0) {
    fail("docs/index.md: missing provider source example");
  }
  for (const [index, block] of docsBlocks.entries()) {
    if (!hasExactProviderPin(block, truth.providerVersion)) {
      fail(
        `docs/index.md: provider example ${index + 1} must contain version ` +
          `= "= ${truth.providerVersion}"`,
      );
    }
  }
  checkImmutableProviderTagDocs(docsSource, truth);

  const htmlFiles = walkFiles(publicRoot, (filePath) =>
    filePath.endsWith(".html"),
  );
  for (const filePath of htmlFiles) {
    const source = read(filePath);
    const blocks = providerCodeBlocksFromHtml(source);
    if (blocks.length === 0) {
      continue;
    }
    for (const [index, block] of blocks.entries()) {
      if (!hasExactProviderPin(block, truth.providerVersion)) {
        fail(
          `${relative(filePath)}: provider example ${index + 1} must contain ` +
            `version = "= ${truth.providerVersion}"`,
        );
      }
    }
    const visibleText = visibleHtmlText(source);
    if (hasNotInstallableWording(visibleText)) {
      fail(`${relative(filePath)}: contains stale not-installable wording`);
    }
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
      ["admission identity", truth.admissionTag],
      ["portable-standard status", "portable-standard"],
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
    if (!/\bRegistry\b/.test(text)) {
      fail(`${relative(filePath)}: missing canonical Registry evidence boundary`);
    }
  }

  const apiTruthFiles = truthFiles.filter(
    (filePath) => filePath !== path.join(repositoryRoot, "SECURITY.md"),
  );
  for (const filePath of apiTruthFiles) {
    if (
      !(textByFile.get(filePath) ?? "").includes(
        "forms.takoform.com/v1alpha1",
      )
    ) {
      fail(`${relative(filePath)}: missing v1alpha1 API boundary`);
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
    if (descriptorFiles.has(filePath)) {
      for (const match of text.matchAll(/\bcandidate-only\b/gi)) {
        const context = text.slice(
          Math.max(0, match.index - 220),
          Math.min(text.length, match.index + 320),
        );
        if (
          !/(?:release\/version\.json|source descriptor)/i.test(context) ||
          !/\bmetadata\b/i.test(context) ||
          !/\blive\b/i.test(context)
        ) {
          fail(
            `${relative(filePath)}: candidate-only appears outside the ` +
              "release descriptor metadata explanation",
          );
        }
      }
    }
  }

  for (const filePath of [
    path.join(repositoryRoot, "README.md"),
    path.join(repositoryRoot, "docs", "index.md"),
    path.join(publicRoot, "docs", "index.html"),
  ]) {
    const text = textByFile.get(filePath) ?? "";
    for (const kind of truth.admittedKinds) {
      if (!text.includes(kind)) {
        fail(`${relative(filePath)}: missing admitted Form ${kind}`);
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

function checkSingleRegistryVocabulary() {
  const historicalAddressGuides = new Set([
    path.join(repositoryRoot, "release", "README.md"),
    path.join(
      repositoryRoot,
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

const packages = Array.isArray(standardSet.packages)
  ? standardSet.packages
  : [];
if (packages.length === 0) {
  fail(
    "forms/standard-package-set.json: packages must not be empty",
  );
}

const forms = packages.map((entry, index) => {
  const pathValue = typeof entry.path === "string" ? entry.path : "";
  const slug = path.posix.basename(pathValue);
  const kind = entry.formRef?.kind;
  const version = entry.formRef?.definitionVersion;
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
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)) {
    fail(
      `forms/standard-package-set.json: packages[${index}] has invalid Form slug ${JSON.stringify(slug)}`,
    );
  }
  if (
    typeof version !== "string" ||
    !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)
  ) {
    fail(
      `forms/standard-package-set.json: packages[${index}] has invalid definitionVersion ${JSON.stringify(version)}`,
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
  "forms/standard-package-set.json Form slugs",
  forms.map(({ slug }) => slug),
  new Set(forms.map(({ slug }) => slug)),
);
compareExact(
  "forms/standard-package-set.json Form kinds",
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
    forms.map(({ kind }) => kind),
  );
}

const formDocNames = forms.map(({ docName }) => docName);
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
for (const example of resourceExampleFiles) {
  checkTerraformProviderExample(
    example,
    publicationTruth?.providerVersion ?? releaseVersion.version,
  );
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
  checkTerraformProviderExample(
    example,
    publicationTruth?.providerVersion ?? releaseVersion.version,
  );
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
checkHtmlFiles();
checkLocalizedResourceCards(formDocNames);
checkDocsPageLinks(formDocNames);
checkStaleWebsiteContent();
checkPublishedProviderExamples(publicationTruth);
checkRetainedPublicationTruthCopy(publicationTruth);
checkSingleRegistryVocabulary();
checkProviderReleaseCommitBindings();
checkPublicSchemas();

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
      `${publicationTruth.publishedCount} published Form Packages, ` +
      `${publicationTruth.admittedCount} admitted Forms, interface data ` +
      "source, docs, examples, website links, and normative schema URLs " +
      "are consistent.",
  );
}

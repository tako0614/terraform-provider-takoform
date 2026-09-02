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
import { verifySiteStatusDocument } from "./site-status.mjs";
import {
  RELEASE_STATE_NEUTRAL_SOURCE_PATHS,
  staleSpecificationReleaseWording,
} from "./specification-release.mjs";
import {
  FAMILY_CANDIDATE_SET,
  PROVIDER_REGISTRY_PUBLISHED_VERSION,
  deriveSiteStatusFacts,
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
  const familyLabels = new Map([
    ["edge.forms.takoform.com", "Edge"],
    ["function.forms.takoform.com", "Function"],
    ["container.forms.takoform.com", "Container"],
    ["queue.forms.takoform.com", "Queue"],
    ["schedule.forms.takoform.com", "Schedule"],
    ["table.forms.takoform.com", "Table"],
    ["topic.forms.takoform.com", "Topic"],
    ["vector.forms.takoform.com", "Vector"],
  ]);
  const familyCounts = new Map();
  for (const { group } of currentFormRoster) {
    familyCounts.set(group, (familyCounts.get(group) ?? 0) + 1);
  }
  for (const [page, label] of [
    ["index.html", "website/public/index.html English resource counts"],
    [
      "ja/index.html",
      "website/public/ja/index.html Japanese resource counts",
    ],
  ]) {
    const text = visibleHtmlText(read(path.join(publicRoot, page)));
    if (!text.includes("Provider 3") || !text.includes("31")) {
      fail(`${label}: missing the retained Provider 3 aggregate count`);
    }
    for (const [group, count] of familyCounts) {
      const family = familyLabels.get(group) ?? group;
      if (!new RegExp(`${family}\\s+${count}\\b`, "u").test(text)) {
        fail(`${label}: missing ${family} count ${count}`);
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

function checkDocsPageLinks(expectedResourceDocNames, expectedFormRoster) {
  const docsPage = path.join(publicRoot, "docs", "index.html");
  const specPage = path.join(publicRoot, "spec", "index.html");
  const formsPage = path.join(publicRoot, "forms", "index.html");
  for (const filePath of [docsPage, formsPage, specPage]) {
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

  if (!existsSync(formsPage)) {
    return;
  }
  const formsText = visibleHtmlText(read(formsPage));
  for (const { kind } of expectedFormRoster) {
    if (!formsText.includes(kind)) {
      fail(
        `${relative(formsPage)}: missing Form inventory row for ${kind}`,
      );
    }
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

function checkSpecificationReleaseWording() {
  for (const relativePath of RELEASE_STATE_NEUTRAL_SOURCE_PATHS) {
    const source = read(path.join(repositoryRoot, relativePath));
    if (staleSpecificationReleaseWording(source)) {
      fail(
        `${relativePath}: current Specification wording hard-codes 1.1 as candidate/open`,
      );
    }
  }

  const ledger = readJson(
    path.join(repositoryRoot, "release", "specification-releases.json"),
  );
  const released = Array.isArray(ledger.releases) &&
    ledger.releases.some((entry) => entry?.version === "1.1");
  if (!released) return;
  const servedPages = walkFiles(publicRoot, (filePath) =>
    filePath.endsWith(".html"),
  );
  for (const filePath of servedPages) {
    const relativePath = relative(filePath);
    const source = visibleHtmlText(read(filePath));
    if (staleSpecificationReleaseWording(source)) {
      fail(
        `${relativePath}: served current page still calls Specification 1.1 candidate/open after its receipt`,
      );
    }
  }
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function checkCurrentProviderSample(filePath) {
  const source = read(filePath);
  if (!source.includes("registry.terraform.io/tako0614/takoform")) {
    fail(`${relative(filePath)}: missing canonical provider source`);
    return;
  }
  if (!/\bversion\s*=\s*"~> 4\.0"/.test(source)) {
    fail(
      `${relative(filePath)}: publisher-specific next-major Provider sample must contain version = "~> 4.0"`,
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

// The Provider reference in docs/ is rendered into the Registry's own immutable
// documentation for the published version, so it can never be corrected after
// the fact: it must carry the verification path for the exact published tag and
// must not carry a status the publication has already falsified. The same
// falsified status is refused on the hand-written landing and reference pages
// in both languages, which are the surfaces a reader reaches first.
const PUBLISHED_INSTALL_PROSE = [
  "README.md",
  "docs/index.md",
  "website/index.md",
  "website/ja/index.md",
  "website/docs/index.md",
  "website/ja/docs/index.md",
];

function checkPublishedProviderInstallDocs(providerVersion) {
  checkImmutableProviderTagDocs(
    read(path.join(repositoryRoot, "docs", "index.md")),
    { providerVersion },
  );
  for (const relativePath of PUBLISHED_INSTALL_PROSE) {
    const source = read(path.join(repositoryRoot, relativePath));
    if (hasNotInstallableWording(source)) {
      fail(
        `${relativePath}: says the published Provider cannot be installed from the Registry`,
      );
    }
  }
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

function checkContractLaneDocumentation() {
  // The identities every lane-narrating document must still state. The
  // withdrawn epochs' identities are deliberately NOT required any more: a
  // document may mention them as history, but nothing forces it to, and the
  // current identities must always be present.
  const currentHostWire = "forms.takoform.com/v1";
  const currentFamily = "edge.forms.takoform.com";
  const currentPackage = "packages.forms.takoform.com/v1alpha5";
  const retainedHostWire = "forms.takoform.com/v1beta1";
  const retainedFamily = "edge.forms.takoform.com/v1beta1";
  const retainedPackage = "packages.forms.takoform.com/v1alpha4";
  const documents = [
    {
      file: path.join(repositoryRoot, "spec", "README.md"),
      required: [currentHostWire, currentFamily, currentPackage, "/.well-known/takoform/v1", retainedHostWire],
    },
    {
      file: path.join(repositoryRoot, "spec", "form-definition", "README.md"),
      required: [currentHostWire, currentFamily, "form-definition-v1.schema.json", retainedFamily],
    },
    {
      file: path.join(repositoryRoot, "spec", "form-package", "README.md"),
      required: [currentPackage, "package-index-v1alpha5.schema.json", retainedPackage],
    },
    {
      file: path.join(repositoryRoot, "spec", "versioning.md"),
      required: [currentHostWire, currentFamily, currentPackage, "/.well-known/takoform/v1", retainedHostWire, retainedFamily, retainedPackage],
    },
    {
      file: path.join(repositoryRoot, "release", "README.md"),
      required: [currentHostWire, currentFamily, currentPackage, retainedHostWire, retainedFamily, retainedPackage],
    },
    {
      file: path.join(repositoryRoot, "proposals", "README.md"),
      required: ["forms/candidates/edge.forms.takoform.com/candidate-set.json"],
    },
  ];

  for (const { file, required } of documents) {
    const source = read(file);
    const normalized = source.replace(/\s+/gu, " ");
    for (const text of required) {
      if (!normalized.includes(text.replace(/\s+/gu, " "))) {
        fail(`${relative(file)}: contract-lane documentation is missing ${JSON.stringify(text)}`);
      }
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
    "internal/currentformselection",
    "internal/currentformsnapshot",
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
  const retainedCodeFiles = new Set();
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
    "Provider release identity ledger",
    "migrations/v2-to-v3.md",
    "31 typed resources across eight versionless families",
    "The signed release-tag commit must be an ancestor of the reviewed",
    "protected-main/readback commit used for the release.",
  ]) {
    if (!releaseGuide.includes(required)) {
      fail(
        `release/README.md: Provider release guide is missing ${JSON.stringify(required)}`,
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
}

// Every hand-written inventory of the resource set, and what each one must say
// about a Form for a reader to be able to find it.
//
// The generated trees (docs/resources, examples/resources, the site
// projections) were already compared against the roster derived from the
// selected artifact index plus Provider projection; nothing checked the prose
// and navigation, which is exactly why WorkerEndpoint reached production
// missing from the README, sidebar and Japanese docs index while every
// generated surface carried it. A Form now cannot be added or removed without
// every one of these learning about it.
function checkHandWrittenInventories(familyRoster) {
  const familyKinds = familyRoster.map(({ kind }) => kind);
  const familyDocNames = familyRoster.map(({ docName }) => docName);
  const familySlugs = familyRoster.map(({ slug }) => slug);

  const resourceDocLinks = (names) =>
    names.map((name) => ({
      needle: `/docs/resources/${name}.html`,
      subject: name,
    }));

  const inventories = [
    {
      file: "website/.vitepress/config.mts",
      label: "the resource sidebar",
      required: resourceDocLinks(familyDocNames),
    },
    {
      file: "website/docs/index.md",
      label: "the English resource reference",
      required: resourceDocLinks(familyDocNames),
    },
    {
      file: "website/forms/index.md",
      label: "the Provider mapping inventory",
      required: familyKinds.map((kind) => ({
        needle: `\`${kind}\``,
        subject: kind,
      })),
    },
    {
      file: "website/ja/docs/index.md",
      label: "the Japanese resource reference",
      required: resourceDocLinks(familyDocNames),
    },
    {
      file: "website/index.md",
      label: "the English landing inventory",
      required: [
        { needle: "**`4.0.0`**", subject: "current Provider" },
        { needle: "| Edge | 17 |", subject: "Edge count" },
        { needle: "Provider reference", subject: "Provider reference" },
      ],
    },
    {
      file: "website/ja/index.md",
      label: "the Japanese landing inventory",
      required: [
        { needle: "**`4.0.0`**", subject: "current Provider" },
        { needle: "| Edge | 17 |", subject: "Edge count" },
        { needle: "Provider reference", subject: "Provider reference" },
      ],
    },
    {
      file: "forms/README.md",
      label: "the generated Provider mapping inventory",
      required: familyKinds.map((kind) => ({
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
// cannot rot that way, so a NEW corpus must carry one. The two
// generation-named corpora that used to be listed here as retained history
// (portable-host-v1 and -v2) were withdrawn with their epochs (decision 0042).
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
  // The three pre-Beta lanes were withdrawn with their epochs (decision 0042):
  // their schemas, corpora and served documents are gone, and their identities
  // are recorded as retired in the published ledgers so they can never be
  // reused meaning something else. The mint reasons stay as history.
  ["forms.takoform.com/v1alpha1", {
    mintedFor: "protocol",
    withdrawn: "the Legacy v1alpha1 epoch was withdrawn before Beta (decision 0042)",
  }],
  ["forms.takoform.com/v1alpha2", {
    mintedFor: "protocol",
    withdrawn: "the v1alpha2 epoch was withdrawn before Beta (decision 0042)",
  }],
  ["forms.takoform.com/v1alpha3", {
    mintedFor: "protocol",
    withdrawn: "the v1alpha3 identities were withdrawn with the v1alpha2 epoch that served them (decision 0042)",
  }],
  ["forms.takoform.com/v1", {
    wireSchema: "spec/schemas/host-api-wire-v1.schema.json",
    document: "spec/host-api/v1.md",
    mintedFor: "graduation",
    evidence: "exact committed normative Specification 1.1 source (decision 0057); candidate and reference evidence are non-authoritative",
  }],
  ["forms.takoform.com/v1beta4", {
    wireSchema: "spec/schemas/host-api-wire-v1beta4.schema.json",
    document: "spec/host-api/v1beta4.md",
    // The claim this lane makes about itself, and the reason it may be
    // checked: its rules are mechanisms a family instantiates rather than
    // paragraphs about particular Forms.
    namesNoFormKind: true,
    // Minted for a structural wire change: a Form Family group may omit its
    // version segment (decision 0049), which moves the FormRef grammar every
    // request and response carries and the shape of every path that names a
    // group. It also carries what the two withdrawn lanes were minted for.
    mintedFor: "protocol",
  }],
  // Minted and then withdrawn before either was ever served (decision 0051).
  // Neither has bytes on disk, and their identities stay here so the names can
  // never be reused meaning something else.
  ["forms.takoform.com/v1beta2", {
    mintedFor: "protocol",
    withdrawn: "never served; the Beta 2 hardening review's wire changes are v1beta4's (decision 0051)",
  }],
  ["forms.takoform.com/v1beta3", {
    mintedFor: "protocol",
    withdrawn: "never served; the declared mechanisms are v1beta4's (decision 0051)",
  }],
  ["forms.takoform.com/v1beta1", {
    wireSchema: "spec/schemas/host-api-wire-v1beta1.schema.json",
    mintedFor: "graduation",
    // Recorded as it happened rather than as it should have. This lane was
    // minted alongside the Edge family channel move in #132, and its wire
    // contract was structurally identical to v1alpha3's — measured in decision
    // 0038, before v1alpha3's bytes were withdrawn. No lane-specific evidence
    // was stated, which is exactly what the rule above now requires. It is
    // frozen into Registry-published provider v2.1.1 and cannot be withdrawn;
    // the entry stands as the record.
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
  // The first three envelopes were withdrawn with the epochs whose FormRefs
  // they wrapped (decision 0042). Their mint reasons stay as history: one real
  // format change (v1alpha1 -> v1alpha2 introduced content addressing) and one
  // rename that followed a FormRef grammar.
  ["packages.forms.takoform.com/v1alpha1", {
    mintedFor: "format",
    withdrawn: "withdrawn with the Legacy v1alpha1 epoch (decision 0042)",
  }],
  ["packages.forms.takoform.com/v1alpha2", {
    mintedFor: "format",
    withdrawn: "withdrawn with the v1alpha2 epoch (decision 0042)",
  }],
  ["packages.forms.takoform.com/v1alpha3", {
    mintedFor: "carried",
    evidence: "re-minted for the retained provider-v2 FormRef grammar; format unchanged (decision 0040)",
    withdrawn: "withdrawn with the v1alpha2 epoch (decision 0042)",
  }],
  ["packages.forms.takoform.com/v1alpha4", {
    schema: "spec/schemas/package-index-v1alpha4.schema.json",
    mintedFor: "carried",
    evidence: "re-minted for the namespaced family FormRef grammar; format unchanged, and it carried two family generations (decision 0040)",
  }],
  ["packages.forms.takoform.com/v1alpha5", {
    schema: "spec/schemas/package-index-v1alpha5.schema.json",
    mintedFor: "carried",
    evidence: "re-minted because the family group stopped carrying a version segment and v1alpha4's FormRef reference requires one; format unchanged (decisions 0040 and 0049)",
  }],
]);

function checkVersionedIdentitiesAreMintedForAReason(label, table, kinds) {
  const shapes = new Map();
  for (const [identity, entry] of table) {
    if (typeof entry.withdrawn === "string") {
      // A withdrawn identity keeps its row so the name can never be reused,
      // and must keep no bytes: a schema still on disk would be the withdrawal
      // not having happened.
      if (entry.withdrawn.trim() === "") {
        fail(`${identity}: a withdrawn ${label} must say why it was withdrawn`);
      }
      if (entry.schema !== undefined) {
        fail(`${identity}: withdrawn, but still names schema ${entry.schema}`);
      }
      continue;
    }
    if (!existsSync(path.join(repositoryRoot, entry.schema))) {
      fail(`${identity}: schema ${entry.schema} does not exist`);
      continue;
    }
    shapes.set(identity, wireContractShape(entry.schema));
  }
  for (const [identity, entry] of table) {
    if (typeof entry.withdrawn === "string") continue;
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

// A lane marked `namesNoFormKind` is one whose rules are stated as mechanisms a
// family instantiates rather than as paragraphs about particular Forms. That
// claim is only worth making if it is checked, and it is checkable exactly:
// collect every Form kind of every family on disk and fail if the protocol
// document contains one.
//
// The earlier lanes are not held to it. They name Form kinds, they are
// published, and their bytes do not move — the point of the new lane is that
// the NEXT family change does not touch a protocol document, not that the old
// ones are rewritten.
function checkExtensibleLanesNameNoFormKind() {
  const kinds = new Map();
  const candidateRoot = path.join(repositoryRoot, "forms", "candidates");
  const families = [];
  for (const family of readdirSync(candidateRoot)) {
    const familyRoot = path.join(candidateRoot, family);
    if (!statSync(familyRoot).isDirectory()) continue;
    const currentSetPath = path.join("forms", "candidates", family, "candidate-set.json");
    if (existsSync(path.join(repositoryRoot, currentSetPath))) {
      families.push(currentSetPath);
    }
    for (const generation of readdirSync(familyRoot)) {
      const setPath = path.join("forms", "candidates", family, generation, "candidate-set.json");
      if (existsSync(path.join(repositoryRoot, setPath))) families.push(setPath);
    }
  }
  for (const setPath of families) {
    const set = readJson(path.join(repositoryRoot, setPath));
    for (const form of set.forms ?? []) {
      const kind = form?.formRef?.kind;
      if (typeof kind === "string" && kind !== "") kinds.set(kind, setPath);
    }
  }
  if (kinds.size === 0) {
    fail("no Form kinds were collected; the extensible-lane check would pass vacuously");
  }
  for (const [apiVersion, lane] of HOST_API_LANES) {
    if (lane.namesNoFormKind !== true) continue;
    const documentPath = lane.document;
    if (typeof documentPath !== "string" || !existsSync(path.join(repositoryRoot, documentPath))) {
      fail(`${apiVersion}: claims to name no Form kind but names no document to check`);
      continue;
    }
    const source = read(path.join(repositoryRoot, documentPath));
    const named = [];
    for (const [kind, setPath] of kinds) {
      if (new RegExp(`\\b${escapeRegExp(kind)}\\b`, "u").test(source)) {
        named.push(`${kind} (${setPath})`);
      }
    }
    if (named.length > 0) {
      fail(
        `${documentPath}: an extensible lane names ${named.length} Form kind(s) — ` +
          `${named.slice(0, 6).join(", ")}${named.length > 6 ? ", …" : ""}. ` +
          "A rule about one Form belongs to that Form's family, stated through a mechanism this lane declares.",
      );
    }
  }
}

function checkHostApiLanesAreMintedForAReason() {
  const shapes = new Map();
  for (const [apiVersion, lane] of HOST_API_LANES) {
    if (typeof lane.withdrawn === "string") {
      if (lane.withdrawn.trim() === "") {
        fail(`${apiVersion}: a withdrawn lane must say why it was withdrawn`);
      }
      if (lane.wireSchema !== undefined) {
        fail(`${apiVersion}: withdrawn, but still names wire schema ${lane.wireSchema}`);
      }
      continue;
    }
    if (!existsSync(path.join(repositoryRoot, lane.wireSchema))) {
      fail(`${apiVersion}: wire schema ${lane.wireSchema} does not exist`);
      continue;
    }
    shapes.set(apiVersion, wireContractShape(lane.wireSchema));
  }
  for (const [apiVersion, lane] of HOST_API_LANES) {
    if (typeof lane.withdrawn === "string") continue;
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
      if (other === apiVersion || shape !== otherShape) continue;
      // Identical bytes are usually a rename. They are not when the lane
      // changes what a conforming host must DO rather than what it puts on
      // the wire — a rule that used to be written per Form and is now derived
      // from a declaration moves no envelope member and is still a different
      // protocol to implement.
      //
      // The exception is tied to `namesNoFormKind`, which is CHECKED against
      // the document, so a lane can only claim it by actually being the thing
      // it claims. A lane asserting a difference nothing verifies would be
      // the rename this check exists to catch.
      // Either side of the pair may carry the exception: the two lanes are
      // the same bytes because ONE of them changed something else.
      if (lane.namesNoFormKind === true || HOST_API_LANES.get(other)?.namesNoFormKind === true) {
        continue;
      }
      fail(
        `${apiVersion}: minted for a protocol change, but its wire contract is ` +
          `structurally identical to ${other}; a lane that changes no bytes is a ` +
          `rename, and every client renegotiates for nothing`,
      );
      break;
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
// moment the corpus moves. When the conformance guide chooses to show one,
// bind it to the corpus; do not force historical counts into reader-facing
// prose merely because the machine can derive them.
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
  if (drivenKinds.length !== 16 || uniqueDrivenKinds.size !== 16) {
    fail(
      `${hostContractPath}: portable-host-v1beta1 must drive exactly 16 distinct ` +
        `Form probes, found ${drivenKinds.length} (${[...uniqueDrivenKinds].join(", ")})`,
    );
  }

  const familyCandidateSet = readJson(
    path.join(repositoryRoot, FAMILY_CANDIDATE_SET),
  );
  const familyKinds = Array.isArray(familyCandidateSet.forms)
    ? familyCandidateSet.forms.map((entry) => entry?.kind).filter(Boolean)
    : [];
  const unprobedKinds = familyKinds.filter((kind) => !uniqueDrivenKinds.has(kind));
  if (familyKinds.length !== 16 || unprobedKinds.length !== 0) {
    fail(
      `${FAMILY_CANDIDATE_SET}: retained portable-host-v1beta1 coverage must ` +
        `match the 16 current Edge Forms exactly (family=${familyKinds.length}, ` +
        `unprobed=${unprobedKinds.join(", ") || "none"})`,
    );
  }

  const schemaCoverageClaims = [
    ...text.matchAll(/pins each of those (\d+) Forms' DESIRED SCHEMA/gi),
  ];
  if (schemaCoverageClaims.length > 0) {
    for (const match of schemaCoverageClaims) {
      if (Number(match[1]) !== drivenKinds.length) {
        fail(
          `conformance/README.md: portable-host-v1beta1 schema coverage is written as ${match[1]}, ` +
            `but the runner drives ${drivenKinds.length} Forms`,
        );
      }
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

// release/version.json is the current Provider 4 release descriptor.
// release/history/provider-v3.0.0.json is the retained Provider 3 writer input
// and the only surviving copy of it, so it stays byte-stable and asserted.
// release/candidates/provider-v4.0.0.json is retained as the pre-publication
// candidate record and must remain byte-identical to the promoted descriptor.
const retainedProvider3Descriptor = readJson(
  path.join(repositoryRoot, "release", "history", "provider-v3.0.0.json"),
);
if (
  retainedProvider3Descriptor.version !== "3.0.0" ||
  retainedProvider3Descriptor.tag !== "v3.0.0" ||
  retainedProvider3Descriptor.publicationStatus !== "candidate-only"
) {
  fail(
    "release/history/provider-v3.0.0.json: retained Provider 3 writer input drifted",
  );
}
const releaseVersion = readJson(
  path.join(repositoryRoot, "release", "version.json"),
);
if (
  !readFileSync(
    path.join(repositoryRoot, "release", "version.json"),
  ).equals(
    readFileSync(
      path.join(
        repositoryRoot,
        "release",
        "candidates",
        "provider-v4.0.0.json",
      ),
    ),
  )
) {
  fail(
    "release/version.json: promoted descriptor is not byte-identical to release/candidates/provider-v4.0.0.json",
  );
}
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

// Provider 3's eight-family index and identity projection are immutable
// aggregate history. The current Provider source selects only the publisher
// Edge publication from that exact verified projection; the public-surface
// check derives Terraform names from the retained ledger instead of carrying
// another name map.
const currentFamilyIndex = readJson(
  path.join(repositoryRoot, "forms", "candidates", "current-family-index.json"),
);
const providerIdentityLedger = readJson(
  path.join(repositoryRoot, "release", "provider-form-identities.json"),
);
const currentProviderIdentity = readJson(
  path.join(
    repositoryRoot,
    "release",
    "candidates",
    "provider-v4.0.0-form-identities.json",
  ),
);
const publisherClosure = readJson(
  path.join(
    repositoryRoot,
    "internal",
    "provider",
    "artifacts",
    "publisher",
    "closure.json",
  ),
);
const publisherSlugByForm = new Map(
  (publisherClosure.packages ?? []).map((entry) => [
    `${entry?.formRef?.apiVersion ?? ""}\u0000${entry?.formRef?.kind ?? ""}`,
    path.posix.basename(typeof entry?.root === "string" ? entry.root : ""),
  ]),
);
if (
  currentProviderIdentity.format !==
    "takoform.provider-candidate-form-identities@v1" ||
  currentProviderIdentity.providerVersion !== releaseVersion.version
) {
  fail(
    `release/candidates/provider-v4.0.0-form-identities.json: missing Provider ${releaseVersion.version} identity projection`,
  );
}
const retainedProvider3Identity = (providerIdentityLedger.releases ?? []).find(
  (entry) => entry?.providerVersion === "3.0.0",
);
if (!retainedProvider3Identity) {
  fail("release/provider-form-identities.json: missing retained Provider 3 identity projection");
}
const retainedProvider3ResourceTypes = new Map(
  (retainedProvider3Identity.forms ?? []).map((entry) => [
    `${entry?.formRef?.apiVersion ?? ""}\u0000${entry?.formRef?.kind ?? ""}`,
    entry?.resourceType ?? "",
  ]),
);
const retainedAggregateRoster = (currentFamilyIndex.families ?? []).flatMap((family) => {
  const candidateSet = readJson(path.join(repositoryRoot, family.candidateSet ?? ""));
  return (candidateSet.forms ?? []).map((entry) => {
    const slug = path.posix.basename(typeof entry?.path === "string" ? entry.path : "");
    const resourceType = retainedProvider3ResourceTypes.get(
      `${candidateSet.family ?? ""}\u0000${entry?.kind ?? ""}`,
    );
    if (typeof resourceType !== "string" || !resourceType.startsWith("takoform_")) {
      fail(
        `release/provider-form-identities.json: missing exact Provider resource type for ${candidateSet.family ?? ""}/${entry?.kind ?? ""}`,
      );
    }
    return {
      group: candidateSet.family,
      kind: entry?.kind ?? "",
      slug,
      docName: resourceType.slice("takoform_".length),
    };
  });
});
if (currentFamilyIndex.families?.length !== 8 || retainedAggregateRoster.length !== 31) {
  fail(
    "forms/candidates/current-family-index.json: expected the exact 8-family, 31-Form current corpus",
  );
}
if (retainedProvider3ResourceTypes.size !== 31) {
  fail(
    `release/provider-form-identities.json: expected 31 retained Provider 3 resource mappings, got ${retainedProvider3ResourceTypes.size}`,
  );
}

const currentFormRoster = (currentProviderIdentity.forms ?? []).map((entry) => {
  const group = entry?.formRef?.apiVersion ?? "";
  const kind = entry?.formRef?.kind ?? "";
  const resourceType = entry?.resourceType ?? "";
  const slug = publisherSlugByForm.get(`${group}\u0000${kind}`) ?? "";
  if (
    group !== "edge.forms.takoform.com" ||
    kind === "" ||
    slug === "" ||
    typeof resourceType !== "string" ||
    !resourceType.startsWith("takoform_")
  ) {
    fail(
      `release/candidates/provider-v4.0.0-form-identities.json: malformed publisher mapping ${group}/${kind} -> ${resourceType}`,
    );
  }
  return {
    group,
    kind,
    slug,
    docName: resourceType.slice("takoform_".length),
  };
});
if (currentFormRoster.length !== 17) {
  fail(
    `publisher-selected Provider source: expected 17 exact Edge mappings, got ${currentFormRoster.length}`,
  );
}

const edgeFamilyRoster = currentFormRoster.filter(
  ({ group }) => group === "edge.forms.takoform.com",
);

const formDocNames = currentFormRoster.map(({ docName }) => docName);
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

const expectedExampleDirectories = currentFormRoster.map(({ docName }) =>
  `takoform_${docName}`,
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
  checkCurrentProviderSample(example);
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
checkMarkdownLinks();
checkWebsiteMarkdownLinks();
checkHtmlFiles();
checkResourceInventory(formDocNames);
checkDocsPageLinks(formDocNames, currentFormRoster);
checkStaleWebsiteContent();
checkSpecificationReleaseWording();
checkContractLaneDocumentation();
checkCurrentLaneSemanticResidue();
checkSingleRegistryVocabulary();
checkProviderReleaseCommitBindings();
checkPublishedProviderInstallDocs(PROVIDER_REGISTRY_PUBLISHED_VERSION);
checkPublicSchemas();
checkWebsiteDocsProjection(formDocNames);
checkHandWrittenInventories(edgeFamilyRoster);
checkHostApiLanesAreMintedForAReason();
checkExtensibleLanesNameNoFormKind();
checkPackageEnvelopesAreMintedForAReason();
checkDocumentedWalkIsRunnable();
checkCorpusNamesStateTheirLane();
checkConformanceCorpusCounts();
for (const failure of verifySiteStatusDocument(repositoryRoot)) {
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
  const siteStatus = deriveSiteStatusFacts(repositoryRoot);
  console.log(
    `Public surfaces OK: Specification 1.1 is ${siteStatus.specificationReleaseStatus}, ` +
      `the Registry-published Provider v${siteStatus.providerPublished} selects 17 tako0614 Edge Forms, Provider v3.0.0 ` +
      "retains the 31-Form Registry history, Provider v2.1.1 remains retained history, and docs, examples, website links, and normative schema URLs are consistent.",
  );
}

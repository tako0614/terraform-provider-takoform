import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitepress";

import { prepareSiteStatus, SITE_STATUS_ROUTE } from "./site-status.mjs";
import {
  CORE_LIBRARY_VERSION,
  HOST_API_LANE,
  JAPANESE_VERSION_MODEL_ROUTE,
  VERSION_MODEL_ROUTE,
} from "./version-model.mjs";

const github = "https://github.com/tako0614/terraform-provider-takoform";

// The published/preview facts are derived from the repository once, here, at
// build time. They reach the footer through themeConfig so every page states
// them as static HTML, and they reach machines through the JSON document this
// call normally writes. Every one of them is a pure function of committed
// repository bytes, so a fresh build reproduces the whole published tree; no
// commit id appears anywhere in it (see .vitepress/site-status.mjs).
const siteDirectory = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const snapshotReadOnlyFlag = process.env.TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY;
if (snapshotReadOnlyFlag !== undefined && snapshotReadOnlyFlag !== "1") {
  throw new Error(
    'TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY must be exactly "1" when set',
  );
}
const siteStatus = {
  ...prepareSiteStatus(siteDirectory, {
    write: snapshotReadOnlyFlag === undefined,
  }),
  route: SITE_STATUS_ROUTE,
};

const siteOrigin = "https://takoform.com";

const routeFromRelativePath = (relativePath: string) => {
  const normalized = relativePath.replaceAll("\\", "/").replace(/\.md$/, "");
  if (normalized === "index") return "/";
  if (normalized.endsWith("/index")) {
    return `/${normalized.slice(0, -"/index".length)}/`;
  }
  return `/${normalized}.html`;
};

const localizedSourceAliases = new Map([
  ["docs/reference-landing.md", "ja/docs/reference.md"],
  ["ja/docs/reference.md", "docs/reference-landing.md"],
]);

const alternateSourcePath = (relativePath: string) =>
  localizedSourceAliases.get(relativePath) ??
  (relativePath.startsWith("ja/")
    ? relativePath.slice(3)
    : `ja/${relativePath}`);

const projectNavItems = [
  { text: "Proposals", link: "/proposals/" },
  { text: "Form inventory", link: "/forms/" },
  { text: "Conformance evidence", link: "/conformance/" },
];

const historicalNavItems = [
  { text: "Historical specification", link: "/spec/" },
  { text: "Legacy reference projection", link: "/docs/reference.html" },
  { text: "Legacy glossary projection", link: "/docs/glossary.html" },
  { text: "Historical releases", link: "/release/" },
  {
    text: "v2 to v3 migration boundary",
    link: "/release/migrations/v2-to-v3.html",
  },
];

const englishNav = [
  { text: "Overview", link: "/" },
  { text: "Get started", link: "/docs/" },
  {
    text: "Concepts",
    items: [
      { text: "Concepts", link: "/docs/concepts.html" },
      { text: "Ownership", link: "/docs/ownership.html" },
    ],
  },
  { text: "Version model", link: VERSION_MODEL_ROUTE },
  { text: "Reference", link: "/docs/reference-landing.html" },
  { text: "History", link: "/docs/history.html" },
  { text: "Project", items: projectNavItems },
  { text: "Historical source", items: historicalNavItems },
  { text: "GitHub", link: github },
];

const japaneseProjectNavItems = [
  { text: "提案 (英語のみ)", link: "/proposals/" },
  { text: "Form 一覧 (英語のみ)", link: "/forms/" },
  { text: "適合性の証拠 (英語のみ)", link: "/conformance/" },
];

const japaneseHistoricalNavItems = [
  { text: "仕様資料（履歴）", link: "/ja/spec/" },
  { text: "用語集（旧 projection・英語のみ）", link: "/docs/glossary.html" },
  { text: "リリース資料（英語のみ）", link: "/release/" },
  {
    text: "v2 から v3 の移行境界（英語のみ）",
    link: "/release/migrations/v2-to-v3.html",
  },
];

const japaneseNav = [
  { text: "概要", link: "/ja/" },
  { text: "はじめる", link: "/ja/docs/" },
  {
    text: "概念",
    items: [
      { text: "概念", link: "/ja/docs/concepts.html" },
      { text: "所有範囲", link: "/ja/docs/ownership.html" },
    ],
  },
  { text: "バージョンモデル", link: JAPANESE_VERSION_MODEL_ROUTE },
  { text: "リファレンス", link: "/ja/docs/reference.html" },
  { text: "履歴", link: "/ja/docs/history.html" },
  { text: "プロジェクト", items: japaneseProjectNavItems },
  { text: "履歴資料", items: japaneseHistoricalNavItems },
  { text: "English", link: "/" },
];

const edgeResourceItems = [
  { text: "Module worker", link: "/docs/resources/module_worker.html" },
  { text: "Worker bundle", link: "/docs/resources/worker_bundle.html" },
  {
    text: "Static asset bundle",
    link: "/docs/resources/static_asset_bundle.html",
  },
  { text: "Worker version", link: "/docs/resources/worker_version.html" },
  { text: "Worker deployment", link: "/docs/resources/worker_deployment.html" },
  {
    text: "Worker custom domain",
    link: "/docs/resources/worker_custom_domain.html",
  },
  { text: "Worker endpoint", link: "/docs/resources/worker_endpoint.html" },
  {
    text: "Worker cron trigger",
    link: "/docs/resources/worker_cron_trigger.html",
  },
  { text: "Edge KV namespace", link: "/docs/resources/edge_kv_namespace.html" },
  { text: "SQLite database", link: "/docs/resources/sqlite_database.html" },
  {
    text: "SQLite migration set",
    link: "/docs/resources/sqlite_migration_set.html",
  },
  {
    text: "SQLite migration application",
    link: "/docs/resources/sqlite_migration_application.html",
  },
  {
    text: "At-least-once queue",
    link: "/docs/resources/at_least_once_queue.html",
  },
  { text: "Queue consumer", link: "/docs/resources/queue_consumer.html" },
  { text: "Durable workflow", link: "/docs/resources/durable_workflow.html" },
  { text: "Actor namespace", link: "/docs/resources/actor_namespace.html" },
];

const currentStackResourceItems = [
  ...edgeResourceItems,
  { text: "Function", link: "/docs/resources/function.html" },
  { text: "Function version", link: "/docs/resources/function_version.html" },
  {
    text: "Function deployment",
    link: "/docs/resources/function_deployment.html",
  },
  { text: "Function endpoint", link: "/docs/resources/function_endpoint.html" },
  {
    text: "Container service",
    link: "/docs/resources/serverless_container_service.html",
  },
  {
    text: "Container revision",
    link: "/docs/resources/container_revision.html",
  },
  { text: "Container traffic", link: "/docs/resources/container_traffic.html" },
  {
    text: "Container endpoint",
    link: "/docs/resources/container_endpoint.html",
  },
  {
    text: "Container custom domain",
    link: "/docs/resources/container_custom_domain.html",
  },
  { text: "Table", link: "/docs/resources/table.html" },
  { text: "Pull queue", link: "/docs/resources/pull_queue.html" },
  { text: "Topic", link: "/docs/resources/topic.html" },
  {
    text: "Topic subscription",
    link: "/docs/resources/topic_subscription.html",
  },
  { text: "Schedule", link: "/docs/resources/message_schedule.html" },
  { text: "Vector index", link: "/docs/resources/dense_vector_index.html" },
];

const specSidebar = [
  {
    text: "Historical Specification 1.1 receipt",
    items: [
      { text: "Contract map (historical)", link: "/spec/" },
      { text: "Overview (historical)", link: "/spec/overview.html" },
    ],
  },
  {
    text: "Historical concepts",
    items: [
      {
        text: "Portability boundary (historical)",
        link: "/spec/portability-boundary.html",
      },
      { text: "Form Families (historical)", link: "/spec/form-families.html" },
      {
        text: "Project lifecycle (historical)",
        link: "/spec/project-lifecycle.html",
      },
      {
        text: "Release evidence policy (historical)",
        link: "/spec/publication-freeze.html",
      },
      { text: "Versioning (historical)", link: "/spec/versioning.html" },
      { text: "Conformance (historical)", link: "/spec/conformance.html" },
    ],
  },
  {
    text: "Historical contracts",
    items: [
      {
        text: "Host API v1 (historical source)",
        link: "/spec/host-api/v1.html",
      },
      {
        text: "Form Definition (historical source)",
        link: "/spec/form-definition/",
      },
      { text: "Form Package (historical source)", link: "/spec/form-package/" },
      {
        text: "Interface contracts (historical source)",
        link: "/spec/interface-contract/",
      },
      {
        text: "Binding contracts (historical source)",
        link: "/spec/binding-contract/",
      },
      {
        text: "Artifact transport (historical source)",
        link: "/spec/artifact-transport/",
      },
      { text: "Trust (historical source)", link: "/spec/trust/" },
      { text: "Decisions (historical source)", link: "/spec/decisions/" },
    ],
  },
  {
    text: "Withdrawn epochs / migration",
    collapsed: true,
    items: [
      { text: "Current version model", link: VERSION_MODEL_ROUTE },
      { text: "Host API lanes (historical)", link: "/spec/host-api/" },
      {
        text: "Retained Host API v1beta4",
        link: "/spec/host-api/v1beta4.html",
      },
      {
        text: "Retained Host API v1beta1",
        link: "/spec/host-api/v1beta1.html",
      },
      {
        text: "v2 to v3 migration boundary",
        link: "/release/migrations/v2-to-v3.html",
      },
    ],
  },
];

const edgeProposalItems = [
  { text: "Family overview", link: "/proposals/edge/" },
  { text: "Module worker", link: "/proposals/edge/module-worker.html" },
  { text: "Worker bundle", link: "/proposals/edge/worker-bundle.html" },
  {
    text: "Static asset bundle",
    link: "/proposals/edge/static-asset-bundle.html",
  },
  { text: "Worker version", link: "/proposals/edge/worker-version.html" },
  { text: "Worker deployment", link: "/proposals/edge/worker-deployment.html" },
  {
    text: "Worker custom domain",
    link: "/proposals/edge/worker-custom-domain.html",
  },
  { text: "Worker endpoint", link: "/proposals/edge/worker-endpoint.html" },
  {
    text: "Worker cron trigger",
    link: "/proposals/edge/worker-cron-trigger.html",
  },
  { text: "Edge KV namespace", link: "/proposals/edge/edge-kv-namespace.html" },
  { text: "SQLite database", link: "/proposals/edge/sqlite-database.html" },
  {
    text: "SQLite migration set",
    link: "/proposals/edge/sqlite-migration-set.html",
  },
  {
    text: "SQLite migration application",
    link: "/proposals/edge/sqlite-migration-application.html",
  },
  {
    text: "At-least-once queue",
    link: "/proposals/edge/at-least-once-queue.html",
  },
  { text: "Queue consumer", link: "/proposals/edge/queue-consumer.html" },
  { text: "Durable workflow", link: "/proposals/edge/durable-workflow.html" },
  { text: "Actor namespace", link: "/proposals/edge/actor-namespace.html" },
];

const englishSidebar = {
  "/docs/": [
    {
      text: "Provider guide",
      items: [
        { text: "Quick start", link: "/docs/" },
        { text: "Concepts", link: "/docs/concepts.html" },
        { text: "Ownership", link: "/docs/ownership.html" },
        { text: "Version model", link: VERSION_MODEL_ROUTE },
        { text: "Reference landing", link: "/docs/reference-landing.html" },
        { text: "History", link: "/docs/history.html" },
      ],
    },
    {
      text: "Provider 3 typed reference (31 current Experimental Forms)",
      items: currentStackResourceItems,
    },
    {
      text: "Historical source / migration",
      collapsed: true,
      items: [
        { text: "Historical specification", link: "/spec/" },
        { text: "Legacy reference projection", link: "/docs/reference.html" },
        { text: "Legacy glossary projection", link: "/docs/glossary.html" },
        { text: "Historical releases", link: "/release/" },
        {
          text: "v2 to v3 migration boundary",
          link: "/release/migrations/v2-to-v3.html",
        },
      ],
    },
  ],
  "/spec/": specSidebar,
  "/proposals/": [
    {
      text: "Current 8-family / 31-Form candidate corpus",
      items: [{ text: "Overview", link: "/proposals/" }, ...edgeProposalItems],
    },
    {
      text: "Additional current families (decision 0043)",
      collapsed: true,
      items: [
        { text: "Function family", link: "/proposals/function/" },
        { text: "Container family", link: "/proposals/container/" },
        { text: "Table family", link: "/proposals/table/" },
        { text: "Pull queue family", link: "/proposals/queue/" },
        { text: "Topic family", link: "/proposals/topic/" },
        { text: "Schedule family", link: "/proposals/schedule/" },
        { text: "Vector family", link: "/proposals/vector/" },
      ],
    },
  ],
  "/forms/": [
    { text: "Forms", items: [{ text: "Current inventory", link: "/forms/" }] },
  ],
  "/conformance/": [
    {
      text: "Conformance",
      items: [{ text: "Evidence map", link: "/conformance/" }],
    },
  ],
  "/release/": [
    {
      text: "Historical release evidence",
      items: [{ text: "Provider and receipt history", link: "/release/" }],
    },
  ],
};

const japaneseSidebar = {
  "/ja/docs/": [
    {
      text: "Provider の導入",
      items: [
        { text: "クイックスタート", link: "/ja/docs/" },
        { text: "概念", link: "/ja/docs/concepts.html" },
        { text: "所有範囲", link: "/ja/docs/ownership.html" },
        { text: "バージョンモデル", link: JAPANESE_VERSION_MODEL_ROUTE },
        { text: "リファレンス", link: "/ja/docs/reference.html" },
        { text: "履歴", link: "/ja/docs/history.html" },
      ],
    },
    {
      text: "Provider 3 のリソース（英語のみ）",
      items: currentStackResourceItems,
    },
    {
      text: "履歴資料 / 移行",
      collapsed: true,
      items: [
        {
          text: "仕様資料（履歴）",
          link: "/ja/spec/",
        },
        {
          text: "用語集（旧 projection・英語のみ）",
          link: "/docs/glossary.html",
        },
        {
          text: "リリース資料（英語のみ）",
          link: "/release/",
        },
        {
          text: "v2 から v3 の移行境界（英語のみ）",
          link: "/release/migrations/v2-to-v3.html",
        },
      ],
    },
  ],
  "/ja/spec/": [
    {
      text: "仕様資料（履歴）",
      items: [
        { text: "契約マップ（履歴）", link: "/ja/spec/" },
        { text: "現行の概念", link: "/ja/docs/concepts.html" },
        { text: "現行のバージョンモデル", link: JAPANESE_VERSION_MODEL_ROUTE },
      ],
    },
  ],
};

export default defineConfig({
  lang: "en",
  title: "Takoform",
  description: `Portable resource contracts for Terraform and OpenTofu. Host API ${HOST_API_LANE}; Core library ${CORE_LIBRARY_VERSION}.`,
  cleanUrls: false,
  lastUpdated: false,
  srcExclude: ["**/README.md", "**/DESIGN.md", "static/**"],
  outDir: "public",
  // The 404 page is generated like every other page; it must not appear in the
  // sitemap. transformItems filters it before the sitemap is written, so the
  // exclusion applies in every build path (local, snapshot, deploy).
  sitemap: {
    hostname: "https://takoform.com",
    transformItems(items) {
      return items.filter(
        (item) => item.url !== "404.html" && !item.url.endsWith("/404.html"),
      );
    },
  },
  vite: {
    build: {
      target: "esnext",
      chunkSizeWarningLimit: 700,
    },
    publicDir: "static",
  },
  // Local search indexes pages concurrently by default. Serializing that
  // pass keeps MiniSearch insertion order (and its emitted chunk hash) stable
  // across fresh builds, including the isolated snapshot output.
  buildConcurrency: 1,
  transformHead({ pageData }) {
    const relativePath = pageData.relativePath.replaceAll("\\", "/");
    const route = routeFromRelativePath(relativePath);
    const alternatePath = alternateSourcePath(relativePath);
    const alternateExists = existsSync(path.join(siteDirectory, alternatePath));
    const isJapanese = relativePath.startsWith("ja/");
    const canonical = `${siteOrigin}${route}`;
    const entries = [["link", { rel: "canonical", href: canonical }]] as [
      string,
      Record<string, string>,
    ][];

    if (alternateExists) {
      const alternateRoute = routeFromRelativePath(alternatePath);
      const englishRoute = isJapanese ? alternateRoute : route;
      const japaneseRoute = isJapanese ? route : alternateRoute;
      entries.push(
        [
          "link",
          {
            rel: "alternate",
            hreflang: "en",
            href: `${siteOrigin}${englishRoute}`,
          },
        ],
        [
          "link",
          {
            rel: "alternate",
            hreflang: "ja",
            href: `${siteOrigin}${japaneseRoute}`,
          },
        ],
        [
          "link",
          {
            rel: "alternate",
            hreflang: "x-default",
            href: `${siteOrigin}${englishRoute}`,
          },
        ],
      );
    }

    return entries;
  },
  themeConfig: {
    outline: { level: [2, 3] },
    search: {
      provider: "local",
    },
    siteStatus,
  },
  locales: {
    root: {
      label: "English",
      lang: "en",
      description: `Portable resource contracts for Terraform and OpenTofu. Host API ${HOST_API_LANE}; Core library ${CORE_LIBRARY_VERSION}.`,
      head: [
        ["meta", { property: "og:locale", content: "en_US" }],
        ["meta", { name: "twitter:card", content: "summary" }],
      ],
      themeConfig: {
        nav: englishNav,
        sidebar: englishSidebar,
        siteStatus,
      },
    },
    ja: {
      label: "日本語",
      lang: "ja",
      link: "/ja/",
      description: `Terraform / OpenTofu の移植可能なリソース契約。Host API ${HOST_API_LANE}、Core ライブラリ ${CORE_LIBRARY_VERSION}。`,
      head: [
        ["meta", { property: "og:locale", content: "ja_JP" }],
        ["meta", { name: "twitter:card", content: "summary" }],
      ],
      themeConfig: {
        nav: japaneseNav,
        sidebar: japaneseSidebar,
        siteStatus,
      },
    },
  },
});

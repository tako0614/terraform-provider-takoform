import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitepress";

import { prepareSiteStatus, SITE_STATUS_ROUTE } from "./site-status.mjs";

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
    "TAKOFORM_WEBSITE_SNAPSHOT_READ_ONLY must be exactly \"1\" when set",
  );
}
const siteStatus = {
  ...prepareSiteStatus(siteDirectory, {
    write: snapshotReadOnlyFlag === undefined,
  }),
  route: SITE_STATUS_ROUTE,
};

// The public JSON document keeps a few legacy Specification fields for the
// historical release/check consumers that still read them. They are not site
// navigation or marketing facts, so do not hydrate them into every page's
// themeConfig (where they would look current and be duplicated in HTML).
const siteStatusForTheme = Object.fromEntries(
  Object.entries(siteStatus).filter(
    ([field]) =>
      ![
        "specificationVersion",
        "specificationReleaseStatus",
        "hostApiPublicationStatus",
      ].includes(field),
  ),
);

const projectNavItems = [
  { text: "Proposals", link: "/proposals/" },
  {
    text: "Provider 3.0 inventory (historical)",
    link: "/forms/",
  },
  { text: "Conformance evidence", link: "/conformance/" },
  { text: "Release", link: "/release/" },
];

// Current design target comes before withdrawn history in every sidebar.
const currentContractItems = [
  { text: "Host API v1", link: "/spec/host-api/v1.html" },
  { text: "Form Definition", link: "/spec/form-definition/" },
  { text: "Form Package (wire envelope)", link: "/spec/form-package/" },
  { text: "Interface contracts", link: "/spec/interface-contract/" },
  { text: "Binding contracts", link: "/spec/binding-contract/" },
  { text: "Core contracts", link: "/spec/core/" },
];

const compatibilityNavItems = [
  { text: "Versions & compatibility", link: "/docs/versions.html" },
  {
    text: "v2 to v3 migration boundary",
    link: "/release/migrations/v2-to-v3.html",
  },
];

const englishNav = [
  { text: "Current", link: "/docs/" },
  { text: "Historical source", link: "/spec/" },
  {
    text: "Compatibility",
    items: compatibilityNavItems,
  },
  { text: "Project", items: projectNavItems },
  { text: "GitHub", link: github },
];

// The project trees are published once, in English. The Japanese navigation
// points at the same targets and marks them 英語のみ, matching the note
// convention used inside the Japanese pages.
const japaneseProjectNavItems = [
  { text: "Proposals (英語のみ)", link: "/proposals/" },
  {
    text: "Provider 3.0 inventory (英語のみ・履歴)",
    link: "/forms/",
  },
  { text: "Conformance evidence (英語のみ)", link: "/conformance/" },
  { text: "Release (英語のみ)", link: "/release/" },
];

const japaneseCurrentContractItems = [
  { text: "Host API v1 (英語のみ)", link: "/spec/host-api/v1.html" },
  { text: "Form Definition (英語のみ)", link: "/spec/form-definition/" },
  {
    text: "Form Package (wire envelope / 英語のみ)",
    link: "/spec/form-package/",
  },
  {
    text: "Interface contracts (英語のみ)",
    link: "/spec/interface-contract/",
  },
  { text: "Binding contracts (英語のみ)", link: "/spec/binding-contract/" },
  { text: "Core contracts (英語のみ)", link: "/spec/core/" },
];

const japaneseCompatibilityNavItems = [
  { text: "Versions & compatibility (英語のみ)", link: "/docs/versions.html" },
  {
    text: "v2 to v3 migration boundary (英語のみ)",
    link: "/release/migrations/v2-to-v3.html",
  },
];

const japaneseNav = [
  { text: "現在", link: "/ja/docs/" },
  { text: "履歴資料", link: "/ja/spec/" },
  {
    text: "互換性",
    items: japaneseCompatibilityNavItems,
  },
  { text: "プロジェクト", items: japaneseProjectNavItems },
  { text: "GitHub", link: github },
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
];

const deferredResourceItems = [
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
  { text: "Container revision", link: "/docs/resources/container_revision.html" },
  { text: "Container traffic", link: "/docs/resources/container_traffic.html" },
  { text: "Container endpoint", link: "/docs/resources/container_endpoint.html" },
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

const japaneseEdgeResourceItems = edgeResourceItems.map(({ text, link }) => ({
  text: `${text} (英語のみ)`,
  link,
}));

const japaneseDeferredResourceItems = deferredResourceItems.map(({ text, link }) => ({
  text: `${text} (英語のみ・保留)`,
  link,
}));

const specSidebar = [
  {
    text: "Historical specification receipts and source",
    items: [
      { text: "Contract map", link: "/spec/" },
      { text: "Overview", link: "/spec/overview.html" },
    ],
  },
  {
    text: "Concepts",
    items: [
      { text: "Portability boundary", link: "/spec/portability-boundary.html" },
      { text: "Form Families", link: "/spec/form-families.html" },
      { text: "Project lifecycle", link: "/spec/project-lifecycle.html" },
      { text: "Release evidence policy", link: "/spec/publication-freeze.html" },
      { text: "Versioning", link: "/spec/versioning.html" },
      { text: "Conformance", link: "/spec/conformance.html" },
    ],
  },
  {
    text: "Contracts",
    items: [
      { text: "Host API v1", link: "/spec/host-api/v1.html" },
      { text: "Form Definition", link: "/spec/form-definition/" },
      { text: "Form Package", link: "/spec/form-package/" },
      { text: "Interface contracts", link: "/spec/interface-contract/" },
      { text: "Binding contracts", link: "/spec/binding-contract/" },
      { text: "Artifact transport", link: "/spec/artifact-transport/" },
      { text: "Trust", link: "/spec/trust/" },
      { text: "Decisions", link: "/spec/decisions/" },
    ],
  },
  {
    text: "Withdrawn lanes / migration history",
    collapsed: true,
    items: [
      { text: "Versions & compatibility", link: "/docs/versions.html" },
      { text: "Host API lanes", link: "/spec/host-api/" },
      { text: "Retained Host API v1beta4", link: "/spec/host-api/v1beta4.html" },
      { text: "Retained Host API v1beta1", link: "/spec/host-api/v1beta1.html" },
      { text: "v2 to v3 migration boundary", link: "/release/migrations/v2-to-v3.html" },
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
      text: "Stable Host API v1 and current Forms",
      items: [
        { text: "Quick start", link: "/docs/" },
        { text: "Versions & compatibility", link: "/docs/versions.html" },
        { text: "Reference", link: "/docs/reference.html" },
        { text: "Glossary", link: "/docs/glossary.html" },
      ],
    },
    {
      text: "Current normative contracts",
      items: currentContractItems,
    },
    {
      text: "Provider typed reference (official Forms)",
      items: currentStackResourceItems,
    },
    {
      text: "Deferred / historical candidate resources",
      collapsed: true,
      items: deferredResourceItems,
    },
    {
      text: "Historical compatibility",
      collapsed: true,
      items: [
        { text: "Versions & compatibility", link: "/docs/versions.html" },
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
      text: "Current Edge16 candidate notes",
      items: [{ text: "Overview", link: "/proposals/" }, ...edgeProposalItems],
    },
    {
      text: "Deferred / historical candidate families",
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
    {
      text: "Provider 3.0 inventory (historical)",
      items: [{ text: "Retained resource inventory", link: "/forms/" }],
    },
  ],
  "/conformance/": [
    {
      text: "Conformance",
      items: [{ text: "Evidence map", link: "/conformance/" }],
    },
  ],
  "/release/": [
    {
      text: "Release records",
      items: [{ text: "Provider releases and historical receipts", link: "/release/" }],
    },
  ],
};

const japaneseSidebar = {
  "/ja/docs/": [
    {
      text: "Stable Host API v1 と current Edge16 Forms",
      items: [
        { text: "クイックスタート", link: "/ja/docs/" },
        {
          text: "Versions & compatibility (英語のみ)",
          link: "/ja/docs/versions.html",
        },
        { text: "用語集 (英語のみ)", link: "/docs/glossary.html" },
      ],
    },
    {
      text: "Current normative contracts (英語のみ)",
      items: japaneseCurrentContractItems,
    },
    {
      text: "Provider typed reference (official Forms / 英語のみ)",
      items: japaneseEdgeResourceItems,
    },
    {
      text: "Deferred / historical candidate resources (英語のみ・保留)",
      collapsed: true,
      items: japaneseDeferredResourceItems,
    },
    {
      text: "履歴互換性",
      collapsed: true,
      items: [
        {
          text: "Versions & compatibility (英語のみ)",
          link: "/ja/docs/versions.html",
        },
        {
          text: "v2 to v3 migration boundary (英語のみ)",
          link: "/release/migrations/v2-to-v3.html",
        },
      ],
    },
  ],
  "/ja/spec/": [
    {
      text: "仕様の履歴資料",
      items: [{ text: "契約マップ", link: "/ja/spec/" }],
    },
  ],
};

export default defineConfig({
  lang: "en",
  title: "Takoform",
  description:
    "Portable contracts for Terraform and OpenTofu, with a stable Host API and typed provider mappings.",
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
  themeConfig: {
    outline: { level: [2, 3] },
    search: {
      provider: "local",
    },
    siteStatus: siteStatusForTheme,
  },
  locales: {
    root: {
      label: "English",
      lang: "en",
      themeConfig: {
        nav: englishNav,
        sidebar: englishSidebar,
        siteStatus: siteStatusForTheme,
      },
    },
    ja: {
      label: "日本語",
      lang: "ja",
      link: "/ja/",
      themeConfig: {
        nav: japaneseNav,
        sidebar: japaneseSidebar,
        siteStatus: siteStatusForTheme,
      },
    },
  },
});

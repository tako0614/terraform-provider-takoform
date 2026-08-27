import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitepress";

import { prepareSiteStatus, SITE_STATUS_ROUTE } from "./site-status.mjs";

const github = "https://github.com/tako0614/terraform-provider-takoform";
const coreSpec = "https://github.com/tako0614/takoform/tree/v1.0.1/spec";

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

const projectNavItems = [
  { text: "Conformance evidence", link: "/conformance/" },
  { text: "Release", link: "/release/" },
];

// Current design target comes before withdrawn history in every sidebar.
const compatibilityNavItems = [
  { text: "Versions & compatibility", link: "/docs/versions.html" },
  {
    text: "v2 to v3 migration boundary",
    link: "/release/migrations/v2-to-v3.html",
  },
];

const englishNav = [
  { text: "Current", link: "/docs/" },
  { text: "Core v1.0.1 source", link: coreSpec },
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
  { text: "Conformance evidence (英語のみ)", link: "/conformance/" },
  { text: "Release (英語のみ)", link: "/release/" },
];

const japaneseCompatibilityNavItems = [
  { text: "Versions & compatibility (英語のみ)", link: "/docs/versions.html" },
  {
    text: "v2 to v3 migration boundary (英語のみ)",
    link: "/release/migrations/v2-to-v3.html",
  },
];

const japaneseNav = [
  { text: "Current", link: "/ja/docs/" },
  { text: "Core v1.0.1 source (英語のみ)", link: coreSpec },
  {
    text: "Compatibility",
    items: japaneseCompatibilityNavItems,
  },
  { text: "Project", items: japaneseProjectNavItems },
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

const englishSidebar = {
  "/docs/": [
    {
      text: "Provider 3 quick start",
      items: [
        { text: "Quick start", link: "/docs/" },
        { text: "Versions & compatibility", link: "/docs/versions.html" },
        { text: "Reference", link: "/docs/reference.html" },
        { text: "Glossary", link: "/docs/glossary.html" },
      ],
    },
    {
      text: "Current Provider 3 mapping (31 resources)",
      items: currentStackResourceItems,
    },
    {
      text: "Withdrawn epochs / Migration",
      collapsed: true,
      items: [
        { text: "Versions & compatibility", link: "/docs/versions.html" },
        { text: "Host API history (Core source)", link: `${coreSpec}/host-api` },
        {
          text: "v2 to v3 migration boundary",
          link: "/release/migrations/v2-to-v3.html",
        },
      ],
    },
  ],
  "/forms/": [
    {
      text: "Provider mapping",
      items: [{ text: "Current mapping", link: "/forms/" }],
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
      text: "Release",
      items: [{ text: "Specification and Provider releases", link: "/release/" }],
    },
  ],
};

const japaneseSidebar = {
  "/ja/docs/": [
    {
      text: "Provider 3 quick start",
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
      text: "Current Provider 3 mapping (31 resources)",
      items: currentStackResourceItems,
    },
    {
      text: "Withdrawn epochs / Migration",
      collapsed: true,
      items: [
        {
          text: "Versions & compatibility (英語のみ)",
          link: "/ja/docs/versions.html",
        },
        {
          text: "Host API history (Core source, 英語のみ)",
          link: `${coreSpec}/host-api`,
        },
        {
          text: "v2 to v3 migration boundary (英語のみ)",
          link: "/release/migrations/v2-to-v3.html",
        },
      ],
    },
  ],
};

export default defineConfig({
  lang: "en",
  title: "Takoform",
  description:
    "One typed provider. Any compatible host. Portable resource contracts for Terraform and OpenTofu.",
  cleanUrls: false,
  lastUpdated: false,
  // Spec and proposal Markdown remains addressable for historical URL and
  // fixture continuity. sync-website-spec projects those sources as concise
  // non-authoritative stubs; they are intentionally absent from navigation.
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
    siteStatus,
  },
  locales: {
    root: {
      label: "English",
      lang: "en",
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
      themeConfig: {
        nav: japaneseNav,
        sidebar: japaneseSidebar,
        siteStatus,
      },
    },
  },
});

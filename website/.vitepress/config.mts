import path from "node:path";
import { fileURLToPath } from "node:url";

import { defineConfig } from "vitepress";

import { prepareSiteStatus, SITE_STATUS_ROUTE } from "./site-status.mjs";

const github = "https://github.com/tako0614/terraform-provider-takoform";

// The published/preview facts are derived from the repository once, here, at
// build time. They reach the footer through themeConfig so every page states
// them as static HTML, and they reach machines through the JSON document this
// call writes. Every one of them is a pure function of committed repository
// bytes, so a fresh build reproduces the whole published tree; no commit id
// appears anywhere in it (see .vitepress/site-status.mjs).
const siteDirectory = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const siteStatus = {
  ...prepareSiteStatus(siteDirectory),
  route: SITE_STATUS_ROUTE,
};

const projectNavItems = [
  { text: "Proposals", link: "/proposals/" },
  { text: "Form inventory", link: "/forms/" },
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
  { text: "Spec", link: "/spec/" },
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
  { text: "Form inventory (英語のみ)", link: "/forms/" },
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
  { text: "Spec", link: "/ja/spec/" },
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
  {
    text: "Edge object bucket",
    link: "/docs/resources/edge_object_bucket.html",
  },
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
];

const currentStackResourceItems = edgeResourceItems;

const specSidebar = [
  {
    text: "Spec",
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
      { text: "Beta release policy", link: "/spec/publication-freeze.html" },
      { text: "Versioning", link: "/spec/versioning.html" },
      { text: "Conformance", link: "/spec/conformance.html" },
    ],
  },
  {
    text: "Contracts",
    items: [
      { text: "Host API v1beta1", link: "/spec/host-api/v1beta1.html" },
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
    text: "Withdrawn epochs / Migration",
    collapsed: true,
    items: [
      { text: "Versions & compatibility", link: "/docs/versions.html" },
      { text: "Host API lanes", link: "/spec/host-api/" },
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
  { text: "Object bucket", link: "/proposals/edge/object-bucket.html" },
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
];

const englishSidebar = {
  "/docs/": [
    {
      text: "Current design target — Provider 2.1.1 / Host API v1beta1",
      items: [
        { text: "Quick start", link: "/docs/" },
        { text: "Versions & compatibility", link: "/docs/versions.html" },
        { text: "Reference", link: "/docs/reference.html" },
        { text: "Glossary", link: "/docs/glossary.html" },
      ],
    },
    {
      text: "Current design target — Edge Form Family v1beta1 (15 Experimental Forms)",
      items: currentStackResourceItems,
    },
    {
      text: "Withdrawn epochs / Migration",
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
      text: "Current design target — Edge Form Family v1beta1 proposals",
      items: [{ text: "Overview", link: "/proposals/" }, ...edgeProposalItems],
    },
    {
      text: "New family proposals (decision 0043)",
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
      text: "Release",
      items: [{ text: "Provider release", link: "/release/" }],
    },
  ],
};

const japaneseSidebar = {
  "/ja/docs/": [
    {
      text: "Current design target — Provider 2.1.1 / Host API v1beta1",
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
      text: "Current design target — Edge Form Family v1beta1 (15 Experimental Forms)",
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
          text: "v2 to v3 migration boundary (英語のみ)",
          link: "/release/migrations/v2-to-v3.html",
        },
      ],
    },
  ],
  "/ja/spec/": [
    {
      text: "Spec",
      items: [{ text: "契約マップ", link: "/ja/spec/" }],
    },
  ],
};

export default defineConfig({
  lang: "en",
  title: "Takoform",
  description:
    "One provider. Dependent on none. Portable resource contracts for Terraform and OpenTofu.",
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

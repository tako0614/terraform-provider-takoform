import { defineConfig } from "vitepress";

const github = "https://github.com/tako0614/terraform-provider-takoform";

const englishNav = [
  { text: "Docs", link: "/docs/" },
  { text: "Spec", link: "/spec/" },
  { text: "GitHub", link: github },
];

const japaneseNav = [
  { text: "Docs", link: "/ja/docs/" },
  { text: "Spec", link: "/ja/spec/" },
  { text: "GitHub", link: github },
];

const resourceItems = [
  { text: "Edge worker", link: "/docs/resources/edge_worker.html" },
  { text: "Relational database", link: "/docs/resources/relational_database.html" },
  { text: "Object bucket", link: "/docs/resources/object_bucket.html" },
  { text: "Key value store", link: "/docs/resources/key_value_store.html" },
  { text: "Queue", link: "/docs/resources/queue.html" },
  { text: "Schedule", link: "/docs/resources/schedule.html" },
  { text: "Container service", link: "/docs/resources/container_service.html" },
  { text: "Stateful entity", link: "/docs/resources/stateful_entity.html" },
  { text: "Vector index", link: "/docs/resources/vector_index.html" },
  { text: "Interface data source", link: "/docs/data-sources/interface.html" },
];

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
      { text: "Project lifecycle", link: "/spec/project-lifecycle.html" },
      { text: "Versioning", link: "/spec/versioning.html" },
      { text: "Conformance", link: "/spec/conformance.html" },
    ],
  },
  {
    text: "Contracts",
    items: [
      { text: "Form Definition", link: "/spec/form-definition/" },
      { text: "Form Package", link: "/spec/form-package/" },
      { text: "Host API", link: "/spec/host-api/" },
      { text: "Interface declaration", link: "/spec/interface-declaration/" },
      { text: "Trust", link: "/spec/trust/" },
      { text: "Decisions", link: "/spec/decisions/" },
    ],
  },
];

const proposalItems = [
  { text: "Edge worker", link: "/proposals/edge-worker.html" },
  { text: "Relational database", link: "/proposals/relational-database.html" },
  { text: "Object bucket", link: "/proposals/object-bucket.html" },
  { text: "Key value store", link: "/proposals/key-value-store.html" },
  { text: "Queue", link: "/proposals/queue.html" },
  { text: "Schedule", link: "/proposals/schedule.html" },
  { text: "Container service", link: "/proposals/container-service.html" },
  { text: "Stateful entity", link: "/proposals/stateful-entity.html" },
  { text: "Vector index", link: "/proposals/vector-index.html" },
];

const englishSidebar = {
  "/docs/": [
    {
      text: "Docs",
      items: [
        { text: "Quick start", link: "/docs/" },
        { text: "Reference", link: "/docs/reference.html" },
      ],
    },
    { text: "Resources", items: resourceItems },
  ],
  "/spec/": specSidebar,
  "/proposals/": [
    {
      text: "Proposals",
      items: [
        { text: "Overview", link: "/proposals/" },
        ...proposalItems,
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
      text: "Docs",
      items: [{ text: "クイックスタート", link: "/ja/docs/" }],
    },
    { text: "Resources", items: resourceItems },
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
  sitemap: {
    hostname: "https://takoform.com",
  },
  vite: {
    build: {
      target: "esnext",
      chunkSizeWarningLimit: 700,
    },
    publicDir: "static",
  },
  themeConfig: {
    outline: false,
    search: {
      provider: "local",
    },
  },
  locales: {
    root: {
      label: "English",
      lang: "en",
      themeConfig: {
        nav: englishNav,
        sidebar: englishSidebar,
      },
    },
    ja: {
      label: "日本語",
      lang: "ja",
      link: "/ja/",
      themeConfig: {
        nav: japaneseNav,
        sidebar: japaneseSidebar,
      },
    },
  },
});

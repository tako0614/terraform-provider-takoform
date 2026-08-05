import {
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { loadPublicationTruth } from "./publication-truth.mjs";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const publicRoot = path.join(repositoryRoot, "website", "public");
const write = process.argv.includes("--write");
const check = process.argv.includes("--check");

if (write === check) {
  throw new Error("usage: bun scripts/website-snapshot.mjs --write|--check");
}

const publication = loadPublicationTruth(repositoryRoot);
const candidates = JSON.parse(
  readFileSync(
    path.join(
      repositoryRoot,
      "forms",
      "candidates",
      "v1alpha2",
      "candidate-set.json",
    ),
    "utf8",
  ),
);
const currentHostContract = JSON.parse(
  readFileSync(
    path.join(repositoryRoot, "spec", "host-api", "operations.json"),
    "utf8",
  ),
);
const legacyHostContract = JSON.parse(
  readFileSync(
    path.join(repositoryRoot, "spec", "host-api", "operations-v1alpha1.json"),
    "utf8",
  ),
);

if (
  candidates.format !== "takoform.current-form-candidates@v2" ||
  candidates.formApiVersion !== "forms.takoform.com/v1alpha2" ||
  candidates.packageApiVersion !== "packages.forms.takoform.com/v1alpha3" ||
  candidates.publicationStatus !== "unpublished" ||
  candidates.lifecycleAuthority !== "forms/lifecycle.json" ||
  Object.hasOwn(candidates, "classification") ||
  Object.hasOwn(candidates, "targetLifecycleState") ||
  Object.hasOwn(candidates, "publicationReady") ||
  !Array.isArray(candidates.forms) ||
  candidates.forms.length !== 9
) {
  throw new Error("current candidate set does not match the website contract");
}
if (
  currentHostContract.format !== "takoform.host-api@v1alpha2" ||
  currentHostContract.apiGroup !== candidates.formApiVersion ||
  currentHostContract.discoveryPath !== "/.well-known/takoform/v1alpha2" ||
  legacyHostContract.format !== "takoform.host-api@v1alpha1" ||
  legacyHostContract.apiGroup !== publication.apiVersion ||
  legacyHostContract.discoveryPath !== "/.well-known/takoform"
) {
  throw new Error("Host API epochs do not match the website contract");
}

const resourceCopy = {
  EdgeWorker: {
    slug: "edge_worker",
    ja: "edge request と event の runtime",
  },
  RelationalDatabase: {
    slug: "relational_database",
    ja: "relational data service",
  },
  ObjectBucket: {
    slug: "object_bucket",
    ja: "object storage",
  },
  KeyValueStore: {
    slug: "key_value_store",
    ja: "key/value state",
  },
  Queue: {
    slug: "queue",
    ja: "非同期message delivery",
  },
  Schedule: {
    slug: "schedule",
    ja: "接続先Resourceを1件invokeするcron lifecycle",
  },
  ContainerService: {
    slug: "container_service",
    ja: "digest固定container service",
  },
  StatefulEntity: {
    slug: "stateful_entity",
    ja: "addressable persistent entity",
  },
  VectorIndex: {
    slug: "vector_index",
    ja: "vector similarity index",
  },
};

const resources = candidates.forms.map((entry) => {
  const copy = resourceCopy[entry.kind];
  const definition = JSON.parse(
    readFileSync(path.join(repositoryRoot, entry.path, "definition.json"), "utf8"),
  );
  if (
    copy === undefined ||
    entry.formRef?.apiVersion !== candidates.formApiVersion ||
    entry.formRef?.definitionVersion !== "0.1.0" ||
    definition.apiVersion !== entry.formRef.apiVersion ||
    definition.kind !== entry.formRef.kind ||
    definition.definitionVersion !== entry.formRef.definitionVersion ||
    typeof definition.description !== "string" ||
    definition.description.length === 0
  ) {
    throw new Error(`unsupported website candidate ${JSON.stringify(entry.kind)}`);
  }
  return {
    kind: entry.kind,
    type: `takoform_${copy.slug}`,
    href:
      "https://github.com/tako0614/terraform-provider-takoform/blob/main/" +
      `docs/resources/${copy.slug}.md`,
    en: definition.description,
    ...copy,
  };
});

const github = "https://github.com/tako0614/terraform-provider-takoform";
const sourceLink = (relativePath) => `${github}/blob/main/${relativePath}`;
const currentProvider = publication.providerVersion;
const legacyProvider = publication.legacyProviderVersion;
const currentFormApi = candidates.formApiVersion;
const packageApi = candidates.packageApiVersion;
const legacyFormApi = publication.apiVersion;
const currentHostWireApi = currentHostContract.apiGroup;
const legacyHostWireApi = legacyHostContract.apiGroup;

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function providerPin(version) {
  return escapeHtml(`terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= ${version}"
    }
  }
}`);
}

function statusDisclosure(lang) {
  if (lang === "ja") {
    return `
      <p class="truth-note">
        Takoformは<strong>Experimental specification project</strong>です。現行FormRefは
        <code>${currentFormApi}</code>、現行Package envelopeは
        <code>${packageApi}</code>です。provider <code>v${currentProvider}</code>は
        Registry公開済みの現行clientです。provider <code>v${legacyProvider}</code>と
        <code>${legacyFormApi}</code>の${publication.publishedCount}件の公開済みForm Package identityは
        immutableな<strong>Legacy</strong>証跡です。現在の中央承認・admissionはありません。
      </p>`;
  }
  return `
      <p class="truth-note">
        Takoform is an <strong>Experimental specification project</strong>. Current FormRefs use
        <code>${currentFormApi}</code> and current package envelopes use
        <code>${packageApi}</code>. Provider <code>v${currentProvider}</code> is the published
        current Registry client. Provider <code>v${legacyProvider}</code> and the
        ${publication.publishedCount} published Form Package identities from
        <code>${legacyFormApi}</code> are immutable <strong>Legacy</strong> evidence. There is no
        current central approval or admission.
      </p>`;
}

function nav(lang, current) {
  const labels =
    lang === "ja"
      ? { home: "概要", docs: "Docs", spec: "Spec", toggle: "English" }
      : { home: "Overview", docs: "Docs", spec: "Spec", toggle: "日本語" };
  const other = lang === "ja" ? "en" : "ja";
  const link = (key, href, label) =>
    `<a${current === key ? ' aria-current="page"' : ""} href="${href}">${label}</a>`;
  return `
    <header class="site-header">
      <nav class="container nav" aria-label="${lang === "ja" ? "主要" : "Primary"}">
        <a class="brand" href="/" aria-label="Takoform home">Takoform</a>
        <div class="nav-links">
          ${link("home", "/", labels.home)}
          ${link("docs", "/docs/", labels.docs)}
          ${link("spec", "/spec/", labels.spec)}
          <button class="lang-switch" type="button" data-set-lang="${other}" aria-label="${labels.toggle}">${labels.toggle}</button>
        </div>
      </nav>
    </header>`;
}

function footer(lang) {
  return `
    <footer class="site-footer">
      <div class="container footer-inner">
        <span class="footer-brand">Takoform</span>
        <span>${lang === "ja" ? "Experimentalなportable resource contract" : "Experimental portable resource contracts"}</span>
        <a href="${github}">GitHub</a>
      </div>
    </footer>`;
}

function languageScript() {
  return `<script>
  (() => {
    const choose = (requested) => {
      const lang = requested === "ja" ? "ja" : "en";
      document.documentElement.lang = lang;
      document.documentElement.dataset.lang = lang;
      document.querySelectorAll(".l10n").forEach((node) => {
        node.hidden = node.lang !== lang;
      });
      if (location.hash.length > 1) {
        let current = location.hash.slice(1);
        try { current = decodeURIComponent(current); } catch {}
        const base = current.endsWith("-ja") ? current.slice(0, -3) : current;
        const translated = lang === "ja" ? base + "-ja" : base;
        if (document.getElementById(translated)) {
          history.replaceState(null, "", "#" + encodeURIComponent(translated));
        }
      }
      try { localStorage.setItem("takoform-lang", lang); } catch {}
    };
    let stored = "";
    try { stored = localStorage.getItem("takoform-lang") || ""; } catch {}
    choose(stored || (navigator.language.startsWith("ja") ? "ja" : "en"));
    document.addEventListener("click", (event) => {
      const button = event.target instanceof Element
        ? event.target.closest("[data-set-lang]")
        : null;
      if (button) choose(button.dataset.setLang);
    });
  })();
  </script>`;
}

function page({ title, description, canonicalPath, bodyClass, english, japanese }) {
  const canonical = `https://takoform.com${canonicalPath}`;
  return `<!doctype html>
<html lang="en" data-lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${escapeHtml(title)}</title>
  <meta name="description" content="${escapeHtml(description)}">
  <meta name="theme-color" content="#0e0a0a">
  <link rel="canonical" href="${canonical}">
  <meta property="og:type" content="website">
  <meta property="og:title" content="${escapeHtml(title)}">
  <meta property="og:description" content="${escapeHtml(description)}">
  <meta property="og:url" content="${canonical}">
  <link rel="icon" href="/tako.png">
  <link rel="stylesheet" href="/styles.css">
</head>
<body class="${bodyClass}">
  <div class="l10n" lang="en">${english}</div>
  <div class="l10n" lang="ja" hidden>${japanese}</div>
  ${languageScript()}
</body>
</html>
`.replace(/[ \t]+$/gmu, "");
}

function resourceRows(lang, linked) {
  return resources
    .map(
      (resource) => linked
        ? `
        <article class="resource">
          <a class="resource-link" href="${resource.href}"><code>${resource.type}</code></a>
          <span>${escapeHtml(resource[lang])}</span>
        </article>`
        : `
        <article class="resource">
          <code>${resource.type}</code>
          <span>${escapeHtml(resource[lang])}</span>
        </article>`,
    )
    .join("");
}

function overview(lang) {
  const ja = lang === "ja";
  return `${nav(lang, "home")}
    <main>
      <section class="hero">
        <div class="container hero-grid">
          <div class="hero-copy">
            <p class="project-line">Experimental · ${currentFormApi}</p>
            <h1>${ja ? "約束より先に、境界を定義する。" : "Define the boundary before the promise."}</h1>
            <p class="lede">${ja
              ? "Takoformは、host-neutralなdesired-state contractを小さく育てる仕様とtoolingです。Takosumi Cloudの実装は設計feedbackであり、標準化やlive availabilityの宣言ではありません。"
              : "Takoform grows small, host-neutral desired-state contracts. Takosumi Cloud implementations provide design feedback; they do not declare standardization or live availability."}</p>
            <div class="cta-row">
              <a class="btn btn-primary" href="/docs/">${ja ? "利用経路を選ぶ" : "Choose a usage lane"}</a>
              <a class="text-link" href="/spec/">${ja ? "仕様境界を確認" : "Inspect the contract"}</a>
            </div>
          </div>
          <dl class="status-ledger" aria-label="${ja ? "現在の状態" : "Current status"}">
            <div><dt>${ja ? "現行Form" : "Current Forms"}</dt><dd>${resources.length} × 0.1.0 source candidates</dd></div>
            <div><dt>Provider</dt><dd>v${currentProvider} ${ja ? "Registry公開済み" : "Registry published"}</dd></div>
            <div><dt>Legacy</dt><dd>v${legacyProvider} ${ja ? "公開済み" : "published"}</dd></div>
            <div><dt>Cloud</dt><dd>${ja ? "repo実装あり · live提供はhostの事実" : "repository implementations · live service is host evidence"}</dd></div>
          </dl>
        </div>
      </section>

      <section class="section section-current">
        <div class="container section-head">
		  <h2>${ja ? "9候補を、平等な出発点にする。" : "Nine candidates. One honest baseline."}</h2>
		  <p>${ja
			? "9種類はすべてProposalから生成した未公開の0.1.0候補で、明示的なlifecycle transitionを経るまでExperimentalではありません。過去のadmissionや人気度で格付けせず、実装・prior art・consumer evidenceから個別に育てます。VerifiedDomainとAIGatewayはCloud serviceでありFormではありません。"
			: "All nine definitions are Proposal-derived, unpublished 0.1.0 candidates. Each needs an explicit lifecycle transition before it can become Experimental. Previous admission and popularity grant no rank; implementation, prior art, and consumer evidence drive each Form independently. VerifiedDomain and AIGateway are Cloud services, not Forms."}</p>
        </div>
        <div class="container resource-grid">${resourceRows(lang, false)}</div>
      </section>

      <section class="section section-contract">
        <div class="container">
          <h2>${ja ? "versionは一つの数字ではない。" : "A version is not one number."}</h2>
          <div class="identity-stack">
            <div><span>${ja ? "現行Host wire" : "Current Host wire"}</span><code>${currentHostWireApi}</code><p>${ja ? "versioned discoveryから到達するprovider v2のwire contract。" : "The provider-v2 wire contract reached through versioned discovery."}</p></div>
            <div><span>${ja ? "Legacy Host wire" : "Legacy Host wire"}</span><code>${legacyHostWireApi}</code><p>${ja ? "非versioned discoveryに凍結したprovider v1互換lane。" : "The frozen provider-v1 lane behind unversioned discovery."}</p></div>
            <div><span>Current FormRef</span><code>${currentFormApi}</code><p>${ja ? "nested exact identity。Legacyと衝突しません。" : "The nested exact identity; it cannot collide with Legacy."}</p></div>
            <div><span>Current package</span><code>${packageApi}</code><p>${ja ? "現行FormRefを運ぶdata-only envelope。" : "The data-only envelope carrying the current FormRef."}</p></div>
            <div><span>Provider</span><code>v${currentProvider} published current client</code><p>${ja ? "Form maturityやCloud availabilityとは独立。" : "Independent from Form maturity and Cloud availability."}</p></div>
          </div>
        </div>
      </section>

      <section class="section section-paths">
        <div class="container">
          <h2>${ja ? "今やることから選ぶ。" : "Choose by the job you have now."}</h2>
          <div class="path-list">
            <article><a href="/docs/#current-source">${ja ? "現行仕様を利用する" : "Use the current line"}</a><span>${ja ? "Registryからv2.0.0をexact pin。" : "Install the exact v2.0.0 pin from the Registry."}</span></article>
            <article><a href="/docs/#legacy">${ja ? "既存stateを保守する" : "Maintain existing state"}</a><span>${ja ? `公開済みprovider v${legacyProvider}を固定。` : `Pin published provider v${legacyProvider}.`}</span></article>
            <article><a href="/docs/#migration">${ja ? "v1から移行する" : "Move from v1"}</a><span>${ja ? "stateを書き換えず、明示的にcreate/importする。" : "Create or import explicitly; never rewrite state."}</span></article>
          </div>
          ${statusDisclosure(lang)}
        </div>
      </section>
    </main>${footer(lang)}`;
}

function docs(lang) {
  const ja = lang === "ja";
  const anchor = (name) => (ja ? `${name}-ja` : name);
  return `${nav(lang, "docs")}
    <main>
      <section class="doc-hero">
        <div class="container">
          <p class="project-line">Documentation · Experimental</p>
          <h1>${ja ? "commandより先に、利用経路を選ぶ。" : "Choose the lane before the command."}</h1>
          <p class="lede">${ja
            ? "同じprovider addressに、公開済みcurrent v2と公開済みLegacy v1があります。自分の目的に合う方だけを使ってください。"
            : "One provider address has a published current v2 line and a published Legacy v1 line. Use only the lane that matches your job."}</p>
        </div>
      </section>
      <div class="container doc-layout">
        <aside class="doc-index" aria-label="${ja ? "このページ" : "On this page"}">
          <a href="#${anchor("current-source")}">${ja ? "現行sourceを評価" : "Evaluate current source"}</a>
          <a href="#${anchor("legacy")}">${ja ? "Legacyを保守" : "Maintain Legacy"}</a>
          <a href="#${anchor("migration")}">${ja ? "移行" : "Migration"}</a>
          <a href="#${anchor("resources")}">${ja ? "Resource reference" : "Resource reference"}</a>
          <a href="#${anchor("host")}">Host / Cloud boundary</a>
        </aside>
        <article class="doc-prose">
          <section id="${anchor("current-source")}">
            <h2>${ja ? "現行v2を利用する" : "Use the current v2 provider"}</h2>
            <p>${ja
              ? `provider v${currentProvider}はRegistry公開済みです。次のexact pinをTerraformまたはOpenTofuでinstallできます。`
              : `Provider v${currentProvider} is published in the Registry. Terraform and OpenTofu can install the exact pin below.`}</p>
            <figure class="code-card"><figcaption>${ja ? "exact Registry pin" : "Exact Registry pin"}</figcaption><pre><code>${providerPin(currentProvider)}</code></pre></figure>
            <p>${ja
              ? "repoの完全gateは、release descriptorに固定されたGo toolchainでproviderをbuildし、TerraformとOpenTofuの隔離されたdev overrideを使って9リソースのlifecycleを検証します。"
              : "The repository gate builds the provider with the Go toolchain pinned by the release descriptor, then exercises all nine resources through isolated Terraform and OpenTofu development overrides."}</p>
            <figure class="code-card"><figcaption>${ja ? "推奨するsource評価" : "Reviewed source evaluation"}</figcaption><pre><code>bun run check:current-form-candidates
go run ./cmd/provider-lifecycle-conformance matrix \
  --opentofu tofu --terraform terraform</code></pre></figure>
            <p>${ja ? "実hostで試す場合は、hostがexact v1alpha2 FormRefをadvertiseしていることを先に確認してください。" : "Against a real host, first verify that the host advertises the exact v1alpha2 FormRef."}</p>
          </section>

          <section id="${anchor("legacy")}">
            <h2>${ja ? "公開済みLegacyを保守する" : "Maintain published Legacy"}</h2>
            <p>${ja
              ? `既存のv1 state、refresh、delete、recoveryには公開済みprovider v${legacyProvider}を固定します。新しいv2 semanticsへ自動変換しません。`
              : `Pin published provider v${legacyProvider} for existing v1 state, refresh, delete, and recovery. It does not turn that state into v2 semantics.`}</p>
            <figure class="code-card"><figcaption>${ja ? "Registryからinstall可能" : "Installable from the Registry"}</figcaption><pre><code>${providerPin(legacyProvider)}</code></pre></figure>
          </section>

          <section id="${anchor("migration")}">
            <h2>${ja ? "stateを編集せずに移行する" : "Migrate without editing state"}</h2>
            <ol class="steps">
              <li>${ja ? "provider v1を固定し、Legacy resourceをrefreshする。" : "Pin provider v1 and refresh the Legacy resource."}</li>
              <li>${ja ? "secretを除くdesired configurationと必要なpublic outputを記録する。" : "Capture non-secret desired configuration and required public outputs."}</li>
              <li>${ja ? "exact v1alpha2 FormRefへ新規createするか、host conformanceが証明されたresourceだけimportする。" : "Create under the exact v1alpha2 FormRef, or import only with host conformance proof."}</li>
              <li>${ja ? "consumerを切り替えてobserveし、rollback不要後にv1でLegacyをdeleteする。" : "Move consumers, observe the result, then delete Legacy through v1 after rollback is no longer needed."}</li>
            </ol>
            <p><a class="text-link" href="${sourceLink("release/migrations/v1-to-v2.md")}">${ja ? "完全なmigration boundary" : "Read the complete migration boundary"}</a></p>
          </section>

          <section id="${anchor("resources")}">
            <h2>${ja ? "現行resource reference" : "Current resource reference"}</h2>
			<p>${ja ? "9種類はすべてProposalから生成した未公開の0.1.0候補で、明示的なlifecycle transitionを経るまでExperimentalではありません。各linkはmain branch上のgenerated provider referenceです。" : "All nine are equally ranked Proposal-derived, unpublished 0.1.0 candidates. None is Experimental until its explicit lifecycle transition. Each link opens the generated provider reference on main."}</p>
            <div class="resource-grid">${resourceRows(lang, true)}
              <article class="resource"><a class="resource-link" href="${sourceLink("docs/data-sources/interface.md")}"><code>data.takoform_interface</code></a><span>${ja ? "read-only host projection" : "Read-only host projection"}</span></article>
            </div>
          </section>

          <section id="${anchor("host")}">
            <h2>Host / Cloud boundary</h2>
            <p>${ja
              ? "Takoformはworkload semantics、schema、exact identity、package、conformanceだけを所有します。host profileはcapability support、adapterは外部互換、operatorはplacement・routing・scaling・credential・recoveryを所有します。Takosumi Cloudはmanaged capacity、billing、quota、SLAを所有します。"
              : "Takoform owns only workload semantics, schemas, exact identities, packages, and conformance. Host profiles own capability support; adapters own external compatibility; operators own placement, routing, scaling, credentials, and recovery. Takosumi Cloud owns managed capacity, billing, quota, and SLA."}</p>
            <p><a class="text-link" href="${sourceLink("spec/portability-boundary.md")}">${ja ? "portable fieldの採用基準を読む" : "Read the portable-field admission rule"}</a></p>
            ${statusDisclosure(lang)}
          </section>
        </article>
      </div>
    </main>${footer(lang)}`;
}

function spec(lang) {
  const ja = lang === "ja";
  const anchor = (name) => (ja ? `${name}-ja` : name);
  const rows = [
    ["Provider distribution", `v${currentProvider} current · v${legacyProvider} Legacy`, ja ? "両方Registry公開済み。Form maturityとは独立。" : "Both are Registry-published; independent from Form maturity."],
    ["Current Host wire", currentHostWireApi, ja ? "versioned discoveryから到達するprovider v2 contract。" : "Provider-v2 contract reached through versioned discovery."],
    ["Legacy Host wire", legacyHostWireApi, ja ? "非versioned discoveryに凍結したprovider v1互換lane。" : "Frozen provider-v1 lane behind unversioned discovery."],
    ["Exact current FormRef", currentFormApi, ja ? "kind、0.1.0、schema digestと一体のidentity。" : "Identity joined with kind, 0.1.0, and schema digest."],
    ["Current package envelope", packageApi, ja ? "exact FormRefとdata-only fixtureを運ぶ。" : "Carries the exact FormRef and data-only fixtures."],
  ];
  return `${nav(lang, "spec")}
    <main>
      <section class="doc-hero compact">
        <div class="container">
          <p class="project-line">Specification map · Experimental</p>
          <h1>${ja ? "4つのlayerに、4つの責務。" : "Four layers. Four different jobs."}</h1>
          <p class="lede">${ja
            ? "同じkind名でもepoch、definition version、digestが違えば別contractです。互換性は名前から推測しません。"
            : "The same kind name is a different contract when its epoch, definition version, or digest changes. Compatibility is never inferred from the name."}</p>
        </div>
      </section>
      <div class="container doc-layout">
        <aside class="doc-index" aria-label="${ja ? "このページ" : "On this page"}">
          <a href="#${anchor("identity")}">${ja ? "Identity stack" : "Identity stack"}</a>
          <a href="#${anchor("contracts")}">${ja ? "Contract map" : "Contract map"}</a>
          <a href="#${anchor("authority")}">${ja ? "Authority" : "Authority"}</a>
          <a href="#${anchor("schemas")}">Schemas</a>
          <a href="#${anchor("prior-art")}">Prior art</a>
        </aside>
        <article class="doc-prose">
          <section id="${anchor("identity")}">
            <h2>${ja ? "identity stack" : "Identity stack"}</h2>
            <div class="spec-table">${rows.map(([label, identity, detail]) => `<div><span>${label}</span><code>${identity}</code><p>${detail}</p></div>`).join("")}</div>
          </section>

          <section id="${anchor("contracts")}">
            <h2>${ja ? "公開contract map" : "Public contract map"}</h2>
            <ul class="reference-list">
              <li><a href="${sourceLink("spec/portability-boundary.md")}">Portable Form boundary</a><span>${ja ? "Form、host profile、adapter、operator、Service Offeringの責務を分離。" : "Separates Form, host profile, adapter, operator, and Service Offering ownership."}</span></li>
              <li><a href="${sourceLink("spec/form-definition/README.md")}">FormRef / Form Definition</a><span>${ja ? "portable desired、observed、output schema。" : "Portable desired, observed, and output schemas."}</span></li>
              <li><a href="${sourceLink("spec/form-package/README.md")}">Form Package</a><span>${ja ? "exact definitionとdata-only fixtureのimmutable bundle。" : "Immutable bundle of one exact definition and data-only fixtures."}</span></li>
              <li><a href="${sourceLink("spec/host-api/README.md")}">Host API</a><span>${ja ? "discovery、preview/apply、observe、refresh、delete。" : "Discovery, preview/apply, observe, refresh, and delete."}</span></li>
              <li><a href="${sourceLink("spec/interface-declaration/README.md")}">Interface projection</a><span>${ja ? "Form由来のread-only descriptor。credentialやgrantではない。" : "Read-only Form-derived descriptors; never credentials or grants."}</span></li>
              <li><a href="${sourceLink("spec/conformance.md")}">Conformance</a><span>${ja ? "各checkが証明する範囲と、証明しない範囲。" : "What each check proves—and what it does not."}</span></li>
            </ul>
          </section>

          <section id="${anchor("authority")}">
            <h2>${ja ? "maturityとavailabilityを分離する" : "Separate maturity from availability"}</h2>
            <div class="authority-grid">
              <div><strong>Takoform</strong><p>${ja ? "Proposal、Experimental、Stable、Legacy。schemaとportable contractのauthority。" : "Proposal, Experimental, Stable, and Legacy; authority for schemas and portable contracts."}</p></div>
              <div><strong>Host profile</strong><p>${ja ? "exact Form Supportとruntime・engineなどのcapability support。" : "Exact Form Support and runtime, engine, and other capability support."}</p></div>
              <div><strong>Adapter / operator</strong><p>${ja ? "外部互換、activation、target、placement、scaling、credential、operation。" : "External compatibility, activation, targets, placement, scaling, credentials, and operations."}</p></div>
              <div><strong>Cloud operator</strong><p>${ja ? "managed availability、capacity、billing、quota、SLA。" : "Managed availability, capacity, billing, quota, and SLA."}</p></div>
            </div>
          </section>

          <section id="${anchor("schemas")}">
            <h2>Normative schemas</h2>
            <ul class="reference-list">
              <li><a href="/schemas/v1alpha2/form-ref.schema.json"><code>v1alpha2/form-ref</code></a><span>Exact current Form identity.</span></li>
              <li><a href="/schemas/v1alpha2/form-definition.schema.json"><code>v1alpha2/form-definition</code></a><span>Current definition document.</span></li>
              <li><a href="/schemas/v1alpha3/package-index.schema.json"><code>v1alpha3/package-index</code></a><span>Current package envelope.</span></li>
              <li><a href="/schemas/v1alpha1/host-api-wire.schema.json"><code>v1alpha1/host-api-wire</code></a><span>Outer lifecycle wire contract.</span></li>
            </ul>
          </section>

          <section id="${anchor("prior-art")}">
            <h2>${ja ? "新規性を仮定しない" : "Do not assume novelty"}</h2>
            <p>${ja
              ? "新しいFormはOCCI、CIMI、TOSCA、Kubernetes、Crossplane、provider-native resourceを先に調査します。Takoformは独立実装やadoptionがない限りindustry standardを名乗りません。"
              : "Every new Form starts with OCCI, CIMI, TOSCA, Kubernetes, Crossplane, and provider-native prior art. Takoform does not claim to be an industry standard without independent implementations and adoption."}</p>
            <p><a class="text-link" href="${sourceLink("spec/decisions/0006-v1alpha2-restarts-form-lines.md")}">${ja ? "v1alpha2 epoch decisionを読む" : "Read the v1alpha2 epoch decision"}</a></p>
            ${statusDisclosure(lang)}
          </section>
        </article>
      </div>
    </main>${footer(lang)}`;
}

const outputs = new Map([
  [
    path.join(publicRoot, "index.html"),
    page({
      title: "Takoform — portable contracts with explicit limits",
      description: "Takoform is an Experimental resource-contract project with nine current v1alpha2 candidates and an explicit Legacy recovery lane.",
      canonicalPath: "/",
      bodyClass: "page-overview",
      english: overview("en"),
      japanese: overview("ja"),
    }),
  ],
  [
    path.join(publicRoot, "docs", "index.html"),
    page({
      title: "Documentation — Takoform",
      description: "Choose between the published Takoform v2 current provider and published v1 Legacy recovery, then use the exact task-based references.",
      canonicalPath: "/docs/",
      bodyClass: "page-reference page-docs",
      english: docs("en"),
      japanese: docs("ja"),
    }),
  ],
  [
    path.join(publicRoot, "spec", "index.html"),
    page({
      title: "Specification map — Takoform",
      description: "Takoform specification epochs, exact identity stack, authority boundaries, normative schemas, and prior-art policy.",
      canonicalPath: "/spec/",
      bodyClass: "page-reference page-spec",
      english: spec("en"),
      japanese: spec("ja"),
    }),
  ],
]);

const drift = [];
for (const [filePath, expected] of outputs) {
  if (write) {
    mkdirSync(path.dirname(filePath), { recursive: true });
    writeFileSync(filePath, expected);
    continue;
  }
  let actual = "";
  try {
    actual = readFileSync(filePath, "utf8");
  } catch {}
  if (actual !== expected) {
    drift.push(path.relative(repositoryRoot, filePath));
  }
}

if (drift.length > 0) {
  throw new Error(
    `generated website snapshot is stale: ${drift.join(", ")}; run bun run website:build`,
  );
}

console.log(
  `${write ? "Wrote" : "Verified"} ${outputs.size} website pages from ` +
    `${resources.length} current candidates and provider publication truth.`,
);

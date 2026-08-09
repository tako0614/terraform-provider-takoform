<script setup lang="ts">
// Every page states which tier it belongs to and which repository commit
// produced it.
//
// The tier facts come from themeConfig, derived from the repository at build
// time (.vitepress/site-status.mjs), so they are static HTML and cannot rot
// against the repository. The source commit is fetched from the same document
// the site serves at /.well-known/takoform-site.json, because a page that
// contained the commit it was built from could never be rebuilt to itself and
// scripts/check-website-dist.mjs requires exactly that.
import { computed, onMounted, ref } from "vue";
import { useData } from "vitepress";

type SiteStatus = {
  providerCurrent: string;
  edgePreviewProvider: string;
  edgeFamilyStatus: string;
  candidateSetDigest: string;
  openPublicationBlockers: number;
  route: string;
};

const { lang, theme } = useData();
const status = computed<SiteStatus>(() => theme.value.siteStatus as SiteStatus);
const sourceCommit = ref("");
const shortCommit = computed(() => sourceCommit.value.slice(0, 12));

onMounted(async () => {
  try {
    const response = await fetch(status.value.route, { cache: "no-cache" });
    if (!response.ok) {
      return;
    }
    const record = await response.json();
    if (
      typeof record?.sourceCommit === "string" &&
      /^[0-9a-f]{40}$/.test(record.sourceCommit)
    ) {
      sourceCommit.value = record.sourceCommit;
    }
  } catch {
    // The link below still reaches the document directly.
  }
});
</script>

<template>
  <footer v-if="status" class="site-status-footer">
    <div class="site-status-footer__inner">
      <p v-if="lang === 'ja'">
        <strong>公開済み</strong>: provider v{{ status.providerCurrent }}、保持される
        v1alpha2 リソース。
        <strong>Edge preview</strong>: provider {{ status.edgePreviewProvider }}、Edge
        Family は {{ status.edgeFamilyStatus }}、公開ブロッカー
        {{ status.openPublicationBlockers }} 件が open。
      </p>
      <p v-else>
        <strong>Current published</strong>: provider v{{ status.providerCurrent }},
        retained v1alpha2 resources.
        <strong>Edge preview</strong>: provider {{ status.edgePreviewProvider }},
        edge family {{ status.edgeFamilyStatus }},
        {{ status.openPublicationBlockers }} publication blockers open.
      </p>
      <p class="site-status-footer__provenance">
        <span v-if="lang === 'ja'">この build の source commit</span>
        <span v-else>Built from source commit</span>
        <code v-if="sourceCommit">{{ shortCommit }}</code>
        <a :href="status.route">takoform-site.json</a>
      </p>
    </div>
  </footer>
</template>

<style scoped>
.site-status-footer {
  border-top: 1px solid var(--vp-c-divider);
  padding: 16px 24px 32px;
}

.site-status-footer__inner {
  margin: 0 auto;
  max-width: 1152px;
}

.site-status-footer p {
  color: var(--vp-c-text-2);
  font-size: 13px;
  line-height: 1.7;
  margin: 0;
}

.site-status-footer__provenance {
  margin-top: 4px;
}

.site-status-footer code {
  font-size: 12px;
  margin-left: 6px;
}

.site-status-footer a {
  color: var(--vp-c-brand-1);
  margin-left: 6px;
}
</style>

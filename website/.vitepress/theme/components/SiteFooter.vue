<script setup lang="ts">
// Every page states which tier it belongs to.
//
// The tier facts come from themeConfig, derived from the repository at build
// time (.vitepress/site-status.mjs), so they are static HTML and cannot rot
// against the repository. The footer links the same facts as machine-readable
// JSON at /.well-known/takoform-site.json.
//
// No commit is displayed. A commit id inside these bytes could only name the
// parent of the commit that carries them, and scripts/check-website-dist.mjs
// requires a fresh build to reproduce every published byte. The commit that
// produced a deployment is recorded in the takoform-website Worker version
// message instead, where it can be true.
import { computed } from "vue";
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
      <p class="site-status-footer__data">
        <span v-if="lang === 'ja'">同じ事実を機械可読で</span>
        <span v-else>The same facts as data</span>
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

.site-status-footer__data {
  margin-top: 4px;
}

.site-status-footer a {
  color: var(--vp-c-brand-1);
  margin-left: 6px;
}
</style>

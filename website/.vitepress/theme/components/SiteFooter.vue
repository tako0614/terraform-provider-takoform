<script setup lang="ts">
// Keep the common footer reader-first. Detailed release evidence lives on the
// versions page; the footer only exposes the
// current Provider/API checkpoint and generated mapping count.
import { computed } from "vue";
import { useData } from "vitepress";

type SiteStatus = {
  providerPublished: string;
  currentFormCount: number;
};

const { lang, theme } = useData();
const status = computed<SiteStatus>(() => theme.value.siteStatus as SiteStatus);
</script>

<template>
  <footer v-if="status" class="site-status-footer">
    <div class="site-status-footer__inner">
      <p v-if="lang === 'ja'">
        Provider {{ status.providerPublished }} · API/Core 1.0.1 ·
        {{ status.currentFormCount }} mappings ·
        <a href="/ja/docs/versions.html">Versions</a>
      </p>
      <p v-else>
        Provider {{ status.providerPublished }} · API/Core 1.0.1 ·
        {{ status.currentFormCount }} mappings ·
        <a href="/docs/versions.html">Versions</a>
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

.site-status-footer a {
  color: var(--vp-c-brand-1);
  margin-left: 6px;
}
</style>

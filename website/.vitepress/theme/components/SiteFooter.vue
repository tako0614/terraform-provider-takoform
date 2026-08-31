<script setup lang="ts">
import { computed } from "vue";
import { useData } from "vitepress";

import {
  CORE_LIBRARY_VERSION,
  HOST_API_LANE,
  JAPANESE_VERSION_MODEL_ROUTE,
  VERSION_MODEL_ROUTE,
} from "../../version-model.mjs";

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
        <strong>Takoform</strong> · Host API <code>{{ HOST_API_LANE }}</code> ·
        Core <code>{{ CORE_LIBRARY_VERSION }}</code> · Provider
        <code>{{ status.providerPublished }}</code> ·
        {{ status.currentFormCount }} Forms
        <a :href="JAPANESE_VERSION_MODEL_ROUTE">バージョンモデル</a>
      </p>
      <p v-else>
        <strong>Takoform</strong> · Host API <code>{{ HOST_API_LANE }}</code> ·
        Core <code>{{ CORE_LIBRARY_VERSION }}</code> · Provider
        <code>{{ status.providerPublished }}</code> ·
        {{ status.currentFormCount }} Forms
        <a :href="VERSION_MODEL_ROUTE">Version model</a>
      </p>
    </div>
  </footer>
</template>

<style scoped>
.site-status-footer {
  border-top: 1px solid var(--vp-c-divider);
  padding: var(--space-md) var(--space-lg) var(--space-xl);
}

.site-status-footer__inner {
  margin: 0 auto;
  max-width: 1152px;
}

.site-status-footer p {
  color: var(--vp-c-text-2);
  font-size: var(--text-xs);
  line-height: 1.7;
  margin: 0;
}

.site-status-footer a {
  color: var(--vp-c-brand-1);
  margin-inline-start: var(--space-2xs);
}
</style>

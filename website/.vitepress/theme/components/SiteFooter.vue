<script setup lang="ts">
// Keep the footer focused on current identities. Numbered Specification
// receipts remain in the historical source tree and are intentionally not
// rendered as a current status stream.
import { computed } from "vue";
import { useData } from "vitepress";

type SiteStatus = {
  format: string;
  providerPublished: string;
  providerTarget: string;
  providerTargetStatus: string;
  hostApiCurrent: string;
  hostApiMaturity: string;
  formPackageApiCurrent: string;
  currentFormCount: number;
  currentFamilyCount: number;
  formMaturity: string;
  formPackagePublicationStatus: string;
  route: string;
};

const { lang, theme } = useData();
const status = computed<SiteStatus>(() => theme.value.siteStatus as SiteStatus);

const apiVersion = (value: string) => value.split("/").at(-1) ?? value;
const maturityLabel = (value: string) =>
  value.length === 0 ? value : `${value[0].toUpperCase()}${value.slice(1)}`;
const hostApiVersion = computed(() => apiVersion(status.value.hostApiCurrent));
const hostApiMaturity = computed(() =>
  maturityLabel(status.value.hostApiMaturity),
);
</script>

<template>
  <footer v-if="status" class="site-status-footer">
    <div class="site-status-footer__inner">
      <p v-if="lang === 'ja'">
        <strong>Current contract</strong>:
        Host API {{ hostApiVersion }} は {{ hostApiMaturity }} の stable wire contract です。
        {{ status.currentFamilyCount }} families / {{ status.currentFormCount }}
        exact 0.x Forms ({{ maturityLabel(status.formMaturity) }}、Form ごとに独立して version)。
        Form Package {{ status.formPackageApiCurrent }} is
        {{ status.formPackagePublicationStatus }} です。番号付き Specification receipt は履歴資料であり、
        現在の version stream ではありません。
      </p>
      <p v-else>
        <strong>Current contract</strong>:
        Host API {{ hostApiVersion }} is a {{ hostApiMaturity.toLowerCase() }} stable wire contract.
        {{ status.currentFamilyCount }} families / {{ status.currentFormCount }}
        exact 0.x Forms ({{ maturityLabel(status.formMaturity) }}, independently versioned).
        Form Package {{ status.formPackageApiCurrent }} is
        {{ status.formPackagePublicationStatus }}. Numbered Specification receipts are historical
        records, not a current version stream.
      </p>
      <p class="site-status-footer__distribution">
        <span v-if="lang === 'ja'">
          <strong>Distribution boundary</strong>: `tako0614/takoform` Provider
          {{ status.providerPublished }} は official Form の typed mapping だけを提供する software tooling です。
          Form は第三者も同じ package / verification path で配布でき、module では複数の Takoform
          Provider と industry-standard provider を組み合わせられます。
        </span>
        <span v-else>
          <strong>Distribution boundary</strong>: the `tako0614/takoform` Provider
          {{ status.providerPublished }} is software tooling with typed mappings for official Forms only,
          not a universal infrastructure provider. Third parties may distribute Forms through the same
          package and verification path, and a module may combine multiple Takoform and industry-standard
          providers.
        </span>
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

.site-status-footer__distribution,
.site-status-footer__data {
  margin-top: 4px;
}

.site-status-footer a {
  color: var(--vp-c-brand-1);
  margin-left: 6px;
}
</style>

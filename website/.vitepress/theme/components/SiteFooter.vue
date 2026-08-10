<script setup lang="ts">
// The footer renders the same status axes as StatusNote. Distribution
// availability is stated separately from the current design target so a
// candidate descriptor is never presented as a Registry publication.
import { computed } from "vue";
import { useData } from "vitepress";

type SiteStatus = {
  format: string;
  providerPublished: string;
  providerTarget: string;
  providerTargetStatus: string;
  hostApiCurrent: string;
  hostApiMaturity: string;
  formFamilyCurrent: string;
  formFamilyMaturity: string;
  formPackageApiCurrent: string;
  currentFormCount: number;
  formMaturity: string;
  formPackagePublicationStatus: string;
  candidateSetDigest: string;
  openPublicationBlockers: number;
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
const formFamilyVersion = computed(() =>
  apiVersion(status.value.formFamilyCurrent),
);
const formFamilyMaturity = computed(() =>
  maturityLabel(status.value.formFamilyMaturity),
);
const formMaturity = computed(() => maturityLabel(status.value.formMaturity));
const providerTargetStatus = computed(() =>
  status.value.providerTargetStatus === "registry-published"
    ? "Registry-published"
    : status.value.providerTargetStatus + " until Registry readback",
);
</script>

<template>
  <footer v-if="status" class="site-status-footer">
    <div class="site-status-footer__inner">
      <p v-if="lang === 'ja'">
        <strong>Current design target</strong>: Provider
        {{ status.providerTarget }} ({{ providerTargetStatus }}, descriptor
        metadata candidate-only); Host API
        {{ hostApiVersion }} ({{ hostApiMaturity }}); Edge Form Family
        {{ formFamilyVersion }} ({{ formFamilyMaturity }} family,
        {{ status.currentFormCount }} {{ formMaturity }} Form definitions,
        definition 0.1.0).
        Form Package {{ status.formPackageApiCurrent }} is
        {{ status.formPackagePublicationStatus }}.
      </p>
      <p v-else>
        <strong>Current design target</strong>: Provider
        {{ status.providerTarget }} ({{ providerTargetStatus }}, descriptor
        metadata candidate-only); Host API
        {{ hostApiVersion }} ({{ hostApiMaturity }}); Edge Form Family
        {{ formFamilyVersion }} ({{ formFamilyMaturity }} family,
        {{ status.currentFormCount }} {{ formMaturity }} Form definitions,
        definition 0.1.0).
        Form Package {{ status.formPackageApiCurrent }} is
        {{ status.formPackagePublicationStatus }}.
      </p>
      <p class="site-status-footer__distribution">
        <span v-if="lang === 'ja'">
          Distribution availability: Provider {{ status.providerPublished }} は
          Registry readback 済みの current distribution、Provider 2.0.0 は
          公開済み compatibility predecessor、Provider 1.0.3 は公開済み
          Legacy です。
        </span>
        <span v-else>
          Distribution availability: Provider {{ status.providerPublished }} is
          the current Registry-readback distribution; Provider 2.0.0 is the
          published compatibility predecessor and Provider 1.0.3 is published
          Legacy.
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

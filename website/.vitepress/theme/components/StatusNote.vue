<script setup lang="ts">
// Locale-aware status note shared by every hand-authored page. The facts come
// from the build-time status document, while the labels make the independent
// Provider, Host API, Form Family, Form definition, and package axes readable.
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
  <div class="status-note">
    <template v-if="lang === 'ja'">
      <p>
        <strong>Current design target</strong>: Provider
        <code>{{ status.providerTarget }}</code> は Registry-published の
        stable release target です。Provider availability は
        <code>{{ providerTargetStatus }}</code>、repository descriptor metadata は
        owner publication 後も <code>candidate-only</code> のままです。Host API
        <code>{{ hostApiVersion }}</code> は
        {{ hostApiMaturity }} protocol、Edge Form Family
        <code>{{ formFamilyVersion }}</code> は {{ formFamilyMaturity }} family で、
        その {{ status.currentFormCount }} 個の Form definition は {{ formMaturity }}
        （definition
        <code>0.1.0</code>）です。Form Package envelope は
        <code>{{ status.formPackageApiCurrent }}</code> で、artifact は
        <code>{{ status.formPackagePublicationStatus }}</code> です。
      </p>
      <p>
        <strong>Distribution availability</strong>: Provider
        <code>{{ status.providerPublished }}</code> は Registry readback 済みの
        current distribution、Provider <code>2.0.0</code> は公開済みの
        compatibility predecessor、Provider <code>1.0.3</code> は公開済み
        Legacy client です。これは current design target の protocol / family
        maturity とは別の事実です。
      </p>
    </template>
    <template v-else>
      <p>
        <strong>Current design target</strong>: Provider
        <code>{{ status.providerTarget }}</code> is the Registry-published
        stable release target. Provider availability is
        <code>{{ providerTargetStatus }}</code>; repository descriptor metadata
        remains <code>candidate-only</code> after owner publication. Host API
        <code>{{ hostApiVersion }}</code> is a
        {{ hostApiMaturity }} protocol. Edge Form Family
        <code>{{ formFamilyVersion }}</code> is a {{ formFamilyMaturity }} family;
        its {{ status.currentFormCount }} Form definitions are {{ formMaturity }}
        at definition <code>0.1.0</code>. The Form Package envelope is
        <code>{{ status.formPackageApiCurrent }}</code
        >, and its artifacts are
        <code>{{ status.formPackagePublicationStatus }}</code
        >.
      </p>
      <p>
        <strong>Distribution availability</strong>: Provider
        <code>{{ status.providerPublished }}</code> is the current
        Registry-readback distribution; Provider <code>2.0.0</code> is the
        published compatibility predecessor and Provider <code>1.0.3</code>
        remains the published Legacy client. This is separate from the current
        target's protocol and family maturity.
      </p>
    </template>
  </div>
</template>

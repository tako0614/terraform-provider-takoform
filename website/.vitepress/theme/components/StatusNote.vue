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
  hostApiPublicationStatus: string;
  formPackageApiCurrent: string;
  currentFormCount: number;
  currentFamilyCount: number;
  formMaturity: string;
  formPackagePublicationStatus: string;
  specificationVersion: string;
  specificationReleaseStatus: string;
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
const formMaturity = computed(() => maturityLabel(status.value.formMaturity));
</script>

<template>
  <div class="status-note">
    <template v-if="lang === 'ja'">
      <p>
        <strong>Historical Specification receipt</strong>:
        Takoform Specification <code>{{ status.specificationVersion }}</code> は
        <code>{{ status.specificationReleaseStatus }}</code> の W09 historical metadata です。
        Public API/Core checkpoint は <code>1.0.1</code> で、Host API
        <code>{{ hostApiVersion }}</code> の実装・support・deployment とは別です。
        <code>{{ status.hostApiPublicationStatus }}</code> は retained compatibility metadata です。
        Active Edge publisher source は 16 個の candidate Form を持ち、Provider 3 は
        {{ status.currentFamilyCount }} family / {{ status.currentFormCount }} 個の typed mapping を
        compatibility history として保持します。Form Package envelope は
        <code>{{ status.formPackageApiCurrent }}</code> で、artifact は
        <code>{{ status.formPackagePublicationStatus }}</code> です。
      </p>
      <p>
        <strong>Distribution availability</strong>: Provider
        <code>{{ status.providerPublished }}</code> は Registry readback 済みの
        current retained distribution、Provider <code>2.0.0</code> は公開済みの
        compatibility predecessor、Provider <code>1.0.3</code> は公開済み
        Legacy client です。Provider 3 は独立した non-normative implementation
        track で、Specification release を block しません。
      </p>
    </template>
    <template v-else>
      <p>
        <strong>Historical Specification receipt</strong>: Takoform Specification
        <code>{{ status.specificationVersion }}</code> is
        <code>{{ status.specificationReleaseStatus }}</code> W09 compatibility metadata.
        The public API/Core checkpoint is <code>1.0.1</code>; Host API
        <code>{{ hostApiVersion }}</code> implementation, support, and deployment are separate.
        <code>{{ status.hostApiPublicationStatus }}</code> remains retained compatibility metadata.
        The active Edge publisher source has 16 candidate Forms, while Provider 3 retains
        {{ status.currentFamilyCount }} families / {{ status.currentFormCount }} typed mappings as
        compatibility history. The Form Package envelope is
        <code>{{ status.formPackageApiCurrent }}</code
        >, and its artifacts are
        <code>{{ status.formPackagePublicationStatus }}</code
        >.
      </p>
      <p>
        <strong>Distribution availability</strong>: Provider
        <code>{{ status.providerPublished }}</code> is the current
        Registry-readback retained distribution; Provider <code>2.0.0</code> is the
        published compatibility predecessor and Provider <code>1.0.3</code>
        remains the published Legacy client. Provider 3 is an independent,
        non-normative implementation track and cannot block the Specification.
      </p>
    </template>
  </div>
</template>

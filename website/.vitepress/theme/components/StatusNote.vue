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
        <strong>Specification candidate</strong>:
        Takoform Specification <code>{{ status.specificationVersion }}</code> は
        <code>{{ status.specificationReleaseStatus }}</code> です。Host API
        <code>{{ hostApiVersion }}</code> は {{ hostApiMaturity }} contract の
        <code>{{ status.hostApiPublicationStatus }}</code> で、
        current corpus の {{ status.currentFamilyCount }} 個の versionless family /
        {{ status.currentFormCount }} 個の exact <code>0.x</code> Form は
        {{ formMaturity }} のままです。Specification release は Form を
        <code>1.0.0</code> に昇格させません。Form Package envelope は
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
        <strong>Specification candidate</strong>: Takoform Specification
        <code>{{ status.specificationVersion }}</code> is
        <code>{{ status.specificationReleaseStatus }}</code>. Host API
        <code>{{ hostApiVersion }}</code> is a {{ hostApiMaturity }} contract and
        remains <code>{{ status.hostApiPublicationStatus }}</code>,
        while the current corpus's {{ status.currentFamilyCount }} versionless
        families and {{ status.currentFormCount }} exact <code>0.x</code> Forms
        remain {{ formMaturity }}. Releasing the Specification does not promote
        those Forms to <code>1.0.0</code>. The Form Package envelope is
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

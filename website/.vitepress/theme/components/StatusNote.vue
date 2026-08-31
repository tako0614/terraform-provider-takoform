<script setup lang="ts">
// Locale-aware status note shared by every hand-authored page. The facts come
// from the build-time status document, while the labels make the current
// Host API, Form, package, and Provider boundaries readable. Specification
// receipts are historical and are not rendered as a current version stream.
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
  <div class="status-note">
    <template v-if="lang === 'ja'">
      <p>
        <strong>Current contract</strong>:
        Host API <code>{{ hostApiVersion }}</code> は {{ hostApiMaturity }} の stable wire contract です。
        current corpus の {{ status.currentFamilyCount }} 個の versionless family /
        {{ status.currentFormCount }} 個の exact <code>0.x</code> Form は
        {{ maturityLabel(status.formMaturity) }} で、Form ごとに独立して version されます。Form Package envelope は
        <code>{{ status.formPackageApiCurrent }}</code> で、artifact は
        <code>{{ status.formPackagePublicationStatus }}</code> です。
        番号付き Specification receipt は履歴資料であり、現在の version stream ではありません。
      </p>
      <p>
        <strong>Distribution boundary</strong>: `tako0614/takoform` Provider
        <code>{{ status.providerPublished }}</code> は official Form の typed mapping だけを提供する software tooling です。
        Form は第三者も同じ package / verification path で配布でき、module では複数の Takoform
        Provider と industry-standard provider を組み合わせられます。
      </p>
    </template>
    <template v-else>
      <p>
        <strong>Current contract</strong>: Host API
        <code>{{ hostApiVersion }}</code> is a {{ hostApiMaturity.toLowerCase() }} stable wire contract.
        The current corpus's {{ status.currentFamilyCount }} versionless
        families and {{ status.currentFormCount }} exact <code>0.x</code> Forms
        remain {{ maturityLabel(status.formMaturity) }} and are independently versioned. The Form Package envelope is
        <code>{{ status.formPackageApiCurrent }}</code
        >, and its artifacts are
        <code>{{ status.formPackagePublicationStatus }}</code
        >.
        Numbered Specification receipts are historical records, not a current version stream.
      </p>
      <p>
        <strong>Distribution boundary</strong>: the `tako0614/takoform` Provider
        <code>{{ status.providerPublished }}</code> is software tooling with typed mappings for official Forms only,
        not a universal infrastructure provider. Third parties may distribute Forms through the same package and
        verification path, and a module may combine multiple Takoform and industry-standard providers.
      </p>
    </template>
  </div>
</template>

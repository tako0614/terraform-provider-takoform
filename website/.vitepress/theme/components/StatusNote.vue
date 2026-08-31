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
        <strong>現在の契約</strong>:
        Host API <code>{{ hostApiVersion }}</code> は安定した wire contract です。
        現在の publisher Form corpus は versionless な Edge family 一つ（exact な Experimental Form 16個）で、
        Form roster は <a href="https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e">publisher roster（英語のみ）</a> を参照してください。Form は
        {{ maturityLabel(status.formMaturity) }} で、Form ごとに独立して version されます。
        Form Package envelope は <code>{{ status.formPackageApiCurrent }}</code> で、artifact は
        <code>{{ status.formPackagePublicationStatus }}</code> です。番号付き Specification
        receipt は履歴資料であり、現在の version stream ではありません。
      </p>
      <p>
        <strong>配布境界</strong>: `tako0614/takoform` Provider
        <code>{{ status.providerPublished }}</code> は明示的に対応する official Form だけを扱う typed tooling です。
        公開済み Provider の旧 projection は履歴であり、Edge16 の official-only mapping は次 major candidate（未公開）です。
        第三者も同じ package / verification path で Form を配布でき、module では複数の Takoform Provider と
        業界標準 provider を組み合わせられます。
      </p>
    </template>
    <template v-else>
      <p>
        <strong>Current contract</strong>: Host API
        <code>{{ hostApiVersion }}</code> is the {{ hostApiMaturity.toLowerCase() }} wire contract.
        The publisher's current corpus is one versionless Edge family with 16 exact Experimental Forms; see the
        <a href="https://github.com/tako0614/takoform-forms/tree/3a395e4d7f9f652a942da52905857fccc41b467e">publisher roster</a> for its exact Forms. Those Forms remain
        {{ maturityLabel(status.formMaturity) }} and are independently versioned. The Form Package envelope is
        <code>{{ status.formPackageApiCurrent }}</code>, and its artifacts are
        <code>{{ status.formPackagePublicationStatus }}</code>.
        Numbered Specification receipts are historical records, not a current version stream.
      </p>
      <p>
        <strong>Distribution boundary</strong>: the `tako0614/takoform` Provider
        <code>{{ status.providerPublished }}</code> is official-Forms-only software tooling, not a universal infrastructure provider.
        Its released aggregate is historical Provider metadata; the official-only Edge16 mapping is a next-major candidate and
        remains unpublished. Third parties may distribute Forms through the same package and verification path, and a module
        may combine multiple Takoform and industry-standard providers.
      </p>
    </template>
  </div>
</template>

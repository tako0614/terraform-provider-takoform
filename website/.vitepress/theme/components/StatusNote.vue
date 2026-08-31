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
};

const { lang, theme } = useData();
const status = computed<SiteStatus>(() => theme.value.siteStatus as SiteStatus);
</script>

<template>
  <div class="status-note">
    <p v-if="lang === 'ja'">
      <strong>バージョンモデル</strong>：独立した 4 つの流れを使います。Host API
      は <code>{{ HOST_API_LANE }}</code
      >、各 Form は <code>definitionVersion</code>、Core ライブラリは SemVer
      <code>{{ CORE_LIBRARY_VERSION }}</code
      >、Provider は SemVer
      <code>{{ status.providerPublished }}</code> です。Form Package / schema ID
      / digest と過去の Specification receipt は artifact の証拠であり、version
      stream ではありません。
      <a :href="JAPANESE_VERSION_MODEL_ROUTE">モデルの詳細</a>。
    </p>
    <p v-else>
      <strong>Version model</strong>: four independent streams: Host API
      <code>{{ HOST_API_LANE }}</code
      >, each Form's <code>definitionVersion</code>, Core library SemVer
      <code>{{ CORE_LIBRARY_VERSION }}</code
      >, and Provider SemVer <code>{{ status.providerPublished }}</code
      >. Form Package/schema IDs, digests, and historical Specification receipts
      are artifact evidence, not version streams.
      <a :href="VERSION_MODEL_ROUTE">Read the model</a>.
    </p>
  </div>
</template>

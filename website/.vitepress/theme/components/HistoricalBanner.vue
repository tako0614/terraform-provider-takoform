<script setup lang="ts">
import { computed } from "vue";
import { useData } from "vitepress";

const { lang, page } = useData();

const historicalKind = computed(() => {
  const relativePath = page.value.relativePath.replaceAll("\\", "/");
  // The `/spec/` path contains both current contract leaves and retained
  // numbered/withdrawn source. Current Host API, Form, Core, Interface,
  // Binding, artifact, trust, and conformance contracts are linked directly
  // from Current navigation and should not be presented as archive pages.
  const currentContract =
    /^spec\/(?:host-api\/v1\.md|form-definition\/|form-package\/|core\/|interface-contract\/|binding-contract\/|artifact-transport\/|trust\/|conformance\.md$|form-families\.md$|portability-boundary\.md$|project-lifecycle\.md$|versioning\.md$)/u.test(
      relativePath,
    );
  if (currentContract) {
    return null;
  }
  if (
    relativePath.startsWith("spec/") ||
    relativePath.startsWith("ja/spec/") ||
    relativePath.startsWith("release/")
  ) {
    return "evidence";
  }
  return null;
});
</script>

<template>
  <aside v-if="historicalKind" class="historical-banner" role="note">
    <p v-if="lang === 'ja'">
      <strong>履歴資料</strong>：この URL は不変の Specification 1.0 / 1.1 receipt と
      withdrawn compatibility evidence を参照するために保持しています。Specification は
      現在の version stream ではありません。<a href="/ja/docs/versions.html"
        >現行のバージョンモデルを見る</a
      >。
    </p>
    <p v-else>
      <strong>Historical source</strong>: this URL is retained for immutable
      Specification 1.0/1.1 receipts and withdrawn compatibility evidence.
      Specification is not a current version stream.
      <a href="/docs/versions.html">Read the current version model</a>.
    </p>
  </aside>
</template>

<style scoped>
.historical-banner {
  margin: 16px auto 0;
  max-width: 1152px;
  padding: 10px 24px;
  border: 1px solid var(--vp-c-warning-2);
  border-radius: 8px;
  background: var(--vp-c-warning-soft);
  color: var(--vp-c-text-2);
  font-size: 13px;
  line-height: 1.6;
}

.historical-banner p {
  margin: 0;
}

.historical-banner a {
  color: var(--vp-c-brand-1);
}
</style>

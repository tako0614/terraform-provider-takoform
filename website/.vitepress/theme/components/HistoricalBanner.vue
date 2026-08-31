<script setup lang="ts">
import { computed } from "vue";
import { useData } from "vitepress";

const { lang, page } = useData();

const historicalKind = computed(() => {
  const relativePath = page.value.relativePath.replaceAll("\\", "/");
  if (
    relativePath.startsWith("spec/") ||
    relativePath.startsWith("ja/spec/") ||
    relativePath.startsWith("release/")
  ) {
    return "evidence";
  }
  if (
    relativePath === "docs/reference.md" ||
    relativePath === "docs/glossary.md"
  ) {
    return "projection";
  }
  return null;
});
</script>

<template>
  <aside v-if="historicalKind" class="historical-banner" role="note">
    <template v-if="historicalKind === 'evidence'">
      <p v-if="lang === 'ja'">
        <strong>履歴資料</strong>：この URL は不変の Specification 1.1 receipt
        と、withdrawn compatibility evidence
        を参照するために維持しています。Specification は現行の version stream
        ではありません。<a href="/ja/docs/versions.html"
          >現行のバージョンモデルを見る</a
        >。
      </p>
      <p v-else>
        <strong>Historical source</strong>: this URL is retained for the
        immutable Specification 1.1 receipt and withdrawn compatibility
        evidence. Specification is not a current version stream.
        <a href="/docs/versions.html">Read the current version model</a>.
      </p>
    </template>
    <template v-else>
      <p v-if="lang === 'ja'">
        <strong>旧 projection</strong>：この URL は generated reference /
        glossary projection として 保持しています。現行の互換性と概念は、<a
          href="/ja/docs/versions.html"
          >バージョンモデル</a
        >から確認してください。
      </p>
      <p v-else>
        <strong>Legacy projection</strong>: this URL is retained as a generated
        reference or glossary projection. Start with the
        <a href="/docs/versions.html">current version model</a> for
        compatibility and concepts.
      </p>
    </template>
  </aside>
</template>

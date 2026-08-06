<script setup lang="ts">
// Locale-aware status note shared by every hand-authored page. This keeps the
// single source of truth for the project-status facts in one component
// instead of duplicating the block in every Markdown file.
import { useData } from "vitepress";

const { lang } = useData();
</script>

<template>
  <div class="status-note">
    <p v-if="lang === 'ja'">
      Takoform は <strong>Experimental specification project</strong> です。現在の設計レーンは
      <code>edge.forms.takoform.com/v1alpha1</code> family で、
      <code>forms.takoform.com/v1alpha3</code> Host API の上で動き、package envelope は
      <code>packages.forms.takoform.com/v1alpha4</code> です。
      <code>forms.takoform.com/v1alpha2</code> epoch と
      <code>packages.forms.takoform.com/v1alpha3</code> envelope は provider-v2 の互換面として
      保持されます。provider <code>v2.0.0</code> は現在の公開済み client、provider
      <code>v1.0.3</code> は公開済みの Legacy client です。provider <code>v2.1.0</code> は
      未公開の source candidate です。
      <code>forms.takoform.com/v1alpha1</code> の公開済み Form Package identity
      34件は、不変の Legacy 証跡です。現在、中央による承認や admission はなく、
      Experimental として公開済みの Form もまだありません。
    </p>
    <p v-else>
      Takoform is an <strong>Experimental specification project</strong>. The
      current design lane is the <code>edge.forms.takoform.com/v1alpha1</code>
      family, served over the <code>forms.takoform.com/v1alpha3</code> Host API
      with <code>packages.forms.takoform.com/v1alpha4</code> package envelopes.
      The <code>forms.takoform.com/v1alpha2</code> epoch and its
      <code>packages.forms.takoform.com/v1alpha3</code> envelope are retained as
      the provider-v2 compatibility surface. Provider <code>v2.0.0</code> is the
      current published client and provider <code>v1.0.3</code> is the published
      Legacy client. Provider <code>v2.1.0</code> is an unpublished source
      candidate. The 34 published Form Package identities from
      <code>forms.takoform.com/v1alpha1</code> are immutable Legacy evidence.
      There is no current central approval or admission, and no Form has an
      Experimental publication yet.
    </p>
  </div>
</template>

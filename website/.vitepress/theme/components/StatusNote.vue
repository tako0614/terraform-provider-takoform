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
      <code>edge.forms.takoform.com/v1beta1</code> family で、
      <code>forms.takoform.com/v1beta1</code> Host API の上で動き、package envelope は
      <code>packages.forms.takoform.com/v1alpha4</code> です。
      <code>forms.takoform.com/v1alpha2</code> epoch と
      <code>packages.forms.takoform.com/v1alpha3</code> envelope は provider-v2 の互換面として
      保持されます。provider <code>v2.0.0</code> は現在の公開済み client、provider
      <code>v1.0.3</code> は公開済みの Legacy client です。stable provider <code>v2.1.1</code> は
      Beta 契約を対象とする release target で、owner が公開するまで descriptor は
      <code>candidate-only</code> です。15 Form は Experimental で package は未公開です。
      <code>forms.takoform.com/v1alpha1</code> の公開済み Form Package identity
      34件は、不変の Legacy 証跡です。現在、中央による承認や admission はなく、
      Beta は API/family channel であり、Form の Stable/GA claim ではありません。
    </p>
    <p v-else>
      Takoform is an <strong>Experimental specification project</strong>. The
      current design channel is the <code>edge.forms.takoform.com/v1beta1</code>
      family, served over the <code>forms.takoform.com/v1beta1</code> Host API
      with <code>packages.forms.takoform.com/v1alpha4</code> package envelopes.
      The <code>forms.takoform.com/v1alpha2</code> epoch and its
      <code>packages.forms.takoform.com/v1alpha3</code> envelope are retained as
      the provider-v2 compatibility surface. Provider <code>v2.0.0</code> is the
      current published client and provider <code>v1.0.3</code> is the published
      Legacy client. Stable provider <code>v2.1.1</code> is the release target
      for the Beta contracts; its descriptor remains <code>candidate-only</code>
      until the owner publishes it. The 15 Forms are Experimental and their
      packages remain unpublished. The 34 published Form Package identities from
      <code>forms.takoform.com/v1alpha1</code> are immutable Legacy evidence.
      There is no current central approval or admission. Beta is an API/family
      channel, not a Stable or GA Form claim.
    </p>
  </div>
</template>

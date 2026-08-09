<script setup lang="ts">
// The default layout plus one addition: every page ends with the tier and
// provenance footer, so no page can be read without knowing whether it
// describes the published tier or the Edge preview.
import DefaultTheme from "vitepress/theme";
import SiteFooter from "./components/SiteFooter.vue";

const { Layout } = DefaultTheme;
</script>

<template>
  <Layout>
    <template #layout-bottom>
      <SiteFooter />
    </template>
  </Layout>
</template>

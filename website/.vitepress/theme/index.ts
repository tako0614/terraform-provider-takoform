import DefaultTheme from "vitepress/theme";
import Layout from "./Layout.vue";
import StatusNote from "./components/StatusNote.vue";
import "./custom.css";

export default {
  extends: DefaultTheme,
  Layout,
  enhanceApp({ app }) {
    app.component("StatusNote", StatusNote);
  },
};

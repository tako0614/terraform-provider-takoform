import DefaultTheme from "vitepress/theme";
import StatusNote from "./components/StatusNote.vue";
import "./custom.css";

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component("StatusNote", StatusNote);
  },
};

/**
 * Match the writer's sole HTML normalization: remove horizontal whitespace
 * at line ends while preserving every generated tag, attribute and byte of
 * executable content.
 */
export function normalizeGeneratedHtml(html) {
  return String(html).replace(/[ \t]+$/gmu, "");
}

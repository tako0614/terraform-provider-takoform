/*
 * The unparseable bundle of conformance/runtime-abi-v1. Its default export
 * object is never closed, so no runtime can compile this module. It exists so
 * the corpus can drive module_syntax_error against real bytes rather than
 * against a flag.
 */

export default {
  async fetch(request) {
    return new Response("unreachable");

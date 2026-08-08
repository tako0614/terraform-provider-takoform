/*
 * The fetch-only bundle of conformance/runtime-abi-v1.
 *
 * Its default export exposes fetch and nothing else. It exists so the corpus
 * can drive handler_not_exported against bytes a runtime has to read: a
 * version declaring scheduled here names a handler this module genuinely does
 * not have. The corpus never declares a handler a module does not export, so
 * every positive case stays passable by a conforming runtime.
 */

const handlers = {
  async fetch(request) {
    return new Response(
      JSON.stringify({ probe: "takoform.runtime-abi-probe@v1", method: request.method }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
  },
};

export default handlers;

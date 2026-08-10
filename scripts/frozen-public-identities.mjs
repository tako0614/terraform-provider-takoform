// Exact public bytes retained for already-minted alpha documentation. The HTML
// and its complete runtime/search asset closure move together; preserving only
// the page would leave an immutable identity that no longer loads.

export const FROZEN_PUBLIC_IDENTITIES = new Map([
  [
    "spec/host-api/v1alpha3.html",
    "b4f12508408f9c932f041db5454c0bc3c523230a4ed8afba488747b85b9ec539",
  ],
  [
    "assets/app.CACmonw9.js",
    "74cb50181418d9d236ddda0091974585c31debee269f64af80fa69667c5938e8",
  ],
  [
    "assets/chunks/framework.BTLSZHNB.js",
    "e57c51f853f7b46fd8d5538127f3a1acc913b846825b3ebb9f94e330e99f0284",
  ],
  [
    "assets/chunks/theme.DqzvtN8i.js",
    "8d3c635318a79df5d3b94a7cd55af4fd485df757dc4bc83c88f88aac9b24a27f",
  ],
  [
    "assets/chunks/VPLocalSearchBox.BUtlVqL_.js",
    "94c7fbff418e04844d11d4dbde5b6dd5591ec4a66149ff37b82a6bada3404c8e",
  ],
  [
    "assets/chunks/@localSearchIndexja.CE6i2S8K.js",
    "23f8940bfcdc5b704e1f228d3388e26f57804c4fd7adee393d6edc295b714a6a",
  ],
  [
    "assets/chunks/@localSearchIndexroot.DtBbonOt.js",
    "1807fc088630a31f60c8d9bf5279653285e2064769064599c43e1239127827e9",
  ],
  [
    "assets/spec_host-api_v1alpha3.md.B4iy7DrK.js",
    "cffa80d1e7fa74b14059b40e8bbf195fbe4097bc6179ea52a0058341ca61da6b",
  ],
  [
    "assets/spec_host-api_v1alpha3.md.B4iy7DrK.lean.js",
    "9e15a6afb9c5adbd919faf0f3dbbe8f4e29ce1bdf37211bcc920953249afead2",
  ],
  [
    "assets/inter-roman-latin.Di8DUHzh.woff2",
    "f7ab715caa2c78facb4334b211c81ee66f037cf9c99ca3f24acd543e84a93278",
  ],
  [
    "assets/style.BYwq6E7_.css",
    "a903693ce3394e1bd1d8514c38839f2bf89e23335fd9033be0a32fb04881e4d1",
  ],
]);

export const FROZEN_PUBLIC_PAGES = new Set([
  "spec/host-api/v1alpha3.html",
]);

// The v1alpha3 page is an already-published route. Its hashmap value is part
// of that public identity, so a fresh VitePress chunk must never remap it to a
// newly generated hash. Both the complete and lean chunks are required by the
// VitePress runtime.
export const FROZEN_HASHMAP_ENTRIES = new Map([
  ["spec_host-api_v1alpha3.md", "B4iy7DrK"],
]);

// These old hashed assets coexist with the freshly generated asset having the
// same Rollup role. The font and stylesheet above are still emitted under the
// same logical role, so they remain part of the normal fresh-build role check.
export const FROZEN_EXTRA_ASSETS = new Set([
  "assets/app.CACmonw9.js",
  "assets/chunks/framework.BTLSZHNB.js",
  "assets/chunks/theme.DqzvtN8i.js",
  "assets/chunks/VPLocalSearchBox.BUtlVqL_.js",
  "assets/chunks/@localSearchIndexja.CE6i2S8K.js",
  "assets/chunks/@localSearchIndexroot.DtBbonOt.js",
  "assets/spec_host-api_v1alpha3.md.B4iy7DrK.js",
  "assets/spec_host-api_v1alpha3.md.B4iy7DrK.lean.js",
  "assets/style.BYwq6E7_.css",
]);

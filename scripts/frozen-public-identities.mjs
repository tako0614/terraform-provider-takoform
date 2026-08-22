// Exact public bytes retained for already-minted documentation that must keep
// serving after a fresh build. Empty since decision 0042: the one frozen page
// (spec/host-api/v1alpha3.html and its asset closure) was withdrawn with the
// v1alpha2 epoch, and its address is recorded as retired in
// release/published-document-lanes.json so it can never quietly come back
// meaning something else. The mechanism stays: the next pre-Stable page that
// must survive a rebuild gets pinned here.

export const FROZEN_PUBLIC_IDENTITIES = new Map();

export const FROZEN_PUBLIC_PAGES = new Set();

export const FROZEN_HASHMAP_ENTRIES = new Map();

export const FROZEN_EXTRA_ASSETS = new Set();

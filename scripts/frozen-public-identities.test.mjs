import { describe, expect, test } from "bun:test";
import { existsSync } from "node:fs";
import path from "node:path";

import {
  FROZEN_EXTRA_ASSETS,
  FROZEN_HASHMAP_ENTRIES,
  FROZEN_PUBLIC_IDENTITIES,
  FROZEN_PUBLIC_PAGES,
} from "./frozen-public-identities.mjs";

const repositoryRoot = path.resolve(import.meta.dir, "..");
const publicRoot = path.join(repositoryRoot, "website", "public");

describe("frozen public identities after the epoch withdrawal", () => {
  test("no page is frozen and the withdrawn route is gone", () => {
    // Decision 0042 withdrew the one frozen page with its epoch. An entry
    // reappearing here must come with a new page that actually needs pinning;
    // the withdrawn route coming back would be a retired address answering
    // again, which release/published-document-lanes.json exists to refuse.
    expect(FROZEN_PUBLIC_IDENTITIES.size).toBe(0);
    expect(FROZEN_PUBLIC_PAGES.size).toBe(0);
    expect(FROZEN_HASHMAP_ENTRIES.size).toBe(0);
    expect(FROZEN_EXTRA_ASSETS.size).toBe(0);
    expect(
      existsSync(path.join(publicRoot, "spec", "host-api", "v1alpha3.html")),
    ).toBe(false);
  });
});

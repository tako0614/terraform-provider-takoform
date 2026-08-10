import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  FROZEN_HASHMAP_ENTRIES,
  FROZEN_PUBLIC_IDENTITIES,
} from "./frozen-public-identities.mjs";

const repositoryRoot = path.resolve(import.meta.dir, "..");
const publicRoot = path.join(repositoryRoot, "website", "public");

describe("frozen v1alpha3 public route", () => {
  test("pins both full and lean chunks and the hashmap route", () => {
    expect(FROZEN_HASHMAP_ENTRIES.get("spec_host-api_v1alpha3.md")).toBe(
      "B4iy7DrK",
    );
    for (const suffix of [".js", ".lean.js"]) {
      const relative = `assets/spec_host-api_v1alpha3.md.B4iy7DrK${suffix}`;
      const expected = FROZEN_PUBLIC_IDENTITIES.get(relative);
      expect(expected).toMatch(/^[0-9a-f]{64}$/u);
      const actual = createHash("sha256")
        .update(readFileSync(path.join(publicRoot, relative)))
        .digest("hex");
      expect(actual).toBe(expected);
    }
  });
});

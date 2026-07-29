import { describe, expect, test } from "bun:test";

import { enforceAppendOnlyPublicSchemaIdentities } from "./public-schema-manifest.mjs";

const identity = (id, sha256 = `sha256:${"a".repeat(64)}`) => ({
  id: `https://forms.takoform.com/schemas/${id}.json`,
  public: `website/public/schemas/${id}.json`,
  sha256,
  source: `spec/schemas/${id}.json`,
});

describe("public schema identity history", () => {
  test("allows adding identities without changing retained identities", () => {
    const retained = identity("retained");
    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities(
        [retained, identity("added")],
        [{ identities: [retained], label: "previous" }],
      ),
    ).not.toThrow();
  });

  test("rejects removing a retained identity", () => {
    const retained = identity("retained");
    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities([], [
        { identities: [retained], label: "previous" },
      ]),
    ).toThrow("identity https://forms.takoform.com/schemas/retained.json was removed");
  });

  test("rejects changing retained identity bytes", () => {
    const retained = identity("retained");
    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities(
        [identity("retained", `sha256:${"b".repeat(64)}`)],
        [{ identities: [retained], label: "previous" }],
      ),
    ).toThrow("identity https://forms.takoform.com/schemas/retained.json was changed");
  });
});

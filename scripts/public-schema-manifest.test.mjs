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

  test("allows a recorded withdrawal of a retained identity", () => {
    const retained = identity("retained");
    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities(
        [],
        [{ identities: [retained], label: "previous" }],
        [{ ...retained, retiredBecause: "pre-Stable lane withdrawn" }],
      ),
    ).not.toThrow();
  });

  test("rejects a withdrawal that restates the bytes it was published with", () => {
    const retained = identity("retained");
    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities(
        [],
        [{ identities: [retained], label: "previous" }],
        [
          {
            ...identity("retained", `sha256:${"b".repeat(64)}`),
            retiredBecause: "pre-Stable lane withdrawn",
          },
        ],
      ),
    ).toThrow("was retired under different bytes");
  });

  test("rejects an identity that is both served and retired", () => {
    const retained = identity("retained");
    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities(
        [retained],
        [{ identities: [retained], label: "previous" }],
        [{ ...retained, retiredBecause: "pre-Stable lane withdrawn" }],
      ),
    ).toThrow("is both served and retired");
  });
});

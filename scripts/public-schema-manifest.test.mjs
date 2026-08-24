import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { readdirSync } from "node:fs";
import path from "node:path";

import {
  discoverPublicSchemas,
  enforceAppendOnlyPublicSchemaIdentities,
  readPublicSchemaIdentityLedger,
} from "./public-schema-manifest.mjs";

const repositoryRoot = path.resolve(import.meta.dir, "..");
const STABLE_V1_SCHEMA_IDS = [
  "https://forms.takoform.com/schemas/operations/v1/operation.schema.json",
  "https://forms.takoform.com/schemas/standards/v1/standard-service-ref.schema.json",
  "https://forms.takoform.com/schemas/support/v1/host-support-profile.schema.json",
  "https://forms.takoform.com/schemas/v1/form-definition.schema.json",
  "https://forms.takoform.com/schemas/v1/form-ref.schema.json",
  "https://forms.takoform.com/schemas/v1/host-api-wire.schema.json",
  "https://forms.takoform.com/schemas/v1/host-discovery.schema.json",
];
const OCCUPIED_IDENTITY_COUNT = 24;
const OCCUPIED_IDENTITIES_SHA256 =
  "b7a0d545d4ff8bb113991539073c377fe83e63d4f7593b5f901878a8bf889669";

const identity = (id, sha256 = `sha256:${"a".repeat(64)}`) => ({
  id: `https://forms.takoform.com/schemas/${id}.json`,
  public: `website/public/schemas/${id}.json`,
  sha256,
  source: `spec/schemas/${id}.json`,
});

describe("public schema identity history", () => {
  test("locks every stable v1 schema and no non-schema v1 document", () => {
    const identities = readPublicSchemaIdentityLedger(repositoryRoot);
    const stableSources = readdirSync(path.join(repositoryRoot, "spec", "schemas"))
      .filter((name) => name.endsWith("-v1.schema.json"))
      .map((name) => `spec/schemas/${name}`)
      .sort();
    const stableIdentities = identities.filter(({ id }) =>
      STABLE_V1_SCHEMA_IDS.includes(id),
    );

    expect(stableIdentities.map(({ id }) => id)).toEqual(STABLE_V1_SCHEMA_IDS);
    expect(
      stableSources.filter(
        (source) => !stableIdentities.some((identity) => identity.source === source),
      ),
    ).toEqual([]);
    expect(discoverPublicSchemas(repositoryRoot)).toHaveLength(identities.length);
  });

  test("does not change or reorder any occupied pre-v1 identity", () => {
    const stableIDs = new Set(STABLE_V1_SCHEMA_IDS);
    const occupied = readPublicSchemaIdentityLedger(repositoryRoot).filter(
      ({ id }) => !stableIDs.has(id),
    );
    const digest = createHash("sha256")
      .update(JSON.stringify(occupied))
      .digest("hex");

    expect(occupied).toHaveLength(OCCUPIED_IDENTITY_COUNT);
    expect(digest).toBe(OCCUPIED_IDENTITIES_SHA256);
  });

  test("rejects drift in a stable v1 identity", () => {
    const current = readPublicSchemaIdentityLedger(repositoryRoot);
    const drifted = current.map((entry) =>
      entry.id === STABLE_V1_SCHEMA_IDS[0]
        ? { ...entry, sha256: `sha256:${"0".repeat(64)}` }
        : entry,
    );

    expect(() =>
      enforceAppendOnlyPublicSchemaIdentities(drifted, [
        { identities: current, label: "stable v1 lock" },
      ]),
    ).toThrow(`identity ${STABLE_V1_SCHEMA_IDS[0]} was changed`);
  });

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

import { describe, expect, test } from "bun:test";

import {
  FROZEN_PROVIDER_RELEASES,
  assertFrozenProviderRelease,
  assertFrozenProviderReleaseDescriptor,
} from "./current-form-families.mjs";

describe("provider identity ledger continuity", () => {
  test("pins the retained v2.1.0 tag and canonical ledger bytes", () => {
    const frozen = FROZEN_PROVIDER_RELEASES.get("2.1.0");
    expect(frozen).toEqual({
      tag: "v2.1.0",
      ledgerDigest:
        "sha256:a3252479c294bd05bd64339ff300c38548fe26fe7b734ee71f7e0502dfde686e",
    });
  });

  test("rejects a retained entry changed together with its candidate catalog", () => {
    const entry = {
      providerVersion: "2.1.0",
      portableApiVersion: "forms.takoform.com/v1beta1",
      family: "edge.forms.takoform.com/v1beta1",
      formMaturity: "experimental",
      forms: [],
    };
    expect(() => assertFrozenProviderRelease(entry)).toThrow(
      /immutable provider 2\.1\.0 identity ledger entry changed/,
    );
  });

  test("rejects a retained release tag changed with its descriptor", () => {
    expect(() =>
      assertFrozenProviderReleaseDescriptor({ version: "2.1.0", tag: "v2.1.1" }),
    ).toThrow(/immutable provider 2\.1\.0 release tag changed/);
  });
});

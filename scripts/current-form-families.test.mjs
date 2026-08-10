import { describe, expect, test } from "bun:test";

import {
  FROZEN_PROVIDER_RELEASES,
  assertFrozenProviderRelease,
  assertFrozenProviderReleaseDescriptor,
} from "./current-form-families.mjs";

describe("provider identity ledger continuity", () => {
  test("pins the forward-repaired v2.1.1 tag and canonical ledger bytes", () => {
    const frozen = FROZEN_PROVIDER_RELEASES.get("2.1.1");
    expect(frozen).toEqual({
      tag: "v2.1.1",
      ledgerDigest:
        "sha256:981181257fac1ec43f85eb250fc12dd271236b1bbde94dc93323ee2180c4255d",
    });
  });

  test("rejects the forward-repaired entry changed together with its candidate catalog", () => {
    const entry = {
      providerVersion: "2.1.1",
      portableApiVersion: "forms.takoform.com/v1beta1",
      family: "edge.forms.takoform.com/v1beta1",
      formMaturity: "experimental",
      forms: [],
    };
    expect(() => assertFrozenProviderRelease(entry)).toThrow(
      /immutable provider 2\.1\.1 identity ledger entry changed/,
    );
  });

  test("rejects the forward-repaired release tag changed with its descriptor", () => {
    expect(() =>
      assertFrozenProviderReleaseDescriptor({ version: "2.1.1", tag: "v2.1.0" }),
    ).toThrow(/immutable provider 2\.1\.1 release tag changed/);
  });
});

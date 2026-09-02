import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import path from "node:path";

import {
  PROVIDER4_DESCRIPTOR,
  PROVIDER_RELEASE_DESCRIPTOR,
  validateProvider4Candidate,
} from "./provider4-candidate.mjs";

const root = path.resolve(import.meta.dirname, "..");

test("promoted Provider 4 release descriptor is publisher-specific and keeps Provider 3 as retained history", () => {
  const candidate = validateProvider4Candidate(root);
  expect(candidate.providerVersion).toBe("4.0.0");
  expect(candidate.portableApiVersion).toBe("forms.takoform.com/v1");
  expect(candidate.families).toEqual(["edge.forms.takoform.com"]);
  expect(candidate.forms).toHaveLength(17);
  expect(new Set(candidate.forms.map((entry) => entry.resourceType)).size).toBe(17);
  expect(candidate.forms.map((entry) => entry.resourceType)).toContain(
    "takoform_edge_object_bucket",
  );

  const retained = JSON.parse(
    readFileSync(path.join(root, "release/history/provider-v3.0.0.json"), "utf8"),
  );
  const current = JSON.parse(
    readFileSync(path.join(root, PROVIDER_RELEASE_DESCRIPTOR), "utf8"),
  );
  const descriptor = JSON.parse(
    readFileSync(path.join(root, PROVIDER4_DESCRIPTOR), "utf8"),
  );
  expect(retained.version).toBe("3.0.0");
  expect(retained.tag).toBe("v3.0.0");
  expect(current.version).toBe("4.0.0");
  expect(current.tag).toBe("v4.0.0");
  // The ledger readback, not this field, is the availability authority: both
  // release workflows hard-require candidate-only on the writer input.
  expect(current.publicationStatus).toBe("candidate-only");
  expect(descriptor.publicationStatus).toBe("candidate-only");
  expect(descriptor.formPublisherRepository).toBe(
    "github.com/tako0614/takoform-forms",
  );
  expect(descriptor.formPublisherCommit).toBe(
    "3231633605b737ce5279d7fc020b4780568e7091",
  );
  expect(descriptor.formSetTag).toBe(
    "forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c",
  );
});

test("current Provider surfaces identify the publisher without a privileged classification", () => {
  for (const relativePath of [
    "README.md",
    "docs/index.md",
    "website/index.md",
    "website/docs/index.md",
    PROVIDER4_DESCRIPTOR,
    PROVIDER_RELEASE_DESCRIPTOR,
  ]) {
    const contents = readFileSync(path.join(root, relativePath), "utf8");
    expect(contents).not.toMatch(/\bofficial(?:-only)?\b/iu);
  }
});

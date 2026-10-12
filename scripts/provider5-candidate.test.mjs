import { expect, test } from "bun:test";
import { copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { readCurrentProviderRelease, validateProvider5ReleaseDescriptor } from "./provider-release-descriptor.mjs";
import { validateProvider5Candidate } from "./provider5-candidate.mjs";
import { readProviderDescriptor, providerReleaseBody } from "./release-deploy.mjs";

const root = resolve(import.meta.dirname, "..");

test("v5 release selects one stable exact-URL candidate without changing v4 history", () => {
  const current = readCurrentProviderRelease(root);
  expect(current.descriptor.version).toBe("5.0.0");
  expect(current.candidateDescriptorPath).toBe("release/candidates/provider-v5.0.0.json");
  expect(current.candidateFormIdentitiesPath).toBe("release/candidates/provider-v5.0.0-form-identities.json");
  expect(validateProvider5Candidate(root).forms).toHaveLength(13);
  expect(readProviderDescriptor(root).versioning.portableApiVersion).toBe("forms.takoform.com/v2");
  expect(providerReleaseBody(current.descriptor)).toContain("Provider v4.1.0 state");
  const published = JSON.parse(readFileSync(resolve(root, "release/provider-release-identities.json"), "utf8"));
  expect(published.entries.find((entry) => entry.version === "5.0.0")).toBeUndefined();
  expect(published.entries.find((entry) => entry.version === "4.1.0")?.registryReadback).toBeDefined();
});

test("v5 descriptor refuses v1 package identity and prerelease ambiguity", () => {
  const descriptor = JSON.parse(readFileSync(resolve(root, "release/version.json"), "utf8"));
  expect(() => validateProvider5ReleaseDescriptor({ ...descriptor, version: "5.0.0-rc.1", tag: "v5.0.0-rc.1" })).toThrow();
  expect(() => validateProvider5ReleaseDescriptor({ ...descriptor, formSetTag: "forms/sets/old" })).toThrow();
  expect(() => validateProvider5ReleaseDescriptor({ ...descriptor, versioning: { ...descriptor.versioning, portableApiVersion: "forms.takoform.com/v1" } })).toThrow();
});

test("v5 candidate refuses a changed URL even if the registered source is changed with it", () => {
  const fixture = mkdtempSync(join(tmpdir(), "provider5-url-"));
  try {
    for (const relative of [
      "release/version.json",
      "release/candidates/provider-v5.0.0.json",
      "release/candidates/provider-v5.0.0-form-identities.json",
      "internal/edgev2/spec.go",
      "internal/provider/v2_schema.go",
    ]) {
      mkdirSync(dirname(join(fixture, relative)), { recursive: true });
      copyFileSync(join(root, relative), join(fixture, relative));
    }
    const identityPath = join(fixture, "release/candidates/provider-v5.0.0-form-identities.json");
    const candidate = JSON.parse(readFileSync(identityPath, "utf8"));
    candidate.forms[0].formURL = candidate.forms[0].formURL.replace("0.2.0", "0.2.1");
    writeFileSync(identityPath, `${JSON.stringify(candidate, null, 2)}\n`);
    expect(() => validateProvider5Candidate(fixture)).toThrow(/stale or mismatches/u);

    const sourcePath = join(fixture, "internal/edgev2/spec.go");
    writeFileSync(sourcePath, readFileSync(sourcePath, "utf8").replace(
      "forms/AtLeastOnceQueue/0.2.0/", "forms/AtLeastOnceQueue/0.2.1/",
    ));
    expect(() => validateProvider5Candidate(fixture)).toThrow(/pinned publisher object/u);
  } finally {
    rmSync(fixture, { recursive: true, force: true });
  }
});

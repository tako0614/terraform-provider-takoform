import { afterEach, describe, expect, test } from "bun:test";
import {
  existsSync,
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { installGeneratedOutputs } from "./current-form-candidates.mjs";

const temporaryRoots = [];

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) {
    rmSync(root, { recursive: true, force: true });
  }
});

function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-generated-output-test-"));
  temporaryRoots.push(root);
  const stagingParent = path.join(root, "staging");
  const stagedRoot = path.join(stagingParent, "v1alpha2");
  const targetRoot = path.join(root, "forms", "v1alpha2");
  const stagedRegistryPath = path.join(stagingParent, "registry_generated.go");
  const targetRegistryPath = path.join(root, "internal", "registry_generated.go");
  mkdirSync(stagedRoot, { recursive: true });
  mkdirSync(targetRoot, { recursive: true });
  mkdirSync(path.dirname(targetRegistryPath), { recursive: true });
  writeFileSync(path.join(stagedRoot, "candidate-set.json"), "new tree\n");
  writeFileSync(path.join(targetRoot, "candidate-set.json"), "old tree\n");
  writeFileSync(targetRegistryPath, "old registry\n");
  return {
    stagedRoot,
    targetRoot,
    stagedRegistryPath,
    targetRegistryPath,
    stagingParent,
  };
}

describe("current Form generated output installation", () => {
  test("installs the candidate tree and provider registry together", () => {
    const paths = fixture();
    writeFileSync(paths.stagedRegistryPath, "new registry\n");

    installGeneratedOutputs(paths);

    expect(readFileSync(path.join(paths.targetRoot, "candidate-set.json"), "utf8")).toBe("new tree\n");
    expect(readFileSync(paths.targetRegistryPath, "utf8")).toBe("new registry\n");
  });

  test("restores both previous outputs when the registry install fails", () => {
    const paths = fixture();

    expect(() => installGeneratedOutputs(paths)).toThrow();

    expect(readFileSync(path.join(paths.targetRoot, "candidate-set.json"), "utf8")).toBe("old tree\n");
    expect(readFileSync(paths.targetRegistryPath, "utf8")).toBe("old registry\n");
    expect(existsSync(paths.stagedRoot)).toBe(false);
  });
});

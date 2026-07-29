import { afterEach, describe, expect, test } from "bun:test";
import {
  mkdtempSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import {
  collectRegularFiles,
  parseCurrentProductionDeployment,
} from "./deploy-safety.mjs";

const temporaryDirectories = [];

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { force: true, recursive: true });
  }
});

describe("production deployment readback", () => {
  const current = {
    id: "4ed02082-0ac8-469f-aca7-cd3862e6b348",
    strategy: "percentage",
    versions: [
      {
        percentage: 100,
        version_id: "37dd55a9-d75b-452f-ab62-77486fb7204e",
      },
    ],
  };

  test("selects the sole version receiving 100% production traffic", () => {
    expect(parseCurrentProductionDeployment(JSON.stringify(current))).toEqual({
      deploymentId: current.id,
      versionId: current.versions[0].version_id,
    });
  });

  test.each([
    {
      ...current,
      versions: [
        {
          percentage: 50,
          version_id: "37dd55a9-d75b-452f-ab62-77486fb7204e",
        },
        {
          percentage: 50,
          version_id: "067ae8ca-1300-4de6-94ee-c56f3ca65000",
        },
      ],
    },
    {
      ...current,
      versions: [{ ...current.versions[0], percentage: 99 }],
    },
  ])("rejects an ambiguous or split deployment", (deployment) => {
    expect(() =>
      parseCurrentProductionDeployment(JSON.stringify(deployment)),
    ).toThrow("single 100% version is required");
  });
});

test("published asset discovery rejects symbolic links", () => {
  const directory = mkdtempSync(join(tmpdir(), "takoform-deploy-safety-"));
  temporaryDirectories.push(directory);
  const outside = join(directory, "outside.txt");
  writeFileSync(outside, "outside\n");
  symlinkSync(outside, join(directory, "linked.txt"));

  expect(() => collectRegularFiles(directory)).toThrow(
    "published asset must not be a symbolic link",
  );
});

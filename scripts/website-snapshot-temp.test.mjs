import { afterEach, expect, test } from "bun:test";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { createWebsiteSnapshotWorkspace } from "./website-snapshot-temp.mjs";

const roots = [];

afterEach(() => {
  for (const root of roots.splice(0)) {
    rmSync(root, { force: true, recursive: true });
  }
});

test("concurrent website snapshots own isolated cleanup roots", () => {
  const repositoryRoot = mkdtempSync(path.join(tmpdir(), "takoform-website-snapshot-test-"));
  roots.push(repositoryRoot);

  const first = createWebsiteSnapshotWorkspace(repositoryRoot);
  const second = createWebsiteSnapshotWorkspace(repositoryRoot);
  expect(first.root).not.toBe(second.root);

  const firstSentinel = path.join(first.root, "first");
  const secondSentinel = path.join(second.root, "second");
  writeFileSync(firstSentinel, "first");
  writeFileSync(secondSentinel, "second");

  first.cleanup();
  expect(existsSync(first.root)).toBe(false);
  expect(existsSync(secondSentinel)).toBe(true);

  // Cleanup is idempotent and remains scoped after another workspace exists.
  first.cleanup();
  expect(existsSync(secondSentinel)).toBe(true);

  second.cleanup();
  expect(existsSync(second.root)).toBe(false);
});

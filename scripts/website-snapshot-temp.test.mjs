import { afterEach, expect, test } from "bun:test";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  assertWebsiteMetadataUnchanged,
  compareGeneratedAssets,
  generatedHtmlMatches,
  snapshotWebsiteMetadata,
} from "./check-website-dist.mjs";
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

test("source metadata snapshot detects a same-size byte mutation", () => {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-website-source-test-"));
  roots.push(root);
  const file = path.join(root, "source.md");
  writeFileSync(file, "first");
  const before = snapshotWebsiteMetadata(root);

  writeFileSync(file, "other");
  const after = snapshotWebsiteMetadata(root);

  expect(() => assertWebsiteMetadataUnchanged(before, after)).toThrow(
    /source entries: source\.md/,
  );
});

test("normalized HTML comparison rejects executable and structural injection", () => {
  const fresh =
    '<!doctype html>\n<html><head><title>safe</title></head> \n<body><main id="app">safe</main></body></html> \n';
  const committed =
    '<!doctype html>\n<html><head><title>safe</title></head>\n<body><main id="app">safe</main></body></html>\n';
  expect(generatedHtmlMatches(committed, fresh)).toBe(true);

  for (const injected of [
    committed.replace(
      "</head>",
      '<script src="https://attacker.invalid/payload.js"></script></head>',
    ),
    committed.replace('id="app"', 'id="app" onclick="steal()"'),
    committed.replace("<main", "<aside><main").replace("</main>", "</main></aside>"),
  ]) {
    expect(generatedHtmlMatches(injected, fresh)).toBe(false);
  }
});

function makeAssetRoots() {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-website-assets-test-"));
  roots.push(root);
  const freshRoot = path.join(root, "fresh");
  const committedRoot = path.join(root, "committed");
  mkdirSync(path.join(freshRoot, "assets"), { recursive: true });
  mkdirSync(path.join(committedRoot, "assets"), { recursive: true });
  return { freshRoot, committedRoot };
}

function writeAsset(root, relativePath, contents) {
  writeFileSync(path.join(root, relativePath), contents);
}

test("rejects role-preserving renamed JavaScript and CSS assets", () => {
  const { freshRoot, committedRoot } = makeAssetRoots();
  const freshFiles = ["assets/app.AAAAAAAA.js", "assets/style.AAAAAAAA.css"];
  const committedFiles = ["assets/app.BBBBBBBB.js", "assets/style.BBBBBBBB.css"];
  writeAsset(freshRoot, freshFiles[0], "fresh app");
  writeAsset(freshRoot, freshFiles[1], "fresh style");
  writeAsset(committedRoot, committedFiles[0], "malicious app");
  writeAsset(committedRoot, committedFiles[1], "malicious style");

  expect(() =>
    compareGeneratedAssets({
      freshFiles,
      committedFiles,
      freshRoot,
      committedRoot,
    }),
  ).toThrow(/assets path set/);
});

test("rejects modified bytes at the same generated asset path", () => {
  const { freshRoot, committedRoot } = makeAssetRoots();
  const files = ["assets/app.AAAAAAAA.js"];
  writeAsset(freshRoot, files[0], "fresh app");
  writeAsset(committedRoot, files[0], "modified app");

  expect(() =>
    compareGeneratedAssets({
      freshFiles: files,
      committedFiles: files,
      freshRoot,
      committedRoot,
    }),
  ).toThrow(/changes the bytes/);
});

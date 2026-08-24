import { afterEach, describe, expect, test } from "bun:test";
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  retainedRoots,
  verifyRetainedPortableHostCorpora,
} from "./retained-portable-host-corpora-check.mjs";

const temporaryRoots = [];

afterEach(() => {
  for (const root of temporaryRoots.splice(0)) rmSync(root, { recursive: true, force: true });
});

function retainedFixture() {
  const root = mkdtempSync(path.join(tmpdir(), "takoform-retained-beta-"));
  temporaryRoots.push(root);
  for (const relativePath of retainedRoots) {
    const destination = path.join(root, relativePath);
    mkdirSync(path.dirname(destination), { recursive: true });
    cpSync(relativePath, destination, { recursive: true });
  }
  expect(verifyRetainedPortableHostCorpora(root).fileCount).toBe(56);
  return root;
}

function appendTamper(root, relativePath) {
  const absolutePath = path.join(root, relativePath);
  writeFileSync(absolutePath, Buffer.concat([readFileSync(absolutePath), Buffer.from("\n")]));
}

describe("retained portable Host corpus immutability", () => {
  test("rejects one-byte v1beta1 corpus drift", () => {
    const root = retainedFixture();
    appendTamper(root, "conformance/portable-host-v1beta1/contract.json");
    expect(() => verifyRetainedPortableHostCorpora(root)).toThrow(
      /retained portable Host beta bytes drifted/,
    );
  });

  test("rejects one-byte v1beta4 corpus drift", () => {
    const root = retainedFixture();
    appendTamper(root, "conformance/portable-host-v1beta4/contract.json");
    expect(() => verifyRetainedPortableHostCorpora(root)).toThrow(
      /retained portable Host beta bytes drifted/,
    );
  });
});

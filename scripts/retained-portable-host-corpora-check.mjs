#!/usr/bin/env bun

import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

export const expectedFileCount = 56;
export const expectedAggregate =
  "6f34b948e38ad82384409d0a6350688c4bc7a115041ea2837b30ad3064f08d10";

export const retainedRoots = [
  "conformance/portable-host-v1beta1",
  "conformance/portable-host-v1beta4",
  "scripts/portable-host-v1beta1-pins.mjs",
  "scripts/portable-host-v1beta4-derive.mjs",
  "spec/host-api/operations-v1beta1.json",
  "spec/host-api/operations-v1beta4.json",
  "spec/host-api/v1beta1.md",
  "spec/host-api/v1beta4.md",
  "spec/schemas/form-definition-v1beta1.schema.json",
  "spec/schemas/form-ref-v1beta1.schema.json",
  "spec/schemas/host-api-wire-v1beta1.schema.json",
  "spec/schemas/host-api-wire-v1beta4.schema.json",
  "spec/schemas/host-discovery-v1beta1.schema.json",
  "spec/schemas/host-discovery-v1beta4.schema.json",
];

const sha256 = (bytes) => createHash("sha256").update(bytes).digest("hex");

export function retainedPortableHostInventory(repositoryRoot = ".") {
  const files = [];
  const collect = (relativePath) => {
    const absolutePath = path.join(repositoryRoot, relativePath);
    const metadata = statSync(absolutePath);
    if (!metadata.isDirectory()) {
      files.push(relativePath);
      return;
    }
    for (const entry of readdirSync(absolutePath).sort()) {
      collect(path.posix.join(relativePath, entry));
    }
  };
  for (const retainedRoot of retainedRoots) collect(retainedRoot);
  files.sort();
  return files.map((relativePath) => ({
    path: relativePath,
    sha256: sha256(readFileSync(path.join(repositoryRoot, relativePath))),
  }));
}

export function verifyRetainedPortableHostCorpora(repositoryRoot = ".") {
  const inventory = retainedPortableHostInventory(repositoryRoot);
  const aggregate = sha256(Buffer.from(JSON.stringify(inventory), "utf8"));
  if (inventory.length !== expectedFileCount || aggregate !== expectedAggregate) {
    throw new Error(
      `retained portable Host beta bytes drifted: ${inventory.length} files, aggregate ${aggregate}; ` +
        `want ${expectedFileCount} files, aggregate ${expectedAggregate}`,
    );
  }
  return { fileCount: inventory.length, aggregate };
}

if (import.meta.main) {
  try {
    const result = verifyRetainedPortableHostCorpora();
    console.log(
      `retained portable Host beta bytes are immutable: ${result.fileCount} files, aggregate ${result.aggregate}`,
    );
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}

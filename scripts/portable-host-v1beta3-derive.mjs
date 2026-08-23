#!/usr/bin/env bun

// Derives conformance/portable-host-v1beta3 from the v1beta2 corpus.
//
// The three corpora measure ONE set of lifecycle semantics through three
// protocol lanes. Hand-authoring the third would let them drift apart
// silently, and conformance corpora that disagree cannot tell a host which of
// them it failed — so this one is derived, and the only hand-written part is
// the delta below.
//
// The delta is small on the wire and large in what it means. The envelope is
// v1beta2's, unchanged. What changed is that the cross-resource rules are
// DECLARED by a Definition rather than written per Form in the protocol
// document, which is why the lane's document names no Form kind and why the
// one added check drives the declaration rather than a Form.

import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/portable-host-v1beta3-derive.mjs --write|--check");
}

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const sourceRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta2");
const targetRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta3");

const source = JSON.parse(readFileSync(path.join(sourceRoot, "contract.json"), "utf8"));

// ---- the lane delta ----

const contract = structuredClone(source);
contract.format = "takoform.portable-host-conformance@v1beta3";
contract.apiVersion = "forms.takoform.com/v1beta3";
contract.discoveryPath = "/.well-known/takoform/v1beta3";
contract.apiPath = "/apis/forms.takoform.com/v1beta3";

// One added check. It drives every exclusive hold the installed family
// declares, discovered from the served Definitions — a host that hardcoded one
// rule and forgot another fails on the one it forgot, which is the failure the
// previous lane's per-Form checks could not see.
// Family-derived checks are appended AFTER the lane's own, so the new lane
// check goes before them rather than at the end of the list — the verifier
// compares the list in order, and a corpus that appended blindly would drift
// from the lane the moment a family added a check.
const FAMILY_DERIVED = new Set(["class-holder-rules-enforced"]);
const inheritedLaneChecks = source.requiredRunnerChecks.filter(
  (check) => !FAMILY_DERIVED.has(check),
);
const inheritedFamilyChecks = source.requiredRunnerChecks.filter((check) =>
  FAMILY_DERIVED.has(check),
);
contract.requiredRunnerChecks = [
  ...inheritedLaneChecks,
  "declared-exclusive-holds-enforced",
  ...inheritedFamilyChecks,
];

// ---- write or check ----

const contractText = `${JSON.stringify(contract, null, 2)}\n`;
const manifest = {
  format: "takoform.portable-host-conformance-manifest@v1beta3",
  contract: "contract.json",
  sha256: createHash("sha256").update(contractText).digest("hex"),
};
const manifestText = `${JSON.stringify(manifest, null, 2)}\n`;

const fixtureDir = path.join(sourceRoot, "fixtures");
const fixtures = readdirSync(fixtureDir).map((name) => ({
  name,
  bytes: readFileSync(path.join(fixtureDir, name)),
}));

const outputs = [
  { file: path.join(targetRoot, "contract.json"), bytes: Buffer.from(contractText) },
  { file: path.join(targetRoot, "manifest.json"), bytes: Buffer.from(manifestText) },
  ...fixtures.map(({ name, bytes }) => ({
    file: path.join(targetRoot, "fixtures", name),
    bytes,
  })),
];

if (mode === "--write") {
  mkdirSync(path.join(targetRoot, "fixtures"), { recursive: true });
  for (const { file, bytes } of outputs) writeFileSync(file, bytes);
  console.log(
    `derived portable-host-v1beta3 from v1beta2: ${contract.requiredRunnerChecks.length} required checks, ` +
      `${contract.errorEnvelope.codes.length} error codes, ${fixtures.length} fixtures`,
  );
} else {
  const drift = [];
  for (const { file, bytes } of outputs) {
    let actual;
    try {
      actual = readFileSync(file);
    } catch {
      drift.push(`${path.relative(repositoryRoot, file)}: missing`);
      continue;
    }
    if (!actual.equals(bytes)) drift.push(`${path.relative(repositoryRoot, file)}: drifted`);
  }
  if (drift.length > 0) {
    for (const line of drift) process.stderr.write(`- ${line}\n`);
    throw new Error(
      "portable-host-v1beta3 is stale; run bun scripts/portable-host-v1beta3-derive.mjs --write",
    );
  }
  console.log(
    `portable-host-v1beta3 matches its derivation from v1beta2 (${contract.requiredRunnerChecks.length} checks)`,
  );
}

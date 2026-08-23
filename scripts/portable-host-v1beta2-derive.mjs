#!/usr/bin/env bun

// Derives conformance/portable-host-v1beta2 from the v1beta1 corpus.
//
// The two corpora measure ONE set of lifecycle semantics through two protocol
// lanes. Hand-authoring the second would let them drift apart silently, and a
// pair of conformance corpora that disagree cannot tell a host which of them it
// failed — so the v1beta2 corpus is derived, and the only hand-authored part is
// the delta below: what the lane actually changed.
//
// The v1beta1 corpus is never written by this script. It is the retained lane,
// Registry-published provider 2.1.1 speaks it, and hosts serve it today.

import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/portable-host-v1beta2-derive.mjs --write|--check");
}

const repositoryRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const sourceRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta1");
const targetRoot = path.join(repositoryRoot, "conformance", "portable-host-v1beta2");

const source = JSON.parse(readFileSync(path.join(sourceRoot, "contract.json"), "utf8"));

// ---- the lane delta ----
//
// Every entry here is a rule the v1beta2 document states and the v1beta1
// document did not. Nothing else about the corpus moves, which is what makes
// the two comparable: a check that fails on one lane and passes on the other
// is reporting a lane difference rather than a corpus difference.

const contract = structuredClone(source);
contract.format = "takoform.portable-host-conformance@v1beta2";
contract.apiVersion = "forms.takoform.com/v1beta2";
contract.discoveryPath = "/.well-known/takoform/v1beta2";
contract.apiPath = "/apis/forms.takoform.com/v1beta2";

// Two codes leave and one status moves. form_identity_conflict was never given
// a trigger; deletion_protected claimed a policy surface no Form provides; and
// form_unavailable is transient backing capacity, which is 503.
const withdrawnCodes = new Set(["form_identity_conflict", "deletion_protected"]);
contract.errorEnvelope.codes = contract.errorEnvelope.codes.filter(
  (code) => !withdrawnCodes.has(code),
);
contract.errorEnvelope.httpStatusByCode = Object.fromEntries(
  Object.entries(contract.errorEnvelope.httpStatusByCode)
    .filter(([code]) => !withdrawnCodes.has(code))
    .map(([code, status]) => [code, code === "form_unavailable" ? 503 : status]),
);

// The sealed external standard-service slot (decision 0045). Its two specs are
// the whole point of the probe: one the host must accept and one it must refuse
// before any mutation, so the vocabulary is closed rather than advisory.
const workerVersionDesired = contract.runnerInput.workerVersion.desired;
contract.runnerInput.externalServices = {
  property: "externalServices",
  serviceApiVersion: "standards.takoform.com/v1alpha1",
  protocols: ["postgresql", "redis", "s3-compatible", "smtp"],
  desiredSpec: {
    ...structuredClone(workerVersionDesired),
    externalServices: [
      {
        name: "PRIMARY_DB",
        service: { apiVersion: "standards.takoform.com/v1alpha1", protocol: "postgresql" },
      },
      {
        name: "MEDIA_STORE",
        service: { apiVersion: "standards.takoform.com/v1alpha1", protocol: "s3-compatible" },
      },
    ],
  },
  unknownProtocolSpec: {
    ...structuredClone(workerVersionDesired),
    externalServices: [
      {
        name: "PRIMARY_DB",
        service: {
          apiVersion: "standards.takoform.com/v1alpha1",
          protocol: "not-a-standard-protocol",
        },
      },
    ],
  },
};

// One check per rule the lane introduced. The list is the corpus's copy of what
// internal/portableconformancev3 requires of a v1beta2 run; the two are compared
// at verify time, so neither can grow a check the other does not know about.
contract.requiredRunnerChecks = [
  ...source.requiredRunnerChecks,
  "fence-matrix-observed",
  "forms-route-enumerates",
  "availability-truth-conditions",
  "cancel-outcomes-closed",
  "external-service-slots-sealed",
];

// ---- write or check ----

const contractText = `${JSON.stringify(contract, null, 2)}\n`;
const manifest = {
  format: "takoform.portable-host-conformance-manifest@v1beta2",
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
    `derived portable-host-v1beta2 from v1beta1: ${contract.requiredRunnerChecks.length} required checks, ` +
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
      "portable-host-v1beta2 is stale; run bun scripts/portable-host-v1beta2-derive.mjs --write",
    );
  }
  console.log(
    `portable-host-v1beta2 matches its derivation from v1beta1 (${contract.requiredRunnerChecks.length} checks)`,
  );
}

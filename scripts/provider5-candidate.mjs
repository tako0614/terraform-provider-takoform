#!/usr/bin/env bun

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { readCurrentProviderRelease } from "./provider-release-descriptor.mjs";

const CANDIDATE_FORMAT = "takoform.provider-candidate-form-urls@v1";
const URL_PREFIX = "https://edge.forms.takoform.com/forms/";
// SHA-256 of JSON.stringify(sorted thirteen URL strings) selected from
// takoform-forms cfd376b724803dd2e5d112669b84589c0de517e0
// spec/forms.freeze.json. This is independent of the Provider source parser.
const PUBLISHED_URL_ROSTER_SHA256 = "bccbbc829927857e108f4827814311bdbefef27b9780ad8fb2ca7d22c5cc39ca";

function fail(message) { throw new Error(`Provider 5 candidate: ${message}`); }
function read(root, path) { return readFileSync(join(root, path), "utf8"); }

// Compare the release projection with the actual registered HCL resource list
// and the exact URL constants in source. Neither list is re-authored here.
export function deriveProvider5Candidate(root) {
  const { descriptor } = readCurrentProviderRelease(root);
  if (descriptor.schemaVersion !== 2) fail("current descriptor is not Provider 5");
  const source = read(root, "internal/edgev2/spec.go");
  const schema = read(root, "internal/provider/v2_schema.go");
  const urls = new Map([...source.matchAll(/^\s*([A-Za-z][A-Za-z0-9]+URL)\s*=\s*"(https:\/\/edge\.forms\.takoform\.com\/forms\/[A-Za-z][A-Za-z0-9]*\/[0-9]+\.[0-9]+\.[0-9]+\/)"/gmu)]
    .map((match) => [match[1], match[2]]));
  const forms = [...schema.matchAll(/^\s*\{"(takoform_[a-z0-9_]+)", edgev2\.([A-Za-z][A-Za-z0-9]+URL),/gmu)]
    .map((match) => ({ resourceType: match[1], formURL: urls.get(match[2]) }));
  if (urls.size !== 13 || forms.length !== 13 ||
      new Set(forms.map((form) => form.resourceType)).size !== 13 ||
      new Set(forms.map((form) => form.formURL)).size !== 13 ||
      forms.some((form) => !form.formURL?.startsWith(URL_PREFIX)) ||
      forms.some((form) => !/^https:\/\/edge\.forms\.takoform\.com\/forms\/([A-Za-z][A-Za-z0-9]*)\/[0-9]+\.[0-9]+\.[0-9]+\/$/u.test(form.formURL))) {
    fail("registered source does not map exactly thirteen unique published Edge URLs");
  }
  const sourceURLs = [...urls.values()].sort();
  if (createHash("sha256").update(JSON.stringify(sourceURLs)).digest("hex") !== PUBLISHED_URL_ROSTER_SHA256) {
    fail("source URL roster differs from the pinned publisher object");
  }
  if (JSON.stringify(forms.map((form) => form.formURL).sort()) !== JSON.stringify(sourceURLs)) {
    fail("registered v2 resources do not cover the exact source URL roster");
  }
  return {
    format: CANDIDATE_FORMAT,
    providerVersion: descriptor.version,
    portableApiVersion: descriptor.versioning.portableApiVersion,
    formPublisherRepository: descriptor.formPublisherRepository,
    formPublisherCommit: descriptor.formPublisherCommit,
    forms: forms.sort((a, b) => a.resourceType.localeCompare(b.resourceType)),
  };
}

export function validateProvider5Candidate(root) {
  const current = readCurrentProviderRelease(root);
  const expected = deriveProvider5Candidate(root);
  let actual;
  try { actual = JSON.parse(read(root, current.candidateFormIdentitiesPath)); }
  catch (error) { fail(`${current.candidateFormIdentitiesPath} is not readable JSON (${error.message})`); }
  if (JSON.stringify(actual) !== JSON.stringify(expected)) {
    fail(`${current.candidateFormIdentitiesPath} is stale or mismatches the registered v2 source`);
  }
  return actual;
}

if (import.meta.main) {
  const root = resolve(import.meta.dirname, "..");
  if (process.argv[2] !== "--check") fail("usage: bun scripts/provider5-candidate.mjs --check");
  const candidate = validateProvider5Candidate(root);
  process.stdout.write(`Provider 5 candidate: ${candidate.forms.length} exact published URLs; no publication claimed\n`);
}

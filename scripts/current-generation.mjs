#!/usr/bin/env bun

// Which identity is current cannot be inferred from a version word. The
// public API/Core release number is a human-readable compatibility checkpoint;
// every compatible 1.x checkpoint remains on the /v1 wire/discovery lane. Form
// definitionVersion is the other domain axis. Provider, package, schema,
// Specification, and retained-lane version words identify artifacts or history.
//
// So stop asking anyone to infer it. The repository already derives these facts
// for /.well-known/takoform-site.json; this is a third caller of the same
// derivation, writing them where someone opening the repository will look
// first. Nothing here is hand-maintained, which is the point: a hand-written
// answer to "which one is current" is the thing that went stale when the lane
// moved.

import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { deriveSiteStatusFacts } from "../website/.vitepress/site-status.mjs";

const mode = process.argv[2];
if (process.argv.length !== 3 || !["--write", "--check"].includes(mode)) {
  throw new Error("usage: bun scripts/current-generation.mjs --write|--check");
}

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const readmePath = path.join(repositoryRoot, "README.md");
const BEGIN = "<!-- current-generation:begin -->";
const END = "<!-- current-generation:end -->";

// This is a provider-facing display projection of the public Core release.
// Core owns the release identity; this repository only links to that external
// authority and never derives Provider or Host publication from it.
const CORE_API_RELEASE_VERSION = "1.0.1";
const CORE_API_RELEASE_URL =
  "https://github.com/tako0614/takoform/releases/tag/v1.0.1";
const CORE_API_LANE = "forms.takoform.com/v1";
const ACTIVE_FORMS_PUBLISHER_REPOSITORY =
  "https://github.com/tako0614/takoform-forms";
const ACTIVE_FORMS_PUBLISHER_FAMILY = "edge.forms.takoform.com";
const ACTIVE_FORMS_PUBLISHER_FORM_COUNT = 16;

function activePublisherFormCount() {
  const candidatePath = path.join(
    repositoryRoot,
    "forms/candidates/edge.forms.takoform.com/candidate-set.json",
  );
  const candidate = JSON.parse(readFileSync(candidatePath, "utf8"));
  if (
    candidate.family !== ACTIVE_FORMS_PUBLISHER_FAMILY ||
    candidate.publicationStatus !== "unpublished" ||
    candidate.forms?.length !== ACTIVE_FORMS_PUBLISHER_FORM_COUNT
  ) {
    throw new Error(
      "active Edge publisher candidate set no longer matches the public display projection",
    );
  }
  return candidate.forms.length;
}

function render(facts, activeEdgeFormCount) {
  const rows = [
    [
      "API/Core release SemVer",
      CORE_API_RELEASE_VERSION,
      `public Core/API checkpoint on [the Core v${CORE_API_RELEASE_VERSION} release](${CORE_API_RELEASE_URL}), using the ${CORE_API_LANE} wire/discovery lane; compatible 1.y.0 checkpoints remain on /v1`,
    ],
    [
      "Form definitionVersion (active publisher)",
      `1 family / ${activeEdgeFormCount} candidate Forms`,
      `active Edge source set ${ACTIVE_FORMS_PUBLISHER_FAMILY}, independently owned by [takoform-forms](${ACTIVE_FORMS_PUBLISHER_REPOSITORY}); package artifacts remain unpublished`,
    ],
    [
      "Host API wire/discovery lane",
      facts.hostApiCurrent,
      "protocol path used by API/Core 1.x checkpoints; this path is not a third domain axis, and Host implementation/support/deployment remains host-owned",
    ],
    [
      "Provider compatibility mapping",
      `${facts.providerPublished} (retained)`,
      `${facts.currentFamilyCount} families / ${facts.currentFormCount} typed mappings retained by Provider 3; this Provider history is not the active publisher roster`,
    ],
    [
      "Historical Specification receipt",
      facts.specificationVersion,
      "sealed exact source receipt; not API release 1.1 or 1.1.0, no /v1.1, and no ongoing Specification stream",
    ],
    [
      "Form Package envelope",
      facts.formPackageApiCurrent,
      `package artifacts are ${facts.formPackagePublicationStatus}`,
    ],
    [
      "Provider distribution",
      facts.providerPublished,
      "Registry-published Provider 3 retaining the typed compatibility mapping; not the active publisher roster or Specification authority",
    ],
  ];
  return [
    BEGIN,
    "",
    "| Identity | Current identity | Meaning |",
    "| --- | --- | --- |",
    ...rows.map(([identity, value, note]) => `| ${identity} | \`${value}\` | ${note} |`),
    "",
    "Only API/Core release SemVer and per-Form definitionVersion are domain",
    "version axes. The active publisher is the standalone Edge candidate/source",
    `${ACTIVE_FORMS_PUBLISHER_FAMILY} (${activeEdgeFormCount} Forms); its package artifacts are unpublished.`,
    "Provider 3's 31 typed mappings are retained compatibility history, not a",
    "current publisher roster. The historical Specification 1.1 receipt is sealed",
    "and separate: it is not API release 1.1 or 1.1.0, does not create `/v1.1`,",
    "and is not an ongoing Specification stream. Form/package publication and",
    "Provider release remain independent artifact identities.",
    "This table is generated from repository bytes",
    "by `bun run sync:current-generation`; the numbered release ledger derives",
    "historical Specification receipt state without changing any API/Core, Form,",
    "package, or Provider identity.",
    "",
    END,
  ].join("\n");
}

const facts = deriveSiteStatusFacts(repositoryRoot);
const activeEdgeFormCount = activePublisherFormCount();
const readme = readFileSync(readmePath, "utf8");
const begin = readme.indexOf(BEGIN);
const end = readme.indexOf(END);
if (begin === -1 || end === -1 || end < begin) {
  throw new Error(
    `README.md must carry the ${BEGIN} … ${END} block this command generates`,
  );
}
const expected =
  readme.slice(0, begin) +
  render(facts, activeEdgeFormCount) +
  readme.slice(end + END.length);

if (mode === "--write") {
  writeFileSync(readmePath, expected);
  process.stdout.write(
    `README current generation: ${facts.hostApiCurrent}, ${facts.currentFamilyCount} families, provider ${facts.providerPublished}\n`,
  );
  process.exit(0);
}

if (readme !== expected) {
  process.stderr.write(
    "README.md states a current generation the repository does not derive; " +
      "run bun run sync:current-generation\n",
  );
  process.exit(1);
}
process.stdout.write(
  `README current generation matches the repository: ${facts.hostApiCurrent}, ` +
    `${facts.currentFamilyCount} families, provider ${facts.providerPublished}\n`,
);

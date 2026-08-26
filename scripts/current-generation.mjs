#!/usr/bin/env bun

// Which identity is the current one cannot be worked out from the numbers.
//
// The axes are independent by design, so they disagree, and they disagree in
// opposite directions: the Host API lane went v1alpha3 -> v1beta1, where the
// digit went DOWN and only alpha < beta says which is newer, while the package
// envelope went v1alpha3 -> v1alpha4, where the digit went up. A reader who
// tries to infer "latest" from a version word is using a rule that holds on one
// axis and fails on the next.
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

function render(facts) {
  const rows = [
    [
      "Specification",
      facts.specificationVersion,
      `${facts.specificationReleaseStatus}; one exact committed normative source snapshot is release authority`,
    ],
    [
      "Host API candidate",
      facts.hostApiCurrent,
      `${facts.hostApiPublicationStatus}; separate protocol identity`,
    ],
    [
      "Current Form corpus",
      facts.currentFamilyIndex,
      `${facts.currentFamilyCount} versionless families, ${facts.currentFormCount} exact \`0.x\` ${facts.formMaturity} Forms`,
    ],
    [
      "Form Package envelope",
      facts.formPackageApiCurrent,
      `package artifacts are ${facts.formPackagePublicationStatus}`,
    ],
    [
      "Provider distribution",
      facts.providerPublished,
      "current Registry-published typed reference implementation; not Specification authority",
    ],
  ];
  return [
    BEGIN,
    "",
    "| Axis | Current identity | |",
    "| --- | --- | --- |",
    ...rows.map(([axis, value, note]) => `| ${axis} | \`${value}\` | ${note} |`),
    "",
    "These identities are independent. A Specification 1.1 release does not",
    "publish or promote the separate Host API v1 candidate, relabel any current",
    "Form as `1.0.0`, publish a Form Package, or release the non-normative Provider.",
    "This table is generated from repository bytes",
    "by `bun run sync:current-generation`; the numbered release ledger derives",
    "the Specification row as `candidate-open` or `released` without changing any",
    "Host API, Form, package, or Provider identity.",
    "",
    END,
  ].join("\n");
}

const facts = deriveSiteStatusFacts(repositoryRoot);
const readme = readFileSync(readmePath, "utf8");
const begin = readme.indexOf(BEGIN);
const end = readme.indexOf(END);
if (begin === -1 || end === -1 || end < begin) {
  throw new Error(
    `README.md must carry the ${BEGIN} … ${END} block this command generates`,
  );
}
const expected =
  readme.slice(0, begin) + render(facts) + readme.slice(end + END.length);

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

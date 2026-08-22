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
    ["Host API lane", facts.hostApiCurrent, "the wire a provider speaks"],
    [
      "Form Family",
      facts.formFamilyCurrent,
      `${facts.currentFormCount} Forms, each \`0.1.0\` and ${facts.formMaturity}`,
    ],
    [
      "Form Package envelope",
      facts.formPackageApiCurrent,
      `package artifacts are ${facts.formPackagePublicationStatus}`,
    ],
    [
      "Provider",
      facts.providerPublished,
      "installed from the Terraform Registry",
    ],
  ];
  return [
    BEGIN,
    "",
    "| Axis | Current identity | |",
    "| --- | --- | --- |",
    ...rows.map(([axis, value, note]) => `| ${axis} | \`${value}\` | ${note} |`),
    "",
    "These four numbers do not line up, and they are not supposed to: the axes",
    "change for different reasons. They also do not sort the same way — the lane",
    "went `v1alpha3` → `v1beta1`, where the digit falls, while the envelope went",
    "`v1alpha3` → `v1alpha4`, where it rises. **Do not infer which identity is",
    "current from a version word.** This table is generated from the repository",
    `by \`bun run sync:current-generation\`, and ${facts.openPublicationBlockers} publication obligations in`,
    "[`spec/publication-blockers.json`](spec/publication-blockers.json) are open.",
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
    `README current generation: ${facts.hostApiCurrent}, ${facts.formFamilyCurrent}, provider ${facts.providerPublished}\n`,
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
    `${facts.formFamilyCurrent}, provider ${facts.providerPublished}\n`,
);

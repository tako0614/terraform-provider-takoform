#!/usr/bin/env bun

// Render the small current-version note in README from the same Provider facts
// used by the site status document. Detailed history belongs on the versions
// page; Form publication belongs to its publisher.

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
const CORE_API_LANE = "forms.takoform.com/v1";
function render(facts) {
  return [
    BEGIN,
    "",
    `Registry Provider \`${facts.providerPublished}\` is retained aggregate history. Provider \`${facts.providerTarget}\` is the candidate at the same \`tako0614/takoform\` source address and registers only the 17 Forms selected from \`tako0614/takoform-forms\`. Core \`${CORE_API_RELEASE_VERSION}\` implements \`${CORE_API_LANE}\`; neither Provider release changes that API. See [Versions and compatibility](website/docs/versions.md).`,
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

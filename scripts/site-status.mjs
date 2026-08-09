#!/usr/bin/env bun

// site-status.mjs — the gate for /.well-known/takoform-site.json.
//
//   bun scripts/site-status.mjs --write          # re-derive the document
//   bun scripts/site-status.mjs --check          # facts must match the repository
//   bun scripts/site-status.mjs --check-commit   # sourceCommit must be a real ancestor
//
// The document is written by the website build (website/.vitepress/config.mts
// calls the same derivation), so a rebuild always refreshes it. These checks
// exist so a committed document cannot quietly disagree with the repository it
// claims to describe.
//
// The two modes are split on purpose. --check reads only files and therefore
// runs everywhere, including the deploy path's Git-free archive. --check-commit
// needs Git metadata, so it is a separate top-level step of `bun run check` and
// never runs inside that archive.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  COMMIT_PATTERN,
  SITE_STATUS_FIELDS,
  SITE_STATUS_REPOSITORY_PATH,
  deriveSiteStatusFacts,
  prepareSiteStatus,
  renderSiteStatusDocument,
  resolveSourceCommit,
} from "../website/.vitepress/site-status.mjs";
import { loadPublicationTruth } from "./publication-truth.mjs";

export { SITE_STATUS_REPOSITORY_PATH };

function documentPath(repositoryRoot) {
  return path.join(repositoryRoot, SITE_STATUS_REPOSITORY_PATH);
}

function readDocument(repositoryRoot, failures) {
  const filePath = documentPath(repositoryRoot);
  if (!existsSync(filePath)) {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: missing; run bun run website:build`,
    );
    return null;
  }
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    failures.push(`${SITE_STATUS_REPOSITORY_PATH}: invalid JSON (${error.message})`);
    return null;
  }
}

/**
 * verifySiteStatusDocument re-derives every published/preview fact and compares
 * it with the committed document. It never consults Git: the source commit is
 * only checked for shape here, and bound to real history by
 * verifySiteStatusCommit.
 *
 * `truth` is the evidence-derived publication truth when the caller already has
 * it; passing it binds providerCurrent to the signed Registry readback rather
 * than to the release descriptor alone.
 */
export function verifySiteStatusDocument(repositoryRoot, truth = null) {
  const failures = [];
  const document = readDocument(repositoryRoot, failures);
  if (document === null) {
    return failures;
  }

  const actualFields = Object.keys(document);
  if (JSON.stringify(actualFields) !== JSON.stringify(SITE_STATUS_FIELDS)) {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: fields are ${actualFields.join(", ")}; ` +
        `want exactly ${SITE_STATUS_FIELDS.join(", ")} in that order`,
    );
  }

  let facts;
  try {
    facts = deriveSiteStatusFacts(repositoryRoot);
  } catch (error) {
    failures.push(`${SITE_STATUS_REPOSITORY_PATH}: cannot derive (${error.message})`);
    return failures;
  }

  for (const [field, derived] of Object.entries(facts)) {
    if (document[field] !== derived) {
      failures.push(
        `${SITE_STATUS_REPOSITORY_PATH}: ${field} = ${JSON.stringify(document[field])}, ` +
          `but the repository derives ${JSON.stringify(derived)}; run bun run website:build`,
      );
    }
  }

  if (truth !== null && document.providerCurrent !== truth.providerVersion) {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: providerCurrent = ${JSON.stringify(document.providerCurrent)}, ` +
        `but the retained Registry readback evidence names v${truth.providerVersion}`,
    );
  }

  if (
    typeof document.sourceCommit !== "string" ||
    !COMMIT_PATTERN.test(document.sourceCommit)
  ) {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: sourceCommit must be a 40-character commit id`,
    );
  }

  return failures;
}

function git(repositoryRoot, args) {
  return execFileSync("git", args, {
    cwd: repositoryRoot,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();
}

/**
 * verifySiteStatusCommit binds the recorded commit to real history: it must be
 * a commit this repository has, and it must be an ancestor of HEAD.
 *
 * An ancestor is the strongest true statement available. A page cannot name the
 * commit that contains it, so the recorded commit is the one the build ran
 * against — necessarily an ancestor of the commit that carries the built bytes.
 * Staleness beyond that is caught elsewhere: check-website-dist proves the
 * pages are not stale, and --check proves the facts are not.
 */
export function verifySiteStatusCommit(repositoryRoot) {
  const failures = [];
  const document = readDocument(repositoryRoot, failures);
  if (document === null) {
    return failures;
  }
  const commit = document.sourceCommit;
  if (typeof commit !== "string" || !COMMIT_PATTERN.test(commit)) {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: sourceCommit must be a 40-character commit id`,
    );
    return failures;
  }
  try {
    if (git(repositoryRoot, ["cat-file", "-t", commit]) !== "commit") {
      failures.push(
        `${SITE_STATUS_REPOSITORY_PATH}: sourceCommit ${commit} is not a commit in this repository`,
      );
      return failures;
    }
  } catch {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: sourceCommit ${commit} is not an object in this repository`,
    );
    return failures;
  }
  try {
    git(repositoryRoot, ["merge-base", "--is-ancestor", commit, "HEAD"]);
  } catch {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: sourceCommit ${commit} is not an ancestor of HEAD; ` +
        "the committed site was built from history this branch does not contain",
    );
  }
  return failures;
}

function main() {
  const repositoryRoot = path.resolve(import.meta.dirname, "..");
  const mode = process.argv[2] ?? "--check";

  if (mode === "--write") {
    const facts = prepareSiteStatus(path.join(repositoryRoot, "website"));
    const commit = resolveSourceCommit(
      repositoryRoot,
      JSON.parse(readFileSync(documentPath(repositoryRoot), "utf8")),
    );
    process.stdout.write(
      `${SITE_STATUS_REPOSITORY_PATH} written: ${renderSiteStatusDocument(facts, commit)}`,
    );
    return;
  }

  let failures;
  if (mode === "--check") {
    failures = verifySiteStatusDocument(repositoryRoot, loadPublicationTruth(repositoryRoot));
  } else if (mode === "--check-commit") {
    failures = verifySiteStatusCommit(repositoryRoot);
  } else {
    process.stderr.write(
      "usage: bun scripts/site-status.mjs [--write|--check|--check-commit]\n",
    );
    process.exitCode = 1;
    return;
  }

  if (failures.length > 0) {
    for (const failure of failures) {
      process.stderr.write(`- ${failure}\n`);
    }
    process.exitCode = 1;
    return;
  }
  process.stdout.write(
    mode === "--check"
      ? `site status OK: ${SITE_STATUS_REPOSITORY_PATH} states the repository's own published and preview facts\n`
      : `site status OK: the recorded source commit is an ancestor of HEAD\n`,
  );
}

if (import.meta.main) {
  main();
}

#!/usr/bin/env bun

// site-status.mjs — the gate for /.well-known/takoform-site.json.
//
//   bun scripts/site-status.mjs --write   # re-derive the document
//   bun scripts/site-status.mjs --check   # both copies must match the repository
//
// The document is written by the website build (website/.vitepress/config.mts
// calls the same derivation), so a rebuild always refreshes it. This check
// exists so a committed document cannot quietly disagree with the repository it
// claims to describe.
//
// There are two committed copies and BOTH are checked. website/static/... is
// the source VitePress copies from; website/public/... is the build output the
// deploy uploads and production serves. Checking only the source would leave
// the served bytes free to drift, which is exactly the hole this gate closes
// alongside scripts/check-website-dist.mjs.
//
// The check reads only files and therefore runs everywhere, including the
// deploy path's Git-free archive. It consults Git for nothing: the document
// records no commit, because a commit id inside the published tree cannot be
// both the commit that carries the bytes and reproducible from it (see
// website/.vitepress/site-status.mjs).

import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";

import {
  SITE_STATUS_DEPRECATED_FIELDS,
  SITE_STATUS_FIELDS,
  SITE_STATUS_PUBLISHED_PATH,
  SITE_STATUS_REPOSITORY_PATH,
  deriveSiteStatusFacts,
  prepareSiteStatus,
  renderSiteStatusDocument,
} from "../website/.vitepress/site-status.mjs";

export { SITE_STATUS_PUBLISHED_PATH, SITE_STATUS_REPOSITORY_PATH };

// Both copies are compared against the same derivation, so neither can be
// corrected by editing the other.
const SITE_STATUS_COPIES = [
  SITE_STATUS_REPOSITORY_PATH,
  SITE_STATUS_PUBLISHED_PATH,
];

function readDocument(repositoryRoot, relativePath, failures) {
  const filePath = path.join(repositoryRoot, relativePath);
  if (!existsSync(filePath)) {
    failures.push(`${relativePath}: missing; run bun run website:build`);
    return null;
  }
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    failures.push(`${relativePath}: invalid JSON (${error.message})`);
    return null;
  }
}

function verifyOneCopy(document, relativePath, facts, failures) {
  const actualFields = Object.keys(document);
  if (JSON.stringify(actualFields) !== JSON.stringify(SITE_STATUS_FIELDS)) {
    failures.push(
      `${relativePath}: fields are ${actualFields.join(", ")}; ` +
        `want exactly ${SITE_STATUS_FIELDS.join(", ")} in that order`,
    );
  }

  for (const field of Object.keys(SITE_STATUS_DEPRECATED_FIELDS)) {
    if (!SITE_STATUS_FIELDS.includes(field)) {
      failures.push(
        `${relativePath}: deprecated field ${field} is not represented in the status schema`,
      );
    }
  }

  for (const [field, derived] of Object.entries(facts)) {
    if (document[field] !== derived) {
      failures.push(
        `${relativePath}: ${field} = ${JSON.stringify(document[field])}, ` +
          `but the repository derives ${JSON.stringify(derived)}; run bun run website:build`,
      );
    }
  }

  // These invariants make the maturity/publication axes mechanically
  // independent. A stale document that labels the family Experimental can no
  // longer pass merely because the candidate set still says its definitions
  // are Experimental.
  //
  // There is no family maturity to check against the definition maturity any
  // more: the axis was retired with the generation it was read out of
  // (decision 0049), and what is left is per-Form.
  if (document.formMaturity !== "experimental") {
    failures.push(
      `${relativePath}: formMaturity must be "experimental" for the current ` +
        "Form definitions",
    );
  }
  if (document.hostApiPublicationStatus !== "unpublished-candidate") {
    failures.push(
      `${relativePath}: retained hostApiPublicationStatus must keep Host API v1 ` +
        "separate and unpublished as W09 compatibility metadata",
    );
  }
  if (
    document.formPackageStatus !== document.formPackagePublicationStatus ||
    document.edgeFamilyStatus !== document.formPackagePublicationStatus
  ) {
    failures.push(
      `${relativePath}: deprecated package aliases must equal ` +
        "formPackagePublicationStatus and must not describe Form Family maturity",
    );
  }

}

/**
 * verifySiteStatusDocument re-derives every published/preview fact and compares
 * it with both committed copies: the VitePress source under website/static and
 * the build output under website/public that the deploy uploads.
 * providerPublished (and its deprecated providerCurrent alias) derive from the
 * append-only Registry-readback entries in release/provider-form-identities.json.
 */
export function verifySiteStatusDocument(repositoryRoot) {
  const failures = [];
  const documents = SITE_STATUS_COPIES.map((relativePath) => ({
    document: readDocument(repositoryRoot, relativePath, failures),
    relativePath,
  })).filter(({ document }) => document !== null);
  if (documents.length === 0) {
    return failures;
  }

  let facts;
  try {
    facts = deriveSiteStatusFacts(repositoryRoot);
  } catch (error) {
    failures.push(
      `${SITE_STATUS_REPOSITORY_PATH}: cannot derive (${error.message})`,
    );
    return failures;
  }

  for (const { document, relativePath } of documents) {
    verifyOneCopy(document, relativePath, facts, failures);
  }

  // The two copies must also be byte-identical: VitePress copies static/
  // verbatim into public/, so any difference means one of them was written by
  // something other than the build.
  if (documents.length === SITE_STATUS_COPIES.length) {
    const [source, published] = SITE_STATUS_COPIES.map((relativePath) =>
      readFileSync(path.join(repositoryRoot, relativePath)),
    );
    if (!source.equals(published)) {
      failures.push(
        `${SITE_STATUS_PUBLISHED_PATH}: bytes differ from ${SITE_STATUS_REPOSITORY_PATH}; ` +
          "the served copy is not the one the build produced; run bun run website:build",
      );
    }
  }

  return failures;
}

function main() {
  const repositoryRoot = path.resolve(import.meta.dirname, "..");
  const mode = process.argv[2] ?? "--check";

  if (mode === "--write") {
    const facts = prepareSiteStatus(path.join(repositoryRoot, "website"));
    process.stdout.write(
      `${SITE_STATUS_REPOSITORY_PATH} written: ${renderSiteStatusDocument(facts)}`,
    );
    return;
  }

  if (mode !== "--check") {
    process.stderr.write("usage: bun scripts/site-status.mjs [--write|--check]\n");
    process.exitCode = 1;
    return;
  }

  const failures = verifySiteStatusDocument(repositoryRoot);
  if (failures.length > 0) {
    for (const failure of failures) {
      process.stderr.write(`- ${failure}\n`);
    }
    process.exitCode = 1;
    return;
  }
  process.stdout.write(
    `site status OK: ${SITE_STATUS_COPIES.join(" and ")} state the repository's own published and preview facts\n`,
  );
}

if (import.meta.main) {
  main();
}

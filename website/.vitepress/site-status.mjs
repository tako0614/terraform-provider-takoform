// site-status.mjs — the one derivation of "what is published and what is
// preview", used by every surface that states it.
//
// Two callers share this module so no surface can hand-maintain a fact the
// repository already knows:
//
//   - website/.vitepress/config.mts writes /.well-known/takoform-site.json at
//     build time and puts the same facts into themeConfig, so the footer is
//     static HTML rather than a second copy of the numbers;
//   - scripts/site-status.mjs re-derives them in the gate and refuses a
//     committed document that disagrees with the repository.
//
// The source commit is the ONE value that may not enter a page.
// scripts/check-website-dist.mjs requires a fresh build to reproduce every
// committed page semantically, and a page that names the commit it was built
// from can never rebuild to itself: the commit that contains the page does not
// exist while the page is being produced. The commit therefore lives only in
// the JSON document, which is not a page and is not compared by that gate, and
// the footer reads it from there at runtime. Its meaning is exact: the
// repository commit this build was produced from, always an ancestor of the
// commit that carries the built bytes.
//
// This file lives under website/ rather than scripts/ on purpose. The deploy
// path re-derives the committed site from a copy that contains only
// package.json, bun.lock, scripts/check-website-dist.mjs and website/, with no
// repository root and no Git metadata. There, deriveSiteStatus finds no root
// and the committed document stands unchanged, which is exactly right: that
// copy is frozen at one commit and has nothing newer to say.

import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

export const SITE_STATUS_ROUTE = "/.well-known/takoform-site.json";
export const SITE_STATUS_SITE_PATH = "static/.well-known/takoform-site.json";
export const SITE_STATUS_REPOSITORY_PATH =
  "website/static/.well-known/takoform-site.json";

// The Edge Platform Family rides an unpublished provider source candidate.
// This is the single JS-side declaration of that line; check-public-surfaces
// binds it to the generated Edge Family examples, which pin the same version,
// so the constant cannot drift away from the bytes the generator produces.
export const EDGE_PREVIEW_PROVIDER_VERSION = "2.1.0";
export const EDGE_PREVIEW_PROVIDER = `${EDGE_PREVIEW_PROVIDER_VERSION}-source`;

export const FAMILY_CANDIDATE_SET =
  "forms/candidates/edge/v1alpha1/candidate-set.json";
const BLOCKER_LEDGER = "spec/publication-blockers.json";
const RELEASE_VERSION = "release/version.json";

export const SITE_STATUS_FIELDS = [
  "sourceCommit",
  "providerCurrent",
  "edgePreviewProvider",
  "edgeFamilyStatus",
  "candidateSetDigest",
  "openPublicationBlockers",
];

export const COMMIT_PATTERN = /^[0-9a-f]{40}$/u;

const ROOT_MARKERS = [BLOCKER_LEDGER, FAMILY_CANDIDATE_SET, RELEASE_VERSION];

/**
 * findRepositoryRoot walks up from a directory until it finds the tree that
 * carries every file the status is derived from. It returns null rather than
 * guessing, because a copy of website/ alone has nothing to derive from.
 */
export function findRepositoryRoot(startDirectory) {
  let directory = path.resolve(startDirectory);
  for (;;) {
    if (ROOT_MARKERS.every((marker) => existsSync(path.join(directory, marker)))) {
      return directory;
    }
    const parent = path.dirname(directory);
    if (parent === directory) {
      return null;
    }
    directory = parent;
  }
}

function readJson(repositoryRoot, relativePath) {
  const filePath = path.join(repositoryRoot, relativePath);
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    throw new Error(`${relativePath}: cannot be read as JSON (${error.message})`);
  }
}

/**
 * deriveSiteStatusFacts reads every published/preview fact out of the
 * repository. Nothing here is a literal a human keeps in step by hand.
 */
export function deriveSiteStatusFacts(repositoryRoot) {
  const releaseVersion = readJson(repositoryRoot, RELEASE_VERSION);
  const providerCurrent = releaseVersion.version;
  if (typeof providerCurrent !== "string" || providerCurrent === "") {
    throw new Error(`${RELEASE_VERSION}: version must be a non-empty string`);
  }

  const candidateSetBytes = readFileSync(
    path.join(repositoryRoot, FAMILY_CANDIDATE_SET),
  );
  const candidateSet = JSON.parse(candidateSetBytes.toString("utf8"));
  const edgeFamilyStatus = candidateSet.publicationStatus;
  if (typeof edgeFamilyStatus !== "string" || edgeFamilyStatus === "") {
    throw new Error(
      `${FAMILY_CANDIDATE_SET}: publicationStatus must be a non-empty string`,
    );
  }

  const ledger = readJson(repositoryRoot, BLOCKER_LEDGER);
  if (!Array.isArray(ledger.blockers)) {
    throw new Error(`${BLOCKER_LEDGER}: blockers must be an array`);
  }
  const openPublicationBlockers = ledger.blockers.filter(
    (blocker) => blocker?.status === "open",
  ).length;

  return {
    providerCurrent,
    edgePreviewProvider: EDGE_PREVIEW_PROVIDER,
    edgeFamilyStatus,
    candidateSetDigest: `sha256:${createHash("sha256").update(candidateSetBytes).digest("hex")}`,
    openPublicationBlockers,
  };
}

/**
 * resolveSourceCommit prefers an explicit build input, then the repository's
 * own HEAD, then the commit the committed document already records. A build
 * that can reach none of them fails rather than inventing provenance.
 */
export function resolveSourceCommit(repositoryRoot, committed = null) {
  const declared = process.env.TAKOFORM_SITE_COMMIT;
  if (typeof declared === "string" && COMMIT_PATTERN.test(declared.trim())) {
    return declared.trim();
  }
  try {
    const head = execFileSync("git", ["rev-parse", "HEAD"], {
      cwd: repositoryRoot,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
    }).trim();
    if (COMMIT_PATTERN.test(head)) {
      return head;
    }
  } catch {
    // No Git metadata reachable; fall through to the committed record.
  }
  if (
    committed !== null &&
    typeof committed.sourceCommit === "string" &&
    COMMIT_PATTERN.test(committed.sourceCommit)
  ) {
    return committed.sourceCommit;
  }
  throw new Error(
    "cannot determine the source commit: set TAKOFORM_SITE_COMMIT, build inside " +
      "the Git repository, or keep the committed status document",
  );
}

export function renderSiteStatusDocument(facts, sourceCommit) {
  if (!COMMIT_PATTERN.test(String(sourceCommit))) {
    throw new Error(`sourceCommit must be a 40-character commit id, got ${sourceCommit}`);
  }
  const document = {
    sourceCommit,
    providerCurrent: facts.providerCurrent,
    edgePreviewProvider: facts.edgePreviewProvider,
    edgeFamilyStatus: facts.edgeFamilyStatus,
    candidateSetDigest: facts.candidateSetDigest,
    openPublicationBlockers: facts.openPublicationBlockers,
  };
  return `${JSON.stringify(document, null, 2)}\n`;
}

function readCommitted(filePath) {
  if (!existsSync(filePath)) {
    return null;
  }
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch {
    return null;
  }
}

/**
 * prepareSiteStatus is the build-time entry point. `siteDirectory` is the
 * VitePress source root (the directory holding .vitepress/ and static/).
 *
 * It returns the facts the footer renders. The commit is deliberately NOT part
 * of that return value: it must not reach a page.
 */
export function prepareSiteStatus(siteDirectory) {
  const statusPath = path.join(siteDirectory, SITE_STATUS_SITE_PATH);
  const committed = readCommitted(statusPath);
  const repositoryRoot = findRepositoryRoot(siteDirectory);

  if (repositoryRoot === null) {
    if (committed === null) {
      throw new Error(
        `${SITE_STATUS_SITE_PATH} is missing and no repository root is reachable ` +
          "from this copy; the status document cannot be derived",
      );
    }
    // A frozen copy of website/ alone: the committed document is the only
    // truth available and re-deriving it here would be a fabrication.
    return {
      providerCurrent: committed.providerCurrent,
      edgePreviewProvider: committed.edgePreviewProvider,
      edgeFamilyStatus: committed.edgeFamilyStatus,
      candidateSetDigest: committed.candidateSetDigest,
      openPublicationBlockers: committed.openPublicationBlockers,
    };
  }

  const facts = deriveSiteStatusFacts(repositoryRoot);
  const sourceCommit = resolveSourceCommit(repositoryRoot, committed);
  const rendered = renderSiteStatusDocument(facts, sourceCommit);
  mkdirSync(path.dirname(statusPath), { recursive: true });
  writeFileSync(statusPath, rendered);
  return facts;
}

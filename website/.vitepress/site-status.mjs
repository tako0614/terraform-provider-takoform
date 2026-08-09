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
// The document records no commit, and that is a decision rather than an
// omission.
//
// A commit id inside the published tree can only be one of two things. If it
// is written when the tree is committed, it names the parent — the commit that
// carries the bytes does not exist while they are produced — so the one link a
// reader would use to reproduce the bytes points at a tree that cannot produce
// them. If instead the deploy stamped it, the value would be right but the
// byte would be one no reviewer read and no checkout reproduces, and
// scripts/check-website-dist.mjs would then be comparing a document production
// does not serve. Both defeat the purpose of recording provenance at all.
//
// So every field here is a pure function of committed repository bytes, and
// the whole published tree is therefore reproducible from the commit that
// carries it: scripts/check-website-dist.mjs proves a fresh build reproduces
// it. The commit is recorded where a commit can be true — the takoform-website
// Worker version message (`takoform.com <commit>`), which scripts/deploy.mjs
// writes, verifies on upload, and reads back with an ancestor check on the
// next deploy, alongside the per-asset digests in its deploy result.
//
// This file lives under website/ rather than scripts/ on purpose. The deploy
// path re-derives the committed site from a copy that contains only
// package.json, bun.lock, scripts/check-website-dist.mjs and website/, with no
// repository root and no Git metadata. There, deriveSiteStatus finds no root
// and the committed document stands unchanged, which is exactly right: that
// copy is frozen at one commit and has nothing newer to say.

import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

export const SITE_STATUS_ROUTE = "/.well-known/takoform-site.json";
export const SITE_STATUS_SITE_PATH = "static/.well-known/takoform-site.json";
export const SITE_STATUS_REPOSITORY_PATH =
  "website/static/.well-known/takoform-site.json";
// The build output copy is the one production actually serves. VitePress
// copies static/ verbatim into public/, so the two must be byte-identical.
export const SITE_STATUS_PUBLISHED_PATH =
  "website/public/.well-known/takoform-site.json";

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
  "providerCurrent",
  "edgePreviewProvider",
  "edgeFamilyStatus",
  "candidateSetDigest",
  "openPublicationBlockers",
];

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
 * repository. Nothing here is a literal a human keeps in step by hand, and
 * nothing here depends on when or where the build ran.
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

export function renderSiteStatusDocument(facts) {
  const document = {
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
 * It returns the facts the footer renders, which are exactly the facts the
 * JSON document states.
 */
export function prepareSiteStatus(siteDirectory) {
  const statusPath = path.join(siteDirectory, SITE_STATUS_SITE_PATH);
  const repositoryRoot = findRepositoryRoot(siteDirectory);

  if (repositoryRoot === null) {
    const committed = readCommitted(statusPath);
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
  mkdirSync(path.dirname(statusPath), { recursive: true });
  writeFileSync(statusPath, renderSiteStatusDocument(facts));
  return facts;
}

// site-status.mjs — the one derivation of the independent Provider, Host API,
// Form Family and Form-definition status axes used by public surfaces.
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

// This constant pins generated examples to the provider release target. It is
// deliberately named as a Provider SemVer, not as an API lane: Provider,
// Host API, Form Family and Form-definition versions are independent axes.
export const PROVIDER_RELEASE_TARGET_VERSION = "3.0.0";
// Retain the legacy field name/value for consumers that still describe the
// Edge preview descriptor. It is metadata only: providerTargetStatus below is
// the independent Registry availability fact.
export const EDGE_PREVIEW_PROVIDER_VERSION = PROVIDER_RELEASE_TARGET_VERSION;
export const EDGE_PREVIEW_PROVIDER = `${EDGE_PREVIEW_PROVIDER_VERSION}-candidate-only`;

export const FAMILY_CANDIDATE_SET =
  "forms/candidates/edge.forms.takoform.com/candidate-set.json";
export const CURRENT_FAMILY_INDEX =
  "forms/candidates/current-family-index.json";
const BLOCKER_LEDGER = "spec/publication-blockers.json";
const RELEASE_VERSION = "release/version.json";
const PROVIDER_RELEASE_IDENTITIES = "release/provider-release-identities.json";
const SPECIFICATION_RELEASES = "release/specification-releases.json";

export const SITE_STATUS_FIELDS = [
  // Keep the v1 field prefix byte/order-compatible for tolerant existing
  // readers. New readers should use the explicit axes below.
  "providerCurrent",
  "edgePreviewProvider",
  "edgeFamilyStatus",
  "candidateSetDigest",
  "openPublicationBlockers",
  "format",
  "providerPublished",
  "providerTarget",
  "providerTargetStatus",
  "hostApiCurrent",
  "hostApiMaturity",
  "formFamilyCurrent",
  "formPackageApiCurrent",
  "formPackageStatus",
  "currentFormCount",
  // Canonical current-lane axes. These are appended so readers that only
  // understand the original v2 prefix keep receiving the same keys/order.
  "formMaturity",
  "formPackagePublicationStatus",
  // Specification 1.0 and the complete current corpus are appended so the
  // original Provider/Edge prefix remains tolerant-reader compatible.
  "specificationVersion",
  "specificationReleaseStatus",
  "currentFamilyIndex",
  "currentFamilyIndexDigest",
  "currentFamilyCount",
];

// These names remain in the JSON document for tolerant, older readers. They
// are compatibility aliases or descriptor metadata, never publication or
// maturity authority. New code must use the explicit fields above.
export const SITE_STATUS_DEPRECATED_FIELDS = Object.freeze({
  providerCurrent: "alias of providerPublished",
  edgePreviewProvider:
    "descriptor metadata only; providerTargetStatus is availability authority",
  edgeFamilyStatus:
    "alias of formPackagePublicationStatus; not Form Family maturity",
  formPackageStatus: "alias of formPackagePublicationStatus",
  formFamilyCurrent:
    "retained Edge-family compatibility field; currentFamilyIndex is corpus authority",
  candidateSetDigest:
    "retained Edge-family compatibility digest; currentFamilyIndexDigest is corpus authority",
  openPublicationBlockers:
    "retained beta-history count; specificationReleaseStatus is Specification authority",
});

const ROOT_MARKERS = [
  BLOCKER_LEDGER,
  FAMILY_CANDIDATE_SET,
  CURRENT_FAMILY_INDEX,
  RELEASE_VERSION,
  PROVIDER_RELEASE_IDENTITIES,
  SPECIFICATION_RELEASES,
];

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
 * deriveSiteStatusFacts reads every release, protocol, family and package fact
 * out of the repository. Nothing here is a literal a human keeps in step by
 * hand, and nothing here depends on when or where the build ran.
 */
export function deriveSiteStatusFacts(repositoryRoot) {
  const releaseVersion = readJson(repositoryRoot, RELEASE_VERSION);
  const providerReleaseTarget = releaseVersion.version;
  if (
    providerReleaseTarget !== PROVIDER_RELEASE_TARGET_VERSION ||
    typeof releaseVersion.publicationStatus !== "string" ||
    releaseVersion.publicationStatus === ""
  ) {
    throw new Error(
      `${RELEASE_VERSION}: expected provider target ${PROVIDER_RELEASE_TARGET_VERSION} ` +
        "with a non-empty publicationStatus",
    );
  }

  const specificationReleases = readJson(repositoryRoot, SPECIFICATION_RELEASES);
  const specificationVersion = specificationReleases.candidate?.version;
  const hostApiCurrent = specificationReleases.candidate?.hostApiLane;
  if (
    specificationReleases.kind !== "takoform.specification-releases@v1" ||
    specificationVersion !== "1.0" ||
    !Array.isArray(specificationReleases.releases)
  ) {
    throw new Error(
      `${SPECIFICATION_RELEASES}: expected the Specification 1.0 candidate and append-only releases`,
    );
  }
  if (typeof hostApiCurrent !== "string" || hostApiCurrent === "") {
    throw new Error(
      `${SPECIFICATION_RELEASES}: candidate.hostApiLane must be a non-empty string`,
    );
  }
  const hostApiMaturityMatch = hostApiCurrent.match(/\/v\d+(alpha|beta)\d+$/);
  const hostApiMaturity = hostApiMaturityMatch?.[1] ??
    (/\/v\d+$/.test(hostApiCurrent) ? "stable" : null);
  if (hostApiMaturity === null) {
    throw new Error(
      `${SPECIFICATION_RELEASES}: cannot derive Host API maturity from ${JSON.stringify(hostApiCurrent)}`,
    );
  }
  const specificationReleaseStatus = specificationReleases.releases.some(
    (release) => release?.version === specificationVersion,
  )
    ? "released"
    : "candidate-open";

  const releaseIdentities = readJson(repositoryRoot, PROVIDER_RELEASE_IDENTITIES);
  const publishedEntries = Array.isArray(releaseIdentities.entries)
    ? releaseIdentities.entries.filter((entry) => entry?.registryReadback)
    : [];
  const providerPublished = publishedEntries.at(-1)?.version;
  if (typeof providerPublished !== "string" || providerPublished === "") {
    throw new Error(
      `${PROVIDER_RELEASE_IDENTITIES}: no retained Registry-readback release`,
    );
  }
  const providerTargetStatus = publishedEntries.some(
    (entry) => entry?.version === providerReleaseTarget,
  )
    ? "registry-published"
    : releaseVersion.publicationStatus;

  const edgeCandidateSetBytes = readFileSync(
    path.join(repositoryRoot, FAMILY_CANDIDATE_SET),
  );
  const edgeCandidateSet = JSON.parse(edgeCandidateSetBytes.toString("utf8"));
  const currentFamilyIndexBytes = readFileSync(
    path.join(repositoryRoot, CURRENT_FAMILY_INDEX),
  );
  const currentFamilyIndexDocument = JSON.parse(
    currentFamilyIndexBytes.toString("utf8"),
  );
  if (
    currentFamilyIndexDocument.format !== "takoform.current-family-index@v1" ||
    !Array.isArray(currentFamilyIndexDocument.families) ||
    currentFamilyIndexDocument.families.length === 0
  ) {
    throw new Error(
      `${CURRENT_FAMILY_INDEX}: expected a non-empty takoform.current-family-index@v1 document`,
    );
  }
  const familyCandidateSets = currentFamilyIndexDocument.families.map((family) => {
    const candidateSetPath = family?.candidateSet;
    if (typeof candidateSetPath !== "string" || candidateSetPath === "") {
      throw new Error(`${CURRENT_FAMILY_INDEX}: family candidateSet must be a path`);
    }
    const bytes = readFileSync(path.join(repositoryRoot, candidateSetPath));
    const digest = createHash("sha256").update(bytes).digest("hex");
    if (family.sha256 !== digest) {
      throw new Error(
        `${CURRENT_FAMILY_INDEX}: ${candidateSetPath} digest differs from the index`,
      );
    }
    const candidateSet = JSON.parse(bytes.toString("utf8"));
    if (
      candidateSet.family !== family.group ||
      !Array.isArray(candidateSet.forms) ||
      candidateSet.forms.length !== family.formCount
    ) {
      throw new Error(
        `${CURRENT_FAMILY_INDEX}: ${candidateSetPath} identity or Form count differs from the index`,
      );
    }
    return candidateSet;
  });
  const exactSharedValue = (field) => {
    const values = new Set(familyCandidateSets.map((family) => family[field]));
    if (values.size !== 1 || typeof [...values][0] !== "string" || [...values][0] === "") {
      throw new Error(`${CURRENT_FAMILY_INDEX}: every family must share one non-empty ${field}`);
    }
    return [...values][0];
  };
  const formPackagePublicationStatus = exactSharedValue("publicationStatus");
  const formFamilyCurrent = edgeCandidateSet.family;
  // There is no family maturity axis to derive. It was read out of the version
  // segment inside the group, and a group carries none (decision 0049): a
  // channel is a property of a generation, and the family has no generations
  // left to attach one to. What survives is per-Form, which is the axis
  // formMaturity already publishes and the only one the repository can still
  // state truthfully.
  const formMaturity = exactSharedValue("formMaturity");
  const formPackageApiCurrent = exactSharedValue("packageApiVersion");
  if (typeof formFamilyCurrent !== "string" || formFamilyCurrent === "") {
    throw new Error(`${FAMILY_CANDIDATE_SET}: family must be a non-empty string`);
  }
  if (typeof formMaturity !== "string" || formMaturity === "") {
    throw new Error(
      `${FAMILY_CANDIDATE_SET}: formMaturity must be a non-empty string`,
    );
  }
  if (formMaturity !== "experimental") {
    throw new Error(
      `${CURRENT_FAMILY_INDEX}: current Form definitions must be experimental`,
    );
  }
  if (
    typeof formPackageApiCurrent !== "string" ||
    formPackageApiCurrent === ""
  ) {
    throw new Error(
      `${FAMILY_CANDIDATE_SET}: packageApiVersion must be a non-empty string`,
    );
  }
  const currentFormCount = familyCandidateSets.reduce(
    (total, family) => total + family.forms.length,
    0,
  );

  const ledger = readJson(repositoryRoot, BLOCKER_LEDGER);
  if (!Array.isArray(ledger.blockers)) {
    throw new Error(`${BLOCKER_LEDGER}: blockers must be an array`);
  }
  const openPublicationBlockers = ledger.blockers.filter(
    (blocker) => blocker?.status === "open",
  ).length;

  const facts = {
    format: "takoform.site-status@v4",
    providerPublished,
    providerTarget: providerReleaseTarget,
    providerTargetStatus,
    hostApiCurrent,
    hostApiMaturity,
    formFamilyCurrent,
    formPackageApiCurrent,
    formPackageStatus: formPackagePublicationStatus,
    currentFormCount,
    formMaturity,
    formPackagePublicationStatus,
    candidateSetDigest: `sha256:${createHash("sha256").update(edgeCandidateSetBytes).digest("hex")}`,
    openPublicationBlockers,
    providerCurrent: providerPublished,
    // The release descriptor intentionally remains candidate-only even after
    // the Provider bytes are Registry-published. Keep this legacy field
    // explicit so callers cannot mistake descriptor metadata for availability.
    edgePreviewProvider: EDGE_PREVIEW_PROVIDER,
    edgeFamilyStatus: formPackagePublicationStatus,
    specificationVersion,
    specificationReleaseStatus,
    currentFamilyIndex: CURRENT_FAMILY_INDEX,
    currentFamilyIndexDigest: `sha256:${createHash("sha256").update(currentFamilyIndexBytes).digest("hex")}`,
    currentFamilyCount: currentFamilyIndexDocument.families.length,
  };
  return Object.fromEntries(
    SITE_STATUS_FIELDS.map((field) => [field, facts[field]]),
  );
}

export function renderSiteStatusDocument(facts) {
  const document = Object.fromEntries(
    SITE_STATUS_FIELDS.map((field) => [field, facts[field]]),
  );
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
    return Object.fromEntries(
      SITE_STATUS_FIELDS.map((field) => [field, committed[field]]),
    );
  }

  const facts = deriveSiteStatusFacts(repositoryRoot);
  mkdirSync(path.dirname(statusPath), { recursive: true });
  writeFileSync(statusPath, renderSiteStatusDocument(facts));
  return facts;
}

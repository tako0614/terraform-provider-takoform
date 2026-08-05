import { lstatSync, readFileSync } from "node:fs";
import path from "node:path";

const SEMVER = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/u;
const SHA256 = /^sha256:[0-9a-f]{64}$/u;

function requireValue(condition, message) {
  if (!condition) throw new Error(message);
}

function requireString(value, label) {
  requireValue(typeof value === "string" && value !== "", `${label} must be a non-empty string`);
  return value;
}

function requireDigest(value, label) {
  requireValue(typeof value === "string" && SHA256.test(value), `${label} must be sha256:<lowercase-hex>`);
  return value;
}

function formIdentity(entry, index) {
  const label = `Form Package publication entries[${index}]`;
  const formRef = entry?.formRef;
  requireValue(formRef !== null && typeof formRef === "object", `${label}.formRef must be an object`);
  const apiVersion = requireString(formRef.apiVersion, `${label}.formRef.apiVersion`);
  const kind = requireString(entry?.kind, `${label}.kind`);
  requireValue(formRef.kind === kind, `${label}.kind must match formRef.kind`);
  requireValue(SEMVER.test(requireString(formRef.definitionVersion, `${label}.formRef.definitionVersion`)), `${label}.formRef.definitionVersion must be exact SemVer`);
  requireDigest(formRef.schemaDigest, `${label}.formRef.schemaDigest`);
  requireDigest(entry?.packageDigest, `${label}.packageDigest`);
  requireValue(entry?.immutable === true, `${label}.immutable must be true`);
  return { apiVersion, kind };
}

export function derivePublicationTruth({ publicationSet, providerIdentities, releaseVersion }) {
  requireValue(
    publicationSet?.format === "takoform.form-package-publication-set@v1",
    "Form Package publication set format must be takoform.form-package-publication-set@v1",
  );
  requireValue(
    publicationSet.publicationStatus === "published-immutable",
    "Form Package publication set status must be published-immutable",
  );
  requireValue(
    publicationSet.generation === "portable-v1",
    "retained Form Package publication set generation must be portable-v1",
  );
  requireValue(
    publicationSet.admissionStatus === "external-required",
    "retained Form Package publication set admissionStatus must remain historical external-required data",
  );
  const entries = publicationSet.entries;
  requireValue(Array.isArray(entries) && entries.length > 0, "Form Package publication entries must be non-empty");
  const identities = entries.map(formIdentity);
  const kinds = identities.map(({ kind }) => kind);
  requireValue(new Set(kinds).size === kinds.length, "Form Package publication entries contain duplicate kinds");
  const apiVersions = new Set(identities.map(({ apiVersion }) => apiVersion));
  requireValue(apiVersions.size === 1, "published Form Package identities disagree on apiVersion");

  requireValue(
    releaseVersion?.publicationStatus === "candidate-only",
    "release/version.json publicationStatus must remain candidate-only descriptor metadata",
  );
  const candidateProviderVersion = requireString(releaseVersion.version, "release/version.json.version");
  requireValue(SEMVER.test(candidateProviderVersion), "release/version.json version must be exact SemVer");
  requireValue(releaseVersion.tag === `v${candidateProviderVersion}`, "release/version.json tag must match version");
  requireValue(
    providerIdentities?.format === "takoform.provider-release-identities@v1" &&
      Array.isArray(providerIdentities.entries) && providerIdentities.entries.length > 0,
    "provider release identity ledger must contain assigned immutable releases",
  );
  const assigned = providerIdentities.entries.filter((entry) => entry?.status === "assigned");
  requireValue(assigned.length > 0, "provider release identity ledger has no assigned release");
  const publishedProvider = assigned.at(-1);
  const providerVersion = requireString(publishedProvider.version, "published provider version");
  requireValue(SEMVER.test(providerVersion), "published provider version must be exact SemVer");
  requireValue(publishedProvider.tag === `v${providerVersion}`, "published provider tag must match version");
  requireValue(providerVersion !== candidateProviderVersion, "current candidate must not reuse an immutable published provider version");

  return {
    apiVersion: [...apiVersions][0],
    providerAddress: requireString(releaseVersion.providerAddress, "release/version.json.providerAddress"),
    providerVersion,
    candidateProviderVersion,
    publishedCount: identities.length,
    publishedKinds: kinds,
  };
}

function normalizedSentences(text) {
  return String(text)
    .replace(/\s+/gu, " ")
    .split(/[!?。！？]+|[.](?=\s+[A-Z]|$)/u)
    .map((sentence) => sentence.trim())
    .filter((sentence) => sentence !== "");
}

function sentenceMatches(sentences, patterns) {
  return sentences.some((sentence) => patterns.every((pattern) => pattern.test(sentence)));
}

// validatePublicationClaimText prevents exact package publication from being
// promoted into a current maturity or central-admission claim in reader-facing
// copy.
export function validatePublicationClaimText(text, truth, label = "document") {
  const sentences = normalizedSentences(text);
  const count = new RegExp(`(^|\\D)${truth.publishedCount}(\\D|$)`, "u");
  requireValue(
    sentenceMatches(sentences, [count, /\b(?:Form Packages?|package identities)\b/iu, /\b(?:published|immutable)\b/iu]),
    `${label}: published package count is missing or ambiguous`,
  );
  requireValue(
    sentenceMatches(sentences, [/\bExperimental\b/iu, /\b(?:specification|project)\b/iu]),
    `${label}: Takoform Experimental project status is missing`,
  );
  requireValue(
    sentenceMatches(sentences, [/\bLegacy\b/iu, /\b(?:historical|published|pre-reset|existing)\b/iu]),
    `${label}: published identities are not classified as Legacy`,
  );
  requireValue(
    sentenceMatches(sentences, [/\b(?:central|Takoform-wide)\b/iu, /\b(?:approval|admission|approved)\b/iu, /\b(?:no|not|does not|without)\b/iu]),
    `${label}: absence of current central approval is not explicit`,
  );
  for (const sentence of sentences) {
    if (/\b(?:admitted|approved|portable-standard)\b/iu.test(sentence) &&
        !/\b(?:historical|Legacy|retained|formerly|old field|no|not|does not|without)\b/iu.test(sentence)) {
      throw new Error(`${label}: presents a historical admission field as a current claim`);
    }
    if (count.test(sentence) && /\b(?:Stable|approved|admitted)\b/iu.test(sentence) &&
        !/\b(?:not|no|does not)\b/iu.test(sentence)) {
      throw new Error(`${label}: confuses publication count with maturity or approval`);
    }
  }
  return true;
}

function readRegularJson(filePath, label) {
  const stats = lstatSync(filePath);
  requireValue(stats.isFile() && !stats.isSymbolicLink(), `${label} must be a regular non-symlink file`);
  try {
    return JSON.parse(readFileSync(filePath, "utf8"));
  } catch (error) {
    throw new Error(`${label} is invalid JSON: ${error.message}`);
  }
}

export function loadPublicationTruth(repositoryRoot) {
  return derivePublicationTruth({
    publicationSet: readRegularJson(
      path.join(repositoryRoot, "admission", "v4", "form-package-publication-set.json"),
      "retained Form Package publication set",
    ),
    releaseVersion: readRegularJson(
      path.join(repositoryRoot, "release", "version.json"),
      "provider release descriptor",
    ),
    providerIdentities: readRegularJson(
      path.join(repositoryRoot, "release", "provider-release-identities.json"),
      "provider release identity ledger",
    ),
  });
}

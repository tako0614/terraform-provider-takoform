import { createHash } from "node:crypto";
import {
  lstatSync,
  readFileSync,
  realpathSync,
} from "node:fs";
import path from "node:path";

const SEMVER = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/;
const SHA256 = /^sha256:[0-9a-f]{64}$/;
const COMMIT = /^[0-9a-f]{40}$/;

function requireValue(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function requireString(value, label) {
  requireValue(typeof value === "string" && value !== "", `${label} must be a non-empty string`);
  return value;
}

function requireDigest(value, label) {
  requireValue(
    typeof value === "string" && SHA256.test(value),
    `${label} must be sha256:<lowercase-hex>`,
  );
  return value;
}

function requireCommit(value, label) {
  requireValue(
    typeof value === "string" && COMMIT.test(value),
    `${label} must be lowercase 40-hex`,
  );
  return value;
}

function requireArray(value, label) {
  requireValue(Array.isArray(value), `${label} must be an array`);
  return value;
}

function requireUnique(values, label) {
  const duplicates = values.filter(
    (value, index) => values.indexOf(value) !== index,
  );
  requireValue(
    duplicates.length === 0,
    `${label} contains duplicate ${[...new Set(duplicates)].sort().join(", ")}`,
  );
}

function formRefIdentity(formRef, label) {
  requireValue(
    formRef !== null && typeof formRef === "object" && !Array.isArray(formRef),
    `${label} must be an object`,
  );
  const apiVersion = requireString(formRef.apiVersion, `${label}.apiVersion`);
  const kind = requireString(formRef.kind, `${label}.kind`);
  const definitionVersion = requireString(
    formRef.definitionVersion,
    `${label}.definitionVersion`,
  );
  requireValue(
    SEMVER.test(definitionVersion),
    `${label}.definitionVersion must be exact SemVer`,
  );
  const schemaDigest = requireDigest(
    formRef.schemaDigest,
    `${label}.schemaDigest`,
  );
  return JSON.stringify({
    apiVersion,
    definitionVersion,
    kind,
    schemaDigest,
  });
}

function publicationIdentity(entry, index) {
  const label = `form-package-publication-set.entries[${index}]`;
  const kind = requireString(entry?.kind, `${label}.kind`);
  requireValue(entry.immutable === true, `${label}.immutable must be true`);
  const formRef = formRefIdentity(entry.formRef, `${label}.formRef`);
  requireValue(entry.formRef.kind === kind, `${label}.kind must match formRef.kind`);
  const packageDigest = requireDigest(
    entry.packageDigest,
    `${label}.packageDigest`,
  );
  const releaseID = requireString(entry.releaseId, `${label}.releaseId`);
  const releaseTag = requireString(entry.tag, `${label}.tag`);
  const releaseCommit = requireCommit(
    entry.peeledCommit,
    `${label}.peeledCommit`,
  );
  const releaseToolingCommit = requireCommit(
    entry.toolingCommit,
    `${label}.toolingCommit`,
  );
  const packageVersion = requireString(entry.version, `${label}.version`);
  requireValue(
    SEMVER.test(packageVersion),
    `${label}.version must be exact SemVer`,
  );
  requireValue(
    releaseTag === `forms/${releaseID}/v${packageVersion}`,
    `${label}.tag must match releaseId and package version`,
  );
  return {
    formRef,
    kind,
    packageDigest,
    releaseCommit,
    releaseTag,
    releaseToolingCommit,
  };
}

function admissionIdentity(entry, index) {
  const label = `standard-admission-set.entries[${index}]`;
  const kind = requireString(entry?.kind, `${label}.kind`);
  requireValue(
    entry.admissionStatus === "portable-standard",
    `${label}.admissionStatus must be portable-standard`,
  );
  const formRef = formRefIdentity(entry.formRef, `${label}.formRef`);
  requireValue(entry.formRef.kind === kind, `${label}.kind must match formRef.kind`);
  return {
    formRef,
    kind,
    packageDigest: requireDigest(
      entry.packageDigest,
      `${label}.packageDigest`,
    ),
    releaseCommit: requireCommit(
      entry.releaseCommit,
      `${label}.releaseCommit`,
    ),
    releaseTag: requireString(entry.releaseTag, `${label}.releaseTag`),
    releaseToolingCommit: requireCommit(
      entry.releaseToolingCommit,
      `${label}.releaseToolingCommit`,
    ),
  };
}

function comparePublishedIdentity(admitted, published, label) {
  for (const field of [
    "formRef",
    "packageDigest",
    "releaseCommit",
    "releaseTag",
    "releaseToolingCommit",
  ]) {
    requireValue(
      admitted[field] === published[field],
      `${label}.${field} does not match the published package identity`,
    );
  }
}

function validateProviderReadback(providerReadback, releaseVersion) {
  requireValue(
    providerReadback?.format === "takoform.provider-registry-readback@v1",
    "provider Registry readback format must be takoform.provider-registry-readback@v1",
  );
  requireValue(
    providerReadback.publicationReady === true,
    "provider Registry readback publicationReady must be true",
  );
  requireValue(
    typeof providerReadback.providerVersion === "string" &&
      SEMVER.test(providerReadback.providerVersion),
    "provider Registry readback version must be exact SemVer",
  );
  requireValue(
    providerReadback.providerReleaseTag ===
      `v${providerReadback.providerVersion}`,
    "provider Registry readback tag must match its retained version",
  );
  requireValue(
    providerReadback.providerAddress === releaseVersion.providerAddress,
    "provider Registry readback address must match release/version.json",
  );

  const actualInstalls = requireArray(
    providerReadback.installs,
    "provider Registry readback installs",
  ).map((entry, index) => {
    const label = `provider Registry readback installs[${index}]`;
    requireValue(
      entry?.providerVersion === providerReadback.providerVersion,
      `${label}.providerVersion must match the retained readback version`,
    );
    requireValue(
      entry?.providerAddress === providerReadback.providerAddress,
      `${label}.providerAddress must match the retained readback address`,
    );
    requireDigest(entry?.providerBinarySha256, `${label}.providerBinarySha256`);
    requireDigest(entry?.providerSchemaSha256, `${label}.providerSchemaSha256`);
    return JSON.stringify({
      product: requireString(entry?.product, `${label}.product`),
      providerAddress: entry.providerAddress,
      version: requireString(entry?.cliVersion, `${label}.cliVersion`),
    });
  });
  requireValue(
    actualInstalls.length === 2,
    "provider Registry readback must contain exactly two CLI installs",
  );
  requireUnique(actualInstalls, "provider Registry readback installs");
}

export function derivePublicationTruth({
  admissionSet,
  checkpoint,
  providerIdentities,
  providerReadback,
  publicationSet,
  releaseVersion,
}) {
  requireValue(
    publicationSet?.format === "takoform.form-package-publication-set@v1",
    "Form Package publication set format must be takoform.form-package-publication-set@v1",
  );
  requireValue(
    publicationSet.publicationStatus === "published-immutable",
    "Form Package publication set status must be published-immutable",
  );
  requireValue(
    publicationSet.admissionStatus === "external-required",
    "Form Package publication set admissionStatus must remain external-required",
  );
  const publicationGeneration = requireString(
    publicationSet.generation,
    "Form Package publication set generation",
  );
  requireValue(
    checkpoint?.format === "takoform.standard-admission-checkpoint@v1",
    "admission checkpoint format must be takoform.standard-admission-checkpoint@v1",
  );
  requireValue(
    typeof checkpoint.version === "string" && SEMVER.test(checkpoint.version),
    "admission checkpoint version must be exact SemVer",
  );
  requireValue(
    checkpoint.tag === `forms/admissions/v${checkpoint.version}`,
    "admission checkpoint tag must match its version",
  );
  requireValue(
    checkpoint.retainedRoot === "admission/v4",
    "admission checkpoint retainedRoot must be admission/v4",
  );
  requireValue(
    admissionSet?.format === "takoform.standard-admission-set@v3",
    "standard admission set format must be takoform.standard-admission-set@v3",
  );
  requireValue(
    admissionSet.admissionReleaseTag === checkpoint.tag,
    "standard admission set tag must match admission/v4/version.json",
  );
  requireValue(
    admissionSet.generation === checkpoint.generation,
    "standard admission set generation must match admission/v4/version.json",
  );
  requireValue(
    releaseVersion?.publicationStatus === "candidate-only",
    "release/version.json publicationStatus must remain candidate-only descriptor metadata",
  );
  requireValue(
    typeof releaseVersion.version === "string" &&
      SEMVER.test(releaseVersion.version),
    "release/version.json version must be exact SemVer",
  );
  requireValue(
    releaseVersion.tag === `v${releaseVersion.version}`,
    "release/version.json tag must match its version",
  );
  requireString(
    releaseVersion.providerAddress,
    "release/version.json.providerAddress",
  );
  const apiVersion = requireString(
    releaseVersion.versioning?.portableApiVersion,
    "release/version.json.versioning.portableApiVersion",
  );

  const published = requireArray(
    publicationSet.entries,
    "form-package-publication-set.entries",
  ).map(publicationIdentity);
  requireValue(published.length > 0, "Form Package publication set must not be empty");
  requireUnique(
    published.map(({ kind }) => kind),
    "Form Package publication kinds",
  );
  const publishedByKind = new Map(
    published.map((entry) => [entry.kind, entry]),
  );

  const admitted = requireArray(
    admissionSet.entries,
    "standard-admission-set.entries",
  ).map(admissionIdentity);
  requireValue(admitted.length > 0, "standard admission set must not be empty");
  requireUnique(
    admitted.map(({ kind }) => kind),
    "standard admission kinds",
  );
  for (const entry of admitted) {
    const publishedEntry = publishedByKind.get(entry.kind);
    requireValue(
      publishedEntry !== undefined,
      `standard admission kind ${entry.kind} is not in the published package set`,
    );
    comparePublishedIdentity(
      entry,
      publishedEntry,
      `standard admission kind ${entry.kind}`,
    );
  }

  const providerReports = requireArray(
    admissionSet.providerReportClosure?.reports,
    "standard-admission-set.providerReportClosure.reports",
  ).map((entry, index) => {
    const label = `standard-admission-set.providerReportClosure.reports[${index}]`;
    const kind = requireString(entry?.kind, `${label}.kind`);
    const identity = entry?.identity;
    return {
      formRef: formRefIdentity(identity?.formRef, `${label}.identity.formRef`),
      kind,
      packageDigest: requireDigest(
        identity?.packageDigest,
        `${label}.identity.packageDigest`,
      ),
    };
  });
  requireValue(
    admissionSet.providerReportClosure?.generation === publicationGeneration,
    "provider report closure generation must match the Form Package publication set",
  );
  requireUnique(
    providerReports.map(({ kind }) => kind),
    "provider report closure kinds",
  );
  requireValue(
    JSON.stringify(providerReports.map(({ kind }) => kind).sort()) ===
      JSON.stringify(published.map(({ kind }) => kind).sort()),
    "provider report closure must cover every published Form Package kind",
  );
  for (const report of providerReports) {
    const publishedEntry = publishedByKind.get(report.kind);
    requireValue(
      report.formRef === publishedEntry.formRef &&
        report.packageDigest === publishedEntry.packageDigest,
      `provider report closure kind ${report.kind} does not match the published package identity`,
    );
  }

  validateProviderReadback(providerReadback, releaseVersion);
  requireValue(
    providerIdentities?.format === "takoform.provider-release-identities@v1" &&
      Array.isArray(providerIdentities.entries) &&
      providerIdentities.entries.length > 0,
    "provider release identity ledger must contain assigned immutable releases",
  );
  const assignedProviders = providerIdentities.entries.map((entry, index) => {
    const label = `provider release identity ledger entries[${index}]`;
    const version = requireString(entry?.version, `${label}.version`);
    requireValue(SEMVER.test(version), `${label}.version must be exact SemVer`);
    requireValue(entry?.tag === `v${version}`, `${label}.tag must match version`);
    requireValue(entry?.status === "assigned", `${label}.status must be assigned`);
    requireCommit(entry?.tagObject, `${label}.tagObject`);
    requireCommit(entry?.commit, `${label}.commit`);
    requireValue(
      /^[0-9A-F]{40}$/u.test(entry?.signingFingerprint),
      `${label}.signingFingerprint must be uppercase 40-hex`,
    );
    return { version, parts: version.split(".").map(Number) };
  });
  requireUnique(
    assignedProviders.map(({ version }) => version),
    "provider release identity ledger versions",
  );
  assignedProviders.sort((left, right) =>
    right.parts[0] - left.parts[0] ||
    right.parts[1] - left.parts[1] ||
    right.parts[2] - left.parts[2],
  );
  const publishedProviderVersion = assignedProviders[0].version;

  return {
    admissionTag: checkpoint.tag,
    admittedCount: admitted.length,
    admittedKinds: admitted.map(({ kind }) => kind),
    apiVersion,
    providerAddress: releaseVersion.providerAddress,
    providerVersion: publishedProviderVersion,
    publishedCount: published.length,
    publishedKinds: published.map(({ kind }) => kind),
    remainingCount: published.length - admitted.length,
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
  return sentences.some((sentence) =>
    patterns.every((pattern) => pattern.test(sentence)),
  );
}

export function validatePublicationClaimText(text, truth, label = "document") {
  const sentences = normalizedSentences(text);
  const count = (value) => new RegExp(`(^|\\D)${value}(\\D|$)`, "u");
  const packageVocabulary =
    /\b(?:Form Packages?|packages?|package identities)\b/iu;

  requireValue(
    sentenceMatches(sentences, [
      count(truth.publishedCount),
      packageVocabulary,
      /\b(?:published|immutable)\b/iu,
    ]),
    `${label}: published count is not bound to the Form Package publication claim`,
  );
  requireValue(
    sentenceMatches(sentences, [
      count(truth.admittedCount),
      /\bexact(?:ly)?\b/iu,
      /\b(?:admits?|admitted|admission)\b/iu,
      /\bportable-standard\b/iu,
    ]),
    `${label}: admitted count is not bound to the portable-standard admission claim`,
  );
  requireValue(
    sentenceMatches(sentences, [
      new RegExp(`\\b(?:remaining|other)\\s+${truth.remainingCount}\\b`, "iu"),
      /\bpublished\b/iu,
      /\bnot\s+admitted\b/iu,
    ]),
    `${label}: remaining count is not bound to the published-but-not-admitted claim`,
  );

  for (const sentence of sentences) {
    if (
      count(truth.publishedCount).test(sentence) &&
      /\b(?:unpublished|not\s+(?:yet\s+)?published)\b/iu.test(sentence)
    ) {
      throw new Error(`${label}: contradicts the all-Form publication claim`);
    }
    if (
      new RegExp(
        `\\b(?:remaining|other)\\s+${truth.remainingCount}\\b`,
        "iu",
      ).test(sentence) &&
      /\badmitted\b/iu.test(sentence) &&
      !/\bnot\s+admitted\b/iu.test(sentence)
    ) {
      throw new Error(`${label}: contradicts the unadmitted remainder claim`);
    }
    if (
      count(truth.admittedCount).test(sentence) &&
      /\b(?:only|exactly)\b/iu.test(sentence) &&
      /\bpublished\b/iu.test(sentence) &&
      !/\b(?:admitted|admission|portable-standard)\b/iu.test(sentence)
    ) {
      throw new Error(
        `${label}: confuses the admitted count with the publication count`,
      );
    }
  }
  return true;
}

function readRegularFile(filePath, label) {
  const stats = lstatSync(filePath);
  requireValue(
    stats.isFile() && !stats.isSymbolicLink(),
    `${label} must be a regular non-symlink file`,
  );
  return readFileSync(filePath);
}

function parseJson(raw, label) {
  try {
    return JSON.parse(raw.toString("utf8"));
  } catch (error) {
    throw new Error(`${label} is invalid JSON: ${error.message}`);
  }
}

export function loadPublicationTruth(repositoryRoot) {
  const admissionRoot = path.join(repositoryRoot, "admission", "v4");
  const publicationSet = parseJson(
    readRegularFile(
      path.join(admissionRoot, "form-package-publication-set.json"),
      "Form Package publication set",
    ),
    "Form Package publication set",
  );
  const checkpoint = parseJson(
    readRegularFile(
      path.join(admissionRoot, "version.json"),
      "admission checkpoint",
    ),
    "admission checkpoint",
  );
  const admissionSet = parseJson(
    readRegularFile(
      path.join(admissionRoot, "standard-admission-set.json"),
      "standard admission set",
    ),
    "standard admission set",
  );
  const releaseVersion = parseJson(
    readRegularFile(
      path.join(repositoryRoot, "release", "version.json"),
      "provider release descriptor",
    ),
    "provider release descriptor",
  );
  const providerIdentities = parseJson(
    readRegularFile(
      path.join(repositoryRoot, "release", "provider-release-identities.json"),
      "provider release identity ledger",
    ),
    "provider release identity ledger",
  );

  const readbackRef = admissionSet.providerRegistryReadback;
  const readbackRelative = requireString(
    readbackRef?.path,
    "standard-admission-set.providerRegistryReadback.path",
  );
  const bundleRelative = requireString(
    readbackRef?.sigstoreBundle,
    "standard-admission-set.providerRegistryReadback.sigstoreBundle",
  );
  requireDigest(
    readbackRef?.digest,
    "standard-admission-set.providerRegistryReadback.digest",
  );
  requireValue(
    !path.posix.isAbsolute(readbackRelative) &&
      path.posix.normalize(readbackRelative) === readbackRelative &&
      !readbackRelative.split("/").includes(".."),
    "provider Registry readback path must be normalized and relative",
  );
  requireValue(
    !path.posix.isAbsolute(bundleRelative) &&
      path.posix.normalize(bundleRelative) === bundleRelative &&
      !bundleRelative.split("/").includes(".."),
    "provider Registry readback Sigstore bundle path must be normalized and relative",
  );
  const canonicalAdmissionRoot = realpathSync(admissionRoot);
  const readbackPath = path.resolve(canonicalAdmissionRoot, readbackRelative);
  const bundlePath = path.resolve(canonicalAdmissionRoot, bundleRelative);
  const canonicalReadbackPath = realpathSync(readbackPath);
  const canonicalBundlePath = realpathSync(bundlePath);
  requireValue(
    canonicalReadbackPath.startsWith(`${canonicalAdmissionRoot}${path.sep}`),
    "provider Registry readback path escapes admission/v4",
  );
  requireValue(
    canonicalBundlePath.startsWith(`${canonicalAdmissionRoot}${path.sep}`),
    "provider Registry readback Sigstore bundle path escapes admission/v4",
  );
  const readbackRaw = readRegularFile(
    readbackPath,
    "provider Registry readback",
  );
  readRegularFile(
    bundlePath,
    "provider Registry readback Sigstore bundle",
  );
  const readbackDigest = `sha256:${createHash("sha256").update(readbackRaw).digest("hex")}`;
  requireValue(
    readbackDigest === readbackRef.digest,
    "provider Registry readback digest does not match standard-admission-set.json",
  );
  const providerReadback = parseJson(
    readbackRaw,
    "provider Registry readback",
  );

  return derivePublicationTruth({
    admissionSet,
    checkpoint,
    providerIdentities,
    providerReadback,
    publicationSet,
    releaseVersion,
  });
}

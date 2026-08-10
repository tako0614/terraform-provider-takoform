import { createHash } from "node:crypto";
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

export function derivePublicationTruth({ publicationSet, providerIdentities, providerReadback, releaseVersion }) {
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
  const readbackProviderVersion = requireString(
    providerReadback?.version,
    "retained Registry-readback provider version",
  );
  const publishedProvider = assigned.find(
    (entry) => entry?.version === readbackProviderVersion,
  );
  requireValue(
    publishedProvider !== undefined,
    "retained Registry readback has no assigned immutable release",
  );
  const providerVersion = requireString(publishedProvider.version, "published provider version");
  requireValue(SEMVER.test(providerVersion), "published provider version must be exact SemVer");
  requireValue(publishedProvider.tag === `v${providerVersion}`, "published provider tag must match version");
  requireValue(
    providerReadback?.version === providerVersion &&
      providerReadback.tag === publishedProvider.tag &&
      providerReadback.commit === publishedProvider.commit &&
      providerReadback.providerAddress === releaseVersion.providerAddress &&
      providerReadback.publicationReady === true &&
      providerReadback.certificateIdentity ===
        "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/provider-registry-readback.yml@refs/heads/main" &&
      Array.isArray(providerReadback.products) &&
      JSON.stringify(providerReadback.products) === JSON.stringify(["OpenTofu", "Terraform"]) &&
      SHA256.test(providerReadback.providerBinaryDigest),
    "current provider assignment lacks exact retained signed Registry readback evidence",
  );
  const legacyProvider = assigned.filter((entry) => entry?.version !== providerVersion).at(-1);
  requireValue(legacyProvider !== undefined, "provider release identity ledger has no Legacy provider release");
  const legacyProviderVersion = requireString(legacyProvider.version, "Legacy provider version");
  requireValue(SEMVER.test(legacyProviderVersion), "Legacy provider version must be exact SemVer");
  requireValue(legacyProvider.tag === `v${legacyProviderVersion}`, "Legacy provider tag must match version");

  return {
    apiVersion: [...apiVersions][0],
    providerAddress: requireString(releaseVersion.providerAddress, "release/version.json.providerAddress"),
    providerVersion,
    legacyProviderVersion,
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

function digestBytes(raw) {
  return `sha256:${createHash("sha256").update(raw).digest("hex")}`;
}

function readProviderReadbackEvidence(providerIdentities, version) {
  const expectedFiles = [
    "SHA256SUMS",
    "provider-lifecycle-matrix.json",
    "provider-readback.json",
    "provider-readback.sigstore.json",
    "provider-registry-readback-manifest.json",
    "signed-provider-registry-readback-candidate.json",
  ];
  const assignment = providerIdentities.entries.find((entry) => entry?.version === version);
  const evidence = assignment?.registryReadback;
  requireValue(
    evidence?.format === "takoform.retained-provider-registry-readback@v1" &&
      typeof evidence.workflowRunId === "string" && /^[1-9][0-9]*$/u.test(evidence.workflowRunId) &&
      Number.isSafeInteger(evidence.workflowRunAttempt) && evidence.workflowRunAttempt > 0 &&
      /^[0-9a-f]{40}$/u.test(evidence.sourceCommit) &&
      evidence.files !== null && typeof evidence.files === "object" && !Array.isArray(evidence.files) &&
      JSON.stringify(Object.keys(evidence.files).sort()) === JSON.stringify(expectedFiles),
    `provider Registry readback ${version} retained envelope is invalid`,
  );
  const raws = Object.fromEntries(
    expectedFiles.map((name) => {
      const encoded = evidence.files[name];
      requireValue(typeof encoded === "string" && encoded !== "", `provider Registry readback ${version} ${name} is empty`);
      const raw = Buffer.from(encoded, "base64");
      requireValue(
        raw.length > 0 && raw.toString("base64") === encoded,
        `provider Registry readback ${version} ${name} is not canonical base64`,
      );
      return [name, raw];
    }),
  );
  const checksumNames = expectedFiles.filter((name) => name !== "SHA256SUMS").sort();
  const expectedChecksums = `${checksumNames
    .map((name) => `${digestBytes(raws[name]).slice(7)}  ${name}`)
    .join("\n")}\n`;
  requireValue(raws.SHA256SUMS.equals(Buffer.from(expectedChecksums)), `provider Registry readback ${version} checksum closure drift`);
  const parse = (name) => JSON.parse(raws[name].toString("utf8"));
  const matrix = parse("provider-lifecycle-matrix.json");
  const readback = parse("provider-readback.json");
  const bundle = parse("provider-readback.sigstore.json");
  const manifest = parse("provider-registry-readback-manifest.json");
  const signed = parse("signed-provider-registry-readback-candidate.json");
  const readbackDigest = digestBytes(raws["provider-readback.json"]);
  const matrixDigest = digestBytes(raws["provider-lifecycle-matrix.json"]);
  const bundleDigest = digestBytes(raws["provider-readback.sigstore.json"]);
  const manifestDigest = digestBytes(raws["provider-registry-readback-manifest.json"]);
  const certificateIdentity =
    "https://github.com/tako0614/terraform-provider-takoform/.github/workflows/provider-registry-readback.yml@refs/heads/main";
  requireValue(
    matrix.installationSource === "direct-registry-install" &&
      manifest.format === "takoform.provider-registry-readback-candidate@v1" &&
      manifest.provider?.version === version &&
      manifest.provider?.tag === `v${version}` &&
      manifest.matrix?.path === "provider-lifecycle-matrix.json" &&
      manifest.matrix?.digest === matrixDigest &&
      manifest.readback?.path === "provider-readback.json" &&
      manifest.readback?.digest === readbackDigest &&
      manifest.readback?.bundlePath === "provider-readback.sigstore.json" &&
      readback.format === "takoform.provider-registry-readback@v1" &&
      readback.publicationReady === true &&
      readback.providerVersion === version &&
      readback.providerReleaseTag === `v${version}` &&
      readback.providerReleaseCommit === manifest.provider.commit &&
      readback.providerAddress === manifest.provider.address &&
      readback.lifecycleMatrixPath === manifest.matrix.path &&
      readback.lifecycleMatrixDigest === matrixDigest &&
      signed.format === "takoform.provider-registry-readback-signed-candidate@v1" &&
      signed.certificateIdentity === certificateIdentity &&
      signed.workflow === ".github/workflows/provider-registry-readback.yml" &&
      signed.manifest?.path === "provider-registry-readback-manifest.json" &&
      signed.manifest?.digest === manifestDigest &&
      signed.readback?.path === "provider-readback.json" &&
      signed.readback?.digest === readbackDigest &&
      signed.readback?.bundlePath === "provider-readback.sigstore.json" &&
      signed.readback?.bundleDigest === bundleDigest &&
      bundle.mediaType === "application/vnd.dev.sigstore.bundle.v0.3+json" &&
      typeof bundle.verificationMaterial?.certificate?.rawBytes === "string" &&
      bundle.verificationMaterial.certificate.rawBytes.length > 0 &&
      Array.isArray(bundle.verificationMaterial?.tlogEntries) &&
      bundle.verificationMaterial.tlogEntries.length > 0 &&
      bundle.messageSignature?.messageDigest?.algorithm === "SHA2_256" &&
      bundle.messageSignature.messageDigest.digest ===
        createHash("sha256").update(raws["provider-readback.json"]).digest("base64"),
    `provider Registry readback ${version} identity or signature binding drift`,
  );
  const installs = readback.installs;
  requireValue(Array.isArray(installs) && installs.length === 2, `provider Registry readback ${version} must bind two installs`);
  const products = installs.map((install) => install.product).sort();
  const binaryDigests = new Set(installs.map((install) => install.providerBinarySha256));
  requireValue(
    JSON.stringify(products) === JSON.stringify(["OpenTofu", "Terraform"]) &&
      installs.every((install) =>
        install.providerVersion === version &&
        install.providerAddress === manifest.provider.address &&
        SHA256.test(install.providerBinarySha256)) &&
      binaryDigests.size === 1,
    `provider Registry readback ${version} install identity drift`,
  );
  return {
    version,
    tag: manifest.provider.tag,
    commit: manifest.provider.commit,
    providerAddress: manifest.provider.address,
    publicationReady: readback.publicationReady,
    certificateIdentity,
    products,
    providerBinaryDigest: installs[0].providerBinarySha256,
  };
}

export function loadPublicationTruth(repositoryRoot) {
  const releaseVersion = readRegularJson(
    path.join(repositoryRoot, "release", "version.json"),
    "provider release descriptor",
  );
  const providerIdentities = readRegularJson(
    path.join(repositoryRoot, "release", "provider-release-identities.json"),
    "provider release identity ledger",
  );
  const publishedAssignment = Array.isArray(providerIdentities.entries)
    ? providerIdentities.entries.filter((entry) => entry?.registryReadback).at(-1)
    : undefined;
  requireValue(
    typeof publishedAssignment?.version === "string",
    "provider release identity ledger has no retained Registry readback",
  );
  return derivePublicationTruth({
    publicationSet: readRegularJson(
      path.join(repositoryRoot, "admission", "v4", "form-package-publication-set.json"),
      "retained Form Package publication set",
    ),
    releaseVersion,
    providerIdentities,
    providerReadback: readProviderReadbackEvidence(
      providerIdentities,
      publishedAssignment.version,
    ),
  });
}

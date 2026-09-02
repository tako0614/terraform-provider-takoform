#!/usr/bin/env bun

// Generate and validate the Specification-wide compatibility inventory.
//
// This file owns an informational W09 report. It is deliberately not a
// numbered-release receipt or prerequisite: all occupied identities are read
// from their owning ledgers and the report only records the resulting view.

import { createHash } from "node:crypto";
import {
  existsSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
export const MANIFEST_PATH = "release/specification-compatibility.json";
export const MIRROR_MANIFEST_PATH =
  "website/static/release/specification-compatibility.json";
export const MANIFEST_FORMAT = "takoform.specification-compatibility@v1";
export const SPECIFICATION_VERSION = "1.1";
export const HOST_API_V1 = "forms.takoform.com/v1";
export const CANONICAL_ORIGIN =
  "https://github.com/tako0614/terraform-provider-takoform.git";
export const CLASS_IDS = Object.freeze([
  "form-package",
  "host-api-lifecycle",
  "family-host-support",
  "interface-binding-artifact-service",
  "trust-revocation-lifecycle-version-release",
]);
export const STATUS_VALUES = Object.freeze([
  "retained",
  "new-independent",
  "unpublished-candidate",
  "withdrawn-retained",
]);

const PUBLIC_DOCUMENT_LEDGER = "release/published-document-lanes.json";
const PUBLIC_SCHEMA_LEDGER = "release/public-schema-identities.json";
const PROVIDER_RELEASE_LEDGER = "release/provider-release-identities.json";
const PROVIDER_FORM_LEDGER = "release/provider-form-identities.json";
const PROVIDER_DESCRIPTOR = "release/version.json";
const SPECIFICATION_RELEASE_LEDGER = "release/specification-releases.json";
const FAMILY_INDEX = "forms/candidates/current-family-index.json";

const SHA256_ID = /^sha256:[0-9a-f]{64}$/u;
const FORM_GROUP = /^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.forms\.takoform\.com(?:\/v[0-9]+(?:(?:alpha|beta)[0-9]+)?)?$/u;
const FORM_VERSION = /^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$/u;

const CLASS_TITLES = Object.freeze({
  "form-package": "Form and Package",
  "host-api-lifecycle": "Host API lifecycle",
  "family-host-support": "Form Family and Host Support",
  "interface-binding-artifact-service":
    "Interface, Binding, artifact, and standard service",
  "trust-revocation-lifecycle-version-release":
    "Trust, revocation, lifecycle, version, and release identities",
});

const STATUS_PRIORITY = Object.freeze({
  "withdrawn-retained": 1,
  retained: 2,
  "new-independent": 3,
  "unpublished-candidate": 4,
});

function fail(message) {
  throw new Error(`specification compatibility: ${message}`);
}

function isRecord(value) {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function canonical(value) {
  if (value === null || typeof value === "string" || typeof value === "boolean") {
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isFinite(value)) fail("manifest contains a non-finite number");
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) return `[${value.map(canonical).join(",")}]`;
  if (!isRecord(value)) fail("manifest contains an unsupported value");
  return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${canonical(value[key])}`).join(",")}}`;
}

export function canonicalJson(value) {
  return canonical(value);
}

function digestBytes(bytes) {
  return `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
}

function digestFile(root, relativePath) {
  const absolute = path.join(root, relativePath);
  if (!existsSync(absolute)) fail(`source path ${relativePath} does not exist`);
  return digestBytes(readFileSync(absolute));
}

function isSafeRelativePath(relativePath) {
  return (
    typeof relativePath === "string" &&
    relativePath !== "" &&
    !relativePath.startsWith("/") &&
    !relativePath.includes("\\") &&
    path.posix.normalize(relativePath) === relativePath &&
    !relativePath.split("/").includes("..")
  );
}

function source(root, relativePath) {
  if (!isSafeRelativePath(relativePath)) {
    fail(`source path ${relativePath} is not a normalized repository path`);
  }
  return { path: relativePath, sha256: digestFile(root, relativePath) };
}

function existingSources(root, paths) {
  const unique = [...new Set(paths.filter((value) => isSafeRelativePath(value)))];
  return unique.filter((relativePath) => existsSync(path.join(root, relativePath)));
}

function readJson(root, relativePath) {
  if (!isSafeRelativePath(relativePath) || !existsSync(path.join(root, relativePath))) {
    fail(`${relativePath} does not exist`);
  }
  try {
    return JSON.parse(readFileSync(path.join(root, relativePath), "utf8"));
  } catch (error) {
    fail(`${relativePath} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function migration(status, details = {}) {
  if (!STATUS_VALUES.includes(status)) fail(`unknown status ${status}`);
  return {
    kind: status,
    from: null,
    to: null,
    ...details,
  };
}

function entry(root, {
  identity,
  status,
  sourcePaths,
  owningLedger,
  migrationDetails,
  formRef,
  packageDigest,
  publication,
}) {
  if (typeof identity !== "string" || identity === "") fail("identity is required");
  if (!STATUS_VALUES.includes(status)) fail(`${identity}: unknown status ${status}`);
  if (!Array.isArray(sourcePaths) || sourcePaths.length === 0) {
    fail(`${identity}: source paths must be non-empty`);
  }
  if (typeof owningLedger !== "string" || owningLedger === "") {
    fail(`${identity}: owningLedger is required`);
  }
  const paths = existingSources(root, [...sourcePaths, owningLedger]);
  if (paths.length === 0) fail(`${identity}: no source bytes are available`);
  const rendered = {
    identity,
    status,
    sources: paths.map((relativePath) => source(root, relativePath)),
    owningLedger,
    migration: migration(status, migrationDetails),
  };
  if (formRef !== undefined) rendered.formRef = formRef;
  if (packageDigest !== undefined) rendered.packageDigest = packageDigest;
  if (publication !== undefined) rendered.publication = publication;
  return rendered;
}

function formIdentity(formRef) {
  if (!isRecord(formRef)) fail("FormRef must be an object");
  const { apiVersion, kind, definitionVersion, schemaDigest } = formRef;
  if (
    typeof apiVersion !== "string" ||
    !FORM_GROUP.test(apiVersion) ||
    typeof kind !== "string" ||
    kind === "" ||
    typeof definitionVersion !== "string" ||
    !FORM_VERSION.test(definitionVersion) ||
    !SHA256_ID.test(schemaDigest)
  ) {
    fail("an exact FormRef is required");
  }
  return `${apiVersion}/${kind}@${definitionVersion}#${schemaDigest}`;
}

function addExpected(catalog, classId, specification) {
  if (!CLASS_IDS.includes(classId)) fail(`unknown class ${classId}`);
  if (!isRecord(specification)) fail(`${classId}: identity specification must be an object`);
  const {
    identity,
    status,
    sourcePaths = [],
    owningLedger,
    migrationDetails = {},
    formRef,
    packageDigest,
    publication,
  } = specification;
  if (typeof identity !== "string" || identity === "") fail(`${classId}: identity is required`);
  if (!STATUS_VALUES.includes(status)) fail(`${identity}: unknown status ${status}`);
  if (typeof owningLedger !== "string" || owningLedger === "") fail(`${identity}: owning ledger is required`);
  const classCatalog = catalog.get(classId);
  const current = classCatalog.get(identity);
  if (!current) {
    classCatalog.set(identity, {
      identity,
      status,
      sourcePaths: [...new Set([...sourcePaths, owningLedger])],
      owningLedger,
      migrationDetails,
      formRef,
      packageDigest,
      publication,
    });
    return;
  }
  current.sourcePaths = [...new Set([...current.sourcePaths, ...sourcePaths, owningLedger])].sort();
  if (STATUS_PRIORITY[status] > STATUS_PRIORITY[current.status]) current.status = status;
  if (current.formRef === undefined && formRef !== undefined) current.formRef = formRef;
  if (current.packageDigest === undefined && packageDigest !== undefined) current.packageDigest = packageDigest;
  if (current.publication === undefined && publication !== undefined) current.publication = publication;
}

function classForApiIdentity(identity) {
  if (typeof identity !== "string") return null;
  const apiIdentity = identity.replace(/#FormPackage$/u, "");
  if (/^forms\.takoform\.com\//u.test(apiIdentity) || /^operations\.takoform\.com\//u.test(apiIdentity)) return "host-api-lifecycle";
  if (/^packages\.forms\.takoform\.com\//u.test(apiIdentity)) return "form-package";
  if (/^trust\.forms\.takoform\.com\//u.test(apiIdentity)) return "trust-revocation-lifecycle-version-release";
  if (/^support\.takoform\.com\//u.test(apiIdentity)) return "family-host-support";
  if (/^(?:interfaces|bindings|artifacts|standards)\.takoform\.com\//u.test(apiIdentity)) return "interface-binding-artifact-service";
  if (FORM_GROUP.test(apiIdentity) && apiIdentity !== "forms.takoform.com") return "family-host-support";
  return null;
}

function reportApiIdentity(identity) {
  return /^packages\.forms\.takoform\.com\//u.test(identity) && !identity.endsWith("#FormPackage")
    ? `${identity}#FormPackage`
    : identity;
}

function schemaPathClass(identity) {
  let pathname;
  try {
    pathname = new URL(identity).pathname;
  } catch {
    return null;
  }
  if (/\/form-package-revocation(?:-checkpoint)?\.schema\.json$/u.test(pathname)) {
    return "trust-revocation-lifecycle-version-release";
  }
  if (/\/interfaces\//u.test(pathname) || /\/bindings\//u.test(pathname) || /\/artifacts\//u.test(pathname) || /\/standards\//u.test(pathname)) {
    return "interface-binding-artifact-service";
  }
  if (/\/support\//u.test(pathname)) return "family-host-support";
  if (/\/operations\//u.test(pathname) || /(?:host-api-wire|host-discovery)\.schema\.json$/u.test(pathname)) {
    return "host-api-lifecycle";
  }
  if (/(?:form-definition|form-ref|package-index)\.schema\.json$/u.test(pathname)) return "form-package";
  return null;
}

function inferSchemaApiIdentity(identity, record, root) {
  if (isSafeRelativePath(record.source) && existsSync(path.join(root, record.source))) {
    try {
      const schema = JSON.parse(readFileSync(path.join(root, record.source), "utf8"));
      const apiVersion = schema?.properties?.apiVersion?.const;
      if (typeof apiVersion === "string") return apiVersion;
    } catch {
      // A retained URL can still be accounted for when its historical bytes
      // are no longer checked out.
    }
  }
  let pathname;
  try {
    pathname = new URL(identity).pathname;
  } catch {
    return null;
  }
  const segments = pathname.split("/").filter(Boolean);
  const schemasIndex = segments.indexOf("schemas");
  if (schemasIndex < 0) return null;
  const section = segments[schemasIndex + 1];
  const version = segments[schemasIndex + 2];
  if (/^(?:interfaces|bindings|artifacts|standards|support)$/u.test(section) && version) {
    const group = section === "support" ? "support.takoform.com" : `${section}.takoform.com`;
    return `${group}/${version}`;
  }
  if (section === "operations" && version) return `operations.takoform.com/${version}`;
  if (/^v[0-9]+(?:alpha|beta)?[0-9]*$/u.test(section)) {
    if (/package-index\.schema\.json$/u.test(pathname)) return `packages.forms.takoform.com/${section}`;
    if (/(?:form-definition|form-ref|host-api-wire|host-discovery)\.schema\.json$/u.test(pathname)) return `forms.takoform.com/${section}`;
    if (/form-package-revocation(?:-checkpoint)?\.schema\.json$/u.test(pathname)) return "trust.forms.takoform.com/v1alpha1";
  }
  return null;
}

function hostApiSources(lane) {
  const supportProfile = lane === "v1"
    ? "v1"
    : lane === "v1beta1"
      ? "v1alpha1"
      : "v1alpha2";
  return [
    `spec/host-api/${lane}.md`,
    `spec/host-api/operations-${lane}.json`,
    `spec/schemas/host-api-wire-${lane}.schema.json`,
    `spec/schemas/host-discovery-${lane}.schema.json`,
    `spec/schemas/host-support-profile-${supportProfile}.schema.json`,
  ];
}

function addPublicSchemaIdentities(root, catalog, publicSchemaLedger) {
  const records = [
    ...(publicSchemaLedger.identities ?? []).map((record) => ({ ...record, status: "retained" })),
    ...(publicSchemaLedger.retired ?? []).map((record) => ({ ...record, status: "withdrawn-retained" })),
  ];
  for (const record of records) {
    if (typeof record.id !== "string" || typeof record.source !== "string") fail(`${PUBLIC_SCHEMA_LEDGER} contains an invalid identity`);
    const directClass = schemaPathClass(record.id);
    if (!directClass) fail(`${PUBLIC_SCHEMA_LEDGER} identity ${record.id} is not covered by the five-class report`);
    addExpected(catalog, directClass, {
      identity: record.id,
      status: record.status,
      sourcePaths: [PUBLIC_SCHEMA_LEDGER, record.source],
      owningLedger: PUBLIC_SCHEMA_LEDGER,
      migrationDetails: {
        from: record.status === "withdrawn-retained" ? record.id : null,
        reason: record.status === "withdrawn-retained"
          ? "retained public schema history; never reactivate or rewrite the URL"
          : "public schema identity remains byte-bound to its owning ledger",
      },
      publication: record.status,
    });
    const compact = inferSchemaApiIdentity(record.id, record, root);
    const compactIdentity = compact ? reportApiIdentity(compact) : compact;
    const compactClass = classForApiIdentity(compactIdentity);
    if (compactIdentity && compactClass) {
      addExpected(catalog, compactClass, {
        identity: compactIdentity,
        status: compactClass === "host-api-lifecycle" && compactIdentity === HOST_API_V1
          ? "unpublished-candidate"
          : record.status,
        sourcePaths: [PUBLIC_SCHEMA_LEDGER, record.source],
        owningLedger: PUBLIC_SCHEMA_LEDGER,
        migrationDetails: {
          from: record.status === "withdrawn-retained" ? compactIdentity : null,
          to: compactClass === "host-api-lifecycle" && compactIdentity === HOST_API_V1 ? compactIdentity : null,
          reason: record.status === "withdrawn-retained"
            ? "retained compact API identity; never reactivate or rewrite the lane"
            : "compact API identity is derived from the public schema ledger",
        },
        publication: compactClass === "host-api-lifecycle" && compactIdentity === HOST_API_V1
          ? "unpublished-candidate"
          : record.status,
      });
    }
  }
}

function addPublishedDocumentIdentities(catalog, documentLedger) {
  for (const section of ["documents", "retired"]) {
    const status = section === "retired" ? "withdrawn-retained" : "retained";
    for (const record of documentLedger[section] ?? []) {
      if (!isRecord(record) || typeof record.apiVersion !== "string" || typeof record.path !== "string") {
        fail(`${PUBLIC_DOCUMENT_LEDGER} ${section} contains an invalid record`);
      }
      const identity = reportApiIdentity(record.apiVersion);
      const classId = classForApiIdentity(identity);
      if (!classId) fail(`${PUBLIC_DOCUMENT_LEDGER} ${section} identity ${record.apiVersion} is not covered by the five-class report`);
      let effectiveStatus = status;
      if (record.apiVersion === HOST_API_V1) effectiveStatus = "unpublished-candidate";
      addExpected(catalog, classId, {
        identity,
        status: effectiveStatus,
        sourcePaths: [PUBLIC_DOCUMENT_LEDGER, record.path],
        owningLedger: PUBLIC_DOCUMENT_LEDGER,
        migrationDetails: {
          from: status === "withdrawn-retained" ? identity : null,
          to: effectiveStatus === "unpublished-candidate" ? identity : null,
          reason: status === "withdrawn-retained"
            ? "retained published-document lane; never reuse the identity"
            : "occupied API identity is read from the published-document ledger",
        },
        publication: effectiveStatus,
      });
    }
  }
}

function addCurrentFamilies(root, catalog, familyIndex) {
  for (const family of familyIndex.families ?? []) {
    if (!isRecord(family) || typeof family.group !== "string" || typeof family.candidateSet !== "string") {
      fail(`${FAMILY_INDEX} contains an invalid family record`);
    }
    addExpected(catalog, "family-host-support", {
      identity: family.group,
      status: "unpublished-candidate",
      sourcePaths: [FAMILY_INDEX, family.candidateSet, "spec/form-families.md"],
      owningLedger: FAMILY_INDEX,
      migrationDetails: {
        to: family.group,
        reason: "current versionless family roster is an unpublished candidate",
      },
      publication: "unpublished-candidate",
    });
    const candidate = readJson(root, family.candidateSet);
    if (candidate.family && candidate.family !== family.group) fail(`${family.candidateSet} family differs from ${family.group}`);
    for (const form of candidate.forms ?? []) addCurrentForm(root, catalog, family.candidateSet, form);
  }
}

function addCurrentForm(root, catalog, candidateSetPath, form) {
  if (!isRecord(form) || !isRecord(form.formRef)) fail(`${candidateSetPath} contains an invalid FormRef`);
  const identity = formIdentity(form.formRef);
  if (typeof form.packageDigest !== "string" || !SHA256_ID.test(form.packageDigest)) fail(`${identity} has an invalid package digest`);
  const formPath = typeof form.path === "string" ? form.path : null;
  const sourcePaths = [candidateSetPath, formPath && `${formPath}/definition.json`, formPath && `${formPath}/package-index.json`].filter(Boolean);
  addExpected(catalog, "form-package", {
    identity,
    status: "unpublished-candidate",
    sourcePaths,
    owningLedger: FAMILY_INDEX,
    migrationDetails: {
      to: identity,
      reason: "current Form and package bytes remain unpublished candidates",
    },
    formRef: form.formRef,
    packageDigest: form.packageDigest,
    publication: "unpublished-candidate",
  });
}

// The current Provider is whichever release the descriptor selects, not a
// literal major: its embedded Forms are the current unpublished-candidate
// projection, while every other release in the append-only ledger is retained
// history for a withdrawn or superseded Provider identity.
function addProviderFormIdentities(catalog, providerFormLedger, currentProviderVersion) {
  for (const release of providerFormLedger.releases ?? []) {
    if (!isRecord(release) || !Array.isArray(release.forms)) fail(`${PROVIDER_FORM_LEDGER} contains an invalid release`);
    for (const form of release.forms) {
      if (!isRecord(form) || !isRecord(form.formRef)) fail(`${PROVIDER_FORM_LEDGER} contains an invalid FormRef`);
      const identity = formIdentity(form.formRef);
      if (typeof form.packageDigest !== "string" || !SHA256_ID.test(form.packageDigest)) fail(`${identity} has an invalid package digest`);
      const kindSlug = String(form.formRef.kind)
        .replace(/([a-z0-9])([A-Z])/gu, "$1-$2")
        .replace(/([A-Z])([A-Z][a-z])/gu, "$1-$2")
        .toLowerCase()
        .replace(/^sq-lite-/u, "sqlite-");
      const group = form.formRef.apiVersion;
      const groupRoot = group.includes("/")
        ? `forms/candidates/${group.split("/", 1)[0]}/${group.slice(group.indexOf("/") + 1)}`
        : `forms/candidates/${group}`;
      const isCurrent = release.providerVersion === currentProviderVersion;
      addExpected(catalog, "form-package", {
        identity,
        status: isCurrent ? "unpublished-candidate" : "withdrawn-retained",
        sourcePaths: [PROVIDER_FORM_LEDGER, `${groupRoot}/${kindSlug}/definition.json`, `${groupRoot}/${kindSlug}/package-index.json`],
        owningLedger: PROVIDER_FORM_LEDGER,
        migrationDetails: {
          from: isCurrent ? null : form.formRef.apiVersion,
          reason: isCurrent
            ? "Provider projection does not publish or promote the current Form package"
            : `retained Provider ${release.providerVersion} Form history; never rewrite the identity`,
        },
        formRef: form.formRef,
        packageDigest: form.packageDigest,
        publication: isCurrent ? "unpublished-candidate" : "withdrawn-retained",
      });
    }
  }
}

function addCandidateContracts(root, catalog, familyIndex) {
  for (const [kind, descriptor] of [["interface", familyIndex.interfaceCandidateSet], ["binding", familyIndex.bindingCandidateSet]]) {
    if (!isRecord(descriptor) || typeof descriptor.path !== "string") fail(`${FAMILY_INDEX} ${kind} candidate set is missing`);
    const candidate = readJson(root, descriptor.path);
    const plural = kind === "interface" ? "interfaces" : "bindings";
    const apiGroup = kind === "interface" ? "interfaces.takoform.com" : "bindings.takoform.com";
    const values = Array.isArray(candidate[plural]) ? candidate[plural] : [];
    if (typeof candidate.format === "string") {
      addExpected(catalog, "interface-binding-artifact-service", {
        identity: candidate.format,
        status: "unpublished-candidate",
        sourcePaths: [FAMILY_INDEX, descriptor.path],
        owningLedger: FAMILY_INDEX,
        migrationDetails: { to: candidate.format, reason: `${kind} candidate format is not published` },
        publication: "unpublished-candidate",
      });
    }
    for (const value of values) {
      if (!isRecord(value) || typeof value.name !== "string" || typeof value.version !== "string" || !SHA256_ID.test(value.schemaDigest)) {
        fail(`${descriptor.path} contains an invalid ${kind} identity`);
      }
      const definitionPath = `${path.posix.dirname(descriptor.path)}/${value.name}/definition.json`;
      let definition;
      if (existsSync(path.join(root, definitionPath))) definition = readJson(root, definitionPath);
      const versionSegment = path.posix.basename(path.posix.dirname(descriptor.path));
      const apiVersion = typeof definition?.apiVersion === "string"
        ? definition.apiVersion
        : `${apiGroup}/${versionSegment}`;
      const identity = `${apiVersion}/${value.name}@${value.version}#${value.schemaDigest}`;
      addExpected(catalog, "interface-binding-artifact-service", {
        identity,
        status: "unpublished-candidate",
        sourcePaths: [FAMILY_INDEX, descriptor.path, definitionPath, `spec/${kind === "interface" ? "interface-contract" : "binding-contract"}/README.md`],
        owningLedger: FAMILY_INDEX,
        migrationDetails: { to: identity, reason: `${kind} definition is an unpublished candidate` },
        publication: "unpublished-candidate",
      });
    }
  }
}

function addProviderVersions(catalog, providerReleaseLedger) {
  for (const release of providerReleaseLedger.entries ?? []) {
    if (!isRecord(release) || typeof release.version !== "string" || release.version === "") fail(`${PROVIDER_RELEASE_LEDGER} contains an invalid Provider version`);
    const identity = `registry.terraform.io/tako0614/takoform@${release.version}`;
    addExpected(catalog, "trust-revocation-lifecycle-version-release", {
      identity,
      status: "retained",
      sourcePaths: [PROVIDER_RELEASE_LEDGER],
      owningLedger: PROVIDER_RELEASE_LEDGER,
      migrationDetails: { reason: "Provider release identity is retained independently of the Specification" },
      publication: "retained",
    });
  }
}

function addSpecificationIdentities(catalog, specificationReleaseLedger) {
  if (!isRecord(specificationReleaseLedger)) fail(`${SPECIFICATION_RELEASE_LEDGER} must be an object`);
  const releases = Array.isArray(specificationReleaseLedger.releases)
    ? specificationReleaseLedger.releases
    : [];
  const releasedVersions = new Set(
    releases
      .filter((release) => isRecord(release) && typeof release.version === "string")
      .map((release) => release.version),
  );
  for (const reserved of specificationReleaseLedger.reserved ?? []) {
    if (!isRecord(reserved) || typeof reserved.version !== "string" || typeof reserved.status !== "string") {
      fail(`${SPECIFICATION_RELEASE_LEDGER}.reserved contains an invalid identity`);
    }
    if (!STATUS_VALUES.includes(reserved.status)) fail(`${SPECIFICATION_RELEASE_LEDGER}: unknown status ${reserved.status}`);
    const identity = `takoform.specification@${reserved.version}`;
    addExpected(catalog, "trust-revocation-lifecycle-version-release", {
      identity,
      status: reserved.status,
      sourcePaths: [SPECIFICATION_RELEASE_LEDGER],
      owningLedger: SPECIFICATION_RELEASE_LEDGER,
      migrationDetails: {
        from: identity,
        reason: "reserved Specification identity is retained history and may never be reused",
      },
      publication: reserved.status,
    });
  }
  const candidate = specificationReleaseLedger.candidate;
  if (
    isRecord(candidate) &&
    typeof candidate.version === "string" &&
    !releasedVersions.has(candidate.version)
  ) {
    const identity = `takoform.specification@${candidate.version}`;
    addExpected(catalog, "trust-revocation-lifecycle-version-release", {
      identity,
      status: "new-independent",
      sourcePaths: [SPECIFICATION_RELEASE_LEDGER],
      owningLedger: SPECIFICATION_RELEASE_LEDGER,
      migrationDetails: {
        to: identity,
        reason: "numbered Specification identity is independent of Host, Form, package, and Provider axes",
      },
      publication: "unpublished-candidate",
    });
  }
  for (const release of releases) {
    if (!isRecord(release) || typeof release.version !== "string") {
      fail(`${SPECIFICATION_RELEASE_LEDGER}.releases contains an invalid identity`);
    }
    const identity = `takoform.specification@${release.version}`;
    addExpected(catalog, "trust-revocation-lifecycle-version-release", {
      identity,
      status: "retained",
      sourcePaths: [SPECIFICATION_RELEASE_LEDGER],
      owningLedger: SPECIFICATION_RELEASE_LEDGER,
      migrationDetails: {
        from: identity,
        reason: "published Specification identity is append-only retained history",
      },
      publication: "retained",
    });
  }
}

function addIntrinsicIdentities(catalog) {
  addExpected(catalog, "trust-revocation-lifecycle-version-release", {
    identity: "takoform.trust.profile@v1",
    status: "retained",
    sourcePaths: ["spec/trust/profile.json", "spec/trust/README.md", "release/trust/trusted-root.json"],
    owningLedger: "release/trust/trusted-root.json",
    migrationDetails: { reason: "issuer and source policy remains operator-selected" },
    publication: "retained",
  });
  addExpected(catalog, "trust-revocation-lifecycle-version-release", {
    identity: "takoform.form-package-revocation@v1",
    status: "retained",
    sourcePaths: ["spec/schemas/form-package-revocation.schema.json", "spec/schemas/form-package-revocation-checkpoint.schema.json", PUBLIC_SCHEMA_LEDGER],
    owningLedger: PUBLIC_SCHEMA_LEDGER,
    migrationDetails: { reason: "revocation is append-only and blocks mutation without erasing retained state" },
    publication: "retained",
  });
  addExpected(catalog, "trust-revocation-lifecycle-version-release", {
    identity: "takoform.form-lifecycle@v1",
    status: "retained",
    sourcePaths: ["spec/project-lifecycle.md", "spec/versioning.md", "spec/publication-freeze.md"],
    owningLedger: "spec/project-lifecycle.md",
    migrationDetails: { reason: "authored, verified, packaged, published, installed, supported, activated, provisioned, client-supported, and offered remain independent facts" },
    publication: "retained",
  });
}

function buildCatalog(root) {
  const documentLedger = readJson(root, PUBLIC_DOCUMENT_LEDGER);
  const publicSchemaLedger = readJson(root, PUBLIC_SCHEMA_LEDGER);
  const providerReleaseLedger = readJson(root, PROVIDER_RELEASE_LEDGER);
  const providerFormLedger = readJson(root, PROVIDER_FORM_LEDGER);
  const specificationReleaseLedger = readJson(root, SPECIFICATION_RELEASE_LEDGER);
  const familyIndex = readJson(root, FAMILY_INDEX);
  const providerDescriptor = readJson(root, PROVIDER_DESCRIPTOR);
  if (typeof providerDescriptor.version !== "string" || providerDescriptor.version === "") {
    fail(`${PROVIDER_DESCRIPTOR} has no current Provider version`);
  }
  const catalog = new Map(CLASS_IDS.map((classId) => [classId, new Map()]));

  addPublicSchemaIdentities(root, catalog, publicSchemaLedger);
  addPublishedDocumentIdentities(catalog, documentLedger);
  addCurrentFamilies(root, catalog, familyIndex);
  addProviderFormIdentities(catalog, providerFormLedger, providerDescriptor.version);
  addCandidateContracts(root, catalog, familyIndex);
  addProviderVersions(catalog, providerReleaseLedger);
  addSpecificationIdentities(catalog, specificationReleaseLedger);
  addIntrinsicIdentities(catalog);

  const requiredPackages = [1, 2, 3, 4, 5].map((version) => `packages.forms.takoform.com/v1alpha${version}#FormPackage`);
  const requiredHosts = ["forms.takoform.com/v1alpha1", "forms.takoform.com/v1alpha2", "forms.takoform.com/v1alpha3"];
  for (const identity of requiredPackages) if (!catalog.get("form-package").has(identity)) fail(`authoritative ledgers do not contain required package envelope ${identity}`);
  for (const identity of requiredHosts) if (!catalog.get("host-api-lifecycle").has(identity)) fail(`authoritative ledgers do not contain required Host lane ${identity}`);
  return catalog;
}

function renderCatalog(root, catalog) {
  return CLASS_IDS.map((classId) => ({
    id: classId,
    title: CLASS_TITLES[classId],
    entries: [...catalog.get(classId).values()]
      .sort((left, right) => left.identity.localeCompare(right.identity))
      .map((specification) => entry(root, specification)),
  }));
}

export function deriveExpectedIdentitySets(root = repositoryRoot) {
  const catalog = buildCatalog(root);
  return Object.fromEntries(CLASS_IDS.map((classId) => [classId, [...catalog.get(classId).keys()].sort()]));
}

export function generateManifest(root = repositoryRoot) {
  const catalog = buildCatalog(root);
  const hostApiV1Sources = hostApiSources("v1").filter((relativePath) => existsSync(path.join(root, relativePath)));
  if (hostApiV1Sources.length !== hostApiSources("v1").length) fail("literal Host API v1 source set is incomplete");
  return {
    format: MANIFEST_FORMAT,
    specificationVersion: SPECIFICATION_VERSION,
    repositoryAuthority: CANONICAL_ORIGIN,
    policy: "Informational/completion report only: generated from committed source and owning ledgers. It is never a numbered-release prerequisite, identity, evidence record, or publication asset; it does not publish a Form, package, Provider, or Host API lane.",
    statusVocabulary: [...STATUS_VALUES],
    classes: renderCatalog(root, catalog),
    hostApiV1Pin: {
      lane: HOST_API_V1,
      status: "unpublished-candidate",
      sources: hostApiV1Sources.map((relativePath) => source(root, relativePath)),
      owningLedger: "spec/host-api/v1.md",
      bytePin: "all listed bytes are exact raw-byte inputs; any change fails compatibility validation",
    },
  };
}

function validateSourceRecord(root, record, context) {
  if (!isRecord(record)) fail(`${context} must be an object`);
  const keys = Object.keys(record).sort();
  if (JSON.stringify(keys) !== JSON.stringify(["path", "sha256"])) fail(`${context} keys must be exactly path, sha256`);
  if (!isSafeRelativePath(record.path)) fail(`${context}.path must be a normalized repository-relative path`);
  if (!SHA256_ID.test(record.sha256)) fail(`${context}.sha256 must be sha256:<64 lowercase hex>`);
  const actual = digestFile(root, record.path);
  if (actual !== record.sha256) fail(`${context} digest does not match ${record.path}`);
}

function validateEntry(root, entryValue, context) {
  if (!isRecord(entryValue)) fail(`${context} must be an object`);
  for (const key of ["identity", "status", "sources", "owningLedger", "migration"]) {
    if (!Object.hasOwn(entryValue, key)) fail(`${context} is missing ${key}`);
  }
  if (typeof entryValue.identity !== "string" || entryValue.identity === "") fail(`${context}.identity is required`);
  if (!STATUS_VALUES.includes(entryValue.status)) fail(`${context}.status is not recognized`);
  if (!Array.isArray(entryValue.sources) || entryValue.sources.length === 0) fail(`${context}.sources must be non-empty`);
  entryValue.sources.forEach((record, index) => validateSourceRecord(root, record, `${context}.sources[${index}]`));
  if (typeof entryValue.owningLedger !== "string" || !isSafeRelativePath(entryValue.owningLedger)) fail(`${context}.owningLedger is required`);
  if (!existsSync(path.join(root, entryValue.owningLedger))) fail(`${context}.owningLedger does not exist`);
  if (!isRecord(entryValue.migration) || entryValue.migration.kind !== entryValue.status) fail(`${context}.migration must state the entry status`);
  if (entryValue.status === "unpublished-candidate" && entryValue.publication === "published") fail(`${context} candidate cannot claim publication`);
  if (entryValue.formRef !== undefined) {
    const ref = entryValue.formRef;
    if (!isRecord(ref) || typeof ref.apiVersion !== "string" || !FORM_GROUP.test(ref.apiVersion) || typeof ref.kind !== "string" || typeof ref.definitionVersion !== "string" || !FORM_VERSION.test(ref.definitionVersion) || !SHA256_ID.test(ref.schemaDigest)) fail(`${context}.formRef is not an exact FormRef`);
  }
  if (entryValue.packageDigest !== undefined && !SHA256_ID.test(entryValue.packageDigest)) fail(`${context}.packageDigest must be sha256:<64 lowercase hex>`);
}

function compareCatalogToManifest(manifest, root) {
  const catalog = buildCatalog(root);
  for (const classId of CLASS_IDS) {
    const expected = catalog.get(classId);
    const actualEntries = manifest.classes.find((value) => value.id === classId)?.entries ?? [];
    const actual = new Set(actualEntries.map((value) => value.identity));
    const missing = [...expected.keys()].filter((identity) => !actual.has(identity)).sort();
    const extra = [...actual].filter((identity) => !expected.has(identity)).sort();
    if (missing.length > 0) fail(`class ${classId} is missing ledger identities: ${missing.join(", ")}`);
    if (extra.length > 0) fail(`class ${classId} has identities not present in owning ledgers: ${extra.join(", ")}`);
    for (const specification of expected.values()) {
      const observed = actualEntries.find((value) => value.identity === specification.identity);
      if (observed.status !== specification.status) fail(`${specification.identity} status differs from owning ledgers (expected ${specification.status})`);
    }
  }
}

export function validateManifest(manifest, root = repositoryRoot) {
  if (!isRecord(manifest)) fail("manifest must be an object");
  const keys = Object.keys(manifest).sort();
  const expectedKeys = ["classes", "format", "hostApiV1Pin", "policy", "repositoryAuthority", "specificationVersion", "statusVocabulary"].sort();
  if (JSON.stringify(keys) !== JSON.stringify(expectedKeys)) fail(`manifest keys must be exactly ${expectedKeys.join(", ")}`);
  if (manifest.format !== MANIFEST_FORMAT) fail(`format must be ${MANIFEST_FORMAT}`);
  if (manifest.specificationVersion !== SPECIFICATION_VERSION) fail("specificationVersion must be 1.1");
  if (manifest.repositoryAuthority !== CANONICAL_ORIGIN) fail("repositoryAuthority must pin the current canonical repository");
  if (typeof manifest.policy !== "string" || !/informational\/completion report only/iu.test(manifest.policy) || !/never a numbered-release prerequisite, identity, evidence record, or publication asset/u.test(manifest.policy)) fail("policy must keep the report separate from numbered-release authority");
  if (JSON.stringify(manifest.statusVocabulary) !== JSON.stringify(STATUS_VALUES)) fail("statusVocabulary must be the exact four-state vocabulary");
  if (!Array.isArray(manifest.classes) || manifest.classes.length !== CLASS_IDS.length) fail(`classes must contain exactly ${CLASS_IDS.length} classes`);
  const seenClasses = new Set();
  const seenEntries = new Set();
  for (const [classIndex, classValue] of manifest.classes.entries()) {
    if (!isRecord(classValue) || classValue.id !== CLASS_IDS[classIndex] || typeof classValue.title !== "string" || !Array.isArray(classValue.entries)) fail(`classes[${classIndex}] must contain ${CLASS_IDS[classIndex]}, title, entries`);
    if (seenClasses.has(classValue.id)) fail(`class ${classValue.id} is duplicated`);
    seenClasses.add(classValue.id);
    if (classValue.entries.length === 0) fail(`class ${classValue.id} must not be empty`);
    classValue.entries.forEach((value, index) => {
      validateEntry(root, value, `classes[${classIndex}].entries[${index}]`);
      if (seenEntries.has(value.identity)) fail(`identity ${value.identity} is duplicated`);
      seenEntries.add(value.identity);
    });
  }
  if (!isRecord(manifest.hostApiV1Pin)) fail("hostApiV1Pin must be an object");
  if (manifest.hostApiV1Pin.lane !== HOST_API_V1 || manifest.hostApiV1Pin.status !== "unpublished-candidate" || manifest.hostApiV1Pin.owningLedger !== "spec/host-api/v1.md") fail("hostApiV1Pin must byte-pin the unpublished literal Host API v1");
  if (!Array.isArray(manifest.hostApiV1Pin.sources) || manifest.hostApiV1Pin.sources.length !== hostApiSources("v1").length) fail("hostApiV1Pin must list every Host API v1 source byte");
  manifest.hostApiV1Pin.sources.forEach((record, index) => validateSourceRecord(root, record, `hostApiV1Pin.sources[${index}]`));
  const expectedHostSources = new Set(hostApiSources("v1"));
  for (const record of manifest.hostApiV1Pin.sources) if (!expectedHostSources.has(record.path)) fail(`hostApiV1Pin contains unexpected source ${record.path}`);
  if (typeof manifest.hostApiV1Pin.bytePin !== "string" || manifest.hostApiV1Pin.bytePin.trim() === "") fail("hostApiV1Pin.bytePin must explain raw-byte pinning");
  compareCatalogToManifest(manifest, root);
  const serialized = JSON.stringify(manifest);
  if (serialized.includes("/v1.1") || serialized.includes("/v2")) fail("manifest must not mint a /v1.1 or /v2 published lane");
  if (serialized.includes("github.com/tako0614/takoform") || serialized.includes("artifact-transport/v1")) fail("manifest contains a future module or obsolete artifact-transport identity");
  return manifest;
}

export function loadManifest(root = repositoryRoot) {
  try {
    return JSON.parse(readFileSync(path.join(root, MANIFEST_PATH), "utf8"));
  } catch (error) {
    fail(`${MANIFEST_PATH} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function checkMirror(root, generated) {
  const mirrorPath = path.join(root, MIRROR_MANIFEST_PATH);
  if (!existsSync(mirrorPath)) return;
  let mirror;
  try {
    mirror = JSON.parse(readFileSync(mirrorPath, "utf8"));
  } catch (error) {
    fail(`${MIRROR_MANIFEST_PATH} is not valid JSON: ${error instanceof Error ? error.message : String(error)}`);
  }
  validateManifest(mirror, root);
  if (canonicalJson(mirror) !== canonicalJson(generated)) fail(`${MIRROR_MANIFEST_PATH} is stale; run bun run sync:specification-compatibility`);
}

export function run(mode, root = repositoryRoot) {
  if (mode !== "--check" && mode !== "--write") fail("usage: bun scripts/specification-compatibility.mjs --check|--write");
  const generated = generateManifest(root);
  if (mode === "--write") {
    writeFileSync(path.join(root, MANIFEST_PATH), `${JSON.stringify(generated, null, 2)}\n`);
    const mirrorPath = path.join(root, MIRROR_MANIFEST_PATH);
    if (existsSync(path.dirname(mirrorPath))) writeFileSync(mirrorPath, `${JSON.stringify(generated, null, 2)}\n`);
    process.stdout.write(`generated ${MANIFEST_PATH} and (when present) ${MIRROR_MANIFEST_PATH} for the informational Specification ${SPECIFICATION_VERSION} report\n`);
    return generated;
  }
  const current = loadManifest(root);
  validateManifest(current, root);
  if (canonicalJson(current) !== canonicalJson(generated)) fail(`${MANIFEST_PATH} is stale; run bun run sync:specification-compatibility`);
  checkMirror(root, generated);
  process.stdout.write(`Specification ${SPECIFICATION_VERSION} compatibility report OK: ${CLASS_IDS.length} classes\n`);
  return current;
}

if (import.meta.main) run(process.argv[2]);

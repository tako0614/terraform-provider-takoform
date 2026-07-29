import { createHash } from "node:crypto";
import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";

export const PUBLIC_SCHEMA_ORIGIN = "https://forms.takoform.com";
export const PUBLIC_SCHEMA_ROUTE = "forms.takoform.com";
export const PUBLIC_SCHEMA_IDENTITY_LEDGER =
  "release/public-schema-identities.json";

const LEDGER_KIND = "takoform.public-schema-identities@v1";
const SHA256 = /^sha256:[0-9a-f]{64}$/u;

function fail(message) {
  throw new Error(`public schema manifest: ${message}`);
}

function sha256(bytes) {
  return `sha256:${createHash("sha256").update(bytes).digest("hex")}`;
}

export function parsePublicSchemaIdentityLedger(raw, label = "ledger") {
  let document;
  try {
    document = JSON.parse(raw);
  } catch (error) {
    fail(`${label} is invalid JSON (${error.message})`);
  }
  if (
    document === null ||
    typeof document !== "object" ||
    document.kind !== LEDGER_KIND ||
    !Array.isArray(document.identities) ||
    document.identities.length === 0
  ) {
    fail(`${label} must be a non-empty ${LEDGER_KIND} document`);
  }

  const identities = [];
  const seenIDs = new Set();
  const seenSources = new Set();
  const seenPublicPaths = new Set();
  for (const [index, identity] of document.identities.entries()) {
    const keys =
      identity !== null && typeof identity === "object"
        ? Object.keys(identity).sort()
        : [];
    if (keys.join(",") !== "id,public,sha256,source") {
      fail(`${label} identity ${index} must have exactly id, sha256, source, public`);
    }
    if (
      typeof identity.id !== "string" ||
      typeof identity.source !== "string" ||
      typeof identity.public !== "string" ||
      typeof identity.sha256 !== "string" ||
      !SHA256.test(identity.sha256)
    ) {
      fail(`${label} identity ${index} has invalid fields`);
    }

    let id;
    try {
      id = new URL(identity.id);
    } catch (error) {
      fail(`${label} identity ${index} has an invalid id (${error.message})`);
    }
    if (
      id.href !== identity.id ||
      id.origin !== PUBLIC_SCHEMA_ORIGIN ||
      !id.pathname.startsWith("/schemas/") ||
      !id.pathname.endsWith(".json") ||
      id.search !== "" ||
      id.hash !== ""
    ) {
      fail(
        `${label} identity ${index} id must be a canonical ${PUBLIC_SCHEMA_ORIGIN}/schemas/*.json URL`,
      );
    }
    if (!/^spec\/schemas\/[^/]+\.json$/u.test(identity.source)) {
      fail(`${label} identity ${index} has an invalid normative source path`);
    }
    if (identity.public !== `website/public${id.pathname}`) {
      fail(`${label} identity ${index} public path does not match its id`);
    }
    if (seenIDs.has(identity.id)) fail(`${label} duplicates id ${identity.id}`);
    if (seenSources.has(identity.source)) {
      fail(`${label} duplicates source ${identity.source}`);
    }
    if (seenPublicPaths.has(identity.public)) {
      fail(`${label} duplicates public path ${identity.public}`);
    }
    seenIDs.add(identity.id);
    seenSources.add(identity.source);
    seenPublicPaths.add(identity.public);
    identities.push({ ...identity });
  }

  const ordered = identities.map(({ id }) => id);
  if (ordered.join("\n") !== [...ordered].sort().join("\n")) {
    fail(`${label} identities must be sorted by id`);
  }
  return identities;
}

export function readPublicSchemaIdentityLedger(repositoryRoot) {
  const ledgerPath = path.join(repositoryRoot, PUBLIC_SCHEMA_IDENTITY_LEDGER);
  return parsePublicSchemaIdentityLedger(
    readFileSync(ledgerPath, "utf8"),
    PUBLIC_SCHEMA_IDENTITY_LEDGER,
  );
}

export function enforceAppendOnlyPublicSchemaIdentities(
  current,
  historicalSets,
) {
  const currentByID = new Map(current.map((identity) => [identity.id, identity]));
  for (const { identities, label } of historicalSets) {
    for (const historical of identities) {
      const candidate = currentByID.get(historical.id);
      if (!candidate) {
        fail(`${label} identity ${historical.id} was removed`);
      }
      if (
        candidate.sha256 !== historical.sha256 ||
        candidate.source !== historical.source ||
        candidate.public !== historical.public
      ) {
        fail(`${label} identity ${historical.id} was changed`);
      }
    }
  }
}

export function discoverPublicSchemas(repositoryRoot) {
  const normativeRoot = path.join(repositoryRoot, "spec", "schemas");
  const identities = readPublicSchemaIdentityLedger(repositoryRoot);
  const currentSources = readdirSync(normativeRoot, { withFileTypes: true })
    .filter((entry) => entry.isFile() && entry.name.endsWith(".json"))
    .map((entry) => `spec/schemas/${entry.name}`)
    .sort();
  const lockedSources = identities.map(({ source }) => source).sort();
  if (currentSources.join("\n") !== lockedSources.join("\n")) {
    fail(
      "normative JSON schema files must equal the append-only identity ledger",
    );
  }

  return identities.map((identity) => {
    const sourcePath = path.resolve(repositoryRoot, identity.source);
    const publicPath = path.resolve(repositoryRoot, identity.public);
    const sourceBytes = readFileSync(sourcePath);
    let document;
    try {
      document = JSON.parse(sourceBytes);
    } catch (error) {
      fail(`${identity.source} is invalid JSON (${error.message})`);
    }
    if (document.$id !== identity.id) {
      fail(`${identity.source} $id differs from the identity ledger`);
    }
    const observedDigest = sha256(sourceBytes);
    if (observedDigest !== identity.sha256) {
      fail(
        `${identity.source} digest ${observedDigest} differs from locked ${identity.sha256}`,
      );
    }

    return {
      digest: identity.sha256,
      id: identity.id,
      name: path.basename(identity.source),
      pathname: new URL(identity.id).pathname,
      publicPath,
      sourcePath,
    };
  });
}

import { createHash } from "node:crypto";
import { appendFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
import process from "node:process";

export const MAX_EVIDENCE_BINDINGS_BYTES = 8192;

const COMMIT = /^[0-9a-f]{40}$/u;
const DIGEST = /^sha256:[0-9a-f]{64}$/u;
const HOST_ID = /^[a-z0-9][a-z0-9-]{0,63}$/u;
const POSITIVE_INTEGER = /^[1-9][0-9]*$/u;
const VERSION = /^[0-9]+\.[0-9]+\.[0-9]+(?:[-.][0-9A-Za-z.-]+)?$/u;
const UUID_V4 =
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/u;

const FIELD_SPECS = Object.freeze({
  admission_version: {
    environment: "ADMISSION_VERSION",
    pattern: VERSION,
    maximumLength: 128,
  },
  host_candidate_path: {
    environment: "HOST_CANDIDATE_PATH",
    pattern:
      /^admission\/v4\/candidates\/host-report-[0-9A-Za-z.-]+-[0-9a-f]{12}-[0-9a-f]{12}$/u,
  },
  host_candidate_tree: {
    environment: "HOST_CANDIDATE_TREE",
    pattern: COMMIT,
  },
  host_head_sha: { environment: "HOST_HEAD_SHA", pattern: COMMIT },
  host_id: { environment: "HOST_ID", pattern: HOST_ID },
  host_manifest_digest: {
    environment: "HOST_MANIFEST_DIGEST",
    pattern: DIGEST,
  },
  host_request_id: {
    environment: "HOST_REQUEST_ID",
    pattern: UUID_V4,
  },
  host_run_attempt: {
    environment: "HOST_RUN_ATTEMPT",
    pattern: POSITIVE_INTEGER,
    maximumLength: 20,
  },
  host_run_id: {
    environment: "HOST_RUN_ID",
    pattern: POSITIVE_INTEGER,
    maximumLength: 20,
  },
  host_signed_manifest_digest: {
    environment: "HOST_SIGNED_MANIFEST_DIGEST",
    pattern: DIGEST,
  },
  host_source_commit: {
    environment: "HOST_SOURCE_COMMIT",
    pattern: COMMIT,
  },
  host_takoform_source_commit: {
    environment: "HOST_TAKOFORM_SOURCE_COMMIT",
    pattern: COMMIT,
  },
  provider_candidate_path: {
    environment: "PROVIDER_CANDIDATE_PATH",
    pattern:
      /^admission\/v4\/candidates\/provider-report-[0-9A-Za-z.-]+-[0-9a-f]{12}$/u,
  },
  provider_candidate_tree: {
    environment: "PROVIDER_CANDIDATE_TREE",
    pattern: COMMIT,
  },
  provider_head_sha: {
    environment: "PROVIDER_HEAD_SHA",
    pattern: COMMIT,
  },
  provider_manifest_digest: {
    environment: "PROVIDER_MANIFEST_DIGEST",
    pattern: DIGEST,
  },
  provider_request_id: {
    environment: "PROVIDER_REQUEST_ID",
    pattern: UUID_V4,
  },
  provider_run_attempt: {
    environment: "PROVIDER_RUN_ATTEMPT",
    pattern: POSITIVE_INTEGER,
    maximumLength: 20,
  },
  provider_run_id: {
    environment: "PROVIDER_RUN_ID",
    pattern: POSITIVE_INTEGER,
    maximumLength: 20,
  },
  provider_signed_manifest_digest: {
    environment: "PROVIDER_SIGNED_MANIFEST_DIGEST",
    pattern: DIGEST,
  },
  provider_source_commit: {
    environment: "PROVIDER_SOURCE_COMMIT",
    pattern: COMMIT,
  },
  registry_candidate_path: {
    environment: "REGISTRY_CANDIDATE_PATH",
    pattern:
      /^admission\/v4\/candidates\/registry-readback-[0-9A-Za-z.-]+-[0-9a-f]{12}$/u,
  },
  registry_candidate_tree: {
    environment: "REGISTRY_CANDIDATE_TREE",
    pattern: COMMIT,
  },
  registry_head_sha: {
    environment: "REGISTRY_HEAD_SHA",
    pattern: COMMIT,
  },
  registry_manifest_digest: {
    environment: "REGISTRY_MANIFEST_DIGEST",
    pattern: DIGEST,
  },
  registry_request_id: {
    environment: "REGISTRY_REQUEST_ID",
    pattern: UUID_V4,
  },
  registry_run_attempt: {
    environment: "REGISTRY_RUN_ATTEMPT",
    pattern: POSITIVE_INTEGER,
    maximumLength: 20,
  },
  registry_run_id: {
    environment: "REGISTRY_RUN_ID",
    pattern: POSITIVE_INTEGER,
    maximumLength: 20,
  },
  registry_signed_manifest_digest: {
    environment: "REGISTRY_SIGNED_MANIFEST_DIGEST",
    pattern: DIGEST,
  },
  snapshot_commit: { environment: "SNAPSHOT_COMMIT", pattern: COMMIT },
  snapshot_tree: { environment: "SNAPSHOT_TREE", pattern: COMMIT },
});

export const EVIDENCE_BINDING_FIELDS = Object.freeze(
  Object.keys(FIELD_SPECS).sort(),
);

function exactObject(value) {
  return (
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    Object.getPrototypeOf(value) === Object.prototype
  );
}

function canonicalJSON(value) {
  return JSON.stringify(
    Object.fromEntries(EVIDENCE_BINDING_FIELDS.map((field) => [field, value[field]])),
  );
}

function validateObject(value) {
  if (!exactObject(value)) {
    throw new Error("evidence_bindings must be one JSON object");
  }
  const actual = Object.keys(value).sort();
  if (JSON.stringify(actual) !== JSON.stringify(EVIDENCE_BINDING_FIELDS)) {
    const missing = EVIDENCE_BINDING_FIELDS.filter(
      (field) => !Object.hasOwn(value, field),
    );
    const unknown = actual.filter(
      (field) => !EVIDENCE_BINDING_FIELDS.includes(field),
    );
    throw new Error(
      `evidence_bindings fields differ: missing=${missing.join(",") || "none"} unknown=${unknown.join(",") || "none"}`,
    );
  }
  for (const field of EVIDENCE_BINDING_FIELDS) {
    const valueForField = value[field];
    const spec = FIELD_SPECS[field];
    const maximumLength = spec.maximumLength ?? 512;
    if (
      typeof valueForField !== "string" ||
      valueForField.length === 0 ||
      valueForField.length > maximumLength ||
      !spec.pattern.test(valueForField)
    ) {
      throw new Error(`evidence_bindings.${field} is not canonical`);
    }
  }
}

export function parseEvidenceBindings(raw) {
  if (
    typeof raw !== "string" ||
    raw.length === 0 ||
    Buffer.byteLength(raw, "utf8") > MAX_EVIDENCE_BINDINGS_BYTES
  ) {
    throw new Error(
      `evidence_bindings must be 1-${MAX_EVIDENCE_BINDINGS_BYTES} UTF-8 bytes`,
    );
  }
  let value;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`evidence_bindings is not JSON: ${error.message}`);
  }
  validateObject(value);
  if (raw !== canonicalJSON(value)) {
    throw new Error(
      "evidence_bindings must be canonical strict I-JSON with sorted keys, no duplicate keys, and no insignificant whitespace",
    );
  }
  return Object.freeze({ ...value });
}

export function serializeEvidenceBindings(value) {
  validateObject(value);
  const raw = canonicalJSON(value);
  if (Buffer.byteLength(raw, "utf8") > MAX_EVIDENCE_BINDINGS_BYTES) {
    throw new Error(
      `evidence_bindings must be at most ${MAX_EVIDENCE_BINDINGS_BYTES} UTF-8 bytes`,
    );
  }
  return raw;
}

export function evidenceBindingEnvironment(bindings) {
  return Object.freeze(
    Object.fromEntries(
      EVIDENCE_BINDING_FIELDS.map((field) => [
        FIELD_SPECS[field].environment,
        bindings[field],
      ]),
    ),
  );
}

export function evidenceBindingsDigest(raw) {
  return `sha256:${createHash("sha256").update(raw, "utf8").digest("hex")}`;
}

function parseArguments(args) {
  if (
    args.length !== 4 ||
    args[0] !== "--github-env" ||
    args[2] !== "--github-output" ||
    args[1] === "" ||
    args[3] === ""
  ) {
    throw new Error(
      "usage: admission-evidence-bindings --github-env PATH --github-output PATH",
    );
  }
  return { environmentPath: args[1], outputPath: args[3] };
}

function runCLI() {
  const options = parseArguments(process.argv.slice(2));
  const raw = process.env.EVIDENCE_BINDINGS ?? "";
  const bindings = parseEvidenceBindings(raw);
  const environment = evidenceBindingEnvironment(bindings);
  const digest = evidenceBindingsDigest(raw);
  const environmentLines = Object.entries(environment)
    .map(([name, value]) => `${name}=${value}`)
    .concat(`EVIDENCE_BINDINGS_SHA256=${digest}`)
    .join("\n");
  appendFileSync(options.environmentPath, `${environmentLines}\n`, "utf8");
  appendFileSync(
    options.outputPath,
    `snapshot_commit=${bindings.snapshot_commit}\n` +
      `admission_version=${bindings.admission_version}\n` +
      `bindings_digest=${digest}\n`,
    "utf8",
  );
}

if (
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  try {
    runCLI();
  } catch (error) {
    process.stderr.write(`admission-evidence-bindings: ${error.message}\n`);
    process.exit(1);
  }
}

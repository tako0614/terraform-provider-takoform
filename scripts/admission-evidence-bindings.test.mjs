import { describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import {
  EVIDENCE_BINDING_FIELDS,
  evidenceBindingEnvironment,
  evidenceBindingsDigest,
  MAX_EVIDENCE_BINDINGS_BYTES,
  parseEvidenceBindings,
  serializeEvidenceBindings,
} from "./admission-evidence-bindings.mjs";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const scriptPath = join(
  repositoryRoot,
  "scripts",
  "admission-evidence-bindings.mjs",
);
const workflowPath = join(
  repositoryRoot,
  ".github",
  "workflows",
  "standard-admission-evidence.yml",
);
const workflow = readFileSync(workflowPath, "utf8");

const expectedFields = [
  "admission_version",
  "host_candidate_path",
  "host_candidate_tree",
  "host_head_sha",
  "host_id",
  "host_manifest_digest",
  "host_request_id",
  "host_run_attempt",
  "host_run_id",
  "host_signed_manifest_digest",
  "host_source_commit",
  "host_takoform_source_commit",
  "provider_candidate_path",
  "provider_candidate_tree",
  "provider_head_sha",
  "provider_manifest_digest",
  "provider_request_id",
  "provider_run_attempt",
  "provider_run_id",
  "provider_signed_manifest_digest",
  "provider_source_commit",
  "registry_candidate_path",
  "registry_candidate_tree",
  "registry_head_sha",
  "registry_manifest_digest",
  "registry_request_id",
  "registry_run_attempt",
  "registry_run_id",
  "registry_signed_manifest_digest",
  "snapshot_commit",
  "snapshot_tree",
];

const bindings = {
  admission_version: "1.0.7",
  host_candidate_path:
    "admission/v4/candidates/host-report-1.0.7-fd32b41be05d-8c8580e85e0a",
  host_candidate_tree: "a1e42ef2eb4f2c9dc88ce4880f39f7b127bb0afb",
  host_head_sha: "fd32b41be05decb0f417541ddcaaffac711a85db",
  host_id: "takosumi-oss-reference",
  host_manifest_digest: `sha256:${"1".repeat(64)}`,
  host_request_id: "b1554906-8eac-4916-be1e-95ffe6958bdc",
  host_run_attempt: "1",
  host_run_id: "30512615371",
  host_signed_manifest_digest: `sha256:${"2".repeat(64)}`,
  host_source_commit: "fd32b41be05decb0f417541ddcaaffac711a85db",
  host_takoform_source_commit:
    "8c8580e85e0ac3bfddac24d32fe72f9f53164ac4",
  provider_candidate_path:
    "admission/v4/candidates/provider-report-1.0.7-8c8580e85e0a",
  provider_candidate_tree:
    "d98b378d94cd69b6ae2ee78a10f9adf1187d9a90",
  provider_head_sha: "8c8580e85e0ac3bfddac24d32fe72f9f53164ac4",
  provider_manifest_digest: `sha256:${"3".repeat(64)}`,
  provider_request_id: "114f6f59-6ef7-4003-8a42-f3d64a01ef10",
  provider_run_attempt: "1",
  provider_run_id: "30511067002",
  provider_signed_manifest_digest: `sha256:${"4".repeat(64)}`,
  provider_source_commit:
    "8c8580e85e0ac3bfddac24d32fe72f9f53164ac4",
  registry_candidate_path:
    "admission/v4/candidates/registry-readback-1.0.7-8c8580e85e0a",
  registry_candidate_tree:
    "57f51a83cceadc0de22205290b8eaa1e4215b2a4",
  registry_head_sha: "8c8580e85e0ac3bfddac24d32fe72f9f53164ac4",
  registry_manifest_digest: `sha256:${"5".repeat(64)}`,
  registry_request_id: "45a35377-9f3f-4835-939d-1f54c71547ba",
  registry_run_attempt: "1",
  registry_run_id: "30511264836",
  registry_signed_manifest_digest: `sha256:${"6".repeat(64)}`,
  snapshot_commit: "de4f741de26f02339b826fb46e74d512c43176de",
  snapshot_tree: "1469ee31683460233c713fd794187f2418b44cf2",
};

function rawObject(value) {
  return JSON.stringify(
    Object.fromEntries(
      Object.keys(value)
        .sort()
        .map((key) => [key, value[key]]),
    ),
  );
}

describe("Standard admission evidence dispatch bindings", () => {
  test("closes over the exact thirty-one required evidence fields", () => {
    expect(EVIDENCE_BINDING_FIELDS).toEqual(expectedFields);
    const raw = serializeEvidenceBindings(bindings);
    expect(parseEvidenceBindings(raw)).toEqual(bindings);
    expect(Buffer.byteLength(raw, "utf8")).toBeLessThanOrEqual(
      MAX_EVIDENCE_BINDINGS_BYTES,
    );
  });

  test("rejects missing, unknown, non-string, and noncanonical fields", () => {
    const missing = { ...bindings };
    delete missing.snapshot_tree;
    expect(() => parseEvidenceBindings(rawObject(missing))).toThrow(
      "missing=snapshot_tree",
    );

    expect(() =>
      parseEvidenceBindings(rawObject({ ...bindings, unexpected: "value" })),
    ).toThrow("unknown=unexpected");

    expect(() =>
      parseEvidenceBindings(
        rawObject({ ...bindings, registry_run_attempt: 1 }),
      ),
    ).toThrow("registry_run_attempt is not canonical");

    const raw = serializeEvidenceBindings(bindings);
    expect(() => parseEvidenceBindings(` ${raw}`)).toThrow(
      "canonical strict I-JSON",
    );
    const duplicate = raw.replace(
      '{"admission_version":',
      `{"admission_version":${JSON.stringify(bindings.admission_version)},"admission_version":`,
    );
    expect(() => parseEvidenceBindings(duplicate)).toThrow(
      "canonical strict I-JSON",
    );
  });

  test("rejects malformed and oversized input before projection", () => {
    expect(() => parseEvidenceBindings("{")).toThrow("is not JSON");
    expect(() =>
      parseEvidenceBindings("x".repeat(MAX_EVIDENCE_BINDINGS_BYTES + 1)),
    ).toThrow(`1-${MAX_EVIDENCE_BINDINGS_BYTES} UTF-8 bytes`);
    expect(() =>
      parseEvidenceBindings(
        rawObject({ ...bindings, snapshot_commit: "not-a-commit" }),
      ),
    ).toThrow("snapshot_commit is not canonical");
    expect(() =>
      parseEvidenceBindings(
        rawObject({ ...bindings, admission_version: "v1.0.7" }),
      ),
    ).toThrow("admission_version is not canonical");
  });

  test("writes only validated newline-safe environment and step outputs", () => {
    const temporaryRoot = mkdtempSync(
      join(tmpdir(), "takoform-admission-bindings-"),
    );
    try {
      const environmentPath = join(temporaryRoot, "environment");
      const outputPath = join(temporaryRoot, "output");
      writeFileSync(environmentPath, "");
      writeFileSync(outputPath, "");
      const raw = serializeEvidenceBindings(bindings);
      execFileSync(
        "node",
        [
          scriptPath,
          "--github-env",
          environmentPath,
          "--github-output",
          outputPath,
        ],
        {
          env: { ...process.env, EVIDENCE_BINDINGS: raw },
          stdio: ["ignore", "pipe", "pipe"],
        },
      );

      const environment = Object.fromEntries(
        readFileSync(environmentPath, "utf8")
          .trim()
          .split("\n")
          .map((line) => {
            const separator = line.indexOf("=");
            return [line.slice(0, separator), line.slice(separator + 1)];
          }),
      );
      expect(environment).toEqual({
        ...evidenceBindingEnvironment(bindings),
        EVIDENCE_BINDINGS_SHA256: evidenceBindingsDigest(raw),
      });
      expect(readFileSync(outputPath, "utf8")).toBe(
        `snapshot_commit=${bindings.snapshot_commit}\n` +
          `admission_version=${bindings.admission_version}\n` +
          `bindings_digest=${evidenceBindingsDigest(raw)}\n`,
      );
    } finally {
      rmSync(temporaryRoot, { force: true, recursive: true });
    }
  });

  test("stays below GitHub's workflow_dispatch property ceiling", () => {
    const parsedWorkflow = Bun.YAML.parse(workflow);
    const inputs = Object.keys(parsedWorkflow.on.workflow_dispatch.inputs);
    expect(inputs).toEqual(["request_id", "evidence_bindings"]);
    expect(inputs.length).toBeLessThanOrEqual(25);
  });

  test("preserves both exact deterministic builder invocations", () => {
    const buildBlock = workflow
      .split("      - name: Build the exact material twice\n")[1]
      .split("      - name: Close the unsigned handoff\n")[0];
    const actual = [...buildBlock.matchAll(/^\s+--([a-z0-9-]+)\s/gmu)]
      .map((match) => match[1])
      .sort();
    const oneInvocation = [
      "admission-version",
      "host-head-sha",
      "host-id",
      "host-reports",
      "host-request-id",
      "host-run-attempt",
      "host-run-id",
      "host-source-commit",
      "host-takoform-source-commit",
      "output-dir",
      "provider-head-sha",
      "provider-reports",
      "provider-request-id",
      "provider-run-attempt",
      "provider-run-id",
      "provider-source-commit",
      "registry-head-sha",
      "registry-readback",
      "registry-request-id",
      "registry-run-attempt",
      "registry-run-id",
    ];
    expect(actual).toEqual(oneInvocation.concat(oneInvocation).sort());
  });
});

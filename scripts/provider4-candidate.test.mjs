import { expect, test } from "bun:test";
import {
  copyFileSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import {
  generateProvider4Identities,
  PROVIDER4_DESCRIPTOR,
  validateProvider4Candidate,
  writeProvider4Candidate,
} from "./provider4-candidate.mjs";
import {
  provider4CandidatePaths,
  readCurrentProvider4Release,
  validateProvider4ReleaseDescriptor,
} from "./provider-release-descriptor.mjs";

const root = path.resolve(import.meta.dirname, "..");

function readJson(relativePath, directory = root) {
  return JSON.parse(readFileSync(path.join(directory, relativePath), "utf8"));
}

function writeJson(relativePath, value, directory) {
  const destination = path.join(directory, relativePath);
  mkdirSync(path.dirname(destination), { recursive: true });
  writeFileSync(destination, `${JSON.stringify(value, null, 2)}\n`);
}

function cloneProviderInputs(prefix = "provider4-candidate") {
  const directory = mkdtempSync(path.join(tmpdir(), `${prefix}-`));
  const files = [
    "release/version.json",
    "release/history/provider-v3.0.0.json",
    "release/provider-form-identities.json",
    "release/provider-release-identities.json",
    "release/candidates/provider-v4.0.0.json",
    "release/candidates/provider-v4.0.0-form-identities.json",
    "release/candidates/provider-v4.0.1.json",
    "internal/provider/artifacts/publisher/closure.json",
    "internal/provider/artifacts/publisher/projection.json",
  ];
  for (const relativePath of files) {
    const destination = path.join(directory, relativePath);
    mkdirSync(path.dirname(destination), { recursive: true });
    copyFileSync(path.join(root, relativePath), destination);
  }
  return directory;
}

function removeTemporary(directory) {
  rmSync(directory, { recursive: true, force: true });
}

function writeGeneratedCurrentCandidate(directory) {
  const generated = generateProvider4Identities(directory);
  const current = readCurrentProvider4Release(directory);
  writeJson(
    current.candidateFormIdentitiesPath,
    generated.candidate,
    directory,
  );
  const ledger = readJson("release/provider-form-identities.json", directory);
  ledger.releases = ledger.releases.filter(
    (entry) => entry.providerVersion !== generated.ledgerEntry.providerVersion,
  );
  ledger.releases.push(generated.ledgerEntry);
  writeJson("release/provider-form-identities.json", ledger, directory);
  return generated;
}

test("current Provider 4.x candidate keeps the exact 17 publisher Forms", () => {
  const current = readCurrentProvider4Release(root);
  const candidate = validateProvider4Candidate(root);
  expect(candidate.providerVersion).toBe("4.0.1");
  expect(candidate.providerVersion).toBe(current.descriptor.version);
  expect(current.candidateDescriptorPath).toBe(
    "release/candidates/provider-v4.0.1.json",
  );
  expect(current.candidateFormIdentitiesPath).toBe(
    "release/candidates/provider-v4.0.1-form-identities.json",
  );
  expect(candidate.portableApiVersion).toBe("forms.takoform.com/v1");
  expect(candidate.families).toEqual(["edge.forms.takoform.com"]);
  expect(candidate.forms).toHaveLength(17);
  expect(new Set(candidate.forms.map((entry) => entry.resourceType)).size).toBe(17);
  expect(candidate.forms.map((entry) => entry.resourceType)).toContain(
    "takoform_edge_object_bucket",
  );

  const retained = readJson("release/history/provider-v3.0.0.json");
  const published = readJson("release/provider-release-identities.json");
  const currentLedger = readJson("release/provider-form-identities.json");
  expect(retained.version).toBe("3.0.0");
  expect(retained.tag).toBe("v3.0.0");
  expect(
    published.entries.find((entry) => entry.version === "4.0.0")?.registryReadback
      ?.installation.resourceSchemaCount,
  ).toBe(17);
  expect(
    published.entries.some((entry) => entry.version === current.descriptor.version),
  ).toBe(false);
  expect(
    currentLedger.releases.find((entry) => entry.providerVersion === "4.0.0")?.forms,
  ).toHaveLength(17);
});

test("generated 4.0.1 identities accept the same 17 Forms without publishing", () => {
  const synthetic = cloneProviderInputs("provider4-generated");
  try {
    const generated = writeGeneratedCurrentCandidate(synthetic);
    const candidate = validateProvider4Candidate(synthetic);
    expect(generated.candidate.providerVersion).toBe("4.0.1");
    expect(candidate.forms).toHaveLength(17);
    expect(candidate.forms).toEqual(generated.candidate.forms);
    expect(
      readJson("release/provider-release-identities.json", synthetic).entries.some(
        (entry) => entry.version === "4.0.1",
      ),
    ).toBe(false);
  } finally {
    removeTemporary(synthetic);
  }
});

test("the current descriptor is the path authority and rejects a mirror mismatch", () => {
  const synthetic = cloneProviderInputs("provider4-path-authority");
  try {
    const current = readCurrentProvider4Release(synthetic);
    expect(current.candidateDescriptorPath).toBe(
      provider4CandidatePaths("4.0.1").descriptorPath,
    );
    writeFileSync(
      path.join(synthetic, "release/candidates/provider-v4.0.0.json"),
      "historical mirror must not become current\n",
    );
    expect(() => readCurrentProvider4Release(synthetic)).not.toThrow();
    writeFileSync(
      path.join(synthetic, "release/version.json"),
      `${readFileSync(path.join(synthetic, "release/version.json"), "utf8")}\n`,
    );
    expect(() => readCurrentProvider4Release(synthetic)).toThrow(
      /byte-identical/u,
    );
  } finally {
    removeTemporary(synthetic);
  }
});

test("Provider 4 descriptor validation rejects malformed versions, tags, and identity drift", () => {
  const descriptor = readJson("release/version.json");
  expect(validateProvider4ReleaseDescriptor(descriptor)).toBe(descriptor);
  for (const version of [
    "4.0.1-rc.1",
    "4.0.1+build.1",
    "4.01.0",
    "04.0.1",
    "4.0.01",
    "4.0",
    "5.0.0",
    "3.0.0",
    "4.0.1/escape",
    "4.0.1\n",
    "4.0.1\r\n",
  ]) {
    expect(() =>
      validateProvider4ReleaseDescriptor({ ...descriptor, version, tag: `v${version}` }),
    ).toThrow(/stable Provider 4\.x/u);
  }
  expect(() =>
    validateProvider4ReleaseDescriptor({ ...descriptor, tag: "v4.0.0" }),
  ).toThrow(/tag/u);

  const identityMutations = [
    ["sourceRepository", "github.com/attacker/provider"],
    ["formPublisherRepository", "github.com/attacker/forms"],
    ["formPublisherCommit", "0".repeat(40)],
    ["formSetTag", "forms/sets/attacker"],
    ["providerAddress", "registry.terraform.io/attacker/provider"],
    ["goModule", "github.com/attacker/provider"],
    ["signingFingerprint", "0".repeat(40)],
    ["publicationStatus", "published"],
  ];
  for (const [field, value] of identityMutations) {
    expect(() =>
      validateProvider4ReleaseDescriptor({ ...descriptor, [field]: value }),
    ).toThrow(/identity|candidate-only/u);
  }
  expect(() =>
    validateProvider4ReleaseDescriptor({
      ...descriptor,
      platforms: [...descriptor.platforms, "linux_386"],
    }),
  ).toThrow(/platforms/u);
  expect(() =>
    validateProvider4ReleaseDescriptor({
      ...descriptor,
      versioning: { ...descriptor.versioning, portableApiVersion: "forms.takoform.com/v2" },
    }),
  ).toThrow(/versioning/u);
  expect(() =>
    validateProvider4ReleaseDescriptor({
      ...descriptor,
      cliMatrix: descriptor.cliMatrix.map((entry) => ({
        ...entry,
        providerAddress: "registry.terraform.io/attacker/provider",
      })),
    }),
  ).toThrow(/cliMatrix/u);
});

test("retained Provider 4.0.0 descriptor, projection, ledger, and readback are immutable", () => {
  const mutations = [
    (synthetic) =>
      writeFileSync(
        path.join(synthetic, "release/candidates/provider-v4.0.0.json"),
        "mutated historical descriptor\n",
      ),
    (synthetic) =>
      writeFileSync(
        path.join(synthetic, "release/candidates/provider-v4.0.0-form-identities.json"),
        "mutated historical projection\n",
      ),
    (synthetic) => {
      const ledger = readJson("release/provider-form-identities.json", synthetic);
      ledger.releases.push(
        ledger.releases.find((entry) => entry.providerVersion === "4.0.0"),
      );
      writeJson("release/provider-form-identities.json", ledger, synthetic);
    },
    (synthetic) => {
      const ledger = readJson("release/provider-form-identities.json", synthetic);
      ledger.releases.find((entry) => entry.providerVersion === "4.0.0").forms[0]
        .packageDigest = `sha256:${"0".repeat(64)}`;
      writeJson("release/provider-form-identities.json", ledger, synthetic);
    },
    (synthetic) => {
      const identities = readJson("release/provider-release-identities.json", synthetic);
      identities.entries.find((entry) => entry.version === "4.0.0").tag = "v4.0.0-mutated";
      writeJson("release/provider-release-identities.json", identities, synthetic);
    },
  ];
  for (const mutate of mutations) {
    const synthetic = cloneProviderInputs("provider4-history-lock");
    try {
      mutate(synthetic);
      expect(() => generateProvider4Identities(synthetic)).toThrow(/immutable|exactly one/u);
    } finally {
      removeTemporary(synthetic);
    }
  }
});

test("a published current Provider 4.x readback permits only an exact no-op writer", () => {
  const synthetic = cloneProviderInputs("provider4-published-current");
  try {
    writeGeneratedCurrentCandidate(synthetic);
    const published = readJson("release/provider-release-identities.json", synthetic);
    const historical = published.entries.find((entry) => entry.version === "4.0.0");
    const currentReadback = JSON.parse(JSON.stringify(historical));
    currentReadback.version = "4.0.1";
    currentReadback.tag = "v4.0.1";
    published.entries.push(currentReadback);
    writeJson("release/provider-release-identities.json", published, synthetic);
    expect(() => validateProvider4Candidate(synthetic)).not.toThrow();
    const identityPath = path.join(
      synthetic,
      "release/candidates/provider-v4.0.1-form-identities.json",
    );
    const ledgerPath = path.join(synthetic, "release/provider-form-identities.json");
    const identityBefore = readFileSync(identityPath);
    const ledgerBefore = readFileSync(ledgerPath);
    expect(() => writeProvider4Candidate(synthetic)).not.toThrow();
    expect(readFileSync(identityPath)).toEqual(identityBefore);
    expect(readFileSync(ledgerPath)).toEqual(ledgerBefore);

    const mutatedIdentity = readJson(
      "release/candidates/provider-v4.0.1-form-identities.json",
      synthetic,
    );
    mutatedIdentity.forms[0].packageDigest = `sha256:${"0".repeat(64)}`;
    writeJson(
      "release/candidates/provider-v4.0.1-form-identities.json",
      mutatedIdentity,
      synthetic,
    );
    expect(() => writeProvider4Candidate(synthetic)).toThrow(/published|changed/u);
  } finally {
    removeTemporary(synthetic);
  }
});

test("current Provider surfaces identify the publisher without a privileged classification", () => {
  for (const relativePath of [
    "README.md",
    "docs/index.md",
    "website/index.md",
    "website/docs/index.md",
    PROVIDER4_DESCRIPTOR,
    "release/version.json",
  ]) {
    const contents = readFileSync(path.join(root, relativePath), "utf8");
    expect(contents).not.toMatch(/\bofficial(?:-only)?\b/iu);
  }
});

// Keep the historical v4.0.0 descriptor assertions below as history checks;
// they intentionally do not describe the current 4.0.1 writer input.
test("historical Provider 4.0.0 and retained Provider 3 descriptors stay named", () => {
  const retained = readJson("release/history/provider-v3.0.0.json");
  const historical = readJson(PROVIDER4_DESCRIPTOR);
  expect(retained.version).toBe("3.0.0");
  expect(retained.tag).toBe("v3.0.0");
  expect(historical.version).toBe("4.0.0");
  expect(historical.tag).toBe("v4.0.0");
  expect(historical.publicationStatus).toBe("candidate-only");
  expect(historical.formPublisherRepository).toBe(
    "github.com/tako0614/takoform-forms",
  );
  expect(historical.formPublisherCommit).toBe(
    "3231633605b737ce5279d7fc020b4780568e7091",
  );
  expect(historical.formSetTag).toBe(
    "forms/sets/e7f8a39311dd011b8467e97e7f300cabb9a6b06c",
  );
});

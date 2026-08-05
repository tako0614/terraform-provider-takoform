import { expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);

const currentAuthoritySurfaces = [
  "README.md",
  "conformance/README.md",
  "forms/README.md",
  "release/form-packages.md",
  "spec/README.md",
  "spec/host-api/README.md",
  "cmd/provider-lifecycle-conformance/main.go",
  "internal/formcatalog/catalog.go",
  "internal/formregistry/registry.go",
  "internal/providerreport/provider_report.go",
  "internal/standardforms/release_plan.go",
  "internal/standardforms/publish_surfaces.go",
  "internal/standardforms/standardforms.go",
];

test("current authority surfaces do not revive retired central admission", () => {
  const source = currentAuthoritySurfaces
    .map((relativePath) =>
      readFileSync(path.join(repositoryRoot, relativePath), "utf8"),
    )
    .join("\n");

  for (const staleClaim of [
    "admission still requires",
    "can become portable standards",
    "Only that authenticated evidence may classify",
    "is an admitted host report",
    "Availability and admission remain host-owned checks",
    "Backend placement, admission, and credentials remain host responsibilities",
    "This is the current provider-v1 portable Service Form inventory",
    "Specs is the active portable Form set",
    "declares every portable Service Form this release",
    "exact current candidate release-source",
    "supported CLI/FQN candidate matrix",
    "Repeat prepare/publish/verify for all 34 plan entries",
    "a Form becomes a portable standard only with",
    "Admission remains a separate decision",
    "This inventory is `structural-candidate`",
    "signed admission evidence are external requirements",
    "lifecycle projection and legacy migration tooling are being implemented",
  ]) {
    expect(source).not.toContain(staleClaim);
  }
});

test("the exported standard-admission parser is explicitly Legacy-only", () => {
  const source = readFileSync(
    path.join(repositoryRoot, "standardform/model.go"),
    "utf8",
  );
  const prose = source.replace(/^\/\/ ?/gm, "").replace(/\s+/g, " ");
  expect(prose).toContain("Legacy verification surface");
  expect(prose).toContain("grant no current Form maturity");
  expect(prose).toContain(
    "must not create or consume new central standard-admission evidence",
  );
});

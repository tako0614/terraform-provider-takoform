import { describe, expect, test } from "bun:test";
import { execFileSync } from "node:child_process";
import {
  mkdtempSync,
  readFileSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";

const repositoryRoot = fileURLToPath(new URL("..", import.meta.url));
const workflowsRoot = join(repositoryRoot, ".github", "workflows");

function embeddedNodeScripts(filePath) {
  const lines = readFileSync(filePath, "utf8").split("\n");
  const scripts = [];
  for (let index = 0; index < lines.length; index += 1) {
    const start = lines[index].match(/^(\s*).*node <<'NODE'\s*$/);
    if (start === null) continue;
    const indent = start[1];
    const end = lines.indexOf(`${indent}NODE`, index + 1);
    if (end < 0) {
      throw new Error(`${filePath}:${index + 1}: unterminated Node heredoc`);
    }
    const source = lines
      .slice(index + 1, end)
      .map((line, offset) => {
        if (line !== "" && !line.startsWith(indent)) {
          throw new Error(
            `${filePath}:${index + offset + 2}: malformed Node heredoc indentation`,
          );
        }
        return line.slice(indent.length);
      })
      .join("\n");
    scripts.push({ line: index + 1, source });
    index = end;
  }
  return scripts;
}

const workflowScripts = readdirSync(workflowsRoot)
  .filter((name) => name.endsWith(".yml"))
  .sort()
  .flatMap((name) =>
    embeddedNodeScripts(join(workflowsRoot, name)).map((script) => ({
      ...script,
      workflow: name,
    })),
  );

describe("workflow embedded Node programs", () => {
  test.each(workflowScripts)(
    "$workflow:$line parses with Node",
    ({ source }) => {
      expect(() =>
        execFileSync("node", ["--check", "-"], {
          input: source,
          stdio: ["pipe", "pipe", "pipe"],
        }),
      ).not.toThrow();
    },
  );

  test("provider provenance generator executes against its exact 13-subject contract", () => {
    const matches = workflowScripts.filter(({ source }) =>
      source.includes(
        "provenance subjects do not exactly close the sorted 13-file payload",
      ),
    );
    expect(matches).toHaveLength(1);

    const version = "1.0.1";
    const base = `terraform-provider-takoform_${version}`;
    const names = [
      "darwin_amd64",
      "darwin_arm64",
      "linux_amd64",
      "linux_arm64",
      "windows_amd64",
    ]
      .flatMap((platform) => {
        const archive = `${base}_${platform}.zip`;
        return [archive, `${archive}.spdx.json`];
      })
      .concat(
        `${base}_manifest.json`,
        `${base}_SHA256SUMS`,
        `${base}_SHA256SUMS.sig`,
      )
      .sort();
    const temporaryRoot = mkdtempSync(
      join(tmpdir(), "takoform-workflow-node-"),
    );
    try {
      const metadataPath = join(temporaryRoot, "payload.tsv");
      const provenancePath = join(temporaryRoot, "provenance.json");
      writeFileSync(
        metadataPath,
        names
          .map((name, index) => `${name}\t${index + 1}\t${"a".repeat(64)}`)
          .join("\n") + "\n",
      );
      const repository = "tako0614/terraform-provider-takoform";
      const tag = `v${version}`;
      const runId = "123";
      const runAttempt = "2";
      const invocationId = `https://github.com/${repository}/actions/runs/${runId}/attempts/${runAttempt}`;
      execFileSync("node", [], {
        input: matches[0].source,
        env: {
          ...process.env,
          GITHUB_REPOSITORY: repository,
          GITHUB_RUN_ATTEMPT: runAttempt,
          GITHUB_RUN_ID: runId,
          GITHUB_WORKFLOW_REF: `${repository}/.github/workflows/release.yml@refs/tags/${tag}`,
          INVOCATION_ID: invocationId,
          PAYLOAD_METADATA_PATH: metadataPath,
          PROVENANCE_PATH: provenancePath,
          RELEASE_TAG: tag,
          REQUEST_ID: "123e4567-e89b-42d3-a456-426614174000",
          SOURCE_COMMIT: "b".repeat(40),
          TAG_OBJECT_OID: "c".repeat(40),
          TAG_OBJECT_SHA256: `sha256:${"d".repeat(64)}`,
          TOOLING_COMMIT: "b".repeat(40),
        },
        stdio: ["pipe", "pipe", "pipe"],
      });
      const provenance = JSON.parse(readFileSync(provenancePath, "utf8"));
      expect(provenance.subject.map(({ name }) => name)).toEqual(names);
      expect(provenance.predicate.runDetails.metadata.invocationId).toBe(
        invocationId,
      );
      expect(basename(provenancePath)).toBe("provenance.json");
    } finally {
      rmSync(temporaryRoot, { force: true, recursive: true });
    }
  });
});

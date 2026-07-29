import { lstatSync, readdirSync } from "node:fs";
import { join } from "node:path";

const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;

export function collectRegularFiles(directory) {
  const found = [];
  for (const entry of readdirSync(directory)) {
    const path = join(directory, entry);
    const metadata = lstatSync(path);
    if (metadata.isSymbolicLink()) {
      throw new Error(`published asset must not be a symbolic link: ${path}`);
    }
    if (metadata.isDirectory()) {
      found.push(...collectRegularFiles(path));
      continue;
    }
    if (!metadata.isFile()) {
      throw new Error(`published asset must be a regular file: ${path}`);
    }
    found.push(path);
  }
  return found;
}

export function parseCurrentProductionDeployment(raw) {
  let deployment;
  try {
    deployment = JSON.parse(raw);
  } catch (error) {
    throw new Error(`current deployment status was not JSON: ${error.message}`);
  }

  if (
    deployment === null ||
    typeof deployment !== "object" ||
    !UUID.test(deployment.id ?? "")
  ) {
    throw new Error("current deployment status has no valid deployment id");
  }
  if (
    deployment.strategy !== "percentage" ||
    !Array.isArray(deployment.versions) ||
    deployment.versions.length !== 1
  ) {
    throw new Error(
      "current deployment is ambiguous or has split traffic; a single 100% version is required",
    );
  }

  const [{ percentage, version_id: versionId }] = deployment.versions;
  if (percentage !== 100 || !UUID.test(versionId ?? "")) {
    throw new Error(
      "current deployment is ambiguous or has split traffic; a single 100% version is required",
    );
  }
  return {
    deploymentId: deployment.id,
    versionId,
  };
}

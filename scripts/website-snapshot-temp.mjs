import { mkdtempSync, rmSync } from "node:fs";
import path from "node:path";

const SNAPSHOT_PREFIX = ".website-snapshot-tmp-";

// Each snapshot run owns one atomic, process-independent workspace. The
// returned cleanup closes over that exact path so a concurrent run can never
// remove another run's VitePress temporary files.
export function createWebsiteSnapshotWorkspace(repositoryRoot) {
  const root = mkdtempSync(path.join(path.resolve(repositoryRoot), SNAPSHOT_PREFIX));
  let cleaned = false;
  return {
    root,
    cleanup() {
      if (cleaned) return;
      cleaned = true;
      rmSync(root, { force: true, recursive: true });
    },
  };
}

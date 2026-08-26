import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

const SNAPSHOT_PREFIX = "takoform-website-snapshot-";

// Each snapshot run owns one atomic, process-independent workspace. The
// returned cleanup closes over that exact path so a concurrent run can never
// remove another run's VitePress temporary files.
export function createWebsiteSnapshotWorkspace(parentDirectory = tmpdir()) {
  const root = mkdtempSync(
    path.join(path.resolve(parentDirectory), SNAPSHOT_PREFIX),
  );
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

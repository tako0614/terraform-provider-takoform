#!/usr/bin/env bun

// takoform の唯一の deploy entrypoint です。
//
// 共通の obligation と trigger は takos-control の
// `engineering.policy.json` → `deploy` が正本です。
//
//   bun run deploy -- takoform-website
//   bun run deploy -- takoform-provider-release <phase> ...
//   bun run deploy -- takoform-form-package-release <phase> ...
//
// `--contract` は副作用なしで、この repo が publish できる surface と、それぞれの
// trigger・義務の果たし方を印字します。
//
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import process from "node:process";

import {
  assertCommittedGitAuthority,
  assertPublicationManifest,
  assertSafeRepositoryGitConfiguration,
  collectRegularFiles,
  createCommittedPublicationManifest,
  createCommittedSnapshot,
  createHardenedGateEnvironment,
  createHardenedGitEnvironment,
  createPinnedWranglerInstallation,
  createPublicationManifest,
  createPublicationManifestFromEntries,
  inspectUncommittedPublicationPaths,
  parseCurrentProductionDeployment,
  pinnedWranglerInvocation,
} from "./deploy-safety.mjs";
import {
  discoverPublicSchemas,
  enforceAppendOnlyPublicSchemaIdentities,
  parsePublicSchemaIdentityLedger,
  PUBLIC_SCHEMA_IDENTITY_LEDGER,
  readPublicSchemaIdentityLedger,
} from "./public-schema-manifest.mjs";
import {
  enforceSchemaPublicationNoOverwrite,
  INITIAL_SCHEMA_ORIGIN_MINT_ACK,
  inspectSchemaPublicationIdentities,
  readPublishedDigest,
} from "./schema-publication-guard.mjs";
import {
  isReleaseSurface,
  RELEASE_SURFACES,
  runReleaseSurface,
} from "./release-deploy.mjs";
import {
  CLOUDFLARE_ACCOUNT_ENV,
  CLOUDFLARE_ZONE_ENV,
  createPinnedWranglerEnvironment,
  EXCLUSIVE_WRITER_ACK,
  parseCloudflareZone,
  parseDomainChangeset,
  parseUploadedVersionId,
  parseUploadedVersionResources,
  parseWebsiteWranglerConfig,
  parseWorkerDomainClosure,
  parseWranglerOAuthToken,
  parseWranglerWhoami,
  proveAuthoritativeHostnameAbsent,
  readExpectedCloudflareIdentity,
  runFencedMutation,
  safeDomainWriteBody,
} from "./website-cloudflare-safety.mjs";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const SITE = {
  surface: "takoform-website",
  worker: "takoform-website",
  assets: "website/public",
  config: "website/wrangler.jsonc",
  authorityGate: "check:public-authority",
  gate: "check:public-surfaces",
  snapshotGate: "check:public-snapshot",
  url: "https://takoform.com",
};
const PUBLICATION_PATHS = [SITE.config, SITE.assets];
const VERIFIED_SNAPSHOT_PATHS = ["."];
const PINNED_WRANGLER_VERSION = "4.115.0";
const COMPATIBILITY_DATE = "2026-07-01";
const APEX_HOSTNAMES = ["takoform.com", "www.takoform.com"];
const SCHEMA_HOSTNAME = "forms.takoform.com";
const HOSTNAMES = [...APEX_HOSTNAMES, SCHEMA_HOSTNAME];
const ZONE_NAME = "takoform.com";
const INITIAL_DOMAIN_RECOVERY = "--recover-initial-schema-domain";
const EXPECTED_DEPLOYMENT_PREFIX = "--expected-deployment=";
const EXPECTED_VERSION_PREFIX = "--expected-version=";
const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;
const CANONICAL_ORIGIN =
  "https://github.com/tako0614/terraform-provider-takoform.git";

const CONTRACT = {
  kind: "takos.deploy-contract@v2",
  surfaces: [
    {
      surface: SITE.surface,
      target: `cloudflare-worker:${SITE.worker}`,
      covers: ["website/wrangler.jsonc"],
      requiresScripts: [
        SITE.gate,
        SITE.authorityGate,
        SITE.snapshotGate,
      ],
      requiresTools: ["git", "bun", "go", "node", "tar"],
      requiresEnv: [CLOUDFLARE_ACCOUNT_ENV, CLOUDFLARE_ZONE_ENV],
      // 静的 asset だけを配る Worker で、durable state も server handler も
      // 持ちません。ただし schema $id は consumer が固定する公開 identity です。
      triggers: ["irreversible", "authority", "published-identity"],
      obligations: {
        provenance: `refuses a dirty or shallow worktree, rejects ignored and untracked publication files, requires main HEAD to equal a fresh read of the canonical HTTPS origin/main ref, creates both an isolated git-archive content snapshot and an independent non-local detached Git authority clone of that exact commit, and removes the clone's remote. The source-retained publication/admission authority gate runs only in the frozen clone; the static public-surface gate, credential scan, digest manifest, and Wrangler input run only in the archive. Both roots are hardened and re-hashed before and after validation and again before every writer. Every archive byte is also proved against its Git blob. Wrangler is installed from the exact committed lock and executed through the fixed absolute Node entrypoint, bypassing PATH and its environment shebang. \`bun run ${SITE.gate}\` remains the composite source gate, while the repository-wide \`bun run check\` remains separate handoff evidence because its other Go and OpenTofu checks do not validate these static bytes.`,
        "post-conditions": `requires the exact three-domain Cloudflare control-plane closure, queries every hostname independently, and reads back ${SITE.url}/, the www root, docs, spec, sitemap, static assets, the custom 404 body/status, and every normative schema $id with the exact committed digest`,
        reversal: `the current version id is read and printed before publishing. A previous version may be restored with \`wrangler versions deploy <previous-id>@100%\` only after proving it still serves every already-minted schema $id byte-for-byte. The initial schema-origin mint has no schema-safe rollback to a version without those identities; repair it forward while preserving the minted bytes.`,
        "failure-handling":
          "records previous, uploaded, and current deployment/version ids; a failed dormant upload never authorizes promotion. An indeterminate initial domain operation emits one id-bound forward-recovery command that uploads or deploys no version, verifies the current version message/static closure and candidate schema bytes through the apex, and either performs the safe absent-domain write or only repeats readback for an exact existing domain.",
        "pre-mutation-proof": `requires explicit operator-private account and zone ids plus an exclusive-writer acknowledgement, rejects ambient Cloudflare/Wrangler/runtime overrides, binds the local OAuth profile to those ids, proves the active zone and authoritative delegation, reads the current ${SITE.worker} deployment and exact domain changeset, and accepts only all-exact existing schemas or an explicitly acknowledged wholly absent first mint. Every upload, version promotion, and domain write runs through a proof-then-whole-tree/source/deployment fence immediately before its writer.`,
        "independent-review": "the TASK-0009 release-surface review independently checked the website, custom-domain, DNS, append-only schema identity, rollback, and live-readback boundary; the operator retains that review with the deploy result before the first origin mint",
        "no-overwrite": `requires the candidate ${PUBLIC_SCHEMA_IDENTITY_LEDGER} to retain every identity and digest recorded by every reachable historical ledger revision, then immediately before any Cloudflare mutation fetches every retained schema $id with cache bypass and requires its served bytes to equal the candidate exactly. A removed historical identity, differing body, HTTP response other than 200, non-DNS transport failure, redirect, or partially existing origin blocks publication. Only when every URL fails specifically with ENOTFOUND may an operator mint the origin by passing ${INITIAL_SCHEMA_ORIGIN_MINT_ACK}; the acknowledgement is rejected once any URL exists and never bypasses a mismatch.`,
      },
    },
    ...RELEASE_SURFACES,
  ],
};

if (process.argv.includes("--contract")) {
  process.stdout.write(`${JSON.stringify(CONTRACT, null, 2)}\n`);
  process.exit(0);
}

const invocation = process.argv.slice(2);
if (isReleaseSurface(invocation[0])) {
  try {
    runReleaseSurface({
      surface: invocation[0],
      args: invocation.slice(1),
      repo,
    });
  } catch (error) {
    process.stderr.write(`deploy blocked: ${error.message}\n`);
    process.exit(1);
  }
  process.exit(0);
}
const acknowledgedInitialSchemaOriginMint = invocation.includes(
  INITIAL_SCHEMA_ORIGIN_MINT_ACK,
);
const acknowledgedExclusiveWriter = invocation.includes(EXCLUSIVE_WRITER_ACK);
const recoveringInitialDomain = invocation.includes(INITIAL_DOMAIN_RECOVERY);
const expectedDeploymentArguments = invocation.filter((argument) =>
  argument.startsWith(EXPECTED_DEPLOYMENT_PREFIX),
);
const expectedVersionArguments = invocation.filter((argument) =>
  argument.startsWith(EXPECTED_VERSION_PREFIX),
);
const expectedRecoveryDeployment =
  expectedDeploymentArguments[0]?.slice(EXPECTED_DEPLOYMENT_PREFIX.length);
const expectedRecoveryVersion =
  expectedVersionArguments[0]?.slice(EXPECTED_VERSION_PREFIX.length);
const unknownOptions = invocation.filter(
  (arg) =>
    arg.startsWith("--") &&
    arg !== INITIAL_SCHEMA_ORIGIN_MINT_ACK &&
    arg !== EXCLUSIVE_WRITER_ACK &&
    arg !== INITIAL_DOMAIN_RECOVERY &&
    !arg.startsWith(EXPECTED_DEPLOYMENT_PREFIX) &&
    !arg.startsWith(EXPECTED_VERSION_PREFIX),
);
const requested = invocation.filter((arg) => !arg.startsWith("--"));
const known = CONTRACT.surfaces.map((entry) => entry.surface);
if (
  requested.length !== 1 ||
  requested[0] !== SITE.surface ||
  unknownOptions.length > 0 ||
  invocation.filter((arg) => arg === INITIAL_SCHEMA_ORIGIN_MINT_ACK).length >
    1 ||
  invocation.filter((arg) => arg === EXCLUSIVE_WRITER_ACK).length > 1 ||
  invocation.filter((arg) => arg === INITIAL_DOMAIN_RECOVERY).length > 1
) {
  process.stderr.write(
    `usage: bun run deploy -- <surface> ${EXCLUSIVE_WRITER_ACK} [${INITIAL_SCHEMA_ORIGIN_MINT_ACK}]\nknown surfaces: ${known.join(", ")}\n`,
  );
  process.exit(1);
}
if (
  expectedDeploymentArguments.length > 1 ||
  expectedVersionArguments.length > 1 ||
  (recoveringInitialDomain &&
    (!UUID.test(expectedRecoveryDeployment ?? "") ||
      !UUID.test(expectedRecoveryVersion ?? "") ||
      !acknowledgedInitialSchemaOriginMint)) ||
  (!recoveringInitialDomain &&
    (expectedDeploymentArguments.length > 0 ||
      expectedVersionArguments.length > 0))
) {
  process.stderr.write(
    `deploy blocked: ${INITIAL_DOMAIN_RECOVERY} requires ${INITIAL_SCHEMA_ORIGIN_MINT_ACK}, ${EXPECTED_DEPLOYMENT_PREFIX}<uuid>, and ${EXPECTED_VERSION_PREFIX}<uuid>; expected ids are forbidden for a normal deploy\n`,
  );
  process.exit(1);
}
if (!acknowledgedExclusiveWriter) {
  process.stderr.write(
    `deploy blocked: publication requires ${EXCLUSIVE_WRITER_ACK}; it asserts that main, the Worker deployment, and its custom domains have one writer for the whole attempt\n`,
  );
  process.exit(1);
}

function die(message, detail = []) {
  process.stderr.write(`deploy blocked: ${message}\n`);
  for (const line of detail) process.stderr.write(`- ${line}\n`);
  process.exit(1);
}

const initialDomainRecoveryCommand = (deploymentId, versionId) =>
  `bun run deploy -- ${SITE.surface} ${EXCLUSIVE_WRITER_ACK} ${INITIAL_SCHEMA_ORIGIN_MINT_ACK} ${INITIAL_DOMAIN_RECOVERY} ${EXPECTED_DEPLOYMENT_PREFIX}${deploymentId} ${EXPECTED_VERSION_PREFIX}${versionId}`;

const gitExecutable = "/usr/bin/git";
const nodeExecutable = "/usr/bin/node";
const tarExecutable = "/usr/bin/tar";
if (
  !existsSync(gitExecutable) ||
  !existsSync(nodeExecutable) ||
  !existsSync(tarExecutable)
) {
  die("publication requires /usr/bin/git, /usr/bin/node, and /usr/bin/tar");
}
const gitEnvironment = createHardenedGitEnvironment(process.env);
const git = (...args) =>
  execFileSync(gitExecutable, args, {
    cwd: repo,
    encoding: "utf8",
    env: gitEnvironment,
    maxBuffer: 64 * 1024 * 1024,
    stdio: ["ignore", "pipe", "pipe"],
  }).trim();

const run = (
  command,
  args,
  { cwd = repo, environment = process.env } = {},
) =>
  execFileSync(command, args, {
    cwd,
    encoding: "utf8",
    env: environment,
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 64 * 1024 * 1024,
  });

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

async function cloudflareRequest(
  token,
  path,
  { body, method = "GET" } = {},
) {
  const url = new URL(path, "https://api.cloudflare.com/client/v4/");
  if (url.origin !== "https://api.cloudflare.com") {
    throw new Error("Cloudflare API request escaped the pinned API origin");
  }
  const response = await fetch(url, {
    body: body === undefined ? undefined : JSON.stringify(body),
    headers: {
      accept: "application/json",
      authorization: `Bearer ${token}`,
      ...(body === undefined ? {} : { "content-type": "application/json" }),
    },
    method,
    redirect: "error",
    signal: AbortSignal.timeout(30_000),
  });
  const raw = await response.text();
  let value;
  try {
    value = JSON.parse(raw);
  } catch {
    throw new Error(`Cloudflare API ${url.pathname} returned non-JSON`);
  }
  if (!response.ok || value?.success !== true) {
    throw new Error(
      `Cloudflare API ${url.pathname} failed with HTTP ${response.status}`,
    );
  }
  return value;
}

let expectedCloudflare;
let wranglerEnvironment;
try {
  expectedCloudflare = readExpectedCloudflareIdentity(process.env);
  wranglerEnvironment = createPinnedWranglerEnvironment(
    process.env,
    {
      ...expectedCloudflare,
      bunLocalPrefix: repo,
      bunUserAgent:
        `bun/${Bun.version} npm/? node/v${process.versions.node} ` +
        `${process.platform} ${process.arch}`,
    },
  );
} catch (error) {
  die(`Cloudflare authority is not explicit and isolated: ${error.message}`);
}

let originURL;
const assertRepositoryGitConfiguration = () => {
  assertSafeRepositoryGitConfiguration(
    execFileSync(
      gitExecutable,
      ["config", "--local", "-z", "--list"],
      {
        cwd: repo,
        encoding: "utf8",
        env: gitEnvironment,
        stdio: ["ignore", "pipe", "pipe"],
      },
    ),
    originURL,
  );
};
try {
  originURL = git("remote", "get-url", "origin");
  if (originURL !== CANONICAL_ORIGIN) {
    throw new Error(`origin is not canonical: ${originURL}`);
  }
  assertRepositoryGitConfiguration();
} catch (error) {
  die(`repository Git configuration is not publication-safe: ${error.message}`);
}

// provenance: 公開バイト列を一つの commit に結び付ける。この site は build 段が
// ないので、公開されるのは commit されている bytes そのものです。
let publicationResidue;
try {
  publicationResidue = inspectUncommittedPublicationPaths({
    environment: gitEnvironment,
    gitExecutable,
    paths: PUBLICATION_PATHS,
    repo,
  });
} catch (error) {
  die(`cannot inspect uncommitted publication paths: ${error.message}`);
}
if (
  publicationResidue.untracked.length > 0 ||
  publicationResidue.ignored.length > 0
) {
  die("untracked or ignored files exist below a publication path", [
    ...publicationResidue.untracked.map((path) => `untracked: ${path}`),
    ...publicationResidue.ignored.map((path) => `ignored: ${path}`),
  ]);
}
const dirty = git("status", "--porcelain");
if (dirty !== "") {
  die(
    "the worktree is not clean; published bytes must belong to one commit",
    dirty.split("\n").slice(0, 20),
  );
}
const commit = git("rev-parse", "HEAD");
const branch = git("rev-parse", "--abbrev-ref", "HEAD");
process.stdout.write(`source ${commit} (${branch})\n`);
if (git("rev-parse", "--is-shallow-repository") !== "false") {
  die(
    "the repository is shallow; complete schema identity history is required before publication",
  );
}
if (branch !== "main") {
  die("publication requires the protected main branch");
}
let originMain;
try {
  const remote = run(gitExecutable, [
    "ls-remote",
    "--exit-code",
    originURL,
    "refs/heads/main",
  ], { cwd: "/", environment: gitEnvironment }).trim();
  const fields = remote.split(/\s+/u);
  if (fields.length !== 2 || fields[1] !== "refs/heads/main") {
    throw new Error(`unexpected origin/main response ${JSON.stringify(remote)}`);
  }
  [originMain] = fields;
} catch (error) {
  die(`cannot read the canonical protected origin/main ref: ${error.message}`);
}
if (originMain !== commit) {
  die(
    `main HEAD ${commit} is not the current canonical origin/main ${originMain}`,
  );
}

let snapshot;
let committedPublicationManifest;
let verifiedAuthorityContentManifest;
let verifiedAuthorityManifest;
let verifiedPublicationManifest;
try {
  snapshot = createCommittedSnapshot({
    commit,
    environment: gitEnvironment,
    gitExecutable,
    repo,
    tarExecutable,
  });
  process.once("exit", () => snapshot.dispose());
  committedPublicationManifest = createCommittedPublicationManifest({
    commit,
    environment: gitEnvironment,
    gitExecutable,
    paths: VERIFIED_SNAPSHOT_PATHS,
    repo,
  });
  verifiedPublicationManifest = createPublicationManifest(
    snapshot.root,
    VERIFIED_SNAPSHOT_PATHS,
  );
  assertPublicationManifest(
    committedPublicationManifest,
    verifiedPublicationManifest,
  );
  assertCommittedGitAuthority({
    authorityRoot: snapshot.authorityRoot,
    commit,
    environment: gitEnvironment,
    gitExecutable,
  });
  verifiedAuthorityManifest = createPublicationManifest(
    snapshot.authorityRoot,
    ["."],
  );
  verifiedAuthorityContentManifest = createPublicationManifestFromEntries(
    snapshot.authorityRoot,
    committedPublicationManifest.entries,
  );
  assertPublicationManifest(
    committedPublicationManifest,
    verifiedAuthorityContentManifest,
  );
  parseWebsiteWranglerConfig(
    readFileSync(join(snapshot.root, SITE.config), "utf8"),
    {
      compatibilityDate: COMPATIBILITY_DATE,
      hostnames: HOSTNAMES,
      worker: SITE.worker,
    },
  );
} catch (error) {
  die(`cannot create the exact committed publication snapshot: ${error.message}`);
}
const publicationRepo = snapshot.root;
const authorityRepo = snapshot.authorityRoot;
const authorityGitRaw = (...args) =>
  execFileSync(
    gitExecutable,
    [
      "--no-replace-objects",
      "-c",
      "advice.graftFileDeprecated=false",
      "-c",
      "core.fsmonitor=false",
      "-c",
      "credential.helper=",
      "-c",
      "core.hooksPath=/dev/null",
      ...args,
    ],
    {
      cwd: authorityRepo,
      encoding: "utf8",
      env: gitEnvironment,
      maxBuffer: 64 * 1024 * 1024,
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
const authorityGit = (...args) => authorityGitRaw(...args).trim();
const authorityGateHome = resolve(publicationRepo, "..", "authority-gate-home");
const snapshotGateHome = resolve(publicationRepo, "..", "snapshot-gate-home");
mkdirSync(authorityGateHome, { mode: 0o700 });
mkdirSync(snapshotGateHome, { mode: 0o700 });
const authorityGateEnvironment = createHardenedGateEnvironment(
  process.env,
  process.execPath,
  authorityGateHome,
);
const snapshotGateEnvironment = createHardenedGateEnvironment(
  process.env,
  process.execPath,
  snapshotGateHome,
);
process.stdout.write(
  `publication snapshot ${verifiedPublicationManifest.entries.length} files sha256 ${verifiedPublicationManifest.sha256}\n`,
);
process.stdout.write(
  `Git authority snapshot ${verifiedAuthorityManifest.entries.length} files sha256 ${verifiedAuthorityManifest.sha256}\n`,
);

// The Go authority checks need complete Git history and immutable tag objects,
// so they run in a separate non-local clone frozen at the same exact commit.
// The archive remains free of Git metadata; only its static bytes reach the
// public-surface checks and Wrangler.
process.stdout.write(`\n==> bun run ${SITE.authorityGate}\n`);
try {
  process.stdout.write(
    run(
      process.execPath,
      ["--config=/dev/null", "--no-env-file", "run", SITE.authorityGate],
      {
        cwd: authorityRepo,
        environment: authorityGateEnvironment,
      },
    ),
  );
  assertCommittedGitAuthority({
    authorityRoot: authorityRepo,
    commit,
    environment: gitEnvironment,
    gitExecutable,
  });
  assertPublicationManifest(
    verifiedAuthorityManifest,
    createPublicationManifest(authorityRepo, ["."]),
  );
  assertPublicationManifest(
    committedPublicationManifest,
    createPublicationManifestFromEntries(
      authorityRepo,
      committedPublicationManifest.entries,
    ),
  );
} catch (error) {
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  die("the public authority gate failed before publication; production is unchanged");
}

process.stdout.write(`\n==> bun run ${SITE.snapshotGate}\n`);
try {
  process.stdout.write(
    run(
      process.execPath,
      ["--config=/dev/null", "--no-env-file", "run", SITE.snapshotGate],
      {
        cwd: publicationRepo,
        environment: snapshotGateEnvironment,
      },
    ),
  );
  assertPublicationManifest(
    verifiedPublicationManifest,
    createPublicationManifest(publicationRepo, VERIFIED_SNAPSHOT_PATHS),
  );
  assertPublicationManifest(
    verifiedAuthorityManifest,
    createPublicationManifest(authorityRepo, ["."]),
  );
  assertPublicationManifest(
    committedPublicationManifest,
    createPublicationManifestFromEntries(
      authorityRepo,
      committedPublicationManifest.entries,
    ),
  );
} catch (error) {
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  die("the public snapshot gate failed before publication; production is unchanged");
}

let schemaIdentities;
try {
  const ledgerObjectAt = (revision) => {
    const listing = authorityGitRaw(
      "ls-tree",
      "-z",
      "--full-tree",
      revision,
      "--",
      PUBLIC_SCHEMA_IDENTITY_LEDGER,
    );
    if (listing === "") return null;
    const match = new RegExp(
      `^100644 blob ([0-9a-f]{40})\\t${PUBLIC_SCHEMA_IDENTITY_LEDGER.replaceAll(".", "\\.")}\\u0000$`,
      "u",
    ).exec(listing);
    if (!match) {
      throw new Error(
        `${PUBLIC_SCHEMA_IDENTITY_LEDGER}@${revision} is not one exact regular Git blob`,
      );
    }
    return match[1];
  };
  if (ledgerObjectAt(commit) === null) {
    throw new Error(
      `${PUBLIC_SCHEMA_IDENTITY_LEDGER} is absent from captured commit ${commit}`,
    );
  }
  schemaIdentities = readPublicSchemaIdentityLedger(publicationRepo);
  const ledgerCommits = authorityGit(
    "log",
    "--full-history",
    "--format=%H",
    commit,
    "--",
    PUBLIC_SCHEMA_IDENTITY_LEDGER,
  )
    .split("\n")
    .filter((value) => value !== "");
  if (ledgerCommits.length === 0) {
    throw new Error("the identity ledger has no reachable committed history");
  }
  const historicalSets = [];
  for (const ledgerCommit of ledgerCommits) {
    const ledgerObject = ledgerObjectAt(ledgerCommit);
    // A deletion commit has no blob at this path. An earlier reachable
    // revision still carries the retained set and is checked below.
    if (ledgerObject === null) continue;
    const historical = authorityGitRaw("cat-file", "blob", ledgerObject);
    historicalSets.push({
      identities: parsePublicSchemaIdentityLedger(
        historical,
        `${PUBLIC_SCHEMA_IDENTITY_LEDGER}@${ledgerCommit}`,
      ),
      label: `${PUBLIC_SCHEMA_IDENTITY_LEDGER}@${ledgerCommit}`,
    });
  }
  enforceAppendOnlyPublicSchemaIdentities(
    schemaIdentities,
    historicalSets,
  );
  process.stdout.write(
    `schema identity ledger retains ${schemaIdentities.length} identities across ${historicalSets.length} committed revision(s)\n`,
  );
  assertCommittedGitAuthority({
    authorityRoot: authorityRepo,
    commit,
    environment: gitEnvironment,
    gitExecutable,
  });
  assertPublicationManifest(
    verifiedAuthorityManifest,
    createPublicationManifest(authorityRepo, ["."]),
  );
  assertPublicationManifest(
    verifiedAuthorityContentManifest,
    createPublicationManifestFromEntries(
      authorityRepo,
      committedPublicationManifest.entries,
    ),
  );
} catch (error) {
  die(`cannot prove append-only schema identity history: ${error.message}`);
}

const assetRoot = resolve(publicationRepo, SITE.assets);
if (!existsSync(join(assetRoot, "index.html"))) {
  die(`${SITE.assets}/index.html is missing`);
}

const CREDENTIAL_SHAPES = [
  /-----BEGIN (?:RSA |EC |OPENSSH |PGP )?PRIVATE KEY-----/u,
  /\bAKIA[0-9A-Z]{16}\b/u,
  /\bsk_live_[0-9A-Za-z]{16,}/u,
  /\bgh[pousr]_[0-9A-Za-z]{30,}/u,
];
let published;
try {
  published = collectRegularFiles(assetRoot);
} catch (error) {
  die(`the site asset tree is not publishable: ${error.message}`);
}
const leaks = [];
for (const path of published) {
  const name = relative(assetRoot, path);
  if (/(^|\/)\.env(\.|$)|\.pem$|\.p12$|\.pfx$|\.key$/u.test(name)) {
    leaks.push(`${name}: credential-shaped file`);
    continue;
  }
  if (/\.(?:png|jpe?g|webp|avif|gif|ico|woff2?|ttf|otf|mp4|pdf)$/u.test(name)) continue;
  for (const shape of CREDENTIAL_SHAPES) {
    if (shape.test(readFileSync(path, "utf8"))) leaks.push(`${name}: matches ${shape}`);
  }
}
if (leaks.length > 0) die("the site assets contain credential material", leaks);

const assetDigests = Object.fromEntries(
  published
    .map((path) => [
      relative(assetRoot, path).split("\\").join("/"),
      digest(readFileSync(path)),
    ])
    .sort(([left], [right]) =>
      left < right ? -1 : left > right ? 1 : 0,
    ),
);
const indexDigest = assetDigests["index.html"];
const publicSchemas = discoverPublicSchemas(publicationRepo);
const schemaDigests = Object.fromEntries(
  publicSchemas.map((schema) => [
    schema.id,
    digest(readFileSync(schema.publicPath)),
  ]),
);
const proveCandidateSchemasOnApex = async () => {
  const mismatches = [];
  for (const schema of publicSchemas) {
    const path = relative(assetRoot, schema.publicPath)
      .split("\\")
      .join("/");
    let served;
    try {
      served = await readPublishedDigest(`${SITE.url}/${path}`);
    } catch (error) {
      mismatches.push(`${path}: ${error.message}`);
      continue;
    }
    if (served !== schemaDigests[schema.id]) {
      mismatches.push(
        `${path}: served ${served}, expected ${schemaDigests[schema.id]}`,
      );
    }
  }
  if (mismatches.length > 0) {
    throw new Error(
      `apex does not serve the candidate schema closure: ${mismatches.join("; ")}`,
    );
  }
};
process.stdout.write(
  `\ncandidate ${published.length} files, index.html sha256 ${indexDigest.slice(0, 16)}, ${publicSchemas.length} normative schema URLs\n`,
);

// reversal: 戻し先の version を先に読む。読めなければ publish しない。
let previous = null;
let wranglerInstallation;
let runWrangler;
try {
  wranglerInstallation = createPinnedWranglerInstallation({
    bunExecutable: process.execPath,
    environment: gitEnvironment,
    snapshotRoot: publicationRepo,
  });
  process.once("exit", () => wranglerInstallation.dispose());
  runWrangler = (args, options = {}) => {
    const invocation = pinnedWranglerInvocation(
      wranglerInstallation.script,
      args,
      nodeExecutable,
    );
    return run(invocation.command, invocation.args, options);
  };
  const wranglerVersion = runWrangler(["--version"], {
    cwd: publicationRepo,
    environment: wranglerEnvironment,
  }).trim();
  if (wranglerVersion !== PINNED_WRANGLER_VERSION) {
    throw new Error(
      `local Wrangler is ${JSON.stringify(wranglerVersion)}, expected ${PINNED_WRANGLER_VERSION}`,
    );
  }
} catch (error) {
  die(
    `cannot use the repository-pinned Wrangler ${PINNED_WRANGLER_VERSION}: ${error.message}`,
  );
}
let cloudflareToken;
let cloudflareZone;
try {
  parseWranglerWhoami(
    runWrangler(["whoami", "--json"], {
      cwd: publicationRepo,
      environment: wranglerEnvironment,
    }),
    expectedCloudflare.accountId,
  );
  cloudflareToken = parseWranglerOAuthToken(
    runWrangler(["auth", "token", "--json"], {
      cwd: publicationRepo,
      environment: wranglerEnvironment,
    }),
  );
  const zoneQuery = new URLSearchParams({
    name: ZONE_NAME,
    status: "active",
  });
  cloudflareZone = parseCloudflareZone(
    await cloudflareRequest(
      cloudflareToken,
      `zones?${zoneQuery.toString()}`,
    ),
    {
      accountId: expectedCloudflare.accountId,
      zoneId: expectedCloudflare.zoneId,
      zoneName: ZONE_NAME,
    },
  );
} catch (error) {
  die(`cannot bind the explicit Cloudflare account and zone: ${error.message}`);
}
try {
  const status = runWrangler([
    "deployments",
    "status",
    "--name",
    SITE.worker,
    "--config",
    SITE.config,
    "--json",
  ], { cwd: publicationRepo, environment: wranglerEnvironment });
  previous = parseCurrentProductionDeployment(status);
} catch (error) {
  die(`cannot prove the current production deployment: ${error.message}`);
}
if (!previous) {
  die("no current production version was readable, so there is no revert point");
}
process.stdout.write(
  `previous deployment ${previous.deploymentId} version ${previous.versionId}\n`,
);

// published-identity / no-overwrite: compare production to the exact candidate
// immediately before mutation. The only exception is a wholly DNS-absent
// origin, and that first mint requires a narrowly named operator
// acknowledgement. A mismatch can never be acknowledged away.
let schemaPublicationPrecondition;
let schemaPublicationObservations;
try {
  schemaPublicationObservations = await inspectSchemaPublicationIdentities(
    publicSchemas.map((schema) => ({
      candidateBytes: readFileSync(schema.publicPath),
      id: schema.id,
    })),
  );
  schemaPublicationPrecondition = enforceSchemaPublicationNoOverwrite(
    schemaPublicationObservations,
    {
      initialOriginMintAcknowledged:
        acknowledgedInitialSchemaOriginMint,
    },
  );
} catch (error) {
  const recoveryCanPoll =
    recoveringInitialDomain &&
    Array.isArray(schemaPublicationObservations) &&
    schemaPublicationObservations.length === publicSchemas.length &&
    schemaPublicationObservations.every(
      ({ kind }) =>
        kind === "dns-not-found" ||
        kind === "http-error" ||
        kind === "match" ||
        kind === "transport-error",
    );
  if (!recoveryCanPoll) {
    die(
      `schema identity no-overwrite proof failed before publication; production is unchanged: ${error.message}`,
    );
  }
  schemaPublicationPrecondition = {
    count: schemaPublicationObservations.length,
    mode: "RECOVERY_PENDING_READBACK",
  };
}
process.stdout.write(
  `schema identity precondition ${schemaPublicationPrecondition.mode} (${schemaPublicationPrecondition.count} exact identities)\n`,
);

const domainOrigins = HOSTNAMES.map((hostname) => ({
  hostname,
  zone_id: cloudflareZone.zoneId,
  zone_name: cloudflareZone.zoneName,
}));
const domainInventoryPath =
  `accounts/${cloudflareZone.accountId}/workers/domains?` +
  new URLSearchParams({
    environment: "production",
    per_page: "100",
    service: SITE.worker,
  }).toString();
const domainChangesetPath =
  `accounts/${cloudflareZone.accountId}/workers/scripts/${SITE.worker}/` +
  "domains/changeset?replace_state=true";
const initialDomainMint =
  schemaPublicationPrecondition.mode ===
  "INITIAL_ORIGIN_MINT_ACKNOWLEDGED";
const readDomainPrecondition = async (
  mode = initialDomainMint ? "INITIAL" : "EXISTING",
) => {
  parseWorkerDomainClosure(
    await cloudflareRequest(cloudflareToken, domainInventoryPath),
    {
      expectedHostnames: mode === "INITIAL" ? APEX_HOSTNAMES : HOSTNAMES,
      service: SITE.worker,
      zoneId: cloudflareZone.zoneId,
      zoneName: cloudflareZone.zoneName,
    },
  );
  const precondition = parseDomainChangeset(
    await cloudflareRequest(cloudflareToken, domainChangesetPath, {
      body: domainOrigins,
      method: "POST",
    }),
    {
      existingHostnames: APEX_HOSTNAMES,
      expectedMode: mode,
      newHostname: SCHEMA_HOSTNAME,
      service: SITE.worker,
      zoneId: cloudflareZone.zoneId,
      zoneName: cloudflareZone.zoneName,
    },
  );
  let authoritativeAbsence = [];
  if (mode === "INITIAL") {
    authoritativeAbsence = await proveAuthoritativeHostnameAbsent({
      hostname: SCHEMA_HOSTNAME,
      nameServers: cloudflareZone.nameServers,
      zoneName: cloudflareZone.zoneName,
    });
  }
  return { authoritativeAbsence, precondition };
};
let domainPrecondition;
let authoritativeAbsence;
try {
  if (recoveringInitialDomain) {
    try {
      ({ authoritativeAbsence, precondition: domainPrecondition } =
        await readDomainPrecondition("INITIAL"));
    } catch {
      ({ authoritativeAbsence, precondition: domainPrecondition } =
        await readDomainPrecondition("EXISTING"));
    }
  } else {
    ({ authoritativeAbsence, precondition: domainPrecondition } =
      await readDomainPrecondition());
  }
} catch (error) {
  die(
    `Cloudflare custom-domain authority is not safe to mutate: ${error.message}`,
  );
}
process.stdout.write(
  `domain precondition ${domainPrecondition.mode}` +
    `${authoritativeAbsence.length > 0 ? ` (${authoritativeAbsence.length} authoritative ENOTFOUND responses)` : ""}\n`,
);

// Last mutation fence. Wrangler reads only the isolated archive, never the
// live worktree. Re-hash that archive and then re-prove both source authority
// and the production version immediately before invoking the first writer.
const assertSourceAndDeploymentFence = (expectedDeployment) => {
  const fencedManifest = createPublicationManifest(
    publicationRepo,
    VERIFIED_SNAPSHOT_PATHS,
  );
  assertPublicationManifest(verifiedPublicationManifest, fencedManifest);
  assertPublicationManifest(committedPublicationManifest, fencedManifest);
  assertCommittedGitAuthority({
    authorityRoot: authorityRepo,
    commit,
    environment: gitEnvironment,
    gitExecutable,
  });
  assertPublicationManifest(
    verifiedAuthorityManifest,
    createPublicationManifest(authorityRepo, ["."]),
  );
  assertPublicationManifest(
    verifiedAuthorityContentManifest,
    createPublicationManifestFromEntries(
      authorityRepo,
      committedPublicationManifest.entries,
    ),
  );

  const fencedResidue = inspectUncommittedPublicationPaths({
    environment: gitEnvironment,
    gitExecutable,
    paths: PUBLICATION_PATHS,
    repo,
  });
  if (
    fencedResidue.untracked.length > 0 ||
    fencedResidue.ignored.length > 0
  ) {
    throw new Error(
      "untracked or ignored files appeared below a publication path",
    );
  }
  assertRepositoryGitConfiguration();
  if (git("status", "--porcelain") !== "") {
    throw new Error("the live worktree changed after snapshot verification");
  }
  if (
    git("rev-parse", "HEAD") !== commit ||
    git("rev-parse", "--abbrev-ref", "HEAD") !== "main"
  ) {
    throw new Error("the protected main source changed after verification");
  }
  const fencedRemote = run(
    gitExecutable,
    [
      "ls-remote",
      "--exit-code",
      originURL,
      "refs/heads/main",
    ],
    { cwd: "/", environment: gitEnvironment },
  ).trim();
  const fencedRemoteFields = fencedRemote.split(/\s+/u);
  if (
    fencedRemoteFields.length !== 2 ||
    fencedRemoteFields[0] !== commit ||
    fencedRemoteFields[1] !== "refs/heads/main"
  ) {
    throw new Error("canonical origin/main changed after verification");
  }

  const current = parseCurrentProductionDeployment(
    runWrangler(
      [
        "deployments",
        "status",
        "--name",
        SITE.worker,
        "--config",
        SITE.config,
        "--json",
      ],
      { cwd: publicationRepo, environment: wranglerEnvironment },
    ),
  );
  if (
    current.deploymentId !== expectedDeployment.deploymentId ||
    current.versionId !== expectedDeployment.versionId
  ) {
    throw new Error(
      `production changed from ${expectedDeployment.deploymentId}/${expectedDeployment.versionId} to ${current.deploymentId}/${current.versionId}`,
    );
  }
};
try {
  assertSourceAndDeploymentFence(previous);
} catch (error) {
  die(`the final pre-mutation fence failed: ${error.message}`);
}

let uploadedVersion = null;
let currentDeployment;
if (recoveringInitialDomain) {
  if (
    previous.deploymentId !== expectedRecoveryDeployment ||
    previous.versionId !== expectedRecoveryVersion
  ) {
    die(
      `initial-domain recovery requires wholly absent schema DNS and current ${expectedRecoveryDeployment}/${expectedRecoveryVersion}; observed ${previous.deploymentId}/${previous.versionId}`,
    );
  }
  try {
    parseUploadedVersionResources(
      runWrangler(
        [
          "versions",
          "view",
          expectedRecoveryVersion,
          "--name",
          SITE.worker,
          "--config",
          SITE.config,
          "--json",
        ],
        { cwd: publicationRepo, environment: wranglerEnvironment },
      ),
      {
        compatibilityDate: COMPATIBILITY_DATE,
        expectedMessage: `takoform.com ${commit}`,
        versionId: expectedRecoveryVersion,
      },
    );
    await proveCandidateSchemasOnApex();
    let domainAlreadyAttached = false;
    await runFencedMutation({
      fence: () => assertSourceAndDeploymentFence(previous),
      remoteProof: async () => {
        try {
          await readDomainPrecondition("INITIAL");
        } catch (initialError) {
          try {
            await readDomainPrecondition("EXISTING");
            domainAlreadyAttached = true;
          } catch (existingError) {
            throw new Error(
              `neither absent nor exact-existing domain state was proven: ${initialError.message}; ${existingError.message}`,
            );
          }
        }
      },
      writer: () =>
        domainAlreadyAttached
          ? undefined
          : cloudflareRequest(
              cloudflareToken,
              `accounts/${cloudflareZone.accountId}/workers/scripts/${SITE.worker}/domains/records`,
              {
                body: safeDomainWriteBody({
                  hostnames: HOSTNAMES,
                  zoneId: cloudflareZone.zoneId,
                  zoneName: cloudflareZone.zoneName,
                }),
                method: "PUT",
              },
            ),
    });
    uploadedVersion = previous.versionId;
    currentDeployment = previous;
    process.stdout.write(
      `recovered initial domain for existing deployment ${currentDeployment.deploymentId}/${currentDeployment.versionId}; no Worker version was uploaded or deployed\n`,
    );
  } catch (error) {
    die(
      `initial schema-domain recovery remains indeterminate for ${previous.deploymentId}/${previous.versionId}: ${error.message}. Do not retry without re-reading domain closure.`,
    );
  }
} else {
process.stdout.write(
  `\n==> staging exact snapshot as a non-public ${SITE.worker} version\n`,
);
try {
  const uploadOutput = await runFencedMutation({
    fence: () => assertSourceAndDeploymentFence(previous),
    remoteProof: async () => {},
    writer: () =>
      runWrangler(
        [
          "versions",
          "upload",
          "--strict",
          "--name",
          SITE.worker,
          "--config",
          SITE.config,
          "--message",
          `takoform.com ${commit}`,
        ],
        { cwd: publicationRepo, environment: wranglerEnvironment },
      ),
  });
  uploadedVersion = parseUploadedVersionId(uploadOutput);
  parseUploadedVersionResources(
    runWrangler(
      [
        "versions",
        "view",
        uploadedVersion,
        "--name",
        SITE.worker,
        "--config",
        SITE.config,
        "--json",
      ],
      { cwd: publicationRepo, environment: wranglerEnvironment },
    ),
    {
      compatibilityDate: COMPATIBILITY_DATE,
      expectedMessage: `takoform.com ${commit}`,
      versionId: uploadedVersion,
    },
  );
} catch (error) {
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  die(
    `version staging or its post-upload fence failed. Production remains at ${previous.deploymentId}/${previous.versionId}. ` +
      `${uploadedVersion ? `Dormant uploaded version ${uploadedVersion} may remain. ` : ""}` +
      "Do not deploy that version manually.",
  );
}
process.stdout.write(`uploaded version ${uploadedVersion} (not public)\n`);

process.stdout.write(`\n==> deploying version ${uploadedVersion} at 100%\n`);
try {
  process.stdout.write(
    await runFencedMutation({
      fence: () => assertSourceAndDeploymentFence(previous),
      remoteProof: () => readDomainPrecondition(),
      writer: () =>
        runWrangler(
          [
            "versions",
            "deploy",
            `${uploadedVersion}@100%`,
            "--yes",
            "--name",
            SITE.worker,
            "--config",
            SITE.config,
            "--message",
            `takoform.com ${commit}`,
          ],
          { cwd: publicationRepo, environment: wranglerEnvironment },
        ),
    }),
  );
  currentDeployment = parseCurrentProductionDeployment(
    runWrangler(
      [
        "deployments",
        "status",
        "--name",
        SITE.worker,
        "--config",
        SITE.config,
        "--json",
      ],
      { cwd: publicationRepo, environment: wranglerEnvironment },
    ),
  );
  if (
    currentDeployment.versionId !== uploadedVersion ||
    currentDeployment.deploymentId === previous.deploymentId
  ) {
    throw new Error(
      `production status is ${currentDeployment.deploymentId}/${currentDeployment.versionId}`,
    );
  }
} catch (error) {
  let observed = "unreadable";
  try {
    const status = parseCurrentProductionDeployment(
      runWrangler(
        [
          "deployments",
          "status",
          "--name",
          SITE.worker,
          "--config",
          SITE.config,
          "--json",
        ],
        { cwd: publicationRepo, environment: wranglerEnvironment },
      ),
    );
    observed = `${status.deploymentId}/${status.versionId}`;
  } catch {
    // Preserve the primary error and fail closed on unreadable authority.
  }
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  die(
    `version deployment is indeterminate. Previous ${previous.deploymentId}/${previous.versionId}; uploaded ${uploadedVersion}; observed ${observed}. Do not retry or roll back blindly.`,
  );
}
process.stdout.write(
  `current deployment ${currentDeployment.deploymentId} version ${currentDeployment.versionId}\n`,
);

if (initialDomainMint) {
  try {
    await proveCandidateSchemasOnApex();
    await runFencedMutation({
      fence: () => assertSourceAndDeploymentFence(currentDeployment),
      remoteProof: () => readDomainPrecondition(),
      writer: () =>
        cloudflareRequest(
          cloudflareToken,
          `accounts/${cloudflareZone.accountId}/workers/scripts/${SITE.worker}/domains/records`,
          {
            body: safeDomainWriteBody({
              hostnames: HOSTNAMES,
              zoneId: cloudflareZone.zoneId,
              zoneName: cloudflareZone.zoneName,
            }),
            method: "PUT",
          },
        ),
    });
  } catch (error) {
    die(
      `initial schema-domain mint is indeterminate after deployment ${currentDeployment.deploymentId}/${uploadedVersion}: ${error.message}. Preserve the committed schema bytes and run only: ${initialDomainRecoveryCommand(currentDeployment.deploymentId, uploadedVersion)}`,
    );
  }
}
}

let domainRecords;
try {
  let lastDomainError;
  for (let attempt = 1; attempt <= 8; attempt += 1) {
    try {
      domainRecords = parseWorkerDomainClosure(
        await cloudflareRequest(cloudflareToken, domainInventoryPath),
        {
          expectedHostnames: HOSTNAMES,
          service: SITE.worker,
          zoneId: cloudflareZone.zoneId,
          zoneName: cloudflareZone.zoneName,
        },
      );
      break;
    } catch (error) {
      lastDomainError = error;
      if (attempt < 8) {
        await new Promise((wake) => setTimeout(wake, 2_000 * attempt));
      }
    }
  }
  if (!domainRecords) throw lastDomainError;
  for (const hostname of HOSTNAMES) {
    const hostnameInventory =
      `accounts/${cloudflareZone.accountId}/workers/domains?` +
      new URLSearchParams({ hostname, per_page: "100" }).toString();
    parseWorkerDomainClosure(
      await cloudflareRequest(cloudflareToken, hostnameInventory),
      {
        expectedHostnames: [hostname],
        service: SITE.worker,
        zoneId: cloudflareZone.zoneId,
        zoneName: cloudflareZone.zoneName,
      },
    );
  }
} catch (error) {
  const recovery =
    initialDomainMint || recoveringInitialDomain
      ? ` Run only: ${initialDomainRecoveryCommand(currentDeployment.deploymentId, currentDeployment.versionId)}`
      : "";
  die(
    `custom-domain closure is indeterminate after deployment ${currentDeployment.deploymentId}/${uploadedVersion}: ${error.message}.${recovery}`,
  );
}

const readbackTargets = [
  {
    digest: indexDigest,
    label: "index.html",
    status: 200,
    url: `${SITE.url}/`,
  },
  {
    digest: indexDigest,
    label: "www index.html",
    status: 200,
    url: "https://www.takoform.com/",
  },
  ...[
    ["docs/index.html", "/docs/"],
    ["spec/index.html", "/spec/"],
    ["ja/index.html", "/ja/"],
    ["styles.css", "/styles.css"],
    ["robots.txt", "/robots.txt"],
    ["sitemap.xml", "/sitemap.xml"],
    ["tako.png", "/tako.png"],
  ].map(([asset, path]) => ({
    digest: assetDigests[asset],
    label: asset,
    status: 200,
    url: `${SITE.url}${path}`,
  })),
  {
    digest: assetDigests["404.html"],
    label: "404.html",
    status: 404,
    url: `${SITE.url}/__takoform_release_readback_missing__`,
  },
  ...publicSchemas.map((schema) => ({
    digest: schemaDigests[schema.id],
    label: relative(assetRoot, schema.publicPath),
    status: 200,
    url: schema.id,
  })),
];
let mismatches = [];
for (let attempt = 1; attempt <= 8; attempt += 1) {
  const observed = await Promise.all(
    readbackTargets.map(async (target) => {
      try {
        return {
          ...target,
          served: await readPublishedDigest(target.url, {
            expectedStatus: target.status,
          }),
        };
      } catch (error) {
        return { ...target, served: `error: ${error.message}` };
      }
    }),
  );
  mismatches = observed.filter((target) => target.served !== target.digest);
  if (mismatches.length === 0) break;
  if (attempt < 8) await new Promise((wake) => setTimeout(wake, 3000 * attempt));
}
const ok = mismatches.length === 0;

const result = {
  kind: "takos.deploy-result@v1",
  surface: SITE.surface,
  target: `cloudflare-worker:${SITE.worker}`,
  commit,
  wranglerVersion: PINNED_WRANGLER_VERSION,
  cloudflareAccount: cloudflareZone.accountId,
  cloudflareZone: cloudflareZone.zoneId,
  assetDigests,
  indexDigest,
  files: published.length,
  schemaFiles: publicSchemas.length,
  schemaDigests,
  schemaPublicationPrecondition: schemaPublicationPrecondition.mode,
  previousDeployment: previous.deploymentId,
  previousVersion: previous.versionId,
  uploadedVersion,
  currentDeployment: currentDeployment.deploymentId,
  currentVersion: currentDeployment.versionId,
  domainRecords,
  publicationManifest: verifiedPublicationManifest.sha256,
  productionReadback: ok ? "EXPECTED_CANDIDATE" : "MISMATCH",
  status: ok ? "PUBLISHED" : "INDETERMINATE",
};
process.stdout.write(`\n${JSON.stringify(result, null, 2)}\n`);

if (!ok) {
  const detail = mismatches
    .map(
      ({ digest: expected, label, served, url }) =>
        `${label} (${url}) served ${served}, expected ${expected}`,
    )
    .join("; ");
  const recovery =
    initialDomainMint || recoveringInitialDomain
      ? `The initial schema origin may already be minted; preserve the candidate bytes and run only: ${initialDomainRecoveryCommand(currentDeployment.deploymentId, currentDeployment.versionId)}`
      : `Read \`wrangler deployments status --name ${SITE.worker} --json\` and compare against ${previous.deploymentId}/${previous.versionId}; restore it only after proving its schema bytes.`;
  process.stderr.write(
    `\nProduction readback mismatch: ${detail}. ${recovery} Do not retry blindly.\n`,
  );
  process.exit(1);
}

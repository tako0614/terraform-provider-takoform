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
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import process from "node:process";

import {
  collectRegularFiles,
  parseCurrentProductionDeployment,
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
  ADMISSION_SURFACE,
  isAdmissionSurface,
  runAdmissionSurface,
} from "./admission-deploy.mjs";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const SITE = {
  surface: "takoform-website",
  worker: "takoform-website",
  assets: "website/public",
  config: "website/wrangler.jsonc",
  gate: "check:public-surfaces",
  url: "https://takoform.com",
};

const CONTRACT = {
  kind: "takos.deploy-contract@v2",
  surfaces: [
    {
      surface: SITE.surface,
      target: `cloudflare-worker:${SITE.worker}`,
      covers: ["website/wrangler.jsonc"],
      requiresScripts: [SITE.gate],
      requiresTools: ["git", "bun", "wrangler"],
      requiresEnv: [],
      // 静的 asset だけを配る Worker で、durable state も server handler も
      // 持ちません。ただし schema $id は consumer が固定する公開 identity です。
      triggers: ["irreversible", "authority", "published-identity"],
      obligations: {
        provenance: `refuses a dirty or shallow worktree, requires main HEAD to equal a fresh read of the canonical origin/main ref, runs the narrow \`bun run ${SITE.gate}\` gate that validates the bilingual website, docs/spec navigation, canonical Form inventory and status claims, local links, anchors, the append-only public-schema identity ledger, and byte parity between normative schemas and their public $id paths, publishes ${SITE.assets} exactly as committed with no build step, scans every published asset for credential material, and records the commit and the published digests. The repository-wide \`bun run check\` remains separate handoff evidence because its Go and OpenTofu checks do not validate these static bytes.`,
        "post-conditions": `fetches ${SITE.url}/ and every normative schema $id under https://forms.takoform.com/schemas/, and requires each response to serve the exact digest that was published`,
        reversal: `the current version id is read and printed before publishing. A previous version may be restored with \`wrangler versions deploy <previous-id>@100%\` only after proving it still serves every already-minted schema $id byte-for-byte. The initial schema-origin mint has no schema-safe rollback to a version without those identities; repair it forward while preserving the minted bytes.`,
        "failure-handling":
          "prints the provider's own stdout and stderr, names whether the failure was before or after publication, and on a readback mismatch exits non-zero without retrying. After an initial schema-origin mint attempt it requires authoritative URL readback and forward repair that preserves any identity that became reachable.",
        "pre-mutation-proof": `before Wrangler can mutate the target, proves local source is the canonical protected main commit, runs the public-surface/schema ledger gate, reads the current ${SITE.worker} production deployment through the locally authenticated Cloudflare account, resolves and reads every schema identity with redirect and cache bypass protections, and accepts only an all-exact existing origin or an explicitly acknowledged wholly DNS-absent first mint`,
        "independent-review": "the TASK-0009 release-surface review independently checked the website, custom-domain, DNS, append-only schema identity, rollback, and live-readback boundary; the operator retains that review with the deploy result before the first origin mint",
        "no-overwrite": `requires the candidate ${PUBLIC_SCHEMA_IDENTITY_LEDGER} to retain every identity and digest recorded by every reachable historical ledger revision, then immediately before any Cloudflare mutation fetches every retained schema $id with cache bypass and requires its served bytes to equal the candidate exactly. A removed historical identity, differing body, HTTP response other than 200, non-DNS transport failure, redirect, or partially existing origin blocks publication. Only when every URL fails specifically with ENOTFOUND may an operator mint the origin by passing ${INITIAL_SCHEMA_ORIGIN_MINT_ACK}; the acknowledgement is rejected once any URL exists and never bypasses a mismatch.`,
      },
    },
    ...RELEASE_SURFACES,
    ADMISSION_SURFACE,
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
if (isAdmissionSurface(invocation[0])) {
  try {
    await runAdmissionSurface({
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
const unknownOptions = invocation.filter(
  (arg) => arg.startsWith("--") && arg !== INITIAL_SCHEMA_ORIGIN_MINT_ACK,
);
const requested = invocation.filter((arg) => !arg.startsWith("--"));
const known = CONTRACT.surfaces.map((entry) => entry.surface);
if (
  requested.length !== 1 ||
  !known.includes(requested[0]) ||
  unknownOptions.length > 0 ||
  invocation.filter((arg) => arg === INITIAL_SCHEMA_ORIGIN_MINT_ACK).length > 1
) {
  process.stderr.write(
    `usage: bun run deploy -- <surface> [${INITIAL_SCHEMA_ORIGIN_MINT_ACK}]\nknown surfaces: ${known.join(", ")}\n`,
  );
  process.exit(1);
}

function die(message, detail = []) {
  process.stderr.write(`deploy blocked: ${message}\n`);
  for (const line of detail) process.stderr.write(`- ${line}\n`);
  process.exit(1);
}

const git = (...args) =>
  execFileSync("git", args, { cwd: repo, encoding: "utf8" }).trim();

const run = (command, args) =>
  execFileSync(command, args, {
    cwd: repo,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 32 * 1024 * 1024,
  });

const digest = (bytes) => createHash("sha256").update(bytes).digest("hex");

// provenance: 公開バイト列を一つの commit に結び付ける。この site は build 段が
// ないので、公開されるのは commit されている bytes そのものです。
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
const originURL = git("remote", "get-url", "origin");
if (
  originURL !== "https://github.com/tako0614/terraform-provider-takoform.git" &&
  originURL !== "git@github.com:tako0614/terraform-provider-takoform.git"
) {
  die(`origin is not the canonical Takoform repository: ${originURL}`);
}
let originMain;
try {
  const remote = run("git", [
    "ls-remote",
    "--exit-code",
    "origin",
    "refs/heads/main",
  ]).trim();
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

// No build step: what is published is exactly what is committed. Validate the
// website/docs bytes and the canonical inventory/status claims they expose.
// The repository-wide owner gate remains separate handoff evidence: its Go and
// OpenTofu checks cannot fail because of these static files.
process.stdout.write(`\n==> bun run ${SITE.gate}\n`);
try {
  process.stdout.write(run("bun", ["run", SITE.gate]));
} catch (error) {
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  die("the public-surface gate failed before publication; production is unchanged");
}

let schemaIdentities;
try {
  git("ls-files", "--error-unmatch", PUBLIC_SCHEMA_IDENTITY_LEDGER);
  schemaIdentities = readPublicSchemaIdentityLedger(repo);
  const ledgerCommits = git(
    "log",
    "--format=%H",
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
    let historical;
    try {
      historical = git(
        "show",
        `${ledgerCommit}:${PUBLIC_SCHEMA_IDENTITY_LEDGER}`,
      );
    } catch {
      // A deletion commit has no blob at this path. An earlier reachable
      // revision still carries the retained set and is checked below.
      continue;
    }
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
} catch (error) {
  die(`cannot prove append-only schema identity history: ${error.message}`);
}

const assetRoot = resolve(repo, SITE.assets);
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

const indexDigest = digest(readFileSync(join(assetRoot, "index.html")));
const publicSchemas = discoverPublicSchemas(repo);
const schemaDigests = Object.fromEntries(
  publicSchemas.map((schema) => [
    schema.id,
    digest(readFileSync(schema.publicPath)),
  ]),
);
process.stdout.write(
  `\ncandidate ${published.length} files, index.html sha256 ${indexDigest.slice(0, 16)}, ${publicSchemas.length} normative schema URLs\n`,
);

// reversal: 戻し先の version を先に読む。読めなければ publish しない。
let previous = null;
try {
  const status = run("wrangler", [
    "deployments",
    "status",
    "--name",
    SITE.worker,
    "--config",
    SITE.config,
    "--json",
  ]);
  previous = parseCurrentProductionDeployment(status).versionId;
} catch (error) {
  die(`cannot prove the current production deployment: ${error.message}`);
}
if (!previous) {
  die("no current production version was readable, so there is no revert point");
}
process.stdout.write(`previous version ${previous}\n`);

// published-identity / no-overwrite: compare production to the exact candidate
// immediately before mutation. The only exception is a wholly DNS-absent
// origin, and that first mint requires a narrowly named operator
// acknowledgement. A mismatch can never be acknowledged away.
let schemaPublicationPrecondition;
try {
  const observations = await inspectSchemaPublicationIdentities(
    publicSchemas.map((schema) => ({
      candidateBytes: readFileSync(schema.publicPath),
      id: schema.id,
    })),
  );
  schemaPublicationPrecondition = enforceSchemaPublicationNoOverwrite(
    observations,
    {
      initialOriginMintAcknowledged:
        acknowledgedInitialSchemaOriginMint,
    },
  );
} catch (error) {
  die(
    `schema identity no-overwrite proof failed before publication; production is unchanged: ${error.message}`,
  );
}
process.stdout.write(
  `schema identity precondition ${schemaPublicationPrecondition.mode} (${schemaPublicationPrecondition.count} exact identities)\n`,
);

process.stdout.write(`\n==> publishing ${SITE.assets} to ${SITE.worker}\n`);
let output;
try {
  output = run("wrangler", ["deploy", "--config", SITE.config]);
} catch (error) {
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  const recovery =
    schemaPublicationPrecondition.mode ===
    "INITIAL_ORIGIN_MINT_ACKNOWLEDGED"
      ? "The schema origin may now be minted. Read every schema URL authoritatively and repair forward without changing or removing any candidate schema bytes."
      : `Reconcile against version ${previous}; restore it only after confirming that it serves every schema identity byte-for-byte.`;
  die(
    "publication failed; production may be unchanged or partially updated. " +
      `${recovery} Do not retry blindly.`,
  );
}
process.stdout.write(output);

const readbackTargets = [
  {
    digest: indexDigest,
    label: "index.html",
    url: `${SITE.url}/`,
  },
  ...publicSchemas.map((schema) => ({
    digest: schemaDigests[schema.id],
    label: relative(assetRoot, schema.publicPath),
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
          served: await readPublishedDigest(target.url),
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
  indexDigest,
  files: published.length,
  schemaFiles: publicSchemas.length,
  schemaDigests,
  schemaPublicationPrecondition: schemaPublicationPrecondition.mode,
  previousVersion: previous,
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
    schemaPublicationPrecondition.mode ===
    "INITIAL_ORIGIN_MINT_ACKNOWLEDGED"
      ? "The initial schema origin may already be minted; inspect every public schema identity and repair forward while preserving the candidate bytes."
      : `Read \`wrangler deployments status --name ${SITE.worker} --json\` and compare against ${previous}; restore it only after proving its schema bytes.`;
  process.stderr.write(
    `\nProduction readback mismatch: ${detail}. ${recovery} Do not retry blindly.\n`,
  );
  process.exit(1);
}

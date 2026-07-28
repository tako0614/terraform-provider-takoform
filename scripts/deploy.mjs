#!/usr/bin/env bun

// takoform の唯一の deploy entrypoint です。
//
// 共通の obligation と trigger は takos-control の
// `engineering.policy.json` → `deploy` が正本です。
//
//   bun run deploy -- website
//
// `--contract` は副作用なしで、この repo が publish できる surface と、それぞれの
// trigger・義務の果たし方を印字します。
//
// provider artifacts と Form Package はまだこの entrypoint に載っていません。
// どちらも `published-identity` で、署名鍵と transparency log を伴います。
// 現状の候補ビルダーは release/README.md が持ちます。

import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import process from "node:process";

const repo = resolve(dirname(fileURLToPath(import.meta.url)), "..");

const SITE = {
  surface: "takoform-website",
  worker: "takoform-website",
  assets: "website/public",
  config: "website/wrangler.jsonc",
  url: "https://takoform.com",
};

const CONTRACT = {
  kind: "takos.deploy-contract@v2",
  surfaces: [
    {
      surface: SITE.surface,
      target: `cloudflare-worker:${SITE.worker}`,
      covers: ["website/wrangler.jsonc"],
      requiresScripts: [],
      requiresTools: ["git", "bun", "wrangler"],
      requiresEnv: [],
      // 静的 asset だけを配る Worker です。durable state も server handler も
      // 持たず、消費者が pin する identity も発行しません。
      triggers: [],
      obligations: {
        provenance: `refuses a dirty worktree, publishes ${SITE.assets} exactly as committed with no build step, scans it for credential material, and records the commit and the index.html sha256. It deliberately does not run the repository-wide gate: this surface is prerendered HTML, and the Go provider tests, gofmt, and \`tofu fmt\` say nothing about it — requiring them would make publishing a page depend on a Terraform toolchain being installed.`,
        "post-conditions": `fetches ${SITE.url}/ and requires it to serve the exact index.html digest that was published`,
        reversal: `the current version id is read and printed before publishing; restore it with \`wrangler versions list --name ${SITE.worker}\` and \`wrangler versions deploy <previous-id>@100%\``,
        "failure-handling":
          "prints the provider's own stdout and stderr, names whether the failure was before or after publication, and on a readback mismatch exits non-zero naming the previous version instead of retrying",
      },
    },
  ],
};

if (process.argv.includes("--contract")) {
  process.stdout.write(`${JSON.stringify(CONTRACT, null, 2)}\n`);
  process.exit(0);
}

const requested = process.argv.slice(2).filter((arg) => !arg.startsWith("--"));
const known = CONTRACT.surfaces.map((entry) => entry.surface);
if (requested.length !== 1 || !known.includes(requested[0])) {
  process.stderr.write(
    `usage: bun run deploy -- <surface>\nknown surfaces: ${known.join(", ")}\n`,
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

function walk(directory) {
  const found = [];
  for (const entry of readdirSync(directory)) {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) found.push(...walk(path));
    else found.push(path);
  }
  return found;
}

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
process.stdout.write(`source ${commit} (${git("rev-parse", "--abbrev-ref", "HEAD")})\n`);

// No build step: what is published is exactly what is committed. The checks that
// could fail because of these bytes are the ones below — the asset exists, and it
// carries no credential material. The repository-wide `bun run check` covers Go,
// gofmt, and OpenTofu, none of which can be broken by a page in website/public.
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
const published = walk(assetRoot);
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
process.stdout.write(
  `\ncandidate ${published.length} files, index.html sha256 ${indexDigest.slice(0, 16)}\n`,
);

// reversal: 戻し先の version を先に読む。読めなければ publish しない。
let previous = null;
try {
  const listed = run("wrangler", ["versions", "list", "--name", SITE.worker, "--config", SITE.config]);
  previous =
    listed.match(/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/u)?.[1] ?? null;
} catch (error) {
  die(`cannot read the current version list: ${error.message}`);
}
if (!previous) {
  die("no current version was readable, so there is no revert point");
}
process.stdout.write(`previous version ${previous}\n`);

process.stdout.write(`\n==> publishing ${SITE.assets} to ${SITE.worker}\n`);
let output;
try {
  output = run("wrangler", ["deploy", "--config", SITE.config]);
} catch (error) {
  process.stderr.write(`${error.stdout ?? ""}${error.stderr ?? ""}\n`);
  die(
    "publication failed; production may be unchanged or partially updated. " +
      `Reconcile against version ${previous} before retrying.`,
  );
}
process.stdout.write(output);

// post-conditions: 実際に配られているバイト列が、いま publish したものであること。
async function servedDigest(url) {
  const response = await fetch(url, {
    headers: { "cache-control": "no-cache" },
    redirect: "follow",
  });
  if (!response.ok) throw new Error(`${url} responded ${response.status}`);
  return digest(Buffer.from(await response.arrayBuffer()));
}

let served = null;
let ok = false;
for (let attempt = 1; attempt <= 8; attempt += 1) {
  try {
    served = await servedDigest(`${SITE.url}/`);
    if (served === indexDigest) {
      ok = true;
      break;
    }
  } catch (error) {
    served = `error: ${error.message}`;
  }
  if (attempt < 8) await new Promise((wake) => setTimeout(wake, 3000 * attempt));
}

const result = {
  kind: "takos.deploy-result@v1",
  surface: SITE.surface,
  target: `cloudflare-worker:${SITE.worker}`,
  commit,
  indexDigest,
  files: published.length,
  previousVersion: previous,
  productionReadback: ok ? "EXPECTED_CANDIDATE" : "MISMATCH",
  status: ok ? "PUBLISHED" : "INDETERMINATE",
};
process.stdout.write(`\n${JSON.stringify(result, null, 2)}\n`);

if (!ok) {
  process.stderr.write(
    `\n${SITE.url}/ served ${served} instead of ${indexDigest}. ` +
      `Do not retry blindly: read \`wrangler versions list --name ${SITE.worker}\` and compare against ${previous} first.\n`,
  );
  process.exit(1);
}

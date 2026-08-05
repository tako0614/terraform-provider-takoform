import { Resolver, resolve4, resolveNs } from "node:dns/promises";

const HEX_ID = /^[0-9a-f]{32}$/u;
const UUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;

export const CLOUDFLARE_ACCOUNT_ENV = "TAKOFORM_CLOUDFLARE_ACCOUNT_ID";
export const CLOUDFLARE_ZONE_ENV = "TAKOFORM_CLOUDFLARE_ZONE_ID";
export const EXCLUSIVE_WRITER_ACK =
  "--acknowledge-exclusive-cloudflare-writer";

const RUNTIME_INJECTION_ENV = new Set([
  "ALL_PROXY",
  "BUN_OPTIONS",
  "DYLD_INSERT_LIBRARIES",
  "DYLD_LIBRARY_PATH",
  "GCONV_PATH",
  "GLIBC_TUNABLES",
  "HTTPS_PROXY",
  "HTTP_PROXY",
  "LD_LIBRARY_PATH",
  "LD_PRELOAD",
  "LOCPATH",
  "NLSPATH",
  "NODE_EXTRA_CA_CERTS",
  "NODE_OPTIONS",
  "SSLKEYLOGFILE",
  "all_proxy",
  "https_proxy",
  "http_proxy",
]);

function parseJson(raw, label) {
  let value;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`${label} was not JSON: ${error.message}`);
  }
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} was not a JSON object`);
  }
  return value;
}

function exactStrings(values, expected) {
  if (!Array.isArray(values)) return false;
  const actual = [...values].sort();
  const wanted = [...expected].sort();
  return JSON.stringify(actual) === JSON.stringify(wanted);
}

function exactKeys(value, expected) {
  return (
    value !== null &&
    typeof value === "object" &&
    !Array.isArray(value) &&
    exactStrings(Object.keys(value), expected)
  );
}

function envelope(raw, label) {
  const parsed = typeof raw === "string" ? parseJson(raw, label) : raw;
  if (
    parsed === null ||
    typeof parsed !== "object" ||
    parsed.success !== true ||
    !("result" in parsed)
  ) {
    throw new Error(`${label} was not a successful Cloudflare API response`);
  }
  return parsed;
}

export function readExpectedCloudflareIdentity(environment = process.env) {
  const accountId = environment[CLOUDFLARE_ACCOUNT_ENV];
  const zoneId = environment[CLOUDFLARE_ZONE_ENV];
  if (!HEX_ID.test(accountId ?? "")) {
    throw new Error(
      `${CLOUDFLARE_ACCOUNT_ENV} must be the exact 32-character lowercase account id`,
    );
  }
  if (!HEX_ID.test(zoneId ?? "")) {
    throw new Error(
      `${CLOUDFLARE_ZONE_ENV} must be the exact 32-character lowercase zone id`,
    );
  }
  if (accountId === zoneId) {
    throw new Error("Cloudflare account and zone ids must be distinct");
  }
  return { accountId, zoneId };
}

export function parseWebsiteWranglerConfig(
  raw,
  { compatibilityDate, hostnames, worker },
) {
  const config = parseJson(raw, "website Wrangler config");
  if (
    !exactKeys(config, [
      "$schema",
      "assets",
      "compatibility_date",
      "name",
      "routes",
    ]) ||
    config.$schema !== "https://unpkg.com/wrangler/config-schema.json" ||
    config.name !== worker ||
    config.compatibility_date !== compatibilityDate ||
    !exactKeys(config.assets, [
      "directory",
      "html_handling",
      "not_found_handling",
    ]) ||
    config.assets.directory !== "./public" ||
    config.assets.html_handling !== "auto-trailing-slash" ||
    config.assets.not_found_handling !== "404-page" ||
    !Array.isArray(config.routes) ||
    config.routes.length !== hostnames.length
  ) {
    throw new Error(
      "website Wrangler config is not the exact static-only deployment closure",
    );
  }
  const routes = config.routes.map((route) => {
    if (
      !exactKeys(route, ["custom_domain", "pattern"]) ||
      route.custom_domain !== true ||
      typeof route.pattern !== "string"
    ) {
      throw new Error(
        "website Wrangler config has a non-canonical custom-domain route",
      );
    }
    return route.pattern;
  });
  if (!exactStrings(routes, hostnames)) {
    throw new Error(
      "website Wrangler config does not contain the exact domain closure",
    );
  }
  return {
    compatibilityDate,
    hostnames: [...routes].sort(),
    worker,
  };
}

export function createPinnedWranglerEnvironment(
  environment,
  { accountId, bunLocalPrefix, bunUserAgent },
) {
  const forbidden = [];
  const cleaned = {};
  for (const [name, value] of Object.entries(environment)) {
    const exactBunRunMarker =
      (name === "npm_config_local_prefix" &&
        typeof bunLocalPrefix === "string" &&
        value === bunLocalPrefix) ||
      (name === "npm_config_user_agent" &&
        typeof bunUserAgent === "string" &&
        value === bunUserAgent);
    if (exactBunRunMarker) {
      continue;
    }
    if (
      name.startsWith("BUN_") ||
      name.startsWith("CF_") ||
      name.startsWith("CLOUDFLARE_") ||
      name.startsWith("DYLD_") ||
      name.startsWith("LD_") ||
      name.startsWith("NODE_") ||
      name.startsWith("NPM_CONFIG_") ||
      name.startsWith("OPENSSL_") ||
      name.startsWith("SSL_CERT_") ||
      name.startsWith("WRANGLER_") ||
      name.startsWith("npm_config_") ||
      RUNTIME_INJECTION_ENV.has(name)
    ) {
      forbidden.push(name);
      continue;
    }
    cleaned[name] = value;
  }
  if (forbidden.length > 0) {
    throw new Error(
      `ambient Cloudflare/Wrangler authority is forbidden: ${forbidden.sort().join(", ")}`,
    );
  }
  return {
    ...cleaned,
    CLOUDFLARE_ACCOUNT_ID: accountId,
  };
}

export function parseWranglerWhoami(raw, expectedAccountId) {
  const whoami = parseJson(raw, "Wrangler identity");
  if (whoami.loggedIn !== true || whoami.authType !== "OAuth Token") {
    throw new Error("Wrangler must be logged in with its local OAuth profile");
  }
  if (!Array.isArray(whoami.accounts)) {
    throw new Error("Wrangler identity has no account inventory");
  }
  const matches = whoami.accounts.filter(
    (account) => account?.id === expectedAccountId,
  );
  if (matches.length !== 1) {
    throw new Error(
      `Wrangler OAuth identity does not expose exactly the expected account ${expectedAccountId}`,
    );
  }
  const requiredPermissions = [
    "account:read",
    "workers_routes:write",
    "workers_scripts:write",
    "zone:read",
  ];
  const permissions = whoami.tokenPermissions;
  const missing = requiredPermissions.filter(
    (permission) => !permissions?.includes(permission),
  );
  if (missing.length > 0) {
    throw new Error(
      `Wrangler OAuth identity lacks required permissions: ${missing.join(", ")}`,
    );
  }
  return { accountId: expectedAccountId, authType: "oauth" };
}

export function parseWranglerOAuthToken(raw) {
  const authority = parseJson(raw, "Wrangler OAuth authority");
  if (
    authority.type !== "oauth" ||
    typeof authority.token !== "string" ||
    authority.token.length < 40 ||
    /\s/u.test(authority.token)
  ) {
    throw new Error("Wrangler did not return one usable OAuth token");
  }
  return authority.token;
}

export function parseCloudflareZone(
  raw,
  { accountId, zoneId, zoneName },
) {
  const response = envelope(raw, "Cloudflare zone lookup");
  if (
    !Array.isArray(response.result) ||
    response.result.length !== 1
  ) {
    throw new Error("Cloudflare zone lookup was absent or ambiguous");
  }
  const [zone] = response.result;
  if (
    zone?.id !== zoneId ||
    zone?.name !== zoneName ||
    zone?.status !== "active" ||
    zone?.account?.id !== accountId ||
    !Array.isArray(zone.name_servers) ||
    zone.name_servers.length < 2 ||
    zone.name_servers.some(
      (server) =>
        typeof server !== "string" ||
        server === "" ||
        server !== server.toLowerCase(),
    )
  ) {
    throw new Error(
      "Cloudflare zone does not match the explicit account, zone, name, active status, and authoritative nameservers",
    );
  }
  return {
    accountId,
    nameServers: [...zone.name_servers].sort(),
    zoneId,
    zoneName,
  };
}

function validateDomain(
  domain,
  { environment, hostname, service, zoneId, zoneName },
) {
  return (
    domain !== null &&
    typeof domain === "object" &&
    domain.hostname === hostname &&
    domain.zone_id === zoneId &&
    domain.zone_name === zoneName &&
    domain.service === service &&
    domain.environment === environment &&
    domain.enabled === true &&
    domain.previews_enabled === false
  );
}

export function parseWorkerDomainClosure(
  raw,
  {
    expectedHostnames,
    service,
    zoneId,
    zoneName,
  },
) {
  const response = envelope(raw, "Cloudflare Worker domain inventory");
  if (!Array.isArray(response.result)) {
    throw new Error("Cloudflare Worker domain inventory was not an array");
  }
  const information = response.result_info;
  if (
    information?.page !== 1 ||
    information?.per_page !== 100 ||
    information?.count !== response.result.length ||
    information?.total_count !== response.result.length
  ) {
    throw new Error(
      "Cloudflare Worker domain inventory is paginated or count-ambiguous",
    );
  }
  const actualHostnames = response.result.map((domain) => domain?.hostname);
  if (!exactStrings(actualHostnames, expectedHostnames)) {
    throw new Error(
      `Cloudflare Worker domain closure differs: ${actualHostnames.join(", ")}`,
    );
  }
  for (const hostname of expectedHostnames) {
    const matches = response.result.filter(
      (domain) =>
        validateDomain(domain, {
          environment: "production",
          hostname,
          service,
          zoneId,
          zoneName,
        }),
    );
    if (matches.length !== 1) {
      throw new Error(
        `Cloudflare Worker domain ${hostname} is not one exact enabled production binding`,
      );
    }
  }
  return response.result
    .map(({ hostname, id }) => ({ hostname, id }))
    .sort((left, right) =>
      left.hostname < right.hostname ? -1 : left.hostname > right.hostname ? 1 : 0,
    );
}

export function parseDomainChangeset(
  raw,
  {
    expectedMode,
    existingHostnames,
    newHostname,
    service,
    zoneId,
    zoneName,
  },
) {
  const response = envelope(raw, "Cloudflare Worker domain changeset");
  const changeset = response.result;
  if (
    changeset === null ||
    typeof changeset !== "object" ||
    !Array.isArray(changeset.added) ||
    !Array.isArray(changeset.updated) ||
    !Array.isArray(changeset.removed) ||
    !Array.isArray(changeset.conflicting) ||
    !Array.isArray(changeset.affected_zones)
  ) {
    throw new Error("Cloudflare Worker domain changeset has an invalid shape");
  }
  if (
    changeset.removed.length !== 0 ||
    changeset.conflicting.length !== 0 ||
    !exactStrings(changeset.affected_zones, [zoneId])
  ) {
    throw new Error(
      "Cloudflare Worker domain changeset removes or conflicts with existing authority",
    );
  }

  const unchanged = (hostname) =>
    changeset.updated.filter(
      (domain) =>
        domain.modified === false &&
        validateDomain(domain, {
          environment: "",
          hostname,
          service,
          zoneId,
          zoneName,
        }),
    ).length === 1;

  if (expectedMode === "INITIAL") {
    if (
      !exactStrings(
        changeset.updated.map((domain) => domain?.hostname),
        existingHostnames,
      ) ||
      existingHostnames.some((hostname) => !unchanged(hostname)) ||
      changeset.added.length !== 1 ||
      !validateDomain(changeset.added[0], {
        environment: "",
        hostname: newHostname,
        service,
        zoneId,
        zoneName,
      })
    ) {
      throw new Error(
        "Cloudflare Worker domain changeset is not the exact initial-hostname addition",
      );
    }
    return { mode: "INITIAL_DOMAIN_ABSENT" };
  }
  if (expectedMode === "EXISTING") {
    const allHostnames = [...existingHostnames, newHostname];
    if (
      changeset.added.length !== 0 ||
      !exactStrings(
        changeset.updated.map((domain) => domain?.hostname),
        allHostnames,
      ) ||
      allHostnames.some((hostname) => !unchanged(hostname))
    ) {
      throw new Error(
        "Cloudflare Worker domain changeset is not an unchanged existing closure",
      );
    }
    return { mode: "EXISTING_DOMAIN_UNCHANGED" };
  }
  throw new Error(`unknown domain changeset mode ${expectedMode}`);
}

export function parseUploadedVersionId(raw) {
  const matches = [
    ...raw.matchAll(
      /(?:^|\n)Worker Version ID: ([0-9a-f-]{36})(?=\n|$)/gu,
    ),
  ];
  if (matches.length !== 1 || !UUID.test(matches[0][1])) {
    throw new Error("Wrangler upload did not report one exact Worker Version ID");
  }
  return matches[0][1];
}

function parseStaticOnlyVersion(raw, { compatibilityDate, versionId }) {
  const version = parseJson(raw, "uploaded Worker version");
  const runtime = version.resources?.script_runtime;
  if (
    version.id !== versionId ||
    version.annotations?.["workers/triggered_by"] !== "version_upload" ||
    runtime?.compatibility_date !== compatibilityDate ||
    runtime?.assets?.html_handling !== "auto-trailing-slash" ||
    runtime?.assets?.not_found_handling !== "404-page" ||
    runtime?.assets?.serve_directly !== true ||
    runtime?.assets?.raw_run_worker_first !== false ||
    !Array.isArray(version.resources?.bindings) ||
    version.resources.bindings.length !== 0
  ) {
    throw new Error(
      "uploaded Worker version does not have the exact static-only runtime closure",
    );
  }
  return version;
}

export function parseUploadedVersionResources(
  raw,
  { compatibilityDate, expectedMessage, versionId },
) {
  const version = parseStaticOnlyVersion(raw, {
    compatibilityDate,
    versionId,
  });
  if (version.annotations?.["workers/message"] !== expectedMessage) {
    throw new Error(
      "uploaded Worker version does not have the exact static-only runtime closure",
    );
  }
  return { versionId };
}

export function parsePublishedVersionSourceCommit(
  raw,
  { compatibilityDate, versionId },
) {
  const version = parseStaticOnlyVersion(raw, {
    compatibilityDate,
    versionId,
  });
  const match = /^takoform\.com ([0-9a-f]{40})$/u.exec(
    version.annotations?.["workers/message"] ?? "",
  );
  if (!match) {
    throw new Error(
      "published Worker version does not identify one exact source commit",
    );
  }
  return { sourceCommit: match[1], versionId };
}

export function safeDomainWriteBody({
  hostnames,
  zoneId,
  zoneName,
}) {
  return {
    override_scope: true,
    override_existing_origin: false,
    override_existing_dns_record: false,
    origins: hostnames.map((hostname) => ({
      hostname,
      zone_id: zoneId,
      zone_name: zoneName,
    })),
  };
}

export async function runFencedMutation({
  fence,
  remoteProof,
  writer,
}) {
  await remoteProof();
  fence();
  return await writer();
}

function hasCode(error, code) {
  return (
    error !== null &&
    typeof error === "object" &&
    (error.code === code || hasCode(error.cause, code))
  );
}

export async function proveAuthoritativeHostnameAbsent(
  { hostname, nameServers, zoneName },
  {
    createResolver = () => new Resolver(),
    resolve4Impl = resolve4,
    resolveNsImpl = resolveNs,
  } = {},
) {
  const delegated = (await resolveNsImpl(zoneName))
    .map((server) => server.toLowerCase())
    .sort();
  if (!exactStrings(delegated, nameServers)) {
    throw new Error(
      "system DNS delegation does not match the Cloudflare zone authority",
    );
  }

  const observations = [];
  for (const nameServer of nameServers) {
    const addresses = await resolve4Impl(nameServer);
    if (!Array.isArray(addresses) || addresses.length === 0) {
      throw new Error(`authoritative nameserver ${nameServer} has no IPv4 address`);
    }
    const resolver = createResolver();
    resolver.setServers([addresses[0]]);
    try {
      const records = await resolver.resolve4(hostname);
      throw new Error(
        `authoritative nameserver ${nameServer} already answers ${hostname}: ${records.join(", ")}`,
      );
    } catch (error) {
      if (!hasCode(error, "ENOTFOUND")) throw error;
      observations.push({ nameServer, result: "ENOTFOUND" });
    }
  }
  return observations;
}

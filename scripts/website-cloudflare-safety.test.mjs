import { describe, expect, test } from "bun:test";

import {
  createPinnedWranglerEnvironment,
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

const accountId = "a".repeat(32);
const zoneId = "b".repeat(32);
const zoneName = "takoform.com";
const service = "takoform-website";
const apexHostnames = ["takoform.com", "www.takoform.com"];
const allHostnames = [...apexHostnames, "forms.takoform.com"];
const compatibilityDate = "2026-07-01";

const domain = (hostname, environment = "production") => ({
  enabled: true,
  environment,
  hostname,
  id: hostname.replaceAll(".", ""),
  previews_enabled: false,
  service,
  zone_id: zoneId,
  zone_name: zoneName,
});

const response = (result, resultInfo) => ({
  errors: [],
  messages: [],
  result,
  ...(resultInfo ? { result_info: resultInfo } : {}),
  success: true,
});

describe("explicit Cloudflare authority", () => {
  test("requires exact operator-private account and zone ids", () => {
    expect(
      readExpectedCloudflareIdentity({
        TAKOFORM_CLOUDFLARE_ACCOUNT_ID: accountId,
        TAKOFORM_CLOUDFLARE_ZONE_ID: zoneId,
      }),
    ).toEqual({ accountId, zoneId });
    expect(() => readExpectedCloudflareIdentity({})).toThrow(
      "TAKOFORM_CLOUDFLARE_ACCOUNT_ID",
    );
    expect(() =>
      readExpectedCloudflareIdentity({
        TAKOFORM_CLOUDFLARE_ACCOUNT_ID: `${accountId}\n`,
        TAKOFORM_CLOUDFLARE_ZONE_ID: zoneId,
      }),
    ).toThrow("exact 32-character");
  });

  test("rejects ambient authority and pins the expected account", () => {
    expect(() =>
      createPinnedWranglerEnvironment(
        {
          BUN_CONFIG_FILE: "/tmp/attacker.toml",
          CLOUDFLARE_API_BASE_URL: "https://attacker.invalid",
          CLOUDFLARE_API_TOKEN: "attacker",
          NODE_EXTRA_CA_CERTS: "/tmp/attacker.pem",
          PATH: "/usr/bin",
          WRANGLER_CI_OVERRIDE_NAME: "other-worker",
          npm_config_userconfig: "/tmp/attacker-npmrc",
        },
        { accountId },
      ),
    ).toThrow(
      "BUN_CONFIG_FILE, CLOUDFLARE_API_BASE_URL, CLOUDFLARE_API_TOKEN, NODE_EXTRA_CA_CERTS, WRANGLER_CI_OVERRIDE_NAME, npm_config_userconfig",
    );
    expect(
      createPinnedWranglerEnvironment({ PATH: "/usr/bin" }, { accountId }),
    ).toEqual({
      CLOUDFLARE_ACCOUNT_ID: accountId,
      PATH: "/usr/bin",
    });
  });

  test("accepts and removes only the exact inert bun run markers", () => {
    const bunLocalPrefix = "/reviewed/takoform";
    const bunUserAgent = "bun/1.3.14 npm/? node/v24.3.0 linux x64";
    expect(
      createPinnedWranglerEnvironment(
        {
          PATH: "/usr/bin",
          npm_config_local_prefix: bunLocalPrefix,
          npm_config_user_agent: bunUserAgent,
        },
        { accountId, bunLocalPrefix, bunUserAgent },
      ),
    ).toEqual({
      CLOUDFLARE_ACCOUNT_ID: accountId,
      PATH: "/usr/bin",
    });
    expect(() =>
      createPinnedWranglerEnvironment(
        {
          npm_config_local_prefix: "/different/repository",
          npm_config_user_agent: bunUserAgent,
        },
        { accountId, bunLocalPrefix, bunUserAgent },
      ),
    ).toThrow("npm_config_local_prefix");
    expect(() =>
      createPinnedWranglerEnvironment(
        {
          npm_config_local_prefix: bunLocalPrefix,
          npm_config_user_agent: `${bunUserAgent} attacker`,
        },
        { accountId, bunLocalPrefix, bunUserAgent },
      ),
    ).toThrow("npm_config_user_agent");
  });

  test("binds one OAuth profile account and exact permissions", () => {
    const whoami = {
      accounts: [{ id: accountId }, { id: "c".repeat(32) }],
      authType: "OAuth Token",
      loggedIn: true,
      tokenPermissions: [
        "account:read",
        "workers_routes:write",
        "workers_scripts:write",
        "zone:read",
      ],
    };
    expect(parseWranglerWhoami(JSON.stringify(whoami), accountId)).toEqual({
      accountId,
      authType: "oauth",
    });
    expect(() =>
      parseWranglerWhoami(
        JSON.stringify({
          ...whoami,
          tokenPermissions: ["account:read"],
        }),
        accountId,
      ),
    ).toThrow("lacks required permissions");
    expect(() =>
      parseWranglerWhoami(
        JSON.stringify({ ...whoami, authType: "API Token" }),
        accountId,
      ),
    ).toThrow("local OAuth profile");
  });

  test("accepts only an OAuth token without exposing it in the result shape", () => {
    expect(parseWranglerOAuthToken(JSON.stringify({
      token: "x".repeat(60),
      type: "oauth",
    }))).toBe("x".repeat(60));
    expect(() =>
      parseWranglerOAuthToken(
        JSON.stringify({ token: "x".repeat(60), type: "api_token" }),
      ),
    ).toThrow("OAuth token");
  });
});

test("Wrangler config can publish only the reviewed static asset and domain closure", () => {
  const config = {
    $schema: "https://unpkg.com/wrangler/config-schema.json",
    assets: {
      directory: "./public",
      html_handling: "auto-trailing-slash",
      not_found_handling: "404-page",
    },
    compatibility_date: compatibilityDate,
    name: service,
    routes: allHostnames.map((pattern) => ({
      custom_domain: true,
      pattern,
    })),
  };
  expect(
    parseWebsiteWranglerConfig(JSON.stringify(config), {
      compatibilityDate,
      hostnames: allHostnames,
      worker: service,
    }),
  ).toEqual({
    compatibilityDate,
    hostnames: [...allHostnames].sort(),
    worker: service,
  });
  for (const drifted of [
    { ...config, main: "attacker.mjs" },
    { ...config, assets: { ...config.assets, directory: "../" } },
    { ...config, routes: config.routes.slice(0, 2) },
  ]) {
    expect(() =>
      parseWebsiteWranglerConfig(JSON.stringify(drifted), {
        compatibilityDate,
        hostnames: allHostnames,
        worker: service,
      }),
    ).toThrow();
  }
});

test("zone lookup binds explicit account, zone, name, and delegation", () => {
  const raw = response([
    {
      account: { id: accountId },
      id: zoneId,
      name: zoneName,
      name_servers: ["duke.ns.cloudflare.com", "aida.ns.cloudflare.com"],
      status: "active",
    },
  ]);
  expect(
    parseCloudflareZone(raw, { accountId, zoneId, zoneName }),
  ).toEqual({
    accountId,
    nameServers: ["aida.ns.cloudflare.com", "duke.ns.cloudflare.com"],
    zoneId,
    zoneName,
  });
  expect(() =>
    parseCloudflareZone(
      response([{ ...raw.result[0], account: { id: "c".repeat(32) } }]),
      { accountId, zoneId, zoneName },
    ),
  ).toThrow("explicit account");
});

describe("custom-domain closure", () => {
  const inventory = (domains) =>
    response(domains, {
      count: domains.length,
      page: 1,
      per_page: 100,
      total_count: domains.length,
    });

  test("requires one complete non-paginated production binding set", () => {
    expect(
      parseWorkerDomainClosure(
        inventory(allHostnames.map((hostname) => domain(hostname))),
        {
        expectedHostnames: allHostnames,
        service,
        zoneId,
        zoneName,
        },
      ).map(({ hostname }) => hostname),
    ).toEqual([...allHostnames].sort());
    expect(() =>
      parseWorkerDomainClosure(
        {
          ...inventory(allHostnames.map((hostname) => domain(hostname))),
          result_info: {
            count: 3,
            page: 1,
            per_page: 100,
            total_count: 4,
          },
        },
        {
          expectedHostnames: allHostnames,
          service,
          zoneId,
          zoneName,
        },
      ),
    ).toThrow("paginated or count-ambiguous");
  });

  test("accepts only the exact non-conflicting initial addition", () => {
    const changeset = response({
      added: [domain("forms.takoform.com", "")],
      affected_zones: [zoneId],
      conflicting: [],
      removed: [],
      updated: apexHostnames.map((hostname) => ({
        ...domain(hostname, ""),
        modified: false,
      })),
    });
    expect(
      parseDomainChangeset(changeset, {
        existingHostnames: apexHostnames,
        expectedMode: "INITIAL",
        newHostname: "forms.takoform.com",
        service,
        zoneId,
        zoneName,
      }),
    ).toEqual({ mode: "INITIAL_DOMAIN_ABSENT" });

    for (const partial of [
      {
        ...changeset,
        result: {
          ...changeset.result,
          added: [],
        },
      },
      {
        ...changeset,
        result: {
          ...changeset.result,
          conflicting: [domain("forms.takoform.com", "")],
        },
      },
      {
        ...changeset,
        result: {
          ...changeset.result,
          updated: [
            { ...domain("takoform.com", ""), modified: true },
            { ...domain("www.takoform.com", ""), modified: false },
          ],
        },
      },
    ]) {
      expect(() =>
        parseDomainChangeset(partial, {
          existingHostnames: apexHostnames,
          expectedMode: "INITIAL",
          newHostname: "forms.takoform.com",
          service,
          zoneId,
          zoneName,
        }),
      ).toThrow();
    }
  });

  test("existing closure must be unchanged and never needs overrides", () => {
    expect(
      parseDomainChangeset(
        response({
          added: [],
          affected_zones: [zoneId],
          conflicting: [],
          removed: [],
          updated: allHostnames.map((hostname) => ({
            ...domain(hostname, ""),
            modified: false,
          })),
        }),
        {
          existingHostnames: apexHostnames,
          expectedMode: "EXISTING",
          newHostname: "forms.takoform.com",
          service,
          zoneId,
          zoneName,
        },
      ),
    ).toEqual({ mode: "EXISTING_DOMAIN_UNCHANGED" });
    expect(
      safeDomainWriteBody({ hostnames: allHostnames, zoneId, zoneName }),
    ).toMatchObject({
      override_existing_dns_record: false,
      override_existing_origin: false,
      override_scope: true,
    });
  });
});

describe("staged Worker version", () => {
  const versionId = "37dd55a9-d75b-452f-ab62-77486fb7204e";

  test("parses one exact upload id and rejects ambiguous output", () => {
    expect(
      parseUploadedVersionId(`Uploaded worker\nWorker Version ID: ${versionId}\n`),
    ).toBe(versionId);
    expect(() =>
      parseUploadedVersionId(
        `Worker Version ID: ${versionId}\nWorker Version ID: ${versionId}\n`,
      ),
    ).toThrow("one exact");
  });

  test("requires the static-only resource closure", () => {
    const value = {
      annotations: {
        "workers/message": "takoform.com abcdef",
        "workers/triggered_by": "version_upload",
      },
      id: versionId,
      resources: {
        bindings: [],
        script_runtime: {
          assets: {
            html_handling: "auto-trailing-slash",
            not_found_handling: "404-page",
            raw_run_worker_first: false,
            serve_directly: true,
          },
          compatibility_date: compatibilityDate,
        },
      },
    };
    expect(
      parseUploadedVersionResources(JSON.stringify(value), {
        compatibilityDate,
        expectedMessage: "takoform.com abcdef",
        versionId,
      }),
    ).toEqual({ versionId });
    expect(() =>
      parseUploadedVersionResources(
        JSON.stringify({
          ...value,
          resources: { ...value.resources, bindings: [{ name: "SECRET" }] },
        }),
        {
          compatibilityDate,
          expectedMessage: "takoform.com abcdef",
          versionId,
        },
      ),
    ).toThrow("static-only");
  });
});

test("authoritative DNS absence requires every delegated nameserver to return ENOTFOUND", async () => {
  const queried = [];
  const observations = await proveAuthoritativeHostnameAbsent(
    {
      hostname: "forms.takoform.com",
      nameServers: ["aida.ns.cloudflare.com", "duke.ns.cloudflare.com"],
      zoneName,
    },
    {
      createResolver: () => ({
        resolve4: async (hostname) => {
          queried.push(hostname);
          const error = new Error("not found");
          error.code = "ENOTFOUND";
          throw error;
        },
        setServers: () => {},
      }),
      resolve4Impl: async (hostname) => [
        hostname.startsWith("aida") ? "192.0.2.1" : "192.0.2.2",
      ],
      resolveNsImpl: async () => [
        "duke.ns.cloudflare.com",
        "aida.ns.cloudflare.com",
      ],
    },
  );
  expect(queried).toEqual([
    "forms.takoform.com",
    "forms.takoform.com",
  ]);
  expect(observations).toHaveLength(2);

  expect(
    proveAuthoritativeHostnameAbsent(
      {
        hostname: "forms.takoform.com",
        nameServers: ["aida.ns.cloudflare.com"],
        zoneName,
      },
      {
        resolveNsImpl: async () => ["other.ns.cloudflare.com"],
      },
    ),
  ).rejects.toThrow("does not match");
});

test("a deployment race during remote proof blocks the writer", async () => {
  let deployment = "previous";
  let writes = 0;
  await expect(
    runFencedMutation({
      fence: () => {
        if (deployment !== "previous") {
          throw new Error("deployment changed");
        }
      },
      remoteProof: async () => {
        deployment = "competing";
      },
      writer: async () => {
        writes += 1;
      },
    }),
  ).rejects.toThrow("deployment changed");
  expect(writes).toBe(0);
});

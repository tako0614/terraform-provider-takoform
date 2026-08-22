import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";

import {
  enforceRetiredSchemaIdentitiesAreNotReused,
  enforceSchemaPublicationNoOverwrite,
  INITIAL_SCHEMA_ORIGIN_MINT_ACK,
  inspectRetiredSchemaIdentities,
  inspectSchemaPublicationIdentities,
  readPublishedDigest,
} from "./schema-publication-guard.mjs";

const schema = (name, bytes = `${name}\n`) => ({
  candidateBytes: Buffer.from(bytes),
  id: `https://forms.takoform.com/schemas/${name}.json`,
});

const dnsPresent = async () => ({ address: "203.0.113.1", family: 4 });

describe("published schema identity no-overwrite guard", () => {
  test("accepts an existing identity only when served bytes are exact", async () => {
    const schemas = [schema("one"), schema("two")];
    const observations = await inspectSchemaPublicationIdentities(schemas, {
      fetchImpl: async (url) => {
        const name = new URL(url).pathname.split("/").at(-1).replace(".json", "");
        return new Response(`${name}\n`, { status: 200 });
      },
      lookupImpl: dnsPresent,
    });

    expect(enforceSchemaPublicationNoOverwrite(observations)).toEqual({
      count: 2,
      mode: "EXISTING_IDENTITIES_UNCHANGED",
    });
  });

  test("rejects changed served bytes even with the initial-mint acknowledgement", async () => {
    const observations = await inspectSchemaPublicationIdentities(
      [schema("one")],
      {
        fetchImpl: async () => new Response("different\n", { status: 200 }),
        lookupImpl: dnsPresent,
      },
    );

    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        initialOriginMintAcknowledged: true,
      }),
    ).toThrow("never bypasses a mismatch");
  });

  test("requires the exact acknowledgement when the entire origin is DNS-absent", async () => {
    const observations = await inspectSchemaPublicationIdentities(
      [schema("one"), schema("two")],
      {
        fetchImpl: async () => {
          throw new Error("fetch must not run for a DNS-absent origin");
        },
        lookupImpl: async () => {
          const error = new Error("getaddrinfo ENOTFOUND forms.takoform.com");
          error.code = "ENOTFOUND";
          throw error;
        },
      },
    );

    expect(() => enforceSchemaPublicationNoOverwrite(observations)).toThrow(
      INITIAL_SCHEMA_ORIGIN_MINT_ACK,
    );
    expect(
      enforceSchemaPublicationNoOverwrite(observations, {
        initialOriginMintAcknowledged: true,
      }),
    ).toEqual({
      count: 2,
      mode: "INITIAL_ORIGIN_MINT_ACKNOWLEDGED",
    });
  });

  test.each([
    {
      label: "an HTTP response",
      response: async () => new Response("not found", { status: 404 }),
      resolver: dnsPresent,
    },
    {
      label: "a non-DNS transport failure",
      response: async () => {
        throw new TypeError("fetch failed", {
          cause: { code: "ECONNRESET" },
        });
      },
      resolver: dnsPresent,
    },
    {
      label: "a temporary DNS failure",
      response: async () => {
        throw new Error("fetch must not run after DNS lookup failure");
      },
      resolver: async () => {
        const error = new Error("getaddrinfo EAI_AGAIN forms.takoform.com");
        error.code = "EAI_AGAIN";
        throw error;
      },
    },
  ])(
    "does not treat $label as an initial-origin mint",
    async ({ response, resolver }) => {
      const observations = await inspectSchemaPublicationIdentities(
        [schema("one")],
        { fetchImpl: response, lookupImpl: resolver },
      );

      expect(() =>
        enforceSchemaPublicationNoOverwrite(observations, {
          initialOriginMintAcknowledged: true,
        }),
      ).toThrow("never bypasses");
    },
  );

  test("rejects a partially existing origin", async () => {
    let partialLookupCalls = 0;
    const observations = await inspectSchemaPublicationIdentities(
      [schema("one"), schema("two")],
      {
        fetchImpl: async (url) => {
          if (url.endsWith("/one.json")) {
            return new Response("one\n", { status: 200 });
          }
          throw new Error("fetch must not run for a DNS-absent identity");
        },
        lookupImpl: async (hostname) => {
          if (hostname === "forms.takoform.com") {
            // The per-URL test simulates a resolver changing between calls;
            // the guard must fail closed on that partial state.
            partialLookupCalls += 1;
            if (partialLookupCalls === 1) {
              return dnsPresent();
            }
            const error = new Error("getaddrinfo ENOTFOUND");
            error.code = "ENOTFOUND";
            throw error;
          }
          return dnsPresent();
        },
      },
    );

    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        initialOriginMintAcknowledged: true,
      }),
    ).toThrow("partially existing origin");
  });

  test("allows 404 only for identities absent from the deployed source ledger", async () => {
    const schemas = [schema("published"), schema("new")];
    const observations = await inspectSchemaPublicationIdentities(schemas, {
      fetchImpl: async (url) =>
        url.endsWith("/published.json")
          ? new Response("published\n", { status: 200 })
          : new Response("not found", { status: 404 }),
      lookupImpl: dnsPresent,
    });

    expect(
      enforceSchemaPublicationNoOverwrite(observations, {
        publishedIdentityIds: [schemas[0].id],
      }),
    ).toEqual({
      count: 2,
      existingCount: 1,
      mode: "APPEND_ONLY_IDENTITIES_MINT",
      newCount: 1,
    });
  });

  test("rejects 404 for an identity retained by the deployed source ledger", async () => {
    const schemas = [schema("published"), schema("new")];
    const observations = await inspectSchemaPublicationIdentities(schemas, {
      fetchImpl: async () => new Response("not found", { status: 404 }),
      lookupImpl: dnsPresent,
    });

    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        publishedIdentityIds: [schemas[0].id],
      }),
    ).toThrow("published schema identity precondition failed");
  });

  test("rejects a mixed exact and absent candidate-only identity set", async () => {
    const schemas = [schema("published"), schema("new-one"), schema("new-two")];
    const observations = await inspectSchemaPublicationIdentities(schemas, {
      fetchImpl: async (url) => {
        if (url.endsWith("/new-two.json")) {
          return new Response("not found", { status: 404 });
        }
        const name = new URL(url).pathname.split("/").at(-1).replace(".json", "");
        return new Response(`${name}\n`, { status: 200 });
      },
      lookupImpl: dnsPresent,
    });

    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        publishedIdentityIds: [schemas[0].id],
      }),
    ).toThrow("published schema identity precondition failed");
  });

  test.each([
    {
      label: "a non-404 response",
      response: new Response("unavailable", { status: 503 }),
    },
    {
      label: "different bytes",
      response: new Response("different\n", { status: 200 }),
    },
  ])("rejects $label for a candidate-only identity", async ({ response }) => {
    const schemas = [schema("published"), schema("new")];
    const observations = await inspectSchemaPublicationIdentities(schemas, {
      fetchImpl: async (url) =>
        url.endsWith("/published.json")
          ? new Response("published\n", { status: 200 })
          : response,
      lookupImpl: dnsPresent,
    });

    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        publishedIdentityIds: [schemas[0].id],
      }),
    ).toThrow("published schema identity precondition failed");
  });

  test("rejects a stale acknowledgement once the origin exists", async () => {
    const observations = await inspectSchemaPublicationIdentities(
      [schema("one")],
      {
        fetchImpl: async () => new Response("one\n", { status: 200 }),
        lookupImpl: dnsPresent,
      },
    );

    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        initialOriginMintAcknowledged: true,
      }),
    ).toThrow("accepted only when every schema URL fails with ENOTFOUND");
  });
});

test("post-publication readback rejects redirects", async () => {
  const fetchImpl = async (_url, options) => {
    if (options.redirect === "error") {
      throw new TypeError("redirect rejected");
    }
    return new Response("redirect target bytes", { status: 200 });
  };

  expect(
    readPublishedDigest("https://forms.takoform.com/schemas/one.json", {
      fetchImpl,
    }),
  ).rejects.toThrow("redirect rejected");
});

test("post-publication readback can bind the exact custom 404 body", async () => {
  const expected = await readPublishedDigest(
    "https://takoform.com/__missing__",
    {
      expectedStatus: 404,
      fetchImpl: async () => new Response("not found\n", { status: 404 }),
    },
  );
  const digest = createHash("sha256").update("not found\n").digest("hex");
  expect(expected).toBe(digest);
});

test("website deploy contract declares the published identity obligation", () => {
  const result = Bun.spawnSync({
    cmd: [process.execPath, "scripts/deploy.mjs", "--contract"],
    cwd: new URL("..", import.meta.url).pathname,
    stderr: "pipe",
    stdout: "pipe",
  });
  expect(result.exitCode).toBe(0);
  const contract = JSON.parse(result.stdout.toString());
  const website = contract.surfaces.find(
    ({ surface }) => surface === "takoform-website",
  );

  expect(website.triggers).toContain("published-identity");
  expect(website.requiresEnv).toEqual([
    "TAKOFORM_CLOUDFLARE_ACCOUNT_ID",
    "TAKOFORM_CLOUDFLARE_ZONE_ID",
  ]);
  for (const variable of website.requiresEnv) {
    expect(Object.values(website.obligations).join("\n")).toContain(variable);
  }
  expect(website.requiresTools).toEqual([
    "git",
    "bun",
    "go",
    "node",
    "tar",
  ]);
  expect(website.requiresScripts).toEqual([
    "check:public-surfaces",
    "check:public-authority",
    "check:public-snapshot",
    "check:website-snapshot",
  ]);
  expect(website.obligations.provenance).toContain(
    "independent non-local detached Git authority clone",
  );
  expect(website.obligations.provenance).toContain(
    "static public-surface gate",
  );
  expect(website.obligations.provenance).toContain(
    "fresh VitePress build",
  );
  expect(website.obligations["no-overwrite"]).toContain(
    INITIAL_SCHEMA_ORIGIN_MINT_ACK,
  );
});

test("website deploy parsing cannot route a release surface through website writers", () => {
  const result = Bun.spawnSync({
    cmd: [
      process.execPath,
      "scripts/deploy.mjs",
      "--acknowledge-exclusive-cloudflare-writer",
      "takoform-provider-release",
    ],
    cwd: new URL("..", import.meta.url).pathname,
    stderr: "pipe",
    stdout: "pipe",
  });
  expect(result.exitCode).toBe(1);
  expect(result.stderr.toString()).toContain("usage:");
  expect(result.stderr.toString()).not.toContain("publishing");
});

// Decision 0037. Retirement had to be possible without the deploy refusing it,
// and impossible to use as a way to change what an address means.
describe("withdrawn schema identities", () => {
  const retiredId = "https://forms.takoform.com/schemas/gone.json";
  const publishedBytes = "gone\n";
  const publishedDigest = `sha256:${createHash("sha256").update(publishedBytes).digest("hex")}`;
  const retired = [
    {
      id: retiredId,
      public: "website/public/schemas/gone.json",
      retiredBecause: "pre-Stable lane withdrawn",
      sha256: publishedDigest,
      source: "spec/schemas/gone.json",
    },
  ];

  test("a withdrawal is not held to an observation it cannot have", async () => {
    const observations = await inspectSchemaPublicationIdentities(
      [schema("kept")],
      {
        fetchImpl: async () => new Response("kept\n", { status: 200 }),
        lookupImpl: dnsPresent,
      },
    );

    // The deployed ledger still lists the identity the candidate withdrew.
    // Before decision 0037 was carried into this guard, that combination threw
    // and no first retirement could ever deploy.
    expect(() =>
      enforceSchemaPublicationNoOverwrite(observations, {
        publishedIdentityIds: [schema("kept").id, retiredId],
      }),
    ).toThrow(`deployed source ledger identity ${retiredId} was not inspected`);

    expect(
      enforceSchemaPublicationNoOverwrite(observations, {
        publishedIdentityIds: [schema("kept").id, retiredId],
        retiredIdentityIds: [retiredId],
      }),
    ).toEqual({ count: 1, mode: "EXISTING_IDENTITIES_UNCHANGED" });
  });

  test("an address that stopped answering passes", async () => {
    const observations = await inspectRetiredSchemaIdentities(retired, {
      fetchImpl: async () => new Response("", { status: 404 }),
      lookupImpl: dnsPresent,
    });
    expect(observations[0].kind).toBe("withdrawn");
    expect(enforceRetiredSchemaIdentitiesAreNotReused(observations)).toEqual({
      withdrawn: 1,
      stillServed: 0,
    });
  });

  test("an address still answering what it published passes", async () => {
    const observations = await inspectRetiredSchemaIdentities(retired, {
      fetchImpl: async () => new Response(publishedBytes, { status: 200 }),
      lookupImpl: dnsPresent,
    });
    expect(observations[0].kind).toBe("withdrawn-still-served");
    expect(enforceRetiredSchemaIdentitiesAreNotReused(observations)).toEqual({
      withdrawn: 1,
      stillServed: 1,
    });
  });

  test("an address answering something new is refused", async () => {
    const observations = await inspectRetiredSchemaIdentities(retired, {
      fetchImpl: async () => new Response("something else\n", { status: 200 }),
      lookupImpl: dnsPresent,
    });
    expect(observations[0].kind).toBe("reused");
    expect(() => enforceRetiredSchemaIdentitiesAreNotReused(observations)).toThrow(
      "answer different bytes",
    );
  });

  test("an unreachable origin is a withdrawal, not a refusal", async () => {
    const observations = await inspectRetiredSchemaIdentities(retired, {
      fetchImpl: async () => {
        throw new Error("connection reset");
      },
      lookupImpl: dnsPresent,
    });
    expect(observations[0].kind).toBe("withdrawn");
    expect(() =>
      enforceRetiredSchemaIdentitiesAreNotReused(observations),
    ).not.toThrow();
  });
});

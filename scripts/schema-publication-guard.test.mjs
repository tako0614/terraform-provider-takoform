import { describe, expect, test } from "bun:test";

import {
  enforceSchemaPublicationNoOverwrite,
  INITIAL_SCHEMA_ORIGIN_MINT_ACK,
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
  expect(website.obligations["no-overwrite"]).toContain(
    INITIAL_SCHEMA_ORIGIN_MINT_ACK,
  );
});

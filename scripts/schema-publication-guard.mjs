import { createHash } from "node:crypto";
import { lookup } from "node:dns/promises";

export const INITIAL_SCHEMA_ORIGIN_MINT_ACK =
  "--acknowledge-initial-schema-origin-mint";

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function hasErrorCode(error, wanted, seen = new Set()) {
  if (error === null || typeof error !== "object" || seen.has(error)) {
    return false;
  }
  seen.add(error);
  if (error.code === wanted) {
    return true;
  }
  if (hasErrorCode(error.cause, wanted, seen)) {
    return true;
  }
  if (Array.isArray(error.errors)) {
    return error.errors.some((nested) => hasErrorCode(nested, wanted, seen));
  }
  return false;
}

function errorDetail(error) {
  if (error instanceof Error && error.message !== "") {
    return error.message;
  }
  return String(error);
}

export async function readPublishedDigest(
  url,
  { fetchImpl = globalThis.fetch } = {},
) {
  const response = await fetchImpl(url, {
    headers: { "cache-control": "no-cache" },
    redirect: "error",
  });
  if (response.status !== 200) {
    throw new Error(`${url} responded ${response.status}`);
  }
  return sha256(Buffer.from(await response.arrayBuffer()));
}

export async function inspectSchemaPublicationIdentities(
  schemas,
  { fetchImpl = globalThis.fetch, lookupImpl = lookup } = {},
) {
  return Promise.all(
    schemas.map(async ({ candidateBytes, id }) => {
      const candidate = Buffer.from(candidateBytes);
      const candidateDigest = sha256(candidate);
      const hostname = new URL(id).hostname;

      // Bun's fetch deliberately hides the resolver error code. Resolve first
      // so the one-time mint branch depends on the resolver's exact
      // `ENOTFOUND` code rather than an error-message guess.
      try {
        await lookupImpl(hostname);
      } catch (error) {
        return {
          candidateDigest,
          detail: errorDetail(error),
          id,
          kind: hasErrorCode(error, "ENOTFOUND")
            ? "dns-not-found"
            : "transport-error",
        };
      }

      let response;
      try {
        response = await fetchImpl(id, {
          headers: {
            "cache-control": "no-cache",
            pragma: "no-cache",
          },
          redirect: "error",
        });
      } catch (error) {
        return {
          candidateDigest,
          detail: errorDetail(error),
          id,
          // DNS was present immediately before fetch. Even if resolution
          // changes during the request, this is not the initial-origin case.
          kind: "transport-error",
        };
      }

      if (response.status !== 200) {
        return {
          candidateDigest,
          detail: `HTTP ${response.status}`,
          id,
          kind: "http-error",
        };
      }

      let served;
      try {
        served = Buffer.from(await response.arrayBuffer());
      } catch (error) {
        return {
          candidateDigest,
          detail: `could not read response body: ${errorDetail(error)}`,
          id,
          kind: "transport-error",
        };
      }

      const servedDigest = sha256(served);
      return {
        candidateDigest,
        id,
        kind: candidate.equals(served) ? "match" : "mismatch",
        servedDigest,
      };
    }),
  );
}

function observationDetail(observation) {
  switch (observation.kind) {
    case "match":
      return `${observation.id}: exact sha256 ${observation.candidateDigest}`;
    case "mismatch":
      return (
        `${observation.id}: served sha256 ${observation.servedDigest}, ` +
        `candidate sha256 ${observation.candidateDigest}`
      );
    default:
      return `${observation.id}: ${observation.kind} (${observation.detail})`;
  }
}

export function enforceSchemaPublicationNoOverwrite(
  observations,
  { initialOriginMintAcknowledged = false } = {},
) {
  if (observations.length === 0) {
    throw new Error("no normative schema identities were inspected");
  }

  if (observations.every(({ kind }) => kind === "match")) {
    if (initialOriginMintAcknowledged) {
      throw new Error(
        `${INITIAL_SCHEMA_ORIGIN_MINT_ACK} is accepted only when every schema URL fails with ENOTFOUND; remove it for an existing origin`,
      );
    }
    return {
      count: observations.length,
      mode: "EXISTING_IDENTITIES_UNCHANGED",
    };
  }

  if (observations.every(({ kind }) => kind === "dns-not-found")) {
    if (!initialOriginMintAcknowledged) {
      throw new Error(
        `the schema origin does not exist yet; initial publication requires the explicit ${INITIAL_SCHEMA_ORIGIN_MINT_ACK} acknowledgement`,
      );
    }
    return {
      count: observations.length,
      mode: "INITIAL_ORIGIN_MINT_ACKNOWLEDGED",
    };
  }

  const detail = observations
    .filter(({ kind }) => kind !== "match")
    .map(observationDetail)
    .join("; ");
  throw new Error(
    `published schema identity precondition failed: ${detail}. ` +
      `${INITIAL_SCHEMA_ORIGIN_MINT_ACK} never bypasses a mismatch, HTTP response, non-DNS transport failure, or partially existing origin`,
  );
}

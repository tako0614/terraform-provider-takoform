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
  { expectedStatus = 200, fetchImpl = globalThis.fetch } = {},
) {
  const response = await fetchImpl(url, {
    headers: {
      "cache-control": "no-cache",
      pragma: "no-cache",
    },
    redirect: "error",
  });
  if (response.status !== expectedStatus) {
    throw new Error(
      `${url} responded ${response.status}, expected ${expectedStatus}`,
    );
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
          status: response.status,
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

// A withdrawn identity is not served any more, so there is no candidate to
// compare it against and it is absent from the inspection above (decision
// 0037). What still has to hold is the half that made retirement safe: the
// address may stop answering, and it may keep answering the bytes it was
// published with, but it may never answer something NEW. A reader who kept
// that URL is owed a 404 or the document they remember — never a different one
// wearing its name.
//
// Unreachable is therefore a pass. Only a 200 carrying different bytes fails,
// so a network problem during a deploy cannot manufacture a refusal about an
// address nobody is publishing.
export async function inspectRetiredSchemaIdentities(
  retired,
  { fetchImpl = globalThis.fetch, lookupImpl = lookup } = {},
) {
  return Promise.all(
    retired.map(async ({ id, sha256: publishedDigest }) => {
      try {
        await lookupImpl(new URL(id).hostname);
      } catch {
        return { id, kind: "withdrawn", detail: "origin does not resolve" };
      }
      let response;
      try {
        response = await fetchImpl(id, {
          headers: { "cache-control": "no-cache", pragma: "no-cache" },
          redirect: "error",
        });
      } catch (error) {
        return { id, kind: "withdrawn", detail: errorDetail(error) };
      }
      if (response.status !== 200) {
        return { id, kind: "withdrawn", detail: `HTTP ${response.status}` };
      }
      let served;
      try {
        served = Buffer.from(await response.arrayBuffer());
      } catch (error) {
        return { id, kind: "withdrawn", detail: errorDetail(error) };
      }
      // The ledger records `sha256:<hex>`; this module's digests are bare hex.
      const servedDigest = `sha256:${sha256(served)}`;
      return servedDigest === publishedDigest
        ? { id, kind: "withdrawn-still-served", detail: servedDigest }
        : { id, kind: "reused", detail: servedDigest, publishedDigest };
    }),
  );
}

export function enforceRetiredSchemaIdentitiesAreNotReused(observations) {
  const reused = observations.filter(({ kind }) => kind === "reused");
  if (reused.length > 0) {
    throw new Error(
      `withdrawn schema identities answer different bytes: ` +
        reused
          .map(
            ({ id, publishedDigest, detail }) =>
              `${id} was published as ${publishedDigest} and now serves ${detail}`,
          )
          .join("; ") +
        `; a withdrawn address may stop answering or keep answering what it ` +
        `published, never something new`,
    );
  }
  return {
    withdrawn: observations.length,
    stillServed: observations.filter(
      ({ kind }) => kind === "withdrawn-still-served",
    ).length,
  };
}

export function enforceSchemaPublicationNoOverwrite(
  observations,
  {
    initialOriginMintAcknowledged = false,
    publishedIdentityIds = [],
    retiredIdentityIds = [],
  } = {},
) {
  if (observations.length === 0) {
    throw new Error("no normative schema identities were inspected");
  }

  if (
    !Array.isArray(publishedIdentityIds) ||
    publishedIdentityIds.some((id) => typeof id !== "string" || id === "") ||
    new Set(publishedIdentityIds).size !== publishedIdentityIds.length
  ) {
    throw new Error("published schema identity ids are invalid");
  }
  const observationsById = new Map();
  for (const observation of observations) {
    if (
      typeof observation?.id !== "string" ||
      observationsById.has(observation.id)
    ) {
      throw new Error("schema publication observations are invalid");
    }
    observationsById.set(observation.id, observation);
  }
  // An identity the candidate has withdrawn is deliberately absent from the
  // inspection: it has no candidate bytes to compare against any more. Holding
  // the deployed ledger's copy of it to an observation would make the first
  // recorded withdrawal fail the deploy it was meant to be possible under.
  const withdrawn = new Set(retiredIdentityIds);
  const published = publishedIdentityIds
    .filter((id) => !withdrawn.has(id))
    .map((id) => {
      const observation = observationsById.get(id);
      if (!observation) {
        throw new Error(
          `deployed source ledger identity ${id} was not inspected`,
        );
      }
      return observation;
    });

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

  if (
    initialOriginMintAcknowledged &&
    published.length > 0 &&
    observations.some(({ kind }) => kind === "match")
  ) {
    throw new Error(
      `${INITIAL_SCHEMA_ORIGIN_MINT_ACK} is accepted only when every schema URL fails with ENOTFOUND; remove it for an existing origin`,
    );
  }

  const publishedExact = published.every(({ kind }) => kind === "match");
  const publishedIds = new Set(publishedIdentityIds);
  const candidateOnly = observations.filter(({ id }) => !publishedIds.has(id));
  const absentCandidateOnly = candidateOnly.filter(
    ({ kind, status }) => kind === "http-error" && status === 404,
  );
  if (
    published.length > 0 &&
    publishedExact &&
    absentCandidateOnly.length > 0 &&
    absentCandidateOnly.length === candidateOnly.length
  ) {
    return {
      count: observations.length,
      existingCount: published.length,
      mode: "APPEND_ONLY_IDENTITIES_MINT",
      newCount: absentCandidateOnly.length,
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

/*
 * The peer bundle of conformance/runtime-abi-v1: the CALLEE a
 * `worker.service` binding addresses.
 *
 * It exists because the two service checks have to be able to tell a call that
 * crossed the binding from one that never left the caller. While the peer ran
 * the caller's own bytes, a host that short-circuited `env.PEER.fetch(...)`
 * back into its own fetch handler produced exactly the answer the runner
 * expected — the same routes, the same accounting, the same timing — so both
 * checks passed a runtime that had implemented no cross-worker projection at
 * all. A required check no incorrect runtime can fail proves nothing.
 *
 * What distinguishes the peer is therefore a fact carried by BYTES rather than
 * by a deployment, a name, or a header: PEER_IDENTITY below is a literal this
 * module has and no other module of the corpus does, and every observation
 * this module emits is stamped with it. The corpus loader enforces both halves
 * — the identity must be present in these bytes and absent from every other
 * bundle's — so the caller running its own pinned bytes cannot produce a
 * stamped answer, whatever the host does with the dispatch.
 *
 * It carries no expectation of its own, exactly as the probe module does not.
 * The two routes it serves are the two the direct streaming checks measure on
 * the caller, taken one worker further along: it accounts for the request body
 * as it reads it, and it separates its response chunks in time. Its default
 * export exposes `fetch` alone, because a callee is invoked through `fetch` and
 * nothing else about this worker is measured. It touches no binding, so an
 * operator deploys it with no vars, no sensitive variable, and no attachments.
 */

const PROBE = "takoform.runtime-abi-probe@v1";

/*
 * The peer's identity: bytes only this module carries. It is not a secret and
 * not a credential — the corpus is public — it is the thing an answer produced
 * by the caller's own bundle cannot contain.
 */
const PEER_IDENTITY = "takoform-runtime-abi-peer-a3f172c9e08b4d56";

const ROUTE_PREFIX = "/abi/";

const encoder = new TextEncoder();

function routeOf(url) {
  const marker = url.pathname.indexOf(ROUTE_PREFIX);
  if (marker < 0) {
    return "";
  }
  return url.pathname.slice(marker + ROUTE_PREFIX.length);
}

function line(body) {
  return encoder.encode(
    JSON.stringify(Object.assign({ probe: PROBE, peer: PEER_IDENTITY }, body)) +
      "\n",
  );
}

function ndjson(stream) {
  return new Response(stream, {
    status: 200,
    headers: { "content-type": "application/x-ndjson" },
  });
}

function sleep(millis) {
  return new Promise((resolve) => {
    setTimeout(resolve, millis);
  });
}

function handleEchoStream(request) {
  const reader = request.body.getReader();
  let reads = 0;
  return ndjson(
    new ReadableStream({
      async pull(controller) {
        const next = await reader.read();
        if (next.done) {
          controller.enqueue(line({ end: true }));
          controller.close();
          return;
        }
        reads += 1;
        controller.enqueue(line({ read: reads, bytes: next.value.byteLength }));
      },
    }),
  );
}

function handleResponseStream(url) {
  const chunks = Number(url.searchParams.get("chunks"));
  const gapMillis = Number(url.searchParams.get("gapMillis"));
  let emitted = 0;
  return ndjson(
    new ReadableStream({
      async pull(controller) {
        if (emitted >= chunks) {
          controller.close();
          return;
        }
        if (emitted > 0) {
          await sleep(gapMillis);
        }
        emitted += 1;
        controller.enqueue(line({ chunk: emitted }));
      },
    }),
  );
}

const handlers = {
  async fetch(request) {
    const url = new URL(request.url);
    const route = routeOf(url);
    if (route === "echo-stream") {
      return handleEchoStream(request);
    }
    if (route === "stream") {
      return handleResponseStream(url);
    }
    return new Response(
      JSON.stringify({
        probe: PROBE,
        peer: PEER_IDENTITY,
        error: "unknown peer route",
        route: route,
      }),
      { status: 404, headers: { "content-type": "application/json" } },
    );
  },
};

export default handlers;

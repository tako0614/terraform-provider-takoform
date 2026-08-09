/*
 * takoform.runtime-abi-probe@v1 — the observation surface of
 * conformance/runtime-abi-v1.
 *
 * A runtime under test loads this file as the main module of the conformance
 * bundle. It carries NO expectation: every route reports what the RUNTIME did
 * with the arguments, environment, globals, streams, deferred work, and
 * bindings it supplied, and the runner alone decides whether that matches the
 * worker.runtime@1.0.0 contract. A probe that knew the answers could pass a
 * runtime that does not implement the ABI.
 *
 * The module uses only the portable globals floor the contract fixes and the
 * three bindings the corpus deployment declares: CACHE (edge.kv), EVENTS
 * (edge.queue) and PEER (worker.service, addressing the second deployment of
 * these same bytes). Base64 is implemented here rather than taken from a
 * global, because base64 is not on the floor.
 *
 * The two PEER routes carry nothing of their own: each hands the callee the
 * stream it is still receiving and returns the callee's response body unread,
 * so what a runner measures across them is the BINDING's behaviour rather than
 * this module's. They are the observation the direct streaming routes make,
 * taken one worker further along.
 *
 * Correlation is the runner's, never the module's. Every route that stores an
 * observation stores it under the correlation value the RUNNER sent, and every
 * route that reports one reports that value back, so the runner can tell an
 * observation its own run caused from one a previous run against the same
 * `edge.kv` namespace left behind. The module never mints a value of its own:
 * a probe that generated its own correlation value would correlate the probe
 * with itself and prove nothing about which run stored what.
 */

const PROBE = "takoform.runtime-abi-probe@v1";
const ROUTE_PREFIX = "/abi/";
const BASE64_ALPHABET =
  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

const encoder = new TextEncoder();
const decoder = new TextDecoder();

function encodeBase64(bytes) {
  let out = "";
  for (let index = 0; index < bytes.length; index += 3) {
    const first = bytes[index];
    const second = index + 1 < bytes.length ? bytes[index + 1] : 0;
    const third = index + 2 < bytes.length ? bytes[index + 2] : 0;
    out += BASE64_ALPHABET[first >> 2];
    out += BASE64_ALPHABET[((first & 3) << 4) | (second >> 4)];
    out +=
      index + 1 < bytes.length
        ? BASE64_ALPHABET[((second & 15) << 2) | (third >> 6)]
        : "=";
    out += index + 2 < bytes.length ? BASE64_ALPHABET[third & 63] : "=";
  }
  return out;
}

function decodeBase64(text) {
  let clean = String(text);
  while (clean.endsWith("=")) {
    clean = clean.slice(0, -1);
  }
  const bytes = new Uint8Array((clean.length * 3) >> 2);
  let accumulator = 0;
  let bits = 0;
  let position = 0;
  for (const character of clean) {
    const value = BASE64_ALPHABET.indexOf(character);
    if (value < 0) {
      throw new Error("invalid base64 input");
    }
    accumulator = (accumulator << 6) | value;
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      bytes[position] = (accumulator >> bits) & 255;
      position += 1;
    }
  }
  return bytes.subarray(0, position);
}

/*
 * toBytes normalises every JavaScript shape a host may legitimately hand the
 * probe for a byte payload. The runtime contract states a queue message body
 * as the family encoded-bytes object on the wire and does not fix its
 * JavaScript form, so the probe accepts each plausible one rather than
 * failing a host over an under-specified projection.
 */
function toBytes(value) {
  if (value === null || value === undefined) {
    return new Uint8Array(0);
  }
  if (typeof value === "string") {
    return encoder.encode(value);
  }
  if (value instanceof Uint8Array) {
    return value;
  }
  if (value instanceof ArrayBuffer) {
    return new Uint8Array(value);
  }
  if (ArrayBuffer.isView(value)) {
    return new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  }
  if (typeof value === "object" && typeof value.data === "string") {
    return decodeBase64(value.data);
  }
  throw new Error("unsupported byte payload shape");
}

function json(body, status) {
  return new Response(JSON.stringify(body), {
    status: status === undefined ? 200 : status,
    headers: { "content-type": "application/json" },
  });
}

function routeOf(url) {
  const marker = url.pathname.indexOf(ROUTE_PREFIX);
  if (marker < 0) {
    return "";
  }
  return url.pathname.slice(marker + ROUTE_PREFIX.length);
}

function resolveGlobal(path) {
  let value = globalThis;
  for (const segment of path.split(".")) {
    if (value === null || value === undefined) {
      return undefined;
    }
    value = value[segment];
  }
  return value;
}

async function kvPut(env, key, document) {
  await env.CACHE.put(key, JSON.stringify(document));
}

async function kvJSON(env, key) {
  const stored = await env.CACHE.get(key);
  if (stored === null || stored === undefined) {
    return null;
  }
  return JSON.parse(decoder.decode(toBytes(stored)));
}

function sleep(millis) {
  return new Promise((resolve) => {
    setTimeout(resolve, millis);
  });
}

async function handleKV(request, env, url) {
  if (request.method === "POST") {
    const body = await request.json();
    await env.CACHE.put(body.key, decodeBase64(body.valueBase64));
    return json({ probe: PROBE, stored: true });
  }
  const stored = await env.CACHE.get(url.searchParams.get("key"));
  if (stored === null || stored === undefined) {
    return json({ probe: PROBE, found: false });
  }
  return json({
    probe: PROBE,
    found: true,
    valueBase64: encodeBase64(toBytes(stored)),
  });
}

function handleEchoStream(request) {
  const reader = request.body.getReader();
  let reads = 0;
  const stream = new ReadableStream({
    async pull(controller) {
      const next = await reader.read();
      if (next.done) {
        controller.enqueue(
          encoder.encode(JSON.stringify({ probe: PROBE, end: true }) + "\n"),
        );
        controller.close();
        return;
      }
      reads += 1;
      controller.enqueue(
        encoder.encode(
          JSON.stringify({
            probe: PROBE,
            read: reads,
            bytes: next.value.byteLength,
          }) + "\n",
        ),
      );
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { "content-type": "application/x-ndjson" },
  });
}

function handleResponseStream(url) {
  const chunks = Number(url.searchParams.get("chunks"));
  const gapMillis = Number(url.searchParams.get("gapMillis"));
  let emitted = 0;
  const stream = new ReadableStream({
    async pull(controller) {
      if (emitted >= chunks) {
        controller.close();
        return;
      }
      if (emitted > 0) {
        await sleep(gapMillis);
      }
      emitted += 1;
      controller.enqueue(
        encoder.encode(
          JSON.stringify({ probe: PROBE, chunk: emitted }) + "\n",
        ),
      );
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { "content-type": "application/x-ndjson" },
  });
}

async function handleWaitUntil(request, env, ctx, url) {
  if (request.method !== "POST") {
    const observed = await kvJSON(env, "wait-until:" + url.searchParams.get("nonce"));
    if (observed === null) {
      return json({ probe: PROBE, settled: false });
    }
    return json({
      probe: PROBE,
      settled: true,
      afterRejection: observed.afterRejection,
      nonce: observed.nonce,
    });
  }
  const body = await request.json();
  const key = "wait-until:" + body.nonce;
  ctx.waitUntil(
    Promise.reject(new Error("takoform runtime conformance rejected task")),
  );
  ctx.waitUntil(
    (async () => {
      await sleep(Number(body.delayMillis));
      await kvPut(env, key, { afterRejection: true, nonce: body.nonce });
    })(),
  );
  return json({ probe: PROBE, registeredTasks: 2, responseSent: true });
}

async function handleScheduledObservation(request, env) {
  if (request.method === "DELETE") {
    await env.CACHE.delete("scheduled:last");
    return json({ probe: PROBE, cleared: true });
  }
  const observed = await kvJSON(env, "scheduled:last");
  if (observed === null) {
    return json({ probe: PROBE, observed: false });
  }
  return json({
    probe: PROBE,
    observed: true,
    cron: observed.cron,
    scheduledTimeMillis: observed.scheduledTimeMillis,
  });
}

async function handleQueue(request, env, url) {
  if (request.method === "POST") {
    const body = await request.json();
    await env.EVENTS.send(
      encoder.encode(
        JSON.stringify({ nonce: body.nonce, payloadBase64: body.payloadBase64 }),
      ),
    );
    return json({ probe: PROBE, sent: true });
  }
  const observed = await kvJSON(env, "queue:" + url.searchParams.get("nonce"));
  if (observed === null) {
    return json({ probe: PROBE, delivered: false });
  }
  return json({
    probe: PROBE,
    delivered: true,
    queue: observed.queue,
    attempts: observed.attempts,
    messageId: observed.messageId,
    payloadBase64: observed.payloadBase64,
    nonce: observed.nonce,
  });
}

const PEER_ORIGIN = "https://peer.invalid";

async function handleServiceEchoStream(request, env) {
  const peer = await env.PEER.fetch(PEER_ORIGIN + "/abi/echo-stream", {
    method: "POST",
    headers: { "content-type": "application/octet-stream" },
    body: request.body,
    duplex: "half",
  });
  return new Response(peer.body, {
    status: peer.status,
    headers: { "content-type": "application/x-ndjson" },
  });
}

async function handleServiceResponseStream(env, url) {
  const query = new URLSearchParams({
    chunks: String(url.searchParams.get("chunks")),
    gapMillis: String(url.searchParams.get("gapMillis")),
  });
  const peer = await env.PEER.fetch(
    PEER_ORIGIN + "/abi/stream?" + query.toString(),
  );
  return new Response(peer.body, {
    status: peer.status,
    headers: { "content-type": "application/x-ndjson" },
  });
}

const handlers = {
  async fetch(request, env, ctx) {
    const argumentCount = arguments.length;
    const url = new URL(request.url);
    const route = routeOf(url);
    if (route === "health") {
      return json({ probe: PROBE, ok: true });
    }
    if (route === "handler") {
      return json({
        probe: PROBE,
        argumentCount: argumentCount,
        requestIsRequest: request instanceof Request,
        requestMethod: request.method,
        envIsObject: typeof env === "object" && env !== null,
        waitUntilIsFunction: typeof ctx.waitUntil === "function",
      });
    }
    if (route === "throw") {
      throw new Error("takoform runtime conformance uncaught throw");
    }
    if (route === "env") {
      const propertyNames = Object.keys(env);
      propertyNames.sort();
      return json({ probe: PROBE, propertyNames: propertyNames });
    }
    if (route === "globals") {
      const requested = String(url.searchParams.get("names")).split(",");
      const present = [];
      const missing = [];
      for (const name of requested) {
        if (resolveGlobal(name) === undefined) {
          missing.push(name);
        } else {
          present.push(name);
        }
      }
      present.sort();
      missing.sort();
      return json({ probe: PROBE, present: present, missing: missing });
    }
    if (route === "kv") {
      return handleKV(request, env, url);
    }
    if (route === "echo-stream") {
      return handleEchoStream(request);
    }
    if (route === "stream") {
      return handleResponseStream(url);
    }
    if (route === "wait-until") {
      return handleWaitUntil(request, env, ctx, url);
    }
    if (route === "scheduled") {
      return handleScheduledObservation(request, env);
    }
    if (route === "queue") {
      return handleQueue(request, env, url);
    }
    if (route === "service-echo-stream") {
      return handleServiceEchoStream(request, env);
    }
    if (route === "service-stream") {
      return handleServiceResponseStream(env, url);
    }
    return json({ probe: PROBE, error: "unknown probe route", route: route }, 404);
  },

  async scheduled(event, env) {
    await kvPut(env, "scheduled:last", {
      cron: event.cron,
      scheduledTimeMillis: event.scheduledTime,
    });
  },

  async queue(batch, env) {
    for (const message of batch.messages) {
      const decoded = JSON.parse(decoder.decode(toBytes(message.body)));
      await kvPut(env, "queue:" + decoded.nonce, {
        queue: batch.queue,
        attempts: message.attempts,
        messageId: message.id,
        payloadBase64: decoded.payloadBase64,
        nonce: decoded.nonce,
      });
    }
  },
};

export default handlers;

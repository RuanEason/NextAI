import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
import test from "node:test";
import { createServer } from "node:net";

const gatewayDir = new URL("../../apps/gateway", import.meta.url);
const DEBUG = process.env.DEBUG_SMOKE === "1";

test("gateway chat flow e2e: create -> stream send -> history", { timeout: 90_000 }, async () => {
  debug("allocate port");
  const port = await getFreePort();
  debug(`port=${port}`);
  const dataDir = await mkdtemp(join(tmpdir(), "copaw-smoke-"));
  const gatewayBin = join(dataDir, "gateway-smoke");
  const baseURL = `http://127.0.0.1:${port}`;

  debug("build gateway");
  await runCommand("go", ["build", "-o", gatewayBin, "./cmd/gateway"], {
    cwd: gatewayDir,
  });
  debug("build done");

  const proc = spawn(gatewayBin, [], {
    cwd: gatewayDir,
    env: {
      ...process.env,
      COPAW_HOST: "127.0.0.1",
      COPAW_PORT: String(port),
      COPAW_DATA_DIR: dataDir,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });

  let logs = "";
  proc.stdout.on("data", (chunk) => {
    logs += chunk.toString();
  });
  proc.stderr.on("data", (chunk) => {
    logs += chunk.toString();
  });

  try {
    debug("wait for health");
    await waitForHealth(`${baseURL}/healthz`);
    debug("health ok");

    const sessionID = `session-${Date.now()}`;
    const userID = "smoke-user";
    const channel = "console";
    const inputText = "hello smoke";

    const createdChat = await requestJSON(`${baseURL}/chats`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        session_id: sessionID,
        user_id: userID,
        channel,
        name: "smoke-chat",
        meta: {},
      }),
    });

    assert.equal(createdChat.session_id, sessionID);
    assert.equal(createdChat.user_id, userID);
    assert.equal(createdChat.channel, channel);
    assert.ok(createdChat.id, "chat id should exist");

    debug("chat created");

    debug("request stream start");
    const streamResponse = await fetch(`${baseURL}/agent/process`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        input: [{ role: "user", type: "message", content: [{ type: "text", text: inputText }] }],
        session_id: sessionID,
        user_id: userID,
        channel,
        stream: true,
      }),
    });
    assert.equal(streamResponse.ok, true, "stream request should succeed");
    assert.ok(streamResponse.body, "stream response body should exist");

    debug("streaming");
    const streamedReply = await readSSEDelta(streamResponse.body);
    assert.match(streamedReply, /Echo:\s*hello smoke/);
    debug("stream done");

    const chats = await requestJSON(
      `${baseURL}/chats?${new URLSearchParams({ user_id: userID, channel }).toString()}`,
    );
    assert.ok(Array.isArray(chats), "chat list should be array");
    assert.ok(chats.some((chat) => chat.id === createdChat.id), "created chat should be listed");

    const history = await requestJSON(`${baseURL}/chats/${encodeURIComponent(createdChat.id)}`);
    assert.ok(Array.isArray(history.messages), "history messages should be array");
    assert.equal(history.messages.length, 2, "history should contain user + assistant message");
    assert.equal(history.messages[0].role, "user");
    assert.equal(history.messages[1].role, "assistant");
    assert.match(history.messages[1].content?.[0]?.text ?? "", /Echo:\s*hello smoke/);
    debug("history checked");
  } finally {
    debug("cleanup begin");
    proc.kill("SIGTERM");
    await onceProcessExit(proc, 4000);
    await rm(dataDir, { recursive: true, force: true });
    debug("cleanup done");
  }

  if (proc.exitCode !== null && proc.exitCode !== 0) {
    throw new Error(`gateway exited unexpectedly (${proc.exitCode})\n${logs}`);
  }
});

async function requestJSON(url, init = undefined) {
  const response = await fetch(url, init);
  const text = await response.text();
  const parsed = text ? JSON.parse(text) : {};
  if (!response.ok) {
    const code = parsed?.error?.code ? `${parsed.error.code}: ` : "";
    const message = parsed?.error?.message ?? response.statusText;
    throw new Error(`${code}${message}`.trim());
  }
  return parsed;
}

async function readSSEDelta(stream) {
  const reader = stream.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let output = "";
  let done = false;

  while (!done) {
    const chunk = await reader.read();
    if (chunk.done) {
      break;
    }
    buffer += decoder.decode(chunk.value, { stream: true }).replaceAll("\r", "");
    const parsed = parseSSEBuffer(buffer);
    buffer = parsed.rest;
    output += parsed.delta;
    done = parsed.done;
    if (done) {
      await reader.cancel();
      break;
    }
  }

  buffer += decoder.decode().replaceAll("\r", "");
  if (!done && buffer.trim() !== "") {
    const parsed = parseSSEBuffer(`${buffer}\n\n`);
    output += parsed.delta;
    done = parsed.done;
  }
  assert.equal(done, true, "SSE stream should end with [DONE]");
  return output;
}

function parseSSEBuffer(raw) {
  let buffer = raw;
  let done = false;
  let delta = "";

  while (!done) {
    const boundary = buffer.indexOf("\n\n");
    if (boundary < 0) {
      break;
    }
    const block = buffer.slice(0, boundary);
    buffer = buffer.slice(boundary + 2);

    const dataLines = block
      .split("\n")
      .filter((line) => line.startsWith("data:"))
      .map((line) => line.slice(5).trimStart());
    if (dataLines.length === 0) {
      continue;
    }

    const data = dataLines.join("\n");
    if (data === "[DONE]") {
      done = true;
      break;
    }
    const payload = JSON.parse(data);
    if (typeof payload.delta === "string") {
      delta += payload.delta;
    }
  }

  return { done, delta, rest: buffer };
}

async function waitForHealth(url, timeoutMs = 30_000) {
  const start = Date.now();
  let lastError = null;
  while (Date.now() - start < timeoutMs) {
    try {
      const response = await fetch(url);
      if (response.ok) {
        return;
      }
      lastError = new Error(`health status: ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await sleep(250);
  }
  throw new Error(`gateway did not become healthy in ${timeoutMs}ms: ${String(lastError)}`);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function onceProcessExit(proc, timeoutMs) {
  await waitForExit(proc, timeoutMs);
  if (proc.exitCode === null) {
    proc.kill("SIGKILL");
    await waitForExit(proc, timeoutMs);
  }
}

function waitForExit(proc, timeoutMs) {
  if (proc.exitCode !== null) {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, timeoutMs);
    proc.once("exit", () => {
      clearTimeout(timer);
      resolve();
    });
  });
}

function getFreePort() {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        server.close(() => reject(new Error("failed to allocate port")));
        return;
      }
      const port = address.port;
      server.close((err) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(port);
      });
    });
  });
}

function debug(message) {
  if (!DEBUG) {
    return;
  }
  console.error(`[smoke] ${message}`);
}

function runCommand(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const proc = spawn(command, args, {
      ...options,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let output = "";
    proc.stdout.on("data", (chunk) => {
      output += chunk.toString();
    });
    proc.stderr.on("data", (chunk) => {
      output += chunk.toString();
    });
    proc.once("error", reject);
    proc.once("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${command} ${args.join(" ")} failed (${code})\n${output}`));
    });
  });
}

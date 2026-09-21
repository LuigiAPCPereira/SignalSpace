import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import vm from "node:vm";

const source = await readFile(new URL("./consent.js", import.meta.url), "utf8");
const requestID = "ABCDEFGHIJKLMNOPQRSTUV";

function makeResponse(status, body, headers = {}) {
  return {
    status,
    ok: status >= 200 && status < 300,
    headers: { get(name) { return headers[name] ?? null; } },
    async json() { return body; },
  };
}

async function runScript(responses) {
  const timers = [];
  const calls = [];
  const elements = {
    consent: { dataset: { requestId: requestID } },
    status: { textContent: "", dataset: {} },
    form: { listeners: {}, addEventListener(name, listener) { this.listeners[name] = listener; } },
    button: { disabled: false },
  };
  let responseIndex = 0;
  const document = {
    getElementById(id) {
      return { consent: elements.consent, "authorization-status": elements.status, "authorization-complete-form": elements.form, "continue-button": elements.button }[id] ?? null;
    },
  };
  const context = {
    document,
    encodeURIComponent,
    fetch: async (url, options) => {
      calls.push({ url, options });
      const next = responses[Math.min(responseIndex++, responses.length - 1)];
      if (next instanceof Error) throw next;
      return next;
    },
    window: {
      setTimeout(callback, delay) {
        timers.push({ callback, delay });
        return timers.length;
      },
      clearTimeout() {},
    },
  };
  vm.runInNewContext(source, context);
  await new Promise((resolve) => setImmediate(resolve));
  return { elements, timers, calls };
}

async function runNextTimer(state) {
  const timer = state.timers.shift();
  assert.ok(timer, "expected a scheduled poll");
  await timer.callback();
  await new Promise((resolve) => setImmediate(resolve));
  return timer;
}

test("polls the own request and enables completion only after APPROVED", async () => {
  const state = await runScript([
    makeResponse(200, { status: "PENDING" }),
    makeResponse(200, { status: "APPROVED" }),
  ]);
  assert.equal(state.elements.button.disabled, true);
  assert.equal(state.elements.status.dataset.state, "pending");
  assert.equal(state.calls[0].url, "/authorize/status?request_id=" + requestID);
	assert.equal(state.calls[0].options.method, "GET");
	assert.equal(state.calls[0].options.credentials, "same-origin");
	assert.equal(state.calls[0].options.cache, "no-store");
	assert.equal(state.calls[0].options.headers.Accept, "application/json");
  const timer = await runNextTimer(state);
  assert.equal(timer.delay, 2000);
  assert.equal(state.elements.button.disabled, false);
  assert.equal(state.elements.status.dataset.state, "approved");
  assert.equal(state.timers.length, 0);
});

for (const terminalState of ["DENIED", "EXPIRED"]) {
  test(`keeps completion disabled for ${terminalState}`, async () => {
    const state = await runScript([makeResponse(200, { status: terminalState })]);
    assert.equal(state.elements.button.disabled, true);
    assert.equal(state.elements.status.dataset.state, terminalState.toLowerCase());
    assert.equal(state.timers.length, 0);
  });
}

test("retries rate limits and transient or malformed responses without success", async () => {
  for (const [response, expectedDelay] of [
    [makeResponse(429, {}, { "Retry-After": "3" }), 3000],
    [makeResponse(503, {}), 5000],
    [makeResponse(200, { status: "UNKNOWN" }), 5000],
    [new Error("network lost"), 5000],
  ]) {
    const state = await runScript([response]);
    assert.equal(state.elements.button.disabled, true);
    assert.equal(state.elements.status.dataset.state, "retry");
    assert.equal(state.timers.length, 1);
    assert.equal(state.timers[0].delay, expectedDelay);
  }
});

for (const status of [403, 404]) {
  test(`stops without success for HTTP ${status}`, async () => {
    const state = await runScript([makeResponse(status, {})]);
    assert.equal(state.elements.button.disabled, true);
    assert.equal(state.elements.status.dataset.state, "error");
    assert.equal(state.timers.length, 0);
  });
}

test("submission remains guarded by the form handler", async () => {
  const state = await runScript([makeResponse(200, { status: "APPROVED" })]);
  assert.equal(state.elements.button.disabled, false);
  state.elements.form.listeners.submit();
  assert.equal(state.elements.button.disabled, true);
  assert.equal(state.elements.status.dataset.state, "completing");
});

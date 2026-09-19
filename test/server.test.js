import { after, before, test } from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { request as httpRequest } from 'node:http';
import { createServer } from '../src/server.js';

const TOKEN = 'local-diagnostic-token-for-tests-only-0123456789';
let server;
let endpoint;

before(async () => {
  server = createServer({ token: TOKEN });
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  endpoint = `http://127.0.0.1:${server.address().port}/mcp`;
});

after(async () => {
  await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
});

function post(body, { token = TOKEN, headers = {} } = {}) {
  return fetch(endpoint, {
    method: 'POST',
    headers: {
      'content-type': 'application/json',
      accept: 'application/json, text/event-stream',
      'mcp-protocol-version': '2025-06-18',
      ...(token ? { authorization: `Bearer ${token}` } : {}),
      ...headers,
    },
    body: typeof body === 'string' ? body : JSON.stringify(body),
  });
}

test('missing or incorrect credentials fail before protocol handling', async () => {
  for (const token of [null, 'incorrect']) {
    const response = await post({ jsonrpc: '2.0', id: 1, method: 'tools/list' }, { token });
    assert.equal(response.status, 401);
    assert.match(response.headers.get('www-authenticate'), /^Bearer /);
  }
});

test('configuration rejects missing and weak credentials', () => {
  assert.throws(() => createServer({ token: undefined }), /TOKEN/);
  assert.throws(() => createServer({ token: 'short' }), /TOKEN/);
});

test('initialization and authenticated diagnostic work', async () => {
  const initialize = await post({ jsonrpc: '2.0', id: 0, method: 'initialize', params: { protocolVersion: '2025-06-18' } });
  assert.equal(initialize.status, 200);
  const init = await initialize.json();
  assert.equal(init.id, 0);
  assert.equal(init.result.protocolVersion, '2025-06-18');
  const listed = await post({ jsonrpc: '2.0', id: 2, method: 'tools/list' });
  assert.equal((await listed.json()).result.tools[0].name, 'connection_diagnostic');
  const called = await post({ jsonrpc: '2.0', id: 3, method: 'tools/call', params: { name: 'connection_diagnostic', arguments: {} } });
  assert.equal(JSON.parse((await called.json()).result.content[0].text).connected, true);
});

test('rejects invalid messages and unexpected hosts without leaking secrets', async () => {
  const invalid = await post('{not-json');
  assert.equal(invalid.status, 400);
  assert.doesNotMatch(await invalid.text(), new RegExp(TOKEN));
  const wrongHost = await new Promise((resolve, reject) => {
    const request = httpRequest(endpoint, { method: 'POST', headers: { host: 'example.com', authorization: `Bearer ${TOKEN}` } }, (response) => {
      response.resume();
      response.on('end', () => resolve(response.statusCode));
    });
    request.on('error', reject);
    request.end(JSON.stringify({ jsonrpc: '2.0', id: 1, method: 'ping' }));
  });
  assert.equal(wrongHost, 403);
  const wrongOrigin = await post({ jsonrpc: '2.0', id: 1, method: 'ping' }, { headers: { origin: 'https://attacker.test' } });
  assert.equal(wrongOrigin.status, 403);
  const missingVersion = await post({ jsonrpc: '2.0', id: 1, method: 'tools/list' }, { headers: { 'mcp-protocol-version': '' } });
  assert.equal(missingVersion.status, 400);
  const oversized = await post('x'.repeat(70_000));
  assert.equal(oversized.status, 413);
});

test('unknown method, invalid tool parameters and notifications are explicit', async () => {
  const missing = await post({ jsonrpc: '2.0', id: 4, method: 'other' });
  assert.equal(missing.status, 404);
  assert.equal((await missing.json()).error.code, -32601);
  const badTool = await post({ jsonrpc: '2.0', id: 5, method: 'tools/call', params: { name: 'connection_diagnostic', arguments: { path: '/' } } });
  assert.equal((await badTool.json()).error.code, -32602);
  const notification = await post({ jsonrpc: '2.0', method: 'notifications/initialized' });
  assert.equal(notification.status, 202);
  assert.equal(await notification.text(), '');
});

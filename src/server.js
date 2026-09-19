import { createServer as createHttpServer } from 'node:http';
import { timingSafeEqual } from 'node:crypto';
import { pathToFileURL } from 'node:url';

const PROTOCOL_VERSION = '2025-06-18';
const MAX_BODY_BYTES = 64 * 1024;
const TOOL_NAME = 'connection_diagnostic';

function sendJson(response, status, payload) {
  response.writeHead(status, {
    'content-type': 'application/json; charset=utf-8',
    'cache-control': 'no-store',
    'x-content-type-options': 'nosniff',
  });
  response.end(JSON.stringify(payload));
}

function rpcError(id, code, message) {
  return { jsonrpc: '2.0', id, error: { code, message } };
}

function hasValidToken(header, expected) {
  if (typeof header !== 'string' || !header.startsWith('Bearer ')) return false;
  const supplied = Buffer.from(header.slice(7), 'utf8');
  const correct = Buffer.from(expected, 'utf8');
  return supplied.length === correct.length && timingSafeEqual(supplied, correct);
}

function readBody(request) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    let exceeded = false;
    request.on('data', (chunk) => {
      if (exceeded) return;
      size += chunk.length;
      if (size > MAX_BODY_BYTES) {
        exceeded = true;
        chunks.length = 0;
        reject(new Error('body_too_large'));
        return;
      }
      chunks.push(chunk);
    });
    request.on('end', () => resolve(Buffer.concat(chunks).toString('utf8')));
    request.on('error', reject);
    request.on('aborted', () => reject(new Error('aborted')));
  });
}

function handleMessage(message) {
  if (message === null || typeof message !== 'object' || Array.isArray(message) ||
      message.jsonrpc !== '2.0' || typeof message.method !== 'string' ||
      (Object.hasOwn(message, 'id') && !(
        typeof message.id === 'string' ||
        (typeof message.id === 'number' && Number.isInteger(message.id))
      ))) {
    return { status: 400, payload: rpcError(null, -32600, 'Invalid Request') };
  }

  if (!Object.hasOwn(message, 'id')) {
    // Notificações não recebem resposta JSON-RPC.
    return { status: 202 };
  }

  const id = message.id;
  if (message.method === 'initialize') {
    if (message.params === null || typeof message.params !== 'object' ||
        typeof message.params.protocolVersion !== 'string') {
      return { status: 400, payload: rpcError(id, -32602, 'Invalid params') };
    }
    return {
      status: 200,
      payload: {
        jsonrpc: '2.0', id,
        result: {
          protocolVersion: PROTOCOL_VERSION,
          capabilities: { tools: {} },
          serverInfo: { name: 'signalspace', version: '0.1.0' },
          instructions: 'Local diagnostic only. OAuth and development tools are not available.',
        },
      },
    };
  }
  if (message.method === 'ping') return { status: 200, payload: { jsonrpc: '2.0', id, result: {} } };
  if (message.method === 'tools/list') {
    return {
      status: 200,
      payload: {
        jsonrpc: '2.0', id,
        result: {
          tools: [{
            name: TOOL_NAME,
            description: 'Check whether this authenticated local MCP endpoint is reachable. Does not access files or execute commands.',
            inputSchema: { type: 'object', properties: {}, additionalProperties: false },
            annotations: { readOnlyHint: true, destructiveHint: false },
          }],
        },
      },
    };
  }
  if (message.method === 'tools/call') {
    if (message.params?.name !== TOOL_NAME || message.params?.arguments === null ||
        typeof message.params?.arguments !== 'object' ||
        Array.isArray(message.params?.arguments) ||
        Object.keys(message.params.arguments).length !== 0) {
      return { status: 200, payload: rpcError(id, -32602, 'Invalid params') };
    }
    return {
      status: 200,
      payload: {
        jsonrpc: '2.0', id,
        result: {
          content: [{ type: 'text', text: JSON.stringify({ connected: true, mode: 'local_diagnostic', chatgptVerified: false }) }],
          isError: false,
        },
      },
    };
  }
  return { status: 404, payload: rpcError(id, -32601, 'Method not found') };
}

export function createServer({ token }) {
  if (typeof token !== 'string' || Buffer.byteLength(token, 'utf8') < 32 || /\s/.test(token)) {
    throw new Error('SIGNALSPACE_LOCAL_TOKEN must be at least 32 non-whitespace characters');
  }

  const server = createHttpServer(async (request, response) => {
    // O serviço permanece local e rejeita Host/Origin inesperados (DNS rebinding).
    const port = server.address()?.port;
    const allowedHosts = new Set([`127.0.0.1:${port}`, `localhost:${port}`]);
    if (!allowedHosts.has(request.headers.host) ||
        (request.headers.origin && !new Set([...allowedHosts].map((host) => `http://${host}`)).has(request.headers.origin))) {
      response.writeHead(403, { 'cache-control': 'no-store' });
      response.end();
      return;
    }
    if (request.url !== '/mcp') {
      response.writeHead(404);
      response.end();
      return;
    }
    if (!hasValidToken(request.headers.authorization, token)) {
      response.writeHead(401, { 'www-authenticate': 'Bearer realm="signalspace-local"', 'cache-control': 'no-store' });
      response.end();
      return;
    }
    if (request.method !== 'POST') {
      response.writeHead(405, { allow: 'POST' });
      response.end();
      return;
    }
    if (!/^application\/json(?:\s*;|$)/i.test(request.headers['content-type'] ?? '')) {
      sendJson(response, 415, rpcError(null, -32600, 'Unsupported media type'));
      return;
    }
    const declaredLength = Number(request.headers['content-length']);
    if (request.headers['content-length'] && (!Number.isSafeInteger(declaredLength) || declaredLength > MAX_BODY_BYTES)) {
      response.writeHead(413, { connection: 'close' });
      response.end();
      return;
    }
    let message;
    try {
      message = JSON.parse(await readBody(request));
    } catch (error) {
      sendJson(response, error.message === 'body_too_large' ? 413 : 400,
        rpcError(null, error.message === 'body_too_large' ? -32600 : -32700,
          error.message === 'body_too_large' ? 'Request too large' : 'Parse error'));
      return;
    }
    if ((message?.method !== 'initialize' && request.headers['mcp-protocol-version'] !== PROTOCOL_VERSION) ||
        (request.headers['mcp-protocol-version'] && request.headers['mcp-protocol-version'] !== PROTOCOL_VERSION)) {
      sendJson(response, 400, rpcError(null, -32600, 'Unsupported MCP protocol version'));
      return;
    }
    const result = handleMessage(message);
    if (result.status === 202) {
      response.writeHead(202, { 'cache-control': 'no-store' });
      response.end();
      return;
    }
    sendJson(response, result.status, result.payload);
  });
  server.requestTimeout = 10_000;
  server.headersTimeout = 5_000;
  server.maxRequestsPerSocket = 50;
  return server;
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    const server = createServer({ token: process.env.SIGNALSPACE_LOCAL_TOKEN });
    server.listen(7676, '127.0.0.1', () => {
      console.log('SignalSpace local diagnostic: http://127.0.0.1:7676/mcp');
      console.log('Not ready for ChatGPT Web: OAuth and tunnel integration are not implemented.');
    });
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}

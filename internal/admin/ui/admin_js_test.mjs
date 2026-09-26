import test from 'node:test';
import assert from 'node:assert/strict';
import { createApp } from './admin.js';

class FakeElement {
  constructor(tagName = 'div') {
    this.tagName = tagName;
    this.children = [];
    this.listeners = new Map();
    this.dataset = {};
    this.hidden = false;
    this.disabled = false;
    this.value = '';
    this._text = '';
    this.formValues = {};
  }

  set textContent(value) { this._text = String(value ?? ''); this.children = []; }
  get textContent() { return this._text + this.children.map((child) => child.textContent).join(''); }
  append(...children) { this.children.push(...children.filter(Boolean)); }
  replaceChildren(...children) { this._text = ''; this.children = children.filter(Boolean); }
  addEventListener(type, listener) { this.listeners.set(type, listener); }
  dispatch(type, extra = {}) {
    const listener = this.listeners.get(type);
    if (!listener) return;
    const event = { currentTarget: this, preventDefault() {}, ...extra };
    listener(event);
  }
  querySelector(selector) {
    if (selector === 'button[type="submit"]') return this.children.find((child) => child.tagName === 'button' && child.type === 'submit') || null;
    return this.find((child) => selector === '[data-request-id]' && child.dataset.requestId);
  }
  querySelectorAll(selector) {
    const result = [];
    this.walk((child) => {
      if (selector === '[data-request-id]' && child.dataset.requestId) result.push(child);
    });
    return result;
  }
  find(predicate) {
    let found = null;
    this.walk((child) => { if (!found && predicate(child)) found = child; });
    return found;
  }
  walk(visitor) { for (const child of this.children) { visitor(child); child.walk(visitor); } }
  reset() { this.formValues = {}; }
  focus() { this.focused = true; }
}

const ids = [
  'page-title', 'page-lead', 'session-badge', 'session-status', 'error', 'loading-section', 'pair-section', 'unlock-section',
  'unknown-section', 'unavailable-section', 'authenticated-section', 'pair-form', 'unlock-form', 'requests-status', 'request-count',
  'requests', 'request-detail', 'detail-title', 'detail-status', 'detail-message', 'detail-client-name', 'detail-client-note', 'detail-client-id',
  'detail-redirect', 'detail-scope', 'detail-grant', 'detail-version', 'detail-expires', 'detail-id', 'decision-actions',
  'deny-request', 'approve-request', 'sidebar-state', 'idle-expires', 'absolute-expires', 'session-actions', 'refresh-session',
  'lock-session', 'retry-session', 'recheck-session'
];

class FakeDocument {
  constructor() {
    this.elements = new Map(ids.map((id) => [id, new FakeElement(id.endsWith('form') ? 'form' : 'div')]));
    for (const id of ['pair-form', 'unlock-form']) {
      const submit = new FakeElement('button'); submit.type = 'submit';
      this.elements.get(id).append(submit);
    }
  }
  getElementById(id) { return this.elements.get(id) || null; }
  createElement(tagName) { return new FakeElement(tagName); }
}

class FakeFormData {
  constructor(form) { this.form = form; }
  entries() { return Object.entries(this.form.formValues); }
}

const authSession = {
  state: 'AUTHENTICATED', authenticated: true, csrf_token: 'csrf-auth',
  idle_expires_at: '2026-09-21T14:00:00Z', absolute_expires_at: '2026-09-21T15:00:00Z'
};
const lockedSession = { state: 'LOCKED', authenticated: false, csrf_token: 'csrf-bootstrap', server_time: '2026-09-21T13:00:00Z' };
const unpairedSession = { state: 'UNPAIRED', authenticated: false, csrf_token: 'csrf-bootstrap', server_time: '2026-09-21T13:00:00Z' };
const request = (overrides = {}) => ({
  id: 'req-1', version: 7, status: 'PENDING',
  client: { id: 'client-1', display_name: 'Cliente registrado', verified: false },
  redirect_uri: 'http://localhost:7676/callback', scope: 'signalspace:diagnostic',
  workspace_read: { required: false, grant_status: 'NOT_APPLICABLE' },
  created_at: '2026-09-21T12:00:00Z', expires_at: '2026-09-21T12:05:00Z', decided_at: null,
  ...overrides
});

function jsonResponse(body, status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => body };
}

function invalidJSONResponse(status = 200) {
  return { ok: status >= 200 && status < 300, status, json: async () => { throw new SyntaxError('truncated response'); } };
}

function harness(plans, extraIds = []) {
  const documentRef = new FakeDocument();
  for (const id of extraIds) documentRef.elements.set(id, new FakeElement('div'));
  const calls = [];
  let index = 0;
  const fetcher = async (path, options) => {
    calls.push({ path, options });
    const plan = plans[index++];
    if (!plan) throw new Error(`unexpected request ${path}`);
    if (typeof plan === 'function') return plan(path, options);
    if (plan instanceof Error) throw plan;
    return plan;
  };
  const app = createApp(documentRef, fetcher, FakeFormData);
  return { app, documentRef, calls };
}

async function settle() {
  for (let i = 0; i < 8; i += 1) await new Promise((resolve) => setImmediate(resolve));
}

function element(documentRef, id) { return documentRef.getElementById(id); }

test('inicialização não exibe pedidos antes da sessão', async () => {
  const h = harness([jsonResponse(unpairedSession)]);
  h.app.start(); await settle();
  assert.equal(element(h.documentRef, 'requests').children.length, 0);
  assert.equal(element(h.documentRef, 'pair-section').hidden, false);
});

test('UNPAIRED permite parear sem pedir uma segunda autenticação', async () => {
  const h = harness([jsonResponse(unpairedSession), jsonResponse(authSession, 201), jsonResponse({ requests: [] })]);
  h.app.start(); await settle();
  const form = element(h.documentRef, 'pair-form');
  form.formValues = { pairing_code: 'terminal-code', passphrase: 'disposable-passphrase' };
  form.dispatch('submit'); await settle();
  assert.deepEqual(h.calls.map((call) => call.path), ['/api/admin/v1/session', '/api/admin/v1/pair', '/api/admin/v1/requests']);
  assert.equal(form.formValues.passphrase, undefined);
});

test('LOCKED permite desbloquear somente com frase-senha', async () => {
  const h = harness([jsonResponse(lockedSession), jsonResponse(authSession), jsonResponse({ requests: [] })]);
  h.app.start(); await settle();
  const form = element(h.documentRef, 'unlock-form');
  form.formValues = { passphrase: 'disposable-passphrase' };
  form.dispatch('submit'); await settle();
  assert.equal(h.calls[1].path, '/api/admin/v1/unlock');
  assert.equal(JSON.parse(h.calls[1].options.body).passphrase, 'disposable-passphrase');
  assert.equal(JSON.parse(h.calls[1].options.body).pairing_code, undefined);
});

test('desbloqueio rejeitado permanece LOCKED e recupera o bootstrap sem repetir POST', async () => {
  const h = harness([
    jsonResponse(lockedSession),
    jsonResponse({ error: { code: 'ACCESS_DENIED', message: 'Acesso negado.' } }, 403),
    jsonResponse(lockedSession)
  ]);
  h.app.start(); await settle();
  const form = element(h.documentRef, 'unlock-form');
  form.formValues = { passphrase: 'wrong-passphrase' };
  form.dispatch('submit'); await settle();
  assert.deepEqual(h.calls.map((call) => call.path), ['/api/admin/v1/session', '/api/admin/v1/unlock', '/api/admin/v1/session']);
  assert.equal(h.calls.filter((call) => call.path === '/api/admin/v1/unlock').length, 1);
  assert.equal(h.app.state.session.state, 'LOCKED');
  assert.equal(element(h.documentRef, 'unlock-section').hidden, false);
  assert.equal(element(h.documentRef, 'unavailable-section').hidden, true);
  assert.match(element(h.documentRef, 'error').textContent, /servidor recusou/);
});

test('resposta perdida no pareamento reconcilia por sessão sem repetir POST', async () => {
  for (const lost of [new Error('connection lost'), invalidJSONResponse(201)]) {
    const h = harness([jsonResponse(unpairedSession), lost, jsonResponse(lockedSession)]);
    h.app.start(); await settle();
    const form = element(h.documentRef, 'pair-form');
    form.formValues = { pairing_code: 'terminal-code', passphrase: 'disposable-passphrase' };
    form.dispatch('submit'); await settle();
    assert.deepEqual(h.calls.map((call) => call.path), ['/api/admin/v1/session', '/api/admin/v1/pair', '/api/admin/v1/session']);
    assert.equal(h.calls.filter((call) => call.path === '/api/admin/v1/pair').length, 1);
    assert.equal(element(h.documentRef, 'unlock-section').hidden, false);
    assert.equal(h.app.state.session.state, 'LOCKED');
  }
});

test('resposta perdida no desbloqueio reconcilia por sessão sem repetir POST', async () => {
  for (const lost of [new Error('connection lost'), invalidJSONResponse(200)]) {
    const h = harness([jsonResponse(lockedSession), lost, jsonResponse(authSession), jsonResponse({ requests: [] })]);
    h.app.start(); await settle();
    const form = element(h.documentRef, 'unlock-form');
    form.formValues = { passphrase: 'disposable-passphrase' };
    form.dispatch('submit'); await settle();
    assert.deepEqual(h.calls.map((call) => call.path), ['/api/admin/v1/session', '/api/admin/v1/unlock', '/api/admin/v1/session', '/api/admin/v1/requests']);
    assert.equal(h.calls.filter((call) => call.path === '/api/admin/v1/unlock').length, 1);
    assert.equal(element(h.documentRef, 'authenticated-section').hidden, false);
    assert.equal(h.app.state.session.state, 'AUTHENTICATED');
  }
});

test('AUTHENTICATED carrega pedidos reais da API', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] })]);
  h.app.start(); await settle();
  assert.equal(element(h.documentRef, 'requests').children.length, 1);
  assert.match(element(h.documentRef, 'requests').textContent, /Cliente registrado/);
});

test('detalhes deixam explícito que o nome do cliente não é atestação', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse(request())]);
  h.app.start(); await settle();
  await h.app.loadRequestDetail('req-1'); await settle();
  assert.match(element(h.documentRef, 'detail-client-note').textContent, /não atestado/);
  assert.match(element(h.documentRef, 'detail-client-id').textContent, /client-1/);
});

test('decisão usa expected_version do detalhe', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse(request()), jsonResponse(request({ status: 'APPROVED', decided_at: '2026-09-21T12:01:00Z' })), jsonResponse({ requests: [request({ status: 'APPROVED' })] })]);
  h.app.start(); await settle(); await h.app.loadRequestDetail('req-1'); await settle();
  await h.app.decide(request(), 'approve'); await settle();
  const decision = h.calls.find((call) => call.path.endsWith('/decision'));
  assert.deepEqual(JSON.parse(decision.options.body), { decision: 'approve', expected_version: 7 });
});

test('duplo clique lógico não duplica POST de decisão', async () => {
  let resolvePost;
  const post = new Promise((resolve) => { resolvePost = resolve; });
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse(request()), () => post, jsonResponse({ requests: [request({ status: 'APPROVED' })] })]);
  h.app.start(); await settle(); await h.app.loadRequestDetail('req-1'); await settle();
  const first = h.app.decide(request(), 'approve');
  const second = h.app.decide(request(), 'approve');
  assert.equal(h.calls.filter((call) => call.path.endsWith('/decision')).length, 1);
  resolvePost(jsonResponse(request({ status: 'APPROVED' }))); await Promise.all([first, second]); await settle();
});

test('perda de resposta reconcilia por GET sem repetir o POST', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse(request()), new Error('connection lost'), jsonResponse(request({ status: 'APPROVED' })), jsonResponse({ requests: [request({ status: 'APPROVED' })] })]);
  h.app.start(); await settle(); await h.app.loadRequestDetail('req-1'); await settle();
  await h.app.decide(request(), 'approve'); await settle();
  assert.equal(h.calls.filter((call) => call.path.endsWith('/decision')).length, 1);
  assert.equal(h.calls.filter((call) => call.path === '/api/admin/v1/requests/req-1').length, 2);
  assert.match(element(h.documentRef, 'detail-message').textContent, /APPROVED/);
});

test('409 não produz aprovação otimista e bloqueia nova decisão', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse(request()), jsonResponse({ error: { code: 'STALE_REQUEST', message: 'stale' } }, 409), jsonResponse(request()), jsonResponse({ requests: [request()] })]);
  h.app.start(); await settle(); await h.app.loadRequestDetail('req-1'); await settle();
  await h.app.decide(request(), 'approve'); await settle();
  assert.equal(element(h.documentRef, 'detail-status').textContent, 'PENDING');
  assert.equal(element(h.documentRef, 'approve-request').disabled, true);
  assert.match(element(h.documentRef, 'detail-message').textContent, /Resultado não confirmado/);
});

test('401 limpa pedidos e ações privilegiados', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse({ error: { code: 'AUTH_REQUIRED', message: 'locked' } }, 401)]);
  h.app.start(); await settle(); await h.app.loadRequestDetail('req-1'); await settle();
  assert.equal(element(h.documentRef, 'requests').children.length, 0);
  assert.equal(element(h.documentRef, 'authenticated-section').hidden, true);
});

test('429, 503 e rede não são mostrados como fila vazia', async (t) => {
  for (const failure of [
    jsonResponse({ error: { code: 'RATE_LIMITED', message: 'slow down' } }, 429),
    jsonResponse({ error: { code: 'TEMPORARILY_UNAVAILABLE', message: 'down' } }, 503),
    new Error('network down')
  ]) {
    await t.test(failure instanceof Error ? 'rede' : failure.status === 429 ? '429' : '503', async () => {
      const h = harness([jsonResponse(authSession), failure]);
      h.app.start(); await settle();
      assert.equal(element(h.documentRef, 'requests').children.length, 0);
      assert.doesNotMatch(element(h.documentRef, 'requests-status').textContent, /Nenhuma solicitação/);
    });
  }
});

test('resposta antiga de /requests após lock não restaura a lista', async () => {
  let resolveRequests;
  const pendingRequests = new Promise((resolve) => { resolveRequests = resolve; });
  const h = harness([jsonResponse(authSession), () => pendingRequests, jsonResponse(lockedSession)]);
  h.app.start(); await settle();
  element(h.documentRef, 'lock-session').dispatch('click'); await settle();
  assert.equal(element(h.documentRef, 'authenticated-section').hidden, true);
  resolveRequests(jsonResponse({ requests: [request()] })); await settle();
  assert.equal(element(h.documentRef, 'requests').children.length, 0);
  assert.equal(element(h.documentRef, 'sidebar-state').textContent, 'Bloqueado');
});

test('mudança de sessão libera bloqueios de decisão da sessão anterior', () => {
  const h = harness([]);
  h.app.state.blockedDecisions.add('req-1');
  h.app.state.decisionInFlight.add('req-1');
  h.app.renderSession(authSession);
  assert.equal(h.app.state.blockedDecisions.size, 0);
  assert.equal(h.app.state.decisionInFlight.size, 0);
});

test('refresh não acontece automaticamente', async () => {
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [] })]);
  h.app.start(); await settle();
  assert.equal(h.calls.some((call) => call.path.endsWith('/session/refresh')), false);
});

test('nome malicioso entra como texto e não executa HTML', async () => {
  const malicious = request({ client: { id: 'evil', display_name: '<img src=x onerror=alert(1)>', verified: true } });
  const h = harness([jsonResponse(authSession), jsonResponse({ requests: [malicious] })]);
  h.app.start(); await settle();
  const row = element(h.documentRef, 'requests').children[0];
  assert.equal(row.children[0].children[0].textContent, '<img src=x onerror=alert(1)>');
  assert.equal(row.children[0].children.length, 2);
});

test('aprovações de capability usam allowed_decisions e reconciliação própria', async () => {
  const approval = {
    request_id: 'apr-1', version: 3, status: 'PENDING', capability: 'workspace.write', tool: 'write_text_file',
    safe_summary: 'Write src/main.go', allowed_decisions: ['ALLOW_SESSION', 'DENY']
  };
  const h = harness([
    jsonResponse(authSession), jsonResponse({ requests: [] }), jsonResponse({ approvals: [approval] }), jsonResponse({ policies: [] }),
    jsonResponse({ request_id: 'apr-1', status: 'APPROVED', version: 4 }), jsonResponse({ approvals: [] })
  ], ['capability-approvals-status', 'capability-approval-count', 'capability-approvals', 'capability-policies-status', 'capability-policies']);
  h.app.start(); await settle();
  assert.equal(element(h.documentRef, 'capability-approvals').children.length, 1);
  assert.match(element(h.documentRef, 'capability-approvals').textContent, /Permitir nesta sessão/);
  await h.app.decideCapability(approval, 'ALLOW_SESSION');
  assert.equal(h.calls[4].path, '/api/admin/v1/capability-approvals/apr-1/decision');
  assert.equal(JSON.parse(h.calls[4].options.body).decision, 'ALLOW_SESSION');
  assert.equal(element(h.documentRef, 'capability-approval-count').textContent, '0');
});

test('GET 404 e 410 são apresentados distintamente', async () => {
  for (const [status, code, expected] of [[404, 'REQUEST_NOT_FOUND', /não significa que ela expirou/], [410, 'REQUEST_EXPIRED', /Solicitação expirada/]]) {
    const h = harness([jsonResponse(authSession), jsonResponse({ requests: [request()] }), jsonResponse({ error: { code, message: 'failure' } }, status)]);
    h.app.start(); await settle(); await h.app.loadRequestDetail('req-1'); await settle();
    assert.match(element(h.documentRef, 'detail-message').textContent, expected);
  }
});

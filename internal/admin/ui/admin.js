(() => {
  'use strict';

  const state = { csrf: '', session: null };
  const byId = (id) => document.getElementById(id);
  const statusElement = byId('session-status');
  const errorElement = byId('error');
  const requestsElement = byId('requests');
  const requestsStatus = byId('requests-status');

  function showError(message) {
    errorElement.textContent = message || '';
    errorElement.hidden = !message;
  }

  async function readResponse(response) {
    let body = null;
    try {
      body = await response.json();
    } catch (_) {
      throw new Error('Resposta JSON inválida.');
    }
    if (!response.ok) {
      const error = new Error(body?.error?.message || 'Operação indisponível.');
      error.status = response.status;
      error.body = body;
      throw error;
    }
    return body;
  }

  async function request(path, options = {}) {
    const headers = { Accept: 'application/json', ...(options.headers || {}) };
    if (options.body) headers['Content-Type'] = 'application/json';
    if (options.method && options.method !== 'GET') headers['X-CSRF-Token'] = state.csrf;
    const response = await fetch(path, { ...options, headers, credentials: 'same-origin' });
    return readResponse(response);
  }

  function clearAuthenticatedData(clearCsrf = true) {
    if (clearCsrf) state.csrf = '';
    state.session = null;
    requestsElement.replaceChildren();
    requestsStatus.textContent = '';
  }

  function renderSession(session) {
    state.session = session;
    state.csrf = session?.csrf_token || '';
    statusElement.textContent = `Estado da sessão: ${session?.state || 'DESCONHECIDO'}.`;
    byId('pair-section').hidden = session?.state !== 'UNPAIRED';
    byId('unlock-section').hidden = session?.state !== 'LOCKED';
    byId('authenticated-section').hidden = session?.state !== 'AUTHENTICATED';
    // O CSRF de bootstrap continua necessário para pair/unlock; só dados
    // privilegiados devem ser descartados ao renderizar UNPAIRED/LOCKED.
    if (session?.state !== 'AUTHENTICATED') clearAuthenticatedData(false);
  }

  async function loadSession() {
    try {
      const session = await request('/api/admin/v1/session');
      showError('');
      renderSession(session);
      if (session.state === 'AUTHENTICATED') await loadRequests();
    } catch (error) {
      clearAuthenticatedData();
      const session = error.body?.session;
      if (error.status === 401 && session) renderSession(session);
      showError(error.message);
    }
  }

  async function submitCredentials(path, form) {
    const data = Object.fromEntries(new FormData(form).entries());
    const button = form.querySelector('button[type="submit"]');
    button.disabled = true;
    try {
      const session = await request(path, { method: 'POST', body: JSON.stringify(data) });
      form.reset();
      showError('');
      renderSession(session);
      await loadRequests();
    } catch (error) {
      showError(error.message);
      if (error.status === 401 && error.body?.session) renderSession(error.body.session);
    } finally {
      button.disabled = false;
    }
  }

  function requestLabel(item) {
    const scope = Array.isArray(item.scope) ? item.scope.join(', ') : String(item.scope || '');
    return `${item.client?.display_name || 'Cliente não atestado'} — ${scope} — versão ${item.version}`;
  }

  async function loadRequests() {
    if (state.session?.state !== 'AUTHENTICATED') return;
    requestsStatus.textContent = 'Consultando solicitações…';
    try {
      const body = await request('/api/admin/v1/requests');
      requestsElement.replaceChildren();
      for (const item of body.requests || []) {
        const row = document.createElement('li');
        row.dataset.requestId = item.id;
        row.textContent = requestLabel(item);
        if (item.status === 'PENDING') {
          for (const decision of ['approve', 'deny']) {
            const button = document.createElement('button');
            button.type = 'button';
            button.textContent = decision === 'approve' ? 'Aprovar' : 'Recusar';
            button.addEventListener('click', () => decide(item, decision, button));
            row.append(' ', button);
          }
        }
        requestsElement.append(row);
      }
      requestsStatus.textContent = body.requests?.length ? '' : 'Nenhuma solicitação pendente.';
    } catch (error) {
      requestsStatus.textContent = 'Solicitações indisponíveis.';
      if (error.status === 401) await loadSession();
      else showError(error.message);
    }
  }

  async function reconcile(id) {
    try {
      const item = await request(`/api/admin/v1/requests/${encodeURIComponent(id)}`);
      showError(`Estado atual reconciliado: ${item.status}.`);
    } catch (error) {
      showError(`Não foi possível reconciliar a solicitação: ${error.message}`);
    }
    await loadRequests();
  }

  async function decide(item, decision, button) {
    button.disabled = true;
    const row = button.closest('li');
    row.querySelectorAll('button').forEach((candidate) => { candidate.disabled = true; });
    try {
      await request(`/api/admin/v1/requests/${encodeURIComponent(item.id)}/decision`, {
        method: 'POST',
        body: JSON.stringify({ decision, expected_version: item.version }),
      });
      showError('Decisão registrada.');
    } catch (error) {
      // Perda de resposta, conflito ou expiração nunca repetem o POST: GET reconcilia.
      await reconcile(item.id);
      if (error.status !== 401) showError(`Decisão não confirmada: ${error.message}`);
    }
    await loadRequests();
  }

  byId('pair-form').addEventListener('submit', (event) => { event.preventDefault(); submitCredentials('/api/admin/v1/pair', event.currentTarget); });
  byId('unlock-form').addEventListener('submit', (event) => { event.preventDefault(); submitCredentials('/api/admin/v1/unlock', event.currentTarget); });
  byId('refresh-session').addEventListener('click', async () => {
    try { renderSession(await request('/api/admin/v1/session/refresh', { method: 'POST', body: '{}' })); await loadRequests(); } catch (error) { showError(error.message); }
  });
  byId('lock-session').addEventListener('click', async () => {
    try { renderSession(await request('/api/admin/v1/lock', { method: 'POST', body: '{}' })); } catch (error) { showError(error.message); }
  });
  loadSession();
})();

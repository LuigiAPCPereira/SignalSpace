(() => {
  'use strict';

  const createApp = (documentRef, fetcher, FormDataCtor) => {
    const byId = (id) => documentRef.getElementById(id);
    const elements = {
      pageTitle: byId('page-title'), pageLead: byId('page-lead'), badge: byId('session-badge'),
      status: byId('session-status'), error: byId('error'), loading: byId('loading-section'),
      pair: byId('pair-section'), unlock: byId('unlock-section'), unknown: byId('unknown-section'),
      unavailable: byId('unavailable-section'), authenticated: byId('authenticated-section'),
      pairForm: byId('pair-form'), unlockForm: byId('unlock-form'),
      requestsStatus: byId('requests-status'), requestCount: byId('request-count'), requests: byId('requests'),
      detail: byId('request-detail'), detailTitle: byId('detail-title'), detailStatus: byId('detail-status'),
      detailMessage: byId('detail-message'), detailClientName: byId('detail-client-name'), detailClientNote: byId('detail-client-note'), detailClientId: byId('detail-client-id'),
      detailRedirect: byId('detail-redirect'), detailScope: byId('detail-scope'), detailGrant: byId('detail-grant'),
      detailVersion: byId('detail-version'), detailExpires: byId('detail-expires'), detailId: byId('detail-id'),
      decisionActions: byId('decision-actions'), deny: byId('deny-request'), approve: byId('approve-request'),
      sidebarState: byId('sidebar-state'), idleExpires: byId('idle-expires'), absoluteExpires: byId('absolute-expires'),
      sessionActions: byId('session-actions'), refresh: byId('refresh-session'), lock: byId('lock-session'),
      retry: byId('retry-session'), recheck: byId('recheck-session')
    };
    const state = {
      csrf: '', session: null, generation: 0, selected: null, detail: null,
      decisionInFlight: new Set(), blockedDecisions: new Set(), requestController: null
    };

    function abortRequestLoad() {
      if (state.requestController) state.requestController.abort();
      state.requestController = null;
    }

    function nextGeneration() {
      state.generation += 1;
      abortRequestLoad();
      return state.generation;
    }

    function showError(message) {
      elements.error.textContent = message || '';
      elements.error.hidden = !message;
    }

    function setMessage(element, message, tone = '') {
      element.textContent = message || '';
      element.dataset.tone = tone;
    }

    function formatServerTime(value) {
      if (!value) return 'n/d';
      const date = new Date(value);
      return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('pt-BR', { dateStyle: 'medium', timeStyle: 'short' });
    }

    function stateCopyText(sessionState) {
      return { LOADING: 'Carregando', UNPAIRED: 'Não pareado', LOCKED: 'Bloqueado', AUTHENTICATED: 'Autenticado', UNAVAILABLE: 'Indisponível', UNKNOWN_RESULT: 'Resultado desconhecido' }[sessionState] || 'Desconhecido';
    }

    function statePresentation(sessionState) {
      return {
        LOADING: ['Verificando o acesso local', 'Consultando a sessão administrativa antes de exibir qualquer pedido.', 'neutral'],
        UNPAIRED: ['Parear esta instalação', 'Primeiro prove o controle desta instalação. Depois, o painel poderá consultar pedidos reais.', 'warning'],
        LOCKED: ['Desbloquear painel local', 'Esta instalação já foi pareada. Informe a frase-senha para iniciar uma sessão nova.', 'neutral'],
        AUTHENTICATED: ['Solicitações de conexão', 'Revise o solicitante e o escopo exato antes de registrar uma decisão no servidor.', 'success'],
        UNAVAILABLE: ['Serviço indisponível', 'Sem uma resposta confiável do backend, pedidos e ações permanecem ocultos.', 'danger'],
        UNKNOWN_RESULT: ['Resultado desconhecido', 'A operação não foi confirmada. Nenhuma credencial ou decisão será repetida automaticamente.', 'warning']
      }[sessionState];
    }

    function clearRequestDom(message = '') {
      elements.requests.replaceChildren();
      elements.requestCount.textContent = 'n/d';
      setMessage(elements.requestsStatus, message);
      elements.detail.hidden = true;
      elements.decisionActions.hidden = true;
      elements.detailMessage.textContent = '';
      state.selected = null;
      state.detail = null;
    }

    function setShell(sessionState) {
      const [title, lead, tone] = statePresentation(sessionState);
      elements.pageTitle.textContent = title;
      elements.pageLead.textContent = lead;
      elements.badge.textContent = stateCopyText(sessionState);
      elements.badge.dataset.tone = tone;
      elements.status.textContent = sessionState === 'LOADING' ? 'Carregando sessão local…' : `Estado da sessão: ${sessionState}.`;
      elements.sidebarState.textContent = stateCopyText(sessionState);
      elements.loading.hidden = sessionState !== 'LOADING';
      elements.pair.hidden = sessionState !== 'UNPAIRED';
      elements.unlock.hidden = sessionState !== 'LOCKED';
      elements.unknown.hidden = sessionState !== 'UNKNOWN_RESULT';
      elements.unavailable.hidden = sessionState !== 'UNAVAILABLE';
      elements.authenticated.hidden = sessionState !== 'AUTHENTICATED';
      elements.sessionActions.hidden = sessionState !== 'AUTHENTICATED';
    }

    function clearPrivilegedData() {
      clearRequestDom('Nenhuma solicitação disponível enquanto a sessão não estiver autenticada.');
    }

    function renderSession(session) {
      nextGeneration();
      state.session = session || null;
      state.csrf = session?.csrf_token || '';
      const sessionState = session?.state || 'UNKNOWN_RESULT';
      setShell(sessionState);
      clearPrivilegedData();
      elements.idleExpires.textContent = formatServerTime(session?.idle_expires_at);
      elements.absoluteExpires.textContent = formatServerTime(session?.absolute_expires_at);
      if (sessionState === 'AUTHENTICATED') setMessage(elements.requestsStatus, 'Consultando solicitações…');
      showError('');
    }

    function renderTransient(sessionState, message) {
      nextGeneration();
      state.session = null;
      state.csrf = '';
      setShell(sessionState);
      clearPrivilegedData(message);
      elements.idleExpires.textContent = 'n/d';
      elements.absoluteExpires.textContent = 'n/d';
    }

    function makeError(message, code, status, body) {
      const error = new Error(message);
      error.code = code; error.status = status; error.body = body;
      return error;
    }

    function errorMessage(error) {
      const code = error.body?.error?.code || error.code;
      const known = {
        AUTH_REQUIRED: 'A sessão administrativa não está autenticada.',
        ACCESS_DENIED: 'O servidor recusou esta operação.',
        REQUEST_NOT_FOUND: 'Solicitação não encontrada (404); isso não significa que ela expirou.',
        REQUEST_EXPIRED: 'Solicitação expirada (410); um novo pedido será necessário.',
        STALE_REQUEST: 'A solicitação mudou; o estado atual precisa ser consultado.',
        ALREADY_DECIDED: 'A solicitação já foi decidida no servidor.',
        WORKSPACE_GRANT_REQUIRED: 'A concessão de workspace necessária não está ativa; ela é um fluxo separado.',
        RATE_LIMITED: 'O servidor limitou novas tentativas temporariamente (429).',
        TEMPORARILY_UNAVAILABLE: 'O serviço está temporariamente indisponível (503).',
        INVALID_REQUEST: 'A requisição foi rejeitada pelo servidor.',
        INVALID_JSON: 'O servidor enviou uma resposta JSON inválida.',
        NETWORK: 'A conexão com o serviço local foi perdida.'
      };
      return known[code] || error.message || 'Operação indisponível.';
    }

    async function readResponse(response) {
      let body;
      try { body = await response.json(); } catch (_) { throw makeError('Resposta JSON inválida.', 'INVALID_JSON', response.status); }
      if (!response.ok) throw makeError(body?.error?.message || 'Operação indisponível.', body?.error?.code || 'HTTP_ERROR', response.status, body);
      return body;
    }

    async function request(path, options = {}, csrfOverride = null) {
      const headers = { Accept: 'application/json', ...(options.headers || {}) };
      if (options.body) headers['Content-Type'] = 'application/json';
      if (options.method && options.method !== 'GET') headers['X-CSRF-Token'] = csrfOverride ?? state.csrf;
      let response;
      try {
        response = await fetcher(path, { ...options, headers, credentials: 'same-origin' });
      } catch (error) {
        throw makeError(error.message || 'Falha de rede.', 'NETWORK', 0);
      }
      return readResponse(response);
    }

    function inconclusive(error) { return error.code === 'NETWORK' || error.code === 'INVALID_JSON' || !error.status; }

    async function loadSession() {
      renderTransient('LOADING', 'Nenhum dado administrativo será mostrado até confirmar a sessão.');
      const probeGeneration = state.generation;
      try {
        const session = await request('/api/admin/v1/session');
        if (probeGeneration !== state.generation) return;
        renderSession(session);
        if (session?.state === 'AUTHENTICATED') await loadRequests();
      } catch (error) {
        if (probeGeneration !== state.generation) return;
        const session = error.body?.session;
        if (error.status === 401 && session) {
          renderSession(session);
          showError(errorMessage(error));
        } else {
          renderTransient(inconclusive(error) ? 'UNKNOWN_RESULT' : 'UNAVAILABLE', errorMessage(error));
          showError(errorMessage(error));
        }
      }
    }

    function renderRequestRow(item) {
      const row = documentRef.createElement('li');
      row.className = 'request-item';
      row.dataset.requestId = item.id;
      row.dataset.selected = item.id === state.selected ? 'true' : 'false';
      const select = documentRef.createElement('button');
      select.type = 'button'; select.className = 'request-select';
      const title = documentRef.createElement('strong');
      title.textContent = item.client?.display_name || 'Cliente não atestado';
      const meta = documentRef.createElement('span');
      meta.textContent = `${item.status || 'DESCONHECIDO'} · ${item.scope || 'escopo não informado'} · versão ${item.version}`;
      select.append(title, meta);
      select.addEventListener('click', () => void loadRequestDetail(item.id));
      row.append(select);
      if (item.status === 'PENDING') {
        const actions = documentRef.createElement('div');
        actions.className = 'request-actions';
        for (const decision of ['deny', 'approve']) {
          const button = documentRef.createElement('button');
          button.type = 'button'; button.className = decision === 'approve' ? 'button primary' : 'button';
          button.textContent = decision === 'approve' ? 'Aprovar' : 'Recusar';
          button.dataset.decision = decision;
          button.disabled = state.decisionInFlight.has(item.id) || state.blockedDecisions.has(item.id);
          button.addEventListener('click', () => void decide(item, decision));
          actions.append(button);
        }
        row.append(actions);
      }
      return row;
    }

    async function loadRequests() {
      if (state.session?.state !== 'AUTHENTICATED') return;
      const generation = state.generation;
      setMessage(elements.requestsStatus, 'Consultando solicitações…');
      const controller = typeof AbortController === 'function' ? new AbortController() : null;
      state.requestController = controller;
      try {
        const body = await request('/api/admin/v1/requests', controller ? { signal: controller.signal } : {});
        if (generation !== state.generation || state.session?.state !== 'AUTHENTICATED') return;
        const items = Array.isArray(body.requests) ? body.requests : [];
        elements.requests.replaceChildren(...items.map(renderRequestRow));
        elements.requestCount.textContent = String(items.length);
        setMessage(elements.requestsStatus, items.length ? '' : 'Nenhuma solicitação encontrada no servidor.');
      } catch (error) {
        if (generation !== state.generation) return;
        elements.requests.replaceChildren();
        elements.requestCount.textContent = 'n/d';
        setMessage(elements.requestsStatus, errorMessage(error), 'error');
        if (error.status === 401) await reconcileSessionAfterAuthError(error);
        else showError(errorMessage(error));
      }
    }

    function renderDetail(item, message = '') {
      state.detail = item; state.selected = item.id;
      elements.detail.hidden = false;
      elements.detailTitle.textContent = item.client?.display_name || 'Cliente não atestado';
      elements.detailStatus.textContent = item.status || 'DESCONHECIDO';
      elements.detailStatus.dataset.tone = item.status === 'APPROVED' ? 'success' : item.status === 'DENIED' || item.status === 'EXPIRED' ? 'danger' : 'neutral';
      elements.detailClientName.textContent = item.client?.display_name || 'Cliente não atestado';
      elements.detailClientNote.textContent = 'Nome fornecido pelo cliente; não atestado pelo nome.';
      elements.detailClientId.textContent = item.client?.id || 'n/d';
      elements.detailRedirect.textContent = item.redirect_uri || 'n/d';
      elements.detailScope.textContent = item.scope || 'n/d';
      elements.detailGrant.textContent = item.workspace_read?.required ? (item.workspace_read.grant_status || 'estado não informado') : 'não aplicável';
      elements.detailVersion.textContent = String(item.version ?? 'n/d');
      elements.detailExpires.textContent = formatServerTime(item.expires_at);
      elements.detailId.textContent = item.id || 'n/d';
      elements.decisionActions.hidden = item.status !== 'PENDING';
      const blocked = state.blockedDecisions.has(item.id) || state.decisionInFlight.has(item.id);
      elements.deny.disabled = blocked; elements.approve.disabled = blocked;
      setMessage(elements.detailMessage, message);
      elements.requests.querySelectorAll('[data-request-id]').forEach((row) => { row.dataset.selected = row.dataset.requestId === item.id ? 'true' : 'false'; });
    }

    async function loadRequestDetail(id) {
      if (state.session?.state !== 'AUTHENTICATED') return;
      const generation = state.generation;
      state.selected = id;
      elements.detail.hidden = false;
      elements.detailTitle.textContent = 'Carregando detalhes…';
      elements.decisionActions.hidden = true;
      setMessage(elements.detailMessage, 'Consultando o pedido no servidor…');
      try {
        const item = await request(`/api/admin/v1/requests/${encodeURIComponent(id)}`);
        if (generation !== state.generation || state.session?.state !== 'AUTHENTICATED') return;
        renderDetail(item);
        elements.detailTitle.focus();
      } catch (error) {
        if (generation !== state.generation) return;
        state.blockedDecisions.add(id);
        elements.decisionActions.hidden = true;
        setMessage(elements.detailMessage, errorMessage(error), 'error');
        if (error.status === 401) await reconcileSessionAfterAuthError(error);
        else showError(errorMessage(error));
      }
    }

    async function reconcileSessionAfterAuthError(error) {
      clearPrivilegedData('Sessão inválida; pedidos e ações foram removidos.');
      const session = error.body?.session;
      if (session) renderSession(session);
      else await loadSession();
    }

    async function reconcile(id, cause) {
      const generation = state.generation;
      state.blockedDecisions.add(id);
      if (state.detail?.id === id) {
        elements.decisionActions.hidden = false;
        elements.deny.disabled = true; elements.approve.disabled = true;
        setMessage(elements.detailMessage, 'Resultado não confirmado; consultando o estado atual…', 'warning');
      }
      try {
        const item = await request(`/api/admin/v1/requests/${encodeURIComponent(id)}`);
        if (generation !== state.generation) return;
        renderDetail(item, `Resultado não confirmado (${errorMessage(cause)}). Estado atual consultado pelo servidor: ${item.status}.`);
        if (item.status === 'PENDING') state.blockedDecisions.add(id);
        await loadRequests();
      } catch (error) {
        if (generation !== state.generation) return;
        elements.decisionActions.hidden = true;
        setMessage(elements.detailMessage, `Resultado desconhecido: não foi possível reconciliar (${errorMessage(error)}).`, 'error');
        setMessage(elements.requestsStatus, 'O estado desta solicitação não foi confirmado; novas decisões permanecem bloqueadas.', 'error');
        if (error.status === 401) await reconcileSessionAfterAuthError(error);
      }
    }

    async function decide(item, decision) {
      if (state.session?.state !== 'AUTHENTICATED' || state.decisionInFlight.has(item.id) || state.blockedDecisions.has(item.id)) return;
      state.decisionInFlight.add(item.id);
      renderDetail(item, 'Enviando decisão ao servidor…');
      elements.decisionActions.hidden = false; elements.deny.disabled = true; elements.approve.disabled = true;
      const generation = state.generation;
      try {
        const result = await request(`/api/admin/v1/requests/${encodeURIComponent(item.id)}/decision`, {
          method: 'POST', body: JSON.stringify({ decision, expected_version: item.version })
        });
        if (generation !== state.generation) return;
        renderDetail(result, 'Decisão confirmada pelo servidor. Isso não afirma emissão de token, conexão MCP ou concessão de workspace.');
        await loadRequests();
      } catch (error) {
        if (generation !== state.generation) return;
        if (error.status === 401) await reconcileSessionAfterAuthError(error);
        else await reconcile(item.id, error);
      } finally {
        state.decisionInFlight.delete(item.id);
        if (generation === state.generation && state.detail?.id === item.id) {
          elements.deny.disabled = state.blockedDecisions.has(item.id);
          elements.approve.disabled = state.blockedDecisions.has(item.id);
        }
      }
    }

    async function submitCredentials(path, form) {
      if (form.dataset.busy === 'true') return;
      const data = Object.fromEntries(new FormDataCtor(form).entries());
      const button = form.querySelector('button[type="submit"]');
      const csrf = state.csrf;
      form.dataset.busy = 'true'; button.disabled = true; form.reset();
      const generation = nextGeneration();
      state.session = null; state.csrf = '';
      setShell('UNKNOWN_RESULT');
      clearPrivilegedData('Aguardando confirmação do servidor; nenhum pedido será exibido ainda.');
      try {
        const session = await request(path, { method: 'POST', body: JSON.stringify(data) }, csrf);
        if (generation !== state.generation) return;
        renderSession(session);
        if (session?.state === 'AUTHENTICATED') await loadRequests();
      } catch (error) {
        if (generation !== state.generation) return;
        if (inconclusive(error)) {
          renderTransient('UNKNOWN_RESULT', 'A resposta de pareamento/desbloqueio foi perdida; verificando a sessão sem repetir o POST.');
          await loadSession();
        } else {
          renderTransient('UNAVAILABLE', errorMessage(error));
          showError(errorMessage(error));
        }
      } finally {
        form.dataset.busy = 'false'; button.disabled = false;
      }
    }

    async function refreshSession() {
      if (state.session?.state !== 'AUTHENTICATED') return;
      const generation = state.generation; const csrf = state.csrf;
      elements.refresh.disabled = true;
      try {
        const session = await request('/api/admin/v1/session/refresh', { method: 'POST', body: '{}' }, csrf);
        if (generation !== state.generation) return;
        renderSession(session); await loadRequests();
      } catch (error) {
        if (generation !== state.generation) return;
        if (error.status === 401) await reconcileSessionAfterAuthError(error); else showError(errorMessage(error));
      } finally { elements.refresh.disabled = false; }
    }

    async function lockSession() {
      if (state.session?.state !== 'AUTHENTICATED') return;
      const csrf = state.csrf; const generation = nextGeneration();
      state.session = null; state.csrf = '';
      setShell('LOADING'); clearPrivilegedData('Bloqueando painel; dados e ações privilegiados foram removidos.');
      try {
        const session = await request('/api/admin/v1/lock', { method: 'POST', body: '{}' }, csrf);
        if (generation !== state.generation) return;
        renderSession(session);
      } catch (_) {
        if (generation !== state.generation) return;
        renderTransient('UNKNOWN_RESULT', 'O bloqueio não foi confirmado; verificando a sessão sem repetir o POST.');
        await loadSession();
      }
    }

    function bind() {
      elements.pairForm.addEventListener('submit', (event) => { event.preventDefault(); void submitCredentials('/api/admin/v1/pair', event.currentTarget); });
      elements.unlockForm.addEventListener('submit', (event) => { event.preventDefault(); void submitCredentials('/api/admin/v1/unlock', event.currentTarget); });
      elements.refresh.addEventListener('click', () => void refreshSession());
      elements.lock.addEventListener('click', () => void lockSession());
      elements.retry.addEventListener('click', () => void loadSession());
      elements.recheck.addEventListener('click', () => void loadSession());
      elements.deny.addEventListener('click', () => { if (state.detail) void decide(state.detail, 'deny'); });
      elements.approve.addEventListener('click', () => { if (state.detail) void decide(state.detail, 'approve'); });
    }

    function start() {
      bind();
      setShell('LOADING');
      clearRequestDom('Nenhum pedido será mostrado antes da autenticação.');
      void loadSession();
    }

    return { start, loadSession, loadRequests, loadRequestDetail, decide, renderSession, state };
  };

  if (typeof module === 'object' && module.exports) {
    module.exports = { createApp };
  } else {
    const app = createApp(document, window.fetch.bind(window), window.FormData);
    app.start();
  }
})();

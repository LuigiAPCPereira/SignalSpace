(() => {
  "use strict";

  const root = document.getElementById("consent");
  const statusNode = document.getElementById("authorization-status");
  const form = document.getElementById("authorization-complete-form");
  const button = document.getElementById("continue-button");
  const validStates = new Set(["PENDING", "APPROVED", "DENIED", "EXPIRED"]);

  if (!root || !statusNode || !form || !button) {
    return;
  }

  const requestID = root.dataset.requestId;
  if (typeof requestID !== "string" || !/^[A-Za-z0-9_-]{22}$/.test(requestID)) {
    statusNode.textContent = "Não foi possível identificar esta solicitação.";
    statusNode.dataset.state = "error";
    button.disabled = true;
    return;
  }

  let stopped = false;
  let timer = null;

  const setStatus = (message, state) => {
    statusNode.textContent = message;
    statusNode.dataset.state = state;
  };

  const stop = () => {
    stopped = true;
    if (timer !== null) {
      window.clearTimeout(timer);
      timer = null;
    }
  };

  const schedule = (delay) => {
    if (!stopped) {
      timer = window.setTimeout(poll, delay);
    }
  };

  const retryAfterMs = (response) => {
    const seconds = Number.parseInt(response.headers.get("Retry-After") || "", 10);
    if (Number.isFinite(seconds) && seconds > 0) {
      return Math.min(seconds * 1000, 60000);
    }
    return 5000;
  };

  const poll = async () => {
    if (stopped) {
      return;
    }
    try {
      const response = await fetch("/authorize/status?request_id=" + encodeURIComponent(requestID), {
        method: "GET",
        credentials: "same-origin",
        cache: "no-store",
        headers: { Accept: "application/json" },
      });

      if (response.status === 429) {
        setStatus("A consulta foi limitada temporariamente; tentando novamente.", "retry");
        schedule(retryAfterMs(response));
        return;
      }
      if (response.status === 403) {
        setStatus("A sessão desta solicitação não está disponível. Nenhum acesso foi concedido.", "error");
        button.disabled = true;
        stop();
        return;
      }
      if (response.status === 404) {
        setStatus("Esta solicitação não está mais disponível. Nenhum acesso foi concedido.", "error");
        button.disabled = true;
        stop();
        return;
      }
      if (!response.ok) {
        setStatus("Não foi possível consultar o estado agora; tentando novamente.", "retry");
        schedule(5000);
        return;
      }

      const data = await response.json();
      if (!data || typeof data.status !== "string" || !validStates.has(data.status)) {
        throw new Error("invalid authorization status");
      }

      if (data.status === "PENDING") {
        button.disabled = true;
        setStatus("Aguardando aprovação local…", "pending");
        schedule(2000);
        return;
      }
      if (data.status === "APPROVED") {
        button.disabled = false;
        setStatus("Aprovação local confirmada. Você pode continuar.", "approved");
        stop();
        return;
      }
      if (data.status === "DENIED") {
        button.disabled = true;
        setStatus("A solicitação foi recusada localmente. Nenhum acesso foi concedido.", "denied");
        stop();
        return;
      }
      button.disabled = true;
      setStatus("A solicitação expirou. Nenhum acesso foi concedido.", "expired");
      stop();
    } catch (_error) {
      button.disabled = true;
      setStatus("Não foi possível consultar o estado; tentando novamente.", "retry");
      schedule(5000);
    }
  };

  button.disabled = true;
  form.addEventListener("submit", () => {
    button.disabled = true;
    setStatus("Concluindo autorização…", "completing");
  });
  setStatus("Aguardando aprovação local…", "pending");
  poll();
})();

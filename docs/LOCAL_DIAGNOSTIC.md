# Diagnóstico MCP local — primeira fatia em Go

**Status:** diagnóstico experimental em Go. Não houve teste com o ChatGPT Web e ainda não existe conector remoto validado.

## Executar

Requisito mínimo: Go 1.23. Usa somente biblioteca padrão, sem dependências externas.

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(openssl rand -hex 32)"
go test ./...
go vet ./...
go run ./cmd/signalspace
```

O serviço escuta somente em `127.0.0.1:7676`. O segredo vem de `SIGNALSPACE_LOCAL_TOKEN`, precisa ter pelo menos 32 caracteres sem espaços e não deve ser versionado ou registrado em logs. Sem token válido, o processo não inicia no modo local.

## Contrato local

- `POST /mcp` com subconjunto JSON-RPC 2.0 / MCP `2025-06-18`.
- `initialize`, `ping`, `tools/list` e `tools/call` para `connection_diagnostic`.
- Bearer local obrigatório antes de processar ferramentas; comparação em tempo constante dos hashes do token.
- Host e Origin limitados ao serviço local, entrada de no máximo 64 KiB e timeouts HTTP.
- Nenhuma operação de arquivo, edição, Git, shell ou processo persistente.

`connected: true` comprova somente que uma chamada autenticada ocorreu; `chatgptVerified: false` é intencional.

## Limite de segurança

**Não exponha o diagnóstico local por Cloudflare Tunnel, ngrok, Tailscale Funnel ou proxy público.** O bearer local não substitui OAuth nem autorização do proprietário.

Existe um modo de **servidor de recursos OAuth** separado e experimental que valida tokens JWT e publica metadados de descoberta. Ele depende de um emissor externo confiável ainda não configurado, não realiza login/consentimento e não foi testado com o ChatGPT Web. Consulte [OAuth no SignalSpace](OAUTH_RESOURCE_SERVER.md). Não publicar essa modalidade antes de concluir os gates do documento.

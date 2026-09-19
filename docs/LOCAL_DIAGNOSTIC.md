# Diagnóstico MCP local — primeira fatia em Go

**Status:** diagnóstico experimental local em Go. Não houve teste do ChatGPT Web e não existe conector remoto utilizável nesta etapa.

## Executar

Requisito mínimo: Go 1.23. A implementação usa somente a biblioteca padrão, sem dependências externas.

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(openssl rand -hex 32)"
go test ./...
go vet ./...
go run ./cmd/signalspace
```

O serviço escuta **somente `127.0.0.1:7676`**. O segredo vem da variável `SIGNALSPACE_LOCAL_TOKEN`, deve ter pelo menos 32 caracteres sem espaços e não deve ser colocado em arquivos versionados nem em logs. O processo não inicia com token ausente ou fraco.

## Contrato atual

- `POST /mcp` com subconjunto de JSON-RPC 2.0 / MCP `2025-06-18`.
- `initialize`, `ping`, `tools/list` e `tools/call` para `connection_diagnostic`.
- Bearer local obrigatório antes de processar ferramentas; comparação em tempo constante dos hashes do token.
- Host e Origin limitados ao endpoint local, corpo de requisição limitado a 64 KiB, timeouts HTTP.
- Sem operações de arquivo, edição, Git, shell, processos persistentes ou frontend.

`connected: true` comprova apenas a chamada local autenticada; `chatgptVerified: false` é intencional.

## Limite de segurança

**Não exponha esta versão via Cloudflare Tunnel, ngrok, Tailscale Funnel ou proxy público.** O bearer local não substitui OAuth, descoberta de recurso protegido, autorização do proprietário e validação do cliente ChatGPT. Estas capacidades ainda não estão implementadas.

## Próxima fatia

Implementar autenticação/autorização remota compatível com MCP e ChatGPT, descoberta de metadados, consentimento e validação de tokens. Só então permitir um endpoint HTTPS público e testar uma chamada real no ChatGPT Web. Não afirmar suporte a plano/modelo apenas pela exibição da integração na interface.

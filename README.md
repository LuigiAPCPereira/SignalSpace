# SignalSpace

**Give ChatGPT a secure connection to your own machine and turn ChatGPT into Codex.**

SignalSpace é um projeto independente em **Go** para oferecer ao **ChatGPT Web** ferramentas de desenvolvimento executadas na máquina do usuário. A conversa permanecerá no ChatGPT; um serviço local disponibilizará MCP por uma conexão HTTPS autenticada.

> **Estado:** diagnóstico MCP local e servidor de recursos OAuth com validação de JWT/JWKS implementados. Há também um **servidor de autorização OAuth integrado experimental**, com registro de cliente, PKCE e aprovação pelo terminal, testado apenas com um cliente simulado. **Nenhum túnel HTTPS ou chamada real no ChatGPT Web foi validado. Não exponha esta versão publicamente.** Arquivos, Git e terminal ainda não foram implementados.

## Diagnóstico local

Requer Go 1.23 ou superior; somente biblioteca padrão:

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(openssl rand -hex 32)"
go test ./...
go vet ./...
go run ./cmd/signalspace
```

O endpoint `http://127.0.0.1:7676/mcp` oferece apenas `connection_diagnostic` com bearer local. Não equivale a OAuth nem a uma integração funcional com ChatGPT. Consulte [diagnóstico local](docs/LOCAL_DIAGNOSTIC.md).

## OAuth integrado (experimental)

O SignalSpace pode emitir os próprios tokens, sem conta obrigatória no Auth0. Para desenvolvimento, ative explicitamente `SIGNALSPACE_AUTH_MODE=embedded` e configure `SIGNALSPACE_RESOURCE_URL` com a URL HTTPS canônica do recurso; a autorização só é aprovada por comando digitado no terminal do proprietário. **Sem persistência:** o registro de clientes e a chave são perdidos quando o processo reinicia. Não publique o endpoint antes de fechar o transporte, a proteção contra abuso e o teste no ChatGPT Web. Veja [autorização integrada e limites](docs/EMBEDDED_OAUTH.md).

O modo alternativo com emissor externo e JWKS HTTPS continua opcional e separado, sem fallback para bearer local. Veja [servidor de recursos OAuth](docs/OAUTH_RESOURCE_SERVER.md), [diagnóstico do provedor](docs/OAUTH_PREFLIGHT.md) e [roteiro Auth0 opcional](docs/AUTH0_INTEGRATION.md).

## Produto

Fluxo desejado: iniciar serviço → configurar túnel HTTPS → conectar pelo ChatGPT Web e autorizar cliente → abrir workspace aprovado → inspecionar código → editar → executar testes/comandos → revisar o resultado.

**Capacidades planejadas:** workspaces autorizados, leitura e edição, terminal/processos, Git e diffs, erros explícitos e continuidade de operações longas. A continuidade entre conversas virá depois de validar a conexão principal.

**Fora do escopo inicial:** frontend substituto do ChatGPT, IA própria, subagentes, integração com `agent-runtime` ou `agent-orchestrator`, fork/dependência do DevSpace e implementação própria de grafos. Graphify pode ser uma integração opcional futura.

## Segurança assumida

Conexões remotas exigem autenticação e autorização efetivas antes de ferramentas de desenvolvimento. O proprietário selecionará as raízes permitidas; ferramentas de arquivos precisarão impedir escapes. **O shell poderá executar com os privilégios normais do usuário: não é sandbox.** A allowlist de arquivos não confina o terminal.

O processo escuta apenas em loopback. Compatibilidade com plano/modelo do ChatGPT requer chamada de ferramenta realmente observada, não apenas uma integração exibida na interface.

## Documentos

- [Produto](docs/PRODUCT.md)
- [MVP](docs/MVP.md)
- [Diagnóstico local](docs/LOCAL_DIAGNOSTIC.md)
- [Autorização OAuth integrada](docs/EMBEDDED_OAUTH.md)
- [Servidor de recursos OAuth externo](docs/OAUTH_RESOURCE_SERVER.md)
- [Instruções para agentes](AGENTS.md)

Projeto original; apenas aprendizados conceituais de [DevSpace](https://github.com/Waishnav/devspace) e [Graphify](https://github.com/Graphify-Labs/graphify), sem incorporar código desses projetos.

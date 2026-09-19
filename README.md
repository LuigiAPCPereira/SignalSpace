# SignalSpace

**Give ChatGPT a secure connection to your own machine and turn ChatGPT into Codex.**

SignalSpace é um projeto independente em **Go** para oferecer ao **ChatGPT Web** ferramentas de desenvolvimento executadas na máquina do usuário. A conversa permanece no ChatGPT; um serviço local disponibilizará MCP por uma conexão HTTPS autenticada.

> **Estado:** há um diagnóstico MCP local e uma fronteira de **servidor de recursos OAuth** com validação JWT/JWKS, ambos em Go. OAuth com provedor real, autorização exclusiva do proprietário, túnel HTTPS, ferramentas de arquivos/terminal/Git e integração observada no ChatGPT Web **ainda não foram concluídos**. Não exponha esta versão publicamente.

## Diagnóstico local

Requer Go 1.23 ou superior; somente biblioteca padrão:

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(openssl rand -hex 32)"
go test ./...
go vet ./...
go run ./cmd/signalspace
```

O endpoint `http://127.0.0.1:7676/mcp` oferece apenas `connection_diagnostic` com bearer local. Não equivale a OAuth nem a uma integração funcional com ChatGPT. Consulte [diagnóstico local](docs/LOCAL_DIAGNOSTIC.md).

A alternativa OAuth é selecionada por configuração explícita do recurso, emissor e JWKS HTTPS, sem fallback para o bearer local. Ela oferece descoberta pública de recurso e validação de tokens antes de qualquer chamada MCP. **Ainda não é um conector instalável:** depende de um provedor externo compatível e de validação do proprietário, transporte e ChatGPT Web. Veja [servidor de recursos OAuth](docs/OAUTH_RESOURCE_SERVER.md).

## Produto

Fluxo desejado: iniciar serviço → configurar túnel HTTPS → conectar pelo ChatGPT Web e autorizar cliente → abrir workspace aprovado → inspecionar código → editar → executar testes/comandos → revisar o resultado.

**Capacidades planejadas:** workspaces autorizados, leitura e edição, terminal/processos, Git e diffs, erros explícitos e continuidade de operações longas. A continuidade entre conversas virá depois de validar a conexão principal.

**Fora do escopo inicial:** frontend substituto do ChatGPT, IA própria, subagentes, integração com `agent-runtime` ou `agent-orchestrator`, fork/dependência do DevSpace e implementação própria de grafos. Graphify pode ser uma integração opcional futura.

## Segurança assumida

Conexões remotas exigem autenticação e autorização efetivas antes de ferramentas de desenvolvimento. O proprietário seleciona as raízes permitidas; ferramentas de arquivos precisarão impedir escapes. **O shell poderá executar com os privilégios normais do usuário: não é sandbox.** A allowlist de arquivos não confina o terminal.

O processo escuta apenas em loopback. Compatibilidade com plano/modelo do ChatGPT requer chamada de ferramenta realmente observada, não apenas uma integração exibida na interface.

## Documentos

- [Produto](docs/PRODUCT.md)
- [MVP](docs/MVP.md)
- [Diagnóstico local](docs/LOCAL_DIAGNOSTIC.md)
- [Servidor de recursos OAuth](docs/OAUTH_RESOURCE_SERVER.md)
- [Instruções para agentes](AGENTS.md)

Projeto original; apenas aprendizados conceituais de [DevSpace](https://github.com/Waishnav/devspace) e [Graphify](https://github.com/Graphify-Labs/graphify), sem incorporar código desses projetos.

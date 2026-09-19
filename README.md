# SignalSpace

**Give ChatGPT a secure connection to your own machine and turn ChatGPT into Codex.**

SignalSpace é um projeto independente em **Go** para dar ao **ChatGPT Web** acesso operacional a projetos autorizados na máquina do usuário. A conversa permanece no ChatGPT; um serviço local oferecerá ferramentas de desenvolvimento por MCP através de uma conexão HTTPS autenticada.

> **Estado:** a branch de implementação possui somente um **diagnóstico MCP local em Go**. OAuth, túnel configurado, aprovação de cliente, ferramentas de arquivos/terminal/Git e integração real com ChatGPT Web ainda não estão implementados. **Não publique o diagnóstico na internet.**

## Diagnóstico local

Requer Go 1.23 ou superior; somente biblioteca padrão:

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(openssl rand -hex 32)"
go test ./...
go vet ./...
go run ./cmd/signalspace
```

O endpoint `http://127.0.0.1:7676/mcp` oferece somente a ferramenta `connection_diagnostic` com token bearer local. Não equivale a OAuth nem a uma integração funcional com o ChatGPT. Mais detalhes em [`docs/LOCAL_DIAGNOSTIC.md`](docs/LOCAL_DIAGNOSTIC.md).

## Produto

Fluxo desejado: iniciar o serviço → configurar túnel HTTPS → conectar pelo ChatGPT Web e autorizar o cliente → abrir workspace aprovado → inspecionar código → editar → executar testes/comandos → revisar o resultado.

**Capacidades planejadas:** workspaces com raízes permitidas, leitura e edição, terminal/processos, Git e diffs, resultados explícitos e continuidade de operações longas. Continuidade entre conversas virá depois da conexão principal funcionar.

**Fora do escopo inicial:** frontend que substitua o ChatGPT, IA própria, subagentes, integração com `agent-runtime` ou `agent-orchestrator`, fork/dependência do DevSpace e implementação própria de grafos. Graphify pode ser uma integração opcional futura.

## Segurança assumida

Conexões remotas exigirão autenticação e autorização efetivas antes de ferramentas de desenvolvimento. O usuário escolhe quais raízes podem ser abertas; ferramentas de arquivos devem bloquear caminhos que escapam dessas raízes. **O shell poderá usar os privilégios normais do usuário: não haverá sandbox nesta fase.** A proteção por raiz de arquivos não confina comandos de shell.

O serviço atual não aceita acesso público: não configure túnel para o diagnóstico local. Compatibilidade com plano/modelo do ChatGPT requer chamada de ferramenta realmente observada.

## Documentos

- [Produto](docs/PRODUCT.md)
- [MVP](docs/MVP.md)
- [Diagnóstico local em Go](docs/LOCAL_DIAGNOSTIC.md)
- [Instruções para agentes](AGENTS.md)

Projeto original; apenas aprendizados conceituais de [DevSpace](https://github.com/Waishnav/devspace) e [Graphify](https://github.com/Graphify-Labs/graphify), sem incorporar código desses projetos.

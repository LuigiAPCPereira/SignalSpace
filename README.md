# SignalSpace

**Give ChatGPT a secure connection to your own machine and turn ChatGPT into Codex.**

SignalSpace é um projeto independente para dar ao **ChatGPT Web** acesso operacional a projetos autorizados na máquina do usuário. A conversa permanece no ChatGPT; um serviço local expõe ferramentas de desenvolvimento por MCP, através de uma conexão HTTPS autenticada.

> **Estado:** há um servidor MCP de **diagnóstico somente local** na branch de implementação; ainda não há OAuth, túnel configurado, aprovação do proprietário, ferramentas de arquivos/terminal/Git ou integração comprovada com ChatGPT Web. **Não publique esta versão na internet.** Veja [diagnóstico local](docs/LOCAL_DIAGNOSTIC.md).

## Executar o diagnóstico local

Requer Node.js 22 ou superior, sem dependências de terceiros nesta fatia:

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(node -e "console.log(require('node:crypto').randomBytes(32).toString('hex'))")"
npm run check
npm test
npm start
```

O endpoint `http://127.0.0.1:7676/mcp` aceita MCP `2025-06-18` com bearer **local** e oferece somente `connection_diagnostic`. Isso não equivale à autenticação OAuth exigida para conectar o ChatGPT Web. Configuração e limitações em [`docs/LOCAL_DIAGNOSTIC.md`](docs/LOCAL_DIAGNOSTIC.md).

## Produto

O fluxo desejado é: iniciar o serviço na máquina → disponibilizar seu endpoint MCP por um túnel HTTPS controlado pelo usuário → conectar no ChatGPT Web e autorizar o cliente → abrir um workspace permitido → inspecionar código → editar arquivos → executar testes/comandos → revisar o resultado.

**Capacidades centrais planejadas:** workspaces com raízes permitidas; leitura e edição; terminal/processos; Git e diffs; tratamento explícito de erros, desconexões e operações de longa duração. Continuidade entre conversas será uma evolução do produto, não uma promessa da primeira entrega.

**Fora do escopo inicial:** interface que substitua o ChatGPT, modelo de IA próprio, subagentes, integrações com `agent-runtime` ou `agent-orchestrator`, fork ou reutilização de código do DevSpace, implementação própria de grafos de código. Graphify pode ser uma integração opcional posterior.

## Arquitetura-alvo

```text
ChatGPT Web
    |
    | MCP
    v
HTTPS endpoint + authenticated client approval
    |
    v
SignalSpace local service
    +-- workspace authorization
    +-- files and patches
    +-- commands and process sessions
    +-- Git and review
    +-- operation records (later)
```

O transporte e a autenticação do ChatGPT ainda precisam ser validados na prática. Uma URL local `127.0.0.1` não é, por si só, acessível ao ChatGPT Web; o túnel é um componente explícito da instalação. O projeto não afirmará compatibilidade com um plano ou modelo sem uma chamada real de ferramenta.

## Segurança e limite deliberado do MVP

O usuário escolhe quais raízes podem ser abertas; ferramentas de arquivos devem validar caminhos e bloquear escapes. Conexões remotas exigirão autenticação OAuth. **O shell inicial poderá executar com os privilégios normais da conta local**, conforme decisão consciente de escopo: raízes de arquivos e worktrees **não** são um sandbox do terminal. Nenhum endpoint de execução será publicado antes de implementar e testar autenticação e autorização.

## Primeira entrega verificável

Consultar [`docs/MVP.md`](docs/MVP.md) para critérios de aceitação e ordem de implementação. O principal gate é uma sessão real no ChatGPT Web que abra um workspace autorizado, leia e modifique um arquivo, execute um comando e devolva uma revisão verificável.

Instruções para agentes: [`AGENTS.md`](AGENTS.md). Contrato de produto: [`docs/PRODUCT.md`](docs/PRODUCT.md).

## Proveniência

Projeto novo, inspirado apenas em aprendizados de engenharia e em padrões de ferramentas como [DevSpace](https://github.com/Waishnav/devspace) e [Graphify](https://github.com/Graphify-Labs/graphify). Nenhum código desses repositórios foi incorporado.

Licença e distribuição permanecem decisões em aberto; Node.js sem dependências externas é uma escolha provisória para o diagnóstico local, não decisão irrevogável da stack final.

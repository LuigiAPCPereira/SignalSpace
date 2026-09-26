# MVP — primeira conexão operacional ao ChatGPT Web

**Status:** especificação inicial. Nenhum dos requisitos abaixo foi implementado ou testado no SignalSpace até a criação deste documento.

> **Reabertura 20/09/2026:** a frase acima é o status histórico da especificação. A fatia local atual está registrada em `TASKLIST.md` como SS-MVP-001…006 e em [`docs/PROGRAMMING_TOOLS.md`](PROGRAMMING_TOOLS.md); ela ainda não constitui integração ChatGPT Web, MCP remoto ou aceite de navegador.

## Objetivo

Em uma conversa no ChatGPT Web, conectar ao SignalSpace em uma máquina do usuário, abrir um projeto autorizado, ler um arquivo, realizar uma alteração delimitada, executar um comando de teste e obter o diff/resultado verdadeiro.

## Entrega em fatias

### 1. Transporte e autorização

- Executar serviço local somente em loopback por padrão.
- Oferecer endpoint MCP compatível com a versão de protocolo escolhida e validada pelo cliente real.
- Usar túnel HTTPS configurado pelo usuário; a URL pública não é considerada segredo.
- Exigir identidade/autorização do cliente antes de qualquer ferramenta de arquivos ou comandos.
- Rejeitar cliente não autorizado; confirmar que o ChatGPT recebe ferramentas de fato, não apenas exibe uma integração conectada.

**Aceitação:** uma sessão real do ChatGPT Web executa ferramenta inofensiva de diagnóstico no serviço; o mesmo pedido sem autorização falha. Registrar variante do modelo, forma de conexão e observação, sem publicar tokens.

### 2. Workspace e leitura

- Configurar raízes autorizadas explicitamente, sem permitir raiz ampla por acidente.
- Abrir projeto sob raiz permitida e retornar `workspace_id` estável durante a sessão.
- Resolver e limitar leituras: recusar caminhos fora da raiz, traversal e escape via symlink.
- Permitir ler arquivos e inspecionar Git HEAD/branch/dirty state, com limites de saída.

**Aceitação:** projeto permitido funciona; projeto não autorizado, `../` e symlink de fuga são negados por testes negativos; arquivo lido pelo ChatGPT corresponde ao conteúdo real.

### 3. Edição, comando e revisão

- Expor edição com resultado verificável e tratamento de conflito; não sobrescrever alterações não previstas.
- Executar comando explicitamente no diretório de trabalho escolhido, com tempo e saída limitados.
- Expor resultados de processo distinguindo exit code, timeout, cancelamento solicitado e resultado desconhecido.
- Expor diff/revisão verificável de Git e informar mudanças preexistentes separadamente.

**Aceitação:** ChatGPT altera arquivo em projeto de teste, executa um teste real e devolve diff e resultado corretos. Falha de teste é reportada como falha. Shell roda como usuário local e **não é sandbox**; a conexão e a autorização do cliente são obrigatórias.

## O que fica para depois

Processos de longa duração com reconexão e recuperação, operações duráveis/idempotência, worktrees, context packs, Graphify, UI local sofisticada e subagentes. Não adicionar integrações com agent-runtime ou agent-orchestrator.

## Gates de implementação

Antes de declarar um marco concluído: inspecionar branch/HEAD e diff; rodar testes unitários e negativos pertinentes; compilar/validar ferramentas; reproduzir o fluxo com ChatGPT Web e capturar evidência sem credenciais. Se a integração do cliente impedir uma ferramenta, registrar incompatibilidade observada, não inventar bypass nem afirmar suporte universal.

## Primeiro trabalho de código

Selecionar stack com base no requisito de MCP HTTP autenticado e no ambiente-alvo, implementar a fatia **1** com teste local de não autorização e um smoke real no ChatGPT Web. Não construir ferramentas de terminal antes de autenticação funcionar.

## Reconciliação arquitetural v2 — 25/09/2026

O MVP inicial acima permanece como especificação histórica de aceitação ponta a ponta. Para a evolução da composição Programming, a tarefa `SS-MVP-002-LOCAL-AUTHORIZATION-V2-DESIGN-001` aceita a separação entre autenticação OAuth da composição e autorização local por capability. O alvo não transforma OAuth em acesso ao computador: workspace, sessão, política, aprovação e precondições continuam obrigatórios, e shell/teste/Git remoto exigem gates próprios.

O próximo trabalho de implementação, se autorizado por tarefa própria, deve seguir [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md). A ADR, esta reconciliação e o checkpoint são documentação; não constituem implementação, aceite de navegador, CI ou permissão nova.

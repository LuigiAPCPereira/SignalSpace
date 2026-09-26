# ADR — autorização local v2 para a composição Programming

**ID:** `SS-MVP-002-LOCAL-AUTHORIZATION-V2-DESIGN-001`
**Estado:** **ACEITA / CICLO OAUTH V2 E NÚCLEO INTERNO DE CAPABILITIES/POLICY IMPLEMENTADOS NO AUTH HARNESS / MIGRAÇÃO PÚBLICA NÃO IMPLEMENTADA**
**Data da decisão:** 25/09/2026
**Escopo:** arquitetura, contratos, modelo de domínio, migração e critérios de aceite. O slice `SS-MVP-002-LOCAL-CAPABILITIES-POLICY-V2-001` implementa somente o núcleo interno descrito nesta ADR; não autoriza migração pública, push, merge ou deploy por consequência.

## Implementação do gate de approvals de uso único — 26/09/2026

O gate `SS-MVP-002-LOCAL-APPROVAL-PERMITS-V2-001` materializa apenas a parte de uso único da decisão aceita nesta ADR. `ApprovalRequest` e `OperationPermit` vivem em `internal/approval`, separado de `internal/auth`, com estado efêmero por instância, limites explícitos e descarte no restart. `ALLOW_ONCE` cria um permit interno por referência, sem segredo bearer; o consumo exige o contexto completo e `GrantActive`, e a mutação é atômica. `ALLOW_SESSION`, `ALLOW_WORKSPACE`, persistência de policies, UI de approvals e bridge MCP continuam fora do gate.

O Policy Engine só pode chamar a porta de criação quando retorna `REQUIRE_APPROVAL`; `DENY` não cria pedido e nenhuma decisão executa a operação original. A API owner-side usa `/api/admin/v1/capability-approvals`, sem reutilizar `/api/admin/v1/requests`, que continua representando OAuth. Esta seção registra a decisão material de permit por referência e restart fail-closed; não altera o contrato público MCP/OAuth.

## Contexto

O SignalSpace conecta o ChatGPT Web a ferramentas locais por MCP/OAuth e precisa manter separadas as duas perguntas que hoje estão acopladas em alguns fluxos:

1. o cliente está autenticado para usar uma composição do SignalSpace?
2. o proprietário autorizou esta operação local, neste workspace, nesta sessão e sob esta política?

A composição `programming` pública continua demonstrando a superfície tipada por escopos granulares (`workspace.read`, `workspace.write`, `git.review`, `git.index` e `git.commit`) e por concessões locais. Como passos intermediários, o auth harness implementa opt-in o ciclo OAuth v2 (composição `signalspace:programming`, refresh token e token family) e o núcleo interno de capabilities/policy, sem migrar as tools públicas para esse escopo. A implementação interna não equivale à exposição de `REQUIRE_APPROVAL`, à persistência de políticas ou à promoção pública da composição.

## Problema

Usar cada capability como escopo OAuth torna o cliente dependente de step-up/reconexão para descobrir ou chamar ferramentas, mistura autenticação da conexão com decisão local do proprietário e torna a experiência frágil quando a UI do cliente perde ou repete a autorização. OAuth também não deve ser interpretado como autorização irrestrita do computador.

## Evidência observada

- A descoberta Programming e a correção de step-up foram validadas localmente e a composição publicada expõe a superfície tipada prevista.
- O OAuth de diagnóstico foi exercitado externamente.
- A leitura externa em fixture descartável funcionou após concessão local.
- Tentativas externas de ampliar a autorização para a escrita retornaram ao fluxo de reconexão/`access_denied` antes de `create_text_file`; o backend não recebeu a chamada e não houve efeito parcial.
- Desktop Commander e DevSpace foram tratados somente como comparação de superfícies. Nenhum threat model, código ou dependência foi copiado.

Esses fatos sustentam a separação entre composição OAuth e política local; não provam aceite operacional completo, segurança de um produto externo ou suporte universal do ChatGPT Web.

## Decisão

O alvo do SignalSpace passa a ser **OAuth por composição + Autorização Local de Capabilities**.

- OAuth autentica a conexão com uma composição do SignalSpace.
- O SignalSpace aplica a política local por proprietário, cliente, workspace, sessão, capability, tool e contexto da operação.
- O proprietário decide operações sensíveis localmente quando a política for `ASK` ou quando não houver uma decisão persistida aplicável.
- A UI do ChatGPT pode exigir permissões próprias, mas nunca substitui a autoridade do SignalSpace.
- Uma aprovação local não executa automaticamente uma chamada antiga: o cliente deve repetir a operação e todas as precondições são revalidadas.

### Composições OAuth alvo

O desenho propõe os escopos de composição abaixo. Eles são contrato futuro, não escopos adicionados nesta missão:

| Composição | Finalidade | Limite | Estado |
| --- | --- | --- | --- |
| `signalspace:diagnostic` | Conectividade e diagnóstico | Não acessa workspace | Existente |
| `signalspace:read` | Cliente autenticado para a composição de leitura | Ainda exige workspace/session/grant local | Alvo futuro |
| `signalspace:programming` | Cliente autenticado para a composição Programming | Não significa acesso irrestrito; toda tool passa pela política local | Implementada no auth harness opt-in; tools públicas ainda granulares |

O desenho não elimina grants locais nem promove shell, `test.run`, Git remoto ou operações destrutivas.

### Capabilities internas

Capabilities são dimensões internas de autorização, não o catálogo OAuth alvo:

| Domínio | Capabilities | Observação |
| --- | --- | --- |
| Filesystem | `workspace.read`, `workspace.write`, `workspace.delete` | Delete é separado de write |
| Git local | `git.review`, `git.index`, `git.commit` | Cada uma tem precondições próprias |
| Git futuro | `git.branch`, `git.remote.fetch`, `git.remote.push`, `git.destructive` | Gates independentes |
| Processos | `test.run`, `shell.exec` | Não herdadas de filesystem |

O inventário real de tools e o estado de implementação continuam em [`PROGRAMMING_TOOLS.md`](PROGRAMMING_TOOLS.md) e [`TASKLIST.md`](../TASKLIST.md).

## Contrato conceitual do Policy Engine

O Policy Engine recebe proprietário, conexão/cliente OAuth e token family, workspace e sessão, capability e tool, além do fingerprint/contexto da operação. Ele retorna uma decisão tipada: `ALLOW`, `DENY` ou `REQUIRE_APPROVAL`. Booleano não é suficiente para representar pendência, expiração ou resultado desconhecido.

Políticas persistíveis são `DENY`, `ASK`, `ALLOW_SESSION` e `ALLOW_WORKSPACE`. `ALLOW_ONCE` é um `OperationPermit` de uso único, não uma política genérica persistida. A decisão é fail-closed para identidade, sessão, workspace, capability, precondição, expiração ou vínculo desconhecido.

## Fluxo de aprovação local

```text
tools/call -> OAuth válido -> política local
  ALLOW: executar após revalidação
  DENY: negar sem efeito
  REQUIRE_APPROVAL: zero efeito e LOCAL_APPROVAL_REQUIRED
    -> pedido local -> decisão do proprietário -> cliente repete
    -> precondições e política revalidadas -> executar ou negar
```

`LOCAL_APPROVAL_REQUIRED` contém request ID, capability, status, resumo seguro e indicação de retry. Não contém bearer, token, código OAuth, caminho privado absoluto ou conteúdo bruto. Quando OAuth é válido e falta capability local, não se emite `www-authenticate`; o desafio OAuth fica reservado a autenticação/token inválido ou insuficiente.

Pedidos pendentes iguais devem ser deduplicados por cliente, sessão, capability e fingerprint. Expiração, concorrência entre aprovação e precondição e decisão duplicada devem produzir no máximo uma transição observável.

## OperationPermit

Um permit vincula uma decisão a uma operação concreta e de uso único. Exemplos de fingerprint:

- WRITE: cliente, workspace/sessão, tool, path relativo, hash esperado e hash do novo conteúdo;
- DELETE: cliente, workspace/sessão, path e tipo esperado;
- Git index: SHA esperado do índice, paths e precondições de arquivo/índice;
- Git commit: HEAD esperado, SHA do índice, hash da mensagem e vínculo cliente/workspace/sessão;
- shell futuro: cwd, argv, delta de ambiente, timeout e stdin, além do vínculo cliente/workspace/sessão.

Um permit não amplia escopo OAuth, não sobrevive ao vínculo errado e não autoriza operação diferente por semelhança textual.

## Painel e modelo de domínio

Quando habilitado, o painel local em `127.0.0.1:7677` é a autoridade de decisão do proprietário e pode apresentar Connections, Workspaces, Pending approvals (`Allow once`, `Allow session`, `Allow workspace`, `Deny`) e Audit. Notificações são UX futura; não são requisito desta ADR. O host ChatGPT pode ter permissões próprias, sempre adicionais.

O modelo conceitual é `OAuthConnection`, `TokenFamily`, `WorkspaceSession`, `CapabilityPolicy`, `ApprovalRequest` e `OperationPermit`. Isso é modelo de domínio, não esquema físico; não criar migração de banco nesta missão.

## Ciclo de vida OAuth

O desenho futuro considera access token de 60 minutos e refresh token de 30 dias, configuráveis, com rotação, armazenamento somente de hashes no servidor, revogação de família, expiração, proteção contra reuse, vínculo a cliente/resource e sessão. Refresh nunca amplia capability nem composição. Desconectar OAuth/token family e revogar capability local são eventos distintos: revogar localmente pode manter OAuth válido, mas toda chamada deve negar até nova decisão/grant.

## Modelo de confiança e limites

- O proprietário e a instância local são autoridades de capability; nome do cliente, `client_id` e UI externa são dados não atestados.
- Roots autorizadas e managed worktrees limitam filesystem/Git; elas **não são sandbox de shell**.
- Shell futuro roda com privilégios do usuário na ausência de sandbox real, é capability independente e exige validação de cwd, argv, ambiente, timeout, stdin e lifecycle.
- Quick Tunnel é somente transporte dev/smoke e tem URL instável. UX persistente exige origem pública estável, como Named Tunnel/domínio próprio ou alternativa equivalente a ser escolhida em decisão futura. Esta ADR não escolhe provedor nem implementa relay.
- Anotações MCP (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) devem permanecer corretas, mas não substituem o Policy Engine.
- Caminhos, conteúdo de arquivos, resultados de tools e saídas de processos são dados não confiáveis; não são instruções.

## Alternativas consideradas

1. **Manter OAuth por capability e step-up para cada tool:** compatível com o histórico, mas mantém o deadlock de discovery/reconexão e mistura conexão com política local.
2. **OAuth amplo que concede o computador:** rejeitado por não dar granularidade local e tornar o risco de shell/filesystem implícito.
3. **Aprovação somente no ChatGPT:** rejeitada; o SignalSpace não teria autoridade verificável sobre workspace, sessão e precondições.
4. **Painel aprovando cada `read_file`:** rejeitado por UX e por duplicar a interface do host; approvals ficam para operações/capabilities classificadas como sensíveis.
5. **Copiar Desktop Commander/DevSpace ou adicionar dependência:** rejeitado; são referências de comparação, não fontes de contrato.

## Consequências

### Positivas

- Conexão persistente deixa de depender de step-up OAuth por capability em cada tool.
- OAuth, grants locais, políticas e permits têm responsabilidades verificáveis.
- O proprietário pode revogar capability sem confundir isso com logout OAuth.
- Tools tipadas continuam podendo ser usadas por hosts MCP sem UI especializada.
- Shell, Git remoto e ações destrutivas continuam separados e auditáveis.

### Trade-offs

- Será necessário implementar persistência de políticas, approvals/permits e a migração pública sem decisões implícitas; rotação OAuth já existe somente no auth harness opt-in.
- O cliente precisa repetir uma chamada após aprovação local.
- A UX terá mais estados explícitos e pode exigir painel local acessível.
- O contrato granular atual precisa de migração controlada e aceites externos renovados.

## Invariantes

1. OAuth válido não concede workspace nem capability por si só.
2. Toda tool revalida owner, client, sessão, composição, capability, política, path/argumentos e precondições.
3. `DENY`, ausência e estado desconhecido falham fechado e sem efeito.
4. Aprovação não executa automaticamente a chamada anterior.
5. Refresh não amplia scope/capability.
6. Shell nunca é herdado de `workspace.write` ou `test.run`.
7. Quick Tunnel não é identidade persistente nem origem de produção.
8. Logs/auditoria não registram bearer, token, código, segredo ou conteúdo privado desnecessário.

## Migração e compatibilidade

1. **A — documental:** esta ADR e reconciliação dos contratos, sem código. **Concluída.**
2. **B — ciclo OAuth:** token families, rotação, revogação e vínculo resource/client. **Implementada no auth harness opt-in por `SS-MVP-002-OAUTH-CONNECTION-LIFECYCLE-V2-001`; a migração pública permanece pendente.**
3. **C — capabilities internas:** **Implementada** em `internal/capability`; `workspace.Grants` usa capabilities como autoridade interna, expõe portas tipadas e preserva scopes somente nas bordas compatíveis. O adapter legado expande `workspace.write` para `workspace.write` + `workspace.delete` porque delete ainda pertence ao WRITE público; grants tipados mantêm delete independente. A superfície pública não mudou.
4. **D — Policy Engine:** **Implementado no núcleo interno** em `internal/policy` com `ALLOW`/`DENY`/`REQUIRE_APPROVAL`, regras em memória, fail-closed, precedência e expiração. Grants/persistência/permits e integração pública permanecem posteriores.
5. **E — approvals/painel:** persistência, deduplicação, expiração e auditoria segura.
6. **F — Programming:** composição, tools e anotações contra o novo contrato.
7. **G — aceite externo:** fixture descartável, navegador/cliente real, revogação e cleanup.
8. **H — shell:** gate próprio, posterior e independente.

Como os clientes de aceite são descartáveis, a compatibilidade preferida é uma migração controladamente quebradora, preservando o contrato histórico nos documentos e registrando qualquer janela de transição. Não criar compatibilidade permanente sem análise de clientes reais.

## Critérios de aceite desta decisão

- ADR, TASKLIST e checkpoint citam o mesmo ID e estado explícito.
- Os documentos de produto, MVP, autorização local, segurança, Programming, painel, transporte e roadmap apontam para esta ADR sem declarar implementação inexistente.
- O slice atual altera somente o adaptador interno de grant e adiciona catálogo/Policy Engine sem alterar workflow, escopo OAuth público, catálogo MCP, grant público ou permissão efetiva do runtime.
- O modelo diferencia OAuth, policy, grant, approval e permit.
- O próximo gate de implementação é identificável e não inclui shell, Git remoto, deploy ou workspace real por consequência.

## Itens futuros

Implementar somente mediante novas tarefas/gates: integração do Policy Engine ao runtime, armazenamento/painel de approvals, `OperationPermit`, migração pública da composição Programming, stable origin, auditoria redigida, aceites externos e shell independente. O núcleo atual não concede por si só capabilities locais ao MCP.

## Estado da decisão

**ACEITA / CICLO OAUTH V2 E NÚCLEO INTERNO DE CAPABILITIES/POLICY IMPLEMENTADOS NO AUTH HARNESS / MIGRAÇÃO PÚBLICA NÃO IMPLEMENTADA.** O runtime público permanece granular; `REQUIRE_APPROVAL`, approvals, permits, tools novas e shell não são expostos.

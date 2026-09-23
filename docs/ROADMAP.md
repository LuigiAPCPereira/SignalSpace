# SignalSpace — roadmap recuperável dos marcos

**Status:** plano de execução documental da frente de backend do [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), não compromisso de datas ou autorização de ampliar o MVP. Fontes do escopo: [produto](PRODUCT.md), [MVP](MVP.md), [contrato administrativo](LOCAL_ADMIN_AUTHORIZATION.md) e pedido do proprietário para continuar o backend mantendo o PR draft. Detalhes executáveis/estado por ID: [`../TASKLIST.md`](../TASKLIST.md). Revalidar branch e HEAD na retomada.

**Estado corrente (23/09/2026):** `codex/mvp-vertical-programming` e `origin/codex/mvp-vertical-programming` estão em `d1bff2c925e83061bcbc15cef299b22846ac897b`. A composição opt-in READ + WRITE + Git observacional de `SS-MVP-002` foi implementada, validada localmente e publicada nessa branch. Isso não é aceite operacional externo/HTTPS/ChatGPT Web e não promove `test.run`, shell ou mutação Git.

| Marco | Resultado verificável | Dependências | Tarefas | Estado observado | Evidência/limitação |
| --- | --- | --- | --- | --- | --- |
| M1 — conexão | MCP/OAuth diagnosticado com teste real e negação sem autorização. | Consentimento e transporte HTTPS. | SS-BE-001 (evidência de diagnóstico) | smoke relatado pelo proprietário; estado limitado à sessão observada | [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), [QUICK_TUNNEL.md](QUICK_TUNNEL.md). Identidade do software cliente não atestada. |
| M2 — leitura opt-in | Leitura e listagem limitadas à concessão explícita, negativas após revogação. | M1, OAuth read e grant terminal separado. | SS-BE-001 | smoke relatado, com lacunas de correlação | [WORKSPACE_SECURITY.md](WORKSPACE_SECURITY.md), PR #1. Não implica edição, Git ou shell. |
| M3 — backend de autorização local | 7676 público/7677 administrativo isolados; pareamento, sessão, pedido OAuth versionado, status público restrito e testes negativos/Quick real. | M1/M2 como baseline, execução SS-BE-002 a SS-BE-008; lifecycle SS-BE-004 antes de exposição. | SS-BE-002, SS-BE-003, SS-BE-004, SS-BE-005, SS-BE-006, SS-BE-007, SS-BE-008 | em andamento; componentes isolados têm CI, integração e smoke pendentes | [LOCAL_ADMIN_AUTHORIZATION.md](LOCAL_ADMIN_AUTHORIZATION.md), [CI #107](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521299236). CI de componentes não valida painel exposto nem navegador. |
| M4 — integração frontend (etapa separada) | Interface funcional revisada contra o backend comprovado e smoke ponta a ponta com workspace descartável. | M3 validado; autorização específica para integrar branch/protótipos; revisão de acessibilidade/CSP. | **Não consta como tarefa ativa de backend**; exige novo escopo e ID após autorização. | não iniciada nesta frente | [Alinhamento de UX na branch frontend](https://github.com/LuigiAPCPereira/SignalSpace/blob/feat/frontend-oauth-consent/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md) registra critérios de entrada ainda não cumpridos. |

## Ordem, riscos e limites

Na próxima fatia de código autorizada, completar **SS-BE-004** (`decided_at`, `COMPLETED`/`EXPIRED`, tombstones, retenção, método legado e reconciliação), depois revisar as dependências de SS-BE-002/003/005/006/007 e executar SS-BE-008 somente após os gates de segurança. Componentes já entregues não precisam ser recriados. Falhas de bind 7677, vazamento de endpoint admin para túnel, grant revogado, corrida ou resposta perdida devem falhar fechado e ser testadas. O PR permanece draft e sem merge até autorização específica.

**Fora do escopo autorizado da frente original M1–M4:** edição, Git, shell, aprovações individuais de ferramentas, HTML funcional e merge. A frente posterior M6 autoriza somente a composição opt-in já descrita abaixo; o MVP de longo prazo contém outras fatias, mas este roadmap não inventa datas nem promove `test.run` ou shell. Histórico de decisões e marcos em [`SESSION_LOG.md`](SESSION_LOG.md); checkpoint derivado em [`PROJECT_STATE.md`](PROJECT_STATE.md).

## Reabertura do MVP — programação vertical local

Esta seção registra a nova frente autorizada pela missão de 20/09/2026 sem apagar a fronteira histórica de M1–M4. A implementação ocorre na branch isolada `codex/mvp-vertical-programming`; a branch `feat/frontend-oauth-consent` não foi alterada.

| Marco | Resultado verificável | Dependências | Tarefas | Estado observado | Evidência/limitação |
| --- | --- | --- | --- | --- | --- |
| M5 — integração administrativa local | Página funcional no listener 7677 consulta sessão, pareia/desbloqueia, lista e decide pedidos com cookie HttpOnly/CSRF em memória. | SS-BE-003/005; contrato frontend reaberto na branch frontend | SS-MVP-001 | implementada não validada | `internal/admin/ui/`, `internal/admin/ui.go`, `internal/admin/ui_test.go`, commits `9fdf575` e `9e26c19`; HTTP local, CSRF bootstrap e sintaxe JS passaram. Sem navegador real. |
| M6 — programação vertical local | Composição opt-in pública com concessão local e escopos independentes para READ, edição e revisão Git somente leitura, com negação após revogação. `test.run` permanece separado. | WORKSPACE_SECURITY; `PROGRAMMING_TOOLS.md`; promotion gate | SS-MVP-002…006 | implementação e validação local confirmadas; publicação remota confirmada; aceite operacional externo pendente | `e97aaf7` e `d1bff2c`; testes OAuth/MCP, suíte Go, race, vet, build, gofmt e diff-check passaram. Não é HTTPS/ChatGPT Web, workspace real ou sandbox de processo; shell e mutação Git permanecem fora. |

O próximo bloco não é repetir a promoção já implementada/publicada: é um aceite operacional externo de READ + WRITE + Git, caso seja autorizado, ou um contrato separado para shell. Ambos exigem missão própria; `SS-MVP-002` permanece PARCIAL.

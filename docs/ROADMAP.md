# SignalSpace — roadmap recuperável dos marcos

## Step-up OAuth por ferramenta — `SS-MVP-002-OAUTH-TOOL-STEPUP-001` — 25/09/2026

Esta fatia corrige a interoperabilidade local da composição `programming`: discovery deixa de depender do scope específico do bearer e os descriptors/challenges passam a representar `diagnostic + capability`. O código em `c2dcdbd` está **IMPLEMENTADO / VALIDADO LOCALMENTE** e foi publicado com a reconciliação documental em `14821b4`; a base/remoto observado antes da missão foi `de8ff7e3d253f4dedbfbba1135bdf53e682cd335`. O aceite externo continua pendente. Não há nova tool, scope ou capability.

**Status:** plano de execução documental da frente de backend do [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), não compromisso de datas ou autorização de ampliar o MVP. Fontes do escopo: [produto](PRODUCT.md), [MVP](MVP.md), [contrato administrativo](LOCAL_ADMIN_AUTHORIZATION.md) e pedido do proprietário para continuar o backend mantendo o PR draft. Detalhes executáveis/estado por ID: [`../TASKLIST.md`](../TASKLIST.md). Revalidar branch e HEAD na retomada.

## Aceite operacional externo — `SS-MVP-002-EXTERNAL-PROGRAMMING-ACCEPTANCE-001` — 25/09/2026

O aceite autorizado usou somente fixture descartável e Quick Tunnel HTTPS real. O aplicativo MCP foi criado no ChatGPT Web e a aprovação local do primeiro pedido OAuth foi registrada, mas o fluxo não emitiu token nem retornou ao callback; `workspace clients` confirmou ausência de cliente com token emitido. A conta observada era Plus e a conversa não expôs o aplicativo criado para invocação após o cadastro.

Resultado: **BLOQUEADO / PARCIAL**. Nenhuma tool programming externa foi chamada, nenhum grant ou managed worktree externo foi criado e não se validou revogação/negação. O processo, túnel e fixture foram limpos. O próximo passo depende de ambiente Web com suporte efetivo a escrita/modificação MCP; a URL Quick Tunnel deverá ser recriada na retomada.

**Estado histórico anterior ao step-up:** a correção posterior ao Git commit v1 estava registrada no remoto live `a0fada0141c314037ec09037b4fb7214d3f3bded`. Esta missão reabriu a ref em `de8ff7e3d253f4dedbfbba1135bdf53e682cd335`, produziu `c2dcdbd` localmente e aguarda publicação fast-forward normal. Isso não é aceite operacional externo/HTTPS/ChatGPT Web e não promove `test.run`, shell, branch ou Git remoto como capability.

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
| M6 — programação vertical local | Composição opt-in pública com concessão local e escopos independentes para READ, edição, onze tools de filesystem tipado e revisão Git somente leitura, com negação após revogação. Managed worktrees v1 é lifecycle local do proprietário e não é MCP público. `test.run` permanece separado. | WORKSPACE_SECURITY; `PROGRAMMING_TOOLS.md`; promotion gate | SS-MVP-002…006 | base/estrutural, `apply_patch`, managed worktree v1 e Git commit v1 implementados/validados; Git commit v1 publicado; aceite operacional externo pendente | `0420c9a` é o remoto live do Git commit v1; a cadeia real inclui o commit documental `ccc4d17`. Testes descartáveis cobrem criação detached, dirty source não copiado, hooks/filtros, restart, revoke/resume, remove limpo/negado e `.git` reservado. Não é HTTPS/ChatGPT Web, workspace real, CI ou sandbox de processo; shell, mutação Git fora da capability tipada e lifecycle MCP permanecem fora. |

O próximo bloco não é repetir a promoção já implementada/publicada: é um aceite operacional externo de READ + WRITE + Git, caso seja autorizado, ou um contrato separado para shell. Ambos exigem missão própria; `SS-MVP-002` permanece PARCIAL.

## Promotion gate de Git index — implementação local — 24/09/2026

O proprietário aprovou `SS-MVP-002-GIT-INDEX-PROMOTION-GATE-001` para a composição opt-in `connect quick programming`. O commit local `e164415` implementa e valida `git_status` sob `signalspace:git.review` e `stage_git_paths`/`unstage_git_paths` sob `signalspace:git.index`, com o segundo escopo permitido somente em managed worktrees aprovadas no console local. O fluxo genérico de checkout permanece sem Git mutável.

O gate está implementado, validado localmente e foi confirmado pelo ChatGPT Web como publicado no remoto live `60c290a88a5c85a411237b53e04313b6d35dfc19`. Essa confirmação vale para a promoção do Git index; não publica o commit gate local descrito abaixo. A fatia não autoriza commit, branch, Git remoto, shell, lifecycle MCP de worktree, CI, merge ou deploy.


## Commit gate Git local — `SS-MVP-002-GIT-COMMIT-GATE-001` — 24/09/2026

O proprietário aprovou a implementação local do próximo degrau tipado de Git, sem push ou alteração remota. A ref foi reconciliada na branch `codex/mvp-vertical-programming`, com remoto live `60c290a88a5c85a411237b53e04313b6d35dfc19` antes da alteração; a implementação foi registrada em `b3f2add` e o hardening de ref/snapshot em `247ad3d`. `signalspace:git.commit` é separado de `workspace.write`, `git.review` e `git.index` e só entra na composição pública opt-in `programming`.

O resultado é limitado a um commit staged-only em managed worktree detached, com identidade owner-local privada, precondições HEAD/índice, ref privada `refs/signalspace/workspaces/<workspace_id>/head` e atualização CAS atômica. Branch, merge, rebase, reset, clean, stash, shell, hooks/signing e Git remoto permanecem fora. A fatia está **IMPLEMENTADA E VALIDADA LOCALMENTE / PUBLICAÇÃO REMOTA PENDENTE**; CI, HTTPS, túnel, navegador, grant externo, workspace real, merge e deploy permanecem desconhecidos/não validados.

## Reconciliação publicada — `SS-MVP-002-GIT-COMMIT-PUBLISH-RECONCILE-001` — 25/09/2026

A implementação Git commit v1 foi publicada por fast-forward normal no remoto `0420c9ae14768e099fbb51452ee71e7dc5af7916`. A cadeia real é `60c290a` → `b3f2add` → `ccc4d17` → `247ad3d` → `0420c9a`; o commit intermediário `ccc4d17` é exclusivamente documental e compatível com o gate autorizado. O publish gate foi concluído com desvio procedural de pré-checagem, porque a instrução de exatamente três commits foi verificada depois do push. Não há reescrita de histórico.

O estado corrente é **DOCUMENTAÇÃO CANÔNICA RECONCILIADA / PUBLICATION INCIDENT REGISTRADO / HISTÓRICO PRESERVADO / PUSH FAST-FORWARD VERIFICADO**. `SS-MVP-002` e `SS-BE-007` permanecem parciais; CI, HTTPS, túnel, navegador, grant externo, workspace real, merge e deploy permanecem desconhecidos/não validados. Nenhuma nova capability é iniciada por esta reconciliação.

## Direção pós-M6 — coding agent completo dentro de SS-MVP-002

A decisão de produto de 23/09/2026 amplia a direção de longo prazo sem declarar implementação pronta. O alvo é um ChatGPT Web capaz de trabalhar como coding agent completo no workspace autorizado, com filesystem e busca nativos, Git tipado e UX especializada por domínio. Essa direção permanece vinculada a `SS-MVP-002` enquanto a fronteira de autorização e composição estiver em evolução; não cria por si só um novo ID, um novo escopo OAuth ou permissão de shell.

Sequenciamento recomendado dentro da frente existente:

1. consolidar o motor seguro de filesystem e adicionar contratos estruturados para inspeção/busca/criação/escrita;
2. completar copy/move/delete e, depois, `apply_patch` sobre o mesmo motor;
3. evoluir Git observacional para Git local tipado com capacidade separada e hardening contra hooks/configuração executável;
4. tratar Git remoto (fetch/push) como fronteira própria de rede/credenciais;
5. adicionar UX especializada por domínio sobre resultados estruturados, preservando funcionamento sem widget;
6. manter shell/execução arbitrária e operações Git destrutivas sob decisões/gates próprios.

O próximo slice técnico recomendado é o item 1. Ele deve preservar `replace_text`, `read_file`, `list_directory` e `review_git_changes` como regressões e não promover shell, `test.run` ou mutação Git por consequência.

## SS-MVP-002 — slice de filesystem tipado base concluído localmente

O item 1 foi executado nesta retomada em dois commits locais: implementação/testes e documentação/checkpoint/direção de worktrees. O motor comum adiciona `stat_path`, `find_paths`, `search_text`, `create_directory`, `create_text_file` e `write_text_file`, com resultados estruturados, limites explícitos, validação de caminho/no-symlink, create-only e precondição/hash para escrita integral. A matriz de escopos permanece separada: READ em `signalspace:workspace.read` e WRITE em `signalspace:workspace.write`.

O segundo commit também registra que worktrees são direção futura, não capacidade exercitada. Nenhuma worktree foi criada; não houve alteração das branches protegidas, integração do frontend, Git mutável, shell, `test.run`, copy/move/delete/apply_patch, CI, túnel, OAuth externo, workspace real, merge ou deploy. Os dois commits foram rebaseados sobre o remoto live sem merge, sem force-push e sem push.

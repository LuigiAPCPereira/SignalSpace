# SignalSpace — checkpoint de continuidade

**Natureza:** snapshot derivado da [TASKLIST](../TASKLIST.md), contratos, Git e CI; não é autorização nem lock. O baseline histórico de 20/09/2026 em [`3deefc1`](https://github.com/LuigiAPCPereira/SignalSpace/commit/3deefc1) permanece documentado abaixo; o checkpoint vigente desta ref é a reconciliação publicada abaixo. PR #1 continua aberto/draft/não mesclado e CI permanece desconhecida quando não explicitamente observada.

## Checkpoint vigente — `SS-MVP-002-OAUTH-TOOL-STEPUP-001` — 25/09/2026

**Estado:** `IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICADO REMOTAMENTE` por fast-forward normal. A causa raiz foi corrigida em `c2dcdbd`: `tools/list` da composição `programming` não depende mais do scope específico do bearer; a execução continua fechada por `verify()` e pelo grant local. `SS-MVP-002` permanece **PARCIAL/em andamento** e o aceite externo permanece pendente.

**Ref:** branch `codex/mvp-vertical-programming`; base local/remota reconciliada em `de8ff7e3d253f4dedbfbba1135bdf53e682cd335` antes da edição; código em `c2dcdbd` e remoto live verificado diretamente após os pushes. Não houve merge, rebase ou force-push.

**Contrato validado:** bearer diagnostic-only descobre a superfície Programming completa, com os descriptors cumulativos `diagnostic + capability`; READ, WRITE, Git review, Git index e Git commit retornam challenge MCP `insufficient_scope` sem executar backend quando falta o scope. `diagnostic` e `read` preservam seus limites e `test.run` continua ausente do entrypoint.

**Evidência:** suíte Go serial, race de `internal/mcp`, `internal/auth` e `cmd/signalspace`, vet, build, Node consent, gofmt e diff-check passaram. Patches protegidos preservados com SHA-256 registrados na TASKLIST. CI, Quick Tunnel/OAuth Web no HEAD novo, workspace real, merge e deploy permanecem desconhecidos/não validados.

**Próxima ação vinculada:** recriar Quick Tunnel/OAuth descartável para aceite externo no SHA publicado; parar no handoff Web se o cliente exigir nova aprovação.

## Checkpoint histórico — `SS-MVP-002-EXTERNAL-PROGRAMMING-ACCEPTANCE-001` — 25/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL** e o aceite operacional externo está **BLOQUEADO** antes da emissão de token programming. O aplicativo MCP foi criado e a aprovação local do primeiro pedido OAuth foi registrada, mas a página ficou em `Concluindo autorização…`; `workspace clients` confirmou que não havia cliente com token emitido. Não há evidência de grant, sessão MCP ou chamada externa de ferramenta.

**Ref/publicação:** branch `codex/mvp-vertical-programming`; este é o registro histórico anterior ao step-up, cuja correção publicada era `a0fada0141c314037ec09037b4fb7214d3f3bded`. O HEAD desta missão é `c2dcdbd`, ainda pendente de publicação; a documentação foi reaberta para refletir essa divergência.

**Limites:** Quick Tunnel HTTPS e cadastro do aplicativo foram observados; READ/WRITE/filesystem, Git review/index/commit, managed worktree, revogação e negação não foram validados no Web. A conta observada era Plus e o aplicativo não ficou disponível na composição da conversa; a orientação exibida apontou suporte beta de escrita/modificação MCP em planos superiores.

**Limpeza/próxima ação:** processo e túnel encerrados, portas `7676`/`7677` livres, estado Quick temporário e fixture descartável removidos. Repetir somente em ambiente Web com suporte efetivo à escrita/modificação MCP, usando nova URL Quick Tunnel; não contornar OAuth nem editar tokens.

## Checkpoint vigente — reconciliação do Git commit v1 — 25/09/2026

**Ref observada:** `codex/mvp-vertical-programming`; remoto live final `0420c9ae14768e099fbb51452ee71e7dc5af7916`. A cadeia publicada real é `60c290a` → `b3f2add` → `ccc4d17` → `247ad3d` → `0420c9a`. O commit `ccc4d17a9ed8b8c744d432edd9da3db5687ac853` é exclusivamente documental e registra o gate já autorizado.

**Estado histórico:** `SS-MVP-002-GIT-COMMIT-GATE-001` está **APROVADO / IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICADO REMOTAMENTE**. `SS-MVP-002-GIT-COMMIT-PUBLISH-001` está **CONCLUÍDO COM DESVIO PROCEDURAL DE PRÉ-CHECAGEM**: o push foi fast-forward normal, mas a checagem de exatamente três commits revelou posteriormente o commit documental intermediário. Não há motivo técnico para reescrever histórico válido. O checkpoint vigente acima substitui este estado para a missão de step-up.

**Preservação e limites:** os dois patches protegidos permanecem não rastreados, não aplicados e intocados; PR #1, `feat/m1-local-mcp-diagnostic` e `feat/frontend-oauth-consent` não foram alvo. Não houve force-push, merge, rebase, CI ou deploy. HTTPS, túnel, navegador, grant ChatGPT Web, workspace real e aceite operacional externo continuam não validados.

**Próxima ação vinculada:** entregar o handoff consolidado ao ChatGPT Web usando `<continuidade_codex>` e aguardar nova missão; não iniciar nova capability.

## Checkpoint histórico — SS-MVP-002 Git local commit gate — 24/09/2026

**Ref observada:** branch `codex/mvp-vertical-programming`; antes da alteração, o remoto live confirmado era `60c290a88a5c85a411237b53e04313b6d35dfc19`. A implementação foi registrada no commit local `b3f2add` (`feat(signalspace): add managed Git commit gate`) e o hardening em `247ad3d`; não houve push, merge, deploy ou mutação remota.

**Objetivo/estado:** `SS-MVP-002` está **IMPLEMENTADA E VALIDADA LOCALMENTE NESTA FATIA / PARCIAL NO ACEITE OPERACIONAL EXTERNO**. `SS-MVP-002-GIT-COMMIT-GATE-001` está **IMPLEMENTADO E VALIDADO LOCALMENTE / PUBLICAÇÃO REMOTA PENDENTE**. CI, HTTPS, túnel, navegador, grant ChatGPT Web, workspace real, merge e deploy permanecem **DESCONHECIDOS/NÃO VALIDADOS**.

**Implementação:** `signalspace:git.commit` é independente de READ, WRITE, `signalspace:git.review` e `signalspace:git.index`; `commit_git_index` aceita somente `session_id`, `expected_head_oid`, `expected_index_sha256` e `message`. A identidade owner-local é privada e versionada. O manager exige managed worktree detached, base ancestral sem merge, índice staged-only, `write-tree`, `commit-tree` por stdin e transação CAS única de HEAD/ref privada; falhas e estado incerto não viram sucesso. Worktrees com commits locais não podem ser removidas.

**Validação:** passaram `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, race serial de `internal/workspace`, `internal/auth`, `internal/mcp` e `cmd/signalspace`, `go vet ./...`, `go build ./...`, `gofmt -l cmd internal` e `git diff --check`. Não há listener/processo SignalSpace ou cloudflared residual. Os dois patches protegidos seguem não rastreados, não aplicados e com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86e3f1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Próxima ação vinculada:** enviar este checkpoint/relatório ao ChatGPT Web usando `<continuidade_codex>` e aguardar a próxima missão; não publicar remotamente sem autorização posterior.

## Checkpoint histórico — SS-MVP-002 apply_patch estruturado — 24/09/2026

**Ref observada:** branch `codex/mvp-vertical-programming`; a sequência local `f2bed05` → `4e597c9` → `e4d5dd2` inclui a implementação, a documentação e o endurecimento local do preflight, enquanto o remoto live foi confirmado em `3628d2fe4b08053c63af50d7d18c1d62fc37613f` antes desta missão. A árvore preserva somente os dois patches não rastreados do proprietário, fora do Git, não aplicados e com os hashes `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Objetivo/estado:** `SS-MVP-002` está **implementada e validada localmente nesta fatia, mas PARCIAL quanto ao aceite operacional externo**. O `apply_patch` é público somente na composição `programming`, com o escopo existente `signalspace:workspace.write`; diagnostic/read não o anunciam. A publicação remota desta fatia não foi autorizada pela missão e não foi feita.

**Implementação:** `internal/workspace/patch.go` adiciona operações fechadas `create_file`, `write_file`, `move_path`, `delete_file` com hash obrigatório para update/delete e `create_directory` limitado; preflight valida caminhos, symlinks, tipos, colisões, limites de 128 operações/1 MiB textual e precondições antes de mutar. A execução reutiliza `Session`/`Grants` e compensa operações anteriores em falha, retornando estado estruturado sem raízes absolutas. `internal/mcp/workspace_patch.go` faz schema/parser fechado, autorização por `workspace.write`, anúncio condicionado à composição e dispatch MCP.

**Validação:** passaram `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, race de `internal/workspace`, `internal/mcp` e `cmd/signalspace`, `go vet ./...`, `go build ./...`, `gofmt -l cmd internal` e `git diff --check`, além dos testes focais com chamada MCP real, regressão de listas/instruções, criação/atualização, stale hash e preflight sem mutação. Não houve HTTPS, túnel, navegador, grant ChatGPT Web, workspace real, CI, merge, deploy ou push.

**Próxima ação vinculada:** devolver ao ChatGPT Web o relatório com os SHAs locais e aguardar nova missão. A publicação remota desta fatia continua fora desta missão.

## Checkpoint histórico — SS-MVP-002 filesystem estrutural — 23/09/2026

**Ref observada:** branch `codex/mvp-vertical-programming`; antes da alteração, HEAD local e remoto live coincidiam em `6b8adc5b44e4856a743f0df336241aff5b5da79e`. A árvore inicial tinha somente `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` não rastreados; seus hashes foram rechecados e permanecem `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`. Não houve alteração de PR #1, `feat/m1-local-mcp-diagnostic` ou `feat/frontend-oauth-consent`.

**Objetivo/estado:** SS-MVP-002 está **implementada e validada localmente nesta fatia, mas PARCIAL quanto ao aceite operacional externo**. O promotion gate continua **APROVADO PELO PROPRIETÁRIO** para READ + WRITE + Git observacional; isso não autoriza push desta fatia, HTTPS, túnel, navegador, grant ChatGPT Web, CI, merge ou deploy.

**Implementação:** commit local `347f84bd290e09f1c50c962623feb64976627d82` adiciona `internal/workspace/structural.go` com `Copy`, `Move`, `DeleteFile` e `DeleteDirectory`; `Grants` revalida owner/client/session e `ScopeWrite`; `internal/mcp` adiciona `copy_path`, `move_path`, `delete_file` e `delete_directory`; a composição `NewOAuthProgrammingHandler` exige as portas estruturais e as registra somente em programming. Copy usa limites `32` níveis/`4096` entradas/`64 MiB`, ordenação, no-follow, no-overwrite, permissões `0600`/`0700` e cleanup explícito; move usa `renameat2` no-replace e rejeita `EXDEV`; delete é não-recursivo e delete-directory aceita somente vazio. `apply_patch`, shell, `test.run`, Git mutável, artifacts e worktree não foram implementados.

**Validação/limites:** passaram `gofmt -l cmd internal`, `git diff --check`, `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, race serial de `internal/workspace`, `internal/mcp` e `cmd/signalspace`, `go vet ./...` e `go build ./...`. Testes focais e negativos cobrem symlink/traversal/tipos, destino existente, limites, cleanup parcial, diretório não vazio/raiz, owner/client/session, escopo/revoke e regressões MCP. O caso `EXDEV` não foi testável na fixture local; escritores externos, transação filesystem, exactly-once, HTTPS, navegador, OAuth externo, workspace real e CI permanecem não validados. O commit documental será criado agora, sem push, e o relatório será enviado ao ChatGPT Web.

## Checkpoint histórico — SS-MVP-002 filesystem tipado base — 23/09/2026

**Ref e preservação:** branch `codex/mvp-vertical-programming`; HEAD local observado antes do slice `41966e1300072f8dc4519f3a7837eccbb41fafd2`. O `git ls-remote` live da branch retornou `1cf598a5222a8118d268752dbf2bbd0993f437ff`; não houve rewind, commit ou push nesta missão. A ref tracking local `origin/...` em `ac5b6ee` é stale e não foi usada como estado remoto atual. Os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` continuam não rastreados, fora do Git, não aplicados e preservados com os hashes registrados. PR #1, `feat/m1-local-mcp-diagnostic` e `feat/frontend-oauth-consent` não foram integrados.

**Implementado localmente:** núcleo comum em `internal/workspace/filesystem.go` e wrappers revogáveis em `internal/workspace/grants.go`; seis tools MCP tipadas e resultados estruturados em `internal/mcp/workspace_read.go`/`workspace_write.go`/`server.go`; portas explícitas e composição `programming` em `internal/mcp/oauth.go` e `cmd/signalspace/main.go`. A matriz é READ (`stat_path`, `find_paths`, `search_text`) e WRITE (`create_directory`, `create_text_file`, `write_text_file`) sob os escopos existentes. `read_file`, `list_directory`, `replace_text`, scopes e modos `diagnostic`/`read` mantêm regressões; `test.run`, shell, mutação Git e worktree não foram habilitados.

**Validado:** `go test ./internal/workspace ./internal/mcp ./cmd/signalspace -p=1 -parallel=1 -count=1 -timeout=300s`; `go test -race ./internal/workspace ./internal/mcp ./cmd/signalspace -p=1 -parallel=1 -count=1 -timeout=300s`; `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`; `go vet ./...`; `go build ./...`; teste público focal `TestPublicProgrammingCompositionPromotesOnlyReadWriteAndGit`; `gofmt -l cmd internal`; `git diff --check`. Os testes cobrem path/traversal/symlink, tipo/ausência, limites, UTF-8/NUL, glob literal, busca bounded, create-only, hash/precondition conflict, owner/client/session/scope/revoke, argumentos inválidos, resultados estruturados e ausência de leak de caminho absoluto.

**Não validado/desconhecido:** não houve validação operacional de 7676/7677, HTTPS, túnel, navegador, grant ChatGPT Web, workspace real, OAuth externo, merge ou deploy. CI permanece desconhecida. Os gates Go desta missão passaram com as portas livres no fechamento; nenhum processo foi interrompido.

**Worktree futura:** somente direção documental em `docs/PROGRAMMING_TOOLS.md`; nenhuma worktree foi criada/removida. Invariantes propostos e questões abertas permanecem explicitamente futuros. **Próxima ação:** retornar ao ChatGPT Web para a decisão A/B/C da missão; não executar commit/push nem iniciar copy/move/delete/apply_patch, Git mutável, worktree ou shell.

## Fontes e fronteiras

- Protocolo canônico na ref: [`DOCUMENTATION_AND_CONTINUITY.md`](../DOCUMENTATION_AND_CONTINUITY.md) v2.0 adaptado e [`AGENTS.md`](../AGENTS.md). Anexo geral do Project estático, sem sincronização automática; acesso Codex/agendamentos não presumido.
- Requisitos: [`LOCAL_ADMIN_AUTHORIZATION.md`](LOCAL_ADMIN_AUTHORIZATION.md), [`WORKSPACE_SECURITY.md`](WORKSPACE_SECURITY.md), [`PRODUCT.md`](PRODUCT.md), [`MVP.md`](MVP.md), [Quick terminal](QUICK_TUNNEL.md) e [Quick API local](QUICK_PANEL.md). Cabeçalhos históricos do contrato não prevalecem sobre código vivo. [TASKLIST](../TASKLIST.md) é autoridade de IDs/status SS-BE-001…008; [ROADMAP](ROADMAP.md), [SESSION_LOG](SESSION_LOG.md) e [ADOPTION_REPORT](ADOPTION_REPORT.md) têm funções distintas.
- `feat/frontend-oauth-consent` é outra frente: HEAD confirmado `3911c4e`, `frontend/consent/prototypes/owner-pairing-flow.html` (pareamento/desbloqueio) e `frontend/consent/prototypes/local-approval-flow-v2.html` (decisão) são **somente demonstrativos** sem HTTP, cookies nem autenticação; `frontend/consent/consent.html` legado. [Alinhamento UX](https://github.com/LuigiAPCPereira/SignalSpace/blob/3911c4eae2ec514ff5f30d904a993cf103f9be32/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md). Preservar branch alheia; sem integração/merge/deploy/escrita/Git/shell MCP sem autorização. Conector GitHub não inspeciona worktree/stashes do proprietário.

## Evidências e grau de observação

- **SS-BE-001:** proprietário relatou no M2 `connection_diagnostic`, `read_file`/`list_directory` e negativas pós-revogação; sem logs individuais completos ou prazo JWT medido. Leitura **não repetida** no smoke M3 com painel.
- **SS-BE-003/004:** Gate Argon2id com sal, limites, sessões/CSRF e máquina OAuth compartilhada, decisões versionadas, retenção até 10 min/64 sem segredos, grant revalidado, código apenas `/authorize/complete`. [CI #134](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35525837834) e [CI #124](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35524113001) PASS componente/domínio; sem auditoria independente. M3 relatou decisão administrativa OAuth efetiva; não atesta detalhes de cada transição interna.
- **SS-BE-005/007 (HTTP local):** OAuth/Gate reais em dois sockets, Host/Origin/CSRF, erros/versões, expiração 410/404 via fixture, perda efetiva de socket após decisão/antes da resposta reconciliada por GET, grant real revogado nega aprovação e conclusão. [CI #146](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35531123085) e [CI #152](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35532258138) PASS; não rede Cloudflare.
- **SS-BE-006:** `/authorize/status` cookie OAuth + ID próprio, CSP JSON, no-referrer/CORP, Host/Origin/no-CORS, [CI #160](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35533172412) PASS. Em [`0ed5f5a`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0ed5f5a), a página OAuth embutida passou a carregar script same-origin; [`0988fc5`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0988fc53101ef87a8f76c7b08865cf75d5cb872c), [CI #188](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35548396262) PASS, corrigiu `connect-src 'self'`, desabilitação inicial, fallback sem JavaScript e cobertura funcional Node dos estados/falhas. Após autorização explícita, Quick Tunnel real passou o preflight HTTPS e o navegador integrado validou PENDING → APPROVED, botão habilitado e ausência de erros/avisos CSP. O POST final e a matriz negativa visual permanecem pendentes.
- **SS-BE-002/008 (harness Quick):** `connect quick [read] panel` opt-in com confirmação distinta, reserva 7676+7677/credencial antes do tunnel, OAuth após URL, prontidão admin antes de público, cloudflared fixo em `http://127.0.0.1:7676`, shutdown conjunto. Testes com cloudflared simulado e sockets reais: bind ocupado, isolamento, pareamento, fila, shutdown/rebind, [CI #169](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536833997). Falha administrativa pós-readiness, antes/depois de anunciar URL, encerra sessão/libera portas, [CI #176](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35538258629); ambas PASS.
- **SS-BE-005/006/008 (fluxo Quick em harness):** `cmd/signalspace/quick_panel_oauth_test.go`, [`98d06de`](https://github.com/LuigiAPCPereira/SignalSpace/commit/98d06deeaf58bafee3972814bae8c5549f065aa0), [CI #182](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35539286668) PASS: registro, `/authorize`, pareamento, fila, status público cookie-scoped, POST admin APPROVED/v2, conclusão 303 e GET COMPLETED em sockets reais. cloudflared e verificador HTTPS **simulados**; não callback/token real nem navegador.
- **NOVO — smoke M3 real relatado pelo proprietário, 20/09/2026, revisão [`bd60fc3`](https://github.com/LuigiAPCPereira/SignalSpace/commit/bd60fc3c17f84261eedf02d5db0f6679ad29a227):** proprietário declarou **APROVADO** o fluxo de diagnóstico OAuth+MCP com túnel Cloudflare real, API administrativa local e decisão OAuth efetiva; relatou correlação de `diagnosticID` entre ChatGPT Web e terminal, `tools/list` autenticado, isolamento das portas e encerramento/limpeza observados. **Fonte exclusiva: relato entregue nesta conversa; esta sessão não executou o smoke, não recebeu logs brutos, não inspecionou a máquina nem viu o ID concreto.** Não extrapolar para chamada read M3, matriz de ataques completa, token individual auditado, HTML, navegador ou atestação do cliente.

## Estado operacional e próxima ação

**Atualização corrente — HEAD `3deefc1`:** o workflow versiona Node.js `22.14.0` e executa o teste funcional JavaScript. Após `APPROVED`, o polling permanece ativo até o servidor reportar `EXPIRED`; o teste Node cobre essa transição e JSON inválido, e o teste Go comprova rejeição de `/authorize/complete` fora do prazo sem código. Formato, diff, testes Go, race, vet, build, `node --check` e `node --test` passaram localmente. A CI remota deste SHA não foi consultada conforme orientação do proprietário; estado remoto **desconhecido**. Sem nova validação visual, novo túnel, grant OAuth, merge ou deploy.

**Reconciliação remota:** push normal confirmado em `9e93b65a2e897cfb4c41539dd3fb65d3ccc576e1`; `git ls-remote` e `origin/feat/m1-local-mcp-diagnostic` apontam para o mesmo SHA. A CI permanece não consultada por orientação do proprietário.

**HEAD atual:** `66f2641`, commit somente documental de reconciliação publicado na mesma branch; o código permanece em `3deefc1` e os gates/documentação anteriores em `9e93b65`. O remoto corresponde ao HEAD local.

**Validada por teste automatizado:** implementação da API opt-in, isolamento e fluxo OAuth Quick local com processo de túnel simulado; [CI #182](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35539286668) PASS no SHA anterior. **Aceite operacional relatado, escopo diagnóstico:** SS-BE-002 (isolamento/túnel/limpeza) e SS-BE-008 (Quick real + decisão/admin + MCP diagnóstico). **Parcial:** SS-BE-007 ainda exige matriz negativa operacional; código/CI cobrem vários cenários, mas relato M3 não os exercita todos. **SS-BE-006 implementada não validada:** [`0988fc5`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0988fc53101ef87a8f76c7b08865cf75d5cb872c) e [CI #188](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35548396262) comprovam a correção CSP, estado inicial, fallback e testes funcionais dos estados/falhas; Quick Tunnel real e navegador integrado validaram PENDING → APPROVED, habilitação do botão e ausência de erros CSP, mas o POST final e a matriz negativa visual ainda não foram executados. M2 read/revoke tem evidência anterior, não foi repetido com painel neste smoke. Não houve merge, deploy nem habilitação de escrita/Git/shell MCP; HTTP loopback não é HTTPS.

**Próxima tarefa SS-BE-006:** completar a validação visual dos estados negativos/expiração e, com autorização específica para o grant descartável, verificar a submissão `/authorize/complete`; somente depois reavaliar eventual integração visual com a branch `feat/frontend-oauth-consent`, preservando ownership e autorização. **Não há integração autorizada de branches, merge ou deploy por esta atualização documental**; preservar PR draft. SS-BE-007 segue pendente de negativas operacionais adicionais e eventual leitura/revogação no fluxo M3, sem rebaixar o smoke de diagnóstico aprovado.

## Checkpoint atualizado — SS-BE-006 validação visual local — 22/09/2026

**Ref verificada antes dos registros:** branch `codex/mvp-vertical-programming`, HEAD `e97b17efc918cf73a202812a222532ef8d515572`; `origin` estava sincronizada após `git fetch --prune` e `git pull --ff-only`. Os dois patches não rastreados pertencem ao usuário e seus hashes foram preservados. Nenhum arquivo de implementação foi alterado nesta missão.

**Resultado:** estados visuais negativos e submissão controlada foram exercitados no navegador integrado usando a página HTML, CSS e `consent.js` reais do produto, servidos por fixture HTTP temporária em loopback. Observados: PENDING→DENIED; APPROVED→EXPIRED; 403; 404; 429 com recuperação a PENDING; 503 com recuperação; JSON malformado com recuperação; e resposta interrompida durante JSON com recuperação. Botão permaneceu desativado nos estados não aprovados; nenhum desses cenários fez POST. No controle aprovado, houve exatamente um POST para stub local, o botão mostrou “Concluindo autorização…” e o polling parou. O stub não invocava o handler de conclusão real, não emitiu código/token e não chamou callback. A instrumentação descartável contou zero erros JS, rejeições não tratadas, violações CSP ou `console.error` nos cenários repetidos. Assim, SS-BE-006 fica **validada no escopo visual local descartável testado**, não validada como emissão OAuth operacional.

**Limites e limpeza:** não houve Quick Tunnel/cloudflared ou HTTPS real nesta validação, chamada ao emissor `/authorize/complete` real, grant ao ChatGPT Web, CI, workspace real, integração de branches, merge ou deploy. Não se encontrou defeito reproduzível, portanto nenhum código do produto foi alterado. O servidor fixture foi encerrado, a aba descartável fechada e o helper temporário removido; não restaram listeners da fixture nem alterações aos dois patches do usuário. O relato não substitui o smoke HTTPS anteriormente registrado.

**Próxima ação:** aguardar missão concreta vinculada à TASKLIST; qualquer emissão/grant OAuth real ou teste com ChatGPT Web depende de autorização e configuração operacional específicas. SS-BE-007 continua em andamento; SS-MVP-002 continua parcial e o gate de promoção permanece pendente. PR #1 draft, branch frontend e CI não foram tocados/consultados nesta missão.

## Atualização SS-BE-007 — fatia local verificável

Na ref atual, a comparação dos onze grupos de [`LOCAL_ADMIN_AUTHORIZATION.md`, §8](LOCAL_ADMIN_AUTHORIZATION.md) foi registrada na [TASKLIST](../TASKLIST.md). A lacuna de maior risco selecionada foi o isolamento do listener público: o roteador OAuth real, servido em HTTP loopback, respondia `307` para `OPTIONS /api/admin/v1//session` por normalização do `http.ServeMux`. A fronteira pública agora rejeita caminhos decodificados sob `/api/admin/` antes da normalização, e o teste `TestQuickPublicListenerRejectsAdministrativeMatrix` exige `404` sem `Set-Cookie` para sessão, pareamento, detalhe, decisão, caminho codificado e barras duplicadas.

O resultado desta missão é **SS-BE-007 em andamento**, não validado integralmente. A evidência nova é automatizada e local; não houve CI, túnel, bypass TLS, navegador, grant OAuth, alteração da branch frontend, merge ou deploy. Permanecem pendentes DNS rebinding/proxy real, restart operacional completo, matriz visual/externa e repetição de leitura/revogação no fluxo M3.

### Atualização SS-BE-007 — grupo 2 e grupo 10

Na continuação da fatia local, `internal/admin/transport_test.go` passou a cobrir literalmente `Host: evil.example` com `Forwarded` e `X-Forwarded-Host` apontando para `localhost:7677`; a fronteira rejeita a tentativa em loopback com `403` e o handler não é executado. Não houve alteração de produção.

`cmd/signalspace/quick_panel_restart_test.go` adiciona a composição local de reinício: uma instância cria sessão administrativa e concessão de workspace, o encerramento invalida ambas, e a composição seguinte inicia sem pareamento nem concessão herdada. O teste complementa, sem duplicar, a prova já existente de identidade OAuth persistente e descarte de pedidos/códigos efêmeros.

O mesmo teste agora envia o cookie administrativo antigo por HTTP ao endpoint `/api/admin/v1/session` da nova `Gate` em servidor loopback. A resposta exige `401 AUTH_REQUIRED` e limpa o cookie obsoleto; portanto a verificação não depende somente da `Gate` antiga já encerrada.

Validação da fatia: testes Go locais. Limites: sem túnel, navegador/rede, proxy real ou DNS rebinding; sem restart operacional completo. CI permanece desconhecida e não foi consultada. SS-BE-007 continua em andamento.

### Atualização SS-BE-007 — revisão do bloqueio antes do `ServeMux`

Na revisão local da correção baseada em `b2ca6e4`, a matriz negativa foi ampliada para variantes de `/admin` e `/api/admin/v1/*` com barras duplicadas no início/meio, segmentos `.` e `..`, componentes codificados e métodos alternativos. A reprodução confirmou que a verificação literal anterior deixava quatro famílias chegarem ao `http.ServeMux`, que respondia `307`; não houve evidência de execução de handler administrativo, mas a resposta violava o contrato `404`.

`rejectPublicAdministrativePaths` agora rejeita qualquer segmento exatamente `admin` em `r.URL.Path` antes do mux. O teste `TestQuickPublicListenerRejectsAdministrativeMatrix` exige `404`, sem `Location` e sem `Set-Cookie` para quinze casos administrativos, e verifica que `GET /mcp`, `GET /authorize` e `GET /token` continuam roteados publicamente sem `404` ou redirecionamento. É evidência automatizada em HTTP loopback, não smoke de túnel, navegador ou porta 7677.

SS-BE-007 permanece **em andamento**. CI não foi consultada; o resultado remoto continua desconhecido. Não houve túnel, bypass TLS, grant OAuth, alteração frontend, merge ou deploy. Permanecem pendentes DNS rebinding/proxy real, restart operacional completo, matriz visual/externa e repetição de leitura/revogação no fluxo M3.

### Atualização SS-BE-007 — grupo 2, Host/Origin e cabeçalhos de proxy

Na ref `f585188`, a implementação de `internal/admin/transport.go` foi reaberta e não precisou de alteração: `Host` canônico e `Origin` continuam sendo verificados diretamente, e `Forwarded`/`X-Forwarded-*` não são consultados como autoridade. A cobertura nova está em `internal/admin/transport_test.go`.

`TestAdminTransportRejectsForgedProxyHeadersOverLoopback` envia combinações de Host inválido/público, Origin cruzado, preflight e mutação sem Origin com cabeçalhos de proxy forjados para um servidor HTTP loopback. As rejeições retornam 403, não concedem CORS e não alcançam o handler; Host/origem válidos sem credenciais de transporte continuam alcançando a fronteira. `TestAdminTransportProxyHeadersDoNotBypassSessionOrCSRF` usa o `Gate` real e confirma `403 ACCESS_DENIED` para CSRF incorreto e `401 AUTH_REQUIRED` para sessão forjada, mesmo com proxy forjado.

O resultado é cobertura automatizada local adicional, sem correção de produto. Não é prova operacional de DNS rebinding, proxy real ou host rewrite de implantação. SS-BE-007 permanece **em andamento**; CI não foi consultada e continua desconhecida, sem túnel, bypass TLS, OAuth, frontend, merge ou deploy.

## Checkpoint atual — SS-MVP-006 — 20/09/2026

**Ref observada:** branch `codex/mvp-vertical-programming`, código integrado até `9e26c19` (`1db1ebd` edição local, `d94915c` programação/teste/diff, `9fdf575` UI administrativa, `9e26c19` hardening). Os dois patches não rastreados `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` foram preservados. O checkpoint é derivado e não é lock, autorização de merge ou prova de publicação.

**Resultado:** SS-MVP-003, SS-MVP-004, SS-MVP-005 e SS-MVP-006 estão validadas no escopo local automatizado; SS-MVP-001 está implementada não validada por falta de navegador; SS-MVP-002 permanece em andamento porque não há contrato de escopos de programação remoto/MCP. SS-BE-006 e SS-BE-007 mantêm seus estados anteriores e não foram artificialmente concluídas.

**Evidência:** `internal/workspace/edit_test.go`, `internal/programming/*_test.go`, `internal/admin/ui_test.go`, `node --check internal/admin/ui/admin.js`; teste vertical executa leitura, edição, `go test ./...`, snapshot/diff, revoke e negação. Limites: sem CI, túnel, grant OAuth, navegador, MCP remoto ou sandbox de processo nesta missão.

**Próxima ação vinculada:** SS-MVP-002 — revisar autorização separada por operação antes de qualquer exposição remota de escrita, execução ou Git. A branch frontend `feat/frontend-oauth-consent` e o PR #1 draft permanecem preservados, sem merge/deploy.

## Checkpoint atualizado — SS-MVP-002 — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` em `9067321` antes da nova mudança; a branch foi publicada por push normal e passou a ter upstream em `origin/codex/mvp-vertical-programming`. O pull foi não aplicável antes da publicação porque não havia upstream. Os dois patches não rastreados permanecem preservados.

**Implementação local:** `workspace.Grants.Grant` agora cria concessão somente de leitura. `GrantWithScopes` aceita somente `signalspace:workspace.read` e `signalspace:workspace.write` no processo local; `ReadText`/`ListDirectory` exigem leitura e `ReplaceText` exige escrita, sempre revalidando owner, clientID, sessão e concessão ativa. Revogar ou fechar limpa os escopos.

**Validação da fatia:** testes de workspace/programming/MCP e race de workspace/programming passaram; `cmd/signalspace` passou em execução serial observável. O primeiro comando agregado excedeu a janela de espera e deixou um teste próprio em 7676; os PIDs identificados foram encerrados e não restaram listeners. Não houve túnel, grant OAuth, alteração MCP, navegador, CI, merge ou deploy.

**Estado:** SS-MVP-002 continua **em andamento**: a fronteira local de escrita está validada, mas a integração de escopo remoto OAuth/MCP, execução e Git por operação ainda não existe. Próxima ação: fechar os gates completos e atualizar a autorização remota somente após contrato próprio; não expor `workspace.write` nesta rodada.

## Checkpoint atualizado — SS-MVP-002 MCP isolado — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` após `8fcc7c0` (revisão da composição MCP) e `c9d2a76` (rastreabilidade documental). `git pull --ff-only` retornou `Already up to date` antes da publicação desta rodada; os dois patches não rastreados permanecem preservados.

**Implementação:** `internal/mcp/workspace_write.go` liga identidade verificada, escopo `signalspace:workspace.write`, sessão e `WorkspaceTextWriter.ReplaceText` com resultado estruturado. A injeção é não exportada em `OAuthConfig`, ficando disponível somente dentro do pacote para o harness; `cmd/signalspace` não consegue registrar essa capacidade. `8fcc7c0` tornou a descoberta scope-aware, rejeitou `null` nos argumentos string e reforçou a prova MCP de cliente/owner divergentes. Os modos de produção continuam sem emissor de escrita e sem `replace_text` em `tools/list`.

**Validação:** `gofmt`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, race de `internal/mcp`, `internal/workspace`, `internal/programming` e `cmd/signalspace`, `go vet ./...`, `go build ./...`, `node --check` dos scripts afetados, `node --test internal/auth/consent_js_test.mjs` (8/8), teste vertical e verificação de ausência de listeners/processos residuais passaram. O teste MCP isolado cobre escrita autorizada, read-only/sem escopo, cliente/owner/sessão/workspace divergentes, revogação, tokens inválidos/expirados, path hostil, conflito, duplicação e ausência pública. A implementação foi publicada até `8fcc7c0` e a documentação de revisão até `c9d2a76`; `git ls-remote` confirmou a ref remota em `c9d2a76` nesta etapa.

**Estado:** SS-MVP-002 continua **em andamento/PARCIAL**: a fronteira MCP de teste está confirmada, mas nenhum escopo de escrita foi adicionado ao OAuth emissor, ao Quick ou ao MCP público. Próxima ação: revisar contrato remoto por operação e decidir se/como um gate próprio de publicação será autorizado; não expor `workspace.write` por inferência.

## Checkpoint atualizado — SS-MVP-002 OAuth isolado — 21/09/2026

**Ref observada antes do commit documental:** `codex/mvp-vertical-programming` com o código funcional em `d84d0df`, e `origin/codex/mvp-vertical-programming` em `59dcb1caa776cb73158470bd77f2b8a526b5e07d` antes da publicação desta rodada. Os dois patches não rastreados, a branch `feat/frontend-oauth-consent` e o PR #1 draft foram preservados.

**Implementação:** `auth.Config` agora possui `WriteScope`/`CanIssueWrite` como opt-in experimental; a combinação canônica de escopos mantém `signalspace:diagnostic` obrigatório e rejeita desconhecidos, duplicados e reordenação. A concessão local é revalidada em `/authorize`, `/authorize/complete` e `/token`; o consentimento diferencia leitura de modificação. A configuração padrão e `cmd/signalspace` continuam sem escrita. `workspace.Grants.AllowsClientScope` fornece a verificação exata da capacidade sem expor raiz ou sessão.

**Evidência principal:** commit funcional `d84d0df`; `internal/mcp/oauth_write_integration_test.go` atravessa o emissor OAuth real, registro, PKCE, aprovação local, código, troca por JWT, verificador estático real, `tools/list`, `replace_text`, conteúdo alterado, revogação com JWT ainda válido, código de uso único e handler público sem `replace_text`. `internal/auth/write_scope_test.go` cobre opt-in, metadata, combinações canônicas, consentimento explícito e revogação antes de conclusão/troca.

**Estado:** SS-MVP-002 permanece **PARCIAL/em andamento**. O fluxo OAuth→MCP de escrita está confirmado somente em harness automatizado local e a escrita continua indisponível nos modos públicos/Quick/ChatGPT Web. Navegador real, túnel, grant ao ChatGPT, CI, merge e deploy não foram usados. Execução e Git continuam capacidades separadas. Próxima ação vinculada: definir o gate de promoção remota sem habilitar `workspace.write` por inferência.

## Checkpoint atualizado — SS-MVP-002 gate de promoção — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` em `4cdb878fa349a4c00c9d6c9eb7d290d356266afc`; `git fetch origin --prune` e `git pull --ff-only` retornaram sem divergência. Os dois patches não rastreados e a branch `feat/frontend-oauth-consent` permanecem preservados; PR #1 continua draft e sem merge.

**Decisão:** `SS-MVP-002-PROMOTION-GATE-001` foi registrada em `docs/PROGRAMMING_TOOLS.md`. A promoção futura será por operação, com owner, cliente, escopo, concessão, sessão e workspace vinculados; a primeira composição autorizada deverá exigir escolha explícita terminal-local. Nenhuma variável, token, metadata ou confirmação do ChatGPT ativa escrita por si só. O gate global permanece PENDENTE.

**Evidência executável:** `cmd/signalspace/promotion_gate_test.go` percorre as composições reais de diagnóstico e leitura usadas pelo entrypoint, confirma metadata sem `workspace.write`, `tools/list` sem `replace_text` e rejeição de `tools/call` para a ferramenta ausente. `quickModeArgs` rejeita `write` e a aprovação de workspace do console continua usando `Grants.Grant` read-only.

**Limites:** a cadeia OAuth→MCP do harness já publicada permanece CONFIRMADA somente localmente. Não houve túnel, HTTPS operacional, navegador/grant real, workspace real, CI, merge ou deploy. Execução e Git continuam capacidades separadas. Próxima ação vinculada: implementar, somente após autorização específica, a composição remota futura e seu aceite operacional; não publicar `workspace.write` neste gate.

## Checkpoint atualizado — SS-MVP-002 lifecycle local — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` após `f5dd8cf` (`test(mcp): verify OAuth write grant lifecycle across recreation`). O worktree mantém somente os dois patches não rastreados previamente preservados; nenhuma alteração foi feita na branch frontend ou no PR #1 draft.

**Evidência:** `TestOAuthWriteLocalCompositionLifecycleDropsPriorGrant` usa o emissor OAuth, PKCE, JWT assinado, `NewStaticJWTVerifier` e handler MCP reais. A edição funciona antes do fechamento; após fechar servidor, `Grants` e identidade, uma nova composição reabre a chave/clientes persistidos, verifica o JWT antigo, mas nega o token com sessão/grant antigos sem modificar o arquivo. Uma concessão e sessão novas autorizam a operação compatível; a sessão antiga continua negada. O recurso fechado também rejeita `ReplaceText` com `workspace.ErrClosed`.

**Estado e limites:** lifecycle da composição local **CONFIRMADO localmente**; restart operacional remoto/HTTPS, navegador, túnel, grant ao ChatGPT, ativação pública de `workspace.write`, CI, merge e deploy permanecem não validados ou fora do escopo. O gate global `SS-MVP-002-PROMOTION-GATE-001` continua PENDENTE. Próxima ação vinculada: preservar a composição pública sem escrita e selecionar nova fatia somente após instrução do ChatGPT Web.

## Checkpoint atualizado — SS-MVP-001 integração visual local — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` após `79a1a37038a202e6e96804d623d7354959925b69`; `origin/feat/frontend-oauth-consent` confirmado separadamente em `3911c4eae2ec514ff5f30d904a993cf103f9be32`. A branch frontend, o PR #1 draft e os dois patches não rastreados foram preservados. A cópia do alinhamento frontend consultada foi a ref `3911c4e`; a adaptação operacional do SignalSpace continua v2.0.

**Implementação:** `internal/admin/ui/index.html`, `admin.css` e `admin.js` incorporam a hierarquia de pareamento/desbloqueio e a apresentação de pedido dos protótipos selecionados, sem carregar seus cenários fictícios. A UI usa somente as rotas administrativas existentes, mostra apenas campos do `RequestSnapshot`, mantém o nome como declarado/não atestado, não expõe caminho local, preserva CSRF apenas em memória e não usa Web Storage. `internal/admin/ui.go` embute CSS same-origin; `internal/admin/transport.go` permite somente esse recurso e declara `style-src 'self'` sem relaxar script/connect.

**Evidência:** `internal/admin/ui/admin_js_test.mjs` executou 18 testes comportamentais: estados iniciais, pair/unlock, fila/detalhe, expected version, duplo envio, reconciliação após perda/409, limpeza em 401, 429/503/rede, resposta antiga após lock, ausência de refresh automático, texto malicioso e distinção 404/410. O mecanismo de geração/cancelamento invalida respostas de requests após mudança de sessão. Go serial/race, vet, build, HTTP/CSP, `node --check`, consentimento Node 8/8 e isolamento público passaram.

**Aceite visual em navegador:** **CONFIRMADO localmente em escopo descartável** via `http://localhost:7677`, fixture controlada, sem túnel, HTTPS, workspace ou grant real: pareamento, lista, detalhe, aprovação confirmada pelo servidor, foco no detalhe, lock e limpeza da fila foram observados. A matriz visual de erros negativos, viewport dedicado pequeno e fluxo operacional Quick/ChatGPT Web permanecem pendentes; não há evidência para declarar M4 completo.

**Estado:** SS-MVP-001 **validada no escopo local**; integração da branch frontend permanece **PENDENTE** e não foi feita. SS-MVP-002 continua **em andamento/PARCIAL**, sem `workspace.write` público, Quick write, exec/shell/Git remoto ou alteração de capacidades. CI não foi consultada e permanece **DESCONHECIDA**. Próxima ação concreta: receber a próxima missão e, se ela exigir aceite visual negativo/viewport dedicado, repetir apenas essa matriz com fixture descartável.

## Checkpoint atualizado — SS-MVP-001 / M4 aceite visual residual — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` após a correção de estado visual desta missão; `origin/codex/mvp-vertical-programming` foi sincronizada antes da alteração e será confirmada após o commit. `origin/feat/frontend-oauth-consent` permanece em `3911c4eae2ec514ff5f30d904a993cf103f9be32`. Os dois patches não rastreados permanecem preservados.

**Implementação:** mudança mínima em `internal/admin/ui/admin.js`: a troca de geração/sessão limpa operações de decisão pendentes e bloqueios herdados. Regressão correspondente em `internal/admin/ui/admin_js_test.mjs`: **19/19**.

**Aceite visual local:** fixture descartável somente em `http://localhost:7677`, sem túnel, HTTPS, workspace ou grant real. Foram observados 401, STALE_REQUEST, WORKSPACE_GRANT_REQUIRED, 503, falha de rede, 404 e 410; nenhum cenário produziu aprovação otimista ou fila vazia indevida. Firefox headless gerou screenshots locais de `390×844` e `320×844` para o pareamento, sem recorte observado; o navegador integrado confirmou textos longos na fila, o detalhe recebeu foco e Tab levou a `Recusar`. A fixture foi encerrada e removida do worktree; não restaram listeners/processos.

**Reconciliação frontend:** pareamento, desbloqueio, lista, detalhe, permissões/escopos, decisões, erros/reconciliação, responsividade e acessibilidade estão **incorporados funcionalmente com adaptação ao backend real**. Os protótipos da branch `feat/frontend-oauth-consent` continuam demonstrativos e preservados; não há lacuna funcional que justifique copiar ou fazer merge. Integração formal de branches permanece **PENDENTE**.

**Estado:** SS-MVP-001 permanece **validada no escopo local**; M4 permanece **PARCIAL** por depender de aceite operacional Quick/ChatGPT Web/HTTPS e de decisão formal sobre branches. SS-MVP-002 permanece **PARCIAL/em andamento** e o gate `SS-MVP-002-PROMOTION-GATE-001` não foi aprovado. CI permanece **DESCONHECIDA** e não foi consultada. Próxima ação: nova missão concreta vinculada à TASKLIST; não repetir a matriz visual já concluída.

## Checkpoint atualizado — SS-MVP-005 — revisão Git MCP isolada — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming`, após sincronização fast-forward com `origin` em `d19caca60eb9102505dd6f34f857077fe58efd9e`; a branch `feat/frontend-oauth-consent` continua em `3911c4eae2ec514ff5f30d904a993cf103f9be32`. Os dois patches não rastreados foram preservados. O checkpoint é derivado e não é lock, autorização de merge ou prova de publicação.

**Implementação funcional:** `review_git_changes` foi adicionado somente à composição MCP de teste por porta não exportada. O tool aceita apenas `session_id`, anuncia-se apenas para token verificado com `signalspace:git.review`, revalida owner/client/session/grant Git e retorna baseline, estado atual, mudanças e completude. A concessão Git é distinta de leitura, edição e execução; não há registro no `cmd/signalspace`, Quick ou transporte público.

**Hardening observado:** `CaptureGitSnapshot` mantém `status`/`diff` read-only, desativa fsmonitor/untracked cache/optional locks, usa `--no-ext-diff`/`--no-textconv` e remove variáveis de configuração, atributos, índice/objetos alternativos e executores auxiliares do ambiente filho. A porta `WithAuthorizedGitProcessDir` não expõe raiz ou `Session` e serializa a observação com edição/revogação.

**Validação automatizada:** `internal/mcp/git_review_integration_test.go` prova fixture Git → baseline → token/escopo de edição separado → `replace_text` → revisão real → status/diff com `M editable.txt` e `-before/+after` → revogação → negação, além de token ausente/inválido, escopos read/write sem Git, divergência de owner/client/session e argumentos extras. `TestGitSnapshotNeutralizesExecutableConfigAndEnvironment` confirma que tentativas de fsmonitor/diff externo por configuração e ambiente não executam helper; o teste adicional cobre truncamento, cancelamento e diretório sem repositório. Os gates Go completos da missão passaram.

**Estado e limites:** SS-MVP-005 está **validada no escopo local automatizado**, e SS-MVP-002 continua **PARCIAL/em andamento**: não houve Git remoto, escrita pública, execução pública, Quick Tunnel, HTTPS operacional, navegador, grant ao ChatGPT, CI, merge ou deploy. M4 e integração da branch frontend permanecem pendentes. Próxima ação: executar gates locais completos, publicar a branch e relatar; depois aguardar nova missão sem promover a ferramenta.

## Checkpoint atualizado — SS-MVP-004 — execução de testes MCP isolada — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` após `03ebf71` (`942c649` implementação e `03ebf71` negativos). O worktree mantém somente os dois patches não rastreados previamente preservados; a branch frontend e o PR #1 draft não foram tocados.

**Implementação funcional:** `workspace.ScopeTest` e `WithAuthorizedTestProcessDir` formam a porta local estreita para `signalspace:test.run`. `internal/mcp/test_runner.go` adiciona `run_workspace_tests` somente por `OAuthConfig.testRunner`, campo não exportado; o tool recebe apenas `session_id`, revalida JWT/owner/client/session/concessão e executa o comando fixo `go test ./...`. A entrada pública, o Quick e o emissor OAuth continuam sem essa porta e sem emissão do escopo.

**Estados e limites:** o resultado estruturado distingue `TEST_PASSED`, `TEST_FAILED`, `TIMED_OUT`, `CANCELED` e `UNKNOWN`, inclui código/saída limitados, truncamento e término observado. `terminated` significa somente que o processo principal foi observado por `cmd.Wait`; descendentes não são atestados separadamente. `GOTOOLCHAIN=local`, `GOPROXY=off` e `GOSUMDB=off` reduzem dependências externas da fixture, mas a execução continua com privilégios do usuário e não é sandbox, read-only ou idempotente.

**Evidência:** `internal/mcp/test_runner_integration_test.go` percorre fixture Git temporária → edição MCP → teste aprovado → revisão Git → teste deliberadamente falhando → revogação/negação. Também cobre timeout, token ausente/inválido, owner/client/session divergentes, tokens read/write/Git sem `test.run`, concessão sem `ScopeTest`, argumentos extras/null/tipo inválido e contador de processos para negativas pré-execução. A regressão pública mantém `tools/list`/`tools/call` sem `run_workspace_tests`; `internal/auth/write_scope_test.go` confirma que o emissor padrão não anuncia nem aceita `signalspace:test.run`.

**Validação:** `gofmt`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, race nos pacotes afetados, `go vet ./...`, `go build ./...` e a regressão pública de não exposição passaram após os commits funcionais. CI permanece **DESCONHECIDA** e não foi consultada; não houve túnel, HTTPS operacional, navegador, grant ao ChatGPT, workspace real, merge, deploy ou force-push. Não restaram processos/listeners de teste em `7676`/`7677` nem `cloudflared`.

**Estado:** SS-MVP-004 está **validada no escopo local automatizado e na composição MCP isolada**; SS-MVP-006 possui a vertical MCP correspondente validada. SS-MVP-005 permanece isolada; SS-MVP-002 continua **PARCIAL/em andamento**, com `workspace.write`, `test.run`, Git e execução não publicados. Próxima ação: enviar o relatório e aguardar missão concreta nova, sem reabrir a matriz visual ou promover ferramentas ao transporte público.

## Checkpoint atualizado — SS-MVP-002/004/006 — OAuth de programação isolado — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` em `2065480` (`feat(auth): integrate isolated OAuth programming scopes`), após sincronização anterior com `origin`; `origin/feat/frontend-oauth-consent` permanece em `3911c4eae2ec514ff5f30d904a993cf103f9be32`. Os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` continuam não rastreados e preservados com os hashes registrados anteriormente. O checkpoint é derivado e não autoriza merge, publicação ou promoção de capacidades.

**Implementação funcional:** `auth.Config` agora permite, somente por opt-in experimental, `GitScope`/`CanIssueGit` e `TestScope`/`CanIssueTest`, ao lado de `WriteScope`/`CanIssueWrite`. O emissor anuncia e aceita apenas escopos explicitamente configurados, na ordem canônica iniciada por `signalspace:diagnostic`, revalidando cada capacidade em `/authorize`, `/authorize/complete` e `/token`. O consentimento distingue teste com privilégios do usuário e workspace sem sandbox de Git read-only, diff potencialmente sensível e ausência de commit/push. A configuração padrão e `cmd/signalspace` continuam sem esses campos.

**Evidência OAuth→MCP:** `internal/mcp/oauth_programming_integration_test.go` usa DCR, PKCE, decisão local via `DecideTerminal`, código de autorização, três trocas `/token` separadas e JWT emitido pelo servidor verificado por `NewStaticJWTVerifier`. Com fixture Git/Go descartável, tokens de escrita/Git/teste percorrem `replace_text` → `run_workspace_tests` → `review_git_changes`; claims de issuer/audience/owner/client/scope são conferidos sem registrar tokens. O cenário nega authorize sem grant e não cria pedido pendente, confirma código one-time, revoga a concessão mantendo JWTs criptograficamente válidos, impede nova edição/teste/diff e verifica que nenhum processo/observação começa após revoke. A composição pública mantém somente diagnóstico e rejeita as três ferramentas.

**Validação:** `gofmt -l cmd internal`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, `go test -race ./cmd/signalspace ./internal/auth ./internal/mcp ./internal/programming ./internal/workspace`, `go vet ./...`, `go build ./...` e `TestPublicCompositionsKeepWorkspaceWriteUnpublished` passaram. Não houve alteração JavaScript; os gates Node previamente válidos permanecem aplicáveis. Não restaram listeners/processos de teste em `7676`/`7677` ou `cloudflared`. CI permanece **DESCONHECIDA** e não foi consultada; não houve túnel, HTTPS operacional, navegador real, grant ao ChatGPT Web, workspace real, merge, deploy ou force-push.

**Estado:** SS-MVP-004 e SS-MVP-006 estão **validadas no escopo local automatizado e na composição OAuth/MCP isolada**; SS-MVP-005 permanece **validada somente no harness local**. SS-MVP-002 continua **PARCIAL/em andamento**: escrita, execução e Git não foram publicados no Quick, entrypoint ou transporte público; o gate `SS-MVP-002-PROMOTION-GATE-001` permanece PENDENTE. Próxima ação: enviar o relatório ao ChatGPT Web e aguardar a próxima missão concreta, sem reabrir a matriz visual nem promover ferramentas.

## Checkpoint atualizado — SS-MVP-002 — aprovação terminal-local experimental — 21/09/2026

**Ref observada:** `codex/mvp-vertical-programming` em `d2c72e9` (`feat(workspace): add explicit programming capability approval`), após `git fetch origin --prune`/`git pull --ff-only`; os dois patches não rastreados permanecem preservados. A branch `feat/frontend-oauth-consent` continua em `3911c4eae2ec514ff5f30d904a993cf103f9be32`. O checkpoint continua derivado e não autoriza a ativação remota.

**Implementação funcional:** `internal/workspace/capability_approval.go` adiciona `CapabilityApproval`, `NormalizeCapabilities` e `ValidateApprovedRoot`. O componente aceita somente cliente OAuth elegível, raiz canônica e capacidades independentes conhecidas; mantém um pedido temporário de dois minutos; exibe a seleção via `workspaceConsole` somente quando essa dependência é explicitamente injetada; e chama `GrantWithScopes` apenas após confirmação de uso único. Cancelamento, expiração, cliente que deixa de ser elegível, symlink, raiz inválida e encerramento não criam concessão. O comando legado `workspace request` permanece read-only.

**Evidência vertical:** `internal/mcp/oauth_programming_integration_test.go` primeiro emite OAuth diagnóstico para tornar o cliente elegível na fonte confiável, usa `CapabilityApproval.Request`/`Confirm` para produzir a concessão de escrita/teste/Git e então percorre OAuth → edição → teste → revisão Git → revogação. Não há chamada direta da fixture a `GrantWithScopes` para a concessão principal. `cmd/signalspace/workspace_console_test.go` comprova o resumo legível, caminho com espaços, confirmação incorreta, capacidades exatas, revogação e indisponibilidade na composição padrão.

**Validação:** `gofmt -l cmd internal`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, `go test -race ./cmd/signalspace ./internal/auth ./internal/mcp ./internal/programming ./internal/workspace -parallel=1 -count=1 -timeout=240s`, `go vet ./...` e `go build ./...` passaram. Não foram alterados arquivos JavaScript. Não restaram listeners em `7676`/`7677` nem processo `cloudflared`; CI não foi consultada e permanece **DESCONHECIDA**.

**Estado:** aprovação terminal-local experimental e concessão de capacidades estão **CONFIRMADAS no harness/console explicitamente composto**; ativação operacional, promoção remota, navegador/HTTPS, grant ao ChatGPT Web, workspace real, merge e deploy permanecem **PENDENTES/BLOQUEADOS**. SS-MVP-002 continua **PARCIAL/em andamento**; SS-MVP-006 continua validada no harness OAuth/MCP. Próxima ação: enviar o relatório ao ChatGPT Web e aguardar a próxima missão concreta vinculada à TASKLIST.

## Checkpoint atualizado — SS-MVP-002 — composição fail-closed — 21/09/2026

**Ref observada antes da alteração:** branch `codex/mvp-vertical-programming`, HEAD local e remoto `140f0b6a042181ba368e024b3d9a5987a1df2477`. A árvore inicial continha apenas os dois patches não rastreados preservados, com os hashes registrados na sessão anterior. A implementação foi registrada em `e1c3cb0aa686148e0d0ab526fc978c9a3e49c0bc`; o push normal concluiu e `git ls-remote` confirmou esse SHA na branch remota naquele momento. A consulta CI não foi feita.

**Implementação:** `compositionMode` admite somente `diagnostic` e `read`; o plano canônico associa os escopos OAuth e a configuração do console. O entrypoint `connect quick` rejeita sintaxe/programação não suportada; `panel` não altera modo/capacidades. Quick valida o plano antes de ambiente, confirmação, portas, túnel e estado OAuth. O construtor do handler rejeita enumeração desconhecida ou plano inconsistente antes de criar emissor/grants. A leitura mantém apenas `signalspace:workspace.read`, `read_file` e `list_directory` junto ao diagnóstico. Escrita, execução e Git seguem ausentes do entrypoint e do MCP público.

**Validação local:** `go test ./cmd/signalspace -count=1 -timeout=180s`; `go test ./... -parallel=1 -count=1 -timeout=180s`; `go test -race ./cmd/signalspace ./internal/auth ./internal/mcp ./internal/programming ./internal/workspace -parallel=1 -count=1 -timeout=240s`; `go vet ./...`; `go build ./...`; e `git diff --check` passaram. Os testes exigem listas exatas de escopos/ferramentas e verificam que modos inválidos não leem confirmação, chamam túnel/verificador/fábrica do painel nem criam estado OAuth.

**Estado e limites:** a composição local fail-closed está **CONFIRMADA no commit `e1c3cb0aa686148e0d0ab526fc978c9a3e49c0bc`**, que foi enviado normalmente; a confirmação remota daquele SHA foi observada. SS-MVP-002 continua **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` continua **PENDENTE**. Não houve túnel real, HTTPS operacional, navegador, grant ao ChatGPT Web, workspace real, consulta CI, merge ou deploy. PR #1 continua draft e `feat/frontend-oauth-consent` permanece intocada. Próxima ação vinculada à TASKLIST: aguardar a missão concreta seguinte; a promoção segue sujeita a um gate próprio.

## Checkpoint atualizado — SS-MVP-006 — OAuth/MCP read-first — 21/09/2026

**Ref observada:** branch `codex/mvp-vertical-programming`, implementação em `2d80caa34105a39cbcf7964a9917fc1ea06895ec`. O worktree preserva sem alteração os patches não rastreados `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch`. O PR #1 permanece draft e a branch `feat/frontend-oauth-consent` não foi tocada.

**Implementação e fluxo:** somente o harness de `internal/mcp/oauth_programming_integration_test.go` foi ampliado. A configuração opt-in do emissor liga `ReadScope`/`CanIssueRead` a `workspace.ScopeRead`; `CapabilityApproval` pede leitura, escrita, Git e teste e confirma a ordem canônica exata. O baseline Git é capturado antes de editar. DCR, PKCE, decisão terminal e tokens independentes do emissor real/verificador percorrem `read_file` na fixture; o conteúdo observado `before\n` é passado diretamente como `expected` a `replace_text`; depois o teste retorna `TEST_PASSED`, o reviewer observa `+after`, e revoke nega leitura/escrita/teste/Git embora os JWTs sigam criptograficamente válidos.

**Negativas e isolamento:** tokens somente de escrita, Git ou teste não leem; token somente de leitura não escreve, executa teste ou revisa Git; sessão divergente não lê. Pedido pendente não cria grant nem permite leitura/edição MCP ou emissão de escopo OAuth. A leitura revogada falha sem retornar conteúdo. Handler público continua somente diagnóstico; os modos `diagnostic` e `read` do entrypoint não mudaram e sua regressão focal passou. Cobertura existente em `internal/mcp/workspace_read_test.go` também exige escopo de leitura sem concessão para negar a operação.

**Validação local no commit de implementação:** teste vertical OAuth/MCP focado; focados de `TestReadScopeRequiresLocalGrantAndSeparateOAuthConsent`, `TestReadScopeDisabledByDefaultAndRejectsMisconfiguration`, testes MCP de leitura e `TestPublicCompositionsKeepWorkspaceWriteUnpublished`; `go test ./... -parallel=1 -count=1 -timeout=180s`; `go test -race ./cmd/signalspace ./internal/auth ./internal/mcp ./internal/programming ./internal/workspace -parallel=1 -count=1 -timeout=240s`; `go vet ./...`; `go build ./...`; `gofmt` e `git diff --check` — todos PASS.

**Estado e limites:** SS-MVP-006 está **validada no escopo local automatizado**. SS-MVP-002 continua **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**. Nenhum código de produção, Quick, entrypoint, modo read público, frontend ou JavaScript foi alterado. CI não foi consultada; não houve túnel, HTTPS operacional, navegador/grant ao ChatGPT Web, workspace real, merge ou deploy. Próxima ação: publicar o relatório desta missão ao ChatGPT Web e buscar a próxima missão vinculada à TASKLIST; não promover as capacidades.

## Reconciliação — SS-MVP-002 — plano de composição explícito — 22/09/2026

**Ref e divergência:** branch `codex/mvp-vertical-programming`, base local/remota `bd1ceec6dd27dde9a4db4f903768cff2be1fe6d3`. O prompt recebido citava `140f0b6`, mas `TASKLIST.md`, histórico e `docs/PROJECT_STATE.md` já registravam a implementação `e1c3cb0` como ancestral da base atual. A implementação não foi repetida: foi ampliada apenas onde a nova missão ainda exigia informação operacional explícita.

**Implementado em `2662d1e`:** o plano canônico agora seleciona o verificador JWT OAuth local e o único endereço MCP permitido, `admin.PublicAddress` (`127.0.0.1:7676`); a reserva real do Quick usa esse endereço depois de validar o plano. A porta administrativa `7677` permanece opção local separada e não entra no handler MCP. Diagnóstico/read continuam sendo os únicos modos; escrita, teste e Git não são selecionáveis.

**Validação local:** testes focados de composição, Quick, não exposição pública e a vertical OAuth/MCP passaram; `gofmt -l cmd internal`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, race nos cinco pacotes afetados, `go vet ./...` e `go build ./...` passaram. O teste de composição comprova rejeição de plano sem verificador ou com MCP apontando para a porta administrativa antes de criar estado OAuth. CI não foi consultada.

**Estado e limites:** SS-MVP-002 permanece **PARCIAL/em andamento** e o gate `SS-MVP-002-PROMOTION-GATE-001` **PENDENTE**. Não houve túnel real, HTTPS operacional, grant/navegador ChatGPT Web, workspace real, merge ou deploy. Os dois patches não rastreados permanecem preservados. Próxima ação: sincronizar e publicar commits normais na mesma branch; não ativar capacidades públicas.

## Continuidade — SS-MVP-002 — consulta e revogação local — 22/09/2026

**Ref reconciliada:** branch `codex/mvp-vertical-programming`, base local/remota observada antes desta fatia `aff5a2df8cffa3c312c4ed30f8dd46a468bfa6b9`; o prompt da missão citava uma base anterior. A alteração de implementação está em [`8fc43f5`](https://github.com/LuigiAPCPereira/SignalSpace/commit/8fc43f5), sucessora da base verificada. Os patches locais não rastreados foram preservados.

**Implementação:** `workspace.Grants.Snapshot` retorna somente ativo/ausente, ID da sessão, ID do cliente e cópia independente dos escopos na ordem read/write/Git/test; ausência e `ErrClosed` são distintos. Não inclui raiz, descritor nem `Session`. O console stdin local implementa `workspace status` e `workspace revoke current`, mantendo `workspace revoke <session-id>`. O revoke-current consulta o snapshot e usa a revogação exata existente; ID obsoleto falha fechado se houver substituição, sem revogar a nova concessão. Nome de cliente ausente na lista confiável não é tratado como revogação. Nenhuma rota HTTP, ferramenta MCP, UI administrativa ou composição pública mudou.

**Validação local no commit de implementação:** testes focados de `Grants` e console, `go test ./internal/workspace ./cmd/signalspace -count=1 -timeout=180s`, `gofmt -l` nos quatro arquivos alterados, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, `go test -race ./cmd/signalspace ./internal/auth ./internal/mcp ./internal/programming ./internal/workspace -parallel=1 -count=1 -timeout=240s`, `go vet ./...` e `go build ./...` — PASS. Substituição entre snapshot e revoke foi simulada deterministicamente; ID antigo foi rejeitado e a nova concessão permaneceu ativa. Sem listeners 7676/7677 ou processo `cloudflared` após os testes.

**Estado e limites:** SS-MVP-002 continua **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` continua **PENDENTE**. CI não foi consultada; nenhum túnel/HTTPS real, navegador ou grant ao ChatGPT Web, workspace real, merge ou deploy foi usado. Os dois patches preservam os hashes registrados no log desta execução. Próxima ação: enviar este relatório ao ChatGPT Web e seguir somente a próxima missão concreta vinculada à TASKLIST; não promover capacidades públicas.

## Atualização — SS-BE-007 grupo 7 — leitura OAuth, painel e revogação — 22/09/2026

**Implementação:** foi adicionado somente o teste `cmd/signalspace/read_panel_live_integration_test.go`. Ele usa a composição READ canônica e os listeners reais `127.0.0.1:7676`/`localhost:7677`, com transporte HTTP loopback, DCR/PKCE, OAuth diagnóstico para elegibilidade, concessão terminal-local read-only, fila/detalhe/pareamento/CSRF/versão no painel, aprovação administrativa separada da emissão, conclusão pública e troca única do código por token.

**Evidência observada:** a fila e o detalhe mostram cliente, escopo READ, versão e `ACTIVE` sem raiz local; a resposta de decisão não contém código/token; cookies administrativos não autenticam MCP e bearer MCP não autentica o painel; `tools/list` é exatamente diagnóstico + leitura + listagem; `read_file` e `list_directory` retornam somente a fixture controlada; depois de `workspace revoke current`, o mesmo JWT continua criptograficamente verificável, mas ambas as ferramentas negam sem conteúdo, o detalhe reconcilia para `COMPLETED`/`REVOKED` e uma nova autorização READ é recusada sem pedido adicional.

**Validação e estado:** o teste focalizado, `go test ./... -parallel=1 -count=1 -timeout=180s`, race dos pacotes afetados, `go vet ./...`, `go build ./...`, `gofmt -l cmd internal` e `git diff --check` passaram. O shutdown é gracioso, os objetos temporários são fechados e as duas portas foram reabertas com sucesso após o teste. SS-BE-007 continua **em andamento**; SS-MVP-002 continua **PARCIAL/em andamento** e o gate `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**. Não houve túnel, HTTPS operacional, navegador/grant real, workspace real, CI, merge ou deploy.

## Checkpoint SS-BE-007 grupo 4 — recuperação após resposta perdida — 22/09/2026

**Estado:** SS-BE-007 grupo 4 **CONFIRMADO no escopo local integrado**; SS-BE-007 global **PARCIAL/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref e integração:** a branch `codex/mvp-vertical-programming` partiu do HEAD confirmado `33c35e41ea78e7e5dcf0e0bd5fb80d5577ccecae`. O commit de implementação/testes é `b5baa99` (`test(admin): reconcile pair and unlock after lost responses`); o commit documental desta atualização será o próximo commit da branch. O pull fast-forward foi executado antes da implementação; o upstream local não estava configurado, então a ref remota foi consultada explicitamente.

**Evidência:** `internal/admin/http_response_loss_test.go` usa um `http.Server` real em `127.0.0.1:7677`, `Host: localhost:7677`, `Gate` e handlers administrativos reais. Pair e Unlock são confirmados no servidor antes de uma falha de transporte controlada. Quatro cenários distinguem resposta inteira perdida de corpo JSON perdido com `Set-Cookie` recebido. O GET `/api/admin/v1/session` não infere autenticação: sem cookie reconcilia `LOCKED`; com cookie recebido confirma `AUTHENTICATED`, e a rota de pedidos é acessível. Cada mutação teve exatamente uma tentativa. `internal/admin/ui/admin_js_test.mjs` verifica a mesma regra no cliente com rede perdida e JSON truncado: um GET de reconciliação, nenhum replay.

**Validação e limites:** teste integrado focalizado e pacote `internal/admin` passaram, assim como `node --check internal/admin/ui/admin.js` e os 21 testes Node. A falha foi simulada no cliente depois da conclusão do handler; isso não é queda de rede externa nem prova de navegador, túnel, HTTPS ou recuperação operacional. Não houve defeito de produção, alteração de runtime, CI, grant ao ChatGPT Web, workspace real, merge ou deploy. Shutdown e rebind de `7677` passaram. Os dois patches não rastreados permanecem preservados.

**Próxima ação:** aguardar do orquestrador uma nova missão concreta vinculada a outro grupo aberto da `SS-BE-007`, sem reabrir SS-BE-006 ou o grupo 4 e sem promover capacidades públicas.

## Checkpoint SS-BE-007 grupo 5 — expiração administrativa via HTTP loopback — 22/09/2026

**Estado:** SS-BE-007 grupo 5 **CONFIRMADO no escopo local automatizado**; SS-BE-007 global **PARCIAL/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref e preservação:** a branch `codex/mvp-vertical-programming` foi reaberta em `8924eb3037fd56c99bf2c6fd7eafc4afcf80f975`; `git fetch origin --prune` e `git pull --ff-only origin codex/mvp-vertical-programming` não trouxeram divergência. O teste novo é `internal/admin/http_session_expiry_test.go`. Os patches não rastreados permaneceram intocados. Recalculados nesta retomada: `signalspace-oauth-read-scope.patch` = `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b`; `signalspace-workspace-client-binding.patch` = `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Implementação/teste:** o teste inicia `NewServer(gate.HandlerWithRequests(fixture))` em listener TCP loopback efêmero, força `Host: localhost:7677` e `Origin` canônica, usa cookie jar/CSRF reais e avança o `Gate.now` de forma controlada, sem sleep ou relógio novo em produção. No cenário de idle, GET de sessão e fila preservam `idle_expires_at`; no deadline, sessão/fila/decisão/refresh retornam `401 AUTH_REQUIRED`, sem ID da fila no erro, sem decisão aplicada e sem reativação: a sessão posterior é `LOCKED`. No cenário absoluto, refresh explícito válido é repetido antes do vencimento, refresh com CSRF inválido retorna `403`, o idle é limitado ao absoluto e, no deadline absoluto, as operações retornam 401 sem leak ou mutação.

**Validação:** `TestAdminHTTPSessionExpiryAndAbsoluteRefresh`, `gofmt -l cmd internal`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, race dos seis pacotes afetados, `go vet ./...`, `go build ./...`, `node --check internal/admin/ui/admin.js` e `node --test internal/admin/ui/admin_js_test.mjs` passaram. O teste exercita somente HTTP loopback local; não prova navegador, túnel/cloudflared, HTTPS operacional, grant ChatGPT Web, CI ou workspace real.

**Correção de relatório anterior:** o grupo 4 registrou textualmente um hash incorreto para o segundo patch; a divergência é apenas documental. O arquivo preservado continua com o hash `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`; nenhum patch foi editado.

**Publicação e próxima ação:** o commit `a523f7912fe279f6afb3ab949a5380a7b214332e` foi enviado normalmente para `origin/codex/mvp-vertical-programming`; `git ls-remote` e o remote-tracking local confirmaram o mesmo SHA, sem divergência de diff. Enviar o relatório ao ChatGPT Web e aguardar a próxima missão concreta sem promover capacidades públicas.

## Checkpoint SS-BE-007 grupo 9 — tombstone OAuth real via HTTP — 22/09/2026

**Estado:** a fatia de tombstone público real do grupo 9 está **CONFIRMADA no escopo local**; o grupo 9 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**. SS-MVP-002 continua **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` continua **PENDENTE**.

**Ref e preservação:** a branch `codex/mvp-vertical-programming` foi revalidada em `857d465daccb36e0c1fd8f2078478b9bf1cac21c`; fetch/prune e pull fast-forward não trouxeram alterações. Os patches seguem fora do Git: `signalspace-oauth-read-scope.patch` = `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b`; `signalspace-workspace-client-binding.patch` = `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Implementação e evidência:** `internal/auth/public_status_expiry_http_test.go` usa `auth.New` real, registro `/register`, pedido `/authorize`, cookie OAuth e `request_id` reais, além de `PublicStatusHandler` e `Server.Handler` em listener TCP efêmero. O teste fixa somente os prazos da fixture sob `Server.mu`, sem clock de produção. O status HTTP observado percorre PENDING → EXPIRED retido → EXPIRED estável → 404 `not_found`. `expires_at` e `retainUntil` não mudam por polling; conclusão tardia não redireciona; a decisão real retorna expiração enquanto retida e not found após limpeza; `codes` e clientes emitidos permanecem sem efeito.

**Separação de evidência:** o 410/404 da API administrativa continua sendo a cobertura existente com fonte controlada, conforme exigido pelo prompt. Este checkpoint confirma o tombstone do emissor verdadeiro via HTTP público, não uma integração administrativa 410/404 com a mesma instância.

**Validação e limites:** o teste novo, regressões de lifecycle/public status/admin, suíte Go completa serial, race de auth/admin/cmd, vet, build, gofmt e diff-check passaram. Não houve mudança de produção, JavaScript, CI, túnel, navegador/ChatGPT Web, HTTPS operacional, workspace real, merge ou deploy. Próxima ação: publicar o relatório e aguardar outro ID aberto da TASKLIST; não fechar o grupo 9 inteiro nem promover capacidades públicas.

## Checkpoint SS-BE-007 grupo 10 — reinício HTTP sem recuperar autorizações — 22/09/2026

**Estado:** a fatia de reinício HTTP local está **CONFIRMADA**; o grupo 10 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**. SS-MVP-002 continua **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref e preservação:** a branch `codex/mvp-vertical-programming` partiu de `78b76d1ff279a6ff1b0698603211ad60f13fe8e1`; fetch/prune e pull fast-forward confirmaram a base. Os patches permanecem fora do Git: `signalspace-oauth-read-scope.patch` = `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b`; `signalspace-workspace-client-binding.patch` = `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Implementação e evidência:** `cmd/signalspace/quick_panel_oauth_restart_http_test.go` usa duas instâncias novas da composição `diagnostic`, o mesmo `StateDir`, sockets HTTP reais nos listeners público/admin, `auth.Server`, `Gate`, DCR, PKCE, cookies reais e API administrativa real. A primeira instância cria A pendente e B; aprova B por HTTP, conclui B e guarda o código sem trocá-lo. Shutdown e rebind dos dois listeners passam. A segunda instância preserva issuer, `kid` JWKS e `client_id`, mas começa UNPAIRED; o cookie admin antigo dá 401, A dá 404, conclusão tardia dá 403 e o código B dá 400 `invalid_grant` duas vezes. C é um novo pedido PENDING, com fila contendo somente C após novo pareamento.

**Fronteiras e limites:** o público não expõe `/api/admin`, o admin não expõe `/mcp` e a consulta pública não autenticada a `tools/list` retorna 401 sem ferramentas de escrita/execução; a lista diagnóstica exata continua coberta pela regressão de composição. Pedidos, códigos, sessão admin e aprovações transitórias não foram restaurados. Esta é evidência local/loopback; não valida processo externo, túnel, HTTPS operacional, navegador, grant ChatGPT Web, workspace real, CI, merge ou deploy. Não houve alteração de produção nem do contrato.

**Validação:** teste focalizado e regressões de identidade/restart/composição passaram; `gofmt -l cmd internal`, `git diff --check`, suíte Go completa serial, race de `cmd/signalspace`, `internal/admin`, `internal/auth`, `internal/mcp` e `internal/workspace`, `go vet ./...` e `go build ./...` passaram. Próxima ação: publicar o relatório ao ChatGPT Web e aguardar ID novo da TASKLIST, sem reabrir SS-BE-006 ou os grupos 4, 5, 7 e 9 e sem promover capacidades públicas.

## Checkpoint SS-BE-007 grupo 3 — fluxo visual local de pareamento, lock e unlock — 22/09/2026

**Estado:** esta fatia do grupo 3 está **CONFIRMADA no escopo visual local descartável**; SS-BE-007 global permanece **PARCIAL/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref, implementação e preservação:** a missão partiu do HEAD `07bd85fcee8b34c53effcdcc55ce6d7070144161` na branch `codex/mvp-vertical-programming`. A correção concreta está em `internal/admin/ui/admin.js` e o teste de regressão em `internal/admin/ui/admin_js_test.mjs`; o commit de código é `0efd64a` (`fix(admin): preserve locked UI after rejected unlock`). Os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` permaneceram fora do Git, sem alteração, com hashes `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Evidência observada:** uma fixture temporária compôs `admin.NewGate`, `Gate.HandlerWithRequests`, `admin.NewServer` e os arquivos reais da UI em `http://localhost:7677/`, no navegador integrado do Codex. O fluxo real observado foi `UNPAIRED` → Pair `201` → `AUTHENTICATED` com fila; Lock `200` → `LOCKED` sem fila e com formulário; Unlock inválido `403` → `LOCKED`, mensagem de rejeição, formulário preservado e um GET de sessão `200`; Unlock válido `200` → `AUTHENTICATED` com fila; reload autenticado preservando a fila; novo Lock `200` e reload `LOCKED`; fila sem sessão `401 AUTH_REQUIRED`. O fixture foi encerrado e removido, sem registrar pairing code, passphrase, cookie ou credencial.

**Defeito e correção:** a rejeição conhecida de desbloqueio era renderizada como `UNAVAILABLE`, escondendo o formulário e descartando o bootstrap. A UI agora trata somente 403/429 de `/api/admin/v1/unlock` com reconciliação por GET de sessão, mantendo `LOCKED` e permitindo nova tentativa; o POST não é repetido. O teste Node comprova uma única chamada de desbloqueio e a recuperação do bootstrap.

**Validação e limites:** passaram os 22 testes Node, `node --check internal/admin/ui/admin.js`, `gofmt -l cmd internal`, `git diff --check`, suíte Go serial completa, race dos pacotes afetados, `go vet ./...` e `go build ./...`. Não foi observado erro visível de JavaScript na navegação. A evidência é local e descartável: não valida HTTPS operacional, túnel/cloudflared, grant real do ChatGPT Web, workspace, CI ou publicação externa. Rate limit/concurrency e grupos restantes não são declarados por este checkpoint. Após a limpeza não restaram listeners em 7676/7677 nem processo `cloudflared`. Próxima ação: publicar o relatório e aguardar o próximo ID aberto da TASKLIST, sem reabrir SS-BE-006 ou os grupos 4, 5, 7, 9 e 10.

## Checkpoint SS-BE-007 grupo 6 — decisões concorrentes sobre solicitações OAuth reais — 22/09/2026

**Estado:** a fatia de concorrência HTTP-versus-terminal está **CONFIRMADA no escopo HTTP local**; SS-BE-007 global permanece **PARCIAL/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref, teste e preservação:** a missão partiu do HEAD `1e12e888a9d350ad0c29f6d79305a5af653484cf` na branch `codex/mvp-vertical-programming`. `cmd/signalspace/oauth_concurrent_decisions_http_test.go` adiciona `TestOAuthRequestsRemainIndependentAcrossConcurrentHTTPAndTerminalDecisions`; o commit de teste é `6976343ca3aa3b90de71720703285b96bd699322`. Os patches continuam fora do Git, sem alteração, com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Evidência HTTP real:** duas composições OAuth/Gate canônicas foram usadas dentro da mesma fixture, com DCR real para clientes distintos A/B, pedidos criados por `/authorize`, cookies/CSRF reais e tráfego pelos listeners loopback `127.0.0.1:7676` e `127.0.0.1:7677`. A fila administrativa autenticada continha exatamente os dois pedidos PENDING, com client IDs, escopo, redirect URI, versão e `verified=false` corretos; os detalhes individuais confirmaram a mesma fonte.

**Isolamento e concorrência:** payload HTTP de A contendo metadados extras de B foi rejeitado com `400 INVALID_REQUEST`, sem modificar nenhum snapshot. `expected_version=2` em A recebeu `409 STALE_REQUEST`. Uma barreira controlada iniciou simultaneamente approve HTTP e deny via `Authorization.DecideTerminal`; exatamente uma operação venceu e a outra recebeu `ALREADY_DECIDED`. A terminou versão 2 com `decided_at` e status coerente; B permaneceu PENDING/versão 1 durante a corrida. A decisão posterior de B produziu DENIED/versão 2 sem contaminar A. Nenhuma decisão gerou código ou token; `IssuedClients()` permaneceu vazio.

**Validação e limites:** passaram o teste focalizado e seu race, `gofmt -l cmd internal`, `git diff --check`, suíte Go serial, race dos cinco pacotes pedidos, `go vet ./...` e `go build ./...`. O shutdown foi concluído e o teste confirmou que 7676/7677 estavam livres para rebind. Esta é prova local por HTTP real, não valida túnel/cloudflared, HTTPS operacional, navegador, ataque externo, workspace, CI, merge ou deploy; não houve alteração de produção, frontend, scopes, MCP público ou composição. Próxima ação: publicar o relatório e aguardar nova missão vinculada a ID existente da TASKLIST, sem declarar SS-BE-007 global concluída.

## Checkpoint SS-BE-007 grupo 9 — saturação da fila e quota pública OAuth — 22/09/2026

**Estado:** a fatia de saturação/quota está **CONFIRMADA no escopo HTTP local**; o grupo 9 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref, teste e preservação:** a base observada foi `380969e94ac70120b332d37dd879edf53e9c663f` na branch `codex/mvp-vertical-programming`; `internal/auth/quota_http_test.go` foi publicado no commit `6f1ce2d673dccdd1d56df45b2023757dfb4036ab`. Os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` continuam não rastreados, sem alteração, com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Evidência observada:** o primeiro teste cria 32 pedidos válidos por `/authorize` através de HTTP TCP real, verifica cookies/IDs próprios, escopo diagnóstico e PENDING, e observa o 33º como `503 temporarily_unavailable` sem novo cookie, pedido ou credencial. A decisão terminal negativa isolada deixa a vaga ocupada; `/authorize/complete` com cookie/CSRF reais retorna `403` sem redirect/código e libera exatamente a vaga encerrada. Um novo `/authorize` ocupa essa vaga sem contaminar os 31 pedidos restantes.

O segundo teste faz 120 GETs válidos de `/authorize/status` para o mesmo cookie/request, verifica `200 PENDING`, `Cache-Control: no-store` e `expires_at` estável, recebe `429 slow_down` com `Retry-After` no excesso, retrocede somente o início da quota sob o mutex de teste e confirma um novo `200` que não renova nem decide o pedido. `codes` e `issued` permanecem vazios e quotas de outras rotas não mudam.

**Validação e limites:** os testes focados e seus races, suíte Go serial completa, race dos cinco pacotes, `gofmt`, diff-check, vet e build passaram. O teste usa somente `httptest.NewServer`/loopback efêmero, sem produção modificada. Não comprova túnel/cloudflared, HTTPS operacional, navegador/grant ChatGPT Web, workspace real, CI, merge ou deploy. Próxima ação: publicar este relatório no ChatGPT Web e aguardar o próximo ID da TASKLIST, sem reabrir as fatias já confirmadas.

## Checkpoint SS-BE-007 grupo 8 — isolamento de sessões OAuth por HTTP — 22/09/2026

**Estado:** a fatia de isolamento e conclusão legítima está **CONFIRMADA no escopo HTTP local**; o grupo 8 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref, teste e preservação:** a base observada foi `a456334fbe4e7842c81327085a3daf9fbf34cdd3` na branch `codex/mvp-vertical-programming`; `internal/auth/session_isolation_http_test.go` foi publicado no commit `6e929e9068888515b73d6145f9a25e008a826119`. Os patches permanecem não rastreados e sem alteração: `signalspace-oauth-read-scope.patch` = `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b`; `signalspace-workspace-client-binding.patch` = `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Evidência HTTP real:** dois clientes DCR (`Fixture A` e `Fixture B`) criaram pedidos reais por `/authorize`, com IDs, cookies Secure e CSRF distintos. Consultas próprias retornaram PENDING com `no-store` e `expires_at` estável; consultas cruzadas e ausência de cookie retornaram 403. A aprovação terminal de A alterou somente A para APPROVED; B permaneceu PENDING e o cookie B continuou impedido de consultar A.

As conclusões com cookie B, cookie ausente e CSRF B foram rejeitadas sem `Location`/code e sem consumir A. A conclusão legítima de A retornou 303 para o callback registrado, preservou `state`/`iss`, criou exatamente um código e marcou A COMPLETED; B permaneceu PENDING. O replay foi rejeitado sem segundo código. Nenhum token ou cliente `issued` foi criado. O código não foi trocado e o callback não foi seguido.

**Validação e limites:** passaram teste focado/race focado, regressões de status/conclusão/expiração, suíte Go serial, `gofmt`, diff-check, vet, build e races dos pacotes/seleção pertinente. A execução ampla de `go test -race` encontrou uma falha externa à fatia em `cmd/signalspace/quick_retry_test.go:81`: o cenário de contexto recebeu `lookup : no such host` em vez do erro esperado de cancelamento. Essa falha é registrada como **não validada/parcial**, sem correção especulativa. O cookie Secure foi reapresentado manualmente pelo harness HTTP; não há prova de navegador/HTTPS. Não houve túnel, callback externo seguido, token, workspace, CI, merge ou deploy. Próxima ação: publicar o relatório e aguardar nova missão por ID existente, sem declarar SS-BE-007 global concluída.

## Checkpoint SS-BE-007 grupo 11 — retry DNS do Quick e cancelamento — 22/09/2026

**Estado:** a fatia de cancelamento durante retry DNS está **CONFIRMADA no escopo automatizado local**; SS-BE-007 global permanece **PARCIAL/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref e publicação:** a missão partiu de `f1f74d0d14a2897a329e214b3367653b8f59409c` em `codex/mvp-vertical-programming`. Após `git fetch origin --prune`, fetch explícito da ref, `git pull --ff-only` e push normal, o commit de código/teste `5f67b0d` foi confirmado em `origin/codex/mvp-vertical-programming`, sem divergência. Os patches não rastreados foram preservados sem alteração: `signalspace-oauth-read-scope.patch` = `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b`; `signalspace-workspace-client-binding.patch` = `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`.

**Causa observada:** o teste original passou uma vez, mas a repetição limitada com `-race -count=100` falhou 4/100 no subteste `context`, retornando `public HTTPS verification failed after DNS retry window, tunnel closed: lookup : no such host` em vez de `context canceled`. Em `awaitQuickTransport`, `readyCtx` é filho de `ctx`; ao cancelar o chamador, ambos os sinais podiam estar prontos e o `select` podia classificar o DNS residual como deadline. A hipótese foi reproduzida e confirmada pelo código e pela frequência observada; não foi tratada como data race.

**Correção e regressão:** `cmd/signalspace/quick.go` agora consulta `ctx.Err()` no ramo de `readyCtx.Done()` e retorna a causa explícita quando há cancelamento. `cmd/signalspace/quick_retry_test.go` mantém o verificador ativo por barreira, cancela durante a verificação e só então libera o erro DNS transitório. Nenhuma janela foi alongada, TLS foi desabilitado ou caminho de erro permanente foi alterado.

**Validação:** os quatro testes de retry passaram em serial e em `-race -count=100`; `go test ./... -count=1 -timeout=300s`, `go test -race ./cmd/signalspace ./internal/admin ./internal/auth ./internal/mcp ./internal/workspace -p=1 -parallel=1 -count=1 -timeout=300s`, `go vet ./...`, `go build ./...`, `gofmt` e `git diff --check` passaram. Não houve CI, cloudflared, túnel, HTTPS operacional, navegador, OAuth externo, workspace real, merge ou deploy. Não restaram listeners em 7676/7677 nem processo `cloudflared`.

**Próxima ação:** nenhum novo ID foi criado; aguardar a próxima missão vinculada a ID existente. Este checkpoint não conclui SS-BE-007 global nem aprova o gate de promoção.

## Checkpoint SS-BE-007 grupo 10 — restart visual local no navegador integrado — 22/09/2026

**Estado:** a fatia de restart/bind/publicação fechada está **CONFIRMADA no escopo HTTP + navegador integrado local descartável**. O grupo 10 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**; SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref e preservação:** o código foi reaberto na branch `codex/mvp-vertical-programming`, HEAD `e652d72`, sem aplicar os dois patches não rastreados do usuário. A fixture manual foi criada temporariamente no pacote `cmd/signalspace`, removida após o `PASS`, e não deixou mudança de produção, frontend ou CI.

**Evidência observada:** duas composições `diagnostic` reais usaram o mesmo `StateDir` temporário, `auth.Server`/`admin.Gate` novos, handlers reais e listeners fixos `127.0.0.1:7676`/`localhost:7677`. No mesmo contexto e mesma aba do navegador integrado, o primeiro ciclo fez pareamento e exibiu uma fila com um pedido OAuth `PENDING`; o detalhe mostrou metadados e `Recusar`/`Aprovar`, sem clique de decisão. O primeiro shutdown foi gracioso e o rebind das portas passou.

**Após reinício:** reload da mesma aba apresentou `UNPAIRED` e a mensagem de sessão administrativa inválida; a fila, o detalhe e as ações antigas não foram exibidos. O segundo pareamento apresentou `AUTHENTICATED`, contador `0` e “Nenhuma solicitação encontrada no servidor”. A captura visual final confirmou a distinção entre sessão autenticada e fila vazia.

**Validação e limites:** a fixture terminou `PASS` em 59,67 s; o teste HTTP focalizado de restart também passou. Não houve defeito reproduzível. Não houve cloudflared, túnel, HTTPS operacional, grant ChatGPT Web, workspace real, CI, merge ou deploy; esta evidência não é aceite externo.

**Próxima ação:** aguardar nova missão vinculada a ID existente, sem declarar SS-BE-007 global concluída e sem reabrir a fatia já confirmada.

## Checkpoint SS-BE-007 grupo 9 — integração administrativa com OAuth real bloqueada — 22/09/2026

**Estado:** a fatia de tombstone público real permanece **CONFIRMADA no escopo local**; a nova integração administrativa com a mesma instância OAuth está **BLOQUEADA** no gate de viabilidade do tempo. O grupo 9 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**. SS-MVP-002 permanece **PARCIAL/em andamento** e `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**.

**Ref e preservação:** branch `codex/mvp-vertical-programming`, HEAD observado `2e86de81193e8acd25b045ab1957d8c27dbc9a59`, após fetch/prune e pull fast-forward sem divergência. Os patches não rastreados foram preservados fora do Git, sem aplicação ou alteração, com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780ebf8a444df1a4e48f2`.

**Gate de viabilidade:** o `auth.Server` usa `time.Now()` ao criar pedidos e ao consultar/limpar snapshots, e mantém `pending`/`terminal` privados. O controle de tempo dos testes existentes só é possível dentro do pacote `auth`; a composição da API administrativa está no pacote `admin`, que importa `auth`. A prova pública real e a prova administrativa controlada passaram separadamente, mas não podem ser apresentadas como a integração pedida. Esperar minutos reais ou ampliar a produção com clock/endpoint de teste não é uma alternativa autorizada nesta missão.

**Validação:** passaram `TestPublicStatusRetainsRealExpiredRequestOverHTTP`, `TestAdminHTTPExpiredDecisionAndRetentionBoundary`, regressões focadas de auth/admin e os 22 testes Node do painel. Não foi criado teste integrado substituto, não houve código de produção, listener, túnel, cloudflared, navegador, HTTPS, CI, merge ou deploy nesta fatia.

**Próxima ação vinculada:** resolver o bloqueio de controle determinístico de tempo com decisão explícita de contrato/teste; até lá, não declarar `410`/`404` administrativos como provenientes da mesma instância OAuth e não repetir grupos já confirmados.

## Checkpoint SS-BE-007 grupo 9 — integração administrativa OAuth real confirmada localmente — 22/09/2026

**Estado:** a fatia administrativa com a mesma instância `auth.Server` está **CONFIRMADA no escopo HTTP local**, usando ponte temporal exclusiva da build tag `signalspace_testtime`. Grupo 9 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**; SS-MVP-002 permanece **PARCIAL/em andamento**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI é **DESCONHECIDA**.

**Ref e preservação:** branch `codex/mvp-vertical-programming`, base `8b697c8ec6aabe57ccf8f37ae51e66a941901e2e`; implementação/teste em `312ef64`. Os dois patches do usuário continuam não rastreados, fora do commit e intocados, com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780ebf8a444df1a4e48f2`.

**Evidência:** o teste `TestAdminOAuthRealExpiryLifecycleWithTestTimeBridge` atravessa DCR, `/authorize`, `auth.Server`, `admin.Gate`, listeners reais de loopback e pareamento HTTP. A transição e limpeza foram disparadas pelos GET administrativos reais, não pela ponte: `PENDING` → `EXPIRED` retido → `410 REQUEST_EXPIRED` → `404 REQUEST_NOT_FOUND`; a fila sem sessão respondeu `401 AUTH_REQUIRED`. A ponte somente ajustou `pending[id].Expires` e, depois de um tombstone verdadeiro, `terminal[id].retainUntil` sob o mutex.

**Fronteira e validação:** `go list` mostrou que a ponte não aparece sem tag e aparece com `-tags=signalspace_testtime`. A suíte tagged de `cmd/signalspace`, foco tagged com race, suíte Go normal serial, race serial dos seis pacotes afetados, vet, build, gofmt, diff-check e regressões Node 22/22 passaram. Nenhuma configuração de distribuição habilita a tag; não houve mudança do comportamento temporal normal.

**Limites e próxima ação:** evidência somente local/loopback; não comprova túnel, HTTPS, navegador, grant ChatGPT Web, workspace real, CI, merge ou deploy. O próximo trabalho deve usar um ID existente da TASKLIST e não promover SS-BE-007 global nem SS-MVP-002.

## Checkpoint SS-BE-007 grupo 9 — falha 503/rede observada no painel — 22/09/2026

**Estado:** a fatia de indisponibilidade do painel com fila OAuth real está **CONFIRMADA no escopo visual + HTTP local com falha injetada**. Grupo 9 completo e SS-BE-007 global permanecem **PARCIAIS/em andamento**; SS-MVP-002 permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI é **DESCONHECIDA**.

**Fixture e evidência:** em `http://localhost:7677/`, uma única composição local descartável criou DCR e `/authorize` reais, uma fila com exatamente um PENDING e pareamento HTTP no navegador integrado. Com o detalhe do pedido aberto, um wrapper temporário interceptou apenas GET `/api/admin/v1/requests` e produziu primeiro o envelope vigente `503 TEMPORARILY_UNAVAILABLE`, depois uma conexão encerrada. A UI manteve `AUTHENTICATED`, mostrou respectivamente `O serviço está temporariamente indisponível (503)` e `A conexão com o serviço local foi perdida.`, manteve contador `n/d`, não mostrou fila vazia, não confirmou decisão e ocultou o detalhe não reconfirmado.

**Recuperação:** após remover as interceptações, uma nova consulta real voltou a mostrar contador `1` e o mesmo pedido `PENDING`. A ação de consulta foi o botão explícito `Renovar sessão`; não houve renovação implícita durante as falhas. Nenhum botão de decisão foi tocado e nenhum POST `/decision` foi emitido; a fonte OAuth permaneceu PENDING.

**Validação e limites:** a fixture terminou PASS em 100,9 s, o navegador e os servidores foram encerrados e não restaram listeners fixos nem `cloudflared`. Passaram regressões focadas, suíte Go serial, race serial dos seis pacotes afetados, vet, build, gofmt, diff-check, `node --check` e 22/22 testes Node. A injeção demonstra o contrato da UI e a recuperação local, não uma queda externa, proxy, Cloudflare, túnel ou HTTPS; não houve defeito nem alteração de produção.

**Próxima ação:** aguardar missão vinculada a SS-BE-007; não declarar o grupo 9 completo, SS-BE-007 global ou o gate de promoção como concluídos.

## Checkpoint SS-BE-007 — roteiro de aceite operacional preparado, smoke não executado — 22/09/2026

**Estado:** matriz dos 11 grupos reconciliada documentalmente; `SS-BE-007` permanece **PARCIAL**, `SS-MVP-002` permanece **PARCIAL**, `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**, CI permanece **DESCONHECIDA** e o aceite operacional novo está **NÃO EXECUTADO**.

**Ref e PR:** branch observada `codex/mvp-vertical-programming`, HEAD local/remoto `e14c8588991338f3f9e8c1e999644f9e2fa8a34b`. O PR #1 continua draft e sua HEAD/branch é `feat/m1-local-mcp-diagnostic`; não é a branch atual e não foi integrado. Os dois patches não rastreados foram preservados, não aplicados e não incluídos.

**Reconciliação da matriz:** grupos 3 e 5 agora refletem a evidência visual local descartável de pareamento/unlock/restart; grupo 9 reflete a evidência visual + HTTP local de fila PENDING real durante 503 e falha de rede, além de quotas, retenção e tombstones. Nenhuma dessas classificações foi elevada a evidência externa.

**Roteiro preparado em [`docs/QUICK_PANEL.md`](QUICK_PANEL.md):** preflight de ref/portas/processos e modo `diagnostic panel`; confirmação de origem fixa `127.0.0.1:7676` e painel local `127.0.0.1:7677`; negativas administrativas pela URL pública; Host/Origin e proxy no admin local; pareamento/decisão descartável; diagnóstico MCP somente com autorização específica; shutdown e verificação de limpeza. Critérios de parada cobrem publicação de 7677, origem incorreta, bind, resposta administrativa pública, segredo em log e falha de limpeza.

**Autorização e limite:** nesta missão não foram iniciados SignalSpace, cloudflared ou túnel, nem consultada CI, aberto workspace, concedido OAuth ao ChatGPT Web, feito merge ou deploy. A autorização permanente para enviar relatórios ao Web não autoriza publicação externa, grant/decisão OAuth, workspace ou chamada MCP.

**Próxima ação:** somente após autorização específica para o smoke, executar o roteiro com evidência redigida; até lá, não iniciar túnel e não promover `SS-BE-007`.

## Checkpoint SS-BE-007 — smoke real de túnel e isolamento — 22/09/2026

**Estado:** a fatia autorizada de isolamento do Quick Tunnel está **CONFIRMADA operacionalmente no escopo descartável**; `SS-BE-007` permanece **PARCIAL**, `SS-MVP-002` permanece **PARCIAL**, `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE** e CI permanece **DESCONHECIDA**.

**Ref e preservação:** branch `codex/mvp-vertical-programming`, HEAD local/remoto `f14c92afe82bf6505090321135c74a895e69b605`. PR #1 continua draft em outra branch e não foi alterado. Os dois patches do usuário continuam não rastreados, não aplicados e intocados, com hashes preservados.

**Evidência observada:** preflight sem processos residuais, portas 7676/7677 livres, `cloudflared 2026.9.1`, origem efetiva `http://127.0.0.1:7676` e admin `127.0.0.1:7677`. Após autorização exclusiva, `go run ./cmd/signalspace connect quick panel` iniciou um único túnel. Dez sondagens públicas sem cookies e sem seguir redirects passaram: nove rotas/variantes administrativas retornaram `404` sem `Location`, `Set-Cookie` ou corpo administrativo; `/.well-known/oauth-protected-resource` retornou `200` JSON. O HTTPS foi validado pelo cliente TLS padrão, sem ignorar certificado.

**Isolamento local e shutdown:** `Host` externo, `Origin` cruzada e preflight cruzado retornaram `403`; `Forwarded`/`X-Forwarded-*` forjados não contornaram a autorização da rota protegida (`401`). O bootstrap com Host canônico retornou `200`, sem registrar cookie ou corpo. Não houve pareamento, OAuth, decisão, MCP ou operação de workspace. Ctrl+C encerrou o processo; 7676/7677 ficaram livres e não restaram processos SignalSpace/cloudflared do smoke. O diretório temporário do Quick e os temporários de probe foram removidos.

**Limites:** confirma somente o encaminhamento público para 7676, os negativos administrativos e o isolamento/Host/Origin observado neste ambiente. Não cobre proxy externo arbitrário, DNS rebinding, configuração externa deliberada, pareamento/decisão, navegador, MCP, CI, merge ou deploy. O túnel não ficou ativo.

## Checkpoint SS-MVP-002 — auditoria da fronteira pública — 22/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL**; `SS-BE-007` permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI permanece **DESCONHECIDA**. A auditoria não promoveu `workspace.write`, `test.run` ou `git.review`.

**Ref e preservação:** a auditoria começou na branch `codex/mvp-vertical-programming`, HEAD local/remoto `a2dd93b6ae3be89bd3871cada8c406ed139ac014`. O PR #1 e `feat/m1-local-mcp-diagnostic` não foram alterados. `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` permaneceram não rastreados, não aplicados e intocados.

**Fronteira auditada:** a composição aceita somente `diagnostic` e `read`; o painel não altera o modo; modos inválidos são rejeitados antes de OAuth, listener, túnel ou efeito; o admin não é MCP; o público usa o listener de 7676 e não encaminha para 7677; `tools/list` não publica escrita, teste ou Git; e grants locais revalidam owner, client, sessão, escopo e revogação. A evidência foi conferida em `cmd/signalspace/{composition.go,main.go,quick.go}` e `internal/mcp/{oauth.go,server.go,workspace_read.go}`, com os testes de composição, promoção, OAuth, revogação e ferramentas.

**Lacuna e correção:** uma composição de leitura anunciava `read_file` para bearer que possuía somente `signalspace:diagnostic`. A chamada de leitura já falhava com desafio de escopo, mas a descoberta indevida violava a separação de escopos. O novo teste reproduziu a exposição; `readToolAccess.advertise` agora só é verdadeiro após verificar `signalspace:workspace.read`, e `tools/list` usa essa condição. As expectativas de integração para tokens de escrita, Git e teste foram alinhadas: cada um anuncia apenas diagnóstico e sua capacidade própria.

**Validação:** passaram a suíte de `internal/mcp`, a suíte serial de `cmd/signalspace`, o race de `internal/mcp`, `go vet` dos pacotes afetados, `go build ./...`, `gofmt` e `git diff --check`. A primeira execução de `cmd/signalspace` falhou somente por expectativas antigas de descoberta; após a reconciliação dos testes, passou. Nenhum serviço, túnel ou cloudflared foi iniciado nesta missão.

**Limites e próxima ação:** trata-se de evidência local de código/testes, não de matriz operacional completa, navegador, CI, grant externo, workspace, merge ou deploy. Não declarar o gate de promoção concluído; aguardar próxima missão vinculada a ID existente.

## Checkpoint SS-MVP-002 — regressão pública READ — 23/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL**; `SS-BE-007` permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI permanece **DESCONHECIDA**.

**Ref e preservação:** base local/remota revalidada em `codex/mvp-vertical-programming`, HEAD `ac5b6ee47c50df71512f8ce1d9eca56e875dc95d`. PR #1, `feat/m1-local-mcp-diagnostic` e frontend não foram alterados. Os dois patches não rastreados permaneceram fora do Git, não aplicados e intocados.

**Evidência pública:** `TestPublicCompositionsKeepWorkspaceWriteUnpublished` usa a mesma composição `read`, DCR/OAuth e grant local descartável. Antes do grant, token somente diagnóstico lista apenas `connection_diagnostic`; depois do grant, o mesmo token mantém essa lista. Um token com `signalspace:workspace.read` lista exatamente `connection_diagnostic`, `read_file` e `list_directory`. O modo `diagnostic` rejeita solicitação adicional de leitura e sua lista permanece somente diagnóstico. A ausência de write/test/Git é preservada pela lista exata e pela rejeição de `workspace.write`; o equivalente de escopo adicional contra handler sem reader permanece coberto pelo teste MCP isolado existente.

**Validação:** focos públicos/MCP/READ/painel/vertical OAuth, suíte `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, race serial dos pacotes afetados `cmd/signalspace` e `internal/mcp`, `go vet ./...`, `go build ./...`, `gofmt` e `git diff --check` passaram. Não houve mudança de produção, túnel/cloudflared, workspace real, grant ChatGPT Web, consulta CI, merge ou deploy.

**Limites e próxima ação:** a regressão fecha a cobertura do entrypoint público para o fix de descoberta, mas não constitui aceite remoto/HTTPS nem promoção de ferramentas. Aguardar missão vinculada a ID existente.

## Checkpoint SS-MVP-002 — instruções MCP alinhadas ao escopo OAuth — 23/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL**; `SS-BE-007` permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI permanece **DESCONHECIDA**. Nenhuma ferramenta de escrita, execução ou Git foi promovida.

**Ref e preservação:** a auditoria começou no HEAD local/remoto `0a774ca2a2b9f0e260b5a68f354bab393d8448ec` e a implementação foi publicada no commit `5f6f2371c7e22047af1dbb767b2f5faf1a5917e6`. PR #1, `feat/m1-local-mcp-diagnostic` e `feat/frontend-oauth-consent` não foram alterados. Os dois patches do usuário permaneceram não rastreados, não aplicados e intocados.

**Fronteira auditada:** CLI → composição → Quick → OAuth → verificador JWT → handler MCP → ferramentas. `composition.go`, `main.go` e `quick.go` continuam fechando os entrypoints em `diagnostic`/`read`; o painel não altera capacidades; o público continua em 7676 e o admin em 7677; `tools/list` público não publica write/test/Git; owner, cliente, sessão, escopo e revogação continuam revalidados na chamada.

**Lacuna reproduzida e corrigida:** `initialize.instructions` usava a existência de `WorkspaceReader`/outras portas internas, e não o escopo do token atual. Com um JWT somente diagnóstico numa composição configurada para leitura, `tools/list` retornava apenas `connection_diagnostic`, mas `initialize` dizia que a leitura de arquivos estava disponível. A regressão focal falhou antes da correção. O servidor agora inclui instruções de leitura, Git ou teste somente quando o respectivo `advertise` foi habilitado pela verificação independente do escopo; a composição pública testa token diagnóstico e token READ sem nova fixture.

**Reconciliação documental:** a matriz resumida do grupo 1 permanece alinhada ao checkpoint `a2dd93b6` e registra nove variantes externas aprovadas no smoke descartável. Isso é evidência de isolamento da fatia exercitada, não aceite da matriz integral de ataques, proxy/DNS rebinding, navegador, CI ou deploy; o histórico anterior não foi reescrito.

**Validação:** passaram o foco da regressão antes/depois, foco público `TestPublicCompositionsKeepWorkspaceWriteUnpublished`, `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, `go test -race ./cmd/signalspace ./internal/mcp -p=1 -parallel=1 -count=1`, `go vet ./...`, `go build ./...`, `gofmt` e `git diff --check`. Não houve listeners residuais em 7676/7677 nem processo `cloudflared` após os testes.

**Limites e próxima ação:** a prova permanece local/automatizada. Não houve túnel/cloudflared, HTTPS operacional, navegador, grant ChatGPT Web, workspace real, consulta CI, merge ou deploy. Aguardar missão vinculada a ID existente sem promover capacidades.

## Checkpoint SS-MVP-002 / SS-BE-007 — auditoria de prontidão da fronteira pública — 23/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL**; `SS-BE-007` permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI permanece **DESCONHECIDA**. Nenhuma ferramenta de escrita, execução ou Git foi promovida.

**Ref e preservação:** a missão informou `a2dd93b6ae3be89bd3871cada8c406ed139ac014` como base, mas a ref efetiva local/remota foi revalidada em `codex/mvp-vertical-programming`, HEAD `47878563c360b6b84d6ce17c8121ea6b491fc912`; não houve rewind. PR #1, `feat/m1-local-mcp-diagnostic` e `feat/frontend-oauth-consent` não foram alterados. Os dois patches não rastreados continuam não aplicados, fora do Git e intocados.

**Fronteira auditada:** `CLI → composição → Quick → OAuth → verificador JWT → handler MCP → ferramentas`. A composição fechada aceita apenas `diagnostic` e `read`; `panel` não muda capacidades; plano inválido falha antes de OAuth, listener, túnel e efeitos; o admin é separado do MCP público; o público é ligado a `127.0.0.1:7676`; `127.0.0.1:7677` permanece administrativo e não há encaminhamento entre os listeners. Testes existentes mantêm `tools/list` público sem `workspace.write`, `test.run` ou `git.review`, com listas exatas por escopo.

**Autorização:** a evidência em OAuth/MCP/workspace confirma que escopo OAuth independente não substitui concessão local: owner, cliente, sessão, escopo e revogação são revalidados na operação. JWT válido após revogação não recupera a autorização. As lacunas de descoberta e de `initialize` já corrigidas foram reabertas na revisão; não surgiu defeito negativo novo demonstrável e nenhuma produção foi alterada nesta missão.

**Validação:** passaram os focos de composição, rejeição de plano, isolamento dos listeners, matriz pública administrativa, promoção READ, ferramentas MCP de leitura/escrita/listagem/Git, escopos OAuth e proteção admin. `git diff --check` passou após esta atualização documental. Não houve SignalSpace, cloudflared, túnel, navegador, grant ChatGPT Web, workspace real, CI, merge ou deploy.

**Reconciliação do grupo 1:** o checkpoint `a2dd93b6` registra nove variantes externas do smoke descartável, incluindo isolamento/metadata/shutdown e negativos administrativos; essa evidência é restrita à fatia exercitada e não comprova a matriz integral de ataques, proxy/DNS rebinding, navegador, CI ou deploy.

**Próxima ação:** aguardar missão vinculada a ID existente, preservando os estados acima e sem promover capacidades públicas.

## Checkpoint SS-MVP-002 — matriz combinada OAuth/MCP e step-up — 23/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL**; `SS-BE-007` permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` permanece **PENDENTE**; CI permanece **DESCONHECIDA**. Nenhuma capacidade pública foi promovida.

**Ref e preservação:** a missão referenciou `47878563c360b6b84d6ce17c8121ea6b491fc912`; a ref efetiva local/remota foi revalidada em `codex/mvp-vertical-programming`, HEAD `23c35b6c7510fe36748b430f1f092b22e6f3a219`. PR #1 e as branches protegidas não foram alterados. Os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` continuam não rastreados, não aplicados e intocados, com SHA-256 preservados.

**Matriz observada:** no harness real OAuth/MCP, metadata lista apenas os escopos configurados pela composição; `initialize` e `tools/list` anunciam somente as capacidades cujo bearer atual passou na verificação independente; `tools/call` revalida o escopo e depois a concessão local. Composição pública diagnostic continua somente `connection_diagnostic`; READ mantém step-up para `workspace.read` sem revelar workspace, sessão, path ou conteúdo; bearer READ sem grant não executa. Os tokens read/write/Git/test do harness demonstraram simetria de escopo independente e chamadas sem escopo não iniciaram writer, runner ou reviewer.

**Defeito e correção:** o bearer `diagnostic + workspace.write` recebia instrução “Diagnostic only” apesar de `replace_text` estar anunciado. O teste combinado de `metadata → initialize → tools/list → tools/call` falhou antes da correção; `internal/mcp/server.go` agora inclui a orientação de escrita somente quando `writeAccess.advertise` é verdadeiro. A correção não torna escrita selecionável no CLI/Quick nem altera o contrato público.

**Validação e limites:** passaram o teste focado antes/depois, as suítes seriais de `internal/mcp` e `cmd/signalspace`, race serial dos dois pacotes, `go vet ./...`, `go build ./...`, `gofmt` e `git diff --check`. Não houve túnel/cloudflared, HTTPS operacional, navegador, grant ChatGPT Web, workspace real, CI, merge ou deploy; listeners 7676/7677 não ficaram ativos.

**Próxima decisão:** **PROMOTION GATE PRONTO PARA DECISÃO DO PROPRIETÁRIO, NÃO APROVADO AUTOMATICAMENTE.** A evidência local não autoriza promoção; aguardar decisão vinculada a `SS-MVP-002-PROMOTION-GATE-001`.

## Checkpoint SS-MVP-002 — promoção local opt-in READ + WRITE + GIT — 23/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL**; `SS-BE-007` permanece **PARCIAL**; `SS-MVP-002-PROMOTION-GATE-001` está **APROVADO PELO PROPRIETÁRIO / IMPLEMENTAÇÃO LOCAL CONFIRMADA**; CI permanece **DESCONHECIDA**. A aprovação cobre somente READ, WRITE e revisão Git observacional no modo explícito `connect quick programming`; shell, `test.run`, comandos arbitrários e mutações Git não foram promovidos.

**Ref e preservação:** antes da implementação, `codex/mvp-vertical-programming` e `origin/codex/mvp-vertical-programming` estavam em `61863d6a6b1d50d34d1a73f989ea97a18360e8cd`. O código foi registrado em `e97aaf7ec84577f2999f0bb725db5d2b8a451ce2` e a documentação do gate em `d1bff2c925e83061bcbc15cef299b22846ac897b`; ambos foram publicados por push normal. O HEAD local e `origin/codex/mvp-vertical-programming` agora coincidem em `d1bff2c925e83061bcbc15cef299b22846ac897b`. PR #1, `feat/m1-local-mcp-diagnostic`, `feat/frontend-oauth-consent` e os dois patches não rastreados permanecem preservados, fora do Git, não aplicados e intocados.

**Implementação:** `cmd/signalspace` agora tem composição fechada `diagnostic`/`read`/`programming`; `internal/mcp` expõe o construtor explícito `NewOAuthProgrammingHandler`; a aprovação terminal-local liga reader/lister/writer/revisor Git somente no modo programming. O revisor Git captura baseline por sessão e só observa status/diff; não faz add, commit, push ou limpeza. `7677` continua administrativo local e não é registrado no handler MCP público.

**Evidência:** a integração `cmd/signalspace/programming_composition_test.go` verificou metadata, `initialize`, `tools/list`, chamadas READ/WRITE/GIT, escopos independentes, divergências owner/client/session, revogação com JWT ainda válido e ausência de `run_workspace_tests`/shell/mutação Git. Passaram os testes focados, a suíte Go serial completa, race dos pacotes afetados, vet, build, gofmt e diff check. No fechamento devem ser rechecados listeners/processos residuais; isso não substitui navegador, HTTPS, túnel, grant real, workspace real ou CI.

**Próxima ação:** eventual aceite operacional externo ou inclusão de shell deve ser uma missão separada, com contrato e gate próprios. A publicação remota desta fatia já foi confirmada; não tratar essa publicação nem a aprovação do gate como conclusão de `SS-MVP-002`.


## Checkpoint SS-MVP-002 — direção aprovada para coding agent completo — 23/09/2026

**Decisão de produto:** o proprietário aprovou evoluir a superfície de programação para que o ChatGPT Web possa operar como coding agent completo em workspace autorizado. O alvo inclui filesystem tipado completo e busca nativa, Git operacional em camadas e resultados estruturados/UX especializada por domínio. DevSpace permanece inspiração de fluxo, não dependência; Graphify permanece referência opcional de compreensão estrutural.

**Estado de implementação:** esta decisão documental não adiciona ferramentas. A composição vigente continua READ + WRITE (`replace_text`) + Git review observacional; `test.run`, shell, comandos arbitrários e mutações Git continuam fora da composição pública. `SS-MVP-002` e `SS-BE-007` permanecem PARCIAIS; CI permanece DESCONHECIDA.

**Fronteiras aprovadas para desenho:** filesystem normal não deverá depender de shell; Git local mutável terá capacidade própria e tools tipadas; Git remoto terá gate separado; hooks/configuração executável não podem virar execução implícita; operações Git destrutivas e shell exigem decisão própria. UX pode especializar workspace/filesystem/search/diff/test/process/artifacts, mas autorização continua no backend e plain MCP deve permanecer funcional.

**Próxima ação vinculada:** `SS-MVP-002` — especificar e implementar o primeiro slice do motor de filesystem: `stat_path`, `find_paths`, `search_text`, `create_directory`, `create_text_file` e `write_text_file`, reutilizando a fronteira segura existente de workspace e emitindo resultados estruturados desde o início. Não incluir copy/move/delete/apply_patch, Git mutável, shell ou aceite externo no mesmo slice.

## Checkpoint histórico — SS-MVP-002 filesystem tipado base após rebase — 23/09/2026

**Estado:** o primeiro slice está **implementado e validado localmente**, rebaseado sobre `1cf598a5222a8118d268752dbf2bbd0993f437ff`, mas ainda não foi publicado nesta missão. `SS-MVP-002` permanece parcial quanto ao aceite operacional externo; `SS-BE-007` permanece parcial/em andamento; CI permanece desconhecida. O gate anterior segue aprovado somente para READ + WRITE + Git review observacional.

**Commits e preservação:** o commit de implementação é `e39d7d9` (`feat(signalspace): add typed workspace filesystem tools`); o commit documental é o HEAD desta missão (`docs(signalspace): record filesystem slice and worktree direction`). O histórico remoto não foi reescrito: a branch local está dois commits à frente do remoto, sem push. PR #1, as branches protegidas e os dois patches não rastreados permanecem fora desta alteração.

**Implementação:** seis tools MCP foram adicionadas ao motor comum: READ (`stat_path`, `find_paths`, `search_text`) e WRITE (`create_directory`, `create_text_file`, `write_text_file`). Caminhos, symlink, limites, tipo, precondição/hash, owner, cliente, sessão, escopo e concessão são tratados fail-closed. `diagnostic`/`read` não ganham escrita; `test.run`, shell, Git mutável, copy/move/delete/apply_patch e worktree funcional permanecem fora.

**Validação:** `gofmt -l cmd internal`, `git diff --check`, testes focais, race dos três pacotes, `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, `go vet ./...` e `go build ./...` passaram após o rebase. Não houve listeners em 7676/7677 nem processo `cloudflared` residual. Não houve CI, túnel, HTTPS, navegador, grant OAuth externo, workspace real, merge ou deploy.

**Próxima ação:** retornar ao ChatGPT Web com o relatório desta missão. A direção de worktrees permanece documental; nenhuma worktree foi criada ou usada.

## Checkpoint SS-MVP-002 — managed worktrees v1 local — 24/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL/em andamento**. A fatia managed worktree v1 está **IMPLEMENTADA E VALIDADA LOCALMENTE EM FIXTURES DESCARTÁVEIS**. O promotion gate anterior continua aprovado apenas para READ + WRITE + Git review observacional; esta missão não cria novo gate público nem promove lifecycle MCP. CI permanece **DESCONHECIDA**.

**Ref e preservação:** branch `codex/mvp-vertical-programming`; HEAD local/remoto observado antes da alteração `d9893664725fe75b8b948790a2d33d9e38d9b3ed`. Os dois patches não rastreados do proprietário permaneceram fora do Git, não aplicados e intocados. Não houve push, merge, deploy ou criação de worktree a partir do checkout SignalSpace.

**Implementação:** `internal/workspace/managed_worktree.go` persiste workspaces sob estado privado, associa origem/base SHA, reabre estados sem auto-reparo, cria detached sem copiar dirty/untracked, rejeita filtros executáveis/submodules/gitlinks e remove somente worktree limpa e inativa. `managed_git_runner.go` desabilita configuração global/system, hooks, fsmonitor, terminal/pager/editor e mantém timeout/limite. `Grants` registra metadados seguros; `workspace_id` persistente e `session_id` efêmero permanecem separados. O console local tem request/approve/cancel/resume/list/remove; a superfície MCP não ganhou lifecycle.

**Segurança filesystem:** o componente exato `.git` foi reservado em leitura, stat, find/search, criação, escrita, cópia, movimento, remoção e `apply_patch`; listagem e varredura da raiz o omitem. Caminhos `.gitignore`, `.gitattributes` e `.gitmodules` não são bloqueados por nome.

**Validação:** passaram `go test ./internal/workspace`, os testes direcionados de `cmd/signalspace` para programação/console, `go test ./internal/mcp` e os testes novos de lifecycle/restart/filtros/remoção/reserva `.git`. A primeira execução ampla de `cmd/signalspace` encontrou a instância local ocupando `127.0.0.1:7676`; a repetição serial com listener livre passou. Após os commits locais `1851c66`, `576924e` e `951205e`, passaram `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, race de `internal/workspace`, `internal/mcp` e `cmd/signalspace`, `go vet ./...`, `go build ./...`, `gofmt` e `git diff --check`.

**Próxima ação vinculada:** `SS-MVP-002` — publicar este checkpoint ao ChatGPT Web e aguardar a próxima missão. Os commits permanecem locais: não fazer push/merge, não expor lifecycle por MCP, não usar shell arbitrário e não declarar aceite externo.

## Checkpoint SS-MVP-002 — Git index promotion gate local — 24/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL/em andamento**; `SS-MVP-002-GIT-INDEX-PROMOTION-GATE-001` está **APROVADO PELO PROPRIETÁRIO / IMPLEMENTADO E VALIDADO LOCALMENTE / PUBLICAÇÃO REMOTA PENDENTE**; CI permanece **DESCONHECIDA**.

**Ref e publicação:** branch `codex/mvp-vertical-programming`; a implementação funcional está em `e164415`, com as correções de regressão de testes em `b2cd8ac` e `a964ccf`, e a documentação deste gate em `d179ac6`. O HEAD local atual é posterior a esses commits; o remoto live observado permanece `6fcf46da1f9e1187a8067d8a71a745d31babc618`; não houve push.

**Implementação:** a autorização passou a tratar `signalspace:git.index` de forma independente em metadata, consentimento, authorize, complete e token. A composição programming injeta `git_status` e os operadores de stage/unstage por portas explícitas; a composição diagnostic/read continua sem Git index. `request-programming` genérico rejeita `git.index`; somente request/resume de managed worktree normalizam e aceitam essa capacidade no console owner-side. Nenhum lifecycle de worktree foi exposto por MCP.

**Validação:** passaram os testes focais, race serial dos pacotes afetados, `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, `go vet ./...`, `go build ./...`, `gofmt -l cmd internal` e `git diff --check`. A cobertura inclui metadata, initialize/tools/list, fluxo público status → stage → status staged → unstage, preservação do working tree, revogação e negativos de escopo/modo; após o encerramento dos testes, não ficaram listeners 7676/7677 nem processos SignalSpace/cloudflared residuais.

**Limites e preservação:** não houve CI, HTTPS, túnel/cloudflared, navegador, grant OAuth externo, workspace real, merge ou deploy. Os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` seguem não rastreados, não aplicados e intocados, com SHA-256 preservados. Próxima ação vinculada: executar os gates finais, registrar a documentação, enviar o relatório ao ChatGPT Web e aguardar a missão seguinte; não fazer push.

## Checkpoint SS-MVP-002 — Git local tipado v1 — 24/09/2026

**Estado:** `SS-MVP-002` permanece **PARCIAL/em andamento**. A fatia `git_status` + stage/unstage está **IMPLEMENTADA E VALIDADA LOCALMENTE EM FIXTURES DESCARTÁVEIS**. O promotion gate anterior continua aprovado apenas para READ + WRITE + Git review observacional; `signalspace:git.index` não foi promovido ao entrypoint público. CI permanece **DESCONHECIDA**.

**Ref e commits:** branch `codex/mvp-vertical-programming`; implementação local `d1f46e7e704647a757d7329da09b53e416dcbd7d`; remoto observado permanece `b1313ae8418086b8089a99c388e9065d67e61c7e`, sem push. O commit contém domínio, runner, grants, MCP isolado, adapter local não injetado no público e testes; documentação seguirá em commit separado.

**Implementação verificada:** `git_status` usa Porcelain v2 e fingerprint SHA-256 do índice; `review_git_changes` diferencia `diff` unstaged e `staged_diff`. `stage_git_paths`/`unstage_git_paths` aceitam apenas paths literais limitados, precondições de índice/arquivo/OID e sessão `WorkspaceModeWorktree` gerenciada. Checkout normal falha fechado. O runner neutraliza hooks/configuração global/system/pager/editor/fsmonitor/índices alternativos/askpass/SSH e rejeita filtros executáveis. A composição pública `NewOAuthProgrammingHandler` e `cmd/signalspace` continuam sem `git.index` e sem stage/unstage.

**Validação:** passaram testes focais e race serial de `internal/workspace`, `internal/programming` e `internal/mcp`, incluindo fluxo status → stage → distinção staged/unstaged → unstage, preservação do working tree, scope independente e negação fora de worktree. Ainda faltam os gates completos e a checagem de limpeza no fechamento desta missão; não inferir CI, navegador, HTTPS, túnel, grant ChatGPT Web, workspace real, merge ou deploy.

**Preservação:** os patches `signalspace-oauth-read-scope.patch` e `signalspace-workspace-client-binding.patch` continuam não rastreados, não aplicados e intocados, com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86f3e1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`. Próxima ação vinculada: gates completos, commit documental, relatório ao ChatGPT Web e espera da próxima missão; não publicar remotamente nesta fatia.

## Checkpoint `SS-MVP-002-LOCAL-AUTHORIZATION-V2-DESIGN-001` — 25/09/2026

**Estado:** **DECISÃO ACEITA / DOCUMENTAÇÃO RECONCILIADA / NÃO IMPLEMENTADA**. `SS-MVP-002` permanece **PARCIAL/em andamento**; `SS-BE-007` permanece **PARCIAL**; CI permanece **DESCONHECIDA**. O checkpoint está vinculado ao ID existente na `TASKLIST.md`.

**Ref e base:** branch `codex/mvp-vertical-programming`, HEAD e remoto revalidados antes da edição em `a62eba66130f0d55581ef97b8c7d9a32afe01c4a`. O trabalho foi autorizado somente para ADR, contratos conceituais, modelo, migração, critérios e reconciliação documental. Não houve código, workflow, capability, escopo runtime, grant, push, merge, rebase ou deploy.

**Decisão:** OAuth autentica uma composição (`signalspace:diagnostic`, `signalspace:read` ou `signalspace:programming` no alvo); a autoridade local decide capabilities internas por owner/client/workspace/session/tool/context. O Policy Engine retorna `ALLOW`, `DENY` ou `REQUIRE_APPROVAL`; permits são concretos e de uso único. Aprovação não executa a chamada anterior. Shell é independente, não é sandbox por root/worktree e exige gate próprio.

**Fontes reconciliadas:** [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md), `AGENTS.md`, `TASKLIST.md`, `docs/PRODUCT.md`, `docs/MVP.md`, `docs/LOCAL_ADMIN_AUTHORIZATION.md`, `docs/WORKSPACE_SECURITY.md`, `docs/PROGRAMMING_TOOLS.md`, `docs/QUICK_PANEL.md`, `docs/TRANSPORT.md`, `docs/ROADMAP.md` e este checkpoint. `DOCUMENTATION_AND_CONTINUITY.md` não foi alterado porque não havia necessidade protocolar.

**Validação e limites:** `git diff --check`, links internos, busca de contradições e verificação de ausência de alterações em `.go`, `.js`, `.ts` e workflows são gates do fechamento desta missão. Os dois patches protegidos permanecem fora do Git, não aplicados e intocados, com SHA-256 `02e9de3193f8e85389406fda3f843aa5837739746091ac575b86e3f1e0d4cd9b` e `0fddedf6ad7ea61751a1aeda2417d956b780dda6cdd670ebf8a444df1a4e48f2`. Não há aceite externo novo, CI, navegador, workspace real, túnel ou permissão nova.

**Próxima ação:** somente uma tarefa posterior pode implementar token family, Policy Engine, approvals/painel, migração Programming, origem estável ou shell independente. Enviar este relatório ao ChatGPT Web e aguardar a próxima missão.

## Checkpoint `SS-MVP-002-OAUTH-CONNECTION-LIFECYCLE-V2-001` — 26/09/2026

**Estado:** **IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICAÇÃO REMOTA PENDENTE**. O ciclo OAuth v2 está disponível somente no auth harness opt-in; `SS-MVP-002` permanece **PARCIAL/em andamento**, porque a migração da superfície pública e o aceite externo continuam pendentes. CI permanece **DESCONHECIDA**.

**Implementação:** `internal/auth` agora suporta a composição `signalspace:programming`, DCR/metadata com `authorization_code + refresh_token`, access TTL configurável (default 60 minutos), refresh TTL configurável (default 30 dias), segredo opaco retornado uma vez e persistido somente por hash SHA-256, token family vinculada a client/resource/scope, rotação, detecção de reuse, revogação persistente e migração v1 explícita. O access JWT preserva issuer, audience, owner subject, client ID e JTI.

**Validação:** `go test ./internal/auth` e `go test -race ./internal/auth` passaram, cobrindo PKCE, código de uso único, binding redirect/resource/client, escopo sem expansão, restart, revogação após restart, concorrência de refresh e preservação do runtime legado sem anúncio v2.

**Limites:** não houve Policy Engine, grant/capability local novo, migração das tools públicas, painel, shell, `test.run`, lifecycle MCP de worktree, workspace real, CI, HTTPS, túnel, navegador, merge ou deploy. Os patches protegidos continuam untracked, não aplicados e intocados com os SHA-256 registrados anteriormente.

**Próxima ação vinculada:** executar gates completos, criar commit focado, verificar fast-forward e publicar somente na branch `codex/mvp-vertical-programming`; depois enviar o relatório ao ChatGPT Web e aguardar a próxima missão.

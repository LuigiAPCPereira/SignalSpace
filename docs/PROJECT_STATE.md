# SignalSpace — checkpoint de continuidade

**Natureza:** snapshot derivado da [TASKLIST](../TASKLIST.md), contratos, Git e CI; não é autorização nem lock. **Último commit de código validado localmente em 20/09/2026:** [`3deefc1`](https://github.com/LuigiAPCPereira/SignalSpace/commit/3deefc1), branch `feat/m1-local-mcp-diagnostic`, [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1) aberto/draft/não mesclado; CI remota deste SHA não consultada conforme orientação do proprietário, estado desconhecido. `gofmt`, `go test ./...`, race dos pacotes afetados, `go vet ./...`, `go build ./...`, `node --check` e teste funcional Node passaram localmente.

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

**Ref observada:** `codex/mvp-vertical-programming` após `b46cda6` (`feat(mcp): test isolated workspace write authorization`). `git pull --ff-only` retornou `Already up to date` contra `origin/codex/mvp-vertical-programming` antes desta mudança; os dois patches não rastreados permanecem preservados.

**Implementação:** `internal/mcp/workspace_write.go` liga identidade verificada, escopo `signalspace:workspace.write`, sessão e `WorkspaceTextWriter.ReplaceText` com resultado estruturado. A injeção é não exportada em `OAuthConfig`, ficando disponível somente dentro do pacote para o harness; `cmd/signalspace` não consegue registrar essa capacidade. Os modos de produção continuam sem emissor de escrita e sem `replace_text` em `tools/list`.

**Validação:** `go test ./internal/mcp ./internal/workspace ./internal/programming ./cmd/signalspace -parallel=1 -count=1 -timeout=180s` passou. O teste MCP isolado cobre escrita autorizada, read-only/sem escopo, cliente/owner/sessão/workspace divergentes, revogação, tokens inválidos/expirados, path hostil, conflito, duplicação e ausência pública. Gates completos e paridade remota final ainda são pendentes nesta etapa documental.

**Estado:** SS-MVP-002 continua **em andamento/PARCIAL**: a fronteira MCP de teste está confirmada, mas nenhum escopo de escrita foi adicionado ao OAuth emissor, ao Quick ou ao MCP público. Próxima ação: revisar contrato remoto por operação e decidir se/como um gate próprio de publicação será autorizado; não expor `workspace.write` por inferência.

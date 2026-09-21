# SignalSpace — histórico recuperável da frente de desenvolvimento

Registro **seletivo**, não transcrição de conversas. Progresso: [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), Git/CI. Comportamento: [produto](PRODUCT.md), [workspace](WORKSPACE_SECURITY.md), [contrato admin](LOCAL_ADMIN_AUTHORIZATION.md). Autoridade de IDs/estado: [TASKLIST](../TASKLIST.md), checkpoint derivado [PROJECT_STATE](PROJECT_STATE.md). Não inventar eventos.

## 19–20/09/2026 — diagnóstico e leitura experimental (SS-BE-001)

- Proprietário relatou conexão OAuth com `diagnosticID` correlacionado após `tools/list`; [QUICK_TUNNEL.md](QUICK_TUNNEL.md) e PR. Software não atestado.
- Em 20/09, relatou `read_file` com `Teste de leitura SignalSpace` e negativa após revoke; `list_directory` raiz `nested`/`readme.txt` e `nested/inside.txt` com negativa após revogação. Sem log por listagem ou medição precisa da expiração JWT; não repetido nesta etapa.
- Apenas diagnóstico/leitura opt-in; nenhuma aprovação por ferramenta, escrita, Git ou shell.

## 20/09/2026 — contrato e preparação de backend (SS-BE-002 a SS-BE-008)

- [Contrato admin](LOCAL_ADMIN_AUTHORIZATION.md) fixa 7676/7677 distintos, pareamento com sessão imediata, bootstrap/CSRF, grant anterior ao OAuth read, estados versionados/retidos e nenhuma aprovação pública. [Alinhamento UX](https://github.com/LuigiAPCPereira/SignalSpace/blob/feat/frontend-oauth-consent/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md) é UX, não segurança/integração.
- Componentes isolados de transporte, sessão, HTTP admin, status público em 7676, snapshots/handlers admin ainda sem Quick na época. [CI #98](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35518520439) passou.
- [`7dbd5d2`](https://github.com/LuigiAPCPereira/SignalSpace/commit/7dbd5d21c3cb358e9f74d2200e2834371dfff689) migrou stdin para `DecideTerminal`, compartilhando decisão/revalidação do grant; [CI #106](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521202757) e [CI #107](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521299236) passaram.

## 20/09/2026 — adoção documental v2

- Ref `08ffb52`: protocolo 1.0 e trio AGENTS/protocolo/checkpoint sem TASKLIST/roadmap/histórico. Anexo do Project é especificação geral v2.0 não sincronizada.
- Adaptação v2.0 com inventário SS-BE-001…008, marcos, histórico, checkpoint SS-BE-004 e [relatório Adoption Gate](ADOPTION_REPORT.md), [commit `0f5ddec`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0f5ddec789a11ebdeb55480561c623a04df14f67). Nenhum Go/frontend/agendamento/merge naquela adoção.

## 20/09/2026 — ciclo OAuth no domínio (SS-BE-004)

- A partir de `0f5ddec`, `decided_at`/versões e delegação `Approve`, snapshots terminais sem PKCE/CSRF/state retidos até dez minutos e 64 entradas; `COMPLETED` significa código, não token. `/authorize/complete` valida grant/capacidade antes de consumir pedido. Testes cobrem resposta perdida simulada, negação após prazo, tombstone/revoke e 20 conclusões concorrentes.
- [CI #120](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35523886303) falhou em teste antigo de 404 imediato, incompatível com retenção; teste corrigido; [CI #122](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35523999575) passou. Corrigida prioridade DENIED vs expiração; [CI #124](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35524113001) PASS no [commit `171fbc8`](https://github.com/LuigiAPCPereira/SignalSpace/commit/171fbc856677f6f7a2f8ed10b6bcc13322773af2). Sem Quick/browser/rede real nessa etapa.

## 20/09/2026 — endurecimento do Gate administrativo (SS-BE-003)

- PBKDF2 próprio sem revisão substituído em [`d4c8c3a`](https://github.com/LuigiAPCPereira/SignalSpace/commit/d4c8c3a9bf1c1d9236bafabddb6560d12a2dd158) por Argon2id `golang.org/x/crypto` v0.41.0 compatível Go 1.23, sal aleatório, 32 MiB/3 passagens/1 via, 32 bytes; limite 64 sessões e 429 na quinta falha. [CI #132](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35525663837) PASS.
- [`1c73a7f`](https://github.com/LuigiAPCPereira/SignalSpace/commit/1c73a7f8f9f1778aee1a780fbe086dd07c7357ac) diferencia 401/403 em lock/refresh com revalidação sob mutex; testes garantem que CSRF inválido não revoga sessão. [CI #134](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35525837834) PASS. Sem auditoria independente ou browser.

## 20/09/2026 — reserva e isolamento HTTP preparados (SS-BE-002)

- [`8ac0746`](https://github.com/LuigiAPCPereira/SignalSpace/commit/8ac0746cc38e0b83186dd50dcc56fc7214e1c80a): `quick_ports.go` reserva apenas 7676 no terminal, duas portas num caminho interno sem fallback; testes com servidores HTTP efêmeros isolam `/mcp` e admin, Host, falha de reserva/rebind. [CI #140](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35530068609) PASS. Naquele marco, não havia seleção na CLI nem 7677 no Quick.

## 20/09/2026 — testes HTTP da fila com OAuth real (SS-BE-005/007)

- [`8c8ac7d`](https://github.com/LuigiAPCPereira/SignalSpace/commit/8c8ac7d1df52d2d6faa1e0d8e519745e5a7f6899) adicionou `admin_oauth_http_test.go`; [CI #144](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35530968196) parou só em formatação; correção [`a268f1b`](https://github.com/LuigiAPCPereira/SignalSpace/commit/a268f1b66447aa4546166655f009b0ec6d2ac4ea); [`ea8165c`](https://github.com/LuigiAPCPereira/SignalSpace/commit/ea8165c6b316f802c49baaa9874ef6cf97d47928) acrescentou 410/404 com fonte controlada.
- Dois sockets HTTP, emissor OAuth e Gate reais provam isolamento público/admin, sessão, CSRF, versões, metadados forjados, POST duplicado, GET após corpo 200 descartado e COMPLETED após `/authorize/complete`. 410/404 é fixture, não avanço de relógio OAuth. [CI #146](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35531123085) PASS; descarte de corpo ainda não era desconexão real.

## 20/09/2026 — socket desconectado e grant revogado no HTTP (SS-BE-007)

- [`a5ffcce`](https://github.com/LuigiAPCPereira/SignalSpace/commit/a5ffcce19f226f74aecced053c69a150632109a7) segura bytes após commit; [`61a2807`](https://github.com/LuigiAPCPereira/SignalSpace/commit/61a2807990f21ba92f57ecc7534827b9dcff1267) testa grant real. [CI #151](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35532159413) falhou pois interceptou também POST duplicado; corrigido em [`f474a8c`](https://github.com/LuigiAPCPereira/SignalSpace/commit/f474a8cd12cbe39c6af0f4fd968f64f39b3c707b).
- [CI #152](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35532258138) PASS: cliente realmente fecha socket antes da resposta após commit; GET novo recupera APPROVED/v2, POST duplicado 409. Grant concedido no console e revogado durante pedidos read expõe REVOKED, recusa approve 409 e complete 403, preserva deny. Sem cloudflared/browser real.

## 20/09/2026 — proteção HTTP do estado OAuth público (SS-BE-006)

- [`f09108b`](https://github.com/LuigiAPCPereira/SignalSpace/commit/f09108b44317d17156fd926cdc01632ce48b162b) endureceu CSP, Referrer-Policy, X-Frame-Options/CORP de `/authorize/status` sem mudar JSON/CORS. Consentimento bloqueia scripts, sem ampliar exceção de estilo inline existente.
- [`01bc5cf`](https://github.com/LuigiAPCPereira/SignalSpace/commit/01bc5cf3cd573f604aa12bfccbec7eb9c7a457c8) testes HTTP; [CI #159](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35533106995) falhou em gofmt; [`30d5ca9`](https://github.com/LuigiAPCPereira/SignalSpace/commit/30d5ca9a1229459025e8a06f2cd77c7813c03bc2) corrigiu, [CI #160](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35533172412) PASS. Sem execução CSP no navegador nem polling JS. Naquele momento listener admin Quick ainda pendente.

## 20/09/2026 — API administrativa opt-in no processo Quick (SS-BE-002/008)

- [`d34035a`](https://github.com/LuigiAPCPereira/SignalSpace/commit/d34035a7150fd73f96fc2bb4d4678418be363c42) adicionou probe limitado e local com Host canônico, somente bootstrap não privilegiado. [`530bcf8`](https://github.com/LuigiAPCPereira/SignalSpace/commit/530bcf8fcb79571bd147393712c4bcebcaf025e9) compôs reserva dupla/`Gate` antes do túnel, OAuth após URL, admin antes do MCP e cleanup conjunto. [`1f8e1b7`](https://github.com/LuigiAPCPereira/SignalSpace/commit/1f8e1b7f617273abb6e82a264b70a847a25aeea2) e [`dfa4003`](https://github.com/LuigiAPCPereira/SignalSpace/commit/dfa4003ec30b8b6fb43851aaae067ce981d7a9a6) acrescentaram CLI `connect quick [read] panel` com confirmação distinta, mantendo modo terminal anterior.
- [`623da58`](https://github.com/LuigiAPCPereira/SignalSpace/commit/623da587d2ded41d1adf13322b8bc94bbcc70d3e) testou CLI, porta 7677 ocupada antes do túnel, dois servidores no processo Quick com **stub cloudflared** que valida URL de origem estritamente `http://127.0.0.1:7676`, separação das rotas, pareamento + fila autenticada, Ctrl+C/rebind. [CI #168](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536736458) falhou em gofmt de novo teste; [`f7efbb3`](https://github.com/LuigiAPCPereira/SignalSpace/commit/f7efbb3f3fb5e04ae542140c15ecd79526e595f9) corrigiu; [CI #169](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536833997) PASS completo. [`e6ddd3`](https://github.com/LuigiAPCPereira/SignalSpace/commit/e6ddd3df8cac5fa2099fa4b1e4a1b24387ca14b4) corrigiu texto de `doctor transport`; [CI #171](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35537024426) PASS no último código. [Guia de operação e limites](QUICK_PANEL.md).
- **Limites desta fase:** API local somente opt-in e testada, sem HTML funcional, cloudflared real, navegador, auditoria independente ou deploy; falha pós-readiness ainda pendia. SS-BE-002/008 sem aceite operacional, SS-BE-006/007 aguardavam integração/smoke; frontend não modificado.

## 20/09/2026 — falha administrativa após prontidão no Quick (SS-BE-002/007/008)

- [`09ee603`](https://github.com/LuigiAPCPereira/SignalSpace/commit/09ee603e3a1b864ca29ce6412ebf1f7281621ef1) acrescentou fábrica do servidor admin apenas como parâmetro interno de teste; produção continua passando `admin.NewServer`, sem configuração por CLI. [`d6d03a8`](https://github.com/LuigiAPCPereira/SignalSpace/commit/d6d03a87df7d1f40527650bafbcb53e1cac69e52) adicionou teste de processo Quick com **cloudflared simulado e sockets HTTP reais**: depois dos probes admin e público, interrompe o admin (a) antes de anunciar URL durante verificação pública, (b) após a URL ter sido anunciada. Em ambos exige erro com identificação do servidor administrativo, encerramento da sessão e rebind de 7676/7677. No caso (a), exige que nem URL pronta nem código de pareamento tenham sido anunciados.
- [CI #176](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35538258629) **PASS integral** no commit `d6d03a8`: formato, testes, race, vet e build. Não simula uma queda arbitrária na rede Cloudflare, não usa navegador e não é smoke remoto. Próximos aceites permanecem em [TASKLIST](../TASKLIST.md) e [checkpoint](PROJECT_STATE.md); PR draft, sem merge.

## 20/09/2026 — decisão OAuth atravessando o processo Quick (SS-BE-005/006/008)

- A outra frente confirmou `feat/frontend-oauth-consent` HEAD [`3911c4e`](https://github.com/LuigiAPCPereira/SignalSpace/commit/3911c4eae2ec514ff5f30d904a993cf103f9be32): protótipos escolhidos `frontend/consent/prototypes/owner-pairing-flow.html` (parear/desbloquear) e `frontend/consent/prototypes/local-approval-flow-v2.html` (decisão). HTML/CSS/JS apenas demonstrativo; sem autenticação, rede ou backend. `frontend/consent/consent.html` é legado. O contrato de UX está em [alinhamento](https://github.com/LuigiAPCPereira/SignalSpace/blob/3911c4eae2ec514ff5f30d904a993cf103f9be32/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md); leitura apenas, sem editar/integrar a branch.
- [`1c475ec`](https://github.com/LuigiAPCPereira/SignalSpace/commit/1c475ec776a014901a57473d24bdab4b0746be6d) criou `quick_panel_oauth_test.go`; [CI #181](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35539218961) parou em gofmt e não executou testes. [`98d06de`](https://github.com/LuigiAPCPereira/SignalSpace/commit/98d06deeaf58bafee3972814bae8c5549f065aa0) corrigiu formato/import; [CI #182](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35539286668) **PASS integral** no último código: formato, testes, race, vet e build.
- O teste novo usa processo cloudflared **simulado e verificador HTTPS simulado**, mas as portas públicas/admin e handlers OAuth/Gate reais do `runQuickWithOptions`: registro de cliente, `/authorize` PENDING, pareamento com bootstrap/CSRF, fila autenticada, `/authorize/status` negado sem cookie e PENDING/APPROVED com cookie próprio, POST admin com Origin/CSRF e versão PENDING/v1 → APPROVED/v2, `/authorize/complete` retorna 303 com código no callback permitido, GET admin retém COMPLETED; cancelamento libera as duas portas. Não efetuou troca de código por token nem callback/ChatGPT reais. **SS-BE-005/006/008 avançam em integração de processo; aceites operacionais continuam pendentes.**
- Não houve HTML/JS funcional integrado, escrita na branch frontend, túnel real, navegador, merge ou deploy. Próximo passo de SS-BE-002/007/008 é smoke externo somente no ambiente descartável autorizado do proprietário; SS-BE-006 depende de frontend funcional, CSP e navegador, com autorização de integração distinta.

## 20/09/2026 — smoke M3 REAL relatado pelo proprietário (SS-BE-002/005/007/008)

- Fonte: relato do proprietário apresentado nesta conversa para a revisão [`bd60fc3`](https://github.com/LuigiAPCPereira/SignalSpace/commit/bd60fc3c17f84261eedf02d5db0f6679ad29a227), datado de 20/09/2026, com resultado **SMOKE REAL · APROVADO** para diagnóstico OAuth+MCP+API administrativa local com túnel Cloudflare real. Relatou correlação de `diagnosticID` entre ChatGPT Web e terminal, `tools/list` autenticado, isolamento das portas e encerramento/limpeza observados; aprovação OAuth administrativa efetiva no caminho de diagnóstico.
- **Grau de evidência:** esta sessão não executou o teste nem recebeu logs brutos, valor concreto do ID, comandos, transcrição do OAuth, comprovantes independentes de rede/portas ou artefato de navegador. Registrar aceitação operacional **somente do caminho de diagnóstico** para SS-BE-002/008, decisão real relatada para SS-BE-005; SS-BE-007 ainda tem matriz negativa operacional incompleta. Não declarar que read/revoke M2 foi repetido no modo painel, que todas as respostas/ataques foram exercitados, que o token foi inspecionado individualmente ou que a interface visual está integrada.
- Próxima tarefa SS-BE-006: interface funcional, polling, CSP e navegador em integração separada e autorizada; protótipos frontend `3911c4e` permanecem dados fictícios, branch preservada. PR #1 permanece draft e sem merge; nenhum código Go/HTML, deploy ou escopo de ferramentas alterado neste registro.

## 20/09/2026 — polling seguro da página OAuth (SS-BE-006)

- [`0ed5f5a`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0ed5f5a) implementou a menor fatia independente do backend: a página `/authorize` agora carrega `consent.js` same-origin com `script-src 'self'`; o script usa somente `GET /authorize/status?request_id=...` com `credentials: same-origin`, não acessa Web Storage nem administra sessão, bloqueia a conclusão enquanto PENDING e habilita o formulário apenas após APPROVED.
- O script trata explicitamente APPROVED, DENIED, EXPIRED, 403, 404, 429 com `Retry-After`, respostas HTTP temporárias, JSON inválido e falhas de rede; a decisão continua exclusivamente no terminal/API admin e `/authorize/complete` permanece a autoridade de conclusão. O request ID é validado no cliente e novamente no servidor; cookie OAuth continua HttpOnly/Secure e o CSRF não é colocado em URL ou script.
- Validação local nesta sessão: `gofmt -l .`, `git diff --check`, `go test ./...`, `go test -race ./internal/auth ./cmd/signalspace`, `go vet ./...`, `go build ./...`, `node --check internal/auth/consent.js` e testes focados de status/Quick — todos PASS. A primeira tentativa deixou um processo de teste próprio segurando 7676 após uma falha de roteamento; o processo foi encerrado, a rota explícita foi corrigida e a sequência completa passou.
- Limites: CI remoto ainda não executada/observada para `0ed5f5a`; não houve execução em navegador real nem integração da branch `feat/frontend-oauth-consent`. PR #1 permanece draft, sem merge/deploy; os dois patches não rastreados do worktree foram preservados.

## 20/09/2026 — revisão da CSP e testes funcionais do polling (SS-BE-006)

- A revisão identificou uma falha comprovada em [`0ed5f5a`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0ed5f5a): `default-src 'none'` sem `connect-src` bloqueava o `fetch` same-origin do polling. Também foi corrigido o estado inicial do botão, que agora nasce `disabled` no HTML.
- [`0988fc5`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0988fc53101ef87a8f76c7b08865cf75d5cb872c) adiciona `connect-src 'self'`, mantém `script-src 'self'`, todas as demais diretivas existentes e um `<noscript>` que preserva a submissão manual sujeita à validação do servidor. `consent_page_test.go` verifica as diretivas, o botão desabilitado e o fallback.
- `consent_js_test.mjs` executa funcionalmente a IIFE com DOM/fetch/timers controlados e cobre PENDING → APPROVED, DENIED, EXPIRED, 403, 404, 429/Retry-After, 5xx, JSON inválido, perda de rede, headers same-origin/no-store e submissão protegida. `gofmt`, `go test ./...`, race dos pacotes afetados, `go vet ./...`, `go build ./...`, `node --check` e os testes Node passaram localmente.
- [CI #188](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35548396262) passou integralmente no SHA. Tentou-se um harness HTTPS local descartável com cookie e aprovação automática, mas o navegador integrado recusou o certificado autoassinado (`ERR_CERT_AUTHORITY_INVALID`); nenhum intersticial foi contornado e nenhum túnel público foi iniciado. A aceitação real de navegador permanece desconhecida/pendente.
- PR #1 permanece draft, sem merge/deploy; `feat/frontend-oauth-consent` e os dois patches não rastreados do worktree foram preservados.

## 20/09/2026 — aceite parcial no navegador via Quick Tunnel real (SS-BE-006)

- Após autorização explícita do proprietário, `go run ./cmd/signalspace connect quick` iniciou `cloudflared` apontando somente para `http://127.0.0.1:7676`. A primeira sessão registrou `registered=true`, mas falhou fechada porque o DNS local não resolveu o hostname durante a janela; nenhuma URL foi anunciada como pronta e as portas foram liberadas.
- Uma segunda sessão passou o preflight HTTPS público do SignalSpace e anunciou uma URL efêmera. Foi registrado um cliente OAuth descartável com callback permitido do ChatGPT; a API administrativa 7677 não foi iniciada nem tunelada.
- No navegador integrado, a página `/authorize` carregou pela URL HTTPS pública sem intersticial. A evidência observada foi: botão `Continuar` inicialmente desabilitado, status PENDING, decisão `approve <id>` no terminal, status `Aprovação local confirmada. Você pode continuar.` e botão habilitado. Logs de console não apresentaram erros ou avisos CSP.
- O botão não foi acionado: isso emitiria um código OAuth e redirecionaria para o callback do ChatGPT, uma concessão que exige autorização específica separada. `/authorize/complete` permanece coberto pelos testes Go/Node e continua sendo a autoridade final. DENIED/EXPIRED/403/404/429/5xx, JSON inválido e perda de rede têm cobertura funcional Node, mas não foram reproduzidos visualmente no navegador.
- A aba descartável foi fechada, o túnel foi encerrado com Ctrl+C e não restaram listeners 7676/7677 nem processo `cloudflared`. O estado de SS-BE-006 permanece implementado não validado no inventário, com aceite visual HTTPS do caminho feliz comprovado e pendências negativas/final explícitas.

## 20/09/2026 — regressões automatizadas de expiração do consentimento (SS-BE-006)

- A missão seguinte exigiu fechar o gate JavaScript e investigar a aprovação que expira antes do clique, preservando `/authorize/complete` como autoridade.
- O commit [`3deefc1`](https://github.com/LuigiAPCPereira/SignalSpace/commit/3deefc1) adicionou `actions/setup-node@v4` com Node.js `22.14.0` e a etapa explícita `node --test internal/auth/consent_js_test.mjs` ao workflow, sem remover gates Go.
- `consent.js` continua o polling depois de `APPROVED`, desabilita a conclusão em respostas inconclusivas e interrompe o timer ao submeter. O navegador não deriva validade pelo relógio local.
- `consent_js_test.mjs` passou a simular parsing JSON inválido e a transição determinística `APPROVED → EXPIRED` antes do clique. `lifecycle_test.go` comprova que a conclusão expirada retorna negativa e não emite código.
- Validação local no SHA `3deefc1`: `gofmt -l cmd internal`, `git diff --check`, `go test ./...`, `go test -race ./internal/auth ./cmd/signalspace`, `go vet ./...`, `go build ./...`, `node --check internal/auth/consent.js` e `node --test internal/auth/consent_js_test.mjs`: PASS. Uma primeira execução paralela dos testes de processo colidiu nos listeners fixos 7676; a repetição sequencial passou e não deixou listeners/processos.
- Conforme orientação do proprietário, a CI remota não foi consultada após este commit; o estado remoto é desconhecido. Não havia navegador/túnel descartável já autorizado para nova execução, então a evidência visual permanece apenas no caminho feliz Quick Tunnel já registrado. Nenhum grant OAuth foi emitido, nenhum túnel novo foi aberto, e branch frontend/patches/PR draft foram preservados.

## 20/09/2026 — reconciliação do push de SS-BE-006

- O primeiro parecer do advisor viu o remoto em `cf06e53` porque a consulta ocorreu antes da publicação. O push normal subsequente retornou `cf06e53..9e93b65` na branch `feat/m1-local-mcp-diagnostic`.
- Evidência local/remota reconsultada: HEAD completo `9e93b65a2e897cfb4c41539dd3fb65d3ccc576e1`; `git ls-remote origin refs/heads/feat/m1-local-mcp-diagnostic` retornou o mesmo SHA; `git diff origin/feat/m1-local-mcp-diagnostic..HEAD` e a diferença inversa ficaram vazias; status preservou somente os dois patches não rastreados.
- A CI do SHA exato não foi consultada, conforme orientação do proprietário; não registrar PASS/FAIL remoto. Nenhum túnel, grant OAuth, merge ou deploy foi realizado.
- Depois da reconciliação, o commit somente documental `66f2641` foi publicado normalmente; ele é o HEAD corrente da branch, enquanto `3deefc1` continua sendo o commit funcional e `9e93b65` o checkpoint anterior.

## 20/09/2026 — fatia local da matriz negativa SS-BE-007

- Reabertos na ref corrente os onze grupos de `docs/LOCAL_ADMIN_AUTHORIZATION.md`, §8, e classificados na [TASKLIST](../TASKLIST.md) como cobertura automatizada local, cobertura parcial ou pendência operacional. Testes simulados e harnesses não foram contados como smoke externo.
- A lacuna selecionada foi o grupo 1, por risco de exposição do painel no listener público. O teste novo `cmd/signalspace/quick_ports_test.go:TestQuickPublicListenerRejectsAdministrativeMatrix` reproduziu `307` para `OPTIONS /api/admin/v1//session`, causado pela normalização do `http.ServeMux`, antes da correção.
- `cmd/signalspace/main.go` passou a rejeitar caminhos decodificados sob `/api/admin/` antes do mux público. A repetição do teste cobre seis variantes — sessão, pareamento, detalhe, decisão, caminho codificado e barras duplicadas — com `404` e sem `Set-Cookie`.
- Validação focada sequencial: matriz pública, isolamento Quick, testes negativos de `internal/admin`/`internal/auth` e testes de `internal/mcp`/`internal/workspace` passaram. CI remota não foi consultada por orientação do proprietário; nenhum túnel, TLS bypass, navegador, grant OAuth, frontend, merge ou deploy foi usado.
- SS-BE-007 permanece em andamento: faltam evidências operacionais de DNS rebinding/proxy, restart completo, estados negativos visuais/externos e repetição de leitura/revogação no fluxo M3.

## 20/09/2026 — revisão do isolamento público antes do ServeMux (SS-BE-007)

- A revisão do commit local `b2ca6e4` confirmou que o bloqueio literal `/api/admin/` não cobria barras duplicadas no início/meio nem segmentos `.`/`..`. O teste ampliado reproduziu `307` em `//api/admin/v1/session`, `/api//admin/v1/session`, `/api/./admin/v1/session` e `/prefix/../api/admin/v1/session`; o problema foi a normalização do `http.ServeMux`, não acesso comprovado ao painel.
- A correção mínima em `cmd/signalspace/main.go` passou a rejeitar qualquer segmento exatamente `admin` do `r.URL.Path` antes do mux. A matriz em `cmd/signalspace/quick_ports_test.go` cobre quinze variantes administrativas, incluindo `/admin`, caminhos codificados e métodos alternativos, exigindo `404`, ausência de `Location` e ausência de `Set-Cookie`.
- O mesmo teste verifica que `GET /mcp`, `GET /authorize` e `GET /token` continuam alcançando seus handlers públicos sem `404` ou redirecionamento. O ambiente é HTTP loopback com somente o listener público; não há encaminhamento/handler admin em 7677.
- A validação focalizada sequencial passou. O código e os testes permanecem locais até a publicação fast-forward; CI não foi consultada conforme orientação do proprietário e continua desconhecida. Nenhum túnel, bypass TLS, navegador, grant OAuth, alteração frontend, merge ou deploy foi realizado.

## 20/09/2026 — negativos de Host, Origin e proxy no loopback (SS-BE-007)

- Reaberta a implementação de `internal/admin/transport.go`: a fronteira exige `Host: localhost:7677`, recusa Origin externo/ausente em mutações e não consulta `Forwarded` nem `X-Forwarded-*`. Não foi necessário corrigir código de produção.
- `TestAdminTransportRejectsForgedProxyHeadersOverLoopback` adicionou servidor HTTP loopback com handler sentinela. Host inválido/público, Origin cruzado, preflight e POST sem Origin permanecem em `403`, sem chamada ao API e sem `Access-Control-Allow-Origin`, mesmo com `Forwarded`, `X-Forwarded-Host`, `X-Forwarded-Proto` e `X-Forwarded-For` forjados. Host/origem válidos continuam alcançando a fronteira de transporte sem credenciais.
- `TestAdminTransportProxyHeadersDoNotBypassSessionOrCSRF` atravessa o `Gate` real: pareamento válido com proxy forjado funciona na camada esperada; refresh com sessão válida e CSRF forjado retorna `403 ACCESS_DENIED`; sessão forjada retorna `401 AUTH_REQUIRED`. Isso separa transporte válido de autenticação/sessão/CSRF.
- A evidência é automatizada em loopback; não demonstra DNS rebinding, proxy real ou host rewrite operacional. CI não foi consultada conforme orientação do proprietário; patches, branch frontend e restrições de túnel/OAuth/merge/deploy foram preservados.

## 20/09/2026 — lacuna literal de Host canônico e reinício local (SS-BE-007)

- `TestAdminTransportRejectsForgedProxyHeadersOverLoopback` passou a cobrir literalmente `Host: evil.example` com `Forwarded` e `X-Forwarded-Host` apontando para `localhost:7677`. A resposta continuou `403`, sem execução do handler e sem alteração de produção.
- `TestQuickInstanceRestartDropsAdministrativeAndWorkspaceAuthorizations` exercitou a composição local do Quick sem túnel: uma sessão administrativa e uma concessão de workspace foram criadas, o encerramento invalidou ambas, e uma nova composição iniciou sem pareamento nem concessão herdada. A prova complementa `internal/auth/store_test.go`, que já cobre identidade OAuth persistente e descarte de pedidos/códigos.
- Testes focados de `internal/admin` e `cmd/signalspace` passaram. A evidência permanece local/automatizada: não inclui túnel real, navegador/rede, DNS rebinding, proxy real ou restart operacional completo. CI não foi consultada e continua desconhecida.

## 20/09/2026 — reforço HTTP do reinício e reconciliação da matriz (SS-BE-007)

- `TestQuickInstanceRestartDropsAdministrativeAndWorkspaceAuthorizations` foi reforçado com servidor HTTP loopback da nova instância. O cookie administrativo emitido pela instância anterior é apresentado a `/api/admin/v1/session`; a nova `Gate` responde `401 AUTH_REQUIRED` e envia a limpeza do cookie obsoleto. A prova continua local e não representa restart operacional completo.
- A tabela da `TASKLIST.md` foi reconciliada com a seção detalhada: o grupo 1 agora registra quinze variantes administrativas, não seis. Nenhuma alteração de produção foi necessária.
- A evidência permanece sem túnel, navegador/rede, DNS rebinding ou proxy real; CI não foi consultada e continua desconhecida.

## Limites do registro

Conector GitHub mostra arquivos versionados e metadados remotos, **não worktree/stashes locais**. Datas acima pertencem a registros observados; não inventar tempos de teste. Documento não substitui contratos, TASKLIST, Git/CI ou versões reais do Project/Codex/agendamentos.

## 20/09/2026 — retomada do MVP e fatia vertical de programação

- Recuperação local: branch isolada `codex/mvp-vertical-programming` criada do backend HEAD `d7111c8`; `origin/feat/frontend-oauth-consent` foi apenas consultada em `3911c4e`; os dois patches não rastreados foram preservados. CI, túnel, grant OAuth, merge e deploy não foram usados.
- Fontes: reabertos `AGENTS.md`, `DOCUMENTATION_AND_CONTINUITY.md`, PRODUCT, MVP, ROADMAP, TASKLIST, PROJECT_STATE, SESSION_LOG, contratos de autorização/workspace e o alinhamento frontend na ref correta. O kit canônico local v2.2 também foi lido nos documentos de continuidade, cenários, rastreabilidade, validação, distribuição/bootstrap e apresentação. A adaptação versionada do SignalSpace continua v2.0 e não foi tratada como cópia sincronizada do Project/Notion.
- Implementação: `1db1ebd` adiciona `ReplaceText` com conteúdo esperado, limites, no-symlink, conflito, permissões e revoke; `d94915c` adiciona `go test ./...` fixo, limites/timeout, snapshot Git somente leitura e teste vertical; `9fdf575` adiciona UI administrativa same-origin em 7677 com sessão, pareamento, unlock, fila, decisão/reconciliação, refresh explícito e lock; `9e26c19` corrige CSRF bootstrap, nome do cliente, publicação atômica, retenção de FD e timeout do diff.
- Validação focalizada: `go test ./internal/workspace ./internal/programming ./internal/admin -count=1` e `node --check internal/admin/ui/admin.js` passaram. A cobertura é local/automatizada; não valida navegador, CI, MCP remoto, sandbox de processo ou escritores externos.
- Planejamento: ROADMAP recebeu M5/M6; TASKLIST recebeu SS-MVP-001…006; `docs/PROGRAMMING_TOOLS.md` define a fronteira local e seus limites; PROJECT_STATE foi vinculado a SS-MVP-006. SS-BE-006/007 permanecem nos estados anteriores.

## 21/09/2026 — primeira fronteira local de escrita (SS-MVP-002)

- Recuperação: `codex/mvp-vertical-programming` estava em `9067321`, sem upstream; `git fetch origin --prune` não trouxe divergência. O delta `d7111c8..9067321` passou `git diff --check`, e os cinco commits anteriores foram publicados por push normal em `origin/codex/mvp-vertical-programming`. O SHA remoto foi confirmado após o push; PR/merge não foram criados.
- Implementação focada em `internal/workspace/grants.go`: `Grant` passou a ser read-only; `GrantWithScopes` é uma fronteira local explícita e fechada para `ScopeRead`/`ScopeWrite`; `ReadText` e `ListDirectory` exigem leitura; `ReplaceText` exige escrita além de owner, cliente e sessão ativos. `Revoke`/`Close` limpam os escopos. Nenhum escopo novo foi ligado ao OAuth/MCP.
- Testes adicionados/reconciliados em `internal/workspace/grants_test.go`, `internal/workspace/edit_test.go` e `internal/programming/vertical_test.go`: read-only negado, write-only não lê, escrita autorizada, escopo desconhecido negado, identidade/cliente/sessão/revogação preservados e concorrência mantida.
- Validação: `gofmt`, `git diff --check`, testes de workspace/programming/MCP e race dos pacotes alterados passaram. `go test ./cmd/signalspace -parallel=1 -count=1 -timeout=90s` passou em 35,872s. A primeira execução agregada excedeu a espera de 30s e a segunda execução sem sessão deixou PIDs de teste em 7676; ambos foram encerrados de modo direcionado, sem listener residual.
- Limites: execução, Git/diff e UI continuam locais; não houve túnel, grant OAuth, exposição MCP de escrita, CI, navegador, merge, deploy ou alteração da branch `feat/frontend-oauth-consent`. SS-MVP-002 permanece em andamento até o contrato remoto por operação ser especificado e validado.

## 21/09/2026 — composição MCP isolada de escrita (SS-MVP-002)

- Recuperação: a branch `codex/mvp-vertical-programming` estava em `2ea209a8c7ad59cdcc725a9c0afdb394b4da6834`; `git fetch origin --prune` e `git pull --ff-only` não encontraram divergência. Os patches não rastreados, a branch `feat/frontend-oauth-consent` e o PR #1 draft foram preservados.
- Os commits `b46cda6` e `8fcc7c0` adicionaram `WorkspaceTextWriter`, `replace_text` e a autorização MCP no pacote interno. A porta de escrita foi mantida não exportada dentro de `OAuthConfig`, restringindo a composição aos testes; a revisão também tornou `tools/list` sensível ao scope, rejeitou `null` nos argumentos string e reforçou os testes MCP de cliente/owner ativos. O emissor OAuth, `connect quick`, `connect quick read` e os handlers de produção não receberam o escopo nem a ferramenta.
- `internal/mcp/workspace_write_test.go` exercita JWT/identidade assinada, scope write, owner/client/session/workspace, concessão ativa, revogação com token válido, token sem escopo, inválido/expirado, traversal, conflito, duplicação e `tools/list` público sem escrita. O resultado de falha é indistinto e não retorna caminhos.
- Validação completa: `gofmt`, `git diff --check`, `go test ./... -parallel=1 -count=1 -timeout=180s`, race dos pacotes afetados, `go vet ./...`, `go build ./...`, `node --check`, `node --test internal/auth/consent_js_test.mjs` (8/8), teste vertical e ausência de listeners/processos residuais passaram. O código foi publicado em `c85dd18c250671ed32e3fe28b`; o checkpoint documental final seguiu por push normal em `aa5fd27`; CI não foi consultada.
- Estado: SS-MVP-002 permanece **PARCIAL/em andamento**. A próxima fatia deve especificar o contrato remoto por operação antes de qualquer promoção da composição isolada para transporte público. Não houve Quick Tunnel, grant OAuth, workspace real, merge, force-push ou deploy.

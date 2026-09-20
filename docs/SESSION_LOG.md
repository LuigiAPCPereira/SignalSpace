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

## Limites do registro

Conector GitHub mostra arquivos versionados e metadados remotos, **não worktree/stashes locais**. Datas acima pertencem a registros observados; não inventar tempos de teste. Documento não substitui contratos, TASKLIST, Git/CI ou versões reais do Project/Codex/agendamentos.

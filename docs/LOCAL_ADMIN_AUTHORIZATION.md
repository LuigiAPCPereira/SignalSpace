# SignalSpace — contrato de autorização OAuth pelo painel local

**Status:** contrato de backend definido para implementação futura; **não implementado e não integrado ao frontend**. Escopo inicial: `connect quick` e `connect quick read` experimentais, exclusivamente na própria máquina. O modo OAuth persistente exige decisão de implantação adicional e não ganha painel por consequência. Contrato HTTP: `/api/admin/v1`. Data da decisão: 20/09/2026.

**Base examinada:** `feat/m1-local-mcp-diagnostic` em `f7c27d0`, `internal/auth/server.go`, `cmd/signalspace/main.go`, `internal/tunnel/quick.go`, `docs/WORKSPACE_SECURITY.md`, `AGENTS.md`, e revisão da frente de interface em [`docs/FRONTEND_ADMIN_CONTRACT_REVIEW.md`](https://github.com/LuigiAPCPereira/SignalSpace/blob/feat/frontend-oauth-consent/docs/FRONTEND_ADMIN_CONTRACT_REVIEW.md). A revisão do frontend identifica pendências, não descreve APIs existentes. O backend atual aprova pelo stdin; nenhuma rota administrativa, sessão de proprietário, pareamento ou consulta pública de estado descrita aqui foi implementada. Não alterar a branch do frontend para executar este contrato.

## 1. Decisões finais e invariantes

1. **Pareamento:** no primeiro acesso após iniciar o modo Quick, o proprietário informa segredo aleatório mostrado somente no terminal e define frase-senha. `POST /pair` consome o segredo e **cria imediatamente uma sessão administrativa autenticada**, com cookie e CSRF novos; não exige desbloqueio duplicado. Desbloqueios posteriores exigem frase-senha. No Quick, a credencial e as sessões vivem somente na memória: cada reinício exige novo pareamento. Recuperação explícita via terminal invalida credencial e todas as sessões.
2. **Sessão/CSRF:** `GET /api/admin/v1/session` informa somente estado mínimo antes de autenticar, cria/renova o bootstrap não privilegiado e fornece seu CSRF. Quando autenticado, devolve estado, prazos e CSRF da sessão administrativa. Trocar de bootstrap para sessão autenticada troca cookie e CSRF atomicamente. CSRF não é autenticação; nenhum segredo ou token é guardado em `localStorage`/`sessionStorage`.
3. **Leitura:** concessão de workspace **deve existir antes da criação** do pedido OAuth com `signalspace:workspace.read`, e ser revalidada antes de aprovar, concluir e emitir token. Um pedido sem concessão para o cliente não entra na fila. A aprovação OAuth não cria/seleciona pasta nem implica acesso MCP. Cada ferramenta continua verificando JWT, proprietário, `client_id`, concessão e `session_id`. Se a concessão for revogada, chamadas são negadas mesmo com JWT válido.
4. **Transporte:** primeiro protótipo em `http://localhost:7677`, com bind **somente** `127.0.0.1:7677`, autenticação própria e restrições fortes de origem. HTTP loopback é uma limitação documentada, não equivalente a HTTPS nem permitido para exposição remota/produção. O Quick Tunnel continuará apontando exclusivamente para `http://127.0.0.1:7676`; a porta administrativa nunca é um destino permitido. HTTPS local ou acesso remoto exigem contrato separado, sem downgrade automático.

A página OAuth pública **nunca aprova**; localhost **não autentica**; toda decisão pertence a ID e versão específicos; falhas negam acesso; não há aprovação individual de chamadas MCP. Tokens OAuth, cookies públicos OAuth, cookies administrativos e concessões de workspace são credenciais/domínios diferentes e não são intercambiáveis. `client_name` é declaração não atestada; `client_id` não prova software nem identifica conversa. Não habilitar escrita, Git ou shell em decorrência deste contrato.

## 2. Limites, superfícies e ownership

- **Público:** listener `127.0.0.1:7676`, roteador próprio com `/mcp`, descoberta OAuth, `/register`, `/authorize`, `/authorize/complete`, `/token`, `/oauth/jwks` e a consulta restrita `/authorize/status`. É a única origem do Quick Tunnel.
- **Administrativo:** listener independente `127.0.0.1:7677`, origem canônica `http://localhost:7677`, roteador próprio para HTML estático público sem dados e `/api/admin/v1/*`. Nenhuma rota administrativa, material de bootstrap, cookie, recurso estático administrativo ou alias deve ser registrado no handler público. O HTML administrativo inicial não inclui pedidos, credenciais ou estado hidratado.
- **Aplicação:** um único serviço de autorização é proprietário da fila, expiração, transições atômicas, decisões e códigos. O terminal e a API administrativa chamam a **mesma operação** de decisão; não existem regras duplicadas no handler ou frontend.
- **Workspace:** serviço de concessões separado; a API de decisão consulta a concessão existente, jamais cria concessão ou decide por ferramenta. Nesta fase, a concessão de workspace continua exigindo o mecanismo local já implementado. A futura migração desse fluxo ao painel requer contrato próprio.

No ponto de composição, reservar os dois listeners antes de iniciar/publicar o túnel. Se o modo painel foi selecionado e a porta 7677, a autenticação ou o listener falharem, abortar publicação, sem fallback silencioso para servidor único ou terminal-only. O modo terminal-only existente permanece selecionável explicitamente. Verificar a origem fixa do túnel no código/configuração; recusar qualquer configuração que aponte para `7677`, `0.0.0.0`, interfaces externas ou encaminhamento do painel. Não confiar em IP de proxy, `Forwarded` nem `X-Forwarded-*` como autenticação.

**Modelo de ameaça e limites:** autenticação de proprietário resiste a outra página da web e a requisições não autenticadas; Host exato, ausência de CORS e CSRF reduzem DNS rebinding/CSRF. Outro processo executado como o mesmo usuário, um navegador comprometido ou um proxy externo arbitrário que exponha deliberadamente 7677 e reescreva cabeçalhos estão fora das garantias do protótipo. Não prometer inacessibilidade diante de toda configuração arbitrária: detectar os caminhos de publicação controlados pelo SignalSpace, negar por padrão e preservar autenticação mesmo se houver encaminhamento indevido. Não habilitar o painel em modo persistente, hospedagem multiusuário, contêiner com rede compartilhada ou acesso remoto sem nova revisão.

## 3. Credencial do proprietário, pareamento e sessão

- Na inicialização Quick, gerar segredo de pareamento com CSPRNG (no mínimo 128 bits de entropia), exibi-lo apenas no terminal local, uma única vez, nunca em URL, corpo público, HTML, logs persistentes, métricas ou API. Prazo de cinco minutos; três erros o invalidam e exigem novo pareamento explícito no terminal. Não há endpoint HTTP para emitir ou recuperar esse segredo. Validar a frase-senha com limites documentados de tamanho e armazenar somente derivação lenta com sal (Argon2id ou equivalente revisado), na memória privada da instância Quick; não reutilizar chave OAuth, token MCP ou `identity.json`. Limitar tentativas de desbloqueio (proposta: cinco falhas por janela de quinze minutos, bloqueio de quinze minutos), usando cota global e por sessão/instância sem confiar em IP encaminhado. Senhas inválidas não revelam detalhes da credencial.
- `POST /pair`: requer bootstrap válido, CSRF e segredo correto dentro do prazo; configura a credencial, consome o segredo e emite sessão autenticada **na mesma transação**. Sessões e credenciais existentes não podem ser sobrescritas por um segundo pareamento. Se a resposta se perder, consultar `/session`: cookie autenticado confirma a sessão; sem cookie, não inferir sucesso. Se o segredo já foi consumido e não houver sessão recuperável, usar recuperação pelo terminal, nunca reabrir pareamento automaticamente.
- `POST /unlock`: apenas após pareamento, requer bootstrap/CSRF e frase-senha válida; emite uma nova sessão administrativa, invalidando o bootstrap. `POST /lock` revoga imediatamente a sessão atual, elimina seu cookie, cria novo bootstrap e devolve estado bloqueado. Recuperação pelo terminal e shutdown revogam todas as sessões.
- IDs de sessão: aleatórios opacos com pelo menos 256 bits, gerados pelo backend; armazenar somente representação derivada/hash em memória quando viável. Cookie host-only **sem atributo `Domain`**, nome exclusivo `signalspace_admin_session`, `Path=/api/admin/v1`, `HttpOnly`, `SameSite=Strict`. Cookie de bootstrap distinto `signalspace_admin_bootstrap`, mesmas restrições; nunca aceitar ambos como autoridade simultânea. A origem pública do túnel tem hostname diferente; cookies não devem ser configurados para `trycloudflare.com`. Como o protótipo utiliza HTTP, a propriedade `Secure` não pode ser assumida como proteção efetiva: configurar cookies de HTTP local sem alegar confidencialidade de transporte. Não utilizar prefixos de cookie que exijam `Secure` nesse modo. Migração HTTPS requer mudança explícita de política, cookies `Secure` e novos testes de navegador.
- Sessão autenticada: prazo ocioso de quinze minutos; prazo absoluto de sessenta minutos, sem extensão do prazo absoluto. `GET /session`, polling de pedidos e `/authorize/status` **não renovam** o prazo ocioso. Para atividade deliberada, `POST /api/admin/v1/session/refresh` requer sessão autenticada, Origin e CSRF, renova somente o prazo ocioso até o limite absoluto. Somente interação real da UI deve acioná-lo; nunca temporizador/polling. Decisão administrativa bem-sucedida também atualiza a atividade. O servidor verifica prazos em toda chamada protegida, independentemente dos timers do navegador. Lock, expiração, reinício e recuperação invalidam cookie e CSRF anteriores.
- Bootstrap: cookie opaco, efêmero, não privilegiado, criado por `GET /session`, TTL de cinco minutos, com token CSRF aleatório vinculado no servidor àquele bootstrap. O bootstrap expira independentemente do código de pareamento. Novas consultas dentro do prazo não rotacionam CSRF; depois do prazo criam outro bootstrap sem emitir outro segredo de pareamento. `pair` e `unlock` consomem o bootstrap atomicamente. Uma sessão administrativa nova recebe CSRF diferente, vinculado ao ID da sessão; um `GET /session` autenticado não rotaciona CSRF. `lock` e reinício rotacionam a fronteira de sessão. Tokens CSRF retornam apenas em JSON na origem administrativa e são enviados em `X-CSRF-Token`; nunca em URL, logs, cookie acessível por JS ou armazenamento do navegador.
- Segurança de requisição: verificar `Host` exato `localhost:7677` **em todos** os caminhos administrativos e recusar cabeçalhos Origin presentes de outra origem. Para `POST`, exigir `Origin: http://localhost:7677` exata, token CSRF válido em comparação constante, `Content-Type: application/json`, JSON estrito e limites de corpo. Não permitir CORS de terceiros, credenciais cross-origin, JSONP, GET mutável ou override de método; recusar preflight cross-origin. Opcionalmente validar `Sec-Fetch-*` como defesa adicional, nunca única. CSP restrita para painel, `frame-ancestors 'none'`, `Referrer-Policy: no-referrer`, `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`; sem scripts/fontes externos no painel. Endpoints desconhecidos sempre 404 sem fallback ao roteador público.

## 4. Contrato HTTP administrativo v1

Todas as rotas abaixo existem **somente** em 7677. `GET /session` é a única API de bootstrap sem autenticação; ainda exige Host exato. Os demais GETs exigem sessão autenticada. Nunca aceitar JWT OAuth ou cookie OAuth como sessão administrativa.

| Método e rota | Requisição e resultado |
| --- | --- |
| `GET /api/admin/v1/session` | Sem cookie: `200 UNPAIRED` ou `200 LOCKED`, cria bootstrap e retorna seu CSRF. Cookie inválido/expirado: `401 AUTH_REQUIRED`, remove cookie inválido e fornece estado mínimo/novo bootstrap. Cookie autenticado: `200 AUTHENTICATED`, prazos absolutos e de inatividade e CSRF da sessão. Não renova inatividade. |
| `POST /api/admin/v1/pair` | JSON `{ "pairing_code": "...", "passphrase": "..." }`, bootstrap+CSRF; `201`, cookie autenticado e modelo `AUTHENTICATED`. Não há desbloqueio subsequente. |
| `POST /api/admin/v1/unlock` | JSON `{ "passphrase": "..." }`, bootstrap+CSRF; `200`, novo cookie autenticado e modelo `AUTHENTICATED`. |
| `POST /api/admin/v1/lock` | JSON `{}`, sessão autenticada+CSRF; `200`, revoga sessão e retorna modelo mínimo `LOCKED` e novo bootstrap/CSRF. |
| `POST /api/admin/v1/session/refresh` | JSON `{}`, sessão autenticada+CSRF; `200`, renova apenas inatividade e retorna modelo autenticado. Não executar por polling. |
| `GET /api/admin/v1/requests` | `200` com `{ "requests": [...], "server_time": "...", "next_poll_after_ms": 2000 }`. Ordenação estável por `created_at` decrescente e desempate por ID; máximo de 32 pendentes e 64 estados terminais retidos, com resposta limitada. `[]` é vazio real somente se GET bem-sucedido. |
| `GET /api/admin/v1/requests/{id}` | `200` com o modelo atual, ou `404` após limpeza definitiva. Nunca interpretar 404 como expiração confirmada. |
| `POST /api/admin/v1/requests/{id}/decision` | JSON `{ "decision": "approve" | "deny", "expected_version": 1 }`; sessão+CSRF+Origin; `200` com modelo atualizado `APPROVED` ou `DENIED`. Não significa código/token emitido. |

### Modelo de `GET /session`

Sem autenticação (`200`), inclusive antes do pareamento:

```json
{
  "state": "UNPAIRED",
  "authenticated": false,
  "csrf_token": "opaque-bootstrap-csrf",
  "bootstrap_expires_at": "2026-09-20T14:05:00Z",
  "server_time": "2026-09-20T14:00:00Z"
}
```

Após pareamento, o mesmo schema retorna `state: "LOCKED"`. Nenhum dos dois modelos contém ID de cliente, pedidos, sessão administrativa, grants, caminho local ou segredo de pareamento. A presença de bootstrap/CSRF **não** autoriza chamadas protegidas. Para cookie administrativo inválido/expirado, `401` devolve o envelope de erro da seção 7 mais `session` com esse modelo mínimo; a interface limpa dados anteriores e recupera o bootstrap sem inferir autenticação.

Autenticado (`200`, também como corpo de `pair`, `unlock` e `refresh`):

```json
{
  "state": "AUTHENTICATED",
  "authenticated": true,
  "csrf_token": "opaque-admin-csrf",
  "idle_expires_at": "2026-09-20T14:15:00Z",
  "absolute_expires_at": "2026-09-20T15:00:00Z",
  "server_time": "2026-09-20T14:00:00Z"
}
```

As datas são exemplos ilustrativos UTC, não valores vivos. Cookies são enviados exclusivamente por `Set-Cookie`; nunca há `session_id`, hash, segredo de pareamento ou frase-senha no corpo. `server_time` evita que a UI invente a noção de prazo a partir do próprio relógio. O frontend não deve tratar o sucesso de um botão ou um JSON antigo como identidade: `/session` e a validação em cada endpoint são a autoridade.

### Modelo de solicitação

```json
{
  "id": "opaque-request-id",
  "version": 1,
  "status": "PENDING",
  "client": {
    "id": "registered-client-id",
    "display_name": "Nome declarado pelo cliente",
    "verified": false
  },
  "redirect_uri": "https://chatgpt.com/example-callback",
  "scope": "signalspace:diagnostic signalspace:workspace.read",
  "workspace_read": { "required": true, "grant_status": "ACTIVE" },
  "created_at": "2026-09-20T14:00:00Z",
  "expires_at": "2026-09-20T14:05:00Z",
  "decided_at": null
}
```

`scope` e `redirect_uri` são os **valores exatos armazenados e validados pelo OAuth**, não strings substituíveis pelo frontend. `workspace_read.required=false` e `grant_status="NOT_APPLICABLE"` para diagnóstico; para leitura, `ACTIVE` ou `REVOKED` segundo a concessão atual. Não retornar a raiz absoluta nem o `session_id` da concessão. `version` começa em 1, sobe em cada transição de estado observável e **não** sobe por polling ou mudança de relógio. A perda/revogação de um grant pode alterar `grant_status` sem mudar o estado OAuth; neste caso a resposta do GET expõe estado atualizado do grant, mas toda decisão revalida o grant dentro da operação. `decided_at` passa a data UTC após decisão, senão `null`. Estado terminal retido por até dez minutos, máximo 64 entradas; depois `404`. O backend precisa de tombstones limitados para responder `EXPIRED`/`410` antes da limpeza; isso não existe no código atual.

## 5. Máquina de estados e decisão atômica

**Sessão administrativa:** `UNPAIRED -> AUTHENTICATED` via pareamento; `LOCKED -> AUTHENTICATED` via desbloqueio; `AUTHENTICATED -> LOCKED` via lock, timeout, recuperação ou restart; restart no modo Quick também remove a credencial, portanto retorna a `UNPAIRED`. Bootstrap é estado de segurança separado sem privilégios. Ausência de resposta de pareamento ou unlock é **resultado desconhecido**: consultar `/session` e confiar apenas no estado retornado e no cookie correspondente.

**Solicitação OAuth:** `PENDING -> APPROVED | DENIED | EXPIRED`; `APPROVED -> COMPLETED | EXPIRED`; `DENIED`, `EXPIRED` e `COMPLETED` não aceitam decisão. `COMPLETED` significa que `/authorize/complete` criou um código OAuth, **não** token trocado ou conexão MCP. O código tem lifecycle independente: criado -> resgatado uma vez ou expirado; o status `REDEEMED` não faz parte do modelo da solicitação v1 porque a API atual não mantém vínculo persistente entre código resgatado e registro de consentimento.

A transição de decisão verifica sob sincronização: sessão administrativa/CSRF, ID exato, `expected_version`, pedido `PENDING`, prazo ainda futuro e, no caso de `approve` com leitura, concessão corrente do **mesmo cliente**. A negativa pode ocorrer mesmo se o grant foi revogado. Apenas uma decisão vence corrida entre painel, terminal, expiração e outra requisição; falha não altera estado. Para pedidos de leitura, o backend atual já verifica grant em `/authorize`, `/authorize/complete` e `/token`; **a checagem na decisão administrativa ainda precisa ser implementada**. Uma troca de concessão não concede pasta por OAuth: o MCP continua exigindo a concessão corrente e o ID da sessão. O JWT OAuth atualmente não fica criptograficamente vinculado à geração específica de uma concessão; não alegar isolamento por geração ou aprovação de uma raiz específica.

`POST /decision` **não** cria código nem redirecionamento. Sucesso `200` confirma somente `APPROVED` ou `DENIED`. Repetição, versão divergente ou solicitação encerrada retorna conflito e nunca reaplica efeito; não implementar retry automático de POST. Se a resposta ou conexão se perder, buscar `GET /requests/{id}` e inspecionar versão/estado; `404` após limpeza não prova qual decisão houve. O terminal e o painel usam a mesma máquina de estados, inclusive ao disputar a mesma solicitação.

## 6. Página OAuth pública e atualização de estados

`GET /authorize/status?request_id=<opaque-id>` será uma **consulta de estado não administrativa**, acessível somente com cookie `signalspace_auth` válido, ligado por hash à solicitação exata, e ID validado. O endpoint público não devolve `client_id` de terceiros, grant, segredo, CSRF administrativo, código ou access token. Resposta `200` mínima `{ "status": "PENDING", "server_time": "...", "expires_at": "..." }`, com `status` em `PENDING | APPROVED | DENIED | EXPIRED`; `403 access_denied` para cookie errado/ausente, `404` para pedido não encontrado após limpeza. `Cache-Control: no-store`; rejeitar Origin externo quando presente e não habilitar CORS. Limite de polling proposto: aproximadamente 2 segundos, sujeito a cota. A consulta não toma decisões nem prolonga expiração.

Quando a página recebe `APPROVED`, ela pode oferecer a conclusão existente `POST /authorize/complete` com seu cookie público e CSRF próprio; o servidor revalida prazo, concessão de leitura e demais condições. Apenas essa conclusão gera código e redireciona ao callback registrado. Evitar dois POST simultâneos: a conclusão consome o pedido sob mutex; a segunda falha sem emitir outro código. `DENIED` e `EXPIRED` jamais emitem código; manter a semântica atual de erro genérico em `/authorize/complete` nesta fase, sem alterar o callback OAuth sem teste separado. Fechar página, perder rede ou receber resposta incerta não provoca aprovação implícita; não repetir POST de conclusão cegamente. Cookies OAuth públicos e administrativos têm nomes, origens e funções separados.

A API administrativa usa polling de `GET /requests` aproximadamente a cada dois segundos e GET pontual após decisão, bloqueio e retorno de rede. Evento opcional `OnRequest` é apenas otimização; o servidor é a fonte de verdade. Ausência de resultado por `401`, `403`, `429`, `503` ou falha de rede não significa lista vazia. A leitura dos pedidos nunca estende sessão por polling.

## 7. Contrato de erros e perda de resposta

Envelope padrão administrativo, sempre `Cache-Control: no-store`:

```json
{
  "error": {
    "code": "STALE_REQUEST",
    "message": "A solicitação mudou; consulte o estado atual."
  },
  "server_time": "2026-09-20T14:01:00Z"
}
```

| HTTP | Código | Comportamento/garantia |
| --- | --- | --- |
| 400 | `INVALID_REQUEST` | JSON inválido, campos extras, ID/versão/decisão inválidos; não alterar estado. |
| 401 | `AUTH_REQUIRED` | Sessão ausente, inválida ou expirada em rota protegida; apagar dados administrativos da UI. Em `/session` com cookie obsoleto, retornar bootstrap mínimo novo. |
| 403 | `ACCESS_DENIED` | Host/Origin, CSRF, sessão sem autoridade ou credencial rejeitados; negar sem divulgar segredo. |
| 404 | `REQUEST_NOT_FOUND` | ID desconhecido ou tombstone removido; **não inferir expiração**. |
| 409 | `ALREADY_DECIDED` | Estado já decidido/concluído, sem efeito adicional. |
| 409 | `STALE_REQUEST` | `expected_version` diferente; recuperar via GET. |
| 409 | `WORKSPACE_GRANT_REQUIRED` | `approve` de leitura sem grant atual do mesmo cliente; pedido não é aprovado. O `/authorize` público existente permanece `403 access_denied` sem grant. |
| 410 | `REQUEST_EXPIRED` | Prazo vencido com registro terminal ainda retido; não emitir código. |
| 415 | `UNSUPPORTED_MEDIA_TYPE` | POST fora de `application/json`. |
| 429 | `RATE_LIMITED` | Excesso de tentativas de pareamento, desbloqueio, polling ou ações; `Retry-After`. |
| 503 | `TEMPORARILY_UNAVAILABLE` | Falha interna; resultado de POST pode ser desconhecido se o cliente perdeu a resposta. Consultar GET antes de agir. |

Falha de rede antes/depois do commit da decisão e resposta HTTP perdida são **resultado desconhecido** até consultar estado atual. Não repetir automaticamente pareamento, desbloqueio, decisão nem conclusão OAuth. `200` de decisão significa somente registro da decisão; não implica código, token, MCP conectado nem workspace criado. Sem cookie autenticado após pareamento perdido, recuperação via terminal, não presumir que o código voltou a ser válido. Todas as respostas negadas são sem dados privados de outros pedidos, segredos ou stack traces.

## 8. Matriz obrigatória de testes negativos e regressão

1. Listeners reais: porta 7676 e túnel recusam `/admin`, `/api/admin/v1/*`, pareamento, desbloqueio, decisão e qualquer recurso administrativo, inclusive caminhos codificados, barras duplicadas, método alternativo e Host adulterado; todas as rotas administrativas na porta pública retornam `404` sem `Set-Cookie` administrativo, mesmo se o cliente enviar cookies. O túnel nunca alcança 7677; configuração de origem incorreta aborta o startup.
2. `7677` só aceita bind loopback e `Host: localhost:7677`; DNS rebinding, host externo, origem cruzada, `Origin` ausente em POST, CORS/preflight e `Forwarded` forjado não autorizam. Proxy externo/host rewrite não é meio suportado de implantação.
3. Bootstrap ausente, CSRF ausente/trocado/de outra sessão, segredo errado, reuso, expiração e três falhas: pareamento negado. Segundo pareamento não substitui credencial; recuperar só pelo terminal. Tentativas de desbloqueio incorretas respeitam bloqueio, sem vazamento de frase-senha ou hash.
4. Pareamento válido emite **uma sessão autenticada imediatamente**; não pede unlock adicional. Pareamento/unlock concorrentes e resposta perdida reconciliam via GET. Cookie OAuth, JWT MCP ou bootstrap não autenticam API administrativa; cookies anteriores não funcionam após troca de sessão.
5. Sessão revogada, expirada (idle e absoluta), bloqueada, recuperada ou após restart: GET de pedidos e POST de decisão retornam `401`; `GET /session` expõe apenas bootstrap sem dados. GET/polling não prolonga prazo; refresh explícito não estende absoluto; token CSRF muda em pareamento, unlock e lock.
6. Solicitações concorrentes A/B: ID de A não decide B; campos `scope`, `client_id`, `redirect_uri` enviados pelo frontend são rejeitados/ignorados estritamente, nunca sobrescrevem o snapshot. Nome declarado, URIs e escopos permanecem dados não confiáveis. `expected_version` inválida/antiga, decisão duplicada, duas decisões simultâneas, expiração concorrente e terminal contra painel produzem no máximo uma transição.
7. Pedido de leitura sem concessão é recusado em `/authorize` **antes** de entrar na fila. Grant revogado antes de aprovação administrativa retorna `WORKSPACE_GRANT_REQUIRED`; revogado entre aprovação/conclusão/token impede emissão; após JWT emitido, leitura/listagem negadas pela concessão revogada. OAuth de diagnóstico não acessa workspace. OAuth não cria grant nem escolhe raiz.
8. Página pública sem cookie, cookie de outro pedido ou ID inventado não observa pedido. Mesmo com cookie válido, `/authorize/status` não aprova, não revela dados administrativos e não emite código. `POST /authorize/complete` antes de aprovar, duplicado, expirado, negado ou sem grant não emite código; código trocado duas vezes é rejeitado.
9. `GET /requests` autenticado vazio retorna `[]` somente em sucesso; `401`, `429`, `503` e rede indisponível nunca equivalem a vazio. Limite de pendentes/terminais, retenção e tombstones são respeitados; `410` somente enquanto expiração é conhecida, depois `404` sem conclusão falsa.
10. Reinício limpa sessões administrativas, bootstrap, frase-senha Quick, pedidos OAuth pendentes e códigos não persistidos; não revive decisão anterior. Falha de bind 7677 ou autenticação aborta modo painel sem publicar túnel; falha de abertura automática do navegador não cria aprovação e permite endereço manual.
11. Executar os testes existentes `go test ./...` e, quando houver implementação, CI de formato, `go test -race` nos pacotes afetados, `go vet ./...` e `go build ./...`; adicionar testes do roteamento e navegador antes de afirmar integração real. O smoke `list_directory` relatado pelo proprietário em 20/09/2026 não valida o painel administrativo, que ainda não existe.

## 9. Responsabilidades e entrega

## 10. Relação com a arquitetura de autorização local v2

Este contrato descreve a fronteira administrativa v1 e permanece a referência do comportamento histórico/atual até uma migração implementada. A ADR [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md), vinculada a `SS-MVP-002-LOCAL-AUTHORIZATION-V2-DESIGN-001`, aceita como alvo uma separação mais explícita: OAuth autentica a composição (`diagnostic`, `read` ou `programming`) e o painel/local owner decide capabilities, policies e permits.

Essa decisão não altera as rotas v1, não cria painel funcional adicional, não muda o mecanismo de terminal vigente e não concede escrita, Git ou shell. Em particular, a concessão de workspace continua distinta de OAuth; uma futura aprovação `REQUIRE_APPROVAL` deve produzir zero efeito e exigir repetição da tool com revalidação. A implementação de token family, Policy Engine, approvals persistentes e migração é posterior e requer tarefas próprias.

**Backend:** listeners, roteadores, credencial/pareamento, sessões/cookies/CSRF, validação de Host/Origin, esquema e respostas HTTP, quotas, relógio e retenção, estado OAuth atômico, isolamento do túnel, autorização de leitura e testes de segurança. **Frontend (outra branch/sessão):** HTML, CSS, JavaScript, acessibilidade, polling e experiência; não decide autorização nem armazena credenciais em Web Storage. Não modificar `feat/frontend-oauth-consent` e não integrar protótipos nesta entrega. Fase seguinte: implementar e verificar backend contra este documento, revisar contrato com frontend, adaptar protótipos e executar integração separada. Nenhum merge, deploy, painel de chamadas MCP, concessão de workspace via OAuth ou ampliação de capacidades está autorizado por este documento.

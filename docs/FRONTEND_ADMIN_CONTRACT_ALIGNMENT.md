# SignalSpace — alinhamento de UX com o contrato administrativo local

**Estado:** aceitação de interface e semântica **para preparação do frontend**, baseada no contrato de backend em `5e11e4c04b9b2a2fabf1cdb67831efcfef3406e7` (`docs/LOCAL_ADMIN_AUTHORIZATION.md`). **Não constitui aceite de segurança, confirmação de API funcionando ou autorização de integração.** Branch exclusiva: `feat/frontend-oauth-consent`. Data: 20/09/2026.

## 1. Decisões fechadas para desenhar a interface

- Primeira inicialização do modo Quick: pareamento pelo código que o processo mostra no terminal, mais definição de frase-senha. `POST /api/admin/v1/pair` confirma com `201 AUTHENTICATED`, cookie e CSRF novos; **não pedir a frase-senha uma segunda vez**. Cada reinício do Quick exige novo pareamento porque credencial e sessões são efêmeras.
- Desbloqueios posteriores: `POST /api/admin/v1/unlock` com a frase-senha e bootstrap válido; `200 AUTHENTICATED` gera sessão nova. `POST /lock` revoga sessão e volta ao estado mínimo bloqueado. Recuperação de pareamento comprometido ocorre somente via terminal local.
- `GET /api/admin/v1/session` é a fonte de verdade: `UNPAIRED`, `LOCKED` ou `AUTHENTICATED`; antes de autenticar só retorna estado mínimo, CSRF de bootstrap e tempos públicos dessa sessão não privilegiada. Credenciais nunca aparecem no HTML inicial, dados de hidratação, URL, logs ou Web Storage. Após autenticar, usar apenas os dados recebidos da API; o cookie administrativo `HttpOnly` não precisa ser lido pelo JS.
- Idle de 15 min e prazo absoluto de 60 min são regras **contratadas**, ainda não observadas no runtime. GET e polling não renovam o idle. Somente ação deliberada do proprietário pode disparar `POST /session/refresh`; nenhuma renovação invisível por timer. Não simular contagem regressiva baseada apenas no relógio local.
- Público: OAuth/MCP em 7676; administração em `http://localhost:7677` com bind somente 127.0.0.1. A página pública pode ler o estado de **sua** solicitação por `/authorize/status` com cookie OAuth correspondente; não recebe sessão nem API administrativa. HTTP loopback é experimental, não seguro para exposição remota.
- Leitura: grant do mesmo cliente é pré-requisito **antes de criar** pedido OAuth com `signalspace:workspace.read`. Se faltar, não há solicitação para mostrar na fila; conceder workspace segue o fluxo local existente, separado do painel OAuth. Se revogado durante a espera, aprovar pode receber `409 WORKSPACE_GRANT_REQUIRED`; recusar continua possível. OAuth nunca escolhe pasta nem autoriza chamadas MCP individualmente.

## 2. Tela a tela e fronteira de dados

| Tela/estado | Fonte contratada | Comportamento do frontend |
| --- | --- | --- |
| Carregando sessão | GET `/session` ainda pendente | Nenhum detalhe administrativo ou botão de decisão. |
| Primeiro pareamento | `200 UNPAIRED` e bootstrap | Explicar código do terminal (válido por 5 min) e definição de frase-senha; código e frase-senha só serão coletados pela UI funcional na origem administrativa; laboratório não coleta. |
| Pareamento enviado | POST `/pair` pendente | Desabilitar envio; sem sucesso otimista. Resposta perdida -> consultar GET `/session`; sem sessão autenticada não presumir reuso do código. |
| Desbloqueio | `200 LOCKED` e bootstrap | Pedir apenas frase-senha, não código de pareamento. Erro/limite não revela se credencial existe. |
| Autenticado | `/session` retorna `AUTHENTICATED` | Buscar `/requests`, então exibir pedidos reais. Não reutilizar dados de outra sessão. Oferecer bloqueio manual e refresh de sessão somente com ação do proprietário. |
| Sem pedidos | GET `/requests` 200 com `requests: []` | Vazio verdadeiro, diferente de falha de rede, 401, 429 e 503. |
| Pedido pendente | GET `/requests/{id}` com `PENDING` | Exibir nome declarado/não atestado, `client.id`, `redirect_uri`, `scope` exato, grant sem caminho privado, versão e prazo do servidor. |
| Decisão | POST `/requests/{id}/decision` | Enviar somente `decision` e `expected_version`; não reproduzir escopos/retorno em payload; desabilitar duplo envio. 200 confirma decisão, não código, token ou conexão MCP. |
| Conflito, grant ausente ou resposta perdida | `409` ou perda de resposta | Suspender ação e reconciliar GET por ID, jamais repetir POST automaticamente. `WORKSPACE_GRANT_REQUIRED` informa necessidade de concessão separada. |
| Sessão inválida/expirada | `401 AUTH_REQUIRED` | Descartar referências a pedidos e dados do DOM; usar somente modelo bootstrap mínimo; exigir desbloqueio/pareamento conforme estado retornado. |
| Pedido expirado/desconhecido | `410` ou GET `404` | 410 = expirado conhecido; 404 = pedido não encontrado, **não** afirmar expiração. |
| Indisponível/desconhecido | 429, 503 ou rede perdida | Não equiparar a vazio nem a autorização; manter decisões indisponíveis até nova fonte confiável. |
| OAuth público | GET `/authorize/status` da própria página | Mostrar PENDING/APPROVED/DENIED/EXPIRED; só após APPROVED permitir concluir pelo POST já validado no servidor; nunca aprovar a partir da página pública. |

Os nomes de rotas e respostas acima são **contrato escrito para implementação futura**, não endpoints atualmente disponíveis.

## 3. Política de protótipos e implementação posterior

- `frontend/consent/prototypes/local-approval-flow-v2.html` continua um laboratório de decisão, sem rede; `frontend/consent/consent.html` é o rascunho antigo orientado a terminal e **não deve ser integrado**.
- O estudo local `SignalSpace-owner-unlock-ux.html` não foi versionado anteriormente e não contempla as telas de pareamento/CSRF. Preparar laboratório autônomo de pareamento e desbloqueio compatível com os estados fechados, sem campos funcionais para coleta de segredo e sem chamadas de API.
- Após backend implementado e testado, frontend pode usar `fetch` same-origin com `credentials: 'same-origin'`, token CSRF somente em memória e `X-CSRF-Token` nas mutações JSON; **nenhuma chamada é introduzida pelo protótipo**. Limpar qualquer informação administrativa ao perder sessão; manter segredo/frase-senha apenas durante o envio quando houver UI funcional, nunca em armazenamento persistente.
- Consultas por polling usam o atraso `next_poll_after_ms` retornado pelo backend e não renovam sessão. `POST /session/refresh` apenas por gesto explícito. Perda de resposta de pair/unlock/decisão exige GET de reconciliação, nunca repetição cega.
- A consulta `/authorize/status` exige JavaScript na página pública, que antes tinha CSP restrita para template/formulário sem esse fluxo: backend deve revisar CSP (`script-src` seguro e `connect-src` somente própria origem, se aplicável), `form-action`, cookies, CSRF e testes de navegador. Não relaxar CSP com `unsafe-inline` por conveniência nem liberar CORS/admin para atender o frontend.
- Validar navegador com `localhost:7677` e listener em `127.0.0.1` inclusive resolução IPv4/IPv6 e redirects; não substituir por `127.0.0.1` no frontend sem revisar o `Host` e a política de cookie que exigem origem canônica exata.

## 4. Critérios de entrada para integração (ainda NÃO cumpridos)

1. Backend implementar listeners separados, endpoints, bootstrap, sessões, CSRF, idempotência de decisões e restrição do Quick Tunnel conforme o contrato versionado.
2. Backend provar testes negativos de Host/Origin, roteamento público, CSRF, expirados, corrida, perda de resposta, grants e emissão de token. PR #1 e CI de documentação não atestam essas novas APIs.
3. Frontend revisar comportamento efetivo de JSON/erros e adaptar a interface real sem dados administrativos antes do GET autenticado; testar teclado, zoom, leitor de tela e telas pequenas.
4. Rodar integração ponta a ponta em workspace descartável e obter autorização explícita antes de integrar branches ou fazer merge.

**Resultado desta revisão:** decisões de UX alinhadas ao documento `5e11e4c`; segurança e operação permanecem responsabilidade da implementação e dos testes do backend. Sem mudanças em Go, rotas, cookies, escopos, grants, merge ou deploy nesta branch.

# Frontend — consentimento OAuth

**Status:** proposta de frontend ainda não integrada. **Branch:** `feat/frontend-oauth-consent`. **Base inicial examinada:** `496802b337adf9761994e6ae0e765a007f509190`, derivada do PR #1 em 20/09/2026.

**Decisão de experiência atualizada:** a aprovação da conexão deve ocorrer em um **painel local protegido do proprietário**, sem comando de terminal para cada solicitação. A página pública somente informa o pedido e acompanha a decisão. Consulte [Painel local de aprovação OAuth](LOCAL_OAUTH_APPROVAL_PANEL.md), que é a referência para a nova direção de UX. Nenhum fluxo foi integrado ou implementado no backend.

## Evidência observada na base

- `internal/auth/server.go` renderiza um template HTML embutido em `GET /authorize` com dados `ID`, `Client`, `ClientID`, `Redirect`, `CSRF`, `Scope` e `Read`.
- `Read` distingue diagnóstico da combinação exata de diagnóstico com leitura. A concessão de pasta é independente do OAuth.
- No runtime examinado, a decisão confiável é efetuada pelo terminal local com `approve <id>` ou `deny <id>`; a página pública não aprova.
- O navegador faz `POST /authorize/complete` com `request` e `csrf`. O servidor retorna `409 authorization_pending` em JSON antes da decisão, `403 access_denied` após recusa ou expiração e redirecionamento `303` após aprovação válida.
- A solicitação expira em cinco minutos. Não há, na base examinada, contrato de consulta de estado no navegador nem autenticação do proprietário para um painel local.
- O nome do cliente é metadado não atestado; `client_id` não comprova que o software seja o ChatGPT.

## Artefatos da branch

- `frontend/consent/consent.html`: template Go `html/template` alternativo criado para o fluxo **antigo de terminal**. Não conectado ao runtime; preservado somente como referência de apresentação e de campos reais. **Não integrar como solução final sem revisão**, pois seu texto ainda orienta `approve <id>`.
- `frontend/consent/prototypes/local-approval-panel.html`: prévia visual estática, com dados demonstrativos e botões desabilitados. Não autentica, não aprova, não comunica com servidor e não pode ser publicada como painel administrativo.
- `docs/LOCAL_OAUTH_APPROVAL_PANEL.md`: definição do fluxo desejado, superfícies, estados, requisitos de segurança e dependências de contrato a discutir com backend antes de integrar.

## Direção de interação

1. A página pública apresenta solicitante não verificado, destino de retorno e somente os escopos efetivamente pedidos.
2. Solicita que o usuário examine a pendência no painel local do SignalSpace, sem oferecer autorização pela página pública.
3. O proprietário deve ser autenticado por mecanismo independente do pedido público; `localhost` sozinho não é autenticação. A escolha desse mecanismo permanece aberta e pertence à frente de backend, em colaboração com UX.
4. O painel exibe os dados reais e ações Autorizar conexão/Recusar para uma pendência específica. Nenhum sucesso é exibido antes da confirmação do backend.
5. A página pública apresenta um estado confirmado e conclui o OAuth somente segundo contrato seguro e testado. Não afirmar que o aplicativo já recebeu token ou que o MCP está conectado.

Não duplicar confirmações por chamada MCP oferecidas pelo ChatGPT. OAuth, concessão de workspace e decisão sobre uma chamada de ferramenta são conceitos diferentes. Não presumir que o ChatGPT pedirá confirmação para toda ferramenta.

## Estados e dependências

| Estado | Comportamento visual desejado | Condição para implementar |
| --- | --- | --- |
| Pendente | Página pública aguarda; painel mostra detalhes após autenticação | Contrato de consulta/notificação seguro |
| Proprietário não autenticado | Painel solicita autenticação, sem mostrar dados sensíveis | Mecanismo de autenticação local acordado |
| Aprovado | Painel mostra decisão confirmada; página pode continuar OAuth | Resultado verificável no backend |
| Recusado | Nenhum acesso concedido; mensagem clara | Resultado verificável no backend |
| Expirado | Ações indisponíveis; reiniciar solicitação | Expiração identificável sem inferência no frontend |
| Erro/indisponível/desconhecido | Não presumir autorização nem permitir ações inseguras | Semânticas diferenciadas do backend |

Uma resposta `409` ou `403` JSON do fluxo antigo não justifica simular estados novos no navegador. Não inventar endpoint, polling, duração de sessão, método de desbloqueio ou resposta HTML sem estudo e testes.

## Fronteira com a equipe de backend

Esta branch não altera `internal/auth`, rotas, cookies, CSRF, PKCE, escopos, concessões de workspace ou tokens. O backend deverá avaliar separação efetiva entre superfície pública e painel administrativo local, autenticação independente do proprietário, autorização por decisão específica, expiração, CSRF e proteção contra solicitações cruzadas. **Não abrir uma rota pública de aprovação** para facilitar a interface.

A janela local pode abrir automaticamente se o backend puder fazê-lo de modo seguro e opcional. Falha na abertura não autoriza nada. O objetivo é evitar voltar ao terminal em cada solicitação, sem enfraquecer as garantias existentes.

## Validação ainda pendente

- Revisar com backend os contratos de estados, segurança, autenticação de proprietário e abertura da janela local.
- Renderizar com dados reais escapados e sem expor sessão administrativa ao túnel.
- Testar autorização, recusa, expiração, sessão inválida, CSRF, cliente divergente e revogação de concessão.
- Inspecionar responsividade (320, 360, 390, 768 e 1440 px), zoom 200%, teclado, foco, textos longos e contraste AA.
- Rodar gates e smoke real com ChatGPT Web apenas após integração controlada.

**Fora do escopo desta entrega:** integração ao runtime, backend, novo servidor local, sistema de aprovação por chamada MCP, GitHub remoto, merge e deploy.
# UX de desbloqueio do painel local — SignalSpace

**Status:** proposta de frontend e laboratório demonstrativo, não integrada e sem autenticação implementada. **Data:** 20/09/2026. **Branch:** `feat/frontend-oauth-consent`.

## Objetivo e separação das decisões

Evitar o terminal **a cada solicitação OAuth**, sem confundir desbloqueio do painel com autorização da conexão. Primeiro o proprietário comprova controle por um mecanismo seguro a ser definido com o backend; só então pode ver detalhes administrativos e decidir **cada solicitação OAuth**. Autorizar conexão não autoriza automaticamente workspaces nem chamadas MCP individuais. As confirmações de ferramentas que o ChatGPT apresentar pertencem à sua própria interface.

A página pública `/authorize` continua sendo a superfície de apresentação e conclusão OAuth, **nunca** a prova de identidade do proprietário. A superfície administrativa local não pode ser exposta pelo Quick Tunnel. `localhost` restringe localização de acesso, mas não é autenticação. Não assumir cookies, senhas, PIN, biometria, links mágicos, pareamento ou endpoints não implementados.

## Prévia criada e significado

O arquivo local `SignalSpace-owner-unlock-ux.html` foi preparado para revisão visual; seu conteúdo ainda **não está versionado nesta branch**. Usa dados fictícios, estado apenas no DOM e ações de laboratório sem credenciais, rede, armazenamento ou autorização. É propositalmente um estudo separado da V2 de decisões OAuth (`frontend/consent/prototypes/local-approval-flow-v2.html`). Não publicar nenhum destes protótipos como rota de produção.

Ao desbloquear ficticiamente, a prévia revela apenas dados demonstrativos e mantém os botões OAuth desabilitados: o objetivo aqui é revisar a fronteira de acesso, não criar um mecanismo de consentimento paralelo.

## Jornada proposta

1. O pedido OAuth remoto é validado pelo servidor; a página pública informa que aguarda decisão local, sem acesso ao painel administrativo.
2. O SignalSpace pode abrir a página **local** do proprietário quando houver um pedido, caso o backend confirme que isso é seguro e configurado. Falha ao abrir navegador exige recuperação confiável, não aprovação implícita.
3. Se o proprietário não tiver sessão válida, painel aparece bloqueado: não renderiza cliente, escopos, destino nem solicitação pendente. Informa que será necessária verificação independente do pedido OAuth; o método de verificação ainda é desconhecido.
4. Durante verificação, continuar bloqueado e preservar estado desconhecido como desconhecido. Nenhum clique em botão da UI é evidência de autenticação.
5. Somente após resultado autenticado e confirmado pelo servidor, expor dados reais daquela solicitação, prazo e ações distintas de `Autorizar conexão` e `Recusar`; antes de executar decisão, backend revalida sessão e pedido.
6. Sessão expirada, verificação rejeitada, serviço indisponível ou resposta perdida ocultam dados e desabilitam todas as decisões; nunca reaproveitar um estado visual desbloqueado como prova de sessão.

## Estados de UX e verdade exigida

| Estado | Dados de solicitações | Possibilidade de aprovar | Mensagem de UX |
| --- | --- | --- | --- |
| Bloqueado | Ocultos | Nenhuma | Comprove seu controle para acessar solicitações. |
| Verificação em andamento | Ocultos | Nenhuma | Aguarde confirmação real; envio não é sucesso. |
| Acesso confirmado | Visíveis somente após consulta autenticada | Depende também de pedido válido | Revise cada pedido, permissões e destino separadamente. |
| Verificação recusada | Ocultos | Nenhuma | Não foi possível desbloquear; nova tentativa segundo contrato. |
| Sessão expirada | Ocultos | Nenhuma | Renove acesso; ações antigas não permanecem válidas. |
| Serviço indisponível | Ocultos | Nenhuma | Sem resposta confiável, não decidir. |
| Resultado desconhecido | Ocultos | Nenhuma | Não sabemos se verificação concluiu; reconciliar sem presumir acesso. |

Se não houver pedidos **após** autenticação e consulta bem-sucedida, exibir vazio real. Isso não é o mesmo que sessão expirada ou falha ao consultar. Expiração de solicitação OAuth, sessão do proprietário, token OAuth e revogação da concessão de workspace são eventos diferentes.

## Contrato a fechar com o outro chat (backend)

**Nenhum item abaixo descreve mecanismo já existente ou solicita implementação nesta fase.** A frente de backend precisa especificar: (a) prova inicial e posterior de controle do proprietário sem terminal a cada pedido; (b) isolamento efetivo da superfície administrativa contra túnel e acesso por terceiros, com autenticação, Host/Origin/CSRF, duração/invalidação de sessão e logout; (c) forma segura de consultar se há sessão válida e pedidos pendentes, e o que ocorre no restart; (d) vínculo imutável de decisão ao ID do pedido, cliente, escopos, destino e expiração; (e) códigos/estados distinguíveis para rejeição, expiração, indisponibilidade e resultado desconhecido; (f) recuperação segura de resposta perdida e de falha ao abrir o navegador. Não introduzir rota de aprovação pública nem ampliar escopos de leitura, Git, edição ou shell por conveniência visual.

## Critérios de aceitação do frontend após contrato real

- Antes da autenticação verificada, nem DOM, HTML inicial, dados de hidratação, logs ou chamadas públicas devem revelar detalhes administrativos. Ocultar com CSS é insuficiente: não enviar dados sensíveis a clientes não autenticados.
- Cada botão de decisão identifica visualmente o pedido e não produz sucesso otimista. O backend decide e confirma, inclusive em caso de sessões vencidas ou pedidos expirados.
- Textos de status não dependem só de cor. O foco vai ao título quando uma ação substitui controles, com `aria-live` sem repetição excessiva; botões e inputs são acessíveis por teclado e as telas se adaptam a 320–1440 px e zoom.
- Reavaliar o design com a resposta efetiva do servidor, testes negativos de segurança e chamada real via ChatGPT Web. Até lá, este documento é especificação de intenção e não garantia de funcionamento.

**Limites desta entrega:** nenhum arquivo de backend, rota, token, sessão, concessão, API, integração, merge ou deploy foi alterado.
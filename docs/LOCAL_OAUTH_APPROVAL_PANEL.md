# Frontend — painel local de aprovação OAuth (proposta)

**Status:** decisão de experiência e protótipo visual; não implementado, não conectado ao servidor e sem contrato de backend aprovado. **Data:** 20/09/2026. **Branch de trabalho:** `feat/frontend-oauth-consent`.

## 1. Problema e decisão

Na implementação examinada do PR #1, a página pública `/authorize` orienta digitar `approve <id>` ou `deny <id>` no terminal e, depois, clicar em Continuar. Isso cria uma troca de contexto desnecessária para uma operação de interface. O proprietário quer decidir em um **painel local protegido**, aberto no computador quando houver uma solicitação, com botões **Autorizar** e **Recusar**.

A proposta recupera a decisão de produto do histórico de frontend fornecido pelo proprietário: página pública somente apresenta e acompanha a solicitação; painel local é a superfície independente de decisão do proprietário; conclusão OAuth continua separada. **Não se trata de aprovação de chamada MCP:** confirmações por ferramenta do ChatGPT não serão duplicadas no SignalSpace.

## 2. Limites observados e o que permanece desconhecido

- O servidor da base `496802b` expõe um template Go `html/template` embutido com `ID`, `Client`, `ClientID`, `Redirect`, `CSRF`, `Scope` e `Read` para a página pública.
- A decisão confiável ocorre no terminal. Não existe endpoint público de aprovação; a página conclui via `POST /authorize/complete` e o servidor nega conclusão prematura.
- Não há contrato implementado de autenticação do proprietário no navegador, interface administrativa local ou consulta segura de estados pela página pública.
- O acesso de leitura exige concessão de workspace própria; OAuth não concede automaticamente arquivos. Nome declarado pelo cliente e `client_id` não atestam que o software seja o ChatGPT.
- Não está comprovado que qualquer método futuro de desbloqueio, abertura automática do navegador ou sincronização entre páginas funcione; tudo isso requer estudo e implementação na frente de backend.

**Não integrar `frontend/consent/consent.html` como solução definitiva:** seu comando de terminal representa o fluxo antigo e serve apenas como referência compatível com o contrato atual. Não o substituir silenciosamente antes de aprovar o fluxo de segurança do painel.

## 3. Duas superfícies com ownership distinto

### Página pública de consentimento (acessível pelo túnel)

Trabalho: informar qual conexão OAuth foi pedida e qual acesso seria concedido; indicar 'Aguardando confirmação no SignalSpace deste computador'. Exibe apenas dados necessários e seguros, sem credenciais, paths privados ou informações da sessão administrativa. Não tem botões capazes de aprovar/recusar, não aceita senha ou código de desbloqueio administrativo e não presume que seu visitante seja o proprietário. Após decisão comprovada pelo servidor, pode oferecer 'Continuar para o aplicativo' ou concluir o OAuth conforme contrato validado. Não atualizar um estado apenas por temporizador ou animação.

### Painel local do proprietário (não publicado pelo túnel)

Trabalho: apresentar solicitações OAuth pendentes ao proprietário **autenticado**. Deve exibir nome declarado/não verificado, `client_id`, destino de retorno, escopos exatos e tradução humana das permissões, ID da solicitação quando necessário e prazo real. Cada decisão se refere à solicitação selecionada, sem aprovação genérica. Botões 'Recusar' e 'Autorizar conexão'; feedback de envio sem sucesso otimista. Após resultado confirmado, exibir decisão e permitir retornar à lista/fechar. Se não houver solicitações, exibir estado vazio real.

A janela pode ser aberta no navegador local quando houver nova solicitação, **somente após o backend definir como descobrir, abrir e autenticar a sessão do proprietário de forma segura**. Falha ao abrir navegador não deve transformar a conexão em aprovada nem eliminar um fallback confiável. Não exigir terminal para *cada* aprovação é um objetivo de UX, mas o método de configuração inicial/desbloqueio ainda precisa ser decidido em conjunto com o backend.

## 4. Fluxo desejado (não implementado)

1. Um cliente inicia o OAuth na página pública; backend valida os parâmetros e cria uma pendência vinculada ao contexto correto.
2. SignalSpace notifica o processo/interface local e, se habilitado, abre o painel no computador do proprietário.
3. Se a sessão do proprietário não estiver autenticada, painel exige prova de controle independente da própria requisição pública. **Loopback/localhost não é autenticação.** Não assumir código de desbloqueio no terminal como solução permanente; avaliar pareamento inicial e autenticação local compatível com o produto.
4. O painel exibe dados reais daquela solicitação, inclusive aviso sobre nome de cliente não verificado e permissões de diagnóstico ou leitura.
5. Proprietário escolhe Autorizar ou Recusar. Backend revalida sessão, origem, CSRF, pendência, expiração e escopo antes de registrar a decisão; falha fecha o acesso.
6. Página pública obtém o estado de forma segura (mecanismo ainda não definido), apresenta a situação correta e só conclui OAuth mediante decisão válida. Conclusão OAuth não significa token emitido, aplicativo conectado ou ferramenta executada.

## 5. Estados de interface

| Estado | Página pública | Painel local |
| --- | --- | --- |
| Solicitação aguardando | Explica que decisão ocorre no computador; não aprova | Exibe detalhes reais e ações após autenticação |
| Proprietário não autenticado | Não expõe estado administrativo | Solicita autenticação por mecanismo ainda a definir |
| Aprovação em envio | Continua aguardando sem presumir sucesso | Bloqueia dupla submissão, anuncia processamento |
| Aprovado confirmado | Permite concluir OAuth conforme contrato | Exibe decisão confirmada, não 'ChatGPT conectado' |
| Recusado confirmado | Informa que acesso não foi concedido | Exibe decisão confirmada |
| Expirado | Informa que a solicitação não pode continuar | Desabilita ações e orienta reiniciar solicitação |
| Sessão administrativa expirada | Não revela dados privados | Solicita autenticação novamente, preservando segurança |
| Indisponível / estado desconhecido | Não oferece conclusão presumida | Não permite decisão até restabelecer verdade do servidor |
| Lista vazia | Não aplicável | 'Nenhuma solicitação pendente', sem simular atividade |

Recusa, expiração e indisponibilidade são resultados diferentes, **somente se o backend puder comprová-los**. Não introduzir polling, WebSocket, refresh automático ou novos endpoints por preferência estética.

## 6. Requisitos de segurança a acordar com backend antes de integrar

- Separação verificável entre interface pública do OAuth e administração local: não expor rotas administrativas no túnel, incluindo por proxy, rotas compartilhadas ou configuração incorreta. Loopback é restrição de transporte, não prova de identidade.
- Autenticação de proprietário independente do próprio pedido OAuth; sessão de proprietário com lifetime, logout/bloqueio e recuperação definidos; proteção de origem/Host, CSRF e medidas pertinentes contra acesso por outras páginas locais/remotas.
- Decisão vinculada a ID único, cliente, destino, escopos e prazo; uso único e idempotência/negação de duplicações; revalidação no momento da decisão e da conclusão.
- Não enviar tokens, segredos, sessão administrativa ou caminhos privados à página pública. Nome do cliente é texto não confiável, renderizado com escape.
- Concessão de pasta é independente da aprovação OAuth; acesso MCP continua exigindo validação no servidor por requisição. A confirmação de ferramentas no ChatGPT não é uma prova de autorização recebida pelo SignalSpace.
- Definir comportamento de negação, expiração, perda de conexão, restart e falha ao abrir navegador, sem contornar a aprovação local confiável atualmente existente.

Esses itens são **necessidades de contrato**, não endpoints ou controles de segurança já implementados nesta branch. Não escolher rotas ou mecanismo de autenticação apenas pelo protótipo.

## 7. Direção visual e composição

Uma janela pequena, com uma única tarefa e sem dashboard: cabeçalho SignalSpace, título 'Nova solicitação de conexão', aviso 'Nome informado pelo aplicativo · não verificado', seção de destino e escopos exatos, resumo compreensível das permissões, prazo real, ações Recusar (secundária) e Autorizar conexão (primária). Em largura reduzida, conteúdo flui em coluna única e botões são amplos. Tipografia de sistema, contraste legível, foco visível, estados textuais, sem animação necessária, sem componentes genéricos artificiais.

`frontend/consent/prototypes/local-approval-panel.html` é **apenas prévia estática**, com valores identificados como demonstração e controles desabilitados. Não autentica, não aprova, não consulta o servidor e não deve ser publicado como rota do SignalSpace.

## 8. Critérios de passagem do design para implementação

1. Frontend e backend concordam no fluxo, especialmente na prova de controle do proprietário sem terminal a cada solicitação.
2. Backend documenta separação de superfícies e contratos reais de estados e ações, com negativos de segurança.
3. Frontend consome exclusivamente os dados e estados disponibilizados, com escape, acessibilidade e responsividade revisados.
4. Testar no backend sessões inválidas, origem/CSRF, dupla decisão, expiração, cliente divergente, concessão revogada e inexistência de aprovação pela página pública.
5. Verificar o fluxo real com ChatGPT Web em ambiente descartável, sem considerar sucesso visual como sucesso de conexão.

**Fora do escopo desta entrega:** mudar `internal/auth`, criar servidor local administrativo, endpoints, tokens, sessão, polling, integrar HTML ao runtime, aprovação por chamada MCP, GitHub remoto e merge.
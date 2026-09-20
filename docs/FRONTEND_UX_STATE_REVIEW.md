# Revisão de UX — autorização OAuth local

**Status:** revisão do protótipo, sem integração e sem contrato de backend aprovado. **Data:** 20/09/2026. **Branch:** `feat/frontend-oauth-consent`.

## Objetivo

Validar a experiência planejada para autorizar uma **conexão OAuth** por um painel local protegido, sem comandos no terminal a cada solicitação. A interface pública é somente acompanhamento; a decisão confiável pertence ao proprietário autenticado no painel local. Não implementar uma fila de aprovação por chamada MCP: as confirmações de ferramentas, quando oferecidas pelo ChatGPT, pertencem ao ChatGPT e não substituem autorização no servidor.

## Artefatos da experiência

- `frontend/consent/prototypes/local-approval-panel.html`: referência estática original, com ações desabilitadas.
- `frontend/consent/prototypes/local-approval-flow.html`: protótipo **isolado e interativo**, com estados demonstrativos, seletor de situações e botões de *simulação*. Usa somente dados fictícios, DOM local e JavaScript sem chamadas de rede. Não autentica nem autoriza e **não deve ser montado como rota de produção**.
- `frontend/consent/consent.html`: template inicial compatível com o fluxo pelo terminal, preservado como referência anterior; não é a experiência-alvo com painel.

## Revisão da hierarquia

A tarefa central é compreender a decisão e seus limites. A ordem é: aviso inequívoco de demonstração (somente no protótipo), contexto do painel local, estado comprovado, solicitante identificado como nome declarado/não verificado, `client_id`, destino de retorno, permissão em linguagem humana, escopo exato, decisão e consequência. Dados e ações administrativas **não** devem ser copiados para a página OAuth pública. Não exibir ícone/marca do ChatGPT como prova de identidade.

O painel mantém uma tarefa por vez, sem dashboard, histórico de chamadas, métricas artificiais, notificação sonora ou indicadores animados. Reutiliza tipografia de sistema, hierarquia discreta, foco visível e adaptação para coluna única no celular.

## Matriz de estados do protótipo

| Estado simulado | Conteúdo principal | Ações disponíveis na prévia | Verdade exigida na implementação |
| --- | --- | --- | --- |
| Pendente | Solicitante, destino, escopos e aviso de identidade não atestada | Somente `Simular autorização` e `Simular recusa` | Pedido validado e sessão do proprietário autenticada |
| Painel bloqueado | Instrução genérica, sem dados da solicitação | Nenhuma ação de autorização | Autenticação independente do visitante público, ainda a definir |
| Decisão em envio | Detalhes preservados, sem afirmar resultado | Nenhuma | Resultado confirmado pelo servidor; evitar dupla submissão |
| Autorizado | Decisão OAuth confirmada; não diz “MCP conectado” | Nenhuma | Decisão registrada, sem pressupor token emitido |
| Recusado | Acesso negado e instrução de nova solicitação | Nenhuma | Recusa efetivamente registrada |
| Expirado | Pedido encerrado e necessidade de reiniciar | Nenhuma | Prazo calculado e imposto no backend |
| Indisponível | Nenhuma decisão permitida | Nenhuma | Falha de comunicação, não confundir com lista vazia |
| Desconhecido | Não presume sucesso nem falha após resposta perdida | Nenhuma | Reconciliação segura antes de repetir efeitos |
| Vazio | Nenhuma solicitação pendente | Nenhuma | Lista vazia verificada, não falha na consulta |

Os textos e dados exibidos são **somente cenários de demonstração**. A prévia não contém cronômetro porque não tem tempo de expiração de fonte confiável; não usa senha, código ou biometria fictícios. Um estado aprovado pelo backend não confirma que o cliente já obteve token nem que uma ferramenta MCP funcionou.

## Pontos de UX a conferir em navegador

- Larguras de 320, 360, 390, 768 e 1440 px: sem rolagem horizontal do documento; identificadores e URLs longos quebram linha sem esconder significado.
- Zoom 200% e navegação somente por teclado; foco visível no seletor, na simulação e no reset.
- Leitura por tecnologia assistiva: status textual com `role=status`, títulos estruturados, rótulo do seletor e controle de estados sem depender apenas de cor.
- Sem transições obrigatórias, requisições de rede, persistência ou permissão real na prévia.
- Contraste e aparência no navegador real; a criação do arquivo HTML, isoladamente, não prova conformidade WCAG nem qualidade visual.

**Validação ainda pendente:** inspeção visual e interação em navegador nas larguras indicadas, zoom, teclado e leitores de tela. Nenhuma compatibilidade real com OAuth foi testada por este protótipo.

## Decisões pendentes para o backend — contrato, não implementação solicitada

1. Como comprovar controle do proprietário no primeiro pareamento e nos desbloqueios seguintes, sem pedir terminal para cada autorização. Loopback não comprova identidade.
2. Como impedir de forma verificável que painel, rotas administrativas, sessão e ações sejam expostos pelo Quick Tunnel; proteção de Host, Origin, CSRF e autenticação da sessão.
3. Como fornecer estados reais para painel e página pública sem permitir que esta aprove solicitações. Não inventar endpoints ou polling antes da especificação.
4. Como identificar a solicitação imutável (cliente, retorno, escopo, expiração), impedir decisão dupla e tratar perda de resposta, reconexão e reinício do processo.
5. Como expressar recusa, expiração, sessão administrativa expirada, indisponibilidade, vazio e resultado desconhecido com semântica não ambígua.
6. Como oferecer recuperação se abertura automática do navegador falhar sem enfraquecer autorização.

Somente após existir contrato aceito, substituir o template do terminal pela UI real, criar testes de segurança no backend e validar o fluxo com ChatGPT Web em workspace descartável. Sem alterar `internal/auth`, tokens, grants, rotas ou política de segurança nesta branch de frontend.

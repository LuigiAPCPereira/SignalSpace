# SignalSpace — revisão frontend do contrato administrativo OAuth proposto

**Status:** revisão de proposta recebida da frente de backend em 20/09/2026; **não é contrato aceito, implementação ou integração**. **Branch:** `feat/frontend-oauth-consent`. Base da proposta: revisão relatada do PR #1 no commit `f7c27d0`. O arquivo `docs/OWNER_PANEL_UNLOCK_UX.md` continua sendo a especificação de experiência anterior, ainda sem mecanismos reais.

## 1. Decisões de UX que a proposta atende

- A conexão OAuth é aprovada/recusada exclusivamente pelo proprietário autenticado no painel local; a página OAuth pública só observa e conclui uma decisão válida.
- A proposta divide os listeners: OAuth/MCP público em `127.0.0.1:7676`, painel administrativo em `127.0.0.1:7677`, com roteadores distintos. Isso é **uma proposta de backend, não separação atualmente implementada**; a porta local não substitui autenticação.
- Pareamento inicial via segredo de uso único no terminal e frase-senha para desbloqueios posteriores evitaria comandos no terminal em cada pedido. Na modalidade Quick descrita, a credencial em memória exige novo pareamento após cada reinicialização. Não apresentar a UX como pareamento permanente.
- Sessão proposta: inatividade de 15 minutos, máximo absoluto de 60 minutos e bloqueio manual; esses números só podem entrar em textos definitivos quando forem aceitos e comprovados por contrato/runtime.
- `GET /api/admin/v1/session`, `/requests` e `/requests/{id}` seriam fontes de verdade. `POST /api/admin/v1/requests/{id}/decision` deve registrar uma transição por ID e `expected_version`; o frontend não deduz sucesso pelo clique.
- `GET /authorize/status?request_id=...` seria apenas leitura do estado da própria solicitação pública, vinculada ao cookie OAuth. Nunca chamar ação administrativa ou transmitir cookie administrativo pela página pública.
- OAuth, concessão de workspace e aprovações de ferramentas do ChatGPT continuam independentes. O painel **não** cria fila por chamada MCP.

## 2. Mapeamento de UI para respostas propostas

| Superfície | Origem/condição proposta | Estado visual e comportamento |
| --- | --- | --- |
| Administração | `GET /session` sem sessão ou `401 AUTH_REQUIRED` | Bloqueado; nenhum dado de pedidos na resposta, HTML inicial, DOM ou estado de hidratação; exibir desbloqueio somente após especificação aceita. |
| Administração | Primeiro pareamento ainda não concluído | Apresentar jornada de configuração local separada do desbloqueio; código só pode ser digitado no listener administrativo protegido, nunca na página OAuth pública. |
| Administração | Autenticação enviada, sem resposta | Não exibir dados nem afirmar sessão ativa; reconciliar com `GET /session` conforme semântica acordada. |
| Administração | Sessão confirmada e `GET /requests` válido com lista vazia | Vazio real: “Nenhuma solicitação pendente”. |
| Administração | `GET /requests/{id}` retorna `PENDING`, versão e prazo | Mostrar nome do cliente como declarado/não verificado, `client_id`, retorno e escopos exatos; habilitar decisões apenas quando contrato comprovar sessão/pedido válido. |
| Administração | POST de decisão em andamento | Desabilitar ações duplicadas; não mostrar aprovado/recusado antes da resposta. |
| Administração | POST confirma decisão `APPROVED` ou `DENIED` | Exibir **decisão OAuth registrada**, não “token emitido”, “MCP conectado” ou “workspace concedido”. |
| Administração | POST sem resposta / `503` | Resultado desconhecido; consultar GET do pedido antes de qualquer nova decisão; não repetir POST automaticamente. |
| Administração | `409 STALE_REQUEST` ou `ALREADY_DECIDED` | Desabilitar ações e buscar estado atual; nunca revalidar automaticamente um consentimento sem revisão. |
| Administração | `410 REQUEST_EXPIRED` ou GET confirma `EXPIRED` | Exibir expirado e orientar a iniciar outro OAuth; após limpeza, `404` não deve ser apresentado como expiração comprovada. |
| Administração | `401 AUTH_REQUIRED` após exibir dados | Remover detalhes administrativos da interface, desabilitar ações e oferecer desbloqueio; a proteção real também é responsabilidade do servidor. |
| Administração | `403 ACCESS_DENIED` ou `429 RATE_LIMITED` | Não revelar detalhes internos; mostrar erro apropriado ao estágio (sessão, pareamento, decisão) e só permitir recuperação prevista no contrato. |
| Administração | Falha de rede / `503` durante consulta | Indisponível, **não** lista vazia; não mostrar dados administrativos que já não tenham sessão comprovada. |
| Pública | Consulta vinculada ao pedido retorna `PENDING` | “Aguardando confirmação neste computador”; nenhuma ação de aprovação na página pública. |
| Pública | Consulta confirma `APPROVED` | Possibilitar conclusão OAuth pelo fluxo existente, ainda sujeita a cookie público e CSRF; não inferir emissão de token. |
| Pública | Consulta confirma `DENIED` ou `EXPIRED` | Mensagem de negativa/prazo, sem emitir código nem oferecer acesso administrativo. |

Essas rotas, códigos e payloads são os **nomes da proposta anexada**, não interfaces disponíveis. Não inserir chamadas de rede nos protótipos até aceitação e implementação do contrato.

## 3. Ajustes necessários no protótipo

1. Criar telas distintas de **primeiro pareamento** (terminal uma vez por inicialização do Quick) e **desbloqueio posterior** (frase-senha). O estudo atual `SignalSpace-owner-unlock-ux.html` não possui essas telas concretas, contém só cenários ilustrativos e não foi versionado na branch.
2. Exibir condições de sessão após confirmação real: prazo apenas se API fornecer informação suficiente; oferecer bloqueio manual somente após endpoint implementado. Não inventar contagem regressiva.
3. Manter a tela de decisão existente (`frontend/consent/prototypes/local-approval-flow-v2.html`) sem alterar seu significado: simulação de autorização, recusa e resposta perdida, sem requests nem tokens.
4. Criar apresentação pública separada, sem dados administrativos, apenas após aceitar o contrato de consulta de estado. O arquivo `frontend/consent/consent.html` ainda descreve o fluxo antigo do terminal e **não deve ser integrado como experiência final**.
5. Evitar coleta ou armazenamento funcional de segredo e frase-senha no laboratório: protótipos só com rótulos, campos desabilitados ou valores explicitamente fictícios, nunca autenticação simulada apresentada como real.

## 4. Pendências que impedem o frontend funcional

**A. Primeiro pareamento e sessão:** `POST /pair` consome código e define frase-senha; ele também cria sessão administrativa autenticada ou responde “configurado, desbloqueie agora”? Definir estado e resposta inequívocos. Confirmar se a exigência de novo pareamento no restart é a experiência aceita para `connect quick`.

**B. Bootstrap CSRF:** especificar obtenção, escopo e rotação de CSRF para `pair`, `unlock`, `lock` e `decision`, inclusive quando ainda não existe sessão. `GET /session` retorna token de CSRF ou há outro contrato? Não colocar segredo em URL, logs nem storage do navegador. Confirmar critérios de Host/Origin em todos os métodos, não somente nas decisões.

**C. Workspace versus OAuth:** a proposta exige concessão de workspace **ativa no momento da aprovação de leitura**. O contrato de produto confirma independência das duas permissões, mas não determina sua ordem. Confirmar se ausência de grant impede consentimento OAuth de leitura ou apenas o uso posterior de ferramentas até grant ativo; documentar UX e erro correspondente antes de codificar.

**D. HTTP local versus HTTPS:** fechar estratégia de implantação, modelo de ameaça e propriedades do cookie antes de afirmar produção segura; cookie em HTTP loopback não oferece proteção de transporte equivalente a HTTPS. Garantir que backend nunca publica o painel pela porta pública como fallback.

**E. Respostas administrativas:** definir schemas de `GET /session`, `GET /requests` e resultados de `pair`, `unlock`, `lock` e `decision`; sem eles o frontend não sabe disponibilidade, autenticado, sessão expirada, data de expiração, CSRF ou resultado após resposta perdida. Especificar evolução de `version`, filtragem/limite da lista e retenção de estados terminais.

**F. Página pública:** especificar se `GET /authorize/status` usa polling e como a tela conclui OAuth sem emitir redirecionamentos duplicados, inclusive recusa, expiração, perda de rede e páginas fechadas. O polling administrativo a cada 2 segundos também é somente uma proposta; evitar afirmar estado atualizado em tempo real.

## 5. Critérios para aceitar a integração mais tarde

- Contrato de backend versionado e explicitamente aceito pelas duas frentes; alinhamento com PR #1 atualizado e testes negativos de fronteira pública/administrativa.
- Zero dados administrativos enviados ao navegador sem autenticação real; nome, URIs e escopos tratados como dados externos e escapados.
- `401` bloqueia e limpa apresentação; `404` não vira `EXPIRED` sem evidência; `409`/perda de resposta reconciliados via GET; POST nunca repetido automaticamente.
- Decisão associada a ID, versão e escopos originais; expiração e concessão verificadas no servidor; não atribuir sucesso à simples UI.
- Fluxo por teclado, foco, leitura assistiva, responsividade e sem requisições externas desnecessárias revisados em navegador; testes ponta a ponta e de segurança pertencem à fase de integração.

**Limites desta revisão:** somente documentação de frontend. Nenhuma rota administrativa, API, código Go, autenticação, armazenamento, política de concessão, conexão real, merge ou deploy é implementada aqui.

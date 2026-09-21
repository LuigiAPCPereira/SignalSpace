# SignalSpace — contrato inicial de programação local

**Estado:** fatia vertical implementada e validada somente localmente na branch `codex/mvp-vertical-programming`; o fluxo OAuth de escrita existe apenas em composição automatizada isolada. Isso não é autorização de publicar novas ferramentas MCP, abrir túnel ou conceder OAuth ao ChatGPT Web.

## Fronteira de autorização

- A edição exige a concessão existente de `owner`, `clientID` e `sessionID` em `workspace.Grants`; `Grants.Revoke` nega chamadas posteriores.
- `Grants.Grant` permanece somente leitura. A fronteira local `Grants.GrantWithScopes` aceita apenas `signalspace:workspace.read` e `signalspace:workspace.write`; uma concessão read-only não pode editar, e uma concessão write-only não pode ler.
- `Grants.ReplaceText` revalida owner, cliente, sessão ativa e `signalspace:workspace.write` a cada chamada, além das validações de arquivo. A revogação limpa os escopos e nega chamadas posteriores.
- O contrato remoto específico para escopos de programação ainda está **em andamento**. `signalspace:workspace.write` só pode aparecer com `WriteScope` e `CanIssueWrite` explicitamente configurados em uma composição de teste; a configuração padrão do emissor e do `cmd/signalspace` não anuncia nem emite esse escopo. Execução e Git continuam independentes e não concedidos.
- O emissor OAuth aceita somente as combinações canônicas `diagnostic`, `diagnostic + workspace.read`, `diagnostic + workspace.write` e, quando ambas estão configuradas, `diagnostic + workspace.read + workspace.write`. Escopos desconhecidos, duplicados ou reordenados são rejeitados. A concessão local é revalidada em `/authorize`, `/authorize/complete` e `/token`; o MCP revalida a concessão antes de cada edição.
- O commit `d84d0df` adiciona essa extensão experimental e seus testes sem alterar a composição padrão. O callback de concessão local é consultado antes de criar o pedido, concluir o consentimento e trocar o código; a revogação posterior continua sendo verificada pelo `Grants.ReplaceText` em cada operação.
- O commit `b46cda6` adiciona a porta interna `WorkspaceTextWriter` e a composição `replace_text` somente ao harness de testes do pacote MCP. A configuração que injeta essa porta é deliberadamente não exportada, portanto os entrypoints de produção não conseguem registrá-la por configuração normal.
- A revisão independente encontrou e o commit `8fcc7c0` corrigiu três pontos concretos: `tools/list` do harness só anuncia escrita quando o token verificado tem `workspace.write`; argumentos `null` são rejeitados antes do caso de uso; e os testes de cliente/owner divergentes usam concessões ativas, provando a negação na passagem MCP.
- Na composição isolada, cada chamada revalida identidade JWT assinada, owner, cliente, `signalspace:workspace.write`, sessão e concessão corrente antes de chamar `Grants.ReplaceText`. O resultado é estruturado e não revela caminhos ou detalhes do filesystem em falhas.
- Os modos públicos continuam sem `signalspace:workspace.write`, sem configuração de escrita em `cmd/signalspace` e sem `replace_text` em `tools/list`/`tools/call`. A prova cobre o handler diagnóstico sem capacidade de escrita e os modos de leitura existentes; não é aceite de transporte remoto.
- A UI administrativa funcional serve somente em `localhost:7677`, usa a sessão administrativa existente, cookie HttpOnly e CSRF em memória. Ela não escolhe raízes nem publica ferramentas de programação.

## Gate de promoção remota — decisão SS-MVP-002-PROMOTION-GATE-001

**Estado global:** PENDENTE. A decisão abaixo fecha a fronteira necessária para uma futura promoção experimental, mas não autoriza nem implementa a publicação de `workspace.write`.

**Decisão:** `workspace.write` continuará sendo uma capacidade por operação, nunca uma consequência de `client_id`, nome do cliente, confirmação exibida pelo ChatGPT, presença de um JWT, variável de ambiente, escopo anunciado ou configuração de teste. Uma futura composição remota só poderá ser criada depois de uma escolha explícita do proprietário no ponto local confiável já usado para concessões. Essa composição deverá ligar o emissor experimental, a concessão corrente e o handler MCP por uma dependência explícita; não será adicionada ao `cmd/signalspace`, ao `connect quick` ou ao `connect quick read` por configuração genérica. O desenho escolhido para esta proposta é terminal-local primeiro; uma UI ou fluxo remoto de ativação exige decisão própria e não fica implícito.

### Unidade de autorização e lifecycle

- A unidade é `owner` autenticado e verificado + cliente OAuth registrado + escopo exato aprovado + concessão local ativa do mesmo cliente/workspace + sessão correta + operação específica. OAuth e concessão de workspace são decisões distintas; `client_id` identifica registro e não atesta o software ChatGPT.
- Escopo solicitado, escopo aprovado, escopo emitido, escopo presente na concessão, capacidade habilitada, anúncio em `tools/list` e autorização de cada `tools/call` são estados diferentes. Nenhuma camada pode ampliar outra silenciosamente. Leitura não implica escrita; escrita não implica leitura, execução ou Git.
- Revogação antes de `/authorize`, `/authorize/complete` ou `/token` é revalidada pelo emissor e impede a etapa correspondente. Depois do token, cada chamada MCP consulta a concessão atual. O JWT pode permanecer criptograficamente válido até expirar; isso não mantém a capacidade após `workspace revoke`.
- `Grants.ReplaceText` e revogação são serializados pelo mutex da instância. Se a edição já tiver adquirido a seção crítica, ela pode efetivar-se antes da revogação; não há preempção nem garantia exactly-once. Em resposta perdida, não repetir a mutação automaticamente: ler/reconciliar o conteúdo e o diff antes de decidir qualquer nova chamada.
- Reinício/encerramento fecha a concessão local e os recursos temporários. `internal/mcp/oauth_write_lifecycle_test.go` fecha a composição, recria a identidade OAuth persistida com uma instância nova de `Grants`, mantém o JWT antigo criptograficamente verificável, nega o grant/sessão antigos e exige uma nova concessão explícita; isso é lifecycle de composição local, não aceite de restart operacional HTTPS. Não afirmar invalidação global do JWT nem isolamento contra escritores externos ou sandbox de processo.

### Transporte, limites e fronteira de outras operações

O transporte futuro mantém `7676` como listener público e `7677` exclusivamente administrativo em loopback; o túnel nunca aponta para `7677`, e nenhuma rota administrativa é registrada no handler público. HTTPS operacional, navegador real, grant ao ChatGPT e escolha de ativação remota continuam fora deste gate. `workspace.write` não concede execução de testes, shell, inspeção Git ou mutação Git. `ReplaceText` continua limitado a caminho relativo seguro, arquivo regular, UTF-8, conteúdo esperado, publicação atômica local e limites já documentados; concorrência externa e perda de resposta permanecem limites explícitos.

### Matriz do gate

| Controle exigido | Estado | Evidência atual | Condição para eventual promoção |
| --- | --- | --- | --- |
| Consentimento e combinações exatas de escopos | CONFIRMADO no harness | `internal/auth/write_scope_test.go`; combinações canônicas e texto de modificação | Revalidar no transporte escolhido sem ampliar o padrão |
| Escolha explícita do proprietário | CONFIRMADO para concessão local; PENDENTE para promoção | `workspaceConsole` exige comando local separado; OAuth não cria raiz | Definir e testar a confirmação terminal-local da composição remota |
| Owner, cliente, workspace e sessão vinculados | CONFIRMADO local | `Grants.ReplaceText`, `AllowsClientScope` e testes MCP/OAuth | Preservar a mesma cadeia sem parâmetros JSON-RPC como autoridade |
| Revogação antes da emissão e durante o uso | CONFIRMADO local | revalidação em authorize/complete/token e negativa com JWT válido | Aceitar explicitamente a semântica sem invalidação global do JWT |
| Conflito e resposta perdida | CONFIRMADO para `ReplaceText`; PENDENTE operacional | conteúdo esperado, diff e testes de duplicação | Definir reconciliação no cliente/transporte antes de qualquer retry |
| Isolamento dos listeners | CONFIRMADO na composição atual | `cmd/signalspace/quick_ports_test.go` e testes de painel | Repetir em aceite HTTPS sem publicar `7677` |
| Ausência de exposição administrativa | CONFIRMADO local | testes de rotas públicas/admin e composição Quick | Manter roteadores e Host/Origin separados |
| Ausência de ampliação read → write | CONFIRMADO | regressão `cmd/signalspace/promotion_gate_test.go` e `Grant` read-only | Nenhuma configuração pública deve injetar `workspaceWriter` |
| Limites de conteúdo, caminho e operação | CONFIRMADO local | `internal/workspace/edit_test.go` e testes MCP | Transportar os limites sem prometer sandbox ou exactly-once |
| Encerramento e reinício | CONFIRMADO localmente; PENDENTE operacional | `internal/mcp/oauth_write_lifecycle_test.go` fecha servidor, grant e identidade, recria a composição, nega a sessão antiga e aceita somente nova concessão/sessão; não é restart HTTPS | Exercitar ciclo completo no modo remoto descartável |
| HTTPS e navegador operacional | BLOQUEADO nesta missão | harness usa HTTP local/Host simulado; nenhum túnel foi aberto | Nova autorização específica e aceite real sem bypass TLS |
| Autorização para ativação remota | PENDENTE | não existe composição pública de escrita | Decisão explícita do proprietário e novo gate no SHA exato |

O gate global não é aprovado por somar testes do harness. Enquanto qualquer linha necessária permanecer PENDENTE ou BLOQUEADA, `workspace.write` não deve ser publicado.

## Edição segura inicial

`Session.ReplaceText` e `Grants.ReplaceText` aceitam apenas caminho relativo canônico, componentes sem symlink, arquivo regular, UTF-8 sem NUL e conteúdo até `MaxTextBytes`. A operação compara o conteúdo esperado exatamente antes de publicar um temporário no mesmo diretório, preserva as permissões e retorna conflito sem sobrescrever silenciosamente. Leitura, edição e revogação são serializadas na instância local.

Essa proteção é uma versão otimista local; outro processo externo pode alterar o arquivo fora do mutex do SignalSpace. Por isso não é apresentada como transação filesystem ou isolamento contra escritores externos.

## Execução controlada inicial

`programming.RunPredefinedTest` executa somente `go test ./...`, sem shell e sem argumentos arbitrários, no diretório da sessão. Há limite configurável de até cinco minutos, saída limitada a 1 MiB, código de saída, distinção entre falha do teste, cancelamento e timeout, e espera pelo encerramento do processo principal. O diretório aprovado **não é sandbox de processo**: o comando mantém os privilégios do usuário e pode acessar recursos permitidos pelo sistema operacional.

## Revisão Git inicial

`CaptureGitSnapshot` observa `git status --porcelain` e `git diff --no-ext-diff --binary`, ambos limitados e sem `add`, commit, push ou limpeza. `CompareGitSnapshots` separa o baseline do estado após a edição. O resultado é somente local e não é uma ferramenta MCP.

## Evidência e próximos limites

O teste vertical descartável percorre leitura → edição → `go test ./...` → snapshot/diff → revogação → negação. `internal/mcp/workspace_write_test.go` cobre a fronteira MCP com tokens montados no harness; `internal/mcp/oauth_write_integration_test.go` acrescenta o fluxo real do próprio emissor OAuth, PKCE, aprovação local, troca por token assinado, `tools/list`, `replace_text`, diff observável no arquivo, revogação e código de uso único. `internal/mcp/oauth_write_lifecycle_test.go` comprova a perda da concessão após fechamento/recriação local mesmo com JWT persistido e a exigência de nova sessão explícita. `internal/auth/write_scope_test.go` cobre opt-in explícito, texto de consentimento, combinações canônicas e revogação antes de conclusão/troca. Os testes continuam sem navegador real, túnel, grant ao ChatGPT, sandbox de processo, escritores externos ou integração com `feat/frontend-oauth-consent`; a promoção para transporte remoto requer contrato e gate próprios.

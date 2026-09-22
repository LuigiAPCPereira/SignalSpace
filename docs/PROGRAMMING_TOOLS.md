# SignalSpace — contrato inicial de programação local

**Estado:** fatia vertical implementada e validada somente localmente na branch `codex/mvp-vertical-programming`; escrita, revisão Git e execução de teste existem apenas em composição automatizada isolada. Isso não é autorização de publicar novas ferramentas MCP, abrir túnel ou conceder OAuth ao ChatGPT Web.

## Composição operacional fail-closed — SS-MVP-002

`cmd/signalspace/composition.go` define uma enumeração interna fechada: `diagnostic` ou `read`. A política mapeia diagnóstico ao escopo `signalspace:diagnostic` e à ferramenta `connection_diagnostic`; leitura mantém esse escopo e acrescenta somente `signalspace:workspace.read`, `read_file` e `list_directory`. Não há modo nem campo de composição para escrita, execução de testes ou Git.

O parser `connect quick` produz somente esses modos; seleções como `write` são inválidas. `planComposition` rejeita valores não suportados antes de inspeção de ambiente, confirmação, reserva de portas, inicialização do túnel ou criação de estado OAuth. `embeddedHandlerForPlan` reconfirma que o plano corresponde exatamente à política antes de criar emissor, concessões ou handler MCP. `panel` permanece apresentação/servidor administrativo exclusivamente local e não muda a composição MCP.

Os testes de `cmd/signalspace` exigem a lista exata de escopos e ferramentas em cada modo e verificam que enum inválido ou plano adulterado não cria estado OAuth; a rejeição Quick não lê confirmação nem chama túnel, verificador ou fábrica do painel. Isso valida o limite de composição no código local, não transporte HTTPS nem aceitação operacional. O gate `SS-MVP-002-PROMOTION-GATE-001` continua **PENDENTE**; capacidades de programação seguem disponíveis apenas no harness de testes.

## Fronteira de autorização

- A edição exige a concessão existente de `owner`, `clientID` e `sessionID` em `workspace.Grants`; `Grants.Revoke` nega chamadas posteriores.
- `Grants.Grant` permanece somente leitura. A fronteira local `Grants.GrantWithScopes` aceita somente capacidades explicitamente compostas (`signalspace:workspace.read`, `signalspace:workspace.write`, `signalspace:git.review` e `signalspace:test.run`); uma concessão read-only não pode editar, executar testes ou revisar Git.
- `Grants.ReplaceText` revalida owner, cliente, sessão ativa e `signalspace:workspace.write` a cada chamada, além das validações de arquivo. A revogação limpa os escopos e nega chamadas posteriores.
- O contrato remoto específico para escopos de programação ainda está **em andamento**. `signalspace:workspace.write`, `signalspace:git.review` e `signalspace:test.run` só podem aparecer com seus pares de configuração/validadores locais explicitamente ligados em uma composição de teste; a configuração padrão do emissor e do `cmd/signalspace` não anuncia nem emite esses escopos.
- O emissor OAuth aceita somente subconjuntos explícitos na ordem canônica configurada, sempre começando por `signalspace:diagnostic`: diagnóstico, leitura, escrita, Git e teste. Escopos desconhecidos, duplicados, não configurados, reordenados ou com whitespace não canônico são rejeitados. Cada capacidade solicitada é revalidada em `/authorize`, `/authorize/complete` e `/token`; o MCP revalida a concessão correspondente antes de cada operação.
- A revisão Git usa o escopo independente `signalspace:git.review`; ele não implica leitura de arquivos, edição, execução ou mutação Git. A concessão local aceita esse escopo somente quando composto explicitamente no processo terminal.
- O commit `d84d0df` adiciona a extensão experimental de escrita e seus testes sem alterar a composição padrão. O callback de concessão local é consultado antes de criar o pedido, concluir o consentimento e trocar o código; a revogação posterior continua sendo verificada pelo `Grants.ReplaceText` em cada operação.
- A integração OAuth de programação adiciona os pares opt-in `GitScope`/`CanIssueGit` e `TestScope`/`CanIssueTest`. O consentimento descreve que teste executa código com os privilégios do usuário e que o workspace não é sandbox; Git descreve status/diff sensíveis e não autoriza commit/push. Aprovar OAuth não cria nem restaura uma concessão de workspace.
- O commit `b46cda6` adiciona a porta interna `WorkspaceTextWriter` e a composição `replace_text` somente ao harness de testes do pacote MCP. A configuração que injeta essa porta é deliberadamente não exportada, portanto os entrypoints de produção não conseguem registrá-la por configuração normal.
- A revisão independente encontrou e o commit `8fcc7c0` corrigiu três pontos concretos: `tools/list` do harness só anuncia escrita quando o token verificado tem `workspace.write`; argumentos `null` são rejeitados antes do caso de uso; e os testes de cliente/owner divergentes usam concessões ativas, provando a negação na passagem MCP.
- A composição Git do harness mantém a mesma separação: `tools/list` só anuncia `review_git_changes` quando o token verificado tem `signalspace:git.review`; cada chamada revalida owner, cliente, sessão e concessão Git antes de executar a observação; revogação nega a chamada mesmo com JWT criptograficamente válido.
- A composição de testes do harness mantém separação própria: `tools/list` só anuncia `run_workspace_tests` quando o token verificado tem `signalspace:test.run`; cada chamada revalida owner, cliente, sessão e concessão de teste antes de executar; revogação nega a chamada mesmo com JWT criptograficamente válido.
- Na composição isolada, cada chamada revalida identidade JWT assinada, owner, cliente, `signalspace:workspace.write`, sessão e concessão corrente antes de chamar `Grants.ReplaceText`. O resultado é estruturado e não revela caminhos ou detalhes do filesystem em falhas.
- Os modos públicos continuam sem `signalspace:workspace.write`, `signalspace:git.review`, `signalspace:test.run`, sem configuração de programação em `cmd/signalspace` e sem `replace_text`, `review_git_changes` ou `run_workspace_tests` em `tools/list`/`tools/call`. A prova cobre o handler diagnóstico sem capacidade de programação e os modos de leitura existentes; não é aceite de transporte remoto.
- A UI administrativa funcional serve somente em `localhost:7677`, usa a sessão administrativa existente, cookie HttpOnly e CSRF em memória. Ela não escolhe raízes nem publica ferramentas de programação.

## Aprovação terminal-local experimental

`workspace request <client-id> <absolute-path>` mantém seu significado anterior: cria, após confirmação local, somente uma concessão de leitura. A seleção de programação existe apenas quando o harness injeta explicitamente `workspace.CapabilityApproval`; a composição padrão, `cmd/signalspace`, `connect quick` e `connect quick read` não a instanciam.

Na composição experimental, o terminal pode receber `workspace request-programming <client-id> <scope1,scope2,...> <absolute-path>`. O cliente precisa aparecer em `workspace clients`, que lista somente registros OAuth desta instância que já concluíram uma troca de token. A lista usa os escopos completos e independentes `signalspace:workspace.read`, `signalspace:workspace.write`, `signalspace:test.run` e `signalspace:git.review`; nenhum é adicionado implicitamente. Caminhos com espaços permanecem no restante da linha e são validados como raízes canônicas, sem raiz ampla, home, componente symlink ou controle.

O pedido exibe nome declarado e ID OAuth, sem atestar o software, além dos efeitos: leitura de arquivos permitidos; escrita de arquivos permitidos; `go test ./...` com privilégios do usuário e sem sandbox; ou inspeção de status/diff Git potencialmente sensível, sem commit/push. Nenhuma concessão é criada nessa etapa. `workspace approve-programming <id>` consome o identificador de uso único somente se ainda válido, revalida o cliente elegível e chama `GrantWithScopes` com exatamente os escopos selecionados; `workspace cancel-programming <id>` descarta o pedido. O prazo é de dois minutos, e `workspace revoke <session-id>` continua revogando todas as capacidades da sessão.

`CapabilityApproval` é a mesma lógica exercitada pelo console e pelo harness OAuth/MCP; o teste vertical deixou de chamar `GrantWithScopes` diretamente para criar a concessão de programação. A aprovação terminal-local continua distinta do consentimento OAuth: depois da confirmação, cada token e cada ferramenta ainda exigem seus escopos e a concessão corrente. A extensão não adiciona rotas administrativas, variáveis de ambiente, flags, comandos públicos, writers, executor ou Git reviewer aos entrypoints.

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
| Escolha explícita do proprietário | CONFIRMADO para concessão local experimental; PENDENTE para promoção | `CapabilityApproval` exige cliente emitido, seleção, resumo, identificador separado, confirmação única/expiração e chama `GrantWithScopes` somente depois; console padrão não injeta a dependência | Preservar a mesma confirmação na composição operacional futura, sem habilitar Quick/entrypoint |
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

`programming.RunPredefinedTest` executa somente `go test ./...`, sem shell e sem argumentos arbitrários, pela porta estreita `workspace.ProcessDirectory`. Há limite configurável de até cinco minutos, saída limitada a 1 MiB, código de saída, estados `TEST_PASSED`, `TEST_FAILED`, `TIMED_OUT`, `CANCELED` e `UNKNOWN`, além de espera pelo encerramento do processo principal. O ambiente da fixture fixa `GOTOOLCHAIN=local`, `GOPROXY=off` e `GOSUMDB=off`, sem prometer isolamento. `terminated` significa que o processo principal foi observado por `cmd.Wait`; descendentes não são atestados individualmente. O diretório aprovado **não é sandbox de processo**: o comando mantém os privilégios do usuário e pode acessar recursos permitidos pelo sistema operacional.

### Execução de testes pelo MCP isolado

O harness de `internal/mcp` injeta a porta não exportada `WorkspaceTestRunner` em `OAuthConfig`. A ferramenta `run_workspace_tests` aceita somente `session_id`, usa o escopo independente `signalspace:test.run` e não recebe comando, script, argumentos, ambiente, caminho ou executável. A composição usa `workspace.Grants.WithAuthorizedTestProcessDir`, serializando a execução com edição e revogação sem entregar `Session`, raiz ou descritor ao transporte.

O resultado estruturado contém comando fixo, código de saída, stdout/stderr limitados, truncamento, timeout, cancelamento, término observado e estado final. Falha do teste retorna `TEST_FAILED` sem ser convertida em erro de transporte; falha de autorização/inicialização permanece indisponível. O teste pode ter efeitos colaterais e não é tratado como read-only ou idempotente.

`internal/mcp/test_runner_integration_test.go` percorre fixture descartável com JWT/escopos independentes: `replace_text` → `run_workspace_tests` aprovado → `review_git_changes`, além de teste deliberadamente falhando, timeout, argumentos extras, escopos read/write/Git sem execução, divergência de cliente/sessão, revoke e composição pública sem a ferramenta. `internal/auth/programming_scope_test.go` cobre metadata, subconjuntos canônicos, consentimento e revalidação do emissor. A emissão OAuth de `signalspace:test.run` agora é exercitada somente no harness integrado abaixo; nenhum entrypoint remoto ou Quick injeta a porta.

## Revisão Git inicial

`CaptureGitSnapshot` observa `git status --porcelain` e `git diff --no-ext-diff --no-textconv --binary`, ambos limitados e sem `add`, commit, push ou limpeza. `CompareGitSnapshots` separa o baseline do estado após a edição.

O harness MCP isolado injeta uma porta `WorkspaceGitReviewer` não exportada no `OAuthConfig`. A ferramenta `review_git_changes` recebe somente `session_id`, retorna baseline/estado atual, `status_changed`, `diff_changed` e completude, e nunca recebe raiz absoluta ou `Session`. A composição de teste usa `workspace.Grants.WithAuthorizedGitProcessDir`, que mantém observação e revogação serializadas pela concessão Git corrente. Não há registro dessa porta no `cmd/signalspace`, no Quick ou no transporte público.

Antes de cada leitura, o executor Git desativa `core.fsmonitor`, `core.untrackedCache` e atualizações opcionais do índice; `diff` usa `--no-ext-diff` e `--no-textconv`. Variáveis `GIT_CONFIG_*`, `GIT_ATTR_*`, `GIT_EXTERNAL_DIFF`, `GIT_DIFF_OPTS`, caminhos alternativos de índice/objetos, SSH/askpass/pager/editor e `GIT_EXEC_PATH` são removidas do ambiente filho; configuração global e de sistema são desabilitadas. A regressão `TestGitSnapshotNeutralizesExecutableConfigAndEnvironment` instala tentativas de fsmonitor/diff externo e confirma que nenhuma é executada. Configuração local não executável/extensões necessárias ao repositório não são tratadas como sandbox; o processo mantém os privilégios do usuário.

## Evidência OAuth integrada e próximos limites

O teste vertical descartável percorre leitura → edição → `go test ./...` → snapshot/diff → revogação → negação. `internal/mcp/workspace_write_test.go` cobre a fronteira MCP com tokens montados no harness; `internal/mcp/oauth_write_integration_test.go` acrescenta o fluxo real do próprio emissor OAuth, PKCE, aprovação local, troca por token assinado, `tools/list`, `replace_text`, diff observável no arquivo, revogação e código de uso único. `internal/mcp/git_review_integration_test.go` percorre fixture Git temporária → baseline → JWT/escopo de edição independente do escopo Git → `replace_text` → `review_git_changes` real → status/diff observável → revoke → deny, além de token ausente/inválido, leitura/escrita sem Git, owner/client/session divergentes, argumentos extras e ausência de repositório. `internal/mcp/test_runner_integration_test.go` acrescenta a composição `run_workspace_tests`, resultado de sucesso/falha/timeout, negativos de escopo e sessão, revogação e não exposição pública.

`internal/mcp/oauth_programming_integration_test.go` percorre no harness OAuth real DCR, PKCE, decisão terminal, `/authorize`, `/authorize/complete`, `/token` e JWT verificado por `NewStaticJWTVerifier`. A seleção local pede exatamente leitura, escrita, Git e teste; o baseline Git é capturado antes de qualquer edição. Tokens separados exercitam `read_file` sobre a fixture, usam o conteúdo retornado como precondição de `replace_text`, executam `go test ./...` e observam o diff Git. A cobertura inclui read-only sem escrita/teste/Git, tokens write/test/Git sem leitura, sessão incompatível, falta de concessão durante aprovação pendente, códigos de uso único, revoke com JWTs ainda válidos e nenhuma operação iniciada depois de revoke. `tools/list` no harness de capacidades mantém leitura configurada; a composição pública e o modo `read` do entrypoint permanecem sem mudança e são verificados separadamente.

`internal/mcp/oauth_write_lifecycle_test.go` comprova a perda da concessão após fechamento/recriação local mesmo com JWT persistido e a exigência de nova sessão explícita. `internal/auth/write_scope_test.go`, `internal/auth/programming_scope_test.go` e o teste de composição OAuth integrada cobrem opt-in explícito, texto de consentimento, combinações canônicas, metadata padrão sem escopos de programação e revalidação antes da emissão. Tudo isso continua restrito ao harness: sem navegador real, túnel, HTTPS operacional, grant ao ChatGPT Web, workspace real ou CI; não autoriza promover as portas ao `cmd/signalspace`, Quick ou transporte público. O processo de teste mantém privilégios do usuário e não é sandbox.

Os testes continuam sem navegador real, túnel, grant ao ChatGPT, workspace real, sandbox de processo, escritores externos ou integração com `feat/frontend-oauth-consent`; a promoção para transporte remoto requer contrato e gate próprios.

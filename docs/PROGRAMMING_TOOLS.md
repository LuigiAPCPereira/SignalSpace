# SignalSpace — contrato inicial de programação local

## Capability approvals de uso único — estado vigente em 26/09/2026

O domínio `internal/approval` prepara a autorização local de uma operação concreta sem ampliar grants permanentes. O fingerprint SHA-256 canônico e o contexto owner/client/token-family/workspace/session/capability/tool são a autoridade; `safe_summary` é apenas apresentação limitada. `ALLOW_ONCE` produz permit interno por referência, e não scope OAuth, bearer, capability nova ou execução automática. `ALLOW_SESSION` e `ALLOW_WORKSPACE` permanecem no próximo gate.

Nenhuma tool MCP pública cria ou consome approvals nesta missão. A ponte `internal/policy` só cria/reusa request em `REQUIRE_APPROVAL`; `DENY` não cria fila. A API owner-side é a rota administrativa separada `/api/admin/v1/capability-approvals`; a fila OAuth `/api/admin/v1/requests` não foi reutilizada. Estado após restart é descartado de modo fail-closed.

## Step-up OAuth por ferramenta — `SS-MVP-002-OAUTH-TOOL-STEPUP-001` — 25/09/2026

**Estado:** **IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICADO REMOTAMENTE** em `14821b4`; a implementação está no commit `c2dcdbd` e a base/remoto observado antes da missão era `de8ff7e3d253f4dedbfbba1135bdf53e682cd335`. O aceite externo no ChatGPT Web ainda não foi repetido no HEAD corrigido.

Na composição `programming`, discovery é derivado das portas configuradas, não do bearer atual. Assim, `initialize` e `tools/list` com somente `signalspace:diagnostic` expõem a superfície Programming existente para que o cliente descubra os mecanismos de step-up. Isso não concede capacidade, grant, sessão ou acesso a filesystem/Git.

Os descriptors e challenges de READ, WRITE, Git review, Git index e Git commit declaram o conjunto cumulativo `signalspace:diagnostic` mais o scope específico. Uma chamada sem o scope específico retorna erro MCP com `_meta["mcp/www_authenticate"]`, `error="insufficient_scope"` e o resource metadata correto antes de consultar ou mutar qualquer backend. Grant local, owner/client/session, managed worktree e precondições continuam obrigatórios e são revalidados na chamada.

A correção renomeia a semântica de anúncio para `discoverable` e mantém `verify()` somente no caminho de execução. `connect quick`/diagnostic, `connect quick read` e suas listas preservam os contratos próprios; `run_workspace_tests`, shell, branch, Git remoto, lifecycle MCP de worktree e novas tools continuam ausentes. Testes verificam lista exata, scopes, zero efeitos, grant/revoke e não dependência de estado global mutable.

**Estado vigente (25/09/2026):** o gate `SS-MVP-002-GIT-COMMIT-GATE-001` está **APROVADO / IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICADO REMOTAMENTE** em `0420c9ae14768e099fbb51452ee71e7dc5af7916`. A cadeia real é `60c290a` → `b3f2add` → `ccc4d17` → `247ad3d` → `0420c9a`; `ccc4d17` é somente documental e não adiciona código ou capability. A publicação foi fast-forward normal; a checagem de exatamente três commits ocorreu depois do push e fica registrada como desvio procedural. `test.run`, shell, comandos arbitrários, branch, Git remoto como capability e lifecycle MCP de worktree continuam fora. Isso não é aceite de túnel, HTTPS, navegador, grant real ao ChatGPT Web, workspace real, CI, merge ou deploy.

O estado do MVP continua **PARCIAL**: a decisão do gate é distinta da conclusão de `SS-MVP-002`, e a evidência atual é local/automatizada. O modo público exige seleção explícita `connect quick programming`, OAuth com escopo exato e concessão terminal-local ativa; a presença de JWT, `client_id`, metadata ou configuração não cria concessão.

## Composição operacional fail-closed — SS-MVP-002

`cmd/signalspace/composition.go` define uma enumeração interna fechada: `diagnostic`, `read` ou `programming`. O plano canônico determina os escopos OAuth, o verificador JWT local e o único endereço MCP permitido (`admin.PublicAddress`, `127.0.0.1:7676`), além do modo do console. Diagnóstico admite somente `signalspace:diagnostic` e `connection_diagnostic`; leitura acrescenta `signalspace:workspace.read`, `read_file` e `list_directory`; programação é um modo explícito que acrescenta READ + WRITE + GIT review + Git index + Git commit, com `git_status` sob `signalspace:git.review`, stage/unstage sob `signalspace:git.index` e `commit_git_index` sob `signalspace:git.commit`, sem `test.run` ou shell. O Git index permanece publicado no remoto live em `60c290a`; o Git commit v1 está publicado no remoto live em `0420c9a`, sem promoção de branch, shell ou Git remoto como capability.

O parser `connect quick` aceita somente as combinações explícitas `diagnostic`, `read` e `programming`, com `panel` opcional; seleções como `write`, `shell` ou `test` são inválidas. `planComposition` rejeita valores não suportados antes de inspeção de ambiente, confirmação, reserva de portas, inicialização do túnel ou criação de estado OAuth. `embeddedHandlerForPlan` reconfirma que o plano corresponde exatamente à política antes de criar emissor, concessões ou handler MCP e usa `mcp.NewOAuthProgrammingHandler` somente no modo opt-in. `reserveQuickPortsForPlan` reserva o endereço MCP do plano; a porta administrativa `7677` é separada, nunca compõe MCP e só existe quando `panel` foi selecionado. `panel` permanece apresentação/servidor administrativo exclusivamente local e não muda a composição MCP.

Os testes de `cmd/signalspace` exigem a lista exata de escopos e ferramentas em cada modo e verificam que enum inválido ou plano adulterado (incluindo sem verificador ou apontando MCP à porta admin) não cria estado OAuth; a rejeição Quick não lê confirmação nem chama túnel, verificador de transporte ou fábrica do painel. O teste vertical de programação prova a matriz bearer → initialize → tools/list → tools/call, concessão local, baseline Git e revogação. Isso valida o limite de composição no código local, não transporte HTTPS nem aceitação operacional externa.

## Fronteira de autorização

- A edição exige a concessão existente de `owner`, `clientID` e `sessionID` em `workspace.Grants`; `Grants.Revoke` nega chamadas posteriores.
- `Grants.Grant` permanece somente leitura. A fronteira local `Grants.GrantWithScopes` aceita somente capacidades explicitamente compostas (`signalspace:workspace.read`, `signalspace:workspace.write`, `signalspace:git.review` e `signalspace:test.run`); uma concessão read-only não pode editar, executar testes ou revisar Git.
- `Grants.ReplaceText` revalida owner, cliente, sessão ativa e `signalspace:workspace.write` a cada chamada, além das validações de arquivo. A revogação limpa os escopos e nega chamadas posteriores.
- O contrato público opt-in de READ + WRITE + GIT agora é composto somente por `mcp.NewOAuthProgrammingHandler`, que exige as portas locais de leitor, listador, escritor e revisor Git vinculadas à concessão corrente. A configuração padrão e os modos `diagnostic`/`read` continuam sem escrita ou Git; `test.run` permanece restrito ao harness.
- O emissor OAuth aceita somente subconjuntos explícitos na ordem canônica configurada, sempre começando por `signalspace:diagnostic`: diagnóstico, leitura, escrita, Git e teste. Escopos desconhecidos, duplicados, não configurados, reordenados ou com whitespace não canônico são rejeitados. Cada capacidade solicitada é revalidada em `/authorize`, `/authorize/complete` e `/token`; o MCP revalida a concessão correspondente antes de cada operação.
- A revisão Git usa o escopo independente `signalspace:git.review`; ele não implica leitura de arquivos, edição, execução ou mutação Git. A concessão local aceita esse escopo somente quando composto explicitamente no processo terminal.
- O commit Git usa o escopo independente `signalspace:git.commit`; ele não é consequência de `workspace.write`, `git.review` ou `git.index`. O owner precisa configurar uma identidade local privada e conceder a capacidade somente para uma managed worktree; checkout comum, branch, shell, hooks, signing e remoto falham fechado.
- O commit `d84d0df` adiciona a extensão experimental de escrita e seus testes sem alterar a composição padrão. O callback de concessão local é consultado antes de criar o pedido, concluir o consentimento e trocar o código; a revogação posterior continua sendo verificada pelo `Grants.ReplaceText` em cada operação.
- A integração OAuth de programação adiciona os pares opt-in `GitScope`/`CanIssueGit` e `TestScope`/`CanIssueTest`. O consentimento descreve que teste executa código com os privilégios do usuário e que o workspace não é sandbox; Git descreve status/diff sensíveis e não autoriza commit/push. Aprovar OAuth não cria nem restaura uma concessão de workspace.
- A porta `WorkspaceTextWriter` continua não configurável por parâmetros HTTP; ela só entra no entrypoint pela composição nomeada e fechada `NewOAuthProgrammingHandler`. A composição exige também o revisor Git e não aceita porta de execução de testes.
- A revisão independente encontrou e o commit `8fcc7c0` corrigiu três pontos concretos: `tools/list` do harness só anuncia escrita quando o token verificado tem `workspace.write`; argumentos `null` são rejeitados antes do caso de uso; e os testes de cliente/owner divergentes usam concessões ativas, provando a negação na passagem MCP.
- A composição Git do harness mantém a mesma separação: `tools/list` só anuncia `review_git_changes` quando o token verificado tem `signalspace:git.review`; cada chamada revalida owner, cliente, sessão e concessão Git antes de executar a observação; revogação nega a chamada mesmo com JWT criptograficamente válido.
- A composição de testes do harness mantém separação própria: `tools/list` só anuncia `run_workspace_tests` quando o token verificado tem `signalspace:test.run`; cada chamada revalida owner, cliente, sessão e concessão de teste antes de executar; revogação nega a chamada mesmo com JWT criptograficamente válido.
- Na composição isolada, cada chamada revalida identidade JWT assinada, owner, cliente, `signalspace:workspace.write`, sessão e concessão corrente antes de chamar `Grants.ReplaceText`. O resultado é estruturado e não revela caminhos ou detalhes do filesystem em falhas.
- Os modos públicos `diagnostic` e `read` continuam sem `signalspace:workspace.write`, `signalspace:git.review` e `signalspace:test.run`. O modo `programming` anuncia `replace_text` e `review_git_changes` somente para bearers com os escopos correspondentes; nunca anuncia `run_workspace_tests`, shell ou mutação Git. A prova permanece local e não é aceite de transporte remoto.
- A UI administrativa funcional serve somente em `localhost:7677`, usa a sessão administrativa existente, cookie HttpOnly e CSRF em memória. Ela não escolhe raízes nem publica ferramentas de programação.

## Aprovação terminal-local experimental

`workspace request <client-id> <absolute-path>` mantém seu significado anterior: cria, após confirmação local, somente uma concessão de leitura. A seleção de programação exige `workspace.CapabilityApproval` e agora é instanciada somente pelo modo explícito `connect quick programming`; `connect quick` e `connect quick read` não a instanciam.

Na composição experimental, o terminal pode receber `workspace request-programming <client-id> <scope1,scope2,...> <absolute-path>`. O cliente precisa aparecer em `workspace clients`, que lista somente registros OAuth desta instância que já concluíram uma troca de token. A lista usa os escopos completos e independentes `signalspace:workspace.read`, `signalspace:workspace.write`, `signalspace:test.run` e `signalspace:git.review`; `signalspace:git.index` é deliberadamente rejeitado nesse fluxo de checkout comum. Caminhos com espaços permanecem no restante da linha e são validados como raízes canônicas, sem raiz ampla, home, componente symlink ou controle.

O pedido exibe nome declarado e ID OAuth, sem atestar o software, além dos efeitos: leitura de arquivos permitidos; escrita de arquivos permitidos; `go test ./...` com privilégios do usuário e sem sandbox; ou inspeção de status/diff Git potencialmente sensível, sem commit/push. Nenhuma concessão é criada nessa etapa. `workspace approve-programming <id>` consome o identificador de uso único somente se ainda válido, revalida o cliente elegível e chama `GrantWithScopes` com exatamente os escopos selecionados; `workspace cancel-programming <id>` descarta o pedido. O prazo é de dois minutos, e `workspace revoke <session-id>` continua revogando todas as capacidades da sessão.

`CapabilityApproval` é a mesma lógica exercitada pelo console e pelo harness OAuth/MCP; o teste vertical público usa a confirmação terminal-local antes de emitir tokens READ/WRITE/GIT/Git index. A aprovação terminal-local continua distinta do consentimento OAuth: depois da confirmação, cada token e cada ferramenta ainda exigem seus escopos e a concessão corrente. A composição não adiciona rotas administrativas, executor, shell ou lifecycle MCP aos entrypoints; o revisor Git mantém baseline por sessão e somente status/diff.

## Consulta e revogação local da concessão

`workspace status` consulta `Grants.Snapshot` sob o mutex e informa somente se há concessão local ativa ou ausente. Com uma concessão ativa, mostra o `session_id`, o `client_id`, o nome declarado apenas se a lista confiável atual de clientes emitidos ainda o contém, os escopos exatos em ordem canônica e as formas de revogar. A falta do nome não prova revogação. O snapshot é uma cópia independente e não expõe raiz, descritor, objeto `Session`, token OAuth, chave privada ou conteúdo; consulta não autoriza, renova nem altera a concessão. Instância encerrada é distinta de estado ausente (`ErrClosed`).

`workspace revoke current` obtém o snapshot e chama o `Grants.Revoke` existente com aquele ID exato; `workspace revoke <session-id>` continua aceito. Se a concessão mudar entre consulta e revogação, a comparação sob o mutex rejeita o ID antigo e preserva a sessão substituta; o terminal orienta consultar `workspace status` novamente. Uma revogação concorrente pode aguardar uma operação local já dentro da seção crítica; esse comando não interrompe processo em andamento nem fornece preempção.

Esses comandos pertencem apenas ao console de stdin local e não criam rota HTTP, ferramenta MCP ou função no painel administrativo. O status descreve apenas `Grants` local e não comprova validade de token OAuth ou conexão/chamada MCP. No console padrão de diagnóstico, concessão interna não publica `read_file` e o MCP continua limitado a `connection_diagnostic`. Em um console experimental com escopos de programação, eles são mostrados como estado local experimental, não como capacidades ativadas remotamente.

## Gate de promoção remota — decisão SS-MVP-002-PROMOTION-GATE-001

**Estado global:** APROVADO PELO PROPRIETÁRIO; implementação local da composição READ + WRITE + GIT confirmada. Isso não equivale à conclusão de `SS-MVP-002` nem ao aceite operacional externo.

**Decisão:** `workspace.write` continua sendo uma capacidade por operação, nunca uma consequência de `client_id`, nome do cliente, confirmação exibida pelo ChatGPT, presença de um JWT, variável de ambiente, escopo anunciado ou configuração de teste. A composição aprovada liga o emissor experimental, a concessão corrente e o handler MCP por uma dependência explícita somente quando o proprietário seleciona `connect quick programming`; ela não altera `connect quick` nem `connect quick read`. Uma UI ou fluxo remoto de ativação exige decisão própria e não fica implícito.

### Unidade de autorização e lifecycle

- A unidade é `owner` autenticado e verificado + cliente OAuth registrado + escopo exato aprovado + concessão local ativa do mesmo cliente/workspace + sessão correta + operação específica. OAuth e concessão de workspace são decisões distintas; `client_id` identifica registro e não atesta o software ChatGPT.
- Escopo solicitado, escopo aprovado, escopo emitido, escopo presente na concessão, capacidade habilitada, anúncio em `tools/list` e autorização de cada `tools/call` são estados diferentes. Nenhuma camada pode ampliar outra silenciosamente. Leitura não implica escrita; escrita não implica leitura, execução ou Git.
- Revogação antes de `/authorize`, `/authorize/complete` ou `/token` é revalidada pelo emissor e impede a etapa correspondente. Depois do token, cada chamada MCP consulta a concessão atual. O JWT pode permanecer criptograficamente válido até expirar; isso não mantém a capacidade após `workspace revoke`.
- `Grants.ReplaceText` e revogação são serializados pelo mutex da instância. Se a edição já tiver adquirido a seção crítica, ela pode efetivar-se antes da revogação; não há preempção nem garantia exactly-once. Em resposta perdida, não repetir a mutação automaticamente: ler/reconciliar o conteúdo e o diff antes de decidir qualquer nova chamada.
- Reinício/encerramento fecha a concessão local e os recursos temporários. `internal/mcp/oauth_write_lifecycle_test.go` fecha a composição, recria a identidade OAuth persistida com uma instância nova de `Grants`, mantém o JWT antigo criptograficamente verificável, nega o grant/sessão antigos e exige uma nova concessão explícita; isso é lifecycle de composição local, não aceite de restart operacional HTTPS. Não afirmar invalidação global do JWT nem isolamento contra escritores externos ou sandbox de processo.

### Transporte, limites e fronteira de outras operações

O transporte mantém `7676` como listener público e `7677` exclusivamente administrativo em loopback; o túnel nunca aponta para `7677`, e nenhuma rota administrativa é registrada no handler público. A composição `programming` já pode ser ativada localmente pelo comando explícito `connect quick programming`; HTTPS operacional, navegador real, grant ao ChatGPT e aceite externo continuam fora deste gate. `workspace.write` não concede execução de testes, shell ou mutação Git; a composição aprovada acrescenta somente revisão Git observacional. `ReplaceText` continua limitado a caminho relativo seguro, arquivo regular, UTF-8, conteúdo esperado, publicação atômica local e limites já documentados; concorrência externa e perda de resposta permanecem limites explícitos.

### Matriz do gate

| Controle exigido | Estado | Evidência atual | Condição para eventual promoção |
| --- | --- | --- | --- |
| Consentimento e combinações exatas de escopos | CONFIRMADO no harness | `internal/auth/write_scope_test.go`; combinações canônicas e texto de modificação | Revalidar no transporte escolhido sem ampliar o padrão |
| Escolha explícita do proprietário | CONFIRMADO no modo `programming` local; PENDENTE para aceite operacional externo | `CapabilityApproval` exige cliente emitido, seleção, resumo, identificador separado, confirmação única/expiração e chama `GrantWithScopes` somente depois; o console padrão não injeta a dependência | Preservar a mesma confirmação em eventual ativação HTTPS/ChatGPT, sem ampliar o conjunto de capacidades |
| Owner, cliente, workspace e sessão vinculados | CONFIRMADO local | `Grants.ReplaceText`, `AllowsClientScope` e testes MCP/OAuth | Preservar a mesma cadeia sem parâmetros JSON-RPC como autoridade |
| Revogação antes da emissão e durante o uso | CONFIRMADO local | revalidação em authorize/complete/token e negativa com JWT válido | Aceitar explicitamente a semântica sem invalidação global do JWT |
| Conflito e resposta perdida | CONFIRMADO para `ReplaceText`; PENDENTE operacional | conteúdo esperado, diff e testes de duplicação | Definir reconciliação no cliente/transporte antes de qualquer retry |
| Isolamento dos listeners | CONFIRMADO na composição atual | `cmd/signalspace/quick_ports_test.go` e testes de painel | Repetir em aceite HTTPS sem publicar `7677` |
| Ausência de exposição administrativa | CONFIRMADO local | testes de rotas públicas/admin e composição Quick | Manter roteadores e Host/Origin separados |
| Ausência de ampliação read → write | CONFIRMADO | regressão `cmd/signalspace/promotion_gate_test.go`, composição `programming` explícita e `Grant` read-only | Somente `connect quick programming` pode compor a porta; `diagnostic`/`read` permanecem sem writer |
| Limites de conteúdo, caminho e operação | CONFIRMADO local | `internal/workspace/edit_test.go` e testes MCP | Transportar os limites sem prometer sandbox ou exactly-once |
| Encerramento e reinício | CONFIRMADO localmente; PENDENTE operacional | `internal/mcp/oauth_write_lifecycle_test.go` fecha servidor, grant e identidade, recria a composição, nega a sessão antiga e aceita somente nova concessão/sessão; não é restart HTTPS | Exercitar ciclo completo no modo remoto descartável |
| HTTPS e navegador operacional | BLOQUEADO nesta missão | composição local e harness usam HTTP; nenhum túnel novo foi aberto | Nova autorização específica e aceite real sem bypass TLS |
| Autorização para ativação remota | PENDENTE | decisão aprovou somente a composição local opt-in; não houve grant ChatGPT Web nem aceite externo | Novo gate operacional no SHA exato, sem inferir permissão de túnel ou navegador |

A aprovação do gate não transforma a evidência local em aceite externo: HTTPS, navegador, grant real, workspace real, CI e deploy permanecem separados. A conclusão de `SS-MVP-002` continua condicionada à implementação e validação efetivamente registradas, não à decisão isolada.


## Direção aprovada de evolução — filesystem completo, Git tipado e UX por tool

Esta seção registra direção de produto/arquitetura aprovada pelo proprietário; **não descreve capacidades já implementadas** além da composição READ + WRITE + Git review documentada acima.

A superfície de filesystem deverá convergir para um motor único de workspace, com resolução e validação de caminho compartilhadas, no-symlink, limites, precondições de concorrência, publicação segura e revalidação de owner/client/session/escopo/grant em cada operação. Sobre esse motor, a superfície-alvo é:

| Família | Tools alvo | Observação de contrato |
| --- | --- | --- |
| inspeção/busca | `stat_path`, `list_directory`, `read_file`, `find_paths`, `search_text` | sem shell; resultados estruturados, limitados e pagináveis quando aplicável |
| criação/edição | `create_directory`, `create_text_file`, `replace_text`, `write_text_file` | create-only quando indicado; escrita integral exige precondição/versionamento, nunca overwrite silencioso |
| estrutura | `copy_path`, `move_path` | origem/destino dentro da mesma raiz autorizada; symlink não vira rota de escape |
| remoção | `delete_file`, `delete_directory` | remoção recursiva/destrutiva não fica escondida em um booleano genérico; exige contrato explícito |
| composição | `apply_patch` estruturado local | reutiliza o mesmo motor/autorização, preflighta operações fechadas, exige hashes para update/delete e compensa falhas; não cria filesystem paralelo |
| artifacts | import/export futuro | binários e arquivos grandes usam canal próprio, separados das tools textuais |

O Git evoluirá em camadas. `signalspace:git.review` permanece observacional. Git local mutável deverá ser uma capacidade distinta, com tools tipadas para stage/unstage, branch e commit, sem aceitar um comando Git arbitrário. Git remoto (fetch/push) exige fronteira própria para rede e credenciais. Hooks, helpers, pagers, editores, diffs externos e configuração/ambiente executável não podem transformar uma operação Git tipada em execução implícita. Reset destrutivo, clean, force-push e equivalentes permanecem fora da superfície normal até decisão separada.

Cada tool deverá publicar um resultado estruturado estável que possa alimentar UX especializada sem fazer a segurança depender do frontend. A direção visual é ter variantes por domínio — workspace, filesystem/search, diff/review, teste/processo e artifacts — reutilizando componentes quando possível. Não existe requisito de iframe/widget pesado para cada leitura ou mutação; plain MCP deve continuar suficiente para o modelo operar corretamente.

**Sequência reconciliada:** o motor/base tipado e as operações estruturais/destrutivas seguras foram concluídos localmente nesta retomada; o próximo bloco continua sendo Git local tipado, depois Git remoto, execução genérica/shell e superfícies visuais, cada qual com capacidade e gate próprios. O aceite HTTPS/ChatGPT Web continua um gate operacional distinto.

## Implementação local reconciliada — filesystem tipado base e estrutural — SS-MVP-002

Os slices base e estrutural da direção aprovada estão implementados localmente sobre o motor comum de workspace. A superfície MCP acrescenta onze tools, todas sob revalidação de owner, cliente, sessão, escopo e concessão na chamada. O código estrutural publicado está no commit `3628d2f`; o `apply_patch` desta missão permanece local até autorização específica de publicação:

| Família | Tools implementadas | Escopo | Estado |
| --- | --- | --- | --- |
| inspeção | `stat_path`, `find_paths`, `search_text` | `signalspace:workspace.read` | implementadas e validadas localmente |
| criação/edição | `create_directory`, `create_text_file`, `write_text_file` | `signalspace:workspace.write` | implementadas e validadas localmente |
| estrutura/remoção | `copy_path`, `move_path`, `delete_file`, `delete_directory` | `signalspace:workspace.write` | implementadas e validadas localmente; commits desta fatia ainda locais |

O motor comum normaliza caminhos relativos, rejeita traversal e symlink, limita profundidade/entradas/conteúdo, mantém resultados estruturados, aplica create-only onde indicado e exige hash/precondição para escrita integral. A publicação é local/atômica quando aplicável; falhas de autorização, tipo, limite ou precondição falham fechado. Nenhum tool recebe shell, comando arbitrário, raiz absoluta ou autoridade do frontend.

As regressões existentes de `read_file`, `list_directory`, `replace_text` e `review_git_changes` foram preservadas. A composição pública `programming` continua sendo a única composição opt-in que reúne READ + WRITE + Git review e agora a fatia estrutural e o `apply_patch`; `diagnostic`/`read` não ganham escrita; `test.run`, shell, Git mutável e worktree funcional permanecem fora desta fatia.

O registro de worktrees é apenas direção arquitetural/documental: worktree poderá ser uma futura superfície separada para isolamento, mas nenhuma foi criada, removida ou usada nesta missão. O código continua no checkout corrente e a segurança não depende de uma worktree.


## Filesystem estrutural — SS-MVP-002

O segundo slice implementa `copy_path`, `move_path`, `delete_file` e `delete_directory` sobre `internal/workspace/structural.go`, com wrappers de `Grants` e composição MCP explícita. A origem e o destino são sempre relativos à `Session`; a composição pública exige as quatro portas estruturais além das portas READ/WRITE/Git existentes, e cada chamada revalida JWT, owner, cliente, escopo, sessão e concessão corrente antes da mutação.

`copy_path` aceita somente arquivo regular ou árvore de diretórios/arquivos regulares, nunca sobrescreve, não segue symlinks nem copia tipos especiais, ordena a travessia e limita profundidade a `32`, entradas a `4096` e bytes a `64 MiB`. Arquivos são criados com `0600` e diretórios com `0700`; ACLs, xattrs e ownership não são prometidos. Se a árvore falhar, o resultado informa cópias, bytes, `partial` e `cleanup`; a limpeza é tentada apenas com descritores relativos/no-follow e estado desconhecido permanece explícito.

`move_path` usa `renameat2(..., RENAME_NOREPLACE)` relativo a descritores, rejeita destino existente, caminho equivalente e diretório dentro de si, e reporta `cross_device_unsupported` em `EXDEV` sem copiar/deletar silenciosamente. `delete_file` aceita somente arquivo regular; `delete_directory` aceita somente diretório vazio e nunca a raiz da sessão. Nenhuma operação é apresentada como transação contra escritores externos; revalidações de identidade e estados `unknown` evitam declarar sucesso quando a publicação não pôde ser confirmada.

Os testes estruturais cobrem traversal/absoluto, symlink de origem e componente, destino existente, tipos especiais, limites de profundidade/entradas/bytes, limpeza parcial, diretório não vazio, raiz, divergência owner/client/session, escopo ausente e revoke; o caso `EXDEV` permanece não testável nesta fixture local. O `apply_patch` cobre parser fechado, limites, preflight sem mutação, hashes obsoletos, criação/atualização/remoção e resultado estruturado; a composição `diagnostic`/`read` não anuncia write e não há shell, `test.run`, Git mutável, artifacts ou UI.

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

O harness MCP isolado injeta uma porta `WorkspaceGitReviewer` não exportada no `OAuthConfig`. A ferramenta `review_git_changes` recebe somente `session_id`, retorna baseline/estado atual, `status_changed`, `diff_changed` e completude, e nunca recebe raiz absoluta ou `Session`. A composição de teste usa `workspace.Grants.WithAuthorizedGitProcessDir`, que mantém observação e revogação serializadas pela concessão Git corrente. A composição pública `programming` também registra essa porta por meio de `NewOAuthProgrammingHandler`, com baseline por sessão e sem `add`, commit, push ou limpeza; a composição padrão continua sem Git.

Antes de cada leitura, o executor Git desativa `core.fsmonitor`, `core.untrackedCache` e atualizações opcionais do índice; `diff` usa `--no-ext-diff` e `--no-textconv`. Variáveis `GIT_CONFIG_*`, `GIT_ATTR_*`, `GIT_EXTERNAL_DIFF`, `GIT_DIFF_OPTS`, caminhos alternativos de índice/objetos, SSH/askpass/pager/editor e `GIT_EXEC_PATH` são removidas do ambiente filho; configuração global e de sistema são desabilitadas. A regressão `TestGitSnapshotNeutralizesExecutableConfigAndEnvironment` instala tentativas de fsmonitor/diff externo e confirma que nenhuma é executada. Configuração local não executável/extensões necessárias ao repositório não são tratadas como sandbox; o processo mantém os privilégios do usuário.

## Evidência OAuth integrada e próximos limites

O teste vertical descartável percorre leitura → edição → `go test ./...` → snapshot/diff → revogação → negação. `internal/mcp/workspace_write_test.go` cobre a fronteira MCP com tokens montados no harness; `internal/mcp/oauth_write_integration_test.go` acrescenta o fluxo real do próprio emissor OAuth, PKCE, aprovação local, troca por token assinado, `tools/list`, `replace_text`, diff observável no arquivo, revogação e código de uso único. `internal/mcp/git_review_integration_test.go` percorre fixture Git temporária → baseline → JWT/escopo de edição independente do escopo Git → `replace_text` → `review_git_changes` real → status/diff observável → revoke → deny, além de token ausente/inválido, leitura/escrita sem Git, owner/client/session divergentes, argumentos extras e ausência de repositório. `internal/mcp/test_runner_integration_test.go` acrescenta a composição `run_workspace_tests`, resultado de sucesso/falha/timeout, negativos de escopo e sessão, revogação e não exposição pública.

`internal/mcp/oauth_programming_integration_test.go` percorre no harness OAuth real DCR, PKCE, decisão terminal, `/authorize`, `/authorize/complete`, `/token` e JWT verificado por `NewStaticJWTVerifier`. A seleção local pede exatamente leitura, escrita, Git e teste; o baseline Git é capturado antes de qualquer edição. Tokens separados exercitam `read_file` sobre a fixture, usam o conteúdo retornado como precondição de `replace_text`, executam `go test ./...` e observam o diff Git. A cobertura inclui read-only sem escrita/teste/Git, tokens write/test/Git sem leitura, sessão incompatível, falta de concessão durante aprovação pendente, códigos de uso único, revoke com JWTs ainda válidos e nenhuma operação iniciada depois de revoke. O teste de composição pública `cmd/signalspace/programming_composition_test.go` acrescenta a verificação do modo opt-in: listas exatas, READ/WRITE/GIT efetivos, negativos entre escopos, revogação e ausência de `run_workspace_tests`/shell/mutação Git.

`internal/mcp/oauth_write_lifecycle_test.go` comprova a perda da concessão após fechamento/recriação local mesmo com JWT persistido e a exigência de nova sessão explícita. `internal/auth/write_scope_test.go`, `internal/auth/programming_scope_test.go` e os testes de composição OAuth integrada cobrem opt-in explícito, texto de consentimento, combinações canônicas, metadata padrão sem escopos de programação e revalidação antes da emissão. A promoção local opt-in ao `cmd/signalspace`/Quick foi implementada e validada; continuam pendentes navegador real, túnel novo, HTTPS operacional, grant ao ChatGPT Web, workspace real e CI. O processo de teste mantém privilégios do usuário e não é sandbox.

Os testes continuam sem navegador real, grant ao ChatGPT, workspace real, sandbox de processo, escritores externos ou integração com `feat/frontend-oauth-consent`; o transporte externo ainda requer aceite operacional próprio. O modo local `programming` é opt-in e permanece limitado a READ + WRITE + revisão Git observacional.

## Managed worktrees v1 — contrato local ainda não público

Esta fatia adiciona managed worktrees persistentes sob o diretório privado de estado, sem alterar a lista de ferramentas MCP. O console local oferece `request-worktree`, `approve-worktree`, `cancel-worktree`, `request-worktree-resume`, `approve-worktree-resume`, `worktrees` e `remove-worktree`; o lifecycle não é selecionável por JSON-RPC nem por parâmetros MCP. A aprovação continua distinta do consentimento OAuth e a cadeia owner → client → session → scope → `Grants` é revalidada na ativação e em cada ferramenta já existente.

O contrato usa `workspace_id` persistente e opaco, mas cria `session_id` novo ao ativar ou resumir. O checkout é detached em SHA local, não copia dirty/untracked da origem, rejeita filtros executáveis, submodules/gitlinks e associações inconsistentes, neutraliza hooks/configuração executável e não faz fetch, push, branch, commit ou limpeza. Revoke preserva a worktree; remove exige inatividade, associação íntegra e estado Git limpo. O diretório `.git` é reservado para as operações filesystem tipadas, com omissão controlada somente na listagem/varredura da raiz.

**Estado:** implementado e validado no escopo local descartável desta missão; não publicado, não exposto no MCP e não aceito como integração ChatGPT Web/HTTPS/Quick Tunnel/CI. O próximo aceite deve usar `SS-MVP-002` e manter o gate de publicação separado.

## Git local tipado v1 — status e índice

Esta fatia adiciona `git_status` como observação estruturada sob `signalspace:git.review`. O resultado contém paths relativos e estados separados (`tracked`, `untracked`, `staged`, `unstaged`, `deleted`, `modified`, `added`, `renamed`, `conflict`), além de `index_sha256` calculado sobre a saída limitada/canônica de `git ls-files --stage -z`. Não retorna raiz absoluta, conteúdo, `.git/index` ou stderr bruto.

`review_git_changes` preserva os campos anteriores e agora separa `staged_diff` de `diff` unstaged, com `staged_diff_changed`. A leitura staged continua somente observacional e usa `--cached`, sem alterar o índice.

`stage_git_paths` e `unstage_git_paths` são portas MCP experimentais sob o escopo independente `signalspace:git.index`. Recebem `session_id`, `expected_index_sha256` e lista fechada de paths literais; cada entrada pode carregar `expected_sha256` do arquivo ou `expected_index_oid` para uma remoção. O limite é 128 paths e 32 KiB de paths acumulados. Não existem pathspec glob, `add -A`, `add .`, `add -u`, `add -p`, reset, clean, commit, branch, merge, rebase, stash, fetch, pull, push, shell ou Git remoto.

As mutações só passam por `Grants.WithAuthorizedManagedGitProcessDir`: owner, cliente, sessão, escopo e associação `WorkspaceModeWorktree` gerenciada são revalidados sob o mesmo mutex. Checkout normal falha com `managed worktree required`. O runner usa argv fixo e stdin controlado (`--pathspec-from-file=-`, `--pathspec-file-nul`), neutraliza configuração global/system, hooks, pager/editor, fsmonitor, índices/objetos alternativos, askpass e SSH. Filtros executáveis, symlink, gitlink, conflito, tipo especial, path absoluto/traversal/glob/NUL, duplicata e precondições obsoletas falham fechado.

**Estado da fatia anterior:** implementado e validado localmente no commit `d1f46e7e704647a757d7329da09b53e416dcbd7d`; a composição corrente foi posteriormente ampliada pelos gates `git.index` e `git.commit` registrados abaixo. O handler isolado/test harness e o entrypoint público continuam sujeitos aos escopos independentes; não houve push desta sequência local.

## Promotion gate local — `SS-MVP-002-GIT-INDEX-PROMOTION-GATE-001` — 24/09/2026

O proprietário aprovou a promoção opt-in. Na ref local do commit `e164415`, `NewOAuthProgrammingHandler` exige portas distintas para `WorkspaceGitStatusReader` e `WorkspaceGitIndexMutator`; a composição injeta ambas a partir do adapter local já existente. `git_status` só aparece com `signalspace:git.review`; `stage_git_paths` e `unstage_git_paths` só aparecem com `signalspace:git.index`.

O emissor OAuth mantém `GitIndexScope` e `CanIssueGitIndex` independentes de `GitScope`, com validação antes de criar pedido, concluir consentimento e trocar código. O console local usa `NormalizeManagedCapabilities` apenas em `request-worktree` e `request-worktree-resume`; `request-programming` continua usando `NormalizeCapabilities` e rejeita `signalspace:git.index` antes de criar concessão. A mutação continua exigindo `WorkspaceModeWorktree`, managed workspace associada e revalidação owner → client → session → scope.

Regressões no commit local cobrem listas/instruções, escopos independentes, revoke, rejeição do checkout comum e uma fixture managed com status → stage → status staged → unstage, preservando os bytes do working tree. O estado é **IMPLEMENTADO E VALIDADO LOCALMENTE / PUBLICADO E CONFIRMADO NO REMOTO LIVE** em `60c290a88a5c85a411237b53e04313b6d35dfc19`; CI, HTTPS, túnel, navegador, workspace real, merge e deploy continuam desconhecidos/não exercitados.

## Commit gate local — `SS-MVP-002-GIT-COMMIT-GATE-001` — 24/09/2026

O proprietário aprovou a implementação local de `commit_git_index`, sem autorizar push ou mutação remota. O commit local `b3f2add` adiciona o escopo independente `signalspace:git.commit` e o schema fechado com exatamente `session_id`, `expected_head_oid`, `expected_index_sha256` e `message`; o hardening `247ad3d` corrige a exposição do escopo no snapshot, rejeita ref privada existente que não seja commit e revalida o índice antes do CAS. A composição pública `programming` só anuncia a tool quando o bearer tem o escopo verificado; diagnostic/read não a compõem.

O owner configura `workspace git-identity set <email> <display-name>`; a identidade fica em arquivo privado versionado por formato, com validação e permissão `0600`, e não entra em argumentos MCP, logs ou resultados. A operação exige owner → cliente → sessão → escopo, `WorkspaceModeWorktree`, associação ao manager e diretório real da managed worktree; a identidade ausente falha fechado.

O fluxo usa somente `git write-tree`, `git commit-tree` por stdin e uma transação única `git update-ref --stdin`: HEAD detached e `refs/signalspace/workspaces/<workspace_id>/head` avançam por CAS. A precondição combina HEAD esperado e SHA-256 do índice; conflitos, índice sem staged, merges, hooks, signing, configuração alternativa, checkout comum, ref ausente/inconsistente e falhas de reconciliação não são tratados como sucesso. Unstaged/untracked permanecem fora do commit; worktrees com commits locais não podem ser removidas.

**Validação local:** `go test ./... -p=1 -parallel=1 -count=1 -timeout=300s`, `go test -race ./internal/workspace ./internal/auth ./internal/mcp ./cmd/signalspace -p=1 -parallel=1 -count=1 -timeout=300s`, `go vet ./...`, `go build ./...`, `gofmt -l cmd internal` e `git diff --check` passaram. Os testes cobrem identidade privada, restart/resume, sessão antiga, diretório fora da managed worktree, staged-only com unstaged preservado, CAS stale-head, índice vazio, hooks não executados, ref privada inválida e independência OAuth/MCP.

**Estado:** `SS-MVP-002` permanece **PARCIAL quanto ao aceite operacional externo**; `SS-MVP-002-GIT-COMMIT-GATE-001` está **APROVADO / IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICADO REMOTAMENTE** em `0420c9a`. `SS-MVP-002-GIT-COMMIT-PUBLISH-001` foi concluído com desvio procedural de pré-checagem, sem impacto material no conteúdo publicado. CI, HTTPS, túnel, navegador, grant ChatGPT Web, workspace real, merge e deploy permanecem **DESCONHECIDOS/NÃO VALIDADOS**. Os patches protegidos continuam fora do Git, não aplicados e com os SHA-256 aprovados.

## Núcleo interno de capabilities e policy — `SS-MVP-002-LOCAL-CAPABILITIES-POLICY-V2-001`

`internal/capability` mantém o catálogo fechado de `workspace.read`, `workspace.write`, `workspace.delete`, `git.review`, `git.index`, `git.commit`, capabilities Git futuras e `test.run`/`shell.exec`. Os seis scopes OAuth legados aceitos pelo grant são adaptados explicitamente para esse catálogo; capabilities futuras não ganham scope OAuth por coincidência.

`workspace.Grants` agora armazena capabilities tipadas e faz a checagem interna com elas. `GrantWithCapabilities`, `GrantManagedWithCapabilities` e `AllowsClientCapability` são as portas tipadas; `GrantWithScopes`, `GrantManagedWithScopes`, `AllowsClientScope` e `GrantSnapshot.Scopes` continuam adaptadores de compatibilidade, enquanto `GrantSnapshot.Capabilities` é a representação interna explícita. O único mapeamento expansivo temporário é `signalspace:workspace.write → workspace.write + workspace.delete`, preservando o contrato histórico; um grant tipado somente `workspace.write` não autoriza delete. Não houve ampliação de acesso, nova tool ou mudança de composição.

`internal/policy` fornece um Policy Engine em memória, determinístico e fail-closed. O contexto exige owner, client, token family, workspace, sessão, capability, tool e fingerprint; regras podem ser `DENY`, `ASK`, `ALLOW_SESSION` ou `ALLOW_WORKSPACE`, com precedência por especificidade, negação em empate e expiração. O resultado é `ALLOW`, `DENY` ou `REQUIRE_APPROVAL`. Nesta fatia o engine não está ligado ao handler MCP: `REQUIRE_APPROVAL`, approvals, permits, persistência e painel permanecem gates posteriores.

## Arquitetura alvo v2 — autorização local por composição

`SS-MVP-002-LOCAL-AUTHORIZATION-V2-DESIGN-001` aceita a direção descrita em [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md). A superfície pública atualmente implementada/publicada continua sendo a matriz tipada registrada nesta documentação, com escopos e grants independentes. O auth harness opt-in agora implementa o ciclo OAuth v2 e a composição `signalspace:programming`, sem alterar discovery, autorização ou catálogo das tools públicas.

No alvo, OAuth solicita a composição (`signalspace:diagnostic`, `signalspace:read` ou `signalspace:programming`) e o SignalSpace decide internamente `workspace.read`, `workspace.write`, `workspace.delete`, `git.review`, `git.index`, `git.commit`, `test.run` e `shell.exec`. A policy deve considerar owner/client/workspace/session/tool/context e responder `ALLOW`, `DENY` ou `REQUIRE_APPROVAL`. Shell, Git remoto, branches e ações destrutivas permanecem capabilities e gates separados.

O aceite desta ADR não adiciona tool, scope, grant, UI ou lifecycle MCP. A implementação futura deve migrar a composição de modo controlado, preservar anotações/resultados estruturados e comprovar novamente discovery, OAuth, aprovação local, revogação, precondições e cleanup em fixture descartável.

## Approvals e policies locais — `SS-MVP-002-LOCAL-APPROVAL-POLICIES-UI-V2-001`

O slice implementa o contrato local sem alterar discovery ou o catálogo MCP. A fila de approvals aceita `ALLOW_ONCE`, `ALLOW_SESSION`, `ALLOW_WORKSPACE` e `DENY`; `ALLOW_ONCE` mantém permit interno de uso único, `ALLOW_SESSION` instala regra somente na memória da instância e `ALLOW_WORKSPACE` persiste somente para managed worktree estável. `DENY` terminaliza o pedido atual; nenhuma decisão executa ou repete a operação original.

O painel administrativo separa `/api/admin/v1/requests`, `/api/admin/v1/capability-approvals` e `/api/admin/v1/capability-policies`. As rotas de policy exigem sessão, CSRF e Host/Origin local; a UI respeita `allowed_decisions`, reconcilia perda de resposta e não trata erro como fila vazia. A policy mantém a exigência de grant ativo: autorização local não é grant OAuth nem escopo.

Estado: **IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICAÇÃO REMOTA PENDENTE** nesta missão. Não houve MCP bridge, nova tool/scope, Quick Tunnel, OAuth externo, navegador real, workspace real, CI, merge ou deploy.

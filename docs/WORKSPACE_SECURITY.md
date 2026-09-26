# Fronteira de workspace — leitura e filesystem tipado opt-in

## Discovery e autorização por ferramenta — 25/09/2026

Na composição opt-in `programming`, a lista de tools é estável por composição: bearer com somente `signalspace:diagnostic` descobre as capabilities já configuradas, mas não as autoriza. Cada chamada revalida bearer, issuer/audience, owner, client, scope específico, grant/session e managed worktree quando exigida. A ausência de scope produz `insufficient_scope` e challenge cumulativo; não há leitura, escrita, Git ou alteração de índice antes dessa verificação.

O estado está **IMPLEMENTADO E VALIDADO LOCALMENTE** no commit `c2dcdbd`, publicado remotamente em `14821b4`; o aceite Web no HEAD corrigido permanece pendente. Diagnostic/read não foram ampliados, não há scope/tool novo e os patches protegidos permanecem preservados.

**Estado de validação (25/09/2026):** `programming` é a composição opt-in para as tools de workspace tipadas sob os escopos independentes `signalspace:workspace.read` e `signalspace:workspace.write`, revisão Git, Git index e Git commit. O slice estrutural e o Git commit v1 permanecem publicados em seus SHAs históricos; o step-up corrente está em `c2dcdbd` e foi publicado no remoto em `14821b4`. O aceite externo desta composição foi tentado no HEAD anterior e ficou bloqueado antes do token; isso ainda não é aceite de produção nem valida workspace real, CI, merge ou deploy. `diagnostic` e `read` continuam sem write ou Git mutável.

## Aceite operacional externo programming — 25/09/2026

A missão usou somente fixture descartável e Quick Tunnel HTTPS real. O aplicativo MCP foi criado no ChatGPT Web e a decisão owner-side local do pedido OAuth inicial foi registrada. O fluxo, contudo, permaneceu em `Concluindo autorização…`, sem callback para a conversa e sem cliente OAuth com token emitido segundo `workspace clients`. A conta observada era Plus e o aplicativo não ficou disponível na composição da conversa para chamadas.

Não existe evidência de grant programming, sessão externa, managed worktree, READ/WRITE/filesystem, Git review/index/commit, revogação ou negação no ChatGPT Web. O estado é **BLOQUEADO/PARCIAL**, compatível com a fronteira fail-closed: não houve bypass, token fabricado, workspace real ou ampliação de capability. O túnel/processos e a fixture foram limpos. A retomada exige ambiente Web com suporte efetivo a escrita/modificação MCP e nova URL Quick Tunnel.

**Reconciliação posterior:** o promotion gate `SS-MVP-002-GIT-INDEX-PROMOTION-GATE-001` foi confirmado pelo ChatGPT Web como publicado no remoto live `60c290a88a5c85a411237b53e04313b6d35dfc19`. O `SS-MVP-002-GIT-COMMIT-GATE-001`, registrado em `b3f2add`, endurecido em `247ad3d` e reconciliado documentalmente em `4a103a6`, também está publicado no remoto. Essas fronteiras permanecem independentes e não promovem branch, Git remoto, shell ou `test.run` público.

## Estado e modos

`connect quick` continua oferecendo somente `connection_diagnostic`, e `connect quick read` continua oferecendo `read_file` e `list_directory` após confirmação local `PUBLICAR LEITURA`. A composição explicitamente opt-in `connect quick programming` possui seis tools READ/WRITE da base, quatro WRITE estruturais, `apply_patch`, `git_status`, `review_git_changes`, `stage_git_paths`, `unstage_git_paths` e `commit_git_index`, sempre com escopos independentes, managed worktree e concessão terminal-local. Isso não amplia os modos `diagnostic`/`read` e não habilita shell, `test.run`, branch ou Git remoto. A listagem e cada porta exigem composição explícita; não são habilitadas apenas por existir uma implementação no domínio. A URL Quick Tunnel é pública e temporária: use somente pastas descartáveis sem segredos. O modo OAuth integrado persistente e o modo bearer local não habilitam arquivos sem composição autorizada.

## Raiz e conteúdo

- `OpenApprovedRoot(root)` recebe somente caminho absoluto canônico aprovado pelo operador local. Recusa `/` e o diretório home como raízes amplas, caminhos relativos, traversal e symlinks. Não há raiz padrão nem autorização por parâmetro HTTP.
- A raiz é aberta componente a componente a partir de `/` com descritores `openat` e `O_NOFOLLOW`. A sessão mantém o descritor e um ID aleatório até revogação ou shutdown, inclusive quando o pathname da raiz é substituído posteriormente.
- `ReadText(relative)` aceita apenas caminho relativo canônico. Recusa `..`, `.`, componente vazio, caminho absoluto, separadores ambíguos, symlinks intermediários ou finais e objetos que não sejam arquivos regulares.
- O retorno de leitura é texto UTF-8, sem byte NUL, com no máximo **32 KiB**. Recusa binários e arquivos excessivos; falhas do MCP não expõem caminhos locais ou conteúdo. Esta proteção de arquivos não é sandbox de processos: hardlinks, montagens e modificações concorrentes por outros processos do usuário não são isolados.
- `Session.ListDirectory(relative)` e `Grants.ListDirectory(owner, clientID, sessionID, relative)` aceitam `.` para a raiz ou caminho relativo canônico. Abrem diretórios com descritores e `O_NOFOLLOW`, retornam apenas nomes UTF-8 em ordem lexicográfica, sem conteúdo, tipos ou caminhos absolutos. Nomes de links podem aparecer, mas os links não são seguidos. A listagem retorna erro sem dados parciais se houver mais de **128 entradas** ou nomes inválidos; exige a concessão atual e é invalidada por substituição, revogação e shutdown. Não há garantia de snapshot atômico contra modificações externas concorrentes.
- A implementação é específica para Linux e usa a biblioteca padrão do Go. Conteúdo e nomes listados são dados não confiáveis, nunca instrução adicional para o assistente.

| Condição | Erro interno |
| --- | --- |
| Raiz implícita, `/` ou home | `ErrInvalidRoot` |
| Symlink ou componente não-diretório | `ErrUnsafePath` |
| Traversal ou caminho não canônico | `ErrInvalidPath` |
| Arquivo inexistente | erro compatível com `os.ErrNotExist` |
| Arquivo não regular | `ErrNotFile` |
| Mais de 32 KiB | `ErrTooLarge` |
| Binário ou UTF-8 inválido | `ErrNotText` |
| Mais de 128 entradas na listagem | `ErrTooManyEntries` |
| Sessão encerrada | `ErrClosed` |

## Consentimento local e cliente

Somente o operador do **stdin local** pode criar ou revogar uma concessão. Após completar OAuth até a emissão do primeiro token, `workspace clients` lista os `client_id` elegíveis; registro dinâmico, autorização pendente ou aprovação sem troca do código não bastam. Nomes informados pelo cliente são metadados não atestados: `client_id` identifica um registro OAuth, **não** prova que o software seja o ChatGPT e **não** identifica conversas.

O operador digita `workspace request <client-id> <raiz-absoluta-canônica>`. O terminal mostra o ID do cliente e o caminho exato, além de `workspace approve <id>` ou `workspace cancel <id>`. O pedido expira em **2 minutos**, não abre a raiz e não cria concessão antes da confirmação. Uma nova solicitação pendente substitui a anterior; uma nova concessão revoga a sessão anterior. `workspace revoke <session-id>` e o encerramento do processo invalidam a concessão. No máximo uma sessão fica ativa por instância.

`workspace.Grants` vincula o proprietário `OwnerSubject`, o cliente OAuth escolhido e o ID de sessão. Leitura e listagem exigem os três identificadores corretos. `AllowsClient` consulta apenas a concessão corrente para o emissor OAuth; não revela o ID de sessão nem o caminho. Outro cliente do mesmo proprietário não reutiliza a concessão, mesmo conhecendo o session ID. Conversas diferentes podem reutilizar a autorização somente se empregarem o **mesmo registro OAuth**, tiverem token de leitura válido e receberem o session ID; o histórico não é transferido entre chats.

## Escopo OAuth e ferramentas MCP

O emissor padrão anuncia e assina apenas `signalspace:diagnostic`. No modo `connect quick read`, a configuração adicional exata `ReadScope=signalspace:workspace.read` e `CanIssueRead` ligada à **mesma instância** `Grants` exige concessão ativa para o cliente selecionado. O cliente precisa solicitar exatamente `signalspace:diagnostic signalspace:workspace.read`, obter novo consentimento OAuth com cliente e escopos e receber aprovação adicional no terminal. Pedidos inválidos, isolados, repetidos, reordenados ou sem concessão são recusados. A concessão é verificada no início da autorização, antes de emitir código e antes de assinar token.

`read_file` requer `WorkspaceReader`; `list_directory` requer também `WorkspaceLister`. Ambas são injetadas pelo ponto de composição somente no modo opt-in, com a mesma instância `Grants`. Sem essas portas, `tools/list` oferece somente diagnóstico; com apenas `WorkspaceReader`, a listagem segue indisponível. Uma configuração somente de `WorkspaceLister` é recusada durante a criação do servidor.

As duas ferramentas exigem `session_id` e `path` como strings não vazias, sem argumentos extras, e verificam **em cada chamada** `VerifyIdentity`: JWT assinado, emissor, audiência, expiração, proprietário, `client_id` e escopo `signalspace:workspace.read`. A identidade verificada é passada à concessão atual, que revalida proprietário, cliente, sessão e caminho. Tokens legados sem `client_id`, bearer somente de diagnóstico, cliente divergente, sessão ausente ou revogada e caminhos hostis não obtêm dados. Não há endpoint HTTP para abrir ou ampliar raízes.

`list_directory` aceita `path: "."` apenas como representação da raiz aprovada; demais paths são relativos e canônicos. Uma listagem bem-sucedida retorna um único conteúdo de texto JSON: `{"entries":["arquivo.txt","src"]}`. Diretório vazio retorna `{"entries":[]}`. Os nomes são ordenados; não retornam tipos, conteúdo de arquivos nem caminhos absolutos. Todas as falhas de domínio, incluindo diretório inexistente, caminho hostil, excesso de entradas e concessão revogada, retornam `isError: true` e exatamente `Workspace directory listing unavailable or not authorized.` sem lista parcial. Argumentos malformados retornam erro JSON-RPC `Invalid params`. Falhas de escopo retornam desafio MCP para `signalspace:workspace.read`.

**Revogação:** um JWT assinado pode continuar válido até expirar, mas cada operação consulta a concessão. Após `workspace revoke`, token ainda válido não obtém mais texto nem nomes. Leitura, listagem e revogação compartilham mutex para impedir retorno da revogação enquanto operações anteriores estiverem ativas. A raiz exata não é retornada pelo MCP; o operador deve informar manualmente o ID de sessão ao chat. Esse ID isolado não é token de acesso.

## Operações estruturais

As quatro operações estruturais reutilizam a mesma `Session` e o mesmo mutex de `Grants`. `copy_path` cria somente arquivos `0600` e diretórios `0700`, com no máximo profundidade `32`, `4096` entradas e `64 MiB`; a travessia é ordenada, não segue symlinks e rejeita tipos especiais. Falha em árvore não é atomicidade: o resultado informa `partial` e `cleanup`, e limpeza não confirmada permanece `unknown`.

`move_path` usa `renameat2` relativo a descritores com `RENAME_NOREPLACE`; não faz fallback de `EXDEV` para copy+delete. `delete_file` só remove arquivo regular e `delete_directory` só remove diretório vazio, sem aceitar a raiz ou `recursive`. Cada operação revalida owner, cliente, sessão, `workspace.write` e concessão corrente antes da mutação, sem prometer isolamento contra escritores externos, exactly-once ou transação filesystem.

## Evidência, limites e teste seguinte

Testes em `internal/workspace/session_test.go`, `grants_test.go`, `list_test.go` e `grants_concurrency_test.go` cobrem limites, symlink, traversal, substituição da raiz, cliente, revogação e operações concorrentes. Testes de console cobrem pedido e confirmação separados, cancelamento, expiração e troca. Testes HTTP/MCP em `internal/mcp` exercitam JWT/JWKS, escopo, autorização e erros. `cmd/signalspace/embedded_read_test.go` integra OAuth real local, leitura e revogação; `embedded_list_test.go` verifica a composição da listagem no modo opt-in, consentimento, resposta e negativa após revogação. Inicialização do Quick Tunnel usa `cloudflared` simulado. A [CI #67](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35512026134) passou em formatação, testes, detector de corrida, `go vet` e build no commit `afe4daf`.

**Smoke de leitura relatado pelo proprietário em 20/09/2026:** `read_file` retornou exatamente `Teste de leitura SignalSpace` para `readme.txt` com leitura aprovada; após revogação relatada, uma chamada com os mesmos argumentos retornou `Workspace read unavailable or not authorized.`. Nenhum outro arquivo foi solicitado nesse teste. Logs de correlação, estado exato do JWT e confirmação da pasta descartável não foram anexados. Não extrapolar para isolamento entre chats ou clientes.

**Smoke de listagem relatado pelo proprietário em 20/09/2026, no ChatGPT Web real:** a branch `feat/m1-local-mcp-diagnostic` foi atualizada por fast-forward até `afe4daf`, com `go test ./...` aprovado localmente e alterações de outra sessão preservadas. O operador iniciou `connect quick read`, confirmou `PUBLICAR LEITURA`, obteve uma nova URL `/mcp` verificada via Quick Tunnel real e concluiu consentimento e concessão local. Com `session_id` ativo (omitido desta documentação), `list_directory(path=".")` devolveu exatamente `{"entries":["nested","readme.txt"]}`; `list_directory(path="nested")` devolveu `{"entries":["inside.txt"]}`. Após `workspace revoke`, a repetição com o mesmo ID retornou `is_error: true` e `Workspace directory listing unavailable or not authorized.`, sem nomes. Segundo o proprietário, o mesmo JWT ainda não havia expirado; isso foi relatado, não verificado independentemente aqui.

**Correlação e alcance da evidência:** o proprietário informou que o log integral do terminal contém duas decisões de autorização, `Authenticated MCP tool discovery served: tools/list`, `connection_diagnostic handled`, criação da concessão e sua revogação. O backend não registra cada `list_directory` individualmente; os nomes observados na resposta da ferramenta e sua negativa após revogação são a evidência específica dessas chamadas, mas não constituem um registro dedicado de invocação no servidor. O relato demonstra o comportamento naquela sessão e não atesta criptograficamente a identidade do software cliente nem prova isolamento entre conversas ou usuários.

**Ainda não validado:** comportamento de erro e estrutura de resposta em produção multiusuário, refresh e revogação OAuth, atestação do software cliente, `read_file` na mesma execução atual da UI, nova concessão após revogação na UI real, negações por outro cliente na conexão pública e interoperabilidade em outras contas/sessões. Quick Tunnel não oferece promessa de produção. Não habilitar shell, branch ou Git remoto com base nesses testes; escrita e Git local permanecem condicionados ao fluxo `programming` e à managed worktree aprovada.

## Managed worktrees v1 — lifecycle local do proprietário

`SS-MVP-002` agora possui uma fatia local de managed worktrees v1 em `internal/workspace/managed_worktree.go`. Ela é separada da superfície MCP: não existem `create_worktree`, `remove_worktree`, `open_workspace` ou `switch_workspace` públicos. O proprietário usa somente o console stdin local para solicitar, aprovar, cancelar, resumir, listar e remover; cada solicitação revalida cliente OAuth emitido e escopos antes de alterar estado.

`workspace_id` é opaco e persistente no estado privado; `session_id` continua efêmero e é criado novamente a cada ativação/resume. `Grants` mantém no máximo uma sessão corrente. Revogar fecha a sessão e preserva a worktree; retomar exige nova aprovação e nova sessão. Fechar o processo também preserva a worktree. Remoção só ocorre para worktree associada, existente, inativa, consistente e limpa de alterações rastreadas, staged e não rastreadas; quando HEAD contém commits locais além de `BaseSHA`, a remoção também é negada, mesmo que o working tree esteja limpo. Não há `worktree prune`, `clean`, reset destrutivo ou remoção forçada normal.

Na criação, a origem precisa ser um repositório Git local canônico, não symlink, não home-wide, não `/`, não uma worktree vinculada e fora do diretório de estado. A base é uma ref local resolvida para SHA imutável; não há fetch, pull, branch ou rede. A worktree é criada detached com `--no-checkout` e materializada depois; alterações sujas e não rastreadas não são copiadas e ficam registradas somente como `dirty_source`. Metadados ficam fora da raiz editável, são versionados e escritos atomicamente com permissões privadas; estados ausente, sujo, stale e inconsistente falham fechado e não são auto-reparados.

O runner Git usa argv fixo, timeout e limite de saída, desativa terminal/pager/editor/fsmonitor/untracked cache/hooks, impede configuração global/system e não usa shell. Filtros `clean`/`smudge`/`process`, submodules/gitlinks e associações Git inconsistentes são rejeitados; não há execução de LFS. Isso não é sandbox: processos autorizados mantêm os privilégios do usuário.

O filesystem reserva o componente exato `.git` em leitura, stat, busca, criação, escrita, cópia, movimento, remoção e `apply_patch`; `list_directory(".")`, `find_paths` e `search_text` omitem esse componente. `.gitignore`, `.gitattributes` e `.gitmodules` continuam nomes normais quando não representam um gitlink. A implementação foi exercitada somente com fixtures descartáveis; não comprova worktree real do proprietário, HTTPS, túnel, ChatGPT Web, CI, merge ou deploy.

## Git index v1 — fronteira de mutação local

O escopo `signalspace:git.index` é independente de leitura de workspace, escrita textual, revisão Git, teste, shell e Git remoto. `git_status` permanece em `signalspace:git.review`; stage/unstage não são consequência desse escopo nem de `workspace.write`. A mutação exige sessão de worktree criada e associada pelo manager, nunca um checkout escolhido diretamente.

Antes de `git add`, cada path é validado sem symlink nos componentes, sem `.git`, sem diretório/tipo especial, sem gitlink ou estado unmerged. O índice precisa corresponder ao `expected_index_sha256`; arquivos regulares exigem SHA-256 atual e deleções exigem OID de índice. A operação revalida filtros executáveis imediatamente antes do add. `unstage` usa somente `git restore --staged --source=HEAD` para paths validados e não recebe `--worktree`; a prova mantém os bytes do working tree.

Os retornos distinguem sucesso, `failed_no_change` e `partial_or_unknown`, incluindo hashes anterior/novo e paths relativos. Uma falha ou resposta perdida não é tratada como transação externa: o índice é reobservado e não há reset/restore/checkout/clean/stash automático. O processo Git continua com privilégios do usuário; neutralização de hooks/configuração não é sandbox.

**Limite de publicação:** o entrypoint público `connect quick programming` injeta e anuncia stage/unstage somente para bearer com `signalspace:git.index`, em managed worktree aprovada; Git commit segue a mesma fronteira com `signalspace:git.commit`. O slice foi exercitado em repositórios temporários/worktrees gerenciadas; o aceite operacional externo desta missão ainda não é garantia de produção, workspace real, CI ou deploy.

## Promotion gate local — Git index — 24/09/2026

O proprietário aprovou `SS-MVP-002-GIT-INDEX-PROMOTION-GATE-001`. Na ref local do commit `e164415`, a composição opt-in `programming` anuncia `git_status` somente sob `signalspace:git.review` e `stage_git_paths`/`unstage_git_paths` somente sob `signalspace:git.index`. `GitIndexScope` possui validador OAuth próprio; pedir o escopo, encontrá-lo na metadata ou apresentar bearer com a string não cria grant local.

O console genérico `request-programming` continua rejeitando `signalspace:git.index`. Somente `request-worktree` e `request-worktree-resume`, ambos owner-side e locais, usam `NormalizeManagedCapabilities` para aceitar o escopo. Cada mutação ainda passa por `WithAuthorizedManagedGitProcessDir`, exigindo owner, cliente, sessão, escopo e associação persistente de managed worktree; checkout comum continua falhando fechado. Não foram adicionados lifecycle MCP/HTTP, commit, branch, reset, clean, stash, fetch, pull, push, shell ou CI.

O gate está **IMPLEMENTADO E VALIDADO LOCALMENTE / PUBLICADO E CONFIRMADO NO REMOTO LIVE** em `60c290a88a5c85a411237b53e04313b6d35dfc19`. O Git commit v1 também está publicado em `0420c9a` e permanece independente do Git index. HTTPS, túnel, navegador, workspace real, CI, merge e deploy continuam desconhecidos e não são inferidos a partir dos testes locais.

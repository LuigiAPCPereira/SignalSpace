# Fronteira de workspace — leitura experimental (M2)

## Estado e modos

O `connect quick` continua oferecendo somente `connection_diagnostic`. A leitura MCP é habilitada **apenas** pelo comando distinto `connect quick read`, que exige a confirmação local `PUBLICAR LEITURA`. A URL Quick Tunnel é pública e temporária: não use pastas reais, pessoais ou contendo segredos. Em 20/09/2026, o proprietário relatou uma leitura positiva e uma negativa após revogação pelo ChatGPT; os limites dessa observação estão registrados abaixo. Git, escrita, edição e shell permanecem indisponíveis. O modo OAuth integrado persistente e o modo bearer local não habilitam leitura.

## Raiz e conteúdo

- `OpenApprovedRoot(root)` recebe somente caminho absoluto canônico aprovado pelo operador local. Recusa `/` e o diretório home como raízes amplas, caminhos relativos, traversal e symlinks. Não há raiz padrão nem autorização por parâmetro HTTP.
- A raiz é aberta componente a componente a partir de `/` com descritores `openat` e `O_NOFOLLOW`. A sessão mantém o descritor e um ID aleatório até revogação ou shutdown, inclusive quando o pathname da raiz é substituído posteriormente.
- `ReadText(relative)` aceita apenas caminho relativo canônico. Recusa `..`, `.`, componente vazio, caminho absoluto, separadores ambíguos, symlinks intermediários ou finais e objetos que não sejam arquivos regulares.
- O retorno é texto UTF-8, sem byte NUL, com no máximo **32 KiB**. Recusa binários e arquivos excessivos; falhas do MCP não expõem caminhos locais ou conteúdo. Esta proteção de arquivos não é sandbox de processos: hardlinks, montagens e modificações concorrentes por outros processos do usuário não são isolados.
- `Session.ListDirectory(relative)` e `Grants.ListDirectory(owner, clientID, sessionID, relative)` são **somente APIs internas**, ainda não publicadas no MCP. Aceitam `.` para a raiz ou caminho relativo canônico, abrem diretórios via descritores com `O_NOFOLLOW` e retornam apenas nomes UTF-8 em ordem lexicográfica, sem conteúdo, tipos ou caminhos absolutos. Nomes de links podem aparecer, mas os links não são seguidos. A listagem retorna erro sem dados parciais se houver mais de **128 entradas** ou nomes inválidos; exige a mesma concessão atual da leitura de texto e é invalidada por substituição, revogação e shutdown. Não há garantia de snapshot atômico contra modificações externas concorrentes.
- A implementação é específica para Linux e usa a biblioteca padrão do Go. Conteúdo de arquivo e nomes listados são dados não confiáveis, nunca instrução adicional para o assistente.

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

`workspace.Grants` vincula o proprietário `OwnerSubject`, o cliente OAuth escolhido e o ID de sessão. Leitura exige todos os três identificadores corretos. `AllowsClient` consulta apenas a concessão corrente para o emissor OAuth; não revela o ID de sessão ou o caminho. Outro cliente do mesmo proprietário não reutiliza a concessão, mesmo conhecendo o session ID. Conversas diferentes podem reutilizar a autorização somente se empregarem o **mesmo registro OAuth**, tiverem token de leitura válido e receberem o session ID; o histórico não é transferido entre chats.

## Escopo OAuth e ferramenta MCP

O emissor padrão anuncia e assina apenas `signalspace:diagnostic`. No modo `connect quick read`, a configuração adicional exata `ReadScope=signalspace:workspace.read` e `CanIssueRead` ligada à **mesma instância** `Grants` exige concessão ativa para o cliente selecionado. O cliente precisa solicitar exatamente `signalspace:diagnostic signalspace:workspace.read`, obter **novo consentimento OAuth** que mostra cliente e escopos e receber aprovação adicional no terminal. Pedidos inválidos, isolados, repetidos, reordenados ou sem concessão são recusados. A concessão é verificada no início da autorização, antes de emitir código e antes de assinar token.

A ferramenta `read_file` só é injetada no modo opt-in via `OAuthConfig.WorkspaceReader`; a ausência da porta mantém `tools/list` somente diagnóstico. Ela exige `session_id` e `path` relativo. Em **cada** chamada, `VerifyIdentity` valida JWT assinado, emissor, audiência, expiração, proprietário, `client_id` e escopo `signalspace:workspace.read`. Somente a identidade verificada é passada à concessão, que revalida proprietário, cliente, sessão e path. Tokens legados sem `client_id`, bearer somente de diagnóstico, cliente divergente, sessão ausente ou revogada e paths hostis não recebem conteúdo. Não há endpoint HTTP para abrir ou ampliar raízes.

**Revogação:** um JWT assinado pode continuar válido até expirar, mas a ferramenta consulta a concessão em toda leitura. Após `workspace revoke`, um token ainda válido não obtém mais texto. Leitura e revogação compartilham mutex para impedir retorno de revogação enquanto há leitura ativa. A raiz exata não é retornada pelo MCP; o operador deve informar manualmente o ID de sessão ao chat se desejar a leitura. O session ID isolado não é token de acesso.

## Evidência, limites e teste seguinte

Os testes em `internal/workspace/session_test.go`, `grants_test.go` e `list_test.go` cobrem limites de arquivo e diretório, symlink, traversal, substituição da raiz, cliente e revogação. Os testes do console cobrem pedido e confirmação separados, cancelamento, expiração e troca. Testes HTTP/MCP em `internal/mcp` exercitam JWT/JWKS, escopo e negativas. `cmd/signalspace/embedded_read_test.go` integra registro OAuth, token de diagnóstico, concessão por stdin local, consentimento separado, token real de leitura, leitura de arquivo descartável, cliente divergente e revogação com token ainda válido. O teste de inicialização do Quick Tunnel usa `cloudflared` simulado e verifica a publicação apenas após `PUBLICAR LEITURA`.

**Smoke real relatado em 20/09/2026:** a ferramenta `read_file` retornou exatamente `Teste de leitura SignalSpace` para `readme.txt` com leitura aprovada; após o proprietário relatar a revogação, uma chamada com o mesmo `session_id` e `path` retornou `Workspace read unavailable or not authorized.`. Nenhum outro arquivo foi solicitado nesse teste. Isso confirma as respostas observadas nessa sessão, mas os logs de correlação do servidor, o estado exato de expiração do JWT e a configuração da pasta descartável não foram anexados. Não extrapolar para isolamento entre chats ou outros clientes.

**Ainda não validado:** nova concessão após revogação na UI real, negações por cliente diferente na conexão pública, interoperabilidade em outras contas/sessões e listagem via MCP (não implementada). Reproduzir novos smoke tests somente em pasta descartável, com consentimento conferido e nenhum segredo. O Quick Tunnel não oferece promessa de produção, refresh/revogação OAuth nem atestação criptográfica do cliente. Não habilitar escrita, Git ou shell com base nestes testes de leitura.

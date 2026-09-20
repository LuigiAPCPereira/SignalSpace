# Fronteira de workspace: leitura de texto (fatia M2 interna)

**Estado:** `internal/workspace` implementa sessões com raiz restrita e um gerenciador de concessão revogável. `connect quick` oferece **pedido e confirmação separados somente pelo terminal local**. **Não há ferramenta MCP de workspace, leitura remota, Git, edição ou shell nesta entrega.** A prova de conectividade do M1 não autoriza implicitamente nenhum diretório.

## Comportamento implementado

- `OpenApprovedRoot(root)` aceita somente uma raiz absoluta e canônica indicada por uma fronteira **local** confiável. Rejeita raiz `/` e o diretório home como raiz ampla. Não possui caminho padrão, não lê variáveis de ambiente para obter uma raiz e não autoriza nada por solicitação HTTP.
- Abre cada componente da raiz a partir de `/` por descritores com `openat`, exigindo diretórios reais e `O_NOFOLLOW`. A sessão conserva o descritor da raiz e cria `ID()` aleatório, estável apenas durante sua existência.
- `ReadText(relative)` exige caminho relativo canônico: recusa `..`, caminhos absolutos, componentes vazios, `.` e separadores ambíguos. Percorre os componentes a partir do descritor da raiz e rejeita symlinks em diretórios intermediários e no arquivo final.
- Lê apenas arquivo regular, no máximo **32 KiB**, UTF-8 válido e sem byte NUL. Arquivos binários, diretórios e tamanho excessivo são recusados; erros não incluem conteúdo do arquivo.
- `Close()` revoga a sessão e fecha o descritor. Uma sessão existente permanece vinculada ao inode original se o pathname da raiz for substituído depois da abertura.

A implementação atual é **específica para Linux**, usa exclusivamente a biblioteca padrão de Go e não promete snapshot imutável contra mudanças concorrentes feitas por outros processos do mesmo usuário.

## Semântica de falhas

| Condição | Resultado |
| --- | --- |
| Raiz implícita, `/` ou home | `ErrInvalidRoot` |
| Symlink ou componente não-diretório | `ErrUnsafePath` |
| Traversal, absoluto ou caminho não canônico | `ErrInvalidPath` |
| Arquivo inexistente | erro compatível com `os.ErrNotExist` |
| Diretório ou arquivo não regular | `ErrNotFile` |
| Mais de 32 KiB | `ErrTooLarge` |
| Conteúdo binário ou UTF-8 inválido | `ErrNotText` |
| Sessão encerrada | `ErrClosed` |

## Aprovação local implementada em `connect quick`

Após a aprovação HTTPS, somente o operador do **stdin local** pode digitar `workspace request /caminho/absoluto/exato`. O terminal devolve o caminho entre aspas, um ID aleatório e um comando separado `workspace approve <id>` ou `workspace cancel <id>`. O pedido expira em dois minutos. Nada é aberto ou concedido durante o pedido; somente a confirmação tenta abrir a raiz via `OpenApprovedRoot`. Raiz `/`, home, caminho relativo, traversal e symlink são recusados. Um segundo pedido substitui o anterior; uma segunda aprovação revoga a sessão anterior. `workspace revoke <session-id>` revoga a concessão ativa. O encerramento normal fecha o descritor.

O gerenciador mantém **uma sessão por instância**, vincula-a ao `OwnerSubject` conhecido da composição OAuth e exige a identidade e o ID exatos para operações internas de leitura. O subject do OAuth embutido representa a mesma identidade de proprietário, **não um cliente externo criptograficamente atestado ou isolamento individual entre clientes**. A concessão não é exibida por HTTP e nenhum endpoint de leitura foi habilitado. Não usar pasta com dados pessoais para demonstrações desta fatia; prefira um diretório descartável.

## Fronteiras pendentes antes de publicar qualquer ferramenta

A exposição MCP ainda precisa receber a identidade autenticada do verificador, impor escopo próprio de leitura, vincular sessão à autorização corrente e negar ID desconhecido ou revogado em testes ponta a ponta. Uma URL pública, um bearer OAuth de diagnóstico ou argumento de ferramenta **não escolhe ou amplia a raiz**. A seleção de projetos, leitura pelo ChatGPT e testes de negação remotos ainda não foram realizados.

A aprovação de uma raiz para arquivos **não confina processos**. Shell, Git e edição permanecem indisponíveis, e não há promessa de sandbox. Não usar Quick Tunnel experimental para expor a futura leitura de dados privados antes de concluir os controles e o teste real com projeto descartável.

## Evidência automatizada

Testes em `internal/workspace/grants_test.go` e `cmd/signalspace/workspace_console_test.go` cobrem concessão inexistente, identidade divergente, confirmação separada, cancelamento, expiração, raízes amplas, revogação, substituição e encerramento. Testes em `internal/workspace/session_test.go` cobrem texto permitido, IDs de sessão, raízes amplas ou symlink, traversal, links intermediários e finais, limites de tamanho, binário, diretório, inexistência, fechamento e substituição de pathname depois da abertura. Isso verifica a camada interna, não uma autorização pelo proprietário nem a execução no ChatGPT Web.

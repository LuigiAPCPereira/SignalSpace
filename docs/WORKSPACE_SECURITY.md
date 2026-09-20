# Fronteira de workspace: leitura de texto (fatia M2 interna)

**Estado:** `internal/workspace` implementa uma sessão de leitura restrita a uma raiz fornecida pelo controle local, com testes negativos. **Não há aprovação de raiz via terminal, ferramenta MCP de workspace, leitura remota, Git, edição ou shell nesta entrega.** A prova de conectividade do M1 não autoriza implicitamente nenhum diretório.

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

## Fronteiras pendentes antes de publicar qualquer ferramenta

A composição local deve apresentar o **caminho real exato** escolhido pelo usuário e pedir aprovação explícita no terminal. Só depois poderá chamar `OpenApprovedRoot`; uma URL pública, um bearer OAuth válido ou um argumento de ferramenta **não podem escolher ou ampliar a raiz**. A exposição MCP precisará associar a sessão à autorização corrente, recusar IDs desconhecidos e revogar as sessões quando a autorização terminar. A seleção de projetos sob uma raiz aprovada, leitura de arquivo real pelo ChatGPT e testes de negação ponta a ponta ainda não foram realizados.

A aprovação de uma raiz para arquivos **não confina processos**. Shell, Git e edição permanecem indisponíveis, e não há promessa de sandbox. Não usar Quick Tunnel experimental para expor a futura leitura de dados privados antes de concluir os controles e o teste real com projeto descartável.

## Evidência automatizada

Testes em `internal/workspace/session_test.go` cobrem texto permitido, IDs de sessão, raízes amplas ou symlink, traversal, links intermediários e finais, limites de tamanho, binário, diretório, inexistência, fechamento e substituição de pathname depois da abertura. Isso verifica a camada interna, não uma autorização pelo proprietário nem a execução no ChatGPT Web.

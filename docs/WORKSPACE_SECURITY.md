# Fronteira de workspace: leitura de texto (fatia M2 interna)

**Estado:** `internal/workspace` implementa sessões com raiz restrita e um gerenciador de concessão revogável. `connect quick` oferece **pedido e confirmação separados somente pelo terminal local**. **A composição executável ainda não habilita ferramenta MCP de workspace, leitura remota, Git, edição ou shell.** A prova de conectividade do M1 não autoriza implicitamente nenhum diretório.

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

O servidor OAuth agora inclui o `client_id` do cliente registrado no JWT assinado. O verificador fornece `VerifyIdentity`, que valida assinatura, emissor, audiência, validade, proprietário e o escopo **solicitado pelo chamador**, recusando tokens sem identificador válido. O método legado `Verify` continua aceitando tokens de diagnóstico antigos sem `client_id` para não quebrar a conexão já existente. `client_id` identifica um registro OAuth, **não atesta que o software cliente seja o ChatGPT**. Não há permissão remota implícita: `VerifyIdentity` ainda não é usado para liberar arquivos, e a concessão local ainda não seleciona um `client_id` específico.

## Fronteiras pendentes antes de publicar qualquer ferramenta

A exposição MCP ainda precisa receber a identidade autenticada do verificador, impor escopo próprio de leitura, vincular sessão à autorização corrente e negar ID desconhecido ou revogado em testes ponta a ponta. Uma URL pública, um bearer OAuth de diagnóstico ou argumento de ferramenta **não escolhe ou amplia a raiz**. A seleção de projetos, leitura pelo ChatGPT e testes de negação remotos ainda não foram realizados.

O emissor integrado **continua emitindo somente `signalspace:diagnostic`**. O teste do verificador com `signalspace:workspace.read` usa um JWT sintético assinado apenas para verificar a exigência de escopo; não demonstra emissão de um token real de leitura. Antes de permitir leitura, a confirmação local deve vincular explicitamente a concessão a um cliente registrado e o token real deve possuir o escopo separado de leitura.

A aprovação de uma raiz para arquivos **não confina processos**. Shell, Git e edição permanecem indisponíveis, e não há promessa de sandbox. Não usar Quick Tunnel experimental para expor a futura leitura de dados privados antes de concluir os controles e o teste real com projeto descartável.

## Evidência automatizada

Testes em `internal/workspace/grants_test.go` e `cmd/signalspace/workspace_console_test.go` cobrem concessão inexistente, identidade divergente, confirmação separada, cancelamento, expiração, raízes amplas, revogação, substituição e encerramento. Testes em `internal/workspace/session_test.go` cobrem texto permitido, IDs de sessão, raízes amplas ou symlink, traversal, links intermediários e finais, limites de tamanho, binário, diretório, inexistência, fechamento e substituição de pathname depois da abertura. Isso verifica a camada interna, não uma autorização pelo proprietário nem a execução no ChatGPT Web.

## Associação explícita ao cliente OAuth (fatia interna seguinte)

Nesta instância, depois de concluir OAuth até a **emissão do token**, o operador local pode usar `workspace clients` para consultar os IDs dos clientes com token gerado. Um simples registro dinâmico, uma solicitação de autorização pendente ou a aprovação sem troca de código **não** torna o cliente elegível. O nome é apenas metadado declarado no registro, não atestação do aplicativo.

O comando mudou para `workspace request <client-id> <raiz-absoluta-canônica>`; a confirmação posterior exibe tanto o ID do cliente quanto a raiz exata. `workspace approve <id>` grava somente essa combinação de proprietário, cliente e sessão. Uma leitura interna deve apresentar os **três identificadores corretos**; outro registro OAuth do mesmo proprietário é rejeitado mesmo conhecendo o ID da sessão. Substituição, revogação e encerramento invalidam a associação anterior. Somente o terminal cria, substitui ou revoga concessões. IDs apresentados por argumentos MCP não são identidades verificadas.

Uma autorização vincula-se ao registro do cliente, **não a um chat**. Conversas diferentes só reutilizarão o mesmo acesso se o cliente efetivamente reutilizar esse registro e uma futura ferramenta receber um token válido com escopo de leitura. O SignalSpace não identifica chats nem transfere histórico/conteúdo entre conversas. Uma nova instância de `connect quick` cria identidade e registro distintos e exige nova autorização.

**Na configuração executável atual não há ferramenta de arquivo exposta nem emissão de `signalspace:workspace.read`.** Antes de habilitar leitura, a fronteira MCP deverá chamar `VerifyIdentity` exigindo escopo específico e repassar exclusivamente o `OwnerSubject` e `ClientID` verificados ao gerenciador, aplicando o ID de sessão e os limites de path. Validar em integração com projeto descartável e testes negativos. A lista local de clientes elegíveis não confere escopo de leitura ou atesta que o software é ChatGPT.

## Escopo OAuth de leitura: suporte opt-in, desabilitado no Quick Tunnel

O emissor integrado agora possui uma configuração opcional `ReadScope`, que só pode ser ativada com o valor exato `signalspace:workspace.read`, escopo básico `signalspace:diagnostic` e callback `CanIssueRead(clientID)` de concessão local. A configuração padrão de `connect quick` **não preenche esses campos**; portanto continua anunciando e emitindo somente diagnóstico e não expõe `read_file`.

Quando uma composição futura ativar essa configuração, o cliente deverá pedir a combinação exata `signalspace:diagnostic signalspace:workspace.read`, passar por uma **nova aprovação OAuth no terminal**, e já possuir uma concessão de pasta ativa para o `client_id` do registro. A página de consentimento mostra o ID do registro, escopos exatos e aviso explícito da permissão de leitura; o terminal mostra esses mesmos identificadores. Pedidos de escopo isolado, repetido, reordenado ou não configurado são recusados. O emissor verifica a concessão no início do pedido, ao liberar o código e imediatamente antes de assinar o token. Uma revogação percebida nesses três pontos faz a tentativa falhar fechada, e o código consumido não pode ser reutilizado; revogação concorrente depois da última checagem exige também o controle em cada ferramenta.

**Limite importante:** a revogação depois da emissão não invalida automaticamente o JWT já assinado, que pode continuar válido até expirar. A futura ferramenta MCP DEVE verificar o escopo de leitura em cada chamada via `VerifyIdentity` e consultar a concessão ativa pelo proprietário, cliente e sessão antes de abrir qualquer arquivo. Ter um token com escopo de leitura não fornece acesso na ausência dessa concessão. Esse caminho de requisição ainda não está implementado nem testado com ChatGPT Web; não habilitar a configuração opcional apenas porque o teste do emissor passou.


## Ferramenta MCP de leitura: implementação opt-in testada, não habilitada no produto

A camada `internal/mcp` agora aceita uma porta opcional `OAuthConfig.WorkspaceReader`. Na ausência dela, `tools/list` continua anunciando apenas `connection_diagnostic`, e `read_file` continua indisponível. O executável (`connect quick`, OAuth embutido padrão, modo externo e diagnóstico local) **não injeta essa porta**; portanto, nenhum arquivo do proprietário passou a ser exposto. A configuração opt-in exige `IdentityVerifier` com `VerifyIdentity`, sem fallback para um verificador que só confirme a autenticidade de um token.

Quando um teste injeta a porta e um verificador real, a ferramenta `read_file` anuncia o escopo OAuth `signalspace:workspace.read` e exige exatamente `session_id` e `path` relativo; não aceita raiz absoluta como argumento. Em cada chamada, a camada HTTP verifica primeiro o token de diagnóstico, depois `VerifyIdentity` com escopo **específico de leitura**, assinatura, emissor, audiência e proprietário, e repassa somente `OwnerSubject` e `ClientID` autenticados à porta. O gerenciador verifica novamente a concessão por proprietário, cliente e sessão durante a leitura. Tokens legados sem identidade, tokens apenas de diagnóstico, outros clientes, concessões ausentes ou revogadas e paths hostis não retornam conteúdo. As falhas de filesystem recebem erro genérico, sem vazar caminho local ou presença de arquivo. Resposta de sucesso contém apenas UTF-8 regular sob limite de 32 KiB; não há escrita, Git ou shell.

Essa implementação foi testada via `httptest` usando JWT assinado, JWKS e `workspace.Grants` reais com arquivos **descartáveis**, incluindo negações de escopo, token adulterado, cliente divergente, ID de sessão desconhecido, substituição de concessão, revogação com token ainda válido, `../`, caminho absoluto, symlink, arquivo binário e tamanho excessivo. Isso **não demonstra** que o ChatGPT Web já consegue ler arquivos nem que um túnel público oferece controles suficientes.

Antes da ativação no produto: compor a mesma instância de `Grants` em aprovação local, servidor OAuth com `ReadScope` e servidor MCP com `WorkspaceReader`; revisar o fluxo real de autorização e obter consentimento independente de leitura; decidir como o usuário recupera o `session_id` sem enviar o caminho de raiz ao cliente; executar smoke com projeto descartável e testes remotos de revogação, concorrência e falhas. O operador deve saber que conceder acesso a uma raiz permite ler os textos nela contidos enquanto a sessão permanecer ativa; não existe separação entre chats pelo `client_id`.

**Limites restantes da fronteira de diretórios:** um arquivo pode mudar simultaneamente por outro processo do mesmo usuário; hardlinks e montagens existentes dentro da raiz não são tratados como um sandbox de filesystem. O conteúdo lido é **dado não confiável**, não uma nova instrução para o assistente.

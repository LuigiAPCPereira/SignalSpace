# SignalSpace — contrato inicial de programação local

**Estado:** fatia vertical implementada e validada somente localmente na branch `codex/mvp-vertical-programming`; a fronteira MCP de escrita existe apenas no harness isolado de testes. Isso não é autorização de publicar novas ferramentas MCP, abrir túnel ou conceder OAuth.

## Fronteira de autorização

- A edição exige a concessão existente de `owner`, `clientID` e `sessionID` em `workspace.Grants`; `Grants.Revoke` nega chamadas posteriores.
- `Grants.Grant` permanece somente leitura. A fronteira local `Grants.GrantWithScopes` aceita apenas `signalspace:workspace.read` e `signalspace:workspace.write`; uma concessão read-only não pode editar, e uma concessão write-only não pode ler.
- `Grants.ReplaceText` revalida owner, cliente, sessão ativa e `signalspace:workspace.write` a cada chamada, além das validações de arquivo. A revogação limpa os escopos e nega chamadas posteriores.
- O contrato de autorização remoto específico para escopos de programação ainda está **em andamento**. Nenhum escopo de escrita, execução ou Git foi adicionado ao OAuth/MCP nesta missão; `GrantWithScopes` é uma fronteira local fechada para testes e composição futura.
- O commit `b46cda6` adiciona a porta interna `WorkspaceTextWriter` e a composição `replace_text` somente ao harness de testes do pacote MCP. A configuração que injeta essa porta é deliberadamente não exportada, portanto os entrypoints de produção não conseguem registrá-la por configuração normal.
- Na composição isolada, cada chamada revalida identidade JWT assinada, owner, cliente, `signalspace:workspace.write`, sessão e concessão corrente antes de chamar `Grants.ReplaceText`. O resultado é estruturado e não revela caminhos ou detalhes do filesystem em falhas.
- Os modos públicos continuam sem `signalspace:workspace.write`, sem emissor OAuth de escrita e sem `replace_text` em `tools/list`/`tools/call`. A prova cobre o handler diagnóstico sem capacidade de escrita e os modos de leitura existentes; não é aceite de transporte remoto.
- A UI administrativa funcional serve somente em `localhost:7677`, usa a sessão administrativa existente, cookie HttpOnly e CSRF em memória. Ela não escolhe raízes nem publica ferramentas de programação.

## Edição segura inicial

`Session.ReplaceText` e `Grants.ReplaceText` aceitam apenas caminho relativo canônico, componentes sem symlink, arquivo regular, UTF-8 sem NUL e conteúdo até `MaxTextBytes`. A operação compara o conteúdo esperado exatamente antes de publicar um temporário no mesmo diretório, preserva as permissões e retorna conflito sem sobrescrever silenciosamente. Leitura, edição e revogação são serializadas na instância local.

Essa proteção é uma versão otimista local; outro processo externo pode alterar o arquivo fora do mutex do SignalSpace. Por isso não é apresentada como transação filesystem ou isolamento contra escritores externos.

## Execução controlada inicial

`programming.RunPredefinedTest` executa somente `go test ./...`, sem shell e sem argumentos arbitrários, no diretório da sessão. Há limite configurável de até cinco minutos, saída limitada a 1 MiB, código de saída, distinção entre falha do teste, cancelamento e timeout, e espera pelo encerramento do processo principal. O diretório aprovado **não é sandbox de processo**: o comando mantém os privilégios do usuário e pode acessar recursos permitidos pelo sistema operacional.

## Revisão Git inicial

`CaptureGitSnapshot` observa `git status --porcelain` e `git diff --no-ext-diff --binary`, ambos limitados e sem `add`, commit, push ou limpeza. `CompareGitSnapshots` separa o baseline do estado após a edição. O resultado é somente local e não é uma ferramenta MCP.

## Evidência e próximos limites

O teste vertical descartável percorre leitura → edição → `go test ./...` → snapshot/diff → revogação → negação. `internal/mcp/workspace_write_test.go` acrescenta a composição isolada: escrita autorizada, token somente leitura/sem escopo, cliente e owner divergentes, sessão/workspace divergentes, revogação com JWT ainda válido, token inválido/expirado, traversal, conflito, duplicação e ausência da ferramenta nos modos públicos. Os testes não validam navegador, OAuth emissor de escrita, túnel, MCP remoto, sandbox de processo, escritores externos ou integração com a branch `feat/frontend-oauth-consent`. A promoção dessas capacidades para transporte remoto requer contrato de escopo, autorização por operação, limites e testes adversariais próprios.

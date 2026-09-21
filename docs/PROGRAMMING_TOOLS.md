# SignalSpace — contrato inicial de programação local

**Estado:** fatia vertical implementada e validada somente localmente na branch `codex/mvp-vertical-programming`; não é autorização de publicar novas ferramentas MCP, abrir túnel ou conceder OAuth.

## Fronteira de autorização

- A edição exige a concessão existente de `owner`, `clientID` e `sessionID` em `workspace.Grants`; `Grants.Revoke` nega chamadas posteriores.
- O contrato de autorização específico para escopos de programação ainda está **em andamento**. Nenhum escopo de escrita, execução ou Git foi adicionado ao OAuth/MCP nesta missão.
- A UI administrativa funcional serve somente em `localhost:7677`, usa a sessão administrativa existente, cookie HttpOnly e CSRF em memória. Ela não escolhe raízes nem publica ferramentas de programação.

## Edição segura inicial

`Session.ReplaceText` e `Grants.ReplaceText` aceitam apenas caminho relativo canônico, componentes sem symlink, arquivo regular, UTF-8 sem NUL e conteúdo até `MaxTextBytes`. A operação compara o conteúdo esperado exatamente antes de publicar um temporário no mesmo diretório, preserva as permissões e retorna conflito sem sobrescrever silenciosamente. Leitura, edição e revogação são serializadas na instância local.

Essa proteção é uma versão otimista local; outro processo externo pode alterar o arquivo fora do mutex do SignalSpace. Por isso não é apresentada como transação filesystem ou isolamento contra escritores externos.

## Execução controlada inicial

`programming.RunPredefinedTest` executa somente `go test ./...`, sem shell e sem argumentos arbitrários, no diretório da sessão. Há limite configurável de até cinco minutos, saída limitada a 1 MiB, código de saída, distinção entre falha do teste, cancelamento e timeout, e espera pelo encerramento do processo principal. O diretório aprovado **não é sandbox de processo**: o comando mantém os privilégios do usuário e pode acessar recursos permitidos pelo sistema operacional.

## Revisão Git inicial

`CaptureGitSnapshot` observa `git status --porcelain` e `git diff --no-ext-diff --binary`, ambos limitados e sem `add`, commit, push ou limpeza. `CompareGitSnapshots` separa o baseline do estado após a edição. O resultado é somente local e não é uma ferramenta MCP.

## Evidência e próximos limites

O teste vertical descartável percorre leitura → edição → `go test ./...` → snapshot/diff → revogação → negação. Os testes não validam navegador, OAuth, MCP remoto, sandbox de processo, escritores externos ou integração com a branch `feat/frontend-oauth-consent`. A promoção dessas capacidades para transporte remoto requer contratos de escopo, autorização por operação, limites e testes adversariais próprios.

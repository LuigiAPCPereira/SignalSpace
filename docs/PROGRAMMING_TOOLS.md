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

## Edição segura inicial

`Session.ReplaceText` e `Grants.ReplaceText` aceitam apenas caminho relativo canônico, componentes sem symlink, arquivo regular, UTF-8 sem NUL e conteúdo até `MaxTextBytes`. A operação compara o conteúdo esperado exatamente antes de publicar um temporário no mesmo diretório, preserva as permissões e retorna conflito sem sobrescrever silenciosamente. Leitura, edição e revogação são serializadas na instância local.

Essa proteção é uma versão otimista local; outro processo externo pode alterar o arquivo fora do mutex do SignalSpace. Por isso não é apresentada como transação filesystem ou isolamento contra escritores externos.

## Execução controlada inicial

`programming.RunPredefinedTest` executa somente `go test ./...`, sem shell e sem argumentos arbitrários, no diretório da sessão. Há limite configurável de até cinco minutos, saída limitada a 1 MiB, código de saída, distinção entre falha do teste, cancelamento e timeout, e espera pelo encerramento do processo principal. O diretório aprovado **não é sandbox de processo**: o comando mantém os privilégios do usuário e pode acessar recursos permitidos pelo sistema operacional.

## Revisão Git inicial

`CaptureGitSnapshot` observa `git status --porcelain` e `git diff --no-ext-diff --binary`, ambos limitados e sem `add`, commit, push ou limpeza. `CompareGitSnapshots` separa o baseline do estado após a edição. O resultado é somente local e não é uma ferramenta MCP.

## Evidência e próximos limites

O teste vertical descartável percorre leitura → edição → `go test ./...` → snapshot/diff → revogação → negação. `internal/mcp/workspace_write_test.go` cobre a fronteira MCP com tokens montados no harness; `internal/mcp/oauth_write_integration_test.go` acrescenta o fluxo real do próprio emissor OAuth, PKCE, aprovação local, troca por token assinado, `tools/list`, `replace_text`, diff observável no arquivo, revogação e código de uso único. `internal/auth/write_scope_test.go` cobre opt-in explícito, texto de consentimento, combinações canônicas e revogação antes de conclusão/troca. Os testes continuam sem navegador real, túnel, grant ao ChatGPT, sandbox de processo, escritores externos ou integração com `feat/frontend-oauth-consent`; a promoção para transporte remoto requer contrato e gate próprios.

# SignalSpace — checkpoint de continuidade

**Natureza:** checkpoint derivado do estado remoto observado em 20/09/2026. Não substitui código, contratos, CI, árvore local ou autorização de merge. Revalidar PR/HEAD antes de escrever; não colocar o próprio SHA do commit documental em um ciclo de atualizações.

## Escopo e fontes

Repositório `LuigiAPCPereira/SignalSpace`; backend na branch `feat/m1-local-mcp-diagnostic`, [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), aberto e draft na última inspeção. Contrato de segurança: [`LOCAL_ADMIN_AUTHORIZATION.md`](LOCAL_ADMIN_AUTHORIZATION.md); regras de trabalho: [`../DOCUMENTATION_AND_CONTINUITY.md`](../DOCUMENTATION_AND_CONTINUITY.md) e [`../AGENTS.md`](../AGENTS.md). A branch `feat/frontend-oauth-consent` e seus protótipos não devem ser alterados nesta etapa. Não fazer merge, escrita, Git, shell ou aprovação por chamada MCP sem autorização própria.

## Implementado e validado

- Smoke real de `read_file`/`list_directory` e negativa após revogação: relato anterior do proprietário, registrado no PR com limitações de correlação; não repetido nesta sessão.
- Componentes já presentes antes desta continuação: listeners 7676/7677 isolados como componentes; pareamento, sessão, CSRF e handlers administrativos de consulta/decisão isolados; status público vinculado ao cookie OAuth. **O listener administrativo não está integrado ao `connect quick` nem o frontend está conectado.**
- Continuação a partir de `97eba3878b2966ed6eb0df1713e5af9bc98649f2`: [`7dbd5d2`](https://github.com/LuigiAPCPereira/SignalSpace/commit/7dbd5d21c3cb358e9f74d2200e2834371dfff689) faz o stdin chamar `DecideTerminal`, que reutiliza `decideLocked` via `DecideVersioned` e revalida a concessão antes da aprovação. Adicionados testes em `internal/auth/terminal_decision_test.go` e `cmd/signalspace/terminal_decision_test.go` para concorrência terminal/painel, expiração, revogação da leitura, negativa posterior e status público.
- [CI #106](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521202757) no commit `7dbd5d2`: formato, `go test ./...`, detector de corridas, `go vet` e build concluíram com sucesso. Essa CI **não prova** integração do painel/Quick Tunnel nem teste real de navegador.

## Pendências objetivas

A operação `Approve` legada ainda existe no servidor OAuth e é usada por alguns testes; o **caminho de terminal em produção já migrou** para `DecideTerminal`, mas a remoção/delegação do método legado deve acompanhar a evolução do lifecycle. Ainda faltam `decided_at` real, estados `COMPLETED`/`EXPIRED` e tombstones com retenção/capacidade limitada; reconciliar resposta HTTP perdida e expiração sob concorrência; revisar derivação da frase-senha; vincular os listeners no modo painel explicitamente opt-in e falhar fechado se 7677 não puder ser reservado; executar testes de composição, de túnel e smoke de navegador na etapa apropriada.

**Próxima ação verificável:** evoluir `internal/auth/server.go`, `decision.go`, `request_snapshot.go` e seus testes como uma única fatia de lifecycle, preservando compatibilidade OAuth e a emissão de código apenas em `/authorize/complete`. Antes de iniciar, verificar o HEAD real do PR, checkout local caso disponível e contrato; após a mudança, testar corrida entre terminal/painel/conclusão, limite de registros terminais e revogação de workspace. PR permanece draft até validação integral; nenhum merge autorizado.

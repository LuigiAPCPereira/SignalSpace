# Quick com API administrativa local — modo experimental

**Estado em 20/09/2026:** composição implementada na branch `feat/m1-local-mcp-diagnostic`, PR #1 em draft; testes de integração com processo `cloudflared` **simulado** e sockets HTTP reais passaram na [CI #169](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536833997), commit [`f7efbb3`](https://github.com/LuigiAPCPereira/SignalSpace/commit/f7efbb3f3fb5e04ae542140c15ecd79526e595f9). Não houve túnel público com painel real nem execução no navegador. Este documento complementa o fluxo terminal histórico de [QUICK_TUNNEL.md](QUICK_TUNNEL.md), sem substituí-lo. A autoridade do comportamento é o [contrato administrativo](LOCAL_ADMIN_AUTHORIZATION.md) mais o código/CI na revisão efetiva.

## Seleção explícita

```bash
# Os comandos existentes mantêm aprovação exclusivamente pelo terminal.
go run ./cmd/signalspace connect quick
go run ./cmd/signalspace connect quick read

# Ativa apenas a API administrativa local, ainda SEM interface HTML integrada.
go run ./cmd/signalspace connect quick panel
go run ./cmd/signalspace connect quick read panel
```

O modo `panel` exige respectivamente `PUBLICAR PAINEL` ou `PUBLICAR LEITURA PAINEL` no terminal; a confirmação antiga `PUBLICAR` não o ativa. O operador deve entender que o **túnel é público**, mas a administração fica em HTTP loopback e não deve ser encaminhada externamente. Para `read`, continua obrigatória a concessão independente de workspace feita pelo terminal e um novo consentimento OAuth. Nenhuma ação de escrita, Git, shell ou aprovação por chamada MCP é acrescentada.

É necessário ter `cloudflared` no PATH, portas **127.0.0.1:7676 e 127.0.0.1:7677** disponíveis e as variáveis de ambiente do Quick isolado ausentes, conforme QUICK_TUNNEL.md. O protótipo não instala `cloudflared` nem modifica configurações do host.

## Fronteira de inicialização e encerramento

1. Após a confirmação explícita, o processo reserva ambas as portas IPv4 de loopback **antes de chamar `tunnel.Start`**; se uma reserva falhar, fecha a outra e não inicia o túnel. Não existe fallback para terminal-only.
2. Cria um `Gate` administrativo efêmero, com código de pareamento aleatório e sessões em memória, antes do túnel. O código só é exibido no terminal depois da validação de inicialização; nunca na resposta pública.
3. O `cloudflared` é chamado com origem **constante** `http://127.0.0.1:7676`. A URL pública gerada é necessária para construir OAuth; por isso os handlers OAuth e admin são compostos após `tunnel.Start`. O servidor administrativo deve responder ao probe local de `/api/admin/v1/session` antes de iniciar o servidor MCP público; falha de readiness aborta a sessão.
4. Só depois da verificação HTTPS externa, o processo informa a URL pública para ChatGPT e, separadamente, o endereço local `http://localhost:7677/api/admin/v1/session` e o código de pareamento no terminal. O servidor público **não** registra as rotas administrativas; o administrativo **não** registra MCP/OAuth público.
5. Um erro de `Serve` de qualquer servidor ou término do túnel encerra a sessão em execução. Ctrl+C solicita fechamento dos dois servidores, espera a confirmação de ambos, encerra o filho `cloudflared`, libera portas e descarta a identidade OAuth temporária e o `Gate` em memória. O probe HTTP inicial cria um bootstrap sem privilégios; não autentica a administração.

A porta `7677` aceita somente socket `127.0.0.1`, Host canônico `localhost:7677`, sessão administrativa e CSRF nas mutações; requisições de outra origem e acesso anônimo aos pedidos são negados. A página de consentimento pública continua dependente de `approve <id>` / `deny <id>` ou de uma decisão feita na API local. Somente `/authorize/complete` emite código; aprovação administrativa sozinha não conecta MCP.

## Limites e aceites ainda pendentes

**A API é funcional, não existe painel visual servido nesta branch.** O operador só pode usar o contrato HTTP; não digite o código de pareamento em sites externos. A branch de interface `feat/frontend-oauth-consent` permanece separada e não foi mesclada. Para a interface funcionar, ainda é necessário implementar/adaptar JS e CSP correspondentes, testar `localhost`/IPv4 no navegador, estados da UI e acessibilidade, e obter autorização específica antes da integração das branches.

A CI prova: argumento de origem do processo simulado fixo em 7676; dois servidores reais em portas fixas locais; rotas isoladas; pareamento e GET de pedidos autenticado; liberação de ambas as portas após cancelamento; e falha de bind da 7677 antes do túnel sem divulgação do segredo. **Não prova** cloudflared real, navegador real, indisponibilidade diante de proxy externo deliberadamente reescrito, auditoria independente, deploy ou produção. O teste de resposta perdida e a revogação do grant permanecem evidência de harness HTTP local [SS-BE-007](../TASKLIST.md), não de rede Cloudflare. Um novo smoke operacional deve ser feito somente em ambiente descartável com consentimento do proprietário.

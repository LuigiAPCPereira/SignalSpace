# Quick com API administrativa local — modo experimental

**Estado em 20/09/2026:** composição implementada na branch `feat/m1-local-mcp-diagnostic`, PR #1 em draft. Testes usam `cloudflared` **simulado** e sockets HTTP reais, [CI #169](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536833997); falha administrativa depois da prontidão validada na [CI #176](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35538258629). Ambas PASS integral. **Nenhum teste de túnel público com painel real ou navegador foi realizado.** Este documento complementa o [fluxo terminal](QUICK_TUNNEL.md). Contrato vigente: [autorização administrativa](LOCAL_ADMIN_AUTHORIZATION.md); código e CI na revisão efetiva prevalecem sobre descrições históricas.

## Seleção explícita

```bash
# Comandos anteriores: aprovação exclusivamente pelo terminal.
go run ./cmd/signalspace connect quick
go run ./cmd/signalspace connect quick read

# Liga apenas a API administrativa local, ainda SEM interface HTML integrada.
go run ./cmd/signalspace connect quick panel
go run ./cmd/signalspace connect quick read panel
```

`panel` exige respectivamente `PUBLICAR PAINEL` ou `PUBLICAR LEITURA PAINEL`; `PUBLICAR` não ativa administração. O **túnel é público**, enquanto a API administrativa usa HTTP loopback e nunca deve ser encaminhada externamente. Em `read`, a concessão de workspace pelo terminal continua independente e um novo consentimento OAuth é obrigatório. Nenhuma escrita, Git, shell ou aprovação individual de chamadas MCP foi acrescentada.

Pré-requisitos: `cloudflared` no PATH, portas **127.0.0.1:7676 e 127.0.0.1:7677** disponíveis e variáveis de ambiente Quick isoladas ausentes, conforme [QUICK_TUNNEL.md](QUICK_TUNNEL.md). Este código não instala `cloudflared`, eleva privilégios nem modifica configurações do host.

## Inicialização e encerramento

1. Após confirmação explícita, reservar ambas as portas IPv4 de loopback **antes de `tunnel.Start`**. Falha em qualquer reserva encerra tudo e libera a reserva parcial, sem fallback terminal-only.
2. Criar `admin.Gate` efêmero e código de pareamento aleatório antes do túnel; exibir segredo somente no terminal e após validar a inicialização, nunca em resposta pública.
3. Iniciar `cloudflared tunnel --url http://127.0.0.1:7676` com destino **constante**. A URL é necessária para compor OAuth, portanto os handlers são criados depois de `tunnel.Start`. Subir 7677 e exigir probe local de `/api/admin/v1/session` antes de iniciar o servidor público 7676; o probe cria apenas bootstrap não privilegiado.
4. Só após o diagnóstico HTTPS externo anunciar a URL pública, e separadamente `http://localhost:7677/api/admin/v1/session` e o segredo no terminal. Público não registra rotas administrativas; admin não registra MCP/OAuth público.
5. Término do túnel, falha de `Serve` em qualquer servidor ou Ctrl+C encerra sessão, fecha ambos servidores, espera goroutines, encerra `cloudflared`, libera portas e descarta identidade OAuth temporária/Gate. Uma falha administrativa não deve deixar o MCP exposto sozinho.

Admin exige socket `127.0.0.1`, Host canônico `localhost:7677`, sessão própria e CSRF nas mutações; nenhuma autenticação por IP local. A página OAuth pública continua dependendo de decisão pelo terminal ou API local. Apenas `/authorize/complete` emite código; aprovar na administração não cria token.

## Evidências e limites

- [CI #169](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536833997): stub de `cloudflared` verifica origem `7676`; portas reais, isolamento HTTP, pareamento e fila autenticada, falha de bind 7677 antes do túnel e liberação de ambas portas após cancelamento.
- [CI #176](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35538258629): `quick_panel_failure_test.go` interrompe o `http.Server` administrativo real depois da prontidão em duas situações: durante a verificação pública, antes do anúncio, e depois de anunciar. Em ambas exige erro em vez de fallback e rebind das portas 7676/7677; a primeira também verifica ausência de URL e segredo no output. O ponto de injeção constrói `admin.NewServer` na produção e é inacessível pela CLI. O processo `cloudflared` permanece simulado no teste.
- [CI #146](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35531123085) e [CI #152](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35532258138): OAuth/decisão, resposta perdida e revogação de grant validados apenas no harness HTTP local, não via rede Cloudflare.

**Faltam os aceites operacionais SS-BE-002/006/007/008:** `cloudflared` real em ambiente descartável autorizado do proprietário, interface HTML/JS funcional em branch própria, CSP de script segura, navegador real, smoke da decisão e shutdown. O conector GitHub não executa o túnel na máquina do proprietário nem enxerga o worktree local. Não confundir CI de Go com execução da política CSP no browser.

**Esta branch serve API, não painel visual.** `feat/frontend-oauth-consent` continua separada; integração de HTML exige confirmação do HEAD, propriedade dos arquivos e autorização específica. Não digitar segredo de pareamento em site externo. HTTP loopback não equivale a HTTPS, não protege contra proxy externo deliberadamente configurado nem garante segurança de produção; Quick Tunnel público/temporário não é deployment. Sem auditoria independente, merge ou deploy.

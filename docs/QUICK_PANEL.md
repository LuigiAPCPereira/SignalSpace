# Quick com API administrativa local — modo experimental

**Estado em 20/09/2026:** composição implementada na branch `feat/m1-local-mcp-diagnostic`, PR #1 em draft. Testes usam `cloudflared` **simulado** e sockets HTTP reais, [CI #169](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35536833997); falha administrativa depois da prontidão validada na [CI #176](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35538258629). Ambas PASS integral. **Nenhum teste de túnel público com painel real ou navegador foi realizado.** Este documento complementa o [fluxo terminal](QUICK_TUNNEL.md). Contrato vigente: [autorização administrativa](LOCAL_ADMIN_AUTHORIZATION.md); código e CI na revisão efetiva prevalecem sobre descrições históricas.

## Seleção explícita

```bash
# Comandos anteriores: aprovação exclusivamente pelo terminal.
go run ./cmd/signalspace connect quick
go run ./cmd/signalspace connect quick read

# Liga a administração HTML/API local em 7677; a página é servida somente no loopback.
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

**Faltam os aceites operacionais SS-BE-002/006/007/008:** `cloudflared` real em ambiente descartável autorizado do proprietário, navegador real, smoke da decisão e shutdown. A UI HTML/JS local agora existe na branch `codex/mvp-vertical-programming`, mas foi validada somente por HTTP local, sintaxe e testes de roteamento; não houve integração da branch frontend nem aceite visual. O conector GitHub não executa o túnel na máquina do proprietário nem enxerga o worktree local. Não confundir CI de Go com execução da política CSP no browser.

**A administração local serve painel e API somente no loopback.** `feat/frontend-oauth-consent` continua separada; não houve integração de seus protótipos. Não digitar segredo de pareamento em site externo. HTTP loopback não equivale a HTTPS, não protege contra proxy externo deliberadamente configurado nem garante segurança de produção; Quick Tunnel público/temporário não é deployment. Sem auditoria independente, merge ou deploy.

## Roteiro de aceite operacional pendente — não executar sem autorização específica

Este roteiro foi preparado para a próxima validação de `SS-BE-007`. O smoke operacional **não foi executado nesta preparação**. Ele não transforma a existência do roteiro, o PR draft ou os testes locais em autorização para publicar o serviço.

### Sequência segura

1. **Preflight sem exposição externa:** revalidar branch/HEAD/worktree, os dois patches não rastreados, ausência das variáveis SignalSpace persistentes, disponibilidade de `127.0.0.1:7676` e `127.0.0.1:7677`, versão do `cloudflared` já instalado e ausência de listeners/processos residuais. Não registrar pairing code, frase-senha, cookie, CSRF, token ou conteúdo bruto de logs.
2. **Modo explícito:** confirmar no terminal o modo `diagnostic panel`; não usar `read`, escrita, execução ou Git. Antes de qualquer túnel, verificar que o painel está ligado a `127.0.0.1:7677` e que o destino público, se iniciado, é exatamente `http://127.0.0.1:7676`.
3. **Publicação autorizada:** somente após nova autorização explícita do proprietário, digitar `PUBLICAR PAINEL` no terminal e iniciar o Quick Tunnel. Registrar apenas a URL/estado necessários para a verificação; não conceder OAuth ao ChatGPT Web por causa deste smoke.
4. **Isolamento pela URL pública:** consultar a URL externa somente para provar que `/admin`, `/api/admin/v1/session`, `/api/admin/v1/requests`, `/api/admin/v1/pair`, `/api/admin/v1/unlock` e `/api/admin/v1/requests/<id>/decision`, incluindo variantes de caminho codificado, barras duplicadas e método alternativo, retornam `404`, sem redirecionamento e sem `Set-Cookie` administrativo. A resposta pública não pode alcançar 7677.
5. **Administração local:** em `http://localhost:7677/`, confirmar `Host: localhost:7677`, rejeição de `Origin` cruzada e que cabeçalhos `Forwarded`/`X-Forwarded-*` não concedem autoridade. Parear apenas uma credencial descartável e verificar sessão, fila e decisão pelo painel local. Qualquer código de pareamento, passphrase, cookie, CSRF ou token fica fora do relatório.
6. **Diagnóstico opcional:** só executar uma chamada MCP de diagnóstico se houver autorização específica para essa etapa. Não habilitar workspace, leitura, escrita, shell, execução ou Git; a ausência dessa permissão deixa essa etapa como `NÃO EXECUTADA`, sem bloquear a prova de isolamento de portas.
7. **Shutdown verificável:** encerrar pelo fluxo previsto, esperar o servidor administrativo, o servidor público e o `cloudflared`, confirmar que as duas portas foram liberadas, que a identidade/StateDir temporários foram removidos e que não restam processos ou listeners do smoke.

### Dependências de autorização

- Podem ser executados sem exposição externa: inspeção de ref/worktree, preflight de portas/processos, verificação documental, validação de Host/Origin em listener local descartável e organização de evidências redigidas.
- Exigem nova autorização explícita imediatamente antes da ação: iniciar o túnel, digitar `PUBLICAR PAINEL`, expor qualquer URL `trycloudflare.com`, parear/decidir um pedido OAuth descartável em ambiente operacional e executar uma chamada MCP, mesmo que seja somente diagnóstico.
- A autorização permanente para enviar relatórios ao ChatGPT Web não inclui publicação de endpoint, concessão OAuth, decisão de pedido, acesso a workspace ou execução de ferramentas.

### Critérios de parada

Abortar e manter o aceite como `BLOQUEADO` diante de: 7677 publicada externamente; origem do túnel diferente de `127.0.0.1:7676`; falha de reserva de qualquer porta; resposta administrativa pela URL pública; Host/Origin ou proxy forjado concedendo acesso; segredo/cookie/token aparecendo em logs; falha de shutdown, remoção do StateDir ou liberação das portas. Não usar proxy, host rewrite ou aumento de janela DNS como configuração suportada.

### Resultado e evidência esperados

O relatório operacional deve separar `IMPLEMENTADO`, `VALIDADO`, `NÃO VALIDADO` e `DESCONHECIDO`, registrar status/códigos/contagens sem segredos e manter `SS-BE-007` **PARCIAL**, `SS-MVP-002` **PARCIAL**, `SS-MVP-002-PROMOTION-GATE-001` **PENDENTE** e CI **DESCONHECIDA** até que a evidência externa autorizada exista. O roteiro sozinho não é aceite operacional.

## Papel futuro do painel na autorização local v2

Como direção aceita em [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md), o painel local é a autoridade owner-side para Connections, Workspaces, Pending approvals e Audit quando a policy retornar `REQUIRE_APPROVAL`. As opções conceituais são `Allow once`, `Allow session`, `Allow workspace` e `Deny`; a escolha não executa a chamada original. OAuth válido, permissões do host ChatGPT e notificações não substituem essa autoridade.

Esta seção não implementa fila, notification, persistência ou nova rota. O painel v1, o terminal e os limites de `127.0.0.1:7677` continuam regidos pelo contrato vigente até uma tarefa de implementação posterior.

## Estado implementado — approvals e policies locais

`SS-MVP-002-LOCAL-APPROVAL-POLICIES-UI-V2-001` adiciona ao painel existente uma seção separada para approvals de programação e outra para policies de managed workspace. O backend é a autoridade: a UI renderiza `allowed_decisions`, não inventa `ALLOW_WORKSPACE`, usa texto seguro e, após perda de resposta, consulta o estado antes de permitir nova decisão. O painel não faz polling agressivo, não executa a operação aprovada e não publica a porta administrativa.

Estado: **IMPLEMENTADO / VALIDADO LOCALMENTE**. A composição Quick cria o store dentro do estado privado descartável da instância; persistência entre reinícios exige um state directory durável em missão própria. MCP público, túnel, navegador e ChatGPT Web permanecem fora desta validação.

## Estado da ponte Programming v2 — 26/09/2026

Quando `connect quick programming` é iniciado com `panel`, `ProgrammingAuthorizer`, `policy.Engine` e `approval.Manager` são compostos na mesma instância privada do Quick. O painel lista e decide approvals por `/api/admin/v1/capability-approvals`; ele não executa a operação e não expõe permit. Sem painel, qualquer ferramenta Programming que precise da ponte falha fechado como `LOCAL_APPROVAL_UNAVAILABLE`.

O teste HTTP local confirmou o ciclo pendente → decisão → retry idêntico → consumo único. A porta 7677 segue loopback-only e nunca é publicada pelo túnel. Estado: **IMPLEMENTADO / VALIDADO LOCALMENTE / PUBLICAÇÃO REMOTA PENDENTE**; túnel, navegador, ChatGPT Web, workspace real e CI não foram exercitados.

# SignalSpace — checkpoint de continuidade

**Natureza:** snapshot derivado de TASKLIST, contratos, Git e CI, não fonte única de verdade, autorização nem lock. **Consulta-base:** 20/09/2026, branch de backend `feat/m1-local-mcp-diagnostic`, [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), HEAD de código anterior à adoção v2 `08ffb520408b6e14cb959b3c632d689f27ed89c3`. Revalidar HEAD do PR antes de editar; os commits documentais posteriores mudam esta revisão sem implicar novo código. Não inserir o SHA do commit deste próprio arquivo num ciclo infinito.

## Fontes e escopo

- **Protocolo canônico da ref:** [`../DOCUMENTATION_AND_CONTINUITY.md`](../DOCUMENTATION_AND_CONTINUITY.md), adaptação SignalSpace 2.0, com entrada [`../AGENTS.md`](../AGENTS.md). A cópia anexada ao ChatGPT Project é especificação v2.0 geral, não cópia automaticamente sincronizada do projeto; acesso de Codex/agendamentos não foi testado.
- **Produto/requisitos:** [`PRODUCT.md`](PRODUCT.md), [`MVP.md`](MVP.md), [`LOCAL_ADMIN_AUTHORIZATION.md`](LOCAL_ADMIN_AUTHORIZATION.md), [`WORKSPACE_SECURITY.md`](WORKSPACE_SECURITY.md). A indicação inicial de 'nenhum requisito implementado' no MVP é histórica e não substitui o estado atual observado.
- **Inventário canônico:** [`../TASKLIST.md`](../TASKLIST.md), IDs SS-BE-001 a SS-BE-008; **tarefa atual: SS-BE-004**. Marcos em [`ROADMAP.md`](ROADMAP.md), contexto/decisões em [`SESSION_LOG.md`](SESSION_LOG.md), relatório da adoção em [`ADOPTION_REPORT.md`](ADOPTION_REPORT.md) após sua publicação. O relatório não redefine requisitos nem tarefas.
- **Escopo vigente:** backend de OAuth local do modo Quick, sem integrar protótipos HTML, sem alterar `feat/frontend-oauth-consent`, sem ampliar escrita/Git/shell, sem aprovação individual de chamada MCP, sem merge sem autorização. O PR segue draft e não integrado na consulta-base.

## Implementado — evidência e limite

- **SS-BE-001:** smoke real relatado pelo proprietário: `read_file`/`list_directory` em pasta descartável, negativa após revoke, conforme descrição do PR. Não repetido nesta adoção; ausência de log por chamada de listagem e estado exato do JWT permanece desconhecida.
- **SS-BE-002/003/005/006:** componentes isolados de listeners 7676/7677, Gate de pareamento/sessão/CSRF e handlers admin de consulta/decisão; `/authorize/status` ligado somente ao roteador público, restrito ao pedido próprio. **O listener administrativo não está ligado ao `connect quick`, nem o frontend à API.** Não atribuir ao runtime cobertura por mera existência do handler.
- **SS-BE-004:** [`7dbd5d2`](https://github.com/LuigiAPCPereira/SignalSpace/commit/7dbd5d21c3cb358e9f74d2200e2834371dfff689) trocou o stdin para `DecideTerminal`, reutilizando a decisão versionada/revalidação do grant; testes de concorrência, expiração e revogação adicionados. `Approve` legado ainda existe em testes; `decided_at`, `COMPLETED`/`EXPIRED` persistidos e tombstones/limites de retenção continuam ausentes.

## Validação e integração

- [CI #107](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521299236) referente ao HEAD de código `08ffb52` consultada em 20/09/2026: formato, `go test ./...`, corrida, `go vet` e build **PASS**. Não valida novas mudanças documentais nem integração Quick/7677, revisão criptográfica ou browser. Revalidar qualquer CI posterior no novo HEAD.
- **Integração:** PR #1 aberto, draft, não mesclado na consulta-base. Deploy não solicitado/não verificado. Árvore de trabalho e stashes locais do proprietário **não inspecionados** pelo conector remoto.
- **Adoção documental v2:** atualizar/verificar separadamente matriz das nove funções e arquivos na ref; resultado auditável em [`ADOPTION_REPORT.md`](ADOPTION_REPORT.md). Não deduzir conclusão documental de CI de Go.

## Bloqueios e próxima ação por ID

**SS-BE-004 — critério:** decisão única compartilhada, `decided_at` real, transições `PENDING → APPROVED/DENIED/EXPIRED`, `APPROVED → COMPLETED/EXPIRED`, retenção limitada de até 64 terminais por até 10 minutos, conflito de versão, reconciliação GET após perda de resposta, revalidação do grant e emissão de código **somente** em `/authorize/complete`. Revisar/deligar `Approve` legado e testes de corrida terminal/painel/conclusão/expiração/revoke. Ação executável na próxima retomada **autorizada de desenvolvimento**: revalidar PR HEAD e código `internal/auth/{server,decision,request_snapshot}.go`, implementar e executar testes negativos/race na ref exata, registrar resultado em TASKLIST e checkpoint.

Depois, fechar SS-BE-002/003/005/006/007, especialmente revisão da derivação da frase-senha e erros HTTP, antes de SS-BE-008: habilitar 7677 no Quick somente opt-in, reservar ambos antes do túnel, falhar fechado e executar smoke real. Contrato de frontend continua etapa separada sem autorização de integração nesta adoção.

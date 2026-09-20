# SignalSpace — checkpoint de continuidade

**Natureza:** fotografia derivada de execução, não substitui requisitos, código, estado Git nem aprova merge. **Consulta registrada:** 20/09/2026; atualizar após cada marco relevante e revalidar HEAD/CI na sessão seguinte.

## Projeto e escopo autorizado

Repositório: `LuigiAPCPereira/SignalSpace`. Produto e limites em [`PRODUCT.md`](PRODUCT.md) e [`MVP.md`](MVP.md). Trabalho ativo da frente backend: autorização OAuth pelo painel local, segundo [`LOCAL_ADMIN_AUTHORIZATION.md`](LOCAL_ADMIN_AUTHORIZATION.md), sem integrar os protótipos HTML. O modo padrão Quick expõe diagnóstico; leitura é opt-in com concessão independente de workspace. Não ampliar escrita, Git, shell ou aprovação por chamada MCP.

## Referências e estado observado

- Branch backend: `feat/m1-local-mcp-diagnostic`; [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), **aberto, draft e não mesclado** na consulta de 20/09/2026. HEAD de código inspecionado antes da adoção: `f2e7e541e69700bd7099cd3b450da50323728722`. A adoção do protocolo adiciona commits documentais posteriores: consultar o HEAD do PR em vez de presumir que esse SHA continua atual.
- Branch da outra frente: `feat/frontend-oauth-consent`. **Não modificar** nem integrar seu HTML/CSS/JS nesta etapa.
- Fonte de continuidade para esta branch: `../DOCUMENTATION_AND_CONTINUITY.md` + `../AGENTS.md`. A cópia no ChatGPT Project pode divergir; não presumir sincronização nem acesso de tarefas agendadas.
- A árvore de trabalho local do proprietário **não foi verificada**: o acesso usado foi o conector remoto do GitHub. Não apagar ou sobrescrever alterações e stashes locais.

## Implementação / evidências

- **Documentado mas não revalidado nesta adoção:** smoke real informado pelo proprietário em 20/09/2026: `read_file` autorizado e negado após revogação; `list_directory` raiz/subdiretório e negativa após `workspace revoke`. Evidência e limites foram consolidados na descrição do PR; sem log por chamada individual de listagem.
- **Confirmado pelo código remoto e PR consultados nesta adoção:** separação de componentes de listeners/roteadores 7676 e 7677; Gate administrativo isolado com pareamento, desbloqueio, sessão, cookie, CSRF e lock; `GET /authorize/status` registrado apenas no roteador público; snapshots e handlers administrativos de consulta/decisão adicionados, mas ainda não integrados ao `connect quick`.
- **Validado anteriormente, não reexecutado nesta adoção:** [CI #98](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35518520439) concluída com sucesso para formato, testes, corrida, `go vet` e build nos componentes então presentes. Esse resultado não valida integração de painel, túnel ou navegador.
- **Integração:** PR em draft; nenhum merge, frontend integrado ou deploy confirmado.

## Bloqueios e próxima ação executável

Completar em `internal/auth` uma máquina de estados compartilhada terminal/painel, com `decided_at` real, transições `COMPLETED`/`EXPIRED`, retenção/tombstones limitados e decisões atômicas. Depois executar testes negativos de concorrência, expiração, workspace revogado e perda de resposta; somente então ligar as APIs ao listener administrativo no ciclo de vida Quick de forma opt-in, falhando fechado se 7677 não puder ser reservado. Revisar derivação da frase-senha antes de expor a superfície administrativa. Validar CI no HEAD exato e, posteriormente, smoke real do painel e integração com o frontend em etapa separada.

**Critério de segurança:** aprovação pública não existe; OAuth não concede pasta implicitamente; token e concessão continuam distintos. Não declarar a integração pronta enquanto os bloqueios acima persistirem. Não fazer merge sem autorização expressa do proprietário.

# SignalSpace — histórico recuperável da frente de desenvolvimento

Registro **seletivo**, não transcrição de conversas. Fonte para detalhes de commits e evolução: [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), Git e CI; decisões de comportamento: [produto](PRODUCT.md), [workspace](WORKSPACE_SECURITY.md), [contrato admin](LOCAL_ADMIN_AUTHORIZATION.md). Tarefas: [`../TASKLIST.md`](../TASKLIST.md). Não inventar eventos ou autoria não observados.

## 19–20/09/2026 — diagnóstico e leitura experimental (SS-BE-001)

- O proprietário relatou conexão OAuth e correlação de `diagnosticID` de `connection_diagnostic` com log do servidor após `tools/list`; ver [QUICK_TUNNEL.md](QUICK_TUNNEL.md) e descrição do PR. Não houve atestação criptográfica do aplicativo.
- Em 20/09, relatou `read_file` autorizado retornando `Teste de leitura SignalSpace` e negação após revoke; `list_directory` raiz com `nested`/`readme.txt`, subdiretório `nested` com `inside.txt` e negação após revogação. O PR registra restrição da prova: não há log de correlação individual da ferramenta, nem medição precisa da expiração do JWT. Nenhum teste real novo foi feito nesta adoção.
- Escopo efetivo limitado a diagnóstico e leitura opt-in; nenhuma aprovação por ferramenta, escrita, Git ou shell.

## 20/09/2026 — contrato de autorização e preparação de backend (SS-BE-002 a SS-BE-008)

- [LOCAL_ADMIN_AUTHORIZATION.md](LOCAL_ADMIN_AUTHORIZATION.md) fixa servidores distintos 7676/7677, pareamento com sessão imediata, bootstrap/CSRF, grant obrigatório antes do OAuth read, estados versionados, expiração, retenção e nenhuma aprovação pública. Revisão de interface separada em [FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md](https://github.com/LuigiAPCPereira/SignalSpace/blob/feat/frontend-oauth-consent/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md) confirma UX, **não** segurança/integração.
- Implementados componentes isolados admin de transporte, sessão e HTTP; consulta pública de status no roteador 7676; snapshots e consulta/decisão admin ainda desconectados do Quick. [CI #98](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35518520439) passou os gates para componentes daquela revisão.
- No commit [`7dbd5d2`](https://github.com/LuigiAPCPereira/SignalSpace/commit/7dbd5d21c3cb358e9f74d2200e2834371dfff689), stdin passou a usar `DecideTerminal` e a compartilhar decisão/revalidação de workspace com API admin; acrescentados testes de revogação e concorrência. [CI #106](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521202757) passou. Permanecem método legado `Approve`, ausência de `decided_at`/tombstones/estado `COMPLETED`, conexão de 7677 ao Quick e smoke browser.
- Checkpoint anterior em [`08ffb52`](https://github.com/LuigiAPCPereira/SignalSpace/commit/08ffb520408b6e14cb959b3c632d689f27ed89c3). [CI #107](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521299236) concluída com sucesso: formato, `go test ./...`, race, vet e build. O PR estava aberto, draft, sem merge, nessa consulta.

## 20/09/2026 — revisão da adoção documental v2

- O repositório na revisão `08ffb52` tinha `DOCUMENTATION_AND_CONTINUITY.md` adaptação **1.0**, `AGENTS.md` e `docs/PROJECT_STATE.md`; não havia `TASKLIST.md`, roadmap nem registro de história suficiente para provar as nove funções do Adoption Gate v2.
- O arquivo disponibilizado ao ChatGPT Project nesta execução informa `Documentation & Continuity Protocol` **2.0**; a cópia do repositório era 1.0. Atualizar a fonte **no repositório** não sincroniza o anexo do Project e não configura acesso do Codex ou agendamentos por si só.
- Adoção foi solicitada em conversa no contexto SignalSpace; alterações desta etapa se restringem a documentos da branch backend. Próxima tarefa de desenvolvimento, **se retomada mediante autorização apropriada:** SS-BE-004; não há aprovação de merge ou de integração frontend.

## Limites do registro

Fontes consultadas por conector GitHub mostram arquivos versionados e metadados remotos, **não a árvore de trabalho nem stashes locais do proprietário**. Datas de marcos acima são as registradas nos documentos/PR; não inventar timestamp exato de testes não observado. Este arquivo documenta motivos e marcos, não substitui os critérios de aceite nem o estado atual da [TASKLIST](../TASKLIST.md) e do [checkpoint](PROJECT_STATE.md).

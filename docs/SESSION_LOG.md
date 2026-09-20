# SignalSpace — histórico recuperável da frente de desenvolvimento

Registro **seletivo**, não transcrição de conversas. Fonte de progresso: [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), Git e CI; decisões de comportamento: [produto](PRODUCT.md), [workspace](WORKSPACE_SECURITY.md), [contrato admin](LOCAL_ADMIN_AUTHORIZATION.md). A autoridade de IDs/estado é [TASKLIST](../TASKLIST.md); checkpoint derivado em [PROJECT_STATE](PROJECT_STATE.md). Não inventar eventos ou autoria não observados.

## 19–20/09/2026 — diagnóstico e leitura experimental (SS-BE-001)

- O proprietário relatou conexão OAuth e correlação do `diagnosticID` de `connection_diagnostic` com log após `tools/list`; ver [QUICK_TUNNEL.md](QUICK_TUNNEL.md) e PR. Não houve atestação criptográfica do cliente.
- Em 20/09, relatou `read_file` com `Teste de leitura SignalSpace` e negativa após revoke; `list_directory` raiz `nested`/`readme.txt` e `nested/inside.txt` com negativa após revogação. Sem log de correlação individual da listagem nem horário exato da expiração JWT. Não foi repetido nesta etapa.
- Escopo efetivo: diagnóstico e leitura opt-in, sem aprovação por ferramenta, escrita, Git ou shell.

## 20/09/2026 — contrato e preparação de backend (SS-BE-002 a SS-BE-008)

- [Contrato admin](LOCAL_ADMIN_AUTHORIZATION.md) fixa 7676/7677 distintos, pareamento com sessão imediata, bootstrap/CSRF, grant anterior ao OAuth read, estados versionados, retenção e ausência de aprovação pública. [Alinhamento UX](https://github.com/LuigiAPCPereira/SignalSpace/blob/feat/frontend-oauth-consent/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md) confirma comportamento de interface, não segurança ou integração.
- Foram adicionados componentes isolados de transporte, sessão e HTTP admin, status público em 7676, snapshots e handlers de consulta/decisão não conectados ao Quick. [CI #98](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35518520439) passou para a revisão correspondente.
- [`7dbd5d2`](https://github.com/LuigiAPCPereira/SignalSpace/commit/7dbd5d21c3cb358e9f74d2200e2834371dfff689) migrou stdin para `DecideTerminal`, compartilhando decisão e revalidação do grant com API admin. [CI #106](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521202757) passou. Naquela revisão o alias legado `Approve` ainda tinha caminho independente, sem timestamps ou terminalização. [CI #107](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521299236) passou após checkpoint `08ffb52`. Estes são fatos históricos, não estado final.

## 20/09/2026 — adoção documental v2

- Antes da adoção, a ref `08ffb52` tinha protocolo 1.0 e trio AGENTS/protocolo/checkpoint sem TASKLIST, roadmap ou histórico. O Project forneceu especificação geral v2.0, que não se sincroniza automaticamente com o repositório.
- A adaptação operacional v2.0 entrou na branch backend com inventário SS-BE-001…008, marcos, histórico, checkpoint vinculado a SS-BE-004 e [relatório do Adoption Gate v2](ADOPTION_REPORT.md); [commit `0f5ddec`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0f5ddec789a11ebdeb55480561c623a04df14f67). Sem alterar Go, frontend, agendamentos ou merge nessa adoção.

## 20/09/2026 — conclusão de lifecycle OAuth no domínio (SS-BE-004)

- A partir de `0f5ddec`, o código passou a registrar `decided_at` UTC e versões, delegar `Approve` ao mesmo caminho do terminal/painel e reter apenas snapshots sem PKCE/CSRF/state em tombstones com limite de dez minutos/64 entradas. `COMPLETED` significa código emitido, não token trocado; status público de `DENIED`/`EXPIRED` mantém vínculo ao cookie OAuth. Expiração e negativa não viram autorização por polling.
- `/authorize/complete` só remove um pedido após revalidar grant, capacidade e gerar código; uma falha de grant não consome uma autorização existente. Foi incluída cobertura para decisão com resposta descartada e reconciliação via GET, negação após o prazo, retenção/limpeza, revogação da leitura antes da conclusão e vinte POSTs concorrentes emitindo um código no máximo.
- [CI #120](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35523886303) falhou em teste antigo que exigia 404 imediato após expiração; a retenção era comportamento novo previsto no contrato. O teste foi atualizado para exigir `EXPIRED` durante retenção e 404 após descarte. [CI #122](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35523999575) passou; em seguida, foi corrigida a corrida lógica de negativa já registrada versus vencimento e adicionada regressão. **[CI #124](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35524113001) PASS integral** no [commit `171fbc8`](https://github.com/LuigiAPCPereira/SignalSpace/commit/171fbc856677f6f7a2f8ed10b6bcc13322773af2): formato, todos os testes Go, race, vet e build.
- **Limite:** SS-BE-004 validada no domínio e CI; 7677 ainda não está conectado ao Quick, interface frontend não está integrada, simulação de resposta perdida não é teste de rede/navegador. Próximo gate: SS-BE-003 revisar derivação de frase-senha e autenticação, depois SS-BE-002/005/006/007 e SS-BE-008. PR permanece draft, sem merge.

## Limites do registro

O conector GitHub mostra arquivos versionados e metadados remotos, **não a worktree/stashes locais do proprietário**. Datas acima pertencem aos registros observados; não inventar timestamps de testes não observados. Este documento não substitui contratos, TASKLIST, Git/CI nem a versão real do Project/Codex/agendamentos.

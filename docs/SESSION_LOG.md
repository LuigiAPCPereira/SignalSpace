# SignalSpace — histórico recuperável da frente de desenvolvimento

Registro **seletivo**, não transcrição de conversas. Progresso: [PR #1](https://github.com/LuigiAPCPereira/SignalSpace/pull/1), Git/CI. Comportamento: [produto](PRODUCT.md), [workspace](WORKSPACE_SECURITY.md), [contrato admin](LOCAL_ADMIN_AUTHORIZATION.md). Autoridade de IDs/estado: [TASKLIST](../TASKLIST.md), checkpoint derivado [PROJECT_STATE](PROJECT_STATE.md). Não inventar eventos.

## 19–20/09/2026 — diagnóstico e leitura experimental (SS-BE-001)

- Proprietário relatou conexão OAuth com `diagnosticID` correlacionado após `tools/list`; [QUICK_TUNNEL.md](QUICK_TUNNEL.md) e PR. Software não atestado.
- Em 20/09, relatou `read_file` com `Teste de leitura SignalSpace` e negativa após revoke; `list_directory` raiz `nested`/`readme.txt` e `nested/inside.txt` com negativa após revogação. Sem log por listagem ou medição precisa da expiração JWT; não repetido nesta etapa.
- Apenas diagnóstico/leitura opt-in; nenhuma aprovação por ferramenta, escrita, Git ou shell.

## 20/09/2026 — contrato e preparação de backend (SS-BE-002 a SS-BE-008)

- [Contrato admin](LOCAL_ADMIN_AUTHORIZATION.md) fixa 7676/7677 distintos, pareamento com sessão imediata, bootstrap/CSRF, grant anterior ao OAuth read, estados versionados/retidos e nenhuma aprovação pública. [Alinhamento UX](https://github.com/LuigiAPCPereira/SignalSpace/blob/feat/frontend-oauth-consent/docs/FRONTEND_ADMIN_CONTRACT_ALIGNMENT.md) é UX, não segurança/integração.
- Componentes isolados de transporte, sessão, HTTP admin, status público em 7676, snapshots/handlers admin ainda sem Quick. [CI #98](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35518520439) passou na época.
- [`7dbd5d2`](https://github.com/LuigiAPCPereira/SignalSpace/commit/7dbd5d21c3cb358e9f74d2200e2834371dfff689) migrou stdin para `DecideTerminal`, compartilhando decisão/revalidação do grant; [CI #106](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521202757) e [CI #107](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35521299236) passaram. Lacunas descritas eram históricas.

## 20/09/2026 — adoção documental v2

- Ref `08ffb52`: protocolo 1.0 e trio AGENTS/protocolo/checkpoint sem TASKLIST/roadmap/histórico. Anexo do Project é especificação geral v2.0 não sincronizada.
- Adaptação v2.0 no backend com inventário SS-BE-001…008, marcos, histórico, checkpoint SS-BE-004 e [relatório Adoption Gate](ADOPTION_REPORT.md), [commit `0f5ddec`](https://github.com/LuigiAPCPereira/SignalSpace/commit/0f5ddec789a11ebdeb55480561c623a04df14f67). Nenhum Go/frontend/agendamento/merge naquela adoção.

## 20/09/2026 — ciclo OAuth no domínio (SS-BE-004)

- A partir de `0f5ddec`, `decided_at`/versões e delegação `Approve`, snapshots terminais sem PKCE/CSRF/state retidos até dez minutos e 64 entradas; `COMPLETED` significa código, não token. `/authorize/complete` valida grant/capacidade antes de consumir pedido. Testes cobrem resposta de decisão perdida simulada reconciliada por GET, negação após prazo, tombstone/revoke e 20 conclusões concorrentes.
- [CI #120](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35523886303) falhou em teste antigo de 404 imediato, incompatível com nova retenção; teste corrigido; [CI #122](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35523999575) passou. Depois corrigida prioridade DENIED vs expiração; [CI #124](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35524113001) PASS no [commit `171fbc8`](https://github.com/LuigiAPCPereira/SignalSpace/commit/171fbc856677f6f7a2f8ed10b6bcc13322773af2). Validação de domínio/CI, sem Quick/browser/rede real.

## 20/09/2026 — endurecimento do Gate administrativo (SS-BE-003)

- Contrato exigia Argon2id ou equivalente revisado; `internal/admin/session.go` empregava PBKDF2-HMAC-SHA256 próprio sem revisão independente. Em [`d4c8c3a`](https://github.com/LuigiAPCPereira/SignalSpace/commit/d4c8c3a9bf1c1d9236bafabddb6560d12a2dd158), substituído por `golang.org/x/crypto/argon2` v0.41.0 compatível com Go 1.23, checksum versionado, sal aleatório 16 bytes, custo fixo 32 MiB/3 passagens/1 via, 32 bytes derivados. Adicionado cap global 64 sessões e erro 429 já na quinta falha de desbloqueio. Testes de capacidade, derivação, invalidade de senha e recuperação; [CI #132](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35525663837) PASS.
- Em [`1c73a7f`](https://github.com/LuigiAPCPereira/SignalSpace/commit/1c73a7f8f9f1778aee1a780fbe086dd07c7357ac), `lock`/`refresh` passaram a diferenciar cookie ausente/expirado (`401 AUTH_REQUIRED`) e CSRF incorreto (`403 ACCESS_DENIED`), revalidando no efeito sob mutex. Teste HTTP usa Gate real e confirma nenhum revogação indevida após erro CSRF, refresh válido, lock e cookie revogado. [CI #134](https://github.com/LuigiAPCPereira/SignalSpace/actions/runs/35525837834) PASS em formato, `go test ./...`, race, vet e build no último commit de código.
- **Limites:** nenhuma auditoria criptográfica independente, porta 7677 ligada ao Quick, browser funcional, perda de rede real, merge ou deploy. SS-BE-003 validada apenas como componente isolado; próxima SS-BE-002, depois SS-BE-005/007/006/008 conforme gates. Frontend não modificado.

## Limites do registro

Conector GitHub mostra arquivos versionados e metadados remotos, **não worktree/stashes locais**. Datas acima pertencem a registros observados; não inventar tempos de teste. Documento não substitui contratos, TASKLIST, Git/CI ou versões reais do Project/Codex/agendamentos.

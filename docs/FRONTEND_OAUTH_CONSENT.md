# Frontend — consentimento OAuth

**Status:** template de interface preparado, ainda não integrado ao servidor. **Branch:** `feat/frontend-oauth-consent`. **Base congelada:** `496802b337adf9761994e6ae0e765a007f509190`, derivada do PR #1 em 20/09/2026.

Este documento registra a primeira fatia exclusivamente de frontend do SignalSpace. A interface principal continua sendo o ChatGPT; esta página existe somente para explicar o pedido OAuth e instruir a confirmação confiável do proprietário. O design não cria nem substitui autorização no backend.

## Evidência observada na base

- `internal/auth/server.go` renderiza um template HTML embutido em `GET /authorize` com dados `ID`, `Client`, `ClientID`, `Redirect`, `CSRF`, `Scope` e `Read`.
- `Read` distingue solicitação somente de diagnóstico da combinação exata de diagnóstico com leitura. A concessão de pasta é independente do OAuth.
- A decisão confiável é efetuada pelo terminal local com `approve <id>` ou `deny <id>`; a página pública não possui rota de aprovação.
- O navegador faz `POST /authorize/complete` com `request` e `csrf`. O servidor responde com `409` JSON enquanto não houve decisão; negação e expiração retornam `403` JSON; aprovação válida gera redirecionamento `303` para o retorno OAuth registrado.
- A solicitação expira em cinco minutos. Não existe, nesta base, um endpoint para consultar o status no navegador ou um contrato de estados de UI além da página inicial.
- O nome do cliente vem de metadado informado pelo aplicativo; o `client_id` não comprova que o software seja o ChatGPT.

## O que foi preparado nesta branch

`frontend/consent/consent.html` é um template Go `html/template` alternativo, **não conectado ao runtime**. Usa exatamente os campos e a ação do formulário que já existem. Inclui tipografia e layout responsivos, descrição contextual do escopo de leitura, identificação explícita do nome não verificado, destino de retorno visível, comando de terminal e uma ação primária. Os estilos ficam no próprio arquivo porque o servidor atual anuncia `style-src 'unsafe-inline'` e `default-src 'none'`; não são adicionados scripts, fontes remotas, imagens ou requisições extras.

A interface não afirma que a conexão foi concluída, que houve aprovação, nem que o token já foi emitido. Não contém botões próprios para confirmar uma chamada MCP, modo de aprovação permanente ou acesso presumido a arquivos e shell.

## Hierarquia e interação

1. Mostrar o propósito: autorizar uma conexão OAuth com o SignalSpace.
2. Exibir cliente com o aviso de que seu nome não é verificado, `client_id` e destino exato de retorno.
3. Mostrar somente as permissões presentes no pedido real, distinguindo diagnóstico de leitura.
4. Instruir a aprovação ou recusa pelo terminal e informar que a página pública não autoriza sozinha.
5. Disponibilizar `Já autorizei no terminal — continuar` como envio do formulário OAuth existente.
6. Informar expiração em cinco minutos e que a etapa não confirma o funcionamento do MCP.

A interface não consulta estados automaticamente e não faz atualização otimista. O formulário pode retornar o JSON de erro atual se for enviado antes da decisão; **isso ainda não foi corrigido** porque o tratamento e o contrato dessa resposta pertencem ao backend. Não simular estado aprovado com JavaScript.

## Estados e dependências do backend

| Estado | Evidência disponível | Comportamento de UI nesta fatia | Dependência para evolução |
| --- | --- | --- | --- |
| Solicitação apresentada | GET validado pelo servidor | Exibir dados reais e instrução local | Nenhuma além da integração do template |
| Aprovação pendente | POST responde `409 authorization_pending` | O template não trata o retorno; erro JSON atual permanece | Definir resposta HTML segura ou endpoint autenticado de estado, sem permitir aprovar |
| Aprovada no terminal | Backend registra a decisão | Usuário envia formulário; redirecionamento só quando o servidor aprovar | Não apresentar aprovado sem estado verificável |
| Recusada | POST responde `403 access_denied` | O template não trata o retorno; erro JSON atual permanece | Definir resposta de recusa orientada ao usuário |
| Expirada | Pendência removida; POST responde `403 access_denied` | Texto informa prazo; não afirma cronômetro em tempo real | Diferenciar expiração de recusa apenas se o backend fornecer essa distinção com segurança |
| Erro / indisponível | Resposta do backend, sem contrato visual | Sem simulação de recuperação | Contrato de erro seguro e mensagem adequada |

Não confundir expiração do pedido OAuth, expiração do token e revogação imediata da concessão de workspace. Uma mudança nos estados HTTP ou uma interface local de aprovação requer definição e testes na frente de backend antes da integração.

## Fronteira de integração com a outra sessão

A equipe de backend poderá avaliar, em mudança própria e após conferir HEAD e diff atualizados, substituir o template embutido pelo arquivo de frontend e adaptar o carregamento de forma explícita. O frontend **não altera** `internal/auth`, rotas, cookies, CSRF, PKCE, escopos, concedentes, tokens ou políticas de autorização nesta branch. O carregamento do template não foi implementado nem testado; não tratar o arquivo como página já publicada.

Se o backend optar por melhorar as respostas `409` e `403`, deve preservar os códigos/semânticas necessárias para consumidores existentes e provar as negativas de segurança. Nenhum endpoint de aprovação pública pode ser introduzido para facilitar o frontend. Um painel local de consentimento do proprietário, se retomado, é uma decisão separada e não deve reproduzir aprovações por chamada MCP do ChatGPT.

## Aceitação e validação pendente

- Integrar exclusivamente a apresentação, comprovando que os campos atuais são renderizados e escapados por `html/template` sem injetar HTML de metadados do cliente.
- Testar pedido de diagnóstico e de leitura com escopos reais; não mostrar permissões futuras como disponíveis.
- Verificar que a submissão preserva `request`, `csrf`, a exigência de decisão local e o redirecionamento registrado.
- Exercitar pendente, recusa, expiração, sessão/CSRF inválidos e concessão revogada com testes negativos; sem transformar falha em sucesso visual.
- Inspecionar as larguras 320, 360, 390, 768 e 1440 px, zoom 200%, teclado/foco, textos longos, contraste AA e sem dependência de movimento.
- Rodar `go test ./...`, `go vet ./...`, `go build ./...` e smoke real apropriado **após** integração ao servidor. Nenhum desses gates foi executado nesta branch exclusivamente documental/de apresentação.

## Fora do escopo

Painel de aprovação de chamadas MCP, novos endpoints, UI de workspace, mudança na emissão de tokens, instalação GitHub, design system genérico e integração com o PR de roadmap GitHub. Não abrir merge da branch de frontend antes de revisar a compatibilidade com a evolução do PR #1.

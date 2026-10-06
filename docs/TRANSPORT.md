# Transporte HTTPS: preparação e diagnóstico (M1)

**Estado:** o comando de diagnóstico está implementado e testado com um servidor HTTPS de teste. O comando `connect quick` gerencia um Quick Tunnel experimental **somente após confirmação explícita**, usa estado OAuth temporário e verifica o HTTPS antes de apresentar a URL. **Não há túnel configurado na máquina do proprietário nem chamada real observada no ChatGPT Web.** O único método MCP continua sendo `connection_diagnostic`; não existem ferramentas de arquivos, Git ou terminal. A exposição pública é permitida somente na sessão experimental confirmada; não publicar o modo persistente como serviço de produção.

Para experimentar gratuitamente **sem conta Cloudflare ou domínio**, veja [Quick Tunnel automatizado](QUICK_TUNNEL.md). Essa é a modalidade inicial; o túnel nomeado abaixo é uma alternativa de URL estável e ainda depende de configuração manual.

## Contrato que o diagnóstico verifica

O serviço continua escutando somente em `127.0.0.1:7676`. Uma URL pública HTTPS estável, por exemplo `https://mcp.example.com/mcp`, será o endpoint MCP e o identificador de recurso OAuth anunciado nos metadados. O emissor integrado será `https://mcp.example.com`, sem `/mcp`. O cliente precisa reutilizar **exatamente** o identificador `resource` anunciado nos pedidos de autorização e token, e esse valor será o `aud` do token. O caminho `/mcp` no identificador é intencional e não deve ser removido apenas porque exemplos da documentação usam a origem sem caminho.

Execute em um terminal separado, **somente depois de ter um transporte autorizado e ativo**:

```bash
export SIGNALSPACE_AUTH_MODE=embedded
export SIGNALSPACE_RESOURCE_URL='https://mcp.example.com/mcp'
go run ./cmd/signalspace doctor transport
```

O diagnóstico usa HTTPS com validação normal de certificado, tempo limite por requisição, limite de resposta e não segue redirecionamentos. Ele verifica os metadados de recurso protegido, emissor, endpoints OAuth, chave JWKS, PKCE S256, DCR e os desafios de autenticação HTTP e MCP. Não envia tokens, não registra clientes e não aprova solicitações. Uma conclusão positiva **não demonstra** login do proprietário, troca de tokens pelo ChatGPT ou invocação real.

O comando exige modo integrado e URL HTTPS canônica e rejeita variáveis de configuração dos outros modos. Não execute `doctor transport` contra um servidor que você não controla: a URL é fornecida pelo usuário, e o diagnóstico fará requisições a ela.

## Exemplo de transporte nomeado Cloudflare (configuração manual; não provisionada)

Essa é **uma opção**, não uma dependência obrigatória. O túnel **nomeado** precisa de conta Cloudflare, domínio sob gerenciamento do serviço, instalação de `cloudflared` e uma rota DNS para um hostname estável. Quick Tunnels em `trycloudflare.com` recebem uma URL aleatória e são suportados no comando separado `connect quick`, com estado isolado, nunca como identidade OAuth persistente. Consulte a [documentação oficial do Tunnel](https://developers.cloudflare.com/tunnel/get-started/) e do [túnel gerenciado localmente](https://developers.cloudflare.com/tunnel/features/locally-managed-tunnels/create-local-tunnel/).

Depois que você tiver um túnel **nomeado** e um domínio, um exemplo de `config.yml` do `cloudflared` é:

```yaml
tunnel: <TUNNEL_UUID>
credentials-file: /home/USER/.cloudflared/<TUNNEL_UUID>.json
ingress:
  - hostname: mcp.example.com
    service: http://127.0.0.1:7676
    originRequest:
      httpHostHeader: mcp.example.com
  - service: http_status:404
```

Substitua os marcadores pelo UUID e pelo arquivo de credenciais gerados na sua máquina. Não versionar `config.yml` com caminhos privados ou arquivos de credenciais; nunca copiar a chave de túnel para o repositório ou para a conversa. O campo `httpHostHeader` deve corresponder **exatamente** ao hostname de `SIGNALSPACE_RESOURCE_URL`; o servidor rejeita cabeçalhos `Host` inesperados. Não habilite cache na rota OAuth/MCP nem aplique autenticação intermediária que impeça o ChatGPT de alcançar a descoberta e o navegador de chegar à aprovação. Não confie em cabeçalhos `X-Forwarded-*` como identidade.

A documentação da Cloudflare descreve `cloudflared tunnel login`, criação do túnel nomeado, roteamento DNS, `cloudflared tunnel ingress validate` e execução do túnel. **Estes passos publicam um serviço acessível na internet; não execute a criação da rota pública nem a inicialização do túnel nomeado até a revisão de segurança do modo persistente.** O SignalSpace não instala `cloudflared` nem abre portas de entrada; somente `connect quick` inicia e encerra seu próprio processo de túnel temporário após consentimento.

## O que falta para marcar a conexão real como validada

Com o transporte e a conta do usuário disponíveis, verificar no ChatGPT Web: descoberta MCP, DCR do cliente correto, consentimento na página HTTPS, aprovação **no terminal local**, retorno `iss`/`state`, troca com PKCE e token `aud` correto e, finalmente, chamada à ferramenta `connection_diagnostic` autenticada. Registrar apenas o resultado, sem tokens ou códigos. O auth harness v2 agora cobre refresh/rotação/revogação de token family localmente; isso não substitui o aceite externo nem prova proteção contra abuso público ou uma experiência de instalação confiável.

## OpenAI Secure MCP Tunnel — transporte privado preferido para Programming

O [Secure MCP Tunnel da OpenAI](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels) passa a ser o transporte privado persistente preferido para a composição Programming. Ele mantém o MCP local sem regra de entrada pública: o `tunnel-client` abre a conexão HTTPS de saída e encaminha o canal principal para `http://127.0.0.1:7676/mcp`. O painel `localhost:7677` nunca é configurado como target.

O modo SignalSpace é explícito: `connect tunnel programming`. Ele não reutiliza o OAuth browser-facing do Quick; no app privado do ChatGPT o contrato alvo é Connection=Tunnel e Authentication=No authentication. O último hop é autenticado por uma credencial local separada, entregue ao `tunnel-client` como `X-SignalSpace-Tunnel-Token: file:<caminho>`. Esse mecanismo não é capability authorization: depois dele, toda tool continua passando por grant, Standard Profile, Policy Engine e approvals locais.

A credencial gera um principal estável para **um runtime/tunnel dedicado ao SignalSpace**. Esse principal é compartilhado por quem puder usar esse Tunnel; não é uma identidade individual atestada de usuário. Por isso não reutilizar o runtime/principal de outro MCP e não inferir permissões por display name. Connector-forwarded headers podem substituir static extra headers; qualquer substituição incorreta da credencial resulta em negação, nunca fallback.

O SignalSpace não cria, registra, inicia nem supervisiona `tunnel-client`: esse lifecycle continua operator-owned. Quick Tunnel permanece disponível como dev/smoke/compatibilidade OAuth, não como requisito da conexão persistente. O modo Tunnel não é uma “origem pública estável”; é um caminho privado hospedado pela OpenAI até um MCP que continua local.

## Relação com autorização local v2

O desenho aceito em [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md) trata OAuth como autenticação da composição, não como concessão de filesystem, Git ou shell. O auth harness opt-in e a composição pública `connect quick programming` implementam `signalspace:programming` e o ciclo refresh v2; `diagnostic`/`read` e o construtor legado continuam no contrato granular. O transporte carrega a identidade e o recurso corretos; a decisão local continua pertencendo ao SignalSpace.

**Reconciliação da ponte v2 (26/09/2026):** a discovery Programming anuncia somente o scope da composição e exatamente as tools aprovadas. Cada `tools/call` exige OAuth Programming e autorização local antes do dispatch; `LOCAL_APPROVAL_REQUIRED` é resultado pendente sem efeito e o retry exato consome o permit uma vez. A prova feita foi HTTP local sem Quick Tunnel; aceite ChatGPT Web/HTTPS externo permanece desconhecido.

Quick Tunnel continua temporário e apropriado somente a dev/smoke. A experiência persistente exigirá uma origem estável, a escolher em tarefa própria; esta reconciliação não escolhe Named Tunnel, domínio, provedor ou relay e não altera o endpoint atual.

## Preflight HTTPS composition-aware (`SS-MVP-002-QUICK-PREFLIGHT-COMPOSITION-V2-001`)

O preflight HTTPS público é composition-aware:
- `CheckEmbeddedTransportForScope(ctx, resourceURL, expectedCompositionScope, client)` valida o transporte público contra o escopo fechado esperado pela composição planejada.
- O wrapper `CheckEmbeddedTransport(...)` preserva compatibilidade exigindo `signalspace:diagnostic`.
- `connect quick programming` deriva exclusivamente do plano local fechado o escopo `signalspace:programming`; `connect quick diagnostic` e `connect quick read` utilizam `signalspace:diagnostic`.
- O preflight valida:
  1. Protected-Resource Metadata (`/.well-known/oauth-protected-resource`): `resource` igual à URL canônica configurada, `authorization_servers` contendo exatamente a origem do emissor e `scopes_supported` contendo o escopo esperado da composição.
  2. Authorization Server Metadata (`/.well-known/oauth-authorization-server`): `issuer`, endpoints OAuth canônicos, JWKS, DCR e confirmação de que `scopes_supported` anuncia o escopo esperado da composição.
  3. Desafio HTTP não autenticado: requisição MCP sem bearer recebe `401 Unauthorized` com cabeçalho `WWW-Authenticate` apontando `resource_metadata` e o `scope` exato da composição.
  4. Desafio MCP não autenticado: chamada a `connection_diagnostic` sem token recebe `isError: true` com desafio `_meta.mcp/www_authenticate` contendo `resource_metadata`, `scope` exato da composição e `error="invalid_token"`.


### Configuração operacional esperada

1. iniciar `signalspace connect tunnel programming` e confirmar a frase owner-side;
2. usar um Tunnel/runtime OpenAI **dedicado** ao SignalSpace;
3. configurar o main MCP para `http://127.0.0.1:7676/mcp`;
4. configurar o extra header `X-SignalSpace-Tunnel-Token` usando a referência `file:` exibida pelo SignalSpace, sem copiar o valor para config versionada;
5. selecionar esse Tunnel no app ChatGPT com autenticação do MCP desativada;
6. manter `localhost:7677` somente para o proprietário;
7. criar o grant Programming por `workspace request-programming` ou `workspace request-worktree`.

A implementação foi validada em CI no SHA `880637ccd6614d9a4bfbba67794977804d7878bd` (run #37517878621). O runtime `tunnel-client` real e o ChatGPT Tunnel ainda exigem aceite operacional no host do proprietário.

# OAuth no SignalSpace: servidor de recursos (fatia M1)

**Status:** código implementado e testado com emissor/JWKS simulado local. Nenhum provedor OAuth real, túnel HTTPS ou conta do ChatGPT Web foi validado. Este documento NÃO é um guia de instalação completa do conector.

## O que a implementação faz

O SignalSpace permanece escutando exclusivamente em `127.0.0.1:7676`. Ao configurar `SIGNALSPACE_RESOURCE_URL`, ele ativa um handler separado do modo local:

- `GET /.well-known/oauth-protected-resource` e `GET /.well-known/oauth-protected-resource/mcp`: metadados com recurso canônico, emissor e escopo `signalspace:diagnostic`.
- `POST /mcp` sem token: `401` e desafio `WWW-Authenticate` com URL dos metadados.
- Tokens JWT RS256: assinatura verificada com JWKS HTTPS configurado; emissor (`iss`), destinatário (`aud`), expiração (`exp`), validade inicial (`nbf`, quando presente), sujeito (`sub`) comparado ao proprietário configurado e escopo verificados em toda requisição MCP. Falha de escopo recebe `403`.
- Busca JWKS limitada a 64 KiB e 3 s, sem redirecionamentos; cache de 5 minutos que não é usado após expirar se a atualização falhar.
- A ferramenta `connection_diagnostic` declara `securitySchemes` OAuth; nenhuma operação de arquivos, shell ou Git é disponibilizada.

O token de diagnóstico local não é aceito no modo OAuth, e configuração OAuth parcial não causa fallback para o modo local.

## Configuração do modo OAuth

Exemplo ilustrativo, NÃO funcional sem um emissor real compatível:

```bash
export SIGNALSPACE_RESOURCE_URL='https://signalspace.example.com/mcp'
export SIGNALSPACE_OAUTH_ISSUER='https://identity.example.com/'
export SIGNALSPACE_JWKS_URL='https://identity.example.com/.well-known/jwks.json'
export SIGNALSPACE_OAUTH_OWNER_SUBJECT='subject-exato-obtido-do-provedor'
go run ./cmd/signalspace
```

A URL do recurso deve terminar exatamente em `/mcp`. O hostname público deve chegar preservado como cabeçalho `Host`; cabeçalhos `X-Forwarded-*` não são fontes de autoridade. O processo local nunca escuta em `0.0.0.0`.

**O emissor OAuth é um componente externo ainda não configurado.** Ele precisa disponibilizar metadados de autorização padronizados, código de autorização com PKCE S256, identificação/registro do cliente (CIMD, DCR ou cliente predefinido), preservar o parâmetro `resource` e emitir JWTs RS256 com o `aud` exato do recurso, escopo e demais campos exigidos. O SignalSpace não implementa login, consentimento ou endpoint de emissão de tokens nesta fatia.

O `SIGNALSPACE_OAUTH_OWNER_SUBJECT` é o valor **exato e estável do `sub` emitido pelo provedor para o proprietário**, não seu nome ou email presumido. Sem ele, o modo OAuth falha na inicialização. Um token válido do mesmo emissor, com o mesmo escopo, mas de outro `sub`, é rejeitado. Não inserir um valor fictício em produção.

O JWKS informado deve pertencer ao provedor confiável. O nome do host, o emissor e o recurso são configuração estática do proprietário; não são determinados pela requisição. Tokens não são registrados.

## O que falta comprovar antes de expor o serviço

1. Selecionar/configurar um emissor OAuth confiável que atenda aos requisitos acima e testar a descoberta, registro do cliente, PKCE e emissão de tokens destinados ao SignalSpace.
2. Validar um túnel HTTPS cujo encaminhamento preserve o `Host`, com autorização negativa e positiva em ambiente controlado.
3. Conectar o ChatGPT Web e observar uma chamada REAL da ferramenta; conferir disponibilidade na configuração da conta/modelo.
4. Acrescentar autorização dos workspaces antes de disponibilizar leitura, edição, Git e shell. O shell rodará com os privilégios do usuário, sem sandbox neste MVP.

Não apresentar esta fatia como conexão de ChatGPT pronta. O resultado `chatgptVerified: false` da ferramenta permanece intencional.

Referências: [autenticação de plug-ins da OpenAI](https://developers.openai.com/pt-BR/plugins/build/auth) e [autorização do MCP](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization).

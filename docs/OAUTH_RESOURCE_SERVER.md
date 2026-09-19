# OAuth no SignalSpace: servidor de recursos (fatia M1)

**Status:** código implementado e testado com emissor/JWKS simulado local. Nenhum provedor OAuth real, túnel HTTPS ou conta do ChatGPT Web foi validado. Este documento **não** é um guia de instalação completa do conector.

## Comportamento implementado

O SignalSpace permanece escutando exclusivamente em `127.0.0.1:7676`. Ao configurar `SIGNALSPACE_RESOURCE_URL`, ele ativa um handler separado do diagnóstico com token local:

- `GET /.well-known/oauth-protected-resource` e `GET /.well-known/oauth-protected-resource/mcp`: metadados com recurso canônico, emissor e escopo `signalspace:diagnostic`.
- `POST /mcp` sem token: `401` e desafio `WWW-Authenticate` com URL dos metadados.
- Tokens JWT RS256: assinatura verificada com JWKS HTTPS configurado; emissor (`iss`), destinatário (`aud`), expiração (`exp`), validade inicial (`nbf`, se presente), sujeito não vazio (`sub`) e escopo verificados em cada requisição MCP. Falta de escopo recebe `403`.
- Busca JWKS limitada a 64 KiB e 3 segundos, sem redirecionamentos; cache de cinco minutos que não é usado após expirar quando a atualização falha.
- A ferramenta `connection_diagnostic` declara `securitySchemes` OAuth; não há operações de arquivos, shell ou Git.

O token do diagnóstico local não funciona no modo OAuth. Configuração OAuth parcial não permite fallback para o modo local.

## Configuração ilustrativa

Este exemplo **não funciona sem um provedor real compatível**:

```bash
export SIGNALSPACE_RESOURCE_URL='https://signalspace.example.com/mcp'
export SIGNALSPACE_OAUTH_ISSUER='https://identity.example.com/'
export SIGNALSPACE_JWKS_URL='https://identity.example.com/.well-known/jwks.json'
go run ./cmd/signalspace
```

A URL do recurso deve terminar exatamente em `/mcp`. O proxy deve preservar o `Host` público e usar HTTPS para a conexão externa. Cabeçalhos `X-Forwarded-*` vindos do cliente não determinam a identidade do recurso. O processo local nunca escuta em `0.0.0.0`.

**O emissor OAuth é externo e ainda não está configurado.** Ele precisa oferecer metadados de autorização padronizados, autorização por código com PKCE S256, identificação/registro do cliente compatível com ChatGPT, suporte ao parâmetro `resource` e JWTs RS256 cujo `aud` é exatamente a URL do SignalSpace, além dos demais campos exigidos. O SignalSpace não implementa login, consentimento ou emissão de tokens. A emissão de token com o escopo não constitui, por si só, prova de autorização exclusiva do proprietário; a configuração do provedor e a vinculação ao proprietário precisam ser validadas antes de permitir ferramentas locais.

O JWKS deve pertencer ao provedor confiável configurado. O nome do host, o emissor e o recurso são configuração estática do proprietário, não derivados da requisição. Tokens não são registrados.

## Gates antes de disponibilizar acesso externo

1. Configurar um emissor OAuth confiável; validar descoberta, registro do cliente, PKCE, identidade/autorização do proprietário e emissão de token com destinatário correto.
2. Testar túnel HTTPS com `Host` preservado e chamadas negativas/positivas em ambiente controlado.
3. Conectar pelo ChatGPT Web e observar uma chamada real da ferramenta com os recursos efetivamente disponíveis na conta/modelo.
4. Implementar autorização de workspaces antes de disponibilizar leitura, edição, Git ou shell. O shell executará como usuário local e não será sandbox.

**Não exponha esta versão publicamente ainda.** O resultado `chatgptVerified: false` é intencional.

Referências: [autenticação de plug-ins da OpenAI](https://developers.openai.com/pt-BR/plugins/build/auth) e [autorização MCP 2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18/basic/authorization).

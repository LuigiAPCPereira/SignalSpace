# Diagnóstico de integração OAuth (sem abrir túnel)

**Estado:** preflight implementado em Go. Verifica documentos e chaves publicadas pelo provedor, sem testar login, consentimento ou ChatGPT Web.

## Executar

Configure as quatro variáveis abaixo com valores reais da configuração do proprietário. Não use valores ilustrativos para interpretar o resultado como válido.

```bash
export SIGNALSPACE_RESOURCE_URL='https://seu-host-mcp.example/mcp'
export SIGNALSPACE_OAUTH_ISSUER='https://seu-provedor.example/'
export SIGNALSPACE_JWKS_URL='https://seu-provedor.example/.well-known/jwks.json'
export SIGNALSPACE_OAUTH_OWNER_SUBJECT='sub-exato-do-proprietario'
go run ./cmd/signalspace doctor oauth
```

O comando valida as URLs, o proprietário explícito e consulta somente endpoints de descoberta derivados do emissor configurado. Tenta `/.well-known/oauth-authorization-server` e, somente em caso de HTTP 404, `/.well-known/openid-configuration`. Não segue redirecionamentos, limita a resposta a 64 KiB e a operação a cinco segundos. Verifica correspondência exata do `issuer`, endpoints HTTPS, suporte anunciado a authorization code + PKCE S256, JWKS correspondente ao configurado e chaves RSA compatíveis. Identifica CIMD, DCR ou a necessidade de pré-cadastro do cliente. Se CIMD estiver anunciado, exige `none` ou `private_key_jwt` nos métodos de autenticação do token endpoint.

**A saída `OAuth metadata: verified` não é atestado de conexão.** A descoberta não prova que o provedor aceita o cliente ChatGPT, preserva `resource` nos pedidos de autorização e de token, emite `aud` e escopo corretos, autentica o proprietário nem realiza consentimento. Também não valida túnel TLS ou invocação real do ChatGPT. Se o resultado indicar `pre_registered_client_required`, falta cadastrar manualmente um cliente e configurar o ChatGPT; o diagnóstico não faz isso automaticamente.

O comando não solicita nem armazena senha, client secret ou access token e não inicia servidor MCP. A aplicação normal continua presa a `127.0.0.1` e nenhuma ferramenta de arquivo, Git ou shell é ativada. **Não publique esta versão por túnel ainda.**

Documentação de referência: [OpenAI — autenticação MCP](https://developers.openai.com/pt-BR/plugins/build/auth), [RFC 8414](https://www.rfc-editor.org/rfc/rfc8414), [Auth0 — uso do parâmetro resource](https://auth0.com/docs/api/authentication/authorization-code-flow-with-pkce/authorize-with-pkce).

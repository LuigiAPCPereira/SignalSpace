# Integração Auth0 — preparação, ainda não validada

Este roteiro **não comprova conexão com ChatGPT Web**. O SignalSpace somente implementa o servidor de recursos OAuth e o diagnóstico MCP; o login, a autorização, a emissão de tokens e o registro do cliente pertencem ao Auth0. Não publique esta versão por túnel ainda.

## Configuração do provedor

1. Em um tenant de desenvolvimento Auth0 controlado pelo proprietário, registre uma API cujo **Identifier** corresponda exatamente ao `SIGNALSPACE_RESOURCE_URL` (por exemplo, `https://seu-dominio.example/mcp`), com assinatura **RS256** e o escopo `signalspace:diagnostic`. Confirme a identidade do emissor pelo documento de descoberta publicado pelo tenant, não a deduza do domínio.
2. Configure o fluxo **authorization code + PKCE S256** e um método de identificação de cliente que o ChatGPT suporte: CIMD, DCR ou cliente predefinido. Se usar DCR, ele está desabilitado por padrão no Auth0: habilite somente em tenant de desenvolvimento e estabeleça as permissões explícitas da API para clientes de terceiros. Não ative o registro aberto indiscriminadamente em um tenant de produção.
3. No Auth0, habilite **Resource Parameter Compatibility Profile**, necessário para que `resource` seja reconhecido como destinatário da API. Verifique que o JWT de acesso emitido contenha `iss`, `aud`, `sub`, `exp` e o escopo esperado. O valor `sub` imutável da conta do proprietário deve ser configurado em `SIGNALSPACE_OAUTH_OWNER_SUBJECT`; email não substitui `sub`.
4. Configure `SIGNALSPACE_RESOURCE_URL`, `SIGNALSPACE_OAUTH_ISSUER`, `SIGNALSPACE_JWKS_URL` e `SIGNALSPACE_OAUTH_OWNER_SUBJECT` como descrito em [OAUTH_RESOURCE_SERVER.md](OAUTH_RESOURCE_SERVER.md). A URL JWKS deve ser a declarada pelo provedor confiável. Não publique segredos, tokens ou dados da conta no repositório.

## Contrato de vinculação implementado

- Metadados do recurso protegido e `WWW-Authenticate` HTTP para clientes ainda não autorizados.
- A descrição de `connection_diagnostic` inclui `securitySchemes` OAuth com escopo `signalspace:diagnostic` após a descoberta autenticada.
- Chamadas `tools/call` reconhecidas sem token, com token inválido ou com escopo insuficiente retornam resultado MCP `isError: true` e `_meta["mcp/www_authenticate"]`, incluindo `error` e `error_description`. Outras mensagens sem autorização continuam recebendo HTTP 401/403.
- Somente tokens do `sub` exato do proprietário podem executar a ferramenta. A ferramenta não abre arquivos nem executa comandos.

## Aceitação pendente

Confirmar, em ambiente controlado: descoberta do emissor e do recurso; cadastro do cliente; login e consentimento; propagação correta de `resource` para `aud`; tokens de proprietário aceitos; tokens ausentes, adulterados, expirados, sem escopo ou de outra conta rejeitados; execução de `connection_diagnostic` **observada no ChatGPT Web**. Validar o comportamento do Host no túnel HTTPS antes de disponibilizar acesso externo. Até lá, não declarar a conexão pronta e não liberar ferramentas de arquivos, Git ou shell.

Referências: [OpenAI — autenticação de servidores MCP](https://developers.openai.com/plugins/build/auth), [Auth0 — registro dinâmico de clientes](https://auth0.com/docs/get-started/applications/dynamic-client-registration), [Auth0 — Resource Parameter Compatibility Profile](https://support.auth0.com/center/s/article/mcp-audience-error-with-auth0).

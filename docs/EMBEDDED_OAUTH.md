# Autorização integrada (fatia experimental do M1)

**Status:** a emissão local de tokens e a autorização no terminal foram implementadas em Go e testadas de ponta a ponta com um cliente MCP simulado. **Não houve conexão real ao ChatGPT Web nem teste de túnel HTTPS. Não exponha esta versão à internet.**

O SignalSpace pode atuar como **servidor de autorização e servidor de recursos no mesmo processo**, sem exigir uma conta no Auth0. O modo anterior, com provedor OAuth externo, continua opcional e separado. Este modo não implementa uma conta de usuário ou um sistema de senhas: a identidade do proprietário é definida pelo controle do terminal que executa o serviço.

## Executar em ambiente de desenvolvimento

```bash
export SIGNALSPACE_AUTH_MODE=embedded
export SIGNALSPACE_RESOURCE_URL='https://signalspace.example/mcp'
go test ./...
go run ./cmd/signalspace
```

A URL acima é **fictícia**: não conecta o ChatGPT. Ela deve ser substituída por uma URL HTTPS real quando houver um transporte aprovado e testado. O serviço permanece escutando somente em `127.0.0.1:7676`; o proxy/túnel deverá preservar o cabeçalho `Host`. Não definir `SIGNALSPACE_OAUTH_ISSUER`, `SIGNALSPACE_JWKS_URL`, `SIGNALSPACE_OAUTH_OWNER_SUBJECT` nem `SIGNALSPACE_LOCAL_TOKEN` neste modo. Configuração misturada falha na inicialização.

A descoberta é publicada em `/.well-known/oauth-protected-resource` e `/.well-known/oauth-authorization-server`. O servidor oferece **DCR** em `/register`, código de autorização com PKCE S256 em `/authorize`, troca em `/token` e chave pública em `/oauth/jwks`. O único escopo é `signalspace:diagnostic`.

O registro aceita apenas URIs de retorno HTTPS cujo host seja exatamente `chatgpt.com`, sem porta ou parâmetros de consulta. Mesmo um cliente que se identifique como ChatGPT não é confiável só por causa de seu nome. O usuário deve conferir o destino exibido e aprovar cada solicitação no terminal.

## Aprovar uma conexão

1. O cliente inicia o OAuth com `resource` igual à URL exata do MCP, código de autorização e PKCE S256. O navegador exibe o nome declarado pelo cliente, o destino de retorno e um identificador aleatório de solicitação.
2. Na **mesma janela do terminal** em que o SignalSpace está rodando, o proprietário digita `approve IDENTIFICADOR` ou `deny IDENTIFICADOR`. Não existe rota HTTP para aprovar a conexão.
3. Após a aprovação, o proprietário clica em **Continuar** no navegador. O SignalSpace confere cookie HttpOnly/Secure e token CSRF, devolve um código de uso único ao retorno registrado e exige PKCE, `client_id`, `redirect_uri` e `resource` exatos na troca.
4. O token JWT RS256 emitido dura 15 minutos e é verificado pelo próprio servidor MCP antes da ferramenta de diagnóstico. A recusa ou expiração impede a emissão. O identificador da solicitação não é uma credencial.

## Limites ainda abertos

- Clientes registrados, códigos e a **chave de assinatura** ficam somente em memória. Reiniciar o processo invalida tokens e o registro DCR; a vinculação do ChatGPT precisaria ser refeita. Persistência segura, revogação e atualização de tokens ainda não estão implementadas.
- O registro dinâmico está limitado a 128 clientes por processo, mas ainda precisa de proteção contra abuso para exposição pública. A aprovação depende de um terminal interativo em primeiro plano; não suporta serviço de fundo ou máquinas sem terminal.
- Metadados OAuth e testes locais **não comprovam** compatibilidade de cadastro e consentimento com o ChatGPT, transporte HTTPS, nem suporte no plano/workspace do proprietário. Não foram implementadas ferramentas de arquivos, Git ou shell.
- A primeira chamada real ao ChatGPT exige um meio de transporte compatível. A opção de [Túnel MCP Seguro da OpenAI](https://developers.openai.com/pt-BR/api/docs/guides/secure-mcp-tunnels) requer configuração e credenciais da Plataforma. Ela não instala ou configura o serviço automaticamente.

Referências externas: [autenticação de plug-ins](https://developers.openai.com/pt-BR/plugins/build/auth) e [especificação de autorização MCP](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).

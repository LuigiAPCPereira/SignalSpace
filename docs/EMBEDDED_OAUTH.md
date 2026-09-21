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

O estado integrado é criado por padrão em `$XDG_STATE_HOME/signalspace` ou, se essa variável não existir, em `~/.local/state/signalspace`. É possível configurar `SIGNALSPACE_STATE_DIR` com um caminho absoluto fora do repositório. O diretório deve pertencer ao usuário e ter permissões `0700`; `identity.json` e `.lock` exigem `0600`. **O arquivo `identity.json` contém a chave RSA privada em texto codificado, não criptografado**: nunca publique, sincronize em repositórios ou compartilhe essa pasta. Proteger o diretório e os backups é responsabilidade do sistema operacional e do proprietário.

O processo recusa symlinks, dados corrompidos, permissões inseguras e uma segunda instância usando a mesma pasta. Se a URL HTTPS pública mudar, o estado anterior é rejeitado, não migrado nem substituído automaticamente. Não apague `identity.json` para contornar erros sem compreender que isso invalida os tokens existentes e obriga a registrar os clientes novamente.

A URL acima é **fictícia**: não conecta o ChatGPT. Ela deve ser substituída por uma URL HTTPS real quando houver um transporte aprovado e testado. O serviço permanece escutando somente em `127.0.0.1:7676`; o proxy/túnel deverá preservar o cabeçalho `Host`. Não definir `SIGNALSPACE_OAUTH_ISSUER`, `SIGNALSPACE_JWKS_URL`, `SIGNALSPACE_OAUTH_OWNER_SUBJECT` nem `SIGNALSPACE_LOCAL_TOKEN` neste modo. Configuração misturada falha na inicialização.

A descoberta é publicada em `/.well-known/oauth-protected-resource` e `/.well-known/oauth-authorization-server`. O servidor oferece **DCR** em `/register`, código de autorização com PKCE S256 em `/authorize`, troca em `/token` e chave pública em `/oauth/jwks`. O único escopo é `signalspace:diagnostic`.

O registro aceita apenas URIs de retorno HTTPS cujo host seja exatamente `chatgpt.com`, sem porta ou parâmetros de consulta. Mesmo um cliente que se identifique como ChatGPT não é confiável só por causa de seu nome. O usuário deve conferir o destino exibido e aprovar cada solicitação no terminal.

## Aprovar uma conexão

1. O cliente inicia o OAuth com `resource` igual à URL exata do MCP, código de autorização e PKCE S256. O navegador exibe o nome declarado pelo cliente, o destino de retorno e um identificador aleatório de solicitação.
2. Na **mesma janela do terminal** em que o SignalSpace está rodando, o proprietário digita `approve IDENTIFICADOR` ou `deny IDENTIFICADOR`. Não existe rota HTTP para aprovar a conexão.
3. Após a aprovação, o proprietário clica em **Continuar** no navegador. O SignalSpace confere cookie HttpOnly/Secure e token CSRF, devolve um código de uso único ao retorno registrado e exige PKCE, `client_id`, `redirect_uri` e `resource` exatos na troca.
4. O token JWT RS256 emitido dura 15 minutos e é verificado pelo próprio servidor MCP antes da ferramenta de diagnóstico. A recusa ou expiração impede a emissão. O identificador da solicitação não é uma credencial.

## Limites ainda abertos

- **Persistem entre reinicializações:** chave de assinatura RSA e clientes DCR, vinculados à mesma URL pública. Tokens existentes podem continuar válidos até expirar (15 minutos). **Não persistem:** autorizações pendentes, códigos de uso único ou decisões ainda não concluídas. Não existe refresh token, revogação nem rotação de chave; depois de expirar o access token será necessária nova autorização. A reconexão automática do ChatGPT ainda não foi testada.
- Há cotas globais por janela de um minuto: 16 registros, 64 solicitações `/authorize`, 128 conclusões e 128 trocas em `/token`. Requisições excedentes retornam `429` e `Retry-After`. O IP de origem do túnel não é utilizado como identidade. Essas cotas reduzem abuso, mas também permitem negação de serviço temporária e **não são suficientes para publicar o serviço na internet**. O máximo é 128 clientes por estado; não há interface de limpeza ou revogação. A aprovação depende de um terminal interativo em primeiro plano, sem suporte a serviço de fundo.
- Metadados OAuth e testes locais **não comprovam** compatibilidade de cadastro e consentimento com o ChatGPT, transporte HTTPS, nem suporte no plano/workspace do proprietário. Não foram implementadas ferramentas de arquivos, Git ou shell.
- A primeira chamada real ao ChatGPT exige um meio de transporte compatível. A opção de [Túnel MCP Seguro da OpenAI](https://developers.openai.com/pt-BR/api/docs/guides/secure-mcp-tunnels) requer configuração e credenciais da Plataforma. Ela não instala ou configura o serviço automaticamente.

Referências externas: [autenticação de plug-ins](https://developers.openai.com/pt-BR/plugins/build/auth) e [especificação de autorização MCP](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization).

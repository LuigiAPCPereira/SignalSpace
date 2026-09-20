# Quick Tunnel: conexão experimental guiada

**Estado observado em 19/09/2026:** `connect quick` passou no preflight HTTPS na máquina do proprietário e apresentou um endpoint ao ChatGPT Web. O primeiro cadastro OAuth real foi rejeitado com `400 invalid_client_metadata`; a correção de compatibilidade DCR descrita abaixo ainda depende de novo teste real. Nenhuma autorização ou chamada MCP do ChatGPT foi comprovada. Apenas `connection_diagnostic` está disponível; não há arquivos, Git ou terminal.

O Cloudflare Quick Tunnel cria uma URL aleatória `https://...trycloudflare.com`, gratuitamente e sem conta ou domínio. É uma opção **exclusivamente para testes**; não oferece SLA, não suporta SSE e pode limitar requisições. O SignalSpace usa o transporte MCP de respostas HTTP JSON nesta fase. Consulte a [documentação oficial de Quick Tunnels](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/).

## Pré-requisitos

- Linux e Go 1.23+ nesta implementação experimental.
- `cloudflared` instalado pelo método oficial para sua distribuição, disponível no `PATH`. Confira com `cloudflared --version`. O SignalSpace **não baixa executáveis, não eleva privilégios e não instala dependências automaticamente**.
- A porta local `127.0.0.1:7676` deve estar livre. Se já houver um SignalSpace ou outro serviço nessa porta, finalize-o conscientemente antes de continuar.
- Não mantenha um arquivo `~/.cloudflared/config.yaml` que conflite com o Quick Tunnel; a Cloudflare informa que essa modalidade não funciona com determinadas configurações existentes. Não renomeie configurações de outros serviços automaticamente.
- As variáveis `SIGNALSPACE_AUTH_MODE`, `SIGNALSPACE_RESOURCE_URL`, `SIGNALSPACE_OAUTH_ISSUER`, `SIGNALSPACE_JWKS_URL`, `SIGNALSPACE_OAUTH_OWNER_SUBJECT`, `SIGNALSPACE_LOCAL_TOKEN` e `SIGNALSPACE_STATE_DIR` devem estar ausentes. O comando **recusa misturar** credenciais ou estado permanentes com esta sessão.

Enquanto a PR não for mesclada, execute a branch de desenvolvimento:

```bash
git clone --branch feat/m1-local-mcp-diagnostic https://github.com/LuigiAPCPereira/SignalSpace.git
cd SignalSpace
go test ./...
go run ./cmd/signalspace connect quick
```

O SignalSpace explica que o túnel publicará um endereço acessível pela internet e solicita que você digite **`PUBLICAR`**. Qualquer outra resposta cancela a ação sem iniciar o túnel. Após a confirmação, o SignalSpace:

1. Reserva exclusivamente `127.0.0.1:7676` para não publicar um processo antigo por acidente.
2. Inicia um processo filho `cloudflared tunnel --url http://127.0.0.1:7676`, sem shell, recupera apenas uma URL `https://<subdomínio>.trycloudflare.com` e não retransmite os logs brutos do túnel.
3. Cria uma identidade OAuth e registros de cliente **isolados e descartáveis**, em pasta temporária privada. Não reutiliza `$XDG_STATE_HOME/signalspace` nem altera o estado permanente.
4. Inicia MCP e OAuth no loopback, acessíveis pelo mesmo hostname externo, e valida via HTTPS os metadados, o certificado, as chaves públicas e os desafios de autenticação, sem tokens. Se a verificação falhar, encerra o túnel e o servidor e **não apresenta a URL como pronta**.
5. Apresenta o endpoint `https://<subdomínio>.trycloudflare.com/mcp` para cadastrar no ChatGPT Web, caso o plano e a configuração permitam conectar um servidor MCP personalizado.

Quando o ChatGPT solicitar autorização, confira o nome declarado pelo cliente e a URL de retorno exibidos. Digite `approve IDENTIFICADOR` ou `deny IDENTIFICADOR` **no terminal que executa o SignalSpace**. O consentimento só prossegue após a decisão local. Não divulgue a URL a terceiros: a descoberta e a autorização são públicas e a identidade do proprietário depende de quem controla esse terminal.

## Cadastro OAuth (DCR) no ChatGPT

Após o diagnóstico HTTPS, crie a conexão com a URL **nova** da sessão, terminada em `/mcp`. O primeiro teste real chegou à etapa de registro dinâmico, mas o servidor rejeitou metadados do cliente. O servidor agora trata `client_name` como opcional (exibe "Cliente sem nome informado" em vez de se passar pelo ChatGPT) e negocia uma solicitação de `authorization_code` + `refresh_token` respondendo **apenas `authorization_code`**. Não emite nem anuncia refresh tokens. Métodos de autenticação do cliente diferentes de `none` e callbacks fora de `chatgpt.com` continuam proibidos.

Se o cadastro falhar de novo, o terminal exibirá `OAuth client registration rejected: CATEGORIA` sem corpo da requisição, URLs, tokens ou outros metadados. Envie **somente essa categoria e a mensagem de erro do ChatGPT**, sem enviar credenciais. Categorias incluem `invalid_client_name`, `invalid_redirect_uris`, `unsupported_grant_types`, `unsupported_token_auth_method`, `unsupported_response_types` e `redirect_uri_not_allowed`. Não é necessário repetir os testes de DNS ou trocar a configuração do sistema.

Como o Quick Tunnel é efêmero, ao reiniciar o comando será necessário **remover ou atualizar a tentativa de conexão antiga** e cadastrar a URL recém-gerada; IDs de clientes anteriores não sobrevivem à nova sessão.

## DNS temporariamente indisponível

O teste real observou `registered=true` e `connection_errors=0`, mas o novo hostname continuou retornando `no such host` mesmo após 60 segundos. A Cloudflare documenta que um registro recém-criado pode ficar invisível por **cache negativo (NXDOMAIN)** se for consultado cedo demais; a causa específica ainda não está comprovada. Agora, após a URL, o SignalSpace aguarda brevemente um registro de conexão do processo e mais três segundos **antes da primeira consulta DNS local**. Depois, espera **até 60 segundos** e repete apenas falhas DNS antes de encerrar a sessão. Erros de TLS, metadados e autenticação continuam interrompendo imediatamente. A espera não altera DNS do sistema, não desliga a verificação TLS e não gera outro túnel automaticamente. Se a falha persistir, o SignalSpace consulta **somente para diagnóstico** o Google Public DNS por HTTPS (DoH) e inclui `public_dns=resolved`, `nxdomain`, `no_a_record` ou `unavailable` no erro. Essa consulta não é usada para conectar, substituir o DNS local, dispensar o teste HTTPS nem aprovar a URL. `public_dns=resolved` com `no such host` local sugere diferença entre os resolvedores; `nxdomain` demonstra resposta negativa no resolvedor consultado, mas não prova ausência no DNS autoritativo. Se a falha persistir, a URL não será apresentada como pronta. Documentação sobre cache negativo: https://developers.cloudflare.com/dns/troubleshooting/dns-issues/ e API usada: https://developers.google.com/speed/public-dns/docs/doh/json.

Para inspecionar o resolvedor no Linux, use o hostname citado no erro anterior (sem `https://` e sem `/mcp`):

```bash
getent ahosts HOSTNAME.trycloudflare.com
```

Se não houver resposta, verifique se o DNS e a rede local conseguem resolver outros domínios. Não altere globalmente o DNS ou cole tokens para contornar uma falha não diagnosticada. No shell **fish**, se houver variáveis SignalSpace antigas, apague-as com `set -e`, por exemplo:

```fish
set -e SIGNALSPACE_AUTH_MODE SIGNALSPACE_RESOURCE_URL SIGNALSPACE_OAUTH_ISSUER SIGNALSPACE_JWKS_URL SIGNALSPACE_OAUTH_OWNER_SUBJECT SIGNALSPACE_LOCAL_TOKEN SIGNALSPACE_STATE_DIR
```

## Encerrar e limitações

Pressione **Ctrl+C**. O SignalSpace fecha o servidor, encerra e aguarda o processo `cloudflared`, libera o bloqueio da identidade e remove a pasta temporária numa saída normal. Um desligamento abrupto pode deixar arquivos temporários privados em disco; investigue antes de excluí-los. Não apaga dados de outros túneis.

Uma nova execução gera outra URL, chave e registro de cliente. **Será necessário atualizar ou recriar a conexão no ChatGPT**; Quick Tunnel não oferece URL estável. A expiração dos tokens de 15 minutos, a ausência de refresh/revogação e a compatibilidade real com o fluxo OAuth do ChatGPT seguem limitações não resolvidas. A aprovação no `doctor transport` não demonstra que o ChatGPT invocou uma ferramenta.

**Proibido neste modo experimental:** compartilhar chaves OAuth; ativar ferramentas de shell, Git ou arquivos; tratar esta conexão como uma implantação de produção ou sem riscos. Esta documentação descreve um teste autorizado pelo usuário, não uma garantia de segurança para exposição contínua.

## Diagnóstico seguro de conexão do cloudflared

Quando o DNS ou o HTTPS falha, `connect quick` apresenta contadores derivados das mensagens de conexão do filho: `registrations`, `registered`, `disconnections`, `connection_errors` e `log_read_errors`. **A URL impressa não comprova registro de conexão.** Esses sinais são observações dos logs, não uma confirmação da Cloudflare nem prova de que o ChatGPT conectou. Os logs brutos, cabeçalhos e segredos não são exibidos nem persistidos pelo SignalSpace.

- `registrations=0`: não apareceu o marcador conhecido de conexão registrada; não conclui que nunca houve conexão se o formato dos logs mudou.
- `registrations>0` e DNS ainda indisponível: o processo anunciou pelo menos uma conexão, mas a publicação/resolução do endereço ainda precisa de investigação independente.
- `connection_errors>0`, `disconnections>0` ou `log_read_errors>0`: existe evidência adicional do processo, sem revelar o conteúdo completo dos logs.

Não aumente a janela de DNS repetidamente nem desabilite a verificação TLS. A causa do `no such host` no ambiente real ainda não foi estabelecida.

# Quick Tunnel: conexão experimental guiada

**Estado:** o comando `connect quick` automatiza o processo `cloudflared` e testa o HTTPS público, mas **não foi executado na máquina do proprietário nem vinculado ao ChatGPT Web**. Apenas `connection_diagnostic` está disponível. Não há ferramentas de arquivos, Git ou terminal.

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

## Encerrar e limitações

Pressione **Ctrl+C**. O SignalSpace fecha o servidor, encerra e aguarda o processo `cloudflared`, libera o bloqueio da identidade e remove a pasta temporária numa saída normal. Um desligamento abrupto pode deixar arquivos temporários privados em disco; investigue antes de excluí-los. Não apaga dados de outros túneis.

Uma nova execução gera outra URL, chave e registro de cliente. **Será necessário atualizar ou recriar a conexão no ChatGPT**; Quick Tunnel não oferece URL estável. A expiração dos tokens de 15 minutos, a ausência de refresh/revogação e a compatibilidade real com o fluxo OAuth do ChatGPT seguem limitações não resolvidas. A aprovação no `doctor transport` não demonstra que o ChatGPT invocou uma ferramenta.

**Proibido neste modo experimental:** compartilhar chaves OAuth; ativar ferramentas de shell, Git ou arquivos; tratar esta conexão como uma implantação de produção ou sem riscos. Esta documentação descreve um teste autorizado pelo usuário, não uma garantia de segurança para exposição contínua.

# SignalSpace

**Give ChatGPT a secure connection to your own machine and turn ChatGPT into Codex.**

SignalSpace é um projeto independente em **Go** para conectar ferramentas de desenvolvimento da máquina do usuário ao **ChatGPT Web** via MCP HTTPS autenticado. **Ainda está em desenvolvimento.** A única ferramenta implementada é `connection_diagnostic`; não há leitura/edição de arquivos, Git ou terminal, e nenhuma chamada real do ChatGPT Web foi validada.

## Primeira experiência: Quick Tunnel experimental

O comando `connect quick` usa o `cloudflared` instalado para criar um túnel gratuito **sem conta nem domínio Cloudflare**, com uma URL temporária. Ele só inicia após a confirmação `PUBLICAR`, usa credenciais OAuth isoladas em diretório temporário, verifica a conexão HTTPS pública e apresenta a URL MCP somente após passar no diagnóstico. Ao encerrar normalmente, fecha o processo filho e elimina o estado temporário. **Expor esse diagnóstico à internet ainda é experimental, não um serviço de produção.**

Com Go 1.23+ e `cloudflared` instalado e disponível no `PATH`, na branch desta PR:

```bash
go test ./...
go run ./cmd/signalspace connect quick
```

Leia [passo a passo, consentimento e limites do Quick Tunnel](docs/QUICK_TUNNEL.md). O SignalSpace não instala executáveis automaticamente. A URL `trycloudflare.com` muda entre sessões; é necessário atualizar/recriar o conector no ChatGPT. O ChatGPT precisa permitir adicionar um servidor MCP personalizado, algo ainda não testado com a conta do proprietário.

## Diagnóstico local, sem publicar

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(openssl rand -hex 32)"
go test ./...
go vet ./...
go run ./cmd/signalspace
```

O endpoint `http://127.0.0.1:7676/mcp` oferece somente `connection_diagnostic` com bearer local. Não é uma conexão funcional do ChatGPT Web. Veja [diagnóstico local](docs/LOCAL_DIAGNOSTIC.md).

## OAuth integrado e transporte persistente

O SignalSpace pode emitir tokens OAuth sem Auth0, com PKCE S256, registro dinâmico do cliente, aprovação local no terminal e verificação JWT. No modo integrado **persistente**, chave privada RSA e clientes ficam em diretório próprio com permissões restritas, vinculados à URL HTTPS exata; autorizações pendentes e códigos não persistem. O arquivo da chave não tem criptografia em repouso. Faltam refresh, revogação, revisão de segurança para exposição contínua e validação pelo ChatGPT. Não confundir esse modo com o Quick Tunnel descartável. Consulte [autorização integrada](docs/EMBEDDED_OAUTH.md) e [diagnóstico e configuração HTTPS](docs/TRANSPORT.md).

O uso de provedor OAuth externo é opcional, sem fallback para bearer local. Veja [servidor de recursos OAuth](docs/OAUTH_RESOURCE_SERVER.md), [diagnóstico do provedor](docs/OAUTH_PREFLIGHT.md) e [Auth0 opcional](docs/AUTH0_INTEGRATION.md).

## Produto e segurança

Fluxo desejado: iniciar serviço → estabelecer HTTPS → conectar e autorizar ChatGPT Web → abrir workspace aprovado → inspecionar código → editar → executar testes/comandos → revisar alterações. Workspaces, leitura/edição, terminal, Git, diffs e continuidade entre conversas são trabalhos futuros. Não planejamos frontend substituto do ChatGPT, IA própria, agent-runtime, agent-orchestrator, fork/dependência do DevSpace ou subagentes nesta fase.

O processo escuta somente em loopback, embora o túnel permita acesso público explicitamente autorizado. Nenhuma ferramenta poderosa é exposta enquanto o vínculo do proprietário não for validado. **Quando implementado, o shell terá os privilégios do usuário local; uma allowlist de arquivos não é sandbox.** O suporte ao plano do ChatGPT só será afirmado após uma chamada real à ferramenta.

## Documentação

- [Produto](docs/PRODUCT.md) e [MVP](docs/MVP.md)
- [Quick Tunnel guiado](docs/QUICK_TUNNEL.md)
- [Transporte HTTPS](docs/TRANSPORT.md)
- [Autorização OAuth integrada](docs/EMBEDDED_OAUTH.md)
- [Instruções para agentes](AGENTS.md)

Projeto original; aprendizados conceituais de [DevSpace](https://github.com/Waishnav/devspace), sem incorporar seu código.

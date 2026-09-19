# Transporte HTTPS: preparação e diagnóstico (M1)

**Estado:** o comando de diagnóstico está implementado e testado com um servidor HTTPS de teste. **Não há túnel configurado na máquina do proprietário nem chamada real observada no ChatGPT Web. Não publique o serviço nesta versão experimental.** O único método MCP existente continua sendo `connection_diagnostic`; não existem ferramentas de arquivos, Git ou terminal.

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

Essa é **uma opção**, não uma dependência obrigatória. O Cloudflare Tunnel precisa de conta Cloudflare, domínio sob gerenciamento do serviço, instalação de `cloudflared` e uma rota DNS para um hostname estável. Quick Tunnels em `trycloudflare.com` recebem uma URL aleatória e não são apropriados para a identidade OAuth persistente. Consulte a [documentação oficial do Tunnel](https://developers.cloudflare.com/tunnel/get-started/) e do [túnel gerenciado localmente](https://developers.cloudflare.com/tunnel/features/locally-managed-tunnels/create-local-tunnel/).

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

A documentação da Cloudflare descreve `cloudflared tunnel login`, criação do túnel nomeado, roteamento DNS, `cloudflared tunnel ingress validate` e execução do túnel. **Estes passos publicam um serviço acessível na internet; não execute a criação da rota pública nem a inicialização do túnel até a revisão de segurança do modo experimental.** O SignalSpace não instala nem executa `cloudflared` automaticamente e não abre portas de entrada.

## O que falta para marcar a conexão real como validada

Com o transporte e a conta do usuário disponíveis, verificar no ChatGPT Web: descoberta MCP, DCR do cliente correto, consentimento na página HTTPS, aprovação **no terminal local**, retorno `iss`/`state`, troca com PKCE e token `aud` correto e, finalmente, chamada à ferramenta `connection_diagnostic` autenticada. Registrar apenas o resultado, sem tokens ou códigos. Revogação, refresh, proteção contra abuso público e uma experiência de instalação confiável continuam pendentes. Nenhum teste local, incluindo `doctor transport`, substitui essa evidência.

O [Túnel MCP Seguro da OpenAI](https://developers.openai.com/pt-BR/api/docs/guides/secure-mcp-tunnels) é outra possibilidade: pode manter o MCP privado, mas o servidor de autorização OAuth precisa ser acessível pelo navegador, pois o túnel não o publica automaticamente. É necessária configuração separada na Plataforma e autorização do workspace.

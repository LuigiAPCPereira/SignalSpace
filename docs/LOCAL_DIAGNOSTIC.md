# Diagnóstico MCP local — primeira fatia

**Status:** implementação experimental local. O teste de integração com ChatGPT Web não foi executado; este documento não descreve um conector ChatGPT utilizável.

## Executar

Requisito: Node.js 22 ou superior. Nenhuma dependência de terceiros é instalada nesta fatia.

```bash
export SIGNALSPACE_LOCAL_TOKEN="$(node -e "console.log(require('node:crypto').randomBytes(32).toString('hex'))")"
npm run check
npm test
npm start
```

O serviço escuta **somente `127.0.0.1:7676`**. O segredo é lido da variável `SIGNALSPACE_LOCAL_TOKEN` e deve ter pelo menos 32 caracteres, sem espaços. Não copie o token para arquivos versionados ou logs. O servidor não inicia sem token válido.

## Contrato implementado

- `POST /mcp` com JSON-RPC 2.0 e protocolo MCP `2025-06-18`.
- `initialize`, `ping`, `tools/list`, `tools/call` para a ferramenta `connection_diagnostic`.
- `Authorization: Bearer` obrigatório antes do processamento de ferramentas, com comparação de bytes em tempo constante quando o comprimento coincide.
- Host e Origin limitados ao serviço local, corpo de requisição limitado a 64 KiB e tempo de requisição limitado.
- Nenhuma ferramenta de arquivo, edição, Git ou shell; nenhuma UI ou processo persistente.

O resultado `connected: true` prova somente que a chamada local autenticada foi concluída. O campo `chatgptVerified: false` é intencional.

## O que não fazer

**Não exponha esta versão por Cloudflare Tunnel, ngrok, Funnel ou qualquer proxy público.** Ainda não existem fluxo OAuth, descoberta de recurso protegido, aprovação do proprietário nem validação de compatibilidade com o ChatGPT Web. O bearer de diagnóstico local não substitui OAuth e não deve ser configurado como uma autorização remota de produção.

## Próxima fatia

Escolher e integrar um provedor/fluxo OAuth compatível com a especificação MCP e com o ChatGPT; fornecer metadados de descoberta, consentimento, verificação de tokens por emissor/destinatário/expiração/escopo e somente então admitir Host público configurado com túnel HTTPS. Provar localmente a negação sem autorização e obter uma chamada real no ChatGPT Web antes de afirmar que a conexão externa funciona.

A documentação oficial indica que clientes ChatGPT usam OAuth 2.1 e descoberta de recurso protegido: https://developers.openai.com/plugins/build/auth . Os recursos efetivos de cada configuração de ChatGPT continuam sujeitos a verificação prática.

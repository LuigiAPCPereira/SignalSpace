# SignalSpace — contrato inicial do produto

## Problema

O ChatGPT Web não possui, automaticamente, acesso ao checkout, ao terminal e aos testes da máquina do usuário. Conversas longas e processos demorados também podem perder contexto ou a resposta. O usuário quer trabalhar no ChatGPT Web com capacidades de um agente de programação, usando seu próprio ambiente.

## Resultado desejado

> Give ChatGPT a secure connection to your own machine and turn ChatGPT into Codex.

Uma aplicação independente executa localmente e disponibiliza ferramentas via MCP através de um endpoint HTTPS autenticado. O usuário concede acesso a projetos específicos. O ChatGPT pode inspecionar arquivos, aplicar mudanças, executar comandos e revisar diffs dentro de um fluxo que o usuário controla.

## Fluxo primário

1. Usuário inicia o serviço local e configura o túnel HTTPS.
2. ChatGPT Web descobre e conecta ao endpoint MCP.
3. Usuário autoriza a conexão.
4. ChatGPT abre um projeto dentro de uma raiz autorizada e recebe um identificador explícito de workspace.
5. ChatGPT inspeciona arquivos e instruções do projeto, propõe/aplica uma mudança e executa um teste ou comando.
6. Ferramentas devolvem resultados reais, erros distintos e um resumo verificável das alterações.

## Responsabilidades

- **ChatGPT Web:** interface, raciocínio e decisão sobre uso de ferramentas; não é serviço de execução local.
- **Transporte e autenticação:** conexão externa, identidade do cliente e autorização; não acessam filesystem diretamente.
- **Workspace:** autorização de raízes, identidade do projeto e resolução de caminhos de ferramentas de arquivo.
- **Ferramentas:** operações de leitura, edição, processos e Git, com efeitos observáveis.
- **Estado operacional (posterior):** identidade de operações longas, resultados, reconciliação e retomada após desconexão.

## Limites assumidos

- Não depender de `agent-runtime` ou `agent-orchestrator` nesta fase.
- Não fazer fork, incorporar ou depender do DevSpace.
- Não construir subagentes, inferência própria, Graphify embutido ou frontend alternativo como condição de MVP.
- O shell poderá rodar sob a conta do usuário. Raízes autorizadas de arquivos **não confinam o shell**. Isso deve ser informado, sem alegar isolamento que não existe.
- Túnel HTTPS e autenticação são obrigatórios antes de oferecer ferramentas perigosas fora do loopback.
- Planos e variantes de modelo do ChatGPT são compatibilidade **a verificar** via chamadas reais. Um aplicativo aparecer na interface não comprova que suas ferramentas foram disponibilizadas.

## Progresso e qualidade

O primeiro marco é uma chamada real do ChatGPT Web que lê, edita e executa em um workspace autorizado. Só depois avançar para operação durável e continuidade entre conversas. Testes e documentação não substituem a prova do fluxo real.

## Referências conceituais

- DevSpace: demonstra conexão de ChatGPT a ambiente local por MCP + túnel, sem compartilhar código com este projeto.
- Graphify: candidato opcional futuro para compreensão estrutural de repositórios.
- Engineering DNA do CODE: observar realidade, preservar desconhecidos, ownership claro e menor solução suficiente.

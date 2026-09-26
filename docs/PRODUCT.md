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


## Direção aprovada — ChatGPT Web como coding agent completo

O objetivo de produto é que o ChatGPT Web consiga atuar, dentro de um workspace explicitamente autorizado, como um coding agent completo: compreender o projeto, navegar e pesquisar, criar e modificar a árvore de arquivos, revisar e operar Git por capacidades tipadas, executar ferramentas autorizadas e apresentar resultados verificáveis. O SignalSpace deve oferecer primitivas próprias e estruturadas em vez de depender de shell para operações normais de filesystem, busca ou Git.

**Filesystem de primeira classe:** a superfície-alvo cobre inspeção e busca (`stat_path`, `list_directory`, `read_file`, `find_paths`, `search_text`), criação e edição (`create_directory`, `create_text_file`, `replace_text`, `write_text_file`), operações estruturais (`copy_path`, `move_path`) e remoção explícita (`delete_file`, `delete_directory`). `apply_patch` é uma composição estruturada em implementação local sobre o mesmo motor seguro, não um segundo caminho de autorização; sua publicação e aceite operacional permanecem separados. Transferência de binários/artifacts permanece uma superfície separada.

**Git de primeira classe:** `git.review` continua sendo a capacidade observacional existente. A direção aprovada inclui Git local tipado para status/diff/log/show/branches e, em etapa própria, stage/unstage, criação/troca de branch e commit. Operações remotas como fetch/push pertencem a uma fronteira separada de rede/credenciais. Operações destrutivas como reset hard, clean, remoção agressiva de branch e force-push não entram implicitamente no Git normal. Mutações Git não devem ganhar execução arbitrária por hooks/configuração: o contrato deve neutralizar hooks e ambiente/configuração executável antes de promovê-las.

**UX por domínio/tool:** cada tool deve ter resultado estruturado e identidade visual coerente com sua função. UI especializada é desejável quando melhora compreensão ou revisão — por exemplo workspace, árvore/busca, diff Git, testes/processos e artifacts — sem exigir um iframe diferente para cada chamada trivial. A tool continua funcional para hosts MCP sem UI especializada; apresentação não substitui o contrato nem a autorização.

DevSpace e Graphify permanecem referências conceituais: o primeiro inspira o fluxo de coding agent conectado ao ambiente local; o segundo inspira compreensão estrutural opcional. O SignalSpace não incorpora código desses projetos nem delega a eles sua fronteira de segurança. Shell continua capacidade de alto risco separada e não é autorizado por esta direção de filesystem/Git.

## Contrato de aprovações e interface de autenticação

A interface de autenticação do SignalSpace cuida **da conexão OAuth e dos escopos solicitados**, não da confirmação de cada chamada MCP. A confirmação de uso de ferramentas que o ChatGPT apresentar pertence à interface do ChatGPT. Sua disponibilidade e suas opções de aprovação podem variar; o backend do SignalSpace **não depende de receber um sinal de aprovação do ChatGPT** para autorizar uma chamada.

- **OAuth:** apresentar ao proprietário o nome declarado pelo cliente (não atestado), o `client_id`, o destino de retorno e os escopos pedidos; distinguir diagnóstico de leitura. Aprovar ou negar a conexão e uma ampliação de escopo permanece um ato explícito e verificável no SignalSpace. A página pública `/authorize` não pode, sozinha, transformar um clique remoto em aprovação do proprietário. No fluxo implementado, a decisão confiável ocorre no terminal local e o navegador conclui o OAuth depois disso. Uma mudança futura dessa experiência deve preservar uma fronteira confiável equivalente, não remover a verificação.
- **Workspace:** conceder uma raiz exata a um `client_id` específico por ação local explícita, informar a sessão e permitir revogação. A aprovação de workspace não é aprovação de ferramenta individual; não há concessão implícita pelo OAuth, pelo nome do aplicativo ou por uma confirmação do ChatGPT. Uma mudança para edição, Git ou shell exigirá contrato e controles próprios, sem aproveitar automaticamente a concessão atual de leitura.
- **Cada requisição MCP:** o backend deve verificar token, identidade, escopo, cliente, sessão, concessão ainda ativa, caminho e limites pertinentes. Negar mesmo que uma confirmação tenha aparecido no ChatGPT, quando faltar qualquer requisito. Uma concessão revogada não pode continuar autorizando leitura só porque o JWT ainda não expirou.
- **Superfícies permitidas:** página de consentimento OAuth clara, indicação das permissões efetivas e do estado de conexão/concessão quando houver dados verificáveis, e acesso à revogação pela fronteira local confiável. Não expor caminhos privados nem tokens numa página pública. Distinguir revogação imediata da concessão de workspace da expiração do token OAuth: o protótipo ainda não oferece revogação de JWT emitido.
- **Fora do escopo da interface própria:** painel ou fila de chamadas MCP aguardando aprovação, botões de aprovação por operação, modos próprios de “aprovar sempre”, duplicação do histórico de confirmações do ChatGPT ou um fluxo de confirmação paralelo para cada `read_file`.

**Critérios de aceitação:** uma chamada autorizada não deve solicitar aprovação adicional do SignalSpace por operação; chamada sem OAuth/escopo/concessão ou após revogação deve falhar no backend independentemente da UI do ChatGPT; a página OAuth deve distinguir claramente o pedido de leitura do diagnóstico; nenhuma tela pode afirmar que o software cliente é o ChatGPT só com base em nome ou `client_id`.

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

## Direção arquitetural aceita — autorização local v2

`SS-MVP-002-LOCAL-AUTHORIZATION-V2-DESIGN-001` aceita como alvo a separação entre **OAuth por composição** e **Autorização Local de Capabilities**. OAuth autentica a conexão; não concede workspace, capability ou shell. A política local deve considerar proprietário, cliente, workspace, sessão, capability, tool e precondições, retornando `ALLOW`, `DENY` ou `REQUIRE_APPROVAL`. Aprovação local de operação sensível não executa automaticamente a chamada original; o cliente deve repetir e o backend revalidar tudo.

O desenho futuro propõe `signalspace:diagnostic`, `signalspace:read` e `signalspace:programming` como escopos de composição. `workspace.read`, `workspace.write`, `workspace.delete`, `git.review`, `git.index`, `git.commit`, `test.run` e `shell.exec` continuam dimensões internas independentes. O contrato detalhado está em [`ADR_LOCAL_AUTHORIZATION_V2.md`](ADR_LOCAL_AUTHORIZATION_V2.md) e está **ACEITO / NÃO IMPLEMENTADO**; a composição e os escopos atualmente publicados não mudam por este registro.

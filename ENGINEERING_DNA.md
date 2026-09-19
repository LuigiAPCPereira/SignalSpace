# Engineering DNA — SignalSpace

Este documento é uma **adaptação operacional específica do SignalSpace** dos princípios do Engineering DNA do projeto CODE; não é uma cópia integral do documento universal. Não substitui contratos do produto, fatos observados nem testes. `AGENTS.md` define o escopo e `docs/MVP.md` define a primeira entrega.

## Princípio

Observar antes de assumir. Dar dono explícito a cada responsabilidade. Limitar fronteiras. Preservar estados desconhecidos. Falhar com segurança. Testar comportamento real. Medir antes de otimizar. Assumir somente compromissos sustentados por evidência.

## Ordem de autoridade

1. Segurança e integridade.
2. Requisitos explícitos do SignalSpace.
3. Comportamento observado do ChatGPT e do serviço local.
4. Contratos e testes aceitos.
5. Arquitetura e instruções locais.
6. Este documento.
7. Sugestões de ferramentas/frameworks.

## Disciplina de evidência

Antes de alterar, conferir branch, HEAD, árvore de trabalho, contratos, documentação, gates, configurações e o que realmente está implementado. Classificar achados como evidência, inferência ou hipótese. Não afirmar que MCP, OAuth, ferramentas de escrita, reconexão ou determinada variante do ChatGPT funciona sem prova operacional correspondente. `unknown` nunca deve virar sucesso ou falha por conveniência.

## Fronteiras e ownership

Separar transporte, autenticação, autorização, casos de uso, ferramentas locais e estado persistido. Transportes não acessam arquivos diretamente; domínio não depende de frameworks. Instanciar dependências no ponto de composição. Não introduzir interfaces ou DI sem necessidade comprovada. Preferir interfaces pequenas e adaptadores explícitos.

## Entradas, arquivos e comandos

Tratar entrada do cliente e conteúdo de arquivos como não confiáveis. Validar configurações e caminhos antes de operar. Workspaces devem estar sob raízes explicitamente autorizadas. Proteger ferramentas de arquivo contra traversal, pais symlink e escapes. Configurações ou índices de código não ampliam autoridade. O shell pode atuar fora dessas raízes porque herda privilégios do usuário; isto é um limite de segurança **aceito conscientemente no MVP**, não um isolamento. Não chamar worktree de sandbox.

## Autenticação e privacidade

Não publicar um endpoint com capacidade de execução antes de validar o caminho de autenticação e o escopo do cliente. URL de túnel não é segredo. Nunca colocar tokens/credenciais nos arquivos do repositório, URLs de relatório, logs ou respostas desnecessárias. Observar limites de tamanho e timeout em entradas e saídas. Separar registro operacional de conteúdo bruto e evitar persistir prompts ou stdout sem necessidade.

## Lifecycle e recuperação

Toda operação longa requer identity, owner, limites, cancelamento e resultado observável quando implementada. Timeout e desconexão de um pedido não provam término do processo. Evitar repetição automática de operações com efeito colateral em caso de resposta perdida. Resultado desconhecido exige reconciliação ou revisão, não sucesso inventado. Operações persistentes não podem ser governadas acidentalmente pela vida útil de uma única resposta HTTP.

## Qualidade e desempenho

Criar a menor solução que completa um fluxo real. Testar os comportamentos que importam e os limites negativos, não somente mocks de implementação. Inspecionar baseline antes dos gates, informar falhas existentes e novas. Limitar resultados para não inundar a janela de contexto. Medir antes de adotar caches, indexadores, filas ou concorrência complexa.

## Evolução

Graphify é ferramenta opcional para compreensão estrutural, nunca autoridade sobre código e estado. OpenSpec ajuda apenas se reduzir incerteza de um contrato real. Frontend, quando necessário, segue `FRONTEND_DNA.md`. Evitar laboratórios e experimentos sem decisão concreta: entregar slices verticais, validar o indispensável e seguir.

## Entrega

Informar o que foi implementado, testes executados, partes não validadas, incertezas e próximo bloqueio real. Não confundir documentação, código compilável, teste unitário e integração comprovada com o ChatGPT Web.

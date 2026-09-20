# Integração nativa com GitHub — proposta para estudo posterior

**Status:** proposta de funcionalidade futura; pesquisa técnica e implementação adiadas. **Registrado em:** 20/09/2026.

Este documento registra uma possibilidade de evolução do SignalSpace, não uma funcionalidade disponível, uma arquitetura aprovada nem um compromisso de entrega. O escopo e os contratos serão definidos somente depois de estudar o serviço externo e observar o funcionamento real do produto.

## 1. Intenção do produto

Permitir que agentes conectados ao MCP do **SignalSpace** consultem e, quando explicitamente autorizados, executem operações em repositórios GitHub por uma integração nativa do próprio SignalSpace. O objetivo é oferecer um ponto coerente para capacidades de desenvolvimento local e remoto, sem exigir que o agente use diretamente o conector GitHub do ChatGPT para os fluxos cobertos pela integração.

SignalSpace é um projeto independente: esta proposta não prevê copiar, incorporar ou depender do DevSpace. Também não prevê substituir o ChatGPT como interface ou criar um sistema genérico de plugins.

**Não confundir:** operações locais de Git no checkout e operações remotas na API do GitHub têm fronteiras, credenciais e falhas diferentes. Uma integração não deve ser presumida como consequência da outra.

## 2. Condição para retomar

Não iniciar a integração GitHub enquanto o fluxo principal do SignalSpace não estiver funcionando corretamente e validado em ambiente real. A retomada requer, no mínimo:

1. Conexão MCP autenticada e autorização verificadas com o ChatGPT Web, incluindo negativas de acesso.
2. Leitura em workspace autorizado, isolamento de caminhos, revogação e comportamento de falha verificados no cliente real.
3. Fluxo do MVP de edição delimitada, execução de teste e revisão de diff demonstrado em projeto descartável, com preservação de alterações preexistentes.
4. Revisão dos limites de segurança para operações com efeitos externos, erros, timeout e resultados desconhecidos.
5. Estabilidade suficiente dos contratos internos de identidade, workspace, ferramentas e composição para adicionar um adaptador externo sem misturar responsabilidades.

Esses itens são **critérios de entrada**, não afirmações de que já foram concluídos. O marco de produto continua definido em [PRODUCT.md](PRODUCT.md) e [MVP.md](MVP.md); esta proposta não muda suas prioridades.

## 3. Hipótese inicial de arquitetura — não aprovada

```text
Agente / ChatGPT
      |
      v
SignalSpace MCP (autenticação e ferramentas)
      |
      v
Casos de uso + autorização de capacidades
      |
      +--> ferramentas de workspace local
      |
      +--> adaptador GitHub (futuro) --> GitHub
```

O SignalSpace deve manter clara a responsabilidade por identidade, autorização, credenciais e resultado das operações. Dados externos seriam validados e normalizados no adaptador; componentes de apresentação e domínio não consumiriam payloads brutos do GitHub. Não implementar uma arquitetura multiprovedor ou abstração genérica antes de existir necessidade observada.

## 4. Questões que exigem análise e estudo

- **Modelo de integração:** avaliar GitHub App e alternativas oficiais, incluindo instalação por repositório, tokens de instalação versus ações em nome do usuário, limites e experiência de consentimento. Nenhuma opção foi escolhida nesta etapa.
- **Vínculo de identidades:** como relacionar proprietário do SignalSpace, cliente MCP, conta/organização GitHub, instalação e repositórios acessíveis sem assumir que `client_id` atesta a identidade do aplicativo ou de uma conversa.
- **Permissões mínimas:** quais escopos/permissões são indispensáveis para cada operação e como lidar com seleção de repositórios, revogação, rotação e expiração.
- **Capacidades prioritárias:** verificar demanda por consultar repositórios, PRs, diffs e checks antes de considerar criação de PR, comentários, reviews ou merge.
- **Aprovações:** distinguir consentimento OAuth/concessão do SignalSpace, permissão concedida no GitHub e confirmação de chamada da ferramenta pelo ChatGPT. O ChatGPT pode oferecer confirmação de ferramentas, mas isso não substitui autorização no servidor nem garante uma confirmação para toda chamada. Não criar painel redundante de aprovação por operação sem requisito independente demonstrado.
- **Execução e falhas:** timeout, rate limit, paginação, idempotência de escritas, resposta perdida após efeito remoto, concorrência e reconciliação de resultado desconhecido.
- **Segurança e privacidade:** armazenar segredos somente no backend, nunca no browser, logs ou relatórios; validar entradas e webhooks caso sejam necessários; respeitar permissões efetivas e impedir acesso entre proprietários/repositórios.
- **Operação independente:** definir como o SignalSpace mantém as funcionalidades locais quando GitHub não está configurado, está indisponível ou foi revogado.
- **Compatibilidade MCP:** verificar na versão real do cliente como as ferramentas e seus efeitos são anunciados e como o usuário vê as confirmações. Não inventar contrato de aprovação do ChatGPT.

## 5. Sequência sugerida quando os critérios forem cumpridos

1. **Discovery e especificação:** observar contratos oficiais, validar identidade/permissões, mapear ameaças e registrar casos de uso, estados e critérios de aceitação. Usar OpenSpec se reduzir ambiguidades de contrato e autorização.
2. **Experimento isolado de leitura:** conectar um repositório de teste e consultar um recurso remoto, com credenciais sintéticas nos testes e smoke real controlado. Nenhuma escrita nesta fatia.
3. **MCP de leitura:** disponibilizar uma ferramenta pequena, de contrato estável, com autorização por chamada, limites e erros distintos.
4. **Escrita, se houver necessidade real:** propor cada operação com seus requisitos próprios de autorização, idempotência, auditoria e reconciliação; não liberar merge ou ações destrutivas por padrão.
5. **Eventos, somente se necessários:** avaliar webhooks após demonstrar que polling/consulta pontual não satisfaz o fluxo. Não antecipar infraestrutura de sincronização.

Não definir nomes definitivos de ferramentas, endpoints, telas ou cronograma antes do estudo.

## 6. Fora do escopo desta documentação

- Implementar login no GitHub, criar GitHub App, solicitar permissões ou armazenar tokens.
- Expor ferramentas de GitHub no MCP ou executar operações em repositórios.
- Alterar OAuth, concessões de workspace ou o painel local de autenticação em desenvolvimento em outra frente.
- Substituir ou desinstalar o conector GitHub existente do ChatGPT antes de existir cobertura e validação equivalentes.
- Fazer merge, alterar `main` ou prometer suporte de produção.

## 7. Critério de conclusão desta proposta

A ideia estará registrada quando este documento estiver versionado em branch isolada. **Isso não conclui o discovery nem autoriza implementação.** A próxima ação técnica será reavaliar o estado do MVP, confirmar os critérios de entrada e realizar o estudo dos contratos reais do GitHub, só então decidir escopo e desenho da primeira fatia.

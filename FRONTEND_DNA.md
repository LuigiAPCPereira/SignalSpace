# Frontend DNA — SignalSpace

Adaptação operacional para o SignalSpace dos princípios de frontend do projeto CODE; não é reprodução integral do documento universal. A interface principal do produto é o **ChatGPT Web**. Uma interface local só deve existir quando realizar uma tarefa que o ChatGPT não possa executar adequadamente, como configurar raízes de workspace, conceder autorização ou visualizar estados operacionais indispensáveis.

## Princípios

Verdade antes de decoração; informação antes de componentes; estados antes de polimento; interação antes de animação; acessibilidade antes de novidade; composição antes de cards; desempenho antes de excesso visual.

## Ordem de decisão

Produto → segurança e privacidade → contratos reais → tarefa do usuário → hierarquia → acessibilidade → interação → responsividade → linguagem visual → frameworks.

## Regra de superfície

Cada tela deve justificar sua existência com uma tarefa concreta. Não duplicar a conversa do ChatGPT, a conta do usuário, status de runtime permanentemente visíveis, indicadores cosméticos de otimização ou dashboards sem ação relevante. Privilegiar fluxo compacto de autorização, configuração e diagnóstico apenas quando houver implementação real.

## Estados e dados

Exibir explicitamente carregando, vazio, parcial, indisponível, negado, expirado, operação em andamento, erro e estado desconhecido quando aplicáveis. Nunca fabricar sucesso, progresso, conexão ativa ou resultados para preencher layout. Fluxos destrutivos exigem comunicação proporcional ao efeito.

## Qualidade

Acesso por teclado, foco visível, contraste, rótulos e textos claros, tamanhos responsivos e estados de erro recuperáveis. Evitar iframes/widgets por chamada de ferramenta sem necessidade; não gerar ruído de UI a cada leitura de arquivo. Impeccable pode revisar estética, mas não redefine segurança, produto ou contratos.

**Neste estágio:** nenhuma UI própria foi implementada ou planejada como requisito do primeiro fluxo vertical. Reavaliar somente quando integração real demonstrar necessidade.

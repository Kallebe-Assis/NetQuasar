/** FAQ da tela Sobre — perguntas frequentes sobre o NetQuasar. */

export type AboutFaqCategory = "tecnico" | "como_funciona" | "funcoes" | "configurar";

export type AboutFaqItem = {
  q: string;
  a: string;
  cat: AboutFaqCategory;
};

export const ABOUT_FAQ_CATEGORIES: { id: AboutFaqCategory; label: string; blurb: string }[] = [
  { id: "tecnico", label: "Técnico", blurb: "Conceitos de rede, protocolos e arquitectura." },
  { id: "como_funciona", label: "Como funciona", blurb: "Fluxos de monitoramento, alertas, mapa e operação do dia a dia." },
  { id: "funcoes", label: "Quais funções tem", blurb: "Módulos, ecrãs e capacidades do NetQuasar." },
  { id: "configurar", label: "Como configurar", blurb: "Limiares, Telegram, backups, permissões e ajustes." },
];

export const ABOUT_FAQ: AboutFaqItem[] = [
  {
    q: "O que é o NetQuasar?",
    a: "O NetQuasar é uma plataforma NOC para ISPs: inventário, monitoramento (ping, SNMP e OLT), alertas, mapa óptico, integrações e ferramentas de diagnóstico numa interface web.",
    cat: "funcoes",
  },
  {
    q: "Para quem o NetQuasar foi feito?",
    a: "Para provedores de internet (ISP) e equipas de operação de rede (NOC) que precisam de visão unificada de equipamentos, OLTs, BNG, MikroTik e alertas.",
    cat: "funcoes",
  },
  {
    q: "Quem desenvolveu o NetQuasar?",
    a: "Kallebe Assis Nogueira. Portfólio: https://portfolio-kallebe-assis.vercel.app/",
    cat: "como_funciona",
  },
  {
    q: "Quando o software foi criado?",
    a: "O projecto inicial data de 11 de maio de 2026; o produto continua em evolução activa.",
    cat: "como_funciona",
  },
  {
    q: "O NetQuasar é open source?",
    a: "É um produto/projeto sob o controlo do desenvolvedor e da organização que o opera. Consulte a licença e política internas da sua instalação.",
    cat: "tecnico",
  },

  // --- Conceitos ---
  {
    q: "O que é OLT?",
    a: "OLT (Optical Line Terminal) é o equipamento central da rede óptico-passiva (GPON/EPON) no provedor. Fica tipicamente no POP e concentra as fibras das ONUs/ONTs dos clientes, gerindo as portas PON e a autenticação óptica.",
    cat: "tecnico",
  },
  {
    q: "O que é ONU / ONT?",
    a: "É o equipamento óptico no lado do cliente (ou na caixa de atendimento). Liga-se a uma porta PON da OLT; o NetQuasar usa esses dados para inventário, potência óptica e alertas de quantidade por PON.",
    cat: "tecnico",
  },
  {
    q: "O que é BNG?",
    a: "BNG (Broadband Network Gateway) é o equipamento que autentica e concentra sessões de assinantes (muitas vezes PPPoE ou sessões IP). No ISP, é onde se vê quantos clientes estão online e onde se detectam quedas massivas de sessão.",
    cat: "tecnico",
  },
  {
    q: "O que é ICMP e TCP no monitoramento?",
    a: "ICMP é o protocolo clássico do “ping”: o NetQuasar pergunta se o equipamento responde na rede. TCP é uma alternativa de teste de conectividade (abrir uma porta) usada quando o ICMP está bloqueado ou falha. Em conjunto, permitem saber se o equipamento está alcançável e estimar a latência.",
    cat: "tecnico",
  },
  {
    q: "O que é SNMP?",
    a: "SNMP é um protocolo padrão para consultar informações do equipamento (CPU, memória, temperatura, interfaces, módulos ópticos, etc.) sem precisar de aceder à interface web do aparelho. O NetQuasar usa-o para telemetria e alertas de limiar.",
    cat: "tecnico",
  },
  {
    q: "O que precisa estar configurado no equipamento para coletar telemetria por SNMP?",
    a: "Em geral: (1) SNMP activado no equipamento; (2) community (ou credenciais) correcta e alinhada com o cadastro no NetQuasar; (3) o servidor do NetQuasar autorizado a consultar (ACL/firewall/lista de IPs no equipamento); (4) rede de gestão alcançável; (5) perfil/categoria correcta no cadastro (MikroTik, OLT, switch, etc.) para o sistema saber o que pedir. Sem isso, o ping pode funcionar e a telemetria falhar.",
    cat: "configurar",
  },
  {
    q: "Porque é preciso ter perfis de OLT?",
    a: "Cada fabricante (e por vezes cada modelo) expõe ONUs, PONs e potências de forma diferente. O perfil diz ao NetQuasar como falar com aquela marca: o que consultar, como interpretar ONUs/PONs e quais passos de coleta usar. Sem o perfil certo, a tela OLT pode ficar vazia, incompleta ou com dados errados.",
    cat: "tecnico",
  },
  {
    q: "Onde escolho / configuro o perfil da OLT?",
    a: "No cadastro do equipamento (categoria/perfil OLT) e em Configurações → vendors/perfis OLT, conforme a marca instalada. Depois de alterar, faça um refresh na tela OLT para validar.",
    cat: "configurar",
  },
  {
    q: "O que é uma porta PON?",
    a: "É a porta óptica da OLT que atende um grupo de ONUs (clientes). O NetQuasar mostra quantidade de ONUs, potências e alertas por PON para a operação saber onde está a saturação ou a falha.",
    cat: "tecnico",
  },
  {
    q: "O que é PPPoE no contexto do NetQuasar?",
    a: "É um tipo comum de sessão de cliente no ISP. O NetQuasar acompanha contagens de sessões (por exemplo no BNG ou em MikroTik) para alertar quando há queda anormal entre coletas.",
    cat: "tecnico",
  },
  {
    q: "O que é um módulo SFP?",
    a: "É o módulo óptico (ou metálico) plugável numa porta. Quando o equipamento expõe esses dados, o NetQuasar pode alertar potência TX/RX e temperatura do módulo.",
    cat: "tecnico",
  },

  {
    q: "Quais tecnologias usa o backend?",
    a: "Go — API, workers de monitoramento, coleta SNMP/ICMP/OLT, alertas e integrações.",
    cat: "tecnico",
  },
  {
    q: "Quais tecnologias usa o frontend?",
    a: "React, TypeScript e Vite, com a interface embutida no serviço em produção.",
    cat: "tecnico",
  },
  {
    q: "Que base de dados é usada?",
    a: "PostgreSQL — inventário, histórico, alertas, configurações e auditoria.",
    cat: "tecnico",
  },
  {
    q: "O Redis é obrigatório?",
    a: "Recomendado para tempo real e cache do dashboard; a aplicação pode operar sem Redis com funcionalidades de tempo real limitadas.",
    cat: "tecnico",
  },
  {
    q: "Como é feito o deploy típico?",
    a: "Docker Compose (serviço + Postgres + Redis opcional) ou binário único com a interface embutida, conforme a documentação de deploy Debian.",
    cat: "tecnico",
  },
  {
    q: "Onde corre o monitoramento automático?",
    a: "No próprio backend, em ciclos contínuos controlados pela tela Monitoramento e pelos intervalos em Configurações.",
    cat: "como_funciona",
  },
  {
    q: "Quais modos de monitoramento existem?",
    a: "Desligado; só ping (latência/disponibilidade); e completo (ping + telemetria SNMP + interfaces + OLT, conforme o pipeline activo).",
    cat: "funcoes",
  },
  {
    q: "Como ligo ou desligo o monitoramento?",
    a: "Na tela Monitoramento (início/parada). Utilizadores com permissão de administração controlam o runtime.",
    cat: "configurar",
  },
  {
    q: "O que é o pipeline de monitoramento?",
    a: "A sequência de passos que o sistema executa em cada ciclo (ping, telemetria, interfaces, OLT…). Ajuste em Configurações → Monitoramento → Pipeline.",
    cat: "como_funciona",
  },
  {
    q: "O ping pode correr em paralelo com outras coletas?",
    a: "Sim. No modo completo, o ping pode correr em paralelo enquanto o resto do pipeline segue, para detectar offline mais depressa sem atrasar a telemetria.",
    cat: "como_funciona",
  },
  {
    q: "Quantas falhas de ping abrem alerta offline?",
    a: "Por defeito, várias leituras consecutivas (tipicamente 3) antes de abrir alerta de inacessível ou de latência alta — para reduzir falsos positivos.",
    cat: "como_funciona",
  },
  {
    q: "O que é telemetria SNMP?",
    a: "Coleta periódica de CPU, memória, temperatura, uptime e outras métricas do perfil do equipamento, gravadas e comparadas com os limiares de alerta.",
    cat: "como_funciona",
  },
  {
    q: "Onde configuro limiares de alerta?",
    a: "Configurações → Alertas (limiar global), com operadores ≥/≤ e faixas Normal / Atenção / Crítico.",
    cat: "configurar",
  },
  {
    q: "Os limiares são avaliados só no refresh manual?",
    a: "Não — também no ciclo automático após uma telemetria bem-sucedida.",
    cat: "como_funciona",
  },
  {
    q: "Que tipos de alerta principais existem?",
    a: "Equipamento offline, latência alta, limiar de telemetria (CPU/memória/temp…), interface que caiu, óptica/temperatura de SFP, ONU/PON na OLT, queda de sessões BNG/PPPoE, entre outros.",
    cat: "funcoes",
  },
  {
    q: "Como ignoro um alerta?",
    a: "Na lista de Alertas, menu do alerta → Ignorar; fica silenciado na interface e no Telegram até reactivar.",
    cat: "como_funciona",
  },
  {
    q: "Como verifico se um alerta ainda é válido?",
    a: "Botão Verificar (individual ou em massa) recolhe dados de novo e reavalia a condição.",
    cat: "como_funciona",
  },
  {
    q: "O que são incidentes correlacionados?",
    a: "Agrupamentos (ex.: POP offline, OLT offline em cascata) para reduzir ruído e spam de notificações.",
    cat: "como_funciona",
  },
  {
    q: "Como configuro Telegram?",
    a: "Configurações → Telegram: token do bot, chats de monitoramento e de relatórios, e teste de envio.",
    cat: "configurar",
  },
  {
    q: "O NetQuasar envia e-mail?",
    a: "Sim, via SMTP configurável, usado em relatórios/resumos quando activado.",
    cat: "configurar",
  },
  {
    q: "O que é a tela de Tempo real?",
    a: "Actualizações ao vivo do painel quando a infra de tempo real (normalmente com Redis) está disponível.",
    cat: "funcoes",
  },
  {
    q: "Como adiciono equipamentos?",
    a: "Menu Equipamentos → Geral: criar/editar, coordenadas, POP, categoria, ping/telemetria e importação CSV quando disponível.",
    cat: "como_funciona",
  },
  {
    q: "O que são Localidades e POPs?",
    a: "Hierarquia geográfica: localidades e pontos de presença (POPs) para organizar equipamentos e correlacionar incidentes.",
    cat: "funcoes",
  },
  {
    q: "A tela Mapa mostra o quê?",
    a: "Equipamentos e infraestrutura (CTO, cabo, poste, emenda, projeto) com pesquisa, filtros e modos de visualização.",
    cat: "funcoes",
  },
  {
    q: "Posso pesquisar endereço no mapa?",
    a: "Sim — endereço, coordenadas ou link do Google Maps (incluindo links curtos).",
    cat: "funcoes",
  },
  {
    q: "O que são Elementos?",
    a: "Gestão de infraestrutura de rede óptica e ligações (CTOs, cabos, etc.) usada no mapa e em operações de campo.",
    cat: "funcoes",
  },
  {
    q: "Como funciona a tela OLT?",
    a: "Lista OLTs, visão de PONs/ONUs, refresh manual, potências ópticas e alertas de quantidade/óptica.",
    cat: "funcoes",
  },
  {
    q: "Quais marcas de OLT são suportadas?",
    a: "Depende dos perfis instalados (ex.: VSOL, ZTE e outras marcas com perfil compatível). Veja Configurações → OLT / vendors.",
    cat: "funcoes",
  },
  {
    q: "O que é a tela MikroTik?",
    a: "Monitor de RouterOS: interfaces, SFP, indicadores e coleta conforme o perfil em Configurações → MikroTik.",
    cat: "funcoes",
  },
  {
    q: "E a tela Switch?",
    a: "Vista semelhante para switches monitorizados, com interfaces e telemetria.",
    cat: "funcoes",
  },
  {
    q: "O que a tela BNG mostra no NetQuasar?",
    a: "Monitorização de sessões/assinantes e alertas quando a contagem cai de forma anormal entre coletas.",
    cat: "funcoes",
  },
  {
    q: "Para que servem as Ferramentas?",
    a: "Diagnósticos pontuais (checks, métricas rápidas MikroTik, etc.) sem esperar o ciclo automático.",
    cat: "funcoes",
  },
  {
    q: "O que são Integrações?",
    a: "Ligações a sistemas externos (ex. IXC, Hubsoft) para consulta e sincronização comercial/operacional.",
    cat: "funcoes",
  },
  {
    q: "A tela Clientes é o quê?",
    a: "Módulo comercial: localidades, clientes e dados associados à operação do ISP.",
    cat: "funcoes",
  },
  {
    q: "Como funcionam Relatórios?",
    a: "Relatórios agregados (capacidade OLT, lacunas de dados, resumos) com envio agendado por Telegram/e-mail quando configurado.",
    cat: "funcoes",
  },
  {
    q: "Onde vejo o histórico operacional?",
    a: "Na tela Alertas (histórico) e no histórico do equipamento. A linha do tempo global de eventos deixou de ser um menu separado.",
    cat: "funcoes",
  },
  {
    q: "Quem pode aceder a Configurações?",
    a: "Administradores ou utilizadores com permissões específicas de configurações.",
    cat: "configurar",
  },
  {
    q: "Como funcionam permissões?",
    a: "Perfis por módulo (mapa, equipamentos, OLT, settings, etc.); o administrador vê tudo.",
    cat: "configurar",
  },
  {
    q: "O NetQuasar guarda auditoria?",
    a: "Sim — regista alterações relevantes nas configurações e eventos importantes da operação.",
    cat: "como_funciona",
  },
  {
    q: "Como faço backup da base de dados?",
    a: "Com Docker, use um dump do Postgres (pg_dump) para um ficheiro .sql e agende diariamente. Há um exemplo na aba Backups da tela Sobre.",
    cat: "configurar",
  },
  {
    q: "O que mais devo incluir no backup?",
    a: "Dump da base, ficheiro de secrets/.env (em local seguro), volumes auxiliares se usados, e export de configurações críticas.",
    cat: "configurar",
  },
  {
    q: "Com que frequência fazer backup?",
    a: "Recomendado diário (completo) e retenção de pelo menos 7–30 dias; testar o restauro periodicamente.",
    cat: "configurar",
  },
  {
    q: "Como restauro um dump SQL?",
    a: "Com a ferramenta do Postgres (psql ou equivalente no Docker), apontando para o utilizador/base correctos. Idealmente com a API parada.",
    cat: "configurar",
  },
  {
    q: "Há backup de configuração de equipamento?",
    a: "Sim — o sistema pode guardar texto de configuração do equipamento quando essa funcionalidade é usada.",
    cat: "funcoes",
  },
  {
    q: "O NetQuasar faz backup automático para a nuvem?",
    a: "Não por defeito; a cópia off-site (S3, NAS, outro site) é responsabilidade do operador.",
    cat: "funcoes",
  },
  {
    q: "Como é produzido o software?",
    a: "Monorepo com backend e frontend; o build da interface é embutido no serviço em produção; a CI pode validar o projecto.",
    cat: "como_funciona",
  },
  {
    q: "Como corro em desenvolvimento?",
    a: "API local + interface em modo desenvolvimento; Postgres/Redis locais ou via Docker.",
    cat: "como_funciona",
  },
  {
    q: "Qual a porta típica do serviço?",
    a: "Definida no ambiente/Compose; a interface e a API partilham o serviço publicado.",
    cat: "tecnico",
  },
  {
    q: "Preciso de internet no servidor NOC?",
    a: "Para Telegram, geolocalização de endereços, actualizações e algumas integrações, sim. O monitoramento local na LAN funciona sem Internet.",
    cat: "como_funciona",
  },
  {
    q: "O SNMP usa que versões?",
    a: "Principalmente SNMP versão 2c (community). Os perfis por marca definem o que é consultado.",
    cat: "tecnico",
  },
  {
    q: "O ping usa só ICMP?",
    a: "Prioriza ICMP e pode recorrer a um teste TCP quando faz sentido (rede que bloqueia ping, por exemplo).",
    cat: "tecnico",
  },
  {
    q: "Porque um equipamento não aparece no mapa?",
    a: "Sem coordenadas válidas, filtros activos, camada desligada, ou fora da área/zoom visível.",
    cat: "como_funciona",
  },
  {
    q: "Porque não recebo Telegram?",
    a: "Confirme token/bot, chat correcto, se o alerta está ignorado e se o envio de monitoramento está activo. Use o teste em Configurações → Telegram.",
    cat: "configurar",
  },
  {
    q: "Latência alta sem alerta — porquê?",
    a: "Limiar de latência não atingido, ainda não houve falhas consecutivas suficientes, equipamento fora do monitoramento, ou alertas desse tipo desactivados.",
    cat: "como_funciona",
  },
  {
    q: "Temperatura MikroTik sem alerta — o que verificar?",
    a: "Se a telemetria está a trazer temperatura, se o limiar de temperatura está activo e se o monitoramento completo está a correr.",
    cat: "como_funciona",
  },
  {
    q: "Interface DOWN em falso — o que mudou?",
    a: "Há confirmação em ciclos e verificação imediata do estado operacional da interface para evitar falsos positivos de coletas incompletas.",
    cat: "como_funciona",
  },
  {
    q: "Posso silenciar só uma PON?",
    a: "Sim — ignorar alerta pode silenciar um padrão específico (equipamento + tipo + contexto, como uma PON ou interface).",
    cat: "como_funciona",
  },
  {
    q: "O que é coleta nocturna?",
    a: "Janela horária para ciclos mais pesados fora do pico comercial (Configurações → Monitoramento).",
    cat: "como_funciona",
  },
  {
    q: "Como limito carga SNMP na rede?",
    a: "Ajuste intervalos e timeouts, desactive passos do pipeline que não precisa e evite refreshes manuais em massa desnecessários.",
    cat: "configurar",
  },
  {
    q: "OLT sem ONUs no snapshot — causas comuns?",
    a: "Perfil incorrecto, equipamento offline ou inacessível por SNMP, coleta incompleta, ou o sistema a preservar o último snapshot válido enquanto estabiliza.",
    cat: "como_funciona",
  },
  {
    q: "Posso importar CSV de equipamentos?",
    a: "Sim, quando a importação está activa em Equipamentos → Geral.",
    cat: "funcoes",
  },
  {
    q: "O mapa aceita KML ou KMZ?",
    a: "Sim. Em Elementos → Projetos pode importar .kml ou .kmz; abre-se um ecrã de revisão para classificar CTO, emenda, poste e cabo.",
    cat: "funcoes",
  },
  {
    q: "Como vejo histórico de alertas?",
    a: "Aba Histórico em Alertas; filtre por intervalo de datas (abertura ou fecho no período).",
    cat: "como_funciona",
  },
  {
    q: "Posso categorizar a lista de alertas?",
    a: "Sim — o botão Categorizar separa por tipo (offline, latência, SFP, etc.).",
    cat: "funcoes",
  },
  {
    q: "Há API para integrações próprias?",
    a: "Sim — API REST sob /api/v1. Parte da documentação está no README do projecto.",
    cat: "tecnico",
  },
  {
    q: "Como actualizo o NetQuasar?",
    a: "Actualize a imagem/binário, aplique migrações (automáticas na arranque quando configurado) e reinicie. Faça backup antes.",
    cat: "configurar",
  },
  {
    q: "As migrações da base são automáticas?",
    a: "Sim, na arranque do serviço, conforme a configuração do projecto.",
    cat: "configurar",
  },
  {
    q: "Posso usar HTTPS?",
    a: "Sim — coloque um reverse proxy (Nginx, Caddy ou Traefik) à frente do serviço com certificado TLS.",
    cat: "configurar",
  },
  {
    q: "Como protejo o acesso?",
    a: "Utilizadores com palavra-passe, sessão, permissões por módulo e restrição à rede de gestão/VPN.",
    cat: "configurar",
  },
  {
    q: "O NetQuasar substitui Zabbix/Grafana?",
    a: "É focado em operação ISP (OLT, BNG, MikroTik, mapa óptico). Pode coexistir com outras ferramentas.",
    cat: "tecnico",
  },
  {
    q: "Suporta multi-tenant?",
    a: "A instalação típica é por ISP/operador; multi-tenant avançado depende da arquitectura da sua implantação.",
    cat: "tecnico",
  },
  {
    q: "Há app mobile nativo?",
    a: "A interface é web responsiva; use o browser no telemóvel. O mapa foi otimizado para uso em campo.",
    cat: "funcoes",
  },
  {
    q: "Como reporto um bug?",
    a: "Ao responsável técnico/desenvolvedor (Kallebe Assis Nogueira) com passos, capturas/logs e versão/ambiente.",
    cat: "configurar",
  },
  {
    q: "Onde vejo a versão / documentação do produto?",
    a: "Na tela Sobre (menu de ajuda junto a Sair), e nas notas de release da sua instalação.",
    cat: "como_funciona",
  },
  {
    q: "Posso editar o que o MikroTik coleta?",
    a: "Sim, em Configurações → coleta MikroTik (métricas e passos).",
    cat: "configurar",
  },
  {
    q: "O dashboard mostra o quê?",
    a: "Resumo operacional: disponibilidade, alertas, capacidade OLT, lacunas de dados e indicadores-chave.",
    cat: "funcoes",
  },
  {
    q: "O indicador de actividade no topo — o que é?",
    a: "Mostra actividade recente do monitoramento/sistema para a interface actualizar dados.",
    cat: "funcoes",
  },
  {
    q: "Posso esconder elementos no mapa?",
    a: "Sim — controlos de camadas e filtros por projecto, POP e categoria.",
    cat: "funcoes",
  },
  {
    q: "O que é modo edição no mapa?",
    a: "Permite reposicionar CTOs/elementos e editar trajetos de cabo quando tem permissão de gestão do mapa.",
    cat: "funcoes",
  },
  {
    q: "GPS no mapa para quê?",
    a: "Localização do técnico e listagem de CTOs próximas para operação de campo.",
    cat: "funcoes",
  },
  {
    q: "Alertas de SFP medem o quê?",
    a: "Potência de transmissão/recepção (dBm) e temperatura do módulo óptico, quando o equipamento expõe esses dados.",
    cat: "funcoes",
  },
  {
    q: "Como o BNG gera alerta de queda?",
    a: "Compara a quantidade de sessões entre coletas e alerta quando a queda ultrapassa o limiar configurado.",
    cat: "funcoes",
  },
  {
    q: "E queda de PPPoE no MikroTik?",
    a: "Mesma ideia: variação anormal de sessões PPPoE entre coletas.",
    cat: "funcoes",
  },
  {
    q: "Posso exportar relatórios?",
    a: "Depende do relatório; vários permitem visualização/download e envio agendado.",
    cat: "funcoes",
  },
  {
    q: "O que é o digest de alertas?",
    a: "Resumo periódico de contagens e severidades enviado por Telegram ou e-mail.",
    cat: "funcoes",
  },
  {
    q: "Relatório ONU mensal?",
    a: "Automação que percorre OLTs, agrega ONUs e envia um resumo (Telegram) quando configurada.",
    cat: "funcoes",
  },
  {
    q: "Como configuro aparência?",
    a: "Configurações → Aparência: tema e cores/ícones do mapa.",
    cat: "configurar",
  },
  {
    q: "Tema claro/escuro existe?",
    a: "Sim, conforme a preferência de aparência guardada.",
    cat: "configurar",
  },
  {
    q: "O NetQuasar grava histórico de ping e telemetria?",
    a: "Sim — histórico de disponibilidade/latência e séries de métricas para gráficos, KPIs e limiares.",
    cat: "tecnico",
  },
  {
    q: "Posso forçar uma coleta manual?",
    a: "Sim — nas telas de equipamento/OLT/MikroTik (refresh) e também pela tela Monitoramento, conforme as permissões.",
    cat: "como_funciona",
  },
  {
    q: "O que acontece se o Postgres cair?",
    a: "A API e o monitoramento deixam de gravar dados. Restabeleça a base a partir do backup e reinicie os serviços.",
    cat: "como_funciona",
  },
  {
    q: "Como contacto o desenvolvedor?",
    a: "Via landing page https://portfolio-kallebe-assis.vercel.app/ (Kallebe Assis Nogueira).",
    cat: "como_funciona",
  },
  {
    q: "O que o módulo Frota faz?",
    a: "Controla veículos, motoristas, postos, combustíveis, centros de custo e tipos de despesa. Permite lançar abastecimentos e outras despesas, ver o dashboard de gastos, alertas de consumo/preço e exportar relatórios (CSV ou Telegram).",
    cat: "funcoes",
  },
  {
    q: "Como importo o histórico da frota?",
    a: "Em Configurações → Frota, use a importação em massa: baixe o modelo CSV (Excel), preencha e importe veículos, motoristas e lançamentos. A importação aceita veículos inativos; lançamentos manuais novos em veículo inativo, vendido ou baixado são recusados.",
    cat: "configurar",
  },
  {
    q: "O hodômetro do veículo atualiza sozinho?",
    a: "No abastecimento, o KM atual do lançamento atualiza o cadastro. Na despesa, se você alterar o KM em relação ao valor do veículo, o sistema pergunta se deve atualizar. A importação CSV não altera o hodômetro do cadastro, para não distorcer histórico antigo.",
    cat: "como_funciona",
  },
  {
    q: "O que é a descoberta SNMP?",
    a: "Fluxo auxiliar para explorar o que o equipamento expõe via SNMP e ajudar a afinar perfis/coleta. Os resultados ficam nos dados do backend conforme a funcionalidade activa.",
    cat: "tecnico",
  },
];

# NetQuasar

Plataforma de **monitoramento e operação de rede** para provedores ISP (NOC). Centraliza inventário, coleta automática (ping, SNMP, OLT, interfaces), alertas, notificações, integrações com sistemas externos e ferramentas de diagnóstico numa interface web única.

| Componente | Tecnologia | Função |
|------------|------------|--------|
| **Backend** | Go (`quasar_backend`) | API REST, workers de monitoramento, alertas, integrações |
| **Frontend** | React + TypeScript + Vite (`quasar_frontend`) | Painel operacional para o dia a dia do NOC |
| **Dados** | PostgreSQL | Inventário, histórico, alertas, configurações, auditoria |
| **Cache / tempo real** | Redis (recomendado) | WebSocket realtime + cache do dashboard |
| **Deploy** | Docker Compose + binário único | API + UI embutida na porta publicada |

Documentação complementar: [Backend](quasar_backend/README-BACKEND.md) · [Frontend](quasar_frontend/README-FRONTEND.md) · [Deploy Debian](deploy/linux-debian/README.md) · [Roadmap](ROADMAP-ARQUITETURAL-DEPLOY.md)

---

## Arquitetura

```text
┌──────────────────────────────────────────────────────────────────┐
│  Browser  →  React (Vite em dev ou UI embutida no Go em prod.)   │
└────────────────────────────┬─────────────────────────────────────┘
                             │ REST /api/v1/…  (+ WebSocket /realtime/ws)
                             ▼
┌──────────────────────────────────────────────────────────────────┐
│  netquasar (Go)                                                  │
│  · internal/api        — handlers HTTP por domínio               │
│  · internal/monitorworker — ping, SNMP, OLT, interfaces, alertas │
│  · internal/alertthresholds / alertignore / alertverify          │
│  · internal/probing / snmp* / oltcollect — sondas e parsing      │
│  · internal/alertnotify — Telegram e resolução de alertas          │
│  · internal/alertcorrelation — incidentes (POP/OLT em cascata)   │
└───────────────┬──────────────────────────────┬───────────────────┘
                ▼                              ▼
         PostgreSQL                      Redis (tempo real)
```

O **worker de monitoramento** (`monitorworker.Run`) corre em goroutine dedicada, lê o estado em `monitoring_runtime` (ligado/desligado, modo) e dispara ciclos conforme `monitoring_intervals` e a ordem de passos em **Configurações → Monitoramento → Pipeline**. Cada alteração relevante atualiza timestamps em `monitoring_runtime` para o frontend invalidar caches (polling + `useMonitoringLiveSync`).

**Modo `full`:** o ping pode correr **em paralelo** (`ping_parallel=true`, intervalo `ping_seconds`) enquanto telemetria, interfaces e OLT correm **em sequência** num pipeline único (intervalo `pipeline_cycle_seconds`). Com `ping_parallel=false`, o ping entra no pipeline em vez de correr à parte. **Modo `simple_ping`:** apenas latência/ICMP-TCP, sempre activo independentemente de `ping_parallel`.

---

## Motor de monitoramento

### Modos

| Modo | Comportamento |
|------|----------------|
| `off` | Worker inactivo |
| `simple_ping` | Apenas ciclo de latência/ICMP-TCP (`ping_seconds`) |
| `full` | Ping + telemetria SNMP + snapshots de interfaces + coleta OLT/PON |

Ligação/desligação: **Monitoramento** na UI ou `POST /api/v1/monitoring/start` / `stop` (admin).

### Ciclos automáticos

| Ciclo | Intervalo (config) | O que faz |
|-------|-------------------|-----------|
| **Latência / ping** | `ping_seconds` (paralelo) ou dentro do pipeline | Para cada equipamento monitorizado: ICMP e fallback TCP; grava `device_probe_cache` (`reach_ok`, `latency_ms`); abre/fecha alerta `ping_unreachable`; histórico em `ping_history`. Requer **3 leituras consecutivas** antes de abrir alerta de latência alta ou offline. |
| **Telemetria SNMP** | Passo no pipeline (`pipeline_cycle_seconds`) | Walk/get conforme perfil do equipamento; amostras em `telemetry_samples` (CPU, memória, temperatura, uptime); **no fim de cada ciclo OK** chama `RunPostTelemetryAlertEval` → alertas `telemetry_threshold` e `uptime_restart_low` (também no refresh manual) |
| **Interfaces (IF-MIB)** | Passo no pipeline | Walk IF-MIB (+ IF-MIB-X); grava `interface_snapshots`; MikroTik: potências SFP **e temperatura do módulo**; alertas `mikrotik_sfp_tx` / `mikrotik_sfp_rx` / `mikrotik_sfp_temp`; detecta transição UP→DOWN → `interface_down_transition` |
| **OLT PON / ONUs** | Passo no pipeline | Por OLT online com telemetria activa: colecta contagem de ONUs por PON, atualiza `olt_snapshots`, deriva status PON (ON se ≥1 ONU online), avalia alertas de queda/subida de ONUs, potência óptica ONU/PON e temperatura da PON |
| **Pipeline completo** | `pipeline_cycle_seconds` | Executa os passos activos em sequência (ping incluído se `ping_parallel=false`); atualiza `last_pipeline_cycle_at` |

Os campos `telemetry_seconds`, `interface_snapshot_seconds` e `olt_if_derived_pon_seconds` continuam disponíveis para ciclos **manuais** (`POST /monitoring/cycles/...`) e referência na UI; no worker automático em modo full, o gatilho principal é `pipeline_cycle_seconds`.

Cada ciclo tem **timeout** próprio (`telemetry_timeout_ms`, `interface_snapshot_timeout_ms`, `olt_if_derived_pon_timeout_ms`) configurável em **Configurações → Monitoramento**.

Execução manual de um ciclo: `POST /api/v1/monitoring/cycles/{latency|telemetry|interfaces|olt-if-derived}` (admin, opcional `force=true`).

### Coleta OLT — como funciona

1. **OLT com derive IF-MIB** (marcas compatíveis, exclui ZTE/Datacom/VSOL): walk IF-MIB, deriva PONs/ONUs (`oltifderive`), estabiliza vs. snapshot anterior, grava `olt_snapshots`.
2. **OLT por perfil de fabricante** (VSOL, ZTE, etc.): lê `olt_vendor_models` (passos SNMP/telnet); executa `onu_metrics_collect` ou `onu_snmp_walk`; grava o mesmo `olt_snapshots`.
3. **Refresh manual** na tela OLT: `POST /olt/devices/{id}/refresh` executa o perfil completo do modelo (scope `full` ou `onu`).

**Serial das ONUs.** O walk único da tabela de serial de uma OLT grande estourava o orçamento de tempo e era cortado em ordem de PON — as primeiras PONs completavam e as últimas ficavam sem serial. Na coleta completa (`oltcollect/serial_completion.go`) o NetQuasar agora volta só nas PONs que ainda têm ONU sem serial e refaz a leitura da sub-árvore daquela PON, com tempo próprio, começando pelas mais incompletas. A fila do enriquecimento por telnet também gira (em vez de reler sempre as mesmas primeiras ONUs). ONU **offline** não informa serial por SNMP — mantém o último serial conhecido. Os modos `baseline`/`status_rx` não leem serial: ele vem da coleta completa.

**Interfaces/PONs sem duplicar.** O inventário de interfaces e o resumo de PON deduplicam a mesma porta quando a OLT a expõe com dois nomes (ex.: `PON 01` e `GPON001` numa VSOL V1600G1) — vale para todas as marcas (`oltifderive`: `CanonicalPonRowKey`, `DedupeOltInterfaceTablePonRows`).

A UI OLT e o Dashboard leem `olt_snapshots` e atualizam-se via polling + sinalização de `monitoring_runtime.activity_updated_at`.

### Coleta nocturna

Configurável em **Monitoramento → Coleta nocturna**: janela horária para ciclos mais pesados sem sobrecarregar o horário comercial (`PATCH /monitoring/nightly-collection`).

---

## Sistema de alertas

### Tipos de alerta

| Tipo | Origem | Condição típica |
|------|--------|-----------------|
| `ping_unreachable` | Worker ping | Equipamento sem resposta ICMP/TCP |
| `latency_high` | Worker ping | Latência acima do limiar global (`latency_ms`) |
| `uptime_restart_low` | Telemetria SNMP | Uptime abaixo do mínimo (possível reinício) |
| `telemetry_threshold` | Telemetria SNMP (worker + refresh) | CPU, memória, temperatura, uptime fora do limiar (`cpu_usage_pct`, `memory_usage_pct`, `temperature_c`, `uptime_minutes`) |
| `interface_down_transition` | Snapshot interfaces | Interface mudou de UP para DOWN |
| `mikrotik_sfp_tx` / `mikrotik_sfp_rx` / `mikrotik_sfp_temp` | Snapshot interfaces MikroTik | Potência óptica ou temperatura do módulo SFP fora do limiar |
| `olt_onu_drop` / `olt_onu_rise` | Coleta OLT | Queda ou subida de ONUs online por PON (contagem ou %) |
| `olt_onu_rx` / `olt_onu_tx` | Coleta OLT | Potência óptica ONU/PON abaixo do limiar (`olt_onu_*_dbm`) |
| `olt_pon_tx` / `olt_pon_rx` / `olt_pon_temp` | Coleta OLT | TX/RX ou temperatura da PON fora do limiar (`olt_pon_*`) |
| `pon_down` | Coleta OLT | Status operacional da PON UP→DOWN |
| `bng_*_drop` / `mikrotik_pppoe_drop` | Telemetria BNG / snapshot MikroTik | Queda de sessões entre coletas |

Limiares globais: **Configurações → Alertas** (aba própria; regra «Limiar global de alertas» em `alert_rules`). Operadores: **≥** (maior ou igual, `gte`) e **≤** (menor ou igual, `lte`). Severidade: `info`, `warning`, `critical`.

Cada métrica na UI só gera alerta se estiver **habilitada**, o perfil global estiver **activo**, e o equipamento coincidir com `apply_categories` (vazio = todos). A avaliação de telemetria (CPU/mem/temp) corre no **ciclo automático do worker**, não apenas no refresh manual.

### Ciclo de vida de um alerta

1. **Detecção** — worker ou refresh manual compara métrica com limiar.
2. **Criação** — `INSERT` em `alert_instances` se não existir alerta aberto do mesmo padrão (`device_id` + `alert_type` + `meta.key`).
3. **Atualização** — mesma condição mantém o alerta aberto e atualiza `message` / `meta` (ex.: latência 243→210 ms).
4. **Notificação** — `alertnotify.SendMonitoringTelegramAndPatchMeta` envia Telegram (config «monitoring») e regista resultado em `meta.telegram`.
5. **Resolução** — quando a condição normaliza, `closed_at` é preenchido e Telegram de resolução é enviado.

### Incidentes correlacionados

`alertcorrelation` agrupa alertas por causa provável:

- **POP offline** — vários equipamentos do mesmo POP sem ping.
- **OLT offline** — OLT offline com efeito em cascata nas ONUs.

Visíveis em **Alertas → Incidentes correlacionados**. Telegram de cascata é suprimido para evitar spam.

### Ignorar, verificar e suprimir

| Acção | Função |
|-------|--------|
| **Ignorar alerta** | Persiste em `alert_ignores` (equipamento + tipo + chave PON/interface/métrica); fecha alertas abertos do padrão; bloqueia novos alertas na UI **e** no Telegram |
| **Verificar** | Reavalia a condição (ping, latência, snapshot SFP, OLT, etc.) e atualiza valor na lista ou fecha se normalizado |
| **Verificar alertas** (global) | Recalcula pings + reverifica até 250 alertas abertos |
| **Alertas ignorados** | Modal com lista completa; opção **Reactivar** remove o silêncio |
| **Supressões** (`alert_suppressions`) | Filtro por scope POP/global na listagem (legado; ignorar por equipamento é o modelo preferido) |

### Revalidação

`POST /alerts/revalidate` — fecha `ping_unreachable` obsoletos quando o probe já está OK.

---

## Módulos da interface

### Dashboard (`/dashboard`)

**Função:** visão executiva da rede.

**Como funciona:** agrega dados de `device_probe_cache`, `alert_instances`, `olt_snapshots` e endpoints `/dashboard/analytics`, `/dashboard/olt-capacity`, `/dashboard/data-gaps`, `/overview/top-latency`. Carregamento progressivo com cache (`dashboardCache`). Abas: **Geral**, **Equipamentos**, **Fibra óptica**, **Infraestrutura**, **Sessões PPPoE**, **Servidor NetQuasar** e **Frota** (as quatro primeiras partilham a janela de dias seleccionada).

- **Fibra óptica** — ONUs online/offline por OLT, capacidade por PON, portas de CTO, e uma lista por PON de quantas ONUs **online** estão com RX abaixo do limiar de "boa" (Configurações → OLT → *Qualidade da potência RX (ONU)*; offline e sem leitura óptica ficam de fora).
- **Infraestrutura** — totais de projetos/CTOs/emendas/distribuições/cabos/postes, estado das CTOs (vazia / disponível / próxima da saturação ≥ 80% / lotada / sem portas cadastradas), CTOs por tipo de splitter (1x8, 1x16…) e elementos por projeto de rede.

---

### Monitoramento (`/monitoring`)

**Função:** painel de controlo do motor de coleta.

**Como funciona:** lê `GET /monitoring/state` (actividade actual, últimos ciclos, `is_running`). Permite iniciar/parar monitoramento, ver equipamentos activos (`/monitoring/active-equipment`), disparar ciclos manuais e configurar coleta nocturna. Indicador global no menu reflecte `current_activity` do worker.

---

### Tempo real (`/realtime`)

**Função:** latência e estado de reachability em fluxo contínuo.

**Como funciona:** WebSocket `GET /realtime/ws` (broker Redis quando configurado) ou polling `GET /realtime/ping`. Atualiza lista de equipamentos sem esperar o intervalo completo do worker.

---

### Integrações (`/integrations`)

**Função:** ligação a ERPs, CRMs e APIs externas do ISP.

**Como funciona:** cada integração tem URL base, autenticação e **pedidos** configuráveis (templates HTTP). O motor `integrationhttp` executa pedidos; `integrationconsumer` expõe acções de consulta (cliente, OS, login PPPoE). Logs em `integration_logs`. Uso típico: pesquisar cliente por CPF/login a partir da tela **Conexões**.

**HubSoft** (`internal/integrationhubsoft`, caminho dedicado — não passa pelo motor genérico acima) tem abas próprias: Consulta, Atendimentos, Ordens de serviço, Financeiro, Dashboard e **Relatório** (`/integrations/hubsoft/relatorio`). O Relatório tem sub-abas:

- **Clientes** — filtro de clientes/serviços por estado/cidade/bairro/status/IPv4/MAC (o cartão do cliente já traz o endereço de instalação).
- **Serviços** — fotografia actual da base inteira: total, repartição por status, por plano e por localidade; a lista por localidade é uma tabela (uma linha por localidade) e clicar abre um modal com abas Status / Plano / Bairro só dessa localidade. Cache no cliente por 5 min. **Enviar por Telegram** abre uma selecção (Total de logins / por status / por plano / por localidade / localidade específica / tudo).
- **Atendimentos** e **Ordens de serviço** por período (O.S. com ranking por técnico), e **Financeiro** (percentual recebido/aberto/vencido, mês específico ou média dos últimos X meses) — cada um com **Enviar por Telegram**.
- **Boletos por forma** (`GET …/hubsoft/report/invoices-by-method?forma=…&data_inicio=…&data_fim=…`, somente leitura) — lista os boletos **em aberto** cuja **fatura** ainda carrega uma forma de cobrança (ex.: «Sicoob - API (G2)»), mesmo que o cliente/serviço já tenha sido migrado para outra: a forma fica gravada na fatura quando o boleto é gerado. Varre `/financeiro/fatura` (100 por página, sem `tipo_resultado=simplificado`, que não traz a forma) e filtra pelo id ou por parte do nome (ignora acento, caixa e pontuação). Mostra a contagem por forma encontrada, a lista (cliente, login, fatura, vencimento, valor, situação, link do boleto) e exporta CSV para a análise manual na HubSoft — a API não tem rota para trocar a forma de cobrança de uma fatura. Se a HubSoft não devolver o campo, o relatório avisa e lista os campos que a fatura traz (`campos_fatura`). Depois da lista, a etapa **«Forma de cobrança atual dos serviços»** (`POST …/invoices-by-method/services-check`, `{ids, forma}`) lê os serviços de cada cliente da lista (`GET /cliente`, somente leitura) e confere se estão na forma escolhida (ex.: «Banco do Brasil (G2)»); serviços cancelados não contam e, se a HubSoft não devolver a forma do serviço, ele sai como «sem dado» (nunca como certo/errado) junto com a lista de campos do serviço. As formas oferecidas nas listas vêm do catálogo da HubSoft (`GET …/invoices-by-method/formas`).

**Cartão do cliente (Consulta):** o modal do cliente tem a aba **Serviços** com ações por serviço — **Habilitar serviço**, **Suspender serviço** (por débito ou a pedido do cliente) e **Limpar MAC** (`POST /integrations/{id}/hubsoft/service/{serviceId}/reset-mac` → `POST /api/v1/integracao/cliente/reset_mac_addr` da HubSoft, corpo `{"id_cliente_servico"}`). Todas pedem confirmação, mostram a mensagem de erro da própria HubSoft (a HubSoft exige que o serviço já tenha dados de autenticação para limpar o MAC) e ficam em `ops_audit_log` (`hubsoft_client_service`: `enable` / `suspend` / `reset_mac`).

#### HubSoft — Configuração API e ferramentas de administração

`/integrations/hubsoft/config` é organizada em abas (a aba aberta fica na URL, `?aba=`). **Conexão** é de uso geral; as demais exigem a permissão **`integrations.hubsoft_bulk`** («HubSoft: edições em massa», perfil de permissão — administradores já têm) e **IXC: logins** exige **`integrations.ixc_logins`**. As rotas correspondentes ficam em dois grupos protegidos por `requirePermissionMiddleware` em `server.go`; no frontend o acesso é decidido por `can()` / `AdminOnly also=[…]`.

| Aba | O que faz | Escreve na HubSoft? |
|-----|-----------|---------------------|
| **Importar clientes** | CSV de clientes novos e de serviços adicionais: validar → conferir com a HubSoft (*preflight*) → importar. Botão de **baixar o modelo CSV** de cada tipo | Sim (cria cliente/serviço/login) |
| **Conferir cadastros** | Compara o mesmo CSV da importação com o que está na HubSoft, campo a campo | Não (só o botão «Registrar» observação de login) |
| **Corrigir senhas** | Varre a base atrás de senhas gravadas como `="12345"` e troca por `12345` | Sim, com confirmação |
| **Conferir endereços** | Compara os 4 endereços de cada serviço (fiscal, cadastral, cobrança, instalação) e lista os divergentes; CSV para correção manual | Não |
| **Data de venda** | Correção em lote da data da venda (CSV, pré-visualização e releitura) | Sim |
| **Catálogos** | Consulta serviços, vencimentos, vendedores, formas de cobrança, status, grupos, equipamentos e POPs da conta | Não |
| **Histórico** | Linhas importadas/recusadas (`ops_audit_log`) | Não |
| **IXC: logins** | Inativa/reativa logins (`radusuarios`) do IXC em massa a partir de uma lista (migração para a HubSoft) | Escreve no **IXC** |

**Catálogos de estoque (patrimônios):** a aba *Catálogos* lista também os IDs necessários para cadastrar produtos e patrimônios — locais de estoque, produtos, categorias, marcas, tipos, status de patrimônio e empresas (`catalog.go`; os catálogos paginados são lidos inteiros no servidor). Pela documentação oficial da API: **não existe rota para criar um patrimônio avulso** — ele nasce da *entrada manual de estoque* (`POST /estoque/movimento_estoque/entrada`) de um produto com `controle_patrimonial = true` (uma unidade = um patrimônio, com código automático e série/MAC vazios, status ESTOQUE); depois `PUT /estoque/produto_item/{id}` grava `numero_serie`, `mac_address` e `identificador_proprio`, e `GET /estoque/produto_item/consultar?busca=mac_address|numero_serie|codigo_item|identificador_proprio&termo_busca=` localiza um patrimônio. O comodato é a *saída para serviço do cliente* (`POST …/movimento_estoque/saida/cliente_servico`, exige `id_tipo_movimento_estoque`, `id_local_estoque` e os patrimônios). A API não expõe «repartições» dentro de um local de estoque.

**Produtos de estoque (aba «Produtos de estoque»)** — `POST …/hubsoft/stock-products/validate|preflight|apply` (`stock_product.go`, permissão `integrations.hubsoft_bulk`): cria produtos a partir de CSV (`codigo`, `nome`, `id_categoria`, `id_marca`, `unidade_medida`, valores e as 7 opções de configuração: NF, venda, comodato, vínculo POP/projeto/usuário/composição; modelo para baixar na tela). Cada produto passa por: checagem de duplicidade (mesmo código ou mesmo nome → nunca recria), `POST /estoque/produto`, `PUT /estoque/produto/{id}` com `produto_configuracao` (a API só aceita a configuração no PUT) e `GET` de conferência. As três etapas aparecem separadas no resultado; se a criação der certo e a configuração falhar, a mensagem diz que o produto FOI criado (com o id). Erros da HubSoft são mostrados com a mensagem e o corpo bruto da resposta; cada linha vai para `ops_audit_log` (`hubsoft_stock_product`, aba Histórico → «Produtos de estoque») e o log do servidor. A conferência relê o produto e reprova se a **marca** ou a **categoria** gravadas forem diferentes das pedidas (a HubSoft já ignorou o `id_produto_marca` sozinho e criou uma marca «API»; por isso o POST envia id **e** nome exato da marca, e uma marca que não esteja no catálogo recusa a linha antes de enviar). Para após 3 falhas seguidas. Os patrimônios em si (série/MAC, entrada no almoxarifado, comodato) são passos seguintes.

**Importação em massa** (`internal/integrationhubsoft`: `bulk_import*.go`)

- **Fluxo:** a validação local confere o formato de cada campo e — com os catálogos carregados — se cada ID existe na conta; o **preflight** (`POST …/bulk-import/preflight`, somente leitura) mostra para cada linha se vai criar cliente novo, adicionar serviço a um cliente existente (mesmo ou outro endereço) ou se o login já existe; a aplicação (`POST …/bulk-import/apply`) roda em **lotes de 15 linhas e para após 3 falhas seguidas**. Cada linha criada/recusada fica no histórico (`ops_audit_log`).
- **Regra de ouro:** nenhum valor não confirmado é enviado à HubSoft — a mesma validação da fase 1 roda de novo em cada linha antes de montar o pedido.
- **Duplicidade:** cliente é localizado por CPF/CNPJ; serviço, por login. Antes de criar, a importação confere se o login já existe em **qualquer** cadastro (`login_radius`); se existir, a linha não é cadastrada (`skipped_login_in_use`). Cliente que já existe ganha o serviço novo no **endereço de instalação do CSV** (e a linha avisa se difere do cadastrado).
- **Login/senha PPPoE:** nenhuma rota da API cria a autenticação do serviço — `configurar_autenticacao` só altera uma existente. Por isso a HubSoft precisa ter a automação de **login automático** (máscara) ativa: o serviço nasce com um login predefinido (`netquasar`) e o NetQuasar o troca pelo do CSV. Depois de configurar, o sistema relê o serviço e confere login e senha (reaplica só a senha se divergir). *Reparo:* um serviço que ficou com o login padrão `netquasar`, ou com a senha provisória `nq12345`, é corrigido ao reenviar a linha — nunca se há 2 ou mais serviços no login padrão.
- **Caixa do login:** a HubSoft padroniza o login em minúsculas e o Radius dela **não diferencia maiúsculas** (confirmado pela HubSoft); a **senha** é exata. Para um futuro ERP que diferencie, grava-se nas **Observações da autenticação** o texto `Login PPPoE configurado no cliente: <login original>` (automático na importação; em lote pelo botão «Registrar» da aba *Conferir cadastros*, `POST …/login-observation`; observações do operador são preservadas).
- **Regras reais da HubSoft aplicadas na validação:** telefone com no mínimo 10 dígitos; e-mail só ASCII e sem ponto no fim; **carnê** (`carne = true`) exige `gerar_carne`, enviado sempre como `nao_gerar_carne` (marca o serviço como carnê sem gerar boleto); **data de nascimento** vazia vira `01/01/1900` (com aviso) e titular menor de 18 anos recusado pela HubSoft é reenviado **uma vez** com `01/01/1990`.
- **Limites da API (corrigir só na HubSoft):** `PUT /cliente/cliente_servico/editar/{id}` altera apenas forma de cobrança, vendedor, status, data da venda, perfil de suspensão e grupos do **serviço**. **Não** há edição de plano, endereço nem grupo do **cliente** — por isso as ferramentas de conferência listam e exportam, mas a correção é feita na interface da HubSoft. *Grupos de serviço* (`grupo_cliente_servico`: pré-pago, pós-pago…) não são os *grupos de cliente* (Residencial, Empresarial, NFCom, NF 62…).

**Conferir cadastros** (`POST …/registration-check`, lotes de até 25 linhas, somente leitura): para cada linha consulta a HubSoft (por CPF/CNPJ ou `id_cliente`; se o CPF não bater, pelo login) e compara nome, CPF/CNPJ, telefone, e-mail, nascimento, RG, endereço, plano, status, vendedor, interface, valor, data da venda, carnê, referência, login e senha. Texto é comparado sem caixa/acento/espaços; **login e senha são exatos**. O que a consulta da HubSoft não devolve (ex.: vencimento, forma de cobrança) aparece como «não verificável». Dois modos (`"mode": "especifica" | "completa"`): **Específica** (nome, CPF/CNPJ, login, senha, valor e velocidade do plano — extraída do nome do plano, com até 50 Mbit/s de margem) e **Completa** (todos os campos). O resultado classifica cada cadastro em *tudo certo*, *divergente*, *não encontrado* ou *ambíguo*, com detalhe arquivo × HubSoft e exportação das divergências em CSV.

**Corrigir senhas** (`GET …/password-fix/scan`, `POST …/password-fix/apply`): varre a base (serviços não cancelados) atrás de senhas no formato `="…"` (resíduo de CSV exportado do IXC/Excel) e, com confirmação, troca pelo conteúdo. Cada serviço é relido antes e só é alterado se ainda estiver nesse formato; a auditoria guarda apenas os ids.

**Conferir endereços** (`GET …/address-check/scan[?cancelados=1]`): lê `/cliente/todos` com as relações de endereço e classifica cada serviço como `fiscal_diferente` (só o fiscal é outro), `instalacao_diferente` («2º ponto») ou `outra`. O que **não** pode divergir é o endereço **fiscal**; instalação diferente costuma ser legítima. A tela filtra por padrão (por omissão, **instalação diferente com os outros 3 iguais entre si**) e o CSV traz o endereço da instalação como endereço-alvo. **Não há como sincronizar/editar endereço pela API:** a documentação oficial (docs.hubsoft.com.br, conferida) só aceita `endereco_instalacao` em *Cadastrar Cliente Serviço* e *Migrar Cliente Serviço* (ambos criam um serviço novo) e *Editar Cliente Serviço*/*Editar Cadastro* não têm campos de endereço — para igualar os 4 endereços use a função «sincronizar endereços» da própria HubSoft.

**IXC: logins** (`POST …/ixc/logins/preview` e `…/apply`, permissão `integrations.ixc_logins`): a lista (CSV de logins) é conferida **somente leitura** (*pronto*, *já inativo*, *não encontrado*, *ambíguo*); o `apply` inativa (`ativo = N`) ou reativa os ids informados, relê antes e depois e grava no histórico o **estado anterior** (para reverter). O próprio IXC derruba a sessão PPPoE do login inativado. O `PUT /radusuarios/{id}` reenvia o registro **completo** lido antes, trocando só `ativo` (`S`/`N`), e a gravação é conferida relendo o login.

**Relatório → Desbloqueio preventivo / Tempo de cliente:** o primeiro conta quantos serviços têm «desbloqueio preventivo» e quantas vezes (a HubSoft só informa isso por cliente, então a consulta corre em lotes com progresso); o segundo mostra clientes ativos por faixa de data da venda. Na aba **Ordens de serviço**, a **Conferência** cruza cada O.S. do período com status de conexão, acesso remoto (HTTP/HTTPS) e IPv6 do cliente. Roda como **tarefa em segundo plano** — `POST …/hubsoft/conference` devolve um `job_id` e a tela consulta `GET …/hubsoft/conference/{jobId}` mostrando uma barra de progresso (de 5 em 5 %, nunca regride). Por omissão pede à HubSoft só as O.S. **finalizadas** (`only_finished`) e mostra data/usuário de fechamento e o motivo; datas começam em *hoje* (fuso local). Até 150 clientes distintos os dados de conexão vêm de uma consulta por cliente; acima disso, da varredura completa da base. No cartão de um cliente (Consulta), as abas *Financeiro*, *Atendimentos* e *O.S.* carregam sozinhas ao abrir e a aba *Identificação* mostra os dados cadastrais completos.

Usa os endpoints `/todos` da HubSoft (paginação real) para os relatórios por período; a Consulta usa `/cliente` (a API não pagina esse endpoint — devolve até 100 e avisa quando o resultado bate no teto). Resultados de Atendimentos/O.S./Financeiro (Dashboard) ficam em cache no servidor (Redis) por alguns minutos — "Carregar dados ao iniciar o sistema" (Configurações → Integrações → HubSoft) aquece esse cache no arranque. Todos os relatórios HubSoft estão também no catálogo de **Automações** (ver abaixo).

---

### POPs (`/pops`)

**Função:** pontos de presença (sites físicos).

**Como funciona:** CRUD em `pops`; associação em massa de equipamentos; contactos por POP; coordenadas usadas no **Mapa** e na correlação de incidentes «POP offline».

---

### Equipamentos (`/devices`)

**Função:** inventário central da rede.

**Como funciona:** cadastro com categoria (OLT, MikroTik, router, etc.), IP, SNMP community, POP, coordenadas, `max_pons` (OLT). Importação CSV. Por equipamento:

- **Relatório** — modal com ping, telemetria, interfaces, alertas (export CSV/PDF).
- **Coleta manual** — ping, telemetria SNMP, refresh interfaces.
- **SNMP walk** — descoberta de OIDs / inventário.
- **Backup de config** — texto guardado em `device_config_backups`.

Estado operacional vem de `device_probe_cache` atualizado pelo worker.

---

### Clientes (`/commercial`)

**Função:** base comercial mensal (localidades, clientes activos, churn).

**Como funciona:** `commercial_localities` e `commercial_monthly_records`; agregados e comparação mês a mês; exportação de relatórios; envio Telegram opcional. Dados introduzidos manualmente ou importados — não dependem da coleta SNMP.

---

### Conexões / Elementos (`/connections`)

**Função:** cadastro de assinantes e infraestrutura FTTH.

**Como funciona:** `client_connections` (login, cliente, plano, coordenadas, CTO/porta). Aba de infraestrutura: projetos, CTOs, cabos, postes, caixas de emenda e **POPs**. Importação CSV; pesquisa; integração ERP. Pontos no **Mapa**.

---

### Alertas (`/alerts`)

**Função:** fila operacional de problemas activos e histórico.

**Como funciona:** lista `GET /alerts/active` (exclui ignorados); filtros por severidade e tipo; estatísticas 24 h; incidentes correlacionados; menu ⋮ por linha (Verificar / Ignorar); botões **Verificar alertas** e **Alertas ignorados**. Histórico em `/alerts/history`. Refresh automático ~2,5 s com a página aberta.

Em alertas de **OLT offline** (`ping_unreachable` numa OLT) ou **PON DOWN**, o menu ⋮ ganha **Clientes afetados**: um modal com os clientes ligados às ONUs no alcance do alerta (a OLT inteira ou só a PON caída, casando serial ↔ `onu_client_links`), com exportar CSV e enviar a lista pelo Telegram (`GET /alerts/{id}/affected-clients`, `POST …/affected-clients-telegram`).

---

### Mapa (`/map`)

**Função:** visualização geográfica da rede e da infra FTTH.

**Como funciona:**
- Equipamentos, logins, **POPs**, CTOs, cabos, postes e caixas de emenda.
- `GET /map/equipment-points`, `/map/connection-points`, `/map/search`.
- Filtros em modal; detalhe em painel lateral.
- Cores/ícones de mapa em `settings_ui` (globais).

---

### Topologia do POP (`/pops/:popId/rack`)

**Função:** diagrama do rack de um POP — equipamentos e as suas portas ligadas por fibra.

**Como funciona:** cada equipamento mostra as suas interfaces numa grelha que só quebra linha depois de **50 portas**. Cada porta tem um tipo — **SFP**, **SFP+**, **Ethernet /100**, **Ethernet /1000** ou **PON** (ícone de sol) — que define o ícone no diagrama.

---

### Topologia (`/topology`)

**Função:** diagrama de rede desenhado **manualmente** (equipamentos, agrupamentos por POP, ligações tipadas) — deliberadamente sem descoberta automática (LLDP na tela BGP é só informativo, não alimenta este diagrama).

**Como funciona:** canvas React Flow com equipamentos arrastados da lista lateral, agrupadores "POP" (quadrado ou círculo, redimensionáveis, nome editável) e ligações entre equipamentos de 4 tipos (fibra, transporte, rádio, UTP/VPN), cada uma com cor/traço próprio e ícone redimensionável. Liga-se a partir de qualquer um dos 4 lados de um equipamento. Desfazer/refazer (Ctrl+Z / Ctrl+Y, com histórico agrupando gestos contínuos como arrastar/redimensionar num só passo) e remoção de um único equipamento, POP ou ligação (apagar um POP desagrupa os equipamentos, não os remove). Documento único gravado em `GET/PUT /api/v1/topology`.

---

### Ferramentas (`/tools`)

**Função:** diagnóstico ad hoc (sem persistir inventário).

**Como funciona:** executa no servidor (com auditoria em `ops_audit_log`):

| Ferramenta | Endpoint |
|------------|----------|
| DNS | `POST /tools/dns/run` |
| HTTP/HTTPS probe | `POST /tools/http-https-probe` |
| Ping ICMP | `POST /tools/icmp/ping` |
| Traceroute | `POST /tools/tracert` |
| Nmap | `POST /tools/nmap` |
| SNMP get / bulk / walk | `POST /tools/snmp/*` |
| Telnet / SSH teste | `POST /tools/telnet/test`, `/ssh/test` |
| MikroTik rápido | `POST /tools/mikrotik/*` |

---

### OLT (`/olt`)

**Função:** monitorização de PONs e ONUs.

**Como funciona:** lista OLTs com snapshot (`olt_snapshots`: `pons`, `summary`, totais computados). Detalhe por OLT: tabela PON (status ON/OFF derivado de ONUs online), ONUs VSOL/ZTE, interfaces, log de coleta SNMP. **Atualizar** dispara refresh pelo perfil do fabricante. Totais globais de ONUs online/offline no dashboard e nesta tela. Coleta periódica via worker (intervalo em configurações).

- **Aba ONUs** — dados que não vêm por a ONU estar offline aparecem como "-" (não valor obsoleto). A coluna RX é classificada por cor (**Boa / Aceitável / Ruim**) pelos limiares em Configurações → OLT → *Qualidade da potência RX (ONU)* (`monitoring_settings.onu_rx_good_dbm` / `onu_rx_bad_dbm`). Cada linha tem **Histórico** nos 3 pontinhos: últimas 10 colectas dessa ONU (`olt_onu_history`, `GET /olt/devices/{id}/onu-history`).
- **Aba Pesquisa de ONUs** — busca entre todas as OLTs por serial/modelo/cliente + filtros de OLT, PON, potência, temperatura, voltagem. Botão de **exportar CSV** (ícone) leva exactamente o que está filtrado (todas as páginas). O limite da consulta HubSoft-independente é a própria API/coleta.
- **Atualizar PON** — com **1 OLT e 1 PON** selecionadas na Pesquisa de ONUs aparece o botão que atualiza só essa PON (`POST /olt/devices/{id}/pons/{pon}/refresh`), via Telnet com o comando configurado por fabricante em Configurações → OLT (bloco *Atualizar ONUs de uma PON*, com o placeholder `{pon}`; um comando por linha ou separados por `;`; usa os pré-comandos do bloco 1). O resultado é mesclado no snapshot sem recoletar a OLT inteira; a OLT informa `pon_refresh_available` quando há comando configurado.
- **Aba Relatório** — histórico de ONUs por OLT (total/online/offline). Ao seleccionar **uma OLT específica**, mostra também um gráfico geral (soma de todas as PONs) e um grid de 4 colunas com o histórico de cada porta PON (`olt_pon_samples`, `GET /olt/reports/pon-history`).

---

### MikroTik (`/mikrotik`)

**Função:** interfaces, tráfego e telemetria SNMP.

**Como funciona:** lista equipamentos MikroTik; detalhe com tabela de interfaces (status, tráfego, SFP dBm), gráficos de taxa, coleta manual. Perfil de coleta SNMP configurável em **Configurações → MikroTik**. Alertas SFP gerados automaticamente no ciclo de interfaces.

---

### Switch (`/switch`)

**Função:** telemetria e interfaces de switches (IF-MIB, VLAN por porta quando o perfil inclui).

**Como funciona:** mesma casca de monitorização que MikroTik/BNG; coleta configurável em **Configurações → Switch**.

---

### BGP (`/bgp`)

**Função:** monitorização de equipamentos com sessões BGP (tipicamente a borda/edge da rede) — peers, tráfego por operadora, saúde do hardware.

**Como funciona:** equipamentos com `bgp_enabled`; perfis SNMP próprios em **Configurações → BGP** (múltiplos perfis nomeados, ao contrário do perfil único do BNG). Abas:

| Aba | Conteúdo |
|-----|----------|
| **Visão geral** | Peers estabelecidos, saúde básica (CPU/memória/uptime), alertas de sessão, tráfego por operadora |
| **Peers** | Tabela de peers BGP (estado, AS remoto, prefixos recebidos/activos/anunciados) e sessões BFD |
| **Interfaces & LAG** | Interfaces IF-MIB e E-Trunk (LAG entre equipamentos: master/backup e motivo real da troca) |
| **Óptica** | Diagnóstico por porta: potência Rx/Tx, corrente do laser, temperatura, tensão |
| **CPU & Memória** | CPU por núcleo (actual + médias 1/5 min) e CPU/memória da Virtual System |
| **Saúde do Chassi** | Semáforo de alarme por placa, ventoinhas, fontes, temperatura e tensão por sensor |
| **QoS** | Descarte por fila/classe (HQoS/CBQoS) |
| **RADIUS** | Saúde por servidor RADIUS (pode não estar visível em VS dedicadas só a BGP) |
| **LLDP** | Vizinhos LLDP por porta — só informativo, não alimenta a Topologia |

O tráfego (Visão geral) mostra sempre 1 gráfico **TOTAL geral** (soma de todas as operadoras) no topo, seguido de 1 gráfico dedicado por operadora — cada um com toggle Separado/Somado próprio quando a operadora tem 2+ interfaces (`GET /api/v1/bgp/devices/{id}/carrier-traffic-history`), selector de período (24 h a 300 dias ou intervalo específico) e teto do eixo Y a partir do limite de banda cadastrado.

Operadoras são um cadastro próprio (`bgp_carriers` — nome, CNPJ, endereço, 1+ AS, limite de banda; `bgp_carrier_as_numbers` para os AS), gerido em **Configurações → BGP → Operadoras**; ligar uma interface a uma operadora sempre escolhe de uma lista já cadastrada (nunca texto livre), com atalho para cadastrar/editar/remover operadoras sem sair do formulário.

---

### BNG / PPPoE (`/bng`)

**Função:** operação do concentrador: sessões PPPoE, autenticações, interfaces, relatório e catálogo de VLANs.

**Como funciona:** equipamentos com categoria BNG; coleta SNMP de sessões (`bng_session_snapshots`, `bng_known_logins`). Abas:

| Aba | Conteúdo |
|-----|----------|
| **Visão geral** | Totais PPPoE/IPv4/IPv6, gráfico histórico, pools e RADIUS |
| **Relatório** | Tempo online, tráfego agregado, infra e CGNAT (sem a lista de VLANs) |
| **VLANs** | Catálogo da rede (`network_vlans`): tipo **PPPoE / Gerência / Transporte**, status, capacidade, conexões online, equipamentos e utilização. VLANs vistas nas sessões aparecem automaticamente até serem guardadas. `GET/POST /api/v1/network-vlans` |
| **Interfaces** | Snapshot IF-MIB do BNG |
| **Autenticações** | Tentativas recentes |
| **Sessões PPPoE** | Lista pesquisável, filtros avançados, detalhe e coleta SNMP |

---

### Eventos da Rede (`/events`)

**Função:** histórico estruturado de manutenções e incidentes (não é texto livre).

**Como funciona:** catálogo com ~12 categorias e tipos estáveis (`networkevents`) gravados em `network_events`. Campos contextuais (POP, equipamento, interface, projeto FTTH, CTO, cabo, poste, VLAN, técnico). A interface do equipamento pode ser escolhida da lista SNMP ou em texto livre («Outros»). Permissões: `network_events.view` / `network_events.manage`. API: `/api/v1/network-events`.

O endpoint legado `GET /api/v1/events` continua a ser a linha do tempo de sistema (`alert.opened` / `alert.closed` / `device.checks`).

---

### Registros (`/registros`)

**Função:** cofre de senhas de acesso (equipamento, servidor ou site). Todos os usuários autenticados vêem o menu; cada um só acede aos **seus** registos. Administradores vêem **todos** e filtram por pessoa. Não está na tela de Usuários.

**Como funciona:** cada registo pertence a um usuário. Pode guardar usuário+senha ou só a senha. Equipamento escolhe-se da lista; servidor pede IP/host; site pede domínio. Senhas cifradas em AES-GCM (`credential_records.password_blob`); revelação pontual em `POST /api/v1/credential-records/{id}/reveal` (com auditoria). API: `/api/v1/credential-records`.

---

### Relatórios (`/reports`)

**Função:** relatórios do sistema (exceto frota), resumidos ou detalhados, com CSV / PDF / Telegram.

Inclui: alertas activos, alertas por categoria, equipamentos em atenção, PONs DOWN, saúde do monitoramento, conexões, OLT, ONUs por PON, BNG, **BGP** (peers established/caídos e operadoras cadastradas), **eventos de rede**, infraestrutura FTTH (POPs, projetos, CTOs, cabos, postes, emendas), equipamentos por POP, visão geral, integrações, **HubSoft** (atendimentos/O.S./financeiro dos últimos 30 dias), automações e base comercial.

---

### Frota (`/fleet/…`)

**Função:** veículos, motoristas, despesas, alertas e relatórios da frota operacional. Independente dos relatórios de rede.

---

### Configurações (`/settings`)

Qualquer usuário autenticado acede às preferências pessoais. As restantes abas exigem `settings.*` ou admin.

| Secção | Função |
|--------|--------|
| **Alertas** | **Pessoal:** toast em qualquer ecrã, som de alerta (4 sons padrão + MP3). **Global** (se tiver permissão): limiares CPU/temp/SFP/OLT/BNG |
| **Aparência** | Tema claro/escuro **por usuário** (`users.preferences`) |
| **Base de dados** | DSN, teste, **limpeza de dados históricos** (apaga e devolve o espaço ao disco — ver abaixo), backup B2 |
| **Usuários** | CRUD, perfis de permissão e «Forçar desconexão» (invalida a sessão de outro usuário — efeito quase imediato, a app já verifica a sessão a cada poucos segundos) |
| **Monitoramento** | Intervalos, timeouts, modo, pipeline |
| **OLT / MikroTik / Switch / BNG / BGP** | Perfis por marca/modelo, coleta e (BGP) cadastro de operadoras (CNPJ, AS, limite de banda). A aba **OLT** tem ainda os limiares de *Qualidade da potência RX (ONU)* (dBm "boa" / "ruim") usados na tabela de ONUs e no Dashboard |
| **Telegram** | Bot monitoring e relatórios |
| **Automações** | Backup, digest de alertas, ONU mensal, totais BNG, base comercial e **Coleta de ONUs (OLT)** — mais **Automações personalizadas** (ilimitadas, qualquer relatório do sistema ou de frota, recorrência própria) |
| **Auditoria** | `ops_audit_log` |

Preferências por conta (novos usuários: toast e som ligados): tema, `alert_toast_everywhere`, `alert_sound_enabled`, som escolhido. API: `GET/PATCH /api/v1/me/preferences`.


**Limpeza de dados históricos (Configurações → Base de dados).** `DELETE` no PostgreSQL só marca linhas como mortas: o arquivo da tabela não encolhe e o disco do servidor continua cheio. A limpeza escolhe, **por tabela**, uma estratégia que de fato devolve espaço: *esvaziar* (`TRUNCATE`, se tudo é antigo), *reescrever só o que fica* (copia o recente, `TRUNCATE` e devolve — quando ≥ 50% é antigo) ou *lotes + `VACUUM`* (+ `VACUUM FULL` se houver folga de disco). Roda em **segundo plano** (progresso, resultado por tabela e disco livre na própria janela), uma por vez, com mínimo de 1 dia, e respeita alertas ainda abertos. A caixa «Devolver o espaço ao disco» pode ser desmarcada (só apaga). O espaço livre vem do disco do servidor (`statfs` no volume de dados; no Compose é o mesmo do Postgres).

Fora do painel, no servidor (mesma lógica; útil quando o disco está cheio demais para abrir a tela):

```bash
bash scripts/purge-old-data.sh 30            # tudo com mais de 30 dias (pede confirmação)
bash scripts/purge-old-data.sh 1 --yes --tables=ping_history   # só o histórico de ping, mantendo 1 dia
bash scripts/purge-old-data.sh 30 --dry-run  # só mostra o que seria apagado
```

A retenção **automática** (`history_retention_days` em `monitoring_intervals`, padrão 90) apaga em lotes mas não encolhe o arquivo; com ~1,2 GB/dia de `ping_history` use um valor baixo (2–7 dias).

---

## Notificações e automações

### Toast e som no browser

Cada usuário liga/desliga em **Configurações → Alertas**:
- **Toast em qualquer ecrã** — se desligado, o aviso só aparece em Monitoramento e Alertas.
- **Som de alerta** — sons padrão ou MP3 próprio (`/api/v1/me/alert-sounds`).

O watcher global reage a `monitoring_runtime.last_alerts_change_at` (offline, PON, SFP, temperatura, latência, etc.).

### Telegram

- **Monitoring** — cada alerta novo e resolução (quando configurado).
- **Relatórios** — ONU mensal, resumo de alertas, totais BNG, base comercial.

### Automações agendadas

Em **Configurações → Automações**: os 6 cadastros fixos — backup PostgreSQL (B2), digest de alertas, relatório ONU, totais BNG, base comercial e **Coleta de ONUs (OLT)** — mais **Automações personalizadas** (`automation_schedules`, tabela própria, sem limite de instâncias): cada uma escolhe qualquer relatório do catálogo `/api/v1/reports/system` ou um relatório de frota/combustível, com recorrência diária/semanal/dias específicos/mensal e janela de dados própria — enviado pelo bot Telegram "reports". Mesmo motor de agendamento (`scheduleutil`) dos cadastros por dia/hora, executando a cada verificação de 30 s do worker. Histórico em `automation_execution_log`.

**Coleta de ONUs (OLT)** (`automation_olt_onu_collection`, migração 155) corre por **intervalo**, em segundo plano, sobre todas as OLTs, em duas cadências independentes: **leve** (status das ONUs/PONs + RX, padrão a cada 5 min — modo `status_rx`) e **completa** (serial, temperatura, TX, modelo e telnet, padrão a cada 6 h — modo `full`, com a fase de completar serial por PON). Quando as duas vencem juntas roda só a completa. Cada OLT é consultada uma vez de cada vez (`snmpdevicelock`). O cartão tem liga/desliga, intervalos, «Executar leve/completa agora» e o resultado da última execução por OLT. API: `GET/PATCH /settings/automation/olt-onu-collection` e `POST …/run` (`{"kind":"light"|"full"}`). Vem **desligada** por padrão.

O catálogo de relatórios do sistema inclui alertas, BGP, OLT/BNG e **HubSoft**: `hubsoft-overview` (combinado), `hubsoft-services-by-plan` / `-by-locality` / `-full`, `hubsoft-work-orders-period` (com ranking por técnico) e `hubsoft-attendance-period` — cada um agendável de forma independente.

---

## Autenticação e API

- **Login UI** — `POST /auth/login` → JWT (`NETQUASAR_SESSION_SECRET`).
- **API keys** — cabeçalho `X-API-Key` (`NETQUASAR_API_KEYS`).
- Permissões por perfil (`permission_profiles`); mutações e coletas exigem a chave correspondente ou admin.
  Chaves ligadas à HubSoft/IXC: `integrations.hubsoft_bulk` (edições em massa na HubSoft) e `integrations.ixc_logins` (inativar logins no IXC). O catálogo fica em `api/permission_catalog.go` e `quasar_frontend/src/lib/permissions.ts`.
- Preferências do usuário autenticado: `/api/v1/me/preferences`.

Health: `GET /health` · Métricas Prometheus: `GET /metrics`

---

## Estrutura do repositório

```text
NetQuasar/
├── quasar_backend/
│   ├── cmd/netquasar/       # Servidor HTTP + worker
│   ├── cmd/migrate/         # Migrações SQL (goose)
│   ├── internal/
│   │   ├── api/             # Handlers REST
│   │   ├── monitorworker/   # Ciclos de coleta
│   │   ├── alertthresholds/ # Limiares e avaliação
│   │   ├── alertignore/     # Ignorar alertas (persistência)
│   │   ├── alertverify/     # Verificação manual
│   │   ├── alertnotify/     # Telegram
│   │   ├── alertcorrelation/# Incidentes
│   │   ├── oltcollect/      # Perfis e coleta OLT
│   │   ├── integrationhubsoft/ # Cliente da API HubSoft: relatórios, importação em massa (bulk_import_*.go),
│   │   │                    #   conferências (conference.go, registration_check.go, address_check.go), correção de senhas
│   │   ├── integrationixc/  # Cliente IXC para inativar/reativar logins (radusuarios)
│   │   ├── oltifderive/     # Derivação PON/ONU via IF-MIB
│   │   ├── bgpcollect/      # Catálogo SNMP e pivot de dados BGP (peers/óptica/hardware/…)
│   │   ├── networkevents/   # Catálogo de eventos de rede
│   │   └── db/migrations/   # Esquema PostgreSQL
│   └── data/mibs/           # MIBs SNMP de referência
├── quasar_frontend/         # SPA React
├── deploy/linux-debian/
├── docker-compose.yml
└── Dockerfile
```

---

## Desenvolvimento local

### Requisitos

| Ferramenta | Versão |
|------------|--------|
| Go | 1.24+ |
| Node.js | 20+ LTS |
| PostgreSQL | 16 |
| Docker (opcional) | Compose para postgres + redis |

### Arranque rápido

```powershell
# Base (Compose)
cp .env.example .env
docker compose up -d postgres redis

# Backend
cd quasar_backend
go run ./cmd/migrate/
go run ./cmd/netquasar/

# Frontend (outro terminal)
cd quasar_frontend
npm install
npm run dev
```

| Serviço | URL |
|---------|-----|
| Frontend (dev) | http://localhost:5173 |
| API | http://localhost:8080 |

Atalho Windows: `iniciar-netquasar-dev.bat`

### Comandos úteis

```powershell
cd quasar_frontend && npm run typecheck && npm run build
cd quasar_backend && go build ./... && go test ./... -short
cd quasar_backend && go run ./cmd/dbping/
```

---

## Deploy com Docker

```bash
cp .env.example .env
# Editar POSTGRES_PASSWORD, NETQUASAR_SESSION_SECRET, etc.
bash scripts/verify-compose-env.sh
docker compose up -d --build
```

UI + API na porta `NETQUASAR_PUBLISH_PORT` (padrão `8080`), com `NETQUASAR_EMBEDDED_UI=true`.

Guia completo: [deploy/linux-debian/README.md](deploy/linux-debian/README.md)

---

## Configuração inicial

1. Aceder à UI → login ou `/config-setup` se a base estiver vazia.
2. **Configurações** (admin) — intervalos, Telegram, perfis OLT, usuários.
3. Cadastrar **POPs** e **Equipamentos** (com IP e SNMP).
4. **Monitoramento** → iniciar modo **Full**.
5. Ajustar **limiares de alerta** em Configurações.
6. (Opcional) Importar **Conexões** CSV e activar logins no mapa.

### Variáveis principais (`.env`)

| Variável | Descrição |
|----------|-----------|
| `NETQUASAR_DATABASE_URL` | DSN PostgreSQL |
| `NETQUASAR_SESSION_SECRET` | Segredo JWT (produção) |
| `NETQUASAR_API_KEYS` | Chaves API |
| `NETQUASAR_REDIS_URL` | Redis para WebSocket tempo real e cache do dashboard (no Compose: `redis://redis:6379/0`) |
| `NETQUASAR_PUBLISH_PORT` | Porta HTTP no Compose |
| `NETQUASAR_LOG_LEVEL` | `debug`, `info`, `warn`, `error` |

---

## Mapa de rotas (SPA)

| Rota | Módulo |
|------|--------|
| `/dashboard` | Dashboard |
| `/monitoring` | Monitoramento |
| `/realtime` | Tempo real |
| `/integrations` | Integrações |
| `/pops` | POPs |
| `/devices` | Equipamentos |
| `/commercial` | Clientes |
| `/connections` | Conexões PPPoE/DHCP |
| `/alerts` | Alertas |
| `/map` | Mapa |
| `/topology` | Topologia (diagrama de rede manual) |
| `/tools` | Ferramentas |
| `/olt` | OLT |
| `/mikrotik` | MikroTik |
| `/switch` | Switch |
| `/bgp` | BGP (peers, tráfego por operadora, hardware) |
| `/bng` | BNG / sessões PPPoE / VLANs |
| `/events` | Eventos da Rede (manutenções e incidentes) |
| `/registros` | Cofre de senhas (próprios; admin vê todos) |
| `/reports` | Relatórios do sistema |
| `/fleet/dashboard` | Frota |
| `/about` | Sobre / FAQ |
| `/settings` | Configurações e preferências pessoais |

Redireccionamentos legados: definidos em `LEGACY_ROUTE_REDIRECTS` (`routes.ts`) e aplicados automaticamente no `AppRouter` (ex.: `/alertas` → `/alerts`, `/comercial` → `/commercial`).

---

## App no celular (PWA)

O NetQuasar é um **PWA instalável** (Android e iPhone): ícone na tela inicial, tela cheia, aviso de «Nova versão» e tela de «Sem conexão». Arquivos: `quasar_frontend/public/manifest.webmanifest`, `public/sw.js`, `public/offline.html`, `public/pwa/*` (ícones) e `src/pwa/*`. O service worker **nunca** guarda `/api/*` em cache. Para instalar: abra o endereço público no Chrome do Android → menu ⋮ → **Instalar app** (no iPhone: Safari → Compartilhar → **Adicionar à Tela de Início**); se havia um atalho antigo, remova-o antes. O plano de evolução (notificações push de alertas, interface mobile e ordem das telas) está em [`README-Mobile.md`](README-Mobile.md).

---

## Segurança

- Não commitar `.env`, certificados (`*.crt`) nem credenciais.
- Palavras-passe fortes em Postgres e `NETQUASAR_SESSION_SECRET` (também deriva a chave do cofre de Registros).
- Em produção: firewall + reverse proxy com TLS.
- Codificar caracteres especiais no DSN (`%2C`, `%24`, …).

---

## Resolução de problemas

| Sintoma | Verificar |
|---------|-----------|
| Frontend sem API | Proxy Vite / mesma origem em produção |
| Autenticação Postgres falha | DSN, password URL-encoded, credenciais Supabase |
| Docker + Supabase | Usar **Session pooler** (IPv4) — ver README-BACKEND |
| Interfaces MikroTik incompletas | Aumentar `interface_snapshot_timeout_ms` |
| OLT sem atualizar PONs | Monitoramento **Full** ligado; intervalo `olt_if_derived_pon_seconds`; equipamento online |
| Alertas não no Telegram | Configurações → Telegram monitoring; bot/chat correctos |
| Migrações em falta | `go run ./cmd/migrate/` |

---

## Licença

Consulte os ficheiros de licença no repositório. Evolução de longo prazo: [ROADMAP-ARQUITETURAL-DEPLOY.md](ROADMAP-ARQUITETURAL-DEPLOY.md).

# NetQuasar — Diagnóstico de Performance e Arquitetura

*Levantamento feito em 26/08/2026, a partir do código em `quasar_backend` (Go) e `quasar_frontend` (React/TS), com foco nos dois pontos que você definiu: performance/escalabilidade e qualidade de código/arquitetura.*

*Atualizado em 26/08/2026: itens 1–4 (performance/escalabilidade) foram implementados e aplicados no repositório, mais uma melhoria específica para detecção ágil de queda de ONUs (não estava na lista original — pedida à parte). Ver "O que foi implementado" e "Detecção ágil de queda de ONUs" mais abaixo. Itens 5–8 (arquitetura/qualidade) continuam como diagnóstico, ainda não implementados.*

*Atualizado em 26/08/2026 (2): investigado e corrigido o bug relatado na tela de Mapa (CTOs e foguetes não apareciam, exceto os 3 mais próximos via GPS). Causa raiz e correção na seção "Tela de Mapa: CTOs e foguetes não aparecem" mais abaixo.*

## Resumo executivo

O projeto está bem estruturado onde mais importa: o worker de monitoramento (`internal/monitorworker`) é decomposto em ~50 arquivos pequenos e focados, usa `errgroup` com limite de concorrência configurável e serializa por equipamento via lock (`snmpdevicelock`), o que é o padrão certo para esse tipo de carga. Os índices nas tabelas de série temporal (`ping_history`, `telemetry_samples`, `interface_snapshots`) também estão corretos (`device_id, tempo DESC`).

O problema mais sério que encontrei não é de performance no sentido de "lento" — é um teto de escala escondido: várias queries centrais (incluindo as duas que alimentam o próprio ciclo de ping/telemetria do worker) têm `LIMIT 500` fixo, sem paginação. Isso significa que, a partir de 500 equipamentos cadastrados, uma parte da rede simplesmente para de ser monitorada, silenciosamente — sem erro, sem alerta, sem log. Esse ponto deveria ser o primeiro a resolver, porque é o que mais rapidamente vira um incidente real em campo (equipamento offline sem ninguém saber).

No lado de arquitetura, o backend está com pacotes muito desbalanceados: `internal/api` tem quase 39 mil linhas em 108 arquivos, com vários handlers passando de 1000–2000 linhas (fazem parsing, validação, SQL e regra de negócio no mesmo arquivo). O frontend tem o problema espelhado: páginas como `BngPage.tsx` (2517 linhas) ou `OltVendorsPanel.tsx` (2304 linhas) concentram busca de dados, estado e UI inteira num único componente. E não há nenhuma automação de teste no frontend (sem Vitest/Jest, zero arquivos `*.test.*`), enquanto o backend tem uma cobertura razoável (113 arquivos de teste para 443 arquivos `.go`, bem concentrada no `monitorworker`, que é a parte mais crítica).

## O que foi implementado (26/08/2026)

Todas as correções de performance/escalabilidade (itens 1–4 abaixo) foram implementadas e aplicadas diretamente no seu repositório, mais 3 migrações novas (`110_sweep_concurrency.sql`, `111_history_retention.sql`, `112_olt_parallel_cycle.sql`) e a melhoria de detecção ágil de queda de ONUs (pedida à parte, ver seção própria). Cada arquivo Go alterado foi verificado com `gofmt` (sintaxe válida) e o novo código foi exercitado num benchmark isolado e contra um Postgres local descartável — não o Supabase de produção do `.env`. **Importante:** não consegui rodar `go build ./... && go test ./... -short` nem `npm run typecheck` completos neste ambiente (o proxy de módulos Go — `proxy.golang.org` — e o registro npm estão fora da lista de rede liberada aqui, e o `tsc` completo não terminou dentro do limite de tempo do bridge com o seu computador). Recomendo fortemente rodar os dois antes de subir para produção:

```powershell
cd quasar_backend && go build ./... && go test ./... -short
cd quasar_frontend && npm run typecheck && npm run build
```

E depois aplicar as 3 migrações novas: `cd quasar_backend && go run ./cmd/migrate/`.

## Achados de performance / escalabilidade

### 1. `LIMIT 500` sem paginação nas queries centrais (crítico) — ✅ Implementado

`internal/monitorworker/devices_for_monitor.go` define `loadPingableDevices` e `loadTelemetryDevices` — as funções que decidem *quais equipamentos o worker sonda* a cada ciclo de ping e de telemetria. Ambas terminam a query com `ORDER BY d.description LIMIT 500`, sem offset e sem qualquer sinalização de que existem mais linhas. O mesmo padrão aparece em `internal/api/handlers_devices.go` (`listDevices`, a listagem usada na tela **Equipamentos**) e em outros oito handlers (`handlers_credential_records.go`, `handlers_fleet_fuelings.go`, `handlers_ops_features.go`, `handlers_system_reports.go`, `handlers_commercial.go`, `handlers_suppressions.go`, `handlers_system_reports_extra.go`).

Na prática: um provedor com mais de 500 equipamentos ativos (bem plausível considerando OLTs, MikroTiks, switches, BNGs e POPs de uma rede de porte médio) vai ter uma fatia da rede — sempre a mesma, porque a ordenação é por `description` — fora do ciclo de ping e telemetria. Nenhum alerta de "equipamento nunca monitorado" existe para pegar esse caso, porque do ponto de vista do worker esses equipamentos simplesmente não existem na consulta.

**Correção aplicada:** as 4 funções em `devices_for_monitor.go` (`loadPingableDevices`, `loadTelemetryDevices`, `loadOltDevicesForCollect`, `loadBngDevicesForCollect`) trocaram `LIMIT 500`/`LIMIT 100` fixo por `maxMonitorDevicesPerSweep = 20000` — um teto de segurança contra cenário patológico, não um limite operacional (nenhum inventário real de ISP deve chegar perto disso). Os 8 handlers de listagem em `internal/api` tiveram o `LIMIT` elevado de 500 para 2000–5000 (mesmo padrão já usado em `client_connections`/`handlers_network_infrastructure.go`). Testei com Postgres local (não o de produção) semeado com 5.000 e depois 20.000 equipamentos sintéticos: a query antiga devolvia exatamente 500 mesmo com 5.000 na base (bug confirmado); a corrigida devolve os 5.000/20.000, com `EXPLAIN ANALYZE` em ~8,7ms (5k linhas) e ~21ms (20k linhas) — sem precisar de índice novo, mesmo nesse volume.

### 2. Concorrência do worker é baixa por padrão e não é visível na UI — ✅ Implementado

`internal/monitorworker/sweep_concurrency.go` define `DefaultSweepConcurrency = 6`, ajustável só por variável de ambiente (`NETQUASAR_SWEEP_CONCURRENCY`, teto 32) — não há campo em **Configurações → Monitoramento** para isso. Com 6 sondas simultâneas, uma varredura de milhares de equipamentos (ping ou SNMP, cada um com timeout de alguns segundos) pode facilmente levar minutos por ciclo, o que empurra o sistema para trás do intervalo configurado (`ping_seconds`/`pipeline_cycle_seconds`) sem que isso apareça de forma clara na tela de Monitoramento. Vale expor esse parâmetro na UI (com um teto sensato) e, mais importante, expor no painel de Monitoramento quanto tempo o último ciclo *de fato* levou vs. o intervalo configurado — hoje dá pra inferir só por `last_pipeline_cycle_at`, mas não há alerta se o ciclo está "atrasando".

**Correção aplicada:** default subido de 6 para 12 (teto de 32 para 64), com base num benchmark de simulação (ver "Testes de stress" abaixo). Além da variável de ambiente `NETQUASAR_SWEEP_CONCURRENCY` (que continua funcionando, com prioridade — é a válvula de escape operacional), a concorrência agora também é lida de uma coluna nova `monitoring_intervals.sweep_concurrency` (migração `110_sweep_concurrency.sql`, 0 = usar o default do código) — falta só um campo na UI de Configurações → Monitoramento para editá-la sem SQL direto (o backend já está pronto para isso).

### 3. Pool de conexões Postgres fixo e sem retenção automática de histórico — ✅ Implementado

`internal/db/db.go` fixa `MaxConns = 32` / `MinConns = 2` no código, sem variável de ambiente — se a API e o worker competirem por conexões sob carga (muitos usuários no NOC + varredura full rodando), não dá para ajustar sem recompilar. Seria simples expor via `.env` (`NETQUASAR_DB_MAX_CONNS`).

Separadamente, `ping_history`, `telemetry_samples` e `interface_snapshots` crescem indefinidamente — existe uma tela de limpeza manual (`handlers_database_cleanup.go`, **Configurações → Base de dados**), mas nenhuma automação agendada de retenção (as automações existentes são backup, digest de alertas, relatório de ONU, totais BNG e base comercial — nenhuma delas é "purgar histórico antigo"). Com o tempo isso tende a inflar o tamanho do banco e degradar consultas de relatório que varrem essas tabelas. Vale um job de retenção configurável (ex.: manter 90 dias de `ping_history` bruto e agregar o resto).

**Correção aplicada:** `NETQUASAR_DB_MAX_CONNS`/`NETQUASAR_DB_MIN_CONNS` agora existem e têm precedência sobre os valores fixos (32/2 continuam sendo o default se as variáveis não forem definidas). E foi adicionado um job de retenção automática (`internal/monitorworker/retention.go`): roda no máximo uma vez por dia, em lotes de 10 mil linhas (para não segurar locks longos nem competir pelo pool), purgando `ping_history`/`telemetry_samples`/`interface_snapshots` mais antigos que `monitoring_intervals.history_retention_days` (migração `111_history_retention.sql`, default 90 dias; 0 desliga — mantém o comportamento antigo de limpeza só manual). Também falta só o campo na UI para ajustar esse número; por SQL direto (`UPDATE monitoring_intervals SET history_retention_days = 90 WHERE id=1`) já funciona hoje.

### 4. Frontend sem virtualização de listas grandes — ✅ Implementado (paginação)

Não encontrei `react-window`, `react-virtual` ou equivalente no frontend. Telas como Equipamentos, Conexões (`client_connections`, que tem `LIMIT 5000` em cinco handlers de `handlers_network_infrastructure.go`) e o Mapa renderizam potencialmente milhares de linhas/pontos direto no DOM. Combinado com o ponto 1 (falta de paginação real), isso é o tipo de coisa que só aparece como problema quando a base de equipamentos/conexões cresce — vale antecipar antes que vire ticket de "tela travando" en produção.

**Correção aplicada (Equipamentos, como exemplo a replicar):** como `npm install` não funciona neste ambiente (registro npm bloqueado), optei por não adicionar `react-window` como dependência nova — em vez disso implementei paginação client-side simples em `DevicesPage.tsx` (`DEVICES_PAGE_SIZE = 200`, com controles Anterior/Próxima no mesmo estilo já usado em `BngPage.tsx`). Isso evita renderizar todas as linhas de uma vez, sem depender de virtualização por scroll (que seria mais complexa de acertar às cegas, sem poder testar visualmente aqui). Conexões, Mapa e outras telas grandes ainda não foram tocadas — o padrão usado aqui (estado de página + slice do array já ordenado/filtrado) é direto de replicar nelas.

## Achados de arquitetura / qualidade de código

### 5. `internal/api` é um pacote monolítico

38.593 linhas em 108 arquivos, vários deles passando de mil linhas: `handlers_network_infrastructure.go` (2042), `handlers_system_reports.go` (1561), `handlers_devices.go` (1392), `handlers_client_connections.go` (1205), `handlers_bng.go` (1172), `handlers_settings.go` (1106). Cada handler mistura parsing de request, validação, SQL inline e serialização de resposta no mesmo arquivo/função. Funciona, mas dificulta teste unitário (a lógica de negócio não é isolável do `http.ResponseWriter`) e revisão de PR (diffs grandes em arquivos que já são grandes). Uma extração gradual — camada de "service"/"repository" separada do handler HTTP, começando pelos arquivos maiores — reduziria bastante o atrito para adicionar funcionalidade nova sem re-tocar arquivos de 2000 linhas.

### 6. Páginas gigantes no frontend, sem separação de dados/UI

`BngPage.tsx` (2517 linhas), `OltVendorsPanel.tsx` (2304), `MapPage.tsx` (2138), `DevicesPage.tsx` (2060), `SettingsPage.tsx` (2013) concentram fetch (react-query), estado local, formulários e renderização num único componente. O uso de `@tanstack/react-query` já é a escolha certa (cache, `refetchInterval`, invalidação) — o ganho aqui é de organização: extrair hooks de dados (`useBngSessions`, `useOltVendorProfile`, etc.) e subcomponentes de UI reduziria o escopo de re-render e tornaria essas telas testáveis isoladamente.

### 7. Zero testes automatizados no frontend

Não há Vitest, Jest, Testing Library nem Playwright no `package.json`, e nenhum arquivo `*.test.*`/`*.spec.*` em `src`. Para um painel operacional de NOC — onde uma regressão visual ou de lógica pode significar um alerta que não aparece na tela — isso é o maior risco de qualidade do projeto hoje. Não precisa ser cobertura ampla de início: começar com Vitest + Testing Library cobrindo a lógica de alertas/thresholds no frontend (onde há mais regra de negócio, não só apresentação) já reduziria bastante o risco.

### 8. ~257 erros de banco ignorados silenciosamente (`_, _ = pool.Exec(...)`)

A maioria são updates de "melhor esforço" (patch de `meta`, flags de `running=false`) onde ignorar falha é uma escolha razoável — mas hoje isso é feito sem nem logar o erro, então se uma dessas escritas começar a falhar de verdade (ex.: coluna renomeada numa migração, deadlock), não há rastro nenhum. Trocar `_, _ = pool.Exec(...)` por algo como `if _, err := pool.Exec(...); err != nil { log.Warn().Err(err)... }` custa pouco e fecha esse ponto cego.

## Detecção ágil de queda de ONUs (pedido à parte, implementado)

Você pediu uma forma de identificar rápido uma queda de 30 ONUs ou 50% das ONUs online numa PON. Investiguei o código de coleta OLT a fundo antes de mexer, porque o sistema já tem uma arquitetura de coleta em camadas bem pensada (`internal/oltcollect/metrics.go`): um tier `pon_status` (só liga/desliga da PON), um tier `onu_counts`/`baseline` (status por ONU — o que dá a contagem exata) e um tier `full` (com telnet e potência óptica completa, mais lento). O alerta `olt_onu_drop`/`olt_onu_rise` (limiar por contagem `olt_onu_drop_count` ou por percentual `olt_onu_drop_percent`, configurável em Configurações → Alertas) já compara a leitura atual com a anterior e dispara **na primeira leitura em que a queda aparece** — não há debounce de "3 leituras" como no ping. Ou seja, a peça que faltava não era o alerta em si, era a *cadência da coleta*. Encontrei dois gargalos concretos:

1. **A coleta de todas as OLTs rodava sequencialmente** (`RunOltCollectAll` em `olt_collect_all.go`): um loop `for idx, it := range queue` processando uma OLT de cada vez, sem nenhuma concorrência — diferente de ping/telemetria, que já usavam `forEachLimited`. Com N OLTs, o tempo total do ciclo crescia linearmente com N.
2. **A coleta OLT só rodava dentro do pipeline sequencial** (interfaces → OLT), disparada apenas a cada `pipeline_cycle_seconds` (default 120s) — ping, telemetria e BNG já tinham ciclos paralelos próprios e mais rápidos (`TryStartParallelTelemetryCycle` etc.), mas OLT não.

**O que implementei** (ambos no seu repositório):

- **Paralelizei a coleta entre OLTs** em `RunOltCollectAll`, usando o mesmo `forEachLimited` + `sweepConcurrency()` que ping/telemetria já usam. Cada equipamento continua serializado individualmente (`snmpdevicelock`), então isto só paraleliza *entre* OLTs diferentes — nunca duas sondas na mesma OLT ao mesmo tempo.
- **Criei um ciclo paralelo dedicado para o tier leve** (`TryStartParallelOltCycle`, em `worker_olt_parallel.go`), no mesmo padrão de `TryStartParallelTelemetryCycle`: roda numa goroutine própria, com cadência independente (`monitoring_intervals.olt_baseline_parallel_seconds`, migração `112_olt_parallel_cycle.sql`, default 30s, piso de segurança 15s) — desacoplado do pipeline sequencial de interfaces. O tier `full` (telnet, potência óptica completa) continua no pipeline sequencial, na cadência mais lenta que já tinha (`SkipOltBaselineInPipeline` evita coleta duplicada).

Resultado esperado: uma queda de ONUs que antes podia levar minutos para aparecer (esperar o pipeline + coleta sequencial de todas as OLTs) passa a ser detectada tipicamente dentro de ~30–45s (o intervalo do novo ciclo) mais o tempo da própria coleta daquela OLT — que, com concorrência, deixa de esperar as outras OLTs na fila. Os números do benchmark (próxima seção) mostram a escala do ganho.

## Testes de stress

Não tenho como gerar tráfego real de ping/SNMP contra 500 equipamentos aqui, então usei duas abordagens complementares, ambas rodadas neste sandbox (nunca contra o Supabase de produção do `.env`):

**1) Benchmark de concorrência (Go, só biblioteca padrão)** — simula o padrão real (`forEachLimited`) coletando OLTs com latência de SNMP walk realista (proporcional ao nº de ONUs, com jitter):

| Cenário | OLTs | Antes (sequencial) | Depois (paralelo) | Ganho |
|---|---|---|---|---|
| Escala atual (~75 equip., 10 OLTs, ~96 ONU/OLT) | 10 | 6,13s | 1,31s (conc.=6) | **4,7x** |
| Escala maior (~500 equip., 65 OLTs, ~96 ONU/OLT) | 65 | 41,49s | 6,90s (conc.=6) / 2,89s (conc.=16) | **6,0x / 14,4x** |
| Escala maior, OLTs cheias (~400 ONU/OLT) | 65 | ~115s (extrapolado) | ~8s (conc.=16) | **~14,4x** |

**2) Query da correção do LIMIT 500, com Postgres local semeado com dados sintéticos** (5.000 e depois 20.000 equipamentos):
- Antes da correção: `SELECT ... LIMIT 500` devolvia **500** de 5.000 equipamentos elegíveis (bug confirmado na prática, não só na leitura do código).
- Depois: devolve os 5.000 (ou 20.000) completos, com `EXPLAIN ANALYZE` em **8,7ms** (5k linhas) e **21ms** (20k linhas) — sequential scan simples, sem necessidade de índice novo mesmo nesse volume. Ou seja: a correção do LIMIT é segura em qualquer escala plausível para um ISP, sem trade-off de performance.

Não encontrei novos gargalos além dos já listados nos achados 1–4 (agora corrigidos) — a arquitetura de coleta (locks por equipamento, ciclos paralelos, índices nas tabelas de série temporal) aguenta bem tanto a escala atual (~75) quanto uma escala 5-10x maior. O próximo ponto a observar, se o inventário crescer muito além de milhares de equipamentos, seria paginação real (cursor) nas listagens da API em vez do teto fixo de 2000–5000 que apliquei agora — mas isso está bem longe da escala atual ou mesmo da de "~500 equipamentos" que você mencionou.

## Tela de Mapa: CTOs e foguetes não aparecem (26/08/2026, pedido à parte)

Você reportou que CTOs e foguetes (caixas de emenda — o app já usa esse apelido em `MapSettingsModal.tsx`/`MapFilterModal.tsx`) não aparecem no mapa, exceto as 3 CTOs mais próximas quando usa "minha localização" (GPS). Auditei o fluxo completo — `quasar_frontend/src/pages/MapPage.tsx` (2138 linhas), `src/components/EquipmentMap.tsx` (1762 linhas, renderização Leaflet/clustering), `src/components/MapFilterModal.tsx` e os handlers `internal/api/handlers_map_infrastructure.go` / `handlers_map_nearest.go` no backend.

**Causa raiz encontrada (não era só na sua cabeça — o próprio código já "confessava" o problema num texto de ajuda escondido no modal de filtros):** a tela pede infraestrutura (CTOs, cabos, postes, foguetes, POPs) ao backend só quando há um **projeto selecionado** no filtro "Projeto" (`shouldLoadMapInfrastructure` em `src/lib/mapProjectFilter.ts`). O valor padrão desse filtro ao abrir o mapa é **"Nenhum"** — e com "Nenhum", a função devolvia `false`, então a query `map-infrastructure-points` nunca era disparada (`enabled: ... && shouldLoadMapInfrastructure(...)` em `MapPage.tsx`). Isso acontecia **mesmo com os toggles "CTOs" e "POPs" ligados por padrão** — os toggles controlam o que é *pedido*, mas esse gate por projeto decidia, antes deles, se a busca acontecia. O próprio modal de filtros já tinha o aviso (que ninguém costuma ler): *"Infraestrutura (CTOs, cabos, etc.) não é carregada. Escolha um projeto ou «Todos» para ver no mapa."*

A única via que ignora esse gate é a busca de "CTOs mais próximas" (`/api/v1/map/nearest-ctos`, acionada pelo botão de localização/GPS) — por isso, e só por isso, as 3 CTOs mais próximas apareciam mesmo com tudo o resto invisível. Bate exatamente com o que você descreveu.

Esse gate por projeto é sobra de uma versão anterior da tela, de antes de existir a busca por viewport (bounding box) com tetos no backend (`mapInfrastructureLimit`/`mapInfraKindCap` em `handlers_map_infrastructure.go`, já preparados para até 15.000 pontos por tipo conforme o zoom). Ou seja: a proteção de performance "certa" (buscar só o que está na tela, com teto) já existia; o gate por projeto ficou como uma segunda trava esquecida, redundante e mal comunicada na UI.

**Correção aplicada:**
- `src/lib/mapProjectFilter.ts` — `shouldLoadMapInfrastructure` passa a devolver sempre `true`; a decisão de pedir infraestrutura volta a ser só das camadas ativas (CTOs/cabos/postes/foguetes/POPs) + área visível do mapa. O filtro de projeto/localidade continua funcionando normalmente — mas agora só *restringe* o resultado (via `project_id`/`locality_id` na query), não decide se ele existe.
- `src/pages/MapPage.tsx` — `showSpliceBoxes` (a camada "foguete") passa a vir **ligada por padrão**, igual às CTOs. Antes vinha desligada por padrão (`useState(false)`), então mesmo depois da correção acima os foguetes continuariam invisíveis até você abrir Filtros e ligar manualmente "Caixas de emenda / foguete".
- `src/components/MapFilterModal.tsx` — corrigido o texto de ajuda do filtro "Nenhum", que agora descreve o comportamento real em vez do antigo (e incorreto, depois da correção) aviso de que nada seria carregado.

**Por que não fiz uma reescrita completa da tela:** depois de ler o fluxo inteiro, a arquitetura de fundo está correta — busca por viewport com debounce (`MapBoundsReporter`, 350 ms), cache incremental por tipo (`infraCacheRef`) para não haver flicker ao mover o mapa, clustering em grade por zoom, tetos no backend por tipo/zoom. O bug era uma condição de habilitação da query, não um problema estrutural de renderização — corrigir isso resolve o sintoma relatado sem o risco de uma reescrita grande sem conseguir rodar `npm run typecheck`/`build` neste ambiente (mesma limitação já registrada acima). A única dívida arquitetural real que encontrei aqui é a mesma já listada no achado 6 da tabela abaixo: `MapPage.tsx` com 2138 linhas concentra busca de dados, estado de UI e lógica de edição no mesmo componente — um bom próximo passo (não urgente) seria extrair hooks como `useMapInfrastructureData`/`useMapGeoTracking`/`useMapPlaceMode`, mas isso é refatoração de manutenibilidade, não uma correção de bug.

**Verificação:** não consegui rodar o app real neste ambiente (mesma limitação de rede/timeout já descrita), então validei por leitura de código + checagem de sintaxe (`ts.transpileModule` sobre os 3 arquivos alterados, sem erros) e por rastrear manualmente o caminho de dados ponta a ponta (estado → query → SQL do backend) para os dois casos (com e sem projeto selecionado). Recomendo confirmar visualmente depois de `npm run build`: abrir o Mapa sem selecionar nenhum projeto e ver se CTOs e foguetes aparecem dentro da área visível.

**Atualizado em 26/08/2026 (3):** revisão de acompanhamento (nova sessão) sobre a mesma correção, com `npm run typecheck` e `npm run build` rodados de verdade desta vez (ambos passaram sem erros) — confirma que a correção acima é sintaticamente válida, o que a sessão anterior não tinha conseguido verificar. Essa revisão também encontrou e corrigiu dois bugs residuais deixados pela própria correção, ambos em `src/pages/MapPage.tsx`:

- O aviso *"Projeto: Nenhum — escolha um projeto ou «Todos» nos filtros para carregar CTOs"*, mostrado no cabeçalho do Mapa quando nenhum projeto está seleccionado, não tinha sido removido — ficou a informar o utilizador de um comportamento que já não existe (o mesmo aviso que motivou a correção original, só que duplicado no cabeçalho da página em vez de só no modal de filtros). Removido; o cabeçalho agora só mostra o aviso de "infra limitada neste zoom" quando aplicável.
- `filterActiveCount` (o contador que acende o ponto azul no botão de filtro) tinha `showSpliceBoxes` na lista de condições de "filtro activo" — fazia sentido quando o padrão da camada era `false` (então `showSpliceBoxes === true` = desvio do padrão), mas a própria correção acima mudou o padrão para `true` sem actualizar esta condição. Resultado: o ponto azul de "filtro activo" ficava sempre aceso, mesmo com o Mapa em estado 100% padrão. Trocado para `!showSpliceBoxes`.

Nenhum dos dois escondia CTOs/foguetes do mapa em si (a correção original já resolve o sintoma relatado) — eram inconsistências de UI deixadas por uma correção que mudou um default sem varrer todos os lugares que dependiam do valor antigo desse default. Não foi encontrado nenhum outro ponto no fluxo de dados (`EquipmentMap.tsx`, clustering em grade, tetos do backend em `handlers_map_infrastructure.go`) que limite artificialmente a quantidade de pontos visíveis — os tetos por zoom lá vão a milhares, muito acima do que motivaria o sintoma "só 3 aparecem". Mantida a mesma conclusão da sessão anterior: não há justificação para reescrever a tela — a arquitetura (busca por viewport, cache incremental, clustering) está correta; o problema era mesmo a condição de habilitação da query, mais estas duas sobras de UI.

**Atualizado em 26/08/2026 (4) — causa raiz real encontrada e corrigida:** depois do rebuild/deploy das correções acima, você reportou que CTOs e foguetes continuavam sem aparecer, enquanto equipamentos apareciam normalmente, e que abrir uma CTO específica pela tela "Elementos" a mostrava perfeitamente no mapa (isolada). Isso indicava que não era mais um problema de habilitação de query nem de dados/coordenadas — tinha de ser algo que só acontece quando o mapa busca **vários tipos de infraestrutura de uma vez** (CTOs + cabos + foguetes + POPs na mesma chamada).

Com o container já reconstruído e rodando, testei a API real (`docker run` numa rede efémera ligada à rede do Compose, já que este ambiente não tem acesso directo a `localhost:8080`) reproduzindo exactamente a chamada que o Mapa faz por omissão: `GET /api/v1/map/infrastructure-points?kinds=ctos,cables,splice_boxes,pops&...`. Resultado: **HTTP 500**, `{"code":"DB","error":"ERROR: column \"display_number\" does not exist (SQLSTATE 42703)"}`.

**Causa:** em `handlers_map_infrastructure.go`, a função `orderNearCenter` (usada para ordenar os resultados pela distância ao centro do mapa) acrescentava incondicionalmente `, display_number` ao `ORDER BY` — mas a tabela `pops` **não tem** coluna `display_number` (confirmado via `information_schema.columns`; `network_ctos`, `network_cables`, `network_poles`, `network_splice_boxes` e `network_projects` têm; `pops` não). Como o Mapa pede todos os tipos activos **numa única chamada HTTP**, o erro na parte de POPs derrubava a resposta inteira com 500 — inclusive as CTOs, cabos e foguetes que já tinham sido buscados com sucesso antes do código chegar ao bloco de POPs. Por isso equipamentos (endpoint `/map/equipment-points`, chamada separada) continuavam a aparecer normalmente, e abrir uma CTO isolada pela tela "Elementos" também funcionava (não passa por este endpoint combinado).

**Correção aplicada:** `orderNearCenter` passou a receber um parâmetro `withDisplayNumber bool`; todas as chamadas para tabelas que têm a coluna (CTOs, cabos, postes, foguetes/emendas, projectos) continuam a pedi-la como critério de desempate na ordenação, e a chamada usada para `pops` passa `false` (ordena só por proximidade ao centro, sem a coluna inexistente). Verificado com `go build ./...` e `go vet ./internal/api/...` (ambos limpos), depois com a API real após rebuild da imagem Docker: a mesma chamada que antes devolvia 500 agora devolve 200 com 461 CTOs, 7 emendas/foguetes, 36 cabos e 4 POPs dentro da área testada — e o caso sem bounding box (`kinds=pops` sozinho, sem viewport) também foi testado e corrigido, porque tinha a mesma falha (`ORDER BY display_number` sem bbox).

Esta era a causa raiz real do sintoma "só equipamentos aparecem, CTOs/foguetes não" — as correções de frontend das actualizações (2) e (3) acima continuam correctas e necessárias (sem elas, mesmo com este bug de SQL corrigido, a infraestrutura só carregaria com projecto seleccionado, ou os foguetes continuariam desligados por omissão), mas não eram suficientes sozinhas: havia um segundo bug independente, no backend, que derrubava a chamada combinada antes mesmo de chegar à renderização no frontend.

**Se ainda faltar algo depois desta correção**, o próximo suspeito mais provável não é mais de código — é de dados: CTOs ligadas a um `project_id` cujo projeto está marcado como `status = 'inativo'` ficam ocultas tanto na busca normal quanto na busca por GPS (`infraMapHideInactiveSQL`/mesma cláusula em `mapNearestCtos` — isso é intencional, "projeto inativo não aparece no mapa"). Vale conferir em Comercial → Projetos de rede se algum projeto com CTOs de produção não está sinalizado como inativo por engano.

## Priorização sugerida

| # | Achado | Categoria | Severidade | Status |
|---|--------|-----------|------------|--------|
| 1 | `LIMIT 500` sem paginação nas queries de monitoramento e listagens | Escalabilidade | Crítico | ✅ Implementado |
| 2 | Concorrência do worker fixa em 6, sem visibilidade de atraso de ciclo | Performance | Alto | ✅ Implementado (falta campo na UI) |
| 3 | Pool de conexões fixo no código + sem retenção automática de histórico | Performance/operação | Médio | ✅ Implementado (falta campo na UI) |
| 4 | Sem virtualização de listas grandes no frontend | Performance (UI) | Médio | ✅ Implementado em Equipamentos (paginação); Conexões/Mapa pendentes |
| — | Coleta OLT sequencial + só no pipeline lento (detecção de queda de ONUs) | Performance | Alto | ✅ Implementado (pedido à parte) |
| — | Mapa: infraestrutura (CTOs/foguetes/cabos/postes/POPs) só carregava com projeto selecionado | Bug de visualização | Crítico | ✅ Implementado (pedido à parte) |
| 5 | `internal/api` monolítico, handlers de 1000–2000 linhas | Arquitetura | Médio | Pendente |
| 6 | Páginas React gigantes sem separação dados/UI | Arquitetura | Médio | Pendente |
| 7 | Zero testes automatizados no frontend | Qualidade | Alto (risco) | Pendente |
| 8 | Erros de escrita no banco ignorados sem log | Qualidade | Baixo–Médio | Pendente |

## Próximos passos

1. **Rodar `go build ./... && go test ./... -short` e `npm run typecheck && npm run build`** antes de subir para produção — não consegui rodar nenhum dos dois até o fim neste ambiente (ver "O que foi implementado").
2. **Aplicar as 3 migrações novas** (`go run ./cmd/migrate/`) — adicionam colunas com `DEFAULT`, não quebram nada em produção, mas precisam rodar antes do binário novo iniciar.
3. **Testar em ambiente de homologação/local antes de produção**, principalmente o ciclo OLT paralelo novo (`TryStartParallelOltCycle`) — é a mudança de comportamento mais nova, ainda que reaproveitando padrões já usados por ping/telemetria.
4. **Conferir a tela de Mapa após o build**: abrir sem selecionar projeto e confirmar que CTOs e foguetes aparecem na área visível. Se algum ainda faltar, verificar se o projeto de rede associado está marcado como inativo (ver seção "Tela de Mapa" acima).
5. Opcional, quando tiver tempo: expor `sweep_concurrency`, `history_retention_days` e `olt_baseline_parallel_seconds` como campos em Configurações → Monitoramento (hoje só ajustáveis por SQL direto ou variável de ambiente — o backend já suporta).
6. Itens 5, 6 e 7 (arquitetura/testes) continuam como trabalho de fundo contínuo — sugiro tratá-los como refatoração incremental (a cada handler/página nova ou alterada, já sair do padrão monolítico) em vez de uma reescrita grande de uma vez. A extração de hooks do `MapPage.tsx` (mencionada acima) é um bom próximo candidato.

Posso detalhar qualquer um dos pontos pendentes — por exemplo, propor a extração de um `useBngSessions`/service layer como exemplo para replicar nas demais telas/handlers, ou levar a paginação de Equipamentos para Conexões e Mapa.

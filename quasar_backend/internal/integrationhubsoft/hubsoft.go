// Package integrationhubsoft é o cliente dedicado da API do HubSoft (ERP do provedor —
// https://docs.hubsoft.com.br). Ao contrário do IXC (que continua no motor genérico de
// "requisições configuráveis" em internal/integrationconsumer), a API da HubSoft é
// conhecida e fixa, então este pacote fala diretamente com os endpoints documentados —
// login OAuth2 (password grant), busca de clientes, atendimentos e ordens de serviço —
// num único arquivo, sem depender de internal/integrationconsumer (não mexe no IXC).
//
// Reaproveita o motor HTTP genérico já existente (internal/integrationhttp: Execute,
// OAuth2PasswordBody, AuthConfig) em vez de reimplementar transporte/autenticação.
package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// Config liga este pacote à linha da tabela `integrations` (slug "hubsoft") — base_url e
// auth_config (client_id/client_secret/username/password) já existem lá.
type Config struct {
	BaseURL string
	Auth    integrationhttp.AuthConfig
	Timeout int // ms; 0 = default do integrationhttp
}

func (c Config) integ(token string) integrationhttp.IntegrationConfig {
	return integrationhttp.IntegrationConfig{
		BaseURL:      c.BaseURL,
		AuthType:     "oauth2_password",
		AuthConfig:   c.Auth,
		SessionToken: token,
		TimeoutMs:    c.Timeout,
	}
}

// Login troca client_id/client_secret/username/password por um access_token (grant
// password). Mesma construção de corpo já validada em produção (form-encoded — a API da
// HubSoft aceita application/x-www-form-urlencoded no /oauth/token, apesar do exemplo da
// doc mostrar JSON).
func Login(ctx context.Context, cfg Config) (token string, expiresInSec int, res integrationhttp.RunResult) {
	ac := cfg.Auth
	if strings.TrimSpace(ac.LoginPath) == "" {
		ac.LoginPath = "/oauth/token"
	}
	if strings.TrimSpace(ac.LoginMethod) == "" {
		ac.LoginMethod = "POST"
	}
	if strings.TrimSpace(ac.GrantType) == "" {
		ac.GrantType = "password"
	}
	if strings.TrimSpace(ac.TokenJSONPath) == "" {
		ac.TokenJSONPath = "access_token"
	}
	body, bodyType := integrationhttp.OAuth2PasswordBody(ac)
	rc := integrationhttp.RequestConfig{
		Method:             ac.LoginMethod,
		Path:               ac.LoginPath,
		BodyTemplate:       body,
		BodyType:           bodyType,
		ExtractJSONPath:    ac.TokenJSONPath,
		OmitDefaultHeaders: true,
	}
	res = integrationhttp.RunWithLoginRequest(ctx, cfg.integ(""), rc, true)
	if res.TokenFromLogin == "" {
		return "", 0, res
	}
	return res.TokenFromLogin, integrationhttp.TokenExpiresInSeconds([]byte(res.ResponsePreview)), res
}

// --- Consulta de clientes (GET /api/v1/integracao/cliente) ---------------------------------

// SearchClientsQueryOverrides parâmetros do endpoint de cliente — resumido ou detalhado
// (detalhado inclui contrato/alarmes/STFC/MVNO/anexos/desbloqueios, usado para "dados
// completos" e para a varredura de IPv4/MAC).
func SearchClientsQueryOverrides(detailed bool) map[string]string {
	if detailed {
		return map[string]string{
			"inativo": "todos", "limit": "100", "cancelado": "sim", "ultima_conexao": "sim",
			"incluir_alarmes": "sim", "incluir_contrato": "sim", "incluir_stfc": "sim",
			"incluir_mvno": "sim", "incluir_anexos": "sim", "incluir_desbloqueios": "sim",
			"order_by": "data_cadastro", "order_type": "desc",
		}
	}
	return map[string]string{
		"inativo": "todos", "limit": "20", "cancelado": "nao", "ultima_conexao": "sim",
		"incluir_alarmes": "nao", "incluir_contrato": "sim", "incluir_stfc": "nao",
		"incluir_mvno": "nao", "incluir_anexos": "nao", "incluir_desbloqueios": "nao",
		"order_by": "data_cadastro", "order_type": "desc",
	}
}

// BuscaOptions tipos de busca nativos do endpoint de cliente.
func BuscaOptions() []BuscaOption {
	return []BuscaOption{
		{Value: "nome_razaosocial", Label: "Nome / Razão social"},
		{Value: "cpf_cnpj", Label: "CPF/CNPJ"},
		{Value: "nome_fantasia", Label: "Nome fantasia"},
		{Value: "codigo_cliente", Label: "Código do cliente"},
		{Value: "telefone", Label: "Telefone"},
		{Value: "login_radius", Label: "Login RADIUS"},
		{Value: "email", Label: "E-mail"},
	}
}

type BuscaOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

func SearchClients(ctx context.Context, cfg Config, token, busca, termo string, detailed bool) (ClientSearchResult, integrationhttp.RunResult) {
	q := SearchClientsQueryOverrides(detailed)
	q["busca"] = busca
	q["termo_busca"] = termo
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente", QueryParams: paramKVs(q),
	})
	return ParseClientSearch(ResponseBodyBytes(res)), res
}

// scanLetters usadas para varrer nome_razaosocial em buscas de IPv4/MAC — testado
// directamente contra a API de produção: o endpoint de cliente EXIGE um `busca` +
// `termo_busca` não vazios (rejeita listagem sem filtro) e não tem paginação funcional
// (nem "page" nem "pagina_atual" avançam a página — devolvem sempre o mesmo primeiro
// resultado; e `limit` acima de 100 é rejeitado pela API: "O limite máximo de resultados
// é de 100 itens"). Como alternativa real, varremos nome_razaosocial por várias letras
// comuns (cada uma devolve até 100 clientes cujo nome contém essa letra) e juntamos os
// resultados — cobre a esmagadora maioria da base sem depender de paginação inexistente.
var scanLetters = []string{"a", "e", "i", "o", "u", "s", "r", "m", "c", "l", "n", "t"}

const maxIPMACScanClients = 240 // teto de segurança (12 letras × até 20 clientes cada — ver comentário sobre o limite de 64KB abaixo).

// SearchByIPOrMAC varre clientes da HubSoft (por várias letras comuns em nome_razaosocial,
// já que não há paginação real disponível — ver scanLetters) e procura
// servicos[].ipv4 / servicos[].mac_addr até achar correspondência. A API da HubSoft não
// tem um parâmetro de busca nativo por IP/MAC (confirmado na documentação oficial), mas o
// retorno detalhado já traz esses campos por serviço.
func SearchByIPOrMAC(ctx context.Context, cfg Config, token, termo, kind string) (ClientSearchResult, error) {
	needle := strings.ToUpper(strings.TrimSpace(termo))
	if kind == "mac" {
		needle = normalizeMAC(needle)
	}
	if needle == "" {
		return ClientSearchResult{Clients: []ClientCard{}}, fmt.Errorf("termo de busca vazio")
	}

	seenClientIDs := map[string]bool{}
	scanned := 0
	for _, letter := range scanLetters {
		if scanned >= maxIPMACScanClients {
			break
		}
		// Limit baixo de propósito: o preview de resposta do motor HTTP partilhado
		// (internal/integrationhttp) tem um teto de 64KB, e cada cliente devolvido pela
		// HubSoft (mesmo sem os includes extra) já pesa ~2-3KB — 100 clientes por página
		// (o cap normal) estoura esse teto e a resposta chega truncada/ilegível (testado
		// directamente contra a API de produção). 20 por letra fica com folga.
		q := map[string]string{
			"inativo": "todos", "limit": "20", "cancelado": "nao", "ultima_conexao": "nao",
			"incluir_alarmes": "nao", "incluir_contrato": "nao", "incluir_stfc": "nao",
			"incluir_mvno": "nao", "incluir_anexos": "nao", "incluir_desbloqueios": "nao",
			"order_by": "data_cadastro", "order_type": "desc",
			"busca": "nome_razaosocial", "termo_busca": letter,
		}
		res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
			Method: "GET", Path: "/api/v1/integracao/cliente", QueryParams: paramKVs(q),
		})
		parsed := ParseClientSearch(ResponseBodyBytes(res))
		if !res.OK {
			continue // uma letra falhando não é fatal — segue para a próxima
		}

		for _, c := range parsed.Clients {
			if seenClientIDs[c.ID] {
				continue
			}
			seenClientIDs[c.ID] = true
			for _, svc := range c.Services {
				var hay string
				if kind == "mac" {
					hay = normalizeMAC(svc.MAC)
				} else {
					hay = strings.ToUpper(strings.TrimSpace(svc.IPv4))
				}
				if hay != "" && hay == needle {
					return ClientSearchResult{OK: true, Clients: []ClientCard{c}}, nil
				}
			}
		}
		scanned += len(parsed.Clients)
	}
	return ClientSearchResult{
		OK: true, Clients: []ClientCard{},
		Message: fmt.Sprintf("Nenhum cliente encontrado com esse %s (varridos %d clientes únicos na base).",
			map[string]string{"ipv4": "IPv4", "mac": "MAC"}[kind], len(seenClientIDs)),
	}, nil
}

func normalizeMAC(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.NewReplacer("-", "", ":", "", ".", "").Replace(s)
	return s
}

// --- Dashboard (amostra agregada) -----------------------------------------------------------
//
// A API da HubSoft não tem um endpoint de listagem/total "toda a base" — /cliente exige
// busca+termo_busca não vazios (confirmado em produção) e não pagina de verdade. Não existe
// forma de obter números exatos da base inteira num pedido só. O dashboard reaproveita a
// mesma técnica de varredura por letras comuns em nome_razaosocial já usada em
// SearchByIPOrMAC (ver scanLetters) para reunir uma AMOSTRA representativa de clientes e
// calcula os agregados (status, tecnologia, ligação, planos, cidades, receita estimada) em
// cima dela — por isso os números são rotulados como "amostra" no resultado, nunca como
// totais exatos da operadora.

// maxDashboardScanClients é só uma salvaguarda de memória/tempo, não um limite pensado para
// ser realmente atingido — na prática a varredura já cobre a base quase toda antes disso.
const maxDashboardScanClients = 6000
const scanConcurrency = 10

// scanBuckets gera os termos de busca por nome_razaosocial usados na varredura: letras
// simples sozinhas SATURAM rápido (testado em produção: só a letra "a" já bate no teto de
// 100 resultados por página que a própria API impõe, e não há paginação real — ver
// scanLetters), então a maior parte da cobertura vem de PARES de duas letras, muito mais
// seletivos (uma substring de 2 letras aparece em bem menos nomes que 1 letra sozinha) —
// isso é o que permite a varredura chegar perto da base inteira apesar do teto de 100/consulta.
func scanBuckets() []string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const commonSeconds = "aeiourstnl"
	buckets := make([]string, 0, len(letters)+len(letters)*len(commonSeconds))
	for _, c := range letters {
		buckets = append(buckets, string(c))
	}
	for _, c1 := range letters {
		for _, c2 := range commonSeconds {
			buckets = append(buckets, string(c1)+string(c2))
		}
	}
	return buckets
}

// ScanClientSample varre nome_razaosocial por letras simples + pares de duas letras (ver
// scanBuckets) em paralelo (scanConcurrency consultas simultâneas) para reunir o maior número
// possível de clientes únicos (incluindo cancelados/inativos, para os agregados de status
// fazerem sentido) — não existe endpoint de listagem/paginação real na API da HubSoft (ver
// comentário da secção Dashboard), então isto é a aproximação mais completa possível. Usada
// só pelo Dashboard "Clientes e Serviços" — demorada de propósito (cobertura quase total),
// o utilizador confirmou que aceita essa demora aí. Atendimentos/Ordens/Financeiro usam
// scanClientSampleFast (mais rápida, amostra menor) — ver comentário lá.
func ScanClientSample(ctx context.Context, cfg Config, token string) (ClientSearchResult, error) {
	return scanClientSampleCore(ctx, cfg, token, scanBuckets(), scanConcurrency, maxDashboardScanClients)
}

// fastScanClientCap/fastScanConcurrency: amostra rápida usada por Atendimentos/Ordens/
// Financeiro — só letras simples (26 consultas, não os ~290 pares usados pelo Dashboard),
// mais concorrência e um teto de clientes bem menor. Prioriza velocidade sobre cobertura
// (o utilizador pediu isso explicitamente: "mais rápido ou até mais limitado" para essas
// três abas, ao contrário do Dashboard onde a demora foi aceite).
const (
	fastScanConcurrency = 16
	fastScanClientCap   = 120
)

func scanClientSampleFast(ctx context.Context, cfg Config, token string) (ClientSearchResult, error) {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	buckets := make([]string, 0, len(letters))
	for _, c := range letters {
		buckets = append(buckets, string(c))
	}
	return scanClientSampleCore(ctx, cfg, token, buckets, fastScanConcurrency, fastScanClientCap)
}

func scanClientSampleCore(ctx context.Context, cfg Config, token string, buckets []string, concurrency, maxClients int) (ClientSearchResult, error) {
	var mu sync.Mutex
	seen := map[string]bool{}
	var all []ClientCard

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, term := range buckets {
		mu.Lock()
		full := len(all) >= maxClients
		mu.Unlock()
		if full {
			break
		}
		wg.Add(1)
		go func(term string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			q := map[string]string{
				"inativo": "todos", "limit": "100", "cancelado": "sim", "ultima_conexao": "sim",
				"incluir_alarmes": "nao", "incluir_contrato": "nao", "incluir_stfc": "nao",
				"incluir_mvno": "nao", "incluir_anexos": "nao", "incluir_desbloqueios": "nao",
				"order_by": "data_cadastro", "order_type": "desc",
				"busca": "nome_razaosocial", "termo_busca": term,
			}
			res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
				Method: "GET", Path: "/api/v1/integracao/cliente", QueryParams: paramKVs(q),
			})
			if !res.OK {
				return // um termo falhando não é fatal — os outros seguem
			}
			parsed := ParseClientSearch(ResponseBodyBytes(res))

			mu.Lock()
			for _, c := range parsed.Clients {
				if seen[c.ID] {
					continue
				}
				seen[c.ID] = true
				all = append(all, c)
			}
			mu.Unlock()
		}(term)
	}
	wg.Wait()

	if len(all) == 0 {
		return ClientSearchResult{OK: true, Clients: []ClientCard{}}, fmt.Errorf("nenhum cliente retornado pela varredura")
	}
	// O teto acima só para de DISPARAR novas consultas — com poucos buckets e concorrência
	// alta, várias já estão em voo quando ele é visto, então o total real pode passar do
	// teto. Corta aqui para garantir o limite de verdade (importa para scanClientSampleFast:
	// sem isto o "rápido" deixa de ser rápido).
	if len(all) > maxClients {
		all = all[:maxClients]
	}
	return ClientSearchResult{OK: true, Clients: all}, nil
}

// NamedCount uma categoria + contagem para os gráficos do dashboard (ex.: status → quantidade).
type NamedCount struct {
	Name  string  `json:"name"`
	Count int     `json:"count"`
	Value float64 `json:"value,omitempty"`
}

type DashboardResult struct {
	OK                      bool         `json:"ok"`
	Message                 string       `json:"message,omitempty"`
	SampleClients           int          `json:"sample_clients"`
	SampleServices          int          `json:"sample_services"`
	StatusBreakdown         []NamedCount `json:"status_breakdown"`
	TechnologyBreakdown     []NamedCount `json:"technology_breakdown"`
	ConnectedBreakdown      []NamedCount `json:"connected_breakdown"`
	TopPlans                []NamedCount `json:"top_plans"`
	TopCities               []NamedCount `json:"top_cities"`
	ActiveServices          int          `json:"active_services"`
	CanceledServices        int          `json:"canceled_services"`
	EstimatedMonthlyRevenue float64      `json:"estimated_monthly_revenue"`
}

// BuildDashboard varre uma amostra de clientes (ScanClientSample) e calcula os agregados
// usados na aba Dashboard — tudo derivado dos mesmos campos já normalizados em ServiceSummary
// (Status, Technology, Connected, PlanValue, City), sem chamadas extra por cliente.
func BuildDashboard(ctx context.Context, cfg Config, token string) DashboardResult {
	sample, err := ScanClientSample(ctx, cfg, token)
	if err != nil {
		return DashboardResult{OK: false, Message: "Falha ao coletar amostra de clientes: " + err.Error()}
	}

	statusCount := map[string]int{}
	techCount := map[string]int{}
	connCount := map[string]int{}
	planCount := map[string]int{}
	planRevenue := map[string]float64{}
	cityCount := map[string]int{}
	active, canceled := 0, 0
	var revenue float64
	serviceTotal := 0

	for _, c := range sample.Clients {
		for _, s := range c.Services {
			serviceTotal++
			status := firstNonEmpty(s.Status, "Sem status")
			statusCount[status]++
			low := strings.ToLower(status)
			switch {
			case strings.Contains(low, "habilit") || strings.Contains(low, "ativ"):
				active++
				revenue += parseBRFloat(s.PlanValue)
			case strings.Contains(low, "cancel"):
				canceled++
			}
			techCount[firstNonEmpty(s.Technology, "Não informada")]++
			switch s.Connected {
			case "true":
				connCount["Conectado"]++
			case "false":
				connCount["Desconectado"]++
			default:
				connCount["Sem dado"]++
			}
			plan := firstNonEmpty(s.Name, "Sem plano")
			planCount[plan]++
			planRevenue[plan] += parseBRFloat(s.PlanValue)
			if s.City != "" {
				cityCount[s.City]++
			}
		}
	}

	return DashboardResult{
		OK:                      true,
		SampleClients:           len(sample.Clients),
		SampleServices:          serviceTotal,
		StatusBreakdown:         topNamedCounts(statusCount, nil, 8),
		TechnologyBreakdown:     topNamedCounts(techCount, nil, 8),
		ConnectedBreakdown:      topNamedCounts(connCount, nil, 4),
		TopPlans:                topNamedCounts(planCount, planRevenue, 8),
		TopCities:               topNamedCounts(cityCount, nil, 8),
		ActiveServices:          active,
		CanceledServices:        canceled,
		EstimatedMonthlyRevenue: revenue,
	}
}

func topNamedCounts(counts map[string]int, values map[string]float64, limit int) []NamedCount {
	out := make([]NamedCount, 0, len(counts))
	for k, v := range counts {
		nc := NamedCount{Name: k, Count: v}
		if values != nil {
			nc.Value = values[k]
		}
		out = append(out, nc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// --- Atendimentos (GET /api/v1/integracao/cliente/atendimento) -----------------------------

func AttendanceQueryOverrides(busca, termo, apenasPendente string) map[string]string {
	if apenasPendente == "" {
		apenasPendente = "nao"
	}
	return map[string]string{
		"busca": busca, "termo_busca": termo, "limit": "50",
		"apenas_pendente": apenasPendente, "order_by": "data_cadastro", "order_type": "desc",
	}
}

func SearchAttendance(ctx context.Context, cfg Config, token, busca, termo string) (AttendanceResult, integrationhttp.RunResult) {
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/atendimento",
		QueryParams: paramKVs(AttendanceQueryOverrides(busca, termo, "nao")),
	})
	return ParseAttendance(ResponseBodyBytes(res)), res
}

// --- Ordens de serviço (GET /api/v1/integracao/cliente/ordem_servico) ----------------------

func WorkOrderQueryOverrides(busca, termo string) map[string]string {
	return map[string]string{
		"busca": busca, "termo_busca": termo, "limit": "50",
		"order_by": "data_cadastro", "order_type": "desc", "exibir_atendimento": "true",
	}
}

func SearchWorkOrders(ctx context.Context, cfg Config, token, busca, termo string) (WorkOrderResult, integrationhttp.RunResult) {
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/ordem_servico",
		QueryParams: paramKVs(WorkOrderQueryOverrides(busca, termo)),
	})
	return ParseWorkOrder(ResponseBodyBytes(res)), res
}

// --- Atendimentos/Ordens recentes (amostra) -------------------------------------------------
//
// Nem /cliente/atendimento nem /cliente/ordem_servico têm modo de listagem "de todos os
// clientes" — testado em produção: sem busca+termo_busca, ambos devolvem "Favor preencher o
// atributo (busca)"; tentar um endpoint irmão sem o prefixo /cliente/ (ex.
// /api/v1/integracao/atendimento) devolve "Método/endpoint não disponível" — não existe. A
// única forma de montar algo parecido com "atendimentos recentes" é: varrer uma amostra de
// clientes e consultar o atendimento/O.S. de cada um, em paralelo, juntando tudo e ordenando
// pela data mais recente — por isso o resultado é rotulado como amostra (não é garantidamente
// O atendimento mais recente da operadora inteira, só o mais recente dentro dos clientes
// efetivamente varridos). Usa scanClientSampleFast (amostra menor, mais rápida) em vez da
// varredura exaustiva do Dashboard — o utilizador pediu velocidade aqui em troca de menos
// cobertura, já que só os 20 mais recentes interessam.

const recentActivityConcurrency = 16

type RecentActivityResult struct {
	OK                        bool             `json:"ok"`
	Message                   string           `json:"message,omitempty"`
	SampleClients             int              `json:"sample_clients"`
	Attendance                []AttendanceItem `json:"attendance"`
	WorkOrders                []WorkOrderItem  `json:"work_orders"`
	TotalAttendanceFound      int              `json:"total_attendance_found"`
	TotalWorkOrdersFound      int              `json:"total_work_orders_found"`
	AttendanceStatusBreakdown []NamedCount     `json:"attendance_status_breakdown"`
	WorkOrderStatusBreakdown  []NamedCount     `json:"work_order_status_breakdown"`
}

// BuildRecentActivity varre uma amostra rápida de clientes e, para cada um (em paralelo),
// busca atendimentos e ordens de serviço — devolve os `limitEach` mais recentes de cada
// categoria (com nome/código do cliente anexado — a resposta desses endpoints não inclui
// isso por si só) mais os agregados por status (usados no modo "Atendimentos e Ordens de
// Serviço" do Dashboard, sem precisar varrer tudo de novo).
func BuildRecentActivity(ctx context.Context, cfg Config, token string, limitEach int) RecentActivityResult {
	sample, err := scanClientSampleFast(ctx, cfg, token)
	if err != nil {
		return RecentActivityResult{OK: false, Message: "Falha ao coletar amostra de clientes: " + err.Error()}
	}
	clients := sample.Clients

	var mu sync.Mutex
	var attendance []AttendanceItem
	var workOrders []WorkOrderItem
	attStatusCount := map[string]int{}
	woStatusCount := map[string]int{}

	sem := make(chan struct{}, recentActivityConcurrency)
	var wg sync.WaitGroup
	for _, c := range clients {
		codigo := firstNonEmpty(c.Code, c.ID)
		if codigo == "" {
			continue
		}
		wg.Add(1)
		go func(c ClientCard, codigo string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			att, attRes := SearchAttendance(ctx, cfg, token, "codigo_cliente", codigo)
			wo, woRes := SearchWorkOrders(ctx, cfg, token, "codigo_cliente", codigo)

			mu.Lock()
			if attRes.OK {
				for _, a := range att.Items {
					a.ClientName, a.ClientCode = c.Name, codigo
					attendance = append(attendance, a)
					attStatusCount[firstNonEmpty(a.Status, "Sem status")]++
				}
			}
			if woRes.OK {
				for _, o := range wo.Items {
					o.ClientName, o.ClientCode = c.Name, codigo
					workOrders = append(workOrders, o)
					woStatusCount[firstNonEmpty(o.Status, "Sem status")]++
				}
			}
			mu.Unlock()
		}(c, codigo)
	}
	wg.Wait()

	totalAtt, totalWO := len(attendance), len(workOrders)

	sort.Slice(attendance, func(i, j int) bool {
		return parseBRDate(attendance[i].OpenedAt).After(parseBRDate(attendance[j].OpenedAt))
	})
	sort.Slice(workOrders, func(i, j int) bool {
		return parseBRDate(workOrders[i].CreatedAt).After(parseBRDate(workOrders[j].CreatedAt))
	})
	if len(attendance) > limitEach {
		attendance = attendance[:limitEach]
	}
	if len(workOrders) > limitEach {
		workOrders = workOrders[:limitEach]
	}

	return RecentActivityResult{
		OK: true, SampleClients: len(clients),
		Attendance: attendance, WorkOrders: workOrders,
		TotalAttendanceFound: totalAtt, TotalWorkOrdersFound: totalWO,
		AttendanceStatusBreakdown: topNamedCounts(attStatusCount, nil, 8),
		WorkOrderStatusBreakdown:  topNamedCounts(woStatusCount, nil, 8),
	}
}

// --- Financeiro / faturas (GET /api/v1/integracao/cliente/financeiro) ----------------------
//
// Testado directamente contra a API de produção: o endpoint "irmão"
// /api/v1/integracao/financeiro/fatura existe mas só devolve a fatura ATUAL do carnê do
// cliente (1 registo) — inútil para "faturas criadas, vencidas e pendentes". Este aqui
// (/api/v1/integracao/cliente/financeiro, mesmo padrão busca/termo_busca dos outros
// endpoints de cliente) devolve o histórico completo; por omissão esconde as já pagas
// (só mostra "aguardando"), por isso `apenas_pendente=nao` é obrigatório para trazer
// pagas + vencidas + pendentes juntas (confirmado: sem o parâmetro vieram 11 faturas
// futuras; com ele vieram as mesmas 11 + as 2 já pagas = 13).

func FinancialQueryOverrides(busca, termo string) map[string]string {
	return map[string]string{
		"busca": busca, "termo_busca": termo, "limit": "100", "apenas_pendente": "nao",
	}
}

func SearchFinancial(ctx context.Context, cfg Config, token, busca, termo string) (FinancialResult, integrationhttp.RunResult) {
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/financeiro",
		QueryParams: paramKVs(FinancialQueryOverrides(busca, termo)),
	})
	return ParseFinancial(ResponseBodyBytes(res)), res
}

// --- Resumo financeiro agregado (amostra) ---------------------------------------------------
//
// Mesma limitação: /cliente/financeiro só existe por cliente, não há "todas as faturas da
// operadora" — o resumo varre uma amostra de clientes (ScanClientSample) e soma as faturas de
// cada um. A API da HubSoft, sendo o faturamento do próprio provedor aos SEUS clientes, só
// expõe "contas a receber" (faturas dos clientes) — não há um conceito de "contas a pagar"
// (despesas do provedor a fornecedores) neste endpoint de integração; o resumo por isso cobre
// pendente/vencido/pago, que é o equivalente real disponível.

const (
	financialSummaryConcurrency = 16
	financialSummaryTopDebtors  = 15
)

type ClientDebt struct {
	ClientName   string  `json:"client_name"`
	ClientCode   string  `json:"client_code"`
	PendingValue float64 `json:"pending_value"`
	OverdueValue float64 `json:"overdue_value"`
	InvoiceCount int     `json:"invoice_count"`
}

type FinancialSummaryResult struct {
	OK              bool         `json:"ok"`
	Message         string       `json:"message,omitempty"`
	SampleClients   int          `json:"sample_clients"`
	ClientsWithDebt int          `json:"clients_with_debt"`
	TotalInvoices   int          `json:"total_invoices"`
	TotalReceivable float64      `json:"total_receivable"` // pendente + vencido
	TotalOverdue    float64      `json:"total_overdue"`
	TotalPending    float64      `json:"total_pending"` // ainda não vencido
	TotalPaid       float64      `json:"total_paid"`
	TopDebtors      []ClientDebt `json:"top_debtors"`
}

// BuildFinancialSummary varre uma amostra rápida de clientes (scanClientSampleFast) e, para
// cada um (em paralelo), busca o financeiro — soma pendente/vencido/pago e devolve os
// maiores devedores (para follow-up de cobrança). Só números totais, sem detalhe fatura a
// fatura — por isso não precisa da varredura exaustiva do Dashboard.
func BuildFinancialSummary(ctx context.Context, cfg Config, token string) FinancialSummaryResult {
	sample, err := scanClientSampleFast(ctx, cfg, token)
	if err != nil {
		return FinancialSummaryResult{OK: false, Message: "Falha ao coletar amostra de clientes: " + err.Error()}
	}
	clients := sample.Clients

	var mu sync.Mutex
	var totalInvoices, clientsWithDebt int
	var totalOverdue, totalPending, totalPaid float64
	var debtors []ClientDebt

	sem := make(chan struct{}, financialSummaryConcurrency)
	var wg sync.WaitGroup
	for _, c := range clients {
		codigo := firstNonEmpty(c.Code, c.ID)
		if codigo == "" {
			continue
		}
		wg.Add(1)
		go func(c ClientCard, codigo string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			fin, res := SearchFinancial(ctx, cfg, token, "codigo_cliente", codigo)
			if !res.OK {
				return
			}

			mu.Lock()
			totalInvoices += fin.Summary.Total
			totalOverdue += fin.Summary.OverdueValue
			totalPending += fin.Summary.PendingValue
			totalPaid += fin.Summary.PaidValue
			if fin.Summary.OverdueValue+fin.Summary.PendingValue > 0 {
				clientsWithDebt++
				debtors = append(debtors, ClientDebt{
					ClientName: c.Name, ClientCode: codigo,
					PendingValue: fin.Summary.PendingValue, OverdueValue: fin.Summary.OverdueValue,
					InvoiceCount: fin.Summary.OverdueCount + fin.Summary.PendingCount,
				})
			}
			mu.Unlock()
		}(c, codigo)
	}
	wg.Wait()

	sort.Slice(debtors, func(i, j int) bool {
		return (debtors[i].PendingValue + debtors[i].OverdueValue) > (debtors[j].PendingValue + debtors[j].OverdueValue)
	})
	if len(debtors) > financialSummaryTopDebtors {
		debtors = debtors[:financialSummaryTopDebtors]
	}

	return FinancialSummaryResult{
		OK: true, SampleClients: len(clients), ClientsWithDebt: clientsWithDebt,
		TotalInvoices: totalInvoices, TotalReceivable: totalOverdue + totalPending,
		TotalOverdue: totalOverdue, TotalPending: totalPending, TotalPaid: totalPaid,
		TopDebtors: debtors,
	}
}

// --- Teste de conexão ------------------------------------------------------------------------

// TestResult resultado do "Configuração e Teste": faz o login real e, se ele funcionar,
// uma chamada leve real (limit=1 na busca de clientes) — prova as credenciais de ponta a
// ponta, ao contrário do teste genérico antigo (GET "/" na base_url, que não valida nada).
type TestResult struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	LatencyMS int64  `json:"latency_ms"`
	Token     string `json:"-"`
	ExpiresIn int    `json:"expires_in,omitempty"`
}

func TestConnection(ctx context.Context, cfg Config) TestResult {
	start := time.Now()
	token, expiresIn, loginRes := Login(ctx, cfg)
	if token == "" {
		msg := loginRes.ErrorMessage
		if msg == "" {
			msg = "Login falhou — confira client_id/client_secret/username/password."
		}
		return TestResult{OK: false, Message: msg, LatencyMS: time.Since(start).Milliseconds()}
	}
	_, callRes := SearchClients(ctx, cfg, token, "codigo_cliente", "", false)
	if !callRes.OK && callRes.StatusCode != 404 {
		// 200 vazio ou 404 (nenhum cliente com esse filtro) ainda provam que o token é aceite;
		// outros erros (401/403/5xx) indicam problema real de credencial/permissão/rede.
		return TestResult{
			OK: false, Token: token, ExpiresIn: expiresIn,
			Message:   fmt.Sprintf("Login OK, mas a chamada de teste falhou: %s", firstNonEmpty(callRes.ErrorMessage, fmt.Sprintf("HTTP %d", callRes.StatusCode))),
			LatencyMS: time.Since(start).Milliseconds(),
		}
	}
	return TestResult{
		OK: true, Token: token, ExpiresIn: expiresIn,
		Message:   "Login e consulta de teste OK.",
		LatencyMS: time.Since(start).Milliseconds(),
	}
}

// --- Parsing de respostas ---------------------------------------------------------------------

// ClientCard dados normalizados de um cliente para a UI.
type ClientCard struct {
	ID        string            `json:"id,omitempty"`
	Code      string            `json:"code,omitempty"`
	Name      string            `json:"name,omitempty"`
	TradeName string            `json:"trade_name,omitempty"`
	Document  string            `json:"document,omitempty"`
	Email     string            `json:"email,omitempty"`
	Phone     string            `json:"phone,omitempty"`
	Status    string            `json:"status,omitempty"`
	Address   string            `json:"address,omitempty"`
	Services  []ServiceSummary  `json:"services,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
	Raw       map[string]any    `json:"raw,omitempty"`
}

// ServiceSummary um serviço/plano/login do cliente — inclui os campos ricos do payload
// detalhado (equipamento, interface/PON, última conexão, endereço de instalação) usados
// na busca de "dados completos".
type ServiceSummary struct {
	ID                 string `json:"id,omitempty"`
	Name               string `json:"name,omitempty"`
	Status             string `json:"status,omitempty"`
	Login              string `json:"login,omitempty"`
	IPv4               string `json:"ipv4,omitempty"`
	MAC                string `json:"mac,omitempty"`
	Technology         string `json:"technology,omitempty"`
	PlanValue          string `json:"plan_value,omitempty"`
	Connected          string `json:"connected,omitempty"`
	LastConnectedAt    string `json:"last_connected_at,omitempty"`
	LastDisconnectedAt string `json:"last_disconnected_at,omitempty"`
	LastIPv4           string `json:"last_ipv4,omitempty"`
	LastNasIP          string `json:"last_nas_ip,omitempty"`
	StatusText         string `json:"status_text,omitempty"`
	OLT                string `json:"olt,omitempty"`
	Concentrator       string `json:"concentrator,omitempty"`
	PON                string `json:"pon,omitempty"`
	InstallAddress     string `json:"install_address,omitempty"`
	Latitude           string `json:"latitude,omitempty"`
	Longitude          string `json:"longitude,omitempty"`
	City               string `json:"city,omitempty"`
}

type ClientSearchResult struct {
	OK        bool         `json:"ok"`
	Message   string       `json:"message,omitempty"`
	Clients   []ClientCard `json:"clients"`
	RawStatus string       `json:"raw_status,omitempty"`
}

func ParseClientSearch(raw []byte) ClientSearchResult {
	out := ClientSearchResult{Clients: []ClientCard{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	out.RawStatus = strings.TrimSpace(scalarToString(doc["status"]))
	if st := strings.ToLower(out.RawStatus); st == "error" || st == "erro" {
		out.Message = firstNonEmpty(scalarToString(doc["msg"]), scalarToString(doc["message"]), "Erro retornado pela API")
		return out
	}
	for _, it := range extractArray(doc, "clientes", "registros", "results", "data", "items") {
		if card, ok := mapClientItem(it); ok {
			out.Clients = append(out.Clients, card)
		}
	}
	out.OK = true
	if len(out.Clients) == 0 {
		out.Message = "Nenhum cliente encontrado para este termo."
	}
	return out
}

func extractArray(doc map[string]any, keys ...string) []any {
	for _, key := range keys {
		if arr, ok := doc[key].([]any); ok && len(arr) > 0 {
			return arr
		}
	}
	if data, ok := doc["data"].(map[string]any); ok {
		for _, key := range keys {
			if arr, ok := data[key].([]any); ok && len(arr) > 0 {
				return arr
			}
		}
	}
	if _, hasName := doc["nome_razaosocial"]; hasName {
		return []any{doc}
	}
	return nil
}

func mapClientItem(it any) (ClientCard, bool) {
	m, ok := it.(map[string]any)
	if !ok {
		return ClientCard{}, false
	}
	card := ClientCard{
		ID:        pickStr(m, "id_cliente", "id", "codigo_cliente"),
		Code:      pickStr(m, "codigo_cliente", "id_cliente"),
		Name:      pickStr(m, "nome_razaosocial", "nome"),
		TradeName: pickStr(m, "nome_fantasia"),
		Document:  pickStr(m, "cpf_cnpj", "cnpj_cpf"),
		Email:     pickStr(m, "email_principal", "email"),
		Phone:     pickStr(m, "telefone_primario", "telefone"),
		Status:    pickStr(m, "status_cadastro", "status"),
		Details:   map[string]string{},
	}
	if card.Name == "" && card.Code == "" && card.Document == "" {
		return ClientCard{}, false
	}
	card.Raw = m
	card.Services = mapServices(m)
	if len(card.Services) > 0 {
		card.Address = card.Services[0].InstallAddress
	}
	for _, k := range []string{"data_cadastro", "data_nascmento"} {
		if v := pickStr(m, k); v != "" {
			card.Details[k] = v
		}
	}
	return card, true
}

func mapServices(m map[string]any) []ServiceSummary {
	arr, ok := m["servicos"].([]any)
	if !ok {
		return nil
	}
	var out []ServiceSummary
	for _, it := range arr {
		sm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		svc := ServiceSummary{
			ID:         pickStr(sm, "id_cliente_servico", "id"),
			Name:       pickStr(sm, "nome"),
			Status:     pickStr(sm, "status"),
			Login:      pickStr(sm, "login"),
			IPv4:       pickStr(sm, "ipv4"),
			MAC:        pickStr(sm, "mac_addr", "phy_addr"),
			Technology: pickStr(sm, "tecnologia"),
			PlanValue:  pickStr(sm, "valor"),
		}
		if ac, ok := sm["ultima_conexao"].(map[string]any); ok {
			svc.Connected = pickStr(ac, "conectado")
			svc.LastConnectedAt = pickStr(ac, "ultima_conexao_datetime", "ultima_conexao")
			svc.LastDisconnectedAt = pickStr(ac, "ultima_desconexao_datetime", "ultima_desconexao")
			svc.LastIPv4 = pickStr(ac, "ultimo_ipv4")
			svc.LastNasIP = pickStr(ac, "ultimo_nas_ip")
			svc.StatusText = pickStr(ac, "status_txt_resumido", "status_txt")
		}
		if eq, ok := sm["equipamento_conexao"].(map[string]any); ok {
			svc.OLT = pickStr(eq, "nome")
		}
		if eq, ok := sm["equipamento_roteamento"].(map[string]any); ok {
			svc.Concentrator = pickStr(eq, "nome")
		}
		if iface, ok := sm["interface"].(map[string]any); ok {
			svc.PON = pickStr(iface, "nome")
		}
		if addr, ok := sm["endereco_instalacao"].(map[string]any); ok {
			svc.InstallAddress = pickStr(addr, "completo")
			svc.City = pickStr(addr, "cidade")
			if coord, ok := addr["coordenadas"].(map[string]any); ok {
				svc.Latitude = pickStr(coord, "latitude")
				svc.Longitude = pickStr(coord, "longitude")
			}
		}
		if svc.Name == "" && svc.Login == "" && svc.ID == "" {
			continue
		}
		out = append(out, svc)
	}
	return out
}

// AttendanceItem atendimento normalizado.
type AttendanceItem struct {
	ID          string `json:"id,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Status      string `json:"status,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Description string `json:"description,omitempty"`
	OpenedAt    string `json:"opened_at,omitempty"`
	ClosedAt    string `json:"closed_at,omitempty"`
	// ClientName/ClientCode só são preenchidos pela varredura "recentes de todos os clientes"
	// (BuildRecentActivity) — na consulta normal por cliente (aba Atendimentos do modal) ficam
	// vazios, o cliente já está óbvio pelo contexto.
	ClientName string `json:"client_name,omitempty"`
	ClientCode string `json:"client_code,omitempty"`
}

type AttendanceResult struct {
	OK        bool             `json:"ok"`
	Message   string           `json:"message,omitempty"`
	Items     []AttendanceItem `json:"items"`
	RawStatus string           `json:"raw_status,omitempty"`
}

func ParseAttendance(raw []byte) AttendanceResult {
	out := AttendanceResult{Items: []AttendanceItem{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	out.RawStatus = strings.TrimSpace(scalarToString(doc["status"]))
	if st := strings.ToLower(out.RawStatus); st == "error" || st == "erro" {
		out.Message = firstNonEmpty(scalarToString(doc["msg"]), scalarToString(doc["message"]), "Erro retornado pela API")
		return out
	}
	for _, it := range extractArray(doc, "atendimentos", "registros", "results", "data", "items") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := AttendanceItem{
			ID:          pickStr(m, "id_atendimento", "id"),
			Protocol:    pickStr(m, "protocolo", "numero_protocolo"),
			Status:      pickStr(m, "status", "situacao"),
			Subject:     pickStr(m, "assunto", "titulo", "motivo", "tipo_atendimento"),
			Description: pickStr(m, "descricao", "observacao", "detalhes", "mensagem"),
			OpenedAt:    pickStr(m, "data_cadastro", "data_abertura"),
			ClosedAt:    pickStr(m, "data_fechamento"),
		}
		if row.Protocol == "" && row.ID == "" && row.Status == "" && row.Subject == "" {
			continue
		}
		out.Items = append(out.Items, row)
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhum atendimento encontrado."
	}
	return out
}

// WorkOrderItem ordem de serviço normalizada.
type WorkOrderItem struct {
	ID          string `json:"id,omitempty"`
	Number      string `json:"number,omitempty"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
	ScheduledAt string `json:"scheduled_at,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	// Ver comentário equivalente em AttendanceItem.
	ClientName string `json:"client_name,omitempty"`
	ClientCode string `json:"client_code,omitempty"`
}

type WorkOrderResult struct {
	OK        bool            `json:"ok"`
	Message   string          `json:"message,omitempty"`
	Items     []WorkOrderItem `json:"items"`
	RawStatus string          `json:"raw_status,omitempty"`
}

func ParseWorkOrder(raw []byte) WorkOrderResult {
	out := WorkOrderResult{Items: []WorkOrderItem{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	out.RawStatus = strings.TrimSpace(scalarToString(doc["status"]))
	if st := strings.ToLower(out.RawStatus); st == "error" || st == "erro" {
		out.Message = firstNonEmpty(scalarToString(doc["msg"]), scalarToString(doc["message"]), "Erro retornado pela API")
		return out
	}
	for _, it := range extractArray(doc, "ordens_servico", "ordem_servico", "ordens", "registros", "results", "data", "items") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := WorkOrderItem{
			ID:          pickStr(m, "id_ordem_servico", "id"),
			Number:      pickStr(m, "numero_ordem_servico", "numero"),
			Status:      pickStr(m, "status", "situacao"),
			Description: pickStr(m, "descricao", "observacao"),
			ScheduledAt: pickStr(m, "data_inicio_programado", "data_agendamento"),
			CreatedAt:   pickStr(m, "data_cadastro"),
		}
		if row.Number == "" {
			row.Number = row.ID
		}
		if row.Number == "" && row.Status == "" && row.Description == "" {
			continue
		}
		out.Items = append(out.Items, row)
	}
	out.OK = true
	if len(out.Items) == 0 {
		out.Message = "Nenhuma ordem de serviço encontrada."
	}
	return out
}

// InvoiceDetailLine item de cobrança dentro de uma fatura (campo `detalhamento`).
type InvoiceDetailLine struct {
	ID            string `json:"id,omitempty"`
	Description   string `json:"description,omitempty"`
	Value         string `json:"value,omitempty"`
	OriginalValue string `json:"original_value,omitempty"`
	Status        string `json:"status,omitempty"`
}

// InvoiceItem fatura normalizada (GET /api/v1/integracao/financeiro/fatura). `Status` vem
// directamente da HubSoft ("vencido" etc.) — usado para classificar vencida/pendente/paga
// sem precisar comparar datas manualmente.
type InvoiceItem struct {
	ID             string              `json:"id,omitempty"`
	Status         string              `json:"status,omitempty"`
	Paid           bool                `json:"paid"`
	Value          string              `json:"value,omitempty"`
	ValuePaid      string              `json:"value_paid,omitempty"`
	DueDate        string              `json:"due_date,omitempty"`
	CreatedAt      string              `json:"created_at,omitempty"`
	PaymentDate    string              `json:"payment_date,omitempty"`
	DocumentDate   string              `json:"document_date,omitempty"`
	NossoNumero    string              `json:"nosso_numero,omitempty"`
	LinhaDigitavel string              `json:"linha_digitavel,omitempty"`
	CodigoBarras   string              `json:"codigo_barras,omitempty"`
	PixCopiaCola   string              `json:"pix_copia_cola,omitempty"`
	BoletoLink     string              `json:"boleto_link,omitempty"`
	Beneficiary    string              `json:"beneficiary,omitempty"`
	ServiceName    string              `json:"service_name,omitempty"`
	Details        []InvoiceDetailLine `json:"details,omitempty"`
}

// FinancialSummary totais calculados a partir das faturas devolvidas — a HubSoft já marca
// cada fatura com `quitado`/`status` (ex. "vencido"), então a classificação é directa, sem
// comparar datas contra "hoje" no NetQuasar.
type FinancialSummary struct {
	Total        int     `json:"total"`
	TotalValue   float64 `json:"total_value"`
	PaidCount    int     `json:"paid_count"`
	PaidValue    float64 `json:"paid_value"`
	OverdueCount int     `json:"overdue_count"`
	OverdueValue float64 `json:"overdue_value"`
	PendingCount int     `json:"pending_count"`
	PendingValue float64 `json:"pending_value"`
}

type FinancialResult struct {
	OK        bool             `json:"ok"`
	Message   string           `json:"message,omitempty"`
	Invoices  []InvoiceItem    `json:"invoices"`
	Summary   FinancialSummary `json:"summary"`
	RawStatus string           `json:"raw_status,omitempty"`
}

func ParseFinancial(raw []byte) FinancialResult {
	out := FinancialResult{Invoices: []InvoiceItem{}}
	if len(raw) == 0 {
		out.Message = "Resposta vazia"
		return out
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		out.Message = "Resposta não é JSON válido"
		return out
	}
	out.RawStatus = strings.TrimSpace(scalarToString(doc["status"]))
	if st := strings.ToLower(out.RawStatus); st == "error" || st == "erro" {
		out.Message = firstNonEmpty(scalarToString(doc["msg"]), scalarToString(doc["message"]), "Erro retornado pela API")
		return out
	}
	for _, it := range extractArray(doc, "faturas", "registros", "results", "data", "items") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		inv := InvoiceItem{
			ID:             pickStr(m, "id_fatura", "id"),
			Status:         pickStr(m, "status"),
			Paid:           pickBool(m, "quitado"),
			Value:          pickStr(m, "valor"),
			ValuePaid:      pickStr(m, "valor_pago"),
			DueDate:        pickStr(m, "data_vencimento"),
			CreatedAt:      pickStr(m, "data_cadastro"),
			PaymentDate:    pickStr(m, "data_pagamento"),
			DocumentDate:   pickStr(m, "data_documento"),
			NossoNumero:    pickStr(m, "nosso_numero"),
			LinhaDigitavel: pickStr(m, "linha_digitavel"),
			CodigoBarras:   pickStr(m, "codigo_barras"),
			PixCopiaCola:   pickStr(m, "pix_copia_cola"),
			BoletoLink:     pickStr(m, "link"),
			Beneficiary:    pickStr(m, "beneficiario"),
		}
		if cli, ok := m["cliente"].(map[string]any); ok {
			if svc, ok := cli["servico"].(map[string]any); ok {
				inv.ServiceName = pickStr(svc, "nome")
			}
		}
		if lines, ok := m["detalhamento"].([]any); ok {
			for _, li := range lines {
				lm, ok := li.(map[string]any)
				if !ok {
					continue
				}
				inv.Details = append(inv.Details, InvoiceDetailLine{
					ID:            pickStr(lm, "id_cobranca", "id"),
					Description:   pickStr(lm, "descricao"),
					Value:         pickStr(lm, "valor"),
					OriginalValue: pickStr(lm, "valor_original"),
					Status:        pickStr(lm, "status"),
				})
			}
		}
		if inv.ID == "" && inv.DueDate == "" && inv.Value == "" {
			continue
		}
		out.Invoices = append(out.Invoices, inv)

		val := parseBRFloat(inv.Value)
		out.Summary.Total++
		out.Summary.TotalValue += val
		switch {
		case inv.Paid:
			out.Summary.PaidCount++
			if paidVal := parseBRFloat(inv.ValuePaid); paidVal > 0 {
				out.Summary.PaidValue += paidVal
			} else {
				out.Summary.PaidValue += val
			}
		case strings.EqualFold(strings.TrimSpace(inv.Status), "vencido"):
			out.Summary.OverdueCount++
			out.Summary.OverdueValue += val
		default:
			out.Summary.PendingCount++
			out.Summary.PendingValue += val
		}
	}
	// Mais recentes primeiro (vencimento decrescente) — os boletos mais antigos ficam no fim
	// da lista, como pedido pelo utilizador.
	sort.SliceStable(out.Invoices, func(i, j int) bool {
		return parseBRDate(out.Invoices[i].DueDate).After(parseBRDate(out.Invoices[j].DueDate))
	})
	out.OK = true
	if len(out.Invoices) == 0 {
		out.Message = "Nenhuma fatura encontrada."
	}
	return out
}

func pickBool(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// parseBRDate lê datas devolvidas pela HubSoft — formato varia por endpoint (confirmado em
// produção): "data_vencimento" em /financeiro vem DD/MM/YYYY sem hora, "data_cadastro" em
// /ordem_servico vem DD/MM/YYYY HH:MM:SS; outros campos usam ISO. Tenta os formatos
// conhecidos em ordem; datas que não parseiam ficam no fim (zero value é a mais antiga
// possível), o que é seguro para ordenar "mais recente primeiro".
func parseBRDate(s string) time.Time {
	s = strings.TrimSpace(s)
	layouts := []string{
		"02/01/2006 15:04:05",
		"02/01/2006",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseBRFloat aceita tanto "123.45" (formato que a API devolve para `valor`) quanto
// "1.234,56" (formato BR) — usado só para somar totais no resumo, não para exibição.
func parseBRFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// --- helpers pequenos (deliberadamente próprios deste pacote — não importa
// internal/integrationconsumer para manter a Hubsoft isolada do código do IXC) -------------

func pickStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s := scalarToString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func scalarToString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func paramKVs(m map[string]string) []integrationhttp.ParamKV {
	out := make([]integrationhttp.ParamKV, 0, len(m))
	for k, v := range m {
		out = append(out, integrationhttp.ParamKV{Key: k, Value: v})
	}
	return out
}

// ResponseBodyBytes normaliza o preview de resposta (a API às vezes envolve o corpo numa
// string JSON escapada — mesma proteção já usada no consumidor genérico).
func ResponseBodyBytes(res integrationhttp.RunResult) []byte {
	body := []byte(res.ResponsePreview)
	if len(body) >= 2 && body[0] == '"' && body[len(body)-1] == '"' {
		var s string
		if json.Unmarshal(body, &s) == nil {
			return []byte(s)
		}
	}
	return body
}

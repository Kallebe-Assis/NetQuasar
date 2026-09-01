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
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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

// SearchClientByConnectionTerm localiza um cliente por IPv4/MAC/login via
// /cliente/extrato_conexao (busca directa e rápida — ver reportFromConnectionExtract) e, a
// partir do código do cliente encontrado, busca o cartão COMPLETO com SearchClients (mesma
// chamada já usada pela Consulta normal). Substitui SearchByIPOrMAC (scan por letras do nome,
// lento e sem garantia de achar) nesse mesmo uso — mantida a assinatura/forma de resposta
// (ClientSearchResult) para não mexer na tela de Consulta.
func SearchClientByConnectionTerm(ctx context.Context, cfg Config, token, termo, kind string) (ClientSearchResult, error) {
	busca := kind // "ipv4" ou "mac" — mesmos valores já usados pelo chamador
	needle := strings.TrimSpace(termo)
	if kind == "mac" {
		needle = normalizeMAC(needle)
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/extrato_conexao",
		QueryParams: paramKVs(map[string]string{"busca": busca, "termo_busca": needle, "limit": "5"}),
	})
	if !res.OK && res.StatusCode != 404 {
		return ClientSearchResult{}, fmt.Errorf("%s", firstNonEmpty(res.ErrorMessage, fmt.Sprintf("HTTP %d", res.StatusCode)))
	}
	var doc map[string]any
	_ = json.Unmarshal(ResponseBodyBytes(res), &doc)
	var codigo string
	for _, it := range extractArray(doc, "registros", "dados", "data") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if cli, ok := m["cliente"].(map[string]any); ok {
			if c := pickStr(cli, "codigo_cliente"); c != "" {
				codigo = c
				break
			}
		}
	}
	if codigo == "" {
		label := map[string]string{"ipv4": "IPv4", "mac": "MAC"}[kind]
		return ClientSearchResult{OK: true, Clients: []ClientCard{}, Message: fmt.Sprintf("Nenhum cliente encontrado com esse %s.", label)}, nil
	}
	result, _ := SearchClients(ctx, cfg, token, "codigo_cliente", codigo, true)
	return result, nil
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

// --- Detalhe completo (aba Relatório/Atendimentos/O.S. → "Ver mais") -------------------------
//
// /cliente/atendimento e /cliente/ordem_servico (usados acima por SearchAttendance/
// SearchWorkOrders) devolvem, quando buscados por protocolo/número (um único registo), um
// formato BEM mais rico que /atendimento/todos e /ordem_servico/todos (usados pela lista —
// BuildRecentActivityFast): campos de texto livre próprios (descricao_abertura — não existe em
// /atendimento/todos, só descricao_fechamento) e, com relacoes=atendimento_mensagem /
// ordem_servico_mensagem, a conversa completa. Por isso o "ver mais" faz UM pedido novo e
// específico (por protocolo/número, limit=1) em vez de reaproveitar os dados já carregados na
// lista — mais rico E mais leve que buscar mensagens de toda a lista de uma vez.

// SupportMessage uma mensagem da conversa de um atendimento ou O.S.
type SupportMessage struct {
	Text string `json:"text,omitempty"`
	At   string `json:"at,omitempty"`
}

func extractSupportMessages(m map[string]any, relKey string) []SupportMessage {
	arr, ok := m[relKey].([]any)
	if !ok {
		return nil
	}
	out := make([]SupportMessage, 0, len(arr))
	for _, it := range arr {
		mm, ok := it.(map[string]any)
		if !ok {
			continue
		}
		text := pickStr(mm, "mensagem", "texto", "descricao", "conteudo")
		if text == "" {
			continue
		}
		out = append(out, SupportMessage{
			Text: text,
			At:   firstNonEmpty(pickStr(mm, "data_cadastro_br"), pickStr(mm, "data_cadastro")),
		})
	}
	return out
}

// AttendanceDetail detalhe completo de UM atendimento (via /cliente/atendimento?busca=protocolo).
type AttendanceDetail struct {
	OK                bool             `json:"ok"`
	Message           string           `json:"message,omitempty"`
	ID                string           `json:"id,omitempty"`
	Protocol          string           `json:"protocol,omitempty"`
	Status            string           `json:"status,omitempty"`
	Subject           string           `json:"subject,omitempty"`
	Description       string           `json:"description,omitempty"`
	OpenedAt          string           `json:"opened_at,omitempty"`
	OpenedByUser      string           `json:"opened_by_user,omitempty"`
	ClosedAt          string           `json:"closed_at,omitempty"`
	ClosedByUser      string           `json:"closed_by_user,omitempty"`
	ClosedDescription string           `json:"closed_description,omitempty"`
	ClosingReason     string           `json:"closing_reason,omitempty"`
	Sector            string           `json:"sector,omitempty"`
	ResponsibleUser   string           `json:"responsible_user,omitempty"`
	ClientName        string           `json:"client_name,omitempty"`
	ClientCode        string           `json:"client_code,omitempty"`
	PlanName          string           `json:"plan_name,omitempty"`
	WorkOrders        []WorkOrderItem  `json:"work_orders,omitempty"`
	Messages          []SupportMessage `json:"messages"`
}

// FetchAttendanceDetail busca UM atendimento pelo protocolo, com a conversa completa.
func FetchAttendanceDetail(ctx context.Context, cfg Config, token, protocolo string) (AttendanceDetail, error) {
	protocolo = strings.TrimSpace(protocolo)
	if protocolo == "" {
		return AttendanceDetail{}, fmt.Errorf("protocolo obrigatório")
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/atendimento",
		QueryParams: paramKVs(map[string]string{
			"busca": "protocolo", "termo_busca": protocolo, "limit": "1",
			"relacoes": "atendimento_mensagem",
		}),
	})
	if !res.OK {
		return AttendanceDetail{}, fmt.Errorf("hubsoft: %s", firstNonEmpty(res.ErrorMessage, "falha ao consultar atendimento"))
	}
	body := ResponseBodyBytes(res)
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return AttendanceDetail{}, fmt.Errorf("resposta inválida da HubSoft")
	}
	arr := extractArray(doc, "atendimentos", "registros", "results", "data", "items")
	if len(arr) == 0 {
		return AttendanceDetail{OK: true, Message: "Atendimento não encontrado."}, nil
	}
	m, _ := arr[0].(map[string]any)
	out := AttendanceDetail{
		OK: true,
		ID: pickStr(m, "id_atendimento"), Protocol: pickStr(m, "protocolo"),
		Status: pickStr(m, "status"), Subject: pickStr(m, "tipo_atendimento"),
		Description: pickStr(m, "descricao_abertura"),
		OpenedAt:    pickStr(m, "data_cadastro"), OpenedByUser: pickStr(m, "usuario_abertura"),
		ClosedAt: pickStr(m, "data_fechamento"), ClosedByUser: pickStr(m, "usuario_fechamento"),
		ClosedDescription: pickStr(m, "descricao_fechamento"),
		ClosingReason:     pickStr(m, "motivo_fechamento"),
		Sector:            pickStr(m, "setor_responsavel"),
		ResponsibleUser:   firstNonEmpty(pickStr(m, "usuario_responsavel"), pickStr(m, "usuario_abertura")),
		Messages:          extractSupportMessages(m, "atendimento_mensagem"),
	}
	if cli, ok := m["cliente"].(map[string]any); ok {
		out.ClientName = pickStr(cli, "nome_razaosocial")
		out.ClientCode = pickStr(cli, "codigo_cliente")
	}
	if sv, ok := m["servico"].(map[string]any); ok {
		out.PlanName = pickStr(sv, "nome")
	}
	if osArr, ok := m["ordens_servico"].([]any); ok {
		for _, oi := range osArr {
			om, ok := oi.(map[string]any)
			if !ok {
				continue
			}
			wo := WorkOrderItem{
				ID: pickStr(om, "id_ordem_servico"), Number: pickStr(om, "numero_ordem_servico"),
				Status:      pickStr(om, "status"),
				Description: pickStr(om, "descricao_abertura"),
				CreatedAt:   pickStr(om, "data_cadastro"), ScheduledAt: pickStr(om, "data_inicio_programado"),
			}
			if sv, ok := om["servico"].(map[string]any); ok {
				wo.PlanName = pickStr(sv, "nome")
			}
			out.WorkOrders = append(out.WorkOrders, wo)
		}
	}
	return out, nil
}

// WorkOrderDetail detalhe completo de UMA O.S. (via /cliente/ordem_servico?busca=numero_ordem_servico).
type WorkOrderDetail struct {
	OK                 bool             `json:"ok"`
	Message            string           `json:"message,omitempty"`
	ID                 string           `json:"id,omitempty"`
	Number             string           `json:"number,omitempty"`
	Type               string           `json:"type,omitempty"`
	Status             string           `json:"status,omitempty"`
	StatusClosed       string           `json:"status_closed,omitempty"`
	Description        string           `json:"description,omitempty"`
	ServiceDescription string           `json:"service_description,omitempty"`
	ClosedDescription  string           `json:"closed_description,omitempty"`
	ScheduledStart     string           `json:"scheduled_start,omitempty"`
	ScheduledEnd       string           `json:"scheduled_end,omitempty"`
	ExecutedStart      string           `json:"executed_start,omitempty"`
	ExecutedEnd        string           `json:"executed_end,omitempty"`
	CreatedAt          string           `json:"created_at,omitempty"`
	OpenedByUser       string           `json:"opened_by_user,omitempty"`
	ClosedByUser       string           `json:"closed_by_user,omitempty"`
	ClientName         string           `json:"client_name,omitempty"`
	ClientCode         string           `json:"client_code,omitempty"`
	PlanName           string           `json:"plan_name,omitempty"`
	AttendanceProtocol string           `json:"attendance_protocol,omitempty"`
	Messages           []SupportMessage `json:"messages"`
}

// FetchWorkOrderDetail busca UMA O.S. pelo número, com a conversa completa.
func FetchWorkOrderDetail(ctx context.Context, cfg Config, token, numero string) (WorkOrderDetail, error) {
	numero = strings.TrimSpace(numero)
	if numero == "" {
		return WorkOrderDetail{}, fmt.Errorf("número da O.S. obrigatório")
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/ordem_servico",
		QueryParams: paramKVs(map[string]string{
			"busca": "numero_ordem_servico", "termo_busca": numero, "limit": "1",
			"relacoes": "ordem_servico_mensagem", "exibir_atendimento": "true",
		}),
	})
	if !res.OK {
		return WorkOrderDetail{}, fmt.Errorf("hubsoft: %s", firstNonEmpty(res.ErrorMessage, "falha ao consultar ordem de serviço"))
	}
	body := ResponseBodyBytes(res)
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return WorkOrderDetail{}, fmt.Errorf("resposta inválida da HubSoft")
	}
	arr := extractArray(doc, "ordens_servico", "ordem_servico", "ordens", "registros", "results", "data", "items")
	if len(arr) == 0 {
		return WorkOrderDetail{OK: true, Message: "Ordem de serviço não encontrada."}, nil
	}
	m, _ := arr[0].(map[string]any)
	out := WorkOrderDetail{
		OK: true,
		ID: pickStr(m, "id_ordem_servico"), Number: pickStr(m, "numero_ordem_servico"),
		Type: pickStr(m, "tipo"), Status: pickStr(m, "status"), StatusClosed: pickStr(m, "status_fechamento"),
		Description: pickStr(m, "descricao_abertura"), ServiceDescription: pickStr(m, "descricao_servico"),
		ClosedDescription: pickStr(m, "descricao_fechamento"),
		ScheduledStart:    pickStr(m, "data_inicio_programado"), ScheduledEnd: pickStr(m, "data_termino_programado"),
		ExecutedStart: pickStr(m, "data_inicio_executado"), ExecutedEnd: pickStr(m, "data_termino_executado"),
		CreatedAt: pickStr(m, "data_cadastro"), OpenedByUser: pickStr(m, "usuario_abertura"),
		ClosedByUser: pickStr(m, "usuario_fechamento"),
		Messages:     extractSupportMessages(m, "ordem_servico_mensagem"),
	}
	if cli, ok := m["cliente"].(map[string]any); ok {
		out.ClientName = pickStr(cli, "nome_razaosocial")
		out.ClientCode = pickStr(cli, "codigo_cliente")
	}
	if sv, ok := m["servico"].(map[string]any); ok {
		out.PlanName = pickStr(sv, "nome")
	}
	if at, ok := m["atendimento"].(map[string]any); ok {
		out.AttendanceProtocol = pickStr(at, "protocolo")
	}
	return out, nil
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

// ResendInvoiceEmail reenvia UMA fatura por e-mail (POST /cliente/financeiro/enviar_email) — a
// própria HubSoft dispara o envio (servidor de e-mail dela, não o NetQuasar); "ver mais" da aba
// Financeiro. extraEmails é opcional (além dos e-mails já cadastrados do cliente).
func ResendInvoiceEmail(ctx context.Context, cfg Config, token, idFatura string, extraEmails []string) error {
	idFatura = strings.TrimSpace(idFatura)
	if idFatura == "" {
		return fmt.Errorf("id_fatura obrigatório")
	}
	body := map[string]any{"id_fatura": idFatura}
	if len(extraEmails) > 0 {
		body["email_adicional"] = extraEmails
	}
	bodyJSON, _ := json.Marshal(body)
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "POST", Path: "/api/v1/integracao/cliente/financeiro/enviar_email",
		BodyTemplate: string(bodyJSON), BodyType: "json",
	})
	if !res.OK {
		var doc map[string]any
		if json.Unmarshal(ResponseBodyBytes(res), &doc) == nil {
			if msg := pickStr(doc, "msg", "message"); msg != "" {
				return fmt.Errorf("%s", msg)
			}
		}
		return fmt.Errorf("hubsoft: %s", firstNonEmpty(res.ErrorMessage, "falha ao reenviar fatura"))
	}
	return nil
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

// --- Paginação real (endpoints "todos"/"listar") ----------------------------------------------
//
// Ao contrário de /cliente (Consultar) e dos endpoints /cliente/atendimento, /cliente/
// ordem_servico, /cliente/financeiro (todos exigem busca+termo_busca por CLIENTE e não paginam
// de verdade — ver comentário no início do ficheiro), a HubSoft tem uma segunda família de
// endpoints "todos"/"listar" que pagina de verdade (paginacao.pagina_atual/ultima_pagina/
// total_registros, até 500 itens/página) e não depende de varrer nome por letras:
//   - GET /api/v1/integracao/cliente/todos            (clientes+serviços, filtros servico_status/cancelado)
//   - GET /api/v1/integracao/atendimento/todos        (todos os atendimentos, filtro por data obrigatório)
//   - GET /api/v1/integracao/ordem_servico/todos      (todas as O.S., filtro por data obrigatório)
//   - GET /api/v1/integracao/financeiro/fatura        (todas as faturas, filtro por data)
// Confirmado na documentação oficial (https://docs.hubsoft.com.br) — a collection completa foi
// consultada directamente (API de docs deles, não só o texto da página) para confirmar nomes de
// parâmetros e formato de resposta antes de implementar isto.

// maxReportPages teto de segurança: 40 páginas × 500 itens = até 20.000 registos por relatório,
// dá para cobrir a base de qualquer ISP de porte médio/grande num pedido só, sem risco de loop
// infinito nem de estourar o timeout do handler HTTP que chama isto.
const maxReportPages = 40

// fetchAllPages percorre um endpoint paginado da HubSoft (mesma estrutura "paginacao" em todos
// os 4 endpoints "todos"/"listar" acima) até esgotar as páginas ou atingir maxPages, juntando o
// array indicado por um dos arrayKeys (tenta cada um — os endpoints usam nomes diferentes:
// "clientes", "atendimentos", "ordens_servico", "faturas"). baseParams não deve incluir "pagina"
// nem "itens_por_pagina" (preenchidos aqui a cada volta). Devolve também total_registros
// (contagem real da API, pode ser maior que len(items) se o teto de páginas foi atingido).
func fetchAllPages(ctx context.Context, cfg Config, token, path string, baseParams map[string]string, maxPages int, arrayKeys ...string) (items []map[string]any, totalRegistros int, err error) {
	if maxPages <= 0 {
		maxPages = maxReportPages
	}
	// 500/página (o máximo aceite) com relações pesadas (endereço, última conexão, etc.) por
	// vezes estoura o timeout HTTP por página antes da HubSoft terminar de montar a resposta —
	// confirmado em produção. 100/página fica bem dentro da margem, só custa mais idas e vindas
	// (o rate limit da API, 20 req/s com rajada de 200, tem folga de sobra para isso).
	const perPage = 100
	// O timeout por omissão do motor HTTP genérico (15s, quando cfg.Timeout não é definido) é
	// curto demais para uma página de 100 itens com relações — usa um piso mais alto só aqui.
	reqCfg := cfg
	if reqCfg.Timeout < 45000 {
		reqCfg.Timeout = 45000
	}
	for page := 0; page < maxPages; page++ {
		q := make(map[string]string, len(baseParams)+2)
		for k, v := range baseParams {
			q[k] = v
		}
		q["pagina"] = strconv.Itoa(page)
		q["itens_por_pagina"] = strconv.Itoa(perPage)
		res := integrationhttp.Execute(ctx, reqCfg.integ(token), integrationhttp.RequestConfig{
			Method: "GET", Path: path, QueryParams: paramKVs(q),
		})
		if !res.OK {
			if page == 0 {
				return nil, 0, fmt.Errorf("%s", firstNonEmpty(res.ErrorMessage, fmt.Sprintf("HTTP %d", res.StatusCode)))
			}
			break // já reunimos algumas páginas — devolve o que há em vez de descartar tudo
		}
		var doc map[string]any
		if jsonErr := json.Unmarshal(ResponseBodyBytes(res), &doc); jsonErr != nil {
			break
		}
		arr := extractArray(doc, arrayKeys...)
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				items = append(items, m)
			}
		}
		if pg, ok := doc["paginacao"].(map[string]any); ok {
			if tr, ok := pg["total_registros"].(float64); ok {
				totalRegistros = int(tr)
			}
			last := scalarToString(pg["ultima_pagina"])
			cur := scalarToString(pg["pagina_atual"])
			if last != "" && cur != "" && cur == last {
				break
			}
		}
		if len(arr) == 0 {
			break
		}
	}
	if totalRegistros < len(items) {
		totalRegistros = len(items)
	}
	return items, totalRegistros, nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// defaultPeriod aplica os últimos 30 dias quando from/to não vêm preenchidos — os endpoints
// "todos" de atendimento/O.S./financeiro exigem um intervalo de datas, e um relatório sem
// período nenhum não faria sentido (varreria a base inteira desde sempre).
func defaultPeriod(from, to string) (string, string) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		now := time.Now()
		to = now.Format("2006-01-02")
		from = now.AddDate(0, 0, -30).Format("2006-01-02")
	}
	return from, to
}

// brStateUF mapeia sigla (UF) -> nome completo dos estados brasileiros — usado por stateVariants
// para o filtro de estado casar independentemente de o tenant guardar a sigla ou o nome completo
// no campo "estado" (confirmado ao vivo: varia por tenant).
var brStateUF = map[string]string{
	"ac": "acre", "al": "alagoas", "ap": "amapa", "am": "amazonas", "ba": "bahia",
	"ce": "ceara", "df": "distrito federal", "es": "espirito santo", "go": "goias",
	"ma": "maranhao", "mt": "mato grosso", "ms": "mato grosso do sul", "mg": "minas gerais",
	"pa": "para", "pb": "paraiba", "pr": "parana", "pe": "pernambuco", "pi": "piaui",
	"rj": "rio de janeiro", "rn": "rio grande do norte", "rs": "rio grande do sul",
	"ro": "rondonia", "rr": "roraima", "sc": "santa catarina", "sp": "sao paulo",
	"se": "sergipe", "to": "tocantins",
}

// stateVariants devolve as formas (minúsculas, sem acento) contra as quais tentar
// strings.Contains: o termo digitado tal como veio, mais a sigla<->nome completo quando aplicável
// (ex.: "RJ" também casa "rio de janeiro"; "Rio de Janeiro" também casa "rj"). Devolve vazio
// quando o termo digitado é vazio (sem filtro).
func stateVariants(input string) []string {
	in := strings.ToLower(strings.TrimSpace(stripAccents(input)))
	if in == "" {
		return nil
	}
	out := []string{in}
	if full, ok := brStateUF[in]; ok {
		out = append(out, full)
	}
	for uf, full := range brStateUF {
		if full == in {
			out = append(out, uf)
			break
		}
	}
	return out
}

// stripAccents troca as vogais acentuadas mais comuns em nomes de estado (ç/ã/é/í/…) pela forma
// sem acento — a resposta da API às vezes vem sem, então normaliza dos dois lados antes de
// comparar (evita "não bater" só por causa de acentuação).
func stripAccents(s string) string {
	repl := strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ç", "c",
		"Á", "a", "À", "a", "Â", "a", "Ã", "a", "Ä", "a",
		"É", "e", "È", "e", "Ê", "e", "Ë", "e",
		"Í", "i", "Ì", "i", "Î", "i", "Ï", "i",
		"Ó", "o", "Ò", "o", "Ô", "o", "Õ", "o", "Ö", "o",
		"Ú", "u", "Ù", "u", "Û", "u", "Ü", "u",
		"Ç", "c",
	)
	return repl.Replace(s)
}

// --- Relatório: clientes/serviços filtrados (aba Relatório → Clientes) -------------------------

// ReportServiceRow uma linha do relatório — um por serviço/login (um cliente pode ter mais de
// um). É deliberadamente enxuto (login/IPv4/MAC/status) — os dados completos só são carregados
// ao clicar no cliente (reaproveita SearchClients com detailed=true, já existente).
type ReportServiceRow struct {
	ClientID     string `json:"client_id,omitempty"`
	ClientCode   string `json:"client_code,omitempty"`
	ClientName   string `json:"client_name,omitempty"`
	Document     string `json:"document,omitempty"`
	ServiceID    string `json:"service_id,omitempty"`
	ServiceName  string `json:"service_name,omitempty"`
	Login        string `json:"login,omitempty"`
	IPv4         string `json:"ipv4,omitempty"`
	MAC          string `json:"mac,omitempty"`
	Status       string `json:"status,omitempty"`
	StatusPrefix string `json:"status_prefix,omitempty"`
	City         string `json:"city,omitempty"`
	State        string `json:"state,omitempty"`
	Neighborhood string `json:"neighborhood,omitempty"`
	Connected    string `json:"connected,omitempty"` // "true"/"false"/"" (sem dado)
}

// ReportListFilter — Estado/Cidade/Bairro não são parâmetros nativos da API (só existe filtro
// por código IBGE da cidade, `ibge_cidade_instalacao`, que exigiria uma tabela de municípios só
// para traduzir nome→código); em vez disso, filtramos aqui do lado do NetQuasar, depois de
// buscar (com paginação real, não é amostra) — o volume já vem reduzido pelos filtros nativos
// (ServiceStatus/Cancelado) quando preenchidos.
type ReportListFilter struct {
	ServiceStatus string
	Cancelado     string // "sim" | "nao" | ""
	State         string // UF, ex.: "MG" — comparado contra o campo `estado` da HubSoft
	City          string // substring, case-insensitive
	Neighborhood  string // substring, case-insensitive
	IPv4          string // preenchido → usa extrato_conexao (busca directa, não pagina tudo)
	MAC           string
	Login         string
}

type ReportListResult struct {
	OK           bool               `json:"ok"`
	Message      string             `json:"message,omitempty"`
	Rows         []ReportServiceRow `json:"rows"`
	TotalScanned int                `json:"total_scanned"`
	Truncated    bool               `json:"truncated,omitempty"`
}

// ListClientServiceReport ponto de entrada do relatório de clientes/serviços — quando IPv4/MAC/
// Login estão preenchidos, usa a busca directa (extrato_conexao, rápida, poucos resultados);
// caso contrário pagina /cliente/todos com os filtros nativos disponíveis e filtra
// Estado/Cidade/Bairro do lado do NetQuasar.
func ListClientServiceReport(ctx context.Context, cfg Config, token string, filter ReportListFilter) (ReportListResult, error) {
	if strings.TrimSpace(filter.IPv4) != "" || strings.TrimSpace(filter.MAC) != "" || strings.TrimSpace(filter.Login) != "" {
		return reportFromConnectionExtract(ctx, cfg, token, filter)
	}
	return reportFromClientsTodos(ctx, cfg, token, filter)
}

func reportFromConnectionExtract(ctx context.Context, cfg Config, token string, filter ReportListFilter) (ReportListResult, error) {
	busca, termo := "", ""
	switch {
	case strings.TrimSpace(filter.IPv4) != "":
		busca, termo = "ipv4", strings.TrimSpace(filter.IPv4)
	case strings.TrimSpace(filter.MAC) != "":
		busca, termo = "mac", normalizeMAC(filter.MAC)
	case strings.TrimSpace(filter.Login) != "":
		busca, termo = "login", strings.TrimSpace(filter.Login)
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente/extrato_conexao",
		QueryParams: paramKVs(map[string]string{"busca": busca, "termo_busca": termo, "limit": "50"}),
	})
	if !res.OK && res.StatusCode != 404 {
		return ReportListResult{}, fmt.Errorf("%s", firstNonEmpty(res.ErrorMessage, fmt.Sprintf("HTTP %d", res.StatusCode)))
	}
	var doc map[string]any
	_ = json.Unmarshal(ResponseBodyBytes(res), &doc)
	seen := map[string]bool{}
	rows := []ReportServiceRow{}
	for _, it := range extractArray(doc, "registros", "dados", "data") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := ReportServiceRow{
			Login: pickStr(m, "username", "login"),
			IPv4:  pickStr(m, "framedipaddress"),
			MAC:   pickStr(m, "callingstationid"),
		}
		if cli, ok := m["cliente"].(map[string]any); ok {
			row.ClientID = pickStr(cli, "id_cliente")
			row.ClientCode = pickStr(cli, "codigo_cliente")
			row.ClientName = pickStr(cli, "nome_razaosocial")
			row.Document = formatCPFCNPJ(pickStr(cli, "cpf_cnpj"))
		}
		if svc, ok := m["servico"].(map[string]any); ok {
			row.ServiceName = pickStr(svc, "nome")
			row.Status = pickStr(svc, "status")
			row.StatusPrefix = pickStr(svc, "status_prefixo")
		}
		if pickStr(m, "acctstoptime") == "" {
			row.Connected = "true" // sem hora de término = sessão ainda activa
		} else {
			row.Connected = "false"
		}
		key := row.ClientCode + "|" + row.Login + "|" + row.IPv4
		if row.ClientCode == "" && row.Login == "" {
			continue
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		rows = append(rows, row)
	}
	msg := ""
	if len(rows) == 0 {
		msg = "Nenhuma conexão encontrada para esse termo (extrato de conexão cobre por omissão os últimos 30 dias)."
	}
	return ReportListResult{OK: true, Rows: rows, TotalScanned: len(rows), Message: msg}, nil
}

func reportFromClientsTodos(ctx context.Context, cfg Config, token string, filter ReportListFilter) (ReportListResult, error) {
	q := map[string]string{"relacoes": "endereco_instalacao,ultima_conexao"}
	if s := strings.TrimSpace(filter.ServiceStatus); s != "" {
		q["servico_status"] = s
	}
	if c := strings.TrimSpace(filter.Cancelado); c != "" {
		q["cancelado"] = c
	}
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", q, maxReportPages, "clientes")
	if err != nil {
		return ReportListResult{}, err
	}

	// Contains (não EqualFold) porque o campo "estado" vem ora como sigla ("RJ"), ora como nome
	// completo ("RIO DE JANEIRO") dependendo do tenant/dado — confirmado ao vivo em produção.
	// stateVariants expande a sigla digitada para o nome completo (e vice-versa) para casar com
	// qualquer uma das duas convenções.
	stateWant := stateVariants(strings.TrimSpace(filter.State))
	cityWant := strings.ToLower(strings.TrimSpace(filter.City))
	neighWant := strings.ToLower(strings.TrimSpace(filter.Neighborhood))

	rows := []ReportServiceRow{}
	for _, m := range items {
		clientID := pickStr(m, "id_cliente")
		clientCode := pickStr(m, "codigo_cliente")
		clientName := pickStr(m, "nome_razaosocial")
		doc := formatCPFCNPJ(pickStr(m, "cpf_cnpj"))
		svcArr, _ := m["servicos"].([]any)
		for _, sit := range svcArr {
			sm, ok := sit.(map[string]any)
			if !ok {
				continue
			}
			row := ReportServiceRow{
				ClientID: clientID, ClientCode: clientCode, ClientName: clientName, Document: doc,
				ServiceID:    pickStr(sm, "id_cliente_servico"),
				ServiceName:  pickStr(sm, "nome"),
				Login:        pickStr(sm, "login"),
				IPv4:         pickStr(sm, "ipv4"),
				MAC:          pickStr(sm, "mac_addr", "phy_addr"),
				Status:       pickStr(sm, "status"),
				StatusPrefix: pickStr(sm, "status_prefixo"),
			}
			if addr, ok := sm["endereco_instalacao"].(map[string]any); ok {
				row.City = pickStr(addr, "cidade")
				row.State = pickStr(addr, "estado")
				row.Neighborhood = pickStr(addr, "bairro")
			}
			if ac, ok := sm["ultima_conexao"].(map[string]any); ok {
				row.Connected = pickStr(ac, "conectado")
			}
			if len(stateWant) > 0 {
				got := strings.ToLower(strings.TrimSpace(stripAccents(row.State)))
				matched := false
				for _, w := range stateWant {
					if strings.Contains(got, w) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if cityWant != "" && !strings.Contains(strings.ToLower(row.City), cityWant) {
				continue
			}
			if neighWant != "" && !strings.Contains(strings.ToLower(row.Neighborhood), neighWant) {
				continue
			}
			rows = append(rows, row)
		}
	}
	return ReportListResult{OK: true, Rows: rows, TotalScanned: total, Truncated: total > len(items)}, nil
}

// --- Relatório: atendimentos por período (aba Relatório → Atendimentos) ------------------------

type AttendancePeriodReport struct {
	OK        bool         `json:"ok"`
	Message   string       `json:"message,omitempty"`
	From      string       `json:"from"`
	To        string       `json:"to"`
	Total     int          `json:"total"`
	Open      int          `json:"open"`
	Closed    int          `json:"closed"`
	ClosedPct float64      `json:"closed_pct"`
	ByStatus  []NamedCount `json:"by_status"`
	Truncated bool         `json:"truncated,omitempty"`
}

// BuildAttendancePeriodReport varre TODOS os atendimentos abertos no período (não uma amostra —
// /atendimento/todos pagina de verdade) e calcula abertos/realizados + repartição por status.
func BuildAttendancePeriodReport(ctx context.Context, cfg Config, token, from, to string) AttendancePeriodReport {
	from, to = defaultPeriod(from, to)
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/atendimento/todos",
		map[string]string{"data_inicio": from, "data_fim": to}, maxReportPages, "atendimentos")
	if err != nil {
		return AttendancePeriodReport{Message: "Falha ao coletar atendimentos: " + err.Error(), From: from, To: to}
	}
	statusCount := map[string]int{}
	open, closed := 0, 0
	for _, m := range items {
		label := "Sem status"
		if st, ok := m["status"].(map[string]any); ok {
			label = firstNonEmpty(pickStr(st, "descricao"), label)
		} else {
			label = firstNonEmpty(pickStr(m, "status"), label)
		}
		statusCount[label]++
		if strings.TrimSpace(pickStr(m, "data_fechamento")) == "" {
			open++
		} else {
			closed++
		}
	}
	n := len(items)
	pct := 0.0
	if n > 0 {
		pct = round2(float64(closed) / float64(n) * 100)
	}
	return AttendancePeriodReport{
		OK: true, From: from, To: to, Total: total, Open: open, Closed: closed, ClosedPct: pct,
		ByStatus: topNamedCounts(statusCount, nil, 20), Truncated: total > n,
	}
}

// --- Relatório: ordens de serviço por período e por técnico (aba Relatório → O.S.) -------------

type TechnicianStat struct {
	Technician    string  `json:"technician"`
	Total         int     `json:"total"`
	Finished      int     `json:"finished"`
	PctOfFinished float64 `json:"pct_of_total_finished"` // % deste técnico sobre o TOTAL finalizado (todos os técnicos juntos)
}

type WorkOrderPeriodReport struct {
	OK           bool             `json:"ok"`
	Message      string           `json:"message,omitempty"`
	From         string           `json:"from"`
	To           string           `json:"to"`
	Total        int              `json:"total"`
	Finished     int              `json:"finished"`
	FinishedPct  float64          `json:"finished_pct"`
	ByStatus     []NamedCount     `json:"by_status"`
	ByTechnician []TechnicianStat `json:"by_technician"`
	Truncated    bool             `json:"truncated,omitempty"`
}

var finishedWorkOrderStatuses = map[string]bool{"finalizado": true, "finalizada": true, "concluido": true, "concluído": true}

func extractTechnicianName(m map[string]any) string {
	tryField := func(key string) string {
		switch v := m[key].(type) {
		case []any:
			var names []string
			for _, it := range v {
				if tm, ok := it.(map[string]any); ok {
					if n := pickStr(tm, "nome", "name", "display", "descricao"); n != "" {
						names = append(names, n)
					}
				}
			}
			return strings.Join(names, ", ")
		case map[string]any:
			return pickStr(v, "nome", "name", "display", "descricao")
		}
		return ""
	}
	for _, key := range []string{"tecnico", "tecnicos", "usuarios_responsaveis", "usuario_responsavel"} {
		if n := tryField(key); n != "" {
			return n
		}
	}
	return pickStr(m, "tecnico", "responsavel")
}

// BuildWorkOrderPeriodReport varre TODAS as O.S. abertas/cadastradas no período
// (/ordem_servico/todos pagina de verdade) e calcula finalizadas/total + repartição por status e
// por técnico responsável (quantidade finalizada e % sobre o total finalizado por todos).
func BuildWorkOrderPeriodReport(ctx context.Context, cfg Config, token, from, to string) WorkOrderPeriodReport {
	from, to = defaultPeriod(from, to)
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/ordem_servico/todos",
		map[string]string{"data_inicio": from, "data_fim": to, "relacoes": "tecnico,tecnicos"}, maxReportPages, "ordens_servico", "ordem_servico", "ordens")
	if err != nil {
		return WorkOrderPeriodReport{Message: "Falha ao coletar ordens de serviço: " + err.Error(), From: from, To: to}
	}
	statusCount := map[string]int{}
	techTotal := map[string]int{}
	techFinished := map[string]int{}
	finishedTotal := 0
	for _, m := range items {
		status := firstNonEmpty(pickStr(m, "status"), "Sem status")
		statusCount[status]++
		isFinished := finishedWorkOrderStatuses[strings.ToLower(strings.TrimSpace(status))] || strings.TrimSpace(pickStr(m, "data_termino_executado")) != ""
		tech := firstNonEmpty(extractTechnicianName(m), "Sem técnico")
		techTotal[tech]++
		if isFinished {
			finishedTotal++
			techFinished[tech]++
		}
	}
	byTech := make([]TechnicianStat, 0, len(techTotal))
	for name, cnt := range techTotal {
		fin := techFinished[name]
		pct := 0.0
		if finishedTotal > 0 {
			pct = round2(float64(fin) / float64(finishedTotal) * 100)
		}
		byTech = append(byTech, TechnicianStat{Technician: name, Total: cnt, Finished: fin, PctOfFinished: pct})
	}
	sort.Slice(byTech, func(i, j int) bool {
		if byTech[i].Finished != byTech[j].Finished {
			return byTech[i].Finished > byTech[j].Finished
		}
		return byTech[i].Technician < byTech[j].Technician
	})
	n := len(items)
	pct := 0.0
	if n > 0 {
		pct = round2(float64(finishedTotal) / float64(n) * 100)
	}
	return WorkOrderPeriodReport{
		OK: true, From: from, To: to, Total: total, Finished: finishedTotal, FinishedPct: pct,
		ByStatus: topNamedCounts(statusCount, nil, 20), ByTechnician: byTech, Truncated: total > n,
	}
}

// --- Relatório: financeiro por período (aba Relatório → Financeiro) ----------------------------

type FinancialPeriodReport struct {
	OK           bool    `json:"ok"`
	Message      string  `json:"message,omitempty"`
	From         string  `json:"from"`
	To           string  `json:"to"`
	Total        int     `json:"total"`
	TotalValue   float64 `json:"total_value"`
	PaidCount    int     `json:"paid_count"`
	PaidValue    float64 `json:"paid_value"`
	PaidPct      float64 `json:"paid_pct"` // % do VALOR pago sobre o valor total do período
	OpenCount    int     `json:"open_count"`
	OpenValue    float64 `json:"open_value"`
	OpenPct      float64 `json:"open_pct"`
	OverdueCount int     `json:"overdue_count"`
	OverdueValue float64 `json:"overdue_value"`
	OverduePct   float64 `json:"overdue_pct"`
	Truncated    bool    `json:"truncated,omitempty"`
}

// BuildFinancialPeriodReport varre TODAS as faturas com vencimento no período
// (/financeiro/fatura pagina de verdade, ao contrário de /cliente/financeiro que precisa de um
// cliente por vez) e calcula o percentual de valor recebido vs em aberto vs vencido.
func BuildFinancialPeriodReport(ctx context.Context, cfg Config, token, from, to string) FinancialPeriodReport {
	from, to = defaultPeriod(from, to)
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/financeiro/fatura",
		map[string]string{"tipo_data": "data_vencimento", "data_inicio": from, "data_fim": to, "tipo_resultado": "simplificado"},
		maxReportPages, "faturas")
	if err != nil {
		return FinancialPeriodReport{Message: "Falha ao coletar faturas: " + err.Error(), From: from, To: to}
	}
	today := time.Now()
	var totalValue, paidValue, openValue, overdueValue float64
	var paidCount, openCount, overdueCount int
	for _, m := range items {
		val := parseBRFloat(pickStr(m, "valor"))
		totalValue += val
		if strings.TrimSpace(pickStr(m, "data_pagamento")) != "" {
			paidCount++
			paidVal := parseBRFloat(pickStr(m, "valor_pago"))
			if paidVal <= 0 {
				paidVal = val
			}
			paidValue += paidVal
			continue
		}
		due := parseBRDate(pickStr(m, "data_vencimento"))
		if !due.IsZero() && due.Before(today) {
			overdueCount++
			overdueValue += val
		} else {
			openCount++
			openValue += val
		}
	}
	pct := func(v float64) float64 {
		if totalValue <= 0 {
			return 0
		}
		return round2(v / totalValue * 100)
	}
	return FinancialPeriodReport{
		OK: true, From: from, To: to, Total: total, TotalValue: round2(totalValue),
		PaidCount: paidCount, PaidValue: round2(paidValue), PaidPct: pct(paidValue),
		OpenCount: openCount, OpenValue: round2(openValue), OpenPct: pct(openValue),
		OverdueCount: overdueCount, OverdueValue: round2(overdueValue), OverduePct: pct(overdueValue),
		Truncated: total > len(items),
	}
}

// --- Relatório: financeiro — lista paginada (aba Financeiro da integração) ---------------------
//
// Ao contrário de BuildFinancialPeriodReport (acima — varre TODAS as páginas para somar
// percentuais), esta função busca UMA página de cada vez, passando a paginação da HubSoft
// directamente para o frontend — a tela "Financeiro" pagina de verdade (1 pedido HTTP por
// página pedida pelo usuário), continua rápida mesmo com milhares de faturas no período.

type InvoiceRow struct {
	ID            string `json:"id,omitempty"`
	ClientName    string `json:"client_name,omitempty"`
	ClientCode    string `json:"client_code,omitempty"`
	Value         string `json:"value,omitempty"`
	ValuePaid     string `json:"value_paid,omitempty"`
	DueDate       string `json:"due_date,omitempty"`
	PaymentDate   string `json:"payment_date,omitempty"`
	Status        string `json:"status,omitempty"` // "paid" | "overdue" | "pending" — derivado, ver deriveInvoiceStatus
	DigitableLine string `json:"digitable_line,omitempty"`
	BarCode       string `json:"bar_code,omitempty"`
	Link          string `json:"link,omitempty"`
}

func deriveInvoiceStatus(m map[string]any) string {
	if strings.TrimSpace(pickStr(m, "data_pagamento")) != "" {
		return "paid"
	}
	due := parseBRDate(pickStr(m, "data_vencimento"))
	if !due.IsZero() && due.Before(time.Now()) {
		return "overdue"
	}
	return "pending"
}

type InvoiceListResult struct {
	OK             bool         `json:"ok"`
	Message        string       `json:"message,omitempty"`
	Invoices       []InvoiceRow `json:"invoices"`
	Page           int          `json:"page"`
	PerPage        int          `json:"per_page"`
	TotalPages     int          `json:"total_pages"`
	TotalRegistros int          `json:"total_registros"`
}

// InvoiceListFilter filtros aceites por ListInvoices — todos opcionais.
type InvoiceListFilter struct {
	From           string // data_inicio (vencimento)
	To             string // data_fim (vencimento)
	ApenasEmAberto string
	ApenasQuitado  string
	Busca          string // termo de busca (nome/código/documento do cliente)
}

// ListInvoices busca UMA página de faturas (/financeiro/fatura) — página/tamanho vindos do
// chamador, sem varrer o período inteiro.
func ListInvoices(ctx context.Context, cfg Config, token string, filter InvoiceListFilter, page, perPage int) (InvoiceListResult, error) {
	if perPage <= 0 || perPage > 100 {
		perPage = 30
	}
	if page < 0 {
		page = 0
	}
	q := map[string]string{
		"pagina": strconv.Itoa(page), "itens_por_pagina": strconv.Itoa(perPage),
		"tipo_data": "data_vencimento", "tipo_resultado": "simplificado",
	}
	if v := strings.TrimSpace(filter.From); v != "" {
		q["data_inicio"] = v
	}
	if v := strings.TrimSpace(filter.To); v != "" {
		q["data_fim"] = v
	}
	if v := strings.TrimSpace(filter.ApenasEmAberto); v != "" {
		q["apenas_em_aberto"] = v
	}
	if v := strings.TrimSpace(filter.ApenasQuitado); v != "" {
		q["apenas_quitado"] = v
	}
	if v := strings.TrimSpace(filter.Busca); v != "" {
		q["busca"] = v
		q["termo_busca"] = v
	}
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/financeiro/fatura", QueryParams: paramKVs(q),
	})
	if !res.OK {
		return InvoiceListResult{}, fmt.Errorf("hubsoft: %s", firstNonEmpty(res.ErrorMessage, "falha ao consultar faturas"))
	}
	body := ResponseBodyBytes(res)
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return InvoiceListResult{}, fmt.Errorf("resposta inválida da HubSoft")
	}
	out := InvoiceListResult{OK: true, Page: page, PerPage: perPage, Invoices: []InvoiceRow{}}
	if pg, ok := doc["paginacao"].(map[string]any); ok {
		if last, err := strconv.Atoi(scalarToString(pg["ultima_pagina"])); err == nil {
			out.TotalPages = last + 1
		}
		if tr, ok := pg["total_registros"].(float64); ok {
			out.TotalRegistros = int(tr)
		}
	}
	for _, it := range extractArray(doc, "faturas") {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		row := InvoiceRow{
			ID: pickStr(m, "id_fatura"), Value: pickStr(m, "valor"), ValuePaid: pickStr(m, "valor_pago"),
			DueDate: pickStr(m, "data_vencimento"), PaymentDate: pickStr(m, "data_pagamento"),
			DigitableLine: pickStr(m, "linha_digitavel"), BarCode: pickStr(m, "codigo_barras"),
			Link: pickStr(m, "link"), Status: deriveInvoiceStatus(m),
		}
		if cli, ok := m["cliente"].(map[string]any); ok {
			row.ClientName = pickStr(cli, "nome_razaosocial")
			row.ClientCode = pickStr(cli, "codigo_cliente")
		}
		out.Invoices = append(out.Invoices, row)
	}
	if len(out.Invoices) == 0 {
		out.Message = "Nenhuma fatura encontrada para o período/filtro."
	}
	return out, nil
}

// --- Versões rápidas do Dashboard (Atendimentos/O.S. recentes e Resumo financeiro) -------------
//
// BuildRecentActivity/BuildFinancialSummary (acima) varrem uma AMOSTRA de clientes e fazem 1-2
// pedidos extra POR CLIENTE em paralelo — lento (dezenas/centenas de pedidos) mesmo com
// concorrência, e "amostra" porque só cobre os clientes que a varredura por letras encontrou.
// Estas versões usam os endpoints "todos"/"listar" (paginação real, ver fetchAllPages) — 1
// pedido paginado cobre TODOS os atendimentos/O.S./faturas do período de uma vez, mais rápido e
// completo (não é amostra). Substituem BuildRecentActivity/BuildFinancialSummary nos handlers
// (handlers_hubsoft.go) — mantidas as antigas só por não termos motivo para as apagar.

const fastRecentMaxPages = 4 // 4×500 = até 2000 registos recentes — de sobra para "os N mais recentes"

// enrichDescriptionConcurrency limita pedidos simultâneos de FetchAttendanceDetail — só é
// chamado para os `limitEach` (20) atendimentos finais da lista, não para os até 2000 varridos.
const enrichDescriptionConcurrency = 6

// enrichAttendanceDescriptions preenche AttendanceItem.Description com a descrição de abertura
// real (descricao_abertura) — /atendimento/todos não tem esse campo (confirmado: nem a relação
// atendimento_mensagem o traz, só notas avulsas sem relação com a abertura). Só é chamado com a
// lista JÁ truncada aos itens finais mostrados na tela — mantém o pedido principal rápido e
// paginado, e adiciona só ~20 pedidos leves e paralelos (1 por atendimento, via
// /cliente/atendimento?busca=protocolo, o mesmo usado pelo "ver mais").
func enrichAttendanceDescriptions(ctx context.Context, cfg Config, token string, items []AttendanceItem) {
	sem := make(chan struct{}, enrichDescriptionConcurrency)
	var wg sync.WaitGroup
	for i := range items {
		protocolo := items[i].Protocol
		if protocolo == "" {
			continue
		}
		wg.Add(1)
		go func(i int, protocolo string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			detail, err := FetchAttendanceDetail(ctx, cfg, token, protocolo)
			if err != nil || !detail.OK {
				return
			}
			items[i].Description = detail.Description
		}(i, protocolo)
	}
	wg.Wait()
}

// BuildRecentActivityFast últimos 30 dias de atendimentos + O.S., devolve os `limitEach` mais
// recentes de cada mais as repartições por status — mesma forma de resposta de
// BuildRecentActivity (RecentActivityResult), só a fonte dos dados muda.
func BuildRecentActivityFast(ctx context.Context, cfg Config, token string, limitEach int) RecentActivityResult {
	from, to := defaultPeriod("", "")

	// relacoes=cliente_servico traz tipo_atendimento (assunto) e cliente_servico.cliente (nome do
	// cliente) — leve. /atendimento/todos NÃO tem descricao_abertura nem a conversa (confirmado:
	// nem com relacoes=atendimento_mensagem — essa relação traz notas avulsas, não a descrição de
	// abertura real); por isso a "Descrição" da lista é preenchida depois, só para os
	// `limitEach` registos finais, com 1 pedido leve por atendimento (ver enrichAttendanceDescriptions).
	attItems, attTotal, attErr := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/atendimento/todos",
		map[string]string{"data_inicio": from, "data_fim": to, "relacoes": "cliente_servico"}, fastRecentMaxPages, "atendimentos")
	woItems, woTotal, woErr := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/ordem_servico/todos",
		map[string]string{"data_inicio": from, "data_fim": to}, fastRecentMaxPages, "ordens_servico", "ordem_servico", "ordens")
	if attErr != nil && woErr != nil {
		return RecentActivityResult{Message: "Falha ao coletar atendimentos/O.S.: " + attErr.Error()}
	}

	attStatusCount := map[string]int{}
	attendance := make([]AttendanceItem, 0, len(attItems))
	for _, m := range attItems {
		label := "Sem status"
		if st, ok := m["status"].(map[string]any); ok {
			label = firstNonEmpty(pickStr(st, "descricao"), label)
		} else {
			label = firstNonEmpty(pickStr(m, "status"), label)
		}
		attStatusCount[label]++
		item := AttendanceItem{
			ID: pickStr(m, "id_atendimento"), Protocol: pickStr(m, "protocolo"),
			Status: label, OpenedAt: pickStr(m, "data_cadastro"), ClosedAt: pickStr(m, "data_fechamento"),
		}
		if tipo, ok := m["tipo_atendimento"].(map[string]any); ok {
			item.Subject = pickStr(tipo, "descricao")
		}
		if cs, ok := m["cliente_servico"].(map[string]any); ok {
			if cli, ok := cs["cliente"].(map[string]any); ok {
				item.ClientName = pickStr(cli, "nome_razaosocial")
				item.ClientCode = pickStr(cli, "codigo_cliente")
			}
		}
		attendance = append(attendance, item)
	}

	woStatusCount := map[string]int{}
	workOrders := make([]WorkOrderItem, 0, len(woItems))
	for _, m := range woItems {
		status := firstNonEmpty(pickStr(m, "status"), "Sem status")
		woStatusCount[status]++
		workOrders = append(workOrders, WorkOrderItem{
			ID:          pickStr(m, "id_ordem_servico"),
			Number:      firstNonEmpty(pickStr(m, "numero"), pickStr(m, "id_ordem_servico")),
			Status:      status,
			Type:        pickStr(m, "tipo"),
			Description: firstNonEmpty(pickStr(m, "descricao_abertura"), pickStr(m, "descricao_servico")),
			// "servico" vem como texto "(numero_plano) NOME DO PLANO" — confirmado na doc/amostra
			// da API, DIFERENTE de descricao_servico (texto livre, não é o nome do plano).
			PlanName:    pickStr(m, "servico"),
			ScheduledAt: pickStr(m, "data_inicio_programado"),
			CreatedAt:   pickStr(m, "data_cadastro"),
			ClientName:  pickStr(m, "cliente"), // já vem como texto "(código) NOME" neste endpoint
		})
	}

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
	enrichAttendanceDescriptions(ctx, cfg, token, attendance)

	return RecentActivityResult{
		OK: true, SampleClients: 0, // já não é amostra por cliente — cobre todos os registos do período (últimos 30 dias)
		Attendance: attendance, WorkOrders: workOrders,
		TotalAttendanceFound: attTotal, TotalWorkOrdersFound: woTotal,
		AttendanceStatusBreakdown: topNamedCounts(attStatusCount, nil, 8),
		WorkOrderStatusBreakdown:  topNamedCounts(woStatusCount, nil, 8),
	}
}

// financialSummaryLookbackDays janela usada pelo "Resumo financeiro" (não é um relatório por
// período escolhido pelo utilizador — é "estado actual" das faturas) — cobre vencimentos dos
// últimos 6 meses, o suficiente para qualquer fatura vencida/pendente realista sem varrer o
// histórico inteiro da base.
const financialSummaryLookbackDays = 180

// BuildFinancialSummaryFast agrupa as faturas do período (por cliente, via o campo `cliente`
// que cada fatura já traz) para montar os maiores devedores — mesma forma de resposta de
// BuildFinancialSummary (FinancialSummaryResult), sem precisar de 1 pedido por cliente.
func BuildFinancialSummaryFast(ctx context.Context, cfg Config, token string) FinancialSummaryResult {
	now := time.Now()
	from := now.AddDate(0, 0, -financialSummaryLookbackDays).Format("2006-01-02")
	to := now.Format("2006-01-02")
	items, _, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/financeiro/fatura",
		map[string]string{"tipo_data": "data_vencimento", "data_inicio": from, "data_fim": to, "tipo_resultado": "simplificado"},
		maxReportPages, "faturas")
	if err != nil {
		return FinancialSummaryResult{OK: false, Message: "Falha ao coletar faturas: " + err.Error()}
	}

	type acc struct {
		name         string
		pendingValue float64
		overdueValue float64
		invoiceCount int
	}
	debtByClient := map[string]*acc{}
	var totalOverdue, totalPending, totalPaid float64
	for _, m := range items {
		val := parseBRFloat(pickStr(m, "valor"))
		if strings.TrimSpace(pickStr(m, "data_pagamento")) != "" {
			paidVal := parseBRFloat(pickStr(m, "valor_pago"))
			if paidVal <= 0 {
				paidVal = val
			}
			totalPaid += paidVal
			continue
		}
		cli, _ := m["cliente"].(map[string]any)
		code := ""
		name := ""
		if cli != nil {
			code = pickStr(cli, "codigo_cliente")
			name = pickStr(cli, "nome_razaosocial")
		}
		due := parseBRDate(pickStr(m, "data_vencimento"))
		a := debtByClient[code]
		if a == nil {
			a = &acc{name: name}
			debtByClient[code] = a
		}
		a.invoiceCount++
		if !due.IsZero() && due.Before(now) {
			totalOverdue += val
			a.overdueValue += val
		} else {
			totalPending += val
			a.pendingValue += val
		}
	}
	debtors := make([]ClientDebt, 0, len(debtByClient))
	for code, a := range debtByClient {
		if a.pendingValue+a.overdueValue <= 0 {
			continue
		}
		debtors = append(debtors, ClientDebt{
			ClientName: a.name, ClientCode: code,
			PendingValue: round2(a.pendingValue), OverdueValue: round2(a.overdueValue), InvoiceCount: a.invoiceCount,
		})
	}
	sort.Slice(debtors, func(i, j int) bool {
		return (debtors[i].PendingValue + debtors[i].OverdueValue) > (debtors[j].PendingValue + debtors[j].OverdueValue)
	})
	if len(debtors) > financialSummaryTopDebtors {
		debtors = debtors[:financialSummaryTopDebtors]
	}

	return FinancialSummaryResult{
		OK: true, SampleClients: 0, ClientsWithDebt: len(debtByClient),
		TotalInvoices: len(items), TotalReceivable: round2(totalOverdue + totalPending),
		TotalOverdue: round2(totalOverdue), TotalPending: round2(totalPending), TotalPaid: round2(totalPaid),
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
		Document:  formatCPFCNPJ(pickStr(m, "cpf_cnpj", "cnpj_cpf")),
		Email:     pickStr(m, "email_principal", "email"),
		Phone:     formatPhoneBR(pickStr(m, "telefone_primario", "telefone")),
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
	ID     string `json:"id,omitempty"`
	Number string `json:"number,omitempty"`
	Status string `json:"status,omitempty"`
	// Type é o tipo da O.S. (campo "tipo" — ex.: "ATIVAÇÃO FIBRA", "RETIRADA DE EQUIPAMENTO"),
	// mostrado na coluna "Tipo de O.S." (substitui a antiga coluna "Atendimento").
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	// PlanName é o plano/serviço contratado (campo "servico" da HubSoft — ex.: "(10) 200MB
	// FIBRA"), DIFERENTE de Description (texto livre de abertura/observação da O.S., campo
	// "descricao_abertura"/"descricao_servico"). Confirmado ao vivo que a coluna "Plano /
	// serviço" da tela estava a mostrar Description por falta deste campo.
	PlanName    string `json:"plan_name,omitempty"`
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

// digitsOnly mantém só os dígitos de uma string — usado para formatar CPF/CNPJ e telefone.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatCPFCNPJ aplica a máscara brasileira padrão: CPF 000.000.000-00 (11 dígitos) ou
// CNPJ 00.000.000/0000-00 (14 dígitos). Fora disso (já formatado de outro jeito, incompleto
// etc.) devolve o valor original sem mexer.
func formatCPFCNPJ(raw string) string {
	d := digitsOnly(raw)
	switch len(d) {
	case 11:
		return d[0:3] + "." + d[3:6] + "." + d[6:9] + "-" + d[9:11]
	case 14:
		return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
	default:
		return raw
	}
}

// formatPhoneBR aplica "(DD) NNNN-NNNN" (fixo, 10 dígitos) ou "(DD) NNNNN-NNNN" (celular,
// 11 dígitos). Fora disso devolve o valor original.
func formatPhoneBR(raw string) string {
	d := digitsOnly(raw)
	switch len(d) {
	case 10:
		return "(" + d[0:2] + ") " + d[2:6] + "-" + d[6:10]
	case 11:
		return "(" + d[0:2] + ") " + d[2:7] + "-" + d[7:11]
	default:
		return raw
	}
}

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
			body = []byte(s)
		}
	}
	return fixLatin1(body)
}

// fixLatin1 corrige respostas da HubSoft que, para alguns endpoints (confirmado ao vivo em
// /cliente/todos), chegam em Latin-1/ISO-8859-1 em vez de UTF-8 — caracteres acentuados ficam
// corrompidos ("Serviço" vira "Servi�o"). Só entra em ação quando os bytes não são UTF-8
// válido, então é inofensivo para os demais endpoints (já corretos hoje).
func fixLatin1(body []byte) []byte {
	if utf8.Valid(body) {
		return body
	}
	runes := make([]rune, len(body))
	for i, b := range body {
		runes[i] = rune(b)
	}
	return []byte(string(runes))
}

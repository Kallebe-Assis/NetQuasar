// Conferência de ordens de serviço por período: cruza as O.S. do período com o cadastro de clientes/serviços
// (login, plano, status, vendedor) para o operador conferir o que foi fechado e por quem.

package integrationhubsoft

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Conferência de O.S. por período (aba Ordens de serviço → botão "Conferência") -------------
//
// Cruza as O.S. de um período com o serviço do cliente correspondente (login, IPv4, estado
// "conectado" — já vem directo da HubSoft, mesmo campo que alimenta o badge Conectado/
// Desconectado da aba Relatório → Clientes) para dar o que a O.S. sozinha não tem. O resto das
// conferências pedidas (IPv6, acesso remoto HTTP/HTTPS) não é dado da HubSoft — fica a cargo do
// handler (internal/api/handlers_hubsoft_conference.go), que tem acesso a bng_known_logins e ao
// probe de rede; este pacote só fala com a API da HubSoft.

var clienteCodigoRe = regexp.MustCompile(`^\((\d+)\)`)

// extractClientCodeFromLabel extrai o código de "(123) FULANO DE TAL" — formato em que o campo
// "cliente" vem no endpoint /ordem_servico/todos (confirmado no comentário de enrichWorkOrders
// acima, "já vem como texto (código) NOME neste endpoint").
func extractClientCodeFromLabel(s string) string {
	m := clienteCodigoRe.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ConferenceOSItem uma O.S. do período, cruzada com o serviço do cliente quando resolvida.
type ConferenceOSItem struct {
	ID          string `json:"id,omitempty"`
	Number      string `json:"number,omitempty"`
	Status      string `json:"status,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	ScheduledAt string `json:"scheduled_at,omitempty"`
	ClientCode  string `json:"client_code,omitempty"`
	ClientName  string `json:"client_name,omitempty"`
	Login       string `json:"login,omitempty"`
	IPv4        string `json:"ipv4,omitempty"`
	// IPv6 prefixo estático do serviço (servicos[].ipv6, irmão de ipv4 — não vem de
	// ultima_conexao). Vazio = cliente sem IPv6 atribuído.
	IPv6 string `json:"ipv6,omitempty"`
	// Connected vem directo da HubSoft ("true"/"false"/"" sem dado) — mesmo campo do relatório
	// de Clientes, não é cruzado com dados nossos.
	Connected string `json:"connected,omitempty"`
	// Resolved indica se achou o serviço do cliente (login/IPv4/connected preenchidos) — uma O.S.
	// pode não resolver se o cliente foi removido/o campo "cliente" não trouxe código válido.
	Resolved bool `json:"resolved"`
	// Dados do fechamento — vêm no próprio /ordem_servico/todos (usuario_fechamento, data_termino_executado,
	// descricao_fechamento, motivo_fechamento, tecnicos), sem chamada extra.
	StartedAt          string   `json:"started_at,omitempty"`          // data_inicio_executado
	ClosedAt           string   `json:"closed_at,omitempty"`           // data_termino_executado
	ClosedBy           string   `json:"closed_by,omitempty"`           // usuario_fechamento.name
	ClosingDescription string   `json:"closing_description,omitempty"` // descricao_fechamento
	ClosingReasons     []string `json:"closing_reasons,omitempty"`     // motivo_fechamento[].descricao
	Technicians        []string `json:"technicians,omitempty"`         // tecnicos[].name
	Protocol           string   `json:"protocol,omitempty"`            // atendimento.protocolo
	ServiceName        string   `json:"service_name,omitempty"`        // plano do serviço da O.S.
}

// ConferenceOptions parâmetros da conferência de O.S. OnlyFinished pede à HubSoft só as finalizadas
// (status=finalizado) e usa a data de término executado como base do período — o filtro roda no servidor da
// HubSoft, então traz menos páginas. Progress recebe o avanço geral (0–100) e o nome da etapa.
type ConferenceOptions struct {
	From, To     string
	OnlyFinished bool
	Progress     func(pct int, label string)
}

type WorkOrderConferenceData struct {
	OK        bool               `json:"ok"`
	Message   string             `json:"message,omitempty"`
	From      string             `json:"from"`
	To        string             `json:"to"`
	Items     []ConferenceOSItem `json:"items"`
	Total     int                `json:"total"`
	Resolved  int                `json:"resolved"`
	Truncated bool               `json:"truncated,omitempty"`
}

// BuildWorkOrderConferenceData faz duas varreduras paginadas (O.S. do período + roster completo
// de clientes/serviços) e junta-as em memória — evita N chamadas extra (uma por O.S.) para
// resolver login/IPv4/IPv6/estado de conexão.
//
// Uma primeira tentativa desta função ligava a O.S. a "qualquer serviço do cliente com login
// preenchido" — errado quando o cliente tem mais de um serviço/login (relatado ao vivo: cliente
// MARLETE GOMES DOMICIANO, O.S. do login "marlete", sistema a conferir "marletegomes" por
// engano). Uma segunda tentativa foi buscar o atendimento vinculado à O.S., mas
// /atendimento/todos?relacoes=cliente_servico voltou 0 resultados úteis em produção (confirmado
// ao vivo via log) — provavelmente essa relação não é suportada nesse endpoint de listagem em
// massa. A solução real, também confirmada ao vivo: exibir_atendimento=true no /ordem_servico/
// todos já embute directamente "dados_servico":{"id_cliente_servico":...} e
// "dados_cliente":{"codigo_cliente":...,"nome_razaosocial":...} em CADA O.S. — o ID exacto do
// serviço envolvido naquela O.S. especificamente, sem precisar de nenhuma chamada extra nem de
// adivinhar por cliente. byServiceID cruza isto directo com ReportServiceRow.ServiceID (o mesmo
// id_cliente_servico já extraído em reportFromClientsTodos). Só cai para o roster completo do
// cliente (heurística antiga, "primeiro com login") nos poucos casos em que dados_servico não
// vem preenchido.
func BuildWorkOrderConferenceData(ctx context.Context, cfg Config, token, from, to string) WorkOrderConferenceData {
	return BuildWorkOrderConference(ctx, cfg, token, ConferenceOptions{From: from, To: to})
}

// conferenceMaxDirectClients — até este nº de clientes distintos, os dados de conexão vêm de uma consulta por
// cliente (poucas chamadas); acima disso, da varredura completa da base (mais barata para muitos clientes).
const conferenceMaxDirectClients = 150

func BuildWorkOrderConference(ctx context.Context, cfg Config, token string, opt ConferenceOptions) WorkOrderConferenceData {
	from, to := defaultPeriod(opt.From, opt.To)
	progress := func(pct int, label string) {
		if opt.Progress != nil {
			opt.Progress(pct, label)
		}
	}
	// scale converte o avanço (páginas) de uma etapa numa faixa [lo, hi] do progresso geral.
	scale := func(lo, hi int, label string) context.Context {
		return WithPageProgress(ctx, func(done, total int) {
			if total <= 0 {
				total = 1
			}
			progress(lo+(hi-lo)*done/total, label)
		})
	}
	q := map[string]string{"data_inicio": from, "data_fim": to, "exibir_atendimento": "true", "relacoes": "tecnicos"}
	if opt.OnlyFinished {
		q["status"] = "finalizado"
		q["tipo_data"] = "data_termino_executado"
	}
	progress(0, "Coletando ordens de serviço")
	osItems, osTotal, err := fetchAllPages(scale(0, 25, "Coletando ordens de serviço"), cfg, token, "/api/v1/integracao/ordem_servico/todos", q, maxReportPages, "ordens_servico", "ordem_servico", "ordens")
	if err != nil {
		return WorkOrderConferenceData{Message: "Falha ao coletar ordens de serviço: " + err.Error(), From: from, To: to}
	}

	// Clientes distintos das O.S. (código) — decide como buscar os dados de conexão.
	codes := map[string]struct{}{}
	for _, m := range osItems {
		if c := osClientCode(m); c != "" {
			codes[c] = struct{}{}
		}
	}
	var svcRows []ReportServiceRow
	if len(codes) > 0 && len(codes) <= conferenceMaxDirectClients {
		progress(25, "Consultando os clientes das O.S.")
		svcRows = fetchServiceRowsByClient(ctx, cfg, token, codes, func(done, total int) {
			progress(25+65*done/maxInt(total, 1), "Consultando os clientes das O.S.")
		})
	} else {
		svcResult, svcErr := ListClientServiceReport(scale(25, 90, "Lendo a base de clientes e serviços"), cfg, token, ReportListFilter{})
		if svcErr != nil {
			return WorkOrderConferenceData{Message: "Falha ao coletar clientes/serviços: " + svcErr.Error(), From: from, To: to}
		}
		svcRows = svcResult.Rows
	}
	progress(90, "Cruzando O.S. com os serviços")
	byServiceID := make(map[string]ReportServiceRow, len(svcRows))
	byClient := make(map[string][]ReportServiceRow, len(svcRows))
	for _, row := range svcRows {
		if row.ServiceID != "" {
			byServiceID[row.ServiceID] = row
		}
		if row.ClientCode != "" {
			byClient[row.ClientCode] = append(byClient[row.ClientCode], row)
		}
	}

	items := make([]ConferenceOSItem, 0, len(osItems))
	resolved := 0
	for _, m := range osItems {
		clienteLabel := pickStr(m, "cliente")
		code := extractClientCodeFromLabel(clienteLabel)
		clientName := strings.TrimSpace(clienteCodigoRe.ReplaceAllString(clienteLabel, ""))
		serviceID := ""
		if dc, ok := m["dados_cliente"].(map[string]any); ok {
			if v := pickStr(dc, "codigo_cliente"); v != "" {
				code = v
			}
			if v := pickStr(dc, "nome_razaosocial"); v != "" {
				clientName = v
			}
		}
		serviceName := ""
		if ds, ok := m["dados_servico"].(map[string]any); ok {
			serviceID = pickStr(ds, "id_cliente_servico")
			serviceName = pickStr(ds, "descricao")
		}
		osType := pickStr(m, "tipo")
		if t, ok := m["tipo_ordem_servico"].(map[string]any); ok {
			osType = firstNonEmpty(pickStr(t, "descricao"), osType)
		}
		protocol := ""
		if at, ok := m["atendimento"].(map[string]any); ok {
			protocol = pickStr(at, "protocolo")
		}
		it := ConferenceOSItem{
			ID:                 pickStr(m, "id_ordem_servico"),
			Number:             firstNonEmpty(pickStr(m, "numero"), pickStr(m, "id_ordem_servico")),
			Status:             firstNonEmpty(pickStr(m, "status"), "Sem status"),
			Type:               osType,
			Description:        firstNonEmpty(pickStr(m, "descricao_abertura"), pickStr(m, "descricao_servico")),
			CreatedAt:          pickStr(m, "data_cadastro"),
			ScheduledAt:        pickStr(m, "data_inicio_programado"),
			ClientCode:         code,
			ClientName:         clientName,
			StartedAt:          pickStr(m, "data_inicio_executado"),
			ClosedAt:           pickStr(m, "data_termino_executado"),
			ClosedBy:           osUserName(m["usuario_fechamento"]),
			ClosingDescription: strings.TrimSpace(pickStr(m, "descricao_fechamento")),
			ClosingReasons:     osNames(m["motivo_fechamento"], "descricao"),
			Technicians:        osNames(m["tecnicos"], "name", "display"),
			Protocol:           protocol,
			ServiceName:        serviceName,
		}

		var svc *ReportServiceRow
		if serviceID != "" {
			if row, ok := byServiceID[serviceID]; ok {
				svc = &row
			}
		}
		if svc == nil {
			// Rede de segurança: dados_servico não veio preenchido nesta O.S. — mantém o
			// comportamento anterior em vez de deixar a O.S. sem dado nenhum. Um cliente pode ter
			// mais de um serviço; sem saber qual é o certo, prioriza o primeiro com login
			// preenchido (caso comum: um só serviço).
			if rows, ok := byClient[code]; code != "" && ok && len(rows) > 0 {
				best := rows[0]
				for _, r := range rows {
					if strings.TrimSpace(r.Login) != "" {
						best = r
						break
					}
				}
				svc = &best
			}
		}
		if svc != nil {
			it.Login = svc.Login
			it.IPv4 = svc.IPv4
			it.IPv6 = svc.IPv6
			it.Connected = svc.Connected
			it.Resolved = true
			resolved++
		}
		items = append(items, it)
	}
	progress(95, "Montando o resultado")

	return WorkOrderConferenceData{
		OK: true, From: from, To: to, Items: items, Total: osTotal, Resolved: resolved,
		Truncated: osTotal > len(osItems),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// osClientCode extrai o código do cliente de uma O.S. de /ordem_servico/todos.
func osClientCode(m map[string]any) string {
	if dc, ok := m["dados_cliente"].(map[string]any); ok {
		if v := pickStr(dc, "codigo_cliente"); v != "" {
			return v
		}
	}
	return extractClientCodeFromLabel(pickStr(m, "cliente"))
}

// osUserName lê "usuario_fechamento": objeto {id,name} quando a O.S. foi fechada, lista vazia enquanto não.
func osUserName(v any) string {
	if m, ok := v.(map[string]any); ok {
		return strings.TrimSpace(firstNonEmpty(pickStr(m, "name"), pickStr(m, "display"), pickStr(m, "nome")))
	}
	return ""
}

// osNames junta, em ordem e sem repetir, o primeiro campo preenchido de cada item de uma lista de objetos.
func osNames(v any, keys ...string) []string {
	arr, _ := v.([]any)
	var out []string
	seen := map[string]bool{}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range keys {
			if n := strings.TrimSpace(pickStr(m, k)); n != "" {
				if !seen[n] {
					seen[n] = true
					out = append(out, n)
				}
				break
			}
		}
	}
	return out
}

// fetchServiceRowsByClient consulta cada cliente (por código) e devolve as linhas de serviço — usado quando a
// conferência envolve poucos clientes, em vez de ler a base inteira. Falhas pontuais deixam o cliente sem dados
// (a O.S. aparece como não resolvida), sem derrubar a conferência.
func fetchServiceRowsByClient(ctx context.Context, cfg Config, token string, codes map[string]struct{}, onProgress func(done, total int)) []ReportServiceRow {
	list := make([]string, 0, len(codes))
	for c := range codes {
		list = append(list, c)
	}
	sort.Strings(list)
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		rows  []ReportServiceRow
		done  int
		total = len(list)
	)
	sem := make(chan struct{}, 6)
	for _, code := range list {
		wg.Add(1)
		go func(code string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
				Method: "GET", Path: "/api/v1/integracao/cliente",
				QueryParams: paramKVs(map[string]string{
					"busca": "codigo_cliente", "termo_busca": code, "limit": "5", "cancelado": "nao", "inativo": "todos",
					"ultima_conexao": "sim", "relacoes": "endereco_instalacao",
				}),
			})
			var got []ReportServiceRow
			if res.OK {
				var doc map[string]any
				if json.Unmarshal(ResponseBodyBytes(res), &doc) == nil {
					for _, it := range extractArray(doc, "clientes") {
						if m, ok := it.(map[string]any); ok && pickStr(m, "codigo_cliente") == code {
							got = append(got, clientServiceRows(m)...)
						}
					}
				}
			}
			mu.Lock()
			rows = append(rows, got...)
			done++
			d := done
			mu.Unlock()
			if onProgress != nil {
				onProgress(d, total)
			}
		}(code)
	}
	wg.Wait()
	return rows
}

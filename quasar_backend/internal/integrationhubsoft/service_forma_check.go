package integrationhubsoft

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

// --- Conferência da forma de cobrança ATUAL dos serviços (aba Relatório → Boletos por forma) ----------------------
//
// Complementa o relatório de boletos: a fatura guarda a forma de cobrança de quando foi gerada; aqui se confere a
// forma que o SERVIÇO tem agora (ex.: «Banco do Brasil (G2)» depois da troca em massa). Para cada id_cliente da lista
// consulta GET /cliente (somente leitura) e lê a forma de cobrança de cada serviço do cliente. Quando a consulta
// não devolve esse dado, o serviço sai como "sem_dado" — nunca como certo ou errado — e o resultado traz a lista de
// campos do serviço (CamposServico) para diagnóstico.

// ServiceFormaStatus é o veredito de um serviço.
const (
	ServiceFormaOK        = "ok"        // a forma do serviço é a esperada
	ServiceFormaDiff      = "diferente" // o serviço tem outra forma de cobrança
	ServiceFormaNoData    = "sem_dado"  // a HubSoft não devolveu a forma de cobrança deste serviço
	serviceFormaMaxClient = 500
)

type ServiceFormaRow struct {
	IDCliente        string `json:"id_cliente"`
	Cliente          string `json:"cliente,omitempty"`
	IDClienteServico string `json:"id_cliente_servico"`
	Login            string `json:"login,omitempty"`
	Plano            string `json:"plano,omitempty"`
	Status           string `json:"status,omitempty"`
	FormaID          string `json:"forma_id,omitempty"`
	FormaNome        string `json:"forma_nome,omitempty"`
	Resultado        string `json:"resultado"`
}

type ServiceFormaReport struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	// Esperada é a forma de cobrança pedida (id ou nome).
	Esperada string `json:"esperada"`
	Clientes int    `json:"clientes"`
	Servicos int    `json:"servicos"`
	// Contagens por veredito (de serviços não cancelados) e de clientes.
	ServicosOK           int               `json:"servicos_ok"`
	ServicosDiff         int               `json:"servicos_diferentes"`
	ServicosSemDado      int               `json:"servicos_sem_dado"`
	ClientesTodosOK      int               `json:"clientes_todos_ok"`
	ClientesComDiferenca []string          `json:"clientes_com_diferenca,omitempty"`
	NaoEncontrados       []string          `json:"nao_encontrados,omitempty"`
	Erros                []string          `json:"erros,omitempty"`
	Rows                 []ServiceFormaRow `json:"rows"`
	// CamposServico: campos (e valores) de um serviço, para diagnóstico quando a forma não vem.
	CamposServico map[string]string `json:"campos_servico,omitempty"`
}

// CheckServiceFormas confere, para cada id_cliente, a forma de cobrança dos serviços contra `esperada` (id numérico da
// forma ou parte do nome). Só leitura; consultas em paralelo moderado.
func CheckServiceFormas(ctx context.Context, cfg Config, token string, ids []string, esperada string) ServiceFormaReport {
	rep := ServiceFormaReport{Esperada: strings.TrimSpace(esperada), Rows: []ServiceFormaRow{}}
	ids = uniqueNonEmpty(ids)
	if rep.Esperada == "" {
		rep.Message = "Informe a forma de cobrança esperada."
		return rep
	}
	if len(ids) == 0 {
		rep.Message = "Nenhum cliente para conferir."
		return rep
	}
	if len(ids) > serviceFormaMaxClient {
		rep.Message = fmt.Sprintf("Clientes demais para uma conferência (%d; máximo %d). Filtre a lista.", len(ids), serviceFormaMaxClient)
		return rep
	}

	type clientOut struct {
		id     string
		nome   string
		found  bool
		rows   []ServiceFormaRow
		sample map[string]string
		err    error
	}
	outs := make([]clientOut, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			outs[i] = clientOut{id: id}
			svcs, nome, found, err := fetchClientServices(ctx, cfg, token, id)
			outs[i].nome, outs[i].found, outs[i].err = nome, found, err
			for _, sv := range svcs {
				fid, fnome := invoiceFormaCobranca(sv)
				row := ServiceFormaRow{
					IDCliente: id, Cliente: nome, IDClienteServico: pickStr(sv, "id_cliente_servico"),
					Login: pickStr(sv, "login"), Plano: pickStr(sv, "nome", "descricao"), Status: pickStr(sv, "status"),
					FormaID: fid, FormaNome: fnome,
				}
				switch {
				case fid == "" && fnome == "":
					row.Resultado = ServiceFormaNoData
				case formaMatches(fid, fnome, rep.Esperada):
					row.Resultado = ServiceFormaOK
				default:
					row.Resultado = ServiceFormaDiff
				}
				outs[i].rows = append(outs[i].rows, row)
				if outs[i].sample == nil {
					outs[i].sample = flattenInvoiceSample(sv)
				}
			}
		}(i, id)
	}
	wg.Wait()

	rep.OK = true
	rep.Clientes = len(ids)
	for _, o := range outs {
		switch {
		case o.err != nil:
			rep.Erros = append(rep.Erros, fmt.Sprintf("cliente %s: %v", o.id, o.err))
			continue
		case !o.found:
			rep.NaoEncontrados = append(rep.NaoEncontrados, o.id)
			continue
		}
		if rep.CamposServico == nil && o.sample != nil {
			rep.CamposServico = o.sample
		}
		allOK, any := true, false
		for _, r := range o.rows {
			// serviço cancelado não conta contra o cliente (a forma dele não importa mais)
			if strings.Contains(normText(r.Status), "cancel") {
				rep.Rows = append(rep.Rows, r)
				continue
			}
			any = true
			rep.Servicos++
			switch r.Resultado {
			case ServiceFormaOK:
				rep.ServicosOK++
			case ServiceFormaDiff:
				rep.ServicosDiff++
				allOK = false
			default:
				rep.ServicosSemDado++
				allOK = false
			}
			rep.Rows = append(rep.Rows, r)
		}
		if any && allOK {
			rep.ClientesTodosOK++
		} else if any {
			rep.ClientesComDiferenca = append(rep.ClientesComDiferenca, o.id)
		}
	}
	sort.SliceStable(rep.Rows, func(i, j int) bool {
		a, b := rep.Rows[i], rep.Rows[j]
		if (a.Resultado == ServiceFormaOK) != (b.Resultado == ServiceFormaOK) {
			return b.Resultado == ServiceFormaOK // divergentes primeiro
		}
		return normText(a.Cliente) < normText(b.Cliente)
	})
	if rep.ServicosSemDado > 0 && rep.ServicosSemDado == rep.Servicos {
		rep.Message = "A consulta de cliente da HubSoft não devolveu a forma de cobrança dos serviços — veja os campos que o serviço traz."
	}
	return rep
}

// fetchClientServices lê os serviços (objetos JSON crus) de um cliente por id_cliente, incluindo cancelados.
func fetchClientServices(ctx context.Context, cfg Config, token, idCliente string) (svcs []map[string]any, nome string, found bool, err error) {
	res := integrationhttp.Execute(ctx, cfg.integ(token), integrationhttp.RequestConfig{
		Method: "GET", Path: "/api/v1/integracao/cliente",
		QueryParams: paramKVs(map[string]string{"busca": "id_cliente", "termo_busca": idCliente, "limit": "5", "cancelado": "todos", "inativo": "todos"}),
	})
	body := ResponseBodyBytes(res)
	if !res.OK {
		if res.StatusCode == 0 {
			return nil, "", false, fmt.Errorf("sem resposta da HubSoft")
		}
		return nil, "", false, fmt.Errorf("%s", firstNonEmpty(hubsoftActionMessageWithErrors(body), fmt.Sprintf("HTTP %d", res.StatusCode)))
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return nil, "", false, fmt.Errorf("resposta inválida da HubSoft")
	}
	for _, it := range extractArray(doc, "clientes") {
		c, ok := it.(map[string]any)
		if !ok || pickStr(c, "id_cliente") != idCliente { // confirma o id — a busca pode não ser estritamente exata
			continue
		}
		arr, _ := c["servicos"].([]any)
		for _, sv := range arr {
			if m, ok := sv.(map[string]any); ok {
				svcs = append(svcs, m)
			}
		}
		return svcs, pickStr(c, "nome_razaosocial"), true, nil
	}
	return nil, "", false, nil
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

package integrationhubsoft

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// --- Conferência dos 4 endereços do serviço (fiscal, cadastral, cobrança, instalação) ----------------------------------
//
// A HubSoft guarda, por serviço, quatro endereços (servicos[].endereco_fiscal / _cadastral / _cobranca / _instalacao,
// pedidos com relacoes=…). Esta rotina lê a base inteira (somente leitura) e lista os serviços em que os quatro NÃO são
// iguais, classificando o padrão:
//   - "fiscal_diferente"    — o fiscal é um e cadastral, cobrança e instalação são outro (todos iguais entre si);
//   - "instalacao_diferente" — fiscal, cadastral e cobrança iguais e só a instalação é outra (ex.: 2º ponto);
//   - "outra".
// A documentação oficial da API (docs.hubsoft.com.br, conferida) não tem rota para editar nem sincronizar endereço: só
// "Cadastrar/Migrar Cliente Serviço" aceitam um endereco_instalacao novo (e ambos CRIAM um serviço). Por isso a correção
// é feita na própria HubSoft (função «sincronizar endereços»).

type AddressCheckRow struct {
	IDCliente        string   `json:"id_cliente"`
	CodigoCliente    string   `json:"codigo_cliente,omitempty"`
	Nome             string   `json:"nome"`
	IDClienteServico string   `json:"id_cliente_servico"`
	Login            string   `json:"login"`
	Status           string   `json:"status,omitempty"`
	NServicos        int      `json:"n_servicos"` // serviços do cliente (os lidos nesta varredura)
	Padrao           string   `json:"padrao"`
	Diferentes       []string `json:"diferentes"` // tipos que diferem do fiscal
	Fiscal           string   `json:"fiscal"`
	Cadastral        string   `json:"cadastral"`
	Cobranca         string   `json:"cobranca"`
	Instalacao       string   `json:"instalacao"`
}

type AddressCheckResult struct {
	ClientesLidos int               `json:"clientes_lidos"`
	ServicosLidos int               `json:"servicos_lidos"`
	Iguais        int               `json:"iguais"`
	Rows          []AddressCheckRow `json:"rows"`
	Incompleto    bool              `json:"incompleto,omitempty"`
	Aviso         string            `json:"aviso,omitempty"`
}

var addrNonAlnum = regexp.MustCompile(`[^A-Z0-9]`)

func addrNorm(s string) string {
	return addrNonAlnum.ReplaceAllString(strings.ToUpper(stripAccents(s)), "")
}

// addressKey identifica o endereço por logradouro + número + bairro + CEP + cidade (complemento/referência não contam).
func addressKey(a map[string]any) (key, full string) {
	if a == nil {
		return "", ""
	}
	key = strings.Join([]string{
		addrNorm(pickStr(a, "endereco")), addrNorm(pickStr(a, "numero")), addrNorm(pickStr(a, "bairro")),
		addrNorm(pickStr(a, "cep")), addrNorm(pickStr(a, "cidade")),
	}, "|")
	full = strings.TrimSpace(pickStr(a, "completo"))
	if full == "" {
		full = strings.TrimSpace(fmt.Sprintf("%s, %s - %s, %s | CEP: %s", pickStr(a, "endereco"), pickStr(a, "numero"), pickStr(a, "bairro"), pickStr(a, "cidade"), pickStr(a, "cep")))
	}
	if strings.Trim(key, "|") == "" {
		return "", ""
	}
	return key, full
}

// ClassifyAddresses compara os quatro endereços de um serviço. Devolve "" quando todos são iguais.
func ClassifyAddresses(fiscal, cadastral, cobranca, instalacao string) (padrao string, diferentes []string) {
	others := map[string]string{"cadastral": cadastral, "cobranca": cobranca, "instalacao": instalacao}
	for _, t := range []string{"cadastral", "cobranca", "instalacao"} {
		if others[t] != fiscal {
			diferentes = append(diferentes, t)
		}
	}
	if len(diferentes) == 0 {
		return "", nil
	}
	switch {
	case len(diferentes) == 3 && cadastral == cobranca && cobranca == instalacao:
		return "fiscal_diferente", diferentes
	case len(diferentes) == 1 && diferentes[0] == "instalacao":
		return "instalacao_diferente", diferentes
	default:
		return "outra", diferentes
	}
}

// ScanAddresses lê todos os clientes (e serviços) da HubSoft com os quatro endereços e lista os que não são iguais.
func ScanAddresses(ctx context.Context, cfg Config, token string, includeCancelled bool) (AddressCheckResult, error) {
	var out AddressCheckResult
	q := map[string]string{"relacoes": "endereco_instalacao,endereco_fiscal,endereco_cobranca,endereco_cadastral"}
	if includeCancelled {
		q["cancelado"] = "sim"
	} else {
		q["cancelado"] = "nao"
	}
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", q, 200, "clientes")
	if err != nil {
		return out, err
	}
	out.ClientesLidos = len(items)
	if total > len(items) {
		out.Incompleto = true
		out.Aviso = fmt.Sprintf("Leitura incompleta da base (%d de %d clientes) — rode de novo.", len(items), total)
	}
	for _, c := range items {
		svcs := servicesOf(c)
		for _, s := range svcs {
			out.ServicosLidos++
			var keys, fulls [4]string
			for i, t := range []string{"fiscal", "cadastral", "cobranca", "instalacao"} {
				a, _ := s["endereco_"+t].(map[string]any)
				keys[i], fulls[i] = addressKey(a)
			}
			padrao, dif := ClassifyAddresses(keys[0], keys[1], keys[2], keys[3])
			if padrao == "" {
				out.Iguais++
				continue
			}
			out.Rows = append(out.Rows, AddressCheckRow{
				IDCliente: pickStr(c, "id_cliente"), CodigoCliente: pickStr(c, "codigo_cliente"), Nome: pickStr(c, "nome_razaosocial"),
				IDClienteServico: pickStr(s, "id_cliente_servico"), Login: pickStr(s, "login"), Status: pickStr(s, "status"),
				NServicos: len(svcs), Padrao: padrao, Diferentes: dif,
				Fiscal: fulls[0], Cadastral: fulls[1], Cobranca: fulls[2], Instalacao: fulls[3],
			})
		}
	}
	return out, nil
}

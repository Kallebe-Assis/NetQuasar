package integrationhubsoft

import (
	"context"
	"fmt"
	"strings"
)

// --- Correção de senhas gravadas como texto de planilha ( ="12345" ) -----------------------------------------
//
// O Excel/IXC exporta números como fórmula de texto — `="12345"` — e, importada sem tratar, a HubSoft gravou a
// senha LITERALMENTE assim (com o "=" e as aspas). Esta rotina varre a base inteira (somente leitura) atrás de
// senhas nesse formato e, quando o operador manda, troca cada uma pela senha limpa (só o conteúdo entre aspas).

// literalExcelPassword — "=\"12345\"" → "12345". ok=false se a senha não tem o formato ="…".
func literalExcelPassword(s string) (clean string, ok bool) {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, `="`) {
		return "", false
	}
	inner := strings.TrimPrefix(t, `="`)
	inner = strings.TrimSuffix(inner, `"`)
	return inner, true
}

type LiteralPasswordRow struct {
	IDCliente        string `json:"id_cliente"`
	CodigoCliente    string `json:"codigo_cliente,omitempty"`
	Nome             string `json:"nome"`
	IDClienteServico string `json:"id_cliente_servico"`
	Login            string `json:"login"`
	Status           string `json:"status,omitempty"`
	SenhaAtual       string `json:"senha_atual"`
	SenhaCorreta     string `json:"senha_correta"`
	Problema         string `json:"problema,omitempty"` // preenchido quando NÃO dá para corrigir sozinho
}

type LiteralPasswordScan struct {
	ClientesLidos int                  `json:"clientes_lidos"`
	ServicosLidos int                  `json:"servicos_lidos"`
	Rows          []LiteralPasswordRow `json:"rows"`
	Incompleto    bool                 `json:"incompleto,omitempty"`
	MensagemAviso string               `json:"aviso,omitempty"`
}

// ScanLiteralPasswords percorre /cliente/todos (todos os serviços não cancelados — serviço cancelado não aceita
// alteração de autenticação) e devolve os que têm senha no formato ="…".
func ScanLiteralPasswords(ctx context.Context, cfg Config, token string) (LiteralPasswordScan, error) {
	var out LiteralPasswordScan
	items, total, err := fetchAllPages(ctx, cfg, token, "/api/v1/integracao/cliente/todos", map[string]string{"cancelado": "nao"}, 400, "clientes")
	if err != nil {
		return out, err
	}
	out.ClientesLidos = len(items)
	if total > len(items) {
		out.Incompleto = true
		out.MensagemAviso = fmt.Sprintf("Leitura incompleta da base (%d de %d clientes) — rode de novo para garantir que nenhum ficou de fora.", len(items), total)
	}
	for _, c := range items {
		for _, s := range servicesOf(c) {
			out.ServicosLidos++
			clean, ok := literalExcelPassword(pickStr(s, "senha"))
			if !ok {
				continue
			}
			row := LiteralPasswordRow{
				IDCliente: pickStr(c, "id_cliente"), CodigoCliente: pickStr(c, "codigo_cliente"), Nome: pickStr(c, "nome_razaosocial"),
				IDClienteServico: pickStr(s, "id_cliente_servico"), Login: pickStr(s, "login"), Status: pickStr(s, "status"),
				SenhaAtual: pickStr(s, "senha"), SenhaCorreta: clean,
			}
			if len(clean) < 3 {
				row.Problema = "a senha limpa teria menos de 3 caracteres — a HubSoft não aceita; corrija à mão"
			}
			out.Rows = append(out.Rows, row)
		}
	}
	return out, nil
}

type LiteralPasswordFixResult struct {
	IDClienteServico string `json:"id_cliente_servico"`
	Login            string `json:"login"`
	OK               bool   `json:"ok"`
	Skipped          bool   `json:"skipped"`
	Message          string `json:"message"`
}

// FixLiteralPassword relê o serviço e, SOMENTE se a senha atual ainda estiver no formato ="…", grava a senha
// limpa (só password no corpo — login e demais campos não são tocados) e confere o resultado. Qualquer outra
// senha (por exemplo, já corrigida ou trocada pelo cliente) nunca é alterada.
func FixLiteralPassword(ctx context.Context, cfg Config, token, idClienteServico string) LiteralPasswordFixResult {
	res := LiteralPasswordFixResult{IDClienteServico: strings.TrimSpace(idClienteServico)}
	if !isPosInt(res.IDClienteServico) {
		res.Message = fmt.Sprintf("id_cliente_servico inválido (%q)", idClienteServico)
		return res
	}
	login, senha, found := serviceAuth(ctx, cfg, token, res.IDClienteServico)
	res.Login = login
	if !found {
		res.Message = "serviço não encontrado na HubSoft"
		return res
	}
	clean, ok := literalExcelPassword(senha)
	if !ok {
		res.OK, res.Skipped = true, true
		res.Message = "a senha atual não está mais no formato =\"…\" — nada a fazer"
		return res
	}
	if len(clean) < 3 {
		res.Message = "a senha limpa teria menos de 3 caracteres — a HubSoft não aceita; corrija à mão"
		return res
	}
	fix := postLoginConfig(ctx, cfg, token, res.IDClienteServico, "", "", clean)
	if !fix.OK {
		res.Message = fix.Message
		return res
	}
	if _, again, ok2 := serviceAuth(ctx, cfg, token, res.IDClienteServico); ok2 && again != clean {
		res.Message = fmt.Sprintf("a HubSoft respondeu sucesso, mas a senha continua %q (esperado %q)", again, clean)
		return res
	}
	res.OK = true
	res.Message = fmt.Sprintf("senha corrigida de %q para %q", senha, clean)
	return res
}

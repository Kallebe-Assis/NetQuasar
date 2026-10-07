// Package integrationixc — operações do NetQuasar sobre logins (radusuarios) do IXC: conferir e inativar/reativar em massa.
//
// A escrita foi validada ao vivo (login de teste): PUT /radusuarios/{id} com o registro COMPLETO lido antes, só com `ativo`
// trocado ("S" ativo, "N" inativo). O IXC responde "Registro atualizado com sucesso!" e derruba a sessão PPPoE do login.
package integrationixc

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/integrationhttp"
)

type Client struct {
	Cfg integrationhttp.IntegrationConfig
}

// Record é o registro bruto de radusuarios devolvido pelo IXC.
type Record map[string]any

func (r Record) Str(key string) string {
	switch v := r[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func (r Record) Active() bool { return strings.EqualFold(r.Str("ativo"), "S") }
func (r Record) Online() bool { return strings.EqualFold(r.Str("online"), "S") }

type listResp struct {
	Total     any              `json:"total"`
	Registros []map[string]any `json:"registros"`
}

func (c Client) do(ctx context.Context, method, path string, headers map[string]string, body any) (integrationhttp.RunResult, error) {
	rc := integrationhttp.RequestConfig{Method: method, Path: path, Headers: headers, BodyType: "json"}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return integrationhttp.RunResult{}, err
		}
		rc.BodyTemplate = string(b)
	}
	res := integrationhttp.Execute(ctx, c.Cfg, rc)
	if !res.OK {
		msg := res.ErrorMessage
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", res.StatusCode)
		}
		return res, fmt.Errorf("%s", msg)
	}
	return res, nil
}

func (c Client) list(ctx context.Context, table, qtype, value string) ([]Record, error) {
	q := map[string]any{"qtype": table + "." + qtype, "query": value, "oper": "=", "page": "1", "rp": "10", "sortname": table + ".id", "sortorder": "desc"}
	res, err := c.do(ctx, "POST", "/"+table, map[string]string{"ixcsoft": "listar"}, q)
	if err != nil {
		return nil, err
	}
	var doc listResp
	if err := json.Unmarshal([]byte(res.ResponsePreview), &doc); err != nil {
		return nil, fmt.Errorf("resposta do IXC não reconhecida")
	}
	out := make([]Record, 0, len(doc.Registros))
	for _, m := range doc.Registros {
		out = append(out, Record(m))
	}
	return out, nil
}

// FindByLogin devolve todos os registros com exatamente esse login (normalmente 0 ou 1).
func (c Client) FindByLogin(ctx context.Context, login string) ([]Record, error) {
	recs, err := c.list(ctx, "radusuarios", "login", strings.TrimSpace(login))
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, r := range recs {
		if strings.EqualFold(r.Str("login"), strings.TrimSpace(login)) {
			out = append(out, r)
		}
	}
	return out, nil
}

// FindByID lê um login pelo id do IXC.
func (c Client) FindByID(ctx context.Context, id string) (Record, error) {
	recs, err := c.list(ctx, "radusuarios", "id", strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		if r.Str("id") == strings.TrimSpace(id) {
			return r, nil
		}
	}
	return nil, nil
}

// ClientName devolve a razão social do cliente (vazio se não achar).
func (c Client) ClientName(ctx context.Context, idCliente string) string {
	recs, err := c.list(ctx, "cliente", "id", idCliente)
	if err != nil || len(recs) == 0 {
		return ""
	}
	for _, r := range recs {
		if r.Str("id") == strings.TrimSpace(idCliente) {
			return r.Str("razao")
		}
	}
	return ""
}

// SetActive troca `ativo` do login e confere a gravação lendo de novo. Devolve o registro ANTES da troca (para o
// histórico/desfazer) e o DEPOIS. Não altera nada se o login já estiver no estado pedido.
func (c Client) SetActive(ctx context.Context, id, loginExpected string, active bool) (before, after Record, changed bool, err error) {
	before, err = c.FindByID(ctx, id)
	if err != nil {
		return nil, nil, false, err
	}
	if before == nil {
		return nil, nil, false, fmt.Errorf("login id %s não encontrado no IXC", id)
	}
	if loginExpected != "" && !strings.EqualFold(before.Str("login"), strings.TrimSpace(loginExpected)) {
		return before, nil, false, fmt.Errorf("o id %s pertence ao login %q, não a %q — nada foi alterado", id, before.Str("login"), loginExpected)
	}
	if before.Active() == active {
		return before, before, false, nil
	}
	body := map[string]any{}
	for k, v := range before {
		body[k] = v
	}
	if active {
		body["ativo"] = "S"
	} else {
		body["ativo"] = "N"
	}
	res, err := c.do(ctx, "PUT", "/radusuarios/"+id, nil, body)
	if err != nil {
		return before, nil, false, err
	}
	var ack struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal([]byte(res.ResponsePreview), &ack)
	if ack.Type != "" && !strings.EqualFold(ack.Type, "success") {
		return before, nil, false, fmt.Errorf("o IXC recusou: %s", strings.TrimSpace(ack.Message))
	}
	after, err = c.FindByID(ctx, id)
	if err != nil || after == nil {
		return before, nil, true, fmt.Errorf("o IXC aceitou, mas não foi possível reler o login para conferir")
	}
	if after.Active() != active {
		return before, after, true, fmt.Errorf("o IXC respondeu sucesso, mas o login continua com ativo=%s", after.Str("ativo"))
	}
	return before, after, true, nil
}

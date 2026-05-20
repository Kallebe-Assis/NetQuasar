package integrationconsumer

import "encoding/json"

// ClientSearchConfig liga a consulta de cliente na UI aa requisição HTTP configurada.
type ClientSearchConfig struct {
	Enabled   bool   `json:"enabled"`
	RequestID string `json:"request_id,omitempty"`
}

// Config agrupa acções de consumo expostas na UI do NetQuasar.
type Config struct {
	ClientSearch ClientSearchConfig `json:"client_search"`
}

func ConfigFromJSON(b []byte) Config {
	var c Config
	_ = json.Unmarshal(b, &c)
	return c
}

func (c Config) ToJSON() json.RawMessage {
	out, _ := json.Marshal(c)
	return out
}

// BuscaOption tipo de pesquisa Hubsoft (GET /integracao/cliente).
type BuscaOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// BuscaOptions lista documentada pela API Hubsoft.
func BuscaOptions() []BuscaOption {
	return []BuscaOption{
		{Value: "cpf_cnpj", Label: "CPF/CNPJ"},
		{Value: "nome_razaosocial", Label: "Nome / Razão social"},
		{Value: "nome_fantasia", Label: "Nome fantasia"},
		{Value: "codigo_cliente", Label: "Código do cliente"},
		{Value: "telefone", Label: "Telefone"},
		{Value: "login_radius", Label: "Login RADIUS"},
		{Value: "email", Label: "E-mail"},
	}
}

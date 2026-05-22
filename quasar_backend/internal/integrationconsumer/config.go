package integrationconsumer

import "encoding/json"

// ClientSearchConfig liga a consulta de cliente na UI à requisição HTTP configurada.
type ClientSearchConfig struct {
	Enabled   bool   `json:"enabled"`
	RequestID string `json:"request_id,omitempty"`
	// Provider: auto | hubsoft | ixc | generic (auto detecta pela requisição/URL).
	Provider string `json:"provider,omitempty"`
	// IxcListAction valor do header ixcsoft na listagem (padrão listar).
	IxcListAction string `json:"ixc_list_action,omitempty"`
	// BuscaOptions tipos de busca na UI; vazio = padrão do ERP detectado.
	BuscaOptions []BuscaOption `json:"busca_options,omitempty"`
	// FieldMappings por chave de busca (ex. cpf_cnpj → qtype, oper, termo_format).
	FieldMappings map[string]SearchFieldConfig `json:"field_mappings,omitempty"`
	// CpfMultiAttempt tenta várias combinações qtype/oper para CPF (padrão true).
	CpfMultiAttempt *bool `json:"cpf_multi_attempt,omitempty"`
}

// ClientAttendanceConfig liga a consulta de atendimentos na UI à requisição HTTP.
type ClientAttendanceConfig struct {
	Enabled   bool   `json:"enabled"`
	RequestID string `json:"request_id,omitempty"`
	// Provider: auto | hubsoft | ixc | generic.
	Provider string `json:"provider,omitempty"`
	// IxcListAction header ixcsoft (padrão listar).
	IxcListAction string `json:"ixc_list_action,omitempty"`
	// FieldMappings qtype/oper por tipo de busca (codigo_cliente, cpf_cnpj, …).
	FieldMappings map[string]SearchFieldConfig `json:"field_mappings,omitempty"`
}

// ClientWorkOrderConfig liga a consulta de ordens de serviço na UI.
type ClientWorkOrderConfig struct {
	Enabled   bool   `json:"enabled"`
	RequestID string `json:"request_id,omitempty"`
	// Provider: auto | hubsoft | ixc | generic.
	Provider string `json:"provider,omitempty"`
	// IxcListAction header ixcsoft (padrão listar).
	IxcListAction string `json:"ixc_list_action,omitempty"`
	// FieldMappings qtype/oper por tipo de busca; vazio reutiliza client_search.
	FieldMappings map[string]SearchFieldConfig `json:"field_mappings,omitempty"`
}

// Config agrupa acções de consumo expostas na UI do NetQuasar.
type Config struct {
	ClientSearch     ClientSearchConfig     `json:"client_search"`
	ClientAttendance ClientAttendanceConfig `json:"client_attendance"`
	ClientWorkOrder  ClientWorkOrderConfig  `json:"client_work_order"`
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

// BuscaOptions lista documentada pela API Hubsoft (GET /integracao/cliente).
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

// BuscaAtendimentoOptions tipos de busca (GET /integracao/cliente/atendimento).
func BuscaAtendimentoOptions() []BuscaOption {
	return []BuscaOption{
		{Value: "codigo_cliente", Label: "Código do cliente"},
		{Value: "cpf_cnpj", Label: "CPF/CNPJ"},
		{Value: "id_cliente_servico", Label: "ID cliente serviço"},
		{Value: "protocolo", Label: "Protocolo"},
	}
}

// BuscaOrdemServicoOptions tipos de busca (GET /integracao/cliente/ordem_servico).
func BuscaOrdemServicoOptions() []BuscaOption {
	return []BuscaOption{
		{Value: "codigo_cliente", Label: "Código do cliente"},
		{Value: "cpf_cnpj", Label: "CPF/CNPJ"},
		{Value: "id_cliente_servico", Label: "ID cliente serviço"},
		{Value: "numero_ordem_servico", Label: "Número da O.S."},
	}
}

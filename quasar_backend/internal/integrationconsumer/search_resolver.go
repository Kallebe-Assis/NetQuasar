package integrationconsumer

import (
	"fmt"
	"strings"
)

// SearchFieldConfig mapeamento configurável por tipo de busca (UI → API do ERP).
type SearchFieldConfig struct {
	Qtype       string `json:"qtype,omitempty"`
	Oper        string `json:"oper,omitempty"`
	TermoFormat string `json:"termo_format,omitempty"` // digits | raw | br_document
}

// ResolveQtype devolve qtype da config ou padrão do ERP.
func (cfg ClientSearchConfig) ResolveQtype(busca string) string {
	if m := cfg.fieldMapping(busca); m.Qtype != "" {
		return m.Qtype
	}
	return IXCBuscaToQtype(busca)
}

// ResolveOper devolve operador da config ou padrão.
func (cfg ClientSearchConfig) ResolveOper(busca string) string {
	if m := cfg.fieldMapping(busca); m.Oper != "" {
		return m.Oper
	}
	return IXCOperForBusca(busca)
}

// ResolveTermo formata o termo conforme a config.
func (cfg ClientSearchConfig) ResolveTermo(busca, termo string) string {
	format := cfg.fieldMapping(busca).TermoFormat
	if format == "" {
		return IXCTermoForQuery(busca, termo)
	}
	termo = strings.TrimSpace(termo)
	switch strings.ToLower(format) {
	case "digits":
		return digitsOnly(termo)
	case "br_document":
		d := digitsOnly(termo)
		if fd := formatBRDocument(d); fd != "" {
			return fd
		}
		return d
	default:
		return termo
	}
}

func (cfg ClientSearchConfig) fieldMapping(busca string) SearchFieldConfig {
	if cfg.FieldMappings == nil {
		return SearchFieldConfig{}
	}
	if m, ok := cfg.FieldMappings[strings.ToLower(strings.TrimSpace(busca))]; ok {
		return m
	}
	return SearchFieldConfig{}
}

// IxcListHeaderValue ação do header ixcsoft (padrão listar).
func (cfg ClientSearchConfig) IxcListHeaderValue() string {
	v := strings.TrimSpace(cfg.IxcListAction)
	if v == "" {
		return "listar"
	}
	return v
}

// CpfMultiAttemptEnabled tentativas múltiplas para CPF/CNPJ (padrão ligado).
func (cfg ClientSearchConfig) CpfMultiAttemptEnabled() bool {
	if cfg.CpfMultiAttempt == nil {
		return true
	}
	return *cfg.CpfMultiAttempt
}

// BuscaOptionsResolved opções da UI: customizadas ou padrão do perfil.
func BuscaOptionsResolved(cfg ClientSearchConfig, profile string) []BuscaOption {
	if len(cfg.BuscaOptions) > 0 {
		return cfg.BuscaOptions
	}
	return BuscaOptionsForProfile(profile)
}

// BuildIXCSearchAttempts monta tentativas IXC (config + padrões).
func BuildIXCSearchAttempts(cfg ClientSearchConfig, busca, termo string) []IXCSearchAttempt {
	busca = strings.ToLower(strings.TrimSpace(busca))
	if busca != "cpf_cnpj" {
		return []IXCSearchAttempt{{
			Qtype: cfg.ResolveQtype(busca),
			Query: cfg.ResolveTermo(busca, termo),
			Oper:  cfg.ResolveOper(busca),
		}}
	}
	if !cfg.CpfMultiAttemptEnabled() {
		return []IXCSearchAttempt{{
			Qtype: cfg.ResolveQtype(busca),
			Query: cfg.ResolveTermo(busca, termo),
			Oper:  cfg.ResolveOper(busca),
		}}
	}
	return buildDefaultCPFAttempts(cfg, termo)
}

func buildDefaultCPFAttempts(cfg ClientSearchConfig, termo string) []IXCSearchAttempt {
	d := digitsOnly(termo)
	t := strings.TrimSpace(termo)
	var attempts []IXCSearchAttempt
	add := func(qtype, query, oper string) {
		query = strings.TrimSpace(query)
		if query == "" {
			return
		}
		attempts = append(attempts, IXCSearchAttempt{Qtype: qtype, Query: query, Oper: oper})
	}
	primaryQ := cfg.ResolveQtype("cpf_cnpj")
	primaryO := cfg.ResolveOper("cpf_cnpj")
	add(primaryQ, cfg.ResolveTermo("cpf_cnpj", termo), primaryO)
	add("cnpj_cpf", d, "=")
	if fd := formatBRDocument(d); fd != d {
		add("cliente.cnpj_cpf", fd, "=")
		add("cnpj_cpf", fd, "=")
	}
	add("cliente.cnpj_cpf", d, ">")
	add("cnpj_cpf", d, "L")
	if t != "" && t != d {
		add("cliente.cnpj_cpf", t, "L")
	}
	return dedupeIXCAttempts(attempts)
}

func formatBRDocument(digits string) string {
	switch len(digits) {
	case 11:
		return fmt.Sprintf("%s.%s.%s-%s", digits[0:3], digits[3:6], digits[6:9], digits[9:11])
	case 14:
		return fmt.Sprintf("%s.%s.%s/%s-%s", digits[0:2], digits[2:5], digits[5:8], digits[8:12], digits[12:14])
	default:
		return digits
	}
}

func dedupeIXCAttempts(in []IXCSearchAttempt) []IXCSearchAttempt {
	seen := map[string]struct{}{}
	var out []IXCSearchAttempt
	for _, a := range in {
		key := a.Qtype + "|" + a.Query + "|" + a.Oper
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, a)
	}
	return out
}

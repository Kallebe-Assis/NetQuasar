package integrationconsumer

import "strings"

// mapIXCServices extrai planos/logins do registro IXC (cliente, contrato, radusuarios, etc.).
func mapIXCServices(m map[string]any) []ServiceSummary {
	out := mapServices(m)
	seen := map[string]struct{}{}
	for _, s := range out {
		seen[servicesDedupeKey(s)] = struct{}{}
	}
	appendSvc := func(sm map[string]any) {
		svc := mapIXCServiceItem(sm)
		if svc.Name == "" && svc.Login == "" && svc.IPv4 == "" && svc.ID == "" {
			return
		}
		k := servicesDedupeKey(svc)
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, svc)
	}
	ixcKeys := []string{
		"contrato", "contratos", "cliente_contrato", "cliente_contratos",
		"radusuarios", "logins", "login", "acesso", "acessos", "planos", "plano",
		"servicos", "services", "cliente_servico", "cliente_servicos",
	}
	for _, key := range ixcKeys {
		if raw, ok := m[key].([]any); ok {
			for _, it := range raw {
				if sm, ok := it.(map[string]any); ok {
					appendSvc(sm)
				}
			}
			continue
		}
		if sm, ok := m[key].(map[string]any); ok {
			appendSvc(sm)
		}
	}
	// Um único login/contrato embutido no cliente.
	if login := pickStr(m, "login", "login_pppoe", "usuario", "login_radius"); login != "" {
		appendSvc(map[string]any{
			"id":                    pickStr(m, "id_login", "id_radusuarios"),
			"login":                 login,
			"nome":                  pickStr(m, "nome_plano", "plano", "descricao_plano"),
			"contrato_plano_venda_": pickStr(m, "contrato_plano_venda_", "contrato_plano_venda"),
			"contrato":              pickStr(m, "contrato", "id_contrato"),
			"status_internet":       pickStr(m, "status_internet"),
			"online":                pickStr(m, "online", "status_conexao"),
			"mac":                   pickStr(m, "mac"),
			"ip":                    pickStr(m, "ip", "ipv4", "ip_fixo"),
		})
	}
	out = enrichServicesWithContractStatus(out)
	return out
}

// enrichServicesWithContractStatus propaga status_internet entre registros já carregados (ex.: contrato[] + login).
func enrichServicesWithContractStatus(services []ServiceSummary) []ServiceSummary {
	if len(services) == 0 {
		return services
	}
	index := map[string]string{}
	for _, s := range services {
		if s.StatusInternet == "" {
			continue
		}
		for _, k := range []string{s.ContratoID, s.Contrato, s.ID} {
			k = strings.TrimSpace(k)
			if k != "" {
				index[k] = s.StatusInternet
			}
		}
	}
	if len(index) == 0 {
		return services
	}
	return ApplyContractStatusIndex(services, index)
}

func servicesDedupeKey(s ServiceSummary) string {
	return s.ID + "|" + s.Login + "|" + s.IPv4 + "|" + s.Name
}

func mapIXCServiceItem(sm map[string]any) ServiceSummary {
	svc := mapServiceItem(sm)
	svc.MAC = pickStr(sm, "mac")
	svc.Online = pickStr(sm, "online", "ativo")
	svc.OnlineLabel = FormatIXCOnline(svc.Online)
	svc.ContratoID = firstNonEmpty(
		pickStr(sm, "id_contrato"),
		pickStr(sm, "id_cliente_contrato"),
		pickStr(sm, "id_contrato_login"),
	)
	svc.Contrato = pickStr(sm, "contrato", "numero_contrato")
	svc.PlanoVenda = firstNonEmpty(
		pickStr(sm, "contrato_plano_venda_", "contrato_plano_venda", "plano_venda"),
	)
	svc.StatusInternet = pickIXCStatusInternet(sm)
	svc.StatusLabel = FormatIXCStatusInternet(svc.StatusInternet)
	svc.ClientID = pickStr(sm, "id_cliente", "cliente_id", "id_cliente_rad")
	svc.ClientName = ixcClientNameFromRow(sm)

	if svc.Name == "" {
		svc.Name = firstNonEmpty(
			svc.PlanoVenda,
			pickStr(sm, "nome_plano", "plano", "pacote", "id_grupo", "descricao_plano"),
			pickStr(sm, "tipo_conexao"),
		)
	}
	if svc.Login == "" {
		svc.Login = pickStr(sm, "login", "usuario", "login_pppoe", "login_radius")
	}
	if svc.Status == "" {
		svc.Status = firstNonEmpty(pickStr(sm, "status_servico", "status_contrato"), pickStr(sm, "status"))
	}
	if svc.ID == "" {
		svc.ID = firstNonEmpty(
			pickStr(sm, "id_radusuarios", "id_login"),
			pickStr(sm, "id"),
		)
	}
	if svc.ContratoID == "" && svc.Login != "" {
		svc.ContratoID = firstNonEmpty(
			pickStr(sm, "id_contrato", "id_cliente_contrato"),
			pickStr(sm, "id_contrato_login"),
		)
	}
	if svc.IPv4 == "" {
		svc.IPv4 = pickIPv4FromMap(sm)
	}
	return svc
}

func mergeServiceLists(base, extra []ServiceSummary) []ServiceSummary {
	byKey := map[string]int{}
	var out []ServiceSummary
	mergeInto := func(s ServiceSummary) {
		k := servicesDedupeKey(s)
		if idx, ok := byKey[k]; ok {
			out[idx] = mergeServiceSummary(out[idx], s)
			return
		}
		byKey[k] = len(out)
		out = append(out, s)
	}
	for _, s := range base {
		mergeInto(s)
	}
	for _, s := range extra {
		mergeInto(s)
	}
	return out
}

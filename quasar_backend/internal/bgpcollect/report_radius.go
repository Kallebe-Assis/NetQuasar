package bgpcollect

import "sort"

// report_radius.go — pivot de saúde do RADIUS por servidor (HUAWEI-BRAS-RADIUS-MIB). Cada
// tabela de estatística (Authen/Acct) já traz o IP do servidor como parte do próprio índice
// (hwRadiusStatAuthenIpv4ServerIP.Vrf / hwRadiusStatAcctIpv4ServerIP.Vrf) — não precisa de
// cruzar com hwRadiusServerTable, extraio o IP directamente do sufixo da OID (mesma heurística
// de lastIPFromSuffix já usada para prefixos de peer). Aviso: esta MIB é tipicamente do lado
// BNG/AAA do chassi — pode não estar visível no agente SNMP de uma virtual-system dedicada só a
// BGP; se vier vazio aqui não é necessariamente um bug (confirmar contra o equipamento BNG).

// RadiusServerReport estatísticas agregadas (Authen + Acct) por servidor RADIUS.
type RadiusServerReport struct {
	ServerIP           string `json:"server_ip"`
	AuthenRequests     string `json:"authen_requests,omitempty"`
	AuthenAccepts      string `json:"authen_accepts,omitempty"`
	AuthenRejects      string `json:"authen_rejects,omitempty"`
	AuthenTimeouts     string `json:"authen_timeouts,omitempty"`
	AuthenNoResponse   string `json:"authen_server_not_reply,omitempty"`
	AcctRequests       string `json:"acct_requests,omitempty"`
	AcctResponses      string `json:"acct_responses,omitempty"`
	AcctTimeouts       string `json:"acct_timeouts,omitempty"`
	AcctNoResponse     string `json:"acct_server_not_reply,omitempty"`
}

func pivotRadiusServers(fields map[string]storedField) []RadiusServerReport {
	m := map[string]*RadiusServerReport{}
	get := func(ip string) *RadiusServerReport {
		if r, ok := m[ip]; ok {
			return r
		}
		r := &RadiusServerReport{ServerIP: ip}
		m[ip] = r
		return r
	}
	assign := func(key string, set func(r *RadiusServerReport, v string)) {
		f, ok := fields[key]
		if !ok || !f.OK {
			return
		}
		for _, v := range walkVars(f) {
			idx := indexSuffix(v.OID, f.OID)
			if idx == "" {
				continue
			}
			ip := lastIPFromSuffix(idx)
			if ip == "" {
				continue
			}
			set(get(ip), v.Value)
		}
	}
	assign("radius_authen_requests", func(r *RadiusServerReport, v string) { r.AuthenRequests = v })
	assign("radius_authen_accepts", func(r *RadiusServerReport, v string) { r.AuthenAccepts = v })
	assign("radius_authen_rejects", func(r *RadiusServerReport, v string) { r.AuthenRejects = v })
	assign("radius_authen_timeouts", func(r *RadiusServerReport, v string) { r.AuthenTimeouts = v })
	assign("radius_authen_server_not_reply", func(r *RadiusServerReport, v string) { r.AuthenNoResponse = v })
	assign("radius_acct_requests", func(r *RadiusServerReport, v string) { r.AcctRequests = v })
	assign("radius_acct_responses", func(r *RadiusServerReport, v string) { r.AcctResponses = v })
	assign("radius_acct_timeouts", func(r *RadiusServerReport, v string) { r.AcctTimeouts = v })
	assign("radius_acct_server_not_reply", func(r *RadiusServerReport, v string) { r.AcctNoResponse = v })

	var out []RadiusServerReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ServerIP < out[j].ServerIP })
	return out
}

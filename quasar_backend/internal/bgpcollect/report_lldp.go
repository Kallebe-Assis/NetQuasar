package bgpcollect

import (
	"sort"
	"strings"
)

// report_lldp.go — pivot de vizinhos LLDP (LLDP-MIB padrão, lldpRemTable). Puramente
// informativo nesta tela — NÃO alimenta nem referencia a tela Topologia
// (topology_canvas/TopologyPage.tsx) de forma alguma; o utilizador confirmou explicitamente
// que quer desenhar a topologia à mão, sem descoberta automática.
//
// Índice real é composto de 3 partes: lldpRemTimeMark.lldpRemLocalPortNum.lldpRemIndex — uso o
// 2º token (LocalPortNum, que na prática corresponde ao ifIndex local) para rotular com o nome
// da interface já coletado; o restante (TimeMark + RemIndex) fica como identificador opaco da
// entrada, já que pode haver mais de um vizinho por porta local.

// LLDPNeighborReport uma linha de lldpRemTable.
type LLDPNeighborReport struct {
	LocalPortNum string `json:"local_port_num"`
	LocalIfName  string `json:"local_if_name,omitempty"`
	RemKey       string `json:"rem_key"` // TimeMark.RemIndex, opaco
	ChassisID    string `json:"chassis_id,omitempty"`
	PortID       string `json:"port_id,omitempty"`
	PortDesc     string `json:"port_desc,omitempty"`
	SysName      string `json:"sys_name,omitempty"`
	SysDesc      string `json:"sys_desc,omitempty"`
}

// lldpLocalPortAndKey extrai o LocalPortNum (2º token) do índice composto
// TimeMark.LocalPortNum.RemIndex, mantendo TimeMark+RemIndex como chave opaca da linha.
func lldpLocalPortAndKey(idx string) (localPort, remKey string) {
	parts := strings.Split(idx, ".")
	if len(parts) < 3 {
		return idx, ""
	}
	localPort = parts[1]
	remKey = parts[0] + "." + parts[2]
	return
}

func pivotLLDPNeighbors(fields map[string]storedField, ifaceNames map[string]string) []LLDPNeighborReport {
	m := map[string]*LLDPNeighborReport{}
	get := func(idx string) *LLDPNeighborReport {
		if r, ok := m[idx]; ok {
			return r
		}
		localPort, remKey := lldpLocalPortAndKey(idx)
		r := &LLDPNeighborReport{LocalPortNum: localPort, LocalIfName: ifaceNames[localPort], RemKey: remKey}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *LLDPNeighborReport, v string)) {
		f, ok := fields[key]
		if !ok || !f.OK {
			return
		}
		for _, v := range walkVars(f) {
			idx := indexSuffix(v.OID, f.OID)
			if idx == "" {
				continue
			}
			set(get(idx), v.Value)
		}
	}
	assign("lldp_rem_chassis_id", func(r *LLDPNeighborReport, v string) { r.ChassisID = v })
	assign("lldp_rem_port_id", func(r *LLDPNeighborReport, v string) { r.PortID = v })
	assign("lldp_rem_port_desc", func(r *LLDPNeighborReport, v string) { r.PortDesc = v })
	assign("lldp_rem_sys_name", func(r *LLDPNeighborReport, v string) { r.SysName = v })
	assign("lldp_rem_sys_desc", func(r *LLDPNeighborReport, v string) { r.SysDesc = v })

	var out []LLDPNeighborReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LocalPortNum != out[j].LocalPortNum {
			return out[i].LocalPortNum < out[j].LocalPortNum
		}
		return out[i].RemKey < out[j].RemKey
	})
	return out
}

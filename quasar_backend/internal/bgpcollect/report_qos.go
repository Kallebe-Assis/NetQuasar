package bgpcollect

import (
	"sort"
	"strings"
)

// report_qos.go — pivot de descarte por classe/fila de QoS (HUAWEI-HQOS-MIB e
// HUAWEI-CBQOS-MIB). Ambas as tabelas têm índice composto de 5 partes começando pelo ifIndex
// (hwhqosIfIndex.Direction.UserLayer1.UserLayer2.QueueIndex e
// hwCBQoSIfApplyPolicyIfIndex.Direction.Vlanid1.Vlanid2.ClassIndex) — extraio só o 1º token
// (ifIndex) para rotular com o nome da interface já coletado; o resto do sufixo fica como
// identificador opaco da fila/classe (não há um campo "nome de fila" simples nestas MIBs).

// QosQueueReport uma linha de hwhqosIfStatTable.
type QosQueueReport struct {
	IfIndex      string `json:"if_index"`
	IfName       string `json:"if_name,omitempty"`
	QueueKey     string `json:"queue_key"` // resto do índice (direção/camadas/fila), opaco
	ForwardBytes string `json:"forward_bytes,omitempty"`
	ForwardPkts  string `json:"forward_packets,omitempty"`
	DropBytes    string `json:"drop_bytes,omitempty"`
	DropPkts     string `json:"drop_packets,omitempty"`
}

// CarStatReport uma linha de hwCBQoSCarStatisticsTable.
type CarStatReport struct {
	IfIndex         string `json:"if_index"`
	IfName          string `json:"if_name,omitempty"`
	ClassKey        string `json:"class_key"` // resto do índice (direção/vlan/classe), opaco
	ConformedBytes  string `json:"conformed_bytes,omitempty"`
	ExceededBytes   string `json:"exceeded_bytes,omitempty"`
	DroppedBytes    string `json:"dropped_bytes,omitempty"`
}

// splitFirstToken separa o 1º token (ifIndex) do resto do índice composto.
func splitFirstToken(idx string) (first, rest string) {
	parts := strings.SplitN(idx, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return idx, ""
}

func pivotQosQueues(fields map[string]storedField, ifaceNames map[string]string) []QosQueueReport {
	m := map[string]*QosQueueReport{}
	get := func(idx string) *QosQueueReport {
		if r, ok := m[idx]; ok {
			return r
		}
		ifIndex, rest := splitFirstToken(idx)
		r := &QosQueueReport{IfIndex: ifIndex, IfName: ifaceNames[ifIndex], QueueKey: rest}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *QosQueueReport, v string)) {
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
	assign("hqos_queue_forward_bytes", func(r *QosQueueReport, v string) { r.ForwardBytes = v })
	assign("hqos_queue_forward_packets", func(r *QosQueueReport, v string) { r.ForwardPkts = v })
	assign("hqos_queue_drop_bytes", func(r *QosQueueReport, v string) { r.DropBytes = v })
	assign("hqos_queue_drop_packets", func(r *QosQueueReport, v string) { r.DropPkts = v })

	var out []QosQueueReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].QueueKey < out[j].QueueKey
	})
	return out
}

func pivotCarStats(fields map[string]storedField, ifaceNames map[string]string) []CarStatReport {
	m := map[string]*CarStatReport{}
	get := func(idx string) *CarStatReport {
		if r, ok := m[idx]; ok {
			return r
		}
		ifIndex, rest := splitFirstToken(idx)
		r := &CarStatReport{IfIndex: ifIndex, IfName: ifaceNames[ifIndex], ClassKey: rest}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *CarStatReport, v string)) {
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
	assign("cbqos_car_conformed_bytes", func(r *CarStatReport, v string) { r.ConformedBytes = v })
	assign("cbqos_car_exceeded_bytes", func(r *CarStatReport, v string) { r.ExceededBytes = v })
	assign("cbqos_car_dropped_bytes", func(r *CarStatReport, v string) { r.DroppedBytes = v })

	var out []CarStatReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].ClassKey < out[j].ClassKey
	})
	return out
}

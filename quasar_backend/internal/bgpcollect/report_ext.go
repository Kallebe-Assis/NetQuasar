package bgpcollect

import (
	"sort"
	"strconv"
	"strings"
)

// report_ext.go — pivot de Virtual System (VS), sessões BFD e E-Trunk (LAG entre
// equipamentos). Ver comentários no metrics.go/plano para os índices confirmados no MIB
// Reference (HUAWEI-VS-MIB, HUAWEI-BFD-MIB, HUAWEI-E-TRUNK-MIB).

// VSInfo uma linha de hwVSTable (índice hwVSVsId).
type VSInfo struct {
	VsID   string `json:"vs_id"`
	Name   string `json:"name,omitempty"`
	Status string `json:"status,omitempty"`
}

// VSResourceReport uma linha de hwVSPhysicalResTable (índice hwVSSlot — NÃO é o VS-ID,
// mantido como lista separada, o MIB não garante correlação 1:1 com hwVSTable).
type VSResourceReport struct {
	Slot        string `json:"slot"`
	Cpu         string `json:"cpu,omitempty"`
	MemUsed     string `json:"mem_used,omitempty"`
	MemTotal    string `json:"mem_total,omitempty"`
}

// BFDSessionReport uma linha de hwBfdSessionTable (índice hwBfdSessIndex).
type BFDSessionReport struct {
	SessIndex   string `json:"sess_index"`
	PeerAddr    string `json:"peer_addr,omitempty"`
	BindIfName  string `json:"bind_if_name,omitempty"`
	State       string `json:"state,omitempty"` // 0=admin down/1=down/2=init/3=up (numeração própria, ≠ BGP)
	StateLabel  string `json:"state_label,omitempty"`
	Diag        string `json:"diag,omitempty"`
	VPNName     string `json:"vpn_name,omitempty"`
	DownReason  string `json:"down_reason,omitempty"`
}

// ETrunkReport uma linha de hwETrunkTable (índice hwETrunkId).
type ETrunkReport struct {
	ETrunkID     string `json:"etrunk_id"`
	Status       string `json:"status,omitempty"` // initialize(1)/backup(2)/master(3)
	StatusLabel  string `json:"status_label,omitempty"`
	StatusReason string `json:"status_reason,omitempty"` // enum: pri/timeout/bfdDown/peerTimeout/peerBfdDown/allMemberDown/init/peerNodeDown
}

// ETrunkMemberReport uma linha de hwETrunkMemberTable (índice composto ParentId.Type.MemberId).
type ETrunkMemberReport struct {
	ParentID     string `json:"parent_id"`
	MemberID     string `json:"member_id"`
	Status       string `json:"status,omitempty"` // backup(1)/master(2)
	StatusLabel  string `json:"status_label,omitempty"`
	StatusReason string `json:"status_reason,omitempty"`
}

var bfdStateLabels = map[string]string{
	"0": "admin down",
	"1": "down",
	"2": "init",
	"3": "up",
}

var etrunkStatusLabels = map[string]string{
	"1": "initialize",
	"2": "backup",
	"3": "master",
}

var etrunkMemberStatusLabels = map[string]string{
	"1": "backup",
	"2": "master",
}

func pivotVSList(fields map[string]storedField) []VSInfo {
	m := map[string]*VSInfo{}
	get := func(idx string) *VSInfo {
		if r, ok := m[idx]; ok {
			return r
		}
		r := &VSInfo{VsID: idx}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *VSInfo, v string)) {
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
	assign("vs_name", func(r *VSInfo, v string) { r.Name = v })
	assign("vs_status", func(r *VSInfo, v string) { r.Status = v })

	var out []VSInfo
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VsID < out[j].VsID })
	return out
}

func pivotVSResources(fields map[string]storedField) []VSResourceReport {
	m := map[string]*VSResourceReport{}
	get := func(idx string) *VSResourceReport {
		if r, ok := m[idx]; ok {
			return r
		}
		r := &VSResourceReport{Slot: idx}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *VSResourceReport, v string)) {
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
	assign("vs_res_cpu", func(r *VSResourceReport, v string) { r.Cpu = v })
	assign("vs_res_mem_used", func(r *VSResourceReport, v string) { r.MemUsed = v })
	assign("vs_res_mem_total", func(r *VSResourceReport, v string) { r.MemTotal = v })

	var out []VSResourceReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot < out[j].Slot })
	return out
}

func pivotBFDSessions(fields map[string]storedField) []BFDSessionReport {
	m := map[string]*BFDSessionReport{}
	get := func(idx string) *BFDSessionReport {
		if r, ok := m[idx]; ok {
			return r
		}
		r := &BFDSessionReport{SessIndex: idx}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *BFDSessionReport, v string)) {
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
	assign("bfd_peer_addr", func(r *BFDSessionReport, v string) { r.PeerAddr = v })
	assign("bfd_bind_if_name", func(r *BFDSessionReport, v string) { r.BindIfName = v })
	assign("bfd_state", func(r *BFDSessionReport, v string) { r.State = v; r.StateLabel = bfdStateLabels[v] })
	assign("bfd_diag", func(r *BFDSessionReport, v string) { r.Diag = v })
	assign("bfd_vpn_name", func(r *BFDSessionReport, v string) { r.VPNName = v })
	assign("bfd_down_reason", func(r *BFDSessionReport, v string) { r.DownReason = v })

	var out []BFDSessionReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		ni, _ := strconv.Atoi(out[i].SessIndex)
		nj, _ := strconv.Atoi(out[j].SessIndex)
		return ni < nj
	})
	return out
}

func pivotETrunks(fields map[string]storedField) []ETrunkReport {
	m := map[string]*ETrunkReport{}
	get := func(idx string) *ETrunkReport {
		if r, ok := m[idx]; ok {
			return r
		}
		r := &ETrunkReport{ETrunkID: idx}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *ETrunkReport, v string)) {
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
	assign("etrunk_status", func(r *ETrunkReport, v string) { r.Status = v; r.StatusLabel = etrunkStatusLabels[v] })
	assign("etrunk_status_reason", func(r *ETrunkReport, v string) { r.StatusReason = v })

	var out []ETrunkReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		ni, _ := strconv.Atoi(out[i].ETrunkID)
		nj, _ := strconv.Atoi(out[j].ETrunkID)
		return ni < nj
	})
	return out
}

// etrunkMemberParentAndID extrai ParentId (1º token) e MemberId (3º token) do índice composto
// hwETrunkMemberParentId.hwETrunkMemberType.hwETrunkMemberId — o tipo (2º token) é descartado,
// não é necessário para exibição.
func etrunkMemberParentAndID(idx string) (parent, member string) {
	parts := strings.Split(idx, ".")
	if len(parts) < 3 {
		return idx, ""
	}
	return parts[0], parts[2]
}

func pivotETrunkMembers(fields map[string]storedField) []ETrunkMemberReport {
	m := map[string]*ETrunkMemberReport{}
	get := func(idx string) *ETrunkMemberReport {
		if r, ok := m[idx]; ok {
			return r
		}
		parent, member := etrunkMemberParentAndID(idx)
		r := &ETrunkMemberReport{ParentID: parent, MemberID: member}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *ETrunkMemberReport, v string)) {
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
	assign("etrunk_member_status", func(r *ETrunkMemberReport, v string) {
		r.Status = v
		r.StatusLabel = etrunkMemberStatusLabels[v]
	})
	assign("etrunk_member_status_reason", func(r *ETrunkMemberReport, v string) { r.StatusReason = v })

	var out []ETrunkMemberReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ParentID != out[j].ParentID {
			return out[i].ParentID < out[j].ParentID
		}
		return out[i].MemberID < out[j].MemberID
	})
	return out
}

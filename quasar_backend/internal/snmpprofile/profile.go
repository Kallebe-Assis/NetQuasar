package snmpprofile

import (
	"strconv"
	"strings"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// CollectProfile OIDs escolhidos para métricas “caixa” (painel) e referência para relatórios.
type CollectProfile struct {
	UptimeOID   string   `json:"uptime_oid,omitempty"`
	SysNameOID  string   `json:"sysname_oid,omitempty"`
	SysDescrOID string   `json:"sysdescr_oid,omitempty"`
	CPUPrimaryOID   string `json:"cpu_primary_oid,omitempty"`
	CPUAvailableOID string `json:"cpu_available_oid,omitempty"`
	MemoryUsedOID string `json:"memory_used_oid,omitempty"`
	MemorySizeOID string `json:"memory_size_oid,omitempty"`
	TempPrimaryOID string `json:"temp_primary_oid,omitempty"`
	CPUOIDs     []string `json:"cpu_oids,omitempty"`
	MemoryOIDs  []string `json:"memory_oids,omitempty"`
	TempOIDs    []string `json:"temp_oids,omitempty"`
	// ExtraOIDLabels mapeia OID (sem ponto inicial) → descrição definida em Configurações para relatórios.
	ExtraOIDLabels map[string]string `json:"extra_oid_labels,omitempty"`
}

type CandidateDecision struct {
	OID      string `json:"oid"`
	Metric   string `json:"metric"`
	Decision string `json:"decision"` // accepted | rejected
	Reason   string `json:"reason"`
}

type DiscoveryDebug struct {
	Selected   map[string]string  `json:"selected"`
	Candidates []CandidateDecision `json:"candidates"`
}

// BuildCollectProfile analisa linhas do walk e escolhe OIDs conhecidos (HOST-RESOURCES / SNMPv2-MIB).
func BuildCollectProfile(rows []probing.SNMPVar) CollectProfile {
	p, _ := BuildCollectProfileWithDebug(rows)
	return p
}

// BuildCollectProfileWithDebug devolve perfil e rastreio de decisões para troubleshooting.
func BuildCollectProfileWithDebug(rows []probing.SNMPVar) (CollectProfile, DiscoveryDebug) {
	var p CollectProfile
	dbg := DiscoveryDebug{
		Selected:   map[string]string{},
		Candidates: []CandidateDecision{},
	}
	add := func(oid, metric, decision, reason string) {
		dbg.Candidates = append(dbg.Candidates, CandidateDecision{
			OID: strings.TrimSpace(oid), Metric: metric, Decision: decision, Reason: reason,
		})
	}
	byOID := make(map[string]string, len(rows))
	for _, r := range rows {
		oid := cleanOID(r.OID)
		if oid == "" {
			continue
		}
		byOID[oid] = r.Value
	}

	if _, ok := byOID["1.3.6.1.2.1.1.3.0"]; ok {
		p.UptimeOID = "1.3.6.1.2.1.1.3.0"
	}
	if _, ok := byOID["1.3.6.1.2.1.1.5.0"]; ok {
		p.SysNameOID = "1.3.6.1.2.1.1.5.0"
	}
	if _, ok := byOID["1.3.6.1.2.1.1.1.0"]; ok {
		p.SysDescrOID = "1.3.6.1.2.1.1.1.0"
	}

	const hrProcLoad = "1.3.6.1.2.1.25.3.3.1.2"
	const cpuMikrotik = "1.3.6.1.4.1.14988.1.1.3.10.0"
	const cpuUcdIdle = "1.3.6.1.4.1.2021.11.11.0"
	for oid := range byOID {
		if strings.HasPrefix(oid, hrProcLoad+".") || oid == hrProcLoad {
			v := strings.TrimSpace(byOID[oid])
			if isSNMPMissingValue(v) {
				add(oid, "cpu", "rejected", "OID hrProcessorLoad sem valor válido")
				continue
			}
			if f, err := strconv.ParseFloat(v, 64); err != nil || f < 0 || f > 10000 {
				add(oid, "cpu", "rejected", "valor CPU fora de faixa esperada")
				continue
			}
			p.CPUOIDs = appendUnique(p.CPUOIDs, oid)
			add(oid, "cpu", "accepted", "prefixo hrProcessorLoad detectado")
		}
	}
	if _, ok := byOID[cpuMikrotik]; ok {
		v := strings.TrimSpace(byOID[cpuMikrotik])
		if isSNMPMissingValue(v) {
			add(cpuMikrotik, "cpu", "rejected", "OID CPU MikroTik sem valor válido")
		} else {
			p.CPUOIDs = appendUnique(p.CPUOIDs, cpuMikrotik)
			add(cpuMikrotik, "cpu", "accepted", "OID CPU MikroTik conhecido")
		}
	}
	if _, ok := byOID[cpuUcdIdle]; ok {
		v := strings.TrimSpace(byOID[cpuUcdIdle])
		if isSNMPMissingValue(v) {
			add(cpuUcdIdle, "cpu", "rejected", "OID UCD CPU idle sem valor válido")
		} else {
			p.CPUOIDs = appendUnique(p.CPUOIDs, cpuUcdIdle)
			add(cpuUcdIdle, "cpu", "accepted", "OID UCD CPU idle conhecido")
		}
	}
	if len(p.CPUOIDs) > 0 {
		p.CPUPrimaryOID = p.CPUOIDs[0]
		dbg.Selected["cpu_primary_oid"] = p.CPUPrimaryOID
	} else {
		add("", "cpu", "rejected", "nenhum OID de CPU encontrado")
	}

	// Memória: hrStorage (Physical memory / RAM)
	const storDescr = "1.3.6.1.2.1.25.2.3.1.3."
	const storType = "1.3.6.1.2.1.25.2.3.1.2."
	const storUsed = "1.3.6.1.2.1.25.2.3.1.6."
	const storSize = "1.3.6.1.2.1.25.2.3.1.5."
	const storRAMTypeOID = "1.3.6.1.2.1.25.2.1.2"
	sizeByIdx := map[string]float64{}
	usedByIdx := map[string]float64{}
	for oid, val := range byOID {
		if strings.HasPrefix(oid, storSize) {
			idx := strings.TrimPrefix(oid, storSize)
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f > 0 {
				sizeByIdx[idx] = f
			}
			continue
		}
		if strings.HasPrefix(oid, storUsed) {
			idx := strings.TrimPrefix(oid, storUsed)
			if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f >= 0 {
				usedByIdx[idx] = f
			}
		}
	}
	for oid, val := range byOID {
		if !strings.HasPrefix(oid, storDescr) {
			continue
		}
		lv := strings.ToLower(val)
		if strings.Contains(lv, "physical memory") || strings.Contains(lv, "ram") || strings.Contains(lv, "real memory") {
			suf := strings.TrimPrefix(oid, storDescr)
			if suf != "" {
				p.MemoryOIDs = appendUnique(p.MemoryOIDs, storUsed+suf)
				p.MemoryOIDs = appendUnique(p.MemoryOIDs, storSize+suf)
				add(storUsed+suf, "memory", "accepted", "descrição de memória RAM detectada")
				add(storSize+suf, "memory", "accepted", "descrição de memória RAM detectada")
				if p.MemoryUsedOID == "" {
					p.MemoryUsedOID = storUsed + suf
				}
				if p.MemorySizeOID == "" {
					p.MemorySizeOID = storSize + suf
				}
			}
		}
	}
	// Heurística genérica: escolhe o maior bloco coerente com par used/size.
	if p.MemoryUsedOID == "" || p.MemorySizeOID == "" {
		bestIdx := ""
		bestSize := -1.0
		for idx, sz := range sizeByIdx {
			used, ok := usedByIdx[idx]
			if !ok {
				continue
			}
			if used < 0 || used > sz || sz <= 0 {
				continue
			}
			if sz > bestSize {
				bestSize = sz
				bestIdx = idx
			}
		}
		if bestIdx != "" {
			p.MemoryUsedOID = storUsed + bestIdx
			p.MemorySizeOID = storSize + bestIdx
			p.MemoryOIDs = appendUnique(p.MemoryOIDs, p.MemoryUsedOID)
			p.MemoryOIDs = appendUnique(p.MemoryOIDs, p.MemorySizeOID)
			add(p.MemoryUsedOID, "memory", "accepted", "heurística genérica: par used/size por maior bloco válido")
			add(p.MemorySizeOID, "memory", "accepted", "heurística genérica: par used/size por maior bloco válido")
		}
	}
	if p.MemoryUsedOID == "" || p.MemorySizeOID == "" {
		for oid, val := range byOID {
			if !strings.HasPrefix(oid, storType) {
				continue
			}
			if strings.TrimSpace(val) != storRAMTypeOID {
				add(oid, "memory", "rejected", "storage type não é RAM")
				continue
			}
			suf := strings.TrimPrefix(oid, storType)
			if suf != "" {
				p.MemoryOIDs = appendUnique(p.MemoryOIDs, storUsed+suf)
				p.MemoryOIDs = appendUnique(p.MemoryOIDs, storSize+suf)
				add(storUsed+suf, "memory", "accepted", "storageType indica RAM")
				add(storSize+suf, "memory", "accepted", "storageType indica RAM")
				p.MemoryUsedOID = storUsed + suf
				p.MemorySizeOID = storSize + suf
				break
			}
		}
	}
	if len(p.MemoryOIDs) == 0 {
		if _, ok := byOID["1.3.6.1.2.1.25.2.2.0"]; ok {
			p.MemoryOIDs = appendUnique(p.MemoryOIDs, "1.3.6.1.2.1.25.2.2.0")
			if p.MemorySizeOID == "" {
				p.MemorySizeOID = "1.3.6.1.2.1.25.2.2.0"
			}
			add("1.3.6.1.2.1.25.2.2.0", "memory", "accepted", "fallback hrMemorySize (capacidade total)")
		}
	}
	if _, ok := byOID["1.3.6.1.4.1.2021.4.5.0"]; ok {
		v := strings.TrimSpace(byOID["1.3.6.1.4.1.2021.4.5.0"])
		if isSNMPMissingValue(v) {
			add("1.3.6.1.4.1.2021.4.5.0", "memory", "rejected", "UCD memTotalReal inválido")
		} else {
			p.MemoryOIDs = appendUnique(p.MemoryOIDs, "1.3.6.1.4.1.2021.4.5.0")
			if p.MemorySizeOID == "" {
				p.MemorySizeOID = "1.3.6.1.4.1.2021.4.5.0"
			}
			add("1.3.6.1.4.1.2021.4.5.0", "memory", "accepted", "UCD memTotalReal detectado")
		}
	}
	if _, ok := byOID["1.3.6.1.4.1.2021.4.6.0"]; ok {
		v := strings.TrimSpace(byOID["1.3.6.1.4.1.2021.4.6.0"])
		if isSNMPMissingValue(v) {
			add("1.3.6.1.4.1.2021.4.6.0", "memory", "rejected", "UCD memAvailReal inválido")
		} else {
			p.MemoryOIDs = appendUnique(p.MemoryOIDs, "1.3.6.1.4.1.2021.4.6.0")
			// Quando há total+avail do UCD, preferimos esse par para cálculo confiável.
			p.MemoryUsedOID = "1.3.6.1.4.1.2021.4.6.0"
			if p.MemorySizeOID == "" {
				p.MemorySizeOID = "1.3.6.1.4.1.2021.4.5.0"
			}
			add("1.3.6.1.4.1.2021.4.6.0", "memory", "accepted", "UCD memAvailReal detectado")
		}
	}
	if p.MemoryUsedOID == "" && p.MemorySizeOID == "" {
		add("", "memory", "rejected", "nenhum par used/size identificado")
	} else {
		if p.MemoryUsedOID != "" {
			dbg.Selected["memory_used_oid"] = p.MemoryUsedOID
		}
		if p.MemorySizeOID != "" {
			dbg.Selected["memory_size_oid"] = p.MemorySizeOID
		}
	}

	// Temperatura: prioriza OIDs padrão de sensores (ENTITY-SENSOR-MIB / CISCO-ENVMON),
	// com fallback para valores com texto de unidade.
	const entPhySensorValue = "1.3.6.1.2.1.99.1.1.1.4"
	const ciscoEnvMonTemp = "1.3.6.1.4.1.9.9.13.1.3.1.3"
	const mikrotikTemp = "1.3.6.1.4.1.14988.1.1.3.14.0"
	for oid := range byOID {
		if strings.HasPrefix(oid, entPhySensorValue+".") || strings.HasPrefix(oid, ciscoEnvMonTemp+".") {
			v := strings.TrimSpace(byOID[oid])
			if isSNMPMissingValue(v) {
				add(oid, "temperature", "rejected", "OID de temperatura sem objeto válido")
				continue
			}
			p.TempOIDs = appendUnique(p.TempOIDs, oid)
			add(oid, "temperature", "accepted", "prefixo padrão de sensor/temperatura")
		}
	}
	if _, ok := byOID[mikrotikTemp]; ok {
		v := strings.TrimSpace(byOID[mikrotikTemp])
		if isSNMPMissingValue(v) {
			add(mikrotikTemp, "temperature", "rejected", "OID MikroTik temperatura retornou inválido")
		} else {
			p.TempOIDs = appendUnique(p.TempOIDs, mikrotikTemp)
			add(mikrotikTemp, "temperature", "accepted", "OID temperatura MikroTik conhecido")
		}
	}
	for oid, val := range byOID {
		lv := strings.ToLower(val)
		if strings.Contains(lv, "celsius") || strings.Contains(lv, "°c") || strings.Contains(lv, "degrees c") {
			p.TempOIDs = appendUnique(p.TempOIDs, oid)
			add(oid, "temperature", "accepted", "valor textual indica Celsius")
			if len(p.TempOIDs) >= 3 {
				break
			}
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f >= 10 && f <= 95 {
			if strings.HasPrefix(oid, entPhySensorValue+".") || strings.HasPrefix(oid, ciscoEnvMonTemp+".") {
				p.TempOIDs = appendUnique(p.TempOIDs, oid)
				add(oid, "temperature", "accepted", "faixa típica de temperatura com OID conhecido")
			}
		}
	}
	if len(p.TempOIDs) > 0 {
		p.TempPrimaryOID = p.TempOIDs[0]
		dbg.Selected["temp_primary_oid"] = p.TempPrimaryOID
	} else {
		add("", "temperature", "rejected", "nenhum OID de temperatura confiável encontrado")
	}
	if p.UptimeOID != "" {
		dbg.Selected["uptime_oid"] = p.UptimeOID
	}

	return p, dbg
}

func appendUnique(list []string, v string) []string {
	v = cleanOID(v)
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func cleanOID(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimLeft(v, ".")
	return v
}

func isSNMPMissingValue(v string) bool {
	lv := strings.ToLower(strings.TrimSpace(v))
	return lv == "" || lv == "<nil>" || strings.Contains(lv, "nosuchobject") || strings.Contains(lv, "nosuchinstance")
}

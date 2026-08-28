package bgpcollect

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// report_hardware.go — pivot de óptica por porta, CPU por núcleo e saúde física do chassi
// (ventoinhas/fontes/temperatura/tensão/luz de alarme por placa). Todas as tabelas
// HUAWEI-ENTITY-EXTENT-MIB/HUAWEI-CPU-MIB — ver comentários no metrics.go para os índices
// confirmados no MIB Reference.

type OpticsReport struct {
	PhysicalIndex string `json:"physical_index"`
	PortLabel     string `json:"port_label,omitempty"`
	RxPower       string `json:"rx_power,omitempty"`
	TxPower       string `json:"tx_power,omitempty"`
	Temperature   string `json:"temperature,omitempty"`
	Voltage       string `json:"voltage,omitempty"`
	BiasCurrent   string `json:"bias_current,omitempty"`
}

type CpuCoreReport struct {
	CoreIndex   string `json:"core_index"`
	Duty        string `json:"duty,omitempty"`
	AvgDuty1Min string `json:"avg_duty_1min,omitempty"`
	AvgDuty5Min string `json:"avg_duty_5min,omitempty"`
}

type FanReport struct {
	Slot        string `json:"slot"`
	Sn          string `json:"sn"`
	Speed       string `json:"speed,omitempty"`
	Present     string `json:"present,omitempty"` // present(1)/absent(2)
	State       string `json:"state,omitempty"`   // normal(1)/abnormal(2)
	StateLabel  string `json:"state_label,omitempty"`
}

type PowerSupplyReport struct {
	Slot       string `json:"slot"`
	Sn         string `json:"sn"`
	Present    string `json:"present,omitempty"`
	State      string `json:"state,omitempty"` // supply(1)/notSupply(2)/sleep(3)/unknown(4)
	StateLabel string `json:"state_label,omitempty"`
	Current    string `json:"current,omitempty"` // mA
	Voltage    string `json:"voltage,omitempty"` // mV
}

type TemperatureReport struct {
	SlotRaw     string `json:"slot_raw"`
	Chassis     int    `json:"chassis,omitempty"`
	Slot        int    `json:"slot,omitempty"`
	I2C         string `json:"i2c"`
	Value       string `json:"value,omitempty"`
	Status      string `json:"status,omitempty"` // normal(1)/minor(2)/major(3)/fatal(4)
	StatusLabel string `json:"status_label,omitempty"`
}

type VoltageReport struct {
	SlotRaw     string `json:"slot_raw"`
	Chassis     int    `json:"chassis,omitempty"`
	Slot        int    `json:"slot,omitempty"`
	I2C         string `json:"i2c"`
	Value       string `json:"value,omitempty"`
	Status      string `json:"status,omitempty"` // abnormal(0)/normal(1)/major(2)/fatal(3)
	StatusLabel string `json:"status_label,omitempty"`
}

// BoardAlarmReport — "semáforo" por placa a partir de hwEntityAlarmLight (BITS).
type BoardAlarmReport struct {
	PhysicalIndex string `json:"physical_index"`
	Name          string `json:"name,omitempty"`
	Raw           string `json:"raw,omitempty"`
	Severity      string `json:"severity"` // normal | aviso | grave | desconhecido
}

// pivotEntityNames faz WALK de entPhysicalName (ENTITY-MIB) — usado para rotular óptica e
// luz de alarme pelo nome da porta/placa em vez de só o índice físico cru.
func pivotEntityNames(fields map[string]storedField) map[string]string {
	out := map[string]string{}
	f, ok := fields["ent_physical_name"]
	if !ok || !f.OK {
		return out
	}
	for _, v := range walkVars(f) {
		idx := indexSuffix(v.OID, f.OID)
		if idx == "" || v.Value == "" {
			continue
		}
		out[idx] = v.Value
	}
	return out
}

func pivotOptics(fields map[string]storedField, names map[string]string) []OpticsReport {
	m := map[string]*OpticsReport{}
	get := func(idx string) *OpticsReport {
		if r, ok := m[idx]; ok {
			return r
		}
		r := &OpticsReport{PhysicalIndex: idx, PortLabel: names[idx]}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *OpticsReport, v string)) {
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
	assign("optical_rx_power", func(r *OpticsReport, v string) { r.RxPower = v })
	assign("optical_tx_power", func(r *OpticsReport, v string) { r.TxPower = v })
	assign("optical_temperature", func(r *OpticsReport, v string) { r.Temperature = v })
	assign("optical_voltage", func(r *OpticsReport, v string) { r.Voltage = v })
	assign("optical_bias_current", func(r *OpticsReport, v string) { r.BiasCurrent = v })

	var out []OpticsReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PhysicalIndex < out[j].PhysicalIndex })
	return out
}

func pivotCpuCores(fields map[string]storedField) []CpuCoreReport {
	m := map[string]*CpuCoreReport{}
	get := func(idx string) *CpuCoreReport {
		if r, ok := m[idx]; ok {
			return r
		}
		r := &CpuCoreReport{CoreIndex: idx}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *CpuCoreReport, v string)) {
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
	assign("cpu_core_duty", func(r *CpuCoreReport, v string) { r.Duty = v })
	assign("cpu_core_avg_1min", func(r *CpuCoreReport, v string) { r.AvgDuty1Min = v })
	assign("cpu_core_avg_5min", func(r *CpuCoreReport, v string) { r.AvgDuty5Min = v })

	var out []CpuCoreReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		ni, _ := strconv.Atoi(out[i].CoreIndex)
		nj, _ := strconv.Atoi(out[j].CoreIndex)
		return ni < nj
	})
	return out
}

var fanPwrStateLabels = map[string]string{"1": "normal", "2": "grave"}
var pwrStateLabels = map[string]string{"1": "normal", "2": "grave", "3": "normal", "4": "aviso"}
var tempStatusLabels = map[string]string{"1": "normal", "2": "grave", "3": "grave", "4": "fatal"}
var voltStatusLabels = map[string]string{"0": "grave", "1": "normal", "2": "grave", "3": "fatal"}

func pivotFans(fields map[string]storedField) []FanReport {
	m := map[string]*FanReport{}
	get := func(idx string) *FanReport {
		if r, ok := m[idx]; ok {
			return r
		}
		slot, sn := splitTwo(idx)
		r := &FanReport{Slot: slot, Sn: sn}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *FanReport, v string)) {
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
	assign("fan_speed", func(r *FanReport, v string) { r.Speed = v })
	assign("fan_present", func(r *FanReport, v string) { r.Present = v })
	assign("fan_state", func(r *FanReport, v string) { r.State = v; r.StateLabel = fanPwrStateLabels[v] })

	var out []FanReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot+"/"+out[i].Sn < out[j].Slot+"/"+out[j].Sn })
	return out
}

func pivotPowerSupplies(fields map[string]storedField) []PowerSupplyReport {
	m := map[string]*PowerSupplyReport{}
	get := func(idx string) *PowerSupplyReport {
		if r, ok := m[idx]; ok {
			return r
		}
		slot, sn := splitTwo(idx)
		r := &PowerSupplyReport{Slot: slot, Sn: sn}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *PowerSupplyReport, v string)) {
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
	assign("power_present", func(r *PowerSupplyReport, v string) { r.Present = v })
	assign("power_state", func(r *PowerSupplyReport, v string) { r.State = v; r.StateLabel = pwrStateLabels[v] })
	assign("power_current", func(r *PowerSupplyReport, v string) { r.Current = v })
	assign("power_voltage", func(r *PowerSupplyReport, v string) { r.Voltage = v })

	var out []PowerSupplyReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slot+"/"+out[i].Sn < out[j].Slot+"/"+out[j].Sn })
	return out
}

// decodeEntitySlot descodifica o slot codificado usado por hwTemperatureThresholdTable/
// hwVoltageInfoTable — algoritmo documentado no próprio MIB Reference: convertido para hex,
// o 1º dígito é o número do chassi e os 2 seguintes são o slot em hex (ex.: 18022399 → hex
// 112FFFF → chassi 1, slot 0x12=18).
func decodeEntitySlot(raw string) (chassis, slot int, ok bool) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n <= 0 {
		return 0, 0, false
	}
	hex := fmt.Sprintf("%X", n)
	if len(hex) < 3 {
		return 0, 0, false
	}
	c, err1 := strconv.ParseInt(hex[0:1], 16, 64)
	s, err2 := strconv.ParseInt(hex[1:3], 16, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return int(c), int(s), true
}

func pivotTemperatures(fields map[string]storedField) []TemperatureReport {
	m := map[string]*TemperatureReport{}
	get := func(idx string) *TemperatureReport {
		if r, ok := m[idx]; ok {
			return r
		}
		slotRaw, i2c := splitTwo(idx)
		r := &TemperatureReport{SlotRaw: slotRaw, I2C: i2c}
		if c, s, ok := decodeEntitySlot(slotRaw); ok {
			r.Chassis, r.Slot = c, s
		}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *TemperatureReport, v string)) {
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
	assign("temp_value", func(r *TemperatureReport, v string) { r.Value = v })
	assign("temp_status", func(r *TemperatureReport, v string) { r.Status = v; r.StatusLabel = tempStatusLabels[v] })

	var out []TemperatureReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotRaw+"/"+out[i].I2C < out[j].SlotRaw+"/"+out[j].I2C })
	return out
}

func pivotVoltages(fields map[string]storedField) []VoltageReport {
	m := map[string]*VoltageReport{}
	get := func(idx string) *VoltageReport {
		if r, ok := m[idx]; ok {
			return r
		}
		slotRaw, i2c := splitTwo(idx)
		r := &VoltageReport{SlotRaw: slotRaw, I2C: i2c}
		if c, s, ok := decodeEntitySlot(slotRaw); ok {
			r.Chassis, r.Slot = c, s
		}
		m[idx] = r
		return r
	}
	assign := func(key string, set func(r *VoltageReport, v string)) {
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
	assign("volt_value", func(r *VoltageReport, v string) { r.Value = v })
	assign("volt_status", func(r *VoltageReport, v string) { r.Status = v; r.StatusLabel = voltStatusLabels[v] })

	var out []VoltageReport
	for _, r := range m {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SlotRaw+"/"+out[i].I2C < out[j].SlotRaw+"/"+out[j].I2C })
	return out
}

// decodeAlarmLightSeverity interpreta hwEntityAlarmLight (BITS: notSupported/underRepair/
// critical/major/minor/alarmOutstanding/warning/indeterminate). O probing devolve BITS como
// octetos em hex separados por ":" (ex.: "20") quando não é texto imprimível — decodificamos o
// byte e verificamos os bits críticos/major/minor/aviso pela numeração BITS padrão (bit 0 =
// MSB do 1º octeto). Precisa de confirmação ao vivo — se o equipamento devolver noutro
// formato, o valor bruto continua disponível em BoardAlarmReport.Raw para ajuste.
func decodeAlarmLightSeverity(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "normal"
	}
	hexStr := strings.ReplaceAll(raw, ":", "")
	b, err := strconv.ParseUint(hexStr, 16, 8)
	if err != nil {
		return "desconhecido"
	}
	if b == 0 {
		return "normal"
	}
	// bit 2 = critical, bit 3 = major → 0x20 / 0x10 (bit0=MSB=0x80)
	if b&0x20 != 0 || b&0x10 != 0 {
		return "grave"
	}
	// bit 4 = minor, bit 6 = warning → 0x08 / 0x02
	if b&0x08 != 0 || b&0x02 != 0 {
		return "aviso"
	}
	return "normal"
}

func pivotBoardAlarms(fields map[string]storedField, names map[string]string) []BoardAlarmReport {
	f, ok := fields["entity_alarm_light"]
	if !ok || !f.OK {
		return nil
	}
	var out []BoardAlarmReport
	for _, v := range walkVars(f) {
		idx := indexSuffix(v.OID, f.OID)
		if idx == "" {
			continue
		}
		out = append(out, BoardAlarmReport{
			PhysicalIndex: idx,
			Name:          names[idx],
			Raw:           v.Value,
			Severity:      decodeAlarmLightSeverity(v.Value),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PhysicalIndex < out[j].PhysicalIndex })
	return out
}

// splitTwo separa um índice composto de 2 partes ("slot.sn") — usado por fans/PSUs/
// temperatura/tensão. Se houver mais de 2 tokens (não deveria, mas por segurança), o resto
// fica anexado ao 2º campo.
func splitTwo(idx string) (string, string) {
	parts := strings.SplitN(idx, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return idx, ""
}

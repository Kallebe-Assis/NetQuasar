package oltcollect

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

// OIDs ZTE Access Node (zxAnVlanTable / service-port user VID).
const (
	// ZteVlanTableOIDBase raiz da tabela zxAnVlanTable (nome + descrição num walk).
	ZteVlanTableOIDBase = "1.3.6.1.4.1.3902.1082.40.50.2.1.2"
	// ZteVlanDescOIDBase zxAnVlanDesc — índice final = VID.
	ZteVlanDescOIDBase = "1.3.6.1.4.1.3902.1082.40.50.2.1.2.1.3"
	// ZteVlanNameOIDBase zxAnVlanName — …2.1.2.1.2.{vid}
	ZteVlanNameOIDBase = "1.3.6.1.4.1.3902.1082.40.50.2.1.2.1.2"
	// ZteSrvPortUserVidOIDBase zxAnSrvPortUserVid — VLAN por service-port/ONU.
	ZteSrvPortUserVidOIDBase = "1.3.6.1.4.1.3902.1082.110.5.2.2.1.8"
)

var (
	pppoePonDescRE = regexp.MustCompile(`(?i)^PPPOE[-_]?PON\s*0*(\d+)\s*$`)
	vlanNameNumRE  = regexp.MustCompile(`(?i)^VLAN0*(\d+)$`)
)

// AuthorizeVlanCatalogEntry entrada guardada no perfil OLT (descoberta SNMP).
type AuthorizeVlanCatalogEntry struct {
	VID         int    `json:"vid"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Pon         int    `json:"pon,omitempty"`
	Ignored     bool   `json:"ignored"`
}

// ZteVlanEntry entrada bruta do catálogo SNMP.
type ZteVlanEntry struct {
	VID         int
	Name        string
	Description string
}

// EffectiveAuthorizeVlanSnmpOID OID a usar na descoberta (perfil ou padrão tabela ZTE).
func EffectiveAuthorizeVlanSnmpOID(cfg OnuReportConfig) string {
	if o := strings.TrimSpace(cfg.AuthorizeVlanSnmpOID); o != "" {
		return probing.NormalizeSNMPOID(o)
	}
	return ZteVlanTableOIDBase
}

// DiscoverZteVlanCatalogFromSNMPVars interpreta walk (tabela, nome ou descrição) e devolve
// todas as VLANs ordenadas por VID, com Pon inferido e Ignored sugerido.
func DiscoverZteVlanCatalogFromSNMPVars(vars []probing.SNMPVar) []AuthorizeVlanCatalogEntry {
	byVID := map[int]*AuthorizeVlanCatalogEntry{}
	ensure := func(vid int) *AuthorizeVlanCatalogEntry {
		if e, ok := byVID[vid]; ok {
			return e
		}
		e := &AuthorizeVlanCatalogEntry{VID: vid}
		byVID[vid] = e
		return e
	}
	for _, v := range vars {
		oid := probing.NormalizeSNMPOID(v.OID)
		vid, kind := classifyZteVlanOID(oid)
		if vid <= 0 {
			continue
		}
		val := strings.TrimSpace(v.Value)
		e := ensure(vid)
		switch kind {
		case "name":
			e.Name = val
		case "desc":
			e.Description = val
		default:
			if pppoePonDescRE.MatchString(val) || strings.EqualFold(val, "GERENCIA") || strings.EqualFold(val, "MANAGEMENT") {
				e.Description = val
			} else if vlanNameNumRE.MatchString(val) {
				e.Name = val
			} else if val != "" && e.Description == "" && e.Name == "" {
				e.Description = val
			}
		}
	}
	out := make([]AuthorizeVlanCatalogEntry, 0, len(byVID))
	for _, e := range byVID {
		e.Pon = PonFromZteVlanDescription(e.Description)
		e.Ignored = SuggestIgnoreZteVlan(*e)
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].VID < out[j].VID })
	return out
}

// MergeAuthorizeVlanCatalog preserva flags "ignored" do perfil ao rediscobrir.
func MergeAuthorizeVlanCatalog(discovered, previous []AuthorizeVlanCatalogEntry) []AuthorizeVlanCatalogEntry {
	prevIgn := map[int]bool{}
	for _, p := range previous {
		prevIgn[p.VID] = p.Ignored
	}
	out := make([]AuthorizeVlanCatalogEntry, len(discovered))
	copy(out, discovered)
	for i := range out {
		if _, ok := prevIgn[out[i].VID]; ok {
			out[i].Ignored = prevIgn[out[i].VID]
		}
	}
	return out
}

// SuggestIgnoreZteVlan marque VLAN 1, gestão e entradas sem PPPOE-PONxx.
func SuggestIgnoreZteVlan(e AuthorizeVlanCatalogEntry) bool {
	if e.VID <= 1 {
		return true
	}
	combo := strings.ToUpper(e.Name + " " + e.Description)
	if strings.Contains(combo, "GERENCIA") || strings.Contains(combo, "MANAGEMENT") {
		return true
	}
	return e.Pon <= 0
}

// ResolveAuthorizeVlanForPon usa o catálogo guardado no perfil (entradas não ignoradas).
func ResolveAuthorizeVlanForPon(cfg OnuReportConfig, pon int) (vlan string, ok bool) {
	if pon <= 0 {
		return "", false
	}
	for _, e := range cfg.AuthorizeVlanCatalog {
		if e.Ignored || e.VID <= 0 {
			continue
		}
		mappedPon := e.Pon
		if mappedPon <= 0 {
			mappedPon = PonFromZteVlanDescription(e.Description)
		}
		if mappedPon == pon {
			return strconv.Itoa(e.VID), true
		}
	}
	return "", false
}

// ParseZteVlanCatalogFromSNMPVars mantém compatibilidade: só entradas provisionáveis (PPPOE).
func ParseZteVlanCatalogFromSNMPVars(vars []probing.SNMPVar) []ZteVlanEntry {
	all := DiscoverZteVlanCatalogFromSNMPVars(vars)
	out := make([]ZteVlanEntry, 0, len(all))
	for _, e := range all {
		if e.Ignored || e.Pon <= 0 {
			continue
		}
		out = append(out, ZteVlanEntry{VID: e.VID, Name: e.Name, Description: e.Description})
	}
	return out
}

// IsProvisionableZteVlan filtra VLAN padrão (1), gestão e o que não for PPPOE-PONxx.
func IsProvisionableZteVlan(e ZteVlanEntry) bool {
	ae := AuthorizeVlanCatalogEntry{VID: e.VID, Name: e.Name, Description: e.Description, Pon: PonFromZteVlanDescription(e.Description)}
	return !SuggestIgnoreZteVlan(ae)
}

// PonFromZteVlanDescription extrai o número da PON de "PPPOE-PON01", "PPPOE-PON9", etc.
func PonFromZteVlanDescription(desc string) int {
	m := pppoePonDescRE.FindStringSubmatch(strings.TrimSpace(desc))
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// ResolveVlanIDForPon devolve o VID associado à PON via descrição PPPOE-PONxx.
func ResolveVlanIDForPon(entries []ZteVlanEntry, pon int) int {
	if pon <= 0 {
		return 0
	}
	for _, e := range entries {
		if PonFromZteVlanDescription(e.Description) == pon {
			return e.VID
		}
	}
	return 0
}

// DiscoverZteVlanCatalogViaSNMP faz um walk único e devolve o catálogo ordenado.
func DiscoverZteVlanCatalogViaSNMP(ctx context.Context, host, community, rootOID string, timeout time.Duration) (entries []AuthorizeVlanCatalogEntry, usedOID string, errMsg string) {
	usedOID = strings.TrimSpace(rootOID)
	if usedOID == "" {
		usedOID = ZteVlanTableOIDBase
	}
	usedOID = probing.NormalizeSNMPOID(usedOID)
	if strings.TrimSpace(host) == "" {
		return nil, usedOID, "host vazio"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	walkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	vars, _, walkErr := probing.SNMPWalk(walkCtx, probing.SNMPWalkParams{
		Host: host, Community: community, RootOID: usedOID,
		Timeout: 8 * time.Second, Retries: 1, MaxRows: 8192,
	})
	if walkErr != "" && len(vars) == 0 {
		return nil, usedOID, walkErr
	}
	return DiscoverZteVlanCatalogFromSNMPVars(vars), usedOID, ""
}

// LookupZtePonVlanViaSNMP consulta SNMP ao vivo (fallback se o catálogo do perfil estiver vazio).
func LookupZtePonVlanViaSNMP(ctx context.Context, host, community string, pon int, rootOID string, timeout time.Duration) (vlan string, source string, errMsg string) {
	if pon <= 0 || strings.TrimSpace(host) == "" {
		return "", "", ""
	}
	entries, usedOID, errMsg := DiscoverZteVlanCatalogViaSNMP(ctx, host, community, rootOID, timeout)
	if errMsg != "" && len(entries) == 0 {
		return "", "", errMsg
	}
	for _, e := range entries {
		if e.Ignored {
			continue
		}
		if e.Pon == pon || PonFromZteVlanDescription(e.Description) == pon {
			return strconv.Itoa(e.VID), "zte_vlan_snmp:" + usedOID, ""
		}
	}
	return "", "", ""
}

func classifyZteVlanOID(oid string) (vid int, kind string) {
	oid = strings.Trim(oid, ".")
	parts := strings.Split(oid, ".")
	if len(parts) < 2 {
		return 0, ""
	}
	vid, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || vid <= 0 {
		return 0, ""
	}
	col, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil {
		return vid, "other"
	}
	switch col {
	case 2:
		return vid, "name"
	case 3:
		return vid, "desc"
	default:
		return vid, "other"
	}
}

func oidLastInt(oid string) int {
	vid, _ := classifyZteVlanOID(oid)
	return vid
}

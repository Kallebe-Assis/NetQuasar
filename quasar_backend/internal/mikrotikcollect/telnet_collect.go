package mikrotikcollect

import (
	"context"
	"strings"
	"time"

	"github.com/netquasar/netquasar/quasar_backend/internal/probing"
)

const maxPerInterfaceCollect = 32

// TelnetFieldResult resultado por métrica telnet.
type TelnetFieldResult struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	OK          bool   `json:"ok"`
	Command     string `json:"command,omitempty"`
	Parser      string `json:"parser,omitempty"`
	Value       any    `json:"value,omitempty"`
	RawOutput   string `json:"raw_output,omitempty"`
	Error       string `json:"error,omitempty"`
	ProfileName string `json:"profile_name,omitempty"`
}

// TelnetCollectOutput resultado completo telnet.
type TelnetCollectOutput struct {
	Fields      map[string]TelnetFieldResult `json:"fields"`
	ProfileID   string                       `json:"profile_id,omitempty"`
	ProfileName string                       `json:"profile_name,omitempty"`
	Collected   int                          `json:"collected"`
	Failed      int                          `json:"failed"`
	Message     string                       `json:"message,omitempty"`
}

type telnetJob struct {
	keys    []string
	entries map[string]TelnetCatalogEntry
	command string
	iface   string
}

// CollectTelnetMetrics executa comandos RouterOS conforme perfil telnet.
func CollectTelnetMetrics(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration) TelnetCollectOutput {
	return CollectTelnetMetricsWithCatalog(ctx, host, creds, profile, timeout, TelnetMetricCatalog)
}

// CollectTelnetMetricsWithCatalog igual a CollectTelnetMetrics, com catálogo explícito (ex.: NX-OS Switch).
func CollectTelnetMetricsWithCatalog(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration, catalog []TelnetCatalogEntry) TelnetCollectOutput {
	out := TelnetCollectOutput{
		Fields:      make(map[string]TelnetFieldResult),
		ProfileID:   profile.ID.String(),
		ProfileName: profile.Name,
	}
	if catalog == nil {
		catalog = TelnetMetricCatalog
	}
	if !hasEnabledTelnetMetricsInCatalog(profile.Metrics, catalog) {
		out.Message = "nenhuma métrica telnet activa no perfil"
		return out
	}
	host = strings.TrimSpace(host)
	if host == "" {
		out.Message = "host em falta"
		return out
	}
	if strings.TrimSpace(creds.User) == "" || strings.TrimSpace(creds.Password) == "" {
		out.Message = "credenciais telnet não configuradas (Definições → Rede e SNMP)"
		return out
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	globalJobs, scopedJobs := buildTelnetJobsFromCatalog(profile, catalog)
	cmdOutputs := map[string]probing.TelnetRunScriptResult{}

	for _, j := range globalJobs {
		runTelnetJob(ctx, host, creds, profile, timeout, j, cmdOutputs, &out)
	}

	if len(scopedJobs) > 0 {
		ifaces := discoverInterfacesForScope(ctx, host, creds, profile, timeout, scopedJobs, cmdOutputs, &out)
		for _, j := range scopedJobs {
			entry0 := j.entries[j.keys[0]]
			names := filterInterfacesForScope(ifaces, entry0.Scope)
			if len(names) == 0 {
				for _, key := range j.keys {
					entry := j.entries[key]
					out.Fields[key] = TelnetFieldResult{
						Key: key, Label: entry.Label, Command: j.command,
						Parser: entry.Parser, OK: false,
						Error:      "nenhuma interface encontrada para este scope",
						ProfileName: profile.Name,
					}
					out.Failed++
				}
				continue
			}
			limit := len(names)
			if limit > maxPerInterfaceCollect {
				limit = maxPerInterfaceCollect
			}
			keyValues := make(map[string][]any, len(j.keys))
			for _, key := range j.keys {
				keyValues[key] = []any{}
			}
			anyOK := false
			for i := 0; i < limit; i++ {
				iface := names[i]
				cmd := substituteInterface(j.command, iface)
				res := runTelnetCommand(ctx, host, creds, profile, timeout, cmd, cmdOutputs)
				if !res.OK {
					continue
				}
				anyOK = true
				for _, key := range j.keys {
					entry := j.entries[key]
					val := parseTelnetOutputForIface(entry.Parser, res.Output, iface)
					keyValues[key] = append(keyValues[key], val)
				}
			}
			for _, key := range j.keys {
				entry := j.entries[key]
				vals := keyValues[key]
				if len(vals) == 0 {
					out.Fields[key] = TelnetFieldResult{
						Key: key, Label: entry.Label, Command: j.command,
						Parser: entry.Parser, OK: false,
						Error:      "falha ao coletar por interface",
						ProfileName: profile.Name,
					}
					out.Failed++
					continue
				}
				out.Fields[key] = TelnetFieldResult{
					Key: key, Label: entry.Label, Command: j.command,
					Parser: entry.Parser, OK: anyOK, Value: vals,
					ProfileName: profile.Name,
				}
				out.Collected++
			}
		}
	}

	if out.Collected == 0 && out.Failed > 0 && out.Message == "" {
		out.Message = "falha na coleta telnet — verifique credenciais e conectividade"
	}
	return out
}

func hasEnabledTelnetMetricsInCatalog(c TelnetMetricsConfig, catalog []TelnetCatalogEntry) bool {
	for _, e := range catalog {
		if def, ok := c[e.Key]; ok && def.Enabled {
			return true
		}
	}
	return false
}

func buildTelnetJobs(profile TelnetProfile) (global []telnetJob, scoped []telnetJob) {
	return buildTelnetJobsFromCatalog(profile, TelnetMetricCatalog)
}

func buildTelnetJobsFromCatalog(profile TelnetProfile, catalog []TelnetCatalogEntry) (global []telnetJob, scoped []telnetJob) {
	type pending struct {
		keys    []string
		entries map[string]TelnetCatalogEntry
	}
	globalByCmd := map[string]*pending{}
	scopedByCmd := map[string]*pending{}
	catByKey := map[string]TelnetCatalogEntry{}
	for _, e := range catalog {
		catByKey[e.Key] = e
	}

	for _, entry := range catalog {
		def, ok := profile.Metrics[entry.Key]
		if !ok || !def.Enabled {
			continue
		}
		cmd := strings.TrimSpace(def.Command)
		if cmd == "" {
			cmd = entry.DefaultCommand
		}
		bucket := globalByCmd
		if entry.Scope != "" {
			bucket = scopedByCmd
		}
		p, exists := bucket[cmd]
		if !exists {
			p = &pending{entries: map[string]TelnetCatalogEntry{}}
			bucket[cmd] = p
		}
		p.keys = append(p.keys, entry.Key)
		p.entries[entry.Key] = entry
	}

	for cmd, p := range globalByCmd {
		global = append(global, telnetJob{keys: p.keys, entries: p.entries, command: cmd})
	}
	for cmd, p := range scopedByCmd {
		scoped = append(scoped, telnetJob{keys: p.keys, entries: p.entries, command: cmd})
	}
	_ = catByKey
	return global, scoped
}

func runTelnetJob(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration, j telnetJob, cache map[string]probing.TelnetRunScriptResult, out *TelnetCollectOutput) {
	res := runTelnetCommand(ctx, host, creds, profile, timeout, j.command, cache)
	for _, key := range j.keys {
		entry := j.entries[key]
		applyTelnetJobResult(out, key, entry, j.command, res, profile.Name)
	}
}

func runTelnetCommand(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration, command string, cache map[string]probing.TelnetRunScriptResult) probing.TelnetRunScriptResult {
	if res, ok := cache[command]; ok {
		return res
	}
	res := probing.TelnetRunScript(ctx, probing.TelnetRunScriptParams{
		Host:         host,
		Port:         creds.Port,
		Timeout:      timeout,
		User:         creds.User,
		Password:     creds.Password,
		Enable:       creds.Enable,
		PreCommands:  profile.PreCommands,
		Commands:     []string{command},
		MaxReadBytes: 2 * 1024 * 1024,
	})
	cache[command] = res
	return res
}

func discoverInterfacesForScope(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration, scoped []telnetJob, cache map[string]probing.TelnetRunScriptResult, out *TelnetCollectOutput) []interfaceDiscovery {
	listCmd := ""
	if def, ok := profile.Metrics["telnet_if_list"]; ok && strings.TrimSpace(def.Command) != "" {
		listCmd = strings.TrimSpace(def.Command)
	}
	if listCmd == "" {
		listCmd = profile.Metrics.CommandFor("telnet_if_list")
	}
	if listCmd == "" {
		if e, ok := catalogEntryForKey("telnet_if_list"); ok {
			listCmd = e.DefaultCommand
		} else {
			listCmd = "/interface print without-paging"
		}
	}
	_ = scoped
	res := runTelnetCommand(ctx, host, creds, profile, timeout, listCmd, cache)
	if !res.OK {
		return nil
	}
	ifaces := DiscoverInterfacesFromPrint(res.Output)
	if len(ifaces) == 0 {
		ifaces = DiscoverInterfacesFromNxosStatus(res.Output)
	}
	return ifaces
}

func filterInterfacesForScope(ifaces []interfaceDiscovery, scope string) []string {
	var names []string
	seen := map[string]bool{}
	for _, iface := range ifaces {
		include := false
		switch scope {
		case TelnetScopePerInterface:
			include = iface.Running || iface.Name != ""
		case TelnetScopePerEthernet:
			include = iface.IsEthernet
		case TelnetScopePerSFP:
			include = iface.IsSFP || (iface.IsEthernet && strings.Contains(strings.ToLower(iface.Name), "sfp"))
		}
		if include && !seen[iface.Name] {
			seen[iface.Name] = true
			names = append(names, iface.Name)
		}
	}
	if scope == TelnetScopePerSFP && len(names) == 0 {
		for _, iface := range ifaces {
			if iface.IsEthernet && !seen[iface.Name] {
				seen[iface.Name] = true
				names = append(names, iface.Name)
			}
		}
	}
	return names
}

func substituteInterface(cmd, iface string) string {
	cmd = strings.ReplaceAll(cmd, "{interface}", iface)
	cmd = strings.ReplaceAll(cmd, "<interface>", iface)
	cmd = strings.ReplaceAll(cmd, "<sfp>", iface)
	return cmd
}

func applyTelnetJobResult(out *TelnetCollectOutput, key string, entry TelnetCatalogEntry, command string, res probing.TelnetRunScriptResult, profileName string) {
	fr := TelnetFieldResult{
		Key: key, Label: entry.Label, Command: command, Parser: entry.Parser, ProfileName: profileName,
	}
	raw := strings.TrimSpace(res.Output)
	if !res.OK {
		fr.Error = strings.TrimSpace(res.Error)
		if fr.Error == "" {
			fr.Error = "telnet falhou"
		}
		out.Fields[key] = fr
		out.Failed++
		return
	}
	fr.OK = true
	fr.RawOutput = raw
	// TelnetRunScript prefixa "$ <cmd>\n" — remove para o parser NX-OS/RouterOS.
	parseIn := stripTelnetScriptEcho(raw, command)
	fr.Value = parseTelnetOutput(entry.Parser, parseIn)
	out.Fields[key] = fr
	out.Collected++
}

func stripTelnetScriptEcho(raw, command string) string {
	raw = strings.TrimSpace(raw)
	command = strings.TrimSpace(command)
	if raw == "" {
		return raw
	}
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return raw
	}
	first := strings.TrimSpace(lines[0])
	if strings.HasPrefix(first, "$ ") {
		rest := strings.TrimSpace(strings.TrimPrefix(first, "$ "))
		if command == "" || strings.EqualFold(rest, command) || strings.HasPrefix(strings.ToLower(rest), strings.ToLower(command)) {
			return strings.TrimSpace(strings.Join(lines[1:], "\n"))
		}
	}
	return raw
}

// CollectTelnetInterfaceMetrics atalho para métricas de interface/optical/wireless activas.
func CollectTelnetInterfaceMetrics(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration) TelnetCollectOutput {
	return CollectTelnetInterfaceMetricsWithCatalog(ctx, host, creds, profile, timeout, TelnetMetricCatalog)
}

// CollectTelnetInterfaceMetricsWithCatalog igual, com catálogo explícito (ex.: NX-OS Switch).
func CollectTelnetInterfaceMetricsWithCatalog(ctx context.Context, host string, creds TelnetCredentials, profile TelnetProfile, timeout time.Duration, catalog []TelnetCatalogEntry) TelnetCollectOutput {
	if catalog == nil {
		catalog = TelnetMetricCatalog
	}
	filtered := TelnetProfile{
		ID: profile.ID, Name: profile.Name, PreCommands: profile.PreCommands,
		Metrics: make(TelnetMetricsConfig),
	}
	for _, e := range catalog {
		if e.Section != "interfaces" && e.Section != "optical" && e.Section != "wireless" {
			continue
		}
		if def, ok := profile.Metrics[e.Key]; ok && def.Enabled {
			filtered.Metrics[e.Key] = def
		}
	}
	return CollectTelnetMetricsWithCatalog(ctx, host, creds, filtered, timeout, catalog)
}

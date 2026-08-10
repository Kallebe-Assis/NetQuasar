package api

import "strings"

type permissionDefinition struct {
	Key         string `json:"key"`
	Module      string `json:"module"`
	ModuleLabel string `json:"module_label"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

var permissionCatalog = []permissionDefinition{
	{Key: "dashboard.view", Module: "dashboard", ModuleLabel: "Dashboard", Label: "Visualizar"},
	{Key: "monitoring.view", Module: "monitoring", ModuleLabel: "Monitoramento", Label: "Visualizar"},
	{Key: "monitoring.control", Module: "monitoring", ModuleLabel: "Monitoramento", Label: "Iniciar, parar e executar ciclos"},
	{Key: "realtime.view", Module: "realtime", ModuleLabel: "Tempo real", Label: "Visualizar"},
	{Key: "integrations.view", Module: "integrations", ModuleLabel: "Integrações", Label: "Visualizar e consultar"},
	{Key: "integrations.execute", Module: "integrations", ModuleLabel: "Integrações", Label: "Executar requisições"},
	{Key: "integrations.manage", Module: "integrations", ModuleLabel: "Integrações", Label: "Criar, editar e excluir"},
	{Key: "pops.view", Module: "pops", ModuleLabel: "Localidades", Label: "Visualizar"},
	{Key: "pops.manage", Module: "pops", ModuleLabel: "Localidades", Label: "Criar, editar e excluir"},
	{Key: "devices.view", Module: "devices", ModuleLabel: "Equipamentos", Label: "Visualizar"},
	{Key: "devices.manage", Module: "devices", ModuleLabel: "Equipamentos", Label: "Criar, editar e excluir"},
	{Key: "devices.collect", Module: "devices", ModuleLabel: "Equipamentos", Label: "Executar ping, telemetria e interfaces"},
	{Key: "devices.backup", Module: "devices", ModuleLabel: "Equipamentos", Label: "Gerir backups"},
	{Key: "commercial.view", Module: "commercial", ModuleLabel: "Clientes", Label: "Visualizar"},
	{Key: "commercial.manage", Module: "commercial", ModuleLabel: "Clientes", Label: "Criar, editar, importar e excluir"},
	{Key: "connections.view", Module: "connections", ModuleLabel: "Elementos", Label: "Visualizar"},
	{Key: "connections.manage", Module: "connections", ModuleLabel: "Elementos", Label: "Criar, editar e excluir"},
	{Key: "alerts.view", Module: "alerts", ModuleLabel: "Alertas", Label: "Visualizar"},
	{Key: "alerts.verify", Module: "alerts", ModuleLabel: "Alertas", Label: "Verificar e reavaliar alertas"},
	{Key: "alerts.manage", Module: "alerts", ModuleLabel: "Alertas", Label: "Regras, supressões e manutenção"},
	{Key: "map.view", Module: "map", ModuleLabel: "Mapa", Label: "Visualizar"},
	{Key: "map.manage", Module: "map", ModuleLabel: "Mapa", Label: "Editar posições e elementos"},
	{Key: "tools.view", Module: "tools", ModuleLabel: "Ferramentas", Label: "Visualizar"},
	{Key: "tools.execute", Module: "tools", ModuleLabel: "Ferramentas", Label: "Executar ferramentas e SNMP Walk"},
	{Key: "olt.view", Module: "olt", ModuleLabel: "OLT", Label: "Visualizar"},
	{Key: "olt.collect", Module: "olt", ModuleLabel: "OLT", Label: "Atualizar e coletar dados"},
	{Key: "olt.onu_manage", Module: "olt", ModuleLabel: "OLT", Label: "Autorizar e desautorizar ONUs"},
	{Key: "mikrotik.view", Module: "mikrotik", ModuleLabel: "MikroTik", Label: "Visualizar"},
	{Key: "mikrotik.collect", Module: "mikrotik", ModuleLabel: "MikroTik", Label: "Atualizar telemetria e interfaces"},
	{Key: "switch.view", Module: "switch", ModuleLabel: "Switch", Label: "Visualizar"},
	{Key: "switch.collect", Module: "switch", ModuleLabel: "Switch", Label: "Atualizar telemetria e interfaces"},
	{Key: "bng.view", Module: "bng", ModuleLabel: "BNG", Label: "Visualizar"},
	{Key: "bng.collect", Module: "bng", ModuleLabel: "BNG", Label: "Atualizar sessões e infraestrutura"},
	{Key: "reports.view", Module: "reports", ModuleLabel: "Relatórios", Label: "Visualizar e exportar"},
	{Key: "reports.send", Module: "reports", ModuleLabel: "Relatórios", Label: "Enviar relatórios"},
	{Key: "reports.manage", Module: "reports", ModuleLabel: "Relatórios", Label: "Gerir agendamentos"},
	{Key: "fleet.view", Module: "fleet", ModuleLabel: "Frota", Label: "Visualizar"},
	{Key: "fleet.manage", Module: "fleet", ModuleLabel: "Frota", Label: "Criar, editar e lançar abastecimentos"},
	{Key: "settings.view", Module: "settings", ModuleLabel: "Configurações", Label: "Visualizar"},
	{Key: "settings.system", Module: "settings", ModuleLabel: "Configurações", Label: "Base de dados, rede e aparência"},
	{Key: "settings.monitoring", Module: "settings", ModuleLabel: "Configurações", Label: "Monitoramento, alertas e coletas"},
	{Key: "settings.notifications", Module: "settings", ModuleLabel: "Configurações", Label: "Notificações e automações"},
	{Key: "settings.users", Module: "settings", ModuleLabel: "Configurações", Label: "Gerir usuários"},
	{Key: "settings.permissions", Module: "settings", ModuleLabel: "Configurações", Label: "Gerir perfis de permissão"},
}

var validPermissionKeys = func() map[string]struct{} {
	out := make(map[string]struct{}, len(permissionCatalog))
	for _, p := range permissionCatalog {
		out[p.Key] = struct{}{}
	}
	return out
}()

func normalizePermissions(in []string) ([]string, []string) {
	seen := make(map[string]struct{}, len(in))
	valid := make([]string, 0, len(in))
	invalid := make([]string, 0)
	for _, raw := range in {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		if key != "*" {
			if _, ok := validPermissionKeys[key]; !ok {
				invalid = append(invalid, key)
				continue
			}
		}
		valid = append(valid, key)
	}
	return valid, invalid
}

func permissionGranted(granted []string, required string) bool {
	required = strings.ToLower(strings.TrimSpace(required))
	for _, key := range granted {
		if key == "*" || strings.EqualFold(strings.TrimSpace(key), required) {
			return true
		}
	}
	return false
}

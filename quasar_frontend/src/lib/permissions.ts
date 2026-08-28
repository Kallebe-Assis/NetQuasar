/** Catálogo de permissões unitárias (espelha o backend). */
export type PermissionKey =
  | "*"
  | "dashboard.view"
  | "monitoring.view"
  | "monitoring.control"
  | "realtime.view"
  | "integrations.view"
  | "integrations.execute"
  | "integrations.manage"
  | "pops.view"
  | "pops.manage"
  | "devices.view"
  | "devices.manage"
  | "devices.collect"
  | "devices.backup"
  | "commercial.view"
  | "commercial.manage"
  | "connections.view"
  | "connections.manage"
  | "alerts.view"
  | "alerts.verify"
  | "alerts.manage"
  | "map.view"
  | "map.manage"
  | "tools.view"
  | "tools.execute"
  | "olt.view"
  | "olt.collect"
  | "olt.onu_manage"
  | "mikrotik.view"
  | "mikrotik.collect"
  | "switch.view"
  | "switch.collect"
  | "bng.view"
  | "bng.collect"
  | "bgp.view"
  | "bgp.collect"
  | "reports.view"
  | "reports.send"
  | "reports.manage"
  | "fleet.view"
  | "fleet.manage"
  | "network_events.view"
  | "network_events.manage"
  | "settings.view"
  | "settings.system"
  | "settings.monitoring"
  | "settings.notifications"
  | "settings.users"
  | "settings.permissions";

export type PermissionDefinition = {
  key: PermissionKey;
  module: string;
  module_label: string;
  label: string;
  description?: string;
};

export type PermissionProfile = {
  id: string;
  name: string;
  slug: string;
  description?: string | null;
  permissions: string[];
  is_system: boolean;
  created_at?: string;
  updated_at?: string;
};

export const PERMISSION_CATALOG: PermissionDefinition[] = [
  { key: "dashboard.view", module: "dashboard", module_label: "Dashboard", label: "Visualizar" },
  { key: "monitoring.view", module: "monitoring", module_label: "Monitoramento", label: "Visualizar" },
  { key: "monitoring.control", module: "monitoring", module_label: "Monitoramento", label: "Iniciar, parar e executar ciclos" },
  { key: "realtime.view", module: "realtime", module_label: "Tempo real", label: "Visualizar" },
  { key: "integrations.view", module: "integrations", module_label: "Integrações", label: "Visualizar e consultar" },
  { key: "integrations.execute", module: "integrations", module_label: "Integrações", label: "Executar requisições" },
  { key: "integrations.manage", module: "integrations", module_label: "Integrações", label: "Criar, editar e excluir" },
  { key: "pops.view", module: "pops", module_label: "Localidades", label: "Visualizar" },
  { key: "pops.manage", module: "pops", module_label: "Localidades", label: "Criar, editar e excluir" },
  { key: "devices.view", module: "devices", module_label: "Equipamentos", label: "Visualizar" },
  { key: "devices.manage", module: "devices", module_label: "Equipamentos", label: "Criar, editar e excluir" },
  { key: "devices.collect", module: "devices", module_label: "Equipamentos", label: "Executar ping, telemetria e interfaces" },
  { key: "devices.backup", module: "devices", module_label: "Equipamentos", label: "Gerir backups" },
  { key: "commercial.view", module: "commercial", module_label: "Clientes", label: "Visualizar" },
  { key: "commercial.manage", module: "commercial", module_label: "Clientes", label: "Criar, editar, importar e excluir" },
  { key: "connections.view", module: "connections", module_label: "Elementos", label: "Visualizar" },
  { key: "connections.manage", module: "connections", module_label: "Elementos", label: "Criar, editar e excluir" },
  { key: "alerts.view", module: "alerts", module_label: "Alertas", label: "Visualizar" },
  { key: "alerts.verify", module: "alerts", module_label: "Alertas", label: "Verificar e reavaliar alertas" },
  { key: "alerts.manage", module: "alerts", module_label: "Alertas", label: "Regras, supressões e manutenção" },
  { key: "map.view", module: "map", module_label: "Mapa", label: "Visualizar" },
  { key: "map.manage", module: "map", module_label: "Mapa", label: "Editar posições e elementos" },
  { key: "tools.view", module: "tools", module_label: "Ferramentas", label: "Visualizar" },
  { key: "tools.execute", module: "tools", module_label: "Ferramentas", label: "Executar ferramentas e SNMP Walk" },
  { key: "olt.view", module: "olt", module_label: "OLT", label: "Visualizar" },
  { key: "olt.collect", module: "olt", module_label: "OLT", label: "Atualizar e coletar dados" },
  { key: "olt.onu_manage", module: "olt", module_label: "OLT", label: "Autorizar e desautorizar ONUs" },
  { key: "mikrotik.view", module: "mikrotik", module_label: "MikroTik", label: "Visualizar" },
  { key: "mikrotik.collect", module: "mikrotik", module_label: "MikroTik", label: "Atualizar telemetria e interfaces" },
  { key: "switch.view", module: "switch", module_label: "Switch", label: "Visualizar" },
  { key: "switch.collect", module: "switch", module_label: "Switch", label: "Atualizar telemetria e interfaces" },
  { key: "bng.view", module: "bng", module_label: "BNG", label: "Visualizar" },
  { key: "bng.collect", module: "bng", module_label: "BNG", label: "Atualizar sessões e infraestrutura" },
  { key: "bgp.view", module: "bgp", module_label: "BGP", label: "Visualizar" },
  { key: "bgp.collect", module: "bgp", module_label: "BGP", label: "Atualizar coleta SNMP" },
  { key: "reports.view", module: "reports", module_label: "Relatórios", label: "Visualizar e exportar" },
  { key: "reports.send", module: "reports", module_label: "Relatórios", label: "Enviar relatórios" },
  { key: "reports.manage", module: "reports", module_label: "Relatórios", label: "Gerir agendamentos" },
  { key: "fleet.view", module: "fleet", module_label: "Frota", label: "Visualizar" },
  { key: "fleet.manage", module: "fleet", module_label: "Frota", label: "Criar, editar e lançar despesas" },
  { key: "network_events.view", module: "network_events", module_label: "Eventos", label: "Visualizar" },
  { key: "network_events.manage", module: "network_events", module_label: "Eventos", label: "Registar, editar e excluir" },
  { key: "settings.view", module: "settings", module_label: "Configurações", label: "Visualizar" },
  { key: "settings.system", module: "settings", module_label: "Configurações", label: "Base de dados, rede e aparência" },
  { key: "settings.monitoring", module: "settings", module_label: "Configurações", label: "Monitoramento, alertas e coletas" },
  { key: "settings.notifications", module: "settings", module_label: "Configurações", label: "Notificações e automações" },
  { key: "settings.users", module: "settings", module_label: "Configurações", label: "Gerir usuários" },
  { key: "settings.permissions", module: "settings", module_label: "Configurações", label: "Gerir perfis de permissão" },
];

/** Espelha o perfil sistema «Usuário» / defaultViewerPermissions do backend (sem Configurações). */
export const DEFAULT_USER_PERMISSIONS: PermissionKey[] = [
  "dashboard.view",
  "monitoring.view",
  "realtime.view",
  "integrations.view",
  "pops.view",
  "devices.view",
  "commercial.view",
  "connections.view",
  "alerts.view",
  "map.view",
  "tools.view",
  "olt.view",
  "mikrotik.view",
  "switch.view",
  "bng.view",
  "bgp.view",
  "reports.view",
  "fleet.view",
  "network_events.view",
];

export function groupPermissionsByModule(catalog: PermissionDefinition[] = PERMISSION_CATALOG) {
  const map = new Map<string, { module: string; module_label: string; items: PermissionDefinition[] }>();
  for (const p of catalog) {
    const cur = map.get(p.module) ?? { module: p.module, module_label: p.module_label, items: [] };
    cur.items.push(p);
    map.set(p.module, cur);
  }
  return [...map.values()];
}

export function hasPermission(granted: string[] | null | undefined, required: string): boolean {
  if (!granted || granted.length === 0) return false;
  const need = required.trim().toLowerCase();
  for (const g of granted) {
    const key = String(g ?? "").trim().toLowerCase();
    if (key === "*" || key === need) return true;
  }
  return false;
}

export function hasAnyPermission(granted: string[] | null | undefined, required: string[]): boolean {
  return required.some((k) => hasPermission(granted, k));
}

/** Mapeia rota da SPA → permissão de visualização do módulo. */
export const ROUTE_VIEW_PERMISSION: Record<string, PermissionKey> = {
  "/dashboard": "dashboard.view",
  "/monitoring": "monitoring.view",
  "/realtime": "realtime.view",
  "/integrations": "integrations.view",
  "/pops": "pops.view",
  "/devices": "devices.view",
  "/commercial": "commercial.view",
  "/connections": "connections.view",
  "/alerts": "alerts.view",
  "/map": "map.view",
  "/topology": "map.view",
  "/tools": "tools.view",
  "/olt": "olt.view",
  "/mikrotik": "mikrotik.view",
  "/switch": "switch.view",
  "/bng": "bng.view",
  "/bgp": "bgp.view",
  "/reports": "reports.view",
  "/fleet": "fleet.view",
  "/fleet/dashboard": "fleet.view",
  "/fleet/vehicles": "fleet.view",
  "/fleet/drivers": "fleet.view",
  "/fleet/elements": "fleet.view",
  "/fleet/fuelings": "fleet.view",
  "/fleet/fuels": "fleet.view",
  "/fleet/stations": "fleet.view",
  "/fleet/cost-centers": "fleet.view",
  "/fleet/alerts": "fleet.view",
  "/fleet/reports": "fleet.view",
  "/events": "network_events.view",
  "/settings": "settings.view",
};

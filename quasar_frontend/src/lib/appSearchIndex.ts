import { APP_ROUTES } from "../app/routes";

/**
 * Índice estático de telas/abas do próprio sistema (não entidades de dados) — permite que a
 * pesquisa geral do Dashboard (GlobalSearchBar.tsx) também encontre e leve directamente a telas
 * como "POPs", "Dashboard da Frota", "Configuração de usuários", em vez de só procurar
 * equipamentos/logins/CTOs/etc. via /api/v1/search/global. Cada entrada pode ter `keywords`
 * extra (sinónimos, nomes populares) além do próprio `label` — ex.: "Localidades" (nome no menu)
 * também é encontrado ao digitar "pop"/"pops" (como o utilizador chama a tela no dia-a-dia).
 */
export type AppScreenEntry = {
  label: string;
  subtitle?: string;
  href: string;
  keywords?: string[];
};

export const APP_SEARCH_INDEX: AppScreenEntry[] = [
  // Navegação principal (ShellLayout.tsx)
  { label: "Dashboard", href: APP_ROUTES.dashboard, keywords: ["início", "home", "visão geral"] },
  { label: "Monitoramento", href: APP_ROUTES.monitoring },
  { label: "Tempo real", href: APP_ROUTES.realtime, keywords: ["realtime"] },
  { label: "Integrações", href: APP_ROUTES.integrations, keywords: ["hubsoft", "ixc", "erp"] },
  { label: "Localidades", subtitle: "POPs", href: APP_ROUTES.pops, keywords: ["pop", "pops", "localidade", "site"] },
  { label: "Equipamentos — Geral", href: APP_ROUTES.devices, keywords: ["equipamentos", "dispositivos", "devices"] },
  { label: "Equipamentos — Mikrotik", href: APP_ROUTES.mikrotik },
  { label: "Equipamentos — OLT", href: APP_ROUTES.olt },
  { label: "Equipamentos — BNG", href: APP_ROUTES.bng },
  { label: "Equipamentos — BGP", href: APP_ROUTES.bgp },
  { label: "Equipamentos — Switch", href: APP_ROUTES.switch },
  { label: "Clientes", subtitle: "Comercial", href: APP_ROUTES.commercial, keywords: ["comercial", "contratos", "planos"] },
  { label: "Mapa", href: APP_ROUTES.map },
  { label: "Topologia", href: APP_ROUTES.topology, keywords: ["topologia 2d", "rede", "fibra"] },
  { label: "Alertas", href: APP_ROUTES.alerts },
  { label: "Eventos", href: APP_ROUTES.events },
  { label: "Registros", href: APP_ROUTES.registros, keywords: ["logs de acesso", "auditoria"] },
  { label: "Ferramentas", href: APP_ROUTES.tools, keywords: ["ping", "traceroute", "telnet", "ssh"] },
  { label: "Relatórios", href: APP_ROUTES.reports },
  { label: "Configurações", href: APP_ROUTES.settings, keywords: ["settings", "config"] },
  { label: "Sobre", href: APP_ROUTES.about },

  // Elementos / infraestrutura (ConnectionsPageShell.tsx — /connections?tab=…)
  { label: "Elementos — Logins", href: `${APP_ROUTES.connections}?tab=logins` },
  { label: "Elementos — CTO", href: `${APP_ROUTES.connections}?tab=cto` },
  { label: "Elementos — Caixa de Emenda", href: `${APP_ROUTES.connections}?tab=splice`, keywords: ["emenda", "foguete"] },
  { label: "Elementos — Cabos", href: `${APP_ROUTES.connections}?tab=cables` },
  { label: "Elementos — Postes", href: `${APP_ROUTES.connections}?tab=poles` },
  { label: "Elementos — Projetos", href: `${APP_ROUTES.connections}?tab=projects` },

  // Frota
  { label: "Frota — Dashboard", href: APP_ROUTES.fleetDashboard, keywords: ["frota"] },
  { label: "Frota — Veículos", href: APP_ROUTES.fleetVehicles },
  { label: "Frota — Motoristas", href: APP_ROUTES.fleetDrivers },
  { label: "Frota — Elementos", href: APP_ROUTES.fleetElements },
  { label: "Frota — Despesas", href: APP_ROUTES.fleetFuelings, keywords: ["combustível", "abastecimento"] },
  { label: "Frota — Combustíveis", href: APP_ROUTES.fleetFuels },
  { label: "Frota — Postos", href: APP_ROUTES.fleetStations },
  { label: "Frota — Centros de custo", href: APP_ROUTES.fleetCostCenters },
  { label: "Frota — Alertas", href: APP_ROUTES.fleetAlerts },
  { label: "Frota — Relatórios", href: APP_ROUTES.fleetReports },

  // HubSoft (integração)
  { label: "HubSoft — Atendimentos", href: APP_ROUTES.hubsoftAttendance },
  { label: "HubSoft — Ordens de serviço", href: APP_ROUTES.hubsoftWorkOrders, keywords: ["os", "o.s."] },
  { label: "HubSoft — Financeiro", href: APP_ROUTES.hubsoftFinancial, keywords: ["faturas", "boletos"] },
  { label: "HubSoft — Dashboard", href: APP_ROUTES.hubsoftDashboard },
  { label: "HubSoft — Relatório", href: APP_ROUTES.hubsoftReport },

  // Configurações (SettingsPage.tsx — /settings?tab=…)
  { label: "Configurações — Base de dados", href: `${APP_ROUTES.settings}?tab=database` },
  { label: "Configurações — Auditoria", href: `${APP_ROUTES.settings}?tab=logs` },
  { label: "Configurações — Usuários", href: `${APP_ROUTES.settings}?tab=users`, keywords: ["usuário", "permissões", "perfis de acesso"] },
  { label: "Configurações — Alertas", href: `${APP_ROUTES.settings}?tab=alerts` },
  { label: "Configurações — Monitoramento", href: `${APP_ROUTES.settings}?tab=monitoring` },
  { label: "Configurações — Aparência", href: `${APP_ROUTES.settings}?tab=appearance`, keywords: ["tema", "cores", "dark mode"] },
  { label: "Configurações — Rede e SNMP", href: `${APP_ROUTES.settings}?tab=connection` },
  { label: "Configurações — Telegram", href: `${APP_ROUTES.settings}?tab=telegram` },
  { label: "Configurações — OLT", href: `${APP_ROUTES.settings}?tab=olt` },
  { label: "Configurações — MikroTik", href: `${APP_ROUTES.settings}?tab=mikrotik` },
  { label: "Configurações — Switch", href: `${APP_ROUTES.settings}?tab=switch` },
  { label: "Configurações — BNG", href: `${APP_ROUTES.settings}?tab=bng` },
  { label: "Configurações — BGP", href: `${APP_ROUTES.settings}?tab=bgp` },
  { label: "Configurações — Frota", href: `${APP_ROUTES.settings}?tab=fleet` },
  { label: "Configurações — Automações", href: `${APP_ROUTES.settings}?tab=automation`, keywords: ["relatórios agendados", "agendamento"] },
];

/** Remove acentos + minúsculas — pesquisa tolerante ("localidade"/"localidáde"/"LOCALIDADE"). */
function normalize(s: string): string {
  return s
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase();
}

export function searchAppScreens(query: string): AppScreenEntry[] {
  const q = normalize(query.trim());
  if (q.length < 2) return [];
  return APP_SEARCH_INDEX.filter((e) => {
    if (normalize(e.label).includes(q)) return true;
    if (e.subtitle && normalize(e.subtitle).includes(q)) return true;
    return (e.keywords ?? []).some((k) => normalize(k).includes(q));
  });
}

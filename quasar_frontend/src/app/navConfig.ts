import type { LucideIcon } from "lucide-react";
import {
  BanknoteArrowDown,
  Bolt,
  CalendarClock,
  ChartPie,
  ClockCheck,
  Component,
  Cpu,
  FileBarChart,
  KeySquare,
  Layers,
  MapPin,
  MonitorSmartphone,
  Network,
  Plug,
  ShieldCheck,
  Share2,
  TriangleAlert,
  Truck,
  UserRoundKey,
  UsersRound,
  Warehouse,
  Waypoints,
  Wrench,
  Zap,
} from "lucide-react";
import { APP_ROUTES } from "./routes";

export type SubmoduleEntry = { label: string; tab: string };
export type NavChildConfig = { to: string; label: string; icons: LucideIcon[]; submodules?: SubmoduleEntry[] };
export type NavLeafConfig = { kind: "link"; to: string; label: string; icons: LucideIcon[]; submodules?: SubmoduleEntry[] };
export type NavGroupConfig = { kind: "group"; id: string; label: string; icons: LucideIcon[]; children: NavChildConfig[] };
export type NavEntryConfig = NavLeafConfig | NavGroupConfig;

/** Localidades (/pops) — PopsPage.tsx, type Tab. */
const POPS_SUBMODULES: SubmoduleEntry[] = [
  { label: "Localidades", tab: "localidades" },
  { label: "POPs", tab: "pops" },
];

/** OLT (/olt) — OltPage.tsx, type OltPageTab. */
const OLT_SUBMODULES: SubmoduleEntry[] = [
  { label: "Equipamentos", tab: "equipamentos" },
  { label: "ONU's", tab: "pesquisa" },
  { label: "Não autorizadas", tab: "nao_autorizadas" },
  { label: "Relatórios", tab: "relatorios" },
];

/** BGP (/bgp) — BgpPage.tsx, BGP_TABS. */
const BGP_SUBMODULES: SubmoduleEntry[] = [
  { label: "Visão Geral", tab: "overview" },
  { label: "Peers", tab: "peers" },
  { label: "Interfaces & LAG", tab: "interfaces" },
  { label: "Óptica", tab: "optics" },
  { label: "CPU & Memória", tab: "cpu" },
  { label: "Saúde do Chassi", tab: "chassis" },
  { label: "QoS", tab: "qos" },
  { label: "RADIUS", tab: "radius" },
  { label: "LLDP", tab: "lldp" },
];

/** BNG (/bng) — BngPage.tsx, BNG_TABS. */
const BNG_SUBMODULES: SubmoduleEntry[] = [
  { label: "Visão geral", tab: "overview" },
  { label: "Relatório", tab: "relatorio" },
  { label: "CGNAT e Pools", tab: "cgnat_pools" },
  { label: "VLANs", tab: "vlans" },
  { label: "Interfaces", tab: "interfaces" },
  { label: "Autenticações", tab: "auth" },
  { label: "Sessões PPPoE", tab: "sessions" },
];

/** Elementos (/connections) — ConnectionsPageShell.tsx, TABS. */
const CONNECTIONS_SUBMODULES: SubmoduleEntry[] = [
  { label: "Logins", tab: "logins" },
  { label: "CTO", tab: "cto" },
  { label: "Caixa de Emenda", tab: "splice" },
  { label: "Cabos", tab: "cables" },
  { label: "Postes", tab: "poles" },
  { label: "Projetos", tab: "projects" },
];

/** Ferramentas (/tools) — ToolsPage.tsx, type Tab. */
const TOOLS_SUBMODULES: SubmoduleEntry[] = [
  { label: "HTTP/HTTPS", tab: "http_matrix" },
  { label: "Domínios", tab: "host_ping" },
  { label: "ICMP", tab: "icmp" },
  { label: "Tracert", tab: "tracert" },
  { label: "Nmap", tab: "nmap" },
  { label: "Sniffer", tab: "sniffer" },
  { label: "SNMP get", tab: "snmp" },
  { label: "SNMP bulk", tab: "snmp_bulk" },
  { label: "Telnet", tab: "telnet" },
  { label: "SSH", tab: "ssh" },
  { label: "SNMP walk", tab: "snmp_walk" },
  { label: "Mikrotik", tab: "mikrotik" },
];

/** Frota — Elementos (/fleet/elements) — FleetElementsPage.tsx, type Tab. */
const FLEET_ELEMENTS_SUBMODULES: SubmoduleEntry[] = [
  { label: "Motoristas", tab: "motoristas" },
  { label: "Postos de combustível", tab: "postos" },
  { label: "Combustíveis", tab: "combustiveis" },
  { label: "Centros de custo", tab: "centros" },
  { label: "Tipos de despesa", tab: "tipos" },
];

/** Configurações (/settings) — SettingsPage.tsx, SETTINGS_TABS/SETTINGS_TAB_LABELS. */
const SETTINGS_SUBMODULES: SubmoduleEntry[] = [
  { label: "Base de dados", tab: "database" },
  { label: "Auditoria", tab: "logs" },
  { label: "Usuários", tab: "users" },
  { label: "Alertas", tab: "alerts" },
  { label: "Monitoramento", tab: "monitoring" },
  { label: "Aparência", tab: "appearance" },
  { label: "Rede e SNMP", tab: "connection" },
  { label: "Telegram", tab: "telegram" },
  { label: "OLT", tab: "olt" },
  { label: "MikroTik", tab: "mikrotik" },
  { label: "Switch", tab: "switch" },
  { label: "BNG", tab: "bng" },
  { label: "BGP", tab: "bgp" },
  { label: "Frota", tab: "fleet" },
  { label: "Automações", tab: "automation" },
];

/**
 * Fonte única de verdade da navegação principal — mesma estrutura que ShellLayout.tsx tinha
 * inline antes, com `submodules` acrescentado às entradas (ou filhos de grupo) que têm abas
 * internas de página. O menu lateral e o índice de pesquisa (appSearchIndex.ts) partilham isto.
 */
export const NAV_CONFIG: NavEntryConfig[] = [
  { kind: "link", to: APP_ROUTES.dashboard, label: "Dashboard", icons: [ChartPie] },
  { kind: "link", to: APP_ROUTES.monitoring, label: "Monitoramento", icons: [ShieldCheck] },
  { kind: "link", to: APP_ROUTES.realtime, label: "Tempo real", icons: [ClockCheck] },
  { kind: "link", to: APP_ROUTES.integrations, label: "Integrações", icons: [Plug] },
  { kind: "link", to: APP_ROUTES.pops, label: "Localidades", icons: [Warehouse], submodules: POPS_SUBMODULES },
  {
    kind: "group",
    id: "equipamentos",
    label: "Equipamentos",
    icons: [MonitorSmartphone],
    children: [
      { to: APP_ROUTES.devices, label: "Geral", icons: [MonitorSmartphone] },
      { to: APP_ROUTES.mikrotik, label: "Mikrotik", icons: [Cpu] },
      { to: APP_ROUTES.olt, label: "OLT", icons: [Zap], submodules: OLT_SUBMODULES },
      { to: APP_ROUTES.bng, label: "BNG", icons: [UserRoundKey], submodules: BNG_SUBMODULES },
      { to: APP_ROUTES.bgp, label: "BGP", icons: [Waypoints], submodules: BGP_SUBMODULES },
      { to: APP_ROUTES.switch, label: "Switch", icons: [Network] },
    ],
  },
  { kind: "link", to: APP_ROUTES.commercial, label: "Clientes", icons: [UsersRound] },
  {
    kind: "group",
    id: "mapa",
    label: "Mapa",
    icons: [MapPin],
    children: [
      { to: APP_ROUTES.map, label: "Mapa", icons: [MapPin] },
      { to: APP_ROUTES.connections, label: "Elementos", icons: [Component], submodules: CONNECTIONS_SUBMODULES },
      { to: APP_ROUTES.topology, label: "Topologia", icons: [Share2] },
    ],
  },
  { kind: "link", to: APP_ROUTES.alerts, label: "Alertas", icons: [TriangleAlert] },
  { kind: "link", to: APP_ROUTES.events, label: "Eventos", icons: [CalendarClock] },
  { kind: "link", to: APP_ROUTES.registros, label: "Registros", icons: [KeySquare] },
  { kind: "link", to: APP_ROUTES.tools, label: "Ferramentas", icons: [Wrench], submodules: TOOLS_SUBMODULES },
  {
    kind: "group",
    id: "frota",
    label: "Frota",
    icons: [Truck],
    children: [
      { to: APP_ROUTES.fleetDashboard, label: "Dashboard", icons: [ChartPie] },
      { to: APP_ROUTES.fleetVehicles, label: "Veículos", icons: [Truck] },
      { to: APP_ROUTES.fleetFuelings, label: "Despesas", icons: [BanknoteArrowDown] },
      { to: APP_ROUTES.fleetElements, label: "Elementos", icons: [Layers], submodules: FLEET_ELEMENTS_SUBMODULES },
      { to: APP_ROUTES.fleetAlerts, label: "Alertas", icons: [TriangleAlert] },
      { to: APP_ROUTES.fleetReports, label: "Relatórios", icons: [FileBarChart] },
    ],
  },
  { kind: "link", to: APP_ROUTES.reports, label: "Relatórios", icons: [FileBarChart] },
  { kind: "link", to: APP_ROUTES.settings, label: "Configurações", icons: [Bolt], submodules: SETTINGS_SUBMODULES },
];

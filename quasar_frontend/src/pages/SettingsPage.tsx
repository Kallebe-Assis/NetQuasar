import { lazy, Suspense, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { OverflowTabs } from "../components/OverflowTabs";
import {
  Activity,
  Bell,
  CalendarClock,
  Cpu,
  Database,
  History,
  type LucideIcon,
  Network,
  Palette,
  Send,
  Truck,
  Users,
  UserRoundKey,
  Waypoints,
  Wifi,
  Zap,
} from "lucide-react";
import { can, isAdminUser } from "../lib/auth";

// Cada aba é um chunk próprio — abrir Configurações não baixa o código das 15 abas de uma vez.
const AppearancePanel = lazy(() => import("./settings/AppearancePanel").then((m) => ({ default: m.AppearancePanel })));
const AlertNotificationPrefsPanel = lazy(() =>
  import("./settings/AlertNotificationPrefsPanel").then((m) => ({ default: m.AlertNotificationPrefsPanel })),
);
const MonitoringSettingsPanel = lazy(() =>
  import("./settings/MonitoringPipelinePanel").then((m) => ({ default: m.MonitoringSettingsPanel })),
);
const AuditingPanel = lazy(() => import("./settings/AuditingPanel").then((m) => ({ default: m.AuditingPanel })));
const ScheduledReportsPanel = lazy(() =>
  import("./settings/ScheduledReportsPanel").then((m) => ({ default: m.ScheduledReportsPanel })),
);
const MikrotikSettingsPanel = lazy(() =>
  import("./settings/MikrotikSettingsPanel").then((m) => ({ default: m.MikrotikSettingsPanel })),
);
const SwitchSettingsPanel = lazy(() =>
  import("./settings/SwitchSettingsPanel").then((m) => ({ default: m.SwitchSettingsPanel })),
);
const BngCollectionPanel = lazy(() =>
  import("./settings/BngCollectionPanel").then((m) => ({ default: m.BngCollectionPanel })),
);
const BgpSnmpProfilesPanel = lazy(() =>
  import("./settings/BgpSnmpProfilesPanel").then((m) => ({ default: m.BgpSnmpProfilesPanel })),
);
const BgpUplinkCarriersPanel = lazy(() =>
  import("./settings/BgpUplinkCarriersPanel").then((m) => ({ default: m.BgpUplinkCarriersPanel })),
);
const DatabasePanel = lazy(() => import("./settings/DatabasePanel").then((m) => ({ default: m.DatabasePanel })));
const FleetSettingsPanel = lazy(() =>
  import("./settings/FleetSettingsPanel").then((m) => ({ default: m.FleetSettingsPanel })),
);
const OltVendorsPanel = lazy(() => import("./settings/OltVendorsPanel").then((m) => ({ default: m.OltVendorsPanel })));
const UsersPanel = lazy(() => import("./settings/UsersPanel").then((m) => ({ default: m.UsersPanel })));
const AlertThresholdsPanel = lazy(() =>
  import("./settings/AlertThresholdsPanel").then((m) => ({ default: m.AlertThresholdsPanel })),
);
const ConnectionPanel = lazy(() => import("./settings/ConnectionPanel").then((m) => ({ default: m.ConnectionPanel })));
const TelegramPanel = lazy(() => import("./settings/TelegramPanel").then((m) => ({ default: m.TelegramPanel })));

type SettingsTab =
  | "database"
  | "logs"
  | "users"
  | "alerts"
  | "monitoring"
  | "appearance"
  | "connection"
  | "telegram"
  | "olt"
  | "mikrotik"
  | "switch"
  | "bng"
  | "bgp"
  | "fleet"
  | "automation";

const SETTINGS_TABS: SettingsTab[] = [
  "database", "logs", "users", "alerts", "monitoring", "appearance", "connection",
  "telegram", "olt", "mikrotik", "switch", "bng", "bgp", "fleet", "automation",
];

const SETTINGS_TAB_LABELS: Record<SettingsTab, string> = {
  database: "Base de dados",
  logs: "Auditoria",
  users: "Usuários",
  alerts: "Alertas",
  monitoring: "Monitoramento",
  appearance: "Aparência",
  connection: "SNMP",
  telegram: "Telegram",
  olt: "OLT",
  mikrotik: "MikroTik",
  switch: "Switch",
  bng: "BNG",
  bgp: "BGP",
  fleet: "Frota",
  automation: "Automações",
};

const SETTINGS_TAB_ICONS: Record<SettingsTab, LucideIcon> = {
  database: Database,
  logs: History,
  users: Users,
  alerts: Bell,
  monitoring: Activity,
  appearance: Palette,
  connection: Wifi,
  telegram: Send,
  olt: Zap,
  mikrotik: Cpu,
  switch: Network,
  bng: UserRoundKey,
  bgp: Waypoints,
  fleet: Truck,
  automation: CalendarClock,
};

function canSeeSettingsTab(tab: SettingsTab): boolean {
  if (isAdminUser()) return true;
  if (tab === "appearance" || tab === "alerts") return true;
  if (tab === "users" || tab === "logs") return can("settings.users") || can("settings.permissions");
  if (tab === "database" || tab === "connection") return can("settings.system");
  if (tab === "monitoring" || tab === "olt" || tab === "mikrotik" || tab === "switch" || tab === "bng" || tab === "bgp") {
    return can("settings.monitoring");
  }
  if (tab === "telegram" || tab === "automation") return can("settings.notifications");
  if (tab === "fleet") return can("fleet.manage") || can("settings.system");
  return can("settings.view");
}

function canEditAlertThresholds(): boolean {
  return isAdminUser() || can("alerts.manage") || can("settings.monitoring");
}

function TabContent({ tab }: { tab: SettingsTab }) {
  switch (tab) {
    case "database":
      return <DatabasePanel />;
    case "logs":
      return <AuditingPanel />;
    case "users":
      return <UsersPanel />;
    case "alerts":
      return (
        <>
          <AlertNotificationPrefsPanel />
          {canEditAlertThresholds() ? <AlertThresholdsPanel /> : null}
        </>
      );
    case "monitoring":
      return <MonitoringSettingsPanel />;
    case "appearance":
      return <AppearancePanel />;
    case "connection":
      return <ConnectionPanel />;
    case "telegram":
      return (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <TelegramPanel id="monitoring" title="Monitorização (alertas)" />
          <TelegramPanel id="reports" title="Relatórios" />
        </div>
      );
    case "olt":
      return <OltVendorsPanel />;
    case "mikrotik":
      return <MikrotikSettingsPanel />;
    case "switch":
      return <SwitchSettingsPanel />;
    case "bng":
      return <BngCollectionPanel />;
    case "bgp":
      return (
        <>
          <BgpSnmpProfilesPanel />
          <BgpUplinkCarriersPanel />
        </>
      );
    case "fleet":
      return <FleetSettingsPanel />;
    case "automation":
      return <ScheduledReportsPanel />;
    default:
      return null;
  }
}

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const visibleTabs = useMemo(() => SETTINGS_TABS.filter(canSeeSettingsTab), []);
  const [tab, setTab] = useState<SettingsTab>(() => {
    const raw = searchParams.get("tab");
    if (SETTINGS_TABS.includes(raw as SettingsTab) && canSeeSettingsTab(raw as SettingsTab)) {
      return raw as SettingsTab;
    }
    return visibleTabs[0] ?? "appearance";
  });

  function selectTab(next: SettingsTab) {
    setTab(next);
    const params = new URLSearchParams(searchParams);
    params.set("tab", next);
    setSearchParams(params, { replace: true });
  }

  return (
    <>
      <h1>Configurações</h1>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Base de dados, usuários, credenciais de rede, Telegram (alertas e relatórios), perfis OLT por marca/modelo, coleta MikroTik/Switch/BNG e relatórios automáticos.
      </p>
      <OverflowTabs
        items={visibleTabs.map((k) => ({ key: k, label: SETTINGS_TAB_LABELS[k], icon: SETTINGS_TAB_ICONS[k] }))}
        active={tab}
        onSelect={selectTab}
      />
      <Suspense fallback={<p style={{ color: "var(--muted)" }}>A carregar…</p>}>
        <TabContent tab={tab} />
      </Suspense>
    </>
  );
}

import { NavLink, Outlet } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import type { LucideIcon } from "lucide-react";
import {
  Bolt,
  CalendarDays,
  ChartBarBig,
  ChartPie,
  ClockCheck,
  Cpu,
  MapPin,
  MonitorSmartphone,
  ShieldCheck,
  TriangleAlert,
  UsersRound,
  Warehouse,
  Wrench,
  Zap,
} from "lucide-react";
import { clearSession, getStoredUserDisplayLabel, isAdminUser } from "../lib/auth";
import { apiFetch } from "../lib/api";
import { OnuReportGlobalToast } from "../components/OnuReportGlobalToast";
import { queryKeys } from "../lib/queryKeys";

const nav: { to: string; label: string; icons: LucideIcon[] }[] = [
  { to: "/dashboard", label: "Dashboard", icons: [ChartPie] },
  { to: "/monitoring", label: "Monitoramento", icons: [ShieldCheck] },
  { to: "/realtime", label: "Tempo real", icons: [ClockCheck] },
  { to: "/metrics", label: "Métricas", icons: [ChartBarBig] },
  { to: "/pops", label: "POPs", icons: [Warehouse] },
  { to: "/devices", label: "Equipamentos", icons: [MonitorSmartphone] },
  { to: "/commercial", label: "Base comercial", icons: [UsersRound] },
  { to: "/alerts", label: "Alertas", icons: [TriangleAlert] },
  { to: "/map", label: "Mapa", icons: [MapPin] },
  { to: "/tools", label: "Ferramentas", icons: [Wrench] },
  { to: "/olt", label: "OLT", icons: [Zap] },
  { to: "/mikrotik", label: "Mikrotik", icons: [Cpu] },
  { to: "/events", label: "Eventos", icons: [CalendarDays] },
  { to: "/settings", label: "Configurações", icons: [Bolt] },
];

const ICON_SZ = 16;
const ICON_STROKE = 2;

export function ShellLayout() {
  const monState = useQuery({
    queryKey: queryKeys.monStateGlobal,
    queryFn: () =>
      apiFetch<{
        is_running?: boolean;
        current_activity?: string | null;
        activity_started_at?: string | null;
        activity_updated_at?: string | null;
        last_activity?: string | null;
        last_activity_finished_at?: string | null;
      }>("/api/v1/monitoring/state"),
    refetchInterval: 1000,
    refetchOnWindowFocus: true,
    staleTime: 0,
  });
  const activity = (monState.data?.current_activity ?? "").trim();
  const running = !!monState.data?.is_running;
  const lastFinishedMs = monState.data?.last_activity_finished_at ? Date.parse(monState.data.last_activity_finished_at) : NaN;
  const showRecentFinished = Number.isFinite(lastFinishedMs) && Date.now() - (lastFinishedMs as number) <= 5000;
  const showIndicator = !!activity || !!showRecentFinished;
  let indicatorText = running ? "Monitoramento ativo (em espera)" : "Monitoramento parado";
  if (activity) {
    indicatorText = activity;
  } else if (monState.data?.last_activity && showRecentFinished) {
    indicatorText = `Finalizado: ${monState.data.last_activity}`;
  }

  return (
    <div className="layout">
      <OnuReportGlobalToast />
      {showIndicator ? (
        <div className={`runtime-indicator ${activity ? "runtime-indicator--busy" : ""}`} title="Atividade atual do sistema">
          <span className="runtime-indicator__dot" />
          <span className="runtime-indicator__txt">{indicatorText}</span>
        </div>
      ) : null}
      <aside className="sidebar">
        <div className="sidebar__brand">NetQuasar</div>
        <nav>
          {(isAdminUser() ? nav : nav.filter((n) => n.to !== "/settings")).map((n) => (
            <NavLink key={n.to} to={n.to} className={({ isActive }) => (isActive ? "active" : "")} title={n.label}>
              <span
                className={`sidebar__nav-icon${n.icons.length > 1 ? " sidebar__nav-icon--pair" : ""}`}
                aria-hidden
              >
                {n.icons.map((Icon, i) => (
                  <Icon key={i} size={ICON_SZ} strokeWidth={ICON_STROKE} className="sidebar__nav-icon__svg" />
                ))}
              </span>
              <span className="sidebar__nav-label">{n.label}</span>
            </NavLink>
          ))}
        </nav>
        <div style={{ marginTop: "auto", padding: "0.75rem 1rem", fontSize: 12, color: "var(--muted)" }}>
          <div style={{ fontWeight: 600, marginBottom: 6 }} title="Sessão actual">
            {getStoredUserDisplayLabel() || "Utilizador"}
          </div>
          <button type="button" className="btn" style={{ marginTop: 4, width: "100%" }} onClick={() => { clearSession(); window.location.href = "/login"; }}>
            Sair
          </button>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
  );
}

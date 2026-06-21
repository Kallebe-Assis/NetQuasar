import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { LucideIcon } from "lucide-react";
import {
  Bolt,
  FileBarChart,
  ChevronLeft,
  ChevronRight,
  Menu,
  Plug,
  ChartPie,
  ClockCheck,
  Cpu,
  MapPin,
  MonitorSmartphone,
  Network,
  ShieldCheck,
  TriangleAlert,
  UsersRound,
  Warehouse,
  Wrench,
  X,
  Zap,
} from "lucide-react";
import { clearSession, getAuthToken, getStoredUserDisplayLabel, isAdminUser } from "../lib/auth";
import { prefetchDashboard } from "../lib/dashboardCache";
import { apiFetch } from "../lib/api";
import { OnuReportGlobalToast } from "../components/OnuReportGlobalToast";
import { AppToastProvider } from "../lib/appToast";
import { queryKeys } from "../lib/queryKeys";
import { APP_ROUTES } from "./routes";

const SIDEBAR_COLLAPSED_KEY = "netquasar.sidebar.collapsed";
const MOBILE_NAV_MQ = "(max-width: 1023px)";

const nav: { to: string; label: string; icons: LucideIcon[] }[] = [
  { to: APP_ROUTES.dashboard, label: "Dashboard", icons: [ChartPie] },
  { to: APP_ROUTES.monitoring, label: "Monitoramento", icons: [ShieldCheck] },
  { to: APP_ROUTES.realtime, label: "Tempo real", icons: [ClockCheck] },
  { to: APP_ROUTES.integrations, label: "Integrações", icons: [Plug] },
  { to: APP_ROUTES.pops, label: "POPs", icons: [Warehouse] },
  { to: APP_ROUTES.devices, label: "Equipamentos", icons: [MonitorSmartphone] },
  { to: APP_ROUTES.commercial, label: "Clientes", icons: [UsersRound] },
  { to: APP_ROUTES.connections, label: "Conexões", icons: [Network] },
  { to: APP_ROUTES.alerts, label: "Alertas", icons: [TriangleAlert] },
  { to: APP_ROUTES.map, label: "Mapa", icons: [MapPin] },
  { to: APP_ROUTES.tools, label: "Ferramentas", icons: [Wrench] },
  { to: APP_ROUTES.olt, label: "OLT", icons: [Zap] },
  { to: APP_ROUTES.mikrotik, label: "Mikrotik", icons: [Cpu] },
  { to: APP_ROUTES.bng, label: "BNG", icons: [Network] },
  { to: APP_ROUTES.reports, label: "Relatórios", icons: [FileBarChart] },
  { to: APP_ROUTES.settings, label: "Configurações", icons: [Bolt] },
];

const ICON_SZ = 16;
const ICON_STROKE = 2;

function useIsMobileNav() {
  const [mobile, setMobile] = useState(() =>
    typeof window !== "undefined" ? window.matchMedia(MOBILE_NAV_MQ).matches : false,
  );

  useEffect(() => {
    const mq = window.matchMedia(MOBILE_NAV_MQ);
    const onChange = () => setMobile(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  return mobile;
}

function pageTitleForPath(pathname: string, items: typeof nav): string {
  const exact = items.find((n) => n.to === pathname);
  if (exact) return exact.label;
  const sorted = [...items].sort((a, b) => b.to.length - a.to.length);
  const prefix = sorted.find((n) => pathname.startsWith(n.to + "/") || pathname === n.to);
  return prefix?.label ?? "NetQuasar";
}

export function ShellLayout() {
  const qc = useQueryClient();
  const location = useLocation();
  const isMobileNav = useIsMobileNav();
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => {
    try {
      return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
    } catch {
      return false;
    }
  });

  const closeMobileNav = useCallback(() => setMobileNavOpen(false), []);

  useEffect(() => {
    closeMobileNav();
  }, [location.pathname, closeMobileNav]);

  useEffect(() => {
    if (!isMobileNav) {
      setMobileNavOpen(false);
    }
  }, [isMobileNav]);

  useEffect(() => {
    if (!isMobileNav || !mobileNavOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [isMobileNav, mobileNavOpen]);

  useEffect(() => {
    try {
      localStorage.setItem(SIDEBAR_COLLAPSED_KEY, sidebarCollapsed ? "1" : "0");
    } catch {
      /* ignore */
    }
  }, [sidebarCollapsed]);

  useEffect(() => {
    if (getAuthToken()) {
      void prefetchDashboard(qc);
    }
  }, [qc]);

  const monState = useQuery({
    queryKey: queryKeys.monState,
    queryFn: () =>
      apiFetch<{
        is_running?: boolean;
        current_activity?: string | null;
        activity_started_at?: string | null;
        activity_updated_at?: string | null;
        last_activity?: string | null;
        last_activity_finished_at?: string | null;
        runtime_updated_at?: string | null;
        last_alerts_change_at?: string | null;
        last_telemetry_cycle_at?: string | null;
        last_latency_cycle_at?: string | null;
        last_interface_snapshot_cycle_at?: string | null;
        last_olt_if_derived_cycle_at?: string | null;
      }>("/api/v1/monitoring/state"),
    refetchInterval: 1500,
    refetchOnWindowFocus: true,
    staleTime: 1000,
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

  const navItems = isAdminUser() ? nav : nav.filter((n) => n.to !== APP_ROUTES.settings);
  const pageTitle = useMemo(() => pageTitleForPath(location.pathname, navItems), [location.pathname, navItems]);

  const layoutClass = [
    "layout",
    !isMobileNav && sidebarCollapsed ? "layout--sidebar-collapsed" : "",
    isMobileNav && mobileNavOpen ? "layout--mobile-nav-open" : "",
  ]
    .filter(Boolean)
    .join(" ");

  const sidebarClass = [
    "sidebar",
    !isMobileNav && sidebarCollapsed ? "sidebar--collapsed" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <AppToastProvider>
    <div className={layoutClass}>
      <header className="mobile-topbar" aria-label="Barra de navegação móvel">
        <button
          type="button"
          className="mobile-topbar__menu"
          aria-label={mobileNavOpen ? "Fechar menu" : "Abrir menu"}
          aria-expanded={mobileNavOpen}
          onClick={() => setMobileNavOpen((v) => !v)}
        >
          {mobileNavOpen ? <X size={22} strokeWidth={2} /> : <Menu size={22} strokeWidth={2} />}
        </button>
        <span className="mobile-topbar__title">{pageTitle}</span>
        <span className="mobile-topbar__brand">NetQuasar</span>
      </header>

      {isMobileNav && mobileNavOpen ? (
        <button
          type="button"
          className="sidebar-backdrop"
          aria-label="Fechar menu"
          onClick={closeMobileNav}
        />
      ) : null}

      <OnuReportGlobalToast />
      {showIndicator ? (
        <div className={`runtime-indicator ${activity ? "runtime-indicator--busy" : ""}`} title="Atividade atual do sistema">
          <span className="runtime-indicator__dot" />
          <span className="runtime-indicator__txt">{indicatorText}</span>
        </div>
      ) : null}
      <aside className={sidebarClass} aria-label="Menu principal">
        <div className="sidebar__head">
          <div className="sidebar__brand">NetQuasar</div>
          {!isMobileNav ? (
            <button
              type="button"
              className="sidebar__collapse-btn"
              aria-label={sidebarCollapsed ? "Expandir menu" : "Minimizar menu"}
              title={sidebarCollapsed ? "Expandir menu" : "Minimizar menu"}
              onClick={() => setSidebarCollapsed((v) => !v)}
            >
              {sidebarCollapsed ? <ChevronRight size={18} /> : <ChevronLeft size={18} />}
            </button>
          ) : null}
        </div>
        <nav>
          {navItems.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.to === APP_ROUTES.integrations}
              className={({ isActive }) => (isActive ? "active" : "")}
              title={n.label}
              onClick={closeMobileNav}
            >
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
        <div className="sidebar__foot">
          <div className="sidebar__user" title="Sessão actual">
            {getStoredUserDisplayLabel() || "Usuário"}
          </div>
          <button type="button" className="btn sidebar__logout" onClick={() => { clearSession(); window.location.href = APP_ROUTES.login; }}>
            Sair
          </button>
        </div>
      </aside>
      <main className="main">
        <Outlet />
      </main>
    </div>
    </AppToastProvider>
  );
}

import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { LucideIcon } from "lucide-react";
import {
  History,
  Bolt,
  CircleHelp,
  FileBarChart,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Menu,
  Plug,
  ChartPie,
  ClockCheck,
  Component,
  Cpu,
  Fuel,
  MapPin,
  MonitorSmartphone,
  Network,
  ShieldCheck,
  TriangleAlert,
  Truck,
  UserRoundKey,
  UsersRound,
  Warehouse,
  Wrench,
  X,
  Zap,
} from "lucide-react";
import { clearSession, getAuthToken, getStoredUserDisplayLabel, getStoredUserPermissionsKey, can, isAdminUser } from "../lib/auth";
import { prefetchStaticPages } from "../lib/prefetchStaticPages";
import { apiFetch } from "../lib/api";
import { OnuReportGlobalToast } from "../components/OnuReportGlobalToast";
import { AppToastProvider } from "../lib/appToast";
import { queryKeys } from "../lib/queryKeys";
import { ROUTE_VIEW_PERMISSION } from "../lib/permissions";
import { APP_ROUTES } from "./routes";

const SIDEBAR_COLLAPSED_KEY = "netquasar.sidebar.collapsed";
const MOBILE_NAV_MQ = "(max-width: 1023px)";

type NavLeaf = { kind: "link"; to: string; label: string; icons: LucideIcon[] };
type NavGroup = {
  kind: "group";
  id: string;
  label: string;
  icons: LucideIcon[];
  children: Array<{ to: string; label: string; icons: LucideIcon[] }>;
};
type NavEntry = NavLeaf | NavGroup;

const nav: NavEntry[] = [
  { kind: "link", to: APP_ROUTES.dashboard, label: "Dashboard", icons: [ChartPie] },
  { kind: "link", to: APP_ROUTES.monitoring, label: "Monitoramento", icons: [ShieldCheck] },
  { kind: "link", to: APP_ROUTES.realtime, label: "Tempo real", icons: [ClockCheck] },
  { kind: "link", to: APP_ROUTES.integrations, label: "Integrações", icons: [Plug] },
  { kind: "link", to: APP_ROUTES.pops, label: "Localidades", icons: [Warehouse] },
  {
    kind: "group",
    id: "equipamentos",
    label: "Equipamentos",
    icons: [MonitorSmartphone],
    children: [
      { to: APP_ROUTES.devices, label: "Geral", icons: [MonitorSmartphone] },
      { to: APP_ROUTES.mikrotik, label: "Mikrotik", icons: [Cpu] },
      { to: APP_ROUTES.olt, label: "OLT", icons: [Zap] },
      { to: APP_ROUTES.bng, label: "BNG", icons: [UserRoundKey] },
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
      { to: APP_ROUTES.connections, label: "Elementos", icons: [Component] },
    ],
  },
  { kind: "link", to: APP_ROUTES.alerts, label: "Alertas", icons: [TriangleAlert] },
  { kind: "link", to: APP_ROUTES.events, label: "Eventos", icons: [History] },
  { kind: "link", to: APP_ROUTES.tools, label: "Ferramentas", icons: [Wrench] },
  {
    kind: "group",
    id: "frota",
    label: "Frota",
    icons: [Truck],
    children: [
      { to: APP_ROUTES.fleetDashboard, label: "Dashboard", icons: [ChartPie] },
      { to: APP_ROUTES.fleetVehicles, label: "Veículos", icons: [Truck] },
      { to: APP_ROUTES.fleetDrivers, label: "Motoristas", icons: [UsersRound] },
      { to: APP_ROUTES.fleetFuelings, label: "Abastecimentos", icons: [Fuel] },
      { to: APP_ROUTES.fleetFuels, label: "Combustíveis", icons: [Fuel] },
      { to: APP_ROUTES.fleetStations, label: "Postos", icons: [MapPin] },
      { to: APP_ROUTES.fleetCostCenters, label: "Centros de custo", icons: [Warehouse] },
      { to: APP_ROUTES.fleetAlerts, label: "Alertas", icons: [TriangleAlert] },
      { to: APP_ROUTES.fleetReports, label: "Relatórios", icons: [FileBarChart] },
    ],
  },
  { kind: "link", to: APP_ROUTES.reports, label: "Relatórios", icons: [FileBarChart] },
  { kind: "link", to: APP_ROUTES.settings, label: "Configurações", icons: [Bolt] },
];

const ICON_SZ = 16;
const ICON_SZ_MOBILE = 14;
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

function canViewRoute(to: string): boolean {
  const perm = ROUTE_VIEW_PERMISSION[to];
  if (!perm) return true;
  if (to === APP_ROUTES.settings) {
    return can("settings.view") || can("settings.users") || can("settings.permissions") || isAdminUser();
  }
  return can(perm) || isAdminUser();
}

function filterNav(entries: NavEntry[]): NavEntry[] {
  const out: NavEntry[] = [];
  for (const n of entries) {
    if (n.kind === "link") {
      if (canViewRoute(n.to)) out.push(n);
      continue;
    }
    const children = n.children.filter((c) => canViewRoute(c.to));
    if (children.length > 0) out.push({ ...n, children });
  }
  return out;
}

function pageTitleForPath(pathname: string, items: NavEntry[]): string {
  for (const n of items) {
    if (n.kind === "link" && n.to === pathname) return n.label;
    if (n.kind === "group") {
      const child = n.children.find((c) => c.to === pathname || pathname.startsWith(c.to + "/"));
      if (child) return child.label;
    }
  }
  const flat = items.flatMap((n) => (n.kind === "link" ? [n] : n.children));
  const sorted = [...flat].sort((a, b) => b.to.length - a.to.length);
  const prefix = sorted.find((n) => pathname.startsWith(n.to + "/") || pathname === n.to);
  return prefix?.label ?? "NetQuasar";
}

function NavIcons({ icons, mobile }: { icons: LucideIcon[]; mobile: boolean }) {
  return (
    <span className={`sidebar__nav-icon${icons.length > 1 ? " sidebar__nav-icon--pair" : ""}`} aria-hidden>
      {icons.map((Icon, i) => (
        <Icon key={i} size={mobile ? ICON_SZ_MOBILE : ICON_SZ} strokeWidth={ICON_STROKE} className="sidebar__nav-icon__svg" />
      ))}
    </span>
  );
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
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({});

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
      void prefetchStaticPages(qc);
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

  const permissionsKey = getStoredUserPermissionsKey();
  const navItems = useMemo(() => filterNav(nav), [permissionsKey]);
  const pageTitle = useMemo(() => pageTitleForPath(location.pathname, navItems), [location.pathname, navItems]);

  useEffect(() => {
    setOpenGroups((prev) => {
      const next = { ...prev };
      for (const n of navItems) {
        if (n.kind !== "group") continue;
        const active = n.children.some((c) => location.pathname === c.to || location.pathname.startsWith(c.to + "/"));
        if (active) next[n.id] = true;
      }
      return next;
    });
  }, [location.pathname, navItems]);

  const layoutClass = [
    "layout",
    !isMobileNav && sidebarCollapsed ? "layout--sidebar-collapsed" : "",
    isMobileNav && mobileNavOpen ? "layout--mobile-nav-open" : "",
  ]
    .filter(Boolean)
    .join(" ");

  const sidebarClass = ["sidebar", !isMobileNav && sidebarCollapsed ? "sidebar--collapsed" : ""].filter(Boolean).join(" ");

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
          <button type="button" className="sidebar-backdrop" aria-label="Fechar menu" onClick={closeMobileNav} />
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
          <div className="sidebar__nav-scroll">
            <nav>
              {navItems.map((n) => {
                if (n.kind === "link") {
                  return (
                    <NavLink
                      key={n.to}
                      to={n.to}
                      end={n.to === APP_ROUTES.integrations}
                      className={({ isActive }) => (isActive ? "active" : "")}
                      title={n.label}
                      onClick={closeMobileNav}
                    >
                      <NavIcons icons={n.icons} mobile={isMobileNav} />
                      <span className="sidebar__nav-label">{n.label}</span>
                    </NavLink>
                  );
                }

                const groupActive = n.children.some(
                  (c) => location.pathname === c.to || location.pathname.startsWith(c.to + "/"),
                );
                const expanded = !!openGroups[n.id] || groupActive;

                return (
                  <div
                    key={n.id}
                    className={`sidebar__group${groupActive ? " sidebar__group--active" : ""}${expanded ? " is-expanded" : ""}`}
                  >
                    <button
                      type="button"
                      className={`sidebar__group-btn${expanded ? " is-open" : ""}${groupActive ? " is-active" : ""}`}
                      title={n.label}
                      aria-expanded={expanded}
                      onClick={() => {
                        if (sidebarCollapsed && !isMobileNav) {
                          setSidebarCollapsed(false);
                          setOpenGroups((p) => ({ ...p, [n.id]: true }));
                          return;
                        }
                        setOpenGroups((p) => ({ ...p, [n.id]: !expanded }));
                      }}
                    >
                      <NavIcons icons={n.icons} mobile={isMobileNav} />
                      <span className="sidebar__nav-label">{n.label}</span>
                      <ChevronDown size={14} className="sidebar__group-chevron" aria-hidden />
                    </button>
                    {!(sidebarCollapsed && !isMobileNav) ? (
                      <div className={`sidebar__submenu${expanded ? " is-open" : ""}`} aria-hidden={!expanded}>
                        <div className="sidebar__submenu-inner">
                          {n.children.map((c) => (
                            <NavLink
                              key={c.to}
                              to={c.to}
                              tabIndex={expanded ? undefined : -1}
                              className={({ isActive }) => `sidebar__sublink${isActive ? " active" : ""}`}
                              title={c.label}
                              onClick={closeMobileNav}
                            >
                              <NavIcons icons={c.icons} mobile={isMobileNav} />
                              <span className="sidebar__nav-label">{c.label}</span>
                            </NavLink>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </nav>
          </div>
          <div className="sidebar__foot">
            <div className="sidebar__user" title="Sessão actual">
              {getStoredUserDisplayLabel() || "Usuário"}
            </div>
            <div className="sidebar__foot-actions">
              <NavLink
                to={APP_ROUTES.about}
                className={({ isActive }) => `btn btn--icon btn--icon-menu sidebar__about${isActive ? " btn--primary" : ""}`}
                title="Sobre o NetQuasar"
                aria-label="Sobre o NetQuasar"
              >
                <CircleHelp size={18} aria-hidden />
              </NavLink>
              <button
                type="button"
                className="btn sidebar__logout"
                onClick={() => {
                  clearSession();
                  window.location.href = APP_ROUTES.login;
                }}
              >
                Sair
              </button>
            </div>
          </div>
        </aside>
        <main className="main">
          <Outlet />
        </main>
      </div>
    </AppToastProvider>
  );
}

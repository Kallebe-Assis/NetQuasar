import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState, type MouseEvent as ReactMouseEvent } from "react";
import type { LucideIcon } from "lucide-react";
import { ChevronDown, ChevronLeft, ChevronRight, CircleHelp, Menu, X } from "lucide-react";
import { clearSession, getAuthToken, getStoredUserDisplayLabel, getStoredUserPermissionsKey, can, isAdminUser } from "../lib/auth";
import { prefetchStaticPages } from "../lib/prefetchStaticPages";
import { installMobileTableCards } from "../lib/mobileTables";
import { apiFetch } from "../lib/api";
import { AlertNotificationWatcher } from "../components/AlertNotificationWatcher";
import { OnuReportGlobalToast } from "../components/OnuReportGlobalToast";
import { ConfirmModal } from "../components/ConfirmModal";
import { SidebarSearch } from "../components/SidebarSearch";
import { AppToastProvider } from "../lib/appToast";
import { queryKeys } from "../lib/queryKeys";
import { ROUTE_VIEW_PERMISSION } from "../lib/permissions";
import { getUnsavedGuard } from "../lib/unsavedChangesGuard";
import { APP_ROUTES } from "./routes";
import { NAV_CONFIG, type NavEntryConfig, type SubmoduleEntry } from "./navConfig";

const SIDEBAR_COLLAPSED_KEY = "netquasar.sidebar.collapsed";
const MOBILE_NAV_MQ = "(max-width: 1023px)";

const nav: NavEntryConfig[] = NAV_CONFIG;

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
    return true;
  }
  return can(perm) || isAdminUser();
}

function filterNav(entries: NavEntryConfig[]): NavEntryConfig[] {
  const out: NavEntryConfig[] = [];
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

function normalizeSearchText(s: string): string {
  return s
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase();
}

function labelMatches(label: string, query: string): boolean {
  return normalizeSearchText(label).includes(query);
}

/**
 * Filtra a árvore de navegação por texto — oculta o que não corresponde ao nome do módulo nem de
 * nenhum dos seus submódulos. Quando só um submódulo corresponde, mantém o módulo/grupo pai
 * (para dar contexto) mas restringe a lista de submódulos mostrados aos que correspondem.
 */
function filterNavByQuery(entries: NavEntryConfig[], rawQuery: string): NavEntryConfig[] {
  const query = normalizeSearchText(rawQuery.trim());
  if (!query) return entries;
  const out: NavEntryConfig[] = [];
  for (const n of entries) {
    if (n.kind === "link") {
      const selfMatch = labelMatches(n.label, query);
      const matchedSubs = (n.submodules ?? []).filter((sm) => labelMatches(sm.label, query));
      if (selfMatch || matchedSubs.length > 0) {
        out.push({ ...n, submodules: selfMatch ? n.submodules : matchedSubs });
      }
      continue;
    }
    const groupSelfMatch = labelMatches(n.label, query);
    const filteredChildren = n.children
      .map((c) => {
        const childSelfMatch = labelMatches(c.label, query);
        const matchedSubs = (c.submodules ?? []).filter((sm) => labelMatches(sm.label, query));
        if (groupSelfMatch || childSelfMatch || matchedSubs.length > 0) {
          return { ...c, submodules: childSelfMatch || groupSelfMatch ? c.submodules : matchedSubs };
        }
        return null;
      })
      .filter((c): c is NonNullable<typeof c> => c !== null);
    if (groupSelfMatch || filteredChildren.length > 0) {
      out.push({ ...n, children: filteredChildren });
    }
  }
  return out;
}

function pageTitleForPath(pathname: string, items: NavEntryConfig[]): string {
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

/**
 * Uma linha do menu (link normal ou grupo) — se tiver `submodules`, o clique no texto continua a
 * navegar para `to` (abre na primeira aba, comportamento igual a antes); um chevron separado
 * expande/recolhe a lista de submódulos (?tab=<valor> na mesma rota). Reaproveitada tanto para
 * entradas de topo (ex.: Localidades) como para filhos de um NavGroup (ex.: OLT dentro de
 * "Equipamentos") — por isso os submódulos ficam um nível mais indentados nesse segundo caso via
 * o mesmo `.sidebar__submenu`/`.sidebar__sublink` reaplicado.
 */
function NavRow({
  to,
  label,
  icons,
  submodules,
  isMobileNav,
  closeMobileNav,
  location,
  openGroups,
  setOpenGroups,
  asSubItem,
  forceExpanded,
}: {
  to: string;
  label: string;
  icons: LucideIcon[];
  submodules?: SubmoduleEntry[];
  isMobileNav: boolean;
  closeMobileNav: () => void;
  location: { pathname: string; search: string };
  openGroups: Record<string, boolean>;
  setOpenGroups: (fn: (p: Record<string, boolean>) => Record<string, boolean>) => void;
  /** true quando é filho de um NavGroup (ex.: OLT dentro de "Equipamentos") — usa o mesmo
   * `.sidebar__sublink` dos irmãos sem submódulos, em vez do `.sidebar a` de topo, senão a lista
   * fica desalinhada (padding/indentação diferentes entre irmãos no mesmo nível). */
  asSubItem?: boolean;
  /** true durante uma pesquisa activa no menu — mostra os submódulos já expandidos, para que os
   * que sobreviveram ao filtro fiquem visíveis sem precisar de mais um clique. */
  forceExpanded?: boolean;
}) {
  const baseLinkClass = asSubItem ? "sidebar__sublink" : "";
  if (!submodules || submodules.length === 0) {
    return (
      <NavLink
        to={to}
        className={({ isActive }) => `${baseLinkClass}${isActive ? `${baseLinkClass ? " " : ""}active` : ""}`}
        title={label}
        onClick={closeMobileNav}
      >
        <NavIcons icons={icons} mobile={isMobileNav} />
        <span className="sidebar__nav-label">{label}</span>
      </NavLink>
    );
  }

  const onThisPage = location.pathname === to;
  const expanded = !!openGroups[to] || onThisPage || !!forceExpanded;
  const curTab = onThisPage ? new URLSearchParams(location.search).get("tab") : null;

  return (
    <div className={`sidebar__group${onThisPage ? " sidebar__group--active" : ""}${expanded ? " is-expanded" : ""}`}>
      <div className={`sidebar__group-row${asSubItem ? " sidebar__group-row--sub" : ""}`}>
        <NavLink
          to={to}
          end
          className={({ isActive }) => `${baseLinkClass}${isActive ? `${baseLinkClass ? " " : ""}active` : ""}`}
          title={label}
          onClick={closeMobileNav}
        >
          <NavIcons icons={icons} mobile={isMobileNav} />
          <span className="sidebar__nav-label">{label}</span>
        </NavLink>
        <button
          type="button"
          className="sidebar__group-chevron-btn"
          aria-label={expanded ? "Recolher submódulos" : "Expandir submódulos"}
          aria-expanded={expanded}
          onClick={() => setOpenGroups((p) => ({ ...p, [to]: !expanded }))}
        >
          <ChevronDown size={14} className={`sidebar__group-chevron${expanded ? " is-open" : ""}`} aria-hidden />
        </button>
      </div>
      <div className={`sidebar__submenu${expanded ? " is-open" : ""}`} aria-hidden={!expanded}>
        <div className="sidebar__submenu-inner">
          {submodules.map((sm, i) => {
            const active = onThisPage && (curTab ? curTab === sm.tab : i === 0);
            return (
              <Link
                key={sm.tab}
                to={`${to}?tab=${sm.tab}`}
                className={active ? "sidebar__sublink active" : "sidebar__sublink"}
                title={sm.label}
                onClick={closeMobileNav}
              >
                <span className="sidebar__nav-label">{sm.label}</span>
              </Link>
            );
          })}
        </div>
      </div>
    </div>
  );
}

export function ShellLayout() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const isMobileNav = useIsMobileNav();
  // "Alterações não salvas" — Topologia (geral e 2D do POP) regista-se em setUnsavedGuard
  // (lib/unsavedChangesGuard.ts) enquanto tiver edições por salvar. Aqui, um único
  // onClickCapture no wrapper de todo o layout intercepta qualquer clique em link (menu lateral
  // ou dentro da própria página, ex.: o botão "Voltar") — cobre tudo com um só sítio, em vez de
  // ter de alterar cada NavLink/Link individualmente.
  const [pendingHref, setPendingHref] = useState<string | null>(null);
  const [unsavedBusy, setUnsavedBusy] = useState(false);
  const handleNavClickCapture = useCallback((e: ReactMouseEvent) => {
    const guard = getUnsavedGuard();
    if (!guard?.dirty) return;
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const anchor = (e.target as HTMLElement).closest("a[href]") as HTMLAnchorElement | null;
    if (!anchor || anchor.target === "_blank" || anchor.hasAttribute("download")) return;
    let url: URL;
    try {
      url = new URL(anchor.href, window.location.href);
    } catch {
      return;
    }
    if (url.origin !== window.location.origin) return;
    const dest = url.pathname + url.search + url.hash;
    if (dest === location.pathname + location.search + location.hash) return;
    e.preventDefault();
    e.stopPropagation();
    setPendingHref(dest);
  }, [location.pathname, location.search, location.hash]);
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
  const [navQuery, setNavQuery] = useState("");
  const navItemsAll = useMemo(() => filterNav(nav), [permissionsKey]);
  const navItems = useMemo(() => filterNavByQuery(navItemsAll, navQuery), [navItemsAll, navQuery]);
  const searching = normalizeSearchText(navQuery.trim()).length > 0;
  const pageTitle = useMemo(() => pageTitleForPath(location.pathname, navItemsAll), [location.pathname, navItemsAll]);

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

  // Telefone: tabelas com muitas colunas viram cartões (evita rolagem horizontal em todas as telas).
  useEffect(() => installMobileTableCards(), []);

  const sidebarClass = ["sidebar", !isMobileNav && sidebarCollapsed ? "sidebar--collapsed" : ""].filter(Boolean).join(" ");

  return (
    <AppToastProvider>
      <div className={layoutClass} onClickCapture={handleNavClickCapture}>
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
        <AlertNotificationWatcher />
        {showIndicator ? (
          <div className={`runtime-indicator ${activity ? "runtime-indicator--busy" : ""}`} title="Atividade atual do sistema">
            <span className="runtime-indicator__dot" />
            <span className="runtime-indicator__txt">{indicatorText}</span>
          </div>
        ) : null}
        <aside className={sidebarClass} aria-label="Menu principal">
          <div className="sidebar__head">
            <div className="sidebar__brand">
              <img src="/Logo-NetQuasar II.png" alt="" className="sidebar__brand-logo" aria-hidden />
              <span>NetQuasar</span>
            </div>
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
          {!isMobileNav && sidebarCollapsed ? null : <SidebarSearch query={navQuery} onQueryChange={setNavQuery} />}
          <div className="sidebar__nav-scroll">
            <nav>
              {navItems.map((n) => {
                if (n.kind === "link") {
                  if (n.to === APP_ROUTES.integrations && !n.submodules) {
                    // Único caso a precisar de `end` explícito sem NavRow (Integrações tem sub-rotas
                    // próprias, não `?tab=`, e não deve marcar-se activo para elas).
                    return (
                      <NavLink
                        key={n.to}
                        to={n.to}
                        end
                        className={({ isActive }) => (isActive ? "active" : "")}
                        title={n.label}
                        onClick={closeMobileNav}
                      >
                        <NavIcons icons={n.icons} mobile={isMobileNav} />
                        <span className="sidebar__nav-label">{n.label}</span>
                      </NavLink>
                    );
                  }
                  return (
                    <NavRow
                      key={n.to}
                      to={n.to}
                      label={n.label}
                      icons={n.icons}
                      submodules={n.submodules}
                      isMobileNav={isMobileNav}
                      closeMobileNav={closeMobileNav}
                      location={location}
                      openGroups={openGroups}
                      setOpenGroups={setOpenGroups}
                      forceExpanded={searching}
                    />
                  );
                }

                const groupActive = n.children.some(
                  (c) => location.pathname === c.to || location.pathname.startsWith(c.to + "/"),
                );
                const expanded = !!openGroups[n.id] || groupActive || searching;

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
                            <NavRow
                              key={c.to}
                              to={c.to}
                              label={c.label}
                              icons={c.icons}
                              submodules={c.submodules}
                              isMobileNav={isMobileNav}
                              closeMobileNav={closeMobileNav}
                              location={location}
                              openGroups={openGroups}
                              setOpenGroups={setOpenGroups}
                              asSubItem
                              forceExpanded={searching}
                            />
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
      <ConfirmModal
        open={pendingHref != null}
        title="Alterações não salvas"
        message="Esta tela tem alterações que ainda não foram salvas. O que deseja fazer?"
        cancelLabel="Cancelar"
        secondaryLabel="Salvar e sair"
        confirmLabel="Sair sem salvar"
        danger
        busy={unsavedBusy}
        onCancel={() => setPendingHref(null)}
        onConfirm={() => {
          const href = pendingHref;
          setPendingHref(null);
          if (href) navigate(href);
        }}
        onSecondary={async () => {
          const guard = getUnsavedGuard();
          const href = pendingHref;
          if (!guard || !href) return;
          setUnsavedBusy(true);
          try {
            await guard.save();
            setPendingHref(null);
            navigate(href);
          } catch {
            // erro já reportado por toast na própria página (save() das telas de Topologia já
            // mostra toastErr) — só não navega, o utilizador decide de novo no mesmo modal.
          } finally {
            setUnsavedBusy(false);
          }
        }}
      />
    </AppToastProvider>
  );
}

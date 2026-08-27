import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { lazy, Suspense, useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { ConfirmModal } from "../components/ConfirmModal";
import { DashboardPageLoader } from "../components/DashboardPageLoader";
import { GlobalSearchBar } from "../components/GlobalSearchBar";
import { InfoHint } from "../components/InfoHint";
import { apiFetch } from "../lib/api";
import {
  DASHBOARD_DEFAULT_DAYS,
  DASHBOARD_GC_MS,
  DASHBOARD_STALE_MS,
  dashboardAnalyticsKey,
  dashboardOltCapacityKey,
  dashboardTopLatencyKey,
  refreshDashboard,
} from "../lib/dashboardCache";
import { pageCachedQueryOptions, wrapPageCachedQueryFn } from "../lib/pageDataCache";
import { displayAlertType } from "../lib/alertLabels";
import { DashboardGeralView } from "./dashboard/DashboardGeralView";
import { DashboardEquipamentosView } from "./dashboard/DashboardEquipamentosView";
import { DashboardFibraView } from "./dashboard/DashboardFibraView";
import { DashboardServidorView } from "./dashboard/DashboardServidorView";
import type { DashboardAnalytics, OltCapacity, TopRow } from "./dashboard/dashboardShared";
import { num, trunc } from "./dashboard/dashboardShared";

// Carregado só quando a aba "Frota" é aberta — evita puxar o código do dashboard de frota
// (gráficos próprios, etc.) para o chunk do Dashboard nas outras 4 vistas.
const FleetDashboardPage = lazy(() =>
  import("./fleet/FleetDashboardPage").then((m) => ({ default: m.FleetDashboardPage })),
);

type DashboardView = "geral" | "equipamentos" | "fibra" | "servidor" | "frota";

const VIEW_LABELS: Record<DashboardView, string> = {
  geral: "Geral",
  equipamentos: "Equipamentos",
  fibra: "Fibra óptica",
  servidor: "Servidor NetQuasar",
  frota: "Frota",
};
const VIEW_ORDER: DashboardView[] = ["geral", "equipamentos", "fibra", "servidor", "frota"];
// Views que partilham os dados agregados de /dashboard/analytics (período/atualizar aplicam-se
// só a estas — "servidor" e "frota" têm as suas próprias fontes de dados e período).
const SHARED_DATA_VIEWS = new Set<DashboardView>(["geral", "equipamentos", "fibra"]);

function DashboardViewTabs({ active, onChange }: { active: DashboardView; onChange: (v: DashboardView) => void }) {
  return (
    <div className="tabs" style={{ marginBottom: 14 }}>
      {VIEW_ORDER.map((v) => (
        <button key={v} type="button" className={active === v ? "active" : ""} onClick={() => onChange(v)}>
          {VIEW_LABELS[v]}
        </button>
      ))}
    </div>
  );
}

export function DashboardPage() {
  const qc = useQueryClient();
  const [view, setView] = useState<DashboardView>("geral");
  const [days, setDays] = useState(DASHBOARD_DEFAULT_DAYS);
  const [catViz, setCatViz] = useState<"pie" | "bar">("pie");
  const [pageIn, setPageIn] = useState(false);
  const [refreshConfirmOpen, setRefreshConfirmOpen] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const dash = useQuery({
    queryKey: dashboardAnalyticsKey(days),
    queryFn: wrapPageCachedQueryFn(dashboardAnalyticsKey(days), () =>
      apiFetch<DashboardAnalytics>(`/api/v1/dashboard/analytics?days=${days}`),
    ),
    ...pageCachedQueryOptions<DashboardAnalytics>(dashboardAnalyticsKey(days), DASHBOARD_STALE_MS, DASHBOARD_GC_MS),
    placeholderData: keepPreviousData,
    enabled: SHARED_DATA_VIEWS.has(view),
  });

  const topProbe = useQuery({
    queryKey: dashboardTopLatencyKey,
    queryFn: wrapPageCachedQueryFn(dashboardTopLatencyKey, () =>
      apiFetch<{ top: TopRow[] }>("/api/v1/overview/top-latency?limit=8"),
    ),
    ...pageCachedQueryOptions<{ top: TopRow[] }>(dashboardTopLatencyKey, DASHBOARD_STALE_MS, DASHBOARD_GC_MS),
    enabled: view === "equipamentos",
  });
  const cap = useQuery({
    queryKey: dashboardOltCapacityKey,
    queryFn: wrapPageCachedQueryFn(dashboardOltCapacityKey, () => apiFetch<OltCapacity>("/api/v1/dashboard/olt-capacity")),
    ...pageCachedQueryOptions<OltCapacity>(dashboardOltCapacityKey, DASHBOARD_STALE_MS, DASHBOARD_GC_MS),
    enabled: view === "fibra",
  });

  const runFullRefresh = async () => {
    setRefreshConfirmOpen(false);
    setRefreshing(true);
    try {
      await refreshDashboard(qc, days);
    } finally {
      setRefreshing(false);
    }
  };

  const totals = dash.data?.totals;

  const catPie = useMemo(() => {
    return (dash.data?.devices_by_category ?? []).map((r) => ({
      name: String(r.category ?? "—"),
      value: num(r.count),
    }));
  }, [dash.data?.devices_by_category]);

  const catBar = useMemo(() => {
    return (dash.data?.devices_by_category ?? []).map((r) => ({
      name: trunc(String(r.category ?? "—"), 14),
      Equipamentos: num(r.count),
    }));
  }, [dash.data?.devices_by_category]);

  const popBar = useMemo(() => {
    return (dash.data?.devices_by_pop ?? [])
      .filter((r) => num(r.count) > 0)
      .map((r) => ({
        name: trunc(String(r.pop_name ?? "POP"), 16),
        Equipamentos: num(r.count),
      }))
      .slice(0, 14);
  }, [dash.data?.devices_by_pop]);

  const locBar = useMemo(() => {
    return (dash.data?.devices_by_locality ?? [])
      .filter((r) => num(r.count) > 0)
      .map((r) => ({
        name: trunc(String(r.locality_name ?? "—"), 16),
        Equipamentos: num(r.count),
      }))
      .slice(0, 14);
  }, [dash.data?.devices_by_locality]);

  const opModeBar = useMemo(() => {
    return (dash.data?.devices_by_operational_mode ?? []).map((r) => ({
      name: String(r.operational_mode ?? "—"),
      Quantidade: num(r.count),
    }));
  }, [dash.data?.devices_by_operational_mode]);

  const alertsPie = useMemo(() => {
    return (dash.data?.alerts_by_type_30d ?? []).map((r) => ({
      name: displayAlertType(r.alert_type),
      value: num(r.count),
      code: String(r.alert_type ?? ""),
    }));
  }, [dash.data?.alerts_by_type_30d]);

  const oltOnuBar = useMemo(() => {
    return (dash.data?.olt_onu_by_device ?? []).map((r) => ({
      name: trunc(String(r.description ?? "?"), 18),
      Online: num(r.onu_online),
      Offline: num(r.onu_offline),
      Total: num(r.onu_count),
      brand: r.brand ?? "",
    }));
  }, [dash.data?.olt_onu_by_device]);

  const oltFleetTotals = useMemo(() => {
    const ft = dash.data?.olt_onu_fleet_totals;
    if (ft) {
      return { total: num(ft.onu_count), online: num(ft.onu_online), offline: num(ft.onu_offline) };
    }
    let total = 0;
    let online = 0;
    let offline = 0;
    for (const r of dash.data?.olt_onu_by_device ?? []) {
      total += num(r.onu_count);
      online += num(r.onu_online);
      offline += num(r.onu_offline);
    }
    return { total, online, offline };
  }, [dash.data?.olt_onu_fleet_totals, dash.data?.olt_onu_by_device]);

  const netDonut = useMemo(() => {
    return (dash.data?.devices_by_network_status ?? []).map((r) => ({
      name: String(r.network_status ?? "—"),
      value: num(r.count),
    }));
  }, [dash.data?.devices_by_network_status]);

  useEffect(() => {
    if (!SHARED_DATA_VIEWS.has(view)) {
      setPageIn(true);
      return;
    }
    if (dash.isLoading && !dash.data) {
      setPageIn(false);
      return;
    }
    if (dash.data && !dash.isError) {
      const id = requestAnimationFrame(() => setPageIn(true));
      return () => cancelAnimationFrame(id);
    }
    setPageIn(false);
  }, [dash.isLoading, dash.data, dash.isError, view]);

  const initialLoading = SHARED_DATA_VIEWS.has(view) && dash.isLoading && !dash.data;
  const isFetching = dash.isFetching || topProbe.isFetching || cap.isFetching || refreshing;

  return (
    <div className={`dashboard-page${pageIn ? " dashboard-page--in" : ""}`}>
      <ConfirmModal
        open={refreshConfirmOpen}
        title="Atualizar dashboard"
        message="Vai recarregar todos os gráficos e indicadores do dashboard. Esta operação pode demorar alguns segundos, conforme o volume de dados no servidor."
        confirmLabel="Atualizar agora"
        cancelLabel="Cancelar"
        busy={refreshing}
        onCancel={() => !refreshing && setRefreshConfirmOpen(false)}
        onConfirm={() => void runFullRefresh()}
      />
      <div className="row dashboard-page__head" style={{ justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 10 }}>
        <h1 style={{ margin: 0, flex: "0 0 auto" }}>
          Dashboard
          <InfoHint label="Sobre o dashboard">
            <p>
              Escolha a vista com as abas abaixo. <strong>Geral</strong>, <strong>Equipamentos</strong> e <strong>Fibra óptica</strong> partilham a
              janela de <strong>{dash.data?.days ?? days}</strong> dias selecionada. <strong>Servidor NetQuasar</strong> mostra a saúde do próprio
              sistema, e <strong>Frota</strong> tem o seu próprio período.
            </p>
          </InfoHint>
        </h1>
        <div style={{ flex: "1 1 320px", display: "flex", justifyContent: "center", minWidth: 0 }}>
          <GlobalSearchBar />
        </div>
        {SHARED_DATA_VIEWS.has(view) ? (
          <button
            type="button"
            className="btn"
            style={{ flex: "0 0 auto" }}
            disabled={refreshing}
            onClick={() => setRefreshConfirmOpen(true)}
            title="Recarregar todos os dados do dashboard"
          >
            <RefreshCw size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
            {refreshing ? "A atualizar…" : "Atualizar dados"}
          </button>
        ) : null}
      </div>

      <DashboardViewTabs active={view} onChange={setView} />

      {SHARED_DATA_VIEWS.has(view) ? (
        <div className="row" style={{ flexWrap: "wrap", gap: 10, alignItems: "center", marginBottom: 8 }}>
          <label style={{ fontSize: 12, color: "var(--muted)" }}>
            Período (dias)
            <select
              className="select"
              style={{ marginLeft: 8, minWidth: 100 }}
              value={days}
              onChange={(e) => setDays(Number(e.target.value) || DASHBOARD_DEFAULT_DAYS)}
              title="Janela temporal das séries"
            >
              {[7, 14, 30, 60, 90].map((d) => (
                <option key={d} value={d}>
                  {d} dias
                </option>
              ))}
            </select>
          </label>
          <span className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
            Gerado: {dash.data?.generated_at ? new Date(dash.data.generated_at).toLocaleString("pt-PT") : "—"}
            {isFetching ? " · a atualizar…" : " · em cache"}
          </span>
        </div>
      ) : null}

      {initialLoading ? (
        <DashboardPageLoader />
      ) : dash.isError && SHARED_DATA_VIEWS.has(view) ? (
        <div className="msg msg--err">{(dash.error as Error).message}</div>
      ) : (
        <>
          {view === "geral" ? (
            <DashboardGeralView
              totals={totals}
              telemetrySamples={dash.data?.telemetry_window?.samples}
              alertsOpen={dash.data?.alerts_open}
              alertsByType={dash.data?.alerts_by_type_30d}
              alertsPie={alertsPie}
            />
          ) : null}
          {view === "equipamentos" ? (
            <DashboardEquipamentosView
              catViz={catViz}
              onCatVizChange={setCatViz}
              catPie={catPie}
              catBar={catBar}
              netDonut={netDonut}
              opModeBar={opModeBar}
              popBar={popBar}
              locBar={locBar}
              worstLatency={dash.data?.ping_ranking_worst_latency}
              bestLatency={dash.data?.ping_ranking_best_latency}
              topLatency={topProbe.data?.top}
              topLatencyLoading={topProbe.isLoading}
              topLatencyError={topProbe.isError ? (topProbe.error as Error).message : null}
            />
          ) : null}
          {view === "fibra" ? (
            <DashboardFibraView
              oltOnuBar={oltOnuBar}
              oltOnuByDevice={dash.data?.olt_onu_by_device}
              oltFleetTotals={oltFleetTotals}
              capacity={cap.data}
              capacityError={cap.isError ? (cap.error as Error).message : null}
            />
          ) : null}
          {view === "servidor" ? <DashboardServidorView /> : null}
          {view === "frota" ? (
            <Suspense fallback={<p>A carregar…</p>}>
              <FleetDashboardPage />
            </Suspense>
          ) : null}
        </>
      )}
    </div>
  );
}
